package dataplane

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// inlineReq is one request replayed against both compiled chains.
type inlineReq struct {
	name string
	req  func() *http.Request
}

// TestInlineAuthGatesLikeMiddleware is the equivalence proof for the inline
// `auth` block on a proxy host: for each auth mode, the SAME spec is compiled
// twice - once as a referenced `type: auth` middleware, once as the host's
// inline block - and every request must be answered identically. If the two ever
// diverge, the direct path is no longer the same gate as the reuse path.
func TestInlineAuthGatesLikeMiddleware(t *testing.T) {
	// auth-request: a stub outpost that authorizes, so the exercised difference
	// is authorized (200) vs. an IP-exempt / unauthorized path below.
	okOutpost := &outpostStub{authStatus: http.StatusOK, authHeaders: map[string]string{"X-authentik-username": "admin"}}
	okSrv := httptest.NewServer(okOutpost.handler())
	defer okSrv.Close()
	denyOutpost := &outpostStub{authStatus: http.StatusUnauthorized}
	denySrv := httptest.NewServer(denyOutpost.handler())
	defer denySrv.Close()

	// client-cert: one CA, one leaf, so a request can present a verified chain.
	_, caCert, caKey := crlCA(t, "corp-ca")
	_, ops := clientCertSerial(t, caCert, caKey, 91, "ops")
	withCert := func() *http.Request {
		r := httptest.NewRequest("GET", "https://app.example.com/", nil)
		r.RemoteAddr = "203.0.113.5:5000"
		r.TLS = &tls.ConnectionState{
			ServerName:       "app.example.com",
			PeerCertificates: []*x509.Certificate{ops},
			VerifiedChains:   [][]*x509.Certificate{{ops, caCert}},
		}
		return r
	}
	plain := func(remote string) func() *http.Request {
		return func() *http.Request {
			r := httptest.NewRequest("GET", "http://app.example.com/", nil)
			r.RemoteAddr = remote
			return r
		}
	}
	asProxy := func(remote, user string) func() *http.Request {
		return func() *http.Request {
			r := plain(remote)()
			r.Header.Set("X-User", user)
			return r
		}
	}

	forwardAuthIdP := model.IdentityProvider{
		ObjectMeta:  model.ObjectMeta{Name: "fa"},
		Type:        model.IdPTypeForwardAuth,
		ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
		RoleMapping: &model.RoleMapping{DefaultRole: "user"},
	}

	tests := []struct {
		name string
		idps []model.IdentityProvider
		spec model.AuthMiddleware
		reqs []inlineReq
	}{
		{
			name: "forward-auth",
			idps: []model.IdentityProvider{forwardAuthIdP},
			spec: model.AuthMiddleware{IdentityProvider: "fa", Mode: model.AuthModeForwardAuth},
			reqs: []inlineReq{
				{"trusted proxy asserting an identity", asProxy("10.1.2.3:5000", "admin")},
				{"untrusted peer asserting an identity", asProxy("203.0.113.5:5000", "attacker")},
				{"no identity at all", plain("10.1.2.3:5000")},
			},
		},
		{
			name: "auth-request authorized",
			idps: []model.IdentityProvider{{
				ObjectMeta:  model.ObjectMeta{Name: "ar"},
				Type:        model.IdPTypeAuthRequest,
				AuthRequest: &model.AuthRequestSpec{OutpostURL: okSrv.URL},
			}},
			spec: model.AuthMiddleware{IdentityProvider: "ar", Mode: model.AuthModeAuthRequest},
			reqs: []inlineReq{{"outpost says 200", plain("203.0.113.5:5000")}},
		},
		{
			name: "auth-request refused",
			idps: []model.IdentityProvider{{
				ObjectMeta:  model.ObjectMeta{Name: "ar"},
				Type:        model.IdPTypeAuthRequest,
				AuthRequest: &model.AuthRequestSpec{OutpostURL: denySrv.URL},
			}},
			spec: model.AuthMiddleware{IdentityProvider: "ar", Mode: model.AuthModeAuthRequest},
			reqs: []inlineReq{{"outpost says 401", plain("203.0.113.5:5000")}},
		},
		{
			name: "auth-request with an allowFrom exemption",
			idps: []model.IdentityProvider{{
				ObjectMeta:  model.ObjectMeta{Name: "ar"},
				Type:        model.IdPTypeAuthRequest,
				AuthRequest: &model.AuthRequestSpec{OutpostURL: denySrv.URL},
			}},
			spec: model.AuthMiddleware{IdentityProvider: "ar", Mode: model.AuthModeAuthRequest, AllowFrom: []string{"10.0.0.0/8"}},
			reqs: []inlineReq{
				{"exempt network bypasses the outpost", plain("10.1.2.3:5000")},
				{"everyone else is still refused", plain("203.0.113.5:5000")},
			},
		},
		{
			name: "client-cert",
			spec: model.AuthMiddleware{Mode: model.AuthModeClientCert},
			reqs: []inlineReq{
				{"verified certificate", withCert},
				{"no certificate", plain("203.0.113.5:5000")},
			},
		},
		{
			name: "client-cert with a role mapping",
			spec: model.AuthMiddleware{
				Mode:            model.AuthModeClientCert,
				RequiredRoles:   []string{"admin"},
				ClientCertRoles: map[string]string{"ops": "admin"},
			},
			reqs: []inlineReq{
				{"mapped subject", withCert},
				{"no certificate", plain("203.0.113.5:5000")},
			},
		},
		{
			name: "client-cert whose subject maps to nothing",
			spec: model.AuthMiddleware{
				Mode:            model.AuthModeClientCert,
				RequiredRoles:   []string{"admin"},
				ClientCertRoles: map[string]string{"someone-else": "admin"},
			},
			reqs: []inlineReq{{"unmapped subject", withCert}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := tc.spec
			refCfg := model.Config{
				IdentityProviders: tc.idps,
				Middlewares: []model.Middleware{{
					ObjectMeta: model.ObjectMeta{Name: "gate"}, Type: model.MWTypeAuth, Auth: &spec,
				}},
			}
			refHost := model.ProxyHost{
				ObjectMeta:  model.ObjectMeta{Name: "app"},
				Domains:     []string{"app.example.com"},
				Middlewares: []string{"gate"},
			}
			refChain := buildChain(okHandler(), refHost, buildRegistry(refCfg), nil)

			inlineCfg := model.Config{IdentityProviders: tc.idps}
			inlineHost := model.ProxyHost{
				ObjectMeta: model.ObjectMeta{Name: "app"},
				Domains:    []string{"app.example.com"},
				Auth:       &spec,
			}
			inlineChainH := buildChain(okHandler(), inlineHost, buildRegistry(inlineCfg), nil)

			for _, r := range tc.reqs {
				refRec := httptest.NewRecorder()
				refChain.ServeHTTP(refRec, r.req())
				inRec := httptest.NewRecorder()
				inlineChainH.ServeHTTP(inRec, r.req())

				if refRec.Code != inRec.Code {
					t.Fatalf("%s: inline auth answered %d, the equivalent middleware answered %d - the two must be the same gate",
						r.name, inRec.Code, refRec.Code)
				}
				if refRec.Body.String() != inRec.Body.String() {
					t.Fatalf("%s: inline auth body %q != middleware body %q",
						r.name, inRec.Body.String(), refRec.Body.String())
				}
			}
		})
	}
}

