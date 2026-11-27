package model

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
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
	DNSProviderWebhook      = "webhook"  // an operator's own HTTP endpoint that writes the record
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
	DNSProviderWebhook,
}

// TokenDNSProviders are the providers whose entire credential is a single API
// token under config.apiToken.
var TokenDNSProviders = []string{
	DNSProviderCloudflare,
	DNSProviderDigitalOcean,
	DNSProviderHetzner,
	DNSProviderDesec,
}

// dnsProviderPlainKeys lists, per provider, the config keys that are
// settings rather than credentials. See DNSProviderPlainKey.
var dnsProviderPlainKeys = map[string][]string{
	DNSProviderRFC2136: {"server", "zone", "tsigKeyName", "tsigAlgorithm", "transport", "ttl", "timeout"},
	DNSProviderACMEDNS: {"baseURL", "subdomain", "allowInsecureLocal"},
	DNSProviderWebhook: {"url", "allowInsecureLocal"},
}

// DNSProviderPlainKey reports whether a DNS provider config key is a plain
// setting rather than a secret. A plain key may hold a literal value: the
// store commits it to git as written and the API returns it as written,
// instead of refusing it (LiteralSecrets) and showing it as "***"
// (DNSProvider.MarshalJSON). A placeholder still works for it. Every other key
// is a secret: rfc2136 tsigSecret, acme-dns username and password, webhook
// token, the REST providers' apiToken, and any key of an unknown provider or
// unknown to its provider.
//
// A webhook url is plain even though a credential could be put in its query
// string: the bearer token is the place for the credential. Anyone who can
// read DNS providers sees a literal url, and it is in the config history;
// when the URL itself is sensitive, write it as a placeholder.
func DNSProviderPlainKey(provider, key string) bool {
	return slices.Contains(dnsProviderPlainKeys[provider], key)
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
	KeyType      string `json:"keyType,omitempty" yaml:"keyType,omitempty"`           // ecdsa only (default when empty); rsa is not implemented
	Challenge    string `json:"challenge,omitempty" yaml:"challenge,omitempty"`       // dns-01 | http-01
	// DNSProvider names a DNSProvider object used to solve dns-01 challenges.
	DNSProvider string `json:"dnsProvider,omitempty" yaml:"dnsProvider,omitempty"`
	// EAB binds this ACME account to an external CA account, when the CA needs it.
	EAB *EABSpec `json:"eab,omitempty" yaml:"eab,omitempty"`
	// Profile names the CA's issuance profile for every order of this
	// certificate (the CA lists its profiles in its directory). Empty means the
	// CA's default.
	Profile string `json:"profile,omitempty" yaml:"profile,omitempty"`
	// CABundle is the trust anchor set for a private CA's ACME directory: inline
	// PEM CERTIFICATE blocks, or a ${FILE:...} / ${ENV:...} placeholder that
	// resolves to them. When set, the directory's TLS certificate is checked
	// against these certificates only, never the system roots. A trust anchor is
	// not a secret, so this is a plain string, not a Secret: an inline bundle is
	// committed and returned by the API as written.
	CABundle string `json:"caBundle,omitempty" yaml:"caBundle,omitempty"`
}

// maxCABundle bounds acme.caBundle, as written and as resolved: a few
// certificates fit in a few KiB, so this leaves room for a long chain while
// keeping a mistaken file reference from loading something large.
const maxCABundle = 64 << 10

