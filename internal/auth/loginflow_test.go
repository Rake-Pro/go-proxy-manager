package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
	jose "github.com/go-jose/go-jose/v4"
	"golang.org/x/crypto/bcrypt"
)

// --- fake OIDC provider ----------------------------------------------------

// authIdP is a minimal OpenID Provider used to drive BeginLogin/CompleteLogin:
// discovery document, JWKS from a generated RSA key, and a token endpoint that
// mints an ID token from claims the test controls.
type authIdP struct {
	srv    *httptest.Server
	signer jose.Signer
	pub    jose.JSONWebKey

	mu     sync.Mutex
	claims map[string]any
}

func newAuthIdP(t *testing.T) *authIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "auth-test-key"
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: kid, Algorithm: "RS256", Use: "sig"}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	f := &authIdP{
		signer: signer,
		pub:    jose.JSONWebKey{Key: &key.PublicKey, KeyID: kid, Algorithm: "RS256", Use: "sig"},
		claims: map[string]any{},
	}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	issuer := f.srv.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/authorize",
			"token_endpoint":                        issuer + "/token",
			"jwks_uri":                              issuer + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{f.pub}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "Bearer", "expires_in": 3600,
			"id_token": f.signIDToken(t),
		})
	})
	return f
}

func (f *authIdP) setClaims(c map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claims = c
}

func (f *authIdP) signIDToken(t *testing.T) string {
	t.Helper()
	claims := map[string]any{
		"iss": f.srv.URL, "aud": "gpm-client", "sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}
	f.mu.Lock()
	for k, v := range f.claims {
		claims[k] = v
	}
	f.mu.Unlock()
	b, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := f.signer.Sign(b)
	if err != nil {
		t.Fatal(err)
	}
	s, err := obj.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// oidcAuthenticator wires an Authenticator to the fake IdP under the name "acme".
func oidcAuthenticator(t *testing.T, idp *authIdP, rm *model.RoleMapping) *Authenticator {
	t.Helper()
	a := NewAuthenticator(Options{Store: testStore(t), SessionTTL: time.Hour})
	a.Configure(
		model.Config{IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "acme", DisplayName: "Acme SSO"},
			Type:        model.IdPTypeOIDC,
			OIDC:        &model.OIDCSpec{IssuerURL: idp.srv.URL, ClientID: "gpm-client"},
			RoleMapping: rm,
		}}},
		model.Settings{ExternalBaseURL: "https://admin.example.test", AdminAuth: model.AdminAuthSettings{Providers: []string{"acme"}}},
	)
	return a
}

// nonceOf pulls the nonce gpm generated out of the authorization URL.
func nonceOf(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	n := u.Query().Get("nonce")
	if n == "" {
		t.Fatalf("authorization URL carries no nonce: %s", authURL)
	}
	return n
}

// --- BeginLogin / CompleteLogin -------------------------------------------

// stateOf pulls the OAuth2 state gpm generated out of the authorization URL.
func stateOf(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	s := u.Query().Get("state")
	if s == "" {
		t.Fatalf("authorization URL carries no state: %s", authURL)
	}
	return s
}

