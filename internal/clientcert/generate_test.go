package clientcert

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestGenerateRequestNormalize is the unit table for the generate request, so the
// rules are pinned here rather than only through the handler.
func TestGenerateRequestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		req     GenerateRequest
		wantCN  string
		wantDay int
		wantErr string
	}{
		{name: "empty defaults to the object name", req: GenerateRequest{},
			wantCN: "corp", wantDay: DefaultCAValidityDays},
		{name: "blank common name defaults too", req: GenerateRequest{CommonName: "   "},
			wantCN: "corp", wantDay: DefaultCAValidityDays},
		{name: "explicit values", req: GenerateRequest{CommonName: "Corp CA", ValidityDays: 30, Organization: "Corp"},
			wantCN: "Corp CA", wantDay: 30},
		{name: "minimum validity", req: GenerateRequest{ValidityDays: MinCAValidityDays},
			wantCN: "corp", wantDay: MinCAValidityDays},
		{name: "maximum validity", req: GenerateRequest{ValidityDays: MaxCAValidityDays},
			wantCN: "corp", wantDay: MaxCAValidityDays},

		{name: "over-long common name", req: GenerateRequest{CommonName: strings.Repeat("a", MaxCommonNameLen+1)},
			wantErr: "commonName must be at most"},
		{name: "control character in common name", req: GenerateRequest{CommonName: "a\nb"},
			wantErr: "commonName contains control characters"},
		{name: "over-long organization", req: GenerateRequest{Organization: strings.Repeat("a", MaxOrganizationLen+1)},
			wantErr: "organization must be at most"},
		{name: "control character in organization", req: GenerateRequest{Organization: "a\rb"},
			wantErr: "organization contains control characters"},
		{name: "validity below the floor", req: GenerateRequest{ValidityDays: -1},
			wantErr: "validityDays must be between"},
		{name: "validity over the cap", req: GenerateRequest{ValidityDays: MaxCAValidityDays + 1},
			wantErr: "validityDays must be between"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			err := req.normalize("corp")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
				}
				if !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("every normalize failure must wrap ErrInvalidRequest, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected success, got: %v", err)
			}
			if req.CommonName != tc.wantCN {
				t.Fatalf("commonName = %q, want %q", req.CommonName, tc.wantCN)
			}
			if req.ValidityDays != tc.wantDay {
				t.Fatalf("validityDays = %d, want %d", req.ValidityDays, tc.wantDay)
			}
		})
	}
}

// TestKeyFileForIsConfined proves the derived key path is always cert-store
// relative, and that a name which WOULD escape the store is refused rather than
// silently written somewhere else. Object names are lowercase alphanumeric plus
// "-_.", which permits an embedded ".." - the confinement rule catches it.
func TestKeyFileForIsConfined(t *testing.T) {
	certDir := t.TempDir()
	for _, name := range []string{"corp", "a.b", "lab-01", "x_y"} {
		got := KeyFileFor(name)
		if got != "client-cas/"+name+".key" {
			t.Fatalf("KeyFileFor(%q) = %q", name, got)
		}
		if filepath.IsAbs(got) || strings.Contains(filepath.Clean(got), "..") {
			t.Fatalf("KeyFileFor(%q) is not confined: %q", name, got)
		}
	}
	// A name with an embedded ".." passes ValidateName but its derived key path
	// does not pass cert-store confinement, so generation refuses it outright.
	_, err := GenerateCA("a..b", certDir, GenerateRequest{})
	if err == nil || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest for a name whose key path is unconfined, got %v", err)
	}
	// The refusal talks about the NAME the caller sent, not about caKeyFile - a
	// field this request has no way to set.
	for _, want := range []string{`"a..b"`, "not confined to the certificate store", "choose a name without"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should mention %q, got %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "caKeyFile") {
		t.Fatalf("error must not blame caKeyFile, which the caller never sent: %v", err)
	}
	if entries, err := os.ReadDir(certDir); err == nil && len(entries) != 0 {
		t.Fatalf("a refused generate must write nothing, found %d entries", len(entries))
	}
	// A bad object name is refused the same way.
	if _, err := GenerateCA("Corp!", certDir, GenerateRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want ErrInvalidRequest for an invalid name, got %v", err)
	}
	// So is an unwired certificate store: there is nowhere to put the key.
	if _, err := GenerateCA("corp", "", GenerateRequest{}); !errors.Is(err, ErrNoStore) {
		t.Fatalf("want ErrNoStore with no cert dir, got %v", err)
	}
}

