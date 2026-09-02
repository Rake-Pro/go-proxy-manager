// Package discovery holds the source-agnostic half of gpm's object discovery:
// the full-state reconcile planner that turns "here is everything the source
// currently declares" into the creates, updates, deletes and skips that bring
// the managed proxy hosts in line with it.
//
// It exists because gpm discovers from two sources - annotated Kubernetes
// Ingresses (internal/k8s) and labelled Docker containers (internal/docker) -
// and every property that makes discovery safe is a property of the PLAN, not
// of the source:
//
//   - Reconcile is FULL-STATE. The desired set is recomputed from a complete
//     listing every time and compared with what the config holds, so drift is
//     repaired in both directions.
//   - Writes are OWNERSHIP-GATED, by name AND by domain. Only hosts carrying
//     this subsystem's managed-by label pair are created, updated or deleted; a
//     hand-written host with the same name, or one already serving a domain the
//     derived host wants, is skipped with a warning and never overwritten.
//   - Operator-owned state (maintenance, a hand-set disabled) is carried
//     forward across a reconcile rather than derived.
//   - A source item that cannot be derived PROTECTS its existing host from
//     deletion, so one bad manifest or label edit cannot take a service
//     offline; the single exception is an unresolvable profile, which fails
//     CLOSED by disabling the host so a retired chain cannot be pinned forever.
//
// Writing those rules twice would mean maintaining two copies of the one part
// of discovery that has security consequences. The source-specific half - how
// to list, what an item's derived name is, which fields of the source are even
// looked at - stays in the per-source package, which hands this one a slice of
// already-derived Items.
//
// The freeze rule (a listing that FAILED never reaches a plan) lives in the
// callers, since only they know what a failed list looks like. This package is
// pure: it performs no I/O and never sees a client.
//
// See design/ingress-discovery.md and design/docker-discovery.md.
package discovery

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// Per-host outcomes reported in a Result.
const (
	ActionCreated   = "created"
	ActionUpdated   = "updated"
	ActionUnchanged = "unchanged"
	ActionDeleted   = "deleted"
	ActionSkipped   = "skipped"
)

// Ownership is everything the planner needs to know about which proxy hosts
// belong to the calling subsystem, and how to talk about them. It is a plain
// struct of values and funcs rather than an interface: the two implementations
// are both thin projections of a settings block, and an interface would add a
// vocabulary without adding a capability.
//
// The label VALUE is what keeps the two reconcilers apart. Both stamp
// <prefix>/managed-by, but with "ingress-discovery" and "docker-discovery"
// respectively, so neither can ever see - let alone delete - the other's hosts.
type Ownership struct {
	// Subsystem names the caller in operator-facing text, e.g. "ingress
	// discovery" / "docker discovery".
	Subsystem string
	// SourceKind names one source object in operator-facing text, e.g.
	// "Ingress" / "container".
	SourceKind string

	// ManagedByKey/DisabledByKey are the CURRENT-prefix label keys, and Value is
	// the label value this subsystem writes into both.
	ManagedByKey  string
	DisabledByKey string
	Value         string

	// Migrate lifts ownership recognition to labels written under a PREVIOUS
	// prefix, so a prefix change can be repaired by the next reconcile instead of
	// orphaning every host. When it is false the three funcs below are never
	// called and may be nil.
	Migrate bool
	// HasStaleManaged/HasStaleDisabled report a managed-by / disabled-by label
	// under some other prefix; StripStale deletes such labels in place.
	HasStaleManaged  func(map[string]string) bool
	HasStaleDisabled func(map[string]string) bool
	StripStale       func(map[string]string)
}

// Item is one source object, already derived by the caller.
//
// Name is returned even on a derive FAILURE whenever the name itself was
// computable, because that is what protects an existing host from deletion. A
// derive that could not even produce a name leaves it empty, and the plan then
// has nothing to protect.
type Item struct {
	// Ref identifies the source object for status and logs, e.g.
	// "monitoring/grafana" or a container name.
	Ref string
	// Name is the derived proxy-host name.
	Name string
	// Host is the fully derived proxy host. Only read when Err is nil.
	Host model.ProxyHost
	// Profile is the settings profile the item resolved to, or the REQUESTED
	// name when resolution failed.
	Profile string
	// Err is the derive failure, if any.
	Err error
	// UnknownProfile marks Err as "the named profile does not resolve", the one
	// failure that fails CLOSED (disable) rather than freezing.
	UnknownProfile bool
}

