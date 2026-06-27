package model

import (
	"fmt"
	"net/url"
)

// BreakGlassSettings defines the safe emergency-access path used when SSO-only
// mode hides local login. This is the deliberate fix for the fork's unauthed
// plaintext :81 break-glass: never an open port, but a real escape hatch.
type BreakGlassSettings struct {
	// LocalhostOnly permits a local admin login only from loopback (and the CLI).
	LocalhostOnly bool `json:"localhostOnly,omitempty" yaml:"localhostOnly,omitempty"`
	// EmergencyTokenHash is a bcrypt hash of a time-limited break-glass token.
	EmergencyTokenHash string `json:"emergencyTokenHash,omitempty" yaml:"emergencyTokenHash,omitempty"`
}

// Configured reports whether at least one safe break-glass mechanism exists.
func (b BreakGlassSettings) Configured() bool {
	return b.LocalhostOnly || b.EmergencyTokenHash != ""
}

// AdminAuthSettings governs how operators authenticate to the admin panel.
type AdminAuthSettings struct {
	// Providers names IdentityProvider objects allowed to log into the panel.
	Providers []string `json:"providers,omitempty" yaml:"providers,omitempty"`
	// LocalLoginEnabled keeps username/password login available (anti-lockout).
	LocalLoginEnabled bool `json:"localLoginEnabled" yaml:"localLoginEnabled"`
	// SSOOnly enforces SSO and hides the local form; requires safe BreakGlass.
	SSOOnly    bool               `json:"ssoOnly,omitempty" yaml:"ssoOnly,omitempty"`
	BreakGlass BreakGlassSettings `json:"breakGlass,omitempty" yaml:"breakGlass,omitempty"`
}

// Settings is the singleton app configuration, stored as config/settings.yaml.
type Settings struct {
	SchemaVersion int `json:"schemaVersion" yaml:"schemaVersion"`

	// ExternalBaseURL is the canonical public URL of the admin panel. It is
	// configured explicitly so OIDC redirect_uri is never derived from
	// X-Forwarded-* headers (the port/scheme footgun that broke fork logins).
	ExternalBaseURL string `json:"externalBaseURL" yaml:"externalBaseURL"`

	AdminAuth AdminAuthSettings `json:"adminAuth,omitempty" yaml:"adminAuth,omitempty"`
}

func (s Settings) Kind() string { return "Settings" }

func (s Settings) Validate() error {
	if s.ExternalBaseURL != "" {
		u, err := url.Parse(s.ExternalBaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("settings: externalBaseURL must be an absolute URL, got %q", s.ExternalBaseURL)
		}
	}
	if s.AdminAuth.SSOOnly {
		if len(s.AdminAuth.Providers) == 0 {
			return fmt.Errorf("settings: ssoOnly requires at least one adminAuth.providers entry")
		}
		if !s.AdminAuth.LocalLoginEnabled && !s.AdminAuth.BreakGlass.Configured() {
			return fmt.Errorf("settings: ssoOnly with local login disabled requires a safe breakGlass (localhostOnly or emergencyTokenHash) - refusing to lock you out")
		}
	}
	return nil
}

// DefaultSettings returns a safe starting configuration.
func DefaultSettings() Settings {
	return Settings{
		SchemaVersion: SchemaVersion,
		AdminAuth:     AdminAuthSettings{LocalLoginEnabled: true},
	}
}
