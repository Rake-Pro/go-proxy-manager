package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

const testClientID = "test-client"

type tokenClaims map[string]any

// mockProvider is an in-memory OIDC provider for end-to-end tests.
type mockProvider struct {
	server *httptest.Server
	signer jose.Signer
	pubJWK jose.JSONWebKey
	// claims used to build the next id_token at /token
	claims tokenClaims
}

func newMockProvider(t *testing.T) *mockProvider {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	const kid = "test-key-1"
	privJWK := jose.JSONWebKey{Key: key, KeyID: kid, Algorithm: "RS256", Use: "sig"}
	pubJWK := jose.JSONWebKey{Key: &key.PublicKey, KeyID: kid, Algorithm: "RS256", Use: "sig"}

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privJWK}, nil)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	mp := &mockProvider{signer: signer, pubJWK: pubJWK}

	mux := http.NewServeMux()
	mp.server = httptest.NewServer(mux)

	issuer := mp.server.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/authorize",
			"token_endpoint":                        issuer + "/token",
			"jwks_uri":                              issuer + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{mp.pubJWK}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(set)
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		idToken := mp.signIDToken(t)
		resp := map[string]any{
			"access_token": "x",
			"token_type":   "Bearer",
			"id_token":     idToken,
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	t.Cleanup(mp.server.Close)
	return mp
}

func (mp *mockProvider) signIDToken(t *testing.T) string {
	t.Helper()
	claims := tokenClaims{
		"iss": mp.server.URL,
		"aud": testClientID,
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range mp.claims {
		claims[k] = v
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	obj, err := mp.signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	s, err := obj.CompactSerialize()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return s
}

func baseConfig(issuer string) Config {
	return Config{
		IssuerURL:   issuer,
		ClientID:    testClientID,
		RedirectURL: "https://app.example/callback",
		UsePKCE:     true,
	}
}

func TestNew(t *testing.T) {
	mp := newMockProvider(t)
	ctx := context.Background()
	c, err := New(ctx, baseConfig(mp.server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.provider == nil || c.verifier == nil || c.oauth2Cfg == nil {
		t.Fatal("client not fully initialized")
	}
	if got := c.oauth2Cfg.Endpoint.TokenURL; got != mp.server.URL+"/token" {
		t.Errorf("token endpoint = %q", got)
	}
	if len(c.oauth2Cfg.Scopes) != 4 {
		t.Errorf("default scopes = %v", c.oauth2Cfg.Scopes)
	}
}

func TestAuthCodeURL_PKCE(t *testing.T) {
	mp := newMockProvider(t)
	c, err := New(context.Background(), baseConfig(mp.server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	verifier := GenerateVerifier()
	raw := c.AuthCodeURL("state-abc", "nonce-xyz", verifier)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := u.Query()
	if q.Get("state") != "state-abc" {
		t.Errorf("state = %q", q.Get("state"))
	}
	if q.Get("nonce") != "nonce-xyz" {
		t.Errorf("nonce = %q", q.Get("nonce"))
	}
	if q.Get("code_challenge") == "" {
		t.Error("missing code_challenge")
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q", q.Get("code_challenge_method"))
	}
}

func TestAuthCodeURL_NoPKCE(t *testing.T) {
	mp := newMockProvider(t)
	cfg := baseConfig(mp.server.URL)
	cfg.UsePKCE = false
	c, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw := c.AuthCodeURL("s", "n", GenerateVerifier())
	u, _ := url.Parse(raw)
	if u.Query().Get("code_challenge") != "" {
		t.Error("code_challenge should be absent when PKCE off")
	}
	if u.Query().Get("nonce") != "n" {
		t.Error("nonce should still be present")
	}
}

func TestExchange_Success(t *testing.T) {
	mp := newMockProvider(t)
	mp.claims = tokenClaims{
		"nonce":          "nonce-1",
		"email":          "user@example.com",
		"email_verified": true,
		"name":           "Test User",
		"groups":         []string{"admins", "devs"},
		"amr":            []string{"pwd", "mfa"},
		"acr":            "urn:acr:high",
	}
	c, err := New(context.Background(), baseConfig(mp.server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	claims, err := c.Exchange(context.Background(), "the-code", GenerateVerifier(), "nonce-1")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("subject = %q", claims.Subject)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("email = %q", claims.Email)
	}
	if !claims.EmailVerified {
		t.Error("email_verified should be true")
	}
	if claims.Name != "Test User" {
		t.Errorf("name = %q", claims.Name)
	}
	if strings.Join(claims.Groups, ",") != "admins,devs" {
		t.Errorf("groups = %v", claims.Groups)
	}
	if strings.Join(claims.AMR, ",") != "pwd,mfa" {
		t.Errorf("amr = %v", claims.AMR)
	}
	if claims.ACR != "urn:acr:high" {
		t.Errorf("acr = %q", claims.ACR)
	}
	if claims.Raw["sub"] != "user-123" {
		t.Errorf("raw sub = %v", claims.Raw["sub"])
	}
}

func TestExchange_CustomGroupsClaim(t *testing.T) {
	mp := newMockProvider(t)
	mp.claims = tokenClaims{
		"nonce": "n2",
		"roles": []string{"editor", "viewer"},
	}
	cfg := baseConfig(mp.server.URL)
	cfg.GroupsClaim = "roles"
	c, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	claims, err := c.Exchange(context.Background(), "code", GenerateVerifier(), "n2")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if strings.Join(claims.Groups, ",") != "editor,viewer" {
		t.Errorf("groups from roles = %v", claims.Groups)
	}
}

func TestExchange_NonceMismatch(t *testing.T) {
	mp := newMockProvider(t)
	mp.claims = tokenClaims{"nonce": "real-nonce"}
	c, err := New(context.Background(), baseConfig(mp.server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = c.Exchange(context.Background(), "code", GenerateVerifier(), "wrong-nonce")
	if err == nil {
		t.Fatal("expected nonce mismatch error")
	}
	if !strings.Contains(err.Error(), "nonce") {
		t.Errorf("error = %v", err)
	}
}

func TestExchange_RequireVerifiedEmail(t *testing.T) {
	mp := newMockProvider(t)
	mp.claims = tokenClaims{
		"nonce":          "n3",
		"email":          "user@example.com",
		"email_verified": false,
	}
	cfg := baseConfig(mp.server.URL)
	cfg.RequireVerifiedEmail = true
	c, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = c.Exchange(context.Background(), "code", GenerateVerifier(), "n3")
	if err == nil {
		t.Fatal("expected error for unverified email")
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("error = %v", err)
	}
}

func TestGenerators(t *testing.T) {
	s1, err := NewState()
	if err != nil || s1 == "" {
		t.Fatalf("NewState: %q err=%v", s1, err)
	}
	s2, _ := NewState()
	if s1 == s2 {
		t.Error("NewState produced identical values")
	}
	n1, err := NewNonce()
	if err != nil || n1 == "" {
		t.Fatalf("NewNonce: %q err=%v", n1, err)
	}
	n2, _ := NewNonce()
	if n1 == n2 {
		t.Error("NewNonce produced identical values")
	}
	if GenerateVerifier() == "" {
		t.Error("GenerateVerifier empty")
	}
}
