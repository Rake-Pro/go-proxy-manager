// Package session provides a SQLite-backed session store for the proxy manager.
package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a session does not exist or has expired.
var ErrNotFound = errors.New("session: not found")

// Session represents an authenticated user session.
type Session struct {
	ID        string
	Subject   string
	Email     string
	Name      string
	Roles     []string
	IdP       string
	AMR       []string
	CSRFToken string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Store is a SQLite-backed session store. It is safe for concurrent use.
type Store struct {
	db *sql.DB
}

// newToken returns a 32-byte cryptographically random token encoded as
// unpadded base64url.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Open opens (creating if necessary) the SQLite database at dbPath, applies
// pragmas, and runs the idempotent migration.
func Open(dbPath string) (*Store, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("session: create db dir: %w", err)
		}
	}

	// Session IDs and CSRF tokens must not be world-readable. Pre-create the
	// file 0600 so SQLite (and its -wal/-shm sidecars) inherit that mode, and
	// tighten any pre-existing file.
	if f, err := os.OpenFile(dbPath, os.O_CREATE, 0o600); err != nil {
		return nil, fmt.Errorf("session: create db file: %w", err)
	} else {
		f.Close()
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("session: chmod db file: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("session: open db: %w", err)
	}

	// SQLite writes serialize anyway; a single connection avoids
	// "database is locked" errors under concurrent writers.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("session: pragma %q: %w", p, err)
		}
	}

	const migration = `
CREATE TABLE IF NOT EXISTS sessions (
	id         TEXT PRIMARY KEY,
	subject    TEXT NOT NULL DEFAULT '',
	email      TEXT NOT NULL DEFAULT '',
	name       TEXT NOT NULL DEFAULT '',
	roles      TEXT NOT NULL DEFAULT '[]',
	idp        TEXT NOT NULL DEFAULT '',
	amr        TEXT NOT NULL DEFAULT '[]',
	csrf_token TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT 0,
	expires_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);`
	if _, err := db.Exec(migration); err != nil {
		db.Close()
		return nil, fmt.Errorf("session: migrate: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Create inserts a session, generating ID, CSRFToken and CreatedAt when unset.
func (s *Store) Create(ctx context.Context, sess *Session) error {
	if sess.ID == "" {
		id, err := newToken()
		if err != nil {
			return fmt.Errorf("session: generate id: %w", err)
		}
		sess.ID = id
	}
	if sess.CSRFToken == "" {
		tok, err := newToken()
		if err != nil {
			return fmt.Errorf("session: generate csrf: %w", err)
		}
		sess.CSRFToken = tok
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}

	roles, err := marshalSlice(sess.Roles)
	if err != nil {
		return fmt.Errorf("session: marshal roles: %w", err)
	}
	amr, err := marshalSlice(sess.AMR)
	if err != nil {
		return fmt.Errorf("session: marshal amr: %w", err)
	}

	const q = `INSERT INTO sessions
		(id, subject, email, name, roles, idp, amr, csrf_token, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, q,
		sess.ID, sess.Subject, sess.Email, sess.Name, roles, sess.IdP, amr,
		sess.CSRFToken, sess.CreatedAt.Unix(), sess.ExpiresAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("session: insert: %w", err)
	}
	return nil
}

// Get returns the session by id. If the row is missing, or expired, it returns
// ErrNotFound; expired rows are deleted as a side effect.
func (s *Store) Get(ctx context.Context, id string) (*Session, error) {
	const q = `SELECT id, subject, email, name, roles, idp, amr, csrf_token, created_at, expires_at
		FROM sessions WHERE id = ?`

	var (
		sess               Session
		roles, amr         string
		createdAt, expires int64
	)
	err := s.db.QueryRowContext(ctx, q, id).Scan(
		&sess.ID, &sess.Subject, &sess.Email, &sess.Name, &roles, &sess.IdP, &amr,
		&sess.CSRFToken, &createdAt, &expires,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("session: query: %w", err)
	}

	if expires <= time.Now().Unix() {
		// Expired: clean up and report as missing.
		_ = s.Delete(ctx, id)
		return nil, ErrNotFound
	}

	if err := json.Unmarshal([]byte(roles), &sess.Roles); err != nil {
		return nil, fmt.Errorf("session: unmarshal roles: %w", err)
	}
	if err := json.Unmarshal([]byte(amr), &sess.AMR); err != nil {
		return nil, fmt.Errorf("session: unmarshal amr: %w", err)
	}
	sess.CreatedAt = time.Unix(createdAt, 0).UTC()
	sess.ExpiresAt = time.Unix(expires, 0).UTC()

	return &sess, nil
}

// Delete removes a session by id. Absence is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM sessions WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("session: delete: %w", err)
	}
	return nil
}

// Touch updates the session's expiry (sliding expiration).
func (s *Store) Touch(ctx context.Context, id string, expiresAt time.Time) error {
	const q = `UPDATE sessions SET expires_at = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, expiresAt.Unix(), id); err != nil {
		return fmt.Errorf("session: touch: %w", err)
	}
	return nil
}

// GC deletes all expired sessions and returns the number removed.
func (s *Store) GC(ctx context.Context) (int, error) {
	const q = `DELETE FROM sessions WHERE expires_at <= ?`
	res, err := s.db.ExecContext(ctx, q, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("session: gc: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("session: gc rows: %w", err)
	}
	return int(n), nil
}

// marshalSlice JSON-encodes a string slice, normalizing nil to an empty array.
func marshalSlice(v []string) (string, error) {
	if v == nil {
		v = []string{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
