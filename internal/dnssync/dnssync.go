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
//   - Deletion is limited to records gpm CREATED ITSELF, recorded explicitly in
//     the git-backed ownership ledger (model.DNSLedger). A record that is not in
//     the ledger is never deleted, whatever its name and whatever it points at.
//
// The ledger replaces an inference that was not ownership. Pi-hole ownership used
// to be "the CNAME target equals apexTarget", because dnsmasq CNAMEs have no
// comment field to mark. On 2026-08-01 an operator enabled the Pi-hole backend for
// the first time with 19 hand-written LAN CNAMEs already pointing at that same
// edge host and no proxy host yet carrying dns.lanDirect: the desired set was
// empty, every one of those records looked "managed", and the first reconcile
// deleted the lot. LAN DNS broke until they were restored by hand.
//
// So ownership is now recorded, not guessed, and the first reconcile of an
// existing deployment is an adopt-only run:
//
//   - desired, absent            -> create, and record ownership
//   - desired, present, right target, not owned -> ADOPT (record ownership, do
//     not recreate)
//   - desired, present, wrong target, not owned -> skip and warn, never overwrite
//   - CREATED by gpm, no longer desired -> delete, and drop from the ledger
//   - ADOPTED by gpm, no longer desired -> RELEASE: drop from the ledger and warn,
//     never delete. Adoption is a claim on a record somebody else made, so it must
//     not become a licence to destroy it: gpm deletes only what it created.
//   - ADOPTED by gpm, still desired, apexTarget since changed -> RELEASE as well.
//     A retarget is a delete plus a create, so retargeting an adopted record would
//     both destroy the operator's record and re-record it as gpm-created, leaving a
//     later host removal free to hard-delete it. Retarget applies only to records
//     gpm created itself.
//   - not owned                  -> untouched, always
//
// Cloudflare keeps its "managed-by:gpm" record comment as a second, independent
// mark: a delete there needs BOTH the ledger entry and the comment.
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
	// records gpm owns (ledger entries) once the run finished; Created, Adopted,
	// Retargeted and Deleted count the changes this run made; Skipped counts
	// desired names gpm left alone - held by a record it does not own, or held by
	// one it had adopted and released when the apex moved.
	Desired    int `json:"desired"`
	Managed    int `json:"managed"`
	Created    int `json:"created"`
	Adopted    int `json:"adopted"`
	Retargeted int `json:"retargeted"`
	Deleted    int `json:"deleted"`
	Skipped    int `json:"skipped"`
	// Untouched is how many records the backend holds that gpm does not own and
	// therefore never considered changing. It is the number the operator wants
	// after a first-enable: it should equal everything they wrote by hand.
	Untouched int `json:"untouched"`
}

// Status is the full result of the last reconcile, served by GET /dns-sync/status.
type Status struct {
	LastRun    time.Time     `json:"lastRun,omitempty"`
	Error      string        `json:"error,omitempty"`
	Pihole     BackendStatus `json:"pihole"`
	Cloudflare BackendStatus `json:"cloudflare"`
}

