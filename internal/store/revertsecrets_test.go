package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// A revert to a commit that carries a literal secret (written before the rule,
// or by hand) is refused, exactly as the write paths refuse it; the tree is
// left at HEAD.
func TestRevertRefusesLiteralSecrets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st := New(dir, NewExecGit(dir))
	if err := st.Init(ctx); err != nil {
		t.Fatal(err)
	}
	// Commit a literal secret behind the store's back.
	if err := os.MkdirAll(filepath.Join(dir, "dns-providers"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "name: cf\nprovider: cloudflare\nconfig:\n  apiToken: literal-token-value\n"
	if err := os.WriteFile(filepath.Join(dir, "dns-providers", "cf.yaml"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	bad, err := st.git.CommitAll(ctx, "hand-edit with a literal secret", Author{})
	if err != nil || bad == "" {
		t.Fatalf("commit raw: %q %v", bad, err)
	}
	// Then fix it through the store, which is the state HEAD must stay at.
	t.Setenv("CF_TOKEN", "x")
	good := model.DNSProvider{ObjectMeta: model.ObjectMeta{Name: "cf"}, Provider: model.DNSProviderCloudflare,
		Config: map[string]model.Secret{"apiToken": "${ENV:CF_TOKEN}"}}
	if _, err := st.Save(ctx, good, Author{}); err != nil {
		t.Fatalf("save placeholder version: %v", err)
	}
	head, _ := st.git.Head(ctx)

	if _, err := st.Revert(ctx, bad, Author{}); err == nil || !strings.Contains(err.Error(), "literal secret") {
		t.Fatalf("tree revert to a literal secret: %v, want a refusal naming the secret", err)
	}
	if _, err := st.RevertObject(ctx, "DNSProvider", "cf", bad, Author{}); err == nil || !strings.Contains(err.Error(), "literal secret") {
		t.Fatalf("object revert to a literal secret: %v, want a refusal naming the secret", err)
	}
	if now, _ := st.git.Head(ctx); now != head {
		t.Fatalf("HEAD moved from %s to %s on a refused revert", head, now)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.DNSProviders[0].Config["apiToken"]; got != "${ENV:CF_TOKEN}" {
		t.Fatalf("live config changed: %q", got)
	}
}
