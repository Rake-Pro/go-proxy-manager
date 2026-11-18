package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
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
	pending map[string]pendingLogin // pending-TOTP token -> password-verified local login

	// stateKey signs the OIDC login-state cookie (see signLoginState). It is
	// random per process: a login started before a restart fails at its
	// callback and is simply started again.
	stateKey []byte

	loginGate *rateGate // local-login failure throttle, keyed by client IP
	userGate  *rateGate // local-login failure throttle, keyed by username (distributed guessing)
}

// rateGate is a per-key rolling-window rate gate: it counts events for each key
// (a client IP or a username) within a window and reports when a key has reached
// its limit. The map of keys is bounded (maxKeys) with opportunistic eviction of
// expired keys, so a flood of distinct keys cannot grow it without bound. It
// backs the local-login failure throttles.
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
		return g.saturatedLocked(time.Now())
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
		if e == nil && g.saturatedLocked(now) {
			return
		}
		e = &gateEntry{}
		g.entries[key] = e
	}
	e.fails++
	e.resetAt = now.Add(g.window)
}

// tryBegin reserves one attempt for key, or reports that key is at its limit.
// Reserve-then-verify under one lock: the check and the count happen together,
// so a burst of parallel requests cannot all read "not at limit" before any of
// them is counted. A successful attempt is forgiven with clear(); a failed one
// stays counted. Refused attempts do not extend the lockout.
func (g *rateGate) tryBegin(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	e := g.entries[key]
	if e != nil && now.After(e.resetAt) {
		delete(g.entries, key)
		e = nil
	}
	if e == nil {
		if g.saturatedLocked(now) {
			return false
		}
		e = &gateEntry{}
		g.entries[key] = e
	} else if e.fails >= g.limit {
		return false
	}
	e.fails++
	e.resetAt = now.Add(g.window)
	return true
}

// sweepLocked drops every entry whose window has passed. Called with g.mu held.
func (g *rateGate) sweepLocked(now time.Time) {
	for k, ev := range g.entries {
		if now.After(ev.resetAt) {
			delete(g.entries, k)
		}
	}
}

// saturatedLocked reports whether the map is full of LIVE entries, sweeping
// expired ones first: a map full of dead entries must not keep every new
// client locked out until the process restarts. Called with g.mu held.
func (g *rateGate) saturatedLocked(now time.Time) bool {
	if len(g.entries) < g.maxKeys {
		return false
	}
	g.sweepLocked(now)
	return len(g.entries) >= g.maxKeys
}

// clear removes key's entry, e.g. a successful login resets its gate.
func (g *rateGate) clear(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, key)
}

// remaining reports how long key stays at its limit, or zero when it is not.
// A key left untracked by a saturated map is at limit (see atLimit) for as long
// as a fresh window, which is the most a tracked key could wait.
func (g *rateGate) remaining(key string) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.entries[key]
	if e == nil {
		if g.saturatedLocked(time.Now()) {
			return g.window
		}
		return 0
	}
	d := time.Until(e.resetAt)
	if d <= 0 || e.fails < g.limit {
		return 0
	}
	return d
}

const (
	maxLoginFails = 5 // failures within the window before lockout
	// Per username the limit is four times the per-IP one: the counter exists to
	// bound a distributed guess against one account, not to let anyone lock an
	// operator out of local login with five bad passwords from one client.
	maxUserLoginFails = 20
	// The username map is capped at what it takes to fill the IP map at the
	// per-IP limit, so filling it is never the cheaper way to lock local login
	// for everyone (keys are at most 64 bytes).
	maxUserLoginGates = maxLoginFails * maxLoginGates
	loginLockout      = 15 * time.Minute // how long failures are remembered / locked
	maxPendingLogins  = 1024             // cap on in-flight pending-TOTP logins (anti-DoS)
	maxLoginGates     = 4096             // cap on tracked client keys (anti-DoS)
)

// pendingLogin is a half-completed LOCAL login: the password was verified and
// only the TOTP code for totpUser is outstanding. Only a correct password
// creates one, so anonymous requests cannot fill the map.
type pendingLogin struct {
	expires  time.Time
	totpUser string
}

