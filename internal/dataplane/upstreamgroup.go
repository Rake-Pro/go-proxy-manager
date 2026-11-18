package dataplane

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// healthManager owns the per-group upstream health state and the active probe
// goroutines. It lives on the data-plane Server (across reloads) and reconciles
// with the desired config on each reload: groups added start probing, groups
// removed stop, and an unchanged group KEEPS its probers and health state - so a
// config write elsewhere never resets a known-down upstream to healthy.
type healthManager struct {
	mu     sync.Mutex
	groups map[string]*groupHealth
}

func newHealthManager() *healthManager {
	return &healthManager{groups: map[string]*groupHealth{}}
}

// groupHealth is the live health state of one UpstreamGroup: the ordered
// upstreams with their up/down markers, the distribution policy, plus the probe
// goroutine's stop handle.
type groupHealth struct {
	name   string
	policy string // model.Policy*; "" = failover
	spec   groupSpec
	ups    []*upstreamHealth
	stop   chan struct{}

	// Cookie stickiness (zero stickyTTL = disabled): stickyCookie is the cookie
	// name, byLabel resolves a cookie's upstream label back to live state.
	stickyCookie string
	stickyTTL    time.Duration
	byLabel      map[string]*upstreamHealth

	// mu guards the smooth-weighted-round-robin state (upstreamHealth.rrCur).
	mu sync.Mutex
}

// groupSpec is the config subset whose change forces a state rebuild; meta-only
// edits (display name, tags) must not restart probing or reset health.
type groupSpec struct {
	Upstreams   []model.GroupUpstream
	Policy      string
	Stickiness  *model.Stickiness
	HealthCheck model.HealthCheck
}

// upstreamHealth tracks one upstream's availability. healthy is read on every
// proxied request (lock-free); the rise/fall counters are fed by both the
// active probe and passive connect failures from live traffic, under mu.
type upstreamHealth struct {
	up     model.Upstream
	addr   string // host:port dial target
	label  string // scheme://host:port for logs/status
	group  string // owning UpstreamGroup name, for the notify hook
	weight int    // effective weight (>= 1)

	healthy atomic.Bool
	// active counts in-flight requests currently held by this upstream
	// (incremented per attempt, decremented on error or when the response body
	// is closed), driving least-connections ordering.
	active atomic.Int64
	// rrCur is the smooth-weighted-round-robin running weight, guarded by the
	// owning groupHealth.mu (not this struct's mu).
	rrCur int

	mu        sync.Mutex
	consecOK  int
	consecBad int
	rise      int
	fall      int
}

func (u *upstreamHealth) reportSuccess() {
	u.mu.Lock()
	u.consecBad = 0
	if u.healthy.Load() {
		u.mu.Unlock()
		return
	}
	u.consecOK++
	flipped := u.consecOK >= u.rise
	if flipped {
		u.consecOK = 0
		u.healthy.Store(true)
	}
	u.mu.Unlock()
	if flipped {
		log.Info().Str("upstream", u.label).Msg("upstream recovered")
		if h := upstreamHealthHook(); h != nil {
			h(UpstreamHealthEvent{Group: u.group, Upstream: u.label, Healthy: true})
		}
	}
}

func (u *upstreamHealth) reportFailure() {
	u.mu.Lock()
	u.consecOK = 0
	if !u.healthy.Load() {
		u.mu.Unlock()
		return
	}
	u.consecBad++
	flipped := u.consecBad >= u.fall
	if flipped {
		u.consecBad = 0
		u.healthy.Store(false)
	}
	u.mu.Unlock()
	if flipped {
		log.Warn().Str("upstream", u.label).Msg("upstream marked unhealthy")
		if h := upstreamHealthHook(); h != nil {
			h(UpstreamHealthEvent{Group: u.group, Upstream: u.label, Healthy: false})
		}
	}
}

// candidates returns the upstreams to try: the healthy set ordered by the
// group's policy, then the unhealthy set in config order. Unhealthy upstreams
// are still appended so the group FAILS OPEN: with every upstream marked down,
// requests are still attempted rather than rejected outright. clientKey (the
// client IP) matters only to the ip-hash policy.
func (g *groupHealth) candidates(clientKey string) []*upstreamHealth {
	healthy := make([]*upstreamHealth, 0, len(g.ups))
	var down []*upstreamHealth
	for _, u := range g.ups {
		if u.healthy.Load() {
			healthy = append(healthy, u)
		} else {
			down = append(down, u)
		}
	}
	switch g.policy {
	case model.PolicyRoundRobin:
		g.orderRoundRobin(healthy)
	case model.PolicyLeastConnections:
		orderLeastConnections(healthy)
	case model.PolicyIPHash:
		orderIPHash(healthy, clientKey)
	}
	return append(healthy, down...)
}

