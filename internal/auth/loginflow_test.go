package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
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

func TestBeginAndCompleteLogin(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
	ctx := context.Background()

	authURL, state, err := a.BeginLogin(ctx, "acme", "/hosts", "203.0.113.7")
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	if state == "" {
		t.Fatal("BeginLogin must return a state value")
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("state") != state {
		t.Fatalf("authorization URL state = %q, want %q", q.Get("state"), state)
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
	sess, returnTo, err := a.CompleteLogin(ctx, state, "the-code")
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
	// The state is consumed exactly once.
	if _, _, err := a.CompleteLogin(ctx, state, "the-code"); err == nil {
		t.Fatal("a state must not be reusable")
	}
}

func TestCompleteLoginFailures(t *testing.T) {
	t.Run("unknown state", func(t *testing.T) {
		a := oidcAuthenticator(t, newAuthIdP(t), &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		if _, _, err := a.CompleteLogin(context.Background(), "never-issued", "code"); err == nil {
			t.Fatal("expected an error for an unknown state")
		}
	})

	t.Run("expired pending login", func(t *testing.T) {
		idp := newAuthIdP(t)
		a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		authURL, state, err := a.BeginLogin(context.Background(), "acme", "/", "203.0.113.7")
		if err != nil {
			t.Fatal(err)
		}
		idp.setClaims(map[string]any{"nonce": nonceOf(t, authURL), "groups": []string{"proxy-admins"}})
		a.pmu.Lock()
		p := a.pending[state]
		p.expires = time.Now().Add(-time.Minute)
		a.pending[state] = p
		a.pmu.Unlock()
		if _, _, err := a.CompleteLogin(context.Background(), state, "code"); err == nil {
			t.Fatal("an expired login state must be rejected")
		}
	})

	t.Run("nonce mismatch", func(t *testing.T) {
		idp := newAuthIdP(t)
		a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		_, state, err := a.BeginLogin(context.Background(), "acme", "/", "203.0.113.7")
		if err != nil {
			t.Fatal(err)
		}
		idp.setClaims(map[string]any{"nonce": "wrong", "groups": []string{"proxy-admins"}})
		_, _, err = a.CompleteLogin(context.Background(), state, "code")
		if err == nil || !strings.Contains(err.Error(), "nonce") {
			t.Fatalf("expected a nonce mismatch error, got %v", err)
		}
	})

	t.Run("no role mapping denies", func(t *testing.T) {
		idp := newAuthIdP(t)
		a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
		authURL, state, err := a.BeginLogin(context.Background(), "acme", "/", "203.0.113.7")
		if err != nil {
			t.Fatal(err)
		}
		idp.setClaims(map[string]any{"nonce": nonceOf(t, authURL), "groups": []string{"nobody"}})
		_, _, err = a.CompleteLogin(context.Background(), state, "code")
		if err == nil || !strings.Contains(err.Error(), "no role mapping") {
			t.Fatalf("expected a role-mapping denial, got %v", err)
		}
	})
}

// A non-default groupsClaim is honoured end to end.
func TestCompleteLoginCustomGroupsClaim(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{GroupsClaim: "roles", UserGroups: []string{"viewers"}})
	authURL, state, err := a.BeginLogin(context.Background(), "acme", "/", "203.0.113.7")
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
	sess, _, err := a.CompleteLogin(context.Background(), state, "code")
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
		name    string
		cfg     model.Config
		base    string
		idpName string
		wantErr string
	}{
		{
			name:    "unknown provider",
			cfg:     model.Config{},
			base:    "https://admin.example.test",
			idpName: "nope",
			wantErr: "not found",
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
			a.Configure(tc.cfg, model.Settings{ExternalBaseURL: tc.base})
			_, _, err := a.BeginLogin(context.Background(), tc.idpName, "/", "203.0.113.7")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tc.wantErr)
			}
			// Nothing was stored for a login that never started.
			a.pmu.Lock()
			n := len(a.pending)
			a.pmu.Unlock()
			if n != 0 {
				t.Fatalf("pending map should stay empty on failure, got %d entries", n)
			}
		})
	}
}

// --- pending-login bookkeeping --------------------------------------------

