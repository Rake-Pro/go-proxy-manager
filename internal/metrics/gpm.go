package metrics

import (
	"net/http"
	"runtime"
	"time"
)

// durationBuckets are the fixed request-latency buckets, in seconds. They span
// a reverse proxy's useful range: sub-10ms cache hits through the 10s mark where
// a request is a problem rather than a data point.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Metrics is gpm's concrete metric set over a Registry. Its data-plane methods
// are exactly the dataplane.MetricsHook interface, which internal/dataplane
// declares structurally so it never has to import this package (or, through it,
// anything else).
type Metrics struct {
	Registry *Registry

	httpRequests   *Counter
	httpDuration   *Histogram
	inFlight       *Gauge
	requestBytes   *Counter
	responseBytes  *Counter
	upstreamErrors *Counter
	wsUpgrades     *Counter
	denials        *Counter
	streamActive   *Gauge
	streamTotal    *Counter
}

// NewMetrics registers the always-on metric set (data plane + build info + Go
// runtime). The optional subsystem collectors are attached by the Register*
// methods below, so an instance that does not run a subsystem exports no
// misleading zeroes for it.
func NewMetrics(version, commit, goVersion string) *Metrics {
	r := New()
	m := &Metrics{
		Registry: r,
		httpRequests: r.Counter(Namespace+"http_requests_total",
			"Data-plane HTTP requests, by proxy host, method and status class.",
			"host", "method", "status"),
		httpDuration: r.Histogram(Namespace+"http_request_duration_seconds",
			"Data-plane HTTP request duration in seconds, by proxy host.",
			durationBuckets, "host"),
		inFlight: r.Gauge(Namespace+"http_requests_in_flight",
			"Data-plane HTTP requests currently being served."),
		requestBytes: r.Counter(Namespace+"http_request_bytes_total",
			"Data-plane request body bytes read from clients, by proxy host.", "host"),
		responseBytes: r.Counter(Namespace+"http_response_bytes_total",
			"Data-plane response body bytes written to clients, by proxy host.", "host"),
		upstreamErrors: r.Counter(Namespace+"http_upstream_errors_total",
			"Upstream failures the reverse proxy answered with 502, by proxy host.", "host"),
		wsUpgrades: r.Counter(Namespace+"http_websocket_upgrades_total",
			"Successful WebSocket (protocol) upgrades, by proxy host.", "host"),
		denials: r.Counter(Namespace+"denials_total",
			"Requests refused by an access-control tier, by proxy host and reason.",
			"host", "reason"),
		streamActive: r.Gauge(Namespace+"stream_connections_active",
			"Raw TCP/UDP stream connections currently open, by stream host.", "host"),
		streamTotal: r.Counter(Namespace+"stream_connections_total",
			"Raw TCP/UDP stream connections accepted, by stream host.", "host"),
	}
	// The Prometheus convention: a constant 1 whose labels carry the identity, so
	// a dashboard can join on it and an upgrade shows as a series changing rather
	// than a value changing.
	r.GaugeFunc(Namespace+"build_info",
		"Always 1; the labels carry the running build identity.",
		[]string{"version", "commit", "go"},
		func() []Sample { return []Sample{{Labels: []string{version, commit, goVersion}, Value: 1}} })
	m.registerRuntime()
	return m
}

// registerRuntime exports the two runtime numbers worth alerting on. Anything
// deeper is what -pprof is for.
func (m *Metrics) registerRuntime() {
	m.Registry.GaugeFunc(Namespace+"go_goroutines", "Goroutines currently running.", nil,
		func() []Sample { return []Sample{{Value: int64(runtime.NumGoroutine())}} })
	m.Registry.GaugeFunc(Namespace+"go_memstats_alloc_bytes", "Heap bytes currently allocated.", nil,
		func() []Sample {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			return []Sample{{Value: int64(ms.Alloc)}}
		})
	m.Registry.GaugeFunc(Namespace+"go_memstats_sys_bytes", "Bytes obtained from the OS.", nil,
		func() []Sample {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			return []Sample{{Value: int64(ms.Sys)}}
		})
}

// Handler serves the exposition. The caller authenticates and authorizes first.
func (m *Metrics) Handler() http.Handler { return m.Registry.Handler() }

// --- data-plane hook (see dataplane.MetricsHook) -----------------------------

// RequestStarted / RequestFinished move the in-flight gauge.
func (m *Metrics) RequestStarted()  { m.inFlight.Add(1) }
func (m *Metrics) RequestFinished() { m.inFlight.Add(-1) }

