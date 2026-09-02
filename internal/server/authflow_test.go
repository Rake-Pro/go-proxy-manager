package server

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
	jose "github.com/go-jose/go-jose/v4"
	"golang.org/x/crypto/bcrypt"
)

// --- fake OIDC provider ----------------------------------------------------

// fakeIdP is a minimal OpenID Provider: discovery document, JWKS backed by a
// freshly generated RSA key, an authorize endpoint that records what the client
// asked for, and a token endpoint that mints a signed ID token from claims the
// test controls (nonce, groups, email, ...).
type fakeIdP struct {
	srv    *httptest.Server
	signer jose.Signer
	pub    jose.JSONWebKey

	mu     sync.Mutex
	claims map[string]any
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key"
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: kid, Algorithm: "RS256", Use: "sig"}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{
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
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		// A real IdP would authenticate the user and redirect back; the tests
		// drive the callback themselves, so just acknowledge.
		w.WriteHeader(http.StatusOK)
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

func (f *fakeIdP) issuer() string { return f.srv.URL }

// setClaims replaces the extra claims baked into the next minted ID token.
func (f *fakeIdP) setClaims(c map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claims = c
}

func (f *fakeIdP) signIDToken(t *testing.T) string {
	t.Helper()
	claims := map[string]any{
		"iss": f.srv.URL,
		"aud": "gpm-client",
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
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

// --- test harness ----------------------------------------------------------

type envOpts struct {
	secure       bool // an https externalBaseURL, which makes cookies Secure + __Host- named
	localUser    string
	localPass    string
	localEnabled bool
	ssoOnly      bool
	noProviders  bool
	roleMapping  *model.RoleMapping
	// localTOTP, when set, is the base32 TOTP secret for the local admin, which
	// turns local login into the two-step password-then-code flow.
	localTOTP string
}

type testEnv struct {
	t     *testing.T
	idp   *fakeIdP
	authn *auth.Authenticator
	srv   *Server
	sess  *session.Store
}

func newTestEnv(t *testing.T, o envOpts) *testEnv {
	t.Helper()
	idp := newFakeIdP(t)
	st, err := session.Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	base := "http://admin.example.test"
	if o.secure {
		base = "https://admin.example.test"
	}
	// SecureMode stays at its auto default: the https externalBaseURL above is
	// what makes o.secure produce Secure, __Host- named cookies.
	ao := auth.Options{Store: st, SessionTTL: time.Hour}
	if o.localUser != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(o.localPass), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		ao.LocalUser = o.localUser
		ao.LocalHash = string(hash)
		ao.LocalTOTPSecret = o.localTOTP
	}
	authn := auth.NewAuthenticator(ao)

	rm := o.roleMapping
	if rm == nil {
		rm = &model.RoleMapping{AdminGroups: []string{"proxy-admins"}, UserGroups: []string{"gpm-users"}}
	}
	cfg := model.Config{}
	var providers []string
	if !o.noProviders {
		cfg.IdentityProviders = []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "acme", DisplayName: "Acme SSO"},
			Type:        model.IdPTypeOIDC,
			OIDC:        &model.OIDCSpec{IssuerURL: idp.issuer(), ClientID: "gpm-client"},
			RoleMapping: rm,
		}}
		providers = []string{"acme"}
	}
	authn.Configure(cfg, model.Settings{
		AppName:         "GPM Test",
		ExternalBaseURL: base,
		AdminAuth: model.AdminAuthSettings{
			Providers:         providers,
			LocalLoginEnabled: o.localEnabled,
			SSOOnly:           o.ssoOnly,
		},
	})
	return &testEnv{t: t, idp: idp, authn: authn, srv: New(":0", nil, authn, nil, nil, nil, false), sess: st}
}

func (e *testEnv) do(req *http.Request) *http.Response {
	e.t.Helper()
	rec := httptest.NewRecorder()
	e.srv.http.Handler.ServeHTTP(rec, req)
	return rec.Result()
}

func (e *testEnv) get(target string, cookies ...*http.Cookie) *http.Response {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return e.do(req)
}

func (e *testEnv) postForm(target string, form url.Values, cookies ...*http.Cookie) *http.Response {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return e.do(req)
}

