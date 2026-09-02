package dataplane

import (
	"net"
	"net/http"
	"net/textproto"
	"sort"
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

	// trustedNets is the fleet-wide settings.trustedProxies set (see clientip.go).
	// It is the fallback used for requests that never reach a proxy host - the
	// access log of a 404/redirect/parked response; a matched host uses its own
	// effective set (hostTrustedProxies), which its trustedProxies may override.
	// Identity-header trust is a DIFFERENT tier computed separately, per host,
	// from forwardAuth.trustedProxies (see hostIdentityTrust).
	trustedNets []*net.IPNet
	// clientIP resolves the client IP against trustedNets (see clientIPResolver).
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
	}
	warnUnresolvedAccessListSources(cfg)
	warnLegacyClientIPTrust(cfg)
	reg.trustedNets = currentTrustedProxies()
	reg.clientIP = clientIPResolver(reg.trustedNets)
	reg.geoCountry = currentGeoDB().Country
	reg.geoLoaded = currentGeoDB().Loaded
	return reg
}

// warnUnresolvedAccessListSources logs, ONCE per reload (buildRegistry runs once
// per router build), every access-list source rule that has no fetched set in
// the ledger yet. Such a rule compiles to the empty set and matches nothing,
// which is the safe direction but is indistinguishable from a working rule in
// the request log - so it is called out where an operator will see it.
func warnUnresolvedAccessListSources(cfg model.Config) {
	sources := currentAccessListSources()
	for _, al := range cfg.AccessLists {
		seen := map[string]bool{}
		for _, r := range al.Rules {
			if r.Source == "" || seen[r.Source] {
				continue
			}
			seen[r.Source] = true
			if len(sources[model.AccessListSourceKey(al.Name, r.Source)]) == 0 {
				log.Warn().Str("accessList", al.Name).Str("source", r.Source).
					Msg("dataplane: access-list source has no fetched entries yet; rules referencing it match nothing until the next successful fetch")
			}
		}
	}
}

// warnLegacyClientIPTrust warns, ONCE per reload, when a config still relies on
// identityProvider.forwardAuth.trustedProxies to make X-Forwarded-For count.
// That field no longer influences the client IP (it governs only which peers may
// assert identity headers); settings.trustedProxies does. The two are NOT merged
// silently, because an identity-header trust and an address-rewrite trust are
// different grants and copying one into the other would widen an operator's
// intent without them asking.
//
// The message carries the exact YAML to paste into config/settings.yaml so the
// fix is a copy, not a research task. It is skipped when the operator has
// already declared trust anywhere - a fleet-wide list or any host override.
func warnLegacyClientIPTrust(cfg model.Config) {
	if len(currentTrustedProxies()) > 0 {
		return
	}
	for _, h := range cfg.ProxyHosts {
		if h.TrustedProxies != nil {
			// Present-but-empty is still a declaration ("trust nobody here"),
			// so the operator has answered this question already.
			return
		}
	}
	seen := map[string]bool{}
	var cidrs []string
	for _, p := range cfg.IdentityProviders {
		if p.ForwardAuth == nil {
			continue
		}
		for _, c := range p.ForwardAuth.TrustedProxies {
			if c == "" || seen[c] {
				continue
			}
			seen[c] = true
			cidrs = append(cidrs, c)
		}
	}
	if len(cidrs) == 0 {
		return
	}
	sort.Strings(cidrs)
	var b strings.Builder
	b.WriteString("trustedProxies:")
	for _, c := range cidrs {
		b.WriteString("\n  - ")
		b.WriteString(c)
	}
	log.Warn().Str("yaml", b.String()).
		Msg("dataplane: identityProvider.forwardAuth.trustedProxies no longer decides the client IP, only which peers may assert identity headers. settings.trustedProxies is empty, so X-Forwarded-For is ignored and every IP control (access lists, geo, rate limits, allowFrom exemptions, the access log) compares the connection peer. Add the block in the 'yaml' field to config/settings.yaml to keep resolving the client IP from X-Forwarded-For")
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
	return identityTrust(hostAuthSpecs(h, reg), reg)
}

// hostAuthSpecs collects every auth spec that applies anywhere on h, in chain
// order: the host's referenced auth middlewares and its inline auth block, then
// the same for each location. Both sources are gates of equal standing, so both
// must contribute their provider's identity headers and trusted proxies - an
// inline block that did not would leave a forward-auth header forgeable.
func hostAuthSpecs(h model.ProxyHost, reg *registry) []model.AuthMiddleware {
	var specs []model.AuthMiddleware
	referenced := func(names []string) {
		for _, name := range names {
			mw, ok := reg.middlewares[name]
			if !ok || mw.Type != model.MWTypeAuth || mw.Auth == nil {
				continue
			}
			specs = append(specs, *mw.Auth)
		}
	}
	referenced(h.Middlewares)
	if h.Auth != nil {
		specs = append(specs, *h.Auth)
	}
	for _, loc := range h.Locations {
		referenced(loc.Middlewares)
		if loc.Auth != nil {
			specs = append(specs, *loc.Auth)
		}
	}
	return specs
}

