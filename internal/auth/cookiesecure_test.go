package auth

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/clientip"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
)

// secureTestAuth builds an authenticator in mode with externalBaseURL base.
func secureTestAuth(t *testing.T, mode CookieSecureMode, base string) *Authenticator {
	t.Helper()
	a := NewAuthenticator(Options{Store: testStore(t), SecureMode: mode, SessionTTL: time.Hour})
	a.Configure(model.Config{}, model.Settings{ExternalBaseURL: base})
	return a
}

// secureTestRequest builds a request with the peer, scheme and forwarded header
// a case describes.
func secureTestRequest(peer, fwdProto string, useTLS bool) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/auth/local", nil)
	r.RemoteAddr = peer
	if fwdProto != "" {
		r.Header.Set("X-Forwarded-Proto", fwdProto)
	}
	if useTLS {
		r.TLS = &tls.ConnectionState{}
	}
	return r
}

func TestParseCookieSecureMode(t *testing.T) {
	cases := []struct {
		in      string
		want    CookieSecureMode
		wantErr bool
	}{
		{"", CookieSecureAuto, false},
		{"auto", CookieSecureAuto, false},
		{"AUTO", CookieSecureAuto, false},
		{"1", CookieSecureAlways, false},
		{"true", CookieSecureAlways, false},
		{"0", CookieSecureNever, false},
		{"false", CookieSecureNever, false},
		{"maybe", CookieSecureAuto, true},
	}
	for _, tc := range cases {
		got, err := ParseCookieSecureMode(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseCookieSecureMode(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("ParseCookieSecureMode(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if CookieSecureAuto.String() != "auto" || CookieSecureAlways.String() != "1" || CookieSecureNever.String() != "0" {
		t.Fatal("CookieSecureMode.String must round-trip the env values")
	}
}

// TestCookieSecureDecision is the table for the per-request Secure decision and
// the cookie name that goes with it.
func TestCookieSecureDecision(t *testing.T) {
	cases := []struct {
		name      string
		mode      CookieSecureMode
		base      string
		trusted   []string
		peer      string
		fwdProto  string
		useTLS    bool
		want      bool
		wantState string
	}{
		{
			name: "auto plain http loopback", mode: CookieSecureAuto, peer: "127.0.0.1:54321",
			want: false, wantState: CookieSecureStateInsecurePrivate,
		},
		{
			name: "auto plain http private lan", mode: CookieSecureAuto, peer: "192.168.10.5:54321",
			want: false, wantState: CookieSecureStateInsecurePrivate,
		},
		{
			name: "auto plain http public peer", mode: CookieSecureAuto, peer: "203.0.113.9:54321",
			want: false, wantState: CookieSecureStateInsecurePublic,
		},
		{
			name: "auto tls", mode: CookieSecureAuto, peer: "203.0.113.9:54321", useTLS: true,
			want: true, wantState: CookieSecureStateSecure,
		},
		{
			name: "auto trusted proxy forwards https", mode: CookieSecureAuto,
			trusted: []string{"10.0.0.0/8"}, peer: "10.1.2.3:41000", fwdProto: "https",
			want: true, wantState: CookieSecureStateSecure,
		},
		{
			name: "auto trusted proxy forwards http", mode: CookieSecureAuto,
			trusted: []string{"10.0.0.0/8"}, peer: "10.1.2.3:41000", fwdProto: "http",
			want: false, wantState: CookieSecureStateInsecurePrivate,
		},
		{
			name: "auto untrusted peer spoofs https", mode: CookieSecureAuto,
			trusted: []string{"10.0.0.0/8"}, peer: "203.0.113.9:41000", fwdProto: "https",
			want: false, wantState: CookieSecureStateInsecurePublic,
		},
		{
			name: "auto https external base url over plain http", mode: CookieSecureAuto,
			base: "https://gpm.example.com", peer: "127.0.0.1:54321",
			want: true, wantState: CookieSecureStateSecure,
		},
		{
			name: "auto http external base url", mode: CookieSecureAuto,
			base: "http://gpm.example.com", peer: "127.0.0.1:54321",
			want: false, wantState: CookieSecureStateInsecurePrivate,
		},
		{
			name: "explicit 1 over plain http loopback", mode: CookieSecureAlways, peer: "127.0.0.1:54321",
			want: true, wantState: CookieSecureStateSecure,
		},
		{
			name: "explicit 0 over tls", mode: CookieSecureNever, peer: "203.0.113.9:443", useTLS: true,
			base: "https://gpm.example.com", want: false, wantState: CookieSecureStateInsecurePublic,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clientip.SetTrusted(tc.trusted)
			t.Cleanup(func() { clientip.SetTrusted(nil) })

			a := secureTestAuth(t, tc.mode, tc.base)
			r := secureTestRequest(tc.peer, tc.fwdProto, tc.useTLS)

			if got := a.secureFor(r); got != tc.want {
				t.Fatalf("secureFor = %v, want %v", got, tc.want)
			}
			if got := a.CookieSecureState(r); got != tc.wantState {
				t.Fatalf("CookieSecureState = %q, want %q", got, tc.wantState)
			}

			// The issued cookie matches the decision, name and all.
			sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
			if err := a.store.Create(context.Background(), sess); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			a.IssueCookie(rec, r, sess)
			name := "gpm_session"
			if tc.want {
				name = "__Host-gpm_session"
			}
			c := findCookie(t, rec.Result(), name)
			if c.Secure != tc.want {
				t.Fatalf("issued cookie Secure = %v, want %v", c.Secure, tc.want)
			}
			if c.Path != "/" || !c.HttpOnly || c.Value != sess.ID {
				t.Fatalf("issued cookie: %+v", c)
			}

			// The OIDC login-state cookie makes the same call.
			rec2 := httptest.NewRecorder()
			a.SetLoginStateCookie(rec2, r, "state-value")
			sc := findCookie(t, rec2.Result(), oidcStateCookie)
			if sc.Secure != tc.want {
				t.Fatalf("login-state cookie Secure = %v, want %v", sc.Secure, tc.want)
			}
			rec3 := httptest.NewRecorder()
			a.ClearLoginStateCookie(rec3, r)
			cc := findCookie(t, rec3.Result(), oidcStateCookie)
			if cc.Secure != tc.want || cc.MaxAge != -1 {
				t.Fatalf("cleared login-state cookie: %+v", cc)
			}
		})
	}
}

// TestPlainHTTPLoopbackLoginEndToEnd is the regression: the first login over
// http://127.0.0.1:8081 must set a usable cookie and authenticate the next
// request. With a Secure-by-default cookie the browser dropped it and the
// operator saw a login that "did nothing".
func TestPlainHTTPLoopbackLoginEndToEnd(t *testing.T) {
	a := secureTestAuth(t, CookieSecureAuto, "")
	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := a.store.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}

	login := secureTestRequest("127.0.0.1:54321", "", false)
	rec := httptest.NewRecorder()
	a.IssueCookie(rec, login, sess)
	c := findCookie(t, rec.Result(), "gpm_session")
	if c.Secure {
		t.Fatal("a loopback plain-HTTP login must not issue a Secure cookie")
	}

	// A real browser would now send exactly this cookie back.
	next := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	next.RemoteAddr = "127.0.0.1:54322"
	next.AddCookie(c)
	var reached bool
	h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, next)
	if !reached || res.Code != http.StatusOK {
		t.Fatalf("plain-HTTP session not accepted: status %d", res.Code)
	}
}

