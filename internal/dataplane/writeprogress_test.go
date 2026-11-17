package dataplane

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func withWriteIdleTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := responseWriteIdleTimeout
	responseWriteIdleTimeout = d
	t.Cleanup(func() { responseWriteIdleTimeout = prev })
}

type countingBufferPool struct {
	inner httputil.BufferPool
	out   atomic.Int64
}

func (p *countingBufferPool) Get() []byte  { p.out.Add(1); return p.inner.Get() }
func (p *countingBufferPool) Put(b []byte) { p.out.Add(-1); p.inner.Put(b) }

// bigUpstream streams up to 256 MiB and counts the requests still being served.
func bigUpstream(t *testing.T, active *atomic.Int64) model.Upstream {
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active.Add(1)
		defer active.Add(-1)
		chunk := make([]byte, 64<<10)
		for i := 0; i < 4096; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	t.Cleanup(closeFn)
	return up
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// h2Listener serves h on a TLS listener offering h2 and returns its address.
func h2Listener(t *testing.T, h http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := newListenerServer(ln.Addr().String(), h)
	srv.TLSConfig = &tls.Config{NextProtos: []string{"h2", "http/1.1"}, Certificates: []tls.Certificate{testTLSCert(t)}}
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func h2Client(t *testing.T, streamWindow int) *http.Client {
	t.Helper()
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
		HTTP2:             &http.HTTP2Config{MaxReceiveBufferPerStream: streamWindow},
	}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr}
}

// smallSendBufListener shrinks each accepted connection's send buffer, so a
// slow reader holds back the server's writes instead of the kernel absorbing
// the whole response.
type smallSendBufListener struct{ net.Listener }

func (l smallSendBufListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetWriteBuffer(16 << 10)
	}
	return c, err
}

