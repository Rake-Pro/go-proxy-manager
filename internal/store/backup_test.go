package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

// archiveWith builds a gzip-tar restore archive from a map of config-relative
// path -> file bytes, mirroring the layout Export produces.
func archiveWith(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRestoreRejectsLiteralSecret(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	// Baseline good state to prove the refused restore rolls back to it.
	if _, err := st.Save(ctx, sampleHost("app"), Author{Name: "admin"}); err != nil {
		t.Fatalf("save baseline: %v", err)
	}

	// An archive whose IdP carries a plaintext client secret (not ${ENV:}/${FILE:}).
	idpYAML, err := yaml.Marshal(sampleIdP("idp", model.Secret("plaintext-secret")))
	if err != nil {
		t.Fatalf("marshal idp: %v", err)
	}
	archive := archiveWith(t, map[string][]byte{"identity-providers/idp.yaml": idpYAML})

	_, err = st.Restore(ctx, archive, Author{Name: "admin"})
	if err == nil || !strings.Contains(err.Error(), "literal secret") {
		t.Fatalf("restore must refuse a literal secret, got err=%v", err)
	}

	// Rollback: the refused restore left the baseline intact and committed nothing.
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "app" {
		t.Fatalf("refused restore must roll back to the baseline, got %+v", cfg.ProxyHosts)
	}
	if len(cfg.IdentityProviders) != 0 {
		t.Fatalf("refused restore must not persist the archived idp, got %d", len(cfg.IdentityProviders))
	}
}

func TestRevertRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	c1, err := st.Save(ctx, sampleHost("app"), Author{Name: "admin"})
	if err != nil {
		t.Fatalf("save app: %v", err)
	}
	if _, err := st.Save(ctx, sampleHost("two"), Author{Name: "admin"}); err != nil {
		t.Fatalf("save two: %v", err)
	}

	// Revert to the state after the first save: "two" should be gone, "app" kept.
	rev, err := st.Revert(ctx, c1, Author{Name: "admin"})
	if err != nil {
		t.Fatalf("revert: %v", err)
	}
	if rev == "" {
		t.Fatal("revert should produce a new commit")
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "app" {
		t.Fatalf("revert did not restore the c1 tree: %+v", cfg.ProxyHosts)
	}

	// The revert is itself a commit on top of history (forward history preserved).
	hist, err := st.RepoHistory(ctx, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) < 4 { // init, app, two, revert
		t.Fatalf("expected revert recorded as a new commit, history=%d", len(hist))
	}
}

func TestRevertRejectsBadHash(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Revert(context.Background(), "not-a-hash", Author{}); err == nil {
		t.Fatal("expected an invalid hash to be rejected")
	}
}

func TestExportRestoreRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if _, err := st.Save(ctx, sampleHost("app"), Author{Name: "admin"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	archive, err := st.Export(ctx)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(archive) == 0 {
		t.Fatal("export produced an empty archive")
	}

	// Mutate after export, then restore should bring back exactly the exported state.
	if _, err := st.Save(ctx, sampleHost("later"), Author{Name: "admin"}); err != nil {
		t.Fatalf("save later: %v", err)
	}
	if _, err := st.Restore(ctx, archive, Author{Name: "admin"}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "app" {
		t.Fatalf("restore did not match the exported state: %+v", cfg.ProxyHosts)
	}
}

func TestRestoreRejectsUnsafePath(t *testing.T) {
	st := newTestStore(t)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	content := []byte("malicious")
	hdr := &tar.Header{Name: "../escape.yaml", Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()

	if _, err := st.Restore(context.Background(), buf.Bytes(), Author{}); err == nil {
		t.Fatal("expected restore to reject a traversal archive entry")
	}
}

// An archive taken before DeadHost became ParkedHost still restores: entries
// under the retired dead-hosts/ directory are mapped onto parked-hosts/. The
// mapping is one-way - nothing is ever written back under the old name.
func TestRestoreMapsLegacyDeadHostsEntries(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	legacy := []byte("name: gone\ndomains: [gone.example.com]\nstatusCode: 410\n")
	archive := archiveWith(t, map[string][]byte{"dead-hosts/gone.yaml": legacy})

	if _, err := st.Restore(ctx, archive, Author{Name: "admin"}); err != nil {
		t.Fatalf("restore of a pre-rename archive: %v", err)
	}

	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ParkedHosts) != 1 || cfg.ParkedHosts[0].Name != "gone" || cfg.ParkedHosts[0].StatusCode != 410 {
		t.Fatalf("legacy dead-hosts entry did not restore as a parked host: %+v", cfg.ParkedHosts)
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "parked-hosts", "gone.yaml")); err != nil {
		t.Fatalf("expected the object under parked-hosts/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "dead-hosts")); !os.IsNotExist(err) {
		t.Fatalf("restore must not recreate the retired dead-hosts/ directory (err=%v)", err)
	}
}

// The remap is a rename of the leading directory only; it must not become a
// traversal hole. A hostile entry that merely starts with the retired prefix is
// still rejected by allowedRestorePath after the rewrite.
func TestRestoreLegacyRemapStillRejectsTraversal(t *testing.T) {
	st := newTestStore(t)
	archive := archiveWith(t, map[string][]byte{"dead-hosts/../../escape.yaml": []byte("name: x\n")})
	if _, err := st.Restore(context.Background(), archive, Author{}); err == nil {
		t.Fatal("restore accepted a traversal path under the retired directory prefix")
	}
}

// Restore, like Revert, keeps the live api-tokens: an archive must not revive a
// rotated digest or a token that has since been deleted.
func TestRestorePreservesLiveAPITokens(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const liveHash = "5555555555555555555555555555555555555555555555555555555555555555"
	const archHash = "4444444444444444444444444444444444444444444444444444444444444444"

	if _, err := st.Save(ctx, sampleHost("web"), Author{}); err != nil {
		t.Fatalf("save host: %v", err)
	}
	tokYAML, err := yaml.Marshal(sampleToken("old", archHash))
	if err != nil {
		t.Fatal(err)
	}
	hostYAML, err := os.ReadFile(filepath.Join(st.dir, "proxy-hosts", "web.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	archive := archiveWith(t, map[string][]byte{
		"proxy-hosts/web.yaml": hostYAML,
		"api-tokens/old.yaml":  tokYAML,
	})

	if _, err := st.Save(ctx, sampleToken("live", liveHash), Author{}); err != nil {
		t.Fatalf("save live token: %v", err)
	}
	if _, err := st.Restore(ctx, archive, Author{}); err != nil {
		t.Fatalf("restore: %v", err)
	}

	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.APITokens) != 1 || cfg.APITokens[0].Name != "live" || cfg.APITokens[0].TokenHash != liveHash {
		t.Fatalf("tokens = %+v, want only the live token", cfg.APITokens)
	}
	if _, err := os.Stat(filepath.Join(st.dir, "api-tokens", "old.yaml")); !os.IsNotExist(err) {
		t.Fatalf("the archive's token file must not be written (stat err = %v)", err)
	}
	if len(cfg.ProxyHosts) != 1 {
		t.Fatalf("hosts = %+v, the rest of the archive must still restore", cfg.ProxyHosts)
	}
	if clean, err := st.git.IsClean(ctx); err != nil || !clean {
		t.Fatalf("restore must leave a clean tree (clean=%v err=%v)", clean, err)
	}
}

// TestRestoreRefusesFileNameThatIsNotAnObjectName proves a restore archive may
// only carry kind files named "<valid object name>.yaml", the path every save,
// delete and revert derives.
func TestRestoreRefusesFileNameThatIsNotAnObjectName(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	head := headOf(t, st)
	host := []byte("name: bar\ndomains: [bar.example.com]\nupstream: {scheme: http, host: 127.0.0.1, port: 9}\n")
	_, err := st.Restore(ctx, archiveWith(t, map[string][]byte{"proxy-hosts/Not A Name!!.yaml": host}), Author{})
	if err == nil || !strings.Contains(err.Error(), "not an allowed config path") {
		t.Fatalf("restore of a file whose base name is not an object name: err = %v", err)
	}
	if h := headOf(t, st); h != head {
		t.Fatalf("a refused restore committed %s", h)
	}
}

// TestLoadWarnsOnFileNameObjectNameMismatch proves a file whose object name
// differs from its file name still loads and is reported as a warning.
func TestLoadWarnsOnFileNameObjectNameMismatch(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	host := []byte("name: bar\ndomains: [bar.example.com]\nupstream: {scheme: http, host: 127.0.0.1, port: 9}\n")
	if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "foo.yaml"), host, 0o640); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("a misnamed file must still load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "bar" {
		t.Fatalf("hosts = %+v, want the one named bar", cfg.ProxyHosts)
	}
	warned := false
	for _, w := range cfg.Warnings() {
		if strings.Contains(w, `proxy-hosts/foo.yaml: object name "bar" does not match the file name`) {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected a file-name mismatch warning, got %v", cfg.Warnings())
	}
}

// misnamedStore returns a store whose committed proxy-hosts/foo.yaml holds the
// object named bar.
func misnamedStore(t *testing.T) *Store {
	t.Helper()
	st := newTestStore(t)
	host := []byte("name: bar\ndomains: [bar.example.com]\nupstream: {scheme: http, host: 127.0.0.1, port: 9}\n")
	if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "foo.yaml"), host, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := st.git.CommitAll(context.Background(), "out of band", Author{}); err != nil {
		t.Fatal(err)
	}
	return st
}

