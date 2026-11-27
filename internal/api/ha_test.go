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

// The two preview routes write nothing, so a follower answers them like the
// leader; the same PUT without the dry-run flag stays refused.
func TestFollowerAllowsPreviews(t *testing.T) {
	h, changed := followerHandler(t, ha.RoleFollower)
	if w := do(t, h, "PUT", "/proxy-hosts/app?dryRun=1", validProxyHost); w.Code != http.StatusOK {
		t.Fatalf("PUT ?dryRun=1 on a follower = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := doCT(t, h, "POST", "/proxy-hosts/_parse", "application/yaml", hostYAML); w.Code != http.StatusOK {
		t.Fatalf("POST _parse on a follower = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := do(t, h, "PUT", "/proxy-hosts/app?dryRun=0", validProxyHost); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("PUT ?dryRun=0 on a follower = %d, want 503", w.Code)
	}
	if *changed != 0 {
		t.Fatalf("a preview reloaded the running config (%d times)", *changed)
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

// dryRunHandler is followerHandler plus the store and counters for the
// runtime side effects a write route can have, so a test can tell that a
// refused dry run changed nothing at all.
type dryRunEffects struct {
	reloads, logToggles, revokes int
}

func dryRunHandler(t *testing.T, role ha.Role) (http.Handler, *store.Store, *dryRunEffects) {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	fx := &dryRunEffects{}
	h := api.New(api.Deps{
		Store:               st,
		Role:                role,
		OnChange:            func() error { fx.reloads++; return nil },
		SetAccessLog:        func(bool) { fx.logToggles++ },
		RevokeAdminSessions: func(context.Context, string) (int, error) { fx.revokes++; return 0, nil },
	})
	return h, st, fx
}

// The write routes that do not implement a dry run, each with a body that
// would really change something.
// route is the pattern the refusal names.
var noDryRunWrites = []struct{ method, path, body, route string }{
	{"PUT", "/settings", `{"appName":"Changed","externalBaseURL":"https://gpm.example.com","adminAuth":{"localLoginEnabled":true}}`, "PUT /settings"},
	{"PUT", "/settings/saved-filters", `[{"name":"LAN","list":"hosts","query":"tag:lan"}]`, "PUT /settings/saved-filters"},
	{"PUT", "/logs", `{"enabled":true}`, "PUT /logs"},
	{"DELETE", "/proxy-hosts/app", "", "DELETE /proxy-hosts/{name}"},
	{"POST", "/sessions/revoke", "", "POST /sessions/revoke"},
}

// On the leader a write route that does not implement a dry run refuses a
// request asking for one (or carrying a dryRun value it cannot read) with 400
// and changes nothing; dryRun=0 is a normal write.
func TestLeaderRefusesDryRunWhereNotSupported(t *testing.T) {
	ctx := context.Background()
	for _, q := range []string{"?dryRun=1", "?dryRun=true", "?dryRun=yes"} {
		for _, tc := range noDryRunWrites {
			t.Run(tc.method+" "+tc.path+q, func(t *testing.T) {
				h, st, fx := dryRunHandler(t, ha.RoleLeader)
				if w := do(t, h, "PUT", "/proxy-hosts/app", validProxyHost); w.Code != http.StatusOK {
					t.Fatalf("seed: %d %s", w.Code, w.Body.String())
				}
				*fx = dryRunEffects{}
				head, _ := st.Head(ctx)
				w := do(t, h, tc.method, tc.path+q, tc.body)
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "dryRun is not supported for "+tc.route+": nothing was written") {
					t.Fatalf("= %d %s, want 400 naming the route", w.Code, w.Body.String())
				}
				if now, _ := st.Head(ctx); now != head || w.Header().Get("X-Config-Commit") != "" {
					t.Fatalf("a refused dry run committed (HEAD %s -> %s)", head, now)
				}
				if *fx != (dryRunEffects{}) {
					t.Fatalf("a refused dry run had side effects: %+v", *fx)
				}
				if cfg, _, err := st.Load(ctx); err != nil || len(cfg.ProxyHosts) != 1 {
					t.Fatalf("the seeded host must survive: %d hosts, %v", len(cfg.ProxyHosts), err)
				}
			})
		}
	}
	// dryRun=0 is an ordinary write.
	h, st, fx := dryRunHandler(t, ha.RoleLeader)
	head, _ := st.Head(ctx)
	if w := do(t, h, "PUT", "/settings?dryRun=0", noDryRunWrites[0].body); w.Code != http.StatusOK || w.Header().Get("X-Config-Commit") == "" {
		t.Fatalf("PUT /settings?dryRun=0 = %d (commit %q), want a normal write: %s", w.Code, w.Header().Get("X-Config-Commit"), w.Body.String())
	}
	if now, _ := st.Head(ctx); now == head || fx.reloads != 1 {
		t.Fatalf("dryRun=0: HEAD moved %v, reloads %d; want a commit and one reload", now != head, fx.reloads)
	}
}

// On a follower the same requests are refused by the read-only guard (503,
// naming the leader role): a dry run passes it only on a route that
// implements one. Nothing is committed, reloaded or toggled.
func TestFollowerRefusesDryRunWhereNotSupported(t *testing.T) {
	ctx := context.Background()
	h, st, fx := dryRunHandler(t, ha.RoleFollower)
	head, _ := st.Head(ctx)
	for _, tc := range noDryRunWrites {
		if tc.path == "/sessions/revoke" {
			continue // allowed on a follower; covered below
		}
		w := do(t, h, tc.method, tc.path+"?dryRun=1", tc.body)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "GPM_HA_ROLE=leader") {
			t.Fatalf("%s %s?dryRun=1 on a follower = %d %s, want 503 naming the leader role", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	// Ending sessions passes the follower guard, so the dry-run refusal answers.
	if w := do(t, h, "POST", "/sessions/revoke?dryRun=1", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("POST /sessions/revoke?dryRun=1 on a follower = %d %s, want 400", w.Code, w.Body.String())
	}
	if now, _ := st.Head(ctx); now != head {
		t.Fatalf("a follower committed (HEAD %s -> %s)", head, now)
	}
	if *fx != (dryRunEffects{}) {
		t.Fatalf("a follower had side effects: %+v", *fx)
	}
}