// orderRoundRobin moves the smooth-weighted-round-robin pick to the front of
// healthy; the rest keep config order as the connect-failure retry tail. SWRR
// (nginx's algorithm) spreads consecutive picks proportionally to weight
// without bursts: each pick adds every weight to its upstream's running score,
// takes the highest, and charges the winner the total.
func (g *groupHealth) orderRoundRobin(healthy []*upstreamHealth) {
	if len(healthy) < 2 {
		return
	}
	g.mu.Lock()
	total := 0
	best := 0
	for i, u := range healthy {
		u.rrCur += u.weight
		total += u.weight
		if u.rrCur > healthy[best].rrCur {
			best = i
		}
	}
	healthy[best].rrCur -= total
	g.mu.Unlock()
	picked := healthy[best]
	copy(healthy[1:best+1], healthy[:best])
	healthy[0] = picked
}

// orderLeastConnections sorts healthy by in-flight requests relative to weight
// (a.active/a.weight ascending, compared cross-multiplied to stay in integers).
func orderLeastConnections(healthy []*upstreamHealth) {
	sort.SliceStable(healthy, func(i, j int) bool {
		return healthy[i].active.Load()*int64(healthy[j].weight) <
			healthy[j].active.Load()*int64(healthy[i].weight)
	})
}

// orderIPHash sorts healthy by rendezvous (highest-random-weight) hash of the
// client key and each upstream label: a client keeps its upstream while that
// upstream stays healthy, and when one dies only its clients move (no global
// reshuffle, unlike modulo hashing). The full ordering doubles as a stable
// per-client retry sequence.
func orderIPHash(healthy []*upstreamHealth, clientKey string) {
	type scored struct {
		u *upstreamHealth
		s uint64
	}
	arr := make([]scored, len(healthy))
	for i, u := range healthy {
		h := fnv.New64a()
		_, _ = h.Write([]byte(clientKey))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(u.label))
		arr[i] = scored{u: u, s: h.Sum64()}
	}
	sort.SliceStable(arr, func(i, j int) bool { return arr[i].s > arr[j].s })
	for i := range arr {
		healthy[i] = arr[i].u
	}
}

// groupResolver resolves an upstream-group name to its live health state during
// a router build. Implemented by healthStage.
type groupResolver interface {
	lookup(name string) *groupHealth
}

// stage computes the health state a new config wants WITHOUT touching the
// running probers: unchanged groups reuse their live state (probers and up/down
// markers survive the reload), while changed or new groups get fresh, not-yet-
// probing state. The router is built against the stage; only after that build
// succeeds does commit() apply it. A failed build discards the stage, leaving
// the running state exactly as it was - mirroring the atomic router swap.
// Disabled groups are dropped like absent ones (validation keeps enabled hosts
// off them). Meta-only edits (display name, tags) compare equal via groupSpec,
// so they never restart probing or reset health.
func (m *healthManager) stage(groups []model.UpstreamGroup) *healthStage {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := &healthStage{m: m, next: map[string]*groupHealth{}}
	for _, g := range groups {
		if g.Disabled {
			continue
		}
		if cur, ok := m.groups[g.Name]; ok && reflect.DeepEqual(cur.spec, specOf(g)) {
			st.next[g.Name] = cur
			continue
		}
		gh := newGroupHealth(g)
		st.next[g.Name] = gh
		st.added = append(st.added, gh)
	}
	return st
}

// healthStage is a prepared-but-inactive health reconciliation (see stage).
type healthStage struct {
	m     *healthManager
	next  map[string]*groupHealth
	added []*groupHealth // fresh state whose probers start on commit
}

func (st *healthStage) lookup(name string) *groupHealth { return st.next[name] }

// commit swaps the staged state in: probers of dropped/replaced groups stop,
// probers of fresh groups start.
func (st *healthStage) commit() {
	st.m.mu.Lock()
	defer st.m.mu.Unlock()
	for name, gh := range st.m.groups {
		if st.next[name] != gh {
			close(gh.stop)
		}
	}
	st.m.groups = st.next
	for _, gh := range st.added {
		go gh.probeLoop()
	}
}

