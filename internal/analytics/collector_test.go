package analytics

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func testCollector(t *testing.T, now *time.Time, store *Rollup) *Collector {
	t.Helper()
	c := NewCollector(Options{
		Queue: 64,
		Country: func(ip net.IP) (string, bool) {
			return map[string]string{"203.0.113.9": "NL", "198.51.100.7": "US"}[ip.String()], ip.String() != "10.0.0.1"
		},
		GeoLoaded: func() bool { return true },
		Store:     store,
		Now:       func() time.Time { return *now },
	})
	c.Apply(Settings{Enabled: true, RetentionDays: 7, ClientsMode: "persist"})
	return c
}

func TestCollectorAggregatesAndReports(t *testing.T) {
	now := time.Date(2026, 10, 26, 10, 30, 0, 0, time.UTC)
	c := testCollector(t, &now, nil)
	ev := func(host string, status int, ip string, deny, sig, path, ua string, minsAgo int) {
		c.ingest(Event{Time: now.Add(-time.Duration(minsAgo) * time.Minute), Host: host, Method: "GET", Status: status, BytesOut: 100, DurMs: 40,
			Client: net.ParseIP(ip), Deny: deny, Signature: sig, Path: path, UserAgent: ua})
	}
	for i := 0; i < 10; i++ {
		ev("www", 200, "198.51.100.7", "", "", "/", "Mozilla/5.0", i)
	}
	ev("www", 403, "203.0.113.9", "geo", "", "/wp-login.php", "Mozilla/5.0", 1)
	ev("www", 403, "203.0.113.9", "guard", "guard:xmlrpc", "/xmlrpc.php", "x", 2)
	ev("-", 404, "203.0.113.9", "", "", "/.env", "zgrab/0.x", 3)
	ev("api", 502, "10.0.0.1", "", "", "/v1", "x", 30)

	rep, err := c.Report(Query{Range: "1h"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Totals.Requests != 14 || rep.Totals.Denied != 2 || rep.Totals.Threats != 3 || rep.Totals.Status5xx != 1 {
		t.Fatalf("1h totals = %+v", rep.Totals)
	}
	if len(rep.Series) != 60 || rep.BucketSeconds != 60 {
		t.Fatalf("series %d bucket %d", len(rep.Series), rep.BucketSeconds)
	}
	var sum int64
	for _, b := range rep.Series {
		sum += b.Requests
	}
	if sum != 14 {
		t.Errorf("series sum %d", sum)
	}
	hosts := map[string]HostRow{}
	for _, h := range rep.Hosts {
		hosts[h.Name] = h
	}
	if hosts["www"].Requests != 12 || hosts["-"].Requests != 1 || hosts["-"].Threats != 1 {
		t.Errorf("hosts = %+v", rep.Hosts)
	}
	if len(rep.Denials) != 2 || rep.Denials[0].N != 1 {
		t.Errorf("denials = %+v", rep.Denials)
	}
	sigs := map[string]Sig{}
	for _, s := range rep.Signatures {
		sigs[s.Label] = s
	}
	if sigs["guard:xmlrpc"].N != 1 || sigs["wp-probe"].N != 1 || sigs["scanner-ua"].N != 1 {
		t.Errorf("signatures = %+v", rep.Signatures)
	}
	if sigs["guard:xmlrpc"].Countries[0] != "NL" {
		t.Errorf("sig countries = %+v", sigs["guard:xmlrpc"])
	}
	cc := map[string]Country{}
	for _, x := range rep.Countries {
		cc[x.Code] = x
	}
	if cc["US"].Requests != 10 || cc["NL"].Requests != 3 || cc["NL"].Denied != 2 {
		t.Errorf("countries = %+v", rep.Countries)
	}
	if len(rep.Clients) != 3 || rep.Clients[0].IP != "198.51.100.7" {
		t.Errorf("clients = %+v", rep.Clients)
	}
	rep6, _ := c.Report(Query{Range: "6h", Host: "api"})
	if rep6.Totals.Requests != 1 || rep6.Totals.Status5xx != 1 || len(rep6.Hosts) != 0 || rep6.Countries[0].Code != unknownCountry {
		t.Errorf("api 6h = %+v countries %+v hosts %d", rep6.Totals, rep6.Countries, len(rep6.Hosts))
	}
	if _, err := c.Report(Query{Range: "1h", Host: "nope"}); err == nil {
		t.Error("unknown host must error")
	}
	if _, err := c.Report(Query{Range: "7d"}); err == nil {
		t.Error("7d without a store must be unavailable")
	}
	if got := c.Ranges(); len(got) != 3 {
		t.Errorf("ranges without store = %v", got)
	}
	// A host the live config knows is accepted even with no traffic yet.
	c.opts.KnownHost = func(h string) bool { return h == "fresh" }
	if r, err := c.Report(Query{Range: "1h", Host: "fresh"}); err != nil || r.Totals.Requests != 0 {
		t.Errorf("known host without traffic: %v %+v", err, r.Totals)
	}
}

func TestCollectorStampsAndSlots(t *testing.T) {
	now := time.Date(2026, 10, 26, 10, 30, 0, 0, time.UTC)
	c := testCollector(t, &now, nil)
	c.ingest(Event{Time: now, Host: "www"})
	// A request that finished now but was stamped 25 h ago (impossible after
	// the completion-time change, but the clamp must still hold) counts in the
	// current minute and never resets the live slot.
	c.ingest(Event{Time: now.Add(-25 * time.Hour), Host: "www"})
	c.ingest(Event{Time: now.Add(2 * time.Hour), Host: "www"})
	c.ingest(Event{Host: "www"}) // zero time
	rep, _ := c.Report(Query{Range: "1h"})
	if rep.Totals.Requests != 4 {
		t.Fatalf("clamped events must all count now: %+v", rep.Totals)
	}
	// An event older than the slot the ring position now holds is dropped
	// rather than wiping the slot.
	m := now.Unix() / 60
	c.minutes[m%minuteSlots].minute = m + 1
	c.minutes[m%minuteSlots].hosts = map[aggKey]*hostMinute{{"www", SourceExternal}: {req: 9}}
	c.ingest(Event{Time: now.Add(-30 * time.Minute), Host: "www"}) // different slot: fine
	c.ingest(Event{Time: now, Host: "www"})                        // this slot holds a newer minute: dropped
	if c.minutes[m%minuteSlots].hosts[aggKey{"www", SourceExternal}].req != 9 {
		t.Error("newer slot was clobbered")
	}
}

func TestCollectorDisabledAndQueue(t *testing.T) {
	now := time.Now()
	c := testCollector(t, &now, nil)
	c.Apply(Settings{Enabled: false, RetentionDays: 7, ClientsMode: "persist"})
	for i := 0; i < 100; i++ {
		c.Record(Event{Host: "www"})
	}
	if len(c.queue) != 0 || c.Dropped() != 0 {
		t.Fatal("disabled collector must not queue or count")
	}
	rep, _ := c.Report(Query{Range: "1h"})
	if rep.Enabled {
		t.Fatal("report must say disabled")
	}
	c.Apply(Settings{Enabled: true, RetentionDays: 7, ClientsMode: "off"})
	for i := 0; i < 200; i++ {
		c.Record(Event{Host: "www", Client: net.ParseIP("203.0.113.9")})
	}
	if c.Dropped() != 200-64 {
		t.Fatalf("dropped = %d", c.Dropped())
	}
	for len(c.queue) > 0 {
		c.ingest(<-c.queue)
	}
	rep, _ = c.Report(Query{Range: "1h"})
	if rep.Totals.Requests != 64 || len(rep.Clients) != 0 || rep.Dropped != 136 {
		t.Errorf("after drain: %+v clients %d dropped %d", rep.Totals, len(rep.Clients), rep.Dropped)
	}
}

func TestCollectorTopClientsOffForgets(t *testing.T) {
	dir := t.TempDir()
	store, _ := OpenRollup(filepath.Join(dir, "a.db"))
	defer store.Close()
	now := time.Date(2026, 10, 26, 9, 10, 0, 0, time.UTC)
	c := testCollector(t, &now, store)
	for i := 0; i < 5; i++ {
		c.ingest(Event{Time: now, Host: "www", Client: net.ParseIP("203.0.113.9")})
	}
	// Addresses recorded under "memory" never reach the rollups, even if the
	// mode is persist again by the time the hour is flushed.
	c.Apply(Settings{Enabled: true, RetentionDays: 7, ClientsMode: "memory"})
	c.ingest(Event{Time: now, Host: "www", Client: net.ParseIP("192.0.2.77")})
	c.Apply(Settings{Enabled: true, RetentionDays: 7, ClientsMode: "persist"})
	now = time.Date(2026, 10, 26, 10, 30, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		c.ingest(Event{Time: now, Host: "www", Client: net.ParseIP("198.51.100.7")})
	}
	c.maybeFlush(false) // persists the previous hour, eligible clients included
	rows, _ := store.ReadHours(0, now.Unix()+1, "", "")
	var clientRows int
	for _, r := range rows {
		if r.Dim == "client" {
			clientRows++
		}
	}
	if clientRows != 1 {
		t.Fatalf("client rows persisted = %d (the memory-mode address must not be written)", clientRows)
	}
	if rep, _ := c.Report(Query{Range: "6h"}); len(rep.Clients) != 3 {
		t.Fatalf("clients before = %d", len(rep.Clients))
	}
	c.Apply(Settings{Enabled: true, RetentionDays: 7, ClientsMode: "off"})
	if rep, _ := c.Report(Query{Range: "6h"}); len(rep.Clients) != 0 {
		t.Errorf("clients after off = %+v", rep.Clients)
	}
	rows, _ = store.ReadHours(0, now.Unix()+1, "", "")
	for _, r := range rows {
		if r.Dim == "client" {
			t.Fatal("persisted client rows survived topClients off")
		}
	}
}

func TestCollectorBounds(t *testing.T) {
	now := time.Now()
	c := testCollector(t, &now, nil)
	for i := 0; i < maxClientsPerHost+50; i++ {
		c.ingest(Event{Time: now, Host: "www", Client: net.ParseIP("2001:db8::" + strconv.Itoa(i)), Signature: "crowdsec:s" + strconv.Itoa(i)})
	}
	hs := &c.hours[(now.Unix()/3600)%hourSlots]
	hh := hs.hosts[aggKey{"www", SourceExternal}]
	if len(hh.clients) > maxClientsPerHost {
		t.Errorf("clients %d > cap", len(hh.clients))
	}
	if len(hh.sig) > maxSigLabels {
		t.Errorf("sig labels %d > cap", len(hh.sig))
	}
	if _, ok := hh.sig[otherLabel]; !ok {
		t.Error("overflow labels must fold into other")
	}
	// Countries per label are capped too.
	for i := 0; i < 20; i++ {
		c.opts.Country = func(net.IP) (string, bool) { return "C" + strconv.Itoa(i), true }
		c.ingest(Event{Time: now, Host: "www", Client: net.ParseIP("203.0.113.1"), Signature: "crowdsec:s0"})
	}
	if n := len(hh.sigCountry["crowdsec:s0"]); n > maxSigCountries {
		t.Errorf("sig countries %d > cap", n)
	}
	for i := 0; i < maxHostsTracked+10; i++ {
		c.ingest(Event{Time: now, Host: "h" + strconv.Itoa(i)})
	}
	if len(c.hostSet) > maxHostsTracked {
		t.Errorf("hosts %d > cap", len(c.hostSet))
	}
	// Row budget: a synthetic hour with far more rows than the budget is cut,
	// most important tiers first.
	c.mu.Lock()
	rows := c.rowsForHourLocked(now.Unix() / 3600)
	c.mu.Unlock()
	if len(rows) == 0 || len(rows) > rowBudgetPerHour {
		t.Errorf("rows = %d", len(rows))
	}
}

func TestCollectorRollupFlushReplayAndLongRanges(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenRollup(filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	c := testCollector(t, &now, store)
	c.Apply(Settings{Enabled: true, RetentionDays: 30, ClientsMode: "persist"})
	for i := 0; i < 5; i++ {
		c.ingest(Event{Time: now.Add(time.Duration(i) * time.Minute), Host: "www", Status: 200, DurMs: 30, Client: net.ParseIP("198.51.100.7")})
	}
	now = time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		c.ingest(Event{Time: now.Add(time.Duration(i) * time.Minute), Host: "www", Status: 403, Deny: "geo", Client: net.ParseIP("203.0.113.9"), Path: "/.git/config"})
	}
	now = time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	// The first flush writes the previous hour only (09:00); 08:00 predates
	// what this instance can vouch for.
	c.maybeFlush(false)
	hours, _ := store.Hours()
	if len(hours) != 1 {
		t.Fatalf("hours after first flush = %v", hours)
	}
	rows, _ := store.ReadHours(hours[0], hours[0]+3600, "", "")
	var total RollupRow
	for _, r := range rows {
		if r.Dim == "total" {
			total = r
		}
	}
	if total.N != 7 || total.D != 7 || total.T != 7 {
		t.Fatalf("flushed total = %+v (rows %d)", total, len(rows))
	}
	rep, err := c.Report(Query{Range: "7d"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Totals.Requests != 12 || rep.Totals.Denied != 7 || rep.BucketSeconds != 3*3600 || len(rep.Series) != 56 {
		t.Errorf("7d = %+v bucket %d series %d", rep.Totals, rep.BucketSeconds, len(rep.Series))
	}
	if len(c.Ranges()) != 5 {
		t.Errorf("ranges = %v", c.Ranges())
	}
	// Cached: a second call within the TTL returns the same numbers without a read.
	rep2, _ := c.Report(Query{Range: "7d"})
	if rep2.Totals != rep.Totals {
		t.Error("cached long report differs")
	}
	// Shutdown flush writes the running hour; a fresh collector replays both.
	c.maybeFlush(true)
	later := now
	c2 := testCollector(t, &later, store)
	c2.replay()
	// Only the stored hour (09:00) comes back: 08:00 was never written (it
	// predates the first flush) and the running hour held nothing.
	rep3, _ := c2.Report(Query{Range: "24h"})
	if rep3.Totals.Requests != 7 || rep3.Totals.Denied != 7 {
		t.Errorf("replayed 24h = %+v", rep3.Totals)
	}
	cc := map[string]Country{}
	for _, x := range rep3.Countries {
		cc[x.Code] = x
	}
	if cc["NL"].Denied != 7 {
		t.Errorf("replayed countries = %+v", rep3.Countries)
	}
	// Retention prune happens even when a write fails: plant an old hour and a
	// bad store state is simulated by closing the DB after prune? Simpler: an
	// old hour disappears on the next flush.
	old := (now.Unix()/3600 - 40*24) * 3600
	if err := store.WriteHour(old, []RollupRow{{Hour: old, Host: "www", Dim: "total", N: 1}}); err != nil {
		t.Fatal(err)
	}
	c.maybeFlush(false)
	hours, _ = store.Hours()
	for _, h := range hours {
		if h == old {
			t.Error("old hour survived prune")
		}
	}
}

func TestCollectorFlushSkipsRejectedHour(t *testing.T) {
	dir := t.TempDir()
	store, _ := OpenRollup(filepath.Join(dir, "a.db"))
	defer store.Close()
	now := time.Date(2026, 10, 26, 9, 10, 0, 0, time.UTC)
	c := testCollector(t, &now, store)
	// A host name the store refuses (too long) must not stall later hours.
	long := make([]byte, rollupMaxHostLen+1)
	for i := range long {
		long[i] = 'h'
	}
	c.ingest(Event{Time: now, Host: string(long)})
	c.ingest(Event{Time: now, Host: "www"})
	now = time.Date(2026, 10, 26, 10, 5, 0, 0, time.UTC)
	c.maybeFlush(false)
	hours, _ := store.Hours()
	if len(hours) != 0 {
		t.Fatalf("rejected hour must not be written: %v", hours)
	}
	if c.flushed != now.Unix()/3600-1 {
		t.Fatalf("flushed watermark must advance past the rejected hour: %d", c.flushed)
	}
	now = time.Date(2026, 10, 26, 10, 30, 0, 0, time.UTC)
	c.ingest(Event{Time: now, Host: "www"})
	now = time.Date(2026, 10, 26, 11, 5, 0, 0, time.UTC)
	c.maybeFlush(false)
	hours, _ = store.Hours()
	if len(hours) != 1 {
		t.Fatalf("next hour must be written: %v", hours)
	}
}

func TestCollectorStartClose(t *testing.T) {
	dir := t.TempDir()
	store, _ := OpenRollup(filepath.Join(dir, "a.db"))
	defer store.Close()
	now := time.Now()
	c := testCollector(t, &now, store)
	c.Start()
	for i := 0; i < 50; i++ {
		c.Record(Event{Host: "www", Status: 200, Client: net.ParseIP("198.51.100.7")})
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			c.Report(Query{Range: "1h"})
		}
		close(done)
	}()
	<-done
	c.Close()
	c.Close() // idempotent
	// The shutdown flush wrote the running hour.
	rows, _ := store.ReadHours(0, now.Unix()+3600, "www", "")
	var n int64
	for _, r := range rows {
		if r.Dim == "total" {
			n += r.N
		}
	}
	if n != 50 {
		t.Errorf("running hour not flushed on close: %d", n)
	}
}

func TestPercentile(t *testing.T) {
	var h [durBucketCount]int64
	if percentile(h, 0.95) != 0 {
		t.Error("empty histogram must be 0")
	}
	h[durBucket(40)] = 90
	h[durBucket(400)] = 10
	if p := percentile(h, 0.50); p != 50 {
		t.Errorf("p50 = %d", p)
	}
	if p := percentile(h, 0.95); p != 500 {
		t.Errorf("p95 = %d", p)
	}
}

func TestCollectorCleansPathBeforeClassify(t *testing.T) {
	now := time.Now()
	c := testCollector(t, &now, nil)
	for _, p := range []string{"//.env", "/./wp-login.php", "/a//../.git/config", "/x/../phpmyadmin/"} {
		c.ingest(Event{Time: now, Host: "www", Path: p})
	}
	rep, _ := c.Report(Query{Range: "1h"})
	if rep.Totals.Threats != 4 {
		t.Errorf("uncleaned probe paths must still classify: %+v %+v", rep.Totals, rep.Signatures)
	}
	if cleanEventPath("bogus") != "bogus" || cleanEventPath("/a/b/") != "/a/b/" || cleanEventPath("//x/./y") != "/x/y" {
		t.Error("cleanEventPath")
	}
}

func TestLongRangeServesRunningHourFromMemory(t *testing.T) {
	dir := t.TempDir()
	store, _ := OpenRollup(filepath.Join(dir, "a.db"))
	defer store.Close()
	now := time.Date(2026, 10, 26, 10, 10, 0, 0, time.UTC)
	c := testCollector(t, &now, store)
	for i := 0; i < 10; i++ {
		c.ingest(Event{Time: now, Host: "www"})
	}
	c.maybeFlush(true) // shutdown: running hour written
	c2 := testCollector(t, &now, store)
	c2.replay()
	for i := 0; i < 5; i++ {
		c2.ingest(Event{Time: now.Add(time.Minute), Host: "www"})
	}
	r24, _ := c2.Report(Query{Range: "24h"})
	r7, _ := c2.Report(Query{Range: "7d"})
	if r24.Totals.Requests != 15 || r7.Totals.Requests != 15 {
		t.Errorf("after restart within the hour: 24h=%d 7d=%d, want 15/15", r24.Totals.Requests, r7.Totals.Requests)
	}
}

func TestCollectorSourceFilter(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenRollup(filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 10, 26, 9, 10, 0, 0, time.UTC)
	c := testCollector(t, &now, store)
	c.Apply(Settings{Enabled: true, RetentionDays: 30, ClientsMode: "persist", InternalNetworks: []string{"198.51.100.0/24", "bogus"}})
	ev := func(host, ip string, deny string) {
		c.ingest(Event{Time: now, Host: host, Method: "GET", Status: 200, DurMs: 10, Client: net.ParseIP(ip), Deny: deny, Path: "/", UserAgent: "Mozilla/5.0"})
	}
	for i := 0; i < 6; i++ {
		ev("www", "10.0.0.1", "") // private: internal
	}
	for i := 0; i < 3; i++ {
		ev("www", "198.51.100.7", "") // configured network: internal
	}
	for i := 0; i < 4; i++ {
		ev("www", "203.0.113.9", "geo") // public: external
	}
	ev("api", "::ffff:192.168.1.5", "")                                                                      // mapped v4 private: internal
	ev("api", "2001:db8::1", "")                                                                             // documentation range: external
	c.ingest(Event{Time: now, Host: "api", Method: "GET", Status: 200, Path: "/", UserAgent: "Mozilla/5.0"}) // no client address: external

	all, _ := c.Report(Query{Range: "1h"})
	in, _ := c.Report(Query{Range: "1h", Scope: SourceInternal})
	ex, _ := c.Report(Query{Range: "1h", Scope: SourceExternal})
	if all.Totals.Requests != 16 || in.Totals.Requests != 10 || ex.Totals.Requests != 6 {
		t.Fatalf("requests all/in/ex = %d/%d/%d", all.Totals.Requests, in.Totals.Requests, ex.Totals.Requests)
	}
	if in.Totals.Denied != 0 || ex.Totals.Denied != 4 || in.Scope != SourceInternal || all.Scope != "" {
		t.Errorf("denied in/ex = %d/%d scope %q/%q", in.Totals.Denied, ex.Totals.Denied, in.Scope, all.Scope)
	}
	hosts := func(r Report) map[string]int64 {
		m := map[string]int64{}
		for _, h := range r.Hosts {
			m[h.Name] = h.Requests
		}
		return m
	}
	if h := hosts(in); h["www"] != 9 || h["api"] != 1 {
		t.Errorf("internal hosts = %+v", in.Hosts)
	}
	if h := hosts(ex); h["www"] != 4 || h["api"] != 2 {
		t.Errorf("external hosts = %+v", ex.Hosts)
	}
	// A configured network keeps its country; the private ranges have none.
	cc := map[string]int64{}
	for _, x := range in.Countries {
		cc[x.Code] = x.Requests
	}
	if len(cc) != 2 || cc[unknownCountry] != 7 || cc["US"] != 3 || len(in.Denials) != 0 || in.Totals.Threats != 0 {
		t.Errorf("internal countries/denials = %+v %+v threats %d", in.Countries, in.Denials, in.Totals.Threats)
	}
	for _, cl := range ex.Clients {
		if cl.IP == "10.0.0.1" || cl.IP == "198.51.100.7" {
			t.Errorf("internal address in external clients: %+v", ex.Clients)
		}
	}
	if _, err := c.Report(Query{Range: "1h", Scope: "lan"}); err == nil {
		t.Error("unknown scope must error")
	}
	// Host filter and scope compose.
	hv, _ := c.Report(Query{Range: "6h", Host: "api", Scope: SourceExternal})
	if hv.Totals.Requests != 2 || len(hv.Hosts) != 0 {
		t.Errorf("api external = %+v", hv.Totals)
	}

	// The source survives the rollups: flush, read back filtered, replay, long range.
	now = time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	c.maybeFlush(false)
	rows, _ := store.ReadHours(0, now.Unix(), "", SourceInternal)
	var total int64
	for _, r := range rows {
		if r.Src != SourceInternal {
			t.Fatalf("filtered read returned src %q", r.Src)
		}
		if r.Dim == "total" {
			total += r.N
		}
	}
	if total != 10 {
		t.Errorf("stored internal total = %d (rows %d)", total, len(rows))
	}
	c2 := testCollector(t, &now, store)
	c2.Apply(Settings{Enabled: true, RetentionDays: 30, ClientsMode: "persist"})
	c2.replay()
	if r, _ := c2.Report(Query{Range: "24h", Scope: SourceInternal}); r.Totals.Requests != 10 {
		t.Errorf("replayed internal 24h = %+v", r.Totals)
	}
	if r, _ := c2.Report(Query{Range: "7d", Scope: SourceExternal}); r.Totals.Requests != 6 || r.Totals.Denied != 4 {
		t.Errorf("external 7d = %+v", r.Totals)
	}
	if r, _ := c2.Report(Query{Range: "7d"}); r.Totals.Requests != 16 {
		t.Errorf("all 7d = %+v", r.Totals)
	}
}

func TestIsInternalIP(t *testing.T) {
	extra, bad := ParseNetworks([]string{"198.51.100.0/24", "203.0.113.5", "nope", "2001:db8:1::/48", "::ffff:192.0.2.0/120"})
	if len(extra) != 4 || len(bad) != 1 || bad[0] != "nope" {
		t.Fatalf("ParseNetworks = %v bad %v", extra, bad)
	}
	if extra[3].String() != "192.0.2.0/24" {
		t.Errorf("4-in-6 prefix = %s, want 192.0.2.0/24", extra[3])
	}
	cases := map[string]bool{
		"10.1.2.3": true, "172.16.0.9": true, "192.168.0.1": true, "127.0.0.1": true, "169.254.1.1": true,
		"::1": true, "fd12::1": true, "fe80::1": true, "::ffff:10.0.0.1": true, "0.0.0.0": true,
		"198.51.100.33": true, "203.0.113.5": true, "2001:db8:1::9": true, "192.0.2.77": true, "::ffff:192.0.2.78": true,
		"203.0.113.6": false, "2001:db8:2::1": false, "198.51.99.1": false,
	}
	for ip, want := range cases {
		if got := IsInternalIP(net.ParseIP(ip), extra); got != want {
			t.Errorf("IsInternalIP(%s) = %v, want %v", ip, got, want)
		}
	}
	if IsInternalIP(nil, extra) {
		t.Error("nil address must not be internal")
	}
}
