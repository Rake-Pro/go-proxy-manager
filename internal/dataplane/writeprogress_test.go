package dataplane

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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

// TestResponseFlushedUnderDeadlineAfterHandlerReturns: a response is flushed
// under a write deadline after the handler returns, on every data-plane
// listener. A client that has not accepted the rest of a generated response
// once the deadline passes has its stream ended, and a client that reads
// normally gets the whole body.
func TestResponseFlushedUnderDeadlineAfterHandlerReturns(t *testing.T) {
	const idle = 300 * time.Millisecond
	withWriteIdleTimeout(t, idle)
	body := strings.Repeat("x", 2048) // fits the server's per-stream write buffer
	returned := make(chan struct{}, 2)
	addr := h2Listener(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
		returned <- struct{}{}
	}))

	resp, err := h2Client(t, 0).Get("https://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(got) != body {
		t.Fatalf("normal client: read %d bytes, err %v; want the whole body", len(got), err)
	}
	<-returned

	// A one-byte receive window: the server can send only the first byte of
	// the body before the handler has returned.
	resp, err = h2Client(t, 1).Get("https://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	<-returned
	time.Sleep(5 * idle)
	got, err = io.ReadAll(resp.Body)
	if err == nil {
		t.Fatalf("read the whole body (%d bytes) %v after the handler returned: the final flush had no write deadline", len(got), 5*idle)
	}
}

// deadlineRecorder is a ResponseWriter that records write deadlines and can
// be hijacked.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.deadlines = append(d.deadlines, t)
	return nil
}

func (d *deadlineRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, peer := net.Pipe()
	_ = peer.Close()
	return c, bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c)), nil
}

// The listener bounds each write the handler makes and arms the write deadline
// again once the handler has returned. No deadline is left set between writes,
// a hijacked connection is left alone, and the HTTP/2 stream limit is
// advertised.
func TestListenerArmsWriteDeadlineAfterHandler(t *testing.T) {
	withWriteIdleTimeout(t, time.Minute)
	var during int
	var rec *deadlineRecorder
	srv := newListenerServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		during = len(rec.deadlines)
		if r.URL.Path == "/hijack" {
			c, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Errorf("hijack through the listener wrapper: %v", err)
				return
			}
			_ = c.Close()
			return
		}
		if _, ok := w.(http.Flusher); !ok {
			t.Error("the listener wrapper hides http.Flusher")
		}
		_, _ = io.WriteString(w, "ok")
	}))
	if srv.HTTP2 == nil || srv.HTTP2.MaxConcurrentStreams != http2MaxConcurrentStreams {
		t.Fatalf("HTTP2 config = %+v, want MaxConcurrentStreams %d", srv.HTTP2, http2MaxConcurrentStreams)
	}

	rec = &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	start := time.Now()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if during != 0 {
		t.Fatalf("a write deadline was set before the handler ran: %v", rec.deadlines)
	}
	// One armed/cleared pair for the write, then the deadline for the flush
	// after the handler returned.
	d := rec.deadlines
	if len(d) != 3 || d[0].Before(start.Add(time.Minute)) || !d[1].IsZero() || d[2].Before(start.Add(time.Minute)) {
		t.Fatalf("deadlines = %v, want armed and cleared around the write, then one at least %v ahead", d, time.Minute)
	}

	rec = &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/hijack", nil))
	if len(rec.deadlines) != 0 {
		t.Fatalf("a hijacked connection got write deadlines %v", rec.deadlines)
	}
}

// generatedResponseServer serves cfg through the full data-plane chain on an
// h2 listener and counts the requests whose handler has not returned yet.
func generatedResponseServer(t *testing.T, cfg model.Config) (string, *atomic.Int64) {
	t.Helper()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config does not validate: %v", err)
	}
	s := New(Config{})
	if err := s.Reload(cfg); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	t.Cleanup(s.health.stopAll)
	inner := s.dataHandler(s.dispatchHTTPS)
	var inHandler atomic.Int64
	addr := h2Listener(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inHandler.Add(1)
		defer inHandler.Add(-1)
		inner.ServeHTTP(w, r)
	}))
	return addr, &inHandler
}

