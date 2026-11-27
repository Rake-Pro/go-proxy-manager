package docker

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/discovery"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

func tps(p *[]string) string {
	if p == nil {
		return "<unset>"
	}
	return "[" + strings.Join(*p, ",") + "]"
}

// The container derive stamps trustedProxies from the default template and
// from a profile when they set it (an empty list included), leaves it unset
// when they do not, and never shares the settings' backing array.
func TestDockerDeriveStampsTemplateTrustedProxies(t *testing.T) {
	list := func(v ...string) *[]string { s := append([]string{}, v...); return &s }
	for _, tc := range []struct {
		name string
		tmpl *[]string
	}{{"unset", nil}, {"empty", list()}, {"list", list("192.0.2.10")}} {
		t.Run(tc.name, func(t *testing.T) {
			conf := testConf(func(s *model.Settings) {
				s.DockerDiscovery.Template.TrustedProxies = tc.tmpl
				prof := s.DockerDiscovery.Template
				prof.TrustedProxies = list("198.51.100.0/24")
				s.DockerDiscovery.Profiles = map[string]model.IngressHostTemplate{"edge": prof}
			})
			p := planReconcile(model.Config{}, conf, []Container{
				container("a", enabled(map[string]string{"gpm.rake.pro/domains": "a.example.com"}), nil),
				container("b", enabled(map[string]string{"gpm.rake.pro/domains": "b.example.com", "gpm.rake.pro/profile": "edge"}), nil),
			}, nil, nil)
			if len(p.Upserts) != 2 {
				t.Fatalf("upserts %+v, want two creates", p.Upserts)
			}
			byName := map[string]model.ProxyHost{}
			for _, u := range p.Upserts {
				byName[u.Name] = u
			}
			a, b := byName["dkr-a"], byName["dkr-b"]
			if tps(a.TrustedProxies) != tps(tc.tmpl) {
				t.Fatalf("default template host trustedProxies %s, want %s", tps(a.TrustedProxies), tps(tc.tmpl))
			}
			if tc.tmpl != nil && *a.TrustedProxies == nil {
				t.Fatal("an empty template list was stamped as a nil slice")
			}
			if tps(b.TrustedProxies) != "[198.51.100.0/24]" {
				t.Fatalf("profile host trustedProxies %s, want the profile's", tps(b.TrustedProxies))
			}
			(*b.TrustedProxies)[0] = "mutated"
			if (*conf.Profiles["edge"].TrustedProxies)[0] != "198.51.100.0/24" {
				t.Fatal("a derived host shares the profile's trustedProxies backing array")
			}
		})
	}
}

// An operator's trustedProxies on a container-derived host survives while the
// template leaves the field unset, including across the domain-conflict
// restamp, and the template's value wins once it sets it.
func TestDockerTrustedProxiesOwnership(t *testing.T) {
	conf := testConf(nil)
	own := []string{"192.0.2.10"}
	cur := managedHostFixture("dkr-app", func(h *model.ProxyHost) { h.TrustedProxies = &own })
	ct := container("app", enabled(nil), nil)
	if p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{cur}}, conf, []Container{ct}, nil, nil); len(p.Upserts) != 0 {
		t.Fatalf("upserts %+v, want the operator's value kept with no write", p.Upserts)
	}

	// The restamp after a domain conflict (here forced by a new container
	// address) keeps it too.
	operator := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "operator-www"},
		Domains:    []string{"www.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "192.0.2.41", Port: 80},
	}
	moved := container("app", enabled(map[string]string{"gpm.rake.pro/domains": "app.example.com,www.example.com"}),
		withNetworks(map[string]string{"bridge": "192.0.2.30"}))
	p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{cur, operator}}, conf, []Container{moved}, nil, nil)
	if len(p.Upserts) != 1 || tps(p.Upserts[0].TrustedProxies) != "[192.0.2.10]" {
		t.Fatalf("restamp %+v, want the operator's trustedProxies kept", p.Upserts)
	}

	// Set on the template: stamped, on the plain update and on the restamp.
	empty := []string{}
	conf = testConf(func(s *model.Settings) { s.DockerDiscovery.Template.TrustedProxies = &empty })
	if p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{cur}}, conf, []Container{ct}, nil, nil); len(p.Upserts) != 1 || tps(p.Upserts[0].TrustedProxies) != "[]" {
		t.Fatalf("upserts %+v, want the template's empty list stamped", p.Upserts)
	}
	// Same container address, so only the template's trustedProxies makes the
	// stored chain stale while the conflict lasts.
	wide := container("app", enabled(map[string]string{"gpm.rake.pro/domains": "app.example.com,www.example.com"}), nil)
	p = planReconcile(model.Config{ProxyHosts: []model.ProxyHost{cur, operator}}, conf, []Container{wide}, nil, nil)
	if len(p.Upserts) != 1 || tps(p.Upserts[0].TrustedProxies) != "[]" || p.Upserts[0].Disabled {
		t.Fatalf("restamp %+v, want the template's empty list stamped", p.Upserts)
	}
}

