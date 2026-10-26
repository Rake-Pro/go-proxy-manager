package analytics

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const rollupH0 int64 = 1_790_000_000 / 3600 * 3600

func openTestRollup(t testing.TB) (*Rollup, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "analytics.db")
	r, err := OpenRollup(path)
	if err != nil {
		t.Fatalf("OpenRollup: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r, path
}

func mustWrite(t *testing.T, r *Rollup, hour int64, rows []RollupRow) {
	t.Helper()
	if err := r.WriteHour(hour, rows); err != nil {
		t.Fatalf("WriteHour(%d): %v", hour, err)
	}
}

func mustRead(t *testing.T, r *Rollup, from, to int64, host string) []RollupRow {
	t.Helper()
	rows, err := r.ReadHours(from, to, host, "")
	if err != nil {
		t.Fatalf("ReadHours: %v", err)
	}
	return rows
}

func TestRollupOpenModes(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("POSIX file modes not available")
	}
	r, path := openTestRollup(t)
	mustWrite(t, r, rollupH0, []RollupRow{{Hour: rollupH0, Host: "www", Dim: "total", N: 1}})

	checks := []struct {
		path string
		want os.FileMode
		opt  bool
	}{
		{filepath.Dir(path), 0o700, false},
		{path, 0o600, false},
		{path + "-wal", 0o600, true},
		{path + "-shm", 0o600, true},
	}
	for _, c := range checks {
		fi, err := os.Stat(c.path)
		if err != nil {
			if c.opt && os.IsNotExist(err) {
				continue
			}
			t.Fatalf("stat %s: %v", c.path, err)
		}
		if got := fi.Mode().Perm(); got != c.want {
			t.Errorf("%s mode = %o, want %o", c.path, got, c.want)
		}
	}
}

func TestRollupReopenIdempotent(t *testing.T) {
	r, path := openTestRollup(t)
	mustWrite(t, r, rollupH0, []RollupRow{{Hour: rollupH0, Host: "www", Dim: "total", N: 5}})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r2, err := OpenRollup(path)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		got, err := r2.ReadHours(rollupH0, rollupH0+3600, "", "")
		r2.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].N != 5 {
			t.Fatalf("reopen %d: rows = %+v", i, got)
		}
	}
}

func TestRollupRoundTripOrdering(t *testing.T) {
	r, _ := openTestRollup(t)
	h1 := rollupH0 + 3600
	// Written out of order on purpose.
	mustWrite(t, r, h1, []RollupRow{
		{Hour: h1, Host: "www", Dim: "status", Key: "2xx", N: 9},
		{Hour: h1, Host: "-", Dim: "total", N: 1, D: 1},
	})
	mustWrite(t, r, rollupH0, []RollupRow{
		{Hour: rollupH0, Host: "www", Dim: "total", N: 10, D: 2, T: 1, B: 4096},
		{Hour: rollupH0, Host: "api", Dim: "dur", Key: "250", N: 3},
		{Hour: rollupH0, Host: "api", Dim: "country", Key: "US", N: 4, D: 1},
		{Hour: rollupH0, Host: "api", Dim: "country", Key: "DE", N: 2},
	})
	want := []RollupRow{
		{Hour: rollupH0, Host: "api", Dim: "country", Key: "DE", N: 2},
		{Hour: rollupH0, Host: "api", Dim: "country", Key: "US", N: 4, D: 1},
		{Hour: rollupH0, Host: "api", Dim: "dur", Key: "250", N: 3},
		{Hour: rollupH0, Host: "www", Dim: "total", N: 10, D: 2, T: 1, B: 4096},
		{Hour: h1, Host: "-", Dim: "total", N: 1, D: 1},
		{Hour: h1, Host: "www", Dim: "status", Key: "2xx", N: 9},
	}
	if got := mustRead(t, r, rollupH0, h1+3600, ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadHours =\n%+v\nwant\n%+v", got, want)
	}
	// to is exclusive.
	if got := mustRead(t, r, rollupH0, h1, ""); len(got) != 4 {
		t.Fatalf("exclusive upper bound: got %d rows, want 4", len(got))
	}
}

