package k8s

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// ErrReconcileInProgress is returned by ReconcileNow when a reconcile is already
// running. The API maps it to 409 Conflict, mirroring the DNS syncer.
var ErrReconcileInProgress = errors.New("ingress discovery: a reconcile is already in progress")

// listDeadline bounds ONE reconcile's list, pagination included. Without it the
// per-request timeout multiplied by the page limit is the real bound (tens of
// minutes), during which the reconcile mutex is held and every manual reconcile
// and poll queues behind it. It is a var only so the tests can shorten it.
var listDeadline = 2 * time.Minute

// Per-host outcomes reported in Status.Hosts.
const (
	ActionCreated   = "created"
	ActionUpdated   = "updated"
	ActionUnchanged = "unchanged"
	ActionDeleted   = "deleted"
	ActionSkipped   = "skipped"
)

// HostResult is what one reconcile decided about one derived (or previously
// derived) proxy host.
type HostResult struct {
	// Name is the derived proxy-host name, e.g. "ing-grafana.monitoring".
	Name string `json:"name"`
	// Ingress is the source object as "<namespace>/<name>", empty for a delete
	// whose Ingress is already gone.
	Ingress string   `json:"ingress,omitempty"`
	Domains []string `json:"domains,omitempty"`
	Action  string   `json:"action"`
	// Profile is the settings.ingressDiscovery profile the host resolved to -
	// "template" for the default block, otherwise the profiles key. On a skip
	// caused by an unknown profile it is the REQUESTED name, so the operator can
	// see what the Ingress asked for. Empty for a delete (no Ingress is left to
	// ask). This is the audit trail for "what chain did that host actually get".
	Profile string `json:"profile,omitempty"`
	// Reason explains a skip, and the one update that is not a normal derive: a
	// host disabled because its profile no longer resolves.
	Reason string `json:"reason,omitempty"`
}

// Status is the result of the last reconcile, served by
// GET /ingress-discovery/status.
//
// LastRun and LastSuccess are separate on purpose: a UI that shows only "last
// run 2 minutes ago" would hide "last SUCCESSFUL run six hours ago", which is
// exactly the state freeze-on-error produces.
type Status struct {
	Enabled     bool      `json:"enabled"`
	LastRun     time.Time `json:"lastRun,omitempty"`
	LastSuccess time.Time `json:"lastSuccess,omitempty"`
	Error       string    `json:"error,omitempty"`

	// Discovered is how many annotated Ingresses the last successful list held;
	// Managed is how many gpm-managed proxy hosts exist after the run.
	Discovered int `json:"discovered"`
	Managed    int `json:"managed"`
	Created    int `json:"created"`
	Updated    int `json:"updated"`
	Deleted    int `json:"deleted"`
	Skipped    int `json:"skipped"`

	// Commit is the config revision this run wrote, empty when it changed nothing.
	Commit string       `json:"commit,omitempty"`
	Hosts  []HostResult `json:"hosts"`
}

// Applier commits one reconcile's worth of changes as a SINGLE revision: every
// upsert and every delete in one commit. See docs/design/ingress-discovery.md §2
// for why granularity is per-reconcile rather than per-object. It returns the
// commit hash, or "" when there was nothing to write.
type Applier func(ctx context.Context, upserts []model.ProxyHost, deletes []string, message string) (string, error)

// Discoverer polls the Kubernetes API for annotated Ingresses and reconciles
// them into managed proxy hosts.
type Discoverer struct {
	load  func(context.Context) (model.Config, model.Settings, error)
	apply Applier
	// onChange is called with the new commit after a reconcile that actually
	// committed something, so the daemon can reload the running config, fire
	// lifecycle webhooks and ask the DNS syncer for a run. Discovery deliberately
	// publishes no DNS records itself - there is one DNS code path, and it is
	// phase 1's.
	onChange func(commit string)

	// newClient is the client constructor, swapped by the tests. Production
	// always uses NewClient.
	newClient func(ClientConfig) (*Client, error)

	mu     sync.Mutex
	status Status
	// enabled caches settings.ingressDiscovery.enabled so the capability probe
	// (hit on every SPA page load) does not have to take the store's read lock and
	// validate the whole config graph behind an in-flight reconcile commit. It is
	// refreshed by every reconcile and every interval read, so it lags a settings
	// change by at most one poll.
	enabled      bool
	enabledKnown bool

	// single serialises reconciles so two runs never plan against each other.
	single sync.Mutex

	// clientMu guards the cached client, which is rebuilt only when the
	// connection settings change (so the token cache and its TTL survive polls).
	clientMu  sync.Mutex
	client    *Client
	clientKey string
}