// stopAll stops every probe goroutine (data-plane shutdown).
func (m *healthManager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, gh := range m.groups {
		close(gh.stop)
		delete(m.groups, name)
	}
}

// UpstreamStatus is one upstream's health as exposed by the status API.
type UpstreamStatus struct {
	Upstream string `json:"upstream"`
	Healthy  bool   `json:"healthy"`
	Weight   int    `json:"weight"`
	// Active is the number of in-flight requests currently held by this upstream.
	Active int64 `json:"active"`
}

// snapshot returns the current health of every group's upstreams, in config
// order, keyed by group name.
func (m *healthManager) snapshot() map[string][]UpstreamStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string][]UpstreamStatus, len(m.groups))
	for name, gh := range m.groups {
		sts := make([]UpstreamStatus, 0, len(gh.ups))
		for _, u := range gh.ups {
			sts = append(sts, UpstreamStatus{
				Upstream: u.label,
				Healthy:  u.healthy.Load(),
				Weight:   u.weight,
				Active:   u.active.Load(),
			})
		}
		out[name] = sts
	}
	return out
}

func specOf(g model.UpstreamGroup) groupSpec {
	return groupSpec{Upstreams: g.Upstreams, Policy: g.Policy, Stickiness: g.Stickiness, HealthCheck: g.HealthCheck}
}

func newGroupHealth(g model.UpstreamGroup) *groupHealth {
	gh := &groupHealth{
		name:    g.Name,
		policy:  g.Policy,
		spec:    specOf(g),
		stop:    make(chan struct{}),
		byLabel: map[string]*upstreamHealth{},
	}
	if g.Stickiness != nil {
		// Validation guarantees the TTL parses; a zero fallback just disables
		// stickiness rather than serving with a bogus duration.
		if ttl, err := g.Stickiness.ParseTTL(); err == nil {
			gh.stickyCookie = g.CookieName()
			gh.stickyTTL = ttl
		}
	}
	for _, up := range g.Upstreams {
		u := &upstreamHealth{
			up:     up.Upstream,
			addr:   net.JoinHostPort(up.Host, strconv.Itoa(up.Port)),
			label:  upstreamLabel(up.Upstream),
			group:  g.Name,
			weight: up.EffectiveWeight(),
			rise:   g.HealthCheck.RiseCount(),
			fall:   g.HealthCheck.FallCount(),
		}
		u.healthy.Store(true) // start optimistic so boot never drops traffic pre-probe
		gh.ups = append(gh.ups, u)
		gh.byLabel[u.label] = u
	}
	return gh
}

// probeLoop actively probes every upstream in the group each interval until the
// group is stopped. An immediate first round runs so a dead primary is detected
// at (re)load time, not one interval later.
func (g *groupHealth) probeLoop() {
	interval := time.Duration(g.spec.HealthCheck.Interval()) * time.Second
	timeout := time.Duration(g.spec.HealthCheck.Timeout()) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for _, u := range g.ups {
			if probeUpstream(u, g.spec.HealthCheck.Path, timeout) {
				u.reportSuccess()
			} else {
				u.reportFailure()
			}
		}
		select {
		case <-g.stop:
			return
		case <-ticker.C:
		}
	}
}

// probeHTTPClient issues health-check requests. Redirects are not followed (a
// redirect is a response, which already proves the entry point is alive) and
// connections are not pooled - each probe exercises a fresh connect, which is
// exactly the signal failover needs.
var probeHTTPClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Transport: &http.Transport{
		DisableKeepAlives: true,
		Proxy:             nil, // never route probes through an egress proxy
	},
}

// probeUpstream checks one upstream: a plain TCP connect, or - when path is set -
// an HTTP GET where any response below 500 counts as alive. The threshold is
// deliberately about the ENTRY POINT, not the application behind it: a shared
// backend app erroring would fail identically through every upstream in the
// group, so treating its errors as upstream death would just churn the group.
// A 5xx is still counted as a failure so a probe path that reflects real
// upstream brokenness (e.g. a proxy's own error page) can drive failover.
func probeUpstream(u *upstreamHealth, path string, timeout time.Duration) bool {
	if path == "" {
		c, err := net.DialTimeout("tcp", u.addr, timeout)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.up.Scheme+"://"+u.addr+path, nil)
	if err != nil {
		return false
	}
	resp, err := probeHTTPClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}

