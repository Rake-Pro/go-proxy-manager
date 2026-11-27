package dataplane

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// The end-to-end walk of per-host oidc mode: complete sign-ins against the
// strict provider in oidcprovider_test.go, through the real HTTPS serving path
// (buildRouter, then serveHTTPS behind a TLS listener), with a cookie-carrying
// client that follows redirects the way a browser does. It holds gpm to
// docs/how-to/per-host-sso.md without any external identity provider.

const (
	walkClient       = "gpm-walk"
	walkSecret       = "gpm-walk-secret"
	walkNoPKCEClient = "gpm-walk-nopkce"
	walkPublicClient = "gpm-walk-public"
	walkIdP          = "op"
)

// --- the upstream ----------------------------------------------------------

type walkSeen struct {
	Host   string
	URI    string
	Header http.Header
}

// walkUpstream records every request it receives and answers with a body that
// names the host and the request URI.
type walkUpstream struct {
	srv  *httptest.Server
	mu   sync.Mutex
	seen []walkSeen
}

func newWalkUpstream(t *testing.T) *walkUpstream {
	t.Helper()
	u := &walkUpstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.seen = append(u.seen, walkSeen{Host: r.Host, URI: r.URL.RequestURI(), Header: r.Header.Clone()})
		u.mu.Unlock()
		_, _ = io.WriteString(w, "UPSTREAM "+r.Host+" "+r.URL.RequestURI())
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *walkUpstream) target(t *testing.T) model.Upstream {
	t.Helper()
	pu, err := url.Parse(u.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(pu.Port())
	if err != nil {
		t.Fatal(err)
	}
	return model.Upstream{Scheme: "http", Host: pu.Hostname(), Port: port}
}

func (u *walkUpstream) requests() []walkSeen {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]walkSeen(nil), u.seen...)
}

func (u *walkUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.seen)
}

// --- the bench -------------------------------------------------------------

type walkBench struct {
	t   *testing.T
	op  *testOP
	up  *walkUpstream
	srv *httptest.Server
}

// newWalkBench starts a provider with three clients (confidential with PKCE
// required, confidential without, public with PKCE required) and four users,
// and an upstream. The clients have no redirect URIs until a test registers
// them.
func newWalkBench(t *testing.T) *walkBench {
	t.Helper()
	// The revocation watermark is process-global; reset it so test order never
	// matters.
	t.Cleanup(func() { ssoNotBefore.Store(0) })
	b := &walkBench{t: t, op: newTestOP(t), up: newWalkUpstream(t)}
	b.op.addClient(opClient{ID: walkClient, Secret: walkSecret, RequirePKCE: true})
	b.op.addClient(opClient{ID: walkNoPKCEClient, Secret: walkSecret})
	b.op.addClient(opClient{ID: walkPublicClient, RequirePKCE: true})
	for _, u := range []opUser{
		{Login: "admin1", Sub: "sub-admin1", Email: "admin1@example.com", EmailVerified: true, Name: "Admin One", Groups: []string{"admins"}},
		{Login: "staff1", Sub: "sub-staff1", Email: "staff1@example.com", EmailVerified: true, Name: "Staff One", Groups: []string{"staff", "readers"}},
		{Login: "nogroup1", Sub: "sub-nogroup1", Email: "nogroup1@example.com", EmailVerified: true, Name: "No Group"},
		{Login: "unverified1", Sub: "sub-unverified1", Email: "unverified1@example.com", EmailVerified: false, Name: "Unverified", Groups: []string{"staff"}},
	} {
		b.op.addUser(u)
	}
	return b
}

// register registers the callback URI of each domain for a client.
func (b *walkBench) register(clientID string, domains ...string) {
	for _, d := range domains {
		b.op.register(clientID, "https://"+d+oidcCallbackPath)
	}
}

// idp is an oidc identity provider for the bench's provider and client
// walkClient; edit adjusts it.
func (b *walkBench) idp(name string, edit func(*model.IdentityProvider)) model.IdentityProvider {
	p := model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: name},
		Type:       model.IdPTypeOIDC,
		OIDC:       &model.OIDCSpec{IssuerURL: b.op.Issuer, ClientID: walkClient, ClientSecret: walkSecret},
	}
	if edit != nil {
		edit(&p)
	}
	return p
}

// host is a proxy host on the bench's upstream, gated inline by auth when it
// is not nil.
func (b *walkBench) host(name string, auth *model.AuthMiddleware, domains ...string) model.ProxyHost {
	return model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: name},
		Domains:    domains,
		Upstream:   b.up.target(b.t),
		TLS:        model.TLSSettings{ForceSSL: true},
		Auth:       auth,
	}
}

func oidcAuth(idp string, roles ...string) *model.AuthMiddleware {
	return &model.AuthMiddleware{IdentityProvider: idp, Mode: model.AuthModeOIDC, RequiredRoles: roles}
}

// serve builds the router from cfg and serves it on a TLS listener.
func (b *walkBench) serve(cfg model.Config) {
	b.t.Helper()
	if err := cfg.Validate(); err != nil {
		b.t.Fatalf("config does not validate: %v", err)
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		b.t.Fatalf("buildRouter: %v", err)
	}
	b.srv = httptest.NewUnstartedServer(http.HandlerFunc(rt.serveHTTPS))
	b.srv.StartTLS()
	b.t.Cleanup(b.srv.Close)
}

// --- the browser -----------------------------------------------------------

// walkHop is one redirect the browser followed: the status and Set-Cookie
// lines of the response that redirected, where it came from and went to.
type walkHop struct {
	Status    int
	From, To  *url.URL
	SetCookie []string
}

