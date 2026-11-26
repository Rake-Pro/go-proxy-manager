package dataplane

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// halfConn is a client or backend socket whose write side can be shut alone.
type halfConn interface {
	net.Conn
	CloseWrite() error
}

// acceptingBackend listens on loopback and hands every accepted connection to
// the test, which drives the backend side by hand.
func acceptingBackend(t *testing.T) (string, <-chan *net.TCPConn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	conns := make(chan *net.TCPConn, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { c.Close() })
			conns <- c.(*net.TCPConn)
		}
	}()
	return ln.Addr().String(), conns
}

// fwdAddr is the forwarder's IPv4 loopback address (it listens on every family).
func fwdAddr(f *tcpForwarder) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(f.ln.Addr().(*net.TCPAddr).Port))
}

func nextBackend(t *testing.T, conns <-chan *net.TCPConn) *net.TCPConn {
	t.Helper()
	select {
	case c := <-conns:
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("the forwarder never dialled the backend")
		return nil
	}
}

// slotsHeld is the total of the forwarder's maxConnsPerSource counters.
func slotsHeld(f *tcpForwarder) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, v := range f.perSource {
		n += v
	}
	return n
}

// waitSlots polls until the forwarder holds want slots, failing after 5 s.
func waitSlots(t *testing.T, f *tcpForwarder, want int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); slotsHeld(f) != want; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("forwarder holds %d connection slots, want %d", slotsHeld(f), want)
		}
	}
}

func setLinger(t *testing.T, d time.Duration) {
	t.Helper()
	prev := streamHalfCloseLinger
	streamHalfCloseLinger = d
	t.Cleanup(func() { streamHalfCloseLinger = prev })
}

func readAll(t *testing.T, c net.Conn, who string) string {
	t.Helper()
	b, err := io.ReadAll(c)
	if err != nil {
		t.Fatalf("%s: reading to EOF: %v (got %q)", who, err, b)
	}
	return string(b)
}

// halfCloseScenarios drive one connection through the forwarder from both
// ends: client is the socket the test dialled, backend the one gpm dialled.
var halfCloseScenarios = []struct {
	name string
	run  func(t *testing.T, client halfConn, backend *net.TCPConn)
}{
	{"client half-closes, server replies and closes", func(t *testing.T, client halfConn, backend *net.TCPConn) {
		if _, err := client.Write([]byte("request")); err != nil {
			t.Fatal(err)
		}
		if err := client.CloseWrite(); err != nil {
			t.Fatal(err)
		}
		if got := readAll(t, backend, "backend"); got != "request" {
			t.Fatalf("backend got %q", got)
		}
		if _, err := backend.Write([]byte("response")); err != nil {
			t.Fatal(err)
		}
		_ = backend.Close()
		if got := readAll(t, client, "client"); got != "response" {
			t.Fatalf("client got %q, want the reply sent after its half-close", got)
		}
	}},
	{"server half-closes first, client keeps sending", func(t *testing.T, client halfConn, backend *net.TCPConn) {
		if _, err := backend.Write([]byte("banner")); err != nil {
			t.Fatal(err)
		}
		if err := backend.CloseWrite(); err != nil {
			t.Fatal(err)
		}
		if got := readAll(t, client, "client"); got != "banner" {
			t.Fatalf("client got %q", got)
		}
		if _, err := client.Write([]byte("late")); err != nil {
			t.Fatal(err)
		}
		if err := client.CloseWrite(); err != nil {
			t.Fatal(err)
		}
		if got := readAll(t, backend, "backend"); got != "late" {
			t.Fatalf("backend got %q, want what the client sent after the server half-closed", got)
		}
	}},
	{"both half-close at once", func(t *testing.T, client halfConn, backend *net.TCPConn) {
		for _, c := range []halfConn{client, backend} {
			if _, err := c.Write([]byte("bye")); err != nil {
				t.Fatal(err)
			}
			if err := c.CloseWrite(); err != nil {
				t.Fatal(err)
			}
		}
		if got := readAll(t, backend, "backend"); got != "bye" {
			t.Fatalf("backend got %q", got)
		}
		if got := readAll(t, client, "client"); got != "bye" {
			t.Fatalf("client got %q", got)
		}
	}},
}

