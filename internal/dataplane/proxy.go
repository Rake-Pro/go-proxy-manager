package dataplane

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// dataplaneTransport is the single pooled transport shared by every upstream
// reverse proxy. Go's default transport caps idle connections per host at 2,
// which starves the many hosts that share a backend (several point at the same
// upstream) and forces constant reconnects under load - a real latency source.
// One tuned, shared pool keeps keep-alive reuse high across reloads.
var dataplaneTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          256,
	MaxIdleConnsPerHost:   64,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	// The reverse proxy must not transparently gunzip upstream bodies (CPU cost,
	// and it strips Content-Length, forcing the flush-per-write chunked path).
	DisableCompression: true,
}

// configureUpstreamTransport tunes the shared transport. Call once at startup
// before any request is served. responseHeaderTimeout caps how long the proxy
// waits for an upstream to begin its response; 0 leaves it unbounded (the prior
// behaviour). It bounds time-to-first-byte only, so it never truncates a slow
// streaming/websocket body once headers have arrived.
func configureUpstreamTransport(responseHeaderTimeout time.Duration) {
	dataplaneTransport.ResponseHeaderTimeout = responseHeaderTimeout
}

// transportFor returns the RoundTripper a host should use: the shared, pooled
// transport when it has no timeout override (the default for every host), or a
// per-host clone with adjusted dial/response timeouts. Cloning gives the override
// its own connection pool, so a custom timeout on one host never affects the
// keep-alive reuse of any other host on the shared pool.
func transportFor(t *model.HostTimeouts) http.RoundTripper {
	if t == nil || (t.ConnectSeconds <= 0 && t.ReadSeconds <= 0) {
		return dataplaneTransport
	}
	tr := dataplaneTransport.Clone()
	if t.ConnectSeconds > 0 {
		tr.DialContext = (&net.Dialer{
			Timeout:   time.Duration(t.ConnectSeconds) * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}
	if t.ReadSeconds > 0 {
		tr.ResponseHeaderTimeout = time.Duration(t.ReadSeconds) * time.Second
	}
	return tr
}

// proxyBufBytes is the fixed size of every buffer proxyBufPool hands out. A
// larger copy buffer cuts the per-write flush count for unknown-length
// (chunked) upstream responses, which the stdlib reverse proxy flushes after
// every write.
const proxyBufBytes = 512 << 10

// proxyBufPool is a sync.Pool-backed httputil.BufferPool of fixed 512KiB
// buffers, used by the main data-plane reverse proxy.
var proxyBufPool httputil.BufferPool = &bufferPool{}

type bufferPool struct {
	pool sync.Pool
}

func (p *bufferPool) Get() []byte {
	if b, ok := p.pool.Get().(*[]byte); ok {
		return *b
	}
	return make([]byte, proxyBufBytes)
}

func (p *bufferPool) Put(b []byte) {
	// Pointer, not slice header, so the Put does not allocate (staticcheck SA6002).
	p.pool.Put(&b)
}

// newReverseProxy builds the terminal reverse-proxy handler for an upstream.
// WebSocket upgrades are carried transparently by httputil.ReverseProxy when the
// request advertises them (the per-host toggle gates whether Upgrade is offered).
// timeouts is nil for the shared, pooled transport, or a per-host override. ep
// is the host's compiled errorPages override (nil if it has none); it drives
// the custom page for a 502/504 upstream-unreachable response and, when
// configured, InterceptUpstream replacement of the upstream's own error body.
func newReverseProxy(up model.Upstream, hostName string, timeouts *model.HostTimeouts, identityHeaders []string, ep *compiledErrorPages) http.Handler {
	target := &url.URL{
		Scheme: up.Scheme,
		Host:   net.JoinHostPort(up.Host, strconv.Itoa(up.Port)),
	}
	return newProxyWith(target, transportFor(timeouts), hostName, identityHeaders, ep)
}

// newGroupReverseProxy builds the terminal handler for a host backed by an
// upstream group. The Rewrite target is nominally the group's first upstream;
// the groupTransport re-points each attempt at the healthiest candidate, so the
// URL set here only seeds scheme/host for X-Forwarded computation.
func newGroupReverseProxy(gh *groupHealth, hostName string, timeouts *model.HostTimeouts, identityHeaders []string, ep *compiledErrorPages) http.Handler {
	first := gh.ups[0]
	target := &url.URL{Scheme: first.up.Scheme, Host: first.addr}
	return newProxyWith(target, &groupTransport{gh: gh, base: transportFor(timeouts)}, hostName, identityHeaders, ep)
}

// reassertIdentity re-applies the identity headers gpm itself set on the inbound
// request to the outbound one.
//
// httputil.ReverseProxy honours the client's "Connection" header by deleting
// every header it names from the outbound request, and it does that BEFORE the
// Rewrite hook runs. gpm's auth tiers (SSO gate, mTLS passthrough, forward-auth)
// have already stripped the client's own copies and set authoritative values on
// the inbound request by then - but a client that sends
// "Connection: X-Forwarded-User" gets exactly that header dropped from the
// request the upstream sees. An upstream that trusts gpm to assert identity then
// sees an ANONYMOUS request on a gated route, which for a backend that falls back
// to a default/guest identity is an authentication bypass, not a broken header.
//
// Restoring the values here, inside Rewrite, is what closes it: the deletion has
// already happened, so the last writer wins. Only headers gpm is the asserter of
// (names is the host's asserted set) and that are actually present inbound are
// restored, so this can never resurrect a header the auth tier deliberately
// stripped.
func reassertIdentity(pr *httputil.ProxyRequest, names []string) {
	for _, name := range names {
		if v, ok := pr.In.Header[name]; ok && len(v) > 0 {
			pr.Out.Header[name] = append([]string(nil), v...)
		}
	}
}

func newProxyWith(target *url.URL, transport http.RoundTripper, hostName string, identityHeaders []string, ep *compiledErrorPages) http.Handler {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()       // X-Forwarded-For / -Host / -Proto
			pr.Out.Host = pr.In.Host // preserve the client's Host header
			reassertIdentity(pr, identityHeaders)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Warn().Str("host", hostName).Str("path", r.URL.Path).Err(err).Msg("upstream error")
			if mh := metricsHook(); mh != nil {
				mh.UpstreamError(hostName)
			}
			// A timeout awaiting the upstream (dial, or ResponseHeaderTimeout/
			// context deadline) is a Gateway Timeout; anything else reaching this
			// upstream (refused, reset, TLS failure) is a Bad Gateway.
			status := http.StatusBadGateway
			if isUpstreamTimeout(err) {
				status = http.StatusGatewayTimeout
			}
			serveErrorPage(w, status, ep, hostName, func() {
				w.WriteHeader(status)
			})
		},
		ModifyResponse: func(resp *http.Response) error {
			rewriteUpstreamRedirect(resp)
			interceptUpstreamResponse(resp, ep, hostName)
			return nil
		},
		Transport:  transport,
		BufferPool: proxyBufPool,
	}
	return longLivedProxy(rp)
}

