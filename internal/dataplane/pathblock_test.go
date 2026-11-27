package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/analytics"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// probeList is the list most tests below use.
func probeList() model.PathBlockMiddleware {
	return model.PathBlockMiddleware{
		Extensions: []string{".php", ".ENV", ".tar.gz"},
		Names:      []string{".git", "wp-admin", "WP-Login.php"},
		Globs:      []string{"/server-status", "/backup/*.zip", "/[0-9][0-9][0-9][0-9].sql", "/Admin-Console/*", "/old"},
	}
}

func TestPathBlockMatches(t *testing.T) {
	p := compilePathBlock(probeList(), nil)
	blocked := []string{
		"/.env", "/prod.env", "/a/b/x.PHP", "/x.php",
		"/x.php/extra/path/info", // a backend that takes path info still runs x.php
		"/x.php.", "/x.php ", "/x.php. .", "/x.php\x00.jpg",
		"/a/.git/config", "/.GIT", "/.git", "/.git./config",
		"/wp-admin/", "/wp-admin", "/blog/wp-login.php", "/a/WP-ADMIN/b",
		"/dump.tar.gz", "/.tar.gz",
		"/server-status", "/Server-Status", "/backup/site.zip", "/BACKUP/SITE.ZIP", "/2024.sql",
		// Globs see the path the way the other lists see a segment: no trailing
		// slash, no trailing dots or spaces, nothing after a NUL.
		"/server-status/", "/SERVER-STATUS/", "/server-status.", "/backup/site.zip.", "/backup/site.zip ",
		"/backup/site.zip\x00", "/backup/site.zip\x00.txt", "/backup./site.zip/", "/backup./site.zip", "/backup /site.zip",
		// A last segment of only dots or spaces folds away with its slash, and
		// nothing after a NUL counts.
		"/server-status/...", "/server-status/ ", "/backup/site.zip/ ", "/backup/site.zip/./ .", "/backup/site.zip\x00/x",
		// A pattern is lower-cased like the path.
		"/admin-console/x", "/ADMIN-CONSOLE/x", "/old", "/old/", "/OLD/",
		// A segment that folds to nothing goes with its slash.
		"/ /server-status", "/.../server-status", "/backup/ /site.zip",
		// The fold only adds matches: the path as it is still counts.
		"/admin-console/", "/admin-console/ ", "/admin-console/...",
	}
	for _, path := range blocked {
		if !p.matches(path) {
			t.Errorf("matches(%q) = false, want blocked", path)
		}
	}
	served := []string{
		"/", "", "/index.html", "/php", "/x.phps", "/x.php5", "/xphp", "/a.env.txt",
		"/.gitignore", "/.github/workflows/ci.yml", "/git", "/wp-admin2", "/my-wp-admin",
		"/.well-known/acme-challenge/token", "/x.tar", "/x.gz",
		"/backup/a/site.zip", "/backup/site.zip/x", "/server-status/x", "/x/server-status",
		"/20245.sql", "/abcd.sql", "/env", "/a/b/c",
		"/admin-console", "/admin-console/x/y", "/old/x", "/older",
	}
	for _, path := range served {
		if p.matches(path) {
			t.Errorf("matches(%q) = true, want served", path)
		}
	}
}

// An entry validation would refuse must not turn into a broad match if it
// reaches the compiler anyway (a hand-edited file).
func TestPathBlockCompileDropsMalformedEntries(t *testing.T) {
	p := compilePathBlock(model.PathBlockMiddleware{
		Extensions: []string{"php", "", "."},
		Names:      []string{""},
		Globs:      []string{"/[", "/ok"},
	}, nil)
	for _, path := range []string{"/", "/morphp", "/x.php", "/a/", "/x."} {
		if p.matches(path) {
			t.Errorf("matches(%q) = true with only malformed entries", path)
		}
	}
	if !p.matches("/ok") {
		t.Error("the valid glob beside a malformed one must still match")
	}
}

