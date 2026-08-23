package dataplane

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// The bouncer middleware is a DENY HOOK, not a WAF. gpm ships no rules, no
// signatures and no detection engine: it asks an operator-run bouncer (CrowdSec
// LAPI, or any HTTP endpoint speaking the generic protocol below) whether the
// client IP is currently banned, and acts on that verdict. Everything that
// decides what "banned" means lives outside gpm, which is the whole point - a
// bundled WAF would be a rules pipeline to maintain, feed and get wrong.
//
// It sits after the access list and before auth in the chain (see buildChain):
// an operator allow-list still wins outright, and a banned IP never reaches the
// IdP, so a botnet cannot drive forward-auth subrequests or OIDC redirects.

// maxBouncerBody bounds how much of a bouncer response is read. A live decision
// lookup answers with a handful of decisions; a stream startup pull can be
// large, so it gets its own (higher) bound. Both exist so a compromised or
// broken bouncer cannot make gpm buffer unbounded memory on the request path.
const (
	maxBouncerBody       = 1 << 20  // 1 MiB, live lookups
	maxBouncerStreamBody = 64 << 20 // 64 MiB, stream startup/delta pulls
)

// crowdsecDecision is the subset of a CrowdSec LAPI decision gpm acts on.
// Scope is "Ip" or "Range" (CrowdSec capitalizes them, but compare
// case-insensitively - other origins are not consistent about it).
type crowdsecDecision struct {
	Type  string `json:"type"`
	Scope string `json:"scope"`
	Value string `json:"value"`
}

// isDeny reports whether a decision means "do not let this client through".
// ban is the obvious one. CAPTCHA is treated as a DENY: a captcha decision is
// the LAPI telling the bouncer this client must prove it is human, and gpm has
// no captcha flow to hand it - serving the request anyway would silently
// downgrade the operator's decision to "allow". Any other type (e.g. a custom
// remediation gpm does not implement) is ignored rather than guessed at.
func (d crowdsecDecision) isDeny() bool {
	switch strings.ToLower(d.Type) {
	case "ban", "captcha":
		return true
	}
	return false
}

// verdictCache is a bounded, TTL'd allow/deny cache keyed by client IP, with
// O(1) LRU eviction at the cap (same shape as the rate limiter's bucket LRU) so
// a rotating-source-IP flood cannot grow it without bound.
type verdictCache struct {
	max int
	now func() time.Time

	mu      sync.Mutex
	entries map[string]*list.Element // key -> *list.Element holding *verdictEntry
	order   *list.List               // *verdictEntry, front = LRU, back = MRU
}

type verdictEntry struct {
	key     string
	deny    bool
	expires time.Time
}

func newVerdictCache(max int) *verdictCache {
	return &verdictCache{max: max, now: time.Now, entries: map[string]*list.Element{}, order: list.New()}
}

// get returns a cached verdict for key. An expired entry is dropped and
// reported as a miss.
func (c *verdictCache) get(key string) (deny bool, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el := c.entries[key]
	if el == nil {
		return false, false
	}
	e := el.Value.(*verdictEntry)
	if !c.now().Before(e.expires) {
		c.order.Remove(el)
		delete(c.entries, key)
		return false, false
	}
	c.order.MoveToBack(el)
	return e.deny, true
}

// put stores a verdict for ttl, evicting the least-recently-used entry when the
// cache is at its bound. A non-positive ttl is not cached at all.
func (c *verdictCache) put(key string, deny bool, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	expires := c.now().Add(ttl)
	if el := c.entries[key]; el != nil {
		e := el.Value.(*verdictEntry)
		e.deny, e.expires = deny, expires
		c.order.MoveToBack(el)
		return
	}
	if c.order.Len() >= c.max {
		if front := c.order.Front(); front != nil {
			c.order.Remove(front)
			delete(c.entries, front.Value.(*verdictEntry).key)
		}
	}
	c.entries[key] = c.order.PushBack(&verdictEntry{key: key, deny: deny, expires: expires})
}

