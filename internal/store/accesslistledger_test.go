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

func accessLedgerEntry(list, source string, entries ...string) model.AccessListSourceEntry {
	return model.AccessListSourceEntry{
		List:      list,
		Source:    source,
		URL:       "https://example.com/" + source + ".txt",
		FetchedAt: "2026-09-01T00:00:00Z",
		SHA256:    model.AccessListSourceHash(model.AccessListSourceKey(list, source), "https://example.com/"+source+".txt", entries),
		Entries:   entries,
	}
}

// A deployment that has never fetched a source has no ledger file. That reads as
// an EMPTY ledger: every source rule then resolves to the empty set and matches
// nothing, which denies rather than opens.
func TestAccessListSourceLedgerMissingFileIsEmpty(t *testing.T) {
	st := newTestStore(t)

	l, _, err := st.LoadAccessListSourceLedger(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(l.Sources) != 0 {
		t.Fatalf("ledger = %+v, want empty", l)
	}
}

func TestAccessListSourceLedgerRoundTripAndCommit(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "access-list-sync", Email: "gpm@localhost"}

	l := model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{
		accessLedgerEntry("home-vpn", "uptimerobot", "1.2.3.0/24", "2001:db8::/32"),
	}}
	commit, err := st.SaveAccessListSourceLedger(ctx, l, author, "")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if commit == "" {
		t.Fatal("saving a ledger must commit")
	}

	got, _, err := st.LoadAccessListSourceLedger(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Sources) != 1 || got.Sources[0].Key() != "home-vpn/uptimerobot" ||
		len(got.Sources[0].Entries) != 2 {
		t.Fatalf("ledger = %+v", got)
	}
	if got.SchemaVersion != model.SchemaVersion {
		t.Fatalf("schemaVersion = %d", got.SchemaVersion)
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "access-list-sources.yaml")); err != nil {
		t.Fatalf("ledger file: %v", err)
	}

	hist, err := st.RepoHistory(ctx, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var found bool
	for _, c := range hist {
		if strings.Contains(c.Message, "Access-list source ledger") {
			found = true
			if c.Author != "access-list-sync" {
				t.Fatalf("ledger commit authored by %q, want the fetcher", c.Author)
			}
		}
	}
	if !found {
		t.Fatalf("ledger commit missing from history: %+v", hist)
	}

	// The whole point of not writing fetchedAt on a no-op: an unchanged ledger
	// must not add a commit every interval.
	again, err := st.SaveAccessListSourceLedger(ctx, l, author, "")
	if err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if again != "" {
		t.Fatalf("an unchanged ledger must not produce a commit, got %q", again)
	}
}

// A fetch is a read-modify-write spanning network I/O that a Revert can rewrite
// in between. A blind write would re-establish a set the revert withdrew - and
// these sets decide who reaches a host - so it is refused instead.
func TestAccessListSourceLedgerSaveRefusesAStaleWrite(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	author := Author{Name: "access-list-sync", Email: "gpm@localhost"}

	if _, err := st.SaveAccessListSourceLedger(ctx,
		model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{
			accessLedgerEntry("l", "s", "1.2.3.0/24"),
		}}, author, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, stale, err := st.LoadAccessListSourceLedger(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// Something else writes in the meantime, moving HEAD.
	if _, err := st.SaveAccessListSourceLedger(ctx,
		model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{
			accessLedgerEntry("l", "s", "9.9.9.0/24"),
		}}, author, ""); err != nil {
		t.Fatalf("concurrent save: %v", err)
	}

	_, err = st.SaveAccessListSourceLedger(ctx,
		model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{
			accessLedgerEntry("l", "s", "1.2.3.0/24"),
		}}, author, stale)
	if !errors.Is(err, ErrAccessListLedgerStale) {
		t.Fatalf("err = %v, want ErrAccessListLedgerStale", err)
	}
}

// A hand-edited ledger is surfaced at load, not served: pasting a network in
// without recomputing the hash is exactly the edit that would silently widen an
// allow list.
func TestAccessListSourceLedgerRejectsATamperedFile(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.SaveAccessListSourceLedger(ctx,
		model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{
			accessLedgerEntry("l", "s", "1.2.3.0/24"),
		}}, Author{Name: "access-list-sync", Email: "gpm@localhost"}, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}
	p := filepath.Join(st.Dir(), "access-list-sources.yaml")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	tampered := strings.Replace(string(b), "1.2.3.0/24", "0.0.0.0/0", 1)
	if tampered == string(b) {
		t.Fatalf("test could not find the entry to tamper with:\n%s", b)
	}
	if err := os.WriteFile(p, []byte(tampered), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err = st.LoadAccessListSourceLedger(ctx)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v, want the recorded hash to catch the edit", err)
	}
}
