package analytics

// Rollup is the hourly traffic aggregate store. The collector keeps the last
// 24 h in memory and, at each hour boundary, writes the closed hour's
// aggregates here; the API reads hours back to build 7 d and 30 d reports, and
// startup replays recent hours into memory.
//
// It is a local SQLite file, deliberately separate from the git-backed config
// store: config is reviewable, versioned operator intent, while this is
// high-churn runtime data that must never be committed to the config repo.
//
// Write contract: WriteHour replaces every row of one hour in a single
// transaction, so retrying the write of an hour after a crash is idempotent.
//
// Retention: the store never deletes on its own. The caller runs Prune on a
// schedule to drop hours older than its retention window.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Cardinality guards: a bug upstream must not be able to bloat the file.
const (
	rollupMaxHostLen = 253 // a proxy host name may be a full DNS name
	rollupMaxKeyLen  = 128
	rollupMaxRows    = 400000 // a sanity cap; the collector budgets well below it
	rollupSchemaVer  = "1"
)

// rollupDims is the closed vocabulary for RollupRow.Dim.
var rollupDims = map[string]bool{
	"total": true, "status": true, "country": true, "deny": true,
	"sig": true, "client": true, "dur": true,
}

// RollupRow is one aggregate for one hour, one proxy host and one dimension
// key. Dim is a short closed vocabulary ("total", "status", "country",
// "deny", "sig", "client", "dur"); Key is the value inside that dimension
// ("" for total, "2xx" for status, "US" for country, "bouncer" for deny, a
// signature label for sig, an address for client, a histogram upper bound in
// milliseconds as decimal text for dur). N, D and T are the three counters
// a dimension carries: requests, denied and threat hits (for dur: N is the
// count in that histogram bucket; for total: N requests, D denied, T threats).
// B is bytes out where it applies (total), else 0.
type RollupRow struct {
	Hour int64  // unix seconds, truncated to the hour, UTC
	Host string // proxy host NAME or "-" (requests that matched no host)
	Dim  string
	Key  string
	N    int64
	D    int64
	T    int64
	B    int64
}

// Rollup is the on-disk hourly store. Safe for concurrent use.
type Rollup struct {
	db *sql.DB
}

// OpenRollup opens or creates the store at path (schema is created
// idempotently) and returns it. The parent directory is created 0700 and the
// file 0600.
func OpenRollup(path string) (*Rollup, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("rollup: create db dir: %w", err)
		}
	}

	// Client addresses live here. Pre-create the file 0600 so SQLite (and its
	// -wal/-shm sidecars) inherit that mode, and tighten any existing file.
	f, err := os.OpenFile(path, os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("rollup: create db file: %w", err)
	}
	f.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("rollup: chmod db file: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("rollup: open db: %w", err)
	}

	// SQLite writes serialize anyway; a single connection avoids
	// "database is locked" errors under concurrent writers.
	db.SetMaxOpenConns(1)

	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("rollup: pragma %q: %w", p, err)
		}
	}

	const migration = `
CREATE TABLE IF NOT EXISTS rollup (
	hour INTEGER NOT NULL,
	host TEXT    NOT NULL,
	dim  TEXT    NOT NULL,
	key  TEXT    NOT NULL,
	n    INTEGER NOT NULL,
	d    INTEGER NOT NULL,
	t    INTEGER NOT NULL,
	b    INTEGER NOT NULL,
	PRIMARY KEY (hour, host, dim, key)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_rollup_hour ON rollup(hour);
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT);
INSERT OR IGNORE INTO meta (k, v) VALUES ('schema_version', '1');`
	if _, err := db.Exec(migration); err != nil {
		db.Close()
		return nil, fmt.Errorf("rollup: migrate: %w", err)
	}

	var ver string
	if err := db.QueryRow(`SELECT v FROM meta WHERE k = 'schema_version'`).Scan(&ver); err != nil {
		db.Close()
		return nil, fmt.Errorf("rollup: read schema version: %w", err)
	}
	if ver != rollupSchemaVer {
		db.Close()
		return nil, fmt.Errorf("rollup: unsupported schema version %q", ver)
	}

	return &Rollup{db: db}, nil
}

// validateRollupRows checks the write contract and cardinality guards for one hour.
func validateRollupRows(hour int64, rows []RollupRow) error {
	if hour%3600 != 0 {
		return fmt.Errorf("rollup: hour %d is not truncated to the hour", hour)
	}
	if len(rows) > rollupMaxRows {
		return fmt.Errorf("rollup: %d rows for hour %d exceeds limit %d", len(rows), hour, rollupMaxRows)
	}
	for i := range rows {
		r := &rows[i]
		switch {
		case r.Hour != hour:
			return fmt.Errorf("rollup: row %d hour %d does not match %d", i, r.Hour, hour)
		case r.Host == "":
			return fmt.Errorf("rollup: row %d has empty host", i)
		case len(r.Host) > rollupMaxHostLen:
			return fmt.Errorf("rollup: row %d host longer than %d bytes", i, rollupMaxHostLen)
		case !rollupDims[r.Dim]:
			return fmt.Errorf("rollup: row %d unknown dim %q", i, r.Dim)
		case len(r.Key) > rollupMaxKeyLen:
			return fmt.Errorf("rollup: row %d key longer than %d bytes", i, rollupMaxKeyLen)
		}
	}
	return nil
}

