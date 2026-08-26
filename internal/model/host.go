package model

import (
	"fmt"
	"regexp"
	"strings"
)

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
	// IdentityHeaders opts this host into forwarding the VERIFIED client
	// certificate's identity to the upstream as gpm-asserted request headers.
	// nil (default) forwards nothing. These headers ride the existing identity
	// model: they are in the baseline denylist, so a direct client can never
	// assert them, and only gpm sets them - after the strip - from a certificate
	// the handshake actually verified.
	IdentityHeaders *ClientCertHeaders `json:"identityHeaders,omitempty" yaml:"identityHeaders,omitempty"`
}

// Default header names for client-certificate identity passthrough. They are
// part of the data plane's baseline identity denylist, so no direct client can
// forge them regardless of whether a host enables passthrough.
const (
	DefaultClientCertSubjectHeader = "X-Client-Cert-Subject"
	ClientCertSANHeader            = "X-Client-Cert-SAN"
	ClientCertSerialHeader         = "X-Client-Cert-Serial"
	ClientCertFingerprintHeader    = "X-Client-Cert-Fingerprint"
)

// ClientCertHeaders selects which verified-client-certificate attributes are
// forwarded upstream. The subject is always sent (under SubjectHeader); the
// other three are opt-in and use fixed header names so the denylist covers them.
type ClientCertHeaders struct {
	// SubjectHeader overrides the header carrying the certificate subject (RFC
	// 2253 form). Empty selects DefaultClientCertSubjectHeader. A custom name is
	// added to this host's strip set, so it is still refused from an untrusted peer.
	SubjectHeader string `json:"subjectHeader,omitempty" yaml:"subjectHeader,omitempty"`
	// SAN sends the certificate's subject alternative names (DNS, email, IP,
	// URI), comma-separated, as X-Client-Cert-SAN.
	SAN bool `json:"san,omitempty" yaml:"san,omitempty"`
	// Serial sends the certificate serial number in lower-case hex as
	// X-Client-Cert-Serial.
	Serial bool `json:"serial,omitempty" yaml:"serial,omitempty"`
	// Fingerprint sends the SHA-256 fingerprint of the DER certificate in
	// lower-case hex as X-Client-Cert-Fingerprint.
	Fingerprint bool `json:"fingerprint,omitempty" yaml:"fingerprint,omitempty"`
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
		if ih := t.ClientAuth.IdentityHeaders; ih != nil && ih.SubjectHeader != "" {
			if !validHeaderName(ih.SubjectHeader) {
				return fmt.Errorf("tls.clientAuth.identityHeaders.subjectHeader %q is not a valid header name", ih.SubjectHeader)
			}
			if strings.HasPrefix(strings.ToLower(ih.SubjectHeader), "x-forwarded-") {
				return fmt.Errorf("tls.clientAuth.identityHeaders.subjectHeader must not be an X-Forwarded-* header (gpm sets those itself), got %q", ih.SubjectHeader)
			}
		}
	}
	return nil
}

// validHeaderName reports whether s is a non-empty RFC 7230 field-name token,
// so a configured passthrough header can never inject CR/LF or a separator into
// the upstream request.
func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r > 0x7e || r <= 0x20 {
			return false
		}
		if strings.ContainsRune(`"(),/:;<=>?@[\]{}`, r) {
			return false
		}
	}
	return true
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

// DefaultCompressionMinBytes is the smallest response body gzip bothers with
// when Compression.MinBytes is unset (0). Smaller bodies are sent uncompressed:
// gzip's own framing overhead can make them larger, not smaller.
const DefaultCompressionMinBytes = 1024

// DefaultCompressionTypes is applied when Compression.Types is empty: a
// conventional text/JS/CSS/SVG/XML/JSON allowlist. Binary and already-compressed
// formats (images, video, fonts, archives) are deliberately excluded - gzipping
// them wastes CPU for no size win.
var DefaultCompressionTypes = []string{
	"text/html", "text/plain", "text/css", "text/csv",
	"application/json", "application/javascript", "text/javascript",
	"application/xml", "text/xml", "image/svg+xml",
}

// Compression optionally gzip-compresses eligible response bodies from this
// host's upstream before they reach the client, honouring the client's
// Accept-Encoding. Off by default (Enabled: false), so an unconfigured host
// behaves exactly as before this feature shipped. See dataplane's compression
// handler for the exclusions (upstream already encoded, non-matching type,
// below MinBytes, websocket/streaming/event-stream responses, 204/304/HEAD).
type Compression struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	// MinBytes is the smallest response body gzip bothers with. 0 (default)
	// selects DefaultCompressionMinBytes.
	MinBytes int `json:"minBytes,omitempty" yaml:"minBytes,omitempty"`
	// Types lists the response Content-Types (the media type only; parameters
	// like charset are ignored) eligible for compression. Empty (default)
	// selects DefaultCompressionTypes.
	Types []string `json:"types,omitempty" yaml:"types,omitempty"`
}