func TestRollupWriteHourReplaces(t *testing.T) {
	r, _ := openTestRollup(t)
	mustWrite(t, r, rollupH0, []RollupRow{
		{Hour: rollupH0, Host: "www", Dim: "total", N: 1},
		{Hour: rollupH0, Host: "api", Dim: "total", N: 2},
		{Hour: rollupH0, Host: "git", Dim: "total", N: 3},
	})
	mustWrite(t, r, rollupH0, []RollupRow{{Hour: rollupH0, Host: "www", Dim: "total", N: 7}})
	got := mustRead(t, r, rollupH0, rollupH0+3600, "")
	if len(got) != 1 || got[0].Host != "www" || got[0].N != 7 {
		t.Fatalf("after replace: %+v", got)
	}
}

func TestRollupEmptyRowsDeletesHour(t *testing.T) {
	r, _ := openTestRollup(t)
	h1 := rollupH0 + 3600
	mustWrite(t, r, rollupH0, []RollupRow{{Hour: rollupH0, Host: "www", Dim: "total", N: 1}})
	mustWrite(t, r, h1, []RollupRow{{Hour: h1, Host: "www", Dim: "total", N: 2}})
	mustWrite(t, r, rollupH0, nil)
	hours, err := r.Hours()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hours, []int64{h1}) {
		t.Fatalf("Hours = %v, want [%d]", hours, h1)
	}
}

func TestRollupHostFilter(t *testing.T) {
	r, _ := openTestRollup(t)
	mustWrite(t, r, rollupH0, []RollupRow{
		{Hour: rollupH0, Host: "www", Dim: "total", N: 1},
		{Hour: rollupH0, Host: "api", Dim: "total", N: 2},
		{Hour: rollupH0, Host: "api", Dim: "status", Key: "5xx", N: 1},
		{Hour: rollupH0, Host: "-", Dim: "total", N: 3},
	})
	cases := []struct {
		host string
		want int
	}{
		{"", 4}, {"api", 2}, {"www", 1}, {"-", 1}, {"nope", 0},
	}
	for _, c := range cases {
		got := mustRead(t, r, rollupH0, rollupH0+3600, c.host)
		if len(got) != c.want {
			t.Errorf("host %q: %d rows, want %d", c.host, len(got), c.want)
		}
		for _, row := range got {
			if c.host != "" && row.Host != c.host {
				t.Errorf("host %q: got row for %q", c.host, row.Host)
			}
		}
	}
}

func TestRollupHoursAscendingDistinct(t *testing.T) {
	r, _ := openTestRollup(t)
	for _, k := range []int64{5, 1, 3} {
		h := rollupH0 + k*3600
		mustWrite(t, r, h, []RollupRow{
			{Hour: h, Host: "www", Dim: "total", N: 1},
			{Hour: h, Host: "api", Dim: "total", N: 1},
		})
	}
	hours, err := r.Hours()
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{rollupH0 + 3600, rollupH0 + 3*3600, rollupH0 + 5*3600}
	if !reflect.DeepEqual(hours, want) {
		t.Fatalf("Hours = %v, want %v", hours, want)
	}
}

func TestRollupPrune(t *testing.T) {
	r, _ := openTestRollup(t)
	for k := int64(0); k < 4; k++ {
		h := rollupH0 + k*3600
		mustWrite(t, r, h, []RollupRow{
			{Hour: h, Host: "www", Dim: "total", N: k + 1},
			{Hour: h, Host: "api", Dim: "total", N: k + 10},
		})
	}
	before := mustRead(t, r, rollupH0+2*3600, rollupH0+4*3600, "")
	n, err := r.Prune(rollupH0 + 2*3600)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("Prune removed %d rows, want 4", n)
	}
	after := mustRead(t, r, 0, rollupH0+100*3600, "")
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("remaining rows changed:\n%+v\nwant\n%+v", after, before)
	}
	if n, err := r.Prune(rollupH0); err != nil || n != 0 {
		t.Fatalf("second Prune = %d, %v; want 0, nil", n, err)
	}
}

func TestRollupStats(t *testing.T) {
	r, _ := openTestRollup(t)
	rows, oldest, newest, err := r.Stats()
	if err != nil || rows != 0 || oldest != 0 || newest != 0 {
		t.Fatalf("empty Stats = %d %d %d %v", rows, oldest, newest, err)
	}
	h2 := rollupH0 + 2*3600
	mustWrite(t, r, h2, []RollupRow{{Hour: h2, Host: "www", Dim: "total"}})
	mustWrite(t, r, rollupH0, []RollupRow{
		{Hour: rollupH0, Host: "www", Dim: "total"},
		{Hour: rollupH0, Host: "api", Dim: "total"},
	})
	rows, oldest, newest, err = r.Stats()
	if err != nil || rows != 3 || oldest != rollupH0 || newest != h2 {
		t.Fatalf("Stats = %d %d %d %v; want 3 %d %d", rows, oldest, newest, err, rollupH0, h2)
	}
}

