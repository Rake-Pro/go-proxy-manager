package accesssync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// memLedger is the in-memory Ledger the tests run against: it records how many
// times a save happened, which is how "an unchanged feed costs no commit" is
// asserted.
type memLedger struct {
	mu    sync.Mutex
	l     model.AccessListSourceLedger
	saves int
}

func (m *memLedger) Load(context.Context) (model.AccessListSourceLedger, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.l, "rev", nil
}

func (m *memLedger) Save(_ context.Context, l model.AccessListSourceLedger, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := l.Validate(); err != nil {
		return err
	}
	m.l = l
	m.saves++
	return nil
}

func (m *memLedger) state() (model.AccessListSourceLedger, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.l, m.saves
}

// testSyncer wires a syncer against one source pointed at ts, with the default
// hardened client swapped for the test server's own - the real client's dialer
// refuses loopback on purpose (see TestSSRFGuardRefusesALoopbackSource), so a
// httptest server is unreachable through it.
func testSyncer(t *testing.T, ts *httptest.Server, src model.AccessListSource, led *memLedger) *Syncer {
	t.Helper()
	src.URL = ts.URL
	cfg := model.Config{AccessLists: []model.AccessList{{
		ObjectMeta: model.ObjectMeta{Name: "home-vpn"},
		Sources:    []model.AccessListSource{src},
		Rules:      []model.IPRule{{Action: model.ActionAllow, Source: src.Name, Paths: []string{"/health"}}},
	}}}
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return cfg, model.Settings{}, nil
	}, led, nil)
	s.client = ts.Client()
	return s
}

func serving(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

const uptimeRobotish = `# UptimeRobot monitoring IPs
# IPv4

203.0.113.10
203.0.113.11
198.51.100.0/24

# IPv6
2001:db8:1::5
2001:db8:2::/48
`

func TestFetchParsesTheSourceFormat(t *testing.T) {
	ts := serving(t, http.StatusOK, uptimeRobotish)
	led := &memLedger{}
	s := testSyncer(t, ts, model.AccessListSource{Name: "uptimerobot"}, led)

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	l, saves := led.state()
	if saves != 1 {
		t.Fatalf("saves = %d, want 1", saves)
	}
	if len(l.Sources) != 1 {
		t.Fatalf("ledger = %+v", l)
	}
	got := l.Sources[0]
	want := []string{
		"198.51.100.0/24", "2001:db8:1::5/128", "2001:db8:2::/48",
		"203.0.113.10/32", "203.0.113.11/32",
	}
	// Sorted lexically, which is what the ledger stores; bare IPs became /32 and
	// /128, comments and blank lines were dropped.
	if len(got.Entries) != len(want) {
		t.Fatalf("entries = %v, want %v", got.Entries, want)
	}
	for _, w := range want {
		var found bool
		for _, e := range got.Entries {
			if e == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("entries = %v, missing %q", got.Entries, w)
		}
	}
	for i := 1; i < len(got.Entries); i++ {
		if got.Entries[i-1] > got.Entries[i] {
			t.Fatalf("entries are not sorted: %v", got.Entries)
		}
	}
	if got.SHA256 != model.AccessListSourceHash(got.Key(), got.URL, got.Entries) {
		t.Fatal("the recorded hash must cover the stored entries")
	}
	if got.Fetched().IsZero() {
		t.Fatalf("fetchedAt = %q, want an RFC3339 timestamp", got.FetchedAt)
	}
	if got.List != "home-vpn" || got.Source != "uptimerobot" || got.URL != ts.URL {
		t.Fatalf("entry identity = %+v", got)
	}
}

// Every refusal keeps the PREVIOUS set: a feed that returns junk, an error, or
// nothing at all must never be able to empty (or widen) a live allow list.
func TestFetchRefusals(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		src     model.AccessListSource
		wantErr string
	}{
		{name: "non-200", status: http.StatusInternalServerError, body: "203.0.113.1", wantErr: "HTTP 500"},
		{name: "not found", status: http.StatusNotFound, body: "", wantErr: "HTTP 404"},
		{name: "zero entries", status: http.StatusOK, body: "# nothing but comments\n\n", wantErr: "no valid entries"},
		{name: "empty body", status: http.StatusOK, body: "", wantErr: "no valid entries"},
		{
			name: "one invalid line refuses the whole body", status: http.StatusOK,
			body: "203.0.113.1\n<html>maintenance</html>\n203.0.113.2\n", wantErr: "is not an IP or CIDR",
		},
		{
			// A hijacked feed with one allow-the-world line must not be able to
			// replace a live allow list, however plausible the rest of it is.
			name: "a default route refuses the whole body", status: http.StatusOK,
			body: "203.0.113.1\n0.0.0.0/0\n203.0.113.2\n", wantErr: "is the default route",
		},
		{
			name: "a private range refuses the whole body", status: http.StatusOK,
			body: "203.0.113.1\n10.0.0.0/8\n", wantErr: "not a public address range",
		},
		{
			name: "over maxEntries", status: http.StatusOK,
			body: "203.0.113.1\n203.0.113.2\n203.0.113.3\n",
			src:  model.AccessListSource{MaxEntries: 2}, wantErr: "more than maxEntries",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := serving(t, tt.status, tt.body)
			prev := []string{"192.0.2.0/24"}
			led := &memLedger{l: model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{{
				List: "home-vpn", Source: "uptimerobot", URL: ts.URL,
				FetchedAt: time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339),
				SHA256:    model.AccessListSourceHash("home-vpn/uptimerobot", ts.URL, prev), Entries: prev,
			}}}}
			src := tt.src
			src.Name = "uptimerobot"
			s := testSyncer(t, ts, src, led)

			if err := s.Reconcile(context.Background()); err != nil {
				t.Fatalf("a refused source must not fail the whole run: %v", err)
			}
			l, saves := led.state()
			if saves != 0 {
				t.Fatalf("a refused fetch must not rewrite the ledger (saves = %d)", saves)
			}
			if len(l.Sources) != 1 || len(l.Sources[0].Entries) != 1 || l.Sources[0].Entries[0] != "192.0.2.0/24" {
				t.Fatalf("previous set was not kept: %+v", l)
			}
			st := s.Status()
			if st.Refused != 1 {
				t.Fatalf("refused = %d, want 1", st.Refused)
			}
			if len(st.Sources) != 1 || !strings.Contains(st.Sources[0].LastError, tt.wantErr) {
				t.Fatalf("status = %+v, want a lastError containing %q", st.Sources, tt.wantErr)
			}
		})
	}
}

