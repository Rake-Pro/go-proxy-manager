package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/oidc"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
	"golang.org/x/crypto/bcrypt"
)

// Authenticator coordinates admin-panel authentication: OIDC login and local
// password login. It maps IdP identities to roles and issues sessions.
//
// Trusted forward-auth HEADER login for the admin panel was removed: minting an
// admin session from X-* identity headers on peer-IP trust alone is a spoofing
// risk if anything in the trusted CIDR forwards client headers. The data-plane
// forward-auth that gates proxied hosts (internal/dataplane) is a separate
// feature and is unaffected.
type Authenticator struct {
	store      *session.Store
	cookieName string
	secure     bool
	sessionTTL time.Duration

	localUser string
	localHash string // bcrypt

	mu       sync.RWMutex
	baseURL  string
	appName  string
	idps     map[string]model.IdentityProvider
	settings model.AdminAuthSettings
	clients  map[string]*oidc.Client // cached OIDC clients by IdP name

	pmu     sync.Mutex
	pending map[string]pendingLogin // OIDC flow state -> login context

	lmu        sync.Mutex
	loginGates map[string]*loginGate // local-login throttle, keyed by client IP
}

// loginGate tracks recent failed local-login attempts for one client key.
type loginGate struct {
	fails   int
	resetAt time.Time // window/lockout expiry; after this the gate is cleared
}

const (
	maxLoginFails    = 5                // failures within the window before lockout
	loginLockout     = 15 * time.Minute // how long failures are remembered / locked
	maxPendingLogins = 1024             // cap on in-flight OIDC states (anti-DoS)
	maxLoginGates    = 4096             // cap on tracked client keys (anti-DoS)
)

type pendingLogin struct {
	idp      string
	nonce    string
	verifier string
	returnTo string
	expires  time.Time
}

// Options configures an Authenticator.
type Options struct {
	Store      *session.Store
	CookieName string
	Secure     bool // when true, the session cookie is Secure + __Host- prefixed
	SessionTTL time.Duration
	LocalUser  string
	LocalHash  string // bcrypt hash of the local admin password
}

// NewAuthenticator builds an Authenticator. Call Configure with the loaded
// config before serving.
func NewAuthenticator(o Options) *Authenticator {
	if o.CookieName == "" {
		o.CookieName = "gpm_session"
	}
	if o.SessionTTL == 0 {
		o.SessionTTL = 12 * time.Hour
	}
	// With Secure cookies, adopt the __Host- prefix: the browser then enforces
	// that the cookie is TLS-only, host-locked (no Domain), and Path=/ - all of
	// which the session cookie already satisfies. Gated on Secure (which honours
	// the operator's GPM_COOKIE_SECURE choice), so a plain-HTTP admin deployment
	// is unaffected - a __Host- cookie requires Secure and would be rejected.
	if o.Secure && !strings.HasPrefix(o.CookieName, "__Host-") && !strings.HasPrefix(o.CookieName, "__Secure-") {
		o.CookieName = "__Host-" + o.CookieName
	}
	return &Authenticator{
		store:      o.Store,
		cookieName: o.CookieName,
		secure:     o.Secure,
		sessionTTL: o.SessionTTL,
		localUser:  o.LocalUser,
		localHash:  o.LocalHash,
		idps:       map[string]model.IdentityProvider{},
		clients:    map[string]*oidc.Client{},
		pending:    map[string]pendingLogin{},
		loginGates: map[string]*loginGate{},
	}
}

// Configure updates the authenticator from the current config/settings. It
// clears the OIDC client cache so changed issuer/client settings take effect.
func (a *Authenticator) Configure(cfg model.Config, settings model.Settings) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.idps = map[string]model.IdentityProvider{}
	for _, p := range cfg.IdentityProviders {
		a.idps[p.Name] = p
	}
	a.settings = settings.AdminAuth
	a.baseURL = settings.ExternalBaseURL
	a.appName = settings.AppName
	a.clients = map[string]*oidc.Client{} // force rebuild on next use
}

// AppName is the brand label for the login page, defaulting when unset.
func (a *Authenticator) AppName() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.appName == "" {
		return "Go Proxy Manager"
	}
	return a.appName
}

// --- OIDC login -----------------------------------------------------------

