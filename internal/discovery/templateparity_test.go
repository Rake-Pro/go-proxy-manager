package discovery

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

func tpList(v ...string) *[]string { s := append([]string{}, v...); return &s }

func tpString(p *[]string) string {
	if p == nil {
		return "<unset>"
	}
	return "[" + strings.Join(*p, ",") + "]"
}

// derivedHost is what a derive produces for "app": the template's tags and,
// when the template sets it, its trustedProxies.
func derivedHost(name string, tags []string, tp *[]string, domains ...string) model.ProxyHost {
	h := host(name, model.ManagedByIngressDiscovery, domains...)
	h.Tags = tags
	h.TrustedProxies = tp
	return h
}

const tpKey = "gpm.rake.pro/trusted-proxies-by"

func ingressOwn() Ownership {
	o := own("a discovery", model.ManagedByIngressDiscovery)
	o.TrustedProxiesByKey = tpKey
	return o
}

// marked returns h carrying the trusted-proxies-by marker, as a host whose
// trustedProxies discovery stamped is stored.
func marked(h model.ProxyHost) model.ProxyHost {
	h.Labels = cloneLabels(h.Labels)
	h.Labels[tpKey] = model.ManagedByIngressDiscovery
	return h
}

func isMarked(h model.ProxyHost) bool { return h.Labels[tpKey] == model.ManagedByIngressDiscovery }

// settle applies one plan of items over cfg, then asserts that three more
// passes over the previous output write nothing, and returns the settled
// config and memory.
func settle(t *testing.T, cfg model.Config, o Ownership, items []Item, mem StampedTags) (model.Config, StampedTags, Result) {
	t.Helper()
	first := planWith(cfg, o, items, mem)
	cfg, mem = applyPlan(cfg, first, nil), first.Stamped
	for pass := range 3 {
		p := planWith(cfg, o, items, mem)
		if len(p.Upserts) != 0 || len(p.Deletes) != 0 {
			t.Fatalf("pass %d after the write: upserts %+v deletes %v, want none", pass+1, p.Upserts, p.Deletes)
		}
		cfg, mem = applyPlan(cfg, p, nil), p.Stamped
	}
	return cfg, mem, first
}

// A template that sets trustedProxies stamps it onto a new host and onto an
// existing one, marked as stamped, and an empty list is stamped as an explicit
// empty list.
func TestPlanTrustedProxiesSetOnTheTemplateIsStamped(t *testing.T) {
	tests := []struct {
		name   string
		stored *[]string
		tmpl   *[]string
	}{
		{"list over inherit", nil, tpList("192.0.2.10")},
		{"list over another list", tpList("198.51.100.0/24"), tpList("192.0.2.10")},
		{"empty over inherit", nil, tpList()},
		{"empty over a list", tpList("198.51.100.0/24"), tpList()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cur := derivedHost("old", nil, tc.stored, "old.example.com")
			cfg := model.Config{ProxyHosts: []model.ProxyHost{cur}}
			items := []Item{
				{Ref: "src/old", Name: "old", Host: derivedHost("old", nil, tc.tmpl, "old.example.com")},
				{Ref: "src/new", Name: "new", Host: derivedHost("new", nil, tc.tmpl, "new.example.com")},
			}
			_, _, p := settle(t, cfg, ingressOwn(), items, nil)
			if p.Created != 1 || p.Updated != 1 || len(p.Upserts) != 2 {
				t.Fatalf("plan %+v, want one create and one update", p)
			}
			for _, u := range p.Upserts {
				if u.TrustedProxies == nil || !slices.Equal(*u.TrustedProxies, *tc.tmpl) {
					t.Fatalf("%s: trustedProxies %s, want the template's %s", u.Name, tpString(u.TrustedProxies), tpString(tc.tmpl))
				}
				if !isMarked(u) {
					t.Fatalf("%s: labels %v, want the trusted-proxies-by marker", u.Name, u.Labels)
				}
				if len(*tc.tmpl) == 0 {
					b, _ := yaml.Marshal(u)
					if !strings.Contains(string(b), "trustedProxies: []") {
						t.Fatalf("%s: an empty template list must be written as an explicit empty list:\n%s", u.Name, b)
					}
				}
			}
			if cur.Labels[tpKey] != "" {
				t.Fatal("the stored host's labels were modified in place")
			}
		})
	}
}

