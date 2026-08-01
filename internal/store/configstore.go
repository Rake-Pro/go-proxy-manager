package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// kindDir maps an object Kind() to its subdirectory under the config repo.
var kindDir = map[string]string{
	"ProxyHost":        "proxy-hosts",
	"RedirectHost":     "redirect-hosts",
	"StreamHost":       "stream-hosts",
	"DeadHost":         "dead-hosts",
	"Certificate":      "certificates",
	"ClientCA":         "client-cas",
	"DNSProvider":      "dns-providers",
	"IdentityProvider": "identity-providers",
	"UpstreamGroup":    "upstream-groups",
	"AccessList":       "access-lists",
	"Middleware":       "middlewares",
	"APIToken":         "api-tokens",
}

const settingsFile = "settings.yaml"

// dnsLedgerFile is the DNS record-ownership ledger singleton (see
// model.DNSLedger). It is deliberately NOT part of Config/Settings: the DNS
// reconciler rewrites it whenever it creates, adopts or drops a record, and
// folding that into settings.yaml would make every reconcile rewrite the
// operator's settings - which in turn triggers another reconcile.
const dnsLedgerFile = "dns-ledger.yaml"

// Sentinel errors so callers can branch on outcome without matching message text.
var (
	// ErrNotFound is returned when an object does not exist.
	ErrNotFound = errors.New("object not found")
	// ErrDanglingRef is returned when a delete would leave a dangling reference.
	ErrDanglingRef = errors.New("delete would leave a dangling reference")
	// ErrGeoDBUnavailable is returned by a write that would commit an AccessList
	// with geo rules while no GeoIP database is loaded to evaluate them. Rejecting
	// at the write boundary keeps such a rule out of git entirely (fail closed),
	// rather than committing a config that can only be served as deny-all.
	ErrGeoDBUnavailable = errors.New("geo rules configured but no GeoIP database is loaded")
	// ErrNotRevertible is returned when the object kind must never be restored
	// from history. APIToken is the only one: its file carries a tokenHash, so
	// restoring an older revision would silently revive a secret the operator
	// rotated (or deleted) away. Rotation has to mean revocation, so the whole
	// revert path refuses instead - and a whole-tree Revert leaves api-tokens
	// exactly as they are (see Revert).
	ErrNotRevertible = errors.New("object kind cannot be reverted")
)

// preservedOnRevert lists the config subdirectories a whole-tree Revert must not
// roll back, for the ErrNotRevertible reason above.
var preservedOnRevert = []string{"api-tokens"}

// Store reads and writes the typed config objects as per-object YAML files in a
// git repo. It is safe for concurrent use.
type Store struct {
	dir string
	git GitRepo
	mu  sync.RWMutex

	// geoLoaded reports whether a GeoIP database is currently loaded. It is the
	// only coupling between the store and the geo subsystem: a plain predicate
	// injected from main (geoResolver.Loaded), never a dataplane import. When nil
	// (e.g. the CLI importer, or a test) the geo-availability gate is skipped, so
	// the store stays usable without a geo wiring.
	geoLoaded func() bool
}

// New returns a Store rooted at dir using the given GitRepo.
func New(dir string, git GitRepo) *Store {
	return &Store{dir: dir, git: git}
}

// SetGeoAvailability injects the predicate the store consults to reject a write
// that configures geo rules while no GeoIP database is loaded (fail closed at
// the write boundary). Passing nil disables the gate.
func (s *Store) SetGeoAvailability(loaded func() bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.geoLoaded = loaded
}

