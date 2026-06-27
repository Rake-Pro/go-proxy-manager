package acme

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cfTestServer emulates the subset of Cloudflare API v4 endpoints the solver
// uses. zones maps a zone name to a zone id; only those names are "owned".
type cfTestServer struct {
	t           *testing.T
	zones       map[string]string // name -> id
	records     []cfDNSRecord     // existing TXT records returned by list/created by POST
	createErr   *cfError          // if set, POST create returns this error envelope
	createCalls []map[string]any
	deleted     []string // ids deleted via DELETE
	gotToken    string
}

func okEnvelope(t *testing.T, w http.ResponseWriter, result any) {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	env := map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result":   json.RawMessage(raw),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(env)
}

func errEnvelope(w http.ResponseWriter, status int, e cfError) {
	env := map[string]any{
		"success":  false,
		"errors":   []cfError{e},
		"messages": []any{},
		"result":   nil,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}

func (s *cfTestServer) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/zones", func(w http.ResponseWriter, r *http.Request) {
		s.gotToken = r.Header.Get("Authorization")
		if r.Method != http.MethodGet {
			s.t.Errorf("zones: method = %s, want GET", r.Method)
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			s.t.Errorf("zones: missing name query")
		}
		var out []cfZone
		if id, ok := s.zones[name]; ok {
			out = append(out, cfZone{ID: id, Name: name})
		}
		okEnvelope(s.t, w, out)
	})

	mux.HandleFunc("/zones/", func(w http.ResponseWriter, r *http.Request) {
		// Paths: /zones/{id}/dns_records  and /zones/{id}/dns_records/{recordId}
		rest := strings.TrimPrefix(r.URL.Path, "/zones/")
		parts := strings.Split(rest, "/")
		if len(parts) < 2 || parts[1] != "dns_records" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			s.t.Errorf("missing Content-Type application/json on %s", r.URL.Path)
		}

		switch {
		case r.Method == http.MethodPost && len(parts) == 2:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				s.t.Fatalf("decode create body: %v", err)
			}
			s.createCalls = append(s.createCalls, body)
			if s.createErr != nil {
				errEnvelope(w, http.StatusBadRequest, *s.createErr)
				return
			}
			rec := cfDNSRecord{
				ID:      "new-record-id",
				Name:    toStr(body["name"]),
				Content: toStr(body["content"]),
				Type:    toStr(body["type"]),
			}
			s.records = append(s.records, rec)
			okEnvelope(s.t, w, rec)

		case r.Method == http.MethodGet && len(parts) == 2:
			name := r.URL.Query().Get("name")
			content := r.URL.Query().Get("content")
			typ := r.URL.Query().Get("type")
			var out []cfDNSRecord
			for _, rec := range s.records {
				if rec.Name == name && rec.Content == content && rec.Type == typ {
					out = append(out, rec)
				}
			}
			okEnvelope(s.t, w, out)

		case r.Method == http.MethodDelete && len(parts) == 3:
			id := parts[2]
			s.deleted = append(s.deleted, id)
			okEnvelope(s.t, w, map[string]string{"id": id})

		default:
			http.NotFound(w, r)
		}
	})

	return mux
}

func toStr(v any) string {
	s, _ := v.(string)
	return s
}

func newSolver(t *testing.T, srv *httptest.Server) *CloudflareSolver {
	t.Helper()
	s, err := NewCloudflareSolver("test-token",
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("NewCloudflareSolver: %v", err)
	}
	return s
}

func TestNewCloudflareSolver_EmptyToken(t *testing.T) {
	if _, err := NewCloudflareSolver(""); err == nil {
		t.Fatal("expected error for empty api token, got nil")
	}
}

func TestNewCloudflareSolver_BaseURLTrimmed(t *testing.T) {
	s, err := NewCloudflareSolver("tok", WithBaseURL("https://example.com/api/"))
	if err != nil {
		t.Fatal(err)
	}
	if s.baseURL != "https://example.com/api" {
		t.Fatalf("baseURL = %q, want trailing slash stripped", s.baseURL)
	}
}

func TestZoneDiscovery_WalksParents(t *testing.T) {
	cf := &cfTestServer{t: t, zones: map[string]string{"example.com": "zone-rake"}}
	srv := httptest.NewServer(cf.handler())
	defer srv.Close()
	s := newSolver(t, srv)

	id, err := s.zoneID(context.Background(), "_acme-challenge.app2.example.com")
	if err != nil {
		t.Fatalf("zoneID: %v", err)
	}
	if id != "zone-rake" {
		t.Fatalf("zone id = %q, want zone-rake", id)
	}
	if cf.gotToken != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want Bearer test-token", cf.gotToken)
	}

	// Cached: a second call must not depend on the server (still works).
	if _, err := s.zoneID(context.Background(), "_acme-challenge.app2.example.com"); err != nil {
		t.Fatalf("cached zoneID: %v", err)
	}
}

