package model

import (
	"path/filepath"
	"strings"
	"testing"
)

// A value is a placeholder only when it is nothing but placeholders: literal
// text around a token is committed and echoed verbatim, so it is a literal.
func TestIsPlaceholderIsAnchored(t *testing.T) {
	yes := []string{"${ENV:TOKEN}", " ${FILE:/run/secrets/a} ", "${ENV:A}${FILE:/run/secrets/b}", "${ENV:A} ${ENV:B}"}
	no := []string{"Bearer ${ENV:TOKEN}", "${ENV:TOKEN}-suffix", "x${FILE:/run/secrets/a}", "plain", "", "${ENV:}"}
	for _, v := range yes {
		if !Secret(v).IsPlaceholder() {
			t.Errorf("%q must count as a placeholder", v)
		}
	}
	for _, v := range no {
		if Secret(v).IsPlaceholder() {
			t.Errorf("%q must not count as a placeholder", v)
		}
	}
	cfg := Config{DNSProviders: []DNSProvider{{ObjectMeta: ObjectMeta{Name: "cf"}, Provider: DNSProviderCloudflare,
		Config: map[string]Secret{"apiToken": Secret("Bearer ${ENV:CF_TOKEN}")}}}}
	if got := LiteralSecrets(cfg); len(got) != 1 {
		t.Fatalf("a mixed value must be reported as a literal, got %v", got)
	}
}

// Explicit discovery token/CA files are confined to the secret file roots,
// and never one of gpm's own credential files; the in-cluster defaults
// (empty fields) are not checked.
func TestIngressDiscoveryFilesAreConfined(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GPM_SECRET_FILE_ROOTS", root)
	base := validIngressDiscovery()
	base.TokenFile, base.CAFile = "", "" // the in-cluster defaults
	if err := base.Validate(); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	inside := base
	inside.TokenFile = filepath.Join(root, "token")
	inside.CAFile = filepath.Join(root, "ca.crt")
	if err := inside.Validate(); err != nil {
		t.Fatalf("files under the root must be accepted: %v", err)
	}
	outside := base
	outside.TokenFile = "/etc/shadow"
	if err := outside.Validate(); err == nil || !strings.Contains(err.Error(), "tokenFile") {
		t.Fatalf("a file outside the roots must be refused, got %v", err)
	}
	relative := base
	relative.CAFile = "ca.crt"
	if err := relative.Validate(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("a relative path must be refused, got %v", err)
	}
}

