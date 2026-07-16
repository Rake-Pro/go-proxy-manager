package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestRobotsNoIndexHeader proves a host opted into no-indexing emits X-Robots-Tag
// on both the HTTPS and HTTP serve paths, and a host without it emits nothing.
func TestRobotsNoIndexHeader(t *testing.T) {
	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{ObjectMeta: model.ObjectMeta{Name: "noindex"}, Domains: []string{"noindex.example.com"}, Upstream: up, RobotsNoIndex: true},
		{ObjectMeta: model.ObjectMeta{Name: "plain"}, Domains: []string{"plain.example.com"}, Upstream: up},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, host, scheme, want string
	}{
		{"https noindex", "noindex.example.com", "https", "noindex, nofollow"},
		{"http noindex", "noindex.example.com", "http", "noindex, nofollow"},
		{"https plain", "plain.example.com", "https", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tc.scheme+"://"+tc.host+"/", nil)
			req.Host = tc.host
			if tc.scheme == "https" {
				rt.serveHTTPS(rec, req)
			} else {
				rt.serveHTTP(rec, req)
			}
			if got := rec.Header().Get("X-Robots-Tag"); got != tc.want {
				t.Fatalf("X-Robots-Tag = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTransportFor proves a host with no timeout override reuses the shared pooled
// transport (so default behaviour and connection reuse are unchanged), while an
// override yields a distinct transport carrying the configured timeouts.
func TestTransportFor(t *testing.T) {
	if got := transportFor(nil); got != http.RoundTripper(dataplaneTransport) {
		t.Fatal("nil timeouts must reuse the shared transport")
	}
	if got := transportFor(&model.HostTimeouts{}); got != http.RoundTripper(dataplaneTransport) {
		t.Fatal("zero timeouts must reuse the shared transport")
	}
	rt := transportFor(&model.HostTimeouts{ConnectSeconds: 3, ReadSeconds: 7})
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("override must return an *http.Transport, got %T", rt)
	}
	if tr == dataplaneTransport {
		t.Fatal("override must clone, not mutate, the shared transport")
	}
	if tr.ResponseHeaderTimeout != 7*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want 7s", tr.ResponseHeaderTimeout)
	}
	if dataplaneTransport.ResponseHeaderTimeout == 7*time.Second {
		t.Fatal("shared transport must be left unchanged by a per-host override")
	}
}

// TestLogRing proves the ring returns newest-first and bounds memory by wrapping.
func TestLogRing(t *testing.T) {
	r := newLogRing(3)
	if got := r.recent(); len(got) != 0 {
		t.Fatalf("empty ring should be empty, got %d", len(got))
	}
	for i := 1; i <= 5; i++ {
		r.add(AccessEntry{Status: i})
	}
	got := r.recent()
	if len(got) != 3 {
		t.Fatalf("ring should cap at 3, got %d", len(got))
	}
	want := []int{5, 4, 3} // newest first, oldest two evicted
	for i, w := range want {
		if got[i].Status != w {
			t.Fatalf("recent()[%d].Status = %d, want %d", i, got[i].Status, w)
		}
	}
}
