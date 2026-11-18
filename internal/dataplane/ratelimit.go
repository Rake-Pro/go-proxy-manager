package dataplane

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
	"weak"

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
	tokens       float64
	last         time.Time
	blockedUntil time.Time     // zero if never blocked
	lastBlock    time.Duration // length of the most recent block, for backoff and forgetting
	strikes      int           // blocks served so far in the current streak
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
	refill   float64       // tokens per second
	blockFor time.Duration // 0 disables the extra block period
	backoff  float64       // > 1 multiplies repeat blocks; 0/1 keeps them fixed
	blockMax time.Duration // cap for a backed-off block
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
	var blockFor time.Duration
	if rl.BlockFor != "" {
		if d, err := time.ParseDuration(rl.BlockFor); err == nil && d > 0 {
			blockFor = d
		}
	}
	var backoff float64
	blockMax := blockFor
	if blockFor > 0 && rl.BlockBackoff > 1 {
		backoff = rl.BlockBackoff
		blockMax = model.DefaultBlockMax
		if rl.BlockMax != "" {
			if d, err := time.ParseDuration(rl.BlockMax); err == nil && d >= blockFor {
				blockMax = d
			}
		}
	}
	return &rateLimiter{
		capacity: float64(burst),
		refill:   rate,
		blockFor: blockFor,
		backoff:  backoff,
		blockMax: blockMax,
		now:      time.Now,
		buckets:  map[string]*list.Element{},
		order:    list.New(),
	}
}

// allow reports whether a request from key may proceed, consuming a token when it
// can, and returns the seconds a denied caller should wait before retrying. When
// blockFor is set, exceeding the limit blocks the key for a fixed period rather
// than the usual "wait for the next token": further requests during the block
// are denied outright, and the block does not extend on repeat hits. With
// backoff set, each block a key earns after the previous one ended lasts
// backoff times longer (capped at blockMax); the streak is forgotten once the
// key has gone one full block length past the end of its last block without
// tripping again (see nextBlock). A blocked key can still be evicted by the LRU
// cap like any other idle bucket, which also forgets its streak.
func (l *rateLimiter) allow(key string) (bool, int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	var b *tokenBucket
	if el := l.buckets[key]; el != nil {
		lb := el.Value.(*lruBucket)
		b = &lb.bucket
		l.order.MoveToBack(el) // mark most-recently-used

		if l.blockFor > 0 && now.Before(b.blockedUntil) {
			retry := b.blockedUntil.Sub(now).Seconds()
			return false, int(math.Ceil(retry))
		}
		if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
			b.tokens = math.Min(l.capacity, b.tokens+elapsed*l.refill)
			b.last = now
		}
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
	if l.blockFor > 0 {
		d := l.nextBlock(b, now)
		b.blockedUntil = now.Add(d)
		b.lastBlock = d
		b.strikes++
		return false, int(math.Ceil(d.Seconds()))
	}
	retry := (1 - b.tokens) / l.refill // time until the next whole token
	return false, int(math.Ceil(retry))
}

