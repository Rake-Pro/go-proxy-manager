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

// doTestServer emulates the DigitalOcean v2 endpoints the solver uses.
type doTestServer struct {
	t        *testing.T
	domains  []string
	records  map[string][]doRecord // domain -> records
	nextID   int64
	deleted  []int64
	created  []map[string]any
	gotAuth  string
	pageOnce bool // serve the domain list in two pages
}

func (s *doTestServer) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v2/domains", func(w http.ResponseWriter, r *http.Request) {
		s.gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodGet {
			s.t.Errorf("domains: method = %s, want GET", r.Method)
		}
		type dom struct {
			Name string `json:"name"`
		}
		out := map[string]any{}
		page := r.URL.Query().Get("page")
		if s.pageOnce && page != "2" {
			out["domains"] = []dom{{Name: s.domains[0]}}
			out["links"] = map[string]any{"pages": map[string]any{"next": "https://api.digitalocean.com/v2/domains?page=2&per_page=200"}}
		} else {
			var ds []dom
			list := s.domains
			if s.pageOnce {
				list = s.domains[1:]
			}
			for _, d := range list {
				ds = append(ds, dom{Name: d})
			}
			out["domains"] = ds
			out["links"] = map[string]any{}
		}
		writeJSON(s.t, w, out)
	})

	mux.HandleFunc("/v2/domains/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/v2/domains/")
		parts := strings.Split(rest, "/")
		if len(parts) < 2 || parts[1] != "records" {
			http.NotFound(w, r)
			return
		}
		domain := parts[0]
		switch {
		case r.Method == http.MethodGet:
			var out []doRecord
			out = append(out, s.records[domain]...)
			writeJSON(s.t, w, map[string]any{"domain_records": out})
		case r.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				s.t.Fatalf("decode create body: %v", err)
			}
			s.created = append(s.created, body)
			s.nextID++
			s.records[domain] = append(s.records[domain], doRecord{
				ID:   s.nextID,
				Type: "TXT",
				Name: body["name"].(string),
				Data: body["data"].(string),
			})
			w.WriteHeader(http.StatusCreated)
			writeJSON(s.t, w, map[string]any{"domain_record": map[string]any{"id": s.nextID}})
		case r.Method == http.MethodDelete:
			if len(parts) != 3 {
				http.NotFound(w, r)
				return
			}
			id, _ := strconv.ParseInt(parts[2], 10, 64)
			kept := s.records[domain][:0]
			found := false
			for _, rec := range s.records[domain] {
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
			s.records[domain] = kept
			s.deleted = append(s.deleted, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func newDOSolver(t *testing.T, fake *doTestServer) (*DigitalOceanSolver, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(srv.URL), WithDOHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return s, srv
}

func TestDigitalOceanPresentAndCleanUp(t *testing.T) {
	fake := &doTestServer{t: t, domains: []string{"example.com", "other.net"}, records: map[string][]doRecord{}}
	s, _ := newDOSolver(t, fake)
	ctx := context.Background()

	name, value := "_acme-challenge.app.example.com", "token-value"
	if err := s.Present(ctx, name, value); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if fake.gotAuth != "Bearer do-token" {
		t.Errorf("Authorization = %q, want bearer token", fake.gotAuth)
	}
	if len(fake.created) != 1 {
		t.Fatalf("created %d records, want 1", len(fake.created))
	}
	if got := fake.created[0]["name"]; got != "_acme-challenge.app" {
		t.Errorf("record name = %v, want the zone-relative name", got)
	}
	if got := fake.created[0]["data"]; got != value {
		t.Errorf("record data = %v, want %q", got, value)
	}
	if got := fake.created[0]["type"]; got != "TXT" {
		t.Errorf("record type = %v", got)
	}

	// Present again: the identical record already exists, so nothing new is created.
	if err := s.Present(ctx, name, value); err != nil {
		t.Fatalf("Present (repeat): %v", err)
	}
	if len(fake.created) != 1 {
		t.Errorf("repeat Present created %d records, want 1", len(fake.created))
	}

	// A second value at the same name is added, not replaced (apex + wildcard).
	if err := s.Present(ctx, name, "second-value"); err != nil {
		t.Fatalf("Present (second value): %v", err)
	}
	if len(fake.records["example.com"]) != 2 {
		t.Fatalf("want 2 records at the shared name, got %d", len(fake.records["example.com"]))
	}

	// CleanUp removes only the matching value.
	if err := s.CleanUp(ctx, name, value); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	left := fake.records["example.com"]
	if len(left) != 1 || left[0].Data != "second-value" {
		t.Fatalf("CleanUp removed the wrong record: %+v", left)
	}

	// CleanUp for an absent value is a no-op, not an error.
	if err := s.CleanUp(ctx, name, "never-there"); err != nil {
		t.Fatalf("CleanUp (absent): %v", err)
	}
}

func TestDigitalOceanZoneLongestSuffixMatch(t *testing.T) {
	// The account holds both the parent and the delegated sub-zone; the more
	// specific one must win.
	fake := &doTestServer{t: t, domains: []string{"example.com", "sub.example.com"}, records: map[string][]doRecord{}}
	s, _ := newDOSolver(t, fake)

	zone, err := s.zoneFor(context.Background(), "_acme-challenge.app.sub.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if zone != "sub.example.com" {
		t.Errorf("zone = %q, want sub.example.com", zone)
	}
	if _, err := s.zoneFor(context.Background(), "_acme-challenge.nope.test"); err == nil {
		t.Error("expected an error for a name in no owned domain")
	}
}

func TestDigitalOceanZonePagination(t *testing.T) {
	fake := &doTestServer{t: t, domains: []string{"first.test", "example.com"}, records: map[string][]doRecord{}, pageOnce: true}
	s, _ := newDOSolver(t, fake)
	zone, err := s.zoneFor(context.Background(), "_acme-challenge.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if zone != "example.com" {
		t.Errorf("zone = %q, want example.com (second page)", zone)
	}
}

func TestDigitalOceanRequiresToken(t *testing.T) {
	if _, err := NewDigitalOceanSolver(""); err == nil {
		t.Error("expected an error for an empty token")
	}
}