// TestInlineAuthGateActuallyGates guards the equivalence test above from being
// vacuous: an inline block must genuinely admit an authorized request and refuse
// an unauthorized one, not pass everything through.
func TestInlineAuthGateActuallyGates(t *testing.T) {
	cfg := model.Config{IdentityProviders: []model.IdentityProvider{{
		ObjectMeta:  model.ObjectMeta{Name: "fa"},
		Type:        model.IdPTypeForwardAuth,
		ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
		RoleMapping: &model.RoleMapping{DefaultRole: "user"},
	}}}
	host := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Auth:       &model.AuthMiddleware{IdentityProvider: "fa", Mode: model.AuthModeForwardAuth},
	}
	h := buildChain(okHandler(), host, buildRegistry(cfg), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.RemoteAddr = "10.1.2.3:5000"
	req.Header.Set("X-User", "admin")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("a trusted proxy's identity must be admitted by the inline gate, got %d", rec.Code)
	}

	if got := serveRL(h, "203.0.113.5:5000", "http://app.example.com/").Code; got != http.StatusUnauthorized {
		t.Fatalf("an untrusted peer must be refused by the inline gate, got %d", got)
	}
}

// TestInlineAuthFailsClosedOnUnknownProvider: an inline block naming a provider
// that does not exist is the whole gate for that host, so it must serve 503
// rather than fall through - the same fail-closed posture a middleware has.
func TestInlineAuthFailsClosedOnUnknownProvider(t *testing.T) {
	host := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Auth:       &model.AuthMiddleware{IdentityProvider: "ghost", Mode: model.AuthModeForwardAuth},
	}
	h := buildChain(okHandler(), host, buildRegistry(model.Config{}), nil)
	rec := serveRL(h, "10.1.2.3:5000", "http://app.example.com/")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("an inline auth block with an unresolvable provider must fail closed (503), got %d", rec.Code)
	}
	if rec.Body.String() == "backend" {
		t.Fatal("the request reached the upstream despite an unresolvable identity provider")
	}
}

