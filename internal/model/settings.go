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