// Container-derived hosts written before trustedProxies and the tag rule
// existed, read back from the YAML the store wrote, come out unchanged on
// every pass, for the default template and a profile.
func TestDockerLegacyHostsAreUnchanged(t *testing.T) {
	tests := []struct {
		name        string
		tmplTags    []string
		storedEmpty bool
	}{
		{"no tags", nil, false},
		{"stored explicit empty tags", nil, true},
		{"template explicit empty tags", []string{}, true},
		{"template tags", []string{"docker"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conf := testConf(func(s *model.Settings) {
				s.DockerDiscovery.Template.Tags = tc.tmplTags
				prof := s.DockerDiscovery.Template
				if tc.tmplTags != nil {
					prof.Tags = append([]string{"edge"}, tc.tmplTags...)
				}
				s.DockerDiscovery.Profiles = map[string]model.IngressHostTemplate{"edge": prof}
			})
			cts := []Container{
				container("a", enabled(map[string]string{"gpm.rake.pro/domains": "a.example.com"}), nil),
				container("b", enabled(map[string]string{"gpm.rake.pro/domains": "b.example.com", "gpm.rake.pro/profile": "edge"}), nil),
			}
			var cfg model.Config
			for _, h := range planReconcile(model.Config{}, conf, cts, nil, nil).Upserts {
				b, err := yaml.Marshal(h)
				if err != nil {
					t.Fatalf("yaml.Marshal: %v", err)
				}
				if strings.Contains(string(b), "trustedProxies") || strings.Contains(string(b), "trusted-proxies-by") {
					t.Fatalf("a template without trustedProxies wrote the key or its marker:\n%s", b)
				}
				doc := string(b)
				if tc.storedEmpty && !strings.Contains(doc, "tags:") {
					doc = "tags: []\n" + doc
				}
				var stored model.ProxyHost
				if err := yaml.Unmarshal([]byte(doc), &stored); err != nil {
					t.Fatalf("yaml.Unmarshal: %v", err)
				}
				cfg.ProxyHosts = append(cfg.ProxyHosts, stored)
			}
			if len(cfg.ProxyHosts) != 2 {
				t.Fatalf("stored %d hosts, want 2", len(cfg.ProxyHosts))
			}
			var mem discovery.StampedTags
			for pass := range 3 {
				p := planReconcile(cfg, conf, cts, nil, mem)
				if len(p.Upserts) != 0 || len(p.Deletes) != 0 || p.Updated != 0 {
					t.Fatalf("pass %d: upserts %+v deletes %v, want none", pass, p.Upserts, p.Deletes)
				}
				mem = p.Stamped
			}
		})
	}
}

