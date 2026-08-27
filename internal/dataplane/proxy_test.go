package dataplane

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func backendUpstream(t *testing.T, h http.Handler) (model.Upstream, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	return model.Upstream{Scheme: "http", Host: u.Hostname(), Port: port}, srv.Close
}

func TestProxyEndToEnd(t *testing.T) {
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-Host") == "" {
			t.Error("expected X-Forwarded-Host to be set by the proxy")
		}
		w.Header().Set("X-Backend", "yes")
		_, _ = w.Write([]byte("backend says hi"))
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app2.example.com"},
		Upstream:   up,
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://app2.example.com/path", nil)
	req.Host = "app2.example.com"
	rt.serveHTTPS(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if rec.Body.String() != "backend says hi" {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
	if rec.Header().Get("X-Backend") != "yes" {
		t.Fatal("expected upstream response header to pass through")
	}
}

func TestProxyLocationRouting(t *testing.T) {
	root, closeRoot := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("root:" + r.URL.Path))
	}))
	defer closeRoot()
	reports, closeReports := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("reports:" + r.URL.Path))
	}))
	defer closeReports()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "claude"},
		Domains:    []string{"app1.example.com"},
		Upstream:   root,
		Locations: []model.Location{{
			Path:     "/reports",
			Upstream: &reports,
		}},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	cases := []struct {
		path, want string
	}{
		{"/", "root:/"},
		{"/terminal", "root:/terminal"},
		{"/reports/", "reports:/reports/"},                 // prefix routed to the sidecar
		{"/reports/a/b.html", "reports:/reports/a/b.html"}, // path forwarded unchanged
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "https://app1.example.com"+c.path, nil)
		req.Host = "app1.example.com"
		rt.serveHTTPS(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d, want 200", c.path, rec.Code)
		}
		if rec.Body.String() != c.want {
			t.Fatalf("%s: body %q, want %q", c.path, rec.Body.String(), c.want)
		}
	}
}

func TestProxyLocationPathNormalization(t *testing.T) {
	root, closeRoot := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("root:" + r.URL.Path))
	}))
	defer closeRoot()
	reports, closeReports := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("reports:" + r.URL.Path))
	}))
	defer closeReports()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "claude"},
		Domains:    []string{"app1.example.com"},
		Upstream:   root,
		Locations:  []model.Location{{Path: "/reports", Upstream: &reports}},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	cases := []struct {
		name, rawTarget, want string
	}{
		{"exact prefix routes to location", "/reports", "reports:/reports"},
		{"boundary blocks sibling over-match", "/reports-evil", "root:/reports-evil"},
		{"dot-segment cleaned then routed", "/x/../reports/a", "reports:/reports/a"},
		{"dot-segment cleaned at root", "/a/../b", "root:/b"},
		{"encoded traversal decoded and cleaned", "/%2e%2e/reports/z", "reports:/reports/z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "https://app1.example.com"+c.rawTarget, nil)
			req.Host = "app1.example.com"
			rt.serveHTTPS(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: got %d, want 200 (body %q)", c.rawTarget, rec.Code, rec.Body.String())
			}
			if rec.Body.String() != c.want {
				t.Fatalf("%s: body %q, want %q", c.rawTarget, rec.Body.String(), c.want)
			}
		})
	}
}

func TestProxyHSTS(t *testing.T) {
	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()
	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{
			ObjectMeta: model.ObjectMeta{Name: "secure"},
			Domains:    []string{"secure.example.com"},
			Upstream:   up,
			TLS:        model.TLSSettings{HSTS: model.HSTS{Enabled: true, IncludeSubdomains: true, Preload: true}},
		},
		{
			ObjectMeta: model.ObjectMeta{Name: "plain"},
			Domains:    []string{"plain.example.com"},
			Upstream:   up,
		},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	serve := func(host string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "https://"+host+"/", nil)
		req.Host = host
		rt.serveHTTPS(rec, req)
		return rec.Header().Get("Strict-Transport-Security")
	}
	if got := serve("secure.example.com"); got != "max-age=31536000; includeSubDomains; preload" {
		t.Fatalf("HSTS header = %q, want default with includeSubDomains+preload", got)
	}
	if got := serve("plain.example.com"); got != "" {
		t.Fatalf("HSTS must not be sent when disabled, got %q", got)
	}

	// HSTS is not emitted over plain HTTP.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://secure.example.com/", nil)
	req.Host = "secure.example.com"
	rt.serveHTTP(rec, req)
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS must not be sent over plain HTTP, got %q", got)
	}
}

