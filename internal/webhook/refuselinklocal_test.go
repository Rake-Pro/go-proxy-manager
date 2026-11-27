package webhook

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// refuseLinkLocal reads the destination the dialer hands to Control. A zone is
// dropped before the address is parsed (an IPv4-mapped literal with a zone
// arrives as "169.254.169.254%eth0"), and an address that still does not parse
// is refused rather than let through. A unix socket path is not a network
// address and passes. The guard is called with address strings only; nothing
// here dials a link-local address.
func TestRefuseLinkLocalFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		network, address string
		refused          bool
	}{
		{"tcp4", "169.254.169.254%eth0:80", true}, // [::ffff:169.254.169.254%25eth0] as Control sees it
		{"tcp6", "[::ffff:169.254.169.254%eth0]:80", true},
		{"tcp6", "[fe80::1%eth0]:80", true},
		{"tcp6", "[ff02::1%eth0]:80", true},
		{"tcp4", "169.254.169.254:80", true},
		{"tcp6", "[fe80::1]:80", true},
		{"tcp4", "224.0.0.251:5353", true},
		{"tcp", "example.com:443", true}, // never a resolved destination
		{"tcp", ":80", false},            // a hostless URL: net dials the local system
		{"tcp", "example.com", true},
		{"tcp", "", true},
		{"tcp4", "127.0.0.1:80", false},
		{"tcp6", "[::1]:80", false},
		{"tcp4", "192.0.2.10:443", false},
		{"tcp4", "198.51.100.7:80", false},
		{"tcp6", "[2001:db8::1%eth0]:443", false},
		{"tcp4", "192.0.2.10%eth0:443", false}, // [::ffff:192.0.2.10%25eth0]: the zone is not the address
		{"unix", "/var/run/docker.sock", false},
	} {
		if err := refuseLinkLocal(tc.network, tc.address, nil); (err != nil) != tc.refused {
			t.Errorf("refuseLinkLocal(%q, %q) = %v, want refused %v", tc.network, tc.address, err, tc.refused)
		}
	}

	// A real dial through the guard still connects: loopback, on a listener
	// of this test.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, err := ln.Accept(); err == nil {
			c.Close()
		}
	}()
	var seen string
	d := net.Dialer{Control: func(network, address string, c syscall.RawConn) error {
		seen = address
		return refuseLinkLocal(network, address, c)
	}}
	c, err := d.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial through the guard to %s (Control saw %q): %v", ln.Addr(), seen, err)
	}
	c.Close()
}

// The DNS provider warning for allowInsecureLocal (model.Config.Warnings)
// names exactly the URL literals this guard refuses. Each literal is dialed
// with a Control that records the address the dialer hands over and then
// aborts, so no connection is ever attempted.
func TestDialRefusedWarningMatchesGuard(t *testing.T) {
	for _, raw := range []string{
		"http://169.254.169.254:80/", "http://[fe80::1%25eth0]:80/", "http://[FE80::1%25eth0]:80/",
		"http://[::ffff:169.254.1.1]:80/", "http://[::ffff:a9fe:a9fe]:80/", "http://[::ffff:169.254.169.254%25eth0]:80/",
		"http://[0:0:0:0:0:ffff:a9fe:101]:80/", "http://224.0.0.251:80/", "http://[ff02::fb]:80/", "http://[ff12::1]:80/",
		"http://[ff02::1%25eth0]:80/", "http://127.0.0.1:80/", "http://[::1]:80/", "http://0.0.0.0:80/", "http://[::a9fe:a9fe]:80/",
		"http://[::ffff:127.0.0.1]:80/", "http://169.254.0.0:80/", "http://169.254.255.255:80/", "http://[febf::1]:80/",
		"http://[fec0::1]:80/", "http://192.0.2.10:80/", "http://[2001:db8::1%25eth0]:80/", "http://[::ffff:192.0.2.10%25eth0]:80/",
		"http://:80/", "https://:8443/hook?x=1",
	} {
		cfg := model.Config{DNSProviders: []model.DNSProvider{{ObjectMeta: model.ObjectMeta{Name: "p"}, Provider: model.DNSProviderWebhook,
			Config: map[string]model.Secret{"url": model.Secret(raw), "token": "t", "allowInsecureLocal": "true"}}}}
		warned := false
		for _, w := range cfg.Warnings() {
			warned = warned || strings.HasPrefix(w, `dns provider "p": config.url points at the link-local address`)
		}
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		var refused bool
		var seen string
		d := net.Dialer{Control: func(network, address string, _ syscall.RawConn) error {
			seen = address
			refused = refuseLinkLocal(network, address, nil) != nil
			return errors.New("aborted before connect")
		}}
		_, _ = d.Dial("tcp", u.Host)
		if seen == "" {
			t.Fatalf("%s: Control never ran", raw)
		}
		if warned != refused {
			t.Errorf("%s: warned %v, guard refuses %v (Control saw %q)", raw, warned, refused, seen)
		}
	}
}

// A hostless URL ("http://:port/") dials the local system, as before the
// guard read every address: the shared client reaches a loopback receiver.
func TestHostlessURLConnects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewSecureClient(5 * time.Second).Get("http://:" + port + "/hook")
	if err != nil {
		t.Fatalf("GET http://:%s/hook: %v", port, err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", res.StatusCode)
	}
}
