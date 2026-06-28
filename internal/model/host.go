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
}

func (t TLSSettings) validate() error {
	switch t.MinTLSVersion {
	case "", "1.2", "1.3":
		return nil
	default:
		return fmt.Errorf(`tls.minTLSVersion must be "1.2" or "1.3", got %q`, t.MinTLSVersion)
	}
}

// Location is a path-scoped override within a proxy host. Locations carry their
// own upstream and their own ordered middleware/access-list references, so
// per-location auth and access control are first-class config, not text snippets.
type Location struct {
	Path        string    `json:"path" yaml:"path"`
	Upstream    *Upstream `json:"upstream,omitempty" yaml:"upstream,omitempty"`
	Middlewares []string  `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	AccessLists []string  `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`
}

// ProxyHost terminates TLS for one or more domains and reverse-proxies to an
// upstream, applying an ordered middleware chain and access lists.
type ProxyHost struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Domains  []string `json:"domains" yaml:"domains"`
	Upstream Upstream `json:"upstream" yaml:"upstream"`

	WebsocketsUpgrade bool `json:"websocketsUpgrade,omitempty" yaml:"websocketsUpgrade,omitempty"`

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
	if err := h.Upstream.validate(); err != nil {
		return fmt.Errorf("proxy host %q: %w", h.Name, err)
	}
	if err := h.TLS.validate(); err != nil {
		return fmt.Errorf("proxy host %q: %w", h.Name, err)
	}
	for _, l := range h.Locations {
		if l.Path == "" {
			return fmt.Errorf("proxy host %q: location with empty path", h.Name)
		}
		if l.Upstream != nil {
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
