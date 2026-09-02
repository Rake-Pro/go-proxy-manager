package dataplane

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// router is an immutable compiled snapshot of the data plane: the per-domain
// handlers and the certificate resolver. A new router is built on each reload
// and swapped in atomically.
type router struct {
	hosts     map[string]*hostHandler // proxy hosts (full middleware chain)
	redirects map[string]*redirectHandler
	parked    map[string]*parkedHandler
	certs     *certResolver

	// tlsConfigs holds a per-domain TLS config for any host (of any type) that
	// pins a non-default minimum TLS version; absent = the listener default.
	tlsConfigs map[string]*tls.Config

	// clientAuth records, per domain, a host's mTLS requirement so it can be
	// re-checked per REQUEST at dispatch. mTLS is negotiated in the per-SNI
	// tls.Config, but Go does not enforce that the request's Host header matches
	// the handshake SNI - so without this gate a client could handshake with a
	// non-mTLS SNI (or none) and then target an mTLS host by Host header,
	// reaching the protected backend with no verified client certificate.
	clientAuth map[string]*clientAuthReq

	// clientIP resolves the real client IP (XFF-aware via the access-list trusted
	// nets) for logging. Identity-header stripping is host-scoped on each
	// hostHandler (see hostHandler.stripUntrustedIdentity).
	clientIP func(*http.Request) net.IP

	// securityHeaders is the settings-level default response-header set, split by
	// scope and applied set-if-absent to responses that have no proxy host to
	// carry a per-host merge: the no-such-host 404, the mTLS 421, and
	// redirect/parked hosts (all gpm-generated, so only the generated subset can
	// ever apply). A proxy host uses its own merged hostHandler.securityHeaders
	// instead. nil when settings.securityHeaders is empty.
	securityHeaders *scopedHeaders
}

// clientAuthReq is a host's per-request mTLS enforcement record. cfg is the host's
// own per-SNI tls.Config (the one carrying its ClientCAs and any min-version pin);
// a request is only allowed to reach this host if the handshake actually used that
// config, i.e. the negotiated SNI resolves to this same config pointer - proving
// SNI==Host at the routing layer. requireChain additionally demands a verified
// client-certificate chain (require mode); optional mode leaves it false.
type clientAuthReq struct {
	cfg          *tls.Config
	requireChain bool
}

