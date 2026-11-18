package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/ha"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
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

// An ACME certificate that sets acme.directoryURL or acme.eab, or changes
// either, is written by an admin only; other certificate writes, edits that
// keep both and edits that clear them stay on certificates:write.
func TestCertificateACMEDestinationNeedsAdmin(t *testing.T) {
	const (
		plain    = `{"name":"c","type":"acme","domains":["c.example.com"],"acme":{"email":"a@example.com","challenge":"http-01"}}`
		withDir  = `{"name":"c","type":"acme","domains":["c.example.com"],"acme":{"email":"a@example.com","challenge":"http-01","directoryURL":"https://ca.example.com/dir","eab":{"kid":"k","hmacKey":"${ENV:EAB_KEY}"}}}`
		keepBoth = `{"name":"c","type":"acme","domains":["c.example.com","d.example.com"],"acme":{"email":"a@example.com","challenge":"http-01","directoryURL":"https://ca.example.com/dir","eab":{"kid":"k","hmacKey":"${ENV:EAB_KEY}"}}}`
		movedDir = `{"name":"c","type":"acme","domains":["c.example.com"],"acme":{"email":"a@example.com","challenge":"http-01","directoryURL":"http://192.0.2.10/dir","eab":{"kid":"k","hmacKey":"${ENV:EAB_KEY}"}}}`
		newKey   = `{"name":"c","type":"acme","domains":["c.example.com"],"acme":{"email":"a@example.com","challenge":"http-01","directoryURL":"https://ca.example.com/dir","eab":{"kid":"k","hmacKey":"${ENV:OTHER}"}}}`
	)
	st := sharedStore(t)
	admin := scopedHandlerOn(t, st, model.ScopeAdmin)
	writer := scopedHandlerOn(t, st, "certificates:write")

	if w := do(t, writer, "PUT", "/certificates/c", withDir); w.Code != http.StatusForbidden {
		t.Fatalf("create with directoryURL and eab on certificates:write: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/c", plain); w.Code != http.StatusOK {
		t.Fatalf("a certificate on the default CA stays on certificates:write: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/c", withDir); w.Code != http.StatusForbidden {
		t.Fatalf("setting directoryURL and eab on certificates:write: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := do(t, admin, "PUT", "/certificates/c", withDir); w.Code != http.StatusOK {
		t.Fatalf("admin sets directoryURL and eab: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/c", keepBoth); w.Code != http.StatusOK {
		t.Fatalf("edit keeping directoryURL and eab: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/c", movedDir); w.Code != http.StatusForbidden {
		t.Fatalf("changing directoryURL on certificates:write: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/c", newKey); w.Code != http.StatusForbidden {
		t.Fatalf("changing the eab key on certificates:write: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/c", plain); w.Code != http.StatusOK {
		t.Fatalf("clearing directoryURL and eab stays on certificates:write: %d %s", w.Code, w.Body.String())
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

// The restore upload lifts the admin server's read deadline for its own
// request, so an archive that arrives slower than the server-wide ReadTimeout
// is still read whole.
func TestRestoreLiftsTheReadDeadline(t *testing.T) {
	h := scopedHandlerOn(t, sharedStore(t), model.ScopeAdmin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/backup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("backup: %d", rec.Code)
	}
	archive := rec.Body.Bytes()

	ts := httptest.NewUnstartedServer(h)
	ts.Config.ReadTimeout = 200 * time.Millisecond
	ts.Start()
	t.Cleanup(ts.Close)

	pr, pw := io.Pipe()
	go func() {
		half := len(archive) / 2
		_, _ = pw.Write(archive[:half])
		time.Sleep(600 * time.Millisecond)
		_, _ = pw.Write(archive[half:])
		_ = pw.Close()
	}()
	req, err := http.NewRequest("POST", ts.URL+"/restore", pr)
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = int64(len(archive))
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || bytes.Contains(body, []byte("timeout")) {
		t.Fatalf("restore over a slow upload: %d %s", res.StatusCode, body)
	}
}

// POST /sessions/revoke ends every admin session but the caller's own, and
// works on an HA follower, whose sessions are its own.
func TestRevokeAdminSessionsRoute(t *testing.T) {
	for _, role := range []ha.Role{ha.RoleLeader, ha.RoleFollower} {
		t.Run(role.String(), func(t *testing.T) {
			sessions, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { sessions.Close() })
			authn := auth.NewAuthenticator(auth.Options{Store: sessions, LocalUser: "admin", LocalHash: "$2a$10$placeholder"})
			authn.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}})
			h := authn.RequireRole(auth.RoleAdmin, api.New(api.Deps{
				Store: sharedStore(t), Role: role, RevokeAdminSessions: authn.RevokeAllSessions,
			}))
			var all []*session.Session
			for i := 0; i < 3; i++ {
				s := &session.Session{Subject: "admin", Roles: []string{string(auth.RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
				if err := sessions.Create(context.Background(), s); err != nil {
					t.Fatal(err)
				}
				all = append(all, s)
			}
			req := httptest.NewRequest("POST", "/sessions/revoke", nil)
			req.AddCookie(&http.Cookie{Name: "gpm_session", Value: all[0].ID})
			req.Header.Set("X-CSRF-Token", all[0].CSRFToken)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"revoked": 2`) {
				t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
			}
			if _, err := sessions.Get(context.Background(), all[0].ID); err != nil {
				t.Fatalf("the caller's own session was ended: %v", err)
			}
			for _, s := range all[1:] {
				if _, err := sessions.Get(context.Background(), s.ID); err == nil {
					t.Fatal("another session survived the revoke")
				}
			}
		})
	}
}

// Ending every admin session is admin-scope only.
func TestRevokeAdminSessionsNeedsAdmin(t *testing.T) {
	h := scopedHandlerOn(t, sharedStore(t), "*:write")
	if w := do(t, h, "POST", "/sessions/revoke", ""); w.Code != http.StatusForbidden {
		t.Fatalf("*:write token: %d, want 403", w.Code)
	}
}
