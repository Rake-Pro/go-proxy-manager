//lint:file-ignore SA1019 test file exercises deprecated AccessList.BasicAuth/SatisfyAny compat evaluation in accesslist.go
package dataplane

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	h := accessListHandler(compileAccessList(al), nil, nil, nil, "", nil, okHandler())

	cases := []struct {
		ip   string
		want int
	}{
		{"10.1.2.3", http.StatusOK},
		{"192.168.1.1", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), nil, nil, "", nil, okHandler())
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
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), nil, nil, "", nil, okHandler())

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
	h := accessListHandler(compileAccessList(al), ipFrom("10.9.9.9"), nil, nil, "", nil, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("satisfyAny trusted IP should pass without creds, got %d", rec.Code)
	}
}

func TestAccessListEmptyIsOpen(t *testing.T) {
	al := model.AccessList{ObjectMeta: model.ObjectMeta{Name: "empty"}}
	h := accessListHandler(compileAccessList(al), ipFrom("8.8.8.8"), nil, nil, "", nil, okHandler())
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
	h := accessListHandler(compileAccessList(al), ipFrom("8.8.8.8"), nil, nil, "", nil, okHandler())
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
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), geo, geoLoadedTrue, "", nil, okHandler())
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
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), geo, geoLoadedTrue, "", nil, okHandler())
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
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), geo, geoLoadedTrue, "", nil, okHandler())
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
	h := accessListHandler(compileAccessList(al), ipFrom("9.9.9.9"), geo, geoLoadedTrue, "", nil, okHandler())
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
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), geo, geoLoadedTrue, "", nil, okHandler())
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
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), geo, geoLoadedTrue, "", nil, okHandler())
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
	h := accessListHandler(compileAccessList(al), ipFrom("9.9.9.9"), geo, geoLoadedTrue, "", nil, okHandler())

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
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), nil, geoLoadedTrue, "", nil, okHandler())
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
			h := accessListHandler(compileAccessList(al), ipFrom("1.1.1.1"), geo, tc.geoLoaded, "", nil, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("geoLoaded=%s: DB unavailable must deny even for a known allow-listed country, got %d", tc.name, rec.Code)
			}
		})
	}
}

// Data-plane basic auth must throttle per client IP: bcrypt is expensive by
// design, so an unthrottled gate is both an online password-guessing oracle and
// a CPU-exhaustion primitive. The lockout answers 401 with the usual challenge,
// never 429, so the response never reveals that a lockout is in force.
func TestAccessListBasicAuthLocksOutAfterRepeatedFailures(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	al := model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: "auth"},
		BasicAuth:  []model.BasicAuthUser{{Username: "admin", PasswordHash: string(hash)}},
	}
	h := accessListHandler(compileAccessList(al), ipFrom("203.0.113.9"), nil, nil, "", nil, okHandler())

	try := func(user, pass string, creds bool) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		if creds {
			r.SetBasicAuth(user, pass)
		}
		h.ServeHTTP(rec, r)
		return rec
	}

	// A credential-less request is not an attempt: browsers send one per fresh
	// page load and must never be able to lock a user out.
	for range maxBasicAuthFails + 3 {
		if rec := try("", "", false); rec.Code != http.StatusUnauthorized {
			t.Fatalf("no-credential request: got %d, want 401", rec.Code)
		}
	}
	if rec := try("admin", "hunter2", true); rec.Code != http.StatusOK {
		t.Fatalf("good credentials after credential-less requests: got %d, want 200", rec.Code)
	}

	// Real failures do count, and the gate closes at the limit.
	for i := range maxBasicAuthFails {
		if rec := try("admin", "wrong", true); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: got %d, want 401", i, rec.Code)
		}
	}
	// Locked out: even the CORRECT password is refused, and with the same 401 +
	// challenge a wrong password gets (no 429, no distinguishable body).
	locked := try("admin", "hunter2", true)
	if locked.Code != http.StatusUnauthorized {
		t.Fatalf("locked-out client: got %d, want 401", locked.Code)
	}
	if locked.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("locked-out response must carry the same challenge as a normal failure")
	}
	wrong := try("admin", "also-wrong", true)
	if wrong.Code != locked.Code || wrong.Body.String() != locked.Body.String() {
		t.Fatalf("lockout is distinguishable from a wrong password: %d %q vs %d %q",
			locked.Code, locked.Body.String(), wrong.Code, wrong.Body.String())
	}

	// A different client IP is unaffected: the throttle is per key, not global.
	other := accessListHandler(compileAccessList(al), ipFrom("198.51.100.7"), nil, nil, "", nil, okHandler())
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("admin", "hunter2")
	other.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("unrelated client IP: got %d, want 200", rec.Code)
	}
}

