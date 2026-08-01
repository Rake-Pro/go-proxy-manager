package dnssync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// fakePihole is a hermetic stand-in for a Pi-hole v6 instance: it authenticates
// one application password, hands out a session id, and keeps an in-memory list
// of "domain,target" CNAME entries.
type fakePihole struct {
	mu       sync.Mutex
	password string
	sid      string
	records  []string
	// forbidConfig makes every config write answer 403, the shape Pi-hole uses
	// for a read-only session or an instance without app_sudo.
	forbidConfig bool
	// failAddRecord makes a PUT fail for exactly this record, so a test can fail
	// the create half of a retarget while leaving the rollback that restores the
	// original able to succeed.
	failAddRecord string
	logins        int
	logouts       int
	sawSID        []string
	// writes counts every mutating call (PUT/DELETE on cnameRecords), so a test
	// can assert that a dry run touched nothing at all.
	writes int
}

func (f *fakePihole) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if body.Password != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"session":{"valid":false}}`))
			return
		}
		f.logins++
		f.sid = "sid-123"
		_, _ = w.Write([]byte(`{"session":{"valid":true,"sid":"sid-123","csrf":"c"}}`))
	})
	mux.HandleFunc("DELETE /api/auth", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.logouts++
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/config/dns/cnameRecords", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkSID(w, r) {
			return
		}
		f.mu.Lock()
		// Always a list, never null: a real Pi-hole with no CNAMEs answers with an
		// empty array, and the distinction matters (see TestPiholeCnameRecordsShapes).
		recs := append([]string{}, f.records...)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"config": map[string]any{"dns": map[string]any{"cnameRecords": recs}},
		})
	})
	mux.HandleFunc("PUT /api/config/dns/cnameRecords/{rec}", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkSID(w, r) || !f.checkAllowed(w) {
			return
		}
		rec, err := url.PathUnescape(r.PathValue("rec"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		if f.failAddRecord != "" && rec == f.failAddRecord {
			f.mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		f.writes++
		f.records = append(f.records, rec)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("DELETE /api/config/dns/cnameRecords/{rec}", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkSID(w, r) || !f.checkAllowed(w) {
			return
		}
		rec, err := url.PathUnescape(r.PathValue("rec"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.writes++
		out := f.records[:0]
		for _, r0 := range f.records {
			if r0 != rec {
				out = append(out, r0)
			}
		}
		f.records = out
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func (f *fakePihole) checkSID(w http.ResponseWriter, r *http.Request) bool {
	f.mu.Lock()
	want := f.sid
	f.sawSID = append(f.sawSID, r.Header.Get("sid"))
	f.mu.Unlock()
	if r.Header.Get("sid") != want || want == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func (f *fakePihole) checkAllowed(w http.ResponseWriter) bool {
	f.mu.Lock()
	forbid := f.forbidConfig
	f.mu.Unlock()
	if forbid {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"key":"forbidden"}}`))
		return false
	}
	return true
}

// writeCount is the number of mutating calls the fake has served, read under the
// lock so a test can assert "nothing was written" without racing the handler.
func (f *fakePihole) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}

func (f *fakePihole) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]string(nil), f.records...)
	sort.Strings(out)
	return out
}

func lanHost(name string, domains ...string) model.ProxyHost {
	return model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: name},
		Domains:    domains,
		Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.5", Port: 8080},
		DNS:        &model.DNSSyncPolicy{LanDirect: true},
	}
}

func startPihole(t *testing.T, f *fakePihole) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return srv
}

// piholeSyncerWith builds a syncer against the fake Pi-hole with an explicit
// ownership ledger. Passing an empty ledger models a first enable: gpm owns
// nothing, so it may create and adopt but must delete nothing.
func piholeSyncerWith(t *testing.T, srv *httptest.Server, hosts []model.ProxyHost, ledger Ledger) *Syncer {
	t.Helper()
	return piholeSyncerApex(t, srv, hosts, ledger, "edge.example.com")
}

