package dataplane

import (
	"context"
	"net"
	"net/http"
	"net/textproto"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

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
	// states supplies rate limiters and basic-auth failure gates that survive a
	// rebuild with an unchanged spec, and one gate per host across its routes.
	// Set by buildRouter; nil (a fresh instance per compiled step) in chains
	// built without a router.
	states *chainStates
	// owned collects the connection pools the build creates (see registry.own).
	owned []idleCloser
	// rateLimitExempt holds each access list an "@<list>" rate-limit exemption
	// of this build names, compiled once (see registry.rateLimitExemption).
	// Filled and read only while handlers are built, under rateLimitExemptMu;
	// requests use the compiled lists, never the map.
	rateLimitExempt   map[string]*rateLimitExemptList
	rateLimitExemptMu sync.Mutex
	// pathBlocks holds each path block list of this build, compiled once and
	// shared by every route that takes it (see registry.pathBlockFor).
	pathBlocks   map[string]*pathBlock
	pathBlocksMu sync.Mutex
}

// pathBlockFor returns the compiled path block list for key ("fleet" for
// settings.pathBlock, "mw:<name>" for a middleware). A compiled list is
// immutable, so the fleet list is compiled once per build instead of once per
// route of every host.
func (reg *registry) pathBlockFor(key string, spec model.PathBlockMiddleware) *pathBlock {
	reg.pathBlocksMu.Lock()
	defer reg.pathBlocksMu.Unlock()
	if p := reg.pathBlocks[key]; p != nil {
		return p
	}
	if reg.pathBlocks == nil {
		reg.pathBlocks = map[string]*pathBlock{}
	}
	p := compilePathBlock(spec, reg.rateLimitExemption)
	reg.pathBlocks[key] = p
	return p
}

// rateLimiterFor returns the limiter for one rate-limit step. owner names the
// step (host, route, middleware); the spec is part of the key.
func (reg *registry) rateLimiterFor(owner string, rl model.RateLimitMiddleware) *rateLimiter {
	spec, ok := specKey(rl)
	if reg == nil || reg.states == nil || !ok {
		return newRateLimiter(rl)
	}
	return reg.states.limiter(owner+"\x00"+spec, func() *rateLimiter { return newRateLimiter(rl) })
}

