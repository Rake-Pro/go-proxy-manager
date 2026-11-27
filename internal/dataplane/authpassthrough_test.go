package dataplane

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

const ptDomain = "app.example.com"

// ptBackend answers "BACKEND"; a pass-through request must never reach it.
func ptBackend() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("BACKEND")) })
}

// ptConfig returns the identity providers and middlewares the pass-through
// tests combine: "ar" (auth-request against outpostURL), "oidc" (an OIDC gate
// against issuer), "fa" (forward-auth trusting 10.0.0.0/8), "basic" (user
// u/p, LAN exempt) and "cert" (client-cert, LAN exempt).
func ptConfig(t *testing.T, outpostURL, issuer string) model.Config {
	t.Helper()
	basic := basicMW(t, "u", "p", "inner", "192.0.2.0/24")
	return model.Config{
		IdentityProviders: []model.IdentityProvider{
			{ObjectMeta: model.ObjectMeta{Name: "ar"}, Type: model.IdPTypeAuthRequest, AuthRequest: &model.AuthRequestSpec{OutpostURL: outpostURL}},
			{ObjectMeta: model.ObjectMeta{Name: "oidc"}, Type: model.IdPTypeOIDC, OIDC: &model.OIDCSpec{IssuerURL: issuer, ClientID: "gpm-client"}},
			{ObjectMeta: model.ObjectMeta{Name: "fa"}, Type: model.IdPTypeForwardAuth, ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"}, RoleMapping: &model.RoleMapping{DefaultRole: "user"}},
		},
		Middlewares: []model.Middleware{
			{ObjectMeta: model.ObjectMeta{Name: "ar"}, Type: model.MWTypeAuth, Auth: &model.AuthMiddleware{IdentityProvider: "ar", Mode: model.AuthModeAuthRequest}},
			{ObjectMeta: model.ObjectMeta{Name: "oidc"}, Type: model.MWTypeAuth, Auth: &model.AuthMiddleware{IdentityProvider: "oidc", Mode: model.AuthModeOIDC}},
			{ObjectMeta: model.ObjectMeta{Name: "fa"}, Type: model.MWTypeAuth, Auth: &model.AuthMiddleware{IdentityProvider: "fa", Mode: model.AuthModeForwardAuth}},
			{ObjectMeta: model.ObjectMeta{Name: "basic"}, Type: model.MWTypeAuth, Auth: &basic},
			{ObjectMeta: model.ObjectMeta{Name: "cert"}, Type: model.MWTypeAuth, Auth: &model.AuthMiddleware{Mode: model.AuthModeClientCert, AllowFrom: []string{"192.0.2.0/24"}}},
		},
	}
}

func ptHost(mws ...string) model.ProxyHost {
	return model.ProxyHost{
		ObjectMeta:  model.ObjectMeta{Name: "app"},
		Domains:     []string{ptDomain},
		Middlewares: mws,
		TLS:         model.TLSSettings{ClientAuth: &model.ClientAuth{CARef: "corp-ca", Mode: "optional"}},
	}
}

// ptRequest builds one request; cred selects what the client brings.
type ptCred int

const (
	credNone ptCred = iota
	credBasic
	credCert
	credForward
	credLAN
)

func ptRequest(t *testing.T, path string, cred ptCred, leaf, ca *x509.Certificate) *http.Request {
	t.Helper()
	req := httptest.NewRequest("GET", "https://"+ptDomain+path, nil)
	req.RemoteAddr = "203.0.113.5:4000"
	switch cred {
	case credBasic:
		req.SetBasicAuth("u", "p")
	case credCert:
		req.TLS = &tls.ConnectionState{ServerName: ptDomain, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, ca}}}
	case credForward:
		req.RemoteAddr = "10.0.0.5:4000"
		req.Header.Set("X-User", "alice")
	case credLAN:
		req.RemoteAddr = "192.0.2.7:4000"
	}
	return req
}

// TestPassThroughMeetsInnerNonInteractiveGates: when an interactive gate runs
// first, a request on its pass-through path still has to pass the
// non-interactive gate inside it, with that gate's full rule (allowFrom too),
// and is then served by the interactive gate, never by the backend.
func TestPassThroughMeetsInnerNonInteractiveGates(t *testing.T) {
	stub := &outpostStub{authStatus: http.StatusUnauthorized}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()
	_, ca, caKey := crlCA(t, "corp-ca")
	_, leaf := clientCertSerial(t, ca, caKey, 300, "ops")
	cfg := ptConfig(t, srv.URL, newMockIdP(t).server.URL)

	outpostPath := "/outpost.goauthentik.io/start?rd=/"
	callbackPath := oidcCallbackPath + "?code=c&state=s"
	served := map[string]func(*httptest.ResponseRecorder) bool{
		"ar":   func(rec *httptest.ResponseRecorder) bool { return strings.HasPrefix(rec.Body.String(), "OUTPOST:") },
		"oidc": func(rec *httptest.ResponseRecorder) bool { return rec.Code == http.StatusBadRequest }, // the callback's "invalid login state"
	}
	paths := map[string]string{"ar": outpostPath, "oidc": callbackPath}

	type step struct {
		cred   ptCred
		served bool // true: the interactive gate served it; false: the inner gate refused 401
	}
	inner := []struct {
		name  string
		steps []step
	}{
		{"basic", []step{{credNone, false}, {credBasic, true}, {credLAN, true}}},
		{"cert", []step{{credNone, false}, {credCert, true}, {credLAN, true}}},
		{"fa", []step{{credNone, false}, {credForward, true}}},
	}
	for _, outer := range []string{"ar", "oidc"} {
		for _, in := range inner {
			t.Run(outer+" before "+in.name, func(t *testing.T) {
				// The last-referenced gate runs first.
				h := buildChain(ptBackend(), ptHost(in.name, outer), buildRegistry(cfg), nil)
				for _, s := range in.steps {
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, ptRequest(t, paths[outer], s.cred, leaf, ca))
					if rec.Body.String() == "BACKEND" {
						t.Fatalf("cred %d: a pass-through request reached the backend", s.cred)
					}
					if got := served[outer](rec); got != s.served {
						t.Fatalf("cred %d: served by %s = %v, want %v (status %d, body %q)", s.cred, outer, got, s.served, rec.Code, rec.Body.String())
					}
					if !s.served && rec.Code != http.StatusUnauthorized {
						t.Fatalf("cred %d: inner %s gate must refuse with 401, got %d", s.cred, in.name, rec.Code)
					}
				}
			})
		}
	}
}

