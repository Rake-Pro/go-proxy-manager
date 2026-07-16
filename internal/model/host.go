package model

import "fmt"

// Upstream is a single backend target a proxy forwards to.
type Upstream struct {
	Scheme string `json:"scheme" yaml:"scheme"` // http | https
	Host   string `json:"host" yaml:"host"`
	Port   int    `json:"port" yaml:"port"`
}

func (u Upstream) validate() error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("upstream scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("upstream host is required")
	}
	if u.Port < 1 || u.Port > 65535 {
		return fmt.Errorf("upstream port %d out of range", u.Port)
	}
	return nil
}

// HSTS configures HTTP Strict Transport Security for a host.
type HSTS struct {
	Enabled           bool `json:"enabled" yaml:"enabled"`
	MaxAge            int  `json:"maxAge,omitempty" yaml:"maxAge,omitempty"` // seconds
	IncludeSubdomains bool `json:"includeSubdomains,omitempty" yaml:"includeSubdomains,omitempty"`
	Preload           bool `json:"preload,omitempty" yaml:"preload,omitempty"`
}

// TLSSettings holds per-host TLS termination options. CertificateRef names a
// Certificate object; the cert content itself lives in the cert store.
type TLSSettings struct {
	CertificateRef string `json:"certificateRef,omitempty" yaml:"certificateRef,omitempty"`
	ForceSSL       bool   `json:"forceSSL,omitempty" yaml:"forceSSL,omitempty"` // redirect http->https
	HTTP2          bool   `json:"http2,omitempty" yaml:"http2,omitempty"`
	HSTS           HSTS   `json:"hsts,omitempty" yaml:"hsts,omitempty"`
	// MinTLSVersion is the lowest TLS version this host accepts: "1.2" (default,
	// empty) or "1.3". Set "1.3" only where every client supports it; the edge
	// otherwise negotiates 1.2 or 1.3 per client with a 1.2 floor.
	MinTLSVersion string `json:"minTLSVersion,omitempty" yaml:"minTLSVersion,omitempty"`
	// ClientAuth opts this host into mTLS: presented client certificates are
	// verified at the TLS handshake against a referenced ClientCA. nil (default)
	// means no client-certificate requirement.
	ClientAuth *ClientAuth `json:"clientAuth,omitempty" yaml:"clientAuth,omitempty"`
}

// ClientAuth configures per-host mTLS client-certificate verification. CARef
// names the ClientCA trust anchor; Mode selects handshake enforcement.
type ClientAuth struct {
	// CARef names the ClientCA object whose bundle verifies client certificates.
	CARef string `json:"caRef" yaml:"caRef"`
	// Mode is "require" (default) or "optional". require rejects the handshake
	// unless a certificate valid against CARef is presented; optional verifies a
	// presented certificate but lets certless requests proceed (so mTLS can be a
	// fallback alongside SSO/forward-auth).
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`
}

func (t TLSSettings) validate() error {
	switch t.MinTLSVersion {
	case "", "1.2", "1.3":
	default:
		return fmt.Errorf(`tls.minTLSVersion must be "1.2" or "1.3", got %q`, t.MinTLSVersion)
	}
	if t.ClientAuth != nil {
		// mTLS must never be servable in the clear: require forceSSL so the plaintext
		// listener redirects to HTTPS where the per-request client-cert gate applies.
		if !t.ForceSSL {
			return fmt.Errorf("tls.clientAuth requires forceSSL: true")
		}
		if t.ClientAuth.CARef == "" {
			return fmt.Errorf("tls.clientAuth.caRef is required when clientAuth is set")
		}
		switch t.ClientAuth.Mode {
		case "", "require", "optional":
		default:
			return fmt.Errorf(`tls.clientAuth.mode must be "require" or "optional", got %q`, t.ClientAuth.Mode)
		}
	}
	return nil
}

// HostTimeouts overrides the default upstream timeouts for a single proxy host.
// Zero fields fall back to the shared transport's defaults. A non-nil value makes
// the host use its own cloned transport (with its own connection pool) so the
// override cannot affect any other host. Values are whole seconds.
type HostTimeouts struct {
	// ConnectSeconds caps establishing the TCP/TLS connection to the upstream.
	ConnectSeconds int `json:"connectSeconds,omitempty" yaml:"connectSeconds,omitempty"`
	// ReadSeconds caps time awaiting the upstream's response headers
	// (time-to-first-byte). It does not bound a slow streaming body once headers
	// have arrived, so it stays safe for SSE / long-poll / websocket upstreams.
	ReadSeconds int `json:"readSeconds,omitempty" yaml:"readSeconds,omitempty"`
}

func (t *HostTimeouts) validate() error {
	if t == nil {
		return nil
	}
	if t.ConnectSeconds < 0 || t.ConnectSeconds > 3600 {
		return fmt.Errorf("timeouts.connectSeconds %d out of range (0-3600)", t.ConnectSeconds)
	}
	if t.ReadSeconds < 0 || t.ReadSeconds > 3600 {
		return fmt.Errorf("timeouts.readSeconds %d out of range (0-3600)", t.ReadSeconds)
	}
	return nil
}

// Location is a path-scoped override within a proxy host. Locations carry their
// own upstream (a single backend OR an upstream-group reference; at most one)
// and their own ordered middleware/access-list references, so per-location auth
// and access control are first-class config, not text snippets. A location with
// neither inherits the host's backend.
type Location struct {
	Path             string    `json:"path" yaml:"path"`
	Upstream         *Upstream `json:"upstream,omitempty" yaml:"upstream,omitempty"`
	UpstreamGroupRef string    `json:"upstreamGroupRef,omitempty" yaml:"upstreamGroupRef,omitempty"`
	Middlewares      []string  `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	AccessLists      []string  `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`
}

