package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// maxRestoreBytes bounds an uploaded restore archive (uncompressed, per entry and
// in total) so a malformed or hostile archive cannot exhaust memory/disk.
const (
	maxRestoreBytes   = 16 << 20 // 16 MiB total uncompressed
	maxRestoreEntries = 10000
)

// Export packages the declarative config (every object YAML plus settings.yaml,
// excluding the .git history) into a portable gzip-compressed tar archive. The
// archive is self-contained and can be restored onto any gpm instance.
func (s *Store) Export(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	add := func(rel string, content []byte) error {
		hdr := &tar.Header{Name: rel, Mode: 0o640, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := tw.Write(content)
		return err
	}

	for _, files := range s.configFiles() {
		b, err := os.ReadFile(filepath.Join(s.dir, files))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if err := add(files, b); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// configFiles returns the relative paths of all config YAML files currently on
// disk (kind subdirectories + settings.yaml), sorted for deterministic archives.
func (s *Store) configFiles() []string {
	var rels []string
	if _, err := os.Stat(filepath.Join(s.dir, settingsFile)); err == nil {
		rels = append(rels, settingsFile)
	}
	for _, sub := range kindDir {
		entries, err := os.ReadDir(filepath.Join(s.dir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
				continue
			}
			rels = append(rels, sub+"/"+e.Name())
		}
	}
	sort.Strings(rels)
	return rels
}

// allowedRestorePath reports whether a tar entry path is a legal config file:
// either settings.yaml, or "<knownKindDir>/<name>.yaml". Anything else - absolute
// paths, traversal, unknown directories, the .git tree - is rejected so a restore
// cannot write outside the managed config layout.
func (s *Store) allowedRestorePath(name string) bool {
	clean := path.Clean(name)
	if clean != name || strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") {
		return false
	}
	if clean == settingsFile {
		return true
	}
	dir, file := path.Split(clean)
	dir = strings.TrimSuffix(dir, "/")
	if filepath.Ext(file) != ".yaml" {
		return false
	}
	for _, sub := range kindDir {
		if dir == sub {
			return true
		}
	}
	return false
}

// remapLegacyRestorePath rewrites an archive entry that names a retired kind
// directory (legacyKindDirs) onto the directory that replaced it, so an archive
// taken before the rename still restores. It is deliberately ONE-WAY and
// restore-only: gpm never writes the old name back out, and the on-disk tree is
// never migrated behind the operator (see checkLegacyKindDirs).
func remapLegacyRestorePath(name string) (string, bool) {
	for old, current := range legacyKindDirs {
		if rest, ok := strings.CutPrefix(name, old+"/"); ok {
			return current + "/" + rest, true
		}
	}
	return "", false
}

// Restore replaces the entire config with the contents of a gzip-tar archive
// produced by Export, validates the result, and commits it as one revision. If
// the archive yields an invalid config the working tree is rolled back to HEAD
// and an error is returned, so a bad restore never lands.
func (s *Store) Restore(ctx context.Context, archive []byte, author Author) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.readArchive(archive)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("restore: archive contains no config files")
	}

	head, err := s.git.Head(ctx)
	if err != nil {
		return "", err
	}

	if err := s.replaceConfigFiles(files); err != nil {
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", err
	}
	restored, restoredSettings, err := s.loadLocked()
	if err != nil {
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("restore refused, archive does not validate: %w", err)
	}
	if err := s.checkGeoAvailable(restored); err != nil {
		// The archive carries a geo rule with no database to evaluate it: undo
		// and refuse, so the fail-closed contract holds at the write boundary too.
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("restore refused: %w", err)
	}
	// Mirror the Save/SaveSettings no-literal-secrets guard at the restore
	// boundary: an uploaded archive must not smuggle a plaintext secret onto disk
	// and into permanent git history, bypassing the ${ENV:}/${FILE:} contract.
	lits := append(model.LiteralSecrets(restored), model.LiteralSecrets(restoredSettings)...)
	if len(lits) > 0 {
		if head != "" {
			_ = s.git.RestoreTree(ctx, head)
		}
		return "", fmt.Errorf("restore refused, archive carries literal secret(s): %v; use ${ENV:...} or ${FILE:...} placeholders", lits)
	}
	return s.git.CommitAll(ctx, "Restore configuration from archive", author)
}

// readArchive decompresses and reads the tar entries into a path->content map,
// enforcing the size/count/path-safety limits.
func (s *Store) readArchive(archive []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("restore: not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	files := map[string][]byte{}
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("restore: read archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue // skip directories and anything non-regular
		}
		entry := hdr.Name
		if mapped, ok := remapLegacyRestorePath(entry); ok {
			log.Info().Str("entry", entry).Str("restoredAs", mapped).
				Msg("restore: archive entry names a retired config directory; mapping it onto its replacement")
			entry = mapped
		}
		if !s.allowedRestorePath(entry) {
			return nil, fmt.Errorf("restore: archive entry %q is not an allowed config path", hdr.Name)
		}
		if len(files) >= maxRestoreEntries {
			return nil, fmt.Errorf("restore: archive has too many entries (> %d)", maxRestoreEntries)
		}
		total += hdr.Size
		if total > maxRestoreBytes {
			return nil, fmt.Errorf("restore: archive too large (> %d bytes uncompressed)", maxRestoreBytes)
		}
		b, err := io.ReadAll(io.LimitReader(tr, maxRestoreBytes))
		if err != nil {
			return nil, fmt.Errorf("restore: read entry %q: %w", hdr.Name, err)
		}
		files[path.Clean(entry)] = b
	}
	return files, nil
}

// replaceConfigFiles clears the existing config YAML files and writes the archive
// contents in their place. Operates on the working tree; the caller commits or
// rolls back.
func (s *Store) replaceConfigFiles(files map[string][]byte) error {
	for _, rel := range s.configFiles() {
		if err := os.Remove(filepath.Join(s.dir, rel)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for rel, content := range files {
		dst := filepath.Join(s.dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(dst, content, 0o640); err != nil {
			return err
		}
	}
	return nil
}
