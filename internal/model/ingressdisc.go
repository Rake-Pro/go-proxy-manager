package model

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Kubernetes Ingress annotations gpm reads. Opt-in is EXACTLY
// AnnotationManaged == "true"; an Ingress carrying anything else (or nothing)
// is invisible to discovery. There is no opt-out mode and no namespace sweep.
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
	// the default. See docs/design/ingress-discovery.md §5a.
	AnnotationProfile = "gpm.rake.pro/profile"
)

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
	// set. See docs/design/ingress-discovery.md §1.
	TLS TLSSettings `json:"tls,omitempty" yaml:"tls,omitempty"`

	WebsocketsUpgrade bool `json:"websocketsUpgrade,omitempty" yaml:"websocketsUpgrade,omitempty"`

	// Middlewares/AccessLists are applied to every derived host, in this order.
	Middlewares []string `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	AccessLists []string `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`

	// DefaultDNS is the dns policy a derived host gets when the corresponding
	// annotation is absent. Each flag is overridden individually by its
	// annotation, so a template default of lanDirect can be turned off per
	// Ingress with gpm.rake.pro/lan-direct: "false".
	DefaultDNS *DNSSyncPolicy `json:"defaultDNS,omitempty" yaml:"defaultDNS,omitempty"`
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
	return nil
}

// validate checks one derived-host shape. path is the settings location used in
// error messages ("template" or `profiles["sso-internal"]`), so an operator can
// tell which block they broke.
func (t IngressHostTemplate) validate(path string) error {
	if t.UpstreamGroupRef == "" {
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
	return nil
}

// AllowedDomain reports whether name falls inside the configured suffix list
// (exact match, or a dot-boundary suffix so "evilexample.com" never matches the
// suffix "example.com"). The name is expected to be already normalised by
// NormalizeHostname.
func (d IngressDiscoverySettings) AllowedDomain(name string) bool {
	for _, raw := range d.AllowedDomainSuffixes {
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
