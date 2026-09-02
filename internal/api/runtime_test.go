package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/ha"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	"github.com/Rake-Pro/go-proxy-manager/internal/webhook"
)

func newRuntimeHandler(t *testing.T, mutate func(*api.Deps)) http.Handler {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	d := api.Deps{Store: st}
	if mutate != nil {
		mutate(&d)
	}
	return api.New(d)
}

func TestRuntimeEndpoint(t *testing.T) {
	accessLog := true
	geo := false
	h := newRuntimeHandler(t, func(d *api.Deps) {
		d.Role = ha.RoleFollower
		d.MetricsEnabled = true
		d.AccessLogEnabled = func() bool { return accessLog }
		d.GeoDBLoaded = func() bool { return geo }
		d.Runtime = api.RuntimeConfig{
			Version:              "v1.2.3",
			HTTPAddr:             ":80",
			HTTPSAddr:            ":443",
			AdminAddr:            ":8081",
			ConfigDir:            "/data/config",
			CertDir:              "/data/certs",
			SessionDB:            "/data/session.db",
			SecretFileRoots:      []string{"/run/secrets"},
			LocalAdminConfigured: true,
			PprofEnabled:         false,
		}
	})

	w := do(t, h, "GET", "/runtime", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /runtime = %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Version   string `json:"version"`
		HARole    string `json:"haRole"`
		Listeners struct {
			HTTP  string `json:"http"`
			HTTPS string `json:"https"`
			Admin string `json:"admin"`
		} `json:"listeners"`
		Paths struct {
			ConfigDir string `json:"configDir"`
			CertDir   string `json:"certDir"`
			SessionDB string `json:"sessionDB"`
		} `json:"paths"`
		MetricsEnabled       bool     `json:"metricsEnabled"`
		AccessLogEnabled     bool     `json:"accessLogEnabled"`
		GeoIPLoaded          bool     `json:"geoipLoaded"`
		SecretFileRoots      []string `json:"secretFileRoots"`
		LocalAdminConfigured bool     `json:"localAdminConfigured"`
		PprofEnabled         bool     `json:"pprofEnabled"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"version", got.Version, "v1.2.3"},
		{"haRole", got.HARole, "follower"},
		{"listeners.http", got.Listeners.HTTP, ":80"},
		{"listeners.https", got.Listeners.HTTPS, ":443"},
		{"listeners.admin", got.Listeners.Admin, ":8081"},
		{"paths.configDir", got.Paths.ConfigDir, "/data/config"},
		{"paths.certDir", got.Paths.CertDir, "/data/certs"},
		{"paths.sessionDB", got.Paths.SessionDB, "/data/session.db"},
		{"metricsEnabled", got.MetricsEnabled, true},
		{"accessLogEnabled", got.AccessLogEnabled, true},
		{"geoipLoaded", got.GeoIPLoaded, false},
		{"localAdminConfigured", got.LocalAdminConfigured, true},
		{"pprofEnabled", got.PprofEnabled, false},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if len(got.SecretFileRoots) != 1 || got.SecretFileRoots[0] != "/run/secrets" {
		t.Errorf("secretFileRoots = %v", got.SecretFileRoots)
	}

	// Live fields track the process, not the startup snapshot.
	accessLog, geo = false, true
	w = do(t, h, "GET", "/runtime", "")
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AccessLogEnabled || !got.GeoIPLoaded {
		t.Errorf("live fields did not follow the toggles: accessLog=%v geoip=%v", got.AccessLogEnabled, got.GeoIPLoaded)
	}

	// It must never report the admin username or the hash, whatever the field names.
	if body := w.Body.String(); strings.Contains(body, "$2a$") || strings.Contains(body, "localAdminUser") {
		t.Errorf("GET /runtime leaks a credential: %s", body)
	}
}

func TestRuntimeEndpointUnwiredDefaults(t *testing.T) {
	h := newRuntimeHandler(t, nil)
	w := do(t, h, "GET", "/runtime", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /runtime = %d", w.Code)
	}
	// secretFileRoots must be [] rather than null so the UI can render a list
	// without a nil guard.
	if !strings.Contains(w.Body.String(), `"secretFileRoots": []`) {
		t.Errorf("want an empty list for secretFileRoots, got: %s", w.Body.String())
	}
}

func TestCapabilitiesReportsAdminLogin(t *testing.T) {
	tests := []struct {
		name       string
		noAdmin    func() bool
		wantConfig bool
	}{
		{"unwired reports configured", nil, true},
		{"admin login present", func() bool { return false }, true},
		{"bootstrap failure state", func() bool { return true }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHandler(t, func(d *api.Deps) { d.NoAdminLogin = tc.noAdmin })
			w := do(t, h, "GET", "/capabilities", "")
			if w.Code != http.StatusOK {
				t.Fatalf("GET /capabilities = %d", w.Code)
			}
			var got struct {
				AdminLogin struct {
					Configured bool `json:"configured"`
				} `json:"adminLogin"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.AdminLogin.Configured != tc.wantConfig {
				t.Errorf("adminLogin.configured = %v, want %v", got.AdminLogin.Configured, tc.wantConfig)
			}
		})
	}
}

