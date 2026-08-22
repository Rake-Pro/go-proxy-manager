package dataplane

import (
	"context"
	"crypto/tls"
	"fmt"
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

// Server runs the reverse-proxy data plane. The compiled router is held in an
// atomic pointer so config reloads swap the whole routing table (and cert set)
// live, with no listener restart and no in-flight request disruption.
type Server struct {
	httpsAddr string
	httpAddr  string
	certDir   string

	// Observability toggles (all off by default; zero overhead when unset).
	accessLog     bool
	slowThreshold time.Duration
	debugHeaders  bool

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
}

// Config holds the data plane's bind addresses, cert directory, and the optional
// observability/transport tunables.
type Config struct {
	HTTPSAddr string
	HTTPAddr  string
	CertDir   string

	// AccessLog logs every request (method, host, path, status, bytes, duration).
	AccessLog bool
	// SlowRequestThreshold, if >0, warn-logs any request at or above this duration
	// even when AccessLog is off - a low-noise way to surface only slow requests.
	SlowRequestThreshold time.Duration
	// DebugHeaders adds X-GPM-* diagnostic response headers (request id, matched
	// host, upstream) so routing can be inspected from the client side.
	DebugHeaders bool
	// AccessLogBufferSize is the number of recent access entries retained in memory
	// for the admin /api/logs viewer (0 selects a default). Only filled while
	// AccessLog is enabled.
	AccessLogBufferSize int
	// UpstreamResponseHeaderTimeout caps time-to-first-byte from an upstream
	// (0 = unbounded). Tunes the shared upstream transport.
	UpstreamResponseHeaderTimeout time.Duration
}

// New constructs a data-plane Server. Reload must be called with a valid config
// before Start so there is a router to serve.
func New(c Config) *Server {
	configureUpstreamTransport(c.UpstreamResponseHeaderTimeout)
	bufSize := c.AccessLogBufferSize
	if bufSize <= 0 {
		bufSize = 1000
	}
	return &Server{
		httpsAddr:     c.HTTPSAddr,
		httpAddr:      c.HTTPAddr,
		certDir:       c.CertDir,
		accessLog:     c.AccessLog,
		slowThreshold: c.SlowRequestThreshold,
		debugHeaders:  c.DebugHeaders,
		logBuf:        newLogRing(bufSize),
		streams:       newStreamManager(),
		health:        newHealthManager(),
	}
}

// AccessLogEnabled reports whether request capture is on. When false the /api/logs
// viewer has nothing to show and the UI surfaces how to enable it.
func (s *Server) AccessLogEnabled() bool { return s.accessLog }

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
// their accumulated up/down state) untouched.
func (s *Server) Reload(cfg model.Config) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	stage := s.health.stage(cfg.UpstreamGroups)
	rt, err := buildRouter(cfg, s.certDir, stage)
	if err != nil {
		return fmt.Errorf("compile data plane: %w", err)
	}
	stage.commit()
	s.cur.Store(rt)
	s.streams.reload(cfg.StreamHosts)
	log.Info().
		Int("proxyHosts", len(rt.hosts)).
		Int("redirectHosts", len(rt.redirects)).
		Int("deadHosts", len(rt.dead)).
		Int("streamHosts", len(cfg.StreamHosts)).
		Msg("data plane reloaded")
	return nil
}

func (s *Server) current() *router { return s.cur.Load() }

// Start runs the HTTP and HTTPS listeners until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	if s.current() == nil {
		return fmt.Errorf("data plane: Reload must be called before Start")
	}

	s.httpSrv = &http.Server{
		Addr:              s.httpAddr,
		Handler:           s.observe(http.HandlerFunc(s.dispatchHTTP)),
		ReadHeaderTimeout: 15 * time.Second,
	}
	s.httpsSrv = &http.Server{
		Addr:              s.httpsAddr,
		Handler:           s.observe(http.HandlerFunc(s.dispatchHTTPS)),
		ReadHeaderTimeout: 15 * time.Second,
		// GetCertificate reads the live router so cert changes apply on reload.
		// GetConfigForClient lets a host pin a higher minimum TLS version than the
		// 1.2 floor: it returns that host's config (by SNI) or nil for the default.
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			CipherSuites: secureCipherSuites,
			NextProtos:   []string{"h2", "http/1.1"},
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				return s.current().certs.GetCertificate(hello)
			},
			GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
				return s.current().tlsConfigForSNI(hello.ServerName), nil
			},
		},
	}

	// Poll every configured client-CA CRL file for an out-of-band refresh, the
	// same way the GeoIP database is watched. Reload already re-reads them; this
	// covers a CRL that changes with no config change.
	go watchCRLs(ctx, crlWatchInterval)

	errCh := make(chan error, 2)
	go func() {
		log.Info().Str("addr", s.httpAddr).Msg("data plane HTTP listening")
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http listener: %w", err)
		}
	}()
	go func() {
		log.Info().Str("addr", s.httpsAddr).Msg("data plane HTTPS listening")
		if err := s.httpsSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("https listener: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s.streams.stopAll()
		s.health.stopAll()
		_ = s.httpSrv.Shutdown(shutdownCtx)
		_ = s.httpsSrv.Shutdown(shutdownCtx)
		return nil
	}
}

func (s *Server) dispatchHTTP(w http.ResponseWriter, r *http.Request) {
	s.current().serveHTTP(w, r)
}

func (s *Server) dispatchHTTPS(w http.ResponseWriter, r *http.Request) {
	s.current().serveHTTPS(w, r)
}