// piholeSyncerApex is the same with an explicit apexTarget, so a test can model
// the operator moving the edge host between two reconciles.
func piholeSyncerApex(t *testing.T, srv *httptest.Server, hosts []model.ProxyHost, ledger Ledger, apex string) *Syncer {
	t.Helper()
	t.Setenv("GPM_PIHOLE_PW", "secret")
	return New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{ProxyHosts: hosts}, model.Settings{
			DNSSync: model.DNSSyncSettings{Pihole: model.PiholeDNSSync{
				Enabled:     true,
				URL:         srv.URL,
				AppPassword: model.Secret("${ENV:GPM_PIHOLE_PW}"),
				ApexTarget:  apex,
			}},
		}, nil
	}, ledger)
}

func piholeSyncer(t *testing.T, srv *httptest.Server, hosts []model.ProxyHost) *Syncer {
	t.Helper()
	return piholeSyncerWith(t, srv, hosts, &memLedger{})
}

func TestPiholeReconcileAddsMissing(t *testing.T) {
	fake := &fakePihole{password: "secret"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := piholeSyncer(t, srv, []model.ProxyHost{lanHost("app", "app.example.com", "alt.example.com")})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if !st.OK {
		t.Fatalf("pihole status not ok: %+v", st)
	}
	if st.Created != 2 || st.Deleted != 0 || st.Desired != 2 {
		t.Fatalf("status = %+v", st)
	}
	want := []string{"alt.example.com,edge.example.com", "app.example.com,edge.example.com"}
	if got := fake.snapshot(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("records = %v, want %v", got, want)
	}
	// The session is opened once and explicitly released.
	if fake.logins != 1 || fake.logouts != 1 {
		t.Fatalf("logins=%d logouts=%d, want 1/1", fake.logins, fake.logouts)
	}
}

func TestPiholeReconcileIsIdempotent(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{"app.example.com,edge.example.com"}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := piholeSyncer(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if st.Created != 0 || st.Deleted != 0 {
		t.Fatalf("a matching backend must be a no-op, got %+v", st)
	}
	if len(fake.snapshot()) != 1 {
		t.Fatalf("records = %v", fake.snapshot())
	}
}

// Deletion is by ledger and by ledger alone: an entry gpm recorded and no longer
// wants goes, and the ledger loses it.
func TestPiholeReconcileRemovesStale(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"gone.example.com,edge.example.com", // ours, no longer wanted
		"app.example.com,edge.example.com",  // ours, still wanted
	}}
	srv := startPihole(t, fake)

	led := ownsPihole("gone.example.com", "edge.example.com", "app.example.com", "edge.example.com")
	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if st.Deleted != 1 || st.Created != 0 || st.Adopted != 0 {
		t.Fatalf("status = %+v", st)
	}
	if got := fake.snapshot(); len(got) != 1 || got[0] != "app.example.com,edge.example.com" {
		t.Fatalf("records = %v", got)
	}
	wantLedger(t, led.pihole(), "app.example.com", "edge.example.com")
}

