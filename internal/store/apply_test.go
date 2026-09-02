package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestStampMeta covers stampMeta directly with a controlled clock, since it is
// the rule CreatedAt-preservation actually depends on: UpdatedAt is always
// stamped to now, and CreatedAt is server-managed - an existing object's
// CreatedAt always wins over whatever the incoming object carries (including a
// client-supplied value), a genuinely new object honours an incoming
// CreatedAt if set, and a legacy existing object with a zero CreatedAt gets
// backfilled to now rather than staying zero.
func TestStampMeta(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	clientSupplied := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		incoming    model.ObjectMeta
		existing    *model.ObjectMeta
		wantCreated time.Time
	}{
		{
			name:        "new object, no incoming createdAt",
			incoming:    model.ObjectMeta{Name: "a"},
			existing:    nil,
			wantCreated: now,
		},
		{
			name:        "new object, incoming createdAt set (e.g. batch import)",
			incoming:    model.ObjectMeta{Name: "a", CreatedAt: created},
			existing:    nil,
			wantCreated: created,
		},
		{
			name:        "existing object, incoming createdAt omitted (web UI PUT)",
			incoming:    model.ObjectMeta{Name: "a"},
			existing:    &model.ObjectMeta{Name: "a", CreatedAt: created},
			wantCreated: created,
		},
		{
			name:        "existing object, incoming createdAt is a different client-supplied value: server wins",
			incoming:    model.ObjectMeta{Name: "a", CreatedAt: clientSupplied},
			existing:    &model.ObjectMeta{Name: "a", CreatedAt: created},
			wantCreated: created,
		},
		{
			name:        "existing object with a legacy zero createdAt is backfilled to now",
			incoming:    model.ObjectMeta{Name: "a", CreatedAt: clientSupplied},
			existing:    &model.ObjectMeta{Name: "a"},
			wantCreated: now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stampMeta(tt.incoming, tt.existing, now)
			if !got.CreatedAt.Equal(tt.wantCreated) {
				t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, tt.wantCreated)
			}
			if !got.UpdatedAt.Equal(now) {
				t.Errorf("UpdatedAt = %v, want %v (always stamped to now)", got.UpdatedAt, now)
			}
		})
	}
}

// stubObject is a minimal model.Object used only to exercise stampTimes'
// default branch for a kind it does not special-case.
type stubObject struct {
	model.ObjectMeta
}

func (stubObject) Kind() string    { return "Stub" }
func (stubObject) Validate() error { return nil }

func TestStampTimesUnknownKindPassesThrough(t *testing.T) {
	obj := stubObject{ObjectMeta: model.ObjectMeta{Name: "x"}}
	got := stampTimes(obj, nil, time.Now())
	if _, ok := got.(stubObject); !ok {
		t.Fatalf("expected the unmodified object back for an unhandled kind, got %#v", got)
	}
}

// TestSavePreservesCreatedAtAcrossUpdate is the end-to-end regression test for
// the bug: the web UI's PUT never sends createdAt, and every save used to
// reset it. Saving the same object twice, the second time with a fresh
// ObjectMeta that carries no CreatedAt (exactly what app.js sends), must keep
// the original creation time.
func TestSavePreservesCreatedAtAcrossUpdate(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}

	if _, err := st.Save(ctx, sampleHost("app"), author); err != nil {
		t.Fatalf("create: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(cfg.ProxyHosts))
	}
	createdAt := cfg.ProxyHosts[0].CreatedAt
	updatedAt := cfg.ProxyHosts[0].UpdatedAt
	if createdAt.IsZero() {
		t.Fatal("expected CreatedAt to be stamped on create")
	}

	// Simulate the UI's PUT: a fresh object, same name, no CreatedAt set, with
	// an unrelated field changed.
	update := sampleHost("app")
	update.Disabled = true
	if _, err := st.Save(ctx, update, author); err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, _, err = st.Load(ctx)
	if err != nil {
		t.Fatalf("load after update: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 {
		t.Fatalf("expected 1 host after update, got %d", len(cfg.ProxyHosts))
	}
	got := cfg.ProxyHosts[0]
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt changed on update: got %v, want %v (unchanged)", got.CreatedAt, createdAt)
	}
	if got.UpdatedAt.Before(updatedAt) {
		t.Errorf("UpdatedAt went backwards: got %v, was %v", got.UpdatedAt, updatedAt)
	}
	if !got.Disabled {
		t.Error("expected the update's field change to still apply")
	}
}

