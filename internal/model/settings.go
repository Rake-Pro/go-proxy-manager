package model

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// AdminAuthSettings governs how operators authenticate to the admin panel.
type AdminAuthSettings struct {
	// Providers names IdentityProvider objects allowed to log into the panel.
	Providers []string `json:"providers,omitempty" yaml:"providers,omitempty"`
	// LocalLoginEnabled keeps username/password login available (anti-lockout).
	LocalLoginEnabled bool `json:"localLoginEnabled" yaml:"localLoginEnabled"`
	// SSOOnly enforces SSO and disables local login entirely. Recovery from an
	// SSO outage is by redeploying with local login re-enabled (no in-band
	// break-glass: a network-position-trusted local door is a spoofing risk).
	SSOOnly bool `json:"ssoOnly,omitempty" yaml:"ssoOnly,omitempty"`
}

// WebhookConfig is a single outbound lifecycle webhook: gpm POSTs a small JSON
// event to URL after every successful config change (create/update/delete,
// restore, revert, settings). Dispatch is asynchronous and best-effort, so a slow
// or unreachable endpoint never blocks or fails the config write.
type WebhookConfig struct {
	// Name is a stable identifier for the target (shown in logs).
	Name string `json:"name" yaml:"name"`
	// URL is the absolute http(s) endpoint that receives the POST.
	URL string `json:"url" yaml:"url"`
	// Secret, if set, is sent as the X-GPM-Webhook-Secret header so the receiver
	// can authenticate the call. Stored as a placeholder, resolved at dispatch.
	Secret Secret `json:"secret,omitempty" yaml:"secret,omitempty"`
	// Disabled keeps the target in config without firing it.
	Disabled bool `json:"disabled,omitempty" yaml:"disabled,omitempty"`
}

// DefaultProxyProtocolTimeout bounds how long a trusted peer has to deliver a
// complete PROXY protocol header before the connection is closed. It is short on
// purpose: the header is the very first thing a conforming sender writes, so a
// stall is a broken or hostile peer, not a slow one.
const DefaultProxyProtocolTimeout = 5 * time.Second

// maxProxyProtocolTimeout caps the configurable header deadline. A large value
// would let a trusted-CIDR peer pin a connection (and its file descriptor) for
// that long by opening a socket and never writing the header.
const maxProxyProtocolTimeout = time.Minute

// ProxyProtocolSettings enables inbound HAProxy PROXY protocol (v1 and v2) on
// the data-plane HTTP/HTTPS listeners and on every TCP stream listener, so gpm
// behind an L4 load balancer sees the real client address instead of the
// balancer's.
//
// The header is honoured ONLY from a peer inside TrustedCIDRs. From anyone else
// the bytes are treated as ordinary payload and the connection peer stays the
// client IP - a PROXY header is an unauthenticated claim about who the client
// is, so accepting one from an arbitrary peer would let any client assert any
// source address and walk straight through IP access lists, geo rules, rate
// limits and the basic-auth lockout.
type ProxyProtocolSettings struct {
	// Enabled turns header parsing on. Off (the default) leaves every listener
	// byte-for-byte as it was.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	// TrustedCIDRs are the peers whose PROXY header is believed: the addresses of
	// your load balancers, as CIDRs or bare IPs. Required when Enabled - there is
	// no "trust everyone" mode.
	TrustedCIDRs []string `json:"trustedCIDRs,omitempty" yaml:"trustedCIDRs,omitempty"`
	// Timeout is the deadline for reading a complete header from a trusted peer,
	// as a Go duration ("5s"). Empty selects DefaultProxyProtocolTimeout.
	Timeout string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// HeaderTimeout returns the configured header deadline, or the default when
// unset or unparseable (Validate rejects an unparseable value at write time, so
// this fallback only covers a config that predates validation).
func (p *ProxyProtocolSettings) HeaderTimeout() time.Duration {
	if p == nil || p.Timeout == "" {
		return DefaultProxyProtocolTimeout
	}
	d, err := time.ParseDuration(p.Timeout)
	if err != nil || d <= 0 {
		return DefaultProxyProtocolTimeout
	}
	return d
}

func (p *ProxyProtocolSettings) validate() error {
	if p == nil || !p.Enabled {
		return nil
	}
	if len(p.TrustedCIDRs) == 0 {
		return fmt.Errorf("settings: proxyProtocol.trustedCIDRs is required when proxyProtocol.enabled is true (a PROXY header from an untrusted peer would let any client spoof its source IP)")
	}
	for _, c := range p.TrustedCIDRs {
		if _, _, err := net.ParseCIDR(c); err != nil {
			if net.ParseIP(c) == nil {
				return fmt.Errorf("settings: proxyProtocol.trustedCIDRs: invalid cidr/ip %q", c)
			}
		}
	}
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return fmt.Errorf("settings: proxyProtocol.timeout %q is not a duration (e.g. 5s)", p.Timeout)
		}
		if d <= 0 || d > maxProxyProtocolTimeout {
			return fmt.Errorf("settings: proxyProtocol.timeout %s out of range (0 < timeout <= %s)", d, maxProxyProtocolTimeout)
		}
	}
	return nil
}

