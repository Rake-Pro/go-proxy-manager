package dataplane

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// bouncerNext is the terminal handler a bouncer chain wraps: reaching it means
// the request was allowed.
func bouncerNext(reached *int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reached != nil {
			atomic.AddInt32(reached, 1)
		}
		w.WriteHeader(http.StatusOK)
	})
}

// bouncerReq builds a request from a fixed peer.
func bouncerReq(peer string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://app.example.com/some/path", nil)
	r.RemoteAddr = peer + ":51000"
	return r
}

func serveBouncer(t *testing.T, spec model.BouncerMiddleware, peer string) (*httptest.ResponseRecorder, int32) {
	t.Helper()
	var reached int32
	h := bouncerHandler(compileBouncer("bnc", spec), "app", clientIPResolver(nil), nil, bouncerNext(&reached))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, bouncerReq(peer))
	return rec, atomic.LoadInt32(&reached)
}

// The CrowdSec LAPI bouncer flow: GET /v1/decisions?ip=<client> with X-Api-Key.
// "null" (and an empty body) mean the IP is clean; a ban or captcha decision
// denies. Captcha is deliberately a DENY - gpm has no captcha flow to hand the
// client, so serving the request would silently downgrade the operator's
// decision to an allow.
func TestCrowdSecVerdicts(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		status   int
		wantCode int
		wantNext bool
	}{
		{name: "null means no decisions", body: "null", status: 200, wantCode: http.StatusOK, wantNext: true},
		{name: "empty body means no decisions", body: "", status: 200, wantCode: http.StatusOK, wantNext: true},
		{name: "empty array means no decisions", body: "[]", status: 200, wantCode: http.StatusOK, wantNext: true},
		{
			name:   "ban denies",
			body:   `[{"type":"ban","scope":"Ip","value":"203.0.113.7"}]`,
			status: 200, wantCode: http.StatusForbidden, wantNext: false,
		},
		{
			name:   "captcha denies",
			body:   `[{"type":"captcha","scope":"Ip","value":"203.0.113.7"}]`,
			status: 200, wantCode: http.StatusForbidden, wantNext: false,
		},
		{
			name:   "unknown remediation is ignored, not guessed at",
			body:   `[{"type":"throttle","scope":"Ip","value":"203.0.113.7"}]`,
			status: 200, wantCode: http.StatusOK, wantNext: true,
		},
		{
			name:   "range decision denies",
			body:   `[{"type":"ban","scope":"Range","value":"203.0.113.0/24"}]`,
			status: 200, wantCode: http.StatusForbidden, wantNext: false,
		},
		// The LAPI erroring is not an answer: fail-open (the default) allows.
		{name: "lapi 500 takes the onError path", body: "boom", status: 500, wantCode: http.StatusOK, wantNext: true},
		{name: "lapi 403 (bad key) takes the onError path", body: "", status: 403, wantCode: http.StatusOK, wantNext: true},
		{name: "undecodable body takes the onError path", body: "{not json", status: 200, wantCode: http.StatusOK, wantNext: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotKey, gotQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotKey = r.Header.Get("X-Api-Key")
				gotQuery = r.URL.Query().Get("ip")
				if r.URL.Path != "/v1/decisions" {
					t.Errorf("path = %q, want /v1/decisions", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			rec, reached := serveBouncer(t, model.BouncerMiddleware{
				Provider: model.BouncerProviderCrowdSec,
				URL:      srv.URL,
				APIKey:   model.Secret("secret-key"),
			}, "203.0.113.7")

			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if (reached > 0) != tc.wantNext {
				t.Errorf("upstream reached = %v, want %v", reached > 0, tc.wantNext)
			}
			if gotKey != "secret-key" {
				t.Errorf("X-Api-Key = %q, want %q", gotKey, "secret-key")
			}
			if gotQuery != "203.0.113.7" {
				t.Errorf("ip query = %q, want %q", gotQuery, "203.0.113.7")
			}
		})
	}
}

