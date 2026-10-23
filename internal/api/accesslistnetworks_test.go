package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// GET /access-list-networks resolves each list to the networks its allow rules
// cover: literal cidr rules as written, a source rule as the ledger's fetched
// set (nothing until a fetch has landed), deny rules never.
func TestAccessListNetworks(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := api.New(api.Deps{Store: st})

	lan := `{"name":"lan","rules":[{"action":"allow","cidr":"10.0.0.0/8"},{"action":"deny","cidr":"10.9.0.0/16"},{"action":"allow","cidr":"192.0.2.10"}]}`
	if w := do(t, h, "PUT", "/access-lists/lan", lan); w.Code != http.StatusOK {
		t.Fatalf("seed lan: %d %s", w.Code, w.Body.String())
	}
	mon := `{"name":"monitor","sources":[{"name":"feed","url":"https://feed.example/ips.txt"}],"rules":[{"action":"allow","source":"feed","paths":["/status"]}]}`
	if w := do(t, h, "PUT", "/access-lists/monitor", mon); w.Code != http.StatusOK {
		t.Fatalf("seed monitor: %d %s", w.Code, w.Body.String())
	}

	get := func() map[string][]string {
		t.Helper()
		w := do(t, h, "GET", "/access-list-networks", "")
		if w.Code != http.StatusOK {
			t.Fatalf("networks: %d %s", w.Code, w.Body.String())
		}
		var out map[string][]string
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	want := map[string][]string{"lan": {"10.0.0.0/8", "192.0.2.10"}, "monitor": {}}
	if got := get(); !reflect.DeepEqual(got, want) {
		t.Fatalf("before fetch: got %v want %v", got, want)
	}

	entries := []string{"198.51.100.7/32", "203.0.113.0/24"}
	key := model.AccessListSourceKey("monitor", "feed")
	url := "https://feed.example/ips.txt"
	ledger := model.AccessListSourceLedger{SchemaVersion: model.SchemaVersion, Sources: []model.AccessListSourceEntry{{
		List: "monitor", Source: "feed", URL: url, FetchedAt: time.Now().UTC().Format(time.RFC3339),
		SHA256: model.AccessListSourceHash(key, url, entries), Entries: entries,
	}}}
	if _, err := st.SaveAccessListSourceLedger(context.Background(), ledger, store.Author{}, ""); err != nil {
		t.Fatal(err)
	}
	want["monitor"] = entries
	if got := get(); !reflect.DeepEqual(got, want) {
		t.Fatalf("after fetch: got %v want %v", got, want)
	}
}