func TestBeginAndCompleteLogin(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
	ctx := context.Background()

	authURL, stateCookie, err := a.BeginLogin(ctx, "acme", "/hosts")
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	if stateCookie == "" {
		t.Fatal("BeginLogin must return a login-state cookie value")
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	state := q.Get("state")
	if state == "" {
		t.Fatal("authorization URL carries no state")
	}
	if q.Get("client_id") != "gpm-client" || q.Get("response_type") != "code" {
		t.Fatalf("authorization URL params = %v", q)
	}
	if q.Get("redirect_uri") != "https://admin.example.test/auth/callback" {
		t.Fatalf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	// PKCE is on by default.
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("expected an S256 PKCE challenge, got %v", q)
	}

	idp.setClaims(map[string]any{
		"nonce": nonceOf(t, authURL), "email": "a@example.test", "name": "A",
		"groups": []string{"proxy-admins"},
	})
	sess, returnTo, err := a.CompleteLogin(ctx, stateCookie, state, "the-code")
	if err != nil {
		t.Fatalf("CompleteLogin: %v", err)
	}
	if returnTo != "/hosts" {
		t.Fatalf("returnTo = %q, want /hosts", returnTo)
	}
	if sess.Subject != "user-123" || sess.Email != "a@example.test" || sess.IdP != "acme" {
		t.Fatalf("session identity = %+v", sess)
	}
	if len(sess.Roles) != 1 || sess.Roles[0] != string(RoleAdmin) {
		t.Fatalf("roles = %v, want [admin]", sess.Roles)
	}
	if sess.ID == "" || sess.CSRFToken == "" {
		t.Fatal("CompleteLogin must persist a session with an id and CSRF token")
	}
	// The session really is in the store.
	if _, err := a.store.Get(ctx, sess.ID); err != nil {
		t.Fatalf("session not persisted: %v", err)
	}
	// The cookie binds exactly this flow's state.
	if _, _, err := a.CompleteLogin(ctx, stateCookie, "another-state", "the-code"); !errors.Is(err, ErrLoginState) {
		t.Fatalf("a login state for another flow: err = %v, want ErrLoginState", err)
	}
}

func TestCompleteLoginFailures(t *testing.T) {
	t.Run("unsigned state cookie", func(t *testing.T) {
		a := oidcAuthenticator(t, newAuthIdP(t), &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		if _, _, err := a.CompleteLogin(context.Background(), "never-issued", "never-issued", "code"); !errors.Is(err, ErrLoginState) {
			t.Fatalf("err = %v, want ErrLoginState", err)
		}
	})

	t.Run("expired login state", func(t *testing.T) {
		a := oidcAuthenticator(t, newAuthIdP(t), &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		c, err := a.signLoginState(loginState{State: "st", Nonce: "n", Verifier: "v", IdP: "acme", Return: "/",
			Exp: time.Now().Add(-time.Minute).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := a.CompleteLogin(context.Background(), c, "st", "code"); !errors.Is(err, ErrLoginState) {
			t.Fatalf("err = %v, want ErrLoginState", err)
		}
	})

	t.Run("tampered login state", func(t *testing.T) {
		a := oidcAuthenticator(t, newAuthIdP(t), &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		authURL, c, err := a.BeginLogin(context.Background(), "acme", "/")
		if err != nil {
			t.Fatal(err)
		}
		payload, sig, _ := strings.Cut(c, ".")
		raw, err := base64.RawURLEncoding.DecodeString(payload)
		if err != nil {
			t.Fatal(err)
		}
		forged := base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(raw), `"r":"/"`, `"r":"/x"`, 1))) + "." + sig
		if _, _, err := a.CompleteLogin(context.Background(), forged, stateOf(t, authURL), "code"); !errors.Is(err, ErrLoginState) {
			t.Fatalf("err = %v, want ErrLoginState", err)
		}
	})

	t.Run("login state from another process", func(t *testing.T) {
		idp := newAuthIdP(t)
		a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		authURL, c, err := a.BeginLogin(context.Background(), "acme", "/")
		if err != nil {
			t.Fatal(err)
		}
		restarted := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		if _, _, err := restarted.CompleteLogin(context.Background(), c, stateOf(t, authURL), "code"); !errors.Is(err, ErrLoginState) {
			t.Fatalf("err = %v, want ErrLoginState", err)
		}
	})

	t.Run("nonce mismatch", func(t *testing.T) {
		idp := newAuthIdP(t)
		a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		authURL, c, err := a.BeginLogin(context.Background(), "acme", "/")
		if err != nil {
			t.Fatal(err)
		}
		idp.setClaims(map[string]any{"nonce": "wrong", "groups": []string{"proxy-admins"}})
		_, _, err = a.CompleteLogin(context.Background(), c, stateOf(t, authURL), "code")
		if err == nil || !strings.Contains(err.Error(), "nonce") {
			t.Fatalf("expected a nonce mismatch error, got %v", err)
		}
	})

	t.Run("no role mapping denies", func(t *testing.T) {
		idp := newAuthIdP(t)
		a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		authURL, c, err := a.BeginLogin(context.Background(), "acme", "/")
		if err != nil {
			t.Fatal(err)
		}
		idp.setClaims(map[string]any{"nonce": nonceOf(t, authURL), "groups": []string{"nobody"}})
		_, _, err = a.CompleteLogin(context.Background(), c, stateOf(t, authURL), "code")
		if err == nil || !strings.Contains(err.Error(), "no role mapping") {
			t.Fatalf("expected a role-mapping denial, got %v", err)
		}
	})
}

// Only a provider listed in adminAuth.providers can sign in to the panel. Any
// other configured OIDC provider is refused at the start of the login and at
// the callback with the error an unknown name gets.
func TestAdminLoginRequiresListedProvider(t *testing.T) {
	idp := newAuthIdP(t)
	a := NewAuthenticator(Options{Store: testStore(t), SessionTTL: time.Hour})
	cfg := model.Config{IdentityProviders: []model.IdentityProvider{
		{ObjectMeta: model.ObjectMeta{Name: "admin-sso"}, Type: model.IdPTypeOIDC,
			OIDC:        &model.OIDCSpec{IssuerURL: idp.srv.URL, ClientID: "gpm-client"},
			RoleMapping: &model.RoleMapping{AdminGroups: []string{"proxy-admins"}}},
		{ObjectMeta: model.ObjectMeta{Name: "site-gate"}, Type: model.IdPTypeOIDC,
			OIDC:        &model.OIDCSpec{IssuerURL: idp.srv.URL, ClientID: "gpm-client"},
			RoleMapping: &model.RoleMapping{AdminGroups: []string{"site-users"}}},
	}}
	settings := model.Settings{ExternalBaseURL: "https://admin.example.test",
		AdminAuth: model.AdminAuthSettings{Providers: []string{"admin-sso"}}}
	a.Configure(cfg, settings)
	ctx := context.Background()

	_, _, err := a.BeginLogin(ctx, "site-gate", "/")
	if err == nil || err.Error() != `identity provider "site-gate" not found` {
		t.Fatalf("BeginLogin with an unlisted provider: err = %v, want the not-found error", err)
	}

	// A login state naming an unlisted provider is refused at the callback too.
	c, err := a.signLoginState(loginState{State: "st", Nonce: "n", Verifier: "v", IdP: "site-gate", Return: "/",
		Exp: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	idp.setClaims(map[string]any{"nonce": "n", "groups": []string{"site-users"}})
	if _, _, err := a.CompleteLogin(ctx, c, "st", "code"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("CompleteLogin with an unlisted provider: err = %v, want not found", err)
	}

	// A provider taken off the list between the start and the callback.
	authURL, c, err := a.BeginLogin(ctx, "admin-sso", "/")
	if err != nil {
		t.Fatalf("BeginLogin with the listed provider: %v", err)
	}
	settings.AdminAuth.Providers = nil
	a.Configure(cfg, settings)
	idp.setClaims(map[string]any{"nonce": nonceOf(t, authURL), "groups": []string{"proxy-admins"}})
	if _, _, err := a.CompleteLogin(ctx, c, stateOf(t, authURL), "code"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("CompleteLogin after the provider was delisted: err = %v, want not found", err)
	}
}

// A non-default groupsClaim is honoured end to end.
func TestCompleteLoginCustomGroupsClaim(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{GroupsClaim: "roles", UserGroups: []string{"viewers"}})
	authURL, c, err := a.BeginLogin(context.Background(), "acme", "/")
	if err != nil {
		t.Fatal(err)
	}
	idp.setClaims(map[string]any{
		"nonce": nonceOf(t, authURL),
		// "groups" holds an admin-ish value that must be ignored; "roles" is the
		// configured claim.
		"groups": []string{"proxy-admins"},
		"roles":  []string{"viewers"},
	})
	sess, _, err := a.CompleteLogin(context.Background(), c, stateOf(t, authURL), "code")
	if err != nil {
		t.Fatalf("CompleteLogin: %v", err)
	}
	if sess.Roles[0] != string(RoleUser) {
		t.Fatalf("roles = %v, want [user]", sess.Roles)
	}
}

func TestBeginLoginConfigErrors(t *testing.T) {
	idp := newAuthIdP(t)
	oidcSpec := &model.OIDCSpec{IssuerURL: idp.srv.URL, ClientID: "gpm-client"}
	cases := []struct {
		name     string
		cfg      model.Config
		base     string
		idpName  string
		unlisted bool // leave idpName out of adminAuth.providers
		wantErr  string
	}{
		{
			name:    "unknown provider",
			cfg:     model.Config{},
			base:    "https://admin.example.test",
			idpName: "nope",
			wantErr: "not found",
		},
		{
			name: "provider not in adminAuth.providers",
			cfg: model.Config{IdentityProviders: []model.IdentityProvider{{
				ObjectMeta: model.ObjectMeta{Name: "acme"}, Type: model.IdPTypeOIDC, OIDC: oidcSpec,
			}}},
			base:     "https://admin.example.test",
			idpName:  "acme",
			unlisted: true,
			wantErr:  "not found",
		},
		{
			name: "provider is not oidc",
			cfg: model.Config{IdentityProviders: []model.IdentityProvider{{
				ObjectMeta: model.ObjectMeta{Name: "fa"}, Type: model.IdPTypeForwardAuth,
				ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
			}}},
			base:    "https://admin.example.test",
			idpName: "fa",
			wantErr: "is not OIDC",
		},
		{
			name: "externalBaseURL unset",
			cfg: model.Config{IdentityProviders: []model.IdentityProvider{{
				ObjectMeta: model.ObjectMeta{Name: "acme"}, Type: model.IdPTypeOIDC, OIDC: oidcSpec,
			}}},
			base:    "",
			idpName: "acme",
			wantErr: "externalBaseURL",
		},
		{
			name: "unresolvable client secret",
			cfg: model.Config{IdentityProviders: []model.IdentityProvider{{
				ObjectMeta: model.ObjectMeta{Name: "acme"}, Type: model.IdPTypeOIDC,
				OIDC: &model.OIDCSpec{IssuerURL: idp.srv.URL, ClientID: "gpm-client",
					ClientSecret: model.Secret("${ENV:GPM_TEST_SECRET_THAT_IS_NOT_SET}")},
			}}},
			base:    "https://admin.example.test",
			idpName: "acme",
			wantErr: "client secret",
		},
		{
			name: "issuer discovery fails",
			cfg: model.Config{IdentityProviders: []model.IdentityProvider{{
				ObjectMeta: model.ObjectMeta{Name: "acme"}, Type: model.IdPTypeOIDC,
				OIDC: &model.OIDCSpec{IssuerURL: "http://127.0.0.1:1/does-not-exist", ClientID: "gpm-client"},
			}}},
			base:    "https://admin.example.test",
			idpName: "acme",
			wantErr: "oidc discovery",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAuthenticator(Options{Store: testStore(t)})
			var providers []string
			if !tc.unlisted {
				providers = []string{tc.idpName}
			}
			a.Configure(tc.cfg, model.Settings{ExternalBaseURL: tc.base, AdminAuth: model.AdminAuthSettings{Providers: providers}})
			_, c, err := a.BeginLogin(context.Background(), tc.idpName, "/")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tc.wantErr)
			}
			if c != "" {
				t.Fatal("a login that never started must not return a login state")
			}
		})
	}
}

// --- login-state storage ----------------------------------------------------

// Starting a login stores nothing on the server, so no number of anonymous
// login starts, from one client or many, can refuse another client's login.
func TestBeginLoginKeepsNoServerState(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})

	a.pmu.Lock()
	for i := 0; i < maxPendingLogins; i++ {
		a.pending[fmt.Sprintf("live-%d", i)] = pendingLogin{totpUser: "admin", expires: time.Now().Add(time.Hour)}
	}
	a.pmu.Unlock()

	for i := 0; i < 100; i++ {
		if _, _, err := a.BeginLogin(context.Background(), "acme", "/"); err != nil {
			t.Fatalf("login start %d refused: %v", i+1, err)
		}
	}
	a.pmu.Lock()
	n := len(a.pending)
	a.pmu.Unlock()
	if n != maxPendingLogins {
		t.Fatalf("pending map changed from %d to %d entries", maxPendingLogins, n)
	}
}

