package dataplane

import (
	"fmt"
	"io"
	"net"
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

	f, err := startTCPForwarder(0, backend) // :0 = ephemeral
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

	f, err := startUDPForwarder(0, bpc.LocalAddr().String())
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
		ObjectMeta:  model.ObjectMeta{Name: "s"},
		ListenPort:  port,
		Protocol:    "tcp",
		ForwardHost: "127.0.0.1",
		ForwardPort: portOf(t, backend),
	}
	m.reload([]model.StreamHost{host})
	if len(m.tcp) != 1 || m.tcp[port] == nil {
		t.Fatalf("expected a tcp forwarder on %d", port)
	}
	// Reconcile to empty -> the forwarder is stopped and removed.
	m.reload(nil)
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
