package docker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Rake-Pro/go-proxy-manager/internal/discovery"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// ErrReconcileInProgress is returned by ReconcileNow and Plan when a reconcile
// is already running. The API maps it to 409 Conflict, mirroring the DNS syncer
// and the Ingress reconciler.
var ErrReconcileInProgress = errors.New("docker discovery: a reconcile is already in progress")

// listDeadline bounds ONE reconcile's list. Without it the reconcile mutex can
// be held for the client timeout while every manual reconcile and poll queues
// behind it. It is a var only so the tests can shorten it.
var listDeadline = 30 * time.Second

// eventDebounce is how long an event burst is allowed to settle before the
// reconcile it triggered runs. `docker compose up` on a ten-service stack emits
// a dozen events in under a second; without this each one would be its own
// reconcile, and each reconcile is a config commit.
var eventDebounce = 2 * time.Second

// eventRetry is the backoff between event-stream reconnects. The stream is a
// latency optimisation, so a permanently unavailable one costs nothing but the
// poll interval.
var eventRetry = 15 * time.Second

// Per-host outcomes, re-exported from the shared planner.
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
	// Name is the derived proxy-host name, e.g. "dkr-grafana".
	Name string `json:"name"`
	// Container is the source container's name, empty for a delete whose
	// container is already gone.
	Container string   `json:"container,omitempty"`
	Domains   []string `json:"domains,omitempty"`
	Action    string   `json:"action"`
	// Profile is the profile the host resolved to - "template" for the default
	// block, otherwise the profiles key. On a skip caused by an unknown profile
	// it is the REQUESTED name, so the operator can see what the container asked
	// for.
	Profile string `json:"profile,omitempty"`
	// Reason explains a skip, and the two updates that are not a plain derive.
	Reason string `json:"reason,omitempty"`
}

// Status is the result of the last reconcile, served by
// GET /docker-discovery/status.
//
// LastRun and LastSuccess are separate on purpose: a UI that shows only "last
// run 2 minutes ago" would hide "last SUCCESSFUL run six hours ago", which is
// exactly the state freeze-on-error produces.
type Status struct {
	Enabled     bool      `json:"enabled"`
	LastRun     time.Time `json:"lastRun,omitempty"`
	LastSuccess time.Time `json:"lastSuccess,omitempty"`
	Error       string    `json:"error,omitempty"`
	// Reachable reports whether the last run could talk to the Engine endpoint at
	// all, so "the socket is not mounted" is distinguishable from "the socket is
	// fine and one container is misconfigured".
	Reachable bool `json:"reachable"`
	// Endpoint is the socket path or host URL the last run used, so the status
	// panel can say WHICH endpoint is unreachable.
	Endpoint string `json:"endpoint,omitempty"`

	// Discovered is how many opted-in containers the last successful list held;
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

// Plan is the read-only preview served by GET /docker-discovery/plan.
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

// Applier commits one reconcile's worth of changes as a SINGLE revision: every
// upsert and every delete in one commit, exactly like the Ingress reconciler's.
// It returns the commit hash, or "" when there was nothing to write.
type Applier func(ctx context.Context, upserts []model.ProxyHost, deletes []string, message string) (string, error)

// Discoverer watches the Docker Engine for labelled containers and reconciles
// them into managed proxy hosts.
type Discoverer struct {
	load     func(context.Context) (model.Config, model.Settings, error)
	apply    Applier
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
	// enabled caches settings.dockerDiscovery.enabled so the capability probe
	// (hit on every SPA page load) does not have to take the store's read lock.
	enabled      bool
	enabledKnown bool

	// single serialises reconciles so two runs never plan against each other.
	single sync.Mutex

	// clientMu guards the cached client, which is rebuilt only when the
	// connection settings change (so the negotiated API version survives polls).
	clientMu  sync.Mutex
	client    *Client
	clientKey string
}

// New returns a Discoverer. load is called on every reconcile so a settings
// change takes effect without re-wiring; apply performs the batched write;
// onChange may be nil.
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

// Enabled reports whether container discovery is configured, for the capability
// probe. A load failure reports disabled rather than guessing.
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
	on := settings.DockerDiscoveryResolved().Enabled
	d.setEnabled(on)
	return on
}

