package model

import (
	"strings"
	"testing"
)

func proxyHost(name string, mod func(*ProxyHost)) ProxyHost {
	h := ProxyHost{
		ObjectMeta: ObjectMeta{Name: name},
		Domains:    []string{name + ".example.com"},
		Upstream:   Upstream{Scheme: "http", Host: "10.0.0.5", Port: 8080},
	}
	if mod != nil {
		mod(&h)
	}
	return h
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string // substring; empty means expect success
	}{
		{
			name: "valid minimal proxy host",
			cfg:  Config{ProxyHosts: []ProxyHost{proxyHost("app", nil)}},
		},
		{
			name: "dangling certificate ref",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.TLS.CertificateRef = "nope"
			})}},
			wantErr: `references unknown certificate "nope"`,
		},
		{
			name: "dangling middleware ref",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.Middlewares = []string{"ghost"}
			})}},
			wantErr: `references unknown middleware "ghost"`,
		},
		{
			name: "resolved certificate ref",
			cfg: Config{
				Certificates: []Certificate{{
					ObjectMeta: ObjectMeta{Name: "wild"}, Type: CertTypeCustom,
					Domains: []string{"*.example.com"},
					Custom:  &CustomCertSpec{CertFile: "c.pem", KeyFile: "k.pem"},
				}},
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.TLS.CertificateRef = "wild"
				})},
			},
		},
		{
			name: "duplicate host name",
			cfg: Config{ProxyHosts: []ProxyHost{
				proxyHost("dup", nil), proxyHost("dup", nil),
			}},
			wantErr: `duplicate host name "dup"`,
		},
		{
			name: "acme cert needs dns provider",
			cfg: Config{Certificates: []Certificate{{
				ObjectMeta: ObjectMeta{Name: "le"}, Type: CertTypeACME,
				Domains: []string{"*.example.com"},
				ACME:    &ACMESpec{Email: "a@b.c", Challenge: "dns-01", DNSProvider: "cf"},
			}}},
			wantErr: `references unknown dnsProvider "cf"`,
		},
		{
			name: "auth middleware needs identity provider",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "sso"}, Type: MWTypeAuth,
				Auth: &AuthMiddleware{IdentityProvider: "authentik"},
			}}},
			wantErr: `references unknown identityProvider "authentik"`,
		},
		{
			name: "valid auth-request identity provider + middleware",
			cfg: Config{
				IdentityProviders: []IdentityProvider{{
					ObjectMeta: ObjectMeta{Name: "authentik"}, Type: IdPTypeAuthRequest,
					AuthRequest: &AuthRequestSpec{OutpostURL: "http://auth-outpost:9000"},
				}},
				Middlewares: []Middleware{{
					ObjectMeta: ObjectMeta{Name: "sso"}, Type: MWTypeAuth,
					Auth: &AuthMiddleware{IdentityProvider: "authentik", Mode: AuthModeAuthRequest},
				}},
			},
		},
		{
			name: "auth-request idp requires a valid outpostURL",
			cfg: Config{IdentityProviders: []IdentityProvider{{
				ObjectMeta: ObjectMeta{Name: "authentik"}, Type: IdPTypeAuthRequest,
				AuthRequest: &AuthRequestSpec{OutpostURL: "not a url"},
			}}},
			wantErr: "outpostURL must be an http(s) URL",
		},
		{
			name: "auth-request mode rejects requiredRoles",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "sso"}, Type: MWTypeAuth,
				Auth: &AuthMiddleware{IdentityProvider: "authentik", Mode: AuthModeAuthRequest, RequiredRoles: []string{"admin"}},
			}}},
			wantErr: "requiredRoles is not supported in auth-request mode",
		},
		{
			name: "valid guard middleware",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "login-lan"}, Type: MWTypeGuard,
				Guard: &GuardMiddleware{
					Triggers:  []GuardTrigger{{Paths: []string{"/login"}, Methods: []string{"POST"}}},
					AllowFrom: []string{"192.0.2.0/24"},
				},
			}}},
		},
		{
			name: "guard requires a trigger",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "g"}, Type: MWTypeGuard, Guard: &GuardMiddleware{},
			}}},
			wantErr: "guard requires at least one trigger",
		},
		{
			name: "guard rejects bad allowFrom",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "g"}, Type: MWTypeGuard,
				Guard: &GuardMiddleware{Triggers: []GuardTrigger{{Paths: []string{"/x"}}}, AllowFrom: []string{"nope"}},
			}}},
			wantErr: "invalid CIDR/IP",
		},
		{
			name: "duplicate cert domain rejected",
			cfg: Config{Certificates: []Certificate{
				{ObjectMeta: ObjectMeta{Name: "a"}, Type: CertTypeCustom, Domains: []string{"*.example.com"}, Custom: &CustomCertSpec{CertFile: "a.pem", KeyFile: "ak.pem"}},
				{ObjectMeta: ObjectMeta{Name: "b"}, Type: CertTypeCustom, Domains: []string{"*.example.com"}, Custom: &CustomCertSpec{CertFile: "b.pem", KeyFile: "bk.pem"}},
			}},
			wantErr: `both claim domain "*.example.com"`,
		},
		{
			name: "disabled cert does not collide",
			cfg: Config{Certificates: []Certificate{
				{ObjectMeta: ObjectMeta{Name: "a"}, Type: CertTypeCustom, Domains: []string{"*.example.com"}, Custom: &CustomCertSpec{CertFile: "a.pem", KeyFile: "ak.pem"}},
				{ObjectMeta: ObjectMeta{Name: "b", Disabled: true}, Type: CertTypeCustom, Domains: []string{"*.example.com"}, Custom: &CustomCertSpec{CertFile: "b.pem", KeyFile: "bk.pem"}},
			}},
		},
		{
			name: "absolute custom cert path rejected",
			cfg: Config{Certificates: []Certificate{
				{ObjectMeta: ObjectMeta{Name: "a"}, Type: CertTypeCustom, Domains: []string{"x.example.com"}, Custom: &CustomCertSpec{CertFile: "/etc/shadow", KeyFile: "k.pem"}},
			}},
			wantErr: "must be relative to the cert store",
		},
		{
			name: "traversal custom cert path rejected",
			cfg: Config{Certificates: []Certificate{
				{ObjectMeta: ObjectMeta{Name: "a"}, Type: CertTypeCustom, Domains: []string{"x.example.com"}, Custom: &CustomCertSpec{CertFile: "../../etc/shadow", KeyFile: "k.pem"}},
			}},
			wantErr: "must be relative to the cert store",
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

func TestSettingsValidate(t *testing.T) {
	tests := []struct {
		name    string
		s       Settings
		wantErr string
	}{
		{name: "default is valid", s: DefaultSettings()},
		{
			name:    "bad base url",
			s:       Settings{ExternalBaseURL: "notaurl"},
			wantErr: "absolute URL",
		},
		{
			name: "sso-only without providers is rejected",
			s: Settings{
				AdminAuth: AdminAuthSettings{SSOOnly: true},
			},
			wantErr: "at least one adminAuth.providers",
		},
		{
			name: "sso-only with a provider is allowed",
			s: Settings{
				AdminAuth: AdminAuthSettings{SSOOnly: true, Providers: []string{"authentik"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.s.Validate()
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected success, got: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestSecretResolve(t *testing.T) {
	t.Setenv("GPM_TEST_SECRET", "s3cr3t")
	got, err := Secret("${ENV:GPM_TEST_SECRET}").Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("got %q, want s3cr3t", got)
	}
	if _, err := Secret("${ENV:GPM_TEST_MISSING}").Resolve(); err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !Secret("${ENV:X}").IsPlaceholder() {
		t.Fatal("expected placeholder detection")
	}
}
