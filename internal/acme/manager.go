package acme

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	renewCooldown       time.Duration
	newSolver           func(model.DNSProvider) (DNSSolver, error)
	onChange            func()
	onRenewFailure      func(certName string, err error)

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

	// lastRun is the unix-nano timestamp of the last EnsureAll pass (the
	// periodic renewal cycle), for the GET /health probe. Zero until the first
	// pass runs. An atomic rather than obsMu since it is set once per pass, not
	// per certificate.
	lastRun atomic.Int64
}

// LastRun returns when EnsureAll last ran. Zero means it has never run (a
// follower, which does not run the renewal loop, or before the first tick).
func (m *Manager) LastRun() time.Time {
	ns := m.lastRun.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}

// CertObservation is one ACME certificate's observable state: when the issued
// artifact expires, how many issue/renew attempts have failed since this
// process started, and the most recent attempt's outcome. All of it is read at
// scrape time by the /metrics collector and by the GET /certificates status
// surface, which is why it lives here rather than being pushed into a metrics
// package the acme package would then have to depend on.
type CertObservation struct {
	Name          string
	NotAfter      time.Time
	RenewFailures int64
	// LastAttempt is when this certificate's last issue/renew attempt started
	// (successful or not). Zero means no attempt has run since this process
	// started.
	LastAttempt time.Time
	// LastError is the most recent issue/renew attempt's failure message,
	// cleared on the next successful attempt. Empty means the last attempt (if
	// any) succeeded.
	LastError string
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

// recordFailure counts one failed issue/renew attempt for a certificate and
// remembers its error for the status surface. It is the single hook point for
// the optional notify.EventCertRenewalFailed alert: every renewOne error path
// (missing provider, solver build, EAB, account/directory client, the order
// itself) routes through here, so wiring one callback covers all of them.
func (m *Manager) recordFailure(name string, err error) {
	m.obsMu.Lock()
	o := m.observation(name)
	o.RenewFailures++
	o.LastError = err.Error()
	m.obsMu.Unlock()

	if m.onRenewFailure != nil {
		m.onRenewFailure(name, err)
	}
}

// recordAttempt notes that an issue/renew attempt for a certificate is starting
// now, before its outcome is known.
func (m *Manager) recordAttempt(name string) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	m.observation(name).LastAttempt = time.Now().UTC()
}

// recordSuccess clears a certificate's last-error state after a successful
// issue/renew attempt.
func (m *Manager) recordSuccess(name string) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	m.observation(name).LastError = ""
}

// Options configures a Manager. Zero values fall back to sane defaults.
type Options struct {
	CertDir             string
	RenewBefore         time.Duration
	PropagationTimeout  time.Duration
	PropagationInterval time.Duration
	// RenewCooldown overrides defaultRenewCooldown (1h), the minimum interval
	// RenewNow enforces between two attempts for the same certificate. Zero
	// means the default; tests use a short value so repeated calls are not
	// throttled.
	RenewCooldown time.Duration
	Resolver      *net.Resolver
	// NewSolver builds a DNSSolver for a provider; overridable in tests.
	NewSolver func(model.DNSProvider) (DNSSolver, error)
	// OnChange is invoked after any certificate is issued or renewed so the
	// caller can reload the data plane's cert set.
	OnChange func()
	// OnRenewFailure, if set, is invoked after every failed issue/renew attempt
	// (see recordFailure) for optional alerting (internal/notify). Best-effort:
	// never blocks or affects the renewal outcome.
	OnRenewFailure func(certName string, err error)
}