func (d *Discoverer) setEnabled(v bool) {
	d.mu.Lock()
	d.enabled, d.enabledKnown = v, true
	d.mu.Unlock()
}

// Run reconciles until ctx is cancelled: on every Engine container event (with
// a short debounce) and, as the fallback that makes correctness independent of
// the stream, on the configured poll interval.
func (d *Discoverer) Run(ctx context.Context) {
	if d == nil || d.load == nil {
		return
	}
	events := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.watch(ctx, events)
	}()
	defer wg.Wait()

	for {
		if err := d.Reconcile(ctx); err != nil && ctx.Err() == nil {
			log.Warn().Err(err).Msg("docker discovery: reconcile failed")
		}
		if !d.wait(ctx, events) {
			return
		}
	}
}

// wait blocks until the poll interval elapses or an event burst settles. It
// returns false when ctx is done.
func (d *Discoverer) wait(ctx context.Context, events <-chan struct{}) bool {
	t := time.NewTimer(d.interval(ctx))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	case <-events:
	}
	// Debounce: `docker compose up` on a ten-service stack emits a burst, and
	// each reconcile is a config commit. Wait for quiet before running.
	db := time.NewTimer(eventDebounce)
	defer db.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-events:
			if !db.Stop() {
				<-db.C
			}
			db.Reset(eventDebounce)
		case <-db.C:
			return true
		}
	}
}

// watch keeps an Engine event stream open, pushing a (coalesced) signal onto
// events for each container lifecycle event. It is best-effort by design: every
// failure path just falls back to the poll.
func (d *Discoverer) watch(ctx context.Context, events chan<- struct{}) {
	notify := func() {
		select {
		case events <- struct{}{}:
		default: // a signal is already pending; one reconcile covers both
		}
	}
	for ctx.Err() == nil {
		conf, ok := d.enabledConf(ctx)
		if !ok {
			sleep(ctx, eventRetry)
			continue
		}
		client, err := d.clientFor(conf)
		if err != nil {
			log.Debug().Err(err).Msg("docker discovery: event watch cannot connect; falling back to the poll interval")
			sleep(ctx, eventRetry)
			continue
		}
		if err := client.WatchEvents(ctx, notify); err != nil && ctx.Err() == nil {
			log.Debug().Err(err).Msg("docker discovery: event stream ended; retrying")
		}
		sleep(ctx, eventRetry)
	}
}

