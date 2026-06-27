package model

import "fmt"

// Identity provider types.
const (
	IdPTypeOIDC        = "oidc"
	IdPTypeForwardAuth = "forward-auth"
)

// OIDCSpec configures a generic OIDC provider (auth-code + PKCE, discovery).
// Used both for admin-panel login and, via auth middleware, to gate hosts.
type OIDCSpec struct {
	IssuerURL    string `json:"issuerURL" yaml:"issuerURL"`
	ClientID     string `json:"clientID" yaml:"clientID"`
	ClientSecret Secret `json:"clientSecret,omitempty" yaml:"clientSecret,omitempty"`
	// Scopes default to [openid, profile, email, groups] when empty.
	Scopes []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	// UsePKCE defaults to true; public clients can run with an empty secret.
	UsePKCE              *bool `json:"usePKCE,omitempty" yaml:"usePKCE,omitempty"`
	RequireVerifiedEmail bool  `json:"requireVerifiedEmail,omitempty" yaml:"requireVerifiedEmail,omitempty"`
	// TrustIdPMFA delegates MFA to the IdP: trust acr/amr instead of prompting a
	// second local TOTP (avoids the NPM TOTP + Authentik MFA double prompt).
	TrustIdPMFA bool `json:"trustIdPMFA,omitempty" yaml:"trustIdPMFA,omitempty"`
}

// ForwardAuthSpec accepts a trusted forward-auth identity asserted by an
// upstream authenticator (e.g. Authentik via X-authentik-* headers) and
// establishes the session directly: one authentication, no second "login with
// X" click. Headers are honoured ONLY when the request arrives from a trusted
// proxy CIDR, otherwise they are stripped to prevent header spoofing.
type ForwardAuthSpec struct {
	TrustedProxies []string `json:"trustedProxies" yaml:"trustedProxies"` // CIDRs allowed to assert identity
	UserHeader     string   `json:"userHeader" yaml:"userHeader"`         // e.g. X-authentik-username
	EmailHeader    string   `json:"emailHeader,omitempty" yaml:"emailHeader,omitempty"`
	NameHeader     string   `json:"nameHeader,omitempty" yaml:"nameHeader,omitempty"`
	GroupsHeader   string   `json:"groupsHeader,omitempty" yaml:"groupsHeader,omitempty"`
	GroupsDelimiter string  `json:"groupsDelimiter,omitempty" yaml:"groupsDelimiter,omitempty"` // default ","
}

// RoleMapping turns IdP groups/claims into local roles so SSO users become
// admins by claim (replacing manual account linking). Values matched in Groups
// against AdminGroups -> admin; otherwise DefaultRole (empty = deny access).
type RoleMapping struct {
	// GroupsClaim is the OIDC claim carrying group membership (default "groups").
	GroupsClaim string   `json:"groupsClaim,omitempty" yaml:"groupsClaim,omitempty"`
	AdminGroups []string `json:"adminGroups,omitempty" yaml:"adminGroups,omitempty"`
	UserGroups  []string `json:"userGroups,omitempty" yaml:"userGroups,omitempty"`
	// DefaultRole applies when no group matches: "" (deny), "user", or "admin".
	DefaultRole string `json:"defaultRole,omitempty" yaml:"defaultRole,omitempty"`
}

// IdentityProvider is a first-class auth source. One IdP can drive admin-panel
// login and also gate proxy hosts when referenced from an auth middleware.
type IdentityProvider struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Type        string           `json:"type" yaml:"type"` // oidc | forward-auth
	OIDC        *OIDCSpec        `json:"oidc,omitempty" yaml:"oidc,omitempty"`
	ForwardAuth *ForwardAuthSpec `json:"forwardAuth,omitempty" yaml:"forwardAuth,omitempty"`
	RoleMapping *RoleMapping     `json:"roleMapping,omitempty" yaml:"roleMapping,omitempty"`
}

func (p IdentityProvider) Kind() string { return "IdentityProvider" }

func (p IdentityProvider) Validate() error {
	if err := ValidateName(p.Name); err != nil {
		return err
	}
	switch p.Type {
	case IdPTypeOIDC:
		if p.OIDC == nil {
			return fmt.Errorf("identity provider %q: oidc spec required", p.Name)
		}
		if p.OIDC.IssuerURL == "" || p.OIDC.ClientID == "" {
			return fmt.Errorf("identity provider %q: oidc issuerURL and clientID are required", p.Name)
		}
	case IdPTypeForwardAuth:
		if p.ForwardAuth == nil {
			return fmt.Errorf("identity provider %q: forwardAuth spec required", p.Name)
		}
		if len(p.ForwardAuth.TrustedProxies) == 0 {
			return fmt.Errorf("identity provider %q: forwardAuth.trustedProxies is required (refuse to trust identity headers from anywhere)", p.Name)
		}
		if p.ForwardAuth.UserHeader == "" {
			return fmt.Errorf("identity provider %q: forwardAuth.userHeader is required", p.Name)
		}
	default:
		return fmt.Errorf("identity provider %q: type must be oidc or forward-auth, got %q", p.Name, p.Type)
	}
	return nil
}