// THE 2026-08-01 INCIDENT, as a test. An operator enables the Pi-hole backend for
// the first time. Their resolver already holds 19 hand-written LAN CNAMEs aimed
// at the very host apexTarget names (their LAN-direct bypass list). No proxy host
// carries dns.lanDirect yet, so the desired set is EMPTY, and the ledger is empty
// because gpm has never created anything.
//
// The old backend inferred ownership from target equality, decided all 19 were
// its own, found none of them wanted, and deleted every one. LAN DNS broke.
//
// The guarantee this asserts: a record gpm did not create is never deleted,
// however exactly its target matches. Zero deletions, zero writes of any kind.
func TestPiholeFirstEnableWithSharedApexDeletesNothing(t *testing.T) {
	var records []string
	for _, name := range []string{
		"plex", "argo", "cloud", "wiki", "paste", "speed", "pantry", "cdn", "go",
		"extensions", "gamewarden", "dotfiles", "ntfy", "grafana", "jackett",
		"qbit", "sonarr", "radarr", "nas",
	} {
		records = append(records, name+".example.com,edge.example.com")
	}
	fake := &fakePihole{password: "secret", records: records}
	srv := startPihole(t, fake)

	led := &memLedger{} // never reconciled before: gpm owns nothing
	s := piholeSyncerWith(t, srv, nil, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	st := s.Status().Pihole
	if st.Deleted != 0 {
		t.Fatalf("REGRESSION: first enable deleted %d hand-written records (status %+v)", st.Deleted, st)
	}
	if st.Created != 0 || st.Adopted != 0 {
		t.Fatalf("nothing is desired, so nothing may be created or adopted: %+v", st)
	}
	if st.Untouched != len(records) {
		t.Fatalf("untouched = %d, want all %d hand-written records reported as left alone", st.Untouched, len(records))
	}
	if got := fake.snapshot(); len(got) != len(records) {
		t.Fatalf("records = %d, want all %d still present:\n%v", len(got), len(records), got)
	}
	if len(led.pihole()) != 0 {
		t.Fatalf("gpm must not claim ownership of records it did not create: %v", led.pihole())
	}
}

// The migration case: a record the config DOES want is already there with the
// right target but predates the ledger. It is adopted - claimed, not recreated
// and above all not deleted - and everything else stays untouched.
func TestPiholeAdoptsMatchingRecordsOnFirstEnable(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"app.example.com,edge.example.com",  // desired, right target: adopt
		"plex.example.com,edge.example.com", // same target, NOT desired: leave alone
		"nas.example.com,truenas.lan",       // unrelated: leave alone
		"other.example.com,other-proxy.lan", // desired name, wrong target: skip
	}}
	srv := startPihole(t, fake)

	led := &memLedger{}
	s := piholeSyncerWith(t, srv, []model.ProxyHost{
		lanHost("app", "app.example.com"),
		lanHost("other", "other.example.com"),
	}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	st := s.Status().Pihole
	if st.Adopted != 1 || st.Created != 0 || st.Deleted != 0 || st.Skipped != 1 {
		t.Fatalf("status = %+v, want exactly one adoption and one skip", st)
	}
	if st.Untouched != 3 {
		t.Fatalf("untouched = %d, want the 3 records gpm has no claim on", st.Untouched)
	}
	// The record was adopted, not recreated: no duplicate entry appeared.
	got := fake.snapshot()
	want := []string{
		"app.example.com,edge.example.com",
		"nas.example.com,truenas.lan",
		"other.example.com,other-proxy.lan",
		"plex.example.com,edge.example.com",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("records = %v\nwant %v", got, want)
	}
	wantLedger(t, led.pihole(), "app.example.com", "edge.example.com")
	if !led.pihole()["app.example.com"].Adopted {
		t.Fatal("an adopted record must be recorded AS adopted, or gpm acquires the right to delete it")
	}

	// And the adoption is not a trap: dropping the host RELEASES the claim and
	// leaves the operator's record exactly where it was. gpm never created it, so
	// gpm never deletes it.
	s2 := piholeSyncerWith(t, srv, nil, led)
	if err := s2.Reconcile(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if st := s2.Status().Pihole; st.Deleted != 0 {
		t.Fatalf("REGRESSION: status = %+v, an adopted record must be released, not deleted", st)
	}
	if got := fake.snapshot(); len(got) != 4 {
		t.Fatalf("records = %v, want all four still present", got)
	}
	if len(led.pihole()) != 0 {
		t.Fatalf("ledger = %v, want the released claim dropped", led.pihole())
	}
}

// A ledger entry authorises a delete only while the record still holds what gpm
// wrote. Re-pointed out of band, it is disowned rather than deleted.
func TestPiholeDisownsRecordChangedOutOfBand(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"app.example.com,someone-elses-proxy.lan",
	}}
	srv := startPihole(t, fake)

	led := ownsPihole("app.example.com", "edge.example.com")
	s := piholeSyncerWith(t, srv, nil, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if st := s.Status().Pihole; st.Deleted != 0 {
		t.Fatalf("status = %+v, want no deletion of a record gpm no longer recognises", st)
	}
	if got := fake.snapshot(); len(got) != 1 || got[0] != "app.example.com,someone-elses-proxy.lan" {
		t.Fatalf("records = %v", got)
	}
	if len(led.pihole()) != 0 {
		t.Fatalf("ledger = %v, want the entry dropped", led.pihole())
	}
}

