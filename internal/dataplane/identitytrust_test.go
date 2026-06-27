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

func forwardAuthRouter(t *testing.T) *router {
	t.Helper()
	cfg := model.Config{IdentityProviders: []model.IdentityProvider{{
		ObjectMeta: model.ObjectMeta{Name: "authentik"},
		Type:       model.IdPTypeForwardAuth,
		ForwardAuth: &model.ForwardAuthSpec{
			TrustedProxies: []string{"10.0.0.0/8"},
			UserHeader:     "X-Authentik-Username",
			GroupsHeader:   "X-Authentik-Groups",
		},
	}}}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	return rt
}

func TestStripUntrustedIdentity(t *testing.T) {
	rt := forwardAuthRouter(t)

	// Untrusted peer: forged identity headers are stripped.
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:5000"
	r.Header.Set("X-Authentik-Username", "attacker")
	r.Header.Set("X-Authentik-Groups", "proxy-admins")
	rt.stripUntrustedIdentity(r)
	if r.Header.Get("X-Authentik-Username") != "" || r.Header.Get("X-Authentik-Groups") != "" {
		t.Fatalf("forged identity headers must be stripped from an untrusted peer, got %v", r.Header)
	}

	// Trusted proxy: its asserted identity headers survive for the chain to read.
	r = httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:5000"
	r.Header.Set("X-Authentik-Username", "admin")
	rt.stripUntrustedIdentity(r)
	if r.Header.Get("X-Authentik-Username") != "admin" {
		t.Fatal("a trusted proxy's identity headers must be preserved")
	}
}

func TestStripUntrustedIdentityNoProvidersIsNoop(t *testing.T) {
	rt, err := buildRouter(model.Config{}, "")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Authentik-Username", "x")
	rt.stripUntrustedIdentity(r)
	if r.Header.Get("X-Authentik-Username") != "x" {
		t.Fatal("with no identity providers configured, nothing should be stripped")
	}
}

func TestAccessListTrustedProxyXFF(t *testing.T) {
	// A host with a forward-auth IdP (trusted 10/8) and an access list allowing
	// only 9.9.9.9. A request from the trusted proxy carrying XFF=9.9.9.9 is
	// allowed; the same allow-list rejects a forged XFF from an untrusted peer.
	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()

	cfg := model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta:  model.ObjectMeta{Name: "authentik"},
			Type:        model.IdPTypeForwardAuth,
			ForwardAuth: &model.ForwardAuthSpec{TrustedProxies: []string{"10.0.0.0/8"}, UserHeader: "X-User"},
		}},
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "only-bob"},
			DefaultAction: model.ActionDeny,
			Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "9.9.9.9/32"}},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"app2.example.com"},
			Upstream:    up,
			AccessLists: []string{"only-bob"},
		}},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	serve := func(remote, xff string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "https://app2.example.com/", nil)
		req.Host = "app2.example.com"
		req.RemoteAddr = remote
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rt.serveHTTPS(rec, req)
		return rec.Code
	}

	if code := serve("10.0.0.1:1", "9.9.9.9"); code != http.StatusOK {
		t.Fatalf("trusted proxy forwarding allowed client should pass, got %d", code)
	}
	if code := serve("203.0.113.1:1", "9.9.9.9"); code != http.StatusForbidden {
		t.Fatalf("forged XFF from untrusted peer must be denied, got %d", code)
	}
}
