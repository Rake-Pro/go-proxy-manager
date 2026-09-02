package auth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
	_ "modernc.org/sqlite"

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
	if _, _, err := a.LocalLogin(context.Background(), "admin", "s3cret"); err == nil {
		t.Fatal("SSO-only must disable local login")
	}
	if a.LocalLoginVisible() {
		t.Fatal("SSO-only must hide the local login form")
	}

	// Not SSO-only with local login enabled: correct creds pass, wrong fail.
	a.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}})
	sess, _, err := a.LocalLogin(context.Background(), "admin", "s3cret")
	if err != nil {
		t.Fatalf("local login should succeed when enabled: %v", err)
	}
	if len(sess.Roles) != 1 || sess.Roles[0] != string(RoleAdmin) {
		t.Fatalf("local admin should get admin role, got %v", sess.Roles)
	}
	if _, _, err := a.LocalLogin(context.Background(), "admin", "wrong"); err == nil {
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

func TestRateGateWithinAndOverLimit(t *testing.T) {
	g := newRateGate(time.Hour, 3, 100)
	key := "203.0.113.7"
	cases := []struct {
		recordFirst bool // record one event before checking
		wantAtLimit bool
	}{
		{false, false}, // fresh key: not at limit
		{true, false},  // 1 event: under limit
		{true, false},  // 2 events: under limit
		{true, true},   // 3 events: at limit
		{true, true},   // 4 events: still at limit
	}
	for i, tc := range cases {
		if tc.recordFirst {
			g.record(key)
		}
		if got := g.atLimit(key, false); got != tc.wantAtLimit {
			t.Fatalf("step %d: atLimit=%v want %v", i, got, tc.wantAtLimit)
		}
	}
	// A different key is unaffected by another key's count.
	if g.atLimit("198.51.100.9", false) {
		t.Fatal("rate gate must be per-key")
	}
}

func TestRateGateWindowReset(t *testing.T) {
	g := newRateGate(time.Hour, 2, 100)
	key := "203.0.113.7"
	g.record(key)
	g.record(key)
	if !g.atLimit(key, false) {
		t.Fatal("must be at limit after reaching the limit")
	}
	// Force the window to expire.
	g.entries[key].resetAt = time.Now().Add(-time.Minute)
	if g.atLimit(key, false) {
		t.Fatal("expired window must no longer be at limit")
	}
	// Recording after expiry starts a fresh window at one event, not accumulating.
	g.record(key)
	if g.entries[key].fails != 1 {
		t.Fatalf("expired entry should reset to 1 fail, got %d", g.entries[key].fails)
	}
	if g.atLimit(key, false) {
		t.Fatal("one event in a fresh window must be under a limit of 2")
	}
}

func TestRateGateAtLimitEviction(t *testing.T) {
	// atLimit(evictExpired=false) leaves an expired entry in place; evictExpired=true
	// deletes it. Both return false (not at limit) for the expired key.
	for _, evict := range []bool{false, true} {
		g := newRateGate(time.Hour, 1, 100)
		key := "203.0.113.7"
		g.record(key)
		g.entries[key].resetAt = time.Now().Add(-time.Minute)
		if g.atLimit(key, evict) {
			t.Fatalf("evict=%v: expired key must not be at limit", evict)
		}
		_, present := g.entries[key]
		if present == evict {
			t.Fatalf("evict=%v: entry present=%v (want deletion only when evict)", evict, present)
		}
	}
}

func TestRateGateEvictsStaleKeysOnRecord(t *testing.T) {
	g := newRateGate(time.Hour, 5, 4)
	// Fill to capacity with expired entries.
	past := time.Now().Add(-time.Minute)
	for _, k := range []string{"a", "b", "c", "d"} {
		g.record(k)
		g.entries[k].resetAt = past
	}
	if len(g.entries) != 4 {
		t.Fatalf("setup: want 4 entries, got %d", len(g.entries))
	}
	// A new key at capacity triggers eviction of the expired entries, then records.
	g.record("new")
	if _, ok := g.entries["new"]; !ok {
		t.Fatal("new key must be recorded after stale entries are evicted")
	}
	if g.entries["new"].fails != 1 {
		t.Fatalf("new key should have 1 fail, got %d", g.entries["new"].fails)
	}
}

func TestRateGateMapFullSkipsRecord(t *testing.T) {
	g := newRateGate(time.Hour, 5, 4)
	// Fill to capacity with LIVE (non-expired) entries; nothing is evictable.
	for _, k := range []string{"a", "b", "c", "d"} {
		g.record(k)
	}
	if len(g.entries) != 4 {
		t.Fatalf("setup: want 4 entries, got %d", len(g.entries))
	}
	// A new key must be skipped (not recorded) rather than grow the bounded map
	// past capacity.
	g.record("new")
	if _, ok := g.entries["new"]; ok {
		t.Fatal("new key must be skipped when the map is full of live entries")
	}
	if len(g.entries) != 4 {
		t.Fatalf("map must stay at capacity, got %d", len(g.entries))
	}
	// That skip must NOT be a bypass: atLimit fails closed for an untracked key
	// while the map is saturated, so a distinct-key flood becomes a lockout, not an
	// unthrottled brute-force (GPM-L2).
	if !g.atLimit("new", false) {
		t.Fatal("untracked key must be reported at-limit while the map is saturated")
	}
	// An EXISTING key at capacity is still counted (no new allocation needed).
	g.record("a")
	if g.entries["a"].fails != 2 {
		t.Fatalf("existing key must still increment when map is full, got %d", g.entries["a"].fails)
	}
}

func TestRateGateClear(t *testing.T) {
	g := newRateGate(time.Hour, 1, 100)
	key := "203.0.113.7"
	g.record(key)
	if !g.atLimit(key, false) {
		t.Fatal("must be at limit after reaching limit of 1")
	}
	g.clear(key)
	if g.atLimit(key, false) {
		t.Fatal("clear must reset the gate")
	}
	if _, ok := g.entries[key]; ok {
		t.Fatal("clear must remove the entry")
	}
}

func TestHostPrefixCookieWhenSecure(t *testing.T) {
	// The authenticator carries both names; which one is written is decided per
	// request (see cookiesecure_test.go). A Secure cookie takes the __Host-
	// prefix, a plain-HTTP one must keep the bare name or the browser drops it.
	a := NewAuthenticator(Options{Store: testStore(t), SecureMode: CookieSecureAlways})
	if a.cookieNameFor(true) != "__Host-gpm_session" {
		t.Fatalf("secure cookie should take the __Host- prefix, got %q", a.cookieNameFor(true))
	}
	if a.cookieNameFor(false) != "gpm_session" {
		t.Fatalf("non-secure cookie must not be prefixed, got %q", a.cookieNameFor(false))
	}
	// A caller that supplies an already-prefixed name gets the same pair back,
	// never "__Host-__Host-gpm_session".
	b := NewAuthenticator(Options{Store: testStore(t), CookieName: "__Host-gpm_session"})
	if b.cookieNameFor(true) != "__Host-gpm_session" || b.cookieNameFor(false) != "gpm_session" {
		t.Fatalf("prefixed CookieName not normalized: %q / %q", b.cookieNameFor(true), b.cookieNameFor(false))
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

func TestSessionWithoutCSRFRejected(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")

	st, err := session.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	st.Close()

	// Simulate a legacy/migrated session row with no CSRF token (store.Create
	// always generates one, so blank it directly through a separate handle).
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET csrf_token = '' WHERE id = ?`, sess.ID); err != nil {
		t.Fatal(err)
	}
	db.Close()

	st2, err := session.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	a := NewAuthenticator(Options{Store: st2})
	a.Configure(model.Config{}, model.Settings{})

	h := a.RequireRole(RoleUser, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: a.cookieName, Value: sess.ID})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("a session without a CSRF token must force re-login (401), got %d", rec.Code)
	}
}
