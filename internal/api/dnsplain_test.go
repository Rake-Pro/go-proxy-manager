package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

var plainDNSProviders = map[string]string{
	"ns":   `{"name":"ns","provider":"rfc2136","config":{"server":"ns1.example.com:53","zone":"example.com","tsigKeyName":"gpm-key","tsigSecret":"${ENV:GPM_TSIG_SECRET}"}}`,
	"adns": `{"name":"adns","provider":"acme-dns","config":{"baseURL":"https://acme-dns.example.com","subdomain":"d420c923","username":"${ENV:ADNS_USER}","password":"${ENV:ADNS_PASSWORD}"}}`,
	"hook": `{"name":"hook","provider":"webhook","config":{"url":"http://127.0.0.1:9000/dns","allowInsecureLocal":"true","token":"${ENV:HOOK_TOKEN}"}}`,
}

func providerConfig(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var got struct {
		Config map[string]string `json:"config"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return got.Config
}

// TestDNSProviderLiteralPlainKeysOverTheAPI: an admin can save literal
// non-secret settings, every read returns them as written (to a reader below
// admin too), secrets stay placeholders or "***", and a GET -> PUT round trip
// (the UI's edit flow) saves again unchanged.
func TestDNSProviderLiteralPlainKeysOverTheAPI(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	admin := api.New(api.Deps{Store: st, OnChange: func() error { return nil }})
	viewer := api.New(api.Deps{Store: st, OnChange: func() error { return nil }, RequireScope: func(_ *http.Request, required string) error {
		if required == model.ScopeAdmin || strings.HasSuffix(required, ":write") {
			return errors.New("read-only")
		}
		return nil
	}})

	for name, body := range plainDNSProviders {
		if w := do(t, admin, "PUT", "/dns-providers/"+name+"?dryRun=true", body); w.Code != http.StatusOK {
			t.Fatalf("dry run %s: %d %s", name, w.Code, w.Body.String())
		}
		if w := do(t, admin, "PUT", "/dns-providers/"+name, body); w.Code != http.StatusOK {
			t.Fatalf("PUT %s: %d %s", name, w.Code, w.Body.String())
		}
	}

	for _, h := range []http.Handler{admin, viewer} {
		for name, body := range plainDNSProviders {
			want := providerConfig(t, []byte(body))
			w := do(t, h, "GET", "/dns-providers/"+name, "")
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s: %d %s", name, w.Code, w.Body.String())
			}
			if got := providerConfig(t, w.Body.Bytes()); len(got) != len(want) {
				t.Errorf("GET %s config = %v, want %v", name, got, want)
			} else {
				for k, v := range want {
					if got[k] != v {
						t.Errorf("GET %s %s = %q, want %q", name, k, got[k], v)
					}
				}
			}
		}
	}
	list := do(t, viewer, "GET", "/dns-providers", "").Body.String()
	if !strings.Contains(list, `"server": "ns1.example.com:53"`) || !strings.Contains(list, `"url": "http://127.0.0.1:9000/dns"`) {
		t.Errorf("GET /dns-providers = %s, want the plain values as written", list)
	}
	if cfg := do(t, admin, "GET", "/config", "").Body.String(); !strings.Contains(cfg, `"baseURL": "https://acme-dns.example.com"`) {
		t.Errorf("GET /config lacks the literal baseURL: %s", cfg)
	}

	// The UI's edit flow: what GET returns, PUT back, is accepted unchanged.
	got := do(t, admin, "GET", "/dns-providers/hook", "").Body.String()
	if w := do(t, admin, "PUT", "/dns-providers/hook", got); w.Code != http.StatusOK {
		t.Errorf("GET -> PUT round trip: %d %s", w.Code, w.Body.String())
	}

	// YAML import of the same provider.
	parse := httpDoYAML(t, admin, "/dns-providers/_parse", "name: hook2\nprovider: webhook\nconfig:\n  url: http://127.0.0.1:9000/dns\n  allowInsecureLocal: \"true\"\n  token: ${ENV:HOOK_TOKEN}\n")
	if !strings.Contains(parse, `"url": "http://127.0.0.1:9000/dns"`) || !strings.Contains(parse, `"valid": true`) {
		t.Errorf("_parse = %s, want the literal url as written and valid", parse)
	}

	// Literal secrets are still refused on write.
	for name, body := range map[string]string{
		"ns":   `{"name":"ns","provider":"rfc2136","config":{"server":"ns1.example.com","zone":"example.com","tsigKeyName":"k","tsigSecret":"c2VjcmV0"}}`,
		"hook": `{"name":"hook","provider":"webhook","config":{"url":"https://hook.example.com/dns","token":"literal-token"}}`,
	} {
		if w := do(t, admin, "PUT", "/dns-providers/"+name, body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "refusing to commit literal secret") {
			t.Errorf("PUT %s with a literal secret: %d %s, want 400", name, w.Code, w.Body.String())
		}
	}

	// A literal secret that reached the file by hand is still shown as "***".
	if err := os.WriteFile(filepath.Join(dir, "dns-providers", "hand.yaml"), []byte("name: hand\nprovider: cloudflare\nconfig:\n  apiToken: literal-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := providerConfig(t, do(t, viewer, "GET", "/dns-providers/hand", "").Body.Bytes()); got["apiToken"] != "***" {
		t.Errorf("hand-edited literal apiToken = %q, want ***", got["apiToken"])
	}
}

func httpDoYAML(t *testing.T, h http.Handler, path, body string) string {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/yaml")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Body.String()
}