// gcPendingLocked prunes expired pending-TOTP logins, so a map full of stale
// entries does not permanently wedge the second step.
func TestPendingTOTPGCReclaimsExpiredEntries(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t), LocalUser: "admin"})

	a.pmu.Lock()
	for i := 0; i < maxPendingLogins; i++ {
		a.pending[fmt.Sprintf("stale-%d", i)] = pendingLogin{totpUser: "admin", expires: time.Now().Add(-time.Minute)}
	}
	a.pmu.Unlock()

	if _, err := a.startPendingTOTP(); err != nil {
		t.Fatalf("expired entries should be reclaimed, got %v", err)
	}
	a.pmu.Lock()
	n := len(a.pending)
	a.pmu.Unlock()
	if n != 1 {
		t.Fatalf("pending map should hold only the new entry, got %d entries", n)
	}
}

// --- login-state cookie ----------------------------------------------------

func TestLoginStateCookieRoundTrip(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "insecure"
		if secure {
			name = "secure"
		}
		t.Run(name, func(t *testing.T) {
			mode := CookieSecureNever
			if secure {
				mode = CookieSecureAlways
			}
			a := NewAuthenticator(Options{Store: testStore(t), SecureMode: mode})
			r := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
			value, err := a.signLoginState(loginState{State: "st", Exp: time.Now().Add(time.Minute).Unix()})
			if err != nil {
				t.Fatal(err)
			}
			// A Secure cookie is __Host- named with Path=/, so a sibling host
			// cannot set one.
			wantName, wantPath := oidcStateCookie, "/auth"
			if secure {
				wantName, wantPath = oidcStateCookieSecure, "/"
			}

			rec := httptest.NewRecorder()
			a.SetLoginStateCookie(rec, r, value)
			c := findCookie(t, rec.Result(), wantName)
			if c.Value != value {
				t.Fatalf("value = %q", c.Value)
			}
			if c.Path != wantPath || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 600 {
				t.Fatalf("attributes: %+v", c)
			}
			if c.Secure != secure {
				t.Fatalf("Secure = %v, want %v", c.Secure, secure)
			}

			// The handler reads it straight back, also when another cookie of
			// the same name is sent first.
			req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
			req.AddCookie(&http.Cookie{Name: wantName, Value: "planted"})
			req.AddCookie(c)
			if got := a.LoginStateCookie(req, "st"); got != value {
				t.Fatalf("LoginStateCookie = %q", got)
			}
			if got := a.LoginStateCookie(req, "other"); got != "" {
				t.Fatalf("LoginStateCookie for another state = %q, want empty", got)
			}

			// Clearing expires it in place.
			rec2 := httptest.NewRecorder()
			a.ClearLoginStateCookie(rec2, r)
			cleared := findCookie(t, rec2.Result(), wantName)
			if cleared.Value != "" || cleared.MaxAge != -1 || cleared.Path != wantPath {
				t.Fatalf("cleared cookie: %+v", cleared)
			}
			if cleared.Secure != secure {
				t.Fatalf("cleared Secure = %v, want %v", cleared.Secure, secure)
			}
		})
	}
}

