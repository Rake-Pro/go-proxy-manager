package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// withGlobalSecurityHeaders installs m as the settings-level default security
// headers for the duration of the test and restores the previous handle on
// cleanup, so tests never leak state through the package-level pointer.
func withGlobalSecurityHeaders(t *testing.T, m map[string]string) {
	t.Helper()
	prev := globalSecurityHeaders.Load()
	SetSecurityHeaders(m)
	t.Cleanup(func() { globalSecurityHeaders.Store(prev) })
}

// defaultSecHeaders is a representative settings-level set used across the table.
var defaultSecHeaders = map[string]string{
	"X-Content-Type-Options": "nosniff",
	"X-Frame-Options":        "DENY",
	"Referrer-Policy":        "strict-origin-when-cross-origin",
}

func assertHasAll(t *testing.T, h http.Header, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Fatalf("header %s = %q, want %q", k, got, v)
		}
	}
}

func assertHasNone(t *testing.T, h http.Header, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if got := h.Get(k); got != "" {
			t.Fatalf("header %s = %q, want it absent", k, got)
		}
	}
}

// TestSecurityHeadersOnGeneratedResponses drives every gpm-generated response
// that is reachable straight through the router and asserts the settings-level
// headers land on it: the no-such-host 404, the path-rejection 400, a parked
// host, and a redirect host. Each is a response gpm writes itself, so the
// headers must be present regardless of any auth outcome.
func TestSecurityHeadersOnGeneratedResponses(t *testing.T) {
	withGlobalSecurityHeaders(t, defaultSecHeaders)
	withGlobalErrorPages(t, nil)

	cfg := model.Config{
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "app"},
			Domains:    []string{"app.example"},
			Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
		}},
		ParkedHosts: []model.ParkedHost{{
			ObjectMeta: model.ObjectMeta{Name: "parked"}, Domains: []string{"parked.example"},
		}},
		RedirectHosts: []model.RedirectHost{{
			ObjectMeta: model.ObjectMeta{Name: "r"}, Domains: []string{"r.example"}, TargetDomain: "dest.example",
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	cases := []struct {
		name       string
		tls        bool
		url, host  string
		wantStatus int
	}{
		{"404 no such host", true, "https://nope.example/", "nope.example", http.StatusNotFound},
		{"400 bad path", true, "https://app.example/a;b", "app.example", http.StatusBadRequest},
		{"parked host", true, "https://parked.example/x", "parked.example", http.StatusNotFound},
		{"redirect host", true, "https://r.example/x", "r.example", http.StatusMovedPermanently},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveOn(rt, tc.tls, "GET", tc.url, tc.host)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			assertHasAll(t, rec.Header(), defaultSecHeaders)
		})
	}
}

// TestSecurityHeadersOnAuthGateRefusal builds a real access-list-gated proxy
// host and asserts the configured headers reach the gate's own 403/401 refusal,
// end to end through the router (the writer is installed in serveHTTPS, the gate
// runs inside the chain beneath it).
func TestSecurityHeadersOnAuthGateRefusal(t *testing.T) {
	withGlobalSecurityHeaders(t, defaultSecHeaders)
	withGlobalErrorPages(t, nil)

	denyAll := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "deny"},
		DefaultAction: model.ActionDeny,
	}
	basic := model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: "basic"},
		BasicAuth:  []model.BasicAuthUser{{Username: "admin", PasswordHash: "$2a$04$abcdefghijklmnopqrstuv"}},
	}
	cfg := model.Config{
		AccessLists: []model.AccessList{denyAll, basic},
		ProxyHosts: []model.ProxyHost{
			{
				ObjectMeta:  model.ObjectMeta{Name: "denied"},
				Domains:     []string{"deny.example"},
				Upstream:    model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
				AccessLists: []string{"deny"},
			},
			{
				ObjectMeta:  model.ObjectMeta{Name: "auth"},
				Domains:     []string{"auth.example"},
				Upstream:    model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
				AccessLists: []string{"basic"},
			},
		},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	rec := serveOn(rt, true, "GET", "https://deny.example/", "deny.example")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("deny: status = %d, want 403", rec.Code)
	}
	assertHasAll(t, rec.Header(), defaultSecHeaders)

	rec = serveOn(rt, true, "GET", "https://auth.example/", "auth.example")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("basic-auth: status = %d, want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("basic-auth 401 should carry a challenge")
	}
	assertHasAll(t, rec.Header(), defaultSecHeaders)
}

