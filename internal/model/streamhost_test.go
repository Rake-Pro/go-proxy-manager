package model

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func streamHost(name string, port int, proto string) StreamHost {
	return StreamHost{
		ObjectMeta: ObjectMeta{Name: name},
		ListenPort: port,
		Protocol:   proto,
		Target:     StreamTarget{Host: "10.0.0.5", Port: 5432},
	}
}

// TestStreamHostTLSValidation is the per-object half of the validation matrix:
// what a single stream host may and may not say about TLS.
func TestStreamHostTLSValidation(t *testing.T) {
	cases := []struct {
		name    string
		host    StreamHost
		wantErr string
	}{
		{
			name: "no tls block is still valid",
			host: streamHost("plain", 5432, "tcp"),
		},
		{
			name: "passthrough with sni",
			host: func() StreamHost {
				h := streamHost("pt", 443, "tcp")
				h.TLS = &StreamTLS{Mode: StreamTLSPassthrough, SNIMatch: []string{"db.example.com"}}
				return h
			}(),
		},
		{
			name: "terminate with a certificate",
			host: func() StreamHost {
				h := streamHost("term", 443, "tcp")
				h.TLS = &StreamTLS{Mode: StreamTLSTerminate, CertificateRef: "c"}
				return h
			}(),
		},
		{
			name: "unknown mode",
			host: func() StreamHost {
				h := streamHost("bad", 443, "tcp")
				h.TLS = &StreamTLS{Mode: "sniff"}
				return h
			}(),
			wantErr: "tls.mode must be",
		},
		{
			name: "terminate without a certificate",
			host: func() StreamHost {
				h := streamHost("bad", 443, "tcp")
				h.TLS = &StreamTLS{Mode: StreamTLSTerminate}
				return h
			}(),
			wantErr: "tls.certificateRef is required",
		},
		{
			name: "passthrough with a certificate",
			host: func() StreamHost {
				h := streamHost("bad", 443, "tcp")
				h.TLS = &StreamTLS{Mode: StreamTLSPassthrough, CertificateRef: "c"}
				return h
			}(),
			wantErr: "not allowed in passthrough",
		},
		{
			name: "tls on a udp stream",
			host: func() StreamHost {
				h := streamHost("bad", 443, "udp")
				h.TLS = &StreamTLS{Mode: StreamTLSPassthrough, SNIMatch: []string{"a.example.com"}}
				return h
			}(),
			wantErr: "tls requires protocol tcp",
		},
		{
			name: "tls on a tcp+udp stream",
			host: func() StreamHost {
				h := streamHost("bad", 443, "both")
				h.TLS = &StreamTLS{Mode: StreamTLSPassthrough, SNIMatch: []string{"a.example.com"}}
				return h
			}(),
			wantErr: "tls requires protocol tcp",
		},
		{
			name: "malformed sni",
			host: func() StreamHost {
				h := streamHost("bad", 443, "tcp")
				h.TLS = &StreamTLS{Mode: StreamTLSPassthrough, SNIMatch: []string{"not a hostname"}}
				return h
			}(),
			wantErr: "invalid tls.sniMatch",
		},
		{
			name: "duplicate sni within one host",
			host: func() StreamHost {
				h := streamHost("bad", 443, "tcp")
				h.TLS = &StreamTLS{Mode: StreamTLSPassthrough, SNIMatch: []string{"a.example.com", "A.example.com"}}
				return h
			}(),
			wantErr: "duplicate tls.sniMatch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.host.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestStreamHostConfigValidation is the cross-object half: port sharing, access
// list references, and the basic-auth rejection that keeps an operator from
// believing a stream port is credential-gated when it cannot be.
func TestStreamHostConfigValidation(t *testing.T) {
	sniHost := func(name string, port int, sni ...string) StreamHost {
		h := streamHost(name, port, "tcp")
		h.TLS = &StreamTLS{Mode: StreamTLSPassthrough, SNIMatch: sni}
		return h
	}

	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "two SNI-routed hosts may share a tcp port",
			cfg: Config{StreamHosts: []StreamHost{
				sniHost("a", 443, "a.example.com"),
				sniHost("b", 443, "b.example.com"),
			}},
		},
		{
			name: "one host per port needs no sni",
			cfg: Config{StreamHosts: []StreamHost{
				streamHost("solo", 5432, "tcp"),
			}},
		},
		{
			name: "sharing a tcp port without sni is rejected",
			cfg: Config{StreamHosts: []StreamHost{
				streamHost("a", 5432, "tcp"),
				streamHost("b", 5432, "tcp"),
			}},
			wantErr: "sets no tls.sniMatch",
		},
		{
			name: "mixing an SNI host with a blind host on one port is rejected",
			cfg: Config{StreamHosts: []StreamHost{
				sniHost("a", 443, "a.example.com"),
				streamHost("b", 443, "tcp"),
			}},
			wantErr: "sets no tls.sniMatch",
		},
		{
			name: "two hosts claiming the same sni on one port is rejected",
			cfg: Config{StreamHosts: []StreamHost{
				sniHost("a", 443, "same.example.com"),
				sniHost("b", 443, "same.example.com"),
			}},
			wantErr: `both claim sni "same.example.com"`,
		},
		{
			name: "a disabled host does not claim its port",
			cfg: Config{StreamHosts: []StreamHost{
				func() StreamHost { h := streamHost("old", 5432, "tcp"); h.Disabled = true; return h }(),
				streamHost("new", 5432, "tcp"),
			}},
		},
		{
			name: "two hosts on one udp port is rejected",
			cfg: Config{StreamHosts: []StreamHost{
				streamHost("a", 5353, "udp"),
				streamHost("b", 5353, "udp"),
			}},
			wantErr: "both listen on udp port",
		},
		{
			name: "an unknown access list is rejected",
			cfg: Config{StreamHosts: []StreamHost{
				func() StreamHost { h := streamHost("a", 5432, "tcp"); h.AccessLists = []string{"nope"}; return h }(),
			}},
			wantErr: `references unknown accessList "nope"`,
		},
		{
			name: "an access list with basic auth is rejected for a stream",
			cfg: Config{
				AccessLists: []AccessList{{
					ObjectMeta: ObjectMeta{Name: "creds"},
					BasicAuth:  []BasicAuthUser{{Username: "u", PasswordHash: "$2a$10$abcdefghijklmnopqrstuv"}},
					Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8"}},
				}},
				StreamHosts: []StreamHost{
					func() StreamHost { h := streamHost("a", 5432, "tcp"); h.AccessLists = []string{"creds"}; return h }(),
				},
			},
			wantErr: "basic auth cannot be evaluated on a raw stream",
		},
		{
			name: "an ip-and-geo access list is accepted for a stream",
			cfg: Config{
				AccessLists: []AccessList{{
					ObjectMeta:    ObjectMeta{Name: "lan"},
					DefaultAction: ActionDeny,
					Rules:         []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8"}},
				}},
				StreamHosts: []StreamHost{
					func() StreamHost { h := streamHost("a", 5432, "tcp"); h.AccessLists = []string{"lan"}; return h }(),
				},
			},
		},
		{
			name: "an unknown certificate reference is rejected",
			cfg: Config{StreamHosts: []StreamHost{
				func() StreamHost {
					h := streamHost("a", 443, "tcp")
					h.TLS = &StreamTLS{Mode: StreamTLSTerminate, CertificateRef: "ghost"}
					return h
				}(),
			}},
			wantErr: `references unknown certificate "ghost"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestProxyProtocolSettingsValidation covers the one rule that matters: the
// feature cannot be turned on without naming the peers whose header is believed,
// because a header from anyone else is a free source-IP spoof.
func TestProxyProtocolSettingsValidation(t *testing.T) {
	base := func(p *ProxyProtocolSettings) Settings {
		s := DefaultSettings()
		s.ProxyProtocol = p
		return s
	}
	cases := []struct {
		name    string
		p       *ProxyProtocolSettings
		wantErr string
	}{
		{name: "absent", p: nil},
		{name: "disabled with no cidrs", p: &ProxyProtocolSettings{}},
		{name: "enabled with cidrs", p: &ProxyProtocolSettings{Enabled: true, TrustedCIDRs: []string{"10.0.0.0/8", "192.0.2.1"}, Timeout: "3s"}},
		{
			name:    "enabled with no cidrs",
			p:       &ProxyProtocolSettings{Enabled: true},
			wantErr: "trustedCIDRs is required",
		},
		{
			name:    "enabled with a malformed cidr",
			p:       &ProxyProtocolSettings{Enabled: true, TrustedCIDRs: []string{"10.0.0.0/33"}},
			wantErr: "invalid cidr/ip",
		},
		{
			name:    "unparseable timeout",
			p:       &ProxyProtocolSettings{Enabled: true, TrustedCIDRs: []string{"10.0.0.0/8"}, Timeout: "soon"},
			wantErr: "is not a duration",
		},
		{
			name:    "timeout out of range",
			p:       &ProxyProtocolSettings{Enabled: true, TrustedCIDRs: []string{"10.0.0.0/8"}, Timeout: "10m"},
			wantErr: "out of range",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := base(tc.p).Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}

	if got := (&ProxyProtocolSettings{}).HeaderTimeout(); got != DefaultProxyProtocolTimeout {
		t.Fatalf("unset timeout = %v, want %v", got, DefaultProxyProtocolTimeout)
	}
	if got := (&ProxyProtocolSettings{Timeout: "7s"}).HeaderTimeout(); got.String() != "7s" {
		t.Fatalf("timeout = %v, want 7s", got)
	}
}

// The retired forwardHost/forwardPort keys must fail loudly rather than decode
// into nothing: neither encoding/json nor yaml.v3 errors on an unknown key, so a
// stale config would otherwise validate with an empty target and silently lose
// its backend.
func TestStreamHostRejectsRetiredForwardKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		host StreamHost
	}{
		{"host only", StreamHost{
			ObjectMeta:        ObjectMeta{Name: "s"},
			ListenPort:        5432,
			Protocol:          "tcp",
			LegacyForwardHost: "10.0.0.5",
		}},
		{"port only", StreamHost{
			ObjectMeta:        ObjectMeta{Name: "s"},
			ListenPort:        5432,
			Protocol:          "tcp",
			LegacyForwardPort: 5432,
		}},
		{"both, alongside a valid target", StreamHost{
			ObjectMeta:        ObjectMeta{Name: "s"},
			ListenPort:        5432,
			Protocol:          "tcp",
			Target:            StreamTarget{Host: "10.0.0.5", Port: 5432},
			LegacyForwardHost: "10.0.0.9",
			LegacyForwardPort: 9999,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.host.Validate()
			if err == nil {
				t.Fatal("a config still using forwardHost/forwardPort must be rejected")
			}
			if !strings.Contains(err.Error(), "target") {
				t.Fatalf("the error must name the new shape, got %v", err)
			}
		})
	}
}

// The rejection has to survive the decoders the store and API actually use:
// both drop unknown keys silently, so the legacy keys are decoded into the
// shadow fields and caught by Validate.
func TestStreamHostRetiredKeysSurviveDecoding(t *testing.T) {
	for _, tc := range []struct {
		name   string
		decode func([]byte, *StreamHost) error
		blob   []byte
	}{
		{"yaml", func(b []byte, h *StreamHost) error { return yaml.Unmarshal(b, h) },
			[]byte("name: s\nlistenPort: 5432\nprotocol: tcp\nforwardHost: 10.0.0.5\nforwardPort: 5432\n")},
		{"json", func(b []byte, h *StreamHost) error { return json.Unmarshal(b, h) },
			[]byte(`{"name":"s","listenPort":5432,"protocol":"tcp","forwardHost":"10.0.0.5","forwardPort":5432}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var h StreamHost
			if err := tc.decode(tc.blob, &h); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if err := h.Validate(); err == nil || !strings.Contains(err.Error(), "target") {
				t.Fatalf("a decoded legacy config must be refused with the new shape named, got %v", err)
			}
		})
	}
}

// Nothing gpm writes carries the retired keys: they are omitempty, so a
// round-trip of a current object stays clean in both encodings.
func TestStreamHostDoesNotEmitRetiredKeys(t *testing.T) {
	h := streamHost("s", 5432, "tcp")
	y, err := yaml.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{string(y), string(j)} {
		if strings.Contains(out, "forwardHost") || strings.Contains(out, "forwardPort") {
			t.Fatalf("marshalled output still carries a retired key:\n%s", out)
		}
	}
}