// A template that leaves trustedProxies unset does not own an UNMARKED host's
// value: it is the operator's, kept across a reconcile, also one that rewrites
// the host for another reason. A new host is created without one, and no
// host gets the marker.
func TestPlanTrustedProxiesUnsetKeepsAnOperatorValue(t *testing.T) {
	for _, stored := range []*[]string{tpList("192.0.2.10"), tpList()} {
		t.Run(tpString(stored), func(t *testing.T) {
			cur := derivedHost("app", nil, stored, "app.example.com")
			cfg := model.Config{ProxyHosts: []model.ProxyHost{cur}}

			p := Plan(cfg, ingressOwn(), []Item{{Ref: "src", Name: "app", Host: derivedHost("app", nil, nil, "app.example.com")}})
			if len(p.Upserts) != 0 || p.Hosts[0].Action != ActionUnchanged {
				t.Fatalf("plan %+v, want the host unchanged", p)
			}

			// A domain change rewrites the host; its trustedProxies rides along.
			_, _, p = settle(t, cfg, ingressOwn(), []Item{
				{Ref: "src", Name: "app", Host: derivedHost("app", nil, nil, "app.example.com", "www.example.com")},
				{Ref: "src/new", Name: "new", Host: derivedHost("new", nil, nil, "new.example.com")},
			}, nil)
			if len(p.Upserts) != 2 {
				t.Fatalf("plan %+v, want an update and a create", p)
			}
			for _, u := range p.Upserts {
				if isMarked(u) {
					t.Fatalf("%s: labels %v, want no marker under an unset template", u.Name, u.Labels)
				}
				switch u.Name {
				case "app":
					if u.TrustedProxies != stored {
						t.Fatalf("updated host trustedProxies %s, want the stored %s", tpString(u.TrustedProxies), tpString(stored))
					}
				case "new":
					if u.TrustedProxies != nil {
						t.Fatalf("new host trustedProxies %s, want none", tpString(u.TrustedProxies))
					}
				}
			}
		})
	}
}

// A value discovery stamped goes when the resolved template stops setting the
// key (the key was removed, or the host now resolves to a profile without it):
// the field is cleared and the marker dropped. Setting it again stamps and
// marks it again. Each step settles in one write.
func TestPlanTrustedProxiesSetUnsetSet(t *testing.T) {
	o := ingressOwn()
	cfg := model.Config{ProxyHosts: []model.ProxyHost{derivedHost("app", nil, nil, "app.example.com")}}
	set := []Item{{Ref: "src", Name: "app", Host: derivedHost("app", nil, tpList("0.0.0.0/0"), "app.example.com")}}
	unset := []Item{{Ref: "src", Name: "app", Host: derivedHost("app", nil, nil, "app.example.com")}}

	cfg, mem, p := settle(t, cfg, o, set, nil)
	if len(p.Upserts) != 1 || tpString(cfg.ProxyHosts[0].TrustedProxies) != "[0.0.0.0/0]" || !isMarked(cfg.ProxyHosts[0]) {
		t.Fatalf("set: host %+v, want the value stamped and marked", cfg.ProxyHosts[0])
	}
	cfg, mem, p = settle(t, cfg, o, unset, mem)
	if len(p.Upserts) != 1 || cfg.ProxyHosts[0].TrustedProxies != nil || isMarked(cfg.ProxyHosts[0]) {
		t.Fatalf("unset: host %+v, want the stamped value and the marker gone", cfg.ProxyHosts[0])
	}
	cfg, _, p = settle(t, cfg, o, set, mem)
	if len(p.Upserts) != 1 || tpString(cfg.ProxyHosts[0].TrustedProxies) != "[0.0.0.0/0]" || !isMarked(cfg.ProxyHosts[0]) {
		t.Fatalf("set again: host %+v, want the value stamped and marked", cfg.ProxyHosts[0])
	}
}

