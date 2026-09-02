package acme

import (
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// defaultNewSolver is the only place a DNSProvider config becomes a solver, so
// every provider type has to survive the round trip from the config map.
func TestDefaultNewSolverEveryKnownProvider(t *testing.T) {
	t.Setenv("GPM_TEST_DNS_TOKEN", "token-value")
	configs := map[string]map[string]model.Secret{
		model.DNSProviderCloudflare:   {"apiToken": "${ENV:GPM_TEST_DNS_TOKEN}"},
		model.DNSProviderDigitalOcean: {"apiToken": "${ENV:GPM_TEST_DNS_TOKEN}"},
		model.DNSProviderHetzner:      {"apiToken": "${ENV:GPM_TEST_DNS_TOKEN}"},
		model.DNSProviderDesec:        {"apiToken": "${ENV:GPM_TEST_DNS_TOKEN}"},
		model.DNSProviderRFC2136: {
			"server":      "ns1.example.com",
			"zone":        "example.com",
			"tsigKeyName": "gpm-key",
			"tsigSecret":  model.Secret(testTSIGSecret),
		},
		model.DNSProviderACMEDNS: {
			"baseURL":   "https://acme-dns.example.com",
			"username":  "user",
			"password":  "pass",
			"subdomain": "sub",
		},
	}
	for _, name := range model.KnownDNSProviders {
		t.Run(name, func(t *testing.T) {
			cfg, ok := configs[name]
			if !ok {
				t.Fatalf("no test config for provider %q; add one when a provider is added", name)
			}
			p := model.DNSProvider{ObjectMeta: model.ObjectMeta{Name: "dns"}, Provider: name, Config: cfg}
			if err := p.Validate(); err != nil {
				t.Fatalf("the test config does not validate: %v", err)
			}
			s, err := defaultNewSolver(p)
			if err != nil {
				t.Fatalf("defaultNewSolver() = %v", err)
			}
			if s == nil {
				t.Fatal("defaultNewSolver() returned a nil solver")
			}
		})
	}
}

func TestDefaultNewSolverRFC2136Options(t *testing.T) {
	p := model.DNSProvider{
		ObjectMeta: model.ObjectMeta{Name: "dns"},
		Provider:   model.DNSProviderRFC2136,
		Config: map[string]model.Secret{
			"server":        "192.0.2.53:5353",
			"zone":          "example.com",
			"tsigKeyName":   "gpm-key",
			"tsigSecret":    model.Secret(testTSIGSecret),
			"tsigAlgorithm": "hmac-sha512",
			"ttl":           "120",
			"transport":     "udp",
			"timeout":       "45s",
		},
	}
	s, err := defaultNewSolver(p)
	if err != nil {
		t.Fatalf("defaultNewSolver() = %v", err)
	}
	got := s.(*RFC2136Solver)
	if got.server != "192.0.2.53:5353" || got.zone != "example.com" {
		t.Errorf("server/zone = %q/%q, want 192.0.2.53:5353/example.com", got.server, got.zone)
	}
	if got.alg.wire != "hmac-sha512." || got.ttl != 120 || got.transport != "udp" || got.timeout != 45*time.Second {
		t.Errorf("alg/ttl/transport/timeout = %s/%d/%s/%s, want hmac-sha512./120/udp/45s", got.alg.wire, got.ttl, got.transport, got.timeout)
	}
}

func TestDefaultNewSolverErrors(t *testing.T) {
	cases := []struct {
		name    string
		p       model.DNSProvider
		wantErr string
	}{
		{
			name:    "unknown provider",
			p:       model.DNSProvider{Provider: "route53", Config: map[string]model.Secret{"apiToken": "t"}},
			wantErr: "unsupported dns provider",
		},
		{
			name: "rfc2136 bad ttl",
			p: model.DNSProvider{Provider: model.DNSProviderRFC2136, Config: map[string]model.Secret{
				"server": "ns1.example.com", "zone": "example.com", "tsigKeyName": "k",
				"tsigSecret": model.Secret(testTSIGSecret), "ttl": "soon",
			}},
			wantErr: "rfc2136 ttl",
		},
		{
			name: "rfc2136 bad timeout",
			p: model.DNSProvider{Provider: model.DNSProviderRFC2136, Config: map[string]model.Secret{
				"server": "ns1.example.com", "zone": "example.com", "tsigKeyName": "k",
				"tsigSecret": model.Secret(testTSIGSecret), "timeout": "a while",
			}},
			wantErr: "rfc2136 timeout",
		},
		{
			name:    "acme-dns missing subdomain",
			p:       model.DNSProvider{Provider: model.DNSProviderACMEDNS, Config: map[string]model.Secret{"baseURL": "https://acme-dns.example.com", "username": "u", "password": "p"}},
			wantErr: "subdomain is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := defaultNewSolver(tc.p)
			if err == nil {
				t.Fatalf("defaultNewSolver() = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("defaultNewSolver() = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