// Changing apexTarget used to orphan every record gpm had made, because ownership
// WAS the target. With the ledger, a record gpm created and nobody has touched is
// still recognisably gpm's, so it is retargeted rather than abandoned.
func TestPiholeRetargetsOwnedRecordsAfterApexChange(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"app.example.com,old-edge.example.com",
		"hand.example.com,old-edge.example.com", // not ours: must not move
	}}
	srv := startPihole(t, fake)

	led := ownsPihole("app.example.com", "old-edge.example.com")
	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if st.Retargeted != 1 || st.Created != 0 || st.Deleted != 0 {
		t.Fatalf("status = %+v, want one retarget", st)
	}
	got := fake.snapshot()
	want := []string{"app.example.com,edge.example.com", "hand.example.com,old-edge.example.com"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("records = %v\nwant %v", got, want)
	}
	wantLedger(t, led.pihole(), "app.example.com", "edge.example.com")
	if led.pihole()["app.example.com"].Adopted {
		t.Fatal("a record gpm created and retargeted must stay recorded as created")
	}
}

// The same apex change over a record gpm only ADOPTED. A retarget is a delete
// plus a create, so retargeting here would destroy the operator's record and then
// record its replacement as gpm-created - which a later host removal would be free
// to hard-delete. The claim is released instead: nothing is written at all.
func TestPiholeReleasesAdoptedRecordWhenApexChanges(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"app.example.com,old-edge.example.com", // hand-written, adopted by gpm
	}}
	srv := startPihole(t, fake)

	led := &memLedger{l: model.DNSLedger{
		Pihole: adoptedEntries("app.example.com", "old-edge.example.com"),
	}}
	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if st.Retargeted != 0 || st.Deleted != 0 || st.Created != 0 {
		t.Fatalf("REGRESSION: status = %+v, an adopted record must never be retargeted or deleted", st)
	}
	if st.Skipped != 1 {
		t.Fatalf("status = %+v, want the released name reported as skipped", st)
	}
	if fake.writeCount() != 0 {
		t.Fatalf("REGRESSION: %d backend writes, want the operator's record touched not at all", fake.writeCount())
	}
	if got := fake.snapshot(); len(got) != 1 || got[0] != "app.example.com,old-edge.example.com" {
		t.Fatalf("REGRESSION: records = %v, want the operator's record exactly as they wrote it", got)
	}
	if len(led.pihole()) != 0 {
		t.Fatalf("ledger = %v, want the adopted claim released", led.pihole())
	}
}

// Provenance is carried, not recomputed: an ordinary reconcile over an adopted
// record that is already exactly right must leave the claim adopted. Reset it to
// "created" and the next removal deletes somebody else's record.
func TestPiholeAdoptedClaimStaysAdoptedAcrossReconciles(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{"app.example.com,edge.example.com"}}
	srv := startPihole(t, fake)

	led := &memLedger{l: model.DNSLedger{
		Pihole: adoptedEntries("app.example.com", "edge.example.com"),
	}}
	hosts := []model.ProxyHost{lanHost("app", "app.example.com")}
	for i := range 3 {
		s := piholeSyncerWith(t, srv, hosts, led)
		if err := s.Reconcile(context.Background()); err != nil {
			t.Fatalf("reconcile %d: %v", i, err)
		}
		if !led.pihole()["app.example.com"].Adopted {
			t.Fatalf("REGRESSION: reconcile %d upgraded an adopted claim to created", i)
		}
	}
	if fake.writeCount() != 0 {
		t.Fatalf("REGRESSION: %d backend writes for a record that was already correct", fake.writeCount())
	}
}

