package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

// symlinkOrSkip creates link -> target, skipping the test on a platform or
// account that cannot create symbolic links.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
}

func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o640); err != nil {
		t.Fatal(err)
	}
}

func hostYAML(t *testing.T, name string) []byte {
	t.Helper()
	b, err := yaml.Marshal(sampleHost(name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func settingsYAML(t *testing.T) []byte {
	t.Helper()
	b, err := yaml.Marshal(model.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestLoadRefusesSymlinksThatLeaveTheConfigDir plants links in a stored tree and
// loads it: a link resolving inside the config dir loads as today, one that
// resolves outside it or nowhere fails the load and names the file.
func TestLoadRefusesSymlinksThatLeaveTheConfigDir(t *testing.T) {
	tests := []struct {
		name string
		// plant gets the config dir and a directory outside it.
		plant func(t *testing.T, dir, outside string)
		// wantErr is the file the error must name; "" means the load succeeds.
		wantErr string
		// wantHost, when set, must be loaded.
		wantHost string
	}{
		{
			name: "object file linked inside, relative",
			plant: func(t *testing.T, dir, _ string) {
				mustWrite(t, filepath.Join(dir, "shared", "app.yaml"), hostYAML(t, "app"))
				symlinkOrSkip(t, filepath.Join("..", "shared", "app.yaml"), filepath.Join(dir, "proxy-hosts", "app.yaml"))
			},
			wantHost: "app",
		},
		{
			name: "object file linked inside, absolute",
			plant: func(t *testing.T, dir, _ string) {
				mustWrite(t, filepath.Join(dir, "shared", "app.yaml"), hostYAML(t, "app"))
				symlinkOrSkip(t, filepath.Join(dir, "shared", "app.yaml"), filepath.Join(dir, "proxy-hosts", "app.yaml"))
			},
			wantHost: "app",
		},
		{
			name: "file name starting with two dots is inside",
			plant: func(t *testing.T, dir, _ string) {
				mustWrite(t, filepath.Join(dir, "..shared", "app.yaml"), hostYAML(t, "app"))
				symlinkOrSkip(t, filepath.Join("..", "..shared", "app.yaml"), filepath.Join(dir, "proxy-hosts", "app.yaml"))
			},
			wantHost: "app",
		},
		{
			name: "configmap style ..data indirection",
			plant: func(t *testing.T, dir, _ string) {
				ts := filepath.Join(dir, "..2026_01_01_00_00_00.000000001")
				mustWrite(t, filepath.Join(ts, "settings.yaml"), settingsYAML(t))
				mustWrite(t, filepath.Join(ts, "proxy-hosts", "app.yaml"), hostYAML(t, "app"))
				symlinkOrSkip(t, filepath.Base(ts), filepath.Join(dir, "..data"))
				if err := os.Remove(filepath.Join(dir, settingsFile)); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, filepath.Join("..data", "settings.yaml"), filepath.Join(dir, settingsFile))
				if err := os.Remove(filepath.Join(dir, "proxy-hosts")); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, filepath.Join("..data", "proxy-hosts"), filepath.Join(dir, "proxy-hosts"))
			},
			wantHost: "app",
		},
		{
			name: "object file linked outside",
			plant: func(t *testing.T, dir, outside string) {
				mustWrite(t, filepath.Join(outside, "app.yaml"), hostYAML(t, "app"))
				symlinkOrSkip(t, filepath.Join(outside, "app.yaml"), filepath.Join(dir, "proxy-hosts", "app.yaml"))
			},
			wantErr: "proxy-hosts/app.yaml",
		},
		{
			name: "object file linked to a sibling dir sharing the name prefix",
			plant: func(t *testing.T, dir, _ string) {
				sibling := dir + "-other"
				mustWrite(t, filepath.Join(sibling, "app.yaml"), hostYAML(t, "app"))
				symlinkOrSkip(t, filepath.Join(sibling, "app.yaml"), filepath.Join(dir, "proxy-hosts", "app.yaml"))
			},
			wantErr: "proxy-hosts/app.yaml",
		},
		{
			name: "object file dangling link",
			plant: func(t *testing.T, dir, outside string) {
				symlinkOrSkip(t, filepath.Join(outside, "missing.yaml"), filepath.Join(dir, "proxy-hosts", "app.yaml"))
			},
			wantErr: "proxy-hosts/app.yaml",
		},
		{
			name: "object file link loop",
			plant: func(t *testing.T, dir, _ string) {
				symlinkOrSkip(t, "b.yaml", filepath.Join(dir, "proxy-hosts", "a.yaml"))
				symlinkOrSkip(t, "a.yaml", filepath.Join(dir, "proxy-hosts", "b.yaml"))
			},
			wantErr: "proxy-hosts/a.yaml",
		},
		{
			name: "kind dir linked outside",
			plant: func(t *testing.T, dir, outside string) {
				mustWrite(t, filepath.Join(outside, "hosts", "app.yaml"), hostYAML(t, "app"))
				if err := os.Remove(filepath.Join(dir, "proxy-hosts")); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, filepath.Join(outside, "hosts"), filepath.Join(dir, "proxy-hosts"))
			},
			wantErr: "proxy-hosts",
		},
		{
			name: "empty kind dir linked outside",
			plant: func(t *testing.T, dir, outside string) {
				if err := os.Remove(filepath.Join(dir, "redirect-hosts")); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(outside, "empty"), 0o750); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, filepath.Join(outside, "empty"), filepath.Join(dir, "redirect-hosts"))
			},
			wantErr: "redirect-hosts",
		},
		{
			name: "kind dir dangling link",
			plant: func(t *testing.T, dir, outside string) {
				if err := os.Remove(filepath.Join(dir, "proxy-hosts")); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, filepath.Join(outside, "nothing"), filepath.Join(dir, "proxy-hosts"))
			},
			wantErr: "proxy-hosts",
		},
		{
			name: "settings linked outside",
			plant: func(t *testing.T, dir, outside string) {
				mustWrite(t, filepath.Join(outside, "settings.yaml"), settingsYAML(t))
				if err := os.Remove(filepath.Join(dir, settingsFile)); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, filepath.Join(outside, "settings.yaml"), filepath.Join(dir, settingsFile))
			},
			wantErr: settingsFile,
		},
		{
			name: "settings dangling link",
			plant: func(t *testing.T, dir, outside string) {
				if err := os.Remove(filepath.Join(dir, settingsFile)); err != nil {
					t.Fatal(err)
				}
				symlinkOrSkip(t, filepath.Join(outside, "settings.yaml"), filepath.Join(dir, settingsFile))
			},
			wantErr: settingsFile,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestStore(t)
			outside := t.TempDir()
			tc.plant(t, st.Dir(), outside)

			cfg, _, err := st.Load(context.Background())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("load: %v", err)
				}
				if tc.wantHost != "" && (len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != tc.wantHost) {
					t.Fatalf("hosts = %+v, want %q", cfg.ProxyHosts, tc.wantHost)
				}
				return
			}
			if !errors.Is(err, ErrSymlinkEscapes) {
				t.Fatalf("load err = %v, want ErrSymlinkEscapes", err)
			}
			if !strings.HasPrefix(err.Error(), tc.wantErr+": ") {
				t.Fatalf("load err = %q, want it to name %s first", err, tc.wantErr)
			}
		})
	}
}

