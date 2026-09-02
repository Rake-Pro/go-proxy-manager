package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// TokenPrefix marks a gpm API token secret. It is what makes the Authorization
// header unambiguous: only a bearer value carrying this prefix is treated as an
// API token, so any other bearer scheme falls through to the cookie path.
const TokenPrefix = "gpm_"

// tokenSecretBytes is the entropy behind a token secret (256 bits).
const tokenSecretBytes = 32

// NewTokenSecret mints a fresh token secret and returns it with its sha256 hex
// digest. Only the digest is ever persisted; the secret is shown to the operator
// exactly once, in the response to the request that created or rotated it.
func NewTokenSecret() (secret, hash string, err error) {
	b := make([]byte, tokenSecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate api token: %w", err)
	}
	secret = TokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return secret, HashTokenSecret(secret), nil
}

// HashTokenSecret returns the lowercase sha256 hex digest of a token secret.
func HashTokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// maxTrackedTokenUses caps the in-memory last-used map so a flood of failed or
// deleted tokens cannot grow it without bound. Entries are only ever added for
// tokens that actually authenticated, so the cap is generous.
const maxTrackedTokenUses = 4096

// SetTokenSource injects the closure that returns the currently-configured API
// tokens. Its result is cached until InvalidateTokenCache is called, which the
// daemon does from the single config-reload path, so a token added, disabled or
// deleted through the API still takes effect immediately. A nil source disables
// bearer authentication entirely.
func (a *Authenticator) SetTokenSource(src func() []model.APIToken) {
	a.mu.Lock()
	a.tokens = src
	a.mu.Unlock()
	a.InvalidateTokenCache()
}

// InvalidateTokenCache drops the cached token set so the next bearer
// authentication re-reads the source. Wired into the daemon's reload path -
// every config change, from any origin, goes through it.
func (a *Authenticator) InvalidateTokenCache() {
	a.tcmu.Lock()
	a.tokenCache, a.tokenCached = nil, false
	a.tcmu.Unlock()
}

// currentTokens returns the configured tokens, calling the injected source at
// most once per config generation.
//
// The cache is a DoS control, not an optimisation. The source reads the whole
// git-backed config (directory walk, YAML parse, whole-graph validation), and a
// failed bearer attempt never reaches the login rate gate - so without it any
// unauthenticated caller could force an unbounded number of full config loads
// from the open internet. Concurrent misses serialise on tcmu, which collapses
// a burst into a single load.
func (a *Authenticator) currentTokens() ([]model.APIToken, bool) {
	a.mu.RLock()
	src := a.tokens
	a.mu.RUnlock()
	if src == nil {
		return nil, false
	}
	a.tcmu.Lock()
	defer a.tcmu.Unlock()
	if !a.tokenCached {
		a.tokenCache, a.tokenCached = src(), true
	}
	return a.tokenCache, true
}

// TokenLastUsed returns a snapshot of the in-memory last-use timestamps, keyed
// by token name. It is deliberately NOT persisted: the config store is git
// backed, and committing a timestamp on every authenticated request would turn
// normal API use into a commit flood. The map is therefore empty after a
// restart, which the UI presents as "never (since restart)".
func (a *Authenticator) TokenLastUsed() map[string]time.Time {
	a.tmu.Lock()
	defer a.tmu.Unlock()
	out := make(map[string]time.Time, len(a.tokenUse))
	for k, v := range a.tokenUse {
		out[k] = v
	}
	return out
}

func (a *Authenticator) noteTokenUse(name string) {
	a.tmu.Lock()
	defer a.tmu.Unlock()
	if a.tokenUse == nil {
		a.tokenUse = map[string]time.Time{}
	}
	if _, known := a.tokenUse[name]; !known && len(a.tokenUse) >= maxTrackedTokenUses {
		return
	}
	a.tokenUse[name] = time.Now().UTC()
}

// bearerTokenSecret extracts a gpm API token secret from the Authorization
// header. ok is false for a missing header, a non-Bearer scheme, or a bearer
// value that is not a gpm token (which stays eligible for the cookie path).
func bearerTokenSecret(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if len(h) < len("Bearer ") || !strings.EqualFold(h[:7], "bearer ") {
		return "", false
	}
	v := strings.TrimSpace(h[7:])
	if !strings.HasPrefix(v, TokenPrefix) {
		return "", false
	}
	return v, true
}