// TestPassThroughOrderAndInteractiveOnlyUnchanged covers the setups that must
// behave exactly as before: a single interactive gate, two interactive gates in
// either order, and a non-interactive gate that already runs before the
// interactive one.
func TestPassThroughOrderAndInteractiveOnlyUnchanged(t *testing.T) {
	stub := &outpostStub{authStatus: http.StatusUnauthorized}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()
	cfg := ptConfig(t, srv.URL, newMockIdP(t).server.URL)
	outpostPath := "/outpost.goauthentik.io/start?rd=/"
	callbackPath := oidcCallbackPath + "?code=c&state=s"

	cases := []struct {
		name     string
		mws      []string
		path     string
		cred     ptCred
		wantCode int
		wantBody string // prefix
	}{
		{"single auth-request, outpost path", []string{"ar"}, outpostPath, credNone, http.StatusOK, "OUTPOST:"},
		{"single oidc, callback", []string{"oidc"}, callbackPath, credNone, http.StatusBadRequest, ""},
		{"oidc first, auth-request inside: outpost path is gated by oidc", []string{"ar", "oidc"}, outpostPath, credNone, http.StatusFound, ""},
		{"oidc first, auth-request inside: callback skips auth-request", []string{"ar", "oidc"}, callbackPath, credNone, http.StatusBadRequest, ""},
		{"auth-request first, oidc inside: outpost path skips oidc", []string{"oidc", "ar"}, outpostPath, credNone, http.StatusOK, "OUTPOST:"},
		{"auth-request first, oidc inside: callback is gated by auth-request", []string{"oidc", "ar"}, callbackPath, credNone, http.StatusFound, ""},
		{"two auth-request gates of one provider", []string{"ar", "ar"}, outpostPath, credNone, http.StatusOK, "OUTPOST:"},
		{"basic first, auth-request inside: outpost path, no credentials", []string{"ar", "basic"}, outpostPath, credNone, http.StatusUnauthorized, ""},
		{"basic first, auth-request inside: outpost path, credentials", []string{"ar", "basic"}, outpostPath, credBasic, http.StatusOK, "OUTPOST:"},
		{"basic first, oidc inside: callback, no credentials", []string{"oidc", "basic"}, callbackPath, credNone, http.StatusUnauthorized, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := buildChain(ptBackend(), ptHost(tc.mws...), buildRegistry(cfg), nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, ptRequest(t, tc.path, tc.cred, nil, nil))
			if rec.Code != tc.wantCode || !strings.HasPrefix(rec.Body.String(), tc.wantBody) {
				t.Fatalf("got %d %q, want %d %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantBody)
			}
		})
	}
}

// sessionOutpost is an auth-request outpost with a session: its /start sets a
// cookie and returns to rd, and its verdict is 200 once the cookie is present.
func sessionOutpost(t *testing.T, prefix, cookie string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case prefix + "/auth/nginx":
			if _, err := r.Cookie(cookie); err == nil {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		case prefix + "/start":
			http.SetCookie(w, &http.Cookie{Name: cookie, Value: "1", Path: "/"})
			http.Redirect(w, r, r.URL.Query().Get("rd"), http.StatusFound)
		default:
			_, _ = w.Write([]byte("OUTPOST:" + r.URL.Path))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// signIn drives a browser through h from "/" until the backend answers,
// keeping cookies and playing the IdP's authorize step for an OIDC gate.
func signIn(t *testing.T, h http.Handler, idp *mockIdP) {
	t.Helper()
	signInAt(t, h, idp, "/")
}

// signInAt is signIn starting from path. It returns the path the backend
// finally answered, so a caller can tell the page it asked for from a
// pass-through path that leaked to the backend.
func signInAt(t *testing.T, h http.Handler, idp *mockIdP, path string) string {
	t.Helper()
	jar := map[string]*http.Cookie{}
	target := "https://" + ptDomain + path
	var trail []string
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest("GET", target, nil)
		req.RemoteAddr = "203.0.113.5:4000"
		for _, c := range jar {
			req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.MaxAge < 0 {
				delete(jar, c.Name)
			} else {
				jar[c.Name] = c
			}
		}
		u, _ := url.Parse(target)
		trail = append(trail, u.Path)
		if rec.Code == http.StatusOK && rec.Body.String() == "BACKEND" {
			return u.Path
		}
		if rec.Code != http.StatusFound {
			t.Fatalf("sign-in stopped at %s with %d %q (trail %v)", u.Path, rec.Code, rec.Body.String(), trail)
		}
		loc := rec.Header().Get("Location")
		if idp != nil && strings.HasPrefix(loc, idp.server.URL) {
			q, _ := url.Parse(loc)
			idp.claims = map[string]any{"nonce": q.Query().Get("nonce"), "email": "user@example.com"}
			target = "https://" + ptDomain + oidcCallbackPath + "?code=abc&state=" + url.QueryEscape(q.Query().Get("state"))
			continue
		}
		if strings.HasPrefix(loc, "/") {
			loc = "https://" + ptDomain + loc
		}
		target = loc
	}
	t.Fatalf("sign-in did not complete in 20 requests (trail %v)", trail)
	return ""
}

// routed serves each request through the handler the host's route() picks for
// its path, as the router does.
func routed(hh *hostHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, _ := hh.route(r.URL.Path)
		h.ServeHTTP(w, r)
	})
}

