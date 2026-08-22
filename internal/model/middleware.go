package model

import (
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

// validCIDROrIP reports whether s parses as a CIDR or a bare IP.
func validCIDROrIP(s string) bool {
	if _, _, err := net.ParseCIDR(s); err == nil {
		return true
	}
	return net.ParseIP(s) != nil
}

// Middleware types. The compiled data plane applies a host/location's referenced
// middlewares as an ordered chain of http.Handler wrappers - so auth, headers,
// (and later rate-limit, WAF hooks) are first-class ordered steps with no
// textual-collision class of bug. New behaviours add a Type, never a rewrite.
const (
	MWTypeAuth      = "auth"
	MWTypeHeaders   = "headers"
	MWTypeGuard     = "guard"
	MWTypeRateLimit = "rate-limit" // per-host, per-client-IP token bucket
	MWTypeRewrite   = "rewrite"    // exact-match upstream-facing request-path replacement
)

// AuthMode selects how an auth middleware authenticates.
const (
	AuthModeOIDC        = "oidc"         // redirect the browser through the OIDC flow
	AuthModeForwardAuth = "forward-auth" // accept a trusted forward-auth identity
	AuthModeAuthRequest = "auth-request" // delegate to an external auth_request endpoint
	AuthModeClientCert  = "client-cert"  // gate on a verified mTLS client certificate
)

// AuthMiddleware gates requests through a named IdentityProvider and, optionally,
// requires the resolved identity to hold one of RequiredRoles.
type AuthMiddleware struct {
	IdentityProvider string   `json:"identityProvider" yaml:"identityProvider"`
	Mode             string   `json:"mode,omitempty" yaml:"mode,omitempty"` // oidc | forward-auth | auth-request (defaults from IdP type)
	RequiredRoles    []string `json:"requiredRoles,omitempty" yaml:"requiredRoles,omitempty"`
	// AllowFrom lists client CIDRs that bypass authentication entirely and are
	// proxied straight through (no auth subrequest, no identity headers). An
	// any-of, network-exempt bypass so trusted networks (e.g. LAN) can skip SSO.
	// Applies to auth-request mode.
	AllowFrom []string `json:"allowFrom,omitempty" yaml:"allowFrom,omitempty"`
	// ClientCertRoles maps a verified client certificate to a role in client-cert
	// mode: the key is the certificate subject (RFC 2253 form, e.g.
	// "CN=ops,O=Corp") or its bare common name, the value is the role name
	// RequiredRoles is checked against. Empty means cert presence alone passes.
	ClientCertRoles map[string]string `json:"clientCertRoles,omitempty" yaml:"clientCertRoles,omitempty"`
}

// HeadersMiddleware mutates request/response headers declaratively (security
// headers, Host spoofing, etc.) instead of via raw config text.
type HeadersMiddleware struct {
	SetRequest     map[string]string `json:"setRequest,omitempty" yaml:"setRequest,omitempty"`
	SetResponse    map[string]string `json:"setResponse,omitempty" yaml:"setResponse,omitempty"`
	RemoveRequest  []string          `json:"removeRequest,omitempty" yaml:"removeRequest,omitempty"`
	RemoveResponse []string          `json:"removeResponse,omitempty" yaml:"removeResponse,omitempty"`
}

// GuardMiddleware expresses conditional deny rules (block a path/method unless
// the client is in an allow-list): it denies a matching request unless the
// client is in an allow-list of networks.
// It fires when ANY Trigger matches; a request that matches and whose client IP
// is not in AllowFrom gets DenyStatus. This expresses rules the whole-host/
// location AccessList cannot - e.g. "POST to /login is LAN-only" (break-glass).
type GuardMiddleware struct {
	Triggers   []GuardTrigger `json:"triggers" yaml:"triggers"`
	AllowFrom  []string       `json:"allowFrom,omitempty" yaml:"allowFrom,omitempty"`   // client CIDRs exempt from the deny
	DenyStatus int            `json:"denyStatus,omitempty" yaml:"denyStatus,omitempty"` // default 403
}

// GuardTrigger matches a request. A trigger matches when ALL of its set fields
// match: the path equals one of Paths (if any), the method is one of Methods (if
// any), and every QueryEquals arg equals its value.
type GuardTrigger struct {
	Paths       []string          `json:"paths,omitempty" yaml:"paths,omitempty"`
	Methods     []string          `json:"methods,omitempty" yaml:"methods,omitempty"`
	QueryEquals map[string]string `json:"queryEquals,omitempty" yaml:"queryEquals,omitempty"`
}

// RateLimitMiddleware throttles a host's requests with a per-client-IP token
// bucket: steady-state rate with a Burst allowance (default ceil(rate)).
// Enforced on the data plane (internal/dataplane/ratelimit.go).
//
// The rate can be expressed two ways, and exactly one must be set:
//   - legacy shorthand: RequestsPerSecond (requests per 1s window), or
//   - Requests + Window (a Go duration string, e.g. "10s", "1m", "1h"), for
//     limits like "100 requests per 1m" that don't reduce cleanly to a
//     per-second rate.
type RateLimitMiddleware struct {
	RequestsPerSecond float64 `json:"requestsPerSecond,omitempty" yaml:"requestsPerSecond,omitempty"`
	// Requests and Window together express "N requests per window" (e.g.
	// Requests: 100, Window: "1m"). Mutually exclusive with RequestsPerSecond.
	Requests float64 `json:"requests,omitempty" yaml:"requests,omitempty"`
	Window   string  `json:"window,omitempty" yaml:"window,omitempty"`
	Burst    int     `json:"burst,omitempty" yaml:"burst,omitempty"`
	// AllowFrom lists client CIDRs that bypass rate limiting entirely: a matching
	// request skips the limiter (no token consumed, no 429). An any-of,
	// network-exempt bypass so trusted networks (e.g. LAN) are never throttled.
	AllowFrom []string `json:"allowFrom,omitempty" yaml:"allowFrom,omitempty"`
	// BlockFor, if set, is a Go duration string: once a client exceeds the limit,
	// further requests from it are rejected for this long regardless of token
	// refill. The block is fixed - it does not extend on repeat requests during
	// the block window. Optional; empty means no extra block (today's behavior).
	BlockFor string `json:"blockFor,omitempty" yaml:"blockFor,omitempty"`
}

// usesWindow reports whether the Requests+Window form is set (vs. the legacy
// RequestsPerSecond shorthand).
func (r RateLimitMiddleware) usesWindow() bool {
	return r.Requests > 0 || r.Window != ""
}

// RateAndDefaultBurst returns the steady-state refill rate (tokens/second) and
// the default burst/capacity (ceil of the configured requests, floored at 1)
// for whichever form (legacy or Requests+Window) is set. Callers must validate
// the middleware first; an invalid Window here falls back to 0/1 rather than
// panicking.
func (r RateLimitMiddleware) RateAndDefaultBurst() (rate float64, defaultBurst int) {
	requests := r.RequestsPerSecond
	if r.usesWindow() {
		requests = r.Requests
		d, err := time.ParseDuration(r.Window)
		if err != nil || d <= 0 {
			return 0, 1
		}
		rate = r.Requests / d.Seconds()
	} else {
		rate = r.RequestsPerSecond
	}
	burst := int(math.Ceil(requests))
	if burst < 1 {
		burst = 1
	}
	return rate, burst
}

// RewriteMiddleware rewrites the request path before proxying to the upstream.
// It is exact-match only (no regex): each key is a full request path that, when
// it equals the incoming path exactly, is replaced by its value. Exact matching
// avoids the path-confusion and ReDoS classes that pattern rewrites invite.
//
// The rewrite is internal - it mutates the proxied request path in place,
// preserving the method and body; it is never an HTTP redirect, so the client
// sees no 3xx and non-idempotent POSTs are forwarded unchanged. It is purely
// upstream-facing and runs innermost (closest to the backend), so auth, guards
// and access lists all evaluate the ORIGINAL client path - a rewrite can never
// move a request past a path-scoped security control.
type RewriteMiddleware struct {
	ReplacePath map[string]string `json:"replacePath,omitempty" yaml:"replacePath,omitempty"`
}

// Middleware is a reusable, named processing step referenced by hosts/locations.
type Middleware struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Type      string               `json:"type" yaml:"type"`
	Auth      *AuthMiddleware      `json:"auth,omitempty" yaml:"auth,omitempty"`
	Headers   *HeadersMiddleware   `json:"headers,omitempty" yaml:"headers,omitempty"`
	Guard     *GuardMiddleware     `json:"guard,omitempty" yaml:"guard,omitempty"`
	RateLimit *RateLimitMiddleware `json:"rateLimit,omitempty" yaml:"rateLimit,omitempty"`
	Rewrite   *RewriteMiddleware   `json:"rewrite,omitempty" yaml:"rewrite,omitempty"`
}

