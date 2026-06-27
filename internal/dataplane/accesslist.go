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
	return c
}

// ipAllowed evaluates the ordered IP rules; the first matching net wins,
// otherwise the default action applies. A nil (unparseable) client IP is denied
// so a malformed peer can never satisfy a default-allow gate.
func (c accessList) ipAllowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for i, n := range c.nets {
		if n.Contains(ip) {
			return c.netActions[i]
		}
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
// client IP used for rule evaluation. The gate fails closed: a misconfigured or
// unmatched request is denied, never allowed by default.
func accessListHandler(c accessList, ipOf func(*http.Request) net.IP, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An empty access list imposes no restriction.
		if !c.hasIP && !c.hasAuth {
			next.ServeHTTP(w, r)
			return
		}
		ipOK := !c.hasIP || c.ipAllowed(ipOf(r))
		authOK := !c.hasAuth || c.authOK(r)

		var pass bool
		if c.satisfyAny && c.hasIP && c.hasAuth {
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
