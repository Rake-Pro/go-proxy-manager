package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// installSources sets the process-wide fetched-set handle for one test and
// restores the previous value afterwards, so tests stay order-independent.
func installSources(t *testing.T, l model.AccessListSourceLedger) {
	t.Helper()
	prev := globalAccessListSources.Load()
	t.Cleanup(func() { globalAccessListSources.Store(prev) })
	SetAccessListSources(l)
}

func sourceLedger(list, source string, entries ...string) model.AccessListSourceLedger {
	return model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{{
		List:    list,
		Source:  source,
		URL:     "https://example.com/i.txt",
		SHA256:  model.AccessListSourceHash(model.AccessListSourceKey(list, source), "https://example.com/i.txt", entries),
		Entries: entries,
	}}}
}

// The headline case: a host that is otherwise LAN-only lets a monitoring feed
// reach its health endpoints and nothing else.
func TestAccessListPathScopedSourceRule(t *testing.T) {
	installSources(t, sourceLedger("home-vpn", "uptimerobot", "203.0.113.0/24", "2001:db8::/32"))

	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "home-vpn"},
		DefaultAction: model.ActionDeny,
		Sources:       []model.AccessListSource{{Name: "uptimerobot", URL: "https://example.com/i.txt"}},
		Rules: []model.IPRule{
			{Action: model.ActionAllow, CIDR: "10.0.0.0/8"},
			{Action: model.ActionAllow, Source: "uptimerobot",
				Paths: []string{"/api/health", "/-/healthy"}, Methods: []string{"GET", "HEAD"}},
		},
	}
	c := compileAccessList(al)

	cases := []struct {
		name   string
		ip     string
		method string
		path   string
		want   int
	}{
		{"prober on a listed path", "203.0.113.7", "GET", "/api/health", http.StatusOK},
		{"prober on the other listed path", "203.0.113.7", "HEAD", "/-/healthy", http.StatusOK},
		{"prober over IPv6", "2001:db8::1", "GET", "/api/health", http.StatusOK},
		{"prober on any other path", "203.0.113.7", "GET", "/admin", http.StatusForbidden},
		{"prober on the site root", "203.0.113.7", "GET", "/", http.StatusForbidden},
		{"prober with a write method", "203.0.113.7", "POST", "/api/health", http.StatusForbidden},
		{"unlisted IP on a listed path", "198.51.100.9", "GET", "/api/health", http.StatusForbidden},
		{"LAN rule is unaffected by path", "10.1.2.3", "POST", "/admin", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := accessListHandler(c, ipFrom(tc.ip), nil, nil, "", nil, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.want {
				t.Fatalf("%s %s from %s: got %d want %d", tc.method, tc.path, tc.ip, rec.Code, tc.want)
			}
		})
	}
}

// An unset methods list on a path rule covers exactly GET and HEAD.
func TestAccessListPathRuleDefaultMethods(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "l"},
		DefaultAction: model.ActionDeny,
		Rules: []model.IPRule{
			{Action: model.ActionAllow, CIDR: "203.0.113.0/24", Paths: []string{"/status.php"}},
		},
	}
	c := compileAccessList(al)
	for _, tc := range []struct {
		method string
		want   int
	}{
		{"GET", http.StatusOK},
		{"HEAD", http.StatusOK},
		{"POST", http.StatusForbidden},
		{"DELETE", http.StatusForbidden},
	} {
		t.Run(tc.method, func(t *testing.T) {
			h := accessListHandler(c, ipFrom("203.0.113.7"), nil, nil, "", nil, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tc.method, "/status.php", nil))
			if rec.Code != tc.want {
				t.Fatalf("%s: got %d want %d", tc.method, rec.Code, tc.want)
			}
		})
	}
}

