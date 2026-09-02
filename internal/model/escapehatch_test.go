package model

import (
	"strings"
	"testing"
)

// upstreamWith returns a valid upstream carrying the given base path and Host
// header override, so each case varies only the field under test.
func upstreamWith(path, hostHeader string) Upstream {
	return Upstream{Scheme: "http", Host: "backend.example.com", Port: 8080, Path: path, HostHeader: hostHeader}
}

func TestUpstreamPathValidation(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "unset is allowed"},
		{name: "absolute path", path: "/api"},
		{name: "nested path", path: "/api/v1"},
		{name: "trailing slash", path: "/api/"},
		{name: "relative path", path: "api", wantErr: "must be absolute"},
		{name: "dot dot segment", path: "/api/../etc", wantErr: `".."`},
		{name: "trailing dot dot", path: "/api/..", wantErr: `".."`},
		{name: "single dot segment", path: "/api/./v1", wantErr: `".."`},
		{name: "query string", path: "/api?x=1", wantErr: "query string"},
		{name: "fragment", path: "/api#frag", wantErr: "query string"},
		{name: "backslash", path: `/api\v1`, wantErr: `\`},
		{name: "matrix parameter", path: "/api;x=1", wantErr: ";"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := upstreamWith(tc.path, "").validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("validate(%q) = %v, want nil", tc.path, err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("validate(%q) = nil, want an error containing %q", tc.path, tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("validate(%q) = %v, want an error containing %q", tc.path, err, tc.wantErr)
			}
		})
	}
}

func TestUpstreamHostHeaderValidation(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "unset keeps the client Host"},
		{name: "upstream sentinel", value: UpstreamHostHeaderUpstream},
		{name: "hostname", value: "backend.example.com"},
		{name: "single label", value: "backend"},
		{name: "hostname with port", value: "backend.example.com:8443"},
		{name: "leading dash label", value: "-bad.example.com", wantErr: true},
		{name: "with a scheme", value: "http://backend.example.com", wantErr: true},
		{name: "with a path", value: "backend.example.com/x", wantErr: true},
		{name: "with a space", value: "backend example.com", wantErr: true},
		{name: "with a header injection attempt", value: "backend.example.com\r\nX-Evil: 1", wantErr: true},
		{name: "too long", value: strings.Repeat("a", 254), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := upstreamWith("", tc.value).validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("validate(hostHeader=%q) = %v, wantErr %v", tc.value, err, tc.wantErr)
			}
		})
	}
}

func TestRewriteMiddlewareValidation(t *testing.T) {
	tests := []struct {
		name    string
		spec    RewriteMiddleware
		wantErr string
	}{
		{
			name:    "no rules at all",
			spec:    RewriteMiddleware{},
			wantErr: "at least one replacePath, prefixRules or regexRules",
		},
		{
			name: "exact rules only",
			spec: RewriteMiddleware{ReplacePath: map[string]string{"/a": "/b"}},
		},
		{
			name: "prefix rules only",
			spec: RewriteMiddleware{PrefixRules: []RewriteRule{{From: "/a", To: "/b"}}},
		},
		{
			name: "regex rules only",
			spec: RewriteMiddleware{RegexRules: []RewriteRule{{From: `/user/([0-9]+)`, To: "/u/$1"}}},
		},
		{
			name:    "prefix from must be absolute",
			spec:    RewriteMiddleware{PrefixRules: []RewriteRule{{From: "a", To: "/b"}}},
			wantErr: "prefixRules[0].from",
		},
		{
			name:    "prefix to must be absolute",
			spec:    RewriteMiddleware{PrefixRules: []RewriteRule{{From: "/a", To: "b"}}},
			wantErr: "prefixRules[0].to",
		},
		{
			name:    "prefix no-op is refused",
			spec:    RewriteMiddleware{PrefixRules: []RewriteRule{{From: "/a", To: "/a"}}},
			wantErr: "prefixRules[0] rewrites",
		},
		{
			name: "the failing rule index is reported",
			spec: RewriteMiddleware{RegexRules: []RewriteRule{
				{From: `/ok`, To: "/fine"},
				{From: `/user/(`, To: "/u"},
			}},
			wantErr: "regexRules[1].from",
		},
		{
			name:    "regex compile failure is surfaced",
			spec:    RewriteMiddleware{RegexRules: []RewriteRule{{From: `/a(?!b)`, To: "/c"}}},
			wantErr: "not a valid regular expression",
		},
		{
			name:    "regex to must be absolute",
			spec:    RewriteMiddleware{RegexRules: []RewriteRule{{From: `/a`, To: "b"}}},
			wantErr: "regexRules[0].to",
		},
		{
			name:    "regex pattern length is capped",
			spec:    RewriteMiddleware{RegexRules: []RewriteRule{{From: "/" + strings.Repeat("a", MaxRewritePatternLen), To: "/b"}}},
			wantErr: "at most 256",
		},
		{
			name:    "prefix rule count is capped",
			spec:    RewriteMiddleware{PrefixRules: manyPrefixRules(MaxRewriteRules + 1)},
			wantErr: "at most 32",
		},
		{
			name: "prefix rule count at the cap is allowed",
			spec: RewriteMiddleware{PrefixRules: manyPrefixRules(MaxRewriteRules)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mw := Middleware{ObjectMeta: ObjectMeta{Name: "rw"}, Type: MWTypeRewrite, Rewrite: &tc.spec}
			err := mw.Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Validate() = %v, want nil", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("Validate() = nil, want an error containing %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("Validate() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func manyPrefixRules(n int) []RewriteRule {
	out := make([]RewriteRule, n)
	for i := range out {
		out[i] = RewriteRule{From: "/from" + strings.Repeat("x", i), To: "/to"}
	}
	return out
}

func TestAnchorRewritePattern(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: "/a", want: "^/a"},
		{in: "^/a", want: "^/a"},
		{in: `/user/([0-9]+)`, want: `^/user/([0-9]+)`},
	}
	for _, tc := range tests {
		if got := AnchorRewritePattern(tc.in); got != tc.want {
			t.Errorf("AnchorRewritePattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestLocationStripPrefixRoundTrips proves the new location and upstream fields
// pass whole-config validation, so an operator's YAML with them loads.
func TestLocationStripPrefixRoundTrips(t *testing.T) {
	cfg := Config{ProxyHosts: []ProxyHost{{
		ObjectMeta: ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   upstreamWith("/api", UpstreamHostHeaderUpstream),
		Locations: []Location{{
			Path:        "/app",
			StripPrefix: true,
			Upstream:    ptrUpstream(upstreamWith("/v1", "backend.example.com")),
		}},
	}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	bad := cfg
	bad.ProxyHosts = []ProxyHost{cfg.ProxyHosts[0]}
	bad.ProxyHosts[0].Locations = []Location{{
		Path:     "/app",
		Upstream: ptrUpstream(upstreamWith("/v1/../etc", "")),
	}}
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() = nil for a location upstream path with a dot-dot segment, want an error")
	}
}

func ptrUpstream(u Upstream) *Upstream { return &u }