// The whole sequence, end to end: adopt a hand-written record, move the edge host,
// then take dns.lanDirect off the proxy host. Each step on its own looks harmless;
// before the release-on-retarget fix the three together reproduced the 2026-08-01
// incident one record at a time, because step two silently re-recorded the
// operator's record as one gpm had created.
func TestPiholeAdoptThenApexChangeThenRemovalKeepsTheOperatorsRecord(t *testing.T) {
	const original = "app.example.com,old-edge.example.com"
	fake := &fakePihole{password: "secret", records: []string{original}}
	srv := startPihole(t, fake)

	led := &memLedger{}
	hosts := []model.ProxyHost{lanHost("app", "app.example.com")}

	// Step 1: dns.lanDirect goes on while apexTarget is still the old edge host, so
	// the operator's record is adopted rather than recreated.
	s1 := piholeSyncerApex(t, srv, hosts, led, "old-edge.example.com")
	if err := s1.Reconcile(context.Background()); err != nil {
		t.Fatalf("adopting reconcile: %v", err)
	}
	if st := s1.Status().Pihole; st.Adopted != 1 || st.Created != 0 {
		t.Fatalf("status = %+v, want the hand-written record adopted", st)
	}

	// Step 2: the edge host moves. The claim is released, not retargeted.
	s2 := piholeSyncerApex(t, srv, hosts, led, "new-edge.example.com")
	if err := s2.Reconcile(context.Background()); err != nil {
		t.Fatalf("apex-change reconcile: %v", err)
	}
	if st := s2.Status().Pihole; st.Retargeted != 0 || st.Deleted != 0 {
		t.Fatalf("REGRESSION: status = %+v, want the adopted claim released", st)
	}

	// Step 3: the flag comes off again. There is no claim left to authorise
	// anything, and there never was one gpm could have deleted on.
	s3 := piholeSyncerApex(t, srv, nil, led, "new-edge.example.com")
	if err := s3.Reconcile(context.Background()); err != nil {
		t.Fatalf("removal reconcile: %v", err)
	}
	if st := s3.Status().Pihole; st.Deleted != 0 {
		t.Fatalf("REGRESSION: status = %+v, the operator's record was deleted", st)
	}
	if got := fake.snapshot(); len(got) != 1 || got[0] != original {
		t.Fatalf("REGRESSION: records = %v, want %q still exactly as the operator wrote it", got, original)
	}
	if len(led.pihole()) != 0 {
		t.Fatalf("ledger = %v, want no claim left", led.pihole())
	}
}

// The ownership rule: only CNAMEs recorded in the ledger belong to gpm.
// Everything else in Pi-hole is read and left strictly alone, even when it names
// a domain gpm also serves - and in particular a name an operator points
// elsewhere is NOT given a second, shadowing entry (the same skip-and-warn the
// Cloudflare backend does).
func TestPiholeNeverTouchesUnmanagedRecords(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"nas.example.com,truenas.lan",       // someone else's entry
		"app.example.com,other-proxy.lan",   // same domain, different target
		"legacy.example.com,edge.other.lan", // similar but not our apex
		"stale.example.com,edge.example.com",
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	// stale is the only one gpm ever created, so it is the only one that can go -
	// note that legacy and nas are NOT in the ledger and survive regardless.
	led := ownsPihole("stale.example.com", "edge.example.com")
	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := fake.snapshot()
	want := []string{
		"app.example.com,other-proxy.lan",   // operator-owned: kept, and NOT shadowed
		"legacy.example.com,edge.other.lan", // untouched
		"nas.example.com,truenas.lan",       // untouched
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("records = %v\nwant %v", got, want)
	}
	st := s.Status().Pihole
	if st.Created != 0 {
		t.Fatalf("status = %+v (an operator-owned CNAME for the same name must not be overwritten)", st)
	}
	if st.Deleted != 1 {
		t.Fatalf("status = %+v (expected exactly the stale gpm record removed)", st)
	}
}

