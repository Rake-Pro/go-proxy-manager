package clientip

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestSetTrustedAndTrusted(t *testing.T) {
	if got := Trusted(); got != nil {
		t.Fatalf("Trusted() before any SetTrusted = %v, want nil (trust-nobody default)", got)
	}

	SetTrusted([]string{"192.0.2.0/24"})
	t.Cleanup(func() { SetTrusted(nil) })

	got := Trusted()
	if len(got) != 1 {
		t.Fatalf("Trusted() after SetTrusted = %v, want one network", got)
	}
	if !got[0].Contains(net.ParseIP("192.0.2.10")) {
		t.Fatalf("Trusted() network %v does not contain 192.0.2.10", got[0])
	}

	// A second SetTrusted call replaces the installed set rather than
	// appending to it.
	SetTrusted([]string{"198.51.100.0/24"})
	got = Trusted()
	if len(got) != 1 || got[0].Contains(net.ParseIP("192.0.2.10")) || !got[0].Contains(net.ParseIP("198.51.100.10")) {
		t.Fatalf("Trusted() after replace = %v, want only 198.51.100.0/24", got)
	}

	SetTrusted(nil)
	if got := Trusted(); got != nil {
		t.Fatalf("Trusted() after SetTrusted(nil) = %v, want nil", got)
	}
}

func TestCompile(t *testing.T) {
	cases := []struct {
		name  string
		cidrs []string
		want  []string // net.IPNet.String() for each compiled entry, in order
	}{
		{name: "empty", cidrs: nil, want: nil},
		{name: "CIDR", cidrs: []string{"10.0.0.0/8"}, want: []string{"10.0.0.0/8"}},
		{name: "bare IPv4 becomes /32", cidrs: []string{"192.0.2.10"}, want: []string{"192.0.2.10/32"}},
		{name: "bare IPv6 becomes /128", cidrs: []string{"2001:db8::1"}, want: []string{"2001:db8::1/128"}},
		{
			name:  "malformed entries are dropped, not fatal",
			cidrs: []string{"not-an-ip", "10.0.0.0/8", "", "999.999.999.999"},
			want:  []string{"10.0.0.0/8"},
		},
		{
			name:  "wildcard is accepted (and only warned about)",
			cidrs: []string{"0.0.0.0/0"},
			want:  []string{"0.0.0.0/0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Compile("settings.trustedProxies", tc.cidrs)
			if len(got) != len(tc.want) {
				t.Fatalf("Compile(%v) = %v, want %v", tc.cidrs, got, tc.want)
			}
			for i, n := range got {
				if n.String() != tc.want[i] {
					t.Errorf("Compile(%v)[%d] = %q, want %q", tc.cidrs, i, n.String(), tc.want[i])
				}
			}
		})
	}
}

