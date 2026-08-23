package dataplane

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// requireIPv6 skips a test on a host (or container) with no usable IPv6 stack.
func requireIPv6(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("no usable IPv6 loopback on this host: %v", err)
	}
	_ = ln.Close()
}

// TestListenerIsDualStack proves the data plane's bind addresses (":80"/":443",
// i.e. a bare ":port") accept BOTH families on one listener - which is what makes
// inbound IPv6 work with no extra listener and no second config knob. Go's
// net.Listen on a bare port binds the IPv6 wildcard with v4-mapped addresses
// enabled, so a v4 client arrives on the same socket.
func TestListenerIsDualStack(t *testing.T) {
	requireIPv6(t)
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("ok"))
			c.Close()
		}
	}()

	for _, addr := range []string{net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), net.JoinHostPort("::1", strconv.Itoa(port))} {
		c, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			t.Fatalf("dial %s: %v (a bare \":port\" bind must accept both families)", addr, err)
		}
		buf := make([]byte, 2)
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, err := io.ReadFull(c, buf); err != nil {
			t.Fatalf("read from %s: %v", addr, err)
		}
		c.Close()
	}
}

// TestProxiedRequestOverIPv6 proves an end-to-end proxied request arriving over
// IPv6 is served, that the access list evaluates the v6 client address (an
// allow-list of ::1 lets it in, and a v4-only list would not), and that the
// upstream sees that same v6 address in X-Forwarded-For.
func TestProxiedRequestOverIPv6(t *testing.T) {
	requireIPv6(t)

	var xff string
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xff = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer closeFn()

	cfg := model.Config{
		AccessLists: []model.AccessList{
			{
				ObjectMeta:    model.ObjectMeta{Name: "v6-loopback"},
				DefaultAction: model.ActionDeny,
				Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "::1/128"}},
			},
			{
				ObjectMeta:    model.ObjectMeta{Name: "v4-only"},
				DefaultAction: model.ActionDeny,
				Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "127.0.0.0/8"}},
			},
		},
		ProxyHosts: []model.ProxyHost{
			{
				ObjectMeta:  model.ObjectMeta{Name: "v6"},
				Domains:     []string{"v6.example.com"},
				Upstream:    up,
				AccessLists: []string{"v6-loopback"},
			},
			{
				ObjectMeta:  model.ObjectMeta{Name: "v4"},
				Domains:     []string{"v4.example.com"},
				Upstream:    up,
				AccessLists: []string{"v4-only"},
			},
		},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(rt.serveHTTP), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	base := "http://" + ln.Addr().String() + "/"
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", base, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "v6.example.com"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (the v6 client is inside ::1/128)", resp.StatusCode)
	}
	if ip := net.ParseIP(xff); ip == nil || ip.To4() != nil || !ip.IsLoopback() {
		t.Fatalf("X-Forwarded-For = %q, want the IPv6 client address ::1", xff)
	}

	// The same connection against a host whose list only allows IPv4 is denied,
	// proving the v6 address really is what the rules are matched against rather
	// than some v4-mapped stand-in.
	req, err = http.NewRequest("GET", base, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "v4.example.com"
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a v6 client against a v4-only access list", resp.StatusCode)
	}
}
