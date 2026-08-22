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
