package model

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Kubernetes Ingress annotations gpm reads under the DEFAULT annotation
// prefix. Opt-in is EXACTLY AnnotationManaged == "true"; an Ingress carrying
// anything else (or nothing) is invisible to discovery. There is no opt-out
// mode and no namespace sweep.
//
// These constants are the DEFAULT prefix's keys only, kept for backward
// compatibility (every deployment that has never set
// ingressDiscovery.annotationPrefix keeps working unchanged). Code that needs
// to honour a possibly-customised prefix must use the IngressDiscoverySettings
// methods below (AnnotationManaged(), ManagedByLabel(), etc.) instead of these
// constants.
const (
	// AnnotationManaged opts an Ingress into gpm discovery.
	AnnotationManaged = "gpm.rake.pro/managed"
	// AnnotationLanDirect sets dns.lanDirect on the derived proxy host.
	AnnotationLanDirect = "gpm.rake.pro/lan-direct"
	// AnnotationPublicCname sets dns.publicCname on the derived proxy host.
	AnnotationPublicCname = "gpm.rake.pro/public-cname"
	// AnnotationProfile NAMES one of the operator-defined
	// settings.ingressDiscovery.profiles entries. It carries a name and nothing
	// else: the Ingress author is untrusted, so they may pick from the set of
	// chains the operator sanctioned but can never describe one. Naming a profile
	// that does not exist SKIPS the Ingress - it is never quietly downgraded to
	// the default. See design/ingress-discovery.md section 5a.
	AnnotationProfile = "gpm.rake.pro/profile"
)

// DefaultAnnotationPrefix is the annotation/label prefix used when
// ingressDiscovery.annotationPrefix is unset, so every existing deployment's
// keys are unchanged unless an operator opts into a different prefix.
const DefaultAnnotationPrefix = "gpm.rake.pro"

// annotationPrefixRe matches a DNS-subdomain-shaped annotation/label key
// prefix: dot-separated lowercase alphanumeric labels (hyphens allowed inside
// a label), no leading or trailing dot, and - since it never includes the "/"
// gpm appends itself - no slash. This is the same shape Kubernetes requires
// for the prefix portion of a qualified annotation/label key.
var annotationPrefixRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

// DefaultProfileName is what a host derived from the default `template` reports
// as its profile in the reconcile status. It is a reserved profile name so the
// audit trail is never ambiguous about which block produced a chain.
const DefaultProfileName = "template"

// ManagedByLabel/ManagedByIngressDiscovery mark the proxy hosts discovery owns.
// Reconciliation creates, updates and deletes ONLY objects carrying this label
// pair; a hand-written host with the same name is skipped with a warning, never
// overwritten - the same ownership rule the DNS backends use for records.
const (
	ManagedByLabel            = "gpm.rake.pro/managed-by"
	ManagedByIngressDiscovery = "ingress-discovery"

	defaultIngressPollInterval = time.Minute
	minimumIngressPollInterval = 15 * time.Second
)

// DisabledByLabel/DisabledByIngressDiscovery mark a managed host's Disabled
// state as DISCOVERY-owned, so a hand-disabled host can never be re-enabled by
// the next poll. `disabled: true` is operator-owned state: if a managed host is
// disabled and this label is absent (or names something other than
// "ingress-discovery"), an operator set it and discovery leaves it alone on
// every subsequent reconcile. Discovery sets this label only on the ONE upsert
// where it itself disables a host (an unresolvable profile - see
// design/ingress-discovery.md section 5a); the label - and with it the
// disabled-ness - is dropped the moment that host next derives successfully, so
// discovery's own disables self-heal while an operator's never do.
const (
	DisabledByLabel            = "gpm.rake.pro/disabled-by"
	DisabledByIngressDiscovery = "ingress-discovery"
)

// ProfileSelection modes for IngressDiscoverySettings.ProfileSelection.
const (
	// ProfileSelectionAnnotationOrRules (the default/empty value) tries
	// ProfileRules first, in order; if none matches it falls back to the
	// gpm.rake.pro/profile annotation, i.e. today's behaviour.
	ProfileSelectionAnnotationOrRules = "annotation-or-rules"
	// ProfileSelectionRulesOnly ignores the annotation entirely. An Ingress that
	// matches no rule gets the default template, exactly as if it carried no
	// annotation at all - the Ingress author has no say in profile selection.
	ProfileSelectionRulesOnly = "rules-only"
)

