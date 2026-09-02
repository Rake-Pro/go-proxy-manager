package dataplane

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// defaultOutpostPrefix is Authentik's canonical proxy-outpost path namespace.
const defaultOutpostPrefix = "/outpost.goauthentik.io"

// defaultAuthRequestHeaders mirrors the X-authentik-* headers an Authentik
// forward-auth outpost sends.
var defaultAuthRequestHeaders = []string{
	"X-authentik-username",
	"X-authentik-groups",
	"X-authentik-entitlements",
	"X-authentik-email",
	"X-authentik-name",
	"X-authentik-uid",
}

// authRequestProxy implements the nginx auth_request pattern: for each request
// it asks an external auth server (an Authentik proxy outpost) whether the
// caller is authenticated, copies the returned identity headers onto the
// upstream, and bounces unauthenticated browsers into the server's sign-in flow.
// The outpost's own endpoints (sign-in, callback, sign-out) are proxied straight
// through under prefix.
type authRequestProxy struct {
	authURL     string   // absolute URL of the per-request auth subrequest endpoint
	prefix      string   // path namespace owned by the auth server
	copyHeaders []string // auth-response headers copied onto the upstream on success
	outpost     *httputil.ReverseProxy
	client      *http.Client
}

func compileAuthRequest(spec model.AuthRequestSpec) (*authRequestProxy, error) {
	base, err := url.Parse(spec.OutpostURL)
	if err != nil {
		return nil, fmt.Errorf("parse outpostURL: %w", err)
	}
	prefix := spec.PathPrefix
	if prefix == "" {
		prefix = defaultOutpostPrefix
	}
	authPath := spec.AuthPath
	if authPath == "" {
		authPath = prefix + "/auth/nginx"
	}
	headers := spec.CopyHeaders
	if len(headers) == 0 {
		headers = defaultAuthRequestHeaders
	}

	outpost := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(base)          // dial the auth server, keep the inbound path
			pr.Out.Host = pr.In.Host // preserve the browser-facing host
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Original-URL", absoluteURL(pr.In))
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Warn().Str("path", r.URL.Path).Err(err).Msg("auth outpost proxy error")
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	return &authRequestProxy{
		authURL:     strings.TrimRight(spec.OutpostURL, "/") + authPath,
		prefix:      prefix,
		copyHeaders: headers,
		outpost:     outpost,
		client: &http.Client{
			Timeout: 10 * time.Second,
			// Never follow the auth server's redirects: we want its 200/401/403
			// verdict, not the HTML of a login page.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			// Dedicated transport, tuned like dataplaneTransport: the default
			// transport caps idle connections per host at 2, which starves the
			// per-request auth subrequest under load. The outpost URL is internal,
			// so proxy env vars are deliberately not honoured.
			Transport: &http.Transport{
				Proxy:               nil,
				DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 64,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 5 * time.Second,
			},
		},
	}, nil
}

// handler gates next behind the external auth server. clientIP resolves the real
// client address; allowNets (per-host) bypass auth entirely.
//
// ep resolves the host's custom error pages for the TERMINAL refusals below (403
// and the 502s). The two non-terminal branches are deliberately excluded: the
// sign-in redirect is a flow, not an error, and the outpost passthrough proxies
// the identity provider's OWN response verbatim - the IdP owns its sign-in,
// callback and sign-out pages, and gpm must not overwrite them with an error
// page. IdP response wins; error pages apply only where gpm generates the body.
func (p *authRequestProxy) handler(clientIP func(*http.Request) net.IP, allowNets []*net.IPNet, hostName string, ep *compiledErrorPages, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip any client-presented identity headers up front so a forged
		// X-authentik-* can never reach the upstream, regardless of branch.
		for _, h := range p.copyHeaders {
			r.Header.Del(h)
		}

		// The auth server owns its sign-in/callback/sign-out endpoints; pass them
		// through untouched (and ungated - the sign-in flow must be reachable).
		if r.URL.Path == p.prefix || strings.HasPrefix(r.URL.Path, p.prefix+"/") {
			p.outpost.ServeHTTP(w, r)
			return
		}

		// AllowFrom networks (e.g. the LAN) bypass SSO entirely: proxied straight
		// through with no auth subrequest and no identity headers (they were just
		// stripped above). An any-of, network-exempt bypass so trusted networks
		// can skip SSO.
		if len(allowNets) > 0 {
			if ip := clientIP(r); ip != nil && ipInNets(ip, allowNets) {
				next.ServeHTTP(w, r)
				return
			}
		}

		resp, err := p.authenticate(r)
		if err != nil {
			log.Warn().Str("host", hostName).Err(err).Msg("auth subrequest failed")
			refuse(w, http.StatusBadGateway, ep, hostName, "authentication backend unavailable")
			return
		}
		defer drainClose(resp.Body)

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			// Copy the authenticated identity onto the upstream request and pass
			// the auth server's (refreshed) session cookie back to the browser.
			for _, h := range p.copyHeaders {
				for _, v := range resp.Header.Values(h) {
					r.Header.Add(h, v)
				}
			}
			copySetCookie(w, resp)
			next.ServeHTTP(w, r)
		case resp.StatusCode == http.StatusUnauthorized:
			// Unauthenticated: send the browser into the sign-in flow.
			copySetCookie(w, resp)
			dest := p.prefix + "/start?rd=" + url.QueryEscape(absoluteURL(r))
			http.Redirect(w, r, dest, http.StatusFound)
		case resp.StatusCode == http.StatusForbidden:
			// Authenticated but not authorized for this application.
			refuse(w, http.StatusForbidden, ep, hostName, "forbidden")
		default:
			log.Warn().Str("host", hostName).Int("status", resp.StatusCode).Msg("unexpected auth subrequest status")
			refuse(w, http.StatusBadGateway, ep, hostName, "authentication backend error")
		}
	})
}

// authenticate performs the per-request GET subrequest to the auth server,
// forwarding the caller's cookies and the original request's coordinates so the
// server can locate the session and the target application.
func (p *authRequestProxy) authenticate(r *http.Request) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, p.authURL, nil)
	if err != nil {
		return nil, err
	}
	req.Host = r.Host // identify the application by its browser-facing host
	if c := r.Header.Get("Cookie"); c != "" {
		req.Header.Set("Cookie", c)
	}
	abs := absoluteURL(r)
	req.Header.Set("X-Original-URL", abs)
	req.Header.Set("X-Forwarded-Proto", requestScheme(r))
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Uri", r.URL.RequestURI())
	// The DERIVED client IP, not the peer: the auth server must see the same
	// address gpm's own gates compared, or an IP-conditional policy on the
	// outpost would disagree with the access list in front of it.
	if ip := requestClientIP(r); ip != nil {
		req.Header.Set("X-Forwarded-For", ip.String())
	}
	return p.client.Do(req)
}

func copySetCookie(w http.ResponseWriter, resp *http.Response) {
	for _, c := range resp.Header.Values("Set-Cookie") {
		w.Header().Add("Set-Cookie", c)
	}
}

func absoluteURL(r *http.Request) string {
	return requestScheme(r) + "://" + r.Host + r.URL.RequestURI()
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func drainClose(b io.ReadCloser) {
	_, _ = io.Copy(io.Discard, b)
	_ = b.Close()
}
