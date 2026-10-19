package server

import (
	"crypto/subtle"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/clientip"
	"github.com/rs/zerolog/log"
)

// sameOriginGuard rejects state-changing cross-site requests as CSRF defense in
// depth. Unsafe methods must present a same-origin Sec-Fetch-Site, or (for
// clients that don't send it) an Origin header whose host matches the request
// host. A same-site-but-cross-subdomain request is rejected - that is exactly
// the sibling-subdomain gap SameSite=Lax leaves open.
func sameOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethod(r.Method) || originOK(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "cross-origin request blocked", http.StatusForbidden)
	})
}

func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func originOK(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "same-site", "cross-site":
		return false
	}
	// No Sec-Fetch-Site (older browser / non-browser client): fall back to Origin.
	// Absent Origin on a non-browser client carries no ambient-cookie CSRF risk;
	// the /api/ CSRF token still gates those routes.
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// loginShellCSS is the login-only layout. Everything else (fonts, the light and
// dark token blocks, buttons, form controls, banners) comes from the SPA's own
// app.css, linked by the pages below, so the first screen a user sees is drawn
// from the same tokens as the app behind it. theme-init.js applies the user's
// gpm.theme choice before first paint; with no choice, app.css follows
// prefers-color-scheme. Both are served unauthenticated by the UI handler, and
// an inline <style> block is within the existing style-src 'unsafe-inline'.
const loginShellCSS = `
  .login{min-height:calc(100vh - var(--masthead-h));display:flex;flex-direction:column;align-items:center;justify-content:center;padding:24px 16px}
  .login-masthead .wordmark{max-width:100%}
  .login-card{width:100%;max-width:400px;background:var(--panel);box-shadow:var(--shadow-sm);padding:32px 24px}
  .login-card h1{font-family:var(--display);font-size:24px;font-weight:500;margin:0 0 20px}
  .login-card h1 + .login-sub{margin-top:-16px}
  .login-sub{color:var(--muted);margin:0 0 20px}
  .login-field{margin-bottom:16px}
  .login-field label{display:block;margin-bottom:6px}
  .login .btn{display:flex;width:100%}
  .login-sso + .login-sso{margin-top:8px}
  .login-or{display:flex;align-items:center;gap:12px;color:var(--muted);margin:16px 0}
  .login-or::before,.login-or::after{content:"";flex:1;height:1px;background:var(--border)}
  .login-empty{color:var(--muted);text-align:center;margin:0}
  .login-err{margin:16px 0 0}
  .login-notice{margin:0 0 20px}
  .login-notice h2{font-size:14px;font-weight:700;margin:0}
  .login-notice code{font-family:var(--mono);font-size:13px}
  .login-notice .code{margin-top:8px;padding:8px 12px}
  .login-aside{font-family:var(--mono);font-size:13px;color:var(--muted)}
  input.field.otp{font-family:var(--mono);font-size:18px;letter-spacing:.35em;text-align:center}
`

// loginMasthead is the app's masthead on the sign-in pages: the same strip,
// logo tile and instance name as the admin shell, without the tools, so the
// first screen a user sees is drawn on the same frame as the app behind it.
// The glyph is the shell's ICON.fanout, kept in sync by hand.
const loginMasthead = `<header class="masthead login-masthead"><span class="wordmark"><span class="logo" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h6M10 12c3.5 0 3.5-5 7-5h2.5M10 12c3.5 0 3.5 5 7 5h2.5M17 4.6l2.4 2.4-2.4 2.4M17 14.6l2.4 2.4-2.4 2.4"/></svg></span><span class="name">{{.AppName}}</span></span></header>
<main class="login">`

// loginHead is the shared <head> of the login and verification-code pages.
// theme-init.js must stay ahead of app.css (see the comment in that file).
const loginHead = `<meta charset="utf-8">
<script src="/theme-init.js"></script>
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<link rel="preload" href="/fonts/red-hat-text-vf.woff2" as="font" type="font/woff2" crossorigin>
<link rel="stylesheet" href="/app.css">
<style>` + loginShellCSS + `</style>`

