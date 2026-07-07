package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func mustNets(cidrs ...string) []*net.IPNet {
	var ns []*net.IPNet
	for _, c := range cidrs {
		if n := parseNet(c); n != nil {
			ns = append(ns, n)
		}
	}
	return ns
}

func TestClientIPResolver(t *testing.T) {
	trusted := mustNets("10.0.0.0/8")
	resolve := clientIPResolver(trusted)

	cases := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"edge peer, no trust", "203.0.113.7:5000", "", "203.0.113.7"},
		{"untrusted peer ignores forged xff", "203.0.113.7:5000", "9.9.9.9", "203.0.113.7"},
		{"trusted peer honours xff", "10.1.2.3:5000", "9.9.9.9", "9.9.9.9"},
		{"trusted chain takes nearest untrusted", "10.1.2.3:5000", "9.9.9.9, 10.4.4.4", "9.9.9.9"},
		{"trusted peer, empty xff falls back to peer", "10.1.2.3:5000", "", "10.1.2.3"},
		{"garbage remote -> nil", "garbage", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			got := resolve(r)
			gotStr := ""
			if got != nil {
				gotStr = got.String()
			}
			if gotStr != tc.want {
				t.Fatalf("got %q, want %q", gotStr, tc.want)
			}
		})
	}
}

// forwardAuthHost builds a host gated by a forward-auth IdP (trusted 10/8) and
// returns its compiled hostHandler so the per-host strip can be exercised.
func forwardAuthHost(t *testing.T) *hostHandler {
	t.Helper()
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
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "sso"},
			Type:       model.MWTypeAuth,
			Auth:       &model.AuthMiddleware{IdentityProvider: "authentik", Mode: model.AuthModeForwardAuth},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"app2.example.com"},
			Upstream:    model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
			Middlewares: []string{"sso"},
		}},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	hh, ok := rt.hosts["app2.example.com"]
	if !ok {
		t.Fatal("compiled host not found")
	}
	return hh
}

func TestStripUntrustedIdentity(t *testing.T) {
	hh := forwardAuthHost(t)

	// Untrusted peer: forged identity headers are stripped.
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:5000"
	r.Header.Set("X-Authentik-Username", "attacker")
	r.Header.Set("X-Authentik-Groups", "proxy-admins")
	hh.stripUntrustedIdentity(r)
	if r.Header.Get("X-Authentik-Username") != "" || r.Header.Get("X-Authentik-Groups") != "" {
		t.Fatalf("forged identity headers must be stripped from an untrusted peer, got %v", r.Header)
	}

	// Trusted proxy: its asserted identity headers survive for the chain to read.
	r = httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:5000"
	r.Header.Set("X-Authentik-Username", "admin")
	hh.stripUntrustedIdentity(r)
	if r.Header.Get("X-Authentik-Username") != "admin" {
		t.Fatal("a trusted proxy's identity headers must be preserved")
	}
}

// TestStripBaselineIdentityNoProviders proves the baseline denylist is stripped
// from an untrusted peer even when the host configures no IdP, while a benign
// header is left intact.
func TestStripBaselineIdentityNoProviders(t *testing.T) {
	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()
	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app2.example.com"},
		Upstream:   up,
	}}}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	hh := rt.hosts["app2.example.com"]

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:5000"
	r.Header.Set("Remote-User", "attacker")
	r.Header.Set("X-Auth-Request-User", "attacker")
	r.Header.Set("X-Authentik-Groups", "proxy-admins")
	r.Header.Set("X-Custom-App", "keep-me")
	// Authentik's own CSRF header matches the X-Authentik- strip prefix but is not
	// an identity assertion; it must survive so a proxied Authentik can validate it
	// against the authentik_csrf cookie (else every login fails "CSRF token missing").
	r.Header.Set("X-authentik-CSRF", "csrf-token-value")
	hh.stripUntrustedIdentity(r)
	for _, h := range []string{"Remote-User", "X-Auth-Request-User", "X-Authentik-Groups"} {
		if r.Header.Get(h) != "" {
			t.Fatalf("baseline header %q must be stripped from an untrusted peer with no IdP", h)
		}
	}
	if r.Header.Get("X-Custom-App") != "keep-me" {
		t.Fatal("non-identity header must be preserved")
	}
	if r.Header.Get("X-authentik-CSRF") != "csrf-token-value" {
		t.Fatal("Authentik CSRF header must be preserved through the identity strip")
	}
}

