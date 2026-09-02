package dataplane

import "sync/atomic"

// UpstreamHealthEvent is one upstream's health-state transition: fired once
// per flip (never per probe), so the installed hook sees "10.0.0.5:80 in
// group app went unhealthy" exactly once, not on every failed health check
// while it stays down.
type UpstreamHealthEvent struct {
	// Group is the UpstreamGroup name.
	Group string
	// Upstream is the upstream's label (scheme://host:port), matching the
	// GET /api/upstream-health payload.
	Upstream string
	Healthy  bool
}

// UpstreamHealthHook receives upstream health-state transitions. Declared
// here, and satisfied structurally by internal/notify's wiring in
// cmd/gpm/main.go, so this package never imports notify - the same
// decoupling MetricsHook uses for internal/metrics.
type UpstreamHealthHook func(UpstreamHealthEvent)

// upstreamHealthHookPtr holds the installed hook. Atomic so wiring it at
// startup races nothing, and the common (unwired) case costs one atomic load.
var upstreamHealthHookPtr atomic.Pointer[UpstreamHealthHook]

// SetUpstreamHealthHook installs the sink for upstream health-state
// transitions. Passing nil detaches it. Mirrors SetMetricsHook.
func SetUpstreamHealthHook(h UpstreamHealthHook) {
	if h == nil {
		upstreamHealthHookPtr.Store(nil)
		return
	}
	upstreamHealthHookPtr.Store(&h)
}

// upstreamHealthHook returns the installed hook, or nil.
func upstreamHealthHook() UpstreamHealthHook {
	if p := upstreamHealthHookPtr.Load(); p != nil {
		return *p
	}
	return nil
}