// A list with globs does not walk a path longer than maxGlobPathLen: it counts
// as listed. A list without globs has no such bound, its cost is linear.
func TestPathBlockGlobPathBound(t *testing.T) {
	withGlobs := compilePathBlock(model.PathBlockMiddleware{Globs: []string{"/x/*.zip"}}, nil)
	noGlobs := compilePathBlock(model.PathBlockMiddleware{Extensions: []string{".zip"}}, nil)
	atBound := "/" + strings.Repeat("a", maxGlobPathLen-1)
	if withGlobs.matches(atBound) {
		t.Error("a path at the bound that no glob matches must be served")
	}
	if !withGlobs.matches(atBound + "a") {
		t.Error("a path over the bound must count as listed when the list has globs")
	}
	if noGlobs.matches(atBound + strings.Repeat("a", 100000)) {
		t.Error("a list without globs must not block a long path")
	}
	// The worst pattern sets validation accepts (the total size of the globs
	// is capped), against the longest path that is still walked.
	var globs, classes []string
	for i := 0; i < model.MaxPathBlockGlobBytes/model.MaxPathBlockEntryLen; i++ {
		globs = append(globs, "/*"+strings.Repeat("a", model.MaxPathBlockEntryLen-3)+string(rune('0'+i%10)))
		classes = append(classes, "/*["+strings.Repeat("a", model.MaxPathBlockEntryLen-5)+"]"+string(rune('0'+i%10)))
	}
	for _, set := range [][]string{globs, classes} {
		spec := model.Middleware{ObjectMeta: model.ObjectMeta{Name: "worst"}, Type: model.MWTypePathBlock, PathBlock: &model.PathBlockMiddleware{Globs: set}}
		if err := spec.Validate(); err != nil {
			t.Fatalf("the worst-case set must be one validation accepts: %v", err)
		}
	}
	worstClasses := compilePathBlock(model.PathBlockMiddleware{Globs: classes}, nil)
	if worstClasses.matches(atBound) {
		t.Fatal("unexpected match")
	}
	worst := compilePathBlock(model.PathBlockMiddleware{Globs: globs}, nil)
	res := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if worst.matches(atBound) {
				b.Fatal("unexpected match")
			}
		}
	})
	if per := res.NsPerOp(); per > 400_000_000 {
		t.Fatalf("the worst glob set took %d ns on a %d byte path", per, len(atBound))
	}
	t.Logf("worst glob set on a %d byte path: %d ns", len(atBound), res.NsPerOp())
}

// A long path of many dotted segments stays cheap: one lookup per distinct
// extension length per segment.
func TestPathBlockMatchIsLinear(t *testing.T) {
	var exts []string
	for i := 2; i < 2+model.MaxPathBlockEntries && i <= model.MaxPathBlockEntryLen; i++ {
		exts = append(exts, "."+strings.Repeat("x", i-1))
	}
	p := compilePathBlock(model.PathBlockMiddleware{Extensions: exts}, nil)
	path := strings.Repeat("/"+strings.Repeat(".", model.MaxPathBlockEntryLen), 4000)
	res := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if p.matches(path) {
				b.Fatal("unexpected match")
			}
		}
	})
	if per := res.NsPerOp(); per > 500_000_000 {
		t.Fatalf("matching a %d byte path took %d ns", len(path), per)
	}
}

// blockStack serves cfg through the whole data plane (path normalisation,
// router, chain) with the fleet list installed, and counts upstream calls.
type blockStack struct {
	h     http.Handler
	calls *atomic.Int64
	hook  *fakeHook
}

func newBlockStack(t *testing.T, fleet *model.PathBlockMiddleware, build func(up model.Upstream) model.Config) blockStack {
	t.Helper()
	var calls atomic.Int64
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("backend"))
	}))
	t.Cleanup(closeFn)
	prev := currentPathBlock()
	t.Cleanup(func() { SetPathBlock(prev) })
	SetPathBlock(fleet)
	hook := installHook(t)
	s := New(Config{})
	if err := s.Reload(build(up)); err != nil {
		t.Fatal(err)
	}
	return blockStack{h: s.observe(http.HandlerFunc(s.dispatchHTTP)), calls: &calls, hook: hook}
}

// do sends one request for the raw request target and returns the status,
// whether the upstream was reached, and the body.
func (b blockStack) do(method, host, target string) (int, bool, string) {
	before := b.calls.Load()
	req := httptest.NewRequest(method, "http://"+host+target, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	b.h.ServeHTTP(rec, req)
	return rec.Code, b.calls.Load() != before, rec.Body.String()
}

func siteHost(up model.Upstream) model.ProxyHost {
	return model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "site"},
		Domains:    []string{"site.example.com"},
		Upstream:   up,
	}
}