// TestSessionCookieDualNameLookup: a session issued under one name keeps working
// after the operator flips modes, in the direction that is not a downgrade.
func TestSessionCookieDualNameLookup(t *testing.T) {
	cases := []struct {
		name       string
		cookieName string
		mode       CookieSecureMode
		useTLS     bool
		wantOK     bool
	}{
		{"bare cookie on plain http", "gpm_session", CookieSecureAuto, false, true},
		{"bare cookie upgraded to tls", "gpm_session", CookieSecureAuto, true, true},
		{"host cookie over tls", "__Host-gpm_session", CookieSecureAuto, true, true},
		{"host cookie forced secure", "__Host-gpm_session", CookieSecureAlways, false, true},
		// The no-downgrade rule: a __Host- cookie presented where the reply would
		// be non-Secure is refused outright, so the session is never re-issued
		// without Secure. The operator logs in again.
		{"host cookie on plain http is refused", "__Host-gpm_session", CookieSecureAuto, false, false},
		{"host cookie with secure disabled is refused", "__Host-gpm_session", CookieSecureNever, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := secureTestAuth(t, tc.mode, "")
			sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
			if err := a.store.Create(context.Background(), sess); err != nil {
				t.Fatal(err)
			}
			r := secureTestRequest("127.0.0.1:54321", "", tc.useTLS)
			r.Method = http.MethodGet
			r.AddCookie(&http.Cookie{Name: tc.cookieName, Value: sess.ID})

			var reached bool
			h := a.RequireRole(RoleAdmin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if reached != tc.wantOK {
				t.Fatalf("authenticated = %v, want %v (status %d)", reached, tc.wantOK, rec.Code)
			}
		})
	}
}