// nextBlock returns how long the block starting now should last. Without
// backoff it is always blockFor. With backoff the streak is first forgotten if
// the key stayed clean for one full lastBlock after its previous block ended,
// then the length is blockFor*backoff^strikes, capped at blockMax.
func (l *rateLimiter) nextBlock(b *tokenBucket, now time.Time) time.Duration {
	if l.backoff <= 1 {
		return l.blockFor
	}
	if b.strikes > 0 && !now.Before(b.blockedUntil.Add(b.lastBlock)) {
		b.strikes = 0
	}
	d := time.Duration(float64(l.blockFor) * math.Pow(l.backoff, float64(b.strikes)))
	if d <= 0 || d > l.blockMax {
		d = l.blockMax
	}
	return d
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
// matches, so it falls through to the shared bucket as before. An "@<name>"
// entry resolves through listNets to the networks that access list allows
// (nil listNets, or an unknown name, contributes nothing). host and ep resolve
// the custom error page for the 429 (see serveErrorPage).
func rateLimitHandler(rl model.RateLimitMiddleware, ipOf func(*http.Request) net.IP, host string, ep *compiledErrorPages, next http.Handler, listNets func(name string) []*net.IPNet) http.Handler {
	return rateLimitHandlerWith(newRateLimiter(rl), rl, ipOf, host, ep, next, listNets)
}

// rateLimitHandlerWith is rateLimitHandler enforcing rl through the given
// limiter, which may carry bucket state from an earlier build (see chainStates).
func rateLimitHandlerWith(l *rateLimiter, rl model.RateLimitMiddleware, ipOf func(*http.Request) net.IP, host string, ep *compiledErrorPages, next http.Handler, listNets func(name string) []*net.IPNet) http.Handler {
	var allowNets []*net.IPNet
	for _, c := range rl.AllowFrom {
		if name, ok := model.AccessListRef(c); ok {
			if listNets != nil {
				allowNets = append(allowNets, listNets(name)...)
			}
			continue
		}
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
			key = ipBucketKey(ip)
		}
		ok, retry := l.allow(key)
		if !ok {
			if retry < 1 {
				retry = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			countDenial(r, "rate-limit")
			serveErrorPage(w, http.StatusTooManyRequests, ep, host, func() {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// chainStates holds the rate-limiter and basic-auth failure-gate instances of
// the compiled chains, keyed by owner (host, route, middleware) plus a hash of
// the spec that built them. A router build asks it for each instance, so a
// rebuilt chain whose spec is unchanged keeps the live buckets, blocks and
// lockouts instead of starting empty, and the routes of one host share one
// basic-auth failure gate. Entries are weak: an instance lives exactly as long
// as a compiled chain (the live router, or a build in progress) refers to it,
// so a removed host or a changed spec leaves nothing behind once the old router
// is gone, and a build that fails or is only a trial drops nothing the live
// router uses.
type chainStates struct {
	mu       sync.Mutex
	limiters weakMap[rateLimiter]
	gates    weakMap[authGate]
}

func (c *chainStates) limiter(key string, mk func() *rateLimiter) *rateLimiter {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.limiters.get(key, mk)
}

func (c *chainStates) gate(key string, mk func() *authGate) *authGate {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gates.get(key, mk)
}

// weakMap is a string-keyed map of weak pointers with amortized removal of
// collected entries. The caller serializes access.
type weakMap[T any] struct {
	m       map[string]weak.Pointer[T]
	sweepAt int
}

func (w *weakMap[T]) get(key string, mk func() *T) *T {
	if v := w.m[key].Value(); v != nil {
		return v
	}
	if w.m == nil {
		w.m = map[string]weak.Pointer[T]{}
	}
	if len(w.m) >= w.sweepAt {
		for k, p := range w.m {
			if p.Value() == nil {
				delete(w.m, k)
			}
		}
		w.sweepAt = 2*len(w.m) + 64
	}
	v := mk()
	w.m[key] = weak.Make(v)
	return v
}

// serverChainStates scopes chainStates to a data-plane Server across reloads.
// The Server's upstream-group health manager is the state that already
// survives its reloads and reaches every build (as the healthStage passed to
// buildRouter), so it identifies the Server here. Keys are weak, so a Server
// that is gone takes its entry with it.
var serverChainStates struct {
	mu sync.Mutex
	m  map[weak.Pointer[healthManager]]*chainStates
}

// chainStatesFor returns the reload-surviving store of the Server whose health
// stage drives this build, or a store for this build alone when the build has
// none (a router built directly, as tests do).
func chainStatesFor(health groupResolver) *chainStates {
	st, ok := health.(*healthStage)
	if !ok || st == nil || st.m == nil {
		return &chainStates{}
	}
	serverChainStates.mu.Lock()
	defer serverChainStates.mu.Unlock()
	if serverChainStates.m == nil {
		serverChainStates.m = map[weak.Pointer[healthManager]]*chainStates{}
	}
	for k := range serverChainStates.m {
		if k.Value() == nil {
			delete(serverChainStates.m, k)
		}
	}
	key := weak.Make(st.m)
	cs := serverChainStates.m[key]
	if cs == nil {
		cs = &chainStates{}
		serverChainStates.m[key] = cs
	}
	return cs
}

// specKey is a stable digest of a middleware spec, so a changed spec gets a
// fresh limiter or gate rather than one built for different parameters. ok is
// false for a spec that does not encode; such an instance is never shared.
func specKey(spec any) (key string, ok bool) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), true
}