// The Docker Discoverer keeps its own tag memory between passes, advanced
// only by an applied plan.
func TestDockerDiscovererRemembersStampedTags(t *testing.T) {
	client := engine(t, []Container{container("app", enabled(nil), nil)}, 0)
	cfg := model.Config{}
	settings := enabledSettings(func(s *model.Settings) { s.DockerDiscovery.Template.Tags = []string{"a", "b"} })
	var writes int
	var failWrite bool
	d := New(
		func(context.Context) (model.Config, model.Settings, error) { return cfg, settings, nil },
		func(_ context.Context, ups []model.ProxyHost, _ []string, _ string) (string, error) {
			if failWrite {
				return "", errors.New("write failed")
			}
			writes++
			for _, u := range ups {
				replaced := false
				for i := range cfg.ProxyHosts {
					if cfg.ProxyHosts[i].Name == u.Name {
						cfg.ProxyHosts[i], replaced = u, true
					}
				}
				if !replaced {
					cfg.ProxyHosts = append(cfg.ProxyHosts, u)
				}
			}
			return "commit", nil
		},
		nil,
	)
	d.newClient = func(ClientConfig) (*Client, error) { return client, nil }
	ctx := context.Background()
	tags := func() []string { return cfg.ProxyHosts[0].Tags }

	if err := d.Reconcile(ctx); err != nil || writes != 1 || !slices.Equal(tags(), []string{"a", "b"}) {
		t.Fatalf("create: err=%v writes=%d tags=%v", err, writes, tags())
	}
	cfg.ProxyHosts[0].Tags = []string{"a", "b", "x"}
	if err := d.Reconcile(ctx); err != nil || writes != 1 {
		t.Fatalf("hand-added tag: err=%v writes=%d, want no write", err, writes)
	}
	settings.DockerDiscovery.Template.Tags = []string{"a"}
	failWrite = true
	if err := d.Reconcile(ctx); err == nil {
		t.Fatal("reconcile succeeded with a failing writer")
	}
	failWrite = false
	if err := d.Reconcile(ctx); err != nil || writes != 2 || !slices.Equal(tags(), []string{"a", "x"}) {
		t.Fatalf("drop b: err=%v writes=%d tags=%v, want [a x]", err, writes, tags())
	}
	settings.DockerDiscovery.Template.Tags = []string{"c"}
	if pl, err := d.Plan(ctx); err != nil || pl.Updated != 1 {
		t.Fatalf("preview: %+v, %v", pl, err)
	}
	if err := d.Reconcile(ctx); err != nil || !slices.Equal(tags(), []string{"c", "x"}) {
		t.Fatalf("after preview: err=%v tags=%v, want [c x]", err, tags())
	}
}

func applyDockerHosts(cfg model.Config, ups []model.ProxyHost, dels []string) model.Config {
	out := model.Config{}
	gone := map[string]bool{}
	for _, d := range dels {
		gone[d] = true
	}
	done := map[string]bool{}
	for _, h := range cfg.ProxyHosts {
		for _, u := range ups {
			if u.Name == h.Name {
				h, done[u.Name] = u, true
			}
		}
		if !gone[h.Name] {
			out.ProxyHosts = append(out.ProxyHosts, h)
		}
	}
	for _, u := range ups {
		if !done[u.Name] {
			out.ProxyHosts = append(out.ProxyHosts, u)
		}
	}
	return out
}

// settleDocker applies one plan, asserts that three more passes over the
// previous output write nothing, and returns the settled config and memory.
func settleDocker(t *testing.T, cfg model.Config, conf model.DockerDiscoverySettings, cts []Container, mem discovery.StampedTags) (model.Config, discovery.StampedTags) {
	t.Helper()
	first := planReconcile(cfg, conf, cts, nil, mem)
	cfg, mem = applyDockerHosts(cfg, first.Upserts, first.Deletes), first.Stamped
	for pass := range 3 {
		p := planReconcile(cfg, conf, cts, nil, mem)
		if len(p.Upserts) != 0 || len(p.Deletes) != 0 {
			t.Fatalf("pass %d after the write: upserts %+v deletes %v, want none", pass+1, p.Upserts, p.Deletes)
		}
		cfg, mem = applyDockerHosts(cfg, p.Upserts, p.Deletes), p.Stamped
	}
	return cfg, mem
}

