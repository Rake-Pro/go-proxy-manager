package dataplane

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// testOP is a strict OpenID provider for the end-to-end walk in
// oidcwalk_test.go. Unlike newMockIdP it behaves like a real provider: it has
// an authorize endpoint with a login page and a session of its own, it checks
// the client, the exact redirect URI, the scopes and PKCE, its codes are single
// use and bound to the client and the redirect URI, and its ID tokens carry
// only the claims the granted scopes release. A walk that passes against it
// proves gpm sends what a real provider demands, not only that gpm accepts
// what a lax mock hands it.

// opSigningKey signs every provider's ID tokens and opForeignKey is the "other
// key" a tampering test signs with. Generated once per test binary: RSA key
// generation dominates the cost of a provider otherwise.
var (
	opSigningKey = sync.OnceValue(func() *rsa.PrivateKey { return opNewKey() })
	opForeignKey = sync.OnceValue(func() *rsa.PrivateKey { return opNewKey() })
)

func opNewKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("testOP: cannot generate an RSA key: " + err.Error())
	}
	return k
}

const (
	opKeyID = "op-key-1"
	// opRequestHeader carries the pending authorization request id on the login
	// page, so the test browser can submit the login without parsing HTML.
	opRequestHeader = "X-Test-OP-Request"
	opSessionCookie = "op_session"
	opCodeTTL       = 60 * time.Second
)

// opClient is a registered relying party. An empty Secret makes it a public
// client, which authenticates with its id alone.
type opClient struct {
	ID           string
	Secret       string
	RedirectURIs []string
	RequirePKCE  bool
}

// opUser is an account at the provider.
type opUser struct {
	Login         string
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
	Groups        []string
}

// opIDToken is what an ID token hook may change before the provider signs:
// the claims, the signing key, or whether an id_token is returned at all.
type opIDToken struct {
	Claims map[string]any
	Key    *rsa.PrivateKey
	Omit   bool
}

// opAuthRecord is one authorization request as the provider saw it; Error is
// the error it answered with, "" when it accepted the request.
type opAuthRecord struct {
	Params url.Values
	Error  string
}

// opTokenRecord is one token request: the client it named, how it
// authenticated ("basic", "post", or "none" for a public client), whether a
// PKCE verifier was checked against a stored challenge, and the error.
type opTokenRecord struct {
	ClientID     string
	AuthStyle    string
	PKCEVerified bool
	Error        string
}

type opAuthRequest struct {
	clientID    string
	redirectURI string
	scopes      []string
	state       string
	nonce       string
	challenge   string
}

type opCode struct {
	req    *opAuthRequest
	login  string
	issued time.Time
	used   bool
}

type testOP struct {
	srv    *httptest.Server
	Issuer string

	mu      sync.Mutex
	clients map[string]*opClient
	users   map[string]*opUser

	// Switches a test sets (through set) before it starts a flow.
	scopes              []string // the scopes the provider defines
	refuseUnknownScopes bool     // false: an unknown scope is ignored
	groupsScope         string   // the scope that releases the groups claim
	down                bool     // discovery answers 500
	idTokenHook         func(*opIDToken)
	denyDescription     string // error_description sent with access_denied

	pending     map[string]*opAuthRequest
	codes       map[string]*opCode
	sessions    map[string]string // provider session id -> login
	authLog     []opAuthRecord
	tokenLog    []opTokenRecord
	discoveries int
	logins      int
}

