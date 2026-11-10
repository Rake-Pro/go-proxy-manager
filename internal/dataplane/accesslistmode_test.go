package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// lanList allows the LAN everywhere; monitorList allows a prober range on the
// health path only. Both deny by default.
func lanList() model.AccessList {
	return model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "lan"},
		DefaultAction: model.ActionDeny,
		Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "192.168.0.0/16"}},
	}
}

func monitorList(paths ...string) model.AccessList {
	return model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "monitor"},
		DefaultAction: model.ActionDeny,
		Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "203.0.113.0/24", Paths: paths}},
	}
}

// serveFrom runs one request from remote through h and returns the status and
// the deny reason the access-control tier recorded.
func serveFrom(h http.Handler, remote, target string) (int, string) {
	m := &reqMeta{}
	r := withReqMeta(httptest.NewRequest(http.MethodGet, target, nil), m)
	r.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code, m.deny
}

// The issue's acceptance table: a full-access list plus a path-scoped prober
// list, under both modes. "all" is the behaviour before modes existed.
func TestAccessListsModeComposition(t *testing.T) {
	reg := buildRegistry(model.Config{AccessLists: []model.AccessList{lanList(), monitorList("/healthz")}})
	const (
		prober  = "203.0.113.7:4000"
		lan     = "192.168.1.20:4000"
		unknown = "198.51.100.9:4000"
	)
	cases := []struct {
		mode       string
		remote     string
		path       string
		want       int
		wantReason string
	}{
		{"any", prober, "/healthz", http.StatusOK, ""},
		{"any", prober, "/", http.StatusForbidden, "access-list-any"},
		{"any", lan, "/", http.StatusOK, ""},
		{"any", lan, "/healthz", http.StatusOK, ""},
		{"any", unknown, "/", http.StatusForbidden, "access-list-any"},
		{"any", unknown, "/healthz", http.StatusForbidden, "access-list-any"},

		{"all", prober, "/healthz", http.StatusForbidden, "access-list"},
		{"all", prober, "/", http.StatusForbidden, "access-list"},
		{"all", lan, "/", http.StatusForbidden, "access-list"},
		{"all", unknown, "/healthz", http.StatusForbidden, "access-list"},
		{"", lan, "/healthz", http.StatusForbidden, "access-list"},
	}
	for _, tc := range cases {
		t.Run(tc.mode+" "+tc.remote+" "+tc.path, func(t *testing.T) {
			host := model.ProxyHost{
				ObjectMeta:      model.ObjectMeta{Name: "app"},
				AccessLists:     []string{"lan", "monitor"},
				AccessListsMode: tc.mode,
			}
			code, reason := serveFrom(buildChain(okHandler(), host, reg, nil), tc.remote, "http://app.example.com"+tc.path)
			if code != tc.want || reason != tc.wantReason {
				t.Fatalf("got %d reason %q, want %d reason %q", code, reason, tc.want, tc.wantReason)
			}
		})
	}
}

// serveAny runs one GET / through an any group and returns the status and the
// recorded deny reason.
func serveAny(lists []accessList, ip string, geoLoaded func() bool) (int, string) {
	ipOf := func(*http.Request) net.IP { return net.ParseIP(ip) }
	m := &reqMeta{}
	rec := httptest.NewRecorder()
	anyAccessListHandler(lists, nil, ipOf, nil, geoLoaded, "app", nil, okHandler()).
		ServeHTTP(rec, withReqMeta(httptest.NewRequest(http.MethodGet, "/", nil), m))
	return rec.Code, m.deny
}

