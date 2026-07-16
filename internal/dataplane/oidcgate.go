package dataplane

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	oidcCallbackPath = "/__gpm/oidc/callback"
	// The __Host- prefix makes the browser enforce what these cookies already set
	// themselves - Secure, host-locked (no Domain), Path=/ - so a sibling subdomain
	// cannot plant a same-named Domain-scoped shadow cookie. The data plane is
	// always HTTPS (setSignedCookie sets Secure unconditionally), so the prefix's
	// Secure requirement always holds.
	oidcSessionCookie = "__Host-gpm_sso"
	oidcStateCookie   = "__Host-gpm_sso_state"
	// oidcSessionTTL is an ABSOLUTE cap, not a sliding window: the cookie is not
	// re-issued on activity, so it expires 1h after login regardless of use. On
	// expiry the gate falls through to beginLogin, which round-trips the IdP again
	// (silent when the IdP session is still alive) and re-checks group membership
	// on the callback. That bounds the offboarding window - a deprivileged or
	// disabled user loses data-plane access within 1h - without server-side
	// session state. See the SSO session note in docs/configuration.md (GPM-L3).
	oidcSessionTTL = 1 * time.Hour
	oidcStateTTL   = 10 * time.Minute
)

// ssoKeyFile is the name of the persisted SSO signing key under the state dir.
const ssoKeyFile = "sso_signing.key"

// ssoKeyDir, set once at startup via SetSSOKeyDir, is the directory used to
// persist an auto-generated SSO signing key when GPM_SSO_SIGNING_KEY is unset.
// Empty means "do not persist" (ephemeral per-process key).
var ssoKeyDir atomic.Pointer[string]

// SetSSOKeyDir configures where a generated data-plane SSO signing key is
// persisted (so sessions survive restarts) when GPM_SSO_SIGNING_KEY is not set.
// Call once at startup before serving; the env var always takes precedence.
func SetSSOKeyDir(dir string) { ssoKeyDir.Store(&dir) }

// ssoSigningKey is the process-wide HMAC key for data-plane SSO cookies. It is
// taken from GPM_SSO_SIGNING_KEY when set; otherwise, if a state dir was
// configured (SetSSOKeyDir), it is loaded from (or generated into) a persisted
// key file so sessions survive restarts; failing both it falls back to a random
// per-process key. Resolved once, lazily, so a deployment without any
// OIDC-gated host never touches the key file or logs the ephemeral-key notice.
var ssoSigningKey = sync.OnceValue(func() []byte {
	if v := strings.TrimSpace(os.Getenv("GPM_SSO_SIGNING_KEY")); v != "" {
		return []byte(v)
	}
	if d := ssoKeyDir.Load(); d != nil && *d != "" {
		if k, err := loadOrCreateSSOKey(*d); err == nil {
			return k
		} else {
			log.Warn().Err(err).Msg("data-plane OIDC: cannot persist SSO signing key; using an ephemeral key for this process")
		}
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		panic("dataplane: cannot generate SSO signing key: " + err.Error())
	}
	log.Warn().Msg("data-plane OIDC: using an ephemeral SSO signing key; set GPM_SSO_SIGNING_KEY to persist sessions across restarts")
	return k
})

// loadOrCreateSSOKey reads the persisted 32-byte SSO signing key (hex-encoded)
// from dir, generating and atomically writing it on first use. The file is 0600
// inside a 0700 dir, mirroring internal/acme/store.go's key handling.
func loadOrCreateSSOKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, ssoKeyFile)
	if b, err := os.ReadFile(path); err == nil {
		k, derr := hex.DecodeString(strings.TrimSpace(string(b)))
		if derr == nil && len(k) == 32 {
			return k, nil
		}
		// The file exists but is corrupt (bad hex or wrong length). Self-heal:
		// quarantine it and fall through to generate a fresh key. Refusing here
		// would fall back to an ephemeral per-process key that silently
		// invalidates every SSO session on each restart, since the bad file would
		// never be replaced. Clients re-authenticate once. A rename failure is a
		// genuine I/O error: refuse without deleting anything.
		if rerr := os.Rename(path, path+".corrupt"); rerr != nil {
			return nil, fmt.Errorf("malformed SSO signing key at %s could not be quarantined: %w", path, rerr)
		}
		log.Error().Str("path", path).Msg("data-plane OIDC: malformed SSO signing key quarantined to .corrupt; generating a new key, clients will re-authenticate once")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(path, []byte(hex.EncodeToString(k)), 0o600); err != nil {
		return nil, err
	}
	log.Info().Str("path", path).Msg("data-plane OIDC: generated persistent SSO signing key")
	return k, nil
}

