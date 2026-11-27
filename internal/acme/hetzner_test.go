package acme

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// hetznerTestServer emulates the Hetzner DNS API v1 endpoints the solver uses.
type hetznerTestServer struct {
	t       *testing.T
	zones   []hetznerZone
	records []hetznerRecord
	nextID  int
	created []map[string]any
	deleted []string
	gotAuth string
}

func (s *hetznerTestServer) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/zones", func(w http.ResponseWriter, r *http.Request) {
		s.gotAuth = r.Header.Get("Auth-API-Token")
		writeJSON(s.t, w, map[string]any{"zones": s.zones})
	})

	mux.HandleFunc("/api/v1/records", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			zoneID := r.URL.Query().Get("zone_id")
			if zoneID == "" {
				s.t.Error("records list: missing zone_id")
			}
			var out []hetznerRecord
			for _, rec := range s.records {
				if rec.ZoneID == zoneID {
					out = append(out, rec)
				}
			}
			writeJSON(s.t, w, map[string]any{"records": out})
		case http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				s.t.Fatalf("decode create body: %v", err)
			}
			s.created = append(s.created, body)
			s.nextID++
			s.records = append(s.records, hetznerRecord{
				ID:     "rec" + strconv.Itoa(s.nextID),
				ZoneID: body["zone_id"].(string),
				Type:   body["type"].(string),
				Name:   body["name"].(string),
				Value:  body["value"].(string),
			})
			w.WriteHeader(http.StatusOK)
			writeJSON(s.t, w, map[string]any{"record": map[string]any{"id": "rec" + strconv.Itoa(s.nextID)}})
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/api/v1/records/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/records/")
		kept := s.records[:0]
		found := false
		for _, rec := range s.records {
			if rec.ID == id {
				found = true
				continue
			}
			kept = append(kept, rec)
		}
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.records = kept
		s.deleted = append(s.deleted, id)
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func newHetznerSolver(t *testing.T, fake *hetznerTestServer) *HetznerSolver {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	s, err := NewHetznerSolver("hz-token", WithHetznerBaseURL(srv.URL), WithHetznerHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHetznerPresentAndCleanUp(t *testing.T) {
	fake := &hetznerTestServer{t: t, zones: []hetznerZone{{ID: "z1", Name: "example.com"}, {ID: "z2", Name: "other.net"}}}
	s := newHetznerSolver(t, fake)
	ctx := context.Background()

	name, value := "_acme-challenge.example.com", "token-value"
	if err := s.Present(ctx, name, value); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if fake.gotAuth != "hz-token" {
		t.Errorf("Auth-API-Token = %q", fake.gotAuth)
	}
	if len(fake.created) != 1 {
		t.Fatalf("created %d records, want 1", len(fake.created))
	}
	if got := fake.created[0]["zone_id"]; got != "z1" {
		t.Errorf("zone_id = %v, want z1", got)
	}
	if got := fake.created[0]["name"]; got != "_acme-challenge" {
		t.Errorf("record name = %v, want the zone-relative name", got)
	}

	// Repeat Present is idempotent.
	if err := s.Present(ctx, name, value); err != nil {
		t.Fatalf("Present (repeat): %v", err)
	}
	if len(fake.created) != 1 {
		t.Errorf("repeat Present created %d records, want 1", len(fake.created))
	}

	// A second value at the same name coexists.
	if err := s.Present(ctx, name, "second-value"); err != nil {
		t.Fatalf("Present (second value): %v", err)
	}
	if len(fake.records) != 2 {
		t.Fatalf("want 2 records, got %d", len(fake.records))
	}

	if err := s.CleanUp(ctx, name, value); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if len(fake.records) != 1 || fake.records[0].Value != "second-value" {
		t.Fatalf("CleanUp removed the wrong record: %+v", fake.records)
	}
	if err := s.CleanUp(ctx, name, "never-there"); err != nil {
		t.Fatalf("CleanUp (absent): %v", err)
	}
}

func TestHetznerZoneMatchAndApexName(t *testing.T) {
	fake := &hetznerTestServer{t: t, zones: []hetznerZone{{ID: "z1", Name: "example.com"}, {ID: "z2", Name: "sub.example.com"}}}
	s := newHetznerSolver(t, fake)

	zone, err := s.zoneFor(context.Background(), "_acme-challenge.app.sub.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if zone.ID != "z2" {
		t.Errorf("zone = %+v, want the longest-suffix match sub.example.com", zone)
	}
	if _, err := s.zoneFor(context.Background(), "_acme-challenge.nope.test"); err == nil {
		t.Error("expected an error for a name in no owned zone")
	}
	// A record at the zone apex is named "@".
	if got := hetznerRecordName("example.com", "example.com"); got != "@" {
		t.Errorf("apex record name = %q, want @", got)
	}
}

func TestHetznerRequiresToken(t *testing.T) {
	if _, err := NewHetznerSolver(""); err == nil {
		t.Error("expected an error for an empty token")
	}
}

// TestHetznerPagination serves the zone and record listings in pages and checks
// that every page is followed, that an unpaginated response is read with the one
// request it always was, and that a listing that never ends or breaks halfway is
// an error rather than a partial result.
func TestHetznerPagination(t *testing.T) {
	rec := func(id, zoneID, value string) hetznerRecord {
		return hetznerRecord{ID: id, ZoneID: zoneID, Type: "TXT", Name: "_acme-challenge", Value: value}
	}
	tests := []struct {
		name        string
		zonePages   [][]hetznerZone
		recordPages [][]hetznerRecord
		lastPage    int // record last_page; 0 = len(recordPages), -1 = no meta at all
		failPage    int // record page answered 404
		wantErr     string
		wantCreated int
		wantQueries []string // record listing queries, in order
	}{
		{
			name:        "unpaginated response",
			zonePages:   [][]hetznerZone{{{ID: "z1", Name: "example.com"}}},
			recordPages: [][]hetznerRecord{{rec("r1", "z1", "other")}},
			lastPage:    -1,
			wantCreated: 1,
			wantQueries: []string{"per_page=500&zone_id=z1"},
		},
		{
			name:        "zone and existing record on later pages",
			zonePages:   [][]hetznerZone{{{ID: "z9", Name: "other.net"}}, {{ID: "z1", Name: "example.com"}}},
			recordPages: [][]hetznerRecord{{rec("r1", "z1", "a")}, {rec("r2", "z1", "b")}, {rec("r3", "z1", "token-value")}},
			wantCreated: 0,
			wantQueries: []string{"per_page=500&zone_id=z1", "page=2&per_page=500&zone_id=z1", "page=3&per_page=500&zone_id=z1"},
		},
		{
			name:        "page bound reached",
			zonePages:   [][]hetznerZone{{{ID: "z1", Name: "example.com"}}},
			recordPages: [][]hetznerRecord{{rec("r1", "z1", "a")}},
			lastPage:    hetznerMaxPages + 50,
			wantErr:     "refusing to use a partial listing",
		},
		{
			name:        "404 on a later page",
			zonePages:   [][]hetznerZone{{{ID: "z1", Name: "example.com"}}},
			recordPages: [][]hetznerRecord{{rec("r1", "z1", "a")}, {rec("r2", "z1", "b")}},
			failPage:    2,
			wantErr:     "page 2 of a paginated listing",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var queries []string
			var created int
			// meta reports last_page n; the lastPage override applies to the
			// record listing only, so the bound case walks the records.
			meta := func(page, n int) map[string]any {
				return map[string]any{"pagination": map[string]any{"page": page, "per_page": 500, "last_page": n}}
			}
			pageOf := func(r *http.Request) int {
				p, _ := strconv.Atoi(r.URL.Query().Get("page"))
				return max(p, 1)
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/zones", func(w http.ResponseWriter, r *http.Request) {
				p := pageOf(r)
				body := map[string]any{"zones": tc.zonePages[min(p, len(tc.zonePages))-1]}
				if tc.lastPage != -1 {
					body["meta"] = meta(p, len(tc.zonePages))
				}
				writeJSON(t, w, body)
			})
			mux.HandleFunc("/api/v1/records", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					created++
					writeJSON(t, w, map[string]any{"record": map[string]any{"id": "new"}})
					return
				}
				queries = append(queries, r.URL.RawQuery)
				p := pageOf(r)
				if p == tc.failPage {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				body := map[string]any{"records": tc.recordPages[min(p, len(tc.recordPages))-1]}
				switch {
				case tc.lastPage > 0:
					body["meta"] = meta(p, tc.lastPage)
				case tc.lastPage == 0:
					body["meta"] = meta(p, len(tc.recordPages))
				}
				writeJSON(t, w, body)
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)
			s, err := NewHetznerSolver("hz-token", WithHetznerBaseURL(srv.URL), WithHetznerHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}

			err = s.Present(context.Background(), "_acme-challenge.example.com", "token-value")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Present error = %v, want it to contain %q", err, tc.wantErr)
				}
				if tc.lastPage > hetznerMaxPages && len(queries) != hetznerMaxPages {
					t.Errorf("followed %d pages, want the bound %d", len(queries), hetznerMaxPages)
				}
				return
			}
			if err != nil {
				t.Fatalf("Present: %v", err)
			}
			if created != tc.wantCreated {
				t.Errorf("created %d records, want %d", created, tc.wantCreated)
			}
			if strings.Join(queries, " ") != strings.Join(tc.wantQueries, " ") {
				t.Errorf("record queries = %q, want %q", queries, tc.wantQueries)
			}
		})
	}
}
