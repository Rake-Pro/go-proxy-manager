package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/ui"
)

func TestSanitizeReturnTo(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/dashboard", "/dashboard"},
		{"/a/b?x=1", "/a/b?x=1"},
		{"", "/"},
		{"relative", "/"},
		{"//evil.com", "/"},
		{`/\evil.com`, "/"},
		{`\/evil.com`, "/"},
		{`\evil.com`, "/"},
		{"https://evil.com", "/"},
		{"/ok\r\nSet-Cookie: x", "/"},
		{"/ok\x00", "/"},
		{"/\t/evil.com", "/"},
		{"/ok\x7f", "/"},
		{"/ok\x1b[0m", "/"},
	}
	for _, c := range cases {
		if got := sanitizeReturnTo(c.in); got != c.want {
			t.Errorf("sanitizeReturnTo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLoginPageShell checks the login page is drawn from the app's own
// stylesheet and theme script, and that both, plus the font they load, are
// reachable before there is a session.
func TestLoginPageShell(t *testing.T) {
	e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true, noProviders: true})
	uiHandler, err := ui.Handler()
	if err != nil {
		t.Fatal(err)
	}
	e.srv = New(":0", nil, e.authn, nil, uiHandler, nil, false)

	body := readBody(t, e.get("/auth/login"))
	for _, want := range []string{
		`<script src="/theme-init.js"></script>`,
		`<link rel="stylesheet" href="/app.css">`,
		`<main class="login">`,
		`<h1>Sign in</h1>`,
		`<button class="btn primary" type="submit">Sign in</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("login page missing %q", want)
		}
	}
	if strings.Index(body, "/theme-init.js") > strings.Index(body, "/app.css") {
		t.Error("theme-init.js must load before app.css so the theme applies before first paint")
	}
	for _, path := range []string{"/app.css", "/theme-init.js", "/fonts/red-hat-text-vf.woff2"} {
		if res := e.get(path); res.StatusCode != http.StatusOK {
			t.Errorf("GET %s without a session = %d, want 200", path, res.StatusCode)
		}
	}
}

// TestLoginErrorsRerenderThePage checks that every refused local login comes
// back as the login page with an error line, the same status as before and the
// username still filled in, never as a bare text/plain body.
func TestLoginErrorsRerenderThePage(t *testing.T) {
	assertPage := func(t *testing.T, res *http.Response, status int, wants ...string) {
		t.Helper()
		if res.StatusCode != status {
			t.Fatalf("status = %d, want %d", res.StatusCode, status)
		}
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("Content-Type = %q, want the HTML login page", ct)
		}
		body := readBody(t, res)
		if !strings.Contains(body, `action="/auth/local"`) {
			t.Fatalf("response is not the login form:\n%s", body)
		}
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("page missing %q:\n%s", want, body)
			}
		}
		if c := cookie(t, res, "gpm_session"); c != nil && c.Value != "" {
			t.Error("a refused login minted a session")
		}
	}

	t.Run("wrong password keeps the username", func(t *testing.T) {
		e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true, noProviders: true})
		res := e.postForm("/auth/local", url.Values{"username": {"operator7"}, "password": {"nope"}, "return": {"/certs"}})
		assertPage(t, res, http.StatusUnauthorized,
			"Wrong username or password.", `name="username" value="operator7"`, `name="return" value="/certs"`)
	})

	t.Run("lockout names the minutes", func(t *testing.T) {
		e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true, noProviders: true})
		const lockout = "Too many attempts. Try again in 15 minutes."
		var res *http.Response
		for i := 0; i < 5; i++ { // auth.maxLoginFails
			res = e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"wrong"}})
		}
		// The failure that trips the lockout already says so.
		assertPage(t, res, http.StatusUnauthorized, lockout, `name="username" value="admin"`)
		res = e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"s3cret"}})
		assertPage(t, res, http.StatusTooManyRequests, lockout, `name="username" value="admin"`)
	})

	t.Run("wrong code goes back to step one", func(t *testing.T) {
		e := totpEnv(t)
		res := e.postForm("/auth/local", url.Values{"username": {"admin"}, "password": {"s3cret"}, "return": {"/"}})
		step2 := readBody(t, res)
		if !strings.Contains(step2, `name="username" value="admin"`) {
			t.Fatal("the code form does not carry the username back to step one")
		}
		_, rest, _ := strings.Cut(step2, `name="pending" value="`)
		pending, _, _ := strings.Cut(rest, `"`)
		if res.StatusCode != http.StatusOK || pending == "" {
			t.Fatalf("password step = %d with pending %q", res.StatusCode, pending)
		}
		res = e.postForm("/auth/local/totp", url.Values{
			"pending": {pending}, "code": {"000000"}, "return": {"/"}, "username": {"admin"},
		})
		assertPage(t, res, http.StatusUnauthorized, "Wrong code.", `name="username" value="admin"`)
	})

	t.Run("unreadable form", func(t *testing.T) {
		e := newTestEnv(t, envOpts{localUser: "admin", localPass: "s3cret", localEnabled: true, noProviders: true})
		req := httptest.NewRequest(http.MethodPost, "/auth/local", strings.NewReader("username=%zz"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		assertPage(t, e.do(req), http.StatusBadRequest, "The form could not be read.")
	})
}
