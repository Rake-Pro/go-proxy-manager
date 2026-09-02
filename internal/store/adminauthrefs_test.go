package store

import (
	"context"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// adminAuth.ssoOnly turns off the local password form, so the SSO buttons are
// the only door. A provider name that does not resolve to an OIDC identity
// provider therefore renders a login page with nothing on it - a lockout
// recoverable only by editing the config repo and redeploying. The refusal has
// to land on the operator's own settings write, not on their next login attempt.
func TestSaveSettingsRejectsUnusableSSOOnlyProviders(t *testing.T) {
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}

	oidc := model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: "authentik-oidc"},
		Type:       model.IdPTypeOIDC,
		OIDC:       &model.OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
	}
	forwardAuth := model.IdentityProvider{
		ObjectMeta:  model.ObjectMeta{Name: "authentik-proxy"},
		Type:        model.IdPTypeForwardAuth,
		ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"192.0.2.0/24"}, UserHeader: "X-authentik-username"},
	}

	tests := []struct {
		name     string
		idps     []model.IdentityProvider
		auth     model.AdminAuthSettings
		wantErr  bool
		contains []string
	}{
		{
			name: "ssoOnly with a resolving oidc provider is saved",
			idps: []model.IdentityProvider{oidc},
			auth: model.AdminAuthSettings{Providers: []string{"authentik-oidc"}, SSOOnly: true},
		},
		{
			name: "a dangling provider without ssoOnly is left alone (local form remains)",
			idps: []model.IdentityProvider{oidc},
			auth: model.AdminAuthSettings{Providers: []string{"typo"}, LocalLoginEnabled: true},
		},
		{
			name:     "ssoOnly with a typo'd provider is refused",
			idps:     []model.IdentityProvider{oidc},
			auth:     model.AdminAuthSettings{Providers: []string{"authentik"}, SSOOnly: true},
			wantErr:  true,
			contains: []string{`"authentik"`, "does not exist"},
		},
		{
			name:     "ssoOnly naming a forward-auth provider is refused",
			idps:     []model.IdentityProvider{forwardAuth},
			auth:     model.AdminAuthSettings{Providers: []string{"authentik-proxy"}, SSOOnly: true},
			wantErr:  true,
			contains: []string{`"authentik-proxy"`, model.IdPTypeForwardAuth},
		},
		{
			name:     "no login method at all is refused by Validate",
			auth:     model.AdminAuthSettings{LocalLoginEnabled: false},
			wantErr:  true,
			contains: []string{"no admin login method configured"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestStore(t)
			for _, idp := range tc.idps {
				if _, err := st.Save(ctx, idp, author); err != nil {
					t.Fatalf("seed identity provider %q: %v", idp.Name, err)
				}
			}
			_, s, err := st.Load(ctx)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			s.AdminAuth = tc.auth

			_, err = st.SaveSettings(ctx, s, author)
			if tc.wantErr != (err != nil) {
				t.Fatalf("SaveSettings() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil {
				// A saved value must actually be readable back.
				_, after, lerr := st.Load(ctx)
				if lerr != nil {
					t.Fatalf("reload: %v", lerr)
				}
				if after.AdminAuth.SSOOnly != tc.auth.SSOOnly {
					t.Errorf("ssoOnly did not round-trip: %+v", after.AdminAuth)
				}
				return
			}
			for _, want := range tc.contains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("SaveSettings() = %q, want it to mention %q", err, want)
				}
			}
			// The refused write must not have landed.
			_, after, lerr := st.Load(ctx)
			if lerr != nil {
				t.Fatalf("reload: %v", lerr)
			}
			if after.AdminAuth.SSOOnly {
				t.Error("a refused settings write still persisted ssoOnly")
			}
		})
	}
}