// The projected ServiceAccount paths used when gpm runs in-cluster and no
// explicit tokenFile/caFile is configured.
const (
	DefaultKubernetesTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	DefaultKubernetesCAFile    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// dnsLabelRe matches a single DNS-1123 label (a Kubernetes namespace).
var dnsLabelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// IsDNSLabel reports whether s is a single DNS-1123 label, i.e. a valid
// Kubernetes namespace. Discovery derives a proxy-host name by joining the
// Ingress name and its namespace with a dot, which is only unambiguous while the
// namespace is dot-free; gpm checks that itself rather than trusting the API
// server to have enforced it.
func IsDNSLabel(s string) bool { return dnsLabelRe.MatchString(s) }

// hostnameRe matches a strict LDH hostname of at least two labels. It is the
// gate every string that arrives from the Kubernetes API passes before it can
// become a served domain, so it deliberately rejects wildcards ("*."), empty
// labels, underscores, spaces, scheme/path fragments and control characters.
var hostnameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// NormalizeHostname lowercases a hostname and strips surrounding whitespace and
// one trailing root dot. It does NOT validate; pair it with IsHostname.
func NormalizeHostname(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}

// IsHostname reports whether s is a valid multi-label LDH hostname of at most
// 253 characters. Wildcards are rejected.
func IsHostname(s string) bool {
	return len(s) <= 253 && hostnameRe.MatchString(s)
}

// IngressHostTemplate is the operator-supplied shape every discovered proxy host
// takes. It is the ONLY source for everything security-relevant: an Ingress
// contributes hostnames (strictly validated) and two DNS booleans, and can never
// supply a middleware, an access list, a certificate, or an upstream. That
// containment is the point - a cluster user who can edit an Ingress must not be
// able to weaken the chain the gpm operator configured.
type IngressHostTemplate struct {
	// Upstream is where discovered hosts forward to: the CLUSTER INGRESS
	// CONTROLLER's address, not the Ingress backend Service. gpm runs off-cluster,
	// so <svc>.<ns>.svc.cluster.local is neither resolvable nor routable from it;
	// the controller routes to the right workload by vhost, and the data plane
	// preserves the browser-facing Host header on the way through.
	//
	// Prefer scheme http to the controller's plain port (TLS is terminated once,
	// at the edge). With https the Go transport derives SNI and certificate
	// verification from THIS host, not from the forwarded Host header, so an https
	// upstream must name a hostname the controller's certificate covers.
	// Exactly one of Upstream / UpstreamGroupRef is set, mirroring ProxyHost.
	Upstream Upstream `json:"upstream,omitempty" yaml:"upstream,omitempty"`

	// UpstreamGroupRef names an UpstreamGroup instead of a single backend, so
	// derived hosts get the same failover the hand-written ones have. A cluster
	// ingress controller usually runs on every node: pinning discovery to one of
	// them would make every discovered service single-node while the operator's
	// own hosts survive a node loss.
	UpstreamGroupRef string `json:"upstreamGroupRef,omitempty" yaml:"upstreamGroupRef,omitempty"`

	// TLS is applied verbatim to every derived host. certificateRef is required:
	// discovery never creates a Certificate and never triggers ACME, so a single
	// operator-maintained (typically wildcard) certificate covers the discovered
	// set. See design/ingress-discovery.md section 1.
	TLS TLSSettings `json:"tls,omitempty" yaml:"tls,omitempty"`

	// Deprecated: no effect, exactly as on ProxyHost - WebSocket upgrades always
	// work. Still stamped onto derived hosts so a template written before the
	// deprecation produces byte-identical hosts (a changed derived host would show
	// up as a spurious update on every reconcile).
	WebsocketsUpgrade bool `json:"websocketsUpgrade,omitempty" yaml:"websocketsUpgrade,omitempty"`

	// RobotsNoIndex is applied verbatim to every derived host, exactly as on a
	// hand-written ProxyHost. Without it, cutting a service over to discovery
	// silently DROPS its no-index header, and the only way back is a headers
	// middleware setting X-Robots-Tag - a second mechanism for something the model
	// already expresses, and one that has to be remembered per host.
	RobotsNoIndex bool `json:"robotsNoIndex,omitempty" yaml:"robotsNoIndex,omitempty"`

	// Timeouts is applied verbatim to every derived host and validated exactly
	// like a proxy host's. A pointer so an unset template omits the key entirely
	// rather than stamping a zero-valued `timeouts: {}` onto every derived object
	// (the same reason ProxyHost.DNS is a pointer).
	Timeouts *HostTimeouts `json:"timeouts,omitempty" yaml:"timeouts,omitempty"`

	// Middlewares/AccessLists are applied to every derived host, in this order.
	Middlewares []string `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	AccessLists []string `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`

	// StripResponseHeaders is applied verbatim to every derived host, exactly as
	// on a hand-written ProxyHost. Without it a per-host strip list on a
	// discovery-managed host is reverted (with a git commit) on the next
	// reconcile, since the reconciler rebuilds the whole object from this
	// template.
	StripResponseHeaders []string `json:"stripResponseHeaders,omitempty" yaml:"stripResponseHeaders,omitempty"`

	// SecurityHeaders is applied verbatim to every derived host, exactly as on a
	// hand-written ProxyHost, and for the same reason StripResponseHeaders is:
	// without it a per-host securityHeaders override on a discovery-managed host
	// is rebuilt away (with a git commit) on the next reconcile.
	SecurityHeaders map[string]SecurityHeaderValue `json:"securityHeaders,omitempty" yaml:"securityHeaders,omitempty"`

	// Tags are applied verbatim to every derived host's ObjectMeta, so the hosts
	// a given profile produces can be grouped and filtered in the UI like any
	// other host. They are free-form UI metadata with no validation rules and no
	// data-plane effect, which is why they can be inherited from a profile an
	// untrusted Ingress selects: the worst a tenant can do is pick a label the
	// operator wrote.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// DefaultDNS is the dns policy a derived host gets when the corresponding
	// annotation is absent. Each flag is overridden individually by its
	// annotation, so a template default of lanDirect can be turned off per
	// Ingress with gpm.rake.pro/lan-direct: "false".
	DefaultDNS *DNSSyncPolicy `json:"defaultDNS,omitempty" yaml:"defaultDNS,omitempty"`

	// AllowedDomainSuffixes NARROWS the global
	// ingressDiscovery.allowedDomainSuffixes for hosts derived from this profile.
	// Empty means no narrowing: the global list applies unchanged. When set, it
	// MUST be a subset of the global list (every entry equal to, or a
	// dot-boundary sub-suffix of, some global entry) - a profile can only shrink
	// the domains a tenant may publish, never grow them. Checked at
	// Settings.Validate so a widening profile fails the settings write, not a
	// later reconcile.
	AllowedDomainSuffixes []string `json:"allowedDomainSuffixes,omitempty" yaml:"allowedDomainSuffixes,omitempty"`
}

// IngressDiscoverySettings configures discovery of annotated cluster Ingresses
// into template-derived, managed-labelled proxy hosts (DNS sync phase 2).
// Disabled (the default) means the subsystem is inert and never contacts
// anything.
type IngressDiscoverySettings struct {
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// APIURL is the Kubernetes API server base URL, e.g.
	// https://k8s.example.lan:6443. Empty uses the in-cluster endpoint derived
	// from KUBERNETES_SERVICE_HOST/KUBERNETES_SERVICE_PORT.
	APIURL string `json:"apiURL,omitempty" yaml:"apiURL,omitempty"`
	// TokenFile is the path to the bearer token for a read-only ServiceAccount.
	// Empty uses the projected in-cluster path. It is a path rather than a Secret
	// placeholder on purpose: projected tokens ROTATE on disk, so a snapshot taken
	// at load would go stale, and a path keeps the credential out of the config
	// repo entirely.
	TokenFile string `json:"tokenFile,omitempty" yaml:"tokenFile,omitempty"`
	// CAFile is the PEM bundle that verifies the API server certificate. Empty
	// uses the projected in-cluster CA. There is no "skip verification" option.
	CAFile string `json:"caFile,omitempty" yaml:"caFile,omitempty"`

	// Namespace restricts the list to one namespace. Empty lists cluster-wide
	// (still annotation-gated: a cluster-wide LIST is not a namespace sweep).
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	// LabelSelector optionally narrows the server-side list, e.g.
	// "app.kubernetes.io/part-of=platform". The opt-in annotation is still
	// required: the Kubernetes API cannot select on annotations, so that filter is
	// applied client-side regardless.
	LabelSelector string `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`

	// PollInterval is a Go duration string (e.g. "60s", "5m"). Empty means 1m;
	// anything below 15s is refused - a reconcile takes the store write lock.
	PollInterval string `json:"pollInterval,omitempty" yaml:"pollInterval,omitempty"`

	// AllowedDomainSuffixes bounds which hostnames discovery will ever publish. A
	// derived domain must equal one of these or end in "." + one of them. It is
	// REQUIRED when discovery is enabled: unrestricted discovery is one annotated
	// manifest away from claiming somebody else's name at the edge.
	AllowedDomainSuffixes []string `json:"allowedDomainSuffixes,omitempty" yaml:"allowedDomainSuffixes,omitempty"`

	// Template is the DEFAULT profile: the shape an Ingress gets when it names no
	// profile at all. It predates Profiles and keeps working unchanged.
	Template IngressHostTemplate `json:"template,omitempty" yaml:"template,omitempty"`

	// Profiles are additional operator-defined chains an Ingress may SELECT BY
	// NAME with gpm.rake.pro/profile. The real fleet is heterogeneous - some hosts
	// are deliberately public, some carry sso, some rate-limit - and one template
	// can only ever adopt the group that happens to match it; adopting the rest
	// would silently drop their middleware or impose an access list on a host that
	// is public on purpose.
	//
	// The security property that makes this safe is that the annotation carries a
	// NAME and nothing else. Every profile is written by the operator here, in the
	// config repo, and validated exactly like Template; a cluster tenant chooses
	// among them but cannot invent one, cannot name a middleware, an access list,
	// a certificate or an upstream, and cannot produce a host weaker than
	// something the operator explicitly sanctioned. A name that matches no profile
	// is skipped, not defaulted.
	//
	// Keys are profile names (ValidateName shape). "template" is reserved for the
	// default block above.
	Profiles map[string]IngressHostTemplate `json:"profiles,omitempty" yaml:"profiles,omitempty"`

	// ProfileRules let the OPERATOR route an Ingress to a profile by namespace
	// and/or label, with no say from the Ingress author at all - the escalation
	// path from named profiles for the one residual risk they carry (every
	// profile is selectable by every annotating Ingress). Evaluated in ORDER;
	// the first matching rule wins and its Profile is used, exactly as if the
	// Ingress had carried that name in gpm.rake.pro/profile. See
	// design/ingress-discovery.md section 5a.
	ProfileRules []IngressProfileRule `json:"profileRules,omitempty" yaml:"profileRules,omitempty"`

	// ProfileSelection controls whether the gpm.rake.pro/profile annotation is
	// consulted at all when no ProfileRules entry matches. Empty means
	// ProfileSelectionAnnotationOrRules. See the ProfileSelection* constants.
	ProfileSelection string `json:"profileSelection,omitempty" yaml:"profileSelection,omitempty"`

	// AnnotationPrefix replaces "gpm.rake.pro" as the prefix for every
	// discovery annotation gpm reads (.../managed, .../lan-direct,
	// .../public-cname, .../profile) and for the managed-by/disabled-by labels
	// gpm writes on the proxy hosts it derives. Empty means
	// DefaultAnnotationPrefix, so an existing deployment that never sets this is
	// completely unaffected. Use the methods below (AnnotationManaged(),
	// ManagedByLabel(), etc.) rather than the package-level Annotation*/
	// ManagedByLabel/DisabledByLabel constants, which are the DEFAULT prefix
	// only.
	//
	// Ownership recognition is keyed on the CURRENT prefix only: changing this
	// value does not retroactively relabel hosts discovery already wrote, so
	// they stop being recognised as discovery-managed until relabelled. See
	// AnnotationPrefixMigrate and "Changing the annotation prefix" in
	// docs/reference/config/settings/ingress-discovery.md.
	AnnotationPrefix string `json:"annotationPrefix,omitempty" yaml:"annotationPrefix,omitempty"`

	// AnnotationPrefixMigrate, when true, allows a settings write that changes
	// AnnotationPrefix even though existing proxy hosts still carry a
	// managed-by label under a DIFFERENT prefix (a write that would otherwise
	// be refused - see ValidateRefs). It does not relabel anything itself: it
	// only lifts the refusal. The relabel happens in the NEXT reconcile, as an
	// ordinary update in that run's single commit (see internal/k8s/discovery.go).
	AnnotationPrefixMigrate bool `json:"annotationPrefixMigrate,omitempty" yaml:"annotationPrefixMigrate,omitempty"`
}

// Prefix returns the configured annotation/label prefix, or
// DefaultAnnotationPrefix when AnnotationPrefix is unset.
func (d IngressDiscoverySettings) Prefix() string {
	if d.AnnotationPrefix == "" {
		return DefaultAnnotationPrefix
	}
	return d.AnnotationPrefix
}

// AnnotationManaged is the CURRENT-prefix key that opts an Ingress into gpm
// discovery. See the package-level AnnotationManaged constant for the exact
// semantics; only the key's prefix is configurable.
func (d IngressDiscoverySettings) AnnotationManaged() string { return d.Prefix() + "/managed" }

// AnnotationLanDirect is the CURRENT-prefix key that sets dns.lanDirect on a
// derived proxy host.
func (d IngressDiscoverySettings) AnnotationLanDirect() string { return d.Prefix() + "/lan-direct" }

// AnnotationPublicCname is the CURRENT-prefix key that sets dns.publicCname on
// a derived proxy host.
func (d IngressDiscoverySettings) AnnotationPublicCname() string {
	return d.Prefix() + "/public-cname"
}

// AnnotationProfile is the CURRENT-prefix key an Ingress uses to name a
// discovery profile.
func (d IngressDiscoverySettings) AnnotationProfile() string { return d.Prefix() + "/profile" }

// ManagedByLabel is the CURRENT-prefix label key discovery stamps (with value
// ManagedByIngressDiscovery) onto every proxy host it owns. A host is
// recognised as discovery-managed ONLY under this, the CURRENT prefix - see
// AnnotationPrefixMigrate for what changing the prefix requires.
func (d IngressDiscoverySettings) ManagedByLabel() string { return d.Prefix() + "/managed-by" }

// DisabledByLabel is the CURRENT-prefix label key discovery stamps (with value
// DisabledByIngressDiscovery) onto a managed host it fail-closed disabled
// itself, so the next clean derive knows it may re-enable it.
func (d IngressDiscoverySettings) DisabledByLabel() string { return d.Prefix() + "/disabled-by" }

// HasStaleManagedByLabel reports whether labels carries a managed-by label
// (value ManagedByIngressDiscovery) under a prefix OTHER than the one d is
// currently configured with - i.e. this object was labelled by discovery
// before the prefix was last changed. Used both to refuse a prefix change that
// would silently orphan hosts (ValidateRefs) and, when
// AnnotationPrefixMigrate is set, to keep recognising those hosts as
// discovery-owned so the next reconcile relabels them.
func (d IngressDiscoverySettings) HasStaleManagedByLabel(labels map[string]string) bool {
	return hasStaleDiscoveryLabel(labels, d.ManagedByLabel(), "/managed-by", ManagedByIngressDiscovery)
}

// HasStaleDisabledByLabel is HasStaleManagedByLabel's counterpart for the
// disabled-by label.
func (d IngressDiscoverySettings) HasStaleDisabledByLabel(labels map[string]string) bool {
	return hasStaleDiscoveryLabel(labels, d.DisabledByLabel(), "/disabled-by", DisabledByIngressDiscovery)
}

// hasStaleDiscoveryLabel reports whether labels carries a key, other than
// currentKey, ending in suffix ("/managed-by" or "/disabled-by") whose value is
// value - i.e. one a discovery reconciler itself writes. value is passed in
// because each reconciler owns a different one ("ingress-discovery" for this
// package's, "docker-discovery" for the container one), and a host owned by one
// must never look stale to the other.
func hasStaleDiscoveryLabel(labels map[string]string, currentKey, suffix, value string) bool {
	for k, v := range labels {
		if k == currentKey || v != value {
			continue
		}
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// StripStaleDiscoveryLabels deletes, in place, any managed-by/disabled-by
// label belonging to a PREVIOUS annotation prefix from labels. Only meaningful
// during an AnnotationPrefixMigrate relabel; harmless (a no-op) otherwise,
// since there is nothing stale to find.
func (d IngressDiscoverySettings) StripStaleDiscoveryLabels(labels map[string]string) {
	managedKey, disabledKey := d.ManagedByLabel(), d.DisabledByLabel()
	for k, v := range labels {
		if v != ManagedByIngressDiscovery {
			continue
		}
		if k == managedKey || k == disabledKey {
			continue
		}
		if strings.HasSuffix(k, "/managed-by") || strings.HasSuffix(k, "/disabled-by") {
			delete(labels, k)
		}
	}
}

// IngressProfileRule is one operator-authored routing rule: an Ingress whose
// namespace and labels match gets Profile, and the annotation is never
// consulted for it (when a rule matches). Namespace and MatchLabels are AND'd
// together; either or both may be empty to widen the match. A rule carries no
// upstream/certificate/middleware of its own - like the annotation, it can only
// SELECT a profile the operator wrote, never describe one.
type IngressProfileRule struct {
	// Namespace, if set, must equal the Ingress's namespace exactly. Empty
	// matches any namespace.
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	// MatchLabels, if set, must ALL be present on the Ingress with the same
	// value. This is a plain equality subset match, not a full
	// metav1.LabelSelector - gpm has no need for set-based operators here. Empty
	// matches any labels.
	MatchLabels map[string]string `json:"matchLabels,omitempty" yaml:"matchLabels,omitempty"`
	// Profile is the profile this rule resolves to: DefaultProfileName
	// ("template") for the default block, or a key in Profiles. Required, and
	// checked against Profiles at Settings.Validate time - a rule can never name
	// a profile that does not exist.
	Profile string `json:"profile,omitempty" yaml:"profile,omitempty"`
}

// matches reports whether rule applies to an Ingress with the given namespace
// and labels.
func (rule IngressProfileRule) matches(namespace string, labels map[string]string) bool {
	if rule.Namespace != "" && rule.Namespace != namespace {
		return false
	}
	for k, v := range rule.MatchLabels {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// ResolveProfile maps the raw gpm.rake.pro/profile annotation value onto the
// template that should shape the derived host. It returns the template, the
// resolved profile name (for the reconcile status), and whether resolution
// succeeded.
//
// The rules are deliberately blunt, because the input is untrusted:
//   - absent or whitespace-only -> the default Template, reported as "template"
//   - an EXACT match (after trimming surrounding whitespace) against a defined
//     profile -> that profile
//   - anything else -> ok=false, and the caller SKIPS the Ingress
//
// There is no prefix matching, no case folding, no nearest-neighbour guess and
// no fallback to the default: those are the ways a junk or hostile annotation
// value turns into a chain nobody chose.
func (d IngressDiscoverySettings) ResolveProfile(raw string) (IngressHostTemplate, string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return d.Template, DefaultProfileName, true
	}
	if p, ok := d.Profiles[name]; ok {
		return p, name, true
	}
	return IngressHostTemplate{}, name, false
}

// ResolveProfileFor is the entry point derive() uses. It widens ResolveProfile
// with settings.ingressDiscovery.profileRules, which let the OPERATOR route an
// Ingress to a profile with no say at all from its author - strictly stronger
// than the annotation (see design/ingress-discovery.md section 5a, option C).
// Rules are evaluated in order; the first match wins and the annotation is
// never consulted for that Ingress.
//
// When no rule matches, the behaviour depends on ProfileSelection:
//   - ProfileSelectionRulesOnly: the annotation is never read at all. No match
//     means the default template, exactly as if the Ingress carried no
//     annotation.
//   - "" / ProfileSelectionAnnotationOrRules (default): falls back to
//     ResolveProfile(raw) - today's annotation-only behaviour, unchanged.
func (d IngressDiscoverySettings) ResolveProfileFor(namespace string, labels map[string]string, raw string) (IngressHostTemplate, string, bool) {
	for _, rule := range d.ProfileRules {
		if !rule.matches(namespace, labels) {
			continue
		}
		if rule.Profile == DefaultProfileName {
			return d.Template, DefaultProfileName, true
		}
		if p, ok := d.Profiles[rule.Profile]; ok {
			return p, rule.Profile, true
		}
		// Settings.Validate refuses to save a rule naming an undefined profile,
		// so this is unreachable in practice; it still fails closed rather than
		// silently falling through to the annotation on a stale settings blob.
		return IngressHostTemplate{}, rule.Profile, false
	}
	if d.ProfileSelection == ProfileSelectionRulesOnly {
		return d.Template, DefaultProfileName, true
	}
	return d.ResolveProfile(raw)
}

// ProfileNames returns the defined profile names in sorted order, so validation
// errors and logs do not depend on map iteration order.
func (d IngressDiscoverySettings) ProfileNames() []string {
	out := make([]string, 0, len(d.Profiles))
	for k := range d.Profiles {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Interval returns the configured poll interval, or the default when unset. It
// assumes Validate has passed; an unparseable value falls back to the default
// rather than panicking a running loop.
func (d IngressDiscoverySettings) Interval() time.Duration {
	if d.PollInterval == "" {
		return defaultIngressPollInterval
	}
	v, err := time.ParseDuration(d.PollInterval)
	if err != nil || v < minimumIngressPollInterval {
		return defaultIngressPollInterval
	}
	return v
}

// Validate checks everything discovery needs before it is allowed to run. A
// disabled block is not validated at all, so a half-filled draft can sit in
// settings without blocking unrelated writes.
func (d IngressDiscoverySettings) Validate() error {
	if !d.Enabled {
		return nil
	}
	if d.APIURL != "" {
		u, err := url.Parse(d.APIURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("settings: ingressDiscovery.apiURL must be an absolute https URL, got %q", d.APIURL)
		}
	}
	for _, f := range []struct{ label, path string }{
		{"tokenFile", d.TokenFile},
		{"caFile", d.CAFile},
	} {
		if f.path != "" && !strings.HasPrefix(f.path, "/") {
			return fmt.Errorf("settings: ingressDiscovery.%s must be an absolute path, got %q", f.label, f.path)
		}
	}
	if d.Namespace != "" && !dnsLabelRe.MatchString(d.Namespace) {
		return fmt.Errorf("settings: ingressDiscovery.namespace %q is not a valid Kubernetes namespace", d.Namespace)
	}
	if d.AnnotationPrefix != "" {
		if len(d.AnnotationPrefix) > 253 || !annotationPrefixRe.MatchString(d.AnnotationPrefix) {
			return fmt.Errorf("settings: ingressDiscovery.annotationPrefix %q is not a valid annotation prefix (lowercase alphanumerics, '-' and '.', no leading/trailing dot, no slash, at most 253 characters)", d.AnnotationPrefix)
		}
	}
	if strings.ContainsAny(d.LabelSelector, "\r\n") {
		return fmt.Errorf("settings: ingressDiscovery.labelSelector must not contain newlines")
	}
	if d.PollInterval != "" {
		v, err := time.ParseDuration(d.PollInterval)
		if err != nil {
			return fmt.Errorf("settings: ingressDiscovery.pollInterval must be a Go duration (e.g. \"60s\", \"5m\"), got %q: %w", d.PollInterval, err)
		}
		if v < minimumIngressPollInterval {
			return fmt.Errorf("settings: ingressDiscovery.pollInterval must be at least %s, got %q", minimumIngressPollInterval, d.PollInterval)
		}
	}
	if len(d.AllowedDomainSuffixes) == 0 {
		return fmt.Errorf("settings: ingressDiscovery.allowedDomainSuffixes is required when discovery is enabled (it bounds which hostnames a cluster manifest can publish at the edge)")
	}
	for i, s := range d.AllowedDomainSuffixes {
		if !IsHostname(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), ".")) {
			return fmt.Errorf("settings: ingressDiscovery.allowedDomainSuffixes[%d] %q is not a valid domain suffix", i, s)
		}
	}
	// The default block and every named profile are held to IDENTICAL standards.
	// A profile is not a lightweight variant: it is a full chain an untrusted
	// Ingress can select, so an invalid one has to fail the settings WRITE rather
	// than surface as a skipped host at the next reconcile.
	if err := d.Template.validate("template"); err != nil {
		return err
	}
	for _, name := range d.ProfileNames() {
		if name == DefaultProfileName {
			return fmt.Errorf("settings: ingressDiscovery.profiles[%q] is reserved for the default template block; pick another name", name)
		}
		if err := ValidateName(name); err != nil {
			return fmt.Errorf("settings: ingressDiscovery.profiles: %w", err)
		}
		if err := d.Profiles[name].validate(fmt.Sprintf("profiles[%q]", name)); err != nil {
			return err
		}
	}
	// A per-profile allowedDomainSuffixes may only NARROW the global list, never
	// widen it - checked here, against the same block, rather than in .validate
	// (which knows nothing about the global list it belongs to).
	if err := validateSuffixSubset("template", d.Template.AllowedDomainSuffixes, d.AllowedDomainSuffixes); err != nil {
		return err
	}
	for _, name := range d.ProfileNames() {
		path := fmt.Sprintf("profiles[%q]", name)
		if err := validateSuffixSubset(path, d.Profiles[name].AllowedDomainSuffixes, d.AllowedDomainSuffixes); err != nil {
			return err
		}
	}
	switch d.ProfileSelection {
	case "", ProfileSelectionAnnotationOrRules, ProfileSelectionRulesOnly:
	default:
		return fmt.Errorf("settings: ingressDiscovery.profileSelection must be %q or %q, got %q",
			ProfileSelectionAnnotationOrRules, ProfileSelectionRulesOnly, d.ProfileSelection)
	}
	for i, rule := range d.ProfileRules {
		path := fmt.Sprintf("ingressDiscovery.profileRules[%d]", i)
		if rule.Namespace != "" && !dnsLabelRe.MatchString(rule.Namespace) {
			return fmt.Errorf("settings: %s.namespace %q is not a valid Kubernetes namespace", path, rule.Namespace)
		}
		for k := range rule.MatchLabels {
			if k == "" {
				return fmt.Errorf("settings: %s.matchLabels has an empty key", path)
			}
		}
		if rule.Profile == "" {
			return fmt.Errorf("settings: %s.profile is required", path)
		}
		if rule.Profile != DefaultProfileName {
			if _, ok := d.Profiles[rule.Profile]; !ok {
				return fmt.Errorf("settings: %s.profile %q is not defined in ingressDiscovery.profiles", path, rule.Profile)
			}
		}
	}
	return nil
}

// validateSuffixSubset checks that narrow is a well-formed domain-suffix list
// and, when non-empty, a SUBSET of global - every entry equal to, or a
// dot-boundary sub-suffix of, some global entry. path names the settings
// location (e.g. "template" or `profiles["sso-internal"]`) for the error.
func validateSuffixSubset(path string, narrow, global []string) error {
	for i, s := range narrow {
		if !IsHostname(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), ".")) {
			return fmt.Errorf("settings: ingressDiscovery.%s.allowedDomainSuffixes[%d] %q is not a valid domain suffix", path, i, s)
		}
	}
	if bad, ok := suffixesSubsetOf(narrow, global); !ok {
		return fmt.Errorf("settings: ingressDiscovery.%s.allowedDomainSuffixes %q is not covered by the global allowedDomainSuffixes (a profile may only narrow the global list, never widen it)", path, bad)
	}
	return nil
}

// suffixesSubsetOf reports whether every entry in narrow is already covered by
// some entry in global - i.e. every hostname the narrow list would allow, the
// global list would also allow. Returns the first offending entry on failure.
func suffixesSubsetOf(narrow, global []string) (string, bool) {
	for _, n := range narrow {
		if !matchesSuffixList(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(n)), "."), global) {
			return n, false
		}
	}
	return "", true
}

// ValidateRefs cross-checks every object the discovery block NAMES - the
// certificate, the upstream group, each middleware, each access list, and the
// mTLS trust anchor - against the objects that actually exist in cfg.
//
// Validate() above only checks name SHAPE, because a Settings value knows
// nothing about the rest of the config. That was enough to write a settings
// object that passes and then wedges discovery permanently: the reconciler
// stamps the dangling ref onto every derived host, the store validates the whole
// merged graph as ONE batch, and the batch is rejected - so every other tenant's
// create, update and delete is dropped too, on every poll, forever, surfacing
// only as an opaque batch-validation error in the reconcile status. Landing the
// error on the operator's own write instead is the difference between a typo and
// an outage.
//
// Like Validate, a disabled block is not checked at all, so a half-filled draft
// can sit in settings without blocking unrelated writes.
func (d IngressDiscoverySettings) ValidateRefs(cfg Config) error {
	if !d.Enabled {
		return nil
	}
	if err := d.checkAnnotationPrefixMigration(cfg); err != nil {
		return err
	}
	certs := map[string]bool{}
	for _, o := range cfg.Certificates {
		certs[o.Name] = true
	}
	mws := map[string]bool{}
	for _, o := range cfg.Middlewares {
		mws[o.Name] = true
	}
	als := map[string]bool{}
	for _, o := range cfg.AccessLists {
		als[o.Name] = true
	}
	ugs := map[string]bool{}
	for _, o := range cfg.UpstreamGroups {
		ugs[o.Name] = true
	}
	clientCAs := map[string]bool{}
	disabledClientCAs := map[string]bool{}
	for _, o := range cfg.ClientCAs {
		clientCAs[o.Name] = true
		if o.Disabled {
			disabledClientCAs[o.Name] = true
		}
	}

	var errs []error
	check := func(path string, t IngressHostTemplate) {
		owner := "settings.ingressDiscovery." + path
		checkRef(&errs, "ingress discovery", owner, "certificate", t.TLS.CertificateRef, certs)
		checkRef(&errs, "ingress discovery", owner, "upstreamGroup", t.UpstreamGroupRef, ugs)
		checkClientAuthRef(&errs, "ingress discovery", owner, t.TLS, clientCAs, disabledClientCAs)
		for _, m := range t.Middlewares {
			checkRef(&errs, "ingress discovery", owner, "middleware", m, mws)
		}
		for _, a := range t.AccessLists {
			checkRef(&errs, "ingress discovery", owner, "accessList", a, als)
		}
	}
	check(DefaultProfileName, d.Template)
	for _, name := range d.ProfileNames() {
		check(fmt.Sprintf("profiles[%q]", name), d.Profiles[name])
	}
	return errors.Join(errs...)
}

// checkAnnotationPrefixMigration refuses a settings write that would silently
// orphan every proxy host discovery already manages: ownership is recognised
// ONLY under the CURRENT prefix (see ManagedByLabel), so saving a changed
// prefix while old-labelled hosts still exist would make the very next
// reconcile treat them as unmanaged - an operator-authored host with the same
// name, never touched - and, once their Ingress stops deriving to a name
// discovery still owns, potentially never clean up either. AnnotationPrefix
// starting empty (the default) counts too: this refuses the FIRST prefix
// change exactly like every later one.
//
// AnnotationPrefixMigrate:true opts out of the refusal; the relabel itself
// happens in the next reconcile (see internal/k8s/discovery.go), as an
// ordinary update in that run's one commit, never as a side effect of this
// settings save.
func (d IngressDiscoverySettings) checkAnnotationPrefixMigration(cfg Config) error {
	if d.AnnotationPrefixMigrate {
		return nil
	}
	stale := 0
	for _, h := range cfg.ProxyHosts {
		if d.HasStaleManagedByLabel(h.Labels) {
			stale++
		}
	}
	if stale == 0 {
		return nil
	}
	return fmt.Errorf("settings: ingressDiscovery.annotationPrefix %q would orphan %d proxy host(s) still labelled managed-by under a different prefix; "+
		"re-label them first, or set ingressDiscovery.annotationPrefixMigrate: true to relabel them automatically in the next reconcile", d.Prefix(), stale)
}

// validate checks one derived-host shape. path is the settings location used in
// error messages ("template" or `profiles["sso-internal"]`), so an operator can
// tell which block they broke.
func (t IngressHostTemplate) validate(path string) error {
	return t.validateWith(path, true)
}

// validateWith is validate with the upstream requirement made optional, for the
// Docker block: there the upstream is the CONTAINER's own address, derived per
// object, so a template needs nothing in that field - and a profile SHARED with
// ingressDiscovery (the default: dockerDiscovery.profiles falls back to
// ingressDiscovery.profiles) carries the cluster controller's address, which
// container discovery simply ignores. An upstream that IS present is still
// shape-checked, so a typo in a shared profile still fails the settings write.
// Every other rule - TLS, timeouts, middlewares, access lists, stripped headers
// - is identical, and deliberately so: the two blocks take the same type and
// must be held to the same standard.
func (t IngressHostTemplate) validateWith(path string, requireUpstream bool) error {
	if !requireUpstream && t.UpstreamGroupRef == "" && t.Upstream == (Upstream{}) {
		// The upstream is derived per object; there is nothing to validate.
	} else if t.UpstreamGroupRef == "" {
		if err := t.Upstream.validate(); err != nil {
			return fmt.Errorf("settings: ingressDiscovery.%s.upstream: %w", path, err)
		}
	} else {
		if t.Upstream != (Upstream{}) {
			return fmt.Errorf("settings: ingressDiscovery.%s: upstream and upstreamGroupRef are mutually exclusive", path)
		}
		if err := ValidateName(t.UpstreamGroupRef); err != nil {
			return fmt.Errorf("settings: ingressDiscovery.%s.upstreamGroupRef: %w", path, err)
		}
	}
	if err := ValidateName(t.TLS.CertificateRef); err != nil {
		return fmt.Errorf("settings: ingressDiscovery.%s.tls.certificateRef is required when discovery is enabled (discovery never issues per-host certificates): %w", path, err)
	}
	if err := t.TLS.validate(); err != nil {
		return fmt.Errorf("settings: ingressDiscovery.%s.tls: %w", path, err)
	}
	// The SAME helper ProxyHost.Validate uses, not a re-statement of its rules: a
	// template that would produce a host the config validator rejects has to fail
	// the settings WRITE, and it has to fail it for the same reasons.
	if err := t.Timeouts.validate(); err != nil {
		return fmt.Errorf("settings: ingressDiscovery.%s: %w", path, err)
	}
	for i, m := range t.Middlewares {
		if err := ValidateName(m); err != nil {
			return fmt.Errorf("settings: ingressDiscovery.%s.middlewares[%d]: %w", path, i, err)
		}
	}
	for i, a := range t.AccessLists {
		if err := ValidateName(a); err != nil {
			return fmt.Errorf("settings: ingressDiscovery.%s.accessLists[%d]: %w", path, i, err)
		}
	}
	// Same helper the ProxyHost and Settings paths use, for the same reason as
	// Timeouts above: a template that would derive a host the config validator
	// rejects must fail the settings write.
	if err := validateStripResponseHeaders(t.StripResponseHeaders); err != nil {
		return fmt.Errorf("settings: ingressDiscovery.%s: %w", path, err)
	}
	if err := validateSecurityHeaders(t.SecurityHeaders); err != nil {
		return fmt.Errorf("settings: ingressDiscovery.%s: %w", path, err)
	}
	return nil
}

// AllowedDomain reports whether name falls inside the configured GLOBAL suffix
// list (exact match, or a dot-boundary suffix so "evilexample.com" never
// matches the suffix "example.com"). The name is expected to be already
// normalised by NormalizeHostname.
func (d IngressDiscoverySettings) AllowedDomain(name string) bool {
	return matchesSuffixList(name, d.AllowedDomainSuffixes)
}

// AllowedDomainFor reports whether name is publishable under tmpl: tmpl's own
// AllowedDomainSuffixes when it sets any (already validated as a SUBSET of the
// global list at Settings.Validate), otherwise the global list. The name is
// expected to be already normalised by NormalizeHostname.
func (d IngressDiscoverySettings) AllowedDomainFor(tmpl IngressHostTemplate, name string) bool {
	if len(tmpl.AllowedDomainSuffixes) > 0 {
		return matchesSuffixList(name, tmpl.AllowedDomainSuffixes)
	}
	return d.AllowedDomain(name)
}

// matchesSuffixList reports whether name equals, or is a dot-boundary
// sub-domain of, some entry in suffixes.
func matchesSuffixList(name string, suffixes []string) bool {
	for _, raw := range suffixes {
		suffix := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), ".")
		if suffix == "" {
			continue
		}
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return false
}
