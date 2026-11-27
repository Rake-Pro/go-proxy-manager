package dataplane

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
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

// udpSettle waits until no session of f is still being dialled, since forward
// dials new sessions in the background.
func udpSettle(f *udpForwarder) {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		f.mu.Lock()
		n := len(f.pending)
		f.mu.Unlock()
		if n == 0 {
			return
		}
	}
}

func udpSessionCount(f *udpForwarder) int {
	udpSettle(f)
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
	udpSettle(f)

	// Every session active: the new source is dropped, nothing is evicted.
	f.forward(src(4), []byte("x"))
	udpSettle(f)
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
	udpSettle(f)
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

// echoOnce writes msg on c and reports whether it came back within wait.
func echoOnce(c net.Conn, msg string, wait time.Duration) bool {
	if _, err := c.Write([]byte(msg)); err != nil {
		return false
	}
	buf := make([]byte, len(msg))
	_ = c.SetReadDeadline(time.Now().Add(wait))
	_, err := io.ReadFull(c, buf)
	return err == nil && string(buf) == msg
}

// closedWithin reports whether the forwarder closed c within wait.
func closedWithin(c net.Conn, wait time.Duration) bool {
	_ = c.SetReadDeadline(time.Now().Add(wait))
	_, err := c.Read(make([]byte, 1))
	return err == io.EOF
}

// idleTimeout closes a TCP stream connection that carried no data for that
// long; traffic keeps it open. Without the key a silent connection stays open.
func TestTCPStreamIdleTimeout(t *testing.T) {
	backend, stop := tcpEcho(t)
	defer stop()
	dial := func(target *streamTarget) net.Conn {
		f, err := startTCPForwarder(0, &streamRoutes{def: target}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(f.stop)
		c, err := net.Dial("tcp", f.ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}

	c := dial(&streamTarget{name: "s", addr: backend, idle: 300 * time.Millisecond})
	for i := 0; i < 4; i++ { // 4 x 150 ms of traffic: past the timeout in total
		if !echoOnce(c, "ping", time.Second) {
			t.Fatalf("active connection: echo %d failed", i)
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !closedWithin(c, 2*time.Second) {
		t.Fatal("a silent connection must be closed after idleTimeout")
	}

	c = dial(&streamTarget{name: "s", addr: backend})
	if !echoOnce(c, "ping", time.Second) {
		t.Fatal("echo failed without idleTimeout")
	}
	if closedWithin(c, 600*time.Millisecond) {
		t.Fatal("without idleTimeout a silent connection must stay open")
	}
}

// maxConnsPerSource closes a connection over the cap before the backend is
// dialled, and a closed connection frees its slot.
func TestTCPStreamMaxConnsPerSource(t *testing.T) {
	backend, stop := tcpEcho(t)
	defer stop()
	f, err := startTCPForwarder(0, &streamRoutes{def: &streamTarget{name: "s", addr: backend, maxPerSource: 2}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	dial := func() net.Conn {
		c, err := net.Dial("tcp", f.ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	a, b := dial(), dial()
	if !echoOnce(a, "a", time.Second) || !echoOnce(b, "b", time.Second) {
		t.Fatal("connections within the cap must be forwarded")
	}
	if over := dial(); !closedWithin(over, 2*time.Second) {
		t.Fatal("a connection over maxConnsPerSource must be closed")
	}
	a.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		c := dial()
		if echoOnce(c, "c", 300*time.Millisecond) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("closing a connection must free its slot")
		}
	}
}

// udpEchoBackend starts a UDP echo server and returns its address.
func udpEchoBackend(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	return pc.LocalAddr().String()
}

// udpClient dials the forwarder from a real loopback socket.
func udpClient(t *testing.T, f *udpForwarder) net.Conn {
	t.Helper()
	c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", f.pc.LocalAddr().(*net.UDPAddr).Port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// udpExpect reads one datagram from c and checks it is want.
func udpExpect(t *testing.T, c net.Conn, want string) {
	t.Helper()
	buf := make([]byte, 64)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("waiting for %q: %v", want, err)
	}
	if got := string(buf[:n]); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func udpSourceSessions(f *udpForwarder, ip net.IP) int {
	udpSettle(f)
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, s := range f.sessions {
		if s.src == udpSourceKey(ip) {
			n++
		}
	}
	return n
}

// newSessionsPerMinute lets one source open at most the limit of new sessions
// in a burst; another source has its own budget, a session that is already open
// keeps flowing, and the budget refills over the minute.
func TestUDPNewSessionsPerMinute(t *testing.T) {
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: udpEchoBackend(t), newPerMinute: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()

	// The first session of 127.0.0.1 is a real client, so its traffic can be
	// checked end to end.
	live := udpClient(t, f)
	if _, err := live.Write([]byte("one")); err != nil {
		t.Fatal(err)
	}
	udpExpect(t, live, "one")

	a, b := net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 2)
	for p := 1; p <= 6; p++ {
		f.forward(&net.UDPAddr{IP: a, Port: 41000 + p}, []byte("x"))
		f.forward(&net.UDPAddr{IP: b, Port: 41000 + p}, []byte("x"))
	}
	if n := udpSourceSessions(f, a); n != 3 {
		t.Fatalf("source a opened %d sessions in a burst, want the limit 3", n)
	}
	if n := udpSourceSessions(f, b); n != 3 {
		t.Fatalf("source b opened %d sessions, want its own budget of 3", n)
	}
	if _, err := live.Write([]byte("two")); err != nil {
		t.Fatal(err)
	}
	udpExpect(t, live, "two")

	// A minute later the budget is back.
	f.mu.Lock()
	f.newSessions[udpSourceKey(a)].last = time.Now().Add(-time.Minute)
	f.mu.Unlock()
	f.forward(&net.UDPAddr{IP: a, Port: 42000}, []byte("x"))
	if n := udpSourceSessions(f, a); n != 4 {
		t.Fatalf("after the refill source a has %d sessions, want 4", n)
	}
}

// The limiter tracks at most maxUDPSessions sources. When that table is full a
// source whose budget has refilled is forgotten to make room; with none, a new
// session from an untracked source is refused, as a full session table does.
// The table is scanned for such sources at most once a second, not per packet.
func TestUDPNewSessionsLimiterIsBounded(t *testing.T) {
	prev := maxUDPSessions
	maxUDPSessions = 2
	t.Cleanup(func() { maxUDPSessions = prev })

	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t), newPerMinute: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	src := func(i byte) *net.UDPAddr { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, i), Port: 40000} }
	f.forward(src(1), []byte("x"))
	f.forward(src(2), []byte("x"))
	udpSettle(f)
	// End both sessions so only the limiter table is full.
	f.mu.Lock()
	for k, s := range f.sessions {
		f.closeSession(s)
		f.dropLocked(k, s)
	}
	f.mu.Unlock()

	f.forward(src(3), []byte("x"))
	if n := udpSessionCount(f); n != 0 {
		t.Fatalf("an untracked source opened a session with the limiter table full, table has %d", n)
	}
	f.mu.Lock()
	f.newSessions[udpSourceKey(src(1).IP)].last = time.Now().Add(-time.Minute)
	f.rateReclaimAt = time.Now().Add(time.Hour) // the last scan was under a second ago
	f.mu.Unlock()
	f.forward(src(3), []byte("x"))
	if n := udpSessionCount(f); n != 0 {
		t.Fatalf("the full table was scanned again within a second (table has %d sessions)", n)
	}
	f.mu.Lock()
	f.rateReclaimAt = time.Time{} // the second has passed
	f.mu.Unlock()
	f.forward(src(3), []byte("x"))
	if n := udpSessionCount(f); n != 1 {
		t.Fatalf("with a refilled source to reclaim, the new source must get a session, table has %d", n)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.newSessions) > maxUDPSessions {
		t.Fatalf("limiter tracks %d sources, want at most %d", len(f.newSessions), maxUDPSessions)
	}
	if _, ok := f.newSessions[udpSourceKey(src(1).IP)]; ok {
		t.Fatal("the refilled source was not reclaimed")
	}
}

// gatedDial is a dial seam that blocks every dial until release is closed.
type gatedDial struct {
	started chan string
	release chan struct{}
	fail    bool
	dials   atomic.Int32

	mu    sync.Mutex
	conns []net.Conn
}

func newGatedDial() *gatedDial {
	return &gatedDial{started: make(chan string, 256), release: make(chan struct{})}
}

func (g *gatedDial) dial(addr string) (net.Conn, error) {
	g.dials.Add(1)
	g.started <- addr
	<-g.release
	if g.fail {
		return nil, fmt.Errorf("dial refused")
	}
	c, err := net.Dial("udp", addr)
	if err == nil {
		g.mu.Lock()
		g.conns = append(g.conns, c)
		g.mu.Unlock()
	}
	return c, err
}

func (g *gatedDial) install(f *udpForwarder) {
	f.mu.Lock()
	f.dial = g.dial
	f.mu.Unlock()
}

func waitDialStarted(t *testing.T, g *gatedDial) {
	t.Helper()
	select {
	case <-g.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the backend dial never started")
	}
}

// A dial that blocks holds up neither the listener nor the other sessions on
// it; further datagrams from the client being dialled are queued for its one
// session (no second dial) and delivered in order once the dial completes.
func TestUDPDialOutsideListenerLock(t *testing.T) {
	hook := installHook(t)
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: udpEchoBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()

	existing := udpClient(t, f)
	if _, err := existing.Write([]byte("a1")); err != nil {
		t.Fatal(err)
	}
	udpExpect(t, existing, "a1")

	g := newGatedDial()
	g.install(f)
	fresh := udpClient(t, f)
	if _, err := fresh.Write([]byte("b1")); err != nil {
		t.Fatal(err)
	}
	waitDialStarted(t, g)
	for _, msg := range []string{"b2", "b3"} {
		if _, err := fresh.Write([]byte(msg)); err != nil {
			t.Fatal(err)
		}
	}
	// The existing session keeps flowing while the dial is blocked.
	if _, err := existing.Write([]byte("a2")); err != nil {
		t.Fatal(err)
	}
	udpExpect(t, existing, "a2")

	close(g.release)
	for _, msg := range []string{"b1", "b2", "b3"} {
		udpExpect(t, fresh, msg)
	}
	if n := g.dials.Load(); n != 1 {
		t.Fatalf("the new client caused %d backend dials, want 1", n)
	}
	if n := udpSessionCount(f); n != 2 {
		t.Fatalf("table has %d sessions, want 2", n)
	}
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if hook.streamsUp["s"] != 2 {
		t.Fatalf("active-session metric opened %d times, want 2", hook.streamsUp["s"])
	}
}

// Slots are reserved before the dial: new clients dialling at once cannot
// overshoot the per-source cap, and a failed dial gives its slot back.
func TestUDPCapsHoldDuringConcurrentDials(t *testing.T) {
	prev := maxUDPSessionsPerSource
	maxUDPSessionsPerSource = 2
	t.Cleanup(func() { maxUDPSessionsPerSource = prev })

	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	g := newGatedDial()
	g.install(f)
	ip := net.IPv4(127, 0, 0, 1)
	for p := 1; p <= 5; p++ {
		f.forward(&net.UDPAddr{IP: ip, Port: 43000 + p}, []byte("x"))
	}
	waitDialStarted(t, g)
	waitDialStarted(t, g)
	if n := g.dials.Load(); n != 2 {
		t.Fatalf("%d dials started for one source, want the cap 2", n)
	}
	g.fail = true // read by the dial goroutines only after release is closed
	close(g.release)
	udpSettle(f)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sessions) != 0 || len(f.pending) != 0 || len(f.perSource) != 0 {
		t.Fatalf("failed dials left sessions=%d pending=%d perSource=%v", len(f.sessions), len(f.pending), f.perSource)
	}
}

// stop() while a dial is in flight: the dialled socket is closed, no session
// appears in the stopped forwarder, and the counters stay balanced.
func TestUDPStopDuringDial(t *testing.T) {
	hook := installHook(t)
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	g := newGatedDial()
	g.install(f)
	f.forward(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 44000}, []byte("x"))
	waitDialStarted(t, g)
	f.stop()
	close(g.release)
	udpSettle(f)

	f.mu.Lock()
	if len(f.sessions) != 0 || len(f.pending) != 0 || len(f.perSource) != 0 {
		f.mu.Unlock()
		t.Fatalf("stopped forwarder has sessions=%d pending=%d perSource=%v", len(f.sessions), len(f.pending), f.perSource)
	}
	f.mu.Unlock()
	g.mu.Lock()
	conns := g.conns
	g.mu.Unlock()
	if len(conns) != 1 {
		t.Fatalf("dialled %d sockets, want 1", len(conns))
	}
	if _, err := conns[0].Write([]byte("x")); err == nil || !errors.Is(err, net.ErrClosed) {
		t.Fatalf("the socket dialled for a stopped forwarder is still open (write err %v)", err)
	}
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if hook.streamsUp["s"] != hook.streamsDn["s"] {
		t.Fatalf("active-session metric unbalanced: opened %d, closed %d", hook.streamsUp["s"], hook.streamsDn["s"])
	}
}

// The queue of a session being dialled is capped by packet count and by bytes
// with a per-entry overhead, so floods of empty or tiny datagrams during a
// slow dial cannot grow it.
func TestUDPPendingQueueIsBounded(t *testing.T) {
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	g := newGatedDial()
	g.install(f)
	defer close(g.release)
	for i, flood := range [][]byte{{}, []byte("x"), make([]byte, 60000)} {
		addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 45000 + i}
		f.forward(addr, flood)
		for range 10000 {
			f.forward(addr, flood)
		}
		f.mu.Lock()
		p := f.pending[addr.String()]
		queued, size := len(p.queue), p.size
		f.mu.Unlock()
		if queued > udpPendingMaxPackets {
			t.Errorf("%d-byte flood: %d datagrams queued, want at most %d", len(flood), queued, udpPendingMaxPackets)
		}
		if limit := max(udpBufSize, len(flood)+udpPendingEntryOverhead); size > limit {
			t.Errorf("%d-byte flood: %d bytes queued, want at most %d", len(flood), size, limit)
		}
	}
}

// Once the dial returns, the session is installed at once even while the
// client keeps sending: the queue is handed over under one lock, not drained
// until the client pauses.
func TestUDPSessionInstalledPromptlyForBusyClient(t *testing.T) {
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	g := newGatedDial()
	g.install(f)
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 46000}
	f.forward(addr, []byte("x"))
	waitDialStarted(t, g)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				f.forward(addr, []byte("x"))
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()

	close(g.release)
	deadline := time.Now().Add(time.Second)
	for {
		f.mu.Lock()
		installed := f.sessions[addr.String()] != nil
		f.mu.Unlock()
		if installed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("a continuously sending client's session was not installed within 1s of the dial returning")
		}
		time.Sleep(time.Millisecond)
	}
}