// The generic http deny hook: 2xx allows, 403 denies, anything else is not an
// answer and the onError policy governs.
func TestHTTPProviderMatrix(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		failOpen bool
		wantCode int
		wantNext bool
	}{
		{name: "200 allows", status: 200, failOpen: true, wantCode: http.StatusOK, wantNext: true},
		{name: "204 allows", status: 204, failOpen: true, wantCode: http.StatusOK, wantNext: true},
		{name: "403 denies", status: 403, failOpen: true, wantCode: http.StatusForbidden, wantNext: false},
		{name: "403 denies under fail-closed too", status: 403, failOpen: false, wantCode: http.StatusForbidden, wantNext: false},
		{name: "401 is no answer, fail-open allows", status: 401, failOpen: true, wantCode: http.StatusOK, wantNext: true},
		{name: "401 is no answer, fail-closed denies", status: 401, failOpen: false, wantCode: http.StatusForbidden, wantNext: false},
		{name: "500 is no answer, fail-open allows", status: 500, failOpen: true, wantCode: http.StatusOK, wantNext: true},
		{name: "500 is no answer, fail-closed denies", status: 500, failOpen: false, wantCode: http.StatusForbidden, wantNext: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotIP, gotHost, gotPath, gotXFF, gotOrig string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotIP = r.URL.Query().Get("ip")
				gotHost = r.URL.Query().Get("host")
				gotPath = r.URL.Query().Get("path")
				gotXFF = r.Header.Get("X-Forwarded-For")
				gotOrig = r.Header.Get("X-Original-URL")
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			onError := model.BouncerOnErrorFailOpen
			if !tc.failOpen {
				onError = model.BouncerOnErrorFailClosed
			}
			rec, reached := serveBouncer(t, model.BouncerMiddleware{
				Provider: model.BouncerProviderHTTP,
				URL:      srv.URL + "/check",
				OnError:  onError,
			}, "198.51.100.9")

			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if (reached > 0) != tc.wantNext {
				t.Errorf("upstream reached = %v, want %v", reached > 0, tc.wantNext)
			}
			if gotIP != "198.51.100.9" || gotXFF != "198.51.100.9" {
				t.Errorf("ip=%q xff=%q, want 198.51.100.9 for both", gotIP, gotXFF)
			}
			if gotHost != "app.example.com" {
				t.Errorf("host = %q, want app.example.com", gotHost)
			}
			if gotPath != "/some/path" {
				t.Errorf("path = %q, want /some/path", gotPath)
			}
			if !strings.HasSuffix(gotOrig, "/some/path") {
				t.Errorf("X-Original-URL = %q, want it to carry the request path", gotOrig)
			}
		})
	}
}

// A bouncer that cannot be reached at all is the operationally important case:
// fail-open (the default) must keep serving, fail-closed must deny.
func TestBouncerUnreachableOnErrorPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	dead := srv.URL
	srv.Close() // nothing is listening now

	for _, tc := range []struct {
		onError  string
		wantCode int
	}{
		{onError: "", wantCode: http.StatusOK},                                    // default is fail-open
		{onError: model.BouncerOnErrorFailOpen, wantCode: http.StatusOK},          //
		{onError: model.BouncerOnErrorFailClosed, wantCode: http.StatusForbidden}, //
	} {
		name := tc.onError
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			rec, _ := serveBouncer(t, model.BouncerMiddleware{
				Provider: model.BouncerProviderCrowdSec,
				URL:      dead,
				APIKey:   model.Secret("k"),
				Timeout:  "200ms",
				OnError:  tc.onError,
			}, "203.0.113.7")
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
		})
	}
}

// A denied verdict is cached, so a second request from the same IP does not hit
// the bouncer again; a different IP does.
func TestBouncerCacheHit(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`[{"type":"ban","scope":"Ip","value":"` + r.URL.Query().Get("ip") + `"}]`))
	}))
	defer srv.Close()

	h := bouncerHandler(compileBouncer("bnc", model.BouncerMiddleware{
		URL: srv.URL, APIKey: model.Secret("k"), CacheTTL: "1h",
	}), "app", clientIPResolver(nil), nil, bouncerNext(nil))

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, bouncerReq("203.0.113.7"))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("request %d: status = %d, want 403", i, rec.Code)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("lapi calls = %d, want 1 (verdict must be cached)", got)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, bouncerReq("203.0.113.8"))
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("lapi calls after a new IP = %d, want 2 (the cache is per client IP)", got)
	}
}