func TestLoginStateCookieAbsent(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t)})
	if got := a.LoginStateCookie(httptest.NewRequest(http.MethodGet, "/auth/callback", nil), "st"); got != "" {
		t.Fatalf("LoginStateCookie = %q, want empty", got)
	}
}

func findCookie(t *testing.T, res *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cookie %q not set (got %v)", name, res.Cookies())
	return nil
}

// --- IssueCookie / Logout --------------------------------------------------

func TestIssueCookieAndLogout(t *testing.T) {
	st := testStore(t)
	a := NewAuthenticator(Options{Store: st, SessionTTL: time.Hour})
	a.Configure(model.Config{}, model.Settings{})
	ctx := context.Background()

	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.Create(ctx, sess); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	a.IssueCookie(rec, httptest.NewRequest(http.MethodPost, "/auth/local", nil), sess)
	c := findCookie(t, rec.Result(), "gpm_session")
	if c.Value != sess.ID || !c.HttpOnly || c.Path != "/" || c.MaxAge <= 0 {
		t.Fatalf("issued cookie: %+v", c)
	}

	rec2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(c)
	a.Logout(rec2, req)
	cleared := findCookie(t, rec2.Result(), "gpm_session")
	if cleared.Value != "" || cleared.MaxAge != -1 {
		t.Fatalf("logout cookie: %+v", cleared)
	}
	if _, err := st.Get(ctx, sess.ID); err == nil {
		t.Fatal("logout must delete the session from the store")
	}
}