// ErrorPagesConfig configures custom HTML pages served for errors gpm ITSELF
// generates - upstream unreachable (502/504), no healthy upstream, access
// denied (access-list/guard/geo), rate-limited (429), a parked host, or a host
// whose middleware/access-list reference cannot be resolved (503). The
// upstream's own error response is left untouched unless its status is also
// listed in InterceptUpstream. See dataplane's error-page renderer.
type ErrorPagesConfig struct {
	// Dir is a directory of html/template files named "<status>.html" (e.g.
	// "502.html") plus an optional "default.html" fallback, relative to the
	// managed cert store - confined exactly like a custom certificate's files
	// (no absolute path, no ".."), so config can never point the loader at an
	// arbitrary host file.
	Dir string `json:"dir,omitempty" yaml:"dir,omitempty"`
	// Inline maps a status code (as a decimal string, e.g. "502") - or the
	// literal "default" for the fallback - directly to html/template source,
	// for a handful of pages an operator would rather keep in config than mount
	// a directory.
	Inline map[string]string `json:"inline,omitempty" yaml:"inline,omitempty"`
	// InterceptUpstream lists status codes for which the UPSTREAM's own error
	// response body is also replaced by the configured page (by default only
	// errors gpm itself generates are replaced, never the upstream's own).
	InterceptUpstream []int `json:"interceptUpstream,omitempty" yaml:"interceptUpstream,omitempty"`
}

// DefaultMaintenanceRetryAfter is the Retry-After (seconds) sent with every
// maintenance 503 when MaintenanceSettings.RetryAfterSeconds is unset: five
// minutes. It is short enough that a client re-checks within a typical
// maintenance window and long enough that a fleet of retrying clients does not
// hammer an edge that is deliberately down.
const DefaultMaintenanceRetryAfter = 300

// maxMaintenanceRetryAfter caps the configurable Retry-After at 24h. A larger
// value is indistinguishable from "gone" to a crawler and would keep clients
// away long after the window closed.
const maxMaintenanceRetryAfter = 86400

// MaintenanceSettings is the fleet-wide downtime switch. It is the global half
// of maintenance mode; the per-host half is ProxyHost.Maintenance.
//
// While a host is in maintenance gpm answers every request to it itself, with a
// 503 and a Retry-After, and never dials the upstream. The page served is the
// configured errorPages template for 503 (host override first, then the
// settings-level pages, exactly like every other gpm-generated error), or gpm's
// built-in maintenance body when neither configures one.
//
// Only proxy hosts are affected. Redirect, parked and stream hosts proxy
// nothing to take out of service, so they keep serving.
type MaintenanceSettings struct {
	// Enabled puts EVERY proxy host into maintenance, whatever its own
	// maintenance flag says: the global toggle wins over a per-host false, so an
	// operator can take the whole edge down (and bring it back) with one write.
	// Off (the default) leaves each host to its own flag.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	// RetryAfterSeconds is the Retry-After header value on every maintenance
	// response. 0 (the default) selects DefaultMaintenanceRetryAfter.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty" yaml:"retryAfterSeconds,omitempty"`
}