// TestSaveMovesAMisnamedFileInOneCommit proves that saving an object stored
// under another file name, through save, batch and apply, writes
// <name>.yaml and removes the old file in the same single commit, so the
// object is never stored twice and stays deletable afterwards.
func TestSaveMovesAMisnamedFileInOneCommit(t *testing.T) {
	ctx := context.Background()
	writes := map[string]func(*Store, model.ProxyHost) error{
		"save": func(st *Store, h model.ProxyHost) error { _, err := st.Save(ctx, h, Author{}); return err },
		"batch": func(st *Store, h model.ProxyHost) error {
			_, err := st.SaveBatch(ctx, []model.Object{h}, "", Author{})
			return err
		},
		"apply": func(st *Store, h model.ProxyHost) error {
			_, err := st.ApplyBatch(ctx, []model.Object{h}, nil, "", Author{}, nil)
			return err
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			st := misnamedStore(t)
			before := headOf(t, st)
			if _, _, _, err := st.DryRun(ctx, sampleHost("bar")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(st.Dir(), "proxy-hosts", "foo.yaml")); err != nil {
				t.Fatalf("a dry run must not move the file: %v", err)
			}
			h := sampleHost("bar")
			h.Maintenance = true
			if err := write(st, h); err != nil {
				t.Fatalf("write bar: %v", err)
			}
			if n := runGit(t, st.Dir(), "rev-list", "--count", before+"..HEAD"); n != "1" {
				t.Fatalf("commits = %s, want 1", n)
			}
			changed := runGit(t, st.Dir(), "show", "--name-status", "--format=", "HEAD")
			if !strings.Contains(changed, "proxy-hosts/foo.yaml") || !strings.Contains(changed, "proxy-hosts/bar.yaml") {
				t.Fatalf("the commit must add bar.yaml and remove foo.yaml, got:\n%s", changed)
			}
			if _, err := os.Stat(filepath.Join(st.Dir(), "proxy-hosts", "foo.yaml")); !os.IsNotExist(err) {
				t.Fatalf("foo.yaml must be gone: %v", err)
			}
			cfg, _, err := st.Load(ctx)
			if err != nil {
				t.Fatalf("load after the write: %v", err)
			}
			if len(cfg.ProxyHosts) != 1 || !cfg.ProxyHosts[0].Maintenance {
				t.Fatalf("hosts = %+v, want the edited bar alone", cfg.ProxyHosts)
			}
			for _, w := range cfg.Warnings() {
				if strings.Contains(w, "does not match the file name") {
					t.Fatalf("no mismatch may be left: %v", w)
				}
			}
			if _, err := st.Delete(ctx, "ProxyHost", "bar", Author{}); err != nil {
				t.Fatalf("delete bar after the move: %v", err)
			}
		})
	}
}