// A marked host whose template still sets the key keeps both, and an equal
// value is no change even when one side points at a nil slice and the other
// at an empty one (the same YAML, but not reflect.DeepEqual). A host that
// carries the marker without a value loses the marker under an unset
// template.
func TestPlanTrustedProxiesMarkerStates(t *testing.T) {
	var nilSlice []string
	o := ingressOwn()
	cur := marked(derivedHost("app", nil, &nilSlice, "app.example.com"))
	if p := Plan(model.Config{ProxyHosts: []model.ProxyHost{cur}}, o,
		[]Item{{Ref: "src", Name: "app", Host: derivedHost("app", nil, tpList(), "app.example.com")}}); len(p.Upserts) != 0 {
		t.Fatalf("plan %+v, want no write", p)
	}

	bare := marked(derivedHost("app", nil, nil, "app.example.com"))
	cfg, _, p := settle(t, model.Config{ProxyHosts: []model.ProxyHost{bare}}, o,
		[]Item{{Ref: "src", Name: "app", Host: derivedHost("app", nil, nil, "app.example.com")}}, nil)
	if len(p.Upserts) != 1 || isMarked(cfg.ProxyHosts[0]) {
		t.Fatalf("plan %+v, want the leftover marker dropped", p)
	}

	// Without a marker key configured nothing is ever marked or cleared.
	o.TrustedProxiesByKey = ""
	if p := Plan(model.Config{ProxyHosts: []model.ProxyHost{marked(derivedHost("app", nil, tpList("192.0.2.10"), "app.example.com"))}}, o,
		[]Item{{Ref: "src", Name: "app", Host: marked(derivedHost("app", nil, nil, "app.example.com"))}}); len(p.Upserts) != 0 {
		t.Fatalf("plan %+v, want no write without a marker key", p)
	}
}

// The domain-conflict restamp treats trustedProxies like a plain update: set
// on the template it is stamped and marked, unset it clears a stamped value
// (which by itself makes the stored chain stale) and keeps an operator's.
func TestPlanTrustedProxiesDomainConflictRestamp(t *testing.T) {
	o := ingressOwn()
	base := model.IngressHostTemplate{
		Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.1", Port: 80},
		Timeouts: &model.HostTimeouts{ReadSeconds: 5},
	}
	stale := &model.HostTimeouts{ReadSeconds: 30}
	tests := []struct {
		name       string
		stored     *[]string
		markStored bool
		timeouts   *model.HostTimeouts // stored timeouts; differ from base to force a restamp
		tmpl       *[]string
		want       *[]string
		wantMarked bool
		restamp    bool
	}{
		{"set and different restamps", nil, false, base.Timeouts, tpList("192.0.2.10"), tpList("192.0.2.10"), true, true},
		{"set empty restamps trust nobody", tpList("192.0.2.10"), true, base.Timeouts, tpList(), tpList(), true, true},
		{"set and equal is a plain skip", tpList("192.0.2.10"), true, base.Timeouts, tpList("192.0.2.10"), nil, false, false},
		{"unset, operator value is not compared", tpList("192.0.2.10"), false, base.Timeouts, nil, nil, false, false},
		{"unset keeps an operator value on a restamp", tpList("192.0.2.10"), false, stale, nil, tpList("192.0.2.10"), false, true},
		{"unset clears a stamped value", tpList("0.0.0.0/0"), true, base.Timeouts, nil, nil, false, true},
		{"unset clears a stamped value on a restamp", tpList("0.0.0.0/0"), true, stale, nil, nil, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := base
			tmpl.TrustedProxies = tc.tmpl
			cur := derivedHost("app", nil, tc.stored, "app.example.com")
			if tc.markStored {
				cur = marked(cur)
			}
			cur.Timeouts = tc.timeouts
			want := derivedHost("app", nil, tc.tmpl, "new.example.com")
			want.Timeouts = base.Timeouts
			cfg := model.Config{ProxyHosts: []model.ProxyHost{cur, host("op", "", "new.example.com")}}
			items := []Item{{Ref: "src", Name: "app", Host: want, Resolved: &tmpl}}

			p := Plan(cfg, o, items)
			if !tc.restamp {
				if len(p.Upserts) != 0 || p.Skipped != 1 {
					t.Fatalf("plan %+v, want a plain skip", p)
				}
				return
			}
			if len(p.Upserts) != 1 || p.Upserts[0].Disabled {
				t.Fatalf("plan %+v, want one restamp", p)
			}
			upd := p.Upserts[0]
			if tpString(upd.TrustedProxies) != tpString(tc.want) || isMarked(upd) != tc.wantMarked || !slices.Equal(upd.Domains, cur.Domains) {
				t.Fatalf("restamp trustedProxies %s marked %v domains %v, want %s marked %v on the current domains",
					tpString(upd.TrustedProxies), isMarked(upd), upd.Domains, tpString(tc.want), tc.wantMarked)
			}
			if tc.markStored && !isMarked(cfg.ProxyHosts[0]) {
				t.Fatal("the stored host's labels were modified in place")
			}
			next := applyPlan(cfg, p, nil)
			for pass := range 3 {
				again := Plan(next, o, items)
				if len(again.Upserts) != 0 || again.Skipped != 1 {
					t.Fatalf("pass %d after the restamp: %+v, want a plain skip", pass+1, again)
				}
			}
		})
	}
}

