package acme

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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
			// The API names the next page with an absolute URL on its own host.
			out["links"] = map[string]any{"pages": map[string]any{"next": "http://" + r.Host + "/v2/domains?page=2&per_page=200"}}
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

func TestDigitalOceanZonePaginationStaysOnBaseURL(t *testing.T) {
	other := 0
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		other++
		writeJSON(t, w, map[string]any{"domains": []map[string]string{{"name": "example.com"}}})
	}))
	t.Cleanup(elsewhere.Close)
	host := strings.TrimPrefix(elsewhere.URL, "http://")
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"domains": []map[string]string{{"name": "first.test"}},
			"links":   map[string]any{"pages": map[string]any{"next": "x:@" + host + "/v2/domains"}},
		})
	}))
	t.Cleanup(api.Close)
	s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL), WithDOHTTPClient(api.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.zoneFor(context.Background(), "_acme-challenge.example.com"); err == nil {
		t.Error("zoneFor found a zone from a page outside the API base URL")
	}
	if other != 0 {
		t.Errorf("%d request(s) left the API base URL, want 0", other)
	}
}

func TestDigitalOceanZonePaginationBound(t *testing.T) {
	var requests int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		// Every page carries the owning domain and names another page.
		writeJSON(t, w, map[string]any{
			"domains": []map[string]string{{"name": "example.com"}},
			"links":   map[string]any{"pages": map[string]any{"next": "/v2/domains?page=" + strconv.Itoa(requests+1)}},
		})
	}))
	t.Cleanup(api.Close)
	s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL), WithDOHTTPClient(api.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.zoneFor(context.Background(), "_acme-challenge.example.com")
	if err == nil || !strings.Contains(err.Error(), "refusing to use a partial listing") {
		t.Fatalf("zoneFor error = %v, want a refusal to use a partial listing", err)
	}
	if requests != doMaxPages {
		t.Errorf("followed %d pages, want the bound %d", requests, doMaxPages)
	}
}

// A listing that ends on the last allowed page is complete and is used.
func TestDigitalOceanZonePaginationWithinBound(t *testing.T) {
	var requests int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := map[string]any{"domains": []map[string]string{{"name": "other" + strconv.Itoa(requests) + ".example.net"}}}
		if requests < doMaxPages {
			page["links"] = map[string]any{"pages": map[string]any{"next": "/v2/domains?page=" + strconv.Itoa(requests+1)}}
		} else {
			page["domains"] = []map[string]string{{"name": "example.com"}}
		}
		writeJSON(t, w, page)
	}))
	t.Cleanup(api.Close)
	s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL), WithDOHTTPClient(api.Client()))
	if err != nil {
		t.Fatal(err)
	}
	zone, err := s.zoneFor(context.Background(), "_acme-challenge.example.com")
	if err != nil || zone != "example.com" {
		t.Fatalf("zoneFor = %q, %v, want example.com from page %d", zone, err, doMaxPages)
	}
}

// A relative next link resolves against the page that carried it, so the
// domain on the second page is found whatever form the link takes.
func TestDigitalOceanZonePaginationRelativeLinks(t *testing.T) {
	for _, next := range []string{"/v2/domains?page=2", "domains?page=2", "?page=2", "//HOST/v2/domains?page=2"} {
		t.Run(next, func(t *testing.T) {
			var pages []string
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pages = append(pages, r.URL.RequestURI())
				if r.URL.Path != "/v2/domains" {
					t.Errorf("request path = %q, want /v2/domains", r.URL.Path)
				}
				if r.URL.Query().Get("page") == "2" {
					writeJSON(t, w, map[string]any{"domains": []map[string]string{{"name": "example.com"}}})
					return
				}
				writeJSON(t, w, map[string]any{
					"domains": []map[string]string{{"name": "first.test"}},
					"links":   map[string]any{"pages": map[string]any{"next": strings.Replace(next, "HOST", r.Host, 1)}},
				})
			}))
			t.Cleanup(api.Close)
			s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL), WithDOHTTPClient(api.Client()))
			if err != nil {
				t.Fatal(err)
			}
			zone, err := s.zoneFor(context.Background(), "_acme-challenge.example.com")
			if err != nil || zone != "example.com" {
				t.Fatalf("zoneFor = %q, %v, want example.com from page 2 (pages %q)", zone, err, pages)
			}
		})
	}
}

