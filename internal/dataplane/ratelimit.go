package dataplane

import (
	"container/list"
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
// bound. At the cap the least-recently-used bucket is evicted in O(1) (see
// evictLRULocked); the LRU bucket is also the one idle longest, so eviction
// still targets the buckets nearest a full refill and never makes the limit
// stricter for an active client.
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

// lruBucket is one map/list node: the bucket plus its key (kept so the front
// element can be deleted from the map on eviction).
type lruBucket struct {
	key    string
	bucket tokenBucket
}

// rateLimiter is a per-client-IP token-bucket limiter. Each instance is scoped to
// one host's middleware (one buildChain wrap), so keying by client IP within it
// yields the per-host, per-client-IP semantics. capacity is the burst size and
// refill is the steady-state requests-per-second. Buckets are held in an LRU
// (order, front = least-recently-used) so eviction at the cap is O(1) rather than
// a full-map scan under a high-cardinality flood.
type rateLimiter struct {
	capacity float64
	refill   float64 // tokens per second
	now      func() time.Time

	mu      sync.Mutex
	buckets map[string]*list.Element // key -> *list.Element holding *lruBucket
	order   *list.List               // *lruBucket, front = LRU, back = MRU
}

func newRateLimiter(rl model.RateLimitMiddleware) *rateLimiter {
	rate, defaultBurst := rl.RateAndDefaultBurst()
	burst := rl.Burst
	if burst <= 0 {
		burst = defaultBurst
	}
	if burst < 1 {
		burst = 1
	}
	return &rateLimiter{
		capacity: float64(burst),
		refill:   rate,
		now:      time.Now,
		buckets:  map[string]*list.Element{},
		order:    list.New(),
	}
}

// allow reports whether a request from key may proceed, consuming a token when it
// can, and returns the seconds a denied caller should wait before retrying.
func (l *rateLimiter) allow(key string) (bool, int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	var b *tokenBucket
	if el := l.buckets[key]; el != nil {
		lb := el.Value.(*lruBucket)
		b = &lb.bucket
		if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
			b.tokens = math.Min(l.capacity, b.tokens+elapsed*l.refill)
			b.last = now
		}
		l.order.MoveToBack(el) // mark most-recently-used
	} else {
		if l.order.Len() >= maxRateLimitBuckets {
			l.evictLRULocked()
		}
		lb := &lruBucket{key: key, bucket: tokenBucket{tokens: l.capacity, last: now}}
		l.buckets[key] = l.order.PushBack(lb)
		b = &lb.bucket
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	retry := (1 - b.tokens) / l.refill // time until the next whole token
	return false, int(math.Ceil(retry))
}

// evictLRULocked drops the least-recently-used bucket in O(1). That bucket is the
// one idle longest, hence nearest a full refill, so removing it never makes the
// limit stricter for an active client - while a rotating-IP flood pays only O(1)
// per new key instead of an O(n) scan. Caller holds l.mu.
func (l *rateLimiter) evictLRULocked() {
	el := l.order.Front()
	if el == nil {
		return
	}
	l.order.Remove(el)
	delete(l.buckets, el.Value.(*lruBucket).key)
}

// rateLimitHandler enforces a per-client-IP token-bucket limit. On exhaustion it
// replies 429 with a Retry-After header and does not call next. The client IP is
// resolved via ipOf (the registry's shared, XFF-aware resolver); an unresolvable
// IP falls back to the shared nil bucket. A client matching rl.AllowFrom bypasses
// the limiter entirely (no token consumed, no 429); a nil/unresolvable IP never
// matches, so it falls through to the shared bucket as before.
func rateLimitHandler(rl model.RateLimitMiddleware, ipOf func(*http.Request) net.IP, next http.Handler) http.Handler {
	l := newRateLimiter(rl)
	var allowNets []*net.IPNet
	for _, c := range rl.AllowFrom {
		if n := parseNet(c); n != nil {
			allowNets = append(allowNets, n)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ipOf(r)
		if ipInNets(ip, allowNets) {
			next.ServeHTTP(w, r)
			return
		}
		key := nilIPRateLimitKey
		if ip != nil {
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
