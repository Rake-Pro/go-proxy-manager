package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
)

// roleEnv mounts the admin server with a stub API handler that answers 200 to
// everything it is reached on. Every non-200 in these tests therefore comes
// from the role/CSRF gates in front of it, which is exactly what is under test.
type roleEnv struct {
	srv   *Server
	store *session.Store
}

func newRoleEnv(t *testing.T) *roleEnv {
	t.Helper()
	return newRoleEnvWith(t, okAPI)
}

// okAPI is the stub API: it answers 200 to everything, so every non-200 in the
// mux-level tests comes from the gates in front of it.
var okAPI = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
})

// scopedAPI is the stub API with the real scope gate applied the way the daemon
// wires it: the first path segment is the scope subject, the method picks the
// verb - which is exactly how internal/api's register() scopes a resource route.
var scopedAPI = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	subject, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	verb := "read"
	if !safeMethod(r.Method) {
		verb = "write"
	}
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "no principal", http.StatusUnauthorized)
		return
	}
	if err := auth.RequireScope(p, subject+":"+verb); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	okAPI(w, r)
})

func newRoleEnvWith(t *testing.T, apiHandler http.Handler) *roleEnv {
	t.Helper()
	st, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	authn := auth.NewAuthenticator(auth.Options{Store: st, SessionTTL: time.Hour})
	return &roleEnv{srv: New(":0", nil, authn, apiHandler, nil, nil, false), store: st}
}

// login creates a session with the given role and returns its cookie value and
// CSRF token.
func (e *roleEnv) login(t *testing.T, role auth.Role) (string, string) {
	t.Helper()
	sess := &session.Session{
		Subject:   "someone@example.com",
		Roles:     []string{string(role)},
		IdP:       "test",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := e.store.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	return sess.ID, sess.CSRFToken
}

func (e *roleEnv) request(t *testing.T, method, target, cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if method != http.MethodGet {
		body = strings.NewReader(`{}`)
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	req.AddCookie(&http.Cookie{Name: "gpm_session", Value: cookie})
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	e.srv.http.Handler.ServeHTTP(rec, req)
	return rec
}

// TestUserRoleRouteTable is the read-only viewer contract at the MUX level: a
// `user` session reaches every GET and is refused on every state-changing
// method, while an admin session is unaffected on both. The stub API handler
// answers 200 to everything, so every non-200 here comes from the method gate
// alone. The per-route SCOPE gate - which additionally refuses
// GET /api/api-tokens to this role - is tested in internal/api
// (TestUserRoleScopeGate) and internal/auth (TestRequireScopeRoleGate), where
// it is actually wired.
func TestUserRoleRouteTable(t *testing.T) {
	reads := []string{
		"/api/proxy-hosts",
		"/api/proxy-hosts/example",
		"/api/certificates",
		"/api/access-lists",
		"/api/settings",
		"/api/runtime",
		"/api/capabilities",
		"/api/config",
		"/api/history",
		"/api/upstream-health",
	}
	writes := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/proxy-hosts/example"},
		{http.MethodDelete, "/api/proxy-hosts/example"},
		{http.MethodPut, "/api/settings"},
		{http.MethodPost, "/api/proxy-hosts/example/revert"},
		{http.MethodPost, "/api/revert"},
		{http.MethodPost, "/api/restore"},
		{http.MethodPut, "/api/api-tokens/ci"},
		{http.MethodPost, "/api/dns-sync/reconcile"},
	}

	roles := []struct {
		role      auth.Role
		wantRead  int
		wantWrite int
	}{
		{auth.RoleUser, http.StatusOK, http.StatusForbidden},
		{auth.RoleAdmin, http.StatusOK, http.StatusOK},
	}
	for _, r := range roles {
		t.Run(string(r.role), func(t *testing.T) {
			e := newRoleEnv(t)
			cookie, csrf := e.login(t, r.role)
			for _, path := range reads {
				t.Run("GET "+path, func(t *testing.T) {
					if got := e.request(t, http.MethodGet, path, cookie, "").Code; got != r.wantRead {
						t.Fatalf("GET %s = %d, want %d", path, got, r.wantRead)
					}
				})
			}
			for _, w := range writes {
				t.Run(w.method+" "+w.path, func(t *testing.T) {
					if got := e.request(t, w.method, w.path, cookie, csrf).Code; got != r.wantWrite {
						t.Fatalf("%s %s = %d, want %d", w.method, w.path, got, r.wantWrite)
					}
				})
			}
		})
	}
}

// TestUserRoleScopeGateComposesAtTheServer wires the real scope gate in front
// of the stub API, the way cmd/gpm does, and checks the one read the role does
// NOT get: API tokens are credentials, not configuration.
func TestUserRoleScopeGateComposesAtTheServer(t *testing.T) {
	tests := []struct {
		name string
		role auth.Role
		path string
		want int
	}{
		{"user is refused the token list", auth.RoleUser, "/api/api-tokens", http.StatusForbidden},
		{"user is refused one token", auth.RoleUser, "/api/api-tokens/ci", http.StatusForbidden},
		{"user keeps the host list", auth.RoleUser, "/api/proxy-hosts", http.StatusOK},
		{"admin keeps the token list", auth.RoleAdmin, "/api/api-tokens", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newRoleEnvWith(t, scopedAPI)
			cookie, _ := e.login(t, tc.role)
			if got := e.request(t, http.MethodGet, tc.path, cookie, "").Code; got != tc.want {
				t.Fatalf("GET %s as %q = %d, want %d", tc.path, tc.role, got, tc.want)
			}
		})
	}
}

// TestUserRoleMeReportsReadOnly checks the flag the SPA gates its UI on.
func TestUserRoleMeReportsReadOnly(t *testing.T) {
	tests := []struct {
		role auth.Role
		want bool
	}{
		{auth.RoleUser, true},
		{auth.RoleAdmin, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.role), func(t *testing.T) {
			e := newRoleEnv(t)
			cookie, _ := e.login(t, tc.role)
			rec := e.request(t, http.MethodGet, "/api/me", cookie, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("GET /api/me = %d, want 200", rec.Code)
			}
			var me struct {
				Role     string `json:"Role"`
				ReadOnly bool   `json:"readOnly"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
				t.Fatalf("decode /api/me: %v (body %s)", err, rec.Body.String())
			}
			if me.Role != string(tc.role) {
				t.Errorf("role = %q, want %q", me.Role, tc.role)
			}
			if me.ReadOnly != tc.want {
				t.Errorf("readOnly = %v, want %v", me.ReadOnly, tc.want)
			}
		})
	}
}

// TestNoRoleIsStillUnauthenticated guards the widened /api/ gate: opening it to
// the user role must not open it to a session with no role at all.
func TestNoRoleIsStillUnauthenticated(t *testing.T) {
	e := newRoleEnv(t)
	cookie, csrf := e.login(t, auth.RoleNone)
	for _, m := range []string{http.MethodGet, http.MethodPut} {
		if got := e.request(t, m, "/api/proxy-hosts/example", cookie, csrf).Code; got != http.StatusUnauthorized {
			t.Fatalf("%s with no role = %d, want 401", m, got)
		}
	}
}
