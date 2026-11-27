package ui

import (
	"regexp"
	"strings"
	"testing"
)

// One aggregator computes the fleet counts; the hosts list and the Overview
// read it instead of deriving their own.
func TestOverviewUsesAggregators(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"function hostStatus(h)",
		"function hostAggregate(hosts)",
		"function certAggregate(certs)",
		"const status = hostStatus(h);",
		"const hostAgg = hostsR.ok ? hostAggregate(hostsR.data) : null;",
		"const certAgg = certsR.ok ? certAggregate(certsWithStatus) : null;",
		// The attention rows take maintenance from the same rule.
		"hostsR.data.filter((h) => hostStatus(h) === 'maintenance')",
		"api('/api/runtime')",
		// What no panel shows stays in the one-line summary.
		"`${upHealthy} of ${upTotal} upstream members healthy`",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing %q", want)
		}
	}
	if n := strings.Count(js, "h.disabled ? 'disabled' : (h.maintenance"); n != 1 {
		t.Errorf("the host status rule is written %d times, want 1", n)
	}
}

func overviewPanels(t *testing.T) string {
	js := loadAppJS(t)
	a := strings.Index(js, "const OV_DASH")
	b := strings.Index(js, "async function viewOverview(c)")
	if a < 0 || b < a {
		t.Fatal("Overview panel helpers not found")
	}
	return js[a:b]
}

func TestOverviewPanelsWording(t *testing.T) {
	p := strings.ToLower(overviewPanels(t))
	for _, bad := range []string{"data plane", "data-plane", "control plane", "control-plane"} {
		if strings.Contains(p, bad) {
			t.Errorf("Overview panels contain %q", bad)
		}
	}
	for _, want := range []string{`data-wizard="hosts"`, `data-wizard="certs"`, `href="#/certs/_new"`, `aria-hidden="true"`} {
		if !strings.Contains(p, want) {
			t.Errorf("Overview panels missing %q", want)
		}
	}
}

// A deep link is read by route(), taken by wireFilter for its own list and
// the hash is rewritten to the bare route without a hashchange.
func TestOverviewDeepLinkFilter(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"const DEEP_FILTER_LISTS = { hosts: 'hosts', certs: 'certificates' };",
		"const raw = hashQ[0];",
		"deepFilter = null;\n  if (hashQ.length > 1 && !parts[1] && DEEP_FILTER_LISTS[section]) {",
		"new URLSearchParams(hashQ.slice(1).join('?')).get('filter')",
		"if (deepFilter && deepFilter.list === list) { setQuery(deepFilter.text); deepFilter = null; }",
		"history.replaceState(null, '', '#/' + section)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing %q", want)
		}
	}
	// The hash is rewritten only by the render that is still current, and the
	// link's filter wins over the remembered one.
	stale := strings.LastIndex(js[:strings.Index(js, "history.replaceState(null, '', '#/' + section)")], "if (seq !== routeSeq) return;")
	sw := strings.Index(js, "case 'overview': await viewOverview(c); break;")
	if stale < sw {
		t.Error("route() rewrites the hash before it checks that the render is still current")
	}
	if a, b := strings.Index(js, "if (deepFilter && deepFilter.list === list)"), strings.Index(js, "else if (stored) setQuery(stored);"); a < 0 || b < a {
		t.Error("wireFilter must apply a deep-link filter in preference to the remembered one")
	}
}

// The hosts read failed: the panel must not claim there are no hosts, offer
// to add one, or show status counts it cannot know.
func TestOverviewHostsPanelFailedRead(t *testing.T) {
	p := overviewPanels(t)
	a := strings.Index(p, "if (!agg) {")
	b := strings.Index(p, "if (!agg.total) {")
	if a < 0 || b < a {
		t.Fatal("overviewHostsPanel: failed-read branch not found ahead of the empty branch")
	}
	blk := p[a:b]
	if !strings.Contains(blk, "Could not read the proxy hosts") || !strings.Contains(blk, "return `") {
		t.Error("the failed-read branch must say so and return")
	}
	for _, bad := range []string{"data-wizard", "No proxy hosts yet", "ov-bar", "status:"} {
		if strings.Contains(blk, bad) {
			t.Errorf("the failed-read branch contains %q", bad)
		}
	}
	if strings.Contains(blk, "sm.maintenance") {
		t.Error("the summary's maintenance count includes disabled hosts and must not feed the panel")
	}
	// Total and Disabled are shown only when the summary read worked, and are
	// the only rows.
	if !strings.Contains(blk, "const known = total != null\n") {
		t.Error("the failed-read branch must show the summary counts only when the summary read worked")
	}
	if n := strings.Count(blk, "ovRow("); n != 2 {
		t.Errorf("the failed-read branch has %d rows, want 2 (Total, Disabled)", n)
	}
}

