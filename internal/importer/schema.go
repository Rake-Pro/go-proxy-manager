package importer

import (
	"database/sql"
	"fmt"
	"strings"
)

// tableExists reports whether a table is present in sqlite_master.
func (s *importState) tableExists(table string) (bool, error) {
	row := s.db.QueryRowContext(s.ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table)
	var n int
	if err := row.Scan(&n); err != nil {
		return false, fmt.Errorf("probe table %q: %w", table, err)
	}
	return n > 0, nil
}

// columnsOf returns the set of column names of a table via PRAGMA table_info.
func (s *importState) columnsOf(table string) (map[string]bool, error) {
	// PRAGMA does not accept bind parameters; the table name comes from a fixed
	// internal allowlist of NPM tables, never user input.
	rows, err := s.db.QueryContext(s.ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("table_info %q: %w", table, err)
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     sql.NullString
			notnull   sql.NullInt64
			dfltValue sql.NullString
			pk        sql.NullInt64
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info %q: %w", table, err)
		}
		cols[name] = true
	}
	return cols, rows.Err()
}

// selectAvailable builds a SELECT that pulls only the columns from `want` that
// actually exist on the table, ordered by id when present. Missing columns are
// returned in the second value so callers can warn. Returns ok=false if the
// table itself is absent or has none of the wanted columns.
func (s *importState) selectAvailable(table string, want []string) (cols []string, missing []string, ok bool, err error) {
	exists, err := s.tableExists(table)
	if err != nil {
		return nil, nil, false, err
	}
	if !exists {
		return nil, nil, false, nil
	}
	have, err := s.columnsOf(table)
	if err != nil {
		return nil, nil, false, err
	}
	for _, c := range want {
		if have[c] {
			cols = append(cols, c)
		} else {
			missing = append(missing, c)
		}
	}
	if len(cols) == 0 {
		return nil, missing, false, nil
	}
	return cols, missing, true, nil
}

// queryRows runs SELECT <cols> FROM <table> and returns each row as a
// column->value map. Soft-deleted rows (is_deleted=1) are filtered out when that
// column exists. Values are kept as the driver's native types (int64, string,
// []byte, nil) and read via the row helpers below.
func (s *importState) queryRows(table string, cols []string) ([]map[string]any, error) {
	have, err := s.columnsOf(table)
	if err != nil {
		return nil, err
	}
	sel := make([]string, len(cols))
	for i, c := range cols {
		sel[i] = fmt.Sprintf("%q", c)
	}
	order := ""
	if have["id"] {
		order = ` ORDER BY "id"`
	}
	q := fmt.Sprintf("SELECT %s FROM %q%s", strings.Join(sel, ", "), table, order)
	rows, err := s.db.QueryContext(s.ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query %q: %w", table, err)
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan %q: %w", table, err)
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = vals[i]
		}
		if have["is_deleted"] && asInt(m["is_deleted"]) == 1 {
			continue
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// asInt coerces a sqlite value (int64, []byte, string, bool, nil) to int64.
func asInt(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case bool:
		if t {
			return 1
		}
		return 0
	case []byte:
		return parseIntString(string(t))
	case string:
		return parseIntString(t)
	default:
		return 0
	}
}

func parseIntString(str string) int64 {
	str = strings.TrimSpace(str)
	var n int64
	neg := false
	for i, r := range str {
		if i == 0 && (r == '-' || r == '+') {
			neg = r == '-'
			continue
		}
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	if neg {
		return -n
	}
	return n
}

// asString coerces a sqlite value to string.
func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%v", t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// asBool treats 1 / "1" / true as true.
func asBool(v any) bool { return asInt(v) == 1 }
