package k8s

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/discovery"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

func parityIngress(ns, name, host string, annotations map[string]string) Ingress {
	var ing Ingress
	ing.Metadata.Namespace, ing.Metadata.Name = ns, name
	ing.Metadata.Annotations = map[string]string{model.AnnotationManaged: "true"}
	for k, v := range annotations {
		ing.Metadata.Annotations[k] = v
	}
	ing.Spec.Rules = []struct {
		Host string `json:"host"`
	}{{Host: host}}
	return ing
}

func parityConf() model.IngressDiscoverySettings {
	return model.IngressDiscoverySettings{
		Enabled:               true,
		AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.40", Port: 80},
			TLS:      model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
		},
	}
}

func tps(p *[]string) string {
	if p == nil {
		return "<unset>"
	}
	return "[" + strings.Join(*p, ",") + "]"
}

// The Ingress derive stamps trustedProxies from the default template and from
// a profile when they set it (an empty list included), leaves it unset when
// they do not, and never shares the settings' backing array.
func TestIngressDeriveStampsTemplateTrustedProxies(t *testing.T) {
	list := func(v ...string) *[]string { s := append([]string{}, v...); return &s }
	conf := parityConf()
	prof := conf.Template
	prof.TrustedProxies = list("198.51.100.0/24")
	conf.Profiles = map[string]model.IngressHostTemplate{"edge": prof}

	for _, tc := range []struct {
		name string
		tmpl *[]string
	}{{"unset", nil}, {"empty", list()}, {"list", list("192.0.2.10")}} {
		t.Run(tc.name, func(t *testing.T) {
			conf.Template.TrustedProxies = tc.tmpl
			p := planReconcile(model.Config{}, conf, []Ingress{
				parityIngress("web", "a", "a.example.com", nil),
				parityIngress("web", "b", "b.example.com", map[string]string{model.AnnotationProfile: "edge"}),
			}, nil, nil)
			if len(p.upserts) != 2 {
				t.Fatalf("upserts %+v, want two creates", p.upserts)
			}
			byName := map[string]model.ProxyHost{}
			for _, u := range p.upserts {
				byName[u.Name] = u
			}
			a, b := byName["ing-a.web"], byName["ing-b.web"]
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

// Hosts written before trustedProxies and the tag rule existed, read back from
// the YAML the store wrote, come out unchanged on every pass: no update, no
// commit. Covered for the default template and a profile, with template tags,
// without, and with an explicit empty list on either side.
func TestIngressLegacyHostsAreUnchanged(t *testing.T) {
	tests := []struct {
		name        string
		tmplTags    []string
		storedEmpty bool // the stored file carries "tags: []"
	}{
		{"no tags", nil, false},
		{"stored explicit empty tags", nil, true},
		{"template explicit empty tags", []string{}, false},
		{"both explicit empty", []string{}, true},
		{"template tags", []string{"cluster", "discovered"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conf := parityConf()
			conf.Template.Tags = tc.tmplTags
			prof := conf.Template
			prof.Middlewares = nil
			prof.Tags = append([]string{"edge"}, tc.tmplTags...)
			if tc.tmplTags == nil {
				prof.Tags = nil
			}
			conf.Profiles = map[string]model.IngressHostTemplate{"edge": prof}
			ings := []Ingress{
				parityIngress("web", "a", "a.example.com", nil),
				parityIngress("web", "b", "b.example.com", map[string]string{model.AnnotationProfile: "edge"}),
			}

			// What this version writes for a new host is what the store holds.
			var cfg model.Config
			for _, h := range planReconcile(model.Config{}, conf, ings, nil, nil).upserts {
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
				p := planReconcile(cfg, conf, ings, nil, mem)
				if len(p.upserts) != 0 || len(p.deletes) != 0 || p.updated != 0 {
					t.Fatalf("pass %d: upserts %+v deletes %v, want none", pass, p.upserts, p.deletes)
				}
				mem = p.stamped
			}
		})
	}
}

// A host that moves to another profile loses the tags the old profile
// stamped and keeps the ones added by hand.
func TestIngressProfileMoveDropsTheOldProfilesTags(t *testing.T) {
	conf := parityConf()
	p1 := conf.Template
	p1.Tags = []string{"p", "shared"}
	p2 := conf.Template
	p2.Tags = []string{"q", "shared"}
	conf.Profiles = map[string]model.IngressHostTemplate{"one": p1, "two": p2}

	ing := parityIngress("web", "a", "a.example.com", map[string]string{model.AnnotationProfile: "one"})
	p := planReconcile(model.Config{}, conf, []Ingress{ing}, nil, nil)
	if len(p.upserts) != 1 {
		t.Fatalf("upserts %+v, want one create", p.upserts)
	}
	h := p.upserts[0]
	h.Tags = append(h.Tags, "hand")
	cfg := model.Config{ProxyHosts: []model.ProxyHost{h}}

	ing.Metadata.Annotations[model.AnnotationProfile] = "two"
	p = planReconcile(cfg, conf, []Ingress{ing}, nil, p.stamped)
	if len(p.upserts) != 1 || !slices.Equal(p.upserts[0].Tags, []string{"q", "shared", "hand"}) {
		t.Fatalf("upserts %+v, want tags [q shared hand]", p.upserts)
	}
	if !slices.Equal(p.stamped["ing-a.web"], []string{"q", "shared"}) {
		t.Fatalf("memory %v, want the new profile's tags", p.stamped)
	}
}

// The Discoverer keeps the tag memory between passes: it advances only when a
// plan was applied (not on a failed write, not on a preview), and a fresh
// Discoverer starts without it.
func TestIngressDiscovererRemembersStampedTags(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "app", map[string]string{model.AnnotationManaged: "true"}, "app.example.com"),
		}, "")
	}
	cfg := model.Config{}
	settings := model.Settings{IngressDiscovery: baseSettings(f)}
	settings.IngressDiscovery.Template.Tags = []string{"a", "b"}
	var writes int
	var failWrite bool
	mk := func() *Discoverer {
		return New(
			func(context.Context) (model.Config, model.Settings, error) { return cfg, settings, nil },
			func(_ context.Context, ups []model.ProxyHost, dels []string, _ string) (string, error) {
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
				if len(dels) != 0 {
					t.Fatalf("unexpected deletes %v", dels)
				}
				return "commit", nil
			},
			nil,
		)
	}
	ctx := context.Background()
	tags := func() []string { return cfg.ProxyHosts[0].Tags }
	d := mk()

	if err := d.Reconcile(ctx); err != nil || writes != 1 || !slices.Equal(tags(), []string{"a", "b"}) {
		t.Fatalf("create: err=%v writes=%d tags=%v", err, writes, tags())
	}
	// A hand-added tag survives, and the pass writes nothing.
	cfg.ProxyHosts[0].Tags = []string{"a", "b", "x"}
	if err := d.Reconcile(ctx); err != nil || writes != 1 {
		t.Fatalf("hand-added tag: err=%v writes=%d, want no write", err, writes)
	}
	// The template drops b; the write fails, so the memory stays where it was
	// and the next, successful, pass still knows b was stamped.
	settings.IngressDiscovery.Template.Tags = []string{"a"}
	failWrite = true
	if err := d.Reconcile(ctx); err == nil {
		t.Fatal("reconcile succeeded with a failing writer")
	}
	failWrite = false
	if err := d.Reconcile(ctx); err != nil || writes != 2 || !slices.Equal(tags(), []string{"a", "x"}) {
		t.Fatalf("drop b: err=%v writes=%d tags=%v, want [a x]", err, writes, tags())
	}
	// A preview does not advance the memory: after it, the real pass still
	// knows a was stamped and drops it.
	settings.IngressDiscovery.Template.Tags = []string{"c"}
	if pl, err := d.Plan(ctx); err != nil || pl.Updated != 1 {
		t.Fatalf("preview: %+v, %v", pl, err)
	}
	if err := d.Reconcile(ctx); err != nil || !slices.Equal(tags(), []string{"c", "x"}) {
		t.Fatalf("after preview: err=%v tags=%v, want [c x]", err, tags())
	}
	// A restart has no memory: a template tag removed while stopped lingers.
	settings.IngressDiscovery.Template.Tags = nil
	before := writes
	d = mk()
	if err := d.Reconcile(ctx); err != nil || writes != before || !slices.Equal(tags(), []string{"c", "x"}) {
		t.Fatalf("restart: err=%v writes=%d tags=%v, want [c x] and no write", err, writes-before, tags())
	}
	// From here on the restarted Discoverer remembers what it stamped (nothing),
	// so c is a hand tag to it now; adding a template tag stamps it in front.
	settings.IngressDiscovery.Template.Tags = []string{"a"}
	if err := d.Reconcile(ctx); err != nil || !slices.Equal(tags(), []string{"a", "c", "x"}) {
		t.Fatalf("after restart: err=%v tags=%v, want [a c x]", err, tags())
	}
}

