package discovery

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// The per-source packages (internal/k8s, internal/docker) exercise this planner
// end to end through their own derive functions. What is tested here is the one
// property neither of them can test alone: two reconcilers sharing a label KEY
// and telling ownership apart by its VALUE must be blind to each other.

func own(subsystem, value string) Ownership {
	return Ownership{
		Subsystem:     subsystem,
		SourceKind:    "source",
		ManagedByKey:  "gpm.rake.pro/managed-by",
		DisabledByKey: "gpm.rake.pro/disabled-by",
		Value:         value,
	}
}

func host(name, value string, domains ...string) model.ProxyHost {
	h := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: name},
		Domains:    domains,
		Upstream:   model.Upstream{Scheme: "http", Host: "192.0.2.1", Port: 80},
	}
	if value != "" {
		h.Labels = map[string]string{"gpm.rake.pro/managed-by": value}
	}
	return h
}

func TestOwnershipIsKeyedOnTheLabelValue(t *testing.T) {
	a, b := own("a discovery", "a-discovery"), own("b discovery", "b-discovery")

	tests := []struct {
		name  string
		host  model.ProxyHost
		wantA bool
		wantB bool
	}{
		{"unlabelled", host("h", ""), false, false},
		{"a's own", host("h", "a-discovery"), true, false},
		{"b's own", host("h", "b-discovery"), false, true},
		{"some other value", host("h", "helm"), false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.Managed(tc.host); got != tc.wantA {
				t.Errorf("a.Managed = %v, want %v", got, tc.wantA)
			}
			if got := b.Managed(tc.host); got != tc.wantB {
				t.Errorf("b.Managed = %v, want %v", got, tc.wantB)
			}
		})
	}
}

// A host owned by the OTHER reconciler is neither adopted nor deleted: it is an
// ordinary foreign object, exactly like a hand-written one.
func TestPlanNeverTouchesAnotherSubsystemsHosts(t *testing.T) {
	theirs := host("their-app", "b-discovery", "their.example.com")
	mine := host("my-app", "a-discovery", "mine.example.com")
	cfg := model.Config{ProxyHosts: []model.ProxyHost{theirs, mine}}

	// Nothing derives anything this run: mine is an orphan, theirs is invisible.
	p := Plan(cfg, own("a discovery", "a-discovery"), nil)
	if len(p.Deletes) != 1 || p.Deletes[0] != "my-app" {
		t.Fatalf("deletes %v, want only my own orphan", p.Deletes)
	}
	if len(p.Upserts) != 0 {
		t.Fatalf("upserts %v, want none", p.Upserts)
	}

	// And a derived host that collides with theirs by NAME is skipped, not
	// overwritten.
	want := host("their-app", "a-discovery", "other.example.com")
	p = Plan(cfg, own("a discovery", "a-discovery"), []Item{{Ref: "src", Name: "their-app", Host: want}})
	if len(p.Upserts) != 0 || p.Skipped != 1 {
		t.Fatalf("plan %+v, want a skip and no write", p)
	}
	if !strings.Contains(p.Hosts[0].Reason, "not managed by a discovery") {
		t.Fatalf("reason %q, want it to name the owning subsystem", p.Hosts[0].Reason)
	}
}

func TestPlanDeriveFailureProtectsTheStoredHost(t *testing.T) {
	cur := host("app", "a-discovery", "app.example.com")
	cfg := model.Config{ProxyHosts: []model.ProxyHost{cur}}

	tests := []struct {
		name           string
		unknownProfile bool
		wantDeletes    int
		wantUpserts    int
		wantDisabled   bool
	}{
		{"a malformed source freezes the host", false, 0, 0, false},
		{"an unresolvable profile disables it", true, 0, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Plan(cfg, own("a discovery", "a-discovery"), []Item{{
				Ref: "src", Name: "app", Err: errors.New("boom"), UnknownProfile: tc.unknownProfile,
			}})
			if len(p.Deletes) != tc.wantDeletes || len(p.Upserts) != tc.wantUpserts {
				t.Fatalf("plan %+v, want %d deletes and %d upserts", p, tc.wantDeletes, tc.wantUpserts)
			}
			if tc.wantDisabled {
				if !p.Upserts[0].Disabled || p.Upserts[0].Labels["gpm.rake.pro/disabled-by"] != "a-discovery" {
					t.Fatalf("upsert %+v, want a labelled fail-closed disable", p.Upserts[0])
				}
				if cur.Disabled || len(cur.Labels) != 1 {
					t.Fatalf("the stored host was mutated in place: %+v", cur)
				}
			}
		})
	}
}