func TestPathBlockFleetList(t *testing.T) {
	fleet := probeList()
	b := newBlockStack(t, &fleet, func(up model.Upstream) model.Config {
		return model.Config{ProxyHosts: []model.ProxyHost{siteHost(up)}}
	})
	for _, target := range []string{
		"/.env", "/wp-admin/x.php", "/a/b/.git/config",
		// Encoded and dot-segment spellings take their final form before the
		// list is read.
		"/%2Eenv", "/%2eENV", "/a/%2e%2e/.env", "/a/../x%2Ephp", "/a%2F.git%2Fconfig", "/./wp-admin/",
		"/x.php%00.png", "/x.php%20", "/x.php.", "/x.php?a=1",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
			code, reached, _ := b.do(method, "site.example.com", target)
			if code != http.StatusNotFound || reached {
				t.Errorf("%s %s = %d (upstream reached %v), want 404 without the upstream", method, target, code, reached)
			}
		}
	}
	if n := b.hook.denials["site|path-block"]; n != 13*3 {
		t.Errorf("denials = %v, want %d site|path-block", b.hook.denials, 13*3)
	}
	for _, target := range []string{"/", "/index.html", "/.well-known/security.txt", "/docs/php", "/x?file=.env", "/x?a=/wp-admin/"} {
		code, reached, _ := b.do(http.MethodGet, "site.example.com", target)
		if code != http.StatusOK || !reached {
			t.Errorf("GET %s = %d (upstream reached %v), want 200 from the upstream", target, code, reached)
		}
	}
}

func TestPathBlockHostSwitchAndMiddleware(t *testing.T) {
	fleet := model.PathBlockMiddleware{Extensions: []string{".php"}}
	b := newBlockStack(t, &fleet, func(up model.Upstream) model.Config {
		inherit := siteHost(up)
		inherit.PathBlock = model.PathBlockInherit

		off := siteHost(up)
		off.Name, off.Domains, off.PathBlock = "php-app", []string{"php.example.com"}, model.PathBlockOff

		// The switch covers the fleet list only: a referenced list still applies,
		// and one referenced by a location applies under that location alone.
		own := siteHost(up)
		own.Name, own.Domains, own.PathBlock = "own", []string{"own.example.com"}, model.PathBlockOff
		own.Middlewares = []string{"no-dumps"}
		own.Locations = []model.Location{{Path: "/files", Upstream: &up, Middlewares: []string{"no-zip"}}}

		edge := siteHost(up)
		edge.Name, edge.Domains, edge.AdminEdge = "edge", []string{"edge.example.com"}, true

		return model.Config{
			Middlewares: []model.Middleware{
				{ObjectMeta: model.ObjectMeta{Name: "no-dumps"}, Type: model.MWTypePathBlock,
					PathBlock: &model.PathBlockMiddleware{Extensions: []string{".sql"}, DenyStatus: 410, DenyWith: model.BouncerDenyWithPlain}},
				{ObjectMeta: model.ObjectMeta{Name: "no-zip"}, Type: model.MWTypePathBlock,
					PathBlock: &model.PathBlockMiddleware{Extensions: []string{".zip"}}},
			},
			ProxyHosts: []model.ProxyHost{inherit, off, own, edge},
		}
	})
	cases := []struct {
		host, target string
		want         int
	}{
		{"site.example.com", "/x.php", 404},
		{"site.example.com", "/x.sql", 200},
		{"php.example.com", "/x.php", 200},
		{"php.example.com", "/index.php/apps/files", 200},
		{"own.example.com", "/x.php", 200},
		{"own.example.com", "/x.sql", 410},
		{"own.example.com", "/files/x.sql", 410},
		{"own.example.com", "/x.zip", 200},
		{"own.example.com", "/files/x.zip", 404},
		{"own.example.com", "/files/x.php", 200},
		// The admin edge host never takes the fleet list.
		{"edge.example.com", "/x.php", 200},
	}
	for _, c := range cases {
		code, reached, body := b.do(http.MethodGet, c.host, c.target)
		if code != c.want || reached != (c.want == 200) {
			t.Errorf("GET %s%s = %d (upstream reached %v), want %d", c.host, c.target, code, reached, c.want)
		}
		if c.want == http.StatusGone && strings.TrimSpace(body) != http.StatusText(http.StatusGone) {
			t.Errorf("GET %s%s body = %q, want the plain status text", c.host, c.target, body)
		}
	}
}

