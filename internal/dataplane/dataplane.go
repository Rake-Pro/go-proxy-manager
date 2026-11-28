package dataplane

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// secureCipherSuites restricts TLS 1.2 to forward-secret AEAD suites (no CBC,
// no non-PFS), removing the weak suites Go's 1.2 defaults still permit. TLS 1.3
// suites are fixed by the runtime and always AEAD.
var secureCipherSuites = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
}

// Data-plane listener timeouts. They are vars, not consts, only so tests can
// shrink them; nothing outside this package (or its tests) writes them.
//
//   - readHeaderTimeout bounds the request line + headers (slowloris).
//   - readTimeout bounds reading a whole request. The stdlib clears this
//     deadline once a request body has been consumed, so it cannot truncate a
//     streamed RESPONSE; the proxy additionally clears it outright for the two
//     request shapes that legitimately outlive it - protocol upgrades and
//     body-bearing requests being streamed to an upstream (see longLivedProxy).
//   - idleTimeout reaps keep-alive connections between requests. Without it an
//     idle connection is held for readTimeout (and, before it was set, forever),
//     so a client could pin file descriptors by opening connections and never
//     sending a second request.
var (
	readHeaderTimeout = 15 * time.Second
	readTimeout       = 60 * time.Second
	idleTimeout       = 90 * time.Second
)

// newListenerServer builds one data-plane http.Server with the shared timeout
// policy and the default header-size limit applied. Both listeners go through
// it (via Server.listenerServer) so neither can drift.
func newListenerServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           flushDeadlineHandler(h),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		// MaxHeaderBytes bounds the request line plus headers on HTTP/1 (the
		// stdlib allows 4096 bytes of read slack on top, then answers 431) and
		// sets the advertised HTTP/2 MAX_HEADER_LIST_SIZE (the stdlib adds 320
		// bytes for per-field overhead). A longer h2 header list never reaches
		// the handler: it gets 431 when it ends in the frame where it
		// overflows, and a connection error (GOAWAY PROTOCOL_ERROR) when
		// CONTINUATION frames follow.
		MaxHeaderBytes: DefaultMaxHeaderBytes,
		// WriteByteTimeout closes an HTTP/2 connection that has data to send and
		// makes no write progress; per-stream writes are bounded by the
		// progressWriter flushDeadlineHandler installs, while the handler runs
		// and after it returns. MaxConcurrentStreams bounds the requests one
		// connection can have in flight at a time.
		HTTP2: &http.HTTP2Config{
			WriteByteTimeout:     responseWriteIdleTimeout,
			MaxConcurrentStreams: http2MaxConcurrentStreams,
		},
		ConnContext: func(ctx context.Context, _ net.Conn) context.Context { return withConnAnchorCache(ctx) },
	}
}

// Bounds for the data-plane request header limit (-max-header-bytes). The
// stdlib default is 1 MiB, which is the maximum here.
const (
	DefaultMaxHeaderBytes = 64 << 10
	MinMaxHeaderBytes     = 4 << 10
	MaxMaxHeaderBytes     = 1 << 20
)

// ValidateMaxHeaderBytes reports whether n is an accepted -max-header-bytes
// value.
func ValidateMaxHeaderBytes(n int) error {
	if n < MinMaxHeaderBytes || n > MaxMaxHeaderBytes {
		return fmt.Errorf("max header bytes must be between %d and %d, got %d", MinMaxHeaderBytes, MaxMaxHeaderBytes, n)
	}
	return nil
}

// listenerServer is newListenerServer with this Server's header limit; Start
// builds both listeners through it.
func (s *Server) listenerServer(addr string, h http.Handler) *http.Server {
	srv := newListenerServer(addr, h)
	srv.MaxHeaderBytes = s.maxHeaderBytes
	return srv
}

// http2MaxConcurrentStreams is the per-connection stream limit the data-plane
// listeners advertise (the stdlib default is 250).
const http2MaxConcurrentStreams = 100

