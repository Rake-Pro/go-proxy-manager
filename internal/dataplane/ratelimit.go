package dataplane

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// maxRateLimitBuckets caps how many distinct client buckets a single rate-limit
// middleware tracks, so a flood of unique source IPs cannot grow the map without
// bound. At the cap, idle (fully refilled) buckets are evicted first, falling
// back to the single oldest bucket to guarantee forward progress.
const maxRateLimitBuckets = 16384

// nilIPRateLimitKey is the shared bucket used when the client IP cannot be
// resolved. Such requests fail safe onto one shared bucket: a keyless peer can
// neither bypass the limit by appearing identity-less nor spray unbounded
// distinct keys. The NUL prefix cannot collide with any real net.IP.String().
const nilIPRateLimitKey = "\x00nil"

type tokenBucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter is a per-client-IP token-bucket limiter. Each instance is scoped to
// one host's middleware (one buildChain wrap), so keying by client IP within it
// yields the per-host, per-client-IP semantics. capacity is the burst size and
// refill is the steady-state requests-per-second.
type rateLimiter struct {
	capacity float64
	refill   float64 // tokens per second
	now      func() time.Time

	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

func newRateLimiter(rl model.RateLimitMiddleware) *rateLimiter {
	burst := rl.Burst
	if burst <= 0 {
		burst = int(math.Ceil(rl.RequestsPerSecond))
	}
	if burst < 1 {
		burst = 1
	}
	return &rateLimiter{
		capacity: float64(burst),
		refill:   rl.RequestsPerSecond,
		now:      time.Now,
		buckets:  map[string]*tokenBucket{},
	}
}

// allow reports whether a request from key may proceed, consuming a token when it
// can, and returns the seconds a denied caller should wait before retrying.
func (l *rateLimiter) allow(key string) (bool, int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.buckets[key]
	if b == nil {
		if len(l.buckets) >= maxRateLimitBuckets {
			l.evictLocked(now)
		}
		b = &tokenBucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	} else if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(l.capacity, b.tokens+elapsed*l.refill)
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	retry := (1 - b.tokens) / l.refill // time until the next whole token
	return false, int(math.Ceil(retry))
}

// evictLocked makes room in the bucket map. It drops every bucket that has fully
// refilled (idle long enough to be indistinguishable from a fresh full one, so
// removing it never makes the limit stricter); if none qualify it evicts the
// single oldest bucket. Caller holds l.mu.
func (l *rateLimiter) evictLocked(now time.Time) {
	var oldestKey string
	var oldest time.Time
	for k, b := range l.buckets {
		refilled := math.Min(l.capacity, b.tokens+now.Sub(b.last).Seconds()*l.refill)
		if refilled >= l.capacity {
			delete(l.buckets, k)
			continue
		}
		if oldestKey == "" || b.last.Before(oldest) {
			oldestKey, oldest = k, b.last
		}
	}
	if len(l.buckets) >= maxRateLimitBuckets && oldestKey != "" {
		delete(l.buckets, oldestKey)
	}
}

// rateLimitHandler enforces a per-client-IP token-bucket limit. On exhaustion it
// replies 429 with a Retry-After header and does not call next. The client IP is
// resolved via ipOf (the registry's shared, XFF-aware resolver); an unresolvable
// IP falls back to the shared nil bucket.
func rateLimitHandler(rl model.RateLimitMiddleware, ipOf func(*http.Request) net.IP, next http.Handler) http.Handler {
	l := newRateLimiter(rl)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := nilIPRateLimitKey
		if ip := ipOf(r); ip != nil {
			key = ip.String()
		}
		ok, retry := l.allow(key)
		if !ok {
			if retry < 1 {
				retry = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