// EffectiveRetryAfter returns RetryAfterSeconds, or DefaultMaintenanceRetryAfter
// when unset.
func (m MaintenanceSettings) EffectiveRetryAfter() int {
	if m.RetryAfterSeconds > 0 {
		return m.RetryAfterSeconds
	}
	return DefaultMaintenanceRetryAfter
}

func (m MaintenanceSettings) validate() error {
	if m.RetryAfterSeconds < 0 || m.RetryAfterSeconds > maxMaintenanceRetryAfter {
		return fmt.Errorf("maintenance.retryAfterSeconds %d out of range (0-%d)", m.RetryAfterSeconds, maxMaintenanceRetryAfter)
	}
	return nil
}

// SecurityScope selects which responses a configured security header applies to.
// The empty scope is normalised to SecurityScopeAll (today's behaviour), so an
// old plain-string-map config keeps working unchanged.
type SecurityScope string

const (
	// SecurityScopeAll applies the header to both gpm-generated and proxied
	// upstream responses (set-if-absent on each). This is the default.
	SecurityScopeAll SecurityScope = "all"
	// SecurityScopeGenerated applies the header ONLY to responses gpm itself
	// writes (auth-gate refusals, sign-in redirects, error pages, the
	// path-rejection 400, the no-such-host 404, parked/redirect hosts) and NEVER
	// to a proxied upstream response. This is the safe placement for headers like
	// Content-Security-Policy frame-ancestors and Permissions-Policy, which can
	// break a proxied app that ships none of its own.
	SecurityScopeGenerated SecurityScope = "generated-only"
	// SecurityScopeProxied applies the header ONLY to proxied upstream responses
	// (still set-if-absent) and never to a gpm-generated response - for headers
	// meaningful only alongside real app content.
	SecurityScopeProxied SecurityScope = "proxied-only"
)

// SecurityHeaderValue is one configured response header: its value plus the
// scope selecting which responses it lands on. It is the map value of both
// settings.securityHeaders and proxyHost.securityHeaders.
//
// Backward compatibility: the config/API historically stored securityHeaders as
// a plain map[string]string (name -> value, no scope). Both wire formats still
// accept that form - a bare string unmarshals to this struct with an empty scope
// (which the data plane treats as SecurityScopeAll), or an object
// {value, scope} carries an explicit scope. The marshallers invert it: an
// all-scope (or empty-scope) header marshals back to a bare string, so an
// unchanged config round-trips byte-for-byte and existing API consumers keep
// seeing plain strings; only a header carrying a non-default scope marshals as
// the {value, scope} object.
type SecurityHeaderValue struct {
	Value string        `json:"value" yaml:"value"`
	Scope SecurityScope `json:"scope,omitempty" yaml:"scope,omitempty"`
}

// securityHeaderObject is the object wire form, used by the (un)marshallers so
// the struct's own methods can accept/emit either shape without recursing.
type securityHeaderObject struct {
	Value string        `json:"value" yaml:"value"`
	Scope SecurityScope `json:"scope,omitempty" yaml:"scope,omitempty"`
}

// UnmarshalJSON accepts either the legacy bare string ("name": "value") or the
// scoped object ("name": {"value": ..., "scope": ...}).
func (v *SecurityHeaderValue) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		v.Value, v.Scope = s, ""
		return nil
	}
	var o securityHeaderObject
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	v.Value, v.Scope = o.Value, o.Scope
	return nil
}

// MarshalJSON emits a bare string for an all/empty-scope header (so old-form
// config and API responses are unchanged) and the {value, scope} object only
// when a non-default scope is set.
func (v SecurityHeaderValue) MarshalJSON() ([]byte, error) {
	if v.Scope == "" || v.Scope == SecurityScopeAll {
		return json.Marshal(v.Value)
	}
	return json.Marshal(securityHeaderObject(v))
}

