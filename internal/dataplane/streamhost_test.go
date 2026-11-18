package dataplane

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// tcpEcho starts a TCP echo server and returns its address + a stop func.
func tcpEcho(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { _, _ = io.Copy(c, c); c.Close() }(c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestTCPForwarder(t *testing.T) {
	backend, stop := tcpEcho(t)
	defer stop()

	f, err := startTCPForwarder(0, &streamRoutes{def: &streamTarget{name: "s", addr: backend}}, nil, nil) // :0 = ephemeral
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()

	port := f.ln.Addr().(*net.TCPAddr).Port
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("hello stream")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 12)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "hello stream" {
		t.Fatalf("echo mismatch: %q", buf)
	}
}

func TestUDPForwarder(t *testing.T) {
	bpc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bpc.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := bpc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = bpc.WriteTo(buf[:n], addr)
		}
	}()

	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: bpc.LocalAddr().String()})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()

	port := f.pc.LocalAddr().(*net.UDPAddr).Port
	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "ping" {
		t.Fatalf("udp echo mismatch: %q", buf[:n])
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestStreamManagerReconcile(t *testing.T) {
	backend, stop := tcpEcho(t)
	defer stop()
	port := freePort(t)
	m := newStreamManager()
	defer m.stopAll()

	host := model.StreamHost{
		ObjectMeta: model.ObjectMeta{Name: "s"},
		ListenPort: port,
		Protocol:   "tcp",
		Target:     model.StreamTarget{Host: "127.0.0.1", Port: portOf(t, backend)},
	}
	m.reload(model.Config{StreamHosts: []model.StreamHost{host}}, nil)
	if len(m.tcp) != 1 || m.tcp[port] == nil {
		t.Fatalf("expected a tcp forwarder on %d", port)
	}
	// Reconcile to empty -> the forwarder is stopped and removed.
	m.reload(model.Config{}, nil)
	if len(m.tcp) != 0 {
		t.Fatalf("forwarder should be removed on reconcile, have %d", len(m.tcp))
	}
}

func portOf(t *testing.T, hostport string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(hostport)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	if _, err := fmt.Sscanf(p, "%d", &n); err != nil {
		t.Fatal(err)
	}
	return n
}

// silentUDPBackend accepts datagrams and never answers.
func silentUDPBackend(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, _, err := pc.ReadFrom(buf); err != nil {
				return
			}
		}
	}()
	return pc.LocalAddr().String()
}

func udpSessionCount(f *udpForwarder) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions)
}

// One source address may hold only its share of the UDP session table; other
// sources still get sessions.
func TestUDPPerSourceSessionCap(t *testing.T) {
	prev := maxUDPSessionsPerSource
	maxUDPSessionsPerSource = 4
	t.Cleanup(func() { maxUDPSessionsPerSource = prev })

	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	for p := 1; p <= 8; p++ {
		f.forward(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40000 + p}, []byte("x"))
	}
	if n := udpSessionCount(f); n != 4 {
		t.Fatalf("one source opened %d sessions, want the per-source cap 4", n)
	}
	f.forward(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 2), Port: 40000}, []byte("x"))
	if n := udpSessionCount(f); n != 5 {
		t.Fatalf("another source must still get a session, table has %d", n)
	}
}

