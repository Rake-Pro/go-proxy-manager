package dataplane

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// withGlobalStripResponseHeaders installs a settings-level strip list for one
// test and restores the previous one, mirroring withGlobalScopedSecurityHeaders.
func withGlobalStripResponseHeaders(t *testing.T, names []string) {
	t.Helper()
	prev := globalStripResponseHeaders.Load()
	SetStripResponseHeaders(names)
	t.Cleanup(func() { globalStripResponseHeaders.Store(prev) })
}

// leakyUpstream is a backend that leaks the usual backend-identifying headers
// alongside a normal body.
func leakyUpstream(t *testing.T) (model.Upstream, func()) {
	t.Helper()
	return backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "Apache/2.4.1 (Unix)")
		w.Header().Set("X-Powered-By", "PHP/8.1.0")
		w.Header().Set("X-AspNet-Version", "4.0.30319")
		w.Header().Set("X-Keep", "kept")
		_, _ = w.Write([]byte("hi"))
	}))
}

// oneHost builds a single-proxy-host router for app.example on up.
func oneHost(t *testing.T, up model.Upstream, mutate func(*model.ProxyHost)) *router {
	t.Helper()
	h := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example"},
		Upstream:   up,
	}
	if mutate != nil {
		mutate(&h)
	}
	rt, err := buildRouter(model.Config{ProxyHosts: []model.ProxyHost{h}}, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	return rt
}

// serveHandler drives an arbitrary handler the way serveOn drives the router, so
// a test can wrap the router in an outer handler (to simulate a header gpm wrote
// before the proxy ran).
func serveHandler(h http.Handler, tls bool, method, rawurl, host string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, rawurl, nil)
	req.Host = host
	if !tls {
		req.TLS = nil
	}
	h.ServeHTTP(rec, req)
	return rec
}

// serveOnWith is serveOn with extra request headers (for the Accept-Encoding the
// compression handler keys on).
func serveOnWith(rt *router, tls bool, method, rawurl, host string, hdr http.Header) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, rawurl, nil)
	req.Host = host
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if tls {
		rt.serveHTTPS(rec, req)
	} else {
		rt.serveHTTP(rec, req)
	}
	return rec
}

// TestStripResponseHeadersOnProxiedResponse is the core of the feature: the
// configured names are gone from a real proxied upstream response, end to end
// through the router, and every other upstream header is untouched. The config
// names them in lower case while the upstream sets the canonical form, so this
// also pins the case-insensitive match.
func TestStripResponseHeadersOnProxiedResponse(t *testing.T) {
	withGlobalStripResponseHeaders(t, []string{"server", "x-powered-by", "X-AspNet-Version"})
	withGlobalScopedSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	up, closeFn := leakyUpstream(t)
	defer closeFn()
	rt := oneHost(t, up, nil)

	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	if rec.Code != http.StatusOK || rec.Body.String() != "hi" {
		t.Fatalf("proxied response not passed through: %d %q", rec.Code, rec.Body.String())
	}
	assertHasNone(t, rec.Header(), "Server", "X-Powered-By", "X-AspNet-Version")
	if got := rec.Header().Get("X-Keep"); got != "kept" {
		t.Fatalf("X-Keep = %q, want the unlisted upstream header untouched", got)
	}
}

