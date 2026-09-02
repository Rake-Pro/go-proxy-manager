package model

import (
	"strings"
	"testing"
)

// oidcIdP builds a minimally valid OIDC identity provider.
func oidcIdP(name string) IdentityProvider {
	return IdentityProvider{
		ObjectMeta: ObjectMeta{Name: name},
		Type:       IdPTypeOIDC,
		OIDC:       &OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
	}
}

func forwardAuthIdP(name string) IdentityProvider {
	return IdentityProvider{
		ObjectMeta:  ObjectMeta{Name: name},
		Type:        IdPTypeForwardAuth,
		ForwardAuth: &ForwardAuthSpec{TrustedProxies: []string{"192.0.2.0/24"}, UserHeader: "X-User"},
	}
}

func TestAdminAuthValidateRefs(t *testing.T) {
	tests := []struct {
		name     string
		auth     AdminAuthSettings
		idps     []IdentityProvider
		wantErr  bool
		contains []string // every fragment must appear in the error
	}{
		{
			name: "ssoOnly off is never checked, even with a dangling provider",
			auth: AdminAuthSettings{Providers: []string{"typo"}, LocalLoginEnabled: true},
			idps: []IdentityProvider{oidcIdP("authentik-oidc")},
		},
		{
			name: "ssoOnly with a resolving oidc provider passes",
			auth: AdminAuthSettings{Providers: []string{"authentik-oidc"}, SSOOnly: true},
			idps: []IdentityProvider{oidcIdP("authentik-oidc")},
		},
		{
			name: "ssoOnly with several resolving oidc providers passes",
			auth: AdminAuthSettings{Providers: []string{"a", "b"}, SSOOnly: true},
			idps: []IdentityProvider{oidcIdP("a"), oidcIdP("b")},
		},
		{
			name:     "ssoOnly with a typo'd provider is the lockout case",
			auth:     AdminAuthSettings{Providers: []string{"authentik"}, SSOOnly: true},
			idps:     []IdentityProvider{oidcIdP("authentik-oidc")},
			wantErr:  true,
			contains: []string{`"authentik"`, "does not exist", "ssoOnly"},
		},
		{
			name:     "ssoOnly naming a forward-auth provider renders no button",
			auth:     AdminAuthSettings{Providers: []string{"authentik-proxy"}, SSOOnly: true},
			idps:     []IdentityProvider{forwardAuthIdP("authentik-proxy")},
			wantErr:  true,
			contains: []string{`"authentik-proxy"`, IdPTypeForwardAuth, IdPTypeOIDC},
		},
		{
			name: "ssoOnly reports EVERY bad provider, not just the first",
			auth: AdminAuthSettings{Providers: []string{"gone", "proxy"}, SSOOnly: true},
			idps: []IdentityProvider{forwardAuthIdP("proxy")},
			// A batch error means one fix round-trip instead of two.
			wantErr:  true,
			contains: []string{`"gone"`, `"proxy"`},
		},
		{
			name:     "ssoOnly with one good and one bad provider still fails",
			auth:     AdminAuthSettings{Providers: []string{"good", "gone"}, SSOOnly: true},
			idps:     []IdentityProvider{oidcIdP("good")},
			wantErr:  true,
			contains: []string{`"gone"`},
		},
		{
			name:     "ssoOnly with no providers at all",
			auth:     AdminAuthSettings{SSOOnly: true},
			wantErr:  true,
			contains: []string{"at least one"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.auth.ValidateRefs(Config{IdentityProviders: tc.idps})
			if tc.wantErr != (err != nil) {
				t.Fatalf("ValidateRefs() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil {
				return
			}
			for _, want := range tc.contains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ValidateRefs() = %q, want it to mention %q", err, want)
				}
			}
			// No Go identifier vocabulary in an operator-facing message.
			if strings.Contains(err.Error(), "IdentityProvider{") {
				t.Errorf("ValidateRefs() leaks Go syntax: %q", err)
			}
		})
	}
}

// TestAdminLoginProviderTypesMatchesLoginOptions pins the list this package uses
// to decide which provider can log an admin in. internal/auth.LoginOptions emits
// a button only for IdPTypeOIDC; if that ever widens, this must widen with it or
// the guard starts refusing a config that would actually work.
func TestAdminLoginProviderTypesMatchesLoginOptions(t *testing.T) {
	if len(AdminLoginProviderTypes) != 1 || AdminLoginProviderTypes[0] != IdPTypeOIDC {
		t.Fatalf("AdminLoginProviderTypes = %v; update internal/auth.LoginOptions and this test together", AdminLoginProviderTypes)
	}
	for _, tc := range []struct {
		typ  string
		want bool
	}{
		{IdPTypeOIDC, true},
		{IdPTypeForwardAuth, false},
		{IdPTypeAuthRequest, false},
		{"", false},
	} {
		if got := adminLoginCapable(tc.typ); got != tc.want {
			t.Errorf("adminLoginCapable(%q) = %v, want %v", tc.typ, got, tc.want)
		}
	}
}