// A next link that leaves the configured API (another host, another scheme,
// an opaque URL) is an error naming the problem, never a silent end of the
// listing, and the token is never sent anywhere but the API host.
func TestDigitalOceanZonePaginationRefusesLinksOffTheAPI(t *testing.T) {
	var otherAuth []string
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherAuth = append(otherAuth, r.Header.Get("Authorization"))
		writeJSON(t, w, map[string]any{"domains": []map[string]string{{"name": "example.com"}}})
	}))
	t.Cleanup(elsewhere.Close)
	otherHost := strings.TrimPrefix(elsewhere.URL, "http://")

	cases := []struct {
		next string // API_HOST is replaced with the API server's host
		want string
	}{
		{"http://" + otherHost + "/v2/domains?page=2", `points at host "` + otherHost + `", not the API host`},
		{"//" + otherHost + "/v2/domains?page=2", `points at host "` + otherHost + `", not the API host`},
		{"https://API_HOST/v2/domains?page=2", `uses scheme "https", not the API's "http"`},
		{"x:@" + otherHost + "/v2/domains", "is opaque"},
		{"/v2/%zz?page=2", "does not parse"},
	}
	for _, tc := range cases {
		t.Run(tc.next, func(t *testing.T) {
			otherAuth = nil
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") != "" {
					t.Errorf("followed %q on the API host", r.URL.RequestURI())
				}
				writeJSON(t, w, map[string]any{
					"domains": []map[string]string{{"name": "first.test"}},
					"links":   map[string]any{"pages": map[string]any{"next": strings.Replace(tc.next, "API_HOST", r.Host, 1)}},
				})
			}))
			t.Cleanup(api.Close)
			// The client would reach both servers: only the solver keeps the
			// token on the API host.
			s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL), WithDOHTTPClient(api.Client()))
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.zoneFor(context.Background(), "_acme-challenge.example.com")
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "refusing to use a partial listing") {
				t.Fatalf("zoneFor error = %v, want one carrying %q", err, tc.want)
			}
			if len(otherAuth) != 0 {
				t.Fatalf("%d request(s) reached another host (Authorization %q), want 0", len(otherAuth), otherAuth)
			}
		})
	}
}

// A base URL with a path prefix: a next link, absolute or relative, is
// followed under that prefix once, and a link outside it is refused.
func TestDigitalOceanZonePaginationUnderABasePath(t *testing.T) {
	cases := []struct {
		next string // HOST is replaced with the API server's host
		want string // "" = the zone on page 2 is found
	}{
		{"http://HOST/prefix/v2/domains?page=2", ""},
		{"/prefix/v2/domains?page=2", ""},
		{"domains?page=2", ""},
		{"?page=2", ""},
		{"/v2/domains?page=2", `is outside the API base path "/prefix"`},
		{"/prefixx/v2/domains?page=2", `is outside the API base path "/prefix"`},
	}
	for _, tc := range cases {
		t.Run(tc.next, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/prefix/v2/domains" {
					t.Errorf("request path = %q, want /prefix/v2/domains", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if r.URL.Query().Get("page") == "2" {
					writeJSON(t, w, map[string]any{"domains": []map[string]string{{"name": "example.com"}}})
					return
				}
				writeJSON(t, w, map[string]any{
					"domains": []map[string]string{{"name": "first.test"}},
					"links":   map[string]any{"pages": map[string]any{"next": strings.Replace(tc.next, "HOST", r.Host, 1)}},
				})
			}))
			t.Cleanup(api.Close)
			s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL+"/prefix/"), WithDOHTTPClient(api.Client()))
			if err != nil {
				t.Fatal(err)
			}
			zone, err := s.zoneFor(context.Background(), "_acme-challenge.example.com")
			if tc.want == "" {
				if err != nil || zone != "example.com" {
					t.Fatalf("zoneFor = %q, %v, want example.com from page 2", zone, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("zoneFor error = %v, want one carrying %q", err, tc.want)
			}
		})
	}
}

// A next link is server-supplied and can be as long as the response: every
// refusal quotes it (and the host or scheme taken from it) cut to about 200
// bytes on a rune boundary, with the cut marked.
func TestDigitalOceanZonePaginationErrorsCutTheLink(t *testing.T) {
	long := strings.Repeat("a", 3000)
	// Two bytes each, after an odd-length prefix: byte 200 lands mid-rune.
	runes := strings.Repeat("\u00e9", 1500)
	cases := []struct {
		next string
		want string
	}{
		{"/v2/a" + runes + "%zz", "does not parse as a URL"},
		{"x:a" + runes, "is opaque"},
		{long + "://HOST/v2/domains", "uses scheme"},
		{"http://" + long + ".example.com/v2/" + runes, "points at host"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, map[string]any{
					"domains": []map[string]string{{"name": "first.test"}},
					"links":   map[string]any{"pages": map[string]any{"next": strings.Replace(tc.next, "HOST", r.Host, 1)}},
				})
			}))
			t.Cleanup(api.Close)
			s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL), WithDOHTTPClient(api.Client()))
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.zoneFor(context.Background(), "_acme-challenge.example.com")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("zoneFor error = %v, want one carrying %q", err, tc.want)
			}
			msg := err.Error()
			if len(msg) > 900 {
				t.Fatalf("error is %d bytes, want the link cut short: %.300s...", len(msg), msg)
			}
			if !strings.Contains(msg, "... (cut, ") {
				t.Fatalf("error does not mark the cut: %s", msg)
			}
			if strings.Contains(msg, `\x`) {
				t.Fatalf("the cut split a rune: %s", msg)
			}
		})
	}
}

