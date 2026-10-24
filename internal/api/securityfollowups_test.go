package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// sharedStore is a fresh config repo that several scoped handlers can share,
// so a test can seed state as an admin and then act on it with a narrower
// token.
func sharedStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	return st
}

func scopedHandlerOn(t *testing.T, st *store.Store, scopes ...string) http.Handler {
	t.Helper()
	p := auth.Principal{Role: auth.RoleAdmin, IsToken: true, Subject: "token:ci", Scopes: scopes}
	return api.New(api.Deps{
		Store:        st,
		RequireScope: func(_ *http.Request, required string) error { return auth.RequireScope(p, required) },
	})
}

const (
	validDNSProvider = `{"name":"cf","provider":"cloudflare","config":{"apiToken":"${ENV:CF_TOKEN}"}}`
	validIdP         = `{"name":"idp","type":"oidc","oidc":{"issuerURL":"https://idp.example.com","clientID":"gpm","clientSecret":"${ENV:OIDC_SECRET}"}}`
	bouncerA         = `{"name":"bnc","type":"bouncer","bouncer":{"provider":"crowdsec","url":"http://10.0.0.9:8080","apiKey":"${ENV:CS_KEY}"}}`
	bouncerATimeout  = `{"name":"bnc","type":"bouncer","bouncer":{"provider":"crowdsec","url":"http://10.0.0.9:8080","apiKey":"${ENV:CS_KEY}","timeout":"3s"}}`
	bouncerMovedURL  = `{"name":"bnc","type":"bouncer","bouncer":{"provider":"crowdsec","url":"http://203.0.113.9:8080","apiKey":"${ENV:CS_KEY}"}}`
	bouncerNoKey     = `{"name":"bnc2","type":"bouncer","bouncer":{"provider":"http","url":"http://10.0.0.9:8080/verdict"}}`
)

// Writing a DNS provider or an identity provider is admin-equivalent (issue
// #49): the per-kind write scope, and even *:write, no longer reach it.
func TestProviderWritesNeedAdmin(t *testing.T) {
	tests := []struct {
		name       string
		scopes     []string
		path       string
		body       string
		wantStatus int
	}{
		{"dns-providers:write cannot write", []string{"dns-providers:write"}, "/dns-providers/cf", validDNSProvider, http.StatusForbidden},
		{"*:write cannot write dns providers", []string{"*:write"}, "/dns-providers/cf", validDNSProvider, http.StatusForbidden},
		{"admin writes dns providers", []string{model.ScopeAdmin}, "/dns-providers/cf", validDNSProvider, http.StatusOK},
		{"identity-providers:write cannot write", []string{"identity-providers:write"}, "/identity-providers/idp", validIdP, http.StatusForbidden},
		{"*:write cannot write identity providers", []string{"*:write"}, "/identity-providers/idp", validIdP, http.StatusForbidden},
		{"admin writes identity providers", []string{model.ScopeAdmin}, "/identity-providers/idp", validIdP, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := scopedHandler(t, tc.scopes...)
			if w := do(t, h, "PUT", tc.path, tc.body); w.Code != tc.wantStatus {
				t.Fatalf("PUT %s: %d %s, want %d", tc.path, w.Code, w.Body.String(), tc.wantStatus)
			}
			// Reading stays on the kind's own read scope.
			if w := do(t, scopedHandler(t, strings.Split(tc.path, "/")[1]+":read"), "GET", tc.path[:strings.LastIndex(tc.path, "/")], ""); w.Code != http.StatusOK {
				t.Fatalf("read scope must still list: %d", w.Code)
			}
		})
	}
}

// A bouncer that sends a placeholder API key to a URL is created, or has its
// url/apiKey changed, by an admin only; other middleware writes and edits that
// keep the pair stay on middlewares:write.
func TestBouncerSecretDestinationNeedsAdmin(t *testing.T) {
	st := sharedStore(t)
	admin := scopedHandlerOn(t, st, model.ScopeAdmin)
	writer := scopedHandlerOn(t, st, "middlewares:write")

	if w := do(t, writer, "PUT", "/middlewares/bnc", bouncerA); w.Code != http.StatusForbidden {
		t.Fatalf("create with placeholder key on middlewares:write: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/middlewares/bnc2", bouncerNoKey); w.Code != http.StatusOK {
		t.Fatalf("a bouncer without a secret stays on middlewares:write: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, admin, "PUT", "/middlewares/bnc", bouncerA); w.Code != http.StatusOK {
		t.Fatalf("admin create: %d %s", w.Code, w.Body.String())
	}
	// As a middlewares:write caller: an edit that keeps url and apiKey is
	// fine, moving the URL is not.
	if w := do(t, writer, "PUT", "/middlewares/bnc", bouncerATimeout); w.Code != http.StatusOK {
		t.Fatalf("edit keeping url+apiKey: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/middlewares/bnc", bouncerMovedURL); w.Code != http.StatusForbidden {
		t.Fatalf("moving the URL on middlewares:write: %d %s, want 403", w.Code, w.Body.String())
	}
	// A revert restores a stored object without the guard, so the middleware
	// revert route is admin-only: middlewares:write is refused at the gate,
	// admin gets past it (and fails on the made-up hash).
	if w := do(t, writer, "POST", "/middlewares/bnc/revert", `{"hash":"abcdef1"}`); w.Code != http.StatusForbidden {
		t.Fatalf("middleware revert on middlewares:write: %d, want 403", w.Code)
	}
	if w := do(t, admin, "POST", "/middlewares/bnc/revert", `{"hash":"abcdef1"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("admin reaches the middleware revert: %d %s", w.Code, w.Body.String())
	}
}

// GET /settings shows a caller below admin the webhook and notification
// target URLs with their path and query removed: for Discord, Slack and ntfy
// the path is the credential.
func TestSettingsRedactsDeliveryURLsBelowAdmin(t *testing.T) {
	st := sharedStore(t)
	admin := scopedHandlerOn(t, st, model.ScopeAdmin)
	body := `{"schemaVersion":1,"adminAuth":{"localLoginEnabled":true},` +
		`"webhooks":[{"name":"d","url":"https://discord.example/api/webhooks/123/supersecret"}],` +
		`"notifications":{"targets":[{"name":"n","type":"ntfy","url":"https://ntfy.example/private-topic"}]}}`
	if w := do(t, admin, "PUT", "/settings", body); w.Code != http.StatusOK {
		t.Fatalf("seed settings: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, admin, "GET", "/settings", ""); !strings.Contains(w.Body.String(), "supersecret") || !strings.Contains(w.Body.String(), "private-topic") {
		t.Fatalf("admin must see the full URLs: %s", w.Body.String())
	}
	viewer := scopedHandlerOn(t, st, "settings:read")
	w := do(t, viewer, "GET", "/settings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("viewer GET: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "supersecret") || strings.Contains(w.Body.String(), "private-topic") {
		t.Fatalf("a settings:read caller must not see delivery paths: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "https://discord.example/(redacted)") || !strings.Contains(w.Body.String(), "https://ntfy.example/(redacted)") {
		t.Fatalf("redaction should keep scheme and host: %s", w.Body.String())
	}
}
