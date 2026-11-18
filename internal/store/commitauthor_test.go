package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestAuthorControlBytesAreDropped proves control characters and angle
// brackets in a principal's name or email never reach git: the commit lands
// under the cleaned identity and stays visible in history.
func TestAuthorControlBytesAreDropped(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.Save(ctx, sampleHost("quiet"), Author{Name: "mallory\x00", Email: "m@example.com"}); err != nil {
		t.Fatalf("save with a NUL in the author name: %v", err)
	}
	if _, err := st.Save(ctx, sampleHost("hidden"), Author{Name: "eve\x1fx\n<root@example.com>", Email: "<e@example.com>\r"}); err != nil {
		t.Fatalf("save with control bytes in the author: %v", err)
	}
	hist, err := st.History(ctx, "ProxyHost", "hidden", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("history entries for hidden = %d, want 1", len(hist))
	}
	if hist[0].Author != "evexroot@example.com" || hist[0].Email != "e@example.com" {
		t.Fatalf("author = %q email = %q, want the cleaned identity", hist[0].Author, hist[0].Email)
	}
	repo, err := st.RepoHistory(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo) != 3 || repo[1].Author != "mallory" {
		t.Fatalf("repo history = %+v, want init, quiet by mallory, hidden", repo)
	}
}

// TestAuthorNameGitRefusesFallsBackToDefault proves a name git would refuse
// (only characters it strips from an ident) is committed under the store's
// default author instead of failing the write.
func TestAuthorNameGitRefusesFallsBackToDefault(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for i, name := range []string{".", " ;,'\" ", "\x00<>"} {
		h := sampleHost(fmt.Sprintf("h%d", i))
		if _, err := st.Save(ctx, h, Author{Name: name, Email: "m@example.com"}); err != nil {
			t.Fatalf("save with author name %q: %v", name, err)
		}
		hist, err := st.History(ctx, "ProxyHost", h.Name, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(hist) != 1 || hist[0].Author != "go-proxy-manager" {
			t.Fatalf("author name %q: history = %+v, want one commit by the default author", name, hist)
		}
	}
}

// TestFailedCommitRestoresTheWorkingTree proves a write whose commit fails
// leaves the working tree as it was, so the change is neither served by the
// next reload nor swept into the next writer's commit.
func TestFailedCommitRestoresTheWorkingTree(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.Save(ctx, sampleHost("keep"), Author{}); err != nil {
		t.Fatal(err)
	}
	real := st.git
	st.git = &failingCommitGit{GitRepo: real, fail: true}

	if _, err := st.Save(ctx, sampleHost("new"), Author{}); err == nil {
		t.Fatal("save: expected the commit failure")
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "proxy-hosts", "new.yaml")); !os.IsNotExist(err) {
		t.Fatalf("save: the file of a failed commit must be removed, stat err = %v", err)
	}
	if _, err := st.Delete(ctx, "ProxyHost", "keep", Author{}); err == nil {
		t.Fatal("delete: expected the commit failure")
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "proxy-hosts", "keep.yaml")); err != nil {
		t.Fatalf("delete: the file of a failed commit must be put back: %v", err)
	}
	if _, _, err := st.UpdateSettings(ctx, func(s *model.Settings) error { s.AppName = "changed"; return nil }, Author{}); err == nil {
		t.Fatal("settings: expected the commit failure")
	}

	st.git = real
	cfg, settings, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "keep" {
		t.Fatalf("hosts after failed commits = %+v, want only keep", cfg.ProxyHosts)
	}
	if settings.AppName == "changed" {
		t.Fatal("settings: a failed commit must not leave the new settings on disk")
	}
	if clean, err := real.IsClean(ctx); err != nil || !clean {
		t.Fatalf("the working tree after failed commits must match HEAD (clean=%v err=%v)", clean, err)
	}
}

// TestGitIgnoresSystemConfig proves every git call runs without the system
// config: a system file that hides untracked files from status and drops diff
// context changes neither the clean check nor a patch.
func TestGitIgnoresSystemConfig(t *testing.T) {
	ctx := context.Background()
	sys := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(sys, []byte("[status]\n\tshowUntrackedFiles = no\n[diff]\n\tcontext = 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_SYSTEM", sys)

	dir := t.TempDir()
	g := NewExecGit(dir)
	if err := g.EnsureRepo(ctx); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clean, err := g.IsClean(ctx)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	if clean {
		t.Fatal("an untracked file must make the tree dirty whatever the system config says")
	}
	first, err := g.CommitAll(ctx, "first", Author{Name: "t", Email: "t@example.com"})
	if err != nil || first == "" {
		t.Fatalf("first commit: %q %v", first, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("one\nTWO\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := g.CommitAll(ctx, "second", Author{Name: "t", Email: "t@example.com"})
	if err != nil || second == "" {
		t.Fatalf("second commit: %q %v", second, err)
	}
	patch, err := g.Diff(ctx, first, second)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(patch, "\n one\n") || !strings.Contains(patch, "\n three\n") {
		t.Errorf("patch has no context lines:\n%s", patch)
	}
}
