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

// stubGit is a GitRepo whose FetchRemote is controllable, so a test can hold a
// "network" fetch open and observe what the store can still do meanwhile.
type stubGit struct {
	GitRepo
	fetchStarted chan struct{}
	releaseFetch chan struct{}
	fetchCtx     context.Context
	merged       atomic.Bool
}

func (g *stubGit) FetchRemote(ctx context.Context) (bool, error) {
	g.fetchCtx = ctx
	close(g.fetchStarted)
	select {
	case <-g.releaseFetch:
	case <-ctx.Done():
		return true, ctx.Err()
	}
	return true, nil
}

func (g *stubGit) MergeFFOnly(context.Context) error {
	g.merged.Store(true)
	return nil
}

// The network fetch must run WITHOUT the config write lock, and the whole pull
// must carry a deadline. Before the fix a hung remote held the store's write
// lock for as long as the peer kept the socket open, blocking every config read
// and every admin write behind it, with no timeout to end it.
func TestPullFFOnlyDoesNotHoldTheStoreLockDuringFetch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	real := NewExecGit(dir)
	s := New(dir, real)
	if err := s.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	g := &stubGit{GitRepo: real, fetchStarted: make(chan struct{}), releaseFetch: make(chan struct{})}
	s.git = g

	pullDone := make(chan error, 1)
	go func() {
		_, err := s.PullFFOnly(ctx)
		pullDone <- err
	}()

	select {
	case <-g.fetchStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("FetchRemote was never called")
	}

	// The fetch is in flight. A config read must not be blocked by it.
	loaded := make(chan error, 1)
	go func() {
		_, _, err := s.Load(ctx)
		loaded <- err
	}()
	select {
	case err := <-loaded:
		if err != nil {
			t.Fatalf("load during fetch: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a config read blocked on an in-flight network fetch: the write lock is still held across it")
	}
	if g.merged.Load() {
		t.Fatal("the fast-forward ran before the fetch completed")
	}

	// The pull is bounded: the context handed to git carries a deadline no
	// further out than PullTimeout.
	dl, ok := g.fetchCtx.Deadline()
	if !ok {
		t.Fatal("the fetch context has no deadline; a hung remote would run forever")
	}
	if budget := time.Until(dl); budget > PullTimeout {
		t.Fatalf("fetch deadline is %s out, want <= %s", budget, PullTimeout)
	}

	close(g.releaseFetch)
	select {
	case err := <-pullDone:
		if err != nil {
			t.Fatalf("pull: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PullFFOnly never returned")
	}
	if !g.merged.Load() {
		t.Fatal("the fast-forward never ran after a successful fetch")
	}
}

// A remote that never answers must not hang the pull forever: the deadline on
// the pull context is what ends it.
func TestPullFFOnlyTimesOutOnAHungRemote(t *testing.T) {
	dir := t.TempDir()
	real := NewExecGit(dir)
	s := New(dir, real)
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	g := &stubGit{GitRepo: real, fetchStarted: make(chan struct{}), releaseFetch: make(chan struct{})}
	s.git = g
	// A caller-supplied deadline shorter than PullTimeout still wins, which is
	// what makes this assertion fast; the point is that the fetch observes one.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := s.PullFFOnly(ctx); err == nil {
		t.Fatal("a hung fetch returned no error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the hung fetch took %s to give up", elapsed)
	}
	if g.merged.Load() {
		t.Fatal("a failed fetch must not be followed by a fast-forward")
	}
}
