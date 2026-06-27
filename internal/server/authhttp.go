package server

import (
	"html/template"
	"net"
	"net/http"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/rs/zerolog/log"
)

var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>go-proxy-manager login</title></head>
<body style="font-family:system-ui;max-width:24rem;margin:4rem auto">
<h1>Sign in</h1>
{{range .Providers}}<p><a href="/auth/login?idp={{.Name}}&return={{$.Return}}">Continue with {{.Name}}</a></p>{{end}}
{{if .Local}}<form method="post" action="/auth/local">
<input type="hidden" name="return" value="{{.Return}}">
<p><input name="username" placeholder="username" autocomplete="username"></p>
<p><input name="password" type="password" placeholder="password" autocomplete="current-password"></p>
<p><button type="submit">Log in</button></p>
</form>{{end}}
{{if not .Providers}}{{if not .Local}}<p>No login methods are available.</p>{{end}}{{end}}
</body></html>`))

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	returnTo := sanitizeReturnTo(r.URL.Query().Get("return"))
	if idp := r.URL.Query().Get("idp"); idp != "" {
		url, err := s.authn.BeginLogin(r.Context(), idp, returnTo)
		if err != nil {
			log.Warn().Str("idp", idp).Err(err).Msg("begin login failed")
			http.Error(w, "login unavailable", http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
		return
	}
	data := struct {
		Providers []auth.LoginOption
		Local     bool
		Return    string
	}{
		Providers: s.authn.LoginOptions(),
		Local:     s.authn.LocalLoginVisible(clientIP(r)),
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
	sess, err := s.authn.LocalLogin(r.Context(), r.PostForm.Get("username"), r.PostForm.Get("password"), clientIP(r))
	if err != nil {
		log.Warn().Err(err).Msg("local login failed")
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}
	s.authn.IssueCookie(w, sess)
	http.Redirect(w, r, sanitizeReturnTo(r.PostForm.Get("return")), http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.authn.Logout(w, r)
	http.Redirect(w, r, "/auth/login", http.StatusFound)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	writeJSON(w, http.StatusOK, p)
}

// sanitizeReturnTo prevents open redirects: only same-site absolute paths pass.
func sanitizeReturnTo(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return "/"
	}
	return p
}

func clientIP(r *http.Request) net.IP {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	return net.ParseIP(strings.TrimSpace(host))
}
