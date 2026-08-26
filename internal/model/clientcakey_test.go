package model

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testCA generates a self-signed CA in-process and returns its certificate PEM
// and PKCS#8 key PEM. isCA=false produces a leaf, for the "the key matches a
// certificate that is not a CA" case.
func testCA(t *testing.T, cn string, isCA bool) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  isCA,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pk8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk8}))
}

// TestClientCASigningKeyValidation covers the optional signing key at config
// validation: the two sources are mutually exclusive, caKeyFile is confined to
// the cert store exactly like crlFile, and an inline key that does not belong to
// any certificate in the bundle is refused before it can be committed.
func TestClientCASigningKeyValidation(t *testing.T) {
	caPEM, keyPEM := testCA(t, "corp-ca", true)
	_, otherKey := testCA(t, "other-ca", true)
	leafPEM, leafKey := testCA(t, "not-a-ca", false)

	tests := []struct {
		name    string
		ca      ClientCA
		wantErr string
	}{
		{"no signing key is still valid", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM}, ""},
		{"matching inline key", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM, CAKeyPEM: Secret(keyPEM)}, ""},
		{"relative caKeyFile", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM, CAKeyFile: "corp-ca.key"}, ""},
		{"both sources", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyFile: "corp-ca.key", CAKeyPEM: Secret(keyPEM)}, "not both"},
		{"absolute caKeyFile", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyFile: "/etc/ssl/private/ca.key"}, "must be relative to the cert store"},
		{"traversal caKeyFile", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyFile: "../../etc/ssl/private/ca.key"}, "must be relative to the cert store"},
		{"key does not match the bundle", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyPEM: Secret(otherKey)}, "does not match any certificate in caPEM"},
		{"key matches a non-CA certificate", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: leafPEM,
			CAKeyPEM: Secret(leafKey)}, "is not a CA"},
		{"unparseable inline key", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyPEM: Secret("-----BEGIN PRIVATE KEY-----\nZm9v\n-----END PRIVATE KEY-----\n")},
			"does not parse as a PKCS#8"},
		{"inline key with no PEM block", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyPEM: Secret("not a key at all")}, "no PEM private-key block"},
		// A placeholder that cannot be resolved in this process is deferred, not
		// rejected - the same contract caPEM's parse check already follows.
		{"unresolvable placeholder is deferred", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyPEM: Secret("${FILE:/run/secrets/corp_ca.key}")}, ""},
		// The expiry warning window is advisory, but a nonsensical one would mark
		// every certificate expiring (or never), so it is bounded.
		{"default warning window", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM}, ""},
		{"custom warning window", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			ExpiryWarningDays: 7}, ""},
		{"maximum warning window", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			ExpiryWarningDays: MaxExpiryWarningDays}, ""},
		{"negative warning window", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			ExpiryWarningDays: -1}, "expiryWarningDays must be between"},
		{"warning window over the cap", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			ExpiryWarningDays: MaxExpiryWarningDays + 1}, "expiryWarningDays must be between"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ca.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// TestClientCASigningKeyLoad covers resolving the key at use time: a verify-only
// CA reports ErrNoSigningKey, a cert-store-relative file loads and matches its
// issuer, and a path escape is refused even if the object never went through
// Validate.
func TestClientCASigningKeyLoad(t *testing.T) {
	caPEM, keyPEM := testCA(t, "corp-ca", true)
	certDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(certDir, "corp-ca.key"), []byte(keyPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	// A key parked outside the cert store, to prove the escape is refused rather
	// than merely failing to open.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "ca.key"), []byte(keyPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("verify-only CA", func(t *testing.T) {
		ca := ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM}
		if ca.HasSigningKey() {
			t.Fatal("HasSigningKey must be false with no key configured")
		}
		if _, _, err := ca.SigningKey(certDir); !errors.Is(err, ErrNoSigningKey) {
			t.Fatalf("want ErrNoSigningKey, got %v", err)
		}
	})

	t.Run("caKeyFile resolves against the cert store", func(t *testing.T) {
		ca := ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM, CAKeyFile: "corp-ca.key"}
		if !ca.HasSigningKey() {
			t.Fatal("HasSigningKey must be true with caKeyFile set")
		}
		issuer, signer, err := ca.SigningKey(certDir)
		if err != nil {
			t.Fatalf("SigningKey: %v", err)
		}
		if issuer.Subject.CommonName != "corp-ca" {
			t.Fatalf("issuer CN = %q", issuer.Subject.CommonName)
		}
		pub, ok := issuer.PublicKey.(interface{ Equal(any) bool })
		if ok && !pub.Equal(signer.Public()) {
			t.Fatal("resolved signer does not belong to the issuer certificate")
		}
	})

	t.Run("absolute caKeyFile refused at load", func(t *testing.T) {
		ca := ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyFile: filepath.Join(outside, "ca.key")}
		_, _, err := ca.SigningKey(certDir)
		if err == nil || !strings.Contains(err.Error(), "must be relative to the cert store") {
			t.Fatalf("want confinement error, got %v", err)
		}
	})

	t.Run("traversal caKeyFile refused at load", func(t *testing.T) {
		ca := ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM,
			CAKeyFile: "../" + filepath.Base(outside) + "/ca.key"}
		_, _, err := ca.SigningKey(certDir)
		if err == nil || !strings.Contains(err.Error(), "must be relative to the cert store") {
			t.Fatalf("want confinement error, got %v", err)
		}
	})

	t.Run("missing caKeyFile", func(t *testing.T) {
		ca := ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: caPEM, CAKeyFile: "absent.key"}
		if _, _, err := ca.SigningKey(certDir); err == nil {
			t.Fatal("expected an error for a missing key file")
		}
	})
}

// TestClientCAKeyRedactedInJSON proves the signing key follows the Secret
// contract: an inline literal never leaves the process in an API response, and a
// placeholder round-trips so the UI can edit it.
func TestClientCAKeyRedactedInJSON(t *testing.T) {
	_, keyPEM := testCA(t, "corp-ca", true)
	b, err := Secret(keyPEM).MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"***"` {
		t.Fatalf("literal key marshalled as %s, want the redaction sentinel", b)
	}
	if b, err = Secret("${FILE:/run/secrets/corp_ca.key}").MarshalJSON(); err != nil {
		t.Fatal(err)
	}
	if string(b) != `"${FILE:/run/secrets/corp_ca.key}"` {
		t.Fatalf("placeholder marshalled as %s, want it verbatim", b)
	}
}

// TestClientCAWarningDays pins the effective warning window: unset (and any
// non-positive value the validator would have rejected) falls back to the
// package default, a set value is used as-is.
func TestClientCAWarningDays(t *testing.T) {
	cases := []struct {
		set  int
		want int
	}{
		{0, DefaultExpiryWarningDays},
		{7, 7},
		{90, 90},
		{MaxExpiryWarningDays, MaxExpiryWarningDays},
	}
	for _, tc := range cases {
		ca := ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", ExpiryWarningDays: tc.set}
		if got := ca.WarningDays(); got != tc.want {
			t.Fatalf("expiryWarningDays %d -> WarningDays %d, want %d", tc.set, got, tc.want)
		}
	}
}
