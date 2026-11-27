package acme

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

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
	// orderBudget, when set, replaces the per-attempt deadline of renewOne
	// (propagationTimeout plus ten minutes); tests shorten it.
	orderBudget time.Duration
	// minWake is the shortest sleep Run takes before an early pass for a
	// certificate entering its renewal window (one minute; tests shorten it).
	minWake time.Duration
	// loadRetry is how long Run waits before another pass after the config
	// could not be loaded (five minutes; tests shorten it).
	loadRetry time.Duration
	// ariTimeout bounds each renewal-information request (ari.go).
	ariTimeout time.Duration
	// now is the manager's clock for renewal and retry decisions; tests
	// replace it.
	now func() time.Time

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
	// RenewAt is when the issued certificate is next due for renewal under the
	// lifetime rule and the CA's renewal information, and RenewReason which
	// rule set it (RenewReasonLifetime, RenewReasonARI, or RenewReasonChanged
	// when the configured domains or profile no longer match the stored
	// certificate, which makes it due at once). Both are zero until the
	// certificate has been issued. A RenewAt in the past means the certificate
	// is due and its renewal has not succeeded yet.
	RenewAt     time.Time
	RenewReason string

	// consecutiveFailures counts the failed attempts since the last success
	// (in memory: a restart starts again at one), for the retry backoff.
	consecutiveFailures int
}

// RenewReason values of CertObservation.
const (
	RenewReasonLifetime = "lifetime"
	RenewReasonARI      = "ari"
	RenewReasonChanged  = "changed"
)

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

// recordIssued notes an issued certificate's current expiry and next renewal
// time, whether or not this cycle renewed it, so the gauge and the status
// surface are populated for healthy certificates too. A certificate whose
// domains or profile changed is due at once, so its renewal time is now.
func (m *Manager) recordIssued(cert model.Certificate, meta *Meta, now time.Time) {
	at, reason := m.renewalTime(meta, m.notBefore(cert.Name, meta))
	if m.orderChanged(cert, meta) {
		at, reason = now.UTC(), RenewReasonChanged
	}
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	o := m.observation(cert.Name)
	o.NotAfter = meta.NotAfter
	o.RenewAt = at
	o.RenewReason = reason
}

// recordFailure counts one failed issue/renew attempt for a certificate and
// remembers its error for the status surface. It is the single hook point for
// the optional notify.EventCertRenewalFailed alert: every renewOne error path
// (missing provider, solver build, EAB, account/directory client, the order
// itself) routes through here, so wiring one callback covers all of them.
func (m *Manager) recordFailure(name string, err error) {
	m.obsMu.Lock()
	msg := truncateError(err.Error())
	o := m.observation(name)
	o.RenewFailures++
	o.consecutiveFailures++
	o.LastError = msg
	m.obsMu.Unlock()

	if m.onRenewFailure != nil {
		if msg != err.Error() {
			err = errors.New(msg)
		}
		m.onRenewFailure(name, err)
	}
}

// maxStoredError bounds the error text kept as LastError and handed to the
// failure notification: a CA's error body becomes part of the error, and its
// size is the CA's choice.
const maxStoredError = 2048

// truncateError cuts s to maxStoredError bytes on a UTF-8 boundary.
func truncateError(s string) string {
	if len(s) <= maxStoredError {
		return s
	}
	cut := maxStoredError
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + " ... (truncated)"
}

// recordAttempt notes that an issue/renew attempt for a certificate is starting
// now, before its outcome is known.
func (m *Manager) recordAttempt(name string) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	m.observation(name).LastAttempt = m.now().UTC()
}

