// Package store implements the git-backed configuration store: the declarative
// per-object YAML files are the source of truth, every UI/API save is a commit,
// and history/rollback come straight from git. A SQLite cache (added when
// runtime state needs persistence) is always rebuildable from these files.
package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ErrPathNotInCommit is returned by RestorePath when the requested path does not
// exist in the target commit, so there is nothing to restore. A scoped revert
// maps this to a clear "object absent at that commit" error rather than silently
// deleting the object.
var ErrPathNotInCommit = errors.New("path not present in target commit")

// execEnv returns the process environment with git made deterministic and
// independent of any host-level/global git config.
func execEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		// Stable English messages: error classification (e.g. RestorePath's
		// pathspec-miss mapping) matches on git's output text.
		"LC_ALL=C",
	)
}

// Author identifies who made a change (the SSO/admin identity, or the CLI).
type Author struct {
	Name  string
	Email string
}

func (a Author) normalised() Author {
	if a.Name == "" {
		a.Name = "go-proxy-manager"
	}
	if a.Email == "" {
		a.Email = "gpm@localhost"
	}
	return a
}

// Commit is one entry from the git history of an object's file.
type Commit struct {
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Email   string    `json:"email"`
	Message string    `json:"message"`
	When    time.Time `json:"when"`
}

// GitRepo is the minimal git surface the config store needs. It is an interface
// so the exec-backed implementation can be swapped for a pure-Go one later
// without touching the store.
type GitRepo interface {
	// EnsureRepo initialises dir as a git repo if it is not one already.
	EnsureRepo(ctx context.Context) error
	// CommitAll stages every change under the repo and commits it. Returns the
	// new commit hash, or ("", nil) if there was nothing to commit.
	CommitAll(ctx context.Context, message string, author Author) (string, error)
	// Log returns up to limit commits touching path (relative to repo root);
	// an empty path returns repo-wide history.
	Log(ctx context.Context, path string, limit int) ([]Commit, error)
	// Head returns the current HEAD commit hash (empty for an unborn branch).
	Head(ctx context.Context) (string, error)
	// RestoreTree makes the index and working tree match treeish exactly (files
	// absent from treeish are removed). It does NOT create a commit. Used to
	// roll the config back to a past commit (the caller then commits the result)
	// and to undo a failed restore.
	RestoreTree(ctx context.Context, treeish string) error
	// RestorePath restores a single tracked path (relative to the repo root) in
	// the working tree and index from treeish, leaving every other file untouched.
	// It does NOT create a commit. The path is always passed after a "--"
	// separator so an untrusted value can never be read as a git option. Returns
	// ErrPathNotInCommit when the path does not exist in treeish.
	RestorePath(ctx context.Context, treeish, rel string) error
	// IsClean reports whether the working tree has no uncommitted changes.
	IsClean(ctx context.Context) (bool, error)
	// FetchRemote updates the remote-tracking refs from the configured remote.
	// It is the NETWORK half of a follower pull and touches neither the index
	// nor the working tree, so a caller can run it without holding a lock that
	// would otherwise be held for the whole round trip. It reports whether a
	// remote is configured at all (false = nothing to fetch, no error).
	FetchRemote(ctx context.Context) (bool, error)
	// MergeFFOnly fast-forwards the current branch onto its already-fetched
	// upstream; it never merges or rebases. This is the LOCAL half of a follower
	// pull. A diverged history is surfaced as an error for the caller to
	// resolve, never auto-merged or discarded.
	MergeFFOnly(ctx context.Context) error
}

// execGit shells out to the system git binary. Real git semantics, zero added
// Go dependency; behind GitRepo so it stays swappable.
type execGit struct {
	dir string

	// fetchMu serializes FetchRemote, which its caller deliberately runs without
	// the config-store lock (a network round trip must not block config reads).
	// It also guards hasRemote, the one-shot "a remote is configured" cache.
	fetchMu   sync.Mutex
	hasRemote bool
}

// NewExecGit returns a GitRepo backed by the git binary operating on dir.
func NewExecGit(dir string) GitRepo { return &execGit{dir: dir} }