// TestSecurityHeadersOnGateHandlers covers the two gpm-generated responses whose
// hermetic construction lives in the auth tier (a 401 auth-gate refusal and a
// 302 sign-in redirect) by wrapping the REAL gate handlers in the exact writer
// serveHTTPS installs. This proves the writer injects on those responses; the
// tests above prove the writer is actually installed in the router.
func TestSecurityHeadersOnGateHandlers(t *testing.T) {
	withGlobalErrorPages(t, nil)
	hdrs := compileSecurityHeaders(defaultSecHeaders)

	t.Run("401 forward-auth refusal", func(t *testing.T) {
		gate := forwardAuthGate(trustedFA(), nil, nil, "m", nil, okNext(t))
		rec := httptest.NewRecorder()
		w := withSecurityHeaders(rec, hdrs)
		r := httptest.NewRequest("GET", "https://m.example/", nil)
		r.RemoteAddr = "203.0.113.5:5000" // untrusted peer -> 401
		r.Header.Set("X-Auth-User", "forged")
		gate.ServeHTTP(w, r)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		assertHasAll(t, rec.Header(), defaultSecHeaders)
	})

	t.Run("302 sign-in redirect", func(t *testing.T) {
		stub := &outpostStub{authStatus: http.StatusUnauthorized}
		srv := httptest.NewServer(stub.handler())
		defer srv.Close()
		gate := newAuthReqProxy(t, srv.URL).handler(peerIP, nil, "app", nil, okNext(t))
		rec := httptest.NewRecorder()
		w := withSecurityHeaders(rec, hdrs)
		gate.ServeHTTP(w, httptest.NewRequest("GET", "http://app2.example.com/", nil))
		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want the 302 sign-in redirect", rec.Code)
		}
		if rec.Header().Get("Location") == "" {
			t.Fatal("sign-in redirect should carry a Location")
		}
		assertHasAll(t, rec.Header(), defaultSecHeaders)
	})

	t.Run("rendered error page", func(t *testing.T) {
		ep := inlinePages(t, map[string]string{"403": "<h1>denied</h1>"})
		rec := httptest.NewRecorder()
		w := withSecurityHeaders(rec, hdrs)
		refuse(w, http.StatusForbidden, ep, "m", "forbidden")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if rec.Body.String() != "<h1>denied</h1>" {
			t.Fatalf("body = %q, want the rendered page", rec.Body.String())
		}
		assertHasAll(t, rec.Header(), defaultSecHeaders)
	})
}

// TestSecurityHeadersPerHostOverride proves the per-key merge: a host override
// replaces the settings value for the header it names and leaves the rest of the
// settings default in place.
func TestSecurityHeadersPerHostOverride(t *testing.T) {
	withGlobalSecurityHeaders(t, defaultSecHeaders)
	withGlobalErrorPages(t, nil)

	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{
			ObjectMeta: model.ObjectMeta{Name: "plain"},
			Domains:    []string{"plain.example"},
			Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
		},
		{
			ObjectMeta:      model.ObjectMeta{Name: "over"},
			Domains:         []string{"over.example"},
			Upstream:        model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
			SecurityHeaders: map[string]string{"X-Frame-Options": "SAMEORIGIN"},
		},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	// The overriding host: its own X-Frame-Options wins, the other two fall through.
	rec := serveOn(rt, true, "GET", "https://over.example/a;b", "over.example")
	if got := rec.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options = %q, want the host override SAMEORIGIN", got)
	}
	assertHasAll(t, rec.Header(), map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	})

	// The unrelated host still gets the settings default unchanged.
	rec = serveOn(rt, true, "GET", "https://plain.example/a;b", "plain.example")
	assertHasAll(t, rec.Header(), defaultSecHeaders)
}