// TestStripResponseHeadersOnlyTouchesUpstreamHeaders is the guard for the whole
// design decision behind stripping at ModifyResponse rather than on the way out
// of the dispatch writer. The writer sees ONE merged header map, so a strip
// there would also delete headers GPM ITSELF wrote earlier in the response:
//
//   - a Set-Cookie an outer handler added (this is exactly what forward-auth's
//     copySetCookie does with the IdP's refreshed session cookie - stripping it
//     would silently break session sliding),
//   - a headers-middleware setResponse value,
//   - an injected security header / HSTS / X-Robots-Tag.
//
// Every one of those is in the strip list here AND sent by the upstream: the
// upstream's copy must be gone, gpm's must survive.
func TestStripResponseHeadersOnlyTouchesUpstreamHeaders(t *testing.T) {
	withGlobalStripResponseHeaders(t, []string{
		"Set-Cookie", "X-Mw", "X-Frame-Options", "Strict-Transport-Security", "X-Robots-Tag",
	})
	withGlobalScopedSecurityHeaders(t, map[string]model.SecurityHeaderValue{
		"X-Frame-Options": {Value: "DENY", Scope: model.SecurityScopeAll},
	})
	withGlobalErrorPages(t, nil)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Set-Cookie", "backend_session=upstream; Path=/")
		w.Header().Set("X-Mw", "upstream")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Strict-Transport-Security", "max-age=1")
		w.Header().Set("X-Robots-Tag", "all")
		_, _ = w.Write([]byte("hi"))
	}))
	defer closeFn()

	cfg := model.Config{
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "mw"},
			Type:       "headers",
			Headers:    &model.HeadersMiddleware{SetResponse: map[string]string{"X-Mw": "gpm"}},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:    model.ObjectMeta{Name: "app"},
			Domains:       []string{"app.example"},
			Upstream:      up,
			Middlewares:   []string{"mw"},
			RobotsNoIndex: true,
			TLS:           model.TLSSettings{HSTS: model.HSTS{Enabled: true, MaxAge: 300}},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	// An outer handler that writes a Set-Cookie BEFORE the proxy runs, standing in
	// for forward-auth's copySetCookie of the IdP's refreshed session cookie.
	outer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "gpm_session=refreshed; Path=/; HttpOnly")
		rt.serveHTTPS(w, r)
	})
	rec := serveHandler(outer, true, "GET", "https://app.example/", "app.example")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// gpm's own Set-Cookie survives; the upstream's is gone.
	cookies := rec.Header().Values("Set-Cookie")
	if len(cookies) != 1 || !strings.HasPrefix(cookies[0], "gpm_session=refreshed") {
		t.Fatalf("Set-Cookie = %v, want only gpm's own refreshed session cookie; a strip at the dispatch writer would have deleted it too", cookies)
	}
	assertHasAll(t, rec.Header(), map[string]string{
		"X-Mw":                      "gpm",         // headers-middleware setResponse
		"X-Frame-Options":           "DENY",        // injected securityHeader
		"Strict-Transport-Security": "max-age=300", // gpm owns HSTS
		"X-Robots-Tag":              "noindex, nofollow",
	})
	for _, k := range []string{"X-Mw", "X-Frame-Options", "Strict-Transport-Security", "X-Robots-Tag"} {
		if got := rec.Header().Values(k); len(got) != 1 {
			t.Fatalf("%s = %v, want a single value (gpm's), not the upstream's alongside it", k, got)
		}
	}
}

// TestStripResponseHeadersGeneratedResponsesUnaffected pins that a gpm-generated
// response - which has no upstream response, so ModifyResponse never runs - keeps
// every header gpm put on it even when that header is in the strip list.
func TestStripResponseHeadersGeneratedResponsesUnaffected(t *testing.T) {
	withGlobalStripResponseHeaders(t, []string{"X-Robots-Tag", "X-Frame-Options"})
	withGlobalScopedSecurityHeaders(t, map[string]model.SecurityHeaderValue{
		"X-Frame-Options": {Value: "DENY", Scope: model.SecurityScopeAll},
	})
	withGlobalErrorPages(t, nil)

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta:    model.ObjectMeta{Name: "dead"},
		Domains:       []string{"dead.example"},
		RobotsNoIndex: true,
		// Port 9 (discard) with nothing listening: the reverse proxy's ErrorHandler
		// writes the 502, so ModifyResponse - and the strip - never run.
		Upstream: model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	t.Run("gpm-generated 502", func(t *testing.T) {
		rec := serveOn(rt, true, "GET", "https://dead.example/", "dead.example")
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want the 502 gpm generates for a dead upstream", rec.Code)
		}
		assertHasAll(t, rec.Header(), map[string]string{
			"X-Frame-Options": "DENY",
			"X-Robots-Tag":    "noindex, nofollow",
		})
	})

	t.Run("gpm-generated 400", func(t *testing.T) {
		rec := serveOn(rt, true, "GET", "https://dead.example/a;b", "dead.example")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want the 400 path rejection", rec.Code)
		}
		// X-Robots-Tag is not asserted here: the path rejection is written before
		// the per-host robots writer is installed, which is pre-existing dispatch
		// ordering and unrelated to stripping.
		assertHasAll(t, rec.Header(), map[string]string{"X-Frame-Options": "DENY"})
	})
}

