package dataplane

import (
	"net/http"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// scopedSet is a representative settings-level set exercising all three scopes:
// an all-scope header (both sides), a generated-only header (gpm's own responses
// only), and a proxied-only header (upstream responses only).
var scopedSet = map[string]model.SecurityHeaderValue{
	"X-Content-Type-Options":  {Value: "nosniff", Scope: model.SecurityScopeAll},
	"Content-Security-Policy": {Value: "frame-ancestors 'none'", Scope: model.SecurityScopeGenerated},
	"X-Proxied-Only":          {Value: "1", Scope: model.SecurityScopeProxied},
}

// TestSecurityHeadersScopeSelectsSubset is the core of the feature: a
// generated-only header lands on responses gpm writes but NEVER on a proxied
// upstream response, a proxied-only header is the exact inverse, and an all-scope
// header lands on both. Driven end to end through the router.
func TestSecurityHeadersScopeSelectsSubset(t *testing.T) {
	withGlobalScopedSecurityHeaders(t, scopedSet)
	withGlobalErrorPages(t, nil)

	// A live backend that sets none of these, so set-if-absent adds whatever the
	// scope selects.
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example"},
		Upstream:   up,
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	t.Run("proxied 200 gets all + proxied-only, not generated-only", func(t *testing.T) {
		rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
		if rec.Code != http.StatusOK || rec.Body.String() != "hi" {
			t.Fatalf("proxied response not passed through: %d %q", rec.Code, rec.Body.String())
		}
		assertHasAll(t, rec.Header(), map[string]string{
			"X-Content-Type-Options": "nosniff", // all
			"X-Proxied-Only":         "1",       // proxied-only
		})
		assertHasNone(t, rec.Header(), "Content-Security-Policy") // generated-only must NOT apply
	})

	t.Run("gpm-generated 400 on the proxy host gets all + generated-only, not proxied-only", func(t *testing.T) {
		// A bad path is rejected by gpm before the upstream is ever contacted, so it
		// is a gpm-generated response on this proxy host's own (merged) header set.
		rec := serveOn(rt, true, "GET", "https://app.example/a;b", "app.example")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want the 400 path rejection", rec.Code)
		}
		assertHasAll(t, rec.Header(), map[string]string{
			"X-Content-Type-Options":  "nosniff",                // all
			"Content-Security-Policy": "frame-ancestors 'none'", // generated-only
		})
		assertHasNone(t, rec.Header(), "X-Proxied-Only") // proxied-only must NOT apply
	})

	t.Run("host-less 404 gets generated-only, not proxied-only", func(t *testing.T) {
		rec := serveOn(rt, true, "GET", "https://nope.example/", "nope.example")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
			t.Fatalf("CSP = %q, want the generated-only header on the no-such-host 404", got)
		}
		assertHasNone(t, rec.Header(), "X-Proxied-Only")
	})
}

// TestSecurityHeadersScopeDeadUpstreamIsGenerated is the safety-critical guard
// for this feature: a dead/unreachable upstream produces a 502 through the
// reverse proxy's ErrorHandler, which - unlike a real upstream response - never
// runs ModifyResponse, so the response must be treated as GPM-GENERATED, not
// proxied. If the ErrorHandler ever (wrongly) marked the response proxied, a
// proxied-only header would leak onto a gpm-generated page and a generated-only
// header would vanish from it - the exact 502-leak regression. This test fails
// against that mutation and passes as-is.
func TestSecurityHeadersScopeDeadUpstreamIsGenerated(t *testing.T) {
	withGlobalScopedSecurityHeaders(t, scopedSet)
	withGlobalErrorPages(t, nil)

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example"},
		// Port 9 (discard) with nothing listening: the dial fails, so the reverse
		// proxy's ErrorHandler writes the 502 - ModifyResponse never fires.
		Upstream: model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want the 502 gpm generates for a dead upstream", rec.Code)
	}
	assertHasAll(t, rec.Header(), map[string]string{
		"X-Content-Type-Options":  "nosniff",                // all
		"Content-Security-Policy": "frame-ancestors 'none'", // generated-only
	})
	// The proxied-only header must NOT be on this gpm-generated 502.
	assertHasNone(t, rec.Header(), "X-Proxied-Only")
}

// TestSecurityHeadersScopeSetIfAbsentOnProxied proves set-if-absent still holds
// per scope: a proxied-only header the upstream already set is preserved, not
// clobbered or duplicated.
func TestSecurityHeadersScopeSetIfAbsentOnProxied(t *testing.T) {
	withGlobalScopedSecurityHeaders(t, map[string]model.SecurityHeaderValue{
		"X-Proxied-Only": {Value: "gpm", Scope: model.SecurityScopeProxied},
	})
	withGlobalErrorPages(t, nil)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Proxied-Only", "upstream")
		_, _ = w.Write([]byte("hi"))
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example"},
		Upstream:   up,
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	if got := rec.Header().Values("X-Proxied-Only"); len(got) != 1 || got[0] != "upstream" {
		t.Fatalf("X-Proxied-Only = %v, want the upstream's single value preserved (set-if-absent)", got)
	}
}

// TestSecurityHeadersScopeMergeCarriesScope proves a per-host override replaces
// the settings header INCLUDING its scope: settings declares X-Frame-Options at
// generated-only, the host overrides it to proxied-only, so on this host the
// header now lands on the proxied response and is absent from the host's own
// gpm-generated response.
func TestSecurityHeadersScopeMergeCarriesScope(t *testing.T) {
	withGlobalScopedSecurityHeaders(t, map[string]model.SecurityHeaderValue{
		"X-Frame-Options": {Value: "DENY", Scope: model.SecurityScopeGenerated},
	})
	withGlobalErrorPages(t, nil)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta:      model.ObjectMeta{Name: "app"},
		Domains:         []string{"app.example"},
		Upstream:        up,
		SecurityHeaders: map[string]model.SecurityHeaderValue{"X-Frame-Options": {Value: "SAMEORIGIN", Scope: model.SecurityScopeProxied}},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	// Proxied 200: the override (proxied-only) now applies.
	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	if got := rec.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("proxied X-Frame-Options = %q, want the host override SAMEORIGIN (scope moved to proxied-only)", got)
	}

	// The host's own gpm-generated 400: the override's proxied-only scope means the
	// header is absent (the settings generated-only value was replaced, not kept).
	rec = serveOn(rt, true, "GET", "https://app.example/a;b", "app.example")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertHasNone(t, rec.Header(), "X-Frame-Options")
}
