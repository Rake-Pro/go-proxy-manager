package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"gopkg.in/yaml.v3"
)

// kindDir maps an object Kind() to its subdirectory under the config repo.
var kindDir = map[string]string{
	"ProxyHost":        "proxy-hosts",
	"RedirectHost":     "redirect-hosts",
	"StreamHost":       "stream-hosts",
	"DeadHost":         "dead-hosts",
	"Certificate":      "certificates",
	"DNSProvider":      "dns-providers",
	"IdentityProvider": "identity-providers",
	"AccessList":       "access-lists",
	"Middleware":       "middlewares",
}

const settingsFile = "settings.yaml"

// Sentinel errors so callers can branch on outcome without matching message text.
var (
	// ErrNotFound is returned when an object does not exist.
	ErrNotFound = errors.New("object not found")
	// ErrDanglingRef is returned when a delete would leave a dangling reference.
	ErrDanglingRef = errors.New("delete would leave a dangling reference")
)

// Store reads and writes the typed config objects as per-object YAML files in a
// git repo. It is safe for concurrent use.
type Store struct {
	dir string
	git GitRepo
	mu  sync.RWMutex
}

// New returns a Store rooted at dir using the given GitRepo.
func New(dir string, git GitRepo) *Store {
	return &Store{dir: dir, git: git}
}

// Dir returns the config repo root.
func (s *Store) Dir() string { return s.dir }

// Init creates the directory tree, initialises the git repo, seeds default
// settings, and makes an initial commit if the repo is empty.
func (s *Store) Init(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return err
	}
	for _, d := range kindDir {
		if err := os.MkdirAll(filepath.Join(s.dir, d), 0o750); err != nil {
			return err
		}
	}
	if err := s.git.EnsureRepo(ctx); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.dir, settingsFile)); os.IsNotExist(err) {
		if err := writeYAML(filepath.Join(s.dir, settingsFile), model.DefaultSettings()); err != nil {
			return err
		}
	}
	if head, err := s.git.Head(ctx); err == nil && head == "" {
		if _, err := s.git.CommitAll(ctx, "Initialise go-proxy-manager config", Author{}); err != nil {
			return err
		}
	}
	return nil
}