func (a *Authenticator) oidcClient(ctx context.Context, idpName string) (*oidc.Client, *model.IdentityProvider, error) {
	a.mu.RLock()
	idp, ok := a.idps[idpName]
	client := a.clients[idpName]
	base := a.baseURL
	a.mu.RUnlock()
	if !ok {
		return nil, nil, fmt.Errorf("identity provider %q not found", idpName)
	}
	if idp.Type != model.IdPTypeOIDC || idp.OIDC == nil {
		return nil, nil, fmt.Errorf("identity provider %q is not OIDC", idpName)
	}
	if client != nil {
		return client, &idp, nil
	}
	if base == "" {
		return nil, nil, fmt.Errorf("externalBaseURL must be configured for OIDC login")
	}
	secret, err := idp.OIDC.ClientSecret.Resolve()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve client secret: %w", err)
	}
	usePKCE := idp.OIDC.UsePKCE == nil || *idp.OIDC.UsePKCE // default on
	c, err := oidc.New(ctx, oidc.Config{
		IssuerURL:            idp.OIDC.IssuerURL,
		ClientID:             idp.OIDC.ClientID,
		ClientSecret:         secret,
		RedirectURL:          base + "/auth/callback",
		Scopes:               idp.OIDC.Scopes,
		UsePKCE:              usePKCE,
		RequireVerifiedEmail: idp.OIDC.RequireVerifiedEmail,
		GroupsClaim:          groupsClaim(idp.RoleMapping),
	})
	if err != nil {
		return nil, nil, err
	}
	a.mu.Lock()
	a.clients[idpName] = c
	a.mu.Unlock()
	return c, &idp, nil
}

// oidcStateCookie holds the login state value so the callback can confirm the
// flow was started by this same browser (CSRF/login-fixation binding). It is
// short-lived and scoped to the auth endpoints.
const oidcStateCookie = "gpm_oidc_state"

// BeginLogin returns the authorization URL to redirect the browser to, along
// with the opaque state value the caller must echo into a browser cookie (see
// SetLoginStateCookie) so the callback can bind the flow to this browser.
func (a *Authenticator) BeginLogin(ctx context.Context, idpName, returnTo string) (string, string, error) {
	client, _, err := a.oidcClient(ctx, idpName)
	if err != nil {
		return "", "", err
	}
	state, err := oidc.NewState()
	if err != nil {
		return "", "", err
	}
	nonce, err := oidc.NewNonce()
	if err != nil {
		return "", "", err
	}
	verifier := oidc.GenerateVerifier()

	a.pmu.Lock()
	a.gcPendingLocked()
	if len(a.pending) >= maxPendingLogins {
		a.pmu.Unlock()
		return "", "", fmt.Errorf("too many pending logins; try again shortly")
	}
	a.pending[state] = pendingLogin{idp: idpName, nonce: nonce, verifier: verifier, returnTo: returnTo, expires: time.Now().Add(10 * time.Minute)}
	a.pmu.Unlock()

	return client.AuthCodeURL(state, nonce, verifier), state, nil
}

// SetLoginStateCookie stores the login state in a short-lived, SameSite=Lax
// cookie (Lax so it survives the top-level redirect back from the IdP). The
// callback compares it to the state query parameter to bind the flow.
func (a *Authenticator) SetLoginStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    state,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
}

// LoginStateCookie returns the login-state cookie value, or "" if absent.
func (a *Authenticator) LoginStateCookie(r *http.Request) string {
	if c, err := r.Cookie(oidcStateCookie); err == nil {
		return c.Value
	}
	return ""
}

// ClearLoginStateCookie expires the login-state cookie (single use).
func (a *Authenticator) ClearLoginStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// CompleteLogin handles the OIDC callback: validates state, exchanges the code,
// maps the identity to a role, and creates a session. Returns the new session
// and the post-login return path.
func (a *Authenticator) CompleteLogin(ctx context.Context, state, code string) (*session.Session, string, error) {
	a.pmu.Lock()
	p, ok := a.pending[state]
	delete(a.pending, state)
	a.pmu.Unlock()
	if !ok || time.Now().After(p.expires) {
		return nil, "", fmt.Errorf("invalid or expired login state")
	}

	client, idp, err := a.oidcClient(ctx, p.idp)
	if err != nil {
		return nil, "", err
	}
	claims, err := client.Exchange(ctx, code, p.verifier, p.nonce)
	if err != nil {
		return nil, "", err
	}

	role := MapRole(claims.Groups, idp.RoleMapping)
	if role == RoleNone {
		return nil, "", fmt.Errorf("user %q has no role mapping for provider %q", claims.Subject, p.idp)
	}
	sess := &session.Session{
		Subject:   claims.Subject,
		Email:     claims.Email,
		Name:      claims.Name,
		Roles:     []string{string(role)},
		IdP:       p.idp,
		AMR:       claims.AMR,
		ExpiresAt: time.Now().Add(a.sessionTTL),
	}
	if err := a.store.Create(ctx, sess); err != nil {
		return nil, "", err
	}
	return sess, p.returnTo, nil
}