// TestIdentityTrustNotSharedAcrossHosts proves a proxy trusted by host A's IdP is
// not implicitly trusted to assert identity headers to host B (no global union).
func TestIdentityTrustNotSharedAcrossHosts(t *testing.T) {
	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "authentik"},
			Type:        model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-Authentik-Username"},
		}},
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "sso"},
			Type:       model.MWTypeAuth,
			Auth:       &model.AuthMiddleware{IdentityProvider: "authentik", Mode: model.AuthModeForwardAuth},
		}},
		ProxyHosts: []model.ProxyHost{
			{ObjectMeta: model.ObjectMeta{Name: "a"}, Domains: []string{"a.example.com"}, Upstream: model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}, Middlewares: []string{"sso"}},
			{ObjectMeta: model.ObjectMeta{Name: "b"}, Domains: []string{"b.example.com"}, Upstream: model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}},
		},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	// The 10/8 proxy is trusted for host A (its IdP) but NOT for host B.
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:5000"
	r.Header.Set("X-Authentik-Username", "admin")
	rt.hosts["b.example.com"].stripUntrustedIdentity(r)
	if r.Header.Get("X-Authentik-Username") != "" {
		t.Fatal("host B must not trust host A's IdP proxy to assert identity")
	}
}

func TestAccessListTrustedProxyXFF(t *testing.T) {
	// Client-IP resolution for an access list honours X-Forwarded-For only from a
	// proxy THIS host trusts, and that trust is per-host: the trusted-proxy set is
	// the forward-auth TrustedProxies of the IdPs the host references, not a global
	// union across every IdP (GPM-L4).
	//
	// Two hosts share the allow-only-9.9.9.9 access list. Host "app-auth" references
	// a forward-auth IdP trusting 10/8, so 10/8 is its trusted proxy; host "app-open"
	// references no IdP, so it trusts no proxy. The access list runs ahead of auth
	// (GPM-L1), so its verdict is visible as 403 (denied) vs a later 401 (allowed by
	// the list, then rejected by auth for lack of identity).
	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()

	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "authentik"},
			Type:        model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
		}},
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "sso"},
			Type:       model.MWTypeAuth,
			Auth:       &model.AuthMiddleware{IdentityProvider: "authentik", Mode: model.AuthModeForwardAuth},
		}},
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "only-bob"},
			DefaultAction: model.ActionDeny,
			Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "9.9.9.9/32"}},
		}},
		ProxyHosts: []model.ProxyHost{
			{
				ObjectMeta:  model.ObjectMeta{Name: "app-auth"},
				Domains:     []string{"auth.example.com"},
				Upstream:    up,
				Middlewares: []string{"sso"},
				AccessLists: []string{"only-bob"},
			},
			{
				ObjectMeta:  model.ObjectMeta{Name: "app-open"},
				Domains:     []string{"open.example.com"},
				Upstream:    up,
				AccessLists: []string{"only-bob"},
			},
		},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	serve := func(host, remote, xff string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "https://"+host+"/", nil)
		req.Host = host
		req.RemoteAddr = remote
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rt.serveHTTPS(rec, req)
		return rec.Code
	}

	// app-auth trusts 10/8: XFF=9.9.9.9 from that proxy resolves to the allowed
	// client, passing the access list (then 401 at auth for lack of identity).
	if code := serve("auth.example.com", "10.0.0.1:1", "9.9.9.9"); code != http.StatusUnauthorized {
		t.Fatalf("trusted-proxy XFF should pass the access list (then 401 at auth), got %d", code)
	}
	// Forged XFF from an untrusted peer is ignored; the peer itself is denied.
	if code := serve("auth.example.com", "203.0.113.1:1", "9.9.9.9"); code != http.StatusForbidden {
		t.Fatalf("forged XFF from untrusted peer must be denied, got %d", code)
	}
	// app-open trusts no proxy: the SAME 10.0.0.1+XFF=9.9.9.9 is no longer honoured
	// (no global union), so the peer 10.0.0.1 is denied by the access list.
	if code := serve("open.example.com", "10.0.0.1:1", "9.9.9.9"); code != http.StatusForbidden {
		t.Fatalf("host with no trusted proxy must ignore XFF and deny the peer, got %d", code)
	}
}