func TestProxyLocationInheritsHostAccessList(t *testing.T) {
	reports, closeReports := backendUpstream(t, okHandler())
	defer closeReports()

	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "deny-all"},
			DefaultAction: model.ActionDeny,
			Rules:         []model.IPRule{{Action: model.ActionDeny, CIDR: "0.0.0.0/0"}},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "claude"},
			Domains:     []string{"app1.example.com"},
			Upstream:    reports,
			AccessLists: []string{"deny-all"},
			Locations:   []model.Location{{Path: "/reports", Upstream: &reports}},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://app1.example.com/reports/", nil)
	req.Host = "app1.example.com"
	req.RemoteAddr = "203.0.113.7:1234"
	rt.serveHTTPS(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("location must inherit host access list: got %d, want 403", rec.Code)
	}
}

func TestProxyUnknownHost(t *testing.T) {
	rt, _ := buildRouter(model.Config{}, "", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://nope.example.com/", nil)
	req.Host = "nope.example.com"
	rt.serveHTTPS(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestProxyWithAccessListAndHeaders(t *testing.T) {
	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()

	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "deny-all"},
			DefaultAction: model.ActionDeny,
			Rules:         []model.IPRule{{Action: model.ActionDeny, CIDR: "0.0.0.0/0"}},
		}},
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "sec-headers"},
			Type:       model.MWTypeHeaders,
			Headers:    &model.HeadersMiddleware{SetResponse: map[string]string{"X-Frame-Options": "DENY"}},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"app2.example.com"},
			Upstream:    up,
			AccessLists: []string{"deny-all"},
			Middlewares: []string{"sec-headers"},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://app2.example.com/", nil)
	req.Host = "app2.example.com"
	req.RemoteAddr = "203.0.113.7:1234"
	rt.serveHTTPS(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected access list to deny (403), got %d", rec.Code)
	}
}

func TestCertResolverSNI(t *testing.T) {
	dir := t.TempDir()
	writeSelfSigned(t, dir, "*.example.com")

	certs := []model.Certificate{{
		ObjectMeta: model.ObjectMeta{Name: "wild"},
		Type:       model.CertTypeCustom,
		Domains:    []string{"*.example.com"},
		Custom:     &model.CustomCertSpec{CertFile: "cert.pem", KeyFile: "key.pem"},
	}}
	r, err := buildCertResolver(certs, dir)
	if err != nil {
		t.Fatalf("buildCertResolver: %v", err)
	}
	if _, err := r.GetCertificate(&tls.ClientHelloInfo{ServerName: "app2.example.com"}); err != nil {
		t.Fatalf("expected wildcard match for app2.example.com: %v", err)
	}
	if _, err := r.GetCertificate(&tls.ClientHelloInfo{ServerName: "other.example"}); err == nil {
		t.Fatal("expected no cert for unrelated domain")
	}
}