// loginPage renders every outcome of the first login step: the plain page, and
// the same page again with .Error set after a refused attempt (wrong
// credentials, a wrong second-factor code, a lockout or an unreadable form),
// keeping the submitted username so only the password has to be retyped.
var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en"><head>` + loginHead + `
<title>{{.AppName}} - Sign in</title></head>
<body>
` + loginMasthead + `
  <div class="login-card">
  <h1>Sign in</h1>
  {{if .NoAdminLogin}}<div class="ro-banner warn login-notice">
    <h2>No administrator sign-in is configured</h2>
    <ul>
      <li>Set <code>GPM_LOCAL_ADMIN_USER</code> and <code>GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE</code> on the container, then restart it.</li>
      <li>Or add an <code>oidc</code> identity provider and choose it under Admin sign-in &gt; Single sign-on providers in Settings <span class="login-aside">(adminAuth.providers)</span>.</li>
    </ul>
    <pre class="code">gpm hashpw &#39;your-password&#39; &gt; /run/secrets/gpm_admin_hash</pre>
  </div>{{end}}
  {{range .Providers}}<a class="btn primary login-sso" href="/auth/login?idp={{.Name}}&return={{$.Return}}">Sign in with {{.Label}}</a>{{end}}
  {{if .Local}}{{if .Providers}}<div class="login-or">or</div>{{end}}
  <form method="post" action="/auth/local">
    <input type="hidden" name="return" value="{{.Return}}">
    <div class="login-field"><label for="u">Username</label>
      <input id="u" class="field" name="username" value="{{.Username}}" placeholder="admin" autocomplete="username"{{if not .Username}} autofocus{{end}}></div>
    <div class="login-field"><label for="p">Password</label>
      <input id="p" class="field" name="password" type="password" placeholder="Password" autocomplete="current-password"{{if .Username}} autofocus{{end}}></div>
    <button class="btn primary" type="submit">Sign in</button>
  </form>{{end}}
  {{if not .Providers}}{{if not .Local}}<p class="login-empty">No sign-in methods are available</p>{{end}}{{end}}
  {{if .Error}}<p class="ro-banner err login-err" role="alert">{{.Error}}</p>{{end}}
  </div>
</main>
</body></html>`))

// totpPage is step two of local login: the password was accepted and a TOTP
// code is outstanding. It carries the opaque pending token forward in a hidden
// field - no session and no cookie exist yet at this point. The username rides
// along only so a wrong code can send the user back to step one with it filled
// in; the pending token is single-use, so a wrong code always means step one.
var totpPage = template.Must(template.New("totp").Parse(`<!doctype html>
<html lang="en"><head>` + loginHead + `
<title>{{.AppName}} - Verification code</title></head>
<body>
` + loginMasthead + `
  <div class="login-card">
  <h1>Verification code</h1>
  <p class="login-sub">Enter the code from your authenticator app</p>
  <form method="post" action="/auth/local/totp">
    <input type="hidden" name="pending" value="{{.Pending}}">
    <input type="hidden" name="return" value="{{.Return}}">
    <input type="hidden" name="username" value="{{.Username}}">
    <div class="login-field"><label for="c">Verification code</label>
      <input id="c" class="field otp" name="code" inputmode="numeric" pattern="[0-9]*" maxlength="6"
             autocomplete="one-time-code" placeholder="000000" autofocus></div>
    <button class="btn primary" type="submit">Verify</button>
  </form>
  </div>
