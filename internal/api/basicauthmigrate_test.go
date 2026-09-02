package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// bcryptFixture is a syntactically valid bcrypt hash. Nothing here verifies a
// password, so no real hashing is needed.
const bcryptFixture = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// migrationPlan mirrors the endpoint's JSON payload for assertions.
type migrationPlan struct {
	AccessList string   `json:"accessList"`
	Middleware string   `json:"middleware"`
	Users      []string `json:"users"`
	AllowFrom  []string `json:"allowFrom"`
	SatisfyAny bool     `json:"satisfyAny"`
	AttachTo   []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"attachTo"`
	DetachAccessList bool     `json:"detachAccessList"`
	Warnings         []string `json:"warnings"`
	Plan             bool     `json:"plan"`
	Commit           string   `json:"commit"`
}

// seedLegacyBasicAuth writes an access list carrying the deprecated basicAuth
// users and a proxy host that references it host-wide and on a location.
func seedLegacyBasicAuth(t *testing.T, h http.Handler, listName string, satisfyAny bool) {
	t.Helper()
	list := map[string]any{
		"name":       listName,
		"satisfyAny": satisfyAny,
		"basicAuth":  []map[string]string{{"username": "admin", "passwordHash": bcryptFixture}},
		"rules": []map[string]any{
			{"action": "allow", "cidr": "10.0.0.0/8"},
			{"action": "deny", "cidr": "203.0.113.0/24"},
		},
		"defaultAction": "deny",
	}
	body, _ := json.Marshal(list)
	if w := do(t, h, "PUT", "/access-lists/"+listName, string(body)); w.Code != http.StatusOK {
		t.Fatalf("seed access list: %d %s", w.Code, w.Body.String())
	}
	host := map[string]any{
		"name":        "app",
		"domains":     []string{"app.example.com"},
		"upstream":    map[string]any{"scheme": "http", "host": "10.0.0.5", "port": 8080},
		"accessLists": []string{listName},
		"locations": []map[string]any{
			{"path": "/admin", "accessLists": []string{listName}},
		},
	}
	body, _ = json.Marshal(host)
	if w := do(t, h, "PUT", "/proxy-hosts/app", string(body)); w.Code != http.StatusOK {
		t.Fatalf("seed proxy host: %d %s", w.Code, w.Body.String())
	}
}

// TestMigrateBasicAuthPlanIsADryRun checks ?plan=1 reports the full change set
// and writes nothing.
func TestMigrateBasicAuthPlanIsADryRun(t *testing.T) {
	h, changed := newHandler(t)
	seedLegacyBasicAuth(t, h, "legacy", true)
	before := *changed

	w := do(t, h, "POST", "/access-lists/legacy/migrate-basic-auth?plan=1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", w.Code, w.Body.String())
	}
	var p migrationPlan
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if !p.Plan || p.Commit != "" {
		t.Fatalf("plan flag/commit = %v/%q, want true/\"\"", p.Plan, p.Commit)
	}
	if p.Middleware != "legacy-basic" {
		t.Fatalf("middleware = %q", p.Middleware)
	}
	if len(p.Users) != 1 || p.Users[0] != "admin" {
		t.Fatalf("users = %v", p.Users)
	}
	if !p.SatisfyAny || len(p.AllowFrom) != 1 || p.AllowFrom[0] != "10.0.0.0/8" {
		t.Fatalf("satisfyAny/allowFrom = %v/%v, want the list's allow CIDR", p.SatisfyAny, p.AllowFrom)
	}
	if len(p.AttachTo) != 2 || p.AttachTo[0].Path != "" || p.AttachTo[1].Path != "/admin" {
		t.Fatalf("attachTo = %+v, want the host and its /admin location", p.AttachTo)
	}
	if *changed != before {
		t.Fatal("a plan must not reload the config")
	}
	// The list is untouched.
	w = do(t, h, "GET", "/access-lists/legacy", "")
	var list model.AccessList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	//lint:ignore SA1019 compat read of deprecated AccessList.BasicAuth/SatisfyAny in migration test
	if len(list.BasicAuth) != 1 || !list.SatisfyAny {
		t.Fatalf("plan changed the list: %+v", list)
	}
	if w := do(t, h, "GET", "/middlewares/legacy-basic", ""); w.Code != http.StatusNotFound {
		t.Fatalf("plan created a middleware: %d", w.Code)
	}
}

// TestMigrateBasicAuthApply checks the full conversion lands in one commit: the
// middleware exists with mode basic, both references gained it, and the list is
// cleared.
func TestMigrateBasicAuthApply(t *testing.T) {
	h, _ := newHandler(t)
	seedLegacyBasicAuth(t, h, "legacy", true)

	w := do(t, h, "POST", "/access-lists/legacy/migrate-basic-auth", "")
	if w.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Config-Commit") == "" {
		t.Fatal("missing X-Config-Commit header")
	}
	var p migrationPlan
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if p.Plan || p.Commit == "" {
		t.Fatalf("plan/commit = %v/%q, want false and a commit", p.Plan, p.Commit)
	}

	w = do(t, h, "GET", "/middlewares/legacy-basic", "")
	if w.Code != http.StatusOK {
		t.Fatalf("middleware not created: %d %s", w.Code, w.Body.String())
	}
	var mw model.Middleware
	if err := json.Unmarshal(w.Body.Bytes(), &mw); err != nil {
		t.Fatalf("decode middleware: %v", err)
	}
	if mw.Type != model.MWTypeAuth || mw.Auth == nil || mw.Auth.Mode != model.AuthModeBasic {
		t.Fatalf("middleware = %+v", mw)
	}
	if mw.Auth.Basic == nil || len(mw.Auth.Basic.Users) != 1 ||
		mw.Auth.Basic.Users[0].Username != "admin" || mw.Auth.Basic.Users[0].PasswordHash != bcryptFixture {
		t.Fatalf("credentials did not carry over: %+v", mw.Auth.Basic)
	}
	if mw.Auth.Basic.Realm != "legacy" {
		t.Fatalf("realm = %q, want the access list name", mw.Auth.Basic.Realm)
	}
	if len(mw.Auth.AllowFrom) != 1 || mw.Auth.AllowFrom[0] != "10.0.0.0/8" {
		t.Fatalf("allowFrom = %v, want the satisfyAny allow CIDR", mw.Auth.AllowFrom)
	}

	w = do(t, h, "GET", "/proxy-hosts/app", "")
	var host model.ProxyHost
	if err := json.Unmarshal(w.Body.Bytes(), &host); err != nil {
		t.Fatalf("decode host: %v", err)
	}
	if len(host.Middlewares) != 1 || host.Middlewares[0] != "legacy-basic" {
		t.Fatalf("host middlewares = %v", host.Middlewares)
	}
	if len(host.Locations) != 1 || len(host.Locations[0].Middlewares) != 1 ||
		host.Locations[0].Middlewares[0] != "legacy-basic" {
		t.Fatalf("location middlewares = %+v", host.Locations)
	}
	// The IP rules stay on the list; only the auth dimension moved.
	if len(host.AccessLists) != 1 || host.AccessLists[0] != "legacy" {
		t.Fatalf("host lost its access list: %v", host.AccessLists)
	}

	w = do(t, h, "GET", "/access-lists/legacy", "")
	var list model.AccessList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	//lint:ignore SA1019 compat read of deprecated AccessList.BasicAuth/SatisfyAny in migration test
	if len(list.BasicAuth) != 0 || list.SatisfyAny {
		t.Fatalf("list still carries the deprecated fields: %+v", list)
	}
	if len(list.Rules) != 2 {
		t.Fatalf("list lost its IP rules: %+v", list.Rules)
	}

	// A second apply has nothing to migrate.
	if w := do(t, h, "POST", "/access-lists/legacy/migrate-basic-auth", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("second apply: %d %s", w.Code, w.Body.String())
	}
}

// TestMigrateBasicAuthWithoutSatisfyAny checks the AND case: the list required
// both the IP and the password, so no network becomes an auth exemption.
func TestMigrateBasicAuthWithoutSatisfyAny(t *testing.T) {
	h, _ := newHandler(t)
	seedLegacyBasicAuth(t, h, "strict", false)

	w := do(t, h, "POST", "/access-lists/strict/migrate-basic-auth", "")
	if w.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	var p migrationPlan
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if p.SatisfyAny || len(p.AllowFrom) != 0 {
		t.Fatalf("allowFrom = %v, want none without satisfyAny", p.AllowFrom)
	}
	w = do(t, h, "GET", "/middlewares/strict-basic", "")
	var mw model.Middleware
	if err := json.Unmarshal(w.Body.Bytes(), &mw); err != nil {
		t.Fatalf("decode middleware: %v", err)
	}
	if len(mw.Auth.AllowFrom) != 0 {
		t.Fatalf("middleware allowFrom = %v, want none", mw.Auth.AllowFrom)
	}
}

// TestMigrateBasicAuthErrors covers the refusals.
func TestMigrateBasicAuthErrors(t *testing.T) {
	h, _ := newHandler(t)
	seedLegacyBasicAuth(t, h, "legacy", true)

	if w := do(t, h, "POST", "/access-lists/nope/migrate-basic-auth", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown list: %d %s", w.Code, w.Body.String())
	}

	// A list with no basicAuth has nothing to migrate.
	ipOnly := `{"name":"ip-only","rules":[{"action":"allow","cidr":"10.0.0.0/8"}]}`
	if w := do(t, h, "PUT", "/access-lists/ip-only", ipOnly); w.Code != http.StatusOK {
		t.Fatalf("seed ip-only: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, "POST", "/access-lists/ip-only/migrate-basic-auth", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("no users: %d %s", w.Code, w.Body.String())
	}

	// A middleware already holding the derived name is a conflict, not a
	// silent overwrite of somebody else's object.
	existing := `{"name":"legacy-basic","type":"headers","headers":{"setRequest":{"X-Test":"1"}}}`
	if w := do(t, h, "PUT", "/middlewares/legacy-basic", existing); w.Code != http.StatusOK {
		t.Fatalf("seed middleware: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, "POST", "/access-lists/legacy/migrate-basic-auth", ""); w.Code != http.StatusConflict {
		t.Fatalf("name collision: %d %s", w.Code, w.Body.String())
	}
}

// TestMigrateBasicAuthWarnsOnUnrepresentableRules checks a source- or
// path-scoped allow rule is reported rather than silently widened into an auth
// exemption.
func TestMigrateBasicAuthWarnsOnUnrepresentableRules(t *testing.T) {
	h, _ := newHandler(t)
	list := `{"name":"mixed","satisfyAny":true,"defaultAction":"deny",
		"basicAuth":[{"username":"admin","passwordHash":"` + bcryptFixture + `"}],
		"sources":[{"name":"probes","url":"https://feeds.example.com/ips.txt"}],
		"rules":[
			{"action":"allow","cidr":"10.0.0.0/8"},
			{"action":"allow","source":"probes","paths":["/healthz"]},
			{"action":"allow","cidr":"192.168.0.0/16","paths":["/status"]}
		]}`
	if w := do(t, h, "PUT", "/access-lists/mixed", list); w.Code != http.StatusOK {
		t.Fatalf("seed: %d %s", w.Code, w.Body.String())
	}
	w := do(t, h, "POST", "/access-lists/mixed/migrate-basic-auth?plan=1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", w.Code, w.Body.String())
	}
	var p migrationPlan
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if len(p.AllowFrom) != 1 || p.AllowFrom[0] != "10.0.0.0/8" {
		t.Fatalf("allowFrom = %v, want only the unscoped literal CIDR", p.AllowFrom)
	}
	// One warning per unrepresentable rule, plus the "list stays attached" note:
	// this list carries a source, so its rules cannot all move into allowFrom.
	if len(p.Warnings) != 3 {
		t.Fatalf("warnings = %v, want one per unrepresentable rule plus the stays-attached warning", p.Warnings)
	}
	if p.DetachAccessList {
		t.Fatal("a list with source-backed rules must stay attached")
	}
}

// TestBasicAuthMiddlewareHashesPlaintextPassword checks the write path: a
// caller may POST a plaintext "password" and the API stores only a bcrypt
// passwordHash, never echoing the plaintext back.
func TestBasicAuthMiddlewareHashesPlaintextPassword(t *testing.T) {
	h, _ := newHandler(t)
	body := `{"name":"gate","type":"auth","auth":{"mode":"basic","basic":{
		"realm":"Internal","users":[{"username":"admin","password":"hunter2"}]}}}`
	w := do(t, h, "PUT", "/middlewares/gate", body)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); strings.Contains(got, "hunter2") {
		t.Fatalf("response echoed the plaintext password: %s", got)
	}
	w = do(t, h, "GET", "/middlewares/gate", "")
	var mw model.Middleware
	if err := json.Unmarshal(w.Body.Bytes(), &mw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	stored := mw.Auth.Basic.Users[0].PasswordHash
	if len(stored) != 60 || stored[:4] != "$2a$" {
		t.Fatalf("passwordHash = %q, want a bcrypt hash", stored)
	}

	// Editing the realm without resending a password keeps the stored hash.
	edit := `{"name":"gate","type":"auth","auth":{"mode":"basic","basic":{
		"realm":"Changed","users":[{"username":"admin","passwordHash":"` + stored + `"}]}}}`
	if w := do(t, h, "PUT", "/middlewares/gate", edit); w.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, "GET", "/middlewares/gate", "")
	if err := json.Unmarshal(w.Body.Bytes(), &mw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if mw.Auth.Basic.Users[0].PasswordHash != stored || mw.Auth.Basic.Realm != "Changed" {
		t.Fatalf("edit changed the hash or lost the realm: %+v", mw.Auth.Basic)
	}

	// A plaintext password with no hash and no "password" field is refused: it
	// could never authenticate anyone.
	bad := `{"name":"gate2","type":"auth","auth":{"mode":"basic","basic":{
		"users":[{"username":"admin","passwordHash":"hunter2"}]}}}`
	if w := do(t, h, "PUT", "/middlewares/gate2", bad); w.Code != http.StatusBadRequest {
		t.Fatalf("plaintext passwordHash: %d %s", w.Code, w.Body.String())
	}
}

// TestInlineBasicAuthHashesPlaintextPassword checks the inline `auth:` block on
// a proxy host and on a location accepts a plaintext password on exactly the
// terms a `type: auth` middleware write does: hashed server-side, stored as
// passwordHash only, never echoed.
func TestInlineBasicAuthHashesPlaintextPassword(t *testing.T) {
	h, _ := newHandler(t)
	body := `{"name":"app","domains":["app.example.com"],
		"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
		"auth":{"mode":"basic","basic":{"realm":"Host","users":[
			{"username":"host-user","password":"hunter2"}]}},
		"locations":[{"path":"/admin",
			"auth":{"mode":"basic","basic":{"realm":"Admin","users":[
				{"username":"loc-user","password":"correct horse"}]}}}]}`
	w := do(t, h, "PUT", "/proxy-hosts/app", body)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); strings.Contains(got, "hunter2") || strings.Contains(got, "correct horse") {
		t.Fatalf("response echoed a plaintext password: %s", got)
	}

	w = do(t, h, "GET", "/proxy-hosts/app", "")
	var host model.ProxyHost
	if err := json.Unmarshal(w.Body.Bytes(), &host); err != nil {
		t.Fatalf("decode: %v", err)
	}
	hostHash := host.Auth.Basic.Users[0].PasswordHash
	locHash := host.Locations[0].Auth.Basic.Users[0].PasswordHash
	for _, c := range []struct{ where, hash string }{{"host", hostHash}, {"location", locHash}} {
		if len(c.hash) != 60 || c.hash[:4] != "$2a$" {
			t.Fatalf("%s passwordHash = %q, want a bcrypt hash", c.where, c.hash)
		}
	}
	if hostHash == locHash {
		t.Fatal("the two users were hashed to the same value; the positional merge is wrong")
	}
	if host.Auth.Basic.Realm != "Host" || host.Locations[0].Auth.Basic.Realm != "Admin" {
		t.Fatalf("realms did not round-trip: %q / %q", host.Auth.Basic.Realm, host.Locations[0].Auth.Basic.Realm)
	}

	// Re-saving with the stored hashes and no plaintext keeps both unchanged.
	edit := `{"name":"app","domains":["app.example.com"],
		"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
		"auth":{"mode":"basic","basic":{"realm":"Host2","users":[
			{"username":"host-user","passwordHash":"` + hostHash + `"}]}},
		"locations":[{"path":"/admin",
			"auth":{"mode":"basic","basic":{"realm":"Admin","users":[
				{"username":"loc-user","passwordHash":"` + locHash + `"}]}}}]}`
	if w := do(t, h, "PUT", "/proxy-hosts/app", edit); w.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, "GET", "/proxy-hosts/app", "")
	if err := json.Unmarshal(w.Body.Bytes(), &host); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if host.Auth.Basic.Users[0].PasswordHash != hostHash ||
		host.Locations[0].Auth.Basic.Users[0].PasswordHash != locHash {
		t.Fatal("an edit with no plaintext password changed a stored hash")
	}
	if host.Auth.Basic.Realm != "Host2" {
		t.Fatalf("realm edit lost: %q", host.Auth.Basic.Realm)
	}

	// A location password that is not a bcrypt hash and carries no plaintext is
	// refused, with the location named.
	bad := `{"name":"app2","domains":["app2.example.com"],
		"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},
		"locations":[{"path":"/admin",
			"auth":{"mode":"basic","basic":{"users":[
				{"username":"u","passwordHash":"hunter2"}]}}}]}`
	w = do(t, h, "PUT", "/proxy-hosts/app2", bad)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("plaintext passwordHash on a location: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "is not a bcrypt hash") {
		t.Fatalf("unexpected error: %s", w.Body.String())
	}
}
