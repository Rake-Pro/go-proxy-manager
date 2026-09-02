package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// testConf builds a resolved dockerDiscovery block the way production does:
// through Settings, so the label prefix and the inherited profile set are
// exactly what the daemon would see.
func testConf(mutate func(*model.Settings)) model.DockerDiscoverySettings {
	s := model.Settings{
		DockerDiscovery: model.DockerDiscoverySettings{
			Enabled:               true,
			AllowedDomainSuffixes: []string{"example.com"},
			Template: model.IngressHostTemplate{
				TLS:         model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
				AccessLists: []string{"home-vpn"},
				Tags:        []string{"docker"},
			},
		},
	}
	if mutate != nil {
		mutate(&s)
	}
	return s.DockerDiscoveryResolved()
}

func container(name string, labels map[string]string, mutate func(*Container)) Container {
	c := Container{
		ID:     "0123456789abcdef",
		Names:  []string{"/" + name},
		State:  "running",
		Labels: labels,
	}
	c.NetworkSettings.Networks = map[string]struct {
		IPAddress string `json:"IPAddress"`
	}{"bridge": {IPAddress: "192.0.2.10"}}
	if mutate != nil {
		mutate(&c)
	}
	return c
}

func enabled(extra map[string]string) map[string]string {
	out := map[string]string{
		"gpm.rake.pro/enabled": "true",
		"gpm.rake.pro/domains": "app.example.com",
		"gpm.rake.pro/port":    "8080",
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func TestDerive(t *testing.T) {
	tests := []struct {
		name      string
		conf      model.DockerDiscoverySettings
		container Container
		wantName  string
		wantHost  string
		wantPort  int
		wantSchem string
		wantDoms  []string
		wantErr   string
	}{
		{
			name:      "container ip and labelled port",
			conf:      testConf(nil),
			container: container("grafana", enabled(nil), nil),
			wantName:  "dkr-grafana",
			wantHost:  "192.0.2.10",
			wantPort:  8080,
			wantSchem: "http",
			wantDoms:  []string{"app.example.com"},
		},
		{
			name: "single exposed port needs no label",
			conf: testConf(nil),
			container: container("radarr", map[string]string{
				"gpm.rake.pro/enabled": "true",
				"gpm.rake.pro/domains": "radarr.example.com",
			}, func(c *Container) {
				c.Ports = []Port{{PrivatePort: 7878, Type: "tcp"}}
			}),
			wantName: "dkr-radarr", wantHost: "192.0.2.10", wantPort: 7878, wantSchem: "http",
			wantDoms: []string{"radarr.example.com"},
		},
		{
			name: "two exposed ports and no label is a skip, never a guess",
			conf: testConf(nil),
			container: container("multi", map[string]string{
				"gpm.rake.pro/enabled": "true",
				"gpm.rake.pro/domains": "multi.example.com",
			}, func(c *Container) {
				c.Ports = []Port{{PrivatePort: 80, Type: "tcp"}, {PrivatePort: 8443, Type: "tcp"}}
			}),
			wantName: "dkr-multi",
			wantErr:  "exposes 2 TCP ports",
		},
		{
			name: "no port at all",
			conf: testConf(nil),
			container: container("bare", map[string]string{
				"gpm.rake.pro/enabled": "true",
				"gpm.rake.pro/domains": "bare.example.com",
			}, nil),
			wantName: "dkr-bare",
			wantErr:  "exposes no TCP port",
		},
		{
			name:      "https scheme label",
			conf:      testConf(nil),
			container: container("secure", enabled(map[string]string{"gpm.rake.pro/scheme": "https"}), nil),
			wantName:  "dkr-secure", wantHost: "192.0.2.10", wantPort: 8080, wantSchem: "https",
			wantDoms: []string{"app.example.com"},
		},
		{
			name:      "invalid scheme label",
			conf:      testConf(nil),
			container: container("weird", enabled(map[string]string{"gpm.rake.pro/scheme": "gopher"}), nil),
			wantName:  "dkr-weird",
			wantErr:   "must be http or https",
		},
		{
			name:      "missing domains label",
			conf:      testConf(nil),
			container: container("nodom", map[string]string{"gpm.rake.pro/enabled": "true"}, nil),
			wantName:  "dkr-nodom",
			wantErr:   "gpm.rake.pro/domains is required",
		},
		{
			name:      "domain outside the allowed suffixes",
			conf:      testConf(nil),
			container: container("evil", enabled(map[string]string{"gpm.rake.pro/domains": "app.evil.test"}), nil),
			wantName:  "dkr-evil",
			wantErr:   "outside allowedDomainSuffixes",
		},
		{
			name:      "wildcard domain is rejected",
			conf:      testConf(nil),
			container: container("wild", enabled(map[string]string{"gpm.rake.pro/domains": "*.example.com"}), nil),
			wantName:  "dkr-wild",
			wantErr:   "not a valid hostname",
		},
		{
			name:      "multiple domains are normalised, de-duplicated and sorted",
			conf:      testConf(nil),
			container: container("many", enabled(map[string]string{"gpm.rake.pro/domains": " B.example.com , a.example.com ,a.example.com"}), nil),
			wantName:  "dkr-many", wantHost: "192.0.2.10", wantPort: 8080, wantSchem: "http",
			wantDoms: []string{"a.example.com", "b.example.com"},
		},
		{
			name:      "unknown profile is never downgraded to the default",
			conf:      testConf(nil),
			container: container("app", enabled(map[string]string{"gpm.rake.pro/profile": "nope"}), nil),
			wantName:  "dkr-app",
			wantErr:   `profile "nope" is not defined`,
		},
		{
			name: "published port mode uses the host address",
			conf: testConf(func(s *model.Settings) {
				s.DockerDiscovery.UsePublishedPorts = true
				s.DockerDiscovery.PublishedHost = "198.51.100.7"
			}),
			container: container("pub", enabled(nil), func(c *Container) {
				c.Ports = []Port{{PrivatePort: 8080, PublicPort: 18080, Type: "tcp"}}
			}),
			wantName: "dkr-pub", wantHost: "198.51.100.7", wantPort: 18080, wantSchem: "http",
			wantDoms: []string{"app.example.com"},
		},
		{
			name: "published port mode with an unpublished port",
			conf: testConf(func(s *model.Settings) {
				s.DockerDiscovery.UsePublishedPorts = true
			}),
			container: container("unpub", enabled(nil), nil),
			wantName:  "dkr-unpub",
			wantErr:   "is not published to the host",
		},
		{
			name: "named network the container is not on",
			conf: testConf(func(s *model.Settings) {
				s.DockerDiscovery.Network = "edge"
			}),
			container: container("offnet", enabled(nil), nil),
			wantName:  "dkr-offnet",
			wantErr:   `not attached to the configured network "edge"`,
		},
		{
			name: "host networking has no per-container address",
			conf: testConf(nil),
			container: container("hostnet", enabled(nil), func(c *Container) {
				c.NetworkSettings.Networks = map[string]struct {
					IPAddress string `json:"IPAddress"`
				}{"host": {}}
			}),
			wantName: "dkr-hostnet",
			wantErr:  "no usable IP address",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, host, _, err := derive(tc.container, tc.conf)
			if name != tc.wantName {
				t.Fatalf("derived name %q, want %q", name, tc.wantName)
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("derive: %v", err)
			}
			if host.Upstream.Host != tc.wantHost || host.Upstream.Port != tc.wantPort || host.Upstream.Scheme != tc.wantSchem {
				t.Fatalf("upstream %+v, want %s://%s:%d", host.Upstream, tc.wantSchem, tc.wantHost, tc.wantPort)
			}
			if strings.Join(host.Domains, ",") != strings.Join(tc.wantDoms, ",") {
				t.Fatalf("domains %v, want %v", host.Domains, tc.wantDoms)
			}
			if host.Labels["gpm.rake.pro/managed-by"] != model.ManagedByDockerDiscovery {
				t.Fatalf("labels %v, want the docker managed-by marker", host.Labels)
			}
			if host.TLS.CertificateRef != "wildcard" || strings.Join(host.AccessLists, ",") != "home-vpn" {
				t.Fatalf("chain %+v/%v, want the template's verbatim", host.TLS, host.AccessLists)
			}
		})
	}
}

// The derived host takes its DNS policy from the profile, with each flag
// overridable by its own label.
func TestDeriveDNSPolicy(t *testing.T) {
	conf := testConf(func(s *model.Settings) {
		s.DockerDiscovery.Template.DefaultDNS = &model.DNSSyncPolicy{LanDirect: true}
	})
	tests := []struct {
		name            string
		labels          map[string]string
		wantNil         bool
		lan, publicName bool
	}{
		{"template default", enabled(nil), false, true, false},
		{"label turns lanDirect off", enabled(map[string]string{"gpm.rake.pro/lan-direct": "false"}), true, false, false},
		{"label adds the public cname", enabled(map[string]string{"gpm.rake.pro/public-cname": "true"}), false, true, true},
		{"junk value keeps the default", enabled(map[string]string{"gpm.rake.pro/lan-direct": "yes-please"}), false, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, host, _, err := derive(container("app", tc.labels, nil), conf)
			if err != nil {
				t.Fatalf("derive: %v", err)
			}
			if tc.wantNil {
				if host.DNS != nil {
					t.Fatalf("dns %+v, want nil for a policy that asks for nothing", host.DNS)
				}
				return
			}
			if host.DNS == nil || host.DNS.LanDirect != tc.lan || host.DNS.PublicCname != tc.publicName {
				t.Fatalf("dns %+v, want lan=%v public=%v", host.DNS, tc.lan, tc.publicName)
			}
		})
	}
}