func TestLoadErrorSaysTheTargetIsOutside(t *testing.T) {
	st := newTestStore(t)
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "app.yaml"), hostYAML(t, "app"))
	symlinkOrSkip(t, filepath.Join(outside, "app.yaml"), filepath.Join(st.Dir(), "proxy-hosts", "app.yaml"))
	_, _, err := st.Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "the link target") || !strings.Contains(err.Error(), "is outside the config directory") {
		t.Fatalf("load err = %v, want it to say the link target is outside the config directory", err)
	}
}

// TestConfigDirReachedThroughALink: the config dir itself is a link, and an
// object links to a file inside the real dir by its real absolute path.
func TestConfigDirReachedThroughALink(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.MkdirAll(realDir, 0o750); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "config")
	symlinkOrSkip(t, realDir, linked)
	st := New(linked, NewExecGit(linked))
	ctx := context.Background()
	if err := st.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	mustWrite(t, filepath.Join(realDir, "shared", "app.yaml"), hostYAML(t, "app"))
	symlinkOrSkip(t, filepath.Join(realDir, "shared", "app.yaml"), filepath.Join(realDir, "proxy-hosts", "app.yaml"))
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 {
		t.Fatalf("hosts = %+v, want app", cfg.ProxyHosts)
	}
	if _, err := st.Save(ctx, sampleHost("web"), Author{}); err != nil {
		t.Fatalf("save through the linked config dir: %v", err)
	}
}

