package dataplane

import (
	"container/list"
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/clientip"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// accessList is a compiled, fast-to-evaluate form of model.AccessList.
type accessList struct {
	name         string
	satisfyAny   bool
	defaultAllow bool
	// explicitDeny is true when DefaultAction was explicitly set to "deny".
	// A list with no IP/auth/geo dimension has nothing to match on; when the
	// operator explicitly asked to deny by default (a deliberate "locked down,
	// allow rules to follow"), such a list must deny rather than silently pass.
	// An unset or "allow" default keeps the historical open behavior.
	explicitDeny bool
	// rules are the compiled IP rules in configuration order; first match wins.
	rules []netRule
	// users is the DEPRECATED access-list basic-auth credential set (see
	// model.AccessList.BasicAuth). It shares its compiled form and its verifier
	// with the auth middleware's `mode: basic` gate - see basicauth.go.
	users   basicAuthUsers
	hasIP   bool
	hasAuth bool
	// gate is the basic-auth failure gate a router build shares across the
	// host's routes and across rebuilds (see registry.authGateFor); nil means
	// a fresh gate per handler.
	gate *authGate

	// Geo rules, evaluated after nets and before defaultAllow (see ipAllowed).
	hasGeo            bool
	geoCountryAllow   map[string]bool // non-nil => whitelist mode; countryDeny is ignored
	geoCountryDeny    map[string]bool
	geoOnUnknownAllow bool
}

// netRule is one compiled IP rule: the networks it covers, the verdict it
// yields, and - for a path-scoped rule - the exact request paths and methods it
// is limited to.
//
// A rule with a nil paths map applies to every request, which is what every rule
// did before path scoping existed. A rule WITH paths matches only when the
// cleaned request path is a key of paths and the method is a key of methods; on
// a raw stream (no request at all) it never matches, so a list mixing both kinds
// degrades to its literal rules rather than to an open gate.
type netRule struct {
	nets    []*net.IPNet
	allow   bool
	paths   map[string]struct{}
	methods map[string]struct{}
}

// matches reports whether the rule applies to a request for path with method.
// An empty path means "no request path available" (a raw stream), which a
// path-scoped rule can never satisfy.
func (r netRule) matches(ip net.IP, path, method string) bool {
	if r.paths != nil {
		if path == "" {
			return false
		}
		if _, ok := r.paths[path]; !ok {
			return false
		}
		if _, ok := r.methods[method]; !ok {
			return false
		}
	}
	for _, n := range r.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func compileAccessList(al model.AccessList) accessList {
	c := accessList{
		name: al.Name,
		//lint:ignore SA1019 compat read of deprecated AccessList.SatisfyAny for legacy access-list evaluation
		satisfyAny:   al.SatisfyAny,
		defaultAllow: al.DefaultAction == model.ActionAllow,
		explicitDeny: al.DefaultAction == model.ActionDeny,
		//lint:ignore SA1019 compat read of deprecated AccessList.BasicAuth for legacy access-list evaluation
		users: compileBasicAuthUsers(al.BasicAuth),
	}
	sources := currentAccessListSources()
	for _, r := range al.Rules {
		nr := netRule{allow: r.Action == model.ActionAllow}
		if r.Source != "" {
			// A source with no ledger entry yet (never fetched, or the fetch was
			// refused before the first success) resolves to the EMPTY set: the rule
			// matches nothing and the list falls through to its defaultAction. The
			// rule is still kept, so the list keeps its IP dimension and a
			// default-deny list stays closed rather than losing its only rule and
			// turning into a list with nothing to match on.
			nr.nets = sourceNets(sources, al, r.Source)
		} else {
			var ipnet *net.IPNet
			_, ipnet, err := net.ParseCIDR(r.CIDR)
			if err != nil {
				// A bare IP becomes a /32 or /128.
				if ip := net.ParseIP(r.CIDR); ip != nil {
					bits := 32
					if ip.To4() == nil {
						bits = 128
					}
					_, ipnet, _ = net.ParseCIDR(r.CIDR + "/" + strconv.Itoa(bits))
				}
			}
			if ipnet == nil {
				continue
			}
			nr.nets = []*net.IPNet{ipnet}
		}
		if r.PathScoped() {
			nr.paths = map[string]struct{}{}
			for _, p := range r.Paths {
				nr.paths[p] = struct{}{}
			}
			nr.methods = map[string]struct{}{}
			for _, m := range r.EffectiveMethods() {
				nr.methods[strings.ToUpper(m)] = struct{}{}
			}
		}
		c.rules = append(c.rules, nr)
	}
	c.hasIP = len(c.rules) > 0
	c.hasAuth = len(c.users) > 0

	c.hasGeo = al.Geo.HasRules()
	if c.hasGeo {
		if len(al.Geo.CountryAllow) > 0 {
			c.geoCountryAllow = map[string]bool{}
			for _, cc := range al.Geo.CountryAllow {
				c.geoCountryAllow[strings.ToUpper(cc)] = true
			}
		} else {
			c.geoCountryDeny = map[string]bool{}
			for _, cc := range al.Geo.CountryDeny {
				c.geoCountryDeny[strings.ToUpper(cc)] = true
			}
		}
		// Default for an IP the database cannot place in a country. An explicit
		// onUnknown always wins. When unset, whitelist mode (countryAllow) fails
		// CLOSED - otherwise any IP absent from the operator's GeoIP database
		// (unallocated ranges, stale-DB gaps, some cloud/VPN space) would slip
		// past a "only these countries" gate. Deny-list mode stays open on
		// unknown (it only ever narrows a default-allow posture).
		switch al.Geo.OnUnknown {
		case model.ActionAllow:
			c.geoOnUnknownAllow = true
		case model.ActionDeny:
			c.geoOnUnknownAllow = false
		default:
			c.geoOnUnknownAllow = len(al.Geo.CountryAllow) == 0
		}
	}
	return c
}

// ipAllowed evaluates, in order: the explicit IP/CIDR rules (first match
// wins), then - if configured - the geo country rules, then the list's
// defaultAction. A nil (unparseable) client IP is denied outright so a
// malformed peer can never satisfy a default-allow gate.
//
// geoLookup resolves ip to an ISO-3166-1 alpha-2 country code (see
// geoip.Resolver.Country); it may be nil when no database is configured, in
// which case every IP is treated as unknown and geoOnUnknownAllow governs.
//
// geoLoaded reports, LIVE (at evaluation time, not at compile time), whether a
// GeoIP database is currently loaded - see registry.geoLoaded in chain.go. It
// is consulted only when c.hasGeo, so this compiled accessList automatically
// honours a database that loads (or unloads) after it was compiled, without
// needing to be rebuilt.
func (c accessList) ipAllowed(ip net.IP, geoLookup func(net.IP) (string, bool), geoLoaded func() bool) bool {
	return c.allowed(ip, "", "", geoLookup, geoLoaded)
}

// allowed is the request-aware form of ipAllowed: path and method are the
// cleaned request path (see normalizeRequestPath, which runs before any
// matching, so this is always the canonical form the upstream will also see) and
// the request method. Passing an empty path means there is no request to scope
// on - the raw-stream case - and path-scoped rules are then skipped entirely.
func (c accessList) allowed(ip net.IP, path, method string, geoLookup func(net.IP) (string, bool), geoLoaded func() bool) bool {
	ok, _ := c.decide(ip, path, method, geoLookup, geoLoaded)
	return ok
}

// decide is allowed plus whether the geo rules produced the verdict (as
// opposed to an IP rule, the default action, a nil address or a missing
// database), so a denial can be reported as "geo" rather than "access-list".
func (c accessList) decide(ip net.IP, path, method string, geoLookup func(net.IP) (string, bool), geoLoaded func() bool) (allow, byGeo bool) {
	if ip == nil {
		return false, false
	}
	// Geo rules configured but no database currently loaded to evaluate them:
	// fail CLOSED. This short-circuits before the explicit IP rules too, so a
	// list whose operator intended geo filtering never serves at all while that
	// filtering is blind - deny is the only safe verdict, never a default-allow
	// or onUnknown=allow. A nil geoLoaded is treated the same as "not loaded"
	// (fail closed by default) rather than silently skipping the check.
	if c.hasGeo && (geoLoaded == nil || !geoLoaded()) {
		return false, false
	}
	for _, r := range c.rules {
		if r.matches(ip, path, method) {
			return r.allow, false
		}
	}
	if c.hasGeo {
		var country string
		var found bool
		if geoLookup != nil {
			country, found = geoLookup(ip)
		}
		if !found {
			return c.geoOnUnknownAllow, true
		}
		country = strings.ToUpper(country)
		if c.geoCountryAllow != nil {
			return c.geoCountryAllow[country], true
		}
		if c.geoCountryDeny[country] {
			return false, true
		}
		// Known country, not on the deny list: geo has no verdict here, fall
		// through to defaultAllow (deny-list mode only narrows, it does not
		// itself grant access - see AccessListGeo's doc comment).
	}
	return c.defaultAllow, false
}

// dummyBcryptHash is a syntactically valid bcrypt hash used to equalize timing
// for unknown users, so a missing username is not distinguishable from a wrong
// password by response time.
var dummyBcryptHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

const (
	// maxBasicAuthFails / basicAuthLockout / maxBasicAuthKeys mirror the admin
	// plane's local-login throttle (internal/auth, rateGate): a client IP that
	// fails this many times inside the window is locked out for the rest of it.
	maxBasicAuthFails = 5
	basicAuthLockout  = 15 * time.Minute
	maxBasicAuthKeys  = 65536

	// maxBasicAuthKnown / basicAuthKnownTTL bound the gate's list of keys that
	// recently authenticated successfully (see authGate.known).
	maxBasicAuthKnown = 1024
	basicAuthKnownTTL = 24 * time.Hour

	// maxBcryptConcurrent bounds how many bcrypt comparisons run at once across
	// the whole data plane. bcrypt is deliberately expensive (~50-100ms of CPU at
	// the cost factors in use); without a bound, one unauthenticated client can
	// convert a request flood directly into full CPU saturation and take down
	// every other host this process serves. Requests above the bound queue rather
	// than fail, so a legitimate burst is slowed, never rejected.
	maxBcryptConcurrent = 16
)

// bcryptSem bounds concurrent bcrypt work (see maxBcryptConcurrent). It is
// process-wide on purpose: the resource being protected is this process's CPU,
// not any one access list.
var bcryptSem = make(chan struct{}, maxBcryptConcurrent)

// authGate is a per-key rolling-window failure throttle, mirroring the admin
// plane's rateGate (internal/auth/authenticator.go) for the data plane's basic
// auth. Keys are client IPs (IPv6 grouped by /56, see authGateKey); the map
// is capped (maxKeys) and fails CLOSED when saturated with live entries, so a
// flood of distinct keys becomes a lockout rather than an unthrottled
// brute-force path.
//
// known is a small LRU of keys that authenticated successfully within knownTTL.
// Such a key is still tracked while the map is saturated, so returning clients
// keep working through a flood of other keys. The map can then grow past
// maxKeys by the known keys that have an entry in the window; each such key
// needs a correct password first, so the growth stays small, but it is not a
// hard bound. Its failures count as usual, and a key that reaches the failure
// limit is dropped from known.
type authGate struct {
	mu      sync.Mutex
	entries map[string]*authGateEntry
	window  time.Duration
	limit   int
	maxKeys int
	// nextSweep is the earliest time a sweep of a saturated map can free an
	// entry; until then saturatedLocked answers without scanning.
	nextSweep time.Time

	known    map[string]*list.Element // value: *authGateKnown
	knownLRU *list.List               // front = most recently authenticated
	maxKnown int
	knownTTL time.Duration
}

type authGateKnown struct {
	key     string
	expires time.Time
}

type authGateEntry struct {
	fails    int           // finished failed attempts in the window
	inflight int           // attempts whose password check has not finished
	wake     chan struct{} // closed when an attempt finishes; nil when nobody waits
	resetAt  time.Time
}

// authGateWaitMax bounds how long an attempt waits for a free slot behind the
// in-flight checks of its key (see begin).
const authGateWaitMax = 10 * time.Second

func newAuthGate(window time.Duration, limit, maxKeys int) *authGate {
	return &authGate{
		entries: map[string]*authGateEntry{}, window: window, limit: limit, maxKeys: maxKeys,
		known: map[string]*list.Element{}, knownLRU: list.New(), maxKnown: maxBasicAuthKnown, knownTTL: basicAuthKnownTTL,
	}
}

// begin admits one password check for key and returns the function that
// records its outcome, or reports that key is locked out. Only finished
// failures lock a key out. In-flight checks are counted apart: while finished
// failures plus in-flight checks would reach the limit, the attempt waits
// (bounded by ctx and authGateWaitMax) for one of them to finish and checks
// again. So a parallel burst of wrong passwords gets at most limit checks in
// total, and a parallel burst of correct ones (a page load sending the cached
// Authorization header on every sub-resource) is queued, not refused. A
// success clears the key's failures and wakes its waiters. An untracked key
// while the map is full of live entries is refused (fail closed under
// saturation), unless it recently authenticated successfully (see known).
func (g *authGate) begin(ctx context.Context, key string) (finish func(ok bool), admitted bool) {
	timeout := time.NewTimer(authGateWaitMax)
	defer timeout.Stop()
	g.mu.Lock()
	for {
		now := time.Now()
		e := g.entries[key]
		if e != nil && now.After(e.resetAt) {
			e.fails = 0
			if e.inflight == 0 {
				delete(g.entries, key)
				e = nil
			}
		}
		if e == nil {
			if !g.knownLocked(key, now) && g.saturatedLocked(now) {
				g.mu.Unlock()
				return nil, false
			}
			e = &authGateEntry{resetAt: now.Add(g.window)}
			g.entries[key] = e
		}
		if e.fails >= g.limit {
			g.mu.Unlock()
			return nil, false
		}
		if e.fails+e.inflight < g.limit {
			e.inflight++
			g.mu.Unlock()
			return func(ok bool) { g.finish(key, e, ok) }, true
		}
		if e.wake == nil {
			e.wake = make(chan struct{})
		}
		wake := e.wake
		g.mu.Unlock()
		select {
		case <-wake:
		case <-ctx.Done():
			return nil, false
		case <-timeout.C:
			return nil, false
		}
		g.mu.Lock()
	}
}

// finish records the outcome of one check admitted by begin and wakes the
// attempts waiting on key.
func (g *authGate) finish(key string, e *authGateEntry, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e.inflight--
	now := time.Now()
	if ok {
		e.fails = 0
		g.rememberLocked(key, now)
	} else {
		e.fails++
		e.resetAt = now.Add(g.window)
		if e.fails >= g.limit {
			g.forgetLocked(key)
		}
	}
	if e.wake != nil {
		close(e.wake)
		e.wake = nil
	}
	if ok && e.inflight == 0 && g.entries[key] == e {
		delete(g.entries, key)
	}
}

// saturatedLocked reports whether the map is full of LIVE entries, sweeping
// expired ones first: a map full of dead entries must not keep every new
// client locked out until a config reload. An entry with a check in flight is
// live. Called with g.mu held.
//
// A sweep records the earliest resetAt of the entries it kept that have no
// check in flight, and the next sweep waits until then: no kept entry can
// expire sooner (an in-flight entry ends with a fresh resetAt or is deleted,
// and a new entry expires a full window later), so a saturated map answers
// in O(1) between sweeps.
func (g *authGate) saturatedLocked(now time.Time) bool {
	if len(g.entries) < g.maxKeys {
		return false
	}
	if now.Before(g.nextSweep) {
		return true
	}
	next := now.Add(g.window)
	for k, ev := range g.entries {
		if ev.inflight != 0 {
			continue
		}
		if now.After(ev.resetAt) {
			delete(g.entries, k)
		} else if ev.resetAt.Before(next) {
			next = ev.resetAt
		}
	}
	g.nextSweep = next
	return len(g.entries) >= g.maxKeys
}

// knownLocked reports whether key authenticated successfully within knownTTL,
// dropping an expired record. Called with g.mu held.
func (g *authGate) knownLocked(key string, now time.Time) bool {
	el := g.known[key]
	if el == nil {
		return false
	}
	if now.After(el.Value.(*authGateKnown).expires) {
		g.forgetLocked(key)
		return false
	}
	return true
}

// rememberLocked records a successful authentication for key, evicting the
// least recently authenticated key past maxKnown. Called with g.mu held.
func (g *authGate) rememberLocked(key string, now time.Time) {
	if g.maxKnown <= 0 {
		return
	}
	if el := g.known[key]; el != nil {
		el.Value.(*authGateKnown).expires = now.Add(g.knownTTL)
		g.knownLRU.MoveToFront(el)
		return
	}
	g.known[key] = g.knownLRU.PushFront(&authGateKnown{key: key, expires: now.Add(g.knownTTL)})
	for g.knownLRU.Len() > g.maxKnown {
		g.forgetLocked(g.knownLRU.Back().Value.(*authGateKnown).key)
	}
}

// forgetLocked removes key from the known list. Called with g.mu held.
func (g *authGate) forgetLocked(key string) {
	if el := g.known[key]; el != nil {
		g.knownLRU.Remove(el)
		delete(g.known, key)
	}
}

// authOK verifies the request's basic-auth credentials against the list's
// DEPRECATED basicAuth users. The verification itself lives in basicauth.go and
// is shared with the auth middleware's `mode: basic` gate, so the two cannot
// drift.
func (c accessList) authOK(r *http.Request) bool { return c.users.verify(r) }

// accessListHandler gates next behind a compiled access list. ipOf extracts the
// client IP used for rule evaluation; geoLookup resolves that IP to a country
// for geo rules (nil if no GeoIP database is configured); geoLoaded reports,
// live, whether a GeoIP database is currently loaded (see
// accessList.ipAllowed). The gate fails closed: a misconfigured or unmatched
// request is denied, never allowed by default. host and ep resolve the custom
// error page for a 403 denial (see serveErrorPage); the basic-auth 401
// challenge is left as plain text - it is a credential prompt, not a
// gpm-generated error page.
func accessListHandler(c accessList, ipOf func(*http.Request) net.IP, geoLookup func(net.IP) (string, bool), geoLoaded func() bool, host string, ep *compiledErrorPages, next http.Handler) http.Handler {
	gate := c.gate
	if gate == nil {
		gate = newAuthGate(basicAuthLockout, maxBasicAuthFails, maxBasicAuthKeys)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A list with no IP, auth, or geo dimension has nothing to match on.
		// Honor an explicit defaultAction: deny (a deliberate "deny all") rather
		// than silently passing; an unset or "allow" default imposes no
		// restriction, as before.
		if !c.hasIP && !c.hasAuth && !c.hasGeo {
			if c.explicitDeny {
				countDenial(r, "access-list")
				serveErrorPage(w, http.StatusForbidden, ep, host, func() {
					http.Error(w, "Forbidden", http.StatusForbidden)
				})
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		ipOK, byGeo := true, false
		if c.hasIP || c.hasGeo {
			ipOK, byGeo = c.decide(ipOf(r), r.URL.Path, strings.ToUpper(r.Method), geoLookup, geoLoaded)
		}
		// A denial the geo rules decided, and a list with geo rules and no
		// database loaded (decide fails closed), both report as "geo": the first
		// so the analytics can say where refused traffic comes from, the second
		// so an operator seeing every request refused can tell "GeoIP database
		// missing" from "the access list works".
		denyReason := "access-list"
		if byGeo || (c.hasGeo && (geoLoaded == nil || !geoLoaded())) {
			denyReason = "geo"
		}

		// Basic auth: throttle per client IP before spending a bcrypt compare.
		// A locked-out client is answered exactly like a wrong password (401 +
		// challenge, never 429), so the response cannot be used as an oracle for
		// whether a username or a password is close to right - or for whether the
		// lockout is in force at all.
		authOK := !c.hasAuth
		if c.hasAuth {
			key := authGateKey(ipOf, r)
			_, _, presented := r.BasicAuth()
			switch {
			case !presented:
				// Only a real attempt counts. Browsers routinely send one
				// credential-less request per fresh page load; counting those
				// would lock out normal users. Nothing to verify: answered with
				// the challenge.
			default:
				finish, admitted := gate.begin(r.Context(), key)
				if !admitted {
					log.Warn().Str("accessList", c.name).Str("client", key).
						Msg("data plane basic auth: client is locked out after repeated failures")
					break
				}
				authOK = c.authOK(r)
				finish(authOK)
			}
		}

		var pass bool
		if c.satisfyAny && (c.hasIP || c.hasGeo) && c.hasAuth {
			pass = ipOK || authOK
		} else {
			pass = ipOK && authOK
		}
		if pass {
			next.ServeHTTP(w, r)
			return
		}
		// Prompt for credentials when basic auth could still satisfy the gate.
		if c.hasAuth && !authOK && (!c.satisfyAny || !ipOK) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+c.name+`"`)
			countDenial(r, "access-list-auth")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		countDenial(r, denyReason)
		serveErrorPage(w, http.StatusForbidden, ep, host, func() {
			http.Error(w, "Forbidden", http.StatusForbidden)
		})
	})
}

// anyAllows is the list's verdict as one member of an accessListsMode: any
// group. "any" composes allow-lists only, so a member grants only what its own
// allow rules or geo.countryAllow name, decided by the list's full verdict
// (decide, with every fail-closed state in it). These members never allow;
// validation refuses each of them under "any", and this is the runtime
// backstop:
//   - a list with the deprecated basicAuth users (its verdict is a challenge),
//   - a list with defaultAction: allow (a blocklist would grant everyone it
//     does not block),
//   - a geo deny-list (countryDeny without countryAllow: it would grant every
//     other country and every address the database cannot place), and
//   - a list with no rules and no geo, which has nothing to grant.
func (c accessList) anyAllows(ip net.IP, path, method string, geoLookup func(net.IP) (string, bool), geoLoaded func() bool) bool {
	if c.hasAuth || c.defaultAllow || (c.hasGeo && c.geoCountryAllow == nil) {
		return false
	}
	if !c.hasIP && !c.hasGeo {
		return false
	}
	return c.allowed(ip, path, method, geoLookup, geoLoaded)
}

// anyAccessListHandler gates next behind a group of compiled lists composed
// with accessListsMode: any. The request passes when at least one list allows
// it (see accessList.anyAllows); an explicit deny in one list does not veto
// another list's allow. names are the configured list names, logged on a
// denial so a 403 can be traced to the group that refused it. The metric and
// analytics reason is a fixed value, never the names: "geo" when a member has
// geo rules and no GeoIP database is loaded (the same signal a single list
// gives for that state, see accessListHandler), else "access-list-any".
func anyAccessListHandler(lists []accessList, names []string, ipOf func(*http.Request) net.IP, geoLookup func(net.IP) (string, bool), geoLoaded func() bool, host string, ep *compiledErrorPages, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ipOf(r)
		method := strings.ToUpper(r.Method)
		for _, c := range lists {
			if c.anyAllows(ip, r.URL.Path, method, geoLookup, geoLoaded) {
				next.ServeHTTP(w, r)
				return
			}
		}
		reason := "access-list-any"
		for _, c := range lists {
			if c.hasGeo && (geoLoaded == nil || !geoLoaded()) {
				reason = "geo"
				break
			}
		}
		countDenial(r, reason)
		log.Debug().Str("host", host).Str("accessListsMode", "any").Strs("accessLists", names).
			Str("client", ip.String()).Str("path", logPath(r.URL.Path)).
			Msg("dataplane: no access list in the group allowed the request")
		serveErrorPage(w, http.StatusForbidden, ep, host, func() {
			http.Error(w, "Forbidden", http.StatusForbidden)
		})
	})
}