// An unchanged feed must produce no ledger write at all: writing a fresh
// fetchedAt every 24h would commit a timestamp-only diff to the config repo
// forever.
func TestUnchangedSetWritesNoLedger(t *testing.T) {
	ts := serving(t, http.StatusOK, uptimeRobotish)
	led := &memLedger{}
	s := testSyncer(t, ts, model.AccessListSource{Name: "uptimerobot", Interval: "1h"}, led)

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if _, saves := led.state(); saves != 1 {
		t.Fatalf("saves after first run = %d, want 1", saves)
	}
	// Force the source to look due again, so the second run really does fetch.
	s.clearAttempt(model.AccessListSourceKey("home-vpn", "uptimerobot"))
	led.mu.Lock()
	led.l.Sources[0].FetchedAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	led.mu.Unlock()

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if _, saves := led.state(); saves != 1 {
		t.Fatalf("saves after an unchanged re-fetch = %d, want it to stay 1", saves)
	}
}

// A changed feed replaces the set, saves, and reloads the data plane.
func TestChangedSetSavesAndReloads(t *testing.T) {
	body := "203.0.113.1\n"
	var mu sync.Mutex
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	led := &memLedger{}
	reloads := 0
	cfg := model.Config{AccessLists: []model.AccessList{{
		ObjectMeta: model.ObjectMeta{Name: "home-vpn"},
		Sources:    []model.AccessListSource{{Name: "uptimerobot", URL: ts.URL, Interval: "1h"}},
	}}}
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return cfg, model.Settings{}, nil
	}, led, func() { reloads++ })
	s.client = ts.Client()

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want 1 after the first fetch", reloads)
	}

	mu.Lock()
	body = "203.0.113.9\n203.0.113.1\n"
	mu.Unlock()
	s.clearAttempt(model.AccessListSourceKey("home-vpn", "uptimerobot"))
	led.mu.Lock()
	led.l.Sources[0].FetchedAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	led.mu.Unlock()

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("second: %v", err)
	}
	l, saves := led.state()
	if saves != 2 || reloads != 2 {
		t.Fatalf("saves = %d, reloads = %d, want 2 and 2", saves, reloads)
	}
	got := l.Sources[0].Entries
	if len(got) != 2 || got[0] != "203.0.113.1/32" || got[1] != "203.0.113.9/32" {
		t.Fatalf("entries = %v, want the new set, sorted", got)
	}
}

