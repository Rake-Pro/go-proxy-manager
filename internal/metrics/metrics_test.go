package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestExpositionGolden pins the exact bytes of the text exposition. The format
// is a wire contract with every Prometheus server that will ever scrape gpm, so
// it is checked byte for byte rather than "looks about right": a stray space, a
// missing # TYPE line or an unsorted series is a scrape that silently drops
// samples.
func TestExpositionGolden(t *testing.T) {
	r := New()
	c := r.Counter("gpm_test_requests_total", "Test counter.", "host", "method")
	g := r.Gauge("gpm_test_in_flight", "Test gauge.")
	h := r.Histogram("gpm_test_duration_seconds", "Test histogram.", []float64{0.01, 0.1}, "host")
	r.GaugeFunc("gpm_test_collected", "Test collector.", []string{"kind"}, func() []Sample {
		// Deliberately out of order: the writer sorts, so a collector cannot
		// make a scrape non-deterministic.
		return []Sample{{Labels: []string{"z"}, Value: 1}, {Labels: []string{"a"}, Value: 2}}
	})
	r.Counter(`gpm_test_escaped_total`, "Help with a \\ backslash.", "label").
		Inc(`va"lue` + "\n" + `\x`)

	c.Inc("a", "GET")
	c.Inc("a", "GET")
	c.Inc("b", "POST")
	g.Set(7)
	h.Observe(5*time.Millisecond, "a")
	h.Observe(50*time.Millisecond, "a")
	h.Observe(time.Second, "a")

	var b strings.Builder
	if _, err := r.WriteTo(&b); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	want := `# HELP gpm_test_requests_total Test counter.
# TYPE gpm_test_requests_total counter
gpm_test_requests_total{host="a",method="GET"} 2
gpm_test_requests_total{host="b",method="POST"} 1
# HELP gpm_test_in_flight Test gauge.
# TYPE gpm_test_in_flight gauge
gpm_test_in_flight 7
# HELP gpm_test_duration_seconds Test histogram.
# TYPE gpm_test_duration_seconds histogram
gpm_test_duration_seconds_bucket{host="a",le="0.01"} 1
gpm_test_duration_seconds_bucket{host="a",le="0.1"} 2
gpm_test_duration_seconds_bucket{host="a",le="+Inf"} 3
gpm_test_duration_seconds_sum{host="a"} 1.055
gpm_test_duration_seconds_count{host="a"} 3
# HELP gpm_test_collected Test collector.
# TYPE gpm_test_collected gauge
gpm_test_collected{kind="a"} 2
gpm_test_collected{kind="z"} 1
# HELP gpm_test_escaped_total Help with a \\ backslash.
# TYPE gpm_test_escaped_total counter
gpm_test_escaped_total{label="va\"lue\n\\x"} 1
`
	if got := b.String(); got != want {
		t.Fatalf("exposition mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestCardinalityBound is the load-bearing safety property: no matter how many
// distinct label sets arrive, one metric holds a bounded number of series. The
// host label is config-derived precisely so this cap is never reached in
// practice - this test proves the cap holds anyway if that rule is ever broken
// upstream of here.
func TestCardinalityBound(t *testing.T) {
	orig := MaxSeriesPerMetric
	MaxSeriesPerMetric = 3
	t.Cleanup(func() { MaxSeriesPerMetric = orig })

	r := New()
	c := r.Counter("gpm_test_bounded_total", "Bounded.", "host")
	for i := 0; i < 1000; i++ {
		c.Inc(string(rune('a' + i%26)))
	}

	var b strings.Builder
	if _, err := r.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	lines := 0
	overflow := 0
	for _, l := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		lines++
		if strings.Contains(l, OverflowLabel) {
			overflow++
		}
	}
	// 3 real series + exactly one overflow series, whatever arrives.
	if lines != MaxSeriesPerMetric+1 {
		t.Fatalf("series count = %d, want %d (cap + one overflow)\n%s", lines, MaxSeriesPerMetric+1, b.String())
	}
	if overflow != 1 {
		t.Fatalf("overflow series count = %d, want exactly 1\n%s", overflow, b.String())
	}
}

// TestHandlerContentType checks the scrape response carries the version marker
// Prometheus negotiates on; without it some scrapers fall back to protobuf.
func TestHandlerContentType(t *testing.T) {
	r := New()
	r.Gauge("gpm_test_x", "x.").Set(1)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "gpm_test_x 1") {
		t.Fatalf("body missing the sample:\n%s", rec.Body.String())
	}
}

// TestStatusClass pins the collapse of status codes to classes - the reason the
// status label can never grow past a handful of values.
func TestStatusClass(t *testing.T) {
	for _, tc := range []struct {
		code int
		want string
	}{
		{101, "1xx"}, {200, "2xx"}, {204, "2xx"}, {308, "3xx"},
		{403, "4xx"}, {429, "4xx"}, {502, "5xx"}, {0, "other"}, {999, "other"},
	} {
		if got := statusClass(tc.code); got != tc.want {
			t.Errorf("statusClass(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

// TestGPMMetricsHookRecords exercises the data-plane hook surface end to end and
// checks the samples land where the exposition says they do.
func TestGPMMetricsHookRecords(t *testing.T) {
	m := NewMetrics("v1.2.3", "abc1234", "go1.26.4")
	m.RequestStarted()
	m.HTTPRequest("app", "GET", 200, 20*time.Millisecond, 11, 22)
	m.RequestFinished()
	m.UpstreamError("app")
	m.WebsocketUpgrade("app")
	m.Denial("app", "rate-limit")
	m.StreamOpened("db")
	m.StreamClosed("db")
	m.RegisterHA("leader")

	var b strings.Builder
	if _, err := m.Registry.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		`gpm_build_info{version="v1.2.3",commit="abc1234",go="go1.26.4"} 1`,
		`gpm_http_requests_total{host="app",method="GET",status="2xx"} 1`,
		`gpm_http_request_bytes_total{host="app"} 11`,
		`gpm_http_response_bytes_total{host="app"} 22`,
		`gpm_http_requests_in_flight 0`,
		`gpm_http_upstream_errors_total{host="app"} 1`,
		`gpm_http_websocket_upgrades_total{host="app"} 1`,
		`gpm_denials_total{host="app",reason="rate-limit"} 1`,
		`gpm_stream_connections_total{host="db"} 1`,
		`gpm_stream_connections_active{host="db"} 0`,
		`gpm_ha_role{role="follower"} 0`,
		`gpm_ha_role{role="leader"} 1`,
		"gpm_http_request_duration_seconds_count{host=\"app\"} 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q\n%s", want, out)
		}
	}
}

// TestSubsystemCollectorsSkipUnsetTimestamps checks a never-run reconciler
// reports 0 rather than a negative unix timestamp from the zero time - a
// "seconds since last success" alert would otherwise read as ~62 billion.
func TestSubsystemCollectorsSkipUnsetTimestamps(t *testing.T) {
	m := NewMetrics("test", "c", "go")
	m.RegisterDNSSync(func() DNSSyncStatus {
		return DNSSyncStatus{Backends: []DNSBackendStatus{
			{Name: "pihole", Enabled: true, OK: true, Desired: 4, Managed: 3},
			{Name: "cloudflare"}, // disabled: must not appear at all
		}}
	})
	m.RegisterIngressDiscovery(func() IngressStatus { return IngressStatus{} })
	m.RegisterACME(func() []CertStatus {
		return []CertStatus{
			{Name: "wildcard", NotAfter: time.Unix(1893456000, 0), RenewFailures: 2},
			{Name: "never-issued"}, // zero expiry: no gauge, but the failure counter still exists
		}
	})

	var b strings.Builder
	if _, err := m.Registry.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"gpm_dns_sync_last_run_timestamp_seconds 0",
		"gpm_dns_sync_last_success_timestamp_seconds 0",
		`gpm_dns_sync_records_desired{backend="pihole"} 4`,
		`gpm_dns_sync_records_managed{backend="pihole"} 3`,
		`gpm_dns_sync_backend_up{backend="pihole"} 1`,
		"gpm_ingress_discovery_last_success_timestamp_seconds 0",
		`gpm_acme_certificate_expiry_timestamp_seconds{certificate="wildcard"} 1893456000`,
		`gpm_acme_renew_failures_total{certificate="never-issued"} 0`,
		`gpm_acme_renew_failures_total{certificate="wildcard"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, `backend="cloudflare"`) {
		t.Error("a disabled DNS backend must export no series (a 0 there reads as a real, empty backend)")
	}
	if strings.Contains(out, `gpm_acme_certificate_expiry_timestamp_seconds{certificate="never-issued"}`) {
		t.Error("a never-issued certificate must export no expiry gauge (0 would read as expired in 1970)")
	}
}
