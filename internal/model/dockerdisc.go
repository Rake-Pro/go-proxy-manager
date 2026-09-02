package model

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Docker container labels gpm reads under the DEFAULT prefix. Opt-in is EXACTLY
// LabelDockerEnabled == "true"; a container carrying anything else (or nothing)
// is invisible to discovery. There is no opt-out mode and no sweep of every
// running container.
//
// These constants are the DEFAULT prefix's keys only. Code that must honour a
// possibly-customised prefix uses the DockerDiscoverySettings methods below
// (LabelEnabled(), ManagedByLabel(), ...) instead.
//
// The set deliberately MIRRORS the Kubernetes annotation contract: same prefix,
// same profile-by-name rule, same two DNS booleans. What a container adds is
// the two things an Ingress carries in its spec instead of an annotation -
// which hostnames to serve, and which container port to forward to.
const (
	// LabelDockerEnabled opts a container into gpm discovery.
	LabelDockerEnabled = "gpm.rake.pro/enabled"
	// LabelDockerDomains is the comma-separated hostname list to serve.
	LabelDockerDomains = "gpm.rake.pro/domains"
	// LabelDockerPort is the CONTAINER port to forward to (the published port
	// when usePublishedPorts is on).
	LabelDockerPort = "gpm.rake.pro/port"
	// LabelDockerScheme is the upstream scheme, http (default) or https.
	LabelDockerScheme = "gpm.rake.pro/scheme"
	// LabelDockerProfile NAMES one of the operator-defined profiles, exactly like
	// the Kubernetes gpm.rake.pro/profile annotation: a name and nothing else.
	LabelDockerProfile = "gpm.rake.pro/profile"
	// LabelDockerLanDirect / LabelDockerPublicCname set the derived host's dns
	// policy flags, overriding the resolved profile's defaultDNS.
	LabelDockerLanDirect   = "gpm.rake.pro/lan-direct"
	LabelDockerPublicCname = "gpm.rake.pro/public-cname"
)

// ManagedByDockerDiscovery / DisabledByDockerDiscovery are the managed-by and
// disabled-by label VALUES the container reconciler writes on the proxy hosts
// it owns. The key is the same <prefix>/managed-by the Ingress reconciler uses;
// only the value differs, and that is what keeps the two apart: each recognises
// ownership by exact value, so neither can ever update or delete a host derived
// by the other.
const (
	ManagedByDockerDiscovery  = "docker-discovery"
	DisabledByDockerDiscovery = "docker-discovery"
)

// Docker connection and derivation defaults.
const (
	// DefaultDockerSocket is the Engine API socket used when neither socket nor
	// host is configured.
	DefaultDockerSocket = "/var/run/docker.sock"
	// DefaultDockerPublishedHost is where a published container port is reached
	// when usePublishedPorts is on and publishedHost is unset: the loopback
	// address, because gpm then runs ON the Docker host.
	DefaultDockerPublishedHost = "127.0.0.1"
	// DockerHostNamePrefix prefixes every derived proxy-host name, so a derived
	// object is recognisable in the host list and cannot collide with the
	// Ingress reconciler's "ing-" names.
	DockerHostNamePrefix = "dkr-"

	defaultDockerPollInterval = time.Minute
	minimumDockerPollInterval = 15 * time.Second
)