// A successful authentication clears the client's failure count, so an operator
// who mistypes a few times is not locked out for the rest of the window.
func TestAccessListBasicAuthSuccessClearsFailures(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	al := model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: "auth"},
		BasicAuth:  []model.BasicAuthUser{{Username: "admin", PasswordHash: string(hash)}},
	}
	h := accessListHandler(compileAccessList(al), ipFrom("203.0.113.10"), nil, nil, "", nil, okHandler())
	try := func(pass string) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.SetBasicAuth("admin", pass)
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	for range maxBasicAuthFails - 1 {
		if got := try("wrong"); got != http.StatusUnauthorized {
			t.Fatalf("failure: got %d, want 401", got)
		}
	}
	if got := try("hunter2"); got != http.StatusOK {
		t.Fatalf("good credentials below the limit: got %d, want 200", got)
	}
	// The counter was reset, so a full fresh run of failures is needed again.
	for i := range maxBasicAuthFails {
		if got := try("wrong"); got != http.StatusUnauthorized {
			t.Fatalf("post-reset failure %d: got %d, want 401", i, got)
		}
	}
	if got := try("hunter2"); got != http.StatusUnauthorized {
		t.Fatalf("locked out after a fresh run of failures: got %d, want 401", got)
	}
}

// failOnce runs one attempt for key through g that ends in a wrong password,
// reporting whether the gate admitted it.
func failOnce(g *authGate, key string) bool {
	finish, admitted := g.begin(context.Background(), key)
	if admitted {
		finish(false)
	}
	return admitted
}

// The throttle map is bounded and fails CLOSED when saturated, so a flood of
// distinct client IPs cannot grow it without bound nor buy an unthrottled
// guessing path.
func TestAuthGateFailsClosedWhenSaturated(t *testing.T) {
	g := newAuthGate(time.Minute, 2, 4)
	for i := range 4 {
		if !failOnce(g, net.IPv4(10, 0, 0, byte(i)).String()) {
			t.Fatalf("key %d refused below the cap", i)
		}
	}
	if len(g.entries) != 4 {
		t.Fatalf("gate holds %d entries, want its 4-key cap", len(g.entries))
	}
	if failOnce(g, "10.0.0.99") { // one key too many: refused...
		t.Fatal("an untracked key must be refused while the map is saturated")
	}
	if len(g.entries) != 4 { // ...and not tracked
		t.Fatalf("gate grew past its cap to %d entries", len(g.entries))
	}
}

// passOnce runs one attempt for key through g that ends in a correct password,
// reporting whether the gate admitted it.
func passOnce(g *authGate, key string) bool {
	finish, admitted := g.begin(context.Background(), key)
	if admitted {
		finish(true)
	}
	return admitted
}

// A key that recently authenticated successfully is still checked while the
// map is saturated by other keys; its failures still count and lock it out,
// and the lockout drops it from the known list.
func TestAuthGateKnownKeyPassesSaturation(t *testing.T) {
	g := newAuthGate(time.Minute, 2, 4)
	if !passOnce(g, "known") {
		t.Fatal("first attempt refused")
	}
	for i := range 4 {
		failOnce(g, net.IPv4(10, 0, 0, byte(i)).String())
	}
	if failOnce(g, "stranger") {
		t.Fatal("an untracked key must still be refused while the map is saturated")
	}
	if !passOnce(g, "known") {
		t.Fatal("a recently authenticated key must be admitted while the map is saturated")
	}
	for i := range 2 {
		if !failOnce(g, "known") {
			t.Fatalf("known key failure %d refused below the limit", i)
		}
	}
	if passOnce(g, "known") {
		t.Fatal("a known key that reached the failure limit must be locked out")
	}
	if _, ok := g.known["known"]; ok {
		t.Fatal("a locked-out key must be dropped from the known list")
	}
	if len(g.entries) > g.maxKeys+g.maxKnown {
		t.Fatalf("gate holds %d entries, more than maxKeys+maxKnown", len(g.entries))
	}
}