// A new source item whose name sorts before an existing managed host must not
// take that host's domain: the existing owner keeps it and the newcomer is the
// one skipped. Before this rule the newcomer claimed first (placement runs in
// name order), the existing host was skipped while its stored object kept the
// same domain, and the batch carried a duplicate domain that failed
// validation on every poll (issue #50).
func TestExistingManagedHostKeepsItsDomainAheadOfANewClaimant(t *testing.T) {
	o := own("a discovery", "a-discovery")
	existing := host("zeta", "a-discovery", "app.example.com")
	cfg := model.Config{ProxyHosts: []model.ProxyHost{existing}}
	items := []Item{
		{Ref: "ns/alpha", Name: "alpha", Host: host("alpha", "a-discovery", "app.example.com")},
		{Ref: "ns/zeta", Name: "zeta", Host: host("zeta", "a-discovery", "app.example.com")},
	}
	p := Plan(cfg, o, items)
	if p.Skipped != 1 {
		t.Fatalf("skipped = %d, want the newcomer alone: %+v", p.Skipped, p.Hosts)
	}
	for _, h := range p.Hosts {
		switch h.Name {
		case "alpha":
			if h.Action != ActionSkipped || !strings.Contains(h.Reason, `claimed by proxy host "zeta"`) {
				t.Fatalf("alpha = %+v, want skipped in favour of zeta", h)
			}
		case "zeta":
			if h.Action == ActionSkipped {
				t.Fatalf("zeta must keep its domain, got %+v", h)
			}
		}
	}
	for _, u := range p.Upserts {
		if u.Name == "alpha" {
			t.Fatalf("alpha must not be written: %+v", u)
		}
	}
	// An existing managed host moving onto a domain ANOTHER existing managed
	// host serves today is the one skipped, whatever the names sort like: the
	// domain stays with the host that has it, so the applied batch never
	// carries a duplicate.
	cfg2 := model.Config{ProxyHosts: []model.ProxyHost{
		host("alpha", "a-discovery", "one.example.com"),
		host("beta", "a-discovery", "two.example.com"),
	}}
	p2 := Plan(cfg2, o, []Item{
		{Ref: "ns/alpha", Name: "alpha", Host: host("alpha", "a-discovery", "two.example.com")},
		{Ref: "ns/beta", Name: "beta", Host: host("beta", "a-discovery", "two.example.com")},
	})
	if p2.Skipped != 1 {
		t.Fatalf("skipped = %d, want alpha alone: %+v", p2.Skipped, p2.Hosts)
	}
	for _, h := range p2.Hosts {
		if h.Name == "alpha" && h.Action != ActionSkipped {
			t.Fatalf("alpha must be skipped, got %+v", h)
		}
		if h.Name == "beta" && h.Action == ActionSkipped {
			t.Fatalf("beta must keep two.example.com, got %+v", h)
		}
	}
	for _, u := range p2.Upserts {
		if u.Name == "alpha" {
			t.Fatalf("alpha must not be written: %+v", u)
		}
	}
	// Applying what the plan writes on top of what it leaves alone must
	// validate: no duplicate domain.
	byName := map[string]model.ProxyHost{}
	for _, h := range cfg2.ProxyHosts {
		byName[h.Name] = h
	}
	for _, u := range p2.Upserts {
		byName[u.Name] = u // SaveBatch replaces by name
	}
	for _, d := range p2.Deletes {
		delete(byName, d)
	}
	applied := model.Config{}
	for _, h := range byName {
		applied.ProxyHosts = append(applied.ProxyHosts, h)
	}
	if err := applied.Validate(); err != nil {
		t.Fatalf("applied plan does not validate: %v", err)
	}
}

