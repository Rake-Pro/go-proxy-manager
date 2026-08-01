package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func sampleCert(name string) model.Certificate {
	return model.Certificate{
		ObjectMeta: model.ObjectMeta{Name: name},
		Type:       model.CertTypeCustom,
		Domains:    []string{name + ".example.com"},
		Custom:     &model.CustomCertSpec{CertFile: name + ".pem", KeyFile: name + "-key.pem"},
	}
}

// TestRevertObjectRestoresOnlyTarget is the exact incident scenario (2026-07-16):
// reverting one proxy host to an earlier commit must restore ONLY that host's
// file and leave every object created after that commit intact - unlike the
// whole-tree Revert, which would wipe the newer objects.
func TestRevertObjectRestoresOnlyTarget(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	v1 := sampleHost("web")
	v1.Upstream.Port = 8080
	target, err := st.Save(ctx, v1, Author{})
	if err != nil {
		t.Fatalf("save v1: %v", err)
	}

	v2 := sampleHost("web")
	v2.Upstream.Port = 9090
	if _, err := st.Save(ctx, v2, Author{}); err != nil {
		t.Fatalf("save v2: %v", err)
	}

	// Objects created AFTER the target commit - these must survive a scoped revert.
	for _, n := range []string{"c1", "c2", "c3"} {
		if _, err := st.Save(ctx, sampleCert(n), Author{}); err != nil {
			t.Fatalf("save cert %s: %v", n, err)
		}
	}

	if _, err := st.RevertObject(ctx, "ProxyHost", "web", target, Author{}); err != nil {
		t.Fatalf("revert object: %v", err)
	}

	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Upstream.Port != 8080 {
		t.Fatalf("host not reverted to v1: %+v", cfg.ProxyHosts)
	}
	if len(cfg.Certificates) != 3 {
		t.Fatalf("scoped revert wiped newer objects: want 3 certs, got %d", len(cfg.Certificates))
	}
}

func TestRevertObjectRejectsBadHash(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.RevertObject(context.Background(), "ProxyHost", "web", "../nope", Author{}); err == nil {
		t.Fatal("expected bad hash to be rejected")
	}
}

func TestRevertObjectRejectsUnknownKind(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.RevertObject(context.Background(), "Bogus", "web", "abc1234", Author{}); err == nil {
		t.Fatal("expected unknown kind to be rejected")
	}
}

func TestRevertObjectRejectsTraversalName(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.RevertObject(context.Background(), "ProxyHost", "../../etc/x", "abc1234", Author{}); err == nil {
		t.Fatal("expected traversal name to be rejected")
	}
}

// TestRevertObjectAbsentAtCommit: reverting to a commit where the object's file
// does not exist yet is refused with ErrPathNotInCommit (a scoped revert never
// recreates a deletion / never resurrects a not-yet-created object).
func TestRevertObjectAbsentAtCommit(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	early, err := st.Save(ctx, sampleHost("first"), Author{})
	if err != nil {
		t.Fatalf("save first: %v", err)
	}
	if _, err := st.Save(ctx, sampleHost("later"), Author{}); err != nil {
		t.Fatalf("save later: %v", err)
	}
	// "later" did not exist at the "early" commit.
	_, err = st.RevertObject(ctx, "ProxyHost", "later", early, Author{})
	if err == nil {
		t.Fatal("expected error reverting an object absent at the target commit")
	}
	if !errors.Is(err, ErrPathNotInCommit) {
		t.Fatalf("want ErrPathNotInCommit, got %v", err)
	}
}

