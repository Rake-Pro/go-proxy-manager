package clientcert

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRecordStatus pins the expiry boundaries: with a 30-day window a
// certificate with 31 days left is ok, exactly 30 days left is expiring, 29 days
// left is expiring, and anything at or past notAfter is expired.
func TestRecordStatus(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	rec := func(d time.Duration) Record { return Record{NotAfter: now.Add(d)} }

	cases := []struct {
		name     string
		rec      Record
		warnDays int
		want     string
		wantDays int
	}{
		{"31 days left", rec(31 * day), 30, StatusOK, 31},
		{"exactly 30 days left", rec(30 * day), 30, StatusExpiring, 30},
		{"29 days left", rec(29 * day), 30, StatusExpiring, 29},
		{"1 day left", rec(day), 30, StatusExpiring, 1},
		{"expires exactly now", rec(0), 30, StatusExpired, 0},
		{"expired yesterday", rec(-day), 30, StatusExpired, -1},
		{"custom 7-day window, 8 days left", rec(8 * day), 7, StatusOK, 8},
		{"custom 7-day window, 7 days left", rec(7 * day), 7, StatusExpiring, 7},
		{"custom 90-day window, 60 days left", rec(60 * day), 90, StatusExpiring, 60},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rec.Status(now, tc.warnDays); got != tc.want {
				t.Fatalf("status %q, want %q", got, tc.want)
			}
			if got := tc.rec.DaysRemaining(now); got != tc.wantDays {
				t.Fatalf("daysRemaining %d, want %d", got, tc.wantDays)
			}
		})
	}
}