// TestInitLeavesADanglingSettingsLinkForLoadToRefuse: Init must not replace a
// settings.yaml link that resolves nowhere with defaults.
func TestInitLeavesADanglingSettingsLinkForLoadToRefuse(t *testing.T) {
	dir := t.TempDir()
	symlinkOrSkip(t, filepath.Join(t.TempDir(), "settings.yaml"), filepath.Join(dir, settingsFile))
	st := New(dir, NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := st.Load(context.Background()); !errors.Is(err, ErrSymlinkEscapes) {
		t.Fatalf("load err = %v, want ErrSymlinkEscapes", err)
	}
}

func TestLedgersRefuseSymlinksThatLeaveTheConfigDir(t *testing.T) {
	ledgers := []struct {
		file string
		load func(*Store) error
	}{
		{dnsLedgerFile, func(st *Store) error { _, _, err := st.LoadDNSLedger(context.Background()); return err }},
		{accessListSourcesFile, func(st *Store) error {
			_, _, err := st.LoadAccessListSourceLedger(context.Background())
			return err
		}},
	}
	for _, l := range ledgers {
		for _, dangling := range []bool{false, true} {
			t.Run(l.file+map[bool]string{false: " outside", true: " dangling"}[dangling], func(t *testing.T) {
				st := newTestStore(t)
				target := filepath.Join(t.TempDir(), l.file)
				if !dangling {
					mustWrite(t, target, []byte("schemaVersion: 1\n"))
				}
				symlinkOrSkip(t, target, filepath.Join(st.Dir(), l.file))
				err := l.load(st)
				if !errors.Is(err, ErrSymlinkEscapes) || !strings.HasPrefix(err.Error(), l.file+": ") {
					t.Fatalf("load err = %v, want ErrSymlinkEscapes naming %s", err, l.file)
				}
			})
		}
	}
}

// TestSaveNeverWritesThroughAKindDirLinkedOutside: with an empty kind dir that
// links outside, the load passes (nothing to read), so the write itself must
// refuse.
func TestSaveNeverWritesThroughAKindDirLinkedOutside(t *testing.T) {
	st := newTestStore(t)
	outside := t.TempDir()
	if err := os.Remove(filepath.Join(st.Dir(), "proxy-hosts")); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(st.Dir(), "proxy-hosts"))
	_, err := st.Save(context.Background(), sampleHost("app"), Author{})
	if !errors.Is(err, ErrSymlinkEscapes) {
		t.Fatalf("save err = %v, want ErrSymlinkEscapes", err)
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("save wrote %d file(s) outside the config dir", len(entries))
	}
}