// TestInlineRateLimitGatesLikeMiddleware: the same limit expressed inline and as
// a `type: rate-limit` middleware sheds the same request with the same status
// and the same Retry-After.
func TestInlineRateLimitGatesLikeMiddleware(t *testing.T) {
	rl := model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1}

	refCfg := model.Config{Middlewares: []model.Middleware{{
		ObjectMeta: model.ObjectMeta{Name: "rl"}, Type: model.MWTypeRateLimit, RateLimit: &rl,
	}}}
	refChain := buildChain(okHandler(),
		model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, Middlewares: []string{"rl"}},
		buildRegistry(refCfg), nil)
	inlineChainH := buildChain(okHandler(),
		model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, RateLimit: &rl},
		buildRegistry(model.Config{}), nil)

	for i, want := range []int{http.StatusOK, http.StatusTooManyRequests, http.StatusTooManyRequests} {
		refRec := serveRL(refChain, "203.0.113.5:1", "http://app/")
		inRec := serveRL(inlineChainH, "203.0.113.5:1", "http://app/")
		if refRec.Code != inRec.Code {
			t.Fatalf("request %d: inline rateLimit answered %d, the middleware answered %d", i, inRec.Code, refRec.Code)
		}
		if inRec.Code != want {
			t.Fatalf("request %d: got %d, want %d", i, inRec.Code, want)
		}
		if got, ref := inRec.Header().Get("Retry-After"), refRec.Header().Get("Retry-After"); got != ref {
			t.Fatalf("request %d: inline Retry-After %q != middleware %q", i, got, ref)
		}
	}
}

// TestInlineRateLimitAllowFrom: the network exemption behaves inline exactly as
// it does on a middleware - an exempt client consumes no token and is never 429.
func TestInlineRateLimitAllowFrom(t *testing.T) {
	host := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		RateLimit:  &model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1, AllowFrom: []string{"10.0.0.0/8"}},
	}
	h := buildChain(okHandler(), host, buildRegistry(model.Config{}), nil)
	for i := 0; i < 5; i++ {
		if got := serveRL(h, "10.1.2.3:1", "http://app/").Code; got != http.StatusOK {
			t.Fatalf("request %d from an exempt network must never be throttled, got %d", i, got)
		}
	}
}

