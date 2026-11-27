package dnssync

import (
	"net"
	"syscall"
	"testing"
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
