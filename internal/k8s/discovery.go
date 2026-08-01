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

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// ErrReconcileInProgress is returned by ReconcileNow when a reconcile is already
// running. The API maps it to 409 Conflict, mirroring the DNS syncer.
var ErrReconcileInProgress = errors.New("ingress discovery: a reconcile is already in progress")

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
	// Reason explains a skip (and only a skip).
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

// Enabled reports whether discovery is configured, for the capability probe. A
// load failure reports disabled rather than guessing.
func (d *Discoverer) Enabled() bool {
	if d == nil || d.load == nil {
		return false
	}
	_, settings, err := d.load(context.Background())
	if err != nil {
		return false
	}
	return settings.IngressDiscovery.Enabled
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
	ingresses, err := client.ListIngresses(ctx)
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
		log.Info().
			Int("created", plan.created).Int("updated", plan.updated).Int("deleted", len(plan.deletes)).
			Str("commit", commit).Msg("ingress discovery: config updated")
		if d.onChange != nil {
			d.onChange(commit)
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

// managedHost reports whether a proxy host is one discovery owns.
func managedHost(h model.ProxyHost) bool {
	return h.Labels[model.ManagedByLabel] == model.ManagedByIngressDiscovery
}

// planReconcile computes the whole reconcile without performing any I/O, which
// is what makes every ownership and freeze rule directly testable.
func planReconcile(cfg model.Config, conf model.IngressDiscoverySettings, ingresses []Ingress) reconcilePlan {
	var p reconcilePlan

	current := map[string]model.ProxyHost{} // every proxy host, by name
	managed := map[string]model.ProxyHost{} // only the ones discovery owns
	for _, h := range cfg.ProxyHosts {
		current[h.Name] = h
		if managedHost(h) {
			managed[h.Name] = h
		}
	}

	desired := map[string]model.ProxyHost{}
	source := map[string]string{}
	// protected holds names that must NOT be deleted even though they are absent
	// from the desired set: their Ingress IS annotated but could not be derived.
	// Deletion follows from absence in a healthy list, never from a parse failure -
	// one bad manifest edit must not take a host offline.
	protected := map[string]bool{}

	for _, ing := range ingresses {
		if ing.Metadata.Annotations[model.AnnotationManaged] != "true" {
			continue // opt-in only: absent or any other value means invisible
		}
		p.discovered++
		ref := ing.Metadata.Namespace + "/" + ing.Metadata.Name
		name, host, err := derive(ing, conf)
		if err != nil {
			if name != "" {
				protected[name] = true
			}
			p.skipped++
			p.results = append(p.results, HostResult{Name: name, Ingress: ref, Action: ActionSkipped, Reason: err.Error()})
			log.Warn().Str("ingress", ref).Err(err).Msg("ingress discovery: skipping annotated Ingress")
			continue
		}
		if prev, dup := source[name]; dup {
			p.skipped++
			p.results = append(p.results, HostResult{Name: name, Ingress: ref, Action: ActionSkipped,
				Reason: "derived name collides with Ingress " + prev})
			log.Warn().Str("ingress", ref).Str("other", prev).Msg("ingress discovery: two Ingresses derive the same host name")
			continue
		}
		desired[name] = host
		source[name] = ref
	}

	for _, name := range sortedKeys(desired) {
		want := desired[name]
		cur, exists := current[name]
		switch {
		case exists && !managedHost(cur):
			// Somebody hand-wrote a host with this name. Overwriting it is exactly
			// what the ownership rule forbids, so skip and say so - the same
			// skip-and-warn the Pi-hole and Cloudflare backends do for a record they
			// do not own.
			p.skipped++
			p.results = append(p.results, HostResult{Name: name, Ingress: source[name], Domains: want.Domains,
				Action: ActionSkipped, Reason: "a proxy host with this name exists and is not managed by ingress discovery"})
			log.Warn().Str("host", name).Str("ingress", source[name]).
				Msg("ingress discovery: name is taken by an operator-authored proxy host; leaving it alone")
		case !exists:
			p.upserts = append(p.upserts, want)
			p.created++
			p.results = append(p.results, HostResult{Name: name, Ingress: source[name], Domains: want.Domains, Action: ActionCreated})
		default:
			// Carry the original creation timestamp so an update does not rewrite it.
			want.CreatedAt = cur.CreatedAt
			if sameHost(cur, want) {
				p.results = append(p.results, HostResult{Name: name, Ingress: source[name], Domains: want.Domains, Action: ActionUnchanged})
				continue
			}
			p.upserts = append(p.upserts, want)
			p.updated++
			p.results = append(p.results, HostResult{Name: name, Ingress: source[name], Domains: want.Domains, Action: ActionUpdated})
		}
	}

	for _, name := range sortedKeys(managed) {
		if _, want := desired[name]; want || protected[name] {
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

// derive builds the proxy host for one annotated Ingress. It returns the derived
// name even on failure when the name itself was computable, so the caller can
// protect an existing host from deletion (see planReconcile's protected set).
//
// EVERYTHING security-relevant comes from the template. The Ingress contributes
// exactly two things: hostnames (strictly validated and suffix-restricted) and
// two DNS booleans. It can never supply an upstream, a certificate, a middleware
// or an access list, so a cluster user who can edit an Ingress can never weaken
// the chain the gpm operator configured, nor aim gpm at an address of their
// choosing.
func derive(ing Ingress, conf model.IngressDiscoverySettings) (string, model.ProxyHost, error) {
	ns, nm := ing.Metadata.Namespace, ing.Metadata.Name
	if ns == "" || nm == "" {
		return "", model.ProxyHost{}, errors.New("Ingress has no namespace/name")
	}
	// "<name>.<namespace>": a dot separator is unambiguous because a Kubernetes
	// namespace is a DNS-1123 label (no dots) while a name may contain "-".
	name := "ing-" + nm + "." + ns
	if err := model.ValidateName(name); err != nil {
		return "", model.ProxyHost{}, fmt.Errorf("derived name %q is not usable: %w", name, err)
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
		if !conf.AllowedDomain(h) {
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
		return name, model.ProxyHost{}, errors.New(reason)
	}
	// Sorted so the derived object does not churn when the API returns rules in a
	// different order.
	sort.Strings(domains)

	for _, t := range ing.Spec.TLS {
		for _, h := range t.Hosts {
			if !seen[model.NormalizeHostname(h)] {
				log.Debug().Str("ingress", ns+"/"+nm).Str("tlsHost", h).
					Msg("ingress discovery: spec.tls names a host the rules do not; gpm takes its certificate from the template, not from the Ingress")
			}
		}
	}

	tmpl := conf.Template
	host := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{
			Name:        name,
			DisplayName: ns + "/" + nm,
			Labels:      map[string]string{model.ManagedByLabel: model.ManagedByIngressDiscovery},
		},
		Domains:           domains,
		Upstream:          tmpl.Upstream,
		TLS:               tmpl.TLS,
		WebsocketsUpgrade: tmpl.WebsocketsUpgrade,
		Middlewares:       append([]string(nil), tmpl.Middlewares...),
		AccessLists:       append([]string(nil), tmpl.AccessLists...),
	}
	if pol := dnsPolicy(ing, conf); pol != nil {
		host.DNS = pol
	}
	return name, host, nil
}

// dnsPolicy resolves the derived host's DNS policy: the template default, with
// each flag individually overridden by its annotation. A policy that asks for
// nothing is nil, so an opted-out host carries no dns key at all.
func dnsPolicy(ing Ingress, conf model.IngressDiscoverySettings) *model.DNSSyncPolicy {
	pol := model.DNSSyncPolicy{}
	if conf.Template.DefaultDNS != nil {
		pol = *conf.Template.DefaultDNS
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
	apply(model.AnnotationLanDirect, &pol.LanDirect)
	apply(model.AnnotationPublicCname, &pol.PublicCname)
	if !pol.Enabled() {
		return nil
	}
	return &pol
}