// stallGenerated opens n requests for url on one HTTP/2 connection whose
// receive window is a single byte and never reads the bodies.
func stallGenerated(t *testing.T, addr, host, path string, n int) {
	t.Helper()
	client := h2Client(t, 1)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for i := 0; i < n; i++ {
		req, _ := http.NewRequest("GET", "https://"+addr+path, nil)
		req.Host = host
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
}

// TestGeneratedResponseWritesUnderDeadline: a response gpm writes itself
// (a configured error page, a redirect) is written under the same per-write
// progress deadline as a proxied one, however large its body. A client that
// does not accept the body ends the write once the deadline passes, and the
// handler returns. A client that reads normally gets the whole page.
func TestGeneratedResponseWritesUnderDeadline(t *testing.T) {
	const idle = 300 * time.Millisecond
	withWriteIdleTimeout(t, idle)

	t.Run("error page larger than the stream buffer", func(t *testing.T) {
		page := "<html><body>" + strings.Repeat("refused ", 4<<10) + "</body></html>"
		if err := SetErrorPages(model.ErrorPagesConfig{Inline: map[string]string{"403": page}}, ""); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = SetErrorPages(model.ErrorPagesConfig{}, "") })
		addr, inHandler := generatedResponseServer(t, model.Config{
			AccessLists: []model.AccessList{{ObjectMeta: model.ObjectMeta{Name: "nobody"}, DefaultAction: "deny"}},
			ProxyHosts: []model.ProxyHost{{ObjectMeta: model.ObjectMeta{Name: "closed"}, Domains: []string{"closed.example.com"},
				Upstream: model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}, AccessLists: []string{"nobody"}}},
		})

		req, _ := http.NewRequest("GET", "https://"+addr+"/", nil)
		req.Host = "closed.example.com"
		resp, err := h2Client(t, 0).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || err != nil || string(got) != page {
			t.Fatalf("normal client: status %d, %d of %d bytes, err %v", resp.StatusCode, len(got), len(page), err)
		}

		stallGenerated(t, addr, "closed.example.com", "/", 5)
		if !waitFor(t, 10*idle, func() bool { return inHandler.Load() == 0 }) {
			t.Fatalf("%d error-page writes still in progress %v after the deadline", inHandler.Load(), 10*idle)
		}
	})

	t.Run("redirect echoing a long path", func(t *testing.T) {
		addr, inHandler := generatedResponseServer(t, model.Config{
			RedirectHosts: []model.RedirectHost{{ObjectMeta: model.ObjectMeta{Name: "moved"}, Domains: []string{"old.example.com"},
				TargetDomain: "new.example.com", PreservePath: true}},
		})
		stallGenerated(t, addr, "old.example.com", "/"+strings.Repeat("a", 8<<10), 5)
		if !waitFor(t, 10*idle, func() bool { return inHandler.Load() == 0 }) {
			t.Fatalf("%d redirect writes still in progress %v after the deadline", inHandler.Load(), 10*idle)
		}
	})
}

// A proxied response behind the listener is bounded by one progress deadline
// per write: every armed deadline is cleared before the next is armed, apart
// from the final one for the flush after the handler returned.
func TestProxiedResponseWritesBoundOnce(t *testing.T) {
	withWriteIdleTimeout(t, time.Minute)
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "proxied")
	}))
	t.Cleanup(closeFn)
	srv := newListenerServer("", newReverseProxy(up, "app", nil, nil, nil, nil))
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "http://app.example.com/", nil))
	if rec.Body.String() != "proxied" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	d := rec.deadlines
	if len(d) < 3 || d[len(d)-1].IsZero() {
		t.Fatalf("deadlines = %v, want armed/cleared pairs and a final armed one", d)
	}
	for i := 0; i < len(d)-1; i++ {
		if d[i].IsZero() != (i%2 == 1) {
			t.Fatalf("deadlines = %v: a write was bounded by more than one progress deadline", d)
		}
	}
}

// Each proxied response holds one copy buffer for as long as its client takes
// to read it, so the buffer is kept at the size of one progress-bounded write.
func TestProxyCopyBufferSize(t *testing.T) {
	if n := len((&bufferPool{}).Get()); n != progressWriteChunk {
		t.Fatalf("proxy copy buffer = %d bytes, want %d", n, progressWriteChunk)
	}
}
