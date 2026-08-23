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
	resolver            txtLookuper
	renewBefore         time.Duration
	propagationTimeout  time.Duration
	propagationInterval time.Duration
	newSolver           func(model.DNSProvider) (DNSSolver, error)
	onChange            func()

	// http01 holds the key authorizations of in-flight HTTP-01 orders; the data
	// plane's plaintext listener serves them (see HTTP01Store).
	http01 *HTTP01Store

	mu sync.Mutex // serialises issuance (one order at a time)

	// obsMu guards obs, the per-certificate expiry and failure counts the
	// optional /metrics endpoint pulls at scrape time. It is a separate lock
	// from mu so a scrape never waits behind a multi-minute DNS-01 propagation
	// wait.
	obsMu sync.Mutex
	obs   map[string]*CertObservation
}

// CertObservation is one ACME certificate's observable state: when the issued
// artifact expires, and how many issue/renew attempts have failed since this
// process started. Both are read at scrape time by the /metrics collector, which
// is why they live here rather than being pushed into a metrics package the
// acme package would then have to depend on.
type CertObservation struct {
	Name          string
	NotAfter      time.Time
	RenewFailures int64
}

// CertObservations returns a snapshot of every ACME certificate this manager has
// looked at, for the /metrics collector. Only the HA leader runs the manager, so
// a follower exports no ACME series at all - which is correct: it is not the
// issuer, and a zero there would read as "expired".
func (m *Manager) CertObservations() []CertObservation {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	out := make([]CertObservation, 0, len(m.obs))
	for _, o := range m.obs {
		out = append(out, *o)
	}
	return out
}

func (m *Manager) observation(name string) *CertObservation {
	o := m.obs[name]
	if o == nil {
		o = &CertObservation{Name: name}
		m.obs[name] = o
	}
	return o
}

// recordExpiry notes a certificate's current expiry, whether or not this cycle
// renewed it, so the gauge is populated for healthy certificates too.
func (m *Manager) recordExpiry(name string, notAfter time.Time) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	m.observation(name).NotAfter = notAfter
}

// recordFailure counts one failed issue/renew attempt for a certificate.
func (m *Manager) recordFailure(name string) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	m.observation(name).RenewFailures++
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
		renewBefore:         o.RenewBefore,
		propagationTimeout:  o.PropagationTimeout,
		propagationInterval: o.PropagationInterval,
		newSolver:           o.NewSolver,
		onChange:            o.OnChange,
		http01:              NewHTTP01Store(),
		obs:                 map[string]*CertObservation{},
	}
	if o.Resolver != nil {
		m.resolver = o.Resolver
	} else {
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

// HTTP01Challenges exposes the in-flight HTTP-01 tokens so the data plane can
// answer /.well-known/acme-challenge/<token> on the plaintext listener.
func (m *Manager) HTTP01Challenges() *HTTP01Store { return m.http01 }

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
		// Record the current expiry for EVERY acme certificate, renewed or not -
		// the gauge is about how close a cert is to expiring, so the healthy ones
		// are the majority of what it has to report.
		if meta, err := loadMeta(m.certDir, cert.Name); err == nil {
			m.recordExpiry(cert.Name, meta.NotAfter)
		}
		if !m.needsRenewal(cert) {
			continue
		}
		// http-01 needs no provider at all; dns-01 (the default whenever a
		// provider is referenced) resolves and builds its solver here.
		var solver DNSSolver
		if cert.ACME.EffectiveChallenge() == model.ChallengeDNS01 {
			provider, ok := providers[cert.ACME.DNSProvider]
			if !ok {
				errs = append(errs, fmt.Errorf("certificate %q: dns provider %q not found", cert.Name, cert.ACME.DNSProvider))
				m.recordFailure(cert.Name)
				continue
			}
			s, err := m.newSolver(provider)
			if err != nil {
				errs = append(errs, fmt.Errorf("certificate %q: build solver: %w", cert.Name, err))
				m.recordFailure(cert.Name)
				continue
			}
			solver = s
		}
		eab, err := externalAccountBinding(cert.ACME.EAB)
		if err != nil {
			errs = append(errs, fmt.Errorf("certificate %q: %w", cert.Name, err))
			m.recordFailure(cert.Name)
			continue
		}
		client, err := newClient(ctx, m.certDir, cert.ACME.DirectoryURL, cert.ACME.Email, eab)
		if err != nil {
			errs = append(errs, fmt.Errorf("certificate %q: %w", cert.Name, err))
			m.recordFailure(cert.Name)
			continue
		}
		log.Info().Str("cert", cert.Name).Str("challenge", cert.ACME.EffectiveChallenge()).Msg("issuing/renewing certificate")
		if err := m.issue(ctx, client, cert, solver); err != nil {
			errs = append(errs, fmt.Errorf("certificate %q: %w", cert.Name, err))
			m.recordFailure(cert.Name)
			continue
		}
		// Re-read the freshly issued artifact so the expiry gauge reflects the new
		// certificate rather than the one it replaced.
		if meta, err := loadMeta(m.certDir, cert.Name); err == nil {
			m.recordExpiry(cert.Name, meta.NotAfter)
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

// defaultNewSolver maps a DNSProvider config to a concrete solver. Every shipped
// provider authenticates with a single token under config.apiToken.
func defaultNewSolver(p model.DNSProvider) (DNSSolver, error) {
	token, err := p.Config["apiToken"].Resolve()
	if err != nil {
		return nil, fmt.Errorf("%s apiToken: %w", p.Provider, err)
	}
	switch p.Provider {
	case "cloudflare":
		return NewCloudflareSolver(token)
	case "digitalocean":
		return NewDigitalOceanSolver(token)
	case "hetzner":
		return NewHetznerSolver(token)
	case "desec":
		return NewDesecSolver(token)
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
