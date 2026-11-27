// Package accesssync keeps model.AccessList sources (remote, published IP
// feeds) up to date in the committed source ledger.
//
// It exists so an operator can allow a monitoring provider's published prober
// addresses - UptimeRobot's ~200 IPv4/IPv6 entries, say - to reach the health
// endpoints of a host that is otherwise LAN- or VPN-only, without pasting those
// addresses into the config by hand and re-pasting them whenever the provider
// changes them.
//
// The whole design point is that a remote body decides who reaches a host, so
// every step fails CLOSED and refuses whole rather than accepting part:
//
//   - https only, and the dialer refuses loopback, link-local, private/ULA and
//     multicast destinations, so a source URL cannot be aimed at a metadata
//     service or an internal endpoint (SSRF),
//   - the body is capped at 1 MiB,
//   - a non-200, an empty result, a result over the source's maxEntries or
//     maxCoverage (when set), or a SINGLE unparseable line refuses the fetch and
//     KEEPS the previously fetched set, rather than replacing it with
//     something gpm does not understand,
//   - unless the source sets maxAge: once its last successful fetch is older
//     than that, the kept set is emptied in the ledger (one commit) and the
//     source matches nothing until a fetch succeeds again.
package accesssync

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// maxBodyBytes caps how much of a source body is read. The published lists this
// exists for are a few kilobytes; 1 MiB is three orders of magnitude of headroom
// and still bounds a hostile or broken endpoint.
const maxBodyBytes = 1 << 20

// ErrReconcileInProgress is returned by ReconcileNow when a run is already in
// flight, mirroring dnssync/k8s: a manual reconcile is a request to see current
// state, so it refuses rather than queueing request-scoped goroutines.
var ErrReconcileInProgress = errors.New("access-list source reconcile already in progress")

// Ledger persists the fetched sets across restarts. It is an interface so the
// syncer depends on the two operations it needs rather than on the config store,
// and so tests can supply an in-memory one.
//
// Load returns an opaque revision for the state it read and Save is handed it
// back, so a concurrent revert makes the write a refusal rather than a lost
// update (see store.SaveAccessListSourceLedger).
//
// A nil Ledger makes every reconcile a no-op error: with nowhere to persist a
// fetched set, running one would only produce work the data plane never sees.
type Ledger interface {
	Load(ctx context.Context) (model.AccessListSourceLedger, string, error)
	Save(ctx context.Context, l model.AccessListSourceLedger, rev string) error
}