func newTestOP(t *testing.T) *testOP {
	t.Helper()
	op := &testOP{
		clients:     map[string]*opClient{},
		users:       map[string]*opUser{},
		scopes:      []string{"openid", "profile", "email", "groups", "offline_access"},
		groupsScope: "groups",
		pending:     map[string]*opAuthRequest{},
		codes:       map[string]*opCode{},
		sessions:    map[string]string{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", op.discovery)
	mux.HandleFunc("/jwks", op.jwks)
	mux.HandleFunc("/authorize", op.authorize)
	mux.HandleFunc("/login", op.login)
	mux.HandleFunc("/logout", op.logout)
	mux.HandleFunc("/token", op.token)
	op.srv = httptest.NewServer(mux)
	op.Issuer = op.srv.URL
	t.Cleanup(op.srv.Close)
	return op
}

// set changes the provider's switches or registrations under its lock.
func (op *testOP) set(f func(*testOP)) {
	op.mu.Lock()
	defer op.mu.Unlock()
	f(op)
}

func (op *testOP) addClient(c opClient) { op.set(func(o *testOP) { o.clients[c.ID] = &c }) }
func (op *testOP) addUser(u opUser)     { op.set(func(o *testOP) { o.users[u.Login] = &u }) }

// register adds redirect URIs to a client.
func (op *testOP) register(clientID string, uris ...string) {
	op.set(func(o *testOP) { o.clients[clientID].RedirectURIs = append(o.clients[clientID].RedirectURIs, uris...) })
}

func (op *testOP) authRequests() []opAuthRecord {
	op.mu.Lock()
	defer op.mu.Unlock()
	return slices.Clone(op.authLog)
}

func (op *testOP) tokenRequests() []opTokenRecord {
	op.mu.Lock()
	defer op.mu.Unlock()
	return slices.Clone(op.tokenLog)
}

// lastAuth and lastToken return the latest record, failing the test when
// there is none.
func (op *testOP) lastAuth(t *testing.T) opAuthRecord {
	t.Helper()
	recs := op.authRequests()
	if len(recs) == 0 {
		t.Fatal("the provider saw no authorization request")
	}
	return recs[len(recs)-1]
}

func (op *testOP) lastToken(t *testing.T) opTokenRecord {
	t.Helper()
	recs := op.tokenRequests()
	if len(recs) == 0 {
		t.Fatal("the provider saw no token request")
	}
	return recs[len(recs)-1]
}

// counts returns how many discoveries, authorization requests, token requests
// and successful logins the provider has served.
func (op *testOP) counts() (discoveries, authorize, token, logins int) {
	op.mu.Lock()
	defer op.mu.Unlock()
	return op.discoveries, len(op.authLog), len(op.tokenLog), op.logins
}

func opRandom() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (op *testOP) discovery(w http.ResponseWriter, _ *http.Request) {
	op.mu.Lock()
	op.discoveries++
	down := op.down
	scopes := slices.Clone(op.scopes)
	op.mu.Unlock()
	if down {
		http.Error(w, "provider unavailable", http.StatusInternalServerError)
		return
	}
	iss := op.Issuer
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                iss,
		"authorization_endpoint":                iss + "/authorize",
		"token_endpoint":                        iss + "/token",
		"jwks_uri":                              iss + "/jwks",
		"end_session_endpoint":                  iss + "/logout",
		"scopes_supported":                      scopes,
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
	})
}

func (op *testOP) jwks(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &opSigningKey().PublicKey, KeyID: opKeyID, Algorithm: "RS256", Use: "sig",
	}}})
}

