package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// tokenAuthenticator returns an Authenticator serving the given tokens, plus a
// handler behind RequireRole(admin) that records the principal it saw.
func tokenAuthenticator(t *testing.T, tokens ...model.APIToken) (*Authenticator, http.Handler, *Principal) {
	t.Helper()
	a := NewAuthenticator(Options{Store: testStore(t)})
	a.SetTokenSource(func() []model.APIToken { return tokens })
	var seen Principal
	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		seen = p
		w.WriteHeader(http.StatusOK)
	}))
	return a, h, &seen
}

func bearerRequest(method, secret string) *http.Request {
	r := httptest.NewRequest(method, "/api/proxy-hosts", nil)
	if secret != "" {
		r.Header.Set("Authorization", "Bearer "+secret)
	}
	return r
}

func TestNewTokenSecretShape(t *testing.T) {
	secret, hash, err := NewTokenSecret()
	if err != nil {
		t.Fatalf("NewTokenSecret: %v", err)
	}
	if len(secret) <= len(TokenPrefix) || secret[:len(TokenPrefix)] != TokenPrefix {
		t.Fatalf("secret %q must carry the %q prefix", secret, TokenPrefix)
	}
	if hash != HashTokenSecret(secret) {
		t.Fatal("returned hash must be the sha256 of the returned secret")
	}
	if len(hash) != 64 {
		t.Fatalf("hash %q is not a sha256 hex digest", hash)
	}
	other, _, err := NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	if other == secret {
		t.Fatal("two generated secrets must differ")
	}
}