// TestResponseWriteStallReleasesProxyResources: a client that stops reading a
// proxied response is cut off once a write makes no progress for
// responseWriteIdleTimeout, releasing the copy buffer and the upstream request.
func TestResponseWriteStallReleasesProxyResources(t *testing.T) {
	const idle = 300 * time.Millisecond
	withWriteIdleTimeout(t, idle)
	pool := &countingBufferPool{inner: &bufferPool{}}
	prevPool := proxyBufPool
	proxyBufPool = pool
	t.Cleanup(func() { proxyBufPool = prevPool })

	var active atomic.Int64
	h := newReverseProxy(bigUpstream(t, &active), "app", nil, nil, nil, nil)

	stallHTTP1 := func(t *testing.T, extraHeaders string) {
		addr := listenWithPolicy(t, h)
		const n = 5
		for i := 0; i < n; i++ {
			c, err := net.Dial("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = c.Close() })
			_ = c.(*net.TCPConn).SetReadBuffer(4096)
			if _, err := io.WriteString(c, "GET /big HTTP/1.1\r\nHost: app.example.com\r\n"+extraHeaders+"\r\n"); err != nil {
				t.Fatal(err)
			}
		}
		if !waitFor(t, 2*time.Second, func() bool { return active.Load() == n }) {
			t.Fatalf("upstream requests in flight = %d, want %d", active.Load(), n)
		}
		if !waitFor(t, 20*idle, func() bool { return active.Load() == 0 && pool.out.Load() == 0 }) {
			t.Fatalf("stalled clients still hold %d upstream requests and %d copy buffers", active.Load(), pool.out.Load())
		}
	}
	t.Run("http1", func(t *testing.T) { stallHTTP1(t, "") })
	// Upgrade headers the upstream ignores: the ordinary response that follows
	// is bounded like any other.
	t.Run("http1 upgrade not accepted", func(t *testing.T) {
		stallHTTP1(t, "Connection: Upgrade\r\nUpgrade: websocket\r\n")
	})

	t.Run("http2", func(t *testing.T) {
		addr := h2Listener(t, h)
		client := h2Client(t, 16<<10)
		for i := 0; i < 3; i++ {
			resp, err := client.Get("https://" + addr + "/big")
			if err != nil {
				t.Fatal(err)
			}
			if resp.ProtoMajor != 2 {
				t.Fatalf("negotiated %s, want HTTP/2", resp.Proto)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			// The body is never read.
		}
		if !waitFor(t, 20*idle, func() bool { return active.Load() == 0 && pool.out.Load() == 0 }) {
			t.Fatalf("stalled streams still hold %d upstream requests and %d copy buffers", active.Load(), pool.out.Load())
		}
	})
}

// TestResponseWriteProgressKeepsIdleAndSlowStreams: the write deadline bounds
// only a write in progress. A server-sent-events stream that is quiet for
// longer than the window, a handler that waits before returning trailers, and
// a client that reads slowly but steadily for many windows all complete.
func TestResponseWriteProgressKeepsIdleAndSlowStreams(t *testing.T) {
	const idle = 300 * time.Millisecond
	withWriteIdleTimeout(t, idle)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Trailer", "X-Done")
			rc := http.NewResponseController(w)
			for i := 0; i < 2; i++ {
				_, _ = io.WriteString(w, "data: tick\n\n")
				_ = rc.Flush()
				time.Sleep(3 * idle)
			}
			w.Header().Set("X-Done", "yes")
		case "/slow":
			w.Header().Set("Content-Length", "1048576")
			_, _ = w.Write(make([]byte, 1<<20))
		}
	}))
	t.Cleanup(closeFn)
	h := newReverseProxy(up, "app", nil, nil, nil, nil)

	checkEvents := func(t *testing.T, client *http.Client, url string) {
		t.Helper()
		resp, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read events (%s): %v", resp.Proto, err)
		}
		if got := strings.Count(string(body), "data: tick"); got != 2 {
			t.Errorf("%s: got %d events, want 2", resp.Proto, got)
		}
		if resp.Trailer.Get("X-Done") != "yes" {
			t.Errorf("%s: trailer X-Done = %q, want yes", resp.Proto, resp.Trailer.Get("X-Done"))
		}
	}

	t.Run("http1", func(t *testing.T) {
		var slowServe atomic.Int64 // how long the proxy handler spent on /slow
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		srv := newListenerServer(ln.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			h.ServeHTTP(w, r)
			if r.URL.Path == "/slow" {
				slowServe.Store(int64(time.Since(start)))
			}
		}))
		go func() { _ = srv.Serve(smallSendBufListener{ln}) }()
		t.Cleanup(func() { _ = srv.Close() })
		addr := ln.Addr().String()
		checkEvents(t, http.DefaultClient, "http://"+addr+"/events")

		// Slow, steady reader on a small socket buffer: 1 MiB at 32 KiB per
		// 50 ms takes several windows in total, but every write progresses.
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		_ = c.(*net.TCPConn).SetReadBuffer(64 << 10)
		if _, err := io.WriteString(c, "GET /slow HTTP/1.1\r\nHost: app.example.com\r\nConnection: close\r\n\r\n"); err != nil {
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		buf := make([]byte, 32<<10)
		for {
			n, err := io.ReadFull(resp.Body, buf)
			total += n
			if err != nil {
				if err != io.EOF && err != io.ErrUnexpectedEOF {
					t.Fatalf("slow read after %d bytes: %v", total, err)
				}
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if total != 1<<20 {
			t.Fatalf("slow reader got %d bytes, want %d", total, 1<<20)
		}
		// The proxy's writes were held back by the reader for several windows.
		if d := time.Duration(slowServe.Load()); d < 3*idle {
			t.Fatalf("proxy handler finished in %v, want writes blocked for several windows", d)
		}
	})

	t.Run("http2", func(t *testing.T) {
		addr := h2Listener(t, h)
		checkEvents(t, h2Client(t, 0), "https://"+addr+"/events")
	})
}

// TestProgressWriterLeavesNoDeadlineOnReusedConnection: after a proxied
// response, a later response on the same keep-alive connection that does not go
// through the proxy is written normally even after the window has passed.
func TestProgressWriterLeavesNoDeadlineOnReusedConnection(t *testing.T) {
	const idle = 200 * time.Millisecond
	withWriteIdleTimeout(t, idle)
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxied")
	}))
	t.Cleanup(closeFn)
	proxy := newReverseProxy(up, "app", nil, nil, nil, nil)
	addr := listenWithPolicy(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/local" {
			_, _ = io.WriteString(w, "local")
			return
		}
		proxy.ServeHTTP(w, r)
	}))

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	br := bufio.NewReader(c)
	for i, path := range []string{"/up", "/local"} {
		if i > 0 {
			time.Sleep(3 * idle)
		}
		if _, err := io.WriteString(c, "GET "+path+" HTTP/1.1\r\nHost: app.example.com\r\n\r\n"); err != nil {
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
	}
}