// enabledConf loads the resolved settings block, reporting whether discovery is
// enabled and valid enough to connect.
func (d *Discoverer) enabledConf(ctx context.Context) (model.DockerDiscoverySettings, bool) {
	_, settings, err := d.load(ctx)
	if err != nil {
		return model.DockerDiscoverySettings{}, false
	}
	conf := settings.DockerDiscoveryResolved()
	if !conf.Enabled || conf.Validate() != nil {
		return conf, false
	}
	return conf, true
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func (d *Discoverer) interval(ctx context.Context) time.Duration {
	_, settings, err := d.load(ctx)
	if err != nil {
		return time.Minute
	}
	conf := settings.DockerDiscoveryResolved()
	d.setEnabled(conf.Enabled)
	return conf.Interval()
}

// Reconcile runs one full-state reconcile, waiting for any in-flight run.
func (d *Discoverer) Reconcile(ctx context.Context) error {
	if d == nil || d.load == nil {
		return fmt.Errorf("docker discovery: no configuration source wired")
	}
	d.single.Lock()
	defer d.single.Unlock()
	return d.reconcileLocked(ctx)
}

// ReconcileNow is the HTTP-triggered variant: it refuses with
// ErrReconcileInProgress rather than queueing behind a run already in flight.
func (d *Discoverer) ReconcileNow(ctx context.Context) error {
	if d == nil || d.load == nil {
		return fmt.Errorf("docker discovery: no configuration source wired")
	}
	if !d.single.TryLock() {
		return ErrReconcileInProgress
	}
	defer d.single.Unlock()
	return d.reconcileLocked(ctx)
}

// Plan computes what Reconcile WOULD do, without changing anything.
func (d *Discoverer) Plan(ctx context.Context) (Plan, error) {
	if d == nil || d.load == nil {
		return Plan{}, fmt.Errorf("docker discovery: no configuration source wired")
	}
	if !d.single.TryLock() {
		return Plan{}, ErrReconcileInProgress
	}
	defer d.single.Unlock()

	p := Plan{GeneratedAt: time.Now().UTC()}
	cfg, settings, err := d.load(ctx)
	if err != nil {
		return p, fmt.Errorf("docker discovery: load config: %w", err)
	}
	conf := settings.DockerDiscoveryResolved()
	p.Enabled = conf.Enabled
	if !conf.Enabled {
		return p, nil
	}
	if err := conf.Validate(); err != nil {
		return p, fmt.Errorf("docker discovery: %w", err)
	}
	client, err := d.clientFor(conf)
	if err != nil {
		return p, fmt.Errorf("docker discovery: %w", err)
	}
	listCtx, cancel := context.WithTimeout(ctx, listDeadline)
	containers, err := client.ListContainers(listCtx, conf.LabelEnabled(), conf.IncludeStopped)
	cancel()
	if err != nil {
		return p, fmt.Errorf("docker discovery: list containers: %w", err)
	}
	res := planReconcile(cfg, conf, containers)
	p.Discovered = res.Discovered
	p.Managed = res.ManagedAfter
	p.Created = res.Created
	p.Updated = res.Updated
	p.Deleted = len(res.Deletes)
	p.Skipped = res.Skipped
	p.Hosts = hostResults(res)
	return p, nil
}

// fail records a run that could not complete and returns the error. It keeps
// LastSuccess from the previous good run: freezing means the managed hosts are
// untouched, and the status has to say how stale that state is.
func (d *Discoverer) fail(now time.Time, enabled, reachable bool, endpoint string, err error) error {
	d.mu.Lock()
	prev := d.status
	d.status = Status{
		Enabled:     enabled,
		LastRun:     now,
		LastSuccess: prev.LastSuccess,
		Error:       err.Error(),
		Reachable:   reachable,
		Endpoint:    endpoint,
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
		return d.fail(now, false, false, "", fmt.Errorf("docker discovery: load config: %w", err))
	}
	conf := settings.DockerDiscoveryResolved()
	endpoint := endpointOf(conf)
	d.setEnabled(conf.Enabled)
	if !conf.Enabled {
		d.mu.Lock()
		d.status = Status{Enabled: false, LastRun: now, Hosts: []HostResult{}}
		d.mu.Unlock()
		return nil
	}
	if err := conf.Validate(); err != nil {
		return d.fail(now, true, false, endpoint, fmt.Errorf("docker discovery: %w", err))
	}
	client, err := d.clientFor(conf)
	if err != nil {
		return d.fail(now, true, false, endpoint, fmt.Errorf("docker discovery: %w", err))
	}

	// The freeze boundary. Any failure here - transport, status, decode, a body
	// that is not a JSON array - returns WITHOUT items, and no write of any kind
	// happens below. A managed host is only ever deleted on the strength of a
	// complete, successful list.
	listCtx, cancel := context.WithTimeout(ctx, listDeadline)
	containers, err := client.ListContainers(listCtx, conf.LabelEnabled(), conf.IncludeStopped)
	cancel()
	if err != nil {
		return d.fail(now, true, false, endpoint, fmt.Errorf("docker discovery: list containers: %w", err))
	}

	res := planReconcile(cfg, conf, containers)

	commit := ""
	if len(res.Upserts) > 0 || len(res.Deletes) > 0 {
		msg := fmt.Sprintf("Docker discovery: reconcile (+%d ~%d -%d)", res.Created, res.Updated, len(res.Deletes))
		if d.apply == nil {
			return d.fail(now, true, true, endpoint, errors.New("docker discovery: no config writer wired"))
		}
		if commit, err = d.apply(ctx, res.Upserts, res.Deletes, msg); err != nil {
			return d.fail(now, true, true, endpoint, fmt.Errorf("docker discovery: apply: %w", err))
		}
		// An empty commit means the writer found nothing left to do. Nothing
		// changed on disk, so firing the reload, webhook and DNS trigger would be a
		// lie with an empty revision attached.
		if commit != "" {
			log.Info().
				Int("created", res.Created).Int("updated", res.Updated).Int("deleted", len(res.Deletes)).
				Str("commit", commit).Msg("docker discovery: config updated")
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
		Reachable:   true,
		Endpoint:    endpoint,
		Discovered:  res.Discovered,
		Managed:     res.ManagedAfter,
		Created:     res.Created,
		Updated:     res.Updated,
		Deleted:     len(res.Deletes),
		Skipped:     res.Skipped,
		Commit:      commit,
		Hosts:       hostResults(res),
	}
	d.mu.Unlock()
	return nil
}

// endpointOf names the Engine endpoint for the status payload.
func endpointOf(conf model.DockerDiscoverySettings) string {
	if conf.Host != "" {
		return conf.Host
	}
	return conf.SocketPath()
}

// clientFor returns the cached client, rebuilding it when the connection
// settings changed. Caching matters: the client owns the negotiated API
// version, and rebuilding per poll would re-negotiate every time.
func (d *Discoverer) clientFor(conf model.DockerDiscoverySettings) (*Client, error) {
	key := strings.Join([]string{conf.Socket, conf.Host, conf.TLSCert, conf.TLSKey, conf.TLSCA}, "\x00")
	d.clientMu.Lock()
	defer d.clientMu.Unlock()
	if d.client != nil && d.clientKey == key {
		return d.client, nil
	}
	c, err := d.newClient(ClientConfig{
		Socket:  conf.Socket,
		Host:    conf.Host,
		TLSCert: conf.TLSCert,
		TLSKey:  conf.TLSKey,
		TLSCA:   conf.TLSCA,
	})
	if err != nil {
		return nil, err
	}
	d.client, d.clientKey = c, key
	return c, nil
}

// ownership projects the settings onto the shared planner's view of which proxy
// hosts this reconciler owns. The label VALUE ("docker-discovery") is what keeps
// it from ever seeing - let alone deleting - a host derived from an Ingress,
// which stamps the same key with "ingress-discovery".
func ownership(conf model.DockerDiscoverySettings) discovery.Ownership {
	return discovery.Ownership{
		Subsystem:        "docker discovery",
		SourceKind:       "container",
		ManagedByKey:     conf.ManagedByLabel(),
		DisabledByKey:    conf.DisabledByLabel(),
		Value:            model.ManagedByDockerDiscovery,
		Migrate:          conf.PrefixMigrate,
		HasStaleManaged:  conf.HasStaleManagedByLabel,
		HasStaleDisabled: conf.HasStaleDisabledByLabel,
		StripStale:       conf.StripStaleDiscoveryLabels,
	}
}

// planReconcile derives every opted-in container and hands the derived set to
// the shared full-state planner (internal/discovery), which owns every
// ownership, freeze and carry-forward rule. Everything above the handover is
// Docker-specific; everything below it is shared with Ingress discovery, so the
// rules with security consequences exist exactly once.
func planReconcile(cfg model.Config, conf model.DockerDiscoverySettings, containers []Container) discovery.Result {
	items := make([]discovery.Item, 0, len(containers))
	for _, ct := range containers {
		if strings.TrimSpace(ct.Labels[conf.LabelEnabled()]) != "true" {
			// Opt-in only. The Engine already filtered on the label server-side;
			// this re-check is what makes a proxy that ignores filters (or a future
			// filter syntax change) fail closed instead of adopting every container.
			continue
		}
		name, host, prof, err := derive(ct, conf)
		items = append(items, discovery.Item{
			Ref:            ct.Name(),
			Name:           name,
			Host:           host,
			Profile:        prof,
			Err:            err,
			UnknownProfile: isUnknownProfile(err),
		})
	}
	return discovery.Plan(cfg, ownership(conf), items)
}

// hostResults projects the shared planner's generic per-host decisions onto
// this package's wire shape, where the source object is a container.
func hostResults(res discovery.Result) []HostResult {
	out := make([]HostResult, 0, len(res.Hosts))
	for _, h := range res.Hosts {
		out = append(out, HostResult{
			Name:      h.Name,
			Container: h.Ref,
			Domains:   h.Domains,
			Action:    h.Action,
			Profile:   h.Profile,
			Reason:    h.Reason,
		})
	}
	return out
}

// unknownProfileError marks the one derive failure the planner treats
// differently from every other: the named profile does not resolve, which fails
// CLOSED (the existing host is disabled) rather than freezing.
type unknownProfileError struct{ err error }

func (e unknownProfileError) Error() string { return e.err.Error() }
func (e unknownProfileError) Unwrap() error { return e.err }

func isUnknownProfile(err error) bool {
	var u unknownProfileError
	return errors.As(err, &u)
}

// ellipsize bounds an untrusted string before it is echoed into a status
// payload or a log line, cutting on a rune boundary so a truncated multi-byte
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

// derive builds the proxy host for one opted-in container, returning the
// derived name, the host, and the profile it resolved to. It returns the
// derived name even on failure when the name itself was computable, so the
// caller can protect an existing host from deletion.
//
// EVERYTHING security-relevant comes from an operator-defined profile. A
// container's labels contribute exactly four things: hostnames (strictly
// validated and suffix-restricted), a port and scheme WITHIN its own address,
// two DNS booleans, and the NAME of one profile. A label can never supply an
// upstream address of its own choosing, a certificate, a middleware or an
// access list - so whoever can write a compose file can publish their own
// container under a sanctioned chain, but can never weaken that chain or point
// gpm at somebody else's machine.
func derive(ct Container, conf model.DockerDiscoverySettings) (string, model.ProxyHost, string, error) {
	raw := ct.Name()
	if raw == "" {
		return "", model.ProxyHost{}, "", errors.New("container has no name")
	}
	// Docker container names allow uppercase; gpm object names do not, so the
	// name is lowercased before the shape check rather than rejected for a case
	// nobody chose deliberately.
	name := model.DockerHostNamePrefix + strings.ToLower(raw)
	if err := model.ValidateName(name); err != nil {
		return "", model.ProxyHost{}, "", fmt.Errorf("derived name %q is not usable: %w", name, err)
	}

	// Profile selection, before anything else is derived. An unknown name is a
	// skip (and a fail-closed disable for an existing host), never a fall back to
	// the default: a container that asked for "public-ratelimited" and silently
	// received the default's access list - or, the other way round, lost its rate
	// limit - is a security-relevant regression nobody would see.
	tmpl, prof, ok := conf.ResolveProfile(ct.Labels[conf.LabelProfile()])
	if !ok {
		prof = ellipsize(prof, 64)
		log.Warn().Str("container", raw).Str("profile", prof).Strs("defined", conf.ProfileNames()).
			Msg("docker discovery: container names an undefined discovery profile; skipping rather than applying the default chain")
		return name, model.ProxyHost{}, prof, unknownProfileError{fmt.Errorf(
			"profile %q is not defined in settings.dockerDiscovery.profiles (defined: %s); "+
				"refusing to fall back to the default template", prof, strings.Join(conf.ProfileNames(), ", "))}
	}

	domains, err := deriveDomains(ct, conf, tmpl)
	if err != nil {
		return name, model.ProxyHost{}, prof, err
	}
	upstream, err := deriveUpstream(ct, conf)
	if err != nil {
		return name, model.ProxyHost{}, prof, err
	}
	// TLSSettings is a value, but ClientAuth inside it is a POINTER: assigning
	// tmpl.TLS verbatim would hand every derived host - and the loaded settings
	// object they were derived from - the same *ClientAuth.
	tlsSettings := tmpl.TLS
	if tlsSettings.ClientAuth != nil {
		ca := *tlsSettings.ClientAuth
		tlsSettings.ClientAuth = &ca
	}
	var timeouts *model.HostTimeouts
	if tmpl.Timeouts != nil {
		to := *tmpl.Timeouts
		timeouts = &to
	}

	host := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{
			Name:        name,
			DisplayName: raw,
			Labels:      map[string]string{conf.ManagedByLabel(): model.ManagedByDockerDiscovery},
			Tags:        append([]string(nil), tmpl.Tags...),
		},
		Domains:  domains,
		Upstream: upstream,
		TLS:      tlsSettings,
		//lint:ignore SA1019 compat copy of deprecated WebsocketsUpgrade so a pre-deprecation template still produces byte-identical derived hosts
		WebsocketsUpgrade: tmpl.WebsocketsUpgrade,
		RobotsNoIndex:     tmpl.RobotsNoIndex,
		Timeouts:          timeouts,
		Middlewares:       append([]string(nil), tmpl.Middlewares...),
		AccessLists:       append([]string(nil), tmpl.AccessLists...),
	}
	if len(tmpl.StripResponseHeaders) > 0 {
		host.StripResponseHeaders = append([]string(nil), tmpl.StripResponseHeaders...)
	}
	if len(tmpl.SecurityHeaders) > 0 {
		host.SecurityHeaders = make(map[string]model.SecurityHeaderValue, len(tmpl.SecurityHeaders))
		for k, v := range tmpl.SecurityHeaders {
			host.SecurityHeaders[k] = v
		}
	}
	if pol := dnsPolicy(ct, tmpl, conf); pol != nil {
		host.DNS = pol
	}
	return name, host, prof, nil
}

// deriveDomains reads the comma-separated domains label, validating every entry
// the same way a hostname from a Kubernetes Ingress is validated: a strict LDH
// hostname inside the configured suffix list. Everything a container says about
// itself is untrusted input.
func deriveDomains(ct Container, conf model.DockerDiscoverySettings, tmpl model.IngressHostTemplate) ([]string, error) {
	raw := ct.Labels[conf.LabelDomains()]
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("label %s is required (comma-separated hostnames)", conf.LabelDomains())
	}
	var domains, rejected []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		h := model.NormalizeHostname(part)
		if h == "" {
			continue
		}
		if !model.IsHostname(h) {
			rejected = append(rejected, ellipsize(h, 64)+" (not a valid hostname; wildcards are not supported)")
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
		log.Warn().Str("container", ct.Name()).Strs("rejected", rejected).
			Msg("docker discovery: rejecting host(s) from a container label")
	}
	if len(domains) == 0 {
		reason := fmt.Sprintf("no usable hostname in label %s", conf.LabelDomains())
		if len(rejected) > 0 {
			reason += ": " + strings.Join(rejected, ", ")
		}
		return nil, errors.New(reason)
	}
	// Sorted so the derived object does not churn when the label order changes.
	sort.Strings(domains)
	return domains, nil
}