// --- local login ----------------------------------------------------------

// LocalLogin verifies local admin credentials, subject to the SSO-only policy,
// and creates an admin session.
func (a *Authenticator) LocalLogin(ctx context.Context, user, pass string) (*session.Session, error) {
	if !a.localAllowed() {
		return nil, fmt.Errorf("local login is disabled")
	}
	if a.localUser == "" || a.localHash == "" {
		return nil, fmt.Errorf("no local admin configured")
	}
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.localUser)) == 1
	passOK := bcrypt.CompareHashAndPassword([]byte(a.localHash), []byte(pass)) == nil
	if !userOK || !passOK {
		return nil, fmt.Errorf("invalid credentials")
	}
	sess := &session.Session{
		Subject:   a.localUser,
		Name:      a.localUser,
		Roles:     []string{string(RoleAdmin)},
		IdP:       "local",
		AMR:       []string{"pwd"},
		ExpiresAt: time.Now().Add(a.sessionTTL),
	}
	if err := a.store.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// LoginThrottled reports whether local-login attempts from key are currently
// locked out after too many recent failures.
func (a *Authenticator) LoginThrottled(key string) bool {
	a.lmu.Lock()
	defer a.lmu.Unlock()
	g := a.loginGates[key]
	if g == nil {
		return false
	}
	if time.Now().After(g.resetAt) {
		delete(a.loginGates, key)
		return false
	}
	return g.fails >= maxLoginFails
}

// NoteLoginResult records the outcome of a local-login attempt for throttling:
// success clears the gate, failure increments it and (re)arms the window.
func (a *Authenticator) NoteLoginResult(key string, ok bool) {
	a.lmu.Lock()
	defer a.lmu.Unlock()
	now := time.Now()
	if ok {
		delete(a.loginGates, key)
		return
	}
	g := a.loginGates[key]
	if g == nil || now.After(g.resetAt) {
		// Opportunistically evict expired entries so the map can't grow without
		// bound; if it is still full, drop this record rather than allocate.
		if g == nil && len(a.loginGates) >= maxLoginGates {
			for k, gv := range a.loginGates {
				if now.After(gv.resetAt) {
					delete(a.loginGates, k)
				}
			}
			if len(a.loginGates) >= maxLoginGates {
				return
			}
		}
		g = &loginGate{}
		a.loginGates[key] = g
	}
	g.fails++
	g.resetAt = now.Add(loginLockout)
}

// localAllowed reports whether local password login is permitted: under SSO-only
// it is always off (recover from lockout by redeploying with local login
// enabled); otherwise it follows LocalLoginEnabled.
func (a *Authenticator) localAllowed() bool {
	a.mu.RLock()
	s := a.settings
	a.mu.RUnlock()
	if s.SSOOnly {
		return false
	}
	return s.LocalLoginEnabled
}

// --- session middleware ---------------------------------------------------

type ctxKey int

const principalKey ctxKey = 0

// Principal is the authenticated subject attached to a request context.
type Principal struct {
	Subject   string
	Email     string
	Name      string
	Role      Role
	IdP       string
	SessionID string
	// CSRFToken is the session's anti-CSRF token, surfaced to the SPA via
	// /api/me and required back as the X-CSRF-Token header on mutating requests.
	CSRFToken string `json:"csrfToken,omitempty"`
}

