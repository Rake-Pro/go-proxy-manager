package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
