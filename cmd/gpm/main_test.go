package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
)

// A request that never went through RequireRole carries no principal. api.Deps
// treats a nil RequireScope as "allow" (the unwired/test case), so the daemon's
// implementation is the only thing between such a request and every route: it
// has to fail CLOSED.
func TestRequireScopeDeniesWithoutPrincipal(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/proxy-hosts", nil)
	for _, required := range []string{"proxy-hosts:read", "*:read", model.ScopeAdmin} {
		if err := requireScope(r, required); err == nil {
			t.Fatalf("requireScope(%q) with no principal must deny", required)
		}
	}
}

// With a real principal attached by the real middleware, the gate behaves as
// documented: sessions are unconstrained, tokens are held to their scopes.
func TestRequireScopeWithPrincipal(t *testing.T) {
	sessStore, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sessStore.Close() })

	secret, hash, err := auth.NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	authn := auth.NewAuthenticator(auth.Options{Store: sessStore})
	authn.SetTokenSource(func() []model.APIToken {
		return []model.APIToken{{
			ObjectMeta: model.ObjectMeta{Name: "ci"},
			TokenHash:  hash,
			Scopes:     []string{"proxy-hosts:write"},
		}}
	})

	tests := []struct {
		name     string
		required string
		wantErr  bool
	}{
		{"granted scope passes", "proxy-hosts:read", false},
		{"write implies read", "proxy-hosts:write", false},
		{"other resource is refused", "certificates:read", true},
		{"admin endpoints are refused", model.ScopeAdmin, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got error
			var reached bool
			h := authn.RequireRole(auth.RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				got = requireScope(r, tc.required)
			}))
			r := httptest.NewRequest("GET", "/api/proxy-hosts", nil)
			r.Header.Set("Authorization", "Bearer "+secret)
			h.ServeHTTP(httptest.NewRecorder(), r)
			if !reached {
				t.Fatal("the token did not authenticate")
			}
			if (got != nil) != tc.wantErr {
				t.Fatalf("requireScope(%q) = %v, wantErr %v", tc.required, got, tc.wantErr)
			}
		})
	}
}
