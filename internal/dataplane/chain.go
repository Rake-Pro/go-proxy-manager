package dataplane

import (
	"net"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// registry indexes reusable config objects by name for chain assembly. It also
// precomputes the trusted-proxy networks used to resolve the access-list client
// IP from X-Forwarded-For.
type registry struct {
	accessLists map[string]model.AccessList
	middlewares map[string]model.Middleware
	idps        map[string]model.IdentityProvider

	// trustedNets are the peers gpm trusts for X-Forwarded-For when resolving the
	// access-list client IP, unioned across all forward-auth identity providers.
	// (Per-host identity-header trust is computed separately, see hostIdentityTrust.)
	trustedNets []*net.IPNet
	// clientIP resolves the access-list client IP, honouring X-Forwarded-For only
	// from trustedNets and otherwise using the connection peer.
	clientIP func(*http.Request) net.IP
}

func buildRegistry(cfg model.Config) *registry {
	reg := &registry{
		accessLists: map[string]model.AccessList{},
		middlewares: map[string]model.Middleware{},
		idps:        map[string]model.IdentityProvider{},
	}
	for _, a := range cfg.AccessLists {
		reg.accessLists[a.Name] = a
	}
	for _, m := range cfg.Middlewares {
		reg.middlewares[m.Name] = m
	}
	for _, p := range cfg.IdentityProviders {
		reg.idps[p.Name] = p
		if fa := p.ForwardAuth; fa != nil {
			for _, c := range fa.TrustedProxies {
				if n := parseNet(c); n != nil {
					reg.trustedNets = append(reg.trustedNets, n)
				}
			}
		}
	}
	reg.clientIP = clientIPResolver(reg.trustedNets)
	return reg
}

// baselineIdentityHeaders is the fixed set of identity headers no direct client
// may assert. They are stripped from every request whose peer is not a proxy the
// target host trusts, regardless of which IdP (if any) is configured - closing
// the gap where a client forges a header an active provider does not reference
// (e.g. Remote-User while only X-authentik-* is configured). X-Forwarded-{For,
// Host,Proto} are deliberately absent: gpm sets those itself.
var baselineIdentityHeaders = []string{
	"Remote-User", "Remote-Groups", "Remote-Name", "Remote-Email",
	"X-Forwarded-User", "X-Forwarded-Email", "X-Forwarded-Groups",
	"X-Forwarded-Preferred-Username",
	"X-User", "X-Email", "X-Groups",
	"X-Webauth-User", "X-Webauth-Email", "X-Webauth-Name", "X-Webauth-Groups",
}

// baselineIdentityPrefixes strip whole header families whose member names vary:
// oauth2-proxy's X-Auth-Request-* and authentik's X-Authentik-*.
var baselineIdentityPrefixes = []string{"X-Auth-Request-", "X-Authentik-"}

// stripIdentityHeaders deletes the baseline identity headers and prefix families,
// plus any extra (provider-configured) names, from h. Header keys are already in
// canonical form, matching the canonicalized baseline names/prefixes.
func stripIdentityHeaders(h http.Header, extra []string) {
	for _, name := range baselineIdentityHeaders {
		h.Del(name)
	}
	for _, name := range extra {
		h.Del(name)
	}
	for key := range h {
		for _, p := range baselineIdentityPrefixes {
			if strings.HasPrefix(key, p) {
				delete(h, key)
				break
			}
		}
	}
}

// hostIdentityTrust returns the provider-configured identity headers a host
// asserts and the peer networks trusted to set them, scoped to this host and its
// locations only - so a proxy trusted by one host's IdP is never implicitly
// trusted to assert identity to a different host (no global trust union).
func hostIdentityTrust(h model.ProxyHost, reg *registry) (headers []string, trusted []*net.IPNet) {
	names := append([]string{}, h.Middlewares...)
	for _, loc := range h.Locations {
		names = append(names, loc.Middlewares...)
	}
	seen := map[string]struct{}{}
	add := func(name string) {
		k := textproto.CanonicalMIMEHeaderKey(name)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		headers = append(headers, k)
	}
	for _, name := range names {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeAuth || mw.Auth == nil {
			continue
		}
		idp, ok := reg.idps[mw.Auth.IdentityProvider]
		if !ok {
			continue
		}
		if fa := idp.ForwardAuth; fa != nil {
			for _, c := range fa.TrustedProxies {
				if n := parseNet(c); n != nil {
					trusted = append(trusted, n)
				}
			}
			for _, hd := range []string{fa.UserHeader, fa.EmailHeader, fa.NameHeader, fa.GroupsHeader, fa.AMRHeader} {
				if hd != "" {
					add(hd)
				}
			}
		}
		if ar := idp.AuthRequest; ar != nil {
			ch := ar.CopyHeaders
			if len(ch) == 0 {
				ch = defaultAuthRequestHeaders
			}
			for _, hd := range ch {
				add(hd)
			}
		}
	}
	return headers, trusted
}

// buildChain wraps the terminal proxy handler in the host's middleware chain.
// Steps run in a fixed canonical order regardless of reference order:
//
//	rate-limit -> auth -> guard -> access-list -> headers -> (WAF ... later) -> proxy
//
// so new behaviours slot into defined positions instead of colliding as text.
// Rate limiting is outermost so a flood is shed before it can drive work in the
// auth/access-control tiers (notably a forward-auth subrequest to the IdP).
func buildChain(proxy http.Handler, host model.ProxyHost, reg *registry) http.Handler {
	h := proxy

	// Innermost: header mutations (closest to the upstream).
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeHeaders || mw.Headers == nil {
			continue
		}
		h = headersHandler(*mw.Headers, h)
	}

	// Access lists (host-level), applied outside the headers stage.
	for _, name := range host.AccessLists {
		al, ok := reg.accessLists[name]
		if !ok {
			continue
		}
		h = accessListHandler(compileAccessList(al), reg.clientIP, h)
	}

	// Guards (conditional deny rules), in the access-control tier.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeGuard || mw.Guard == nil {
			continue
		}
		h = guardHandler(compileGuard(*mw.Guard), reg.clientIP, h)
	}

	// Authentication: forward-auth / auth-request / per-host OIDC gating.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeAuth || mw.Auth == nil {
			continue
		}
		h = authMiddlewareHandler(mw, reg, host.Name, h)
	}

	// Outermost: per-client-IP rate limiting, ahead of auth/access-control so a
	// flood is rejected (429) before it can drive an auth subrequest or any other
	// per-request work.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeRateLimit || mw.RateLimit == nil {
			continue
		}
		h = rateLimitHandler(*mw.RateLimit, reg.clientIP, h)
	}

	return h
}
