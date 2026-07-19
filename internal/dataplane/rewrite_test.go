package dataplane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestRewriteHandler proves the exact-match path rewrite: a matching path is
// swapped to its target as seen by the next handler, a non-match passes through
// unchanged, and method/body/query are preserved across the rewrite.
func TestRewriteHandler(t *testing.T) {
	spec := model.RewriteMiddleware{ReplacePath: map[string]string{
		"/application/o/token": "/application/o/token/",
	}}

	var (
		gotPath   string
		gotRawQ   string
		gotMethod string
		gotBody   string
	)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRawQ = r.URL.RawQuery
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	})
	h := rewriteHandler(spec, next)

	cases := []struct {
		name         string
		method, url  string
		body         string
		wantPath     string
		wantRawQuery string
	}{
		{
			name:     "exact match rewritten",
			method:   "GET",
			url:      "http://c/application/o/token",
			wantPath: "/application/o/token/",
		},
		{
			name:     "non-match passes through",
			method:   "GET",
			url:      "http://c/application/o/authorize",
			wantPath: "/application/o/authorize",
		},
		{
			name:         "method and body preserved on rewrite",
			method:       "POST",
			url:          "http://c/application/o/token",
			body:         "grant_type=authorization_code&code=abc",
			wantPath:     "/application/o/token/",
			wantRawQuery: "",
		},
		{
			name:         "query preserved on rewrite",
			method:       "GET",
			url:          "http://c/application/o/token?state=xyz&foo=bar",
			wantPath:     "/application/o/token/",
			wantRawQuery: "state=xyz&foo=bar",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.url, body)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
			if gotRawQ != tc.wantRawQuery {
				t.Errorf("raw query = %q, want %q", gotRawQ, tc.wantRawQuery)
			}
			if gotMethod != tc.method {
				t.Errorf("method = %q, want %q", gotMethod, tc.method)
			}
			if gotBody != tc.body {
				t.Errorf("body = %q, want %q", gotBody, tc.body)
			}
		})
	}
}