// ptLocationHost is a host with no gate of its own and one location, /admin,
// that carries auth (inline) or mws (referenced). Host and location each get
// a live backend of their own, so route()'s upstream label (returned as
// hostUp and locUp) tells the two routes apart.
func ptLocationHost(t *testing.T, auth *model.AuthMiddleware, mws ...string) (host model.ProxyHost, hostUp, locUp string) {
	t.Helper()
	backend := func() (model.Upstream, string) {
		srv := httptest.NewServer(ptBackend())
		t.Cleanup(srv.Close)
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			t.Fatal(err)
		}
		return model.Upstream{Scheme: "http", Host: u.Hostname(), Port: port}, srv.URL
	}
	hostBackend, hostUp := backend()
	locBackend, locUp := backend()
	return model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{ptDomain},
		Upstream:   hostBackend,
		Locations:  []model.Location{{Path: "/admin", Auth: auth, Middlewares: mws, Upstream: &locBackend}},
	}, hostUp, locUp
}

// ptServe sends one GET for path from remote through route().
func ptServe(hh *hostHandler, path, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "https://"+ptDomain+path, nil)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	routed(hh).ServeHTTP(rec, req)
	return rec
}

const (
	ptWAN        = "203.0.113.5:4000"
	ptLAN        = "192.0.2.7:4000"
	ptOutpostURL = "/outpost.goauthentik.io/x"
	ptCallback   = oidcCallbackPath + "?code=c&state=s"
)

// ptGateServed reports whether rec is the answer of the gate that owns path:
// the outpost's passthrough, or the callback's "invalid login state".
func ptGateServed(rec *httptest.ResponseRecorder, path string) bool {
	if strings.HasPrefix(path, oidcCallbackPath) {
		return rec.Code == http.StatusBadRequest && rec.Body.String() != "BACKEND"
	}
	return rec.Code == http.StatusOK && strings.HasPrefix(rec.Body.String(), "OUTPOST:")
}

func ptBuild(t *testing.T, cfg model.Config, host model.ProxyHost) *hostHandler {
	t.Helper()
	cfg.ProxyHosts = []model.ProxyHost{host}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	return rt.hosts[ptDomain]
}

var (
	ptOIDC = &model.AuthMiddleware{IdentityProvider: "oidc", Mode: model.AuthModeOIDC}
	ptAR   = &model.AuthMiddleware{IdentityProvider: "ar", Mode: model.AuthModeAuthRequest}
)

// TestLocationOnlyGateFinishesSignIn: an interactive gate that only a location
// carries starts a sign-in that comes back on the gate's own path, outside the
// location. route() hands that path to the host's pass-through route, where
// the gate serves it; on the host route it reached the upstream and the
// sign-in could never complete. Every other path routes as before.
func TestLocationOnlyGateFinishesSignIn(t *testing.T) {
	ar := sessionOutpost(t, "/outpost.goauthentik.io", "ar")
	idp := newMockIdP(t)
	cases := []struct {
		name     string
		auth     *model.AuthMiddleware
		mws      []string
		owned    string
		notOwned string
	}{
		{"oidc inline", ptOIDC, nil, ptCallback, oidcCallbackPath + "/"},
		{"oidc inline, mode from the provider type", &model.AuthMiddleware{IdentityProvider: "oidc"}, nil, ptCallback, oidcCallbackPath + "x"},
		{"oidc middleware", nil, []string{"oidc"}, ptCallback, ptOutpostURL},
		{"auth-request inline", ptAR, nil, ptOutpostURL, "/outpost.goauthentik.io-x"},
		{"auth-request inline, mode from the provider type", &model.AuthMiddleware{IdentityProvider: "ar"}, nil, "/outpost.goauthentik.io", ptCallback},
		{"auth-request middleware", nil, []string{"ar"}, ptOutpostURL, ptCallback},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, hostUp, locUp := ptLocationHost(t, tc.auth, tc.mws...)
			hh := ptBuild(t, ptConfig(t, ar.URL, idp.server.URL), host)
			for path, want := range map[string]string{"/": hostUp, "/public": hostUp, "/admin": locUp, "/admin/x": locUp} {
				if _, got := hh.route(path); got != want {
					t.Errorf("route(%q) = %s, want %s", path, got, want)
				}
			}
			if rec := ptServe(hh, tc.owned, ptWAN); !ptGateServed(rec, tc.owned) {
				t.Fatalf("%s: got %d %q, want the gate's own answer", tc.owned, rec.Code, rec.Body.String())
			}
			for _, path := range []string{tc.notOwned, "/public"} {
				if rec := ptServe(hh, path, ptWAN); rec.Code != http.StatusOK || rec.Body.String() != "BACKEND" {
					t.Fatalf("%s: got %d %q, want the backend", path, rec.Code, rec.Body.String())
				}
			}
			// The whole flow: the backend must first answer on the page that was
			// asked for, never on the gate's own path.
			if got := signInAt(t, routed(hh), idp, "/admin/x"); got != "/admin/x" {
				t.Fatalf("the backend first answered on %q, want /admin/x", got)
			}
		})
	}
}

