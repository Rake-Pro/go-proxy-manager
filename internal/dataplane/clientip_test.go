package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// withGlobalTrustedProxies installs a fleet-wide settings.trustedProxies list
// for one test and restores the trust-nobody default afterwards, so the package
// global never leaks between tests.
func withGlobalTrustedProxies(t *testing.T, cidrs ...string) {
	t.Helper()
	SetTrustedProxies(cidrs)
	t.Cleanup(func() { SetTrustedProxies(nil) })
}

// TestDeriveClientIP is the table for THE client-IP derivation: the
// rightmost-untrusted walk, the trust-nobody default, and the forged-header case
// that every allowFrom exemption depends on.
func TestDeriveClientIP(t *testing.T) {
	cases := []struct {
		name        string
		trusted     []string
		remote      string
		xff         []string
		want        string
		wantTrusted bool
	}{
		{
			name:   "no trust configured: peer is the client",
			remote: "203.0.113.7:5000",
			want:   "203.0.113.7",
		},
		{
			name:   "no trust configured: a forged chain is not read at all",
			remote: "203.0.113.7:5000",
			xff:    []string{"10.1.2.3"},
			want:   "203.0.113.7",
		},
		{
			name:        "trusted peer, one hop",
			trusted:     []string{"192.0.2.10/32"},
			remote:      "192.0.2.10:443",
			xff:         []string{"198.51.100.4"},
			want:        "198.51.100.4",
			wantTrusted: true,
		},
		{
			name:        "trusted peer, two hops: rightmost untrusted wins",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"198.51.100.4, 192.0.2.20"},
			want:        "198.51.100.4",
			wantTrusted: true,
		},
		{
			name:        "trusted peer, three hops with a client-prepended fake",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"10.9.9.9, 198.51.100.4, 192.0.2.20"},
			want:        "198.51.100.4",
			wantTrusted: true,
		},
		{
			name:        "trusted peer, chain of trusted proxies only: peer stands",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"192.0.2.20, 192.0.2.30"},
			want:        "192.0.2.10",
			wantTrusted: true,
		},
		{
			name:        "trusted peer, empty chain: peer stands",
			trusted:     []string{"192.0.2.10/32"},
			remote:      "192.0.2.10:443",
			want:        "192.0.2.10",
			wantTrusted: true,
		},
		{
			name:    "untrusted peer with a spoofed chain: chain ignored",
			trusted: []string{"192.0.2.10/32"},
			remote:  "203.0.113.7:5000",
			xff:     []string{"10.1.2.3, 192.0.2.10"},
			want:    "203.0.113.7",
		},
		{
			name:        "chain split across two header lines",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"198.51.100.4", "192.0.2.20"},
			want:        "198.51.100.4",
			wantTrusted: true,
		},
		{
			name:        "entry carrying a port is still parsed",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"198.51.100.4:51234"},
			want:        "198.51.100.4",
			wantTrusted: true,
		},
		{
			name:        "ipv6 peer and ipv6 client",
			trusted:     []string{"2001:db8:1::/48"},
			remote:      "[2001:db8:1::10]:443",
			xff:         []string{"2001:db8:99::5"},
			want:        "2001:db8:99::5",
			wantTrusted: true,
		},
		{
			name:        "ipv6 bracketed entry with a port",
			trusted:     []string{"2001:db8:1::/48"},
			remote:      "[2001:db8:1::10]:443",
			xff:         []string{"[2001:db8:99::5]:51234"},
			want:        "2001:db8:99::5",
			wantTrusted: true,
		},
		{
			name:    "ipv6 client cannot spoof from an untrusted peer",
			trusted: []string{"2001:db8:1::/48"},
			remote:  "[2001:db8:ff::9]:443",
			xff:     []string{"2001:db8:1::10"},
			want:    "2001:db8:ff::9",
		},
		{
			name:   "unparseable RemoteAddr yields no client IP",
			remote: "garbage",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			got := deriveClientIP(r, mustNets(tc.trusted...))
			gotIP := ""
			if got.ip != nil {
				gotIP = got.ip.String()
			}
			if gotIP != tc.want {
				t.Fatalf("client ip = %q, want %q", gotIP, tc.want)
			}
			if got.peerTrusted != tc.wantTrusted {
				t.Fatalf("peerTrusted = %v, want %v", got.peerTrusted, tc.wantTrusted)
			}
		})
	}
}