// TestWriteRefusesAFileHeldByAnotherObject proves a write whose file holds a
// different object is refused instead of replacing it, unless that object is
// moved out in the same batch.
func TestWriteRefusesAFileHeldByAnotherObject(t *testing.T) {
	ctx := context.Background()
	st := misnamedStore(t)
	head := headOf(t, st)
	foo := sampleHost("foo")
	if _, err := st.Save(ctx, foo, Author{}); err == nil || !strings.Contains(err.Error(), `holds the object "bar"`) {
		t.Fatalf("save foo over bar's file: err = %v, want refused", err)
	}
	if _, err := st.ApplyBatch(ctx, []model.Object{foo}, nil, "", Author{}, nil); err == nil {
		t.Fatal("apply foo over bar's file must be refused")
	}
	if h := headOf(t, st); h != head {
		t.Fatalf("a refused write committed %s", h)
	}
	if _, err := st.SaveBatch(ctx, []model.Object{foo, sampleHost("bar")}, "", Author{}); err != nil {
		t.Fatalf("a batch that moves bar out and writes foo: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil || len(cfg.ProxyHosts) != 2 {
		t.Fatalf("load = %v, hosts %+v; want foo and bar", err, cfg.ProxyHosts)
	}
	for _, n := range []string{"foo", "bar"} {
		var got model.ProxyHost
		if err := readYAML(st.Dir(), filepath.Join(st.Dir(), "proxy-hosts", n+".yaml"), &got); err != nil || got.Name != n {
			t.Fatalf("%s.yaml holds %q (%v), want %q", n, got.Name, err, n)
		}
	}
}

// TestTwoFilesWithOneNameAreLeftAlone proves no file is picked for removal
// when more than one holds the object name.
func TestTwoFilesWithOneNameAreLeftAlone(t *testing.T) {
	st := misnamedStore(t)
	host := []byte("name: bar\ndomains: [bar2.example.com]\nupstream: {scheme: http, host: 127.0.0.1, port: 9}\n")
	if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "baz.yaml"), host, 0o640); err != nil {
		t.Fatal(err)
	}
	k, err := st.kindFilesLocked("proxy-hosts")
	if err != nil {
		t.Fatal(err)
	}
	if strays, _ := k.strays("bar"); len(strays) != 0 {
		t.Fatalf("strays = %v, want none when two files hold the name", strays)
	}
}

