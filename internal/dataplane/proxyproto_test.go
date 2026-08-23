package dataplane

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// ppConfig compiles a trusted-peer list for the tests.
func ppConfig(t *testing.T, timeout time.Duration, cidrs ...string) *proxyProtoConfig {
	t.Helper()
	c := &proxyProtoConfig{timeout: timeout}
	for _, s := range cidrs {
		n := parseNet(s)
		if n == nil {
			t.Fatalf("bad test CIDR %q", s)
		}
		c.trusted = append(c.trusted, n)
	}
	return c
}

// proxyProtoExchange writes wire to a real TCP connection wrapped by
// proxyProtoConn and returns the resulting client address plus the payload the
// caller would see after the header was stripped.
func proxyProtoExchange(t *testing.T, cfg *proxyProtoConfig, wire []byte) (net.Addr, []byte, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type accepted struct {
		c   net.Conn
		err error
	}
	ch := make(chan accepted, 1)
	go func() {
		c, err := ln.Accept()
		ch <- accepted{c, err}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	go func() {
		_, _ = client.Write(wire)
		_ = client.(*net.TCPConn).CloseWrite()
	}()

	a := <-ch
	if a.err != nil {
		t.Fatal(a.err)
	}
	defer a.c.Close()

	pc := &proxyProtoConn{Conn: a.c, cfg: cfg}
	payload, readErr := io.ReadAll(pc)
	if readErr != nil {
		return nil, nil, readErr
	}
	return pc.RemoteAddr(), payload, nil
}

// v2Header assembles a PROXY protocol v2 header with the given command/family
// and address block (TLVs may be appended to addr).
func v2Header(verCmd, family byte, addr []byte) []byte {
	h := append([]byte{}, v2Signature...)
	h = append(h, verCmd, family, 0, 0)
	binary.BigEndian.PutUint16(h[14:16], uint16(len(addr)))
	return append(h, addr...)
}

func v2IPv4Addr(src, dst net.IP, sport, dport uint16) []byte {
	b := make([]byte, 12)
	copy(b[0:4], src.To4())
	copy(b[4:8], dst.To4())
	binary.BigEndian.PutUint16(b[8:10], sport)
	binary.BigEndian.PutUint16(b[10:12], dport)
	return b
}

func v2IPv6Addr(src, dst net.IP, sport, dport uint16) []byte {
	b := make([]byte, 36)
	copy(b[0:16], src.To16())
	copy(b[16:32], dst.To16())
	binary.BigEndian.PutUint16(b[32:34], sport)
	binary.BigEndian.PutUint16(b[34:36], dport)
	return b
}

// TestProxyProtocolParseVectors covers both wire formats end to end: the client
// address each header asserts, the payload that must survive untouched, and the
// malformed shapes that have to fail (and close) rather than be guessed at.
func TestProxyProtocolParseVectors(t *testing.T) {
	trusted := func() *proxyProtoConfig { return ppConfig(t, 2*time.Second, "127.0.0.0/8", "::1/128") }

	cases := []struct {
		name    string
		wire    []byte
		want    string // expected client IP; "" = the real peer stands
		payload string
		wantErr bool
	}{
		{
			name:    "v1 tcp4",
			wire:    []byte("PROXY TCP4 203.0.113.7 198.51.100.1 51234 443\r\nhello"),
			want:    "203.0.113.7",
			payload: "hello",
		},
		{
			name:    "v1 tcp6",
			wire:    []byte("PROXY TCP6 2001:db8::1 2001:db8::2 51234 443\r\nhello"),
			want:    "2001:db8::1",
			payload: "hello",
		},
		{
			name:    "v1 unknown keeps the peer",
			wire:    []byte("PROXY UNKNOWN\r\npayload"),
			want:    "",
			payload: "payload",
		},
		{
			name:    "v2 tcp4",
			wire:    append(v2Header(0x21, 0x11, v2IPv4Addr(net.ParseIP("203.0.113.9"), net.ParseIP("198.51.100.2"), 4321, 443)), []byte("body")...),
			want:    "203.0.113.9",
			payload: "body",
		},
		{
			name:    "v2 tcp6",
			wire:    append(v2Header(0x21, 0x21, v2IPv6Addr(net.ParseIP("2001:db8::abcd"), net.ParseIP("2001:db8::1"), 4321, 443)), []byte("body")...),
			want:    "2001:db8::abcd",
			payload: "body",
		},
		{
			name: "v2 tlvs are consumed and ignored",
			wire: append(v2Header(0x21, 0x11, append(
				v2IPv4Addr(net.ParseIP("203.0.113.10"), net.ParseIP("198.51.100.3"), 1234, 443),
				// one TLV: type 0x03 (CRC32C), length 4, value
				0x03, 0x00, 0x04, 0xde, 0xad, 0xbe, 0xef,
			)), []byte("after-tlv")...),
			want:    "203.0.113.10",
			payload: "after-tlv",
		},
		{
			name:    "v2 LOCAL keeps the peer",
			wire:    append(v2Header(0x20, 0x00, nil), []byte("healthcheck")...),
			want:    "",
			payload: "healthcheck",
		},
		{
			name:    "v2 AF_UNSPEC keeps the peer",
			wire:    append(v2Header(0x21, 0x00, nil), []byte("x")...),
			want:    "",
			payload: "x",
		},
		{
			name:    "v1 with too few fields",
			wire:    []byte("PROXY TCP4 203.0.113.7 198.51.100.1\r\n"),
			wantErr: true,
		},
		{
			name:    "v1 with a bad source address",
			wire:    []byte("PROXY TCP4 not-an-ip 198.51.100.1 1 2\r\n"),
			wantErr: true,
		},
		{
			name:    "v1 tcp4 carrying an IPv6 source",
			wire:    []byte("PROXY TCP4 2001:db8::1 198.51.100.1 1 2\r\n"),
			wantErr: true,
		},
		{
			name:    "v1 tcp6 carrying an IPv4 source",
			wire:    []byte("PROXY TCP6 203.0.113.7 2001:db8::2 1 2\r\n"),
			wantErr: true,
		},
		{
			name:    "v1 with an out-of-range port",
			wire:    []byte("PROXY TCP4 203.0.113.7 198.51.100.1 70000 443\r\n"),
			wantErr: true,
		},
		{
			name:    "v1 with no CRLF before the length limit",
			wire:    []byte("PROXY TCP4 " + strings.Repeat("9", 120)),
			wantErr: true,
		},
		{
			name:    "v2 with a truncated IPv4 address block",
			wire:    v2Header(0x21, 0x11, make([]byte, 6)),
			wantErr: true,
		},
		{
			name:    "v2 with an unsupported version",
			wire:    v2Header(0x31, 0x11, v2IPv4Addr(net.ParseIP("203.0.113.7"), net.ParseIP("198.51.100.1"), 1, 2)),
			wantErr: true,
		},
		{
			name:    "v2 with an unsupported address family",
			wire:    v2Header(0x21, 0x31, make([]byte, 216)),
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, payload, err := proxyProtoExchange(t, trusted(), tc.wire)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected a malformed header to fail, got addr=%v payload=%q", addr, payload)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(payload) != tc.payload {
				t.Fatalf("payload = %q, want %q", payload, tc.payload)
			}
			gotIP := addrIP(addr)
			if tc.want == "" {
				if gotIP == nil || !gotIP.IsLoopback() {
					t.Fatalf("client IP = %v, want the real (loopback) peer", gotIP)
				}
				return
			}
			if gotIP == nil || !gotIP.Equal(net.ParseIP(tc.want)) {
				t.Fatalf("client IP = %v, want %s", gotIP, tc.want)
			}
		})
	}
}