func TestWebhookStatusAndTestRoutes(t *testing.T) {
	var hits int
	recv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		var ev webhook.Event
		_ = json.NewDecoder(r.Body).Decode(&ev)
		if ev.Action != "test" {
			t.Errorf("received action %q, want %q", ev.Action, "test")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer recv.Close()

	dispatcher := webhook.New(func() []model.WebhookConfig {
		return []model.WebhookConfig{{Name: "ci", URL: recv.URL}}
	})
	h := newRuntimeHandler(t, func(d *api.Deps) {
		d.WebhookStatus = func() any { return dispatcher.Status() }
		d.WebhookTest = func(ctx context.Context, name string) (any, error) {
			return dispatcher.Test(ctx, name)
		}
	})

	// Before any delivery: listed, never fired.
	w := do(t, h, "GET", "/webhooks/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /webhooks/status = %d", w.Code)
	}
	var status []webhook.Delivery
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(status) != 1 || status[0].Name != "ci" || status[0].LastAttempt != "" || status[0].OK {
		t.Fatalf("status before any delivery = %+v", status)
	}

	// Test send.
	w = do(t, h, "POST", "/webhooks/ci/test", "")
	if w.Code != http.StatusOK {
		t.Fatalf("POST test = %d: %s", w.Code, w.Body.String())
	}
	var res webhook.TestResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !res.OK || res.Status != http.StatusNoContent || res.Error != "" {
		t.Fatalf("test result = %+v", res)
	}
	if hits != 1 {
		t.Fatalf("receiver hits = %d, want 1", hits)
	}

	// The attempt is now visible in the status listing.
	w = do(t, h, "GET", "/webhooks/status", "")
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(status) != 1 || !status[0].OK || status[0].LastAttempt == "" || status[0].Status != http.StatusNoContent {
		t.Fatalf("status after the test = %+v", status)
	}

	// An unknown target is a 404 with a human noun, not a Go type name.
	w = do(t, h, "POST", "/webhooks/nope/test", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown target = %d, want 404", w.Code)
	}
	var errBody struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if errBody.Error != `webhook "nope" not found` {
		t.Errorf("unknown target error = %q", errBody.Error)
	}
}

func TestWebhookTestReportsReceiverFailureAsOK200(t *testing.T) {
	recv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer recv.Close()

	dispatcher := webhook.New(func() []model.WebhookConfig {
		// Disabled on purpose: a test send must still run, so a receiver can be
		// proved before the target is turned on.
		return []model.WebhookConfig{{Name: "ci", URL: recv.URL, Disabled: true}}
	})
	h := newRuntimeHandler(t, func(d *api.Deps) {
		d.WebhookTest = func(ctx context.Context, name string) (any, error) {
			return dispatcher.Test(ctx, name)
		}
	})

	w := do(t, h, "POST", "/webhooks/ci/test", "")
	if w.Code != http.StatusOK {
		t.Fatalf("POST test = %d, want 200 (the test RAN; the receiver failed)", w.Code)
	}
	var res webhook.TestResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.OK || res.Status != http.StatusInternalServerError || res.Error == "" {
		t.Fatalf("test result = %+v, want ok:false with an error", res)
	}
}

func TestWebhookRoutesUnwired(t *testing.T) {
	h := newRuntimeHandler(t, nil)

	w := do(t, h, "GET", "/webhooks/status", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("unwired status = %d %q, want 200 []", w.Code, w.Body.String())
	}
	w = do(t, h, "POST", "/webhooks/ci/test", "")
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("unwired test = %d, want 501", w.Code)
	}
	// An invalid name is rejected before anything is dispatched.
	w = do(t, h, "POST", "/webhooks/not%20a%20name/test", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid name = %d, want 400", w.Code)
	}
}

// TestWebhookTestPropagatesUnknownTargetSentinel guards the errors.Is contract
// the handler relies on to answer 404 rather than 500.
func TestWebhookTestPropagatesUnknownTargetSentinel(t *testing.T) {
	d := webhook.New(func() []model.WebhookConfig { return nil })
	if _, err := d.Test(context.Background(), "ghost"); !errors.Is(err, webhook.ErrUnknownTarget) {
		t.Fatalf("Test() error = %v, want ErrUnknownTarget", err)
	}
}
