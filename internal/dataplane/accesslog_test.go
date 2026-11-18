package dataplane

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/go-proxy-manager/internal/analytics"
)

// The access-log toggle must flip live: with every startup toggle off the
// switch serves the plain chain (no observer at all), enabling capture swaps
// the observed chain in so requests land in the ring, and disabling swaps the
// plain chain back.
func TestAccessLogLiveToggle(t *testing.T) {
	SetMetricsHook(nil)
	s := New(Config{})
	inner := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}
	hs := s.dataHandler(inner)
	s.httpSwitch = hs

	serve := func() {
		t.Helper()
		rec := httptest.NewRecorder()
		hs.ServeHTTP(rec, httptest.NewRequest("GET", "http://example.test/x", nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("status = %d, want 418", rec.Code)
		}
	}

	if hs.active.Load() != &hs.plain {
		t.Fatal("all toggles off: switch must start on the plain chain")
	}
	serve()
	if got := len(s.RecentLogs()); got != 0 {
		t.Fatalf("captured %d entries with access log off, want 0", got)
	}

	s.SetAccessLog(true)
	if !s.AccessLogEnabled() {
		t.Fatal("AccessLogEnabled = false after SetAccessLog(true)")
	}
	if hs.active.Load() != &hs.observed {
		t.Fatal("toggle on: switch must serve the observed chain")
	}
	serve()
	entries := s.RecentLogs()
	if len(entries) != 1 {
		t.Fatalf("captured %d entries with access log on, want 1", len(entries))
	}
	if entries[0].Status != http.StatusTeapot || entries[0].Host != "example.test" {
		t.Fatalf("entry = %+v", entries[0])
	}

	s.SetAccessLog(false)
	if s.AccessLogEnabled() {
		t.Fatal("AccessLogEnabled = true after SetAccessLog(false)")
	}
	if hs.active.Load() != &hs.plain {
		t.Fatal("toggle off: switch must return to the plain chain")
	}
	serve()
	if got := len(s.RecentLogs()); got != 1 {
		t.Fatalf("captured %d entries after disabling, want still 1", got)
	}

	// ClearLogs empties the ring without touching the capture state, and a
	// later capture lands in the empty ring.
	s.ClearLogs()
	if got := len(s.RecentLogs()); got != 0 {
		t.Fatalf("captured %d entries after ClearLogs, want 0", got)
	}
	s.SetAccessLog(true)
	serve()
	serve()
	if got := len(s.RecentLogs()); got != 2 {
		t.Fatalf("captured %d entries after clear + 2 requests, want 2", got)
	}
}

func TestLogRingClearAfterWrap(t *testing.T) {
	r := newLogRing(3)
	for i := 0; i < 5; i++ {
		r.add(AccessEntry{Status: i})
	}
	if got := len(r.recent()); got != 3 {
		t.Fatalf("len = %d, want 3", got)
	}
	r.clear()
	if got := len(r.recent()); got != 0 {
		t.Fatalf("len after clear = %d, want 0", got)
	}
	r.add(AccessEntry{Status: 7})
	r.add(AccessEntry{Status: 8})
	got := r.recent()
	if len(got) != 2 || got[0].Status != 8 || got[1].Status != 7 {
		t.Fatalf("after clear: %+v, want newest first [8 7]", got)
	}
}

// resize keeps the newest entries that fit and the ring keeps working at the
// new size, both when shrinking after a wrap and when growing.
func TestLogRingResize(t *testing.T) {
	r := newLogRing(3)
	for i := 1; i <= 5; i++ {
		r.add(AccessEntry{Status: i})
	}
	r.resize(2)
	if got := r.recent(); len(got) != 2 || got[0].Status != 5 || got[1].Status != 4 || r.capacity() != 2 {
		t.Fatalf("after shrink: %+v cap=%d, want [5 4] cap 2", got, r.capacity())
	}
	r.resize(10)
	r.add(AccessEntry{Status: 6})
	if got := r.recent(); len(got) != 3 || got[0].Status != 6 || got[2].Status != 4 || r.capacity() != 10 {
		t.Fatalf("after grow: %+v cap=%d, want [6 5 4] cap 10", got, r.capacity())
	}
	for i := 7; i <= 20; i++ {
		r.add(AccessEntry{Status: i})
	}
	if got := r.recent(); len(got) != 10 || got[0].Status != 20 || got[9].Status != 11 {
		t.Fatalf("after wrap at new size: len=%d first=%d last=%d, want 10/20/11", len(got), got[0].Status, got[len(got)-1].Status)
	}
}