func TestParseNet(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" means nil
	}{
		{name: "CIDR v4", in: "10.0.0.0/8", want: "10.0.0.0/8"},
		{name: "CIDR v6", in: "2001:db8::/32", want: "2001:db8::/32"},
		{name: "bare IPv4", in: "192.0.2.10", want: "192.0.2.10/32"},
		{name: "bare IPv6", in: "::1", want: "::1/128"},
		{name: "malformed", in: "not-an-ip", want: ""},
		{name: "empty", in: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseNet(tc.in)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("ParseNet(%q) = %v, want nil", tc.in, got)
				}
				return
			}
			if got == nil || got.String() != tc.want {
				t.Fatalf("ParseNet(%q) = %v, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestInNets(t *testing.T) {
	nets := Compile("test", []string{"192.0.2.0/24", "2001:db8::/32"})
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "in first net", ip: "192.0.2.55", want: true},
		{name: "in second net", ip: "2001:db8::1", want: true},
		{name: "outside both", ip: "198.51.100.1", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := InNets(net.ParseIP(tc.ip), nets); got != tc.want {
				t.Errorf("InNets(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}

	// An empty/nil net list never matches anything.
	if InNets(net.ParseIP("192.0.2.1"), nil) {
		t.Error("InNets with nil nets = true, want false")
	}
}

func TestPeerIP(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		want       string // "" means nil
	}{
		{name: "IPv4 with port", remoteAddr: "203.0.113.7:5000", want: "203.0.113.7"},
		{name: "IPv6 with port", remoteAddr: "[2001:db8::1]:5000", want: "2001:db8::1"},
		{name: "bare IPv4, no port", remoteAddr: "203.0.113.7", want: "203.0.113.7"},
		{name: "bare IPv6, no port", remoteAddr: "2001:db8::1", want: "2001:db8::1"},
		{name: "IPv4-mapped IPv6 with port", remoteAddr: "[::ffff:203.0.113.7]:5000", want: "203.0.113.7"},
		{name: "malformed", remoteAddr: "not-an-address", want: ""},
		{name: "empty", remoteAddr: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			got := PeerIP(r)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("PeerIP(%q) = %v, want nil", tc.remoteAddr, got)
				}
				return
			}
			if got == nil || !got.Equal(net.ParseIP(tc.want)) {
				t.Fatalf("PeerIP(%q) = %v, want %s", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

func TestParseForwardedEntry(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" means nil (unparseable/rejected)
	}{
		{name: "bare IPv4", in: "192.0.2.1", want: "192.0.2.1"},
		{name: "IPv4 with port", in: "192.0.2.1:443", want: "192.0.2.1"},
		{name: "bracketed IPv6 with port", in: "[2001:db8::1]:443", want: "2001:db8::1"},
		{name: "bare IPv6", in: "2001:db8::1", want: "2001:db8::1"},
		{name: "bracketed IPv6 without port", in: "[2001:db8::1]", want: "2001:db8::1"},
		// net.ParseIP does not strip IPv6 zone identifiers, so a zone-id
		// suffixed entry is unparseable here - exactly like any other token
		// gpm cannot read, it ends the Derive walk rather than being trusted.
		{name: "IPv6 zone id is unparseable, not silently accepted", in: "fe80::1%eth0", want: ""},
		{name: "whitespace padded", in: "  192.0.2.1  ", want: "192.0.2.1"},
		{name: "unparseable", in: "not-an-ip", want: ""},
		{name: "empty", in: "", want: ""},
		{name: "unspecified IPv4 rejected", in: "0.0.0.0", want: ""},
		{name: "unspecified IPv6 rejected", in: "::", want: ""},
		{name: "broadcast rejected", in: "255.255.255.255", want: ""},
		{name: "multicast IPv4 rejected", in: "224.0.0.1", want: ""},
		{name: "multicast IPv6 rejected", in: "ff02::1", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseForwardedEntry(tc.in)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("ParseForwardedEntry(%q) = %v, want nil", tc.in, got)
				}
				return
			}
			if got == nil || !got.Equal(net.ParseIP(tc.want)) {
				t.Fatalf("ParseForwardedEntry(%q) = %v, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestForwardedChain(t *testing.T) {
	t.Run("single header, comma separated", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Forwarded-For", "198.51.100.1, 192.0.2.10, 192.0.2.20")
		got := ForwardedChain(r)
		want := []string{"198.51.100.1", "192.0.2.10", "192.0.2.20"}
		if len(got) != len(want) {
			t.Fatalf("ForwardedChain = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("ForwardedChain[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("multiple header lines concatenate in order", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Add("X-Forwarded-For", "198.51.100.1")
		r.Header.Add("X-Forwarded-For", "192.0.2.10, 192.0.2.20")
		got := ForwardedChain(r)
		want := []string{"198.51.100.1", "192.0.2.10", "192.0.2.20"}
		if len(got) != len(want) {
			t.Fatalf("ForwardedChain = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("ForwardedChain[%d] = %q, want %q", i, got[i], want[i])
			}
		}
		// The rightmost hop (closest to gpm) is the last element.
		if got[len(got)-1] != "192.0.2.20" {
			t.Errorf("rightmost hop = %q, want 192.0.2.20", got[len(got)-1])
		}
	})

	t.Run("no header yields nil", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if got := ForwardedChain(r); got != nil {
			t.Errorf("ForwardedChain with no header = %v, want nil", got)
		}
	})

	t.Run("empty and whitespace-only elements are dropped", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Forwarded-For", "198.51.100.1,, ,192.0.2.10")
		got := ForwardedChain(r)
		want := []string{"198.51.100.1", "192.0.2.10"}
		if len(got) != len(want) {
			t.Fatalf("ForwardedChain = %v, want %v", got, want)
		}
	})

	t.Run("bound at MaxForwardedEntries, keeping the rightmost", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		var hdr string
		for i := 0; i < MaxForwardedEntries+10; i++ {
			if i > 0 {
				hdr += ", "
			}
			hdr += net.IPv4(10, 0, byte(i>>8), byte(i)).String()
		}
		r.Header.Set("X-Forwarded-For", hdr)
		got := ForwardedChain(r)
		if len(got) != MaxForwardedEntries {
			t.Fatalf("ForwardedChain len = %d, want %d (MaxForwardedEntries)", len(got), MaxForwardedEntries)
		}
		// The kept entries are the last MaxForwardedEntries of the 74 total,
		// i.e. indices 10..73 -> the final one is index 73.
		wantLast := net.IPv4(10, 0, byte(73>>8), byte(73)).String()
		if got[len(got)-1] != wantLast {
			t.Errorf("last kept entry = %q, want %q", got[len(got)-1], wantLast)
		}
	})
}

// TestDerive is THE client-IP derivation table: the rightmost-untrusted walk,
// the trust-nobody default, and the forged-header case every allowFrom
// exemption depends on.
func TestDerive(t *testing.T) {
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
			name:        "untrusted peer: XFF ignored even if trusted list is non-empty",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "203.0.113.7:5000",
			xff:         []string{"10.1.2.3"},
			want:        "203.0.113.7",
			wantTrusted: false,
		},
		{
			name:        "trusted peer, zero hops in XFF: falls back to peer",
			trusted:     []string{"192.0.2.10/32"},
			remote:      "192.0.2.10:443",
			want:        "192.0.2.10",
			wantTrusted: true,
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
			name:        "trusted peer, three hops through two trusted proxies",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"198.51.100.4, 192.0.2.30, 192.0.2.20"},
			want:        "198.51.100.4",
			wantTrusted: true,
		},
		{
			name:        "trusted peer, every hop trusted: falls back to peer",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"192.0.2.30, 192.0.2.20"},
			want:        "192.0.2.10",
			wantTrusted: true,
		},
		{
			name:        "unparseable rightmost entry ends the walk at the peer",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"198.51.100.4, not-an-ip"},
			want:        "192.0.2.10",
			wantTrusted: true,
		},
		{
			name:        "IPv6 zone-id entry is unparseable and ends the walk",
			trusted:     []string{"192.0.2.0/24"},
			remote:      "192.0.2.10:443",
			xff:         []string{"198.51.100.4, fe80::1%eth0"},
			want:        "192.0.2.10",
			wantTrusted: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nets := Compile("test", tc.trusted)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for _, x := range tc.xff {
				r.Header.Add("X-Forwarded-For", x)
			}
			ip, trusted := Derive(r, nets)
			if ip == nil || !ip.Equal(net.ParseIP(tc.want)) {
				t.Errorf("Derive() ip = %v, want %s", ip, tc.want)
			}
			if trusted != tc.wantTrusted {
				t.Errorf("Derive() trusted = %v, want %v", trusted, tc.wantTrusted)
			}
		})
	}
}

