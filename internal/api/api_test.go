package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

func newHandler(t *testing.T) (http.Handler, *int) {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	changed := 0
	h := api.New(api.Deps{Store: st, OnChange: func() error { changed++; return nil }})
	return h, &changed
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const validProxyHost = `{"name":"app","domains":["app2.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080}}`

const validCert = `{"name":"wild","type":"custom","domains":["*.example.com"],"custom":{"certFile":"c.pem","keyFile":"k.pem"}}`

func TestProxyHostCRUD(t *testing.T) {
	h, changed := newHandler(t)

	// PUT create.
	w := do(t, h, "PUT", "/proxy-hosts/app", validProxyHost)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT want 200 got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Config-Commit") == "" {
		t.Fatal("missing X-Config-Commit header")
	}
	if *changed != 1 {
		t.Fatalf("OnChange want 1 got %d", *changed)
	}

	// List contains app.
	w = do(t, h, "GET", "/proxy-hosts", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list want 200 got %d", w.Code)
	}
	var hosts []model.ProxyHost
	if err := json.Unmarshal(w.Body.Bytes(), &hosts); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "app" {
		t.Fatalf("list = %+v", hosts)
	}

	// Get one.
	w = do(t, h, "GET", "/proxy-hosts/app", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get want 200 got %d", w.Code)
	}
	var got model.ProxyHost
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Upstream.Host != "10.0.0.5" {
		t.Fatalf("get upstream = %+v", got.Upstream)
	}

	// Missing -> 404.
	w = do(t, h, "GET", "/proxy-hosts/missing", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing want 404 got %d", w.Code)
	}

	// History non-empty.
	w = do(t, h, "GET", "/proxy-hosts/app/history", "")
	if w.Code != http.StatusOK {
		t.Fatalf("history want 200 got %d", w.Code)
	}
	var commits []store.Commit
	if err := json.Unmarshal(w.Body.Bytes(), &commits); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("history empty")
	}
}

func TestEmptyListIsArray(t *testing.T) {
	h, _ := newHandler(t)
	w := do(t, h, "GET", "/middlewares", "")
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty list = %q want []", strings.TrimSpace(w.Body.String()))
	}
}

func TestDanglingCertRefRejected(t *testing.T) {
	h, _ := newHandler(t)
	body := `{"name":"app","domains":["app2.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},"tls":{"certificateRef":"nope"}}`
	w := do(t, h, "PUT", "/proxy-hosts/app", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("dangling ref want 400 got %d: %s", w.Code, w.Body.String())
	}
	var e map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || e["error"] == "" {
		t.Fatalf("error body = %q", w.Body.String())
	}
}

func TestDeleteConflictAndSuccess(t *testing.T) {
	h, _ := newHandler(t)

	// Create cert and a host that references it.
	if w := do(t, h, "PUT", "/certificates/wild", validCert); w.Code != http.StatusOK {
		t.Fatalf("PUT cert want 200 got %d: %s", w.Code, w.Body.String())
	}
	hostWithCert := `{"name":"app","domains":["app2.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},"tls":{"certificateRef":"wild"}}`
	if w := do(t, h, "PUT", "/proxy-hosts/app", hostWithCert); w.Code != http.StatusOK {
		t.Fatalf("PUT host want 200 got %d: %s", w.Code, w.Body.String())
	}

	// Deleting referenced cert -> 409.
	if w := do(t, h, "DELETE", "/certificates/wild", ""); w.Code != http.StatusConflict {
		t.Fatalf("delete referenced cert want 409 got %d: %s", w.Code, w.Body.String())
	}

	// Deleting the host (unreferenced by anything) -> 204.
	if w := do(t, h, "DELETE", "/proxy-hosts/app", ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete host want 204 got %d", w.Code)
	}

	// Now cert is unreferenced -> 204.
	if w := do(t, h, "DELETE", "/certificates/wild", ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete cert want 204 got %d", w.Code)
	}
}