// TestProxyProtocolUntrustedPeerPassthrough proves the header is an
// unauthenticated claim: from a peer outside trustedCIDRs it is NOT parsed, the
// bytes are delivered verbatim as payload, and the peer address stands. Without
// this, any client could assert any source IP and walk through every IP-based
// control gpm has.
func TestProxyProtocolUntrustedPeerPassthrough(t *testing.T) {
	wire := []byte("PROXY TCP4 203.0.113.7 198.51.100.1 51234 443\r\nhello")
	// Loopback (the test peer) is deliberately NOT in the trusted set.
	addr, payload, err := proxyProtoExchange(t, ppConfig(t, 2*time.Second, "198.51.100.0/24"), wire)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != string(wire) {
		t.Fatalf("payload = %q, want the header bytes delivered verbatim", payload)
	}
	if ip := addrIP(addr); ip == nil || !ip.IsLoopback() {
		t.Fatalf("client IP = %v, want the untouched loopback peer", ip)
	}
}

// TestProxyProtocolTrustedPeerWithoutHeader proves a trusted peer that opens a
// bare connection (a load-balancer health check) is served normally rather than
// dropped, with its own address as the client IP.
func TestProxyProtocolTrustedPeerWithoutHeader(t *testing.T) {
	addr, payload, err := proxyProtoExchange(t, ppConfig(t, 2*time.Second, "127.0.0.0/8"), []byte("GET / HTTP/1.0\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "GET / HTTP/1.0\r\n\r\n" {
		t.Fatalf("payload = %q, want it delivered verbatim", payload)
	}
	if ip := addrIP(addr); ip == nil || !ip.IsLoopback() {
		t.Fatalf("client IP = %v, want the loopback peer", ip)
	}
}

// TestProxyProtocolStalledHeaderTimesOut proves a trusted peer that opens a
// connection and never writes the header is cut off at the configured deadline
// instead of pinning a goroutine and a file descriptor indefinitely.
func TestProxyProtocolStalledHeaderTimesOut(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type accepted struct{ c net.Conn }
	ch := make(chan accepted, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			ch <- accepted{c}
		}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	a := <-ch
	defer a.c.Close()

	pc := &proxyProtoConn{Conn: a.c, cfg: ppConfig(t, 150*time.Millisecond, "127.0.0.0/8")}
	start := time.Now()
	if _, err := pc.Read(make([]byte, 16)); err == nil {
		t.Fatal("a stalled header must fail the read, not hang")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("read returned after %v, want the ~150ms header deadline", elapsed)
	}
	// The error is sticky and the connection is closed, so no half-parsed header
	// can leak through as payload on a retry.
	if _, err := pc.Read(make([]byte, 16)); err == nil {
		t.Fatal("the header error must be sticky")
	}
}

// TestProxyProtocolRemoteAddrReachesTheProxiedRequest is the end-to-end proof
// that the parsed source becomes the client IP everywhere: the access list
// evaluates it (deny for the real peer, allow for the asserted client) and the
// upstream sees it in X-Forwarded-For.
func TestProxyProtocolRemoteAddrReachesTheProxiedRequest(t *testing.T) {
	var xff string
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xff = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer closeFn()

	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "only-the-real-client"},
			DefaultAction: model.ActionDeny,
			Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "203.0.113.0/24"}},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"pp.example.com"},
			Upstream:    up,
			AccessLists: []string{"only-the-real-client"},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := wrapProxyProtocol(ln, func() *proxyProtoConfig {
		return ppConfig(t, 2*time.Second, "127.0.0.0/8")
	})
	srv := &http.Server{Handler: http.HandlerFunc(rt.serveHTTP), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(wrapped) }()
	defer srv.Close()

	request := "GET / HTTP/1.1\r\nHost: pp.example.com\r\nConnection: close\r\n\r\n"

	// With a PROXY header from the trusted peer the access list sees the real
	// client and lets it through.
	status, err := rawHTTP(t, ln.Addr().String(), "PROXY TCP4 203.0.113.7 198.51.100.1 51234 443\r\n"+request)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (the asserted client is inside the allowed range)", status)
	}
	if xff != "203.0.113.7" {
		t.Fatalf("X-Forwarded-For = %q, want the PROXY-protocol source 203.0.113.7", xff)
	}

	// Without one, the connection peer (loopback) is the client and is denied.
	status, err = rawHTTP(t, ln.Addr().String(), request)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for the loopback peer", status)
	}
}

