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
			name: "disabled clientCA ref rejected",
			cfg: Config{
				ClientCAs: []ClientCA{{ObjectMeta: ObjectMeta{Name: "corp", Disabled: true}, CAPEM: "x"}},
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.TLS.ForceSSL = true
					h.TLS.ClientAuth = &ClientAuth{CARef: "corp", Mode: "require"}
				})},
			},
			wantErr: `references clientCA "corp", which is disabled`,
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

// TestClientCARevocationValidation covers the phase-2 CRL fields: the file ref is
// confined to the cert store like a custom certificate's, the two sources are
// mutually exclusive, and a policy without a source is a configuration error.
func TestClientCARevocationValidation(t *testing.T) {
	tests := []struct {
		name    string
		ca      ClientCA
		wantErr string
	}{
		{"no revocation configured", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x"}, ""},
		{"relative crlFile", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", CRLFile: "corp.crl"}, ""},
		{"inline crlPEM", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", CRLPEM: "-----BEGIN X509 CRL-----"}, ""},
		{"fail-open policy", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", CRLFile: "c.crl", CRLPolicy: CRLPolicyFailOpen}, ""},
		{"both sources", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", CRLFile: "c.crl", CRLPEM: "p"}, "not both"},
		{"absolute crlFile", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", CRLFile: "/etc/shadow"}, "must be relative to the cert store"},
		{"traversal crlFile", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", CRLFile: "../../etc/shadow"}, "must be relative to the cert store"},
		{"unknown policy", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", CRLFile: "c.crl", CRLPolicy: "maybe"}, "crlPolicy must be"},
		{"policy without a source", ClientCA{ObjectMeta: ObjectMeta{Name: "corp"}, CAPEM: "x", CRLPolicy: CRLPolicyFailOpen}, "no crlFile or crlPEM"},
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

// TestClientCertPassthroughValidation covers tls.clientAuth.identityHeaders: the
// default header name needs no config, and a custom one must be a real header
// token that does not collide with the X-Forwarded-* headers gpm sets itself.
func TestClientCertPassthroughValidation(t *testing.T) {
	withHeaders := func(ih *ClientCertHeaders) TLSSettings {
		return TLSSettings{ForceSSL: true, ClientAuth: &ClientAuth{CARef: "corp", IdentityHeaders: ih}}
	}
	tests := []struct {
		name    string
		tls     TLSSettings
		wantErr string
	}{
		{"defaults", withHeaders(&ClientCertHeaders{}), ""},
		{"all attributes", withHeaders(&ClientCertHeaders{SAN: true, Serial: true, Fingerprint: true}), ""},
		{"custom name", withHeaders(&ClientCertHeaders{SubjectHeader: "X-Corp-Client"}), ""},
		{"header with a space", withHeaders(&ClientCertHeaders{SubjectHeader: "X Corp"}), "not a valid header name"},
		{"header with CRLF", withHeaders(&ClientCertHeaders{SubjectHeader: "X-C\r\nInjected"}), "not a valid header name"},
		{"x-forwarded collision", withHeaders(&ClientCertHeaders{SubjectHeader: "X-Forwarded-User"}), "must not be an X-Forwarded-*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tls.validate()
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

// TestClientCertAuthModeValidation covers the client-cert auth middleware mode:
// it takes no identity provider, and requiredRoles without a subject mapping
// could never match, so it is refused rather than compiled into a dead gate.
func TestClientCertAuthModeValidation(t *testing.T) {
	mw := func(a AuthMiddleware) Middleware {
		return Middleware{ObjectMeta: ObjectMeta{Name: "cert"}, Type: MWTypeAuth, Auth: &a}
	}
	tests := []struct {
		name    string
		mw      Middleware
		wantErr string
	}{
		{"bare client-cert", mw(AuthMiddleware{Mode: AuthModeClientCert}), ""},
		{"with a subject mapping", mw(AuthMiddleware{Mode: AuthModeClientCert,
			RequiredRoles: []string{"admin"}, ClientCertRoles: map[string]string{"ops": "admin"}}), ""},
		{"identity provider set", mw(AuthMiddleware{Mode: AuthModeClientCert, IdentityProvider: "authentik"}),
			"not used in client-cert mode"},
		{"roles without a mapping", mw(AuthMiddleware{Mode: AuthModeClientCert, RequiredRoles: []string{"admin"}}),
			"needs auth.clientCertRoles"},
		{"mapping in another mode", mw(AuthMiddleware{Mode: AuthModeForwardAuth, IdentityProvider: "authentik",
			ClientCertRoles: map[string]string{"ops": "admin"}}), "only used in client-cert mode"},
		{"no provider in another mode", mw(AuthMiddleware{Mode: AuthModeOIDC}), "auth.identityProvider is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mw.Validate()
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
