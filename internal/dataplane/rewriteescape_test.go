package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestRewriteDotSegmentNeverReachesUpstream is the end-to-end regression for the
// confirmed escape: a rewrite target carrying ".." composed with the upstream's
// base path into "/base/../admin/secret", which a backend that re-collapses dot
// segments serves as "/admin/secret". Model validation now refuses such a
// target; this asserts the data plane's own defence, so a config that reached
// the router without validation still cannot escape the base path.
func TestRewriteDotSegmentNeverReachesUpstream(t *testing.T) {
	var gotPath string
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer closeFn()
	up.Path = "/base"

	cfg := model.Config{
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "esc"},
			Type:       model.MWTypeRewrite,
			Rewrite:    &model.RewriteMiddleware{PrefixRules: []model.RewriteRule{{From: "/public", To: "/../admin"}}},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"app.example.com"},
			Upstream:    up,
			Middlewares: []string{"esc"},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app.example.com/public/secret", nil)
	req.Host = "app.example.com"
	rt.serveHTTP(rec, req)

	if gotPath == "/base/../admin/secret" || gotPath == "/admin/secret" {
		t.Fatalf("upstream received %q: the rewrite escaped the upstream base path", gotPath)
	}
	if gotPath != "" && gotPath != "/base/" && !hasPrefix(gotPath, "/base/") {
		t.Fatalf("upstream received %q, want a path inside the %q base path", gotPath, "/base")
	}
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

// TestConfineUpstreamPath covers the composition guard directly: a clean path
// under the base passes through untouched, and anything that climbs out of the
// base is replaced by the base itself.
func TestConfineUpstreamPath(t *testing.T) {
	tests := []struct {
		name, base, in, want string
	}{
		{"clean path under the base", "/base", "/base/app", "/base/app"},
		{"trailing slash preserved", "/base", "/base/app/", "/base/app/"},
		{"base itself", "/base", "/base", "/base"},
		{"dot segment collapsed inside the base", "/base", "/base/x/../app", "/base/app"},
		{"escape replaced by the base", "/base", "/base/../admin/secret", "/base/"},
		{"deep escape replaced by the base", "/base", "/base/../../etc/passwd", "/base/"},
		{"no base leaves a clean path alone", "", "/anything", "/anything"},
		{"no base still cleans", "", "/a/../b", "/b"},
		{"asterisk form untouched", "/base", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := confineUpstreamPath(tc.base, tc.in); got != tc.want {
				t.Fatalf("confineUpstreamPath(%q, %q) = %q, want %q", tc.base, tc.in, got, tc.want)
			}
		})
	}
}

// TestJoinUpstreamBasePathConfines is the group-failover twin of the check
// above: the same composition happens there, so it gets the same guard.
func TestJoinUpstreamBasePathConfines(t *testing.T) {
	if got := joinUpstreamBasePath("/base", "/../admin"); got != "/base/" {
		t.Fatalf("joinUpstreamBasePath(/base, /../admin) = %q, want the base path", got)
	}
	if got := joinUpstreamBasePath("/base", "/app"); got != "/base/app" {
		t.Fatalf("joinUpstreamBasePath(/base, /app) = %q, want /base/app", got)
	}
}

// TestRewriteHandlerRejectsSmuggledSeparator: a regex rewrite composes its
// target at request time from capture text, so the composed path is re-checked
// the way the inbound path is. A path a backend could re-collapse onto another
// route is refused, not forwarded.
func TestRewriteHandlerRejectsSmuggledSeparator(t *testing.T) {
	var reached bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	spec := model.RewriteMiddleware{RegexRules: []model.RewriteRule{{From: "^/x/(.*)$", To: "/base/$1"}}}
	h := rewriteHandler(spec, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://c/x/a", nil)
	req.URL.Path = `/x/a\..\admin` // a path the router would have refused; forced here
	h.ServeHTTP(rec, req)
	if reached || rec.Code != http.StatusBadRequest {
		t.Fatalf("rewritten path with a backslash: reached=%v status=%d, want a 400 and no forward", reached, rec.Code)
	}
}
