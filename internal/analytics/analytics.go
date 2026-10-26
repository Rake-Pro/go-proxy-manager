// Package analytics defines the traffic analytics report the admin UI's
// Analytics view renders, the time ranges it accepts, and a deterministic
// seed-data generator used while the real collector is being built.
//
// Every aggregate in a Report is keyed by a bounded vocabulary: the operator's
// proxy host NAMES (never a client Host header), status classes, ISO country
// codes, denial reasons and signature labels. Client addresses appear only in
// the bounded top-clients list.
package analytics

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

// Query selects the slice a Report covers.
type Query struct {
	// Range is one of Ranges(): "1h", "6h", "24h", "7d", "30d".
	Range string
	// Host narrows the report to one proxy host name; empty means every host.
	Host string
	// Scope narrows the report to one network source, SourceExternal or
	// SourceInternal (see IsInternalIP); empty means both.
	Scope string
}

type rangeSpec struct {
	dur, bucket time.Duration
}

var rangeSpecs = map[string]rangeSpec{
	"1h":  {time.Hour, time.Minute},
	"6h":  {6 * time.Hour, 5 * time.Minute},
	"24h": {24 * time.Hour, 30 * time.Minute},
	"7d":  {7 * 24 * time.Hour, 3 * time.Hour},
	"30d": {30 * 24 * time.Hour, 12 * time.Hour},
}

var rangeOrder = []string{"1h", "6h", "24h", "7d", "30d"}

// Ranges lists the accepted range keys, shortest first.
func Ranges() []string { return append([]string(nil), rangeOrder...) }

// ParseRange returns the window and bucket width for a range key.
func ParseRange(s string) (dur, bucket time.Duration, ok bool) {
	r, ok := rangeSpecs[s]
	return r.dur, r.bucket, ok
}