// trustedProxies through the Ingress reconciler: an operator's value on a
// managed host survives while the template leaves the field unset, and the
// template's value wins once it sets it.
func TestIngressDiscovererTrustedProxiesOwnership(t *testing.T) {
	conf := parityConf()
	ing := parityIngress("web", "app", "app.example.com", nil)
	p := planReconcile(model.Config{}, conf, []Ingress{ing}, nil, nil)
	h := p.upserts[0]
	if h.TrustedProxies != nil {
		t.Fatalf("new host trustedProxies %s, want none", tps(h.TrustedProxies))
	}
	own := []string{"192.0.2.10"}
	h.TrustedProxies = &own
	cfg := model.Config{ProxyHosts: []model.ProxyHost{h}}
	// Several passes: the operator's value stays, unmarked, and nothing is
	// written.
	cfg, mem, first := settlePlan(t, cfg, conf, []Ingress{ing}, p.stamped)
	if len(first.upserts) != 0 || tps(hostNamed(t, cfg, h.Name).TrustedProxies) != "[192.0.2.10]" || hostNamed(t, cfg, h.Name).Labels[model.TrustedProxiesByLabel] != "" {
		t.Fatalf("upserts %+v, want the operator's value kept, unmarked, with no write", first.upserts)
	}
	empty := []string{}
	conf.Template.TrustedProxies = &empty
	cfg, _, first = settlePlan(t, cfg, conf, []Ingress{ing}, mem)
	if got := hostNamed(t, cfg, h.Name); len(first.upserts) != 1 || tps(got.TrustedProxies) != "[]" || got.Labels[model.TrustedProxiesByLabel] != model.ManagedByIngressDiscovery {
		t.Fatalf("upserts %+v, want the template's empty list stamped and marked", first.upserts)
	}
}

