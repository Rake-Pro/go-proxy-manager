package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/analytics"
	"github.com/Rake-Pro/go-proxy-manager/internal/api"
)

func TestAnalyticsUnwiredIs501(t *testing.T) {
	h := newRuntimeHandler(t, nil)
	w := do(t, h, "GET", "/analytics", "")
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("GET /analytics unwired = %d, want 501: %s", w.Code, w.Body.String())
	}
}

func TestAnalyticsDemoReport(t *testing.T) {
	now := time.Date(2026, 10, 26, 4, 0, 0, 0, time.UTC)
	h := newRuntimeHandler(t, func(d *api.Deps) {
		d.Analytics = func(rng, host string) (any, error) {
			return analytics.Demo([]string{"www", "api"}, now, analytics.Query{Range: rng, Host: host})
		}
	})
	w := do(t, h, "GET", "/analytics", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /analytics = %d: %s", w.Code, w.Body.String())
	}
	var rep analytics.Report
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.Demo || rep.Range != "24h" || len(rep.Series) != 48 || len(rep.Hosts) != 3 {
		t.Errorf("default report: demo=%v range=%s series=%d hosts=%d", rep.Demo, rep.Range, len(rep.Series), len(rep.Hosts))
	}

	w = do(t, h, "GET", "/analytics?range=1h&host=api", "")
	if w.Code != http.StatusOK {
		t.Fatalf("host view = %d: %s", w.Code, w.Body.String())
	}
	var hv analytics.Report // fresh: hosts is omitempty, a reused struct would keep the old rows
	if err := json.Unmarshal(w.Body.Bytes(), &hv); err != nil {
		t.Fatal(err)
	}
	if hv.Host != "api" || len(hv.Hosts) != 0 || len(hv.Series) != 60 {
		t.Errorf("host view: host=%s hosts=%d series=%d", hv.Host, len(hv.Hosts), len(hv.Series))
	}
	if !strings.Contains(w.Body.String(), "203.0.113.") || strings.Contains(w.Body.String(), "\"hosts\": [") {
		t.Errorf("host view body: want documentation-range clients and no hosts key: %s", w.Body.String()[:200])
	}

	for _, bad := range []string{"/analytics?range=2h", "/analytics?host=nope", "/analytics?host=../x", "/analytics?host=api;x"} {
		if w := do(t, h, "GET", bad, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, w.Code)
		}
	}
}