// applyPlan is what ApplyBatch does with a plan, minus the disk: upserts
// replace by name, except that an upsert of a stored host in unwritable is
// dropped (the store's unknown-keys skip), deletes remove, and the result is
// one config.
func applyPlan(cfg model.Config, p Result, unwritable map[string]bool) model.Config {
	byName := map[string]model.ProxyHost{}
	var order []string
	for _, h := range cfg.ProxyHosts {
		byName[h.Name] = h
		order = append(order, h.Name)
	}
	for _, u := range p.Upserts {
		_, stored := byName[u.Name]
		if stored && unwritable[u.Name] {
			continue
		}
		if !stored {
			order = append(order, u.Name)
		}
		byName[u.Name] = u
	}
	for _, d := range p.Deletes {
		delete(byName, d)
	}
	out := model.Config{RedirectHosts: cfg.RedirectHosts, ParkedHosts: cfg.ParkedHosts}
	for _, n := range order {
		if h, ok := byName[n]; ok {
			out.ProxyHosts = append(out.ProxyHosts, h)
		}
	}
	return out
}

// Domain moves between hosts of the same source (issue #74). A move converges
// in ONE poll exactly when every domain it gains is released by a managed host
// of this source that is itself moving or deleted in the same plan; anything
// else - a hand-written holder, the other discovery source, a managed host that
// keeps the domain, a protected host - refuses the move, and refusal propagates
// back along a chain so no host is stranded half-moved.
func TestPlanDomainMovesBetweenManagedHosts(t *testing.T) {
	const (
		one  = "one.example.com"
		two  = "two.example.com"
		thr  = "three.example.com"
		hand = "hand.example.com"
	)
	mine := func(name string, domains ...string) model.ProxyHost { return host(name, "a-discovery", domains...) }
	item := func(name string, domains ...string) Item {
		return Item{Ref: "src/" + name, Name: name, Host: mine(name, domains...)}
	}
	tests := []struct {
		name        string
		cfg         []model.ProxyHost
		items       []Item
		wantUpserts []string
		wantDeletes []string
		// wantSkipped maps each skipped host to a substring of its reason.
		wantSkipped map[string]string
		// unwritable names stored hosts the store refuses to rewrite.
		unwritable []string
		redirects  []model.RedirectHost
	}{
		{
			name:        "two-host swap",
			cfg:         []model.ProxyHost{mine("alpha", one), mine("beta", two)},
			items:       []Item{item("alpha", two), item("beta", one)},
			wantUpserts: []string{"alpha", "beta"},
		},
		{
			name:        "three-host cycle",
			cfg:         []model.ProxyHost{mine("a", one), mine("b", two), mine("c", thr)},
			items:       []Item{item("a", two), item("b", thr), item("c", one)},
			wantUpserts: []string{"a", "b", "c"},
		},
		{
			name:        "chain with a free tail",
			cfg:         []model.ProxyHost{mine("a", one), mine("b", two)},
			items:       []Item{item("a", two), item("b", thr)},
			wantUpserts: []string{"a", "b"},
		},
		{
			name: "swap plus an unrelated create and delete",
			cfg:  []model.ProxyHost{mine("alpha", one), mine("beta", two), mine("delta", "delta.example.com")},
			items: []Item{item("alpha", two), item("beta", one),
				item("gamma", "gamma.example.com")},
			wantUpserts: []string{"alpha", "beta", "gamma"},
			wantDeletes: []string{"delta"},
		},
		{
			name:        "a newcomer takes a domain a moving host releases",
			cfg:         []model.ProxyHost{mine("alpha", one)},
			items:       []Item{item("alpha", two), item("gamma", one)},
			wantUpserts: []string{"alpha", "gamma"},
		},
		{
			name:  "swap where one side collides with a hand-written host",
			cfg:   []model.ProxyHost{mine("alpha", one), mine("beta", two), host("op", "", hand)},
			items: []Item{item("alpha", two), item("beta", one, hand)},
			wantSkipped: map[string]string{
				"alpha": `"two.example.com" is already claimed by proxy host "beta"`,
				"beta":  `"hand.example.com" is already claimed by proxy host "op"`,
			},
		},
		{
			name:  "swap where one side collides with the other discovery source",
			cfg:   []model.ProxyHost{mine("alpha", one), mine("beta", two), host("theirs", "b-discovery", hand)},
			items: []Item{item("alpha", two), item("beta", one, hand)},
			wantSkipped: map[string]string{
				"alpha": `claimed by proxy host "beta"`,
				"beta":  `claimed by proxy host "theirs"`,
			},
		},
		{
			name:  "chain whose tail collides with a hand-written host is skipped whole",
			cfg:   []model.ProxyHost{mine("a", one), mine("b", two), mine("c", thr), host("op", "", hand)},
			items: []Item{item("a", two), item("b", thr), item("c", hand)},
			wantSkipped: map[string]string{
				"a": `claimed by proxy host "b"`,
				"b": `claimed by proxy host "c"`,
				"c": `claimed by proxy host "op"`,
			},
		},
		{
			name: "chain whose tail collides with a protected managed host is skipped whole",
			cfg:  []model.ProxyHost{mine("a", one), mine("b", two), mine("p", hand)},
			items: []Item{item("a", two), item("b", hand),
				{Ref: "src/p", Name: "p", Err: errors.New("malformed")}},
			wantSkipped: map[string]string{
				"a": `claimed by proxy host "b"`,
				"b": `claimed by proxy host "p"`,
				"p": "malformed",
			},
		},
		{
			name:  "the other side keeps the domain instead of releasing it",
			cfg:   []model.ProxyHost{mine("alpha", one), mine("beta", two)},
			items: []Item{item("alpha", two), item("beta", one, two)},
			wantSkipped: map[string]string{
				"alpha": `claimed by proxy host "beta"`,
				"beta":  `claimed by proxy host "alpha"`,
			},
		},
		{
			name:        "two moving hosts want one free domain: the first by name wins",
			cfg:         []model.ProxyHost{mine("alpha", one), mine("beta", two)},
			items:       []Item{item("alpha", thr), item("beta", thr)},
			wantUpserts: []string{"alpha"},
			wantSkipped: map[string]string{"beta": `claimed by proxy host "alpha"`},
		},
		{
			name:        "two moving hosts want one released domain: the first by name wins",
			cfg:         []model.ProxyHost{mine("alpha", one), mine("beta", two), mine("gamma", thr)},
			items:       []Item{item("alpha", thr), item("beta", thr), item("gamma", "four.example.com")},
			wantUpserts: []string{"alpha", "gamma"},
			wantSkipped: map[string]string{"beta": `claimed by proxy host "alpha"`},
		},
		{
			name:        "a newcomer may not take a domain a refused host still holds",
			cfg:         []model.ProxyHost{mine("alpha", one), host("op", "", hand)},
			items:       []Item{item("alpha", hand), item("gamma", one)},
			wantSkipped: map[string]string{"alpha": `claimed by proxy host "op"`, "gamma": `claimed by proxy host "alpha"`},
		},
		{
			// a takes n.x in the first round and b loses the contest; a is then
			// refused (b holds b.x), so n.x is free again and b moves after all.
			name:        "a contest loser moves once the winner is refused",
			cfg:         []model.ProxyHost{mine("a", "a.example.com"), mine("b", "b.example.com")},
			items:       []Item{item("a", "n.example.com", "b.example.com"), item("b", "n.example.com")},
			wantUpserts: []string{"b"},
			wantSkipped: map[string]string{"a": `"n.example.com" is already claimed by proxy host "b", also managed by a discovery`},
		},
		{
			name: "a contest loser that moves after all still beats a newcomer",
			cfg:  []model.ProxyHost{mine("a", "a.example.com"), mine("b", "b.example.com")},
			items: []Item{item("a", "n.example.com", "b.example.com"), item("b", "n.example.com"),
				item("aa", "n.example.com")},
			wantUpserts: []string{"b"},
			wantSkipped: map[string]string{
				"a":  `claimed by proxy host "b"`,
				"aa": `claimed by proxy host "b"`,
			},
		},
		{
			name:       "swap with an unwritable side moves nothing",
			cfg:        []model.ProxyHost{mine("alpha", one), mine("beta", two)},
			items:      []Item{item("alpha", two), item("beta", one)},
			unwritable: []string{"alpha"},
			wantSkipped: map[string]string{
				"alpha": "keys this gpm does not recognise",
				"beta":  `"one.example.com" is already claimed by proxy host "alpha"`,
			},
		},
		{
			name:        "a newcomer may not take an unwritable host's domain",
			cfg:         []model.ProxyHost{mine("alpha", one)},
			items:       []Item{item("alpha", two), item("gamma", one)},
			unwritable:  []string{"alpha"},
			wantSkipped: map[string]string{"alpha": "keys this gpm does not recognise", "gamma": `claimed by proxy host "alpha"`},
		},
		{
			// Same domains, another field changed: planned as before, and the
			// store is what skips the write.
			name: "an unwritable host whose domains stay put is still planned as an update",
			cfg:  []model.ProxyHost{mine("alpha", one)},
			items: []Item{{Ref: "src/alpha", Name: "alpha", Host: func() model.ProxyHost {
				h := mine("alpha", one)
				h.Upstream.Port = 8080
				return h
			}()}},
			unwritable:  []string{"alpha"},
			wantUpserts: []string{"alpha"},
		},
		{
			name: "a redirect holder is named as one",
			cfg:  []model.ProxyHost{mine("alpha", one)},
			redirects: []model.RedirectHost{{ObjectMeta: model.ObjectMeta{Name: "r"}, Domains: []string{hand},
				TargetDomain: "elsewhere.example.com"}},
			items: []Item{item("alpha", one, hand)},
			wantSkipped: map[string]string{
				"alpha": `"hand.example.com" is already claimed by redirect host "r", which a discovery does not manage`,
			},
		},
		{
			// The kept domain sorts first; the gained one is held by an operator.
			name:        "a host keeping one domain may not gain a hand-written host's",
			cfg:         []model.ProxyHost{mine("alpha", "a.example.com"), host("op", "", hand)},
			items:       []Item{item("alpha", "a.example.com", hand)},
			wantSkipped: map[string]string{"alpha": `claimed by proxy host "op"`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := own("a discovery", "a-discovery")
			unwritable := map[string]bool{}
			for _, n := range tc.unwritable {
				unwritable[n] = true
			}
			o.Unwritable = func(h model.ProxyHost) bool { return unwritable[h.Name] }
			cfg := model.Config{ProxyHosts: tc.cfg, RedirectHosts: tc.redirects}
			p := Plan(cfg, o, tc.items)

			var gotUpserts []string
			for _, u := range p.Upserts {
				gotUpserts = append(gotUpserts, u.Name)
			}
			if strings.Join(gotUpserts, ",") != strings.Join(tc.wantUpserts, ",") {
				t.Fatalf("upserts = %v, want %v (hosts %+v)", gotUpserts, tc.wantUpserts, p.Hosts)
			}
			if strings.Join(p.Deletes, ",") != strings.Join(tc.wantDeletes, ",") {
				t.Fatalf("deletes = %v, want %v", p.Deletes, tc.wantDeletes)
			}
			if p.Skipped != len(tc.wantSkipped) {
				t.Fatalf("skipped = %d, want %d: %+v", p.Skipped, len(tc.wantSkipped), p.Hosts)
			}
			for _, h := range p.Hosts {
				want, skip := tc.wantSkipped[h.Name]
				if skip != (h.Action == ActionSkipped) {
					t.Fatalf("%s: action %q, want skipped=%v", h.Name, h.Action, skip)
				}
				if skip && !strings.Contains(h.Reason, want) {
					t.Fatalf("%s: reason %q, want it to contain %q", h.Name, h.Reason, want)
				}
			}

			// The batch is validated as one config before it is committed, so
			// what the plan writes on top of what it leaves alone must validate.
			applied := applyPlan(cfg, p, unwritable)
			if err := applied.Validate(); err != nil {
				t.Fatalf("applied plan does not validate: %v", err)
			}
			// And the next poll against the applied state changes nothing (an
			// upsert the store drops may be planned again; it still lands nowhere).
			again := Plan(applied, o, tc.items)
			if len(again.Deletes) != 0 || !reflect.DeepEqual(applyPlan(applied, again, unwritable), applied) {
				t.Fatalf("second poll changes the config: upserts=%d deletes=%d %+v", len(again.Upserts), len(again.Deletes), again.Hosts)
			}
			for _, u := range again.Upserts {
				if !unwritable[u.Name] {
					t.Fatalf("second poll rewrites %s: %+v", u.Name, again.Hosts)
				}
			}
		})
	}
}

