package dnssync

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func join(v []string) string { return strings.Join(v, ",") }

// The dry run has to be exactly that: a preview that reports the decisions a
// reconcile would take and issues not one write while doing so. That is what
// makes enabling a backend on a resolver full of hand-written records checkable
// BEFORE it is done - the 2026-08-01 incident was only discoverable by running it.
func TestPlanPreviewsWithoutWriting(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"app.example.com,edge.example.com",   // desired + right target, unowned: adopt
		"plex.example.com,edge.example.com",  // same target, not desired: untouched
		"held.example.com,other-proxy.lan",   // desired name, foreign record: skip
		"stale.example.com,edge.example.com", // owned, no longer desired: delete
	}}
	srv := startPihole(t, fake)

	led := ownsPihole("stale.example.com", "edge.example.com")
	hosts := []model.ProxyHost{
		lanHost("app", "app.example.com"),
		lanHost("held", "held.example.com"),
		lanHost("new", "new.example.com"),
	}
	s := piholeSyncerWith(t, srv, hosts, led)

	plan, err := s.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	p := plan.Pihole
	if !p.Enabled || !p.OK || p.Error != "" {
		t.Fatalf("plan = %+v", p)
	}
	if join(p.Create) != "new.example.com" {
		t.Fatalf("create = %v", p.Create)
	}
	if join(p.Adopt) != "app.example.com" {
		t.Fatalf("adopt = %v", p.Adopt)
	}
	if join(p.Delete) != "stale.example.com" {
		t.Fatalf("delete = %v", p.Delete)
	}
	if join(p.Skip) != "held.example.com" {
		t.Fatalf("skip = %v", p.Skip)
	}
	// plex + held are gpm's to leave alone; app and stale it has a claim on.
	if p.Untouched != 2 {
		t.Fatalf("untouched = %d, want 2", p.Untouched)
	}

	// Nothing was written, and the ledger was not saved.
	fake.mu.Lock()
	writes := fake.writes
	fake.mu.Unlock()
	if writes != 0 {
		t.Fatalf("a dry run made %d writes against Pi-hole", writes)
	}
	if len(fake.snapshot()) != 4 {
		t.Fatalf("records = %v, want all four unchanged", fake.snapshot())
	}
	led.mu.Lock()
	saves := led.saves
	led.mu.Unlock()
	if saves != 0 {
		t.Fatalf("a dry run saved the ledger %d times", saves)
	}

	// And the reconcile that follows takes exactly the decisions the plan showed -
	// the same NAMES, not merely the same counts. A plan that lists the right
	// number of the wrong domains is not a preview of anything.
	before := domainsOf(fake.snapshot())
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	after := domainsOf(fake.snapshot())

	if got := difference(after, before); join(got) != join(p.Create) {
		t.Fatalf("reconcile created %v, the plan said %v", got, p.Create)
	}
	if got := difference(before, after); join(got) != join(p.Delete) {
		t.Fatalf("reconcile deleted %v, the plan said %v", got, p.Delete)
	}
	// Adoption leaves the backend alone, so it shows up as a claim on a record that
	// was already there and was not created by this run.
	var adopted []string
	for name, claim := range led.pihole() {
		if claim.Adopted && slices.Contains(before, name) {
			adopted = append(adopted, name)
		}
	}
	sort.Strings(adopted)
	if join(adopted) != join(p.Adopt) {
		t.Fatalf("reconcile adopted %v, the plan said %v", adopted, p.Adopt)
	}
	// The skipped name is still held by the foreign record it was held by.
	for _, name := range p.Skip {
		if !slices.Contains(after, name) {
			t.Fatalf("the skipped name %q is no longer in the resolver: %v", name, after)
		}
	}
	st := s.Status().Pihole
	if st.Created != len(p.Create) || st.Adopted != len(p.Adopt) ||
		st.Deleted != len(p.Delete) || st.Skipped != len(p.Skip) || st.Untouched != p.Untouched {
		t.Fatalf("reconcile %+v does not match the plan %+v", st, p)
	}
}