// A TCP stream forwards a half-close in either direction: the side that is
// done sending shuts its write side, and the other direction keeps flowing
// until it is done too. The connection slot is released once both are done.
func TestTCPStreamHalfClose(t *testing.T) {
	setLinger(t, time.Hour) // a pair that only ends by the linger bound fails waitSlots
	pp := func() *proxyProtoConfig { return ppConfig(t, 2*time.Second, "127.0.0.0/8") }
	variants := []struct {
		name   string
		target streamTarget
		pp     func() *proxyProtoConfig
		header string
	}{
		{name: "maxConnsPerSource", target: streamTarget{maxPerSource: 4}},
		{name: "idleTimeout", target: streamTarget{idle: time.Minute, maxPerSource: 4}},
		{name: "proxy protocol", target: streamTarget{maxPerSource: 4}, pp: pp, header: "PROXY TCP4 192.0.2.10 192.0.2.1 50000 443\r\n"},
	}
	for _, v := range variants {
		for _, sc := range halfCloseScenarios {
			t.Run(v.name+"/"+sc.name, func(t *testing.T) {
				addr, conns := acceptingBackend(t)
				target := v.target
				target.name, target.addr = "s", addr
				f, err := startTCPForwarder(0, &streamRoutes{def: &target}, v.pp, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(f.stop)
				c, err := net.Dial("tcp", fwdAddr(f))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { c.Close() })
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				if v.header != "" {
					if _, err := c.Write([]byte(v.header)); err != nil {
						t.Fatal(err)
					}
				}
				sc.run(t, c.(*net.TCPConn), nextBackend(t, conns))
				waitSlots(t, f, 0)
			})
		}
	}
}

// terminateForwarder starts a terminate-mode forwarder for stream.example.com
// in front of the backend at addr, with maxConnsPerSource set so tests can
// wait for its slots.
func terminateForwarder(t *testing.T, addr string) *tcpForwarder {
	t.Helper()
	dir := t.TempDir()
	writeSelfSigned(t, dir, "stream.example.com")
	cfg := model.Config{
		Certificates: []model.Certificate{{
			ObjectMeta: model.ObjectMeta{Name: "stream-cert"},
			Type:       model.CertTypeCustom,
			Domains:    []string{"stream.example.com"},
			Custom:     &model.CustomCertSpec{CertFile: "cert.pem", KeyFile: "key.pem"},
		}},
		StreamHosts: []model.StreamHost{streamHostFor("secure", addr, 4433, &model.StreamTLS{
			Mode:           model.StreamTLSTerminate,
			SNIMatch:       []string{"stream.example.com"},
			CertificateRef: "stream-cert",
		})},
	}
	certs, err := buildCertResolver(cfg.Certificates, dir)
	if err != nil {
		t.Fatal(err)
	}
	routes, _ := buildStreamRoutes(cfg, certs)
	if routes[4433] == nil {
		t.Fatal("terminate route did not compile")
	}
	routes[4433].match("stream.example.com").maxPerSource = 4
	f, err := startTCPForwarder(0, routes[4433], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.stop)
	return f
}

// Terminate mode forwards the half-close as close_notify plus FIN toward the
// client, and a client close_notify as FIN toward the backend.
func TestTCPStreamHalfCloseTLSTerminate(t *testing.T) {
	setLinger(t, time.Hour)
	for _, sc := range halfCloseScenarios {
		t.Run(sc.name, func(t *testing.T) {
			addr, conns := acceptingBackend(t)
			f := terminateForwarder(t, addr)
			c, err := tls.Dial("tcp", fwdAddr(f), &tls.Config{ServerName: "stream.example.com", InsecureSkipVerify: true}) //nolint:gosec // self-signed test cert
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { c.Close() })
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			sc.run(t, c, nextBackend(t, conns))
			waitSlots(t, f, 0)
		})
	}
}

// recordingConn keeps every byte read from the socket, so a test can see the
// TLS records the peer sent.
type recordingConn struct {
	net.Conn
	read []byte
}

func (c *recordingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.read = append(c.read, p[:n]...)
	return n, err
}

