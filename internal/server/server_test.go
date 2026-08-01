package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; " +
			"connect-src 'self'; object-src 'none'; base-uri 'none'; " +
			"form-action 'self'; frame-ancestors 'none'",
		"Referrer-Policy": "same-origin",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	// HSTS must NOT be set by the admin server: the data plane is the TLS edge and
	// owns it; emitting it here duplicated the header on the proxied admin path.
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
	authn := auth.NewAuthenticator(auth.Options{Store: sessStore})

	s := New(":0", nil, authn, nil, nil, false)
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
	authn := auth.NewAuthenticator(auth.Options{Store: sessStore})

	s := New(":0", nil, authn, nil, nil, true)

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
	authn := auth.NewAuthenticator(auth.Options{Store: sessStore})
	authn.SetTokenSource(func() []model.APIToken {
		return []model.APIToken{
			{ObjectMeta: model.ObjectMeta{Name: "reader"}, TokenHash: scopedHash, Scopes: []string{"proxy-hosts:read"}},
			{ObjectMeta: model.ObjectMeta{Name: "root"}, TokenHash: adminHash, Scopes: []string{model.ScopeAdmin}},
		}
	})
	s := New(":0", nil, authn, nil, nil, true)

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
