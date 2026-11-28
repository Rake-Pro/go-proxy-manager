package dataplane

import (
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func TestRateLimiterUnderLimitPasses(t *testing.T) {
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 5, Burst: 5})
	l.now = func() time.Time { return time.Unix(0, 0) } // freeze: no refill

	for i := 0; i < 5; i++ {
		if ok, _ := l.allow("1.2.3.4"); !ok {
			t.Fatalf("request %d within burst should pass", i)
		}
	}
	ok, retry := l.allow("1.2.3.4")
	if ok {
		t.Fatal("burst+1 request should be denied")
	}
	if retry < 1 {
		t.Fatalf("denied request must report Retry-After >= 1, got %d", retry)
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 2, Burst: 2})
	l.now = func() time.Time { return now }

	l.allow("ip")
	l.allow("ip")
	if ok, _ := l.allow("ip"); ok {
		t.Fatal("bucket should be drained after burst")
	}

	now = now.Add(500 * time.Millisecond) // 1 token at 2 rps
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("one token should have refilled after 0.5s")
	}
	if ok, _ := l.allow("ip"); ok {
		t.Fatal("only one token should have refilled")
	}
}

func TestRateLimiterBurstDefaultsToCeilRPS(t *testing.T) {
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 2.5})
	if l.capacity != 3 {
		t.Fatalf("burst should default to ceil(rps)=3, got %v", l.capacity)
	}
}

func TestRateLimiterWindowFormEnforces(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{Requests: 2, Window: "1s"})
	l.now = func() time.Time { return now }

	if l.capacity != 2 {
		t.Fatalf("burst should default to ceil(requests)=2, got %v", l.capacity)
	}
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("request 1 within burst should pass")
	}
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("request 2 within burst should pass")
	}
	if ok, _ := l.allow("ip"); ok {
		t.Fatal("burst+1 request should be denied")
	}

	now = now.Add(500 * time.Millisecond) // 1 token at 2 requests/1s
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("one token should have refilled after 0.5s")
	}
}

func TestRateLimiterWindowFormBurstDefaultsToCeilRequests(t *testing.T) {
	l := newRateLimiter(model.RateLimitMiddleware{Requests: 2.5, Window: "1m"})
	if l.capacity != 3 {
		t.Fatalf("burst should default to ceil(requests)=3, got %v", l.capacity)
	}
	if l.refill != 2.5/60 {
		t.Fatalf("refill should be requests/window.Seconds()=%v, got %v", 2.5/60, l.refill)
	}
}

func TestRateLimiterSlowRefillRetryAfterIsLarge(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{Requests: 1, Window: "1m"})
	l.now = func() time.Time { return now }

	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("first request should pass (burst 1)")
	}
	ok, retry := l.allow("ip")
	if ok {
		t.Fatal("second request should be denied")
	}
	// 1 request per 1m => refill is 1/60 tokens/sec; a fully-drained bucket must
	// wait close to 60s for the next token, not a clamped/truncated small value.
	if retry < 55 || retry > 60 {
		t.Fatalf("expected Retry-After near 60s for a 1-per-1m limit, got %d", retry)
	}
}

func TestRateLimiterPerKeyIsolation(t *testing.T) {
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1})
	l.now = func() time.Time { return time.Unix(0, 0) }

	if ok, _ := l.allow("a"); !ok {
		t.Fatal("first request for key a should pass")
	}
	if ok, _ := l.allow("b"); !ok {
		t.Fatal("a separate key b must have its own bucket")
	}
	if ok, _ := l.allow("a"); ok {
		t.Fatal("second request for key a should be denied")
	}
}

func TestRateLimiterEvictsIdleAtCap(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1000, Burst: 1})
	l.now = func() time.Time { return now }

	for i := 0; i < maxRateLimitBuckets; i++ {
		l.allow("seed-" + string(rune(i)))
	}
	if len(l.buckets) != maxRateLimitBuckets {
		t.Fatalf("expected map at cap, got %d", len(l.buckets))
	}
	// All seeds refill (1000 rps, burst 1) within a second; advancing time and
	// inserting a new key must trigger eviction rather than unbounded growth.
	now = now.Add(time.Second)
	l.allow("fresh")
	if len(l.buckets) > maxRateLimitBuckets {
		t.Fatalf("bucket map exceeded cap: %d", len(l.buckets))
	}
}

func TestRateLimiterEvictsLRUNotRecentlyUsed(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 100})
	l.now = func() time.Time { return now }

	// Fill to capacity; insertion order makes seed-0 the LRU, seed-(N-1) the MRU.
	keys := make([]string, maxRateLimitBuckets)
	for i := range keys {
		keys[i] = "seed-" + strconv.Itoa(i)
		l.allow(keys[i])
	}
	// Touch the current LRU so it becomes most-recently-used; seed-1 is now the LRU.
	l.allow(keys[0])

	// A new key evicts exactly one bucket - the LRU (seed-1), not the just-touched
	// seed-0 nor the new key.
	l.allow("fresh")
	if _, ok := l.buckets[keys[1]]; ok {
		t.Fatal("the least-recently-used bucket must be evicted")
	}
	if _, ok := l.buckets[keys[0]]; !ok {
		t.Fatal("a recently-used bucket must survive eviction")
	}
	if _, ok := l.buckets["fresh"]; !ok {
		t.Fatal("the newly inserted bucket must be present")
	}
	if len(l.buckets) != maxRateLimitBuckets {
		t.Fatalf("map must stay at cap, got %d", len(l.buckets))
	}
}

// TestRateLimiterBlockForOutlastsTokenRefill proves a client that trips the
// limit stays blocked for the configured blockFor even once tokens would
// otherwise have refilled - a paused-then-resumed client must not sail
// straight through.
func TestRateLimiterBlockForOutlastsTokenRefill(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 2, Burst: 2, BlockFor: "1m"})
	l.now = func() time.Time { return now }

	l.allow("ip")
	l.allow("ip") // drains the burst of 2

	ok, retry := l.allow("ip") // trips the limit -> blocked for 1m
	if ok {
		t.Fatal("burst+1 request should be denied")
	}
	if retry != 60 {
		t.Fatalf("expected Retry-After of 60s (blockFor), got %d", retry)
	}

	// Pause long enough for tokens to have fully refilled (at 2 rps) but well
	// short of the 1m block.
	now = now.Add(5 * time.Second)
	ok, retry = l.allow("ip")
	if ok {
		t.Fatal("request during the block period must still be denied despite refilled tokens")
	}
	if retry != 55 {
		t.Fatalf("expected Retry-After of 55s (remaining block), got %d", retry)
	}
}

// TestRateLimiterBlockForExpiresThenNormalBucketResumes proves that once the
// block period elapses, ordinary token-bucket rules resume - the client gets
// at most a normal refill/capacity, not an instant re-burst.
func TestRateLimiterBlockForExpiresThenNormalBucketResumes(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 2, Burst: 2, BlockFor: "1m"})
	l.now = func() time.Time { return now }

	l.allow("ip")
	l.allow("ip")
	if ok, _ := l.allow("ip"); ok {
		t.Fatal("burst+1 request should be denied and start the block")
	}

	// Advance past the block window. Tokens have had plenty of time to refill
	// to capacity, but capacity is still capped at 2 (burst), not unbounded.
	now = now.Add(2 * time.Minute)
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("request after the block expires should be allowed")
	}
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("second request after the block should still pass (capacity 2)")
	}
	if ok, _ := l.allow("ip"); ok {
		t.Fatal("third request should exceed capacity and be denied (no unbounded re-burst)")
	}
}

// TestRateLimiterBlockForNotExtendedByRepeatHits proves repeat requests during
// an active block do not push blockedUntil further out.
func TestRateLimiterBlockForNotExtendedByRepeatHits(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m"})
	l.now = func() time.Time { return now }

	l.allow("ip") // consumes the single token
	if ok, _ := l.allow("ip"); ok {
		t.Fatal("second request should trip the limit and start the block")
	}

	// Hammer the key repeatedly during the block; none of these should extend it.
	for i := 0; i < 5; i++ {
		now = now.Add(10 * time.Second)
		if ok, _ := l.allow("ip"); ok {
			t.Fatalf("request %d during the block should still be denied", i)
		}
	}
	// Total elapsed so far: 50s. The block was for 60s from t=0, so at t=60s it
	// must have expired - if a repeat hit had extended it, this would still deny.
	now = now.Add(10 * time.Second) // t=60s, exactly at expiry
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("block must expire exactly at the original blockFor, not extended by repeat hits")
	}
}

