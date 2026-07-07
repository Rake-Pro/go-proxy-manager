package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend"))
	})
}

func ipFrom(s string) func(*http.Request) net.IP {
	return func(*http.Request) net.IP { return net.ParseIP(s) }
}

func TestAccessListIP(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "lan"},
		DefaultAction: model.ActionDeny,
		Rules: []model.IPRule{
			{Action: model.ActionAllow, CIDR: "10.0.0.0/8"},
			{Action: model.ActionDeny, CIDR: "0.0.0.0/0"},
		},
	}
	h := accessListHandler(compileAccessList(al), nil, nil, nil, okHandler())

	cases := []struct {
		ip   string
		want int
	}{
		{"10.1.2.3", http.StatusOK},
		{"192.168.1.1", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), nil, nil, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != c.want {
				t.Fatalf("ip %s: got %d want %d", c.ip, rec.Code, c.want)
			}
		})
	}
	_ = h
}

func TestAccessListBasicAuth(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	al := model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: "auth"},
		BasicAuth:  []model.BasicAuthUser{{Username: "admin", PasswordHash: string(hash)}},
	}
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), nil, nil, okHandler())

	// No creds -> 401 with challenge.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("expected 401 challenge, got %d (%q)", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}

	// Good creds -> 200.
	rec = httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("admin", "hunter2")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with good creds, got %d", rec.Code)
	}

	// Wrong password -> 401.
	rec = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("admin", "wrong")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with bad creds, got %d", rec.Code)
	}
}

func TestAccessListSatisfyAny(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "any"},
		SatisfyAny:    true,
		DefaultAction: model.ActionDeny,
		Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "10.0.0.0/8"}},
		BasicAuth:     []model.BasicAuthUser{{Username: "u", PasswordHash: string(hash)}},
	}
	// Trusted IP, no creds -> allowed because satisfyAny.
	h := accessListHandler(compileAccessList(al), ipFrom("10.9.9.9"), nil, nil, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("satisfyAny trusted IP should pass without creds, got %d", rec.Code)
	}
}

func TestAccessListEmptyIsOpen(t *testing.T) {
	al := model.AccessList{ObjectMeta: model.ObjectMeta{Name: "empty"}}
	h := accessListHandler(compileAccessList(al), ipFrom("8.8.8.8"), nil, nil, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("empty access list should impose no restriction, got %d", rec.Code)
	}
}

// A list with no rules but an explicit defaultAction: deny is a deliberate
// "deny all" (lock the host down, add allow rules later); it must deny rather
// than being treated as an empty, unrestricted list.
func TestAccessListEmptyDefaultDenyIsClosed(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "locked"},
		DefaultAction: model.ActionDeny,
	}
	h := accessListHandler(compileAccessList(al), ipFrom("8.8.8.8"), nil, nil, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-rule list with defaultAction:deny must deny, got %d", rec.Code)
	}
}

// geoLookupFrom builds a fake geo resolver for tests: it maps an IP string to
// a country code; any IP not present is reported unknown (not found), like a
// private IP or one absent from a real database.
func geoLookupFrom(countries map[string]string) func(net.IP) (string, bool) {
	return func(ip net.IP) (string, bool) {
		cc, ok := countries[ip.String()]
		return cc, ok
	}
}

// geoLoadedTrue simulates a live, currently-loaded GeoIP database, for tests
// that exercise geo rule evaluation itself rather than the DB-unavailable
// fail-closed path (see TestAccessListGeoDBUnavailableFailsClosed and
// geo_test.go for that path).
func geoLoadedTrue() bool { return true }

