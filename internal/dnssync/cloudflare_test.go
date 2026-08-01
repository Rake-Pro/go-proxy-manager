package dnssync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// fakeCloudflare is a hermetic stand-in for the Cloudflare API v4: one zone,
// an in-memory record set, and paginated listing so the paging path is exercised.
type fakeCloudflare struct {
	mu       sync.Mutex
	zoneName string
	zoneID   string
	records  []cfRecord
	nextID   int
	perPage  int // records returned per listing page (defaults to 100)
	// deleted records the IDs the client asked to remove, so a test can assert
	// that an unmanaged record was never even attempted.
	deleted []string
	// failCreateContent fails a create whose content is exactly this, so a test
	// can fail the create half of a retarget while leaving the rollback that
	// restores the original able to succeed.
	failCreateContent string
	// omitResultInfo drops result_info from listing responses, the shape that used
	// to truncate pagination at page 1.
	omitResultInfo bool
	// writes counts every mutating call, so a test can assert that a dry run
	// touched nothing at all.
	writes int
}

func (f *fakeCloudflare) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /zones", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkAuth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var zones []map[string]string
		if r.URL.Query().Get("name") == f.zoneName {
			zones = append(zones, map[string]string{"id": f.zoneID, "name": f.zoneName})
		}
		writeEnvelope(w, zones, 1, 1)
	})
	mux.HandleFunc("GET /zones/{zone}/dns_records", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkAuth(w, r) || !f.checkZone(w, r) {
			return
		}
		if r.URL.Query().Get("type") != "CNAME" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		per := f.perPage
		if per <= 0 {
			per = 100
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		total := (len(f.records) + per - 1) / per
		if total < 1 {
			total = 1
		}
		start := (page - 1) * per
		end := start + per
		if start > len(f.records) {
			start = len(f.records)
		}
		if end > len(f.records) {
			end = len(f.records)
		}
		if f.omitResultInfo {
			writeResultOnly(w, f.records[start:end])
			return
		}
		writeEnvelope(w, f.records[start:end], page, total)
	})
	mux.HandleFunc("POST /zones/{zone}/dns_records", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkAuth(w, r) || !f.checkZone(w, r) {
			return
		}
		var body cfRecord
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failCreateContent != "" && body.Content == f.failCreateContent {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1004,"message":"record create failed"}]}`))
			return
		}
		f.writes++
		f.nextID++
		body.ID = "rec-" + strconv.Itoa(f.nextID)
		f.records = append(f.records, body)
		writeEnvelope(w, body, 1, 1)
	})
	mux.HandleFunc("DELETE /zones/{zone}/dns_records/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkAuth(w, r) || !f.checkZone(w, r) {
			return
		}
		id := r.PathValue("id")
		f.mu.Lock()
		defer f.mu.Unlock()
		f.writes++
		f.deleted = append(f.deleted, id)
		out := f.records[:0]
		for _, rec := range f.records {
			if rec.ID != id {
				out = append(out, rec)
			}
		}
		f.records = out
		writeEnvelope(w, map[string]string{"id": id}, 1, 1)
	})
	return mux
}

func (f *fakeCloudflare) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") != "Bearer cf-token" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"bad token"}]}`))
		return false
	}
	return true
}