// The list answers before the access lists and authentication (so a probe
// drives neither) and after the rate limit (so a probing client spends its
// budget).
func TestPathBlockChainPosition(t *testing.T) {
	fleet := model.PathBlockMiddleware{Names: []string{".env"}}
	b := newBlockStack(t, &fleet, func(up model.Upstream) model.Config {
		gated := siteHost(up)
		gated.AccessLists = []string{"deny-all"}
		gated.Auth = &model.AuthMiddleware{Mode: model.AuthModeBasic, Basic: &model.BasicAuthSpec{
			Users: []model.BasicAuthUser{{Username: "u", PasswordHash: "$2a$04$abcdefghijklmnopqrstuuUXVJ8mqrH1qd0vQNvY5oZb0kTnTaXIm"}},
		}}

		limited := siteHost(up)
		limited.Name, limited.Domains = "limited", []string{"limited.example.com"}
		limited.RateLimit = &model.RateLimitMiddleware{Requests: 2, Window: "1h"}

		authOnly := siteHost(up)
		authOnly.Name, authOnly.Domains = "auth-only", []string{"auth.example.com"}
		authOnly.Auth = gated.Auth

		return model.Config{
			AccessLists: []model.AccessList{{
				ObjectMeta:    model.ObjectMeta{Name: "deny-all"},
				DefaultAction: model.ActionDeny,
			}},
			ProxyHosts: []model.ProxyHost{gated, limited, authOnly},
		}
	})
	if code, _, _ := b.do(http.MethodGet, "site.example.com", "/.env"); code != 404 {
		t.Errorf("listed path on an access-list host = %d, want 404 from the path block", code)
	}
	if code, _, _ := b.do(http.MethodGet, "site.example.com", "/"); code != 403 {
		t.Errorf("other path on an access-list host = %d, want 403 from the access list", code)
	}
	if b.hook.denials["site|path-block"] != 1 || b.hook.denials["site|access-list"] != 1 {
		t.Errorf("denials = %v, want one path-block and one access-list", b.hook.denials)
	}
	if code, _, _ := b.do(http.MethodGet, "auth.example.com", "/.env"); code != 404 {
		t.Errorf("listed path on an auth host = %d, want 404 from the path block", code)
	}
	if code, _, _ := b.do(http.MethodGet, "auth.example.com", "/"); code != 401 {
		t.Errorf("other path on an auth host = %d, want 401 from the gate", code)
	}
	for i, want := range []int{404, 404, 429} {
		if code, _, _ := b.do(http.MethodGet, "limited.example.com", "/.env"); code != want {
			t.Errorf("probe %d on a rate-limited host = %d, want %d", i+1, code, want)
		}
	}
}

func TestPathBlockAllowFrom(t *testing.T) {
	// httptest requests come from 192.0.2.1.
	lists := []model.AccessList{
		{ObjectMeta: model.ObjectMeta{Name: "office"}, DefaultAction: model.ActionDeny,
			Rules: []model.IPRule{{Action: model.ActionAllow, CIDR: "192.0.2.0/24"}}},
		// First match wins: the deny rule comes first, so the client is not exempt.
		{ObjectMeta: model.ObjectMeta{Name: "office-minus-one"}, DefaultAction: model.ActionDeny,
			Rules: []model.IPRule{{Action: model.ActionDeny, CIDR: "192.0.2.1"}, {Action: model.ActionAllow, CIDR: "192.0.2.0/24"}}},
	}
	cases := []struct {
		name      string
		allowFrom []string
		want      int
	}{
		{"no exemption", nil, 404},
		{"cidr holding the client", []string{"192.0.2.0/24"}, 200},
		{"bare address", []string{"192.0.2.1"}, 200},
		{"other network", []string{"198.51.100.0/24", "2001:db8::/32"}, 404},
		{"list allowing the client", []string{"@office"}, 200},
		{"list denying the client first", []string{"@office-minus-one"}, 404},
		{"any entry is enough", []string{"198.51.100.0/24", "@office-minus-one", "@office"}, 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fleet := model.PathBlockMiddleware{Extensions: []string{".php"}, AllowFrom: c.allowFrom}
			b := newBlockStack(t, &fleet, func(up model.Upstream) model.Config {
				return model.Config{AccessLists: lists, ProxyHosts: []model.ProxyHost{siteHost(up)}}
			})
			if code, _, _ := b.do(http.MethodGet, "site.example.com", "/x.php"); code != c.want {
				t.Errorf("status = %d, want %d", code, c.want)
			}
		})
	}
}

