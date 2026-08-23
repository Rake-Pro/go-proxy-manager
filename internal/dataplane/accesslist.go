package dataplane

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
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
	nets         []*net.IPNet
	netActions   []bool            // parallel to nets: true=allow, false=deny
	users        map[string]string // username -> bcrypt hash
	hasIP        bool
	hasAuth      bool

	// Geo rules, evaluated after nets and before defaultAllow (see ipAllowed).
	hasGeo            bool
	geoCountryAllow   map[string]bool // non-nil => whitelist mode; countryDeny is ignored
	geoCountryDeny    map[string]bool
	geoOnUnknownAllow bool
}

func compileAccessList(al model.AccessList) accessList {
	c := accessList{
		name:         al.Name,
		satisfyAny:   al.SatisfyAny,
		defaultAllow: al.DefaultAction == model.ActionAllow,
		explicitDeny: al.DefaultAction == model.ActionDeny,
		users:        map[string]string{},
	}
	for _, r := range al.Rules {
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
		c.nets = append(c.nets, ipnet)
		c.netActions = append(c.netActions, r.Action == model.ActionAllow)
	}
	for _, u := range al.BasicAuth {
		c.users[u.Username] = u.PasswordHash
	}
	c.hasIP = len(c.nets) > 0
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
	if ip == nil {
		return false
	}
	// Geo rules configured but no database currently loaded to evaluate them:
	// fail CLOSED. This short-circuits before the explicit IP rules too, so a
	// list whose operator intended geo filtering never serves at all while that
	// filtering is blind - deny is the only safe verdict, never a default-allow
	// or onUnknown=allow. A nil geoLoaded is treated the same as "not loaded"
	// (fail closed by default) rather than silently skipping the check.
	if c.hasGeo && (geoLoaded == nil || !geoLoaded()) {
		return false
	}
	for i, n := range c.nets {
		if n.Contains(ip) {
			return c.netActions[i]
		}
	}
	if c.hasGeo {
		var country string
		var found bool
		if geoLookup != nil {
			country, found = geoLookup(ip)
		}
		if !found {
			return c.geoOnUnknownAllow
		}
		country = strings.ToUpper(country)
		if c.geoCountryAllow != nil {
			return c.geoCountryAllow[country]
		}
		if c.geoCountryDeny[country] {
			return false
		}
		// Known country, not on the deny list: geo has no verdict here, fall
		// through to defaultAllow (deny-list mode only narrows, it does not
		// itself grant access - see AccessListGeo's doc comment).
	}
	return c.defaultAllow
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
	maxBasicAuthKeys  = 4096

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
// auth. Keys are client IPs; the map is capped (maxKeys) and fails CLOSED when
// saturated, so a flood of distinct keys becomes a lockout rather than an
// unthrottled brute-force path.
type authGate struct {
	mu      sync.Mutex
	entries map[string]*authGateEntry
	window  time.Duration
	limit   int
	maxKeys int
}

type authGateEntry struct {
	fails   int
	resetAt time.Time
}

func newAuthGate(window time.Duration, limit, maxKeys int) *authGate {
	return &authGate{entries: map[string]*authGateEntry{}, window: window, limit: limit, maxKeys: maxKeys}
}

// atLimit reports whether key is currently locked out, evicting its entry if the
// window has passed. An untracked key while the map is full counts as at-limit
// (fail closed under saturation).
func (g *authGate) atLimit(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.entries[key]
	if e == nil {
		return len(g.entries) >= g.maxKeys
	}
	if time.Now().After(e.resetAt) {
		delete(g.entries, key)
		return false
	}
	return e.fails >= g.limit
}

// record counts one failure against key over a fresh window, opportunistically
// evicting expired entries so the map cannot grow without bound.
func (g *authGate) record(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	e := g.entries[key]
	if e == nil || now.After(e.resetAt) {
		if e == nil && len(g.entries) >= g.maxKeys {
			for k, ev := range g.entries {
				if now.After(ev.resetAt) {
					delete(g.entries, k)
				}
			}
			if len(g.entries) >= g.maxKeys {
				return // atLimit treats an untracked key as locked while saturated
			}
		}
		e = &authGateEntry{}
		g.entries[key] = e
	}
	e.fails++
	e.resetAt = now.Add(g.window)
}

// clear forgets key's failures, e.g. after a successful authentication.
func (g *authGate) clear(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, key)
}

