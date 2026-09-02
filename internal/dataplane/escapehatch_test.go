package dataplane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// upstreamEcho is a backend that records the path, raw query and Host header of
// the last request it served.
type upstreamEcho struct {
	srv   *httptest.Server
	path  string
	query string
	host  string
	body  string
}

func newUpstreamEcho(t *testing.T) *upstreamEcho {
	t.Helper()
	e := &upstreamEcho{}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		e.path, e.query, e.host, e.body = r.URL.Path, r.URL.RawQuery, r.Host, string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(e.srv.Close)
	return e
}

// upstream addresses the echo server with the given base path and hostHeader.
func (e *upstreamEcho) upstream(t *testing.T, basePath, hostHeader string) model.Upstream {
	t.Helper()
	u, err := url.Parse(e.srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return model.Upstream{Scheme: "http", Host: u.Hostname(), Port: port, Path: basePath, HostHeader: hostHeader}
}

// addr is the echo server's own host:port, the value an upstream-mode Host
// header must carry.
func (e *upstreamEcho) addr() string { return strings.TrimPrefix(e.srv.URL, "http://") }

// escapeHatchRouter validates cfg and compiles it into a router, staging any
// upstream groups without starting their probe goroutines.
func escapeHatchRouter(t *testing.T, cfg model.Config) *router {
	t.Helper()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("fixture config invalid: %v", err)
	}
	st := newHealthManager().stage(cfg.UpstreamGroups)
	rt, err := buildRouter(cfg, "", st)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	return rt
}

func serveEscapeHatch(t *testing.T, rt *router, method, target string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "http://app.example.com"+target, body)
	w := httptest.NewRecorder()
	rt.serveHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	return w
}