// A refused fetch does not start the interval clock: retrying only after the
// source's own interval would leave a list pinned to a stale set for up to a day
// after one transient blip. The next poll tick retries.
func TestRefusedFetchIsRetriedOnTheNextPoll(t *testing.T) {
	hits := 0
	fail := true
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("203.0.113.1\n"))
	}))
	defer ts.Close()

	led := &memLedger{}
	s := testSyncer(t, ts, model.AccessListSource{Name: "uptimerobot", Interval: "24h"}, led)
	now := time.Now()
	s.now = func() time.Time { return now }

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	if hits != 1 || s.Status().Refused != 1 {
		t.Fatalf("hits = %d, refused = %d, want a refused first fetch", hits, s.Status().Refused)
	}
	// One poll tick later - nowhere near the 24h interval.
	now = now.Add(15 * time.Minute)
	fail = false
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("second: %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want the refused source retried on the next poll", hits)
	}
	if _, saves := led.state(); saves != 1 {
		t.Fatalf("saves = %d, want the retry to have landed", saves)
	}
	// Now that it HAS succeeded, the interval clock is running.
	now = now.Add(time.Hour)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("third: %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want the interval to gate once a fetch succeeded", hits)
	}
}

// The SSRF guard inspects the address actually dialled. Through an HTTP proxy
// that address is the PROXY, so every internal destination would sail past the
// check - an environment variable must not be able to disable a security control.
func TestSyncClientUsesNoProxy(t *testing.T) {
	s := New(nil, nil, nil)
	tr, ok := s.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T", s.client.Transport)
	}
	if tr.Proxy != nil {
		t.Fatal("the source-fetch client must not honour a proxy: it would bypass the dialer's SSRF guard")
	}
}

// A source is only fetched once its interval has elapsed, so a 15m poll does not
// hammer a feed configured for 24h.
func TestIntervalGating(t *testing.T) {
	hits := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("203.0.113.1\n"))
	}))
	defer ts.Close()

	led := &memLedger{}
	s := testSyncer(t, ts, model.AccessListSource{Name: "uptimerobot", Interval: "24h"}, led)
	now := time.Now()
	s.now = func() time.Time { return now }

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want the first run to fetch", hits)
	}
	// An hour later: not due.
	now = now.Add(time.Hour)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("second: %v", err)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want no fetch before the interval elapses", hits)
	}
	// A day later: due again.
	now = now.Add(24 * time.Hour)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("third: %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want a re-fetch once the interval elapsed", hits)
	}
}

// The fetcher is off when settings say so, and never touches the network then.
func TestDisabledSyncFetchesNothing(t *testing.T) {
	hits := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("203.0.113.1\n"))
	}))
	defer ts.Close()

	off := false
	led := &memLedger{}
	cfg := model.Config{AccessLists: []model.AccessList{{
		ObjectMeta: model.ObjectMeta{Name: "home-vpn"},
		Sources:    []model.AccessListSource{{Name: "uptimerobot", URL: ts.URL}},
	}}}
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return cfg, model.Settings{AccessListSync: model.AccessListSyncSettings{Enabled: &off}}, nil
	}, led, nil)
	s.client = ts.Client()

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if hits != 0 {
		t.Fatalf("hits = %d, want none while accessListSync is disabled", hits)
	}
	if s.Status().Enabled {
		t.Fatal("status must report the fetcher as disabled")
	}
}

// A source the config no longer declares stops being carried, so a set nothing
// references cannot keep granting access.
func TestDroppedSourceLeavesTheLedger(t *testing.T) {
	stale := []string{"192.0.2.0/24"}
	led := &memLedger{l: model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{{
		List: "home-vpn", Source: "gone", URL: "https://example.com/i.txt",
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		SHA256:    model.AccessListSourceHash("home-vpn/gone", "https://example.com/i.txt", stale), Entries: stale,
	}}}}
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{}, nil
	}, led, nil)

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	l, saves := led.state()
	if saves != 1 || len(l.Sources) != 0 {
		t.Fatalf("ledger = %+v (saves %d), want the undeclared source dropped", l, saves)
	}
}

// The SSRF guard is the reason a source URL cannot be aimed at something
// internal. httptest listens on loopback, which is exactly what the real
// (unswapped) client must refuse.
func TestSSRFGuardRefusesALoopbackSource(t *testing.T) {
	ts := serving(t, http.StatusOK, "203.0.113.1\n")
	led := &memLedger{}
	cfg := model.Config{AccessLists: []model.AccessList{{
		ObjectMeta: model.ObjectMeta{Name: "home-vpn"},
		// A perfectly well-formed https source URL: what refuses it is the DIALER,
		// on the destination address, not the scheme check above it.
		Sources: []model.AccessListSource{{Name: "uptimerobot", URL: ts.URL}},
	}}}
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return cfg, model.Settings{}, nil
	}, led, nil)

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, saves := led.state(); saves != 0 {
		t.Fatalf("saves = %d, want the loopback destination refused", saves)
	}
	st := s.Status()
	if st.Refused != 1 || len(st.Sources) != 1 ||
		!strings.Contains(st.Sources[0].LastError, "internal destination") {
		t.Fatalf("status = %+v, want the dialer's refusal", st.Sources)
	}
}