// UnmarshalYAML mirrors UnmarshalJSON: a scalar node is the legacy bare value; a
// mapping node is the scoped object.
func (v *SecurityHeaderValue) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		v.Value, v.Scope = n.Value, ""
		return nil
	}
	var o securityHeaderObject
	if err := n.Decode(&o); err != nil {
		return err
	}
	v.Value, v.Scope = o.Value, o.Scope
	return nil
}

// MarshalYAML mirrors MarshalJSON.
func (v SecurityHeaderValue) MarshalYAML() (interface{}, error) {
	if v.Scope == "" || v.Scope == SecurityScopeAll {
		return v.Value, nil
	}
	return securityHeaderObject(v), nil
}

// RecommendedSecurityHeaders is a safe, paste-ready default set operators can
// copy into settings.securityHeaders. It is documentation only: gpm ships
// NOTHING by default (an empty map is today's behaviour), so an existing
// deployment is never surprised by a new header. Content-Security-Policy
// (frame-ancestors) and Permissions-Policy are placed at scope generated-only:
// safe on gpm's own pages but liable to break a proxied app that ships none of
// its own (see docs/configuration.md). The rest are scope all.
var RecommendedSecurityHeaders = map[string]SecurityHeaderValue{
	"X-Content-Type-Options":  {Value: "nosniff", Scope: SecurityScopeAll},
	"Referrer-Policy":         {Value: "strict-origin-when-cross-origin", Scope: SecurityScopeAll},
	"X-Frame-Options":         {Value: "DENY", Scope: SecurityScopeAll},
	"Permissions-Policy":      {Value: "geolocation=(), camera=(), microphone=()", Scope: SecurityScopeGenerated},
	"Content-Security-Policy": {Value: "frame-ancestors 'none'", Scope: SecurityScopeGenerated},
}

// hopByHopHeaders are the RFC 7230 connection-scoped headers. They are
// meaningless (and dangerous) as a configured, per-response header, so
// securityHeaders rejects them.
var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

// validHeaderValue reports whether v is a valid HTTP field value: no CR, LF or
// NUL and no other control byte except horizontal tab. This is what blocks a
// configured value from injecting an extra response header (CRLF injection).
func validHeaderValue(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\t' {
			continue
		}
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// validateSecurityHeaders validates a settings- or host-level securityHeaders
// map: every key must be a valid RFC 7230 field-name token, keys are
// de-duplicated case-insensitively (so a header is declared exactly once, at one
// scope - it cannot appear at two different scopes), Strict-Transport-Security
// is refused (the per-host hsts setting owns it), hop-by-hop headers are
// refused, every value must be free of CR/LF/control bytes, and every scope must
// be one of the three known values (the empty scope is allowed and means "all").
func validateSecurityHeaders(m map[string]SecurityHeaderValue) error {
	seen := make(map[string]struct{}, len(m))
	for k, v := range m {
		if !validHeaderName(k) {
			return fmt.Errorf("securityHeaders: %q is not a valid header name", k)
		}
		lk := strings.ToLower(k)
		if lk == "strict-transport-security" {
			return fmt.Errorf("securityHeaders: Strict-Transport-Security is managed by the per-host hsts setting, not securityHeaders")
		}
		if hopByHopHeaders[lk] {
			return fmt.Errorf("securityHeaders: %q is a hop-by-hop header and cannot be set as a response header", k)
		}
		if _, dup := seen[lk]; dup {
			return fmt.Errorf("securityHeaders: duplicate header %q (names are case-insensitive; a header is declared once, at one scope)", k)
		}
		seen[lk] = struct{}{}
		if !validHeaderValue(v.Value) {
			return fmt.Errorf("securityHeaders[%q]: value contains an invalid character (CR/LF/control)", k)
		}
		switch v.Scope {
		case "", SecurityScopeAll, SecurityScopeGenerated, SecurityScopeProxied:
		default:
			return fmt.Errorf("securityHeaders[%q]: unknown scope %q (want %q, %q, or %q)", k, v.Scope, SecurityScopeAll, SecurityScopeGenerated, SecurityScopeProxied)
		}
	}
	return nil
}