// lastRecordType is the content type of the last complete TLS record in b.
func lastRecordType(b []byte) byte {
	var typ byte
	for len(b) >= 5 {
		n := 5 + (int(b[3])<<8 | int(b[4]))
		if n > len(b) {
			break
		}
		typ, b = b[0], b[n:]
	}
	return typ
}

// When the backend half-closes a terminated stream, the client receives a
// close_notify alert and then the TCP FIN, and can still send afterwards. Go's
// TLS client reads both a bare FIN and a close_notify as io.EOF, so the test
// reads the records (TLS 1.2, where an alert keeps its record type) and the
// raw socket itself.
func TestTCPStreamHalfCloseTLSTerminateSendsCloseNotifyAndFIN(t *testing.T) {
	setLinger(t, time.Hour)
	addr, conns := acceptingBackend(t)
	f := terminateForwarder(t, addr)
	raw, err := net.Dial("tcp", fwdAddr(f))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
	rec := &recordingConn{Conn: raw}
	c := tls.Client(rec, &tls.Config{ServerName: "stream.example.com", InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12}) //nolint:gosec // self-signed test cert
	if err := c.Handshake(); err != nil {
		t.Fatal(err)
	}
	backend := nextBackend(t, conns)
	if _, err := backend.Write([]byte("banner")); err != nil {
		t.Fatal(err)
	}
	if err := backend.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, c, "client"); got != "banner" {
		t.Fatalf("client got %q", got)
	}
	if typ := lastRecordType(rec.read); typ != 0x15 {
		t.Fatalf("last TLS record before EOF has type %#x, want an alert (0x15): no close_notify", typ)
	}
	_ = raw.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := raw.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("raw read after the TLS EOF = %v, want io.EOF: no FIN after the close_notify", err)
	}
	_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("late")); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, backend, "backend"); got != "late" {
		t.Fatalf("backend got %q", got)
	}
	waitSlots(t, f, 0)
}

// readReset reads c to its end and requires a reset: an error that is neither
// io.EOF nor a deadline.
func readReset(t *testing.T, c net.Conn, who string) {
	t.Helper()
	b, err := io.ReadAll(c)
	var ne net.Error
	if err == nil || (errors.As(err, &ne) && ne.Timeout()) {
		t.Fatalf("%s: read %q, end %v; want a connection reset", who, b, err)
	}
}