// deriveUpstream resolves where a derived host forwards: the container's own
// address on a Docker network, or the host-published port. See
// docs/reference/config/settings/docker-discovery.md for which of the two a
// given deployment wants.
func deriveUpstream(ct Container, conf model.DockerDiscoverySettings) (model.Upstream, error) {
	scheme := strings.ToLower(strings.TrimSpace(ct.Labels[conf.LabelScheme()]))
	switch scheme {
	case "":
		scheme = "http"
	case "http", "https":
	default:
		return model.Upstream{}, fmt.Errorf("label %s must be http or https, got %q", conf.LabelScheme(), ellipsize(scheme, 32))
	}

	port, err := containerPort(ct, conf)
	if err != nil {
		return model.Upstream{}, err
	}

	if conf.UsePublishedPorts {
		published, err := publishedPort(ct, port)
		if err != nil {
			return model.Upstream{}, err
		}
		return model.Upstream{Scheme: scheme, Host: conf.PublishedAddress(), Port: published}, nil
	}

	ip, err := containerIP(ct, conf)
	if err != nil {
		return model.Upstream{}, err
	}
	return model.Upstream{Scheme: scheme, Host: ip, Port: port}, nil
}

// containerPort resolves the CONTAINER-side port: the label when set, otherwise
// the single exposed TCP port. More than one exposed port with no label is a
// skip, not a guess - picking one would silently publish the wrong service.
func containerPort(ct Container, conf model.DockerDiscoverySettings) (int, error) {
	if raw, ok := ct.Labels[conf.LabelPort()]; ok && strings.TrimSpace(raw) != "" {
		p, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || p < 1 || p > 65535 {
			return 0, fmt.Errorf("label %s %q is not a port number in 1-65535", conf.LabelPort(), ellipsize(raw, 32))
		}
		return p, nil
	}
	var found []int
	seen := map[int]bool{}
	for _, p := range ct.Ports {
		if p.Type != "" && p.Type != "tcp" {
			continue
		}
		if p.PrivatePort == 0 || seen[p.PrivatePort] {
			continue
		}
		seen[p.PrivatePort] = true
		found = append(found, p.PrivatePort)
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return 0, fmt.Errorf("label %s is required: the container exposes no TCP port to infer one from", conf.LabelPort())
	default:
		sort.Ints(found)
		return 0, fmt.Errorf("label %s is required: the container exposes %d TCP ports (%s) and gpm will not guess", conf.LabelPort(), len(found), joinInts(found))
	}
}