// TestPassThroughRouteKeepsHostChecks: on the pass-through route a request
// still meets every host-level tier it met on the host route (access lists,
// non-interactive gates), and a path that a location covers stays with that
// location, whatever a gate elsewhere owns.
func TestPassThroughRouteKeepsHostChecks(t *testing.T) {
	ar := sessionOutpost(t, "/outpost.goauthentik.io", "ar")
	idp := newMockIdP(t)
	base := func() model.Config {
		cfg := ptConfig(t, ar.URL, idp.server.URL)
		cfg.AccessLists = []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "lan"},
			DefaultAction: model.ActionDeny,
			Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "192.0.2.0/24"}},
		}}
		return cfg
	}
	for _, gate := range []struct {
		name  string
		auth  *model.AuthMiddleware
		owned string
	}{{"oidc", ptOIDC, ptCallback}, {"auth-request", ptAR, ptOutpostURL}} {
		t.Run(gate.name+": host access list", func(t *testing.T) {
			host, _, _ := ptLocationHost(t, gate.auth)
			host.AccessLists = []string{"lan"}
			hh := ptBuild(t, base(), host)
			if rec := ptServe(hh, gate.owned, ptWAN); rec.Code != http.StatusForbidden {
				t.Fatalf("outside the list: got %d %q, want 403", rec.Code, rec.Body.String())
			}
			if rec := ptServe(hh, gate.owned, ptLAN); !ptGateServed(rec, gate.owned) {
				t.Fatalf("inside the list: got %d %q, want the gate's own answer", rec.Code, rec.Body.String())
			}
		})
		t.Run(gate.name+": host basic gate", func(t *testing.T) {
			host, _, _ := ptLocationHost(t, gate.auth)
			host.Middlewares = []string{"basic"}
			hh := ptBuild(t, base(), host)
			if rec := ptServe(hh, gate.owned, ptWAN); rec.Code != http.StatusUnauthorized {
				t.Fatalf("no credentials: got %d %q, want 401 from the host's basic gate", rec.Code, rec.Body.String())
			}
			if rec := ptServe(hh, gate.owned, ptLAN); !ptGateServed(rec, gate.owned) {
				t.Fatalf("LAN exemption: got %d %q, want the gate's own answer", rec.Code, rec.Body.String())
			}
		})
		t.Run(gate.name+": a location that covers the path keeps it", func(t *testing.T) {
			host, _, _ := ptLocationHost(t, gate.auth)
			host.Locations = append(host.Locations, model.Location{Path: "/", AccessLists: []string{"lan"}})
			hh := ptBuild(t, base(), host)
			if rec := ptServe(hh, gate.owned, ptWAN); rec.Code != http.StatusForbidden {
				t.Fatalf("got %d %q, want 403 from the covering location's list", rec.Code, rec.Body.String())
			}
			// Inside the list the path reaches that location's backend, as before:
			// the covering location does not carry the gate.
			if rec := ptServe(hh, gate.owned, ptLAN); rec.Body.String() != "BACKEND" {
				t.Fatalf("got %d %q, want the covering location's backend", rec.Code, rec.Body.String())
			}
		})
		t.Run(gate.name+": host path block", func(t *testing.T) {
			cfg := base()
			cfg.Middlewares = append(cfg.Middlewares, model.Middleware{
				ObjectMeta: model.ObjectMeta{Name: "pb"}, Type: model.MWTypePathBlock,
				PathBlock: &model.PathBlockMiddleware{Names: []string{"__gpm", "outpost.goauthentik.io"}},
			})
			host, _, _ := ptLocationHost(t, gate.auth)
			host.Middlewares = []string{"pb"}
			hh := ptBuild(t, cfg, host)
			if rec := ptServe(hh, gate.owned, ptWAN); rec.Code != http.StatusNotFound || ptGateServed(rec, gate.owned) {
				t.Fatalf("got %d %q, want 404 from the host's path block", rec.Code, rec.Body.String())
			}
		})
		for _, inline := range []bool{false, true} {
			t.Run(gate.name+": host rate limit shares its bucket"+map[bool]string{false: ", middleware", true: ", inline"}[inline], func(t *testing.T) {
				rl := model.RateLimitMiddleware{Requests: 1, Window: "1h", Burst: 1}
				cfg := base()
				cfg.Middlewares = append(cfg.Middlewares, model.Middleware{ObjectMeta: model.ObjectMeta{Name: "rl"}, Type: model.MWTypeRateLimit, RateLimit: &rl})
				host, _, _ := ptLocationHost(t, gate.auth)
				if inline {
					host.RateLimit = &rl
				} else {
					host.Middlewares = []string{"rl"}
				}
				hh := ptBuild(t, cfg, host)
				// One request is allowed per client: the one on the host route
				// spends it, so the pass-through path is limited too.
				if rec := ptServe(hh, "/public", ptWAN); rec.Code != http.StatusOK {
					t.Fatalf("first request: got %d, want 200", rec.Code)
				}
				if rec := ptServe(hh, gate.owned, ptWAN); rec.Code != http.StatusTooManyRequests {
					t.Fatalf("pass-through path: got %d %q, want 429 from the host's rate limit", rec.Code, rec.Body.String())
				}
			})
		}
		t.Run(gate.name+": host inline basic gate", func(t *testing.T) {
			host, _, _ := ptLocationHost(t, gate.auth)
			inline := basicMW(t, "u", "p", "inline", "192.0.2.0/24")
			host.Auth = &inline
			hh := ptBuild(t, base(), host)
			if rec := ptServe(hh, gate.owned, ptWAN); rec.Code != http.StatusUnauthorized {
				t.Fatalf("no credentials: got %d %q, want 401 from the host's inline basic gate", rec.Code, rec.Body.String())
			}
			if rec := ptServe(hh, gate.owned, ptLAN); !ptGateServed(rec, gate.owned) {
				t.Fatalf("LAN exemption: got %d %q, want the gate's own answer", rec.Code, rec.Body.String())
			}
		})
		// A host gate that asks for an interactive mode its provider cannot
		// serve fails closed, on the pass-through paths like everywhere else:
		// whether the location gate reaches them through the extra route or as
		// a covering location.
		for _, broken := range []model.AuthMiddleware{
			{IdentityProvider: "fa", Mode: model.AuthModeOIDC},
			{IdentityProvider: "oidc", Mode: model.AuthModeAuthRequest},
			{IdentityProvider: "ar", Mode: model.AuthModeOIDC},
		} {
			for _, covering := range []bool{false, true} {
				t.Run(gate.name+": broken host gate "+broken.IdentityProvider+"/"+broken.Mode+map[bool]string{false: "", true: ", covering location"}[covering], func(t *testing.T) {
					host, _, _ := ptLocationHost(t, gate.auth)
					b := broken
					host.Auth = &b
					if covering {
						host.Locations[0].Path = "/"
					}
					hh := ptBuild(t, base(), host)
					if !covering && len(hh.passThrough) != 0 {
						t.Fatal("a pass-through route was built on a host with an interactive-mode gate")
					}
					// Under the location its own gate runs first and redirects
					// into sign-in, as it always did; the pass-through path and
					// the rest of the host get the failed gate's 503.
					paths := []string{gate.owned, "/public"}
					if covering {
						paths = []string{gate.owned}
					}
					for _, path := range paths {
						if rec := ptServe(hh, path, ptWAN); rec.Code != http.StatusServiceUnavailable {
							t.Fatalf("%s: got %d, want 503 from the host's failed gate", path, rec.Code)
						}
					}
				})
			}
		}
	}
	t.Run("a location on the outpost prefix keeps it", func(t *testing.T) {
		host, _, _ := ptLocationHost(t, ptAR)
		host.Locations = append(host.Locations, model.Location{Path: "/outpost.goauthentik.io", AccessLists: []string{"lan"}})
		hh := ptBuild(t, base(), host)
		if rec := ptServe(hh, ptOutpostURL, ptWAN); rec.Code != http.StatusForbidden {
			t.Fatalf("got %d %q, want 403", rec.Code, rec.Body.String())
		}
	})
	t.Run("a gated root location serves the callback for a stricter sub-location", func(t *testing.T) {
		cfg := base()
		for i := range cfg.IdentityProviders {
			if cfg.IdentityProviders[i].Name == "oidc" {
				cfg.IdentityProviders[i].RoleMapping = &model.RoleMapping{AdminGroups: []string{"admins"}, DefaultRole: "user"}
			}
		}
		host, _, _ := ptLocationHost(t, &model.AuthMiddleware{IdentityProvider: "oidc", Mode: model.AuthModeOIDC, RequiredRoles: []string{"admin"}})
		host.Locations = append(host.Locations, model.Location{Path: "/", Auth: ptOIDC})
		hh := ptBuild(t, cfg, host)
		// A user with the default role signs in at the root: the root
		// location's gate answers the callback, not the admin-only one.
		if got := signInAt(t, routed(hh), idp, "/"); got != "/" {
			t.Fatalf("the backend first answered on %q, want /", got)
		}
	})
}

