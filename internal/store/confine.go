package store

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrSymlinkEscapes is returned for a config file, or a directory on the way to
// it, that is a symbolic link resolving outside the config directory, or one
// that does not resolve at all (a missing target or a link loop). A link that
// resolves to a file inside the config directory (a Kubernetes ConfigMap or
// Secret volume's ..data indirection, for one) is read like a regular file.
var ErrSymlinkEscapes = errors.New("symbolic link does not resolve to a file inside the config directory")

// resolveInDir returns the real path p resolves to, compared against the real
// path of root (so a config directory reached through a link works). A path
// that resolves outside root, or a link that does not resolve, is refused with
// ErrSymlinkEscapes. A missing path that is not a link returns the os error
// unchanged, so callers keep their not-exist handling.
func resolveInDir(root, p string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", err
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(absP)
	if err != nil {
		if fi, lerr := os.Lstat(absP); lerr == nil && fi.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: the link target is missing or is a link loop", ErrSymlinkEscapes)
		}
		return "", err
	}
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: the link target %s is outside the config directory %s", ErrSymlinkEscapes, real, realRoot)
	}
	return real, nil
}

// readInDir reads the config file at p, refusing one that resolves outside
// root (see resolveInDir). The file is opened first and then checked to be the
// file its resolved path names, so a link swapped between the check and the
// open cannot redirect the read.
func readInDir(root, p string) ([]byte, error) {
	f, err := os.Open(p) //nolint:gosec // G304: callers pass store-built paths under root; the resolved location is checked below
	if err != nil {
		if _, rerr := resolveInDir(root, p); errors.Is(rerr, ErrSymlinkEscapes) {
			return nil, rerr
		}
		return nil, err
	}
	defer f.Close()
	real, err := resolveInDir(root, p)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	at, err := os.Stat(real)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(opened, at) {
		return nil, errors.New("the file changed while it was being read; retry")
	}
	return io.ReadAll(f)
}