// TestRateLimiterBlockForUnsetBehavesAsToday proves that with BlockFor unset,
// a paused-then-resumed client is limited only by ordinary token refill, same
// as before this feature existed.
func TestRateLimiterBlockForUnsetBehavesAsToday(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 2, Burst: 2})
	l.now = func() time.Time { return now }

	l.allow("ip")
	l.allow("ip")
	if ok, _ := l.allow("ip"); ok {
		t.Fatal("burst+1 request should be denied")
	}

	now = now.Add(500 * time.Millisecond) // 1 token refills at 2 rps
	if ok, _ := l.allow("ip"); !ok {
		t.Fatal("without blockFor, a refilled token should let the request through")
	}
}

func serveRL(h http.Handler, remote, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", target, nil)
	req.RemoteAddr = remote
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimitHandler429WithRetryAfter(t *testing.T) {
	h := rateLimitHandler(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 2}, peerIP, "", nil, okHandler(), nil)

	if rec := serveRL(h, "9.9.9.9:1", "http://c/"); rec.Code != http.StatusOK {
		t.Fatalf("request 1 should pass, got %d", rec.Code)
	}
	if rec := serveRL(h, "9.9.9.9:1", "http://c/"); rec.Code != http.StatusOK {
		t.Fatalf("request 2 (within burst) should pass, got %d", rec.Code)
	}
	rec := serveRL(h, "9.9.9.9:1", "http://c/")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("burst+1 should be 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 response must carry a Retry-After header")
	}
}

func TestRateLimitHandlerNilIPSharedBucket(t *testing.T) {
	nilIP := func(*http.Request) net.IP { return nil }
	h := rateLimitHandler(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1}, nilIP, "", nil, okHandler(), nil)

	// Distinct source addresses, but an unresolvable IP collapses them to one
	// shared bucket, so the second request is denied regardless of source.
	if rec := serveRL(h, "1.1.1.1:1", "http://c/"); rec.Code != http.StatusOK {
		t.Fatalf("first nil-IP request should pass, got %d", rec.Code)
	}
	if rec := serveRL(h, "2.2.2.2:1", "http://c/"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second nil-IP request must share the bucket and be denied, got %d", rec.Code)
	}
}

func TestChainEnforcesRateLimit(t *testing.T) {
	cfg := model.Config{
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "rl"},
			Type:       model.MWTypeRateLimit,
			RateLimit:  &model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1},
		}},
	}
	reg := buildRegistry(cfg)
	host := model.ProxyHost{
		ObjectMeta:  model.ObjectMeta{Name: "app"},
		Middlewares: []string{"rl"},
	}
	h := buildChain(okHandler(), host, reg, nil)

	if rec := serveRL(h, "203.0.113.5:1", "http://app/"); rec.Code != http.StatusOK {
		t.Fatalf("first request should reach the backend, got %d", rec.Code)
	}
	if rec := serveRL(h, "203.0.113.5:1", "http://app/"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("burst+1 through the chain should be 429, got %d", rec.Code)
	}
}

// TestRateLimitHandlerAllowFromBypasses proves a client matching AllowFrom
// skips the limiter entirely: it never gets 429 no matter how many requests it
// sends, while a non-matching client is still limited as today.
func TestRateLimitHandlerAllowFromBypasses(t *testing.T) {
	rl := model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, AllowFrom: []string{"10.0.0.0/8"}}
	h := rateLimitHandler(rl, peerIP, "", nil, okHandler(), nil)

	for i := 0; i < 10; i++ {
		if rec := serveRL(h, "10.1.2.3:1", "http://c/"); rec.Code != http.StatusOK {
			t.Fatalf("allowlisted request %d should never be rate limited, got %d", i, rec.Code)
		}
	}

	// A non-matching client still gets limited exactly as today: first request
	// passes (burst 1), second is 429.
	if rec := serveRL(h, "203.0.113.5:1", "http://c/"); rec.Code != http.StatusOK {
		t.Fatalf("first non-allowlisted request should pass, got %d", rec.Code)
	}
	if rec := serveRL(h, "203.0.113.5:1", "http://c/"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second non-allowlisted request should be 429, got %d", rec.Code)
	}
}

// TestRateLimitHandlerNilIPNeverMatchesAllowFrom proves an unresolvable client
// IP never matches AllowFrom (fails closed onto the shared nil bucket), even
// when AllowFrom is configured.
func TestRateLimitHandlerNilIPNeverMatchesAllowFrom(t *testing.T) {
	nilIP := func(*http.Request) net.IP { return nil }
	rl := model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, AllowFrom: []string{"0.0.0.0/0"}}
	h := rateLimitHandler(rl, nilIP, "", nil, okHandler(), nil)

	if rec := serveRL(h, "1.1.1.1:1", "http://c/"); rec.Code != http.StatusOK {
		t.Fatalf("first nil-IP request should pass, got %d", rec.Code)
	}
	if rec := serveRL(h, "2.2.2.2:1", "http://c/"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second nil-IP request must still share the bucket and be denied, got %d", rec.Code)
	}
}

// TestRateLimitRunsBeforeAuth proves rate limiting is outermost: an over-limit
// request is shed with 429 before the auth tier runs (so a flood never reaches a
// forward-auth subrequest). A host gated by forward-auth would 401 an untrusted
// peer; once the bucket is empty the same peer gets 429 instead.
func TestRateLimitRunsBeforeAuth(t *testing.T) {
	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "fa"},
			Type:        model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
		}},
		Middlewares: []model.Middleware{
			{ObjectMeta: model.ObjectMeta{Name: "sso"}, Type: model.MWTypeAuth, Auth: &model.AuthMiddleware{IdentityProvider: "fa", Mode: model.AuthModeForwardAuth}},
			{ObjectMeta: model.ObjectMeta{Name: "rl"}, Type: model.MWTypeRateLimit, RateLimit: &model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1}},
		},
	}
	reg := buildRegistry(cfg)
	host := model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, Middlewares: []string{"sso", "rl"}}
	h := buildChain(okHandler(), host, reg, nil)

	// First request: rate-limit admits, auth rejects an untrusted peer -> 401.
	if rec := serveRL(h, "203.0.113.5:1", "http://app/"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("first request should be rejected by auth (401), got %d", rec.Code)
	}
	// Second request: bucket empty -> 429 from the outermost rate limiter, before auth.
	if rec := serveRL(h, "203.0.113.5:1", "http://app/"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over-limit request must be shed with 429 ahead of auth, got %d", rec.Code)
	}
}

// TestAccessListRunsBeforeAuth proves the access-list is wrapped outside auth: an
// IP the list denies is dropped with 403 before the auth tier runs, so it never
// drives a forward-auth subrequest to the IdP (GPM-L1). The forward-auth host
// would 401 an untrusted peer that reaches auth; a denied IP getting 403 instead
// of 401 shows the access-list short-circuited ahead of auth. An allowed peer
// still falls through to auth (401), proving the list did not wholesale replace it.
func TestAccessListRunsBeforeAuth(t *testing.T) {
	base := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "fa"},
			Type:        model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
		}},
		Middlewares: []model.Middleware{
			{ObjectMeta: model.ObjectMeta{Name: "sso"}, Type: model.MWTypeAuth, Auth: &model.AuthMiddleware{IdentityProvider: "fa", Mode: model.AuthModeForwardAuth}},
		},
	}

	serve := func(al model.AccessList) int {
		cfg := base
		cfg.AccessLists = []model.AccessList{al}
		reg := buildRegistry(cfg)
		host := model.ProxyHost{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Middlewares: []string{"sso"},
			AccessLists: []string{al.Name},
		}
		h := buildChain(okHandler(), host, reg, nil)
		return serveRL(h, "203.0.113.5:1", "http://app/").Code
	}

	// Deny-all list: the untrusted peer is dropped by the access-list (403) before
	// auth can 401 it.
	deny := model.AccessList{ObjectMeta: model.ObjectMeta{Name: "deny-all"}, DefaultAction: model.ActionDeny}
	if code := serve(deny); code != http.StatusForbidden {
		t.Fatalf("denied IP must be dropped by the access-list (403) ahead of auth, got %d", code)
	}

	// Allow-all list: the access-list passes, so the untrusted peer falls through
	// to auth and is rejected there (401) - auth still runs behind the list.
	allow := model.AccessList{ObjectMeta: model.ObjectMeta{Name: "allow-all"}, DefaultAction: model.ActionAllow}
	if code := serve(allow); code != http.StatusUnauthorized {
		t.Fatalf("allowed IP must fall through the access-list to auth (401), got %d", code)
	}
}

