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
	store *session.Store
	// cookieName is the bare session-cookie name and secureCookieName its
	// __Host- prefixed twin. Which one is written is a per-request decision
	// (see secureFor); BOTH are accepted on the way in, so a session issued in
	// one mode survives the operator putting TLS in front of the panel.
	cookieName       string
	secureCookieName string
	secureMode       CookieSecureMode
	insecureWarn     insecureWarner
	sessionTTL       time.Duration

	localUser string
	localHash string // bcrypt

	// localTOTPSecret is the configured base32 TOTP secret for the local admin,
	// as supplied. Its PRESENCE enables the second factor; localTOTPKey is its
	// decoded form. An unparseable secret leaves the key nil while the secret
	// stays set, which fails closed: TOTP is demanded and no code can satisfy it.
	localTOTPSecret string
	localTOTPKey    []byte

	// otpmu guards otpLastCounter, the last TOTP step accepted per local user.
	// A code is single-use: re-presenting one that has already been accepted
	// (shoulder-surfed, or replayed off a proxied login POST) is refused.
	otpmu          sync.Mutex
	otpLastCounter map[string]uint64

	mu       sync.RWMutex
	baseURL  string
	appName  string
	idps     map[string]model.IdentityProvider
	settings model.AdminAuthSettings
	clients  map[string]*oidc.Client // cached OIDC clients by IdP name
	tokens   func() []model.APIToken // live API-token source (see SetTokenSource)

	tmu      sync.Mutex
	tokenUse map[string]time.Time // in-memory, non-persisted last-use per token name

	// tcmu guards the API-token cache (see currentTokens). It is separate from
	// mu because a cache miss calls out to the injected source, which loads the
	// whole config - work that must not be done holding the authenticator's
	// general-purpose lock.
	tcmu        sync.Mutex
	tokenCache  []model.APIToken
	tokenCached bool

	pmu     sync.Mutex
	pending map[string]pendingLogin // OIDC flow state -> login context

	loginGate   *rateGate // local-login failure throttle, keyed by client IP
	pendingGate *rateGate // OIDC begin-login throttle, keyed by client IP
}

// rateGate is a per-key rolling-window rate gate: it counts events for each key
// (a client IP) within a window and reports when a key has reached its limit. The
// map of keys is bounded (maxKeys) with opportunistic eviction of expired keys, so
// a flood of distinct keys cannot grow it without bound. It backs both the
// local-login failure throttle and the OIDC begin-login throttle, which differ
// only in their window and limit.
type rateGate struct {
	mu      sync.Mutex
	entries map[string]*gateEntry
	window  time.Duration // rolling window / lockout duration
	limit   int           // events within the window before the gate is "at limit"
	maxKeys int           // cap on tracked keys (anti-DoS)
}

// gateEntry tracks recent counted events for one key.
type gateEntry struct {
	fails   int
	resetAt time.Time // window/lockout expiry; after this the entry is cleared
}

func newRateGate(window time.Duration, limit, maxKeys int) *rateGate {
	return &rateGate{entries: map[string]*gateEntry{}, window: window, limit: limit, maxKeys: maxKeys}
}

// atLimit reports whether key has reached its limit within the current window.
// When evictExpired is set, an entry found past its window is deleted on the way
// out (opportunistic read-path cleanup); otherwise the entry is left untouched.
//
// Fail-closed under saturation: when the map is full of live entries, record()
// can no longer count new keys, so an untracked key is treated as at-limit rather
// than admitted unthrottled. This turns a distinct-key flood (an attacker holding
// maxKeys entries live) into a login lockout instead of a brute-force bypass -
// the safe failure mode for a credential gate.
func (g *rateGate) atLimit(key string, evictExpired bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.entries[key]
	if e == nil {
		return len(g.entries) >= g.maxKeys
	}
	if time.Now().After(e.resetAt) {
		if evictExpired {
			delete(g.entries, key)
		}
		return false
	}
	return e.fails >= g.limit
}

// record counts one event against key over a fresh window. It opportunistically
// evicts expired entries so the map can't grow without bound; if the map is still
// at capacity when a new key would be added, it skips recording rather than
// allocate. That skip is not a bypass: atLimit treats an untracked key as
// at-limit while the map is saturated, so the gate fails closed (see atLimit).
func (g *rateGate) record(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	e := g.entries[key]
	if e == nil || now.After(e.resetAt) {
		if e == nil && len(g.entries) >= g.maxKeys {
			for k, ev := range g.entries {
				if now.After(ev.resetAt) {
					delete(g.entries, k)
				}
			}
			if len(g.entries) >= g.maxKeys {
				return
			}
		}
		e = &gateEntry{}
		g.entries[key] = e
	}
	e.fails++
	e.resetAt = now.Add(g.window)
}

