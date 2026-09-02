package model

import (
	"strings"
	"testing"
)

func TestACMEEffectiveChallenge(t *testing.T) {
	cases := []struct {
		name string
		spec *ACMESpec
		want string
	}{
		{"nil spec", nil, ChallengeHTTP01},
		{"explicit http-01", &ACMESpec{Challenge: ChallengeHTTP01}, ChallengeHTTP01},
		{"explicit dns-01", &ACMESpec{Challenge: ChallengeDNS01, DNSProvider: "cf"}, ChallengeDNS01},
		{"provider implies dns-01 (back-compat)", &ACMESpec{DNSProvider: "cf"}, ChallengeDNS01},
		{"no provider, no challenge -> http-01", &ACMESpec{}, ChallengeHTTP01},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.spec.EffectiveChallenge(); got != tc.want {
				t.Errorf("EffectiveChallenge() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCertificateValidateChallengeMatrix(t *testing.T) {
	base := func(domains []string, spec *ACMESpec) Certificate {
		return Certificate{
			ObjectMeta: ObjectMeta{Name: "c1"},
			Type:       CertTypeACME,
			Domains:    domains,
			ACME:       spec,
		}
	}
	email := "admin@example.com"

	cases := []struct {
		name    string
		cert    Certificate
		wantErr string // substring; empty means the config must validate
	}{
		{
			name: "dns-01 with provider",
			cert: base([]string{"*.example.com", "example.com"}, &ACMESpec{Email: email, Challenge: ChallengeDNS01, DNSProvider: "cf"}),
		},
		{
			name: "legacy config: provider, no challenge",
			cert: base([]string{"*.example.com"}, &ACMESpec{Email: email, DNSProvider: "cf"}),
		},
		{
			name: "http-01 without provider",
			cert: base([]string{"app.example.com"}, &ACMESpec{Email: email, Challenge: ChallengeHTTP01}),
		},
		{
			name: "no challenge, no provider defaults to http-01",
			cert: base([]string{"app.example.com"}, &ACMESpec{Email: email}),
		},
		{
			name:    "dns-01 without provider",
			cert:    base([]string{"app.example.com"}, &ACMESpec{Email: email, Challenge: ChallengeDNS01}),
			wantErr: "dnsProvider is required",
		},
		{
			name:    "http-01 with provider",
			cert:    base([]string{"app.example.com"}, &ACMESpec{Email: email, Challenge: ChallengeHTTP01, DNSProvider: "cf"}),
			wantErr: "only valid with the dns-01 challenge",
		},
		{
			name:    "wildcard over http-01",
			cert:    base([]string{"*.example.com"}, &ACMESpec{Email: email, Challenge: ChallengeHTTP01}),
			wantErr: "requires the dns-01 challenge",
		},
		{
			name:    "wildcard with no challenge and no provider",
			cert:    base([]string{"example.com", "*.example.com"}, &ACMESpec{Email: email}),
			wantErr: "requires the dns-01 challenge",
		},
		{
			name:    "unknown challenge",
			cert:    base([]string{"app.example.com"}, &ACMESpec{Email: email, Challenge: "tls-alpn-01"}),
			wantErr: "acme.challenge must be",
		},
		{
			name:    "missing email",
			cert:    base([]string{"app.example.com"}, &ACMESpec{Challenge: ChallengeHTTP01}),
			wantErr: "acme.email is required",
		},
		{
			name: "eab complete",
			cert: base([]string{"app.example.com"}, &ACMESpec{Email: email, EAB: &EABSpec{KID: "k", HMACKey: "${ENV:EAB_HMAC}"}}),
		},
		{
			name:    "eab without kid",
			cert:    base([]string{"app.example.com"}, &ACMESpec{Email: email, EAB: &EABSpec{HMACKey: "abc"}}),
			wantErr: "acme.eab.kid is required",
		},
		{
			name:    "eab without hmac key",
			cert:    base([]string{"app.example.com"}, &ACMESpec{Email: email, EAB: &EABSpec{KID: "k"}}),
			wantErr: "acme.eab.hmacKey is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cert.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestDNSProviderValidate(t *testing.T) {
	withToken := map[string]Secret{"apiToken": "${ENV:DNS_TOKEN}"}

	// One minimal valid config per known provider. A new provider without an
	// entry here fails the loop, so the table cannot silently fall behind.
	validConfigs := map[string]map[string]Secret{
		DNSProviderCloudflare:   withToken,
		DNSProviderDigitalOcean: withToken,
		DNSProviderHetzner:      withToken,
		DNSProviderDesec:        withToken,
		DNSProviderRFC2136: {
			"server":      "ns1.example.com",
			"zone":        "example.com",
			"tsigKeyName": "gpm-key",
			"tsigSecret":  "${ENV:GPM_TSIG_SECRET}",
		},
		DNSProviderACMEDNS: {
			"baseURL":   "https://acme-dns.example.com",
			"username":  "user",
			"password":  "${ENV:ACME_DNS_PASSWORD}",
			"subdomain": "d420c923-0000-4000-8000-000000000002",
		},
	}
	for _, provider := range KnownDNSProviders {
		cfg, ok := validConfigs[provider]
		if !ok {
			t.Errorf("provider %q has no valid config in this test; add one", provider)
			continue
		}
		p := DNSProvider{ObjectMeta: ObjectMeta{Name: "dns"}, Provider: provider, Config: cfg}
		if err := p.Validate(); err != nil {
			t.Errorf("provider %q: Validate() = %v, want nil", provider, err)
		}
	}

	rfc2136 := func(overrides map[string]Secret) DNSProvider {
		cfg := map[string]Secret{}
		for k, v := range validConfigs[DNSProviderRFC2136] {
			cfg[k] = v
		}
		for k, v := range overrides {
			if v == "" {
				delete(cfg, k)
				continue
			}
			cfg[k] = v
		}
		return DNSProvider{ObjectMeta: ObjectMeta{Name: "dns"}, Provider: DNSProviderRFC2136, Config: cfg}
	}
	acmeDNS := func(overrides map[string]Secret) DNSProvider {
		cfg := map[string]Secret{}
		for k, v := range validConfigs[DNSProviderACMEDNS] {
			cfg[k] = v
		}
		for k, v := range overrides {
			if v == "" {
				delete(cfg, k)
				continue
			}
			cfg[k] = v
		}
		return DNSProvider{ObjectMeta: ObjectMeta{Name: "dns"}, Provider: DNSProviderACMEDNS, Config: cfg}
	}

	cases := []struct {
		name    string
		p       DNSProvider
		wantErr string
	}{
		{
			name:    "unknown provider",
			p:       DNSProvider{ObjectMeta: ObjectMeta{Name: "dns"}, Provider: "route53", Config: withToken},
			wantErr: "provider must be one of",
		},
		{
			name:    "missing provider",
			p:       DNSProvider{ObjectMeta: ObjectMeta{Name: "dns"}, Config: withToken},
			wantErr: "provider is required",
		},
		{
			name:    "missing api token",
			p:       DNSProvider{ObjectMeta: ObjectMeta{Name: "dns"}, Provider: "hetzner"},
			wantErr: "config.apiToken is required",
		},
		{
			name:    "rfc2136 missing server",
			p:       rfc2136(map[string]Secret{"server": ""}),
			wantErr: "config.server is required for provider rfc2136",
		},
		{
			name:    "rfc2136 missing zone",
			p:       rfc2136(map[string]Secret{"zone": ""}),
			wantErr: "config.zone is required for provider rfc2136",
		},
		{
			name:    "rfc2136 missing tsig key name",
			p:       rfc2136(map[string]Secret{"tsigKeyName": ""}),
			wantErr: "config.tsigKeyName is required",
		},
		{
			name:    "rfc2136 missing tsig secret",
			p:       rfc2136(map[string]Secret{"tsigSecret": ""}),
			wantErr: "config.tsigSecret is required",
		},
		{
			name:    "rfc2136 hmac-md5 refused",
			p:       rfc2136(map[string]Secret{"tsigAlgorithm": "hmac-md5"}),
			wantErr: "hmac-md5 is refused",
		},
		{
			name:    "rfc2136 unknown algorithm",
			p:       rfc2136(map[string]Secret{"tsigAlgorithm": "hmac-sha3"}),
			wantErr: "config.tsigAlgorithm must be one of",
		},
		{
			name:    "rfc2136 bad transport",
			p:       rfc2136(map[string]Secret{"transport": "quic"}),
			wantErr: "config.transport must be tcp or udp",
		},
		{
			name:    "rfc2136 bad ttl",
			p:       rfc2136(map[string]Secret{"ttl": "0"}),
			wantErr: "config.ttl must be a whole number",
		},
		{
			name:    "rfc2136 bad timeout",
			p:       rfc2136(map[string]Secret{"timeout": "30"}),
			wantErr: "config.timeout must be a positive Go duration",
		},
		{
			name:    "acme-dns missing subdomain",
			p:       acmeDNS(map[string]Secret{"subdomain": ""}),
			wantErr: "config.subdomain is required for provider acme-dns",
		},
		{
			name:    "acme-dns missing password",
			p:       acmeDNS(map[string]Secret{"password": ""}),
			wantErr: "config.password is required",
		},
		{
			name:    "acme-dns bad base url",
			p:       acmeDNS(map[string]Secret{"baseURL": "acme-dns.example.com"}),
			wantErr: "config.baseURL must be an http or https URL",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
