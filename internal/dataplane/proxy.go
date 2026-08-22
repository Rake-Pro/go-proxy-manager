package dataplane

import (
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
	if b, ok := p.pool.Get().([]byte); ok {
		return b
	}
	return make([]byte, proxyBufBytes)
}

func (p *bufferPool) Put(b []byte) {
	p.pool.Put(b)
}

// newReverseProxy builds the terminal reverse-proxy handler for an upstream.
// WebSocket upgrades are carried transparently by httputil.ReverseProxy when the
// request advertises them (the per-host toggle gates whether Upgrade is offered).
// timeouts is nil for the shared, pooled transport, or a per-host override.
func newReverseProxy(up model.Upstream, hostName string, timeouts *model.HostTimeouts) *httputil.ReverseProxy {
	target := &url.URL{
		Scheme: up.Scheme,
		Host:   net.JoinHostPort(up.Host, strconv.Itoa(up.Port)),
	}
	return newProxyWith(target, transportFor(timeouts), hostName)
}

// newGroupReverseProxy builds the terminal handler for a host backed by an
// upstream group. The Rewrite target is nominally the group's first upstream;
// the groupTransport re-points each attempt at the healthiest candidate, so the
// URL set here only seeds scheme/host for X-Forwarded computation.
func newGroupReverseProxy(gh *groupHealth, hostName string, timeouts *model.HostTimeouts) *httputil.ReverseProxy {
	first := gh.ups[0]
	target := &url.URL{Scheme: first.up.Scheme, Host: first.addr}
	return newProxyWith(target, &groupTransport{gh: gh, base: transportFor(timeouts)}, hostName)
}

func newProxyWith(target *url.URL, transport http.RoundTripper, hostName string) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()       // X-Forwarded-For / -Host / -Proto
			pr.Out.Host = pr.In.Host // preserve the client's Host header
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Warn().Str("host", hostName).Str("path", r.URL.Path).Err(err).Msg("upstream error")
			w.WriteHeader(http.StatusBadGateway)
		},
		ModifyResponse: func(resp *http.Response) error {
			rewriteUpstreamRedirect(resp)
			return nil
		},
		Transport:  transport,
		BufferPool: proxyBufPool,
	}
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