// authGateFor returns the basic-auth failure gate for owner (a host plus the
// credential source), shared by every route of that host.
func (reg *registry) authGateFor(owner string, spec any) *authGate {
	mk := func() *authGate { return newAuthGate(basicAuthLockout, maxBasicAuthFails, maxBasicAuthKeys) }
	key, ok := specKey(spec)
	if reg == nil || reg.states == nil || !ok {
		return mk()
	}
	return reg.states.gate(owner+"\x00"+key, mk)
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
			if len(sources[model.AccessListSourceKey(al.Name, r.Source)]) > 0 && len(sourceNets(sources, al, r.Source)) == 0 {
				log.Warn().Str("accessList", al.Name).Str("source", r.Source).
					Msg("dataplane: access-list source set is over its coverage budget (maxCoverage); rules referencing it match nothing")
				continue
			}
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

// baselineOverrideHeaders are request headers that ask a backend (or a proxy
// further in) to treat the request as something other than what it is: a
// different URL, method or client address. They are stripped from every
// request whose peer is not a proxy the host trusts, like the identity
// headers above, from EVERY peer, so nobody can smuggle a path past a guard
// or a location match or override the method (see
// hostHandler.stripUntrustedIdentity, which applies them). gpm sets
// X-Original-URL only on its own auth-request and bouncer calls.
var baselineOverrideHeaders = []string{
	"X-Original-Url", "X-Rewrite-Url", "X-Original-Uri", "X-Forwarded-Uri", "X-Forwarded-Method",
	"X-Http-Method-Override", "X-Http-Method", "X-Method-Override",
}

// clientAddressHeaders name a client address the way a CDN or load balancer
// does, and the other forwarded-request headers front proxies set (the
// original scheme, port, prefix, server, path or host, and the client
// certificate a front TLS terminator verified). They follow the client-IP
// trust tier: a peer whose X-Forwarded-For the host believes may carry them to
// the backend, any other peer loses them. The URL and method overrides above
// are never passed on from anyone: a CDN forwards those unchanged from the
// client, so trusting the CDN would reopen the guard bypass.
var clientAddressHeaders = []string{
	"True-Client-Ip", "X-Client-Ip", "X-Real-Ip", "Cf-Connecting-Ip", "Forwarded",
	"X-Original-Forwarded-For", "X-Cluster-Client-Ip", "Fastly-Client-Ip", "X-Azure-Clientip",
	"X-Forwarded-Port", "X-Forwarded-Prefix", "X-Forwarded-Scheme", "X-Forwarded-Ssl",
	"X-Forwarded-Server", "Front-End-Https", "X-Url-Scheme",
	"X-Forwarded-Path", "X-Envoy-Original-Path", "X-Original-Host", "X-Host",
	"X-Forwarded-Tls-Client-Cert", "X-Forwarded-Tls-Client-Cert-Info", "X-Forwarded-Client-Cert",
	"Ssl-Client-Cert", "Ssl-Client-Verify", "X-Ssl-Client-Dn",
}

// proxyWrittenHeaders are removed from every inbound request and rewritten by
// the reverse proxy itself (see setForwardedClient); only their underscore
// spellings need stripping, which stripUnderscoreSpellings does for every peer.
var proxyWrittenHeaders = []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"}

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

// stripUnderscoreSpellings deletes every header whose name contains "_" and,
// read with "_" as "-", names a header the strip lists cover. Some upstream
// servers (CGI-style environments) map both spellings to one variable, so the
// underscore form would stand in for the stripped one. keepAddress and
// keepIdentity are the peer's trust tiers, as in stripUntrustedIdentity; extra
// is the host's own identity-header set.
func stripUnderscoreSpellings(h http.Header, extra []string, keepAddress, keepIdentity bool) {
	for key := range h {
		if !strings.Contains(key, "_") {
			continue
		}
		name := textproto.CanonicalMIMEHeaderKey(strings.ReplaceAll(key, "_", "-"))
		if slices.Contains(baselineOverrideHeaders, name) || slices.Contains(proxyWrittenHeaders, name) ||
			(!keepAddress && slices.Contains(clientAddressHeaders, name)) ||
			(!keepIdentity && isIdentityHeaderName(name, extra)) {
			delete(h, key)
		}
	}
}

// isIdentityHeaderName reports whether the canonical name is one
// stripIdentityHeaders removes.
func isIdentityHeaderName(name string, extra []string) bool {
	if slices.Contains(baselineIdentityHeaders, name) || slices.Contains(extra, name) {
		return true
	}
	if _, keep := identityPrefixExceptions[name]; keep {
		return false
	}
	for _, p := range baselineIdentityPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// hostIdentityTrust returns the provider-configured identity headers a host
// asserts and the peer networks trusted to set them, scoped to this host and its
// locations only - so a proxy trusted by one host's IdP is never implicitly
// trusted to assert identity to a different host (no global trust union).
func hostIdentityTrust(h model.ProxyHost, reg *registry) (headers []string, trusted []*net.IPNet) {
	return identityTrust(hostAuthSpecs(h, reg), reg)
}

// hostAuthSpecs collects every auth spec that applies anywhere on h, in
// declaration order (not run order, which does not matter for the trust sets
// built from it): the host's referenced auth middlewares and its inline auth
// block, then the same for each location. Both sources are gates of equal standing, so both
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
// buildLocations folds a location's own middleware names into the host copy it
// passes to buildChain and carries its access lists in inline, so checking both
// covers locations too.
func unresolvedRefs(host model.ProxyHost, inline inlineChain, reg *registry) string {
	for _, name := range append(append([]string(nil), host.AccessLists...), inline.accessLists...) {
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
// The tiers run in a fixed canonical order regardless of reference order
// (within a tier, reference order does matter; see the auth tier below):
//
//	rate-limit -> path-block -> access-list -> bouncer -> auth -> guard -> headers -> rewrite -> proxy
//
// Rewrite is innermost (closest to upstream), so the path replacement it applies
// is upstream-facing only: rate-limit/path-block/access-list/bouncer/auth/guard
// all evaluate the ORIGINAL client path, never the rewritten one.
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

	// route is the normalized location prefix of a location route, "" for the
	// host route. It scopes the route's rate-limit state (see rateLimiterFor).
	route string

	// accessLists and accessListsMode are a location route's OWN access lists:
	// a second group, composed under the location's mode, wrapped outside the
	// host's group (host.AccessLists under host.AccessListsMode). Both groups
	// must pass, so a mode on one level never opens the other.
	accessLists     []string
	accessListsMode string
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
// as a location's middleware names are APPENDED to the host's rather than
// replacing them, and carries the location's own access lists as a second group
// beside the host's: a location is always at least as restrictive as its host.
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
	in.route = normalizeLocationPrefix(loc.Path)
	in.accessLists = loc.AccessLists
	in.accessListsMode = loc.AccessListsMode
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
	if missing := unresolvedRefs(host, inline, reg); missing != "" {
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

	// Authentication: forward-auth / auth-request / per-host OIDC gating. Each
	// gate wraps the previous one, so several gates run in this order: a
	// location's inline block, the host's inline block, then the referenced
	// auth middlewares LAST-referenced first (a location's references follow
	// its host's in host.Middlewares, so they run before the host's). The order
	// matters: the first gate that refuses or redirects answers the request.
	// A request on an interactive gate's own pass-through path (the
	// auth-request outpost prefix, the OIDC callback) still meets the
	// non-interactive gates that run after it, while the interactive ones step
	// aside; see passThroughGate.
	h = passThroughTerminal(h)
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeAuth || mw.Auth == nil {
			continue
		}
		h = passThroughGate(authHandlerFor(*mw.Auth, host, reg, clientIP, ep, h), h)
	}
	// Inline auth blocks wrap OUTSIDE the referenced auth middlewares, so they
	// run first (the location's, appended last, outermost); all must pass.
	for _, a := range inline.auth {
		h = passThroughGate(authHandlerFor(a, host, reg, clientIP, ep, h), h)
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
		b := compileBouncer(mw.Name, *mw.Bouncer)
		reg.own(b.client)
		h = bouncerHandler(b, host.Name, clientIP, ep, h)
	}

	// Access lists, wrapped outside auth so an IP the list would deny is
	// dropped before any auth work runs (no forward-auth subrequest to the IdP,
	// no OIDC redirect/401 disclosing the auth flow). The host's group first,
	// then a location's own group outside it; with both modes "all" this is
	// exactly one handler per list in host-then-location order, as before modes
	// existed. Any mode other than "any" composes as "all", the stricter reading.
	// Under "any" a disabled list is skipped, so disabling it revokes what it
	// grants (a group whose members are all disabled denies everyone).
	wrapAccessLists := func(names []string, mode string) {
		if mode == model.AccessListsModeAny {
			var lists []accessList
			for _, name := range names {
				if al, ok := reg.accessLists[name]; ok && !al.Disabled {
					lists = append(lists, compileAccessList(al))
				}
			}
			h = anyAccessListHandler(lists, names, clientIP, reg.geoCountry, reg.geoLoaded, host.Name, ep, h)
			return
		}
		for _, name := range names {
			al, ok := reg.accessLists[name]
			if !ok {
				continue
			}
			cal := compileAccessList(al)
			if cal.hasAuth {
				//lint:ignore SA1019 compat read of deprecated AccessList.BasicAuth: its failure gate is shared per host like mode: basic
				cal.gate = reg.authGateFor(host.Name+"\x00acl:"+al.Name, al.BasicAuth)
			}
			// geo availability (reg.geoLoaded) is intentionally NOT baked into cal
			// here: accessListHandler/ipAllowed consult it live, at request time,
			// so a database that (un)loads after this chain is built takes effect
			// without a rebuild (see accessList.ipAllowed).
			h = accessListHandler(cal, clientIP, reg.geoCountry, reg.geoLoaded, host.Name, ep, h)
		}
	}
	wrapAccessLists(host.AccessLists, host.AccessListsMode)
	wrapAccessLists(inline.accessLists, inline.accessListsMode)

	// An "@<list>" exemption (path block and rate limit allowFrom) walks that
	// list's rules first-match per request, compiled once per build (see
	// registry.rateLimitExemption).
	listExempt := reg.rateLimitExemption

	// Path block lists, outside the access lists and authentication: a request
	// for a listed path (a dotfile, a ".php" drop path) is answered here, so it
	// never drives an access-list lookup, a bouncer call or an auth subrequest,
	// and is never proxied. Still inside the rate limit, so a scanner working
	// through a word list spends its budget like any other client. The
	// referenced middlewares are wrapped first (so the last-referenced one runs
	// first among them, a location's before its host's), then the fleet list
	// (settings.pathBlock) outermost, unless the host turned it off; the admin
	// edge host never takes the fleet list, so a list that blocks too much
	// cannot lock the edge panel out.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypePathBlock || mw.PathBlock == nil {
			continue
		}
		h = pathBlockHandler(reg.pathBlockFor("mw:"+name, *mw.PathBlock), clientIP, host.Name, ep, h)
	}
	if fleet := currentPathBlock(); fleet != nil && host.PathBlock != model.PathBlockOff && !host.AdminEdge {
		h = pathBlockHandler(reg.pathBlockFor("fleet", *fleet), clientIP, host.Name, ep, h)
	}

	// Outermost: per-client-IP rate limiting, ahead of auth/access-control so a
	// flood is rejected (429) before it can drive an auth subrequest or any other
	// per-request work.
	// Limiter state is per route (host route or one location) and per step; a
	// name referenced twice on one route gets one limiter per reference. A step
	// with scope: host drops the route from its key, so the host route and every
	// location resolve to the same limiter (the host's references come first in
	// a location's list, so the reference numbers line up).
	route := host.Name + "\x00" + inline.route + "\x00"
	scoped := func(rl model.RateLimitMiddleware) string {
		if rl.Scope == model.RateLimitScopeHost {
			return host.Name + "\x00\x00"
		}
		return route
	}
	refs := map[string]int{}
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeRateLimit || mw.RateLimit == nil {
			continue
		}
		refs[name]++
		l := reg.rateLimiterFor(scoped(*mw.RateLimit)+"mw:"+name+"#"+strconv.Itoa(refs[name]), *mw.RateLimit)
		h = rateLimitHandlerWith(l, *mw.RateLimit, clientIP, host.Name, ep, h, listExempt)
	}
	// Inline rate-limit blocks, outside the referenced ones for the same reason:
	// an inline limit sheds the flood first, and every limit still applies.
	for i, rl := range inline.rateLimit {
		l := reg.rateLimiterFor(scoped(rl)+"inline#"+strconv.Itoa(i), rl)
		h = rateLimitHandlerWith(l, rl, clientIP, host.Name, ep, h, listExempt)
	}

	return h
}

