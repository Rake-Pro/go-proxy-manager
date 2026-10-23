package discovery

import (
	"errors"
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
