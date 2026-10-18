package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// wizardEnv is a handler plus everything a "persisted nothing" assertion needs:
// the store (HEAD, history), the config dir (files), and counters for the
// reload and lifecycle-event hooks a real write fires.
type wizardEnv struct {
	h       http.Handler
	st      *store.Store
	dir     string
	changes int
	events  int
}

func newWizardEnv(t *testing.T) *wizardEnv {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	e := &wizardEnv{st: st, dir: dir}
	e.h = api.New(api.Deps{
		Store:    st,
		OnChange: func() error { e.changes++; return nil },
		OnEvent:  func(string, string, string, string) { e.events++ },
	})
	return e
}

// snapshot returns HEAD and the repo history length.
func (e *wizardEnv) snapshot(t *testing.T) (string, int) {
	t.Helper()
	ctx := context.Background()
	head, err := e.st.Head(ctx)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	hist, err := e.st.RepoHistory(ctx, 1000)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	return head, len(hist)
}

// assertUntouched fails when anything was persisted or fired since the snapshot.
func (e *wizardEnv) assertUntouched(t *testing.T, head string, histLen int, rel string) {
	t.Helper()
	if h, n := e.snapshot(t); h != head || n != histLen {
		t.Fatalf("store changed: head %q -> %q, history %d -> %d", head, h, histLen, n)
	}
	if rel != "" {
		if _, err := os.Stat(filepath.Join(e.dir, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist after a non-persisting call (stat err %v)", rel, err)
		}
	}
	if e.changes != 0 || e.events != 0 {
		t.Fatalf("OnChange fired %d times, OnEvent %d times; want 0", e.changes, e.events)
	}
}

func doCT(t *testing.T, h http.Handler, method, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

type dryRunReply struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	YAML   string `json:"yaml"`
	Exists bool   `json:"exists"`
}

func TestPutDryRunPreviewsWithoutPersisting(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		body     string
		wantKind string
		wantPath string
		wantYAML []string
	}{
		{"proxy host dryRun=1", "/proxy-hosts/app?dryRun=1", validProxyHost, "ProxyHost", "proxy-hosts/app.yaml",
			[]string{"name: app", "app2.example.com", "host: 10.0.0.5", "createdAt:", "updatedAt:"}},
		{"proxy host dryRun=true", "/proxy-hosts/app?dryRun=true", validProxyHost, "ProxyHost", "proxy-hosts/app.yaml",
			[]string{"name: app", "port: 8080"}},
		{"certificate", "/certificates/wild?dryRun=1", validCert, "Certificate", "certificates/wild.yaml",
			[]string{"name: wild", "'*.example.com'"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newWizardEnv(t)
			head, n := e.snapshot(t)
			w := do(t, e.h, "PUT", tc.path, tc.body)
			if w.Code != http.StatusOK {
				t.Fatalf("dry run = %d, want 200: %s", w.Code, w.Body.String())
			}
			if c := w.Header().Get("X-Config-Commit"); c != "" {
				t.Fatalf("dry run set X-Config-Commit %q", c)
			}
			var got dryRunReply
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v: %s", err, w.Body.String())
			}
			wantName := strings.TrimSuffix(filepath.Base(tc.wantPath), ".yaml")
			if got.Kind != tc.wantKind || got.Name != wantName || got.Path != tc.wantPath {
				t.Fatalf("reply = %+v, want kind %q name %q path %q", got, tc.wantKind, wantName, tc.wantPath)
			}
			for _, s := range tc.wantYAML {
				if !strings.Contains(got.YAML, s) {
					t.Fatalf("yaml missing %q:\n%s", s, got.YAML)
				}
			}
			e.assertUntouched(t, head, n, tc.wantPath)
		})
	}
}

// The dry-run YAML is the file a real save then writes, timestamps aside.
func TestPutDryRunMatchesSavedFile(t *testing.T) {
	e := newWizardEnv(t)
	w := do(t, e.h, "PUT", "/proxy-hosts/app?dryRun=1", validProxyHost)
	if w.Code != http.StatusOK {
		t.Fatalf("dry run = %d: %s", w.Code, w.Body.String())
	}
	var got dryRunReply
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if w := do(t, e.h, "PUT", "/proxy-hosts/app", validProxyHost); w.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", w.Code, w.Body.String())
	}
	saved, err := os.ReadFile(filepath.Join(e.dir, got.Path))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if a, b := dropTimes(got.YAML), dropTimes(string(saved)); a != b {
		t.Fatalf("dry run YAML differs from the saved file:\n--- dry run\n%s\n--- saved\n%s", a, b)
	}
}