// Once a long server-supplied next path has been followed, every error from
// that request carries the path cut short; the request itself goes out whole.
func TestDigitalOceanFollowedLongPathIsCutInErrors(t *testing.T) {
	long := "/v2/domains/" + strings.Repeat("a", 20000)
	cases := []struct {
		name  string
		page2 func(w http.ResponseWriter, r *http.Request)
		want  string
	}{
		{"server error", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}, ": 500: boom"},
		{"transport failure", func(w http.ResponseWriter, r *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			conn.Close()
		}, ": EOF"},
		{"redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		}, "redirects are not followed"},
		{"undecodable body", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("{not json"))
		}, "response: invalid character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Read under the lock: on the hijacked connection nothing else orders
			// the handler's write before the test's read.
			var mu sync.Mutex
			var gotPath string
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") == "2" {
					mu.Lock()
					gotPath = r.URL.Path
					mu.Unlock()
					tc.page2(w, r)
					return
				}
				writeJSON(t, w, map[string]any{
					"domains": []map[string]string{{"name": "first.test"}},
					"links":   map[string]any{"pages": map[string]any{"next": long + "?page=2"}},
				})
			}))
			t.Cleanup(api.Close)
			// Like the production client, this one does not follow redirects.
			client := api.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL), WithDOHTTPClient(client))
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.zoneFor(context.Background(), "_acme-challenge.example.com")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("zoneFor error = %.300v, want one carrying %q", err, tc.want)
			}
			mu.Lock()
			got := gotPath
			mu.Unlock()
			if got != long {
				t.Fatalf("page 2 was requested at a %d-byte path, want the whole %d-byte link", len(got), len(long))
			}
			msg := err.Error()
			if len(msg) > 600 {
				t.Fatalf("error is %d bytes, want the path cut short: %.300s...", len(msg), msg)
			}
			if !strings.Contains(msg, "... (cut, "+strconv.Itoa(len(long+"?page=2"))+" bytes in all)") {
				t.Fatalf("error does not mark the cut: %s", msg)
			}
		})
	}
}

// A next link whose decoded path has a "." or ".." segment (escaped, so
// resolving the link leaves it in place) is refused under a plain base and
// under a prefixed one: a server that normalises it would serve a path
// outside the API path.
func TestDigitalOceanZonePaginationRefusesDotSegments(t *testing.T) {
	cases := []struct{ base, next string }{
		{"", "/v2/%2e%2e/x?page=2"},
		{"", "/v2/%2E/domains?page=2"},
		{"", "%2e%2e/%2e%2e/x?page=2"},
		{"/prefix", "/prefix/%2e%2e/v2/domains?page=2"},
		{"/prefix", "/prefix/v2/%2e%2e/%2e%2e/%2e%2e/v2/domains?page=2"},
	}
	for _, tc := range cases {
		t.Run(tc.base+" "+tc.next, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") != "" {
					t.Errorf("followed %q", r.URL.RawPath)
					writeJSON(t, w, map[string]any{"domains": []map[string]string{{"name": "example.com"}}})
					return
				}
				writeJSON(t, w, map[string]any{
					"domains": []map[string]string{{"name": "first.test"}},
					"links":   map[string]any{"pages": map[string]any{"next": tc.next}},
				})
			}))
			t.Cleanup(api.Close)
			s, err := NewDigitalOceanSolver("do-token", WithDOBaseURL(api.URL+tc.base), WithDOHTTPClient(api.Client()))
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.zoneFor(context.Background(), "_acme-challenge.example.com")
			if err == nil || !strings.Contains(err.Error(), `path segment`) || !strings.Contains(err.Error(), "refusing to use a partial listing") {
				t.Fatalf("zoneFor error = %v, want a refusal naming the dot segment", err)
			}
		})
	}
}