// applyHosts writes upserts and deletes into cfg the way the store does.
func applyHosts(cfg model.Config, ups []model.ProxyHost, dels []string) model.Config {
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

// settlePlan applies one plan, asserts that three more passes over the
// previous output write nothing, and returns the settled config, memory and
// the first plan.
func settlePlan(t *testing.T, cfg model.Config, conf model.IngressDiscoverySettings, ings []Ingress, mem discovery.StampedTags) (model.Config, discovery.StampedTags, reconcilePlan) {
	t.Helper()
	first := planReconcile(cfg, conf, ings, nil, mem)
	cfg, mem = applyHosts(cfg, first.upserts, first.deletes), first.stamped
	for pass := range 3 {
		p := planReconcile(cfg, conf, ings, nil, mem)
		if len(p.upserts) != 0 || len(p.deletes) != 0 {
			t.Fatalf("pass %d after the write: upserts %+v deletes %v, want none", pass+1, p.upserts, p.deletes)
		}
		cfg, mem = applyHosts(cfg, p.upserts, p.deletes), p.stamped
	}
	return cfg, mem, first
}

func hostNamed(t *testing.T, cfg model.Config, name string) model.ProxyHost {
	t.Helper()
	for _, h := range cfg.ProxyHosts {
		if h.Name == name {
			return h
		}
	}
	t.Fatalf("no host %q in %+v", name, cfg.ProxyHosts)
	return model.ProxyHost{}
}

// A trustedProxies value a profile stamped does not outlive the profile: a host
// moved from a profile that sets it to one that does not (by annotation or by
// profile rule) loses the value and the marker, exactly like a new host on
// that profile, and so does a host whose template drops the key.
func TestIngressStampedTrustedProxiesFollowTheProfile(t *testing.T) {
	wide := []string{"0.0.0.0/0"}
	base := parityConf()
	lb := base.Template
	lb.TrustedProxies = &wide
	strict := base.Template
	mkConf := func() model.IngressDiscoverySettings {
		c := parityConf()
		c.Profiles = map[string]model.IngressHostTemplate{"lb": lb, "strict": strict}
		return c
	}
	marker := model.TrustedProxiesByLabel

	t.Run("annotation", func(t *testing.T) {
		conf := mkConf()
		ing := parityIngress("web", "app", "app.example.com", map[string]string{model.AnnotationProfile: "lb"})
		cfg, mem, _ := settlePlan(t, model.Config{}, conf, []Ingress{ing}, nil)
		h := hostNamed(t, cfg, "ing-app.web")
		if tps(h.TrustedProxies) != "[0.0.0.0/0]" || h.Labels[marker] != model.ManagedByIngressDiscovery {
			t.Fatalf("on lb: %+v, want the profile's value and the marker", h)
		}
		ing.Metadata.Annotations[model.AnnotationProfile] = "strict"
		newcomer := parityIngress("web", "new", "new.example.com", map[string]string{model.AnnotationProfile: "strict"})
		cfg, _, _ = settlePlan(t, cfg, conf, []Ingress{ing, newcomer}, mem)
		for _, name := range []string{"ing-app.web", "ing-new.web"} {
			h := hostNamed(t, cfg, name)
			if h.TrustedProxies != nil || h.Labels[marker] != "" {
				t.Fatalf("%s on strict: trustedProxies %s labels %v, want neither", name, tps(h.TrustedProxies), h.Labels)
			}
		}
	})

	t.Run("profile rule", func(t *testing.T) {
		conf := mkConf()
		ing := parityIngress("web", "app", "app.example.com", map[string]string{model.AnnotationProfile: "lb"})
		cfg, mem, _ := settlePlan(t, model.Config{}, conf, []Ingress{ing}, nil)
		conf.ProfileRules = []model.IngressProfileRule{{Namespace: "web", Profile: "strict"}}
		cfg, _, _ = settlePlan(t, cfg, conf, []Ingress{ing}, mem)
		if h := hostNamed(t, cfg, "ing-app.web"); h.TrustedProxies != nil || h.Labels[marker] != "" {
			t.Fatalf("routed to strict: %+v, want no trustedProxies and no marker", h)
		}
	})

	t.Run("key removed from the template", func(t *testing.T) {
		conf := parityConf()
		conf.Template.TrustedProxies = &wide
		ing := parityIngress("web", "app", "app.example.com", nil)
		cfg, mem, _ := settlePlan(t, model.Config{}, conf, []Ingress{ing}, nil)
		conf.Template.TrustedProxies = nil
		cfg, mem, _ = settlePlan(t, cfg, conf, []Ingress{ing}, mem)
		if h := hostNamed(t, cfg, "ing-app.web"); h.TrustedProxies != nil || h.Labels[marker] != "" {
			t.Fatalf("after removal: %+v, want no trustedProxies and no marker", h)
		}
		// Set again: stamped and marked again.
		conf.Template.TrustedProxies = &wide
		cfg, _, _ = settlePlan(t, cfg, conf, []Ingress{ing}, mem)
		if h := hostNamed(t, cfg, "ing-app.web"); tps(h.TrustedProxies) != "[0.0.0.0/0]" || h.Labels[marker] == "" {
			t.Fatalf("set again: %+v, want the value and the marker", h)
		}
	})
}

// During an annotation prefix migration the marker moves with the other
// labels: a host still marked under the old prefix is recognised as stamped,
// cleared when its template leaves the key unset, relabelled when it sets it,
// and keeps the marker (under the new prefix) on a fail-closed disable and on a
// domain-conflict restamp.
func TestIngressPrefixMigrationCarriesTheMarker(t *testing.T) {
	wide := []string{"0.0.0.0/0"}
	stale := managedHostFixture("ing-app.default", "app.example.com")
	stale.Upstream = model.Upstream{Scheme: "http", Host: "192.0.2.40", Port: 80}
	stale.TLS = model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true}
	stale.DisplayName = "default/app"
	stale.Labels = map[string]string{
		model.ManagedByLabel:        model.ManagedByIngressDiscovery,
		model.TrustedProxiesByLabel: model.ManagedByIngressDiscovery,
	}
	stale.TrustedProxies = &wide
	conf := parityConf()
	conf.AnnotationPrefix = "acme.example.com"
	conf.AnnotationPrefixMigrate = true
	ing := func(annotations map[string]string, hosts ...string) Ingress {
		var i Ingress
		i.Metadata.Namespace, i.Metadata.Name = "default", "app"
		i.Metadata.Annotations = map[string]string{"acme.example.com/managed": "true"}
		for k, v := range annotations {
			i.Metadata.Annotations[k] = v
		}
		for _, h := range hosts {
			i.Spec.Rules = append(i.Spec.Rules, struct {
				Host string `json:"host"`
			}{Host: h})
		}
		return i
	}
	cfg := model.Config{ProxyHosts: []model.ProxyHost{stale}}

	t.Run("unset template clears", func(t *testing.T) {
		got, _, _ := settlePlan(t, cfg, conf, []Ingress{ing(nil, "app.example.com")}, nil)
		h := hostNamed(t, got, "ing-app.default")
		if h.TrustedProxies != nil || !mapsEqual(h.Labels, map[string]string{"acme.example.com/managed-by": model.ManagedByIngressDiscovery}) {
			t.Fatalf("host %+v, want no trustedProxies and only the new managed-by", h)
		}
	})
	t.Run("set template relabels the marker", func(t *testing.T) {
		c := conf
		own := []string{"192.0.2.10"}
		c.Template.TrustedProxies = &own
		got, _, _ := settlePlan(t, cfg, c, []Ingress{ing(nil, "app.example.com")}, nil)
		h := hostNamed(t, got, "ing-app.default")
		want := map[string]string{
			"acme.example.com/managed-by":         model.ManagedByIngressDiscovery,
			"acme.example.com/trusted-proxies-by": model.ManagedByIngressDiscovery,
		}
		if tps(h.TrustedProxies) != "[192.0.2.10]" || !mapsEqual(h.Labels, want) {
			t.Fatalf("host %+v, want the template's value and the new-prefix labels", h)
		}
	})
	t.Run("fail-closed keeps the marker", func(t *testing.T) {
		p := planReconcile(cfg, conf, []Ingress{ing(map[string]string{"acme.example.com/profile": "nope"}, "app.example.com")}, nil, nil)
		if len(p.upserts) != 1 || !p.upserts[0].Disabled {
			t.Fatalf("upserts %+v, want one fail-closed disable", p.upserts)
		}
		want := map[string]string{
			"acme.example.com/managed-by":         model.ManagedByIngressDiscovery,
			"acme.example.com/disabled-by":        model.DisabledByIngressDiscovery,
			"acme.example.com/trusted-proxies-by": model.ManagedByIngressDiscovery,
		}
		if h := p.upserts[0]; tps(h.TrustedProxies) != "[0.0.0.0/0]" || !mapsEqual(h.Labels, want) {
			t.Fatalf("disabled host %+v, want the stored value and the new-prefix labels", h)
		}
	})
	t.Run("restamp clears and relabels", func(t *testing.T) {
		op := operatorHost("www", "www.example.com")
		c := model.Config{ProxyHosts: []model.ProxyHost{stale, op}}
		got, _, first := settlePlan(t, c, conf, []Ingress{ing(nil, "app.example.com", "www.example.com")}, nil)
		if first.updated != 1 || first.skipped != 0 {
			t.Fatalf("first plan %+v, want one restamp", first.results)
		}
		h := hostNamed(t, got, "ing-app.default")
		if h.TrustedProxies != nil || !slices.Equal(h.Domains, []string{"app.example.com"}) ||
			!mapsEqual(h.Labels, map[string]string{"acme.example.com/managed-by": model.ManagedByIngressDiscovery}) {
			t.Fatalf("restamped host %+v, want no trustedProxies, its current domain and only the new managed-by", h)
		}
	})
}