func writeSelfSigned(t *testing.T, dir, domain string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{domain},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A client must not be able to delete the identity gpm asserted by naming it in
// the Connection header. net/http/httputil strips every header the Connection
// header lists BEFORE the Rewrite hook runs, so "Connection: X-Forwarded-User"
// used to reach the upstream as an anonymous request on a gated route - an
// authentication bypass against any backend that trusts gpm's identity header
// and falls back to a guest identity when it is absent.
func TestProxyReassertsIdentityStrippedByConnectionHeader(t *testing.T) {
	var got http.Header
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer closeFn()

	h := newReverseProxy(up, "app", nil, []string{"X-Forwarded-User", "X-Forwarded-Groups", "X-Client-Cert-Subject"}, nil, nil)

	r := httptest.NewRequest("GET", "http://app.example.com/x", nil)
	// What gpm's auth tier asserted...
	r.Header.Set("X-Forwarded-User", "alice@example.com")
	r.Header.Set("X-Forwarded-Groups", "admins")
	// ...and the client's attempt to have it dropped on the way to the upstream.
	r.Header.Set("Connection", "X-Forwarded-User, X-Forwarded-Groups")
	h.ServeHTTP(httptest.NewRecorder(), r)

	if got.Get("X-Forwarded-User") != "alice@example.com" {
		t.Fatalf("upstream saw X-Forwarded-User %q, want the identity gpm asserted", got.Get("X-Forwarded-User"))
	}
	if got.Get("X-Forwarded-Groups") != "admins" {
		t.Fatalf("upstream saw X-Forwarded-Groups %q, want \"admins\"", got.Get("X-Forwarded-Groups"))
	}
	// An asserted name gpm did NOT set stays unset: the restore only ever copies
	// values that are actually on the inbound request.
	if _, ok := got["X-Client-Cert-Subject"]; ok {
		t.Fatalf("a header gpm never set was invented on the outbound request: %q", got.Get("X-Client-Cert-Subject"))
	}
	// Connection itself is still hop-by-hop and must not reach the upstream.
	if v := got.Get("Connection"); v != "" {
		t.Fatalf("Connection header forwarded upstream: %q", v)
	}
}

// The restore must never resurrect a header the router stripped: on a host with
// no identity provider, a client's forged X-Forwarded-User is stripped and must
// stay gone, Connection trick or not.
func TestProxyDoesNotResurrectStrippedClientIdentity(t *testing.T) {
	var got http.Header
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   up,
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	r := httptest.NewRequest("GET", "http://app.example.com/x", nil)
	r.Host = "app.example.com"
	r.RemoteAddr = "198.18.0.9:1234"
	r.Header.Set("X-Forwarded-User", "evil@example.com")
	r.Header.Set("Connection", "X-Forwarded-User")
	rt.serveHTTP(httptest.NewRecorder(), r)

	if v := got.Get("X-Forwarded-User"); v != "" {
		t.Fatalf("a forged client identity reached the upstream: %q", v)
	}
}

// The set of names the proxy is allowed to restore is exactly the set gpm may
// assert: the baseline denylist, the host's provider headers, and its
// client-certificate subject header.
func TestAssertedIdentityHeaders(t *testing.T) {
	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "fa"},
			Type:        model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{UserHeader: "X-Custom-User", TrustedProxies: []string{"10.0.0.0/8"}},
		}},
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "sso"},
			Type:       model.MWTypeAuth,
			Auth:       &model.AuthMiddleware{Mode: model.AuthModeForwardAuth, IdentityProvider: "fa"},
		}},
	}
	host := model.ProxyHost{
		ObjectMeta:  model.ObjectMeta{Name: "app"},
		Domains:     []string{"app.example.com"},
		Middlewares: []string{"sso"},
		TLS: model.TLSSettings{ClientAuth: &model.ClientAuth{
			CARef:           "ca",
			IdentityHeaders: &model.ClientCertHeaders{SubjectHeader: "X-My-Cert-Subject"},
		}},
	}
	names := assertedIdentityHeaders(host, buildRegistry(cfg))
	has := func(n string) bool {
		for _, got := range names {
			if got == n {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"X-Forwarded-User", "X-Forwarded-Email", "X-Forwarded-Groups", "X-Custom-User", "X-My-Cert-Subject"} {
		if !has(want) {
			t.Errorf("asserted set is missing %q: %v", want, names)
		}
	}
	if has("X-Forwarded-For") {
		t.Errorf("X-Forwarded-For must not be in the asserted set (SetXForwarded owns it): %v", names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("asserted set contains %q twice: %v", n, names)
		}
		seen[n] = true
	}
}
