package dataplane

import (
	"context"
	"net"
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/clientip"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// This file is the ONE place gpm answers two questions:
//
//  1. which peers' forwarded headers do we believe (the trusted-proxy set), and
//  2. what is "the client IP" for this request.
//
// Everything that compares an IP reads the value derived here and nothing else:
// access-list rules and sources, geo lookups, rate-limit buckets, the
// basic-auth lockout key, guard / auth-request / client-cert / bouncer
// allowFrom exemptions, the access log, and the X-Forwarded-For / X-Real-IP gpm
// sends upstream. Adding a new IP-based control means calling one of these
// helpers, never re-reading a header.
//
// Three tiers of trust feed it, in order, and none substitutes for another
// (see docs/concepts/request-pipeline.md, "Client IP and the three trust
// tiers"):
//
//	L4       settings.proxyProtocol.trustedCIDRs - who may rewrite RemoteAddr
//	         itself, via a PROXY protocol header (internal/dataplane/proxyproto.go).
//	         It runs before any HTTP parsing, so by the time this file sees a
//	         request, RemoteAddr is already the L4-derived source.
//	L7       settings.trustedProxies, or a proxy host's own trustedProxies
//	         override - whose X-Forwarded-For is believed. That is this file.
//	identity identityProvider.forwardAuth.trustedProxies - who may assert
//	         Remote-User and friends (see chain.go, hostIdentityTrust). It has
//	         NO influence on the client IP.

// The trusted-proxy set and the derivation itself live in internal/clientip, so
// the admin server's login lockout can key on exactly the same address the data
// plane compares (see internal/server/authhttp.go). The wrappers below keep the
// data plane's own vocabulary and are the only thing the rest of this package
// calls.

// SetTrustedProxies compiles and installs the fleet-wide trusted-proxy set. It
// must be called BEFORE the data-plane Reload: each host's resolver is compiled
// into the router, so the list has to be in place before the router is built.
// An empty list trusts nobody, which is the safe default (RemoteAddr is the
// client).
func SetTrustedProxies(cidrs []string) { clientip.SetTrusted(cidrs) }

// currentTrustedProxies returns the installed fleet-wide set, or nil when
// SetTrustedProxies has never run (an embedder or a test that builds a router
// directly) - which is the trust-nobody default, not an error.
func currentTrustedProxies() []*net.IPNet { return clientip.Trusted() }

// compileTrustedProxies parses a trustedProxies list into networks, dropping
// (and logging) an entry that does not parse and warning on a wildcard.
func compileTrustedProxies(field string, cidrs []string) []*net.IPNet {
	return clientip.Compile(field, cidrs)
}

// clientIPInfo is the derived client-IP decision for one request: the IP every
// control compares, and whether the immediate peer was inside the effective
// trusted-proxy set (which is what decides whether an inbound X-Forwarded-For
// chain may be passed on to the upstream).
type clientIPInfo struct {
	ip          net.IP
	peerTrusted bool
}

type clientIPCtxKey struct{}

// withClientIP stashes a derived decision on the request so every tier below
// compares the SAME value. The router dispatch does this once, before the host's
// middleware chain runs.
func withClientIP(r *http.Request, info clientIPInfo) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), clientIPCtxKey{}, info))
}

// clientIPInfoOf returns the decision stashed at dispatch, if any.
func clientIPInfoOf(r *http.Request) (clientIPInfo, bool) {
	info, ok := r.Context().Value(clientIPCtxKey{}).(clientIPInfo)
	return info, ok
}

// requestClientIP is the client IP for r: the value derived at dispatch, or -
// for a handler reached without going through the router (a directly built
// chain, a test) - the connection peer, which is exactly what the trust-nobody
// default resolves to.
func requestClientIP(r *http.Request) net.IP {
	if info, ok := clientIPInfoOf(r); ok {
		return info.ip
	}
	return peerIP(r)
}

// requestPeerTrusted reports whether this request arrived from a peer inside the
// effective trusted-proxy set. False for a request that never passed dispatch,
// which is the safe reading (treat the inbound chain as unverified).
func requestPeerTrusted(r *http.Request) bool {
	info, ok := clientIPInfoOf(r)
	return ok && info.peerTrusted
}

// peerIP is the IP of the immediate connection peer (RemoteAddr). It is never
// spoofable by a forwarded header and is the basis for every trust decision.
// On a PROXY-protocol listener it is already the L4-asserted source, because
// proxyProtoConn overrides RemoteAddr before net/http reads it.
func peerIP(r *http.Request) net.IP { return clientip.PeerIP(r) }

// deriveClientIP is the data plane's view of clientip.Derive: the
// rightmost-untrusted walk, plus whether the immediate peer was trusted (which
// is what decides whether an inbound chain may be passed on upstream).
func deriveClientIP(r *http.Request, trusted []*net.IPNet) clientIPInfo {
	ip, peerTrusted := clientip.Derive(r, trusted)
	return clientIPInfo{ip: ip, peerTrusted: peerTrusted}
}

// clientIPResolver returns the client-IP function a middleware chain is built
// with. It prefers the decision already made at dispatch so every tier of a
// request compares one value, and derives from trusted only when there is none
// (a chain built directly, outside the router).
func clientIPResolver(trusted []*net.IPNet) func(*http.Request) net.IP {
	return func(r *http.Request) net.IP {
		if info, ok := clientIPInfoOf(r); ok {
			return info.ip
		}
		return deriveClientIP(r, trusted).ip
	}
}

// hostTrustedProxies is the effective trusted-proxy set for one host: its own
// trustedProxies override when it declares one, otherwise the fleet-wide
// settings.trustedProxies. The override REPLACES rather than extends, so a host
// reached directly can trust nothing while the fleet default is non-empty.
func hostTrustedProxies(h model.ProxyHost) []*net.IPNet {
	if h.TrustedProxies != nil {
		// PRESENT and empty means "trust nobody", which is a different answer
		// from absent ("inherit the fleet list") - hence the pointer on the
		// field. len() cannot tell the two apart, and reading an explicit
		// "trust nothing" as "inherit" trusts MORE than the operator asked for.
		return compileTrustedProxies("proxyHost "+h.Name+".trustedProxies", *h.TrustedProxies)
	}
	return currentTrustedProxies()
}
