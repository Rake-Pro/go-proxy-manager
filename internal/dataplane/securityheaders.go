package dataplane

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// globalSecurityHeaders holds the compiled settings-level default security
// headers, installed once per config reload via SetSecurityHeaders and read by
// buildRouter (to compose the per-host set) and by the router directly (for
// host-less responses: the no-such-host 404, and redirect/parked hosts, which
// carry no per-host override). It mirrors globalErrorPages' package-level
// handle: threading Settings through buildRouter for a value that only ever
// changes alongside a full reload would touch far more of the data plane. The
// stored value keeps each header's scope so the per-host merge below can carry
// it; partitionSecurityHeaders splits it into the writer's two subsets.
var globalSecurityHeaders atomic.Pointer[map[string]securityHeaderRule]

// securityHeaderRule is one compiled response header: its (canonicalized) value
// and the scope selecting which responses it lands on.
type securityHeaderRule struct {
	value string
	scope model.SecurityScope
}

// SetSecurityHeaders compiles and installs the settings-level default response
// headers. It is called before the data-plane Reload so the header set is in
// place before any request is served. The map has already passed
// model.Settings.Validate at config-write time; compile canonicalizes the names,
// normalizes an empty scope to "all", and defensively drops anything a valid
// config could not contain (an empty name or Strict-Transport-Security, which
// the per-host hsts setting owns).
func SetSecurityHeaders(m map[string]model.SecurityHeaderValue) {
	c := compileSecurityHeaders(m)
	globalSecurityHeaders.Store(&c)
}

func currentSecurityHeaders() map[string]securityHeaderRule {
	if p := globalSecurityHeaders.Load(); p != nil {
		return *p
	}
	return nil
}

