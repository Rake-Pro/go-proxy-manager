package dataplane

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// inlinePages compiles an inline error-page set for a test.
func inlinePages(t *testing.T, entries map[string]string) *compiledErrorPages {
	t.Helper()
	ep, err := compileErrorPages(model.ErrorPagesConfig{Inline: entries}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	return ep
}

// okNext is a backend that must not be reached by a refused request.
func okNext(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a refused request reached the upstream")
		w.WriteHeader(http.StatusOK)
	})
}

// trustedFA is a forward-auth compiled to trust 10.0.0.0/8 and read one header.
func trustedFA() auth.ForwardAuth {
	return auth.CompileForwardAuth(model.ForwardAuthSpec{
		TrustedProxies: []string{"10.0.0.0/8"},
		UserHeader:     "X-Auth-User",
		GroupsHeader:   "X-Auth-Groups",
	}, "idp")
}

// TestAuthGateRefusalsUseErrorPages is the table for the whole feature: every
// terminal refusal an auth gate writes itself must render the configured page,
// and must fall back to today's exact plain-text body when nothing is
// configured. The gates are exercised through their real constructors, not a
// stand-in, so the plumbing is covered along with the rendering.
func TestAuthGateRefusalsUseErrorPages(t *testing.T) {
	_, caCert, caKey := crlCA(t, "corp-ca")
	_, ops := clientCertSerial(t, caCert, caKey, 200, "ops")
	withCert := func(r *http.Request) *http.Request {
		r.TLS = &tls.ConnectionState{
			ServerName:       "m.example",
			PeerCertificates: []*x509.Certificate{ops},
			VerifiedChains:   [][]*x509.Certificate{{ops, caCert}},
		}
		return r
	}
	// A forward-auth request from a trusted proxy asserting an unmapped group:
	// authenticated, but no role - the 403 branch.
	faForbidden := func() *http.Request {
		r := httptest.NewRequest("GET", "https://m.example/", nil)
		r.RemoteAddr = "10.1.2.3:5000"
		r.Header.Set("X-Auth-User", "someone")
		r.Header.Set("X-Auth-Groups", "nobody")
		return r
	}

	cases := []struct {
		name        string
		gate        func(ep *compiledErrorPages) http.Handler
		req         func() *http.Request
		wantStatus  int
		wantDefault string // exact plain-text body with no pages configured
	}{
		{
			name: "forward-auth 401 untrusted peer",
			gate: func(ep *compiledErrorPages) http.Handler {
				return forwardAuthGate(trustedFA(), nil, nil, "m", ep, okNext(t))
			},
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "https://m.example/", nil)
				r.RemoteAddr = "203.0.113.5:5000"
				r.Header.Set("X-Auth-User", "forged")
				return r
			},
			wantStatus:  http.StatusUnauthorized,
			wantDefault: "authentication required\n",
		},
		{
			name: "forward-auth 403 unmapped role",
			gate: func(ep *compiledErrorPages) http.Handler {
				rm := &model.RoleMapping{AdminGroups: []string{"admins"}}
				return forwardAuthGate(trustedFA(), rm, []string{"admin"}, "m", ep, okNext(t))
			},
			req:         faForbidden,
			wantStatus:  http.StatusForbidden,
			wantDefault: "forbidden\n",
		},
		{
			name: "client-cert 401 no certificate",
			gate: func(ep *compiledErrorPages) http.Handler {
				return clientCertGate(model.AuthMiddleware{Mode: model.AuthModeClientCert}, peerIP, nil, "m", ep, okNext(t))
			},
			req:         func() *http.Request { return httptest.NewRequest("GET", "https://m.example/", nil) },
			wantStatus:  http.StatusUnauthorized,
			wantDefault: "authentication required\n",
		},
		{
			name: "client-cert 403 unmapped subject",
			gate: func(ep *compiledErrorPages) http.Handler {
				return clientCertGate(model.AuthMiddleware{
					Mode:            model.AuthModeClientCert,
					RequiredRoles:   []string{"admin"},
					ClientCertRoles: map[string]string{"someone-else": "admin"},
				}, peerIP, nil, "m", ep, okNext(t))
			},
			req:         func() *http.Request { return withCert(httptest.NewRequest("GET", "https://m.example/", nil)) },
			wantStatus:  http.StatusForbidden,
			wantDefault: "forbidden\n",
		},
		{
			// Driven through the REAL authHandler, not a hand-built
			// dataOIDC: `gate.ep = ep` in auth.go is the single line wiring error
			// pages into the OIDC gate, and a test that constructs the gate itself
			// would keep passing with that line deleted. The refusal used here is
			// the request-Host check, which is the one OIDC terminal refusal that
			// needs no discovery and no network.
			name: "oidc 404 unserved request host (through authHandler)",
			gate: func(ep *compiledErrorPages) http.Handler {
				idp := model.IdentityProvider{
					ObjectMeta: model.ObjectMeta{Name: "sso"},
					Type:       model.IdPTypeOIDC,
					OIDC:       &model.OIDCSpec{IssuerURL: "https://idp.example", ClientID: "gpm"},
				}
				mw := model.Middleware{
					ObjectMeta: model.ObjectMeta{Name: "sso-gate"},
					Type:       model.MWTypeAuth,
					Auth:       &model.AuthMiddleware{Mode: model.AuthModeOIDC, IdentityProvider: "sso"},
				}
				reg := buildRegistry(model.Config{IdentityProviders: []model.IdentityProvider{idp}})
				return authHandler(*mw.Auth, reg, "m", []string{"m.example"}, peerIP, ep, okNext(t))
			},
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "https://other.example/", nil)
				r.Host = "other.example"
				return r
			},
			wantStatus:  http.StatusNotFound,
			wantDefault: "no such host\n",
		},
		{
			name: "failClosed 503",
			gate: func(ep *compiledErrorPages) http.Handler {
				return failClosed("m", "unknown identity provider", ep)
			},
			req:         func() *http.Request { return httptest.NewRequest("GET", "https://m.example/", nil) },
			wantStatus:  http.StatusServiceUnavailable,
			wantDefault: "authentication not available\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/plain-text fallback", func(t *testing.T) {
			withGlobalErrorPages(t, nil)
			rec := httptest.NewRecorder()
			tc.gate(nil).ServeHTTP(rec, tc.req())
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Body.String(); got != tc.wantDefault {
				t.Fatalf("body = %q, want the unchanged plain-text refusal %q", got, tc.wantDefault)
			}
			// Byte-identical to http.Error, headers included.
			if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
				t.Fatalf("Content-Type = %q, want the http.Error default", ct)
			}
			if x := rec.Header().Get("X-Content-Type-Options"); x != "nosniff" {
				t.Fatalf("X-Content-Type-Options = %q, want nosniff", x)
			}
		})

		t.Run(tc.name+"/status-specific page", func(t *testing.T) {
			withGlobalErrorPages(t, nil)
			ep := inlinePages(t, map[string]string{
				statusKey(tc.wantStatus): "<h1>{{.Status}} {{.StatusText}} on {{.Host}}</h1>",
			})
			rec := httptest.NewRecorder()
			tc.gate(ep).ServeHTTP(rec, tc.req())
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			want := "<h1>" + statusKey(tc.wantStatus) + " " + http.StatusText(tc.wantStatus) + " on m</h1>"
			if got := rec.Body.String(); got != want {
				t.Fatalf("body = %q, want %q", got, want)
			}
			// Same response shape the access-list denial produces.
			if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
				t.Fatalf("Content-Type = %q", ct)
			}
			if cl := rec.Header().Get("Content-Length"); cl == "" {
				t.Fatal("Content-Length must be set on a rendered error page")
			}
		})

		t.Run(tc.name+"/default template", func(t *testing.T) {
			withGlobalErrorPages(t, nil)
			ep := inlinePages(t, map[string]string{"default": "generic {{.Status}}"})
			rec := httptest.NewRecorder()
			tc.gate(ep).ServeHTTP(rec, tc.req())
			if got := rec.Body.String(); got != "generic "+statusKey(tc.wantStatus) {
				t.Fatalf("body = %q, want the default template to fill in", got)
			}
		})

		t.Run(tc.name+"/host override beats settings", func(t *testing.T) {
			withGlobalErrorPages(t, inlinePages(t, map[string]string{"default": "SETTINGS"}))
			// No host override: the settings-level page applies.
			rec := httptest.NewRecorder()
			tc.gate(nil).ServeHTTP(rec, tc.req())
			if got := rec.Body.String(); got != "SETTINGS" {
				t.Fatalf("body = %q, want the settings-level page for a host with no override", got)
			}
			// With one, it wins.
			rec2 := httptest.NewRecorder()
			tc.gate(inlinePages(t, map[string]string{"default": "HOST"})).ServeHTTP(rec2, tc.req())
			if got := rec2.Body.String(); got != "HOST" {
				t.Fatalf("body = %q, want the per-host override to win", got)
			}
		})
	}
}