// TestInlineAuthRunsBeforeReferencedMiddleware pins the documented order: the
// inline block sits OUTSIDE the referenced auth middlewares, so it is evaluated
// first. The two gates are chosen to answer with different statuses - the inline
// client-cert gate refuses an unmapped subject with 403, the referenced
// forward-auth gate refuses an untrusted peer with 401 - so the status alone says
// which one ran.
func TestInlineAuthRunsBeforeReferencedMiddleware(t *testing.T) {
	_, caCert, caKey := crlCA(t, "corp-ca")
	_, ops := clientCertSerial(t, caCert, caKey, 92, "ops")

	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "fa"},
			Type:        model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
		}},
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "sso"}, Type: model.MWTypeAuth,
			Auth: &model.AuthMiddleware{IdentityProvider: "fa", Mode: model.AuthModeForwardAuth},
		}},
	}
	host := model.ProxyHost{
		ObjectMeta:  model.ObjectMeta{Name: "app"},
		Domains:     []string{"app.example.com"},
		Middlewares: []string{"sso"},
		Auth: &model.AuthMiddleware{
			Mode:            model.AuthModeClientCert,
			RequiredRoles:   []string{"admin"},
			ClientCertRoles: map[string]string{"someone-else": "admin"},
		},
	}
	h := buildChain(okHandler(), host, buildRegistry(cfg), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://app.example.com/", nil)
	req.RemoteAddr = "203.0.113.5:5000"
	req.TLS = &tls.ConnectionState{
		ServerName:       "app.example.com",
		PeerCertificates: []*x509.Certificate{ops},
		VerifiedChains:   [][]*x509.Certificate{{ops, caCert}},
	}
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("the inline gate must run before the referenced auth middleware (403 from client-cert), got %d", rec.Code)
	}

	// And the referenced middleware is still there: a request the inline gate
	// admits is handed on to it, not straight to the upstream.
	adm := model.ProxyHost{
		ObjectMeta:  model.ObjectMeta{Name: "app"},
		Domains:     []string{"app.example.com"},
		Middlewares: []string{"sso"},
		Auth:        &model.AuthMiddleware{Mode: model.AuthModeClientCert},
	}
	h2 := buildChain(okHandler(), adm, buildRegistry(cfg), nil)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "https://app.example.com/", nil)
	req2.RemoteAddr = "203.0.113.5:5000"
	req2.TLS = &tls.ConnectionState{
		ServerName:       "app.example.com",
		PeerCertificates: []*x509.Certificate{ops},
		VerifiedChains:   [][]*x509.Certificate{{ops, caCert}},
	}
	h2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("a request the inline gate admits must still face the referenced middleware (401), got %d", rec2.Code)
	}
}

// TestInlineRateLimitRunsBeforeReferencedMiddleware: same ordering rule one tier
// out. The inline bucket is the tighter of the two, so it is the one that sheds -
// which can only happen if it is wrapped outside the referenced limiter.
func TestInlineRateLimitRunsBeforeReferencedMiddleware(t *testing.T) {
	cfg := model.Config{Middlewares: []model.Middleware{{
		ObjectMeta: model.ObjectMeta{Name: "rl"}, Type: model.MWTypeRateLimit,
		RateLimit: &model.RateLimitMiddleware{RequestsPerSecond: 100, Burst: 100},
	}}}
	host := model.ProxyHost{
		ObjectMeta:  model.ObjectMeta{Name: "app"},
		Middlewares: []string{"rl"},
		RateLimit:   &model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1},
	}
	h := buildChain(okHandler(), host, buildRegistry(cfg), nil)

	if got := serveRL(h, "203.0.113.5:1", "http://app/").Code; got != http.StatusOK {
		t.Fatalf("first request must pass both limiters, got %d", got)
	}
	if got := serveRL(h, "203.0.113.5:1", "http://app/").Code; got != http.StatusTooManyRequests {
		t.Fatalf("the inline limiter must shed the second request, got %d", got)
	}
}