// buildRouter compiles the config into a router. certDir resolves relative
// custom-certificate paths; health supplies the live upstream-group state that
// group-backed hosts bind to (reconciled by the caller before the build).
func buildRouter(cfg model.Config, certDir string, health groupResolver) (*router, error) {
	certs, err := buildCertResolver(cfg.Certificates, certDir)
	if err != nil {
		return nil, err
	}
	clientCAs, err := buildClientCAAnchors(cfg.ClientCAs, certDir)
	if err != nil {
		return nil, err
	}
	// Publish the file-backed CRLs of THIS build to the mtime watch, so the watch
	// loop started once at listener start always polls the live set.
	registerCRLAnchors(clientCAs)
	reg := buildRegistry(cfg)
	reg.health = health

	rt := &router{
		hosts:      map[string]*hostHandler{},
		redirects:  map[string]*redirectHandler{},
		parked:     map[string]*parkedHandler{},
		certs:      certs,
		tlsConfigs: map[string]*tls.Config{},
		clientAuth: map[string]*clientAuthReq{},
		clientIP:   reg.clientIP,
		// Settings-level default headers for host-less responses (404/421) and
		// redirect/parked hosts. A proxy host composes its own merge below.
		securityHeaders: partitionSecurityHeaders(currentSecurityHeaders()),
	}
	// pinTLS records the per-host TLS config (min-version pin and/or mTLS) for each
	// domain. It composes both dimensions into one config, since a host with both a
	// version pin and client-cert auth needs a single tls.Config carrying both.
	pinTLS := func(domains []string, tlsSettings model.TLSSettings) error {
		c, err := hostSNIConfig(tlsSettings, clientCAs, certs)
		if err != nil {
			return err
		}
		if c == nil {
			return nil
		}
		for _, d := range domains {
			rt.tlsConfigs[hostKey(d)] = c
		}
		if tlsSettings.ClientAuth != nil {
			// require is the fail-closed default; only an explicit "optional" mode
			// waives the verified-chain requirement (matching clientAuthType).
			req := &clientAuthReq{cfg: c, requireChain: tlsSettings.ClientAuth.Mode != "optional"}
			for _, d := range domains {
				rt.clientAuth[hostKey(d)] = req
			}
		}
		return nil
	}

	for _, h := range cfg.ProxyHosts {
		if h.Disabled {
			continue
		}
		// Host-level errorPages override, compiled once per host so every gpm-
		// generated error site below (the terminal proxy and every middleware in
		// its chain) shares the same compiled templates. nil when the host sets
		// none - the settings-level pages (SetErrorPages) still apply as a
		// fallback in that case (see serveErrorPage).
		var hostEP *compiledErrorPages
		if h.ErrorPages != nil {
			var epErr error
			hostEP, epErr = compileErrorPages(*h.ErrorPages, certDir)
			if epErr != nil {
				return nil, fmt.Errorf("proxy host %q: errorPages: %w", h.Name, epErr)
			}
		}
		proxy, upLabel, err := hostProxy(h, reg, hostEP)
		if err != nil {
			return nil, fmt.Errorf("proxy host %q: %w", h.Name, err)
		}
		handler := buildChain(proxy, h, reg, hostEP)
		hh := &hostHandler{host: h.Name, handler: handler, forceSSL: h.TLS.ForceSSL, upstream: upLabel, errorPages: hostEP, maintenance: h.Maintenance}
		if hh.locations, err = buildLocations(h, reg, hostEP); err != nil {
			return nil, fmt.Errorf("proxy host %q: %w", h.Name, err)
		}
		hh.identityHeaders, hh.trustedNets = hostIdentityTrust(h, reg)
		// A DIFFERENT trust tier from trustedNets above: this one decides whose
		// X-Forwarded-For sets the client IP, that one decides who may assert an
		// identity header. Compiled here so dispatch can derive the client IP once
		// per request, before the chain runs (see clientip.go).
		hh.trustedProxies = hostTrustedProxies(h)
		// Client-certificate passthrough headers ride the same strip model: the
		// fixed names are in the baseline denylist, and a custom subject header is
		// added to this host's strip set, so no untrusted peer can assert either -
		// only gpm sets them, after the strip, from a verified certificate.
		if hh.certID = compileCertIdentity(h.TLS); hh.certID != nil {
			hh.identityHeaders = append(hh.identityHeaders, hh.certID.subject)
		}
		hh.hsts = hstsHeader(h.TLS.HSTS)
		hh.robots = robotsHeader(h.RobotsNoIndex)
		hh.securityHeaders = partitionSecurityHeaders(mergedSecurityHeaders(h.SecurityHeaders))
		for _, d := range h.Domains {
			rt.hosts[hostKey(d)] = hh
		}
		if err := pinTLS(h.Domains, h.TLS); err != nil {
			return nil, fmt.Errorf("proxy host %q: %w", h.Name, err)
		}
	}
	for _, h := range cfg.RedirectHosts {
		if h.Disabled {
			continue
		}
		rh := newRedirectHandler(h)
		for _, d := range h.Domains {
			rt.redirects[hostKey(d)] = rh
		}
		if err := pinTLS(h.Domains, h.TLS); err != nil {
			return nil, fmt.Errorf("redirect host %q: %w", h.Name, err)
		}
	}
	for _, h := range cfg.ParkedHosts {
		if h.Disabled {
			continue
		}
		ph := newParkedHandler(h)
		for _, d := range h.Domains {
			rt.parked[hostKey(d)] = ph
		}
		if err := pinTLS(h.Domains, h.TLS); err != nil {
			return nil, fmt.Errorf("parked host %q: %w", h.Name, err)
		}
	}
	return rt, nil
}

// hostKey normalizes a domain or Host header to the router map key: port stripped,
// lower-cased, trimmed.
func hostKey(host string) string {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return strings.ToLower(strings.TrimSpace(host))
}

// upstreamLabel renders an upstream as scheme://host:port for debug/logging.
func upstreamLabel(up model.Upstream) string {
	return up.Scheme + "://" + net.JoinHostPort(up.Host, strconv.Itoa(up.Port))
}

// hstsDefaultMaxAge is used when HSTS is enabled without an explicit maxAge:
// one year, the common floor (the preload list requires >= 1 year).
const hstsDefaultMaxAge = 31536000

