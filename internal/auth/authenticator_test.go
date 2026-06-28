package auth

import (
	"context"
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

	// SSO-only disables local login entirely (no in-band break-glass door).
	a.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{
		SSOOnly:   true,
		Providers: []string{"x"},
	}})
	if _, err := a.LocalLogin(context.Background(), "admin", "s3cret"); err == nil {
		t.Fatal("SSO-only must disable local login")
	}
	if a.LocalLoginVisible() {
		t.Fatal("SSO-only must hide the local login form")
	}

	// Not SSO-only with local login enabled: correct creds pass, wrong fail.
	a.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}})
	sess, err := a.LocalLogin(context.Background(), "admin", "s3cret")
	if err != nil {
		t.Fatalf("local login should succeed when enabled: %v", err)
	}
	if len(sess.Roles) != 1 || sess.Roles[0] != string(RoleAdmin) {
		t.Fatalf("local admin should get admin role, got %v", sess.Roles)
	}
	if _, err := a.LocalLogin(context.Background(), "admin", "wrong"); err == nil {
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

func TestRequireRoleRejectsForwardAuthHeaders(t *testing.T) {
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
		_, _ = w.Write([]byte("ok"))
	}))

	// Even a trusted peer asserting an admin group header must NOT auto-login:
	// admin forward-auth header login was removed as a spoofing risk.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.RemoteAddr = "10.1.2.3:4000"
	req.Header.Set("X-Authentik-Username", "admin")
	req.Header.Set("X-Authentik-Groups", "proxy-admins")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forward-auth headers must not authenticate the admin panel, got %d", rec.Code)
	}
}

func TestRequireRoleEnforcesCSRF(t *testing.T) {
	st := testStore(t)
	a := NewAuthenticator(Options{Store: st})
	a.Configure(model.Config{}, model.Settings{})

	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	req := func(method, csrf string) *http.Request {
		r := httptest.NewRequest(method, "/api/proxy-hosts/x", nil)
		r.AddCookie(&http.Cookie{Name: "gpm_session", Value: sess.ID})
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		return r
	}
	cases := []struct {
		name   string
		method string
		csrf   string
		want   int
	}{
		{"GET needs no token", "GET", "", http.StatusOK},
		{"mutating without token", "DELETE", "", http.StatusForbidden},
		{"mutating with wrong token", "DELETE", "wrong", http.StatusForbidden},
		{"mutating with valid token", "DELETE", sess.CSRFToken, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req(tc.method, tc.csrf))
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestLoginThrottle(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t)})
	key := "203.0.113.7"
	if a.LoginThrottled(key) {
		t.Fatal("fresh key must not be throttled")
	}
	for i := 0; i < maxLoginFails; i++ {
		if a.LoginThrottled(key) {
			t.Fatalf("must not lock out before %d failures (at %d)", maxLoginFails, i)
		}
		a.NoteLoginResult(key, false)
	}
	if !a.LoginThrottled(key) {
		t.Fatalf("must lock out after %d failures", maxLoginFails)
	}
	// A success clears the gate.
	a.NoteLoginResult(key, true)
	if a.LoginThrottled(key) {
		t.Fatal("a successful login must reset the throttle")
	}
	// A different client is unaffected.
	for i := 0; i < maxLoginFails; i++ {
		a.NoteLoginResult(key, false)
	}
	if a.LoginThrottled("198.51.100.9") {
		t.Fatal("throttle must be per-client-key")
	}
}

func TestHostPrefixCookieWhenSecure(t *testing.T) {
	// Secure cookies take the __Host- prefix.
	a := NewAuthenticator(Options{Store: testStore(t), Secure: true})
	if a.cookieName != "__Host-gpm_session" {
		t.Fatalf("secure cookie should take the __Host- prefix, got %q", a.cookieName)
	}
	// A plain-HTTP deployment (e.g. GPM_COOKIE_SECURE=0) keeps the bare name and
	// is unaffected - a __Host- cookie requires Secure and would be rejected.
	b := NewAuthenticator(Options{Store: testStore(t), Secure: false})
	if b.cookieName != "gpm_session" {
		t.Fatalf("non-secure cookie must not be prefixed, got %q", b.cookieName)
	}
}

func TestSlidingSessionExtendsExpiry(t *testing.T) {
	st := testStore(t)
	a := NewAuthenticator(Options{Store: st, SessionTTL: time.Hour})
	a.Configure(model.Config{}, model.Settings{})

	// A session already past the half-TTL threshold should be extended on use.
	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(10 * time.Minute)}
	if err := st.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: a.cookieName, Value: sess.ID})
	h.ServeHTTP(rec, req)

	got, err := st.Get(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ExpiresAt.After(sess.ExpiresAt) {
		t.Fatalf("session expiry should slide forward: was %v, now %v", sess.ExpiresAt, got.ExpiresAt)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("sliding refresh should re-issue the session cookie")
	}
}