// maxSessionAge is the absolute lifetime of an admin session, counted from its
// creation. Sliding expiry extends a session in use, but never past this: the
// role mapped at login is not re-checked against the identity provider, so a
// session that is used every day must still end and sign in again.
const maxSessionAge = 7 * 24 * time.Hour

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
	stateKey := make([]byte, 32)
	if _, err := rand.Read(stateKey); err != nil {
		panic("auth: cannot generate the login-state signing key: " + err.Error())
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
		stateKey:         stateKey,
		loginGate:        newRateGate(loginLockout, maxLoginFails, maxLoginGates),
		userGate:         newRateGate(loginLockout, maxUserLoginFails, maxUserLoginGates),
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

// oidcClient returns the OIDC client for an admin login provider. Only a name
// listed in adminAuth.providers is one: every other configured provider
// (a site gate, for example) answers exactly like a name that does not exist.
// Both BeginLogin and CompleteLogin go through here, so a provider removed
// from the list between the start of a login and its callback is refused too.
func (a *Authenticator) oidcClient(ctx context.Context, idpName string) (*oidc.Client, *model.IdentityProvider, error) {
	a.mu.RLock()
	idp, ok := a.idps[idpName]
	listed := slices.Contains(a.settings.Providers, idpName)
	client := a.clients[idpName]
	base := a.baseURL
	a.mu.RUnlock()
	if !ok || !listed {
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

// oidcStateCookie carries the signed login state (see loginState) so the
// callback can confirm the flow was started by this same browser
// (CSRF/login-fixation binding). It is short-lived and scoped to the auth
// endpoints.
//
// On a Secure request it is oidcStateCookieSecure with Path=/ instead: a
// __Host- cookie can only be set by this host itself, so a sibling host under
// the same parent domain cannot plant one to break or hijack the callback.
const (
	oidcStateCookie       = "gpm_oidc_state"
	oidcStateCookieSecure = "__Host-gpm_oidc_state"
)

// loginStateCookieName is the login-state cookie's name and path for a request
// whose cookies are (or are not) Secure.
func loginStateCookieName(secure bool) (name, path string) {
	if secure {
		return oidcStateCookieSecure, "/"
	}
	return oidcStateCookie, "/auth"
}

// loginStateTTL bounds an OIDC login from its start to its callback.
const loginStateTTL = 10 * time.Minute

// maxLoginReturnLen caps the return path carried in the login-state cookie, so
// the cookie stays well inside browser size limits.
const maxLoginReturnLen = 1024

// macLabelLoginState is mixed into the login-state MAC so the key cannot be
// used to forge any other token type.
const macLabelLoginState = "gpm-admin-login-state-v1|"

// ErrLoginState is returned by CompleteLogin when the login-state cookie is
// absent, not signed by this process, expired, or for a different flow than
// the state the identity provider echoed back.
var ErrLoginState = errors.New("invalid or expired login state")

// loginState is everything the callback needs to finish an OIDC login. It
// travels in the signed, HttpOnly login-state cookie instead of a server-side
// map, so starting a login stores nothing on the server and no number of
// anonymous login starts can block another client's login.
type loginState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	IdP      string `json:"i"`
	Return   string `json:"r"`
	Exp      int64  `json:"exp"`
}

// signLoginState encodes st as base64url(json).base64url(HMAC-SHA256(label ||
// json)), the format of the data-plane SSO cookies.
func (a *Authenticator) signLoginState(st loginState) (string, error) {
	b, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, a.stateKey)
	mac.Write([]byte(macLabelLoginState))
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// readLoginState verifies and decodes a signLoginState value.
func (a *Authenticator) readLoginState(v string) (loginState, bool) {
	var st loginState
	payloadPart, sigPart, ok := strings.Cut(v, ".")
	if !ok {
		return st, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return st, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil {
		return st, false
	}
	mac := hmac.New(sha256.New, a.stateKey)
	mac.Write([]byte(macLabelLoginState))
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return st, false
	}
	return st, json.Unmarshal(payload, &st) == nil
}

// BeginLogin returns the authorization URL to redirect the browser to, along
// with the signed login state the caller must store in a browser cookie (see
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
	if len(returnTo) > maxLoginReturnLen {
		returnTo = "/"
	}
	cookie, err := a.signLoginState(loginState{
		State: state, Nonce: nonce, Verifier: verifier, IdP: idpName, Return: returnTo,
		Exp: time.Now().Add(loginStateTTL).Unix(),
	})
	if err != nil {
		return "", "", err
	}
	return client.AuthCodeURL(state, nonce, verifier), cookie, nil
}

// SetLoginStateCookie stores the signed login state in a short-lived,
// SameSite=Lax cookie (Lax so it survives the top-level redirect back from the
// IdP). The callback hands it to CompleteLogin, which checks it against the
// state query parameter to bind the flow.
func (a *Authenticator) SetLoginStateCookie(w http.ResponseWriter, r *http.Request, value string) {
	secure := a.cookieSecure(r)
	name, path := loginStateCookieName(secure)
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(loginStateTTL.Seconds()),
	})
}

