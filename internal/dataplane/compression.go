package dataplane

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// gzipWriterPool recycles *gzip.Writer values (Reset onto the real destination
// on Get, Reset onto io.Discard before Put so the pool holds no reference to a
// finished request's connection).
var gzipWriterPool = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

// acceptsGzip reports whether the client's Accept-Encoding lists gzip (or "*")
// as acceptable, per RFC 7231 (quality values and "identity;q=0" refinements
// are not evaluated - a bare token match is enough for this on/off decision).
func acceptsGzip(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept-Encoding") {
		for _, tok := range strings.Split(v, ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(tok), ";")
			if strings.EqualFold(name, "gzip") || name == "*" {
				return true
			}
		}
	}
	return false
}

// baseContentType strips any ";charset=..." (or other) parameters and
// lower-cases the media type, so "text/html; charset=utf-8" matches a
// configured "text/html" entry.
func baseContentType(ct string) string {
	base, _, _ := strings.Cut(ct, ";")
	return strings.ToLower(strings.TrimSpace(base))
}

// compressionHandler wraps next, gzip-compressing eligible responses per spec:
// honours the client's Accept-Encoding, skips a response the upstream already
// encoded, skips a non-matching Content-Type, buffers up to minBytes before
// deciding (so a body that never reaches it is sent uncompressed), sets Vary:
// Accept-Encoding on a compressed response, strips Content-Length when
// compressing, and never touches a HEAD request, a 204/304/101 response, or a
// response that starts streaming (an early Flush, before the decision is
// forced) - which also covers text/event-stream and WebSocket upgrades.
//
// BREACH: compressing a response whose size depends on attacker-controlled
// input reflected alongside a secret (e.g. a CSRF token) can leak that secret
// through the compressed size; that trade-off is the operator's call per host,
// which is why this is opt-in rather than default-on.
func compressionHandler(spec model.Compression, next http.Handler) http.Handler {
	minBytes := spec.EffectiveMinBytes()
	types := make(map[string]struct{})
	for _, t := range spec.EffectiveTypes() {
		if t = baseContentType(t); t != "" {
			types[t] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w, minBytes: minBytes, types: types}
		next.ServeHTTP(gw, r)
		gw.finish()
	})
}

// gzipResponseWriter buffers a response until it can decide whether to
// compress it (enough bytes seen, or the handler finished / flushed first).
type gzipResponseWriter struct {
	http.ResponseWriter
	minBytes int
	types    map[string]struct{}

	status      int
	wroteHeader bool
	buf         []byte
	decided     bool
	plain       bool // decided: pass through uncompressed
	gz          *gzip.Writer
}

func (gw *gzipResponseWriter) WriteHeader(status int) {
	if gw.wroteHeader {
		return
	}
	gw.status = status
	gw.wroteHeader = true
}

func (gw *gzipResponseWriter) Write(p []byte) (int, error) {
	if !gw.wroteHeader {
		gw.WriteHeader(http.StatusOK)
	}
	if gw.decided {
		if gw.plain {
			return gw.ResponseWriter.Write(p)
		}
		return gw.gz.Write(p)
	}
	gw.buf = append(gw.buf, p...)
	if len(gw.buf) >= gw.minBytes {
		gw.decide(true)
	}
	return len(p), nil
}

// Flush forces the compress/pass-through decision if it has not been made yet,
// treating an early flush (before minBytes is reached) as a streaming response
// that must never be buffered-and-maybe-compressed - see compressionHandler's
// doc comment.
func (gw *gzipResponseWriter) Flush() {
	if !gw.decided {
		gw.decide(false)
	}
	if !gw.plain && gw.gz != nil {
		_ = gw.gz.Flush()
	}
	if f, ok := gw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards a protocol-upgrade takeover (WebSocket and friends)
// untouched: gzip never applies to a hijacked connection.
func (gw *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := gw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	gw.decided = true
	gw.plain = true
	return hj.Hijack()
}

// Unwrap lets http.ResponseController (used by longLivedProxy to clear
// deadlines on upgrades/streamed bodies) reach the real underlying writer.
func (gw *gzipResponseWriter) Unwrap() http.ResponseWriter { return gw.ResponseWriter }

// finish decides (if nothing already forced it) once the handler has returned:
// a response that never reached minBytes is sent uncompressed, matching the
// "skip below minBytes" rule.
func (gw *gzipResponseWriter) finish() {
	if gw.decided {
		if !gw.plain && gw.gz != nil {
			_ = gw.gz.Close()
			gw.gz.Reset(io.Discard)
			gzipWriterPool.Put(gw.gz)
		}
		return
	}
	if !gw.wroteHeader {
		return // nothing was ever written; leave the real writer's own default alone
	}
	gw.decide(false)
}

// decide commits to compressing or passing the response through plain, writes
// the (now final) status and headers to the real ResponseWriter, and flushes
// whatever was buffered so far down the chosen path. sizeEligible is true only
// when the buffered/seen body has already reached minBytes.
func (gw *gzipResponseWriter) decide(sizeEligible bool) {
	gw.decided = true
	ct := baseContentType(gw.Header().Get("Content-Type"))
	_, typeOK := gw.types[ct]
	eligible := sizeEligible && typeOK &&
		ct != "text/event-stream" &&
		gw.Header().Get("Content-Encoding") == "" &&
		gw.status != http.StatusNoContent &&
		gw.status != http.StatusNotModified &&
		gw.status != http.StatusSwitchingProtocols
	if !eligible {
		gw.plain = true
		gw.ResponseWriter.WriteHeader(gw.status)
		if len(gw.buf) > 0 {
			_, _ = gw.ResponseWriter.Write(gw.buf)
		}
		gw.buf = nil
		return
	}
	gw.Header().Del("Content-Length")
	gw.Header().Set("Content-Encoding", "gzip")
	gw.Header().Add("Vary", "Accept-Encoding")
	gw.ResponseWriter.WriteHeader(gw.status)
	gz, _ := gzipWriterPool.Get().(*gzip.Writer)
	gz.Reset(gw.ResponseWriter)
	gw.gz = gz
	if len(gw.buf) > 0 {
		_, _ = gz.Write(gw.buf)
	}
	gw.buf = nil
}
