package dataplane

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"sync/atomic"
)

// globalSecurityHeaders holds the compiled settings-level default security
// headers, installed once per config reload via SetSecurityHeaders and read by
// buildRouter (to compose the per-host set) and by the router directly (for
// host-less responses: the no-such-host 404, and redirect/parked hosts, which
// carry no per-host override). It mirrors globalErrorPages' package-level
// handle: threading Settings through buildRouter for a value that only ever
// changes alongside a full reload would touch far more of the data plane.
var globalSecurityHeaders atomic.Pointer[http.Header]

// SetSecurityHeaders compiles and installs the settings-level default response
// headers. It is called before the data-plane Reload so the header set is in
// place before any request is served. The map has already passed
// model.Settings.Validate at config-write time; compile canonicalizes the names
// and defensively drops anything a valid config could not contain (an empty
// name or Strict-Transport-Security, which the per-host hsts setting owns).
func SetSecurityHeaders(m map[string]string) {
	h := compileSecurityHeaders(m)
	globalSecurityHeaders.Store(&h)
}

func currentSecurityHeaders() http.Header {
	if p := globalSecurityHeaders.Load(); p != nil {
		return *p
	}
	return nil
}

// compileSecurityHeaders canonicalizes m into an http.Header, or returns nil
// when it configures nothing. Model validation is authoritative; this only
// canonicalizes and skips the two entries a bypassed config might still carry.
func compileSecurityHeaders(m map[string]string) http.Header {
	if len(m) == 0 {
		return nil
	}
	h := make(http.Header, len(m))
	for k, v := range m {
		ck := textproto.CanonicalMIMEHeaderKey(k)
		if ck == "" || strings.EqualFold(ck, "Strict-Transport-Security") {
			continue
		}
		h.Set(ck, v)
	}
	if len(h) == 0 {
		return nil
	}
	return h
}

// mergedSecurityHeaders composes the effective header set for one proxy host:
// the settings-level default with the host's own override merged over it per
// key (the host value wins for a header it names; a header it omits falls
// through to the settings default). Returns nil when neither configures
// anything, so an unconfigured host takes the zero-overhead unwrapped path.
func mergedSecurityHeaders(override map[string]string) http.Header {
	base := currentSecurityHeaders()
	ov := compileSecurityHeaders(override)
	if len(base) == 0 && len(ov) == 0 {
		return nil
	}
	out := make(http.Header, len(base)+len(ov))
	for k, vs := range base {
		out[k] = append([]string(nil), vs...)
	}
	for k, vs := range ov {
		out[k] = append([]string(nil), vs...) // host override replaces this key
	}
	return out
}

// securityHeaderWriter injects gpm's configured response headers on the way out,
// set-if-absent: a header already present (set by gpm's own generated response
// leaves it absent, so it is added; a proxied upstream response has its own
// value copied in before WriteHeader, so it is preserved). This is the same
// response layer HSTS is emitted at, so the headers land on every gpm-generated
// denial/redirect/error page regardless of the auth outcome, while never
// clobbering a backed app's own security headers. It forwards the optional
// interfaces the reverse proxy relies on (Flusher, Hijacker, Unwrap), exactly
// like responseObserver.
type securityHeaderWriter struct {
	http.ResponseWriter
	// headers is the configured security-header set, injected set-if-absent. It
	// is the per-host merged map (shared, read-only - never mutated here).
	headers http.Header
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
	for k, vs := range s.headers {
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
	// responses (>= 200) inject and latch.
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
func withSecurityHeaders(w http.ResponseWriter, headers http.Header) http.ResponseWriter {
	if len(headers) == 0 {
		return w
	}
	return &securityHeaderWriter{ResponseWriter: w, headers: headers}
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
