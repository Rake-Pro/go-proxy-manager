package dataplane

import (
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// rewriteHandler rewrites the request path for exact matches before proxying.
// It matches on the decoded r.URL.Path (exact equality only - no prefix or
// pattern matching) and, on a hit, swaps in the target path. RawPath is cleared
// so Go re-derives the escaped form from the new Path; our configured paths are
// ASCII, so the plain and escaped forms coincide. Exact matching keeps the
// rewrite unambiguous and cheap - a single map lookup, no regex - and cannot be
// tricked into moving a request onto an unintended upstream path.
func rewriteHandler(spec model.RewriteMiddleware, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if to, ok := spec.ReplacePath[r.URL.Path]; ok {
			r.URL.Path = to
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}