// A trustedProxies value a profile stamped does not outlive the profile: a
// container moved by label from a profile that sets it to one that does not
// loses the value and the marker, and so does a host whose template drops the
// key. Setting it again stamps and marks it again.
func TestDockerStampedTrustedProxiesFollowTheProfile(t *testing.T) {
	wide := []string{"0.0.0.0/0"}
	marker := model.TrustedProxiesByLabel
	conf := testConf(func(s *model.Settings) {
		lb := s.DockerDiscovery.Template
		lb.TrustedProxies = &wide
		s.DockerDiscovery.Profiles = map[string]model.IngressHostTemplate{"lb": lb, "strict": s.DockerDiscovery.Template}
	})
	onLB := container("app", enabled(map[string]string{"gpm.rake.pro/profile": "lb"}), nil)
	cfg, mem := settleDocker(t, model.Config{}, conf, []Container{onLB}, nil)
	if h := cfg.ProxyHosts[0]; tps(h.TrustedProxies) != "[0.0.0.0/0]" || h.Labels[marker] != model.ManagedByDockerDiscovery {
		t.Fatalf("on lb: %+v, want the profile's value and the marker", h)
	}
	onStrict := container("app", enabled(map[string]string{"gpm.rake.pro/profile": "strict"}), nil)
	cfg, _ = settleDocker(t, cfg, conf, []Container{onStrict}, mem)
	if h := cfg.ProxyHosts[0]; h.TrustedProxies != nil || h.Labels[marker] != "" {
		t.Fatalf("on strict: %+v, want no trustedProxies and no marker", h)
	}

	withKey := testConf(func(s *model.Settings) { s.DockerDiscovery.Template.TrustedProxies = &wide })
	plain := container("app", enabled(nil), nil)
	cfg, mem = settleDocker(t, model.Config{}, withKey, []Container{plain}, nil)
	cfg, mem = settleDocker(t, cfg, testConf(nil), []Container{plain}, mem)
	if h := cfg.ProxyHosts[0]; h.TrustedProxies != nil || h.Labels[marker] != "" {
		t.Fatalf("after removal: %+v, want no trustedProxies and no marker", h)
	}
	cfg, _ = settleDocker(t, cfg, withKey, []Container{plain}, mem)
	if h := cfg.ProxyHosts[0]; tps(h.TrustedProxies) != "[0.0.0.0/0]" || h.Labels[marker] == "" {
		t.Fatalf("set again: %+v, want the value and the marker", h)
	}
}

// During a prefix migration a container-derived host still marked under the
// old prefix is recognised as stamped and cleared under an unset template,
// with only the new prefix's managed-by left.
func TestDockerPrefixMigrationCarriesTheMarker(t *testing.T) {
	wide := []string{"0.0.0.0/0"}
	stale := managedHostFixture("dkr-app", func(h *model.ProxyHost) {
		h.Labels[model.TrustedProxiesByLabel] = model.ManagedByDockerDiscovery
		h.TrustedProxies = &wide
	})
	conf := testConf(func(s *model.Settings) {
		s.IngressDiscovery.AnnotationPrefix = "acme.example.com"
		s.IngressDiscovery.AnnotationPrefixMigrate = true
	})
	ct := container("app", map[string]string{
		"acme.example.com/enabled": "true",
		"acme.example.com/domains": "app.example.com",
		"acme.example.com/port":    "8080",
	}, nil)
	cfg, _ := settleDocker(t, model.Config{ProxyHosts: []model.ProxyHost{stale}}, conf, []Container{ct}, nil)
	h := cfg.ProxyHosts[0]
	if h.TrustedProxies != nil || len(h.Labels) != 1 || h.Labels["acme.example.com/managed-by"] != model.ManagedByDockerDiscovery {
		t.Fatalf("host %+v, want no trustedProxies and only the new managed-by", h)
	}

	// A fail-closed disable relabels onto the new prefix and keeps the stored
	// value with its marker, without the old-prefix copy.
	revoked := container("app", map[string]string{
		"acme.example.com/enabled": "true",
		"acme.example.com/domains": "app.example.com",
		"acme.example.com/port":    "8080",
		"acme.example.com/profile": "nope",
	}, nil)
	p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{stale}}, conf, []Container{revoked}, nil, nil)
	if len(p.Upserts) != 1 || !p.Upserts[0].Disabled {
		t.Fatalf("upserts %+v, want one fail-closed disable", p.Upserts)
	}
	want := map[string]string{
		"acme.example.com/managed-by":         model.ManagedByDockerDiscovery,
		"acme.example.com/disabled-by":        model.DisabledByDockerDiscovery,
		"acme.example.com/trusted-proxies-by": model.ManagedByDockerDiscovery,
	}
	if got := p.Upserts[0]; tps(got.TrustedProxies) != "[0.0.0.0/0]" || !maps.Equal(got.Labels, want) {
		t.Fatalf("disabled host %+v, want the stored value and only the new-prefix labels", got)
	}
}

type dockerParity struct {
	cfg      model.Config
	settings model.Settings
	writes   int
	empty    bool
}

func (h *dockerParity) discoverer(client *Client) *Discoverer {
	d := New(
		func(context.Context) (model.Config, model.Settings, error) { return h.cfg, h.settings, nil },
		func(_ context.Context, ups []model.ProxyHost, dels []string, _ string) (string, error) {
			if h.empty {
				return "", nil
			}
			h.writes++
			h.cfg = applyDockerHosts(h.cfg, ups, dels)
			return "commit", nil
		},
		nil,
	)
	d.newClient = func(ClientConfig) (*Client, error) { return client, nil }
	return d
}

