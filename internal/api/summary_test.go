package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
)

const validRedirectHost = `{"name":"old","domains":["old.example.com"],"targetDomain":"new.example.com"}`

// disabledMaintenanceProxyHost is a second proxy host, distinct from
// validProxyHost, with both operator-owned runtime flags set so the summary's
// disabled/maintenance counts have something to count.
const disabledMaintenanceProxyHost = `{"name":"paused","domains":["paused.example.com"],"upstream":{"scheme":"http","host":"10.0.0.6","port":8080},"disabled":true,"maintenance":true}`

// summaryResponse mirrors configSummaryResponse's JSON shape for decoding in
// tests, without importing the unexported type.
type summaryResponse struct {
	Counts      map[string]int `json:"counts"`
	Disabled    map[string]int `json:"disabled"`
	Maintenance map[string]int `json:"maintenance"`
	Head        string         `json:"head"`
}

func TestConfigSummary(t *testing.T) {
	h, _ := newHandler(t)

	for _, tc := range []struct{ method, path, body string }{
		{"PUT", "/proxy-hosts/app", validProxyHost},
		{"PUT", "/proxy-hosts/paused", disabledMaintenanceProxyHost},
		{"PUT", "/redirect-hosts/old", validRedirectHost},
	} {
		if w := do(t, h, tc.method, tc.path, tc.body); w.Code != http.StatusOK {
			t.Fatalf("seed %s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	w := do(t, h, "GET", "/config/summary", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /config/summary = %d, want 200: %s", w.Code, w.Body.String())
	}

	var got summaryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantCounts := map[string]int{
		"proxy-hosts":        2,
		"redirect-hosts":     1,
		"stream-hosts":       0,
		"parked-hosts":       0,
		"certificates":       0,
		"client-cas":         0,
		"identity-providers": 0,
		"access-lists":       0,
		"middlewares":        0,
		"upstream-groups":    0,
		"dns-providers":      0,
		"api-tokens":         0,
	}
	for kind, want := range wantCounts {
		if got.Counts[kind] != want {
			t.Errorf("counts[%q] = %d, want %d", kind, got.Counts[kind], want)
		}
	}
	if len(got.Counts) != len(wantCounts) {
		t.Errorf("counts has %d keys, want %d: %v", len(got.Counts), len(wantCounts), got.Counts)
	}

	if got.Disabled["proxy-hosts"] != 1 {
		t.Errorf("disabled[proxy-hosts] = %d, want 1", got.Disabled["proxy-hosts"])
	}
	if got.Maintenance["proxy-hosts"] != 1 {
		t.Errorf("maintenance[proxy-hosts] = %d, want 1", got.Maintenance["proxy-hosts"])
	}
	if got.Head == "" {
		t.Error("head is empty, want the config repo's current commit")
	}
}

// TestConfigSummaryOmitsAPITokenCount mirrors
// TestUserRoleConfigOmitsAPITokens: a caller without api-tokens:read must not
// learn how many tokens exist by reading the count instead of the rows.
func TestConfigSummaryOmitsAPITokenCount(t *testing.T) {
	st := newRoleStore(t)
	admin := roleHandlerOn(t, st, auth.RoleAdmin)
	if w := do(t, admin, "PUT", "/api-tokens/ci", validToken); w.Code != http.StatusOK {
		t.Fatalf("seed token: %d %s", w.Code, w.Body.String())
	}

	var adminSummary summaryResponse
	if err := json.Unmarshal(do(t, admin, "GET", "/config/summary", "").Body.Bytes(), &adminSummary); err != nil {
		t.Fatalf("decode admin response: %v", err)
	}
	if adminSummary.Counts["api-tokens"] != 1 {
		t.Fatalf("admin summary api-tokens = %d, want 1 (so the rest of this test proves something)", adminSummary.Counts["api-tokens"])
	}

	user := roleHandlerOn(t, st, auth.RoleUser)
	w := do(t, user, "GET", "/config/summary", "")
	if w.Code != http.StatusOK {
		t.Fatalf("user GET /config/summary = %d, want 200", w.Code)
	}
	var userSummary summaryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &userSummary); err != nil {
		t.Fatalf("decode user response: %v", err)
	}
	if userSummary.Counts["api-tokens"] != 0 {
		t.Fatalf("user summary api-tokens = %d, want 0", userSummary.Counts["api-tokens"])
	}
}
