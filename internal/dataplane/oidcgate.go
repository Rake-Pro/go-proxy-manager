package dataplane

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/oidc"
	"github.com/rs/zerolog/log"
)

// Data-plane OIDC relying-party gating. A proxied host with auth mode "oidc"
// becomes its own OIDC client: unauthenticated requests are redirected to the
// IdP, the callback exchanges the code and issues a stateless signed session
// cookie, and subsequent requests are admitted (subject to role policy) without
// an upstream auth subrequest. This is distinct from forward-auth/auth-request,
// which delegate to an external authenticator.
const (
	// oidcCallbackPath is the reserved path gpm handles for the OIDC redirect.
	// The IdP must list https://<host>/__gpm/oidc/callback as a redirect URI.
	oidcCallbackPath  = "/__gpm/oidc/callback"
	oidcSessionCookie = "gpm_sso"
	oidcStateCookie   = "gpm_sso_state"
	oidcSessionTTL    = 12 * time.Hour
	oidcStateTTL      = 10 * time.Minute
)

// ssoSigningKey is the process-wide HMAC key for data-plane SSO cookies. It is
// taken from GPM_SSO_SIGNING_KEY when set (so sessions survive restarts) and is
// otherwise a random per-process key. Resolved once, lazily, so a deployment
// without any OIDC-gated host never logs the ephemeral-key notice.
var ssoSigningKey = sync.OnceValue(func() []byte {
	if v := strings.TrimSpace(os.Getenv("GPM_SSO_SIGNING_KEY")); v != "" {
		return []byte(v)
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		panic("dataplane: cannot generate SSO signing key: " + err.Error())
	}
	log.Warn().Msg("data-plane OIDC: using an ephemeral SSO signing key; set GPM_SSO_SIGNING_KEY to persist sessions across restarts")
	return k
})

// signToken returns payload as base64url(payload).base64url(HMAC-SHA256(payload)).
func signToken(payload []byte) string {
	mac := hmac.New(sha256.New, ssoSigningKey())
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyToken checks the HMAC and returns the payload bytes, or ok=false.
func verifyToken(tok string) ([]byte, bool) {
	i := strings.IndexByte(tok, '.')
	if i < 0 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(tok[:i])
	if err != nil {
		return nil, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(tok[i+1:])
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, ssoSigningKey())
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, false
	}
	return payload, true
}

// oidcSession is the signed end-user session cookie payload.
type oidcSession struct {
	Sub    string   `json:"sub"`
	Email  string   `json:"email,omitempty"`
	Name   string   `json:"name,omitempty"`
	Groups []string `json:"groups,omitempty"`
	Exp    int64    `json:"exp"`
}

// oidcLoginState is the signed, short-lived state cookie binding a login to this
// browser: it carries the OAuth2 state, the OIDC nonce, the PKCE verifier, and
// the URL to return to after login.
type oidcLoginState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Return   string `json:"r"`
	Exp      int64  `json:"exp"`
}

func setSignedCookie(w http.ResponseWriter, name string, v any, ttl time.Duration) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    signToken(b),
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // the data plane terminates TLS; SSO is HTTPS-only
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSignedCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", HttpOnly: true, Secure: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func readSignedCookie(r *http.Request, name string, v any) bool {
	c, err := r.Cookie(name)
	if err != nil {
		return false
	}
	payload, ok := verifyToken(c.Value)
	if !ok {
		return false
	}
	return json.Unmarshal(payload, v) == nil
}

// dataOIDC gates a proxied host as an OIDC relying party.
type dataOIDC struct {
	cfgTmpl     oidc.Config // RedirectURL is filled per request host
	roleMapping *model.RoleMapping
	required    []string
	hostName    string

	mu      sync.Mutex
	clients map[string]*oidc.Client // discovery result cached per request host
}

// compileDataOIDC builds the gate from an OIDC IdP. The client secret is resolved
// now (env/file), but OIDC discovery is deferred to the first request so a build
// never blocks on the network.
func compileDataOIDC(idp model.IdentityProvider, required []string, hostName string) (*dataOIDC, error) {
	secret, err := idp.OIDC.ClientSecret.Resolve()
	if err != nil {
		return nil, err
	}
	usePKCE := idp.OIDC.UsePKCE == nil || *idp.OIDC.UsePKCE
	groups := "groups"
	if idp.RoleMapping != nil && idp.RoleMapping.GroupsClaim != "" {
		groups = idp.RoleMapping.GroupsClaim
	}
	return &dataOIDC{
		cfgTmpl: oidc.Config{
			IssuerURL:            idp.OIDC.IssuerURL,
			ClientID:             idp.OIDC.ClientID,
			ClientSecret:         secret,
			Scopes:               idp.OIDC.Scopes,
			UsePKCE:              usePKCE,
			RequireVerifiedEmail: idp.OIDC.RequireVerifiedEmail,
			GroupsClaim:          groups,
		},
		roleMapping: idp.RoleMapping,
		required:    required,
		hostName:    hostName,
		clients:     map[string]*oidc.Client{},
	}, nil
}