// isUpstreamTimeout reports whether err represents a timeout waiting on the
// upstream (dial timeout, TLS handshake timeout, ResponseHeaderTimeout, or a
// context deadline) rather than an outright connection failure.
func isUpstreamTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// longLivedProxy clears the listener's per-connection read/write deadlines for
// the request shapes that legitimately outlive readTimeout, so the timeout added
// for idle/slow clients can never truncate real traffic:
//
//   - a protocol upgrade (WebSocket and friends). The connection is hijacked and
//     then lives for as long as both peers want it to; any deadline still set on
//     the socket would kill it mid-session.
//   - a request carrying a body. The body is streamed to the upstream at the
//     upstream's pace, so a large or slow upload is bounded by the client and the
//     backend, not by a proxy-side read deadline. (readTimeout still applies to
//     bodies read by non-proxy handlers, and readHeaderTimeout still bounds every
//     request's headers.)
//
// A bodiless, non-upgrade request keeps its deadline - and the stdlib clears that
// one itself once the handler starts, so a long-streamed RESPONSE is unaffected
// either way.
func longLivedProxy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUpgradeRequest(r) || r.ContentLength != 0 {
			rc := http.NewResponseController(w)
			// Both are best-effort: an http.ResponseWriter that does not support
			// deadlines (an httptest recorder, HTTP/2) returns ErrNotSupported,
			// which is not a reason to fail the request.
			_ = rc.SetReadDeadline(time.Time{})
			_ = rc.SetWriteDeadline(time.Time{})
		}
		next.ServeHTTP(w, r)
	})
}

// isUpgradeRequest reports whether a request asks to switch protocols, per RFC
// 7230: "Connection: Upgrade" (token match, the header may list several) plus a
// non-empty Upgrade header.
func isUpgradeRequest(r *http.Request) bool {
	if r.Header.Get("Upgrade") == "" {
		return false
	}
	for _, v := range r.Header.Values("Connection") {
		for _, tok := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), "upgrade") {
				return true
			}
		}
	}
	return false
}

