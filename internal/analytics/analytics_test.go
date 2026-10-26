package analytics

import (
	"testing"
	"time"
)

func TestParseRange(t *testing.T) {
	for _, r := range Ranges() {
		dur, bucket, ok := ParseRange(r)
		if !ok || dur <= 0 || bucket <= 0 || dur%bucket != 0 {
			t.Errorf("%s: dur=%v bucket=%v ok=%v", r, dur, bucket, ok)
		}
	}
	if _, _, ok := ParseRange("2h"); ok {
		t.Error("2h should not parse")
	}
}

func TestDemoIsDeterministicAndBounded(t *testing.T) {
	now := time.Date(2026, 10, 26, 3, 14, 0, 0, time.UTC)
	hosts := []string{"a", "b", "c"}
	a, err := Demo(hosts, now, Query{Range: "24h"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Demo(hosts, now.Add(20*time.Minute), Query{Range: "24h"})
	if a.Totals != b.Totals {
		t.Error("same hour must give the same totals")
	}
	if len(a.Series) != 48 || a.BucketSeconds != 1800 {
		t.Errorf("24h: %d buckets of %ds", len(a.Series), a.BucketSeconds)
	}
	if !a.Demo || !a.Enabled {
		t.Error("demo report must be flagged demo and enabled")
	}
	var sum int64
	for _, s := range a.Series {
		if s.Denied > s.Requests || s.Threats > s.Requests {
			t.Errorf("bucket %d: denied/threats exceed requests", s.T)
		}
		sum += s.Requests
	}
	if sum != a.Totals.Requests {
		t.Errorf("series sum %d != totals %d", sum, a.Totals.Requests)
	}
	if len(a.Hosts) != len(hosts)+1 || a.Hosts[len(a.Hosts)-1].Name != NoHost {
		t.Errorf("hosts rows = %+v", a.Hosts)
	}
	for _, c := range a.Countries {
		if c.Denied > c.Requests {
			t.Errorf("%s denied > requests", c.Code)
		}
	}
}

func TestDemoHostFilter(t *testing.T) {
	now := time.Now()
	r, err := Demo([]string{"a", "b"}, now, Query{Range: "1h", Host: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Host != "b" || len(r.Hosts) != 0 {
		t.Errorf("host view must not carry the hosts table: %+v", r.Hosts)
	}
	if _, err := Demo([]string{"a"}, now, Query{Range: "1h", Host: "zz"}); err == nil {
		t.Error("unknown host must error")
	}
	if _, err := Demo(nil, now, Query{Range: "9h"}); err == nil {
		t.Error("unknown range must error")
	}
}

func TestDemoHostRowMatchesHostView(t *testing.T) {
	now := time.Date(2026, 10, 26, 5, 0, 0, 0, time.UTC)
	hosts := []string{"www", "api", "app"}
	g, _ := Demo(hosts, now, Query{Range: "24h"})
	for _, h := range hosts {
		v, err := Demo(hosts, now, Query{Range: "24h", Host: h})
		if err != nil {
			t.Fatal(err)
		}
		var row HostRow
		for _, r := range g.Hosts {
			if r.Name == h {
				row = r
			}
		}
		if row.Requests != v.Totals.Requests || row.Denied != v.Totals.Denied || row.Threats != v.Totals.Threats || row.P95ms != v.Totals.P95ms {
			t.Errorf("%s: row %+v != view totals %+v", h, row, v.Totals)
		}
	}
	var sum int64
	for _, r := range g.Hosts {
		sum += r.Requests
	}
	if sum != g.Totals.Requests {
		t.Errorf("host rows sum %d != global total %d", sum, g.Totals.Requests)
	}
}
