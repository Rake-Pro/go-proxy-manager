package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// Two more syntactically valid bcrypt hashes, so a test can tell users apart.
const (
	bcryptFixtureB = "$2a$10$abcdefghijklmnopqrstuuJbG2b3e5hWv6rBqk1eYc0Zp7uQ9u1y6"
	bcryptFixtureC = "$2a$10$zyxwvutsrqponmlkjihgfuJbG2b3e5hWv6rBqk1eYc0Zp7uQ9u1y6"
)

// newHandlerStore is newHandler plus the store, so a test can read what was
// committed now that no API read returns a passwordHash.
func newHandlerStore(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	return api.New(api.Deps{Store: st, OnChange: func() error { return nil }}), st
}

func storedConfig(t *testing.T, st *store.Store) (model.Config, model.Settings) {
	t.Helper()
	cfg, s, err := st.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg, s
}

func storedMiddleware(t *testing.T, st *store.Store, name string) model.Middleware {
	t.Helper()
	cfg, _ := storedConfig(t, st)
	for _, m := range cfg.Middlewares {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("middleware %q not stored", name)
	return model.Middleware{}
}

func storedProxyHost(t *testing.T, st *store.Store, name string) model.ProxyHost {
	t.Helper()
	cfg, _ := storedConfig(t, st)
	for _, h := range cfg.ProxyHosts {
		if h.Name == name {
			return h
		}
	}
	t.Fatalf("proxy host %q not stored", name)
	return model.ProxyHost{}
}

func storedAccessList(t *testing.T, st *store.Store, name string) model.AccessList {
	t.Helper()
	cfg, _ := storedConfig(t, st)
	for _, a := range cfg.AccessLists {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("access list %q not stored", name)
	return model.AccessList{}
}

func mustPut(t *testing.T, h http.Handler, path, body string) {
	t.Helper()
	if w := do(t, h, "PUT", path, body); w.Code != http.StatusOK {
		t.Fatalf("PUT %s: %d %s", path, w.Code, w.Body.String())
	}
}

const (
	redactMiddleware = `{"name":"gate","type":"auth","auth":{"mode":"basic","basic":{"users":[
		{"username":"alice","passwordHash":"` + bcryptFixture + `"},
		{"username":"bob","passwordHash":"` + bcryptFixtureB + `"}]}}}`
	// Two locations with the same username and different hashes: the case a
	// positional match would get wrong once a location is removed.
	redactHost = `{"name":"app","domains":["app.example.com"],
		"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
		"auth":{"mode":"basic","basic":{"users":[{"username":"host-user","passwordHash":"` + bcryptFixture + `"}]}},
		"locations":[
			{"path":"/a","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"` + bcryptFixtureB + `"}]}}},
			{"path":"/b","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"` + bcryptFixtureC + `"}]}}}]}`
	redactList = `{"name":"legacy","defaultAction":"deny","rules":[{"action":"allow","cidr":"10.0.0.0/8"}],
		"basicAuth":[{"username":"admin","passwordHash":"` + bcryptFixture + `"}]}`
	redactSettings = `{"adminAuth":{"localLoginEnabled":true},"ingressDiscovery":{"template":{"locations":[{"path":"/admin",
		"auth":{"mode":"basic","basic":{"users":[{"username":"ops","passwordHash":"` + bcryptFixtureC + `"}]}}}]}}}`
)

func seedRedactFixtures(t *testing.T, h http.Handler) {
	t.Helper()
	mustPut(t, h, "/middlewares/gate", redactMiddleware)
	mustPut(t, h, "/proxy-hosts/app", redactHost)
	mustPut(t, h, "/access-lists/legacy", redactList)
	mustPut(t, h, "/settings", redactSettings)
}

// No API read returns a stored basic-auth hash: not the per-object reads, the
// lists, the whole-config dump, settings, the reply to a write, a dry-run
// preview or a history diff. Each reply carries the placeholder instead.
func TestReadsRedactBasicAuthHashes(t *testing.T) {
	h, st := newHandlerStore(t)
	seedRedactFixtures(t, h)
	head, err := st.Head(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, method, path, body string
	}{
		{"middleware get", "GET", "/middlewares/gate", ""},
		{"middleware list", "GET", "/middlewares", ""},
		{"proxy host get", "GET", "/proxy-hosts/app", ""},
		{"proxy host list", "GET", "/proxy-hosts", ""},
		{"access list get", "GET", "/access-lists/legacy", ""},
		{"access list list", "GET", "/access-lists", ""},
		{"whole config", "GET", "/config", ""},
		{"settings", "GET", "/settings", ""},
		{"write reply", "PUT", "/middlewares/gate", redactMiddleware},
		{"dry run preview", "PUT", "/proxy-hosts/app?dryRun=1", redactHost},
		{"history diff", "GET", "/history/diff?to=" + head, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, h, tc.method, tc.path, tc.body)
			if w.Code != http.StatusOK {
				t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, hash := range []string{bcryptFixture, bcryptFixtureB, bcryptFixtureC} {
				if strings.Contains(body, hash) {
					t.Fatalf("reply carries a stored passwordHash: %s", body)
				}
			}
			if !strings.Contains(body, `***`) {
				t.Fatalf("reply carries no placeholder where the hash was: %s", body)
			}
		})
	}
}

// A write that sends the read placeholder back keeps the stored hash of that
// user; a placeholder that names no stored user is refused, and the store is
// left as it was.
func TestPlaceholderWriteKeepsStoredHash(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    string
		wantErr string
		check   func(t *testing.T, st *store.Store)
	}{
		{
			name: "middleware edit without retyping passwords",
			path: "/middlewares/gate",
			body: `{"name":"gate","type":"auth","auth":{"mode":"basic","basic":{"realm":"Edited","users":[
				{"username":"alice","passwordHash":"***"},{"username":"bob","passwordHash":"***"}]}}}`,
			check: func(t *testing.T, st *store.Store) {
				b := storedMiddleware(t, st, "gate").Auth.Basic
				if b.Realm != "Edited" || b.Users[0].PasswordHash != bcryptFixture || b.Users[1].PasswordHash != bcryptFixtureB {
					t.Fatalf("stored = %+v", b)
				}
			},
		},
		{
			name: "middleware users reordered",
			path: "/middlewares/gate",
			body: `{"name":"gate","type":"auth","auth":{"mode":"basic","basic":{"users":[
				{"username":"bob","passwordHash":"***"},{"username":"alice","passwordHash":"***"}]}}}`,
			check: func(t *testing.T, st *store.Store) {
				u := storedMiddleware(t, st, "gate").Auth.Basic.Users
				if u[0].Username != "bob" || u[0].PasswordHash != bcryptFixtureB || u[1].PasswordHash != bcryptFixture {
					t.Fatalf("stored = %+v", u)
				}
			},
		},
		{
			name: "middleware add a user and change one password",
			path: "/middlewares/gate",
			body: `{"name":"gate","type":"auth","auth":{"mode":"basic","basic":{"users":[
				{"username":"alice","passwordHash":"***"},
				{"username":"bob","passwordHash":"***","password":"new secret"},
				{"username":"carol","password":"carol secret"}]}}}`,
			check: func(t *testing.T, st *store.Store) {
				u := storedMiddleware(t, st, "gate").Auth.Basic.Users
				if len(u) != 3 || u[0].PasswordHash != bcryptFixture {
					t.Fatalf("stored = %+v", u)
				}
				for _, x := range u[1:] {
					if len(x.PasswordHash) != 60 || x.PasswordHash == bcryptFixtureB || x.PasswordHash == "***" {
						t.Fatalf("user %q passwordHash = %q, want a fresh bcrypt hash", x.Username, x.PasswordHash)
					}
				}
			},
		},
		{
			name: "middleware rename with the placeholder is refused",
			path: "/middlewares/gate",
			body: `{"name":"gate","type":"auth","auth":{"mode":"basic","basic":{"users":[
				{"username":"alice2","passwordHash":"***"},{"username":"bob","passwordHash":"***"}]}}}`,
			wantErr: `user "alice2": passwordHash is the redacted placeholder`,
		},
		{
			name: "new object with the placeholder is refused",
			path: "/middlewares/gate-copy",
			body: `{"name":"gate-copy","type":"auth","auth":{"mode":"basic","basic":{"users":[
				{"username":"alice","passwordHash":"***"}]}}}`,
			wantErr: `user "alice": passwordHash is the redacted placeholder`,
		},
		{
			name: "proxy host location removed",
			path: "/proxy-hosts/app",
			body: `{"name":"app","domains":["app.example.com"],
				"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
				"auth":{"mode":"basic","basic":{"users":[{"username":"host-user","passwordHash":"***"}]}},
				"locations":[
					{"path":"/b","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}}]}`,
			check: func(t *testing.T, st *store.Store) {
				h := storedProxyHost(t, st, "app")
				if h.Auth.Basic.Users[0].PasswordHash != bcryptFixture {
					t.Fatalf("host user hash = %q", h.Auth.Basic.Users[0].PasswordHash)
				}
				if len(h.Locations) != 1 || h.Locations[0].Auth.Basic.Users[0].PasswordHash != bcryptFixtureC {
					t.Fatalf("location /b took another location's hash: %+v", h.Locations)
				}
			},
		},
		{
			name: "proxy host location renamed, user unique in the object",
			path: "/proxy-hosts/app",
			body: `{"name":"app","domains":["app.example.com"],
				"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
				"locations":[
					{"path":"/admin","auth":{"mode":"basic","basic":{"users":[{"username":"host-user","passwordHash":"***"}]}}}]}`,
			check: func(t *testing.T, st *store.Store) {
				h := storedProxyHost(t, st, "app")
				if h.Locations[0].Auth.Basic.Users[0].PasswordHash != bcryptFixture {
					t.Fatalf("location hash = %q", h.Locations[0].Auth.Basic.Users[0].PasswordHash)
				}
			},
		},
		{
			name: "proxy host location renamed takes the hash of the location it replaces",
			path: "/proxy-hosts/app",
			body: `{"name":"app","domains":["app.example.com"],
				"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
				"locations":[
					{"path":"/b","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}},
					{"path":"/c","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}}]}`,
			check: func(t *testing.T, st *store.Store) {
				h := storedProxyHost(t, st, "app")
				if h.Locations[0].Auth.Basic.Users[0].PasswordHash != bcryptFixtureC || h.Locations[1].Auth.Basic.Users[0].PasswordHash != bcryptFixtureB {
					t.Fatalf("locations = %+v / %+v", h.Locations[0].Auth.Basic.Users, h.Locations[1].Auth.Basic.Users)
				}
			},
		},
		{
			name: "proxy host new location, username stored under two hashes",
			path: "/proxy-hosts/app",
			body: `{"name":"app","domains":["app.example.com"],
				"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
				"auth":{"mode":"basic","basic":{"users":[{"username":"host-user","passwordHash":"***"}]}},
				"locations":[
					{"path":"/a","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}},
					{"path":"/b","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}},
					{"path":"/c","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}}]}`,
			wantErr: `locations["/c"].auth.basic.users: user "admin": passwordHash is the redacted placeholder "***", but that username is stored with different hashes in locations["/a"].auth.basic.users, locations["/b"].auth.basic.users`,
		},
		{
			// The writer may not know the host user's password; a placeholder
			// must not carry that hash into a list that already exists.
			name: "proxy host existing location gains the host user",
			path: "/proxy-hosts/app",
			body: `{"name":"app","domains":["app.example.com"],
				"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
				"auth":{"mode":"basic","basic":{"users":[{"username":"host-user","passwordHash":"***"}]}},
				"locations":[
					{"path":"/a","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"},{"username":"host-user","passwordHash":"***"}]}}},
					{"path":"/b","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}}]}`,
			wantErr: `locations["/a"].auth.basic.users: user "host-user": passwordHash is the redacted placeholder "***", but this user list has no stored user of that name`,
		},
		{
			name: "proxy host existing host list gains a location user",
			path: "/proxy-hosts/app",
			body: `{"name":"app","domains":["app.example.com"],
				"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
				"auth":{"mode":"basic","basic":{"users":[{"username":"host-user","passwordHash":"***"},{"username":"admin","passwordHash":"***"}]}},
				"locations":[
					{"path":"/b","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}}]}`,
			wantErr: `auth.basic.users: user "admin": passwordHash is the redacted placeholder "***", but this user list has no stored user of that name`,
		},
		{
			name: "legacy access list kept",
			path: "/access-lists/legacy",
			body: `{"name":"legacy","defaultAction":"deny","rules":[{"action":"allow","cidr":"10.0.0.0/8"}],
				"basicAuth":[{"username":"admin","passwordHash":"***"}]}`,
			check: func(t *testing.T, st *store.Store) {
				//lint:ignore SA1019 compat read of deprecated AccessList.BasicAuth
				if got := storedAccessList(t, st, "legacy").BasicAuth[0].PasswordHash; got != bcryptFixture {
					t.Fatalf("stored = %q", got)
				}
			},
		},
		{
			// The access-list tier only checks the hash is non-empty, so without
			// the refusal the placeholder itself would be committed.
			name: "legacy access list rename refused",
			path: "/access-lists/legacy",
			body: `{"name":"legacy","defaultAction":"deny","rules":[{"action":"allow","cidr":"10.0.0.0/8"}],
				"basicAuth":[{"username":"root","passwordHash":"***"}]}`,
			wantErr: `basicAuth: user "root"`,
		},
		{
			name: "settings template round trip",
			path: "/settings",
			body: `{"adminAuth":{"localLoginEnabled":true},"ingressDiscovery":{"template":{"locations":[{"path":"/admin",
				"auth":{"mode":"basic","basic":{"realm":"Ops","users":[{"username":"ops","passwordHash":"***"}]}}}]}}}`,
			check: func(t *testing.T, st *store.Store) {
				_, s := storedConfig(t, st)
				b := s.IngressDiscovery.Template.Locations[0].Auth.Basic
				if b.Realm != "Ops" || b.Users[0].PasswordHash != bcryptFixtureC {
					t.Fatalf("stored = %+v", b)
				}
			},
		},
		{
			name: "settings template new user refused",
			path: "/settings",
			body: `{"adminAuth":{"localLoginEnabled":true},"ingressDiscovery":{"template":{"locations":[{"path":"/admin",
				"auth":{"mode":"basic","basic":{"users":[{"username":"ops2","passwordHash":"***"}]}}}]}}}`,
			wantErr: `user "ops2"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, st := newHandlerStore(t)
			seedRedactFixtures(t, h)
			before, err := st.Head(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			w := do(t, h, "PUT", tc.path, tc.body)
			if tc.wantErr != "" {
				var e struct {
					Error string `json:"error"`
				}
				_ = json.Unmarshal(w.Body.Bytes(), &e)
				if w.Code != http.StatusBadRequest || !strings.Contains(e.Error, tc.wantErr) {
					t.Fatalf("PUT = %d %s, want 400 with %q", w.Code, w.Body.String(), tc.wantErr)
				}
				if after, _ := st.Head(context.Background()); after != before {
					t.Fatal("a refused write committed")
				}
				return
			}
			if w.Code != http.StatusOK {
				t.Fatalf("PUT = %d %s", w.Code, w.Body.String())
			}
			tc.check(t, st)
		})
	}
}

// The full UI cycle on an object: read it, send the read body back unchanged,
// and the stored hashes are exactly what they were.
func TestGetPutRoundTripKeepsHashes(t *testing.T) {
	h, st := newHandlerStore(t)
	seedRedactFixtures(t, h)
	for _, path := range []string{"/middlewares/gate", "/proxy-hosts/app", "/access-lists/legacy", "/settings"} {
		w := do(t, h, "GET", path, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: %d", path, w.Code)
		}
		mustPut(t, h, path, w.Body.String())
	}
	if got := storedMiddleware(t, st, "gate").Auth.Basic.Users[1].PasswordHash; got != bcryptFixtureB {
		t.Fatalf("middleware hash = %q", got)
	}
	host := storedProxyHost(t, st, "app")
	if host.Locations[0].Auth.Basic.Users[0].PasswordHash != bcryptFixtureB || host.Locations[1].Auth.Basic.Users[0].PasswordHash != bcryptFixtureC {
		t.Fatalf("host hashes = %+v", host.Locations)
	}
	_, s := storedConfig(t, st)
	if got := s.IngressDiscovery.Template.Locations[0].Auth.Basic.Users[0].PasswordHash; got != bcryptFixtureC {
		t.Fatalf("settings hash = %q", got)
	}
}

// The YAML import route echoes the caller's own document, so its hashes are
// returned as sent: the wizard creates the object from that reply.
func TestParseKeepsCallerHashes(t *testing.T) {
	h, _ := newHandlerStore(t)
	body := "name: gate\ntype: auth\nauth:\n  mode: basic\n  basic:\n    users:\n      - username: alice\n        passwordHash: " + bcryptFixture + "\n"
	w := doCT(t, h, "POST", "/middlewares/_parse", "application/yaml", body)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), bcryptFixture) {
		t.Fatalf("parse = %d %s, want the caller's hash back", w.Code, w.Body.String())
	}
	var resp struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || !resp.Valid {
		t.Fatalf("parse reply = %s", w.Body.String())
	}
}

// A member that is merely called passwordHash (a label, a header name) is not
// a basic-auth hash: it reads back as stored, and an unchanged GET-then-PUT
// keeps it.
func TestRedactionLeavesOtherPasswordHashMembers(t *testing.T) {
	h, st := newHandlerStore(t)
	mustPut(t, h, "/proxy-hosts/lab", `{"name":"lab","labels":{"passwordHash":"keep-me"},"domains":["lab.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080}}`)
	mustPut(t, h, "/middlewares/hdr", `{"name":"hdr","type":"headers","headers":{"setRequest":{"passwordHash":"keep-too"}}}`)
	for _, path := range []string{"/proxy-hosts/lab", "/middlewares/hdr"} {
		w := do(t, h, "GET", path, "")
		if strings.Contains(w.Body.String(), "***") {
			t.Fatalf("GET %s redacted a non-hash member: %s", path, w.Body.String())
		}
		mustPut(t, h, path, w.Body.String())
	}
	if got := storedProxyHost(t, st, "lab").Labels["passwordHash"]; got != "keep-me" {
		t.Fatalf("label after round trip = %q", got)
	}
	if got := storedMiddleware(t, st, "hdr").Headers.SetRequest["passwordHash"]; got != "keep-too" {
		t.Fatalf("header after round trip = %q", got)
	}
}

// Reads redact a copy: after several reads the stored objects still hold the
// real hashes.
func TestReadsDoNotAlterStoredHashes(t *testing.T) {
	h, st := newHandlerStore(t)
	seedRedactFixtures(t, h)
	for i := 0; i < 2; i++ {
		for _, path := range []string{"/middlewares/gate", "/middlewares", "/proxy-hosts", "/config", "/settings"} {
			if w := do(t, h, "GET", path, ""); w.Code != http.StatusOK || strings.Contains(w.Body.String(), bcryptFixture) {
				t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
			}
		}
	}
	if got := storedMiddleware(t, st, "gate").Auth.Basic.Users[0].PasswordHash; got != bcryptFixture {
		t.Fatalf("stored hash after reads = %q", got)
	}
	_, s := storedConfig(t, st)
	if got := s.IngressDiscovery.Template.Locations[0].Auth.Basic.Users[0].PasswordHash; got != bcryptFixtureC {
		t.Fatalf("stored settings hash after reads = %q", got)
	}
}

// A host with two locations on the same path, each with the same username
// under a different hash (validation allows both), round-trips unchanged.
func TestDuplicateLocationPathsRoundTrip(t *testing.T) {
	h, st := newHandlerStore(t)
	mustPut(t, h, "/proxy-hosts/dup", `{"name":"dup","domains":["dup.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
		"locations":[
			{"path":"/x","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"`+bcryptFixture+`"}]}}},
			{"path":"/x","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"`+bcryptFixtureB+`"}]}}}]}`)
	mustPut(t, h, "/proxy-hosts/dup", do(t, h, "GET", "/proxy-hosts/dup", "").Body.String())
	l := storedProxyHost(t, st, "dup").Locations
	if l[0].Auth.Basic.Users[0].PasswordHash != bcryptFixture || l[1].Auth.Basic.Users[0].PasswordHash != bcryptFixtureB {
		t.Fatalf("hashes after round trip = %q / %q", l[0].Auth.Basic.Users[0].PasswordHash, l[1].Auth.Basic.Users[0].PasswordHash)
	}
}

// Discovery profiles in settings: a renamed profile keeps its users' hashes
// even when another profile holds the same username, and a placeholder never
// moves a hash into another list that already exists (template onto profile,
// Ingress onto Docker discovery).
func TestSettingsProfilesPlaceholders(t *testing.T) {
	loc := func(user, hash string) string {
		return `{"locations":[{"path":"/x","auth":{"mode":"basic","basic":{"users":[{"username":"` + user + `","passwordHash":"` + hash + `"}]}}}]}`
	}
	seed := `{"adminAuth":{"localLoginEnabled":true},
		"ingressDiscovery":{"template":` + loc("tmpl", bcryptFixtureC) + `,"profiles":{"a":` + loc("ops", bcryptFixture) + `,"b":` + loc("ops", bcryptFixtureB) + `}},
		"dockerDiscovery":{"template":` + loc("dk", bcryptFixtureC) + `,"profiles":{"a":` + loc("ops", bcryptFixtureB) + `}}}`
	tests := []struct {
		name    string
		edit    func(s map[string]any)
		wantErr string
		check   func(t *testing.T, s model.Settings)
	}{
		{
			name: "ingress profile renamed",
			edit: func(s map[string]any) {
				pr := s["ingressDiscovery"].(map[string]any)["profiles"].(map[string]any)
				pr["c"] = pr["a"]
				delete(pr, "a")
			},
			check: func(t *testing.T, s model.Settings) {
				if got := s.IngressDiscovery.Profiles["c"].Locations[0].Auth.Basic.Users[0].PasswordHash; got != bcryptFixture {
					t.Fatalf("renamed profile hash = %q", got)
				}
				if got := s.IngressDiscovery.Profiles["b"].Locations[0].Auth.Basic.Users[0].PasswordHash; got != bcryptFixtureB {
					t.Fatalf("other profile hash = %q", got)
				}
			},
		},
		{
			name: "docker profile renamed",
			edit: func(s map[string]any) {
				pr := s["dockerDiscovery"].(map[string]any)["profiles"].(map[string]any)
				pr["z"] = pr["a"]
				delete(pr, "a")
			},
			check: func(t *testing.T, s model.Settings) {
				if got := s.DockerDiscovery.Profiles["z"].Locations[0].Auth.Basic.Users[0].PasswordHash; got != bcryptFixtureB {
					t.Fatalf("renamed profile hash = %q", got)
				}
			},
		},
		{
			name: "template user onto an existing profile list",
			edit: func(s map[string]any) {
				addUser(s["ingressDiscovery"].(map[string]any)["profiles"].(map[string]any)["a"], "tmpl")
			},
			wantErr: `ingressDiscovery.profiles[a].locations["/x"].auth.basic.users: user "tmpl"`,
		},
		{
			name: "ingress user onto the existing docker template list",
			edit: func(s map[string]any) {
				addUser(s["dockerDiscovery"].(map[string]any)["template"], "tmpl")
			},
			wantErr: `dockerDiscovery.template.locations["/x"].auth.basic.users: user "tmpl"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, st := newHandlerStore(t)
			mustPut(t, h, "/settings", seed)
			var s map[string]any
			if err := json.Unmarshal(do(t, h, "GET", "/settings", "").Body.Bytes(), &s); err != nil {
				t.Fatal(err)
			}
			tc.edit(s)
			b, _ := json.Marshal(s)
			w := do(t, h, "PUT", "/settings", string(b))
			if tc.wantErr != "" {
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), strings.ReplaceAll(tc.wantErr, `"`, `\"`)) {
					t.Fatalf("PUT = %d %s, want 400 with %q", w.Code, w.Body.String(), tc.wantErr)
				}
				return
			}
			if w.Code != http.StatusOK {
				t.Fatalf("PUT = %d %s", w.Code, w.Body.String())
			}
			_, stored := storedConfig(t, st)
			tc.check(t, stored)
		})
	}
}

// addUser appends a placeholder user to the first location of a template or
// profile decoded from GET /settings.
func addUser(tmpl any, name string) {
	l := tmpl.(map[string]any)["locations"].([]any)[0].(map[string]any)
	basic := l["auth"].(map[string]any)["basic"].(map[string]any)
	basic["users"] = append(basic["users"].([]any), map[string]any{"username": name, "passwordHash": "***"})
}

// A hand-edited object file that is not in the store's own YAML form still
// shows no part of its hash in a history diff.
func TestHistoryDiffRedactsHandWrittenForms(t *testing.T) {
	head := "name: hand\ndomains: [hand.example.com]\nupstream: {scheme: http, host: 10.0.0.5, port: 8080}\nauth:\n  mode: basic\n  basic:\n    users:\n      - username: a\n"
	forms := map[string]string{
		"json": `{"name":"hand","domains":["hand.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},` + "\n" +
			` "auth":{"mode":"basic","basic":{"users":[{"username":"a","passwordHash":"` + bcryptFixture + `"}]}}}` + "\n",
		"quoted key":   head + `        "passwordHash": "` + bcryptFixture + `"` + "\n",
		"anchor":       head + `        passwordHash: &h "` + bcryptFixture + `"` + "\n",
		"block scalar": head + "        passwordHash: >-\n          " + bcryptFixture + "\n",
	}
	for name, doc := range forms {
		t.Run(name, func(t *testing.T) {
			h, st := newHandlerStore(t)
			if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "hand.yaml"), []byte(doc), 0o640); err != nil {
				t.Fatal(err)
			}
			// A write commits the whole tree, the hand-edited file with it.
			mustPut(t, h, "/proxy-hosts/other", `{"name":"other","domains":["other.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080}}`)
			sha, err := st.Head(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			w := do(t, h, "GET", "/history/diff?to="+sha, "")
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hand.yaml") {
				t.Fatalf("diff = %d %s", w.Code, w.Body.String())
			}
			// The hash and its distinctive tail, so a partial leak also fails.
			for _, part := range []string{bcryptFixture, bcryptFixture[29:]} {
				if strings.Contains(w.Body.String(), part) {
					t.Fatalf("diff carries the hash: %s", w.Body.String())
				}
			}
		})
	}
}