// A source that cannot be derived freezes its host only while the stored
// trustedProxies is what the profile renders: the profile's value when it sets
// the key, no value when it does not and the stored one was stamped. An
// operator's value under an unset profile is not compared. The fail-closed
// write keeps the value and its marker.
func TestPlanTrustedProxiesDeriveFailure(t *testing.T) {
	tests := []struct {
		name         string
		stored, tmpl *[]string
		markStored   bool
		wantDisabled bool
	}{
		{"unset template freezes a host with an operator value", tpList("192.0.2.10"), nil, false, false},
		{"unset template fails a stamped value closed", tpList("0.0.0.0/0"), nil, true, true},
		{"set template matching freezes", tpList("192.0.2.10"), tpList("192.0.2.10"), true, false},
		{"set template differing fails closed", nil, tpList("192.0.2.10"), false, true},
		{"empty template against a list fails closed", tpList("192.0.2.10"), tpList(), true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := model.IngressHostTemplate{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.1", Port: 80}, TrustedProxies: tc.tmpl}
			cur := derivedHost("app", nil, tc.stored, "app.example.com")
			if tc.markStored {
				cur = marked(cur)
			}
			cfg := model.Config{ProxyHosts: []model.ProxyHost{cur}}
			p := Plan(cfg, ingressOwn(), []Item{{Ref: "src", Name: "app", Err: errors.New("boom"), Resolved: &tmpl}})
			disabled := len(p.Upserts) == 1 && p.Upserts[0].Disabled
			if disabled != tc.wantDisabled || (!tc.wantDisabled && len(p.Upserts) != 0) {
				t.Fatalf("upserts %+v, want disabled=%v", p.Upserts, tc.wantDisabled)
			}
			if disabled && (tpString(p.Upserts[0].TrustedProxies) != tpString(tc.stored) || isMarked(p.Upserts[0]) != tc.markStored) {
				t.Fatalf("fail-closed write %+v, want the stored value and marker kept", p.Upserts[0])
			}
		})
	}
}