// client returns the discovered OIDC client for a request host, building (and
// caching) it with that host's redirect URI on first use.
func (d *dataOIDC) client(ctx context.Context, host string) (*oidc.Client, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c, ok := d.clients[host]; ok {
		return c, nil
	}
	cfg := d.cfgTmpl
	cfg.RedirectURL = "https://" + host + oidcCallbackPath
	c, err := oidc.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	d.clients[host] = c
	return c, nil
}

// authorized applies the role policy. With neither a role mapping nor required
// roles, the gate means "require a valid SSO login" and any authenticated user
// passes; otherwise the mapped role must satisfy the requirement.
func (d *dataOIDC) authorized(groups []string) bool {
	if d.roleMapping == nil && len(d.required) == 0 {
		return true
	}
	return roleAllowed(auth.MapRole(groups, d.roleMapping), d.required)
}

func (d *dataOIDC) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == oidcCallbackPath {
			d.handleCallback(w, r)
			return
		}
		var sess oidcSession
		if readSignedCookie(r, oidcSessionCookie, &sess) && sess.Exp > time.Now().Unix() {
			if !d.authorized(sess.Groups) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			injectSSOIdentity(r, sess)
			next.ServeHTTP(w, r)
			return
		}
		d.beginLogin(w, r)
	})
}

func (d *dataOIDC) beginLogin(w http.ResponseWriter, r *http.Request) {
	client, err := d.client(r.Context(), r.Host)
	if err != nil {
		log.Warn().Str("host", d.hostName).Err(err).Msg("oidc discovery failed")
		http.Error(w, "authentication unavailable", http.StatusBadGateway)
		return
	}
	state, err := oidc.NewState()
	if err != nil {
		http.Error(w, "authentication unavailable", http.StatusInternalServerError)
		return
	}
	nonce, _ := oidc.NewNonce()
	verifier := oidc.GenerateVerifier()
	setSignedCookie(w, oidcStateCookie, oidcLoginState{
		State: state, Nonce: nonce, Verifier: verifier,
		Return: sanitizeSSOReturn(r.URL.RequestURI()),
		Exp:    time.Now().Add(oidcStateTTL).Unix(),
	}, oidcStateTTL)
	http.Redirect(w, r, client.AuthCodeURL(state, nonce, verifier), http.StatusFound)
}

func (d *dataOIDC) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Error(w, "login error: "+e, http.StatusUnauthorized)
		return
	}
	code, state := q.Get("code"), q.Get("state")
	var st oidcLoginState
	ok := readSignedCookie(r, oidcStateCookie, &st)
	clearSignedCookie(w, oidcStateCookie)
	if !ok || st.Exp < time.Now().Unix() || state == "" ||
		subtle.ConstantTimeCompare([]byte(st.State), []byte(state)) != 1 {
		http.Error(w, "invalid login state", http.StatusBadRequest)
		return
	}
	client, err := d.client(r.Context(), r.Host)
	if err != nil {
		http.Error(w, "authentication unavailable", http.StatusBadGateway)
		return
	}
	claims, err := client.Exchange(r.Context(), code, st.Verifier, st.Nonce)
	if err != nil {
		log.Warn().Str("host", d.hostName).Err(err).Msg("oidc exchange failed")
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}
	if !d.authorized(claims.Groups) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	setSignedCookie(w, oidcSessionCookie, oidcSession{
		Sub: claims.Subject, Email: claims.Email, Name: claims.Name,
		Groups: claims.Groups, Exp: time.Now().Add(oidcSessionTTL).Unix(),
	}, oidcSessionTTL)
	http.Redirect(w, r, sanitizeSSOReturn(st.Return), http.StatusFound)
}

// injectSSOIdentity sets authoritative identity headers for the upstream after a
// successful gate. The router already stripped client-forged identity headers
// from the (untrusted) browser, so these values are gpm's own.
func injectSSOIdentity(r *http.Request, s oidcSession) {
	user := s.Email
	if user == "" {
		user = s.Sub
	}
	r.Header.Set("X-Forwarded-User", user)
	if s.Email != "" {
		r.Header.Set("X-Forwarded-Email", s.Email)
	}
	if len(s.Groups) > 0 {
		r.Header.Set("X-Forwarded-Groups", strings.Join(s.Groups, ","))
	}
}

// sanitizeSSOReturn keeps the post-login redirect on this site: only a rooted,
// non-protocol-relative, control-char-free path is allowed.
func sanitizeSSOReturn(p string) string {
	if p == "" || p[0] != '/' || strings.HasPrefix(p, "//") ||
		strings.ContainsAny(p, "\\\r\n\x00") {
		return "/"
	}
	return p
}