// caseInsensitiveFS makes sameFile and removeFile behave as on a
// case-insensitive file system for the rest of the test: two names in one
// directory that differ only in case are one file.
func caseInsensitiveFS(t *testing.T, removes bool) {
	t.Helper()
	oldSame, oldRemove := sameFile, removeFile
	t.Cleanup(func() { sameFile, removeFile = oldSame, oldRemove })
	sameFile = func(a, b string) bool {
		return filepath.Dir(a) == filepath.Dir(b) && strings.EqualFold(filepath.Base(a), filepath.Base(b))
	}
	if !removes {
		return
	}
	sameFile = oldSame
	removeFile = func(p string) error {
		err := os.Remove(p)
		entries, _ := os.ReadDir(filepath.Dir(p))
		for _, e := range entries {
			if strings.EqualFold(e.Name(), filepath.Base(p)) {
				_ = os.Remove(filepath.Join(filepath.Dir(p), e.Name()))
			}
		}
		return err
	}
}

// caseMisnamedStore returns a store whose committed proxy-hosts/Bar.yaml
// holds the object named bar.
func caseMisnamedStore(t *testing.T) *Store {
	t.Helper()
	st := newTestStore(t)
	host := []byte("name: bar\ndomains: [bar.example.com]\nupstream: {scheme: http, host: 127.0.0.1, port: 9}\n")
	if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "Bar.yaml"), host, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := st.git.CommitAll(context.Background(), "out of band", Author{}); err != nil {
		t.Fatal(err)
	}
	return st
}

// TestCaseOnlyFileNameIsTheObjectsOwnFile proves a file whose name differs
// from <name>.yaml only in case is not removed as a stray, and counts as the
// object's own file for the occupant check, where the file system resolves
// both names to one file.
func TestCaseOnlyFileNameIsTheObjectsOwnFile(t *testing.T) {
	st := caseMisnamedStore(t)
	k, err := st.kindFilesLocked("proxy-hosts")
	if err != nil {
		t.Fatal(err)
	}
	if strays, _ := k.strays("bar"); len(strays) != 1 {
		t.Fatalf("case-sensitive: strays = %v, want Bar.yaml", strays)
	}
	caseInsensitiveFS(t, false)
	if strays, occupant := k.strays("bar"); len(strays) != 0 || occupant != "" {
		t.Fatalf("case-insensitive: strays = %v, occupant = %q, want none", strays, occupant)
	}
	k.byFile["Bar.yaml"] = "baz"
	k.byName = map[string][]string{"baz": {"Bar.yaml"}}
	if _, occupant := k.strays("bar"); occupant != "baz" {
		t.Fatalf("case-insensitive: occupant = %q, want baz", occupant)
	}
}

