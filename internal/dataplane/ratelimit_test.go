package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
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

func serveRL(h http.Handler, remote, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", target, nil)
	req.RemoteAddr = remote
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimitHandler429WithRetryAfter(t *testing.T) {
	h := rateLimitHandler(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 2}, peerIP, okHandler())

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
	h := rateLimitHandler(model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1}, nilIP, okHandler())

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
	h := buildChain(okHandler(), host, reg)

	if rec := serveRL(h, "203.0.113.5:1", "http://app/"); rec.Code != http.StatusOK {
		t.Fatalf("first request should reach the backend, got %d", rec.Code)
	}
	if rec := serveRL(h, "203.0.113.5:1", "http://app/"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("burst+1 through the chain should be 429, got %d", rec.Code)
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
	h := buildChain(okHandler(), host, reg)

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
		h := buildChain(okHandler(), host, reg)
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
