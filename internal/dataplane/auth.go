package dataplane

import (
	"net"
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// authHandler gates a proxied host behind an identity provider. It is the ONE
// compiler for the auth chain position: a `type: auth` middleware and the inline
// `auth` block on a proxy host or location both come through here, so the gate,
// its metrics, its error pages and its denial counting are identical.
//
//   - forward-auth: accept a trusted forward-auth identity (header-spoof safe),
//     map groups to a role, and enforce the middleware's RequiredRoles.
//   - oidc: gpm acts as the OIDC relying party for the host - unauthenticated
//     requests are redirected to the IdP and a signed SSO session cookie admits
//     subsequent ones (see oidcgate.go).
//   - client-cert: admit only requests whose TLS handshake verified a client
//     certificate, optionally mapping its subject to a role, with the same
//     AllowFrom network exemption auth-request has (see clientCertGate).
//   - basic: HTTP basic auth against the middleware's own username/bcrypt-hash
//     set, no identity provider involved, with the same AllowFrom exemption
//     (see basicAuthGate). This replaces the deprecated AccessList.basicAuth.
//
// domains are the gated host's configured domains; the OIDC gate validates the
// request Host against them before caching a per-Host relying-party client.
//
// ep is the host's resolved error pages, threaded in exactly as the access-list,
// guard, bouncer and rate-limit tiers already thread it (see buildChain). Every
// TERMINAL refusal these gates write themselves - 401, 403, and the 4xx/5xx an
// auth backend failure produces - renders through it, falling back to the same
// plain-text body as before when nothing is configured. Redirects into a sign-in
// flow and responses proxied from the IdP are deliberately untouched.
func authHandler(a model.AuthMiddleware, reg *registry, hostName string, domains []string, clientIP func(*http.Request) net.IP, ep *compiledErrorPages, next http.Handler) http.Handler {
	// client-cert takes its identity from the TLS handshake and basic from a
	// local credential set, so both are handled before the identity-provider
	// lookup - they are the two modes with no IdP.
	if a.Mode == model.AuthModeClientCert {
		return clientCertGate(a, clientIP, allowFromNets(a.AllowFrom), hostName, ep, next)
	}
	if a.Mode == model.AuthModeBasic {
		return basicAuthGate(a, hostName, clientIP, allowFromNets(a.AllowFrom), hostName, ep, next)
	}
	idpName := a.IdentityProvider
	idp, ok := reg.idps[idpName]
	if !ok {
		return failClosed(hostName, "auth references unknown identity provider "+idpName, ep)
	}

	mode := a.Mode
	if mode == "" {
		mode = idp.Type // default the mode from the IdP type
	}

	switch mode {
	case model.AuthModeForwardAuth:
		if idp.ForwardAuth == nil {
			return failClosed(hostName, "forward-auth mode requires a forward-auth identity provider", ep)
		}
		fa := auth.CompileForwardAuth(*idp.ForwardAuth, idpName)
		return forwardAuthGate(fa, idp.RoleMapping, a.RequiredRoles, hostName, ep, next)
	case model.AuthModeAuthRequest:
		if idp.AuthRequest == nil {
			return failClosed(hostName, "auth-request mode requires an auth-request identity provider", ep)
		}
		arp, err := compileAuthRequest(*idp.AuthRequest)
		if err != nil {
			return failClosed(hostName, "auth-request: "+err.Error(), ep)
		}
		return arp.handler(clientIP, allowFromNets(a.AllowFrom), hostName, ep, next)
	case model.AuthModeOIDC:
		if idp.OIDC == nil {
			return failClosed(hostName, "oidc mode requires an oidc identity provider", ep)
		}
		gate, err := compileDataOIDC(idp, a.RequiredRoles, hostName, domains)
		if err != nil {
			return failClosed(hostName, "oidc: "+err.Error(), ep)
		}
		gate.ep = ep
		return gate.handler(next)
	default:
		return failClosed(hostName, "unknown auth mode "+mode, ep)
	}
}

// forwardAuthGate enforces a trusted forward-auth identity and role policy. host
// and ep resolve the custom error page for either refusal (see refuse).
func forwardAuthGate(fa auth.ForwardAuth, rm *model.RoleMapping, required []string, host string, ep *compiledErrorPages, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := fa.Identity(r)
		if !ok {
			// Either the peer is untrusted or no identity was asserted. Strip any
			// forged identity headers before refusing so nothing leaks upstream.
			// The strip stays AHEAD of the refusal: rendering an error page must
			// not change what this request carries onward.
			fa.Strip(r)
			refuse(w, http.StatusUnauthorized, ep, host, "authentication required")
			return
		}
		role := auth.MapRole(id.Groups, rm)
		if !roleAllowed(role, required) {
			refuse(w, http.StatusForbidden, ep, host, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientCertGate admits a request only when the TLS handshake VERIFIED a client
// certificate for this host, i.e. the mTLS trust anchor (and, when configured,
// its CRL) accepted it. It is the auth-tier half of per-host mTLS: the host runs
// tls.clientAuth in "optional" mode so certless clients still reach the chain,
// and this gate is what actually refuses them - leaving an SSO middleware free
// to cover a different host or location.
//
// With clientCertRoles set, the certificate subject (RFC 2253 form, or its bare
// common name) must map to a role, and that role must satisfy requiredRoles; an
// unmapped subject is refused. With no mapping, a verified certificate is enough.
//
// allowNets are the middleware's AllowFrom networks and carry exactly the meaning
// they do in auth-request mode: a client on one of them is proxied straight
// through with no certificate requirement and no role check at all - the pattern
// for "the LAN does not need a client certificate, the internet does". Such a
// request necessarily reaches the upstream with no client-certificate identity
// headers, because those are set only from a handshake-verified certificate (see
// the identity-passthrough strip in router.go), and it has none.
func clientCertGate(spec model.AuthMiddleware, clientIP func(*http.Request) net.IP, allowNets []*net.IPNet, host string, ep *compiledErrorPages, next http.Handler) http.Handler {
	roles := spec.ClientCertRoles
	required := spec.RequiredRoles
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The network exemption is decided first, so an exempt client is never
		// asked for a certificate and never role-checked.
		if len(allowNets) > 0 && clientIP != nil {
			if ip := clientIP(r); ip != nil && ipInNets(ip, allowNets) {
				next.ServeHTTP(w, r)
				return
			}
		}
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
			refuse(w, http.StatusUnauthorized, ep, host, "authentication required")
			return
		}
		if len(roles) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		subj := r.TLS.PeerCertificates[0].Subject
		role, ok := roles[subj.String()]
		if !ok {
			role, ok = roles[subj.CommonName]
		}
		if !ok || !roleAllowed(auth.Role(role), required) {
			refuse(w, http.StatusForbidden, ep, host, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allowFromNets parses a middleware's AllowFrom CIDR/IP list into networks,
// dropping anything malformed (config validation already rejects those, so this
// is the belt).
func allowFromNets(cidrs []string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range cidrs {
		if n := parseNet(c); n != nil {
			out = append(out, n)
		}
	}
	return out
}

// roleAllowed denies RoleNone always; with no RequiredRoles any mapped role
// passes; otherwise the role must be in the required set.
func roleAllowed(role auth.Role, required []string) bool {
	if role == auth.RoleNone {
		return false
	}
	if len(required) == 0 {
		return true
	}
	for _, r := range required {
		if auth.Role(r) == role {
			return true
		}
	}
	return false
}

func failClosed(host, msg string, ep *compiledErrorPages) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		log.Warn().Str("host", host).Msg(msg)
		refuse(w, http.StatusServiceUnavailable, ep, host, "authentication not available")
	})
}
