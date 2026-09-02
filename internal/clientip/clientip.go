// Package clientip owns the ONE trusted-proxy set and the ONE client-IP
// derivation gpm compares addresses against.
//
// It lives in its own package because two independent listeners need the same
// answer: the data plane (access lists, geo, rate limits, guard/auth allowFrom
// exemptions, the access log, the X-Forwarded-For sent upstream) and the admin
// server (the login and TOTP lockout buckets). Deriving them differently is how
// a deployment that fronts the admin UI with gpm itself collapses every
// administrator into one throttle bucket.
//
// The rule is the rightmost-untrusted walk: a peer outside the trusted set IS
// the client and its X-Forwarded-For is not read at all; a peer inside it may
// name the client, and the walk takes the rightmost entry that is not itself a
// trusted proxy. An entry that cannot be parsed ENDS the walk - a token gpm
// cannot read is evidence the chain is not the one it expects, not a licence to
// believe the entry to its left.
package clientip

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// MaxForwardedEntries bounds how many X-Forwarded-For elements are examined
// (from the right, where the trustworthy hops are). A real chain is a handful of
// hops; a client that sends thousands is buying per-request work, not proxies.
// Entries beyond the cap are ignored, which can only fall back towards the peer.
const MaxForwardedEntries = 64

// global holds the compiled settings.trustedProxies list, installed once per
// config reload via SetTrusted and read by every listener.
var global atomic.Pointer[[]*net.IPNet]

// SetTrusted compiles and installs the fleet-wide trusted-proxy set. It must be
// called BEFORE a data-plane reload: each host's resolver is compiled into the
// router, so the list has to be in place before the router is built. An empty
// list trusts nobody, which is the safe default (RemoteAddr is the client).
func SetTrusted(cidrs []string) {
	nets := Compile("settings.trustedProxies", cidrs)
	global.Store(&nets)
}

// Trusted returns the installed fleet-wide set, or nil when SetTrusted has never
// run (an embedder or a test that builds a router directly) - which is the
// trust-nobody default, not an error.
func Trusted() []*net.IPNet {
	if p := global.Load(); p != nil {
		return *p
	}
	return nil
}

// Compile parses a trustedProxies list into networks. Model validation is
// authoritative and rejects a malformed entry at config-write time, so an
// unparseable value here can only come from a config that bypassed it; such an
// entry is dropped (trusting less, never more) and logged.
//
// A wildcard entry is accepted - an operator on a closed network may mean it -
// but warned about every time it is compiled, because it hands every client
// control of the IP that every access-control tier compares.
func Compile(field string, cidrs []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, c := range cidrs {
		n := ParseNet(c)
		if n == nil {
			log.Warn().Str("field", field).Str("value", c).
				Msg("clientip: trusted-proxy entry is not a CIDR or IP address; ignored (that peer's X-Forwarded-For is not believed)")
			continue
		}
		nets = append(nets, n)
	}
	for _, w := range model.TrustedProxyWildcards(cidrs) {
		log.Warn().Str("field", field).Str("value", w).
			Msg("clientip: trusted-proxy entry covers the whole address space, so EVERY peer's X-Forwarded-For is believed and the client IP becomes client-controlled; list your proxies' real addresses instead")
	}
	return nets
}

// ParseNet parses a CIDR or bare IP into an *net.IPNet (a bare IP becomes a /32
// or /128). Returns nil on a malformed value.
func ParseNet(s string) *net.IPNet {
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

// InNets reports whether ip falls inside any of nets.
func InNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// PeerIP is the IP of the immediate connection peer (RemoteAddr). It is never
// spoofable by a forwarded header and is the basis for every trust decision. On
// a PROXY-protocol listener it is already the L4-asserted source, because the
// wrapped connection overrides RemoteAddr before net/http reads it.
func PeerIP(r *http.Request) net.IP {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	return net.ParseIP(strings.TrimSpace(host))
}

// ParseForwardedEntry parses one X-Forwarded-For element. Conforming senders
// write a bare address, but a port is common enough in the wild
// ("192.0.2.1:443", "[2001:db8::1]:443") that dropping those entries would
// silently fall back to the proxy's own address.
//
// An unspecified (0.0.0.0, ::), broadcast or multicast address is reported as
// unparseable: those are not a client, and accepting one merges every request a
// misbehaving proxy could not identify into a single rate-limit, lockout and
// access-log bucket.
func ParseForwardedEntry(s string) net.IP {
	ip := parseAddr(s)
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return nil
	}
	return ip
}

func parseAddr(s string) net.IP {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return net.ParseIP(strings.Trim(h, "[]"))
	}
	return net.ParseIP(strings.Trim(s, "[]"))
}

// ForwardedChain returns the X-Forwarded-For elements of r in order. Several
// header lines concatenate, so the last element of the last line is the
// rightmost hop. The result is capped at MaxForwardedEntries, keeping the
// RIGHTMOST entries: those are the hops closest to gpm and the only ones it has
// any reason to believe.
func ForwardedChain(r *http.Request) []string {
	var parts []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for _, e := range strings.Split(v, ",") {
			if e = strings.TrimSpace(e); e != "" {
				parts = append(parts, e)
			}
		}
	}
	if len(parts) > MaxForwardedEntries {
		parts = parts[len(parts)-MaxForwardedEntries:]
	}
	return parts
}

// Derive is THE client-IP derivation, the rightmost-untrusted algorithm:
//
//   - if the peer is not inside trusted, the peer IS the client and
//     X-Forwarded-For is not read at all (a client cannot forge a header it is
//     not trusted to send);
//   - otherwise walk X-Forwarded-For from the right and take the first entry
//     that is not itself a trusted proxy - the address the outermost proxy gpm
//     trusts actually observed;
//   - an entry that does not parse as an address STOPS the walk and falls back
//     to the peer, because the chain is not the shape gpm was told to expect;
//   - if every entry is trusted, or there is no usable one, fall back to the
//     peer.
//
// It returns the derived address and whether the immediate peer was trusted.
// With no trusted proxies configured this is exactly RemoteAddr.
func Derive(r *http.Request, trusted []*net.IPNet) (net.IP, bool) {
	peer := PeerIP(r)
	if peer == nil || !InNets(peer, trusted) {
		return peer, false
	}
	parts := ForwardedChain(r)
	for i := len(parts) - 1; i >= 0; i-- {
		ip := ParseForwardedEntry(parts[i])
		if ip == nil {
			break // unreadable hop: stop believing the chain, keep the peer
		}
		if InNets(ip, trusted) {
			continue
		}
		return ip, true
	}
	return peer, true
}

// Key is the string form of the derived client IP for r under trusted, for use
// as a throttle or lockout bucket key. It falls back to the raw RemoteAddr when
// no address can be derived at all, so two peers never share a bucket by
// accident.
func Key(r *http.Request, trusted []*net.IPNet) string {
	if ip, _ := Derive(r, trusted); ip != nil {
		return ip.String()
	}
	return r.RemoteAddr
}
