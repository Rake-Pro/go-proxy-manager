package accesssync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// memLedger is the in-memory Ledger the tests run against: it records how many
// times a save happened, which is how "an unchanged feed costs no commit" is
// asserted.
type memLedger struct {
	mu    sync.Mutex
	l     model.AccessListSourceLedger
	saves int
	// failSaves makes that many next saves fail, as a refused commit would.
	failSaves int
}

func (m *memLedger) Load(context.Context) (model.AccessListSourceLedger, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.l, "rev", nil
}

func (m *memLedger) Save(_ context.Context, l model.AccessListSourceLedger, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failSaves > 0 {
		m.failSaves--
		return errors.New("save refused")
	}
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
		ObjectMeta: model.ObjectMeta{Name: "lan-vpn"},
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
	if got.List != "lan-vpn" || got.Source != "uptimerobot" || got.URL != ts.URL {
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
		{
			// Each /8 passes the per-entry guard; together they exceed the
			// one-/8 budget a maxCoverage with no family set applies.
			name: "over the coverage budget", status: http.StatusOK,
			body: "11.0.0.0/8\n12.0.0.0/8\n",
			src:  model.AccessListSource{MaxCoverage: &model.AccessListSourceCoverage{}}, wantErr: "coverage budget",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := serving(t, tt.status, tt.body)
			prev := []string{"192.0.2.0/24"}
			led := &memLedger{l: model.AccessListSourceLedger{Sources: []model.AccessListSourceEntry{{
				List: "lan-vpn", Source: "uptimerobot", URL: ts.URL,
				FetchedAt: time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339),
				SHA256:    model.AccessListSourceHash("lan-vpn/uptimerobot", ts.URL, prev), Entries: prev,
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
	s.clearAttempt(model.AccessListSourceKey("lan-vpn", "uptimerobot"))
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
		ObjectMeta: model.ObjectMeta{Name: "lan-vpn"},
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
	s.clearAttempt(model.AccessListSourceKey("lan-vpn", "uptimerobot"))
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
		ObjectMeta: model.ObjectMeta{Name: "lan-vpn"},
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
		List: "lan-vpn", Source: "gone", URL: "https://example.com/i.txt",
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		SHA256:    model.AccessListSourceHash("lan-vpn/gone", "https://example.com/i.txt", stale), Entries: stale,
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
		ObjectMeta: model.ObjectMeta{Name: "lan-vpn"},
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

// predicateRanges spells out what isNonPublic's net.IP predicates match, so
// every network in it must start at an address isNonPublic refuses.
func TestPredicateRangesAreNonPublic(t *testing.T) {
	for _, r := range predicateRanges {
		if !isNonPublic(r.IP) {
			t.Errorf("%s is listed as non-public but isNonPublic(%s) is false", r, r.IP)
		}
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
		// An IPv4-mapped prefix matches IPv4 clients, so the IPv4 limits apply.
		{name: "IPv4-mapped prefix broader than the IPv4 limit", body: "::ffff:128.0.0.0/97\n", max: 10, wantErr: "broader than the /8 limit"},
		{name: "IPv4-mapped /2", body: "::ffff:64.0.0.0/98\n", max: 10, wantErr: "broader than the /8 limit"},
		{name: "IPv4-mapped private prefix", body: "::ffff:10.0.0.0/104\n", max: 10, wantErr: "not a public address range"},
		{
			name: "IPv4-mapped prefix is stored as IPv4",
			body: "::ffff:198.51.100.0/120\n",
			max:  10,
			want: []string{"198.51.100.0/24"},
		},
		// A network that starts at a public address but covers non-public space.
		{name: "covers RFC1918", body: "172.0.0.0/8\n", max: 10, wantErr: "not a public address range"},
		{name: "covers CGNAT", body: "100.0.0.0/8\n", max: 10, wantErr: "not a public address range"},
		{name: "covers IETF protocol assignments", body: "192.0.0.0/16\n", max: 10, wantErr: "not a public address range"},
		{
			name: "a public /8 that overlaps nothing is still accepted",
			body: "8.0.0.0/8\n",
			max:  10,
			want: []string{"8.0.0.0/8"},
		},
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

// With maxCoverage set, a feed may cover at most that budget per family, summed
// over its entries, so many individually acceptable prefixes cannot add up to
// most of the internet; a family left at 0 is bounded at one /8 of IPv4 or one
// /24 of IPv6. Without maxCoverage there is no budget.
func TestCheckCoverage(t *testing.T) {
	nets := func(cidrs ...string) []*net.IPNet {
		var out []*net.IPNet
		for _, c := range cidrs {
			_, n, err := net.ParseCIDR(c)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, n)
		}
		return out
	}
	var slash8s []string
	for a := 1; a <= 9; a++ {
		slash8s = append(slash8s, fmt.Sprintf("%d.0.0.0/8", a))
	}
	set := &model.AccessListSourceCoverage{}
	tests := []struct {
		name string
		nets []*net.IPNet
		cov  *model.AccessListSourceCoverage
		ok   bool
	}{
		{"no maxCoverage: many /8s", nets(slash8s...), nil, true},
		{"no maxCoverage: more than a /24 of IPv6", nets("2a00::/24", "2a01::/32"), nil, true},
		{"single addresses", nets("203.0.113.1/32", "198.51.100.0/24", "2001:db8::1/128"), set, true},
		{"exactly one /8 of IPv4", nets("11.0.0.0/9", "12.0.0.0/9"), set, true},
		{"CDN-sized IPv4 and IPv6", nets("11.0.0.0/13", "12.0.0.0/13", "13.0.0.0/14", "14.0.0.0/15",
			"2001:db8::/32", "2001:db9::/32", "2001:dba::/29"), set, true},
		{"many /8s", nets(slash8s...), set, false},
		{"one /8 plus one address", nets("11.0.0.0/8", "203.0.113.1/32"), set, false},
		{"many /8s under a raised budget", nets(slash8s...), &model.AccessListSourceCoverage{IPv4: 4}, true},
		{"more than a /24 of IPv6", nets("2a00::/24", "2a01::/32"), set, false},
		{"IPv6 bounded when only ipv4 is set", nets("2a00::/24", "2a01::/32"), &model.AccessListSourceCoverage{IPv4: 4}, false},
		{"IPv6 under a raised budget", nets("2a00::/24", "2a01::/32"), &model.AccessListSourceCoverage{IPv6: 20}, true},
		{"a lowered budget", nets("203.0.113.0/24", "198.51.100.0/24"), &model.AccessListSourceCoverage{IPv4: 24}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckCoverage(tt.nets, model.AccessListSource{MaxCoverage: tt.cov})
			if (err == nil) != tt.ok {
				t.Fatalf("CheckCoverage = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

// A broad feed on a source without maxCoverage is stored and served exactly as
// before the coverage budget existed, and the fetch logs a warning that names
// maxCoverage. A narrow feed logs nothing.
func TestBroadSourceWithoutMaxCoverageIsAcceptedAndWarned(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		warn       bool
	}{
		{"broad", "11.0.0.0/8\n12.0.0.0/8\n2001:db8::1\n", true},
		{"narrow", "203.0.113.1\n198.51.100.0/24\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			prev := log.Logger
			log.Logger = zerolog.New(&buf)
			t.Cleanup(func() { log.Logger = prev })

			ts := serving(t, http.StatusOK, tt.body)
			led := &memLedger{}
			s := testSyncer(t, ts, model.AccessListSource{Name: "feed"}, led)
			if err := s.Reconcile(context.Background()); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			if st := s.Status(); st.Refused != 0 {
				t.Fatalf("a source without maxCoverage must not be refused for coverage: %+v", st.Sources)
			}
			l, _ := led.state()
			if len(l.Sources) != 1 || len(l.Sources[0].Entries) == 0 {
				t.Fatalf("the fetched set must be stored: %+v", l)
			}
			if got := strings.Contains(buf.String(), "maxCoverage"); got != tt.warn {
				t.Fatalf("broad-feed warning logged = %v, want %v; log: %s", got, tt.warn, buf.String())
			}
		})
	}
}

// A source URL may carry a token in its query; refusal log lines name only the
// host, and a transport failure does not repeat the URL.
func TestRefusedFetchLogOmitsSourceURLQuery(t *testing.T) {
	for _, tt := range []struct {
		name   string
		closed bool
	}{
		{"http error", false},
		{"transport error", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			prev := log.Logger
			log.Logger = zerolog.New(&buf)
			t.Cleanup(func() { log.Logger = prev })

			ts := serving(t, http.StatusInternalServerError, "")
			src := model.AccessListSource{Name: "feed", URL: ts.URL + "/list?token=s3cr3t-value"}
			cfg := model.Config{AccessLists: []model.AccessList{{
				ObjectMeta: model.ObjectMeta{Name: "lan-vpn"},
				Sources:    []model.AccessListSource{src},
				Rules:      []model.IPRule{{Action: model.ActionAllow, Source: src.Name, Paths: []string{"/health"}}},
			}}}
			s := New(func(context.Context) (model.Config, model.Settings, error) {
				return cfg, model.Settings{}, nil
			}, &memLedger{}, nil)
			s.client = ts.Client()
			if tt.closed {
				ts.Close()
			}
			_ = s.Reconcile(context.Background())
			if st := s.Status(); st.Refused != 1 {
				t.Fatalf("refused = %d, want 1", st.Refused)
			}
			if strings.Contains(buf.String(), "s3cr3t-value") || strings.Contains(buf.String(), "/list") {
				t.Errorf("log line carries the source URL path or query: %s", buf.String())
			}
			if !strings.Contains(buf.String(), strings.TrimPrefix(ts.URL, "https://")) {
				t.Errorf("log line does not name the source host: %s", buf.String())
			}
		})
	}
}

// flakyFeed serves body while ok is set and a 502 otherwise.
type flakyFeed struct {
	mu   sync.Mutex
	ok   bool
	body string
}

func (f *flakyFeed) set(ok bool, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ok, f.body = ok, body
}

func (f *flakyFeed) server(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.ok {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })
	return &buf
}

const expiredMsg = "access-list source expired"

// Without maxAge a refused fetch keeps the stored set however old it is: the
// behaviour every existing config relies on.
func TestUnsetMaxAgeKeepsAStaleSetForever(t *testing.T) {
	buf := captureLog(t)
	feed := &flakyFeed{ok: true, body: "203.0.113.1\n"}
	led := &memLedger{}
	s := testSyncer(t, feed.server(t), model.AccessListSource{Name: "feed", Interval: "24h"}, led)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	feed.set(false, "")
	now = now.Add(365 * 24 * time.Hour)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("a year later: %v", err)
	}
	l, saves := led.state()
	if saves != 1 || len(l.Sources) != 1 || len(l.Sources[0].Entries) != 1 || l.Sources[0].ExpiredAt != "" {
		t.Fatalf("saves = %d, ledger = %+v, want the year-old set kept untouched", saves, l)
	}
	if st := s.Status(); st.Sources[0].Expired || st.Sources[0].EntryCount != 1 {
		t.Fatalf("status = %+v, want the kept set reported, not expired", st.Sources[0])
	}
	if l.Sources[0].MaxAgeSince != "" {
		t.Fatalf("maxAgeSince = %q, want nothing written for a source without maxAge", l.Sources[0].MaxAgeSince)
	}
	if strings.Contains(buf.String(), expiredMsg) {
		t.Fatalf("an unset maxAge must never expire a set; log: %s", buf.String())
	}
}

// With maxAge set, a set whose last successful fetch is older is emptied once
// (one save, one reload, one log line), stays expired across further ticks and
// a restart without another commit, and is restored by the next successful
// fetch with no operator action.
func TestMaxAgeExpiresAStaleSetOnceAndRecovers(t *testing.T) {
	buf := captureLog(t)
	feed := &flakyFeed{ok: true, body: "203.0.113.1\n"}
	ts := feed.server(t)
	led := &memLedger{}
	src := model.AccessListSource{Name: "feed", Interval: "24h", MaxAge: "72h"}
	s := testSyncer(t, ts, src, led)
	reloads := 0
	s.onChange = func() { reloads++ }
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := t0
	s.now = func() time.Time { return now }
	tick := func(s *Syncer, at time.Duration) {
		t.Helper()
		now = t0.Add(at)
		if err := s.Reconcile(context.Background()); err != nil {
			t.Fatalf("reconcile at +%s: %v", at, err)
		}
	}

	tick(s, 0)
	feed.set(false, "")
	// Due again after 24h, refused, still young enough to serve.
	tick(s, 48*time.Hour)
	tick(s, 72*time.Hour)
	if l, saves := led.state(); saves != 1 || len(l.Sources[0].Entries) != 1 {
		t.Fatalf("saves = %d, ledger = %+v, want the set kept up to maxAge", saves, l)
	}
	if strings.Contains(buf.String(), expiredMsg) {
		t.Fatalf("expired before maxAge passed; log: %s", buf.String())
	}

	tick(s, 72*time.Hour+15*time.Minute)
	l, saves := led.state()
	e := l.Sources[0]
	if saves != 2 || reloads != 2 {
		t.Fatalf("saves = %d, reloads = %d, want exactly one more of each for the expiry", saves, reloads)
	}
	if len(e.Entries) != 0 || e.ExpiredAt == "" || e.FetchedAt != t0.Format(time.RFC3339) {
		t.Fatalf("entry = %+v, want no entries, expiredAt set, fetchedAt the last success", e)
	}
	ss := s.Status().Sources[0]
	if !ss.Expired || ss.EntryCount != 0 || !ss.FetchedAt.Equal(t0) || ss.LastError == "" {
		t.Fatalf("status = %+v, want expired with the last success and the refusal", ss)
	}

	// Further ticks, and a restarted process on the same ledger, change nothing.
	tick(s, 73*time.Hour)
	tick(s, 80*time.Hour)
	restarted := testSyncer(t, ts, src, led)
	restarted.now = func() time.Time { return now }
	tick(restarted, 90*time.Hour)
	if _, saves := led.state(); saves != 2 {
		t.Fatalf("saves = %d, want the expiry committed once", saves)
	}
	if n := strings.Count(buf.String(), expiredMsg); n != 1 {
		t.Fatalf("expiry logged %d times, want once; log: %s", n, buf.String())
	}
	if ss := restarted.Status().Sources[0]; !ss.Expired {
		t.Fatalf("status after restart = %+v, want still expired", ss)
	}

	// The feed comes back: the set is restored on the next tick.
	feed.set(true, "203.0.113.1\n")
	tick(restarted, 90*time.Hour+15*time.Minute)
	l, saves = led.state()
	e = l.Sources[0]
	if saves != 3 || len(e.Entries) != 1 || e.ExpiredAt != "" {
		t.Fatalf("saves = %d, entry = %+v, want the set restored and expiredAt cleared", saves, e)
	}
	if ss := restarted.Status().Sources[0]; ss.Expired || ss.EntryCount != 1 {
		t.Fatalf("status = %+v, want healthy again", ss)
	}
}

// maxAge counts from the last SUCCESSFUL fetch, which for an unchanged feed is
// later than the last change: fetchedAt is advanced for such a source so the
// age survives a restart, and a refused attempt never counts.
func TestMaxAgeCountsFromTheLastSuccessfulFetch(t *testing.T) {
	feed := &flakyFeed{ok: true, body: "203.0.113.1\n"}
	ts := feed.server(t)
	led := &memLedger{}
	src := model.AccessListSource{Name: "feed", Interval: "24h", MaxAge: "72h"}
	s := testSyncer(t, ts, src, led)
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := t0
	s.now = func() time.Time { return now }

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Unchanged success two days later: fetchedAt moves to it.
	now = t0.Add(48 * time.Hour)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	l, saves := led.state()
	if saves != 2 || l.Sources[0].FetchedAt != now.Format(time.RFC3339) {
		t.Fatalf("saves = %d, fetchedAt = %s, want the unchanged success recorded", saves, l.Sources[0].FetchedAt)
	}

	// Then the feed fails. A process restarted 70h after the last success (and
	// 118h after the set last changed) still serves it; past 72h it does not.
	feed.set(false, "")
	restarted := testSyncer(t, ts, src, led)
	restarted.now = func() time.Time { return now }
	for _, tc := range []struct {
		at      time.Duration
		expired bool
	}{
		{70 * time.Hour, false},
		{73 * time.Hour, true},
	} {
		now = t0.Add(48*time.Hour + tc.at)
		if err := restarted.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if ss := restarted.Status().Sources[0]; ss.Expired != tc.expired {
			t.Fatalf("+%s after the last success: status = %+v, want expired=%v", tc.at, ss, tc.expired)
		}
	}
}

// A source with maxAge that has never fetched is reported as never fetched,
// not expired: there is no ledger entry and nothing is committed.
func TestNeverFetchedSourceIsNotReportedExpired(t *testing.T) {
	buf := captureLog(t)
	feed := &flakyFeed{}
	led := &memLedger{}
	s := testSyncer(t, feed.server(t), model.AccessListSource{Name: "feed", Interval: "24h", MaxAge: "72h"}, led)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		if err := s.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		now = now.Add(100 * time.Hour)
	}
	if _, saves := led.state(); saves != 0 {
		t.Fatalf("saves = %d, want none", saves)
	}
	ss := s.Status().Sources[0]
	if ss.Expired || !ss.FetchedAt.IsZero() || ss.LastError == "" {
		t.Fatalf("status = %+v, want never fetched (zero fetchedAt), refused, not expired", ss)
	}
	if strings.Contains(buf.String(), expiredMsg) {
		t.Fatalf("a never-fetched source must not log an expiry; log: %s", buf.String())
	}
}

// srcSyncer wires a syncer whose one source is read through src on every
// reconcile, so a test can add or remove maxAge between runs, and which
// counts data-plane reloads.
func srcSyncer(t *testing.T, ts *httptest.Server, src *model.AccessListSource, led *memLedger, now *time.Time, reloads *int) *Syncer {
	t.Helper()
	src.URL = ts.URL
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{AccessLists: []model.AccessList{{
			ObjectMeta: model.ObjectMeta{Name: "lan-vpn"},
			Sources:    []model.AccessListSource{*src},
		}}}, model.Settings{}, nil
	}, led, func() { *reloads++ })
	s.client = ts.Client()
	s.now = func() time.Time { return *now }
	return s
}

// A dead feed expires about maxAge after its last successful fetch however
// often the process restarts: everything the decision uses is in the ledger.
func TestMaxAgeDeadFeedExpiresDespiteRestarts(t *testing.T) {
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	const tick = 15 * time.Minute
	for _, tc := range []struct {
		name    string
		restart func(at time.Duration) bool
	}{
		{"restart every 20h", func(at time.Duration) bool { return at > 0 && at%(20*time.Hour) == 0 }},
		{"single restart at +71h", func(at time.Duration) bool { return at == 71*time.Hour }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			feed := &flakyFeed{ok: true, body: "203.0.113.1\n"}
			ts := feed.server(t)
			led := &memLedger{}
			src := model.AccessListSource{Name: "feed", Interval: "24h", MaxAge: "72h"}
			now, reloads := t0, 0
			s := srcSyncer(t, ts, &src, led, &now, &reloads)
			var expiredAt time.Duration
			for at := time.Duration(0); at <= 30*24*time.Hour; at += tick {
				now = t0.Add(at)
				if tc.restart(at) {
					s = srcSyncer(t, ts, &src, led, &now, &reloads)
				}
				if err := s.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
				if at == 0 {
					feed.set(false, "")
				}
				if s.Status().Sources[0].Expired {
					expiredAt = at
					break
				}
			}
			if expiredAt <= 72*time.Hour || expiredAt > 72*time.Hour+tick {
				t.Fatalf("expired after %s, want within one poll tick after 72h", expiredAt)
			}
		})
	}
}

// Under maxAge a fetch whose set CHANGED but whose commit failed is not
// credited to the old set the ledger still holds: that set is never
// relabelled as freshly fetched, and it ages from its own fetch.
func TestMaxAgeChangedSetFailedCommitDoesNotRelabelTheOldSet(t *testing.T) {
	feed := &flakyFeed{ok: true, body: "203.0.113.1\n"}
	ts := feed.server(t)
	led := &memLedger{}
	src := model.AccessListSource{Name: "feed", Interval: "24h", MaxAge: "72h"}
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now, reloads := t0, 0
	s := srcSyncer(t, ts, &src, led, &now, &reloads)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	feed.set(true, "203.0.113.2\n")
	led.mu.Lock()
	led.failSaves = 1
	led.mu.Unlock()
	now = t0.Add(24 * time.Hour)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatal("want the refused save reported")
	}
	feed.set(false, "")
	now = t0.Add(24*time.Hour + 15*time.Minute)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	l, saves := led.state()
	if saves != 1 || l.Sources[0].FetchedAt != t0.Format(time.RFC3339) || l.Sources[0].Entries[0] != "203.0.113.1/32" {
		t.Fatalf("saves = %d, entry = %+v, want the old set untouched with its own fetchedAt", saves, l.Sources[0])
	}
	now = t0.Add(72*time.Hour + time.Minute)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s.Status().Sources[0].Expired {
		t.Fatal("want the old set expired 72h after its own fetch")
	}
}

// A source that ran without maxAge has a fetchedAt that was not kept current
// (ten unchanged days here). Opting in records maxAgeSince (one commit, no
// reload), and the set expires maxAge after that, not at once, with or
// without a restart in between.
func TestMaxAgeOptInExpiresMaxAgeAfterOptIn(t *testing.T) {
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	optIn := t0.Add(10*24*time.Hour + 2*time.Hour)
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprintf("restart=%v", restart), func(t *testing.T) {
			feed := &flakyFeed{ok: true, body: "203.0.113.1\n"}
			ts := feed.server(t)
			led := &memLedger{}
			src := model.AccessListSource{Name: "feed", Interval: "24h"}
			now, reloads := t0, 0
			s := srcSyncer(t, ts, &src, led, &now, &reloads)
			for d := 0; d <= 10; d++ {
				now = t0.Add(time.Duration(d) * 24 * time.Hour)
				if err := s.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if l, saves := led.state(); saves != 1 || l.Sources[0].FetchedAt != t0.Format(time.RFC3339) {
				t.Fatalf("saves = %d, fetchedAt = %s, want the opted-out path to leave fetchedAt at the first fetch", saves, l.Sources[0].FetchedAt)
			}

			src.MaxAge = "72h"
			feed.set(false, "")
			now = optIn
			if err := s.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			l, saves := led.state()
			if saves != 2 || reloads != 1 || l.Sources[0].MaxAgeSince != optIn.Format(time.RFC3339) {
				t.Fatalf("saves = %d, reloads = %d, maxAgeSince = %q, want it recorded once without a reload", saves, reloads, l.Sources[0].MaxAgeSince)
			}
			if restart {
				s = srcSyncer(t, ts, &src, led, &now, &reloads)
			}
			for _, tc := range []struct {
				at      time.Duration
				expired bool
			}{
				{24 * time.Hour, false},
				{71 * time.Hour, false},
				{72*time.Hour + time.Minute, true},
			} {
				now = optIn.Add(tc.at)
				if err := s.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
				if got := s.Status().Sources[0].Expired; got != tc.expired {
					t.Fatalf("+%s after the opt-in: expired = %v, want %v", tc.at, got, tc.expired)
				}
			}
			if _, saves := led.state(); saves != 3 || reloads != 2 {
				t.Fatalf("saves = %d, reloads = %d, want one more of each for the expiry", saves, reloads)
			}
		})
	}
}

// Removing maxAge removes the marker once, without a reload; adding it back
// later starts a fresh baseline rather than reusing the old one.
func TestMaxAgeOptOutRemovesTheMarkerAndReAddStartsAfresh(t *testing.T) {
	feed := &flakyFeed{ok: true, body: "203.0.113.1\n"}
	ts := feed.server(t)
	led := &memLedger{}
	src := model.AccessListSource{Name: "feed", Interval: "24h", MaxAge: "72h"}
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now, reloads := t0, 0
	s := srcSyncer(t, ts, &src, led, &now, &reloads)
	reconcile := func(at time.Duration) {
		t.Helper()
		now = t0.Add(at)
		if err := s.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	reconcile(0)
	if l, _ := led.state(); l.Sources[0].MaxAgeSince != t0.Format(time.RFC3339) {
		t.Fatalf("maxAgeSince = %q, want it set by the first fetch", l.Sources[0].MaxAgeSince)
	}

	src.MaxAge = ""
	reconcile(time.Hour)
	reconcile(2 * time.Hour)
	l, saves := led.state()
	if saves != 2 || reloads != 1 || l.Sources[0].MaxAgeSince != "" {
		t.Fatalf("saves = %d, reloads = %d, maxAgeSince = %q, want it removed once without a reload", saves, reloads, l.Sources[0].MaxAgeSince)
	}

	// Ten opted-out days: unchanged fetches commit nothing.
	for d := 1; d <= 10; d++ {
		reconcile(time.Duration(d) * 24 * time.Hour)
	}
	src.MaxAge = "72h"
	readd := 10*24*time.Hour + time.Hour
	feed.set(false, "")
	reconcile(readd)
	if l, _ := led.state(); l.Sources[0].MaxAgeSince != t0.Add(readd).Format(time.RFC3339) {
		t.Fatalf("maxAgeSince = %q, want a fresh baseline at the re-add", l.Sources[0].MaxAgeSince)
	}
	reconcile(readd + 71*time.Hour)
	if s.Status().Sources[0].Expired {
		t.Fatal("expired before maxAge after the re-add")
	}
	reconcile(readd + 72*time.Hour + time.Minute)
	if !s.Status().Sources[0].Expired {
		t.Fatal("want expired maxAge after the re-add")
	}
}

// An unchanged fetch whose fetchedAt commit failed leaves the ledger one
// interval behind; the next tick persists the in-memory success (once), so a
// restart does not measure the age from the older value.
func TestMaxAgeFailedAdvanceIsPersistedOnTheNextTick(t *testing.T) {
	feed := &flakyFeed{ok: true, body: "203.0.113.1\n"}
	ts := feed.server(t)
	led := &memLedger{}
	s := testSyncer(t, ts, model.AccessListSource{Name: "feed", Interval: "24h", MaxAge: "72h"}, led)
	reloads := 0
	s.onChange = func() { reloads++ }
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := t0
	s.now = func() time.Time { return now }

	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	led.mu.Lock()
	led.failSaves = 1
	led.mu.Unlock()
	now = t0.Add(24 * time.Hour)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatal("want the refused save reported")
	}
	if l, _ := led.state(); l.Sources[0].FetchedAt != t0.Format(time.RFC3339) {
		t.Fatalf("fetchedAt = %s, want the ledger still one interval behind", l.Sources[0].FetchedAt)
	}
	for i := 1; i <= 2; i++ {
		now = t0.Add(24*time.Hour + time.Duration(i)*15*time.Minute)
		if err := s.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	l, saves := led.state()
	if saves != 2 || reloads != 1 || l.Sources[0].FetchedAt != t0.Add(24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("saves = %d, reloads = %d, fetchedAt = %s, want the success persisted once, without a reload", saves, reloads, l.Sources[0].FetchedAt)
	}
}

// For a source with maxAge an unchanged fetch commits the advanced fetchedAt
// (that is what lets the age survive a restart) but does not reload the data
// plane, since the served set is identical.
func TestMaxAgeUnchangedFetchCommitsWithoutReload(t *testing.T) {
	ts := serving(t, http.StatusOK, "203.0.113.1\n")
	led := &memLedger{}
	s := testSyncer(t, ts, model.AccessListSource{Name: "feed", Interval: "24h", MaxAge: "72h"}, led)
	reloads := 0
	s.onChange = func() { reloads++ }
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := t0
	s.now = func() time.Time { return now }
	for _, at := range []time.Duration{0, 24 * time.Hour} {
		now = t0.Add(at)
		if err := s.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	l, saves := led.state()
	if saves != 2 || l.Sources[0].FetchedAt != now.Format(time.RFC3339) {
		t.Fatalf("saves = %d, fetchedAt = %s, want the unchanged fetch committed", saves, l.Sources[0].FetchedAt)
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want only the first fetch to reload", reloads)
	}
}

// With a clock that is not on whole seconds, an unchanged feed under maxAge
// still commits once per interval, not once per poll: the in-memory success is
// compared with the ledger at the ledger's one-second resolution.
func TestMaxAgeCommitsOncePerIntervalWithASubSecondClock(t *testing.T) {
	ts := serving(t, http.StatusOK, "203.0.113.1\n")
	led := &memLedger{}
	s := testSyncer(t, ts, model.AccessListSource{Name: "feed", Interval: "1h", MaxAge: "1h"}, led)
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 123456789, time.UTC)
	now := t0
	s.now = func() time.Time { return now }
	polls := 0
	for at := time.Duration(0); at <= 10*time.Hour; at += 15*time.Minute + 7*time.Millisecond {
		now = t0.Add(at)
		polls++
		if err := s.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if s.Status().Sources[0].Expired {
			t.Fatalf("expired at +%s with a healthy feed", at)
		}
	}
	// One save for the first fetch and at most one per elapsed interval.
	if _, saves := led.state(); saves > 11 {
		t.Fatalf("saves = %d over %d polls and 10 intervals, want at most 11", saves, polls)
	}
}