// TestSaveOverAnInsideLinkWorksAsBefore: a link to a file inside the config dir
// is replaced by the written file, as the temp-file-and-rename write always did.
func TestSaveOverAnInsideLinkWorksAsBefore(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	shared := filepath.Join(st.Dir(), "shared", "app.yaml")
	mustWrite(t, shared, hostYAML(t, "app"))
	symlinkOrSkip(t, filepath.Join("..", "shared", "app.yaml"), filepath.Join(st.Dir(), "proxy-hosts", "app.yaml"))
	h := sampleHost("app")
	h.Domains = []string{"changed.example.com"}
	if _, err := st.Save(ctx, h, Author{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil || len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Domains[0] != "changed.example.com" {
		t.Fatalf("load after save: %+v, %v", cfg.ProxyHosts, err)
	}
	if b, _ := os.ReadFile(shared); !bytes.Equal(b, hostYAML(t, "app")) {
		t.Fatal("the write must replace the link, not write through it")
	}
}

// TestFailedWriteRestoresAReplacedLink: a write replaces a link inside the
// config dir with a regular file and a delete removes it; when the commit then
// fails, the rollback puts the link back as it was (not a copy of its file),
// so the tree is exactly HEAD again and the link's target is untouched.
func TestFailedWriteRestoresAReplacedLink(t *testing.T) {
	ctx := context.Background()
	changed := sampleHost("app")
	changed.Domains = []string{"changed.example.com"}
	ops := []struct {
		name string
		link string // the linked path, relative to the config dir
		op   func(st *Store) error
	}{
		{"save", "proxy-hosts/app.yaml", func(st *Store) error {
			_, err := st.Save(ctx, changed, Author{})
			return err
		}},
		{"save batch", "proxy-hosts/app.yaml", func(st *Store) error {
			_, err := st.SaveBatch(ctx, []model.Object{changed}, "batch", Author{})
			return err
		}},
		{"apply batch upsert", "proxy-hosts/app.yaml", func(st *Store) error {
			_, err := st.ApplyBatch(ctx, []model.Object{changed}, nil, "batch", Author{}, nil)
			return err
		}},
		{"apply batch delete", "proxy-hosts/app.yaml", func(st *Store) error {
			_, err := st.ApplyBatch(ctx, nil, []ObjectRef{{Kind: "ProxyHost", Name: "app"}}, "batch", Author{}, nil)
			return err
		}},
		{"delete", "proxy-hosts/app.yaml", func(st *Store) error {
			_, err := st.Delete(ctx, "ProxyHost", "app", Author{})
			return err
		}},
		{"save settings", settingsFile, func(st *Store) error {
			_, s, err := st.Load(ctx)
			if err != nil {
				return err
			}
			s.AppName = "changed"
			_, err = st.SaveSettings(ctx, s, Author{})
			return err
		}},
	}
	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			git := &failingCommitGit{GitRepo: NewExecGit(dir)}
			st := New(dir, git)
			if err := st.Init(ctx); err != nil {
				t.Fatal(err)
			}
			shared := filepath.Join(dir, "shared", "app.yaml")
			mustWrite(t, shared, hostYAML(t, "app"))
			mustWrite(t, filepath.Join(dir, "shared", settingsFile), settingsYAML(t))
			if err := os.Remove(filepath.Join(dir, settingsFile)); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, filepath.Join("shared", settingsFile), filepath.Join(dir, settingsFile))
			symlinkOrSkip(t, filepath.Join("..", "shared", "app.yaml"), filepath.Join(dir, "proxy-hosts", "app.yaml"))
			head := commitTree(t, st, "links")
			linked := filepath.Join(dir, filepath.FromSlash(tc.link))
			target, err := os.Readlink(linked)
			if err != nil {
				t.Fatal(err)
			}
			targetBytes, err := os.ReadFile(linked)
			if err != nil {
				t.Fatal(err)
			}

			git.fail = true
			if err := tc.op(st); err == nil {
				t.Fatal("the write must fail when its commit fails")
			}
			git.fail = false

			if got, err := os.Readlink(linked); err != nil || got != target {
				fi, _ := os.Lstat(linked)
				t.Fatalf("%s after the rollback: link %q (%v), mode %v; want the link to %q back", tc.link, got, err, fi.Mode(), target)
			}
			if b, err := os.ReadFile(linked); err != nil || !bytes.Equal(b, targetBytes) {
				t.Fatalf("the link's target changed: %q, %v", b, err)
			}
			if clean, err := git.IsClean(ctx); err != nil || !clean {
				t.Fatalf("the working tree must be clean after the rollback: clean=%v err=%v", clean, err)
			}
			if now, _ := st.Head(ctx); now != head {
				t.Fatal("HEAD must not move")
			}
		})
	}
}

