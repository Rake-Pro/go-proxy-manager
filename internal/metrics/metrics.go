// Package metrics is a dependency-free implementation of the small slice of
// Prometheus that gpm actually needs: integer counters and gauges, fixed-bucket
// histograms, and a text-exposition (version 0.0.4) writer.
//
// It exists instead of client_golang because that library and its transitive
// tree (protobuf, procfs, common/expfmt, ...) would multiply this project's
// vetted dependency set several times over for one read-only endpoint - which
// is the opposite of the reason this project exists (see CLAUDE.md). The
// trade-offs it accepts in exchange are deliberate: values are int64, not
// float64 (every metric gpm exports is a count, a byte total or a unix
// timestamp); histograms have fixed buckets chosen at registration; and there
// is no push gateway, no exemplars and no native histograms.
//
// Series cardinality is bounded per metric (MaxSeriesPerMetric). A label set
// arriving past the cap is folded into a single overflow series rather than
// allocating an unbounded map, so no request-driven label value can grow the
// process without limit - the whole reason host labels are the operator's
// ProxyHost name and never a client-supplied Host header.
package metrics

import (
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Namespace prefixes every metric name gpm exports.
const Namespace = "gpm_"

// MaxSeriesPerMetric bounds how many distinct label sets one metric keeps. It
// is a var only so tests can shrink it.
var MaxSeriesPerMetric = 512

// OverflowLabel replaces every label value of a series that arrives after a
// metric hit MaxSeriesPerMetric.
const OverflowLabel = "__overflow__"

type kind uint8

const (
	kindCounter kind = iota
	kindGauge
	kindHistogram
)

func (k kind) String() string {
	switch k {
	case kindGauge:
		return "gauge"
	case kindHistogram:
		return "histogram"
	default:
		return "counter"
	}
}

// Sample is one label set plus its value, returned by a pull-based collector.
// Labels are values aligned with the metric's registered label names.
type Sample struct {
	Labels []string
	Value  int64
}

// series is one label set's storage. Only the fields its metric's kind uses are
// ever touched.
type series struct {
	labels  []string
	value   atomic.Int64    // counter / gauge
	buckets []atomic.Uint64 // histogram: per-bucket (non-cumulative) counts
	sumUS   atomic.Uint64   // histogram: sum of observations, microseconds
	count   atomic.Uint64   // histogram: observation count
}

// vec is one registered metric: a name, help text, kind, label names, and either
// live series or a pull-based collector.
type vec struct {
	name    string
	help    string
	k       kind
	labels  []string
	buckets []float64
	collect func() []Sample

	mu       sync.RWMutex
	series   map[string]*series
	overflow *series
}

// Registry holds the registered metrics in registration order.
type Registry struct {
	mu    sync.Mutex
	order []*vec
}

// New returns an empty Registry.
func New() *Registry { return &Registry{} }

func (r *Registry) add(v *vec) *vec {
	v.series = map[string]*series{}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, v)
	return v
}

// Counter registers a monotonically increasing integer counter.
func (r *Registry) Counter(name, help string, labels ...string) *Counter {
	return &Counter{r.add(&vec{name: name, help: help, k: kindCounter, labels: labels})}
}

// Gauge registers a settable integer gauge.
func (r *Registry) Gauge(name, help string, labels ...string) *Gauge {
	return &Gauge{r.add(&vec{name: name, help: help, k: kindGauge, labels: labels})}
}

// Histogram registers a fixed-bucket histogram. buckets are upper bounds in
// seconds and must be sorted ascending; the implicit +Inf bucket is added by the
// exposition writer.
func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *Histogram {
	b := append([]float64(nil), buckets...)
	sort.Float64s(b)
	return &Histogram{r.add(&vec{name: name, help: help, k: kindHistogram, labels: labels, buckets: b})}
}

