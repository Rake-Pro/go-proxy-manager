package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
)

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/login", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"font-src 'self'; img-src 'self' data:; " +
			"connect-src 'self'; object-src 'none'; base-uri 'none'; " +
			"form-action 'self'; frame-ancestors 'none'",
		"Referrer-Policy": "same-origin",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	// The panel vendors its fonts, so the CSP must name no external origin at
	// all: every host in it is a third party the admin browser would talk to.
	if csp := rec.Header().Get("Content-Security-Policy"); strings.Contains(csp, "//") {
		t.Errorf("CSP must have no external origins, got %q", csp)
	}
	// HSTS must NOT be set by the admin server: the proxy listeners are the TLS edge and
	// own it; emitting it here duplicated the header on the proxied admin path.
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("admin server must not set Strict-Transport-Security, got %q", got)
	}
}

func TestPprofDisabledByDefault(t *testing.T) {
	sessStore, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sessStore.Close() })
	authn := localLoginAuthenticator(auth.Options{Store: sessStore})

	s := New(":0", nil, authn, nil, nil, nil, false)
	rec := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/debug/pprof/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("pprof disabled: expected 404, got %d", rec.Code)
	}
}

func TestPprofEnabledIsAdminGated(t *testing.T) {
	sessStore, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sessStore.Close() })
	authn := localLoginAuthenticator(auth.Options{Store: sessStore})

	s := New(":0", nil, authn, nil, nil, nil, true)

	// No session -> 401, not the profiling data.
	rec := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/debug/pprof/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("pprof enabled, no session: expected 401, got %d", rec.Code)
	}

	// Valid admin session -> 200 with the pprof index body.
	sess := &session.Session{Subject: "admin", Roles: []string{string(auth.RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := sessStore.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req.AddCookie(&http.Cookie{Name: "gpm_session", Value: sess.ID})
	s.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pprof enabled, admin session: expected 200, got %d", rec.Code)
	}
}

// Every API-token principal is admin-ROLE by construction, so the role gate
// alone would hand a resource-scoped token the heap profile and the process
// command line - which carry resolved Cloudflare/Pi-hole secrets in cleartext.
// pprof therefore requires the admin SCOPE from a token principal.
func TestPprofRequiresAdminScopeForTokens(t *testing.T) {
	sessStore, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sessStore.Close() })

	scopedSecret, scopedHash, err := auth.NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	adminSecret, adminHash, err := auth.NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	authn := localLoginAuthenticator(auth.Options{Store: sessStore})
	authn.SetTokenSource(func() []model.APIToken {
		return []model.APIToken{
			{ObjectMeta: model.ObjectMeta{Name: "reader"}, TokenHash: scopedHash, Scopes: []string{"proxy-hosts:read"}},
			{ObjectMeta: model.ObjectMeta{Name: "root"}, TokenHash: adminHash, Scopes: []string{model.ScopeAdmin}},
		}
	})
	s := New(":0", nil, authn, nil, nil, nil, true)

	for _, tc := range []struct {
		name       string
		secret     string
		path       string
		wantStatus int
	}{
		{"resource-scoped token is refused", scopedSecret, "/debug/pprof/", http.StatusForbidden},
		{"resource-scoped token cannot read cmdline", scopedSecret, "/debug/pprof/cmdline", http.StatusForbidden},
		{"admin-scoped token is allowed", adminSecret, "/debug/pprof/", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+tc.secret)
			s.http.Handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s = %d, want %d", tc.path, rec.Code, tc.wantStatus)
			}
		})
	}
}

// The admin server bounds reading a request, body included: a request whose
// declared body never arrives is ended by the server rather than held for as
// long as the client keeps the socket open.
func TestAdminServerEndsARequestWhoseBodyNeverArrives(t *testing.T) {
	e := newTestEnv(t, envOpts{})
	if e.srv.http.ReadTimeout <= 0 {
		t.Fatal("admin server has no ReadTimeout")
	}
	e.srv.http.ReadTimeout = 500 * time.Millisecond // shortened so the test is fast
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = e.srv.http.Serve(ln) }()
	t.Cleanup(func() { _ = e.srv.http.Close() })

	for _, head := range []string{
		"POST /auth/local HTTP/1.1\r\nHost: x\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 5\r\n\r\n",
		"POST /api/proxy-hosts HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n",
	} {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(c, head); err != nil {
			t.Fatal(err)
		}
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, err = io.ReadAll(c)
		c.Close()
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatalf("%q: the server held the request with its body outstanding", head)
		}
	}
}

// localLoginAuthenticator is an authenticator with local login configured and
// allowed, so a test can create "local" sessions directly: a session is only
// honoured while the sign-in method it came from is allowed.
func localLoginAuthenticator(o auth.Options) *auth.Authenticator {
	o.LocalUser, o.LocalHash = "admin", "$2a$10$placeholder"
	a := auth.NewAuthenticator(o)
	a.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}})
	return a
}
