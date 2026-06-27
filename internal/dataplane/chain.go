package dataplane

import (
	"net"
	"net/http"
	"net/textproto"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// registry indexes reusable config objects by name for chain assembly. It also
// precomputes the trusted-proxy networks and the set of identity headers that
// the data plane must not accept from untrusted peers.
type registry struct {
	accessLists map[string]model.AccessList
	middlewares map[string]model.Middleware
	idps        map[string]model.IdentityProvider

	// trustedNets are the peers gpm trusts to set forwarded headers (identity +
	// X-Forwarded-For), unioned across all forward-auth identity providers.
	trustedNets []*net.IPNet
	// identityHeaders is the union of every identity header a forward-auth or
	// auth-request provider relies on; they are stripped from untrusted inbound
	// requests so a client cannot forge an identity to a backend.
	identityHeaders []string
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
	hdrs := map[string]struct{}{}
	for _, p := range cfg.IdentityProviders {
		reg.idps[p.Name] = p
		if fa := p.ForwardAuth; fa != nil {
			for _, c := range fa.TrustedProxies {
				if n := parseNet(c); n != nil {
					reg.trustedNets = append(reg.trustedNets, n)
				}
			}
			for _, h := range []string{fa.UserHeader, fa.EmailHeader, fa.NameHeader, fa.GroupsHeader, fa.AMRHeader} {
				if h != "" {
					hdrs[textproto.CanonicalMIMEHeaderKey(h)] = struct{}{}
				}
			}
		}
		if ar := p.AuthRequest; ar != nil {
			ch := ar.CopyHeaders
			if len(ch) == 0 {
				ch = defaultAuthRequestHeaders
			}
			for _, h := range ch {
				hdrs[textproto.CanonicalMIMEHeaderKey(h)] = struct{}{}
			}
		}
	}
	for h := range hdrs {
		reg.identityHeaders = append(reg.identityHeaders, h)
	}
	reg.clientIP = clientIPResolver(reg.trustedNets)
	return reg
}

// buildChain wraps the terminal proxy handler in the host's middleware chain.
// Steps run in a fixed canonical order regardless of reference order:
//
//	auth -> access-list -> headers -> (rate-limit, WAF ... later) -> proxy
//
// so new behaviours slot into defined positions instead of colliding as text.
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

	// Outermost: authentication. forward-auth is enforced here; per-host OIDC
	// gating fails closed until P1 (see authMiddlewareHandler).
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeAuth || mw.Auth == nil {
			continue
		}
		h = authMiddlewareHandler(mw, reg, host.Name, h)
	}

	return h
}