func TestRollupWriteHourRejects(t *testing.T) {
	ok := RollupRow{Hour: rollupH0, Host: "www", Dim: "total", N: 1}
	with := func(f func(*RollupRow)) []RollupRow {
		bad := ok
		f(&bad)
		// The bad row comes after a good one: nothing at all may be written.
		return []RollupRow{ok, bad}
	}
	tooMany := make([]RollupRow, rollupMaxRows+1)
	for i := range tooMany {
		tooMany[i] = RollupRow{Hour: rollupH0, Host: "www", Dim: "client", Key: strconv.Itoa(i)}
	}
	cases := []struct {
		name string
		hour int64
		rows []RollupRow
	}{
		{"mismatched hour", rollupH0, with(func(r *RollupRow) { r.Hour = rollupH0 + 3600 })},
		{"hour not truncated", rollupH0 + 1, []RollupRow{{Hour: rollupH0 + 1, Host: "www", Dim: "total"}}},
		{"empty host", rollupH0, with(func(r *RollupRow) { r.Host = "" })},
		{"long host", rollupH0, with(func(r *RollupRow) { r.Host = strings.Repeat("h", rollupMaxHostLen+1) })},
		{"unknown dim", rollupH0, with(func(r *RollupRow) { r.Dim = "path" })},
		{"empty dim", rollupH0, with(func(r *RollupRow) { r.Dim = "" })},
		{"long key", rollupH0, with(func(r *RollupRow) { r.Dim, r.Key = "sig", strings.Repeat("k", rollupMaxKeyLen+1) })},
		{"too many rows", rollupH0, tooMany},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := openTestRollup(t)
			if err := r.WriteHour(c.hour, c.rows); err == nil {
				t.Fatal("WriteHour accepted invalid input")
			}
			if rows, _, _, err := r.Stats(); err != nil || rows != 0 {
				t.Fatalf("rows written after rejection: %d, %v", rows, err)
			}
		})
	}

	// A rejected rewrite must leave the existing hour intact.
	r, _ := openTestRollup(t)
	mustWrite(t, r, rollupH0, []RollupRow{ok})
	if err := r.WriteHour(rollupH0, with(func(r *RollupRow) { r.Dim = "bogus" })); err == nil {
		t.Fatal("WriteHour accepted invalid dim")
	}
	if got := mustRead(t, r, rollupH0, rollupH0+3600, ""); !reflect.DeepEqual(got, []RollupRow{ok}) {
		t.Fatalf("existing hour changed: %+v", got)
	}

	// The limits themselves are accepted.
	edge := RollupRow{Hour: rollupH0, Host: strings.Repeat("h", rollupMaxHostLen), Dim: "sig", Key: strings.Repeat("k", rollupMaxKeyLen)}
	mustWrite(t, r, rollupH0, []RollupRow{edge})
}