func (g *execGit) run(ctx context.Context, env []string, args ...string) (string, error) {
	// Fixed argv (no shell): literal git binary, repo dir from server config,
	// args are internal literals from the GitRepo methods.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.dir}, args...)...) //noaikido
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (g *execGit) EnsureRepo(ctx context.Context) error {
	if out, err := g.run(ctx, nil, "rev-parse", "--is-inside-work-tree"); err == nil && out == "true" {
		return nil
	}
	if _, err := g.run(ctx, nil, "init", "-q"); err != nil {
		return err
	}
	// Ensure a committer identity exists locally so commits never fail on a
	// machine without global git config.
	_, _ = g.run(ctx, nil, "config", "user.name", "go-proxy-manager")
	_, _ = g.run(ctx, nil, "config", "user.email", "gpm@localhost")
	return nil
}

func (g *execGit) CommitAll(ctx context.Context, message string, author Author) (string, error) {
	if _, err := g.run(ctx, nil, "add", "-A"); err != nil {
		return "", err
	}
	clean, err := g.IsClean(ctx)
	if err != nil {
		return "", err
	}
	if clean {
		return "", nil // nothing to commit
	}
	a := author.normalised()
	// Author and committer set explicitly so the commit reflects the operator.
	env := append(execEnv(),
		"GIT_AUTHOR_NAME="+a.Name, "GIT_AUTHOR_EMAIL="+a.Email,
		"GIT_COMMITTER_NAME="+a.Name, "GIT_COMMITTER_EMAIL="+a.Email,
	)
	if _, err := g.run(ctx, env, "commit", "-q", "-m", message); err != nil {
		return "", err
	}
	return g.Head(ctx)
}

func (g *execGit) Log(ctx context.Context, path string, limit int) ([]Commit, error) {
	args := []string{"log", fmt.Sprintf("-n%d", limit), "--pretty=format:%H%x1f%an%x1f%ae%x1f%aI%x1f%s"}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := g.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x1f")
		if len(f) != 5 {
			continue
		}
		when, _ := time.Parse(time.RFC3339, f[3])
		commits = append(commits, Commit{Hash: f[0], Author: f[1], Email: f[2], When: when, Message: f[4]})
	}
	return commits, nil
}

func (g *execGit) Head(ctx context.Context) (string, error) {
	out, err := g.run(ctx, nil, "rev-parse", "HEAD")
	if err != nil {
		// Unborn branch (no commits yet) is not a hard error.
		if strings.Contains(err.Error(), "unknown revision") || strings.Contains(err.Error(), "ambiguous argument") {
			return "", nil
		}
		return "", err
	}
	return out, nil
}

func (g *execGit) RestoreTree(ctx context.Context, treeish string) error {
	// read-tree --reset -u sets the index AND working tree to match treeish,
	// removing tracked files that are not in it. No commit is made.
	if _, err := g.run(ctx, nil, "read-tree", "--reset", "-u", treeish); err != nil {
		return err
	}
	// read-tree leaves UNTRACKED files in place. A refused Restore writes the
	// uploaded archive's files to the working tree before staging (see
	// replaceConfigFiles); without this clean they would survive the rollback and
	// be picked up by the next disk load - e.g. a plaintext secret the restore was
	// refused for. clean -fd purges them so the working tree exactly matches
	// treeish (the hard-reset semantics this method promises).
	if _, err := g.run(ctx, nil, "clean", "-fd"); err != nil {
		return err
	}
	return nil
}

func (g *execGit) RestorePath(ctx context.Context, treeish, rel string) error {
	// checkout <treeish> -- <rel> rewrites only that path from the target commit;
	// the "--" separator makes rel a pathspec, never an option. If rel is not in
	// the commit, git fails with a "did not match" pathspec error, which we map to
	// the ErrPathNotInCommit sentinel so a scoped revert reports it cleanly.
	if _, err := g.run(ctx, execEnv(), "checkout", treeish, "--", rel); err != nil {
		if strings.Contains(err.Error(), "did not match") ||
			strings.Contains(err.Error(), "does not exist") {
			return fmt.Errorf("%q at %s: %w", rel, treeish, ErrPathNotInCommit)
		}
		return err
	}
	return nil
}

func (g *execGit) IsClean(ctx context.Context) (bool, error) {
	out, err := g.run(ctx, nil, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

func (g *execGit) FetchRemote(ctx context.Context) (bool, error) {
	g.fetchMu.Lock()
	defer g.fetchMu.Unlock()
	if !g.hasRemote {
		if out, _ := g.run(ctx, execEnv(), "remote"); strings.TrimSpace(out) == "" {
			return false, nil // no remote configured; nothing to fetch
		}
		g.hasRemote = true
	}
	// execEnv() is what keeps GIT_TERMINAL_PROMPT=0 in force: without it a remote
	// that asks for credentials blocks this git process on a terminal prompt that
	// nothing will ever answer, and the caller's ctx deadline is the only thing
	// left to stop it.
	if _, err := g.run(ctx, execEnv(), "fetch", "-q"); err != nil {
		return true, err
	}
	return true, nil
}

func (g *execGit) MergeFFOnly(ctx context.Context) error {
	if _, err := g.run(ctx, execEnv(), "merge", "--ff-only", "-q", "@{u}"); err != nil {
		return fmt.Errorf("config repo diverged from remote (refusing to auto-merge or discard): %w", err)
	}
	return nil
}