func TestConfigWarningsCertificateRefCoverage(t *testing.T) {
	certs := []Certificate{
		{ObjectMeta: ObjectMeta{Name: "wildcard"}, Domains: []string{"*.example.com"}},
		{ObjectMeta: ObjectMeta{Name: "apex"}, Domains: []string{"example.com"}},
		{ObjectMeta: ObjectMeta{Name: "other"}, Domains: []string{"*.example.net"}},
	}
	tests := []struct {
		name     string
		cfg      Config
		wantWarn bool
		contains []string
	}{
		{
			name: "wildcard covers a one-label child",
			cfg: Config{Certificates: certs, ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "app"},
				Domains:    []string{"app.example.com"},
				TLS:        TLSSettings{CertificateRef: "wildcard"},
			}}},
		},
		{
			name: "exact match",
			cfg: Config{Certificates: certs, ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "root"},
				Domains:    []string{"example.com"},
				TLS:        TLSSettings{CertificateRef: "apex"},
			}}},
		},
		{
			name: "one of several domains covered is enough",
			cfg: Config{Certificates: certs, ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "app"},
				Domains:    []string{"nope.example.org", "app.example.com"},
				TLS:        TLSSettings{CertificateRef: "wildcard"},
			}}},
		},
		{
			name: "no certificateRef is never a warning",
			cfg: Config{Certificates: certs, ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "app"},
				Domains:    []string{"app.example.com"},
			}}},
		},
		{
			name: "dangling ref is left to Validate, not warned about twice",
			cfg: Config{Certificates: certs, ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "app"},
				Domains:    []string{"app.example.com"},
				TLS:        TLSSettings{CertificateRef: "missing"},
			}}},
		},
		{
			name: "proxy host pointed at a certificate for another zone",
			cfg: Config{Certificates: certs, ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "app"},
				Domains:    []string{"app.example.com"},
				TLS:        TLSSettings{CertificateRef: "other"},
			}}},
			wantWarn: true,
			contains: []string{"proxy host", `"app"`, `"other"`, "app.example.com", "SNI"},
		},
		{
			name: "wildcard does not cover a two-label child",
			cfg: Config{Certificates: certs, ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "deep"},
				Domains:    []string{"a.b.example.com"},
				TLS:        TLSSettings{CertificateRef: "wildcard"},
			}}},
			wantWarn: true,
			contains: []string{"a.b.example.com"},
		},
		{
			name: "wildcard does not cover its own apex",
			cfg: Config{Certificates: certs, ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "root"},
				Domains:    []string{"example.com"},
				TLS:        TLSSettings{CertificateRef: "wildcard"},
			}}},
			wantWarn: true,
		},
		{
			name: "redirect host is checked too",
			cfg: Config{Certificates: certs, RedirectHosts: []RedirectHost{{
				ObjectMeta: ObjectMeta{Name: "old"},
				Domains:    []string{"old.example.net"},
				TLS:        TLSSettings{CertificateRef: "apex"},
			}}},
			wantWarn: true,
			contains: []string{"redirect host"},
		},
		{
			name: "parked host is checked too",
			cfg: Config{Certificates: certs, ParkedHosts: []ParkedHost{{
				ObjectMeta: ObjectMeta{Name: "parked"},
				Domains:    []string{"parked.example.net"},
				TLS:        TLSSettings{CertificateRef: "apex"},
			}}},
			wantWarn: true,
			contains: []string{"parked host"},
		},
		{
			name: "case and surrounding space do not defeat the match",
			cfg: Config{
				Certificates: []Certificate{{ObjectMeta: ObjectMeta{Name: "c"}, Domains: []string{" *.EXAMPLE.com "}}},
				ProxyHosts: []ProxyHost{{
					ObjectMeta: ObjectMeta{Name: "app"},
					Domains:    []string{"App.Example.COM"},
					TLS:        TLSSettings{CertificateRef: "c"},
				}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.Warnings()
			if tc.wantWarn != (len(got) > 0) {
				t.Fatalf("Warnings() = %v, wantWarn %v", got, tc.wantWarn)
			}
			if len(got) == 0 {
				return
			}
			joined := strings.Join(got, "\n")
			for _, want := range tc.contains {
				if !strings.Contains(joined, want) {
					t.Errorf("Warnings() = %q, want it to mention %q", joined, want)
				}
			}
		})
	}
}
