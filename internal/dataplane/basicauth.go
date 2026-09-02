package dataplane

import (
	"net"
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

// basicAuthUsers is a compiled credential set: username -> bcrypt hash. It is
// the ONE place credentials are verified, shared by the auth middleware's
// `mode: basic` gate and by the deprecated AccessList.BasicAuth path, so the two
// cannot drift in their timing behaviour or their bcrypt bounding.
type basicAuthUsers map[string]string

func compileBasicAuthUsers(users []model.BasicAuthUser) basicAuthUsers {
	out := make(basicAuthUsers, len(users))
	for _, u := range users {
		out[u.Username] = u.PasswordHash
	}
	return out
}

// verify checks the request's basic-auth credentials against the set. The bcrypt
// compare runs under bcryptSem so concurrent verifications stay bounded; r's
// context cancels the wait, so a client that goes away does not keep a slot
// queued. An unknown username still costs one compare (against dummyBcryptHash),
// so a missing user is not distinguishable from a wrong password by timing.
func (u basicAuthUsers) verify(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	select {
	case bcryptSem <- struct{}{}:
		defer func() { <-bcryptSem }()
	case <-r.Context().Done():
		return false
	}
	hash, known := u[user]
	if !known {
		_ = bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte(pass))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass)) == nil
}

// basicAuthGate is the auth-tier handler for `mode: basic`: HTTP basic auth
// against a local credential set, with no identity provider involved. It sits at
// the auth position of the chain like every other auth mode, so an access list,
// a bouncer and a rate limit all still run outside it.
//
// It behaves exactly like the deprecated access-list basic auth it replaces:
// the same per-client-IP failure throttle (a locked-out client is answered with
// the same 401 + challenge, never a 429, so the response is no oracle), the same
// bounded bcrypt work, and the same WWW-Authenticate challenge. What it adds is
// the treatment every other auth mode already has - the host's custom error
// pages render the refusal, and the denial is counted - plus allowFrom, the
// any-of network exemption auth-request and client-cert carry.
//
// realm is the challenge realm: the middleware's configured realm, or the owner
// name (middleware or host) when none is set.
func basicAuthGate(spec model.AuthMiddleware, realm string, clientIP func(*http.Request) net.IP, allowNets []*net.IPNet, host string, ep *compiledErrorPages, next http.Handler) http.Handler {
	if spec.Basic == nil {
		return failClosed(host, "basic mode requires an auth.basic block", ep)
	}
	users := compileBasicAuthUsers(spec.Basic.Users)
	if len(users) == 0 {
		// Validation refuses this, so reaching it means the compiled config was
		// built past the validator. A credential set with nobody in it must deny,
		// never admit.
		return failClosed(host, "basic mode has no auth.basic.users", ep)
	}
	challenge := `Basic realm="` + spec.Basic.RealmOrDefault(realm) + `"`
	// One gate per compiled middleware, exactly as the access-list tier does: a
	// config reload rebuilds the chain and resets the counters, which is operator
	// driven and cannot be provoked by the client being throttled.
	gate := newAuthGate(basicAuthLockout, maxBasicAuthFails, maxBasicAuthKeys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The network exemption is decided first, so an exempt client is never
		// challenged and never costs a bcrypt compare.
		if len(allowNets) > 0 && clientIP != nil {
			if ip := clientIP(r); ip != nil && ipInNets(ip, allowNets) {
				next.ServeHTTP(w, r)
				return
			}
		}
		key := authGateKey(clientIP, r)
		ok := false
		if gate.atLimit(key) {
			log.Warn().Str("host", host).Str("client", key).
				Msg("basic auth middleware: client is locked out after repeated failures")
		} else {
			ok = users.verify(r)
			if _, _, presented := r.BasicAuth(); presented {
				// Only a real attempt counts. Browsers routinely send one
				// credential-less request per fresh page load; counting those
				// would lock out normal users.
				if ok {
					gate.clear(key)
				} else {
					gate.record(key)
				}
			}
		}
		if ok {
			next.ServeHTTP(w, r)
			return
		}
		// The challenge header is set BEFORE the body is rendered, so it goes out
		// with the 401 whether the host has a custom error page or not - a browser
		// must still get the credential prompt.
		w.Header().Set("WWW-Authenticate", challenge)
		countDenial(r, "auth-basic")
		refuse(w, http.StatusUnauthorized, ep, host, "authentication required")
	})
}