// The tag rule: the template's tags first, then the stored host's other tags
// in stored order without duplicates, minus the tags stamped at the previous
// pass that the template no longer has.
func TestPlanTagsMerge(t *testing.T) {
	tests := []struct {
		name        string
		tmpl        []string
		stored      []string
		prev        []string // nil: no memory of this host
		noMemory    bool
		want        []string
		wantUpdated bool
	}{
		{name: "hand-added tag is kept", tmpl: []string{"a"}, stored: []string{"a", "x"}, prev: []string{"a"}, want: []string{"a", "x"}},
		{name: "template tag added later appears", tmpl: []string{"a", "b"}, stored: []string{"a", "x"}, prev: []string{"a"}, want: []string{"a", "b", "x"}, wantUpdated: true},
		{name: "template tag removed while running is dropped", tmpl: []string{"a"}, stored: []string{"a", "b", "x"}, prev: []string{"a", "b"}, want: []string{"a", "x"}, wantUpdated: true},
		{name: "every template tag removed while running", tmpl: nil, stored: []string{"a", "b", "x"}, prev: []string{"a", "b"}, want: []string{"x"}, wantUpdated: true},
		{name: "no memory drops nothing", tmpl: []string{"a"}, stored: []string{"a", "b", "x"}, noMemory: true, want: []string{"a", "b", "x"}},
		{name: "hand-removed template tag comes back", tmpl: []string{"a", "b"}, stored: []string{"b", "x"}, prev: []string{"a", "b"}, want: []string{"a", "b", "x"}, wantUpdated: true},
		{name: "template tags go first", tmpl: []string{"a"}, stored: []string{"x", "a"}, prev: []string{"a"}, want: []string{"a", "x"}, wantUpdated: true},
		{name: "hand-added tags keep their stored order", tmpl: []string{"a"}, stored: []string{"a", "z", "b"}, prev: []string{"a"}, want: []string{"a", "z", "b"}},
		{name: "stored duplicates collapse", tmpl: []string{"b", "a"}, stored: []string{"x", "a", "y", "x", "b"}, prev: []string{"a", "b"}, want: []string{"b", "a", "x", "y"}, wantUpdated: true},
		{name: "a stale tag also in the template is kept", tmpl: []string{"a"}, stored: []string{"a"}, prev: []string{"a", "b"}, want: []string{"a"}},
		{name: "equal to the template", tmpl: []string{"a", "b"}, stored: []string{"a", "b"}, prev: []string{"a", "b"}, want: []string{"a", "b"}},
		{name: "duplicate template tags stay as stamped", tmpl: []string{"a", "a"}, stored: []string{"a", "a"}, noMemory: true, want: []string{"a", "a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := model.Config{ProxyHosts: []model.ProxyHost{derivedHost("app", tc.stored, nil, "app.example.com")}}
			items := []Item{{Ref: "src", Name: "app", Host: derivedHost("app", tc.tmpl, nil, "app.example.com")}}
			var mem StampedTags
			if !tc.noMemory {
				mem = StampedTags{"app": tc.prev}
			}
			p := planWith(cfg, ingressOwn(), items, mem)
			if got := len(p.Upserts) == 1; got != tc.wantUpdated {
				t.Fatalf("updated=%v, want %v (plan %+v)", got, tc.wantUpdated, p.Hosts)
			}
			after := applyPlan(cfg, p, nil)
			if got := after.ProxyHosts[0].Tags; !slices.Equal(got, tc.want) {
				t.Fatalf("tags %v, want %v", got, tc.want)
			}
			if got := p.Stamped["app"]; !slices.Equal(got, tc.tmpl) {
				t.Fatalf("memory %v, want the template's %v", got, tc.tmpl)
			}
			// Stability: the next pass with the returned memory writes nothing.
			if again := planWith(after, ingressOwn(), items, p.Stamped); len(again.Upserts) != 0 {
				t.Fatalf("second pass %+v, want no writes", again.Upserts)
			}
			if mem != nil && !slices.Equal(mem["app"], tc.prev) {
				t.Fatalf("Plan modified its input memory: %v", mem)
			}
		})
	}
}

// The tag rule applies to the domain-conflict restamp too: the restamped host
// keeps a hand-added tag and loses a stale one, and the memory follows.
func TestPlanTagsOnDomainConflictRestamp(t *testing.T) {
	o := ingressOwn()
	tmpl := model.IngressHostTemplate{
		Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.1", Port: 80},
		Timeouts: &model.HostTimeouts{ReadSeconds: 5},
		Tags:     []string{"a"},
	}
	cur := derivedHost("app", []string{"a", "b", "x"}, nil, "app.example.com")
	want := derivedHost("app", []string{"a"}, nil, "new.example.com")
	want.Timeouts = tmpl.Timeouts
	cfg := model.Config{ProxyHosts: []model.ProxyHost{cur, host("op", "", "new.example.com")}}
	items := []Item{{Ref: "src", Name: "app", Host: want, Resolved: &tmpl}}

	p := planWith(cfg, o, items, StampedTags{"app": {"a", "b"}})
	if len(p.Upserts) != 1 || !slices.Equal(p.Upserts[0].Tags, []string{"a", "x"}) {
		t.Fatalf("plan %+v, want one restamp with tags [a x]", p.Upserts)
	}
	if !slices.Equal(p.Stamped["app"], []string{"a"}) {
		t.Fatalf("memory %v, want [a]", p.Stamped)
	}
	// A conflict that needs no restamp writes nothing, tags included, and the
	// memory of the host is carried over.
	p = planWith(applyPlan(cfg, p, nil), o, items, p.Stamped)
	if len(p.Upserts) != 0 || !slices.Equal(p.Stamped["app"], []string{"a"}) {
		t.Fatalf("second pass upserts %+v memory %v, want none and [a]", p.Upserts, p.Stamped)
	}
}

