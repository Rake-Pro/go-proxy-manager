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
	"os"
	"os/signal"
	"syscall"

	"github.com/Rake-Pro/go-proxy-manager/internal/logging"
	"github.com/Rake-Pro/go-proxy-manager/internal/server"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	"github.com/Rake-Pro/go-proxy-manager/internal/version"
	"github.com/rs/zerolog/log"
)

func main() {
	var (
		configDir  = flag.String("config-dir", envOr("GPM_CONFIG_DIR", "/data/config"), "git-backed config repository directory")
		adminAddr  = flag.String("admin-addr", envOr("GPM_ADMIN_ADDR", ":8081"), "admin HTTP listen address")
		logLevel   = flag.String("log-level", envOr("GPM_LOG_LEVEL", "info"), "log level (trace|debug|info|warn|error)")
		logConsole = flag.Bool("log-console", os.Getenv("GPM_LOG_CONSOLE") == "1", "human-friendly console logging")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println(version.String())
		return
	}

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

	srv := server.New(*adminAddr, st)
	if err := srv.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("admin server error")
	}
	log.Info().Msg("shutdown complete")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