func dropTimes(s string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "createdAt:") || strings.HasPrefix(l, "updatedAt:") {
			continue
		}
		keep = append(keep, l)
	}
	return strings.Join(keep, "\n")
}

func TestPutDryRunRejects(t *testing.T) {
	dangling := `{"name":"app","domains":["app2.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},"tls":{"certificateRef":"nope"}}`
	noUpstream := `{"name":"app","domains":["app2.example.com"]}`
	tests := []struct {
		name string
		path string
		body string
		// sameAsSave: the 400 message must equal the real PUT's for this body.
		sameAsSave bool
	}{
		{"dangling reference", "/proxy-hosts/app?dryRun=1", dangling, true},
		{"object invalid", "/proxy-hosts/app?dryRun=1", noUpstream, true},
		{"bad name", "/proxy-hosts/Bad_Name?dryRun=1", validProxyHost, true},
		{"malformed body", "/proxy-hosts/app?dryRun=1", `{"domains":`, true},
		{"unknown dryRun value", "/proxy-hosts/app?dryRun=yes", validProxyHost, false},
		{"empty dryRun value", "/proxy-hosts/app?dryRun", validProxyHost, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newWizardEnv(t)
			head, n := e.snapshot(t)
			w := do(t, e.h, "PUT", tc.path, tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("dry run = %d, want 400: %s", w.Code, w.Body.String())
			}
			e.assertUntouched(t, head, n, "proxy-hosts/app.yaml")
			if !tc.sameAsSave {
				return
			}
			real := do(t, e.h, "PUT", strings.SplitN(tc.path, "?", 2)[0], tc.body)
			if real.Code != http.StatusBadRequest || real.Body.String() != w.Body.String() {
				t.Fatalf("real PUT = %d %s; dry run = %s", real.Code, real.Body.String(), w.Body.String())
			}
		})
	}
}