// groupTransport is the failover RoundTripper for an upstream group. It tries
// candidates in preference order, retrying ONLY on connect-phase errors - the
// request was never transmitted, so retrying is safe for any method, idempotent
// or not. An error after the request may have been sent (reset mid-response,
// upstream 5xx, timeout awaiting headers) is returned as-is: replaying it could
// double-apply a write, and an application-level failure would fail through
// every entry point equally anyway.
//
// Because http.Transport closes the request body even when the dial fails,
// bodies up to groupRetryBufBytes are buffered so each attempt replays the same
// bytes. A larger body streams through a single attempt at the preferred
// candidate instead - health-first ordering still steers it away from a
// known-down upstream, it just cannot retry mid-flight.
type groupTransport struct {
	gh   *groupHealth
	base http.RoundTripper
}

// groupRetryBufBytes caps how much of a request body is buffered to keep a
// failover retry possible. Beyond it (large uploads) the body streams and the
// request gets exactly one attempt.
const groupRetryBufBytes = 64 << 10

// Retry buffering is bounded process-wide and in time as well as per request.
// Each growth of a request's buffer is reserved from the shared budget before
// it is made, so the budget counts buffer memory actually held. A request
// whose next growth the budget cannot cover, or whose body has not ended
// groupRetryBufWait after buffering started, is sent on a single attempt
// instead, like a body over the size cap. The time bound is checked as each
// read returns. Vars only so tests can shrink them.
var (
	groupRetryBudgetBytes int64 = 32 << 20
	groupRetryBufWait           = 5 * time.Second
)

// groupRetryBufInUse is the part of groupRetryBudgetBytes currently reserved.
var groupRetryBufInUse atomic.Int64

func reserveRetryBuf(n int64) bool {
	if groupRetryBufInUse.Add(n) > groupRetryBudgetBytes {
		groupRetryBufInUse.Add(-n)
		return false
	}
	return true
}

func (t *groupTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cands := t.gh.candidates(clientKey(req))
	// Cookie stickiness: a valid, unexpired cookie naming a healthy upstream
	// promotes it ahead of the policy pick; the policy order stays behind it as
	// the connect-failure retry tail.
	pinned := t.gh.stickyUpstream(req)
	if pinned != nil {
		cands = promote(cands, pinned)
	}
	body, retriable, err := newBodySource(req)
	if err != nil {
		return nil, err
	}
	defer body.done()
	if !retriable && len(cands) > 1 {
		cands = cands[:1]
	}
	var lastErr error
	for _, u := range cands {
		if err := req.Context().Err(); err != nil {
			return nil, err // client gone; stop trying
		}
		out := req.Clone(req.Context())
		out.URL.Scheme = u.up.Scheme
		out.URL.Host = u.addr
		// Each member carries its own base path and Host-header choice, applied
		// here because only this layer knows which candidate the attempt went to.
		// req.Clone deep-copies the URL, so every attempt composes from the same
		// original path rather than stacking prefixes on a retry.
		if u.up.Path != "" {
			out.URL.Path = joinUpstreamBasePath(u.up.Path, out.URL.Path)
			out.URL.RawPath = ""
		}
		if u.up.HostHeader != "" {
			applyHostHeader(out, u.up.HostHeader, req.Host)
		}
		out.Body = body.next()
		u.active.Add(1)
		resp, err := t.base.RoundTrip(out)
		if err == nil {
			// Live traffic reaching an upstream is success evidence, feeding the
			// same rise counter as the active probe. The request stays counted
			// against the upstream until its response body is closed.
			u.reportSuccess()
			resp.Body = countBody(resp.Body, u)
			// (Re)issue the affinity cookie only when the assignment is new or
			// moved (failover / expiry / first visit) - a still-valid pin that
			// was honored needs no Set-Cookie noise. Fixed window, not sliding:
			// the TTL runs from assignment, matching "stick for X".
			if t.gh.stickyTTL > 0 && u != pinned {
				resp.Header.Add("Set-Cookie", t.gh.stickyCookieFor(u, req).String())
			}
			return resp, nil
		}
		u.active.Add(-1)
		if !isConnectError(err) {
			return nil, err
		}
		u.reportFailure()
		lastErr = err
		log.Warn().Str("group", t.gh.name).Str("upstream", u.label).Err(err).Msg("upstream connect failed, trying next")
	}
	return nil, lastErr
}

// promote moves u to the front of cands, preserving the rest of the order.
func promote(cands []*upstreamHealth, u *upstreamHealth) []*upstreamHealth {
	for i, c := range cands {
		if c == u {
			copy(cands[1:i+1], cands[:i])
			cands[0] = u
			return cands
		}
	}
	return cands
}

