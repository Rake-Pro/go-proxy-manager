package dataplane

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/geoip"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// withGeoDB installs res as the package-level GeoIP database for the duration
// of the test and restores whatever was configured before, so tests never leak
// state into each other via the shared package-level handle (see geo.go).
func withGeoDB(t *testing.T, res *geoip.Resolver) {
	t.Helper()
	prev := currentGeoDB()
	SetGeoDB(res)
	t.Cleanup(func() { SetGeoDB(prev) })
}

// TestBuildRouterFailsClosedWithoutGeoDB proves the invariant: a geo-configured
// AccessList with NO loaded GeoIP database evaluates to DENY (fail closed), never
// allow-all. The config is deliberately built so a naive evaluation would fail OPEN
// - countryAllow with onUnknown=allow AND defaultAction=allow means "let unknown
// IPs through" - which, with no database, treats EVERY IP as unknown and would
// serve everyone. buildRouter must still succeed (no boot-loop / log.Fatal), and
// the host must return 403.
func TestBuildRouterFailsClosedWithoutGeoDB(t *testing.T) {
	withGeoDB(t, &geoip.Resolver{}) // unloaded

	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "us-only"},
			DefaultAction: model.ActionAllow,
			Geo:           &model.AccessListGeo{CountryAllow: []string{"US"}, OnUnknown: model.ActionAllow},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"app2.example.com"},
			AccessLists: []string{"us-only"},
			Upstream:    model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 1},
		}},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatalf("buildRouter must not fail on a geo rule without a DB (fail closed at compile, not abort): %v", err)
	}

	req := httptest.NewRequest("GET", "https://app2.example.com/", nil)
	req.Host = "app2.example.com"
	req.RemoteAddr = "216.160.83.56:12345" // a real, routable IP
	rec := httptest.NewRecorder()
	rt.serveHTTPS(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("geo list with no GeoIP database must DENY (fail closed), got %d (want 403)", rec.Code)
	}
}

// TestReloadDoesNotFatalWithoutGeoDB proves startup no longer boot-loops on a
// geo host with no database: dp.Reload (which main.go turns into log.Fatal on
// error) must succeed, compiling the offending host to fail-closed while other
// hosts serve normally.
func TestReloadDoesNotFatalWithoutGeoDB(t *testing.T) {
	withGeoDB(t, &geoip.Resolver{}) // unloaded

	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta: model.ObjectMeta{Name: "geoblock"},
			Geo:        &model.AccessListGeo{CountryDeny: []string{"CN"}},
		}},
		ProxyHosts: []model.ProxyHost{
			{
				ObjectMeta:  model.ObjectMeta{Name: "geo-app"},
				Domains:     []string{"geo.example.com"},
				AccessLists: []string{"geoblock"},
				Upstream:    model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 1},
			},
			{
				ObjectMeta: model.ObjectMeta{Name: "plain-app"},
				Domains:    []string{"plain.example.com"},
				Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 2},
			},
		},
	}
	dp := New(Config{})
	if err := dp.Reload(cfg); err != nil {
		t.Fatalf("Reload must not error on a geo host without a DB (startup would log.Fatal): %v", err)
	}
	rt := dp.current()
	if _, ok := rt.lookup("plain.example.com"); !ok {
		t.Fatal("the non-geo host must still be compiled and served")
	}
}

// TestBuildRouterGeoEndToEnd exercises the whole path: a loaded database, an
// AccessList with country rules attached to a proxy host, and a live request
// through the compiled router.
func TestBuildRouterGeoEndToEnd(t *testing.T) {
	path := filepath.Join("..", "geoip", "testdata", "GeoIP2-Country-Test.mmdb")
	res, err := geoip.Open(path)
	if err != nil {
		t.Skipf("test fixture not available: %v", err)
	}
	t.Cleanup(func() { _ = res.Close() })
	withGeoDB(t, res)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(closeFn)

	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "no-gb"},
			DefaultAction: model.ActionAllow,
			Geo:           &model.AccessListGeo{CountryDeny: []string{"GB"}, OnUnknown: model.ActionAllow},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"app2.example.com"},
			AccessLists: []string{"no-gb"},
			Upstream:    up,
		}},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	// 81.2.69.142 resolves to GB in the test fixture: denied.
	req := httptest.NewRequest("GET", "https://app2.example.com/", nil)
	req.Host = "app2.example.com"
	req.RemoteAddr = "81.2.69.142:12345"
	rec := httptest.NewRecorder()
	rt.serveHTTPS(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GB client should be denied, got %d", rec.Code)
	}

	// 216.160.83.56 resolves to US: allowed through to the (real) backend.
	req = httptest.NewRequest("GET", "https://app2.example.com/", nil)
	req.Host = "app2.example.com"
	req.RemoteAddr = "216.160.83.56:12345"
	rec = httptest.NewRecorder()
	rt.serveHTTPS(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("US client should reach the backend, got %d", rec.Code)
	}
}

// TestGeoAvailabilityIsLiveNotBakedAtBuildTime is the regression test for LOW-1:
// a GeoIP database that loads AFTER the router/chain was built must be honoured
// immediately, on the SAME compiled chain, with no rebuild - exactly what
// Resolver.Watch does in production (it calls Reload, never rebuilds the
// router). Previously geoUnavailable was a snapshot taken once at buildChain
// time, so a geo host stayed 403 until an unrelated config change or restart
// even after GET /api/capabilities reported dbLoaded:true.
func TestGeoAvailabilityIsLiveNotBakedAtBuildTime(t *testing.T) {
	res := &geoip.Resolver{} // unloaded at build time
	withGeoDB(t, res)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(closeFn)

	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "no-gb"},
			DefaultAction: model.ActionAllow,
			Geo:           &model.AccessListGeo{CountryDeny: []string{"GB"}, OnUnknown: model.ActionAllow},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"app2.example.com"},
			AccessLists: []string{"no-gb"},
			Upstream:    up,
		}},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}

	req := httptest.NewRequest("GET", "https://app2.example.com/", nil)
	req.Host = "app2.example.com"
	req.RemoteAddr = "216.160.83.56:12345" // resolves to US once a DB is loaded

	rec := httptest.NewRecorder()
	rt.serveHTTPS(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no DB loaded at build time: want fail-closed 403, got %d", rec.Code)
	}

	// Simulate what Resolver.Watch does on an operator's geoipupdate refresh:
	// Reload swaps the reader on the SAME *geoip.Resolver already wired via
	// SetGeoDB. It does NOT rebuild the router or recompile the chain - rt is
	// untouched.
	path := filepath.Join("..", "geoip", "testdata", "GeoIP2-Country-Test.mmdb")
	if err := res.Reload(path); err != nil {
		t.Skipf("test fixture not available: %v", err)
	}

	rec = httptest.NewRecorder()
	rt.serveHTTPS(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("same compiled chain after a live DB load (no rebuild) must now evaluate geo normally: got %d, want 200", rec.Code)
	}
}