// flushDeadlineHandler bounds every write of a response by progress, whoever
// writes it: next gets a progressWriter, so a response gpm generates itself
// (an error page, a redirect) is written under the same per-write deadline as
// a proxied one, and the proxy adds no second one (see longLivedProxy). Once
// next has returned the deadline is armed again for what the server still has
// to write (the buffered tail, an HTTP/2 stream's final DATA frames). Between
// writes no deadline is set, so a long download, an event stream or a slow
// upload is never cut short. A hijacked connection belongs to its handler and
// is left alone. HTTP/1.1 clears the deadline itself before the next request
// on the connection; on HTTP/2 the deadline belongs to the stream.
func flushDeadlineHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pw := &progressWriter{ResponseWriter: w, rc: http.NewResponseController(w)}
		next.ServeHTTP(pw, r)
		// An HTTP/2 stream the client already reset has nothing left to
		// write; a deadline there would only keep the stream's state
		// reachable until its timer fired.
		if r.ProtoMajor != 2 || r.Context().Err() == nil {
			pw.arm()
		}
	})
}

// Server runs the reverse-proxy data plane. The compiled router is held in an
// atomic pointer so config reloads swap the whole routing table (and cert set)
// live, with no listener restart and no in-flight request disruption.
type Server struct {
	httpsAddr string
	httpAddr  string
	certDir   string

	// Observability toggles (all off by default; zero overhead when unset).
	// slowThreshold and debugHeaders are fixed at startup; accessLog can be
	// flipped live (SetAccessLog), which swaps each listener's handlerSwitch
	// between the plain and observed chains.
	accessLog     atomic.Bool
	slowThreshold time.Duration
	debugHeaders  bool
	// listen opens every TCP listener this plane owns (http, https, stream
	// hosts) so the MPTCP decision is made in one place.
	listen net.ListenConfig

	// logBuf retains the most recent access entries for the admin /api/logs viewer.
	// It is only written while accessLog is enabled.
	logBuf *logRing

	cur atomic.Pointer[router]

	// reloadMu serializes Reload end to end. Callers race legitimately (admin
	// API writes and the ACME renewal loop both trigger reloads), and the
	// health stage -> build -> commit sequence mutates shared prober state that
	// two interleaved reloads would corrupt (a replaced group's stopped prober
	// could be reinstated, leaving the group unprobed and a later close(stop)
	// panicking on the already-closed channel).
	reloadMu sync.Mutex

	streams *streamManager
	health  *healthManager

	httpsSrv *http.Server
	httpSrv  *http.Server

	// maxHeaderBytes is MaxHeaderBytes for both listeners (Config.MaxHeaderBytes).
	maxHeaderBytes int

	// httpSwitch/httpsSwitch are the listeners' swappable handler chains: plain
	// dispatch while every observability toggle is off, the observed wrapper
	// otherwise. nil until Start; SetAccessLog swaps them live.
	httpSwitch  *handlerSwitch
	httpsSwitch *handlerSwitch

	// acmeChallenge serves in-flight ACME HTTP-01 tokens on the plaintext
	// listener. Wired once at startup (SetACMEChallengeStore); atomic so a late
	// or replaced wiring races nothing.
	acmeChallenge atomic.Pointer[ACMEChallengeStore]

	// proxyProto holds the compiled inbound PROXY protocol config. It is read
	// per accepted connection (never cached in the listener), so a settings
	// change applies to the next connection with no listener restart. nil = off.
	proxyProto atomic.Pointer[proxyProtoConfig]

	// httpListening/httpsListening flip true once the respective net.Listen call
	// in Start succeeds, for the GET /api/health probe. A bind failure that
	// aborts startup leaves the corresponding flag false.
	httpListening  atomic.Bool
	httpsListening atomic.Bool
}

// ListenerStatus reports whether the HTTP and HTTPS data-plane listeners are
// currently bound, for the GET /api/health probe.
func (s *Server) ListenerStatus() (httpsListening, httpListening bool) {
	return s.httpsListening.Load(), s.httpListening.Load()
}

// Config holds the data plane's bind addresses, cert directory, and the optional
// observability/transport tunables.
type Config struct {
	HTTPSAddr string
	HTTPAddr  string
	CertDir   string

	// AccessLog logs every request (method, host, path, status, bytes, duration).
	// This is the initial state; SetAccessLog flips it live at runtime.
	AccessLog bool
	// SlowRequestThreshold, if >0, warn-logs any request at or above this duration
	// even when AccessLog is off - a low-noise way to surface only slow requests.
	SlowRequestThreshold time.Duration
	// DebugHeaders adds X-GPM-* diagnostic response headers (request id, matched
	// host, upstream) so routing can be inspected from the client side.
	DebugHeaders bool
	// AccessLogBufferSize is the number of recent access entries retained in memory
	// for the admin /api/logs viewer (0 selects DefaultAccessLogCapacity; other
	// values are clamped to [MinAccessLogCapacity, MaxAccessLogCapacity]). Only
	// filled while AccessLog is enabled; SetAccessLogCapacity changes it live.
	AccessLogBufferSize int
	// UpstreamResponseHeaderTimeout caps time-to-first-byte from an upstream
	// (0 = unbounded). Tunes the shared upstream transport.
	UpstreamResponseHeaderTimeout time.Duration
	// MPTCP allows Multipath TCP on the data-plane and stream listeners. Off by
	// default: nothing gpm fronts needs it, and Go 1.24+ would otherwise open
	// MPTCP-capable listeners wherever the kernel has net.mptcp.enabled=1.
	MPTCP bool
	// MaxHeaderBytes caps the request line plus headers on both data-plane
	// listeners, HTTP/1 and HTTP/2 (0 selects DefaultMaxHeaderBytes; other
	// values are clamped to [MinMaxHeaderBytes, MaxMaxHeaderBytes]).
	MaxHeaderBytes int
}