// TestRateLimiterBlockBackoffGrowsRepeatBlocks proves that with blockBackoff
// set, each block earned after the previous one ended lasts backoff times
// longer, up to blockMax, while requests during a block never extend it.
func TestRateLimiterBlockBackoffGrowsRepeatBlocks(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m", BlockBackoff: 2, BlockMax: "5m"})
	l.now = func() time.Time { return now }

	trip := func(want int) {
		t.Helper()
		if ok, _ := l.allow("ip"); !ok {
			t.Fatal("first request after a block must pass (bucket refilled)")
		}
		ok, retry := l.allow("ip")
		if ok {
			t.Fatal("second request within the same second must trip the limit")
		}
		if retry != want {
			t.Fatalf("expected a %ds block, got %d", want, retry)
		}
		// Hammering during the block neither passes nor extends it.
		now = now.Add(30 * time.Second)
		if ok, r := l.allow("ip"); ok || r > want-30 {
			t.Fatalf("mid-block request must be denied with the remaining time (<= %d), got ok=%v retry=%d", want-30, ok, r)
		}
		now = now.Add(time.Duration(want-30) * time.Second) // block just expired
	}
	trip(60)  // first block: blockFor
	trip(120) // second: x2
	trip(240) // third: x4
	trip(300) // fourth: capped at blockMax
	trip(300) // stays at the cap
}

// TestRateLimiterBlockBackoffForgetsAfterQuietPeriod proves the streak resets
// once the client stays within the limit for one full last-block length after
// that block ended, so an old offender starts over at blockFor.
func TestRateLimiterBlockBackoffForgetsAfterQuietPeriod(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m", BlockBackoff: 2})
	l.now = func() time.Time { return now }

	l.allow("ip")
	if _, retry := l.allow("ip"); retry != 60 {
		t.Fatalf("first block should be 60s, got %d", retry)
	}
	now = now.Add(60 * time.Second) // block over
	l.allow("ip")
	if _, retry := l.allow("ip"); retry != 120 {
		t.Fatalf("second block should be 120s, got %d", retry)
	}
	// Block ends at t=180s; staying clean until t=300s (one more lastBlock)
	// forgets the streak. One request per second stays within the limit.
	now = now.Add(120 * time.Second)
	for i := 0; i < 120; i++ {
		now = now.Add(time.Second)
		if ok, _ := l.allow("ip"); !ok {
			t.Fatalf("in-limit request %d must pass after the block ended", i)
		}
	}
	if ok, retry := l.allow("ip"); ok || retry != 60 {
		t.Fatalf("after a quiet period the block must restart at blockFor (60s), got ok=%v retry=%d", ok, retry)
	}
}

// TestRateLimiterBlockBackoffDefaultsCapTo24h proves an unset blockMax caps a
// backed-off block at model.DefaultBlockMax.
func TestRateLimiterBlockBackoffDefaultsCapTo24h(t *testing.T) {
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, BlockFor: "1h", BlockBackoff: 1000})
	if l.blockMax != model.DefaultBlockMax {
		t.Fatalf("blockMax default = %s, want %s", l.blockMax, model.DefaultBlockMax)
	}
	now := l.now()
	b := &tokenBucket{strikes: 3, blockedUntil: now, lastBlock: time.Hour} // last block ended just now
	if d := l.nextBlock(b, now); d != model.DefaultBlockMax {
		t.Fatalf("backed-off block = %s, want the 24h cap", d)
	}
}

// TestRateLimiterBlockWithoutBackoffStaysFixed proves blockFor alone keeps every
// block at the same length, the behavior before blockBackoff existed.
func TestRateLimiterBlockWithoutBackoffStaysFixed(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m"})
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		l.allow("ip")
		if _, retry := l.allow("ip"); retry != 60 {
			t.Fatalf("block %d should stay 60s without backoff, got %d", i+1, retry)
		}
		now = now.Add(60 * time.Second)
	}
}

// TestRateLimitHandlerAccessListExemption proves an "@<list>" allowFrom entry
// exempts the networks that access list allows (literal cidr rules and fetched
// source sets), skips its path-scoped rules, and ignores an unknown list.
func TestRateLimitHandlerAccessListExemption(t *testing.T) {
	SetAccessListSources(model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{
		{List: "lan", Source: "probes", Entries: []string{"203.0.113.0/24"}},
	}})
	t.Cleanup(func() { SetAccessListSources(model.AccessListSourceLedger{}) })
	reg := buildRegistry(model.Config{AccessLists: []model.AccessList{
		{ObjectMeta: model.ObjectMeta{Name: "lan"}, DefaultAction: model.ActionDeny, Rules: []model.IPRule{
			{Action: model.ActionAllow, CIDR: "10.0.0.0/8"},
			{Action: model.ActionAllow, Source: "probes"},
			{Action: model.ActionAllow, CIDR: "0.0.0.0/0", Paths: []string{"/public"}},
			{Action: model.ActionDeny, CIDR: "10.9.0.0/16"},
		}},
	}})
	rl := model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, AllowFrom: []string{"@lan", "@nope", "192.0.2.1"}}
	h := rateLimitHandlerWith(newRateLimiter(rl), rl, peerIP, "", nil, okHandler(), reg.rateLimitExemption)

	for _, ip := range []string{"10.1.2.3", "203.0.113.7", "192.0.2.1"} {
		for i := 0; i < 3; i++ {
			if rec := serveRL(h, ip+":1", "http://c/"); rec.Code != http.StatusOK {
				t.Fatalf("%s request %d should be exempt, got %d", ip, i+1, rec.Code)
			}
		}
	}
	// The path-scoped allow-all rule must not have exempted everyone.
	serveRL(h, "198.51.100.5:1", "http://c/public")
	if rec := serveRL(h, "198.51.100.5:1", "http://c/public"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a client outside the list must still be limited, got %d", rec.Code)
	}
}

// chainStateUpstream is a reachable upstream for chain-state tests.
func chainStateUpstream(t *testing.T) model.Upstream {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(up.Close)
	u, _ := url.Parse(up.URL)
	port, _ := strconv.Atoi(u.Port())
	return model.Upstream{Scheme: "http", Host: u.Hostname(), Port: port}
}

func chainStateDo(serve func(http.ResponseWriter, *http.Request), host, path, pass string) int {
	r := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
	r.RemoteAddr = "203.0.113.9:1000"
	if pass != "" {
		r.SetBasicAuth("admin", pass)
	}
	rec := httptest.NewRecorder()
	serve(rec, r)
	return rec.Code
}