// TestNoPassThroughRouteWhenInDoubt: the pass-through route exists only when
// it cannot give a request fewer checks than the host route and there is no
// doubt about which gate answers. In every case below the host routes exactly
// as it did before the route existed.
func TestNoPassThroughRouteWhenInDoubt(t *testing.T) {
	ar := sessionOutpost(t, "/outpost.goauthentik.io", "ar")
	idp := newMockIdP(t)
	admin := &model.AuthMiddleware{IdentityProvider: "oidc", Mode: model.AuthModeOIDC, RequiredRoles: []string{"admin"}}
	second := model.Location{Path: "/app", Upstream: &model.Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80}}
	cases := []struct {
		name string
		edit func(cfg *model.Config, host *model.ProxyHost)
	}{
		{"no location gate", func(_ *model.Config, h *model.ProxyHost) { h.Locations[0].Auth = nil }},
		{"basic gate on the location", func(_ *model.Config, h *model.ProxyHost) {
			h.Locations[0].Auth, h.Locations[0].Middlewares = nil, []string{"basic"}
		}},
		{"forward-auth gate on the location", func(_ *model.Config, h *model.ProxyHost) {
			h.Locations[0].Auth, h.Locations[0].Middlewares = nil, []string{"fa"}
		}},
		{"host carries a referenced interactive gate", func(_ *model.Config, h *model.ProxyHost) {
			h.Middlewares = []string{"oidc"}
			h.Locations[0].Auth = ptAR
		}},
		{"host carries an inline interactive gate", func(_ *model.Config, h *model.ProxyHost) { h.Auth = ptAR }},
		// The mode comes from the provider type on both of these.
		{"host carries an auth-request gate, mode from the provider type", func(_ *model.Config, h *model.ProxyHost) {
			h.Auth = &model.AuthMiddleware{IdentityProvider: "ar"}
		}},
		{"host carries an oidc gate, mode from the provider type", func(_ *model.Config, h *model.ProxyHost) {
			h.Auth = &model.AuthMiddleware{IdentityProvider: "oidc"}
			h.Locations[0].Auth = ptAR
		}},
		{"two locations, same provider, different roles", func(_ *model.Config, h *model.ProxyHost) {
			l := second
			l.Auth = admin
			h.Locations = append(h.Locations, l)
		}},
		{"outpost prefix is the whole host", func(cfg *model.Config, h *model.ProxyHost) {
			cfg.IdentityProviders = append(cfg.IdentityProviders, model.IdentityProvider{
				ObjectMeta: model.ObjectMeta{Name: "root-ar"}, Type: model.IdPTypeAuthRequest,
				AuthRequest: &model.AuthRequestSpec{OutpostURL: ar.URL, PathPrefix: "/"},
			})
			h.Locations[0].Auth = &model.AuthMiddleware{IdentityProvider: "root-ar", Mode: model.AuthModeAuthRequest}
		}},
		{"unknown identity provider", func(_ *model.Config, h *model.ProxyHost) {
			h.Locations[0].Auth = &model.AuthMiddleware{IdentityProvider: "missing", Mode: model.AuthModeOIDC}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ptConfig(t, ar.URL, idp.server.URL)
			host, hostUp, _ := ptLocationHost(t, ptOIDC)
			tc.edit(&cfg, &host)
			hh := ptBuild(t, cfg, host)
			if len(hh.passThrough) != 0 {
				t.Fatal("a pass-through route was built")
			}
			for _, path := range []string{"/", oidcCallbackPath, ptOutpostURL} {
				if _, got := hh.route(path); got != hostUp {
					t.Errorf("route(%q) = %s, want the host route", path, got)
				}
			}
		})
	}
}

