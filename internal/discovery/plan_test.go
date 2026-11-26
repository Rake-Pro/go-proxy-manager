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

// The domain gate keys on the router's key: an operator host whose stored
// domain carries a port routes as the bare name, so a discovered host asking
// for that bare name is skipped instead of taking the operator host's traffic.
func TestDomainGateKeysOnTheRouterKey(t *testing.T) {
	operator := host("app", "", "App.example.com:443")
	cfg := model.Config{ProxyHosts: []model.ProxyHost{operator}}
	p := Plan(cfg, own("a discovery", "a-discovery"), []Item{
		{Ref: "ns/ing", Name: "ing", Host: host("ing", "a-discovery", "app.example.com")},
	})
	if p.Skipped != 1 || len(p.Upserts) != 0 {
		t.Fatalf("a discovered host must not claim an operator host's router key: skipped=%d upserts=%+v", p.Skipped, p.Upserts)
	}
	if !strings.Contains(p.Hosts[0].Reason, `claimed by proxy host "app"`) {
		t.Fatalf("reason = %q, want the operator host named", p.Hosts[0].Reason)
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

// fill sets v to a non-zero value of its type, so the YAML form differs from
// the zero value's.
func fill(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k := reflect.New(v.Type().Key()).Elem()
		fill(t, k)
		m.SetMapIndex(k, reflect.New(v.Type().Elem()).Elem())
		v.Set(m)
	case reflect.Struct:
		fill(t, v.Field(0))
	default:
		t.Fatalf("fill: unhandled kind %s", v.Kind())
	}
}

// Every IngressHostTemplate field is either compared by ChainDiffers or listed
// here as deliberately excluded. A new template field fails this test until it
// is classified, so a field a derive stamps onto hosts cannot be left out of
// the comparison by accident.
func TestChainDiffersCoversEveryTemplateField(t *testing.T) {
	compared := map[string]bool{
		"Upstream": true, "UpstreamGroupRef": true, // Ingress discovery only
		"TLS": true, "RobotsNoIndex": true, "Timeouts": true,
		"Middlewares": true, "AccessLists": true, "AccessListsMode": true,
		"StripResponseHeaders": true, "SecurityHeaders": true, "Locations": true,
	}
	excluded := map[string]string{
		"WebsocketsUpgrade":     "deprecated, no effect",
		"Tags":                  "UI metadata with no data-plane effect",
		"DefaultDNS":            "overridden per source object; compared against the derived host where there is one",
		"AllowedDomainSuffixes": "not copied onto the host; checked by Ownership.DomainAllowed",
		"AllowedHostnames":      "not copied onto the host; checked by Ownership.DomainAllowed",
	}
	upstreamOnly := map[string]bool{"Upstream": true, "UpstreamGroupRef": true}

	typ := reflect.TypeOf(model.IngressHostTemplate{})
	for i := range typ.NumField() {
		f := typ.Field(i)
		_, skip := excluded[f.Name]
		if compared[f.Name] == skip {
			t.Errorf("IngressHostTemplate.%s must be listed exactly once, as compared or excluded", f.Name)
			continue
		}
		if skip {
			continue
		}
		t.Run(f.Name, func(t *testing.T) {
			var tmpl model.IngressHostTemplate
			fill(t, reflect.ValueOf(&tmpl).Elem().Field(i))
			if !ChainDiffers(model.ProxyHost{}, tmpl, true) {
				t.Fatalf("a changed %s is not reported", f.Name)
			}
			if got := ChainDiffers(model.ProxyHost{}, tmpl, false); got == upstreamOnly[f.Name] {
				t.Fatalf("upstream=false: ChainDiffers = %v for %s", got, f.Name)
			}
			// The host carrying the same value is what a derive stamps: equal.
			var h model.ProxyHost
			hf := reflect.ValueOf(&h).Elem().FieldByName(f.Name)
			if !hf.IsValid() {
				t.Fatalf("ProxyHost has no field %s", f.Name)
			}
			hf.Set(reflect.ValueOf(tmpl).Field(i))
			if ChainDiffers(h, tmpl, true) {
				t.Fatalf("a host stamped with the template's %s still differs", f.Name)
			}
		})
	}
}

// A stored host is frozen only while every template-driven field still
// matches its profile; a change to any of them (upstream, upstream group,
// timeouts, robotsNoIndex) fails it closed. The upstream counts only for
// Ingress discovery, where the profile supplies it.
func TestPlanDeriveFailureComparesEveryTemplateField(t *testing.T) {
	base := model.IngressHostTemplate{
		Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.1", Port: 80},
		TLS:      model.TLSSettings{CertificateRef: "wildcard"},
		Timeouts: &model.HostTimeouts{ReadSeconds: 30},
	}
	stored := func(value string) model.ProxyHost {
		h := host("app", value, "app.example.com")
		h.TLS = base.TLS
		h.Timeouts = &model.HostTimeouts{ReadSeconds: 30}
		return h
	}
	tests := []struct {
		name         string
		value        string
		change       func(*model.IngressHostTemplate)
		wantDisabled bool
	}{
		{"unchanged profile freezes", model.ManagedByIngressDiscovery, func(*model.IngressHostTemplate) {}, false},
		{"timeouts changed", model.ManagedByIngressDiscovery, func(t *model.IngressHostTemplate) { t.Timeouts = &model.HostTimeouts{ReadSeconds: 5} }, true},
		{"timeouts removed", model.ManagedByIngressDiscovery, func(t *model.IngressHostTemplate) { t.Timeouts = nil }, true},
		{"robotsNoIndex changed", model.ManagedByIngressDiscovery, func(t *model.IngressHostTemplate) { t.RobotsNoIndex = true }, true},
		{"upstream changed", model.ManagedByIngressDiscovery, func(t *model.IngressHostTemplate) { t.Upstream.Host = "192.0.2.2" }, true},
		{"upstream group set", model.ManagedByIngressDiscovery, func(t *model.IngressHostTemplate) {
			t.Upstream, t.UpstreamGroupRef = model.Upstream{}, "pool"
		}, true},
		{"tags only freeze", model.ManagedByIngressDiscovery, func(t *model.IngressHostTemplate) { t.Tags = []string{"x"} }, false},
		{"docker: profile upstream is ignored", model.ManagedByDockerDiscovery, func(t *model.IngressHostTemplate) { t.Upstream.Host = "192.0.2.2" }, false},
		{"docker: timeouts changed", model.ManagedByDockerDiscovery, func(t *model.IngressHostTemplate) { t.Timeouts = nil }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := base
			tc.change(&tmpl)
			cfg := model.Config{ProxyHosts: []model.ProxyHost{stored(tc.value)}}
			p := Plan(cfg, own("a discovery", tc.value), []Item{{
				Ref: "src", Name: "app", Err: errors.New("boom"), Resolved: &tmpl,
			}})
			disabled := len(p.Upserts) == 1 && p.Upserts[0].Disabled
			if disabled != tc.wantDisabled || (!tc.wantDisabled && len(p.Upserts) != 0) {
				t.Fatalf("upserts %+v, want disabled=%v", p.Upserts, tc.wantDisabled)
			}
		})
	}
}