// The memory a plan returns: the template tags of every host created, updated
// or unchanged; the previous entry of a managed host kept as stored (frozen,
// disabled, held); nothing for a deleted host or one this subsystem does not
// own.
func TestPlanTagMemory(t *testing.T) {
	stored := func(name string, tags ...string) model.ProxyHost {
		return derivedHost(name, tags, nil, name+".example.com")
	}
	foreign := host("foreign", "", "foreign.example.com")
	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		stored("keep", "t"),
		stored("upd", "old"),
		stored("gone", "t"),
		stored("frozen", "old"),
		stored("revoked", "old"),
		stored("unremembered", "t"),
		foreign,
	}}
	item := func(name string) Item {
		return Item{Ref: "src/" + name, Name: name, Host: derivedHost(name, []string{"t"}, nil, name+".example.com")}
	}
	items := []Item{
		item("new"), item("keep"), item("upd"), item("foreign"), item("unremembered"),
		{Ref: "src/frozen", Name: "frozen", Err: errors.New("malformed")},
		{Ref: "src/revoked", Name: "revoked", Err: errors.New("no such profile"), UnknownProfile: true},
	}
	prev := StampedTags{
		"keep": {"t"}, "upd": {"old"}, "gone": {"t"}, "frozen": {"old"}, "revoked": {"old"}, "foreign": {"t"},
	}
	snapshot := maps.Clone(prev)

	p := planWith(cfg, ingressOwn(), items, prev)
	want := StampedTags{
		"new": {"t"}, "keep": {"t"}, "upd": {"t"}, "unremembered": {"t"},
		"frozen": {"old"}, "revoked": {"old"},
	}
	if !reflect.DeepEqual(p.Stamped, want) {
		t.Fatalf("memory %v, want %v", p.Stamped, want)
	}
	if !reflect.DeepEqual(prev, snapshot) {
		t.Fatalf("Plan modified its input memory: %v", prev)
	}
	if !slices.Equal(p.Deletes, []string{"gone"}) {
		t.Fatalf("deletes %v, want [gone]", p.Deletes)
	}
	// The memory is a copy: writing to it cannot reach the next plan's input.
	p.Stamped["keep"][0] = "mutated"
	if prev["keep"][0] != "t" {
		t.Fatal("the returned memory shares a backing array with the input")
	}
}

// The hard requirement for a deployment that does not use the new behaviour:
// hosts written before it (no trustedProxies key, tags equal to the template's,
// in YAML exactly as the store wrote them) come out byte-identical, with no
// memory and with the memory of a previous pass, so no reconcile writes.
func TestPlanLegacyHostsAreUnchanged(t *testing.T) {
	tests := []struct {
		name string
		yaml string // the tags line of the stored file, "" for none
		tmpl []string
	}{
		{"no tags anywhere", "", nil},
		{"explicit empty tags, template without", "tags: []\n", nil},
		{"no tags, template with an explicit empty list", "", []string{}},
		{"explicit empty tags and template list", "tags: []\n", []string{}},
		{"template tags", "tags:\n    - cluster\n    - discovered\n", []string{"cluster", "discovered"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cfg model.Config
			for _, name := range []string{"a", "b", "c"} {
				doc := "name: " + name + "\n" +
					"labels:\n    gpm.rake.pro/managed-by: ingress-discovery\n" + tc.yaml +
					"domains:\n    - " + name + ".example.com\n" +
					"upstream:\n    scheme: http\n    host: 192.0.2.1\n    port: 80\n"
				var h model.ProxyHost
				if err := yaml.Unmarshal([]byte(doc), &h); err != nil {
					t.Fatalf("yaml.Unmarshal: %v", err)
				}
				if h.TrustedProxies != nil {
					t.Fatalf("fixture %s already has trustedProxies", name)
				}
				cfg.ProxyHosts = append(cfg.ProxyHosts, h)
			}
			var items []Item
			for _, h := range cfg.ProxyHosts {
				d := derivedHost(h.Name, append([]string(nil), tc.tmpl...), nil, h.Domains...)
				items = append(items, Item{Ref: "src/" + h.Name, Name: h.Name, Host: d})
			}
			mem := StampedTags(nil)
			for pass := range 3 {
				p := planWith(cfg, ingressOwn(), items, mem)
				if len(p.Upserts) != 0 || len(p.Deletes) != 0 || p.Updated != 0 {
					t.Fatalf("pass %d: %d upserts, %d deletes, want none: %+v", pass, len(p.Upserts), len(p.Deletes), p.Hosts)
				}
				for _, hr := range p.Hosts {
					if hr.Action != ActionUnchanged {
						t.Fatalf("pass %d: %+v, want unchanged", pass, hr)
					}
				}
				mem = p.Stamped
			}
		})
	}
}

