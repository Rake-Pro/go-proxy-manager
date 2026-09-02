package ui

import (
	"regexp"
	"strings"
	"testing"
)

// The sidebar is defined as grouped data (NAV_GROUPS) and the router is a
// switch, so nothing in JavaScript ties the two together: a nav entry whose
// route id has no case in route() renders a button that navigates to a blank
// page, and the browser's own URL bar is the only place that shows it went
// wrong. There is no JS test toolchain in this repo, so - like the other
// static-bundle tests in this package - these scan app.js as text.

var (
	navGroupsRe = regexp.MustCompile(`(?s)const NAV_GROUPS = \[(.*?)\n\];`)
	navItemRe   = regexp.MustCompile(`\{ id: '([a-z]+)', label: '([^']+)'`)
	titlesRe    = regexp.MustCompile(`(?s)const TITLES = \{(.*?)\n\};`)
	routeCaseRe = regexp.MustCompile(`case '([a-z]+)':`)
)

func navBundle(t *testing.T) string {
	t.Helper()
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	return string(b)
}

// navRoutes returns every (id, label) the sidebar renders.
func navRoutes(t *testing.T, js string) [][2]string {
	t.Helper()
	m := navGroupsRe.FindStringSubmatch(js)
	if m == nil {
		t.Fatal("app.js has no NAV_GROUPS block - the sidebar is no longer grouped data this test can read")
	}
	items := navItemRe.FindAllStringSubmatch(m[1], -1)
	if len(items) == 0 {
		t.Fatal("NAV_GROUPS contains no nav items")
	}
	out := make([][2]string, 0, len(items))
	for _, it := range items {
		out = append(out, [2]string{it[1], it[2]})
	}
	return out
}

// TestNavRouteIDsHaveAView is the invariant: every id in the sidebar is handled
// by route(), and has a page title. A section added to the nav without a route
// (or renamed on one side only) fails here rather than shipping a dead button.
func TestNavRouteIDsHaveAView(t *testing.T) {
	js := navBundle(t)

	cases := map[string]bool{}
	for _, m := range routeCaseRe.FindAllStringSubmatch(js, -1) {
		cases[m[1]] = true
	}
	titles := titlesRe.FindStringSubmatch(js)
	if titles == nil {
		t.Fatal("app.js has no TITLES block")
	}

	for _, item := range navRoutes(t, js) {
		id, label := item[0], item[1]
		if !cases[id] {
			t.Errorf("nav entry %q (%s) has no `case '%s':` in route() - the button would render an empty page", label, id, id)
		}
		if !strings.Contains(titles[1], id+":") {
			t.Errorf("nav entry %q (%s) has no TITLES entry - the topbar would fall back to the app name", label, id)
		}
	}
}

// TestNavLabelsAreTitleCase pins the naming pass: multi-word sidebar labels are
// Title Case ("Parked Hosts", not "Parked hosts"), so the sidebar does not mix
// two conventions in one list.
func TestNavLabelsAreTitleCase(t *testing.T) {
	js := navBundle(t)
	// Short joining words stay lowercase in title case; none are in use today,
	// but spelling the exception out keeps a future "Rules and Filters" honest.
	small := map[string]bool{"and": true, "or": true, "of": true, "the": true, "for": true, "to": true}

	for _, item := range navRoutes(t, js) {
		for i, word := range strings.Fields(item[1]) {
			if i > 0 && small[strings.ToLower(word)] {
				continue
			}
			first := word[:1]
			if first != strings.ToUpper(first) {
				t.Errorf("nav label %q is not Title Case (word %q)", item[1], word)
			}
		}
	}
}

// TestNavGroupsAreCollapsibleAndRemembered pins the two behaviours the grouped
// sidebar depends on: the ADVANCED group is the collapsible one, and its state
// is stored per browser instead of resetting on every page load.
func TestNavGroupsAreCollapsibleAndRemembered(t *testing.T) {
	js := navBundle(t)
	for _, want := range []string{
		"label: 'Advanced', collapsible: true",
		"const NAV_GROUP_KEY = 'gpm.nav.'",
		"function navGroupDefaultOpen(group)",
		"function applyNavGroupDefaults()",
		// a deep link into a collapsed group opens it
		"setNavGroupOpen(group.label, true, false)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("grouped sidebar is not wired: missing %q", want)
		}
	}
}