// BackendPlan is what a reconcile WOULD do to one backend, computed without
// changing anything. Every list holds domain names, sorted, so a plan is
// diffable against the next one.
type BackendPlan struct {
	Enabled bool   `json:"enabled"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	// Create is desired names with no record; Adopt is desired names that already
	// hold the right record but are not in the ledger yet (first-enable
	// migration); Retarget is names gpm CREATED whose record still carries the
	// target gpm published it with while the configured apex has since changed;
	// Delete is gpm-created names the config no longer wants; Skip is desired names
	// gpm will not write, either because a record it does not own holds the name or
	// because the claim it held was adopted and the apex has moved (released).
	Create   []string `json:"create"`
	Adopt    []string `json:"adopt"`
	Retarget []string `json:"retarget"`
	Delete   []string `json:"delete"`
	Skip     []string `json:"skip"`
	// Untouched counts records in the backend gpm does not own and will not
	// consider. On a first enable this is the count an operator should recognise
	// as "everything I wrote by hand".
	Untouched int `json:"untouched"`
}

// Plan is the read-only preview served by GET /dns-sync/plan.
type Plan struct {
	GeneratedAt time.Time   `json:"generatedAt"`
	Error       string      `json:"error,omitempty"`
	Pihole      BackendPlan `json:"pihole"`
	Cloudflare  BackendPlan `json:"cloudflare"`
}

// decisions is the backend-independent outcome of comparing the desired set, the
// records a backend holds and the ownership ledger. Both backends compute one and
// then either render it (Plan) or apply it (Reconcile), so a preview can never
// disagree with the run it previews.
type decisions struct {
	create    []string
	adopt     []string
	retarget  []string
	del       []string
	skip      []string
	untouched int
	// owned is the ledger state the run would end with if every step succeeded.
	owned map[string]model.DNSClaim
}

func (d decisions) plan() BackendPlan {
	return BackendPlan{
		OK:        true,
		Create:    d.create,
		Adopt:     d.adopt,
		Retarget:  d.retarget,
		Delete:    d.del,
		Skip:      d.skip,
		Untouched: d.untouched,
	}
}

// Ledger persists the record-ownership ledger across restarts. It is an
// interface so the syncer depends on the two operations it needs rather than on
// the config store, and so tests can supply an in-memory one.
//
// A nil Ledger puts the syncer in a deliberately inert mode: it owns nothing, so
// it creates and adopts but NEVER deletes. That is the safe direction to fail.
//
// Load also returns an opaque revision for the state it read, and Save is handed
// that revision back. A reconcile is a read-modify-write of a file another writer
// (a config revert) can rewrite in between, and the revision is what lets the
// store refuse the write rather than silently re-establishing claims the revert
// withdrew. An implementation that cannot version its state may return "".
type Ledger interface {
	Load(ctx context.Context) (model.DNSLedger, string, error)
	Save(ctx context.Context, l model.DNSLedger, rev string) error
}

// Syncer reconciles DNS records for the opted-in proxy hosts.
type Syncer struct {
	load   func(context.Context) (model.Config, model.Settings, error)
	ledger Ledger
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
// ledger is the ownership store that decides what may be deleted; passing nil
// means gpm owns nothing and deletes nothing (see Ledger).
//
// The HTTP client is hardened the same way the webhook client is: redirects are
// never followed, and link-local destinations are refused at connect time
// (post-DNS) so an admin-configured Pi-hole URL cannot be pointed at a cloud
// metadata service. Private ranges stay allowed - a LAN Pi-hole is the point.
func New(load func(context.Context) (model.Config, model.Settings, error), ledger Ledger) *Syncer {
	return &Syncer{
		load:   load,
		ledger: ledger,
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

// Plan computes what a reconcile WOULD do, without changing anything: it reads
// the config, the ledger and each enabled backend's records, and returns the same
// decisions Reconcile would take. Nothing is created, adopted or deleted, and the
// ledger is not written.
//
// It exists so enabling a backend is previewable. The 2026-08-01 incident was
// unpreviewable: the only way to find out what the first reconcile would do was
// to run it, and by then 19 records were gone.
//
// Like ReconcileNow it refuses rather than queues behind a run in flight - a
// preview of a moving target is worth less than an honest 409, and Pi-hole has
// few session slots to spend on overlapping reads.
func (s *Syncer) Plan(ctx context.Context) (Plan, error) {
	if s == nil || s.load == nil {
		return Plan{}, fmt.Errorf("dnssync: no configuration source wired")
	}
	if !s.single.TryLock() {
		return Plan{}, ErrReconcileInProgress
	}
	defer s.single.Unlock()

	p := Plan{GeneratedAt: time.Now().UTC()}
	cfg, settings, err := s.load(ctx)
	if err != nil {
		return p, fmt.Errorf("dnssync: load config: %w", err)
	}
	ledger, _, err := s.loadLedger(ctx)
	if err != nil {
		return p, err
	}
	if c := settings.DNSSync.Pihole; c.Enabled {
		p.Pihole = s.planPihole(ctx, cfg, c, model.DNSLedgerMap(ledger.Pihole))
		p.Pihole.Enabled = true
	}
	if c := settings.DNSSync.Cloudflare; c.Enabled {
		p.Cloudflare = s.planCloudflare(ctx, cfg, c, model.DNSLedgerMap(ledger.Cloudflare))
		p.Cloudflare.Enabled = true
	}
	return p, nil
}

// loadLedger reads the ownership ledger, or returns an empty one when no ledger
// store is wired. A read failure is fatal to the run: continuing with an empty
// ledger would be safe for deletion (nothing is owned, so nothing is deleted) but
// would re-adopt and re-create records on every run, so it is reported instead.
func (s *Syncer) loadLedger(ctx context.Context) (model.DNSLedger, string, error) {
	if s.ledger == nil {
		return model.DNSLedger{}, "", nil
	}
	l, rev, err := s.ledger.Load(ctx)
	if err != nil {
		return model.DNSLedger{}, "", fmt.Errorf("dnssync: load ownership ledger: %w", err)
	}
	return l, rev, nil
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
	ledger, rev, err := s.loadLedger(ctx)
	if err != nil {
		// Refuse to touch any backend without knowing what gpm owns.
		s.mu.Lock()
		s.status = Status{LastRun: now, Error: err.Error()}
		s.mu.Unlock()
		return err
	}

	st := Status{LastRun: now}
	next := ledger
	// Each backend returns the ledger state it actually reached, even on a partial
	// failure, so a record created just before an error is still recorded as owned.
	// A disabled backend's entries are left exactly as they are: turning a backend
	// off must not silently disown the records it published.
	if p := settings.DNSSync.Pihole; p.Enabled {
		var owned map[string]model.DNSClaim
		st.Pihole, owned = s.syncPihole(ctx, cfg, p, model.DNSLedgerMap(ledger.Pihole), rev)
		st.Pihole.Enabled = true
		st.Pihole.LastRun = now
		next.Pihole = model.DNSLedgerEntries(owned)
	}
	if c := settings.DNSSync.Cloudflare; c.Enabled {
		var owned map[string]model.DNSClaim
		st.Cloudflare, owned = s.syncCloudflare(ctx, cfg, c, model.DNSLedgerMap(ledger.Cloudflare), rev)
		st.Cloudflare.Enabled = true
		st.Cloudflare.LastRun = now
		next.Cloudflare = model.DNSLedgerEntries(owned)
	}
	if s.ledger != nil && !ledgerEqual(ledger, next) {
		if err := s.saveLedger(ctx, ledger, next, rev); err != nil {
			// The records are already changed at the backend; failing to persist that
			// is serious (a later run would not know it owns them), so it is surfaced
			// in status rather than swallowed.
			st.Error = fmt.Sprintf("dns records reconciled but the ownership ledger could not be saved: %v", err)
			log.Error().Err(err).Msg("dnssync: could not persist the DNS ownership ledger")
		}
	}
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
	return nil
}

// ledgerEqual reports whether two ledgers hold the same entries, so a reconcile
// that changed nothing does not write (and commit) an identical file.
func ledgerEqual(a, b model.DNSLedger) bool {
	return model.DNSLedgerEntriesEqual(a.Pihole, b.Pihole) &&
		model.DNSLedgerEntriesEqual(a.Cloudflare, b.Cloudflare)
}

// saveLedger persists the ledger this run ended with. A reconcile is a
// read-modify-write over a file a concurrent config REVERT can also rewrite, so
// the store is handed the revision the ledger was read at and refuses the write
// if the config repo has moved since. On that refusal the current ledger is
// re-read and the write is retried ONCE, with the concurrent writer's
// withdrawals honoured: a claim the revert removed stays removed, and a claim it
// restored is not resurrected. Both of those are the direction that cannot
// delete a record - a claim gpm drops only means it stops managing a record, a
// claim it re-establishes is a licence to delete one.
func (s *Syncer) saveLedger(ctx context.Context, base, next model.DNSLedger, rev string) error {
	err := s.ledger.Save(ctx, next, rev)
	if err == nil {
		return nil
	}
	current, curRev, loadErr := s.ledger.Load(ctx)
	if loadErr != nil || curRev == rev {
		// Not a staleness problem (or the ledger cannot be re-read): report the
		// original failure rather than retrying into the same wall.
		return err
	}
	merged := model.DNSLedger{
		SchemaVersion: next.SchemaVersion,
		Pihole:        mergeLedgerEntries(base.Pihole, current.Pihole, next.Pihole),
		Cloudflare:    mergeLedgerEntries(base.Cloudflare, current.Cloudflare, next.Cloudflare),
	}
	log.Warn().Err(err).Str("readAt", rev).Str("now", curRev).
		Msg("dnssync: the ownership ledger changed under this reconcile (a config revert?); re-writing it without the claims that were withdrawn")
	return s.ledger.Save(ctx, merged, curRev)
}

// mergeLedgerEntries returns next minus every claim that was withdrawn between
// base (what this run read) and current (what is on disk now). Claims that only
// appear in current are deliberately NOT carried over: an entry gpm did not put
// there this run is a restored claim, and re-adopting one is how a revert
// resurrects the authority to delete a record an operator has since recreated.
func mergeLedgerEntries(base, current, next []model.DNSLedgerEntry) []model.DNSLedgerEntry {
	was := model.DNSLedgerMap(base)
	now := model.DNSLedgerMap(current)
	out := make([]model.DNSLedgerEntry, 0, len(next))
	for _, e := range next {
		key := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(e.Domain), "."))
		if _, known := was[key]; known {
			if _, kept := now[key]; !kept {
				continue // withdrawn while this run was in flight
			}
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// decide compares the desired set against what a backend actually holds and
// against the ownership ledger, and returns what to do. It is the single place
// the create/adopt/retarget/delete/skip rules live: both backends call it, and so
// does the dry-run planner, so a preview can never disagree with the reconcile it
// previews.
//
// present maps every record name the backend holds to its CNAME target. owned is
// the ledger for this backend (name -> the claim gpm recorded: the target it
// published, and whether the claim came from adoption). mark reports whether the
// backend's own secondary ownership marker is on the record - Cloudflare's
// "managed-by:gpm" comment, and unconditionally true on Pi-hole, whose dnsmasq
// CNAMEs have nowhere to put one. That asymmetry is the whole reason the ledger
// exists.
//
// Three invariants every branch below preserves: a name absent from owned is
// NEVER put in the delete list, whatever it points at; a name gpm ADOPTED rather
// than created is never put in the delete OR the retarget list (both destroy the
// record, one of them permanently) - it is released instead; and a claim's
// provenance is carried forward untouched, never quietly upgraded from adopted to
// created. The single exception to that last one is the !exists branch, where the
// adopted record has gone and gpm really does create the one that replaces it.
func decide(backend string, desired []string, present map[string]string, apex string, owned map[string]model.DNSClaim, mark func(name string) bool) decisions {
	d := decisions{owned: make(map[string]model.DNSClaim, len(owned))}
	for k, v := range owned {
		d.owned[k] = v
	}
	want := make(map[string]bool, len(desired))
	for _, name := range desired {
		want[name] = true
	}

	for _, name := range desired {
		cur, exists := present[name]
		claim, isOwned := owned[name]
		switch {
		case !exists:
			// Either brand new, or one that has been removed out of band. A full-state
			// reconcile (re)creates it either way, and this is the ONE place a claim
			// may go from adopted to created: the adopted record is gone, so the
			// record that will stand here afterwards is genuinely one gpm made, and
			// deleting that later destroys nothing but gpm's own write.
			d.create = append(d.create, name)
			d.owned[name] = model.DNSClaim{Target: apex}
		case cur == apex && isOwned:
			// Already exactly right, and already ours. Nothing to do beyond keeping
			// the recorded target honest; the provenance is carried over untouched.
			d.owned[name] = model.DNSClaim{Target: apex, Adopted: claim.Adopted}
		case cur == apex && mark(name):
			// ADOPTION. The record the config asks for is already there, carrying
			// whatever mark this backend can hold, but predates the ledger. Claim it
			// instead of recreating it - and, above all, instead of deleting it. This
			// is what makes enabling a backend on an existing deployment a no-op. The
			// claim is recorded AS an adoption, so gpm can manage the record without
			// ever acquiring the right to destroy it.
			d.adopt = append(d.adopt, name)
			d.owned[name] = model.DNSClaim{Target: apex, Adopted: true}
		case isOwned && claim.Adopted:
			// Ours only by ADOPTION - the operator wrote this record, gpm merely
			// claimed it - and the apex has moved out from under it. Retargeting would
			// DELETE an operator-authored record and record its replacement as
			// gpm-created, so a later host removal would then hard-delete the name for
			// good: adopt -> apex change -> remove host is the 2026-08-01 incident in
			// slow motion. The claim is RELEASED instead: dropped from the ledger,
			// nothing touched in the backend. Publishing the name under the new apex
			// needs the operator to re-point or remove their own record first. The same
			// applies if they have re-pointed it themselves: either way the record is
			// theirs and gpm's only correct move is to stop claiming it.
			d.skip = append(d.skip, name)
			delete(d.owned, name)
			log.Warn().Str("backend", backend).Str("domain", name).Str("target", cur).Str("apex", apex).
				Msg("dnssync: a record gpm adopted rather than created no longer matches the configured apex; releasing the claim and leaving the record exactly as it stands")
		case isOwned && !claim.Adopted && cur == claim.Target && mark(name):
			// Ours because gpm CREATED it, and it still holds exactly what gpm wrote,
			// but the configured apex has moved since. Replacing it is safe precisely
			// because it is unchanged and gpm's own - it destroys nothing an operator
			// authored - and what stands there afterwards is again a record gpm created.
			d.retarget = append(d.retarget, name)
			d.owned[name] = model.DNSClaim{Target: apex}
		default:
			// Somebody else's record on a name gpm also wants. Adding a second entry
			// would shadow a deliberate one and removing theirs is exactly what the
			// ownership rule forbids, so gpm reports it and does neither.
			d.skip = append(d.skip, name)
			log.Warn().Str("backend", backend).Str("domain", name).Str("target", cur).
				Msg("dnssync: the name is held by a record gpm does not own; leaving it alone")
			if isOwned {
				delete(d.owned, name)
			}
		}
	}

	// Removals: only ever names the ledger says gpm created, and only when the
	// record still is the one gpm left behind.
	var stale []string
	for name := range owned {
		if !want[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		cur, exists := present[name]
		switch {
		case !exists:
			// Already gone (deleted out of band): stop claiming it.
			delete(d.owned, name)
		case cur != owned[name].Target || !mark(name):
			// It no longer holds what gpm wrote, so an operator has taken it over.
			// Disown it rather than delete it - being wrong here is how records get
			// lost.
			log.Warn().Str("backend", backend).Str("domain", name).Str("target", cur).
				Msg("dnssync: a record gpm owned has changed out of band; disowning it instead of deleting it")
			delete(d.owned, name)
		case owned[name].Adopted:
			// gpm never created this record, it only claimed one that was already
			// there and already correct. Losing interest in a name is not authority to
			// destroy somebody else's record, so the claim is RELEASED and the record
			// left standing. Without this, adoption would be a one-way trap: turn on
			// dns.lanDirect for a name an operator had hand-written, turn it off
			// again, and the next reconcile would delete their record.
			log.Warn().Str("backend", backend).Str("domain", name).Str("target", cur).
				Msg("dnssync: releasing a record gpm adopted but no longer manages; it was not created by gpm, so it is left in place rather than deleted")
			delete(d.owned, name)
		default:
			d.del = append(d.del, name)
			delete(d.owned, name)
		}
	}

	// Untouched: records the backend holds that this run neither owns nor changes.
	// It is the number that matters after a first enable - it should be everything
	// the operator wrote by hand.
	touched := make(map[string]bool, len(d.del))
	for _, name := range d.del {
		touched[name] = true
	}
	for name := range present {
		if _, ours := d.owned[name]; !ours && !touched[name] {
			d.untouched++
		}
	}
	return d
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