// TestLocationInlineAuthStacksOnHost: a location's inline blocks are APPENDED to
// the host's, exactly as its middleware names are - the host gate still applies
// on the location's path, and the location adds its own on top.
func TestLocationInlineAuthStacksOnHost(t *testing.T) {
	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "fa"},
			Type:        model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "app"},
			Domains:    []string{"app.example.com"},
			Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
			Auth:       &model.AuthMiddleware{IdentityProvider: "fa", Mode: model.AuthModeForwardAuth},
			Locations: []model.Location{{
				Path:      "/api",
				RateLimit: &model.RateLimitMiddleware{RequestsPerSecond: 1, Burst: 1},
			}},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	hh, ok := rt.hosts["app.example.com"]
	if !ok {
		t.Fatal("compiled host not found")
	}
	if len(hh.locations) != 1 {
		t.Fatalf("want 1 compiled location, got %d", len(hh.locations))
	}
	loc := hh.locations[0].handler

	// The HOST's inline auth still gates the location: an untrusted peer is 401,
	// never 429 and never the upstream.
	if got := serveRL(loc, "203.0.113.5:1", "http://app.example.com/api").Code; got != http.StatusUnauthorized {
		t.Fatalf("the host's inline auth must still gate the location, got %d", got)
	}
	// The LOCATION's inline rate limit is outermost, so it sheds first once the
	// bucket is empty - even for a request auth would have refused.
	if got := serveRL(loc, "203.0.113.5:1", "http://app.example.com/api").Code; got != http.StatusTooManyRequests {
		t.Fatalf("the location's inline rate limit must shed the second request, got %d", got)
	}
}

// TestInlineForwardAuthContributesIdentityTrust: an inline forward-auth block is
// a gate of equal standing to a middleware, so its provider must contribute the
// host's trusted proxies and the identity headers stripped from an untrusted
// peer. Without this an inline block would leave those headers forgeable.
func TestInlineForwardAuthContributesIdentityTrust(t *testing.T) {
	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta: model.ObjectMeta{Name: "authentik"},
			Type:       model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{
				TrustedProxies: []string{"10.0.0.0/8"},
				UserHeader:     "X-Authentik-Username",
				GroupsHeader:   "X-Authentik-Groups",
			},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "app"},
			Domains:    []string{"app.example.com"},
			Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
			Auth:       &model.AuthMiddleware{IdentityProvider: "authentik", Mode: model.AuthModeForwardAuth},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	hh, ok := rt.hosts["app.example.com"]
	if !ok {
		t.Fatal("compiled host not found")
	}
	if len(hh.trustedNets) == 0 {
		t.Fatal("an inline forward-auth block must contribute its provider's trusted proxies")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:5000"
	r.Header.Set("X-Authentik-Username", "attacker")
	r.Header.Set("X-Authentik-Groups", "proxy-admins")
	hh.stripUntrustedIdentity(r)
	if r.Header.Get("X-Authentik-Username") != "" || r.Header.Get("X-Authentik-Groups") != "" {
		t.Fatalf("forged identity headers must be stripped for a host gated by an inline block, got %v", r.Header)
	}
}

// TestLocationInlineForwardAuthContributesIdentityTrust: same rule for a block
// declared on a LOCATION - hostAuthSpecs walks h.Locations so a location's own
// inline provider contributes its identity headers to the HOST's strip set, or a
// forged header would reach the backend on a path the location does not cover.
// (Client-IP trust is a separate tier and comes from trustedProxies, not from
// any provider; see clientip.go.)
func TestLocationInlineForwardAuthContributesIdentityTrust(t *testing.T) {
	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta: model.ObjectMeta{Name: "authentik"},
			Type:       model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{
				TrustedProxies: []string{"10.0.0.0/8"},
				UserHeader:     "X-Authentik-Username",
			},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "app"},
			Domains:    []string{"app.example.com"},
			Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
			Locations: []model.Location{{
				Path: "/admin",
				Auth: &model.AuthMiddleware{IdentityProvider: "authentik", Mode: model.AuthModeForwardAuth},
			}},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	hh := rt.hosts["app.example.com"]
	if hh == nil || len(hh.locations) != 1 {
		t.Fatal("compiled location not found")
	}
	// The host-level strip set also sees the location's inline provider, so a
	// forged header never reaches the backend on any path of this host.
	found := false
	for _, name := range hh.identityHeaders {
		if name == "X-Authentik-Username" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a location's inline auth block must contribute its provider's identity headers, got %v", hh.identityHeaders)
	}

	// The gate itself is live on the location route: an untrusted peer is 401.
	if got := serveRL(hh.locations[0].handler, "203.0.113.5:1", "http://app.example.com/admin").Code; got != http.StatusUnauthorized {
		t.Fatalf("the location's inline auth must gate the location route, got %d", got)
	}
}
