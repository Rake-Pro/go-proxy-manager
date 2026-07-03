package dataplane

import (
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/geoip"
)

// geoDB, set once at startup via SetGeoDB, is the GeoIP database consulted by
// AccessList geo rules (see internal/geoip and docs/design/http3-geoip-mtls.md).
// It mirrors the ssoKeyDir pattern in oidcgate.go: a package-level handle set
// once from main, rather than threaded through every constructor. An unset (or
// never-loaded) database means the feature is unavailable: any AccessList that
// configures geo rules then evaluates fail CLOSED (deny) on the hosts using it
// (see registry.geoLoaded/accessList.ipAllowed), while every other host builds
// and serves normally. This availability check is live, at request-evaluation
// time, not baked into the compiled accessList - so a database that loads (or
// is swapped) after the router was built takes effect on the next request,
// with no config change or restart (see Resolver.Watch). Reject-at-write (see
// internal/store) additionally refuses to commit a new geo rule while no
// database is loaded, so this fail-closed evaluation is the safety net for a
// config that predates the missing DB (e.g. a restart with GPM_GEOIP_DB still
// absent), never the primary path.
var geoDB atomic.Pointer[geoip.Resolver]

// SetGeoDB configures the GeoIP database consulted by AccessList geo rules.
// res may itself have no database loaded (Resolver.Loaded() == false) if
// GPM_GEOIP_DB is unset or unreadable; access-list geo rules then compile to
// deny-all (fail closed) on the hosts using them until it is fixed.
func SetGeoDB(res *geoip.Resolver) { geoDB.Store(res) }

// currentGeoDB returns the configured resolver, or nil if SetGeoDB was never
// called (e.g. in tests). geoip.Resolver's methods are nil-receiver safe.
func currentGeoDB() *geoip.Resolver { return geoDB.Load() }
