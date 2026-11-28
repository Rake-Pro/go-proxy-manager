package docker

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.yaml.in/yaml/v3"
)

// A pin on the Docker template's own upstream is refused by settings
// validation (TestDockerDiscoveryTemplateRejectsPins in internal/model): the
// host upstream is the container's. A template location's pinned upstream is
// stamped, in its own slice, and the settled host plans unchanged after a
// store round trip.
func TestDeriveCarriesLocationUpstreamPin(t *testing.T) {
	pin := strings.Repeat("c", 64)
	lu := model.Upstream{Scheme: "https", Host: "192.0.2.20", Port: 8443, TLS: model.UpstreamTLS{PinnedCertSHA256: []string{pin}}}
	var settings model.Settings
	conf := testConf(func(s *model.Settings) {
		s.AdminAuth.LocalLoginEnabled = true
		s.DockerDiscovery.Template.Locations = []model.Location{{Path: "/console", Upstream: &lu}}
		settings = *s
	})
	if err := settings.Validate(); err != nil {
		t.Fatalf("a pinned template location was refused: %v", err)
	}
	settings.DockerDiscovery.Template.Upstream = model.Upstream{Scheme: "https", Host: "192.0.2.30", Port: 443, TLS: model.UpstreamTLS{PinnedCertSHA256: []string{pin}}}
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "dockerDiscovery.template.upstream.tls.pinnedCertSHA256: certificate pins are not supported on Docker discovery templates") {
		t.Fatalf("a pinned template upstream: %v, want refused", err)
	}
	ct := container("app", enabled(nil), nil)
	_, host, _, err := derive(ct, conf)
	if err != nil {
		t.Fatal(err)
	}
	if !host.Upstream.TLS.IsZero() || host.Upstream.Host != "192.0.2.10" {
		t.Fatalf("host upstream = %+v, want the container's address", host.Upstream)
	}
	if len(host.Locations) != 1 || !host.Locations[0].Upstream.Equal(lu) {
		t.Fatalf("locations = %+v, want the template's pinned location upstream", host.Locations)
	}
	if &host.Locations[0].Upstream.TLS.PinnedCertSHA256[0] == &conf.Template.Locations[0].Upstream.TLS.PinnedCertSHA256[0] {
		t.Fatal("the derived location shares the template's pin slice")
	}
	if err := host.Validate(); err != nil {
		t.Fatal(err)
	}

	cfg, mem := settleDocker(t, model.Config{}, conf, []Container{ct}, nil)
	for i, h := range cfg.ProxyHosts {
		b, err := yaml.Marshal(h)
		if err != nil {
			t.Fatal(err)
		}
		var back model.ProxyHost
		if err := yaml.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		cfg.ProxyHosts[i] = back
	}
	if p := planReconcile(cfg, conf, []Container{ct}, nil, mem); len(p.Upserts) != 0 || len(p.Deletes) != 0 {
		t.Fatalf("after a store round trip: upserts %+v deletes %v, want none", p.Upserts, p.Deletes)
	}
}

// The domain-conflict restamp compares the stored upstream with the derived
// one including its pins: a pin added to a managed host by hand is a
// difference, restamped away like any other, while the conflict lasts.
func TestDomainConflictRestampNoticesAPinDifference(t *testing.T) {
	conf := testConf(nil)
	operator := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "operator-www"},
		Domains:    []string{"www.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "192.0.2.41", Port: 80},
	}
	ct := container("app", enabled(map[string]string{
		"gpm.rake.pro/domains": "app.example.com,www.example.com",
		"gpm.rake.pro/scheme":  "https",
	}), withNetworks(map[string]string{"bridge": "192.0.2.20"}))
	stale := managedHostFixture("dkr-app", func(h *model.ProxyHost) { h.AccessLists = nil })
	p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{stale, operator}}, conf, []Container{ct}, nil, nil)
	if len(p.Upserts) != 1 {
		t.Fatalf("plan %+v, want one restamp", p)
	}
	u := p.Upserts[0]
	if u.Upstream.Scheme != "https" || !u.Upstream.TLS.IsZero() {
		t.Fatalf("restamped upstream %+v, want the container's https address without pins", u.Upstream)
	}
	// Control: the restamped host is current, so the next plan leaves it alone.
	p = planReconcile(model.Config{ProxyHosts: []model.ProxyHost{u, operator}}, conf, []Container{ct}, nil, nil)
	if len(p.Upserts) != 0 {
		t.Fatalf("second plan %+v, want no write", p)
	}
	// Only a pin differs from what the container renders.
	pinned := u
	pinned.Upstream = u.Upstream.Clone()
	pinned.Upstream.TLS.PinnedCertSHA256 = []string{strings.Repeat("c", 64)}
	p = planReconcile(model.Config{ProxyHosts: []model.ProxyHost{pinned, operator}}, conf, []Container{ct}, nil, nil)
	if len(p.Upserts) != 1 || !p.Upserts[0].Upstream.TLS.IsZero() {
		t.Fatalf("plan with a hand-added pin %+v, want one restamp that drops it", p)
	}
}

// Docker discovery that inherits a pinned ingressDiscovery profile is accepted
// (the profile is right for Ingress hosts), and a container selecting it gets
// the container's address without the pin. That is not silent: the reconcile
// logs one warning naming the profile, once per settings change, not per poll.
func TestInheritedPinnedProfileWarnsOnce(t *testing.T) {
	pinned := model.Upstream{Scheme: "https", Host: "192.0.2.30", Port: 443, TLS: model.UpstreamTLS{PinnedCertSHA256: []string{strings.Repeat("c", 64)}}}
	settings := enabledSettings(func(s *model.Settings) {
		s.IngressDiscovery.Profiles = map[string]model.IngressHostTemplate{"pinned": {Upstream: pinned, TLS: model.TLSSettings{CertificateRef: "wildcard"}}}
	})
	conf := settings.DockerDiscoveryResolved()
	if err := conf.Validate(); err != nil {
		t.Fatalf("an inherited pinned profile was refused: %v", err)
	}
	ct := container("app", enabled(map[string]string{conf.LabelProfile(): "pinned", conf.LabelScheme(): "https"}), nil)
	_, host, prof, err := derive(ct, conf)
	if err != nil || prof != "pinned" {
		t.Fatalf("derive: profile %q, err %v", prof, err)
	}
	if !host.Upstream.TLS.IsZero() || host.Upstream.Host != "192.0.2.10" {
		t.Fatalf("host upstream = %+v, want the container's address without pins", host.Upstream)
	}

	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })
	d, _ := newTestDiscoverer(t, model.Config{}, settings, engine(t, []Container{ct}, 0))
	for range 2 {
		if err := d.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	want := `dockerDiscovery inherits ingressDiscovery profile \"pinned\", whose upstream.tls.pinnedCertSHA256 does not apply to hosts derived from containers`
	if n := strings.Count(buf.String(), want); n != 1 {
		t.Fatalf("warning logged %d times over two reconciles, want once; log:\n%s", n, buf.String())
	}

	// Profiles of its own: nothing is inherited, nothing to warn about.
	settings.DockerDiscovery.Profiles = map[string]model.IngressHostTemplate{"plain": {TLS: model.TLSSettings{CertificateRef: "wildcard"}}}
	if w := settings.DockerInheritedPinWarnings(); w != nil {
		t.Fatalf("warnings with own profiles: %q", w)
	}
	settings.DockerDiscovery.Profiles = nil
	settings.DockerDiscovery.Enabled = false
	if w := settings.DockerInheritedPinWarnings(); w != nil {
		t.Fatalf("warnings with docker discovery disabled: %q", w)
	}
}
