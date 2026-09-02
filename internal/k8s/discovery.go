package k8s

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Rake-Pro/go-proxy-manager/internal/discovery"
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

// Per-host outcomes reported in Status.Hosts. They are the shared planner's
// (internal/discovery), re-exported here so this package's callers and tests do
// not have to know where the reconcile engine lives.
const (
	ActionCreated   = discovery.ActionCreated
	ActionUpdated   = discovery.ActionUpdated
	ActionUnchanged = discovery.ActionUnchanged
	ActionDeleted   = discovery.ActionDeleted
	ActionSkipped   = discovery.ActionSkipped
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
// upsert and every delete in one commit. See docs/design/ingress-discovery.md section 2
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

	// onFreeze, if set (via SetOnFreeze), is called whenever a reconcile fails
	// at or past the freeze boundary - see fail() - for optional alerting
	// (internal/notify.EventDiscoveryFrozen). Best-effort, never touches
	// discovery state itself.
	onFreeze func(err error)

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

// SetOnFreeze installs the callback fired whenever a reconcile fails (see
// fail()), for optional notify wiring. A setter rather than a New parameter
// so it does not disturb the existing constructor signature; nil detaches it.
// Safe to call at any time.
func (d *Discoverer) SetOnFreeze(fn func(err error)) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.onFreeze = fn
	d.mu.Unlock()
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
	onFreeze := d.onFreeze
	d.mu.Unlock()
	if onFreeze != nil {
		onFreeze(err)
	}
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

// ownership projects the discovery settings onto the shared planner's view of
// which proxy hosts this reconciler owns and how to name itself in
// operator-facing text. The label VALUE ("ingress-discovery") is what keeps it
// from ever seeing - let alone deleting - a host owned by Docker discovery,
// which stamps the same key with "docker-discovery".
func ownership(conf model.IngressDiscoverySettings) discovery.Ownership {
	return discovery.Ownership{
		Subsystem:        "ingress discovery",
		SourceKind:       "Ingress",
		ManagedByKey:     conf.ManagedByLabel(),
		DisabledByKey:    conf.DisabledByLabel(),
		Value:            model.ManagedByIngressDiscovery,
		Migrate:          conf.AnnotationPrefixMigrate,
		HasStaleManaged:  conf.HasStaleManagedByLabel,
		HasStaleDisabled: conf.HasStaleDisabledByLabel,
		StripStale:       conf.StripStaleDiscoveryLabels,
	}
}

// planReconcile derives every annotated Ingress and hands the derived set to
// the shared full-state planner (internal/discovery), which owns every
// ownership, freeze and carry-forward rule. Everything above the handover is
// Kubernetes-specific: the opt-in annotation gate, the derived name, and what a
// derive failure means. Everything below it is shared with Docker discovery, so
// the rules with security consequences exist exactly once.
func planReconcile(cfg model.Config, conf model.IngressDiscoverySettings, ingresses []Ingress) reconcilePlan {
	items := make([]discovery.Item, 0, len(ingresses))
	for _, ing := range ingresses {
		if ing.Metadata.Annotations[conf.AnnotationManaged()] != "true" {
			continue // opt-in only: absent or any other value means invisible
		}
		name, host, prof, err := derive(ing, conf)
		items = append(items, discovery.Item{
			Ref:            ing.Metadata.Namespace + "/" + ing.Metadata.Name,
			Name:           name,
			Host:           host,
			Profile:        prof,
			Err:            err,
			UnknownProfile: isUnknownProfile(err),
		})
	}

	res := discovery.Plan(cfg, ownership(conf), items)
	p := reconcilePlan{
		upserts:      res.Upserts,
		deletes:      res.Deletes,
		discovered:   res.Discovered,
		created:      res.Created,
		updated:      res.Updated,
		skipped:      res.Skipped,
		managedAfter: res.ManagedAfter,
		results:      make([]HostResult, 0, len(res.Hosts)),
	}
	// The shared planner speaks of a generic source "ref"; this package's status
	// payload has always called it "ingress", and that is a published wire shape.
	for _, h := range res.Hosts {
		p.results = append(p.results, HostResult{
			Name:    h.Name,
			Ingress: h.Ref,
			Domains: h.Domains,
			Action:  h.Action,
			Profile: h.Profile,
			Reason:  h.Reason,
		})
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
		Domains:          domains,
		Upstream:         tmpl.Upstream,
		UpstreamGroupRef: tmpl.UpstreamGroupRef,
		TLS:              tlsSettings,
		//lint:ignore SA1019 compat copy of deprecated WebsocketsUpgrade so a pre-deprecation template still produces byte-identical derived hosts
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
