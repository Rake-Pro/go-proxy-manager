package dataplane

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
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
	dead      map[string]*deadHandler
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
// custom-certificate paths.
func buildRouter(cfg model.Config, certDir string) (*router, error) {
	certs, err := buildCertResolver(cfg.Certificates, certDir)
	if err != nil {
		return nil, err
	}
	clientCAs, err := buildClientCAPools(cfg.ClientCAs)
	if err != nil {
		return nil, err
	}
	reg := buildRegistry(cfg)

	rt := &router{
		hosts:      map[string]*hostHandler{},
		redirects:  map[string]*redirectHandler{},
		dead:       map[string]*deadHandler{},
		certs:      certs,
		tlsConfigs: map[string]*tls.Config{},
		clientAuth: map[string]*clientAuthReq{},
		clientIP:   reg.clientIP,
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
		proxy := newReverseProxy(h.Upstream, h.Name, h.Timeouts)
		handler := buildChain(proxy, h, reg)
		hh := &hostHandler{host: h.Name, handler: handler, forceSSL: h.TLS.ForceSSL, upstream: upstreamLabel(h.Upstream)}
		hh.locations = buildLocations(h, reg)
		hh.identityHeaders, hh.trustedNets = hostIdentityTrust(h, reg)
		hh.hsts = hstsHeader(h.TLS.HSTS)
		hh.robots = robotsHeader(h.RobotsNoIndex)
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
	for _, h := range cfg.DeadHosts {
		if h.Disabled {
			continue
		}
		dh := newDeadHandler(h)
		for _, d := range h.Domains {
			rt.dead[hostKey(d)] = dh
		}
		if err := pinTLS(h.Domains, h.TLS); err != nil {
			return nil, fmt.Errorf("dead host %q: %w", h.Name, err)
		}
	}
	return rt, nil
}

// buildClientCAPools resolves each ClientCA object's PEM (inline or via a
// ${FILE:...}/${ENV:...} placeholder) into an x509 pool keyed by name. A CA whose
// PEM resolves but parses to zero certificates is a hard error so a host requiring
// mTLS never compiles against an empty trust anchor (which would reject everyone).
func buildClientCAPools(cas []model.ClientCA) (map[string]*x509.CertPool, error) {
	pools := map[string]*x509.CertPool{}
	for _, ca := range cas {
		if ca.Disabled {
			continue
		}
		pem, err := model.Secret(ca.CAPEM).Resolve()
		if err != nil {
			return nil, fmt.Errorf("client CA %q: %w", ca.Name, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(pem)) {
			return nil, fmt.Errorf("client CA %q: caPEM parsed to no certificates", ca.Name)
		}
		pools[ca.Name] = pool
	}
	return pools, nil
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

// buildLocations compiles a host's path-scoped locations into routes ordered
// longest-prefix first, so the most specific path wins. Each location proxies to
// its own upstream (falling back to the host upstream) and inherits the host's
// middleware/access-list chain with its own appended, so per-location auth is
// applied on top of the host gate rather than replacing it. The request path is
// forwarded unchanged - the upstream receives the full request path unmodified.
func buildLocations(h model.ProxyHost, reg *registry) []locationRoute {
	if len(h.Locations) == 0 {
		return nil
	}
	locs := append([]model.Location{}, h.Locations...)
	sort.SliceStable(locs, func(i, j int) bool { return len(locs[i].Path) > len(locs[j].Path) })
	routes := make([]locationRoute, 0, len(locs))
	for _, loc := range locs {
		lh := h
		if loc.Upstream != nil {
			lh.Upstream = *loc.Upstream
		}
		lh.Middlewares = append(append([]string{}, h.Middlewares...), loc.Middlewares...)
		lh.AccessLists = append(append([]string{}, h.AccessLists...), loc.AccessLists...)
		lh.Locations = nil
		proxy := newReverseProxy(lh.Upstream, h.Name, h.Timeouts)
		routes = append(routes, locationRoute{
			prefix:   normalizeLocationPrefix(loc.Path),
			handler:  buildChain(proxy, lh, reg),
			upstream: upstreamLabel(lh.Upstream),
		})
	}
	return routes
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
func hostSNIConfig(t model.TLSSettings, clientCAs map[string]*x509.CertPool, certs *certResolver) (*tls.Config, error) {
	c := hostTLSConfig(t.MinTLSVersion, certs)
	if t.ClientAuth == nil {
		return c, nil // version pin only (may be nil)
	}
	if c == nil {
		c = baseHostTLSConfig(certs)
	}
	pool := clientCAs[t.ClientAuth.CARef]
	if pool == nil {
		return nil, fmt.Errorf("clientAuth references client CA %q with no usable certificates", t.ClientAuth.CARef)
	}
	c.ClientCAs = pool
	c.ClientAuth = clientAuthType(t.ClientAuth.Mode)
	return c, nil
}

// tlsConfigForSNI returns the per-host TLS config for the SNI server name, or nil
// to use the listener's default. Wired to tls.Config.GetConfigForClient so a
// host can pin a higher minimum TLS version than the global floor.
func (rt *router) tlsConfigForSNI(serverName string) *tls.Config {
	return rt.tlsConfigs[hostKey(serverName)] // nil if not pinned
}

// lookup returns the proxy-host handler for the request's Host (port stripped),
// if any. Redirect and dead hosts are dispatched separately in serveHTTP(S).
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
// proxy hosts run the middleware chain; redirect and dead hosts serve directly.
func (rt *router) serveHTTPS(w http.ResponseWriter, r *http.Request) {
	name := hostKey(r.Host)
	// mTLS is enforced per REQUEST here, not only by the SNI-selected tls.Config:
	// reject any request whose Host targets an mTLS host unless the handshake used
	// that host's own config (SNI==Host) and, for require mode, actually verified a
	// client certificate. Hosts without mTLS take no new rejection path.
	if req := rt.clientAuth[name]; req != nil && !rt.clientAuthSatisfied(req, r) {
		http.Error(w, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
		return
	}
	if hh, ok := rt.hosts[name]; ok {
		if !normalizeRequestPath(r) {
			http.Error(w, "bad request path", http.StatusBadRequest)
			return
		}
		hh.stripUntrustedIdentity(r)
		if hh.hsts != "" {
			// Set before serving so it rides the upstream's response; only on HTTPS
			// (browsers ignore HSTS received over plain HTTP anyway).
			w.Header().Set("Strict-Transport-Security", hh.hsts)
		}
		if hh.robots != "" {
			w.Header().Set("X-Robots-Tag", hh.robots)
		}
		handler, _ := hh.route(r.URL.Path)
		handler.ServeHTTP(w, r)
		return
	}
	if rh, ok := rt.redirects[name]; ok {
		rh.serve(w, r)
		return
	}
	if dh, ok := rt.dead[name]; ok {
		dh.serve(w, r)
		return
	}
	http.Error(w, "no such host", http.StatusNotFound)
}

// serveHTTP handles plaintext requests: force-SSL hosts (of any type) are
// redirected to https, others are served in the clear.
func (rt *router) serveHTTP(w http.ResponseWriter, r *http.Request) {
	name := hostKey(r.Host)
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
		hh.stripUntrustedIdentity(r)
		if hh.robots != "" {
			w.Header().Set("X-Robots-Tag", hh.robots)
		}
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
	if dh, ok := rt.dead[name]; ok {
		if dh.forceSSL {
			redirectToHTTPS(w, r)
			return
		}
		dh.serve(w, r)
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
