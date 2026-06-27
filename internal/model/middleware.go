package model

import "fmt"

// Middleware types. The compiled data plane applies a host/location's referenced
// middlewares as an ordered chain of http.Handler wrappers - so auth, headers,
// (and later rate-limit, WAF hooks) are first-class ordered steps with no
// textual-collision class of bug. New behaviours add a Type, never a rewrite.
const (
	MWTypeAuth      = "auth"
	MWTypeHeaders   = "headers"
	MWTypeRateLimit = "rate-limit" // defined now, enforced in P1
)

// AuthMode selects how an auth middleware authenticates.
const (
	AuthModeOIDC        = "oidc"         // redirect the browser through the OIDC flow
	AuthModeForwardAuth = "forward-auth" // accept a trusted forward-auth identity
)

// AuthMiddleware gates requests through a named IdentityProvider and, optionally,
// requires the resolved identity to hold one of RequiredRoles.
type AuthMiddleware struct {
	IdentityProvider string   `json:"identityProvider" yaml:"identityProvider"`
	Mode             string   `json:"mode,omitempty" yaml:"mode,omitempty"` // oidc | forward-auth (defaults from IdP type)
	RequiredRoles    []string `json:"requiredRoles,omitempty" yaml:"requiredRoles,omitempty"`
}

// HeadersMiddleware mutates request/response headers declaratively (security
// headers, Host spoofing, etc.) instead of via raw config text.
type HeadersMiddleware struct {
	SetRequest     map[string]string `json:"setRequest,omitempty" yaml:"setRequest,omitempty"`
	SetResponse    map[string]string `json:"setResponse,omitempty" yaml:"setResponse,omitempty"`
	RemoveRequest  []string          `json:"removeRequest,omitempty" yaml:"removeRequest,omitempty"`
	RemoveResponse []string          `json:"removeResponse,omitempty" yaml:"removeResponse,omitempty"`
}

// RateLimitMiddleware is defined in the schema now so P1 enforcement is additive.
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
		if mode := m.Auth.Mode; mode != "" && mode != AuthModeOIDC && mode != AuthModeForwardAuth {
			return fmt.Errorf("middleware %q: auth.mode must be oidc|forward-auth, got %q", m.Name, mode)
		}
	case MWTypeHeaders:
		if m.Headers == nil {
			return fmt.Errorf("middleware %q: headers spec required", m.Name)
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