// HostResult is what one reconcile decided about one derived (or previously
// derived) proxy host. Callers project it onto their own JSON shape, so the
// wire format of each subsystem's status endpoint stays its own business.
type HostResult struct {
	// Name is the derived proxy-host name.
	Name string
	// Ref is the source object, empty for a delete whose source is already gone.
	Ref     string
	Domains []string
	Action  string
	// Profile is the profile the host resolved to ("template" for the default
	// block), or the REQUESTED name on a profile skip. Empty for a delete.
	Profile string
	// Reason explains a skip, and the two updates that are not a plain derive:
	// a fail-closed disable, and its reversal.
	Reason string
}

// Result is the whole plan, before anything is written.
type Result struct {
	Upserts []model.ProxyHost
	Deletes []string

	Discovered   int
	Created      int
	Updated      int
	Skipped      int
	ManagedAfter int
	Hosts        []HostResult
}

// Managed reports whether a proxy host is one this subsystem owns: it carries
// managed-by under the CURRENT prefix with this subsystem's value. When
// Migrate is set, a host still carrying the pair under a DIFFERENT (older)
// prefix is ALSO recognised, so it is treated as owned for this run rather
// than skipped as hand-written - which is what lets its normal derive/update
// path relabel it onto the current prefix.
func (o Ownership) Managed(h model.ProxyHost) bool {
	if h.Labels[o.ManagedByKey] == o.Value {
		return true
	}
	return o.Migrate && o.HasStaleManaged != nil && o.HasStaleManaged(h.Labels)
}

// OperatorDisabled reports whether a managed host's Disabled: true was set by
// the OPERATOR rather than by discovery's own fail-closed revocation path.
// Discovery must never re-enable a host it did not disable itself - that would
// turn a hand-disable, the obvious move when an app has to come offline now,
// into a no-op on the very next poll.
func (o Ownership) OperatorDisabled(cur model.ProxyHost) bool {
	if !cur.Disabled {
		return false
	}
	if cur.Labels[o.DisabledByKey] == o.Value {
		return false
	}
	if o.Migrate && o.HasStaleDisabled != nil && o.HasStaleDisabled(cur.Labels) {
		return false
	}
	return true
}

