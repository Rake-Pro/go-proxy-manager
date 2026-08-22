package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/ha"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

func followerHandler(t *testing.T, role ha.Role) (http.Handler, *int) {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	changed := 0
	h := api.New(api.Deps{Store: st, Role: role, OnChange: func() error { changed++; return nil }})
	return h, &changed
}

// A follower refuses every config write, with an error naming the leader role,
// and never touches the store or the running config.
func TestFollowerRefusesWrites(t *testing.T) {
	h, changed := followerHandler(t, ha.RoleFollower)

	for _, tc := range []struct{ method, path, body string }{
		{"PUT", "/proxy-hosts/app", validProxyHost},
		{"DELETE", "/proxy-hosts/app", ""},
		{"PUT", "/settings", `{"externalBaseURL":"https://gpm.example.com"}`},
		{"POST", "/revert", `{"hash":"abcdef1"}`},
		{"POST", "/restore", ""},
		{"POST", "/sso/revoke", ""},
		{"POST", "/dns-sync/reconcile", ""},
	} {
		w := do(t, h, tc.method, tc.path, tc.body)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s = %d, want 503: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s: response is not JSON: %v", tc.method, tc.path, err)
		}
		if !strings.Contains(body.Error, "GPM_HA_ROLE=leader") {
			t.Fatalf("%s %s error does not name the leader role: %q", tc.method, tc.path, body.Error)
		}
	}
	if *changed != 0 {
		t.Fatalf("a refused write still reloaded the running config (%d times)", *changed)
	}
}

// Reads keep working on a follower - that is the whole point of a warm standby.
func TestFollowerAllowsReads(t *testing.T) {
	h, _ := followerHandler(t, ha.RoleFollower)
	for _, path := range []string{"/proxy-hosts", "/config", "/history", "/settings", "/capabilities"} {
		if w := do(t, h, "GET", path, ""); w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200: %s", path, w.Code, w.Body.String())
		}
	}
}

// The leader (and the unset zero value) behaves exactly as before.
func TestLeaderAcceptsWrites(t *testing.T) {
	for _, role := range []ha.Role{"", ha.RoleLeader} {
		h, changed := followerHandler(t, role)
		w := do(t, h, "PUT", "/proxy-hosts/app", validProxyHost)
		if w.Code != http.StatusOK {
			t.Fatalf("role %q: PUT = %d, want 200: %s", role, w.Code, w.Body.String())
		}
		if *changed != 1 {
			t.Fatalf("role %q: reloads = %d, want 1", role, *changed)
		}
	}
}

// The SPA greys out write controls from this probe, so the role has to be on it.
func TestCapabilitiesReportsRole(t *testing.T) {
	for _, tc := range []struct {
		role     ha.Role
		wantRole string
		wantRO   bool
	}{
		{role: "", wantRole: "leader"},
		{role: ha.RoleLeader, wantRole: "leader"},
		{role: ha.RoleFollower, wantRole: "follower", wantRO: true},
	} {
		h, _ := followerHandler(t, tc.role)
		w := do(t, h, "GET", "/capabilities", "")
		if w.Code != http.StatusOK {
			t.Fatalf("role %q: GET /capabilities = %d", tc.role, w.Code)
		}
		var got struct {
			HA struct {
				Role     string `json:"role"`
				ReadOnly bool   `json:"readOnly"`
			} `json:"ha"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.HA.Role != tc.wantRole || got.HA.ReadOnly != tc.wantRO {
			t.Fatalf("role %q: capabilities.ha = %+v, want role=%s readOnly=%v", tc.role, got.HA, tc.wantRole, tc.wantRO)
		}
	}
}