// TestRevertObjectRollsBackOnInvalid: if restoring the single file leaves the
// whole config invalid (a dangling reference), the working tree is rolled back
// to HEAD and nothing is committed.
func TestRevertObjectRollsBackOnInvalid(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.Save(ctx, sampleCert("c1"), Author{}); err != nil {
		t.Fatalf("save cert: %v", err)
	}
	withRef := sampleHost("web")
	withRef.TLS.CertificateRef = "c1"
	target, err := st.Save(ctx, withRef, Author{})
	if err != nil {
		t.Fatalf("save host w/ ref: %v", err)
	}
	// Drop the reference, then remove the cert so the target version is now invalid.
	if _, err := st.Save(ctx, sampleHost("web"), Author{}); err != nil {
		t.Fatalf("save host w/o ref: %v", err)
	}
	if _, err := st.Delete(ctx, "Certificate", "c1", Author{}); err != nil {
		t.Fatalf("delete cert: %v", err)
	}

	head, _ := st.Head(ctx)
	if _, err := st.RevertObject(ctx, "ProxyHost", "web", target, Author{}); err == nil {
		t.Fatal("expected revert to be refused when the restored file dangles")
	}
	if after, _ := st.Head(ctx); after != head {
		t.Fatalf("nothing must be committed on refused revert: head moved %q -> %q", head, after)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load after rollback: %v", err)
	}
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].TLS.CertificateRef != "" {
		t.Fatalf("working tree not rolled back cleanly: %+v", cfg.ProxyHosts)
	}
	// The rollback must leave no staged-but-uncommitted state behind (the
	// checkout stages the restored file into the index, not just the worktree).
	clean, err := st.git.IsClean(ctx)
	if err != nil {
		t.Fatalf("IsClean after rollback: %v", err)
	}
	if !clean {
		t.Fatal("index/worktree dirty after refused revert; rollback incomplete")
	}
}

func sampleToken(name, hash string) model.APIToken {
	return model.APIToken{
		ObjectMeta: model.ObjectMeta{Name: name},
		TokenHash:  hash,
		Scopes:     []string{"proxy-hosts:read"},
	}
}

// The tokenHash field is json:"-" so it never leaves through the API, but it is
// still what has to persist: the YAML round trip must carry it unchanged.
func TestAPITokenHashRoundTrips(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const hash = "1111111111111111111111111111111111111111111111111111111111111111"
	if _, err := st.Save(ctx, sampleToken("ci", hash), Author{}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.APITokens) != 1 {
		t.Fatalf("tokens = %+v", cfg.APITokens)
	}
	if cfg.APITokens[0].TokenHash != hash {
		t.Fatalf("tokenHash = %q, want %q (at-rest persistence must be unchanged)", cfg.APITokens[0].TokenHash, hash)
	}
	b, err := os.ReadFile(filepath.Join(st.Dir(), "api-tokens", "ci.yaml"))
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if !strings.Contains(string(b), "tokenHash:") || !strings.Contains(string(b), hash) {
		t.Fatalf("token file does not carry the digest:\n%s", b)
	}
}

