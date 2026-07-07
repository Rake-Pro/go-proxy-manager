package dataplane

import (
	"net/http/httptest"
	"testing"
)

// TestNormalizeRequestPathRejectsAmbiguous proves the M1 fix: a path that
// path.Clean leaves structurally ambiguous - a backslash or a surviving ';'
// matrix parameter - is rejected, so it can never dodge a path-scoped
// location's auth or a guard trigger by presenting a variant the matcher does
// not recognize but a backend re-collapses onto the protected path.
func TestNormalizeRequestPathRejectsAmbiguous(t *testing.T) {
	reject := []string{
		"/admin;x",
		"/admin;jsessionid=1",
		`/admin\..\x`,
		`/a\b`,
	}
	for _, p := range reject {
		r := httptest.NewRequest("GET", "http://h.example.com/", nil)
		r.URL.Path = p
		if normalizeRequestPath(r) {
			t.Errorf("path %q must be rejected", p)
		}
	}

	// Legitimate paths (including a '..' that eliminates a ';' segment) are
	// accepted and canonicalized to the dot-segment-free form.
	accept := map[string]string{
		"/admin":       "/admin",
		"/x/../admin":  "/admin",
		"/x;/../admin": "/admin", // the ';' segment is removed by '..', so no ambiguity remains
		"/a/b/":        "/a/b/",
		"/":            "/",
	}
	for in, want := range accept {
		r := httptest.NewRequest("GET", "http://h.example.com/", nil)
		r.URL.Path = in
		if !normalizeRequestPath(r) {
			t.Errorf("path %q must be accepted", in)
			continue
		}
		if r.URL.Path != want {
			t.Errorf("normalizeRequestPath(%q) = %q, want %q", in, r.URL.Path, want)
		}
	}
}