// When a save removes or renames several lists at one level, which stored
// list became which written one cannot be told, so none is paired: the write
// either resolves the username to its one stored hash or is refused, and never
// takes another list's hash.
func TestAmbiguousRenamesNeverTakeAnotherListsHash(t *testing.T) {
	const ambiguous = "but that username is stored with different hashes in"
	loc := func(path, hash string) string {
		return `{"path":"` + path + `","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"` + hash + `"}]}}}`
	}
	host := func(locs ...string) string {
		return `{"name":"app","domains":["app.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},"locations":[` + strings.Join(locs, ",") + `]}`
	}
	prof := func(hash string) string {
		return `{"locations":[{"path":"/x","auth":{"mode":"basic","basic":{"users":[{"username":"ops","passwordHash":"` + hash + `"}]}}}]}`
	}
	tests := []struct {
		name, seedPath, seed, path, body string
	}{
		{
			name:     "location deleted and another renamed in one save",
			seedPath: "/proxy-hosts/app", seed: host(loc("/a", bcryptFixtureB), loc("/b", bcryptFixtureC)),
			path: "/proxy-hosts/app", body: host(loc("/c", "***")),
		},
		{
			name:     "two profiles renamed in one save",
			seedPath: "/settings", seed: `{"adminAuth":{"localLoginEnabled":true},"ingressDiscovery":{"profiles":{"a":` + prof(bcryptFixture) + `,"b":` + prof(bcryptFixtureB) + `}}}`,
			path: "/settings", body: `{"adminAuth":{"localLoginEnabled":true},"ingressDiscovery":{"profiles":{"zz":` + strings.ReplaceAll(prof(""), `""`, `"***"`) + `,"aa":` + strings.ReplaceAll(prof(""), `""`, `"***"`) + `}}}`,
		},
		{
			name:     "one of two locations on the same path deleted",
			seedPath: "/proxy-hosts/app", seed: host(loc("/x", bcryptFixture), loc("/x", bcryptFixtureB)),
			path: "/proxy-hosts/app", body: host(loc("/x", "***")),
		},
		{
			// /a is stored twice and written once, so its leftovers count too:
			// /b is not "the one rename" of /c.
			name:     "a shared path shrinks while another location is renamed",
			seedPath: "/proxy-hosts/app", seed: host(userLoc("/a", "bob", bcryptFixture), userLoc("/a", "alice", bcryptFixtureB), userLoc("/c", "alice", bcryptFixtureC)),
			path: "/proxy-hosts/app", body: host(userLoc("/a", "bob", "***"), userLoc("/b", "alice", "***")),
		},
		{
			name:     "a shared path grows while another location is renamed",
			seedPath: "/proxy-hosts/app", seed: hostWithAuth("alice", bcryptFixture, userLoc("/a", "bob", bcryptFixtureB), userLoc("/c", "alice", bcryptFixtureC)),
			path: "/proxy-hosts/app", body: hostWithAuth("alice", "***", userLoc("/a", "bob", "***"), userLoc("/a", "dave", bcryptFixture), userLoc("/b", "alice", "***")),
		},
		{
			name:     "a renamed profile whose shared path shrinks while another location is renamed",
			seedPath: "/settings", seed: `{"adminAuth":{"localLoginEnabled":true},"ingressDiscovery":{"profiles":{"a":{"locations":[` + userLoc("/p", "bob", bcryptFixture) + `,` + userLoc("/p", "ops", bcryptFixtureB) + `,` + userLoc("/q", "ops", bcryptFixtureC) + `]}}}}`,
			path: "/settings", body: `{"adminAuth":{"localLoginEnabled":true},"ingressDiscovery":{"profiles":{"c":{"locations":[` + userLoc("/p", "bob", "***") + `,` + userLoc("/r", "ops", "***") + `]}}}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, st := newHandlerStore(t)
			mustPut(t, h, tc.seedPath, tc.seed)
			before, _ := st.Head(context.Background())
			w := do(t, h, "PUT", tc.path, tc.body)
			var e struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &e)
			if w.Code != http.StatusBadRequest || !strings.Contains(e.Error, ambiguous) {
				t.Fatalf("PUT = %d %s, want 400 %q", w.Code, w.Body.String(), ambiguous)
			}
			if after, _ := st.Head(context.Background()); after != before {
				t.Fatal("the refused write committed")
			}
		})
	}
}

// Swapping two location paths keeps each path's hash; a single rename at a
// level still pairs with the one stored list that went away.
func TestUnambiguousLocationMovesKeepTheirHash(t *testing.T) {
	h, st := newHandlerStore(t)
	mustPut(t, h, "/proxy-hosts/app", redactHost)
	mustPut(t, h, "/proxy-hosts/app", `{"name":"app","domains":["app.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
		"auth":{"mode":"basic","basic":{"users":[{"username":"host-user","passwordHash":"***"}]}},
		"locations":[
			{"path":"/b","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}},
			{"path":"/a","auth":{"mode":"basic","basic":{"users":[{"username":"admin","passwordHash":"***"}]}}}]}`)
	l := storedProxyHost(t, st, "app").Locations
	if l[0].Path != "/b" || l[0].Auth.Basic.Users[0].PasswordHash != bcryptFixtureC || l[1].Auth.Basic.Users[0].PasswordHash != bcryptFixtureB {
		t.Fatalf("after swap: %+v", l)
	}
}

func userLoc(path, user, hash string) string {
	return `{"path":"` + path + `","auth":{"mode":"basic","basic":{"users":[{"username":"` + user + `","passwordHash":"` + hash + `"}]}}}`
}

func hostWithAuth(user, hash string, locs ...string) string {
	return `{"name":"app","domains":["app.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},` +
		`"auth":{"mode":"basic","basic":{"users":[{"username":"` + user + `","passwordHash":"` + hash + `"}]}},"locations":[` + strings.Join(locs, ",") + `]}`
}

// The deprecated access-list basicAuth accepts a username twice. An unchanged
// GET-then-PUT keeps each occurrence's hash; a write that lists the name a
// different number of times is refused.
func TestDuplicateUsernamesRoundTrip(t *testing.T) {
	h, st := newHandlerStore(t)
	mustPut(t, h, "/access-lists/legacy", `{"name":"legacy","defaultAction":"deny","rules":[{"action":"allow","cidr":"10.0.0.0/8"}],
		"basicAuth":[{"username":"admin","passwordHash":"`+bcryptFixture+`"},{"username":"admin","passwordHash":"`+bcryptFixtureB+`"}]}`)
	mustPut(t, h, "/access-lists/legacy", do(t, h, "GET", "/access-lists/legacy", "").Body.String())
	//lint:ignore SA1019 compat read of deprecated AccessList.BasicAuth
	if u := storedAccessList(t, st, "legacy").BasicAuth; len(u) != 2 || u[0].PasswordHash != bcryptFixture || u[1].PasswordHash != bcryptFixtureB {
		t.Fatalf("after round trip: %+v", u)
	}
	w := do(t, h, "PUT", "/access-lists/legacy", `{"name":"legacy","defaultAction":"deny","rules":[{"action":"allow","cidr":"10.0.0.0/8"}],
		"basicAuth":[{"username":"admin","passwordHash":"***"}]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("one of two duplicates sent back: %d %s, want 400", w.Code, w.Body.String())
	}
}

// A hunk of a settings diff that starts inside an inline error page stays
// readable; only lines that could continue a hash are blanked at a hunk start.
func TestHistoryDiffHunkStartStaysReadable(t *testing.T) {
	h, st := newHandlerStore(t)
	page := func(msg string) string {
		var l []string
		for i := 0; i < 20; i++ {
			l = append(l, "<p>line "+string(rune('a'+i))+"</p>")
		}
		l[12] = "<p>" + msg + "</p>"
		return strings.Join(l, "\n")
	}
	put := func(msg string) {
		b, _ := json.Marshal(map[string]any{"adminAuth": map[string]any{"localLoginEnabled": true},
			"errorPages": map[string]any{"inline": map[string]string{"502": page(msg)}}})
		mustPut(t, h, "/settings", string(b))
	}
	put("We are down for maintenance")
	put("Back at 10:00 UTC")
	sha, _ := st.Head(context.Background())
	var d struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(do(t, h, "GET", "/history/diff?to="+sha, "").Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	// The case under test: the hunk starts inside the page's block scalar.
	_, hunk, _ := strings.Cut(d.Patch, "\n@@ ")
	if _, first, _ := strings.Cut(hunk, "\n"); !strings.HasPrefix(strings.TrimSpace(first), "<p>line j</p>") {
		t.Fatalf("the hunk does not start inside the page:\n%s", d.Patch)
	}
	for _, want := range []string{"<p>We are down for maintenance</p>", "<p>Back at 10:00 UTC</p>", "<p>line j</p>"} {
		if !strings.Contains(d.Patch, want) {
			t.Fatalf("%q was redacted:\n%s", want, d.Patch)
		}
	}
	if strings.Contains(d.Patch, "***") {
		t.Fatalf("a settings diff with no hash shows the placeholder:\n%s", d.Patch)
	}
}