// authOK verifies the request's basic-auth credentials. The bcrypt compare runs
// under bcryptSem so concurrent verifications stay bounded; r's context cancels
// the wait, so a client that goes away does not keep a slot queued.
func (c accessList) authOK(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	select {
	case bcryptSem <- struct{}{}:
		defer func() { <-bcryptSem }()
	case <-r.Context().Done():
		return false
	}
	hash, known := c.users[user]
	if !known {
		// Run one compare anyway so the unknown-user path costs the same as a
		// wrong password (no username-enumeration timing oracle).
		_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(pass))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass)) == nil
}

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
	// One gate per compiled list. A config reload rebuilds the chain and so
	// resets the counters, which is acceptable: a reload is operator-driven (an
	// admin write or a certificate renewal) and cannot be provoked by the client
	// being throttled.
	gate := newAuthGate(basicAuthLockout, maxBasicAuthFails, maxBasicAuthKeys)
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
		ipOK := true
		if c.hasIP || c.hasGeo {
			ipOK = c.ipAllowed(ipOf(r), geoLookup, geoLoaded)
		}
		// A list with geo rules and no database loaded denies everything
		// (ipAllowed fails closed). That is a distinct operational state from a
		// CIDR rule doing its job, so it gets its own denial reason - an operator
		// seeing every request refused should be able to tell "GeoIP database
		// missing" from "the access list works".
		denyReason := "access-list"
		if c.hasGeo && (geoLoaded == nil || !geoLoaded()) {
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
			switch {
			case gate.atLimit(key):
				log.Warn().Str("accessList", c.name).Str("client", key).
					Msg("data plane basic auth: client is locked out after repeated failures")
			default:
				authOK = c.authOK(r)
				if _, _, presented := r.BasicAuth(); presented {
					// Only a real attempt counts. Browsers routinely send one
					// credential-less request per fresh page load; counting those
					// would lock out normal users.
					if authOK {
						gate.clear(key)
					} else {
						gate.record(key)
					}
				}
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
	return ip.String()
}

// peerIP is the IP of the immediate connection peer (RemoteAddr). It is never
// spoofable by a forwarded header and is the basis for every trust decision.
func peerIP(r *http.Request) net.IP {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	return net.ParseIP(strings.TrimSpace(host))
}

// clientIPResolver returns a function that yields the real client IP for
// access-list evaluation. When the connection peer is a trusted proxy, it walks
// X-Forwarded-For from the right and returns the nearest entry that is not itself
// a trusted proxy; otherwise (gpm is the edge for this peer) it uses the peer IP.
// With no trusted proxies configured this is exactly RemoteAddr - the safe
// default when gpm terminates connections directly.
func clientIPResolver(trusted []*net.IPNet) func(*http.Request) net.IP {
	return func(r *http.Request) net.IP {
		peer := peerIP(r)
		if peer == nil || !ipInNets(peer, trusted) {
			return peer
		}
		xff := r.Header.Get("X-Forwarded-For")
		if xff == "" {
			return peer
		}
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip == nil || ipInNets(ip, trusted) {
				continue
			}
			return ip
		}
		return peer
	}
}

func ipInNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// parseNet parses a CIDR or bare IP into an *net.IPNet (a bare IP becomes a
// /32 or /128). Returns nil on a malformed value.
func parseNet(s string) *net.IPNet {
	if _, n, err := net.ParseCIDR(s); err == nil {
		return n
	}
	if ip := net.ParseIP(s); ip != nil {
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
	}
	return nil
}