// SameHost is what decides "no write"; the merged tags of a host whose tags
// equal the template's must be the stored slice itself, so nil and an empty
// list never compare as a change in either direction.
func TestMergeTagsReturnsTheStoredSliceWhenEqual(t *testing.T) {
	empty := []string{}
	tests := []struct {
		name       string
		tmpl, cur  []string
		prev       []string
		wantStored bool
	}{
		{"nil template, empty stored", nil, empty, nil, true},
		{"empty template, nil stored", empty, nil, nil, true},
		{"equal lists", []string{"a"}, []string{"a"}, []string{"a"}, true},
		{"different", []string{"a"}, []string{"b"}, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeTags(tc.tmpl, tc.cur, tc.prev)
			a := model.ProxyHost{ObjectMeta: model.ObjectMeta{Tags: tc.cur}}
			b := model.ProxyHost{ObjectMeta: model.ObjectMeta{Tags: got}}
			if SameHost(a, b) != tc.wantStored {
				t.Fatalf("mergeTags(%v, %v) = %#v; SameHost with the stored %#v = %v, want %v", tc.tmpl, tc.cur, got, tc.cur, !tc.wantStored, tc.wantStored)
			}
		})
	}
}

// planWith runs Plan with mem as the previous pass's tag memory.
func planWith(cfg model.Config, o Ownership, items []Item, mem StampedTags) Result {
	o.Stamped = mem
	return Plan(cfg, o, items)
}