// clear removes key's entry, e.g. a successful login resets its gate.
func (g *rateGate) clear(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, key)
}

const (
	maxLoginFails    = 5                // failures within the window before lockout
	loginLockout     = 15 * time.Minute // how long failures are remembered / locked
	maxPendingLogins = 1024             // cap on in-flight OIDC states (anti-DoS)
	maxLoginGates    = 4096             // cap on tracked client keys (anti-DoS)

	// maxPendingPerIP caps how many OIDC logins one client IP may start within
	// pendingLoginWindow, so a client looping GET /auth/login cannot exhaust the
	// global maxPendingLogins budget for everyone. Kept generous so many users
	// behind a shared NAT are unaffected; a completed callback never counts here.
	maxPendingPerIP    = 30
	pendingLoginWindow = 10 * time.Minute
)

type pendingLogin struct {
	idp      string
	nonce    string
	verifier string
	returnTo string
	expires  time.Time
	// totpUser marks a half-completed LOCAL login: the password was verified and
	// only the TOTP code is outstanding. Non-empty exactly for those entries, so
	// the two flows share this map's bounds and garbage collection without an
	// OIDC state ever being redeemable as a TOTP one (or the reverse).
	totpUser string
}

// pendingTOTPTTL is how long a password-verified login may wait for its TOTP
// code. Short enough that a leaked pending token is near-worthless, long enough
// to open an authenticator app.
const pendingTOTPTTL = 5 * time.Minute