// TestDeriveClientIPAfterProxyProtocol proves the L4 and L7 tiers compose: the
// PROXY header rewrites RemoteAddr, and the rewritten address is then what the
// trusted-proxy test is applied to - so a balancer that fronts an L7 proxy still
// ends up with the browser's address.
func TestDeriveClientIPAfterProxyProtocol(t *testing.T) {
	// The L4 balancer (the real TCP peer, 127.0.0.1) asserts that the connection
	// came from 192.0.2.10 - which is the L7 proxy gpm trusts for XFF.
	addr, _, err := proxyProtoExchange(t, ppConfig(t, time.Second, "127.0.0.0/8"),
		[]byte("PROXY TCP4 192.0.2.10 198.51.100.1 1111 443\r\n"))
	if err != nil {
		t.Fatalf("proxy protocol exchange: %v", err)
	}
	if addr == nil {
		t.Fatal("no client address from the PROXY header")
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = addr.String()
	r.Header.Set("X-Forwarded-For", "198.51.100.77")

	got := deriveClientIP(r, mustNets("192.0.2.10/32"))
	if !got.peerTrusted {
		t.Fatalf("the PROXY-asserted source %s must be tested against trustedProxies", addr)
	}
	if got.ip == nil || got.ip.String() != "198.51.100.77" {
		t.Fatalf("client ip = %v, want the X-Forwarded-For client 198.51.100.77", got.ip)
	}

	// The same header from a peer OUTSIDE proxyProtocol.trustedCIDRs asserts
	// nothing, so the L7 tier never sees a trusted peer either.
	addr2, _, err := proxyProtoExchange(t, ppConfig(t, time.Second, "10.0.0.0/8"),
		[]byte("PROXY TCP4 192.0.2.10 198.51.100.1 1111 443\r\n"))
	if err != nil {
		t.Fatalf("proxy protocol exchange: %v", err)
	}
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = addr2.String()
	r2.Header.Set("X-Forwarded-For", "198.51.100.77")
	if got := deriveClientIP(r2, mustNets("192.0.2.10/32")); got.peerTrusted {
		t.Fatal("an untrusted L4 peer must not be able to claim an L7-trusted source address")
	}
}

// TestHostTrustedProxiesOverride: a proxy host's own trustedProxies REPLACES
// settings.trustedProxies for that host, and a host that declares none inherits
// the fleet-wide list.
func TestHostTrustedProxiesOverride(t *testing.T) {
	withGlobalTrustedProxies(t, "10.0.0.0/8")

	inherits := model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "inherits"}}
	overrides := model.ProxyHost{
		ObjectMeta:     model.ObjectMeta{Name: "overrides"},
		TrustedProxies: &[]string{"192.0.2.10/32"},
	}

	req := func(remote, xff string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", xff)
		return r
	}
	ipOf := func(h model.ProxyHost, r *http.Request) string {
		ip := deriveClientIP(r, hostTrustedProxies(h)).ip
		if ip == nil {
			return ""
		}
		return ip.String()
	}

	if got := ipOf(inherits, req("10.0.0.1:1", "198.51.100.4")); got != "198.51.100.4" {
		t.Fatalf("a host with no override must inherit settings.trustedProxies, got %q", got)
	}
	if got := ipOf(overrides, req("10.0.0.1:1", "198.51.100.4")); got != "10.0.0.1" {
		t.Fatalf("the override must REPLACE the fleet list, so 10/8 is no longer trusted; got %q", got)
	}
	if got := ipOf(overrides, req("192.0.2.10:443", "198.51.100.4")); got != "198.51.100.4" {
		t.Fatalf("the override's own CIDR must be trusted, got %q", got)
	}
}

// certOrLANRouter compiles one host gated by a client-cert auth middleware whose
// allowFrom exempts the LAN, with the given per-host trustedProxies. The host has
// no tls.clientAuth, so requests are served in the clear and the middleware alone
// decides - which is exactly the shape the mTLS-bypass warning is about.
func certOrLANRouter(t *testing.T, hostTrusted *[]string) *router {
	t.Helper()
	up, closeFn := backendUpstream(t, okHandler())
	t.Cleanup(closeFn)
	cfg := model.Config{
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "cert-or-lan"},
			Type:       model.MWTypeAuth,
			Auth: &model.AuthMiddleware{
				Mode:      model.AuthModeClientCert,
				AllowFrom: []string{"10.0.0.0/8"},
			},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:     model.ObjectMeta{Name: "app"},
			Domains:        []string{"app.example.com"},
			Upstream:       up,
			Middlewares:    []string{"cert-or-lan"},
			TrustedProxies: hostTrusted,
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	return rt
}