// parityHarness wires a Discoverer against the fake API with a writer that
// applies to cfg, can fail, or can report an empty commit without writing.
type parityHarness struct {
	cfg      model.Config
	settings model.Settings
	writes   int
	fail     bool
	empty    bool
}

func (h *parityHarness) discoverer() *Discoverer {
	return New(
		func(context.Context) (model.Config, model.Settings, error) { return h.cfg, h.settings, nil },
		func(_ context.Context, ups []model.ProxyHost, dels []string, _ string) (string, error) {
			if h.fail {
				return "", errors.New("write failed")
			}
			if h.empty {
				return "", nil
			}
			h.writes++
			h.cfg = applyHosts(h.cfg, ups, dels)
			return "commit", nil
		},
		nil,
	)
}

func parityHarnessFor(t *testing.T, tags ...string) *parityHarness {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "app", map[string]string{model.AnnotationManaged: "true"}, "app.example.com"),
		}, "")
	}
	h := &parityHarness{settings: model.Settings{IngressDiscovery: baseSettings(f)}}
	h.settings.IngressDiscovery.Template.Tags = tags
	return h
}

// A writer that reports an empty commit wrote nothing, so the memory of the
// hosts the plan wanted to write stays where it was: a template tag removed in
// that window is still recognised as stale once a write goes through.
func TestIngressEmptyCommitKeepsThePreviousMemory(t *testing.T) {
	h := parityHarnessFor(t, "a", "b")
	d := h.discoverer()
	ctx := context.Background()
	if err := d.Reconcile(ctx); err != nil || h.writes != 1 {
		t.Fatalf("create: %v writes=%d", err, h.writes)
	}
	h.settings.IngressDiscovery.Template.Tags = []string{"a"}
	h.empty = true
	if err := d.Reconcile(ctx); err != nil {
		t.Fatalf("empty commit: %v", err)
	}
	h.empty = false
	if err := d.Reconcile(ctx); err != nil || h.writes != 2 || !slices.Equal(h.cfg.ProxyHosts[0].Tags, []string{"a"}) {
		t.Fatalf("after the empty commit: err=%v writes=%d tags=%v, want b dropped", err, h.writes, h.cfg.ProxyHosts[0].Tags)
	}
}

