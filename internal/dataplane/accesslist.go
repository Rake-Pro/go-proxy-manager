package dataplane

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// accessList is a compiled, fast-to-evaluate form of model.AccessList.
type accessList struct {
	name         string
	satisfyAny   bool
	defaultAllow bool
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
		c.geoOnUnknownAllow = al.Geo.OnUnknown != model.ActionDeny // default allow
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

func (c accessList) authOK(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
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
// request is denied, never allowed by default.
func accessListHandler(c accessList, ipOf func(*http.Request) net.IP, geoLookup func(net.IP) (string, bool), geoLoaded func() bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An empty access list imposes no restriction.
		if !c.hasIP && !c.hasAuth && !c.hasGeo {
			next.ServeHTTP(w, r)
			return
		}
		ipOK := true
		if c.hasIP || c.hasGeo {
			ipOK = c.ipAllowed(ipOf(r), geoLookup, geoLoaded)
		}
		authOK := !c.hasAuth || c.authOK(r)

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
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
	})
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