func (c *verdictCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// streamState is the in-memory decision set maintained in CrowdSec stream mode:
// a full pull at first use, then deltas every cacheTTL, so the request hot path
// is a local map/CIDR lookup with no call to the LAPI at all.
//
// Refreshes are driven by requests rather than by a background ticker on
// purpose: a chain is rebuilt on every config reload and certificate renewal, so
// a per-bouncer goroutine would leak one poller per reload. A stale set triggers
// ONE short-lived refresh goroutine and keeps serving the current decisions
// meanwhile; only the very first (startup) pull blocks a request.
type streamState struct {
	mu       sync.RWMutex
	ips      map[string]struct{}
	nets     []*net.IPNet
	ready    bool      // a startup pull has succeeded at least once
	next     time.Time // when the next delta pull is due
	inFlight bool      // a refresh goroutine is running

	startupMu sync.Mutex // serializes the blocking startup pull
}

func newStreamState() *streamState {
	return &streamState{ips: map[string]struct{}{}}
}

// lookup reports whether ip is in the current decision set.
func (s *streamState) lookup(ip net.IP) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.ips[ip.String()]; ok {
		return true
	}
	for _, n := range s.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// apply folds one stream response into the set. On a startup pull the previous
// set is replaced outright; on a delta the new decisions are added and the
// deleted ones removed.
func (s *streamState) apply(resp crowdsecStream, startup bool, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if startup {
		s.ips = map[string]struct{}{}
		s.nets = nil
	}
	for _, d := range resp.Deleted {
		if v := strings.TrimSpace(d.Value); v != "" {
			delete(s.ips, v)
			s.nets = removeNet(s.nets, v)
		}
	}
	for _, d := range resp.New {
		if !d.isDeny() {
			continue
		}
		v := strings.TrimSpace(d.Value)
		if v == "" {
			continue
		}
		if strings.EqualFold(d.Scope, "range") || strings.Contains(v, "/") {
			if n := parseNet(v); n != nil {
				s.nets = removeNet(s.nets, v)
				s.nets = append(s.nets, n)
			}
			continue
		}
		s.ips[v] = struct{}{}
	}
	s.ready = true
	s.next = time.Now().Add(ttl)
}

// removeNet drops the entry for the CIDR/IP text v from nets. Ranges are few
// relative to single IPs, so a linear scan is the right shape here.
func removeNet(nets []*net.IPNet, v string) []*net.IPNet {
	n := parseNet(v)
	if n == nil {
		return nets
	}
	want := n.String()
	out := nets[:0]
	for _, existing := range nets {
		if existing.String() == want {
			continue
		}
		out = append(out, existing)
	}
	return out
}

// crowdsecStream is the /v1/decisions/stream response envelope.
type crowdsecStream struct {
	New     []crowdsecDecision `json:"new"`
	Deleted []crowdsecDecision `json:"deleted"`
}

// bouncer is a compiled BouncerMiddleware.
type bouncer struct {
	name       string
	provider   string
	baseURL    string
	apiKey     string
	apiKeyErr  error // secret resolution failure; every request then runs the onError policy
	timeout    time.Duration
	ttl        time.Duration
	failOpen   bool
	denyStatus int
	denyWith   string
	client     *http.Client
	cache      *verdictCache
	stream     *streamState // nil unless crowdsec stream mode
	allowNets  []*net.IPNet // clients that bypass the bouncer entirely
}

func compileBouncer(name string, spec model.BouncerMiddleware) *bouncer {
	b := &bouncer{
		name:       name,
		provider:   spec.ProviderOrDefault(),
		baseURL:    strings.TrimRight(spec.URL, "/"),
		timeout:    spec.TimeoutOrDefault(),
		ttl:        spec.CacheTTLOrDefault(),
		failOpen:   spec.FailOpen(),
		denyStatus: spec.DenyStatusOrDefault(),
		denyWith:   spec.DenyWith,
		cache:      newVerdictCache(spec.CacheMaxEntriesOrDefault()),
	}
	if !spec.APIKey.IsEmpty() {
		key, err := spec.APIKey.Resolve()
		if err != nil {
			// Resolved at build, not per request, so a missing secret is one log
			// line rather than one per request. The middleware stays installed and
			// runs its onError policy: an operator who chose fail-closed still gets
			// fail-closed, and one who chose fail-open is not taken offline by a
			// mis-referenced secret.
			b.apiKeyErr = err
			log.Error().Str("middleware", name).Err(err).
				Msg("bouncer: apiKey could not be resolved; every request will take the onError path")
		}
		b.apiKey = key
	}
	for _, c := range spec.AllowFrom {
		if n := parseNet(c); n != nil {
			b.allowNets = append(b.allowNets, n)
		}
	}
	if spec.Stream && b.provider == model.BouncerProviderCrowdSec {
		b.stream = newStreamState()
	}
	// Dedicated transport, like the auth-request client: the default transport
	// caps idle connections per host at 2, which starves a per-request lookup
	// under load. The bouncer URL is internal, so proxy env vars are not honoured.
	b.client = &http.Client{
		Timeout:       b.timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         (&net.Dialer{Timeout: b.timeout, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:        64,
			MaxIdleConnsPerHost: 64,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: b.timeout,
		},
	}
	return b
}

// errCacheTTL is how long a verdict that came from an ERROR (not a real answer)
// may be cached: capped hard, so a bouncer outage cannot pin a full cacheTTL of
// guessed verdicts and keep guessing long after the bouncer recovered.
func (b *bouncer) errCacheTTL() time.Duration {
	if b.ttl < model.MaxBouncerErrorCacheTTL {
		return b.ttl
	}
	return model.MaxBouncerErrorCacheTTL
}

// verdict asks the bouncer about ip and returns whether to deny. A non-nil
// error means no usable answer was obtained; the caller applies the onError
// policy. In stream mode the answer comes from the local decision set.
func (b *bouncer) verdict(r *http.Request, ip net.IP) (deny bool, err error) {
	if b.apiKeyErr != nil {
		return false, b.apiKeyErr
	}
	if b.stream != nil {
		return b.streamVerdict(r.Context(), ip)
	}
	switch b.provider {
	case model.BouncerProviderHTTP:
		return b.httpVerdict(r, ip)
	default:
		return b.crowdsecVerdict(r.Context(), ip)
	}
}

// crowdsecVerdict performs a live LAPI bouncer lookup:
//
//	GET {url}/v1/decisions?ip=<client>   X-Api-Key: <key>
//
// The LAPI answers with a JSON array of decisions, or the literal "null" (and
// some versions an empty body) when the IP is clean. It resolves range
// decisions itself, so a single ip= query covers CIDR bans too.
func (b *bouncer) crowdsecVerdict(ctx context.Context, ip net.IP) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	u := b.baseURL + "/v1/decisions?ip=" + url.QueryEscape(ip.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-Api-Key", b.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return false, err
	}
	defer drainClose(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("crowdsec lapi: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBouncerBody))
	if err != nil {
		return false, err
	}
	trimmed := strings.TrimSpace(string(body))
	// "null" and an empty body both mean "no decisions for this IP".
	if trimmed == "" || trimmed == "null" {
		return false, nil
	}
	var decisions []crowdsecDecision
	if err := json.Unmarshal(body, &decisions); err != nil {
		return false, fmt.Errorf("crowdsec lapi: decode decisions: %w", err)
	}
	for _, d := range decisions {
		if d.isDeny() {
			return true, nil
		}
	}
	return false, nil
}

// streamVerdict answers from the local decision set, pulling it if needed. The
// first call blocks on the startup pull (there is nothing to answer from yet);
// afterwards a stale set is refreshed in the background while the current one
// keeps serving, so no request ever waits on a delta.
func (b *bouncer) streamVerdict(ctx context.Context, ip net.IP) (bool, error) {
	s := b.stream
	s.mu.RLock()
	ready, due, inFlight := s.ready, !time.Now().Before(s.next), s.inFlight
	s.mu.RUnlock()

	if !ready {
		s.startupMu.Lock()
		s.mu.RLock()
		ready = s.ready
		s.mu.RUnlock()
		if !ready {
			err := b.pullStream(ctx, true)
			s.startupMu.Unlock()
			if err != nil {
				return false, err
			}
		} else {
			s.startupMu.Unlock()
		}
		return s.lookup(ip), nil
	}

	if due && !inFlight {
		s.mu.Lock()
		start := !s.inFlight
		if start {
			s.inFlight = true
		}
		s.mu.Unlock()
		if start {
			// Detached from the request context on purpose: the refresh outlives
			// this request, and cancelling it when the client goes away would keep
			// the set stale for as long as clients keep disconnecting.
			go func() {
				defer func() {
					s.mu.Lock()
					s.inFlight = false
					s.mu.Unlock()
				}()
				bg, cancel := context.WithTimeout(context.Background(), b.timeout)
				defer cancel()
				if err := b.pullStream(bg, false); err != nil {
					log.Warn().Str("middleware", b.name).Err(err).
						Msg("bouncer: crowdsec stream refresh failed; serving the previous decision set")
					// Back off one interval rather than retrying on every request.
					s.mu.Lock()
					s.next = time.Now().Add(b.ttl)
					s.mu.Unlock()
				}
			}()
		}
	}
	return s.lookup(ip), nil
}

// pullStream fetches the decision stream and folds it into the local set.
func (b *bouncer) pullStream(ctx context.Context, startup bool) error {
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	u := b.baseURL + "/v1/decisions/stream?startup=" + map[bool]string{true: "true", false: "false"}[startup]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", b.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer drainClose(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("crowdsec lapi stream: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBouncerStreamBody))
	if err != nil {
		return err
	}
	var out crowdsecStream
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			return fmt.Errorf("crowdsec lapi stream: decode: %w", err)
		}
	}
	b.stream.apply(out, startup, b.ttl)
	return nil
}

