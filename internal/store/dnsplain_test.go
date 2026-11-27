package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// plainProviders are DNS providers whose non-secret settings are literal and
// whose credentials are placeholders: the shape every write path must accept.
func plainProviders() []model.DNSProvider {
	return []model.DNSProvider{
		{ObjectMeta: model.ObjectMeta{Name: "ns"}, Provider: model.DNSProviderRFC2136, Config: map[string]model.Secret{
			"server": "ns1.example.com:53", "zone": "example.com", "tsigKeyName": "gpm-key", "tsigSecret": "${ENV:GPM_TSIG_SECRET}",
		}},
		{ObjectMeta: model.ObjectMeta{Name: "adns"}, Provider: model.DNSProviderACMEDNS, Config: map[string]model.Secret{
			"baseURL": "https://acme-dns.example.com", "subdomain": "d420c923", "username": "${ENV:ADNS_USER}", "password": "${ENV:ADNS_PASSWORD}",
		}},
		{ObjectMeta: model.ObjectMeta{Name: "hook"}, Provider: model.DNSProviderWebhook, Config: map[string]model.Secret{
			"url": "http://127.0.0.1:9000/dns", "allowInsecureLocal": "true", "token": "${ENV:HOOK_TOKEN}",
		}},
	}
}

func TestStoreSavesLiteralPlainDNSKeys(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	for _, p := range plainProviders() {
		if _, _, _, err := st.DryRun(ctx, p); err != nil {
			t.Errorf("DryRun %s: %v", p.Name, err)
		}
		if _, err := st.Save(ctx, p, Author{Name: "admin"}); err != nil {
			t.Fatalf("Save %s: %v", p.Name, err)
		}
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DNSProviders) != 3 {
		t.Fatalf("stored providers = %d, want 3", len(cfg.DNSProviders))
	}
	for _, p := range cfg.DNSProviders {
		for _, want := range plainProviders() {
			if want.Name != p.Name {
				continue
			}
			for k, v := range want.Config {
				if p.Config[k] != v {
					t.Errorf("%s %s = %q, want %q as written", p.Name, k, p.Config[k], v)
				}
			}
		}
	}
	// The file at rest holds the literal as written.
	b, err := os.ReadFile(filepath.Join(st.dir, "dns-providers", "hook.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "url: http://127.0.0.1:9000/dns") || !strings.Contains(string(b), `allowInsecureLocal: "true"`) {
		t.Errorf("hook.yaml = %s", b)
	}

	// Batch writes and backup/restore take the same rule.
	batch := plainProviders()
	for i := range batch {
		batch[i].Name += "-b"
	}
	if _, err := st.SaveBatch(ctx, []model.Object{batch[0], batch[1], batch[2]}, "batch", Author{Name: "admin"}); err != nil {
		t.Fatalf("SaveBatch: %v", err)
	}
	archive, err := st.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Restore(ctx, archive, Author{Name: "admin"}); err != nil {
		t.Fatalf("Restore of an archive with literal plain keys: %v", err)
	}
}

func TestStoreRefusesLiteralDNSSecrets(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cases := []struct {
		p    model.DNSProvider
		want string
	}{
		{model.DNSProvider{ObjectMeta: model.ObjectMeta{Name: "ns"}, Provider: model.DNSProviderRFC2136, Config: map[string]model.Secret{
			"server": "ns1.example.com", "zone": "example.com", "tsigKeyName": "k", "tsigSecret": "c2VjcmV0",
		}}, "Config[tsigSecret]"},
		{model.DNSProvider{ObjectMeta: model.ObjectMeta{Name: "adns"}, Provider: model.DNSProviderACMEDNS, Config: map[string]model.Secret{
			"baseURL": "https://acme-dns.example.com", "subdomain": "s", "username": "${ENV:U}", "password": "pw",
		}}, "Config[password]"},
		{model.DNSProvider{ObjectMeta: model.ObjectMeta{Name: "hook"}, Provider: model.DNSProviderWebhook, Config: map[string]model.Secret{
			"url": "https://hook.example.com/dns", "token": "literal-token",
		}}, "Config[token]"},
		{model.DNSProvider{ObjectMeta: model.ObjectMeta{Name: "cf"}, Provider: model.DNSProviderCloudflare, Config: map[string]model.Secret{
			"apiToken": "literal-token",
		}}, "Config[apiToken]"},
	}
	for _, tc := range cases {
		_, err := st.Save(ctx, tc.p, Author{Name: "admin"})
		if err == nil || !strings.Contains(err.Error(), "refusing to commit literal secret(s)") || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Save err = %v, want the literal %s refused", tc.p.Name, err, tc.want)
		}
		if _, _, _, err := st.DryRun(ctx, tc.p); err == nil {
			t.Errorf("%s: DryRun accepted a literal secret", tc.p.Name)
		}
		if _, err := st.SaveBatch(ctx, []model.Object{tc.p}, "batch", Author{Name: "admin"}); err == nil {
			t.Errorf("%s: SaveBatch accepted a literal secret", tc.p.Name)
		}
	}
}