func TestAccessListGeoCountryDeny(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "no-sanctioned"},
		DefaultAction: model.ActionAllow,
		Geo: &model.AccessListGeo{
			CountryDeny: []string{"CN", "RU", "KP"},
			OnUnknown:   model.ActionAllow,
		},
	}
	geo := geoLookupFrom(map[string]string{
		"1.2.3.4": "CN",
		"5.6.7.8": "US",
	})
	cases := []struct {
		ip   string
		want int
	}{
		{"1.2.3.4", http.StatusForbidden}, // denied country
		{"5.6.7.8", http.StatusOK},        // known, not denied: falls through to defaultAction (allow)
		{"9.9.9.9", http.StatusOK},        // unknown: onUnknown allow
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), geo, geoLoadedTrue, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != c.want {
				t.Fatalf("ip %s: got %d want %d", c.ip, rec.Code, c.want)
			}
		})
	}
}

func TestAccessListGeoCountryAllow(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "us-only"},
		DefaultAction: model.ActionDeny,
		Geo: &model.AccessListGeo{
			CountryAllow: []string{"US"},
			OnUnknown:    model.ActionDeny,
		},
	}
	geo := geoLookupFrom(map[string]string{
		"1.1.1.1": "US",
		"2.2.2.2": "CA",
	})
	cases := []struct {
		ip   string
		want int
	}{
		{"1.1.1.1", http.StatusOK},        // allow-listed country
		{"2.2.2.2", http.StatusForbidden}, // known but not allow-listed
		{"3.3.3.3", http.StatusForbidden}, // unknown: onUnknown deny
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), geo, geoLoadedTrue, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != c.want {
				t.Fatalf("ip %s: got %d want %d", c.ip, rec.Code, c.want)
			}
		})
	}
}

// TestAccessListGeoWhitelistUnknownDefaultsClosed proves the M4 fix: in
// whitelist (countryAllow) mode with onUnknown unset, an IP the database cannot
// place in a country must be DENIED - otherwise any address absent from the
// operator's GeoIP DB slips past a "these countries only" gate.
func TestAccessListGeoWhitelistUnknownDefaultsClosed(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "us-only-default"},
		DefaultAction: model.ActionDeny,
		Geo:           &model.AccessListGeo{CountryAllow: []string{"US"}}, // no onUnknown
	}
	geo := geoLookupFrom(map[string]string{"1.1.1.1": "US"})
	cases := []struct {
		ip   string
		want int
	}{
		{"1.1.1.1", http.StatusOK},        // allow-listed country
		{"9.9.9.9", http.StatusForbidden}, // absent from DB: fail closed by default
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), geo, geoLoadedTrue, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != c.want {
				t.Fatalf("ip %s: got %d want %d", c.ip, rec.Code, c.want)
			}
		})
	}
}

// Deny-list (countryDeny) mode with onUnknown unset keeps the historical
// allow-on-unknown behavior: it only ever narrows a default-allow posture, so a
// missing-from-DB IP is not the thing it is trying to block.
func TestAccessListGeoDenyListUnknownDefaultsOpen(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "not-cn-default"},
		DefaultAction: model.ActionAllow,
		Geo:           &model.AccessListGeo{CountryDeny: []string{"CN"}}, // no onUnknown
	}
	geo := geoLookupFrom(map[string]string{"1.2.3.4": "CN"})
	h := accessListHandler(compileAccessList(al), ipFrom("9.9.9.9"), geo, geoLoadedTrue, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("deny-list unknown IP with onUnknown unset should be allowed, got %d", rec.Code)
	}
}

func TestAccessListGeoIPRulesTakePriority(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "mixed"},
		DefaultAction: model.ActionAllow,
		Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "1.2.3.4/32"}},
		Geo:           &model.AccessListGeo{CountryDeny: []string{"CN"}},
	}
	// 1.2.3.4 resolves to CN in the fake geo lookup, but the explicit IP
	// allow rule matches first and wins outright.
	geo := geoLookupFrom(map[string]string{"1.2.3.4": "CN"})
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), geo, geoLoadedTrue, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("explicit IP rule should take priority over geo, got %d", rec.Code)
	}
}

