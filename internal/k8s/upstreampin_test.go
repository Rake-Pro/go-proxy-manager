package k8s

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

// A template whose upstream (and a location's upstream) pins a certificate
// derives hosts that carry the pins in their own slices; once stored (a YAML
// round trip, as the store writes it) the next reconcile plans them
// unchanged, and a changed pin is restamped.
func TestDerivedHostCarriesUpstreamPin(t *testing.T) {
	pinA, pinB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("infra", "bmc", map[string]string{model.AnnotationManaged: "true"}, "bmc.example.com"),
		}, "")
	}
	s := baseSettings(f)
	s.Template.Upstream = model.Upstream{Scheme: "https", Host: "10.0.0.40", Port: 443, TLS: model.UpstreamTLS{PinnedCertSHA256: []string{pinA}}}
	lu := model.Upstream{Scheme: "https", Host: "10.0.0.6", Port: 8443, TLS: model.UpstreamTLS{PinnedCertSHA256: []string{pinB}}}
	s.Template.Locations = []model.Location{{Path: "/console", Upstream: &lu}}
	settings := &model.Settings{IngressDiscovery: s}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	h := rec.upserts[0]
	if !h.Upstream.Equal(s.Template.Upstream) || len(h.Locations) != 1 || !h.Locations[0].Upstream.Equal(lu) {
		t.Fatalf("derived upstream %+v, location %+v; want the template's pins", h.Upstream, h.Locations)
	}
	if &h.Upstream.TLS.PinnedCertSHA256[0] == &s.Template.Upstream.TLS.PinnedCertSHA256[0] ||
		&h.Locations[0].Upstream.TLS.PinnedCertSHA256[0] == &lu.TLS.PinnedCertSHA256[0] {
		t.Fatal("the derived host shares a pin slice with the template")
	}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}

	b, err := yaml.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var stored model.ProxyHost
	if err := yaml.Unmarshal(b, &stored); err != nil {
		t.Fatal(err)
	}
	// (Hosts without the field stay unchanged too: TestReconcileNoOpWritesNothing.)
	cfg.ProxyHosts = []model.ProxyHost{stored}
	rec.upserts = nil
	calls := rec.calls
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec.calls != calls {
		t.Fatalf("a stored pinned host replanned: upserts %+v", rec.upserts)
	}
	for _, hr := range d.Status().Hosts {
		if hr.Name == stored.Name && hr.Action != "unchanged" {
			t.Fatalf("stored pinned host planned %q, want unchanged", hr.Action)
		}
	}

	settings.IngressDiscovery.Template.Upstream.TLS.PinnedCertSHA256 = []string{pinB}
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rec.upserts) != 1 || !slicesEqual(rec.upserts[0].Upstream.TLS.PinnedCertSHA256, []string{pinB}) {
		t.Fatalf("after a pin change: upserts %+v, want the host restamped with the new pin", rec.upserts)
	}
}

func slicesEqual(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }
