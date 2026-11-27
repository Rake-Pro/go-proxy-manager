package acme

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// privateTLSServer is an HTTPS server whose certificate chains to a fresh
// test root; it returns the server and that root as PEM.
func privateTLSServer(t *testing.T, h http.Handler) (*httptest.Server, string) {
	t.Helper()
	pair, rootPEM := testServerCert(t)
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{pair}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, rootPEM
}

// TestACMEHTTPClientForBundle: with acme.caBundle the client trusts the bundle
// and nothing else, and keeps the shared client's redirect refusal, body cap,
// timeout and environment proxy settings.
func TestACMEHTTPClientForBundle(t *testing.T) {
	chunk := bytes.Repeat([]byte("A"), 64<<10)
	srv, rootPEM := privateTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/big":
			for i := 0; i < (2*maxACMEBody)/len(chunk); i++ {
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		default:
			_, _ = io.WriteString(w, "fine")
		}
	}))
	_, otherRoot := testServerCert(t)

	get := func(c *http.Client, path string) (string, error) {
		res, err := c.Get(srv.URL + path)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		return string(b), err
	}

	withBundle, err := acmeHTTPClientFor(&model.ACMESpec{CABundle: rootPEM})
	if err != nil {
		t.Fatal(err)
	}
	if body, err := get(withBundle, "/ok"); err != nil || body != "fine" {
		t.Fatalf("with the bundle: body = %q, err = %v; want the exchange to work", body, err)
	}

	if _, err := get(acmeHTTPClient, "/ok"); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("without the bundle (system roots): err = %v, want a certificate verification failure", err)
	}
	other, err := acmeHTTPClientFor(&model.ACMESpec{CABundle: otherRoot})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := get(other, "/ok"); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("bundle of another CA: err = %v, want a certificate verification failure", err)
	}

	if _, err := get(withBundle, "/redirect"); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Errorf("redirect: err = %v, want it refused", err)
	}
	if _, err := get(withBundle, "/big"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("oversized body: err = %v, want the cap to apply", err)
	}
	if withBundle.Timeout != acmeHTTPTimeout {
		t.Errorf("timeout = %s, want %s", withBundle.Timeout, acmeHTTPTimeout)
	}
	tr, ok := withBundle.Transport.(cappedTransport).base.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("transport = %T, want an *http.Transport that keeps proxy settings from the environment", withBundle.Transport)
	}
	// The pool holds the bundle and nothing else: no system roots added.
	certs, err := model.ParseCABundle(rootPEM)
	if err != nil {
		t.Fatal(err)
	}
	want := x509.NewCertPool()
	for _, c := range certs {
		want.AddCert(c)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.RootCAs.Equal(want) {
		t.Error("the bundle client's root pool is not exactly the bundle")
	}
	if sys, err := x509.SystemCertPool(); err == nil && tr.TLSClientConfig.RootCAs.Equal(sys) {
		t.Error("the bundle client trusts the system roots")
	}

	// No bundle: the shared client itself.
	if c, err := acmeHTTPClientFor(&model.ACMESpec{}); err != nil || c != acmeHTTPClient {
		t.Errorf("no bundle: client = %p, err = %v; want the shared client", c, err)
	}
}

func TestACMEHTTPClientForBundlePlaceholders(t *testing.T) {
	_, rootPEM := testServerCert(t)
	root := t.TempDir()
	t.Setenv("GPM_SECRET_FILE_ROOTS", root)
	file := filepath.Join(root, "private-ca.pem")
	if err := os.WriteFile(file, []byte(rootPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GPM_TEST_PRIVATE_CA", rootPEM)

	for _, b := range []string{"${FILE:" + file + "}", "${ENV:GPM_TEST_PRIVATE_CA}"} {
		if _, err := acmeHTTPClientFor(&model.ACMESpec{CABundle: b}); err != nil {
			t.Errorf("%s: %v", b, err)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "junk.pem"), []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The size cap also holds for what a placeholder resolves to.
	if err := os.WriteFile(filepath.Join(root, "huge.pem"), []byte(strings.Repeat(rootPEM, 64<<10/len(rootPEM)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := []struct{ bundle, want string }{
		{"${FILE:" + filepath.Join(root, "huge.pem") + "}", "acme.caBundle: must be at most 65536 bytes, got"},
		{"${ENV:GPM_TEST_UNSET_PRIVATE_CA}", `acme.caBundle: secret env var "GPM_TEST_UNSET_PRIVATE_CA" is not set`},
		{"${FILE:" + filepath.Join(root, "junk.pem") + "}", "acme.caBundle: contains text that is not a PEM CERTIFICATE block"},
		{"${FILE:/etc/hostname}", "acme.caBundle: secret file \"/etc/hostname\" is outside the allowed secret roots"},
	}
	for _, tc := range bad {
		if _, err := acmeHTTPClientFor(&model.ACMESpec{CABundle: tc.bundle}); err == nil || !strings.HasPrefix(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.bundle, err, tc.want)
		}
	}
}

// TestEnsureAllWithCABundle runs a full order against a private CA served
// over HTTPS: it works with the bundle, fails without it or with another CA's,
// and a bundle that does not resolve fails the attempt before any request.
func TestEnsureAllWithCABundle(t *testing.T) {
	_, otherRoot := testServerCert(t)
	cases := []struct {
		name   string
		bundle func(root string) string
		want   string // empty: issued
	}{
		{"bundle of the CA", func(root string) string { return root }, ""},
		{"no bundle", func(string) string { return "" }, "certificate"},
		{"bundle of another CA", func(string) string { return otherRoot }, "certificate"},
		{"unresolvable bundle", func(string) string { return "${ENV:GPM_TEST_UNSET_PRIVATE_CA}" }, `certificate "app": acme.caBundle: secret env var "GPM_TEST_UNSET_PRIVATE_CA" is not set`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ca, root := newTLSStubCA(t, []string{"http-01"})
			m := NewManager(Options{CertDir: t.TempDir()})
			cfg := acmeCert(ca.url("/directory"), "app.example.com")
			cfg.Certificates[0].ACME.CABundle = tc.bundle(root)
			changed, err := m.EnsureAll(context.Background(), cfg)
			if tc.want == "" {
				if err != nil || !changed {
					t.Fatalf("EnsureAll: changed = %v, err = %v; want issued", changed, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("EnsureAll err = %v, want it to contain %q", err, tc.want)
			}
			if obs := m.CertObservations(); len(obs) != 1 || obs[0].RenewFailures != 1 {
				t.Errorf("observations = %+v, want one recorded failure", obs)
			}
			if strings.HasPrefix(tc.name, "unresolvable") && ca.directoryGets != 0 {
				t.Errorf("directory fetched %d times with an unresolvable bundle, want none", ca.directoryGets)
			}
		})
	}
}