// Disabling the block forgets the tag memory, so enabling it again behaves
// like a restart: a tag the template dropped meanwhile is not treated as
// stale.
func TestIngressDisableForgetsTheMemory(t *testing.T) {
	h := parityHarnessFor(t, "a", "b")
	d := h.discoverer()
	ctx := context.Background()
	if err := d.Reconcile(ctx); err != nil || h.writes != 1 {
		t.Fatalf("create: %v writes=%d", err, h.writes)
	}
	h.settings.IngressDiscovery.Enabled = false
	if err := d.Reconcile(ctx); err != nil {
		t.Fatalf("disabled pass: %v", err)
	}
	h.settings.IngressDiscovery.Enabled = true
	h.settings.IngressDiscovery.Template.Tags = []string{"a"}
	if err := d.Reconcile(ctx); err != nil || h.writes != 1 || !slices.Equal(h.cfg.ProxyHosts[0].Tags, []string{"a", "b"}) {
		t.Fatalf("re-enabled: err=%v writes=%d tags=%v, want b kept and no write", err, h.writes, h.cfg.ProxyHosts[0].Tags)
	}
}

// An operator changed the annotation prefix and relabelled managed-by by hand,
// with annotationPrefixMigrate off, leaving the old prefix's marker behind:
// the value is still read as stamped, so an unset template clears it and a
// set one restamps it under the new prefix. One write, then nothing.
func TestIngressOldPrefixMarkerWithoutMigrate(t *testing.T) {
	wide := []string{"0.0.0.0/0"}
	conf := parityConf()
	conf.AnnotationPrefix = "acme.example.com"
	ing := parityIngress("web", "app", "app.example.com", nil)
	ing.Metadata.Annotations = map[string]string{"acme.example.com/managed": "true"}
	// What this version writes for the host under the new prefix, plus the
	// stale marker and its value.
	created := planReconcile(model.Config{}, conf, []Ingress{ing}, nil, nil).upserts[0]
	stored := created
	stored.Labels = map[string]string{
		"acme.example.com/managed-by": model.ManagedByIngressDiscovery,
		model.TrustedProxiesByLabel:   model.ManagedByIngressDiscovery,
	}
	stored.TrustedProxies = &wide

	got, _, first := settlePlan(t, model.Config{ProxyHosts: []model.ProxyHost{stored}}, conf, []Ingress{ing}, nil)
	h := hostNamed(t, got, "ing-app.web")
	if len(first.upserts) != 1 || h.TrustedProxies != nil || !mapsEqual(h.Labels, map[string]string{"acme.example.com/managed-by": model.ManagedByIngressDiscovery}) {
		t.Fatalf("unset: host %+v, want no value and only the new managed-by", h)
	}

	own := []string{"192.0.2.10"}
	conf.Template.TrustedProxies = &own
	got, _, first = settlePlan(t, model.Config{ProxyHosts: []model.ProxyHost{stored}}, conf, []Ingress{ing}, nil)
	h = hostNamed(t, got, "ing-app.web")
	want := map[string]string{
		"acme.example.com/managed-by":         model.ManagedByIngressDiscovery,
		"acme.example.com/trusted-proxies-by": model.ManagedByIngressDiscovery,
	}
	if len(first.upserts) != 1 || tps(h.TrustedProxies) != "[192.0.2.10]" || !mapsEqual(h.Labels, want) {
		t.Fatalf("set: host %+v, want the template's value and only the new-prefix marker", h)
	}
}
