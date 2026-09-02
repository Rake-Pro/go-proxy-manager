package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
)

// loginPageFor renders GET /auth/login against an authenticator built from the
// given local credential and config/settings, and returns the HTML.
func loginPageFor(t *testing.T, localUser, localHash string, cfg model.Config, settings model.Settings) string {
	t.Helper()
	sessStore, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("session store: %v", err)
	}
	t.Cleanup(func() { _ = sessStore.Close() })

	authn := auth.NewAuthenticator(auth.Options{
		Store:     sessStore,
		LocalUser: localUser,
		LocalHash: localHash,
	})
	authn.Configure(cfg, settings)

	s := &Server{authn: authn}
	w := httptest.NewRecorder()
	// select=1 so the single-provider auto-redirect never fires: this test is
	// about what the page SAYS, not about the redirect shortcut.
	s.handleLogin(w, httptest.NewRequest(http.MethodGet, "/auth/login?select=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /auth/login = %d, want 200", w.Code)
	}
	return w.Body.String()
}

func oidcProvider(name string) model.IdentityProvider {
	return model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: name},
		Type:       model.IdPTypeOIDC,
		OIDC:       &model.OIDCSpec{IssuerURL: "https://idp.example.com", ClientID: "gpm"},
	}
}

const bootstrapHeading = "No administrator login is configured"

func TestLoginPageBootstrapBanner(t *testing.T) {
	// A real-looking bcrypt hash; nothing verifies it here, only its presence.
	const hash = "$2a$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012"

	tests := []struct {
		name       string
		localUser  string
		localHash  string
		cfg        model.Config
		settings   model.Settings
		wantBanner bool
	}{
		{
			name:       "nothing configured at all",
			settings:   model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}},
			wantBanner: true,
		},
		{
			name:      "username set but no hash: the form can never succeed",
			localUser: "admin",
			settings:  model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}},
			// The form still renders (LocalLoginVisible), which is exactly the
			// silent failure the banner exists to name.
			wantBanner: true,
		},
		{
			name:      "local login turned off with no providers",
			localUser: "admin",
			localHash: hash,
			settings:  model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: false}},
			// Anti-lockout validation refuses this on write, but an older config or
			// a hand-edited repo can still boot into it.
			wantBanner: true,
		},
		{
			name:      "providers naming a provider that does not exist",
			localUser: "admin",
			localHash: hash,
			settings: model.Settings{AdminAuth: model.AdminAuthSettings{
				Providers: []string{"typo"}, SSOOnly: true,
			}},
			cfg:        model.Config{IdentityProviders: []model.IdentityProvider{oidcProvider("authentik-oidc")}},
			wantBanner: true,
		},
		{
			name:      "usable local credential",
			localUser: "admin",
			localHash: hash,
			settings:  model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}},
		},
		{
			name: "usable oidc provider",
			settings: model.Settings{AdminAuth: model.AdminAuthSettings{
				Providers: []string{"authentik-oidc"}, SSOOnly: true,
			}},
			cfg: model.Config{IdentityProviders: []model.IdentityProvider{oidcProvider("authentik-oidc")}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := loginPageFor(t, tc.localUser, tc.localHash, tc.cfg, tc.settings)
			got := strings.Contains(body, bootstrapHeading)
			if got != tc.wantBanner {
				t.Fatalf("banner present = %v, want %v\npage:\n%s", got, tc.wantBanner, body)
			}
			if !tc.wantBanner {
				return
			}
			// The banner must carry both remedies and the command, not just the
			// bad news.
			for _, want := range []string{
				"GPM_LOCAL_ADMIN_USER",
				"GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE",
				"adminAuth.providers",
				"oidc",
				"hashpw",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("banner does not mention %q", want)
				}
			}
			// No credential may be echoed into the page.
			if tc.localHash != "" && strings.Contains(body, tc.localHash) {
				t.Error("login page echoes the bcrypt hash")
			}
		})
	}
}

func TestNoAdminLoginConfigured(t *testing.T) {
	const hash = "$2a$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012"
	tests := []struct {
		name      string
		localUser string
		localHash string
		cfg       model.Config
		settings  model.Settings
		want      bool
	}{
		{name: "nothing", settings: model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}}, want: true},
		{name: "half a local credential", localUser: "admin", settings: model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}}, want: true},
		{name: "full local credential", localUser: "admin", localHash: hash, settings: model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}}},
		{
			name: "local credential but ssoOnly with no usable provider",
			// ssoOnly forbids local login, so the credential grants nothing.
			localUser: "admin", localHash: hash,
			settings: model.Settings{AdminAuth: model.AdminAuthSettings{SSOOnly: true, Providers: []string{"ghost"}}},
			want:     true,
		},
		{
			name:     "oidc provider only",
			settings: model.Settings{AdminAuth: model.AdminAuthSettings{Providers: []string{"idp"}}},
			cfg:      model.Config{IdentityProviders: []model.IdentityProvider{oidcProvider("idp")}},
		},
		{
			name: "forward-auth provider renders no button",
			settings: model.Settings{AdminAuth: model.AdminAuthSettings{
				Providers: []string{"fa"}, SSOOnly: true,
			}},
			cfg: model.Config{IdentityProviders: []model.IdentityProvider{{
				ObjectMeta:  model.ObjectMeta{Name: "fa"},
				Type:        model.IdPTypeForwardAuth,
				ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"192.0.2.0/24"}, UserHeader: "X-User"},
			}}},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sessStore, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
			if err != nil {
				t.Fatalf("session store: %v", err)
			}
			defer sessStore.Close()
			authn := auth.NewAuthenticator(auth.Options{Store: sessStore, LocalUser: tc.localUser, LocalHash: tc.localHash})
			authn.Configure(tc.cfg, tc.settings)
			if got := authn.NoAdminLoginConfigured(); got != tc.want {
				t.Errorf("NoAdminLoginConfigured() = %v, want %v", got, tc.want)
			}
		})
	}
}