// A desired name that is free is still created even while another name in the
// zone is operator-owned: the skip is per-name, not a whole-run bail-out.
func TestPiholeSkipsOnlyTheConflictingName(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"taken.example.com,other-proxy.lan",
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := piholeSyncer(t, srv, []model.ProxyHost{lanHost("app", "taken.example.com", "free.example.com")})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	want := []string{"free.example.com,edge.example.com", "taken.example.com,other-proxy.lan"}
	if got := fake.snapshot(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("records = %v\nwant %v", got, want)
	}
	if st := s.Status().Pihole; st.Created != 1 || st.Deleted != 0 {
		t.Fatalf("status = %+v, want exactly one create", st)
	}
}

// Disabled hosts and hosts not opted into lanDirect contribute nothing, and
// wildcard domains are skipped (dnsmasq CNAMEs are exact-name only).
func TestPiholeDesiredSetFiltering(t *testing.T) {
	fake := &fakePihole{password: "secret"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	optedOut := lanHost("nosync", "nosync.example.com")
	optedOut.DNS = nil
	disabled := lanHost("off", "off.example.com")
	disabled.Disabled = true
	wildcard := lanHost("wild", "*.wild.example.com", "wild.example.com")
	selfTarget := lanHost("apex", "edge.example.com")

	s := piholeSyncer(t, srv, []model.ProxyHost{optedOut, disabled, wildcard, selfTarget})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := fake.snapshot()
	if len(got) != 1 || got[0] != "wild.example.com,edge.example.com" {
		t.Fatalf("records = %v", got)
	}
}

func TestPiholeForbiddenIsDistinct(t *testing.T) {
	fake := &fakePihole{password: "secret", forbidConfig: true}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := piholeSyncer(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile must not fail the call: %v", err)
	}
	st := s.Status().Pihole
	if st.OK {
		t.Fatal("a 403 must not report success")
	}
	if !strings.Contains(st.Error, "app_sudo") {
		t.Fatalf("403 must surface as the distinct read-only/app_sudo error, got %q", st.Error)
	}
	// The session is still released even on the failure path.
	if fake.logouts != 1 {
		t.Fatalf("logouts = %d, want 1", fake.logouts)
	}
}

func TestPiholeBadPasswordReported(t *testing.T) {
	fake := &fakePihole{password: "different"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := piholeSyncer(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if st.OK || !strings.Contains(st.Error, "appPassword") {
		t.Fatalf("status = %+v", st)
	}
}

// THE ADOPTION TRAP. An operator hand-writes a LAN CNAME. Later a proxy host is
// given dns.lanDirect for that same name, so gpm adopts the record. Later still
// the flag is removed. Before this was fixed the next reconcile DELETED their
// record: adoption had quietly converted somebody else's record into one gpm
// believed it had made. The incident, deferred by one config edit.
func TestPiholeAdoptionIsNotAOneWayTrapToDeletion(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{"x.example.com,edge.example.com"}}
	srv := startPihole(t, fake)

	led := &memLedger{}
	// Step 1: the operator turns on dns.lanDirect for a name they wrote by hand.
	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("x", "x.example.com")}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("adopting reconcile: %v", err)
	}
	if st := s.Status().Pihole; st.Adopted != 1 || st.Created != 0 {
		t.Fatalf("status = %+v, want the hand-written record adopted", st)
	}

	// Step 2: they change their mind and take the flag off again.
	s2 := piholeSyncerWith(t, srv, nil, led)
	if err := s2.Reconcile(context.Background()); err != nil {
		t.Fatalf("releasing reconcile: %v", err)
	}
	st := s2.Status().Pihole
	if st.Deleted != 0 {
		t.Fatalf("REGRESSION: adoption became a licence to delete: %+v", st)
	}
	if got := fake.snapshot(); len(got) != 1 || got[0] != "x.example.com,edge.example.com" {
		t.Fatalf("REGRESSION: the operator's record is %v, want it untouched", got)
	}
	if len(led.pihole()) != 0 {
		t.Fatalf("ledger = %v, want the claim released", led.pihole())
	}
}

