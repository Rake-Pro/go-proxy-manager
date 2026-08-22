package dataplane

import (
	"net/http"
	"strings"
)

// acmeChallengePrefix is the well-known path ACME HTTP-01 validation fetches.
const acmeChallengePrefix = "/.well-known/acme-challenge/"

// ACMEChallengeStore resolves an HTTP-01 token to its key authorization. The
// ACME manager owns the live token map and the data plane only reads it, so the
// two subsystems stay decoupled (dataplane does not import internal/acme).
type ACMEChallengeStore interface {
	KeyAuth(token string) (string, bool)
}

// serveACMEChallenge answers an in-flight HTTP-01 validation request and reports
// whether it handled the request. It runs on the plaintext listener before any
// host lookup, force-SSL redirect, or auth so a challenge is answerable for a
// host that is not routable yet (or that redirects everything to https).
//
// A challenge path whose token is not in flight here is deliberately NOT claimed:
// it falls through to normal routing, so a proxied upstream that runs its own
// ACME client keeps working.
func serveACMEChallenge(store ACMEChallengeStore, w http.ResponseWriter, r *http.Request) bool {
	if store == nil || !strings.HasPrefix(r.URL.Path, acmeChallengePrefix) {
		return false
	}
	token := strings.TrimPrefix(r.URL.Path, acmeChallengePrefix)
	if token == "" || strings.Contains(token, "/") {
		return false
	}
	keyAuth, ok := store.KeyAuth(token)
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(keyAuth))
	return true
}