// unstrippableHeaders are response headers that carry the response's own
// semantics rather than a backend detail, so removing one corrupts the response
// instead of hiding an implementation. They are refused alongside the hop-by-hop
// set.
//
// Content-Type is the sharpest of them: net/http falls back to
// DetectContentType when a handler sets no Content-Type, so stripping it hands
// the body to content sniffing - a JSON or text response whose bytes happen to
// look like markup would be re-labelled text/html, turning a config typo into
// stored XSS. Content-Length and Content-Encoding are framing and body encoding
// (dropping Content-Encoding leaves a gzip body labelled as plain), Vary is the
// cache key (dropping it lets a shared cache serve one variant to every client),
// and Location is the entire meaning of a 3xx.
//
// The Sec-Websocket-* trio is here for the same reason, and matters because 101
// Upgrade responses ARE in scope for stripping: Sec-Websocket-Accept is the
// server's proof it understood the handshake, and every browser aborts the
// connection without it, so stripping one would break WebSockets on the host
// entirely. Protocol and Extensions are the negotiated result the client must
// see. (These are the canonical MIME forms - textproto lower-cases the "S" in
// "WebSocket" - but the check is case-insensitive regardless.)
//
// Set-Cookie and WWW-Authenticate are deliberately NOT here. Both are sharp -
// stripping the backend's Set-Cookie breaks that app's own sessions, and
// stripping WWW-Authenticate suppresses its basic-auth challenge - but both are
// legitimate operator choices, and stripping only ever touches what the upstream
// sent (see the data plane's ModifyResponse hook), never a cookie gpm's own
// forward-auth refresh added. They are documented as sharp edges instead of
// refused.
var unstrippableHeaders = map[string]bool{
	"content-encoding":         true,
	"content-length":           true,
	"content-type":             true,
	"location":                 true,
	"vary":                     true,
	"sec-websocket-accept":     true,
	"sec-websocket-protocol":   true,
	"sec-websocket-extensions": true,
}

// validateStripResponseHeaders validates a settings- or host-level
// stripResponseHeaders list: every entry must be a valid RFC 7230 field-name
// token (an empty or malformed name is a config error, not a silent no-op),
// entries are de-duplicated case-insensitively (a header is named once; matching
// is case-insensitive anyway), and hop-by-hop and response-semantic headers are
// refused (see unstrippableHeaders).
//
// There is deliberately NO denylist for the headers gpm itself sets: stripping
// happens on the UPSTREAM's response header map, before those headers are copied
// onto the client response, so nothing gpm adds (securityHeaders, HSTS,
// X-Robots-Tag, a forward-auth Set-Cookie refresh, gzip's Content-Encoding, a
// headers middleware's setResponse) is reachable by it in the first place.
func validateStripResponseHeaders(names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, k := range names {
		if !validHeaderName(k) {
			return fmt.Errorf("stripResponseHeaders: %q is not a valid header name", k)
		}
		lk := strings.ToLower(k)
		if hopByHopHeaders[lk] {
			return fmt.Errorf("stripResponseHeaders: %q is a hop-by-hop header and cannot be stripped (it carries the response's own framing)", k)
		}
		if unstrippableHeaders[lk] {
			return fmt.Errorf("stripResponseHeaders: %q carries the response's own semantics and cannot be stripped", k)
		}
		if _, dup := seen[lk]; dup {
			return fmt.Errorf("stripResponseHeaders: duplicate header %q (names are case-insensitive)", k)
		}
		seen[lk] = struct{}{}
	}
	return nil
}

// StripResponseHeaderRefused reports whether a header name is refused by
// stripResponseHeaders validation (hop-by-hop or response-semantic). It is
// exported for the data plane's defence-in-depth compile step, which drops such
// a name rather than trusting that every config reaching it passed validation.
func StripResponseHeaderRefused(name string) bool {
	lk := strings.ToLower(name)
	return hopByHopHeaders[lk] || unstrippableHeaders[lk]
}