// Two gate shapes that load but cannot work as written are reported through
// Warnings: a client-cert gate on a host without its own tls.clientAuth (the
// data plane fails it closed), and more than one OIDC gate on a host (they
// share one session cookie and callback and sign the user out of each other).
func TestConfigWarningsGates(t *testing.T) {
	idps := append(inlineIdPs(), IdentityProvider{
		ObjectMeta: ObjectMeta{Name: "sso2"}, Type: IdPTypeOIDC,
		OIDC: &OIDCSpec{IssuerURL: "https://idp2.example.com", ClientID: "gpm"},
	})
	// The "fine" host below restricts a location by role, which needs the
	// provider to map roles.
	for i := range idps {
		if idps[i].Name == "sso" {
			idps[i].RoleMapping = &RoleMapping{AdminGroups: []string{"admins"}}
		}
	}
	cfg := Config{
		IdentityProviders: idps,
		Middlewares: []Middleware{
			{ObjectMeta: ObjectMeta{Name: "mtls"}, Type: MWTypeAuth, Auth: &AuthMiddleware{Mode: AuthModeClientCert}},
			{ObjectMeta: ObjectMeta{Name: "sso"}, Type: MWTypeAuth, Auth: &AuthMiddleware{IdentityProvider: "sso", Mode: AuthModeOIDC}},
		},
		ProxyHosts: []ProxyHost{
			proxyHost("certless", func(h *ProxyHost) { h.Middlewares = []string{"mtls"} }),
			// Two providers on one host: warned.
			proxyHost("double", func(h *ProxyHost) {
				h.Middlewares = []string{"sso"}
				h.Locations = []Location{{Path: "/admin", Upstream: &Upstream{Scheme: "http", Host: "10.0.0.5", Port: 80}, Auth: &AuthMiddleware{IdentityProvider: "sso2", Mode: AuthModeOIDC}}}
			}),
			// The same provider gating the host and a role-restricted location
			// shares one session: fine.
			proxyHost("fine", func(h *ProxyHost) {
				h.Middlewares = []string{"sso"}
				h.Locations = []Location{{Path: "/admin", Upstream: &Upstream{Scheme: "http", Host: "10.0.0.5", Port: 80}, Auth: &AuthMiddleware{IdentityProvider: "sso", Mode: AuthModeOIDC, RequiredRoles: []string{"admin"}}}}
			}),
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the config must still load: %v", err)
	}
	warns := strings.Join(cfg.Warnings(), "\n")
	if !strings.Contains(warns, `proxy host "certless": middleware "mtls" uses auth mode client-cert, which needs this host's own tls.clientAuth`) {
		t.Fatalf("missing client-cert warning in:\n%s", warns)
	}
	if !strings.Contains(warns, `proxy host "double": OIDC gates for 2 different identity providers (sso, sso2)`) {
		t.Fatalf("missing double-OIDC warning in:\n%s", warns)
	}
	if strings.Contains(warns, `proxy host "fine"`) {
		t.Fatalf("a host with one OIDC gate must not be warned about:\n%s", warns)
	}
}

// The documented in-cluster token and CA paths are accepted even though they
// sit outside the secret file roots; any other explicit path is confined.
func TestIngressDiscoveryDefaultsAreExempt(t *testing.T) {
	t.Setenv("GPM_SECRET_FILE_ROOTS", t.TempDir())
	d := validIngressDiscovery()
	d.TokenFile, d.CAFile = DefaultKubernetesTokenFile, DefaultKubernetesCAFile
	if err := d.Validate(); err != nil {
		t.Fatalf("the projected in-cluster paths must be accepted: %v", err)
	}
	d.TokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/other"
	if err := d.Validate(); err == nil {
		t.Fatal("a sibling path outside the roots must be refused")
	}
}

// requiredRoles names roles, not groups. An entry that is not a role, a
// provider that maps no role, and requiredRoles under an inherited
// auth-request mode all load and then do not do what was written: each is
// reported through Warnings, wherever the gate is written (a middleware, a
// host, a location), and never fails the load.
func TestConfigWarningsRequiredRoles(t *testing.T) {
	oidcSpec := func(issuer string) *OIDCSpec { return &OIDCSpec{IssuerURL: issuer, ClientID: "gpm"} }
	idps := []IdentityProvider{
		{ObjectMeta: ObjectMeta{Name: "mapped"}, Type: IdPTypeOIDC, OIDC: oidcSpec("https://idp.example.com"),
			RoleMapping: &RoleMapping{AdminGroups: []string{"proxy-admins"}, UserGroups: []string{"staff"}}},
		{ObjectMeta: ObjectMeta{Name: "unmapped"}, Type: IdPTypeOIDC, OIDC: oidcSpec("https://idp2.example.com")},
		{ObjectMeta: ObjectMeta{Name: "empty-map"}, Type: IdPTypeOIDC, OIDC: oidcSpec("https://idp3.example.com"), RoleMapping: &RoleMapping{}},
		{ObjectMeta: ObjectMeta{Name: "edge"}, Type: IdPTypeForwardAuth, ForwardAuth: &ForwardAuthSpec{TrustedProxies: []string{"192.0.2.0/24"}, UserHeader: "X-User"},
			RoleMapping: &RoleMapping{DefaultRole: "user"}},
		{ObjectMeta: ObjectMeta{Name: "edge-unmapped"}, Type: IdPTypeForwardAuth, ForwardAuth: &ForwardAuthSpec{TrustedProxies: []string{"192.0.2.0/24"}, UserHeader: "X-User"}},
		{ObjectMeta: ObjectMeta{Name: "outpost"}, Type: IdPTypeAuthRequest, AuthRequest: &AuthRequestSpec{OutpostURL: "http://auth-outpost:9000"}},
	}
	loc := func(a *AuthMiddleware) []Location {
		return []Location{{Path: "/admin", Upstream: &Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80}, Auth: a}}
	}
	mw := func(name string, a AuthMiddleware) Middleware {
		return Middleware{ObjectMeta: ObjectMeta{Name: name}, Type: MWTypeAuth, Auth: &a}
	}
	cfg := Config{
		IdentityProviders: idps,
		Middlewares: []Middleware{
			mw("group-name", AuthMiddleware{IdentityProvider: "mapped", Mode: AuthModeOIDC, RequiredRoles: []string{"staff", "admin"}}),
			mw("roles-ok", AuthMiddleware{IdentityProvider: "mapped", Mode: AuthModeOIDC, RequiredRoles: []string{"admin", "user"}}),
			mw("no-roles", AuthMiddleware{IdentityProvider: "unmapped", Mode: AuthModeOIDC}),
			mw("fa-group", AuthMiddleware{IdentityProvider: "edge", Mode: AuthModeForwardAuth, RequiredRoles: []string{"ops"}}),
			mw("fa-ok", AuthMiddleware{IdentityProvider: "edge", Mode: AuthModeForwardAuth, RequiredRoles: []string{"user"}}),
			mw("fa-unmapped", AuthMiddleware{IdentityProvider: "edge-unmapped", Mode: AuthModeForwardAuth, RequiredRoles: []string{"user"}}),
			mw("empty-map", AuthMiddleware{IdentityProvider: "empty-map", Mode: AuthModeOIDC, RequiredRoles: []string{"user"}}),
			// Mode left unset: auth-request comes from the provider type, and
			// requiredRoles is then silently not applied.
			mw("ar-inherited", AuthMiddleware{IdentityProvider: "outpost", RequiredRoles: []string{"admin"}}),
			mw("ar-plain", AuthMiddleware{IdentityProvider: "outpost"}),
		},
		ProxyHosts: []ProxyHost{
			// Mode left unset: oidc comes from the provider type.
			proxyHost("inline", func(h *ProxyHost) {
				h.Auth = &AuthMiddleware{IdentityProvider: "unmapped", RequiredRoles: []string{"user"}}
			}),
			proxyHost("scoped", func(h *ProxyHost) {
				h.Locations = loc(&AuthMiddleware{IdentityProvider: "mapped", Mode: AuthModeOIDC, RequiredRoles: []string{"Admin"}})
			}),
			proxyHost("clean", func(h *ProxyHost) {
				h.Auth = &AuthMiddleware{IdentityProvider: "mapped", Mode: AuthModeOIDC, RequiredRoles: []string{"user"}}
			}),
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the config must still load: %v", err)
	}
	got := cfg.Warnings()
	want := []string{
		`middleware "group-name": auth.requiredRoles: "staff" is not a role, so it admits no one; the roles are "admin" and "user" (a roleMapping maps groups to them)`,
		`middleware "fa-group": auth.requiredRoles: "ops" is not a role, so it admits no one; the roles are "admin" and "user" (a roleMapping maps groups to them)`,
		`middleware "fa-unmapped": auth.requiredRoles is set but the roleMapping of identity provider "edge-unmapped" gives no user a role, so every request is refused; map groups to roles on the provider or remove requiredRoles`,
		`middleware "empty-map": auth.requiredRoles is set but the roleMapping of identity provider "empty-map" gives no user a role, so every request is refused; map groups to roles on the provider or remove requiredRoles`,
		`middleware "ar-inherited": auth.requiredRoles is not applied: the gate takes auth-request mode from identity provider "outpost", where the auth server decides who may pass; remove requiredRoles`,
		`proxy host "inline": auth.requiredRoles is set but the roleMapping of identity provider "unmapped" gives no user a role, so every request is refused; map groups to roles on the provider or remove requiredRoles`,
		`proxy host "scoped": location "/admin": auth.requiredRoles: "Admin" is not a role, so it admits no one; the roles are "admin" and "user" (a roleMapping maps groups to them)`,
	}
	if len(got) != len(want) {
		t.Errorf("got %d warnings, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	all := strings.Join(got, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("missing warning:\n%s\nin:\n%s", w, all)
		}
	}
}