// hstsHeader renders the Strict-Transport-Security value for a host's HSTS
// settings, or "" when disabled. maxAge defaults to one year; includeSubDomains
// and preload are appended when set.
func hstsHeader(h model.HSTS) string {
	if !h.Enabled {
		return ""
	}
	maxAge := h.MaxAge
	if maxAge <= 0 {
		maxAge = hstsDefaultMaxAge
	}
	v := "max-age=" + strconv.Itoa(maxAge)
	if h.IncludeSubdomains {
		v += "; includeSubDomains"
	}
	if h.Preload {
		v += "; preload"
	}
	return v
}

// robotsHeader returns the X-Robots-Tag value for a host that opts into
// no-indexing, or "" otherwise. "nofollow" is included so linked pages are not
// crawled either - the common intent for an internal/admin host.
func robotsHeader(noIndex bool) string {
	if !noIndex {
		return ""
	}
	return "noindex, nofollow"
}

// normalizeLocationPrefix canonicalizes a configured location path for boundary
// matching: cleaned, and with any trailing slash trimmed (root stays "/") so the
// "prefix + \"/\"" boundary test in route() composes correctly.
func normalizeLocationPrefix(p string) string {
	c := cleanPath(p)
	if c != "/" {
		c = strings.TrimRight(c, "/")
	}
	return c
}

// hostProxy returns the terminal reverse-proxy handler and its upstream label
// for a host: a single upstream, or a health-checked failover group resolved
// from the live health state (reconciled before the router build, so a missing
// group here is a hard build error rather than a silently unreachable host). ep is the
// host's compiled errorPages override (nil if it has none), handed to the
// reverse proxy for a 502/504 page and InterceptUpstream. When the host opts
// into compression, the terminal handler is wrapped to gzip eligible responses
// - outside (closer to the client than) any InterceptUpstream body
// replacement, so a rendered error page is itself eligible for compression.
func hostProxy(h model.ProxyHost, reg *registry, ep *compiledErrorPages) (http.Handler, string, error) {
	ident := assertedIdentityHeaders(h, reg)
	// The host's effective strip list (settings default unioned with its own),
	// handed to the reverse proxy so ModifyResponse can remove those headers from
	// the upstream's own response before the stdlib copies it out. A location
	// inherits it, since buildLocations derives each location from this host.
	strip := mergedStripResponseHeaders(h.StripResponseHeaders)
	var proxy http.Handler
	var label string
	if h.UpstreamGroupRef == "" {
		proxy, label = newReverseProxy(h.Upstream, h.Name, h.Timeouts, ident, ep, strip), upstreamLabel(h.Upstream)
	} else {
		var gh *groupHealth
		if reg.health != nil {
			gh = reg.health.lookup(h.UpstreamGroupRef)
		}
		if gh == nil {
			return nil, "", fmt.Errorf("upstream group %q is not available", h.UpstreamGroupRef)
		}
		proxy, label = newGroupReverseProxy(gh, h.Name, h.Timeouts, ident, ep, strip), groupLabel(h.UpstreamGroupRef)
	}
	if h.Compression.Enabled {
		proxy = compressionHandler(h.Compression, proxy)
	}
	return proxy, label, nil
}

// assertedIdentityHeaders is the canonical set of header names gpm may set on a
// request to this host on its own authority: the baseline denylist (which is
// exactly the set of names gpm strips from untrusted clients and then re-asserts
// from a verified identity), the host's provider-configured headers, and its
// client-certificate subject header. It is handed to the reverse proxy so those
// names can be restored after the stdlib's Connection-header purge (see
// reassertIdentity); it is NOT an authorization input.
func assertedIdentityHeaders(h model.ProxyHost, reg *registry) []string {
	extra, _ := hostIdentityTrust(h, reg)
	names := make([]string, 0, len(baselineIdentityHeaders)+len(extra)+1)
	seen := make(map[string]struct{}, cap(names))
	add := func(n string) {
		k := textproto.CanonicalMIMEHeaderKey(n)
		if k == "" {
			return
		}
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		names = append(names, k)
	}
	for _, n := range baselineIdentityHeaders {
		add(n)
	}
	for _, n := range extra {
		add(n)
	}
	if ci := compileCertIdentity(h.TLS); ci != nil {
		add(ci.subject)
	}
	return names
}

