package dataplane

import (
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// authMiddlewareHandler gates a proxied host behind an identity provider.
//
//   - forward-auth: accept a trusted forward-auth identity (header-spoof safe),
//     map groups to a role, and enforce the middleware's RequiredRoles.
//   - oidc: per-host OIDC relying-party gating is a P1 feature; until then it
//     fails closed so the host is never left unintentionally open.
func authMiddlewareHandler(mw model.Middleware, reg *registry, hostName string, next http.Handler) http.Handler {
	idpName := mw.Auth.IdentityProvider
	idp, ok := reg.idps[idpName]
	if !ok {
		return failClosed(hostName, "auth references unknown identity provider "+idpName)
	}

	mode := mw.Auth.Mode
	if mode == "" {
		mode = idp.Type // default the mode from the IdP type
	}

	switch mode {
	case model.AuthModeForwardAuth:
		if idp.ForwardAuth == nil {
			return failClosed(hostName, "forward-auth mode requires a forward-auth identity provider")
		}
		fa := auth.CompileForwardAuth(*idp.ForwardAuth, idpName)
		return forwardAuthGate(fa, idp.RoleMapping, mw.Auth.RequiredRoles, next)
	case model.AuthModeAuthRequest:
		if idp.AuthRequest == nil {
			return failClosed(hostName, "auth-request mode requires an auth-request identity provider")
		}
		arp, err := compileAuthRequest(*idp.AuthRequest)
		if err != nil {
			return failClosed(hostName, "auth-request: "+err.Error())
		}
		return arp.handler(hostName, next)
	case model.AuthModeOIDC:
		return failClosedf(hostName, "per-host OIDC gating is not yet implemented (P1); denying")
	default:
		return failClosed(hostName, "unknown auth mode "+mode)
	}
}

// forwardAuthGate enforces a trusted forward-auth identity and role policy.
func forwardAuthGate(fa auth.ForwardAuth, rm *model.RoleMapping, required []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := fa.Identity(r)
		if !ok {
			// Either the peer is untrusted or no identity was asserted. Strip any
			// forged identity headers before refusing so nothing leaks upstream.
			fa.Strip(r)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		role := auth.MapRole(id.Groups, rm)
		if !roleAllowed(role, required) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// roleAllowed denies RoleNone always; with no RequiredRoles any mapped role
// passes; otherwise the role must be in the required set.
func roleAllowed(role auth.Role, required []string) bool {
	if role == auth.RoleNone {
		return false
	}
	if len(required) == 0 {
		return true
	}
	for _, r := range required {
		if auth.Role(r) == role {
			return true
		}
	}
	return false
}

func failClosed(host, msg string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		log.Warn().Str("host", host).Msg(msg)
		http.Error(w, "authentication not available", http.StatusServiceUnavailable)
	})
}

func failClosedf(host, msg string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		log.Warn().Str("host", host).Msg(msg)
		http.Error(w, "authentication not available", http.StatusNotImplemented)
	})
}
