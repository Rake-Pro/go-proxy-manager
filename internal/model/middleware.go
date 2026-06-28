package model

import (
	"fmt"
	"net"
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
)

// AuthMode selects how an auth middleware authenticates.
const (
	AuthModeOIDC        = "oidc"         // redirect the browser through the OIDC flow
	AuthModeForwardAuth = "forward-auth" // accept a trusted forward-auth identity
	AuthModeAuthRequest = "auth-request" // delegate to an external auth_request endpoint
)

// AuthMiddleware gates requests through a named IdentityProvider and, optionally,
// requires the resolved identity to hold one of RequiredRoles.
type AuthMiddleware struct {
	IdentityProvider string   `json:"identityProvider" yaml:"identityProvider"`
	Mode             string   `json:"mode,omitempty" yaml:"mode,omitempty"` // oidc | forward-auth | auth-request (defaults from IdP type)
	RequiredRoles    []string `json:"requiredRoles,omitempty" yaml:"requiredRoles,omitempty"`
	// AllowFrom lists client CIDRs that bypass authentication entirely and are
	// proxied straight through (no auth subrequest, no identity headers). The
	// typed form of NPM "satisfy any; allow <LAN>; deny all" - LAN skips SSO.
	// Applies to auth-request mode.
	AllowFrom []string `json:"allowFrom,omitempty" yaml:"allowFrom,omitempty"`
}

// HeadersMiddleware mutates request/response headers declaratively (security
// headers, Host spoofing, etc.) instead of via raw config text.
type HeadersMiddleware struct {
	SetRequest     map[string]string `json:"setRequest,omitempty" yaml:"setRequest,omitempty"`
	SetResponse    map[string]string `json:"setResponse,omitempty" yaml:"setResponse,omitempty"`
	RemoveRequest  []string          `json:"removeRequest,omitempty" yaml:"removeRequest,omitempty"`
	RemoveResponse []string          `json:"removeResponse,omitempty" yaml:"removeResponse,omitempty"`
}

// GuardMiddleware is the typed form of NPM's conditional nginx "if" blocks: it
// denies a matching request unless the client is in an allow-list of networks.
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
// bucket: steady-state RequestsPerSecond with a Burst allowance (default
// ceil(rps)). Enforced on the data plane (internal/dataplane/ratelimit.go).
type RateLimitMiddleware struct {
	RequestsPerSecond float64 `json:"requestsPerSecond" yaml:"requestsPerSecond"`
	Burst             int     `json:"burst,omitempty" yaml:"burst,omitempty"`
}

// Middleware is a reusable, named processing step referenced by hosts/locations.
type Middleware struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Type      string               `json:"type" yaml:"type"`
	Auth      *AuthMiddleware      `json:"auth,omitempty" yaml:"auth,omitempty"`
	Headers   *HeadersMiddleware   `json:"headers,omitempty" yaml:"headers,omitempty"`
	Guard     *GuardMiddleware     `json:"guard,omitempty" yaml:"guard,omitempty"`
	RateLimit *RateLimitMiddleware `json:"rateLimit,omitempty" yaml:"rateLimit,omitempty"`
}

func (m Middleware) Kind() string { return "Middleware" }

func (m Middleware) Validate() error {
	if err := ValidateName(m.Name); err != nil {
		return err
	}
	switch m.Type {
	case MWTypeAuth:
		if m.Auth == nil || m.Auth.IdentityProvider == "" {
			return fmt.Errorf("middleware %q: auth.identityProvider is required", m.Name)
		}
		switch mode := m.Auth.Mode; mode {
		case "", AuthModeOIDC, AuthModeForwardAuth:
		case AuthModeAuthRequest:
			if len(m.Auth.RequiredRoles) > 0 {
				return fmt.Errorf("middleware %q: auth.requiredRoles is not supported in auth-request mode (the auth server enforces authorization via its application bindings)", m.Name)
			}
		default:
			return fmt.Errorf("middleware %q: auth.mode must be oidc|forward-auth|auth-request, got %q", m.Name, mode)
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
		if m.RateLimit == nil || m.RateLimit.RequestsPerSecond <= 0 {
			return fmt.Errorf("middleware %q: rateLimit.requestsPerSecond must be > 0", m.Name)
		}
	default:
		return fmt.Errorf("middleware %q: unknown type %q", m.Name, m.Type)
	}
	return nil
}
