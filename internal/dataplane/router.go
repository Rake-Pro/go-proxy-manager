package dataplane

import (
	"crypto/tls"
	"net"
	"net/http"
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
		upstream := h.Upstream.Scheme + "://" + net.JoinHostPort(h.Upstream.Host, strconv.Itoa(h.Upstream.Port))
		hh := &hostHandler{host: h.Name, handler: handler, forceSSL: h.TLS.ForceSSL, upstream: upstream}
		for _, d := range h.Domains {
			rt.hosts[strings.ToLower(strings.TrimSpace(d))] = hh
		}
	}
	return rt, nil
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
	hh.handler.ServeHTTP(w, r)
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
	hh.handler.ServeHTTP(w, r)
}