// authHandlerFor is authHandler plus the one check that needs the host object:
// a client-cert gate on a host with no tls.clientAuth of its own admits only
// its allowFrom exemptions. In steady state such a host never sees a verified
// chain (its SNI configuration asks for no certificate, and the gate refuses a
// chain verified under another host's SNI), but a keep-alive or HTTP/2
// connection that outlives a reload which moved a domain from an mTLS host,
// or removed the host's clientAuth, still carries the chain it verified
// before. Validation warns about this configuration (checkClientCertMode).
func authHandlerFor(a model.AuthMiddleware, host model.ProxyHost, reg *registry, clientIP func(*http.Request) net.IP, ep *compiledErrorPages, next http.Handler) http.Handler {
	if a.Mode != model.AuthModeClientCert || host.TLS.ClientAuth != nil {
		return authHandler(a, reg, host.Name, host.Domains, clientIP, ep, next)
	}
	allowNets := allowFromNets(a.AllowFrom)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(allowNets) > 0 && clientIP != nil {
			if ip := clientIP(r); ip != nil && ipInNets(ip, allowNets) {
				next.ServeHTTP(w, r)
				return
			}
		}
		countDenial(r, "auth-client-cert")
		refuse(w, http.StatusUnauthorized, ep, host.Name, "authentication required")
	})
}