func managedHostFixture(name string, mutate func(*model.ProxyHost)) model.ProxyHost {
	h := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{
			Name:        name,
			DisplayName: strings.TrimPrefix(name, model.DockerHostNamePrefix),
			Labels:      map[string]string{"gpm.rake.pro/managed-by": model.ManagedByDockerDiscovery},
			Tags:        []string{"docker"},
		},
		Domains:     []string{"app.example.com"},
		Upstream:    model.Upstream{Scheme: "http", Host: "192.0.2.10", Port: 8080},
		TLS:         model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
		AccessLists: []string{"home-vpn"},
	}
	if mutate != nil {
		mutate(&h)
	}
	return h
}

func TestPlanReconcile(t *testing.T) {
	conf := testConf(nil)

	t.Run("creates a host for a new container", func(t *testing.T) {
		p := planReconcile(model.Config{}, conf, []Container{container("grafana", enabled(nil), nil)})
		if p.Created != 1 || len(p.Upserts) != 1 {
			t.Fatalf("plan %+v, want one create", p)
		}
		if p.Upserts[0].Name != "dkr-grafana" {
			t.Fatalf("created %q, want dkr-grafana", p.Upserts[0].Name)
		}
	})

	t.Run("steady state writes nothing", func(t *testing.T) {
		cur := managedHostFixture("dkr-app", nil)
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{cur}}, conf, []Container{container("app", enabled(nil), nil)})
		if len(p.Upserts) != 0 || len(p.Deletes) != 0 {
			t.Fatalf("plan %+v, want no writes", p)
		}
		if p.Hosts[0].Action != ActionUnchanged {
			t.Fatalf("action %q, want unchanged", p.Hosts[0].Action)
		}
	})

	t.Run("never touches a hand-written host with the same name", func(t *testing.T) {
		// A different domain, so the plan reaches the NAME-ownership branch rather
		// than being stopped earlier by the domain gate (tested separately below).
		operator := model.ProxyHost{
			ObjectMeta: model.ObjectMeta{Name: "dkr-app"},
			Domains:    []string{"hand-written.example.com"},
			Upstream:   model.Upstream{Scheme: "http", Host: "203.0.113.9", Port: 80},
		}
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{operator}}, conf, []Container{container("app", enabled(nil), nil)})
		if len(p.Upserts) != 0 || len(p.Deletes) != 0 || p.Skipped != 1 {
			t.Fatalf("plan %+v, want a single skip and no writes", p)
		}
		if !strings.Contains(p.Hosts[0].Reason, "not managed by docker discovery") {
			t.Fatalf("reason %q", p.Hosts[0].Reason)
		}
	})

	// The two reconcilers share the managed-by KEY and differ only in its value.
	// Neither may adopt, rewrite or delete the other's hosts.
	t.Run("never touches an ingress-discovery host", func(t *testing.T) {
		ing := model.ProxyHost{
			ObjectMeta: model.ObjectMeta{
				Name:   "dkr-app",
				Labels: map[string]string{"gpm.rake.pro/managed-by": model.ManagedByIngressDiscovery},
			},
			Domains:  []string{"cluster.example.com"},
			Upstream: model.Upstream{Scheme: "http", Host: "203.0.113.9", Port: 80},
		}
		other := model.ProxyHost{
			ObjectMeta: model.ObjectMeta{
				Name:   "ing-other.apps",
				Labels: map[string]string{"gpm.rake.pro/managed-by": model.ManagedByIngressDiscovery},
			},
			Domains:  []string{"other.example.com"},
			Upstream: model.Upstream{Scheme: "http", Host: "203.0.113.9", Port: 80},
		}
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{ing, other}}, conf,
			[]Container{container("app", enabled(nil), nil)})
		if len(p.Upserts) != 0 || len(p.Deletes) != 0 {
			t.Fatalf("plan %+v, want no writes at all", p)
		}
		if p.ManagedAfter != 0 {
			t.Fatalf("managedAfter %d, want 0 - neither host belongs to docker discovery", p.ManagedAfter)
		}
	})

	t.Run("deletes its own orphan", func(t *testing.T) {
		orphan := managedHostFixture("dkr-gone", nil)
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{orphan}}, conf, nil)
		if len(p.Deletes) != 1 || p.Deletes[0] != "dkr-gone" {
			t.Fatalf("deletes %v, want [dkr-gone]", p.Deletes)
		}
	})

	t.Run("a derive failure freezes rather than deletes", func(t *testing.T) {
		cur := managedHostFixture("dkr-app", nil)
		broken := container("app", map[string]string{
			"gpm.rake.pro/enabled": "true",
			"gpm.rake.pro/domains": "not a hostname",
		}, nil)
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{cur}}, conf, []Container{broken})
		if len(p.Deletes) != 0 || len(p.Upserts) != 0 {
			t.Fatalf("plan %+v, want the stored host left exactly as it is", p)
		}
		if p.Skipped != 1 {
			t.Fatalf("skipped %d, want 1", p.Skipped)
		}
	})

	t.Run("an unresolvable profile disables the existing host", func(t *testing.T) {
		cur := managedHostFixture("dkr-app", nil)
		ct := container("app", enabled(map[string]string{"gpm.rake.pro/profile": "retired"}), nil)
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{cur}}, conf, []Container{ct})
		if len(p.Upserts) != 1 || !p.Upserts[0].Disabled {
			t.Fatalf("plan %+v, want one disabling upsert", p)
		}
		if p.Upserts[0].Labels["gpm.rake.pro/disabled-by"] != model.DisabledByDockerDiscovery {
			t.Fatalf("labels %v, want the docker disabled-by marker", p.Upserts[0].Labels)
		}
		// The stored object the caller still holds must not have been mutated.
		if cur.Disabled || len(cur.Labels) != 1 {
			t.Fatalf("the input host was mutated: %+v", cur)
		}
	})

	t.Run("operator state is carried forward", func(t *testing.T) {
		cur := managedHostFixture("dkr-app", func(h *model.ProxyHost) {
			h.Maintenance = true
			h.Disabled = true
			h.Upstream.Port = 9999 // stale, so the plan produces an update
		})
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{cur}}, conf, []Container{container("app", enabled(nil), nil)})
		if len(p.Upserts) != 1 {
			t.Fatalf("plan %+v, want one update", p)
		}
		got := p.Upserts[0]
		if !got.Maintenance || !got.Disabled {
			t.Fatalf("host %+v, want maintenance and the operator's disable preserved", got)
		}
		if got.Upstream.Port != 8080 {
			t.Fatalf("upstream port %d, want the freshly derived 8080", got.Upstream.Port)
		}
	})

	t.Run("a domain owned by another host is never shadowed", func(t *testing.T) {
		operator := model.ProxyHost{
			ObjectMeta: model.ObjectMeta{Name: "sso"},
			Domains:    []string{"App.example.com"},
			Upstream:   model.Upstream{Scheme: "http", Host: "203.0.113.9", Port: 80},
		}
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{operator}}, conf, []Container{container("app", enabled(nil), nil)})
		if len(p.Upserts) != 0 || p.Skipped != 1 {
			t.Fatalf("plan %+v, want a skip", p)
		}
		if !strings.Contains(p.Hosts[0].Reason, `already claimed by proxy host "sso"`) {
			t.Fatalf("reason %q", p.Hosts[0].Reason)
		}
	})

	t.Run("two containers deriving one name", func(t *testing.T) {
		a := container("app", enabled(nil), nil)
		b := container("APP", enabled(map[string]string{"gpm.rake.pro/domains": "other.example.com"}), nil)
		p := planReconcile(model.Config{}, conf, []Container{a, b})
		if p.Created != 1 || p.Skipped != 1 {
			t.Fatalf("plan %+v, want one create and one skip", p)
		}
	})

	// A container the Engine returned without the opt-in label (a proxy that
	// ignores filters) must stay invisible.
	t.Run("unlabelled containers are invisible", func(t *testing.T) {
		p := planReconcile(model.Config{}, conf, []Container{container("app", map[string]string{"gpm.rake.pro/domains": "app.example.com"}, nil)})
		if p.Discovered != 0 || len(p.Upserts) != 0 {
			t.Fatalf("plan %+v, want nothing discovered", p)
		}
	})
}