// Options configures an Authenticator.
type Options struct {
	Store      *session.Store
	CookieName string
	// SecureMode decides the Secure attribute on admin cookies. The zero value
	// is CookieSecureAuto: Secure (and the __Host- name) whenever the request is
	// TLS, forwarded as https by a trusted proxy, or externalBaseURL is https.
	SecureMode CookieSecureMode
	SessionTTL time.Duration
	LocalUser  string
	LocalHash  string // bcrypt hash of the local admin password
	// LocalTOTPSecret is the base32 TOTP secret for the local admin
	// (GPM_LOCAL_ADMIN_TOTP_SECRET / _FILE). Empty means no second factor.
	LocalTOTPSecret string
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
	// A Secure cookie takes the __Host- prefix: the browser then enforces that it
	// is TLS-only, host-locked (no Domain) and Path=/, all of which the session
	// cookie already satisfies. A non-Secure cookie MUST keep the bare name - a
	// browser rejects a __Host- cookie without Secure outright, which is exactly
	// how the plain-HTTP first login used to fail silently.
	o.CookieName = strings.TrimPrefix(strings.TrimPrefix(o.CookieName, "__Host-"), "__Secure-")
	if o.CookieName == "" {
		o.CookieName = "gpm_session"
	}
	// A malformed secret is NOT treated as "no TOTP": the key stays nil while the
	// secret stays set, so LocalTOTPEnabled reports true and every code is
	// refused. The daemon validates the value at startup (see cmd/gpm) and
	// refuses to start on a bad one, so this is the belt to that braces.
	var totpKey []byte
	if o.LocalTOTPSecret != "" {
		if k, err := NormalizeTOTPSecret(o.LocalTOTPSecret); err == nil {
			totpKey = k
		}
	}
	return &Authenticator{
		store:            o.Store,
		cookieName:       o.CookieName,
		secureCookieName: "__Host-" + o.CookieName,
		secureMode:       o.SecureMode,
		sessionTTL:       o.SessionTTL,
		localUser:        o.LocalUser,
		localHash:        o.LocalHash,
		localTOTPSecret:  o.LocalTOTPSecret,
		localTOTPKey:     totpKey,
		otpLastCounter:   map[string]uint64{},
		idps:             map[string]model.IdentityProvider{},
		clients:          map[string]*oidc.Client{},
		pending:          map[string]pendingLogin{},
		loginGate:        newRateGate(loginLockout, maxLoginFails, maxLoginGates),
		pendingGate:      newRateGate(pendingLoginWindow, maxPendingPerIP, maxLoginGates),
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
// clientKey (the client IP) is rate-limited so a client cannot fill the pending
// map and block OIDC logins for everyone.
func (a *Authenticator) BeginLogin(ctx context.Context, idpName, returnTo, clientKey string) (string, string, error) {
	if a.pendingLoginAtCap(clientKey) {
		return "", "", fmt.Errorf("too many login attempts; try again shortly")
	}
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

	// Count the attempt only now that a login state was actually created and
	// stored, so retries against a down/misconfigured IdP (which fail in
	// oidcClient above) don't burn the user's per-IP budget.
	a.recordPendingLogin(clientKey)

	return client.AuthCodeURL(state, nonce, verifier), state, nil
}

// SetLoginStateCookie stores the login state in a short-lived, SameSite=Lax
// cookie (Lax so it survives the top-level redirect back from the IdP). The
// callback compares it to the state query parameter to bind the flow.
func (a *Authenticator) SetLoginStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    state,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   a.cookieSecure(r),
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
func (a *Authenticator) ClearLoginStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   a.secureFor(r),
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
	if !ok || p.totpUser != "" || time.Now().After(p.expires) {
		// p.totpUser != "" is a half-finished LOCAL login being replayed at the
		// OIDC callback. It shares this map, so refuse it explicitly rather than
		// relying on the empty idp name failing later.
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

// LocalLogin verifies local admin credentials, subject to the SSO-only policy.
//
// It returns EITHER a session (password only, no second factor configured) or a
// pending-login token (TOTP is enabled: the password was right and the caller
// must now present a code to CompleteTOTPLogin). Exactly one of the two is
// non-empty on success.
func (a *Authenticator) LocalLogin(ctx context.Context, user, pass string) (*session.Session, string, error) {
	if !a.localAllowed() {
		return nil, "", fmt.Errorf("local login is disabled")
	}
	if a.localUser == "" || a.localHash == "" {
		return nil, "", fmt.Errorf("no local admin configured")
	}
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.localUser)) == 1
	passOK := bcrypt.CompareHashAndPassword([]byte(a.localHash), []byte(pass)) == nil
	if !userOK || !passOK {
		return nil, "", fmt.Errorf("invalid credentials")
	}
	if a.LocalTOTPEnabled() {
		token, err := a.startPendingTOTP()
		if err != nil {
			return nil, "", err
		}
		return nil, token, nil
	}
	sess, err := a.newLocalSession(ctx, []string{"pwd"})
	if err != nil {
		return nil, "", err
	}
	return sess, "", nil
}

// LocalTOTPEnabled reports whether the local admin must present a TOTP code
// after their password. It is on exactly when a secret was configured.
func (a *Authenticator) LocalTOTPEnabled() bool {
	return a.localTOTPSecret != ""
}

// startPendingTOTP mints the opaque token that carries a password-verified
// login to its second step. It reuses the pending-login map (and therefore its
// global cap and expiry sweep) so the two half-finished login flows share one
// bounded, garbage-collected store.
func (a *Authenticator) startPendingTOTP() (string, error) {
	token, err := oidc.NewState()
	if err != nil {
		return "", err
	}
	a.pmu.Lock()
	defer a.pmu.Unlock()
	a.gcPendingLocked()
	if len(a.pending) >= maxPendingLogins {
		return "", fmt.Errorf("too many pending logins; try again shortly")
	}
	a.pending[token] = pendingLogin{totpUser: a.localUser, expires: time.Now().Add(pendingTOTPTTL)}
	return token, nil
}

// CompleteTOTPLogin finishes a local login: it redeems the pending token from
// LocalLogin and, if code is a currently-valid and not-yet-used TOTP code,
// creates the admin session.
//
// The pending token is single-use whatever the outcome, so a wrong code costs a
// fresh password submission rather than an unlimited guessing loop against one
// token. Callers must still count the failure toward the per-IP login gate.
func (a *Authenticator) CompleteTOTPLogin(ctx context.Context, token, code string) (*session.Session, error) {
	a.pmu.Lock()
	p, ok := a.pending[token]
	delete(a.pending, token)
	a.pmu.Unlock()
	if !ok || p.totpUser == "" || time.Now().After(p.expires) {
		return nil, fmt.Errorf("invalid or expired login")
	}
	if !a.localAllowed() {
		return nil, fmt.Errorf("local login is disabled")
	}
	counter, ok := verifyTOTP(a.localTOTPKey, code, time.Now().Unix())
	if !ok {
		return nil, fmt.Errorf("invalid verification code")
	}
	if !a.acceptTOTPCounter(p.totpUser, counter) {
		return nil, fmt.Errorf("verification code has already been used")
	}
	return a.newLocalSession(ctx, []string{"pwd", "otp", "mfa"})
}

// acceptTOTPCounter records a successfully verified step for user and reports
// whether it was fresh. Replay protection is deliberately in-memory only: a
// process restart is a far rarer event than a login, and the alternative -
// committing a counter to the git-backed config on every sign-in - is worse.
func (a *Authenticator) acceptTOTPCounter(user string, counter uint64) bool {
	a.otpmu.Lock()
	defer a.otpmu.Unlock()
	if a.otpLastCounter == nil {
		a.otpLastCounter = map[string]uint64{}
	}
	if last, seen := a.otpLastCounter[user]; seen && counter <= last {
		return false
	}
	a.otpLastCounter[user] = counter
	return true
}

// newLocalSession mints the admin session for a completed local login. amr
// records how it was authenticated, so a downstream policy can tell a
// password-only session from one that also cleared a second factor.
func (a *Authenticator) newLocalSession(ctx context.Context, amr []string) (*session.Session, error) {
	sess := &session.Session{
		Subject:   a.localUser,
		Name:      a.localUser,
		Roles:     []string{string(RoleAdmin)},
		IdP:       "local",
		AMR:       amr,
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
	return a.loginGate.atLimit(key, true)
}

// NoteLoginResult records the outcome of a local-login attempt for throttling:
// success clears the gate, failure increments it and (re)arms the window.
func (a *Authenticator) NoteLoginResult(key string, ok bool) {
	if ok {
		a.loginGate.clear(key)
		return
	}
	a.loginGate.record(key)
}

// pendingLoginAtCap reports whether key has already hit its per-IP pending-login
// budget, without recording an attempt. BeginLogin checks this before the
// failure-prone IdP discovery (oidcClient) so a user's own retries against a
// down/misconfigured IdP don't burn their budget; the attempt is only counted
// via recordPendingLogin once a login state is actually created. The tiny
// check-then-record gap under concurrency is acceptable for rate limiting.
func (a *Authenticator) pendingLoginAtCap(key string) bool {
	return a.pendingGate.atLimit(key, false)
}

// recordPendingLogin counts one started OIDC login against key's per-IP budget.
// It mirrors the local-login gate: a per-IP counter over a rolling window. Only
// login starts (BeginLogin) are counted - a successful callback goes through
// CompleteLogin and never touches this gate.
func (a *Authenticator) recordPendingLogin(key string) {
	a.pendingGate.record(key)
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
	SessionID string `json:"-"`
	// CSRFToken is the session's anti-CSRF token, surfaced to the SPA via
	// /api/me and required back as the X-CSRF-Token header on mutating requests.
	CSRFToken string `json:"csrfToken,omitempty"`
	// IsToken marks a principal authenticated by an API token rather than a
	// session cookie. Such a principal is scope-limited and CSRF-exempt.
	IsToken bool `json:"isToken,omitempty"`
	// Scopes are the API token's granted scopes; empty for a session principal
	// (whose access is governed by its role alone).
	Scopes []string `json:"scopes,omitempty"`
	// ReadOnly is true for the `user` role: every write route refuses it, so the
	// SPA renders itself in a read-only mode (banner, Save buttons disabled)
	// rather than offering controls whose every submission answers 403.
	ReadOnly bool `json:"readOnly"`
}

// PrincipalFrom returns the authenticated principal, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// RequireRole guards a handler: it requires a valid session cookie (or API
// token) whose role satisfies min. On unsafe (state-changing) methods it also
// enforces the session CSRF token via the X-CSRF-Token header (double-submit).
// Otherwise it responds 401 (no session/role) or 403 (CSRF).
//
// API-token principals are exempt from the CSRF check: CSRF defends against a
// browser attaching ambient credentials (cookies) to a cross-site request, and a
// bearer token is never attached automatically - it must be set explicitly by
// the caller, which a cross-origin attacker cannot do.
func (a *Authenticator) RequireRole(min Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := a.authenticate(w, r)
		if !ok || !roleSatisfies(p.Role, min) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if !p.IsToken && !safeMethod(r.Method) && !csrfTokenValid(r, p.CSRFToken) {
			http.Error(w, "invalid or missing CSRF token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

// authenticate resolves the request to a principal. A gpm API token presented as
// `Authorization: Bearer gpm_...` is resolved FIRST and never falls through to
// the cookie path: a presented-but-invalid token is an authentication failure,
// not an invitation to try another credential. Otherwise the session cookie is
// resolved with sliding expiry - an active session past the refresh threshold is
// extended in the store and its cookie re-issued, so continued use keeps it alive.
func (a *Authenticator) authenticate(w http.ResponseWriter, r *http.Request) (Principal, bool) {
	if secret, ok := bearerTokenSecret(r); ok {
		return a.authenticateToken(secret)
	}
	c, hostPrefixed := a.sessionCookie(r)
	if c == nil {
		return Principal{}, false
	}
	// No downgrade. A __Host- cookie was minted for a Secure context; if this
	// request would not get a Secure cookie, honouring it means the sliding
	// re-issue would hand the same session id back without Secure. Fail closed
	// and make the operator log in again on the channel they are actually using.
	secure := a.secureFor(r)
	if hostPrefixed && !secure {
		return Principal{}, false
	}
	sess, err := a.store.Get(r.Context(), c.Value)
	if err != nil {
		return Principal{}, false
	}
	if sess.CSRFToken == "" {
		// A session with no anti-CSRF token is anomalous - every session minted by
		// store.Create has one. Treat it as invalid so a stale/legacy token-less
		// session forces a clean re-login (401) instead of silently failing every
		// mutating request with "invalid or missing CSRF token".
		return Principal{}, false
	}
	a.maybeSlide(w, r.Context(), sess, secure)
	return principalOf(sess), true
}

// sessionCookie returns the session cookie r presents, preferring the __Host-
// prefixed name, and reports whether that is the one found. Accepting both names
// is what lets a live session survive the operator putting TLS in front of the
// admin panel (or taking it away) without a forced re-login.
func (a *Authenticator) sessionCookie(r *http.Request) (*http.Cookie, bool) {
	if c, err := r.Cookie(a.secureCookieName); err == nil {
		return c, true
	}
	if c, err := r.Cookie(a.cookieName); err == nil {
		return c, false
	}
	return nil, false
}

// cookieNameFor is the __Host- name for a Secure cookie and the bare name
// otherwise.
func (a *Authenticator) cookieNameFor(secure bool) string {
	if secure {
		return a.secureCookieName
	}
	return a.cookieName
}

// maybeSlide extends a still-valid session whose remaining lifetime has dropped
// below half the configured TTL, re-issuing the cookie with the new expiry. The
// half-TTL threshold avoids a store write and Set-Cookie on every request.
func (a *Authenticator) maybeSlide(w http.ResponseWriter, ctx context.Context, sess *session.Session, secure bool) {
	remaining := time.Until(sess.ExpiresAt)
	if remaining <= 0 || remaining > a.sessionTTL/2 {
		return
	}
	newExp := time.Now().Add(a.sessionTTL)
	if err := a.store.Touch(ctx, sess.ID, newExp); err != nil {
		return
	}
	sess.ExpiresAt = newExp
	session.SetSessionCookie(w, a.cookieNameFor(secure), sess.ID, newExp, secure)
}

// IssueCookie writes the session cookie for a freshly created session, Secure
// (and __Host- named) or not according to how this request reached gpm.
func (a *Authenticator) IssueCookie(w http.ResponseWriter, r *http.Request, sess *session.Session) {
	secure := a.cookieSecure(r)
	session.SetSessionCookie(w, a.cookieNameFor(secure), sess.ID, sess.ExpiresAt, secure)
}

// Logout clears the session and BOTH cookie names: a session may have been
// issued under either one, and leaving the other in the browser would leave a
// dead-but-present credential behind.
func (a *Authenticator) Logout(w http.ResponseWriter, r *http.Request) {
	if c, _ := a.sessionCookie(r); c != nil {
		_ = a.store.Delete(r.Context(), c.Value)
	}
	session.ClearSessionCookie(w, a.cookieName, false)
	session.ClearSessionCookie(w, a.secureCookieName, true)
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

// LocalAdminConfigured reports whether a USABLE local credential exists - both
// the username and its bcrypt hash. LocalLoginVisible only says whether the FORM
// is rendered, which is also true for a half-configured pair (username set, hash
// missing) whose every submission fails with "authentication failed".
func (a *Authenticator) LocalAdminConfigured() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.localUser != "" && a.localHash != ""
}

// NoAdminLoginConfigured reports the bootstrap failure state: no usable local
// credential that policy allows AND no SSO provider that renders a sign-in
// button, so NOBODY can authenticate. This used to be visible only as one warn
// line in the startup log while the login page rendered as though it worked.
func (a *Authenticator) NoAdminLoginConfigured() bool {
	if len(a.LoginOptions()) > 0 {
		return false
	}
	return !(a.LocalAdminConfigured() && a.localAllowed())
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
	return Principal{
		Subject: s.Subject, Email: s.Email, Name: s.Name, Role: role, IdP: s.IdP,
		SessionID: s.ID, CSRFToken: s.CSRFToken, ReadOnly: role == RoleUser,
	}
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
