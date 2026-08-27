package dataplane

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// withShortTimeouts shrinks the listener timeouts for the duration of a test.
func withShortTimeouts(t *testing.T, read, idle time.Duration) {
	t.Helper()
	oldRead, oldIdle := readTimeout, idleTimeout
	readTimeout, idleTimeout = read, idle
	t.Cleanup(func() { readTimeout, idleTimeout = oldRead, oldIdle })
}

// listenWithPolicy runs h on a loopback listener with the data-plane timeout policy and
// returns its address.
func listenWithPolicy(t *testing.T, h http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := newListenerServer(ln.Addr().String(), h)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

// An idle keep-alive connection must be reaped. Without IdleTimeout a client
// could hold connections (and their file descriptors) open indefinitely by
// completing one request and then simply never sending another.
func TestListenerClosesIdleKeepAliveConnections(t *testing.T) {
	withShortTimeouts(t, 2*time.Second, 200*time.Millisecond)
	addr := listenWithPolicy(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: app.example.com\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("first response: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", resp.StatusCode)
	}

	// The connection is now idle. It must be closed by the server well inside
	// readTimeout, which is what proves IdleTimeout (not ReadTimeout) reaped it.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := br.Read(make([]byte, 1)); err == nil {
		t.Fatal("the server kept an idle keep-alive connection open")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("the idle keep-alive connection was still open after IdleTimeout")
	}
}

// hijackEcho is a minimal upstream that answers an Upgrade request with 101 and
// then echoes lines on the raw connection, standing in for a WebSocket backend.
func hijackEcho(t *testing.T) (model.Upstream, func()) {
	t.Helper()
	return backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: " +
			r.Header.Get("Upgrade") + "\r\nConnection: Upgrade\r\n\r\n")
		_ = brw.Flush()
		for {
			line, err := brw.ReadString('\n')
			if err != nil {
				return
			}
			if _, err := brw.WriteString("echo:" + line); err != nil {
				return
			}
			if err := brw.Flush(); err != nil {
				return
			}
		}
	}))
}

// A protocol upgrade proxied through gpm must outlive the listener's read and
// idle timeouts: the tunnel is long-lived by definition, and a deadline left on
// the socket would cut it mid-session.
func TestUpgradeSurvivesListenerTimeouts(t *testing.T) {
	const short = 200 * time.Millisecond
	withShortTimeouts(t, short, short)

	up, closeFn := hijackEcho(t)
	defer closeFn()
	addr := listenWithPolicy(t, newReverseProxy(up, "ws", nil, nil, nil, nil))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: ws.example.com\r\n"+
		"Connection: Upgrade\r\nUpgrade: websocket\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("reading the upgrade response: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("upgrade response was %q, want 101", strings.TrimSpace(status))
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading the upgrade response headers: %v", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// Idle for longer than both timeouts, then use the tunnel. (For a bodiless
	// request the stdlib also drops its own read deadline once the handler
	// starts, so this asserts the end-to-end guarantee rather than the explicit
	// clearing alone - which is what an operator actually depends on.)
	time.Sleep(3 * short)
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, "still-here\n"); err != nil {
		t.Fatalf("writing to the tunnel after %s idle: %v", 3*short, err)
	}
	got, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("reading from the tunnel after %s idle: %v", 3*short, err)
	}
	if strings.TrimSpace(got) != "echo:still-here" {
		t.Fatalf("tunnel echoed %q", strings.TrimSpace(got))
	}
}

// A body streamed to an upstream must not be cut off by the listener's read
// timeout: a large or slow upload is bounded by the client and the backend, not
// by a proxy-side deadline.
func TestSlowUploadIsNotTruncatedByReadTimeout(t *testing.T) {
	const short = 200 * time.Millisecond
	withShortTimeouts(t, short, short)

	body := make(chan string, 1)
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("upstream read: %v", err)
		}
		body <- string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer closeFn()
	addr := listenWithPolicy(t, newReverseProxy(up, "upload", nil, nil, nil, nil))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(conn, "POST /upload HTTP/1.1\r\nHost: up.example.com\r\n"+
		"Content-Length: 6\r\n\r\nabc"); err != nil {
		t.Fatal(err)
	}
	// Send the rest of the body only after the read timeout would have fired.
	time.Sleep(3 * short)
	if _, err := io.WriteString(conn, "def"); err != nil {
		t.Fatalf("writing the tail of a slow upload: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("reading the upload response: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("slow upload: got %d, want 200", resp.StatusCode)
	}
	select {
	case got := <-body:
		if got != "abcdef" {
			t.Fatalf("upstream received %q, want %q", got, "abcdef")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the upstream never received the body")
	}
}

// The policy itself: every data-plane listener carries all three timeouts.
func TestListenerTimeoutPolicy(t *testing.T) {
	srv := newListenerServer(":0", http.NotFoundHandler())
	if srv.ReadHeaderTimeout != readHeaderTimeout || srv.ReadTimeout != readTimeout || srv.IdleTimeout != idleTimeout {
		t.Fatalf("listener timeouts = header:%s read:%s idle:%s, want %s/%s/%s",
			srv.ReadHeaderTimeout, srv.ReadTimeout, srv.IdleTimeout,
			readHeaderTimeout, readTimeout, idleTimeout)
	}
	if readTimeout <= 0 || idleTimeout <= 0 {
		t.Fatal("readTimeout and idleTimeout must both be set")
	}
	// A bodiless, non-upgrade request keeps the listener's deadline; only the
	// long-lived shapes clear it.
	if isUpgradeRequest(httptest.NewRequest("GET", "/", nil)) {
		t.Fatal("a plain GET must not be treated as an upgrade")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "keep-alive, Upgrade")
	if !isUpgradeRequest(r) {
		t.Fatal("a websocket upgrade must be recognised when Connection lists several tokens")
	}
	r.Header.Set("Connection", "keep-alive")
	if isUpgradeRequest(r) {
		t.Fatal("an Upgrade header without a Connection: Upgrade token is not an upgrade")
	}
}
