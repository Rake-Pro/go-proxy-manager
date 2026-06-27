// Package auth turns external identities (OIDC or trusted forward-auth) into
// local roles and admin sessions. The security-sensitive glue - header-trust
// boundaries, role mapping, SSO-only enforcement and break-glass - lives here;
// the OIDC protocol client and the session store are separate packages.
package auth

import "github.com/Rake-Pro/go-proxy-manager/internal/model"

// Role is a local authorization role.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
	RoleNone  Role = "" // no access
)

// Identity is a normalised, provider-agnostic view of an authenticated subject.
type Identity struct {
	Subject       string   // stable IdP subject identifier
	Email         string   //
	EmailVerified bool     //
	Name          string   //
	Groups        []string // group/role claims from the IdP
	AMR           []string // authentication methods (for MFA delegation)
	ACR           string   // authentication context class
	IdP           string   // name of the IdentityProvider object that issued this
}

// MapRole resolves IdP groups to a local role using the provider's mapping.
// A nil mapping denies (RoleNone) - SSO users must be explicitly granted a role,
// never auto-promoted. AdminGroups win over UserGroups; with no group match the
// configured DefaultRole applies (empty default = deny).
func MapRole(groups []string, rm *model.RoleMapping) Role {
	if rm == nil {
		return RoleNone
	}
	have := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		have[g] = struct{}{}
	}
	for _, g := range rm.AdminGroups {
		if _, ok := have[g]; ok {
			return RoleAdmin
		}
	}
	for _, g := range rm.UserGroups {
		if _, ok := have[g]; ok {
			return RoleUser
		}
	}
	switch Role(rm.DefaultRole) {
	case RoleAdmin:
		return RoleAdmin
	case RoleUser:
		return RoleUser
	default:
		return RoleNone
	}
}

// MFASatisfied reports whether the IdP asserted a multi-factor authentication,
// so we can delegate MFA to the IdP instead of double-prompting. It recognises
// the standard AMR method tokens (RFC 8176) and a non-empty, non-trivial ACR.
func MFASatisfied(id Identity) bool {
	for _, m := range id.AMR {
		switch m {
		case "mfa", "otp", "hwk", "sms", "swk", "tel", "fpt", "face", "iris", "retina", "vbm", "pin":
			return true
		}
	}
	// Some IdPs only signal step-up via acr; treat any acr beyond the common
	// "no factor"/level-0 sentinels as satisfied.
	switch id.ACR {
	case "", "0", "urn:mace:incommon:iap:silver":
		return false
	default:
		return true
	}
}