func (e ErrorPagesConfig) validate() error {
	if f := e.Dir; f != "" {
		// Same confinement as a custom certificate's files / a ClientCA's crlFile.
		if filepath.IsAbs(f) || strings.HasPrefix(f, "/") || strings.HasPrefix(f, `\`) || strings.Contains(filepath.Clean(f), "..") {
			return fmt.Errorf("errorPages.dir %q must be relative to the cert store (no absolute or .. paths)", f)
		}
	}
	for k := range e.Inline {
		if k == "default" {
			continue
		}
		n, err := strconv.Atoi(k)
		if err != nil || n < 100 || n > 599 {
			return fmt.Errorf(`errorPages.inline key %q must be a 3-digit status code or "default"`, k)
		}
	}
	for _, s := range e.InterceptUpstream {
		if s < 400 || s > 599 {
			return fmt.Errorf("errorPages.interceptUpstream %d must be a 4xx/5xx status code", s)
		}
	}
	return nil
}

// Settings is the singleton app configuration, stored as config/settings.yaml.
type Settings struct {
	SchemaVersion int `json:"schemaVersion" yaml:"schemaVersion"`

	// AppName is the brand label shown in the admin nav and the login page.
	// Empty falls back to "Go Proxy Manager".
	AppName string `json:"appName,omitempty" yaml:"appName,omitempty"`

	// ExternalBaseURL is the canonical public URL of the admin panel. It is
	// configured explicitly so OIDC redirect_uri is never derived from
	// X-Forwarded-* headers (the port/scheme footgun that broke fork logins).
	ExternalBaseURL string `json:"externalBaseURL" yaml:"externalBaseURL"`

	AdminAuth AdminAuthSettings `json:"adminAuth,omitempty" yaml:"adminAuth,omitempty"`

	// Webhooks are outbound lifecycle notifications fired after every config change.
	Webhooks []WebhookConfig `json:"webhooks,omitempty" yaml:"webhooks,omitempty"`

	// DNSSync configures the optional local/public DNS record reconcilers that
	// publish CNAMEs for proxy hosts opted in via their dns policy.
	DNSSync DNSSyncSettings `json:"dnsSync,omitempty" yaml:"dnsSync,omitempty"`

	// ProxyProtocol enables inbound PROXY protocol on the data-plane listeners
	// (see ProxyProtocolSettings). nil or disabled leaves them untouched.
	ProxyProtocol *ProxyProtocolSettings `json:"proxyProtocol,omitempty" yaml:"proxyProtocol,omitempty"`

	// IngressDiscovery configures the optional read-only Kubernetes Ingress
	// reconciler that derives managed proxy hosts from annotated cluster
	// Ingresses, which then feed DNSSync above.
	IngressDiscovery IngressDiscoverySettings `json:"ingressDiscovery,omitempty" yaml:"ingressDiscovery,omitempty"`

	// Maintenance is the fleet-wide downtime switch (see MaintenanceSettings)
	// and the Retry-After every maintenance response carries. The zero value is
	// off: each proxy host is governed by its own maintenance flag alone.
	Maintenance MaintenanceSettings `json:"maintenance,omitempty" yaml:"maintenance,omitempty"`

	// ErrorPages configures the default custom error pages for every host; a
	// ProxyHost's own errorPages overrides this. Zero value keeps today's plain
	// gpm error output.
	ErrorPages ErrorPagesConfig `json:"errorPages,omitempty" yaml:"errorPages,omitempty"`

	// SecurityHeaders is the fleet-default set of response headers gpm emits on
	// the responses IT generates - auth-gate denials, sign-in redirects, error
	// pages, path-rejection 400s, the no-such-host 404, parked/redirect hosts.
	// A ProxyHost's own securityHeaders merges over this per key (the host value
	// wins for a header it names; keys it omits fall through to this default).
	// Empty (the default) ships nothing, so an existing deployment is unchanged;
	// see RecommendedSecurityHeaders for a paste-ready set. Each header carries a
	// per-header scope (all / generated-only / proxied-only); the default (and an
	// old plain-string-map value) is "all". On a PROXIED upstream response an
	// applicable header is set-if-absent, so an app's own X-Frame-Options /
	// Referrer-Policy is never clobbered. Strict-Transport-Security is NOT
	// settable here - the per-host hsts setting owns it.
	SecurityHeaders map[string]SecurityHeaderValue `json:"securityHeaders,omitempty" yaml:"securityHeaders,omitempty"`

	// StripResponseHeaders is the fleet-default list of response headers removed
	// from what an UPSTREAM sends - the backend-identifying headers an app leaks
	// (Server, X-Powered-By, X-AspNet-Version, ...). Matching is
	// case-insensitive.
	//
	// The removal happens on the upstream response's own header map before those
	// headers are copied onto the client response, so it reaches exactly what the
	// backend sent and nothing gpm adds (securityHeaders, HSTS, X-Robots-Tag, a
	// forward-auth Set-Cookie refresh, gzip's Content-Encoding, a headers
	// middleware's setResponse). It covers 101 Upgrade responses too, so a
	// WebSocket handshake does not leak the fingerprint an ordinary response
	// hides. gpm's own generated responses (denials, redirects, error pages, the
	// 404, the upstream-unreachable 502/504) have no upstream response at all and
	// are untouched.
	//
	// A ProxyHost's own stripResponseHeaders is the UNION with this default (a
	// list has no per-key value to override, so a host can only ADD to the fleet
	// baseline - it cannot re-expose a header the fleet strips).
	//
	// Empty (the default) strips nothing.
	StripResponseHeaders []string `json:"stripResponseHeaders,omitempty" yaml:"stripResponseHeaders,omitempty"`
}

func (s Settings) Kind() string { return "Settings" }

func (s Settings) Validate() error {
	if s.ExternalBaseURL != "" {
		u, err := url.Parse(s.ExternalBaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("settings: externalBaseURL must be an absolute URL, got %q", s.ExternalBaseURL)
		}
	}
	if s.AdminAuth.SSOOnly && len(s.AdminAuth.Providers) == 0 {
		return fmt.Errorf("settings: ssoOnly requires at least one adminAuth.providers entry")
	}
	// Anti-lockout: at least one admin login method must remain. Without local
	// login AND without any SSO provider there is no way into the panel; reject the
	// commit instead of silently locking the operator out (recoverable only by a
	// redeploy). ssoOnly+providers and localLoginEnabled each satisfy this.
	if !s.AdminAuth.LocalLoginEnabled && len(s.AdminAuth.Providers) == 0 {
		return fmt.Errorf("settings: no admin login method configured (enable adminAuth.localLoginEnabled or add adminAuth.providers)")
	}
	seen := map[string]struct{}{}
	for i, w := range s.Webhooks {
		if err := ValidateName(w.Name); err != nil {
			return fmt.Errorf("settings: webhook[%d]: %w", i, err)
		}
		if _, dup := seen[w.Name]; dup {
			return fmt.Errorf("settings: duplicate webhook name %q", w.Name)
		}
		seen[w.Name] = struct{}{}
		u, err := url.Parse(w.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("settings: webhook %q: url must be an absolute http(s) URL, got %q", w.Name, w.URL)
		}
	}
	if err := s.ProxyProtocol.validate(); err != nil {
		return err
	}
	if err := s.DNSSync.Validate(); err != nil {
		return err
	}
	if err := s.IngressDiscovery.Validate(); err != nil {
		return err
	}
	if err := s.ErrorPages.validate(); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := s.Maintenance.validate(); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := validateSecurityHeaders(s.SecurityHeaders); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := validateStripResponseHeaders(s.StripResponseHeaders); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}

// DefaultSettings returns a safe starting configuration.
func DefaultSettings() Settings {
	return Settings{
		SchemaVersion: SchemaVersion,
		AppName:       "Go Proxy Manager",
		AdminAuth:     AdminAuthSettings{LocalLoginEnabled: true},
	}
}