// rawHTTP writes a literal request (optionally preceded by a PROXY header) and
// returns the response status.
func rawHTTP(t *testing.T, addr, wire string) (int, error) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, wire); err != nil {
		return 0, err
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// TestCompileProxyProtocol proves the compile step is fail-safe: off unless
// explicitly enabled, and never "trust everyone" when the CIDR list is unusable.
func TestCompileProxyProtocol(t *testing.T) {
	if compileProxyProtocol(nil) != nil {
		t.Error("nil settings must leave the feature off")
	}
	if compileProxyProtocol(&model.ProxyProtocolSettings{TrustedCIDRs: []string{"10.0.0.0/8"}}) != nil {
		t.Error("enabled:false must leave the feature off")
	}
	if compileProxyProtocol(&model.ProxyProtocolSettings{Enabled: true}) != nil {
		t.Error("no trusted CIDRs must leave the feature off, never trust every peer")
	}
	if compileProxyProtocol(&model.ProxyProtocolSettings{Enabled: true, TrustedCIDRs: []string{"nonsense"}}) != nil {
		t.Error("only unparseable CIDRs must leave the feature off")
	}
	c := compileProxyProtocol(&model.ProxyProtocolSettings{Enabled: true, TrustedCIDRs: []string{"10.0.0.0/8", "192.0.2.1"}, Timeout: "2s"})
	if c == nil || len(c.trusted) != 2 || c.timeout != 2*time.Second {
		t.Fatalf("compiled config = %+v, want 2 nets and a 2s timeout", c)
	}
	def := compileProxyProtocol(&model.ProxyProtocolSettings{Enabled: true, TrustedCIDRs: []string{"10.0.0.0/8"}})
	if def == nil || def.timeout != model.DefaultProxyProtocolTimeout {
		t.Fatalf("unset timeout must fall back to %s", model.DefaultProxyProtocolTimeout)
	}
}