func TestConfigHistorySettings(t *testing.T) {
	h, _ := newHandler(t)
	if w := do(t, h, "PUT", "/proxy-hosts/app", validProxyHost); w.Code != http.StatusOK {
		t.Fatalf("PUT host want 200 got %d", w.Code)
	}

	// Full config includes the host.
	w := do(t, h, "GET", "/config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("config want 200 got %d", w.Code)
	}
	var cfg model.Config
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 {
		t.Fatalf("config proxy hosts = %d", len(cfg.ProxyHosts))
	}

	// Repo history.
	w = do(t, h, "GET", "/history", "")
	if w.Code != http.StatusOK {
		t.Fatalf("history want 200 got %d", w.Code)
	}
	var commits []store.Commit
	if err := json.Unmarshal(w.Body.Bytes(), &commits); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("repo history empty")
	}

	// Settings round trip.
	if w := do(t, h, "GET", "/settings", ""); w.Code != http.StatusOK {
		t.Fatalf("get settings want 200 got %d", w.Code)
	}
	settingsBody := `{"externalBaseURL":"https://proxy.example.com","adminAuth":{"localLoginEnabled":true}}`
	w = do(t, h, "PUT", "/settings", settingsBody)
	if w.Code != http.StatusOK {
		t.Fatalf("put settings want 200 got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Config-Commit") == "" {
		t.Fatal("settings missing X-Config-Commit")
	}
}

func TestDeleteRejectsTraversal(t *testing.T) {
	h, changed := newHandler(t)
	w := do(t, h, "DELETE", "/proxy-hosts/..%2f..%2fetc%2fx", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("DELETE traversal want 400 got %d: %s", w.Code, w.Body.String())
	}
	if *changed != 0 {
		t.Fatalf("OnChange should not fire, got %d", *changed)
	}
}

func TestLiteralSecretRejectedViaAPI(t *testing.T) {
	h, _ := newHandler(t)
	body := `{"name":"idp","type":"oidc","oidc":{"issuerURL":"https://idp.example.com","clientID":"gpm","clientSecret":"plaintext"}}`
	w := do(t, h, "PUT", "/identity-providers/idp", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT literal secret want 400 got %d: %s", w.Code, w.Body.String())
	}
}

func TestBackupRestoreAndRevertViaAPI(t *testing.T) {
	h, changed := newHandler(t)

	// Seed a host, then download a backup of that state.
	if w := do(t, h, "PUT", "/proxy-hosts/app", validProxyHost); w.Code != http.StatusOK {
		t.Fatalf("PUT host want 200 got %d", w.Code)
	}
	w := do(t, h, "GET", "/backup", "")
	if w.Code != http.StatusOK {
		t.Fatalf("backup want 200 got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("backup content-type = %q", ct)
	}
	archive := w.Body.Bytes()
	if len(archive) == 0 {
		t.Fatal("empty backup archive")
	}

	// Add a second host, then restore the earlier backup -> second host gone.
	second := `{"name":"two","domains":["two.example.com"],"upstream":{"scheme":"http","host":"10.0.0.6","port":80}}`
	if w := do(t, h, "PUT", "/proxy-hosts/two", second); w.Code != http.StatusOK {
		t.Fatalf("PUT second want 200 got %d", w.Code)
	}
	*changed = 0
	rr := httptest.NewRequest("POST", "/restore", bytes.NewReader(archive))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, rr)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore want 200 got %d: %s", rec.Code, rec.Body.String())
	}
	if *changed != 1 {
		t.Fatalf("restore OnChange want 1 got %d", *changed)
	}
	w = do(t, h, "GET", "/config", "")
	var cfg model.Config
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "app" {
		t.Fatalf("restore did not roll back to the backup: %+v", cfg.ProxyHosts)
	}

	// Revert to the first commit (before any host) using the repo history tail.
	w = do(t, h, "GET", "/history", "")
	var commits []store.Commit
	if err := json.Unmarshal(w.Body.Bytes(), &commits); err != nil {
		t.Fatal(err)
	}
	first := commits[len(commits)-1].Hash
	w = do(t, h, "POST", "/revert", `{"hash":"`+first+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("revert want 200 got %d: %s", w.Code, w.Body.String())
	}
	w = do(t, h, "GET", "/config", "")
	cfg = model.Config{}
	_ = json.Unmarshal(w.Body.Bytes(), &cfg)
	if len(cfg.ProxyHosts) != 0 {
		t.Fatalf("revert to initial commit should leave no hosts, got %d", len(cfg.ProxyHosts))
	}
}

