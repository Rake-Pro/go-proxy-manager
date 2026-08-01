package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// A deployment that has never reconciled has no ledger file. That must read as an
// EMPTY ledger, never as an error and never as "everything is unowned, delete it".
func TestDNSLedgerMissingFileIsEmpty(t *testing.T) {
	st := newTestStore(t)

	l, _, err := st.LoadDNSLedger(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(l.Pihole) != 0 || len(l.Cloudflare) != 0 {
		t.Fatalf("ledger = %+v, want empty", l)
	}
}

func TestDNSLedgerRoundTripAndCommit(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	l := model.DNSLedger{
		Pihole: []model.DNSLedgerEntry{
			{Domain: "app.example.com", Target: "edge.example.com"},
			{Domain: "alt.example.com", Target: "edge.example.com"},
		},
		Cloudflare: []model.DNSLedgerEntry{{Domain: "www.example.com", Target: "edge.example.com"}},
	}
	commit, err := st.SaveDNSLedger(ctx, l, Author{Name: "dns-sync", Email: "gpm@localhost"}, "")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if commit == "" {
		t.Fatal("saving a ledger must commit")
	}

	got, _, err := st.LoadDNSLedger(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Pihole) != 2 || got.Pihole[0].Domain != "app.example.com" || got.Cloudflare[0].Target != "edge.example.com" {
		t.Fatalf("ledger = %+v", got)
	}
	if got.SchemaVersion != model.SchemaVersion {
		t.Fatalf("schemaVersion = %d", got.SchemaVersion)
	}

	// It is a plain file in the config repo, so it is diffable and auditable like
	// every other object - and it appears in the repo history.
	if _, err := os.Stat(filepath.Join(st.Dir(), "dns-ledger.yaml")); err != nil {
		t.Fatalf("ledger file: %v", err)
	}
	hist, err := st.RepoHistory(ctx, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var found bool
	for _, c := range hist {
		if strings.Contains(c.Message, "DNS sync ledger") {
			found = true
			if c.Author != "dns-sync" {
				t.Fatalf("ledger commit authored by %q, want the reconciler", c.Author)
			}
		}
	}
	if !found {
		t.Fatalf("ledger commit missing from history: %+v", hist)
	}

	// Saving an identical ledger changes nothing, so it must not commit again.
	again, err := st.SaveDNSLedger(ctx, l, Author{Name: "dns-sync", Email: "gpm@localhost"}, "")
	if err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if again != "" {
		t.Fatalf("an unchanged ledger must not produce a commit, got %q", again)
	}
}

// The ledger lives in the config repo precisely so it reverts with everything
// else: rolling the config back to before a host existed must also roll back
// gpm's claim on the record that host published, or the next reconcile would
// delete a record it no longer has any business owning.
func TestDNSLedgerRevertsWithTheTree(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "dns-sync", Email: "gpm@localhost"}

	before, err := st.SaveDNSLedger(ctx, model.DNSLedger{
		Pihole: []model.DNSLedgerEntry{{Domain: "app.example.com", Target: "edge.example.com"}},
	}, author, "")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := st.SaveDNSLedger(ctx, model.DNSLedger{
		Pihole: []model.DNSLedgerEntry{
			{Domain: "app.example.com", Target: "edge.example.com"},
			{Domain: "later.example.com", Target: "edge.example.com"},
		},
	}, author, ""); err != nil {
		t.Fatalf("second save: %v", err)
	}

	if _, err := st.Revert(ctx, before, Author{Name: "admin", Email: "admin@example.com"}); err != nil {
		t.Fatalf("revert: %v", err)
	}
	got, _, err := st.LoadDNSLedger(ctx)
	if err != nil {
		t.Fatalf("load after revert: %v", err)
	}
	if len(got.Pihole) != 1 || got.Pihole[0].Domain != "app.example.com" {
		t.Fatalf("ledger after revert = %+v, want the earlier state", got)
	}
}

// A reconcile reads the ledger, spends minutes talking to DNS backends, then
// writes it back. A Revert can rewrite the very same file in between, and a blind
// write would silently re-establish the ownership claims the revert withdrew -
// claims that authorise DELETING records. The write is refused instead, and the
// revert's state stands.
func TestDNSLedgerSaveRefusesAStaleWrite(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "dns-sync", Email: "gpm@localhost"}

	if _, err := st.SaveDNSLedger(ctx, model.DNSLedger{
		Pihole: []model.DNSLedgerEntry{{Domain: "app.example.com", Target: "edge.example.com"}},
	}, author, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A reconcile reads the ledger here...
	_, rev, err := st.LoadDNSLedger(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rev == "" {
		t.Fatal("a load must report the revision it read at, or a stale write cannot be detected")
	}

	// ...and another writer moves the config repo on while it is off talking to
	// Pi-hole (a revert, in the case that matters).
	if _, err := st.Save(ctx, sampleHost("concurrent"), Author{Name: "admin", Email: "admin@example.com"}); err != nil {
		t.Fatalf("concurrent write: %v", err)
	}

	_, err = st.SaveDNSLedger(ctx, model.DNSLedger{
		Pihole: []model.DNSLedgerEntry{
			{Domain: "app.example.com", Target: "edge.example.com"},
			{Domain: "resurrected.example.com", Target: "edge.example.com"},
		},
	}, author, rev)
	if !errors.Is(err, ErrLedgerStale) {
		t.Fatalf("REGRESSION: a stale ledger write returned %v, want ErrLedgerStale", err)
	}
	got, newRev, err := st.LoadDNSLedger(ctx)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(got.Pihole) != 1 {
		t.Fatalf("REGRESSION: the refused write landed anyway: %+v", got)
	}
	// And the write succeeds once it is based on what is actually there.
	if _, err := st.SaveDNSLedger(ctx, got, author, newRev); err != nil {
		t.Fatalf("re-based save: %v", err)
	}
}

// A ledger file written before provenance existed carries no `adopted` key. It
// must load as ADOPTED - the only reading that cannot let an upgrade delete an
// operator's record.
func TestDNSLedgerWithoutProvenanceLoadsAsAdopted(t *testing.T) {
	st := newTestStore(t)

	legacy := "schemaVersion: 1\npihole:\n  - domain: app.example.com\n    target: edge.example.com\n"
	if err := os.WriteFile(filepath.Join(st.Dir(), "dns-ledger.yaml"), []byte(legacy), 0o640); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, _, err := st.LoadDNSLedger(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Pihole) != 1 || !got.Pihole[0].IsAdopted() {
		t.Fatalf("REGRESSION: a legacy entry loaded as %+v, want it treated as adopted (never auto-deleted)", got.Pihole)
	}
}

// A hand-edited or corrupted ledger is surfaced rather than acted on: an entry
// with no target cannot say whether the record is still the one gpm wrote.
func TestDNSLedgerRejectsInvalid(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	bad := model.DNSLedger{Pihole: []model.DNSLedgerEntry{{Domain: "app.example.com"}}}
	if _, err := st.SaveDNSLedger(ctx, bad, Author{}, ""); err == nil {
		t.Fatal("a ledger entry with no target must be rejected")
	}
	dup := model.DNSLedger{Pihole: []model.DNSLedgerEntry{
		{Domain: "app.example.com", Target: "a"},
		{Domain: "app.example.com", Target: "b"},
	}}
	if _, err := st.SaveDNSLedger(ctx, dup, Author{}, ""); err == nil {
		t.Fatal("a duplicated domain must be rejected")
	}

	if err := os.WriteFile(filepath.Join(st.Dir(), "dns-ledger.yaml"), []byte("pihole:\n  - domain: x\n"), 0o640); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := st.LoadDNSLedger(ctx); err == nil {
		t.Fatal("loading an invalid ledger must fail rather than be acted on")
	}
}