// acmeProfileRe is the accepted form of acme.profile.
var acmeProfileRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ParseCABundle parses an acme.caBundle value (already resolved, if it was a
// placeholder) into its certificates. It accepts one or more PEM CERTIFICATE
// blocks and nothing else: another block type, text outside the blocks or a
// certificate that does not parse is an error, so a pasted private key or a
// truncated bundle is refused instead of being half used.
func ParseCABundle(bundle string) ([]*x509.Certificate, error) {
	if len(bundle) > maxCABundle {
		return nil, fmt.Errorf("must be at most %d bytes, got %d", maxCABundle, len(bundle))
	}
	errText := errors.New("contains text that is not a PEM CERTIFICATE block")
	var certs []*x509.Certificate
	rest := bytes.TrimSpace([]byte(bundle))
	for len(rest) > 0 {
		// pem.Decode skips text before a block, and a malformed block along with
		// it; requiring every block to start right after the previous one (bar
		// whitespace) keeps any part of the value from being silently ignored.
		if !bytes.HasPrefix(rest, []byte("-----BEGIN ")) {
			return nil, errText
		}
		block, next := pem.Decode(rest)
		if block == nil || bytes.Count(rest[:len(rest)-len(next)], []byte("-----BEGIN ")) != 1 {
			return nil, errText
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("contains a %q PEM block; only CERTIFICATE blocks are allowed", block.Type)
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("certificate %d does not parse: %v", len(certs)+1, err)
		}
		certs = append(certs, c)
		rest = bytes.TrimSpace(next)
	}
	if len(certs) == 0 {
		return nil, errors.New("contains no PEM CERTIFICATE block")
	}
	return certs, nil
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
		return fmt.Errorf("certificate %q: domains: at least one domain is required", c.Name)
	}
	switch c.Type {
	case CertTypeACME:
		if c.ACME == nil {
			return fmt.Errorf("certificate %q: acme: spec required for type acme", c.Name)
		}
		if c.ACME.Email == "" {
			return fmt.Errorf("certificate %q: acme.email: is required", c.Name)
		}
		// Only ecdsa is actually issued (see internal/acme/order.go); catching an
		// unsupported keyType here means a config write fails immediately instead
		// of only surfacing at the next issue/renew attempt.
		if kt := c.ACME.KeyType; kt != "" && kt != "ecdsa" {
			return fmt.Errorf("certificate %q: acme.keyType: %q is not supported (only ecdsa is issued)", c.Name, kt)
		}
		if ch := c.ACME.Challenge; ch != "" && ch != ChallengeDNS01 && ch != ChallengeHTTP01 {
			return fmt.Errorf("certificate %q: acme.challenge: must be %s or %s, got %q", c.Name, ChallengeDNS01, ChallengeHTTP01, ch)
		}
		switch c.ACME.EffectiveChallenge() {
		case ChallengeDNS01:
			if c.ACME.DNSProvider == "" {
				return fmt.Errorf("certificate %q: acme.dnsProvider: is required for dns-01", c.Name)
			}
		case ChallengeHTTP01:
			if c.ACME.DNSProvider != "" {
				return fmt.Errorf("certificate %q: acme.dnsProvider: is only valid with the dns-01 challenge", c.Name)
			}
			// HTTP-01 validates a single name over port 80; a wildcard can only be
			// proven by control of the zone, i.e. dns-01.
			for _, d := range c.Domains {
				if strings.HasPrefix(d, "*.") {
					return fmt.Errorf("certificate %q: domains: wildcard domain %q requires the dns-01 challenge", c.Name, d)
				}
			}
		}
		if eab := c.ACME.EAB; eab != nil {
			if strings.TrimSpace(eab.KID) == "" {
				return fmt.Errorf("certificate %q: acme.eab.kid: is required when eab is set", c.Name)
			}
			if eab.HMACKey.IsEmpty() {
				return fmt.Errorf("certificate %q: acme.eab.hmacKey: is required when eab is set", c.Name)
			}
		}
		if p := c.ACME.Profile; p != "" && !acmeProfileRe.MatchString(p) {
			return fmt.Errorf("certificate %q: acme.profile: must be 1 to 64 characters of letters, digits, \"_\" or \"-\", got %q", c.Name, p)
		}
		if b := c.ACME.CABundle; b != "" {
			// A placeholder has no value at write time; it is resolved and parsed
			// at every renewal attempt instead.
			if len(b) > maxCABundle {
				return fmt.Errorf("certificate %q: acme.caBundle: must be at most %d bytes, got %d", c.Name, maxCABundle, len(b))
			}
			if !Secret(b).IsPlaceholder() {
				if _, err := ParseCABundle(b); err != nil {
					return fmt.Errorf("certificate %q: acme.caBundle: %w", c.Name, err)
				}
			}
		}
	case CertTypeCustom:
		if c.Custom == nil || c.Custom.CertFile == "" || c.Custom.KeyFile == "" {
			return fmt.Errorf("certificate %q: custom: certFile and keyFile are required", c.Name)
		}
		// Confine custom cert files to the managed cert store: reject absolute
		// paths and traversal so a config write cannot point the loader at an
		// arbitrary host file. The leading-slash/backslash checks make this
		// OS-agnostic: filepath.IsAbs only recognises a leading "/" on Unix, so a
		// Windows build would otherwise accept "/etc/shadow" or a "\"-rooted path.
		for _, f := range []string{c.Custom.CertFile, c.Custom.KeyFile} {
			if filepath.IsAbs(f) || strings.HasPrefix(f, "/") || strings.HasPrefix(f, `\`) || strings.Contains(filepath.Clean(f), "..") {
				return fmt.Errorf("certificate %q: custom: cert path %q must be relative to the cert store (no absolute or .. paths)", c.Name, f)
			}
		}
	default:
		return fmt.Errorf("certificate %q: type: must be custom or acme, got %q", c.Name, c.Type)
	}
	return nil
}

// DNSProvider holds reusable credentials for an ACME DNS-01 provider. Modelled
// as its own object so multiple certificates can share one credential set and so
// new providers are added by implementing an interface, not editing core. The
// REST providers authenticate with a single API token under config.apiToken;
// rfc2136, acme-dns and webhook take their own key sets. The Config map stays
// provider-specific and secret-valued.
type DNSProvider struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Provider string            `json:"provider" yaml:"provider"` // one of KnownDNSProviders
	Config   map[string]Secret `json:"config,omitempty" yaml:"config,omitempty"`
}

func (p DNSProvider) Kind() string { return "DNSProvider" }

// MarshalJSON keeps the struct's JSON shape and the redaction of secret
// config values (Secret.MarshalJSON: a placeholder verbatim, a literal as
// "***"), and returns a plain key's value as written (see
// DNSProviderPlainKey). YAML is not affected: the stored file keeps every
// value as written.
func (p DNSProvider) MarshalJSON() ([]byte, error) {
	type fields DNSProvider // the same fields without this method
	out := struct {
		fields
		// Shadows fields.Config: the shallower field wins in encoding/json.
		Config map[string]any `json:"config,omitempty"`
	}{fields: fields(p)}
	if len(p.Config) > 0 {
		out.Config = make(map[string]any, len(p.Config))
		for k, v := range p.Config {
			if DNSProviderPlainKey(p.Provider, k) {
				out.Config[k] = string(v)
			} else {
				out.Config[k] = v
			}
		}
	}
	return json.Marshal(out)
}

func (p DNSProvider) Validate() error {
	if err := ValidateName(p.Name); err != nil {
		return err
	}
	if p.Provider == "" {
		return fmt.Errorf("dns provider %q: provider: is required", p.Name)
	}
	known := false
	for _, k := range KnownDNSProviders {
		if p.Provider == k {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("dns provider %q: provider: must be one of %s, got %q", p.Name, strings.Join(KnownDNSProviders, ", "), p.Provider)
	}
	switch p.Provider {
	case DNSProviderRFC2136:
		return p.validateRFC2136()
	case DNSProviderACMEDNS:
		return p.validateACMEDNS()
	case DNSProviderWebhook:
		return p.validateWebhook()
	default:
		if p.Config["apiToken"].IsEmpty() {
			return fmt.Errorf("dns provider %q: config.apiToken: is required", p.Name)
		}
	}
	return nil
}

// requireConfig reports a missing required key with the exact config path an
// operator has to fix.
func (p DNSProvider) requireConfig(keys ...string) error {
	for _, k := range keys {
		if p.Config[k].IsEmpty() {
			return fmt.Errorf("dns provider %q: config.%s: is required for provider %s", p.Name, k, p.Provider)
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
			return fmt.Errorf("dns provider %q: config.tsigAlgorithm: hmac-md5 is refused; regenerate the key with hmac-sha256", p.Name)
		}
		if !slices.Contains(TSIGAlgorithms, alg) {
			return fmt.Errorf("dns provider %q: config.tsigAlgorithm: must be one of %s, got %q", p.Name, strings.Join(TSIGAlgorithms, ", "), alg)
		}
	}
	if t := strings.ToLower(strings.TrimSpace(string(p.Config["transport"]))); t != "" && t != "tcp" && t != "udp" {
		return fmt.Errorf("dns provider %q: config.transport: must be tcp or udp, got %q", p.Name, t)
	}
	if ttl := strings.TrimSpace(string(p.Config["ttl"])); ttl != "" {
		n, err := strconv.Atoi(ttl)
		if err != nil || n < 1 || n > 86400 {
			return fmt.Errorf("dns provider %q: config.ttl: must be a whole number of seconds between 1 and 86400, got %q", p.Name, ttl)
		}
	}
	if to := strings.TrimSpace(string(p.Config["timeout"])); to != "" {
		d, err := time.ParseDuration(to)
		if err != nil || d <= 0 {
			return fmt.Errorf("dns provider %q: config.timeout: must be a positive Go duration such as 30s, got %q", p.Name, to)
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

// validateWebhook checks the webhook solver's endpoint and bearer token. The
// URL is checked like an acme-dns baseURL: the token is sent to it.
func (p DNSProvider) validateWebhook() error {
	if err := p.requireConfig("url", "token"); err != nil {
		return err
	}
	if !p.Config["url"].IsPlaceholder() {
		raw := strings.TrimSpace(string(p.Config["url"]))
		allowLocal := IsTruthyConfig(string(p.Config["allowInsecureLocal"]))
		if err := ValidateOutboundBaseURL(fmt.Sprintf("dns provider %q: config.url", p.Name), raw, allowLocal); err != nil {
			return err
		}
		if HasURLUserInfo(raw) {
			return fmt.Errorf("dns provider %q: config.url: must not contain user info (user:password@); it is never sent, put the credential in config.token", p.Name)
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
// allowLocal (config.allowInsecureLocal) skips all of these address-literal
// checks at validation time, not just the loopback one: with it set, a
// link-local, unspecified or multicast literal is accepted here too. A
// link-local destination is still refused when the client connects (the
// shared outbound client's dial guard, see webhook.NewSecureClient).
//
// A hostname is not resolved here (validation is offline and a name can be
// re-pointed later); the runtime guard on the shared outbound HTTP client
// refuses a link-local destination post-DNS, which is what closes the rebinding
// case. This is the write-time half of that pair.
func ValidateOutboundBaseURL(field, raw string, allowLocal bool) error {
	return checkOutboundBaseURL(field, raw, allowLocal, true)
}

// ValidateResolvedOutboundURL is ValidateOutboundBaseURL for a value resolved
// from a ${ENV:...} / ${FILE:...} placeholder at run time: the same rules, but
// no error ever contains the value or any part of it, since the URL can carry
// a credential (user info, a query key) and the error reaches the
// certificate's last error, notifications and logs. ValidateOutboundBaseURL
// may echo the value: at write time it is what the caller just sent.
func ValidateResolvedOutboundURL(field, raw string, allowLocal bool) error {
	return checkOutboundBaseURL(field, raw, allowLocal, false)
}

func checkOutboundBaseURL(field, raw string, allowLocal, echo bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		if !echo {
			return fmt.Errorf("%s: is not an http or https URL (the value is not shown, it may hold a credential)", field)
		}
		return fmt.Errorf("%s: must be an http or https URL, got %q", field, raw)
	}
	host := u.Hostname()
	addr, err := netip.ParseAddr(host) // keeps a zone, which net.ParseIP would reject
	if err != nil || allowLocal {
		return nil
	}
	ip := net.IP(addr.WithZone("").AsSlice())
	switch {
	case ip.IsLoopback():
		if !echo {
			return fmt.Errorf("%s: points at a loopback address; set config.allowInsecureLocal: \"true\" if the service really runs beside gpm", field)
		}
		return fmt.Errorf("%s: points at the loopback address %s; set config.allowInsecureLocal: \"true\" if the service really runs beside gpm", field, ip)
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		if !echo {
			return fmt.Errorf("%s: points at a link-local address, which is the cloud metadata range and never a DNS provider", field)
		}
		return fmt.Errorf("%s: points at the link-local address %s, which is the cloud metadata range and never a DNS provider", field, ip)
	case ip.IsUnspecified() || ip.IsMulticast():
		if !echo {
			return fmt.Errorf("%s: points at an unspecified or multicast address, which is not a destination gpm can send credentials to", field)
		}
		return fmt.Errorf("%s: points at %s, which is not a destination gpm can send credentials to", field, ip)
	}
	return nil
}

// dialRefusedHost returns the host of raw when it is an address literal the
// shared outbound client refuses at connect time whatever the validation said:
// a link-local unicast or multicast address (refuseLinkLocal on
// webhook.NewSecureClient in internal/webhook/webhook.go, the client of the
// acme-dns and webhook DNS solvers). An IPv4-mapped IPv6 literal is checked as
// the IPv4 address it is dialed as. The guard refuses nothing else, so neither
// does this.
func dialRefusedHost(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	addr, err := netip.ParseAddr(u.Hostname())
	if err != nil {
		return "", false
	}
	addr = addr.Unmap()
	return u.Hostname(), addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast()
}

// HasURLUserInfo reports whether raw parses as a URL with user info
// (user:password@host). The webhook solver never sends it (it sets its own
// Authorization header), so a webhook url carrying it is refused rather than
// silently dropping the credential.
func HasURLUserInfo(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.User != nil
}