// The basic-auth failure gate of a host is one budget per client across all of
// the host's routes, for the auth middleware and for an access list's
// basicAuth alike: failures on one location lock the client out of the others.
func TestBasicAuthFailureGateSharedAcrossHostRoutes(t *testing.T) {
	upstream := chainStateUpstream(t)
	mw := basicMW(t, "admin", "hunter2", "")
	locs := []model.Location{{Path: "/a"}, {Path: "/b"}}
	cfg := model.Config{
		Middlewares: []model.Middleware{{ObjectMeta: model.ObjectMeta{Name: "pw"}, Type: model.MWTypeAuth, Auth: &mw}},
		AccessLists: []model.AccessList{{ObjectMeta: model.ObjectMeta{Name: "acl-pw"}, BasicAuth: mw.Basic.Users}},
		ProxyHosts: []model.ProxyHost{
			{ObjectMeta: model.ObjectMeta{Name: "mw-host"}, Domains: []string{"mw.example.com"}, Upstream: upstream,
				Middlewares: []string{"pw"}, Locations: locs},
			{ObjectMeta: model.ObjectMeta{Name: "inline-host"}, Domains: []string{"inline.example.com"}, Upstream: upstream,
				Auth: &mw, Locations: locs},
			{ObjectMeta: model.ObjectMeta{Name: "acl-host"}, Domains: []string{"acl.example.com"}, Upstream: upstream,
				AccessLists: []string{"acl-pw"}, Locations: locs},
		},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	for _, host := range []string{"mw.example.com", "inline.example.com", "acl.example.com"} {
		for i := 0; i < maxBasicAuthFails; i++ {
			chainStateDo(rt.serveHTTP, host, "/", "wrong")
		}
		for _, p := range []string{"/", "/a", "/b"} {
			if got := chainStateDo(rt.serveHTTP, host, p, "hunter2"); got != http.StatusUnauthorized {
				t.Errorf("%s%s after %d failures on /: correct password -> %d, want 401 (locked out on every route)", host, p, maxBasicAuthFails, got)
			}
		}
	}
	// Another host keeps its own budget.
	if got := chainStateDo(rt.serveHTTP, "inline.example.com", "/", ""); got != http.StatusUnauthorized {
		t.Fatalf("challenge without credentials: %d", got)
	}
}

// A reload keeps a client's basic-auth lockout and rate-limit block when the
// spec that built them is unchanged, also when another host changed. A changed
// spec starts fresh, and two data planes never share state.
func TestReloadKeepsLockoutAndRateLimitState(t *testing.T) {
	upstream := chainStateUpstream(t)
	mw := basicMW(t, "admin", "hunter2", "")
	mk := func(other string, burst int, users []model.BasicAuthUser) model.Config {
		auth := mw
		auth.Basic = &model.BasicAuthSpec{Users: users}
		return model.Config{ProxyHosts: []model.ProxyHost{
			{ObjectMeta: model.ObjectMeta{Name: "basic"}, Domains: []string{"basic.example.com"}, Upstream: upstream, Auth: &auth,
				Locations: []model.Location{{Path: "/a"}}},
			{ObjectMeta: model.ObjectMeta{Name: "limited"}, Domains: []string{"limited.example.com"}, Upstream: upstream,
				RateLimit: &model.RateLimitMiddleware{RequestsPerSecond: 0.001, Burst: burst, BlockFor: "24h"},
				Locations: []model.Location{{Path: "/a"}}},
			{ObjectMeta: model.ObjectMeta{Name: "tenant"}, Domains: []string{other}, Upstream: upstream},
		}}
	}
	users := mw.Basic.Users
	s := New(Config{})
	if err := s.Reload(mk("t1.example.com", 1, users)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxBasicAuthFails; i++ {
		chainStateDo(s.dispatchHTTP, "basic.example.com", "/", "wrong")
	}
	chainStateDo(s.dispatchHTTP, "limited.example.com", "/", "")
	chainStateDo(s.dispatchHTTP, "limited.example.com", "/a", "")
	if got := chainStateDo(s.dispatchHTTP, "limited.example.com", "/", ""); got != http.StatusTooManyRequests {
		t.Fatalf("rate-limited host before reload: %d, want 429", got)
	}

	if err := s.Reload(mk("t2.example.com", 1, users)); err != nil {
		t.Fatal(err)
	}
	runtime.GC() // the previous router is unreachable now; its state must not be needed
	if got := chainStateDo(s.dispatchHTTP, "basic.example.com", "/a", "hunter2"); got != http.StatusUnauthorized {
		t.Fatalf("after an unrelated reload: correct password -> %d, want 401 (lockout kept)", got)
	}
	for _, p := range []string{"/", "/a"} {
		if got := chainStateDo(s.dispatchHTTP, "limited.example.com", p, ""); got != http.StatusTooManyRequests {
			t.Fatalf("after an unrelated reload: rate-limited route %s -> %d, want 429 (block kept)", p, got)
		}
	}

	// Another data plane with the same config starts empty.
	other := New(Config{})
	if err := other.Reload(mk("t2.example.com", 1, users)); err != nil {
		t.Fatal(err)
	}
	if got := chainStateDo(other.dispatchHTTP, "basic.example.com", "/", "hunter2"); got != http.StatusOK {
		t.Fatalf("second data plane: correct password -> %d, want 200", got)
	}

	// A changed spec builds fresh state.
	extra := basicMW(t, "ops", "s3cret", "").Basic.Users
	if err := s.Reload(mk("t2.example.com", 2, append(append([]model.BasicAuthUser{}, users...), extra...))); err != nil {
		t.Fatal(err)
	}
	if got := chainStateDo(s.dispatchHTTP, "basic.example.com", "/", "hunter2"); got != http.StatusOK {
		t.Fatalf("after a credential change: correct password -> %d, want 200 (fresh gate)", got)
	}
	if got := chainStateDo(s.dispatchHTTP, "limited.example.com", "/", ""); got != http.StatusOK {
		t.Fatalf("after a limit change: %d, want 200 (fresh limiter)", got)
	}
}

// Entries whose instance is no longer referenced by any chain are dropped.
func TestChainStatesDropUnreferencedEntries(t *testing.T) {
	var w weakMap[rateLimiter]
	keep := w.get("keep", func() *rateLimiter { return &rateLimiter{} })
	for i := 0; i < 500; i++ {
		w.get("gone"+strconv.Itoa(i), func() *rateLimiter { return &rateLimiter{} })
	}
	runtime.GC()
	w.sweepAt = 0
	w.get("new", func() *rateLimiter { return &rateLimiter{} })
	if len(w.m) > 2 {
		t.Fatalf("map holds %d entries after the unreferenced ones were collected, want 2", len(w.m))
	}
	if got := w.get("keep", func() *rateLimiter { return &rateLimiter{} }); got != keep {
		t.Fatal("a referenced instance was replaced")
	}
	runtime.KeepAlive(keep)
}

// A rate limit with scope: host is one budget per client across the host route
// and all of its locations, for an inline block and a referenced middleware
// alike, and the shared budget survives a reload with an unchanged spec. The
// default scope keeps one budget per route.
func TestRateLimitScopeHostSharedAcrossLocations(t *testing.T) {
	upstream := chainStateUpstream(t)
	locs := []model.Location{{Path: "/a"}, {Path: "/b"}}
	limit := func(scope string) *model.RateLimitMiddleware {
		return &model.RateLimitMiddleware{RequestsPerSecond: 0.001, Burst: 1, Scope: scope}
	}
	mk := func(other string) model.Config {
		return model.Config{
			Middlewares: []model.Middleware{{ObjectMeta: model.ObjectMeta{Name: "rl"}, Type: model.MWTypeRateLimit, RateLimit: limit(model.RateLimitScopeHost)}},
			ProxyHosts: []model.ProxyHost{
				{ObjectMeta: model.ObjectMeta{Name: "inline-host"}, Domains: []string{"inline.example.com"}, Upstream: upstream,
					RateLimit: limit(model.RateLimitScopeHost), Locations: locs},
				{ObjectMeta: model.ObjectMeta{Name: "mw-host"}, Domains: []string{"mw.example.com"}, Upstream: upstream,
					Middlewares: []string{"rl"}, Locations: locs},
				{ObjectMeta: model.ObjectMeta{Name: "route-host"}, Domains: []string{"route.example.com"}, Upstream: upstream,
					RateLimit: limit(""), Locations: locs},
				{ObjectMeta: model.ObjectMeta{Name: "tenant"}, Domains: []string{other}, Upstream: upstream},
			},
		}
	}
	s := New(Config{})
	if err := s.Reload(mk("t1.example.com")); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"inline.example.com", "mw.example.com"} {
		if got := chainStateDo(s.dispatchHTTP, host, "/", ""); got != http.StatusOK {
			t.Fatalf("%s first request: %d, want 200", host, got)
		}
		for _, p := range []string{"/a", "/b", "/"} {
			if got := chainStateDo(s.dispatchHTTP, host, p, ""); got != http.StatusTooManyRequests {
				t.Fatalf("%s%s after the host budget is spent: %d, want 429", host, p, got)
			}
		}
	}
	for _, p := range []string{"/", "/a", "/b"} {
		if got := chainStateDo(s.dispatchHTTP, "route.example.com", p, ""); got != http.StatusOK {
			t.Fatalf("default scope: first request on route %s -> %d, want 200 (one budget per route)", p, got)
		}
	}

	if err := s.Reload(mk("t2.example.com")); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	for _, host := range []string{"inline.example.com", "mw.example.com"} {
		if got := chainStateDo(s.dispatchHTTP, host, "/b", ""); got != http.StatusTooManyRequests {
			t.Fatalf("%s/b after an unrelated reload: %d, want 429 (shared budget kept)", host, got)
		}
	}
}

// TestRateLimitZonedPeerOwnBucket proves a peer whose address carries a zone
// ("[fe80::1%eth0]:port"), which parses to no client IP, gets the bucket of its
// zone (one per interface) instead of the bucket shared by every unresolvable
// peer, that rotating its self-assigned address inside the zone, at any
// ipv6Prefix, never earns a fresh bucket, and that an access list still denies
// it.
func TestRateLimitZonedPeerOwnBucket(t *testing.T) {
	for _, bits := range []int{0, model.MinRateLimitIPv6Prefix} {
		cfg := model.Config{
			Middlewares: []model.Middleware{{
				ObjectMeta: model.ObjectMeta{Name: "rl"},
				Type:       model.MWTypeRateLimit,
				RateLimit:  &model.RateLimitMiddleware{RequestsPerSecond: 0.001, Burst: 2, IPv6Prefix: bits},
			}},
			AccessLists: []model.AccessList{{
				ObjectMeta:    model.ObjectMeta{Name: "allow-all"},
				Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "::/0"}, {Action: model.ActionAllow, CIDR: "0.0.0.0/0"}},
				DefaultAction: model.ActionAllow,
			}},
		}
		reg := buildRegistry(cfg)
		h := buildChain(okHandler(), model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, Middlewares: []string{"rl"}}, reg, nil)
		serve := func(remote string, want int) {
			t.Helper()
			if rec := serveRL(h, remote, "http://app/"); rec.Code != want {
				t.Fatalf("ipv6Prefix=%d: %s: got %d, want %d", bits, remote, rec.Code, want)
			}
		}

		// One peer rotating its source through fe80::/10 in one zone: the
		// burst of 2, then 429 whatever address it picks.
		for i := 0; i < 50; i++ {
			want := http.StatusOK
			if i >= 2 {
				want = http.StatusTooManyRequests
			}
			serve(fmt.Sprintf("[fe80:0:0:%x::1%%eth0]:1", i), want)
		}
		serve("[fe80::1%eth1]:1", http.StatusOK) // another interface has its own bucket
		serve("[fe80::1%eth1]:2", http.StatusOK)
		serve("[fe80::1%eth1]:3", http.StatusTooManyRequests)
		// An interface may be named like an address; its zone's bucket is
		// still not that client's bucket.
		serve("[fe80::1%192.0.2.7]:1", http.StatusOK)
		serve("[fe80::1%192.0.2.7]:2", http.StatusOK)
		serve("192.0.2.7:1", http.StatusOK)
		// A peer without a zone that parses to no address (the zone branch is
		// reached, the port splits off) still shares the old nil bucket, and
		// the zones did not touch it.
		serve("not-an-address:1", http.StatusOK)
		serve("also-not-an-address:1", http.StatusOK)
		serve("third-not-an-address:1", http.StatusTooManyRequests)

		// Access lists keep denying a client without a parsed address.
		al := buildChain(okHandler(), model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, AccessLists: []string{"allow-all"}}, reg, nil)
		if rec := serveRL(al, "[fe80::1%eth0]:1", "http://app/"); rec.Code != http.StatusForbidden {
			t.Fatalf("access list on a zoned peer: got %d, want 403", rec.Code)
		}
	}
}