func TestRevertRejectsBadHashViaAPI(t *testing.T) {
	h, _ := newHandler(t)
	w := do(t, h, "POST", "/revert", `{"hash":"../nope"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad hash want 400 got %d: %s", w.Code, w.Body.String())
	}
}

// TestPerObjectRevertViaAPI is the incident scenario at the API boundary:
// POST /proxy-hosts/{name}/revert restores only that host to a past commit and
// leaves a host created afterwards intact.
func TestPerObjectRevertViaAPI(t *testing.T) {
	h, changed := newHandler(t)

	w := do(t, h, "PUT", "/proxy-hosts/app", validProxyHost)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT v1 want 200 got %d", w.Code)
	}
	target := w.Header().Get("X-Config-Commit")
	if target == "" {
		t.Fatal("missing commit header on first save")
	}

	updated := `{"name":"app","domains":["app2.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":9090}}`
	if w := do(t, h, "PUT", "/proxy-hosts/app", updated); w.Code != http.StatusOK {
		t.Fatalf("PUT v2 want 200 got %d", w.Code)
	}
	second := `{"name":"two","domains":["two.example.com"],"upstream":{"scheme":"http","host":"10.0.0.6","port":80}}`
	if w := do(t, h, "PUT", "/proxy-hosts/two", second); w.Code != http.StatusOK {
		t.Fatalf("PUT second want 200 got %d", w.Code)
	}

	*changed = 0
	w = do(t, h, "POST", "/proxy-hosts/app/revert", `{"hash":"`+target+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("per-object revert want 200 got %d: %s", w.Code, w.Body.String())
	}
	if *changed != 1 {
		t.Fatalf("revert OnChange want 1 got %d", *changed)
	}

	w = do(t, h, "GET", "/config", "")
	var cfg model.Config
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.ProxyHosts) != 2 {
		t.Fatalf("scoped revert must keep the later host: want 2 got %d", len(cfg.ProxyHosts))
	}
	var app *model.ProxyHost
	for i := range cfg.ProxyHosts {
		if cfg.ProxyHosts[i].Name == "app" {
			app = &cfg.ProxyHosts[i]
		}
	}
	if app == nil || app.Upstream.Port != 8080 {
		t.Fatalf("app not reverted to v1 port: %+v", app)
	}
}

func TestPerObjectRevertRejectsBadHashViaAPI(t *testing.T) {
	h, _ := newHandler(t)
	w := do(t, h, "POST", "/proxy-hosts/app/revert", `{"hash":"../nope"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad hash want 400 got %d: %s", w.Code, w.Body.String())
	}
}

// TestSaveSurfacesApplyFailure proves a write that commits but cannot be applied
// to the running config (OnChange/reload error, e.g. a geo rule saved while no
// GeoIP database is loaded) returns 5xx, not a misleading 200.
func TestSaveSurfacesApplyFailure(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	h := api.New(api.Deps{Store: st, OnChange: func() error {
		return errors.New("no GeoIP database is loaded")
	}})

	w := do(t, h, "PUT", "/proxy-hosts/app", validProxyHost)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("apply failure want 500 got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "could not be applied") {
		t.Fatalf("response should explain the apply failure, got %s", w.Body.String())
	}
}

// TestCapabilitiesGeoIP verifies the read-only capability probe reflects the
// injected GeoDBLoaded predicate and shapes the JSON the UI builds against.
func TestCapabilitiesGeoIP(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}

	for _, tc := range []struct {
		name   string
		loaded bool
	}{
		{"db-loaded", true},
		{"db-missing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded := tc.loaded
			h := api.New(api.Deps{Store: st, GeoDBLoaded: func() bool { return loaded }})
			w := do(t, h, "GET", "/capabilities", "")
			if w.Code != http.StatusOK {
				t.Fatalf("GET /capabilities want 200 got %d", w.Code)
			}
			var got struct {
				GeoIP struct {
					DBLoaded bool `json:"dbLoaded"`
				} `json:"geoip"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v; body=%s", err, w.Body.String())
			}
			if got.GeoIP.DBLoaded != tc.loaded {
				t.Fatalf("geoip.dbLoaded want %v got %v", tc.loaded, got.GeoIP.DBLoaded)
			}
		})
	}
}