// TestStripResponseHeadersHostUnionsSettings pins the merge semantics: a host's
// own list ADDS to the settings default rather than replacing it, so the fleet
// baseline still applies on a host that names an extra header, and a host with no
// list of its own gets the baseline unchanged.
func TestStripResponseHeadersHostUnionsSettings(t *testing.T) {
	withGlobalStripResponseHeaders(t, []string{"Server"})
	withGlobalScopedSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	up, closeFn := leakyUpstream(t)
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{
			ObjectMeta:           model.ObjectMeta{Name: "app"},
			Domains:              []string{"app.example"},
			Upstream:             up,
			StripResponseHeaders: []string{"X-Powered-By"},
		},
		{
			ObjectMeta: model.ObjectMeta{Name: "plain"},
			Domains:    []string{"plain.example"},
			Upstream:   up,
		},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	// The overriding host: BOTH the settings header and its own are stripped. If
	// the host list replaced the settings default, Server would still be here.
	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	assertHasNone(t, rec.Header(), "Server", "X-Powered-By")

	// The host with no list of its own: the settings default still applies, and
	// the host-only header is untouched.
	rec = serveOn(rt, true, "GET", "https://plain.example/", "plain.example")
	assertHasNone(t, rec.Header(), "Server")
	if got := rec.Header().Get("X-Powered-By"); got != "PHP/8.1.0" {
		t.Fatalf("X-Powered-By = %q, want the other host's override not to leak onto this host", got)
	}
}

// TestStripResponseHeadersOnUpgradeResponse covers the response the dispatch
// writer can never see: a 101. The stdlib hijacks the connection and copies the
// upstream's headers straight to it, so a strip on the ResponseWriter would miss
// the WebSocket handshake entirely and leak exactly the fingerprint this feature
// removes from every other response. Stripping in ModifyResponse - which the
// stdlib runs for a 101 BEFORE handleUpgradeResponse - covers it.
//
// Connection/Upgrade cannot be stripped (hop-by-hop names are refused at
// validation), so the handshake itself stays intact.
func TestStripResponseHeadersOnUpgradeResponse(t *testing.T) {
	withGlobalStripResponseHeaders(t, []string{"Server", "X-Powered-By"})
	withGlobalScopedSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	// A backend that answers the upgrade with 101 plus the leaked headers.
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: " + r.Header.Get("Upgrade") + "\r\nConnection: Upgrade\r\n" +
			"Server: Apache/2.4.1 (Unix)\r\nX-Powered-By: PHP/8.1.0\r\nX-Keep: kept\r\n\r\n")
		_ = brw.Flush()
	}))
	defer closeFn()

	rt := oneHost(t, up, nil)
	// A real listener: an httptest recorder is not a Hijacker, so the 101 path
	// needs a live socket to switch protocols on.
	addr := listenWithPolicy(t, http.HandlerFunc(rt.serveHTTP))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: app.example\r\n"+
		"Connection: Upgrade\r\nUpgrade: websocket\r\n\r\n"); err != nil {
		t.Fatal(err)
	}

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("reading the upgrade response: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("upgrade response was %q, want 101", strings.TrimSpace(status))
	}
	hdrs := map[string]string{}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading the upgrade response headers: %v", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		k, v, _ := strings.Cut(line, ":")
		hdrs[http.CanonicalHeaderKey(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}

	for _, k := range []string{"Server", "X-Powered-By"} {
		if got, ok := hdrs[k]; ok {
			t.Fatalf("%s = %q on the 101 handshake, want it stripped", k, got)
		}
	}
	if hdrs["X-Keep"] != "kept" {
		t.Fatalf("X-Keep = %q on the 101, want the unlisted header carried through", hdrs["X-Keep"])
	}
	// The handshake must still be a handshake.
	if !strings.EqualFold(hdrs["Upgrade"], "websocket") || !strings.EqualFold(hdrs["Connection"], "Upgrade") {
		t.Fatalf("upgrade headers = %q/%q, want the handshake intact", hdrs["Upgrade"], hdrs["Connection"])
	}
}

