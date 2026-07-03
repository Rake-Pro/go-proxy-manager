package model

import (
	"strings"
	"testing"
)

func TestClientCAValidate(t *testing.T) {
	tests := []struct {
		name    string
		ca      ClientCA
		wantErr string
	}{
		{
			name: "valid inline pem",
			ca:   ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"},
		},
		{
			name: "valid file placeholder",
			ca:   ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "${FILE:/run/secrets/corp_client_ca.pem}"},
		},
		{
			name:    "empty caPEM rejected",
			ca:      ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}},
			wantErr: "caPEM is required",
		},
		{
			name:    "bad name rejected",
			ca:      ClientCA{ObjectMeta: ObjectMeta{Name: "Corp!"}, CAPEM: "x"},
			wantErr: "invalid name",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ca.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestClientAuthValidation(t *testing.T) {
	clientCA := ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x"}

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "resolved clientAuth ref require",
			cfg: Config{
				ClientCAs: []ClientCA{clientCA},
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.TLS.ForceSSL = true
					h.TLS.ClientAuth = &ClientAuth{CARef: "corp", Mode: "require"}
				})},
			},
		},
		{
			name: "resolved clientAuth ref optional",
			cfg: Config{
				ClientCAs: []ClientCA{clientCA},
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.TLS.ForceSSL = true
					h.TLS.ClientAuth = &ClientAuth{CARef: "corp", Mode: "optional"}
				})},
			},
		},
		{
			name: "empty mode defaults valid",
			cfg: Config{
				ClientCAs: []ClientCA{clientCA},
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.TLS.ForceSSL = true
					h.TLS.ClientAuth = &ClientAuth{CARef: "corp"}
				})},
			},
		},
		{
			name: "dangling clientAuth ref rejected",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.TLS.ForceSSL = true
				h.TLS.ClientAuth = &ClientAuth{CARef: "nope", Mode: "require"}
			})}},
			wantErr: `references unknown clientCA "nope"`,
		},
		{
			name: "missing caRef rejected",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.TLS.ForceSSL = true
				h.TLS.ClientAuth = &ClientAuth{Mode: "require"}
			})}},
			wantErr: "caRef is required",
		},
		{
			name: "bad mode rejected",
			cfg: Config{
				ClientCAs: []ClientCA{clientCA},
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.TLS.ForceSSL = true
					h.TLS.ClientAuth = &ClientAuth{CARef: "corp", Mode: "sometimes"}
				})},
			},
			wantErr: `clientAuth.mode must be "require" or "optional"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}
