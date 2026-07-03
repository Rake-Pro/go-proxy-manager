package store

import (
	"context"
	"errors"
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

func sampleIdP(name string, secret model.Secret) model.IdentityProvider {
	return model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: name},
		Type:       model.IdPTypeOIDC,
		OIDC: &model.OIDCSpec{
			IssuerURL:    "https://idp.example.com",
			ClientID:     "gpm",
			ClientSecret: secret,
		},
	}
}

func TestStoreRejectsLiteralSecret(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.Save(ctx, sampleIdP("idp", model.Secret("plaintext-secret")), Author{}); err == nil {
		t.Fatal("expected save to be rejected for literal secret")
	}

	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.IdentityProviders) != 0 {
		t.Fatalf("expected no idp persisted, got %d", len(cfg.IdentityProviders))
	}
}

func TestStoreAcceptsPlaceholderSecret(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.Save(ctx, sampleIdP("idp", model.Secret("${ENV:OIDC_CLIENT_SECRET}")), Author{}); err != nil {
		t.Fatalf("expected placeholder secret to be accepted, got %v", err)
	}

	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.IdentityProviders) != 1 {
		t.Fatalf("expected 1 idp persisted, got %d", len(cfg.IdentityProviders))
	}
}

func TestStoreDeleteRejectsTraversal(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	victim := filepath.Join(filepath.Dir(st.Dir()), "victim.yaml")
	if err := os.WriteFile(victim, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed victim: %v", err)
	}
	if _, err := st.Delete(ctx, "ProxyHost", "../../victim", Author{}); err == nil {
		t.Fatal("expected delete to reject traversal name")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("victim file outside dir was touched: %v", err)
	}
}

func TestStoreHistoryRejectsTraversal(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.History(context.Background(), "ProxyHost", "../../etc/x", 10); err == nil {
		t.Fatal("expected history to reject traversal name")
	}
}

func geoAccessList(name string) model.AccessList {
	return model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: name},
		Geo:        &model.AccessListGeo{CountryDeny: []string{"CN"}},
	}
}

// TestSaveRejectsGeoRuleWithoutDB is the reject-at-write gate: with the geo
// predicate reporting "no database", a Save of an AccessList carrying geo rules
// must be refused (ErrGeoDBUnavailable) and commit NOTHING - HEAD is unchanged.
func TestSaveRejectsGeoRuleWithoutDB(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	st.SetGeoAvailability(func() bool { return false }) // no DB loaded

	head, _ := st.Head(ctx)

	_, err := st.Save(ctx, geoAccessList("geoblock"), Author{})
	if err == nil {
		t.Fatal("Save must refuse a geo rule while no GeoIP database is loaded")
	}
	if !errors.Is(err, ErrGeoDBUnavailable) {
		t.Fatalf("want ErrGeoDBUnavailable, got %v", err)
	}
	if after, _ := st.Head(ctx); after != head {
		t.Fatalf("nothing must be committed on reject: head moved %q -> %q", head, after)
	}
}

// TestSaveAllowsGeoRuleWithDB proves the gate does not over-reach: with the
// predicate reporting a loaded database, the identical geo Save succeeds.
func TestSaveAllowsGeoRuleWithDB(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	st.SetGeoAvailability(func() bool { return true }) // DB loaded

	if _, err := st.Save(ctx, geoAccessList("geoblock"), Author{}); err != nil {
		t.Fatalf("Save of a geo rule with a loaded DB should succeed: %v", err)
	}
}

// TestSaveGeoRuleNoPredicateSkipsGate confirms an unwired store (nil predicate,
// e.g. the CLI importer) is unaffected by the gate.
func TestSaveGeoRuleNoPredicateSkipsGate(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Save(context.Background(), geoAccessList("geoblock"), Author{}); err != nil {
		t.Fatalf("with no geo predicate wired the gate must be a no-op: %v", err)
	}
}
