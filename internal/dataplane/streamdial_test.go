package dataplane

import (
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// blockConn is a backend socket whose Write waits for the write deadline while
// block is set, and whose Read waits for Close.
type blockConn struct {
	net.Conn
	mu       sync.Mutex
	wdl      time.Time
	block    atomic.Bool
	writes   atomic.Int32
	closed   chan struct{}
	once     sync.Once
	readSeen chan struct{}
	rOnce    sync.Once
}

func (c *blockConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.wdl = t
	c.mu.Unlock()
	return nil
}

func (c *blockConn) SetReadDeadline(time.Time) error { return nil }

func (c *blockConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	dl := c.wdl
	c.mu.Unlock()
	if c.block.Load() {
		if dl.IsZero() {
			return 0, errors.New("blocked with no deadline")
		}
		time.Sleep(time.Until(dl))
		return 0, os.ErrDeadlineExceeded
	}
	if !dl.IsZero() && time.Now().After(dl) {
		return 0, os.ErrDeadlineExceeded
	}
	c.writes.Add(1)
	return len(p), nil
}

func (c *blockConn) Read([]byte) (int, error) {
	c.rOnce.Do(func() { close(c.readSeen) })
	<-c.closed
	return 0, net.ErrClosed
}

func (c *blockConn) Close() error { c.once.Do(func() { close(c.closed) }); return nil }

// A flush of queued packets that times out still leaves a usable session: it
// is installed, counted once, its reader runs, its dial slot is returned, and
// the write deadline is cleared so later packets are written.
func TestUDPFlushTimeoutLeavesAUsableSession(t *testing.T) {
	hook := installHook(t)
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	bc := &blockConn{closed: make(chan struct{}), readSeen: make(chan struct{})}
	bc.block.Store(true)
	release := make(chan struct{})
	f.mu.Lock()
	f.dial = func(string) (net.Conn, error) { <-release; return bc, nil }
	f.mu.Unlock()
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 51000}
	f.forward(addr, []byte("a"))
	f.forward(addr, []byte("b"))
	close(release)
	udpSettle(f)
	f.mu.Lock() // waits for the flush, which runs under f.mu
	_, installed := f.sessions[addr.String()]
	dials, perSource := len(f.dialsPerSource), f.perSource[udpSourceKey(addr.IP)]
	f.mu.Unlock()
	select {
	case <-bc.readSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("the backend reader was not started")
	}
	hook.mu.Lock()
	up := hook.streamsUp["s"]
	hook.mu.Unlock()
	if !installed || dials != 0 || perSource != 1 || up != 1 {
		t.Fatalf("installed=%v dialsPerSource=%d perSource=%d opened=%d, want true 0 1 1", installed, dials, perSource, up)
	}
	// Past the flush deadline, a packet on the session must still be written:
	// a deadline left on the socket would fail it.
	bc.block.Store(false)
	time.Sleep(udpFlushTimeout + 100*time.Millisecond)
	f.forward(addr, []byte("c"))
	if n := bc.writes.Load(); n != 1 {
		t.Fatalf("writes after the timed-out flush = %d, want 1", n)
	}
}

// Every dial slot comes back, on success and on failure: one address can open
// more sessions in sequence than the per-source dial cap, and nothing is left
// in the dial bookkeeping afterwards.
func TestUDPDialSlotsAreReturned(t *testing.T) {
	f, err := startUDPForwarder(0, streamTarget{name: "s", addr: silentUDPBackend(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	ip := net.IPv4(127, 0, 0, 1)
	for p := 1; p <= 20; p++ {
		f.forward(&net.UDPAddr{IP: ip, Port: 52000 + p}, []byte("x"))
		udpSettle(f)
	}
	if n := udpSessionCount(f); n != 20 {
		t.Fatalf("sessions = %d, want 20", n)
	}
	f.mu.Lock()
	f.dial = func(string) (net.Conn, error) { return nil, errors.New("dial refused") }
	f.mu.Unlock()
	for p := 21; p <= 40; p++ {
		f.forward(&net.UDPAddr{IP: ip, Port: 52000 + p}, []byte("x"))
		udpSettle(f)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.dialsPerSource) != 0 || len(f.pending) != 0 || f.perSource[udpSourceKey(ip)] != 20 {
		t.Fatalf("dialsPerSource=%v pending=%d perSource=%v, want empty, 0 and 20", f.dialsPerSource, len(f.pending), f.perSource)
	}
}
