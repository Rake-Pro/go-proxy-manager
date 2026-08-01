package dnssync

import (
	"context"
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

	// And the reconcile that follows takes exactly the decisions the plan showed.
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if st.Created != len(p.Create) || st.Adopted != len(p.Adopt) ||
		st.Deleted != len(p.Delete) || st.Skipped != len(p.Skip) || st.Untouched != p.Untouched {
		t.Fatalf("reconcile %+v does not match the plan %+v", st, p)
	}
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
