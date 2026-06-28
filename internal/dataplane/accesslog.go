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
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// AccessEntry is one captured request line, surfaced by the admin /api/logs
// endpoint for the in-UI access-log viewer.
type AccessEntry struct {
	Time   time.Time `json:"time"`
	Method string    `json:"method"`
	Host   string    `json:"host"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
	Bytes  int64     `json:"bytes"`
	DurMs  int64     `json:"durMs"`
	Client string    `json:"client"`
}

// logRing is a fixed-capacity, mutex-guarded ring buffer of recent access
// entries. It is written only while access logging is enabled (the default
// off-path never allocates or captures), so the memory cost is bounded and opt-in.
type logRing struct {
	mu  sync.Mutex
	buf []AccessEntry
	n   int // total entries ever written
	cap int
}

func newLogRing(capacity int) *logRing { return &logRing{cap: capacity} }

func (r *logRing) add(e AccessEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, e)
	} else {
		r.buf[r.n%r.cap] = e
	}
	r.n++
}

// recent returns up to cap entries, newest first.
func (r *logRing) recent() []AccessEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := len(r.buf)
	out := make([]AccessEntry, 0, count)
	for i := 0; i < count; i++ {
		// logical newest index is r.n-1; element j sits at buf[j%cap].
		out = append(out, r.buf[(r.n-1-i)%r.cap])
	}
	return out
}

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
					if _, up := hh.route(r.URL.Path); up != "" {
						w.Header().Set("X-GPM-Upstream", up)
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

		var clientStr string
		if ip := s.clientIPOf(r); ip != nil {
			clientStr = ip.String()
		}

		// Capture into the in-memory ring for the /api/logs viewer. Only when access
		// logging is on, so the default (off) path stays allocation-free.
		if s.accessLog && s.logBuf != nil {
			s.logBuf.add(AccessEntry{
				Time:   start,
				Method: r.Method,
				Host:   hostOnly(r.Host),
				Path:   r.URL.Path,
				Status: status,
				Bytes:  obs.bytes,
				DurMs:  dur.Milliseconds(),
				Client: clientStr,
			})
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
		if clientStr != "" {
			ev = ev.Str("client", clientStr)
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
