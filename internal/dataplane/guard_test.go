package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func TestGuardNextcloudBreakGlass(t *testing.T) {
	// The exact cloud.example.com rule: POST or ?direct=1 to the login paths is
	// LAN-only.
	g := compileGuard(model.GuardMiddleware{
		Triggers: []model.GuardTrigger{
			{Paths: []string{"/login", "/index.php/login"}, Methods: []string{"POST"}},
			{Paths: []string{"/login", "/index.php/login"}, QueryEquals: map[string]string{"direct": "1"}},
		},
		AllowFrom: []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"},
	})
	h := guardHandler(g, peerIP, okHandler())

	serve := func(method, target, remote string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, target, nil)
		req.RemoteAddr = remote
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	cases := []struct {
		name           string
		method, target string
		remote         string
		want           int
	}{
		{"external POST /login denied", "POST", "http://c/login", "198.18.0.9:1", http.StatusForbidden},
		{"external ?direct=1 denied", "GET", "http://c/login?direct=1", "198.18.0.9:1", http.StatusForbidden},
		{"external plain GET /login allowed", "GET", "http://c/login", "198.18.0.9:1", http.StatusOK},
		{"external POST elsewhere allowed", "POST", "http://c/files", "198.18.0.9:1", http.StatusOK},
		{"index.php variant denied external", "POST", "http://c/index.php/login", "198.18.0.9:1", http.StatusForbidden},
		{"LAN POST /login allowed", "POST", "http://c/login", "192.0.2.5:1", http.StatusOK},
		{"LAN ?direct=1 allowed", "GET", "http://c/login?direct=1", "203.0.113.7:1", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serve(tc.method, tc.target, tc.remote); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGuardFailsClosedOnNilIP(t *testing.T) {
	g := compileGuard(model.GuardMiddleware{
		Triggers:  []model.GuardTrigger{{Paths: []string{"/login"}, Methods: []string{"POST"}}},
		AllowFrom: []string{"10.0.0.0/8"},
	})
	h := guardHandler(g, peerIP, okHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://c/login", nil)
	req.RemoteAddr = "garbage"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a matched request with an unresolvable IP must be denied, got %d", rec.Code)
	}
}

// A guard that matches on query parameters must refuse a raw query containing
// ';'. Go's url.Values does not split on ';', so "?direct=1" written as
// "?a=1;direct=1" does not fire the trigger - but RawQuery is forwarded to the
// upstream unchanged, and a backend that still honours the legacy separator acts
// on direct=1. Failing closed is what keeps gpm's view and the upstream's view
// of the query from diverging.
func TestGuardFailsClosedOnSemicolonQuery(t *testing.T) {
	g := compileGuard(model.GuardMiddleware{
		Triggers: []model.GuardTrigger{
			{Paths: []string{"/login"}, QueryEquals: map[string]string{"direct": "1"}},
		},
		AllowFrom: []string{"192.0.2.0/24"},
	})
	h := guardHandler(g, peerIP, okHandler())

	serve := func(target, remote string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", target, nil)
		req.RemoteAddr = remote
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	const external = "198.18.0.9:1"
	cases := []struct {
		name, target, remote string
		want                 int
	}{
		{"semicolon smuggling the guarded parameter", "http://c/login?a=1;direct=1", external, http.StatusBadRequest},
		{"semicolon anywhere in the query", "http://c/x?a=1;b=2", external, http.StatusBadRequest},
		{"semicolon refused for allowed clients too", "http://c/login?a=1;direct=1", "192.0.2.5:1", http.StatusBadRequest},
		{"plain guarded query still denied", "http://c/login?direct=1", external, http.StatusForbidden},
		{"plain guarded query still allowed from the LAN", "http://c/login?direct=1", "192.0.2.5:1", http.StatusOK},
		{"unrelated query untouched", "http://c/login?other=1", external, http.StatusOK},
		{"no query untouched", "http://c/login", external, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serve(tc.target, tc.remote); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// A guard with no query dimension has nothing to be confused by, so a ';' in the
// query is none of its business and must not turn working requests into 400s.
func TestGuardWithoutQueryTriggersIgnoresSemicolons(t *testing.T) {
	g := compileGuard(model.GuardMiddleware{
		Triggers:  []model.GuardTrigger{{Paths: []string{"/login"}, Methods: []string{"POST"}}},
		AllowFrom: []string{"192.0.2.0/24"},
	})
	h := guardHandler(g, peerIP, okHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://c/files?a=1;b=2", nil)
	req.RemoteAddr = "198.18.0.9:1"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}