// engine serves the two endpoints a reconcile needs.
func engine(t *testing.T, containers []Container, status int) *Client {
	t.Helper()
	c, _ := newSocketServer(t, versionMux("1.51", "1.24", func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(containers)
	}))
	return c
}

func newTestDiscoverer(t *testing.T, cfg model.Config, settings model.Settings, client *Client) (*Discoverer, *[]string) {
	t.Helper()
	var commits []string
	d := New(
		func(context.Context) (model.Config, model.Settings, error) { return cfg, settings, nil },
		func(_ context.Context, upserts []model.ProxyHost, deletes []string, msg string) (string, error) {
			commits = append(commits, msg)
			return "abc123", nil
		},
		nil,
	)
	d.newClient = func(ClientConfig) (*Client, error) { return client, nil }
	return d, &commits
}

func enabledSettings(mutate func(*model.Settings)) model.Settings {
	s := model.Settings{
		DockerDiscovery: model.DockerDiscoverySettings{
			Enabled:               true,
			AllowedDomainSuffixes: []string{"example.com"},
			Template: model.IngressHostTemplate{
				TLS:         model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
				AccessLists: []string{"home-vpn"},
				Tags:        []string{"docker"},
			},
		},
	}
	if mutate != nil {
		mutate(&s)
	}
	return s
}

