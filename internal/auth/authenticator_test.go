package auth

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
	"golang.org/x/crypto/bcrypt"
)

func testStore(t *testing.T) *session.Store {
	t.Helper()
	st, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestLocalLoginPolicy(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	a := NewAuthenticator(Options{Store: testStore(t), LocalUser: "admin", LocalHash: string(hash)})

	// SSO-only with loopback break-glass: deny from LAN, allow from loopback.
	a.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{
		SSOOnly:    true,
		Providers:  []string{"x"},
		BreakGlass: model.BreakGlassSettings{LocalhostOnly: true},
	}})

	if _, err := a.LocalLogin(context.Background(), "admin", "s3cret", net.ParseIP("10.0.0.9")); err == nil {
		t.Fatal("SSO-only must deny local login from a non-loopback address")
	}
	sess, err := a.LocalLogin(context.Background(), "admin", "s3cret", net.ParseIP("127.0.0.1"))
	if err != nil {
		t.Fatalf("loopback break-glass should allow local login: %v", err)
	}
	if len(sess.Roles) != 1 || sess.Roles[0] != string(RoleAdmin) {
		t.Fatalf("local admin should get admin role, got %v", sess.Roles)
	}

	// Wrong password is rejected even from loopback.
	if _, err := a.LocalLogin(context.Background(), "admin", "wrong", net.ParseIP("127.0.0.1")); err == nil {
		t.Fatal("wrong password must be rejected")
	}
}

func TestRequireRoleWithSession(t *testing.T) {
	st := testStore(t)
	a := NewAuthenticator(Options{Store: st})
	a.Configure(model.Config{}, model.Settings{})

	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		_, _ = w.Write([]byte(p.Subject))
	}))

	// No cookie -> 401.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session should be 401, got %d", rec.Code)
	}

	// Valid admin session -> 200.
	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "gpm_session", Value: sess.ID})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "admin" {
		t.Fatalf("valid admin session should pass, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestRequireRoleForwardAuthAutoLogin(t *testing.T) {
	st := testStore(t)
	a := NewAuthenticator(Options{Store: st})
	a.Configure(model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta: model.ObjectMeta{Name: "authentik"},
			Type:       model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{
				TrustedProxies: []string{"10.0.0.0/8"},
				UserHeader:     "X-Authentik-Username",
				GroupsHeader:   "X-Authentik-Groups",
			},
			RoleMapping: &model.RoleMapping{AdminGroups: []string{"proxy-admins"}},
		}},
	}, model.Settings{AdminAuth: model.AdminAuthSettings{Providers: []string{"authentik"}}})

	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		_, _ = w.Write([]byte(p.Subject))
	}))

	// Trusted peer with admin group header -> auto-login, 200, Set-Cookie issued.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.RemoteAddr = "10.1.2.3:4000"
	req.Header.Set("X-Authentik-Username", "admin")
	req.Header.Set("X-Authentik-Groups", "proxy-admins")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "admin" {
		t.Fatalf("trusted forward-auth should auto-login admin, got %d %q", rec.Code, rec.Body.String())
	}
	if rec.Result().Cookies() == nil || len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a session cookie to be issued on forward-auth auto-login")
	}

	// Untrusted peer asserting the same header -> 401 (no spoofing).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/me", nil)
	req.RemoteAddr = "203.0.113.5:4000"
	req.Header.Set("X-Authentik-Username", "attacker")
	req.Header.Set("X-Authentik-Groups", "proxy-admins")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("untrusted forward-auth header must not authenticate, got %d", rec.Code)
	}
}
