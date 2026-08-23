package dataplane

import (
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// redirectHandler serves a RedirectHost: every request to its domains is sent to
// the target with the configured scheme/status, optionally preserving the path.
type redirectHandler struct {
	forceSSL     bool
	targetScheme string // "", "auto", "http", or "https"
	targetDomain string
	status       int
	preservePath bool
}

func newRedirectHandler(h model.RedirectHost) *redirectHandler {
	status := h.StatusCode
	if status == 0 {
		status = http.StatusMovedPermanently // 301, the conventional default for a permanent redirect
	}
	return &redirectHandler{
		forceSSL:     h.TLS.ForceSSL,
		targetScheme: h.TargetScheme,
		targetDomain: h.TargetDomain,
		status:       status,
		preservePath: h.PreservePath,
	}
}

func (h *redirectHandler) serve(w http.ResponseWriter, r *http.Request) {
	scheme := h.targetScheme
	if scheme == "" || scheme == "auto" {
		scheme = requestScheme(r)
	}
	target := scheme + "://" + h.targetDomain
	if h.preservePath {
		target += r.URL.RequestURI() // path + raw query
	}
	http.Redirect(w, r, target, h.status)
}

// deadHandler serves a DeadHost: a fixed status (default 404) for claimed domains,
// so an unmatched vhost is absorbed instead of leaking to a default host.
type deadHandler struct {
	forceSSL bool
	status   int
	name     string
}

func newDeadHandler(h model.DeadHost) *deadHandler {
	status := h.StatusCode
	if status == 0 {
		status = http.StatusNotFound
	}
	return &deadHandler{forceSSL: h.TLS.ForceSSL, status: status, name: h.Name}
}

// serve renders the settings-level error page for h.status when one is
// configured (a DeadHost has no errorPages override of its own), falling back
// to the historical plain-text status body otherwise.
func (h *deadHandler) serve(w http.ResponseWriter, r *http.Request) {
	serveErrorPage(w, h.status, nil, h.name, func() {
		http.Error(w, http.StatusText(h.status), h.status)
	})
}