// TestStripResponseHeadersLeavesGzipHeadersAlone pins the compression
// interaction. Content-Encoding and Vary are refused at validation, so the strip
// list here names an allowed header on a gzip-enabled host: the gzip headers gpm
// adds must be untouched and the response must still decode.
func TestStripResponseHeadersLeavesGzipHeadersAlone(t *testing.T) {
	withGlobalStripResponseHeaders(t, []string{"Server", "X-Powered-By"})
	withGlobalScopedSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	// Sized so the upstream sends a Content-Length: a chunked upstream response
	// makes the stdlib proxy flush immediately, which the compression handler
	// (correctly) treats as streaming and passes through uncompressed.
	body := strings.Repeat("compress me ", 120)
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Server", "Apache/2.4.1 (Unix)")
		_, _ = w.Write([]byte(body))
	}))
	defer closeFn()

	rt := oneHost(t, up, func(h *model.ProxyHost) {
		h.Compression = model.Compression{Enabled: true}
	})

	rec := serveOnWith(rt, true, "GET", "https://app.example/", "app.example", http.Header{
		"Accept-Encoding": []string{"gzip"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	assertHasNone(t, rec.Header(), "Server")
	assertHasAll(t, rec.Header(), map[string]string{
		"Content-Encoding": "gzip",
		"Content-Type":     "text/plain; charset=utf-8",
	})
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want the gzip Vary intact", got)
	}
}

// TestStripResponseHeadersUnconfigured keeps the zero-overhead promise: with
// nothing configured at either level no upstream header is touched.
func TestStripResponseHeadersUnconfigured(t *testing.T) {
	withGlobalStripResponseHeaders(t, nil)
	withGlobalScopedSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	if got := mergedStripResponseHeaders(nil); got != nil {
		t.Fatalf("mergedStripResponseHeaders(nil) = %v, want nil for an unconfigured host", got)
	}

	up, closeFn := leakyUpstream(t)
	defer closeFn()
	rt := oneHost(t, up, nil)

	rec := serveOn(rt, true, "GET", "https://app.example/", "app.example")
	assertHasAll(t, rec.Header(), map[string]string{
		"Server":       "Apache/2.4.1 (Unix)",
		"X-Powered-By": "PHP/8.1.0",
	})
}

// TestStripResponseHeadersOnPlaintextListener covers the second dispatch path:
// a non-forceSSL host served over plain HTTP strips too.
func TestStripResponseHeadersOnPlaintextListener(t *testing.T) {
	withGlobalStripResponseHeaders(t, []string{"Server"})
	withGlobalScopedSecurityHeaders(t, nil)
	withGlobalErrorPages(t, nil)

	up, closeFn := leakyUpstream(t)
	defer closeFn()
	rt := oneHost(t, up, nil)

	rec := serveOn(rt, false, "GET", "http://app.example/", "app.example")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	assertHasNone(t, rec.Header(), "Server")
}

// TestCompileStripResponseHeaders covers the compile step on its own:
// canonicalization, de-duplication across case, the nil result that selects the
// zero-overhead path, and the defence-in-depth drop of names validation refuses
// (a config that bypassed validation must not be able to corrupt a response).
func TestCompileStripResponseHeaders(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"empty names dropped", []string{""}, nil},
		{"canonicalized", []string{"x-powered-by"}, []string{"X-Powered-By"}},
		{"case-insensitive dedupe", []string{"Server", "SERVER", "server"}, []string{"Server"}},
		{"order preserved", []string{"Server", "X-Powered-By"}, []string{"Server", "X-Powered-By"}},
		{"hop-by-hop dropped", []string{"Connection", "Transfer-Encoding", "Server"}, []string{"Server"}},
		{"response-semantic dropped", []string{"Content-Type", "Content-Length", "Content-Encoding", "Vary", "Location", "Server"}, []string{"Server"}},
		{"all-refused compiles to nil", []string{"Content-Type", "Connection"}, nil},
		// The compile step shares the model's refusal predicate, so the websocket
		// handshake headers are dropped here too - including the spelling a
		// client uses, which canonicalizes to "Sec-Websocket-Accept".
		{"websocket handshake headers dropped", []string{"Sec-WebSocket-Accept", "Sec-Websocket-Protocol", "sec-websocket-extensions", "Server"}, []string{"Server"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := compileStripResponseHeaders(tc.in)
			// reflect.DeepEqual so a nil result is distinguished from an empty
			// slice: only nil selects the zero-overhead path.
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("compile(%v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