func (f *fakeCloudflare) checkZone(w http.ResponseWriter, r *http.Request) bool {
	if r.PathValue("zone") != f.zoneID {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1049,"message":"unknown zone"}]}`))
		return false
	}
	return true
}

func writeEnvelope(w http.ResponseWriter, result any, page, totalPages int) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"errors":      []any{},
		"result":      result,
		"result_info": map[string]int{"page": page, "total_pages": totalPages},
	})
}

// writeResultOnly answers with a successful envelope that carries NO result_info
// at all - a shape the API is free to use, and one the client must not read as
// "there is only one page".
func writeResultOnly(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"errors":  []any{},
		"result":  result,
	})
}

func (f *fakeCloudflare) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.records {
		out = append(out, r.Name+"|"+r.Content+"|"+r.Comment)
	}
	sort.Strings(out)
	return out
}

func publicHost(name string, domains ...string) model.ProxyHost {
	return model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: name},
		Domains:    domains,
		Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.5", Port: 8080},
		DNS:        &model.DNSSyncPolicy{PublicCname: true},
	}
}

func startCloudflare(t *testing.T, f *fakeCloudflare) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return srv
}

func cloudflareSyncer(t *testing.T, srv *httptest.Server, hosts []model.ProxyHost, proxied bool) *Syncer {
	t.Helper()
	return cloudflareSyncerWith(t, srv, hosts, proxied, &memLedger{})
}

func cloudflareSyncerWith(t *testing.T, srv *httptest.Server, hosts []model.ProxyHost, proxied bool, ledger Ledger) *Syncer {
	t.Helper()
	t.Setenv("GPM_CF_TOKEN", "cf-token")
	prev := cfBaseURL
	cfBaseURL = srv.URL
	t.Cleanup(func() { cfBaseURL = prev })

	return New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{
				ProxyHosts: hosts,
				DNSProviders: []model.DNSProvider{{
					ObjectMeta: model.ObjectMeta{Name: "cf"},
					Provider:   "cloudflare",
					Config:     map[string]model.Secret{"apiToken": "${ENV:GPM_CF_TOKEN}"},
				}},
			}, model.Settings{
				DNSSync: model.DNSSyncSettings{Cloudflare: model.CloudflareDNSSync{
					Enabled:        true,
					DNSProviderRef: "cf",
					ZoneName:       "example.com",
					ApexTarget:     "edge.example.com",
					Proxied:        proxied,
				}},
			}, nil
	}, ledger)
}

func TestCloudflareReconcileCreates(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := cloudflareSyncer(t, srv, []model.ProxyHost{publicHost("app", "app.example.com")}, true)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Cloudflare
	if !st.OK || st.Created != 1 || st.Deleted != 0 {
		t.Fatalf("status = %+v", st)
	}
	got := fake.names()
	if len(got) != 1 || got[0] != "app.example.com|edge.example.com|"+ManagedComment {
		t.Fatalf("records = %v", got)
	}
	fake.mu.Lock()
	proxiedFlag := fake.records[0].Type
	fake.mu.Unlock()
	if proxiedFlag != "CNAME" {
		t.Fatalf("record type = %q", proxiedFlag)
	}
}

func TestCloudflareReconcileNoOpAndDelete(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: []cfRecord{
		{ID: "keep", Type: "CNAME", Name: "app.example.com", Content: "edge.example.com", Comment: ManagedComment},
		{ID: "stale", Type: "CNAME", Name: "old.example.com", Content: "edge.example.com", Comment: ManagedComment},
	}}
	srv := startCloudflare(t, fake)

	led := ownsCloudflare("app.example.com", "edge.example.com", "old.example.com", "edge.example.com")
	s := cloudflareSyncerWith(t, srv, []model.ProxyHost{publicHost("app", "app.example.com")}, false, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Cloudflare
	if st.Created != 0 || st.Deleted != 1 || st.Managed != 1 {
		t.Fatalf("status = %+v", st)
	}
	if got := fake.names(); len(got) != 1 || !strings.HasPrefix(got[0], "app.example.com|") {
		t.Fatalf("records = %v", got)
	}
	wantLedger(t, led.cloudflare(), "app.example.com", "edge.example.com")
}

// The Pi-hole incident, replayed against Cloudflare: the comment marker already
// made this safe, and the ledger must not have weakened it. Commented records
// that gpm never recorded creating are not deleted either - the ledger is
// authoritative, and the comment is only ever an additional condition.
func TestCloudflareFirstEnableDeletesNothing(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: []cfRecord{
		{ID: "hand-1", Type: "CNAME", Name: "docs.example.com", Content: "edge.example.com", Comment: ""},
		{ID: "hand-2", Type: "CNAME", Name: "blog.example.com", Content: "edge.example.com", Comment: "hand written"},
		{ID: "commented", Type: "CNAME", Name: "old.example.com", Content: "edge.example.com", Comment: ManagedComment},
	}}
	srv := startCloudflare(t, fake)

	led := &memLedger{}
	s := cloudflareSyncerWith(t, srv, nil, false, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	fake.mu.Lock()
	deleted := append([]string(nil), fake.deleted...)
	fake.mu.Unlock()
	if len(deleted) != 0 {
		t.Fatalf("REGRESSION: first reconcile with an empty ledger deleted %v", deleted)
	}
	st := s.Status().Cloudflare
	if st.Deleted != 0 || st.Created != 0 || st.Adopted != 0 || st.Untouched != 3 {
		t.Fatalf("status = %+v", st)
	}
	if len(led.cloudflare()) != 0 {
		t.Fatalf("ledger = %v, want empty", led.cloudflare())
	}
}

// Migration for an existing Cloudflare deployment: records the config wants that
// already carry the managed-by comment are adopted into the ledger on the first
// run, so they stay deletable later. A record with the right content but no
// comment is NOT adopted - Cloudflare deletion requires both marks.
func TestCloudflareAdoptsOnlyCommentedDesiredRecords(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: []cfRecord{
		{ID: "ours", Type: "CNAME", Name: "app.example.com", Content: "edge.example.com", Comment: ManagedComment},
		{ID: "theirs", Type: "CNAME", Name: "docs.example.com", Content: "edge.example.com", Comment: ""},
	}}
	srv := startCloudflare(t, fake)

	led := &memLedger{}
	s := cloudflareSyncerWith(t, srv, []model.ProxyHost{
		publicHost("app", "app.example.com"),
		publicHost("docs", "docs.example.com"),
	}, false, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Cloudflare
	if st.Adopted != 1 || st.Created != 0 || st.Deleted != 0 || st.Skipped != 1 {
		t.Fatalf("status = %+v, want one adoption and one skip", st)
	}
	wantLedger(t, led.cloudflare(), "app.example.com", "edge.example.com")
	if len(fake.names()) != 2 {
		t.Fatalf("records = %v, want both untouched", fake.names())
	}
}

// Records without the managed-by comment are never deleted, even when their name
// is one gpm no longer wants - and a name already owned by someone else is not
// overwritten with a second record.
func TestCloudflareNeverTouchesUnmanagedRecords(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: []cfRecord{
		{ID: "foreign-1", Type: "CNAME", Name: "docs.example.com", Content: "pages.dev", Comment: ""},
		{ID: "foreign-2", Type: "CNAME", Name: "app.example.com", Content: "somewhere.else", Comment: "hand written"},
		{ID: "ours", Type: "CNAME", Name: "old.example.com", Content: "edge.example.com", Comment: ManagedComment},
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	led := ownsCloudflare("old.example.com", "edge.example.com")
	s := cloudflareSyncerWith(t, srv, []model.ProxyHost{publicHost("app", "app.example.com")}, false, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	fake.mu.Lock()
	deleted := append([]string(nil), fake.deleted...)
	fake.mu.Unlock()
	if len(deleted) != 1 || deleted[0] != "ours" {
		t.Fatalf("deleted = %v, want only the gpm-managed record", deleted)
	}
	got := fake.names()
	want := []string{
		"app.example.com|somewhere.else|hand written",
		"docs.example.com|pages.dev|",
	}
	if strings.Join(got, "%") != strings.Join(want, "%") {
		t.Fatalf("records = %v\nwant %v", got, want)
	}
	st := s.Status().Cloudflare
	if st.Created != 0 {
		t.Fatalf("a name owned by a foreign record must not be created: %+v", st)
	}
}

// deleteRecord is the last line of defence: it re-checks ownership itself so it
// can never become an arbitrary-delete primitive.
func TestCloudflareDeleteRefusesUnmanaged(t *testing.T) {
	c := &cloudflareClient{token: "x", base: "http://127.0.0.1:1", client: http.DefaultClient}
	err := c.deleteRecord(context.Background(), "zone-1", cfRecord{ID: "abc", Name: "x.example.com", Comment: "someone else"})
	if err == nil || !strings.Contains(err.Error(), "not gpm-managed") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if err := c.deleteRecord(context.Background(), "zone-1", cfRecord{Comment: ManagedComment}); err == nil {
		t.Fatal("a record with no id must be refused")
	}
}

func TestCloudflareListPaginates(t *testing.T) {
	var recs []cfRecord
	for i := 0; i < 5; i++ {
		recs = append(recs, cfRecord{
			ID: "r" + strconv.Itoa(i), Type: "CNAME",
			Name: "h" + strconv.Itoa(i) + ".example.com", Content: "edge.example.com", Comment: ManagedComment,
		})
	}
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: recs, perPage: 2}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	// Desire every existing record so nothing is created or deleted: the only
	// way the counts come out right is if all 5 pages-worth were read.
	var hosts []model.ProxyHost
	for i := 0; i < 5; i++ {
		hosts = append(hosts, publicHost("h"+strconv.Itoa(i), "h"+strconv.Itoa(i)+".example.com"))
	}
	s := cloudflareSyncer(t, srv, hosts, false)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Cloudflare
	if st.Managed != 5 || st.Created != 0 || st.Deleted != 0 {
		t.Fatalf("status = %+v (pagination likely truncated the listing)", st)
	}
}

// Pagination must be driven by the page itself. result_info is advisory and can
// be missing entirely: trusting it as the sole terminator made a FULL page with
// no result_info look like the end of the zone, which silently hides records -
// orphaning ledger entries and re-attempting creates that already exist.
func TestCloudflareListPaginatesWithoutResultInfo(t *testing.T) {
	var recs []cfRecord
	for i := 0; i < cfPageSize+50; i++ {
		recs = append(recs, cfRecord{
			ID: "r" + strconv.Itoa(i), Type: "CNAME",
			Name: "h" + strconv.Itoa(i) + ".example.com", Content: "edge.example.com", Comment: ManagedComment,
		})
	}
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: recs, omitResultInfo: true}
	srv := startCloudflare(t, fake)

	c := &cloudflareClient{token: "cf-token", base: srv.URL, client: srv.Client()}
	got, err := c.listCNAMEs(context.Background(), "zone-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != len(recs) {
		t.Fatalf("REGRESSION: listed %d of %d records - pagination stopped at the first full page", len(got), len(recs))
	}
}

// A retarget deletes the record and then creates its replacement. When the create
// fails the name is left unresolved, so the original has to go back, the run has
// to fail loudly, and the status must not report a run that changed nothing.
func TestCloudflareRetargetRestoresTheOriginalWhenTheCreateFails(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: []cfRecord{
		{ID: "ours", Type: "CNAME", Name: "app.example.com", Content: "old-edge.example.com", Comment: ManagedComment, Proxied: true},
	}}
	fake.failCreateContent = "edge.example.com" // the new target, not the old one
	srv := startCloudflare(t, fake)

	led := ownsCloudflare("app.example.com", "old-edge.example.com")
	s := cloudflareSyncerWith(t, srv, []model.ProxyHost{publicHost("app", "app.example.com")}, true, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Cloudflare
	if st.OK || !strings.Contains(st.Error, "restored") {
		t.Fatalf("status = %+v, want a loud failure saying the original was restored", st)
	}
	if st.Retargeted != 1 {
		t.Fatalf("REGRESSION: status = %+v, a landed delete must be counted even when the create fails", st)
	}
	got := fake.names()
	if len(got) != 1 || !strings.HasPrefix(got[0], "app.example.com|old-edge.example.com|") {
		t.Fatalf("REGRESSION: records = %v, want the original record restored", got)
	}
	fake.mu.Lock()
	proxied := fake.records[0].Proxied
	fake.mu.Unlock()
	if !proxied {
		t.Fatal("the restored record must carry the orange-cloud flag it had before")
	}
}

// Cloudflare adoption is the same one-way trap as Pi-hole's if an adopted record
// can later be deleted: the record carries the comment because gpm ADOPTED it,
// not because gpm made it.
func TestCloudflareAdoptedRecordIsReleasedNotDeleted(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1", records: []cfRecord{
		{ID: "theirs", Type: "CNAME", Name: "app.example.com", Content: "edge.example.com", Comment: ManagedComment},
	}}
	srv := startCloudflare(t, fake)

	led := &memLedger{l: model.DNSLedger{Cloudflare: adoptedEntries("app.example.com", "edge.example.com")}}
	s := cloudflareSyncerWith(t, srv, nil, false, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if st := s.Status().Cloudflare; st.Deleted != 0 {
		t.Fatalf("REGRESSION: an adopted record was deleted: %+v", st)
	}
	fake.mu.Lock()
	deleted := len(fake.deleted)
	fake.mu.Unlock()
	if deleted != 0 {
		t.Fatalf("REGRESSION: %d delete calls were issued for an adopted record", deleted)
	}
	if len(led.cloudflare()) != 0 {
		t.Fatalf("ledger = %v, want the claim released", led.cloudflare())
	}
}

func TestCloudflareMissingProviderRef(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "example.com", zoneID: "zone-1"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	prev := cfBaseURL
	cfBaseURL = srv.URL
	defer func() { cfBaseURL = prev }()

	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{DNSSync: model.DNSSyncSettings{
			Cloudflare: model.CloudflareDNSSync{Enabled: true, DNSProviderRef: "nope", ZoneName: "example.com", ApexTarget: "edge.example.com"},
		}}, nil
	}, &memLedger{})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Cloudflare
	if st.OK || !strings.Contains(st.Error, "dnsProviderRef") {
		t.Fatalf("status = %+v", st)
	}
}

func TestCloudflareUnknownZone(t *testing.T) {
	fake := &fakeCloudflare{zoneName: "other.com", zoneID: "zone-9"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := cloudflareSyncer(t, srv, []model.ProxyHost{publicHost("app", "app.example.com")}, false)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Cloudflare
	if st.OK || !strings.Contains(st.Error, "no zone found") {
		t.Fatalf("status = %+v", st)
	}
}