// New clamps the configured buffer size and SetAccessLogCapacity rejects
// out-of-range values without touching the ring.
func TestSourceOf(t *testing.T) {
	SetInternalNetworks([]string{"198.51.100.0/24", "not-a-cidr"})
	t.Cleanup(func() { SetInternalNetworks(nil) })
	cases := map[string]string{
		"10.0.0.7": analytics.SourceInternal, "::ffff:192.168.1.1": analytics.SourceInternal, "fd00::1": analytics.SourceInternal,
		"127.0.0.1": analytics.SourceInternal, "198.51.100.9": analytics.SourceInternal,
		"203.0.113.9": analytics.SourceExternal, "2001:db8::1": analytics.SourceExternal,
	}
	for ip, want := range cases {
		if got := sourceOf(net.ParseIP(ip)); got != want {
			t.Errorf("sourceOf(%s) = %q, want %q", ip, got, want)
		}
	}
	if got := sourceOf(nil); got != "" {
		t.Errorf("sourceOf(nil) = %q", got)
	}
}

func TestAccessLogCapacityBounds(t *testing.T) {
	SetMetricsHook(nil)
	if got := New(Config{}).AccessLogCapacity(); got != DefaultAccessLogCapacity {
		t.Fatalf("default capacity = %d, want %d", got, DefaultAccessLogCapacity)
	}
	if got := New(Config{AccessLogBufferSize: 1}).AccessLogCapacity(); got != MinAccessLogCapacity {
		t.Fatalf("tiny config clamped to %d, want %d", got, MinAccessLogCapacity)
	}
	s := New(Config{AccessLogBufferSize: MaxAccessLogCapacity + 1})
	if got := s.AccessLogCapacity(); got != MaxAccessLogCapacity {
		t.Fatalf("huge config clamped to %d, want %d", got, MaxAccessLogCapacity)
	}
	if err := s.SetAccessLogCapacity(MinAccessLogCapacity - 1); err == nil {
		t.Fatal("below minimum accepted")
	}
	if err := s.SetAccessLogCapacity(MaxAccessLogCapacity + 1); err == nil {
		t.Fatal("above maximum accepted")
	}
	if got := s.AccessLogCapacity(); got != MaxAccessLogCapacity {
		t.Fatalf("rejected resize changed capacity to %d", got)
	}
	if err := s.SetAccessLogCapacity(250); err != nil || s.AccessLogCapacity() != 250 {
		t.Fatalf("resize to 250: err=%v cap=%d", err, s.AccessLogCapacity())
	}
}

// A startup toggle that needs the observer (here: a metrics hook) keeps the
// observed chain active even when access logging is switched off live; only
// the access-log work inside the wrapper is skipped.
func TestAccessLogToggleKeepsObserverForMetrics(t *testing.T) {
	SetMetricsHook(newFakeHook())
	t.Cleanup(func() { SetMetricsHook(nil) })
	s := New(Config{AccessLog: true})
	hs := s.dataHandler(func(http.ResponseWriter, *http.Request) {})
	s.httpsSwitch = hs

	if hs.active.Load() != &hs.observed {
		t.Fatal("metrics on: switch must serve the observed chain")
	}
	s.SetAccessLog(false)
	if hs.active.Load() != &hs.observed {
		t.Fatal("metrics on: disabling access log must keep the observed chain")
	}
	rec := httptest.NewRecorder()
	hs.ServeHTTP(rec, httptest.NewRequest("GET", "http://x.test/", nil))
	if got := len(s.RecentLogs()); got != 0 {
		t.Fatalf("captured %d entries with access log off, want 0", got)
	}
}