// Logging out without a session cookie is a no-op that still clears the cookie.
func TestLogoutWithoutCookie(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t)})
	rec := httptest.NewRecorder()
	a.Logout(rec, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))
	cleared := findCookie(t, rec.Result(), "gpm_session")
	if cleared.MaxAge != -1 {
		t.Fatalf("cookie should still be expired, got %+v", cleared)
	}
}

// --- local-login lockout window -------------------------------------------

// The lockout is time-boxed: once the window passes the same client can log in
// again. A short-window gate stands in for the 15-minute production default.
func TestLocalLoginLockoutExpiresWithWindow(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAuthenticator(Options{Store: testStore(t), LocalUser: "admin", LocalHash: string(hash), SessionTTL: time.Hour})
	a.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}})
	const window = 150 * time.Millisecond
	a.loginGate = newRateGate(window, 2, 16)

	ctx := context.Background()
	const key = "203.0.113.7"
	// attempt mirrors what handleLocalLogin does: gate first, then verify.
	attempt := func(pass string) (throttled bool, ok bool) {
		if !a.LoginAttempt(key) {
			return true, false
		}
		_, _, err := a.LocalLogin(ctx, "admin", pass)
		a.NoteLoginResult(key, err == nil)
		return false, err == nil
	}

	for i := 0; i < 2; i++ {
		if throttled, ok := attempt("wrong"); throttled || ok {
			t.Fatalf("attempt %d: throttled=%v ok=%v, want a plain failure", i+1, throttled, ok)
		}
	}
	// Locked out: even the correct password is refused before it is checked.
	if throttled, _ := attempt("s3cret"); !throttled {
		t.Fatal("the correct password must be refused while locked out")
	}

	time.Sleep(window + 50*time.Millisecond)

	throttled, ok := attempt("s3cret")
	if throttled {
		t.Fatal("the lockout must lift once the window passes")
	}
	if !ok {
		t.Fatal("the correct password must succeed after the window passes")
	}
	// A success clears the gate outright.
	if a.LoginThrottled(key) {
		t.Fatal("a successful login must reset the gate")
	}
}