// Ordering is unchanged: top-down, first match wins - and a path-scoped rule that
// does not apply to this request is simply skipped, never treated as a match.
// Path rules are allow-only (validation refuses deny+paths, because an exact
// match cannot be relied on to deny), so ordering is exercised with a narrow
// allow ahead of a broad deny.
func TestAccessListPathRuleOrdering(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "l"},
		DefaultAction: model.ActionDeny,
		Rules: []model.IPRule{
			{Action: model.ActionAllow, CIDR: "203.0.113.0/24", Paths: []string{"/api/health"}},
			{Action: model.ActionDeny, CIDR: "203.0.113.0/24"},
		},
	}
	if err := al.Validate(); err != nil {
		t.Fatalf("fixture must be a valid config: %v", err)
	}
	c := compileAccessList(al)
	for _, tc := range []struct {
		name string
		path string
		want int
	}{
		{"first (allow) rule wins on its path", "/api/health", http.StatusOK},
		{"later deny rule governs every other path", "/", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := accessListHandler(c, ipFrom("203.0.113.7"), nil, nil, "", nil, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))
			if rec.Code != tc.want {
				t.Fatalf("got %d want %d", rec.Code, tc.want)
			}
		})
	}
}

// A path-scoped DENY would fail OPEN on any spelling the exact match misses
// ("/admin/" for "/admin"), so it is refused at config-write time rather than
// silently trusted. This is the regression guard for that.
func TestAccessListPathRulesAreAllowOnly(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "l"},
		DefaultAction: model.ActionAllow,
		Rules:         []model.IPRule{{Action: model.ActionDeny, CIDR: "203.0.113.0/24", Paths: []string{"/admin"}}},
	}
	if err := al.Validate(); err == nil {
		t.Fatal("a path-scoped deny must be refused: an exact match cannot be relied on to deny")
	}
}

// The rules are matched against the path the ROUTER produces, not the raw
// request-line, so these go through normalizeRequestPath exactly as a live
// request does. The property under test is that no alternative spelling of a
// listed path can be smuggled past - and, because path rules are allow-only,
// that every near-miss falls through to defaultAction (deny), never past it.
func TestAccessListPathMatchingIsOnTheNormalizedPath(t *testing.T) {
	installSources(t, sourceLedger("home-vpn", "uptimerobot", "203.0.113.0/24"))

	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "home-vpn"},
		DefaultAction: model.ActionDeny,
		Sources:       []model.AccessListSource{{Name: "uptimerobot", URL: "https://example.com/i.txt"}},
		Rules: []model.IPRule{
			{Action: model.ActionAllow, Source: "uptimerobot", Paths: []string{"/api/health"}},
		},
	}
	if err := al.Validate(); err != nil {
		t.Fatalf("fixture must be a valid config: %v", err)
	}
	c := compileAccessList(al)

	cases := []struct {
		name   string
		target string
		want   int
	}{
		{"the exact listed path", "/api/health", http.StatusOK},
		{"a dot segment collapsing onto it still matches", "/api/x/../health", http.StatusOK},
		{"a doubled slash collapses onto it", "//api/health", http.StatusOK},
		// An encoded slash decodes into the path BEFORE matching, and RawPath is
		// cleared, so the upstream receives exactly the "/api/health" that was
		// matched - matcher and backend cannot disagree, which is why allowing it
		// is correct rather than a bypass.
		{"encoded slash decodes to the same path gpm forwards", "/api%2Fhealth", http.StatusOK},
		// Every near miss must DENY. A trailing slash is preserved by cleanPath and
		// matching is case-sensitive; neither is folded onto the listed path. Path
		// rules are allow-only precisely so that a miss denies instead of passing.
		{"trailing slash is not the listed path", "/api/health/", http.StatusForbidden},
		{"different case is not the listed path", "/API/HEALTH", http.StatusForbidden},
		{"a prefix of the listed path", "/api", http.StatusForbidden},
		{"a longer path under it", "/api/health/deep", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := accessListHandler(c, ipFrom("203.0.113.7"), nil, nil, "", nil, okHandler())
			r := httptest.NewRequest("GET", tc.target, nil)
			if !normalizeRequestPath(r) {
				// The router answers 400 here; for this test that is "not allowed",
				// which is the property being asserted.
				if tc.want == http.StatusOK {
					t.Fatalf("%s: normalization rejected a path that should have matched", tc.target)
				}
				return
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != tc.want {
				t.Fatalf("%s (normalized %q): got %d want %d", tc.target, r.URL.Path, rec.Code, tc.want)
			}
		})
	}
}

