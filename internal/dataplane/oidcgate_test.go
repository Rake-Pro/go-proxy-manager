package dataplane

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	// GPM-I2: the cookie carries the __Host- prefix and satisfies its invariants
	// (Secure, Path=/, no Domain), so a sibling subdomain cannot shadow it.
	if !strings.HasPrefix(sessCookie.Name, "__Host-") {
		t.Fatalf("SSO cookie must use the __Host- prefix, got %q", sessCookie.Name)
	}
	if !sessCookie.Secure || sessCookie.Path != "/" || sessCookie.Domain != "" {
		t.Fatalf("__Host- cookie invariants violated: secure=%v path=%q domain=%q", sessCookie.Secure, sessCookie.Path, sessCookie.Domain)
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

// TestDataOIDCSessionBoundToHost proves the M2 fix: the process-wide HMAC key
// means a session cookie minted for one gated host verifies on another, so the
// gate must additionally reject a cookie whose signed Host is not its own -
// otherwise a copied cookie carrying host A's groups would be re-evaluated
// against host B's role mapping.
func TestDataOIDCSessionBoundToHost(t *testing.T) {
	idpSrv := newMockIdP(t)
	newGate := func(host string) http.Handler {
		g, err := compileDataOIDC(model.IdentityProvider{
			ObjectMeta: model.ObjectMeta{Name: "idp"},
			Type:       model.IdPTypeOIDC,
			OIDC:       &model.OIDCSpec{IssuerURL: idpSrv.server.URL, ClientID: "gpm-client"},
		}, nil, host)
		if err != nil {
			t.Fatalf("compile %s: %v", host, err)
		}
		return g.handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("upstream"))
		}))
	}

	// A valid session cookie bound to host "app".
	payload, _ := json.Marshal(oidcSession{Sub: "u", Host: "app", Exp: time.Now().Add(time.Hour).Unix()})
	appCookie := &http.Cookie{Name: oidcSessionCookie, Value: signToken(payload)}

	// Its own host admits it.
	recSame := httptest.NewRecorder()
	rSame := httptest.NewRequest("GET", "https://app.example.com/x", nil)
	rSame.AddCookie(appCookie)
	newGate("app").ServeHTTP(recSame, rSame)
	if recSame.Code != http.StatusOK || recSame.Body.String() != "upstream" {
		t.Fatalf("cookie must be honored by its own host: got %d %q", recSame.Code, recSame.Body.String())
	}

	// A different host must not honor the replayed cookie; it starts a fresh login.
	recCross := httptest.NewRecorder()
	rCross := httptest.NewRequest("GET", "https://other.example.com/x", nil)
	rCross.AddCookie(appCookie)
	newGate("other").ServeHTTP(recCross, rCross)
	if recCross.Code == http.StatusOK {
		t.Fatalf("cookie bound to host \"app\" must not be honored by host \"other\"")
	}
	if recCross.Code != http.StatusFound {
		t.Fatalf("cross-host replay should trigger a fresh login redirect, got %d", recCross.Code)
	}
}

func TestSSORevocationWatermark(t *testing.T) {
	idpSrv := newMockIdP(t)
	g, err := compileDataOIDC(model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: "idp"},
		Type:       model.IdPTypeOIDC,
		OIDC:       &model.OIDCSpec{IssuerURL: idpSrv.server.URL, ClientID: "gpm-client"},
	}, nil, "app")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	gate := g.handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("upstream"))
	}))
	// The watermark is process-global; reset it so test order never matters.
	t.Cleanup(func() { ssoNotBefore.Store(0) })

	mkCookie := func(iat int64) *http.Cookie {
		payload, _ := json.Marshal(oidcSession{
			Sub: "u", Host: "app",
			Exp: time.Now().Add(time.Hour).Unix(),
			Iat: iat,
		})
		return &http.Cookie{Name: oidcSessionCookie, Value: signToken(payload)}
	}
	serve := func(c *http.Cookie) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://app.example.com/x", nil)
		r.AddCookie(c)
		gate.ServeHTTP(rec, r)
		return rec.Code
	}

	old := mkCookie(time.Now().Add(-10 * time.Second).Unix())
	legacy := mkCookie(0) // pre-Iat cookie shape

	if code := serve(old); code != http.StatusOK {
		t.Fatalf("pre-revocation session rejected: %d", code)
	}
	if code := serve(legacy); code != http.StatusOK {
		t.Fatalf("legacy (iat-less) session rejected before any revocation: %d", code)
	}

	if err := RevokeAllSSOSessions(); err != nil {
		t.Fatalf("RevokeAllSSOSessions: %v", err)
	}

	// Everything issued before the watermark - including legacy cookies - now
	// bounces to a fresh login instead of reaching the upstream.
	if code := serve(old); code != http.StatusFound {
		t.Fatalf("revoked session got %d, want 302 login redirect", code)
	}
	if code := serve(legacy); code != http.StatusFound {
		t.Fatalf("legacy session survived revocation: %d", code)
	}

	// A session minted after (or at) the watermark is valid.
	if code := serve(mkCookie(time.Now().Unix())); code != http.StatusOK {
		t.Fatalf("post-revocation session rejected: %d", code)
	}
}

func TestSSORevocationPersists(t *testing.T) {
	// With a state dir configured, the watermark is written next to the signing
	// key so revocation survives a restart.
	dir := t.TempDir()
	SetSSOKeyDir(dir)
	t.Cleanup(func() {
		SetSSOKeyDir("")
		ssoNotBefore.Store(0)
	})

	if err := RevokeAllSSOSessions(); err != nil {
		t.Fatalf("RevokeAllSSOSessions: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ssoNotBeforeFile))
	if err != nil {
		t.Fatalf("watermark not persisted: %v", err)
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || v <= 0 {
		t.Fatalf("persisted watermark %q unusable: %v", b, err)
	}
	if got := ssoNotBefore.Load(); got != v {
		t.Fatalf("in-memory watermark %d != persisted %d", got, v)
	}
}