func TestPendingLoginGlobalCap(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})

	a.pmu.Lock()
	for i := 0; i < maxPendingLogins; i++ {
		a.pending[fmt.Sprintf("live-%d", i)] = pendingLogin{expires: time.Now().Add(time.Hour)}
	}
	a.pmu.Unlock()

	_, _, err := a.BeginLogin(context.Background(), "acme", "/", "203.0.113.7")
	if err == nil || !strings.Contains(err.Error(), "too many pending logins") {
		t.Fatalf("err = %v, want a pending-login cap error", err)
	}
}

// gcPendingLocked prunes expired states, so a map full of stale entries does not
// permanently wedge logins.
func TestPendingLoginGCReclaimsExpiredStates(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})

	a.pmu.Lock()
	for i := 0; i < maxPendingLogins; i++ {
		a.pending[fmt.Sprintf("stale-%d", i)] = pendingLogin{expires: time.Now().Add(-time.Minute)}
	}
	a.pmu.Unlock()

	if _, _, err := a.BeginLogin(context.Background(), "acme", "/", "203.0.113.7"); err != nil {
		t.Fatalf("expired states should be reclaimed, got %v", err)
	}
	a.pmu.Lock()
	n := len(a.pending)
	a.pmu.Unlock()
	if n != 1 {
		t.Fatalf("pending map should hold only the new state, got %d entries", n)
	}
}

func TestPendingLoginPerIPCap(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
	const key = "203.0.113.7"

	for i := 0; i < maxPendingPerIP; i++ {
		if _, _, err := a.BeginLogin(context.Background(), "acme", "/", key); err != nil {
			t.Fatalf("attempt %d should be allowed: %v", i+1, err)
		}
	}
	_, _, err := a.BeginLogin(context.Background(), "acme", "/", key)
	if err == nil || !strings.Contains(err.Error(), "too many login attempts") {
		t.Fatalf("err = %v, want a per-IP cap error", err)
	}
	// The cap is per client IP.
	if _, _, err := a.BeginLogin(context.Background(), "acme", "/", "198.51.100.9"); err != nil {
		t.Fatalf("another client must be unaffected: %v", err)
	}
}

// Retries against a down or misconfigured IdP must not burn the caller's per-IP
// budget: the attempt is only counted once a login state actually exists.
func TestFailedBeginLoginDoesNotBurnBudget(t *testing.T) {
	idp := newAuthIdP(t)
	a := oidcAuthenticator(t, idp, &model.RoleMapping{AdminGroups: []string{"proxy-admins"}})
	const key = "203.0.113.7"

	for i := 0; i < maxPendingPerIP*2; i++ {
		if _, _, err := a.BeginLogin(context.Background(), "unknown-idp", "/", key); err == nil {
			t.Fatal("expected the unknown provider to fail")
		}
	}
	if a.pendingLoginAtCap(key) {
		t.Fatal("failed login starts must not count against the per-IP budget")
	}
	if _, _, err := a.BeginLogin(context.Background(), "acme", "/", key); err != nil {
		t.Fatalf("a real login start should still be allowed: %v", err)
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

			rec := httptest.NewRecorder()
			a.SetLoginStateCookie(rec, r, "state-value")
			c := findCookie(t, rec.Result(), oidcStateCookie)
			if c.Value != "state-value" {
				t.Fatalf("value = %q", c.Value)
			}
			if c.Path != "/auth" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 600 {
				t.Fatalf("attributes: %+v", c)
			}
			if c.Secure != secure {
				t.Fatalf("Secure = %v, want %v", c.Secure, secure)
			}

			// The handler reads it straight back.
			req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
			req.AddCookie(c)
			if got := a.LoginStateCookie(req); got != "state-value" {
				t.Fatalf("LoginStateCookie = %q", got)
			}

			// Clearing expires it in place.
			rec2 := httptest.NewRecorder()
			a.ClearLoginStateCookie(rec2, r)
			cleared := findCookie(t, rec2.Result(), oidcStateCookie)
			if cleared.Value != "" || cleared.MaxAge != -1 || cleared.Path != "/auth" {
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
	if got := a.LoginStateCookie(httptest.NewRequest(http.MethodGet, "/auth/callback", nil)); got != "" {
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
		if a.LoginThrottled(key) {
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
