package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st := New(dir, NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	return st
}

func sampleHost(name string) model.ProxyHost {
	return model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: name},
		Domains:    []string{name + ".example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.5", Port: 8080},
	}
}

func TestStoreInitAndLoadEmpty(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cfg, settings, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 0 {
		t.Fatalf("expected no hosts, got %d", len(cfg.ProxyHosts))
	}
	if !settings.AdminAuth.LocalLoginEnabled {
		t.Fatal("default settings should enable local login")
	}
	head, err := st.Head(ctx)
	if err != nil || head == "" {
		t.Fatalf("expected an initial commit, head=%q err=%v", head, err)
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	commit, err := st.Save(ctx, sampleHost("app"), Author{Name: "admin", Email: "admin@example.com"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if commit == "" {
		t.Fatal("expected a commit hash")
	}

	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "app" {
		t.Fatalf("round-trip failed: %+v", cfg.ProxyHosts)
	}
	if cfg.ProxyHosts[0].UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be stamped")
	}

	// File exists where expected.
	if _, err := os.Stat(filepath.Join(st.Dir(), "proxy-hosts", "app.yaml")); err != nil {
		t.Fatalf("expected object file: %v", err)
	}

	// History reflects the save commit.
	hist, err := st.History(ctx, "ProxyHost", "app", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) == 0 || hist[0].Author != "admin" {
		t.Fatalf("expected authored history, got %+v", hist)
	}
}

func TestStoreRejectsDanglingRef(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	bad := sampleHost("app")
	bad.TLS.CertificateRef = "does-not-exist"
	if _, err := st.Save(ctx, bad, Author{}); err == nil {
		t.Fatal("expected save to be rejected for dangling certificate ref")
	}

	// Nothing should have been committed beyond the initial commit.
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 0 {
		t.Fatalf("expected no hosts persisted, got %d", len(cfg.ProxyHosts))
	}
}