// TestDisabledAccessListExemptsNobody proves an "@<list>" allowFrom entry
// naming a disabled list exempts nobody, from a rate limit and from a path
// block alike, and that toggling the list takes effect on the next reload.
func TestDisabledAccessListExemptsNobody(t *testing.T) {
	upstream := chainStateUpstream(t)
	mk := func(disabled bool) model.Config {
		return model.Config{
			AccessLists: []model.AccessList{{
				ObjectMeta:    model.ObjectMeta{Name: "lan", Disabled: disabled},
				Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "203.0.113.0/24"}},
				DefaultAction: model.ActionDeny,
			}},
			Middlewares: []model.Middleware{{
				ObjectMeta: model.ObjectMeta{Name: "block"},
				Type:       model.MWTypePathBlock,
				PathBlock:  &model.PathBlockMiddleware{Names: []string{".env"}, AllowFrom: []string{"@lan"}},
			}},
			ProxyHosts: []model.ProxyHost{{
				ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"}, Upstream: upstream,
				Middlewares: []string{"block"},
				RateLimit:   &model.RateLimitMiddleware{RequestsPerSecond: 0.001, Burst: 1, AllowFrom: []string{"@lan"}},
			}},
		}
	}
	s := New(Config{})
	check := func(disabled bool) {
		t.Helper()
		if err := s.Reload(mk(disabled)); err != nil {
			t.Fatal(err)
		}
		wantBlock, wantSecond := http.StatusOK, http.StatusOK
		if disabled {
			wantBlock, wantSecond = http.StatusNotFound, http.StatusTooManyRequests
		}
		if got := chainStateDo(s.dispatchHTTP, "app.example.com", "/.env", ""); got != wantBlock {
			t.Fatalf("disabled=%v: blocked path -> %d, want %d", disabled, got, wantBlock)
		}
		if got := chainStateDo(s.dispatchHTTP, "app.example.com", "/", ""); got != wantSecond {
			t.Fatalf("disabled=%v: request after the burst -> %d, want %d", disabled, got, wantSecond)
		}
	}
	check(false)
	check(true)
	check(false)
}

// rateLimiterStateErr reports the first inconsistency between the map, the
// order list and the blocked heap, or "" when they agree: every map entry is
// in exactly one structure, heap indexes and the heap order are right, the
// sizes add up and the map is within the cap.
func rateLimiterStateErr(l *rateLimiter) string {
	if len(l.buckets) > l.maxBuckets {
		return fmt.Sprintf("map holds %d, over the cap %d", len(l.buckets), l.maxBuckets)
	}
	if n := l.order.Len() + len(l.blocked); n != len(l.buckets) {
		return fmt.Sprintf("order %d + heap %d != map %d", l.order.Len(), len(l.blocked), len(l.buckets))
	}
	seen := map[string]bool{}
	for el := l.order.Front(); el != nil; el = el.Next() {
		lb := el.Value.(*lruBucket)
		if lb.el != el || lb.heapIdx != -1 || l.buckets[lb.key] != lb || seen[lb.key] {
			return fmt.Sprintf("order node %q: el ok %v, heapIdx %d, map ok %v, dup %v", lb.key, lb.el == el, lb.heapIdx, l.buckets[lb.key] == lb, seen[lb.key])
		}
		seen[lb.key] = true
	}
	for i, lb := range l.blocked {
		if lb.heapIdx != i || lb.el != nil || l.buckets[lb.key] != lb || seen[lb.key] {
			return fmt.Sprintf("heap node %q at %d: heapIdx %d, el set %v, map ok %v, dup %v", lb.key, i, lb.heapIdx, lb.el != nil, l.buckets[lb.key] == lb, seen[lb.key])
		}
		if i > 0 && l.blocked.Less(i, (i-1)/2) {
			return fmt.Sprintf("heap order broken at %d (%q)", i, lb.key)
		}
		seen[lb.key] = true
	}
	return ""
}