// A member list that fails closed, that is not an allow-list, or that has
// nothing to grant never counts as an allow under "any"; an explicit deny in
// one list does not veto another.
func TestAccessListsModeAnyFailClosedMembers(t *testing.T) {
	lan := compileAccessList(lanList())
	// An allow-list by country: only US, plus every address the database
	// cannot place. With nil geoLookup every address is unknown.
	geoUSOrUnknown := compileAccessList(model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "geo"},
		DefaultAction: model.ActionDeny,
		Geo:           &model.AccessListGeo{CountryAllow: []string{"US"}, OnUnknown: model.ActionAllow},
	})
	// Blocklist-style members: on their own verdict they admit 8.8.8.8 and an
	// unplaceable address, which "any" must not turn into a grant.
	blocknet := compileAccessList(model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "blocknet"},
		DefaultAction: model.ActionAllow,
		Rules:         []model.IPRule{{Action: model.ActionDeny, CIDR: "10.66.0.0/16"}},
	})
	noKP := compileAccessList(model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: "no-kp"},
		Geo:        &model.AccessListGeo{CountryDeny: []string{"KP"}},
	})
	geoUp := func() bool { return true }
	if !blocknet.allowed(net.ParseIP("8.8.8.8"), "/", "GET", nil, geoUp) || !noKP.allowed(net.ParseIP("8.8.8.8"), "/", "GET", nil, geoUp) {
		t.Fatal("fixture: the blocklist members must admit 8.8.8.8 on their own verdict, or the rows below prove nothing")
	}
	emptyUnset := compileAccessList(model.AccessList{ObjectMeta: model.ObjectMeta{Name: "todo"}})
	emptyAllow := compileAccessList(model.AccessList{ObjectMeta: model.ObjectMeta{Name: "open"}, DefaultAction: model.ActionAllow})
	legacyAuth := compileAccessList(model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "legacy"},
		DefaultAction: model.ActionDeny,
		//lint:ignore SA1019 the deprecated basicAuth list is the runtime backstop under test
		SatisfyAny: true,
		//lint:ignore SA1019 the deprecated basicAuth list is the runtime backstop under test
		BasicAuth: []model.BasicAuthUser{{Username: "u", PasswordHash: string(dummyBcryptHash)}},
		Rules:     []model.IPRule{{Action: model.ActionAllow, CIDR: "198.51.100.0/24"}},
	})
	denyLAN := compileAccessList(model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "deny-lan"},
		DefaultAction: model.ActionDeny,
		Rules:         []model.IPRule{{Action: model.ActionDeny, CIDR: "192.168.0.0/16"}},
	})
	geoDown := func() bool { return false }
	const anyReason = "access-list-any"

	cases := []struct {
		name       string
		lists      []accessList
		ip         string
		geoLoaded  func() bool
		want       int
		wantReason string
	}{
		// The control row: the same geo list allows once a database is loaded,
		// so the row below it is refused for the missing database alone, and
		// that refusal reports "geo" as a single list would.
		{"geo list with a database loaded", []accessList{lan, geoUSOrUnknown}, "198.51.100.9", geoUp, http.StatusOK, ""},
		{"geo list with no database loaded", []accessList{lan, geoUSOrUnknown}, "198.51.100.9", geoDown, http.StatusForbidden, "geo"},
		{"geo list with a nil loaded probe", []accessList{lan, geoUSOrUnknown}, "198.51.100.9", nil, http.StatusForbidden, "geo"},
		{"geo list does not shadow the LAN list", []accessList{lan, geoUSOrUnknown}, "192.168.1.1", geoDown, http.StatusOK, ""},
		{"no geo member, no database", []accessList{lan}, "198.51.100.9", geoDown, http.StatusForbidden, anyReason},
		{"blocklist member never grants", []accessList{lan, blocknet}, "8.8.8.8", geoUp, http.StatusForbidden, anyReason},
		{"geo deny-list member never grants", []accessList{lan, noKP}, "8.8.8.8", geoUp, http.StatusForbidden, anyReason},
		{"empty list with unset default", []accessList{lan, emptyUnset}, "198.51.100.9", geoDown, http.StatusForbidden, anyReason},
		{"empty list with explicit allow", []accessList{lan, emptyAllow}, "198.51.100.9", geoDown, http.StatusForbidden, anyReason},
		{"legacy basicAuth list never allows", []accessList{lan, legacyAuth}, "198.51.100.9", geoDown, http.StatusForbidden, anyReason},
		{"explicit deny does not veto", []accessList{denyLAN, lan}, "192.168.1.1", geoDown, http.StatusOK, ""},
		// lan allows a real LAN address (row above); no address at all is denied.
		{"no client address", []accessList{lan}, "", geoDown, http.StatusForbidden, anyReason},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, reason := serveAny(tc.lists, tc.ip, tc.geoLoaded)
			if code != tc.want || reason != tc.wantReason {
				t.Fatalf("got %d reason %q, want %d reason %q", code, reason, tc.want, tc.wantReason)
			}
		})
	}
}

// A source rule with no fetched set matches nothing, so under "any" its list
// grants nothing through it. The control half proves the same list does grant
// once the set is fetched, so the refusal is the empty set, not a dropped rule.
func TestAccessListsModeAnyUnfetchedSourceDoesNotAllow(t *testing.T) {
	feed := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "feed"},
		DefaultAction: model.ActionDeny,
		Sources:       []model.AccessListSource{{Name: "probers", URL: "https://example.com/i.txt"}},
		Rules:         []model.IPRule{{Action: model.ActionAllow, Source: "probers"}},
	}
	lan := compileAccessList(lanList())

	installSources(t, sourceLedger("feed", "probers", "198.51.100.0/24"))
	if code, _ := serveAny([]accessList{lan, compileAccessList(feed)}, "198.51.100.9", nil); code != http.StatusOK {
		t.Fatalf("fetched source: got %d, want 200", code)
	}
	installSources(t, model.AccessListSourceLedger{})
	if code, reason := serveAny([]accessList{lan, compileAccessList(feed)}, "198.51.100.9", nil); code != http.StatusForbidden || reason != "access-list-any" {
		t.Fatalf("unfetched source: got %d reason %q, want 403 access-list-any", code, reason)
	}
}