// httpVerdict calls the generic deny hook:
//
//	GET {url}?ip=<client>&host=<host>&path=<path>
//	X-Forwarded-For: <client>
//	X-Original-URL: <absolute request URL>
//
// 2xx = allow, 403 = deny, anything else = no usable answer (onError policy).
// That contract is deliberately trivial so any bouncer - a shell script behind
// inetd, a fail2ban shim, a corporate threat feed - can implement it.
func (b *bouncer) httpVerdict(r *http.Request, ip net.IP) (bool, error) {
	ctx, cancel := context.WithTimeout(r.Context(), b.timeout)
	defer cancel()

	u, err := url.Parse(b.baseURL)
	if err != nil {
		return false, err
	}
	q := u.Query()
	q.Set("ip", ip.String())
	q.Set("host", r.Host)
	q.Set("path", r.URL.Path)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, err
	}
	// The resolved client IP, not the inbound header: whatever the client sent is
	// untrusted, and the bouncer must judge the peer gpm actually resolved.
	req.Header.Set("X-Forwarded-For", ip.String())
	req.Header.Set("X-Original-URL", absoluteURL(r))
	if b.apiKey != "" {
		req.Header.Set("X-Api-Key", b.apiKey)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return false, err
	}
	defer drainClose(resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return false, nil
	case resp.StatusCode == http.StatusForbidden:
		return true, nil
	default:
		return false, fmt.Errorf("bouncer hook: unexpected status %d", resp.StatusCode)
	}
}