func checkRateLimiterState(t *testing.T, l *rateLimiter) {
	t.Helper()
	if e := rateLimiterStateErr(l); e != "" {
		t.Fatal(e)
	}
}

// floodRateLimiter makes n fresh keys appear, one in-limit request each.
func floodRateLimiter(l *rateLimiter, prefix string, n int) {
	for i := 0; i < n; i++ {
		l.allow(prefix + strconv.Itoa(i))
	}
}

// selfBlockFlood makes n fresh keys appear that each trip their own limit
// (burst 1 assumed: two requests each).
func selfBlockFlood(l *rateLimiter, prefix string, n int) {
	for i := 0; i < n; i++ {
		k := prefix + strconv.Itoa(i)
		l.allow(k)
		l.allow(k)
	}
}

// TestRateLimiterBlockedKeySurvivesEvictionFlood proves a key whose block is
// still running is not evicted by a flood of fresh keys that stay within their
// own limit, so it stays refused with the remaining block as Retry-After.
func TestRateLimiterBlockedKeySurvivesEvictionFlood(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rl    model.RateLimitMiddleware
		flood int
	}{
		{"cap fresh keys", model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m"}, maxRateLimitBuckets},
		{"twice the cap", model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m"}, 2 * maxRateLimitBuckets},
		{"with backoff", model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m", BlockBackoff: 2}, maxRateLimitBuckets + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(0, 0)
			l := newRateLimiter(tc.rl)
			l.now = func() time.Time { return now }

			l.allow("victim")
			if ok, retry := l.allow("victim"); ok || retry != 60 {
				t.Fatalf("victim should be blocked for 60s, got ok=%v retry=%d", ok, retry)
			}
			now = now.Add(10 * time.Second)
			floodRateLimiter(l, "fresh-", tc.flood)
			checkRateLimiterState(t, l)
			if len(l.buckets) != maxRateLimitBuckets {
				t.Fatalf("map must stay at cap, got %d", len(l.buckets))
			}
			if ok, retry := l.allow("victim"); ok || retry != 50 {
				t.Fatalf("victim must still be refused with Retry-After 50, got ok=%v retry=%d", ok, retry)
			}
		})
	}
}

// TestRateLimiterExpiredBlockIsEvictable proves a bucket whose block is over
// counts as unblocked again: an eviction moves it back to the unblocked
// buckets as the most recently used one, and from there it is evicted in LRU
// order like any other, with the map staying at the cap.
func TestRateLimiterExpiredBlockIsEvictable(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m"})
	l.now = func() time.Time { return now }

	l.allow("victim")
	l.allow("victim") // blocked until t=60s
	floodRateLimiter(l, "fresh-", maxRateLimitBuckets)
	if _, ok := l.buckets["victim"]; !ok {
		t.Fatal("victim must survive the flood during its block")
	}

	now = now.Add(61 * time.Second)
	l.allow("late") // migrates the victim to the MRU end, evicts the LRU fresh key
	checkRateLimiterState(t, l)
	if len(l.blocked) != 0 {
		t.Fatalf("no bucket is blocked any more, heap holds %d", len(l.blocked))
	}
	if _, ok := l.buckets["victim"]; !ok {
		t.Fatal("a bucket whose block just ended counts as recently used and must survive the next eviction")
	}
	// cap-2 fresh keys remain ahead of the victim in LRU order.
	floodRateLimiter(l, "later-", maxRateLimitBuckets-2)
	if _, ok := l.buckets["victim"]; !ok {
		t.Fatal("victim must outlast every bucket that was idle longer")
	}
	l.allow("one-more")
	if _, ok := l.buckets["victim"]; ok {
		t.Fatal("once it is the LRU unblocked bucket, the victim must be evicted")
	}
	checkRateLimiterState(t, l)
	if len(l.buckets) != maxRateLimitBuckets {
		t.Fatalf("map must stay at cap, got %d", len(l.buckets))
	}
}

// TestRateLimiterBackoffStreakSurvivesFullTable proves an offender cannot
// reset blockBackoff at a full table by sending one request from a second
// address after each block: the bucket whose block ended rejoins as the most
// recently used one, so the blocks double as on an empty table.
func TestRateLimiterBackoffStreakSurvivesFullTable(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m", BlockBackoff: 2})
	l.now = func() time.Time { return now }
	floodRateLimiter(l, "busy-", maxRateLimitBuckets-1) // a busy table at the cap
	var blocks []int
	for cycle := 0; cycle < 4; cycle++ {
		l.allow("A")
		ok, retry := l.allow("A")
		if ok {
			t.Fatalf("cycle %d: A's second request must trip the limit", cycle)
		}
		blocks = append(blocks, retry)
		now = now.Add(time.Duration(retry) * time.Second)
		l.allow("Z" + strconv.Itoa(cycle)) // one request from another address
		checkRateLimiterState(t, l)
		if len(l.buckets) != maxRateLimitBuckets {
			t.Fatalf("cycle %d: map %d, want the cap", cycle, len(l.buckets))
		}
	}
	if fmt.Sprint(blocks) != "[60 120 240 480]" {
		t.Fatalf("block lengths %v, want [60 120 240 480] (doubling, streak kept)", blocks)
	}
}

// TestRateLimiterAllBlockedStaysAtCap proves that with more self-blocked keys
// than the cap the map stays at the cap: while more than half the table is
// blocked a new key evicts the block ending soonest (here the earliest, as all
// blocks have one length), and a later in-limit client does the same.
func TestRateLimiterAllBlockedStaysAtCap(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1h"})
	l.now = func() time.Time { return now }

	const extra = 100
	key := func(i int) string { return "blocked-" + strconv.Itoa(i) }
	for i := 0; i < maxRateLimitBuckets+extra; i++ {
		now = now.Add(time.Millisecond)
		l.allow(key(i))
		if ok, _ := l.allow(key(i)); ok {
			t.Fatalf("key %d should be blocked", i)
		}
	}
	checkRateLimiterState(t, l)
	if len(l.buckets) != maxRateLimitBuckets || len(l.blocked) != maxRateLimitBuckets || l.order.Len() != 0 {
		t.Fatalf("map %d, heap %d, order %d, want map and heap at cap %d", len(l.buckets), len(l.blocked), l.order.Len(), maxRateLimitBuckets)
	}
	for _, tc := range []struct {
		i    int
		want bool
	}{{0, false}, {extra - 1, false}, {extra, true}, {maxRateLimitBuckets + extra - 1, true}} {
		if _, ok := l.buckets[key(tc.i)]; ok != tc.want {
			t.Fatalf("key %d present=%v, want %v (soonest-ending blocks go first)", tc.i, ok, tc.want)
		}
	}
	if ok, _ := l.allow("in-limit"); !ok {
		t.Fatal("an in-limit client must get a bucket")
	}
	if _, ok := l.buckets[key(extra)]; ok {
		t.Fatal("the in-limit client must evict the block ending soonest")
	}
	checkRateLimiterState(t, l)
	if ok, retry := l.allow(key(maxRateLimitBuckets + extra - 1)); ok || retry != 3600 {
		t.Fatalf("surviving key must stay blocked, got ok=%v retry=%d", ok, retry)
	}
}

// TestRateLimiterBackoffStreakSurvivesFlood proves a blocked key that survives
// a flood keeps its streak: its next violation after the block doubles.
func TestRateLimiterBackoffStreakSurvivesFlood(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m", BlockBackoff: 2})
	l.now = func() time.Time { return now }

	l.allow("victim")
	if _, retry := l.allow("victim"); retry != 60 {
		t.Fatalf("first block should be 60s, got %d", retry)
	}
	now = now.Add(5 * time.Second)
	floodRateLimiter(l, "fresh-", 2*maxRateLimitBuckets)
	now = now.Add(55 * time.Second) // block just over, inside the quiet window

	if ok, _ := l.allow("victim"); !ok {
		t.Fatal("first request after the block must pass")
	}
	if ok, retry := l.allow("victim"); ok || retry != 120 {
		t.Fatalf("second block must double to 120s, got ok=%v retry=%d", ok, retry)
	}
	checkRateLimiterState(t, l)
}

