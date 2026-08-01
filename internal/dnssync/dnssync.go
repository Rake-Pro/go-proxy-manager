// Package dnssync keeps DNS records in step with the proxy hosts that opted into
// it. Each enabled proxy host carrying a `dns` policy contributes its domains to
// a desired set, which is reconciled against two independent backends: a local
// resolver (Pi-hole v6, for lanDirect) and the authoritative public zone
// (Cloudflare, for publicCname).
//
// Two properties define the design:
//
//   - Reconcile is FULL-STATE, not diff-based. The desired set is recomputed
//     from the whole config on every run and compared with what the backend
//     actually holds, so a record lost to an out-of-band edit is restored and a
//     host deleted while gpm was down is still cleaned up.
//   - Deletion is limited to records gpm demonstrably owns. On Pi-hole that
//     means a CNAME whose target is exactly the configured apexTarget; on
//     Cloudflare a record carrying the "managed-by:gpm" comment. Anything else
//     in the zone - including records for hosts gpm does not know about - is
//     read and ignored, never removed.
//
// Delivery follows the webhook dispatcher's shape: settings are read live on
// every run, triggers are non-blocking, and a failing backend is reported in
// status rather than propagated into the config write that triggered it.
package dnssync

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// ManagedComment is the ownership marker gpm writes on every Cloudflare record
// it creates. A record without it is never deleted, whatever its name.
const ManagedComment = "managed-by:gpm"

// ErrReconcileInProgress is returned by ReconcileNow when a reconcile is
// already running. The API maps it to 409 Conflict.
var ErrReconcileInProgress = errors.New("dnssync: a reconcile is already in progress")

// maxRespBody caps how much of a backend response is read into memory.
const maxRespBody = 4 << 20

// BackendStatus is the outcome of the last reconcile against one backend.
type BackendStatus struct {
	Enabled bool      `json:"enabled"`
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
	LastRun time.Time `json:"lastRun,omitempty"`
	// Desired is how many records the config asks for; Managed is how many
	// gpm-owned records the backend already held; Created/Deleted count the
	// changes this run made.
	Desired int `json:"desired"`
	Managed int `json:"managed"`
	Created int `json:"created"`
	Deleted int `json:"deleted"`
}

// Status is the full result of the last reconcile, served by GET /dns-sync/status.
type Status struct {
	LastRun    time.Time     `json:"lastRun,omitempty"`
	Error      string        `json:"error,omitempty"`
	Pihole     BackendStatus `json:"pihole"`
	Cloudflare BackendStatus `json:"cloudflare"`
}

// Syncer reconciles DNS records for the opted-in proxy hosts.
type Syncer struct {
	load   func(context.Context) (model.Config, model.Settings, error)
	client *http.Client

	mu     sync.Mutex
	status Status

	// single serialises reconciles so two runs never race the same backend.
	single sync.Mutex

	// gate guards the trigger coalescing state: while a run is in flight, any
	// number of triggers collapse into a single queued follow-up run, so a burst
	// of config writes costs at most one extra reconcile.
	gate    sync.Mutex
	running bool
	queued  bool
}

// New returns a Syncer. load is called on every reconcile so configuration
// changes take effect without re-wiring (the webhook dispatcher's live-targets
// pattern). A nil load makes every reconcile a no-op error.
//
// The HTTP client is hardened the same way the webhook client is: redirects are
// never followed, and link-local destinations are refused at connect time
// (post-DNS) so an admin-configured Pi-hole URL cannot be pointed at a cloud
// metadata service. Private ranges stay allowed - a LAN Pi-hole is the point.
func New(load func(context.Context) (model.Config, model.Settings, error)) *Syncer {
	return &Syncer{
		load: load,
		client: &http.Client{
			Timeout: 20 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
					Control:   refuseLinkLocal,
				}).DialContext,
				MaxIdleConns:    4,
				IdleConnTimeout: 60 * time.Second,
			},
		},
	}
}

func refuseLinkLocal(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return fmt.Errorf("dnssync: link-local destination %s refused", ip)
	}
	return nil
}

