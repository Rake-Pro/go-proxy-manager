package analytics

import (
	"fmt"
	"sort"
	"time"
)

// Report builds the analytics report for q from the memory window (1h, 6h,
// 24h) or from the hourly rollups plus the running hours (7d, 30d). The store
// is read without holding the collector lock, and a long-range report is
// cached for longReportCacheTTL so repeated readers do not re-scan the file.
func (c *Collector) Report(q Query) (Report, error) {
	dur, bucket, ok := ParseRange(q.Range)
	if !ok {
		return Report{}, fmt.Errorf("unknown range %q", q.Range)
	}
	if !ValidScope(q.Scope) {
		return Report{}, fmt.Errorf("unknown scope %q", q.Scope)
	}
	now := c.opts.Now()
	rep := Report{Enabled: c.enabled.Load(), Range: q.Range, BucketSeconds: int(bucket.Seconds()), Host: q.Host, Scope: q.Scope, GeneratedAt: now, Ranges: c.Ranges()}
	if c.opts.GeoLoaded != nil {
		rep.GeoIP = c.opts.GeoLoaded()
	}
	rep.Dropped = c.dropped.Load()
	if !rep.Enabled {
		rep.Series, rep.Countries, rep.Denials, rep.Signatures, rep.Clients = []Bucket{}, []Country{}, []Denial{}, []Sig{}, []Client{}
		return rep, nil
	}
	if !contains(rep.Ranges, q.Range) {
		return Report{}, ErrRangeUnavailable
	}
	if q.Host != "" && !c.knownHost(q.Host) {
		return Report{}, fmt.Errorf("%w %q", ErrUnknownHost, q.Host)
	}
	long := dur > 24*time.Hour
	key := q.Range + "|" + q.Host + "|" + q.Scope
	if long {
		c.cacheMu.Lock()
		if e, ok := c.cache[key]; ok && now.Sub(e.at) < longReportCacheTTL {
			c.cacheMu.Unlock()
			e.rep.GeneratedAt = now
			e.rep.Dropped = rep.Dropped
			return e.rep, nil
		}
		c.cacheMu.Unlock()
	}
	end := now.Truncate(bucket).Add(bucket)
	start := end.Add(-dur)
	var agg, prev *window
	var err error
	if !long {
		c.mu.Lock()
		agg = c.memoryWindow(start, end, bucket, q.Host, q.Scope)
		prev = c.memoryWindow(start.Add(-dur), start, bucket, q.Host, q.Scope)
		c.mu.Unlock()
		if dur == 24*time.Hour && c.opts.Store != nil {
			// The previous day is not in memory; read it from the rollups.
			if pw, perr := c.storeWindow(start.Add(-dur), start, bucket, q.Host, q.Scope); perr == nil {
				prev = pw
			}
		}
	} else {
		agg, err = c.storeWindow(start, end, bucket, q.Host, q.Scope)
		if err != nil {
			return Report{}, err
		}
		prev, err = c.storeWindow(start.Add(-dur), start, bucket, q.Host, q.Scope)
		if err != nil {
			prev = newWindow(start.Add(-dur), start, bucket)
		}
	}
	rep.Previous = prev.totals()
	agg.fill(&rep, q.Host == "", c.clients.Load().(string) != "off")
	if long {
		c.cacheMu.Lock()
		c.cache[key] = cachedReport{at: now, rep: rep}
		c.cacheMu.Unlock()
	}
	return rep, nil
}

func (c *Collector) knownHost(h string) bool {
	if c.opts.KnownHost != nil && c.opts.KnownHost(h) {
		return true
	}
	c.mu.Lock()
	_, ok := c.hostSet[h]
	c.mu.Unlock()
	return ok
}