// Random configs and source states, fixed seed. Whatever the mix of managed,
// hand-written, other-source, redirect, parked, protected, unwritable, doomed
// and new hosts, the plan must be safe, deterministic and a fixed point.
func TestPlanDomainMovesRandomized(t *testing.T) {
	r := rand.New(rand.NewPCG(74, 2026))
	pool := make([]string, 9)
	for i := range pool {
		pool[i] = fmt.Sprintf("d%d.example.com", i)
	}
	o := own("a discovery", "a-discovery")
	// pick returns 1-2 distinct domains from the whole pool.
	pick := func() []string {
		idx := r.Perm(len(pool))[:1+r.IntN(2)]
		sort.Ints(idx)
		var out []string
		for _, i := range idx {
			out = append(out, pool[i])
		}
		return out
	}
	sortedHosts := func(hs []HostResult) []HostResult {
		out := append([]HostResult(nil), hs...)
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}

	for n := 0; n < 3000; n++ {
		// The stored config holds each domain at most once, as a valid one does.
		free := r.Perm(len(pool))
		take := func() []string {
			var out []string
			for k := 1 + r.IntN(2); k > 0 && len(free) > 0; k-- {
				out = append(out, pool[free[0]])
				free = free[1:]
			}
			return out
		}
		var cfg model.Config
		var items []Item
		unwritable := map[string]bool{}
		for _, kind := range r.Perm(10) {
			ds := take()
			if len(ds) == 0 {
				break
			}
			name := fmt.Sprintf("h%d", kind)
			switch {
			case kind < 6: // managed by this source
				cfg.ProxyHosts = append(cfg.ProxyHosts, host(name, "a-discovery", ds...))
				unwritable[name] = r.IntN(4) == 0
				switch r.IntN(6) {
				case 0: // source gone: doomed
				case 1: // protected
					items = append(items, Item{Ref: "src/" + name, Name: name, Err: errors.New("malformed")})
				default:
					items = append(items, Item{Ref: "src/" + name, Name: name, Host: host(name, "a-discovery", pick()...)})
				}
			case kind == 6:
				cfg.ProxyHosts = append(cfg.ProxyHosts, host(name, "", ds...))
			case kind == 7:
				cfg.ProxyHosts = append(cfg.ProxyHosts, host(name, "b-discovery", ds...))
			case kind == 8:
				cfg.RedirectHosts = append(cfg.RedirectHosts, model.RedirectHost{ObjectMeta: model.ObjectMeta{Name: name},
					Domains: ds, TargetDomain: "elsewhere.example.com"})
			default:
				cfg.ParkedHosts = append(cfg.ParkedHosts, model.ParkedHost{ObjectMeta: model.ObjectMeta{Name: name}, Domains: ds})
			}
		}
		for k := r.IntN(3); k > 0; k-- {
			name := fmt.Sprintf("new%d", k)
			items = append(items, Item{Ref: "src/" + name, Name: name, Host: host(name, "a-discovery", pick()...)})
		}
		o.Unwritable = func(h model.ProxyHost) bool { return unwritable[h.Name] }

		p := Plan(cfg, o, items)
		fail := func(format string, args ...any) {
			t.Helper()
			t.Fatalf("case %d: %s\ncfg %+v\nitems %+v\nplan %+v", n, fmt.Sprintf(format, args...), cfg, items, p.Hosts)
		}

		applied := applyPlan(cfg, p, unwritable)
		if err := applied.Validate(); err != nil {
			fail("applied plan does not validate: %v", err)
		}
		for _, h := range applied.ProxyHosts {
			if len(h.Domains) == 0 {
				fail("%s ends with no domains", h.Name)
			}
		}

		// A host may gain a stored domain only from a managed host of this
		// source that is deleted, or rewritten without it, in this same batch.
		holder := map[string]string{}
		for _, h := range cfg.ProxyHosts {
			for _, d := range h.Domains {
				holder[d] = h.Name
			}
		}
		for _, h := range cfg.RedirectHosts {
			for _, d := range h.Domains {
				holder[d] = "redirect:" + h.Name
			}
		}
		for _, h := range cfg.ParkedHosts {
			for _, d := range h.Domains {
				holder[d] = "parked:" + h.Name
			}
		}
		stored := map[string]model.ProxyHost{}
		for _, h := range cfg.ProxyHosts {
			stored[h.Name] = h
		}
		landed := map[string]model.ProxyHost{}
		for _, h := range applied.ProxyHosts {
			landed[h.Name] = h
		}
		for _, h := range applied.ProxyHosts {
			had := domainSet(stored[h.Name].Domains)
			for _, d := range h.Domains {
				from, held := holder[d]
				if had[d] || !held {
					continue
				}
				after, kept := landed[from]
				if kept && (!o.Managed(after) || domainSet(after.Domains)[d] || reflect.DeepEqual(after, stored[from])) {
					fail("%s took %s from %s, which does not release it", h.Name, d, from)
				}
				if !kept && !slices.Contains(p.Deletes, from) {
					fail("%s took %s from %s, which is not a proxy host this plan deletes", h.Name, d, from)
				}
			}
		}

		// Input order must not matter.
		shuffledCfg := cfg
		shuffledCfg.ProxyHosts = append([]model.ProxyHost(nil), cfg.ProxyHosts...)
		r.Shuffle(len(shuffledCfg.ProxyHosts), func(i, j int) {
			shuffledCfg.ProxyHosts[i], shuffledCfg.ProxyHosts[j] = shuffledCfg.ProxyHosts[j], shuffledCfg.ProxyHosts[i]
		})
		shuffledItems := append([]Item(nil), items...)
		r.Shuffle(len(shuffledItems), func(i, j int) { shuffledItems[i], shuffledItems[j] = shuffledItems[j], shuffledItems[i] })
		q := Plan(shuffledCfg, o, shuffledItems)
		if !reflect.DeepEqual(q.Upserts, p.Upserts) || !reflect.DeepEqual(q.Deletes, p.Deletes) ||
			!reflect.DeepEqual(sortedHosts(q.Hosts), sortedHosts(p.Hosts)) {
			fail("plan depends on input order: %+v", q.Hosts)
		}

		// And the next poll changes nothing.
		again := Plan(applied, o, items)
		if len(again.Deletes) != 0 || !reflect.DeepEqual(applyPlan(applied, again, unwritable), applied) {
			fail("second poll changes the config: %+v", again.Hosts)
		}
	}
}