// TestCapabilitiesNilPredicate confirms a nil GeoDBLoaded reports not-loaded
// rather than panicking.
func TestCapabilitiesNilPredicate(t *testing.T) {
	h, _ := newHandler(t)
	w := do(t, h, "GET", "/capabilities", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"dbLoaded": false`) {
		t.Fatalf("nil predicate should report dbLoaded=false, got %s", w.Body.String())
	}
}

// TestStripResponseHeadersRoundTrip pins the API surface of the response-header
// strip list: both the settings default and a per-host addition survive a
// PUT/GET round trip unchanged, and an invalid header name is a 400 at config
// write rather than a silent no-op at runtime.
func TestStripResponseHeadersRoundTrip(t *testing.T) {
	h, _ := newHandler(t)

	settingsBody := `{"externalBaseURL":"https://proxy.example.com","adminAuth":{"localLoginEnabled":true},"stripResponseHeaders":["Server","X-Powered-By"]}`
	if w := do(t, h, "PUT", "/settings", settingsBody); w.Code != http.StatusOK {
		t.Fatalf("put settings want 200 got %d: %s", w.Code, w.Body.String())
	}
	w := do(t, h, "GET", "/settings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get settings want 200 got %d", w.Code)
	}
	var st model.Settings
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if len(st.StripResponseHeaders) != 2 || st.StripResponseHeaders[0] != "Server" || st.StripResponseHeaders[1] != "X-Powered-By" {
		t.Fatalf("settings.stripResponseHeaders = %v, want the configured list round-tripped", st.StripResponseHeaders)
	}

	hostBody := `{"name":"app","domains":["app2.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},"stripResponseHeaders":["x-aspnet-version"]}`
	if w := do(t, h, "PUT", "/proxy-hosts/app", hostBody); w.Code != http.StatusOK {
		t.Fatalf("put host want 200 got %d: %s", w.Code, w.Body.String())
	}
	w = do(t, h, "GET", "/proxy-hosts/app", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get host want 200 got %d", w.Code)
	}
	var got model.ProxyHost
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode host: %v", err)
	}
	if len(got.StripResponseHeaders) != 1 || got.StripResponseHeaders[0] != "x-aspnet-version" {
		t.Fatalf("host stripResponseHeaders = %v, want the configured list round-tripped verbatim", got.StripResponseHeaders)
	}

	// Invalid name -> 400, on both surfaces.
	bad := `{"externalBaseURL":"https://proxy.example.com","adminAuth":{"localLoginEnabled":true},"stripResponseHeaders":["X Powered By"]}`
	if w := do(t, h, "PUT", "/settings", bad); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid settings strip name want 400 got %d: %s", w.Code, w.Body.String())
	}
	badHost := `{"name":"app","domains":["app2.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080},"stripResponseHeaders":[""]}`
	if w := do(t, h, "PUT", "/proxy-hosts/app", badHost); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid host strip name want 400 got %d: %s", w.Code, w.Body.String())
	}
}