// TestLinkRollbackNeverLeavesThePathEmpty: when putting the link back fails,
// the path keeps what the write left there. A rollback that removed the file
// first would leave the path empty, and the next writer's commit would record
// the object's deletion.
func TestLinkRollbackNeverLeavesThePathEmpty(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	git := &failingCommitGit{GitRepo: NewExecGit(dir)}
	st := New(dir, git)
	if err := st.Init(ctx); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "shared", "app.yaml"), hostYAML(t, "app"))
	linked := filepath.Join(dir, "proxy-hosts", "app.yaml")
	symlinkOrSkip(t, filepath.Join("..", "shared", "app.yaml"), linked)
	commitTree(t, st, "link")

	orig := symlinkFile
	t.Cleanup(func() { symlinkFile = orig })
	symlinkFile = func(string, string) error { return errors.New("symlink: injected failure") }
	git.fail = true
	changed := sampleHost("app")
	changed.Domains = []string{"changed.example.com"}
	if _, err := st.Save(ctx, changed, Author{}); err == nil {
		t.Fatal("the save must fail when its commit fails")
	}
	if _, err := os.Lstat(linked); err != nil {
		t.Fatalf("a failed link rollback left the path empty: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(linked))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("rollback left %s behind", e.Name())
		}
	}
}

// TestLinkRollbackCleansUpWhenTheRenameFails: the link is made under a
// temporary name and renamed into place; a rename that fails (here: a
// directory now stands at the path) removes the temporary link again.
func TestLinkRollbackCleansUpWhenTheRenameFails(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "shared", "app.yaml"), hostYAML(t, "app"))
	path := filepath.Join(dir, "proxy-hosts", "app.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join("..", "shared", "app.yaml"), path)
	snap, err := snapshotFile(dir, path)
	if err != nil || snap.link == "" {
		t.Fatalf("snapshot = %+v, %v; want the link recorded", snap, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(path, "occupied"), []byte("x"))
	restoreFiles([]fileState{snap})
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("a failed rename left %s behind", e.Name())
		}
	}
}

// commitTree commits the working tree as it is, symbolic links included.
func commitTree(t *testing.T, st *Store, msg string) string {
	t.Helper()
	sha, err := st.git.CommitAll(context.Background(), msg, Author{})
	if err != nil || sha == "" {
		t.Fatalf("commit %q: %q, %v", msg, sha, err)
	}
	return sha
}

