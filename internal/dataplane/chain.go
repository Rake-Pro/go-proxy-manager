package dataplane

import (
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// registry indexes reusable config objects by name for chain assembly.
type registry struct {
	accessLists map[string]model.AccessList
	middlewares map[string]model.Middleware
	idps        map[string]model.IdentityProvider
}

func buildRegistry(cfg model.Config) *registry {
	reg := &registry{
		accessLists: map[string]model.AccessList{},
		middlewares: map[string]model.Middleware{},
		idps:        map[string]model.IdentityProvider{},
	}
	for _, a := range cfg.AccessLists {
		reg.accessLists[a.Name] = a
	}
	for _, m := range cfg.Middlewares {
		reg.middlewares[m.Name] = m
	}
	for _, p := range cfg.IdentityProviders {
		reg.idps[p.Name] = p
	}
	return reg
}

// buildChain wraps the terminal proxy handler in the host's middleware chain.
// Steps run in a fixed canonical order regardless of reference order:
//
//	auth -> access-list -> headers -> (rate-limit, WAF ... later) -> proxy
//
// so new behaviours slot into defined positions instead of colliding as text.
func buildChain(proxy http.Handler, host model.ProxyHost, reg *registry) http.Handler {
	h := proxy

	// Innermost: header mutations (closest to the upstream).
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeHeaders || mw.Headers == nil {
			continue
		}
		h = headersHandler(*mw.Headers, h)
	}

	// Access lists (host-level), applied outside the headers stage.
	for _, name := range host.AccessLists {
		al, ok := reg.accessLists[name]
		if !ok {
			continue
		}
		h = accessListHandler(compileAccessList(al), clientIP, h)
	}

	// Outermost: authentication. Until P0d this fails closed so a host that
	// declares auth is never silently left open.
	for _, name := range host.Middlewares {
		mw, ok := reg.middlewares[name]
		if !ok || mw.Type != model.MWTypeAuth {
			continue
		}
		h = unimplementedAuthHandler(host.Name, name, h)
	}

	return h
}

// unimplementedAuthHandler denies all requests for an auth middleware that is
// not yet implemented, rather than passing them through.
func unimplementedAuthHandler(host, mw string, _ http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Warn().Str("host", host).Str("middleware", mw).Msg("auth middleware not yet implemented (P0d) - denying")
		http.Error(w, "Authentication not configured", http.StatusNotImplemented)
	})
}