// buildLocations compiles a host's path-scoped locations into routes ordered
// longest-prefix first, so the most specific path wins. Each location proxies to
// its own upstream (falling back to the host upstream or upstream group) and
// inherits the host's middleware/access-list chain with its own appended, so
// per-location auth is applied on top of the host gate rather than replacing it.
// The request path is forwarded unchanged unless the location sets
// stripPrefix, which removes the matched prefix on the way to the upstream (see
// stripPrefixHandler). ep is the host's compiled errorPages override
// (already resolved by the caller); a location has no override of its own, so
// it shares its host's.
func buildLocations(h model.ProxyHost, reg *registry, ep *compiledErrorPages) ([]locationRoute, error) {
	if len(h.Locations) == 0 {
		return nil, nil
	}
	locs := append([]model.Location{}, h.Locations...)
	sort.SliceStable(locs, func(i, j int) bool { return len(locs[i].Path) > len(locs[j].Path) })
	routes := make([]locationRoute, 0, len(locs))
	for _, loc := range locs {
		lh := h
		switch {
		case loc.Upstream != nil: // explicit single upstream overrides any group
			lh.Upstream = *loc.Upstream
			lh.UpstreamGroupRef = ""
		case loc.UpstreamGroupRef != "": // location's own group overrides the host backend
			lh.UpstreamGroupRef = loc.UpstreamGroupRef
			lh.Upstream = model.Upstream{}
		}
		lh.Middlewares = append(append([]string{}, h.Middlewares...), loc.Middlewares...)
		lh.AccessLists = append(append([]string{}, h.AccessLists...), loc.AccessLists...)
		lh.Locations = nil
		proxy, upLabel, err := hostProxy(lh, reg, ep)
		if err != nil {
			return nil, fmt.Errorf("location %q: %w", loc.Path, err)
		}
		routes = append(routes, locationRoute{
			prefix:   normalizeLocationPrefix(loc.Path),
			handler:  buildChainInline(proxy, lh, locationInline(h, loc), reg, ep),
			upstream: upLabel,
		})
	}
	return routes, nil
}

// baseHostTLSConfig returns a per-host TLS config with the shared cert resolver,
// AEAD cipher suites, h2 ALPN, and the default 1.2 floor - a complete drop-in for
// the handshake via GetConfigForClient that callers then specialise.
func baseHostTLSConfig(certs *certResolver) *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		CipherSuites:   secureCipherSuites, // ignored for 1.3 (suites are fixed), kept for consistency
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: certs.GetCertificate,
	}
}

// hostTLSConfig returns a per-host TLS config pinning a non-default minimum
// version, or nil when the host uses the listener default (TLS 1.2 floor).
func hostTLSConfig(minTLSVersion string, certs *certResolver) *tls.Config {
	if minTLSVersion != "1.3" {
		return nil // "", "1.2", or unknown -> default listener config (1.2 floor)
	}
	c := baseHostTLSConfig(certs)
	c.MinVersion = tls.VersionTLS13
	return c
}

// clientAuthType maps a ClientAuth mode to the tls.ClientAuthType. An empty mode
// defaults to require, the fail-closed choice for a host that opted into mTLS.
func clientAuthType(mode string) tls.ClientAuthType {
	if mode == "optional" {
		return tls.VerifyClientCertIfGiven
	}
	return tls.RequireAndVerifyClientCert
}

// hostSNIConfig composes a host's per-SNI TLS config from its version pin and
// optional mTLS client-auth. It returns nil when the host needs neither (the
// listener default applies). A clientAuth referencing a CA that did not compile
// into a pool is a hard error, so the host fails closed rather than serving
// without the client-certificate check.
func hostSNIConfig(t model.TLSSettings, clientCAs map[string]*clientCAAnchor, certs *certResolver) (*tls.Config, error) {
	c := hostTLSConfig(t.MinTLSVersion, certs)
	if t.ClientAuth == nil {
		return c, nil // version pin only (may be nil)
	}
	if c == nil {
		c = baseHostTLSConfig(certs)
	}
	anchor := clientCAs[t.ClientAuth.CARef]
	if anchor == nil || anchor.pool == nil {
		return nil, fmt.Errorf("clientAuth references client CA %q with no usable certificates", t.ClientAuth.CARef)
	}
	c.ClientCAs = anchor.pool
	c.ClientAuth = clientAuthType(t.ClientAuth.Mode)
	if anchor.crlConfigured {
		// Revocation is the one check stdlib chain verification does not make, so
		// it hangs off VerifyPeerCertificate - which runs after the chain builds,
		// on the handshake, for every host referencing this CA.
		c.VerifyPeerCertificate = anchor.verifyPeer
	}
	return c, nil
}