// A client retrying while the session table is full takes no
// newSessionsPerMinute token: only a reserved slot costs one. The session
// table is full while the limiter table (bounded by the same number) is not,
// so b is tracked by the limiter and dropped by the session table.
func TestUDPRateTokenTakenOnlyForReservedSlot(t *testing.T) {
	prev := maxUDPSessions
	maxUDPSessions = 2
	t.Cleanup(func() { maxUDPSessions = prev })

	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t), newPerMinute: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	a := net.IPv4(127, 0, 0, 1)
	f.forward(&net.UDPAddr{IP: a, Port: 47000}, []byte("x"))
	f.forward(&net.UDPAddr{IP: a, Port: 47001}, []byte("x"))
	if n := udpSessionCount(f); n != 2 {
		t.Fatalf("source a opened %d sessions, want 2 to fill the table", n)
	}
	b := net.IPv4(127, 0, 0, 2)
	for p := 1; p <= 5; p++ {
		f.forward(&net.UDPAddr{IP: b, Port: 47010 + p}, []byte("x")) // table full of active sessions
	}
	if n := udpSourceSessions(f, b); n != 0 {
		t.Fatalf("source b opened %d sessions into a full table", n)
	}
	f.mu.Lock()
	_, tracked := f.newSessions[udpSourceKey(b)]
	f.mu.Unlock()
	if !tracked {
		t.Fatal("source b has no limiter entry, so its drops never reached the token check")
	}
	maxUDPSessions = 10
	for p := 1; p <= 3; p++ {
		f.forward(&net.UDPAddr{IP: b, Port: 47100 + p}, []byte("x"))
	}
	if n := udpSourceSessions(f, b); n != 2 {
		t.Fatalf("after the table freed up source b opened %d sessions, want its untouched budget of 2", n)
	}
}