// Report is the GET /api/analytics payload.
type Report struct {
	Enabled       bool      `json:"enabled"`
	Demo          bool      `json:"demo"`
	GeoIP         bool      `json:"geoip"`
	Range         string    `json:"range"`
	BucketSeconds int       `json:"bucketSeconds"`
	Host          string    `json:"host"`
	Scope         string    `json:"scope"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Totals        Totals    `json:"totals"`
	Previous      Totals    `json:"previous"`
	Series        []Bucket  `json:"series"`
	Countries     []Country `json:"countries"`
	Denials       []Denial  `json:"denials"`
	Signatures    []Sig     `json:"signatures"`
	Hosts         []HostRow `json:"hosts,omitempty"`
	Clients       []Client  `json:"clients"`
	// Ranges lists the range keys this instance can serve (the long ranges
	// need rollups and enough retention); Dropped counts events lost to a full
	// queue since start.
	Ranges  []string `json:"ranges,omitempty"`
	Dropped int64    `json:"dropped,omitempty"`
}

// Totals are the headline figures for a window.
type Totals struct {
	Requests  int64 `json:"requests"`
	BytesOut  int64 `json:"bytesOut"`
	Denied    int64 `json:"denied"`
	Threats   int64 `json:"threats"`
	Status4xx int64 `json:"status4xx"`
	Status5xx int64 `json:"status5xx"`
	P50ms     int   `json:"p50ms"`
	P95ms     int   `json:"p95ms"`
}

// Bucket is one time slot of the series.
type Bucket struct {
	T         int64 `json:"t"`
	Requests  int64 `json:"requests"`
	Denied    int64 `json:"denied"`
	Threats   int64 `json:"threats"`
	Status5xx int64 `json:"status5xx"`
	P95ms     int   `json:"p95ms"`
	BytesOut  int64 `json:"bytesOut"`
}

// Country is one row of the countries table. Code "--" is unknown or private.
type Country struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Requests int64  `json:"requests"`
	Denied   int64  `json:"denied"`
	Threats  int64  `json:"threats"`
}

// Denial is one row of the denials-by-reason table.
type Denial struct {
	Reason string `json:"reason"`
	N      int64  `json:"n"`
}

// Sig is one row of the threat-signatures table.
type Sig struct {
	Label     string   `json:"label"`
	N         int64    `json:"n"`
	Countries []string `json:"countries"`
	Hosts     int      `json:"hosts"`
}

// HostRow is one row of the per-host table (global view only).
type HostRow struct {
	Name      string `json:"name"`
	Requests  int64  `json:"requests"`
	Status4xx int64  `json:"status4xx"`
	Status5xx int64  `json:"status5xx"`
	Denied    int64  `json:"denied"`
	Threats   int64  `json:"threats"`
	P95ms     int    `json:"p95ms"`
}

// Client is one row of the top-clients table.
type Client struct {
	IP       string    `json:"ip"`
	Country  string    `json:"country"`
	Requests int64     `json:"requests"`
	Denied   int64     `json:"denied"`
	Threats  int64     `json:"threats"`
	LastSeen time.Time `json:"lastSeen"`
}

// ErrUnknownHost is returned when a Query names a host the report does not
// cover; the API maps it to 400 and everything else to a generic 500.
var ErrUnknownHost = errors.New("unknown host")

// NoHost is the host name for requests that matched no proxy host (unknown
// Host header, bare IP). It is the same label the metrics use.
const NoHost = "-"

var demoCountries = []Country{
	{"US", "United States", 0, 0, 0}, {"DE", "Germany", 0, 0, 0}, {"NL", "Netherlands", 0, 0, 0},
	{"CN", "China", 0, 0, 0}, {"RU", "Russia", 0, 0, 0}, {"SG", "Singapore", 0, 0, 0},
	{"GB", "United Kingdom", 0, 0, 0}, {"FR", "France", 0, 0, 0}, {"BR", "Brazil", 0, 0, 0},
	{"IN", "India", 0, 0, 0}, {"--", "Unknown / private", 0, 0, 0},
}

// share of requests, share of denied, share of threats per country row above
var demoCountryMix = [][3]float64{
	{0.52, 0.07, 0.03}, {0.09, 0.03, 0.02}, {0.08, 0.12, 0.13}, {0.06, 0.22, 0.26}, {0.05, 0.20, 0.23},
	{0.04, 0.12, 0.13}, {0.04, 0.01, 0.00}, {0.03, 0.04, 0.04}, {0.02, 0.07, 0.07}, {0.02, 0.06, 0.06}, {0.05, 0.06, 0.03},
}

var demoReasons = []string{"bouncer", "access-list", "geo", "rate-limit", "guard", "auth-forward", "auth-oidc", "auth-basic"}
var demoReasonMix = []float64{0.33, 0.27, 0.20, 0.09, 0.04, 0.04, 0.02, 0.01}

var demoSigs = []Sig{
	{"wp-probe", 0, []string{"CN", "RU", "SG"}, 0}, {"secret-probe", 0, []string{"NL", "RU", "BR"}, 0},
	{"scanner-ua", 0, []string{"US", "DE", "NL"}, 0}, {"iot-rce", 0, []string{"CN", "IN"}, 0},
	{"admin-probe", 0, []string{"RU", "CN", "FR"}, 0}, {"traversal", 0, []string{"BR", "RU"}, 0},
	{"crowdsec:http-bad-user-agent", 0, []string{"NL", "SG"}, 0}, {"injection", 0, []string{"CN", "US"}, 0},
	{"guard:xmlrpc", 0, []string{"RU"}, 0},
}
var demoSigMix = []float64{0.31, 0.25, 0.19, 0.09, 0.07, 0.03, 0.03, 0.02, 0.01}

// Documentation-range addresses only (RFC 5737 / RFC 3849): seed data must
// never look like observed traffic in a public repository.
var demoClients = []Client{
	{"203.0.113.61", "NL", 0, 0, 0, time.Time{}}, {"198.51.100.9", "SG", 0, 0, 0, time.Time{}},
	{"192.0.2.34", "DE", 0, 0, 0, time.Time{}}, {"203.0.113.188", "CN", 0, 0, 0, time.Time{}},
	{"198.51.100.172", "RU", 0, 0, 0, time.Time{}}, {"2001:db8:2::1", "NL", 0, 0, 0, time.Time{}},
	{"192.0.2.44", "US", 0, 0, 0, time.Time{}},
}

// Private addresses for the internal-source seed view (RFC 1918 / ULA).
var demoInternalClients = []Client{
	{"10.0.0.12", "--", 0, 0, 0, time.Time{}}, {"10.0.0.40", "--", 0, 0, 0, time.Time{}},
	{"10.0.1.7", "--", 0, 0, 0, time.Time{}}, {"fd00::1a", "--", 0, 0, 0, time.Time{}},
}

// DefaultDemoHosts is used when the config has no proxy hosts to name.
var DefaultDemoHosts = []string{"www", "api", "app", "docs", "git", "media"}

// Demo builds a deterministic, plausible report for hosts over q. The same
// (hosts, q, now-hour) always produce the same numbers so a page refresh does
// not shuffle the tables, and a host's row in the global view equals the
// totals of that host's own view, because both come from the same per-host
// series. It exists so the admin UI can be built and reviewed against
// realistic data before the collector ships; the API marks it demo.
func Demo(hosts []string, now time.Time, q Query) (Report, error) {
	dur, bucket, ok := ParseRange(q.Range)
	if !ok {
		return Report{}, fmt.Errorf("unknown range %q (want one of %v)", q.Range, rangeOrder)
	}
	if !ValidScope(q.Scope) {
		return Report{}, fmt.Errorf("unknown scope %q (want one of %v)", q.Scope, Scopes())
	}
	if len(hosts) == 0 {
		hosts = DefaultDemoHosts
	}
	if q.Host != "" {
		found := false
		for _, h := range hosts {
			if h == q.Host {
				found = true
				break
			}
		}
		if !found {
			return Report{}, fmt.Errorf("%w %q", ErrUnknownHost, q.Host)
		}
	}
	// Seed from the range and the hour so the demo is stable within an hour
	// and drifts after.
	base := hashSeed(q.Range) + now.Truncate(time.Hour).Unix()
	n := int(dur / bucket)
	start := now.Truncate(bucket).Add(-time.Duration(n-1) * bucket)
	// Per-host weight: first host is the busiest; "no host" takes a fixed 18 %
	// scan floor of the fleet.
	weights := make([]float64, len(hosts))
	for i := range hosts {
		weights[i] = 1 / math.Pow(1.7, float64(i))
	}
	total := sumf(weights)
	perBucketBase := 240.0 * (bucket.Minutes() / 30) // ~240 req per 30 min fleet-wide
	// Source mix: about a third of the seed traffic comes from private
	// addresses and carries no denials or threats; almost none of it lands on
	// "no host".
	internal := q.Scope == SourceInternal
	scopeScale, noHostScale := 1.0, 0.18
	switch q.Scope {
	case SourceExternal:
		scopeScale = 0.68
	case SourceInternal:
		scopeScale, noHostScale = 0.32, 0.01
	}
	scrub := func(b []Bucket) []Bucket {
		if internal {
			for i := range b {
				b[i].Denied, b[i].Threats = 0, 0
			}
		}
		return b
	}
	burst := n * 2 / 3
	gen := func(seed int64, scale float64) []Bucket {
		rng := rand.New(rand.NewSource(seed))
		out := make([]Bucket, n)
		for i := 0; i < n; i++ {
			t := start.Add(time.Duration(i) * bucket)
			diurnal := 1 + 0.5*math.Sin(float64(i)/float64(n)*2*math.Pi*math.Max(1, dur.Hours()/24))
			r := perBucketBase * diurnal * scale * (0.8 + 0.4*rng.Float64())
			d := r * (0.04 + 0.10*rng.Float64())
			th := d * (0.3 + 0.6*rng.Float64())
			if i == burst {
				r += perBucketBase * 1.7 * scale
				d += perBucketBase * 1.1 * scale
				th += perBucketBase * 0.9 * scale
			}
			out[i] = Bucket{T: t.Unix(), Requests: int64(r), Denied: int64(d), Threats: int64(th),
				Status5xx: int64(rng.Intn(3)), P95ms: 300 + rng.Intn(180), BytesOut: int64(r * (35000 + 20000*rng.Float64()))}
		}
		return out
	}
	p95Of := func(name string) int { return 30 + int(hashSeed(name)%400) }
	sum := func(b []Bucket) Totals {
		var t Totals
		for _, x := range b {
			t.Requests += x.Requests
			t.Denied += x.Denied
			t.Threats += x.Threats
			t.Status5xx += x.Status5xx
			t.BytesOut += x.BytesOut
		}
		t.Status4xx = t.Denied + t.Requests/60
		return t
	}

	rep := Report{Enabled: true, Demo: true, GeoIP: true, Range: q.Range, BucketSeconds: int(bucket.Seconds()), Host: q.Host, Scope: q.Scope, GeneratedAt: now, Ranges: Ranges()}
	rng := rand.New(rand.NewSource(base))
	if q.Host != "" {
		for i, h := range hosts {
			if h == q.Host {
				rep.Series = scrub(gen(base+hashSeed(h), weights[i]/total*0.82*scopeScale))
			}
		}
		rep.Totals = sum(rep.Series)
		rep.Totals.P95ms = p95Of(q.Host)
	} else {
		rep.Series = make([]Bucket, n)
		for i := range rep.Series {
			rep.Series[i].T = start.Add(time.Duration(i) * bucket).Unix()
		}
		add := func(b []Bucket) {
			for i, x := range b {
				rep.Series[i].Requests += x.Requests
				rep.Series[i].Denied += x.Denied
				rep.Series[i].Threats += x.Threats
				rep.Series[i].Status5xx += x.Status5xx
				rep.Series[i].BytesOut += x.BytesOut
				if x.P95ms > rep.Series[i].P95ms {
					rep.Series[i].P95ms = x.P95ms
				}
			}
		}
		for i, h := range hosts {
			b := scrub(gen(base+hashSeed(h), weights[i]/total*0.82*scopeScale))
			t := sum(b)
			rep.Hosts = append(rep.Hosts, HostRow{Name: h, Requests: t.Requests, Status4xx: t.Status4xx, Status5xx: t.Status5xx,
				Denied: t.Denied, Threats: t.Threats, P95ms: p95Of(h)})
			add(b)
		}
		// Requests that matched nothing: most of the scanning lands here.
		nb := gen(base+hashSeed(NoHost), noHostScale)
		for i := range nb {
			nb[i].Denied = nb[i].Requests * 9 / 10
			nb[i].Threats = nb[i].Requests * 7 / 10
			nb[i].Status5xx = 0
			nb[i].P95ms = 3
		}
		nb = scrub(nb)
		nt := sum(nb)
		rep.Hosts = append(rep.Hosts, HostRow{Name: NoHost, Requests: nt.Requests, Status4xx: nt.Requests, Denied: nt.Denied, Threats: nt.Threats, P95ms: 3})
		add(nb)
		rep.Totals = sum(rep.Series)
		rep.Totals.P95ms = 380 + rng.Intn(60)
	}
	rep.Totals.P50ms = rep.Totals.P95ms / 8
	rep.Previous = Totals{
		Requests: int64(float64(rep.Totals.Requests) * 0.89), Denied: int64(float64(rep.Totals.Denied) * 0.72),
		Threats: int64(float64(rep.Totals.Threats) * 0.62), Status5xx: rep.Totals.Status5xx + 10,
		Status4xx: int64(float64(rep.Totals.Status4xx) * 0.8), P50ms: rep.Totals.P50ms + 2, P95ms: rep.Totals.P95ms + 12,
		BytesOut: int64(float64(rep.Totals.BytesOut) * 0.96),
	}
	rep.Denials, rep.Signatures, rep.Clients = []Denial{}, []Sig{}, []Client{}
	if internal {
		// Private sources have no country, no refusals and no scanner hits.
		rep.Countries = []Country{{Code: unknownCountry, Name: countryName(unknownCountry), Requests: rep.Totals.Requests}}
		for i, c := range demoInternalClients {
			c.Requests = int64(float64(rep.Totals.Requests) * (0.35 - 0.08*float64(i)))
			c.LastSeen = now.Add(-time.Duration(1+i*4) * time.Minute)
			rep.Clients = append(rep.Clients, c)
		}
		return rep, nil
	}
	for i, c := range demoCountries {
		m := demoCountryMix[i]
		if q.Scope == SourceExternal && c.Code == unknownCountry {
			continue // the private share is not in the external view
		}
		c.Requests = int64(float64(rep.Totals.Requests) * m[0])
		c.Denied = minInt(int64(float64(rep.Totals.Denied)*m[1]), c.Requests)
		c.Threats = minInt(int64(float64(rep.Totals.Threats)*m[2]), c.Denied+c.Requests/50)
		rep.Countries = append(rep.Countries, c)
	}
	sort.SliceStable(rep.Countries, func(i, j int) bool { return rep.Countries[i].Requests > rep.Countries[j].Requests })
	for i, r := range demoReasons {
		rep.Denials = append(rep.Denials, Denial{Reason: r, N: int64(float64(rep.Totals.Denied) * demoReasonMix[i])})
	}
	for i, sg := range demoSigs {
		sg.N = int64(float64(rep.Totals.Threats) * demoSigMix[i])
		sg.Hosts = 1 + rng.Intn(len(hosts))
		if q.Host != "" {
			sg.Hosts = 1
		}
		rep.Signatures = append(rep.Signatures, sg)
	}
	for i, c := range demoClients {
		c.Requests = int64(float64(rep.Totals.Threats) * (0.22 - 0.025*float64(i)))
		c.Denied = c.Requests
		c.Threats = int64(float64(c.Requests) * 0.92)
		if c.IP == "192.0.2.44" {
			c.Denied = 0
		}
		c.LastSeen = now.Add(-time.Duration(2+i*i*9) * time.Minute)
		rep.Clients = append(rep.Clients, c)
	}
	return rep, nil
}

// hashSeed folds a string into a stable int64 (FNV-1a) for seeding.
func hashSeed(s string) int64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return int64(h >> 1)
}

func sumf(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s
}

func minInt(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// isoNames gives display names for the country codes most traffic carries;
// anything else is shown as its code.
var isoNames = map[string]string{
	"US": "United States", "CA": "Canada", "MX": "Mexico", "BR": "Brazil", "AR": "Argentina", "CL": "Chile", "CO": "Colombia",
	"GB": "United Kingdom", "IE": "Ireland", "FR": "France", "DE": "Germany", "NL": "Netherlands", "BE": "Belgium", "LU": "Luxembourg",
	"CH": "Switzerland", "AT": "Austria", "IT": "Italy", "ES": "Spain", "PT": "Portugal", "SE": "Sweden", "NO": "Norway", "DK": "Denmark",
	"FI": "Finland", "IS": "Iceland", "PL": "Poland", "CZ": "Czechia", "SK": "Slovakia", "HU": "Hungary", "RO": "Romania", "BG": "Bulgaria",
	"GR": "Greece", "TR": "Turkey", "UA": "Ukraine", "RU": "Russia", "BY": "Belarus", "LT": "Lithuania", "LV": "Latvia", "EE": "Estonia",
	"RS": "Serbia", "HR": "Croatia", "SI": "Slovenia", "MD": "Moldova", "IL": "Israel", "AE": "United Arab Emirates", "SA": "Saudi Arabia",
	"IR": "Iran", "IQ": "Iraq", "EG": "Egypt", "ZA": "South Africa", "NG": "Nigeria", "KE": "Kenya", "MA": "Morocco",
	"IN": "India", "PK": "Pakistan", "BD": "Bangladesh", "CN": "China", "HK": "Hong Kong", "TW": "Taiwan", "JP": "Japan", "KR": "South Korea",
	"SG": "Singapore", "MY": "Malaysia", "ID": "Indonesia", "TH": "Thailand", "VN": "Vietnam", "PH": "Philippines", "AU": "Australia", "NZ": "New Zealand",
}
