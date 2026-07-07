package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretResolveFileConfinement(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "token")
	if err := os.WriteFile(secretPath, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GPM_SECRET_FILE_ROOTS", dir)

	// In-root read succeeds and is trimmed.
	got, err := Secret("${FILE:" + secretPath + "}").Resolve()
	if err != nil {
		t.Fatalf("in-root resolve: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("got %q want %q", got, "s3cr3t")
	}

	// Traversal out of the root is rejected before any read.
	escape := filepath.Join(dir, "..", "etc", "shadow")
	if _, err := (Secret("${FILE:" + escape + "}")).Resolve(); err == nil {
		t.Fatal("expected traversal out of secret root to be rejected")
	}

	// An absolute path under a different root is rejected.
	if _, err := (Secret("${FILE:/etc/shadow}")).Resolve(); err == nil {
		t.Fatal("expected out-of-root absolute path to be rejected")
	}
}

func TestSecretResolveEnvGuard(t *testing.T) {
	// gpm's own reserved secrets are never resolvable via ${ENV:...}, even when set.
	t.Setenv("GPM_SSO_SIGNING_KEY", "signing-key")
	t.Setenv("GPM_LOCAL_ADMIN_PASSWORD_HASH", "hash")
	for _, name := range []string{"GPM_SSO_SIGNING_KEY", "GPM_LOCAL_ADMIN_PASSWORD_HASH"} {
		if _, err := Secret("${ENV:" + name + "}").Resolve(); err == nil {
			t.Fatalf("reserved env var %q must not resolve via ${ENV:...}", name)
		}
	}

	// Default (no GPM_SECRET_ENV_PREFIXES): any non-reserved name resolves.
	t.Setenv("CF_TOKEN", "cf")
	if got, err := Secret("${ENV:CF_TOKEN}").Resolve(); err != nil || got != "cf" {
		t.Fatalf("unrestricted resolve: got %q err %v", got, err)
	}

	// Strict allowlist mode: only names carrying an allowed prefix resolve.
	t.Setenv("GPM_SECRET_ENV_PREFIXES", "APP_,SVC_")
	t.Setenv("APP_TOKEN", "ok")
	if got, err := Secret("${ENV:APP_TOKEN}").Resolve(); err != nil || got != "ok" {
		t.Fatalf("prefixed resolve: got %q err %v", got, err)
	}
	if _, err := Secret("${ENV:CF_TOKEN}").Resolve(); err == nil {
		t.Fatal("name outside the allowed prefixes must be rejected in strict mode")
	}
}

func TestSecretMarshalJSONRedacts(t *testing.T) {
	cases := []struct {
		name string
		in   Secret
		want string
	}{
		{"literal", Secret("hunter2"), `"***"`},
		{"placeholder env", Secret("${ENV:OIDC_CLIENT_SECRET}"), `"${ENV:OIDC_CLIENT_SECRET}"`},
		{"placeholder file", Secret("${FILE:/run/secrets/cf}"), `"${FILE:/run/secrets/cf}"`},
		{"empty", Secret(""), `""`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(c.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != c.want {
				t.Fatalf("got %s want %s", b, c.want)
			}
		})
	}
}

func TestLiteralSecrets(t *testing.T) {
	cfg := Config{
		IdentityProviders: []IdentityProvider{{
			ObjectMeta: ObjectMeta{Name: "idp"},
			Type:       IdPTypeOIDC,
			OIDC:       &OIDCSpec{IssuerURL: "https://i", ClientID: "c", ClientSecret: Secret("plaintext")},
		}},
		DNSProviders: []DNSProvider{{
			ObjectMeta: ObjectMeta{Name: "cf"},
			Provider:   "cloudflare",
			Config:     map[string]Secret{"apiToken": Secret("${ENV:CF_TOKEN}"), "extra": Secret("raw")},
		}},
	}
	got := LiteralSecrets(cfg)
	if len(got) != 2 {
		t.Fatalf("want 2 literal paths, got %d: %v", len(got), got)
	}
	want := map[string]bool{
		"IdentityProviders[0].OIDC.ClientSecret": true,
		"DNSProviders[0].Config[extra]":          true,
	}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected literal path %q in %v", p, got)
		}
	}
}

func TestLiteralSecretsAllPlaceholders(t *testing.T) {
	cfg := Config{
		IdentityProviders: []IdentityProvider{{
			ObjectMeta: ObjectMeta{Name: "idp"},
			Type:       IdPTypeOIDC,
			OIDC:       &OIDCSpec{IssuerURL: "https://i", ClientID: "c", ClientSecret: Secret("${ENV:S}")},
		}},
	}
	if got := LiteralSecrets(cfg); len(got) != 0 {
		t.Fatalf("want no literal paths, got %v", got)
	}
}