// TestMisnamedMoveSurvivesOneFileForBothNames proves save, batch and apply
// remove the old file before writing <name>.yaml: where removing Bar.yaml
// also removes bar.yaml (one file on a case-insensitive file system), the
// object is still stored afterwards.
func TestMisnamedMoveSurvivesOneFileForBothNames(t *testing.T) {
	ctx := context.Background()
	writes := map[string]func(*Store, model.ProxyHost) error{
		"save": func(st *Store, h model.ProxyHost) error { _, err := st.Save(ctx, h, Author{}); return err },
		"batch": func(st *Store, h model.ProxyHost) error {
			_, err := st.SaveBatch(ctx, []model.Object{h}, "", Author{})
			return err
		},
		"apply": func(st *Store, h model.ProxyHost) error {
			_, err := st.ApplyBatch(ctx, []model.Object{h}, nil, "", Author{}, nil)
			return err
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			st := caseMisnamedStore(t)
			caseInsensitiveFS(t, true)
			h := sampleHost("bar")
			h.Maintenance = true
			if err := write(st, h); err != nil {
				t.Fatalf("write bar: %v", err)
			}
			cfg, _, err := st.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.ProxyHosts) != 1 || !cfg.ProxyHosts[0].Maintenance {
				t.Fatalf("hosts = %+v, want the edited bar", cfg.ProxyHosts)
			}
			if out := runGit(t, st.Dir(), "status", "--porcelain"); out != "" {
				t.Fatalf("the move must be committed, status:\n%s", out)
			}
		})
	}
}

// TestSaveBatchRestoresAMovedFileWhenTheCommitFails proves a failed batch
// puts back the old file it removed and leaves no new file behind.
func TestSaveBatchRestoresAMovedFileWhenTheCommitFails(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	git := &failingCommitGit{GitRepo: NewExecGit(dir)}
	st := New(dir, git)
	if err := st.Init(ctx); err != nil {
		t.Fatal(err)
	}
	host := []byte("name: bar\ndomains: [bar.example.com]\nupstream: {scheme: http, host: 127.0.0.1, port: 9}\n")
	if err := os.WriteFile(filepath.Join(dir, "proxy-hosts", "foo.yaml"), host, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := git.CommitAll(ctx, "out of band", Author{}); err != nil {
		t.Fatal(err)
	}
	git.fail = true
	if _, err := st.SaveBatch(ctx, []model.Object{sampleHost("bar"), sampleHost("new")}, "", Author{}); err == nil {
		t.Fatal("a failing commit must fail the batch")
	}
	got, err := os.ReadFile(filepath.Join(dir, "proxy-hosts", "foo.yaml"))
	if err != nil || !bytes.Equal(got, host) {
		t.Fatalf("foo.yaml must be put back unchanged: %v", err)
	}
	for _, n := range []string{"bar.yaml", "new.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, "proxy-hosts", n)); !os.IsNotExist(err) {
			t.Fatalf("%s must not be left behind: %v", n, err)
		}
	}
}

// TestFollowerLeavesAMisnamedFileAlone proves the move happens on a leader's
// write only: a follower that pulls and loads a misnamed file rewrites
// nothing, and the leader's next save of the object moves it.
func TestFollowerLeavesAMisnamedFileAlone(t *testing.T) {
	ctx := context.Background()
	leader, follower, push := haPair(t)
	host := []byte("name: bar\ndomains: [bar.example.com]\nupstream: {scheme: http, host: 127.0.0.1, port: 9}\n")
	if err := os.WriteFile(filepath.Join(leader.Dir(), "proxy-hosts", "foo.yaml"), host, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := leader.git.CommitAll(ctx, "out of band", Author{}); err != nil {
		t.Fatal(err)
	}
	push()
	if _, err := follower.PullFFOnly(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := follower.Load(ctx); err != nil {
		t.Fatalf("follower load: %v", err)
	}
	if st := runGit(t, follower.Dir(), "status", "--porcelain"); st != "" {
		t.Fatalf("a follower load must not touch the tree, got %q", st)
	}
	if _, err := os.Stat(filepath.Join(follower.Dir(), "proxy-hosts", "foo.yaml")); err != nil {
		t.Fatalf("follower foo.yaml: %v", err)
	}
	if _, err := leader.Save(ctx, sampleHost("bar"), Author{}); err != nil {
		t.Fatal(err)
	}
	push()
	if _, err := follower.PullFFOnly(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(follower.Dir(), "proxy-hosts", "foo.yaml")); !os.IsNotExist(err) {
		t.Fatalf("the leader's move must replicate: %v", err)
	}
}