// Load assembles the full Config and Settings from disk and validates them
// (per-object plus whole-config referential integrity). A validation failure is
// returned so a bad on-disk state is surfaced, not silently compiled.
func (s *Store) Load(ctx context.Context) (model.Config, model.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() (model.Config, model.Settings, error) {
	var cfg model.Config
	cfg.SchemaVersion = model.SchemaVersion
	var err error

	if cfg.ProxyHosts, err = loadDir[model.ProxyHost](s.dir, "proxy-hosts"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.RedirectHosts, err = loadDir[model.RedirectHost](s.dir, "redirect-hosts"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.StreamHosts, err = loadDir[model.StreamHost](s.dir, "stream-hosts"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.DeadHosts, err = loadDir[model.DeadHost](s.dir, "dead-hosts"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.Certificates, err = loadDir[model.Certificate](s.dir, "certificates"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.DNSProviders, err = loadDir[model.DNSProvider](s.dir, "dns-providers"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.IdentityProviders, err = loadDir[model.IdentityProvider](s.dir, "identity-providers"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.AccessLists, err = loadDir[model.AccessList](s.dir, "access-lists"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.Middlewares, err = loadDir[model.Middleware](s.dir, "middlewares"); err != nil {
		return cfg, model.Settings{}, err
	}

	settings := model.DefaultSettings()
	sp := filepath.Join(s.dir, settingsFile)
	if _, statErr := os.Stat(sp); statErr == nil {
		if err = readYAML(sp, &settings); err != nil {
			return cfg, settings, fmt.Errorf("%s: %w", settingsFile, err)
		}
	}

	if err = cfg.Validate(); err != nil {
		return cfg, settings, fmt.Errorf("config validation failed: %w", err)
	}
	if err = settings.Validate(); err != nil {
		return cfg, settings, fmt.Errorf("settings validation failed: %w", err)
	}
	return cfg, settings, nil
}

// Save validates obj, verifies the whole resulting config still has intact
// references, writes the object's YAML file, and commits. The commit is
// authored by the operator who made the change.
func (s *Store) Save(ctx context.Context, obj model.Object, author Author) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := obj.Validate(); err != nil {
		return "", err
	}
	dir, ok := kindDir[obj.Kind()]
	if !ok {
		return "", fmt.Errorf("unknown object kind %q", obj.Kind())
	}

	// Referential-integrity gate: apply this object to the current config and
	// validate the whole graph BEFORE writing, so no dangling reference is ever
	// committed.
	cfg, _, err := s.loadLocked()
	if err != nil {
		return "", err
	}
	merged := withObject(&cfg, obj)
	if err := merged.Validate(); err != nil {
		return "", fmt.Errorf("rejecting save: %w", err)
	}
	if lits := model.LiteralSecrets(merged); len(lits) > 0 {
		return "", fmt.Errorf("refusing to commit literal secret(s): %v; use ${ENV:...} or ${FILE:...} placeholders", lits)
	}

	meta := obj.GetMeta()
	now := time.Now().UTC()
	path := filepath.Join(s.dir, dir, meta.Name+".yaml")
	if err := writeYAML(path, stampTimes(obj, now)); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s %q: update", obj.Kind(), meta.Name)
	return s.git.CommitAll(ctx, msg, author)
}

// Delete removes the named object of the given kind. It refuses the delete if
// doing so would leave a dangling reference (e.g. removing a certificate still
// used by a host), surfacing the referrers instead. On success it removes the
// YAML file and commits.
func (s *Store) Delete(ctx context.Context, kind, name string, author Author) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := model.ValidateName(name); err != nil {
		return "", err
	}
	dir, ok := kindDir[kind]
	if !ok {
		return "", fmt.Errorf("unknown object kind %q", kind)
	}
	path := filepath.Join(s.dir, dir, name+".yaml")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("%s %q: %w", kind, name, ErrNotFound)
	}

	cfg, _, err := s.loadLocked()
	if err != nil {
		return "", err
	}
	if err := withoutObject(&cfg, kind, name).Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrDanglingRef, err)
	}

	if err := os.Remove(path); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s %q: delete", kind, name)
	return s.git.CommitAll(ctx, msg, author)
}

// SaveBatch writes many objects in a single commit. It merges them onto the
// current config, validates the whole graph once, and only then writes every
// file and commits - so a bulk import (e.g. from NPM) lands atomically as one
// revision, or fails without writing a partial, invalid state.
func (s *Store) SaveBatch(ctx context.Context, objs []model.Object, message string, author Author) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, _, err := s.loadLocked()
	if err != nil {
		return "", err
	}
	merged := cfg
	for _, o := range objs {
		merged = withObject(&merged, o)
	}
	if err := merged.Validate(); err != nil {
		return "", fmt.Errorf("batch validation failed: %w", err)
	}
	if lits := model.LiteralSecrets(merged); len(lits) > 0 {
		return "", fmt.Errorf("refusing to commit literal secret(s): %v; use ${ENV:...} or ${FILE:...} placeholders", lits)
	}

	now := time.Now().UTC()
	for _, o := range objs {
		dir, ok := kindDir[o.Kind()]
		if !ok {
			return "", fmt.Errorf("unknown object kind %q", o.Kind())
		}
		path := filepath.Join(s.dir, dir, o.GetMeta().Name+".yaml")
		if err := writeYAML(path, stampTimes(o, now)); err != nil {
			return "", err
		}
	}
	if message == "" {
		message = "Import configuration"
	}
	return s.git.CommitAll(ctx, message, author)
}

// Head returns the current config repo HEAD commit hash.
func (s *Store) Head(ctx context.Context) (string, error) {
	return s.git.Head(ctx)
}

// History returns the git history for one object's file.
func (s *Store) History(ctx context.Context, kind, name string, limit int) ([]Commit, error) {
	if err := model.ValidateName(name); err != nil {
		return nil, err
	}
	dir, ok := kindDir[kind]
	if !ok {
		return nil, fmt.Errorf("unknown object kind %q", kind)
	}
	rel := filepath.Join(dir, name+".yaml")
	return s.git.Log(ctx, rel, limit)
}

// RepoHistory returns the repo-wide config change history.
func (s *Store) RepoHistory(ctx context.Context, limit int) ([]Commit, error) {
	return s.git.Log(ctx, "", limit)
}

// SaveSettings validates and writes the singleton settings object, then commits.
func (s *Store) SaveSettings(ctx context.Context, settings model.Settings, author Author) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := settings.Validate(); err != nil {
		return "", err
	}
	if lits := model.LiteralSecrets(settings); len(lits) > 0 {
		return "", fmt.Errorf("refusing to commit literal secret(s): %v; use ${ENV:...} or ${FILE:...} placeholders", lits)
	}
	if settings.SchemaVersion == 0 {
		settings.SchemaVersion = model.SchemaVersion
	}
	if err := writeYAML(filepath.Join(s.dir, settingsFile), settings); err != nil {
		return "", err
	}
	return s.git.CommitAll(ctx, "Settings: update", author)
}

func loadDir[T any](root, sub string) ([]T, error) {
	dir := filepath.Join(root, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []T
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		var v T
		if err := readYAML(filepath.Join(dir, e.Name()), &v); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", sub, e.Name(), err)
		}
		out = append(out, v)
	}
	return out, nil
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

// writeYAML atomically writes v as YAML to path (temp file + rename).
func writeYAML(path string, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
