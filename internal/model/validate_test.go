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
			wantErr: "outpostURL: must be an http(s) URL",
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
			wantErr: `defaultRole: must be "", "user", or "admin"`,
		},
		{
			name: "auth-request mode rejects requiredRoles",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "sso"}, Type: MWTypeAuth,
				Auth: &AuthMiddleware{IdentityProvider: "authentik", Mode: AuthModeAuthRequest, RequiredRoles: []string{"admin"}},
			}}},
			wantErr: "requiredRoles: is not supported in auth-request mode",
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
			wantErr: "guard: requires at least one trigger",
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
			wantErr: "requestsPerSecond: must be > 0",
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
			wantErr: "window: must be > 0",
		},
		{
			name: "rate-limit rejects requests<=0 with window set",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{Window: "1m"},
			}}},
			wantErr: "rateLimit.requests: must be > 0",
		},
		{
			name: "valid rate-limit middleware with blockFor and legacy rate form",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockFor: "5m"},
			}}},
		},
		{
			name: "valid rate-limit middleware with blockFor and requests+window form",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{Requests: 100, Window: "1m", BlockFor: "30s"},
			}}},
		},
		{
			name: "valid rate-limit middleware exempting an access list",
			cfg: Config{
				AccessLists: []AccessList{{ObjectMeta: ObjectMeta{Name: "lan"}, DefaultAction: ActionDeny, Rules: []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8"}}}},
				Middlewares: []Middleware{{
					ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
					RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, AllowFrom: []string{"@lan", "192.0.2.0/24"}},
				}},
			},
		},
		{
			name: "rate-limit rejects an unknown access list exemption",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, AllowFrom: []string{"@nope"}},
			}}},
			wantErr: `middleware "rl": rateLimit.allowFrom: references unknown accessList "nope"`,
		},
		{
			name: "rate-limit rejects a malformed access list exemption",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, AllowFrom: []string{"@Not A List"}},
			}}},
			wantErr: "is not a valid access list reference",
		},
		{
			name: "valid rate-limit middleware with blockBackoff and blockMax",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockFor: "1m", BlockBackoff: 2, BlockMax: "1h"},
			}}},
		},
		{
			name: "rate-limit rejects blockBackoff without blockFor",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockBackoff: 2},
			}}},
			wantErr: "blockBackoff: requires blockFor",
		},
		{
			name: "rate-limit rejects blockBackoff of 1 or less",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockFor: "1m", BlockBackoff: 1},
			}}},
			wantErr: "blockBackoff: must be > 1",
		},
		{
			name: "rate-limit rejects blockMax shorter than blockFor",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockFor: "1m", BlockBackoff: 2, BlockMax: "30s"},
			}}},
			wantErr: "blockMax: must be at least blockFor",
		},
		{
			name: "rate-limit rejects unparseable blockMax",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockFor: "1m", BlockMax: "soon"},
			}}},
			wantErr: "blockMax: must be a valid duration",
		},
		{
			name: "rate-limit rejects unparseable blockFor",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockFor: "1fortnight"},
			}}},
			wantErr: "blockFor: must be a valid duration",
		},
		{
			name: "rate-limit rejects zero blockFor",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockFor: "0s"},
			}}},
			wantErr: "blockFor: must be > 0",
		},
		{
			name: "rate-limit rejects negative blockFor",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
				RateLimit: &RateLimitMiddleware{RequestsPerSecond: 10, BlockFor: "-5s"},
			}}},
			wantErr: "blockFor: must be > 0",
		},
		{
			name: "valid rewrite middleware",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "authentik-token-slash"}, Type: MWTypeRewrite,
				Rewrite: &RewriteMiddleware{ReplacePath: map[string]string{"/application/o/token": "/application/o/token/"}},
			}}},
		},
		{
			name: "rewrite requires a spec",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rw"}, Type: MWTypeRewrite,
			}}},
			wantErr: "rewrite: requires at least one replacePath",
		},
		{
			name: "rewrite rejects empty replacePath",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rw"}, Type: MWTypeRewrite,
				Rewrite: &RewriteMiddleware{ReplacePath: map[string]string{}},
			}}},
			wantErr: "rewrite: requires at least one replacePath",
		},
		{
			name: "rewrite rejects non-absolute key",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rw"}, Type: MWTypeRewrite,
				Rewrite: &RewriteMiddleware{ReplacePath: map[string]string{"token": "/token/"}},
			}}},
			wantErr: "must be an absolute path",
		},
		{
			name: "rewrite rejects non-absolute value",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rw"}, Type: MWTypeRewrite,
				Rewrite: &RewriteMiddleware{ReplacePath: map[string]string{"/token": "token/"}},
			}}},
			wantErr: "must be an absolute path",
		},
		{
			name: "rewrite rejects no-op key==value",
			cfg: Config{Middlewares: []Middleware{{
				ObjectMeta: ObjectMeta{Name: "rw"}, Type: MWTypeRewrite,
				Rewrite: &RewriteMiddleware{ReplacePath: map[string]string{"/same": "/same"}},
			}}},
			wantErr: "no-op",
		},
		{
			name: "duplicate cert domain rejected",
			cfg: Config{Certificates: []Certificate{
				{ObjectMeta: ObjectMeta{Name: "a"}, Type: CertTypeCustom, Domains: []string{"*.example.com"}, Custom: &CustomCertSpec{CertFile: "a.pem", KeyFile: "ak.pem"}},
				{ObjectMeta: ObjectMeta{Name: "b"}, Type: CertTypeCustom, Domains: []string{"*.example.com"}, Custom: &CustomCertSpec{CertFile: "b.pem", KeyFile: "bk.pem"}},
			}},
			wantErr: `domains: *.example.com is already claimed by certificate`,
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

// A broken inline template used to commit cleanly and only surface on the NEXT
// restart, so it is refused at write time instead.
func TestSettingsErrorPagesInlineTemplateMustParse(t *testing.T) {
	base := func() Settings {
		return Settings{SchemaVersion: SchemaVersion, AdminAuth: AdminAuthSettings{LocalLoginEnabled: true}}
	}
	s := base()
	s.ErrorPages = ErrorPagesConfig{Inline: map[string]string{"502": "<h1>{{.Status}}</h1>"}}
	if err := s.Validate(); err != nil {
		t.Fatalf("a valid inline template must be accepted: %v", err)
	}
	s.ErrorPages = ErrorPagesConfig{Inline: map[string]string{"502": "<h1>{{.Status}</h1>"}}
	if err := s.Validate(); err == nil {
		t.Fatal("an unparseable inline error-page template must be rejected at write time")
	}
}

func TestSettingsTLSMinVersion(t *testing.T) {
	base := func() Settings {
		return Settings{SchemaVersion: SchemaVersion, AdminAuth: AdminAuthSettings{LocalLoginEnabled: true}}
	}
	for _, v := range []string{"", "1.2", "1.3"} {
		s := base()
		s.TLS.MinVersion = v
		if err := s.Validate(); err != nil {
			t.Errorf("settings.tls.minVersion %q should be valid, got %v", v, err)
		}
	}
	s := base()
	s.TLS.MinVersion = "1.1"
	if err := s.Validate(); err == nil {
		t.Fatal("settings.tls.minVersion 1.1 must be rejected")
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

func TestCompressionValidate(t *testing.T) {
	// Disabled is always valid, whatever else is set - it is inert.
	disabled := proxyHost("app", func(h *ProxyHost) {
		h.Compression = Compression{Enabled: false, MinBytes: -1}
	})
	if err := disabled.Validate(); err != nil {
		t.Fatalf("disabled compression should be valid regardless of other fields, got %v", err)
	}
	negative := proxyHost("app", func(h *ProxyHost) {
		h.Compression = Compression{Enabled: true, MinBytes: -1}
	})
	if err := negative.Validate(); err == nil {
		t.Fatal("negative minBytes must be rejected when enabled")
	}
	emptyType := proxyHost("app", func(h *ProxyHost) {
		h.Compression = Compression{Enabled: true, Types: []string{"text/html", ""}}
	})
	if err := emptyType.Validate(); err == nil {
		t.Fatal("an empty compression.types entry must be rejected")
	}
	ok := proxyHost("app", func(h *ProxyHost) {
		h.Compression = Compression{Enabled: true, MinBytes: 2048, Types: []string{"text/html"}}
	})
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid compression should pass, got %v", err)
	}
}

func TestCompressionEffectiveDefaults(t *testing.T) {
	var c Compression
	if got := c.EffectiveMinBytes(); got != DefaultCompressionMinBytes {
		t.Fatalf("EffectiveMinBytes() = %d, want default %d", got, DefaultCompressionMinBytes)
	}
	if got := c.EffectiveTypes(); len(got) == 0 {
		t.Fatal("EffectiveTypes() must return the default list when unset")
	}
	c = Compression{MinBytes: 5000, Types: []string{"application/x-custom"}}
	if got := c.EffectiveMinBytes(); got != 5000 {
		t.Fatalf("EffectiveMinBytes() = %d, want 5000", got)
	}
	if got := c.EffectiveTypes(); len(got) != 1 || got[0] != "application/x-custom" {
		t.Fatalf("EffectiveTypes() = %v, want the configured override", got)
	}
}

func TestErrorPagesConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		e       ErrorPagesConfig
		wantErr string
	}{
		{name: "empty is valid"},
		{name: "relative dir is valid", e: ErrorPagesConfig{Dir: "errors"}},
		{name: "absolute dir is rejected", e: ErrorPagesConfig{Dir: "/etc/passwd"}, wantErr: "relative"},
		{name: "traversal dir is rejected", e: ErrorPagesConfig{Dir: "../secrets"}, wantErr: "relative"},
		{name: "inline default key is valid", e: ErrorPagesConfig{Inline: map[string]string{"default": "<p>x</p>"}}},
		{name: "inline status key is valid", e: ErrorPagesConfig{Inline: map[string]string{"502": "<p>x</p>"}}},
		{name: "inline template at the size limit is valid", e: ErrorPagesConfig{Inline: map[string]string{"default": strings.Repeat("x", MaxInlineErrorPageBytes)}}},
		{name: "inline template over the size limit is rejected", e: ErrorPagesConfig{Inline: map[string]string{"default": strings.Repeat("x", MaxInlineErrorPageBytes+1)}}, wantErr: "byte limit"},
		{name: "inline non-numeric key is rejected", e: ErrorPagesConfig{Inline: map[string]string{"oops": "<p>x</p>"}}, wantErr: "status code"},
		{name: "interceptUpstream 4xx/5xx is valid", e: ErrorPagesConfig{InterceptUpstream: []int{404, 502}}},
		{name: "interceptUpstream out of range is rejected", e: ErrorPagesConfig{InterceptUpstream: []int{200}}, wantErr: "4xx/5xx"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.e.validate()
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected success, got: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// An error-page template outside the allow-list is refused when it is new or
// changed, and accepted when the stored block holds the same source. Validate
// (which also runs at load) does not refuse it. Values built inside a pipeline
// (a variable doubled by assignment, nested calls, a call with many arguments,
// the whole page data as dot) are refused as well as loops, because they never
// reach the size-capped output writer.
func TestErrorPagesCheckNewTemplates(t *testing.T) {
	var doubled strings.Builder
	doubled.WriteString(`{{$a := printf "%1000000s" ""}}`)
	for i := 0; i < 8; i++ {
		doubled.WriteString(`{{$a = printf "%s%s" $a $a}}`)
	}
	doubled.WriteString(`{{len $a}}`)
	nested := `{{len (print` + strings.Repeat(` (printf "%1000000s" "")`, 300) + `)}}`
	refused := []string{
		doubled.String(),
		nested,
		// Many arguments to one call, and dot as the whole page data.
		`{{print` + strings.Repeat(" .", 2000) + `}}`,
		`{{printf "` + strings.Repeat("%9999v", 10) + `"` + strings.Repeat(" .", 10) + `}}`,
		`{{urlquery` + strings.Repeat(" .Host", 2000) + `}}`,
		`{{.}}`,
		`{{if .}}x{{end}}`,
		`{{if .Status}}{{.}}{{end}}`,
		`{{with .Host}}x{{else}}{{.}}{{end}}`,
		`{{with eq .Status 502}}{{.}}{{end}}`,
		`{{print .Status .Host}}`,
		`{{html .Host .Host}}`,
		`{{len}}`,
		`{{.Host | print .Host}}`,
		`{{printf "%s%s%s%s%s" .Host .Host .Host .Host .Host}}`,
		`{{.Host | printf "%s%s%s%s%s" .Host .Host .Host .Host}}`,
		`{{printf "%1000s" .Host}}`,
		`{{printf "%.1000s" .Host}}`,
		`{{printf "` + strings.Repeat("x", 65) + `"}}`,
		`{{eq .Host .Host .Host .Host .Host}}`,
		`{{if eq .Host "` + strings.Repeat("x", 65) + `"}}x{{end}}`,
		strings.Repeat(`{{.Host}}`, 129),
		"{{range 10}}x{{end}}",
		"{{$n := 9}}{{range $n}}x{{end}}",
		"{{if .Status}}{{with .Host}}{{range 3}}x{{end}}{{end}}{{end}}",
		"{{if .Status}}a{{else}}{{range 3}}x{{end}}{{end}}",
		`{{define "a"}}{{template "a"}}{{end}}{{template "a"}}`,
		`{{block "b" .}}x{{end}}`,
		`{{define "c"}}{{range 2}}x{{end}}{{end}}ok`,
		`{{define "d"}}x{{end}}ok`,
		`{{$x := .Host}}{{$x}}`,
		`{{$}}`,
		`{{(.Host)}}`,
		`{{html (print .Host)}}`,
		`{{.Host | html | js}}`,
		`{{.Host | printf "%s" | html}}`,
		`{{index .Host 0}}`,
		`{{slice .Host 1}}`,
		`{{call .Host}}`,
		`{{.Missing}}`,
		`{{.Host.Len}}`,
		`{{"literal"}}`,
		`{{print "a"}}`,
		`{{printf .Host}}`,
		`{{printf "%1000000s" .Host}}`,
		`{{printf "%.99999s" .Host}}`,
		`{{printf "%*d" .Status .Status}}`,
		`{{printf "%[1]s" .Host}}`,
		`{{if printf "%s" .Host}}x{{end}}`,
		`{{if len .Host}}x{{end}}`,
		`{{if eq (len .Host) 3}}x{{end}}`,
		`{{if or (lt .Status 500) (ge .Status 599)}}x{{end}}`,
		`{{with $h := .Host}}{{$h}}{{end}}`,
	}
	for _, src := range refused {
		e := ErrorPagesConfig{Inline: map[string]string{"502": src}}
		if err := e.validate(); err != nil {
			t.Fatalf("%.80q: validate must not refuse it (a stored one has to keep loading): %v", src, err)
		}
		if err := e.CheckNewTemplates(nil); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("%.80q: new template: err = %v, want a refusal", src, err)
		}
		if err := e.CheckNewTemplates(&ErrorPagesConfig{Inline: map[string]string{"502": src}}); err != nil {
			t.Fatalf("%.80q: stored unchanged: %v", src, err)
		}
		if err := e.CheckNewTemplates(&ErrorPagesConfig{Inline: map[string]string{"502": src + " "}}); err == nil {
			t.Fatalf("%.80q: changed template must be refused", src)
		}
	}
	// The documented examples and the allowed constructs.
	allowed := []string{
		`<html><body><h1>Slow down</h1><p>{{.Host}} is rate-limiting you.</p></body></html>`,
		`{{.Status}} {{.StatusText}} {{.Host}} {{.RequestID}}`,
		`<h1>{{.Status}} {{.StatusText}}</h1>{{if .RequestID}}<p>{{.RequestID}}</p>{{else}}-{{end}}{{with .Host}}{{.}}{{end}}`,
		`{{if eq .Status 502 504}}down{{else if and .RequestID .Host}}x{{else if ne .Status 404}}y{{end}}`,
		`{{if not .RequestID}}-{{end}}{{if lt .Status 500}}a{{end}}{{if ge .Status 1}}{{end}}{{if gt .Status 1}}{{end}}{{if le .Status 1}}{{end}}{{if or .RequestID true}}{{end}}`,
		`{{with .Host}}{{.}}{{else with .RequestID}}{{.}}{{end}}`,
		`{{or .RequestID "none"}}`,
		`{{html .Host}}{{js .Host}}{{urlquery .Host}}{{len .Host}}{{print .Status}}`,
		`{{.Host | html}}{{.Host | urlquery}}{{.Status | printf "%03d"}}`,
		`{{printf "%s (%d %s)" .Host .Status .StatusText}}{{printf "%-999.999s|%%" .Host}}{{printf "plain"}}`,
		`{{.Host | printf "%s %s %s %s" .Host .StatusText .RequestID}}`,
		`{{with .Host}}{{print .}}{{html .}}{{printf "%s" .}}{{. | js}}{{if eq . "x"}}{{.}}{{end}}{{end}}`,
		`{{if eq .Host "` + strings.Repeat("x", 64) + `"}}x{{end}}{{eq .Status 1 2 3}}`,
		`{{if .RequestID}}<p>request {{.RequestID}}</p>{{end}}`,
		strings.Repeat(`{{.Host}}`, 64) + `{{if .Status}}` + strings.Repeat(`{{.Status}}`, 64) + `{{end}}`,
		`{{/* a comment */}}{{- .Host -}}`,
	}
	for _, src := range allowed {
		e := ErrorPagesConfig{Inline: map[string]string{"default": src}}
		if err := e.CheckNewTemplates(nil); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
	}
}

func TestProxyHostErrorPagesOverrideValidate(t *testing.T) {
	bad := proxyHost("app", func(h *ProxyHost) {
		h.ErrorPages = &ErrorPagesConfig{Dir: "/etc/passwd"}
	})
	if err := bad.Validate(); err == nil {
		t.Fatal("proxy host errorPages override must be validated")
	}
	good := proxyHost("app", func(h *ProxyHost) {
		h.ErrorPages = &ErrorPagesConfig{Inline: map[string]string{"502": "<p>x</p>"}}
	})
	if err := good.Validate(); err != nil {
		t.Fatalf("valid errorPages override should pass, got %v", err)
	}
}

func TestSettingsErrorPagesValidate(t *testing.T) {
	s := DefaultSettings()
	s.ErrorPages = ErrorPagesConfig{Dir: "../escape"}
	if err := s.Validate(); err == nil {
		t.Fatal("settings.errorPages must be validated")
	}
	s.ErrorPages = ErrorPagesConfig{Inline: map[string]string{"default": "<p>x</p>"}}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid settings.errorPages should pass, got %v", err)
	}
}

func TestValidateSecurityHeaders(t *testing.T) {
	sh := func(v string) SecurityHeaderValue { return SecurityHeaderValue{Value: v} }
	tests := []struct {
		name    string
		m       map[string]SecurityHeaderValue
		wantErr string
	}{
		{name: "empty is valid"},
		{name: "nil is valid", m: nil},
		{name: "recommended set is valid", m: RecommendedSecurityHeaders},
		{name: "simple set is valid", m: map[string]SecurityHeaderValue{"X-Content-Type-Options": sh("nosniff")}},
		{name: "value with tab is valid", m: map[string]SecurityHeaderValue{"X-Foo": sh("a\tb")}},
		{name: "explicit all scope is valid", m: map[string]SecurityHeaderValue{"X-Foo": {Value: "1", Scope: SecurityScopeAll}}},
		{name: "generated-only scope is valid", m: map[string]SecurityHeaderValue{"X-Foo": {Value: "1", Scope: SecurityScopeGenerated}}},
		{name: "proxied-only scope is valid", m: map[string]SecurityHeaderValue{"X-Foo": {Value: "1", Scope: SecurityScopeProxied}}},
		{name: "unknown scope rejected", m: map[string]SecurityHeaderValue{"X-Foo": {Value: "1", Scope: "everywhere"}}, wantErr: "unknown scope"},
		{name: "invalid name (space)", m: map[string]SecurityHeaderValue{"X Frame": sh("DENY")}, wantErr: "valid header name"},
		{name: "invalid name (colon)", m: map[string]SecurityHeaderValue{"X:Frame": sh("DENY")}, wantErr: "valid header name"},
		{name: "CRLF in value rejected", m: map[string]SecurityHeaderValue{"X-Foo": sh("bar\r\nSet-Cookie: x=1")}, wantErr: "invalid character"},
		{name: "bare LF in value rejected", m: map[string]SecurityHeaderValue{"X-Foo": sh("bar\nbaz")}, wantErr: "invalid character"},
		{name: "HSTS key rejected", m: map[string]SecurityHeaderValue{"Strict-Transport-Security": sh("max-age=1")}, wantErr: "hsts"},
		{name: "HSTS key rejected case-insensitively", m: map[string]SecurityHeaderValue{"strict-transport-security": sh("max-age=1")}, wantErr: "hsts"},
		{name: "hop-by-hop Connection rejected", m: map[string]SecurityHeaderValue{"Connection": sh("close")}, wantErr: "hop-by-hop"},
		{name: "hop-by-hop Transfer-Encoding rejected", m: map[string]SecurityHeaderValue{"transfer-encoding": sh("chunked")}, wantErr: "hop-by-hop"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSecurityHeaders(tc.m)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected success, got: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantErr)) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// Case-insensitive de-duplication cannot be expressed with a Go map literal (two
// keys differing only in case collapse to one), so it is checked on its own. It
// doubles as the duplicate-name-across-scopes guard: expressing the same header
// at two scopes requires two case-variant keys, which this rejects - so a header
// is declared exactly once, at one scope.
func TestValidateSecurityHeadersRejectsCaseInsensitiveDuplicates(t *testing.T) {
	err := validateSecurityHeaders(map[string]SecurityHeaderValue{
		"X-Frame-Options": {Value: "DENY", Scope: SecurityScopeGenerated},
		"x-frame-options": {Value: "SAMEORIGIN", Scope: SecurityScopeProxied},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected a case-insensitive duplicate error, got: %v", err)
	}
}

func TestSettingsSecurityHeadersValidate(t *testing.T) {
	s := DefaultSettings()
	s.SecurityHeaders = map[string]SecurityHeaderValue{"Strict-Transport-Security": {Value: "max-age=1"}}
	if err := s.Validate(); err == nil {
		t.Fatal("settings.securityHeaders must reject the HSTS key")
	}
	s.SecurityHeaders = RecommendedSecurityHeaders
	if err := s.Validate(); err != nil {
		t.Fatalf("recommended settings.securityHeaders should pass, got %v", err)
	}
}

func TestProxyHostSecurityHeadersValidate(t *testing.T) {
	bad := proxyHost("app", func(h *ProxyHost) {
		h.SecurityHeaders = map[string]SecurityHeaderValue{"X-Foo": {Value: "bad\r\nInjected: 1"}}
	})
	if err := bad.Validate(); err == nil {
		t.Fatal("proxy host securityHeaders override must be validated")
	}
	good := proxyHost("app", func(h *ProxyHost) {
		h.SecurityHeaders = map[string]SecurityHeaderValue{"X-Frame-Options": {Value: "DENY", Scope: SecurityScopeGenerated}}
	})
	if err := good.Validate(); err != nil {
		t.Fatalf("valid securityHeaders override should pass, got %v", err)
	}
}

func TestValidateStripResponseHeaders(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		wantErr string
	}{
		{name: "empty is valid"},
		{name: "nil is valid", in: nil},
		{name: "typical leaked headers", in: []string{"Server", "X-Powered-By", "X-AspNet-Version"}},
		{name: "lower case is valid", in: []string{"x-powered-by"}},
		{name: "empty name rejected", in: []string{""}, wantErr: "valid header name"},
		{name: "invalid name (space)", in: []string{"X Powered By"}, wantErr: "valid header name"},
		{name: "invalid name (colon)", in: []string{"X-Powered-By:"}, wantErr: "valid header name"},
		{name: "CRLF in name rejected", in: []string{"X-Foo\r\nInjected"}, wantErr: "valid header name"},
		{name: "case-insensitive duplicate rejected", in: []string{"Server", "SERVER"}, wantErr: "duplicate"},
		// Every hop-by-hop name is refused: they carry the response's own
		// connection semantics, so removing one breaks the response.
		{name: "hop-by-hop Connection rejected", in: []string{"Connection"}, wantErr: "hop-by-hop"},
		{name: "hop-by-hop Keep-Alive rejected", in: []string{"Keep-Alive"}, wantErr: "hop-by-hop"},
		{name: "hop-by-hop Proxy-Authenticate rejected", in: []string{"Proxy-Authenticate"}, wantErr: "hop-by-hop"},
		{name: "hop-by-hop Proxy-Authorization rejected", in: []string{"Proxy-Authorization"}, wantErr: "hop-by-hop"},
		{name: "hop-by-hop TE rejected", in: []string{"TE"}, wantErr: "hop-by-hop"},
		{name: "hop-by-hop Trailer rejected", in: []string{"Trailer"}, wantErr: "hop-by-hop"},
		{name: "hop-by-hop Transfer-Encoding rejected", in: []string{"transfer-encoding"}, wantErr: "hop-by-hop"},
		{name: "hop-by-hop Upgrade rejected", in: []string{"Upgrade"}, wantErr: "hop-by-hop"},
		// Response-semantic headers are refused too. Content-Type is the sharpest:
		// stripping it hands the body to Go's content sniffing, which can re-label
		// a JSON/text response as text/html - stored XSS from a config typo.
		{name: "Content-Type rejected", in: []string{"Content-Type"}, wantErr: "own semantics"},
		{name: "Content-Length rejected", in: []string{"content-length"}, wantErr: "own semantics"},
		{name: "Content-Encoding rejected", in: []string{"Content-Encoding"}, wantErr: "own semantics"},
		{name: "Vary rejected", in: []string{"Vary"}, wantErr: "own semantics"},
		{name: "Location rejected", in: []string{"location"}, wantErr: "own semantics"},
		// 101 responses are in scope for stripping, so the handshake's own
		// headers are refused: without Sec-WebSocket-Accept every browser aborts
		// the connection. Spelled here as a client sends it (capital S in
		// "WebSocket"), which the case-insensitive check must still catch.
		{name: "Sec-WebSocket-Accept rejected", in: []string{"Sec-WebSocket-Accept"}, wantErr: "own semantics"},
		{name: "Sec-WebSocket-Protocol rejected", in: []string{"Sec-WebSocket-Protocol"}, wantErr: "own semantics"},
		{name: "Sec-WebSocket-Extensions rejected", in: []string{"sec-websocket-extensions"}, wantErr: "own semantics"},
		// Deliberately allowed. Stripping only ever reaches what the UPSTREAM
		// sent, so these remove a backend's value and never one gpm added; they
		// are sharp edges, documented as such, not config errors.
		{name: "Set-Cookie is allowed", in: []string{"Set-Cookie"}},
		{name: "WWW-Authenticate is allowed", in: []string{"WWW-Authenticate"}},
		{name: "HSTS is allowed", in: []string{"Strict-Transport-Security"}},
		{name: "a security header is allowed", in: []string{"X-Frame-Options"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateStripResponseHeaders(tc.in)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected success, got: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantErr)) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestSettingsStripResponseHeadersValidate(t *testing.T) {
	s := DefaultSettings()
	s.StripResponseHeaders = []string{"X Powered By"}
	if err := s.Validate(); err == nil {
		t.Fatal("settings.stripResponseHeaders must reject an invalid header name")
	}
	s.StripResponseHeaders = []string{"Server", "X-Powered-By"}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid settings.stripResponseHeaders should pass, got %v", err)
	}
}

func TestProxyHostStripResponseHeadersValidate(t *testing.T) {
	bad := proxyHost("app", func(h *ProxyHost) {
		h.StripResponseHeaders = []string{"Content-Type"}
	})
	if err := bad.Validate(); err == nil {
		t.Fatal("proxy host stripResponseHeaders must be validated")
	}
	good := proxyHost("app", func(h *ProxyHost) {
		h.StripResponseHeaders = []string{"x-powered-by"}
	})
	if err := good.Validate(); err != nil {
		t.Fatalf("valid stripResponseHeaders should pass, got %v", err)
	}
}

// Two enabled hosts claiming the same domain is a load-time error, whatever
// wrote them. The data plane keys its per-domain maps by hostname and fills them
// in config load order, so a duplicate is resolved by YAML filename rather than
// by intent - which is how an automated writer (Ingress discovery) could
// otherwise shadow an operator-authored host without ever colliding on a name.
func TestConfigValidateDuplicateDomainsAcrossHosts(t *testing.T) {
	redirect := func(name, domain string) RedirectHost {
		return RedirectHost{
			ObjectMeta:   ObjectMeta{Name: name},
			Domains:      []string{domain},
			TargetDomain: "elsewhere.example.com",
		}
	}
	parked := func(name, domain string) ParkedHost {
		return ParkedHost{ObjectMeta: ObjectMeta{Name: name}, Domains: []string{domain}}
	}
	withDomains := func(name string, domains ...string) ProxyHost {
		return proxyHost(name, func(h *ProxyHost) { h.Domains = domains })
	}

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "two proxy hosts",
			cfg:     Config{ProxyHosts: []ProxyHost{withDomains("sso", "sso.example.com"), withDomains("ing-grab.tenant", "sso.example.com")}},
			wantErr: `domains: sso.example.com is already claimed by `,
		},
		{
			name:    "case and trailing whitespace do not evade the check",
			cfg:     Config{ProxyHosts: []ProxyHost{withDomains("a", "SSO.Example.com"), withDomains("b", " sso.example.com ")}},
			wantErr: `domains: sso.example.com is already claimed by `,
		},
		{
			name: "proxy host and redirect host",
			cfg: Config{
				ProxyHosts:    []ProxyHost{withDomains("app", "app.example.com")},
				RedirectHosts: []RedirectHost{redirect("old-app", "app.example.com")},
			},
			wantErr: `domains: app.example.com is already claimed by `,
		},
		{
			name: "proxy host and parked host",
			cfg: Config{
				ProxyHosts:  []ProxyHost{withDomains("app", "app.example.com")},
				ParkedHosts: []ParkedHost{parked("absorb", "app.example.com")},
			},
			wantErr: `domains: app.example.com is already claimed by `,
		},
		{
			name: "one domain of several collides",
			cfg: Config{ProxyHosts: []ProxyHost{
				withDomains("a", "a.example.com", "shared.example.com"),
				withDomains("b", "b.example.com", "shared.example.com"),
			}},
			wantErr: `domains: shared.example.com is already claimed by `,
		},
		// A disabled host is excluded from the compiled data plane entirely, so it
		// cannot shadow anything - and staging a replacement beside the live host is
		// a legitimate workflow that a strict global check would break.
		{
			name: "a disabled host may share a domain with the live one",
			cfg: Config{ProxyHosts: []ProxyHost{
				withDomains("app", "app.example.com"),
				proxyHost("app-next", func(h *ProxyHost) {
					h.Domains = []string{"app.example.com"}
					h.Disabled = true
				}),
			}},
		},
		{
			name: "two disabled hosts may share a domain",
			cfg: Config{ProxyHosts: []ProxyHost{
				proxyHost("a", func(h *ProxyHost) { h.Domains = []string{"x.example.com"}; h.Disabled = true }),
				proxyHost("b", func(h *ProxyHost) { h.Domains = []string{"x.example.com"}; h.Disabled = true }),
			}},
		},
		{
			name: "distinct domains are fine",
			cfg: Config{
				ProxyHosts:    []ProxyHost{withDomains("a", "a.example.com")},
				RedirectHosts: []RedirectHost{redirect("b", "b.example.com")},
				ParkedHosts:   []ParkedHost{parked("c", "c.example.com")},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected a validation error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// settings.clientLimits is optional: absent or zero sets no limit, a negative
// or oversized maxInFlight and a malformed allowFrom entry are refused.
func TestSettingsClientLimitsValidate(t *testing.T) {
	base := DefaultSettings()
	for _, tc := range []struct {
		name    string
		cl      *ClientLimitsSettings
		wantErr string
	}{
		{name: "absent", cl: nil},
		{name: "zero", cl: &ClientLimitsSettings{}},
		{name: "limit with exemptions", cl: &ClientLimitsSettings{MaxInFlight: 256, AllowFrom: []string{"192.0.2.0/24", "2001:db8::1"}}},
		{name: "negative", cl: &ClientLimitsSettings{MaxInFlight: -1}, wantErr: "clientLimits.maxInFlight"},
		{name: "too large", cl: &ClientLimitsSettings{MaxInFlight: MaxClientInFlightLimit + 1}, wantErr: "clientLimits.maxInFlight"},
		{name: "bad allowFrom", cl: &ClientLimitsSettings{MaxInFlight: 10, AllowFrom: []string{"lan"}}, wantErr: "clientLimits.allowFrom"},
	} {
		s := base
		s.ClientLimits = tc.cl
		err := s.Validate()
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: error = %v, want one mentioning %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestIsDomainName(t *testing.T) {
	tests := []struct {
		d        string
		wildcard bool
		want     bool
	}{
		{"app.example.com", false, true},
		{"App.Example.COM", false, true},
		{" app.example.com ", false, true},
		{"localhost", false, true},
		{"192.0.2.10", false, true},
		{"app.example.com:8443", false, false},
		{"app.example.com.", false, false},
		{"app..example.com", false, false},
		{"-app.example.com", false, false},
		{"app_1.example.com", false, false},
		{"app.example.com/path", false, false},
		{"", false, false},
		{"*.example.com", false, false},
		{"*.example.com", true, true},
		{"*.*.example.com", true, false},
		{"a.*.example.com", true, false},
		{strings.Repeat("a", 64) + ".example.com", false, false},
	}
	for _, tc := range tests {
		if got := IsDomainName(tc.d, tc.wildcard); got != tc.want {
			t.Errorf("IsDomainName(%q, %v) = %v, want %v", tc.d, tc.wildcard, got, tc.want)
		}
	}
}

// TestInvalidDomainsWarnedAndMayNotGrow proves a stored invalid domain is a
// config warning, not a Validate error, and that only an added invalid domain
// is reported as an error for a write.
func TestInvalidDomainsWarnedAndMayNotGrow(t *testing.T) {
	cfg := Config{
		ProxyHosts: []ProxyHost{proxyHost("a", func(h *ProxyHost) { h.Domains = []string{"a.example.com:8443"} })},
		Certificates: []Certificate{{ObjectMeta: ObjectMeta{Name: "c"}, Type: CertTypeCustom,
			Domains: []string{"*.example.com", "odd.example.com."}, Custom: &CustomCertSpec{CertFile: "c.pem", KeyFile: "c.key"}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("an invalid stored domain must not fail Validate: %v", err)
	}
	var warned []string
	for _, w := range cfg.Warnings() {
		if strings.Contains(w, "must be a hostname") {
			warned = append(warned, w)
		}
	}
	if len(warned) != 2 || !strings.Contains(warned[0], `certificate "c": domains: "odd.example.com."`) || !strings.Contains(warned[1], `proxy host "a": domains: "a.example.com:8443"`) {
		t.Fatalf("warnings = %v, want one per object naming the invalid domain only", warned)
	}
	before := cfg.InvalidDomains()
	if err := InvalidDomainsError(before, cfg.InvalidDomains()); err != nil {
		t.Fatalf("an unchanged invalid domain must pass: %v", err)
	}
	cfg.ProxyHosts[0].Domains = []string{"A.example.com:8443", "b.example.com"}
	if err := InvalidDomainsError(before, cfg.InvalidDomains()); err != nil {
		t.Fatalf("keeping the stored invalid domain (any case) must pass: %v", err)
	}
	cfg.ProxyHosts[0].Domains = append(cfg.ProxyHosts[0].Domains, "b.example.com.")
	if err := InvalidDomainsError(before, cfg.InvalidDomains()); err == nil || !strings.Contains(err.Error(), `"b.example.com."`) {
		t.Fatalf("an added invalid domain must be refused and named: %v", err)
	}
	cfg.ProxyHosts = append(cfg.ProxyHosts[:0], proxyHost("x", func(h *ProxyHost) { h.Domains = []string{"a.example.com:8443"} }))
	if err := InvalidDomainsError(before, cfg.InvalidDomains()); err == nil || !strings.Contains(err.Error(), `proxy host "x"`) {
		t.Fatalf("another object's stored invalid domain does not license a new object: %v", err)
	}
}

func TestMinTLS13WithoutForceSSLIsWarned(t *testing.T) {
	for _, force := range []bool{false, true} {
		cfg := Config{ProxyHosts: []ProxyHost{proxyHost("a", func(h *ProxyHost) {
			h.TLS.MinTLSVersion = "1.3"
			h.TLS.ForceSSL = force
		})}}
		warned := false
		for _, w := range cfg.Warnings() {
			if strings.Contains(w, `proxy host "a": tls.minTLSVersion is "1.3" but tls.forceSSL is off`) {
				warned = true
			}
		}
		if warned == force {
			t.Errorf("forceSSL %v: warned = %v, want %v", force, warned, !force)
		}
	}
}

// checkErrorPageSrc parses src as an inline error page and runs the write check.
func checkErrorPageSrc(src string) error {
	return ErrorPagesConfig{Inline: map[string]string{"default": src}}.CheckNewTemplates(nil)
}

// More than 16 levels of if/with (an else if or else with is one level more)
// and more than 512 conditions are refused, so a render's stack and time stay
// small; a stored page that breaks either rule still validates.
func TestErrorPageTemplateDepthLimit(t *testing.T) {
	nested := func(n int) string {
		return `{{with .Host}}` + strings.Repeat(`{{if .}}`, n-1) + "x" + strings.Repeat(`{{end}}`, n-1) + `{{end}}`
	}
	elseIf := func(n int) string {
		return `{{if .Host}}a` + strings.Repeat(`{{else if not .Status}}b`, n-1) + `{{end}}`
	}
	if err := checkErrorPageSrc(nested(16)); err != nil {
		t.Fatalf("16 levels: %v", err)
	}
	if err := checkErrorPageSrc(elseIf(16)); err != nil {
		t.Fatalf("an else if chain of 16: %v", err)
	}
	for name, src := range map[string]string{
		"17 levels":             nested(17),
		"else if chain of 17":   elseIf(17),
		"16000 levels":          nested(16000),
		"else with chain of 17": `{{with .Host}}a` + strings.Repeat(`{{else with .Status}}b`, 16) + `{{end}}`,
		"513 conditions":        strings.Repeat(`{{if .Host}}x{{end}}`, 513),
	} {
		if err := checkErrorPageSrc(src); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("%s: err = %v, want refused", name, err)
		}
		if len(src) <= MaxInlineErrorPageBytes {
			if err := (ErrorPagesConfig{Inline: map[string]string{"default": src}}).validate(); err != nil {
				t.Fatalf("%s: validate must not refuse it: %v", name, err)
			}
		}
	}
	if err := checkErrorPageSrc(strings.Repeat(`{{if .Host}}x{{end}}`, 512)); err != nil {
		t.Fatalf("512 conditions: %v", err)
	}
}

// A template over 32 KiB is refused by the check; validation keeps its own
// 256 KiB limit, so a stored page between the two still loads.
func TestErrorPageTemplateSizeLimit(t *testing.T) {
	for name, src := range map[string]string{
		"256 KiB of <":            strings.Repeat("<", 256<<10),
		"text past 32 KiB":        strings.Repeat("a", MaxErrorPageTemplateBytes+1),
		"action past 32 KiB":      strings.Repeat("a", MaxErrorPageTemplateBytes-5) + `{{.Host}}`,
		"textarea of <a past cap": "<textarea>" + strings.Repeat("<a", 20<<10) + "</textarea>",
	} {
		if err := checkErrorPageSrc(src); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("%s: err = %v, want refused", name, err)
		}
		if err := (ErrorPagesConfig{Inline: map[string]string{"default": src}}).validate(); err != nil {
			t.Fatalf("%s: validate must not refuse it: %v", name, err)
		}
	}
	if err := checkErrorPageSrc(strings.Repeat("a", MaxErrorPageTemplateBytes)); err != nil {
		t.Fatalf("exactly 32 KiB: %v", err)
	}
}

// The '#' and ' ' printf flags are refused: they widen a verb past its width.
func TestErrorPageTemplatePrintfFlagRefusal(t *testing.T) {
	for _, src := range []string{
		`{{printf "% #x% #x% #x% #x" .Host .Host .Host .Host}}`,
		`{{printf "%#x" .Host}}`,
		`{{printf "% x" .Host}}`,
		`{{printf "%-#5v" .Status}}`,
	} {
		if err := checkErrorPageSrc(src); err == nil || !strings.Contains(err.Error(), "flag") {
			t.Fatalf("%q: err = %v, want the flag refused", src, err)
		}
	}
	if err := checkErrorPageSrc(`{{printf "%-5s|%+d|%05d|%x" .Host .Status .Status .Host}}`); err != nil {
		t.Fatalf("other flags: %v", err)
	}
}

// The page shapes operators write: static HTML with the four fields, each
// optionally inside an if on that field.
func TestErrorPageTemplateRealWorldShapes(t *testing.T) {
	for _, src := range []string{
		`<!doctype html><html><head><title>{{.Status}} {{.StatusText}}</title><style>body{font-family:sans-serif}</style></head>` +
			`<body><h1>{{.Status}} {{.StatusText}}</h1><p>{{.Host}} could not answer.</p><p>Request {{.RequestID}}</p></body></html>`,
		`<!doctype html><html><body>{{if .Status}}<h1>{{.Status}}</h1>{{end}}{{if .StatusText}}<p>{{.StatusText}}</p>{{end}}` +
			`{{if .Host}}<p>{{.Host}}</p>{{end}}{{if .RequestID}}<p>Request {{.RequestID}}</p>{{end}}</body></html>`,
	} {
		if err := checkErrorPageSrc(src); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
	}
}