func TestAccessListGeoOnlyStillGates(t *testing.T) {
	// No IP/CIDR rules configured at all (hasIP is false); hasGeo alone must
	// still gate the request rather than being treated as an open list.
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "geo-only"},
		DefaultAction: model.ActionAllow,
		Geo:           &model.AccessListGeo{CountryDeny: []string{"CN"}},
	}
	geo := geoLookupFrom(map[string]string{"1.2.3.4": "CN"})
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), geo, geoLoadedTrue, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("geo-only access list should still gate, got %d", rec.Code)
	}
}

func TestAccessListGeoSatisfyAny(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "geo-or-auth"},
		SatisfyAny:    true,
		DefaultAction: model.ActionDeny,
		Geo:           &model.AccessListGeo{CountryAllow: []string{"US"}, OnUnknown: model.ActionDeny},
		BasicAuth:     []model.BasicAuthUser{{Username: "u", PasswordHash: string(hash)}},
	}
	geo := geoLookupFrom(map[string]string{"9.9.9.9": "RU"}) // not allow-listed
	h := accessListHandler(compileAccessList(al), ipFrom("9.9.9.9"), geo, geoLoadedTrue, okHandler())

	// Wrong country, no creds -> 401 challenge: under satisfyAny, valid creds
	// could still let the request through, so the gate prompts rather than
	// flatly denying (the same behaviour a failed IP dimension gets today).
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong country + no creds should prompt for credentials, got %d", rec.Code)
	}

	// Wrong country, good creds -> passes because satisfyAny.
	rec = httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("u", "pw")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("satisfyAny should let good creds substitute for a failing geo check, got %d", rec.Code)
	}
}

func TestAccessListGeoNilLookupTreatsEveryIPAsUnknown(t *testing.T) {
	// Raw ipAllowed primitive with a database LIVE (geoLoadedTrue) but a nil
	// geoLookup: every IP is unknown, never crashes, and onUnknown governs. This
	// is NOT the production no-database path - there the DB itself is not
	// loaded, so geoLoaded() is false and the list fails closed regardless of
	// onUnknown (see TestAccessListGeoDBUnavailableFailsClosed and
	// TestBuildRouterFailsClosedWithoutGeoDB); this covers only the "DB loaded
	// but this particular lookup call found nothing" primitive.
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "no-db"},
		DefaultAction: model.ActionDeny,
		Geo:           &model.AccessListGeo{CountryDeny: []string{"CN"}, OnUnknown: model.ActionAllow},
	}
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), nil, geoLoadedTrue, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("nil geoLookup should be treated as unknown-country (onUnknown), got %d", rec.Code)
	}
}

// TestAccessListGeoDBUnavailableFailsClosed proves the LOW-1 fix at the
// accessList primitive level: a geo-configured list denies outright when
// geoLoaded reports the database is not currently loaded, even though the
// config here (defaultAction allow + onUnknown allow) would fail OPEN if the
// unavailable-DB case were mistakenly treated as "every IP unknown" instead of
// its own fail-closed branch. Both a false-returning geoLoaded and a nil one
// must deny (see accessList.ipAllowed's nil-is-not-loaded default). The
// runtime-toggle regression (same compiled chain, DB loads without a rebuild)
// is covered end-to-end in geo_test.go.
func TestAccessListGeoDBUnavailableFailsClosed(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "geo-unavailable"},
		DefaultAction: model.ActionAllow,
		Geo:           &model.AccessListGeo{CountryAllow: []string{"US"}, OnUnknown: model.ActionAllow},
	}
	geo := geoLookupFrom(map[string]string{"1.1.1.1": "US"})

	for _, tc := range []struct {
		name      string
		geoLoaded func() bool
	}{
		{"false", func() bool { return false }},
		{"nil", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := accessListHandler(compileAccessList(al), ipFrom("1.1.1.1"), geo, tc.geoLoaded, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("geoLoaded=%s: DB unavailable must deny even for a known allow-listed country, got %d", tc.name, rec.Code)
			}
		})
	}
}
