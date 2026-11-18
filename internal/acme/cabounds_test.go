package acme

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func acmeCert(dirURL string, domains ...string) model.Config {
	return model.Config{Certificates: []model.Certificate{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Type:       model.CertTypeACME,
		Domains:    domains,
		ACME:       &model.ACMESpec{Email: "admin@example.com", DirectoryURL: dirURL},
	}}}
}

// TestIssuedChainIsCheckedBeforeSaving proves a chain whose leaf is for
// another key, or does not cover the requested names, is refused and not
// stored, so the certificate already on disk keeps serving.
func TestIssuedChainIsCheckedBeforeSaving(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir})
	ca.onAccept = func(string, string) error { return nil }

	if _, err := m.EnsureAll(context.Background(), acmeCert(ca.url("/directory"), "app.example.com")); err != nil {
		t.Fatalf("first issuance: %v", err)
	}
	stored, err := os.ReadFile(IssuedCertPath(dir, "app"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		setup   func()
		domains []string
		want    string
	}{
		{"leaf for another key", func() { ca.signOtherKey = true }, []string{"app.example.com", "www.example.com"}, "not for the requested key"},
		{"leaf missing a name", func() { ca.signOtherKey = false; ca.leafNames = []string{"app.example.com"} }, []string{"app.example.com", "www.example.com"}, `does not cover "www.example.com"`},
	}
	for _, tc := range cases {
		tc.setup()
		cfg := acmeCert(ca.url("/directory"), tc.domains...)
		_, err := m.EnsureAll(context.Background(), cfg)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
		now, err := os.ReadFile(IssuedCertPath(dir, "app"))
		if err != nil || !bytes.Equal(now, stored) {
			t.Fatalf("%s: the stored certificate must be untouched (err=%v)", tc.name, err)
		}
		if !m.needsRenewal(cfg.Certificates[0]) {
			t.Fatalf("%s: a refused issuance must leave the certificate due for renewal", tc.name)
		}
	}
}

// TestIssuedChainAcceptsALeafNotYetValidLocally proves the chain check does not
// depend on the local clock being in step with the CA's: a leaf whose validity
// starts after the local "now" is accepted, an expired leaf is refused.
func TestIssuedChainAcceptsALeafNotYetValidLocally(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	leaf := func(notBefore, notAfter time.Time) [][]byte {
		tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"app.example.com"}, NotBefore: notBefore, NotAfter: notAfter}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		return [][]byte{der}
	}
	ahead := leaf(now.Add(2*time.Hour), now.Add(90*24*time.Hour))
	if err := checkIssuedChain(ahead, encodeChain(ahead), key, []string{"app.example.com"}, now); err != nil {
		t.Fatalf("leaf from a CA whose clock is ahead: %v", err)
	}
	expired := leaf(now.Add(-48*time.Hour), now.Add(-time.Hour))
	if err := checkIssuedChain(expired, encodeChain(expired), key, []string{"app.example.com"}, now); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired leaf: err = %v, want an expiry refusal", err)
	}
}

// TestACMEResponsesAreBounded proves a CA's response cannot grow memory or the
// stored error without bound, and a redirect is not followed.
func TestACMEResponsesAreBounded(t *testing.T) {
	chunk := bytes.Repeat([]byte("A"), 64<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect/directory":
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		case "/big-ok/directory":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"newNonce":"`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		for i := 0; i < (4<<20)/len(chunk); i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	cases := []struct{ path, want string }{
		{"/error/directory", ""},
		{"/big-ok/directory", "exceeds"},
		{"/redirect/directory", "redirect"},
	}
	for _, tc := range cases {
		var notified string
		m := NewManager(Options{CertDir: t.TempDir(), OnRenewFailure: func(_ string, err error) { notified = err.Error() }})
		if _, err := m.EnsureAll(context.Background(), acmeCert(srv.URL+tc.path, "x.example.com")); err == nil {
			t.Fatalf("%s: expected an error", tc.path)
		}
		obs := m.CertObservations()
		if len(obs) != 1 {
			t.Fatalf("%s: observations = %+v", tc.path, obs)
		}
		limit := maxStoredError + len(" ... (truncated)")
		if len(obs[0].LastError) > limit || len(notified) > limit {
			t.Fatalf("%s: stored error %d bytes, notified %d bytes, want at most %d", tc.path, len(obs[0].LastError), len(notified), limit)
		}
		if tc.want != "" && !strings.Contains(obs[0].LastError, tc.want) {
			t.Fatalf("%s: LastError = %.200q, want it to mention %q", tc.path, obs[0].LastError, tc.want)
		}
	}
}