// TestPassThroughRouteSharedAndFailClosed: one gate on several locations
// shares the route (each location's sign-in completes); two gates of
// different kinds each keep their own path; a gate that cannot compile fails
// closed on its path.
func TestPassThroughRouteSharedAndFailClosed(t *testing.T) {
	ar := sessionOutpost(t, "/outpost.goauthentik.io", "ar")
	idp := newMockIdP(t)
	second := func(t *testing.T) model.Location {
		h, _, _ := ptLocationHost(t, nil)
		l := h.Locations[0]
		l.Path = "/app"
		return l
	}
	t.Run("one middleware on two locations", func(t *testing.T) {
		host, _, _ := ptLocationHost(t, nil, "oidc")
		l := second(t)
		l.Middlewares = []string{"oidc"}
		host.Locations = append(host.Locations, l)
		hh := ptBuild(t, ptConfig(t, ar.URL, idp.server.URL), host)
		if len(hh.passThrough) != 1 {
			t.Fatalf("got %d pass-through routes, want 1 (the spec counts once)", len(hh.passThrough))
		}
		for _, start := range []string{"/admin/x", "/app/x"} {
			if got := signInAt(t, routed(hh), idp, start); got != start {
				t.Fatalf("from %s the backend first answered on %q", start, got)
			}
		}
	})
	t.Run("equal inline blocks on two locations", func(t *testing.T) {
		// Written differently (mode set or taken from the provider type, roles
		// in another order), the two blocks are still one gate.
		cfg := ptConfig(t, ar.URL, idp.server.URL)
		for i := range cfg.IdentityProviders {
			if cfg.IdentityProviders[i].Name == "oidc" {
				cfg.IdentityProviders[i].RoleMapping = &model.RoleMapping{AdminGroups: []string{"admins"}, DefaultRole: "user"}
			}
		}
		host, _, _ := ptLocationHost(t, &model.AuthMiddleware{IdentityProvider: "oidc", Mode: model.AuthModeOIDC, RequiredRoles: []string{"admin", "user"}})
		l := second(t)
		l.Auth = &model.AuthMiddleware{IdentityProvider: "oidc", RequiredRoles: []string{"user", "admin"}}
		host.Locations = append(host.Locations, l)
		hh := ptBuild(t, cfg, host)
		if len(hh.passThrough) != 1 {
			t.Fatalf("got %d pass-through routes, want 1", len(hh.passThrough))
		}
		for _, start := range []string{"/admin/x", "/app/x"} {
			if got := signInAt(t, routed(hh), idp, start); got != start {
				t.Fatalf("from %s the backend first answered on %q", start, got)
			}
		}
	})
	t.Run("oidc on one location, auth-request on another", func(t *testing.T) {
		// Each gate gets a route of its own. The oidc gate admits admins only
		// and the user below is not one, so a route that stacked both gates
		// would stop the outpost sign-in at the oidc gate.
		cfg := ptConfig(t, ar.URL, idp.server.URL)
		for i := range cfg.IdentityProviders {
			if cfg.IdentityProviders[i].Name == "oidc" {
				cfg.IdentityProviders[i].RoleMapping = &model.RoleMapping{AdminGroups: []string{"admins"}, DefaultRole: "user"}
			}
		}
		for _, order := range []string{"oidc first", "auth-request first"} {
			host, _, _ := ptLocationHost(t, ptAR)
			l := second(t)
			l.Auth = &model.AuthMiddleware{IdentityProvider: "oidc", Mode: model.AuthModeOIDC, RequiredRoles: []string{"admin"}}
			host.Locations = append(host.Locations, l)
			if order == "oidc first" {
				host.Locations[0], host.Locations[1] = host.Locations[1], host.Locations[0]
			}
			hh := ptBuild(t, cfg, host)
			if len(hh.passThrough) != 2 {
				t.Fatalf("%s: got %d pass-through routes, want 2", order, len(hh.passThrough))
			}
			if got := signInAt(t, routed(hh), idp, "/admin/x"); got != "/admin/x" {
				t.Fatalf("%s: the outpost sign-in first reached the backend on %q", order, got)
			}
			// The oidc gate still answers its own callback, and on nothing else.
			if rec := ptServe(hh, ptCallback, ptWAN); !ptGateServed(rec, ptCallback) {
				t.Fatalf("%s: callback: got %d %q", order, rec.Code, rec.Body.String())
			}
			if rec := ptServe(hh, ptOutpostURL, ptWAN); !ptGateServed(rec, ptOutpostURL) {
				t.Fatalf("%s: outpost path: got %d %q", order, rec.Code, rec.Body.String())
			}
		}
	})
	t.Run("a gate that cannot compile fails closed on its path", func(t *testing.T) {
		cfg := ptConfig(t, ar.URL, idp.server.URL)
		cfg.IdentityProviders = append(cfg.IdentityProviders, model.IdentityProvider{
			ObjectMeta: model.ObjectMeta{Name: "nosecret"}, Type: model.IdPTypeOIDC,
			OIDC: &model.OIDCSpec{IssuerURL: idp.server.URL, ClientID: "gpm-client", ClientSecret: "${ENV:GPM_TEST_UNSET_OIDC_SECRET}"},
		})
		host, _, _ := ptLocationHost(t, &model.AuthMiddleware{IdentityProvider: "nosecret", Mode: model.AuthModeOIDC})
		hh := ptBuild(t, cfg, host)
		rec := ptServe(hh, ptCallback, ptWAN)
		if rec.Code != http.StatusServiceUnavailable || rec.Body.String() == "BACKEND" {
			t.Fatalf("got %d %q, want 503 and never the backend", rec.Code, rec.Body.String())
		}
	})
}

