package server

import (
	"crypto/subtle"
	"html/template"
	"net/http"
	"net/url"
	"strings"

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

const loginShellCSS = `
  :root{--bg1:#0b1220;--bg2:#020617;--card:#0f1729;--ink:#e2e8f0;--muted:#94a3b8;
        --line:#1e293b;--accent:#3b82f6;--accent-h:#60a5fa;--field:#0b1220;
        --glow:#172033;--heading:#f1f5f9}
  @media (prefers-color-scheme:light){
    :root{--bg1:#eef1f8;--bg2:#f7f8fc;--card:#ffffff;--ink:#161b2c;--muted:#5b6377;
          --line:#dfe3ee;--accent:#3b82f6;--accent-h:#2f6fe0;--field:#f4f6fb;
          --glow:#dbe3f7;--heading:#161b2c}
  }
  *{box-sizing:border-box}
  body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
       padding:1.5rem;font-family:system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;
       color:var(--ink);background:radial-gradient(1200px 600px at 50% -10%,var(--glow),var(--bg2)),linear-gradient(135deg,var(--bg1),var(--bg2))}
  .card{width:100%;max-width:23rem;background:var(--card);border:1px solid var(--line);border-radius:14px;
        box-shadow:0 20px 50px rgba(0,0,0,.55);padding:2.25rem 2rem 2rem}
  .brand{text-align:center;margin-bottom:1.5rem}
  .brand h1{font-size:1.2rem;font-weight:650;margin:0;letter-spacing:.01em;color:var(--heading)}
  .brand p{margin:.4rem 0 0;color:var(--muted);font-size:.85rem}
  .btn{display:block;width:100%;border:0;border-radius:9px;padding:.72rem 1rem;
       font-size:.95rem;font-weight:600;cursor:pointer;text-align:center;text-decoration:none}
  .btn-primary{background:var(--accent);color:#fff;transition:background .15s}
  .btn-primary:hover{background:var(--accent-h)}
  .sso{margin-bottom:.6rem}
  .divider{display:flex;align-items:center;gap:.75rem;color:var(--muted);
           font-size:.72rem;text-transform:uppercase;letter-spacing:.1em;margin:1.25rem 0}
  .divider::before,.divider::after{content:"";flex:1;height:1px;background:var(--line)}
  label{display:block;font-size:.78rem;color:var(--muted);margin:0 0 .3rem}
  .field{margin-bottom:.9rem}
  input{width:100%;padding:.62rem .7rem;border:1px solid var(--line);border-radius:9px;
        background:var(--field);font-size:.95rem;color:var(--ink)}
  input::placeholder{color:#475569}
  input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 3px rgba(59,130,246,.25)}
  form{margin:0}
  .empty{color:var(--muted);text-align:center;font-size:.9rem;margin:0}
  .notice{border:1px solid var(--line);border-left:3px solid var(--accent);border-radius:9px;
          background:var(--field);padding:.9rem 1rem;margin-bottom:1.25rem}
  .notice h2{margin:0 0 .5rem;font-size:.92rem;font-weight:650;color:var(--heading)}
  .notice ul{margin:0;padding-left:1.1rem;color:var(--muted);font-size:.8rem;line-height:1.5}
  .notice li+li{margin-top:.35rem}
  .notice code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.76rem;color:var(--ink)}
  .notice pre{margin:.7rem 0 0;padding:.55rem .65rem;border-radius:7px;background:var(--bg2);
              color:var(--ink);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
              font-size:.74rem;overflow-x:auto}
`

var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.AppName}} - Sign in</title>
<style>` + loginShellCSS + `</style></head>
<body>
<div class="card">
  <div class="brand"><h1>{{.AppName}}</h1><p>Sign in to continue</p></div>
  {{if .NoAdminLogin}}<div class="notice">
    <h2>No administrator login is configured</h2>
    <ul>
      <li>Set <code>GPM_LOCAL_ADMIN_USER</code> and <code>GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE</code> on the container, then restart it.</li>
      <li>Or add an <code>oidc</code> identity provider and list its name under <code>adminAuth.providers</code> in settings.</li>
    </ul>
    <pre>gpm hashpw &#39;your-password&#39; &gt; /run/secrets/gpm_admin_hash</pre>
  </div>{{end}}
  {{range .Providers}}<a class="btn btn-primary sso" href="/auth/login?idp={{.Name}}&return={{$.Return}}">Login with {{.Label}}</a>{{end}}
  {{if .Local}}{{if .Providers}}<div class="divider">or</div>{{end}}
  <form method="post" action="/auth/local">
    <input type="hidden" name="return" value="{{.Return}}">
    <div class="field"><label for="u">Username</label>
      <input id="u" name="username" placeholder="admin" autocomplete="username" autofocus></div>
    <div class="field"><label for="p">Password</label>
      <input id="p" name="password" type="password" placeholder="Password" autocomplete="current-password"></div>
    <button class="btn btn-primary" type="submit">Log in</button>
  </form>{{end}}
  {{if not .Providers}}{{if not .Local}}<p class="empty">No login methods are available.</p>{{end}}{{end}}
