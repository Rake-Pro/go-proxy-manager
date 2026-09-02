package model

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
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

// DNS-01 provider identifiers. The token-authenticated REST providers share one
// credential key (apiToken); the last two are generic escape hatches that work
// with any nameserver.
const (
	DNSProviderCloudflare   = "cloudflare"
	DNSProviderDigitalOcean = "digitalocean"
	DNSProviderHetzner      = "hetzner"
	DNSProviderDesec        = "desec"
	DNSProviderRFC2136      = "rfc2136"  // dynamic DNS UPDATE with a TSIG key
	DNSProviderACMEDNS      = "acme-dns" // joohoi/acme-dns, reached over a CNAME
)

// KnownDNSProviders lists the dns-01 providers with a built-in solver. The UI
// offers exactly these and validation rejects anything else, so a typo fails at
// config write time instead of at renewal time.
var KnownDNSProviders = []string{
	DNSProviderCloudflare,
	DNSProviderDigitalOcean,
	DNSProviderHetzner,
	DNSProviderDesec,
	DNSProviderRFC2136,
	DNSProviderACMEDNS,
}

// TokenDNSProviders are the providers whose entire credential is a single API
// token under config.apiToken.
var TokenDNSProviders = []string{
	DNSProviderCloudflare,
	DNSProviderDigitalOcean,
	DNSProviderHetzner,
	DNSProviderDesec,
}

// TSIGAlgorithms are the TSIG MAC algorithms the rfc2136 solver will sign with.
// HMAC-MD5 is deliberately absent: RFC 8945 keeps it only for backwards
// compatibility, and a key using it is refused with an explicit message.
var TSIGAlgorithms = []string{"hmac-sha1", "hmac-sha224", "hmac-sha256", "hmac-sha384", "hmac-sha512"}

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
// new providers are added by implementing an interface, not editing core. The
// REST providers authenticate with a single API token under config.apiToken;
// rfc2136 and acme-dns take their own key sets. The Config map stays
// provider-specific and secret-valued.
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
	switch p.Provider {
	case DNSProviderRFC2136:
		return p.validateRFC2136()
	case DNSProviderACMEDNS:
		return p.validateACMEDNS()
	default:
		if p.Config["apiToken"].IsEmpty() {
			return fmt.Errorf("dns provider %q: config.apiToken is required", p.Name)
		}
	}
	return nil
}

// requireConfig reports a missing required key with the exact config path an
// operator has to fix.
func (p DNSProvider) requireConfig(keys ...string) error {
	for _, k := range keys {
		if p.Config[k].IsEmpty() {
			return fmt.Errorf("dns provider %q: config.%s is required for provider %s", p.Name, k, p.Provider)
		}
	}
	return nil
}

// validateRFC2136 checks the dynamic-update credentials. Only literal (resolved
// at load time) fields are range-checked; secrets are checked for presence only,
// since a ${ENV:...} placeholder has no value yet at write time.
func (p DNSProvider) validateRFC2136() error {
	if err := p.requireConfig("server", "zone", "tsigKeyName", "tsigSecret"); err != nil {
		return err
	}
	if alg := strings.ToLower(strings.TrimSpace(string(p.Config["tsigAlgorithm"]))); alg != "" {
		if alg == "hmac-md5" || alg == "hmac-md5.sig-alg.reg.int" {
			return fmt.Errorf("dns provider %q: config.tsigAlgorithm hmac-md5 is refused; regenerate the key with hmac-sha256", p.Name)
		}
		if !slices.Contains(TSIGAlgorithms, alg) {
			return fmt.Errorf("dns provider %q: config.tsigAlgorithm must be one of %s, got %q", p.Name, strings.Join(TSIGAlgorithms, ", "), alg)
		}
	}
	if t := strings.ToLower(strings.TrimSpace(string(p.Config["transport"]))); t != "" && t != "tcp" && t != "udp" {
		return fmt.Errorf("dns provider %q: config.transport must be tcp or udp, got %q", p.Name, t)
	}
	if ttl := strings.TrimSpace(string(p.Config["ttl"])); ttl != "" {
		n, err := strconv.Atoi(ttl)
		if err != nil || n < 1 || n > 86400 {
			return fmt.Errorf("dns provider %q: config.ttl must be a whole number of seconds between 1 and 86400, got %q", p.Name, ttl)
		}
	}
	if to := strings.TrimSpace(string(p.Config["timeout"])); to != "" {
		d, err := time.ParseDuration(to)
		if err != nil || d <= 0 {
			return fmt.Errorf("dns provider %q: config.timeout must be a positive Go duration such as 30s, got %q", p.Name, to)
		}
	}
	return nil
}

// validateACMEDNS checks the acme-dns account credentials. The operator still has
// to create the CNAME from _acme-challenge.<domain> to <subdomain>.<acme-dns
// zone>; that cannot be validated here, so the solver warns about it at runtime.
func (p DNSProvider) validateACMEDNS() error {
	if err := p.requireConfig("baseURL", "username", "password", "subdomain"); err != nil {
		return err
	}
	base := strings.TrimSpace(string(p.Config["baseURL"]))
	if !p.Config["baseURL"].IsPlaceholder() {
		allowLocal := IsTruthyConfig(string(p.Config["allowInsecureLocal"]))
		if err := ValidateOutboundBaseURL(fmt.Sprintf("dns provider %q: config.baseURL", p.Name), base, allowLocal); err != nil {
			return err
		}
	}
	return nil
}

// IsTruthyConfig reads an opt-in flag out of a string-valued config map. Only an
// explicit affirmative counts; anything else (including an unset key) is false,
// so a typo can never turn a guard off.
func IsTruthyConfig(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "1", "on":
		return true
	}
	return false
}

// ValidateOutboundBaseURL checks a base URL gpm will POST operator credentials
// to. It must be an absolute http(s) URL, and - unless allowLocal is set - its
// host must not be an IP literal in a range that is not a real destination for
// an outbound integration: loopback, link-local (the cloud metadata range
// 169.254.169.254 / fe80::), unspecified or multicast.
//
// A hostname is not resolved here (validation is offline and a name can be
// re-pointed later); the runtime guard on the shared outbound HTTP client
// refuses a link-local destination post-DNS, which is what closes the rebinding
// case. This is the write-time half of that pair.
func ValidateOutboundBaseURL(field, raw string, allowLocal bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%s must be an http or https URL, got %q", field, raw)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip == nil || allowLocal {
		return nil
	}
	switch {
	case ip.IsLoopback():
		return fmt.Errorf("%s points at the loopback address %s; set config.allowInsecureLocal: \"true\" if the service really runs beside gpm", field, ip)
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return fmt.Errorf("%s points at the link-local address %s, which is the cloud metadata range and never a DNS provider", field, ip)
	case ip.IsUnspecified() || ip.IsMulticast():
		return fmt.Errorf("%s points at %s, which is not a destination gpm can send credentials to", field, ip)
	}
	return nil
}