// TestRateLimiterBlockedRetentionNeverStricterForActiveClient proves keeping
// blocked buckets does not make the limit stricter for an unblocked client:
// whether its bucket survives the flood or is evicted (and restarts full), its
// next request within its remaining budget passes.
func TestRateLimiterBlockedRetentionNeverStricterForActiveClient(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 2, BlockFor: "1h"})
	l.now = func() time.Time { return now }

	for i := 0; i < maxRateLimitBuckets/2; i++ {
		k := "blocked-" + strconv.Itoa(i)
		l.allow(k)
		l.allow(k)
		l.allow(k)
	}
	if ok, _ := l.allow("active"); !ok {
		t.Fatal("active client's first request must pass")
	}
	floodRateLimiter(l, "fresh-", maxRateLimitBuckets)
	checkRateLimiterState(t, l)
	if ok, _ := l.allow("active"); !ok {
		t.Fatal("active client's second request (burst 2) must pass after the flood")
	}
	if len(l.blocked) != maxRateLimitBuckets/2 {
		t.Fatalf("half the table blocked: all blocks must survive an in-limit flood, heap holds %d", len(l.blocked))
	}
}

// TestRateLimiterBlockedFillKeepsLimitingUnblocked proves a table nearly full
// of blocked buckets does not disable the limit for unblocked clients: two
// alternating addresses are limited exactly as on an empty table.
func TestRateLimiterBlockedFillKeepsLimitingUnblocked(t *testing.T) {
	rl := model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1h"}
	alternate := func(l *rateLimiter) (allowed int) {
		for i := 0; i < 1000; i++ {
			if ok, _ := l.allow([]string{"K", "F"}[i%2]); ok {
				allowed++
			}
		}
		return allowed
	}
	now := time.Unix(0, 0)
	clock := func() time.Time { return now }

	empty := newRateLimiter(rl)
	empty.now = clock
	want := alternate(empty)

	l := newRateLimiter(rl)
	l.now = clock
	selfBlockFlood(l, "filler-", maxRateLimitBuckets-1)
	if got := alternate(l); got != want {
		t.Fatalf("after a blocked fill %d of 1000 requests passed, an empty table allows %d", got, want)
	}
	checkRateLimiterState(t, l)
	if want != 2 {
		t.Fatalf("burst 1, no time passing: an empty table should allow 2, allowed %d", want)
	}
}

// TestRateLimiterEvictsExpiredBlockBehindRunningOne proves that with backoff,
// where blocks differ in length, a bucket whose block ended is evicted ahead
// of one whose longer block is still running, and the running one stays
// refused.
func TestRateLimiterEvictsExpiredBlockBehindRunningOne(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m", BlockBackoff: 2})
	l.maxBuckets = 2
	l.now = func() time.Time { return now }

	l.allow("A")
	l.allow("A") // block 1: until 60s
	now = now.Add(61 * time.Second)
	l.allow("A")
	l.allow("A") // block 2, doubled: until 181s
	l.allow("B")
	l.allow("B")                    // until 121s, set after A's
	now = now.Add(70 * time.Second) // t=131: B ended, A runs until 181
	l.allow("C")
	checkRateLimiterState(t, l)
	if _, ok := l.buckets["B"]; ok {
		t.Fatal("B, whose block ended, must be the victim")
	}
	if ok, retry := l.allow("A"); ok || retry != 50 {
		t.Fatalf("A must still be refused with Retry-After 50, got ok=%v retry=%d", ok, retry)
	}
}

// TestRateLimiterSelfBlockingFlood pins the documented behaviour under a
// flood of addresses that each get themselves blocked: once more than half
// the table is blocked the block ending soonest is evicted, so a victim whose
// backed-off block is longer than the fillers' outlives them, while a victim
// whose block ends first goes as soon as the blocked share passes half.
func TestRateLimiterSelfBlockingFlood(t *testing.T) {
	t.Run("longer backoff block outlives the flood", func(t *testing.T) {
		now := time.Unix(0, 0)
		l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m", BlockBackoff: 2, BlockMax: "1h"})
		l.now = func() time.Time { return now }
		for _, want := range []int{60, 120, 240} {
			l.allow("victim")
			if ok, retry := l.allow("victim"); ok || retry != want {
				t.Fatalf("victim block = %v/%d, want %ds", ok, retry, want)
			}
			now = now.Add(time.Duration(want) * time.Second)
		}
		now = now.Add(-240 * time.Second) // back to the start of the 240s block
		selfBlockFlood(l, "flood-", maxRateLimitBuckets+1)
		checkRateLimiterState(t, l)
		if len(l.buckets) != maxRateLimitBuckets {
			t.Fatalf("map must stay at cap, got %d", len(l.buckets))
		}
		if ok, retry := l.allow("victim"); ok || retry != 240 {
			t.Fatalf("victim must outlive 60s fillers, got ok=%v retry=%d", ok, retry)
		}
	})
	t.Run("shorter block goes once more than half is blocked", func(t *testing.T) {
		now := time.Unix(0, 0)
		l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m"})
		l.now = func() time.Time { return now }
		l.allow("victim")
		l.allow("victim") // until 60s
		now = now.Add(10 * time.Second)
		floodRateLimiter(l, "in-limit-", maxRateLimitBuckets-1)
		selfBlockFlood(l, "flood-", maxRateLimitBuckets/2) // each until 70s
		checkRateLimiterState(t, l)
		if len(l.blocked) != maxRateLimitBuckets/2+1 {
			t.Fatalf("heap holds %d, want just over half (%d)", len(l.blocked), maxRateLimitBuckets/2+1)
		}
		if ok, retry := l.allow("victim"); ok || retry != 50 {
			t.Fatalf("victim must survive up to half the table blocked, got ok=%v retry=%d", ok, retry)
		}
		selfBlockFlood(l, "flood-more-", 1)
		if _, ok := l.buckets["victim"]; ok {
			t.Fatal("past half the table blocked, the block ending soonest (the victim's) must be evicted")
		}
		checkRateLimiterState(t, l)
	})
}

// TestRateLimiterMigratedBucketTouchStaysConsistent proves a bucket that an
// eviction moved from the heap back to order is a plain unblocked bucket at
// the MRU end: touching it, blocking it again and migrating it again leave the
// map, list and heap consistent with nothing leaked.
func TestRateLimiterMigratedBucketTouchStaysConsistent(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "10s"})
	l.maxBuckets = 4
	l.now = func() time.Time { return now }

	l.allow("p0")
	l.allow("p1")
	for _, k := range []string{"X", "Y"} {
		l.allow(k)
		l.allow(k)
	}
	checkRateLimiterState(t, l)
	for round := 0; round < 5; round++ {
		now = now.Add(11 * time.Second)
		n := "n" + strconv.Itoa(round)
		l.allow(n) // migrates X and Y (X ended first) to the MRU end, evicts the LRU bucket
		checkRateLimiterState(t, l)
		if len(l.blocked) != 0 || l.order.Len() != 4 {
			t.Fatalf("round %d: heap %d, order %d, want 0 and 4", round, len(l.blocked), l.order.Len())
		}
		var keys []string
		for el := l.order.Front(); el != nil; el = el.Next() {
			keys = append(keys, el.Value.(*lruBucket).key)
		}
		if got := fmt.Sprint(keys[1:]); got != "[X Y "+n+"]" {
			t.Fatalf("round %d: order %v, want X, Y then %s at the MRU end", round, keys, n)
		}
		for _, k := range []string{"X", "Y"} {
			if ok, _ := l.allow(k); !ok {
				t.Fatalf("round %d: %s must pass after its block", round, k)
			}
			checkRateLimiterState(t, l)
			if ok, _ := l.allow(k); ok {
				t.Fatalf("round %d: %s must trip again", round, k)
			}
			checkRateLimiterState(t, l)
		}
		if len(l.buckets) != 4 || len(l.blocked) != 2 || l.order.Len() != 2 {
			t.Fatalf("round %d: map %d, order %d, heap %d, want 4, 2, 2", round, len(l.buckets), l.order.Len(), len(l.blocked))
		}
	}
}

