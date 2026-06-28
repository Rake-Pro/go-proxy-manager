package server

import (
	"crypto/subtle"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
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

var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.AppName}} - Sign in</title>
<style>
  :root{--bg1:#0b1220;--bg2:#020617;--card:#0f1729;--ink:#e2e8f0;--muted:#94a3b8;
        --line:#1e293b;--accent:#3b82f6;--accent-h:#60a5fa;--field:#0b1220}
  *{box-sizing:border-box}
  body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
       padding:1.5rem;font-family:system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;
       color:var(--ink);background:radial-gradient(1200px 600px at 50% -10%,#172033,var(--bg2)),linear-gradient(135deg,var(--bg1),var(--bg2))}
  .card{width:100%;max-width:23rem;background:var(--card);border:1px solid var(--line);border-radius:14px;
        box-shadow:0 20px 50px rgba(0,0,0,.55);padding:2.25rem 2rem 2rem}
  .brand{text-align:center;margin-bottom:1.5rem}
  .brand h1{font-size:1.2rem;font-weight:650;margin:0;letter-spacing:.01em;color:#f1f5f9}
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
</style></head>
<body>
<div class="card">
  <div class="brand"><h1>{{.AppName}}</h1><p>Sign in to continue</p></div>
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

// startLogin begins an OIDC flow: it sets the state-binding cookie and redirects
// the browser to the IdP. The callback verifies the cookie against the returned
// state to reject flows not started by this browser.
func (s *Server) startLogin(w http.ResponseWriter, r *http.Request, idp, returnTo string) {
	authURL, state, err := s.authn.BeginLogin(r.Context(), idp, returnTo)
	if err != nil {
		log.Warn().Str("idp", idp).Err(err).Msg("begin login failed")
		http.Error(w, "login unavailable", http.StatusBadGateway)
		return
	}
	s.authn.SetLoginStateCookie(w, state)
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
	}{
		AppName:   s.authn.AppName(),
		Providers: providers,
		Local:     local,
		Return:    returnTo,
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
	s.authn.ClearLoginStateCookie(w)
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
	s.authn.IssueCookie(w, sess)
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
	sess, err := s.authn.LocalLogin(r.Context(), r.PostForm.Get("username"), r.PostForm.Get("password"))
	s.authn.NoteLoginResult(key, err == nil)
	if err != nil {
		log.Warn().Err(err).Msg("local login failed")
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}
	s.authn.IssueCookie(w, sess)
	http.Redirect(w, r, sanitizeReturnTo(r.PostForm.Get("return")), http.StatusFound)
}

// clientIPKey is the throttle key for a login attempt: the connection peer IP
// (admin is reached directly, so RemoteAddr is the real client). Falls back to
// the raw RemoteAddr if it has no port.
func clientIPKey(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
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
