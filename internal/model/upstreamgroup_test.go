package model

import (
	"strings"
	"testing"
	"time"
)

func validGroup() UpstreamGroup {
	return UpstreamGroup{
		ObjectMeta: ObjectMeta{Name: "edge-nodes"},
		Upstreams: []GroupUpstream{
			{Upstream: Upstream{Scheme: "http", Host: "192.0.2.11", Port: 80}},
			{Upstream: Upstream{Scheme: "http", Host: "192.0.2.12", Port: 80}},
		},
	}
}

func TestUpstreamGroupValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*UpstreamGroup)
		wantErr string // substring; "" = valid
	}{
		{name: "valid two upstreams", mutate: func(g *UpstreamGroup) {}},
		{name: "single upstream is allowed", mutate: func(g *UpstreamGroup) { g.Upstreams = g.Upstreams[:1] }},
		{name: "no upstreams", mutate: func(g *UpstreamGroup) { g.Upstreams = nil }, wantErr: "at least one upstream"},
		{name: "bad upstream scheme", mutate: func(g *UpstreamGroup) { g.Upstreams[0].Scheme = "ftp" }, wantErr: "scheme"},
		{name: "duplicate upstream", mutate: func(g *UpstreamGroup) { g.Upstreams[1] = g.Upstreams[0] }, wantErr: "duplicate upstream"},
		{name: "health path without leading slash", mutate: func(g *UpstreamGroup) { g.HealthCheck.Path = "ping" }, wantErr: "healthCheck.path"},
		{name: "health path ok", mutate: func(g *UpstreamGroup) { g.HealthCheck.Path = "/ping" }},
		{name: "interval out of range", mutate: func(g *UpstreamGroup) { g.HealthCheck.IntervalSeconds = 9999 }, wantErr: "intervalSeconds"},
		{name: "timeout out of range", mutate: func(g *UpstreamGroup) { g.HealthCheck.TimeoutSeconds = 120 }, wantErr: "timeoutSeconds"},
		{name: "rise out of range", mutate: func(g *UpstreamGroup) { g.HealthCheck.Rise = 11 }, wantErr: "rise"},
		{name: "fall out of range", mutate: func(g *UpstreamGroup) { g.HealthCheck.Fall = -1 }, wantErr: "fall"},
		{name: "policy failover", mutate: func(g *UpstreamGroup) { g.Policy = PolicyFailover }},
		{name: "policy round-robin", mutate: func(g *UpstreamGroup) { g.Policy = PolicyRoundRobin }},
		{name: "policy least-connections", mutate: func(g *UpstreamGroup) { g.Policy = PolicyLeastConnections }},
		{name: "policy ip-hash", mutate: func(g *UpstreamGroup) { g.Policy = PolicyIPHash }},
		{name: "unknown policy", mutate: func(g *UpstreamGroup) { g.Policy = "random" }, wantErr: "policy"},
		{name: "weight ok", mutate: func(g *UpstreamGroup) { g.Upstreams[0].Weight = 5 }},
		{name: "weight out of range", mutate: func(g *UpstreamGroup) { g.Upstreams[0].Weight = 300 }, wantErr: "weight"},
		{name: "negative weight", mutate: func(g *UpstreamGroup) { g.Upstreams[0].Weight = -1 }, wantErr: "weight"},
		{name: "stickiness ok", mutate: func(g *UpstreamGroup) { g.Stickiness = &Stickiness{TTL: "12h"} }},
		{name: "stickiness day suffix", mutate: func(g *UpstreamGroup) { g.Stickiness = &Stickiness{TTL: "3d"} }},
		{name: "stickiness custom cookie", mutate: func(g *UpstreamGroup) { g.Stickiness = &Stickiness{TTL: "1h", Cookie: "my-affinity"} }},
		{name: "stickiness ttl missing", mutate: func(g *UpstreamGroup) { g.Stickiness = &Stickiness{} }, wantErr: "stickiness.ttl"},
		{name: "stickiness ttl invalid", mutate: func(g *UpstreamGroup) { g.Stickiness = &Stickiness{TTL: "soon"} }, wantErr: "stickiness.ttl"},
		{name: "stickiness ttl zero", mutate: func(g *UpstreamGroup) { g.Stickiness = &Stickiness{TTL: "0s"} }, wantErr: "must be > 0"},
		{name: "stickiness bad cookie name", mutate: func(g *UpstreamGroup) { g.Stickiness = &Stickiness{TTL: "1h", Cookie: "bad;name"} }, wantErr: "cookie name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := validGroup()
			tc.mutate(&g)
			err := g.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestStickinessHelpers(t *testing.T) {
	if d, err := (Stickiness{TTL: "3d"}).ParseTTL(); err != nil || d != 72*time.Hour {
		t.Fatalf("ParseTTL(3d) = %v, %v; want 72h", d, err)
	}
	if d, err := (Stickiness{TTL: "90m"}).ParseTTL(); err != nil || d != 90*time.Minute {
		t.Fatalf("ParseTTL(90m) = %v, %v; want 90m", d, err)
	}
	g := validGroup()
	if g.CookieName() != "gpm-sticky-edge-nodes" {
		t.Fatalf("default cookie name = %q", g.CookieName())
	}
	g.Stickiness = &Stickiness{TTL: "1h", Cookie: "aff"}
	if g.CookieName() != "aff" {
		t.Fatalf("explicit cookie name = %q", g.CookieName())
	}
}

func TestHealthCheckDefaults(t *testing.T) {
	var h HealthCheck
	if h.Interval() != DefaultHealthIntervalSeconds || h.Timeout() != DefaultHealthTimeoutSeconds ||
		h.RiseCount() != DefaultHealthRise || h.FallCount() != DefaultHealthFall {
		t.Fatalf("zero HealthCheck defaults = %d/%d/%d/%d", h.Interval(), h.Timeout(), h.RiseCount(), h.FallCount())
	}
	h = HealthCheck{IntervalSeconds: 10, TimeoutSeconds: 4, Rise: 3, Fall: 5}
	if h.Interval() != 10 || h.Timeout() != 4 || h.RiseCount() != 3 || h.FallCount() != 5 {
		t.Fatalf("explicit HealthCheck values not honoured")
	}
}

func TestProxyHostUpstreamGroupXOR(t *testing.T) {
	host := func() ProxyHost {
		return ProxyHost{
			ObjectMeta: ObjectMeta{Name: "app"},
			Domains:    []string{"app.example.com"},
		}
	}

	h := host()
	h.UpstreamGroupRef = "edge-nodes"
	if err := h.Validate(); err != nil {
		t.Fatalf("groupRef-only host: %v", err)
	}

	h = host()
	h.Upstream = Upstream{Scheme: "http", Host: "192.0.2.10", Port: 8080}
	if err := h.Validate(); err != nil {
		t.Fatalf("upstream-only host: %v", err)
	}

	h = host()
	h.Upstream = Upstream{Scheme: "http", Host: "192.0.2.10", Port: 8080}
	h.UpstreamGroupRef = "edge-nodes"
	if err := h.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("both set: err = %v, want mutually exclusive", err)
	}

	h = host() // neither set: upstream validation must still fire
	if err := h.Validate(); err == nil {
		t.Fatal("neither upstream nor upstreamGroupRef set: want error")
	}
}

func TestConfigValidateUpstreamGroupRefs(t *testing.T) {
	base := func() Config {
		return Config{
			UpstreamGroups: []UpstreamGroup{validGroup()},
			ProxyHosts: []ProxyHost{{
				ObjectMeta:       ObjectMeta{Name: "app"},
				Domains:          []string{"app.example.com"},
				UpstreamGroupRef: "edge-nodes",
			}},
		}
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}

	c := base()
	c.ProxyHosts[0].UpstreamGroupRef = "no-such-group"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "unknown upstreamGroup") {
		t.Fatalf("dangling ref: err = %v", err)
	}

	c = base()
	c.UpstreamGroups[0].Disabled = true
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled group ref: err = %v", err)
	}

	// A disabled host referencing a disabled group is fine (nothing compiles).
	c = base()
	c.UpstreamGroups[0].Disabled = true
	c.ProxyHosts[0].Disabled = true
	if err := c.Validate(); err != nil {
		t.Fatalf("disabled host + disabled group: %v", err)
	}

	c = base()
	c.UpstreamGroups = append(c.UpstreamGroups, validGroup())
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate upstreamGroup") {
		t.Fatalf("duplicate group name: err = %v", err)
	}
}

func TestLocationUpstreamGroupRef(t *testing.T) {
	base := func() Config {
		return Config{
			UpstreamGroups: []UpstreamGroup{validGroup()},
			ProxyHosts: []ProxyHost{{
				ObjectMeta: ObjectMeta{Name: "app"},
				Domains:    []string{"app.example.com"},
				Upstream:   Upstream{Scheme: "http", Host: "192.0.2.10", Port: 8080},
				Locations:  []Location{{Path: "/svc", UpstreamGroupRef: "edge-nodes"}},
			}},
		}
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("location group ref: %v", err)
	}

	c := base()
	c.ProxyHosts[0].Locations[0].UpstreamGroupRef = "ghost"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "unknown upstreamGroup") {
		t.Fatalf("dangling location ref: err = %v", err)
	}

	c = base()
	c.UpstreamGroups[0].Disabled = true
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("location ref to disabled group: err = %v", err)
	}

	c = base()
	c.ProxyHosts[0].Locations[0].Upstream = &Upstream{Scheme: "http", Host: "192.0.2.20", Port: 80}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("location with both upstream and groupRef: err = %v", err)
	}
}
