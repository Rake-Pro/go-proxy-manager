package dataplane

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/go-proxy-manager/internal/analytics"
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
	// Country is the ISO-3166-1 alpha-2 code of Client when a GeoIP database
	// is loaded; empty otherwise (or for private/unknown addresses).
	Country string `json:"country,omitempty"`
	// Source is "internal" for a private, loopback, link-local or configured
	// internal-network client (settings.analytics.internalNetworks), else
	// "external"; empty when no client address was resolved.
	Source string `json:"source,omitempty"`
}

// internalNets is the operator's extra internal networks, shared with the
// analytics collector (SetInternalNetworks installs the same list).
var internalNets atomic.Pointer[[]netip.Prefix]

// SetInternalNetworks installs settings.analytics.internalNetworks for the
// access log's source column. Entries that do not parse are ignored (settings
// validation rejects them before they get here).
func SetInternalNetworks(cidrs []string) {
	nets, _ := analytics.ParseNetworks(cidrs)
	internalNets.Store(&nets)
}

// sourceOf classifies a client address the way the analytics collector does.
func sourceOf(ip net.IP) string {
	if ip == nil {
		return ""
	}
	var extra []netip.Prefix
	if p := internalNets.Load(); p != nil {
		extra = *p
	}
	if analytics.IsInternalIP(ip, extra) {
		return analytics.SourceInternal
	}
	return analytics.SourceExternal
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

// clear drops every retained entry. Capture continues: the next request
// lands in an empty ring.
func (r *logRing) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = r.buf[:0]
	r.n = 0
}

// capacity returns the ring's current size under the lock (resize may run
// concurrently).
func (r *logRing) capacity() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cap
}

