package model

import (
	"strings"
	"testing"
)

// inlineIdPs is the provider set the inline-block tests validate against: one of
// each type, so an inline auth block can name a resolvable provider of the shape
// each mode needs.
func inlineIdPs() []IdentityProvider {
	return []IdentityProvider{
		{
			ObjectMeta: ObjectMeta{Name: "authentik"}, Type: IdPTypeAuthRequest,
			AuthRequest: &AuthRequestSpec{OutpostURL: "http://auth-outpost:9000"},
		},
		{
			ObjectMeta: ObjectMeta{Name: "sso"}, Type: IdPTypeOIDC,
			OIDC: &OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
		},
	}
}

// TestInlineAuthAndRateLimitValidate covers the host- and location-level inline
// auth / rateLimit blocks. They carry the middleware shapes verbatim, so the
// point of the table is that every rule a `type: auth` / `type: rate-limit`
// middleware is held to fires identically here, that the identity provider must
// still resolve, and that an inline block and a referenced middleware of the
// same kind may coexist (no mutual exclusivity).
func TestInlineAuthAndRateLimitValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string // substring; empty means expect success
	}{
		{
			name: "inline auth with a resolvable provider",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "sso", Mode: AuthModeOIDC}
				})},
			},
		},
		{
			name: "inline auth naming a provider that does not exist",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "ghost", Mode: AuthModeOIDC}
				})},
			},
			wantErr: `proxy host "app" references unknown identityProvider "ghost"`,
		},
		{
			name: "inline auth without a provider",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.Auth = &AuthMiddleware{Mode: AuthModeOIDC}
			})}},
			wantErr: `proxy host "app": auth.identityProvider is required`,
		},
		{
			name: "inline auth with an unknown mode",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "sso", Mode: "magic"}
				})},
			},
			wantErr: `proxy host "app": auth.mode must be oidc|forward-auth|auth-request|client-cert`,
		},
		{
			name: "inline auth allowFrom is refused in oidc mode",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "sso", Mode: AuthModeOIDC, AllowFrom: []string{"10.0.0.0/8"}}
				})},
			},
			wantErr: `proxy host "app": auth.allowFrom is only supported in auth-request, client-cert and basic modes`,
		},
		{
			name: "inline auth allowFrom is accepted in auth-request mode",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "authentik", Mode: AuthModeAuthRequest, AllowFrom: []string{"10.0.0.0/8"}}
				})},
			},
		},
		{
			name: "inline auth allowFrom with an invalid CIDR",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "authentik", Mode: AuthModeAuthRequest, AllowFrom: []string{"10.0.0.0/8", "nope"}}
				})},
			},
			wantErr: `proxy host "app": auth.allowFrom has invalid CIDR/IP "nope"`,
		},
		{
			// The mode defaults from the provider type, which only the whole
			// config knows - the same gap checkAuthAllowFromMode closes for a
			// middleware must be closed for an inline block.
			name: "inline auth allowFrom with no mode against an oidc provider",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "sso", AllowFrom: []string{"10.0.0.0/8"}}
				})},
			},
			wantErr: `proxy host "app": auth.allowFrom is only supported in auth-request and client-cert modes; auth.mode is unset`,
		},
		{
			name: "inline client-cert auth needs no provider",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.Auth = &AuthMiddleware{Mode: AuthModeClientCert}
			})}},
		},
		{
			name: "inline client-cert auth refuses a provider",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{Mode: AuthModeClientCert, IdentityProvider: "sso"}
				})},
			},
			wantErr: `proxy host "app": auth.identityProvider is not used in client-cert mode`,
		},
		{
			name: "inline client-cert requiredRoles without a subject mapping",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.Auth = &AuthMiddleware{Mode: AuthModeClientCert, RequiredRoles: []string{"admin"}}
			})}},
			wantErr: `proxy host "app": auth.requiredRoles in client-cert mode needs auth.clientCertRoles`,
		},
		{
			name: "inline auth-request refuses requiredRoles",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "authentik", Mode: AuthModeAuthRequest, RequiredRoles: []string{"admin"}}
				})},
			},
			wantErr: `proxy host "app": auth.requiredRoles is not supported in auth-request mode`,
		},
		{
			name: "inline clientCertRoles outside client-cert mode",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Auth = &AuthMiddleware{IdentityProvider: "sso", Mode: AuthModeOIDC, ClientCertRoles: map[string]string{"CN=ops": "admin"}}
				})},
			},
			wantErr: `proxy host "app": auth.clientCertRoles is only used in client-cert mode`,
		},

		{
			name: "inline rateLimit with requestsPerSecond",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.RateLimit = &RateLimitMiddleware{RequestsPerSecond: 10, Burst: 20}
			})}},
		},
		{
			name: "inline rateLimit with requests+window",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.RateLimit = &RateLimitMiddleware{Requests: 100, Window: "1m"}
			})}},
		},
		{
			name: "inline rateLimit with neither form",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.RateLimit = &RateLimitMiddleware{Burst: 5}
			})}},
			wantErr: `proxy host "app": rateLimit.requestsPerSecond must be > 0 (or set requests+window)`,
		},
		{
			name: "inline rateLimit with both forms",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.RateLimit = &RateLimitMiddleware{RequestsPerSecond: 1, Requests: 100, Window: "1m"}
			})}},
			wantErr: `proxy host "app": rateLimit must set either requestsPerSecond or requests+window, not both`,
		},
		{
			name: "inline rateLimit with an unparseable window",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.RateLimit = &RateLimitMiddleware{Requests: 100, Window: "1 minute"}
			})}},
			wantErr: `proxy host "app": rateLimit.window must be a valid duration`,
		},
		{
			name: "inline rateLimit with an invalid allowFrom",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.RateLimit = &RateLimitMiddleware{RequestsPerSecond: 1, AllowFrom: []string{"lan"}}
			})}},
			wantErr: `proxy host "app": rateLimit.allowFrom has invalid CIDR/IP "lan"`,
		},
		{
			name: "inline rateLimit with an invalid blockFor",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.RateLimit = &RateLimitMiddleware{RequestsPerSecond: 1, BlockFor: "-1s"}
			})}},
			wantErr: `proxy host "app": rateLimit.blockFor must be > 0`,
		},

		{
			name: "location inline auth with a resolvable provider",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Locations = []Location{{
						Path: "/admin",
						Auth: &AuthMiddleware{IdentityProvider: "sso", Mode: AuthModeOIDC, RequiredRoles: []string{"admin"}},
					}}
				})},
			},
		},
		{
			name: "location inline auth naming a provider that does not exist",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.Locations = []Location{{Path: "/admin", Auth: &AuthMiddleware{IdentityProvider: "ghost", Mode: AuthModeOIDC}}}
			})}},
			wantErr: `proxy host "app location /admin" references unknown identityProvider "ghost"`,
		},
		{
			name: "location inline auth with a bad mode",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.Locations = []Location{{Path: "/admin", Auth: &AuthMiddleware{IdentityProvider: "sso", Mode: "magic"}}}
			})}},
			wantErr: `proxy host "app" location "/admin": auth.mode must be`,
		},
		{
			name: "location inline rateLimit with neither form",
			cfg: Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
				h.Locations = []Location{{Path: "/api", RateLimit: &RateLimitMiddleware{}}}
			})}},
			wantErr: `proxy host "app" location "/api": rateLimit.requestsPerSecond must be > 0`,
		},

		{
			// Reuse and the direct path are not exclusive: a host may run an
			// inline gate AND reference a shared middleware of the same kind.
			name: "inline blocks coexist with referenced middlewares of the same kind",
			cfg: Config{
				IdentityProviders: inlineIdPs(),
				Middlewares: []Middleware{
					{
						ObjectMeta: ObjectMeta{Name: "fleet-sso"}, Type: MWTypeAuth,
						Auth: &AuthMiddleware{IdentityProvider: "authentik", Mode: AuthModeAuthRequest},
					},
					{
						ObjectMeta: ObjectMeta{Name: "fleet-rl"}, Type: MWTypeRateLimit,
						RateLimit: &RateLimitMiddleware{RequestsPerSecond: 5},
					},
				},
				ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) {
					h.Middlewares = []string{"fleet-sso", "fleet-rl"}
					h.Auth = &AuthMiddleware{IdentityProvider: "sso", Mode: AuthModeOIDC}
					h.RateLimit = &RateLimitMiddleware{Requests: 100, Window: "1m"}
				})},
			},
		},
		{
			name: "no inline blocks at all still validates",
			cfg:  Config{ProxyHosts: []ProxyHost{proxyHost("app", nil)}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Validate() = %v, want no error", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestInlineBlocksAreOmittedWhenUnset pins the additive, backwards-compatible
// shape: an unset block is a nil pointer, so every existing config keeps
// serialising byte-for-byte as it did.
func TestInlineBlocksAreOmittedWhenUnset(t *testing.T) {
	h := proxyHost("app", nil)
	if h.Auth != nil || h.RateLimit != nil {
		t.Fatalf("a host with no inline blocks must have nil auth/rateLimit, got %+v / %+v", h.Auth, h.RateLimit)
	}
	l := Location{Path: "/"}
	if l.Auth != nil || l.RateLimit != nil {
		t.Fatalf("a location with no inline blocks must have nil auth/rateLimit, got %+v / %+v", l.Auth, l.RateLimit)
	}
}
