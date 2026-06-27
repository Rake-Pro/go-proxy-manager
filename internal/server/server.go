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

// New constructs the admin server bound to addr.
func New(addr string, st *store.Store, authn *auth.Authenticator) *Server {
	s := &Server{addr: addr, store: st, authn: authn}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /version", s.handleVersion)

	// Authentication endpoints.
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.HandleFunc("POST /auth/local", s.handleLocalLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)

	// Protected: proves the session/role gate. The real admin API lands in P0e.
	mux.Handle("GET /api/me", s.authn.RequireRole(auth.RoleUser, http.HandlerFunc(s.handleMe)))
	s.http = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
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