// A ledger written before provenance was recorded says nothing about who made
// each record. The only reading of that silence which cannot destroy an
// operator's record is "adopted", so an upgrade must never delete on the strength
// of a legacy entry.
func TestPiholeLegacyLedgerEntryIsNeverDeleted(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{"old.example.com,edge.example.com"}}
	srv := startPihole(t, fake)

	// No `adopted` field at all - exactly what the previous version wrote.
	led := &memLedger{l: model.DNSLedger{Pihole: []model.DNSLedgerEntry{
		{Domain: "old.example.com", Target: "edge.example.com"},
	}}}
	s := piholeSyncerWith(t, srv, nil, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if st := s.Status().Pihole; st.Deleted != 0 {
		t.Fatalf("REGRESSION: an upgrade deleted on the strength of a ledger entry with no provenance: %+v", st)
	}
	if got := fake.snapshot(); len(got) != 1 {
		t.Fatalf("records = %v, want the record left in place", got)
	}
}

// A record gpm CREATED is still deleted when it is no longer wanted - the point
// is that gpm deletes what it made, not that it stops deleting.
func TestPiholeCreatedRecordIsStillDeleted(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{"gone.example.com,edge.example.com"}}
	srv := startPihole(t, fake)

	led := ownsPihole("gone.example.com", "edge.example.com") // recorded as created
	s := piholeSyncerWith(t, srv, nil, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if st := s.Status().Pihole; st.Deleted != 1 {
		t.Fatalf("status = %+v, want the record gpm created removed", st)
	}
	if got := fake.snapshot(); len(got) != 0 {
		t.Fatalf("records = %v, want gpm's own record gone", got)
	}
}

// A delete is the one operation that destroys something, and the authority for it
// is a recorded claim that a config revert can make older than the tree it is
// applied to. It has to be loud, and it has to name the revision it read.
func TestPiholeDeleteLogsTheClaimItActedOn(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{"gone.example.com,edge.example.com"}}
	srv := startPihole(t, fake)

	led := ownsPihole("gone.example.com", "edge.example.com")
	led.rev = "abc123"
	logs := captureLogs(t)
	s := piholeSyncerWith(t, srv, nil, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	out := logs.String()
	if !strings.Contains(out, `"level":"warn"`) || !strings.Contains(out, "on the authority of the ownership ledger") {
		t.Fatalf("a deletion must warn about the claim it acted on, logs:\n%s", out)
	}
	if !strings.Contains(out, `"ledgerRev":"abc123"`) {
		t.Fatalf("a deletion must name the ledger revision that authorised it, logs:\n%s", out)
	}
}

// The session must be released even when the run was cut short. logout runs on a
// deferred path, so reusing the caller's (cancelled) context is exactly how a
// disconnecting HTTP client leaks a Pi-hole session slot - and Pi-hole has very
// few of them.
func TestPiholeLogoutSurvivesCancelledContext(t *testing.T) {
	fake := &fakePihole{password: "secret"}
	srv := startPihole(t, fake)

	c := newPiholeClient(srv.URL, "secret", srv.Client())
	ctx, cancel := context.WithCancel(context.Background())
	if err := c.login(ctx); err != nil {
		t.Fatalf("login: %v", err)
	}
	cancel() // the caller went away mid-run
	c.logout(ctx)

	fake.mu.Lock()
	logins, logouts := fake.logins, fake.logouts
	fake.mu.Unlock()
	if logins != 1 || logouts != 1 {
		t.Fatalf("REGRESSION: logins=%d logouts=%d - a cancelled caller leaked the session", logins, logouts)
	}
}