// TestRevertNeverWritesThroughAnAPITokenLinkFromHistory: a commit whose
// api-tokens file is a link outside the config dir. The revert restores that
// tree and then writes the live tokens back, which must not follow the link.
func TestRevertNeverWritesThroughAnAPITokenLinkFromHistory(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const hash = "4444444444444444444444444444444444444444444444444444444444444444"
	if _, err := st.Save(ctx, sampleToken("ci", hash), Author{}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	tokenFile := filepath.Join(st.Dir(), "api-tokens", "ci.yaml")
	live, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim")
	mustWrite(t, victim, []byte("untouched"))
	if err := os.Remove(tokenFile); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, victim, tokenFile)
	linked := commitTree(t, st, "plant a link")
	if err := os.Remove(tokenFile); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, tokenFile, live)
	commitTree(t, st, "back to a regular file")

	if _, err := st.Revert(ctx, linked, Author{}); !errors.Is(err, ErrSymlinkEscapes) {
		t.Fatalf("revert err = %v, want ErrSymlinkEscapes", err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "untouched" {
		t.Fatalf("revert wrote through the link: %q", b)
	}
	if _, _, err := st.Load(ctx); err != nil {
		t.Fatalf("the refused revert must leave a loadable tree: %v", err)
	}
}

// TestRevertNeverPrunesThroughAnAPITokenDirLinkFromHistory: the same for a
// commit whose api-tokens directory is a link outside: pruning it would delete
// files there.
func TestRevertNeverPrunesThroughAnAPITokenDirLinkFromHistory(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const hash = "4444444444444444444444444444444444444444444444444444444444444444"
	if _, err := st.Save(ctx, sampleToken("ci", hash), Author{}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	tokens := filepath.Join(st.Dir(), "api-tokens")
	live, err := os.ReadFile(filepath.Join(tokens, "ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "victim.yaml"), []byte("untouched"))
	if err := os.RemoveAll(tokens); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, tokens)
	linked := commitTree(t, st, "plant a dir link")
	if err := os.Remove(tokens); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(tokens, "ci.yaml"), live)
	commitTree(t, st, "back to a regular dir")

	if _, err := st.Revert(ctx, linked, Author{}); !errors.Is(err, ErrSymlinkEscapes) {
		t.Fatalf("revert err = %v, want ErrSymlinkEscapes", err)
	}
	if b, err := os.ReadFile(filepath.Join(outside, "victim.yaml")); err != nil || string(b) != "untouched" {
		t.Fatalf("revert touched the outside dir: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "ci.yaml")); !os.IsNotExist(err) {
		t.Fatalf("revert wrote a token outside the config dir: %v", err)
	}
}

// TestRevertObjectRefusesALinkFromHistory: a scoped revert to a revision where
// the object file was a link outside is refused at the load that follows.
func TestRevertObjectRefusesALinkFromHistory(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if _, err := st.Save(ctx, sampleHost("app"), Author{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	file := filepath.Join(st.Dir(), "proxy-hosts", "app.yaml")
	live, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "app.yaml")
	mustWrite(t, outside, hostYAML(t, "app"))
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, file)
	linked := commitTree(t, st, "plant a link")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, file, live)
	commitTree(t, st, "back to a regular file")

	if _, err := st.RevertObject(ctx, "ProxyHost", "app", linked, Author{}); !errors.Is(err, ErrSymlinkEscapes) {
		t.Fatalf("revert err = %v, want ErrSymlinkEscapes", err)
	}
	if fi, err := os.Lstat(file); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the refused revert must roll back to the regular file: %v", err)
	}
}

func TestExportRefusesALinkOutsideAndInlinesOneInside(t *testing.T) {
	t.Run("outside", func(t *testing.T) {
		st := newTestStore(t)
		outside := filepath.Join(t.TempDir(), "app.yaml")
		mustWrite(t, outside, hostYAML(t, "app"))
		symlinkOrSkip(t, outside, filepath.Join(st.Dir(), "proxy-hosts", "app.yaml"))
		_, err := st.Export(context.Background())
		if !errors.Is(err, ErrSymlinkEscapes) || !strings.Contains(err.Error(), "proxy-hosts/app.yaml") {
			t.Fatalf("export err = %v, want ErrSymlinkEscapes naming the file", err)
		}
	})
	t.Run("inside", func(t *testing.T) {
		st := newTestStore(t)
		mustWrite(t, filepath.Join(st.Dir(), "shared", "app.yaml"), hostYAML(t, "app"))
		symlinkOrSkip(t, filepath.Join("..", "shared", "app.yaml"), filepath.Join(st.Dir(), "proxy-hosts", "app.yaml"))
		archive, err := st.Export(context.Background())
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			t.Fatal(err)
		}
		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				t.Fatal("proxy-hosts/app.yaml missing from the archive")
			}
			if err != nil {
				t.Fatal(err)
			}
			if hdr.Name != "proxy-hosts/app.yaml" {
				continue
			}
			b, _ := io.ReadAll(tr)
			if hdr.Typeflag != tar.TypeReg || !bytes.Equal(b, hostYAML(t, "app")) {
				t.Fatalf("entry type %v content %q, want the link target as a regular file", hdr.Typeflag, b)
			}
			return
		}
	})
}