// TestPathComposition walks the whole composition order in one place: the
// location prefix strip, then a rewrite middleware, then the upstream base path.
func TestPathComposition(t *testing.T) {
	tests := []struct {
		name        string
		locPath     string
		stripPrefix bool
		rewrite     *model.RewriteMiddleware
		basePath    string
		request     string
		wantPath    string
		wantQuery   string
	}{
		{
			name:      "nothing configured forwards the path unchanged",
			locPath:   "/app",
			request:   "/app/foo?a=1&b=2",
			wantPath:  "/app/foo",
			wantQuery: "a=1&b=2",
		},
		{
			name:        "strip prefix",
			locPath:     "/app",
			stripPrefix: true,
			request:     "/app/foo",
			wantPath:    "/foo",
		},
		{
			name:        "strip prefix at the location root yields /",
			locPath:     "/app",
			stripPrefix: true,
			request:     "/app",
			wantPath:    "/",
		},
		{
			name:        "strip prefix with a trailing slash yields /",
			locPath:     "/app",
			stripPrefix: true,
			request:     "/app/",
			wantPath:    "/",
		},
		{
			name:        "strip prefix keeps a deep trailing slash",
			locPath:     "/app",
			stripPrefix: true,
			request:     "/app/foo/",
			wantPath:    "/foo/",
		},
		{
			name:        "strip prefix preserves the raw query",
			locPath:     "/app",
			stripPrefix: true,
			request:     "/app/foo?q=x%20y&r=1",
			wantPath:    "/foo",
			wantQuery:   "q=x%20y&r=1",
		},
		{
			name:        "strip prefix does not cut a sibling path",
			locPath:     "/app",
			stripPrefix: true,
			request:     "/application", // routed by the host default, not the location
			wantPath:    "/application",
		},
		{
			name:        "configured location path is normalized before stripping",
			locPath:     "/app/",
			stripPrefix: true,
			request:     "/app/foo",
			wantPath:    "/foo",
		},
		{
			name:     "upstream base path",
			locPath:  "/app",
			basePath: "/api",
			request:  "/app/foo",
			wantPath: "/api/app/foo",
		},
		{
			name:     "upstream base path with a trailing slash collapses",
			locPath:  "/app",
			basePath: "/api/",
			request:  "/app/foo",
			wantPath: "/api/app/foo",
		},
		{
			name:        "strip prefix then base path",
			locPath:     "/app",
			stripPrefix: true,
			basePath:    "/api",
			request:     "/app/v1/x?k=v",
			wantPath:    "/api/v1/x",
			wantQuery:   "k=v",
		},
		{
			name:        "strip prefix to root then base path",
			locPath:     "/app",
			stripPrefix: true,
			basePath:    "/api",
			request:     "/app",
			wantPath:    "/api/",
		},
		{
			name:        "exact rewrite runs after the strip",
			locPath:     "/app",
			stripPrefix: true,
			rewrite:     &model.RewriteMiddleware{ReplacePath: map[string]string{"/token": "/token/"}},
			request:     "/app/token",
			wantPath:    "/token/",
		},
		{
			name:        "prefix rewrite runs after the strip",
			locPath:     "/app",
			stripPrefix: true,
			rewrite: &model.RewriteMiddleware{PrefixRules: []model.RewriteRule{
				{From: "/old", To: "/new"},
			}},
			request:  "/app/old/thing",
			wantPath: "/new/thing",
		},
		{
			name:    "prefix rewrite is boundary matched",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{PrefixRules: []model.RewriteRule{
				{From: "/reports", To: "/r"},
			}},
			request:  "/reports-evil",
			wantPath: "/reports-evil",
		},
		{
			name:    "prefix rewrite matches the prefix exactly",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{PrefixRules: []model.RewriteRule{
				{From: "/reports", To: "/r"},
			}},
			request:  "/reports",
			wantPath: "/r",
		},
		{
			name:    "longest prefix rule wins",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{PrefixRules: []model.RewriteRule{
				{From: "/a", To: "/short"},
				{From: "/a/b/c", To: "/long"},
			}},
			request:  "/a/b/c/d",
			wantPath: "/long/d",
		},
		{
			name:    "prefix rule targeting root",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{PrefixRules: []model.RewriteRule{
				{From: "/app", To: "/"},
			}},
			request:  "/app/x",
			wantPath: "/x",
		},
		{
			name:    "regex rewrite with a capture group",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{RegexRules: []model.RewriteRule{
				{From: `/user/([0-9]+)`, To: "/u/$1"},
			}},
			request:  "/user/42/profile",
			wantPath: "/u/42/profile",
		},
		{
			name:    "regex rewrite is anchored at the start",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{RegexRules: []model.RewriteRule{
				{From: `/admin`, To: "/private"},
			}},
			request:  "/public/admin",
			wantPath: "/public/admin",
		},
		{
			name:    "regex rewrite honours an explicit leading anchor",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{RegexRules: []model.RewriteRule{
				{From: `^/v(1|2)/`, To: "/api/"},
			}},
			request:  "/v2/things",
			wantPath: "/api/things",
		},
		{
			name:    "exact wins over prefix and regex",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{
				ReplacePath: map[string]string{"/a/b": "/exact"},
				PrefixRules: []model.RewriteRule{{From: "/a", To: "/prefix"}},
				RegexRules:  []model.RewriteRule{{From: "/a.*", To: "/regex"}},
			},
			request:  "/a/b",
			wantPath: "/exact",
		},
		{
			name:    "prefix wins over regex",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{
				PrefixRules: []model.RewriteRule{{From: "/a", To: "/prefix"}},
				RegexRules:  []model.RewriteRule{{From: "/a.*", To: "/regex"}},
			},
			request:  "/a/b",
			wantPath: "/prefix/b",
		},
		{
			name:    "regex rewrite preserves the query",
			locPath: "/",
			rewrite: &model.RewriteMiddleware{RegexRules: []model.RewriteRule{
				{From: `/user/([0-9]+)`, To: "/u/$1"},
			}},
			request:   "/user/7?x=1",
			wantPath:  "/u/7",
			wantQuery: "x=1",
		},
		{
			name:        "strip, rewrite and base path together",
			locPath:     "/app",
			stripPrefix: true,
			rewrite: &model.RewriteMiddleware{PrefixRules: []model.RewriteRule{
				{From: "/old", To: "/new"},
			}},
			basePath:  "/api",
			request:   "/app/old/x?z=1",
			wantPath:  "/api/new/x",
			wantQuery: "z=1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			echo := newUpstreamEcho(t)
			host := model.ProxyHost{
				ObjectMeta: model.ObjectMeta{Name: "app"},
				Domains:    []string{"app.example.com"},
				Upstream:   echo.upstream(t, tc.basePath, ""),
				Locations:  []model.Location{{Path: tc.locPath, StripPrefix: tc.stripPrefix}},
			}
			cfg := model.Config{ProxyHosts: []model.ProxyHost{host}}
			if tc.rewrite != nil {
				cfg.Middlewares = []model.Middleware{{
					ObjectMeta: model.ObjectMeta{Name: "rw"},
					Type:       model.MWTypeRewrite,
					Rewrite:    tc.rewrite,
				}}
				cfg.ProxyHosts[0].Locations[0].Middlewares = []string{"rw"}
			}
			serveEscapeHatch(t, escapeHatchRouter(t, cfg), http.MethodGet, tc.request, nil)

			if echo.path != tc.wantPath {
				t.Errorf("upstream path = %q, want %q", echo.path, tc.wantPath)
			}
			if echo.query != tc.wantQuery {
				t.Errorf("upstream query = %q, want %q", echo.query, tc.wantQuery)
			}
		})
	}
}