// At most maxUDPDials dials run at once per listener. A new-session packet
// that finds no dial slot is dropped with nothing reserved and no
// newSessionsPerMinute token or limiter entry taken.
func TestUDPConcurrentDialsAreCapped(t *testing.T) {
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t), newPerMinute: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	g := newGatedDial()
	g.install(f)
	src := func(i int) *net.UDPAddr { return &net.UDPAddr{IP: net.IPv4(127, 0, 1, byte(i)), Port: 48000} }
	for i := 1; i <= maxUDPDials+6; i++ {
		f.forward(src(i), []byte("x"))
	}
	f.mu.Lock()
	pending, reserved := len(f.pending), 0
	for _, n := range f.perSource {
		reserved += n
	}
	_, tracked := f.newSessions[udpSourceKey(src(maxUDPDials+1).IP)]
	f.mu.Unlock()
	if pending != maxUDPDials || reserved != maxUDPDials {
		t.Fatalf("pending=%d reserved=%d, want both at the dial cap %d", pending, reserved, maxUDPDials)
	}
	if tracked {
		t.Fatal("a packet dropped for lack of a dial slot took a newSessionsPerMinute limiter entry")
	}
	close(g.release)
	if n := udpSessionCount(f); n != maxUDPDials {
		t.Fatalf("table has %d sessions after the dials, want %d", n, maxUDPDials)
	}
	// The dropped sources still have their budget.
	f.forward(src(maxUDPDials+1), []byte("x"))
	if n := udpSessionCount(f); n != maxUDPDials+1 {
		t.Fatalf("a source dropped for lack of a dial slot could not open a session afterwards (table %d)", n)
	}
}

