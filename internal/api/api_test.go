package api_test

import (
	"context"
	"encoding/json"
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
	h := api.New(api.Deps{Store: st, OnChange: func() { changed++ }})
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