// TestSlideNeverDowngradesCookie: a session that slides over plain HTTP must not
// be re-issued under the __Host- name, and one that slides over TLS must not be
// re-issued without Secure.
func TestSlideNeverDowngradesCookie(t *testing.T) {
	for _, tc := range []struct {
		name     string
		useTLS   bool
		wantName string
	}{
		{"plain http keeps the bare name", false, "gpm_session"},
		{"tls slides onto the __Host- name", true, "__Host-gpm_session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := secureTestAuth(t, CookieSecureAuto, "")
			// Past the half-TTL threshold, so the next use slides it.
			sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(10 * time.Minute)}
			if err := a.store.Create(context.Background(), sess); err != nil {
				t.Fatal(err)
			}
			r := secureTestRequest("127.0.0.1:54321", "", tc.useTLS)
			r.Method = http.MethodGet
			r.AddCookie(&http.Cookie{Name: "gpm_session", Value: sess.ID})

			rec := httptest.NewRecorder()
			a.RequireRole(RoleAdmin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, r)
			c := findCookie(t, rec.Result(), tc.wantName)
			if c.Secure != tc.useTLS {
				t.Fatalf("slid cookie Secure = %v, want %v", c.Secure, tc.useTLS)
			}
		})
	}
}

// TestLogoutClearsBothCookieNames: whichever name carried the session, logout
// must not leave the other one sitting in the browser.
func TestLogoutClearsBothCookieNames(t *testing.T) {
	a := secureTestAuth(t, CookieSecureAuto, "")
	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := a.store.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: "__Host-gpm_session", Value: sess.ID})
	rec := httptest.NewRecorder()
	a.Logout(rec, r)

	for _, n := range []string{"gpm_session", "__Host-gpm_session"} {
		c := findCookie(t, rec.Result(), n)
		if c.Value != "" || c.MaxAge != -1 {
			t.Fatalf("cookie %q not cleared: %+v", n, c)
		}
	}
	if _, err := a.store.Get(context.Background(), sess.ID); err == nil {
		t.Fatal("logout must delete the session presented under the __Host- name")
	}
}

// TestInsecureCookieWarnRateLimit: the first-run nudge fires for a public client
// and then stays quiet for an hour; a private client never triggers it.
func TestInsecureCookieWarnRateLimit(t *testing.T) {
	a := secureTestAuth(t, CookieSecureAuto, "")
	now := time.Now()
	if !a.insecureWarn.allow(now) {
		t.Fatal("first warning must be allowed")
	}
	if a.insecureWarn.allow(now.Add(time.Minute)) {
		t.Fatal("a second warning within the hour must be suppressed")
	}
	if !a.insecureWarn.allow(now.Add(insecureWarnEvery + time.Second)) {
		t.Fatal("the warning must be allowed again after the window")
	}

	// A loopback client is the normal bootstrap case and must not warn at all.
	b := secureTestAuth(t, CookieSecureAuto, "")
	sess := &session.Session{Subject: "admin", Roles: []string{string(RoleAdmin)}, IdP: "local", ExpiresAt: time.Now().Add(time.Hour)}
	if err := b.store.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	b.IssueCookie(httptest.NewRecorder(), secureTestRequest("127.0.0.1:54321", "", false), sess)
	if !b.insecureWarn.last.IsZero() {
		t.Fatal("a loopback client must not trip the insecure-cookie warning")
	}
	// A public client does.
	b.IssueCookie(httptest.NewRecorder(), secureTestRequest("203.0.113.9:54321", "", false), sess)
	if b.insecureWarn.last.IsZero() {
		t.Fatal("a public client over plain HTTP must trip the insecure-cookie warning")
	}
}

// TestForwardedProtoListTakesLeftmost: the leftmost element is the scheme the
// browser used, which is what decides whether it stores a Secure cookie.
func TestForwardedProtoListTakesLeftmost(t *testing.T) {
	clientip.SetTrusted([]string{"10.0.0.0/8"})
	t.Cleanup(func() { clientip.SetTrusted(nil) })
	a := secureTestAuth(t, CookieSecureAuto, "")
	for proto, want := range map[string]bool{"https, http": true, "http, https": false} {
		r := secureTestRequest("10.1.2.3:41000", proto, false)
		if got := a.secureFor(r); got != want {
			t.Errorf("X-Forwarded-Proto %q: secureFor = %v, want %v", proto, got, want)
		}
	}
	if !strings.Contains(CookieSecureStateInsecurePublic, "public") {
		t.Fatal("state constant renamed without updating the UI contract")
	}
}