// NewManager constructs a Manager.
func NewManager(o Options) *Manager {
	m := &Manager{
		certDir:             o.CertDir,
		renewBefore:         o.RenewBefore,
		propagationTimeout:  o.PropagationTimeout,
		propagationInterval: o.PropagationInterval,
		renewCooldown:       o.RenewCooldown,
		newSolver:           o.NewSolver,
		onChange:            o.OnChange,
		onRenewFailure:      o.OnRenewFailure,
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
	if m.renewCooldown == 0 {
		m.renewCooldown = defaultRenewCooldown
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
	m.lastRun.Store(time.Now().UTC().UnixNano())

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
		if err := m.renewOne(ctx, providers, cert); err != nil {
			errs = append(errs, err)
			continue
		}
		changed = true
	}
	if changed && m.onChange != nil {
		m.onChange()
	}
	return changed, errors.Join(errs...)
}

// ErrCertNotFound, ErrNotACME, ErrRenewInFlight and ErrRenewCooldown are the
// sentinel errors RenewNow returns, for POST /certificates/{name}/renew to map
// onto an HTTP status without string-matching an error message.
var (
	ErrCertNotFound  = errors.New("certificate not found")
	ErrNotACME       = errors.New("certificate is not an acme certificate")
	ErrRenewInFlight = errors.New("an order is already in progress")
	// ErrRenewCooldown is returned by RenewNow when the certificate's last
	// issue/renew attempt (successful or not) started less than renewCooldown
	// ago. See renewCooldownRemaining.
	ErrRenewCooldown = errors.New("renew is on cooldown")
)

// defaultRenewCooldown is the minimum time RenewNow requires between two
// attempts for the same certificate, absent an Options.RenewCooldown override.
// EnsureAll's periodic loop is unaffected - it is already gated by
// needsRenewal's 30-day window - but RenewNow exists specifically to bypass
// that window on demand, and certificates:write is deliberately narrower than
// admin. Without its own throttle, a caller holding only that scope could
// script repeated renews and exhaust the ACME directory's duplicate-
// certificate limit (Let's Encrypt: 5 per exact SAN set per week), taking the
// operator's real renewal loop offline for the rest of the window. A failed
// attempt counts the same as a successful one, since repeated failures trip
// the CA's failed-validation limit too.
const defaultRenewCooldown = time.Hour

// renewCooldownRemaining returns how much longer RenewNow must wait before
// name's cooldown elapses, or zero if it may proceed now.
func (m *Manager) renewCooldownRemaining(name string) time.Duration {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	o := m.obs[name]
	if o == nil || o.LastAttempt.IsZero() {
		return 0
	}
	if elapsed := time.Since(o.LastAttempt); elapsed < m.renewCooldown {
		return m.renewCooldown - elapsed
	}
	return 0
}

// RenewNow validates the request and, if accepted, starts an immediate
// issue/renew of one ACME certificate in the background, ignoring the renewal
// window EnsureAll otherwise honours. It returns as soon as the order has
// started (or is refused) rather than once the order completes: DNS-01
// propagation alone can take minutes, far longer than an HTTP request handler
// should block. It shares renewOne with EnsureAll so there is exactly one
// ordering code path.
//
// Only one order runs at a time (m.mu, the same lock EnsureAll holds for its
// whole pass): a renew requested while any order is already in flight is
// refused with ErrRenewInFlight rather than queued, so the caller can report it
// as a 409 immediately instead of blocking the request until the other order
// finishes. A renew requested within renewCooldown of the certificate's last
// attempt is likewise refused, with ErrRenewCooldown, before the lock is even
// attempted - see renewCooldownRemaining.
func (m *Manager) RenewNow(ctx context.Context, cfg model.Config, name string) error {
	var cert *model.Certificate
	for i := range cfg.Certificates {
		if cfg.Certificates[i].Name == name {
			cert = &cfg.Certificates[i]
			break
		}
	}
	if cert == nil {
		return ErrCertNotFound
	}
	if cert.Type != model.CertTypeACME || cert.ACME == nil {
		return ErrNotACME
	}
	if remaining := m.renewCooldownRemaining(name); remaining > 0 {
		return fmt.Errorf("%w: retry in %s", ErrRenewCooldown, remaining.Round(time.Second))
	}
	if !m.mu.TryLock() {
		return ErrRenewInFlight
	}

	providers := map[string]model.DNSProvider{}
	for _, p := range cfg.DNSProviders {
		providers[p.Name] = p
	}
	target := *cert
	go func() {
		defer m.mu.Unlock()
		// A fresh, independent context: the HTTP request that accepted this order
		// has already returned by the time a multi-minute DNS-01 propagation wait
		// or a slow CA finishes, so it must not be the one that can cancel it.
		if err := m.renewOne(context.Background(), providers, target); err == nil && m.onChange != nil {
			m.onChange()
		}
	}()
	return nil
}

// renewOne runs one certificate's full issue/renew order: it builds the dns-01
// solver (or none, for http-01), constructs the ACME client, and calls issue.
// Callers must hold m.mu. Every outcome is recorded on the manager's
// observation for that certificate (attempt time, failure, or the new expiry).
func (m *Manager) renewOne(ctx context.Context, providers map[string]model.DNSProvider, cert model.Certificate) error {
	m.recordAttempt(cert.Name)

	// http-01 needs no provider at all; dns-01 (the default whenever a provider
	// is referenced) resolves and builds its solver here.
	var solver DNSSolver
	if cert.ACME.EffectiveChallenge() == model.ChallengeDNS01 {
		provider, ok := providers[cert.ACME.DNSProvider]
		if !ok {
			err := fmt.Errorf("certificate %q: dns provider %q not found", cert.Name, cert.ACME.DNSProvider)
			m.recordFailure(cert.Name, err)
			return err
		}
		s, err := m.newSolver(provider)
		if err != nil {
			werr := fmt.Errorf("certificate %q: build solver: %w", cert.Name, err)
			m.recordFailure(cert.Name, werr)
			return werr
		}
		solver = s
	}
	eab, err := externalAccountBinding(cert.ACME.EAB)
	if err != nil {
		werr := fmt.Errorf("certificate %q: %w", cert.Name, err)
		m.recordFailure(cert.Name, werr)
		return werr
	}
	client, err := newClient(ctx, m.certDir, cert.ACME.DirectoryURL, cert.ACME.Email, eab)
	if err != nil {
		werr := fmt.Errorf("certificate %q: %w", cert.Name, err)
		m.recordFailure(cert.Name, werr)
		return werr
	}
	log.Info().Str("cert", cert.Name).Str("challenge", cert.ACME.EffectiveChallenge()).Msg("issuing/renewing certificate")
	if err := m.issue(ctx, client, cert, solver); err != nil {
		werr := fmt.Errorf("certificate %q: %w", cert.Name, err)
		m.recordFailure(cert.Name, werr)
		return werr
	}
	// Re-read the freshly issued artifact so the expiry gauge reflects the new
	// certificate rather than the one it replaced.
	if meta, err := loadMeta(m.certDir, cert.Name); err == nil {
		m.recordExpiry(cert.Name, meta.NotAfter)
	}
	m.recordSuccess(cert.Name)
	return nil
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

// defaultNewSolver maps a DNSProvider config to a concrete solver. The REST
// providers authenticate with a single token under config.apiToken; rfc2136 and
// acme-dns read their own key sets out of the same Config map.
func defaultNewSolver(p model.DNSProvider) (DNSSolver, error) {
	switch p.Provider {
	case model.DNSProviderRFC2136:
		return newRFC2136FromConfig(p)
	case model.DNSProviderACMEDNS:
		return newACMEDNSFromConfig(p)
	}
	token, err := p.Config["apiToken"].Resolve()
	if err != nil {
		return nil, fmt.Errorf("%s apiToken: %w", p.Provider, err)
	}
	switch p.Provider {
	case model.DNSProviderCloudflare:
		return NewCloudflareSolver(token)
	case model.DNSProviderDigitalOcean:
		return NewDigitalOceanSolver(token)
	case model.DNSProviderHetzner:
		return NewHetznerSolver(token)
	case model.DNSProviderDesec:
		return NewDesecSolver(token)
	default:
		return nil, fmt.Errorf("unsupported dns provider %q", p.Provider)
	}
}

// resolveConfig resolves one Config key, turning a ${ENV:...}/${FILE:...}
// placeholder into its value and naming the key in any error.
func resolveConfig(p model.DNSProvider, key string) (string, error) {
	v, err := p.Config[key].Resolve()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", p.Provider, key, err)
	}
	return strings.TrimSpace(v), nil
}

// newRFC2136FromConfig builds an RFC 2136 solver from the config map. Optional
// numeric/duration keys are parsed here so a bad value fails at solver
// construction with the key name, not at UPDATE time.
func newRFC2136FromConfig(p model.DNSProvider) (DNSSolver, error) {
	get := func(key string) (string, error) { return resolveConfig(p, key) }

	var (
		c   RFC2136Config
		err error
	)
	if c.Server, err = get("server"); err != nil {
		return nil, err
	}
	if c.Zone, err = get("zone"); err != nil {
		return nil, err
	}
	if c.KeyName, err = get("tsigKeyName"); err != nil {
		return nil, err
	}
	if c.Secret, err = get("tsigSecret"); err != nil {
		return nil, err
	}
	if c.Algorithm, err = get("tsigAlgorithm"); err != nil {
		return nil, err
	}
	if c.Transport, err = get("transport"); err != nil {
		return nil, err
	}
	ttl, err := get("ttl")
	if err != nil {
		return nil, err
	}
	if ttl != "" {
		if c.TTL, err = strconv.Atoi(ttl); err != nil {
			return nil, fmt.Errorf("rfc2136 ttl: %q is not a whole number of seconds", ttl)
		}
	}
	timeout, err := get("timeout")
	if err != nil {
		return nil, err
	}
	if timeout != "" {
		if c.Timeout, err = time.ParseDuration(timeout); err != nil {
			return nil, fmt.Errorf("rfc2136 timeout: %q is not a Go duration such as 30s", timeout)
		}
	}
	return NewRFC2136Solver(c)
}

// newACMEDNSFromConfig builds an acme-dns solver from the config map.
func newACMEDNSFromConfig(p model.DNSProvider) (DNSSolver, error) {
	var (
		c   ACMEDNSConfig
		err error
	)
	if c.BaseURL, err = resolveConfig(p, "baseURL"); err != nil {
		return nil, err
	}
	if c.Username, err = resolveConfig(p, "username"); err != nil {
		return nil, err
	}
	if c.Password, err = resolveConfig(p, "password"); err != nil {
		return nil, err
	}
	if c.Subdomain, err = resolveConfig(p, "subdomain"); err != nil {
		return nil, err
	}
	c.AllowInsecureLocal = model.IsTruthyConfig(string(p.Config["allowInsecureLocal"]))
	return NewACMEDNSSolver(c)
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
