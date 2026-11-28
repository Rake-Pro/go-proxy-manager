package dataplane

import (
	"container/heap"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"
	"weak"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// maxRateLimitBuckets caps how many distinct client buckets a single rate-limit
// middleware tracks, so a flood of unique source IPs cannot grow the map without
// bound. At the cap one bucket is evicted (see evictLocked). While at most half
// the tracked buckets are blocked, that is the least-recently-used unblocked
// bucket, which restarts full, so eviction never makes the limit stricter for
// an active client and new clients that stay within their own limit never lift
// a block. While more than half are blocked, it is the running block that ends
// soonest, so blocked buckets cannot crowd unblocked clients out of the table.
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

// lruBucket is one tracked client: the bucket plus its key (kept so an evicted
// node can be deleted from the map). It is in exactly one of the two
// structures: the order list (el set, heapIdx -1) or the blocked heap (el nil,
// heapIdx its position there). seq orders blocks that end at the same instant.
type lruBucket struct {
	key     string
	bucket  tokenBucket
	el      *list.Element
	heapIdx int
	seq     uint64
}

// blockHeap is a min-heap of blocked buckets by blockedUntil, ties by seq (the
// block started first ends first). It keeps each node's heapIdx current.
type blockHeap []*lruBucket

func (h blockHeap) Len() int { return len(h) }
func (h blockHeap) Less(i, j int) bool {
	a, b := h[i].bucket.blockedUntil, h[j].bucket.blockedUntil
	if !a.Equal(b) {
		return a.Before(b)
	}
	return h[i].seq < h[j].seq
}
func (h blockHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIdx = i
	h[j].heapIdx = j
}
func (h *blockHeap) Push(x any) {
	lb := x.(*lruBucket)
	lb.heapIdx = len(*h)
	*h = append(*h, lb)
}
func (h *blockHeap) Pop() any {
	old := *h
	lb := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	lb.heapIdx = -1
	return lb
}

// rateLimiter is a per-client-IP token-bucket limiter. Each instance is scoped to
// one host's middleware (one buildChain wrap), so keying by client IP within it
// yields the per-host, per-client-IP semantics. capacity is the burst size and
// refill is the steady-state requests-per-second. Unblocked buckets are held in
// an LRU (order, front = least-recently-used); a bucket moves to the blocked
// min-heap when a block starts and back to order when its block is over, on its
// own next request or when an eviction finds it at the heap top. Cost: O(1) per
// request, O(log n) when a block starts or ends and when a blocked bucket is
// evicted, never a scan of the table.
type rateLimiter struct {
	capacity float64
	refill   float64       // tokens per second
	blockFor time.Duration // 0 disables the extra block period
	backoff  float64       // > 1 multiplies repeat blocks; 0/1 keeps them fixed
	blockMax time.Duration // cap for a backed-off block
	now      func() time.Time

	mu         sync.Mutex
	maxBuckets int                   // maxRateLimitBuckets; a field so tests can use small tables
	buckets    map[string]*lruBucket // every tracked client, in order or in blocked
	order      *list.List            // unblocked *lruBucket, front = LRU, back = MRU
	blocked    blockHeap             // blocked *lruBucket, top = block ending soonest
	blockSeq   uint64                // last seq handed out to a block
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
		capacity:   float64(burst),
		refill:     rate,
		blockFor:   blockFor,
		backoff:    backoff,
		blockMax:   blockMax,
		now:        time.Now,
		maxBuckets: maxRateLimitBuckets,
		buckets:    map[string]*lruBucket{},
		order:      list.New(),
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
// tripping again (see nextBlock). An eviction at the cap forgets the bucket,
// block and streak included; see evictLocked for which bucket that is.
func (l *rateLimiter) allow(key string) (bool, int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	lb := l.buckets[key]
	if lb != nil {
		b := &lb.bucket
		if l.blockFor > 0 && now.Before(b.blockedUntil) {
			// A hit during the block changes neither structure.
			retry := b.blockedUntil.Sub(now).Seconds()
			return false, int(math.Ceil(retry))
		}
		if lb.heapIdx >= 0 {
			heap.Remove(&l.blocked, lb.heapIdx)
			lb.el = l.order.PushBack(lb)
		} else {
			l.order.MoveToBack(lb.el) // mark most-recently-used
		}
		if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
			b.tokens = math.Min(l.capacity, b.tokens+elapsed*l.refill)
			b.last = now
		}
	} else {
		if len(l.buckets) >= l.maxBuckets {
			l.evictLocked(now)
		}
		lb = &lruBucket{key: key, bucket: tokenBucket{tokens: l.capacity, last: now}, heapIdx: -1}
		lb.el = l.order.PushBack(lb)
		l.buckets[key] = lb
	}
	b := &lb.bucket

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	if l.blockFor > 0 {
		d := l.nextBlock(b, now)
		b.blockedUntil = now.Add(d)
		b.lastBlock = d
		b.strikes++
		// Every path here left the node in order; it moves to the heap.
		l.order.Remove(lb.el)
		lb.el = nil
		l.blockSeq++
		lb.seq = l.blockSeq
		heap.Push(&l.blocked, lb)
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

// evictLocked drops one bucket to make room for a new key. First every bucket
// at the heap top whose block has ended rejoins order at the most-recently-used
// end, earliest end first: a block end counts as recent activity, so the
// bucket and its backoff streak are kept as long as any active client's
// bucket. Each bucket moves at most once per block, so this costs O(log n) per
// block. Then:
//
//   - While more than half the table is blocked, the running block that ends
//     soonest is dropped, in O(log n): the least block time is lost, and a
//     repeat offender's long backed-off block outlives fresh short ones. An
//     unblocked bucket is therefore dropped only while at least half the table
//     is unblocked, so a table full of blocks cannot turn every new client
//     into a fresh, full bucket.
//   - Otherwise the least-recently-used unblocked bucket is dropped, in O(1).
//     It is the one idle longest, hence nearest a full refill, so removing it
//     never makes the limit stricter for an active client.
//
// So new clients that stay within their own limit never lift a block while at
// most half the table is blocked; clients that each get themselves blocked can
// push the blocked share past half, and then blocks are lifted early, the one
// ending soonest first. One more consequence of ended blocks rejoining as most
// recently used: a client that last sent while almost the whole table was
// blocked can be the next bucket dropped once those blocks end together. Like
// every eviction that only restarts it with a full bucket. Caller holds l.mu.
func (l *rateLimiter) evictLocked(now time.Time) {
	for len(l.blocked) > 0 && !l.blocked[0].bucket.blockedUntil.After(now) {
		lb := heap.Pop(&l.blocked).(*lruBucket)
		lb.el = l.order.PushBack(lb)
	}
	// The map is at the cap here, so an empty order means a full heap, which is
	// more than half for any cap >= 1: the order and heap checks below are
	// defensive only.
	if len(l.blocked) > l.maxBuckets/2 || l.order.Len() == 0 {
		if len(l.blocked) == 0 {
			return
		}
		lb := heap.Pop(&l.blocked).(*lruBucket)
		delete(l.buckets, lb.key)
		return
	}
	lb := l.order.Remove(l.order.Front()).(*lruBucket)
	lb.el = nil
	delete(l.buckets, lb.key)
}

// rateLimitHandlerWith enforces a per-client-IP token-bucket limit through l,
// which may carry bucket state from an earlier build (see chainStates). On
// exhaustion it replies 429 with a Retry-After header and does not call next.
// The client IP is resolved via ipOf (the registry's shared, XFF-aware
// resolver); an unresolvable IP falls back to the shared nil bucket, except for
// a peer address with a zone, which gets one per zone (see zonedPeerKey). A client
// matching rl.AllowFrom bypasses the limiter entirely (no token consumed, no
// 429); a nil/unresolvable IP never matches, so it falls through to the shared
// bucket as before. An "@<name>" entry is the predicate listExempt returns for
// that list (see registry.rateLimitExemption); nil listExempt, or a nil
// predicate for an unknown name, exempts nobody. An IPv6 client is keyed by its
// rl.IPv6Prefix. host and ep resolve the custom error page for the 429 (see
// serveErrorPage).
func rateLimitHandlerWith(l *rateLimiter, rl model.RateLimitMiddleware, ipOf func(*http.Request) net.IP, host string, ep *compiledErrorPages, next http.Handler, listExempt func(name string) func(net.IP) bool) http.Handler {
	var allowNets []*net.IPNet
	var listExempts []func(net.IP) bool
	for _, c := range rl.AllowFrom {
		if name, ok := model.AccessListRef(c); ok {
			if listExempt != nil {
				if f := listExempt(name); f != nil {
					listExempts = append(listExempts, f)
				}
			}
			continue
		}
		if n := parseNet(c); n != nil {
			allowNets = append(allowNets, n)
		}
	}
	v6Bits := rl.IPv6Prefix
	if v6Bits == 0 {
		v6Bits = model.DefaultRateLimitIPv6Prefix
	}
	v6Mask := net.CIDRMask(v6Bits, 128)
	v6Suffix := "/" + strconv.Itoa(v6Bits)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ipOf(r)
		if ipInNets(ip, allowNets) {
			next.ServeHTTP(w, r)
			return
		}
		if ip != nil {
			for _, exempt := range listExempts {
				if exempt(ip) {
					next.ServeHTTP(w, r)
					return
				}
			}
		}
		key := nilIPRateLimitKey
		if ip != nil {
			// IPv6 clients share one bucket per ipv6Prefix (default /64, the
			// same key as ipBucketKey); IPv4 keys stay the address.
			key = ipBucketKey(ip)
			if ip.To4() == nil && v6Bits != model.DefaultRateLimitIPv6Prefix {
				key = ip.Mask(v6Mask).String() + v6Suffix
			}
		} else if zk, ok := zonedPeerKey(r); ok {
			key = zk
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

// zonedPeerKey is the bucket key for a connection peer whose address carries a
// zone ("[fe80::1%eth0]:port"). Such an address parses to no client IP, so
// without this every scoped peer would share nilIPRateLimitKey. The key is the
// zone alone, one bucket per interface: a scoped address is self-assigned
// anywhere in its range, so any part of it would let one peer rotate into
// fresh buckets (and evict other clients' buckets at the cap). It serves the
// rate limiter alone: the client IP stays nil for every other control, which
// denies such a peer. The NUL prefix keeps the key apart from every address
// key and from nilIPRateLimitKey.
func zonedPeerKey(r *http.Request) (string, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", false
	}
	a, err := netip.ParseAddr(host)
	if err != nil || a.Zone() == "" {
		return "", false
	}
	return "\x00zone%" + a.Zone(), true
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