// publishedPort maps a container port onto the host port it is published on.
func publishedPort(ct Container, container int) (int, error) {
	for _, p := range ct.Ports {
		if p.Type != "" && p.Type != "tcp" {
			continue
		}
		if p.PrivatePort == container && p.PublicPort != 0 {
			return p.PublicPort, nil
		}
	}
	return 0, fmt.Errorf("container port %d is not published to the host, and usePublishedPorts is on", container)
}

// containerIP resolves the container's address on the configured network, or on
// its only usable network when none is configured. "host" and "none" are
// skipped: neither carries a per-container address.
func containerIP(ct Container, conf model.DockerDiscoverySettings) (string, error) {
	nets := ct.NetworkSettings.Networks
	if conf.Network != "" {
		n, ok := nets[conf.Network]
		if !ok {
			return "", fmt.Errorf("container is not attached to the configured network %q (attached: %s)", conf.Network, joinStrings(networkNames(nets)))
		}
		if n.IPAddress == "" {
			return "", fmt.Errorf("container has no IP address on network %q (is it running?)", conf.Network)
		}
		if net.ParseIP(n.IPAddress) == nil {
			return "", fmt.Errorf("container reports an unusable IP address on network %q", conf.Network)
		}
		return n.IPAddress, nil
	}
	for _, name := range networkNames(nets) {
		if name == "host" || name == "none" {
			continue
		}
		if ip := nets[name].IPAddress; ip != "" && net.ParseIP(ip) != nil {
			return ip, nil
		}
	}
	return "", errors.New("container has no usable IP address on any Docker network; set dockerDiscovery.network, or usePublishedPorts when gpm runs on the host")
}