func serveApp(rt *router, remote, xff string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rt.serveHTTP(rec, req)
	return rec.Code
}

// TestAllowFromUsesDerivedClientIP is the documented mTLS/allowFrom trap,
// asserted from both ends: with gpm behind a DECLARED trusted proxy the
// exemption follows the forwarded client, and a spoofed X-Forwarded-For from any
// other peer is refused rather than exempted.
func TestAllowFromUsesDerivedClientIP(t *testing.T) {
	rt := certOrLANRouter(t, &[]string{"192.0.2.10/32"})

	cases := []struct {
		name   string
		remote string
		xff    string
		want   int
	}{
		{"trusted proxy forwards a LAN client", "192.0.2.10:443", "10.1.2.3", http.StatusOK},
		{"trusted proxy forwards a WAN client", "192.0.2.10:443", "203.0.113.5", http.StatusUnauthorized},
		{"untrusted peer cannot spoof the LAN", "203.0.113.5:443", "10.1.2.3", http.StatusUnauthorized},
		{"LAN peer with no proxy in front", "10.1.2.3:5000", "", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := serveApp(rt, tc.remote, tc.xff); got != tc.want {
				t.Fatalf("status %d, want %d", got, tc.want)
			}
		})
	}
}

// TestAllowFromWithNoTrustedProxiesIgnoresXFF: the default (no trustedProxies
// anywhere) is trust-nobody, so an L7 proxy in front of gpm cannot place a
// client inside the exemption by asserting a header.
func TestAllowFromWithNoTrustedProxiesIgnoresXFF(t *testing.T) {
	rt := certOrLANRouter(t, nil)
	if got := serveApp(rt, "192.0.2.10:443", "10.1.2.3"); got != http.StatusUnauthorized {
		t.Fatalf("with no trusted proxies a forwarded LAN address must not exempt, got %d", got)
	}
}

// TestAllowFromUsesGlobalTrustedProxies proves the fleet-wide setting reaches a
// host that declares no override of its own.
func TestAllowFromUsesGlobalTrustedProxies(t *testing.T) {
	withGlobalTrustedProxies(t, "192.0.2.10/32")
	rt := certOrLANRouter(t, nil)
	if got := serveApp(rt, "192.0.2.10:443", "10.1.2.3"); got != http.StatusOK {
		t.Fatalf("settings.trustedProxies must apply to a host with no override, got %d", got)
	}
	if got := serveApp(rt, "203.0.113.5:443", "10.1.2.3"); got != http.StatusUnauthorized {
		t.Fatalf("a peer outside settings.trustedProxies must not be believed, got %d", got)
	}
}

// TestRateLimitKeysOnDerivedClientIP: the throttle bucket is keyed on the same
// derived address. Behind a trusted proxy two forwarded clients get their own
// buckets; from an untrusted peer a rotating X-Forwarded-For shares one bucket,
// so a flood cannot escape the limit by cycling a header.
func TestRateLimitKeysOnDerivedClientIP(t *testing.T) {
	newRouter := func(t *testing.T) *router {
		t.Helper()
		up, closeFn := backendUpstream(t, okHandler())
		t.Cleanup(closeFn)
		cfg := model.Config{ProxyHosts: []model.ProxyHost{{
			ObjectMeta:     model.ObjectMeta{Name: "app"},
			Domains:        []string{"app.example.com"},
			Upstream:       up,
			TrustedProxies: &[]string{"192.0.2.10/32"},
			RateLimit:      &model.RateLimitMiddleware{Requests: 1, Window: "1m", Burst: 1},
		}}}
		rt, err := buildRouter(cfg, "", nil)
		if err != nil {
			t.Fatalf("buildRouter: %v", err)
		}
		return rt
	}

	t.Run("trusted proxy: separate buckets per forwarded client", func(t *testing.T) {
		rt := newRouter(t)
		if got := serveApp(rt, "192.0.2.10:443", "198.51.100.1"); got != http.StatusOK {
			t.Fatalf("first request from client A: %d", got)
		}
		if got := serveApp(rt, "192.0.2.10:443", "198.51.100.1"); got != http.StatusTooManyRequests {
			t.Fatalf("second request from client A must be throttled, got %d", got)
		}
		if got := serveApp(rt, "192.0.2.10:443", "198.51.100.2"); got != http.StatusOK {
			t.Fatalf("client B must have its own bucket, got %d", got)
		}
	})

	t.Run("untrusted peer: one bucket, header rotation does not escape it", func(t *testing.T) {
		rt := newRouter(t)
		if got := serveApp(rt, "203.0.113.9:5000", "198.51.100.1"); got != http.StatusOK {
			t.Fatalf("first request: %d", got)
		}
		if got := serveApp(rt, "203.0.113.9:5000", "198.51.100.2"); got != http.StatusTooManyRequests {
			t.Fatalf("a rotated X-Forwarded-For from an untrusted peer must share the peer's bucket, got %d", got)
		}
	})
}