func TestVerdictCacheExpiry(t *testing.T) {
	now := time.Unix(1700000000, 0)
	c := newVerdictCache(16)
	c.now = func() time.Time { return now }

	c.put("a", true, 30*time.Second)
	if deny, ok := c.get("a"); !ok || !deny {
		t.Fatalf("get(a) = (%v,%v), want (true,true)", deny, ok)
	}
	now = now.Add(29 * time.Second)
	if _, ok := c.get("a"); !ok {
		t.Error("entry expired early")
	}
	now = now.Add(2 * time.Second)
	if _, ok := c.get("a"); ok {
		t.Error("expired entry was still served")
	}
	if c.len() != 0 {
		t.Errorf("expired entry was not evicted: len = %d", c.len())
	}
}

// The cache is bounded so a rotating-source-IP flood cannot grow it without
// bound; the least-recently-used entry is the one dropped.
func TestVerdictCacheBounded(t *testing.T) {
	c := newVerdictCache(4)
	for i := 0; i < 100; i++ {
		c.put(fmt.Sprintf("10.0.0.%d", i), true, time.Hour)
	}
	if c.len() != 4 {
		t.Errorf("cache len = %d, want the 4-entry bound", c.len())
	}
	if _, ok := c.get("10.0.0.0"); ok {
		t.Error("the least-recently-used entry survived eviction")
	}
	if _, ok := c.get("10.0.0.99"); !ok {
		t.Error("the most-recently-used entry was evicted")
	}
}

// A verdict that came from an ERROR is capped at 5s regardless of cacheTTL, so
// an outage cannot pin a long TTL of guessed verdicts.
func TestBouncerErrorVerdictCacheIsCapped(t *testing.T) {
	for _, tc := range []struct {
		ttl  string
		want time.Duration
	}{
		{ttl: "1h", want: model.MaxBouncerErrorCacheTTL},
		{ttl: "60s", want: model.MaxBouncerErrorCacheTTL},
		{ttl: "1s", want: time.Second}, // a TTL below the cap is not extended to it
	} {
		t.Run(tc.ttl, func(t *testing.T) {
			b := compileBouncer("bnc", model.BouncerMiddleware{URL: "http://x", APIKey: model.Secret("k"), CacheTTL: tc.ttl})
			if got := b.errCacheTTL(); got != tc.want {
				t.Errorf("errCacheTTL() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Stream mode: one startup pull builds the whole decision set, then deltas
// adjust it, and the request path never calls the LAPI again. CIDR (Range)
// decisions must match every address inside them.
func TestCrowdSecStreamMode(t *testing.T) {
	var startupCalls, deltaCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/decisions/stream" {
			t.Errorf("path = %q, want /v1/decisions/stream", r.URL.Path)
		}
		if r.URL.Query().Get("startup") == "true" {
			atomic.AddInt32(&startupCalls, 1)
			_, _ = w.Write([]byte(`{"new":[
				{"type":"ban","scope":"Ip","value":"203.0.113.7"},
				{"type":"ban","scope":"Range","value":"198.51.100.0/24"},
				{"type":"captcha","scope":"Ip","value":"203.0.113.9"},
				{"type":"throttle","scope":"Ip","value":"203.0.113.10"}
			],"deleted":null}`))
			return
		}
		atomic.AddInt32(&deltaCalls, 1)
		_, _ = w.Write([]byte(`{"new":[{"type":"ban","scope":"Ip","value":"192.0.2.5"}],
			"deleted":[{"type":"ban","scope":"Ip","value":"203.0.113.7"},
			           {"type":"ban","scope":"Range","value":"198.51.100.0/24"}]}`))
	}))
	defer srv.Close()

	b := compileBouncer("bnc", model.BouncerMiddleware{
		URL: srv.URL, APIKey: model.Secret("k"), Stream: true, CacheTTL: "1h",
	})
	// The verdict cache would mask the stream set on repeat lookups; this test is
	// about the set itself, so keep every request a cache miss.
	h := bouncerHandler(b, "app", clientIPResolver(nil), nil, bouncerNext(nil))
	check := func(peer string, want int) {
		t.Helper()
		b.cache = newVerdictCache(16)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, bouncerReq(peer))
		if rec.Code != want {
			t.Errorf("%s: status = %d, want %d", peer, rec.Code, want)
		}
	}

	check("203.0.113.7", http.StatusForbidden)   // exact ban
	check("198.51.100.42", http.StatusForbidden) // inside the banned range
	check("203.0.113.9", http.StatusForbidden)   // captcha is a deny
	check("203.0.113.10", http.StatusOK)         // unimplemented remediation, ignored
	check("192.0.2.1", http.StatusOK)            // never mentioned

	if got := atomic.LoadInt32(&startupCalls); got != 1 {
		t.Fatalf("startup pulls = %d, want exactly 1 (the set must be reused)", got)
	}
	if got := atomic.LoadInt32(&deltaCalls); got != 0 {
		t.Fatalf("delta pulls = %d, want 0 before the TTL elapses", got)
	}

	// Force the set stale and let one background refresh run, then re-check: the
	// delta must have added 192.0.2.5 and released both deleted decisions.
	b.stream.mu.Lock()
	b.stream.next = time.Now().Add(-time.Second)
	b.stream.mu.Unlock()
	check("192.0.2.1", http.StatusOK) // triggers the background refresh

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&deltaCalls) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&deltaCalls); got != 1 {
		t.Fatalf("delta pulls = %d, want 1", got)
	}
	// The refresh goroutine publishes under the lock; wait for it to land.
	for time.Now().Before(deadline) && !b.stream.lookup(mustIP("192.0.2.5")) {
		time.Sleep(10 * time.Millisecond)
	}

	check("192.0.2.5", http.StatusForbidden) // added by the delta
	check("203.0.113.7", http.StatusOK)      // deleted by the delta
	check("198.51.100.42", http.StatusOK)    // range deleted by the delta
	if got := atomic.LoadInt32(&startupCalls); got != 1 {
		t.Errorf("startup pulls = %d, want still 1 (a delta must not re-pull the world)", got)
	}
}