// A saturated map is scanned only once an entry can have expired: a refused
// attempt does not rescan before the earliest expiry the last sweep saw, and a
// known key's check never scans. An expired entry planted after a sweep shows
// whether a scan ran (a scan deletes it).
func TestAuthGateSaturatedMapScansOnlyWhenEntriesCanExpire(t *testing.T) {
	g := newAuthGate(time.Minute, 2, 4)
	if !passOnce(g, "known") {
		t.Fatal("first attempt refused")
	}
	for i := range 4 {
		failOnce(g, net.IPv4(10, 0, 0, byte(i)).String())
	}
	if failOnce(g, "stranger") {
		t.Fatal("an untracked key must be refused while the map is saturated")
	}
	plant := func() {
		g.mu.Lock()
		g.entries["stale"] = &authGateEntry{resetAt: time.Now().Add(-time.Second)}
		g.mu.Unlock()
	}
	planted := func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		return g.entries["stale"] != nil
	}
	plant()
	if failOnce(g, "stranger2") {
		t.Fatal("an untracked key must be refused while the map is saturated")
	}
	if !planted() {
		t.Fatal("a refused attempt rescanned the map before any entry could expire")
	}
	if !passOnce(g, "known") {
		t.Fatal("a known key must be admitted while the map is saturated")
	}
	if !planted() {
		t.Fatal("a known key's check scanned the map")
	}

	// Once the recorded expiry is reached the next refusal sweeps again.
	g.mu.Lock()
	g.nextSweep = time.Time{}
	g.mu.Unlock()
	if failOnce(g, "stranger3") {
		t.Fatal("an untracked key must be refused while the map is saturated")
	}
	if planted() {
		t.Fatal("the sweep due after the earliest expiry did not run")
	}
}

// The known list is bounded (least recently authenticated evicted first) and
// its records expire.
func TestAuthGateKnownListBoundedAndTimeLimited(t *testing.T) {
	g := newAuthGate(time.Minute, 2, 16)
	g.maxKnown = 2
	for _, k := range []string{"a", "b", "c"} {
		passOnce(g, k)
	}
	if g.knownLRU.Len() != 2 || g.known["a"] != nil {
		t.Fatalf("known list holds %d keys (a present: %v), want 2 without the oldest", g.knownLRU.Len(), g.known["a"] != nil)
	}
	g.knownTTL = 10 * time.Millisecond
	passOnce(g, "d")
	time.Sleep(20 * time.Millisecond)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.knownLocked("d", time.Now()) {
		t.Fatal("a known record must expire after knownTTL")
	}
	if g.known["d"] != nil {
		t.Fatal("an expired known record must be dropped")
	}
}

// A map full of expired entries is not saturated: they are swept before
// saturation is reported, so a past burst of distinct keys does not keep every
// new client locked out until a reload.
func TestAuthGateSweepsExpiredEntriesBeforeSaturation(t *testing.T) {
	g := newAuthGate(20*time.Millisecond, 2, 4)
	for i := range 4 {
		failOnce(g, net.IPv4(10, 0, 0, byte(i)).String())
	}
	if failOnce(g, "10.0.0.99") {
		t.Fatal("a new key must be refused while the map is full of live entries")
	}
	time.Sleep(40 * time.Millisecond)
	if !failOnce(g, "10.0.0.99") {
		t.Fatal("a new key must be admitted once the tracked entries have expired")
	}
	if len(g.entries) != 1 {
		t.Fatalf("gate holds %d entries after the sweep, want 1", len(g.entries))
	}
}

// Finished failures lock a key out; a success clears them.
func TestAuthGateLocksOutOnFinishedFailures(t *testing.T) {
	g := newAuthGate(time.Minute, 3, 16)
	for i := range 2 {
		if !failOnce(g, "k") {
			t.Fatalf("attempt %d refused below the limit", i)
		}
	}
	finish, admitted := g.begin(context.Background(), "k")
	if !admitted {
		t.Fatal("attempt below the limit refused")
	}
	finish(true)
	for i := range 3 {
		if !failOnce(g, "k") {
			t.Fatalf("attempt %d after a success refused: the success must clear the failures", i)
		}
	}
	if failOnce(g, "k") {
		t.Fatal("attempt past the limit must be refused")
	}
}