// A dry run must not run beforeSave: for api-tokens that hook mints a secret.
func TestPutDryRunSkipsBeforeSave(t *testing.T) {
	e := newWizardEnv(t)
	head, n := e.snapshot(t)
	w := do(t, e.h, "PUT", "/api-tokens/ci?dryRun=1", validToken)
	if w.Code != http.StatusOK {
		t.Fatalf("dry run = %d: %s", w.Code, w.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["token"]; ok {
		t.Fatalf("dry run minted a token: %s", w.Body.String())
	}
	if y, _ := raw["yaml"].(string); strings.Contains(y, "tokenHash") || !strings.Contains(y, "proxy-hosts:read") {
		t.Fatalf("yaml = %q, want the scopes and no tokenHash", y)
	}
	if raw["path"] != "api-tokens/ci.yaml" || raw["kind"] != "APIToken" {
		t.Fatalf("reply = %v", raw)
	}
	e.assertUntouched(t, head, n, "api-tokens/ci.yaml")
}

type parseReply struct {
	Name   string          `json:"name"`
	Object json.RawMessage `json:"object"`
	Valid  bool            `json:"valid"`
	Error  string          `json:"error"`
}

const hostYAML = `name: app
domains:
  - app2.example.com
upstream:
  scheme: http
  host: 10.0.0.5
  port: 8080
`

// billionLaughs is a small exponential-alias document: 9 levels of 9 aliases.
const billionLaughs = `name: app
a: &a ["x","x","x","x","x","x","x","x","x"]
b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]
d: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c]
e: &e [*d,*d,*d,*d,*d,*d,*d,*d,*d]
f: &f [*e,*e,*e,*e,*e,*e,*e,*e,*e]
g: &g [*f,*f,*f,*f,*f,*f,*f,*f,*f]
h: &h [*g,*g,*g,*g,*g,*g,*g,*g,*g]
i: &i [*h,*h,*h,*h,*h,*h,*h,*h,*h]
`

func TestParseYAML(t *testing.T) {
	noUpstreamErr := model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app2.example.com"}}.Validate()
	if noUpstreamErr == nil {
		t.Fatal("fixture: a host without an upstream must fail Validate")
	}
	tests := []struct {
		name        string
		path        string
		contentType string
		body        string
		wantStatus  int
		wantName    string
		wantValid   bool
		wantErr     string // substring of the reply's error (400/415) or Validate message (200)
	}{
		{"happy path", "/proxy-hosts/_parse", "application/yaml", hostYAML, 200, "app", true, ""},
		{"x-yaml with charset", "/proxy-hosts/_parse", "application/x-yaml; charset=utf-8", hostYAML, 200, "app", true, ""},
		{"text/yaml", "/proxy-hosts/_parse", "text/yaml", hostYAML, 200, "app", true, ""},
		{"name from query", "/proxy-hosts/_parse?name=web", "application/yaml", strings.TrimPrefix(hostYAML, "name: app\n"), 200, "web", true, ""},
		{"body name wins over query", "/proxy-hosts/_parse?name=web", "application/yaml", hostYAML, 200, "app", true, ""},
		{"decodes but fails Validate", "/proxy-hosts/_parse", "application/yaml", "name: app\ndomains: [app2.example.com]\n", 200, "app", false, noUpstreamErr.Error()},
		{"invalid name is reported, not refused", "/proxy-hosts/_parse", "application/yaml", strings.Replace(hostYAML, "name: app", "name: Bad_Name", 1), 200, "Bad_Name", false, "invalid name"},
		{"invalid YAML", "/proxy-hosts/_parse", "application/yaml", "name: app\ndomains: [unclosed\n", 400, "", false, "invalid YAML"},
		{"sequence document", "/proxy-hosts/_parse", "application/yaml", "- a\n- b\n", 400, "", false, "must be a mapping"},
		{"scalar document", "/proxy-hosts/_parse", "application/yaml", "just a string\n", 400, "", false, "must be a mapping"},
		{"empty body", "/proxy-hosts/_parse", "application/yaml", "", 400, "", false, "empty"},
		{"two documents", "/proxy-hosts/_parse", "application/yaml", hostYAML + "---\nname: other\n", 400, "", false, "exactly one YAML document"},
		{"wrong field type", "/proxy-hosts/_parse", "application/yaml", "name: app\ndomains: notalist\n", 400, "", false, "domains"},
		{"missing name", "/proxy-hosts/_parse", "application/yaml", "domains: [app2.example.com]\n", 400, "", false, "name is required"},
		{"non-string name", "/proxy-hosts/_parse", "application/yaml", "name: [a]\n", 400, "", false, "name must be a string"},
		{"json content type", "/proxy-hosts/_parse", "application/json", hostYAML, 415, "", false, "Content-Type"},
		{"no content type", "/proxy-hosts/_parse", "", hostYAML, 415, "", false, "Content-Type"},
		{"excessive aliasing", "/proxy-hosts/_parse", "application/yaml", billionLaughs, 400, "", false, "excessive aliasing"},
		{"body over the cap", "/proxy-hosts/_parse", "application/yaml", "name: app\nx: " + strings.Repeat("a", 1<<20) + "\n", 400, "", false, "too large"},
		{"other kind", "/certificates/_parse", "application/yaml", "name: wild\ntype: custom\ndomains: ['*.example.com']\ncustom: {certFile: c.pem, keyFile: k.pem}\n", 200, "wild", true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newWizardEnv(t)
			head, n := e.snapshot(t)
			w := doCT(t, e.h, "POST", tc.path, tc.contentType, tc.body)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			e.assertUntouched(t, head, n, "proxy-hosts/app.yaml")
			if tc.wantStatus != http.StatusOK {
				var er map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &er); err != nil || !strings.Contains(er["error"], tc.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", w.Body.String(), tc.wantErr)
				}
				return
			}
			var got parseReply
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v: %s", err, w.Body.String())
			}
			if got.Name != tc.wantName || got.Valid != tc.wantValid {
				t.Fatalf("reply = %+v, want name %q valid %v", got, tc.wantName, tc.wantValid)
			}
			if tc.wantValid && got.Error != "" {
				t.Fatalf("valid reply carries error %q", got.Error)
			}
			if !tc.wantValid && (got.Error == "" || !strings.Contains(got.Error, tc.wantErr)) {
				t.Fatalf("error = %q, want it to contain %q", got.Error, tc.wantErr)
			}
			var meta struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(got.Object, &meta); err != nil || meta.Name != tc.wantName {
				t.Fatalf("object = %s, want name %q", got.Object, tc.wantName)
			}
		})
	}
}

