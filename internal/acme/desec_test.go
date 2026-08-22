package acme

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// desecTestServer emulates the deSEC RRset endpoints the solver uses. rrsets is
// keyed by "<domain>/<subname>" and holds the TXT values in presentation form.
type desecTestServer struct {
	t       *testing.T
	domains []string
	rrsets  map[string][]string
	posts   []desecRRset
	patches []map[string]any
	gotAuth string
}

func (s *desecTestServer) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/domains/", func(w http.ResponseWriter, r *http.Request) {
		s.gotAuth = r.Header.Get("Authorization")
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/domains/")
		if rest == "" {
			var out []desecDomain
			for _, d := range s.domains {
				out = append(out, desecDomain{Name: d})
			}
			writeJSON(s.t, w, out)
			return
		}
		parts := strings.Split(strings.TrimSuffix(rest, "/"), "/")
		// {domain}/rrsets            -> POST a new RRset
		// {domain}/rrsets/{sub}/TXT  -> GET / PATCH an existing one
		if len(parts) == 2 && parts[1] == "rrsets" && r.Method == http.MethodPost {
			var body desecRRset
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				s.t.Fatalf("decode rrset post: %v", err)
			}
			s.posts = append(s.posts, body)
			sub := body.Subname
			if sub == "" {
				sub = "@"
			}
			s.rrsets[parts[0]+"/"+sub] = body.Records
			w.WriteHeader(http.StatusCreated)
			writeJSON(s.t, w, body)
			return
		}
		if len(parts) == 4 && parts[1] == "rrsets" && parts[3] == "TXT" {
			key := parts[0] + "/" + parts[2]
			switch r.Method {
			case http.MethodGet:
				records, ok := s.rrsets[key]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				writeJSON(s.t, w, desecRRset{Subname: parts[2], Type: "TXT", TTL: desecTTL, Records: records})
			case http.MethodPatch:
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					s.t.Fatalf("decode rrset patch: %v", err)
				}
				s.patches = append(s.patches, body)
				var records []string
				for _, v := range body["records"].([]any) {
					records = append(records, v.(string))
				}
				if len(records) == 0 {
					delete(s.rrsets, key)
				} else {
					s.rrsets[key] = records
				}
				writeJSON(s.t, w, body)
			default:
				http.NotFound(w, r)
			}
			return
		}
		http.NotFound(w, r)
	})
	return mux
}

func newDesecSolver(t *testing.T, fake *desecTestServer) *DesecSolver {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	s, err := NewDesecSolver("ds-token", WithDesecBaseURL(srv.URL), WithDesecHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDesecPresentAndCleanUp(t *testing.T) {
	fake := &desecTestServer{t: t, domains: []string{"example.com", "other.net"}, rrsets: map[string][]string{}}
	s := newDesecSolver(t, fake)
	ctx := context.Background()

	name := "_acme-challenge.example.com"
	if err := s.Present(ctx, name, "value-apex"); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if fake.gotAuth != "Token ds-token" {
		t.Errorf("Authorization = %q, want the deSEC token scheme", fake.gotAuth)
	}
	if len(fake.posts) != 1 {
		t.Fatalf("posted %d rrsets, want 1", len(fake.posts))
	}
	if fake.posts[0].Subname != "_acme-challenge" || fake.posts[0].Type != "TXT" {
		t.Errorf("rrset = %+v, want subname _acme-challenge type TXT", fake.posts[0])
	}
	if got := fake.rrsets["example.com/_acme-challenge"]; len(got) != 1 || got[0] != `"value-apex"` {
		t.Errorf("records = %v, want the quoted presentation form", got)
	}

	// A second value at the same name is merged into the RRset, not replaced -
	// this is what lets an apex + wildcard order validate.
	if err := s.Present(ctx, name, "value-wild"); err != nil {
		t.Fatalf("Present (second value): %v", err)
	}
	got := fake.rrsets["example.com/_acme-challenge"]
	if len(got) != 2 || got[0] != `"value-apex"` || got[1] != `"value-wild"` {
		t.Fatalf("records = %v, want both values", got)
	}
	if len(fake.posts) != 1 {
		t.Errorf("second value re-POSTed the rrset (%d posts), want a PATCH", len(fake.posts))
	}

	// Repeat Present is idempotent.
	patches := len(fake.patches)
	if err := s.Present(ctx, name, "value-wild"); err != nil {
		t.Fatalf("Present (repeat): %v", err)
	}
	if len(fake.patches) != patches {
		t.Errorf("repeat Present issued a write")
	}

	// CleanUp removes only its own value...
	if err := s.CleanUp(ctx, name, "value-apex"); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if got := fake.rrsets["example.com/_acme-challenge"]; len(got) != 1 || got[0] != `"value-wild"` {
		t.Fatalf("records after CleanUp = %v, want only the wildcard value", got)
	}
	// ... and the last one deletes the RRset.
	if err := s.CleanUp(ctx, name, "value-wild"); err != nil {
		t.Fatalf("CleanUp (last): %v", err)
	}
	if _, ok := fake.rrsets["example.com/_acme-challenge"]; ok {
		t.Error("empty rrset should be deleted")
	}
	// CleanUp on a missing RRset is a no-op.
	if err := s.CleanUp(ctx, name, "value-wild"); err != nil {
		t.Fatalf("CleanUp (absent): %v", err)
	}
}

func TestDesecZoneLongestSuffixMatch(t *testing.T) {
	fake := &desecTestServer{t: t, domains: []string{"example.com", "sub.example.com"}, rrsets: map[string][]string{}}
	s := newDesecSolver(t, fake)

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
	if got := desecSubname("example.com", "example.com"); got != "@" {
		t.Errorf("apex subname = %q, want @", got)
	}
}

func TestDesecRequiresToken(t *testing.T) {
	if _, err := NewDesecSolver(""); err == nil {
		t.Error("expected an error for an empty token")
	}
}
