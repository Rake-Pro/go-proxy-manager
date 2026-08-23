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
	"gopkg.in/yaml.v3"
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
