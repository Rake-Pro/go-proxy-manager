package dataplane

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The outpost passthrough waits for the outpost's response headers no longer
// than the auth subrequest may take as a whole.
func TestAuthRequestPassthroughHeaderTimeoutMatchesSubrequest(t *testing.T) {
	p := newAuthReqProxy(t, "http://outpost.example.com:9000")
	tr := p.outpost.Transport.(*http.Transport)
	if tr.ResponseHeaderTimeout != p.client.Timeout || tr.ResponseHeaderTimeout <= 0 {
		t.Fatalf("passthrough ResponseHeaderTimeout = %v, want the subrequest bound %v", tr.ResponseHeaderTimeout, p.client.Timeout)
	}
}

// A stalled outpost is cut off with a 502 once the header bound passes, while a
// response whose headers arrived in time is relayed in full however long its
// body takes.
func TestAuthRequestPassthroughStallAndSlowBody(t *testing.T) {
	release := make(chan struct{})
	outpost := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/outpost.goauthentik.io/stall":
			<-release
		case "/outpost.goauthentik.io/slow-body":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("first,"))
			w.(http.Flusher).Flush()
			time.Sleep(400 * time.Millisecond)
			_, _ = w.Write([]byte("second"))
		}
	})
	srv := httptest.NewServer(outpost)
	defer srv.Close()
	defer close(release)

	p := newAuthReqProxy(t, srv.URL)
	// Shortened so the test is quick; the production value is pinned above.
	p.outpost.Transport.(*http.Transport).ResponseHeaderTimeout = 150 * time.Millisecond
	h := p.handler(peerIP, nil, "app", nil, okHandler())

	cases := []struct {
		name     string
		path     string
		wantCode int
		wantBody string
		maxTime  time.Duration
	}{
		{"stalled outpost", "/outpost.goauthentik.io/stall", http.StatusBadGateway, "", 3 * time.Second},
		{"slow body after headers", "/outpost.goauthentik.io/slow-body", http.StatusOK, "first,second", 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "http://app.example.com"+tc.path, nil)
			start := time.Now()
			h.ServeHTTP(rec, req)
			if d := time.Since(start); d > tc.maxTime {
				t.Fatalf("took %v, want under %v", d, tc.maxTime)
			}
			if rec.Code != tc.wantCode {
				t.Fatalf("status %d, want %d", rec.Code, tc.wantCode)
			}
			if tc.wantBody != "" {
				if b, _ := io.ReadAll(rec.Body); string(b) != tc.wantBody {
					t.Fatalf("body %q, want %q", b, tc.wantBody)
				}
			}
		})
	}
}

// The outpost passthrough carries the derived client IP in X-Forwarded-For and
// X-Real-Ip, exactly as the auth subrequest's X-Forwarded-For does, and never a
// value the client sent itself.
func TestAuthRequestPassthroughSendsDerivedClientIP(t *testing.T) {
	type seen struct{ xff, xri, subXFF string }
	var got seen
	outpost := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/outpost.goauthentik.io/auth/nginx" {
			got.subXFF = r.Header.Get("X-Forwarded-For")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got.xff, got.xri = r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-Ip")
	})
	srv := httptest.NewServer(outpost)
	defer srv.Close()
	h := newAuthReqProxy(t, srv.URL).handler(peerIP, nil, "app", nil, okHandler())

	cases := []struct {
		name    string
		remote  string
		derived *clientIPInfo // nil: request reached the gate without dispatch
		want    string
	}{
		{"untrusted peer with forged headers", "203.0.113.5:4000", nil, "203.0.113.5"},
		{"untrusted peer, dispatch decision", "203.0.113.5:4000", &clientIPInfo{ip: net.ParseIP("203.0.113.5")}, "203.0.113.5"},
		{"trusted proxy in front of the client", "198.51.100.1:4000", &clientIPInfo{ip: net.ParseIP("192.0.2.10"), peerTrusted: true}, "192.0.2.10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = seen{}
			for _, path := range []string{"/outpost.goauthentik.io/start?rd=/", "/app"} {
				req := httptest.NewRequest("GET", "http://app.example.com"+path, nil)
				req.RemoteAddr = tc.remote
				req.Header.Set("X-Forwarded-For", "192.0.2.99")
				req.Header.Set("X-Real-Ip", "192.0.2.99")
				if tc.derived != nil {
					req = withClientIP(req, *tc.derived)
				}
				h.ServeHTTP(httptest.NewRecorder(), req)
			}
			if got.xff != tc.want || got.xri != tc.want {
				t.Fatalf("passthrough X-Forwarded-For %q, X-Real-Ip %q, want both %q", got.xff, got.xri, tc.want)
			}
			if got.subXFF != got.xff {
				t.Fatalf("passthrough X-Forwarded-For %q differs from the subrequest's %q", got.xff, got.subXFF)
			}
		})
	}

	// No derivable client address: a client-supplied X-Real-Ip must not pass.
	got = seen{xri: "unset"}
	req := httptest.NewRequest("GET", "http://app.example.com/outpost.goauthentik.io/start?rd=/", nil)
	req.Header.Set("X-Real-Ip", "192.0.2.99")
	h.ServeHTTP(httptest.NewRecorder(), withClientIP(req, clientIPInfo{}))
	if got.xri != "" {
		t.Fatalf("X-Real-Ip %q reached the outpost with no derived client address", got.xri)
	}
}