// SourceStatus is the last-known state of one access-list source.
type SourceStatus struct {
	List        string    `json:"list"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	FetchedAt   time.Time `json:"fetchedAt,omitempty"`
	EntryCount  int       `json:"entryCount"`
	LastError   string    `json:"lastError,omitempty"`
	LastAttempt time.Time `json:"lastAttempt,omitempty"`
	// Expired is true when maxAge passed with no successful fetch: the set is
	// empty and FetchedAt is the last successful fetch.
	Expired bool `json:"expired,omitempty"`
}

// Status is the result of the last reconcile, served by
// GET /access-list-sources/status.
//
// LastRun and LastSuccess are separate for the same reason the DNS syncer keeps
// them separate: "last run 2 minutes ago" hides "last clean run six hours ago".
// Refused counts sources whose most recent fetch was rejected (bad status, empty
// body, over the cap, unparseable line) and whose previously fetched set is
// therefore still what the data plane serves.
type Status struct {
	Enabled     bool           `json:"enabled"`
	LastRun     time.Time      `json:"lastRun,omitempty"`
	LastSuccess time.Time      `json:"lastSuccess,omitempty"`
	Error       string         `json:"error,omitempty"`
	Refused     int            `json:"refused"`
	Sources     []SourceStatus `json:"sources"`
}

// Syncer fetches the sources declared by the configured access lists.
type Syncer struct {
	load     func(context.Context) (model.Config, model.Settings, error)
	ledger   Ledger
	onChange func()
	client   *http.Client
	now      func() time.Time

	// ctx is the lifecycle context triggered runs inherit; see SetContext.
	ctx context.Context

	mu          sync.Mutex
	status      Status
	lastSuccess time.Time
	// attempts records, per "<list>/<source>", when this PROCESS last tried a
	// fetch. It is what makes an unchanged feed cost no git commit: the ledger's
	// fetchedAt only advances when the set actually changed (writing it on every
	// no-op would commit a timestamp-only diff every interval, forever), so
	// without this every poll would re-fetch every source. See
	// docs/reference/config/settings/access-list-sync.md.
	attempts map[string]time.Time
	// hashes is the sha256 of the set each recorded attempt fetched, so a
	// success is only credited to the ledger entry holding that same set.
	hashes map[string]string

	// single serialises reconciles so two runs never race the same ledger.
	single sync.Mutex

	gate    sync.Mutex
	running bool
	queued  bool
}

// New returns a Syncer. load is called on every reconcile so configuration
// changes take effect without re-wiring; onChange is invoked after the ledger is
// successfully written, to reload the data plane so the new set is served on the
// very next request. Either may be nil.
func New(load func(context.Context) (model.Config, model.Settings, error), ledger Ledger, onChange func()) *Syncer {
	return &Syncer{
		load:     load,
		ledger:   ledger,
		onChange: onChange,
		now:      time.Now,
		attempts: map[string]time.Time{},
		hashes:   map[string]string{},
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				// No proxy, deliberately, and NOT ProxyFromEnvironment. The SSRF
				// guard below inspects the address actually dialled; through a
				// proxy that address is the PROXY, so every internal destination
				// would sail past the check while the proxy happily fetched it. An
				// HTTP_PROXY in the environment must not be able to disable a
				// security control.
				Proxy: nil,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
					Control:   refuseInternalDest,
				}).DialContext,
				MaxIdleConns:    4,
				IdleConnTimeout: 60 * time.Second,
			},
		},
	}
}

// nonPublicRanges are the address blocks that are never legitimately part of a
// public monitoring feed, and never a legitimate place to FETCH one from. One
// table serves both jobs: the SSRF guard (refuseInternalDest, which decides
// where gpm will connect) and the entry filter (RefuseEntry, which decides what
// a feed may contain). They are the same question asked in two directions, and
// keeping two lists in step by hand is how one of them ends up short.
//
// net.IP's own predicates cover loopback, link-local, ULA, multicast, RFC1918
// and the unspecified address (see isNonPublic); these are the blocks they miss.
// Documentation/TEST-NET ranges are deliberately NOT listed: they are harmless in
// a feed and are what the tests use.
var nonPublicRanges = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{
		"100.64.0.0/10", // RFC 6598 CGNAT - shared space, routable on a LAN
		"192.0.0.0/24",  // RFC 6890 IETF protocol assignments
		"198.18.0.0/15", // RFC 2544 benchmarking
		"64:ff9b::/96",  // RFC 6052 NAT64 - an internal v4 address in v6 clothes
	} {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// predicateRanges are the blocks net.IP's predicates in isNonPublic match,
// spelled out as networks. isNonPublic answers for one address; RefuseEntry
// also needs to know whether a feed network merely overlaps one of them (a
// 172.0.0.0/8 entry starts at a public address but covers 172.16.0.0/12).
var predicateRanges = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{
		"0.0.0.0/32", "::/128", // unspecified
		"127.0.0.0/8", "::1/128", // loopback
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7", // private, ULA
		"169.254.0.0/16", "fe80::/10", // link-local unicast
		"224.0.0.0/4", "ff00::/8", // multicast (covers link- and interface-local)
	} {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// isNonPublic reports whether ip is in a range no public feed and no public
// endpoint ever legitimately uses.
func isNonPublic(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	for _, n := range nonPublicRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// refuseInternalDest is the SSRF guard, applied POST-DNS at connect time so a
// hostname that resolves to an internal address is refused just as a literal one
// is. Unlike the DNS syncer - whose whole purpose is talking to a LAN Pi-hole -
// nothing legitimate here is internal: a source is a published list on the
// public internet.
func refuseInternalDest(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("accesssync: destination %q is not an IP address", host)
	}
	if isNonPublic(ip) {
		return fmt.Errorf("accesssync: internal destination %s refused (a source must be a public https URL)", ip)
	}
	return nil
}

// Minimum prefix lengths an entry may carry. A published prober list names
// individual machines and small blocks; a /7 is not a monitoring range, it is an
// attempt to allow a continent. Anything shorter is refused, which - because
// parsing is strict - refuses the whole fetch.
const (
	minEntryBitsV4 = 8
	minEntryBitsV6 = 32
)

// RefuseEntry reports why one parsed network may not appear in a feed, or nil.
// The data plane applies it again to every stored entry when it compiles the
// ledger, so an entry stored before a guard existed is not served.
//
// This is the guard that matters most in the whole subsystem: a source rule
// WITHOUT paths grants its networks unrestricted access to the host, so a single
// hijacked or typo'd line reading "0.0.0.0/0" would otherwise pass every other
// check (valid CIDR, under maxEntries, HTTP 200) and allow the entire internet.
// The default routes are refused unconditionally, over-broad prefixes are
// refused, and so is anything inside a range a public feed can never legitimately
// contain - which would otherwise let a feed hand an attacker a foothold on the
// LAN side of the proxy.
func RefuseEntry(n *net.IPNet) error {
	ones, bits := n.Mask.Size()
	if bits == 0 {
		return fmt.Errorf("%s has a non-contiguous mask", n)
	}
	if ones == 0 {
		return fmt.Errorf("%s is the default route: a feed may never allow every address", n)
	}
	min := minEntryBitsV6
	if bits == 32 {
		min = minEntryBitsV4
	}
	if ones < min {
		return fmt.Errorf("%s is broader than the /%d limit for this family", n, min)
	}
	nonPublic := isNonPublic(n.IP)
	for _, set := range [][]*net.IPNet{predicateRanges, nonPublicRanges} {
		for _, r := range set {
			// Two prefixes overlap exactly when one contains the other's
			// network address.
			if n.Contains(r.IP) || r.Contains(n.IP) {
				nonPublic = true
			}
		}
	}
	if nonPublic {
		return fmt.Errorf("%s is not a public address range, or overlaps one (loopback, link-local, private, ULA, CGNAT, multicast, reserved or documentation space)", n)
	}
	return nil
}

// CheckCoverage reports, or nil, why a set of networks covers more address space
// than src's coverage budget allows (see model.AccessListSource.MaxCoverage). A
// source without maxCoverage has no budget.
func CheckCoverage(nets []*net.IPNet, src model.AccessListSource) error {
	limV4, limV6, ok := src.CoverageLimits()
	if !ok {
		return nil
	}
	return checkCoverage(nets, limV4, limV6)
}

// checkCoverage sums each family on its own; overlapping entries count twice,
// which only errs toward refusing. IPv6 is counted in /64 units, an entry
// longer than /64 counting as one.
func checkCoverage(nets []*net.IPNet, limV4, limV6 int) error {
	budget4, budget6 := uint64(1)<<(32-limV4), uint64(1)<<(64-limV6)
	var sum4, sum6 uint64
	for _, n := range nets {
		ones, bits := n.Mask.Size()
		if bits == 32 {
			size := uint64(1) << (32 - ones)
			if size > budget4-sum4 {
				return fmt.Errorf("IPv4 entries cover more than the /%d-equivalent (%d addresses) coverage budget for this source", limV4, budget4)
			}
			sum4 += size
			continue
		}
		size := uint64(1)
		if ones < 64 {
			if ones < 1 {
				return fmt.Errorf("%s exceeds the IPv6 coverage budget for this source", n)
			}
			size = uint64(1) << (64 - ones)
		}
		if size > budget6-sum6 {
			return fmt.Errorf("IPv6 entries cover more than the /%d-equivalent coverage budget for this source", limV6)
		}
		sum6 += size
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

// Enabled reports whether the fetcher is turned on, for the capability probe. A
// load failure reports disabled rather than guessing.
func (s *Syncer) Enabled() bool {
	if s == nil || s.load == nil {
		return false
	}
	_, settings, err := s.load(context.Background())
	if err != nil {
		return false
	}
	return settings.AccessListSync.IsEnabled()
}

// Trigger asks for a reconcile without blocking the caller. At most one run
// happens at a time and repeated triggers during a run coalesce into one
// follow-up, so a bulk config change costs one extra reconcile.
//
// The run inherits the context Run was started with (SetContext), so a triggered
// reconcile is cancelled by shutdown like every other one - a Trigger arriving
// as the process stops must not outlive it and race the store on the way out.
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

// SetContext installs the lifecycle context triggered runs use. Run calls it, so
// wiring Run is all that is needed; before that (and in tests) triggered runs
// fall back to context.Background().
func (s *Syncer) SetContext(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
}

func (s *Syncer) runContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *Syncer) drain() {
	for {
		ctx := s.runContext()
		if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
			log.Warn().Err(err).Msg("accesssync: reconcile failed")
		}
		s.gate.Lock()
		if ctx.Err() != nil {
			// Shutting down: drop any queued follow-up rather than spinning on a
			// cancelled context.
			s.queued, s.running = false, false
			s.gate.Unlock()
			return
		}
		if !s.queued {
			s.running = false
			s.gate.Unlock()
			return
		}
		s.queued = false
		s.gate.Unlock()
	}
}

// Run polls until ctx is cancelled, checking on the configured interval whether
// any source is due. The interval is re-read from settings every iteration, so
// enabling, disabling or re-tuning the fetcher needs no restart.
func (s *Syncer) Run(ctx context.Context) {
	if s == nil || s.load == nil {
		return
	}
	s.SetContext(ctx)
	for {
		if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
			log.Warn().Err(err).Msg("accesssync: reconcile failed")
		}
		t := time.NewTimer(s.pollInterval(ctx))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

func (s *Syncer) pollInterval(ctx context.Context) time.Duration {
	_, settings, err := s.load(ctx)
	if err != nil {
		return model.DefaultAccessListSyncPollInterval
	}
	return settings.AccessListSync.Poll()
}

// Reconcile runs one pass synchronously, waiting for any in-flight run.
func (s *Syncer) Reconcile(ctx context.Context) error {
	if s == nil || s.load == nil {
		return fmt.Errorf("accesssync: no configuration source wired")
	}
	s.single.Lock()
	defer s.single.Unlock()
	return s.reconcileLocked(ctx)
}

// ReconcileNow is the HTTP-triggered variant: it refuses with
// ErrReconcileInProgress rather than queueing behind a run already in flight.
func (s *Syncer) ReconcileNow(ctx context.Context) error {
	if s == nil || s.load == nil {
		return fmt.Errorf("accesssync: no configuration source wired")
	}
	if !s.single.TryLock() {
		return ErrReconcileInProgress
	}
	defer s.single.Unlock()
	return s.reconcileLocked(ctx)
}

func (s *Syncer) reconcileLocked(ctx context.Context) error {
	now := s.now()
	cfg, settings, err := s.load(ctx)
	if err != nil {
		s.setStatus(Status{LastRun: now, Error: err.Error()})
		return err
	}
	if !settings.AccessListSync.IsEnabled() {
		s.setStatus(Status{LastRun: now})
		return nil
	}
	if s.ledger == nil {
		err := fmt.Errorf("accesssync: no ledger wired; fetched sets would have nowhere to live")
		s.setStatus(Status{Enabled: true, LastRun: now, Error: err.Error()})
		return err
	}

	ledger, rev, err := s.ledger.Load(ctx)
	if err != nil {
		s.setStatus(Status{Enabled: true, LastRun: now, Error: err.Error()})
		return err
	}
	entries := ledger.AccessListSourceMap()

	st := Status{Enabled: true, LastRun: now}
	// changed means the ledger must be saved; reload means the save changes what
	// the data plane serves. A save that only advances fetchedAt (maxAge
	// bookkeeping) is committed, and so replicated, without a reload.
	changed, reload := false, false
	// Only sources the config still declares survive into the next ledger, so a
	// source an operator removed stops being carried (and stops being served)
	// rather than lingering as an allow list nothing references.
	next := map[string]model.AccessListSourceEntry{}
	for _, al := range cfg.AccessLists {
		for _, src := range al.Sources {
			key := model.AccessListSourceKey(al.Name, src.Name)
			prev, had := entries[key]
			if had && prev.URL == src.URL {
				next[key] = prev
			} else if had {
				// The URL moved: the old body is no longer an answer to the
				// question the config asks, so it is dropped and re-fetched now
				// (the attempt recorded against the old URL is forgotten too).
				changed, reload = true, true
				s.clearAttempt(key)
			}
			ss := SourceStatus{List: al.Name, Name: src.Name, URL: src.URL}
			if e, ok := next[key]; ok {
				ss.FetchedAt = e.Fetched()
				ss.EntryCount = len(e.Entries)
				ss.Expired = e.ExpiredAt != ""
			}
			ss.LastAttempt = s.attempt(key)

			// maxAge. The age of a set is measured from the last successful
			// fetch recorded in the ledger (fetchedAt, advanced on every success
			// while maxAge is set), or from maxAgeSince, when maxAge was first
			// applied to the source, if that is later. A success this process
			// made but could not commit counts only while the process runs and
			// only for the set the ledger holds; after a restart it is lost.
			// Every write here changes timestamps only: saved, never reloaded.
			maxAge, hasMaxAge := src.MaxAgeLimit()
			stale := false
			if e, ok := next[key]; ok {
				switch {
				case hasMaxAge && e.MaxAgeSince == "":
					e.MaxAgeSince = now.UTC().Format(time.RFC3339)
					changed = true
				case !hasMaxAge && e.MaxAgeSince != "":
					e.MaxAgeSince = ""
					changed = true
				}
				if hasMaxAge && e.ExpiredAt == "" {
					// Compared at the ledger's one-second resolution, so it is
					// written once, not every tick.
					at, hash := s.success(key)
					if at = at.UTC().Truncate(time.Second); hash == e.SHA256 && at.After(e.Fetched()) {
						e.FetchedAt = at.Format(time.RFC3339)
						changed = true
						ss.FetchedAt = at
					}
					base := e.Fetched()
					if since, err := time.Parse(time.RFC3339, e.MaxAgeSince); err == nil && since.After(base) {
						base = since
					}
					stale = now.Sub(base) > maxAge
				}
				next[key] = e
			}

			// A stale set needs no extra fetch: its baseline is at least one
			// interval old, so it is due anyway, and a refused fetch is retried
			// on every poll. The one exception is a changed set whose commit
			// failed; that waits for its interval like any other.
			if !s.due(key, next[key], src, now) {
				st.Sources = append(st.Sources, ss)
				continue
			}
			ss.LastAttempt = now
			fetched, err := s.fetch(ctx, src)
			if err != nil {
				// Refusal: the previously fetched set (already copied into next)
				// stays exactly as it was. The attempt is deliberately NOT
				// recorded, so the retry happens on the next poll tick (minutes)
				// rather than after the source's own interval (up to a day) - a
				// transient blip must not leave a list pinned to a stale set for
				// 24 hours.
				st.Refused++
				ss.LastError = err.Error()
				log.Warn().Err(err).Str("accessList", al.Name).Str("source", src.Name).Str("host", sourceHost(src.URL)).
					Msg("accesssync: refused a source fetch; keeping the previously fetched set")
				// A stale set is emptied, once: ExpiredAt then marks it and it is
				// no longer stale.
				if stale {
					e := next[key]
					next[key] = model.AccessListSourceEntry{
						List:        al.Name,
						Source:      src.Name,
						URL:         src.URL,
						FetchedAt:   e.FetchedAt,
						ExpiredAt:   now.UTC().Format(time.RFC3339),
						MaxAgeSince: e.MaxAgeSince,
						SHA256:      model.AccessListSourceHash(key, src.URL, nil),
					}
					changed, reload = true, true
					ss.EntryCount = 0
					ss.Expired = true
					log.Warn().Str("accessList", al.Name).Str("source", src.Name).Str("host", sourceHost(src.URL)).
						Str("maxAge", src.MaxAge).Str("lastSuccess", e.FetchedAt).Str("maxAgeSince", e.MaxAgeSince).
						Msg("accesssync: access-list source expired (maxAge passed with no successful fetch); rules referencing it match nothing until a fetch succeeds")
				}
				st.Sources = append(st.Sources, ss)
				continue
			}
			hash := model.AccessListSourceHash(key, src.URL, fetched)
			// Success: this is what starts the interval clock (see due()).
			s.markAttempt(key, now, hash)
			if src.MaxCoverage == nil && checkCoverage(entryNets(fetched), model.BroadAccessListSourceCoverageV4, model.BroadAccessListSourceCoverageV6) != nil {
				log.Warn().Str("accessList", al.Name).Str("source", src.Name).Str("host", sourceHost(src.URL)).
					Msg("accesssync: access-list source is broad (more than one /8 of IPv4 or one /24 of IPv6 in total) and has no maxCoverage; set maxCoverage on the source to bound it")
			}
			if prevEntry, ok := next[key]; ok && prevEntry.SHA256 == hash {
				// Unchanged. fetchedAt is deliberately NOT advanced: doing so would
				// commit a timestamp-only diff on every interval forever. The
				// in-memory attempt above is what stops the next poll re-fetching.
				// The exception is a source with maxAge, whose expiry is measured
				// from fetchedAt after a restart: it must name the last successful
				// fetch, so it is advanced (one commit per interval, opt-in, with
				// no data-plane reload since the set is unchanged).
				if hasMaxAge {
					prevEntry.FetchedAt = now.UTC().Format(time.RFC3339)
					next[key] = prevEntry
					changed = true
					ss.FetchedAt = now.UTC()
				}
				ss.EntryCount = len(prevEntry.Entries)
				st.Sources = append(st.Sources, ss)
				continue
			}
			added, removed := diffCounts(next[key].Entries, fetched)
			since := next[key].MaxAgeSince
			if hasMaxAge && since == "" {
				since = now.UTC().Format(time.RFC3339)
			}
			next[key] = model.AccessListSourceEntry{
				List:        al.Name,
				Source:      src.Name,
				URL:         src.URL,
				FetchedAt:   now.UTC().Format(time.RFC3339),
				MaxAgeSince: since,
				SHA256:      hash,
				Entries:     fetched,
			}
			changed, reload = true, true
			ss.FetchedAt = now.UTC()
			ss.EntryCount = len(fetched)
			ss.Expired = false
			st.Sources = append(st.Sources, ss)
			log.Info().Str("accessList", al.Name).Str("source", src.Name).
				Int("entries", len(fetched)).Int("added", added).Int("removed", removed).
				Msg("accesssync: access-list source updated")
		}
	}
	// A source the config dropped is a change even though nothing was fetched.
	for key := range entries {
		if _, kept := next[key]; !kept {
			changed, reload = true, true
			break
		}
	}

	if changed {
		ledger.Sources = model.AccessListSourceEntries(next)
		if err := s.ledger.Save(ctx, ledger, rev); err != nil {
			st.Error = err.Error()
			s.setStatus(st)
			return err
		}
		if reload && s.onChange != nil {
			s.onChange()
		}
	}
	s.setStatus(st)
	return nil
}

// due reports whether a source should be fetched now: never fetched at all, or
// the longer of its ledger fetchedAt and this process's last attempt is at least
// one interval old.
func (s *Syncer) due(key string, e model.AccessListSourceEntry, src model.AccessListSource, now time.Time) bool {
	last := e.Fetched()
	if a := s.attempt(key); a.After(last) {
		last = a
	}
	if last.IsZero() {
		return true
	}
	return now.Sub(last) >= src.FetchInterval()
}

func (s *Syncer) attempt(key string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts[key]
}

// success returns this process's last successful fetch of key and the hash
// of the set it fetched.
func (s *Syncer) success(key string) (time.Time, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts[key], s.hashes[key]
}

func (s *Syncer) markAttempt(key string, t time.Time, hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[key] = t
	s.hashes[key] = hash
}

func (s *Syncer) clearAttempt(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.attempts, key)
	delete(s.hashes, key)
}

func (s *Syncer) setStatus(st Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.Error == "" && st.Refused == 0 && !st.LastRun.IsZero() {
		s.lastSuccess = st.LastRun
	}
	st.LastSuccess = s.lastSuccess
	s.status = st
}

// fetch retrieves and parses one source, returning the sorted, deduplicated CIDR
// set. Every failure mode returns an error, which the caller reads as "keep the
// previous set".
func (s *Syncer) fetch(ctx context.Context, src model.AccessListSource) ([]string, error) {
	u, err := url.Parse(src.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("source url must be an absolute https URL, got %q", src.URL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := s.client.Do(req)
	if err != nil {
		// The *url.Error text repeats the full source URL, whose query may
		// carry a token; the log line and status keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, fmt.Errorf("fetch from %s: %w", u.Host, ue.Err)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("source returned HTTP %d", resp.StatusCode)
	}
	// One byte over the cap so a truncated body is reported as such rather than
	// silently parsed as a short (and therefore narrower) list.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBodyBytes {
		return nil, fmt.Errorf("source body exceeds the %d byte limit", maxBodyBytes)
	}
	entries, err := parseEntries(string(body), src.EntryLimit())
	if err != nil {
		return nil, err
	}
	if err := CheckCoverage(entryNets(entries), src); err != nil {
		return nil, err
	}
	return entries, nil
}

// sourceHost is the host part of a source URL, for log lines: the path and
// query of a feed URL may carry a token.
func sourceHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// entryNets parses parseEntries' normalised CIDR strings.
func entryNets(entries []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(entries))
	for _, e := range entries {
		if _, n, err := net.ParseCIDR(e); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

// parseEntries reads the fixed source format: one IP or CIDR per line, "#"
// comment lines and blank lines ignored, a bare IP read as a /32 or /128. The
// result is normalised to masked CIDR form, deduplicated and sorted, so an
// upstream that merely reorders its list produces no change and therefore no
// commit.
//
// Parsing is STRICT: one line that is neither a comment nor a valid IP/CIDR
// refuses the whole body. A feed that changed shape is a feed gpm no longer
// understands, and quietly keeping the subset it still parses would be a silent,
// partial allow list.
func parseEntries(body string, max int) ([]string, error) {
	seen := map[string]struct{}{}
	out := []string{}
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		t := strings.TrimSpace(sc.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		n := ParseEntry(t)
		if n == nil {
			return nil, fmt.Errorf("line %d: %q is not an IP or CIDR", line, t)
		}
		// Strict, like every other parse failure: one over-broad or non-public
		// network refuses the WHOLE body and keeps the previous set. A feed that
		// has started naming ranges no monitoring provider owns is a feed gpm has
		// no business acting on, however plausible the rest of it looks.
		if err := RefuseEntry(n); err != nil {
			return nil, fmt.Errorf("line %d: %v", line, err)
		}
		c := n.String()
		if _, dup := seen[c]; dup {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
		if len(out) > max {
			return nil, fmt.Errorf("source has more than maxEntries (%d) networks", max)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("source produced no valid entries")
	}
	sort.Strings(out)
	return out, nil
}

// ParseEntry normalises one line to a masked network, or nil when it is neither
// an IP nor a CIDR. A bare IP becomes a /32 or /128.
func ParseEntry(s string) *net.IPNet {
	if _, n, err := net.ParseCIDR(s); err == nil {
		if v4 := n.IP.To4(); v4 != nil && len(n.Mask) == net.IPv6len {
			// An IPv4-mapped prefix (::ffff:a.b.c.d/k) matches IPv4 clients as
			// a.b.c.d/(k-96), so it is judged, and stored, as that network.
			// ParseCIDR has already masked the address, so To4 succeeding means
			// k >= 96 and the last four mask bytes are the IPv4 mask.
			return &net.IPNet{IP: v4, Mask: n.Mask[12:]}
		}
		return n
	}
	if ip := net.ParseIP(s); ip != nil {
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		return &net.IPNet{IP: ip.Mask(net.CIDRMask(bits, bits)), Mask: net.CIDRMask(bits, bits)}
	}
	return nil
}

// diffCounts reports how many entries next adds and drops relative to prev, for
// the change log line.
func diffCounts(prev, next []string) (added, removed int) {
	was := make(map[string]struct{}, len(prev))
	for _, e := range prev {
		was[e] = struct{}{}
	}
	for _, e := range next {
		if _, ok := was[e]; !ok {
			added++
		} else {
			delete(was, e)
		}
	}
	return added, len(was)
}