// DockerDiscoverySettings configures discovery of labelled Docker containers
// into template-derived, managed-labelled proxy hosts - the container-native
// half of Ingress discovery, for the far more common homelab deployment where
// the workloads are compose services rather than cluster Ingresses.
//
// Everything security-relevant is shared with settings.ingressDiscovery: the
// same IngressHostTemplate shape, the same profiles (this block's are optional
// and fall back to ingressDiscovery.profiles), the same label prefix, and the
// same rule that a container may only SELECT a chain the operator wrote, never
// describe one. Disabled (the default) means the subsystem is inert and never
// opens the socket.
type DockerDiscoverySettings struct {
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Socket is the Docker Engine API unix socket path. Empty means
	// DefaultDockerSocket. Ignored when Host is set.
	Socket string `json:"socket,omitempty" yaml:"socket,omitempty"`
	// Host is a tcp:// (or https://) Engine API endpoint, used INSTEAD of the
	// socket - the recommended shape is a read-only socket proxy, so gpm never
	// gets the raw socket at all. See docs/how-to/docker-discovery.md.
	Host string `json:"host,omitempty" yaml:"host,omitempty"`
	// TLSCert/TLSKey/TLSCA are absolute paths to a client certificate, its key
	// and the CA bundle that verifies the endpoint, for a TLS-protected Host.
	// They are paths rather than secret placeholders for the same reason
	// ingressDiscovery.tokenFile is: the credential stays out of the config repo.
	TLSCert string `json:"tlsCert,omitempty" yaml:"tlsCert,omitempty"`
	TLSKey  string `json:"tlsKey,omitempty" yaml:"tlsKey,omitempty"`
	TLSCA   string `json:"tlsCA,omitempty" yaml:"tlsCA,omitempty"`

	// Network is the Docker network whose per-container IP address becomes the
	// upstream host. Empty picks the container's first network by name, skipping
	// "host" and "none". Ignored when UsePublishedPorts is set.
	Network string `json:"network,omitempty" yaml:"network,omitempty"`
	// UsePublishedPorts forwards to the HOST-published port on PublishedHost
	// instead of the container IP on Network. Use it when gpm runs on the Docker
	// host rather than inside a shared Docker network; see
	// docs/reference/config/settings/docker-discovery.md for the trade-off.
	UsePublishedPorts bool `json:"usePublishedPorts,omitempty" yaml:"usePublishedPorts,omitempty"`
	// PublishedHost is the address a published port is reached on. Empty means
	// DefaultDockerPublishedHost. Only read when UsePublishedPorts is set.
	PublishedHost string `json:"publishedHost,omitempty" yaml:"publishedHost,omitempty"`
	// IncludeStopped lists non-running containers too. Off by default: a stopped
	// container has no IP and no published port, so a host derived from it would
	// serve 502s rather than the maintenance page an operator would have chosen.
	IncludeStopped bool `json:"includeStopped,omitempty" yaml:"includeStopped,omitempty"`

	// PollInterval is the FALLBACK loop interval (Go duration, empty means 1m,
	// floor 15s). Reconciles are normally driven by the Engine event stream; the
	// poll is what repairs drift if the stream is missed entirely.
	PollInterval string `json:"pollInterval,omitempty" yaml:"pollInterval,omitempty"`

	// AllowedDomainSuffixes bounds which hostnames discovery will ever publish,
	// exactly as in ingressDiscovery. REQUIRED when enabled: a container label is
	// as untrusted as an Ingress annotation, and unrestricted discovery is one
	// compose file away from claiming somebody else's name at the edge.
	AllowedDomainSuffixes []string `json:"allowedDomainSuffixes,omitempty" yaml:"allowedDomainSuffixes,omitempty"`

	// Template is the DEFAULT profile: the shape a container gets when it names
	// no profile. It is the SAME type as ingressDiscovery.template, validated by
	// the same rules, so a chain written once can be pasted into either block.
	Template IngressHostTemplate `json:"template,omitempty" yaml:"template,omitempty"`

	// Profiles are additional named chains a container may SELECT with
	// <prefix>/profile. EMPTY (the normal case) means "use
	// ingressDiscovery.profiles": the profile vocabulary is a property of the
	// deployment, not of the discovery source, and duplicating it would mean two
	// places to tighten when a chain changes. Set it only to give containers a
	// deliberately different set from cluster Ingresses.
	Profiles map[string]IngressHostTemplate `json:"profiles,omitempty" yaml:"profiles,omitempty"`

	// LabelPrefix and PrefixMigrate are RESOLVED fields, filled in from
	// settings.ingressDiscovery by Settings.DockerDiscoveryResolved. They are
	// never serialised: there is exactly one annotation/label prefix per
	// deployment, and a second knob would let the two reconcilers disagree about
	// which labels mark ownership.
	LabelPrefix   string `json:"-" yaml:"-"`
	PrefixMigrate bool   `json:"-" yaml:"-"`
}

// DockerDiscoveryResolved returns the docker discovery block with the fields it
// shares with ingressDiscovery filled in: the label prefix, the prefix-migration
// flag, and (when the block defines none of its own) the profile set. Every
// caller - validation, the reconciler, the API - uses this rather than the raw
// struct, so "docker discovery inherits the prefix and the profiles" is stated
// once.
func (s Settings) DockerDiscoveryResolved() DockerDiscoverySettings {
	d := s.DockerDiscovery
	d.LabelPrefix = s.IngressDiscovery.Prefix()
	d.PrefixMigrate = s.IngressDiscovery.AnnotationPrefixMigrate
	if len(d.Profiles) == 0 {
		d.Profiles = s.IngressDiscovery.Profiles
	}
	return d
}

