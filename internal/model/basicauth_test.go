package model

import (
	"strings"
	"testing"
)

// bcryptFixture is a syntactically valid bcrypt hash. Validation only checks the
// SHAPE of a hash, so no real hashing is needed (and none is wanted: bcrypt is
// deliberately slow and this table has many rows).
const bcryptFixture = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func basicMiddleware(a AuthMiddleware) Middleware {
	return Middleware{ObjectMeta: ObjectMeta{Name: "basic"}, Type: MWTypeAuth, Auth: &a}
}

func basicSpec(users ...BasicAuthUser) *BasicAuthSpec {
	return &BasicAuthSpec{Users: users}
}

func TestBasicAuthModeValidation(t *testing.T) {
	ok := BasicAuthUser{Username: "admin", PasswordHash: bcryptFixture}

	tests := []struct {
		name    string
		mw      Middleware
		wantErr string
	}{
		{"minimal", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic, Basic: basicSpec(ok)}), ""},
		{"with realm", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: &BasicAuthSpec{Users: []BasicAuthUser{ok}, Realm: "Staging area"}}), ""},
		{"with allowFrom", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			AllowFrom: []string{"10.0.0.0/8"}, Basic: basicSpec(ok)}), ""},

		{"no basic block", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic}),
			"auth.basic is required in basic mode"},
		{"no users", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic, Basic: basicSpec()}),
			"requires at least one user"},
		{"missing username", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: basicSpec(BasicAuthUser{PasswordHash: bcryptFixture})}), "no username"},
		{"colon in username", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: basicSpec(BasicAuthUser{Username: "a:b", PasswordHash: bcryptFixture})}), "must not contain"},
		{"duplicate username", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: basicSpec(ok, ok)}), "duplicate username"},
		{"missing hash", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: basicSpec(BasicAuthUser{Username: "admin"})}), "requires passwordHash"},
		{"plaintext instead of a hash", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: basicSpec(BasicAuthUser{Username: "admin", PasswordHash: "hunter2"})}), "is not a bcrypt hash"},
		{"truncated hash", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: basicSpec(BasicAuthUser{Username: "admin", PasswordHash: bcryptFixture[:59]})}), "is not a bcrypt hash"},
		{"wrong hash algorithm", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: basicSpec(BasicAuthUser{Username: "admin", PasswordHash: "$1$" + bcryptFixture[3:]})}), "is not a bcrypt hash"},
		{"too many users", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: &BasicAuthSpec{Users: manyUsers(MaxBasicAuthUsers + 1)}}), "at most 64 are allowed"},

		{"quote in realm", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: &BasicAuthSpec{Users: []BasicAuthUser{ok}, Realm: `a"b`}}), "auth.basic.realm contains"},
		{"backslash in realm", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: &BasicAuthSpec{Users: []BasicAuthUser{ok}, Realm: `a\b`}}), "auth.basic.realm contains"},
		{"newline in realm", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: &BasicAuthSpec{Users: []BasicAuthUser{ok}, Realm: "a\nb"}}), "auth.basic.realm contains"},
		{"non-ascii realm", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: &BasicAuthSpec{Users: []BasicAuthUser{ok}, Realm: "caf\u00e9"}}), "auth.basic.realm contains"},
		{"over-long realm", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			Basic: &BasicAuthSpec{Users: []BasicAuthUser{ok}, Realm: strings.Repeat("a", MaxBasicAuthRealmLen+1)}}),
			"at most 128 are allowed"},

		{"identity provider set", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			IdentityProvider: "authentik", Basic: basicSpec(ok)}), "not used in basic mode"},
		{"required roles", basicMiddleware(AuthMiddleware{Mode: AuthModeBasic,
			RequiredRoles: []string{"admin"}, Basic: basicSpec(ok)}), "not supported in basic mode"},
		{"basic block in another mode", basicMiddleware(AuthMiddleware{Mode: AuthModeOIDC,
			IdentityProvider: "authentik", Basic: basicSpec(ok)}), "only used in basic mode"},
		{"unknown mode still names basic", basicMiddleware(AuthMiddleware{Mode: "password",
			IdentityProvider: "authentik"}), "oidc|forward-auth|auth-request|client-cert|basic"},
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

func manyUsers(n int) []BasicAuthUser {
	out := make([]BasicAuthUser, 0, n)
	for i := range n {
		out = append(out, BasicAuthUser{Username: string(rune('a'+i%26)) + strings.Repeat("x", i/26+1), PasswordHash: bcryptFixture})
	}
	return out
}

// TestBasicAuthModeInlineOnHost checks the inline `auth:` block on a proxy host
// and on a location is held to the same rules, since both reuse AuthMiddleware.
func TestBasicAuthModeInlineOnHost(t *testing.T) {
	ok := BasicAuthUser{Username: "admin", PasswordHash: bcryptFixture}
	host := func(a *AuthMiddleware, loc *AuthMiddleware) Config {
		h := ProxyHost{
			ObjectMeta: ObjectMeta{Name: "app"},
			Domains:    []string{"app.example.com"},
			Upstream:   Upstream{Scheme: "http", Host: "127.0.0.1", Port: 8080},
			Auth:       a,
		}
		if loc != nil {
			h.Locations = []Location{{Path: "/admin", Auth: loc}}
		}
		return Config{ProxyHosts: []ProxyHost{h}}
	}
	good := &AuthMiddleware{Mode: AuthModeBasic, Basic: basicSpec(ok)}
	bad := &AuthMiddleware{Mode: AuthModeBasic, Basic: basicSpec(BasicAuthUser{Username: "admin", PasswordHash: "plaintext"})}

	if err := host(good, good).Validate(); err != nil {
		t.Fatalf("inline basic auth should validate, got: %v", err)
	}
	if err := host(bad, nil).Validate(); err == nil || !strings.Contains(err.Error(), `proxy host "app"`) {
		t.Fatalf("expected a proxy-host-scoped error, got: %v", err)
	}
	if err := host(nil, bad).Validate(); err == nil || !strings.Contains(err.Error(), "is not a bcrypt hash") {
		t.Fatalf("expected a location-scoped hash error, got: %v", err)
	}
}

// TestDeprecatedAccessListBasicAuthWarns checks a config still carrying
// AccessList.basicAuth loads (no validation error) and reports the deprecation
// once, naming the lists.
func TestDeprecatedAccessListBasicAuthWarns(t *testing.T) {
	cfg := Config{AccessLists: []AccessList{
		{ObjectMeta: ObjectMeta{Name: "legacy"}, BasicAuth: []BasicAuthUser{{Username: "admin", PasswordHash: bcryptFixture}}},
		{ObjectMeta: ObjectMeta{Name: "ip-only"}, Rules: []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8"}}},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a deprecated field must still load, got: %v", err)
	}
	var found string
	for _, w := range cfg.Warnings() {
		if strings.Contains(w, "basicAuth") {
			found = w
		}
	}
	if found == "" {
		t.Fatal("expected a deprecation warning naming basicAuth")
	}
	if !strings.Contains(found, "legacy") {
		t.Fatalf("warning should name the list, got: %q", found)
	}
	if strings.Contains(found, "ip-only") {
		t.Fatalf("warning should not name a list with no basicAuth, got: %q", found)
	}
}
