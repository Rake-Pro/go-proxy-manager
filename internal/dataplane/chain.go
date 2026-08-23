package dataplane

import (
	"net"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
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
	// geoCountry resolves a client IP to an ISO-3166-1 alpha-2 country code for
	// AccessList geo rules (see internal/geoip). Never nil, but reports every IP
	// as not-found when no GeoIP database is configured.
	geoCountry func(net.IP) (string, bool)
	// health resolves upstream-group names to their live health state so a
	// group-backed host binds to the (reload-surviving) group probers. Set by
	// buildRouter; nil in chains built without a router (tests).
	health groupResolver
	// geoLoaded reports, LIVE, whether a GeoIP database is currently loaded. It
	// is consulted at request-evaluation time by accessList.ipAllowed, not baked
	// into the compiled accessList at build time - so a database that loads (or
	// is later swapped out) after this registry/router was built takes effect on
	// the very next request, no config change or restart needed (see
	// internal/geoip.Resolver.Watch). A list with geo rules but no loaded
	// database still fails CLOSED (deny), never treats every IP as unknown.
	geoLoaded func() bool
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
	reg.geoCountry = currentGeoDB().Country
	reg.geoLoaded = currentGeoDB().Loaded
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
	// mTLS client-certificate passthrough (see TLSSettings.ClientAuth.
	// IdentityHeaders). Listed unconditionally, exactly like the IdP headers
	// above: a client must never be able to assert a certificate identity to a
	// backend, whether or not the target host enables passthrough. A host that
	// overrides the subject header name adds that name to its own strip set.
	model.DefaultClientCertSubjectHeader,
	model.ClientCertSANHeader,
	model.ClientCertSerialHeader,
	model.ClientCertFingerprintHeader,
}

// baselineIdentityPrefixes strip whole header families whose member names vary:
// oauth2-proxy's X-Auth-Request-* and authentik's X-Authentik-*.
var baselineIdentityPrefixes = []string{"X-Auth-Request-", "X-Authentik-"}

// identityPrefixExceptions are headers that match a baseline prefix but are NOT
// identity assertions and must reach the backend unmodified. X-authentik-CSRF
// (canonicalized to X-Authentik-Csrf) is Authentik's CSRF token header: its web
// frontend sends it on every flow-executor API POST, read from the authentik_csrf
// cookie. It is validated against that cookie by Authentik, so forwarding it is no
// identity-escalation risk; stripping it makes Authentik reject every login with
// "CSRF Failed: CSRF token missing" when Authentik is itself proxied through gpm.
var identityPrefixExceptions = map[string]struct{}{
	"X-Authentik-Csrf": {},
}

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
		if _, keep := identityPrefixExceptions[key]; keep {
			continue
		}
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

// unresolvedRefs returns a description of the first middleware/access-list name
// on host that the registry cannot resolve, or "" when every reference lands.
// buildLocations folds a location's own names into the host copy it passes to
// buildChain, so checking the host here covers locations too.
func unresolvedRefs(host model.ProxyHost, reg *registry) string {
	for _, name := range host.AccessLists {
		if _, ok := reg.accessLists[name]; !ok {
			return "accessList " + name
		}
	}
	for _, name := range host.Middlewares {
		if _, ok := reg.middlewares[name]; !ok {
			return "middleware " + name
		}
	}
	return ""
}