// TestSaveIgnoresClientSuppliedCreatedAtOnUpdate documents the server-wins
// rule: an update naming an object that already exists ignores whatever
// CreatedAt the client sends, even if it is a non-zero, deliberately
// different value - only a genuinely new object's CreatedAt is honoured.
func TestSaveIgnoresClientSuppliedCreatedAtOnUpdate(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}

	if _, err := st.Save(ctx, sampleHost("app"), author); err != nil {
		t.Fatalf("create: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	original := cfg.ProxyHosts[0].CreatedAt

	spoofed := sampleHost("app")
	spoofed.ObjectMeta.CreatedAt = time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := st.Save(ctx, spoofed, author); err != nil {
		t.Fatalf("update with spoofed createdAt: %v", err)
	}

	cfg, _, err = st.Load(ctx)
	if err != nil {
		t.Fatalf("load after spoofed update: %v", err)
	}
	got := cfg.ProxyHosts[0].CreatedAt
	if got.Equal(spoofed.ObjectMeta.CreatedAt) {
		t.Fatal("client-supplied createdAt on an update must be ignored, but it was written")
	}
	if !got.Equal(original) {
		t.Errorf("CreatedAt = %v, want the original %v", got, original)
	}
}

// TestSaveBatchPreservesCreatedAt covers the same rule through SaveBatch,
// which re-imports objects rather than saving one at a time.
func TestSaveBatchPreservesCreatedAt(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}

	if _, err := st.SaveBatch(ctx, []model.Object{sampleHost("app")}, "import", author); err != nil {
		t.Fatalf("initial batch: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	original := cfg.ProxyHosts[0].CreatedAt

	if _, err := st.SaveBatch(ctx, []model.Object{sampleHost("app")}, "re-import", author); err != nil {
		t.Fatalf("second batch: %v", err)
	}
	cfg, _, err = st.Load(ctx)
	if err != nil {
		t.Fatalf("load after second batch: %v", err)
	}
	if got := cfg.ProxyHosts[0].CreatedAt; !got.Equal(original) {
		t.Errorf("CreatedAt = %v, want the original %v", got, original)
	}
}

// TestApplyBatchPreservesCreatedAt covers the same rule through ApplyBatch,
// the reconciler's upsert path.
func TestApplyBatchPreservesCreatedAt(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}

	upserts := []model.Object{sampleHost("app")}
	if _, err := st.ApplyBatch(ctx, upserts, nil, "apply", author, nil); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	original := cfg.ProxyHosts[0].CreatedAt

	changed := sampleHost("app")
	changed.Disabled = true
	if _, err := st.ApplyBatch(ctx, []model.Object{changed}, nil, "apply again", author, nil); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	cfg, _, err = st.Load(ctx)
	if err != nil {
		t.Fatalf("load after second apply: %v", err)
	}
	if got := cfg.ProxyHosts[0].CreatedAt; !got.Equal(original) {
		t.Errorf("CreatedAt = %v, want the original %v", got, original)
	}
	if !cfg.ProxyHosts[0].Disabled {
		t.Error("expected the second apply's field change to still apply")
	}
}

// TestApplyBatchSkipsUpsertOnUnknownKeys is the rollback-guard regression test
// (docs/operations/upgrading.md#rollback): ApplyBatch's only two callers are
// the Ingress and Docker discovery reconcilers, both unattended. A file a
// newer gpm wrote (here: a proxy host with an unknown key appended, exactly
// what an older gpm's own writer would otherwise silently re-marshal away)
// must not be overwritten by an automatic upsert - the object is skipped, the
// batch commits nothing else, and the unknown key survives on disk untouched.
func TestApplyBatchSkipsUpsertOnUnknownKeys(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "ingress-discovery", Email: "gpm@localhost"}

	if _, err := st.Save(ctx, managedByFixture("app"), Author{Name: "admin", Email: "admin@example.com"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	path := filepath.Join(st.Dir(), "proxy-hosts", "app.yaml")
	appendYAMLLine(t, path, "futureField: true")

	changed := managedByFixture("app")
	changed.Disabled = true
	sha, err := st.ApplyBatch(ctx, []model.Object{changed}, nil, "reconcile", author, ownedByDiscovery)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if sha != "" {
		t.Fatalf("expected no commit (the only upsert was skipped), got %q", sha)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(raw), "futureField") {
		t.Fatal("the file's unknown key must survive an automatic writer's skipped upsert")
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProxyHosts[0].Disabled {
		t.Fatal("the skipped upsert's field change must not have been applied")
	}
}

// TestApplyBatchAllowsUpsertWhenNoUnknownKeys is the control case: a file
// with nothing unrecognised in it is upserted by ApplyBatch as normal.
func TestApplyBatchAllowsUpsertWhenNoUnknownKeys(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "ingress-discovery", Email: "gpm@localhost"}

	if _, err := st.Save(ctx, managedByFixture("app"), Author{Name: "admin", Email: "admin@example.com"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	changed := managedByFixture("app")
	changed.Disabled = true
	sha, err := st.ApplyBatch(ctx, []model.Object{changed}, nil, "reconcile", author, ownedByDiscovery)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if sha == "" {
		t.Fatal("expected a commit for a clean upsert")
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.ProxyHosts[0].Disabled {
		t.Fatal("expected the upsert's field change to apply when the file has no unknown keys")
	}
}
