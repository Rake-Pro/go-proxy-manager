package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestStripUntrustedIdentityUnderscoreSpellings: a header whose name, read with
// "_" as "-", is one the strip lists cover is removed under the same trust
// tier as its hyphen form. An unrelated underscore header is kept.
func TestStripUntrustedIdentityUnderscoreSpellings(t *testing.T) {
	always := []string{
		"X_Original_Url", "X_Rewrite_Url", "X_Http_Method_Override",
		"X_Forwarded_For", "X_Forwarded_Host", "X_Forwarded_Proto",
	}
	address := []string{
		"True_Client_Ip", "Cf_Connecting_Ip", "X_Real_Ip", "X_Forwarded_Port",
		"X-Forwarded-Prefix", "X_Forwarded_Prefix", "X-Original-Host", "X_Envoy_Original_Path",
		"X-Forwarded-Client-Cert", "Ssl_Client_Verify",
	}
	identity := []string{
		"Remote_User", "X_Forwarded_User", "X_Auth_Request_User", "X_Authentik_Username",
		"X_Client_Cert_Subject", "X_Client_Cert_Fingerprint", "X_Tenant_Subject",
	}
	cases := []struct {
		name                        string
		addrTrusted, identTrusted   bool
		wantAddress, wantIdentities bool
	}{
		{"untrusted peer", false, false, false, false},
		{"client-ip trusted peer", true, false, true, false},
		{"identity trusted peer", false, true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hh := &hostHandler{identityHeaders: []string{"X-Tenant-Subject"}}
			if tc.addrTrusted {
				hh.trustedProxies = mustNets("192.0.2.0/24")
			}
			if tc.identTrusted {
				hh.trustedNets = mustNets("192.0.2.0/24")
			}
			r := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)
			r.RemoteAddr = "192.0.2.10:4000"
			for _, group := range [][]string{always, address, identity} {
				for _, name := range group {
					r.Header.Set(name, "forged")
				}
			}
			r.Header.Set("X_Custom_Setting", "kept")
			r.Header.Set("X_Authentik_Csrf", "kept")
			hh.stripUntrustedIdentity(r)

			check := func(names []string, want bool) {
				for _, name := range names {
					if got := r.Header.Get(name) != ""; got != want {
						t.Errorf("%s present = %v, want %v", name, got, want)
					}
				}
			}
			check(always, false)
			check(address, tc.wantAddress)
			check(identity, tc.wantIdentities)
			check([]string{"X_Custom_Setting", "X_Authentik_Csrf"}, true)
		})
	}
}

// TestForwardedFamilyHeadersStripped: the forwarded-family names (scheme,
// port, prefix, path, host, client address, front-proxy client certificate) do
// not reach the upstream from a peer that is not a trusted proxy, and a peer in
// trustedProxies still forwards every one of them, through the real router.
func TestForwardedFamilyHeadersStripped(t *testing.T) {
	t.Run("untrusted peer", func(t *testing.T) { forwardedFamilyCase(t, false) })
	t.Run("trusted proxy", func(t *testing.T) {
		withGlobalTrustedProxies(t, "127.0.0.0/8")
		forwardedFamilyCase(t, true)
	})
}

func forwardedFamilyCase(t *testing.T, trusted bool) {
	var mu sync.Mutex
	var seen http.Header
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = r.Header.Clone()
		mu.Unlock()
	}))
	defer closeFn()
	rt, err := buildRouter(model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   up,
	}}}, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	addr := listenWithPolicy(t, http.HandlerFunc(rt.serveHTTP))

	stripped := []string{
		"X-Forwarded-Port", "X-Forwarded-Prefix", "X-Forwarded-Scheme", "X-Forwarded-Ssl",
		"X-Forwarded-Server", "Front-End-Https", "X-Url-Scheme",
		"X-Forwarded-Tls-Client-Cert", "X-Forwarded-Tls-Client-Cert-Info", "X-Forwarded-Client-Cert",
		"Ssl-Client-Cert", "Ssl-Client-Verify", "X-Ssl-Client-Dn",
		"X-Original-Host", "X-Host", "X-Forwarded-Path", "X-Envoy-Original-Path",
		"X-Original-Forwarded-For", "X-Cluster-Client-Ip", "Fastly-Client-Ip", "X-Azure-Clientip",
		"X_Forwarded_Prefix", "X_Original_Host",
	}
	// Dropped whatever the peer: an identity header (no identity trust here)
	// and the underscore spelling of a name the proxy rewrites itself.
	dropped := []string{"Remote_User", "X_Client_Cert_Subject", "X_Forwarded_For"}
	var b strings.Builder
	b.WriteString("GET / HTTP/1.1\r\nHost: app.example.com\r\nConnection: close\r\nX_Custom_Setting: kept\r\n")
	for _, name := range append(append([]string(nil), stripped...), dropped...) {
		b.WriteString(name + ": forged\r\n")
	}
	b.WriteString("\r\n")
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := rawExchange(t, c, b.String()); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, name := range stripped {
		if got := seen.Get(name) == "forged"; got != trusted {
			t.Errorf("upstream received %s = %v, want %v", name, got, trusted)
		}
	}
	for _, name := range dropped {
		if seen.Get(name) == "forged" {
			t.Errorf("upstream received %s", name)
		}
	}
	if seen.Get("X_Custom_Setting") != "kept" {
		t.Errorf("an unrelated underscore header was dropped")
	}
}