func TestReconcileWritesOneCommit(t *testing.T) {
	client := engine(t, []Container{container("grafana", enabled(nil), nil)}, 0)
	d, commits := newTestDiscoverer(t, model.Config{}, enabledSettings(nil), client)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(*commits) != 1 || !strings.HasPrefix((*commits)[0], "Docker discovery: reconcile (+1 ~0 -0)") {
		t.Fatalf("commits %v, want one create commit", *commits)
	}
	st := d.Status()
	if !st.Enabled || st.Discovered != 1 || st.Created != 1 || st.Commit != "abc123" {
		t.Fatalf("status %+v", st)
	}
	if !st.Reachable || st.Endpoint == "" {
		t.Fatalf("status %+v, want a reachable endpoint recorded", st)
	}
	if st.LastSuccess.IsZero() {
		t.Fatal("lastSuccess is zero after a successful run")
	}
}

// The freeze rule: a list that fails must leave every managed host alone and
// must not advance lastSuccess.
func TestReconcileFreezesOnListFailure(t *testing.T) {
	client := engine(t, nil, http.StatusInternalServerError)
	cur := managedHostFixture("dkr-app", nil)
	d, commits := newTestDiscoverer(t, model.Config{ProxyHosts: []model.ProxyHost{cur}}, enabledSettings(nil), client)

	err := d.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile succeeded against a failing endpoint")
	}
	if len(*commits) != 0 {
		t.Fatalf("commits %v, want none: a failed list must not write", *commits)
	}
	st := d.Status()
	if st.Error == "" || !st.LastSuccess.IsZero() || st.Reachable {
		t.Fatalf("status %+v, want an error with no success and reachable=false", st)
	}
}

