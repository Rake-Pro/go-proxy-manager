package dataplane

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	jose "github.com/go-jose/go-jose/v4"
)

func TestSSOTokenRoundTrip(t *testing.T) {
	payload := []byte(`{"sub":"admin"}`)
	tok := signToken(payload)
	got, ok := verifyToken(tok)
	if !ok || string(got) != string(payload) {
		t.Fatalf("round trip failed: ok=%v got=%q", ok, got)
	}
	// Tampering with the payload is rejected.
	if _, ok := verifyToken("ZmFrZQ." + tok[len(tok)-10:]); ok {
		t.Fatal("tampered token must not verify")
	}
}

func TestSanitizeSSOReturn(t *testing.T) {
	cases := map[string]string{
		"/dash?x=1":  "/dash?x=1",
		"":           "/",
		"relative":   "/",
		"//evil.com": "/",
		`/\evil`:     "/",
		"/ok\nx":     "/",
	}
	for in, want := range cases {
		if got := sanitizeSSOReturn(in); got != want {
			t.Errorf("sanitizeSSOReturn(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDataOIDCAuthorized(t *testing.T) {
	// No mapping, no required roles: any authenticated user passes.
	open := &dataOIDC{}
	if !open.authorized(nil) {
		t.Fatal("login-only gate must admit any authenticated user")
	}
	// With a role mapping, group membership is enforced.
	gated := &dataOIDC{roleMapping: &model.RoleMapping{AdminGroups: []string{"proxy-admins"}}}
	if gated.authorized([]string{"other"}) {
		t.Fatal("user without the mapped group must be denied")
	}
	if !gated.authorized([]string{"proxy-admins"}) {
		t.Fatal("user in the mapped admin group must be allowed")
	}
}

// --- end-to-end gate flow against a mock IdP -------------------------------

type mockIdP struct {
	server *httptest.Server
	signer jose.Signer
	pubJWK jose.JSONWebKey
	claims map[string]any
}

func newMockIdP(t *testing.T) *mockIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "k1"
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256,
		Key: jose.JSONWebKey{Key: key, KeyID: kid, Algorithm: "RS256", Use: "sig"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := &mockIdP{signer: signer, pubJWK: jose.JSONWebKey{Key: &key.PublicKey, KeyID: kid, Algorithm: "RS256", Use: "sig"}}
	mux := http.NewServeMux()
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	issuer := m.server.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
			"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{m.pubJWK}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "x", "token_type": "Bearer", "expires_in": 3600,
			"id_token": m.signIDToken(t),
		})
	})
	return m
}

func (m *mockIdP) signIDToken(t *testing.T) string {
	t.Helper()
	claims := map[string]any{
		"iss": m.server.URL, "aud": "gpm-client", "sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}
	for k, v := range m.claims {
		claims[k] = v
	}
	b, _ := json.Marshal(claims)
	obj, err := m.signer.Sign(b)
	if err != nil {
		t.Fatal(err)
	}
	s, err := obj.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func cookieByName(res *http.Response, name string) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestDataOIDCEndToEnd(t *testing.T) {
	idpSrv := newMockIdP(t)
	gate, err := compileDataOIDC(model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: "idp"},
		Type:       model.IdPTypeOIDC,
		OIDC:       &model.OIDCSpec{IssuerURL: idpSrv.server.URL, ClientID: "gpm-client"},
	}, nil, "app")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	h := gate.handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("upstream:" + r.Header.Get("X-Forwarded-Email")))
	}))

	// 1. Unauthenticated request -> redirect to the IdP + a state cookie.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://app2.example.com/dash", nil)
	req.Host = "app2.example.com"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("begin: got %d, want 302 (body %q)", rec.Code, rec.Body.String())
	}
	stateCookie := cookieByName(rec.Result(), oidcStateCookie)
	if stateCookie == nil {
		t.Fatal("begin must set a state cookie")
	}
	payload, ok := verifyToken(stateCookie.Value)
	if !ok {
		t.Fatal("state cookie must be a valid signed token")
	}
	var st oidcLoginState
	if err := json.Unmarshal(payload, &st); err != nil {
		t.Fatal(err)
	}

	// The id_token the IdP will mint must echo the gpm-issued nonce.
	idpSrv.claims = map[string]any{"nonce": st.Nonce, "email": "admin@example.com", "groups": []string{"team"}}

	// 2. IdP redirects back to the callback with code+state.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "https://app2.example.com"+oidcCallbackPath+"?code=abc&state="+url.QueryEscape(st.State), nil)
	req2.Host = "app2.example.com"
	req2.AddCookie(stateCookie)
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusFound {
		t.Fatalf("callback: got %d, want 302 (body %q)", rec2.Code, rec2.Body.String())
	}
	if loc := rec2.Header().Get("Location"); loc != "/dash" {
		t.Fatalf("callback should return to the original path, got %q", loc)
	}
	sessCookie := cookieByName(rec2.Result(), oidcSessionCookie)
	if sessCookie == nil {
		t.Fatal("callback must set the SSO session cookie")
	}

	// 3. A request bearing the session cookie is admitted and gets identity headers.
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "https://app2.example.com/dash", nil)
	req3.Host = "app2.example.com"
	req3.AddCookie(sessCookie)
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK || rec3.Body.String() != "upstream:admin@example.com" {
		t.Fatalf("authed request: got %d %q", rec3.Code, rec3.Body.String())
	}

	// 4. A callback with no state cookie is rejected.
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest("GET", "https://app2.example.com"+oidcCallbackPath+"?code=abc&state=x", nil)
	req4.Host = "app2.example.com"
	h.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusBadRequest {
		t.Fatalf("callback without state cookie: got %d, want 400", rec4.Code)
	}
}
