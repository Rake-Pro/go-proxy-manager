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
			name: "defaultRole admin without opt-in is rejected",
			cfg: Config{IdentityProviders: []IdentityProvider{{
				ObjectMeta: ObjectMeta{Name: "authentik"}, Type: IdPTypeOIDC,
				OIDC:        &OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
				RoleMapping: &RoleMapping{DefaultRole: "admin"},
			}}},
			wantErr: "set roleMapping.allowDefaultAdmin: true",
		},
		{
			name: "defaultRole admin with explicit opt-in is allowed",
			cfg: Config{IdentityProviders: []IdentityProvider{{
				ObjectMeta: ObjectMeta{Name: "authentik"}, Type: IdPTypeOIDC,
				OIDC:        &OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
				RoleMapping: &RoleMapping{DefaultRole: "admin", AllowDefaultAdmin: true},
			}}},
		},
		{
			name: "defaultRole user needs no opt-in",
			cfg: Config{IdentityProviders: []IdentityProvider{{
				ObjectMeta: ObjectMeta{Name: "authentik"}, Type: IdPTypeOIDC,
				OIDC:        &OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
				RoleMapping: &RoleMapping{DefaultRole: "user"},
			}}},
		},
		{
			name: "empty defaultRole is valid",
			cfg: Config{IdentityProviders: []IdentityProvider{{
				ObjectMeta: ObjectMeta{Name: "authentik"}, Type: IdPTypeOIDC,
				OIDC:        &OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
				RoleMapping: &RoleMapping{AdminGroups: []string{"proxy-admins"}},
			}}},
		},
		{
			name: "unknown defaultRole is rejected",
			cfg: Config{IdentityProviders: []IdentityProvider{{
				ObjectMeta: ObjectMeta{Name: "authentik"}, Type: IdPTypeOIDC,
				OIDC:        &OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
				RoleMapping: &RoleMapping{DefaultRole: "superuser"},
			}}},
			wantErr: `defaultRole must be "", "user", or "admin"`,
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
			name: "valid rate-limit middleware with allowFrom",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, AllowFrom: []string{"10.0.0.0/8"}},
			}}},
		},
		{
			name: "rate-limit rejects bad allowFrom",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, AllowFrom: []string{"nope"}},
			}}},
			wantErr: "invalid CIDR/IP",
		},
		{
			name: "valid rate-limit middleware with requests+window",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{Requests: 100, Window: "1m"},
			}}},
		},
		{
			name: "rate-limit rejects both requestsPerSecond and requests+window",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, Requests: 100, Window: "1m"},
			}}},
			wantErr: "not both",
		},
		{
			name: "rate-limit rejects neither requestsPerSecond nor requests+window",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{},
			}}},
			wantErr: "requestsPerSecond must be > 0",
		},
		{
			name: "rate-limit rejects unparseable window",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{Requests: 100, Window: "1fortnight"},
			}}},
			wantErr: "must be a valid duration",
		},
		{
			name: "rate-limit rejects zero window",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{Requests: 100, Window: "0s"},
			}}},
			wantErr: "window must be > 0",
		},
		{
			name: "rate-limit rejects requests<=0 with window set",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{Window: "1m"},
			}}},
			wantErr: "rateLimit.requests must be > 0",
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
		{
			name: "no admin login method is rejected",
			s: Settings{
				AdminAuth: AdminAuthSettings{LocalLoginEnabled: false},
			},
			wantErr: "no admin login method",
		},
		{
			name: "valid webhook is allowed",
			s: Settings{
				AdminAuth: AdminAuthSettings{LocalLoginEnabled: true},
				Webhooks:  []WebhookConfig{{Name: "ci", URL: "https://ci.example.com/hook"}},
			},
		},
		{
			name: "webhook with a non-absolute url is rejected",
			s: Settings{
				AdminAuth: AdminAuthSettings{LocalLoginEnabled: true},
				Webhooks:  []WebhookConfig{{Name: "ci", URL: "/relative"}},
			},
			wantErr: "absolute http(s) URL",
		},
		{
			name: "duplicate webhook name is rejected",
			s: Settings{
				AdminAuth: AdminAuthSettings{LocalLoginEnabled: true},
				Webhooks: []WebhookConfig{
					{Name: "ci", URL: "https://a.example.com"},
					{Name: "ci", URL: "https://b.example.com"},
				},
			},
			wantErr: "duplicate webhook name",
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

func TestProxyHostMinTLSVersion(t *testing.T) {
	for _, v := range []string{"", "1.2", "1.3"} {
		h := proxyHost("app", func(h *ProxyHost) { h.TLS.MinTLSVersion = v })
		if err := h.Validate(); err != nil {
			t.Errorf("minTLSVersion %q should be valid, got %v", v, err)
		}
	}
	h := proxyHost("app", func(h *ProxyHost) { h.TLS.MinTLSVersion = "1.1" })
	if err := h.Validate(); err == nil {
		t.Fatal("minTLSVersion 1.1 must be rejected")
	}
}

func TestClientAuthRequiresForceSSL(t *testing.T) {
	// mTLS without forceSSL must be rejected so the host can never be served in the clear.
	noForce := proxyHost("app", func(h *ProxyHost) {
		h.TLS.ClientAuth = &ClientAuth{CARef: "corp", Mode: "require"}
	})
	if err := noForce.Validate(); err == nil {
		t.Fatal("clientAuth without forceSSL must be rejected")
	}
	// With forceSSL:true the same host validates.
	withForce := proxyHost("app", func(h *ProxyHost) {
		h.TLS.ForceSSL = true
		h.TLS.ClientAuth = &ClientAuth{CARef: "corp", Mode: "require"}
	})
	if err := withForce.Validate(); err != nil {
		t.Fatalf("clientAuth with forceSSL:true should be valid, got %v", err)
	}
}