// TestSecurityHeadersSetIfAbsentOnProxied is the property that keeps a backed app
// safe: on a proxied upstream response, a header the upstream already set is
// preserved, and only a header it omitted is added.
func TestSecurityHeadersSetIfAbsentOnProxied(t *testing.T) {
	withGlobalSecurityHeaders(t, map[string]string{
		"X-Frame-Options":        "DENY",    // upstream sets its own; must NOT be clobbered
		"X-Content-Type-Options": "nosniff", // upstream omits; gpm adds
	})
	withGlobalErrorPages(t, nil)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
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
	if rec.Code != http.StatusOK || rec.Body.String() != "hi" {
		t.Fatalf("proxied response not passed through: %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Values("X-Frame-Options"); len(got) != 1 || got[0] != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options = %v, want the upstream's single SAMEORIGIN preserved (not clobbered/duplicated)", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want gpm to add the header the upstream omitted", got)
	}
}

// TestSecurityHeadersEmptyIsNoChange proves the opt-in default: with nothing
// configured, no header is added and the writer does not wrap the response.
func TestSecurityHeadersEmptyIsNoChange(t *testing.T) {
	withGlobalSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example"},
		Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	if rt.securityHeadersFor("app.example") != nil {
		t.Fatal("an unconfigured host must resolve a nil header set (zero-overhead path)")
	}
	// http.Error itself always sets X-Content-Type-Options: nosniff, so assert on
	// the headers this feature owns and stdlib does not touch.
	rec := serveOn(rt, true, "GET", "https://nope.example/", "nope.example")
	assertHasNone(t, rec.Header(), "X-Frame-Options", "Referrer-Policy")
}

// TestSecurityHeadersHSTSIndependent proves this feature is additive: HSTS is
// still emitted by its own mechanism, unchanged, alongside the configured
// security headers, and the HSTS header is never one of them.
func TestSecurityHeadersHSTSIndependent(t *testing.T) {
	withGlobalSecurityHeaders(t, defaultSecHeaders)
	withGlobalErrorPages(t, nil)

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example"},
		Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
		TLS:        model.TLSSettings{HSTS: model.HSTS{Enabled: true, MaxAge: 60}},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	// A request that reaches the proxy: the upstream (127.0.0.1:9) refuses so gpm
	// generates a 502, and HSTS (its own override emission) and the security
	// headers all ride it. Both mechanisms fire, independently.
	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want the 502 gpm generates for a dead upstream", rec.Code)
	}
	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=60" {
		t.Fatalf("HSTS = %q, want the unchanged max-age=60", got)
	}
	assertHasAll(t, rec.Header(), defaultSecHeaders)

	// The compiled host set must never carry HSTS (the map rejects it, and compile
	// drops it defensively).
	if v := rt.hosts["app.example"].securityHeaders.Get("Strict-Transport-Security"); v != "" {
		t.Fatalf("securityHeaders carried an HSTS header %q; hsts owns that header", v)
	}
}

// TestHSTSOverridesUpstreamValue pins the override + single-header semantics for
// HSTS: gpm is the TLS edge and owns Strict-Transport-Security, so when an
// upstream also sets it the FINAL response carries gpm's value ONCE, not the
// upstream's and not both. A mutation flipping HSTS to set-if-absent (letting the
// upstream win) must fail this.
func TestHSTSOverridesUpstreamValue(t *testing.T) {
	withGlobalSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=1") // weak upstream value
		_, _ = w.Write([]byte("hi"))
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example"},
		Upstream:   up,
		TLS:        model.TLSSettings{HSTS: model.HSTS{Enabled: true, MaxAge: 200}},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	got := rec.Header().Values("Strict-Transport-Security")
	if len(got) != 1 || got[0] != "max-age=200" {
		t.Fatalf("Strict-Transport-Security = %v, want gpm's single overriding max-age=200 (not the upstream's, not both)", got)
	}
}

// TestRobotsDoesNotOverrideUpstreamValue pins the set-if-absent semantics for
// X-Robots-Tag: an upstream (or a headers middleware) that sets it explicitly
// wins, gpm does not clobber it. A mutation flipping robots to override must fail
// this.
func TestRobotsDoesNotOverrideUpstreamValue(t *testing.T) {
	withGlobalSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Robots-Tag", "index") // upstream deliberately allows indexing
		_, _ = w.Write([]byte("hi"))
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta:    model.ObjectMeta{Name: "app"},
		Domains:       []string{"app.example"},
		Upstream:      up,
		RobotsNoIndex: true, // gpm would emit "noindex, nofollow" if it overrode
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	got := rec.Header().Values("X-Robots-Tag")
	if len(got) != 1 || got[0] != "index" {
		t.Fatalf("X-Robots-Tag = %v, want the upstream's single \"index\" to win (set-if-absent)", got)
	}
}