// buildChain wraps the terminal proxy handler in the host's middleware chain.
// Steps run in a fixed canonical order regardless of reference order:
//
//	rate-limit -> access-list -> bouncer -> auth -> guard -> headers -> rewrite -> proxy
//
// Rewrite is innermost (closest to upstream), so the path replacement it applies
// is upstream-facing only: rate-limit/access-list/bouncer/auth/guard all evaluate
// the ORIGINAL client path, never the rewritten one.
//
// so new behaviours slot into defined positions instead of colliding as text.
// Rate limiting is outermost so a flood is shed before it can drive work in the
// auth/access-control tiers (notably a forward-auth subrequest to the IdP). The
// access-list sits just inside rate-limit, ahead of auth, so an IP the list would
// deny is dropped before any auth work (forward-auth subrequest, OIDC redirect).
// The bouncer deny hook sits between them: an operator allow-list still wins
// outright (it is evaluated first), and an IP the external bouncer has banned
// never reaches the IdP either.
//
// ep is host's own compiled errorPages override (nil if it has none), threaded
// into every gpm-generated denial/error site so a custom page renders there;
// the settings-level pages still apply as a fallback even when ep is nil (see
// serveErrorPage).
func buildChain(proxy http.Handler, host model.ProxyHost, reg *registry, ep *compiledErrorPages) http.Handler {
	// FAIL CLOSED on a name that resolves to nothing. Every loop below skips a
	// name it cannot find, which for an access list means a typo silently turns a
	// restricted host into an open one - the exact opposite of what the reference
	// was for. Config.Validate rejects dangling references and is the primary
	// guard, but it is the ONLY thing between a typo and an unauthenticated route,
	// and a security boundary should not rest on a single check in a different
	// package.
	//
	// Middlewares are treated the same way rather than merely skipped: an
	// unresolvable name there can just as easily be the auth or rate-limit tier,
	// and "serve it without the gate" is never the safer reading of an operator's
	// intent. The blast radius is deliberately ONE host (not the whole router
	// build): a config that cannot pass validation anyway should not be able to
	// take unrelated hosts down as a side effect of this defence in depth.
	if missing := unresolvedRefs(host, reg); missing != "" {
		log.Error().Str("host", host.Name).Str("unresolved", missing).
			Msg("dataplane: host references a middleware or access list that does not exist; serving 503 rather than dropping the gate")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveErrorPage(w, http.StatusServiceUnavailable, ep, host.Name, func() {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			})
		})
	}

	h := proxy

	// Per-host client-IP resolver for the IP-based controls (access-list, guard,
	// rate-limit, auth-request allow-from): X-Forwarded-For is honoured only from
	// the proxies THIS host actually trusts (the forward-auth TrustedProxies of the
	// IdPs it references), mirroring the host-scoped identity-strip model - not the
	// global union of every IdP's proxies. A host with no trusted proxy in front
	// falls back to the connection peer, so a proxy trusted by some other host can
	// no longer spoof this host's client IP via XFF (see GPM-L4).
	_, hostTrusted := hostIdentityTrust(host, reg)
	clientIP := clientIPResolver(hostTrusted)

	// Innermost: exact-match request-path rewrites (closest to the upstream), so
	// the rewritten path is upstream-facing only and every security tier above
	// still sees the original client path.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeRewrite || mw.Rewrite == nil {
			continue
		}
		h = rewriteHandler(*mw.Rewrite, h)
	}

	// Header mutations, just outside the rewrite.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeHeaders || mw.Headers == nil {
			continue
		}
		h = headersHandler(*mw.Headers, h)
	}

	// Guards (conditional deny rules), in the access-control tier.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeGuard || mw.Guard == nil {
			continue
		}
		h = guardHandler(compileGuard(*mw.Guard), clientIP, host.Name, ep, h)
	}

	// Authentication: forward-auth / auth-request / per-host OIDC gating.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeAuth || mw.Auth == nil {
			continue
		}
		h = authMiddlewareHandler(mw, reg, host.Name, host.Domains, clientIP, h)
	}

	// Bouncer deny hooks, just outside auth: an IP an operator-run bouncer
	// (CrowdSec LAPI or a generic HTTP hook) currently bans is dropped before any
	// auth work runs, and still inside the access list so an explicit operator
	// allow-list is never overridden by an external feed's verdict.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeBouncer || mw.Bouncer == nil {
			continue
		}
		h = bouncerHandler(compileBouncer(mw.Name, *mw.Bouncer), host.Name, clientIP, ep, h)
	}

	// Access lists (host-level), wrapped outside auth so an IP the list would
	// deny is dropped before any auth work runs (no forward-auth subrequest to
	// the IdP, no OIDC redirect/401 disclosing the auth flow).
	for _, name := range host.AccessLists {
		al, ok := reg.accessLists[name]
		if !ok {
			continue
		}
		cal := compileAccessList(al)
		// geo availability (reg.geoLoaded) is intentionally NOT baked into cal
		// here: accessListHandler/ipAllowed consult it live, at request time, so
		// a database that (un)loads after this chain is built takes effect
		// without a rebuild (see accessList.ipAllowed).
		h = accessListHandler(cal, clientIP, reg.geoCountry, reg.geoLoaded, host.Name, ep, h)
	}

	// Outermost: per-client-IP rate limiting, ahead of auth/access-control so a
	// flood is rejected (429) before it can drive an auth subrequest or any other
	// per-request work.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeRateLimit || mw.RateLimit == nil {
			continue
		}
		h = rateLimitHandler(*mw.RateLimit, clientIP, host.Name, ep, h)
	}

	return h
}
