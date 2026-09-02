// Package ui serves the embedded admin single-page app. The built assets are
// compiled into the binary (go:embed) so the daemon stays a single artifact
// with no separate web bundle to deploy.
package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
)

//go:embed static
var staticFS embed.FS

// The in-app help registry. It lives outside static/ so it is authored and
// reviewed as data rather than as part of the bundle, but it is served from the
// same route as app.js (and with the same ETag + no-cache revalidation), so the
// SPA fetches it with a plain relative "hints.json" and no API call is involved.
//
//go:embed hints/hints.json
var hintsFS embed.FS

// Handler returns an http.Handler serving the SPA: real embedded assets are
// served directly; any other path falls back to index.html so client-side
// (hash) routing and deep links work.
//
// Caching. embed.FS entries have a zero ModTime, so http.FileServer emits
// neither Last-Modified nor ETag and every full page load re-downloaded the
// whole bundle (app.js alone is ~500 KB, hints.json ~80 KB). The assets are
// baked into the binary, so their content IS the version: a strong ETag is
// computed once at startup from the bytes themselves and paired with
// Cache-Control: no-cache, which means "revalidate every time" rather than "do
// not store". The browser then spends one conditional request per asset per
// load and gets 304s, and an upgraded binary invalidates every entry with no
// cache-busting query string to maintain. The shell keeps no-store: it is 700
// bytes and it is what a stale deploy would pin.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}
	etags, err := buildETags(sub)
	if err != nil {
		return nil, err
	}
	hintsBytes, err := hintsFS.ReadFile("hints/hints.json")
	if err != nil {
		return nil, err
	}
	etags["hints.json"] = etagOf(hintsBytes)

	files := http.FileServer(http.FS(sub))
	hints := http.FileServer(http.FS(hintsFS))

	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(index)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		// "index.html" is handled here rather than left to http.FileServer,
		// which answers it with a 301 to "./" - a redirect that would carry the
		// asset cache headers instead of the shell's no-store.
		if p == "" || p == "index.html" {
			serveIndex(w)
			return
		}
		if p == "hints.json" {
			setAssetCacheHeaders(w, etags[p])
			r2 := r.Clone(r.Context())
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path = "/hints/hints.json"
			hints.ServeHTTP(w, r2)
			return
		}
		if _, err := fs.Stat(sub, p); err != nil {
			serveIndex(w) // unknown path -> SPA shell
			return
		}
		setAssetCacheHeaders(w, etags[p])
		files.ServeHTTP(w, r)
	}), nil
}

// setAssetCacheHeaders arms http.ServeContent's built-in conditional handling:
// it compares If-None-Match against the ETag already on the ResponseWriter and
// answers 304 itself, so nothing here has to parse the request.
func setAssetCacheHeaders(w http.ResponseWriter, etag string) {
	if etag == "" {
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
}

// etagOf is a strong validator over the exact bytes served. Strong, not weak:
// the payload is byte-identical for the life of the binary, so a range request
// against it is valid too.
func etagOf(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// buildETags hashes every embedded asset once, at startup. The tree is a few
// dozen files, so the cost is a one-off millisecond and the map is then
// read-only for the process lifetime (no locking on the request path).
func buildETags(sub fs.FS) (map[string]string, error) {
	out := map[string]string{}
	err := fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		out[p] = etagOf(b)
		return nil
	})
	return out, err
}