func TestLocalLoginWithoutConfiguredAdmin(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t)})
	a.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}})
	if _, _, err := a.LocalLogin(context.Background(), "admin", "whatever"); err == nil {
		t.Fatal("local login must fail when no local admin is configured")
	}
	if a.LocalLoginVisible() {
		t.Fatal("the local form must be hidden when no local admin is configured")
	}
}

// --- session sliding expiry ------------------------------------------------

// A session with plenty of life left is not touched: no store write, no
// Set-Cookie. Only sessions past the half-TTL threshold slide.
func TestSlidingExpiryLeavesFreshSessionsAlone(t *testing.T) {
	st := testStore(t)
	a := NewAuthenticator(Options{Store: st, SessionTTL: time.Hour})
	a.Configure(model.Config{}, model.Settings{})

	exp := time.Now().Add(55 * time.Minute)
	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: exp}
	if err := st.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}

	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: a.cookieName, Value: sess.ID})
	h.ServeHTTP(rec, req)

	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("a fresh session must not re-issue its cookie, got %v", rec.Result().Cookies())
	}
	got, err := st.Get(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExpiresAt.Sub(exp) > time.Second || exp.Sub(got.ExpiresAt) > time.Second {
		t.Fatalf("expiry should be unchanged: was %v, now %v", exp, got.ExpiresAt)
	}
}