// WriteHour replaces every row for hour (unix seconds, truncated) with rows,
// in one transaction, so a retried write after a crash is idempotent. rows
// whose Hour differs from hour are rejected with an error before anything is
// written. An empty rows slice deletes the hour.
func (r *Rollup) WriteHour(hour int64, rows []RollupRow) (err error) {
	if err := validateRollupRows(hour, rows); err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("rollup: begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM rollup WHERE hour = ?`, hour); err != nil {
		return fmt.Errorf("rollup: delete hour: %w", err)
	}
	if len(rows) > 0 {
		var stmt *sql.Stmt
		stmt, err = tx.Prepare(`INSERT INTO rollup (hour, host, dim, key, n, d, t, b) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("rollup: prepare insert: %w", err)
		}
		defer stmt.Close()
		for i := range rows {
			row := &rows[i]
			if _, err = stmt.Exec(row.Hour, row.Host, row.Dim, row.Key, row.N, row.D, row.T, row.B); err != nil {
				return fmt.Errorf("rollup: insert row %d: %w", i, err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("rollup: commit: %w", err)
	}
	return nil
}

// ReadHours returns every row with from <= Hour < to, ordered by Hour, Host,
// Dim, Key. host "" means every host; otherwise only that host's rows.
func (r *Rollup) ReadHours(from, to int64, host string) ([]RollupRow, error) {
	const q = `SELECT hour, host, dim, key, n, d, t, b FROM rollup
		WHERE hour >= ? AND hour < ? AND (? = '' OR host = ?)
		ORDER BY hour, host, dim, key`
	rs, err := r.db.Query(q, from, to, host, host)
	if err != nil {
		return nil, fmt.Errorf("rollup: query: %w", err)
	}
	defer rs.Close()

	var out []RollupRow
	for rs.Next() {
		var row RollupRow
		if err := rs.Scan(&row.Hour, &row.Host, &row.Dim, &row.Key, &row.N, &row.D, &row.T, &row.B); err != nil {
			return nil, fmt.Errorf("rollup: scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rs.Err(); err != nil {
		return nil, fmt.Errorf("rollup: rows: %w", err)
	}
	return out, nil
}

// Hours returns the distinct hours stored, ascending.
func (r *Rollup) Hours() ([]int64, error) {
	rs, err := r.db.Query(`SELECT DISTINCT hour FROM rollup ORDER BY hour`)
	if err != nil {
		return nil, fmt.Errorf("rollup: query hours: %w", err)
	}
	defer rs.Close()

	var out []int64
	for rs.Next() {
		var h int64
		if err := rs.Scan(&h); err != nil {
			return nil, fmt.Errorf("rollup: scan hour: %w", err)
		}
		out = append(out, h)
	}
	if err := rs.Err(); err != nil {
		return nil, fmt.Errorf("rollup: hours: %w", err)
	}
	return out, nil
}

// Prune deletes every row with Hour < before and returns how many rows went.
func (r *Rollup) Prune(before int64) (int64, error) {
	res, err := r.db.Exec(`DELETE FROM rollup WHERE hour < ?`, before)
	if err != nil {
		return 0, fmt.Errorf("rollup: prune: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rollup: prune rows: %w", err)
	}
	return n, nil
}

// Stats reports row count and the oldest and newest hour (0,0 when empty).
func (r *Rollup) Stats() (rows int64, oldest, newest int64, err error) {
	err = r.db.QueryRow(`SELECT COUNT(*), COALESCE(MIN(hour), 0), COALESCE(MAX(hour), 0) FROM rollup`).
		Scan(&rows, &oldest, &newest)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("rollup: stats: %w", err)
	}
	return rows, oldest, newest, nil
}

// Close closes the database.
func (r *Rollup) Close() error {
	return r.db.Close()
}

// DeleteDim removes every row of one dimension (for example "client" when the
// operator turns client tracking off) and returns how many rows went.
func (r *Rollup) DeleteDim(dim string) (int64, error) {
	if !rollupDims[dim] {
		return 0, fmt.Errorf("rollup: unknown dim %q", dim)
	}
	res, err := r.db.Exec(`DELETE FROM rollup WHERE dim = ?`, dim)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
