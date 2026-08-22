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

// ACME challenge types.
const (
	ChallengeDNS01  = "dns-01"
	ChallengeHTTP01 = "http-01"
)

// KnownDNSProviders lists the dns-01 providers with a built-in solver. The UI
// offers exactly these and validation rejects anything else, so a typo fails at
// config write time instead of at renewal time.
var KnownDNSProviders = []string{"cloudflare", "digitalocean", "hetzner", "desec"}

// EABSpec carries External Account Binding credentials (RFC 8555 s7.3.4): the
// key id and symmetric HMAC key a CA such as ZeroSSL or Google Public CA hands
// out to tie an ACME account to an existing customer account. Both are required
// together; the HMAC key is base64url (or standard base64) as issued by the CA.
type EABSpec struct {
	KID     string `json:"kid" yaml:"kid"`
	HMACKey Secret `json:"hmacKey" yaml:"hmacKey"`
}

// ACMESpec describes how to obtain a certificate from an ACME CA. Challenges are
// solved either over DNS-01 (via a referenced DNSProvider, the only way to get a
// wildcard) or HTTP-01 on the data plane's plaintext :80 listener; the directory
// URL is configurable so non-LE CAs (ZeroSSL, Google) slot in, with EAB where
// they require it.
type ACMESpec struct {
	Email        string `json:"email" yaml:"email"`
	DirectoryURL string `json:"directoryURL,omitempty" yaml:"directoryURL,omitempty"` // default: LE production
	KeyType      string `json:"keyType,omitempty" yaml:"keyType,omitempty"`           // ecdsa (default) | rsa
	Challenge    string `json:"challenge,omitempty" yaml:"challenge,omitempty"`       // dns-01 | http-01
	// DNSProvider names a DNSProvider object used to solve dns-01 challenges.
	DNSProvider string `json:"dnsProvider,omitempty" yaml:"dnsProvider,omitempty"`
	// EAB binds this ACME account to an external CA account, when the CA needs it.
	EAB *EABSpec `json:"eab,omitempty" yaml:"eab,omitempty"`
}

// EffectiveChallenge resolves the challenge type actually used: the explicit
// value when set, else dns-01 when a DNSProvider is referenced (back-compat with
// configs written before the field existed) and http-01 otherwise.
func (a *ACMESpec) EffectiveChallenge() string {
	if a == nil {
		return ChallengeHTTP01
	}
	if a.Challenge != "" {
		return a.Challenge
	}
	if a.DNSProvider != "" {
		return ChallengeDNS01
	}
	return ChallengeHTTP01
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
		if ch := c.ACME.Challenge; ch != "" && ch != ChallengeDNS01 && ch != ChallengeHTTP01 {
			return fmt.Errorf("certificate %q: acme.challenge must be %s or %s, got %q", c.Name, ChallengeDNS01, ChallengeHTTP01, ch)
		}
		switch c.ACME.EffectiveChallenge() {
		case ChallengeDNS01:
			if c.ACME.DNSProvider == "" {
				return fmt.Errorf("certificate %q: acme.dnsProvider is required for dns-01", c.Name)
			}
		case ChallengeHTTP01:
			if c.ACME.DNSProvider != "" {
				return fmt.Errorf("certificate %q: acme.dnsProvider is only valid with the dns-01 challenge", c.Name)
			}
			// HTTP-01 validates a single name over port 80; a wildcard can only be
			// proven by control of the zone, i.e. dns-01.
			for _, d := range c.Domains {
				if strings.HasPrefix(d, "*.") {
					return fmt.Errorf("certificate %q: wildcard domain %q requires the dns-01 challenge", c.Name, d)
				}
			}
		}
		if eab := c.ACME.EAB; eab != nil {
			if strings.TrimSpace(eab.KID) == "" {
				return fmt.Errorf("certificate %q: acme.eab.kid is required when eab is set", c.Name)
			}
			if eab.HMACKey.IsEmpty() {
				return fmt.Errorf("certificate %q: acme.eab.hmacKey is required when eab is set", c.Name)
			}
		}
	case CertTypeCustom:
		if c.Custom == nil || c.Custom.CertFile == "" || c.Custom.KeyFile == "" {
			return fmt.Errorf("certificate %q: custom certFile and keyFile are required", c.Name)
		}
		// Confine custom cert files to the managed cert store: reject absolute
		// paths and traversal so a config write cannot point the loader at an
		// arbitrary host file. The leading-slash/backslash checks make this
		// OS-agnostic: filepath.IsAbs only recognises a leading "/" on Unix, so a
		// Windows build would otherwise accept "/etc/shadow" or a "\"-rooted path.
		for _, f := range []string{c.Custom.CertFile, c.Custom.KeyFile} {
			if filepath.IsAbs(f) || strings.HasPrefix(f, "/") || strings.HasPrefix(f, `\`) || strings.Contains(filepath.Clean(f), "..") {
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
// new providers are added by implementing an interface, not editing core. Every
// shipped provider authenticates with a single API token under config.apiToken;
// the Config map stays provider-specific and secret-valued.
type DNSProvider struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Provider string            `json:"provider" yaml:"provider"` // one of KnownDNSProviders
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
	known := false
	for _, k := range KnownDNSProviders {
		if p.Provider == k {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("dns provider %q: provider must be one of %s, got %q", p.Name, strings.Join(KnownDNSProviders, ", "), p.Provider)
	}
	if p.Config["apiToken"].IsEmpty() {
		return fmt.Errorf("dns provider %q: config.apiToken is required", p.Name)
	}
	return nil
}