// resize changes the capacity live, keeping the newest entries that still fit
// (oldest first in the new backing slice, so the ring restarts unwrapped).
func (r *logRing) resize(capacity int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keep := len(r.buf)
	if keep > capacity {
		keep = capacity
	}
	nb := make([]AccessEntry, 0, keep)
	for i := keep - 1; i >= 0; i-- {
		nb = append(nb, r.buf[(r.n-1-i)%r.cap])
	}
	r.buf = nb
	r.n = keep
	r.cap = capacity
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

// handlerSwitch serves whichever of two prebuilt handler chains is active,
// chosen by one atomic pointer load per request. It exists so access logging
// can be toggled live without a permanently installed observer: while every
// observability toggle is off the plain chain serves - no wrapper, no
// allocations, no clock reads - and a toggle swaps the observed chain in for
// subsequent requests. An in-flight request finishes on the chain it started
// with, so it logs (or doesn't) consistently.
type handlerSwitch struct {
	plain    http.Handler
	observed http.Handler
	active   atomic.Pointer[http.Handler]
}

func newHandlerSwitch(plain, observed http.Handler, observe bool) *handlerSwitch {
	hs := &handlerSwitch{plain: plain, observed: observed}
	hs.set(observe)
	return hs
}

// set routes future requests through the observed (true) or plain (false) chain.
func (hs *handlerSwitch) set(observed bool) {
	h := &hs.plain
	if observed {
		h = &hs.observed
	}
	hs.active.Store(h)
}

func (hs *handlerSwitch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	(*hs.active.Load()).ServeHTTP(w, r)
}

// dataHandler builds one listener's swappable chain: the plain dispatch handler
// and its observed wrapper are constructed once, and SetAccessLog picks the
// active side at runtime.
func (s *Server) dataHandler(dispatch http.HandlerFunc) *handlerSwitch {
	return newHandlerSwitch(dispatch, s.observe(dispatch), s.observeActive())
}

// observe wraps next with request access logging, slow-request warnings, debug
// headers, and the /metrics counters, per the Server's toggles. It always
// wraps; whether a request pays for the observer at all is decided by the
// handlerSwitch each listener serves through, which keeps the plain chain
// active while every toggle is off - so the default configuration still has
// zero per-request overhead.
//
// It is also where a request is tagged with the ProxyHost NAME it matched, which
// the access-control tiers read back for their denial counters (see
// metricshook.go). Resolving it once here keeps the label bounded by config and
// keeps every middleware constructor's signature unchanged.
func (s *Server) observe(next http.Handler) http.Handler {
	mh := metricsHook()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// One snapshot per request: the toggle can flip mid-flight, and the ring
		// capture, log line, and slow-warn decision below must agree.
		accessLog := s.accessLog.Load()

		// The matched proxy host, resolved once: the debug headers and the metric
		// labels want the same answer, and lookup is a map hit.
		hostName := ""
		if s.debugHeaders || mh != nil || analyticsActive() {
			if rt := s.current(); rt != nil {
				if hh, ok := rt.lookup(r.Host); ok {
					hostName = hh.host
				}
			}
		}

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

		var body *countingBody
		if mh != nil {
			mh.RequestStarted()
			defer mh.RequestFinished()
			r = withHostName(r, hostName)
			if r.Body != nil && r.Body != http.NoBody {
				body = &countingBody{ReadCloser: r.Body}
				r.Body = body
			}
		}
		// The analytics note: one small allocation per observed request, only
		// while a collector is switched on. The tiers write the denial reason and
		// signature into it; the event below is built after the handler returns.
		sink := analyticsSink()
		var meta *reqMeta
		var origPath, origQuery string
		if sink != nil && sink.Active() {
			meta = &reqMeta{}
			r = withReqMeta(r, meta)
			// The classifier must see what the client sent, not what a rewrite or
			// strip-prefix middleware turned it into.
			origPath, origQuery = r.URL.Path, r.URL.RawQuery
		} else {
			sink = nil
		}

		obs := &responseObserver{ResponseWriter: w}
		next.ServeHTTP(obs, r)

		dur := time.Since(start)
		if mh != nil {
			status := obs.status
			if status == 0 {
				status = http.StatusOK
			}
			label := hostName
			if label == "" {
				label = unknownHostLabel
			}
			var in int64
			if body != nil {
				in = body.n
			}
			mh.HTTPRequest(label, r.Method, status, dur, in, obs.bytes)
			// A hijacked upgrade never writes a status through the observer, so
			// treat the hijack itself as the success signal alongside a 101.
			if isUpgradeRequest(r) && (obs.hijacked || status == http.StatusSwitchingProtocols) {
				mh.WebsocketUpgrade(label)
			}
		}

		slow := s.slowThreshold > 0 && dur >= s.slowThreshold
		if sink == nil && !accessLog && !slow {
			return
		}
		status := obs.status
		if status == 0 {
			status = http.StatusOK // handler wrote nothing / hijacked
		}
		// Derived once for the analytics event and the log line alike.
		clientIP := s.clientIPOf(r)

		if sink != nil {
			evHost := hostName
			if evHost == "" {
				evHost = unknownHostLabel
			}
			// Record never blocks: the collector queues the event (or drops it).
			// The event is stamped with the completion time, so a long-lived
			// connection counts in the minute it ended, never in a slot the
			// collector has already rolled past.
			sink.Record(analytics.Event{
				Time:      time.Now(),
				Host:      evHost,
				Method:    r.Method,
				Status:    status,
				BytesOut:  obs.bytes,
				DurMs:     dur.Milliseconds(),
				Client:    clientIP,
				Deny:      meta.deny,
				Signature: meta.sig,
				Path:      origPath,
				RawQuery:  origQuery,
				UserAgent: r.UserAgent(),
			})
		}
		if !accessLog && !slow {
			return
		}

		var clientStr, country, source string
		if ip := clientIP; ip != nil {
			clientStr = ip.String()
			source = sourceOf(ip)
			// Country for the log viewer and the log line, only while a GeoIP
			// database is loaded. The lookup is an in-memory mmdb walk (about a
			// microsecond) after the handler has finished writing.
			if cc, ok := currentGeoDB().Country(ip); ok {
				country = cc
			}
		}

		// Capture into the in-memory ring for the /api/logs viewer. Only when access
		// logging is on, so the default (off) path stays allocation-free.
		if accessLog && s.logBuf != nil {
			s.logBuf.add(AccessEntry{
				Time:    start,
				Method:  r.Method,
				Host:    hostOnly(r.Host),
				Path:    r.URL.Path,
				Status:  status,
				Bytes:   obs.bytes,
				DurMs:   dur.Milliseconds(),
				Client:  clientStr,
				Country: country,
				Source:  source,
			})
		}

		ev := log.Info()
		if slow && !accessLog {
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
		if country != "" {
			ev = ev.Str("country", country)
		}
		if source != "" {
			ev = ev.Str("source", source)
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

// clientIPOf is the client IP the log records: the one the dispatch derived from
// the matched host's trusted-proxy set (so the log agrees, byte for byte, with
// what the access list and the rate limiter compared), or - for a request that
// matched no proxy host, e.g. a 404, a redirect or a parked host - the
// fleet-wide resolver, and finally the connection peer.
func (s *Server) clientIPOf(r *http.Request) net.IP {
	if info, ok := clientIPInfoOf(r); ok {
		return info.ip
	}
	rt := s.current()
	if rt == nil {
		return peerIP(r)
	}
	// The observer wraps the dispatch, so it holds the request as it ARRIVED,
	// before dispatch stashed the derived value on a child context. Re-derive it
	// from the same host's set with the same (pure) function, so the logged
	// address is identical to the one every gate compared.
	if hh, ok := rt.lookup(r.Host); ok {
		return deriveClientIP(r, hh.trustedProxies).ip
	}
	if rt.clientIP != nil {
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