// authenticateToken resolves a presented bearer secret to a principal. Every
// enabled, unexpired token's stored digest is compared in constant time against
// the digest of the presented secret, so neither timing nor the number of
// configured tokens leaks which one matched.
//
// A token principal is always admin-role: the coarse role gate stays satisfied
// and the actual authorization is the scope check applied per route. It carries
// no CSRF token because it carries no ambient authority - nothing a browser
// attaches automatically - so RequireRole skips the CSRF check for it.
func (a *Authenticator) authenticateToken(secret string) (Principal, bool) {
	tokens, ok := a.currentTokens()
	if !ok {
		log.Warn().Msg("api token auth: presented bearer token but no token source is wired")
		return Principal{}, false
	}
	want := HashTokenSecret(secret)
	now := time.Now()
	for _, t := range tokens {
		if t.Disabled || t.TokenHash == "" || t.Expired(now) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(t.TokenHash), []byte(want)) != 1 {
			continue
		}
		a.noteTokenUse(t.Name)
		log.Info().Str("token", t.Name).Strs("scopes", t.Scopes).Msg("api token authenticated")
		return Principal{
			Subject: "token:" + t.Name,
			Name:    t.Name,
			Role:    RoleAdmin,
			IdP:     "api-token",
			IsToken: true,
			Scopes:  append([]string(nil), t.Scopes...),
		}, true
	}
	// Never log the presented secret or its digest: a rejected value may still be
	// a real credential for another system, and the digest is offline-crackable.
	log.Warn().Msg("api token auth: no enabled, unexpired token matches the presented bearer credential")
	return Principal{}, false
}

// ReadOnlyRoleScopes is what the non-admin `user` role grants: every read, no
// write. It is expressed as a scope grant so the role gate and the API-token
// scope gate are the SAME mechanism and compose by intersection - a token can
// never grant more than its scopes, and a role can never grant more than its
// scopes either.
//
// Read it together with ReadOnlyRoleExcludedSubjects: the effective grant is
// every `*:read` EXCEPT the subjects listed there. The exclusion cannot be
// expressed in the scope grammar itself (a wildcard subject matches every
// concrete one, and enumerating concrete subjects instead would lose the
// whole-config reads that literally require `*:read` - GET /config, /history,
// /logs, /health, /upstream-health), so it is a separate, explicit list checked
// first.
var ReadOnlyRoleScopes = []string{"*:read"}

// ReadOnlyRoleExcludedSubjects are the scope subjects the read-only role does
// NOT reach despite the wildcard read grant above.
//
// `api-tokens` is excluded because an API token is a CREDENTIAL, not
// configuration: listing them hands a viewer the name, scopes, expiry and
// stored digest of every automation credential on the instance - the map an
// attacker would want, and none of it something a viewer needs to see to view
// the proxy configuration. Token management is admin-only in both directions.
var ReadOnlyRoleExcludedSubjects = []string{"api-tokens"}

// readOnlyRoleExcludes reports whether required names a subject the read-only
// role is barred from, in either direction (read or write).
func readOnlyRoleExcludes(required string) bool {
	for _, subject := range ReadOnlyRoleExcludedSubjects {
		if strings.HasPrefix(required, subject+":") {
			return true
		}
	}
	return false
}

// RoleScopes returns the scope grant a role carries, or nil for "no role-level
// limit". Admin is unconstrained. RoleNone is nil here too, not an empty grant:
// an unauthenticated caller is refused by the role gate (RequireRole) before
// any scope is ever consulted, and re-deciding authentication here would only
// give the same answer a second, less obvious way.
//
// For the read-only role the grant is only half the answer: see
// ReadOnlyRoleExcludedSubjects, which RequireScope applies on top.
func RoleScopes(r Role) []string {
	if r == RoleUser {
		return ReadOnlyRoleScopes
	}
	return nil
}

// RequireScope reports whether p may perform the action described by required.
//
// Two independent gates compose, both of which must pass:
//
//   - the ROLE gate: an admin principal is unconstrained; a `user` principal is
//     read-only (ReadOnlyRoleScopes) and additionally barred from the subjects
//     in ReadOnlyRoleExcludedSubjects, so every write route and every
//     credential-management route refuses it.
//   - the TOKEN gate: an API token is limited to its own granted scopes.
//
// Because they compose, a token never grants more than its scopes and a
// read-only role never gains a write through a token that happens to hold one.
func RequireScope(p Principal, required string) error {
	if p.Role == RoleUser && readOnlyRoleExcludes(required) {
		return fmt.Errorf("the %q role cannot access %q", string(p.Role), required)
	}
	if rs := RoleScopes(p.Role); rs != nil && !model.ScopeSatisfied(rs, required) {
		return fmt.Errorf("the %q role is read-only and cannot perform %q", string(p.Role), required)
	}
	if !p.IsToken {
		return nil
	}
	if model.ScopeSatisfied(p.Scopes, required) {
		return nil
	}
	return fmt.Errorf("api token %q lacks the %q scope", strings.TrimPrefix(p.Subject, "token:"), required)
}