func TestRollupConcurrent(t *testing.T) {
	r, _ := openTestRollup(t)
	const writers = 8
	const rounds = 20
	errs := make(chan error, writers*rounds+rounds)
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			h := rollupH0 + int64(w)*3600
			for i := 0; i < rounds; i++ {
				rows := []RollupRow{
					{Hour: h, Host: "www", Dim: "total", N: int64(i)},
					{Hour: h, Host: "www", Dim: "status", Key: "2xx", N: int64(i)},
				}
				if err := r.WriteHour(h, rows); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if _, err := r.ReadHours(rollupH0, rollupH0+writers*3600, "", ""); err != nil {
				errs <- err
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	rows, _, _, err := r.Stats()
	if err != nil || rows != writers*2 {
		t.Fatalf("Stats rows = %d, %v; want %d", rows, err, writers*2)
	}
}

func BenchmarkRollupWriteHour5000(b *testing.B) {
	r, _ := openTestRollup(b)
	rows := make([]RollupRow, 5000)
	for i := range rows {
		rows[i] = RollupRow{Hour: rollupH0, Host: "www", Dim: "client", Key: "10.0." + strconv.Itoa(i/256) + "." + strconv.Itoa(i%256), N: int64(i)}
	}
	for b.Loop() {
		if err := r.WriteHour(rollupH0, rows); err != nil {
			b.Fatal(err)
		}
	}
}

// A schema 1 file (no src column) is rebuilt in place: every row comes back
// under src "", counted by an unfiltered read and left out of a filtered one.
func TestRollupUpgradesSchema1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	const v1 = `
CREATE TABLE rollup (hour INTEGER NOT NULL, host TEXT NOT NULL, dim TEXT NOT NULL, key TEXT NOT NULL,
	n INTEGER NOT NULL, d INTEGER NOT NULL, t INTEGER NOT NULL, b INTEGER NOT NULL,
	PRIMARY KEY (hour, host, dim, key)) WITHOUT ROWID;
CREATE INDEX idx_rollup_hour ON rollup(hour);
CREATE TABLE meta (k TEXT PRIMARY KEY, v TEXT);
INSERT INTO meta (k, v) VALUES ('schema_version', '1');`
	if _, err := db.Exec(v1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rollup VALUES (?, 'www', 'total', '', 5, 1, 0, 100), (?, 'www', 'country', 'US', 5, 1, 0, 0)`, rollupH0, rollupH0); err != nil {
		t.Fatal(err)
	}
	db.Close()

	r, err := OpenRollup(path)
	if err != nil {
		t.Fatalf("OpenRollup on a schema 1 file: %v", err)
	}
	defer r.Close()
	var ver string
	if err := r.db.QueryRow(`SELECT v FROM meta WHERE k = 'schema_version'`).Scan(&ver); err != nil || ver != "2" {
		t.Fatalf("schema version after upgrade = %q, %v", ver, err)
	}
	got := mustRead(t, r, rollupH0, rollupH0+3600, "")
	if len(got) != 2 || got[0].Src != "" || got[1].Src != "" || got[1].N != 5 {
		t.Fatalf("migrated rows = %+v", got)
	}
	if rows, err := r.ReadHours(rollupH0, rollupH0+3600, "", SourceExternal); err != nil || len(rows) != 0 {
		t.Fatalf("filtered read of legacy rows = %d rows, %v", len(rows), err)
	}
	// New rows coexist with the legacy ones under their own source.
	mustWrite(t, r, rollupH0+3600, []RollupRow{
		{Hour: rollupH0 + 3600, Host: "www", Src: SourceExternal, Dim: "total", N: 1},
		{Hour: rollupH0 + 3600, Host: "www", Src: SourceInternal, Dim: "total", N: 2},
	})
	if rows, _ := r.ReadHours(rollupH0, rollupH0+7200, "", SourceInternal); len(rows) != 1 || rows[0].N != 2 {
		t.Fatalf("internal rows = %+v", rows)
	}
	// Reopening an upgraded file is a no-op.
	r2, err := OpenRollup(path)
	if err != nil {
		t.Fatal(err)
	}
	r2.Close()
}

// A schema 1 table whose meta row is missing (or wrong) is still recognised
// by its columns and upgraded rather than read with the schema 2 queries.
func TestRollupUpgradesSchema1WithoutMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	const v1 = `
CREATE TABLE rollup (hour INTEGER NOT NULL, host TEXT NOT NULL, dim TEXT NOT NULL, key TEXT NOT NULL,
	n INTEGER NOT NULL, d INTEGER NOT NULL, t INTEGER NOT NULL, b INTEGER NOT NULL,
	PRIMARY KEY (hour, host, dim, key)) WITHOUT ROWID;`
	if _, err := db.Exec(v1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rollup VALUES (?, 'www', 'total', '', 3, 0, 0, 0)`, rollupH0); err != nil {
		t.Fatal(err)
	}
	db.Close()
	r, err := OpenRollup(path)
	if err != nil {
		t.Fatalf("OpenRollup: %v", err)
	}
	defer r.Close()
	if got := mustRead(t, r, rollupH0, rollupH0+3600, ""); len(got) != 1 || got[0].N != 3 || got[0].Src != "" {
		t.Fatalf("rows = %+v", got)
	}
}

func TestRollupRejectsUnknownSrc(t *testing.T) {
	r, _ := openTestRollup(t)
	err := r.WriteHour(rollupH0, []RollupRow{{Hour: rollupH0, Host: "www", Src: "lan", Dim: "total", N: 1}})
	if err == nil || !strings.Contains(err.Error(), "unknown src") {
		t.Fatalf("err = %v", err)
	}
}