// TestForwardedHeadersSentUpstream: the upstream sees the same address the gates
// compared. From an untrusted peer the client's own chain is replaced (a backend
// reading the leftmost entry must not be handed a forged one); behind a trusted
// proxy the genuine chain is preserved and extended.
func TestForwardedHeadersSentUpstream(t *testing.T) {
	var gotXFF, gotReal string
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFF = r.Header.Get("X-Forwarded-For")
		gotReal = r.Header.Get("X-Real-Ip")
		w.WriteHeader(http.StatusOK)
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta:     model.ObjectMeta{Name: "app"},
		Domains:        []string{"app.example.com"},
		Upstream:       up,
		TrustedProxies: &[]string{"192.0.2.10/32"},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	if code := serveApp(rt, "203.0.113.9:5000", "10.1.2.3"); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if gotXFF != "203.0.113.9" {
		t.Fatalf("X-Forwarded-For = %q, want the peer only: a forged chain from an untrusted peer must not be forwarded", gotXFF)
	}
	if gotReal != "203.0.113.9" {
		t.Fatalf("X-Real-Ip = %q, want 203.0.113.9", gotReal)
	}

	if code := serveApp(rt, "192.0.2.10:443", "198.51.100.4"); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if gotXFF != "198.51.100.4, 192.0.2.10" {
		t.Fatalf("X-Forwarded-For = %q, want the genuine chain plus the trusted peer", gotXFF)
	}
	if gotReal != "198.51.100.4" {
		t.Fatalf("X-Real-Ip = %q, want the derived client 198.51.100.4", gotReal)
	}
}

// TestAccessLogClientIPMatchesTheGates: the address written to the access log is
// the one the gates compared, including behind a per-host trusted proxy.
func TestAccessLogClientIPMatchesTheGates(t *testing.T) {
	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()
	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta:     model.ObjectMeta{Name: "app"},
		Domains:        []string{"app.example.com"},
		Upstream:       up,
		TrustedProxies: &[]string{"192.0.2.10/32"},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	s := &Server{}
	s.cur.Store(rt)

	req := func(remote, xff string) *http.Request {
		r := httptest.NewRequest("GET", "http://app.example.com/", nil)
		r.Host = "app.example.com"
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", xff)
		return r
	}
	if got := s.clientIPOf(req("192.0.2.10:443", "198.51.100.4")); got == nil || got.String() != "198.51.100.4" {
		t.Fatalf("access log client = %v, want the derived 198.51.100.4", got)
	}
	if got := s.clientIPOf(req("203.0.113.9:5000", "198.51.100.4")); got == nil || got.String() != "203.0.113.9" {
		t.Fatalf("access log client = %v, want the peer 203.0.113.9 (forged header)", got)
	}
}

// TestCompileTrustedProxiesDropsUnparseable: model validation rejects a bad
// entry at write time, so one can only reach the compiler through a config that
// bypassed it - and there it must trust LESS, never more.
func TestCompileTrustedProxiesDropsUnparseable(t *testing.T) {
	nets := compileTrustedProxies("test", []string{"192.0.2.10/32", "not-an-ip", "0.0.0.0/0"})
	if len(nets) != 2 {
		t.Fatalf("compiled %d nets, want 2 (the unparseable entry dropped)", len(nets))
	}
	if !ipInNets(net.ParseIP("203.0.113.1"), nets) {
		t.Fatal("0.0.0.0/0 is accepted (warned, not dropped)")
	}
}
