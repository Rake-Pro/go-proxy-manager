package dataplane

import (
	"crypto/tls"
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
	hosts map[string]*hostHandler
	certs *certResolver

	// clientIP resolves the real client IP (XFF-aware via the access-list trusted
	// nets) for logging. Identity-header stripping is host-scoped on each
	// hostHandler (see hostHandler.stripUntrustedIdentity).
	clientIP func(*http.Request) net.IP
}

// buildRouter compiles the config into a router. certDir resolves relative
// custom-certificate paths.
func buildRouter(cfg model.Config, certDir string) (*router, error) {
	certs, err := buildCertResolver(cfg.Certificates, certDir)
	if err != nil {
		return nil, err
	}
	reg := buildRegistry(cfg)

	rt := &router{
		hosts:    map[string]*hostHandler{},
		certs:    certs,
		clientIP: reg.clientIP,
	}
	for _, h := range cfg.ProxyHosts {
		if h.Disabled {
			continue
		}
		proxy := newReverseProxy(h.Upstream, h.Name)
		handler := buildChain(proxy, h, reg)
		hh := &hostHandler{host: h.Name, handler: handler, forceSSL: h.TLS.ForceSSL, upstream: upstreamLabel(h.Upstream)}
		hh.locations = buildLocations(h, reg)
		hh.identityHeaders, hh.trustedNets = hostIdentityTrust(h, reg)
		hh.hsts = hstsHeader(h.TLS.HSTS)
		hh.tlsConfig = hostTLSConfig(h.TLS.MinTLSVersion, certs)
		for _, d := range h.Domains {
			rt.hosts[strings.ToLower(strings.TrimSpace(d))] = hh
		}
	}
	return rt, nil
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
// forwarded unchanged - the upstream sees the full prefix, matching NPM's
// proxy_pass-without-URI behaviour.
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
		proxy := newReverseProxy(lh.Upstream, h.Name)
		routes = append(routes, locationRoute{
			prefix:   normalizeLocationPrefix(loc.Path),
			handler:  buildChain(proxy, lh, reg),
			upstream: upstreamLabel(lh.Upstream),
		})
	}
	return routes
}

// hostTLSConfig returns a per-host TLS config pinning a non-default minimum
// version, or nil when the host uses the listener default (TLS 1.2 floor). The
// returned config carries the shared cert resolver, AEAD cipher suites, and h2
// ALPN, so it is a complete drop-in for the handshake via GetConfigForClient.
func hostTLSConfig(minTLSVersion string, certs *certResolver) *tls.Config {
	if minTLSVersion != "1.3" {
		return nil // "", "1.2", or unknown -> default listener config (1.2 floor)
	}
	return &tls.Config{
		MinVersion:     tls.VersionTLS13,
		CipherSuites:   secureCipherSuites, // ignored for 1.3 (suites are fixed), kept for consistency
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: certs.GetCertificate,
	}
}

// tlsConfigForSNI returns the per-host TLS config for the SNI server name, or nil
// to use the listener's default. Wired to tls.Config.GetConfigForClient so a
// host can pin a higher minimum TLS version than the global floor.
func (rt *router) tlsConfigForSNI(serverName string) *tls.Config {
	if hh, ok := rt.lookup(serverName); ok {
		return hh.tlsConfig
	}
	return nil
}

// lookup returns the handler for the request's Host (port stripped), if any.
func (rt *router) lookup(hostHeader string) (*hostHandler, bool) {
	name := hostHeader
	if i := strings.IndexByte(name, ':'); i >= 0 {
		name = name[:i]
	}
	hh, ok := rt.hosts[strings.ToLower(strings.TrimSpace(name))]
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
func normalizeRequestPath(r *http.Request) {
	r.URL.Path = cleanPath(r.URL.Path)
	r.URL.RawPath = ""
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

// serveHTTPS dispatches a TLS-terminated request to its host handler.
func (rt *router) serveHTTPS(w http.ResponseWriter, r *http.Request) {
	hh, ok := rt.lookup(r.Host)
	if !ok {
		http.Error(w, "no such host", http.StatusNotFound)
		return
	}
	normalizeRequestPath(r)
	hh.stripUntrustedIdentity(r)
	if hh.hsts != "" {
		// Set before serving so it rides the upstream's response; only on HTTPS
		// (browsers ignore HSTS received over plain HTTP anyway).
		w.Header().Set("Strict-Transport-Security", hh.hsts)
	}
	handler, _ := hh.route(r.URL.Path)
	handler.ServeHTTP(w, r)
}

// serveHTTP handles plaintext requests: force-SSL hosts are redirected to https,
// others are proxied in the clear.
func (rt *router) serveHTTP(w http.ResponseWriter, r *http.Request) {
	hh, ok := rt.lookup(r.Host)
	if !ok {
		http.Error(w, "no such host", http.StatusNotFound)
		return
	}
	normalizeRequestPath(r)
	if hh.forceSSL {
		host := r.Host
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i] // redirect to the canonical https host, not the inbound port
		}
		u := *r.URL
		u.Scheme = "https"
		u.Host = host
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
		return
	}
	hh.stripUntrustedIdentity(r)
	handler, _ := hh.route(r.URL.Path)
	handler.ServeHTTP(w, r)
}