// PrincipalFrom returns the authenticated principal, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// RequireRole guards a handler: it requires a valid session cookie whose role
// satisfies min. On unsafe (state-changing) methods it also enforces the session
// CSRF token via the X-CSRF-Token header (double-submit). Otherwise it responds
// 401 (no session/role) or 403 (CSRF).
func (a *Authenticator) RequireRole(min Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := a.authenticate(w, r)
		if !ok || !roleSatisfies(p.Role, min) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if !safeMethod(r.Method) && !csrfTokenValid(r, p.CSRFToken) {
			http.Error(w, "invalid or missing CSRF token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

// authenticate resolves the session cookie to a principal and applies sliding
// expiry: an active session past the refresh threshold is extended in the store
// and its cookie re-issued, so continued use keeps the session alive.
func (a *Authenticator) authenticate(w http.ResponseWriter, r *http.Request) (Principal, bool) {
	c, err := r.Cookie(a.cookieName)
	if err != nil {
		return Principal{}, false
	}
	sess, err := a.store.Get(r.Context(), c.Value)
	if err != nil {
		return Principal{}, false
	}
	a.maybeSlide(w, r.Context(), sess)
	return principalOf(sess), true
}

// maybeSlide extends a still-valid session whose remaining lifetime has dropped
// below half the configured TTL, re-issuing the cookie with the new expiry. The
// half-TTL threshold avoids a store write and Set-Cookie on every request.
func (a *Authenticator) maybeSlide(w http.ResponseWriter, ctx context.Context, sess *session.Session) {
	remaining := time.Until(sess.ExpiresAt)
	if remaining <= 0 || remaining > a.sessionTTL/2 {
		return
	}
	newExp := time.Now().Add(a.sessionTTL)
	if err := a.store.Touch(ctx, sess.ID, newExp); err != nil {
		return
	}
	sess.ExpiresAt = newExp
	session.SetSessionCookie(w, a.cookieName, sess.ID, newExp, a.secure)
}

// IssueCookie writes the session cookie for a freshly created session.
func (a *Authenticator) IssueCookie(w http.ResponseWriter, sess *session.Session) {
	session.SetSessionCookie(w, a.cookieName, sess.ID, sess.ExpiresAt, a.secure)
}

// Logout clears the session and cookie.
func (a *Authenticator) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(a.cookieName); err == nil {
		_ = a.store.Delete(r.Context(), c.Value)
	}
	session.ClearSessionCookie(w, a.cookieName, a.secure)
}

// LoginOption describes an OIDC admin provider the user can click to log in.
// Name is the stable provider id (used to route the login request); Label is the
// human-facing button text (the provider's DisplayName, falling back to Name).
type LoginOption struct {
	Name  string
	Label string
	Type  string
}

// LoginOptions lists the OIDC admin providers (forward-auth needs no button).
func (a *Authenticator) LoginOptions() []LoginOption {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var opts []LoginOption
	for _, name := range a.settings.Providers {
		if idp, ok := a.idps[name]; ok && idp.Type == model.IdPTypeOIDC {
			label := idp.DisplayName
			if label == "" {
				label = name
			}
			opts = append(opts, LoginOption{Name: name, Label: label, Type: idp.Type})
		}
	}
	return opts
}

// LocalLoginVisible reports whether the local login form should be shown, per
// the SSO-only policy.
func (a *Authenticator) LocalLoginVisible() bool {
	return a.localUser != "" && a.localAllowed()
}

// --- helpers --------------------------------------------------------------

func (a *Authenticator) gcPendingLocked() {
	now := time.Now()
	for k, v := range a.pending {
		if now.After(v.expires) {
			delete(a.pending, k)
		}
	}
}

func principalOf(s *session.Session) Principal {
	role := RoleNone
	if len(s.Roles) > 0 {
		role = Role(s.Roles[0])
	}
	return Principal{Subject: s.Subject, Email: s.Email, Name: s.Name, Role: role, IdP: s.IdP, SessionID: s.ID, CSRFToken: s.CSRFToken}
}

// safeMethod reports whether m is a read-only HTTP method that needs no CSRF check.
func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// csrfTokenValid constant-time compares the request's X-CSRF-Token header against
// the session's token. An empty session token never matches.
func csrfTokenValid(r *http.Request, want string) bool {
	if want == "" {
		return false
	}
	got := r.Header.Get("X-CSRF-Token")
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// roleSatisfies treats admin as satisfying a user requirement.
func roleSatisfies(have, min Role) bool {
	switch min {
	case RoleUser:
		return have == RoleUser || have == RoleAdmin
	case RoleAdmin:
		return have == RoleAdmin
	default:
		return false
	}
}

func groupsClaim(rm *model.RoleMapping) string {
	if rm != nil && rm.GroupsClaim != "" {
		return rm.GroupsClaim
	}
	return "groups"
}