// An existing host skipped for a domain conflict is restamped from its profile
// when a template-driven field or its derived dns policy changed, and the
// restamp carries every compared field, so the next poll sees it as current.
func TestPlanDomainConflictRestampsEveryTemplateField(t *testing.T) {
	o := own("a discovery", model.ManagedByIngressDiscovery)
	tmpl := model.IngressHostTemplate{
		Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.1", Port: 80},
		Timeouts: &model.HostTimeouts{ReadSeconds: 5},
	}
	want := host("app", model.ManagedByIngressDiscovery, "new.example.com")
	want.Timeouts = &model.HostTimeouts{ReadSeconds: 5}
	tests := []struct {
		name     string
		timeouts *model.HostTimeouts
		dns      *model.DNSSyncPolicy
	}{
		{"timeouts changed", &model.HostTimeouts{ReadSeconds: 30}, nil},
		{"derived dns policy changed", &model.HostTimeouts{ReadSeconds: 5}, &model.DNSSyncPolicy{LanDirect: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cur := host("app", model.ManagedByIngressDiscovery, "app.example.com")
			cur.Timeouts, cur.DNS = tc.timeouts, tc.dns
			cfg := model.Config{ProxyHosts: []model.ProxyHost{cur, host("op", "", "new.example.com")}}
			p := Plan(cfg, o, []Item{{Ref: "src", Name: "app", Host: want, Resolved: &tmpl}})
			if len(p.Upserts) != 1 {
				t.Fatalf("upserts %+v, want one restamp", p.Upserts)
			}
			upd := p.Upserts[0]
			if upd.Disabled || !slices.Equal(upd.Domains, cur.Domains) || upd.Timeouts == nil || upd.Timeouts.ReadSeconds != 5 || upd.DNS != nil {
				t.Fatalf("restamp %+v, want current domains, the profile's timeouts and no dns policy", upd)
			}
			// Once restamped, the host matches its profile and is left alone.
			cfg.ProxyHosts[0] = upd
			if p := Plan(cfg, o, []Item{{Ref: "src", Name: "app", Host: want, Resolved: &tmpl}}); len(p.Upserts) != 0 || p.Skipped != 1 {
				t.Fatalf("second poll %+v, want a plain skip", p)
			}
		})
	}
}

// A derived name that a redirect, parked or stream host already uses is
// skipped (the validator would reject the duplicate host name and fail the
// whole batch), and the rest of the batch still applies.
func TestPlanSkipsANameUsedByAnotherHostKind(t *testing.T) {
	cfg := model.Config{
		RedirectHosts: []model.RedirectHost{{ObjectMeta: model.ObjectMeta{Name: "redir"}, Domains: []string{"redir.example.com"}}},
		ParkedHosts:   []model.ParkedHost{{ObjectMeta: model.ObjectMeta{Name: "parked"}, Domains: []string{"parked.example.com"}}},
		StreamHosts:   []model.StreamHost{{ObjectMeta: model.ObjectMeta{Name: "stream"}}},
	}
	tests := []struct{ name, kind string }{
		{"redir", "redirect host"},
		{"parked", "parked host"},
		{"stream", "stream host"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Plan(cfg, own("a discovery", "a-discovery"), []Item{
				{Ref: "src", Name: tc.name, Host: host(tc.name, "a-discovery", "other.example.com")},
				{Ref: "ok", Name: "ok", Host: host("ok", "a-discovery", "ok.example.com")},
			})
			if p.Skipped != 1 || p.Created != 1 || len(p.Upserts) != 1 || p.Upserts[0].Name != "ok" {
				t.Fatalf("plan %+v, want %s skipped and ok created", p, tc.name)
			}
			for _, h := range p.Hosts {
				if h.Name == tc.name && (h.Action != ActionSkipped || !strings.Contains(h.Reason, "a "+tc.kind+" with this name exists")) {
					t.Fatalf("host %+v, want a skip naming the %s", h, tc.kind)
				}
			}
		})
	}
}