// tlsConfigForSNI returns the per-host TLS config for the SNI server name, or nil
// to use the listener's default. Wired to tls.Config.GetConfigForClient so a
// host can pin a higher minimum TLS version than the global floor.
func (rt *router) tlsConfigForSNI(serverName string) *tls.Config {
	return rt.tlsConfigs[hostKey(serverName)] // nil if not pinned
}

// securityHeadersFor returns the response-header set to inject for a request
// keyed by host: a proxy host's own merged set, else the settings-level default
// (used for redirect/parked hosts and the no-such-host / misdirected responses).
func (rt *router) securityHeadersFor(name string) *scopedHeaders {
	if hh, ok := rt.hosts[name]; ok {
		return hh.securityHeaders
	}
	return rt.securityHeaders
}

// lookup returns the proxy-host handler for the request's Host (port stripped),
// if any. Redirect and parked hosts are dispatched separately in serveHTTP(S).
func (rt *router) lookup(hostHeader string) (*hostHandler, bool) {
	hh, ok := rt.hosts[hostKey(hostHeader)]
	return hh, ok
}

// cleanPath returns the canonical, dot-segment-free form of an HTTP request
// path, mirroring net/http's own cleanPath: a leading slash is ensured and a
// trailing slash is preserved (it is semantically distinct for prefix matching
// and some upstreams). path.Clean cannot escape the root, so "/x/../../etc"
// collapses to "/etc", never above "/".
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}

// normalizeRequestPath rewrites r.URL to the cleaned path so location/guard
// matching, access control, and the upstream all agree on one canonical path -
// a request cannot present "/x/../admin" (or an encoded "%2e%2e") to dodge a
// location's auth and then reach the upstream's "/admin". RawPath is cleared so
// the cleaned, decoded Path is what gets forwarded.
//
// It returns false (caller replies 400) when the path contains a backslash or a
// ';' matrix parameter. path.Clean treats neither as path structure, so a
// request like "/admin;x" or "/admin\..\x" survives normalization unchanged and
// fails to match a path-scoped location's auth or a guard trigger keyed on
// "/admin" - yet some upstreams re-collapse it onto the protected path (IIS
// treats '\' as a separator; Servlet containers strip ';jsessionid'). Rejecting
// these keeps gpm's matched path and the forwarded path byte-identical, so no
// such matcher/backend divergence exists.
func normalizeRequestPath(r *http.Request) bool {
	p := cleanPath(r.URL.Path)
	if strings.ContainsAny(p, `\;`) {
		return false
	}
	r.URL.Path = p
	r.URL.RawPath = ""
	return true
}

// stripUntrustedIdentity removes identity headers from a request unless its peer
// is a proxy this host trusts, so a direct client can never forge an identity to
// a backend. The baseline denylist is stripped for every host (even one with no
// IdP); a host's own provider headers are stripped on top.
func (hh *hostHandler) stripUntrustedIdentity(r *http.Request) {
	if ip := peerIP(r); ip != nil && ipInNets(ip, hh.trustedNets) {
		return // a proxy this host trusts may assert identity headers
	}
	stripIdentityHeaders(r.Header, hh.identityHeaders)
}

// clientAuthSatisfied reports whether a request may reach an mTLS-protected host.
// It requires (a) the handshake used this host's own per-SNI tls.Config - i.e. the
// negotiated SNI resolves to the same config pointer, so SNI==Host and the host's
// client-cert requirement (and any pinned min-TLS version) actually applied - and
// (b) for require mode, a client certificate verified during the handshake. An
// empty or foreign SNI resolves to a different (or nil) config and fails (a), which
// is also what closes the min-TLS-version-by-SNI dodge for any mTLS host.
func (rt *router) clientAuthSatisfied(req *clientAuthReq, r *http.Request) bool {
	if r.TLS == nil {
		return false
	}
	if rt.tlsConfigForSNI(r.TLS.ServerName) != req.cfg {
		return false
	}
	if req.requireChain && len(r.TLS.VerifiedChains) == 0 {
		return false
	}
	return true
}

