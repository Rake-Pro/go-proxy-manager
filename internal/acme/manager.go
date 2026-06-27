package acme

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// Manager issues and renews the ACME certificates declared in the config and
// keeps the issued artifacts on disk for the data plane to load.
type Manager struct {
	certDir             string
	resolver            *net.Resolver
	renewBefore         time.Duration
	propagationTimeout  time.Duration
	propagationInterval time.Duration
	newSolver           func(model.DNSProvider) (DNSSolver, error)
	onChange            func()

	mu sync.Mutex // serialises issuance (one order at a time)
}

// Options configures a Manager. Zero values fall back to sane defaults.
type Options struct {
	CertDir             string
	RenewBefore         time.Duration
	PropagationTimeout  time.Duration
	PropagationInterval time.Duration
	Resolver            *net.Resolver
	// NewSolver builds a DNSSolver for a provider; overridable in tests.
	NewSolver func(model.DNSProvider) (DNSSolver, error)
	// OnChange is invoked after any certificate is issued or renewed so the
	// caller can reload the data plane's cert set.
	OnChange func()
}

// NewManager constructs a Manager.
func NewManager(o Options) *Manager {
	m := &Manager{
		certDir:             o.CertDir,
		resolver:            o.Resolver,
		renewBefore:         o.RenewBefore,
		propagationTimeout:  o.PropagationTimeout,
		propagationInterval: o.PropagationInterval,
		newSolver:           o.NewSolver,
		onChange:            o.OnChange,
	}
	if m.resolver == nil {
		m.resolver = publicResolver()
	}
	if m.renewBefore == 0 {
		m.renewBefore = 30 * 24 * time.Hour
	}
	if m.propagationTimeout == 0 {
		m.propagationTimeout = 3 * time.Minute
	}
	if m.propagationInterval == 0 {
		m.propagationInterval = 5 * time.Second
	}
	if m.newSolver == nil {
		m.newSolver = defaultNewSolver
	}
	return m
}

// EnsureAll issues or renews every ACME certificate that needs it. A failure on
// one certificate is logged and does not block the others. Returns whether any
// certificate changed.
func (m *Manager) EnsureAll(ctx context.Context, cfg model.Config) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	providers := map[string]model.DNSProvider{}
	for _, p := range cfg.DNSProviders {
		providers[p.Name] = p
	}

	var changed bool
	var errs []error
	for _, cert := range cfg.Certificates {
		if cert.Disabled || cert.Type != model.CertTypeACME || cert.ACME == nil {
			continue
		}
		if !m.needsRenewal(cert) {
			continue
		}
		provider, ok := providers[cert.ACME.DNSProvider]
		if !ok {
			errs = append(errs, fmt.Errorf("certificate %q: dns provider %q not found", cert.Name, cert.ACME.DNSProvider))
			continue
		}
		solver, err := m.newSolver(provider)
		if err != nil {
			errs = append(errs, fmt.Errorf("certificate %q: build solver: %w", cert.Name, err))
			continue
		}
		client, err := newClient(ctx, m.certDir, cert.ACME.DirectoryURL, cert.ACME.Email)
		if err != nil {
			errs = append(errs, fmt.Errorf("certificate %q: %w", cert.Name, err))
			continue
		}
		log.Info().Str("cert", cert.Name).Msg("issuing/renewing certificate")
		if err := m.issue(ctx, client, cert, solver); err != nil {
			errs = append(errs, fmt.Errorf("certificate %q: %w", cert.Name, err))
			continue
		}
		changed = true
	}
	if changed && m.onChange != nil {
		m.onChange()
	}
	return changed, errors.Join(errs...)
}

// needsRenewal reports whether a cert must be (re)issued: never issued, its
// domain set changed, or it is within the renewal window of expiry.
func (m *Manager) needsRenewal(cert model.Certificate) bool {
	meta, err := loadMeta(m.certDir, cert.Name)
	if err != nil {
		return true // not issued yet (or unreadable) -> issue
	}
	if !sameStringSet(meta.Domains, cert.Domains) {
		return true
	}
	return time.Until(meta.NotAfter) < m.renewBefore
}

// Run does an initial EnsureAll then renews on an interval until ctx is done.
// loadConfig fetches the current config each cycle so newly-added certs are
// picked up without a restart.
func (m *Manager) Run(ctx context.Context, interval time.Duration, loadConfig func(context.Context) (model.Config, error)) {
	if interval == 0 {
		interval = 12 * time.Hour
	}
	run := func() {
		cfg, err := loadConfig(ctx)
		if err != nil {
			log.Error().Err(err).Msg("acme: failed to load config for renewal cycle")
			return
		}
		if _, err := m.EnsureAll(ctx, cfg); err != nil {
			log.Error().Err(err).Msg("acme: one or more certificates failed to issue/renew")
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// defaultNewSolver maps a DNSProvider config to a concrete solver.
func defaultNewSolver(p model.DNSProvider) (DNSSolver, error) {
	switch p.Provider {
	case "cloudflare":
		token, err := p.Config["apiToken"].Resolve()
		if err != nil {
			return nil, fmt.Errorf("cloudflare apiToken: %w", err)
		}
		return NewCloudflareSolver(token)
	default:
		return nil, fmt.Errorf("unsupported dns provider %q", p.Provider)
	}
}

// publicResolver queries a public DNS server directly so propagation checks see
// authoritative state, not a stale local cache.
func publicResolver() *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 10 * time.Second}
			return d.DialContext(ctx, network, "1.1.1.1:53")
		},
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
