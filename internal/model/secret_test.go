package model

import (
	"encoding/json"
	"testing"
)

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
