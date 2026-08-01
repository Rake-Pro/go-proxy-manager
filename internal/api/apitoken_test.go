package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/dnssync"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	"gopkg.in/yaml.v3"
)

// scopedHandler builds an API handler whose RequireScope behaves as if every
// request came from an API token holding the given scopes. This is the same
// injection point cmd/gpm uses; passing no scopes at all is how a session
// principal is modelled (nil RequireScope, checked by the other tests).
func scopedHandler(t *testing.T, scopes ...string) http.Handler {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	p := auth.Principal{Role: auth.RoleAdmin, IsToken: true, Subject: "token:ci", Scopes: scopes}
	return api.New(api.Deps{
		Store:        st,
		RequireScope: func(_ *http.Request, required string) error { return auth.RequireScope(p, required) },
	})
}

// newHandlerDir is newHandler plus the config repo path, for the assertions that
// have to read the stored token digest off disk (it is json:"-", so no response
// carries it).
func newHandlerDir(t *testing.T) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	return api.New(api.Deps{Store: st}), dir
}

const validToken = `{"scopes":["proxy-hosts:read"]}`

func TestAPITokenMintAndRotate(t *testing.T) {
	h, dir := newHandlerDir(t)

	// Create: the plaintext secret is returned exactly once.
	w := do(t, h, "PUT", "/api-tokens/ci", validToken)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT want 200 got %d: %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	secret, _ := created["token"].(string)
	if !strings.HasPrefix(secret, auth.TokenPrefix) {
		t.Fatalf("response token %q must be a gpm secret", secret)
	}
	if created["tokenNote"] == nil {
		t.Fatal("response must say the token is shown once")
	}
	// The digest is stored, never served: it is offline-crackable, so it stays
	// out of every response (see model.APIToken.TokenHash's json:"-").
	if _, leaked := created["tokenHash"]; leaked {
		t.Fatal("the create response must not carry the stored digest")
	}
	hash := auth.HashTokenSecret(secret)
	if stored := storedTokenHash(t, dir, "ci"); stored != hash {
		t.Fatalf("stored hash %q is not the digest of the returned secret", stored)
	}

	// Read back: the secret is gone forever and the digest is not served either.
	w = do(t, h, "GET", "/api-tokens/ci", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET want 200 got %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, leaked := got["token"]; leaked {
		t.Fatal("GET must never return the plaintext token")
	}
	if _, leaked := got["tokenHash"]; leaked {
		t.Fatal("GET must never return the stored digest")
	}

	// Plain edit: scopes change, the digest is carried forward, no new secret.
	w = do(t, h, "PUT", "/api-tokens/ci", `{"scopes":["proxy-hosts:read","certificates:read"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("edit want 200 got %d: %s", w.Code, w.Body.String())
	}
	var edited map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &edited)
	if _, minted := edited["token"]; minted {
		t.Fatal("an ordinary edit must not mint a new secret")
	}
	if stored := storedTokenHash(t, dir, "ci"); stored != hash {
		t.Fatalf("an ordinary edit must keep the stored digest, got %q", stored)
	}

	// Rotation: a new secret and a new digest.
	w = do(t, h, "PUT", "/api-tokens/ci?rotate=1", validToken)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate want 200 got %d: %s", w.Code, w.Body.String())
	}
	var rotated map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rotated)
	newSecret, _ := rotated["token"].(string)
	if newSecret == "" || newSecret == secret {
		t.Fatalf("rotation must mint a different secret, got %q", newSecret)
	}
	if stored := storedTokenHash(t, dir, "ci"); stored != auth.HashTokenSecret(newSecret) {
		t.Fatalf("rotation must store the digest of the new secret, got %q", stored)
	}
}

// storedTokenHash reads the digest straight off disk, since it is deliberately
// absent from every API response.
func storedTokenHash(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "api-tokens", name+".yaml"))
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	var tok struct {
		TokenHash string `yaml:"tokenHash"`
	}
	if err := yaml.Unmarshal(b, &tok); err != nil {
		t.Fatalf("decode token file: %v", err)
	}
	return tok.TokenHash
}

// The digest must not leak through the whole-config reads either - those need
// only "*:read", which is a far weaker grant than token management.
func TestAPITokenHashNeverLeaksThroughConfigDump(t *testing.T) {
	h, dir := newHandlerDir(t)
	if w := do(t, h, "PUT", "/api-tokens/ci", validToken); w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	hash := storedTokenHash(t, dir, "ci")
	if hash == "" {
		t.Fatal("no digest was stored")
	}
	for _, path := range []string{"/config", "/api-tokens", "/api-tokens/ci"} {
		w := do(t, h, "GET", path, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, w.Code)
		}
		if strings.Contains(w.Body.String(), hash) {
			t.Fatalf("GET %s leaked the stored token digest", path)
		}
	}
}

// A client-supplied tokenHash must be discarded: accepting one would let a
// caller install a digest whose preimage only they know.
func TestAPITokenRejectsClientSuppliedHash(t *testing.T) {
	h, dir := newHandlerDir(t)
	planted := strings.Repeat("a", 64)
	w := do(t, h, "PUT", "/api-tokens/ci", `{"scopes":["*:read"],"tokenHash":"`+planted+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT want 200 got %d: %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	stored := storedTokenHash(t, dir, "ci")
	if stored == planted {
		t.Fatal("the server must ignore a client-supplied tokenHash")
	}
	secret, _ := created["token"].(string)
	if stored != auth.HashTokenSecret(secret) {
		t.Fatal("stored digest must match the server-minted secret")
	}
}

// Reverting an API token would restore an older digest and revive a secret the
// operator rotated away, so it is refused with an explanation - the API surfaces
// the store's refusal as a 400 rather than quietly committing it.
func TestAPITokenRevertIsRefused(t *testing.T) {
	h, _ := newHandlerDir(t)
	if w := do(t, h, "PUT", "/api-tokens/ci", validToken); w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	w := do(t, h, "GET", "/api-tokens/ci/history", "")
	if w.Code != http.StatusOK {
		t.Fatalf("history: %d", w.Code)
	}
	var commits []struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &commits); err != nil || len(commits) == 0 {
		t.Fatalf("decode history: %v (%s)", err, w.Body.String())
	}

	w = do(t, h, "POST", "/api-tokens/ci/revert", `{"hash":"`+commits[0].Hash+`"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("revert of an API token = %d, want 400: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cannot be reverted") {
		t.Fatalf("the refusal must say why, got %s", w.Body.String())
	}
}

func TestAPITokenValidationRejectsBadScope(t *testing.T) {
	h, _ := newHandler(t)
	w := do(t, h, "PUT", "/api-tokens/ci", `{"scopes":["widgets:read"]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown scope subject want 400 got %d: %s", w.Code, w.Body.String())
	}
	w = do(t, h, "PUT", "/api-tokens/ci", `{"scopes":[]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty scope list want 400 got %d", w.Code)
	}
}

func TestAPITokenCRUDLifecycle(t *testing.T) {
	h, _ := newHandler(t)
	if w := do(t, h, "PUT", "/api-tokens/ci", validToken); w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	w := do(t, h, "GET", "/api-tokens", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	var list []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0]["name"] != "ci" {
		t.Fatalf("list = %v", list)
	}
	if w := do(t, h, "GET", "/api-tokens/ci/history", ""); w.Code != http.StatusOK {
		t.Fatalf("history: %d", w.Code)
	}
	if w := do(t, h, "DELETE", "/api-tokens/ci", ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, h, "GET", "/api-tokens/ci", ""); w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: %d", w.Code)
	}
}

const validSettings = `{"schemaVersion":1,"adminAuth":{"localLoginEnabled":true}}`

func TestScopeEnforcement(t *testing.T) {
	tests := []struct {
		name       string
		scopes     []string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"read scope allows list", []string{"proxy-hosts:read"}, "GET", "/proxy-hosts", "", http.StatusOK},
		{"read scope blocks write", []string{"proxy-hosts:read"}, "PUT", "/proxy-hosts/app", validProxyHost, http.StatusForbidden},
		{"write scope allows write", []string{"proxy-hosts:write"}, "PUT", "/proxy-hosts/app", validProxyHost, http.StatusOK},
		{"write scope allows read", []string{"proxy-hosts:write"}, "GET", "/proxy-hosts", "", http.StatusOK},
		{"wrong resource is blocked", []string{"certificates:write"}, "GET", "/proxy-hosts", "", http.StatusForbidden},
		{"wildcard read allows list", []string{"*:read"}, "GET", "/proxy-hosts", "", http.StatusOK},
		{"wildcard read blocks write", []string{"*:read"}, "PUT", "/proxy-hosts/app", validProxyHost, http.StatusForbidden},
		{"delete needs write", []string{"proxy-hosts:read"}, "DELETE", "/proxy-hosts/app", "", http.StatusForbidden},
		{"object revert needs write", []string{"proxy-hosts:read"}, "POST", "/proxy-hosts/app/revert", `{"hash":"abcdef1"}`, http.StatusForbidden},
		{"history needs read", []string{"certificates:read"}, "GET", "/proxy-hosts/app/history", "", http.StatusForbidden},

		{"settings read scope", []string{"settings:read"}, "GET", "/settings", "", http.StatusOK},
		{"settings read blocks write", []string{"settings:read"}, "PUT", "/settings", validSettings, http.StatusForbidden},
		{"host scope does not reach settings", []string{"proxy-hosts:write"}, "GET", "/settings", "", http.StatusForbidden},
		// Writing settings is admin-equivalent (it can aim dnsSync/webhooks at an
		// attacker with a ${ENV:...} credential, and rewrite adminAuth), so a
		// settings:write grant is deliberately NOT enough.
		{"settings write scope does not reach PUT", []string{"settings:write"}, "PUT", "/settings", validSettings, http.StatusForbidden},
		{"wildcard write does not reach settings PUT", []string{"*:write"}, "PUT", "/settings", validSettings, http.StatusForbidden},
		{"admin writes settings", []string{model.ScopeAdmin}, "PUT", "/settings", validSettings, http.StatusOK},
		{"settings write still allows read", []string{"settings:write"}, "GET", "/settings", "", http.StatusOK},

		{"api-tokens needs admin", []string{"*:write"}, "GET", "/api-tokens", "", http.StatusForbidden},
		{"api-tokens write needs admin", []string{"*:write"}, "PUT", "/api-tokens/x", validToken, http.StatusForbidden},
		{"api-tokens delete needs admin", []string{"*:write"}, "DELETE", "/api-tokens/x", "", http.StatusForbidden},
		{"api-tokens revert needs admin", []string{"*:write"}, "POST", "/api-tokens/x/revert", `{"hash":"abcdef1"}`, http.StatusForbidden},
		{"admin reaches api-tokens", []string{model.ScopeAdmin}, "GET", "/api-tokens", "", http.StatusOK},
		// 404: past the scope gate, refused by the store because it does not exist.
		{"admin reaches api-tokens delete", []string{model.ScopeAdmin}, "DELETE", "/api-tokens/x", "", http.StatusNotFound},
		{"restore needs admin", []string{"*:write"}, "POST", "/restore", "", http.StatusForbidden},
		{"global revert needs admin", []string{"*:write"}, "POST", "/revert", `{"hash":"abcdef1"}`, http.StatusForbidden},
		{"sso revoke needs admin", []string{"*:write"}, "POST", "/sso/revoke", "", http.StatusForbidden},
		// The admin cases below get past the scope gate and fail on the request
		// itself (empty archive / unknown commit) or on an unwired dependency,
		// which is exactly what proves the gate let them through.
		{"admin reaches restore", []string{model.ScopeAdmin}, "POST", "/restore", "", http.StatusBadRequest},
		{"admin reaches global revert", []string{model.ScopeAdmin}, "POST", "/revert", `{"hash":"abcdef1"}`, http.StatusBadRequest},
		{"admin reaches sso revoke", []string{model.ScopeAdmin}, "POST", "/sso/revoke", "", http.StatusNotImplemented},

		{"config dump needs wildcard read", []string{"proxy-hosts:read"}, "GET", "/config", "", http.StatusForbidden},
		{"wildcard read reaches config dump", []string{"*:read"}, "GET", "/config", "", http.StatusOK},
		// The backup archive is raw on-disk YAML, so it carries the api-tokens'
		// stored digests: admin scope, not "*:read".
		{"backup needs admin", []string{"proxy-hosts:read"}, "GET", "/backup", "", http.StatusForbidden},
		{"wildcard read does not reach backup", []string{"*:read"}, "GET", "/backup", "", http.StatusForbidden},
		{"admin reaches backup", []string{model.ScopeAdmin}, "GET", "/backup", "", http.StatusOK},
		{"repo history needs wildcard read", []string{"proxy-hosts:read"}, "GET", "/history", "", http.StatusForbidden},
		{"wildcard read reaches repo history", []string{"*:read"}, "GET", "/history", "", http.StatusOK},

		{"logs need wildcard read", []string{"proxy-hosts:read"}, "GET", "/logs", "", http.StatusForbidden},
		{"wildcard read reaches logs", []string{"*:read"}, "GET", "/logs", "", http.StatusOK},
		{"upstream health needs wildcard read", []string{"proxy-hosts:read"}, "GET", "/upstream-health", "", http.StatusForbidden},
		{"wildcard read reaches upstream health", []string{"*:read"}, "GET", "/upstream-health", "", http.StatusOK},

		{"dns-sync read scope", []string{"dns-sync:read"}, "GET", "/dns-sync/status", "", http.StatusNotImplemented},
		{"dns-sync read blocks reconcile", []string{"dns-sync:read"}, "POST", "/dns-sync/reconcile", "", http.StatusForbidden},
		{"dns-sync write reaches reconcile", []string{"dns-sync:write"}, "POST", "/dns-sync/reconcile", "", http.StatusNotImplemented},
		{"unrelated scope blocks dns-sync", []string{"proxy-hosts:write"}, "GET", "/dns-sync/status", "", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := scopedHandler(t, tc.scopes...)
			w := do(t, h, tc.method, tc.path, tc.body)
			if w.Code != tc.wantStatus {
				t.Fatalf("%s %s = %d, want %d: %s", tc.method, tc.path, w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}
}

// /capabilities is the one route every authenticated caller can reach: a token
// must be able to discover what the instance supports whatever its scopes.
func TestCapabilitiesNeedsNoScope(t *testing.T) {
	h := scopedHandler(t, "proxy-hosts:read")
	w := do(t, h, "GET", "/capabilities", "")
	if w.Code != http.StatusOK {
		t.Fatalf("capabilities = %d, want 200: %s", w.Code, w.Body.String())
	}
	var caps struct {
		DNSSync struct {
			PiholeEnabled     bool `json:"piholeEnabled"`
			CloudflareEnabled bool `json:"cloudflareEnabled"`
		} `json:"dnsSync"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if caps.DNSSync.PiholeEnabled || caps.DNSSync.CloudflareEnabled {
		t.Fatal("dnsSync capabilities must report disabled with no syncer wired")
	}
}

func TestCapabilitiesReportsGroups(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	h := api.New(api.Deps{
		Store:          st,
		RequireScope:   func(*http.Request, string) error { return nil },
		DNSSyncEnabled: func() (bool, bool) { return true, false },
	})
	w := do(t, h, "GET", "/capabilities", "")
	var caps map[string]map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if caps["apiTokens"]["enabled"] != true {
		t.Fatalf("apiTokens capability = %v", caps["apiTokens"])
	}
	if caps["dnsSync"]["piholeEnabled"] != true || caps["dnsSync"]["cloudflareEnabled"] != false {
		t.Fatalf("dnsSync capability = %v", caps["dnsSync"])
	}
}

// With no RequireScope wired, every route stays open - the session-principal
// behaviour the admin SPA has always relied on.
func TestNilScopeCheckAllowsEverything(t *testing.T) {
	h, _ := newHandler(t)
	for _, path := range []string{"/config", "/history", "/settings", "/api-tokens", "/capabilities"} {
		if w := do(t, h, "GET", path, ""); w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200 (unscoped handler must allow)", path, w.Code)
		}
	}
}

func TestDNSSyncEndpoints(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	runs := 0
	failing := false
	h := api.New(api.Deps{
		Store: st,
		DNSSyncReconcile: func(context.Context) error {
			runs++
			if failing {
				return fmt.Errorf("pihole unreachable")
			}
			return nil
		},
		DNSSyncStatus: func() any { return map[string]any{"runs": runs} },
	})

	w := do(t, h, "GET", "/dns-sync/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if w = do(t, h, "POST", "/dns-sync/reconcile", ""); w.Code != http.StatusOK {
		t.Fatalf("reconcile = %d: %s", w.Code, w.Body.String())
	}
	if runs != 1 {
		t.Fatalf("reconcile ran %d times, want 1", runs)
	}
	failing = true
	if w = do(t, h, "POST", "/dns-sync/reconcile", ""); w.Code != http.StatusBadGateway {
		t.Fatalf("failing reconcile = %d, want 502", w.Code)
	}
}

// A reconcile already in flight is a conflict, not a backend failure: the
// manual endpoint refuses rather than queueing a blocked goroutine behind it.
func TestDNSSyncReconcileInProgressIsConflict(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	h := api.New(api.Deps{
		Store: st,
		DNSSyncReconcile: func(context.Context) error {
			return fmt.Errorf("manual run refused: %w", dnssync.ErrReconcileInProgress)
		},
	})
	if w := do(t, h, "POST", "/dns-sync/reconcile", ""); w.Code != http.StatusConflict {
		t.Fatalf("reconcile while running = %d, want 409: %s", w.Code, w.Body.String())
	}
}

// encoding/json ignores omitempty on a struct value, so a plain DNSSyncPolicy
// field put a noise "dns":{} on every proxy-host response. It is a pointer for
// exactly that reason - an opted-out host must carry no dns key at all.
func TestProxyHostOmitsEmptyDNSPolicy(t *testing.T) {
	h, _ := newHandler(t)
	if w := do(t, h, "PUT", "/proxy-hosts/app", validProxyHost); w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	w := do(t, h, "GET", "/proxy-hosts/app", "")
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, present := got["dns"]; present {
		t.Fatalf("a host with no dns policy must omit the key entirely, got %v", got["dns"])
	}

	// An opted-in host round-trips the policy.
	body := `{"domains":["opted.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},"dns":{"lanDirect":true}}`
	if w := do(t, h, "PUT", "/proxy-hosts/opted", body); w.Code != http.StatusOK {
		t.Fatalf("create opted-in: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, "GET", "/proxy-hosts/opted", "")
	var opted struct {
		DNS *model.DNSSyncPolicy `json:"dns"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &opted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if opted.DNS == nil || !opted.DNS.LanDirect || opted.DNS.PublicCname {
		t.Fatalf("dns policy = %+v", opted.DNS)
	}
}

func TestDNSSyncEndpointsUnwired(t *testing.T) {
	h, _ := newHandler(t)
	if w := do(t, h, "GET", "/dns-sync/status", ""); w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", w.Code)
	}
	if w := do(t, h, "POST", "/dns-sync/reconcile", ""); w.Code != http.StatusNotImplemented {
		t.Fatalf("reconcile = %d, want 501", w.Code)
	}
}

func TestAPITokenLastUsedDecoration(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	used := map[string]time.Time{}
	h := api.New(api.Deps{Store: st, TokenLastUsed: func() map[string]time.Time { return used }})

	if w := do(t, h, "PUT", "/api-tokens/ci", validToken); w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	w := do(t, h, "GET", "/api-tokens/ci", "")
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if _, present := got["lastUsed"]; present {
		t.Fatal("an unused token must not carry a lastUsed timestamp")
	}

	used["ci"] = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	w = do(t, h, "GET", "/api-tokens/ci", "")
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["lastUsed"] != "2026-03-04T05:06:07Z" {
		t.Fatalf("lastUsed = %v", got["lastUsed"])
	}
}