// serveHTTPS dispatches a TLS-terminated request to its host by Host header:
// proxy hosts run the middleware chain; redirect and parked hosts serve directly.
func (rt *router) serveHTTPS(w http.ResponseWriter, r *http.Request) {
	name := hostKey(r.Host)
	// Configured security headers are injected set-if-absent at this dispatch
	// layer - the same response layer HSTS rides - so they reach every response
	// gpm generates below (the mTLS 421, the 400 bad path, the 404 no-such-host,
	// every auth-gate denial and sign-in redirect in the chain, redirect/parked
	// hosts) regardless of the auth outcome, while a proxied upstream response
	// keeps its own headers untouched.
	w = withSecurityHeaders(w, rt.securityHeadersFor(name))
	// mTLS is enforced per REQUEST here, not only by the SNI-selected tls.Config:
	// reject any request whose Host targets an mTLS host unless the handshake used
	// that host's own config (SNI==Host) and, for require mode, actually verified a
	// client certificate. Hosts without mTLS take no new rejection path.
	if req := rt.clientAuth[name]; req != nil && !rt.clientAuthSatisfied(req, r) {
		http.Error(w, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
		return
	}
	if hh, ok := rt.hosts[name]; ok {
		// Maintenance short-circuits the whole chain: no path normalization, no
		// auth work, no upstream dial - the host is deliberately out of service,
		// so gpm answers for it. It sits INSIDE the mTLS gate above, so a host
		// that requires a client certificate still refuses an uncertified request
		// with a 421 rather than disclosing a maintenance window to it.
		if maintenanceActive(hh.maintenance) {
			serveMaintenance(w, r, hh.errorPages, hh.host)
			return
		}
		if !normalizeRequestPath(r) {
			http.Error(w, "bad request path", http.StatusBadRequest)
			return
		}
		// Derive the client IP ONCE, from this host's trusted-proxy set, and stash
		// it for the whole request: the chain's access list, geo, rate limit,
		// guards, allowFrom exemptions, the upstream X-Forwarded-For/X-Real-IP
		// rewrite and the access log all read this one value rather than each
		// re-interpreting the header (see clientip.go).
		r = withClientIP(r, deriveClientIP(r, hh.trustedProxies))
		hh.stripUntrustedIdentity(r)
		if hh.certID != nil {
			hh.certID.apply(r)
		}
		// HSTS (HTTPS-only) and X-Robots-Tag ride the dispatch writer, emitted at
		// the FINAL WriteHeader, so an upstream 1xx interim response's header-map
		// clear (httputil.ReverseProxy's Got1xxResponse) cannot drop them.
		w = withHostResponseHeaders(w, hh.hsts, hh.robots)
		// Stash the dispatch writer so a proxied upstream response (detected in the
		// reverse proxy's ModifyResponse) can select the proxied header subset; a
		// gpm-generated response below never marks it and keeps the generated subset.
		r = requestWithSecurityWriter(r, w)
		handler, _ := hh.route(r.URL.Path)
		handler.ServeHTTP(w, r)
		return
	}
	if rh, ok := rt.redirects[name]; ok {
		rh.serve(w, r)
		return
	}
	if ph, ok := rt.parked[name]; ok {
		ph.serve(w, r)
		return
	}
	http.Error(w, "no such host", http.StatusNotFound)
}

// serveHTTP handles plaintext requests: force-SSL hosts (of any type) are
// redirected to https, others are served in the clear.
func (rt *router) serveHTTP(w http.ResponseWriter, r *http.Request) {
	name := hostKey(r.Host)
	// Same set-if-absent injection as serveHTTPS (HSTS aside, which is HTTPS-only):
	// gpm-generated plaintext responses (a non-forceSSL host's denial, a 400/404)
	// carry the configured headers too.
	w = withSecurityHeaders(w, rt.securityHeadersFor(name))
	// An mTLS-required host must never be served in the clear: redirect to HTTPS so
	// the client re-arrives on the TLS listener, where serveHTTPS enforces the
	// per-request client-cert gate. Config validation forces forceSSL:true for any
	// mTLS host, so this only fires for a config that bypassed validation (e.g.
	// hand-edited or imported) - defense in depth that fails closed regardless.
	if rt.clientAuth[name] != nil {
		redirectToHTTPS(w, r)
		return
	}
	if hh, ok := rt.hosts[name]; ok {
		if !normalizeRequestPath(r) {
			http.Error(w, "bad request path", http.StatusBadRequest)
			return
		}
		if hh.forceSSL {
			redirectToHTTPS(w, r)
			return
		}
		// Same short-circuit as serveHTTPS, placed after the force-SSL redirect so
		// an HTTPS-only host still bounces the client to TLS and serves the
		// maintenance page there. ACME HTTP-01 is answered before host routing
		// (see dispatchHTTP), so certificates still renew during a window.
		if maintenanceActive(hh.maintenance) {
			serveMaintenance(w, r, hh.errorPages, hh.host)
			return
		}
		r = withClientIP(r, deriveClientIP(r, hh.trustedProxies)) // see serveHTTPS
		hh.stripUntrustedIdentity(r)
		// X-Robots-Tag rides the dispatch writer for the same 1xx-survival reason
		// as HSTS on the HTTPS path; HSTS itself is never emitted over plain HTTP.
		w = withHostResponseHeaders(w, "", hh.robots)
		r = requestWithSecurityWriter(r, w) // see serveHTTPS
		handler, _ := hh.route(r.URL.Path)
		handler.ServeHTTP(w, r)
		return
	}
	if rh, ok := rt.redirects[name]; ok {
		if rh.forceSSL {
			redirectToHTTPS(w, r)
			return
		}
		rh.serve(w, r)
		return
	}
	if ph, ok := rt.parked[name]; ok {
		if ph.forceSSL {
			redirectToHTTPS(w, r)
			return
		}
		ph.serve(w, r)
		return
	}
	http.Error(w, "no such host", http.StatusNotFound)
}

// redirectToHTTPS sends a plaintext request to the canonical https URL on the
// same host (port stripped), preserving path and query.
func redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	u := *r.URL
	u.Scheme = "https"
	u.Host = hostKey(r.Host)
	http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
}