type walkResult struct {
	Status int
	URL    *url.URL
	Header http.Header
	Body   string
	Hops   []walkHop
}

// loginPage reports whether the result is the provider's login page.
func (r walkResult) loginPage() bool {
	return r.Status == http.StatusOK && r.Header.Get(opRequestHeader) != ""
}

// hopFrom returns the hop that left path, or nil.
func (r walkResult) hopFrom(path string) *walkHop {
	for i := range r.Hops {
		if r.Hops[i].From.Path == path {
			return &r.Hops[i]
		}
	}
	return nil
}

// visited reports whether the browser passed through path on the way.
func (r walkResult) visited(path string) bool {
	for _, h := range r.Hops {
		if h.To.Path == path {
			return true
		}
	}
	return false
}

type walkBrowser struct {
	t      *testing.T
	b      *walkBench
	jar    *cookiejar.Jar
	c      *http.Client
	hops   []walkHop
	stopAt string // a path the browser does not follow a redirect to
}

// browser is a fresh browser: an empty cookie jar, every *.example.com
// address (any port) dialled to the TLS listener, the listener's certificate
// trusted, and every redirect hop recorded.
func (b *walkBench) browser() *walkBrowser {
	b.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		b.t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(b.srv.Certificate())
	listener := b.srv.Listener.Addr().String()
	var d net.Dialer
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if h, _, err := net.SplitHostPort(addr); err == nil && (h == "example.com" || strings.HasSuffix(h, ".example.com")) {
				addr = listener
			}
			return d.DialContext(ctx, network, addr)
		},
		// The httptest certificate covers example.com, not its subdomains.
		TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "example.com"},
	}
	b.t.Cleanup(tr.CloseIdleConnections)
	br := &walkBrowser{t: b.t, b: b, jar: jar}
	br.c = &http.Client{Jar: jar, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		res := req.Response
		br.hops = append(br.hops, walkHop{Status: res.StatusCode, From: res.Request.URL, To: req.URL, SetCookie: res.Header.Values("Set-Cookie")})
		if br.stopAt != "" && req.URL.Path == br.stopAt {
			return http.ErrUseLastResponse
		}
		if len(via) >= 20 {
			return errors.New("stopped after 20 redirects")
		}
		return nil
	}}
	return br
}

func (br *walkBrowser) do(req *http.Request) walkResult {
	br.t.Helper()
	br.hops = nil
	res, err := br.c.Do(req)
	if err != nil {
		br.t.Fatalf("%s %s: %v (hops %v)", req.Method, req.URL, err, hopTrail(br.hops))
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return walkResult{Status: res.StatusCode, URL: res.Request.URL, Header: res.Header, Body: string(body), Hops: br.hops}
}

func (br *walkBrowser) get(raw string) walkResult {
	br.t.Helper()
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		br.t.Fatal(err)
	}
	return br.do(req)
}