// One source can hold at most maxUDPDialsPerSource dials at once, so a single
// client with many source ports cannot take every dial slot of the listener
// from other sources.
func TestUDPDialsPerSourceAreCapped(t *testing.T) {
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t), newPerMinute: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	g := newGatedDial()
	g.install(f)
	defer close(g.release)
	a, b := net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 2)
	for p := 1; p <= maxUDPDials; p++ {
		f.forward(&net.UDPAddr{IP: a, Port: 49000 + p}, []byte("x"))
	}
	f.forward(&net.UDPAddr{IP: b, Port: 49000}, []byte("x"))
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.dialsPerSource[udpSourceKey(a)]; got != maxUDPDialsPerSource {
		t.Fatalf("source a holds %d dials, want the per-source cap %d", got, maxUDPDialsPerSource)
	}
	if got := f.perSource[udpSourceKey(a)]; got != maxUDPDialsPerSource {
		t.Fatalf("source a holds %d reserved slots, want %d (drops must reserve nothing)", got, maxUDPDialsPerSource)
	}
	// Spent tokens are whole; the fraction is refill over the test's runtime.
	if got := f.newSessions[udpSourceKey(a)].tokens; got < 100-maxUDPDialsPerSource || got >= 100-maxUDPDialsPerSource+1 {
		t.Fatalf("source a has %v tokens, want %d (drops must take no token)", got, 100-maxUDPDialsPerSource)
	}
	if got := f.dialsPerSource[udpSourceKey(b)]; got != 1 {
		t.Fatalf("source b holds %d dials, want 1: a must not block it", got)
	}
}