// A parallel burst of wrong passwords from one key gets at most the failure
// limit of password checks in total: attempts beyond the budget wait for the
// in-flight checks and are then refused.
func TestAuthGateParallelWrongBurstBoundedByLimit(t *testing.T) {
	const limit = 5
	g := newAuthGate(time.Minute, limit, 16)
	var checks atomic.Int32
	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() {
			finish, admitted := g.begin(context.Background(), "k")
			if !admitted {
				return
			}
			checks.Add(1)
			time.Sleep(20 * time.Millisecond)
			finish(false)
		})
	}
	wg.Wait()
	if n := checks.Load(); n > limit {
		t.Fatalf("burst of wrong passwords got %d checks, want at most %d", n, limit)
	}
	if failOnce(g, "k") {
		t.Fatal("key must be locked out after the burst")
	}
}

// An attempt waiting behind in-flight checks gives up when its request is
// cancelled.
func TestAuthGateWaitHonorsContext(t *testing.T) {
	g := newAuthGate(time.Minute, 1, 16)
	finish, admitted := g.begin(context.Background(), "k")
	if !admitted {
		t.Fatal("first attempt refused")
	}
	defer finish(true)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, ok := g.begin(ctx, "k"); ok {
		t.Fatal("attempt over the in-flight budget must not be admitted while the check runs")
	}
}

// Parallel requests with the correct password (a browser sends the cached
// Authorization header on every sub-resource of a page) are all admitted
// through the deprecated access-list basic auth.
func TestAccessListBasicAuthParallelCorrectCredentialsAllPass(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	h := accessListHandler(compileAccessList(model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: "auth"},
		BasicAuth:  []model.BasicAuthUser{{Username: "admin", PasswordHash: string(hash)}},
	}), ipFrom("203.0.113.9"), nil, nil, "", nil, okHandler())
	if refused := parallelCorrectBurst(h, "admin", "hunter2", 20); refused != 0 {
		t.Fatalf("%d of 20 parallel correct-password requests refused, want 0", refused)
	}
}

// parallelCorrectBurst sends n parallel requests with the given credentials
// while every bcrypt slot is held, so all of them are in flight at once, and
// returns how many were not answered 200.
func parallelCorrectBurst(h http.Handler, user, pass string, n int) int {
	for range maxBcryptConcurrent {
		bcryptSem <- struct{}{}
	}
	var refused atomic.Int32
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			r := httptest.NewRequest("GET", "/", nil)
			r.SetBasicAuth(user, pass)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != http.StatusOK {
				refused.Add(1)
			}
		})
	}
	time.Sleep(100 * time.Millisecond)
	for range maxBcryptConcurrent {
		<-bcryptSem
	}
	wg.Wait()
	return int(refused.Load())
}

// IPv6 clients share one basic-auth throttle key per /56; IPv4 keys stay the
// address.
func TestAuthGateKeyGroupsIPv6By56(t *testing.T) {
	key := func(s string) string { return authGateKey(ipFrom(s), httptest.NewRequest("GET", "/", nil)) }
	a, b, c := key("2001:db8:1:200::1"), key("2001:db8:1:2ff:ffff::1"), key("2001:db8:1:300::1")
	if a != b {
		t.Fatalf("two addresses in one /56 must share a key: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("different /56s must not share a key: %q", a)
	}
	if a != "2001:db8:1:200::/56" {
		t.Fatalf("IPv6 key = %q, want the /56 prefix", a)
	}
	if got := key("192.0.2.10"); got != "192.0.2.10" {
		t.Fatalf("IPv4 key = %q, want the address", got)
	}
}

// bcrypt verification is bounded process-wide, so a request flood against a
// basic-auth host cannot saturate every core.
func TestBcryptConcurrencyIsBounded(t *testing.T) {
	if cap(bcryptSem) != maxBcryptConcurrent {
		t.Fatalf("bcrypt semaphore capacity is %d, want %d", cap(bcryptSem), maxBcryptConcurrent)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	c := compileAccessList(model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: "auth"},
		BasicAuth:  []model.BasicAuthUser{{Username: "u", PasswordHash: string(hash)}},
	})

	// Fill the semaphore, then prove a verification cannot proceed past it and
	// that a cancelled request gives up its wait rather than queueing forever.
	for range maxBcryptConcurrent {
		bcryptSem <- struct{}{}
	}
	defer func() {
		for range maxBcryptConcurrent {
			<-bcryptSem
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	r.SetBasicAuth("u", "pw")
	done := make(chan bool, 1)
	go func() { done <- c.authOK(r) }()
	select {
	case <-done:
		t.Fatal("a bcrypt compare ran while the semaphore was full")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("a cancelled request must not authenticate")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled request kept waiting for a bcrypt slot")
	}
}