// TestSignInCompletesWithTwoInteractiveGates: a route with two interactive
// gates signs in to both, in either order. Making each gate's pass-through
// paths wait on the other gate would loop here forever.
func TestSignInCompletesWithTwoInteractiveGates(t *testing.T) {
	px := sessionOutpost(t, "/px", "px")
	py := sessionOutpost(t, "/py", "py")
	ar := sessionOutpost(t, "/outpost.goauthentik.io", "ar")
	idp := newMockIdP(t)
	cfg := ptConfig(t, ar.URL, idp.server.URL)
	cfg.IdentityProviders = append(cfg.IdentityProviders,
		model.IdentityProvider{ObjectMeta: model.ObjectMeta{Name: "px"}, Type: model.IdPTypeAuthRequest, AuthRequest: &model.AuthRequestSpec{OutpostURL: px.URL, PathPrefix: "/px"}},
		model.IdentityProvider{ObjectMeta: model.ObjectMeta{Name: "py"}, Type: model.IdPTypeAuthRequest, AuthRequest: &model.AuthRequestSpec{OutpostURL: py.URL, PathPrefix: "/py"}},
	)
	cfg.Middlewares = append(cfg.Middlewares,
		model.Middleware{ObjectMeta: model.ObjectMeta{Name: "px"}, Type: model.MWTypeAuth, Auth: &model.AuthMiddleware{IdentityProvider: "px", Mode: model.AuthModeAuthRequest}},
		model.Middleware{ObjectMeta: model.ObjectMeta{Name: "py"}, Type: model.MWTypeAuth, Auth: &model.AuthMiddleware{IdentityProvider: "py", Mode: model.AuthModeAuthRequest}},
	)
	cases := []struct {
		name string
		mws  []string
	}{
		{"auth-request /py before /px", []string{"px", "py"}},
		{"auth-request /px before /py", []string{"py", "px"}},
		{"oidc before auth-request", []string{"ar", "oidc"}},
		{"auth-request before oidc", []string{"oidc", "ar"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			signIn(t, buildChain(ptBackend(), ptHost(tc.mws...), buildRegistry(cfg), nil), idp)
		})
	}
}

// TestPassThroughOnLocationMeetsInheritedGate: on a location route the host's
// inherited non-interactive gate runs inside the location's own inline
// auth-request gate, and still applies on that gate's outpost prefix.
func TestPassThroughOnLocationMeetsInheritedGate(t *testing.T) {
	stub := &outpostStub{authStatus: http.StatusUnauthorized}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()
	cfg := ptConfig(t, srv.URL, newMockIdP(t).server.URL)
	cfg.IdentityProviders = append(cfg.IdentityProviders, model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: "loc-ar"}, Type: model.IdPTypeAuthRequest,
		AuthRequest: &model.AuthRequestSpec{OutpostURL: srv.URL, PathPrefix: "/loc/outpost"},
	})
	host := ptHost("basic")
	host.TLS = model.TLSSettings{}
	host.Upstream = model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}
	host.Locations = []model.Location{{
		Path: "/loc",
		Auth: &model.AuthMiddleware{IdentityProvider: "loc-ar", Mode: model.AuthModeAuthRequest},
	}}
	cfg.ProxyHosts = []model.ProxyHost{host}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	loc := rt.hosts[ptDomain].locations[0].handler

	cases := []struct {
		name     string
		cred     ptCred
		wantCode int
	}{
		{"no credentials: inherited basic refuses", credNone, http.StatusUnauthorized},
		{"credentials: outpost serves", credBasic, http.StatusOK},
		{"LAN exemption: outpost serves", credLAN, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			loc.ServeHTTP(rec, ptRequest(t, "/loc/outpost/start?rd=/", tc.cred, nil, nil))
			if rec.Code != tc.wantCode {
				t.Fatalf("got %d %q, want %d", rec.Code, rec.Body.String(), tc.wantCode)
			}
			if tc.wantCode == http.StatusOK && !strings.HasPrefix(rec.Body.String(), "OUTPOST:") {
				t.Fatalf("want the outpost's answer, got %q", rec.Body.String())
			}
		})
	}
}

