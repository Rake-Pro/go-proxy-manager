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
		{"external POST /login denied", "POST", "http://c/login", "203.0.113.9:1", http.StatusForbidden},
		{"external ?direct=1 denied", "GET", "http://c/login?direct=1", "203.0.113.9:1", http.StatusForbidden},
		{"external plain GET /login allowed", "GET", "http://c/login", "203.0.113.9:1", http.StatusOK},
		{"external POST elsewhere allowed", "POST", "http://c/files", "203.0.113.9:1", http.StatusOK},
		{"index.php variant denied external", "POST", "http://c/index.php/login", "203.0.113.9:1", http.StatusForbidden},
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