// New constructs a data-plane Server. Reload must be called with a valid config
// before Start so there is a router to serve.
func New(c Config) *Server {
	configureUpstreamTransport(c.UpstreamResponseHeaderTimeout)
	bufSize := c.AccessLogBufferSize
	if bufSize <= 0 {
		bufSize = DefaultAccessLogCapacity
	} else if bufSize < MinAccessLogCapacity {
		bufSize = MinAccessLogCapacity
	} else if bufSize > MaxAccessLogCapacity {
		bufSize = MaxAccessLogCapacity
	}
	maxHdr := c.MaxHeaderBytes
	if maxHdr <= 0 {
		maxHdr = DefaultMaxHeaderBytes
	} else {
		maxHdr = min(max(maxHdr, MinMaxHeaderBytes), MaxMaxHeaderBytes)
	}
	s := &Server{
		httpsAddr:     c.HTTPSAddr,
		httpAddr:      c.HTTPAddr,
		certDir:       c.CertDir,
		slowThreshold: c.SlowRequestThreshold,
		debugHeaders:  c.DebugHeaders,
		logBuf:        newLogRing(bufSize),
		streams:       newStreamManager(),
		health:        newHealthManager(),
	}
	s.maxHeaderBytes = maxHdr
	s.accessLog.Store(c.AccessLog)
	s.listen.SetMultipathTCP(c.MPTCP)
	s.streams.listen = &s.listen
	// Stream listeners read the PROXY protocol config through this accessor, so a
	// listener opened before the setting arrives (or while it is off) still picks
	// the change up on its next connection instead of needing a rebind.
	s.streams.proxyProto = s.currentProxyProtocol
	return s
}

// ClearLogs empties the access-log ring (admin DELETE /api/logs). Capture
// state is untouched.
func (s *Server) ClearLogs() {
	if s.logBuf != nil {
		s.logBuf.clear()
	}
	log.Info().Msg("access-log buffer cleared")
}

// Bounds for the access-log ring: a few hundred bytes per entry, so the
// maximum is tens of MB at worst.
const (
	DefaultAccessLogCapacity = 5000
	MinAccessLogCapacity     = 100
	MaxAccessLogCapacity     = 100000
)

// AccessLogCapacity is the number of entries the ring retains before the
// oldest is overwritten.
func (s *Server) AccessLogCapacity() int {
	if s.logBuf == nil {
		return 0
	}
	return s.logBuf.capacity()
}

// SetAccessLogCapacity resizes the ring live (admin PUT /api/logs with
// "capacity"), keeping the newest entries that fit. Runtime-only like the
// toggle: the -access-log-buffer flag decides the size after a restart.
func (s *Server) SetAccessLogCapacity(n int) error {
	if n < MinAccessLogCapacity || n > MaxAccessLogCapacity {
		return fmt.Errorf("capacity must be between %d and %d", MinAccessLogCapacity, MaxAccessLogCapacity)
	}
	if s.logBuf == nil {
		return errors.New("access-log buffer is not wired")
	}
	s.logBuf.resize(n)
	log.Info().Int("capacity", n).Msg("access-log buffer resized")
	return nil
}

// AccessLogEnabled reports whether request capture is on. When false the /api/logs
// viewer has nothing to show and the UI surfaces how to enable it.
func (s *Server) AccessLogEnabled() bool { return s.accessLog.Load() }