// Plan computes the whole reconcile without performing any I/O, which is what
// makes every ownership and freeze rule directly testable. items must be the
// COMPLETE derived set from a successful listing: an incomplete one read as
// complete is precisely what would delete managed hosts that should be kept.
func Plan(cfg model.Config, own Ownership, items []Item) Result {
	var p Result

	current := map[string]model.ProxyHost{} // every proxy host, by name
	managed := map[string]model.ProxyHost{} // only the ones this subsystem owns
	for _, h := range cfg.ProxyHosts {
		current[h.Name] = h
		if own.Managed(h) {
			managed[h.Name] = h
		}
	}

	desired := map[string]model.ProxyHost{}
	source := map[string]string{}
	// profile records which settings profile each desired host resolved to, so
	// the status can say what chain a host actually got.
	profile := map[string]string{}
	// protected holds names that must NOT be deleted even though they are absent
	// from the desired set: their source object IS opted in but could not be
	// derived. Deletion follows from absence in a healthy list, never from a
	// parse failure - one bad edit must not take a host offline.
	protected := map[string]bool{}
	// disable holds managed hosts this run must FAIL CLOSED on: their source
	// names a profile that no longer resolves, so the chain they are still
	// serving is one nobody can point at any more.
	disable := map[string]model.ProxyHost{}

	for _, it := range items {
		p.Discovered++
		if it.Err != nil {
			if it.Name != "" {
				protected[it.Name] = true
			}
			// Two derive failures, two opposite safe answers.
			//
			// A MALFORMED source object (bad hostname, unusable derived name) is a
			// tenant typo against a chain the operator still sanctions: the host on
			// disk is the last good rendering of a policy that has not changed, so
			// freezing it keeps a working service up while the manifest is fixed.
			// Failing closed there would let anyone take their own service offline
			// with a one-character edit, and would do nothing for security.
			//
			// An UNRESOLVABLE PROFILE is the opposite: the chain that host is serving
			// is one the operator has just tightened, renamed or retired. Freezing
			// would let a tenant pin a revoked chain forever simply by flipping the
			// annotation or label to a name that does not exist - the security
			// property would hold for creating a host but not for REVOKING one. So
			// the host is disabled instead: the object is preserved (nothing is
			// destroyed, the operator can re-add the profile and the very next
			// reconcile re-enables it), but it stops serving the revoked chain.
			if cur, ok := managed[it.Name]; ok && it.UnknownProfile && !cur.Disabled {
				off := cur
				off.Disabled = true
				// Mark the disable as DISCOVERY's own, so the next reconcile that
				// resolves cleanly is free to clear it - an operator's own disable
				// (no label, or a different one) never gets this treatment.
				// off.Labels aliases cur.Labels (off := cur is a shallow copy), so it
				// is cloned before being written to, or this would mutate cur - and,
				// through it, the config the caller still holds - out from under the
				// read.
				off.Labels = cloneLabels(cur.Labels)
				// During a prefix migration, cur may only carry a STALE prefix's
				// labels (that is how it got into managed[] above); strip them so the
				// host ends up with exactly the current prefix's pair, rather than old
				// and new keys both lingering.
				if own.Migrate && own.StripStale != nil {
					own.StripStale(off.Labels)
				}
				off.Labels[own.ManagedByKey] = own.Value
				off.Labels[own.DisabledByKey] = own.Value
				disable[it.Name] = off
				p.Updated++
				p.Hosts = append(p.Hosts, HostResult{Name: it.Name, Ref: it.Ref, Domains: cur.Domains,
					Action: ActionUpdated, Profile: it.Profile,
					Reason: "disabled (fails closed rather than keep serving a chain that no longer resolves): " + it.Err.Error()})
				log.Warn().Str("host", it.Name).Str("source", it.Ref).Err(it.Err).
					Msg(own.Subsystem + ": profile no longer resolves; disabling the derived host rather than leaving it serving the old chain")
				continue
			}
			p.Skipped++
			p.Hosts = append(p.Hosts, HostResult{Name: it.Name, Ref: it.Ref, Action: ActionSkipped, Profile: it.Profile, Reason: it.Err.Error()})
			log.Warn().Str("source", it.Ref).Err(it.Err).Msg(own.Subsystem + ": skipping opted-in " + own.SourceKind)
			continue
		}
		if prev, dup := source[it.Name]; dup {
			p.Skipped++
			p.Hosts = append(p.Hosts, HostResult{Name: it.Name, Ref: it.Ref, Action: ActionSkipped, Profile: it.Profile,
				Reason: "derived name collides with " + own.SourceKind + " " + prev})
			log.Warn().Str("source", it.Ref).Str("other", prev).
				Msg(own.Subsystem + ": two source objects derive the same host name")
			continue
		}
		desired[it.Name] = it.Host
		source[it.Name] = it.Ref
		profile[it.Name] = it.Profile
	}

	// Which managed hosts this run would remove. Needed before the domain gate
	// below: a host on its way out must not keep claiming its domains, or a
	// renamed source object could never hand its hostname over.
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
	// this reconcile is not rewriting is off limits; a derived host that wants
	// one is skipped and reported, exactly like a name collision.
	claimed := map[string]string{}
	claim := func(owner string, domains []string) {
		for _, dom := range domains {
			key := DomainKey(dom)
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
		// everything else - every operator-authored host, every host owned by the
		// OTHER discovery source, and every managed host being kept as-is - holds
		// on to them.
		if own.Managed(h) {
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

	for _, name := range SortedKeys(desired) {
		want := desired[name]
		cur, exists := current[name]
		// A skipped host leaves whatever is on disk in place, so it must re-assert
		// that object's domains: otherwise a later-sorted derived host could claim
		// one and produce a duplicate the config validator would reject, failing
		// the whole batch.
		skip := func(reason string) {
			p.Skipped++
			p.Hosts = append(p.Hosts, HostResult{Name: name, Ref: source[name], Domains: want.Domains,
				Action: ActionSkipped, Profile: profile[name], Reason: reason})
			if exists {
				claim(name, cur.Domains)
			}
		}
		if conflict, owner := firstClaimed(want.Domains, claimed); conflict != "" {
			skip(fmt.Sprintf("domain %q is already claimed by proxy host %q, which %s does not own", conflict, owner, own.Subsystem))
			log.Warn().Str("host", name).Str("source", source[name]).
				Str("domain", conflict).Str("owner", owner).
				Msg(own.Subsystem + ": an existing host already serves this domain; refusing to shadow it")
			continue
		}
		switch {
		case exists && !own.Managed(cur):
			// Somebody hand-wrote a host with this name (or the other discovery
			// source owns it). Overwriting it is exactly what the ownership rule
			// forbids, so skip and say so - the same skip-and-warn the Pi-hole and
			// Cloudflare backends do for a record they do not own.
			skip("a proxy host with this name exists and is not managed by " + own.Subsystem)
			log.Warn().Str("host", name).Str("source", source[name]).
				Msg(own.Subsystem + ": name is taken by a proxy host this reconciler does not own; leaving it alone")
		case !exists:
			claim(name, want.Domains)
			p.Upserts = append(p.Upserts, want)
			p.Created++
			p.Hosts = append(p.Hosts, HostResult{Name: name, Ref: source[name], Domains: want.Domains, Action: ActionCreated, Profile: profile[name]})
		default:
			claim(name, want.Domains)
			// Carry the original creation timestamp so an update does not rewrite it.
			want.CreatedAt = cur.CreatedAt
			// maintenance is OPERATOR-owned runtime state, like disabled below. No
			// source field derives it, so a derive always produces false - and
			// without carrying the stored value forward the next poll would quietly
			// put a host back into service while someone is still working on its
			// backend, which is the exact failure the flag exists to prevent.
			want.Maintenance = cur.Maintenance
			// disabled: true is OPERATOR-owned state once discovery did not set it
			// itself: a hand-disabled host must not be re-enabled by the next poll
			// just because its source still derives cleanly. A host discovery
			// disabled (<prefix>/disabled-by) is exempt - that disable is
			// discovery's own fail-closed hold, and a clean derive is exactly the
			// signal that lifts it (want.Disabled is already false here, and want
			// carries no disabled-by label, so it clears on its own).
			reenable := false
			if own.OperatorDisabled(cur) {
				want.Disabled = true
			} else if cur.Disabled {
				reenable = true
			}
			if SameHost(cur, want) {
				p.Hosts = append(p.Hosts, HostResult{Name: name, Ref: source[name], Domains: want.Domains, Action: ActionUnchanged, Profile: profile[name]})
				continue
			}
			p.Upserts = append(p.Upserts, want)
			p.Updated++
			reason := ""
			switch {
			case own.OperatorDisabled(cur):
				reason = "operator-disabled host: other fields refreshed, disabled state preserved"
			case reenable:
				reason = "profile resolves again: re-enabling the host discovery had disabled"
			}
			p.Hosts = append(p.Hosts, HostResult{Name: name, Ref: source[name], Domains: want.Domains, Action: ActionUpdated, Profile: profile[name], Reason: reason})
		}
	}

	// The fail-closed writes. They are plain upserts of an existing managed
	// object with disabled: true, so they go through the same ownership guard and
	// the same single commit as everything else. Their domains were already
	// re-asserted by the claim loop above (the host is neither rewritten nor
	// doomed), so a later-sorted derived host cannot pick up a hostname a
	// disabled host is holding - the disable is a hold, not a handover.
	for _, name := range SortedKeys(disable) {
		p.Upserts = append(p.Upserts, disable[name])
	}

	for _, name := range SortedKeys(managed) {
		if !doomed[name] {
			continue
		}
		p.Deletes = append(p.Deletes, name)
		p.Hosts = append(p.Hosts, HostResult{Name: name, Domains: managed[name].Domains, Action: ActionDeleted})
		log.Warn().Str("host", name).Msg(own.Subsystem + ": no opted-in source object derives this managed host any more; removing it")
	}

	p.ManagedAfter = len(managed) + p.Created - len(p.Deletes)
	if p.Hosts == nil {
		p.Hosts = []HostResult{}
	}
	return p
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

// DomainKey normalises a configured domain for comparison, matching the key the
// config validator and the router's per-domain maps use.
func DomainKey(d string) string { return strings.ToLower(strings.TrimSpace(d)) }

// firstClaimed returns the first of domains already claimed by another host, and
// that host's name, or ("", "") when none is.
func firstClaimed(domains []string, claimed map[string]string) (string, string) {
	for _, d := range domains {
		if owner, taken := claimed[DomainKey(d)]; taken {
			return DomainKey(d), owner
		}
	}
	return "", ""
}

// SortedKeys returns a map's keys in sorted order, so a plan does not depend on
// map iteration order.
func SortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SameHost compares a stored host with a freshly derived one, ignoring the
// store-maintained timestamps, so a steady-state reconcile writes nothing.
func SameHost(a, b model.ProxyHost) bool {
	a.CreatedAt, a.UpdatedAt = time.Time{}, time.Time{}
	b.CreatedAt, b.UpdatedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(a, b)
}
