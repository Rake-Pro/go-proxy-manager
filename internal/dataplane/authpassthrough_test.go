package dataplane

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	jar := map[string]*http.Cookie{}
	target := "https://" + ptDomain + "/"
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
			return
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
