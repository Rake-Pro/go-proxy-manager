// Package ui serves the embedded admin single-page app. The built assets are
// compiled into the binary (go:embed) so the daemon stays a single artifact
// with no separate web bundle to deploy.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var staticFS embed.FS

// Handler returns an http.Handler serving the SPA: real embedded assets are
// served directly; any other path falls back to index.html so client-side
// (hash) routing and deep links work.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}
	files := http.FileServer(http.FS(sub))

	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(index)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveIndex(w)
			return
		}
		if _, err := fs.Stat(sub, p); err != nil {
			serveIndex(w) // unknown path -> SPA shell
			return
		}
		files.ServeHTTP(w, r)
	}), nil
}
