package model

import (
	"fmt"
	"net/url"
)

// Identity provider types.
const (
	IdPTypeOIDC        = "oidc"
	IdPTypeForwardAuth = "forward-auth"
	IdPTypeAuthRequest = "auth-request"
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
	TrustedProxies  []string `json:"trustedProxies" yaml:"trustedProxies"` // CIDRs allowed to assert identity
	UserHeader      string   `json:"userHeader" yaml:"userHeader"`         // e.g. X-authentik-username
	EmailHeader     string   `json:"emailHeader,omitempty" yaml:"emailHeader,omitempty"`
	NameHeader      string   `json:"nameHeader,omitempty" yaml:"nameHeader,omitempty"`
	GroupsHeader    string   `json:"groupsHeader,omitempty" yaml:"groupsHeader,omitempty"`
	GroupsDelimiter string   `json:"groupsDelimiter,omitempty" yaml:"groupsDelimiter,omitempty"` // default ","
	// AMRHeader, when set, carries the authentication methods the upstream
	// actually performed (space/comma separated, RFC 8176 tokens). It drives MFA
	// delegation. When empty, no MFA is asserted - never claim mfa the upstream
	// didn't prove.
	AMRHeader string `json:"amrHeader,omitempty" yaml:"amrHeader,omitempty"`
}

// AuthRequestSpec delegates authentication to an external auth server using the
// nginx auth_request pattern (e.g. an Authentik proxy outpost). Unlike
// ForwardAuthSpec - which TRUSTS identity headers already set by a fronting proxy
// - here gpm itself is the auth_request client: for every request it calls the
// auth server, copies the identity headers the server returns onto the upstream,
// and redirects unauthenticated browsers into the server's sign-in flow. This
// reproduces the per-host Authentik "Advanced" nginx snippet as typed config.
type AuthRequestSpec struct {
	// OutpostURL is the base URL of the auth server reachable from gpm,
	// e.g. http://auth-outpost:9000. Its host:port is the dial target; the
	// browser-facing host is preserved from the original request.
	OutpostURL string `json:"outpostURL" yaml:"outpostURL"`
	// PathPrefix is the path namespace the auth server owns (its /auth, /start,
	// /callback, /sign_out endpoints), proxied straight through and excluded from
	// gating. Default "/outpost.goauthentik.io".
	PathPrefix string `json:"pathPrefix,omitempty" yaml:"pathPrefix,omitempty"`
	// AuthPath is the per-request authentication subrequest endpoint, relative to
	// OutpostURL. Default PathPrefix + "/auth/nginx".
	AuthPath string `json:"authPath,omitempty" yaml:"authPath,omitempty"`
	// CopyHeaders are the response headers from the auth subrequest copied onto the
	// upstream request on success (and stripped from untrusted inbound requests so
	// a client cannot forge them). Default the Authentik X-authentik-* set.
	CopyHeaders []string `json:"copyHeaders,omitempty" yaml:"copyHeaders,omitempty"`
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
	// AllowDefaultAdmin must be true to permit defaultRole: "admin". Without it,
	// defaultRole: "admin" fails validation: it would grant full admin to EVERY
	// user the IdP authenticates, with no group gating. Requiring an explicit
	// opt-in stops that from happening silently by config typo.
	AllowDefaultAdmin bool `json:"allowDefaultAdmin,omitempty" yaml:"allowDefaultAdmin,omitempty"`
}

// IdentityProvider is a first-class auth source. One IdP can drive admin-panel
// login and also gate proxy hosts when referenced from an auth middleware.
type IdentityProvider struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Type        string           `json:"type" yaml:"type"` // oidc | forward-auth | auth-request
	OIDC        *OIDCSpec        `json:"oidc,omitempty" yaml:"oidc,omitempty"`
	ForwardAuth *ForwardAuthSpec `json:"forwardAuth,omitempty" yaml:"forwardAuth,omitempty"`
	AuthRequest *AuthRequestSpec `json:"authRequest,omitempty" yaml:"authRequest,omitempty"`
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
	case IdPTypeAuthRequest:
		if p.AuthRequest == nil {
			return fmt.Errorf("identity provider %q: authRequest spec required", p.Name)
		}
		u, err := url.Parse(p.AuthRequest.OutpostURL)
		if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("identity provider %q: authRequest.outpostURL must be an http(s) URL with a host, got %q", p.Name, p.AuthRequest.OutpostURL)
		}
		if pp := p.AuthRequest.PathPrefix; pp != "" && pp[0] != '/' {
			return fmt.Errorf("identity provider %q: authRequest.pathPrefix must start with /, got %q", p.Name, pp)
		}
	default:
		return fmt.Errorf("identity provider %q: type must be oidc, forward-auth or auth-request, got %q", p.Name, p.Type)
	}
	if rm := p.RoleMapping; rm != nil {
		switch rm.DefaultRole {
		case "", "user":
		case "admin":
			if !rm.AllowDefaultAdmin {
				return fmt.Errorf("identity provider %q: roleMapping.defaultRole %q grants admin to EVERY authenticated user with no group gating; set roleMapping.allowDefaultAdmin: true to confirm this is intended, or use adminGroups instead", p.Name, rm.DefaultRole)
			}
		default:
			return fmt.Errorf("identity provider %q: roleMapping.defaultRole must be \"\", \"user\", or \"admin\", got %q", p.Name, rm.DefaultRole)
		}
	}
	return nil
}