// A stream startup pull that fails leaves no set to answer from: that is an
// error, so the onError policy governs (and it must not be cached as an allow).
func TestCrowdSecStreamStartupFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rec, reached := serveBouncer(t, model.BouncerMiddleware{
		URL: srv.URL, APIKey: model.Secret("k"), Stream: true, OnError: model.BouncerOnErrorFailClosed,
	}, "203.0.113.7")
	if rec.Code != http.StatusForbidden || reached > 0 {
		t.Errorf("status = %d, reached = %d; want 403 and no upstream call", rec.Code, reached)
	}
}

// A ${ENV:...} apiKey must reach the bouncer resolved, never as the placeholder.
func TestBouncerResolvesSecretAPIKey(t *testing.T) {
	t.Setenv("GPM_TEST_BOUNCER_KEY", "resolved-key")
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	rec, _ := serveBouncer(t, model.BouncerMiddleware{
		URL: srv.URL, APIKey: model.Secret("${ENV:GPM_TEST_BOUNCER_KEY}"),
	}, "203.0.113.7")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if gotKey != "resolved-key" {
		t.Errorf("X-Api-Key = %q, want the resolved value", gotKey)
	}
}

// An unresolvable secret must not silently become an empty key that the LAPI
// rejects forever: it takes the onError path, loudly and from the first request.
func TestBouncerUnresolvableSecretTakesOnErrorPath(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	rec, _ := serveBouncer(t, model.BouncerMiddleware{
		URL:     srv.URL,
		APIKey:  model.Secret("${ENV:GPM_TEST_BOUNCER_MISSING}"),
		OnError: model.BouncerOnErrorFailClosed,
	}, "203.0.113.7")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("lapi calls = %d, want 0 (there is no key to authenticate with)", got)
	}
}

// An operator allow-list wins outright over the external feed: no lookup, no
// denial, whatever the bouncer would have said.
func TestBouncerAllowFromBypass(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`[{"type":"ban","scope":"Ip","value":"10.0.0.5"}]`))
	}))
	defer srv.Close()

	spec := model.BouncerMiddleware{URL: srv.URL, APIKey: model.Secret("k"), AllowFrom: []string{"10.0.0.0/8"}}

	rec, reached := serveBouncer(t, spec, "10.0.0.5")
	if rec.Code != http.StatusOK || reached != 1 {
		t.Errorf("allow-listed client: status = %d, reached = %d; want 200 and one upstream call", rec.Code, reached)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("lapi calls = %d, want 0 for an allow-listed client", got)
	}

	rec, reached = serveBouncer(t, spec, "203.0.113.7")
	if rec.Code != http.StatusForbidden || reached != 0 {
		t.Errorf("non-exempt client: status = %d, reached = %d; want 403 and no upstream call", rec.Code, reached)
	}
}

func TestBouncerDenyStatusOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"type":"ban","scope":"Ip","value":"203.0.113.7"}]`))
	}))
	defer srv.Close()

	rec, _ := serveBouncer(t, model.BouncerMiddleware{
		URL: srv.URL, APIKey: model.Secret("k"), DenyStatus: 429,
	}, "203.0.113.7")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want the configured 429", rec.Code)
	}
}

// Chain order: rate-limit -> access-list -> bouncer -> auth -> ... The access
// list is evaluated before the bouncer, so a list that denies an IP drops it
// without any bouncer lookup; and a banned IP is denied before the auth tier
// runs, so a botnet can never drive auth work.
func TestBouncerChainOrder(t *testing.T) {
	var bouncerCalls, authCalls int32
	lapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&bouncerCalls, 1)
		if r.URL.Query().Get("ip") == "203.0.113.7" {
			_, _ = w.Write([]byte(`[{"type":"ban","scope":"Ip","value":"203.0.113.7"}]`))
			return
		}
		_, _ = w.Write([]byte("null"))
	}))
	defer lapi.Close()

	// The auth tier is an auth-request outpost: any call to it means the request
	// reached authentication.
	outpost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&authCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer outpost.Close()

	reg := buildRegistry(model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "no-doc-net"},
			DefaultAction: model.ActionAllow,
			Rules:         []model.IPRule{{Action: model.ActionDeny, CIDR: "192.0.2.0/24"}},
		}},
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "outpost"},
			Type:        model.IdPTypeAuthRequest,
			AuthRequest: &model.AuthRequestSpec{OutpostURL: outpost.URL},
		}},
		Middlewares: []model.Middleware{
			{
				ObjectMeta: model.ObjectMeta{Name: "crowdsec"},
				Type:       model.MWTypeBouncer,
				Bouncer:    &model.BouncerMiddleware{URL: lapi.URL, APIKey: model.Secret("k")},
			},
			{
				ObjectMeta: model.ObjectMeta{Name: "sso"},
				Type:       model.MWTypeAuth,
				Auth:       &model.AuthMiddleware{IdentityProvider: "outpost", Mode: model.AuthModeAuthRequest},
			},
		},
	})

	var upstream int32
	host := model.ProxyHost{
		ObjectMeta:  model.ObjectMeta{Name: "app"},
		AccessLists: []string{"no-doc-net"},
		Middlewares: []string{"crowdsec", "sso"},
	}
	h := buildChain(bouncerNext(&upstream), host, reg, nil)

	serve := func(peer string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, bouncerReq(peer))
		return rec.Code
	}

	// 1. An IP the ACCESS LIST denies never reaches the bouncer at all.
	if code := serve("192.0.2.10"); code != http.StatusForbidden {
		t.Errorf("access-list-denied client: status = %d, want 403", code)
	}
	if got := atomic.LoadInt32(&bouncerCalls); got != 0 {
		t.Errorf("bouncer calls = %d, want 0 (the access list is evaluated first)", got)
	}
	if got := atomic.LoadInt32(&authCalls); got != 0 {
		t.Errorf("auth calls = %d, want 0", got)
	}

	// 2. A BANNED IP is denied by the bouncer and never reaches auth.
	if code := serve("203.0.113.7"); code != http.StatusForbidden {
		t.Errorf("banned client: status = %d, want 403", code)
	}
	if got := atomic.LoadInt32(&bouncerCalls); got != 1 {
		t.Errorf("bouncer calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&authCalls); got != 0 {
		t.Errorf("auth calls = %d, want 0 - a banned IP must never drive an auth subrequest", got)
	}
	if got := atomic.LoadInt32(&upstream); got != 0 {
		t.Errorf("upstream calls = %d, want 0", got)
	}

	// 3. A clean IP passes the bouncer and goes on to auth.
	if code := serve("198.51.100.4"); code != http.StatusOK {
		t.Errorf("clean client: status = %d, want 200", code)
	}
	if got := atomic.LoadInt32(&authCalls); got != 1 {
		t.Errorf("auth calls = %d, want 1 for a clean client", got)
	}
}

func mustIP(s string) net.IP { return net.ParseIP(s) }