// writeFileAtomic writes data to a temp file in the target dir and renames it
// into place, so a concurrent reader never sees a partial file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ssoNotBeforeFile persists the revocation watermark next to the signing key.
const ssoNotBeforeFile = "sso_not_before"

// ssoNotBefore is the SSO revocation watermark: sessions issued strictly
// before it are invalid. Loaded lazily from the state dir; 0 = never revoked.
var (
	ssoNotBefore     atomic.Int64
	ssoNotBeforeOnce sync.Once
)

// ssoRevokedAt returns the current revocation watermark (unix seconds).
func ssoRevokedAt() int64 {
	ssoNotBeforeOnce.Do(func() {
		d := ssoKeyDir.Load()
		if d == nil || *d == "" {
			return
		}
		b, err := os.ReadFile(filepath.Join(*d, ssoNotBeforeFile))
		if err != nil {
			return // absent (or unreadable) = no revocation on record
		}
		if v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
			ssoNotBefore.Store(v)
		}
	})
	return ssoNotBefore.Load()
}

// RevokeAllSSOSessions invalidates every outstanding data-plane SSO session by
// moving the revocation watermark to now: a session issued strictly before it
// fails the gate and the user re-authenticates at the IdP (sessions minted in
// the same second survive, so a login racing the revocation cannot loop).
// The watermark is persisted next to the signing key so revocation survives
// restarts; without a state dir it is process-local, matching the ephemeral
// signing key's semantics.
func RevokeAllSSOSessions() error {
	now := time.Now().Unix()
	// Force the lazy load so the store below cannot be overwritten, and never
	// move the watermark backwards (clock skew across restarts must not weaken
	// a prior revocation).
	if cur := ssoRevokedAt(); cur > now {
		now = cur
	}
	ssoNotBefore.Store(now)
	log.Info().Int64("notBefore", now).Msg("data-plane SSO: all sessions revoked")
	d := ssoKeyDir.Load()
	if d == nil || *d == "" {
		return nil
	}
	if err := os.MkdirAll(*d, 0o700); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(*d, ssoNotBeforeFile), []byte(strconv.FormatInt(now, 10)), 0o600)
}

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

// oidcSession is the signed end-user session cookie payload. Host binds the
// cookie to the gate that issued it: the HMAC key is process-wide across every
// OIDC-gated host, so without this a valid cookie from host A (or an IdP that
// host A trusts) could be replayed to host B and re-evaluated against B's role
// mapping using A's groups. The gate rejects a cookie whose Host is not its own.
type oidcSession struct {
	Sub    string   `json:"sub"`
	Email  string   `json:"email,omitempty"`
	Name   string   `json:"name,omitempty"`
	Groups []string `json:"groups,omitempty"`
	Host   string   `json:"host"`
	Exp    int64    `json:"exp"`
	// Iat is the issue time, checked against the revocation watermark (see
	// RevokeAllSSOSessions). Pre-watermark sessions - including legacy cookies
	// without the field - are rejected once a revocation is on record.
	Iat int64 `json:"iat,omitempty"`
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
		if readSignedCookie(r, oidcSessionCookie, &sess) && sess.Exp > time.Now().Unix() && sess.Host == d.hostName &&
			sess.Iat >= ssoRevokedAt() {
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
		Groups: claims.Groups, Host: d.hostName,
		Exp: time.Now().Add(oidcSessionTTL).Unix(),
		Iat: time.Now().Unix(),
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