// certIdentity is a host's compiled client-certificate identity passthrough:
// which attributes of the VERIFIED peer certificate are forwarded upstream and
// under which header names.
type certIdentity struct {
	subject     string // canonicalized header name for the certificate subject
	san         bool
	serial      bool
	fingerprint bool
}

// compileCertIdentity returns the passthrough for a host's TLS settings, or nil
// when the host did not opt in (no clientAuth, or no identityHeaders block).
func compileCertIdentity(t model.TLSSettings) *certIdentity {
	if t.ClientAuth == nil || t.ClientAuth.IdentityHeaders == nil {
		return nil
	}
	ih := t.ClientAuth.IdentityHeaders
	name := ih.SubjectHeader
	if name == "" {
		name = model.DefaultClientCertSubjectHeader
	}
	return &certIdentity{
		subject:     textproto.CanonicalMIMEHeaderKey(name),
		san:         ih.SAN,
		serial:      ih.Serial,
		fingerprint: ih.Fingerprint,
	}
}

// apply sets the passthrough headers from the client certificate the handshake
// VERIFIED. It is a no-op without a verified chain - in optional mode a certless
// (or unverified) request must not reach the upstream carrying an identity - and
// it always runs AFTER stripUntrustedIdentity, so gpm is the only asserter.
func (ci *certIdentity) apply(r *http.Request) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		return
	}
	c := r.TLS.PeerCertificates[0]
	r.Header.Set(ci.subject, headerSafe(c.Subject.String()))
	if ci.san {
		r.Header.Set(model.ClientCertSANHeader, headerSafe(strings.Join(certSANs(c), ",")))
	}
	if ci.serial {
		r.Header.Set(model.ClientCertSerialHeader, serialKey(c.SerialNumber))
	}
	if ci.fingerprint {
		sum := sha256.Sum256(c.Raw)
		r.Header.Set(model.ClientCertFingerprintHeader, hex.EncodeToString(sum[:]))
	}
}

// certSANs flattens a certificate's subject alternative names into one ordered
// list (DNS, email, IP, URI) for the X-Client-Cert-SAN header.
func certSANs(c *x509.Certificate) []string {
	var out []string
	out = append(out, c.DNSNames...)
	out = append(out, c.EmailAddresses...)
	for _, ip := range c.IPAddresses {
		out = append(out, ip.String())
	}
	for _, u := range c.URIs {
		out = append(out, u.String())
	}
	return out
}

// headerSafe strips anything outside printable US-ASCII from a value taken from
// a certificate, so an attacker-chosen subject (which a CA operator may allow to
// contain arbitrary UTF-8) cannot inject CR/LF or control bytes into the
// upstream request.
func headerSafe(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if c := v[i]; c >= 0x20 && c < 0x7f {
			b.WriteByte(c)
		}
	}
	return b.String()
}