// SetAccessLog flips request capture live (admin PUT /api/logs). The change is
// runtime-only: the -access-log flag decides the state after a restart. Each
// listener's switch is re-derived from the stored value, so concurrent toggles
// converge on the last write without extra locking. An in-flight request
// finishes on the chain it started with.
func (s *Server) SetAccessLog(enabled bool) {
	s.accessLog.Store(enabled)
	active := s.observeActive()
	if s.httpSwitch != nil {
		s.httpSwitch.set(active)
	}
	if s.httpsSwitch != nil {
		s.httpsSwitch.set(active)
	}
	log.Info().Bool("enabled", enabled).Msg("access logging toggled")
}

// RefreshObserve re-evaluates which chain the listeners serve after an
// observability input changed outside this package (the analytics collector
// was switched on or off by a settings reload).
func (s *Server) RefreshObserve() {
	active := s.observeActive()
	if s.httpSwitch != nil {
		s.httpSwitch.set(active)
	}
	if s.httpsSwitch != nil {
		s.httpsSwitch.set(active)
	}
}

// observeActive reports whether requests must run through the observed chain
// right now: the live access-log toggle, or any startup toggle that always
// needs the wrapper.
func (s *Server) observeActive() bool {
	return s.accessLog.Load() || s.slowThreshold > 0 || s.debugHeaders || metricsHook() != nil || analyticsActive()
}

// UpstreamHealth returns the live health of every upstream group's upstreams,
// keyed by group name, for the admin status API.
func (s *Server) UpstreamHealth() map[string][]UpstreamStatus {
	return s.health.snapshot()
}

// RecentLogs returns the buffered access entries, newest first (nil when capture
// has never run).
func (s *Server) RecentLogs() []AccessEntry {
	if s.logBuf == nil {
		return nil
	}
	return s.logBuf.recent()
}

// Reload compiles cfg into a new router (HTTP/S hosts) and reconciles the raw
// TCP/UDP stream listeners and upstream-group health probers, swapping the
// router in atomically. Group health is staged first and committed only after
// the router compiles, so a rejected config leaves the running probers (and
// their accumulated up/down state) untouched. The idle connections of the
// pools only the replaced router used are closed (see closeReplacedIdle).
func (s *Server) Reload(cfg model.Config) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	stage := s.health.stage(cfg.UpstreamGroups)
	rt, err := buildRouter(cfg, s.certDir, stage)
	if err != nil {
		return fmt.Errorf("compile data plane: %w", err)
	}
	stage.commit()
	rt.live = s.current
	if old := s.cur.Swap(rt); old != nil {
		closeReplacedIdle(old, rt)
	}
	// Published only now: a build that failed above never replaces the anchors
	// the live router checks revocation against.
	registerCRLAnchors(rt.clientCAs)
	// Upgraded connections whose host, chain or client certificate this
	// build no longer admits are closed.
	recheckTunnels()
	s.streams.reload(cfg, rt.certs)
	log.Info().
		Int("proxyHosts", len(rt.hosts)).
		Int("redirectHosts", len(rt.redirects)).
		Int("parkedHosts", len(rt.parked)).
		Int("streamHosts", len(cfg.StreamHosts)).
		Msg("data plane reloaded")
	return nil
}

// CheckConfig compiles cfg and the settings-level error pages exactly as a
// reload would and discards the result, so a config write can be refused
// before it is committed. It changes no live state: the router is not swapped,
// the CRL watch keeps the live anchors, upstream-group probers are neither
// started nor stopped (the health stage is never committed), stream listeners
// are untouched and the installed error pages stay in place.
func (s *Server) CheckConfig(cfg model.Config, errorPages model.ErrorPagesConfig) error {
	if _, err := compileErrorPages(errorPages, s.certDir); err != nil {
		return fmt.Errorf("settings errorPages: %w", err)
	}
	if _, err := buildRouter(cfg, s.certDir, s.health.stage(cfg.UpstreamGroups)); err != nil {
		return fmt.Errorf("compile data plane: %w", err)
	}
	return nil
}

func (s *Server) current() *router { return s.cur.Load() }