// statusKey renders a status code as the string key an errorPages map uses.
func statusKey(status int) string { return strconv.Itoa(status) }

// TestForwardAuthStripsIdentityOnEveryRefusalPath proves the property the error
// page must not disturb: a refused request has its forged identity headers
// stripped and never reaches the upstream, on BOTH the plain-text and the
// rendered-page path. (The strip is also written ahead of the refusal in the
// source; that ordering is not observable here because refuse() never touches
// the request, so this asserts the outcome rather than the sequence.)
func TestForwardAuthStripsIdentityOnEveryRefusalPath(t *testing.T) {
	withGlobalErrorPages(t, nil)
	for _, tc := range []struct {
		name string
		ep   *compiledErrorPages
	}{
		{"plain text", nil},
		{"rendered page", inlinePages(t, map[string]string{"401": "<p>denied</p>"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen *http.Request
			h := forwardAuthGate(trustedFA(), nil, nil, "m", tc.ep,
				http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r }))
			r := httptest.NewRequest("GET", "https://m.example/", nil)
			r.RemoteAddr = "203.0.113.5:5000" // untrusted peer
			r.Header.Set("X-Auth-User", "forged")
			r.Header.Set("X-Auth-Groups", "admins")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)

			if seen != nil {
				t.Fatal("the refused request reached the upstream")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			for _, hd := range []string{"X-Auth-User", "X-Auth-Groups"} {
				if v := r.Header.Get(hd); v != "" {
					t.Fatalf("%s survived the refusal as %q; a refused request must never carry a forged identity onward", hd, v)
				}
			}
		})
	}
}