// TestUpstreamHostHeader asserts r.Host at the backend for every hostHeader
// setting, on a single upstream and on an upstream-group member.
func TestUpstreamHostHeader(t *testing.T) {
	tests := []struct {
		name       string
		hostHeader string
		group      bool
		// want is the literal expected Host; when it is empty the expectation is
		// the upstream's own host:port, which the test only learns at runtime.
		want string
	}{
		{name: "default keeps the client Host", want: "app.example.com"},
		{name: "explicit hostname", hostHeader: "backend.example.com", want: "backend.example.com"},
		{name: "hostname with port", hostHeader: "backend.example.com:8443", want: "backend.example.com:8443"},
		{name: "upstream sentinel", hostHeader: model.UpstreamHostHeaderUpstream},
		{name: "group default keeps the client Host", group: true, want: "app.example.com"},
		{name: "group explicit hostname", group: true, hostHeader: "backend.example.com", want: "backend.example.com"},
		{name: "group upstream sentinel", group: true, hostHeader: model.UpstreamHostHeaderUpstream},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			echo := newUpstreamEcho(t)
			up := echo.upstream(t, "", tc.hostHeader)
			host := model.ProxyHost{
				ObjectMeta: model.ObjectMeta{Name: "app"},
				Domains:    []string{"app.example.com"},
			}
			cfg := model.Config{}
			if tc.group {
				host.UpstreamGroupRef = "g"
				cfg.UpstreamGroups = []model.UpstreamGroup{{
					ObjectMeta:  model.ObjectMeta{Name: "g"},
					Upstreams:   []model.GroupUpstream{{Upstream: up}},
					HealthCheck: model.HealthCheck{IntervalSeconds: 3600},
				}}
			} else {
				host.Upstream = up
			}
			cfg.ProxyHosts = []model.ProxyHost{host}
			serveEscapeHatch(t, escapeHatchRouter(t, cfg), http.MethodGet, "/x", nil)

			want := tc.want
			if want == "" {
				want = echo.addr()
			}
			if echo.host != want {
				t.Errorf("upstream Host = %q, want %q", echo.host, want)
			}
		})
	}
}

// TestGroupMemberBasePath proves an upstream-group member's own base path is
// honoured by the failover transport, which composes the path itself rather than
// riding on the reverse proxy's target URL.
func TestGroupMemberBasePath(t *testing.T) {
	echo := newUpstreamEcho(t)
	cfg := model.Config{
		UpstreamGroups: []model.UpstreamGroup{{
			ObjectMeta:  model.ObjectMeta{Name: "g"},
			Upstreams:   []model.GroupUpstream{{Upstream: echo.upstream(t, "/api", "")}},
			HealthCheck: model.HealthCheck{IntervalSeconds: 3600},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:       model.ObjectMeta{Name: "app"},
			Domains:          []string{"app.example.com"},
			UpstreamGroupRef: "g",
		}},
	}
	serveEscapeHatch(t, escapeHatchRouter(t, cfg), http.MethodGet, "/v1/x?a=1", nil)

	if echo.path != "/api/v1/x" {
		t.Errorf("upstream path = %q, want %q", echo.path, "/api/v1/x")
	}
	if echo.query != "a=1" {
		t.Errorf("upstream query = %q, want %q", echo.query, "a=1")
	}
}

// TestEscapeHatchPreservesMethodAndBody keeps the promise the exact-match
// rewrite has always carried: path composition is internal, never a redirect,
// and the method and body reach the upstream unchanged.
func TestEscapeHatchPreservesMethodAndBody(t *testing.T) {
	echo := newUpstreamEcho(t)
	cfg := model.Config{
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "rw"},
			Type:       model.MWTypeRewrite,
			Rewrite: &model.RewriteMiddleware{RegexRules: []model.RewriteRule{
				{From: `/sub(mit)`, To: "/post$1"},
			}},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "app"},
			Domains:    []string{"app.example.com"},
			Upstream:   echo.upstream(t, "/api", ""),
			Locations: []model.Location{{
				Path:        "/app",
				StripPrefix: true,
				Middlewares: []string{"rw"},
			}},
		}},
	}
	w := serveEscapeHatch(t, escapeHatchRouter(t, cfg), http.MethodPost, "/app/submit", strings.NewReader("payload"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never a redirect)", w.Code)
	}
	if echo.path != "/api/postmit" {
		t.Errorf("upstream path = %q, want %q", echo.path, "/api/postmit")
	}
	if echo.body != "payload" {
		t.Errorf("upstream body = %q, want %q", echo.body, "payload")
	}
}