// Status returns a snapshot of the last reconcile result.
func (s *Syncer) Status() Status {
	if s == nil {
		return Status{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Enabled reports which backends are configured, for the capability probe. A
// load failure reports both disabled rather than guessing.
func (s *Syncer) Enabled() (pihole, cloudflare bool) {
	if s == nil || s.load == nil {
		return false, false
	}
	_, settings, err := s.load(context.Background())
	if err != nil {
		return false, false
	}
	return settings.DNSSync.Pihole.Enabled, settings.DNSSync.Cloudflare.Enabled
}

// Trigger asks for a reconcile without blocking the caller. It is safe to call
// from a config-write handler: at most one reconcile runs at a time and repeated
// triggers during a run coalesce into one follow-up, so a bulk change (restore,
// revert, import) does not queue a run per object.
func (s *Syncer) Trigger() {
	if s == nil || s.load == nil {
		return
	}
	s.gate.Lock()
	if s.running {
		s.queued = true
		s.gate.Unlock()
		return
	}
	s.running = true
	s.gate.Unlock()
	go s.drain()
}

func (s *Syncer) drain() {
	for {
		if err := s.Reconcile(context.Background()); err != nil {
			log.Warn().Err(err).Msg("dnssync: reconcile failed")
		}
		s.gate.Lock()
		if !s.queued {
			s.running = false
			s.gate.Unlock()
			return
		}
		s.queued = false
		s.gate.Unlock()
	}
}

// Reconcile runs one full-state reconcile synchronously and records the result
// in Status, WAITING for any in-flight run to finish first. This is the
// event-triggered path (see drain): waiting is what makes trigger coalescing
// correct - the follow-up run must observe the config that caused it.
//
// The returned error describes a failure to run at all (config could not be
// loaded); a per-backend failure is reported in that backend's status and does
// not fail the other backend or the call.
func (s *Syncer) Reconcile(ctx context.Context) error {
	if s == nil || s.load == nil {
		return fmt.Errorf("dnssync: no configuration source wired")
	}
	s.single.Lock()
	defer s.single.Unlock()
	return s.reconcileLocked(ctx)
}

// ReconcileNow is the HTTP-triggered variant: it refuses with
// ErrReconcileInProgress instead of queueing behind a run already in flight. A
// manual reconcile is a request to see current state, so blocking would only
// pile up goroutines - each holding a request-scoped context - behind a slow or
// hung backend, which is the whole point of the 20s client timeout being a
// ceiling rather than a guarantee.
func (s *Syncer) ReconcileNow(ctx context.Context) error {
	if s == nil || s.load == nil {
		return fmt.Errorf("dnssync: no configuration source wired")
	}
	if !s.single.TryLock() {
		return ErrReconcileInProgress
	}
	defer s.single.Unlock()
	return s.reconcileLocked(ctx)
}

// reconcileLocked does the work; callers hold s.single.
func (s *Syncer) reconcileLocked(ctx context.Context) error {
	now := time.Now().UTC()
	cfg, settings, err := s.load(ctx)
	if err != nil {
		s.mu.Lock()
		s.status = Status{LastRun: now, Error: err.Error()}
		s.mu.Unlock()
		return fmt.Errorf("dnssync: load config: %w", err)
	}

	st := Status{LastRun: now}
	if p := settings.DNSSync.Pihole; p.Enabled {
		st.Pihole = s.syncPihole(ctx, cfg, p)
		st.Pihole.Enabled = true
		st.Pihole.LastRun = now
	}
	if c := settings.DNSSync.Cloudflare; c.Enabled {
		st.Cloudflare = s.syncCloudflare(ctx, cfg, c)
		st.Cloudflare.Enabled = true
		st.Cloudflare.LastRun = now
	}
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
	return nil
}

// desiredDomains collects the domains every enabled proxy host asks to publish
// through the given policy flag. Disabled hosts contribute nothing (they are not
// in the compiled data plane, so a record pointing at gpm would be a lie), and
// two domains are always skipped:
//
//   - wildcards ("*.example.com"), which neither backend can express as a plain
//     managed CNAME in the way this reconciler owns records, and
//   - the apex target itself, which would be a CNAME loop.
//
// The result is lowercased, de-duplicated and sorted so a reconcile is
// deterministic and diffable.
func desiredDomains(cfg model.Config, want func(model.DNSSyncPolicy) bool, apexTarget string) []string {
	apex := strings.ToLower(strings.TrimSuffix(apexTarget, "."))
	seen := map[string]bool{}
	var out []string
	for _, h := range cfg.ProxyHosts {
		if h.Disabled || h.DNS == nil || !want(*h.DNS) {
			continue
		}
		for _, d := range h.Domains {
			name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d), "."))
			if name == "" || strings.HasPrefix(name, "*.") || name == apex || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