func TestKey(t *testing.T) {
	t.Run("derived IP forms the key", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "203.0.113.7:5000"
		if got, want := Key(r, nil), "203.0.113.7"; got != want {
			t.Errorf("Key() = %q, want %q", got, want)
		}
	})

	t.Run("trusted peer's forwarded hop forms the key", func(t *testing.T) {
		nets := Compile("test", []string{"192.0.2.10/32"})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "192.0.2.10:443"
		r.Header.Set("X-Forwarded-For", "198.51.100.4")
		if got, want := Key(r, nets), "198.51.100.4"; got != want {
			t.Errorf("Key() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to raw RemoteAddr when no address can be derived", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "not-an-address"
		if got, want := Key(r, nil), "not-an-address"; got != want {
			t.Errorf("Key() = %q, want %q", got, want)
		}
	})
}

// TestConcurrentSetTrustedAndTrusted exercises SetTrusted/Trusted under -race:
// the installed set is an atomic pointer swap, so concurrent readers must never
// observe a torn value and the race detector must stay silent.
func TestConcurrentSetTrustedAndTrusted(t *testing.T) {
	t.Cleanup(func() { SetTrusted(nil) })

	const iterations = 500
	var wg sync.WaitGroup

	// Writers keep swapping the installed set.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cidr := []string{"192.0.2.0/24"}
			if n%2 == 0 {
				cidr = []string{"198.51.100.0/24"}
			}
			for j := 0; j < iterations; j++ {
				SetTrusted(cidr)
			}
		}(i)
	}

	// Readers just observe whatever is currently installed.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = Trusted()
			}
		}()
	}

	wg.Wait()
}