// login submits the provider's login form shown by page.
func (br *walkBrowser) login(page walkResult, login string, deny bool) walkResult {
	br.t.Helper()
	form := url.Values{"request": {page.Header.Get(opRequestHeader)}, "login": {login}}
	if deny {
		form.Set("deny", "1")
	}
	req, err := http.NewRequest(http.MethodPost, br.b.op.Issuer+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		br.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := br.do(req)
	res.Hops = append(append([]walkHop(nil), page.Hops...), res.Hops...)
	return res
}

// signIn opens raw and, when that ends on the provider's login page, signs in
// as login.
func (br *walkBrowser) signIn(raw, login string) walkResult {
	br.t.Helper()
	res := br.get(raw)
	if res.loginPage() {
		return br.login(res, login, false)
	}
	return res
}

func (br *walkBrowser) cookie(raw, name string) *http.Cookie {
	u, _ := url.Parse(raw)
	for _, c := range br.jar.Cookies(u) {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (br *walkBrowser) setCookie(raw string, c *http.Cookie) {
	u, _ := url.Parse(raw)
	br.jar.SetCookies(u, []*http.Cookie{c})
}

func hopTrail(hops []walkHop) []string {
	var out []string
	for _, h := range hops {
		out = append(out, strconv.Itoa(h.Status)+" "+h.From.String()+" -> "+h.To.String())
	}
	return out
}

// mustServe asserts that res is the upstream's answer for exactly want.
func mustServe(t *testing.T, res walkResult, want string) {
	t.Helper()
	w, _ := url.Parse(want)
	if res.Status != http.StatusOK || res.URL.String() != want || res.Body != "UPSTREAM "+w.Host+" "+w.RequestURI() {
		t.Fatalf("want the upstream on %s, got %d on %s %q (hops %v)", want, res.Status, res.URL, res.Body, hopTrail(res.Hops))
	}
}

// mustRefuseOnCallback asserts that a sign-in ended in status on the callback,
// set no session and reached no upstream.
func mustRefuseOnCallback(t *testing.T, b *walkBench, br *walkBrowser, res walkResult, status int, upBefore int) {
	t.Helper()
	if res.Status != status || res.URL.Path != oidcCallbackPath {
		t.Fatalf("want %d on the callback, got %d on %s %q (hops %v)", status, res.Status, res.URL, res.Body, hopTrail(res.Hops))
	}
	for _, line := range res.Header.Values("Set-Cookie") {
		if strings.HasPrefix(line, oidcSessionCookie+"=") {
			t.Fatalf("a refused sign-in set a session cookie: %s", line)
		}
	}
	if c := br.cookie("https://"+res.URL.Host+"/", oidcSessionCookie); c != nil {
		t.Fatalf("the browser holds a session after a refused sign-in")
	}
	if n := b.up.count(); n != upBefore {
		t.Fatalf("a refused sign-in reached the upstream (%d requests, want %d)", n, upBefore)
	}
}

// parseSetCookie finds the Set-Cookie line for name.
func parseSetCookie(t *testing.T, lines []string, name string) *http.Cookie {
	t.Helper()
	for _, line := range lines {
		c, err := http.ParseSetCookie(line)
		if err == nil && c.Name == name {
			return c
		}
	}
	return nil
}

// waitNextSecond sleeps to the next whole second, at most one second.
func waitNextSecond() {
	time.Sleep(time.Until(time.Unix(time.Now().Unix()+1, 0)))
}

// --- the walk --------------------------------------------------------------

func TestOIDCWalk(t *testing.T) {
	t.Run("sign-in returns to the page asked for", func(t *testing.T) {
		b := newWalkBench(t)
		b.register(walkClient, "app.example.com")
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{b.idp(walkIdP, nil)},
			ProxyHosts:        []model.ProxyHost{b.host("app", oidcAuth(walkIdP), "app.example.com")},
		})
		br := b.browser()
		const page = "https://app.example.com/some/path?x=1&y=two"
		// The application's own cookie, which must still reach it.
		br.setCookie(page, &http.Cookie{Name: "app_pref", Value: "dark", Path: "/"})
		res := br.signIn(page, "staff1")
		mustServe(t, res, page)

		// What gpm asked the provider for.
		auths := b.op.authRequests()
		if len(auths) != 1 || auths[0].Error != "" {
			t.Fatalf("authorization requests: %+v", auths)
		}
		p := auths[0].Params
		for k, want := range map[string]string{
			"response_type":         "code",
			"client_id":             walkClient,
			"redirect_uri":          "https://app.example.com" + oidcCallbackPath,
			"scope":                 "openid profile email groups",
			"code_challenge_method": "S256",
		} {
			if got := p.Get(k); got != want {
				t.Errorf("authorization request %s = %q, want %q", k, got, want)
			}
		}
		for _, k := range []string{"state", "nonce", "code_challenge"} {
			if p.Get(k) == "" {
				t.Errorf("authorization request carries no %s", k)
			}
		}
		toks := b.op.tokenRequests()
		if len(toks) != 1 || toks[0].Error != "" || toks[0].AuthStyle != "basic" || toks[0].ClientID != walkClient || !toks[0].PKCEVerified {
			t.Fatalf("token requests: %+v, want one, client-authenticated, PKCE verified", toks)
		}

		// The callback's cookies.
		cb := res.hopFrom(oidcCallbackPath)
		if cb == nil || cb.Status != http.StatusFound {
			t.Fatalf("no redirect from the callback (hops %v)", hopTrail(res.Hops))
		}
		sc := parseSetCookie(t, cb.SetCookie, oidcSessionCookie)
		if sc == nil {
			t.Fatalf("the callback set no session cookie: %v", cb.SetCookie)
		}
		if !sc.Secure || !sc.HttpOnly || sc.SameSite != http.SameSiteLaxMode || sc.Path != "/" || sc.Domain != "" || sc.MaxAge != 3600 {
			t.Errorf("session cookie attributes: secure=%v httpOnly=%v sameSite=%v path=%q domain=%q maxAge=%d",
				sc.Secure, sc.HttpOnly, sc.SameSite, sc.Path, sc.Domain, sc.MaxAge)
		}
		if st := parseSetCookie(t, cb.SetCookie, oidcStateCookie); st == nil || st.MaxAge >= 0 {
			t.Errorf("the callback did not clear the state cookie: %v", cb.SetCookie)
		}
		if br.cookie(page, oidcStateCookie) != nil {
			t.Error("the browser still holds the state cookie after the callback")
		}

		// What the application received.
		seen := b.up.requests()
		if len(seen) != 1 || seen[0].URI != "/some/path?x=1&y=two" {
			t.Fatalf("upstream requests: %+v", seen)
		}
		h := seen[0].Header
		for k, want := range map[string]string{
			"X-Forwarded-User":   "staff1@example.com",
			"X-Forwarded-Email":  "staff1@example.com",
			"X-Forwarded-Groups": "staff,readers",
		} {
			if got := h.Get(k); got != want {
				t.Errorf("upstream %s = %q, want %q", k, got, want)
			}
		}
		cookies := strings.Join(h.Values("Cookie"), "; ")
		if !strings.Contains(cookies, "app_pref=dark") {
			t.Errorf("the application's own cookie did not reach it: %q", cookies)
		}
		if strings.Contains(cookies, "__Host-gpm_sso") {
			t.Errorf("a gate cookie reached the upstream: %q", cookies)
		}

		// The session serves the next request with no redirect at all.
		_, authorize, _, _ := b.op.counts()
		again := br.get(page)
		mustServe(t, again, page)
		if len(again.Hops) != 0 {
			t.Fatalf("a request with a session was redirected: %v", hopTrail(again.Hops))
		}
		if _, n, _, _ := b.op.counts(); n != authorize {
			t.Fatal("a request with a session contacted the provider")
		}
	})

	t.Run("one redirect URI per domain and never a port", func(t *testing.T) {
		b := newWalkBench(t)
		b.register(walkClient, "app.example.com") // alt.example.com is not registered yet
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{b.idp(walkIdP, nil)},
			ProxyHosts:        []model.ProxyHost{b.host("app", oidcAuth(walkIdP), "app.example.com", "alt.example.com")},
		})
		// A request on another port first, before any request has made gpm build
		// this domain's client: the redirect URI carries no port.
		page := b.browser().get("https://app.example.com:8443/")
		if !page.loginPage() {
			t.Fatalf("port 8443: want the login page, got %d on %s", page.Status, page.URL)
		}
		if got := b.op.lastAuth(t).Params.Get("redirect_uri"); got != "https://app.example.com"+oidcCallbackPath {
			t.Fatalf("port 8443: redirect_uri = %q, want it without the port", got)
		}

		br := b.browser()
		res := br.get("https://alt.example.com/")
		if res.Status != http.StatusBadRequest || res.URL.Path != "/authorize" || len(res.Hops) != 1 {
			t.Fatalf("unregistered domain: want the provider's 400, got %d on %s (hops %v)", res.Status, res.URL, hopTrail(res.Hops))
		}
		if got := res.URL.Query().Get("redirect_uri"); got != "https://alt.example.com"+oidcCallbackPath {
			t.Fatalf("redirect_uri for the second domain = %q", got)
		}
		if br.cookie("https://alt.example.com/", oidcSessionCookie) != nil || b.up.count() != 0 {
			t.Fatal("a sign-in the provider refused left a session or reached the upstream")
		}

		b.register(walkClient, "alt.example.com")
		mustServe(t, br.signIn("https://alt.example.com/", "staff1"), "https://alt.example.com/")
		// The first domain signs in on its own: it is sent through the provider
		// (silently, the provider session is alive) rather than admitted on the
		// second domain's session.
		res = br.get("https://app.example.com/")
		mustServe(t, res, "https://app.example.com/")
		if !res.visited("/authorize") || res.loginPage() {
			t.Fatalf("the first domain was admitted without its own sign-in (hops %v)", hopTrail(res.Hops))
		}
	})

	t.Run("roles are matched exactly and refused on the callback", func(t *testing.T) {
		b := newWalkBench(t)
		hosts := []struct {
			name  string
			roles []string
			pass  map[string]bool
		}{
			{"admin", []string{"admin"}, map[string]bool{"admin1": true}},
			{"user", []string{"user"}, map[string]bool{"staff1": true}},
			{"both", []string{"admin", "user"}, map[string]bool{"admin1": true, "staff1": true}},
			{"any", nil, map[string]bool{"admin1": true, "staff1": true}},
			// A group name is not a role: it admits no one.
			{"group", []string{"staff"}, map[string]bool{}},
		}
		cfg := model.Config{IdentityProviders: []model.IdentityProvider{b.idp(walkIdP, func(p *model.IdentityProvider) {
			p.RoleMapping = &model.RoleMapping{AdminGroups: []string{"admins"}, UserGroups: []string{"staff"}}
		})}}
		for _, h := range hosts {
			d := h.name + ".example.com"
			b.register(walkClient, d)
			cfg.ProxyHosts = append(cfg.ProxyHosts, b.host(h.name, oidcAuth(walkIdP, h.roles...), d))
		}
		var warned bool
		for _, w := range cfg.Warnings() {
			if strings.Contains(w, `proxy host "group"`) && strings.Contains(w, `"staff" is not a role`) {
				warned = true
			}
		}
		if !warned {
			t.Fatalf("no warning names the group in requiredRoles: %q", cfg.Warnings())
		}
		b.serve(cfg)
		for _, h := range hosts {
			for _, user := range []string{"admin1", "staff1", "nogroup1"} {
				br := b.browser()
				page := "https://" + h.name + ".example.com/"
				up := b.up.count()
				res := br.signIn(page, user)
				if h.pass[user] {
					mustServe(t, res, page)
					continue
				}
				mustRefuseOnCallback(t, b, br, res, http.StatusForbidden, up)
			}
		}
	})

	t.Run("scopes and the groups claim", func(t *testing.T) {
		b := newWalkBench(t)
		// A provider without a groups scope that ignores unknown scopes and
		// releases groups under profile.
		b.op.set(func(o *testOP) {
			o.scopes = []string{"openid", "profile", "email", "offline_access"}
			o.groupsScope = "profile"
		})
		b.register(walkClient, "app.example.com", "narrow.example.com")
		cfg := model.Config{
			IdentityProviders: []model.IdentityProvider{
				b.idp(walkIdP, func(p *model.IdentityProvider) {
					p.RoleMapping = &model.RoleMapping{UserGroups: []string{"staff"}}
				}),
				b.idp("narrow", func(p *model.IdentityProvider) { p.OIDC.Scopes = []string{"openid", "profile", "email"} }),
			},
			ProxyHosts: []model.ProxyHost{
				b.host("app", oidcAuth(walkIdP, "user"), "app.example.com"),
				b.host("narrow", oidcAuth("narrow"), "narrow.example.com"),
			},
		}
		b.serve(cfg)
		mustServe(t, b.browser().signIn("https://app.example.com/", "staff1"), "https://app.example.com/")
		if got := b.up.requests()[0].Header.Get("X-Forwarded-Groups"); got != "staff,readers" {
			t.Fatalf("groups released under profile: upstream X-Forwarded-Groups = %q", got)
		}

		// The same provider refusing unknown scopes: the default scopes end in
		// gpm's 401 without the provider's error text, the narrowed ones work.
		b.op.set(func(o *testOP) { o.refuseUnknownScopes = true })
		br := b.browser()
		up := b.up.count()
		res := br.get("https://app.example.com/")
		mustRefuseOnCallback(t, b, br, res, http.StatusUnauthorized, up)
		if !strings.Contains(res.Hops[len(res.Hops)-1].To.RawQuery, "error=invalid_scope") {
			t.Fatalf("the provider did not refuse the scope (hops %v)", hopTrail(res.Hops))
		}
		if strings.Contains(res.Body, "invalid_scope") || strings.Contains(res.Body, "not supported") {
			t.Fatalf("gpm echoed the provider's error: %q", res.Body)
		}
		mustServe(t, b.browser().signIn("https://narrow.example.com/", "staff1"), "https://narrow.example.com/")
	})

	t.Run("tampering never reaches the upstream", func(t *testing.T) {
		b := newWalkBench(t)
		b.register(walkClient, "app.example.com", "other.example.com")
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{b.idp(walkIdP, nil)},
			ProxyHosts: []model.ProxyHost{
				b.host("app", oidcAuth(walkIdP), "app.example.com"),
				b.host("other", oidcAuth(walkIdP), "other.example.com"),
			},
		})
		const app = "https://app.example.com/"

		// stopped runs a sign-in up to the provider's redirect to the callback
		// and returns the callback URL, the browser holding the state cookie.
		stopped := func() (*walkBrowser, string) {
			br := b.browser()
			br.stopAt = oidcCallbackPath
			res := br.signIn(app, "staff1")
			if res.Status != http.StatusFound || len(res.Hops) == 0 || res.Hops[len(res.Hops)-1].To.Path != oidcCallbackPath {
				t.Fatalf("did not stop at the callback: %d on %s (hops %v)", res.Status, res.URL, hopTrail(res.Hops))
			}
			br.stopAt = ""
			return br, res.Hops[len(res.Hops)-1].To.String()
		}

		t.Run("a callback with no state cookie", func(t *testing.T) {
			br, cb := stopped()
			br.setCookie(app, &http.Cookie{Name: oidcStateCookie, Path: "/", MaxAge: -1})
			mustRefuseOnCallback(t, b, br, br.get(cb), http.StatusBadRequest, b.up.count())
		})
		t.Run("a callback whose state does not match", func(t *testing.T) {
			br, cb := stopped()
			u, _ := url.Parse(cb)
			q := u.Query()
			q.Set("state", q.Get("state")+"x")
			u.RawQuery = q.Encode()
			mustRefuseOnCallback(t, b, br, br.get(u.String()), http.StatusBadRequest, b.up.count())
		})
		t.Run("a replayed callback", func(t *testing.T) {
			br, cb := stopped()
			state := br.cookie(app, oidcStateCookie)
			mustServe(t, br.get(cb), app)
			// The same browser again: its state cookie is spent.
			up := b.up.count()
			res := br.get(cb)
			if res.Status != http.StatusBadRequest || res.URL.Path != oidcCallbackPath || b.up.count() != up {
				t.Fatalf("replay in the same browser: got %d on %s", res.Status, res.URL)
			}
			// A fresh browser that also replays the captured state cookie gets past
			// the state check, and the provider refuses the spent code.
			thief := b.browser()
			thief.setCookie(app, &http.Cookie{Name: oidcStateCookie, Value: state.Value, Path: "/", Secure: true})
			mustRefuseOnCallback(t, b, thief, thief.get(cb), http.StatusUnauthorized, up)
			if last := b.op.lastToken(t); last.Error != "invalid_grant" {
				t.Fatalf("the replayed code was not refused by the provider: %+v", last)
			}
		})
		t.Run("an altered session cookie", func(t *testing.T) {
			br := b.browser()
			mustServe(t, br.signIn(app, "staff1"), app)
			sess := br.cookie(app, oidcSessionCookie)
			payload, sig, _ := strings.Cut(sess.Value, ".")
			raw, err := base64.RawURLEncoding.DecodeString(payload)
			if err != nil {
				t.Fatal(err)
			}
			var s map[string]any
			if err := json.Unmarshal(raw, &s); err != nil {
				t.Fatal(err)
			}
			s["groups"] = []string{"admins"}
			s["email"] = "admin1@example.com"
			raw, _ = json.Marshal(s)
			br.setCookie(app, &http.Cookie{Name: oidcSessionCookie, Value: base64.RawURLEncoding.EncodeToString(raw) + "." + sig, Path: "/", Secure: true})
			br.stopAt = "/authorize"
			up := b.up.count()
			res := br.get(app)
			if res.Status != http.StatusFound || len(res.Hops) != 1 || res.Hops[0].To.Path != "/authorize" || b.up.count() != up {
				t.Fatalf("altered session: got %d on %s (hops %v), want a redirect into sign-in", res.Status, res.URL, hopTrail(res.Hops))
			}
		})
		t.Run("a session of one host presented to another", func(t *testing.T) {
			br := b.browser()
			mustServe(t, br.signIn(app, "staff1"), app)
			sess := br.cookie(app, oidcSessionCookie)
			br.setCookie("https://other.example.com/", &http.Cookie{Name: oidcSessionCookie, Value: sess.Value, Path: "/", Secure: true})
			br.stopAt = "/authorize"
			up := b.up.count()
			res := br.get("https://other.example.com/")
			if res.Status != http.StatusFound || len(res.Hops) != 1 || res.Hops[0].To.Path != "/authorize" || b.up.count() != up {
				t.Fatalf("cross-host session: got %d on %s (hops %v), want a redirect into sign-in", res.Status, res.URL, hopTrail(res.Hops))
			}
		})
		// intoSignIn asserts that a request for app with the browser's cookies is
		// sent back into sign-in and reaches no upstream.
		intoSignIn := func(t *testing.T, br *walkBrowser, what string) {
			t.Helper()
			br.stopAt = "/authorize"
			up := b.up.count()
			res := br.get(app)
			if res.Status != http.StatusFound || len(res.Hops) != 1 || res.Hops[0].To.Path != "/authorize" || b.up.count() != up {
				t.Fatalf("%s: got %d on %s (hops %v), want a redirect into sign-in", what, res.Status, res.URL, hopTrail(res.Hops))
			}
		}
		t.Run("a session past its own expiry", func(t *testing.T) {
			// The cookie's Max-Age is the browser's to honour; the expiry inside
			// the signed payload is gpm's. A validly signed session whose payload
			// has expired must not be served, however long a client keeps it.
			br := b.browser()
			mustServe(t, br.signIn(app, "staff1"), app)
			payload, ok := verifyToken(macLabelSSOSession, br.cookie(app, oidcSessionCookie).Value)
			if !ok {
				t.Fatal("the session cookie does not verify")
			}
			var sess oidcSession
			if err := json.Unmarshal(payload, &sess); err != nil {
				t.Fatal(err)
			}
			sess.Exp = time.Now().Add(-time.Minute).Unix()
			raw, _ := json.Marshal(sess)
			br.setCookie(app, &http.Cookie{Name: oidcSessionCookie, Value: signToken(macLabelSSOSession, raw), Path: "/", Secure: true})
			intoSignIn(t, br, "expired session")
		})
		t.Run("a sign-in state past its own expiry", func(t *testing.T) {
			// Same for the ten-minute sign-in state: a validly signed state whose
			// payload has expired does not complete a sign-in, even with the right
			// state parameter and an unspent code.
			br, cb := stopped()
			payload, ok := verifyToken(macLabelSSOState, br.cookie(app, oidcStateCookie).Value)
			if !ok {
				t.Fatal("the state cookie does not verify")
			}
			var st oidcLoginState
			if err := json.Unmarshal(payload, &st); err != nil {
				t.Fatal(err)
			}
			st.Exp = time.Now().Add(-time.Minute).Unix()
			raw, _ := json.Marshal(st)
			br.setCookie(app, &http.Cookie{Name: oidcStateCookie, Value: signToken(macLabelSSOState, raw), Path: "/", Secure: true})
			mustRefuseOnCallback(t, b, br, br.get(cb), http.StatusBadRequest, b.up.count())
		})
		t.Run("a session of one provider is not accepted by a gate of another", func(t *testing.T) {
			// One host, two providers (a shape gpm warns about, since both gates
			// share the cookie): the host gate uses "one", the /two location's
			// gate uses "two". A session minted by "one" names its provider, and
			// the gate for "two" must send the browser into its own sign-in.
			b := newWalkBench(t)
			b.register(walkClient, "app.example.com")
			h := b.host("app", oidcAuth("one"), "app.example.com")
			up := b.up.target(t)
			h.Locations = []model.Location{{Path: "/two", Upstream: &up, Auth: oidcAuth("two")}}
			b.serve(model.Config{
				IdentityProviders: []model.IdentityProvider{b.idp("one", nil), b.idp("two", nil)},
				ProxyHosts:        []model.ProxyHost{h},
			})
			br := b.browser()
			mustServe(t, br.signIn(app, "staff1"), app)
			br.stopAt = "/authorize"
			before := b.up.count()
			res := br.get(app + "two/x")
			if res.Status != http.StatusFound || len(res.Hops) != 1 || res.Hops[0].To.Path != "/authorize" || b.up.count() != before {
				t.Fatalf("got %d on %s (hops %v), want the second provider's sign-in", res.Status, res.URL, hopTrail(res.Hops))
			}
		})
		t.Run("a provider error with markup is not echoed", func(t *testing.T) {
			b.op.set(func(o *testOP) { o.denyDescription = `<script>alert("x")</script> denied by policy` })
			br := b.browser()
			page := br.get(app)
			if !page.loginPage() {
				t.Fatalf("want the login page, got %d", page.Status)
			}
			up := b.up.count()
			res := br.login(page, "staff1", true)
			mustRefuseOnCallback(t, b, br, res, http.StatusUnauthorized, up)
			if strings.Contains(res.Body, "<script") || strings.Contains(res.Body, "denied by policy") || strings.Contains(res.Body, "access_denied") {
				t.Fatalf("gpm echoed the provider's error: %q", res.Body)
			}
		})
	})

	t.Run("ID token checks", func(t *testing.T) {
		b := newWalkBench(t)
		b.register(walkClient, "app.example.com")
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{b.idp(walkIdP, nil)},
			ProxyHosts:        []model.ProxyHost{b.host("app", oidcAuth(walkIdP), "app.example.com")},
		})
		for name, hook := range map[string]func(*opIDToken){
			"wrong nonce":           func(tok *opIDToken) { tok.Claims["nonce"] = "not-the-nonce-gpm-sent" },
			"wrong audience":        func(tok *opIDToken) { tok.Claims["aud"] = "another-client" },
			"wrong issuer":          func(tok *opIDToken) { tok.Claims["iss"] = "https://issuer.example.com" },
			"expired":               func(tok *opIDToken) { tok.Claims["exp"] = time.Now().Add(-time.Hour).Unix() },
			"signed by another key": func(tok *opIDToken) { tok.Key = opForeignKey() },
			"no id_token":           func(tok *opIDToken) { tok.Omit = true },
		} {
			t.Run(name, func(t *testing.T) {
				b.op.set(func(o *testOP) { o.idTokenHook = hook })
				br := b.browser()
				up := b.up.count()
				mustRefuseOnCallback(t, b, br, br.signIn("https://app.example.com/", "staff1"), http.StatusUnauthorized, up)
				if last := b.op.lastToken(t); last.Error != "" {
					t.Fatalf("the provider refused the token request (%+v): the test would not reach the ID token check", last)
				}
			})
		}
		// The same bench signs in once the hook is gone, so each refusal above is
		// the hook's doing.
		b.op.set(func(o *testOP) { o.idTokenHook = nil })
		mustServe(t, b.browser().signIn("https://app.example.com/", "staff1"), "https://app.example.com/")
	})

	t.Run("client authentication and PKCE", func(t *testing.T) {
		b := newWalkBench(t)
		off := false
		b.register(walkClient, "badsecret.example.com", "nopkce.example.com")
		b.register(walkNoPKCEClient, "plain.example.com")
		b.register(walkPublicClient, "public.example.com")
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{
				b.idp("badsecret", func(p *model.IdentityProvider) { p.OIDC.ClientSecret = "not-the-secret" }),
				b.idp("nopkce", func(p *model.IdentityProvider) { p.OIDC.UsePKCE = &off }),
				b.idp("plain", func(p *model.IdentityProvider) { p.OIDC.ClientID = walkNoPKCEClient; p.OIDC.UsePKCE = &off }),
				b.idp("public", func(p *model.IdentityProvider) { p.OIDC.ClientID = walkPublicClient; p.OIDC.ClientSecret = "" }),
			},
			ProxyHosts: []model.ProxyHost{
				b.host("badsecret", oidcAuth("badsecret"), "badsecret.example.com"),
				b.host("nopkce", oidcAuth("nopkce"), "nopkce.example.com"),
				b.host("plain", oidcAuth("plain"), "plain.example.com"),
				b.host("public", oidcAuth("public"), "public.example.com"),
			},
		})

		br := b.browser()
		mustRefuseOnCallback(t, b, br, br.signIn("https://badsecret.example.com/", "staff1"), http.StatusUnauthorized, 0)
		for _, tr := range b.op.tokenRequests() {
			if tr.Error != "invalid_client" {
				t.Fatalf("wrong secret: token request %+v, want invalid_client", tr)
			}
		}

		br = b.browser()
		res := br.get("https://nopkce.example.com/")
		mustRefuseOnCallback(t, b, br, res, http.StatusUnauthorized, 0)
		if last := b.op.lastAuth(t); last.Error != "invalid_request" || last.Params.Get("code_challenge") != "" {
			t.Fatalf("usePKCE false against a PKCE client: authorization request %+v", last)
		}

		ntok := len(b.op.tokenRequests())
		mustServe(t, b.browser().signIn("https://plain.example.com/", "staff1"), "https://plain.example.com/")
		auth := b.op.lastAuth(t)
		toks := b.op.tokenRequests()
		if auth.Params.Get("code_challenge") != "" || len(toks) != ntok+1 || toks[ntok].PKCEVerified || toks[ntok].AuthStyle != "basic" {
			t.Fatalf("usePKCE false: authorize %+v, token %+v", auth.Params, toks[ntok:])
		}

		mustServe(t, b.browser().signIn("https://public.example.com/", "staff1"), "https://public.example.com/")
		if last := b.op.lastToken(t); last.ClientID != walkPublicClient || last.AuthStyle != "none" || !last.PKCEVerified {
			t.Fatalf("public client: token request %+v, want no secret and PKCE verified", last)
		}
	})

	t.Run("requireVerifiedEmail", func(t *testing.T) {
		b := newWalkBench(t)
		b.register(walkClient, "strict.example.com", "lenient.example.com")
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{
				b.idp("strict", func(p *model.IdentityProvider) { p.OIDC.RequireVerifiedEmail = true }),
				b.idp("lenient", nil),
			},
			ProxyHosts: []model.ProxyHost{
				b.host("strict", oidcAuth("strict"), "strict.example.com"),
				b.host("lenient", oidcAuth("lenient"), "lenient.example.com"),
			},
		})
		br := b.browser()
		res := br.signIn("https://strict.example.com/", "unverified1")
		mustRefuseOnCallback(t, b, br, res, http.StatusUnauthorized, 0)
		const msg = "sign-in refused: the identity provider has not verified the email address of this account"
		if !strings.Contains(res.Body, msg) {
			t.Fatalf("unverified email: body %q, want the documented message", res.Body)
		}
		mustServe(t, b.browser().signIn("https://strict.example.com/", "staff1"), "https://strict.example.com/")
		mustServe(t, b.browser().signIn("https://lenient.example.com/", "unverified1"), "https://lenient.example.com/")
	})

	t.Run("how a session ends", func(t *testing.T) {
		b := newWalkBench(t)
		b.register(walkClient, "app.example.com")
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{b.idp(walkIdP, nil)},
			ProxyHosts:        []model.ProxyHost{b.host("app", oidcAuth(walkIdP), "app.example.com")},
		})
		const app = "https://app.example.com/"
		live, gone := b.browser(), b.browser()
		mustServe(t, live.signIn(app, "staff1"), app)
		mustServe(t, gone.signIn(app, "staff1"), app)

		// (a) Signing out at the provider does not end the gpm session.
		gone.get(b.op.Issuer + "/logout")
		_, authorize, token, _ := b.op.counts()
		res := gone.get(app + "after-logout")
		mustServe(t, res, app+"after-logout")
		if _, a, tk, _ := b.op.counts(); len(res.Hops) != 0 || a != authorize || tk != token {
			t.Fatalf("after a provider logout the request went to the provider (hops %v)", hopTrail(res.Hops))
		}

		// (b) Revoking all sessions does. A session issued in the same second as
		// the revocation survives it, so the revocation waits for the next
		// second once.
		waitNextSecond()
		if err := RevokeAllSSOSessions(); err != nil {
			t.Fatal(err)
		}
		_, _, _, logins := b.op.counts()
		res = live.get(app + "after-revoke")
		mustServe(t, res, app+"after-revoke")
		if !res.visited("/authorize") || !res.visited(oidcCallbackPath) {
			t.Fatalf("a revoked session was still admitted (hops %v)", hopTrail(res.Hops))
		}
		if _, _, _, l := b.op.counts(); l != logins {
			t.Fatal("with a live provider session the re-authentication showed the login page")
		}
		up := b.up.count()
		page := gone.get(app + "after-revoke")
		if !page.loginPage() || b.up.count() != up {
			t.Fatalf("revoked session without a provider session: want the login page, got %d on %s", page.Status, page.URL)
		}
		mustServe(t, gone.login(page, "staff1", false), app+"after-revoke")

		// (c) There is no sign-out path: /__gpm/oidc/logout is an ordinary path
		// that reaches the upstream with the session. This pins the documented
		// behaviour ("No sign-out URL" in docs/how-to/per-host-sso.md) and must
		// change with it.
		const logout = "https://app.example.com/__gpm/oidc/logout"
		mustServe(t, live.get(logout), logout)
		mustServe(t, live.get(app), app)
	})

	t.Run("where the gate sits", func(t *testing.T) {
		b := newWalkBench(t)
		domains := []string{"inline.example.com", "mw.example.com", "loc.example.com", "stack.example.com"}
		b.register(walkClient, domains...)
		mw := b.host("mw", nil, "mw.example.com")
		mw.Middlewares = []string{"sso"}
		loc := b.host("loc", nil, "loc.example.com")
		loc.Locations = []model.Location{{Path: "/admin", Auth: oidcAuth(walkIdP)}}
		stack := b.host("stack", oidcAuth("roles"), "stack.example.com")
		stack.Locations = []model.Location{{Path: "/admin", Auth: oidcAuth("roles", "admin")}}
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{
				b.idp(walkIdP, nil),
				b.idp("roles", func(p *model.IdentityProvider) {
					p.RoleMapping = &model.RoleMapping{AdminGroups: []string{"admins"}, UserGroups: []string{"staff"}}
				}),
			},
			Middlewares: []model.Middleware{{ObjectMeta: model.ObjectMeta{Name: "sso"}, Type: model.MWTypeAuth, Auth: oidcAuth(walkIdP)}},
			ProxyHosts:  []model.ProxyHost{b.host("inline", oidcAuth(walkIdP), "inline.example.com"), mw, loc, stack},
		})

		for _, d := range []string{"inline.example.com", "mw.example.com"} {
			page := "https://" + d + "/x"
			res := b.browser().signIn(page, "staff1")
			mustServe(t, res, page)
			if !res.visited(oidcCallbackPath) {
				t.Fatalf("%s: served without a sign-in (hops %v)", d, hopTrail(res.Hops))
			}
		}

		// A gate on a location only: the open part stays open, the sign-in
		// started under /admin completes there, and the upstream never sees the
		// callback.
		br := b.browser()
		open := br.get("https://loc.example.com/")
		mustServe(t, open, "https://loc.example.com/")
		if len(open.Hops) != 0 {
			t.Fatalf("the open part redirected: %v", hopTrail(open.Hops))
		}
		res := br.signIn("https://loc.example.com/admin/x", "staff1")
		mustServe(t, res, "https://loc.example.com/admin/x")
		if !res.visited(oidcCallbackPath) {
			t.Fatalf("/admin was served without a sign-in (hops %v)", hopTrail(res.Hops))
		}
		mustServe(t, br.get("https://loc.example.com/public"), "https://loc.example.com/public")

		// A host gate plus a location that adds requiredRoles for the same
		// provider: a user signs in on the host, is refused under /admin on the
		// same session, and an admin passes both.
		staff := b.browser()
		mustServe(t, staff.signIn("https://stack.example.com/", "staff1"), "https://stack.example.com/")
		up := b.up.count()
		res = staff.get("https://stack.example.com/admin/x")
		if res.Status != http.StatusForbidden || len(res.Hops) != 0 || b.up.count() != up {
			t.Fatalf("a user under the admin-only location: got %d on %s (hops %v), want 403 with no redirect", res.Status, res.URL, hopTrail(res.Hops))
		}
		admin := b.browser()
		mustServe(t, admin.signIn("https://stack.example.com/admin/x", "admin1"), "https://stack.example.com/admin/x")
		mustServe(t, admin.get("https://stack.example.com/"), "https://stack.example.com/")

		for _, s := range b.up.requests() {
			if strings.HasPrefix(s.URI, oidcCallbackPath) {
				t.Fatalf("the upstream saw the callback: %s %s", s.Host, s.URI)
			}
		}
	})

	t.Run("provider outage", func(t *testing.T) {
		b := newWalkBench(t)
		b.register(walkClient, "app.example.com")
		b.op.set(func(o *testOP) { o.down = true })
		b.serve(model.Config{
			IdentityProviders: []model.IdentityProvider{b.idp(walkIdP, nil)},
			ProxyHosts:        []model.ProxyHost{b.host("app", oidcAuth(walkIdP), "app.example.com")},
		})
		const app = "https://app.example.com/"
		br := b.browser()
		for i := 0; i < 2; i++ {
			res := br.get(app)
			if res.Status != http.StatusBadGateway || len(res.Hops) != 0 || b.up.count() != 0 {
				t.Fatalf("provider down: got %d on %s (hops %v), want 502 and nothing upstream", res.Status, res.URL, hopTrail(res.Hops))
			}
		}
		if d, _, _, _ := b.op.counts(); d != 2 {
			t.Fatalf("discoveries while down = %d, want one per request (a failure is not cached)", d)
		}
		b.op.set(func(o *testOP) { o.down = false })
		mustServe(t, br.signIn(app, "staff1"), app)
	})
}