// After one side half-closed, a host without idleTimeout resets a pair whose
// other side has not finished within streamHalfCloseLinger, so the cut is not
// read as a clean end of stream. A host with idleTimeout has no linger bound:
// an active transfer runs on and a silent pair ends by the idle deadline. A
// read error (a reset) ends the pair at once instead of half-closing it. Every
// case releases the connection slot.
func TestTCPStreamHalfCloseBounds(t *testing.T) {
	cases := []struct {
		name   string
		linger time.Duration
		idle   time.Duration
		run    func(t *testing.T, client, backend *net.TCPConn)
	}{
		{"linger bound resets a client waiting for a reply", 100 * time.Millisecond, 0, func(t *testing.T, client, backend *net.TCPConn) {
			_ = client.CloseWrite()
			if got := readAll(t, backend, "backend"); got != "" {
				t.Fatalf("backend got %q", got)
			}
			// Part of a reply, then the backend stays open: only the linger bound ends it.
			if _, err := backend.Write([]byte("partial")); err != nil {
				t.Fatal(err)
			}
			readReset(t, client, "client")
		}},
		{"linger bound resets a backend waiting for a reply", 100 * time.Millisecond, 0, func(t *testing.T, client, backend *net.TCPConn) {
			_ = backend.CloseWrite()
			if got := readAll(t, client, "client"); got != "" {
				t.Fatalf("client got %q", got)
			}
			if _, err := client.Write([]byte("partial")); err != nil {
				t.Fatal(err)
			}
			readReset(t, backend, "backend")
		}},
		{"idleTimeout: an active transfer after a half-close outlives the linger value", 100 * time.Millisecond, time.Second, func(t *testing.T, client, backend *net.TCPConn) {
			_ = client.CloseWrite()
			if got := readAll(t, backend, "backend"); got != "" {
				t.Fatalf("backend got %q", got)
			}
			want := ""
			for i := 0; i < 25; i++ {
				want += fmt.Sprintf("chunk%02d;", i)
			}
			go func() {
				// 25 chunks 20 ms apart: about five times the linger value.
				for i := 0; i < 25; i++ {
					if _, err := fmt.Fprintf(backend, "chunk%02d;", i); err != nil {
						return
					}
					time.Sleep(20 * time.Millisecond)
				}
				_ = backend.Close()
			}()
			if got := readAll(t, client, "client"); got != want {
				t.Fatalf("client got %q, want the whole reply", got)
			}
		}},
		{"idleTimeout ends a silent half-closed pair by the idle deadline", 100 * time.Millisecond, 300 * time.Millisecond, func(t *testing.T, client, backend *net.TCPConn) {
			start := time.Now()
			_ = client.CloseWrite()
			_ = readAll(t, backend, "backend")
			// A clean EOF, not the linger reset, and no earlier than the idle deadline.
			_ = readAll(t, client, "client")
			if d := time.Since(start); d < 300*time.Millisecond {
				t.Fatalf("pair ended after %v, before the 300ms idle deadline", d)
			}
		}},
		{"backend reset tears down both sides", time.Hour, 0, func(t *testing.T, client, backend *net.TCPConn) {
			_ = backend.SetLinger(0)
			_ = backend.Close()
			_ = readAll(t, client, "client")
		}},
		{"client reset tears down both sides", time.Hour, 0, func(t *testing.T, client, backend *net.TCPConn) {
			_ = client.SetLinger(0)
			_ = client.Close()
			_ = readAll(t, backend, "backend")
		}},
		{"reset after a half-close tears down at once", time.Hour, 0, func(t *testing.T, client, backend *net.TCPConn) {
			_ = client.CloseWrite()
			_ = readAll(t, backend, "backend")
			_ = backend.SetLinger(0)
			_ = backend.Close()
			_ = readAll(t, client, "client")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setLinger(t, tc.linger)
			addr, conns := acceptingBackend(t)
			f, err := startTCPForwarder(0, &streamRoutes{def: &streamTarget{name: "s", addr: addr, idle: tc.idle, maxPerSource: 4}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(f.stop)
			c, err := net.Dial("tcp", fwdAddr(f))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { c.Close() })
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			backend := nextBackend(t, conns)
			waitSlots(t, f, 1)
			tc.run(t, c.(*net.TCPConn), backend)
			waitSlots(t, f, 0)
		})
	}
}

// A half-closed pair releases its own slot exactly once: another open
// connection from the same source keeps its slot counted.
func TestTCPStreamHalfCloseReleasesSlotOnce(t *testing.T) {
	setLinger(t, time.Hour)
	addr, conns := acceptingBackend(t)
	f, err := startTCPForwarder(0, &streamRoutes{def: &streamTarget{name: "s", addr: addr, maxPerSource: 2}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.stop)
	dial := func() *net.TCPConn {
		c, err := net.Dial("tcp", fwdAddr(f))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		return c.(*net.TCPConn)
	}
	a := dial()
	nextBackend(t, conns)
	b := dial()
	halfCloseScenarios[0].run(t, b, nextBackend(t, conns))
	waitSlots(t, f, 1)
	for i := 0; i < 50; i++ {
		if n := slotsHeld(f); n != 1 {
			t.Fatalf("slot count moved to %d after the pair ended, want 1", n)
		}
	}
	_ = a.SetLinger(0)
	_ = a.Close()
	waitSlots(t, f, 0)
}

// A client conn without CloseWrite (a pipe) keeps today's behaviour: the first
// EOF ends the pair at once, not after the linger bound.
func TestTCPStreamHalfCloseFallsBackWithoutCloseWrite(t *testing.T) {
	addr, conns := acceptingBackend(t)
	f := &tcpForwarder{perSource: map[string]int{}, linger: time.Hour}
	f.routes.Store(&streamRoutes{def: &streamTarget{name: "s", addr: addr}})
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() { f.handle(server); close(done) }()
	backend := nextBackend(t, conns)
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a pipe client's EOF must end the pair at once")
	}
	if got := readAll(t, backend, "backend"); got != "" {
		t.Fatalf("backend got %q", got)
	}
}