// Prefix returns the resolved annotation/label prefix, or the default when the
// block was not resolved through Settings.DockerDiscoveryResolved.
func (d DockerDiscoverySettings) Prefix() string {
	if d.LabelPrefix == "" {
		return DefaultAnnotationPrefix
	}
	return d.LabelPrefix
}

// The CURRENT-prefix label keys gpm READS off a container.
func (d DockerDiscoverySettings) LabelEnabled() string { return d.Prefix() + "/enabled" }
func (d DockerDiscoverySettings) LabelDomains() string { return d.Prefix() + "/domains" }
func (d DockerDiscoverySettings) LabelPort() string    { return d.Prefix() + "/port" }
func (d DockerDiscoverySettings) LabelScheme() string  { return d.Prefix() + "/scheme" }
func (d DockerDiscoverySettings) LabelProfile() string { return d.Prefix() + "/profile" }
func (d DockerDiscoverySettings) LabelLanDirect() string {
	return d.Prefix() + "/lan-direct"
}
func (d DockerDiscoverySettings) LabelPublicCname() string { return d.Prefix() + "/public-cname" }

// ManagedByLabel / DisabledByLabel are the CURRENT-prefix label keys gpm WRITES
// on the proxy hosts it derives. The keys are shared with Ingress discovery;
// the values (ManagedByDockerDiscovery) are not.
func (d DockerDiscoverySettings) ManagedByLabel() string  { return d.Prefix() + "/managed-by" }
func (d DockerDiscoverySettings) DisabledByLabel() string { return d.Prefix() + "/disabled-by" }

// HasStaleManagedByLabel reports whether labels carries a docker-discovery
// managed-by label under a prefix OTHER than the current one - the same
// orphan-detection Ingress discovery does, keyed on this reconciler's own label
// value so the two never see each other's hosts.
func (d DockerDiscoverySettings) HasStaleManagedByLabel(labels map[string]string) bool {
	return hasStaleDiscoveryLabel(labels, d.ManagedByLabel(), "/managed-by", ManagedByDockerDiscovery)
}

// HasStaleDisabledByLabel is HasStaleManagedByLabel's counterpart for the
// disabled-by label.
func (d DockerDiscoverySettings) HasStaleDisabledByLabel(labels map[string]string) bool {
	return hasStaleDiscoveryLabel(labels, d.DisabledByLabel(), "/disabled-by", DisabledByDockerDiscovery)
}