func TestZoneDiscovery_NotFound(t *testing.T) {
	cf := &cfTestServer{t: t, zones: map[string]string{}}
	srv := httptest.NewServer(cf.handler())
	defer srv.Close()
	s := newSolver(t, srv)

	if _, err := s.zoneID(context.Background(), "_acme-challenge.app2.example.com"); err == nil {
		t.Fatal("expected error when no zone found")
	}
}

func TestPresent_PostsCorrectBody(t *testing.T) {
	cf := &cfTestServer{t: t, zones: map[string]string{"example.com": "zone-rake"}}
	srv := httptest.NewServer(cf.handler())
	defer srv.Close()
	s := newSolver(t, srv)

	if err := s.Present(context.Background(), "_acme-challenge.example.com", "tokenvalue"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if len(cf.createCalls) != 1 {
		t.Fatalf("create calls = %d, want 1", len(cf.createCalls))
	}
	body := cf.createCalls[0]
	if body["type"] != "TXT" {
		t.Errorf("type = %v, want TXT", body["type"])
	}
	if body["name"] != "_acme-challenge.example.com" {
		t.Errorf("name = %v", body["name"])
	}
	if body["content"] != "tokenvalue" {
		t.Errorf("content = %v", body["content"])
	}
	// JSON numbers decode to float64.
	if ttl, ok := body["ttl"].(float64); !ok || ttl != 120 {
		t.Errorf("ttl = %v, want 120", body["ttl"])
	}
}

func TestPresent_AlreadyExistsIsSuccess(t *testing.T) {
	tests := []struct {
		name string
		err  cfError
	}{
		{"code 81057", cfError{Code: 81057, Message: "An identical record already exists."}},
		{"code 81058", cfError{Code: 81058, Message: "A record with that hostname already exists."}},
		{"message only", cfError{Code: 9999, Message: "Record already exists"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cf := &cfTestServer{
				t:         t,
				zones:     map[string]string{"example.com": "zone-rake"},
				createErr: &tt.err,
			}
			srv := httptest.NewServer(cf.handler())
			defer srv.Close()
			s := newSolver(t, srv)

			if err := s.Present(context.Background(), "_acme-challenge.example.com", "v"); err != nil {
				t.Fatalf("Present should treat duplicate as success, got: %v", err)
			}
		})
	}
}

func TestPresent_RealErrorPropagates(t *testing.T) {
	cf := &cfTestServer{
		t:         t,
		zones:     map[string]string{"example.com": "zone-rake"},
		createErr: &cfError{Code: 1003, Message: "Invalid something"},
	}
	srv := httptest.NewServer(cf.handler())
	defer srv.Close()
	s := newSolver(t, srv)

	err := s.Present(context.Background(), "_acme-challenge.example.com", "v")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "1003") {
		t.Errorf("error %q should contain code 1003", err.Error())
	}
}

func TestCleanUp_FindsAndDeletes(t *testing.T) {
	cf := &cfTestServer{
		t:     t,
		zones: map[string]string{"example.com": "zone-rake"},
		records: []cfDNSRecord{
			{ID: "match-id", Name: "_acme-challenge.example.com", Content: "wanted", Type: "TXT"},
			{ID: "other-id", Name: "_acme-challenge.example.com", Content: "different", Type: "TXT"},
		},
	}
	srv := httptest.NewServer(cf.handler())
	defer srv.Close()
	s := newSolver(t, srv)

	if err := s.CleanUp(context.Background(), "_acme-challenge.example.com", "wanted"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if len(cf.deleted) != 1 || cf.deleted[0] != "match-id" {
		t.Fatalf("deleted = %v, want [match-id]", cf.deleted)
	}
}

func TestCleanUp_NoMatchReturnsNil(t *testing.T) {
	cf := &cfTestServer{
		t:       t,
		zones:   map[string]string{"example.com": "zone-rake"},
		records: []cfDNSRecord{{ID: "x", Name: "_acme-challenge.example.com", Content: "other", Type: "TXT"}},
	}
	srv := httptest.NewServer(cf.handler())
	defer srv.Close()
	s := newSolver(t, srv)

	if err := s.CleanUp(context.Background(), "_acme-challenge.example.com", "missing"); err != nil {
		t.Fatalf("CleanUp with no match should return nil, got: %v", err)
	}
	if len(cf.deleted) != 0 {
		t.Fatalf("deleted = %v, want none", cf.deleted)
	}
}

func TestEnvelopeFailureYieldsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errEnvelope(w, http.StatusOK, cfError{Code: 1003, Message: "boom"})
	}))
	defer srv.Close()
	s := newSolver(t, srv)

	_, err := s.zoneID(context.Background(), "_acme-challenge.example.com")
	if err == nil {
		t.Fatal("expected error from success:false envelope")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "1003") {
		t.Errorf("error %q should include joined message and code", err.Error())
	}
}

// ensure CloudflareSolver satisfies DNSSolver at compile time.
var _ DNSSolver = (*CloudflareSolver)(nil)
