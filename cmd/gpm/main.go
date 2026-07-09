// Command gpm is the go-proxy-manager daemon: a native-Go reverse-proxy manager
// with a git-backed declarative config store and first-class SSO.
//
// P0a scaffolds the foundation: config store, honest versioning, admin health.
// The proxy data plane, ACME, and auth land in subsequent phases.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/acme"
	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/dataplane"
	"github.com/Rake-Pro/go-proxy-manager/internal/geoip"
	"github.com/Rake-Pro/go-proxy-manager/internal/logging"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/server"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	"github.com/Rake-Pro/go-proxy-manager/internal/ui"
	"github.com/Rake-Pro/go-proxy-manager/internal/version"
	"github.com/Rake-Pro/go-proxy-manager/internal/webhook"
	"github.com/rs/zerolog/log"
)

func main() {
	// Subcommands. Default (no subcommand) runs the daemon.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "import":
			runImport(os.Args[2:])
			return
		case "hashpw":
			runHashpw(os.Args[2:])
			return
		}
	}

	var (
		configDir     = flag.String("config-dir", envOr("GPM_CONFIG_DIR", "/data/config"), "git-backed config repository directory")
		certDir       = flag.String("cert-dir", envOr("GPM_CERT_DIR", "/data/certs"), "certificate storage directory")
		adminAddr     = flag.String("admin-addr", envOr("GPM_ADMIN_ADDR", ":8081"), "admin HTTP listen address")
		httpsAddr     = flag.String("https-addr", envOr("GPM_HTTPS_ADDR", ":443"), "data plane HTTPS listen address")
		httpAddr      = flag.String("http-addr", envOr("GPM_HTTP_ADDR", ":80"), "data plane HTTP listen address")
		sessionDB     = flag.String("session-db", envOr("GPM_SESSION_DB", "/data/session.db"), "session database path")
		localUser     = flag.String("local-admin-user", os.Getenv("GPM_LOCAL_ADMIN_USER"), "local admin username")
		cookieSecure  = flag.Bool("cookie-secure", os.Getenv("GPM_COOKIE_SECURE") != "0", "set the Secure flag on session cookies (disable only for local HTTP testing)")
		logLevel      = flag.String("log-level", envOr("GPM_LOG_LEVEL", "info"), "log level (trace|debug|info|warn|error)")
		logConsole    = flag.Bool("log-console", os.Getenv("GPM_LOG_CONSOLE") == "1", "human-friendly console logging")
		accessLog     = flag.Bool("access-log", os.Getenv("GPM_ACCESS_LOG") == "1", "log every data-plane request (method, host, path, status, bytes, duration)")
		slowReqMS     = flag.Int("slow-request-ms", envInt("GPM_SLOW_REQUEST_MS", 0), "warn-log data-plane requests slower than N ms, even with access-log off (0 = disabled)")
		debugHeaders  = flag.Bool("debug-headers", os.Getenv("GPM_DEBUG_HEADERS") == "1", "add X-GPM-* diagnostic response headers (request id, matched host, upstream)")
		upstreamHdrTO = flag.Duration("upstream-response-header-timeout", envDur("GPM_UPSTREAM_RESPONSE_HEADER_TIMEOUT", 0), "cap on time awaiting upstream response headers, e.g. 30s (0 = unbounded)")
		geoDBPath     = flag.String("geoip-db", envOr("GPM_GEOIP_DB", ""), "path to an operator-supplied GeoLite2/GeoIP2 .mmdb file for AccessList geo rules (unset disables geo rules; none is bundled)")
		pprofEnabled  = flag.Bool("pprof", os.Getenv("GPM_PPROF") == "1", "expose net/http/pprof profiling endpoints on the admin server at /debug/pprof/ (admin-role gated)")
		showVer       = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println(version.String())
		return
	}

	// The bcrypt admin hash is read only from the environment / _FILE secret,
	// never a CLI flag (a flag value is visible in the process table).
	localHash := secretFromEnv("GPM_LOCAL_ADMIN_PASSWORD_HASH")

	logging.Setup(*logLevel, *logConsole)
	log.Info().Str("build", version.String()).Msg("starting go-proxy-manager")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st := store.New(*configDir, store.NewExecGit(*configDir))
	if err := st.Init(ctx); err != nil {
		log.Fatal().Err(err).Str("dir", *configDir).Msg("failed to initialise config store")
	}

	cfg, settings, err := st.Load(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}
	log.Info().
		Int("proxyHosts", len(cfg.ProxyHosts)).
		Int("redirectHosts", len(cfg.RedirectHosts)).
		Int("streamHosts", len(cfg.StreamHosts)).
		Int("deadHosts", len(cfg.DeadHosts)).
		Int("certificates", len(cfg.Certificates)).
		Int("identityProviders", len(cfg.IdentityProviders)).
		Int("accessLists", len(cfg.AccessLists)).
		Int("middlewares", len(cfg.Middlewares)).
		Str("externalBaseURL", settings.ExternalBaseURL).
		Msg("config loaded")

	dp := dataplane.New(dataplane.Config{
		HTTPSAddr:                     *httpsAddr,
		HTTPAddr:                      *httpAddr,
		CertDir:                       *certDir,
		AccessLog:                     *accessLog,
		SlowRequestThreshold:          time.Duration(*slowReqMS) * time.Millisecond,
		DebugHeaders:                  *debugHeaders,
		UpstreamResponseHeaderTimeout: *upstreamHdrTO,
	})
	// Persist the data-plane SSO signing key under the cert dir when the operator
	// has not pinned one via GPM_SSO_SIGNING_KEY, so SSO sessions survive restarts.
	dataplane.SetSSOKeyDir(*certDir)
	if *accessLog || *slowReqMS > 0 || *debugHeaders {
		log.Info().
			Bool("accessLog", *accessLog).
			Int("slowRequestMs", *slowReqMS).
			Bool("debugHeaders", *debugHeaders).
			Msg("data plane debug toggles enabled")
	}

	// GeoIP: no database is bundled (GeoLite2 licensing forbids redistribution),
	// so a load failure here is not fatal - it just means access lists with geo
	// rules deny all traffic on their hosts (live fail-closed evaluation in the
	// access-list chain, see dataplane/accesslist.go's ipAllowed) and the store
	// refuses to commit any new geo rule (see Store.SetGeoAvailability below)
	// until the operator fixes GPM_GEOIP_DB. Watch keeps picking up an
	// out-of-band refresh (e.g. a geoipupdate cron) without requiring a gpm
	// config change or restart.
	geoResolver := &geoip.Resolver{}
	if *geoDBPath != "" {
		if err := geoResolver.Reload(*geoDBPath); err != nil {
			log.Error().Err(err).Str("path", *geoDBPath).
				Msg("failed to load initial GeoIP database; access lists with geo rules will refuse to activate until this is fixed")
		} else {
			log.Info().Str("path", *geoDBPath).Msg("GeoIP database loaded")
		}
		go geoResolver.Watch(ctx, *geoDBPath, geoip.DefaultWatchInterval)
	}
	dataplane.SetGeoDB(geoResolver)
	// Reject-at-write: the store refuses to commit an AccessList with geo rules
	// while no GeoIP database is loaded, so such a rule never lands in git. The
	// data-plane compile fails closed (deny) for any geo rule already committed
	// before the DB went missing, so a geo host can never boot-loop or serve open.
	st.SetGeoAvailability(geoResolver.Loaded)

	if err := dp.Reload(cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to compile data plane")
	}

	sessStore, err := session.Open(*sessionDB)
	if err != nil {
		log.Fatal().Err(err).Str("path", *sessionDB).Msg("failed to open session store")
	}
	defer sessStore.Close()
	go sessionGC(ctx, sessStore)

	authn := auth.NewAuthenticator(auth.Options{
		Store:     sessStore,
		Secure:    *cookieSecure,
		LocalUser: *localUser,
		LocalHash: localHash,
	})
	authn.Configure(cfg, settings)

	// reload re-reads the config and applies it to both the auth layer and the
	// data plane. It is the single path used after any config or certificate
	// change (API writes, ACME issuance) so the running state never drifts.
	reload := func() error {
		c, st2, err := st.Load(ctx)
		if err != nil {
			log.Error().Err(err).Msg("reload: failed to load config")
			return err
		}
		// Reload the data plane FIRST: only reconfigure the auth layer once the
		// data plane has accepted the new config, so a rejected reload never
		// leaves auth and dataplane drifted against each other.
		if err := dp.Reload(c); err != nil {
			log.Error().Err(err).Msg("reload: failed to reload data plane")
			return err
		}
		authn.Configure(c, st2)
		return nil
	}

	// ACME manager: issues/renews DNS-01 certs and reloads on certificate change.
	// The reload error is already logged; ACME has no caller to surface it to.
	acmeMgr := acme.NewManager(acme.Options{CertDir: *certDir, OnChange: func() { _ = reload() }})
	go acmeMgr.Run(ctx, 0, func(ctx context.Context) (model.Config, error) {
		c, _, err := st.Load(ctx)
		return c, err
	})

	// Lifecycle webhooks: fire-and-forget notifications after each config change,
	// reading the live targets from settings on every event.
	hooks := webhook.New(func() []model.WebhookConfig {
		_, s2, err := st.Load(ctx)
		if err != nil {
			return nil
		}
		return s2.Webhooks
	})

	// REST CRUD API: writes go through the git-backed store; commits are authored
	// by the requesting admin principal; OnChange reloads the running state.
	apiHandler := api.New(api.Deps{
		Store:    st,
		OnChange: reload,
		Author: func(r *http.Request) store.Author {
			p, _ := auth.PrincipalFrom(r.Context())
			name := p.Name
			if name == "" {
				name = p.Subject
			}
			return store.Author{Name: name, Email: p.Email}
		},
		OnEvent: func(action, kind, name, commit string) {
			hooks.Dispatch(webhook.Event{Action: action, Kind: kind, Name: name, Commit: commit})
		},
		RecentLogs: func() any {
			return map[string]any{"enabled": dp.AccessLogEnabled(), "entries": dp.RecentLogs()}
		},
		GeoDBLoaded: geoResolver.Loaded,
	})

	uiHandler, err := ui.Handler()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialise admin UI")
	}

	admin := server.New(*adminAddr, st, authn, apiHandler, uiHandler, *pprofEnabled)

	errc := make(chan error, 2)
	go func() { errc <- admin.Start(ctx) }()
	go func() { errc <- dp.Start(ctx) }()

	if err := <-errc; err != nil {
		log.Error().Err(err).Msg("server error, shutting down")
		stop() // cancel ctx so the sibling server shuts down too
	}
	<-errc
	log.Info().Msg("shutdown complete")
}

// sessionGC periodically purges expired sessions.
func sessionGC(ctx context.Context, st *session.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := st.GC(ctx); err != nil {
				log.Warn().Err(err).Msg("session GC failed")
			} else if n > 0 {
				log.Debug().Int("removed", n).Msg("expired sessions purged")
			}
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// secretFromEnv reads a secret value, preferring <key>_FILE (a file path, e.g. a
// mounted Docker secret) over the plain <key> env var. Reading from a file keeps
// secrets like the bcrypt admin hash - whose $ signs are otherwise mangled by
// shell/compose interpolation - out of the environment entirely.
func secretFromEnv(key string) string {
	if p := os.Getenv(key + "_FILE"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read %s_FILE (%s): %v\n", key, p, err)
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	return os.Getenv(key)
}