// StripStaleDiscoveryLabels deletes, in place, any docker-discovery
// managed-by/disabled-by label belonging to a PREVIOUS prefix. Only meaningful
// during a prefix migration; a no-op otherwise.
func (d DockerDiscoverySettings) StripStaleDiscoveryLabels(labels map[string]string) {
	managedKey, disabledKey := d.ManagedByLabel(), d.DisabledByLabel()
	for k, v := range labels {
		if v != ManagedByDockerDiscovery {
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

// ResolveProfile maps the raw <prefix>/profile label value onto the template
// that shapes the derived host, with exactly the rules ingressDiscovery uses:
// absent or blank means the default Template; an EXACT match (after trimming)
// means that profile, verbatim; anything else fails, and the caller SKIPS the
// container rather than quietly applying a chain nobody chose.
func (d DockerDiscoverySettings) ResolveProfile(raw string) (IngressHostTemplate, string, bool) {
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
func (d DockerDiscoverySettings) ProfileNames() []string {
	out := make([]string, 0, len(d.Profiles))
	for k := range d.Profiles {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Interval returns the configured fallback poll interval, or the default when
// unset. It assumes Validate has passed; an unparseable value falls back to the
// default rather than panicking a running loop.
func (d DockerDiscoverySettings) Interval() time.Duration {
	if d.PollInterval == "" {
		return defaultDockerPollInterval
	}
	v, err := time.ParseDuration(d.PollInterval)
	if err != nil || v < minimumDockerPollInterval {
		return defaultDockerPollInterval
	}
	return v
}

// SocketPath returns the unix socket to dial when no Host is configured.
func (d DockerDiscoverySettings) SocketPath() string {
	if d.Socket == "" {
		return DefaultDockerSocket
	}
	return d.Socket
}

// PublishedAddress returns the address a published container port is reached
// on. Only meaningful with UsePublishedPorts.
func (d DockerDiscoverySettings) PublishedAddress() string {
	if d.PublishedHost == "" {
		return DefaultDockerPublishedHost
	}
	return d.PublishedHost
}

// AllowedDomain reports whether name falls inside the configured GLOBAL suffix
// list (exact match, or a dot-boundary suffix). The name is expected to be
// already normalised by NormalizeHostname.
func (d DockerDiscoverySettings) AllowedDomain(name string) bool {
	return matchesSuffixList(name, d.AllowedDomainSuffixes)
}

// AllowedDomainFor reports whether name is publishable under tmpl: tmpl's own
// AllowedDomainSuffixes when it sets any (validated as a SUBSET of the global
// list at Settings.Validate), otherwise the global list.
func (d DockerDiscoverySettings) AllowedDomainFor(tmpl IngressHostTemplate, name string) bool {
	if len(tmpl.AllowedDomainSuffixes) > 0 {
		return matchesSuffixList(name, tmpl.AllowedDomainSuffixes)
	}
	return d.AllowedDomain(name)
}

// Validate checks everything container discovery needs before it is allowed to
// run. A disabled block is not validated at all, so a half-filled draft can sit
// in settings without blocking unrelated writes.
func (d DockerDiscoverySettings) Validate() error {
	if !d.Enabled {
		return nil
	}
	if d.Host != "" {
		u, err := url.Parse(d.Host)
		if err != nil || u.Host == "" || (u.Scheme != "tcp" && u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("settings: dockerDiscovery.host must be an absolute tcp:// or https:// URL (e.g. tcp://socket-proxy:2375), got %q", d.Host)
		}
		if u.Scheme != "https" && (d.TLSCert != "" || d.TLSKey != "") {
			return fmt.Errorf("settings: dockerDiscovery.tlsCert/tlsKey need an https:// host; %q is plaintext", d.Host)
		}
	} else if d.Socket != "" && !strings.HasPrefix(d.Socket, "/") {
		return fmt.Errorf("settings: dockerDiscovery.socket must be an absolute path, got %q", d.Socket)
	}
	if (d.TLSCert == "") != (d.TLSKey == "") {
		return errors.New("settings: dockerDiscovery.tlsCert and dockerDiscovery.tlsKey must be set together")
	}
	for _, f := range []struct{ label, path string }{
		{"tlsCert", d.TLSCert},
		{"tlsKey", d.TLSKey},
		{"tlsCA", d.TLSCA},
	} {
		if f.path != "" && !strings.HasPrefix(f.path, "/") {
			return fmt.Errorf("settings: dockerDiscovery.%s must be an absolute path, got %q", f.label, f.path)
		}
	}
	if d.PublishedHost != "" && !d.UsePublishedPorts {
		return errors.New("settings: dockerDiscovery.publishedHost only applies with dockerDiscovery.usePublishedPorts: true")
	}
	if d.PublishedHost != "" && strings.ContainsAny(d.PublishedHost, " /\r\n") {
		return fmt.Errorf("settings: dockerDiscovery.publishedHost %q is not a bare host or IP address", d.PublishedHost)
	}
	if d.Network != "" && d.UsePublishedPorts {
		return errors.New("settings: dockerDiscovery.network and dockerDiscovery.usePublishedPorts are mutually exclusive (one forwards to a container IP, the other to a host-published port)")
	}
	if d.PollInterval != "" {
		v, err := time.ParseDuration(d.PollInterval)
		if err != nil {
			return fmt.Errorf("settings: dockerDiscovery.pollInterval must be a Go duration (e.g. \"60s\", \"5m\"), got %q: %w", d.PollInterval, err)
		}
		if v < minimumDockerPollInterval {
			return fmt.Errorf("settings: dockerDiscovery.pollInterval must be at least %s, got %q", minimumDockerPollInterval, d.PollInterval)
		}
	}
	if len(d.AllowedDomainSuffixes) == 0 {
		return errors.New("settings: dockerDiscovery.allowedDomainSuffixes is required when discovery is enabled (it bounds which hostnames a container label can publish at the edge)")
	}
	for i, s := range d.AllowedDomainSuffixes {
		if !IsHostname(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), ".")) {
			return fmt.Errorf("settings: dockerDiscovery.allowedDomainSuffixes[%d] %q is not a valid domain suffix", i, s)
		}
	}
	// The default block and every named profile are held to IDENTICAL standards,
	// by the same helper the ingressDiscovery block uses: a profile is a full
	// chain an untrusted container can select, so an invalid one has to fail the
	// settings WRITE rather than surface as a skipped host at the next reconcile.
	if err := d.Template.validateWith("template", false); err != nil {
		return dockerSettingsErr(err)
	}
	for _, name := range d.ProfileNames() {
		if name == DefaultProfileName {
			return fmt.Errorf("settings: dockerDiscovery.profiles[%q] is reserved for the default template block; pick another name", name)
		}
		if err := ValidateName(name); err != nil {
			return fmt.Errorf("settings: dockerDiscovery.profiles: %w", err)
		}
		if err := d.Profiles[name].validateWith(fmt.Sprintf("profiles[%q]", name), false); err != nil {
			return dockerSettingsErr(err)
		}
	}
	if err := validateSuffixSubset("template", d.Template.AllowedDomainSuffixes, d.AllowedDomainSuffixes); err != nil {
		return dockerSettingsErr(err)
	}
	for _, name := range d.ProfileNames() {
		path := fmt.Sprintf("profiles[%q]", name)
		if err := validateSuffixSubset(path, d.Profiles[name].AllowedDomainSuffixes, d.AllowedDomainSuffixes); err != nil {
			return dockerSettingsErr(err)
		}
	}
	return nil
}

// dockerSettingsErr re-points an error raised by the SHARED template validators
// (which name the ingressDiscovery block, because that is where the type was
// introduced) at the docker block, so an operator is told which of the two
// blocks they actually broke.
func dockerSettingsErr(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(strings.Replace(err.Error(), "settings: ingressDiscovery.", "settings: dockerDiscovery.", 1))
}

// ValidateRefs cross-checks every object the docker discovery block NAMES - the
// certificate, the upstream group, each middleware, each access list and the
// mTLS trust anchor - against the objects that actually exist in cfg, for the
// same reason ingressDiscovery does: a dangling ref there is stamped onto every
// derived host and rejects the whole reconcile batch on every poll, dropping
// unrelated changes with it. A disabled block is not checked at all.
func (d DockerDiscoverySettings) ValidateRefs(cfg Config) error {
	if !d.Enabled {
		return nil
	}
	if err := d.checkPrefixMigration(cfg); err != nil {
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
		owner := "settings.dockerDiscovery." + path
		checkRef(&errs, "docker discovery", owner, "certificate", t.TLS.CertificateRef, certs)
		checkRef(&errs, "docker discovery", owner, "upstreamGroup", t.UpstreamGroupRef, ugs)
		checkClientAuthRef(&errs, "docker discovery", owner, t.TLS, clientCAs, disabledClientCAs)
		for _, m := range t.Middlewares {
			checkRef(&errs, "docker discovery", owner, "middleware", m, mws)
		}
		for _, a := range t.AccessLists {
			checkRef(&errs, "docker discovery", owner, "accessList", a, als)
		}
	}
	check(DefaultProfileName, d.Template)
	for _, name := range d.ProfileNames() {
		check(fmt.Sprintf("profiles[%q]", name), d.Profiles[name])
	}
	return errors.Join(errs...)
}

// checkPrefixMigration refuses a settings write that would silently orphan
// every container-derived proxy host, exactly as the ingressDiscovery block
// does for its own: ownership is recognised ONLY under the current prefix, and
// the prefix is shared, so a prefix change has to account for BOTH reconcilers'
// hosts. ingressDiscovery.annotationPrefixMigrate opts out of the refusal for
// both, and the relabel happens in each reconciler's next run.
func (d DockerDiscoverySettings) checkPrefixMigration(cfg Config) error {
	if d.PrefixMigrate {
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
	return fmt.Errorf("settings: ingressDiscovery.annotationPrefix %q would orphan %d container-derived proxy host(s) still labelled managed-by under a different prefix; "+
		"re-label them first, or set ingressDiscovery.annotationPrefixMigrate: true to relabel them automatically in the next reconcile", d.Prefix(), stale)
}
