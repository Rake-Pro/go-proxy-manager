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
	logins       int
	logouts      int
	sawSID       []string
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
		recs := append([]string(nil), f.records...)
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

func piholeSyncer(t *testing.T, srv *httptest.Server, hosts []model.ProxyHost) *Syncer {
	t.Helper()
	t.Setenv("GPM_PIHOLE_PW", "secret")
	return New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{ProxyHosts: hosts}, model.Settings{
			DNSSync: model.DNSSyncSettings{Pihole: model.PiholeDNSSync{
				Enabled:     true,
				URL:         srv.URL,
				AppPassword: model.Secret("${ENV:GPM_PIHOLE_PW}"),
				ApexTarget:  "edge.example.com",
			}},
		}, nil
	})
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

func TestPiholeReconcileRemovesStale(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"gone.example.com,edge.example.com", // ours, no longer wanted
		"app.example.com,edge.example.com",  // ours, still wanted
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := piholeSyncer(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status().Pihole
	if st.Deleted != 1 || st.Created != 0 {
		t.Fatalf("status = %+v", st)
	}
	if got := fake.snapshot(); len(got) != 1 || got[0] != "app.example.com,edge.example.com" {
		t.Fatalf("records = %v", got)
	}
}

// The ownership rule: only CNAMEs whose target is exactly the configured apex
// belong to gpm. Everything else in Pi-hole is read and left strictly alone,
// even when it names a domain gpm also serves - and in particular a name an
// operator points elsewhere is NOT given a second, shadowing entry (the same
// skip-and-warn the Cloudflare backend does).
func TestPiholeNeverTouchesUnmanagedRecords(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"nas.example.com,truenas.lan",       // someone else's entry
		"app.example.com,other-proxy.lan",   // same domain, different target
		"legacy.example.com,edge.other.lan", // similar but not our apex
		"stale.example.com,edge.example.com",
	}}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	s := piholeSyncer(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")})
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

func TestEscapeRecord(t *testing.T) {
	if got := escapeRecord("app.example.com,edge.example.com"); got != "app.example.com%2Cedge.example.com" {
		t.Fatalf("escapeRecord = %q", got)
	}
}