// Unwritten keeps the previous entry (or none) for every host a plan wanted
// to write and the new entry for the rest, without touching either input.
func TestStampedTagsUnwritten(t *testing.T) {
	next := StampedTags{"upd": {"new"}, "created": {"new"}, "same": {"new"}}
	prev := StampedTags{"upd": {"old"}, "same": {"old"}, "gone": {"old"}}
	got := next.Unwritten(prev, []model.ProxyHost{{ObjectMeta: model.ObjectMeta{Name: "upd"}}, {ObjectMeta: model.ObjectMeta{Name: "created"}}})
	want := StampedTags{"upd": {"old"}, "same": {"new"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Unwritten = %v, want %v", got, want)
	}
	if !slices.Equal(next["upd"], []string{"new"}) || len(next) != 3 || !slices.Equal(prev["upd"], []string{"old"}) {
		t.Fatalf("inputs modified: next=%v prev=%v", next, prev)
	}
	got["upd"][0] = "mutated"
	if prev["upd"][0] != "old" {
		t.Fatal("the result shares a backing array with prev")
	}
}

// A marker under an old prefix (with no migration under way) or with the
// other reconciler's value still marks the value as stamped: an unset template
// clears value and marker, a set one restamps the value and rewrites the
// marker under the current prefix with this reconciler's value. One write,
// then nothing, on the plain update, the conflict restamp and the fail-closed
// write alike.
func TestPlanTrustedProxiesForeignAndOldMarkers(t *testing.T) {
	o := ingressOwn() // Migrate is off
	wide := tpList("0.0.0.0/0")
	markers := map[string][2]string{
		"old prefix":           {"old.example.com/trusted-proxies-by", model.ManagedByIngressDiscovery},
		"other reconciler":     {tpKey, model.ManagedByDockerDiscovery},
		"old prefix and other": {"old.example.com/trusted-proxies-by", model.ManagedByDockerDiscovery},
	}
	only := func(h model.ProxyHost, want map[string]string) bool {
		return maps.Equal(h.Labels, want)
	}
	plain := map[string]string{"gpm.rake.pro/managed-by": model.ManagedByIngressDiscovery}
	stampedLabels := map[string]string{"gpm.rake.pro/managed-by": model.ManagedByIngressDiscovery, tpKey: model.ManagedByIngressDiscovery}
	for name, m := range markers {
		stored := func() model.ProxyHost {
			h := derivedHost("app", nil, wide, "app.example.com")
			h.Labels[m[0]] = m[1]
			return h
		}
		t.Run(name+"/update, unset", func(t *testing.T) {
			cfg, _, p := settle(t, model.Config{ProxyHosts: []model.ProxyHost{stored()}}, o,
				[]Item{{Ref: "src", Name: "app", Host: derivedHost("app", nil, nil, "app.example.com")}}, nil)
			if h := cfg.ProxyHosts[0]; len(p.Upserts) != 1 || h.TrustedProxies != nil || !only(h, plain) {
				t.Fatalf("host %+v, want no value and no marker", h)
			}
		})
		t.Run(name+"/update, set", func(t *testing.T) {
			cfg, _, p := settle(t, model.Config{ProxyHosts: []model.ProxyHost{stored()}}, o,
				[]Item{{Ref: "src", Name: "app", Host: derivedHost("app", nil, tpList("192.0.2.10"), "app.example.com")}}, nil)
			if h := cfg.ProxyHosts[0]; len(p.Upserts) != 1 || tpString(h.TrustedProxies) != "[192.0.2.10]" || !only(h, stampedLabels) {
				t.Fatalf("host %+v, want the template's value and only the current marker", h)
			}
		})
		for _, set := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/restamp, set=%v", name, set), func(t *testing.T) {
				tmpl := model.IngressHostTemplate{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.1", Port: 80}}
				wantTP, wantLabels := "<unset>", plain
				if set {
					tmpl.TrustedProxies = tpList("192.0.2.10")
					wantTP, wantLabels = "[192.0.2.10]", stampedLabels
				}
				want := derivedHost("app", nil, tmpl.TrustedProxies, "app.example.com", "new.example.com")
				cfg := model.Config{ProxyHosts: []model.ProxyHost{stored(), host("op", "", "new.example.com")}}
				items := []Item{{Ref: "src", Name: "app", Host: want, Resolved: &tmpl}}
				got, _, p := settle(t, cfg, o, items, nil)
				if h := got.ProxyHosts[0]; len(p.Upserts) != 1 || h.Disabled || tpString(h.TrustedProxies) != wantTP || !only(h, wantLabels) {
					t.Fatalf("restamped host %+v, want trustedProxies %s and labels %v", h, wantTP, wantLabels)
				}
			})
		}
		t.Run(name+"/fail-closed", func(t *testing.T) {
			p := Plan(model.Config{ProxyHosts: []model.ProxyHost{stored()}}, o,
				[]Item{{Ref: "src", Name: "app", Err: errors.New("no such profile"), UnknownProfile: true}})
			want := maps.Clone(stampedLabels)
			want["gpm.rake.pro/disabled-by"] = model.ManagedByIngressDiscovery
			if len(p.Upserts) != 1 || !p.Upserts[0].Disabled || tpString(p.Upserts[0].TrustedProxies) != "[0.0.0.0/0]" || !only(p.Upserts[0], want) {
				t.Fatalf("upserts %+v, want a disable keeping the value with only the current marker", p.Upserts)
			}
		})
	}
}

// A label that only looks like a marker (another value) is not one: the value
// stays the operator's.
func TestPlanTrustedProxiesMarkerNeedsADiscoveryValue(t *testing.T) {
	h := derivedHost("app", nil, tpList("192.0.2.10"), "app.example.com")
	h.Labels["old.example.com/trusted-proxies-by"] = "helm"
	p := Plan(model.Config{ProxyHosts: []model.ProxyHost{h}}, ingressOwn(),
		[]Item{{Ref: "src", Name: "app", Host: derivedHost("app", nil, nil, "app.example.com")}})
	if len(p.Upserts) != 1 || tpString(p.Upserts[0].TrustedProxies) != "[192.0.2.10]" {
		t.Fatalf("upserts %+v, want the operator's value kept", p.Upserts)
	}
}
