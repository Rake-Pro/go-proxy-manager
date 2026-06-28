package dataplane

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// responseObserver wraps an http.ResponseWriter to capture the status code and
// body byte count for access logging, while transparently forwarding the
// optional interfaces the reverse proxy relies on: Flusher (streaming/SSE),
// Hijacker (websocket upgrades), and Unwrap (so http.ResponseController reaches
// the base writer for read/write deadlines). Without these, upgrades and
// streaming would break the moment access logging is enabled.
type responseObserver struct {
	http.ResponseWriter
	status   int
	bytes    int64
	wrote    bool
	hijacked bool
}

func (o *responseObserver) WriteHeader(code int) {
	if !o.wrote {
		o.status = code
		o.wrote = true
	}
	o.ResponseWriter.WriteHeader(code)
}

func (o *responseObserver) Write(b []byte) (int, error) {
	if !o.wrote {
		o.status = http.StatusOK
		o.wrote = true
	}
	n, err := o.ResponseWriter.Write(b)
	o.bytes += int64(n)
	return n, err
}

func (o *responseObserver) Flush() {
	if f, ok := o.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (o *responseObserver) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := o.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter is not an http.Hijacker")
	}
	o.hijacked = true
	return h.Hijack()
}

// Unwrap lets http.ResponseController (Go 1.20+) find the base writer.
func (o *responseObserver) Unwrap() http.ResponseWriter { return o.ResponseWriter }

// observe wraps next with request access logging, slow-request warnings, and
// debug headers, per the Server's toggles. When all three are off it returns next
// unwrapped so there is zero per-request overhead in the default configuration.
func (s *Server) observe(next http.Handler) http.Handler {
	if !s.accessLog && s.slowThreshold <= 0 && !s.debugHeaders {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var rid string
		if s.debugHeaders {
			rid = newRequestID()
			w.Header().Set("X-GPM-Request-Id", rid)
			// Headers must be set before the handler writes; resolve the route now.
			if rt := s.current(); rt != nil {
				if hh, ok := rt.lookup(r.Host); ok {
					w.Header().Set("X-GPM-Host", hh.host)
					if hh.upstream != "" {
						w.Header().Set("X-GPM-Upstream", hh.upstream)
					}
				}
			}
		}

		obs := &responseObserver{ResponseWriter: w}
		next.ServeHTTP(obs, r)

		dur := time.Since(start)
		slow := s.slowThreshold > 0 && dur >= s.slowThreshold
		if !s.accessLog && !slow {
			return
		}
		status := obs.status
		if status == 0 {
			status = http.StatusOK // handler wrote nothing / hijacked
		}

		ev := log.Info()
		if slow && !s.accessLog {
			ev = log.Warn()
		}
		ev = ev.Str("component", "access").
			Str("method", r.Method).
			Str("host", hostOnly(r.Host)).
			Str("path", r.URL.Path).
			Int("status", status).
			Int64("bytes", obs.bytes).
			Dur("dur", dur).
			Str("peer", peerAddr(r))
		if ip := s.clientIPOf(r); ip != nil {
			ev = ev.Str("client", ip.String())
		}
		if rid != "" {
			ev = ev.Str("rid", rid)
		}
		if obs.hijacked {
			ev = ev.Bool("hijacked", true)
		}
		if slow {
			ev = ev.Bool("slow", true)
		}
		ev.Msg("access")
	})
}

// clientIPOf resolves the real client IP using the live router's XFF-aware
// resolver, falling back to the connection peer.
func (s *Server) clientIPOf(r *http.Request) net.IP {
	if rt := s.current(); rt != nil && rt.clientIP != nil {
		if ip := rt.clientIP(r); ip != nil {
			return ip
		}
	}
	return peerIP(r)
}

func peerAddr(r *http.Request) string {
	if ip := peerIP(r); ip != nil {
		return ip.String()
	}
	return r.RemoteAddr
}

func hostOnly(h string) string {
	if i := strings.IndexByte(h, ':'); i >= 0 {
		return h[:i]
	}
	return h
}

// newRequestID returns a short random hex id for correlating a client-visible
// X-GPM-Request-Id header with its access-log line.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}