// authorize validates an authorization request. A request it cannot trust to
// redirect (unknown client, a redirect URI that is not an exact registered
// match) is answered here with 400 and never redirected; any other error goes
// back to the redirect URI, as real providers do.
func (op *testOP) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	op.mu.Lock()
	defer op.mu.Unlock()
	rec := opAuthRecord{Params: q}
	defer func() { op.authLog = append(op.authLog, rec) }()

	cl, ok := op.clients[q.Get("client_id")]
	if !ok {
		rec.Error = "unknown client"
		http.Error(w, "unknown client_id", http.StatusBadRequest)
		return
	}
	redirectURI := q.Get("redirect_uri")
	if !slices.Contains(cl.RedirectURIs, redirectURI) {
		rec.Error = "unregistered redirect_uri"
		http.Error(w, "redirect_uri is not registered for this client", http.StatusBadRequest)
		return
	}
	state := q.Get("state")
	fail := func(code, desc string) {
		rec.Error = code
		v := url.Values{"error": {code}, "error_description": {desc}}
		if state != "" {
			v.Set("state", state)
		}
		http.Redirect(w, r, redirectURI+"?"+v.Encode(), http.StatusFound)
	}
	if q.Get("response_type") != "code" {
		fail("unsupported_response_type", "only response_type=code is supported")
		return
	}
	requested := strings.Fields(q.Get("scope"))
	if !slices.Contains(requested, "openid") {
		fail("invalid_scope", "the openid scope is required")
		return
	}
	var granted []string
	for _, s := range requested {
		switch {
		case slices.Contains(op.scopes, s):
			granted = append(granted, s)
		case op.refuseUnknownScopes:
			fail("invalid_scope", fmt.Sprintf("scope %q is not supported", s))
			return
		}
	}
	challenge := q.Get("code_challenge")
	if challenge != "" && q.Get("code_challenge_method") != "S256" {
		fail("invalid_request", "only the S256 code challenge method is supported")
		return
	}
	if cl.RequirePKCE && challenge == "" {
		fail("invalid_request", "this client must send a PKCE code challenge")
		return
	}
	req := &opAuthRequest{
		clientID: cl.ID, redirectURI: redirectURI, scopes: granted,
		state: state, nonce: q.Get("nonce"), challenge: challenge,
	}
	// A live provider session re-authenticates silently.
	if c, err := r.Cookie(opSessionCookie); err == nil {
		if login, ok := op.sessions[c.Value]; ok {
			op.issueCodeLocked(w, r, req, login)
			return
		}
	}
	id := opRandom()
	op.pending[id] = req
	w.Header().Set(opRequestHeader, id)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<form method="post" action="/login"><input type="hidden" name="request" value="%s">`+
		`<input name="login"><button>Sign in</button><button name="deny" value="1">Deny</button></form>`, html.EscapeString(id))
}

func (op *testOP) issueCodeLocked(w http.ResponseWriter, r *http.Request, req *opAuthRequest, login string) {
	code := opRandom()
	op.codes[code] = &opCode{req: req, login: login, issued: time.Now()}
	v := url.Values{"code": {code}}
	if req.state != "" {
		v.Set("state", req.state)
	}
	http.Redirect(w, r, req.redirectURI+"?"+v.Encode(), http.StatusFound)
}

// login answers the login form: an unknown user is a 401 on the provider, a
// denial goes back with access_denied, a success starts a provider session and
// goes back with a code.
func (op *testOP) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	op.mu.Lock()
	defer op.mu.Unlock()
	id := r.PostForm.Get("request")
	req, ok := op.pending[id]
	if !ok {
		http.Error(w, "unknown or finished authorization request", http.StatusBadRequest)
		return
	}
	if r.PostForm.Get("deny") != "" {
		delete(op.pending, id)
		v := url.Values{"error": {"access_denied"}}
		if op.denyDescription != "" {
			v.Set("error_description", op.denyDescription)
		}
		if req.state != "" {
			v.Set("state", req.state)
		}
		http.Redirect(w, r, req.redirectURI+"?"+v.Encode(), http.StatusFound)
		return
	}
	login := r.PostForm.Get("login")
	if _, ok := op.users[login]; !ok {
		http.Error(w, "unknown user", http.StatusUnauthorized)
		return
	}
	delete(op.pending, id)
	op.logins++
	sid := opRandom()
	op.sessions[sid] = login
	http.SetCookie(w, &http.Cookie{Name: opSessionCookie, Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	op.issueCodeLocked(w, r, req, login)
}

func (op *testOP) logout(w http.ResponseWriter, r *http.Request) {
	op.mu.Lock()
	if c, err := r.Cookie(opSessionCookie); err == nil {
		delete(op.sessions, c.Value)
	}
	op.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: opSessionCookie, Value: "", Path: "/", MaxAge: -1})
	_, _ = w.Write([]byte("signed out"))
}

// token redeems a code. Client authentication by HTTP Basic or by form fields;
// the code must exist, belong to the client, be unused and fresh; the
// redirect_uri must be the one of the authorization request; a stored code
// challenge must be met by the verifier.
func (op *testOP) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	op.mu.Lock()
	defer op.mu.Unlock()
	rec := opTokenRecord{}
	defer func() { op.tokenLog = append(op.tokenLog, rec) }()
	fail := func(status int, code, desc string) {
		rec.Error = code
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": desc})
	}

	id, secret, basic := r.BasicAuth()
	if basic {
		rec.AuthStyle = "basic"
		// RFC 6749 2.3.1: both parts are form-urlencoded inside the header.
		id, _ = url.QueryUnescape(id)
		secret, _ = url.QueryUnescape(secret)
		if r.PostForm.Get("client_secret") != "" {
			fail(http.StatusBadRequest, "invalid_request", "more than one client authentication method")
			return
		}
	} else {
		rec.AuthStyle = "post"
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	rec.ClientID = id
	cl, ok := op.clients[id]
	if !ok || subtle.ConstantTimeCompare([]byte(secret), []byte(cl.Secret)) != 1 {
		fail(http.StatusUnauthorized, "invalid_client", "client authentication failed")
		return
	}
	if cl.Secret == "" {
		rec.AuthStyle = "none"
	}
	if g := r.PostForm.Get("grant_type"); g != "authorization_code" {
		fail(http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported")
		return
	}
	c, ok := op.codes[r.PostForm.Get("code")]
	if !ok || c.req.clientID != cl.ID {
		fail(http.StatusBadRequest, "invalid_grant", "unknown code")
		return
	}
	if c.used {
		fail(http.StatusBadRequest, "invalid_grant", "code already used")
		return
	}
	c.used = true
	if time.Since(c.issued) > opCodeTTL {
		fail(http.StatusBadRequest, "invalid_grant", "code expired")
		return
	}
	if r.PostForm.Get("redirect_uri") != c.req.redirectURI {
		fail(http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	}
	if c.req.challenge != "" {
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if r.PostForm.Get("code_verifier") == "" || base64.RawURLEncoding.EncodeToString(sum[:]) != c.req.challenge {
			fail(http.StatusBadRequest, "invalid_grant", "code_verifier does not match the code challenge")
			return
		}
		rec.PKCEVerified = true
	}

	u := op.users[c.login]
	now := time.Now()
	claims := map[string]any{
		"iss": op.Issuer, "sub": u.Sub, "aud": cl.ID,
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(),
	}
	if c.req.nonce != "" {
		claims["nonce"] = c.req.nonce
	}
	if slices.Contains(c.req.scopes, "email") {
		claims["email"] = u.Email
		claims["email_verified"] = u.EmailVerified
	}
	if slices.Contains(c.req.scopes, "profile") {
		claims["name"] = u.Name
		claims["preferred_username"] = u.Login
	}
	if slices.Contains(c.req.scopes, op.groupsScope) {
		claims["groups"] = slices.Clone(u.Groups)
	}
	tok := &opIDToken{Claims: claims, Key: opSigningKey()}
	if op.idTokenHook != nil {
		op.idTokenHook(tok)
	}
	resp := map[string]any{"access_token": opRandom(), "token_type": "Bearer", "expires_in": 300}
	if !tok.Omit {
		signed, err := opSign(tok)
		if err != nil {
			fail(http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		resp["id_token"] = signed
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

func opSign(tok *opIDToken) (string, error) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256,
		Key: jose.JSONWebKey{Key: tok.Key, KeyID: opKeyID, Algorithm: "RS256", Use: "sig"}},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(tok.Claims)
	if err != nil {
		return "", err
	}
	obj, err := signer.Sign(b)
	if err != nil {
		return "", err
	}
	return obj.CompactSerialize()
}

// --- the provider's own strictness ------------------------------------------

const (
	opTestClient   = "rp-1"
	opTestSecret   = "rp-1-secret"
	opTestRedirect = "https://app.example.com/__gpm/oidc/callback"
)

// opTestProvider is a provider with one confidential PKCE client and one user.
func opTestProvider(t *testing.T) *testOP {
	t.Helper()
	op := newTestOP(t)
	op.addClient(opClient{ID: opTestClient, Secret: opTestSecret, RedirectURIs: []string{opTestRedirect}, RequirePKCE: true})
	op.addClient(opClient{ID: "rp-2", Secret: "rp-2-secret", RedirectURIs: []string{opTestRedirect}})
	op.addUser(opUser{Login: "user1", Sub: "sub-user1", Email: "user1@example.com", EmailVerified: true, Name: "User One", Groups: []string{"staff"}})
	return op
}

// opNoFollow is a client that keeps cookies and stops at every redirect.
func opNoFollow(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func opAuthParams(clientID, verifier string) url.Values {
	sum := sha256.Sum256([]byte(verifier))
	return url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {opTestRedirect},
		"scope": {"openid email"}, "state": {"st-1"}, "nonce": {"nonce-1"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}
}

// opGetCode runs authorize and the login form and returns the code.
func opGetCode(t *testing.T, op *testOP, c *http.Client, params url.Values) string {
	t.Helper()
	res, err := c.Get(op.Issuer + "/authorize?" + params.Encode())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get(opRequestHeader) == "" {
		t.Fatalf("authorize: got %d, want the login page", res.StatusCode)
	}
	res, err = c.PostForm(op.Issuer+"/login", url.Values{"request": {res.Header.Get(opRequestHeader)}, "login": {"user1"}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil || res.StatusCode != http.StatusFound || loc.Query().Get("code") == "" {
		t.Fatalf("login: got %d %q, want a redirect with a code", res.StatusCode, res.Header.Get("Location"))
	}
	if loc.Query().Get("state") != params.Get("state") {
		t.Fatalf("login: state %q, want %q", loc.Query().Get("state"), params.Get("state"))
	}
	return loc.Query().Get("code")
}

// opRedeem posts a token request; basic selects HTTP Basic client
// authentication. It returns the status and the decoded JSON body.
func opRedeem(t *testing.T, op *testOP, form url.Values, id, secret string, basic bool) (int, map[string]any) {
	t.Helper()
	if !basic {
		form.Set("client_id", id)
		form.Set("client_secret", secret)
	}
	req, err := http.NewRequest(http.MethodPost, op.Issuer+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basic {
		req.SetBasicAuth(url.QueryEscape(id), url.QueryEscape(secret))
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body
}

// TestOIDCProviderRefusesUntrustedAuthorizeRequests: an unknown client or a
// redirect URI that is not an exact registered match is answered on the
// provider with 400 and never redirected, so the walk's "ends on the
// provider's 400" means the redirect URI gpm sent was wrong. Every other
// defect goes back to the redirect URI with an error and the state.
func TestOIDCProviderRefusesUntrustedAuthorizeRequests(t *testing.T) {
	op := opTestProvider(t)
	c := opNoFollow(t)
	for name, edit := range map[string]func(url.Values){
		"unknown client":               func(v url.Values) { v.Set("client_id", "nobody") },
		"trailing slash":               func(v url.Values) { v.Set("redirect_uri", opTestRedirect+"/") },
		"with a port":                  func(v url.Values) { v.Set("redirect_uri", "https://app.example.com:8443/__gpm/oidc/callback") },
		"other domain":                 func(v url.Values) { v.Set("redirect_uri", "https://other.example.com/__gpm/oidc/callback") },
		"http instead of https":        func(v url.Values) { v.Set("redirect_uri", "http://app.example.com/__gpm/oidc/callback") },
		"extra query on the callback":  func(v url.Values) { v.Set("redirect_uri", opTestRedirect+"?x=1") },
		"redirect_uri missing at all":  func(v url.Values) { v.Del("redirect_uri") },
		"case differs in the hostname": func(v url.Values) { v.Set("redirect_uri", "https://App.example.com/__gpm/oidc/callback") },
	} {
		v := opAuthParams(opTestClient, "verifier-0123456789-0123456789-0123456789")
		edit(v)
		res, err := c.Get(op.Issuer + "/authorize?" + v.Encode())
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest || res.Header.Get("Location") != "" {
			t.Errorf("%s: got %d Location %q, want 400 and no redirect", name, res.StatusCode, res.Header.Get("Location"))
		}
	}
	op.set(func(o *testOP) { o.refuseUnknownScopes = true })
	for name, tc := range map[string]struct {
		edit func(url.Values)
		want string
	}{
		"response_type token":         {func(v url.Values) { v.Set("response_type", "token") }, "unsupported_response_type"},
		"no openid scope":             {func(v url.Values) { v.Set("scope", "email") }, "invalid_scope"},
		"unknown scope, refuse":       {func(v url.Values) { v.Set("scope", "openid frobnicate") }, "invalid_scope"},
		"PKCE required, no challenge": {func(v url.Values) { v.Del("code_challenge"); v.Del("code_challenge_method") }, "invalid_request"},
		"plain challenge method":      {func(v url.Values) { v.Set("code_challenge_method", "plain") }, "invalid_request"},
	} {
		v := opAuthParams(opTestClient, "verifier-0123456789-0123456789-0123456789")
		tc.edit(v)
		res, err := c.Get(op.Issuer + "/authorize?" + v.Encode())
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		loc, _ := url.Parse(res.Header.Get("Location"))
		if res.StatusCode != http.StatusFound || loc == nil || loc.Query().Get("error") != tc.want || loc.Query().Get("state") != "st-1" {
			t.Errorf("%s: got %d %q, want a redirect with error=%s and the state", name, res.StatusCode, res.Header.Get("Location"), tc.want)
		}
	}
}

// TestOIDCProviderTokenEndpointIsStrict: a code is redeemed once, by its own
// client with the right secret, for the redirect URI it was issued to, and
// with the verifier of its challenge. Each of these is what makes a walk step
// that passes against this provider mean that gpm got it right.
func TestOIDCProviderTokenEndpointIsStrict(t *testing.T) {
	op := opTestProvider(t)
	const verifier = "verifier-0123456789-0123456789-0123456789"
	form := func(code string) url.Values {
		return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {opTestRedirect}, "code_verifier": {verifier}}
	}
	newCode := func() string { return opGetCode(t, op, opNoFollow(t), opAuthParams(opTestClient, verifier)) }

	cases := []struct {
		name   string
		edit   func(url.Values)
		id     string
		secret string
		status int
		err    string
	}{
		{"wrong secret", nil, opTestClient, "wrong", http.StatusUnauthorized, "invalid_client"},
		{"unknown client", nil, "nobody", "x", http.StatusUnauthorized, "invalid_client"},
		{"another client's code", nil, "rp-2", "rp-2-secret", http.StatusBadRequest, "invalid_grant"},
		{"redirect_uri mismatch", func(v url.Values) { v.Set("redirect_uri", "https://other.example.com/__gpm/oidc/callback") }, opTestClient, opTestSecret, http.StatusBadRequest, "invalid_grant"},
		{"redirect_uri missing", func(v url.Values) { v.Del("redirect_uri") }, opTestClient, opTestSecret, http.StatusBadRequest, "invalid_grant"},
		{"wrong verifier", func(v url.Values) { v.Set("code_verifier", verifier+"x") }, opTestClient, opTestSecret, http.StatusBadRequest, "invalid_grant"},
		{"no verifier", func(v url.Values) { v.Del("code_verifier") }, opTestClient, opTestSecret, http.StatusBadRequest, "invalid_grant"},
		{"wrong grant type", func(v url.Values) { v.Set("grant_type", "password") }, opTestClient, opTestSecret, http.StatusBadRequest, "unsupported_grant_type"},
	}
	for _, tc := range cases {
		for _, basic := range []bool{true, false} {
			f := form(newCode())
			if tc.edit != nil {
				tc.edit(f)
			}
			status, body := opRedeem(t, op, f, tc.id, tc.secret, basic)
			if status != tc.status || body["error"] != tc.err || body["id_token"] != nil {
				t.Errorf("%s (basic=%v): got %d %v, want %d %s", tc.name, basic, status, body, tc.status, tc.err)
			}
		}
	}

	// A good redemption works once, in either authentication style, and the
	// ID token verifies against the JWKS and carries what the scopes release.
	for _, basic := range []bool{true, false} {
		f := form(newCode())
		status, body := opRedeem(t, op, f, opTestClient, opTestSecret, basic)
		if status != http.StatusOK {
			t.Fatalf("good redemption (basic=%v): got %d %v", basic, status, body)
		}
		raw, _ := body["id_token"].(string)
		jws, err := jose.ParseSignedCompact(raw, []jose.SignatureAlgorithm{jose.RS256})
		if err != nil {
			t.Fatal(err)
		}
		payload, err := jws.Verify(&opSigningKey().PublicKey)
		if err != nil {
			t.Fatalf("id_token does not verify with the published key: %v", err)
		}
		var claims map[string]any
		_ = json.Unmarshal(payload, &claims)
		if claims["iss"] != op.Issuer || claims["aud"] != opTestClient || claims["sub"] != "sub-user1" ||
			claims["nonce"] != "nonce-1" || claims["email"] != "user1@example.com" || claims["email_verified"] != true {
			t.Fatalf("id_token claims: %v", claims)
		}
		// Only openid and email were granted: no profile or groups claims.
		if _, ok := claims["groups"]; ok {
			t.Fatalf("groups released without the groups scope: %v", claims)
		}
		if _, ok := claims["name"]; ok {
			t.Fatalf("name released without the profile scope: %v", claims)
		}
		if status, body := opRedeem(t, op, f, opTestClient, opTestSecret, basic); status != http.StatusBadRequest || body["error"] != "invalid_grant" {
			t.Fatalf("code reuse (basic=%v): got %d %v, want 400 invalid_grant", basic, status, body)
		}
	}
	recs := op.tokenRequests()
	last := recs[len(recs)-1]
	if last.Error != "invalid_grant" || last.AuthStyle != "post" {
		t.Fatalf("token record: %+v", last)
	}
	if ok := recs[len(recs)-2]; !ok.PKCEVerified || ok.Error != "" {
		t.Fatalf("a good redemption must be recorded as PKCE-verified: %+v", ok)
	}
}

// TestOIDCProviderSilentReauthAndLogout: with a live provider session the
// authorize endpoint answers with a code straight away and no login page;
// after logout it shows the login page again. The walk's session tests rely
// on telling these two apart.
func TestOIDCProviderSilentReauthAndLogout(t *testing.T) {
	op := opTestProvider(t)
	c := opNoFollow(t)
	params := opAuthParams(opTestClient, "verifier-0123456789-0123456789-0123456789")
	opGetCode(t, op, c, params)
	res, err := c.Get(op.Issuer + "/authorize?" + params.Encode())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if loc, _ := url.Parse(res.Header.Get("Location")); res.StatusCode != http.StatusFound || loc.Query().Get("code") == "" {
		t.Fatalf("with a provider session: got %d %q, want a redirect with a code", res.StatusCode, res.Header.Get("Location"))
	}
	if res, err = c.Get(op.Issuer + "/logout"); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res, err = c.Get(op.Issuer + "/authorize?" + params.Encode()); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get(opRequestHeader) == "" {
		t.Fatalf("after logout: got %d, want the login page", res.StatusCode)
	}
	// An unknown user is refused on the provider itself.
	res, err = c.PostForm(op.Issuer+"/login", url.Values{"request": {res.Header.Get(opRequestHeader)}, "login": {"nobody"}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown user: got %d, want 401", res.StatusCode)
	}
}
