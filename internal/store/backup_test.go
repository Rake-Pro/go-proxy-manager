package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"testing"
)

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