// CounterFunc registers a counter whose samples are pulled from fn at scrape
// time. Used for values another subsystem already tracks (a reconciler's run
// counts, a certificate's expiry), so nothing has to be mirrored into a second
// source of truth.
func (r *Registry) CounterFunc(name, help string, labels []string, fn func() []Sample) {
	r.add(&vec{name: name, help: help, k: kindCounter, labels: labels, collect: fn})
}

// GaugeFunc registers a gauge whose samples are pulled from fn at scrape time.
func (r *Registry) GaugeFunc(name, help string, labels []string, fn func() []Sample) {
	r.add(&vec{name: name, help: help, k: kindGauge, labels: labels, collect: fn})
}

// lookup returns the series for these label values, creating it if there is
// room. Past MaxSeriesPerMetric every new label set collapses onto one shared
// overflow series, so cardinality is bounded no matter what drives the labels.
func (v *vec) lookup(values []string) *series {
	key := strings.Join(values, "\x1f")
	v.mu.RLock()
	s := v.series[key]
	v.mu.RUnlock()
	if s != nil {
		return s
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if s = v.series[key]; s != nil {
		return s
	}
	if len(v.series) >= MaxSeriesPerMetric {
		if v.overflow == nil {
			over := make([]string, len(v.labels))
			for i := range over {
				over[i] = OverflowLabel
			}
			v.overflow = v.newSeries(over)
			v.series[strings.Join(over, "\x1f")] = v.overflow
		}
		return v.overflow
	}
	s = v.newSeries(append([]string(nil), values...))
	v.series[key] = s
	return s
}

func (v *vec) newSeries(labels []string) *series {
	s := &series{labels: labels}
	if v.k == kindHistogram {
		s.buckets = make([]atomic.Uint64, len(v.buckets))
	}
	return s
}

// Counter is an integer counter vector.
type Counter struct{ v *vec }

// Inc adds one to the series identified by labelValues.
func (c *Counter) Inc(labelValues ...string) { c.Add(1, labelValues...) }

// Add adds n (which must not be negative) to the series identified by labelValues.
func (c *Counter) Add(n int64, labelValues ...string) {
	if n <= 0 {
		return
	}
	c.v.lookup(labelValues).value.Add(n)
}

// Gauge is an integer gauge vector.
type Gauge struct{ v *vec }

// Set replaces the series' value.
func (g *Gauge) Set(n int64, labelValues ...string) { g.v.lookup(labelValues).value.Store(n) }

// Add applies a delta (negative to decrement).
func (g *Gauge) Add(d int64, labelValues ...string) { g.v.lookup(labelValues).value.Add(d) }

// Histogram is a fixed-bucket duration histogram vector.
type Histogram struct{ v *vec }

// Observe records one duration against the series identified by labelValues.
func (h *Histogram) Observe(d time.Duration, labelValues ...string) {
	if d < 0 {
		d = 0
	}
	s := h.v.lookup(labelValues)
	secs := d.Seconds()
	for i, ub := range h.v.buckets {
		if secs <= ub {
			s.buckets[i].Add(1)
			break
		}
	}
	s.sumUS.Add(uint64(d.Microseconds()))
	s.count.Add(1)
}

// WriteTo renders the whole registry in Prometheus text exposition format
// 0.0.4. Metrics appear in registration order and series in sorted label order,
// so a scrape (and the golden test) is byte-stable.
func (r *Registry) WriteTo(w io.Writer) (int64, error) {
	var b strings.Builder
	r.mu.Lock()
	vecs := append([]*vec(nil), r.order...)
	r.mu.Unlock()
	for _, v := range vecs {
		v.write(&b)
	}
	n, err := io.WriteString(w, b.String())
	return int64(n), err
}

func (v *vec) write(b *strings.Builder) {
	b.WriteString("# HELP ")
	b.WriteString(v.name)
	b.WriteByte(' ')
	b.WriteString(escapeHelp(v.help))
	b.WriteByte('\n')
	b.WriteString("# TYPE ")
	b.WriteString(v.name)
	b.WriteByte(' ')
	b.WriteString(v.k.String())
	b.WriteByte('\n')

	if v.collect != nil {
		samples := v.collect()
		sort.SliceStable(samples, func(i, j int) bool {
			return strings.Join(samples[i].Labels, "\x1f") < strings.Join(samples[j].Labels, "\x1f")
		})
		for _, s := range samples {
			b.WriteString(v.name)
			writeLabels(b, v.labels, s.Labels, "", "")
			b.WriteByte(' ')
			b.WriteString(strconv.FormatInt(s.Value, 10))
			b.WriteByte('\n')
		}
		return
	}

	v.mu.RLock()
	all := make([]*series, 0, len(v.series))
	for _, s := range v.series {
		all = append(all, s)
	}
	v.mu.RUnlock()
	sort.Slice(all, func(i, j int) bool {
		return strings.Join(all[i].labels, "\x1f") < strings.Join(all[j].labels, "\x1f")
	})

	for _, s := range all {
		if v.k != kindHistogram {
			b.WriteString(v.name)
			writeLabels(b, v.labels, s.labels, "", "")
			b.WriteByte(' ')
			b.WriteString(strconv.FormatInt(s.value.Load(), 10))
			b.WriteByte('\n')
			continue
		}
		var cum uint64
		for i, ub := range v.buckets {
			cum += s.buckets[i].Load()
			b.WriteString(v.name)
			b.WriteString("_bucket")
			writeLabels(b, v.labels, s.labels, "le", formatFloat(ub))
			b.WriteByte(' ')
			b.WriteString(strconv.FormatUint(cum, 10))
			b.WriteByte('\n')
		}
		total := s.count.Load()
		b.WriteString(v.name)
		b.WriteString("_bucket")
		writeLabels(b, v.labels, s.labels, "le", "+Inf")
		b.WriteByte(' ')
		b.WriteString(strconv.FormatUint(total, 10))
		b.WriteByte('\n')

		b.WriteString(v.name)
		b.WriteString("_sum")
		writeLabels(b, v.labels, s.labels, "", "")
		b.WriteByte(' ')
		b.WriteString(formatFloat(float64(s.sumUS.Load()) / 1e6))
		b.WriteByte('\n')

		b.WriteString(v.name)
		b.WriteString("_count")
		writeLabels(b, v.labels, s.labels, "", "")
		b.WriteByte(' ')
		b.WriteString(strconv.FormatUint(total, 10))
		b.WriteByte('\n')
	}
}

// writeLabels renders {name="value",...}, optionally appending one extra pair
// (the histogram "le"). A label with no value at that index is skipped, so a
// caller passing fewer values than names cannot produce a malformed line.
func writeLabels(b *strings.Builder, names, values []string, extraName, extraValue string) {
	if len(names) == 0 && extraName == "" {
		return
	}
	b.WriteByte('{')
	first := true
	for i, n := range names {
		if i >= len(values) {
			break
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString(n)
		b.WriteString(`="`)
		b.WriteString(escapeLabel(values[i]))
		b.WriteByte('"')
	}
	if extraName != "" {
		if !first {
			b.WriteByte(',')
		}
		b.WriteString(extraName)
		b.WriteString(`="`)
		b.WriteString(escapeLabel(extraValue))
		b.WriteByte('"')
	}
	b.WriteByte('}')
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
var helpEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`)

func escapeLabel(s string) string { return labelEscaper.Replace(s) }
func escapeHelp(s string) string  { return helpEscaper.Replace(s) }

// formatFloat renders a float the way the exposition format expects: shortest
// round-tripping decimal, with +Inf/-Inf spelled out.
func formatFloat(f float64) string {
	switch {
	case f > 1e308*1.7:
		return "+Inf"
	case f < -1e308*1.7:
		return "-Inf"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// Handler serves the registry as a Prometheus scrape target. Callers are
// responsible for authenticating and authorizing the request first: the payload
// names every configured proxy host and certificate, which is not public data.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = r.WriteTo(w)
	})
}