</div>
</body></html>`))

// totpPage is step two of local login: the password was accepted and a TOTP
// code is outstanding. It carries the opaque pending token forward in a hidden
// field - no session and no cookie exist yet at this point.
var totpPage = template.Must(template.New("totp").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.AppName}} - Verification code</title>
<style>` + loginShellCSS + `
  .code{letter-spacing:.35em;text-align:center;font-size:1.15rem;
        font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
  .err{border:1px solid var(--line);border-left:3px solid #ef4444;border-radius:9px;
       background:var(--field);padding:.7rem .8rem;margin-bottom:1rem;font-size:.82rem}
</style></head>
<body>
<div class="card">
  <div class="brand"><h1>{{.AppName}}</h1><p>Enter the code from your authenticator app</p></div>
  {{if .Error}}<div class="err">{{.Error}}</div>{{end}}
  <form method="post" action="/auth/local/totp">
    <input type="hidden" name="pending" value="{{.Pending}}">
    <input type="hidden" name="return" value="{{.Return}}">
    <div class="field"><label for="c">Verification code</label>
      <input id="c" class="code" name="code" inputmode="numeric" pattern="[0-9]*" maxlength="6"
             autocomplete="one-time-code" placeholder="000000" autofocus></div>
    <button class="btn btn-primary" type="submit">Verify</button>
  </form>
</div>
</body></html>`))