func cookie(t *testing.T, res *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func mustCookie(t *testing.T, res *http.Response, name string) *http.Cookie {
	t.Helper()
	c := cookie(t, res, name)
	if c == nil {
		t.Fatalf("expected cookie %q, got %v", name, res.Cookies())
	}
	return c
}

// beginLogin runs GET /auth/login?idp=acme and returns the state cookie plus the
// state and nonce the server sent to the IdP.
func (e *testEnv) beginLogin(returnTo string) (stateCookie *http.Cookie, state, nonce string) {
	e.t.Helper()
	target := "/auth/login?idp=acme"
	if returnTo != "" {
		target += "&return=" + url.QueryEscape(returnTo)
	}
	res := e.get(target)
	if res.StatusCode != http.StatusFound {
		e.t.Fatalf("begin login: got %d, want 302", res.StatusCode)
	}
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		e.t.Fatal(err)
	}
	if !strings.HasPrefix(loc.String(), e.idp.issuer()) {
		e.t.Fatalf("login should redirect to the IdP, got %q", loc)
	}
	q := loc.Query()
	return mustCookie(e.t, res, "gpm_oidc_state"), q.Get("state"), q.Get("nonce")
}

// callback plays the IdP redirect back into GET /auth/callback.
func (e *testEnv) callback(state, code string, cookies ...*http.Cookie) *http.Response {
	e.t.Helper()
	target := "/auth/callback?state=" + url.QueryEscape(state) + "&code=" + url.QueryEscape(code)
	return e.get(target, cookies...)
}

// --- happy path ------------------------------------------------------------

func TestOIDCLoginHappyPath(t *testing.T) {
	e := newTestEnv(t, envOpts{})

	stateCookie, state, nonce := e.beginLogin("/hosts")
	if state == "" || nonce == "" {
		t.Fatal("authorize URL must carry both state and nonce")
	}
	if stateCookie.Value != state {
		t.Fatalf("state cookie %q must equal the state sent to the IdP %q", stateCookie.Value, state)
	}
	if stateCookie.Path != "/auth" || !stateCookie.HttpOnly || stateCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("state cookie attributes: path=%q httpOnly=%v sameSite=%v", stateCookie.Path, stateCookie.HttpOnly, stateCookie.SameSite)
	}

	e.idp.setClaims(map[string]any{
		"nonce": nonce, "email": "admin@example.test", "name": "Admin",
		"groups": []string{"proxy-admins"},
	})
	res := e.callback(state, "auth-code", stateCookie)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("callback: got %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/hosts" {
		t.Fatalf("callback should honour the return path, got %q", loc)
	}
	// The state cookie is single use: the callback clears it.
	if c := cookie(t, res, "gpm_oidc_state"); c == nil || c.MaxAge != -1 {
		t.Fatalf("callback must clear the login-state cookie, got %+v", c)
	}
	sc := mustCookie(t, res, "gpm_session")
	if sc.Value == "" || !sc.HttpOnly || sc.Path != "/" {
		t.Fatalf("session cookie attributes: %+v", sc)
	}

	// /api/me reflects the mapped identity.
	me := e.get("/api/me", sc)
	if me.StatusCode != http.StatusOK {
		t.Fatalf("/api/me: got %d, want 200", me.StatusCode)
	}
	var body map[string]any
	raw := readBody(t, me)
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	if body["Subject"] != "user-123" || body["Email"] != "admin@example.test" {
		t.Fatalf("/api/me identity = %v", body)
	}
	if body["Role"] != "admin" || body["IdP"] != "acme" {
		t.Fatalf("/api/me role/idp = %v", body)
	}
	if tok, _ := body["csrfToken"].(string); tok == "" {
		t.Fatalf("/api/me must surface the CSRF token, got %v", body)
	}
	// The session id is the bearer credential: it must never be echoed back.
	if _, ok := body["SessionID"]; ok {
		t.Fatal("/api/me must not expose SessionID")
	}
	if strings.Contains(raw, sc.Value) {
		t.Fatalf("/api/me body leaks the session id: %s", raw)
	}
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- callback rejection paths ---------------------------------------------

func TestCallbackRejectsBadState(t *testing.T) {
	t.Run("missing state cookie", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		_, state, nonce := e.beginLogin("/")
		e.idp.setClaims(map[string]any{"nonce": nonce, "groups": []string{"proxy-admins"}})
		res := e.callback(state, "code") // no cookie
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", res.StatusCode)
		}
		if c := cookie(t, res, "gpm_session"); c != nil {
			t.Fatal("a rejected callback must not mint a session")
		}
	})

	t.Run("state cookie mismatch", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		_, state, nonce := e.beginLogin("/")
		e.idp.setClaims(map[string]any{"nonce": nonce, "groups": []string{"proxy-admins"}})
		// An attacker-injected callback carrying a server-valid state, but a
		// state cookie from a different (this browser's) flow.
		res := e.callback(state, "code", &http.Cookie{Name: "gpm_oidc_state", Value: "some-other-state"})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", res.StatusCode)
		}
	})

	t.Run("missing state or code", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		for _, target := range []string{"/auth/callback", "/auth/callback?state=x", "/auth/callback?code=y"} {
			if res := e.get(target); res.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s: got %d, want 400", target, res.StatusCode)
			}
		}
	})

	t.Run("idp reported error", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		res := e.get("/auth/callback?error=access_denied")
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", res.StatusCode)
		}
	})

	t.Run("unknown state is not in the pending map", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		res := e.callback("never-issued", "code", &http.Cookie{Name: "gpm_oidc_state", Value: "never-issued"})
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", res.StatusCode)
		}
	})

	t.Run("state cannot be replayed", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		sc, state, nonce := e.beginLogin("/")
		e.idp.setClaims(map[string]any{"nonce": nonce, "groups": []string{"proxy-admins"}})
		if res := e.callback(state, "code", sc); res.StatusCode != http.StatusFound {
			t.Fatalf("first callback: got %d, want 302", res.StatusCode)
		}
		res := e.callback(state, "code", sc)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("replayed state: got %d, want 401", res.StatusCode)
		}
	})
}