// A retarget is a delete followed by a create. If the create fails the record is
// simply gone, and "it heals on the next reconcile" is not an answer for a name
// that stops resolving in the meantime: the original must be put back, the run
// must fail loudly, and the status must not claim nothing happened.
func TestPiholeRetargetRestoresTheOriginalWhenTheCreateFails(t *testing.T) {
	fake := &fakePihole{
		password:      "secret",
		records:       []string{"app.example.com,old-edge.example.com"},
		failAddRecord: "app.example.com,edge.example.com", // only the NEW record fails to land
	}
	srv := startPihole(t, fake)

	led := ownsPihole("app.example.com", "old-edge.example.com")
	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if st.OK || st.Error == "" {
		t.Fatalf("a destroyed-and-not-replaced record must fail the run: %+v", st)
	}
	if !strings.Contains(st.Error, "restored") {
		t.Fatalf("error = %q, want it to say the original was restored", st.Error)
	}
	if st.Retargeted != 1 {
		t.Fatalf("REGRESSION: status = %+v, a landed delete must be counted even when the create fails", st)
	}
	if got := fake.snapshot(); len(got) != 1 || got[0] != "app.example.com,old-edge.example.com" {
		t.Fatalf("REGRESSION: records = %v, want the original record restored", got)
	}
	// The claim survives with its original target, matching what is actually there.
	wantLedger(t, led.pihole(), "app.example.com", "old-edge.example.com")
}

// A Pi-hole that answers with a shape this code cannot read must be an ERROR. A
// nil slice reads as "the resolver holds nothing", which a full-state reconciler
// answers by emptying the ledger and reporting a clean run - the same failure
// class as a frozen client returning no objects.
func TestPiholeCnameRecordsShapes(t *testing.T) {
	tests := []struct {
		name, body string
		wantErr    bool
		wantLen    int
	}{
		{"well formed", `{"config":{"dns":{"cnameRecords":["a.example.com,edge.example.com"]}}}`, false, 1},
		{"legitimately empty", `{"config":{"dns":{"cnameRecords":[]}}}`, false, 0},
		{"field renamed", `{"config":{"dns":{"cnames":["a.example.com,edge.example.com"]}}}`, true, 0},
		{"field null", `{"config":{"dns":{"cnameRecords":null}}}`, true, 0},
		{"dns section gone", `{"config":{}}`, true, 0},
		{"envelope gone", `{}`, true, 0},
		{"error body", `{"error":{"key":"bad_request"}}`, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c := newPiholeClient(srv.URL, "secret", srv.Client())
			c.sid = "sid-123"
			got, err := c.cnameRecords(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("REGRESSION: %q decoded to %v records instead of failing", tc.body, len(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("records = %v, want %d", got, tc.wantLen)
			}
		})
	}
}

// And the same shape change, seen through a whole reconcile: it must report a
// failed run rather than a successful one that wiped the ledger.
func TestPiholeUnreadableListingDoesNotWipeTheLedger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"session":{"valid":true,"sid":"sid-123"}}`))
			return
		}
		// The listing endpoint answers with a renamed field.
		_, _ = w.Write([]byte(`{"config":{"dns":{"cnames":[]}}}`))
	}))
	defer srv.Close()

	led := ownsPihole("app.example.com", "edge.example.com")
	s := piholeSyncerWith(t, srv, nil, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if st := s.Status().Pihole; st.OK || st.Error == "" {
		t.Fatalf("REGRESSION: an unreadable listing reported a clean run: %+v", st)
	}
	if len(led.pihole()) != 1 {
		t.Fatalf("REGRESSION: the ledger was emptied by an unreadable listing: %v", led.pihole())
	}
}

func TestEscapeRecord(t *testing.T) {
	if got := escapeRecord("app.example.com,edge.example.com"); got != "app.example.com%2Cedge.example.com" {
		t.Fatalf("escapeRecord = %q", got)
	}
}
