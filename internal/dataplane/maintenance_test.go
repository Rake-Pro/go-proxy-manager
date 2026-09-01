package dataplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// withMaintenance installs m as the settings-level maintenance switch for the
// duration of the test and restores the previous value on cleanup, so tests
// never leak state into each other through the package-level handle (same
// pattern as withGlobalErrorPages).
func withMaintenance(t *testing.T, m model.MaintenanceSettings) {
	t.Helper()
	prev := globalMaintenance.Load()
	SetMaintenance(m)
	t.Cleanup(func() { globalMaintenance.Store(prev) })
}

// maintenanceRouter builds a two-host router: "down.example.com" carries
// hostFlag as its own maintenance flag, "up.example.com" never does. Both point
// at a live upstream that echoes "upstream", so "stops proxying to the
// upstream" is asserted against a backend that would otherwise answer.
func maintenanceRouter(t *testing.T, hostFlag bool) *router {
	t.Helper()
	upstream := upstreamFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("upstream"))
	}))
	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{
			ObjectMeta:  model.ObjectMeta{Name: "down"},
			Domains:     []string{"down.example.com"},
			Upstream:    upstream,
			Maintenance: hostFlag,
		},
		{
			ObjectMeta: model.ObjectMeta{Name: "up"},
			Domains:    []string{"up.example.com"},
			Upstream:   upstream,
		},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	return rt
}

func TestMaintenanceHostServes503NotUpstream(t *testing.T) {
	withMaintenance(t, model.MaintenanceSettings{})
	withGlobalErrorPages(t, nil)
	rt := maintenanceRouter(t, true)

	rec := serveOn(rt, true, "GET", "https://down.example.com/anything", "down.example.com")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "upstream") {
		t.Fatalf("the upstream was proxied to during maintenance: %q", body)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "300" {
		t.Fatalf("Retry-After = %q, want the 300s default", ra)
	}
	// Every maintenance response must carry a Content-Type and a body: a
	// bodyless, type-less error is what broke a real API client in production.
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatal("maintenance response has no Content-Type")
	}
	if rec.Body.Len() == 0 {
		t.Fatal("maintenance response has an empty body")
	}

	// A host that is not in maintenance still proxies.
	live := serveOn(rt, true, "GET", "https://up.example.com/", "up.example.com")
	if live.Code != http.StatusOK || live.Body.String() != "upstream" {
		t.Fatalf("unflagged host: got %d %q, want 200 upstream", live.Code, live.Body.String())
	}
}

func TestMaintenanceGlobalOverridesPerHostOff(t *testing.T) {
	withGlobalErrorPages(t, nil)
	rt := maintenanceRouter(t, false) // neither host sets its own flag

	// Off: both hosts proxy.
	withMaintenance(t, model.MaintenanceSettings{})
	if rec := serveOn(rt, true, "GET", "https://up.example.com/", "up.example.com"); rec.Code != http.StatusOK {
		t.Fatalf("global off: status = %d, want 200", rec.Code)
	}

	// Global on wins over a per-host false, and applies to EVERY host - live,
	// against the router that was already built, with no rebuild.
	SetMaintenance(model.MaintenanceSettings{Enabled: true, RetryAfterSeconds: 60})
	for _, host := range []string{"up.example.com", "down.example.com"} {
		rec := serveOn(rt, true, "GET", "https://"+host+"/", host)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("global on, %s: status = %d, want 503", host, rec.Code)
		}
		if ra := rec.Header().Get("Retry-After"); ra != "60" {
			t.Fatalf("global on, %s: Retry-After = %q, want 60", host, ra)
		}
	}

	// Toggling back off resumes proxying on the same router, again with no rebuild.
	SetMaintenance(model.MaintenanceSettings{})
	rec := serveOn(rt, true, "GET", "https://up.example.com/", "up.example.com")
	if rec.Code != http.StatusOK || rec.Body.String() != "upstream" {
		t.Fatalf("global toggled off: got %d %q, want 200 upstream", rec.Code, rec.Body.String())
	}
}

func TestMaintenanceOnPlaintextListener(t *testing.T) {
	withMaintenance(t, model.MaintenanceSettings{})
	withGlobalErrorPages(t, nil)
	rt := maintenanceRouter(t, true)

	rec := serveOn(rt, false, "GET", "http://down.example.com/", "down.example.com")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 on the plaintext listener too", rec.Code)
	}
}

func TestMaintenanceUsesConfiguredErrorPage(t *testing.T) {
	withMaintenance(t, model.MaintenanceSettings{})
	global, err := compileErrorPages(model.ErrorPagesConfig{
		Inline: map[string]string{"503": "<h1>back at 14:00 - {{.Host}}</h1>"},
	}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	withGlobalErrorPages(t, global)
	rt := maintenanceRouter(t, true)

	rec := serveOn(rt, true, "GET", "https://down.example.com/", "down.example.com")
	if got := rec.Body.String(); got != "<h1>back at 14:00 - down</h1>" {
		t.Fatalf("body = %q, want the configured 503 error page", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "300" {
		t.Fatalf("Retry-After = %q, want it set on a custom page too", ra)
	}
}

func TestMaintenanceBodyNegotiation(t *testing.T) {
	tests := []struct {
		name       string
		accept     string
		wantType   string
		wantSubstr string
	}{
		{"json client", "application/json", "application/json; charset=utf-8", `"error":"maintenance"`},
		{"browser", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8", "text/html; charset=utf-8", "<h1>Down for maintenance</h1>"},
		{"curl wildcard", "*/*", "text/plain; charset=utf-8", "Down for maintenance"},
		{"no accept header", "", "text/plain; charset=utf-8", "Down for maintenance"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withMaintenance(t, model.MaintenanceSettings{})
			withGlobalErrorPages(t, nil)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "https://down.example.com/", nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			serveMaintenance(rec, req, nil, "down")
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != tc.wantType {
				t.Fatalf("Content-Type = %q, want %q", ct, tc.wantType)
			}
			if !strings.Contains(rec.Body.String(), tc.wantSubstr) {
				t.Fatalf("body = %q, want it to contain %q", rec.Body.String(), tc.wantSubstr)
			}
		})
	}
}

func TestMaintenanceEffectiveRetryAfter(t *testing.T) {
	if got := (model.MaintenanceSettings{}).EffectiveRetryAfter(); got != model.DefaultMaintenanceRetryAfter {
		t.Fatalf("unset retryAfterSeconds = %d, want the default %d", got, model.DefaultMaintenanceRetryAfter)
	}
	if got := (model.MaintenanceSettings{RetryAfterSeconds: 45}).EffectiveRetryAfter(); got != 45 {
		t.Fatalf("configured retryAfterSeconds = %d, want 45", got)
	}
}

// A router built with no SetMaintenance call at all (an embedder, or any of the
// other tests in this package) must behave exactly as it did before the feature.
func TestMaintenanceUninstalledIsOff(t *testing.T) {
	prev := globalMaintenance.Load()
	globalMaintenance.Store(nil)
	t.Cleanup(func() { globalMaintenance.Store(prev) })
	if maintenanceActive(false) {
		t.Fatal("maintenance must be off when SetMaintenance has never run")
	}
	if !maintenanceActive(true) {
		t.Fatal("a host's own flag must still apply with no settings installed")
	}
}
