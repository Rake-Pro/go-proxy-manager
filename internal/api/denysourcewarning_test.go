package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// A deny rule whose source has no fetched set matches nothing. GET /health,
// which feeds the UI's Needs attention list, names it in configWarnings, and
// stops once the source has a set.
func TestHealthWarnsOnUnresolvedDenySource(t *testing.T) {
	const url = "https://feed.example.com/block.txt"
	key := model.AccessListSourceKey("block", "feed")
	now := time.Now().UTC().Format(time.RFC3339)
	entries := []string{"198.51.100.0/24"}
	fetched := model.AccessListSourceEntry{List: "block", Source: "feed", URL: url, FetchedAt: now,
		SHA256: model.AccessListSourceHash(key, url, entries), Entries: entries}
	expired := model.AccessListSourceEntry{List: "block", Source: "feed", URL: url, FetchedAt: now, ExpiredAt: now,
		SHA256: model.AccessListSourceHash(key, url, nil)}
	// Stored before the entry guard refused RFC1918 ranges: the data plane
	// drops it on compile.
	private := []string{"10.0.0.0/8"}
	guarded := model.AccessListSourceEntry{List: "block", Source: "feed", URL: url, FetchedAt: now,
		SHA256: model.AccessListSourceHash(key, url, private), Entries: private}

	tests := []struct {
		name   string
		ledger *model.AccessListSourceEntry // nil: no ledger entry
		broken bool                         // the ledger file does not load
		budget string                       // maxCoverage on the deny list's source
		want   string                       // substring of the one deny warning, "" for none
	}{
		{name: "never fetched", want: `access list "block": deny rule source "feed" has never been fetched successfully`},
		{name: "fetched", ledger: &fetched},
		{name: "expired", ledger: &expired, want: `access list "block": deny rule source "feed" expired at ` + now},
		{name: "ledger does not load", broken: true, want: "the access-list source ledger could not be loaded"},
		{name: "every entry refused by the entry guard", ledger: &guarded, want: `deny rule source "feed" has a stored set that resolves to no networks`},
		{name: "over maxCoverage", ledger: &fetched, budget: `,"maxCoverage":{"ipv4":30}`, want: `deny rule source "feed" has a stored set over its maxCoverage budget`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st := store.New(dir, store.NewExecGit(dir))
			if err := st.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			h := api.New(api.Deps{Store: st})
			src := `"sources":[{"name":"feed","url":"` + url + `"}]`
			blockSrc := `"sources":[{"name":"feed","url":"` + url + `"` + tc.budget + `}]`
			mustPut(t, h, "/access-lists/block", `{"name":"block",`+blockSrc+`,"rules":[{"action":"deny","source":"feed"}],"defaultAction":"allow"}`)
			// Not warned: an allow source that was never fetched (the safe
			// direction), and a deny source on a disabled list.
			mustPut(t, h, "/access-lists/monitor", `{"name":"monitor",`+src+`,"rules":[{"action":"allow","source":"feed"}]}`)
			mustPut(t, h, "/access-lists/off", `{"name":"off","disabled":true,`+src+`,"rules":[{"action":"deny","source":"feed"}]}`)
			if tc.ledger != nil {
				l := model.AccessListSourceLedger{SchemaVersion: model.SchemaVersion, Sources: []model.AccessListSourceEntry{*tc.ledger}}
				if _, err := st.SaveAccessListSourceLedger(context.Background(), l, store.Author{}, ""); err != nil {
					t.Fatal(err)
				}
			}
			if tc.broken {
				if err := os.WriteFile(filepath.Join(dir, "access-list-sources.yaml"), []byte("schemaVersion: [\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			w := do(t, h, "GET", "/health", "")
			if w.Code != http.StatusOK {
				t.Fatalf("health: %d %s", w.Code, w.Body.String())
			}
			var resp struct {
				ConfigWarnings []string `json:"configWarnings"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, wn := range resp.ConfigWarnings {
				if strings.Contains(wn, "deny") {
					got = append(got, wn)
				}
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("unexpected deny warnings: %q", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("deny warnings = %q, want one containing %q", got, tc.want)
			}
		})
	}
}