// captureLog sends the global logger to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })
	return &buf
}

// Method, Host and path are clipped to fixed bounds before they are stored in
// the access-log ring or written to the access line; ordinary values are kept
// as they are.
func TestAccessLogClipsRequestStrings(t *testing.T) {
	SetMetricsHook(nil)
	logs := captureLog(t)
	s := New(Config{AccessLog: true})
	hs := s.dataHandler(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })

	req := httptest.NewRequest("GET", "http://example.test/", nil)
	req.Method = strings.Repeat("M", 100)
	req.Host = strings.Repeat("h", 5000) + ".example.test:443"
	req.URL.Path = "/" + strings.Repeat("a", 100000)
	hs.ServeHTTP(httptest.NewRecorder(), req)

	entries := s.RecentLogs()
	if len(entries) != 1 {
		t.Fatalf("captured %d entries, want 1", len(entries))
	}
	e := entries[0]
	if len(e.Method) > logMethodMax+3 || len(e.Host) > logHostMax+3 || len(e.Path) > logPathMax+3 {
		t.Fatalf("entry lengths method=%d host=%d path=%d, want at most %d/%d/%d",
			len(e.Method), len(e.Host), len(e.Path), logMethodMax+3, logHostMax+3, logPathMax+3)
	}
	if !strings.HasSuffix(e.Path, "...") || !strings.HasPrefix(e.Path, "/aaa") {
		t.Fatalf("clipped path = %.20q...%q, want the start of the path and a marker", e.Path, e.Path[len(e.Path)-5:])
	}
	if n := logs.Len(); n > 4096 {
		t.Fatalf("access line is %d bytes, want it bounded", n)
	}

	logs.Reset()
	hs.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "http://app.example.test:8443/a/b", nil))
	e = s.RecentLogs()[0]
	if e.Method != "POST" || e.Host != "app.example.test" || e.Path != "/a/b" {
		t.Fatalf("ordinary request logged as %q %q %q", e.Method, e.Host, e.Path)
	}
	if !strings.Contains(logs.String(), `"path":"/a/b"`) {
		t.Fatalf("access line = %s", logs.String())
	}
}

// The upstream-error line carries the clipped path.
func TestUpstreamErrorLogClipsPath(t *testing.T) {
	logs := captureLog(t)
	h := newReverseProxy(deadUpstream(t), "app", nil, nil, nil, nil)
	req := httptest.NewRequest("GET", "http://app.example.test/", nil)
	req.URL.Path = "/" + strings.Repeat("a", 100000)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(logs.String(), "upstream error") {
		t.Fatalf("no upstream error line: %s", logs.String())
	}
	if n := logs.Len(); n > 4096 {
		t.Fatalf("upstream error line is %d bytes, want it bounded", n)
	}
}

// The line logged for a composed path outside the upstream base path carries
// the path clipped like every other log line.
func TestConfinedPathLogClipsPath(t *testing.T) {
	logs := captureLog(t)
	if got := confineUpstreamPath("/base", "/other/"+strings.Repeat("a", 100000)); got != "/base/" {
		t.Fatalf("confined path = %q, want /base/", got[:min(len(got), 32)])
	}
	if !strings.Contains(logs.String(), "escaped the upstream base path") {
		t.Fatalf("no confinement line: %.200s", logs.String())
	}
	if n := logs.Len(); n > 4096 {
		t.Fatalf("confinement line is %d bytes, want it bounded", n)
	}
}

// clear drops the old backing array, so cleared entries are not kept
// reachable until they happen to be overwritten.
func TestLogRingClearReleasesEntries(t *testing.T) {
	r := newLogRing(3)
	for i := 0; i < 3; i++ {
		r.add(AccessEntry{Path: strings.Repeat("p", 1000)})
	}
	r.clear()
	if cap(r.buf) != 0 {
		t.Fatalf("ring keeps a backing array of %d entries after clear", cap(r.buf))
	}
}