// HTTPRequest records one completed data-plane request. host is the operator's
// ProxyHost NAME, never the client's Host header, so the label set is bounded by
// the config rather than by whatever an attacker sends.
func (m *Metrics) HTTPRequest(host, method string, status int, dur time.Duration, bytesIn, bytesOut int64) {
	m.httpRequests.Inc(host, method, statusClass(status))
	m.httpDuration.Observe(dur, host)
	m.requestBytes.Add(bytesIn, host)
	m.responseBytes.Add(bytesOut, host)
}

func (m *Metrics) UpstreamError(host string)    { m.upstreamErrors.Inc(host) }
func (m *Metrics) WebsocketUpgrade(host string) { m.wsUpgrades.Inc(host) }
func (m *Metrics) Denial(host, reason string)   { m.denials.Inc(host, reason) }

func (m *Metrics) StreamOpened(host string) {
	m.streamTotal.Inc(host)
	m.streamActive.Add(1, host)
}

func (m *Metrics) StreamClosed(host string) { m.streamActive.Add(-1, host) }

// statusClass collapses a status code to its class, so the status label can
// never carry more than a handful of values.
func statusClass(code int) string {
	switch {
	case code < 100:
		return "other"
	case code < 200:
		return "1xx"
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	case code < 600:
		return "5xx"
	}
	return "other"
}

// --- pull-based subsystem collectors -----------------------------------------

// CertStatus is one ACME certificate's observable state.
type CertStatus struct {
	Name          string
	NotAfter      time.Time
	RenewFailures int64
}

// DNSBackendStatus is one DNS-sync backend's last-run counts.
type DNSBackendStatus struct {
	Name    string
	Enabled bool
	OK      bool
	Desired int
	Managed int
}

// DNSSyncStatus is the DNS reconciler's observable state.
type DNSSyncStatus struct {
	LastRun     time.Time
	LastSuccess time.Time
	Backends    []DNSBackendStatus
}

// IngressStatus is the Kubernetes Ingress-discovery reconciler's observable state.
type IngressStatus struct {
	Enabled     bool
	LastRun     time.Time
	LastSuccess time.Time
	Discovered  int
	Managed     int
}

// RegisterACME exports per-certificate expiry and renewal-failure counts. Only
// the HA leader runs the ACME manager, so only it calls this.
func (m *Metrics) RegisterACME(fn func() []CertStatus) {
	m.Registry.GaugeFunc(Namespace+"acme_certificate_expiry_timestamp_seconds",
		"Unix timestamp at which an ACME certificate expires, by certificate name.",
		[]string{"certificate"},
		func() []Sample {
			var out []Sample
			for _, c := range fn() {
				if c.NotAfter.IsZero() {
					continue
				}
				out = append(out, Sample{Labels: []string{c.Name}, Value: c.NotAfter.Unix()})
			}
			return out
		})
	m.Registry.CounterFunc(Namespace+"acme_renew_failures_total",
		"Failed issue/renew attempts since start, by certificate name.",
		[]string{"certificate"},
		func() []Sample {
			var out []Sample
			for _, c := range fn() {
				out = append(out, Sample{Labels: []string{c.Name}, Value: c.RenewFailures})
			}
			return out
		})
}

// RegisterDNSSync exports the DNS reconciler's run timestamps and per-backend
// record counts.
func (m *Metrics) RegisterDNSSync(fn func() DNSSyncStatus) {
	m.Registry.GaugeFunc(Namespace+"dns_sync_last_run_timestamp_seconds",
		"Unix timestamp of the last DNS reconcile, successful or not (0 = never run).", nil,
		func() []Sample { return []Sample{{Value: unix(fn().LastRun)}} })
	m.Registry.GaugeFunc(Namespace+"dns_sync_last_success_timestamp_seconds",
		"Unix timestamp of the last DNS reconcile that fully succeeded (0 = never).", nil,
		func() []Sample { return []Sample{{Value: unix(fn().LastSuccess)}} })
	m.Registry.GaugeFunc(Namespace+"dns_sync_backend_up",
		"1 when the last reconcile against a DNS backend succeeded, 0 otherwise.",
		[]string{"backend"},
		func() []Sample {
			var out []Sample
			for _, b := range fn().Backends {
				if !b.Enabled {
					continue
				}
				out = append(out, Sample{Labels: []string{b.Name}, Value: boolGauge(b.OK)})
			}
			return out
		})
	m.Registry.GaugeFunc(Namespace+"dns_sync_records_desired",
		"DNS records the config asks a backend to hold.", []string{"backend"},
		func() []Sample {
			var out []Sample
			for _, b := range fn().Backends {
				if !b.Enabled {
					continue
				}
				out = append(out, Sample{Labels: []string{b.Name}, Value: int64(b.Desired)})
			}
			return out
		})
	m.Registry.GaugeFunc(Namespace+"dns_sync_records_managed",
		"DNS records gpm owns in a backend (ownership-ledger entries).", []string{"backend"},
		func() []Sample {
			var out []Sample
			for _, b := range fn().Backends {
				if !b.Enabled {
					continue
				}
				out = append(out, Sample{Labels: []string{b.Name}, Value: int64(b.Managed)})
			}
			return out
		})
}