// Under "any" a disabled member is skipped, so disabling it revokes its grant;
// under "all" a disabled list is still enforced, as it always was.
func TestAccessListsModeAnySkipsDisabledMembers(t *testing.T) {
	monitor := monitorList("/healthz")
	monitor.Disabled = true
	reg := buildRegistry(model.Config{AccessLists: []model.AccessList{lanList(), monitor}})
	const (
		prober = "203.0.113.7:4000"
		lan    = "192.168.1.20:4000"
	)
	cases := []struct {
		mode   string
		lists  []string
		remote string
		path   string
		want   int
	}{
		{"any", []string{"lan", "monitor"}, prober, "/healthz", http.StatusForbidden},
		{"any", []string{"lan", "monitor"}, lan, "/", http.StatusOK},
		{"any", []string{"monitor"}, prober, "/healthz", http.StatusForbidden},
		{"any", []string{"monitor"}, lan, "/", http.StatusForbidden},
		{"all", []string{"lan", "monitor"}, lan, "/", http.StatusForbidden},
	}
	for _, tc := range cases {
		host := model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, AccessLists: tc.lists, AccessListsMode: tc.mode}
		if code, _ := serveFrom(buildChain(okHandler(), host, reg, nil), tc.remote, "http://app.example.com"+tc.path); code != tc.want {
			t.Errorf("%s %v from %s %s: got %d, want %d", tc.mode, tc.lists, tc.remote, tc.path, code, tc.want)
		}
	}
}

// A location's own lists are a second group under the location's mode, and the
// host's group still applies: a mode on one level never opens the other.
func TestAccessListsModeLocationGroups(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()
	const (
		prober = "203.0.113.7:4000"
		lan    = "192.168.1.20:4000"
	)
	cases := []struct {
		name   string
		host   model.ProxyHost
		remote string
		path   string
		want   int
	}{
		{
			name: "location any cannot widen a host all group",
			host: model.ProxyHost{AccessLists: []string{"lan"},
				Locations: []model.Location{{Path: "/status", AccessLists: []string{"lan", "monitor"}, AccessListsMode: "any"}}},
			remote: prober, path: "/status/healthz", want: http.StatusForbidden,
		},
		{
			name: "location any group still lets the LAN in",
			host: model.ProxyHost{AccessLists: []string{"lan"},
				Locations: []model.Location{{Path: "/status", AccessLists: []string{"lan", "monitor"}, AccessListsMode: "any"}}},
			remote: lan, path: "/status/healthz", want: http.StatusOK,
		},
		{
			name: "location all group narrows a host any group",
			host: model.ProxyHost{AccessLists: []string{"lan", "monitor"}, AccessListsMode: "any",
				Locations: []model.Location{{Path: "/status", AccessLists: []string{"lan"}}}},
			remote: prober, path: "/status/healthz", want: http.StatusForbidden,
		},
		{
			name: "location without lists inherits the host any group",
			host: model.ProxyHost{AccessLists: []string{"lan", "monitor"}, AccessListsMode: "any",
				Locations: []model.Location{{Path: "/status"}}},
			remote: prober, path: "/status/healthz", want: http.StatusOK,
		},
		{
			name: "host any group on the host route",
			host: model.ProxyHost{AccessLists: []string{"lan", "monitor"}, AccessListsMode: "any",
				Locations: []model.Location{{Path: "/status", AccessLists: []string{"lan"}}}},
			remote: prober, path: "/", want: http.StatusForbidden,
		},
		{
			name: "both groups all is the old fold",
			host: model.ProxyHost{AccessLists: []string{"lan"},
				Locations: []model.Location{{Path: "/status", AccessLists: []string{"monitor"}}}},
			remote: lan, path: "/status/healthz", want: http.StatusForbidden,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := tc.host
			h.Name = "app"
			h.Domains = []string{"app.example.com"}
			h.Upstream = upstreamOf(t, backend)
			cfg := model.Config{
				AccessLists: []model.AccessList{lanList(), monitorList("/status/healthz")},
				ProxyHosts:  []model.ProxyHost{h},
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("fixture config invalid: %v", err)
			}
			rt, err := buildRouter(cfg, "", nil)
			if err != nil {
				t.Fatalf("buildRouter: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "http://app.example.com"+tc.path, nil)
			req.RemoteAddr = tc.remote
			rec := httptest.NewRecorder()
			rt.serveHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