// TestGenerateCAWritesUsableKey proves the generated key is on disk at 0600, in
// a form SigningKey can load, and that it matches the returned certificate - the
// contract the ClientCA object saved afterwards depends on.
func TestGenerateCAWritesUsableKey(t *testing.T) {
	certDir := t.TempDir()
	res, err := GenerateCA("corp", certDir, GenerateRequest{ValidityDays: 30})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if res.KeyFile != "client-cas/corp.key" {
		t.Fatalf("KeyFile = %q", res.KeyFile)
	}
	keyPath := filepath.Join(certDir, res.KeyFile)
	fi, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v, want 0600", fi.Mode().Perm())
	}

	// The generated pair round-trips through the real loader.
	ca := model.ClientCA{
		ObjectMeta: model.ObjectMeta{Name: "corp"},
		CAPEM:      res.CertPEM,
		CAKeyFile:  res.KeyFile,
	}
	issuer, signer, err := ca.SigningKey(certDir)
	if err != nil {
		t.Fatalf("the generated CA must load as a signing key: %v", err)
	}
	block, _ := pem.Decode([]byte(res.CertPEM))
	want, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !issuer.Equal(want) {
		t.Fatal("SigningKey resolved a different issuer than the generated certificate")
	}
	if pub, ok := issuer.PublicKey.(interface{ Equal(any) bool }); ok && !pub.Equal(signer.Public()) {
		t.Fatal("the stored key does not belong to the generated certificate")
	}

	// A second generate for the same name never clobbers the first key.
	before, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateCA("corp", certDir, GenerateRequest{}); !errors.Is(err, ErrKeyFileExists) {
		t.Fatalf("want ErrKeyFileExists, got %v", err)
	}
	after, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a refused second generate must leave the existing key byte-for-byte intact")
	}

	// RemoveGeneratedKey is the rollback, and is idempotent.
	if err := RemoveGeneratedKey("corp", certDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("key should be gone, stat err = %v", err)
	}
	if err := RemoveGeneratedKey("corp", certDir); err != nil {
		t.Fatalf("removing an absent key must be a no-op, got %v", err)
	}
	if err := RemoveGeneratedKey("corp", ""); err != nil {
		t.Fatalf("removing with no cert dir must be a no-op, got %v", err)
	}
}

// TestGenerateSweepsStaleTempFiles proves the janitorial sweep: a temp file left
// behind by an interrupted generate holds a raw, unencrypted RSA private key at a
// name nothing references, so an abandoned one is removed. A RECENT temp file is
// left alone, because it may belong to a generate running right now and deleting
// it would break that request.
func TestGenerateSweepsStaleTempFiles(t *testing.T) {
	certDir := t.TempDir()
	dir := filepath.Join(certDir, "client-cas")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, ".tmp-abandoned")
	fresh := filepath.Join(dir, ".tmp-inflight")
	keeper := filepath.Join(dir, "other.key")
	for _, p := range []string{stale, fresh, keeper} {
		if err := os.WriteFile(p, []byte("KEY MATERIAL"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * staleTempAge)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	// A real key file that happens to be old is NOT a temp file and must survive.
	if err := os.Chtimes(keeper, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := GenerateCA("corp", certDir, GenerateRequest{ValidityDays: 1}); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("an abandoned temp file must be swept, stat err = %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("a recent temp file may belong to a concurrent generate and must survive: %v", err)
	}
	if _, err := os.Stat(keeper); err != nil {
		t.Fatalf("an old real key file is not a temp file and must survive: %v", err)
	}
	// The sweep is best effort and never fails the operation: a missing directory
	// is simply nothing to do.
	sweepStaleTemps(filepath.Join(t.TempDir(), "absent"))
}
