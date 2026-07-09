package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
)

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/login", nil))

	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "frame-ancestors 'none'",
		"Referrer-Policy":         "same-origin",
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