// Rotation has to mean revocation: a scoped revert of an APIToken would restore
// an older tokenHash and silently revive a secret the operator rotated away.
func TestRevertObjectRefusedForAPIToken(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const oldHash = "2222222222222222222222222222222222222222222222222222222222222222"
	const newHash = "3333333333333333333333333333333333333333333333333333333333333333"
	before, err := st.Save(ctx, sampleToken("ci", oldHash), Author{})
	if err != nil {
		t.Fatalf("save token: %v", err)
	}
	if _, err := st.Save(ctx, sampleToken("ci", newHash), Author{}); err != nil {
		t.Fatalf("rotate token: %v", err)
	}

	_, err = st.RevertObject(ctx, "APIToken", "ci", before, Author{})
	if err == nil {
		t.Fatal("reverting an API token must be refused")
	}
	if !errors.Is(err, ErrNotRevertible) {
		t.Fatalf("want ErrNotRevertible, got %v", err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.APITokens[0].TokenHash != newHash {
		t.Fatalf("tokenHash = %q, want the rotated-to digest %q", cfg.APITokens[0].TokenHash, newHash)
	}
}

// A whole-tree revert rolls everything else back but must leave api-tokens
// exactly as they are: neither reviving a rotated digest nor resurrecting a
// token that has since been deleted.
func TestRevertPreservesAPITokens(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const oldHash = "4444444444444444444444444444444444444444444444444444444444444444"
	const newHash = "5555555555555555555555555555555555555555555555555555555555555555"

	if _, err := st.Save(ctx, sampleToken("ci", oldHash), Author{}); err != nil {
		t.Fatalf("save ci: %v", err)
	}
	if _, err := st.Save(ctx, sampleToken("legacy", oldHash), Author{}); err != nil {
		t.Fatalf("save legacy: %v", err)
	}
	target, err := st.Save(ctx, sampleHost("web"), Author{})
	if err != nil {
		t.Fatalf("save host: %v", err)
	}

	// After the revert target: ci is rotated, legacy is deleted, a new host lands.
	if _, err := st.Save(ctx, sampleToken("ci", newHash), Author{}); err != nil {
		t.Fatalf("rotate ci: %v", err)
	}
	if _, err := st.Delete(ctx, "APIToken", "legacy", Author{}); err != nil {
		t.Fatalf("delete legacy: %v", err)
	}
	if _, err := st.Save(ctx, sampleHost("later"), Author{}); err != nil {
		t.Fatalf("save later host: %v", err)
	}

	if _, err := st.Revert(ctx, target, Author{}); err != nil {
		t.Fatalf("revert: %v", err)
	}

	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("load after revert: %v", err)
	}
	// Everything else really did roll back.
	if len(cfg.ProxyHosts) != 1 || cfg.ProxyHosts[0].Name != "web" {
		t.Fatalf("hosts = %+v, want only web (the revert must still roll other kinds back)", cfg.ProxyHosts)
	}
	if len(cfg.APITokens) != 1 {
		t.Fatalf("tokens = %+v, want only ci (a deleted token must not be resurrected)", cfg.APITokens)
	}
	if cfg.APITokens[0].Name != "ci" || cfg.APITokens[0].TokenHash != newHash {
		t.Fatalf("token = %+v, want ci with the rotated-to digest %q", cfg.APITokens[0], newHash)
	}

	// The preserved state is committed, not just sitting in the working tree.
	clean, err := st.git.IsClean(ctx)
	if err != nil {
		t.Fatalf("is clean: %v", err)
	}
	if !clean {
		t.Fatal("the revert commit must include the preserved api-tokens")
	}
}

// ApplyBatch is the Ingress-discovery reconciler's write primitive: one commit
// for every upsert AND every delete in a run.
func TestApplyBatchUpsertsAndDeletesInOneCommit(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// Seed two hosts through the ordinary path.
	for _, name := range []string{"keep", "drop"} {
		if _, err := s.Save(ctx, model.ProxyHost{
			ObjectMeta: model.ObjectMeta{Name: name},
			Domains:    []string{name + ".example.com"},
			Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.1", Port: 80},
		}, Author{}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	before, err := s.RepoHistory(ctx, 100)
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	sha, err := s.ApplyBatch(ctx,
		[]model.Object{
			model.ProxyHost{
				ObjectMeta: model.ObjectMeta{Name: "added"},
				Domains:    []string{"added.example.com"},
				Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.2", Port: 80},
			},
			model.ProxyHost{
				ObjectMeta: model.ObjectMeta{Name: "keep"},
				Domains:    []string{"keep2.example.com"},
				Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.1", Port: 80},
			},
		},
		[]ObjectRef{{Kind: "ProxyHost", Name: "drop"}},
		"Ingress discovery: reconcile (+1 ~1 -1)", Author{})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	if sha == "" {
		t.Fatal("ApplyBatch must return the new commit")
	}

	after, err := s.RepoHistory(ctx, 100)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("batch produced %d commits, want exactly 1", len(after)-len(before))
	}
	if after[0].Message != "Ingress discovery: reconcile (+1 ~1 -1)" {
		t.Fatalf("commit message = %q", after[0].Message)
	}

	cfg, _, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	names := map[string]string{}
	for _, h := range cfg.ProxyHosts {
		names[h.Name] = strings.Join(h.Domains, ",")
	}
	if _, gone := names["drop"]; gone {
		t.Fatal("the deleted host is still present")
	}
	if names["added"] != "added.example.com" || names["keep"] != "keep2.example.com" {
		t.Fatalf("post-batch hosts = %v", names)
	}
}

// A steady-state reconcile must not produce empty revisions.
func TestApplyBatchEmptyIsANoOp(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	before, _ := s.RepoHistory(ctx, 100)
	sha, err := s.ApplyBatch(ctx, nil, nil, "nothing", Author{})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	if sha != "" {
		t.Fatalf("an empty batch must not commit, got %q", sha)
	}
	after, _ := s.RepoHistory(ctx, 100)
	if len(after) != len(before) {
		t.Fatalf("an empty batch committed %d revisions", len(after)-len(before))
	}
}

// The batch is all-or-nothing: an invalid member leaves the whole tree untouched.
func TestApplyBatchRejectsInvalidWithoutWriting(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	dir := s.Dir()
	head, _ := s.Head(ctx)

	_, err := s.ApplyBatch(ctx, []model.Object{
		model.ProxyHost{
			ObjectMeta: model.ObjectMeta{Name: "good"},
			Domains:    []string{"good.example.com"},
			Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.1", Port: 80},
		},
		model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "bad"}}, // no domains, no upstream
	}, nil, "batch", Author{})
	if err == nil {
		t.Fatal("an invalid member must fail the batch")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "proxy-hosts", "good.yaml")); statErr == nil {
		t.Fatal("a rejected batch must write nothing at all")
	}
	if now, _ := s.Head(ctx); now != head {
		t.Fatal("a rejected batch must not commit")
	}
}