// compileSecurityHeaders canonicalizes m into a name->rule map, or returns nil
// when it configures nothing. Model validation is authoritative; this only
// canonicalizes, normalizes the scope, and skips the two entries a bypassed
// config might still carry.
func compileSecurityHeaders(m map[string]model.SecurityHeaderValue) map[string]securityHeaderRule {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]securityHeaderRule, len(m))
	for k, v := range m {
		ck := textproto.CanonicalMIMEHeaderKey(k)
		if ck == "" || strings.EqualFold(ck, "Strict-Transport-Security") {
			continue
		}
		scope := v.Scope
		if scope == "" {
			scope = model.SecurityScopeAll
		}
		out[ck] = securityHeaderRule{value: v.Value, scope: scope}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// mergedSecurityHeaders composes the effective rule set for one proxy host: the
// settings-level default with the host's own override merged over it per key
// (the host value wins for a header it names, including that header's scope; a
// header it omits falls through to the settings default). Returns nil when
// neither configures anything, so an unconfigured host takes the zero-overhead
// unwrapped path.
func mergedSecurityHeaders(override map[string]model.SecurityHeaderValue) map[string]securityHeaderRule {
	base := currentSecurityHeaders()
	ov := compileSecurityHeaders(override)
	if len(base) == 0 && len(ov) == 0 {
		return nil
	}
	out := make(map[string]securityHeaderRule, len(base)+len(ov))
	for k, r := range base {
		out[k] = r
	}
	for k, r := range ov {
		out[k] = r // host override replaces this key, including its scope
	}
	return out
}

// scopedHeaders is a compiled rule set split by WHERE each header applies:
// generated is emitted on gpm-generated responses (scope all + generated-only),
// proxied on proxied upstream responses (scope all + proxied-only). Both are
// injected set-if-absent. Either http.Header may be nil (no header for that
// side). A nil *scopedHeaders means nothing is configured at all.
type scopedHeaders struct {
	generated http.Header
	proxied   http.Header
}

func (s *scopedHeaders) empty() bool {
	return s == nil || (len(s.generated) == 0 && len(s.proxied) == 0)
}

// partitionSecurityHeaders splits a merged rule set into the two subsets the
// dispatch writer selects between at inject time. Returns nil when the split
// produces nothing (so an unconfigured host resolves the zero-overhead path).
func partitionSecurityHeaders(rules map[string]securityHeaderRule) *scopedHeaders {
	if len(rules) == 0 {
		return nil
	}
	var s scopedHeaders
	for k, r := range rules {
		if r.scope == model.SecurityScopeAll || r.scope == model.SecurityScopeGenerated {
			if s.generated == nil {
				s.generated = make(http.Header)
			}
			s.generated[k] = []string{r.value}
		}
		if r.scope == model.SecurityScopeAll || r.scope == model.SecurityScopeProxied {
			if s.proxied == nil {
				s.proxied = make(http.Header)
			}
			s.proxied[k] = []string{r.value}
		}
	}
	if s.empty() {
		return nil
	}
	return &s
}

// securityWriterKey is the request-context key under which serveHTTP(S) stashes
// the dispatch writer, so the reverse proxy's ModifyResponse (which fires only
// when an actual upstream response arrives - never on the ErrorHandler's
// gpm-generated 502/504) can mark the response as proxied at inject time. The
// ErrorHandler path leaves the flag false, so a gpm-generated error page keeps
// the generated-only subset. Mirrors metricshook's hostNameKey pattern.
type securityWriterKey struct{}

// requestWithSecurityWriter stashes w on r's context when w is the dispatch
// writer, so a downstream proxied response can flip its scope. When w carries no
// securityHeaderWriter (nothing configured for this host) r is returned
// unchanged - there is nothing to inject either way.
func requestWithSecurityWriter(r *http.Request, w http.ResponseWriter) *http.Request {
	if sw, ok := w.(*securityHeaderWriter); ok {
		return r.WithContext(context.WithValue(r.Context(), securityWriterKey{}, sw))
	}
	return r
}

// markProxiedResponse flips the dispatch writer (if any) to emit the proxied
// subset. Called from the reverse proxy's ModifyResponse, i.e. only when the
// upstream actually responded.
func markProxiedResponse(ctx context.Context) {
	if sw, ok := ctx.Value(securityWriterKey{}).(*securityHeaderWriter); ok {
		sw.isProxied = true
	}
}

// securityHeaderWriter injects gpm's configured response headers on the way out,
// set-if-absent: a header already present (a proxied upstream response has its
// own value copied in before WriteHeader) is preserved, while a gpm-generated
// response - which set none - gets it added. At inject time it selects the
// subset for this response: the proxied subset once markProxiedResponse has run
// (an upstream actually answered), else the generated subset (every gpm terminal
// handler, including the ErrorHandler's 502/504). This is the same response
// layer HSTS is emitted at, so the headers land on every gpm-generated
// denial/redirect/error page regardless of the auth outcome, while never
// clobbering a backed app's own security headers. It forwards the optional
// interfaces the reverse proxy relies on (Flusher, Hijacker, Unwrap), exactly
// like responseObserver.
type securityHeaderWriter struct {
	http.ResponseWriter
	// generated and proxied are the two scoped subsets (shared, read-only - never
	// mutated here). inject picks one by isProxied.
	generated http.Header
	proxied   http.Header
	// isProxied is set (via the request context) by the reverse proxy's
	// ModifyResponse when an upstream response arrives, selecting the proxied
	// subset. It stays false for every gpm-generated response.
	isProxied bool
	// hsts and robots are the host's per-host response headers, moved onto this
	// same writer so they survive a 1xx interim response's header-map clear (the
	// old "Set() before serving" put them in the map that Got1xxResponse wipes).
	// hsts is emitted with OVERRIDE semantics (gpm is the TLS edge and owns
	// Strict-Transport-Security); robots is emitted set-if-absent so a headers
	// middleware that sets X-Robots-Tag explicitly still wins.
	hsts   string
	robots string
	wrote  bool
}

func (s *securityHeaderWriter) inject() {
	if s.wrote {
		return
	}
	s.wrote = true
	dst := s.ResponseWriter.Header()
	src := s.generated
	if s.isProxied {
		src = s.proxied
	}
	for k, vs := range src {
		if len(dst[k]) > 0 {
			continue // set-if-absent: never clobber a value already on the response
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
	if s.robots != "" && dst.Get("X-Robots-Tag") == "" {
		dst.Set("X-Robots-Tag", s.robots) // set-if-absent: a headers middleware wins
	}
	if s.hsts != "" {
		dst.Set("Strict-Transport-Security", s.hsts) // override: gpm owns HSTS
	}
}

func (s *securityHeaderWriter) WriteHeader(code int) {
	// A 1xx is an interim response: httputil.ReverseProxy writes it via
	// Got1xxResponse and then CLEARS the response header map to keep interim
	// headers out of the final response. Injecting (and latching wrote) here
	// would seed headers that the clear immediately deletes, and the wrote guard
	// would then suppress re-injection on the final WriteHeader - so the
	// configured headers would vanish from the final response. Only final
	// responses (>= 200) inject and latch. By the final WriteHeader the proxied
	// flag has already been set (ModifyResponse ran on the upstream response
	// before its headers are written), so the right subset is chosen.
	if code >= 200 {
		s.inject()
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *securityHeaderWriter) Write(b []byte) (int, error) {
	s.inject()
	return s.ResponseWriter.Write(b)
}

func (s *securityHeaderWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *securityHeaderWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter is not an http.Hijacker")
	}
	return h.Hijack()
}

// Unwrap lets http.ResponseController find the base writer.
func (s *securityHeaderWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// withSecurityHeaders wraps w so headers are injected set-if-absent when the
// response is written. It returns w unwrapped when there is nothing to inject,
// so an unconfigured deployment has zero per-request overhead.
func withSecurityHeaders(w http.ResponseWriter, sh *scopedHeaders) http.ResponseWriter {
	if sh.empty() {
		return w
	}
	return &securityHeaderWriter{ResponseWriter: w, generated: sh.generated, proxied: sh.proxied}
}

// withHostResponseHeaders routes a proxy host's HSTS and X-Robots-Tag through the
// dispatch writer (reusing an existing one, else wrapping) so they are emitted at
// the FINAL WriteHeader and survive a 1xx interim response's header-map clear.
// hsts is passed empty on the plaintext listener (HSTS is HTTPS-only). Returns w
// unchanged when the host emits neither.
func withHostResponseHeaders(w http.ResponseWriter, hsts, robots string) http.ResponseWriter {
	if hsts == "" && robots == "" {
		return w
	}
	sw, ok := w.(*securityHeaderWriter)
	if !ok {
		sw = &securityHeaderWriter{ResponseWriter: w}
	}
	if hsts != "" {
		sw.hsts = hsts
	}
	if robots != "" {
		sw.robots = robots
	}
	return sw
}