// A source with no ledger entry yet resolves to the EMPTY set. The rule must
// then match nothing - and crucially must NOT disappear, which would leave a
// default-deny list with no IP dimension at all and turn it into an open gate.
func TestAccessListUnresolvedSourceMatchesNothing(t *testing.T) {
	installSources(t, model.AccessListSourceLedger{})

	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "home-vpn"},
		DefaultAction: model.ActionDeny,
		Sources:       []model.AccessListSource{{Name: "uptimerobot", URL: "https://example.com/i.txt"}},
		Rules: []model.IPRule{
			{Action: model.ActionAllow, Source: "uptimerobot", Paths: []string{"/api/health"}},
		},
	}
	c := compileAccessList(al)
	if !c.hasIP {
		t.Fatal("an unresolved source rule must keep the list's IP dimension, or a default-deny list becomes open")
	}
	h := accessListHandler(c, ipFrom("203.0.113.7"), nil, nil, "", nil, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403 while the source has never been fetched", rec.Code)
	}
}

// A source set is keyed by "<list>/<source>", so one list's feed can never be
// picked up by another list that happens to name the same source.
func TestAccessListSourceIsScopedToItsList(t *testing.T) {
	installSources(t, sourceLedger("other-list", "uptimerobot", "203.0.113.0/24"))

	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "home-vpn"},
		DefaultAction: model.ActionDeny,
		Sources:       []model.AccessListSource{{Name: "uptimerobot", URL: "https://example.com/i.txt"}},
		Rules:         []model.IPRule{{Action: model.ActionAllow, Source: "uptimerobot"}},
	}
	h := accessListHandler(compileAccessList(al), ipFrom("203.0.113.7"), nil, nil, "", nil, okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403: a set fetched for another list must not resolve here", rec.Code)
	}
}

// A source rule without paths behaves like any other IP rule, on every path.
func TestAccessListSourceRuleWithoutPaths(t *testing.T) {
	installSources(t, sourceLedger("feeds", "cdn", "203.0.113.0/24"))

	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "feeds"},
		DefaultAction: model.ActionDeny,
		Sources:       []model.AccessListSource{{Name: "cdn", URL: "https://example.com/i.txt"}},
		Rules:         []model.IPRule{{Action: model.ActionAllow, Source: "cdn"}},
	}
	c := compileAccessList(al)
	for _, path := range []string{"/", "/admin", "/api/health"} {
		h := accessListHandler(c, ipFrom("203.0.113.7"), nil, nil, "", nil, okHandler())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("path %s: got %d want 200", path, rec.Code)
		}
	}
}

// At L4 there is no request path, so a path-scoped rule can never match. The
// stream compile keeps the rest of the list working rather than opening the port.
func TestAccessListPathRulesNeverMatchOnAStream(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "l"},
		DefaultAction: model.ActionDeny,
		Rules: []model.IPRule{
			{Action: model.ActionAllow, CIDR: "203.0.113.0/24", Paths: []string{"/health"}},
			{Action: model.ActionAllow, CIDR: "10.0.0.0/8"},
		},
	}
	c := compileAccessList(al)
	if c.allowIP(mustIP("203.0.113.7"), nil, nil) {
		t.Fatal("a path-scoped rule must never match on a raw stream")
	}
	if !c.allowIP(mustIP("10.1.2.3"), nil, nil) {
		t.Fatal("the list's literal rules must still apply on a raw stream")
	}
}
