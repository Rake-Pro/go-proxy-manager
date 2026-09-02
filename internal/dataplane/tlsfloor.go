package dataplane

import (
	"crypto/tls"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// globalTLSFloor holds the fleet-wide minimum TLS version (settings.tls),
// installed once per config reload via SetTLSFleetDefaults. It mirrors
// globalSecurityHeaders' package-level handle for the same reason: Settings is
// not part of model.Config, so threading it through buildRouter and every
// listener constructor would touch far more of the data plane for a value that
// only ever changes alongside a full reload.
var globalTLSFloor atomic.Pointer[model.TLSFleetSettings]

// SetTLSFleetDefaults installs the fleet-wide TLS floor. It is called before the
// data-plane Reload so the floor is in place before the first handshake: every
// per-host TLS config and every stream-terminate listener is built from it.
func SetTLSFleetDefaults(t model.TLSFleetSettings) { globalTLSFloor.Store(&t) }

// currentTLSFleetFloor returns the configured fleet floor as a crypto/tls
// version, or TLS 1.2 when SetTLSFleetDefaults has never run (an embedder, or a
// test that builds a router directly).
func currentTLSFleetFloor() uint16 {
	if p := globalTLSFloor.Load(); p != nil {
		return tlsFloor(p.MinVersion)
	}
	return tls.VersionTLS12
}

// tlsFloor maps a configured version string to a crypto/tls constant. Anything
// other than "1.3" is the 1.2 floor: "" and "1.2" both mean the default, and
// validation has already rejected everything else.
func tlsFloor(v string) uint16 {
	if v == "1.3" {
		return tls.VersionTLS13
	}
	return tls.VersionTLS12
}

// effectiveTLSFloor resolves the floor for one host: its own tls.minTLSVersion
// when it pins one, else the fleet default. A host pin wins in BOTH directions -
// a legacy host can stay on 1.2 under a 1.3 fleet floor, and a single host can
// be pinned to 1.3 under the default one.
func effectiveTLSFloor(hostPin string, fleet uint16) uint16 {
	if hostPin != "" {
		return tlsFloor(hostPin)
	}
	return fleet
}