// rewriteUpstreamRedirect fixes Location headers that point back at the upstream's
// own address. Some backends (e.g. Pi-hole/civetweb) build absolute redirect URLs
// from their listening socket, ignoring the forwarded Host - so a client behind
// the proxy gets bounced to the internal host:port over http. We rewrite only
// redirects whose host matches the upstream that actually served the response
// (resp.Request.URL - correct even under group failover, where the serving
// upstream may differ from the Rewrite target), swapping in the client-facing
// scheme (from X-Forwarded-Proto, set by SetXForwarded) and host (the original
// request Host, preserved on the outbound request). Redirects to any other host -
// an IdP, an external site - are left untouched.
func rewriteUpstreamRedirect(resp *http.Response) {
	loc := resp.Header.Get("Location")
	if loc == "" {
		return
	}
	if resp.Request == nil || resp.Request.URL == nil {
		return
	}
	u, err := url.Parse(loc)
	if err != nil || !u.IsAbs() {
		return // relative redirects already resolve against the public URL
	}
	if u.Host != resp.Request.URL.Host {
		return // only rewrite redirects aimed at our own upstream
	}
	if resp.Request.Host == "" {
		return
	}
	scheme := resp.Request.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
	}
	u.Scheme = scheme
	u.Host = resp.Request.Host
	resp.Header.Set("Location", u.String())
}

// locationRoute is a path-scoped handler within a host: requests whose path has
// the prefix are served by handler (its own upstream + chain) instead of the
// host default. The prefix is matched on a segment boundary (see route).
type locationRoute struct {
	prefix   string
	handler  http.Handler
	upstream string // scheme://host:port, for debug headers/logging only
}

// hostHandler is the compiled handler for one ProxyHost: its middleware chain
// wrapping the reverse proxy. forceSSL records whether plaintext requests should
// be redirected to HTTPS by the HTTP listener. locations are path-scoped
// overrides, ordered longest-prefix first; the host handler is the fallback.
type hostHandler struct {
	host      string
	handler   http.Handler
	forceSSL  bool
	upstream  string // scheme://host:port, for debug headers/logging only
	locations []locationRoute

	// hsts is the precomputed Strict-Transport-Security header value, or "" when
	// HSTS is disabled for this host. Emitted only on the HTTPS listener.
	hsts string

	// robots is the precomputed X-Robots-Tag value ("noindex, nofollow"), or ""
	// when this host should not discourage indexing. Emitted on HTTP and HTTPS.
	robots string

	// securityHeaders is this host's effective response-header set (the
	// settings-level default merged with the host's own securityHeaders
	// override), canonicalized. nil when neither configures anything. Injected
	// set-if-absent at the router dispatch layer (see securityHeaderWriter), so
	// it reaches every gpm-generated denial/redirect/error page for this host
	// without clobbering an upstream's own security headers.
	securityHeaders http.Header

	// identityHeaders are the provider-configured identity headers this host
	// asserts; trustedNets are the peers this host trusts to set them. Both are
	// scoped to this host (see hostIdentityTrust); the baseline denylist is always
	// stripped on top, regardless of these.
	identityHeaders []string
	trustedNets     []*net.IPNet

	// certID is the compiled client-certificate identity passthrough, or nil when
	// this host does not forward the peer certificate's identity upstream. Applied
	// in serveHTTPS, always after the identity strip.
	certID *certIdentity

	// errorPages is this host's own compiled errorPages override, or nil when it
	// has none (the settings-level pages, if any, still apply - see
	// serveErrorPage). Threaded into the middleware chain and the terminal proxy
	// at build time (see buildRouter/hostProxy).
	errorPages *compiledErrorPages
}

// route returns the handler and upstream label for a request path: the
// longest-matching location prefix, or the host default when none match. The
// path is matched on a segment boundary (exact, or prefix followed by "/") so
// "/reports" does not capture "/reports-evil"; callers pass an already-cleaned
// path (see normalizeRequestPath) so dot-segments cannot dodge the match.
func (hh *hostHandler) route(path string) (http.Handler, string) {
	for _, l := range hh.locations {
		if l.prefix == "/" || path == l.prefix || strings.HasPrefix(path, l.prefix+"/") {
			return l.handler, l.upstream
		}
	}
	return hh.handler, hh.upstream
}
