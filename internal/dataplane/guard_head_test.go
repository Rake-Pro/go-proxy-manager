package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// A trigger that lists GET also fires on HEAD; nothing else about method
// matching changes (no other implied methods, no method-override parameters).
func TestGuardGetTriggerCoversHead(t *testing.T) {
	cases := []struct {
		name    string
		methods []string
		method  string
		target  string
		want    int
	}{
		{"GET trigger, GET", []string{"GET"}, "GET", "http://c/admin", http.StatusForbidden},
		{"GET trigger, HEAD", []string{"GET"}, "HEAD", "http://c/admin", http.StatusForbidden},
		{"lower-case get trigger, HEAD", []string{"get"}, "HEAD", "http://c/admin", http.StatusForbidden},
		{"GET trigger, lower-case head", []string{"GET"}, "head", "http://c/admin", http.StatusForbidden},
		{"GET trigger, POST", []string{"GET"}, "POST", "http://c/admin", http.StatusOK},
		{"GET trigger, OPTIONS", []string{"GET"}, "OPTIONS", "http://c/admin", http.StatusOK},
		{"HEAD trigger, GET", []string{"HEAD"}, "GET", "http://c/admin", http.StatusOK},
		{"POST trigger, HEAD", []string{"POST"}, "HEAD", "http://c/admin", http.StatusOK},
		{"GET trigger, POST with _method=GET", []string{"GET"}, "POST", "http://c/admin?_method=GET", http.StatusOK},
		{"GET trigger, HEAD other path", []string{"GET"}, "HEAD", "http://c/other", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := compileGuard(model.GuardMiddleware{
				Triggers: []model.GuardTrigger{{Paths: []string{"/admin"}, Methods: tc.methods}},
			})
			h := guardHandler(g, peerIP, "", nil, okHandler())
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.target, nil)
			req.RemoteAddr = "198.18.0.9:1"
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
