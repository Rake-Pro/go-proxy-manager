package analytics

import (
	"errors"
	"net"
	"net/netip"
	"path"
	"slices"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// Event is one finished data-plane request as the collector sees it. The
// request path builds it and hands it over with a single non-blocking queue
// send; everything that costs anything (country lookup, signature
// classification, map updates) happens on the collector's worker.
//
// Path, RawQuery and UserAgent are consumed by the classifier only and are
// never stored or aggregated: every aggregate key is bounded (host name,
// status class, country, denial reason, signature label), and the only
// per-client data is the bounded top-clients list.
type Event struct {
	Time      time.Time // completion time
	Host      string    // proxy host NAME or NoHost
	Method    string
	Status    int
	BytesOut  int64
	DurMs     int64
	Client    net.IP
	Deny      string // denial reason, "" when the request was served
	Signature string // label noted by a tier (guard:<name>, crowdsec:<scenario>), "" otherwise
	Path      string
	RawQuery  string
	UserAgent string
}

// ErrRangeUnavailable is returned for a range the instance cannot serve: the
// 7d and 30d ranges need the on-disk rollups and a retention that covers them.
var ErrRangeUnavailable = errors.New("range not available on this instance")

// Options configure a Collector.
type Options struct {
	// Queue is the event queue depth (default 8192). A full queue drops the
	// event and counts it; the request is never delayed.
	Queue int
	// Country resolves a client address to an ISO country code; nil or a miss
	// records "--".
	Country func(net.IP) (string, bool)
	// GeoLoaded reports whether a GeoIP database is loaded, for the report flag.
	GeoLoaded func() bool
	// KnownHost reports whether a proxy host name exists in the live config,
	// for the host filter; nil falls back to "seen by the collector".
	KnownHost func(string) bool
	// Store persists closed hours; nil keeps the 24 h memory window only.
	Store *Rollup
	// Now is the clock (tests); nil means time.Now.
	Now func() time.Time
}

// Settings are the operator settings the collector follows (the analytics
// block of settings.yaml); Apply installs them.
type Settings struct {
	Enabled       bool
	RetentionDays int    // 0 selects the 7 day default
	ClientsMode   string // "persist" (default when empty), "memory" or "off"
	// InternalNetworks are extra CIDRs (or single addresses) counted as
	// internal traffic besides the private, loopback and link-local ranges.
	InternalNetworks []string
}

// Network sources: every aggregate is kept per source so a report can show
// the traffic the internet sent, the traffic the LAN sent, or both.
const (
	SourceExternal = "external"
	SourceInternal = "internal"
)

// Scopes lists the Query.Scope values that narrow a report to one source.
func Scopes() []string { return []string{SourceExternal, SourceInternal} }

// ValidScope reports whether s is "" (every source) or one of Scopes().
func ValidScope(s string) bool { return s == "" || s == SourceExternal || s == SourceInternal }

// IsInternalIP reports whether ip is a private (RFC 1918, ULA), loopback,
// link-local or unspecified address, or falls inside one of extra. An IPv4
// address mapped into IPv6 is unmapped first. nil is not internal.
func IsInternalIP(ip net.IP, extra []netip.Prefix) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range extra {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ParseNetworks parses CIDRs into prefixes; a bare address is a single-host
// network and an IPv4 prefix written in IPv4-mapped IPv6 form is converted to
// IPv4 (IsInternalIP compares unmapped addresses). Entries that do not parse
// are dropped (settings validation rejects them before they get here) and
// reported in the second result.
func ParseNetworks(list []string) ([]netip.Prefix, []string) {
	var out []netip.Prefix
	var bad []string
	for _, s := range list {
		if p, err := netip.ParsePrefix(s); err == nil {
			if p.Addr().Is4In6() && p.Bits() >= 96 {
				p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
			}
			out = append(out, p.Masked())
			continue
		}
		if a, err := netip.ParseAddr(s); err == nil {
			out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
			continue
		}
		bad = append(bad, s)
	}
	return out, bad
}

// Bounds on the in-memory aggregates. Every one of them is a cap on a key set
// a remote client can influence, so the collector's memory stays fixed no
// matter what traffic looks like.
const (
	minuteSlots        = 24 * 60
	hourSlots          = 25 // 24 whole hours plus the running one
	maxSigLabels       = 64 // per host-hour, "other" included
	maxSigCountries    = 8  // per label per host-hour, "other" included
	maxClientsPerHost  = 512
	persistedClients   = 50 // per host-hour written to the rollups
	maxHostsTracked    = 512
	rowBudgetPerHour   = 200000 // rollup rows the collector writes for one hour at most
	otherLabel         = "other"
	unknownCountry     = "--"
	defaultQueueDepth  = 8192
	durBucketCount     = 12 // durBounds plus overflow
	dropLogEvery       = time.Minute
	flushCheckInterval = 30 * time.Second
	staleEventAfter    = time.Hour // older events are counted in the current minute
	longReportCacheTTL = 30 * time.Second
	closeTimeout       = 5 * time.Second
)

// durBounds are the latency histogram upper bounds in milliseconds, the same
// ladder the Prometheus histogram uses, so a p95 estimate here matches one
// derived from /metrics.
var durBounds = [durBucketCount - 1]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

type hostMinute struct {
	req, den, thr, bytes   int64
	s2, s3, s4, s5, sOther int64
	dur                    [durBucketCount]int64
}

// aggKey is what every in-memory aggregate is kept under: a proxy host name
// and the request's network source.
type aggKey struct {
	host, src string
}

type minuteSlot struct {
	minute int64 // unix minutes
	hosts  map[aggKey]*hostMinute
}

type dimCounts struct{ n, d, t int64 }

type clientAgg struct {
	n, d, t  int64
	country  string
	lastSeen int64 // unix seconds
	persist  bool  // recorded while topClients was "persist": eligible for the rollups
}

type hostHour struct {
	country    map[string]*dimCounts
	deny       map[string]int64
	sig        map[string]*dimCounts       // n hits
	sigCountry map[string]map[string]int64 // label -> cc -> hits
	clients    map[string]*clientAgg
}

func newHostHour() *hostHour {
	return &hostHour{country: map[string]*dimCounts{}, deny: map[string]int64{}, sig: map[string]*dimCounts{}, sigCountry: map[string]map[string]int64{}, clients: map[string]*clientAgg{}}
}

type hourSlot struct {
	hour  int64 // unix hours
	hosts map[aggKey]*hostHour
}

// Collector aggregates Events into bounded in-memory tables and serves
// Reports from them (and from the Rollup store for the long ranges).
type Collector struct {
	opts        Options
	queue       chan Event
	enabled     atomic.Bool
	clients     atomic.Value // string mode
	retDays     atomic.Int32
	internal    atomic.Pointer[[]netip.Prefix] // configured internal networks
	dropped     atomic.Int64
	lastDropLog atomic.Int64
	lastErrLog  atomic.Int64

	mu      sync.Mutex
	minutes [minuteSlots]minuteSlot
	hours   [hourSlots]hourSlot
	flushed int64 // last hour (unix hours) written to the store
	hostSet map[string]struct{}

	cacheMu sync.Mutex
	cache   map[string]cachedReport

	done    chan struct{}
	stopped chan struct{}
	once    sync.Once
	closed  sync.Once
}

type cachedReport struct {
	at  time.Time
	rep Report
}

// NewCollector builds a collector. It starts disabled; Apply turns it on.
func NewCollector(o Options) *Collector {
	if o.Queue <= 0 {
		o.Queue = defaultQueueDepth
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	c := &Collector{opts: o, queue: make(chan Event, o.Queue), hostSet: map[string]struct{}{}, cache: map[string]cachedReport{},
		done: make(chan struct{}), stopped: make(chan struct{})}
	c.clients.Store("persist")
	c.retDays.Store(7)
	return c
}

// Apply installs the settings: the on/off switch, the rollup retention, the
// top-clients mode and the internal networks. Safe to call on every config
// reload. Switching the clients mode to "off" forgets every address already
// held, in memory and in the rollups. A changed internal-networks list applies
// to requests from then on; what was already aggregated keeps its source.
func (c *Collector) Apply(s Settings) {
	enabled, retentionDays, clientsMode := s.Enabled, s.RetentionDays, s.ClientsMode
	if retentionDays <= 0 {
		retentionDays = 7
	}
	if clientsMode == "" {
		clientsMode = "persist"
	}
	nets, bad := ParseNetworks(s.InternalNetworks)
	if len(bad) > 0 {
		log.Warn().Strs("ignored", bad).Msg("analytics: internalNetworks entries that do not parse were ignored")
	}
	prevNets := c.internal.Swap(&nets)
	netsChanged := prevNets == nil || !slices.Equal(*prevNets, nets)
	was := c.enabled.Swap(enabled)
	prevRet := c.retDays.Swap(int32(retentionDays))
	prevMode, _ := c.clients.Swap(clientsMode).(string)
	if was != enabled || prevMode != clientsMode || prevRet != int32(retentionDays) || netsChanged {
		log.Info().Bool("enabled", enabled).Int("retentionDays", retentionDays).Str("topClients", clientsMode).Int("internalNetworks", len(nets)).Msg("analytics collector")
	}
	if clientsMode == "off" && prevMode != "off" {
		c.forgetClients()
	}
	c.invalidateCache()
}

func (c *Collector) forgetClients() {
	c.mu.Lock()
	for i := range c.hours {
		for _, hh := range c.hours[i].hosts {
			hh.clients = map[string]*clientAgg{}
		}
	}
	c.mu.Unlock()
	if c.opts.Store != nil {
		if n, err := c.opts.Store.DeleteDim("client"); err != nil {
			log.Error().Err(err).Msg("analytics: could not delete persisted client rows")
		} else if n > 0 {
			log.Info().Int64("rows", n).Msg("analytics: persisted client rows deleted (topClients off)")
		}
	}
}

// source classifies a client address for the aggregates.
func (c *Collector) source(ip net.IP) string {
	var extra []netip.Prefix
	if p := c.internal.Load(); p != nil {
		extra = *p
	}
	if IsInternalIP(ip, extra) {
		return SourceInternal
	}
	return SourceExternal
}

// Active reports whether events are being collected (the data plane uses it
// to decide whether the observed chain is needed).
func (c *Collector) Active() bool { return c.enabled.Load() }

// Dropped is how many events were lost to a full queue since start.
func (c *Collector) Dropped() int64 { return c.dropped.Load() }

// Record hands an event to the worker. It never blocks: a full queue drops
// the event, counts it, and logs at most once a minute.
func (c *Collector) Record(ev Event) {
	if !c.enabled.Load() {
		return
	}
	select {
	case c.queue <- ev:
	default:
		n := c.dropped.Add(1)
		c.logLimited(&c.lastDropLog, func() {
			log.Warn().Int64("dropped", n).Msg("analytics: event queue full, dropping events")
		})
	}
}

// logLimited runs emit at most once per dropLogEvery per slot.
func (c *Collector) logLimited(slot *atomic.Int64, emit func()) {
	now := c.opts.Now().Unix()
	last := slot.Load()
	if now-last >= int64(dropLogEvery/time.Second) && slot.CompareAndSwap(last, now) {
		emit()
	}
}

// Start runs the worker until Close. Call once.
func (c *Collector) Start() {
	c.once.Do(func() {
		if c.opts.Store != nil {
			c.replay()
		}
		go c.run()
	})
}

// Close stops the worker, drains the queue and flushes the running hour to
// the store, waiting up to closeTimeout so a restart loses at most what was
// still in flight.
func (c *Collector) Close() {
	c.closed.Do(func() {
		close(c.done)
		select {
		case <-c.stopped:
		case <-time.After(closeTimeout):
			log.Warn().Msg("analytics: shutdown flush did not finish in time")
		}
	})
}

func (c *Collector) run() {
	defer close(c.stopped)
	tick := time.NewTicker(flushCheckInterval)
	defer tick.Stop()
	for {
		select {
		case ev := <-c.queue:
			c.ingest(ev)
		case <-tick.C:
			c.maybeFlush(false)
		case <-c.done:
			for {
				select {
				case ev := <-c.queue:
					c.ingest(ev)
				default:
					c.maybeFlush(true)
					return
				}
			}
		}
	}
}

func statusClass(status int) int {
	switch {
	case status >= 200 && status < 300:
		return 2
	case status >= 300 && status < 400:
		return 3
	case status >= 400 && status < 500:
		return 4
	case status >= 500 && status < 600:
		return 5
	}
	return 0
}

func durBucket(ms int64) int {
	for i, b := range durBounds {
		if ms <= b {
			return i
		}
	}
	return durBucketCount - 1
}

func (c *Collector) ingest(ev Event) {
	now := c.opts.Now()
	// Stamp sanity: an event older than an hour (a connection that lived that
	// long) or from the future counts in the current minute, never in a slot
	// the window has rolled past or not reached.
	if ev.Time.IsZero() || now.Sub(ev.Time) > staleEventAfter || ev.Time.After(now.Add(time.Minute)) {
		ev.Time = now
	}
	host := ev.Host
	if host == "" {
		host = NoHost
	}
	country := unknownCountry
	if ev.Client != nil && c.opts.Country != nil {
		if cc, ok := c.opts.Country(ev.Client); ok && cc != "" {
			country = cc
		}
	}
	sig := ev.Signature
	if sig == "" {
		// The event carries the path as the client sent it (before any rewrite);
		// the catalogue is written against the router's canonical form, so clean
		// it the same way here ("//.env" and "/./wp-login.php" must still match).
		sig = Classify(ev.Method, cleanEventPath(ev.Path), ev.RawQuery, ev.UserAgent)
	}
	threat := sig != ""
	denied := ev.Deny != ""
	mode, _ := c.clients.Load().(string)
	src := c.source(ev.Client)

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.hostSet[host]; !ok {
		if len(c.hostSet) >= maxHostsTracked {
			host = NoHost
		} else {
			c.hostSet[host] = struct{}{}
		}
	}
	key := aggKey{host, src}
	// Minute counters. A slot holding a newer minute is never reset by an
	// older event (the stamp clamp above makes that rare; this makes it safe).
	min := ev.Time.Unix() / 60
	ms := &c.minutes[min%minuteSlots]
	if ms.minute != min {
		if ms.minute > min {
			return
		}
		ms.minute = min
		ms.hosts = map[aggKey]*hostMinute{}
	}
	hm := ms.hosts[key]
	if hm == nil {
		hm = &hostMinute{}
		ms.hosts[key] = hm
	}
	hm.req++
	hm.bytes += ev.BytesOut
	if denied {
		hm.den++
	}
	if threat {
		hm.thr++
	}
	switch statusClass(ev.Status) {
	case 2:
		hm.s2++
	case 3:
		hm.s3++
	case 4:
		hm.s4++
	case 5:
		hm.s5++
	default:
		hm.sOther++
	}
	hm.dur[durBucket(ev.DurMs)]++

	// Hourly dimensions.
	hr := ev.Time.Unix() / 3600
	hs := &c.hours[hr%hourSlots]
	if hs.hour != hr {
		if hs.hour > hr {
			return
		}
		hs.hour = hr
		hs.hosts = map[aggKey]*hostHour{}
	}
	hh := hs.hosts[key]
	if hh == nil {
		hh = newHostHour()
		hs.hosts[key] = hh
	}
	cc := hh.country[country]
	if cc == nil {
		cc = &dimCounts{}
		hh.country[country] = cc
	}
	cc.n++
	if denied {
		cc.d++
		hh.deny[ev.Deny]++
	}
	if threat {
		cc.t++
		label := sig
		if _, ok := hh.sig[label]; !ok && len(hh.sig) >= maxSigLabels-1 && label != otherLabel {
			label = otherLabel
		}
		sc := hh.sig[label]
		if sc == nil {
			sc = &dimCounts{}
			hh.sig[label] = sc
			hh.sigCountry[label] = map[string]int64{}
		}
		sc.n++
		scc := hh.sigCountry[label]
		ck := country
		if _, ok := scc[ck]; !ok && len(scc) >= maxSigCountries-1 && ck != otherLabel {
			ck = otherLabel
		}
		scc[ck]++
	}
	if mode != "off" && ev.Client != nil {
		key := ev.Client.String()
		ca := hh.clients[key]
		if ca == nil {
			// Once the per-host cap is reached, further new addresses are not
			// tracked for this hour: a flood of rotating sources must not cost a
			// scan per event, and an address that first appears that late is by
			// definition not a top client of the hour.
			if len(hh.clients) >= maxClientsPerHost {
				return
			}
			ca = &clientAgg{country: country, persist: mode == "persist"}
			hh.clients[key] = ca
		}
		// The mode at record time decides whether the address may ever reach
		// disk; a later switch to persist does not retroactively qualify it.
		ca.persist = ca.persist && mode == "persist"
		ca.n++
		if denied {
			ca.d++
		}
		if threat {
			ca.t++
		}
		ca.lastSeen = ev.Time.Unix()
	}
}

// --- rollups -------------------------------------------------------------

// maybeFlush prunes beyond retention, then writes every completed hour since
// the last flush to the store. force also writes the running hour (shutdown).
// A write the store rejects is logged (rate-limited) and that hour skipped;
// one bad hour never stalls persistence or pruning.
func (c *Collector) maybeFlush(force bool) {
	if c.opts.Store == nil {
		return
	}
	nowHour := c.opts.Now().Unix() / 3600
	// Retention is enforced only while the collector is on: switching it off
	// freezes the history rather than letting a lowered or defaulted retention
	// delete it. Prune runs before the writes so a rejected hour never blocks it.
	if c.enabled.Load() {
		ret := int64(c.retDays.Load())
		if n, err := c.opts.Store.Prune((nowHour - ret*24) * 3600); err != nil {
			c.logLimited(&c.lastErrLog, func() { log.Error().Err(err).Msg("analytics: rollup prune failed") })
		} else if n > 0 {
			log.Info().Int64("rows", n).Int64("retentionDays", ret).Msg("analytics: rollups beyond retention pruned")
		}
	}

	c.mu.Lock()
	first := c.flushed + 1
	if c.flushed == 0 {
		// First flush after a start without replay: only the previous hour,
		// earlier ones were never seen.
		first = nowHour - 1
	}
	var hours []int64
	for h := first; h < nowHour; h++ {
		hours = append(hours, h)
	}
	if force {
		hours = append(hours, nowHour)
	}
	batches := make(map[int64][]RollupRow, len(hours))
	for _, h := range hours {
		if rows := c.rowsForHourLocked(h); rows != nil {
			batches[h] = rows
		}
	}
	c.mu.Unlock()

	for _, h := range hours {
		rows, ok := batches[h]
		if !ok {
			continue
		}
		if err := c.opts.Store.WriteHour(h*3600, rows); err != nil {
			c.logLimited(&c.lastErrLog, func() {
				log.Error().Err(err).Int64("hour", h*3600).Int("rows", len(rows)).Msg("analytics: rollup write failed; hour skipped")
			})
		}
	}
	if !force {
		c.mu.Lock()
		c.flushed = nowHour - 1
		c.mu.Unlock()
	}
	c.invalidateCache()
}

// rowsForHourLocked renders one hour's aggregates as rollup rows, or nil when
// nothing was recorded for it, within rowBudgetPerHour: totals, status,
// latency and denial reasons first, then countries, signatures, signature
// countries and clients, so a budget overrun drops the least important rows.
// Caller holds mu.
func (c *Collector) rowsForHourLocked(h int64) []RollupRow {
	hs := &c.hours[h%hourSlots]
	hour := h * 3600
	persistClients := c.clients.Load().(string) == "persist"
	totals := map[aggKey]*hostMinute{}
	for m := h * 60; m < (h+1)*60; m++ {
		ms := &c.minutes[m%minuteSlots]
		if ms.minute != m {
			continue
		}
		for k, hm := range ms.hosts {
			t := totals[k]
			if t == nil {
				t = &hostMinute{}
				totals[k] = t
			}
			t.req += hm.req
			t.den += hm.den
			t.thr += hm.thr
			t.bytes += hm.bytes
			t.s2 += hm.s2
			t.s3 += hm.s3
			t.s4 += hm.s4
			t.s5 += hm.s5
			t.sOther += hm.sOther
			for i := range hm.dur {
				t.dur[i] += hm.dur[i]
			}
		}
	}
	var tier0, tier1, tier2, tier3, tier4 []RollupRow
	for k, t := range totals {
		host, src := k.host, k.src
		tier0 = append(tier0, RollupRow{Hour: hour, Host: host, Src: src, Dim: "total", Key: "", N: t.req, D: t.den, T: t.thr, B: t.bytes})
		for _, kv := range []struct {
			k string
			v int64
		}{{"2xx", t.s2}, {"3xx", t.s3}, {"4xx", t.s4}, {"5xx", t.s5}, {"other", t.sOther}} {
			if kv.v > 0 {
				tier0 = append(tier0, RollupRow{Hour: hour, Host: host, Src: src, Dim: "status", Key: kv.k, N: kv.v})
			}
		}
		for i, v := range t.dur {
			if v > 0 {
				tier0 = append(tier0, RollupRow{Hour: hour, Host: host, Src: src, Dim: "dur", Key: durKey(i), N: v})
			}
		}
	}
	if hs.hour == h {
		for ak, hh := range hs.hosts {
			host, src := ak.host, ak.src
			for k, v := range hh.deny {
				tier0 = append(tier0, RollupRow{Hour: hour, Host: host, Src: src, Dim: "deny", Key: k, N: v})
			}
			for k, v := range hh.country {
				tier1 = append(tier1, RollupRow{Hour: hour, Host: host, Src: src, Dim: "country", Key: k, N: v.n, D: v.d, T: v.t})
			}
			for k, v := range hh.sig {
				tier2 = append(tier2, RollupRow{Hour: hour, Host: host, Src: src, Dim: "sig", Key: k, N: v.n})
				for _, cc := range topKeys(hh.sigCountry[k], 3) {
					tier3 = append(tier3, RollupRow{Hour: hour, Host: host, Src: src, Dim: "sig", Key: k + "|" + cc, N: hh.sigCountry[k][cc]})
				}
			}
			if persistClients {
				type kv struct {
					k string
					v *clientAgg
				}
				list := make([]kv, 0, len(hh.clients))
				for k, v := range hh.clients {
					if v.persist {
						list = append(list, kv{k, v})
					}
				}
				sort.Slice(list, func(i, j int) bool { return list[i].v.n > list[j].v.n })
				if len(list) > persistedClients {
					list = list[:persistedClients]
				}
				for _, e := range list {
					tier4 = append(tier4, RollupRow{Hour: hour, Host: host, Src: src, Dim: "client", Key: e.k + "|" + e.v.country, N: e.v.n, D: e.v.d, T: e.v.t, B: e.v.lastSeen})
				}
			}
		}
	}
	rows := make([]RollupRow, 0, len(tier0)+len(tier1)+len(tier2)+len(tier3)+len(tier4))
	for _, tier := range [][]RollupRow{tier0, tier1, tier2, tier3, tier4} {
		if len(rows)+len(tier) > rowBudgetPerHour {
			tier = tier[:max(0, rowBudgetPerHour-len(rows))]
		}
		rows = append(rows, tier...)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows
}

func durKey(i int) string {
	if i >= len(durBounds) {
		return "inf"
	}
	return strconv.FormatInt(durBounds[i], 10)
}

func durIndex(key string) int {
	if key == "inf" {
		return durBucketCount - 1
	}
	v, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return durBucketCount - 1
	}
	for i, b := range durBounds {
		if b == v {
			return i
		}
	}
	return durBucketCount - 1
}

func topKeys(m map[string]int64, n int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > n {
		keys = keys[:n]
	}
	return keys
}

// replay loads the last 24 h of rollups, the running hour included, into the
// memory window at start so a restart does not blank the view. Hourly totals
// are spread evenly over the hour's minutes (the store has no finer
// resolution); the running hour only over the minutes already elapsed.
func (c *Collector) replay() {
	now := c.opts.Now()
	nowHour := now.Unix() / 3600
	rows, err := c.opts.Store.ReadHours((nowHour-24)*3600, (nowHour+1)*3600, "", "")
	if err != nil {
		log.Error().Err(err).Msg("analytics: rollup replay failed")
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushed = nowHour - 1
	for _, r := range rows {
		c.applyRowLocked(r, now.Unix()/60)
	}
	if len(rows) > 0 {
		log.Info().Int("rows", len(rows)).Msg("analytics: rollups replayed into the memory window")
	}
}

func (c *Collector) applyRowLocked(r RollupRow, nowMin int64) {
	h := r.Hour / 3600
	hs := &c.hours[h%hourSlots]
	if hs.hour != h {
		hs.hour = h
		hs.hosts = map[aggKey]*hostHour{}
	}
	key := aggKey{r.Host, r.Src}
	hh := hs.hosts[key]
	if hh == nil {
		hh = newHostHour()
		hs.hosts[key] = hh
	}
	if len(c.hostSet) < maxHostsTracked {
		c.hostSet[r.Host] = struct{}{}
	}
	switch r.Dim {
	case "country", "deny", "sig", "client":
		applyDimRow(hh, r)
	case "total", "status", "dur":
		last := (h + 1) * 60
		if last > nowMin+1 {
			last = nowMin + 1
		}
		span := last - h*60
		if span <= 0 {
			return
		}
		for m := h * 60; m < last; m++ {
			ms := &c.minutes[m%minuteSlots]
			if ms.minute != m {
				ms.minute = m
				ms.hosts = map[aggKey]*hostMinute{}
			}
			hm := ms.hosts[key]
			if hm == nil {
				hm = &hostMinute{}
				ms.hosts[key] = hm
			}
			share := func(v int64) int64 {
				q := v / span
				if (m - h*60) < v%span {
					q++
				}
				return q
			}
			switch r.Dim {
			case "total":
				hm.req += share(r.N)
				hm.den += share(r.D)
				hm.thr += share(r.T)
				hm.bytes += share(r.B)
			case "status":
				switch r.Key {
				case "2xx":
					hm.s2 += share(r.N)
				case "3xx":
					hm.s3 += share(r.N)
				case "4xx":
					hm.s4 += share(r.N)
				case "5xx":
					hm.s5 += share(r.N)
				default:
					hm.sOther += share(r.N)
				}
			case "dur":
				hm.dur[durIndex(r.Key)] += share(r.N)
			}
		}
	}
}

// applyDimRow folds one country/deny/sig/client rollup row into hh.
func applyDimRow(hh *hostHour, r RollupRow) {
	switch r.Dim {
	case "country":
		hh.country[r.Key] = &dimCounts{n: r.N, d: r.D, t: r.T}
	case "deny":
		hh.deny[r.Key] = r.N
	case "sig":
		if i := indexByte(r.Key, '|'); i >= 0 {
			label, cc := r.Key[:i], r.Key[i+1:]
			if hh.sigCountry[label] == nil {
				hh.sigCountry[label] = map[string]int64{}
			}
			hh.sigCountry[label][cc] = r.N
			if hh.sig[label] == nil {
				hh.sig[label] = &dimCounts{}
			}
		} else {
			if hh.sig[r.Key] == nil {
				hh.sig[r.Key] = &dimCounts{}
			}
			hh.sig[r.Key].n = r.N
			if hh.sigCountry[r.Key] == nil {
				hh.sigCountry[r.Key] = map[string]int64{}
			}
		}
	case "client":
		if i := indexByte(r.Key, '|'); i >= 0 {
			hh.clients[r.Key[:i]] = &clientAgg{n: r.N, d: r.D, t: r.T, country: r.Key[i+1:], lastSeen: r.B, persist: true}
		}
	}
}

// cleanEventPath mirrors the router's canonical form: a leading slash, dot
// segments resolved, a trailing slash kept. A path that does not start with
// "/" is left alone so the classifier can still label it proto-abuse.
func cleanEventPath(p string) string {
	if p == "" || p[0] != '/' {
		return p
	}
	np := path.Clean(p)
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func (c *Collector) invalidateCache() {
	c.cacheMu.Lock()
	c.cache = map[string]cachedReport{}
	c.cacheMu.Unlock()
}
