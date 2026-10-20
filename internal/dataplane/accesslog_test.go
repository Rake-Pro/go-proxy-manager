package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