// Every link filters on a field the list really has, and the certificate
// states are the ones the API reports.
func TestOverviewLinksUseRealFilterFields(t *testing.T) {
	js := loadAppJS(t)
	p := overviewPanels(t)
	hostFields := map[string]bool{}
	certFields := map[string]bool{}
	for _, f := range []struct {
		anchor string
		set    map[string]bool
	}{{"const FILTER_FIELDS = {\n    domain: 'a domain'", hostFields}, {"const FILTER_FIELDS = {\n    name: 'object or display name', domain: 'a domain or SAN'", certFields}} {
		i := strings.Index(js, f.anchor)
		if i < 0 {
			t.Fatalf("FILTER_FIELDS block %q not found", f.anchor)
		}
		blk := js[i : i+strings.Index(js[i:], "};")]
		for _, m := range regexp.MustCompile(`(?:^|[\s{,])([a-z]+): '`).FindAllStringSubmatch(blk, -1) {
			f.set[m[1]] = true
		}
	}
	re := regexp.MustCompile(`ovCountRow\([^\n]*?'(hosts|certs)', '([a-z]+):`)
	links := re.FindAllStringSubmatch(p, -1)
	if len(links) < 7 {
		t.Fatalf("found %d count links, want at least 7", len(links))
	}
	for _, m := range links {
		list, field := m[1], m[2]
		set := certFields
		if list == "hosts" {
			set = hostFields
		}
		if !set[field] {
			t.Errorf("%s link filters on %q, which that list does not have", list, field)
		}
	}
	// A count and its link name the same state.
	for _, want := range []string{
		"ovCountRow('Expiring', agg.expiring, 'certs', 'state:expiring')",
		"ovCountRow('Failed', agg.error, 'certs', 'state:error')",
		"ovCountRow('Expired', agg.expired, 'certs', 'state:expired')",
		"ovCountRow('ACME', agg.acme, 'certs', 'type:acme')",
		"ovCountRow('Custom', agg.custom, 'certs', 'type:custom')",
		"ovCountRow(sg[1], agg[sg[0]], 'hosts', 'status:' + sg[0], sg[2])",
		"ovCountRow(MANAGED_CHIPS[k].text, agg.managed[k], 'hosts', 'source:' + k)",
		// The managed count uses the list filter's own matching.
		"const m = managedBy(h).toLowerCase();",
		"if (m.indexOf(k) !== -1) a.managed[k]++;",
		// The next expiry is a certificate in service; under a day reads as such.
		"if (!ct.disabled && (ct.state === 'valid' || ct.state === 'expiring') && ct.daysRemaining != null",
		"agg.soonest.days < 1 ? 'within a day' : 'in ' + plural(agg.soonest.days, 'day', 'days')",
		// A count link says what its number is; a zero is plain text.
		"const v = n ? `<a href=\"${ovFilterHref(section, filter)}\" aria-label=\"${esc(label)}: ${n}\">${n}</a>` : '<span class=\"muted\">0</span>';",
		// A link into the hosts list also replaces the remembered zone choice.
		"if (deepFilter && deepFilter.list === 'hosts' && zonesOff.size) { zonesOff.clear(); saveZonesOff(); }",
		"if (['valid', 'expiring', 'expired', 'error'].indexOf(ct.state) !== -1) a[ct.state]++;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing %q", want)
		}
	}
	for _, want := range []string{"'status:' + sg[0]", "'source:' + k", "'type:acme'", "'type:custom'", "'state:expiring'", "'state:error'", "'state:expired'"} {
		if !strings.Contains(p, want) {
			t.Errorf("Overview panels missing link filter %s", want)
		}
	}
}
