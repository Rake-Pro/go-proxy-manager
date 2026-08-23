package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/metrics"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
)

func metricsAuthenticator(t *testing.T) (*auth.Authenticator, *session.Store) {
	t.Helper()
	sessStore, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sessStore.Close() })
	return auth.NewAuthenticator(auth.Options{Store: sessStore}), sessStore
}

// With metrics off the route must 404, not fall through to the SPA catch-all
// (which answers 200 with the app shell for any unknown path) and not 401 -
// "is this instance exporting metrics?" has to be answerable from a scrape.
func TestMetricsDisabledIs404(t *testing.T) {
	authn, _ := metricsAuthenticator(t)
	s := New(":0", nil, authn, nil, nil, nil, false)

	rec := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("metrics disabled: expected 404, got %d", rec.Code)
	}
}

// Enabled but unauthenticated is a 401: the exposition names every configured
// host and certificate, so it is admin data, not a public liveness probe.
func TestMetricsEnabledRequiresAuth(t *testing.T) {
	authn, sessStore := metricsAuthenticator(t)
	m := metrics.NewMetrics("test", "c", "go")
	m.HTTPRequest("app", "GET", 200, time.Millisecond, 0, 0)
	s := New(":0", nil, authn, nil, nil, m.Handler(), false)

	rec := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("metrics enabled, no credential: expected 401, got %d", rec.Code)
	}

	// An admin session needs no scope - scopes only ever constrain tokens.
	sess := &session.Session{Subject: "admin", Roles: []string{string(auth.RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := sessStore.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.AddCookie(&http.Cookie{Name: "gpm_session", Value: sess.ID})
	s.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics enabled, admin session: expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "gpm_http_requests_total") {
		t.Fatalf("exposition body does not look like metrics:\n%s", body)
	}
}

// Every API-token principal is admin-ROLE by construction, so the role gate
// alone would hand any token the exposition. It takes the narrow metrics:read
// scope - and, being a read, "write" on the same subject satisfies it while a
// different resource scope does not.
func TestMetricsRequiresMetricsScopeForTokens(t *testing.T) {
	authn, _ := metricsAuthenticator(t)

	type tok struct {
		name   string
		scopes []string
		secret string
		hash   string
	}
	toks := []*tok{
		{name: "hosts", scopes: []string{"proxy-hosts:read"}},
		{name: "scraper", scopes: []string{model.ScopeMetricsRead}},
		{name: "root", scopes: []string{model.ScopeAdmin}},
		{name: "wildcard", scopes: []string{"*:read"}},
	}
	var objs []model.APIToken
	for _, tk := range toks {
		secret, hash, err := auth.NewTokenSecret()
		if err != nil {
			t.Fatal(err)
		}
		tk.secret, tk.hash = secret, hash
		objs = append(objs, model.APIToken{ObjectMeta: model.ObjectMeta{Name: tk.name}, TokenHash: hash, Scopes: tk.scopes})
	}
	authn.SetTokenSource(func() []model.APIToken { return objs })

	m := metrics.NewMetrics("test", "c", "go")
	s := New(":0", nil, authn, nil, nil, m.Handler(), false)

	want := map[string]int{
		"hosts":    http.StatusForbidden,
		"scraper":  http.StatusOK,
		"root":     http.StatusOK,
		"wildcard": http.StatusOK,
	}
	for _, tk := range toks {
		t.Run(tk.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/metrics", nil)
			req.Header.Set("Authorization", "Bearer "+tk.secret)
			s.http.Handler.ServeHTTP(rec, req)
			if rec.Code != want[tk.name] {
				t.Fatalf("token %q: got %d, want %d", tk.name, rec.Code, want[tk.name])
			}
		})
	}
}

// A "metrics" scope subject must be accepted by APIToken.Validate, or the
// scrape credential above could never be committed in the first place.
func TestMetricsScopeIsAValidSubject(t *testing.T) {
	tk := model.APIToken{ObjectMeta: model.ObjectMeta{Name: "scraper"}, Scopes: []string{model.ScopeMetricsRead}}
	if err := tk.Validate(); err != nil {
		t.Fatalf("metrics:read must be a valid scope: %v", err)
	}
}