func TestCallbackRejectsNonceMismatch(t *testing.T) {
	e := newTestEnv(t, envOpts{})
	sc, state, _ := e.beginLogin("/")
	// The ID token echoes a nonce this flow never issued.
	e.idp.setClaims(map[string]any{"nonce": "not-the-issued-nonce", "groups": []string{"proxy-admins"}})
	res := e.callback(state, "code", sc)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", res.StatusCode)
	}
	if c := cookie(t, res, "gpm_session"); c != nil {
		t.Fatal("nonce mismatch must not mint a session")
	}
}

func TestCallbackRoleMapping(t *testing.T) {
	cases := []struct {
		name     string
		groups   []string
		wantCode int
		wantRole string
	}{
		{"admin group maps to admin", []string{"proxy-admins"}, http.StatusFound, "admin"},
		{"user group maps to user", []string{"gpm-users"}, http.StatusFound, "user"},
		{"unmapped group is denied", []string{"marketing"}, http.StatusUnauthorized, ""},
		{"no groups is denied", nil, http.StatusUnauthorized, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEnv(t, envOpts{})
			sc, state, nonce := e.beginLogin("/")
			e.idp.setClaims(map[string]any{"nonce": nonce, "groups": tc.groups})
			res := e.callback(state, "code", sc)
			if res.StatusCode != tc.wantCode {
				t.Fatalf("got %d, want %d", res.StatusCode, tc.wantCode)
			}
			if tc.wantRole == "" {
				if cookie(t, res, "gpm_session") != nil {
					t.Fatal("denied login must not mint a session")
				}
				return
			}
			me := e.get("/api/me", mustCookie(t, res, "gpm_session"))
			var body map[string]any
			if err := json.Unmarshal([]byte(readBody(t, me)), &body); err != nil {
				t.Fatal(err)
			}
			if body["Role"] != tc.wantRole {
				t.Fatalf("role = %v, want %s", body["Role"], tc.wantRole)
			}
		})
	}
}

// Open-redirect defense through the real handlers: a hostile ?return= must never
// survive to the callback's Location header.
func TestReturnToSanitizedThroughLoginFlow(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/hosts?tab=1", "/hosts?tab=1"},
		{"//evil.example", "/"},
		{`/\evil.example`, "/"},
		{`\/evil.example`, "/"},
		{"https://evil.example", "/"},
		{"relative", "/"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			e := newTestEnv(t, envOpts{})
			sc, state, nonce := e.beginLogin(tc.in)
			e.idp.setClaims(map[string]any{"nonce": nonce, "groups": []string{"proxy-admins"}})
			res := e.callback(state, "code", sc)
			if res.StatusCode != http.StatusFound {
				t.Fatalf("got %d, want 302", res.StatusCode)
			}
			if loc := res.Header.Get("Location"); loc != tc.want {
				t.Fatalf("Location = %q, want %q", loc, tc.want)
			}
		})
	}
}