// LoginStateCookie returns the login-state cookie value that verifies, is
// unexpired and carries state, or "" when none does. Every cookie of the name
// is tried, so another cookie of the same name sent first cannot hide the
// real one.
func (a *Authenticator) LoginStateCookie(r *http.Request, state string) string {
	name, _ := loginStateCookieName(a.secureFor(r))
	for _, c := range r.CookiesNamed(name) {
		if st, ok := a.readLoginState(c.Value); ok && st.Exp >= time.Now().Unix() && state != "" &&
			subtle.ConstantTimeCompare([]byte(st.State), []byte(state)) == 1 {
			return c.Value
		}
	}
	return ""
}

// ClearLoginStateCookie expires the login-state cookie (single use).
func (a *Authenticator) ClearLoginStateCookie(w http.ResponseWriter, r *http.Request) {
	secure := a.secureFor(r)
	name, path := loginStateCookieName(secure)
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// CompleteLogin handles the OIDC callback: validates the signed login state
// from the cookie against the state the IdP echoed back, exchanges the code,
// maps the identity to a role, and creates a session. Returns the new session
// and the post-login return path. A bad login state is ErrLoginState.
//
// The state is not single-use on the server: the caller clears the cookie at
// the callback, and the IdP redeems each authorization code only once.
func (a *Authenticator) CompleteLogin(ctx context.Context, stateCookie, state, code string) (*session.Session, string, error) {
	p, ok := a.readLoginState(stateCookie)
	if !ok || p.Exp < time.Now().Unix() || state == "" ||
		subtle.ConstantTimeCompare([]byte(p.State), []byte(state)) != 1 {
		return nil, "", ErrLoginState
	}

	client, idp, err := a.oidcClient(ctx, p.IdP)
	if err != nil {
		return nil, "", err
	}
	claims, err := client.Exchange(ctx, code, p.Verifier, p.Nonce)
	if err != nil {
		return nil, "", err
	}

	role := MapRole(claims.Groups, idp.RoleMapping)
	if role == RoleNone {
		return nil, "", fmt.Errorf("user %q has no role mapping for provider %q", claims.Subject, p.IdP)
	}
	sess := &session.Session{
		Subject:   claims.Subject,
		Email:     claims.Email,
		Name:      claims.Name,
		Roles:     []string{string(role)},
		IdP:       p.IdP,
		AMR:       claims.AMR,
		ExpiresAt: time.Now().Add(a.sessionTTL),
	}
	if err := a.store.Create(ctx, sess); err != nil {
		return nil, "", err
	}
	return sess, p.Return, nil
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
// login to its second step, in the bounded, garbage-collected pending map.
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

// LoginLockoutRemaining reports how much longer key is locked out of local
// login, or zero when it is not, so the login page can say when to retry.
func (a *Authenticator) LoginLockoutRemaining(key string) time.Duration {
	return a.loginGate.remaining(key)
}

// LoginAttempt reserves one local-login attempt for key and reports whether
// it may proceed. The attempt is counted before the credential is checked, so
// parallel guesses cannot slip past the limit; a completed login clears the
// count through NoteLoginResult.
func (a *Authenticator) LoginAttempt(key string) bool {
	return a.loginGate.tryBegin(key)
}

// NoteLoginResult records the outcome of a local-login attempt reserved with
// LoginAttempt: a completed login clears the key's count, a failure stays
// counted (it was counted when the attempt was reserved).
func (a *Authenticator) NoteLoginResult(key string, ok bool) {
	if ok {
		a.loginGate.clear(key)
	}
}

// loginUserKey normalizes a submitted username into its throttle key: trimmed,
// lowercased and capped, so case or padding variants share one counter and a
// huge value cannot bloat the map. Every submitted name is counted, existing
// or not, so a locked response reveals nothing about which accounts exist.
func loginUserKey(username string) string {
	k := strings.ToLower(strings.TrimSpace(username))
	if len(k) > 64 {
		k = k[:64]
	}
	return k
}

// UserLoginAttempt is LoginAttempt for the per-username counter: it reserves
// one attempt against username, or reports that the name is locked out.
func (a *Authenticator) UserLoginAttempt(username string) bool {
	return a.userGate.tryBegin(loginUserKey(username))
}

// UserLoginThrottled reports whether username is currently locked out.
func (a *Authenticator) UserLoginThrottled(username string) bool {
	return a.userGate.atLimit(loginUserKey(username), true)
}

// UserLoginLockoutRemaining reports how much longer username stays locked out.
func (a *Authenticator) UserLoginLockoutRemaining(username string) time.Duration {
	return a.userGate.remaining(loginUserKey(username))
}

// NoteUserLoginResult clears username's count after a completed login.
func (a *Authenticator) NoteUserLoginResult(username string, ok bool) {
	if ok {
		a.userGate.clear(loginUserKey(username))
	}
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
// extended in the store and its cookie re-issued, so continued use keeps it alive
// up to its absolute lifetime (maxSessionAge).
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
	if time.Since(sess.CreatedAt) >= maxSessionAge {
		_ = a.store.Delete(r.Context(), sess.ID)
		return Principal{}, false
	}
	// The sign-in policy is checked only at login, so a session is refused once
	// the method it was created with is no longer allowed.
	if !a.sessionMethodAllowed(sess.IdP) {
		return Principal{}, false
	}
	a.maybeSlide(w, r.Context(), sess, secure)
	return principalOf(sess), true
}

// sessionMethodAllowed reports whether a session created through idp ("local"
// or an OIDC provider name) could still be created now: local login still
// allowed and configured, or the provider still listed in adminAuth.providers
// and still an OIDC provider.
func (a *Authenticator) sessionMethodAllowed(idp string) bool {
	if idp == "local" {
		return a.localAllowed() && a.localUser != "" && a.localHash != ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	p, ok := a.idps[idp]
	return ok && p.Type == model.IdPTypeOIDC && p.OIDC != nil && slices.Contains(a.settings.Providers, idp)
}

// RevokeAllSessions ends every admin session except keepID (the caller's own;
// "" ends all) and returns how many were ended.
func (a *Authenticator) RevokeAllSessions(ctx context.Context, keepID string) (int, error) {
	return a.store.DeleteAllExcept(ctx, keepID)
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
// half-TTL threshold avoids a store write and Set-Cookie on every request. The
// new expiry never passes the session's absolute lifetime (maxSessionAge).
func (a *Authenticator) maybeSlide(w http.ResponseWriter, ctx context.Context, sess *session.Session, secure bool) {
	remaining := time.Until(sess.ExpiresAt)
	if remaining <= 0 || remaining > a.sessionTTL/2 {
		return
	}
	newExp := time.Now().Add(a.sessionTTL)
	if limit := sess.CreatedAt.Add(maxSessionAge); newExp.After(limit) {
		newExp = limit
	}
	if !newExp.After(sess.ExpiresAt) {
		return
	}
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
