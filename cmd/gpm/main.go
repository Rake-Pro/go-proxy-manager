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
	"sync"
	"syscall"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/accesssync"
	"github.com/Rake-Pro/go-proxy-manager/internal/acme"
	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/dataplane"
	"github.com/Rake-Pro/go-proxy-manager/internal/dnssync"
	"github.com/Rake-Pro/go-proxy-manager/internal/geoip"
	"github.com/Rake-Pro/go-proxy-manager/internal/ha"
	"github.com/Rake-Pro/go-proxy-manager/internal/k8s"
	"github.com/Rake-Pro/go-proxy-manager/internal/logging"
	"github.com/Rake-Pro/go-proxy-manager/internal/metrics"
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
		haRole        = flag.String("ha-role", envOr("GPM_HA_ROLE", string(ha.RoleLeader)), "HA role: leader (runs ACME and Ingress discovery, accepts config writes) or follower (read-only, pulls the leader's config repo)")
		haPollInt     = flag.Duration("ha-poll-interval", envDur("GPM_HA_POLL_INTERVAL", store.DefaultFollowInterval), "how often a follower fast-forwards the config repo from the leader remote")
		pprofEnabled  = flag.Bool("pprof", os.Getenv("GPM_PPROF") == "1", "expose net/http/pprof profiling endpoints on the admin server at /debug/pprof/ (admin role + admin scope gated)")
		metricsOn     = flag.Bool("metrics", os.Getenv("GPM_METRICS") == "1", "expose a Prometheus text exposition on the admin server at /metrics (admin role + metrics:read scope gated)")
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

	// HA role (docs/design/ha.md phase 1). Exactly one instance is the writer:
	// the leader runs the ACME and Ingress-discovery loops and accepts config
	// writes; a follower does neither and pulls the leader's repo instead. An
	// unparseable value is fatal rather than defaulting, so a typo cannot start a
	// second ACME writer against the same account.
	role, err := ha.ParseRole(*haRole)
	if err != nil {
		log.Fatal().Err(err).Msg("invalid HA role")
	}
	log.Info().Str("role", role.String()).Msg("HA role")

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
		Int("parkedHosts", len(cfg.ParkedHosts)).
		Int("certificates", len(cfg.Certificates)).
		Int("identityProviders", len(cfg.IdentityProviders)).
		Int("accessLists", len(cfg.AccessLists)).
		Int("middlewares", len(cfg.Middlewares)).
		Str("externalBaseURL", settings.ExternalBaseURL).
		Msg("config loaded")

	// Prometheus metrics, opt-in. The registry is built before the data plane so
	// SetMetricsHook lands before the first Reload and before Start: the observe
	// wrapper captures the hook when Start builds the handler chains, and the
	// listeners' handler switch serves the plain chain while every observability
	// toggle is off, so a run without -metrics carries no per-request cost.
	var mx *metrics.Metrics
	if *metricsOn {
		bi := version.Get()
		mx = metrics.NewMetrics(bi.Version, bi.Commit, bi.Go)
		mx.RegisterHA(role.String())
		dataplane.SetMetricsHook(mx)
		log.Info().Msg("prometheus metrics enabled (GET /metrics on the admin listener)")
	}

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
	// Re-read the SSO revocation watermark periodically so a revoke issued on a
	// peer (shared cert dir) - or edited out of band - is honored here without a
	// restart, instead of only at the next process start.
	go dataplane.WatchSSOWatermark(ctx, 0)
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

	// Inbound PROXY protocol (settings, not Config) is applied before the first
	// compile so the stream listeners opened by Reload are wrapped from their
	// very first connection.
	dp.SetProxyProtocol(settings.ProxyProtocol)

	// Settings-level custom error pages, compiled before the first Reload for
	// the same reason as PROXY protocol above: it is consulted live by every
	// host that has no errorPages override of its own, so it must be in place
	// before any request can be served.
	if err := dataplane.SetErrorPages(settings.ErrorPages, *certDir); err != nil {
		log.Fatal().Err(err).Msg("failed to compile settings-level error pages")
	}

	// Settings-level default security headers, installed before the first Reload
	// for the same reason as the error pages above: buildRouter composes each
	// host's effective set from this default, and host-less responses (404/421,
	// redirect/parked hosts) read it directly.
	dataplane.SetSecurityHeaders(settings.SecurityHeaders)

	// Settings-level default response-header strip list, installed for the same
	// reason: buildRouter unions it into each proxy host's effective list.
	dataplane.SetStripResponseHeaders(settings.StripResponseHeaders)

	// Fleet-wide maintenance switch, installed before the first request for the
	// same reason as the error pages: it is read live on every request, so a
	// deployment that boots with maintenance on must never serve a single request
	// to an upstream first.
	dataplane.SetMaintenance(settings.Maintenance)

	// Fetched access-list source sets, installed before the first Reload for the
	// same reason as the error pages above: compileAccessList resolves a
	// source-backed rule against them at build time, so a restart must serve the
	// last committed set rather than an empty one until the first fetch lands.
	// A ledger that will not load is NOT fatal: an unreadable ledger leaves every
	// source rule matching nothing, which denies, and denying is the safe half of
	// the failure - refusing to boot the whole edge is not.
	if l, _, err := st.LoadAccessListSourceLedger(ctx); err != nil {
		log.Error().Err(err).Msg("failed to load the access-list source ledger; source-backed rules match nothing until the next successful fetch")
	} else {
		dataplane.SetAccessListSources(l)
	}

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

	// With neither a local admin nor any OIDC provider configured, nothing can
	// ever authenticate to the admin panel - and it fails silently (a login
	// page with no way to log in) rather than with an error. Name both knobs so
	// this is diagnosable from the startup log instead of a support thread.
	if (*localUser == "" || localHash == "") && len(settings.AdminAuth.Providers) == 0 {
		log.Warn().Msg("no admin authentication configured: set GPM_LOCAL_ADMIN_USER/GPM_LOCAL_ADMIN_PASSWORD_HASH for local login, or settings.adminAuth.providers for OIDC - the admin panel is unreachable until one is set")
	}

	// Scoped API tokens. The authenticator caches this closure's result and drops
	// the cache from reload() below, so a token created, disabled or deleted
	// through the API takes effect immediately WITHOUT an unauthenticated bearer
	// attempt being able to force a full config load per request.
	authn.SetTokenSource(func() []model.APIToken {
		c, _, err := st.Load(ctx)
		if err != nil {
			log.Error().Err(err).Msg("api token auth: failed to load config; refusing token authentication")
			return nil
		}
		return c.APITokens
	})

	// Production guard for GPM_COOKIE_SECURE=0: an https externalBaseURL says
	// this deployment is TLS-fronted, so non-Secure admin session cookies are a
	// misconfiguration (they would ride any plain-HTTP request). Warn loudly
	// rather than refuse - a LAN-only plain-HTTP admin listener alongside the
	// public URL is a known deliberate setup, and hard-failing would brick it.
	if !*cookieSecure && strings.HasPrefix(strings.ToLower(settings.ExternalBaseURL), "https://") {
		log.Warn().Msg("GPM_COOKIE_SECURE=0 while settings.externalBaseURL is https: admin session cookies are sent without the Secure flag and can leak over plain HTTP; unset GPM_COOKIE_SECURE unless a plain-HTTP admin listener is intentional")
	}

	// reload re-reads the config and applies it to both the auth layer and the
	// data plane. It is the single path used after any config or certificate
	// change (API writes, ACME issuance) so the running state never drifts.
	reload := func() error {
		c, st2, err := st.Load(ctx)
		if err != nil {
			log.Error().Err(err).Msg("reload: failed to load config")
			return err
		}
		// Inbound PROXY protocol first: it governs which address the data plane
		// treats as the client, and it is read live per connection, so applying it
		// ahead of the compile means any listener the compile opens is already
		// wrapped.
		dp.SetProxyProtocol(st2.ProxyProtocol)
		// Same reasoning as the initial load: compile the settings-level error
		// pages before the data plane, so a template that fails to parse fails
		// the whole reload with a clear message rather than serving half of a
		// changed config.
		if err := dataplane.SetErrorPages(st2.ErrorPages, *certDir); err != nil {
			log.Error().Err(err).Msg("reload: failed to compile settings-level error pages")
			return err
		}
		// Settings-level default security headers, refreshed before the compile so
		// buildRouter composes each host's set from the new default.
		dataplane.SetSecurityHeaders(st2.SecurityHeaders)
		dataplane.SetStripResponseHeaders(st2.StripResponseHeaders)
		// Fleet-wide maintenance, applied BEFORE the compile like every other
		// settings-level switch above: turning it on takes the edge out of service
		// on the very next request rather than after the router build. If the
		// compile then fails, the hosts still running the previous router honour
		// the new switch - which is the reading an operator flipping it wants.
		dataplane.SetMaintenance(st2.Maintenance)
		// Fetched access-list source sets, refreshed before the compile so a
		// reload triggered by a completed fetch serves the new set on the very
		// next request (see accesssync).
		if l, _, lerr := st.LoadAccessListSourceLedger(ctx); lerr != nil {
			log.Error().Err(lerr).Msg("reload: failed to load the access-list source ledger; source-backed rules match nothing")
		} else {
			dataplane.SetAccessListSources(l)
		}
		// Reload the data plane FIRST: only reconfigure the auth layer once the
		// data plane has accepted the new config, so a rejected reload never
		// leaves auth and dataplane drifted against each other.
		if err := dp.Reload(c); err != nil {
			log.Error().Err(err).Msg("reload: failed to reload data plane")
			return err
		}
		authn.Configure(c, st2)
		// Every config change funnels through here, so this is the one place the
		// cached API-token set has to be dropped.
		authn.InvalidateTokenCache()
		return nil
	}

	// ACME manager: issues/renews DNS-01 certs and reloads on certificate change.
	// The reload error is already logged; ACME has no caller to surface it to.
	// Single-writer ACME: only the leader renews. Two instances running this loop
	// would race the same order (duplicate issuance, wasted rate limit, divergent
	// keypairs); the follower serves the certs replicated into the shared cert dir.
	if role.IsFollower() {
		log.Info().Msg("HA follower: ACME renewal loop disabled (the leader is the only issuer)")
	} else {
		acmeMgr := acme.NewManager(acme.Options{CertDir: *certDir, OnChange: func() { _ = reload() }})
		if mx != nil {
			// Leader-only on purpose: a follower is not the issuer, and exporting a
			// zero expiry there would read as "expired" to any sane alert.
			mx.RegisterACME(func() []metrics.CertStatus {
				obs := acmeMgr.CertObservations()
				out := make([]metrics.CertStatus, 0, len(obs))
				for _, o := range obs {
					out = append(out, metrics.CertStatus{Name: o.Name, NotAfter: o.NotAfter, RenewFailures: o.RenewFailures})
				}
				return out
			})
		}
		// HTTP-01 challenges are answered by the data plane's plaintext listener
		// from the manager's in-flight token map.
		dp.SetACMEChallengeStore(acmeMgr.HTTP01Challenges())
		go acmeMgr.Run(ctx, 0, func(ctx context.Context) (model.Config, error) {
			c, _, err := st.Load(ctx)
			return c, err
		})
	}

	// Follower config replication: fast-forward the leader's repo on a poll and
	// reload only when HEAD moved. The follower never commits, so the repo cannot
	// diverge; a pull that is not a clean fast-forward is logged, never merged.
	if role.IsFollower() {
		go st.FollowRemote(ctx, *haPollInt, reload)
	}

	// Lifecycle webhooks: fire-and-forget notifications after each config change,
	// reading the live targets from settings on every event.
	hooks := webhook.New(func() []model.WebhookConfig {
		_, s2, err := st.Load(ctx)
		if err != nil {
			return nil
		}
		return s2.Webhooks
	})

	// DNS sync: publishes CNAMEs for proxy hosts that opted in, into the local
	// resolver (Pi-hole) and/or the public zone (Cloudflare). Like the webhook
	// dispatcher it reads live config on every run, so nothing needs re-wiring
	// when settings change.
	// The ownership ledger is what authorises a DNS deletion: gpm only ever removes
	// records this file says it created. It is committed to the config repo like
	// everything else, authored as the reconciler rather than as an operator.
	dnsSyncer := dnssync.New(func(c context.Context) (model.Config, model.Settings, error) {
		return st.Load(c)
	}, dnsLedgerStore{st})

	if mx != nil {
		mx.RegisterDNSSync(func() metrics.DNSSyncStatus {
			st := dnsSyncer.Status()
			return metrics.DNSSyncStatus{
				LastRun:     st.LastRun,
				LastSuccess: st.LastSuccess,
				Backends: []metrics.DNSBackendStatus{
					{Name: "pihole", Enabled: st.Pihole.Enabled, OK: st.Pihole.OK, Desired: st.Pihole.Desired, Managed: st.Pihole.Managed},
					{Name: "cloudflare", Enabled: st.Cloudflare.Enabled, OK: st.Cloudflare.OK, Desired: st.Cloudflare.Desired, Managed: st.Cloudflare.Managed},
				},
			}
		})
	}

	// Kubernetes Ingress discovery: derives managed proxy hosts from annotated
	// cluster Ingresses (read-only against the cluster) and writes them as ONE
	// commit per reconcile. It publishes no DNS itself - after a write it asks the
	// phase-1 syncer for a run, so there is a single DNS code path.
	ingressDisc := k8s.New(
		func(c context.Context) (model.Config, model.Settings, error) { return st.Load(c) },
		func(c context.Context, upserts []model.ProxyHost, deletes []string, message string) (string, error) {
			objs := make([]model.Object, 0, len(upserts))
			for _, h := range upserts {
				objs = append(objs, h)
			}
			refs := make([]store.ObjectRef, 0, len(deletes))
			for _, name := range deletes {
				refs = append(refs, store.ObjectRef{Kind: "ProxyHost", Name: name})
			}
			// The reconciler's plan is made from a snapshot taken before a
			// multi-second cluster list, so ownership is re-checked here under the
			// store lock: anything that stopped being a discovery-owned object in
			// the meantime is left alone rather than overwritten or deleted.
			guard := func(existing model.Object) error {
				if existing.GetMeta().Labels[model.ManagedByLabel] != model.ManagedByIngressDiscovery {
					return fmt.Errorf("%s %q is not labelled %s=%s", existing.Kind(), existing.GetMeta().Name,
						model.ManagedByLabel, model.ManagedByIngressDiscovery)
				}
				return nil
			}
			return st.ApplyBatch(c, objs, refs, message, store.Author{Name: "ingress-discovery", Email: "gpm@localhost"}, guard)
		},
		func(commit string) {
			if err := reload(); err != nil {
				log.Error().Err(err).Msg("ingress discovery: config written but reload failed")
			}
			hooks.Dispatch(webhook.Event{Action: "ingress-discovery", Kind: "ProxyHost", Commit: commit})
			// Reuse the phase-1 trigger: it is non-blocking and coalescing, so the
			// derived hosts' dns policies are published by the ONE DNS reconciler.
			dnsSyncer.Trigger()
		},
	)
	if mx != nil {
		mx.RegisterIngressDiscovery(func() metrics.IngressStatus {
			st := ingressDisc.Status()
			return metrics.IngressStatus{
				Enabled:     st.Enabled,
				LastRun:     st.LastRun,
				LastSuccess: st.LastSuccess,
				Discovered:  st.Discovered,
				Managed:     st.Managed,
			}
		})
	}

	// Access-list source sync: keeps the remote IP feeds an AccessList references
	// (rule.source) fetched into the committed ledger, so a path-scoped allow rule
	// for a monitoring provider stays current without the operator re-pasting a
	// couple of hundred CIDRs. Like the reconcilers above it reads live config on
	// every run; after a ledger commit it reloads the data plane so the new set is
	// served immediately.
	accessSyncer := accesssync.New(func(c context.Context) (model.Config, model.Settings, error) {
		return st.Load(c)
	}, accessLedgerStore{st}, func() {
		if err := reload(); err != nil {
			log.Error().Err(err).Msg("accesssync: ledger written but reload failed")
		}
	})

	// Both reconcilers are tracked, not fire-and-forget: a run that is mid-commit
	// when the process is asked to stop has to be allowed to finish (or roll back)
	// before main returns, or shutdown is exactly the window that leaves the config
	// repo written but uncommitted. The access-list fetcher commits the source
	// ledger, so it needs the same treatment.
	// Leader-only, both: they commit to the config repo, and a follower that
	// commits locally would diverge from the leader and break its ff-only pull. A
	// follower still SERVES the fetched sets, since it replicates the ledger file
	// with the rest of the config.
	var discWG sync.WaitGroup
	if !role.IsFollower() {
		discWG.Add(2)
		go func() {
			defer discWG.Done()
			ingressDisc.Run(ctx)
		}()
		go func() {
			defer discWG.Done()
			accessSyncer.Run(ctx)
		}()
	} else {
		log.Info().Msg("HA follower: Ingress discovery reconciler and access-list source sync disabled (leader-only writers)")
	}

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
			// Anything that can change a host's domains or its dns policy - a
			// proxy-host write, a settings change, or a whole-tree
			// restore/revert - asks the DNS reconciler for a run. Trigger is
			// non-blocking and coalescing, so a bulk change costs one reconcile.
			switch {
			case kind == "ProxyHost", kind == "Settings", action == "restore", action == "revert":
				dnsSyncer.Trigger()
			}
			// A new or re-pointed source has to be fetched before its rules mean
			// anything, so an access-list write asks for a run rather than waiting
			// out the poll interval. Trigger is non-blocking and coalescing.
			switch {
			case kind == "AccessList", action == "restore", action == "revert":
				accessSyncer.Trigger()
			}
		},
		RecentLogs: func() any {
			return map[string]any{"enabled": dp.AccessLogEnabled(), "entries": dp.RecentLogs()}
		},
		SetAccessLog:      dp.SetAccessLog,
		GeoDBLoaded:       geoResolver.Loaded,
		MaintenanceGlobal: dataplane.MaintenanceGlobalEnabled,
		UpstreamHealth: func() any {
			return dp.UpstreamHealth()
		},
		RevokeSSOSessions: dataplane.RevokeAllSSOSessions,
		RequireScope:      requireScope,
		TokenLastUsed:     authn.TokenLastUsed,
		DNSSyncReconcile:  dnsSyncer.ReconcileNow,
		DNSSyncStatus:     func() any { return dnsSyncer.Status() },
		DNSSyncPlan: func(c context.Context) (any, error) {
			return dnsSyncer.Plan(c)
		},
		DNSSyncEnabled: dnsSyncer.Enabled,

		IngressDiscoveryReconcile: ingressDisc.ReconcileNow,
		IngressDiscoveryStatus:    func() any { return ingressDisc.Status() },
		IngressDiscoveryPlan: func(c context.Context) (any, error) {
			return ingressDisc.Plan(c)
		},
		IngressDiscoveryEnabled: ingressDisc.Enabled,

		AccessListSourceReconcile: accessSyncer.ReconcileNow,
		AccessListSourceStatus:    func() any { return accessSyncer.Status() },

		// Reported so the SPA can grey out the metrics link instead of pointing at
		// a route that answers 404. The endpoint itself is on the admin mux, not
		// under /api/.
		MetricsEnabled: mx != nil,

		// A follower serves the admin API read-only: writes are refused with a
		// 503 naming the leader, and the SPA greys the controls out.
		Role: role,

		// Resolves a ClientCA's cert-store-relative caKeyFile when issuing a
		// client certificate.
		CertDir: *certDir,
	})

	uiHandler, err := ui.Handler()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialise admin UI")
	}

	var metricsHandler http.Handler
	if mx != nil {
		metricsHandler = mx.Handler()
	}
	admin := server.New(*adminAddr, st, authn, apiHandler, uiHandler, metricsHandler, *pprofEnabled)

	errc := make(chan error, 2)
	go func() { errc <- admin.Start(ctx) }()
	go func() { errc <- dp.Start(ctx) }()

	if err := <-errc; err != nil {
		log.Error().Err(err).Msg("server error, shutting down")
		stop() // cancel ctx so the sibling server shuts down too
	}
	<-errc
	discWG.Wait()
	log.Info().Msg("shutdown complete")
}

