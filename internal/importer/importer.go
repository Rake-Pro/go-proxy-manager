// Package importer provides a one-time, best-effort migration from an existing
// Nginx Proxy Manager (NPM) / NPMplus /data directory into go-proxy-manager's
// typed config model.
//
// It is a clean-room reimplementation: it only READS the NPM SQLite database and
// certificate PEM files and maps them onto the model types. No NPM/NPMplus code
// or config templates are copied. The importer writes nothing; the caller is
// responsible for persisting the returned Result (config objects + cert files).
//
// The reader is deliberately defensive: NPM and NPMplus schemas drift across
// versions, so it probes sqlite_master and PRAGMA table_info to learn which
// tables and columns exist and skips-with-warning anything missing instead of
// failing the whole import.
package importer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	_ "modernc.org/sqlite"
)

// Warning records something that could not be imported faithfully, so the
// migration is never silently lossy.
type Warning struct {
	Object string `json:"object"` // e.g. `proxy_host #3 (app2.example.com)`
	Field  string `json:"field"`  // e.g. "advanced_config"
	Reason string `json:"reason"` // human explanation of what wasn't imported and why
}

// CertCopy tells the caller to copy a certificate's PEM files into the cert store
// under the given object Name (the caller owns the actual file copy).
type CertCopy struct {
	Name    string `json:"name"`
	CertPEM string `json:"certPem"` // absolute source path to fullchain.pem
	KeyPEM  string `json:"keyPem"`  // absolute source path to privkey.pem
}

// Result is the full outcome of a best-effort import.
type Result struct {
	Objects  []model.Object // mapped ProxyHost/RedirectHost/ParkedHost/StreamHost/AccessList/Certificate values
	Certs    []CertCopy
	Warnings []Warning
	Summary  map[string]int // counts keyed by kind, e.g. {"ProxyHost":7,"Certificate":2,...}
}

// Import reads the NPM data dir (locating the sqlite DB and cert files within)
// and returns the mapping. It writes NOTHING; the caller persists Result.
func Import(ctx context.Context, npmDataDir string) (*Result, error) {
	dbPath, err := locateDB(npmDataDir)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	defer db.Close()

	imp := &importState{
		ctx:     ctx,
		db:      db,
		dataDir: npmDataDir,
		res: &Result{
			Summary: map[string]int{},
		},
		certNames: map[int64]string{}, // certificate id -> object Name
		certOK:    map[int64]bool{},   // certificate id -> files found (safe to reference)
		alNames:   map[int64]string{}, // access_list id -> object Name
		usedNames: map[string]map[string]bool{},
	}

	if err := imp.run(); err != nil {
		return nil, err
	}
	return imp.res, nil
}

// dbCandidates is the ordered list of database filenames to probe.
var dbCandidates = []string{
	"database.sqlite",
	"database.sqlite3",
	"nginxproxymanager.sqlite",
	"db.sqlite",
}

func locateDB(dir string) (string, error) {
	for _, name := range dbCandidates {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("no NPM sqlite database found under %q (tried %s)", dir, strings.Join(dbCandidates, ", "))
}

type importState struct {
	ctx     context.Context
	db      *sql.DB
	dataDir string
	res     *Result

	certNames map[int64]string
	certOK    map[int64]bool
	alNames   map[int64]string

	// usedNames[kind][name] tracks taken names per kind for global uniqueness.
	usedNames map[string]map[string]bool
}

func (s *importState) warn(object, field, reason string) {
	s.res.Warnings = append(s.res.Warnings, Warning{Object: object, Field: field, Reason: reason})
}

// run orders the import so that referenced objects (certs, access lists) are
// mapped before the hosts that reference them.
func (s *importState) run() error {
	if err := s.importCertificates(); err != nil {
		return err
	}
	if err := s.importAccessLists(); err != nil {
		return err
	}
	if err := s.importProxyHosts(); err != nil {
		return err
	}
	if err := s.importRedirectionHosts(); err != nil {
		return err
	}
	if err := s.importParkedHosts(); err != nil {
		return err
	}
	if err := s.importStreams(); err != nil {
		return err
	}
	return nil
}

// add validates an object and, on success, appends it and bumps the summary.
// On validation failure it warns and drops the object, returning false.
func (s *importState) add(object, field string, o model.Object) bool {
	if err := o.Validate(); err != nil {
		s.warn(object, field, fmt.Sprintf("object failed validation and was skipped: %v", err))
		return false
	}
	s.res.Objects = append(s.res.Objects, o)
	s.res.Summary[o.Kind()]++
	return true
}
