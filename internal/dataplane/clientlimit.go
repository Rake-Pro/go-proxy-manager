package dataplane

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// clientLimitConfig is the compiled settings.clientLimits: how many requests
// one client address may have in progress at once, and the networks the limit
// does not apply to.
type clientLimitConfig struct {
	maxInFlight int
	allow       []*net.IPNet
}

// clientLimit holds the live config; nil (the default) means no limit, and the
// listeners then pay one atomic load per request.
var clientLimit atomic.Pointer[clientLimitConfig]

// SetClientLimits installs settings.clientLimits. nil or a zero maxInFlight
// turns the limit off. A change applies to the next request; requests already
// counted are released against the same table, so a change never leaves a
// count behind.
func SetClientLimits(c *model.ClientLimitsSettings) {
	if c == nil || c.MaxInFlight <= 0 {
		clientLimit.Store(nil)
		return
	}
	cfg := &clientLimitConfig{maxInFlight: c.MaxInFlight}
	for _, s := range c.AllowFrom {
		if n := parseNet(s); n != nil {
			cfg.allow = append(cfg.allow, n)
		}
	}
	clientLimit.Store(cfg)
}

// maxInFlightClients bounds the in-flight table. An entry exists only while
// its client has a request in progress, so the table holds one entry per
// distinct client address being served at this moment. A new address that
// finds it full is served uncounted (fail open): a full table must not refuse
// clients that hold nothing, which would make the limit a way to lock out
// everyone else.
const maxInFlightClients = 1 << 16

// inFlight counts the requests in progress per client key. uncounted is the
// number of requests served uncounted because the table was full, reported by
// a log line at most once per inFlightFullLogEvery.
var inFlight struct {
	mu        sync.Mutex
	n         map[string]int
	uncounted int
	lastLog   time.Time
}

const inFlightFullLogEvery = time.Minute

// acquireInFlight counts a request for key. counted reports whether a count
// was taken (and must be released); ok is false when key is at its limit.
func acquireInFlight(key string, limit int) (counted, ok bool) {
	inFlight.mu.Lock()
	defer inFlight.mu.Unlock()
	c := inFlight.n[key]
	if c >= limit {
		return false, false
	}
	if c == 0 && len(inFlight.n) >= maxInFlightClients {
		inFlight.uncounted++
		if now := time.Now(); now.Sub(inFlight.lastLog) >= inFlightFullLogEvery {
			log.Warn().Int("clients", len(inFlight.n)).Int("uncounted", inFlight.uncounted).
				Msg("clientLimits: in-flight table full, new client addresses served without a limit")
			inFlight.lastLog, inFlight.uncounted = now, 0
		}
		return false, true
	}
	if inFlight.n == nil {
		inFlight.n = map[string]int{}
	}
	inFlight.n[key] = c + 1
	return true, true
}

func releaseInFlight(key string) {
	inFlight.mu.Lock()
	defer inFlight.mu.Unlock()
	if c := inFlight.n[key]; c > 1 {
		inFlight.n[key] = c - 1
	} else {
		delete(inFlight.n, key)
	}
}

// limitInFlight wraps a listener's dispatch, so every request is counted
// whoever answers it (a proxied response, a 404, a redirect, an auth refusal).
// The count is released when dispatch returns, on every path including a
// panic; an upgraded connection is served inside dispatch, so it stays counted
// until the tunnel closes. The client key is the client address the matched
// host's gates compare (the fleet resolver for a request that matches no
// proxy host), with IPv6 grouped by /64 like the rate limiter.
func (s *Server) limitInFlight(dispatch http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := clientLimit.Load()
		if cfg == nil {
			dispatch(w, r)
			return
		}
		ip := s.clientIPOf(r)
		if ipInNets(ip, cfg.allow) {
			dispatch(w, r)
			return
		}
		key := nilIPRateLimitKey
		if ip != nil {
			key = ipBucketKey(ip)
		}
		counted, ok := acquireInFlight(key, cfg.maxInFlight)
		if !ok {
			s.refuseInFlight(w, r)
			return
		}
		if counted {
			defer releaseInFlight(key)
		}
		dispatch(w, r)
	}
}

// refuseInFlight answers a request over the per-client limit with 429, the
// rate limiter's status, through the matched host's error page and security
// headers when the Host names a proxy host.
func (s *Server) refuseInFlight(w http.ResponseWriter, r *http.Request) {
	var ep *compiledErrorPages
	host := ""
	if rt := s.current(); rt != nil {
		name := hostKey(r.Host)
		w = withSecurityHeaders(w, rt.securityHeadersFor(name))
		if hh, ok := rt.hosts[name]; ok {
			ep, host = hh.errorPages, hh.host
		}
	}
	w.Header().Set("Retry-After", "1")
	countDenial(r, "in-flight-limit")
	serveErrorPage(w, http.StatusTooManyRequests, ep, host, func() {
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
	})
}
