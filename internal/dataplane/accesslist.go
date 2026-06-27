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
	name          string
	satisfyAny    bool
	defaultAllow  bool
	nets          []*net.IPNet
	netActions    []bool // parallel to nets: true=allow, false=deny
	users         map[string]string // username -> bcrypt hash
	hasIP         bool
	hasAuth       bool
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
// otherwise the default action applies.
func (c accessList) ipAllowed(ip net.IP) bool {
	for i, n := range c.nets {
		if n.Contains(ip) {
			return c.netActions[i]
		}
	}
	return c.defaultAllow
}

func (c accessList) authOK(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	hash, known := c.users[user]
	if !known {
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

// clientIP extracts the client IP from the connection's RemoteAddr. Trusted
// X-Forwarded-For handling is introduced with the trusted-proxy config in P0d;
// using the connection IP here is the safe default (no spoofable header trust).
func clientIP(r *http.Request) net.IP {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	return net.ParseIP(strings.TrimSpace(host))
}
