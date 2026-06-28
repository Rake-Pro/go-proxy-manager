package model

import (
	"fmt"
	"net/url"
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