// authGateKey is the basic-auth throttle key for a request: the access list's
// own client-IP resolver, falling back to the connection peer. An IP that cannot
// be parsed at all collapses to one shared key rather than to no key, so such
// requests are still throttled (together) instead of escaping the gate.
func authGateKey(ipOf func(*http.Request) net.IP, r *http.Request) string {
	var ip net.IP
	if ipOf != nil {
		ip = ipOf(r)
	} else {
		ip = peerIP(r)
	}
	if ip == nil {
		return "unknown"
	}
	if ip.To4() == nil {
		// IPv6 clients are grouped by /56 here, wider than the rate limiter's
		// /64: the map is bounded and fails closed when full, and a single
		// delegated prefix commonly holds 256 /64s.
		return ip.Mask(net.CIDRMask(56, 128)).String() + "/56"
	}
	return ip.String()
}

// peerIP, clientIPResolver and the rest of the client-IP derivation live in
// clientip.go: one place decides which peers are trusted and what "the client
// IP" is, and every tier here compares that value.

func ipInNets(ip net.IP, nets []*net.IPNet) bool { return clientip.InNets(ip, nets) }

// parseNet parses a CIDR or bare IP into an *net.IPNet (a bare IP becomes a
// /32 or /128). Returns nil on a malformed value.
func parseNet(s string) *net.IPNet { return clientip.ParseNet(s) }

// accessListAllowNets returns the networks an access list allows without a
// path condition: its allow cidr rules plus the fetched sets of its allow
// source rules. Path-scoped rules are skipped because they admit an address
// only for some paths and must not exempt it everywhere. A rate limit's
// "@<list>" allowFrom entry resolves through this.
func accessListAllowNets(al model.AccessList) []*net.IPNet {
	sources := currentAccessListSources()
	var nets []*net.IPNet
	for _, r := range al.Rules {
		if r.Action != model.ActionAllow || r.PathScoped() {
			continue
		}
		if r.Source != "" {
			nets = append(nets, sourceNets(sources, al, r.Source)...)
			continue
		}
		if n := parseNet(r.CIDR); n != nil {
			nets = append(nets, n)
		}
	}
	return nets
}