// A list reference that does not resolve, and a client address that cannot be
// resolved, exempt nobody.
func TestPathBlockExemptionFailsClosed(t *testing.T) {
	spec := model.PathBlockMiddleware{Extensions: []string{".php"}, AllowFrom: []string{"@gone", "0.0.0.0/0", "::/0"}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	run := func(p *pathBlock, ipOf func(*http.Request) net.IP) int {
		rec := httptest.NewRecorder()
		pathBlockHandler(p, ipOf, "site", nil, next).ServeHTTP(rec, httptest.NewRequest("GET", "/x.php", nil))
		return rec.Code
	}
	missing := func(string) func(net.IP) bool { return nil }
	if code := run(compilePathBlock(spec, missing), func(*http.Request) net.IP { return nil }); code != 404 {
		t.Errorf("unresolved client address = %d, want 404", code)
	}
	if code := run(compilePathBlock(spec, missing), nil); code != 404 {
		t.Errorf("no client address resolver = %d, want 404", code)
	}
	only := model.PathBlockMiddleware{Extensions: []string{".php"}, AllowFrom: []string{"@gone"}}
	if code := run(compilePathBlock(only, missing), ipFrom("192.0.2.1")); code != 404 {
		t.Errorf("unresolved list = %d, want 404", code)
	}
	if code := run(compilePathBlock(only, nil), ipFrom("192.0.2.1")); code != 404 {
		t.Errorf("no list resolver = %d, want 404", code)
	}
}

// denyWith: error-page (the default) renders the configured page for the
// status; plain never does.
func TestPathBlockDenyWith(t *testing.T) {
	for _, c := range []struct {
		denyWith string
		wantPage bool
	}{{"", true}, {model.BouncerDenyWithErrorPage, true}, {model.BouncerDenyWithPlain, false}} {
		fleet := model.PathBlockMiddleware{Extensions: []string{".php"}, DenyWith: c.denyWith}
		b := newBlockStack(t, &fleet, func(up model.Upstream) model.Config {
			h := siteHost(up)
			h.ErrorPages = &model.ErrorPagesConfig{Inline: map[string]string{"404": "custom not found page"}}
			return model.Config{ProxyHosts: []model.ProxyHost{h}}
		})
		code, _, body := b.do(http.MethodGet, "site.example.com", "/x.php")
		if code != 404 {
			t.Fatalf("denyWith %q: status = %d, want 404", c.denyWith, code)
		}
		if got := strings.Contains(body, "custom not found page"); got != c.wantPage {
			t.Errorf("denyWith %q: custom page rendered = %v, want %v (body %q)", c.denyWith, got, c.wantPage, body)
		}
	}
}

// With no settings.pathBlock nothing is blocked.
func TestPathBlockUnsetBlocksNothing(t *testing.T) {
	b := newBlockStack(t, nil, func(up model.Upstream) model.Config {
		return model.Config{ProxyHosts: []model.ProxyHost{siteHost(up)}}
	})
	if code, _, _ := b.do(http.MethodGet, "site.example.com", "/x.php"); code != 200 {
		t.Fatalf("no fleet list: status = %d, want 200", code)
	}
}

// The access log entry of a refused request names the tier that refused it,
// with analytics off; a served request carries no reason.
func TestAccessLogRecordsDenialReason(t *testing.T) {
	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()
	prev := currentPathBlock()
	t.Cleanup(func() { SetPathBlock(prev) })
	SetPathBlock(&model.PathBlockMiddleware{Names: []string{".env"}})

	gated := siteHost(up)
	gated.Name, gated.Domains, gated.AccessLists = "gated", []string{"gated.example.com"}, []string{"deny-all"}
	s := New(Config{})
	s.SetAccessLog(true)
	if err := s.Reload(model.Config{
		AccessLists: []model.AccessList{{ObjectMeta: model.ObjectMeta{Name: "deny-all"}, DefaultAction: model.ActionDeny}},
		ProxyHosts:  []model.ProxyHost{siteHost(up), gated},
	}); err != nil {
		t.Fatal(err)
	}
	h := s.observe(http.HandlerFunc(s.dispatchHTTP))
	for _, c := range []struct{ host, path string }{
		{"site.example.com", "/.env"}, {"site.example.com", "/"}, {"gated.example.com", "/"},
	} {
		req := httptest.NewRequest("GET", "http://"+c.host+c.path, nil)
		req.Host = c.host
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	got := map[string]string{}
	for _, e := range s.RecentLogs() {
		got[e.Host+e.Path] = e.Deny
	}
	want := map[string]string{"site.example.com/.env": "path-block", "site.example.com/": "", "gated.example.com/": "access-list"}
	for k, v := range want {
		if d, ok := got[k]; !ok || d != v {
			t.Errorf("entry %s: deny = %q (present %v), want %q", k, d, ok, v)
		}
	}
}

// A referenced path-block middleware resolves "@<list>" exemptions and renders
// the host's error page, exactly like the fleet list.
func TestPathBlockMiddlewareExemptionAndErrorPage(t *testing.T) {
	b := newBlockStack(t, nil, func(up model.Upstream) model.Config {
		open := siteHost(up)
		open.Middlewares = []string{"exempt-office"}
		open.ErrorPages = &model.ErrorPagesConfig{Inline: map[string]string{"404": "custom not found page"}}

		closed := siteHost(up)
		closed.Name, closed.Domains, closed.Middlewares = "closed", []string{"closed.example.com"}, []string{"exempt-elsewhere"}
		closed.ErrorPages = open.ErrorPages
		return model.Config{
			AccessLists: []model.AccessList{
				{ObjectMeta: model.ObjectMeta{Name: "office"}, DefaultAction: model.ActionDeny,
					Rules: []model.IPRule{{Action: model.ActionAllow, CIDR: "192.0.2.0/24"}}},
				{ObjectMeta: model.ObjectMeta{Name: "elsewhere"}, DefaultAction: model.ActionDeny,
					Rules: []model.IPRule{{Action: model.ActionAllow, CIDR: "198.51.100.0/24"}}},
			},
			Middlewares: []model.Middleware{
				{ObjectMeta: model.ObjectMeta{Name: "exempt-office"}, Type: model.MWTypePathBlock,
					PathBlock: &model.PathBlockMiddleware{Extensions: []string{".php"}, AllowFrom: []string{"@office"}}},
				{ObjectMeta: model.ObjectMeta{Name: "exempt-elsewhere"}, Type: model.MWTypePathBlock,
					PathBlock: &model.PathBlockMiddleware{Extensions: []string{".php"}, AllowFrom: []string{"@elsewhere"}}},
			},
			ProxyHosts: []model.ProxyHost{open, closed},
		}
	})
	if code, reached, _ := b.do(http.MethodGet, "site.example.com", "/x.php"); code != 200 || !reached {
		t.Errorf("client in the middleware's exempt list = %d, want 200", code)
	}
	code, _, body := b.do(http.MethodGet, "closed.example.com", "/x.php")
	if code != 404 || !strings.Contains(body, "custom not found page") {
		t.Errorf("client outside the exempt list = %d, body %q, want 404 with the host's error page", code, body)
	}
}

// The exemption compares the client address the host derives (X-Forwarded-For
// from a trusted proxy), not the connection peer.
func TestPathBlockExemptionUsesDerivedClientIP(t *testing.T) {
	cases := []struct {
		name      string
		allowFrom string
		xff       string
		want      int
	}{
		{"client behind the proxy is exempt", "198.51.100.0/24", "198.51.100.7", 200},
		{"the proxy's own address does not exempt its clients", "192.0.2.0/24", "198.51.100.7", 404},
		{"no forwarded address: the peer is the client", "192.0.2.0/24", "", 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fleet := model.PathBlockMiddleware{Extensions: []string{".php"}, AllowFrom: []string{c.allowFrom}}
			b := newBlockStack(t, &fleet, func(up model.Upstream) model.Config {
				h := siteHost(up)
				h.TrustedProxies = &[]string{"192.0.2.1"} // httptest requests come from here
				return model.Config{ProxyHosts: []model.ProxyHost{h}}
			})
			req := httptest.NewRequest(http.MethodGet, "http://site.example.com/x.php", nil)
			req.Host = "site.example.com"
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			rec := httptest.NewRecorder()
			b.h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}

// One compiled list per build: every route that takes the fleet list or the
// same middleware shares it.
func TestPathBlockCompiledOncePerBuild(t *testing.T) {
	reg := buildRegistry(model.Config{})
	spec := model.PathBlockMiddleware{Extensions: []string{".php"}}
	a, b := reg.pathBlockFor("fleet", spec), reg.pathBlockFor("fleet", spec)
	if a != b {
		t.Error("the fleet list was compiled twice in one build")
	}
	if reg.pathBlockFor("mw:x", spec) == a {
		t.Error("different lists must not share a compiled list")
	}

	// The chain takes its lists from that cache: two routes, one compiled fleet
	// list and one compiled middleware.
	prev := currentPathBlock()
	t.Cleanup(func() { SetPathBlock(prev) })
	SetPathBlock(&spec)
	cfg := model.Config{Middlewares: []model.Middleware{{ObjectMeta: model.ObjectMeta{Name: "probes"}, Type: model.MWTypePathBlock, PathBlock: &spec}}}
	reg = buildRegistry(cfg)
	for _, name := range []string{"one", "two"} {
		buildChain(okHandler(), model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: name}, Middlewares: []string{"probes"}}, reg, nil)
	}
	if len(reg.pathBlocks) != 2 || reg.pathBlocks["fleet"] == nil || reg.pathBlocks["mw:probes"] == nil {
		t.Errorf("compiled lists after two routes = %d (%v), want the fleet list and the middleware once each", len(reg.pathBlocks), reg.pathBlocks)
	}
}

// A WebSocket or other long-lived tunnel is tied to what decides access to its
// host; the path block lists are part of that, so a change closes the tunnel
// at the next reload like a guard change does.
func TestHostChainKeyCoversPathBlock(t *testing.T) {
	prev := currentPathBlock()
	t.Cleanup(func() { SetPathBlock(prev) })
	SetPathBlock(nil)
	h := model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "site"}, Middlewares: []string{"probes"}}
	mw := func(ext string) model.Config {
		return model.Config{Middlewares: []model.Middleware{{ObjectMeta: model.ObjectMeta{Name: "probes"}, Type: model.MWTypePathBlock,
			PathBlock: &model.PathBlockMiddleware{Extensions: []string{ext}}}}}
	}
	base := hostChainKey(h, mw(".php"), nil)
	if hostChainKey(h, mw(".php"), nil) != base {
		t.Fatal("the key is not stable")
	}
	if hostChainKey(h, mw(".sql"), nil) == base {
		t.Error("a changed path-block middleware must change the key")
	}
	SetPathBlock(&model.PathBlockMiddleware{Names: []string{".git"}})
	fleet := hostChainKey(h, mw(".php"), nil)
	if fleet == base {
		t.Error("a new fleet list must change the key of a host it covers")
	}
	off := h
	off.PathBlock = model.PathBlockOff
	offKey := hostChainKey(off, mw(".php"), nil)
	if offKey == fleet {
		t.Error("pathBlock: off must change the key")
	}
	SetPathBlock(&model.PathBlockMiddleware{Names: []string{".svn"}})
	if hostChainKey(h, mw(".php"), nil) == fleet {
		t.Error("a changed fleet list must change the key of a host it covers")
	}
	if hostChainKey(off, mw(".php"), nil) != offKey {
		t.Error("a changed fleet list must not change the key of a host that opted out")
	}
	// The admin edge host never takes the fleet list either.
	edge := h
	edge.AdminEdge = true
	edgeKey := hostChainKey(edge, mw(".php"), nil)
	SetPathBlock(&model.PathBlockMiddleware{Names: []string{".hg"}})
	if hostChainKey(edge, mw(".php"), nil) != edgeKey {
		t.Error("a changed fleet list must not change the key of the admin edge host")
	}
}

// flipSink changes its answer on every Active call, like a collector switched
// on and off while requests are in flight: whatever the number of reads per
// request, two reads that should agree soon disagree.
type flipSink struct{ calls atomic.Int32 }

func (f *flipSink) Record(analytics.Event) {}
func (f *flipSink) Active() bool           { return f.calls.Add(1)%2 == 0 }

// A collector that turns on mid-request must not leave the observer with an
// event to record and no request note to read the denial reason from.
func TestObserveReadsAnalyticsActiveOnce(t *testing.T) {
	sink := &flipSink{}
	SetAnalyticsSink(sink)
	t.Cleanup(func() { SetAnalyticsSink(nil) })
	s := New(Config{})
	h := s.observe(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for i := 0; i < 8; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "http://site.example.com/", nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %d: status = %d", i, rec.Code)
		}
	}
}