// checkGeoAvailable rejects cfg when it configures geo rules but no GeoIP
// database is loaded, so such a config is never committed. With no predicate
// wired the gate is a no-op. Callers hold s.mu.
func (s *Store) checkGeoAvailable(cfg model.Config) error {
	if s.geoLoaded == nil || s.geoLoaded() {
		return nil
	}
	for _, a := range cfg.AccessLists {
		if a.Geo.HasRules() {
			return fmt.Errorf("access list %q: %w (set GPM_GEOIP_DB)", a.Name, ErrGeoDBUnavailable)
		}
	}
	return nil
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
	if cfg.ClientCAs, err = loadDir[model.ClientCA](s.dir, "client-cas"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.DNSProviders, err = loadDir[model.DNSProvider](s.dir, "dns-providers"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.IdentityProviders, err = loadDir[model.IdentityProvider](s.dir, "identity-providers"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.UpstreamGroups, err = loadDir[model.UpstreamGroup](s.dir, "upstream-groups"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.AccessLists, err = loadDir[model.AccessList](s.dir, "access-lists"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.Middlewares, err = loadDir[model.Middleware](s.dir, "middlewares"); err != nil {
		return cfg, model.Settings{}, err
	}
	if cfg.APITokens, err = loadDir[model.APIToken](s.dir, "api-tokens"); err != nil {
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
	if err := s.checkGeoAvailable(merged); err != nil {
		return "", err
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
	if err := s.checkGeoAvailable(merged); err != nil {
		return "", err
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

// ObjectRef names one object for ApplyBatch's delete list.
type ObjectRef struct {
	Kind string
	Name string
}

// ApplyGuard authorises ApplyBatch to touch one object that ALREADY exists in
// the config, and is consulted under the store lock against the freshly loaded
// state - not against whatever the caller planned from. Returning an error skips
// that single upsert or delete; it never fails the batch.
//
// It exists because a caller's plan is computed from a pre-network snapshot: the
// Ingress reconciler loads the config, spends seconds listing the cluster, and
// only then applies. An object created, deleted or relabelled inside that window
// would otherwise be written or removed on the strength of an ownership check
// made against state that no longer exists.
type ApplyGuard func(existing model.Object) error

// findObject returns the object of the given kind and name in cfg, if present.
func findObject(cfg model.Config, kind, name string) (model.Object, bool) {
	var list []model.Object
	switch kind {
	case "ProxyHost":
		for _, o := range cfg.ProxyHosts {
			list = append(list, o)
		}
	case "RedirectHost":
		for _, o := range cfg.RedirectHosts {
			list = append(list, o)
		}
	case "StreamHost":
		for _, o := range cfg.StreamHosts {
			list = append(list, o)
		}
	case "DeadHost":
		for _, o := range cfg.DeadHosts {
			list = append(list, o)
		}
	case "Certificate":
		for _, o := range cfg.Certificates {
			list = append(list, o)
		}
	case "ClientCA":
		for _, o := range cfg.ClientCAs {
			list = append(list, o)
		}
	case "DNSProvider":
		for _, o := range cfg.DNSProviders {
			list = append(list, o)
		}
	case "IdentityProvider":
		for _, o := range cfg.IdentityProviders {
			list = append(list, o)
		}
	case "UpstreamGroup":
		for _, o := range cfg.UpstreamGroups {
			list = append(list, o)
		}
	case "AccessList":
		for _, o := range cfg.AccessLists {
			list = append(list, o)
		}
	case "Middleware":
		for _, o := range cfg.Middlewares {
			list = append(list, o)
		}
	case "APIToken":
		for _, o := range cfg.APITokens {
			list = append(list, o)
		}
	}
	for _, o := range list {
		if o.GetMeta().Name == name {
			return o, true
		}
	}
	return nil, false
}

// fileState is one config file as it was before a batch touched it, so the batch
// can be undone if any later step fails.
type fileState struct {
	path   string
	data   []byte
	exists bool
}

// snapshotFile captures a file's current contents (or its absence).
func snapshotFile(path string) (fileState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileState{path: path}, nil
		}
		return fileState{}, err
	}
	return fileState{path: path, data: b, exists: true}, nil
}

// restoreFiles puts every snapshotted path back the way it was. It is a
// best-effort undo run on a failure path, so it reports rather than returns.
func restoreFiles(snaps []fileState) {
	for _, f := range snaps {
		var err error
		if f.exists {
			err = os.WriteFile(f.path, f.data, 0o640)
		} else if err = os.Remove(f.path); os.IsNotExist(err) {
			err = nil
		}
		if err != nil {
			log.Error().Err(err).Str("path", f.path).
				Msg("config store: could not roll back a failed batch; the working tree may hold uncommitted changes")
		}
	}
}

// ApplyBatch writes every object in upserts and removes every object named in
// deletes as ONE commit. It is SaveBatch plus removals: the whole merged graph
// (upserts applied, deletes removed) is validated once - including the
// dangling-reference check Delete performs per object - before anything touches
// the working tree, so a batch is all-or-nothing and never commits an
// intermediate state that never existed as a whole.
//
// It exists for the Ingress-discovery reconciler, whose unit of work is a whole
// reconcile: a poll that adds two hosts and removes one is a single revision an
// operator can read and revert, not four commits with a reload, webhook and DNS
// trigger apiece (see docs/design/ingress-discovery.md §2).
//
// An empty batch is a no-op: it returns ("", nil) without committing, so a
// steady-state reconcile produces no empty revisions. A delete naming an object
// that does not exist is skipped rather than failing the batch - the reconciler
// works from a snapshot, and an object removed underneath it is already in the
// desired end state.
//
// guard (may be nil) re-checks ownership of every object that already exists,
// under this lock and against the state just loaded, closing the window between
// the caller planning the batch and the batch landing. A guarded-away object is
// skipped, and a batch left with nothing to do returns ("", nil).
//
// Atomicity is enforced, not assumed: every file the batch touches is
// snapshotted first, and a failed write, removal or commit rolls the working
// tree back. Leaving it mutated but uncommitted would make the next Load see
// changes the caller was told had failed - and the following unrelated commit
// would sweep them in.
func (s *Store) ApplyBatch(ctx context.Context, upserts []model.Object, deletes []ObjectRef, message string, author Author, guard ApplyGuard) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, _, err := s.loadLocked()
	if err != nil {
		return "", err
	}

	applied := make([]model.Object, 0, len(upserts))
	var removals []string
	merged := cfg
	for _, o := range upserts {
		if err := o.Validate(); err != nil {
			return "", err
		}
		if _, ok := kindDir[o.Kind()]; !ok {
			return "", fmt.Errorf("unknown object kind %q", o.Kind())
		}
		if existing, ok := findObject(cfg, o.Kind(), o.GetMeta().Name); ok && guard != nil {
			if err := guard(existing); err != nil {
				log.Warn().Err(err).Str("kind", o.Kind()).Str("name", o.GetMeta().Name).
					Msg("config store: batch upsert skipped, the object on disk is no longer one the caller owns")
				continue
			}
		}
		merged = withObject(&merged, o)
		applied = append(applied, o)
	}
	for _, ref := range deletes {
		if err := model.ValidateName(ref.Name); err != nil {
			return "", err
		}
		dir, ok := kindDir[ref.Kind]
		if !ok {
			return "", fmt.Errorf("unknown object kind %q", ref.Kind)
		}
		path := filepath.Join(s.dir, dir, ref.Name+".yaml")
		if _, err := os.Stat(path); err != nil {
			continue // already gone; the desired end state is satisfied
		}
		if guard != nil {
			existing, ok := findObject(cfg, ref.Kind, ref.Name)
			if !ok {
				continue // on disk but not in the loaded config: nothing to authorise
			}
			if err := guard(existing); err != nil {
				log.Warn().Err(err).Str("kind", ref.Kind).Str("name", ref.Name).
					Msg("config store: batch delete skipped, the object on disk is no longer one the caller owns")
				continue
			}
		}
		merged = withoutObject(&merged, ref.Kind, ref.Name)
		removals = append(removals, path)
	}
	if len(applied) == 0 && len(removals) == 0 {
		return "", nil
	}

	if err := merged.Validate(); err != nil {
		return "", fmt.Errorf("batch validation failed: %w", err)
	}
	if err := s.checkGeoAvailable(merged); err != nil {
		return "", err
	}
	if lits := model.LiteralSecrets(merged); len(lits) > 0 {
		return "", fmt.Errorf("refusing to commit literal secret(s): %v; use ${ENV:...} or ${FILE:...} placeholders", lits)
	}

	// Snapshot every path the batch will touch, so any failure below - including a
	// commit cancelled by shutdown - can be undone completely.
	snaps := make([]fileState, 0, len(applied)+len(removals))
	for _, o := range applied {
		snap, err := snapshotFile(filepath.Join(s.dir, kindDir[o.Kind()], o.GetMeta().Name+".yaml"))
		if err != nil {
			return "", err
		}
		snaps = append(snaps, snap)
	}
	for _, path := range removals {
		snap, err := snapshotFile(path)
		if err != nil {
			return "", err
		}
		snaps = append(snaps, snap)
	}

	now := time.Now().UTC()
	for _, o := range applied {
		path := filepath.Join(s.dir, kindDir[o.Kind()], o.GetMeta().Name+".yaml")
		if err := writeYAML(path, stampTimes(o, now)); err != nil {
			restoreFiles(snaps)
			return "", err
		}
	}
	for _, path := range removals {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			restoreFiles(snaps)
			return "", err
		}
	}
	if message == "" {
		message = "Apply configuration batch"
	}
	sha, err := s.git.CommitAll(ctx, message, author)
	if err != nil {
		restoreFiles(snaps)
		return "", err
	}
	return sha, nil
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

// commitHashRe matches a git commit hash (short or full); used to keep an
// untrusted revert target from being interpreted as a git option/path.
var commitHashRe = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

// Revert restores the entire config to the state at commit hash and records the
// result as a NEW commit, so forward history is preserved (a revert can itself be
// reverted). The restored config is validated before committing; if it does not
// load cleanly the working tree is rolled back to HEAD and an error is returned.
// Reverting to the current HEAD is a no-op ("" hash, nil error).
func (s *Store) Revert(ctx context.Context, hash string, author Author) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !commitHashRe.MatchString(hash) {
		return "", fmt.Errorf("invalid commit hash %q", hash)
	}
	head, err := s.git.Head(ctx)
	if err != nil {
		return "", err
	}

	// Snapshot the never-reverted directories BEFORE the tree changes, and write
	// them back over the restored tree, so the revert cannot resurrect a rotated
	// or deleted API token digest (see ErrNotRevertible).
	preserved, err := s.snapshotDirs(preservedOnRevert)
	if err != nil {
		return "", fmt.Errorf("revert: snapshot %v: %w", preservedOnRevert, err)
	}

	if err := s.git.RestoreTree(ctx, hash); err != nil {
		return "", fmt.Errorf("revert: restore tree %q: %w", hash, err)
	}
	if err := s.writeDirs(preserved); err != nil {
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("revert: preserve %v: %w", preservedOnRevert, err)
	}
	restored, _, err := s.loadLocked()
	if err != nil {
		// The restored state is invalid (e.g. schema drift): undo and report.
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("revert refused, config at %q does not validate: %w", hash, err)
	}
	if err := s.checkGeoAvailable(restored); err != nil {
		// The target config would re-introduce a geo rule with no database to
		// evaluate it: undo the tree change and refuse, committing nothing.
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("revert refused, config at %q: %w", hash, err)
	}

	short := hash
	if len(short) > 12 {
		short = short[:12]
	}
	sha, err := s.git.CommitAll(ctx, fmt.Sprintf("Revert config to %s", short), author)
	if err != nil {
		// Don't leave the restored-but-uncommitted tree behind: a later write's
		// commit would silently sweep it in.
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("revert to %q: commit: %w", hash, err)
	}
	return sha, nil
}

// RevertObject restores ONLY the named object's file to its state at commit hash
// and records the result as a NEW commit, leaving every other object untouched -
// unlike the whole-tree Revert, which resets the entire config. The object kind
// selects the managed subdirectory and the name is validated, so the restored
// path is derived from the trusted kind mapping, never from client input (no
// traversal, absolute path, or unknown kind can reach git). After restoring the
// single file the whole config is loaded and validated exactly like Revert; on
// any failure the working tree is rolled back to HEAD and nothing is committed.
// If the object does not exist at the target commit the revert is refused with a
// clear error (a scoped revert never recreates a deletion).
func (s *Store) RevertObject(ctx context.Context, kind, name, hash string, author Author) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !commitHashRe.MatchString(hash) {
		return "", fmt.Errorf("invalid commit hash %q", hash)
	}
	if err := model.ValidateName(name); err != nil {
		return "", err
	}
	dir, ok := kindDir[kind]
	if !ok {
		return "", fmt.Errorf("unknown object kind %q", kind)
	}
	for _, d := range preservedOnRevert {
		if d == dir {
			return "", fmt.Errorf("%s %q: %w - restoring an older revision would restore its stored token digest and revive a secret that was rotated or deleted away; create a replacement token instead", kind, name, ErrNotRevertible)
		}
	}
	rel := dir + "/" + name + ".yaml"

	head, err := s.git.Head(ctx)
	if err != nil {
		return "", err
	}

	if err := s.git.RestorePath(ctx, hash, rel); err != nil {
		if errors.Is(err, ErrPathNotInCommit) {
			return "", fmt.Errorf("%s %q is not present at commit %s (a scoped revert cannot recreate a deletion): %w", kind, name, hash, err)
		}
		return "", fmt.Errorf("revert %s %q: restore %q from %q: %w", kind, name, rel, hash, err)
	}

	restored, _, err := s.loadLocked()
	if err != nil {
		// The single-file restore left the whole config invalid (e.g. it references
		// an object deleted since): undo the file change and refuse.
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("revert refused, config with %s %q at %q does not validate: %w", kind, name, hash, err)
	}
	if err := s.checkGeoAvailable(restored); err != nil {
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("revert refused, config at %q: %w", hash, err)
	}

	short := hash
	if len(short) > 12 {
		short = short[:12]
	}
	sha, err := s.git.CommitAll(ctx, fmt.Sprintf("Revert %s/%s to %s", kind, name, short), author)
	if err != nil {
		// Same as Revert: never leave the restored file uncommitted for a later
		// write's commit to sweep in.
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("revert %s %q to %q: commit: %w", kind, name, hash, err)
	}
	return sha, nil
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

// LoadDNSLedger reads the DNS record-ownership ledger. A missing file is an
// EMPTY ledger, not an error: that is the state every deployment starts in, and
// it means "gpm owns nothing yet", which the reconciler treats as adopt-only -
// it can never be read as "everything is unowned, delete it".
func (s *Store) LoadDNSLedger() (model.DNSLedger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var l model.DNSLedger
	path := filepath.Join(s.dir, dnsLedgerFile)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return model.DNSLedger{SchemaVersion: model.SchemaVersion}, nil
		}
		return l, err
	}
	if err := readYAML(path, &l); err != nil {
		return l, fmt.Errorf("%s: %w", dnsLedgerFile, err)
	}
	if err := l.Validate(); err != nil {
		return l, fmt.Errorf("%s: %w", dnsLedgerFile, err)
	}
	return l, nil
}

// SaveDNSLedger validates and writes the ledger singleton, then commits. Writing
// an unchanged ledger produces no commit (CommitAll is a no-op on a clean tree),
// so a steady-state reconcile leaves no history noise.
func (s *Store) SaveDNSLedger(ctx context.Context, l model.DNSLedger, author Author) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := l.Validate(); err != nil {
		return "", err
	}
	if l.SchemaVersion == 0 {
		l.SchemaVersion = model.SchemaVersion
	}
	if err := writeYAML(filepath.Join(s.dir, dnsLedgerFile), l); err != nil {
		return "", err
	}
	return s.git.CommitAll(ctx, "DNS sync ledger: update", author)
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

// snapshotDirs reads every regular file directly under each named config
// subdirectory into memory, keyed by "<sub>/<file>". A missing directory is not
// an error - it just contributes nothing. Callers hold s.mu.
func (s *Store) snapshotDirs(subs []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, sub := range subs {
		entries, err := os.ReadDir(filepath.Join(s.dir, sub))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(s.dir, sub, e.Name()))
			if err != nil {
				return nil, err
			}
			out[sub+"/"+e.Name()] = b
		}
	}
	return out, nil
}

// writeDirs makes each snapshotted subdirectory contain exactly the files the
// snapshot holds: files added by the intervening tree change are removed, so a
// token deleted since the revert target stays deleted rather than coming back.
// Callers hold s.mu.
func (s *Store) writeDirs(files map[string][]byte) error {
	for _, sub := range preservedOnRevert {
		full := filepath.Join(s.dir, sub)
		if err := os.MkdirAll(full, 0o750); err != nil {
			return err
		}
		entries, err := os.ReadDir(full)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if _, keep := files[sub+"/"+e.Name()]; !keep {
				if err := os.Remove(filepath.Join(full, e.Name())); err != nil {
					return err
				}
			}
		}
	}
	for rel, b := range files {
		if err := os.WriteFile(filepath.Join(s.dir, filepath.FromSlash(rel)), b, 0o640); err != nil {
			return err
		}
	}
	return nil
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