// An empty commit keeps the previous memory for the hosts the plan wanted to
// write, and disabling the block forgets the memory.
func TestDockerMemoryOnEmptyCommitAndDisable(t *testing.T) {
	ctx := context.Background()
	client := engine(t, []Container{container("app", enabled(nil), nil)}, 0)

	h := &dockerParity{settings: enabledSettings(func(s *model.Settings) { s.DockerDiscovery.Template.Tags = []string{"a", "b"} })}
	d := h.discoverer(client)
	if err := d.Reconcile(ctx); err != nil || h.writes != 1 {
		t.Fatalf("create: %v writes=%d", err, h.writes)
	}
	h.settings.DockerDiscovery.Template.Tags = []string{"a"}
	h.empty = true
	if err := d.Reconcile(ctx); err != nil {
		t.Fatalf("empty commit: %v", err)
	}
	h.empty = false
	if err := d.Reconcile(ctx); err != nil || h.writes != 2 || !slices.Equal(h.cfg.ProxyHosts[0].Tags, []string{"a"}) {
		t.Fatalf("after the empty commit: err=%v writes=%d tags=%v, want b dropped", err, h.writes, h.cfg.ProxyHosts[0].Tags)
	}

	h = &dockerParity{settings: enabledSettings(func(s *model.Settings) { s.DockerDiscovery.Template.Tags = []string{"a", "b"} })}
	d = h.discoverer(client)
	if err := d.Reconcile(ctx); err != nil || h.writes != 1 {
		t.Fatalf("create: %v writes=%d", err, h.writes)
	}
	h.settings.DockerDiscovery.Enabled = false
	if err := d.Reconcile(ctx); err != nil {
		t.Fatalf("disabled pass: %v", err)
	}
	h.settings.DockerDiscovery.Enabled = true
	h.settings.DockerDiscovery.Template.Tags = []string{"a"}
	if err := d.Reconcile(ctx); err != nil || h.writes != 1 || !slices.Equal(h.cfg.ProxyHosts[0].Tags, []string{"a", "b"}) {
		t.Fatalf("re-enabled: err=%v writes=%d tags=%v, want b kept and no write", err, h.writes, h.cfg.ProxyHosts[0].Tags)
	}
}

// A host moved by hand from Ingress discovery to Docker discovery (managed-by
// relabelled, the Ingress reconciler's marker left behind): the value is
// still read as stamped, so an unset template clears it and a set one
// restamps it with this reconciler's marker. One write, then nothing.
func TestDockerOtherReconcilersMarker(t *testing.T) {
	wide := []string{"0.0.0.0/0"}
	stored := managedHostFixture("dkr-app", func(h *model.ProxyHost) {
		h.Labels[model.TrustedProxiesByLabel] = model.ManagedByIngressDiscovery
		h.TrustedProxies = &wide
	})
	ct := container("app", enabled(nil), nil)

	cfg, _ := settleDocker(t, model.Config{ProxyHosts: []model.ProxyHost{stored}}, testConf(nil), []Container{ct}, nil)
	h := cfg.ProxyHosts[0]
	if h.TrustedProxies != nil || !maps.Equal(h.Labels, map[string]string{"gpm.rake.pro/managed-by": model.ManagedByDockerDiscovery}) {
		t.Fatalf("unset: host %+v, want no value and only managed-by", h)
	}

	own := []string{"192.0.2.10"}
	conf := testConf(func(s *model.Settings) { s.DockerDiscovery.Template.TrustedProxies = &own })
	cfg, _ = settleDocker(t, model.Config{ProxyHosts: []model.ProxyHost{stored}}, conf, []Container{ct}, nil)
	h = cfg.ProxyHosts[0]
	want := map[string]string{
		"gpm.rake.pro/managed-by":         model.ManagedByDockerDiscovery,
		"gpm.rake.pro/trusted-proxies-by": model.ManagedByDockerDiscovery,
	}
	if tps(h.TrustedProxies) != "[192.0.2.10]" || !maps.Equal(h.Labels, want) {
		t.Fatalf("set: host %+v, want the template's value and only this reconciler's marker", h)
	}
}