// ProxyHost terminates TLS for one or more domains and reverse-proxies to an
// upstream, applying an ordered middleware chain and access lists.
type ProxyHost struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Domains []string `json:"domains" yaml:"domains"`

	// Exactly one of Upstream / UpstreamGroupRef is set. Upstream forwards to a
	// single backend; UpstreamGroupRef names an UpstreamGroup whose ordered
	// upstreams are tried with health-checked failover.
	Upstream         Upstream `json:"upstream,omitempty" yaml:"upstream,omitempty"`
	UpstreamGroupRef string   `json:"upstreamGroupRef,omitempty" yaml:"upstreamGroupRef,omitempty"`

	WebsocketsUpgrade bool `json:"websocketsUpgrade,omitempty" yaml:"websocketsUpgrade,omitempty"`

	// RobotsNoIndex emits an "X-Robots-Tag: noindex, nofollow" response header so
	// search engines do not index this host. A headers middleware that sets
	// X-Robots-Tag explicitly still wins (it is applied closer to the response).
	RobotsNoIndex bool `json:"robotsNoIndex,omitempty" yaml:"robotsNoIndex,omitempty"`

	// Timeouts optionally overrides upstream dial/response timeouts for this host;
	// nil keeps the shared, pooled transport used by every host (the default).
	Timeouts *HostTimeouts `json:"timeouts,omitempty" yaml:"timeouts,omitempty"`

	TLS TLSSettings `json:"tls,omitempty" yaml:"tls,omitempty"`

	// Middlewares/AccessLists apply host-wide (top-down), before any Location-scoped ones.
	Middlewares []string   `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	AccessLists []string   `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`
	Locations   []Location `json:"locations,omitempty" yaml:"locations,omitempty"`
}

func (h ProxyHost) Kind() string { return "ProxyHost" }

func (h ProxyHost) Validate() error {
	if err := ValidateName(h.Name); err != nil {
		return err
	}
	if len(h.Domains) == 0 {
		return fmt.Errorf("proxy host %q: at least one domain is required", h.Name)
	}
	if h.UpstreamGroupRef == "" {
		if err := h.Upstream.validate(); err != nil {
			return fmt.Errorf("proxy host %q: %w", h.Name, err)
		}
	} else if h.Upstream != (Upstream{}) {
		return fmt.Errorf("proxy host %q: upstream and upstreamGroupRef are mutually exclusive", h.Name)
	}
	if err := h.TLS.validate(); err != nil {
		return fmt.Errorf("proxy host %q: %w", h.Name, err)
	}
	if err := h.Timeouts.validate(); err != nil {
		return fmt.Errorf("proxy host %q: %w", h.Name, err)
	}
	for _, l := range h.Locations {
		if l.Path == "" {
			return fmt.Errorf("proxy host %q: location with empty path", h.Name)
		}
		if l.Upstream != nil {
			if l.UpstreamGroupRef != "" {
				return fmt.Errorf("proxy host %q location %q: upstream and upstreamGroupRef are mutually exclusive", h.Name, l.Path)
			}
			if err := l.Upstream.validate(); err != nil {
				return fmt.Errorf("proxy host %q location %q: %w", h.Name, l.Path, err)
			}
		}
	}
	return nil
}

// RedirectHost issues HTTP redirects for its domains.
type RedirectHost struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Domains      []string    `json:"domains" yaml:"domains"`
	TargetScheme string      `json:"targetScheme,omitempty" yaml:"targetScheme,omitempty"` // http|https|auto
	TargetDomain string      `json:"targetDomain" yaml:"targetDomain"`
	StatusCode   int         `json:"statusCode,omitempty" yaml:"statusCode,omitempty"` // 301|302|307|308
	PreservePath bool        `json:"preservePath,omitempty" yaml:"preservePath,omitempty"`
	TLS          TLSSettings `json:"tls,omitempty" yaml:"tls,omitempty"`
}

func (h RedirectHost) Kind() string { return "RedirectHost" }

func (h RedirectHost) Validate() error {
	if err := ValidateName(h.Name); err != nil {
		return err
	}
	if len(h.Domains) == 0 {
		return fmt.Errorf("redirect host %q: at least one domain is required", h.Name)
	}
	if h.TargetDomain == "" {
		return fmt.Errorf("redirect host %q: targetDomain is required", h.Name)
	}
	switch h.StatusCode {
	case 0, 301, 302, 307, 308:
	default:
		return fmt.Errorf("redirect host %q: invalid statusCode %d", h.Name, h.StatusCode)
	}
	return nil
}

// StreamHost forwards raw TCP/UDP from a listen port to a backend.
type StreamHost struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	ListenPort  int    `json:"listenPort" yaml:"listenPort"`
	Protocol    string `json:"protocol" yaml:"protocol"` // tcp | udp | both
	ForwardHost string `json:"forwardHost" yaml:"forwardHost"`
	ForwardPort int    `json:"forwardPort" yaml:"forwardPort"`
}

func (h StreamHost) Kind() string { return "StreamHost" }

func (h StreamHost) Validate() error {
	if err := ValidateName(h.Name); err != nil {
		return err
	}
	switch h.Protocol {
	case "tcp", "udp", "both":
	default:
		return fmt.Errorf("stream host %q: protocol must be tcp|udp|both, got %q", h.Name, h.Protocol)
	}
	if h.ListenPort < 1 || h.ListenPort > 65535 {
		return fmt.Errorf("stream host %q: listenPort %d out of range", h.Name, h.ListenPort)
	}
	if h.ForwardHost == "" {
		return fmt.Errorf("stream host %q: forwardHost is required", h.Name)
	}
	if h.ForwardPort < 1 || h.ForwardPort > 65535 {
		return fmt.Errorf("stream host %q: forwardPort %d out of range", h.Name, h.ForwardPort)
	}
	return nil
}

// DeadHost serves a 404 (or custom status) for claimed domains, useful to
// absorb unmatched vhosts and stop default-host leakage.
type DeadHost struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Domains    []string    `json:"domains" yaml:"domains"`
	StatusCode int         `json:"statusCode,omitempty" yaml:"statusCode,omitempty"` // default 404
	TLS        TLSSettings `json:"tls,omitempty" yaml:"tls,omitempty"`
}

func (h DeadHost) Kind() string { return "DeadHost" }

func (h DeadHost) Validate() error {
	if err := ValidateName(h.Name); err != nil {
		return err
	}
	if len(h.Domains) == 0 {
		return fmt.Errorf("dead host %q: at least one domain is required", h.Name)
	}
	return nil
}