func networkNames(nets map[string]struct {
	IPAddress string `json:"IPAddress"`
}) []string {
	out := make([]string, 0, len(nets))
	for k := range nets {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinStrings(v []string) string {
	if len(v) == 0 {
		return "none"
	}
	return strings.Join(v, ", ")
}

func joinInts(v []int) string {
	parts := make([]string, 0, len(v))
	for _, n := range v {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ", ")
}

// dnsPolicy resolves the derived host's DNS policy: the resolved profile's
// default, with each flag individually overridden by its label. A policy that
// asks for nothing is nil, so an opted-out host carries no dns key at all.
func dnsPolicy(ct Container, tmpl model.IngressHostTemplate, conf model.DockerDiscoverySettings) *model.DNSSyncPolicy {
	pol := model.DNSSyncPolicy{}
	if tmpl.DefaultDNS != nil {
		pol = *tmpl.DefaultDNS
	}
	apply := func(key string, dst *bool) {
		raw, ok := ct.Labels[key]
		if !ok {
			return
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true":
			*dst = true
		case "false":
			*dst = false
		default:
			log.Warn().Str("container", ct.Name()).Str("label", key).Str("value", ellipsize(raw, 32)).
				Msg(`docker discovery: label value must be "true" or "false"; keeping the template default`)
		}
	}
	apply(conf.LabelLanDirect(), &pol.LanDirect)
	apply(conf.LabelPublicCname(), &pol.PublicCname)
	if !pol.Enabled() {
		return nil
	}
	return &pol
}