// Start runs the HTTP and HTTPS listeners until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	if s.current() == nil {
		return fmt.Errorf("data plane: Reload must be called before Start")
	}

	s.httpSwitch = s.dataHandler(s.dispatchHTTP)
	s.httpsSwitch = s.dataHandler(s.dispatchHTTPS)
	s.httpSrv = s.listenerServer(s.httpAddr, s.httpSwitch)
	s.httpsSrv = s.listenerServer(s.httpsAddr, s.httpsSwitch)
	// GetCertificate reads the live router so cert changes apply on reload.
	// GetConfigForClient lets a host pin a higher minimum TLS version than the
	// 1.2 floor: it returns that host's config (by SNI) or nil for the default.
	s.httpsSrv.TLSConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: secureCipherSuites,
		NextProtos:   []string{"h2", "http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return s.current().certs.GetCertificate(hello)
		},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			return s.current().tlsConfigForSNI(hello.ServerName), nil
		},
	}

	// Poll every configured client-CA CRL file for an out-of-band refresh, the
	// same way the GeoIP database is watched. Reload already re-reads them; this
	// covers a CRL that changes with no config change.
	go watchCRLs(ctx, crlWatchInterval)

	// The listeners are opened explicitly rather than via ListenAndServe so the
	// inbound PROXY protocol wrapper can sit under them. A bare ":port" address
	// binds the v6 wildcard with v4-mapped addresses enabled (Go's default
	// dual-stack behaviour), so both families reach the same handlers and the
	// client IP an access list sees is whichever family the client used. Both
	// go through s.listen so the MPTCP opt-out applies.
	httpLn, err := s.listen.Listen(ctx, "tcp", s.httpAddr)
	if err != nil {
		return fmt.Errorf("http listener: %w", err)
	}
	s.httpListening.Store(true)
	httpsLn, err := s.listen.Listen(ctx, "tcp", s.httpsAddr)
	if err != nil {
		_ = httpLn.Close()
		s.httpListening.Store(false)
		return fmt.Errorf("https listener: %w", err)
	}
	s.httpsListening.Store(true)
	httpLn = wrapProxyProtocol(httpLn, s.currentProxyProtocol)
	httpsLn = wrapProxyProtocol(httpsLn, s.currentProxyProtocol)

	errCh := make(chan error, 2)
	go func() {
		log.Info().Str("addr", s.httpAddr).Msg("data plane HTTP listening")
		if err := s.httpSrv.Serve(httpLn); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http listener: %w", err)
		}
	}()
	go func() {
		log.Info().Str("addr", s.httpsAddr).Msg("data plane HTTPS listening")
		if err := s.httpsSrv.ServeTLS(httpsLn, "", ""); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("https listener: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// A reload in flight finishes (and commits its stage) before the
		// stream listeners and probers are stopped.
		s.reloadMu.Lock()
		s.streams.stopAll()
		s.health.stopAll()
		s.reloadMu.Unlock()
		_ = s.httpSrv.Shutdown(shutdownCtx)
		_ = s.httpsSrv.Shutdown(shutdownCtx)
		return nil
	}
}

// SetProxyProtocol configures inbound PROXY protocol from settings. It applies
// to the HTTP/HTTPS listeners and to every TCP stream listener, and takes effect
// on the next accepted connection - no listener restart, no dropped conns. A nil
// or disabled setting turns it off and restores untouched connections.
//
// Settings live outside model.Config, so this is called alongside Reload rather
// than from it.
func (s *Server) SetProxyProtocol(p *model.ProxyProtocolSettings) {
	cfg := compileProxyProtocol(p)
	s.proxyProto.Store(cfg)
	if cfg == nil {
		log.Debug().Msg("proxy protocol: disabled")
		return
	}
	log.Info().Int("trustedCIDRs", len(cfg.trusted)).Dur("timeout", cfg.timeout).
		Msg("proxy protocol: inbound headers accepted from trusted peers")
}

// currentProxyProtocol is the live config read by every listener wrapper.
func (s *Server) currentProxyProtocol() *proxyProtoConfig { return s.proxyProto.Load() }

// SetACMEChallengeStore wires the ACME manager's in-flight HTTP-01 tokens into
// the plaintext listener. Passing nil detaches it.
func (s *Server) SetACMEChallengeStore(store ACMEChallengeStore) {
	if store == nil {
		s.acmeChallenge.Store(nil)
		return
	}
	s.acmeChallenge.Store(&store)
}

func (s *Server) acmeChallengeStore() ACMEChallengeStore {
	if p := s.acmeChallenge.Load(); p != nil {
		return *p
	}
	return nil
}

func (s *Server) dispatchHTTP(w http.ResponseWriter, r *http.Request) {
	// ACME HTTP-01 first: before host routing, force-SSL redirect, and auth.
	if serveACMEChallenge(s.acmeChallengeStore(), w, r) {
		return
	}
	s.current().serveHTTP(w, r)
}

func (s *Server) dispatchHTTPS(w http.ResponseWriter, r *http.Request) {
	s.current().serveHTTPS(w, r)
}