// passThroughKey carries, on a request for an interactive gate's own
// pass-through path, the gate that serves it (see passThroughGate).
type passThroughKey struct{}

// passThroughGate wraps one compiled auth gate; next is the rest of the chain
// inside it. A non-interactive gate (basic, client-cert, forward-auth) is
// returned unchanged, so it applies its full rule to every request, including
// another gate's pass-through. An interactive gate (oidc, auth-request) that
// owns the request path does not serve it here: it hands the request on to the
// gates inside it, tagged with itself, and passThroughTerminal serves it once
// they have passed. An interactive gate that sees a request already tagged by
// an outer gate steps aside, as a sign-in flow must never wait on another
// gate's sign-in. Every other request reaches the gate as before.
func passThroughGate(gate, next http.Handler) http.Handler {
	ig, ok := gate.(interactiveGate)
	if !ok {
		return gate
	}
	if ig.owns == nil {
		// An interactive gate that failed to compile: it refuses every request
		// and has no sign-in flow to protect, so it must not step aside for a
		// request another gate tagged.
		return ig.Handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, tagged := r.Context().Value(passThroughKey{}).(http.Handler); tagged {
			next.ServeHTTP(w, r)
			return
		}
		if ig.owns(r.URL.Path) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), passThroughKey{}, ig.Handler)))
			return
		}
		ig.Handler.ServeHTTP(w, r)
	})
}

// passThroughTerminal sits just inside the innermost auth gate. A request
// tagged by passThroughGate is served by the gate that tagged it (which, on
// its own pass-through path, answers without calling its next handler), so it
// never reaches the guard, headers, rewrite or proxy steps, exactly as before.
func passThroughTerminal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gate, ok := r.Context().Value(passThroughKey{}).(http.Handler); ok {
			gate.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