// stickyPayloadPrefix versions the sticky-cookie payload shape.
const stickyPayloadPrefix = "sticky2"

// stickyTag is the opaque stand-in for an upstream label inside the affinity
// cookie: a keyed hash of the group and label, so the cookie (which the client
// can decode, being signed rather than encrypted) names no upstream. The
// group resolves a tag back to an upstream by comparing against its members.
func stickyTag(group, label string) string {
	mac := hmac.New(sha256.New, ssoSigningKey())
	mac.Write([]byte("gpm-sticky-tag-v1|" + group + "|" + label))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))[:22]
}

// stickyUpstream returns the healthy upstream a request's affinity cookie pins
// it to, or nil (stickiness off, no/invalid/expired cookie, foreign group, or
// the pinned upstream is down - all of which fall back to the policy and get a
// fresh assignment cookie).
func (g *groupHealth) stickyUpstream(req *http.Request) *upstreamHealth {
	if g.stickyTTL <= 0 {
		return nil
	}
	c, err := req.Cookie(g.stickyCookie)
	if err != nil {
		return nil
	}
	payload, ok := verifyToken(macLabelSticky, c.Value)
	if !ok {
		return nil
	}
	parts := strings.Split(string(payload), "|")
	if len(parts) != 4 || parts[0] != stickyPayloadPrefix || parts[1] != g.name {
		return nil
	}
	exp, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		return nil // TTL enforced server-side; a replayed cookie cannot outlive it
	}
	for label, u := range g.byLabel {
		if stickyTag(g.name, label) == parts[2] {
			if !u.healthy.Load() {
				return nil
			}
			return u
		}
	}
	return nil
}

// stickyCookieFor builds the signed affinity cookie pinning a client to u. The
// expiry rides inside the signed payload (authoritative) as well as in Max-Age
// (advisory, lets well-behaved clients drop it on time). Secure follows the
// client-facing scheme so plain-HTTP hosts still get affinity.
func (g *groupHealth) stickyCookieFor(u *upstreamHealth, req *http.Request) *http.Cookie {
	exp := time.Now().Add(g.stickyTTL)
	payload := strings.Join([]string{stickyPayloadPrefix, g.name, stickyTag(g.name, u.label), strconv.FormatInt(exp.Unix(), 10)}, "|")
	return &http.Cookie{
		Name:     g.stickyCookie,
		Value:    signToken(macLabelSticky, []byte(payload)),
		Path:     "/",
		MaxAge:   int(g.stickyTTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   req.Header.Get("X-Forwarded-Proto") == "https",
	}
}

// clientKey derives the ip-hash stickiness key from the DERIVED client IP, not
// from RemoteAddr: behind a trusted proxy every request shares one peer address,
// which would pin the whole fleet to a single upstream. It falls back to the raw
// RemoteAddr (port stripped, so a client's parallel connections hash
// identically) only when no address can be derived at all.
func clientKey(req *http.Request) string {
	if ip := requestClientIP(req); ip != nil {
		return ip.String()
	}
	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		return host
	}
	return req.RemoteAddr
}

// countedBody keeps an upstream's in-flight count accurate: the request is
// released when the response body is closed (the reverse proxy always closes
// it). Close is idempotent so a double close cannot underflow the gauge.
type countedBody struct {
	io.ReadCloser
	u    *upstreamHealth
	once sync.Once
}

func (b *countedBody) Close() error {
	b.once.Do(func() { b.u.active.Add(-1) })
	return b.ReadCloser.Close()
}

// countedRWBody is countedBody for a bidirectional body: the 101 Switching
// Protocols body is the upstream connection itself, and httputil.ReverseProxy
// type-asserts it to io.ReadWriteCloser before copying both directions.
// Wrapping it as a plain ReadCloser makes every WebSocket handshake through a
// group fail with "101 switching protocols response with non-writable body"
// (a 502 to the client) and leaks the upstream connection.
type countedRWBody struct {
	*countedBody
	io.Writer
}

// countBody wraps an upstream response body so the request stays counted
// against u until the body is closed, preserving the Writer side when the
// body has one (protocol switches).
func countBody(body io.ReadCloser, u *upstreamHealth) io.ReadCloser {
	cb := &countedBody{ReadCloser: body, u: u}
	if w, ok := body.(io.Writer); ok {
		return &countedRWBody{countedBody: cb, Writer: w}
	}
	return cb
}

