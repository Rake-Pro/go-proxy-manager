package model

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func testRootPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func acmeCertWith(mut func(*ACMESpec)) Certificate {
	spec := &ACMESpec{Email: "admin@example.com"}
	mut(spec)
	return Certificate{ObjectMeta: ObjectMeta{Name: "app"}, Type: CertTypeACME, Domains: []string{"app.example.com"}, ACME: spec}
}

func TestACMEProfileValidation(t *testing.T) {
	for _, p := range []string{"", "classic", "shortlived", "tls-server_2", strings.Repeat("a", 64)} {
		if err := acmeCertWith(func(s *ACMESpec) { s.Profile = p }).Validate(); err != nil {
			t.Errorf("profile %q: Validate() = %v, want nil", p, err)
		}
	}
	for _, p := range []string{strings.Repeat("a", 65), "short lived", "tls.server", "profil\u00e9", "a/b"} {
		err := acmeCertWith(func(s *ACMESpec) { s.Profile = p }).Validate()
		want := `certificate "app": acme.profile: must be 1 to 64 characters of letters, digits, "_" or "-", got`
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("profile %q: Validate() = %v, want %q", p, err, want)
		}
	}
}

func TestACMECABundleValidation(t *testing.T) {
	root := testRootPEM(t)
	other := testRootPEM(t)
	keyBlock := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte{1, 2, 3}}))
	badCert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not der")}))

	ok := map[string]string{
		"one certificate":        root,
		"two certificates":       root + other,
		"surrounding whitespace": "\n  " + root + "\n\n" + other + "  \n",
		"file placeholder":       "${FILE:/run/secrets/private-ca.pem}",
		"env placeholder":        "${ENV:GPM_PRIVATE_CA}",
	}
	for name, b := range ok {
		if err := acmeCertWith(func(s *ACMESpec) { s.CABundle = b }).Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want nil", name, err)
		}
	}

	bad := []struct {
		name, bundle, want string
	}{
		{"not pem", "hello", `certificate "app": acme.caBundle: contains text that is not a PEM CERTIFICATE block`},
		{"text before", "my root\n" + root, "contains text that is not a PEM CERTIFICATE block"},
		{"text after", root + "trailing", "contains text that is not a PEM CERTIFICATE block"},
		{"text between", root + "x\n" + other, "contains text that is not a PEM CERTIFICATE block"},
		{"truncated block", root + "-----BEGIN CERTIFICATE-----\nAAAA\n", "contains text that is not a PEM CERTIFICATE block"},
		{"private key", root + keyBlock, `certificate "app": acme.caBundle: contains a "EC PRIVATE KEY" PEM block; only CERTIFICATE blocks are allowed`},
		{"unparsable certificate", badCert, `certificate "app": acme.caBundle: certificate 1 does not parse`},
		{"whitespace only", "  \n ", `certificate "app": acme.caBundle: contains no PEM CERTIFICATE block`},
		{"placeholder mixed with pem", root + "${FILE:/run/secrets/x.pem}", "contains text that is not a PEM CERTIFICATE block"},
		{"too large", strings.Repeat(root, 64<<10/len(root)+1), `certificate "app": acme.caBundle: must be at most 65536 bytes, got`},
	}
	for _, tc := range bad {
		err := acmeCertWith(func(s *ACMESpec) { s.CABundle = tc.bundle }).Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate() = %v, want %q", tc.name, err, tc.want)
		}
	}
}

// TestCABundleIsNotASecret: an inline bundle is committed as written and
// returned by the API as written; it is a trust anchor, not a credential.
func TestCABundleIsNotASecret(t *testing.T) {
	c := acmeCertWith(func(s *ACMESpec) { s.CABundle = testRootPEM(t) })
	if paths := LiteralSecrets(c); len(paths) != 0 {
		t.Errorf("LiteralSecrets = %v, want none for an inline CA bundle", paths)
	}
}

// TestAllowInsecureLocalPlaceholderAtWriteTime: write-time validation sees
// the raw value, so a placeholder there is never read as the opt-in, whatever
// it resolves to later; a loopback literal URL is still refused.
func TestAllowInsecureLocalPlaceholderAtWriteTime(t *testing.T) {
	for _, flag := range []Secret{"${ENV:GPM_ALLOW_LOCAL}", "${FILE:/run/secrets/true}", "${ENV:true}"} {
		if IsTruthyConfig(string(flag)) {
			t.Errorf("IsTruthyConfig(%q) = true, want false", flag)
		}
		for provider, cfg := range map[string]map[string]Secret{
			DNSProviderACMEDNS: {"baseURL": "http://127.0.0.1:8053", "username": "u", "password": "${ENV:P}", "subdomain": "s", "allowInsecureLocal": flag},
			DNSProviderWebhook: {"url": "http://127.0.0.1:8053/hook", "token": "${ENV:T}", "allowInsecureLocal": flag},
		} {
			err := DNSProvider{ObjectMeta: ObjectMeta{Name: "dns"}, Provider: provider, Config: cfg}.Validate()
			if err == nil || !strings.Contains(err.Error(), "points at the loopback address") {
				t.Errorf("%s with allowInsecureLocal %q: Validate() = %v, want the loopback refusal", provider, flag, err)
			}
		}
	}
}