// EffectiveMinBytes returns MinBytes, or DefaultCompressionMinBytes when unset.
func (c Compression) EffectiveMinBytes() int {
	if c.MinBytes > 0 {
		return c.MinBytes
	}
	return DefaultCompressionMinBytes
}

// EffectiveTypes returns Types, or DefaultCompressionTypes when unset.
func (c Compression) EffectiveTypes() []string {
	if len(c.Types) > 0 {
		return c.Types
	}
	return DefaultCompressionTypes
}

func (c Compression) validate() error {
	if !c.Enabled {
		return nil
	}
	if c.MinBytes < 0 {
		return fmt.Errorf("compression.minBytes must be >= 0, got %d", c.MinBytes)
	}
	for _, t := range c.Types {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("compression.types entries must not be empty")
		}
	}
	return nil
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

	// DNS opts this host into automatic DNS record management for its domains
	// (see settings.dnsSync). A nil policy publishes nothing. It is a POINTER
	// because encoding/json ignores omitempty on a struct value: as a plain
	// struct every proxy-host response carried a noise `"dns":{}`.
	DNS *DNSSyncPolicy `json:"dns,omitempty" yaml:"dns,omitempty"`

	// Middlewares/AccessLists apply host-wide (top-down), before any Location-scoped ones.
	Middlewares []string   `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	AccessLists []string   `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`
	Locations   []Location `json:"locations,omitempty" yaml:"locations,omitempty"`

	// Compression opts this host into gzip response compression. The zero value
	// (Enabled: false) is today's behaviour: no compression.
	Compression Compression `json:"compression,omitempty" yaml:"compression,omitempty"`

	// ErrorPages overrides settings.errorPages for this host's own gpm-generated
	// error responses (upstream unreachable, access denied, rate-limited, a
	// dangling middleware/access-list reference). nil (default) uses the
	// settings-level pages, if any. POINTER for the same reason as DNS above: a
	// struct value's omitempty is ignored by encoding/json.
	ErrorPages *ErrorPagesConfig `json:"errorPages,omitempty" yaml:"errorPages,omitempty"`

	// SecurityHeaders overrides/merges over settings.securityHeaders for this
	// host. It is a per-key merge: a header this map names replaces the settings
	// default value, and a header it omits still falls through to the settings
	// default (matching how errorPages templates resolve). Same scope and
	// set-if-absent-on-proxied rules as the settings-level default. nil (default)
	// uses the settings-level headers unchanged.
	SecurityHeaders map[string]string `json:"securityHeaders,omitempty" yaml:"securityHeaders,omitempty"`
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
	if err := h.Compression.validate(); err != nil {
		return fmt.Errorf("proxy host %q: %w", h.Name, err)
	}
	if h.ErrorPages != nil {
		if err := h.ErrorPages.validate(); err != nil {
			return fmt.Errorf("proxy host %q: %w", h.Name, err)
		}
	}
	if err := validateSecurityHeaders(h.SecurityHeaders); err != nil {
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

// Stream TLS modes (StreamTLS.Mode).
const (
	// StreamTLSPassthrough peeks the ClientHello for SNI and forwards the
	// connection byte-for-byte, encrypted end to end. gpm never sees plaintext
	// and needs no certificate.
	StreamTLSPassthrough = "passthrough"
	// StreamTLSTerminate completes the TLS handshake at gpm using the referenced
	// certificate and forwards plaintext to the backend.
	StreamTLSTerminate = "terminate"
)

// StreamTLS adds TLS awareness to a TCP stream host: SNI-based routing on a
// shared listen port, with the handshake either forwarded untouched
// (passthrough) or terminated at gpm (terminate).
//
// It is TCP-only. UDP carries no TLS record stream to inspect, so a UDP (or
// "both") stream host cannot express it - see StreamHost.Validate.
type StreamTLS struct {
	// Mode is passthrough or terminate. Required when tls is set.
	Mode string `json:"mode" yaml:"mode"`
	// SNIMatch lists the server names this host claims on its listen port.
	// Entries may be exact ("db.example.com") or a single-label wildcard
	// ("*.example.com"). It is REQUIRED for every host that shares its listen
	// port with another stream host - that is the only thing that makes two
	// hosts on one port separable. A host alone on its port may leave it empty
	// and take every connection.
	SNIMatch []string `json:"sniMatch,omitempty" yaml:"sniMatch,omitempty"`
	// CertificateRef names the Certificate used to terminate. Required in
	// terminate mode, and forbidden in passthrough (which never decrypts).
	CertificateRef string `json:"certificateRef,omitempty" yaml:"certificateRef,omitempty"`
}

func (t *StreamTLS) validate(hostName, protocol string) error {
	if t == nil {
		return nil
	}
	if protocol != "tcp" {
		return fmt.Errorf("stream host %q: tls requires protocol tcp (TLS/SNI has no meaning for udp), got %q", hostName, protocol)
	}
	switch t.Mode {
	case StreamTLSPassthrough:
		if t.CertificateRef != "" {
			return fmt.Errorf("stream host %q: tls.certificateRef is not allowed in passthrough mode (the connection is never decrypted)", hostName)
		}
	case StreamTLSTerminate:
		if t.CertificateRef == "" {
			return fmt.Errorf("stream host %q: tls.certificateRef is required in terminate mode", hostName)
		}
	default:
		return fmt.Errorf("stream host %q: tls.mode must be %s|%s, got %q", hostName, StreamTLSPassthrough, StreamTLSTerminate, t.Mode)
	}
	seen := map[string]bool{}
	for _, n := range t.SNIMatch {
		name := strings.ToLower(strings.TrimSpace(n))
		if name == "" {
			return fmt.Errorf("stream host %q: tls.sniMatch contains an empty server name", hostName)
		}
		if !validSNIMatch(name) {
			return fmt.Errorf("stream host %q: invalid tls.sniMatch %q (want a hostname or a *.suffix wildcard)", hostName, n)
		}
		if seen[name] {
			return fmt.Errorf("stream host %q: duplicate tls.sniMatch %q", hostName, name)
		}
		seen[name] = true
	}
	return nil
}

var sniLabelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// validSNIMatch reports whether s is an acceptable SNI match: a dotted hostname,
// optionally prefixed with "*." for a single-level wildcard.
func validSNIMatch(s string) bool {
	s = strings.TrimPrefix(s, "*.")
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !sniLabelRe.MatchString(label) {
			return false
		}
	}
	return true
}

// StreamTarget is the backend a stream host forwards to. It mirrors Upstream's
// host/port vocabulary deliberately, minus the scheme: a raw TCP/UDP stream
// carries an arbitrary protocol, so an http/https scheme has no meaning for it.
type StreamTarget struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
}

func (t StreamTarget) validate(hostName string) error {
	if t.Host == "" {
		return fmt.Errorf("stream host %q: target.host is required", hostName)
	}
	if t.Port < 1 || t.Port > 65535 {
		return fmt.Errorf("stream host %q: target.port %d out of range", hostName, t.Port)
	}
	return nil
}

// StreamHost forwards raw TCP/UDP from a listen port to a backend.
type StreamHost struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	ListenPort int    `json:"listenPort" yaml:"listenPort"`
	Protocol   string `json:"protocol" yaml:"protocol"` // tcp | udp | both

	// Target is the backend this port forwards to.
	Target StreamTarget `json:"target" yaml:"target"`

	// LegacyForwardHost/LegacyForwardPort exist ONLY so a config still written in
	// the pre-target shape fails loudly instead of silently losing its backend.
	// Neither encoding/json nor yaml.v3 errors on an unknown key here, so the two
	// retired keys are decoded into these fields and rejected by Validate. They
	// are omitempty, so nothing gpm writes ever carries them.
	LegacyForwardHost string `json:"forwardHost,omitempty" yaml:"forwardHost,omitempty"`
	LegacyForwardPort int    `json:"forwardPort,omitempty" yaml:"forwardPort,omitempty"`

	// TLS opts a TCP stream host into SNI routing and/or TLS termination. nil
	// (the default) keeps the historical blind byte forwarder.
	TLS *StreamTLS `json:"tls,omitempty" yaml:"tls,omitempty"`

	// AccessLists gate the connection at L4 on the client IP, evaluated before
	// any backend is dialled. Only the IP/CIDR and geo dimensions of a list
	// apply - basic auth is an HTTP challenge/response and has nowhere to live
	// in a raw stream, so a list carrying basicAuth is rejected at validation
	// rather than silently ignored.
	AccessLists []string `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`
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
	if h.LegacyForwardHost != "" || h.LegacyForwardPort != 0 {
		return fmt.Errorf("stream host %q: forwardHost/forwardPort were replaced by a single target; write `target: {host: <host>, port: <port>}` instead", h.Name)
	}
	if err := h.Target.validate(h.Name); err != nil {
		return err
	}
	if err := h.TLS.validate(h.Name, h.Protocol); err != nil {
		return err
	}
	return nil
}

// SNINames returns this stream host's lower-cased SNI matches, or nil when it
// does not route by SNI.
func (h StreamHost) SNINames() []string {
	if h.TLS == nil || len(h.TLS.SNIMatch) == 0 {
		return nil
	}
	out := make([]string, 0, len(h.TLS.SNIMatch))
	for _, n := range h.TLS.SNIMatch {
		out = append(out, strings.ToLower(strings.TrimSpace(n)))
	}
	return out
}

// ParkedHost serves a 404 (or custom status) for claimed domains: it reserves a
// name without serving anything, absorbing unmatched vhosts so nothing leaks to
// a default host.
type ParkedHost struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Domains    []string    `json:"domains" yaml:"domains"`
	StatusCode int         `json:"statusCode,omitempty" yaml:"statusCode,omitempty"` // default 404
	TLS        TLSSettings `json:"tls,omitempty" yaml:"tls,omitempty"`
}

func (h ParkedHost) Kind() string { return "ParkedHost" }

func (h ParkedHost) Validate() error {
	if err := ValidateName(h.Name); err != nil {
		return err
	}
	if len(h.Domains) == 0 {
		return fmt.Errorf("parked host %q: at least one domain is required", h.Name)
	}
	return nil
}
