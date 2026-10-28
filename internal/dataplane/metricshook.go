package dataplane

import (
	"context"
	"github.com/Rake-Pro/go-proxy-manager/internal/analytics"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// MetricsHook receives the data plane's observations for the optional admin
// /metrics endpoint. It is declared here, and satisfied structurally by
// internal/metrics, so this package never imports the metrics registry (nor,
// through it, internal/server) - the same decoupling ACMEChallengeStore uses for
// the ACME manager. Nothing is wired by default: with no hook installed every
// call site below is a single nil-pointer load.
//
// host is always the operator's ProxyHost NAME, never the client's Host header.
// A Host header is attacker-chosen and would let one client mint unbounded label
// series; a host name comes from committed config.
type MetricsHook interface {
	// RequestStarted and RequestFinished bracket a data-plane request for the
	// in-flight gauge.
	RequestStarted()
	RequestFinished()
	// HTTPRequest records one completed request.
	HTTPRequest(host, method string, status int, dur time.Duration, bytesIn, bytesOut int64)
	// UpstreamError records a backend failure answered with 502.
	UpstreamError(host string)
	// WebsocketUpgrade records a successful protocol upgrade.
	WebsocketUpgrade(host string)
	// Denial records a request an access-control tier refused.
	Denial(host, reason string)
	// MaintenanceResponse records a maintenance 503 served for a host.
	MaintenanceResponse(host string)
	// StreamOpened and StreamClosed bracket a raw TCP/UDP stream connection.
	StreamOpened(host string)
	StreamClosed(host string)
}

// metricsHookPtr holds the installed hook. Atomic so wiring it at startup races
// nothing, and so the common (unwired) case costs one atomic load.
var metricsHookPtr atomic.Pointer[MetricsHook]

// SetMetricsHook installs the data-plane metrics sink. Passing nil detaches it.
// Call it before the first Reload: the compiled chain reads the hook per
// request, but the listener wrappers decide at build time whether to observe at
// all.
func SetMetricsHook(h MetricsHook) {
	if h == nil {
		metricsHookPtr.Store(nil)
		return
	}
	metricsHookPtr.Store(&h)
}

// metricsHook returns the installed hook, or nil.
func metricsHook() MetricsHook {
	if p := metricsHookPtr.Load(); p != nil {
		return *p
	}
	return nil
}

// hostNameKey tags a request with the matched ProxyHost name so the
// access-control tiers deeper in the chain can label a denial without every
// middleware constructor having to take the host name as a parameter.
type hostNameKey struct{}

// unknownHostLabel is the host label for a request that matched no proxy host -
// an unknown Host header, or a redirect/parked host. Folding them all onto one
// label is what keeps an unrouted flood from inflating the series count.
const unknownHostLabel = "-"

func withHostName(r *http.Request, name string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), hostNameKey{}, name))
}

func hostNameOf(r *http.Request) string {
	if v, ok := r.Context().Value(hostNameKey{}).(string); ok && v != "" {
		return v
	}
	return unknownHostLabel
}

// reqMeta is the per-request note the access-control tiers leave for the
// analytics collector: which tier refused the request (first one wins) and an
// optional signature label (a named guard trigger, a CrowdSec scenario). The
// observe wrapper allocates it only while something is observing, and reads
// it after the handler returns; tiers reach it through the context pointer so
// a derived request (a child context) still writes to the same note.
type reqMeta struct {
	deny string
	sig  string
}

type reqMetaKey struct{}

func withReqMeta(r *http.Request, m *reqMeta) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), reqMetaKey{}, m))
}

func reqMetaOf(r *http.Request) *reqMeta {
	m, _ := r.Context().Value(reqMetaKey{}).(*reqMeta)
	return m
}

// countDenial records a request refused by an access-control tier: the
// Prometheus counter (when metrics are on) and the analytics note (when the
// request is observed). Both are a single pointer load when off.
func countDenial(r *http.Request, reason string) {
	if h := metricsHook(); h != nil {
		h.Denial(hostNameOf(r), reason)
	}
	if m := reqMetaOf(r); m != nil && m.deny == "" {
		m.deny = reason
	}
}

// noteSignature attaches a signature label (e.g. "guard:<name>",
// "crowdsec:<scenario>") to the request for the analytics collector. The
// first label wins; the built-in classifier only runs when none was noted.
func noteSignature(r *http.Request, label string) {
	if label == "" {
		return
	}
	if m := reqMetaOf(r); m != nil && m.sig == "" {
		m.sig = label
	}
}

// AnalyticsSink receives one Event per observed request. Record must never
// block (the collector hands it to a queue); Active reports whether events
// are wanted at all, so the plain chain can serve while analytics is off.
type AnalyticsSink interface {
	Record(analytics.Event)
	Active() bool
}

var analyticsSinkPtr atomic.Pointer[AnalyticsSink]

// SetAnalyticsSink installs the collector. nil removes it.
func SetAnalyticsSink(s AnalyticsSink) {
	if s == nil {
		analyticsSinkPtr.Store(nil)
		return
	}
	analyticsSinkPtr.Store(&s)
}

func analyticsSink() AnalyticsSink {
	if p := analyticsSinkPtr.Load(); p != nil {
		return *p
	}
	return nil
}

// analyticsActive reports whether a sink is installed and switched on.
func analyticsActive() bool {
	s := analyticsSink()
	return s != nil && s.Active()
}

// countingBody totals the bytes actually read from a request body, so a chunked
// upload (which carries no Content-Length) is measured too. It is installed only
// while metrics are enabled, and forwards Close so the stdlib's own body
// lifecycle is unchanged.
type countingBody struct {
	io.ReadCloser
	n int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n += int64(n)
	return n, err
}
