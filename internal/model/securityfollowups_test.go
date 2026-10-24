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