// A batch that would leave a dangling reference is refused, exactly like Delete.
func TestApplyBatchRefusesDanglingDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Save(ctx, sampleCert("wildcard"), Author{}); err != nil {
		t.Fatalf("seed certificate: %v", err)
	}
	if _, err := s.Save(ctx, model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.1", Port: 80},
		TLS:        model.TLSSettings{CertificateRef: "wildcard"},
	}, Author{}); err != nil {
		t.Fatalf("seed host: %v", err)
	}
	if _, err := s.ApplyBatch(ctx, nil, []ObjectRef{{Kind: "Certificate", Name: "wildcard"}}, "drop cert", Author{}); err == nil {
		t.Fatal("deleting a referenced certificate in a batch must be refused")
	}
	cfg, _, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatal("the refused delete must have left the certificate in place")
	}
}

// The reconciler works from a snapshot, so an object already removed underneath
// it is the desired end state, not an error.
func TestApplyBatchIgnoresAbsentDeletes(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	before, _ := s.RepoHistory(ctx, 100)
	sha, err := s.ApplyBatch(ctx, nil, []ObjectRef{{Kind: "ProxyHost", Name: "never-existed"}}, "drop", Author{})
	if err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	if sha != "" {
		t.Fatal("deleting nothing must not commit")
	}
	after, _ := s.RepoHistory(ctx, 100)
	if len(after) != len(before) {
		t.Fatal("deleting an absent object must not produce a revision")
	}
}

func TestApplyBatchRejectsUnknownKindAndBadName(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.ApplyBatch(ctx, nil, []ObjectRef{{Kind: "Nonsense", Name: "x"}}, "m", Author{}); err == nil {
		t.Fatal("an unknown kind must be refused")
	}
	if _, err := s.ApplyBatch(ctx, nil, []ObjectRef{{Kind: "ProxyHost", Name: "../escape"}}, "m", Author{}); err == nil {
		t.Fatal("a name that is not a valid object name must be refused")
	}
}