// domainsOf reduces raw "domain,target" Pi-hole entries to sorted domain names.
func domainsOf(records []string) []string {
	var out []string
	for _, rec := range records {
		if domain, _, ok := piholeRecordTarget(rec); ok {
			out = append(out, domain)
		}
	}
	sort.Strings(out)
	return out
}

// difference returns the sorted names in a that are not in b.
func difference(a, b []string) []string {
	var out []string
	for _, name := range a {
		if !slices.Contains(b, name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// A dry run against Cloudflare is read-only too: zone listing only, no create and
// no delete.
func TestPlanCloudflareIsReadOnly(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: []cfRecord{
		{ID: "ours", Type: "CNAME", Name: "app.example.com", Content: "edge.example.com", Comment: ManagedComment},
		{ID: "theirs", Type: "CNAME", Name: "docs.example.com", Content: "pages.dev", Comment: ""},
	}}
	srv := startCloudflare(t, fake)

	led := ownsCloudflare("gone.example.com", "edge.example.com")
	s := cloudflareSyncerWith(t, srv, []model.ProxyHost{
		publicHost("app", "app.example.com"),
		publicHost("new", "new.example.com"),
	}, false, led)

	plan, err := s.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	p := plan.Cloudflare
	if join(p.Create) != "new.example.com" || join(p.Adopt) != "app.example.com" {
		t.Fatalf("plan = %+v", p)
	}
	// gone.example.com is owned but no longer in the zone, so there is nothing to
	// delete - the ledger entry is simply dropped by the run.
	if len(p.Delete) != 0 {
		t.Fatalf("delete = %v, want none", p.Delete)
	}
	if p.Untouched != 1 {
		t.Fatalf("untouched = %d, want the one foreign record", p.Untouched)
	}
	fake.mu.Lock()
	writes := fake.writes
	fake.mu.Unlock()
	if writes != 0 {
		t.Fatalf("a dry run made %d writes against Cloudflare", writes)
	}
	led.mu.Lock()
	saves := led.saves
	led.mu.Unlock()
	if saves != 0 {
		t.Fatalf("a dry run saved the ledger %d times", saves)
	}
}

// The incident scenario, previewed: with nothing desired and nothing owned, the
// plan must show zero deletions and report every hand-written record as left
// alone. This is the check an operator gets to run before enabling a backend.
func TestPlanOnFirstEnableShowsNoDeletions(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"plex.example.com,edge.example.com",
		"argo.example.com,edge.example.com",
		"wiki.example.com,edge.example.com",
	}}
	srv := startPihole(t, fake)

	s := piholeSyncerWith(t, srv, nil, &memLedger{})
	plan, err := s.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Pihole.Delete) != 0 {
		t.Fatalf("REGRESSION: a first-enable plan proposes deleting %v", plan.Pihole.Delete)
	}
	if plan.Pihole.Untouched != 3 {
		t.Fatalf("untouched = %d, want all 3 hand-written records", plan.Pihole.Untouched)
	}
}

// A backend that is off contributes an empty, disabled plan rather than an error.
func TestPlanWithBackendsDisabled(t *testing.T) {
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{}, nil
	}, &memLedger{})
	plan, err := s.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Pihole.Enabled || plan.Cloudflare.Enabled {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.GeneratedAt.IsZero() {
		t.Fatal("a plan must stamp generatedAt")
	}
}

// A backend that cannot be reached reports its error in the plan instead of
// failing the whole preview.
func TestPlanReportsBackendError(t *testing.T) {
	fake := &fakePihole{password: "different"}
	srv := startPihole(t, fake)

	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")}, &memLedger{})
	plan, err := s.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Pihole.OK || !strings.Contains(plan.Pihole.Error, "appPassword") {
		t.Fatalf("plan = %+v", plan.Pihole)
	}
}

func TestPlanUnwired(t *testing.T) {
	var s *Syncer
	if _, err := s.Plan(context.Background()); err == nil {
		t.Fatal("a nil syncer must refuse a plan")
	}
}