// TestRestoreNeverWritesOrRemovesThroughLinks covers the tree a restore
// replaces: a kind dir that links outside is refused before anything there is
// removed, and a settings.yaml link to nothing is replaced, not written through
// (which would create its target outside the config dir).
func TestRestoreNeverWritesOrRemovesThroughLinks(t *testing.T) {
	t.Run("kind dir linked outside", func(t *testing.T) {
		st := newTestStore(t)
		outside := t.TempDir()
		mustWrite(t, filepath.Join(outside, "victim.yaml"), hostYAML(t, "victim"))
		if err := os.Remove(filepath.Join(st.Dir(), "proxy-hosts")); err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, outside, filepath.Join(st.Dir(), "proxy-hosts"))
		archive := archiveWith(t, map[string][]byte{"proxy-hosts/app.yaml": hostYAML(t, "app")})
		if _, err := st.Restore(context.Background(), archive, Author{}); !errors.Is(err, ErrSymlinkEscapes) {
			t.Fatalf("restore err = %v, want ErrSymlinkEscapes", err)
		}
		if _, err := os.Stat(filepath.Join(outside, "victim.yaml")); err != nil {
			t.Fatalf("restore removed a file outside the config dir: %v", err)
		}
		if _, err := os.Stat(filepath.Join(outside, "app.yaml")); !os.IsNotExist(err) {
			t.Fatalf("restore wrote a file outside the config dir: %v", err)
		}
	})
	t.Run("dangling settings link", func(t *testing.T) {
		st := newTestStore(t)
		target := filepath.Join(t.TempDir(), "settings.yaml")
		sp := filepath.Join(st.Dir(), settingsFile)
		if err := os.Remove(sp); err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, target, sp)
		archive := archiveWith(t, map[string][]byte{settingsFile: settingsYAML(t), "proxy-hosts/app.yaml": hostYAML(t, "app")})
		if _, err := st.Restore(context.Background(), archive, Author{}); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("restore created the link target outside the config dir: %v", err)
		}
		if fi, err := os.Lstat(sp); err != nil || !fi.Mode().IsRegular() {
			t.Fatalf("settings.yaml after restore: %v, want a regular file", err)
		}
	})
}

// TestRestoreSkipsLinkEntries: a restore never creates a link from an archive.
func TestRestoreSkipsLinkEntries(t *testing.T) {
	st := newTestStore(t)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	app := hostYAML(t, "app")
	for _, hdr := range []*tar.Header{
		{Name: "proxy-hosts/app.yaml", Mode: 0o600, Size: int64(len(app)), Typeflag: tar.TypeReg},
		{Name: "proxy-hosts/evil.yaml", Linkname: "/etc/hostname", Typeflag: tar.TypeSymlink},
		{Name: "proxy-hosts/hard.yaml", Linkname: "proxy-hosts/app.yaml", Typeflag: tar.TypeLink},
	} {
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write(app); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Restore(context.Background(), buf.Bytes(), Author{}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	for _, name := range []string{"evil.yaml", "hard.yaml"} {
		if _, err := os.Lstat(filepath.Join(st.Dir(), "proxy-hosts", name)); !os.IsNotExist(err) {
			t.Fatalf("restore created %s from a link entry: %v", name, err)
		}
	}
}
