package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Certificate types.
const (
	CertTypeCustom = "custom"
	CertTypeACME   = "acme"
)

// ACMESpec describes how to obtain a certificate from an ACME CA. P0 targets
// DNS-01 (for *.example.com wildcard) via a referenced DNSProvider; the directory
// URL is configurable so non-LE CAs (ZeroSSL, Google) slot in later.
type ACMESpec struct {
	Email        string `json:"email" yaml:"email"`
	DirectoryURL string `json:"directoryURL,omitempty" yaml:"directoryURL,omitempty"` // default: LE production
	KeyType      string `json:"keyType,omitempty" yaml:"keyType,omitempty"`           // ecdsa (default) | rsa
	Challenge    string `json:"challenge,omitempty" yaml:"challenge,omitempty"`       // dns-01 (P0)
	// DNSProvider names a DNSProvider object used to solve dns-01 challenges.
	DNSProvider string `json:"dnsProvider,omitempty" yaml:"dnsProvider,omitempty"`
}

// CustomCertSpec references a user-supplied certificate. Files live under the
// cert store (managed dir); they are not committed as config object fields.
type CustomCertSpec struct {
	CertFile string `json:"certFile" yaml:"certFile"` // PEM chain
	KeyFile  string `json:"keyFile" yaml:"keyFile"`   // PEM private key
}

// Certificate is the desired state for a TLS certificate. Issuance/renewal
// status (expiry, last error) is runtime state kept in the cache, not here.
type Certificate struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Type    string          `json:"type" yaml:"type"` // custom | acme
	Domains []string        `json:"domains" yaml:"domains"`
	ACME    *ACMESpec       `json:"acme,omitempty" yaml:"acme,omitempty"`
	Custom  *CustomCertSpec `json:"custom,omitempty" yaml:"custom,omitempty"`
}

func (c Certificate) Kind() string { return "Certificate" }

func (c Certificate) Validate() error {
	if err := ValidateName(c.Name); err != nil {
		return err
	}
	if len(c.Domains) == 0 {
		return fmt.Errorf("certificate %q: at least one domain is required", c.Name)
	}
	switch c.Type {
	case CertTypeACME:
		if c.ACME == nil {
			return fmt.Errorf("certificate %q: acme spec required for type acme", c.Name)
		}
		if c.ACME.Email == "" {
			return fmt.Errorf("certificate %q: acme.email is required", c.Name)
		}
		if ch := c.ACME.Challenge; ch != "" && ch != "dns-01" {
			return fmt.Errorf("certificate %q: only dns-01 challenge is supported in P0, got %q", c.Name, ch)
		}
		if c.ACME.DNSProvider == "" {
			return fmt.Errorf("certificate %q: acme.dnsProvider is required for dns-01", c.Name)
		}
	case CertTypeCustom:
		if c.Custom == nil || c.Custom.CertFile == "" || c.Custom.KeyFile == "" {
			return fmt.Errorf("certificate %q: custom certFile and keyFile are required", c.Name)
		}
		// Confine custom cert files to the managed cert store: reject absolute
		// paths and traversal so a config write cannot point the loader at an
		// arbitrary host file.
		for _, f := range []string{c.Custom.CertFile, c.Custom.KeyFile} {
			if filepath.IsAbs(f) || strings.Contains(filepath.Clean(f), "..") {
				return fmt.Errorf("certificate %q: custom cert path %q must be relative to the cert store (no absolute or .. paths)", c.Name, f)
			}
		}
	default:
		return fmt.Errorf("certificate %q: type must be custom or acme, got %q", c.Name, c.Type)
	}
	return nil
}

// DNSProvider holds reusable credentials for an ACME DNS-01 provider. Modelled
// as its own object so multiple certificates can share one credential set and so
// new providers are added by implementing an interface, not editing core. P0
// ships cloudflare; the Config map is provider-specific and secret-valued.
type DNSProvider struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Provider string            `json:"provider" yaml:"provider"` // cloudflare (P0)
	Config   map[string]Secret `json:"config,omitempty" yaml:"config,omitempty"`
}

func (p DNSProvider) Kind() string { return "DNSProvider" }

func (p DNSProvider) Validate() error {
	if err := ValidateName(p.Name); err != nil {
		return err
	}
	if p.Provider == "" {
		return fmt.Errorf("dns provider %q: provider is required", p.Name)
	}
	return nil
}