func (m Middleware) Kind() string { return "Middleware" }

func (m Middleware) Validate() error {
	if err := ValidateName(m.Name); err != nil {
		return err
	}
	switch m.Type {
	case MWTypeAuth:
		if m.Auth == nil {
			return fmt.Errorf("middleware %q: auth spec required", m.Name)
		}
		// client-cert mode authenticates from the TLS handshake, so it is the one
		// auth mode with no identity provider to name.
		if m.Auth.Mode != AuthModeClientCert && m.Auth.IdentityProvider == "" {
			return fmt.Errorf("middleware %q: auth.identityProvider is required", m.Name)
		}
		switch mode := m.Auth.Mode; mode {
		case "", AuthModeOIDC, AuthModeForwardAuth:
		case AuthModeClientCert:
			if m.Auth.IdentityProvider != "" {
				return fmt.Errorf("middleware %q: auth.identityProvider is not used in client-cert mode (the TLS handshake is the identity source)", m.Name)
			}
			// requiredRoles with no mapping could never match, silently denying
			// every request; reject it rather than compile a dead gate.
			if len(m.Auth.RequiredRoles) > 0 && len(m.Auth.ClientCertRoles) == 0 {
				return fmt.Errorf("middleware %q: auth.requiredRoles in client-cert mode needs auth.clientCertRoles to map a subject to a role", m.Name)
			}
		case AuthModeAuthRequest:
			if len(m.Auth.RequiredRoles) > 0 {
				return fmt.Errorf("middleware %q: auth.requiredRoles is not supported in auth-request mode (the auth server enforces authorization via its application bindings)", m.Name)
			}
		default:
			return fmt.Errorf("middleware %q: auth.mode must be oidc|forward-auth|auth-request|client-cert, got %q", m.Name, mode)
		}
		if len(m.Auth.ClientCertRoles) > 0 && m.Auth.Mode != AuthModeClientCert {
			return fmt.Errorf("middleware %q: auth.clientCertRoles is only used in client-cert mode", m.Name)
		}
		for _, c := range m.Auth.AllowFrom {
			if !validCIDROrIP(c) {
				return fmt.Errorf("middleware %q: auth.allowFrom has invalid CIDR/IP %q", m.Name, c)
			}
		}
	case MWTypeHeaders:
		if m.Headers == nil {
			return fmt.Errorf("middleware %q: headers spec required", m.Name)
		}
	case MWTypeGuard:
		if m.Guard == nil || len(m.Guard.Triggers) == 0 {
			return fmt.Errorf("middleware %q: guard requires at least one trigger", m.Name)
		}
		for _, c := range m.Guard.AllowFrom {
			if !validCIDROrIP(c) {
				return fmt.Errorf("middleware %q: guard.allowFrom has invalid CIDR/IP %q", m.Name, c)
			}
		}
		if s := m.Guard.DenyStatus; s != 0 && (s < 400 || s > 599) {
			return fmt.Errorf("middleware %q: guard.denyStatus must be a 4xx/5xx code, got %d", m.Name, s)
		}
	case MWTypeRateLimit:
		if m.RateLimit == nil {
			return fmt.Errorf("middleware %q: rateLimit.requestsPerSecond must be > 0", m.Name)
		}
		rl := m.RateLimit
		legacySet := rl.RequestsPerSecond > 0
		windowSet := rl.Requests > 0 || rl.Window != ""
		switch {
		case legacySet && windowSet:
			return fmt.Errorf("middleware %q: rateLimit must set either requestsPerSecond or requests+window, not both", m.Name)
		case !legacySet && !windowSet:
			return fmt.Errorf("middleware %q: rateLimit.requestsPerSecond must be > 0 (or set requests+window)", m.Name)
		case windowSet:
			if rl.Requests <= 0 {
				return fmt.Errorf("middleware %q: rateLimit.requests must be > 0", m.Name)
			}
			d, err := time.ParseDuration(rl.Window)
			if err != nil {
				return fmt.Errorf("middleware %q: rateLimit.window must be a valid duration (e.g. \"10s\", \"1m\", \"1h\"), got %q: %w", m.Name, rl.Window, err)
			}
			if d <= 0 {
				return fmt.Errorf("middleware %q: rateLimit.window must be > 0, got %q", m.Name, rl.Window)
			}
		}
		for _, c := range m.RateLimit.AllowFrom {
			if !validCIDROrIP(c) {
				return fmt.Errorf("middleware %q: rateLimit.allowFrom has invalid CIDR/IP %q", m.Name, c)
			}
		}
		if rl.BlockFor != "" {
			d, err := time.ParseDuration(rl.BlockFor)
			if err != nil {
				return fmt.Errorf("middleware %q: rateLimit.blockFor must be a valid duration (e.g. \"10s\", \"1m\", \"1h\"), got %q: %w", m.Name, rl.BlockFor, err)
			}
			if d <= 0 {
				return fmt.Errorf("middleware %q: rateLimit.blockFor must be > 0, got %q", m.Name, rl.BlockFor)
			}
		}
	case MWTypeRewrite:
		if m.Rewrite == nil || len(m.Rewrite.ReplacePath) == 0 {
			return fmt.Errorf("middleware %q: rewrite requires at least one replacePath entry", m.Name)
		}
		for k, v := range m.Rewrite.ReplacePath {
			if k == "" || !strings.HasPrefix(k, "/") {
				return fmt.Errorf("middleware %q: rewrite.replacePath key %q must be an absolute path (start with %q)", m.Name, k, "/")
			}
			if v == "" || !strings.HasPrefix(v, "/") {
				return fmt.Errorf("middleware %q: rewrite.replacePath[%q] target %q must be an absolute path (start with %q)", m.Name, k, v, "/")
			}
			if k == v {
				return fmt.Errorf("middleware %q: rewrite.replacePath[%q] rewrites a path to itself (no-op)", m.Name, k)
			}
		}
	default:
		return fmt.Errorf("middleware %q: unknown type %q", m.Name, m.Type)
	}
	return nil
}
