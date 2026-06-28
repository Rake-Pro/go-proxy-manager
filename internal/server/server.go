// Package server hosts the admin HTTP surface. P0a exposes health and an honest
// version/build-identity endpoint; the REST API and UI land in P0e.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	"github.com/Rake-Pro/go-proxy-manager/internal/version"
	"github.com/rs/zerolog/log"
)

// Server is the admin HTTP server.
type Server struct {
	addr  string
	store *store.Store
	authn *auth.Authenticator
	http  *http.Server
}

// New constructs the admin server bound to addr. apiHandler, if non-nil, is the
// REST CRUD API (mounted under /api/ behind an admin-role gate); uiHandler, if
// non-nil, is the embedded SPA (mounted at /).
func New(addr string, st *store.Store, authn *auth.Authenticator, apiHandler, uiHandler http.Handler) *Server {
	s := &Server{addr: addr, store: st, authn: authn}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /version", s.handleVersion)

	// Authentication endpoints. The credential-bearing POSTs are wrapped in the
	// same-origin guard so a sibling subdomain cannot force login/logout (the
	// gap SameSite=Lax leaves); /auth/local has no session yet, so the Origin
	// check is its only CSRF defense.
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.Handle("POST /auth/local", sameOriginGuard(http.HandlerFunc(s.handleLocalLogin)))
	mux.Handle("POST /auth/logout", sameOriginGuard(http.HandlerFunc(s.handleLogout)))

	// Current principal (user-level): proves the session/role gate.
	mux.Handle("GET /api/me", s.authn.RequireRole(auth.RoleUser, http.HandlerFunc(s.handleMe)))

	// REST CRUD API (admin-only). The more specific /api/me above takes
	// precedence over this subtree for that exact path. RequireRole enforces the
	// CSRF token on mutating methods; sameOriginGuard is the outer belt.
	if apiHandler != nil {
		mux.Handle("/api/", sameOriginGuard(s.authn.RequireRole(auth.RoleAdmin, http.StripPrefix("/api", apiHandler))))
	}

	// The embedded SPA at the catch-all root. More specific routes above win;
	// the shell is public and the app redirects to /auth/login on a 401.
	if uiHandler != nil {
		mux.Handle("/", uiHandler)
	}
	s.http = &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// securityHeaders sets baseline hardening headers on every admin/login response:
// nosniff, clickjacking protection (frame-ancestors + the legacy X-Frame-Options),
// and a conservative referrer policy. HSTS is deliberately NOT set here: the admin
// panel is either reached over plain HTTP on its direct port (where browsers ignore
// HSTS) or fronted by the data plane over TLS, which is the actual TLS edge and
// owns the Strict-Transport-Security header for the host. Emitting it here too
// produced a duplicate HSTS header on the proxied admin path. The CSP is
// intentionally limited to frame-ancestors so it cannot break the SPA.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// Start runs the server until ctx is cancelled, then shuts it down gracefully.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", s.addr).Msg("admin server listening")
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		version.Info
		ConfigCommit string `json:"configCommit,omitempty"`
	}{Info: version.Get()}
	if s.store != nil {
		if head, err := s.store.Head(r.Context()); err == nil {
			resp.ConfigCommit = head
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
