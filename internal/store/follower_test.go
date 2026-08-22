package store

import (
	"context"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// haPair builds a leader store publishing to a bare repo, plus a follower store
// cloned from it - the phase-1 topology from docs/design/ha.md, on temp dirs.
func haPair(t *testing.T) (leader, follower *Store, push func()) {
	t.Helper()
	ctx := context.Background()
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare", "-q")

	leaderDir := t.TempDir()
	leader = New(leaderDir, NewExecGit(leaderDir))
	if err := leader.Init(ctx); err != nil {
		t.Fatalf("leader init: %v", err)
	}
	branch := runGit(t, leaderDir, "rev-parse", "--abbrev-ref", "HEAD")
	runGit(t, leaderDir, "remote", "add", "origin", bare)
	push = func() {
		t.Helper()
		runGit(t, leaderDir, "push", "-q", "origin", branch)
	}
	push()
	runGit(t, bare, "symbolic-ref", "HEAD", "refs/heads/"+branch)

	followerDir := t.TempDir()
	runGit(t, t.TempDir(), "clone", "-q", bare, followerDir)
	follower = New(followerDir, NewExecGit(followerDir))
	return leader, follower, push
}

func TestPullFFOnlyReportsWhetherHeadMoved(t *testing.T) {
	ctx := context.Background()
	leader, follower, push := haPair(t)

	moved, err := follower.PullFFOnly(ctx)
	if err != nil {
		t.Fatalf("pull with nothing new: %v", err)
	}
	if moved {
		t.Fatal("HEAD reported moved with nothing to pull")
	}

	if _, err := leader.Save(ctx, sampleHost("added-on-leader"), Author{}); err != nil {
		t.Fatalf("leader save: %v", err)
	}
	push()

	moved, err = follower.PullFFOnly(ctx)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if !moved {
		t.Fatal("HEAD did not report moved after the leader committed")
	}
	cfg, _, err := follower.Load(ctx)
	if err != nil {
		t.Fatalf("follower load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "added-on-leader" {
		t.Fatalf("follower did not pick up the leader's host: %+v", cfg.ProxyHosts)
	}

	// A second pull with nothing new must not report a change (that is what
	// keeps the follower from reloading on every tick).
	if moved, err = follower.PullFFOnly(ctx); err != nil || moved {
		t.Fatalf("second pull moved=%v err=%v, want false/nil", moved, err)
	}
}

// A follower that somehow acquired its own commit must surface the divergence,
// never merge, rebase or discard it.
func TestPullFFOnlyRefusesDivergedHistory(t *testing.T) {
	ctx := context.Background()
	leader, follower, push := haPair(t)

	if _, err := follower.Save(ctx, sampleHost("local-only"), Author{}); err != nil {
		t.Fatalf("follower save: %v", err)
	}
	localHead, _ := follower.Head(ctx)
	if _, err := leader.Save(ctx, sampleHost("on-leader"), Author{}); err != nil {
		t.Fatalf("leader save: %v", err)
	}
	push()

	moved, err := follower.PullFFOnly(ctx)
	if err == nil {
		t.Fatal("diverged history pulled without an error")
	}
	if moved {
		t.Fatal("diverged pull reported a moved HEAD")
	}
	if head, _ := follower.Head(ctx); head != localHead {
		t.Fatalf("diverged pull changed HEAD from %s to %s", localHead, head)
	}
}

// With no remote there is nothing to pull and nothing to report.
func TestPullFFOnlyWithoutRemote(t *testing.T) {
	s := newTestStore(t)
	moved, err := s.PullFFOnly(context.Background())
	if err != nil || moved {
		t.Fatalf("PullFFOnly without a remote = %v, %v; want false, nil", moved, err)
	}
}

func TestFollowRemoteReloadsOnlyWhenHeadMoves(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	leader, follower, push := haPair(t)

	var reloads atomic.Int64
	done := make(chan struct{})
	go func() {
		follower.FollowRemote(ctx, 5*time.Millisecond, func() error { reloads.Add(1); return nil })
		close(done)
	}()

	// Several ticks with nothing new must not reload.
	time.Sleep(60 * time.Millisecond)
	if n := reloads.Load(); n != 0 {
		t.Fatalf("reloaded %d times with no config change", n)
	}

	if _, err := leader.Save(ctx, sampleHost("new-host"), Author{}); err != nil {
		t.Fatalf("leader save: %v", err)
	}
	push()

	deadline := time.After(3 * time.Second)
	for reloads.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("follower never reloaded after the leader committed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	// ...and exactly once: further ticks over the same HEAD are no-ops.
	time.Sleep(60 * time.Millisecond)
	if n := reloads.Load(); n != 1 {
		t.Fatalf("reloaded %d times for one config change", n)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FollowRemote did not return after cancel")
	}
}