// TestPassThroughHeadersUnchangedByInnerGate: a non-interactive gate that runs
// before the pass-through changes nothing on the request, so the outpost sees
// the same headers whether that gate runs before or after the auth-request
// gate, and the gate's forged-identity strip still happens.
func TestPassThroughHeadersUnchangedByInnerGate(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte("OUTPOST:" + r.URL.Path))
	}))
	defer srv.Close()
	cfg := ptConfig(t, srv.URL, newMockIdP(t).server.URL)

	for _, tc := range []struct {
		inner string
		cred  ptCred
	}{{"basic", credBasic}, {"fa", credForward}} {
		t.Run(tc.inner, func(t *testing.T) {
			seen := map[string]http.Header{}
			for order, mws := range map[string][]string{
				"inner gate first":   {"ar", tc.inner},
				"auth-request first": {tc.inner, "ar"},
			} {
				got = nil
				h := buildChain(ptBackend(), ptHost(mws...), buildRegistry(cfg), nil)
				req := ptRequest(t, "/outpost.goauthentik.io/start?rd=/", tc.cred, nil, nil)
				req.Header.Set("X-Authentik-Username", "forged")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK || got == nil {
					t.Fatalf("%s: got %d %q", order, rec.Code, rec.Body.String())
				}
				if v := got.Values("X-Authentik-Username"); len(v) != 0 {
					t.Fatalf("%s: forged identity header reached the outpost: %v", order, v)
				}
				seen[order] = got
			}
			a, b := seen["inner gate first"], seen["auth-request first"]
			if len(a) != len(b) {
				t.Fatalf("header sets differ:\n%v\n%v", a, b)
			}
			for k, v := range a {
				if strings.Join(v, "\x00") != strings.Join(b[k], "\x00") {
					t.Fatalf("header %s differs: %q vs %q", k, v, b[k])
				}
			}
		})
	}
}

// TestOwnedPathsOverlap: two gates clash when some path belongs to both. The
// test is symmetric and covers every pairing of the exact callback path and
// outpost prefixes (equal, nested, disjoint, covering the callback).
func TestOwnedPathsOverlap(t *testing.T) {
	cb := ownedPaths{exact: oidcCallbackPath}
	pfx := func(p string) ownedPaths { return ownedPaths{prefix: p} }
	cases := []struct {
		a, b ownedPaths
		want bool
	}{
		{cb, cb, true},
		{cb, pfx("/outpost.goauthentik.io"), false},
		{cb, pfx("/__gpm"), true},
		{cb, pfx("/__gpm/oidc"), true},
		{cb, pfx(oidcCallbackPath), true},
		{cb, pfx("/__gpm/oidc/call"), false},
		{pfx("/out"), pfx("/out"), true},
		{pfx("/out"), pfx("/out/inner"), true},
		{pfx("/out"), pfx("/outer"), false},
		{pfx("/a"), pfx("/b"), false},
	}
	for _, tc := range cases {
		if got := tc.a.overlaps(tc.b); got != tc.want {
			t.Errorf("%+v overlaps %+v = %v, want %v", tc.a, tc.b, got, tc.want)
		}
		if got := tc.b.overlaps(tc.a); got != tc.want {
			t.Errorf("%+v overlaps %+v = %v, want %v (reverse)", tc.b, tc.a, got, tc.want)
		}
	}
}

// TestDebugUpstreamHeaderOmittedOnGatePaths: on an interactive gate's own path
// (the OIDC callback, the outpost prefix) the gate answers and the upstream is
// never contacted, so X-GPM-Upstream is left out, as on every other response
// gpm produces itself. Every other path still names the route's upstream.
func TestDebugUpstreamHeaderOmittedOnGatePaths(t *testing.T) {
	ar := sessionOutpost(t, "/outpost.goauthentik.io", "ar")
	idp := newMockIdP(t)
	cases := []struct {
		name string
		edit func(h *model.ProxyHost)
		gate string                                   // the gate's own path
		up   func(host, loc string) map[string]string // path -> X-GPM-Upstream
	}{
		{"location-only oidc gate", func(*model.ProxyHost) {}, ptCallback,
			func(host, loc string) map[string]string { return map[string]string{"/public": host, "/admin": loc} }},
		{"host auth-request gate", func(h *model.ProxyHost) { h.Auth, h.Locations[0].Auth = ptAR, nil }, ptOutpostURL,
			func(host, loc string) map[string]string { return map[string]string{"/public": host, "/admin": loc} }},
		{"auth-request gate on a location covering the gate path", func(h *model.ProxyHost) {
			h.Locations[0].Path, h.Locations[0].Auth = "/", ptAR
		}, ptOutpostURL, func(_, loc string) map[string]string { return map[string]string{"/public": loc} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, hostUp, locUp := ptLocationHost(t, ptOIDC)
			tc.edit(&host)
			cfg := ptConfig(t, ar.URL, idp.server.URL)
			cfg.ProxyHosts = []model.ProxyHost{host}
			s := New(Config{DebugHeaders: true})
			if err := s.Reload(cfg); err != nil {
				t.Fatal(err)
			}
			h := s.observe(http.HandlerFunc(s.dispatchHTTP))
			serve := func(path string) *httptest.ResponseRecorder {
				req := httptest.NewRequest("GET", "http://"+ptDomain+path, nil)
				req.RemoteAddr = ptWAN
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				return rec
			}
			rec := serve(tc.gate)
			if !ptGateServed(rec, tc.gate) {
				t.Fatalf("%s: got %d %q, want the gate's own answer", tc.gate, rec.Code, rec.Body.String())
			}
			if v, ok := rec.Header()["X-Gpm-Upstream"]; ok {
				t.Errorf("%s: X-GPM-Upstream = %q, want none", tc.gate, v)
			}
			if got := rec.Header().Get("X-GPM-Host"); got != "app" {
				t.Errorf("%s: X-GPM-Host = %q, want app", tc.gate, got)
			}
			for path, want := range tc.up(hostUp, locUp) {
				if got := serve(path).Header().Get("X-GPM-Upstream"); got != want {
					t.Errorf("%s: X-GPM-Upstream = %q, want %q", path, got, want)
				}
			}
		})
	}
}