func TestReconcileDisabledIsInert(t *testing.T) {
	client := engine(t, []Container{container("grafana", enabled(nil), nil)}, 0)
	settings := enabledSettings(func(s *model.Settings) { s.DockerDiscovery.Enabled = false })
	d, commits := newTestDiscoverer(t, model.Config{}, settings, client)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(*commits) != 0 {
		t.Fatalf("commits %v, want none while disabled", *commits)
	}
	if st := d.Status(); st.Enabled {
		t.Fatalf("status %+v, want enabled=false", st)
	}
	if d.Enabled() {
		t.Fatal("Enabled() true while dockerDiscovery.enabled is false")
	}
}

func TestPlanWritesNothing(t *testing.T) {
	client := engine(t, []Container{container("grafana", enabled(nil), nil)}, 0)
	d, commits := newTestDiscoverer(t, model.Config{}, enabledSettings(nil), client)

	p, err := d.Plan(context.Background())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !p.Enabled || p.Created != 1 || len(p.Hosts) != 1 || p.Hosts[0].Container != "grafana" {
		t.Fatalf("plan %+v", p)
	}
	if len(*commits) != 0 {
		t.Fatalf("commits %v, want none from a dry run", *commits)
	}
	if st := d.Status(); !st.LastRun.IsZero() {
		t.Fatalf("status %+v, want a plan not to record a run", st)
	}
}

func TestReconcileRefusesWhileRunning(t *testing.T) {
	d := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, enabledSettings(nil), nil
	}, nil, nil)
	d.single.Lock()
	defer d.single.Unlock()

	if err := d.ReconcileNow(context.Background()); err != ErrReconcileInProgress {
		t.Fatalf("ReconcileNow error %v, want ErrReconcileInProgress", err)
	}
	if _, err := d.Plan(context.Background()); err != ErrReconcileInProgress {
		t.Fatalf("Plan error %v, want ErrReconcileInProgress", err)
	}
}