// requireScope is the API's scope gate. A session principal is never constrained
// (auth.RequireScope returns nil for one); only API tokens are.
//
// No principal at all DENIES. The API mux is mounted behind RequireRole, so a
// request can only arrive here with a principal attached - reaching this branch
// means the wiring is broken, and the safe answer to a broken authorization
// check is to refuse, not to wave the request through.
func requireScope(r *http.Request, required string) error {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		return fmt.Errorf("no authenticated principal on the request (refusing %q)", required)
	}
	return auth.RequireScope(p, required)
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

// dnsLedgerStore adapts the config store to the dnssync.Ledger interface. It
// exists so the DNS reconciler depends on the two ledger operations it needs
// rather than on the whole store, and so the ledger commit is authored as the
// reconciler instead of as whichever operator happened to trigger the run.
type dnsLedgerStore struct{ st *store.Store }

func (d dnsLedgerStore) Load(ctx context.Context) (model.DNSLedger, string, error) {
	return d.st.LoadDNSLedger(ctx)
}

// Save detaches the write from the caller's context. A reconcile can be driven by
// an HTTP request, and the client hanging up must not cancel the commit half way:
// SaveDNSLedger writes the file and then commits it, so a cancellation between
// the two leaves the ledger on disk but out of git, to be swept into whatever
// unrelated commit lands next. The write is short, local and already ordered
// after the DNS changes it records, so seeing it through is strictly safer than
// abandoning it.
func (d dnsLedgerStore) Save(ctx context.Context, l model.DNSLedger, rev string) error {
	_, err := d.st.SaveDNSLedger(context.WithoutCancel(ctx), l, store.Author{Name: "dns-sync", Email: "gpm@localhost"}, rev)
	return err
}

// accessLedgerStore adapts the config store to the accesssync.Ledger interface,
// for the same reasons dnsLedgerStore does: the fetcher depends on the two
// operations it needs rather than on the whole store, and the commit is authored
// as the fetcher instead of as whichever operator happened to trigger the run.
type accessLedgerStore struct{ st *store.Store }

func (a accessLedgerStore) Load(ctx context.Context) (model.AccessListSourceLedger, string, error) {
	return a.st.LoadAccessListSourceLedger(ctx)
}

// Save detaches the write from the caller's context for the same reason
// dnsLedgerStore.Save does: an HTTP-triggered run whose client hangs up must not
// leave the ledger written to disk but out of git.
func (a accessLedgerStore) Save(ctx context.Context, l model.AccessListSourceLedger, rev string) error {
	_, err := a.st.SaveAccessListSourceLedger(context.WithoutCancel(ctx), l,
		store.Author{Name: "access-list-sync", Email: "gpm@localhost"}, rev)
	return err
}
