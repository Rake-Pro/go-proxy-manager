package dataplane

import (
	"crypto/tls"
	"net"
	"net/http"
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

	// identityHeaders are stripped from every inbound request whose peer is not a
	// trusted proxy, closing the gap where a direct client forges X-* identity
	// headers to an unprotected backend. trustedNets are the peers exempt from
	// stripping (they legitimately assert forward-auth identities).
	identityHeaders []string
	trustedNets     []*net.IPNet
	// clientIP resolves the real client IP (XFF-aware via trustedNets) for logging.
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
		hosts:           map[string]*hostHandler{},
		certs:           certs,
		identityHeaders: reg.identityHeaders,
		trustedNets:     reg.trustedNets,
		clientIP:        reg.clientIP,
	}
	for _, h := range cfg.ProxyHosts {
		if h.Disabled {
			continue
		}
		proxy := newReverseProxy(h.Upstream, h.Name)
		handler := buildChain(proxy, h, reg)
		hh := &hostHandler{host: h.Name, handler: handler, forceSSL: h.TLS.ForceSSL, upstream: upstreamLabel(h.Upstream)}
		hh.locations = buildLocations(h, reg)
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
			prefix:   loc.Path,
			handler:  buildChain(proxy, lh, reg),
			upstream: upstreamLabel(lh.Upstream),
		})
	}
	return routes
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

func (rt *router) tlsConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: rt.certs.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		CipherSuites:   secureCipherSuites,
		NextProtos:     []string{"h2", "http/1.1"},
	}
}

// stripUntrustedIdentity removes identity headers from a request unless its peer
// is a trusted proxy, so a direct client can never forge an identity to a backend.
func (rt *router) stripUntrustedIdentity(r *http.Request) {
	if len(rt.identityHeaders) == 0 {
		return
	}
	if ip := peerIP(r); ip != nil && ipInNets(ip, rt.trustedNets) {
		return // a trusted proxy may assert identity headers
	}
	for _, h := range rt.identityHeaders {
		r.Header.Del(h)
	}
}

// serveHTTPS dispatches a TLS-terminated request to its host handler.
func (rt *router) serveHTTPS(w http.ResponseWriter, r *http.Request) {
	hh, ok := rt.lookup(r.Host)
	if !ok {
		http.Error(w, "no such host", http.StatusNotFound)
		return
	}
	rt.stripUntrustedIdentity(r)
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
	rt.stripUntrustedIdentity(r)
	handler, _ := hh.route(r.URL.Path)
	handler.ServeHTTP(w, r)
}