// A session past its absolute lifetime is refused and deleted even though its
// sliding expiry is still in the future, and sliding never extends a session
// past that lifetime.
func TestSessionAbsoluteLifetime(t *testing.T) {
	st := testStore(t)
	a := NewAuthenticator(withLocalAdmin(Options{Store: st, SessionTTL: time.Hour}))
	a.Configure(model.Config{}, localLoginOn(model.Settings{}))
	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	call := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		req.AddCookie(&http.Cookie{Name: a.cookieName, Value: id})
		h.ServeHTTP(rec, req)
		return rec
	}

	old := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local",
		CreatedAt: time.Now().Add(-maxSessionAge - time.Minute), ExpiresAt: time.Now().Add(50 * time.Minute)}
	if err := st.Create(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if rec := call(old.ID); rec.Code != http.StatusUnauthorized {
		t.Fatalf("session past its absolute lifetime: got %d, want 401", rec.Code)
	}
	if _, err := st.Get(context.Background(), old.ID); err == nil {
		t.Fatal("a session past its absolute lifetime must be deleted")
	}

	created := time.Now().Add(-maxSessionAge + 10*time.Minute)
	near := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local",
		CreatedAt: created, ExpiresAt: time.Now().Add(5 * time.Minute)}
	if err := st.Create(context.Background(), near); err != nil {
		t.Fatal(err)
	}
	if rec := call(near.ID); rec.Code != http.StatusOK {
		t.Fatalf("session inside its lifetime: got %d, want 200", rec.Code)
	}
	got, err := st.Get(context.Background(), near.ID)
	if err != nil {
		t.Fatal(err)
	}
	if limit := created.Add(maxSessionAge); got.ExpiresAt.After(limit.Add(time.Second)) {
		t.Fatalf("sliding moved expiry to %v, past the absolute limit %v", got.ExpiresAt, limit)
	}
	if !got.ExpiresAt.After(near.ExpiresAt) {
		t.Fatalf("sliding should still extend up to the limit: was %v, now %v", near.ExpiresAt, got.ExpiresAt)
	}
}

// --- login page metadata ---------------------------------------------------

func TestAppNameDefaultsAndOverride(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t)})
	a.Configure(model.Config{}, model.Settings{})
	if got := a.AppName(); got != "Go Proxy Manager" {
		t.Fatalf("AppName = %q, want the default", got)
	}
	a.Configure(model.Config{}, model.Settings{AppName: "Edge Manager"})
	if got := a.AppName(); got != "Edge Manager" {
		t.Fatalf("AppName = %q, want Edge Manager", got)
	}
}

func TestLoginOptions(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t)})
	a.Configure(model.Config{IdentityProviders: []model.IdentityProvider{
		{ObjectMeta: model.ObjectMeta{Name: "acme", DisplayName: "Acme SSO"}, Type: model.IdPTypeOIDC,
			OIDC: &model.OIDCSpec{IssuerURL: "https://idp.example.test", ClientID: "c"}},
		{ObjectMeta: model.ObjectMeta{Name: "bare"}, Type: model.IdPTypeOIDC,
			OIDC: &model.OIDCSpec{IssuerURL: "https://idp2.example.test", ClientID: "c"}},
		{ObjectMeta: model.ObjectMeta{Name: "fa"}, Type: model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"}},
	}}, model.Settings{AdminAuth: model.AdminAuthSettings{
		// "fa" is forward-auth (no login button) and "ghost" does not exist.
		Providers: []string{"acme", "bare", "fa", "ghost"},
	}})

	opts := a.LoginOptions()
	if len(opts) != 2 {
		t.Fatalf("LoginOptions = %+v, want 2 entries", opts)
	}
	if opts[0].Name != "acme" || opts[0].Label != "Acme SSO" {
		t.Fatalf("opts[0] = %+v", opts[0])
	}
	// A provider with no DisplayName falls back to its name.
	if opts[1].Name != "bare" || opts[1].Label != "bare" {
		t.Fatalf("opts[1] = %+v", opts[1])
	}
}

func TestGroupsClaimDefault(t *testing.T) {
	if got := groupsClaim(nil); got != "groups" {
		t.Fatalf("groupsClaim(nil) = %q", got)
	}
	if got := groupsClaim(&model.RoleMapping{}); got != "groups" {
		t.Fatalf("groupsClaim(empty) = %q", got)
	}
	if got := groupsClaim(&model.RoleMapping{GroupsClaim: "roles"}); got != "roles" {
		t.Fatalf("groupsClaim(roles) = %q", got)
	}
}