// Ranges lists the ranges this instance can serve: the memory ranges always,
// the long ones when rollups exist and retention covers them.
func (c *Collector) Ranges() []string {
	out := []string{"1h", "6h", "24h"}
	if c.opts.Store != nil {
		ret := int(c.retDays.Load())
		if ret >= 7 {
			out = append(out, "7d")
		}
		if ret >= 30 {
			out = append(out, "30d")
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// window is the intermediate aggregate a report is rendered from, independent
// of whether it came from the minute slots or from rollup rows.
type window struct {
	start, end time.Time
	bucket     time.Duration
	series     []Bucket
	seriesDur  [][durBucketCount]int64
	byHost     map[string]*hostMinute // totals per host over the window
	country    map[string]*dimCounts
	deny       map[string]int64
	sig        map[string]*dimCounts
	sigCountry map[string]map[string]int64
	sigHosts   map[string]map[string]struct{}
	clients    map[string]*clientAgg
	dur        [durBucketCount]int64
}

func newWindow(start, end time.Time, bucket time.Duration) *window {
	n := int(end.Sub(start) / bucket)
	w := &window{start: start, end: end, bucket: bucket, series: make([]Bucket, n), seriesDur: make([][durBucketCount]int64, n),
		byHost: map[string]*hostMinute{}, country: map[string]*dimCounts{}, deny: map[string]int64{}, sig: map[string]*dimCounts{},
		sigCountry: map[string]map[string]int64{}, sigHosts: map[string]map[string]struct{}{}, clients: map[string]*clientAgg{}}
	for i := range w.series {
		w.series[i].T = start.Add(time.Duration(i) * bucket).Unix()
	}
	return w
}

func (w *window) bucketIndex(t time.Time) int {
	if t.Before(w.start) || !t.Before(w.end) {
		return -1
	}
	return int(t.Sub(w.start) / w.bucket)
}

func (w *window) hostTotals(host string) *hostMinute {
	t := w.byHost[host]
	if t == nil {
		t = &hostMinute{}
		w.byHost[host] = t
	}
	return t
}

func (w *window) addMinute(host string, at time.Time, hm *hostMinute) {
	i := w.bucketIndex(at)
	if i < 0 {
		return
	}
	b := &w.series[i]
	b.Requests += hm.req
	b.Denied += hm.den
	b.Threats += hm.thr
	b.Status5xx += hm.s5
	b.BytesOut += hm.bytes
	for k := range hm.dur {
		w.seriesDur[i][k] += hm.dur[k]
		w.dur[k] += hm.dur[k]
	}
	t := w.hostTotals(host)
	t.req += hm.req
	t.den += hm.den
	t.thr += hm.thr
	t.bytes += hm.bytes
	t.s2 += hm.s2
	t.s3 += hm.s3
	t.s4 += hm.s4
	t.s5 += hm.s5
	t.sOther += hm.sOther
	for k := range hm.dur {
		t.dur[k] += hm.dur[k]
	}
}

func (w *window) addHour(host string, hh *hostHour) {
	for k, v := range hh.country {
		w.addCountry(k, v.n, v.d, v.t)
	}
	for k, v := range hh.deny {
		w.deny[k] += v
	}
	for k, v := range hh.sig {
		w.addSig(host, k, v.n)
		for cc, n := range hh.sigCountry[k] {
			w.sigCountry[k][cc] += n
		}
	}
	for k, v := range hh.clients {
		w.addClient(k, v.country, v.n, v.d, v.t, v.lastSeen)
	}
}

func (w *window) addCountry(code string, n, d, t int64) {
	c := w.country[code]
	if c == nil {
		c = &dimCounts{}
		w.country[code] = c
	}
	c.n += n
	c.d += d
	c.t += t
}

func (w *window) addSig(host, label string, n int64) {
	s := w.sig[label]
	if s == nil {
		s = &dimCounts{}
		w.sig[label] = s
		w.sigCountry[label] = map[string]int64{}
		w.sigHosts[label] = map[string]struct{}{}
	}
	s.n += n
	w.sigHosts[label][host] = struct{}{}
}

func (w *window) addClient(ip, country string, n, d, t, lastSeen int64) {
	c := w.clients[ip]
	if c == nil {
		c = &clientAgg{country: country}
		w.clients[ip] = c
	}
	c.n += n
	c.d += d
	c.t += t
	if lastSeen > c.lastSeen {
		c.lastSeen = lastSeen
	}
}

// addRow folds one rollup row straight into the window (no intermediate
// per-row structures: a 30 d read is millions of rows).
func (w *window) addRow(r RollupRow) {
	at := time.Unix(r.Hour, 0)
	switch r.Dim {
	case "total":
		w.addMinute(r.Host, at, &hostMinute{req: r.N, den: r.D, thr: r.T, bytes: r.B})
	case "status":
		t := w.hostTotals(r.Host)
		switch r.Key {
		case "2xx":
			t.s2 += r.N
		case "3xx":
			t.s3 += r.N
		case "4xx":
			t.s4 += r.N
		case "5xx":
			t.s5 += r.N
			if i := w.bucketIndex(at); i >= 0 {
				w.series[i].Status5xx += r.N
			}
		default:
			t.sOther += r.N
		}
	case "dur":
		k := durIndex(r.Key)
		w.hostTotals(r.Host).dur[k] += r.N
		w.dur[k] += r.N
		if i := w.bucketIndex(at); i >= 0 {
			w.seriesDur[i][k] += r.N
		}
	case "country":
		w.addCountry(r.Key, r.N, r.D, r.T)
	case "deny":
		w.deny[r.Key] += r.N
	case "sig":
		if i := indexByte(r.Key, '|'); i >= 0 {
			label, cc := r.Key[:i], r.Key[i+1:]
			if w.sig[label] == nil {
				w.addSig(r.Host, label, 0)
			}
			w.sigCountry[label][cc] += r.N
		} else {
			w.addSig(r.Host, r.Key, r.N)
		}
	case "client":
		if i := indexByte(r.Key, '|'); i >= 0 {
			w.addClient(r.Key[:i], r.Key[i+1:], r.N, r.D, r.T, r.B)
		}
	}
}

// memoryWindow aggregates the minute and hour slots overlapping [start,end).
// Caller holds c.mu. host "" means every host; src "" means every source.
func (c *Collector) memoryWindow(start, end time.Time, bucket time.Duration, host, src string) *window {
	w := newWindow(start, end, bucket)
	c.addMemoryLocked(w, start, end, host, src, nil)
	return w
}

// addMemoryLocked folds the memory slots within [start,end) into w, skipping
// hours present in skipHours (those came from the store). Caller holds c.mu.
// A src filter leaves out schema 1 rows replayed under src "" (their source
// is unknown); the unfiltered view counts them.
func (c *Collector) addMemoryLocked(w *window, start, end time.Time, host, src string, skipHours map[int64]struct{}) {
	for m := start.Unix() / 60; m < end.Unix()/60; m++ {
		if _, skip := skipHours[m/60]; skip {
			continue
		}
		ms := &c.minutes[m%minuteSlots]
		if ms.minute != m {
			continue
		}
		at := time.Unix(m*60, 0)
		for k, hm := range ms.hosts {
			if (host != "" && k.host != host) || (src != "" && k.src != src) {
				continue
			}
			w.addMinute(k.host, at, hm)
		}
	}
	for hr := start.Unix() / 3600; hr < (end.Unix()+3599)/3600; hr++ {
		if _, skip := skipHours[hr]; skip {
			continue
		}
		hs := &c.hours[hr%hourSlots]
		if hs.hour != hr {
			continue
		}
		for k, hh := range hs.hosts {
			if (host != "" && k.host != host) || (src != "" && k.src != src) {
				continue
			}
			w.addHour(k.host, hh)
		}
	}
}

// storeWindow aggregates rollup rows for [start,end) plus the memory slots
// for hours the store does not hold (the running hour, anything unflushed).
// The store is read WITHOUT the collector lock; only the memory pass locks.
func (c *Collector) storeWindow(start, end time.Time, bucket time.Duration, host, src string) (*window, error) {
	w := newWindow(start, end, bucket)
	stored := map[int64]struct{}{}
	if c.opts.Store != nil {
		rows, err := c.opts.Store.ReadHours(start.Unix(), end.Unix(), host, src)
		if err != nil {
			return nil, err
		}
		// The running hour is always served from memory: a shutdown flush may
		// have written a partial copy of it, and replay put that copy back into
		// memory, so folding the stored rows too would hide everything recorded
		// since the restart.
		nowHour := c.opts.Now().Unix() / 3600
		for _, r := range rows {
			if r.Hour/3600 >= nowHour {
				continue
			}
			stored[r.Hour/3600] = struct{}{}
			w.addRow(r)
		}
	}
	c.mu.Lock()
	c.addMemoryLocked(w, start, end, host, src, stored)
	c.mu.Unlock()
	return w, nil
}

func (w *window) totals() Totals {
	var t Totals
	for _, b := range w.series {
		t.Requests += b.Requests
		t.Denied += b.Denied
		t.Threats += b.Threats
		t.Status5xx += b.Status5xx
		t.BytesOut += b.BytesOut
	}
	for _, h := range w.byHost {
		t.Status4xx += h.s4
	}
	t.P50ms = percentile(w.dur, 0.50)
	t.P95ms = percentile(w.dur, 0.95)
	return t
}

// percentile estimates a latency percentile from the fixed histogram: the
// upper bound of the bucket where the cumulative count crosses p.
func percentile(h [durBucketCount]int64, p float64) int {
	var total int64
	for _, v := range h {
		total += v
	}
	if total == 0 {
		return 0
	}
	target := int64(float64(total) * p)
	var cum int64
	for i, v := range h {
		cum += v
		if cum >= target && cum > 0 {
			if i >= len(durBounds) {
				return int(durBounds[len(durBounds)-1]) * 2
			}
			return int(durBounds[i])
		}
	}
	return int(durBounds[len(durBounds)-1]) * 2
}

// fill renders the window into the report tables.
func (w *window) fill(rep *Report, withHosts, withClients bool) {
	rep.Series = w.series
	for i := range rep.Series {
		rep.Series[i].P95ms = percentile(w.seriesDur[i], 0.95)
	}
	rep.Totals = w.totals()
	rep.Countries = make([]Country, 0, len(w.country))
	for k, v := range w.country {
		rep.Countries = append(rep.Countries, Country{Code: k, Name: countryName(k), Requests: v.n, Denied: v.d, Threats: v.t})
	}
	sort.Slice(rep.Countries, func(i, j int) bool {
		if rep.Countries[i].Requests != rep.Countries[j].Requests {
			return rep.Countries[i].Requests > rep.Countries[j].Requests
		}
		return rep.Countries[i].Code < rep.Countries[j].Code
	})
	if len(rep.Countries) > 30 {
		// The tail folds into one row (empty code, so the UI offers no copy) so
		// the table still sums to the totals.
		var rest Country
		for _, x := range rep.Countries[30:] {
			rest.Requests += x.Requests
			rest.Denied += x.Denied
			rest.Threats += x.Threats
		}
		rest.Name = fmt.Sprintf("Other (%d more)", len(rep.Countries)-30)
		rep.Countries = append(rep.Countries[:30], rest)
	}
	rep.Denials = make([]Denial, 0, len(w.deny))
	for k, v := range w.deny {
		rep.Denials = append(rep.Denials, Denial{Reason: k, N: v})
	}
	sort.Slice(rep.Denials, func(i, j int) bool {
		if rep.Denials[i].N != rep.Denials[j].N {
			return rep.Denials[i].N > rep.Denials[j].N
		}
		return rep.Denials[i].Reason < rep.Denials[j].Reason
	})
	rep.Signatures = make([]Sig, 0, len(w.sig))
	for k, v := range w.sig {
		rep.Signatures = append(rep.Signatures, Sig{Label: k, N: v.n, Countries: topKeys(w.sigCountry[k], 3), Hosts: len(w.sigHosts[k])})
	}
	sort.Slice(rep.Signatures, func(i, j int) bool {
		if rep.Signatures[i].N != rep.Signatures[j].N {
			return rep.Signatures[i].N > rep.Signatures[j].N
		}
		return rep.Signatures[i].Label < rep.Signatures[j].Label
	})
	rep.Clients = []Client{}
	if withClients {
		rep.Clients = make([]Client, 0, len(w.clients))
		for k, v := range w.clients {
			rep.Clients = append(rep.Clients, Client{IP: k, Country: v.country, Requests: v.n, Denied: v.d, Threats: v.t, LastSeen: time.Unix(v.lastSeen, 0).UTC()})
		}
		sort.Slice(rep.Clients, func(i, j int) bool {
			if rep.Clients[i].Requests != rep.Clients[j].Requests {
				return rep.Clients[i].Requests > rep.Clients[j].Requests
			}
			return rep.Clients[i].IP < rep.Clients[j].IP
		})
		if len(rep.Clients) > 50 {
			rep.Clients = rep.Clients[:50]
		}
	}
	if withHosts {
		rep.Hosts = make([]HostRow, 0, len(w.byHost))
		for h, t := range w.byHost {
			rep.Hosts = append(rep.Hosts, HostRow{Name: h, Requests: t.req, Status4xx: t.s4, Status5xx: t.s5, Denied: t.den, Threats: t.thr, P95ms: percentile(t.dur, 0.95)})
		}
		sort.Slice(rep.Hosts, func(i, j int) bool {
			if rep.Hosts[i].Requests != rep.Hosts[j].Requests {
				return rep.Hosts[i].Requests > rep.Hosts[j].Requests
			}
			return rep.Hosts[i].Name < rep.Hosts[j].Name
		})
	}
	if rep.Series == nil {
		rep.Series = []Bucket{}
	}
}

// countryName maps the ISO codes the catalogue knows to a display name; an
// unknown code is shown as itself.
func countryName(code string) string {
	if code == unknownCountry {
		return "Unknown / private"
	}
	if n, ok := isoNames[code]; ok {
		return n
	}
	return code
}