</main>
</body></html>`))

// startLogin begins an OIDC flow: it sets the state-binding cookie and redirects
// the browser to the IdP. The callback verifies the cookie against the returned
// state to reject flows not started by this browser.
func (s *Server) startLogin(w http.ResponseWriter, r *http.Request, idp, returnTo string) {
	authURL, state, err := s.authn.BeginLogin(r.Context(), idp, returnTo, clientIPKey(r))
	if err != nil {
		log.Warn().Str("idp", idp).Err(err).Msg("begin login failed")
		s.renderLoginPage(w, http.StatusBadGateway, returnTo, "", msgSSOUnavailable)
		return
	}
	s.authn.SetLoginStateCookie(w, r, state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	returnTo := sanitizeReturnTo(r.URL.Query().Get("return"))
	if idp := r.URL.Query().Get("idp"); idp != "" {
		s.startLogin(w, r, idp, returnTo)
		return
	}

	// SSO-only with a single provider: skip the chooser and go straight to the
	// IdP - there is nothing to choose. ?select=1 (used by logout) forces the
	// page so a signed-out user is not immediately bounced back into a still-live
	// IdP session.
	if providers := s.authn.LoginOptions(); !s.authn.LocalLoginVisible() && len(providers) == 1 && r.URL.Query().Get("select") == "" {
		s.startLogin(w, r, providers[0].Name, returnTo)
		return
	}
	s.renderLoginPage(w, http.StatusOK, returnTo, "", "")
}

// Messages shown under the login form after a refused attempt.
const (
	msgWrongCredentials = "Wrong username or password."
	msgWrongCode        = "Wrong code. Enter your password, then a new code."
	msgBadForm          = "The form could not be read. Try again."
	msgSSOFailed        = "Sign-in with the identity provider failed. Try again."
	msgSSOIncomplete    = "The sign-in reply was incomplete. Try again."
	msgSSOWrongBrowser  = "This sign-in did not start in this browser. Try again."
	msgSSOUnavailable   = "Sign-in is unavailable right now. Try again in a moment."
)

// renderLoginPage draws step one of local login with the given status. A
// refused attempt re-renders it with errMsg under the form and the submitted
// username kept, never a bare text response.
func (s *Server) renderLoginPage(w http.ResponseWriter, status int, returnTo, username, errMsg string) {
	data := struct {
		AppName   string
		Providers []auth.LoginOption
		Local     bool
		Return    string
		Username  string
		Error     string
		// NoAdminLogin renders the bootstrap banner. Without it this page looks
		// identical whether admin auth is misconfigured or simply signed out: the
		// operator sees "No sign-in methods are available" with no way to learn
		// why or what to set.
		NoAdminLogin bool
	}{
		AppName:      s.authn.AppName(),
		Providers:    s.authn.LoginOptions(),
		Local:        s.authn.LocalLoginVisible(),
		Return:       returnTo,
		Username:     username,
		Error:        errMsg,
		NoAdminLogin: s.authn.NoAdminLoginConfigured(),
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = loginPage.Execute(w, data)
}

// lockoutMessage names how long the client stays locked out, in whole minutes
// rounded up so the page never says "0 minutes".
func (s *Server) lockoutMessage(key string) string {
	n := int((s.authn.LoginLockoutRemaining(key) + time.Minute - 1) / time.Minute)
	if n <= 1 {
		return "Too many attempts. Try again in 1 minute."
	}
	return fmt.Sprintf("Too many attempts. Try again in %d minutes.", n)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state, code := q.Get("state"), q.Get("code")
	// The state cookie is single use whatever happens next, and a pending login
	// behind a callback that cannot complete is dropped with it.
	cookieState := s.authn.LoginStateCookie(r)
	s.authn.ClearLoginStateCookie(w, r)
	// The IdP's error code goes to the log, trimmed, not to the page: it is
	// attacker-supplied text on a URL anyone can send.
	if e := q.Get("error"); e != "" {
		log.Warn().Str("idp_error", idpErrorForLog(e)).Msg("oidc callback refused by the identity provider")
		s.authn.AbandonLogin(state)
		s.renderLoginPage(w, http.StatusUnauthorized, "/", "", msgSSOFailed)
		return
	}
	if state == "" || code == "" {
		s.authn.AbandonLogin(state)
		s.renderLoginPage(w, http.StatusBadRequest, "/", "", msgSSOIncomplete)
		return
	}
	// Bind the flow to this browser: the state echoed by the IdP must match the
	// cookie set at login start. This blocks login-CSRF / session-fixation where
	// an attacker injects a callback URL carrying their own (server-valid) state.
	if cookieState == "" || subtle.ConstantTimeCompare([]byte(cookieState), []byte(state)) != 1 {
		s.renderLoginPage(w, http.StatusBadRequest, "/", "", msgSSOWrongBrowser)
		return
	}
	sess, returnTo, err := s.authn.CompleteLogin(r.Context(), state, code)
	if err != nil {
		log.Warn().Err(err).Msg("oidc callback failed")
		s.renderLoginPage(w, http.StatusUnauthorized, "/", "", msgSSOFailed)
		return
	}
	s.authn.IssueCookie(w, r, sess)
	http.Redirect(w, r, sanitizeReturnTo(returnTo), http.StatusFound)
}

func (s *Server) handleLocalLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderLoginPage(w, http.StatusBadRequest, "/", "", msgBadForm)
		return
	}
	returnTo := sanitizeReturnTo(r.PostForm.Get("return"))
	username := r.PostForm.Get("username")
	key := clientIPKey(r)
	if !s.authn.LoginAttempt(key) {
		s.renderLoginPage(w, http.StatusTooManyRequests, returnTo, username, s.lockoutMessage(key))
		return
	}
	sess, pending, err := s.authn.LocalLogin(r.Context(), username, r.PostForm.Get("password"))
	// A correct password with a second factor still outstanding is NOT a
	// success: clearing the gate here would let an attacker who already has the
	// password reset the lockout before every code guess, which would make the
	// throttle on the TOTP step meaningless. Only a completed login clears it.
	if err != nil || pending == "" {
		s.authn.NoteLoginResult(key, err == nil)
	}
	if err != nil {
		log.Warn().Err(err).Msg("local login failed")
		s.renderLoginPage(w, http.StatusUnauthorized, returnTo, username, s.failureMessage(key, msgWrongCredentials))
		return
	}
	if pending != "" {
		// Password accepted, second factor outstanding. No session and no cookie
		// exist yet: the only thing carried forward is the opaque, single-use,
		// short-lived pending token in this form.
		s.renderTOTPPage(w, pending, returnTo, username)
		return
	}
	s.authn.IssueCookie(w, r, sess)
	http.Redirect(w, r, returnTo, http.StatusFound)
}

// failureMessage is msg, or the lockout message when the failure just recorded
// was the one that locked the client out, so the user learns it now rather
// than on the next attempt.
func (s *Server) failureMessage(key, msg string) string {
	if s.authn.LoginThrottled(key) {
		return s.lockoutMessage(key)
	}
	return msg
}

// handleLocalTOTP is step two of local login: it redeems the pending token from
// handleLocalLogin against a TOTP code. A wrong code counts toward the SAME
// per-IP lockout as a wrong password, so guessing six digits is throttled
// exactly like guessing the password. The pending token is spent either way,
// so every failure goes back to step one.
func (s *Server) handleLocalTOTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderLoginPage(w, http.StatusBadRequest, "/", "", msgBadForm)
		return
	}
	returnTo := sanitizeReturnTo(r.PostForm.Get("return"))
	username := r.PostForm.Get("username")
	key := clientIPKey(r)
	if !s.authn.LoginAttempt(key) {
		s.renderLoginPage(w, http.StatusTooManyRequests, returnTo, username, s.lockoutMessage(key))
		return
	}
	sess, err := s.authn.CompleteTOTPLogin(r.Context(), r.PostForm.Get("pending"), r.PostForm.Get("code"))
	s.authn.NoteLoginResult(key, err == nil)
	if err != nil {
		log.Warn().Err(err).Msg("local login second factor failed")
		s.renderLoginPage(w, http.StatusUnauthorized, returnTo, username, s.failureMessage(key, msgWrongCode))
		return
	}
	s.authn.IssueCookie(w, r, sess)
	http.Redirect(w, r, returnTo, http.StatusFound)
}

// renderTOTPPage draws the second step of local login in the same shell as the
// login page.
func (s *Server) renderTOTPPage(w http.ResponseWriter, pending, returnTo, username string) {
	data := struct {
		AppName  string
		Pending  string
		Return   string
		Username string
	}{AppName: s.authn.AppName(), Pending: pending, Return: returnTo, Username: username}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = totpPage.Execute(w, data)
}

// clientIPKey is the throttle key for a login attempt: the client IP derived
// exactly the way the proxy listeners derive it, from settings.trustedProxies (see
// internal/clientip). RemoteAddr alone is wrong for the supported deployment in
// which the admin listener is bound to loopback and fronted by a gpm proxy host
// - every attempt then arrives from one address, so the login lockout, the TOTP
// throttle and the pending-login cap all collapse into a single global bucket
// and one attacker locks out every administrator.
//
// An untrusted peer's X-Forwarded-For is not read at all, so a direct client
// cannot mint itself a fresh bucket per attempt. With no trusted proxies
// configured this is exactly RemoteAddr, which is the previous behaviour.
// clientIPKey is the login gates' key for a client. IPv6 clients are grouped
// by /64: one subscriber holds a whole /64, so keying each address on its own
// would hand them a fresh gate per address.
func clientIPKey(r *http.Request) string {
	k := clientip.Key(r, clientip.Trusted())
	if ip := net.ParseIP(k); ip != nil && ip.To4() == nil {
		return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
	}
	return k
}

// idpErrorForLog trims an identity provider's error code to the RFC 6749
// charset and 64 bytes: it arrives on a URL anyone can send, and the log line
// is the only thing it may grow.
func idpErrorForLog(e string) string {
	var b strings.Builder
	for i := 0; i < len(e) && b.Len() < 64; i++ {
		c := e[i]
		if c == 0x20 || c == 0x21 || (c >= 0x23 && c <= 0x5b) || (c >= 0x5d && c <= 0x7e) {
			b.WriteByte(c)
		}
	}
	if len(e) > 64 {
		b.WriteString("...")
	}
	return b.String()
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.authn.Logout(w, r)
	// select=1 shows the login page instead of auto-bouncing back into the IdP,
	// so signing out actually lands on a sign-in screen.
	http.Redirect(w, r, "/auth/login?select=1", http.StatusFound)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	writeJSON(w, http.StatusOK, p)
}

// sanitizeReturnTo prevents open redirects: only same-site absolute paths pass.
// It must reject anything a browser could read as another origin: protocol-
// relative forms ("//evil.com") and backslash variants ("/\evil.com", "\evil"),
// since browsers normalize "\" to "/". A control char (CR/LF/NUL) also voids it.
func sanitizeReturnTo(p string) string {
	if p == "" || p[0] != '/' {
		return "/"
	}
	if strings.ContainsAny(p, "\\\r\n\x00") {
		return "/"
	}
	// Every other control byte too: http.Redirect passes a path url.Parse
	// refuses through unchanged, and browsers drop a TAB from a URL, which
	// turns "/\t/evil.com" into the "//evil.com" refused just below.
	for i := 0; i < len(p); i++ {
		if p[i] < 0x20 || p[i] == 0x7f {
			return "/"
		}
	}
	if strings.HasPrefix(p, "//") {
		return "/"
	}
	return p
}