// bodySource hands out a fresh request Body per failover attempt (the transport
// closes the body it is given, even on a dial error, so attempts cannot share
// one reader).
type bodySource struct {
	buf     []byte        // the complete body, replayed on every attempt
	single  io.ReadCloser // non-nil for a body that gets exactly one attempt
	none    bool
	release func() // returns the budget reservation; nil when nothing is reserved
}

// newBodySource prepares req's body for (possibly repeated) sending. The bool
// reports whether the request is safe to retry: bodiless and fully-buffered
// bodies are; a body over the size cap, one still arriving after
// groupRetryBufWait, or one that found the retry budget exhausted is not (its
// remainder can stream only once).
func newBodySource(req *http.Request) (*bodySource, bool, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return &bodySource{none: true}, true, nil
	}
	data, held, complete, err := readRetryPrefix(req.Body)
	release := retryBufRelease(held)
	if err != nil {
		release()
		_ = req.Body.Close()
		return nil, false, err
	}
	if data == nil {
		return &bodySource{single: req.Body}, false, nil
	}
	if complete {
		_ = req.Body.Close()
		return &bodySource{buf: data, release: release}, true, nil
	}
	// The prefix is referenced only by the attempt body, which drops it once
	// it has been read, so a body that keeps arriving slowly holds no buffered
	// bytes for the rest of the request.
	return &bodySource{single: &prefixBody{prefix: data, rest: req.Body, release: release}, release: release}, false, nil
}

// readRetryPrefix reads body until it ends (complete), passes
// groupRetryBufBytes, is still arriving groupRetryBufWait after the first read
// started, or needs a buffer growth the retry budget cannot cover. The buffer
// never grows past groupRetryBufBytes+1. held is the budget reserved for the
// buffer, its capacity, on every return; data is nil when not even the first
// allocation could be reserved.
func readRetryPrefix(body io.Reader) (data []byte, held int64, complete bool, err error) {
	const limit = groupRetryBufBytes + 1
	deadline := time.Now().Add(groupRetryBufWait)
	for {
		if len(data) == cap(data) {
			grow := min(max(2*cap(data), 512), limit)
			if !reserveRetryBuf(int64(grow - cap(data))) {
				return data, held, false, nil
			}
			held += int64(grow - cap(data))
			nb := make([]byte, len(data), grow)
			copy(nb, data)
			data = nb
		}
		n, err := body.Read(data[len(data):cap(data)])
		data = data[:len(data)+n]
		if err == io.EOF {
			return data, held, true, nil
		}
		if err != nil {
			return nil, held, false, err
		}
		if len(data) == limit || time.Now().After(deadline) {
			return data, held, false, nil
		}
	}
}

func retryBufRelease(n int64) func() {
	var once sync.Once
	return func() { once.Do(func() { groupRetryBufInUse.Add(-n) }) }
}

// done returns the request's budget reservation once RoundTrip is over.
func (b *bodySource) done() {
	if b.release != nil {
		b.release()
	}
}

func (b *bodySource) next() io.ReadCloser {
	if b.none {
		return nil
	}
	if b.single != nil {
		return b.single
	}
	return io.NopCloser(bytes.NewReader(b.buf))
}

// prefixBody is a single-attempt body: the buffered prefix, then the rest of
// the client's body. Read drops the prefix and returns its reservation as soon
// as the prefix has been consumed; Close returns it in any case.
type prefixBody struct {
	prefix  []byte
	rest    io.ReadCloser
	release func()
}

func (p *prefixBody) Read(b []byte) (int, error) {
	if p.prefix != nil {
		n := copy(b, p.prefix)
		p.prefix = p.prefix[n:]
		if len(p.prefix) == 0 {
			p.prefix = nil
			p.release()
		}
		if n > 0 || len(b) == 0 {
			return n, nil
		}
	}
	return p.rest.Read(b)
}

func (p *prefixBody) Close() error {
	p.release()
	return p.rest.Close()
}

// isConnectError reports whether err happened while establishing the upstream
// connection - dial (refused / no route / dial timeout) or TLS handshake - i.e.
// before a single request byte was sent, making a retry against another
// upstream unconditionally safe. Anything else is treated as non-retryable.
func isConnectError(err error) bool {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return true
	}
	var rh tls.RecordHeaderError
	if errors.As(err, &rh) {
		return true
	}
	var cv *tls.CertificateVerificationError
	return errors.As(err, &cv)
}

// groupLabel names a group in debug headers / access logs, where a single
// upstream would show scheme://host:port.
func groupLabel(name string) string {
	return fmt.Sprintf("group:%s", name)
}
