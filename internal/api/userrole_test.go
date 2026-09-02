package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// roleHandler builds an API handler whose scope gate is bound to a SESSION
// principal of the given role, which is what the daemon does for a browser
// login. It is the mirror of scopedHandler (which models an API token).
func roleHandler(t *testing.T, role auth.Role) http.Handler {
	t.Helper()
	return roleHandlerOn(t, newRoleStore(t), role)
}

func newRoleStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	return st
}

// roleHandlerOn is roleHandler against an existing store, so two roles can read
// the same config.
func roleHandlerOn(t *testing.T, st *store.Store, role auth.Role) http.Handler {
	t.Helper()
	p := auth.Principal{Role: role, Subject: "viewer@example.com"}
	return api.New(api.Deps{
		Store:        st,
		RequireScope: func(_ *http.Request, required string) error { return auth.RequireScope(p, required) },
	})
}

// TestUserRoleScopeGate is the route-level half of the read-only role: the
// scope gate the daemon wires refuses api-tokens and every write to a `user`
// session while leaving the ordinary reads open. The method-level guard in
// internal/server is tested separately; either one alone is sufficient to
// refuse a write.
func TestUserRoleScopeGate(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"list hosts", "GET", "/proxy-hosts", "", http.StatusOK},
		{"list certificates", "GET", "/certificates", "", http.StatusOK},
		{"read settings", "GET", "/settings", "", http.StatusOK},
		{"whole config", "GET", "/config", "", http.StatusOK},
		{"history", "GET", "/history", "", http.StatusOK},
		{"capabilities", "GET", "/capabilities", "", http.StatusOK},

		// API tokens are credentials, not configuration: excluded from the
		// read-only role in both directions.
		{"list api tokens", "GET", "/api-tokens", "", http.StatusForbidden},
		{"read one api token", "GET", "/api-tokens/ci", "", http.StatusForbidden},
		{"api token history", "GET", "/api-tokens/ci/history", "", http.StatusForbidden},
		{"mint an api token", "PUT", "/api-tokens/ci", validToken, http.StatusForbidden},
		{"delete an api token", "DELETE", "/api-tokens/ci", "", http.StatusForbidden},

		// Writes, and the admin-scoped reads.
		{"write a host", "PUT", "/proxy-hosts/app", validProxyHost, http.StatusForbidden},
		{"delete a host", "DELETE", "/proxy-hosts/app", "", http.StatusForbidden},
		{"write settings", "PUT", "/settings", "{}", http.StatusForbidden},
		{"whole-config revert", "POST", "/revert", "{}", http.StatusForbidden},
		{"backup download", "GET", "/backup", "", http.StatusForbidden},
	}
	h := roleHandler(t, auth.RoleUser)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := do(t, h, tc.method, tc.path, tc.body).Code; got != tc.want {
				t.Fatalf("%s %s = %d, want %d", tc.method, tc.path, got, tc.want)
			}
		})
	}
}

// TestAdminRoleKeepsAPITokens is the guard against the exclusion leaking into
// the admin role: an admin session still lists tokens.
func TestAdminRoleKeepsAPITokens(t *testing.T) {
	h := roleHandler(t, auth.RoleAdmin)
	for _, path := range []string{"/api-tokens", "/config", "/backup"} {
		if got := do(t, h, "GET", path, "").Code; got != http.StatusOK {
			t.Errorf("admin GET %s = %d, want 200", path, got)
		}
	}
}

// TestUserRoleConfigOmitsAPITokens checks the exclusion is not cosmetic: the
// rows refused at /api-tokens are not handed over by asking for the whole tree
// instead.
func TestUserRoleConfigOmitsAPITokens(t *testing.T) {
	// Mint a token as an admin so there is something to leak.
	st := newRoleStore(t)
	admin := roleHandlerOn(t, st, auth.RoleAdmin)
	if w := do(t, admin, "PUT", "/api-tokens/ci", validToken); w.Code != http.StatusOK {
		t.Fatalf("seed token: %d %s", w.Code, w.Body.String())
	}
	if body := do(t, admin, "GET", "/config", "").Body.String(); !strings.Contains(body, `"ci"`) {
		t.Fatalf("admin GET /config does not carry the seeded token, so the test proves nothing: %s", body)
	}

	// The same store, read through a user-role principal.
	user := roleHandlerOn(t, st, auth.RoleUser)
	w := do(t, user, "GET", "/config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("user GET /config = %d, want 200", w.Code)
	}
	var cfg struct {
		APITokens []any `json:"apiTokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if len(cfg.APITokens) != 0 {
		t.Fatalf("GET /config handed a viewer %d api token(s)", len(cfg.APITokens))
	}
	if strings.Contains(w.Body.String(), `"ci"`) {
		t.Fatalf("GET /config body still names the token: %s", w.Body.String())
	}
}