func TestRefuseInternalDest(t *testing.T) {
	for _, addr := range []string{
		"127.0.0.1:443", "[::1]:443", "10.1.2.3:443", "192.168.1.1:443", "172.16.0.1:443",
		"169.254.169.254:80", "[fd00::1]:443", "[fe80::1]:443", "224.0.0.1:443", "0.0.0.0:443",
		// Blocks net.IP's own predicates miss, shared with the entry filter.
		"100.64.1.1:443",       // CGNAT
		"192.0.0.8:443",        // IETF protocol assignments
		"198.18.5.5:443",       // benchmarking
		"[64:ff9b::a00:1]:443", // NAT64-wrapped 10.0.0.1
	} {
		if err := refuseInternalDest("tcp", addr, nil); err == nil {
			t.Fatalf("%s must be refused", addr)
		}
	}
	for _, addr := range []string{"203.0.113.10:443", "[2001:db8::1]:443"} {
		if err := refuseInternalDest("tcp", addr, nil); err != nil {
			t.Fatalf("%s must be allowed: %v", addr, err)
		}
	}
}

// A manual reconcile refuses rather than queueing behind a run in flight.
func TestReconcileNowRefusesAConcurrentRun(t *testing.T) {
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{}, nil
	}, &memLedger{}, nil)
	s.single.Lock()
	defer s.single.Unlock()
	if err := s.ReconcileNow(context.Background()); !errors.Is(err, ErrReconcileInProgress) {
		t.Fatalf("err = %v, want ErrReconcileInProgress", err)
	}
}

func TestParseEntries(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		max     int
		want    []string
		wantErr string
	}{
		{
			name: "comments, blanks, bare IPs and CIDRs",
			body: "# header\n\n  203.0.113.5  \n198.51.100.0/24\n2001:db8::1\n",
			max:  10,
			want: []string{"198.51.100.0/24", "2001:db8::1/128", "203.0.113.5/32"},
		},
		{
			name: "duplicates collapse",
			body: "203.0.113.5\n203.0.113.5/32\n",
			max:  10,
			want: []string{"203.0.113.5/32"},
		},
		{
			name: "a host bit set in a CIDR is masked to the network",
			body: "198.51.100.7/24\n",
			max:  10,
			want: []string{"198.51.100.0/24"},
		},
		{name: "invalid line", body: "203.0.113.5\nnot-an-ip\n", max: 10, wantErr: "is not an IP or CIDR"},
		// The single most dangerous line a hijacked feed could carry: a source rule
		// without paths would then allow the entire internet.
		{name: "IPv4 default route", body: "203.0.113.5\n0.0.0.0/0\n", max: 10, wantErr: "is the default route"},
		{name: "IPv6 default route", body: "203.0.113.5\n::/0\n", max: 10, wantErr: "is the default route"},
		{name: "over-broad IPv4 prefix", body: "10.0.0.0/7\n", max: 10, wantErr: "broader than the /8 limit"},
		{name: "over-broad IPv6 prefix", body: "2001:db8::/31\n", max: 10, wantErr: "broader than the /32 limit"},
		{name: "RFC1918", body: "10.0.0.0/8\n", max: 10, wantErr: "not a public address range"},
		{name: "RFC1918 /16", body: "192.168.1.0/24\n", max: 10, wantErr: "not a public address range"},
		{name: "loopback", body: "127.0.0.1\n", max: 10, wantErr: "not a public address range"},
		{name: "IPv6 loopback", body: "::1\n", max: 10, wantErr: "not a public address range"},
		{name: "link-local", body: "169.254.169.254\n", max: 10, wantErr: "not a public address range"},
		{name: "ULA", body: "fd00::/48\n", max: 10, wantErr: "not a public address range"},
		{name: "CGNAT", body: "100.64.0.0/10\n", max: 10, wantErr: "not a public address range"},
		{name: "IETF protocol assignments", body: "192.0.0.8\n", max: 10, wantErr: "not a public address range"},
		{name: "benchmarking range", body: "198.18.0.0/15\n", max: 10, wantErr: "not a public address range"},
		{name: "NAT64", body: "64:ff9b::/96\n", max: 10, wantErr: "not a public address range"},
		{name: "multicast", body: "224.0.0.1\n", max: 10, wantErr: "not a public address range"},
		{name: "unspecified address", body: "0.0.0.0\n", max: 10, wantErr: "not a public address range"},
		{name: "empty", body: "\n\n# only comments\n", max: 10, wantErr: "no valid entries"},
		{name: "over the cap", body: "203.0.113.1\n203.0.113.2\n", max: 1, wantErr: "more than maxEntries"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEntries(tt.body, tt.max)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("entries = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("entries = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