// recordSuccess clears a certificate's last-error state after a successful
// issue/renew attempt.
func (m *Manager) recordSuccess(name string) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	o := m.observation(name)
	o.LastError = ""
	o.consecutiveFailures = 0
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
		minWake:             time.Minute,
		loadRetry:           5 * time.Minute,
		ariTimeout:          ariHTTPTimeout,
		now:                 time.Now,
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
	ari := m.newARIPass()
	for _, cert := range cfg.Certificates {
		if cert.Disabled || cert.Type != model.CertTypeACME || cert.ACME == nil {
			continue
		}
		now := m.now()
		meta, err := loadMeta(m.certDir, cert.Name)
		if err != nil {
			meta = nil // not issued yet (or unreadable) -> issue
		} else {
			// The CA's renewal information first, so a window it moved forward
			// is acted on in this same pass.
			m.pollARI(ctx, ari, cert, meta, now)
			// Record the current expiry for EVERY acme certificate, renewed or
			// not - the gauge is about how close a cert is to expiring, so the
			// healthy ones are the majority of what it has to report.
			m.recordIssued(cert, meta, now)
		}
		if !m.dueNow(cert, meta, now) {
			continue
		}
		// A certificate whose last attempt failed waits out its retry backoff,
		// whatever woke this pass, so failures never come faster than that.
		if at, failed := m.retryAt(cert, meta, now); failed && now.Before(at) {
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
// EnsureAll's periodic loop is unaffected - it is already gated by the
// renewal window - but RenewNow exists specifically to bypass
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
	// A last attempt in the future means the wall clock stepped back since:
	// it counts as long ago, or a wrong clock would block RenewNow for as
	// long as it was wrong.
	if elapsed := m.now().Sub(o.LastAttempt); elapsed >= 0 && elapsed < m.renewCooldown {
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
		// A fresh, independent context: the HTTP request that accepted this order
		// has already returned by the time a multi-minute DNS-01 propagation wait
		// or a slow CA finishes, so it must not be the one that can cancel it.
		// m.mu is released BEFORE onChange fires, so a caller that reacts to the
		// change notification with another RenewNow never sees a stale
		// ErrRenewInFlight from the order that just finished.
		err := func() error {
			defer m.mu.Unlock()
			return m.renewOne(context.Background(), providers, target)
		}()
		if err == nil && m.onChange != nil {
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

	// Every attempt has its own deadline: a CA that leaves an authorization
	// pending (or any other call that never returns) would otherwise hold the
	// issuance lock, and with it every other certificate, for good. The
	// attempt then fails like any other, and the retry backoff applies.
	budget := m.orderBudget
	if budget == 0 {
		budget = m.propagationTimeout + 10*time.Minute
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	// overran names the deadline in the error, so the last error says why.
	overran := func(err error) error {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
			return fmt.Errorf("the attempt did not finish within %s: %w", budget, err)
		}
		return err
	}

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
	httpClient, err := acmeHTTPClientFor(cert.ACME)
	if err != nil {
		werr := fmt.Errorf("certificate %q: %w", cert.Name, err)
		m.recordFailure(cert.Name, werr)
		return werr
	}
	client, err := newClient(ctx, m.certDir, cert.ACME.DirectoryURL, cert.ACME.Email, eab, httpClient)
	if err != nil {
		werr := fmt.Errorf("certificate %q: %w", cert.Name, overran(err))
		m.recordFailure(cert.Name, werr)
		return werr
	}
	log.Info().Str("cert", cert.Name).Str("challenge", cert.ACME.EffectiveChallenge()).Msg("issuing/renewing certificate")
	if err := m.issue(ctx, client, cert, solver); err != nil {
		werr := fmt.Errorf("certificate %q: %w", cert.Name, overran(err))
		m.recordFailure(cert.Name, werr)
		return werr
	}
	// Re-read the freshly issued artifact so the expiry gauge reflects the new
	// certificate rather than the one it replaced.
	if meta, err := loadMeta(m.certDir, cert.Name); err == nil {
		m.recordIssued(cert, meta, m.now())
	}
	m.recordSuccess(cert.Name)
	return nil
}

// needsRenewal reports whether a cert must be (re)issued now: never issued,
// its domain set or profile changed, or its renewal time has passed (see
// renewalTime).
func (m *Manager) needsRenewal(cert model.Certificate) bool {
	meta, err := loadMeta(m.certDir, cert.Name)
	if err != nil {
		meta = nil
	}
	return m.dueNow(cert, meta, m.now())
}

// dueNow is needsRenewal at time now for already loaded metadata; meta nil
// means the certificate has not been issued.
func (m *Manager) dueNow(cert model.Certificate, meta *Meta, now time.Time) bool {
	if meta == nil || m.orderChanged(cert, meta) {
		return true
	}
	at, _ := m.renewalTime(meta, m.notBefore(cert.Name, meta))
	return now.After(at)
}

// orderChanged reports whether the configured order no longer matches the one
// the stored certificate came from: another domain set or another profile.
// Metadata written before profiles existed has none, which equals an unset
// profile in the config.
func (m *Manager) orderChanged(cert model.Certificate, meta *Meta) bool {
	profile := ""
	if cert.ACME != nil {
		profile = cert.ACME.Profile
	}
	return !sameStringSet(meta.Domains, cert.Domains) || meta.Profile != profile
}

// renewalTime returns when an issued certificate is due for renewal and which
// rule set that time.
//
// The lifetime rule renews when min(renewBefore, lifetime/3) remains, where
// lifetime is NotAfter - NotBefore: a 90-day certificate keeps the 30-day
// window, a longer one keeps 30 days, and a short-lived one renews a third of
// its lifetime before expiry instead of on every pass. The lifetime is rounded
// to the minute because CAs commonly issue a validity one second short of a
// round period (RFC 5280's NotAfter is inclusive); without the rounding a
// 90-day certificate would get a window a third of a second under 30 days.
//
// The CA's renewal information (ARI) can only bring that time forward. Its
// chosen time counts only once the certificate is at least min(72h,
// lifetime/3) old, so a CA that keeps answering "renew now" gets at most one
// order per that age: about 2.3 a week for a 90-day certificate, 3.2 for a
// 160-hour one and 21 for a 24-hour one. Orders carry no ARI "replaces"
// field, so a CA's rate limits apply to each of them.
func (m *Manager) renewalTime(meta *Meta, notBefore time.Time) (time.Time, string) {
	lifetime := meta.NotAfter.Sub(notBefore).Round(time.Minute)
	window := m.renewBefore
	minAge := ariMinAge
	if lifetime > 0 {
		window = min(window, lifetime/3)
		minAge = min(minAge, lifetime/3)
	}
	at, reason := meta.NotAfter.Add(-window), RenewReasonLifetime
	if a := meta.ARI; a != nil && !a.RenewAt.IsZero() {
		issued := meta.IssuedAt
		if issued.IsZero() {
			issued = notBefore
		}
		ariAt := a.RenewAt
		if earliest := issued.Add(minAge); ariAt.Before(earliest) {
			ariAt = earliest
		}
		if ariAt.Before(at) {
			at, reason = ariAt, RenewReasonARI
		}
	}
	return at, reason
}

// notBefore returns an issued certificate's NotBefore. Metadata written before
// the field existed has none; it is then read from the stored leaf, and failing
// that IssuedAt stands in (a close lower bound for a fresh certificate). The
// metadata file is not rewritten just to add it.
func (m *Manager) notBefore(name string, meta *Meta) time.Time {
	if !meta.NotBefore.IsZero() {
		return meta.NotBefore
	}
	if b, err := os.ReadFile(IssuedCertPath(m.certDir, name)); err == nil { //nolint:gosec // G304: path is certDir/acme/issued/<name>/fullchain.pem; name passed model.ValidateName
		if info, err := ParseLeaf(b); err == nil {
			return info.NotBefore
		}
	}
	return meta.IssuedAt
}

// renewWindow is the lifetime rule's window for a certificate: how long
// before expiry it is renewed. A certificate not issued yet has the default
// renewBefore.
func (m *Manager) renewWindow(name string, meta *Meta) time.Duration {
	if meta == nil {
		return m.renewBefore
	}
	if lifetime := meta.NotAfter.Sub(m.notBefore(name, meta)).Round(time.Minute); lifetime > 0 {
		return min(m.renewBefore, lifetime/3)
	}
	return m.renewBefore
}

// Bounds of retryBackoff.
const (
	minRetryBase    = 5 * time.Minute
	maxRetryBase    = time.Hour
	maxRetryBackoff = 12 * time.Hour
)

// retryBackoff is how long after its n-th consecutive failed attempt (n >= 1)
// a certificate is tried again. The base is a quarter of its renewal window,
// between five minutes and an hour, and it doubles with every further
// consecutive failure up to twelve hours, the regular pass interval:
//
//   - a 90-day or never-issued certificate (base 1h) waits 1h, 2h, 4h, 8h,
//     then 12h: at most 5 attempts in the first day of failures, then 2 a day;
//   - a 24-hour certificate (8h window, base 1h) the same, so 4 tries inside
//     its window;
//   - a 1-hour certificate (20m window, base 5m) waits 5m, 10m, 20m, ...: 3
//     tries inside its window, at most 9 attempts in the first day, then 2 a
//     day.
//
// A CA that keeps failing a domain whose validation is broken for good thus
// sees the old steady rate after the first day, not one attempt an hour.
func retryBackoff(window time.Duration, n int) time.Duration {
	d := min(max(window/4, minRetryBase), maxRetryBase)
	for i := 1; i < n && d < maxRetryBackoff; i++ {
		d *= 2
	}
	return min(d, maxRetryBackoff)
}

// lastAttempt returns when a certificate's last issue/renew attempt in this
// process started, whether it failed and how many attempts in a row have
// failed. A last attempt after now (the wall clock stepped back since) is
// reported as zero, "long ago": otherwise a clock that was wrong when the
// attempt ran, a likely cause of the failure itself, would hold off every
// retry until it caught up.
func (m *Manager) lastAttempt(name string, now time.Time) (time.Time, bool, int) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	o := m.obs[name]
	if o == nil || o.LastAttempt.IsZero() {
		return time.Time{}, false, 0
	}
	last := o.LastAttempt
	if last.After(now) {
		last = time.Time{}
	}
	return last, o.LastError != "", o.consecutiveFailures
}

// retryAt returns when a certificate whose last attempt failed may be tried
// again; failed is false when its last attempt (if any) succeeded.
func (m *Manager) retryAt(cert model.Certificate, meta *Meta, now time.Time) (time.Time, bool) {
	last, failed, n := m.lastAttempt(cert.Name, now)
	if !failed {
		return time.Time{}, false
	}
	if last.IsZero() {
		return now, true // the clock stepped back: retry now
	}
	return last.Add(retryBackoff(m.renewWindow(cert.Name, meta), n)), true
}

// nextDue returns when Run should pass again for cfg's ACME certificates, or
// zero when nothing needs a pass before the regular tick:
//
//   - a due certificate whose last attempt failed: its retry time (see
//     retryBackoff), so a short-lived certificate is retried inside its window;
//   - an issued certificate: its renewal time when that is still ahead, or when
//     it passed without an attempt since (it became due while the pass ran).
//
// A certificate that was attempted successfully and is still due (a CA handing
// out a certificate that is due on arrival) is left to the regular tick, so no
// combination of answers can make Run order in a tight loop.
func (m *Manager) nextDue(cfg model.Config, now time.Time) time.Time {
	var next time.Time
	consider := func(at time.Time) {
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	for _, cert := range cfg.Certificates {
		if cert.Disabled || cert.Type != model.CertTypeACME || cert.ACME == nil {
			continue
		}
		meta, err := loadMeta(m.certDir, cert.Name)
		if err != nil {
			meta = nil
		}
		if m.dueNow(cert, meta, now) {
			if at, failed := m.retryAt(cert, meta, now); failed {
				consider(at)
				continue
			}
		}
		if meta == nil || m.orderChanged(cert, meta) {
			continue // attempted on every pass that finds it due
		}
		at, _ := m.renewalTime(meta, m.notBefore(cert.Name, meta))
		if last, _, _ := m.lastAttempt(cert.Name, now); at.After(now) || last.Before(at) {
			consider(at)
		}
	}
	return next
}

// earlyWake returns how long Run sleeps before a pass for a certificate whose
// renewal time is due, never less than floor; ok is false when there is no
// such certificate. Run waits for the earlier of this and its regular tick.
func earlyWake(now, due time.Time, floor time.Duration) (time.Duration, bool) {
	if due.IsZero() {
		return 0, false
	}
	return max(due.Sub(now), floor), true
}

// Run does an initial EnsureAll then renews on an interval until ctx is done.
// loadConfig fetches the current config each cycle so newly-added certs are
// picked up without a restart.
//
// The regular interval (12h by default) is longer than the renewal window of
// a short-lived certificate (a 24h certificate has an 8h window), so after
// each pass Run also wakes early for the next certificate entering its window
// or due for a retry (see nextDue), and five minutes after a pass that could
// not load the config. The regular ticks keep their own cadence either way.
func (m *Manager) Run(ctx context.Context, interval time.Duration, loadConfig func(context.Context) (model.Config, error)) {
	if interval == 0 {
		interval = 12 * time.Hour
	}
	run := func() (model.Config, bool) {
		cfg, err := loadConfig(ctx)
		if err != nil {
			log.Error().Err(err).Msg("acme: failed to load config for renewal cycle")
			return model.Config{}, false
		}
		if _, err := m.EnsureAll(ctx, cfg); err != nil {
			log.Error().Err(err).Msg("acme: one or more certificates failed to issue/renew")
		}
		return cfg, true
	}
	cfg, ok := run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		var (
			timer *time.Timer
			wake  <-chan time.Time
		)
		if ok {
			now := m.now()
			if d, early := earlyWake(now, m.nextDue(cfg, now), m.minWake); early {
				timer = time.NewTimer(d)
				wake = timer.C
			}
		} else {
			// Without the config nothing above can be computed; try again soon
			// rather than leaving a due certificate to the regular tick.
			timer = time.NewTimer(m.loadRetry)
			wake = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-ticker.C:
		case <-wake:
		}
		if timer != nil {
			timer.Stop()
		}
		cfg, ok = run()
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
	case model.DNSProviderWebhook:
		return newWebhookDNSFromConfig(p)
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

// resolveFlag reads an opt-in flag out of the config map. Every value there is
// a Secret, and an API write refuses a literal one, so the flag is resolved
// like any other key (a ${ENV:...} placeholder resolving to "true" turns it
// on) and only then read with model.IsTruthyConfig. A literal in a hand-edited
// file resolves to itself.
func resolveFlag(p model.DNSProvider, key string) (bool, error) {
	v, err := resolveConfig(p, key)
	if err != nil {
		return false, err
	}
	return model.IsTruthyConfig(v), nil
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

// newWebhookDNSFromConfig builds a webhook solver from the config map.
func newWebhookDNSFromConfig(p model.DNSProvider) (DNSSolver, error) {
	var (
		c   WebhookDNSConfig
		err error
	)
	if c.URL, err = resolveConfig(p, "url"); err != nil {
		return nil, err
	}
	if c.Token, err = resolveConfig(p, "token"); err != nil {
		return nil, err
	}
	if c.AllowInsecureLocal, err = resolveFlag(p, "allowInsecureLocal"); err != nil {
		return nil, err
	}
	return NewWebhookDNSSolver(c)
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
	if c.AllowInsecureLocal, err = resolveFlag(p, "allowInsecureLocal"); err != nil {
		return nil, err
	}
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