func TestLocalLoginReturnToSanitized(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true})
	res := e.postForm("/auth/local", url.Values{
		"username": {"admin"}, "password": {"s3cret"}, "return": {`/\evil.example`},
	})
	if res.StatusCode != http.StatusFound {
		t.Fatalf("got %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/" {
		t.Fatalf("Location = %q, want /", loc)
	}
}

// --- login page ------------------------------------------------------------

func TestLoginPageRendering(t *testing.T) {
	t.Run("sso plus local shows both", func(t *testing.T) {
		e := newTestEnv(t, envOpts{localUser: "admin", localPass: "pw", localEnabled: true})
		res := e.get("/auth/login")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d, want 200", res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("Content-Type = %q", ct)
		}
		body := readBody(t, res)
		for _, want := range []string{"GPM Test", "Login with Acme SSO", `action="/auth/local"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("login page missing %q:\n%s", want, body)
			}
		}
	})

	t.Run("single sso provider auto-redirects", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		res := e.get("/auth/login")
		if res.StatusCode != http.StatusFound {
			t.Fatalf("got %d, want 302", res.StatusCode)
		}
		if !strings.HasPrefix(res.Header.Get("Location"), e.idp.issuer()) {
			t.Fatalf("Location = %q, want the IdP", res.Header.Get("Location"))
		}
	})

	t.Run("select=1 forces the chooser", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		res := e.get("/auth/login?select=1")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d, want 200", res.StatusCode)
		}
		if !strings.Contains(readBody(t, res), "Login with Acme SSO") {
			t.Fatal("chooser should list the provider")
		}
	})

	t.Run("no providers and no local", func(t *testing.T) {
		e := newTestEnv(t, envOpts{noProviders: true})
		res := e.get("/auth/login")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("got %d, want 200", res.StatusCode)
		}
		if !strings.Contains(readBody(t, res), "No login methods are available") {
			t.Fatal("expected the empty-state message")
		}
	})

	t.Run("sso-only hides the local form", func(t *testing.T) {
		e := newTestEnv(t, envOpts{localUser: "admin", localPass: "pw", localEnabled: true, ssoOnly: true})
		res := e.get("/auth/login?select=1")
		if strings.Contains(readBody(t, res), `action="/auth/local"`) {
			t.Fatal("SSO-only must not render the local login form")
		}
	})

	t.Run("unknown idp fails closed", func(t *testing.T) {
		e := newTestEnv(t, envOpts{})
		res := e.get("/auth/login?idp=nope")
		if res.StatusCode != http.StatusBadGateway {
			t.Fatalf("got %d, want 502", res.StatusCode)
		}
		if cookie(t, res, "gpm_oidc_state") != nil {
			t.Fatal("a failed login start must not set a state cookie")
		}
	})
}

// --- local login -----------------------------------------------------------

func TestLocalLoginFlow(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true})

	// Wrong password -> 401, no session.
	res := e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"nope"}})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: got %d, want 401", res.StatusCode)
	}
	if cookie(t, res, "gpm_session") != nil {
		t.Fatal("failed local login must not mint a session")
	}

	// Correct password -> 302 + session.
	res = e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"s3cret"}, "return": {"/certs"}})
	if res.StatusCode != http.StatusFound {
		t.Fatalf("got %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/certs" {
		t.Fatalf("Location = %q, want /certs", loc)
	}
	sc := mustCookie(t, res, "gpm_session")

	me := e.get("/api/me", sc)
	if me.StatusCode != http.StatusOK {
		t.Fatalf("/api/me: got %d, want 200", me.StatusCode)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(readBody(t, me)), &body); err != nil {
		t.Fatal(err)
	}
	if body["Role"] != "admin" || body["IdP"] != "local" || body["Subject"] != "admin" {
		t.Fatalf("/api/me = %v", body)
	}
}

func TestLocalLoginBlockedWhenSSOOnly(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true, ssoOnly: true})
	res := e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"s3cret"}})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("SSO-only must refuse local login: got %d, want 401", res.StatusCode)
	}
	if cookie(t, res, "gpm_session") != nil {
		t.Fatal("SSO-only local login must not mint a session")
	}
}

// Repeated failures lock the client out; once locked, even the CORRECT password
// is refused with 429 for the rest of the window.
func TestLocalLoginLockout(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true})
	const attempts = 5 // auth.maxLoginFails
	for i := 0; i < attempts; i++ {
		res := e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"wrong"}})
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d, want 401", i+1, res.StatusCode)
		}
	}
	res := e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"s3cret"}})
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("locked out: got %d, want 429", res.StatusCode)
	}
	if cookie(t, res, "gpm_session") != nil {
		t.Fatal("a throttled request must not mint a session")
	}

	// The lockout is per client IP: a different peer is unaffected.
	req := httptest.NewRequest(http.MethodPost, "/auth/local",
		strings.NewReader(url.Values{"username": {"admin"}, "password": {"s3cret"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.9:5000"
	if got := e.do(req).StatusCode; got != http.StatusFound {
		t.Fatalf("other client: got %d, want 302", got)
	}
}

// --- logout ----------------------------------------------------------------

func TestLogoutClearsCookieAndInvalidatesSession(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true})
	res := e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"s3cret"}})
	sc := mustCookie(t, res, "gpm_session")

	out := e.postForm("/auth/logout", nil, sc)
	if out.StatusCode != http.StatusFound {
		t.Fatalf("logout: got %d, want 302", out.StatusCode)
	}
	if loc := out.Header.Get("Location"); loc != "/auth/login?select=1" {
		t.Fatalf("logout Location = %q", loc)
	}
	cleared := mustCookie(t, out, "gpm_session")
	if cleared.Value != "" || cleared.MaxAge != -1 {
		t.Fatalf("logout must expire the session cookie, got %+v", cleared)
	}
	// The session is gone server-side too: replaying the old cookie fails.
	if got := e.get("/api/me", sc).StatusCode; got != http.StatusUnauthorized {
		t.Fatalf("replayed session after logout: got %d, want 401", got)
	}
}

// --- secure / __Host- cookies ---------------------------------------------

func TestSecureCookiesUseHostPrefix(t *testing.T) {
	e := newTestEnv(t, envOpts{secure: true, localUser: "admin", localPass: "s3cret", localEnabled: true})

	// Login-state cookie is Secure.
	stateCookie, state, nonce := e.beginLogin("/")
	if !stateCookie.Secure {
		t.Fatal("login-state cookie must be Secure when cookies are secure")
	}
	e.idp.setClaims(map[string]any{"nonce": nonce, "groups": []string{"proxy-admins"}})
	res := e.callback(state, "code", stateCookie)

	sc := mustCookie(t, res, "__Host-gpm_session")
	if !sc.Secure || sc.Path != "/" || sc.Domain != "" {
		t.Fatalf("__Host- cookie invariants violated: %+v", sc)
	}
	if cookie(t, res, "gpm_session") != nil {
		t.Fatal("the unprefixed cookie name must not also be set")
	}
	if got := e.get("/api/me", sc).StatusCode; got != http.StatusOK {
		t.Fatalf("/api/me with the __Host- cookie: got %d, want 200", got)
	}
}

// TestPlainHTTPFirstLoginWorks is the quick-start regression: over
// http://127.0.0.1:8081 with no externalBaseURL, the login POST must set a
// usable, non-Secure, bare-named cookie and the next request must be
// authenticated. A Secure-by-default cookie was silently dropped by the browser,
// so the first login of a fresh install appeared to do nothing.
func TestPlainHTTPFirstLoginWorks(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true, noProviders: true})

	req := httptest.NewRequest(http.MethodPost, "/auth/local",
		strings.NewReader(url.Values{"username": {"admin"}, "password": {"s3cret"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:54321"
	res := e.do(req)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("local login: got %d, want 302", res.StatusCode)
	}
	sc := mustCookie(t, res, "gpm_session")
	if sc.Secure {
		t.Fatalf("plain-HTTP login must issue a non-Secure cookie: %+v", sc)
	}
	if cookie(t, res, "__Host-gpm_session") != nil {
		t.Fatal("a non-Secure cookie must not take the __Host- prefix")
	}

	me := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	me.RemoteAddr = "127.0.0.1:54322"
	me.AddCookie(sc)
	if got := e.do(me).StatusCode; got != http.StatusOK {
		t.Fatalf("/api/me after a plain-HTTP login: got %d, want 200", got)
	}
}

// --- origin guard ----------------------------------------------------------

func TestOriginOK(t *testing.T) {
	cases := []struct {
		name     string
		secFetch string
		origin   string
		host     string
		want     bool
	}{
		{"sec-fetch same-origin", "same-origin", "", "admin.example.test", true},
		{"sec-fetch none (address bar)", "none", "", "admin.example.test", true},
		{"sec-fetch same-site sibling subdomain", "same-site", "", "admin.example.test", false},
		{"sec-fetch cross-site", "cross-site", "", "admin.example.test", false},
		{"sec-fetch wins over a matching origin", "cross-site", "http://admin.example.test", "admin.example.test", false},
		{"no headers at all (non-browser client)", "", "", "admin.example.test", true},
		{"origin matches host", "", "https://admin.example.test", "admin.example.test", true},
		{"origin case-insensitive host match", "", "https://ADMIN.example.test", "admin.example.test", true},
		{"origin host mismatch", "", "https://evil.example", "admin.example.test", false},
		{"origin sibling subdomain", "", "https://other.example.test", "admin.example.test", false},
		{"origin port mismatch", "", "https://admin.example.test:8443", "admin.example.test", false},
		{"origin null", "", "null", "admin.example.test", false},
		{"origin unparseable", "", "http://a b c/", "admin.example.test", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/auth/local", nil)
			r.Host = tc.host
			if tc.secFetch != "" {
				r.Header.Set("Sec-Fetch-Site", tc.secFetch)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := originOK(r); got != tc.want {
				t.Fatalf("originOK = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSameOriginGuardOnCredentialPosts(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true})
	for _, target := range []string{"/auth/local", "/auth/logout"} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, target,
				strings.NewReader(url.Values{"username": {"admin"}, "password": {"s3cret"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Host = "admin.example.test"
			res := e.do(req)
			if res.StatusCode != http.StatusForbidden {
				t.Fatalf("cross-site POST: got %d, want 403", res.StatusCode)
			}
			if cookie(t, res, "gpm_session") != nil {
				t.Fatal("a blocked cross-origin POST must not mint a session")
			}
		})
	}
	// GET /auth/login is a safe method and passes the guard regardless.
	req := httptest.NewRequest(http.MethodGet, "/auth/login?select=1", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if got := e.do(req).StatusCode; got != http.StatusOK {
		t.Fatalf("GET login with cross-site Sec-Fetch-Site: got %d, want 200", got)
	}
}

// --- throttle key ----------------------------------------------------------

// clientIPKey is deliberately the connection peer only: the admin plane is
// reached directly, so an X-Forwarded-For a client can set must never become the
// lockout key (that would let one attacker rotate the key per attempt).
func TestClientIPKey(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{"host:port", "203.0.113.7:44321", nil, "203.0.113.7"},
		{"ipv6 host:port", "[2001:db8::1]:44321", nil, "2001:db8::1"},
		{"no port falls back to the raw value", "203.0.113.7", nil, "203.0.113.7"},
		{"empty remote addr", "", nil, ""},
		{"x-forwarded-for is ignored", "203.0.113.7:44321",
			map[string]string{"X-Forwarded-For": "198.51.100.1", "X-Real-IP": "198.51.100.2"}, "203.0.113.7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/auth/local", nil)
			r.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := clientIPKey(r); got != tc.want {
				t.Fatalf("clientIPKey = %q, want %q", got, tc.want)
			}
		})
	}
}

// A spoofed X-Forwarded-For must not let a locked-out client keep guessing.
func TestLockoutIgnoresForwardedForRotation(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true})
	post := func(xff string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/local",
			strings.NewReader(url.Values{"username": {"admin"}, "password": {"wrong"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", xff)
		req.RemoteAddr = "203.0.113.7:1000"
		return e.do(req).StatusCode
	}
	for i := 0; i < 5; i++ {
		if got := post("10.0.0." + strconv.Itoa(i+1)); got != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d, want 401", i+1, got)
		}
	}
	if got := post("10.0.0.99"); got != http.StatusTooManyRequests {
		t.Fatalf("rotating X-Forwarded-For must not dodge the lockout: got %d, want 429", got)
	}
}

// --- /api/me gate ----------------------------------------------------------

func TestMeRequiresSession(t *testing.T) {
	e := newTestEnv(t, envOpts{})
	if got := e.get("/api/me").StatusCode; got != http.StatusUnauthorized {
		t.Fatalf("anonymous /api/me: got %d, want 401", got)
	}
	if got := e.get("/api/me", &http.Cookie{Name: "gpm_session", Value: "bogus"}).StatusCode; got != http.StatusUnauthorized {
		t.Fatalf("bogus session /api/me: got %d, want 401", got)
	}
}

func TestLocalLoginRejectsUnparseableForm(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true})
	req := httptest.NewRequest(http.MethodPost, "/auth/local", strings.NewReader("username=admin&password=%zz"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := e.do(req)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", res.StatusCode)
	}
}
