package dataplane

import (
	"context"
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
// an unknown Host header, or a redirect/dead host. Folding them all onto one
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

// countDenial records a request refused by an access-control tier. It is a
// no-op (one atomic load) when metrics are off, which is the default.
func countDenial(r *http.Request, reason string) {
	if h := metricsHook(); h != nil {
		h.Denial(hostNameOf(r), reason)
	}
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
