package dataplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// outpostStub is a mock Authentik proxy outpost. authStatus/authHeaders drive the
// /auth/nginx verdict; any other path under the prefix is treated as a passthrough
// endpoint (sign-in, callback) and echoes a marker so tests can detect it.
type outpostStub struct {
	authStatus  int
	authHeaders map[string]string
	authCookie  string
	gotCookie   string // Cookie header the auth subrequest carried
	gotOriginal string // X-Original-URL the auth subrequest carried
}

func (o *outpostStub) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/outpost.goauthentik.io/auth/nginx" {
			o.gotCookie = r.Header.Get("Cookie")
			o.gotOriginal = r.Header.Get("X-Original-URL")
			for k, v := range o.authHeaders {
				w.Header().Set(k, v)
			}
			if o.authCookie != "" {
				w.Header().Add("Set-Cookie", o.authCookie)
			}
			w.WriteHeader(o.authStatus)
			return
		}
		// Outpost-owned endpoint (e.g. /start, /callback).
		_, _ = w.Write([]byte("OUTPOST:" + r.URL.Path))
	})
}

func newAuthReqProxy(t *testing.T, outpostURL string) *authRequestProxy {
	t.Helper()
	p, err := compileAuthRequest(model.AuthRequestSpec{OutpostURL: outpostURL})
	if err != nil {
		t.Fatalf("compileAuthRequest: %v", err)
	}
	return p
}

// recordingBackend captures the identity headers the upstream would receive.
func recordingBackend(seen *http.Header) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = r.Header.Clone()
		_, _ = w.Write([]byte("BACKEND"))
	})
}

func TestAuthRequestAuthenticated(t *testing.T) {
	stub := &outpostStub{
		authStatus:  http.StatusOK,
		authHeaders: map[string]string{"X-authentik-username": "admin", "X-authentik-email": "admin@example.com"},
		authCookie:  "authentik_session=refreshed; Path=/",
	}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	var seen http.Header
	h := newAuthReqProxy(t, srv.URL).handler("app", recordingBackend(&seen))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app2.example.com/dashboard?x=1", nil)
	req.Header.Set("Cookie", "authentik_session=abc")
	req.Header.Set("X-authentik-username", "attacker") // forged; must be replaced
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "BACKEND" {
		t.Fatalf("expected backend reached, got %d %q", rec.Code, rec.Body.String())
	}
	if got := seen.Get("X-authentik-username"); got != "admin" {
		t.Fatalf("upstream username = %q, want admin (forged value must be overwritten)", got)
	}
	if got := seen.Values("X-authentik-username"); len(got) != 1 {
		t.Fatalf("expected exactly one username header, got %v", got)
	}
	if got := seen.Get("X-authentik-email"); got != "admin@example.com" {
		t.Fatalf("upstream email = %q", got)
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "refreshed") {
		t.Fatalf("expected refreshed session cookie passed to client, got %q", rec.Header().Get("Set-Cookie"))
	}
	if stub.gotCookie != "authentik_session=abc" {
		t.Fatalf("auth subrequest cookie = %q", stub.gotCookie)
	}
	if stub.gotOriginal != "http://app2.example.com/dashboard?x=1" {
		t.Fatalf("auth subrequest X-Original-URL = %q", stub.gotOriginal)
	}
}

func TestAuthRequestStripsForgedHeaderWhenAuthSilent(t *testing.T) {
	// Auth says 200 but asserts no username: the client's forged header must not
	// survive to the upstream.
	stub := &outpostStub{authStatus: http.StatusOK}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	var seen http.Header
	h := newAuthReqProxy(t, srv.URL).handler("app", recordingBackend(&seen))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app2.example.com/", nil)
	req.Header.Set("X-authentik-username", "attacker")
	h.ServeHTTP(rec, req)

	if got := seen.Get("X-authentik-username"); got != "" {
		t.Fatalf("forged username leaked to upstream: %q", got)
	}
}

func TestAuthRequestUnauthenticatedRedirects(t *testing.T) {
	stub := &outpostStub{authStatus: http.StatusUnauthorized, authCookie: "authentik_csrf=z; Path=/"}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	backendHit := false
	h := newAuthReqProxy(t, srv.URL).handler("app", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { backendHit = true }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app2.example.com/secret/page", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("got %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	want := "/outpost.goauthentik.io/start?rd=" + "http%3A%2F%2Fapp2.example.com%2Fsecret%2Fpage"
	if loc != want {
		t.Fatalf("redirect = %q, want %q", loc, want)
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "authentik_csrf") {
		t.Fatalf("expected auth cookie carried on redirect, got %q", rec.Header().Get("Set-Cookie"))
	}
	if backendHit {
		t.Fatal("backend must not be reached when unauthenticated")
	}
}

func TestAuthRequestForbidden(t *testing.T) {
	stub := &outpostStub{authStatus: http.StatusForbidden}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	backendHit := false
	h := newAuthReqProxy(t, srv.URL).handler("app", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { backendHit = true }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app2.example.com/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
	if backendHit {
		t.Fatal("backend must not be reached on 403")
	}
}

func TestAuthRequestOutpostPassthrough(t *testing.T) {
	stub := &outpostStub{authStatus: http.StatusOK}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	backendHit := false
	h := newAuthReqProxy(t, srv.URL).handler("app", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { backendHit = true }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app2.example.com/outpost.goauthentik.io/start?rd=/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "OUTPOST:") {
		t.Fatalf("expected outpost passthrough, got %d %q", rec.Code, rec.Body.String())
	}
	if backendHit {
		t.Fatal("outpost path must not reach the backend")
	}
}

func TestAuthRequestBackendUnavailable(t *testing.T) {
	// Point at a closed server: the auth subrequest must fail closed (502), never
	// fall through to the backend.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	backendHit := false
	h := newAuthReqProxy(t, url).handler("app", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { backendHit = true }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app2.example.com/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
	if backendHit {
		t.Fatal("backend must not be reached when the auth server is down")
	}
}