func TestBearerTokenAuth(t *testing.T) {
	secret, hash, err := NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	tests := []struct {
		name       string
		token      model.APIToken
		present    string // secret to present; empty means use the valid one
		wantStatus int
	}{
		{
			name:       "valid token authenticates",
			token:      model.APIToken{ObjectMeta: model.ObjectMeta{Name: "ci"}, TokenHash: hash, Scopes: []string{"proxy-hosts:read"}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unexpired token authenticates",
			token:      model.APIToken{ObjectMeta: model.ObjectMeta{Name: "ci"}, TokenHash: hash, Scopes: []string{"*:read"}, ExpiresAt: &future},
			wantStatus: http.StatusOK,
		},
		{
			name:       "expired token is rejected",
			token:      model.APIToken{ObjectMeta: model.ObjectMeta{Name: "ci"}, TokenHash: hash, Scopes: []string{"*:read"}, ExpiresAt: &past},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "disabled token is rejected",
			token:      model.APIToken{ObjectMeta: model.ObjectMeta{Name: "ci", Disabled: true}, TokenHash: hash, Scopes: []string{"*:read"}},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong secret is rejected",
			token:      model.APIToken{ObjectMeta: model.ObjectMeta{Name: "ci"}, TokenHash: hash, Scopes: []string{"*:read"}},
			present:    TokenPrefix + "definitely-not-the-right-secret",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token with no stored hash never matches",
			token:      model.APIToken{ObjectMeta: model.ObjectMeta{Name: "ci"}, Scopes: []string{"*:read"}},
			wantStatus: http.StatusUnauthorized,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, h, _ := tokenAuthenticator(t, tc.token)
			present := tc.present
			if present == "" {
				present = secret
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, bearerRequest(http.MethodGet, present))
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

func TestBearerTokenPrincipal(t *testing.T) {
	secret, hash, err := NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	_, h, seen := tokenAuthenticator(t, model.APIToken{
		ObjectMeta: model.ObjectMeta{Name: "ci"},
		TokenHash:  hash,
		Scopes:     []string{"proxy-hosts:read", "certificates:write"},
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, bearerRequest(http.MethodGet, secret))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !seen.IsToken {
		t.Fatal("principal must be marked as a token principal")
	}
	if seen.Role != RoleAdmin {
		t.Fatalf("role = %q, want admin", seen.Role)
	}
	if seen.Subject != "token:ci" {
		t.Fatalf("subject = %q, want token:ci", seen.Subject)
	}
	if len(seen.Scopes) != 2 || seen.Scopes[0] != "proxy-hosts:read" {
		t.Fatalf("scopes = %v", seen.Scopes)
	}
}

// A token principal carries no ambient authority, so the CSRF double-submit
// check must not apply to it - otherwise every scripted write would 403.
func TestBearerTokenSkipsCSRF(t *testing.T) {
	secret, hash, err := NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	_, h, _ := tokenAuthenticator(t, model.APIToken{
		ObjectMeta: model.ObjectMeta{Name: "ci"},
		TokenHash:  hash,
		Scopes:     []string{"*:write"},
	})
	w := httptest.NewRecorder()
	// No X-CSRF-Token header on a mutating method.
	h.ServeHTTP(w, bearerRequest(http.MethodPut, secret))
	if w.Code != http.StatusOK {
		t.Fatalf("token write status = %d, want 200 (CSRF must be skipped for token principals)", w.Code)
	}
}

// A presented-but-invalid gpm bearer token is an authentication failure. It must
// never fall through to the cookie path, where a stale session cookie riding
// along on the same request would silently grant access.
func TestBearerTokenNeverFallsThroughToCookie(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t)})
	a.SetTokenSource(func() []model.APIToken { return nil })
	sess, _, err := a.LocalLogin(t.Context(), "", "")
	if err == nil {
		t.Fatalf("unexpected session %v", sess)
	}

	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := bearerRequest(http.MethodGet, TokenPrefix+"bogus")
	r.AddCookie(&http.Cookie{Name: "gpm_session", Value: "whatever"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// A non-gpm bearer scheme is not ours to judge: it must fall through to the
// cookie path rather than hard-failing a request that has a valid session.
func TestNonGPMBearerFallsThrough(t *testing.T) {
	r := bearerRequest(http.MethodGet, "some-other-systems-token")
	if _, ok := bearerTokenSecret(r); ok {
		t.Fatal("a non-gpm bearer value must not be treated as an API token")
	}
	plain := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	if _, ok := bearerTokenSecret(plain); ok {
		t.Fatal("a request with no Authorization header must not look like a token")
	}
}

func TestTokenLastUsedTracking(t *testing.T) {
	secret, hash, err := NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	a, h, _ := tokenAuthenticator(t, model.APIToken{
		ObjectMeta: model.ObjectMeta{Name: "ci"},
		TokenHash:  hash,
		Scopes:     []string{"*:read"},
	})
	if len(a.TokenLastUsed()) != 0 {
		t.Fatal("last-used must start empty")
	}
	h.ServeHTTP(httptest.NewRecorder(), bearerRequest(http.MethodGet, secret))
	used := a.TokenLastUsed()
	if _, ok := used["ci"]; !ok {
		t.Fatalf("expected a last-used entry for ci, got %v", used)
	}
}

// An unauthenticated bearer attempt must not force a config load. The source
// closure reads the whole git-backed config (walk + parse + whole-graph
// validate) and a failed bearer auth never reaches the login rate gate, so
// re-reading per request would be an unthrottled internet-facing DoS lever.
func TestTokenSourceIsCachedAcrossRequests(t *testing.T) {
	secret, hash, err := NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	tokens := []model.APIToken{{
		ObjectMeta: model.ObjectMeta{Name: "ci"},
		TokenHash:  hash,
		Scopes:     []string{"*:read"},
	}}

	a := NewAuthenticator(Options{Store: testStore(t)})
	a.SetTokenSource(func() []model.APIToken {
		loads++
		return tokens
	})
	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// A flood of bogus bearer credentials costs exactly one load.
	for i := 0; i < 25; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, bearerRequest(http.MethodGet, TokenPrefix+"bogus"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("bogus bearer status = %d, want 401", w.Code)
		}
	}
	if loads != 1 {
		t.Fatalf("token source called %d times for 25 requests, want 1", loads)
	}

	// A genuine token still authenticates off the cache.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, bearerRequest(http.MethodGet, secret))
	if w.Code != http.StatusOK {
		t.Fatalf("valid token status = %d, want 200", w.Code)
	}
	if loads != 1 {
		t.Fatalf("token source called %d times, want 1 (still cached)", loads)
	}

	// A config change invalidates it: the edited token set takes effect on the
	// very next request, which is what makes the cache safe.
	tokens = nil
	a.InvalidateTokenCache()
	w = httptest.NewRecorder()
	h.ServeHTTP(w, bearerRequest(http.MethodGet, secret))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("after invalidation status = %d, want 401 (the deleted token must stop working)", w.Code)
	}
	if loads != 2 {
		t.Fatalf("token source called %d times, want 2 (one reload after invalidation)", loads)
	}
}

// Re-injecting a source drops whatever the previous one had cached.
func TestSetTokenSourceDropsCache(t *testing.T) {
	secret, hash, err := NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	a := NewAuthenticator(Options{Store: testStore(t)})
	a.SetTokenSource(func() []model.APIToken { return nil })
	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), bearerRequest(http.MethodGet, secret)) // primes the cache

	a.SetTokenSource(func() []model.APIToken {
		return []model.APIToken{{ObjectMeta: model.ObjectMeta{Name: "ci"}, TokenHash: hash, Scopes: []string{"*:read"}}}
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, bearerRequest(http.MethodGet, secret))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a new source must not serve the old cache)", w.Code)
	}
}

func TestRequireScope(t *testing.T) {
	session := Principal{Role: RoleAdmin}
	if err := RequireScope(session, "api-tokens:write"); err != nil {
		t.Fatalf("a session principal must never be scope-limited: %v", err)
	}
	tok := Principal{Role: RoleAdmin, IsToken: true, Subject: "token:ci", Scopes: []string{"proxy-hosts:read"}}
	if err := RequireScope(tok, "proxy-hosts:read"); err != nil {
		t.Fatalf("granted scope must pass: %v", err)
	}
	if err := RequireScope(tok, "proxy-hosts:write"); err == nil {
		t.Fatal("a read-only token must not pass a write check")
	}
	if err := RequireScope(tok, "admin"); err == nil {
		t.Fatal("a resource-scoped token must not reach admin endpoints")
	}
}