// identityTrust returns the provider-configured identity headers the given auth
// specs assert and the peer networks trusted to set them. Headers are
// deduplicated in canonical form; networks may repeat harmlessly (membership is
// an any-of test).
func identityTrust(specs []model.AuthMiddleware, reg *registry) (headers []string, trusted []*net.IPNet) {
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
	for _, spec := range specs {
		idp, ok := reg.idps[spec.IdentityProvider]
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
// A host's (or location's) INLINE auth / rateLimit block occupies the same
// position as a middleware of that kind and compiles through the same function,
// so it behaves identically; it is wrapped just outside the referenced ones,
// which means an inline gate runs FIRST and both still have to pass.
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
	return buildChainInline(proxy, host, hostInline(host), reg, ep)
}

// inlineChain is the set of INLINE middleware blocks that apply to one compiled
// route: the host's own, plus (for a location route) the location's. They are
// carried beside the host rather than folded into it because a location route is
// compiled from a COPY of its host, and a single pointer field cannot hold both
// levels' blocks at once.
type inlineChain struct {
	auth      []model.AuthMiddleware
	rateLimit []model.RateLimitMiddleware

	// stripPrefix is the normalized location prefix to remove from the request
	// path before it is forwarded, or "" for a host route and any location that
	// did not ask for it. It is carried here, not on the host copy, because it is
	// a property of the ROUTE and there is no host-level equivalent.
	stripPrefix string
}

// hostInline returns the inline blocks declared directly on a host.
func hostInline(h model.ProxyHost) inlineChain {
	var in inlineChain
	if h.Auth != nil {
		in.auth = append(in.auth, *h.Auth)
	}
	if h.RateLimit != nil {
		in.rateLimit = append(in.rateLimit, *h.RateLimit)
	}
	return in
}

// locationInline stacks a location's inline blocks on top of its host's, exactly
// as a location's middleware/access-list names are APPENDED to the host's rather
// than replacing them: a location is always at least as restrictive as its host.
func locationInline(h model.ProxyHost, loc model.Location) inlineChain {
	in := hostInline(h)
	if loc.Auth != nil {
		in.auth = append(in.auth, *loc.Auth)
	}
	if loc.RateLimit != nil {
		in.rateLimit = append(in.rateLimit, *loc.RateLimit)
	}
	if loc.StripPrefix {
		in.stripPrefix = normalizeLocationPrefix(loc.Path)
	}
	return in
}

// buildChainInline is buildChain with the inline blocks passed explicitly, so a
// location route can carry both its host's and its own. Inline blocks compile
// through the SAME per-kind function a middleware of that kind does and sit at
// the same chain position, just outside the referenced ones - so an inline gate
// runs FIRST and the two are otherwise indistinguishable at runtime.
func buildChainInline(proxy http.Handler, host model.ProxyHost, inline inlineChain, reg *registry, ep *compiledErrorPages) http.Handler {
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

	// One client-IP resolver for every IP-based control on this host (access
	// list, geo, guard, rate limit, bouncer, and the auth-request / client-cert
	// allowFrom exemptions). It reads the decision the router dispatch already
	// made from this host's effective trusted-proxy set - its own trustedProxies
	// override, else settings.trustedProxies - so all of them compare the SAME
	// address and none of them re-reads a header. See clientip.go.
	clientIP := clientIPResolver(hostTrustedProxies(host))

	// Innermost: request-path rewrites (closest to the upstream), so the
	// rewritten path is upstream-facing only and every security tier above still
	// sees the original client path.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeRewrite || mw.Rewrite == nil {
			continue
		}
		h = rewriteHandler(*mw.Rewrite, h)
	}

	// A location's prefix strip sits just OUTSIDE the rewrites, so the composed
	// path is "strip, then rewrite, then the upstream's base path" - a rewrite
	// rule is written against the path the backend's own namespace uses. It is
	// still inside every security tier, which all evaluate the original client
	// path (that is what the location itself matched on).
	if inline.stripPrefix != "" {
		h = stripPrefixHandler(inline.stripPrefix, h)
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
		h = authHandler(*mw.Auth, reg, host.Name, host.Domains, clientIP, ep, h)
	}
	// Inline auth blocks wrap OUTSIDE the referenced auth middlewares, so they
	// run first; both must pass, exactly as two referenced auth middlewares do.
	for _, a := range inline.auth {
		h = authHandler(a, reg, host.Name, host.Domains, clientIP, ep, h)
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
	// Inline rate-limit blocks, outside the referenced ones for the same reason:
	// an inline limit sheds the flood first, and every limit still applies.
	for _, rl := range inline.rateLimit {
		h = rateLimitHandler(rl, clientIP, host.Name, ep, h)
	}

	return h
}