// _parse returns the bare object, never the decorated GET projection: the
// certificate decorate step reads certFile from the certificate store, and an
// unvalidated object could point it outside that store. The reply must carry
// no runtime field and the traversal must only show up as a Validate error.
func TestParseYAMLDoesNotDecorate(t *testing.T) {
	e := newWizardEnv(t)
	body := "name: wild\ntype: custom\ndomains: ['*.example.com']\ncustom: {certFile: ../../outside/other.pem, keyFile: ../../outside/other.key}\n"
	w := doCT(t, e.h, "POST", "/certificates/_parse", "application/yaml", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var got parseReply
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Valid {
		t.Fatalf("a certFile outside the store must not validate: %s", w.Body.String())
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(got.Object, &fields); err != nil {
		t.Fatalf("object: %v", err)
	}
	for _, k := range []string{"state", "issuer", "sans", "notAfter", "daysRemaining", "lastError", "certStatusError"} {
		if _, ok := fields[k]; ok {
			t.Fatalf("reply carries decorated field %q: %s", k, got.Object)
		}
	}
}

// The proxy-host decode hashes inline basic-auth passwords; _parse must go
// through it so a plaintext password never comes back.
func TestParseYAMLHashesInlineBasicAuth(t *testing.T) {
	e := newWizardEnv(t)
	body := hostYAML + `auth:
  mode: basic
  basic:
    realm: Internal
    users:
      - username: admin
        password: hunter2
`
	w := doCT(t, e.h, "POST", "/proxy-hosts/_parse", "application/yaml", body)
	if w.Code != http.StatusOK {
		t.Fatalf("parse = %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "hunter2") {
		t.Fatalf("reply echoed the plaintext password: %s", w.Body.String())
	}
	var got struct {
		Object model.ProxyHost `json:"object"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if a := got.Object.Auth; a == nil || a.Basic == nil || len(a.Basic.Users) != 1 ||
		!strings.HasPrefix(a.Basic.Users[0].PasswordHash, "$2a$") {
		t.Fatalf("auth = %+v, want one user with a bcrypt passwordHash", got.Object.Auth)
	}
}

// "_parse" cannot be an object name, and the POST pattern does not shadow the
// GET {name} lookup: GET /proxy-hosts/_parse is an ordinary not-found.
func TestParseRouteDoesNotShadowLookup(t *testing.T) {
	if err := model.ValidateName("_parse"); err == nil {
		t.Fatal("_parse must not be a valid object name")
	}
	e := newWizardEnv(t)
	if w := do(t, e.h, "GET", "/proxy-hosts/_parse", ""); w.Code != http.StatusNotFound {
		t.Fatalf("GET /proxy-hosts/_parse = %d, want 404: %s", w.Code, w.Body.String())
	}
}

// If-None-Match: * turns the PUT into a create: a stored name answers 409 and
// nothing moves; a new name is created; the dry run reports exists.
func TestPutCreateOnly(t *testing.T) {
	e := newWizardEnv(t)
	req := func(path string, ifNoneMatch bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", path, strings.NewReader(validProxyHost))
		r.Header.Set("Content-Type", "application/json")
		if ifNoneMatch {
			r.Header.Set("If-None-Match", "*")
		}
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		return w
	}
	exists := func(w *httptest.ResponseRecorder) bool {
		var got dryRunReply
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil {
			t.Fatalf("dry run = %d: %s", w.Code, w.Body.String())
		}
		return got.Exists
	}
	if exists(req("/proxy-hosts/app?dryRun=1", false)) {
		t.Fatal("dry run before create reports exists=true")
	}
	if w := req("/proxy-hosts/app", true); w.Code != http.StatusOK {
		t.Fatalf("create-only PUT of a new name = %d: %s", w.Code, w.Body.String())
	}
	if !exists(req("/proxy-hosts/app?dryRun=1", false)) {
		t.Fatal("dry run after create reports exists=false")
	}
	head, n := e.snapshot(t)
	w := req("/proxy-hosts/app", true)
	if w.Code != http.StatusConflict {
		t.Fatalf("create-only PUT of a stored name = %d, want 409: %s", w.Code, w.Body.String())
	}
	if c := w.Header().Get("X-Config-Commit"); c != "" {
		t.Fatalf("refused create set X-Config-Commit %q", c)
	}
	if after, m := e.snapshot(t); after != head || m != n {
		t.Fatalf("refused create changed the store: head %q -> %q, history %d -> %d", head, after, n, m)
	}
	if w := req("/proxy-hosts/app", false); w.Code != http.StatusOK {
		t.Fatalf("plain PUT (update) of a stored name = %d: %s", w.Code, w.Body.String())
	}
}