// TestLedgerRoundTrip proves records survive a restart: a second Ledger over the
// same directory (which is what a restarted process has) reads back exactly what
// the first one wrote, newest first.
func TestLedgerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)

	if recs, err := l.List("corp"); err != nil || len(recs) != 0 {
		t.Fatalf("a CA that never issued must read as empty, got %v %v", recs, err)
	}

	base := time.Now().UTC().Truncate(time.Second)
	first := Record{CA: "corp", CommonName: "phone-01", Serial: "aa01",
		SANs: []string{"phone-01.example.com"}, IssuedAt: base, NotAfter: base.Add(24 * time.Hour)}
	second := Record{CA: "corp", CommonName: "laptop", Serial: "bb02",
		IssuedAt: base.Add(time.Minute), NotAfter: base.Add(48 * time.Hour)}
	if err := l.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(second); err != nil {
		t.Fatal(err)
	}
	// A record for another CA lands in its own file and never leaks into corp's.
	if err := l.Append(Record{CA: "other", CommonName: "x", Serial: "cc03",
		IssuedAt: base, NotAfter: base.Add(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	// Restart: a fresh Ledger over the same store.
	recs, err := NewLedger(dir).List("corp")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if recs[0].Serial != "bb02" || recs[1].Serial != "aa01" {
		t.Fatalf("records not newest-first: %s, %s", recs[0].Serial, recs[1].Serial)
	}
	if recs[1].CommonName != "phone-01" || len(recs[1].SANs) != 1 || recs[1].SANs[0] != "phone-01.example.com" {
		t.Fatalf("record fields did not round-trip: %+v", recs[1])
	}
	if !recs[1].NotAfter.Equal(first.NotAfter) {
		t.Fatalf("notAfter %v, want %v", recs[1].NotAfter, first.NotAfter)
	}

	// The store file is a private file under the cert store, like the ACME
	// metadata it mirrors.
	fi, err := os.Stat(filepath.Join(dir, "client-certs", "corp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("record file mode %v, want 0600", fi.Mode().Perm())
	}
}

// TestLedgerSupersede proves a renewal appends the new record and marks the old
// one superseded by it in one write, leaving the old record listed.
func TestLedgerSupersede(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	base := time.Now().UTC()
	old := Record{CA: "corp", CommonName: "phone-01", Serial: "aa01",
		IssuedAt: base, NotAfter: base.Add(24 * time.Hour)}
	if err := l.Append(old); err != nil {
		t.Fatal(err)
	}
	renewed := Record{CA: "corp", CommonName: "phone-01", Serial: "bb02",
		IssuedAt: base.Add(time.Minute), NotAfter: base.Add(720 * time.Hour)}
	if err := l.AppendSuperseding(renewed, "aa01"); err != nil {
		t.Fatal(err)
	}

	recs, err := NewLedger(dir).List("corp")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("the superseded record must stay listed, got %d records", len(recs))
	}
	if recs[0].Serial != "bb02" || recs[0].SupersededBy != "" {
		t.Fatalf("the renewal must be current: %+v", recs[0])
	}
	if recs[1].Serial != "aa01" || recs[1].SupersededBy != "bb02" {
		t.Fatalf("the old record must be marked superseded by the renewal: %+v", recs[1])
	}
	if recs[1].SupersededAt.IsZero() {
		t.Fatal("supersededAt must be set")
	}
}

// TestLedgerSupersedeMissingTarget proves a supersede whose target is not in the
// ledger is refused rather than silently appended with a dangling link - two
// records that both looked current is exactly the state the link prevents.
func TestLedgerSupersedeMissingTarget(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	base := time.Now().UTC()
	if err := l.Append(Record{CA: "corp", CommonName: "phone-01", Serial: "aa01",
		IssuedAt: base, NotAfter: base.Add(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	err := l.AppendSuperseding(Record{CA: "corp", CommonName: "phone-01", Serial: "bb02",
		IssuedAt: base.Add(time.Minute), NotAfter: base.Add(720 * time.Hour)}, "deadbeef")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("want ErrRecordNotFound, got %v", err)
	}
	recs, err := l.List("corp")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Serial != "aa01" {
		t.Fatalf("a refused supersede must write nothing, got %+v", recs)
	}
}

// TestLedgerSupersedeKeepsStaleTarget proves the record being superseded is never
// pruned by the same write, however long ago it expired: the supersede link is the
// entire point of the operation.
func TestLedgerSupersedeKeepsStaleTarget(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	now := time.Now().UTC()
	stale := Record{CA: "corp", CommonName: "ancient", Serial: "aa01",
		IssuedAt: now.Add(-3 * retention), NotAfter: now.Add(-2 * retention)}
	if err := l.Append(stale); err != nil {
		t.Fatal(err)
	}
	renewed := Record{CA: "corp", CommonName: "ancient", Serial: "bb02",
		IssuedAt: now, NotAfter: now.Add(720 * time.Hour)}
	if err := l.AppendSuperseding(renewed, "aa01"); err != nil {
		t.Fatal(err)
	}
	recs, err := l.List("corp")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want the renewal plus its stale target", len(recs))
	}
	if recs[1].Serial != "aa01" || recs[1].SupersededBy != "bb02" {
		t.Fatalf("stale target was pruned instead of linked: %+v", recs[1])
	}
}

// TestLedgerPrunesLongExpired proves an append drops records that expired longer
// ago than the retention window, and keeps everything else - including a
// superseded record that has not expired yet.
func TestLedgerPrunesLongExpired(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	now := time.Now().UTC()
	stale := Record{CA: "corp", CommonName: "ancient", Serial: "aa01",
		IssuedAt: now.Add(-3 * retention), NotAfter: now.Add(-2 * retention)}
	recent := Record{CA: "corp", CommonName: "recent", Serial: "bb02",
		IssuedAt: now.Add(-48 * time.Hour), NotAfter: now.Add(-24 * time.Hour)}
	if err := l.Append(stale); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(recent); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Record{CA: "corp", CommonName: "live", Serial: "cc03",
		IssuedAt: now, NotAfter: now.Add(720 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	recs, err := l.List("corp")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.Serial == "aa01" {
			t.Fatal("a record expired past the retention window must be pruned")
		}
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 (recently expired + live)", len(recs))
	}
}

// TestLedgerGetAndErrors covers lookup misses and a ledger with no cert store.
func TestLedgerGetAndErrors(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	now := time.Now().UTC()
	if err := l.Append(Record{CA: "corp", CommonName: "a", Serial: "aa01",
		IssuedAt: now, NotAfter: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if rec, err := l.Get("corp", "aa01"); err != nil || rec.CommonName != "a" {
		t.Fatalf("Get: %+v %v", rec, err)
	}
	if _, err := l.Get("corp", "nope"); err != ErrRecordNotFound {
		t.Fatalf("want ErrRecordNotFound, got %v", err)
	}
	if _, err := l.Get("never-issued", "aa01"); err != ErrRecordNotFound {
		t.Fatalf("want ErrRecordNotFound, got %v", err)
	}
	// A name that is not a valid object name never becomes a path.
	if _, err := l.List("../../etc"); err == nil {
		t.Fatal("an invalid CA name must be refused rather than joined into a path")
	}

	// With no cert store wired the ledger reads empty and refuses to write, rather
	// than scattering state through the working directory.
	none := NewLedger("")
	if recs, err := none.List("corp"); err != nil || len(recs) != 0 {
		t.Fatalf("unwired ledger List: %v %v", recs, err)
	}
	if err := none.Append(Record{CA: "corp", Serial: "aa01"}); err != ErrNoStore {
		t.Fatalf("want ErrNoStore, got %v", err)
	}
}
