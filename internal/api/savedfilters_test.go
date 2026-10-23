package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// The saved-filters routes read and replace only that section of settings:
// the rest of the settings object survives a write, the list comes back as
// [] rather than null when empty, and a malformed entry is refused.
func TestSavedFilters(t *testing.T) {
	h, _ := newHandler(t)

	if w := do(t, h, "GET", "/settings/saved-filters", ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty list: %d %q", w.Code, w.Body.String())
	}

	// Something unrelated in settings that the saved-filters write must keep.
	if w := do(t, h, "PUT", "/settings", `{"appName":"Edge","externalBaseURL":"https://gpm.example.com","adminAuth":{"localLoginEnabled":true}}`); w.Code != http.StatusOK {
		t.Fatalf("seed settings: %d %s", w.Code, w.Body.String())
	}

	good := `[{"name":"Outside, errors","list":"logs","query":"-client:@home-only -status:2xx"},{"name":"LAN","list":"hosts","query":"tag:lan"}]`
	if w := do(t, h, "PUT", "/settings/saved-filters", good); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	w := do(t, h, "GET", "/settings/saved-filters", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"Outside, errors"`) || !strings.Contains(w.Body.String(), `"tag:lan"`) {
		t.Fatalf("read back: %d %s", w.Code, w.Body.String())
	}
	w = do(t, h, "GET", "/settings", "")
	if !strings.Contains(w.Body.String(), `"appName": "Edge"`) || !strings.Contains(w.Body.String(), `"savedFilters"`) {
		t.Fatalf("settings after save lost a section: %s", w.Body.String())
	}

	bad := []string{
		`[{"name":"","list":"logs","query":"x"}]`,
		`[{"name":"a","list":"Logs","query":"x"}]`,
		`[{"name":"a","list":"logs","query":""}]`,
		`[{"name":"a","list":"logs","query":"x"},{"name":"A","list":"logs","query":"y"}]`,
		`{"name":"a"}`,
	}
	for _, b := range bad {
		if w := do(t, h, "PUT", "/settings/saved-filters", b); w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d %s", b, w.Code, w.Body.String())
		}
	}

	if w := do(t, h, "PUT", "/settings/saved-filters", `[]`); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("clear: %d %q", w.Code, w.Body.String())
	}
}
