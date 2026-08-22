package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// fakeChallengeStore is a static token -> key authorization map.
type fakeChallengeStore map[string]string

func (f fakeChallengeStore) KeyAuth(token string) (string, bool) {
	v, ok := f[token]
	return v, ok
}

// acmeTestServer builds a data plane with one force-SSL proxy host, so any
// request that reaches routing is answered with a 301 to https - which makes the
// challenge path's precedence unambiguous.
func acmeTestServer(t *testing.T) *Server {
	t.Helper()
	s := New(Config{})
	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "192.0.2.10", Port: 8080},
		TLS:        model.TLSSettings{ForceSSL: true},
	}}}
	if err := s.Reload(cfg); err != nil {
		t.Fatalf("reload: %v", err)
	}
	return s
}

func doHTTP(s *Server, path, host string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
	req.Host = host
	s.dispatchHTTP(rec, req)
	return rec
}

func TestACMEChallengeServedBeforeRouting(t *testing.T) {
	s := acmeTestServer(t)
	s.SetACMEChallengeStore(fakeChallengeStore{"tok123": "tok123.keyauth"})

	// 1. In-flight token on a force-SSL host: served in the clear, no redirect.
	rec := doHTTP(s, "/.well-known/acme-challenge/tok123", "app.example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("challenge on force-SSL host: status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "tok123.keyauth" {
		t.Errorf("body = %q, want the key authorization", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}

	// 2. A host with no route at all still answers the challenge (a certificate
	// can be issued before its host exists).
	if rec := doHTTP(s, "/.well-known/acme-challenge/tok123", "new.example.com"); rec.Code != http.StatusOK {
		t.Errorf("challenge on unrouted host: status = %d, want 200", rec.Code)
	}

	// 3. Unknown token falls through to normal routing (an upstream may run its
	// own ACME client), so the force-SSL redirect still applies.
	if rec := doHTTP(s, "/.well-known/acme-challenge/other", "app.example.com"); rec.Code != http.StatusPermanentRedirect {
		t.Errorf("unknown token: status = %d, want 308 (fall through to routing)", rec.Code)
	}

	// 4. A nested path under the prefix is not a token.
	if rec := doHTTP(s, "/.well-known/acme-challenge/tok123/extra", "app.example.com"); rec.Code != http.StatusPermanentRedirect {
		t.Errorf("nested challenge path: status = %d, want 308", rec.Code)
	}

	// 5. An empty token is not served.
	if rec := doHTTP(s, "/.well-known/acme-challenge/", "app.example.com"); rec.Code != http.StatusPermanentRedirect {
		t.Errorf("empty token: status = %d, want 308", rec.Code)
	}

	// 6. Ordinary traffic is unaffected.
	if rec := doHTTP(s, "/anything", "app.example.com"); rec.Code != http.StatusPermanentRedirect {
		t.Errorf("normal request: status = %d, want 308", rec.Code)
	}
}

func TestACMEChallengeNoStore(t *testing.T) {
	s := acmeTestServer(t)
	// Never wired: the challenge path routes like any other request.
	if rec := doHTTP(s, "/.well-known/acme-challenge/tok123", "app.example.com"); rec.Code != http.StatusPermanentRedirect {
		t.Errorf("no store: status = %d, want 308", rec.Code)
	}
	// Wired then detached.
	s.SetACMEChallengeStore(fakeChallengeStore{"tok123": "ka"})
	if rec := doHTTP(s, "/.well-known/acme-challenge/tok123", "app.example.com"); rec.Code != http.StatusOK {
		t.Fatalf("wired: status = %d, want 200", rec.Code)
	}
	s.SetACMEChallengeStore(nil)
	if rec := doHTTP(s, "/.well-known/acme-challenge/tok123", "app.example.com"); rec.Code != http.StatusPermanentRedirect {
		t.Errorf("detached: status = %d, want 308", rec.Code)
	}
}

func TestACMEChallengeOnlyPlaintextListener(t *testing.T) {
	// The HTTPS dispatcher has no challenge hook: HTTP-01 is validated over :80
	// only, so a TLS request for the path routes normally (here: 404, no host).
	s := acmeTestServer(t)
	s.SetACMEChallengeStore(fakeChallengeStore{"tok123": "ka"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://nope.example.com/.well-known/acme-challenge/tok123", nil)
	req.Host = "nope.example.com"
	s.dispatchHTTPS(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("https challenge path: status = %d, want 404", rec.Code)
	}
}