// startLogin begins an OIDC flow: it sets the state-binding cookie and redirects
// the browser to the IdP. The callback verifies the cookie against the returned
// state to reject flows not started by this browser.
func (s *Server) startLogin(w http.ResponseWriter, r *http.Request, idp, returnTo string) {
	authURL, state, err := s.authn.BeginLogin(r.Context(), idp, returnTo, clientIPKey(r))
	if err != nil {
		log.Warn().Str("idp", idp).Err(err).Msg("begin login failed")
		http.Error(w, "login unavailable", http.StatusBadGateway)
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
	providers := s.authn.LoginOptions()
	local := s.authn.LocalLoginVisible()

	// SSO-only with a single provider: skip the chooser and go straight to the
	// IdP - there is nothing to choose. ?select=1 (used by logout) forces the
	// page so a signed-out user is not immediately bounced back into a still-live
	// IdP session.
	if !local && len(providers) == 1 && r.URL.Query().Get("select") == "" {
		s.startLogin(w, r, providers[0].Name, returnTo)
		return
	}

	data := struct {
		AppName   string
		Providers []auth.LoginOption
		Local     bool
		Return    string
		// NoAdminLogin renders the bootstrap banner. Without it this page looks
		// identical whether admin auth is misconfigured or simply signed out: the
		// operator sees "No login methods are available." with no way to learn why
		// or what to set.
		NoAdminLogin bool
	}{
		AppName:      s.authn.AppName(),
		Providers:    providers,
		Local:        local,
		Return:       returnTo,
		NoAdminLogin: s.authn.NoAdminLoginConfigured(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginPage.Execute(w, data)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Error(w, "login error: "+e, http.StatusUnauthorized)
		return
	}
	state, code := q.Get("state"), q.Get("code")
	if state == "" || code == "" {
		http.Error(w, "missing state or code", http.StatusBadRequest)
		return
	}
	// Bind the flow to this browser: the state echoed by the IdP must match the
	// cookie set at login start. This blocks login-CSRF / session-fixation where
	// an attacker injects a callback URL carrying their own (server-valid) state.
	cookieState := s.authn.LoginStateCookie(r)
	s.authn.ClearLoginStateCookie(w, r)
	if cookieState == "" || subtle.ConstantTimeCompare([]byte(cookieState), []byte(state)) != 1 {
		http.Error(w, "invalid login state", http.StatusBadRequest)
		return
	}
	sess, returnTo, err := s.authn.CompleteLogin(r.Context(), state, code)
	if err != nil {
		log.Warn().Err(err).Msg("oidc callback failed")
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}
	s.authn.IssueCookie(w, r, sess)
	http.Redirect(w, r, sanitizeReturnTo(returnTo), http.StatusFound)
}

func (s *Server) handleLocalLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	key := clientIPKey(r)
	if s.authn.LoginThrottled(key) {
		http.Error(w, "too many login attempts; try again later", http.StatusTooManyRequests)
		return
	}
	returnTo := sanitizeReturnTo(r.PostForm.Get("return"))
	sess, pending, err := s.authn.LocalLogin(r.Context(), r.PostForm.Get("username"), r.PostForm.Get("password"))
	// A correct password with a second factor still outstanding is NOT a
	// success: clearing the gate here would let an attacker who already has the
	// password reset the lockout before every code guess, which would make the
	// throttle on the TOTP step meaningless. Only a completed login clears it.
	if err != nil || pending == "" {
		s.authn.NoteLoginResult(key, err == nil)
	}
	if err != nil {
		log.Warn().Err(err).Msg("local login failed")
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}
	if pending != "" {
		// Password accepted, second factor outstanding. No session and no cookie
		// exist yet: the only thing carried forward is the opaque, single-use,
		// short-lived pending token in this form.
		s.renderTOTPPage(w, pending, returnTo, "")
		return
	}
	s.authn.IssueCookie(w, r, sess)
	http.Redirect(w, r, returnTo, http.StatusFound)
}

// handleLocalTOTP is step two of local login: it redeems the pending token from
// handleLocalLogin against a TOTP code. A wrong code counts toward the SAME
// per-IP lockout as a wrong password, so guessing six digits is throttled
// exactly like guessing the password.
func (s *Server) handleLocalTOTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	key := clientIPKey(r)
	if s.authn.LoginThrottled(key) {
		http.Error(w, "too many login attempts; try again later", http.StatusTooManyRequests)
		return
	}
	returnTo := sanitizeReturnTo(r.PostForm.Get("return"))
	sess, err := s.authn.CompleteTOTPLogin(r.Context(), r.PostForm.Get("pending"), r.PostForm.Get("code"))
	s.authn.NoteLoginResult(key, err == nil)
	if err != nil {
		log.Warn().Err(err).Msg("local login second factor failed")
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}
	s.authn.IssueCookie(w, r, sess)
	http.Redirect(w, r, returnTo, http.StatusFound)
}

// renderTOTPPage draws the second step of local login in the same shell as the
// login page.
func (s *Server) renderTOTPPage(w http.ResponseWriter, pending, returnTo, errMsg string) {
	data := struct {
		AppName string
		Pending string
		Return  string
		Error   string
	}{AppName: s.authn.AppName(), Pending: pending, Return: returnTo, Error: errMsg}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = totpPage.Execute(w, data)
}

// clientIPKey is the throttle key for a login attempt: the client IP derived
// exactly the way the data plane derives it, from settings.trustedProxies (see
// internal/clientip). RemoteAddr alone is wrong for the supported deployment in
// which the admin listener is bound to loopback and fronted by a gpm proxy host
// - every attempt then arrives from one address, so the login lockout, the TOTP
// throttle and the pending-login cap all collapse into a single global bucket
// and one attacker locks out every administrator.
//
// An untrusted peer's X-Forwarded-For is not read at all, so a direct client
// cannot mint itself a fresh bucket per attempt. With no trusted proxies
// configured this is exactly RemoteAddr, which is the previous behaviour.
func clientIPKey(r *http.Request) string {
	return clientip.Key(r, clientip.Trusted())
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
	if strings.HasPrefix(p, "//") {
		return "/"
	}
	return p
}