// RegisterIngressDiscovery exports the Ingress reconciler's run timestamps and
// counts. LastRun and LastSuccess are separate because freeze-on-error is a
// state where they diverge - which is exactly what an alert should catch.
func (m *Metrics) RegisterIngressDiscovery(fn func() IngressStatus) {
	m.Registry.GaugeFunc(Namespace+"ingress_discovery_enabled",
		"1 when Kubernetes Ingress discovery is turned on in settings.", nil,
		func() []Sample { return []Sample{{Value: boolGauge(fn().Enabled)}} })
	m.Registry.GaugeFunc(Namespace+"ingress_discovery_last_run_timestamp_seconds",
		"Unix timestamp of the last Ingress reconcile, successful or not (0 = never run).", nil,
		func() []Sample { return []Sample{{Value: unix(fn().LastRun)}} })
	m.Registry.GaugeFunc(Namespace+"ingress_discovery_last_success_timestamp_seconds",
		"Unix timestamp of the last Ingress reconcile that completed cleanly (0 = never).", nil,
		func() []Sample { return []Sample{{Value: unix(fn().LastSuccess)}} })
	m.Registry.GaugeFunc(Namespace+"ingress_discovery_discovered_ingresses",
		"Annotated Ingresses the last successful cluster list held.", nil,
		func() []Sample { return []Sample{{Value: int64(fn().Discovered)}} })
	m.Registry.GaugeFunc(Namespace+"ingress_discovery_managed_hosts",
		"Proxy hosts labelled as owned by Ingress discovery.", nil,
		func() []Sample { return []Sample{{Value: int64(fn().Managed)}} })
}

// AccessListSyncStatus is the access-list source fetcher's observable state.
// Refused is the number to alert on: those sources are still serving their
// PREVIOUSLY fetched set, so a stale feed denies or admits by yesterday's data
// with nothing else in the exposition to give it away.
type AccessListSyncStatus struct {
	Enabled     bool
	LastRun     time.Time
	LastSuccess time.Time
	Sources     int
	Refused     int
}

// RegisterAccessListSync exports the access-list source fetcher's run timestamps
// and refusal count, so a staleness alert lives beside the DNS-sync and Ingress
// ones instead of in a scripted poll of the status endpoint.
func (m *Metrics) RegisterAccessListSync(fn func() AccessListSyncStatus) {
	m.Registry.GaugeFunc(Namespace+"access_list_sync_enabled",
		"1 when access-list source fetching is turned on in settings.", nil,
		func() []Sample { return []Sample{{Value: boolGauge(fn().Enabled)}} })
	m.Registry.GaugeFunc(Namespace+"access_list_sync_last_run_timestamp_seconds",
		"Unix timestamp of the last access-list source reconcile, successful or not (0 = never run).", nil,
		func() []Sample { return []Sample{{Value: unix(fn().LastRun)}} })
	m.Registry.GaugeFunc(Namespace+"access_list_sync_last_success_timestamp_seconds",
		"Unix timestamp of the last access-list source reconcile that completed cleanly (0 = never).", nil,
		func() []Sample { return []Sample{{Value: unix(fn().LastSuccess)}} })
	m.Registry.GaugeFunc(Namespace+"access_list_sync_sources",
		"Access-list sources declared by the current config.", nil,
		func() []Sample { return []Sample{{Value: int64(fn().Sources)}} })
	m.Registry.GaugeFunc(Namespace+"access_list_sync_refused_sources",
		"Sources whose most recent fetch was refused; each still serves its previously fetched set.", nil,
		func() []Sample { return []Sample{{Value: int64(fn().Refused)}} })
}

// RegisterHA exports this instance's static HA role as a 1/0 gauge per role, so
// a two-node pair with two leaders (or none) is a query rather than a log hunt.
func (m *Metrics) RegisterHA(role string) {
	m.Registry.GaugeFunc(Namespace+"ha_role",
		"1 for this instance's configured HA role, 0 for the others.",
		[]string{"role"},
		func() []Sample {
			return []Sample{
				{Labels: []string{"leader"}, Value: boolGauge(role == "leader")},
				{Labels: []string{"follower"}, Value: boolGauge(role == "follower")},
			}
		})
}

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func boolGauge(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
