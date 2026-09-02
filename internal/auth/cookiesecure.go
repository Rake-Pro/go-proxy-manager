package auth

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/clientip"
	"github.com/rs/zerolog/log"
)

// CookieSecureMode decides whether the admin cookies (session and OIDC login
// state) carry the Secure attribute.
//
// The default is CookieSecureAuto, which answers per request. A fixed "always"
// default made the very first login fail: the browser refuses to store a Secure
// cookie received over http://127.0.0.1:8081, so the login POST succeeded and
// the next request was anonymous again.
type CookieSecureMode int

const (
	// CookieSecureAuto sets Secure exactly when the request reached gpm over TLS,
	// arrived through a trusted proxy asserting X-Forwarded-Proto: https, or
	// settings.externalBaseURL is an https URL. Otherwise the cookie is issued
	// without Secure so plain-HTTP bootstrapping works.
	CookieSecureAuto CookieSecureMode = iota
	// CookieSecureAlways always sets Secure (GPM_COOKIE_SECURE=1).
	CookieSecureAlways
	// CookieSecureNever never sets Secure (GPM_COOKIE_SECURE=0).
	CookieSecureNever
)

// String renders the mode as the value an operator writes in GPM_COOKIE_SECURE.
func (m CookieSecureMode) String() string {
	switch m {
	case CookieSecureAlways:
		return "1"
	case CookieSecureNever:
		return "0"
	default:
		return "auto"
	}
}

// ParseCookieSecureMode reads a GPM_COOKIE_SECURE / -cookie-secure value.
// Accepted: "auto" (or empty), "1"/"true"/"yes"/"on", "0"/"false"/"no"/"off".
func ParseCookieSecureMode(s string) (CookieSecureMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return CookieSecureAuto, nil
	case "1", "true", "yes", "on":
		return CookieSecureAlways, nil
	case "0", "false", "no", "off":
		return CookieSecureNever, nil
	}
	return CookieSecureAuto, fmt.Errorf("invalid cookie-secure value %q (want auto, 1 or 0)", s)
}

// Cookie-secure states reported by CookieSecureState and by
// GET /api/capabilities as capabilities.adminLogin.cookieSecure.
const (
	// CookieSecureStateSecure: cookies for this request carry Secure.
	CookieSecureStateSecure = "secure"
	// CookieSecureStateInsecurePrivate: no Secure, but the client is loopback or
	// an RFC 1918 / ULA address - the ordinary first-run and LAN case.
	CookieSecureStateInsecurePrivate = "insecure-private"
	// CookieSecureStateInsecurePublic: no Secure and the client is a public
	// address, so the session cookie crosses untrusted networks in the clear.
	CookieSecureStateInsecurePublic = "insecure-public"
)

// insecureWarnEvery bounds how often the plain-HTTP-from-a-public-address
// warning is logged, so a scripted client cannot turn it into a log flood.
const insecureWarnEvery = time.Hour

// insecureWarner rate-limits the "cookie issued without Secure" warning.
type insecureWarner struct {
	mu   sync.Mutex
	last time.Time
}

func (n *insecureWarner) allow(now time.Time) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.last.IsZero() && now.Sub(n.last) < insecureWarnEvery {
		return false
	}
	n.last = now
	return true
}

// secureFor is the per-request Secure decision. It never reads an untrusted
// peer's X-Forwarded-Proto: a client that could set that header at will could
// otherwise talk gpm into (or out of) marking its own cookie Secure.
func (a *Authenticator) secureFor(r *http.Request) bool {
	switch a.secureMode {
	case CookieSecureAlways:
		return true
	case CookieSecureNever:
		return false
	}
	if r != nil {
		if r.TLS != nil {
			return true
		}
		if forwardedHTTPS(r) {
			return true
		}
	}
	a.mu.RLock()
	base := a.baseURL
	a.mu.RUnlock()
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(base)), "https://")
}

// cookieSecure resolves the Secure decision at cookie-issuance time and, in auto
// mode, nudges the operator when the answer is "no" for a client that is not on
// a private network.
func (a *Authenticator) cookieSecure(r *http.Request) bool {
	secure := a.secureFor(r)
	if !secure && a.secureMode == CookieSecureAuto && r != nil && !privateClient(r) {
		if a.insecureWarn.allow(time.Now()) {
			log.Warn().Msg("admin session cookie sent without Secure over plain HTTP from a non-private address; " +
				"set externalBaseURL to https or front the admin listener with TLS")
		}
	}
	return secure
}

// CookieSecureState classifies the cookie decision for r so the UI can show a
// banner: "secure", "insecure-private" or "insecure-public".
func (a *Authenticator) CookieSecureState(r *http.Request) string {
	if a.secureFor(r) {
		return CookieSecureStateSecure
	}
	if privateClient(r) {
		return CookieSecureStateInsecurePrivate
	}
	return CookieSecureStateInsecurePublic
}

// forwardedHTTPS reports whether a TRUSTED proxy said this request reached it
// over TLS. The leftmost X-Forwarded-Proto element is the scheme the browser
// actually used, which is the one that decides whether it will store a Secure
// cookie.
func forwardedHTTPS(r *http.Request) bool {
	peer := clientip.PeerIP(r)
	if peer == nil || !clientip.InNets(peer, clientip.Trusted()) {
		return false
	}
	v := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.EqualFold(strings.TrimSpace(v), "https")
}

// privateClient reports whether the derived client address is loopback, RFC 1918
// or ULA (fc00::/7), i.e. the bootstrap and LAN case in which a plain-HTTP admin
// session is a deliberate choice rather than an exposure. An address gpm cannot
// derive at all is treated as public: that is the cautious half.
func privateClient(r *http.Request) bool {
	if r == nil {
		return false
	}
	ip, _ := clientip.Derive(r, clientip.Trusted())
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