// bouncerHandler gates next behind the compiled bouncer. ipOf resolves the
// client IP the same XFF-aware way every other IP control uses; ep carries the
// host's custom error pages for a denyWith: error-page denial.
func bouncerHandler(b *bouncer, hostName string, ipOf func(*http.Request) net.IP, ep *compiledErrorPages, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ipOf(r)
		if ip == nil {
			// No IP to ask about. This is not an allow: it is an unanswerable
			// query, so it takes the same onError path as an unreachable bouncer.
			if b.failOpen {
				next.ServeHTTP(w, r)
				return
			}
			b.deny(w, r, hostName, ep)
			return
		}

		// An explicit operator allow-list wins outright over the external feed:
		// no lookup, no verdict, no denial.
		if ipInNets(ip, b.allowNets) {
			next.ServeHTTP(w, r)
			return
		}

		key := ip.String()
		deny, cached := b.cache.get(key)
		if !cached {
			var err error
			deny, err = b.verdict(r, ip)
			ttl := b.ttl
			if err != nil {
				deny = !b.failOpen
				ttl = b.errCacheTTL()
				log.Warn().Str("middleware", b.name).Str("host", hostName).Str("client", key).
					Bool("failOpen", b.failOpen).Err(err).Msg("bouncer: verdict unavailable, applying onError policy")
			}
			b.cache.put(key, deny, ttl)
		}
		if deny {
			b.deny(w, r, hostName, ep)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// deny writes the denial and records it against the host's denial counter
// (countDenial, the shared metrics seam every other deny tier uses).
func (b *bouncer) deny(w http.ResponseWriter, r *http.Request, hostName string, ep *compiledErrorPages) {
	countDenial(r, "bouncer")
	plain := func() { http.Error(w, http.StatusText(b.denyStatus), b.denyStatus) }
	// denyWith "plain" opts out of the custom error page deliberately (a bare
	// status body gives a scanner nothing to fingerprint); "error-page" (the
	// default) renders the host's configured page, falling back to the same plain
	// body when none is configured.
	if b.denyWith == model.BouncerDenyWithPlain {
		plain()
		return
	}
	serveErrorPage(w, b.denyStatus, ep, hostName, plain)
}