// TestRateLimiterUnblockByRequestStaysConsistent proves the request path that
// takes a bucket out of the heap (its own request after the block ended)
// leaves it a proper order node: it can be blocked again, unblocked again, and
// survive or suffer evictions with the structures consistent at every step.
func TestRateLimiterUnblockByRequestStaysConsistent(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "10s"})
	l.maxBuckets = 3
	l.now = func() time.Time { return now }

	step := func(what string, f func()) {
		t.Helper()
		f()
		if e := rateLimiterStateErr(l); e != "" {
			t.Fatalf("after %s: %s", what, e)
		}
	}
	for round := 0; round < 3; round++ {
		r := strconv.Itoa(round)
		step("A trips", func() { l.allow("A"); l.allow("A") })
		if l.buckets["A"].heapIdx < 0 {
			t.Fatalf("round %d: A must be in the heap", round)
		}
		now = now.Add(11 * time.Second)
		step("A returns after its block", func() {
			if ok, _ := l.allow("A"); !ok {
				t.Fatalf("round %d: A must pass after its block", round)
			}
		})
		if lb := l.buckets["A"]; lb.heapIdx != -1 || lb.el == nil || l.order.Back().Value.(*lruBucket) != lb {
			t.Fatalf("round %d: A must be the MRU order node", round)
		}
		step("A trips again", func() { l.allow("A") })
		now = now.Add(11 * time.Second)
		step("A returns again", func() { l.allow("A") })
		for i := 0; i < 4; i++ {
			step("new key "+strconv.Itoa(i), func() { l.allow("new-" + r + "-" + strconv.Itoa(i)) })
		}
		if _, ok := l.buckets["A"]; ok {
			t.Fatalf("round %d: A, now idle longest, must have been evicted", round)
		}
	}
}

// TestRateLimiterHitsDuringBlockDoNotReorderEviction proves repeated requests
// during a block do not change which blocked bucket is evicted next.
func TestRateLimiterHitsDuringBlockDoNotReorderEviction(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, BlockFor: "1m"})
	l.maxBuckets = 4
	l.now = func() time.Time { return now }

	selfBlockFlood(l, "b", 3) // b0, b1, b2: same block end, b0 set first
	l.allow("p")
	for i := 0; i < 10; i++ {
		now = now.Add(time.Second)
		if ok, _ := l.allow("b0"); ok {
			t.Fatal("b0 must be refused during its block")
		}
	}
	l.allow("n") // three of four blocked: evicts the block ending soonest
	checkRateLimiterState(t, l)
	if _, ok := l.buckets["b0"]; ok {
		t.Fatal("hits during b0's block must not move it behind b1 and b2")
	}
	for _, k := range []string{"b1", "b2", "p", "n"} {
		if _, ok := l.buckets[k]; !ok {
			t.Fatalf("%s must survive", k)
		}
	}
}

// TestRateLimiterRandomizedStateConsistency drives small tables with random
// keys and clock steps (backwards included) and checks after every request
// that the structures agree and that each eviction took the bucket an
// independent model of the policy names. The model keeps its own LRU order and
// blocked set, updated by the documented rules, and reads only the limiter's
// answers and bucket times, never its list or heap.
func TestRateLimiterRandomizedStateConsistency(t *testing.T) {
	for _, rl := range []model.RateLimitMiddleware{
		{RequestsPerSecond: 1, Burst: 2},
		{RequestsPerSecond: 1, Burst: 1, BlockFor: "5s"},
		{RequestsPerSecond: 1, Burst: 3, BlockFor: "5s"},
		{RequestsPerSecond: 1, Burst: 1, BlockFor: "5s", BlockBackoff: 2, BlockMax: "40s"},
	} {
		for _, capN := range []int{1, 2, 3, 5, 8} {
			for seed := uint64(1); seed <= 3; seed++ {
				runRateLimiterModel(t, rl, capN, seed, 700)
			}
		}
	}
}

// rlModel is the independent policy model: order is the LRU order of the
// unblocked keys (front first), blocked holds the keys with a block in its
// own structure, until the last block end per key.
type rlModel struct {
	capN    int
	order   []string
	blocked map[string]uint64 // key -> block sequence number
	until   map[string]time.Time
	seq     uint64
}

func (m *rlModel) remove(key string) {
	for i, k := range m.order {
		if k == key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
}

func (m *rlModel) blockedLess(a, b string) bool {
	if !m.until[a].Equal(m.until[b]) {
		return m.until[a].Before(m.until[b])
	}
	return m.blocked[a] < m.blocked[b]
}

// evict applies the eviction rules and returns the victim.
func (m *rlModel) evict(now time.Time) string {
	for {
		first := ""
		for k := range m.blocked {
			if !m.until[k].After(now) && (first == "" || m.blockedLess(k, first)) {
				first = k
			}
		}
		if first == "" {
			break
		}
		delete(m.blocked, first)
		m.order = append(m.order, first)
	}
	var victim string
	if len(m.blocked) > m.capN/2 {
		for k := range m.blocked {
			if victim == "" || m.blockedLess(k, victim) {
				victim = k
			}
		}
		delete(m.blocked, victim)
	} else {
		victim = m.order[0]
		m.order = m.order[1:]
	}
	delete(m.until, victim)
	return victim
}

func runRateLimiterModel(t *testing.T, rl model.RateLimitMiddleware, capN int, seed uint64, steps int) {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, uint64(capN)))
	now := time.Unix(1000, 0)
	l := newRateLimiter(rl)
	l.maxBuckets = capN
	l.now = func() time.Time { return now }
	m := &rlModel{capN: capN, blocked: map[string]uint64{}, until: map[string]time.Time{}}
	clock := []time.Duration{0, 0, 100 * time.Millisecond, time.Second, 3 * time.Second, 6 * time.Second, 30 * time.Second, -2 * time.Second}
	for step := 0; step < steps; step++ {
		now = now.Add(clock[r.IntN(len(clock))])
		key := "k" + strconv.Itoa(r.IntN(3*capN))
		if r.IntN(4) == 0 {
			key = "hot"
		}
		where := fmt.Sprintf("rl %+v cap %d seed %d step %d key %s", rl, capN, seed, step, key)

		until, known := m.until[key]
		runningBefore := known && l.blockFor > 0 && now.Before(until)
		want := ""
		switch {
		case !known:
			if len(m.until) >= capN {
				want = m.evict(now)
			}
			m.order = append(m.order, key)
		case runningBefore:
			// a hit during the block changes nothing
		default:
			if _, ok := m.blocked[key]; ok {
				delete(m.blocked, key)
			} else {
				m.remove(key)
			}
			m.order = append(m.order, key)
		}

		before := make(map[string]bool, len(l.buckets))
		for k := range l.buckets {
			before[k] = true
		}
		ok, _ := l.allow(key)
		if !ok && l.blockFor > 0 && !runningBefore {
			m.remove(key)
			m.seq++
			m.blocked[key] = m.seq
		}
		if lb := l.buckets[key]; lb != nil {
			m.until[key] = lb.bucket.blockedUntil
		}

		if e := rateLimiterStateErr(l); e != "" {
			t.Fatalf("%s: %s", where, e)
		}
		var gone []string
		for k := range before {
			if _, ok := l.buckets[k]; !ok {
				gone = append(gone, k)
			}
		}
		if want == "" && len(gone) != 0 || want != "" && (len(gone) != 1 || gone[0] != want) {
			t.Fatalf("%s: evicted %v, model wants %q", where, gone, want)
		}
		if len(l.buckets) != len(m.until) || len(l.buckets) != len(m.order)+len(m.blocked) {
			t.Fatalf("%s: map %d, model %d keys (%d order + %d blocked)", where, len(l.buckets), len(m.until), len(m.order), len(m.blocked))
		}
	}
}
