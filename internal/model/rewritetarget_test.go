package model

import (
	"strings"
	"testing"
)

// TestRewriteTargetRejectsDotSegments is the regression test for the confirmed
// escape: a rewrite target carrying a ".." segment composes with an upstream
// base path into "/base/../admin/...", which a backend that re-collapses dot
// segments serves as "/admin". Upstream.Path is validated against exactly this,
// so the rewrite side has to be too - and the rewrite scope is narrower than the
// proxy-host scope, so it is a privilege boundary as well.
func TestRewriteTargetRejectsDotSegments(t *testing.T) {
	tests := []struct {
		name    string
		rw      RewriteMiddleware
		wantErr string
	}{
		{
			name:    "prefix rule target with a parent segment",
			rw:      RewriteMiddleware{PrefixRules: []RewriteRule{{From: "/public", To: "/../admin"}}},
			wantErr: `must not contain "." or ".." segments`,
		},
		{
			name:    "prefix rule target with an interior parent segment",
			rw:      RewriteMiddleware{PrefixRules: []RewriteRule{{From: "/public", To: "/base/../admin"}}},
			wantErr: `must not contain "." or ".." segments`,
		},
		{
			name:    "replacePath target with a parent segment",
			rw:      RewriteMiddleware{ReplacePath: map[string]string{"/public": "/../admin"}},
			wantErr: `must not contain "." or ".." segments`,
		},
		{
			name:    "replacePath target with a current-directory segment",
			rw:      RewriteMiddleware{ReplacePath: map[string]string{"/public": "/./admin"}},
			wantErr: `must not contain "." or ".." segments`,
		},
		{
			name:    "regex replacement template with a parent segment",
			rw:      RewriteMiddleware{RegexRules: []RewriteRule{{From: "^/public/(.*)$", To: "/../admin/$1"}}},
			wantErr: `must not contain "." or ".." segments`,
		},
		{
			name:    "target with a backslash",
			rw:      RewriteMiddleware{PrefixRules: []RewriteRule{{From: "/public", To: `/admin\..\x`}}},
			wantErr: "must not contain",
		},
		{
			name:    "target with a matrix parameter",
			rw:      RewriteMiddleware{PrefixRules: []RewriteRule{{From: "/public", To: "/admin;x"}}},
			wantErr: "must not contain",
		},
		{
			name:    "target with a query string",
			rw:      RewriteMiddleware{PrefixRules: []RewriteRule{{From: "/public", To: "/admin?x=1"}}},
			wantErr: "must not contain a query string",
		},
		{
			name: "ordinary targets still validate",
			rw: RewriteMiddleware{
				ReplacePath: map[string]string{"/application/o/token": "/application/o/token/"},
				PrefixRules: []RewriteRule{{From: "/public", To: "/static"}},
				RegexRules:  []RewriteRule{{From: "^/v1/(.*)$", To: "/api/$1"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mw := Middleware{ObjectMeta: ObjectMeta{Name: "esc"}, Type: MWTypeRewrite, Rewrite: &tc.rw}
			err := mw.Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Validate() = %v, want nil", err)
			case tc.wantErr == "":
			case err == nil:
				t.Fatalf("Validate() = nil, want an error containing %q", tc.wantErr)
			case !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("Validate() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateOutboundBaseURL covers the acme-dns / REST DNS base URL guard:
// gpm POSTs provider credentials there, so a loopback or link-local literal is
// refused unless the operator explicitly opted in.
func TestValidateOutboundBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		allowLocal bool
		wantErr    bool
	}{
		{name: "https host", raw: "https://acme-dns.example.com", wantErr: false},
		{name: "http host", raw: "http://acme-dns.example.com:8080", wantErr: false},
		{name: "no scheme", raw: "acme-dns.example.com", wantErr: true},
		{name: "ftp scheme", raw: "ftp://acme-dns.example.com", wantErr: true},
		{name: "link-local metadata address", raw: "http://169.254.169.254/latest/meta-data", wantErr: true},
		{name: "link-local v6", raw: "http://[fe80::1]/", wantErr: true},
		{name: "loopback refused by default", raw: "http://127.0.0.1:8080", wantErr: true},
		{name: "loopback allowed on request", raw: "http://127.0.0.1:8080", allowLocal: true, wantErr: false},
		{name: "unspecified address", raw: "http://0.0.0.0:8080", wantErr: true},
		{name: "routable literal", raw: "https://192.0.2.10", wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOutboundBaseURL("baseURL", tc.raw, tc.allowLocal)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateOutboundBaseURL(%q, allowLocal=%v) = %v, wantErr=%v", tc.raw, tc.allowLocal, err, tc.wantErr)
			}
		})
	}
}