// New returns a Discoverer. load is called on every reconcile so a settings
// change takes effect without re-wiring (the DNS syncer's live-config pattern).
// apply performs the batched write; onChange may be nil.
func New(load func(context.Context) (model.Config, model.Settings, error), apply Applier, onChange func(commit string)) *Discoverer {
	return &Discoverer{load: load, apply: apply, onChange: onChange, newClient: NewClient}
}

// Status returns a snapshot of the last reconcile result.
func (d *Discoverer) Status() Status {
	if d == nil {
		return Status{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

// Enabled reports whether discovery is configured, for the capability probe. It
// answers from the cached flag once anything has read settings; only the very
// first call (before the first poll) loads, so the probe cannot become a config
// load per admin page view. A load failure reports disabled rather than guessing.
func (d *Discoverer) Enabled() bool {
	if d == nil || d.load == nil {
		return false
	}
	d.mu.Lock()
	if d.enabledKnown {
		v := d.enabled
		d.mu.Unlock()
		return v
	}
	d.mu.Unlock()

	_, settings, err := d.load(context.Background())
	if err != nil {
		return false
	}
	d.setEnabled(settings.IngressDiscovery.Enabled)
	return settings.IngressDiscovery.Enabled
}

func (d *Discoverer) setEnabled(v bool) {
	d.mu.Lock()
	d.enabled, d.enabledKnown = v, true
	d.mu.Unlock()
}

// Run polls until ctx is cancelled, reconciling on the configured interval. The
// interval is re-read from settings every iteration, so enabling, disabling or
// re-pointing discovery needs no restart.
func (d *Discoverer) Run(ctx context.Context) {
	if d == nil || d.load == nil {
		return
	}
	for {
		if err := d.Reconcile(ctx); err != nil && ctx.Err() == nil {
			log.Warn().Err(err).Msg("ingress discovery: reconcile failed")
		}
		t := time.NewTimer(d.interval(ctx))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

func (d *Discoverer) interval(ctx context.Context) time.Duration {
	_, settings, err := d.load(ctx)
	if err != nil {
		return time.Minute
	}
	d.setEnabled(settings.IngressDiscovery.Enabled)
	return settings.IngressDiscovery.Interval()
}

// Reconcile runs one full-state reconcile, waiting for any in-flight run.
func (d *Discoverer) Reconcile(ctx context.Context) error {
	if d == nil || d.load == nil {
		return fmt.Errorf("ingress discovery: no configuration source wired")
	}
	d.single.Lock()
	defer d.single.Unlock()
	return d.reconcileLocked(ctx)
}

// ReconcileNow is the HTTP-triggered variant: it refuses with
// ErrReconcileInProgress rather than queueing behind a run already in flight, so
// repeated clicks cannot pile request-scoped goroutines up behind a slow API
// server (the same reasoning as dnssync.ReconcileNow).
func (d *Discoverer) ReconcileNow(ctx context.Context) error {
	if d == nil || d.load == nil {
		return fmt.Errorf("ingress discovery: no configuration source wired")
	}
	if !d.single.TryLock() {
		return ErrReconcileInProgress
	}
	defer d.single.Unlock()
	return d.reconcileLocked(ctx)
}

// Plan is the read-only preview served by GET /ingress-discovery/plan. It
// mirrors dnssync.Plan (see internal/dnssync/dnssync.go): the same per-host
// decisions Reconcile would take, computed by listing the cluster and running
// planReconcile, without ever calling Applier.
type Plan struct {
	GeneratedAt time.Time    `json:"generatedAt"`
	Enabled     bool         `json:"enabled"`
	Error       string       `json:"error,omitempty"`
	Discovered  int          `json:"discovered"`
	Managed     int          `json:"managed"`
	Created     int          `json:"created"`
	Updated     int          `json:"updated"`
	Deleted     int          `json:"deleted"`
	Skipped     int          `json:"skipped"`
	Hosts       []HostResult `json:"hosts"`
}

// Plan computes what Reconcile WOULD do, without changing anything: it reads
// the config, lists the cluster and runs the exact same planReconcile
// Reconcile does, but never applies it. Like ReconcileNow it refuses rather
// than queue behind a run already in flight - a preview of a moving target is
// worth less than an honest 409.
func (d *Discoverer) Plan(ctx context.Context) (Plan, error) {
	if d == nil || d.load == nil {
		return Plan{}, fmt.Errorf("ingress discovery: no configuration source wired")
	}
	if !d.single.TryLock() {
		return Plan{}, ErrReconcileInProgress
	}
	defer d.single.Unlock()

	p := Plan{GeneratedAt: time.Now().UTC()}
	cfg, settings, err := d.load(ctx)
	if err != nil {
		return p, fmt.Errorf("ingress discovery: load config: %w", err)
	}
	conf := settings.IngressDiscovery
	p.Enabled = conf.Enabled
	if !conf.Enabled {
		return p, nil
	}
	if err := conf.Validate(); err != nil {
		return p, fmt.Errorf("ingress discovery: %w", err)
	}
	client, err := d.clientFor(conf)
	if err != nil {
		return p, fmt.Errorf("ingress discovery: %w", err)
	}
	listCtx, cancelList := context.WithTimeout(ctx, listDeadline)
	ingresses, err := client.ListIngresses(listCtx)
	cancelList()
	if err != nil {
		return p, fmt.Errorf("ingress discovery: list ingresses: %w", err)
	}
	plan := planReconcile(cfg, conf, ingresses)
	p.Discovered = plan.discovered
	p.Managed = plan.managedAfter
	p.Created = plan.created
	p.Updated = plan.updated
	p.Deleted = len(plan.deletes)
	p.Skipped = plan.skipped
	p.Hosts = plan.results
	return p, nil
}

// fail records a run that could not complete and returns the error. It keeps
// LastSuccess from the previous good run: freezing means the managed hosts are
// untouched, and the status has to say how stale that state is.
func (d *Discoverer) fail(now time.Time, enabled bool, err error) error {
	d.mu.Lock()
	prev := d.status
	d.status = Status{
		Enabled:     enabled,
		LastRun:     now,
		LastSuccess: prev.LastSuccess,
		Error:       err.Error(),
		Discovered:  prev.Discovered,
		Managed:     prev.Managed,
		Hosts:       prev.Hosts,
	}
	d.mu.Unlock()
	return err
}

func (d *Discoverer) reconcileLocked(ctx context.Context) error {
	now := time.Now().UTC()

	cfg, settings, err := d.load(ctx)
	if err != nil {
		return d.fail(now, false, fmt.Errorf("ingress discovery: load config: %w", err))
	}
	conf := settings.IngressDiscovery
	d.setEnabled(conf.Enabled)
	if !conf.Enabled {
		d.mu.Lock()
		d.status = Status{Enabled: false, LastRun: now, Hosts: []HostResult{}}
		d.mu.Unlock()
		return nil
	}
	if err := conf.Validate(); err != nil {
		return d.fail(now, true, fmt.Errorf("ingress discovery: %w", err))
	}
	client, err := d.clientFor(conf)
	if err != nil {
		return d.fail(now, true, fmt.Errorf("ingress discovery: %w", err))
	}

	// The freeze boundary. Any failure here - transport, status, decode, a page
	// that failed mid-pagination - returns WITHOUT items, and no write of any
	// kind happens below. A managed host is only ever deleted on the strength of
	// a complete, successful list.
	listCtx, cancelList := context.WithTimeout(ctx, listDeadline)
	ingresses, err := client.ListIngresses(listCtx)
	cancelList()
	if err != nil {
		return d.fail(now, true, fmt.Errorf("ingress discovery: list ingresses: %w", err))
	}

	plan := planReconcile(cfg, conf, ingresses)

	commit := ""
	if len(plan.upserts) > 0 || len(plan.deletes) > 0 {
		msg := fmt.Sprintf("Ingress discovery: reconcile (+%d ~%d -%d)", plan.created, plan.updated, len(plan.deletes))
		if d.apply == nil {
			return d.fail(now, true, errors.New("ingress discovery: no config writer wired"))
		}
		if commit, err = d.apply(ctx, plan.upserts, plan.deletes, msg); err != nil {
			return d.fail(now, true, fmt.Errorf("ingress discovery: apply: %w", err))
		}
		// An empty commit means the writer found nothing left to do (every planned
		// delete was already gone, or an ownership re-check under the store lock
		// dropped the whole batch). Nothing changed on disk, so firing the reload,
		// webhook and DNS trigger would be a lie with an empty revision attached.
		if commit != "" {
			log.Info().
				Int("created", plan.created).Int("updated", plan.updated).Int("deleted", len(plan.deletes)).
				Str("commit", commit).Msg("ingress discovery: config updated")
			if d.onChange != nil {
				d.onChange(commit)
			}
		}
	}

	d.mu.Lock()
	d.status = Status{
		Enabled:     true,
		LastRun:     now,
		LastSuccess: now,
		Discovered:  plan.discovered,
		Managed:     plan.managedAfter,
		Created:     plan.created,
		Updated:     plan.updated,
		Deleted:     len(plan.deletes),
		Skipped:     plan.skipped,
		Commit:      commit,
		Hosts:       plan.results,
	}
	d.mu.Unlock()
	return nil
}

// clientFor returns the cached client, rebuilding it when the connection
// settings changed. Caching matters: the client owns the bearer-token TTL, and
// rebuilding per poll would re-read the token file every time (and re-read the
// CA bundle with it).
func (d *Discoverer) clientFor(conf model.IngressDiscoverySettings) (*Client, error) {
	key := strings.Join([]string{conf.APIURL, conf.TokenFile, conf.CAFile, conf.Namespace, conf.LabelSelector}, "\x00")
	d.clientMu.Lock()
	defer d.clientMu.Unlock()
	if d.client != nil && d.clientKey == key {
		return d.client, nil
	}
	c, err := d.newClient(ClientConfig{
		APIURL:        conf.APIURL,
		TokenFile:     conf.TokenFile,
		CAFile:        conf.CAFile,
		Namespace:     conf.Namespace,
		LabelSelector: conf.LabelSelector,
	})
	if err != nil {
		return nil, err
	}
	d.client, d.clientKey = c, key
	return c, nil
}

// reconcilePlan is what one reconcile decided, before anything is written.
type reconcilePlan struct {
	upserts []model.ProxyHost
	deletes []string

	discovered   int
	created      int
	updated      int
	skipped      int
	managedAfter int
	results      []HostResult
}

// managedHost reports whether a proxy host is one discovery owns: it carries
// managed-by under conf's CURRENT annotation prefix. When
// conf.AnnotationPrefixMigrate is set, a host still carrying managed-by under
// a DIFFERENT (older) prefix is ALSO recognised, so it is treated as owned for
// this run rather than skipped as hand-written - which is what lets its normal
// derive/update path (in planReconcile below) relabel it onto the current
// prefix, since derive() always stamps a fresh Labels map under the current
// prefix.
func managedHost(h model.ProxyHost, conf model.IngressDiscoverySettings) bool {
	if h.Labels[conf.ManagedByLabel()] == model.ManagedByIngressDiscovery {
		return true
	}
	return conf.AnnotationPrefixMigrate && conf.HasStaleManagedByLabel(h.Labels)
}

// operatorDisabled reports whether a managed host's Disabled: true was set by
// the OPERATOR rather than by discovery's own fail-closed revocation path (see
// IngressDiscoverySettings.DisabledByLabel). Discovery must never re-enable a
// host it did not disable itself - that would turn a hand-disable, the obvious
// move when an app has to come offline now, into a no-op on the very next
// poll. During an AnnotationPrefixMigrate relabel, a disabled-by label under a
// stale prefix is ALSO recognised as discovery's own, for the same reason
// managedHost widens above.
func operatorDisabled(cur model.ProxyHost, conf model.IngressDiscoverySettings) bool {
	if !cur.Disabled {
		return false
	}
	if cur.Labels[conf.DisabledByLabel()] == model.DisabledByIngressDiscovery {
		return false
	}
	if conf.AnnotationPrefixMigrate && conf.HasStaleDisabledByLabel(cur.Labels) {
		return false
	}
	return true
}

// cloneLabels copies a label map so a write through the copy can never mutate
// the map the caller's ProxyHost still shares (Go maps are reference types, and
// `off := cur` only shallow-copies the struct).
func cloneLabels(m map[string]string) map[string]string {
	out := make(map[string]string, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// planReconcile computes the whole reconcile without performing any I/O, which
// is what makes every ownership and freeze rule directly testable.
func planReconcile(cfg model.Config, conf model.IngressDiscoverySettings, ingresses []Ingress) reconcilePlan {
	var p reconcilePlan

	current := map[string]model.ProxyHost{} // every proxy host, by name
	managed := map[string]model.ProxyHost{} // only the ones discovery owns
	for _, h := range cfg.ProxyHosts {
		current[h.Name] = h
		if managedHost(h, conf) {
			managed[h.Name] = h
		}
	}

	desired := map[string]model.ProxyHost{}
	source := map[string]string{}
	// profile records which settings profile each desired host resolved to, so
	// the status can say what chain a host actually got.
	profile := map[string]string{}
	// protected holds names that must NOT be deleted even though they are absent
	// from the desired set: their Ingress IS annotated but could not be derived.
	// Deletion follows from absence in a healthy list, never from a parse failure -
	// one bad manifest edit must not take a host offline.
	protected := map[string]bool{}
	// disable holds managed hosts this run must FAIL CLOSED on: their Ingress
	// names a profile that no longer resolves, so the chain they are still serving
	// is one nobody can point at any more (see the switch below).
	disable := map[string]model.ProxyHost{}

	for _, ing := range ingresses {
		if ing.Metadata.Annotations[conf.AnnotationManaged()] != "true" {
			continue // opt-in only: absent or any other value means invisible
		}
		p.discovered++
		ref := ing.Metadata.Namespace + "/" + ing.Metadata.Name
		name, host, prof, err := derive(ing, conf)
		if err != nil {
			if name != "" {
				protected[name] = true
			}
			// Two derive failures, two opposite safe answers.
			//
			// A MALFORMED Ingress (bad hostname, unusable derived name) is a tenant
			// typo against a chain the operator still sanctions: the host on disk is
			// the last good rendering of a policy that has not changed, so freezing it
			// keeps a working service up while the manifest is fixed. Failing closed
			// there would let any tenant take their own service offline with a
			// one-character edit, and would do nothing for security.
			//
			// An UNRESOLVABLE PROFILE is the opposite: the chain that host is serving
			// is one the operator has just tightened, renamed or retired, or one the
			// tenant is pointing away from. Freezing would let a tenant pin a revoked
			// chain forever simply by flipping the annotation to a name that does not
			// exist - the security property would hold for creating a host but not for
			// REVOKING one. So the host is disabled instead: the object is preserved
			// (nothing is destroyed, the operator can re-add the profile and the very
			// next reconcile re-enables it), but it stops serving the revoked chain.
			if cur, ok := managed[name]; ok && isUnknownProfile(err) && !cur.Disabled {
				off := cur
				off.Disabled = true
				// Mark the disable as DISCOVERY's own, so the next reconcile that
				// resolves cleanly is free to clear it - an operator's own disable
				// (no label, or a different one) never gets this treatment. off.Labels
				// aliases cur.Labels (off := cur is a shallow copy), so it is cloned
				// before being written to, or this would mutate cur - and, through it,
				// the config the caller still holds - out from under the read.
				off.Labels = cloneLabels(cur.Labels)
				// During an AnnotationPrefixMigrate relabel, cur may only carry a
				// STALE prefix's labels (that is how it got into managed[] above); strip
				// them so the host ends up with exactly the current prefix's pair,
				// rather than old and new keys both lingering.
				if conf.AnnotationPrefixMigrate {
					conf.StripStaleDiscoveryLabels(off.Labels)
				}
				off.Labels[conf.ManagedByLabel()] = model.ManagedByIngressDiscovery
				off.Labels[conf.DisabledByLabel()] = model.DisabledByIngressDiscovery
				disable[name] = off
				p.updated++
				p.results = append(p.results, HostResult{Name: name, Ingress: ref, Domains: cur.Domains,
					Action: ActionUpdated, Profile: prof,
					Reason: "disabled (fails closed rather than keep serving a chain that no longer resolves): " + err.Error()})
				log.Warn().Str("host", name).Str("ingress", ref).Err(err).
					Msg("ingress discovery: profile no longer resolves; disabling the derived host rather than leaving it serving the old chain")
				continue
			}
			p.skipped++
			p.results = append(p.results, HostResult{Name: name, Ingress: ref, Action: ActionSkipped, Profile: prof, Reason: err.Error()})
			log.Warn().Str("ingress", ref).Err(err).Msg("ingress discovery: skipping annotated Ingress")
			continue
		}
		if prev, dup := source[name]; dup {
			p.skipped++
			p.results = append(p.results, HostResult{Name: name, Ingress: ref, Action: ActionSkipped, Profile: prof,
				Reason: "derived name collides with Ingress " + prev})
			log.Warn().Str("ingress", ref).Str("other", prev).Msg("ingress discovery: two Ingresses derive the same host name")
			continue
		}
		desired[name] = host
		source[name] = ref
		profile[name] = prof
	}

	// Which managed hosts this run would remove. Needed before the domain gate
	// below: a host on its way out must not keep claiming its domains, or a
	// renamed Ingress could never hand its hostname over.
	doomed := map[string]bool{}
	for name := range managed {
		if _, want := desired[name]; !want && !protected[name] {
			doomed[name] = true
		}
	}

	// The DOMAIN ownership gate. Ownership of the derived NAME is not enough:
	// hosts are routed by domain, and the router's per-domain maps are filled in
	// config load order, so a derived host whose name sorts late would silently
	// take over an operator host's hostname (and its TLS/mTLS pinning with it)
	// without ever colliding on a name. Every domain already claimed by a host
	// this reconcile is not rewriting is off limits; a derived host that wants one
	// is skipped and reported, exactly like a name collision.
	claimed := map[string]string{}
	claim := func(owner string, domains []string) {
		for _, dom := range domains {
			key := domainKey(dom)
			if key == "" {
				continue
			}
			if _, taken := claimed[key]; !taken {
				claimed[key] = owner
			}
		}
	}
	for _, h := range cfg.ProxyHosts {
		// A managed host that this run rewrites or removes releases its domains;
		// everything else - every operator-authored host, and every managed host
		// being kept as-is - holds on to them.
		if managedHost(h, conf) {
			if _, rewritten := desired[h.Name]; rewritten || doomed[h.Name] {
				continue
			}
		}
		claim(h.Name, h.Domains)
	}
	for _, h := range cfg.RedirectHosts {
		claim(h.Name, h.Domains)
	}
	for _, h := range cfg.ParkedHosts {
		claim(h.Name, h.Domains)
	}

	for _, name := range sortedKeys(desired) {
		want := desired[name]
		cur, exists := current[name]
		// A skipped host leaves whatever is on disk in place, so it must re-assert
		// that object's domains: otherwise a later-sorted derived host could claim
		// one and produce a duplicate the config validator would reject, failing the
		// whole batch.
		skip := func(reason string) {
			p.skipped++
			p.results = append(p.results, HostResult{Name: name, Ingress: source[name], Domains: want.Domains,
				Action: ActionSkipped, Profile: profile[name], Reason: reason})
			if exists {
				claim(name, cur.Domains)
			}
		}
		if conflict, owner := firstClaimed(want.Domains, claimed); conflict != "" {
			skip(fmt.Sprintf("domain %q is already claimed by proxy host %q, which ingress discovery does not own", conflict, owner))
			log.Warn().Str("host", name).Str("ingress", source[name]).
				Str("domain", conflict).Str("owner", owner).
				Msg("ingress discovery: an existing host already serves this domain; refusing to shadow it")
			continue
		}
		switch {
		case exists && !managedHost(cur, conf):
			// Somebody hand-wrote a host with this name. Overwriting it is exactly
			// what the ownership rule forbids, so skip and say so - the same
			// skip-and-warn the Pi-hole and Cloudflare backends do for a record they
			// do not own.
			skip("a proxy host with this name exists and is not managed by ingress discovery")
			log.Warn().Str("host", name).Str("ingress", source[name]).
				Msg("ingress discovery: name is taken by an operator-authored proxy host; leaving it alone")
		case !exists:
			claim(name, want.Domains)
			p.upserts = append(p.upserts, want)
			p.created++
			p.results = append(p.results, HostResult{Name: name, Ingress: source[name], Domains: want.Domains, Action: ActionCreated, Profile: profile[name]})
		default:
			claim(name, want.Domains)
			// Carry the original creation timestamp so an update does not rewrite it.
			want.CreatedAt = cur.CreatedAt
			// disabled: true is OPERATOR-owned state once discovery did not set it
			// itself: a hand-disabled host must not be re-enabled by the next poll
			// just because its Ingress still derives cleanly. A host discovery
			// disabled (gpm.rake.pro/disabled-by: ingress-discovery) is exempt - that
			// disable is discovery's own fail-closed hold, and a clean derive is
			// exactly the signal that lifts it (want.Disabled is already false here,
			// and want carries no disabled-by label, so it clears on its own).
			reenable := false
			if operatorDisabled(cur, conf) {
				want.Disabled = true
			} else if cur.Disabled {
				reenable = true
			}
			if sameHost(cur, want) {
				p.results = append(p.results, HostResult{Name: name, Ingress: source[name], Domains: want.Domains, Action: ActionUnchanged, Profile: profile[name]})
				continue
			}
			p.upserts = append(p.upserts, want)
			p.updated++
			reason := ""
			switch {
			case operatorDisabled(cur, conf):
				reason = "operator-disabled host: other fields refreshed, disabled state preserved"
			case reenable:
				reason = "profile resolves again: re-enabling the host discovery had disabled"
			}
			p.results = append(p.results, HostResult{Name: name, Ingress: source[name], Domains: want.Domains, Action: ActionUpdated, Profile: profile[name], Reason: reason})
		}
	}

	// The fail-closed writes. They are plain upserts of an existing managed object
	// with disabled: true, so they go through the same ownership guard and the same
	// single commit as everything else. Their domains were already re-asserted by
	// the claim loop above (the host is neither rewritten nor doomed), so a
	// later-sorted derived host cannot pick up a hostname a disabled host is
	// holding - the disable is a hold, not a handover.
	for _, name := range sortedKeys(disable) {
		p.upserts = append(p.upserts, disable[name])
	}

	for _, name := range sortedKeys(managed) {
		if !doomed[name] {
			continue
		}
		p.deletes = append(p.deletes, name)
		p.results = append(p.results, HostResult{Name: name, Domains: managed[name].Domains, Action: ActionDeleted})
		log.Warn().Str("host", name).Msg("ingress discovery: no annotated Ingress derives this managed host any more; removing it")
	}

	p.managedAfter = len(managed) + p.created - len(p.deletes)
	if p.results == nil {
		p.results = []HostResult{}
	}
	return p
}

// ellipsize bounds an untrusted string before it is echoed into a status
// payload or a log line. It cuts on a rune boundary so a truncated multi-byte
// sequence cannot produce invalid UTF-8 in the JSON response.
func ellipsize(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// domainKey normalises a configured domain for comparison, matching the key the
// config validator and the router's per-domain maps use.
func domainKey(d string) string { return strings.ToLower(strings.TrimSpace(d)) }

// firstClaimed returns the first of domains already claimed by another host, and
// that host's name, or ("", "") when none is.
func firstClaimed(domains []string, claimed map[string]string) (string, string) {
	for _, d := range domains {
		if owner, taken := claimed[domainKey(d)]; taken {
			return domainKey(d), owner
		}
	}
	return "", ""
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sameHost compares a stored host with a freshly derived one, ignoring the
// store-maintained timestamps, so a steady-state reconcile writes nothing.
func sameHost(a, b model.ProxyHost) bool {
	a.CreatedAt, a.UpdatedAt = time.Time{}, time.Time{}
	b.CreatedAt, b.UpdatedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(a, b)
}

// unknownProfileError marks the one derive failure planReconcile treats
// differently from every other: the named profile does not resolve. It is a
// distinct type rather than a string match so the wording of the operator-facing
// message can change without silently changing the fail-closed behaviour.
type unknownProfileError struct{ err error }

func (e unknownProfileError) Error() string { return e.err.Error() }
func (e unknownProfileError) Unwrap() error { return e.err }

// isUnknownProfile reports whether err came from profile resolution.
func isUnknownProfile(err error) bool {
	var u unknownProfileError
	return errors.As(err, &u)
}

// derive builds the proxy host for one annotated Ingress, returning the derived
// name, the host, and the profile it resolved to. It returns the derived name
// even on failure when the name itself was computable, so the caller can protect
// an existing host from deletion (see planReconcile's protected set).
//
// EVERYTHING security-relevant comes from an operator-defined profile. The
// Ingress contributes exactly three things: hostnames (strictly validated and
// suffix-restricted), two DNS booleans, and the NAME of one profile. It can
// never supply an upstream, a certificate, a middleware or an access list, and
// it cannot describe a chain - only pick one the operator already wrote down. So
// a cluster user who can edit an Ingress can never weaken the chain the gpm
// operator configured, nor aim gpm at an address of their choosing.
func derive(ing Ingress, conf model.IngressDiscoverySettings) (string, model.ProxyHost, string, error) {
	ns, nm := ing.Metadata.Namespace, ing.Metadata.Name
	if ns == "" || nm == "" {
		return "", model.ProxyHost{}, "", errors.New("Ingress has no namespace/name")
	}
	// "<name>.<namespace>": a dot separator is unambiguous only while the
	// namespace is a DNS-1123 label (no dots). The API server enforces that, but
	// the derived name is an ownership boundary, so gpm checks it here rather than
	// trusting whatever answered the LIST. No name is returned: an ambiguous one
	// must not protect an existing host from deletion.
	if !model.IsDNSLabel(ns) {
		return "", model.ProxyHost{}, "", fmt.Errorf("namespace %q is not a DNS-1123 label", ns)
	}
	name := "ing-" + nm + "." + ns
	if err := model.ValidateName(name); err != nil {
		return "", model.ProxyHost{}, "", fmt.Errorf("derived name %q is not usable: %w", name, err)
	}

	// Profile selection, before anything else is derived. An unknown name is a
	// SKIP, never a fall back to the default: an Ingress that asked for
	// "public-ratelimited" and silently received the default's home-vpn access
	// list (or, the other way round, lost its rate limit) is a security-relevant
	// regression that nobody would see. The name is returned so the caller
	// protects any existing host for this Ingress from deletion - a typo in an
	// annotation must not take a host offline either.
	tmpl, prof, ok := conf.ResolveProfileFor(ns, ing.Metadata.Labels, ing.Metadata.Annotations[conf.AnnotationProfile()])
	if !ok {
		// The rejected name is echoed back so the operator can see the typo, but it
		// is cluster-supplied and an annotation value can be very large, so it is
		// truncated before it reaches the status payload and the log.
		prof = ellipsize(prof, 64)
		log.Warn().Str("ingress", ns+"/"+nm).Str("profile", prof).Strs("defined", conf.ProfileNames()).
			Msg("ingress discovery: Ingress names an undefined discovery profile; skipping rather than applying the default chain")
		return name, model.ProxyHost{}, prof, unknownProfileError{fmt.Errorf(
			"profile %q is not defined in settings.ingressDiscovery.profiles (defined: %s); "+
				"refusing to fall back to the default template", prof, strings.Join(conf.ProfileNames(), ", "))}
	}

	var domains []string
	var rejected []string
	seen := map[string]bool{}
	for _, rule := range ing.Spec.Rules {
		h := model.NormalizeHostname(rule.Host)
		if h == "" {
			continue
		}
		if !model.IsHostname(h) {
			rejected = append(rejected, rule.Host+" (not a valid hostname; wildcards are not supported)")
			continue
		}
		if !conf.AllowedDomainFor(tmpl, h) {
			rejected = append(rejected, h+" (outside allowedDomainSuffixes)")
			continue
		}
		if seen[h] {
			continue
		}
		seen[h] = true
		domains = append(domains, h)
	}
	if len(rejected) > 0 {
		log.Warn().Str("ingress", ns+"/"+nm).Strs("rejected", rejected).
			Msg("ingress discovery: rejecting host(s) from an Ingress")
	}
	if len(domains) == 0 {
		reason := "no usable host in spec.rules"
		if len(rejected) > 0 {
			reason += ": " + strings.Join(rejected, ", ")
		}
		return name, model.ProxyHost{}, prof, errors.New(reason)
	}
	// Sorted so the derived object does not churn when the API returns rules in a
	// different order.
	sort.Strings(domains)

	for _, t := range ing.Spec.TLS {
		for _, h := range t.Hosts {
			if !seen[model.NormalizeHostname(h)] {
				log.Debug().Str("ingress", ns+"/"+nm).Str("tlsHost", h).
					Msg("ingress discovery: spec.tls names a host the rules do not; gpm takes its certificate from the profile, not from the Ingress")
			}
		}
	}

	// TLSSettings is a value, but ClientAuth inside it is a POINTER: assigning
	// tmpl.TLS verbatim would hand every derived host - and the loaded settings
	// object they were derived from - the same *ClientAuth. Middlewares,
	// AccessLists and DefaultDNS are already copied for the same reason; this
	// closes the last aliasing hole, so no writer of one host's mTLS requirement
	// can reach any other host's.
	tlsSettings := tmpl.TLS
	if tlsSettings.ClientAuth != nil {
		ca := *tlsSettings.ClientAuth
		tlsSettings.ClientAuth = &ca
	}

	// Timeouts is a POINTER for the same reason: assigning it straight through
	// would give every derived host - and the settings object they came from - the
	// same *HostTimeouts. nil stays nil so an unset template produces a host with
	// no timeouts key at all.
	var timeouts *model.HostTimeouts
	if tmpl.Timeouts != nil {
		to := *tmpl.Timeouts
		timeouts = &to
	}

	host := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{
			Name:        name,
			DisplayName: ns + "/" + nm,
			Labels:      map[string]string{conf.ManagedByLabel(): model.ManagedByIngressDiscovery},
			Tags:        append([]string(nil), tmpl.Tags...),
		},
		Domains:           domains,
		Upstream:          tmpl.Upstream,
		UpstreamGroupRef:  tmpl.UpstreamGroupRef,
		TLS:               tlsSettings,
		WebsocketsUpgrade: tmpl.WebsocketsUpgrade,
		RobotsNoIndex:     tmpl.RobotsNoIndex,
		Timeouts:          timeouts,
		Middlewares:       append([]string(nil), tmpl.Middlewares...),
		AccessLists:       append([]string(nil), tmpl.AccessLists...),
	}
	// Set out of the literal (like the dns policy below) so an unset template
	// leaves the field nil rather than widening every other field's alignment.
	if len(tmpl.StripResponseHeaders) > 0 {
		host.StripResponseHeaders = append([]string(nil), tmpl.StripResponseHeaders...)
	}
	if pol := dnsPolicy(ing, tmpl, conf); pol != nil {
		host.DNS = pol
	}
	return name, host, prof, nil
}

// dnsPolicy resolves the derived host's DNS policy: the resolved profile's
// default, with each flag individually overridden by its annotation. A policy
// that asks for nothing is nil, so an opted-out host carries no dns key at all.
func dnsPolicy(ing Ingress, tmpl model.IngressHostTemplate, conf model.IngressDiscoverySettings) *model.DNSSyncPolicy {
	pol := model.DNSSyncPolicy{}
	if tmpl.DefaultDNS != nil {
		pol = *tmpl.DefaultDNS
	}
	apply := func(key string, dst *bool) {
		raw, ok := ing.Metadata.Annotations[key]
		if !ok {
			return
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true":
			*dst = true
		case "false":
			*dst = false
		default:
			log.Warn().Str("ingress", ing.Metadata.Namespace+"/"+ing.Metadata.Name).
				Str("annotation", key).Str("value", raw).
				Msg(`ingress discovery: annotation value must be "true" or "false"; keeping the template default`)
		}
	}
	apply(conf.AnnotationLanDirect(), &pol.LanDirect)
	apply(conf.AnnotationPublicCname(), &pol.PublicCname)
	if !pol.Enabled() {
		return nil
	}
	return &pol
}