// A full UDP session table makes room for a new source only by closing a
// session that has been silent for at least udpEvictIdle; when every session is
// active, the new source is dropped and no session is closed.
func TestUDPFullTableEvictsOnlyIdleSession(t *testing.T) {
	prev := maxUDPSessions
	maxUDPSessions = 3
	t.Cleanup(func() { maxUDPSessions = prev })

	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	src := func(i byte) *net.UDPAddr { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, i), Port: 40000} }
	for i := byte(1); i <= 3; i++ {
		f.forward(src(i), []byte("x"))
	}

	// Every session active: the new source is dropped, nothing is evicted.
	f.forward(src(4), []byte("x"))
	f.mu.Lock()
	if len(f.sessions) != 3 || f.sessions[src(4).String()] != nil {
		f.mu.Unlock()
		t.Fatalf("with every session active a new source must be dropped, table has %d", len(f.sessions))
	}
	for i := byte(1); i <= 3; i++ {
		if s := f.sessions[src(i).String()]; s == nil || s.done.Load() {
			f.mu.Unlock()
			t.Fatalf("active session %d was closed", i)
		}
	}
	// Session 2 silent past udpEvictIdle (session 1 is the oldest still active).
	idle := f.sessions[src(2).String()]
	idle.last.Store(time.Now().Add(-udpEvictIdle - time.Second).UnixNano())
	f.mu.Unlock()

	f.forward(src(5), []byte("x"))
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sessions) != 3 {
		t.Fatalf("table has %d sessions, want 3", len(f.sessions))
	}
	if f.sessions[src(5).String()] == nil {
		t.Fatal("the new source must take the slot of the idle session")
	}
	if f.sessions[src(2).String()] != nil || !idle.done.Load() {
		t.Fatal("the idle session must be evicted and closed")
	}
	if f.sessions[src(1).String()] == nil || f.sessions[src(3).String()] == nil {
		t.Fatal("active sessions must be kept")
	}
	if f.perSource[udpSourceKey(src(2).IP)] != 0 {
		t.Fatalf("the evicted session's source slot was not released: %v", f.perSource)
	}
}

// UDP sources are capped per IPv4 address or per IPv6 /56: addresses from
// different /64s of one /56 draw on one share of the session table.
func TestUDPPerSourceCapGroupsIPv6By56(t *testing.T) {
	a := udpSourceKey(net.ParseIP("2001:db8:0:1::10"))
	b := udpSourceKey(net.ParseIP("2001:db8:0:ff::1"))
	c := udpSourceKey(net.ParseIP("2001:db8:0:100::1"))
	if a != b || a != "2001:db8::/56" {
		t.Fatalf("two /64s of one /56 must share a key: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("different /56s must not share a key: %q", a)
	}
	if got := udpSourceKey(net.ParseIP("192.0.2.10")); got != "192.0.2.10" {
		t.Fatalf("IPv4 key = %q", got)
	}

	prev := maxUDPSessionsPerSource
	maxUDPSessionsPerSource = 4
	t.Cleanup(func() { maxUDPSessionsPerSource = prev })
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	for i := 1; i <= 8; i++ {
		f.forward(&net.UDPAddr{IP: net.ParseIP(fmt.Sprintf("2001:db8:0:%x::1", i)), Port: 40000}, []byte("x"))
	}
	if n := udpSessionCount(f); n != 4 {
		t.Fatalf("eight /64s of one /56 opened %d sessions, want the per-source cap 4", n)
	}
}

// failingListener returns errs from Accept, in order, before accepting for real.
type failingListener struct {
	net.Listener
	mu   sync.Mutex
	errs []error
}

func (l *failingListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if len(l.errs) > 0 {
		err := l.errs[0]
		l.errs = l.errs[1:]
		l.mu.Unlock()
		return nil, err
	}
	l.mu.Unlock()
	return l.Listener.Accept()
}

// An Accept error other than a closed listener (a descriptor shortage) is
// retried, so the stream port keeps serving afterwards; closing the listener
// still ends the loop.
func TestTCPAcceptLoopRetriesTemporaryErrors(t *testing.T) {
	backend, stop := tcpEcho(t)
	defer stop()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	emfile := &net.OpError{Op: "accept", Net: "tcp", Err: os.NewSyscallError("accept4", syscall.EMFILE)}
	f := &tcpForwarder{ln: &failingListener{Listener: ln, errs: []error{emfile, emfile, emfile}}}
	f.routes.Store(&streamRoutes{def: &streamTarget{name: "s", addr: backend}})
	done := make(chan struct{})
	go func() { f.acceptLoop(); close(done) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("after accept errors the port must still serve: %q, %v", buf, err)
	}

	f.stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("acceptLoop did not return after the listener closed")
	}
}