// TestAuthRequestErrorPagePrecedence covers the one place an error page must NOT
// apply: the outpost passthrough proxies the identity provider's own response,
// and the IdP owns its sign-in pages. It also proves the 403 deny path (which
// gpm generates itself) does render a page, and that the sign-in redirect is
// left alone.
func TestAuthRequestErrorPagePrecedence(t *testing.T) {
	withGlobalErrorPages(t, nil)
	ep := inlinePages(t, map[string]string{"default": "GPM PAGE"})

	t.Run("outpost passthrough keeps the IdP response", func(t *testing.T) {
		stub := &outpostStub{authStatus: http.StatusOK}
		srv := httptest.NewServer(stub.handler())
		defer srv.Close()
		h := newAuthReqProxy(t, srv.URL).handler(peerIP, nil, "app", ep, okNext(t))

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "http://app2.example.com/outpost.goauthentik.io/start?rd=/", nil))
		if !strings.HasPrefix(rec.Body.String(), "OUTPOST:") {
			t.Fatalf("the IdP's own response must win over the error page, got %q", rec.Body.String())
		}
	})

	t.Run("401 redirect is not an error page", func(t *testing.T) {
		stub := &outpostStub{authStatus: http.StatusUnauthorized}
		srv := httptest.NewServer(stub.handler())
		defer srv.Close()
		h := newAuthReqProxy(t, srv.URL).handler(peerIP, nil, "app", ep, okNext(t))

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "http://app2.example.com/", nil))
		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want the 302 sign-in redirect", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "GPM PAGE") {
			t.Fatal("a sign-in redirect must not render an error page")
		}
	})

	t.Run("403 deny renders the page", func(t *testing.T) {
		stub := &outpostStub{authStatus: http.StatusForbidden}
		srv := httptest.NewServer(stub.handler())
		defer srv.Close()
		h := newAuthReqProxy(t, srv.URL).handler(peerIP, nil, "app", ep, okNext(t))

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "http://app2.example.com/", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if got := rec.Body.String(); got != "GPM PAGE" {
			t.Fatalf("body = %q, want the configured error page", got)
		}
	})

	t.Run("backend failure renders the page", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close() // nothing listening: the subrequest fails
		h := newAuthReqProxy(t, url).handler(peerIP, nil, "app", ep, okNext(t))

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "http://app2.example.com/", nil))
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", rec.Code)
		}
		if got := rec.Body.String(); got != "GPM PAGE" {
			t.Fatalf("body = %q, want the configured error page", got)
		}
	})
}

// TestOIDCGateRefusalUsesErrorPages covers the OIDC gate's terminal refusals.
// The redirect into the IdP is a flow and stays untouched.
func TestOIDCGateRefusalUsesErrorPages(t *testing.T) {
	withGlobalErrorPages(t, nil)
	gate := &dataOIDC{
		hostName: "m",
		domains:  map[string]struct{}{"m.example": {}},
		clients:  map[string]*oidcClientEntry{},
		ep:       inlinePages(t, map[string]string{"404": "NOT HERE", "default": "GPM PAGE"}),
	}
	h := gate.handler(okNext(t))

	// A Host this gate does not serve: terminal 404, rendered.
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "https://other.example/", nil)
	r.Host = "other.example"
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := rec.Body.String(); got != "NOT HERE" {
		t.Fatalf("body = %q, want the status-specific page", got)
	}

	// With no pages configured the body is the unchanged plain text.
	plain := &dataOIDC{hostName: "m", domains: gate.domains, clients: map[string]*oidcClientEntry{}}
	rec2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "https://other.example/", nil)
	r2.Host = "other.example"
	plain.handler(okNext(t)).ServeHTTP(rec2, r2)
	if got := rec2.Body.String(); got != "no such host\n" {
		t.Fatalf("body = %q, want the unchanged plain-text refusal", got)
	}
}
