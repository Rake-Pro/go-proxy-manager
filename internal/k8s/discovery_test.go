package k8s

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// baseSettings is a minimal valid discovery configuration pointed at f.
func baseSettings(f *fakeAPI) model.IngressDiscoverySettings {
	return model.IngressDiscoverySettings{
		Enabled:               true,
		APIURL:                f.srv.URL,
		TokenFile:             f.tokenFile,
		CAFile:                f.caFile,
		AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:      model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true, HTTP2: true},
		},
	}
}

// recorder captures what the reconciler asked to write.
type recorder struct {
	mu      sync.Mutex
	calls   int
	upserts []model.ProxyHost
	deletes []string
	message string
	err     error
	changed int
}

func (r *recorder) apply(_ context.Context, upserts []model.ProxyHost, deletes []string, message string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return "", r.err
	}
	r.upserts, r.deletes, r.message = upserts, deletes, message
	return fmt.Sprintf("commit%d", r.calls), nil
}

// newDiscoverer wires a Discoverer against the fake API and an in-memory config.
func newDiscoverer(f *fakeAPI, cfg *model.Config, settings *model.Settings, rec *recorder) *Discoverer {
	d := New(
		func(context.Context) (model.Config, model.Settings, error) { return *cfg, *settings, nil },
		rec.apply,
		func(string) { rec.changed++ },
	)
	return d
}

func managedHostFixture(name string, domains ...string) model.ProxyHost {
	return model.ProxyHost{
		ObjectMeta: model.ObjectMeta{
			Name:   name,
			Labels: map[string]string{model.ManagedByLabel: model.ManagedByIngressDiscovery},
		},
		Domains:  domains,
		Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
		TLS:      model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true, HTTP2: true},
	}
}

func TestReconcileCreatesFromAnnotatedIngress(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"}, "grafana.example.com"),
			// Unannotated: invisible. Opt-in is the only mode.
			ingressJSON("default", "secret-internal", nil, "secret.example.com"),
			// Explicitly not "true": still invisible.
			ingressJSON("default", "off", map[string]string{model.AnnotationManaged: "yes"}, "off.example.com"),
		}, "")
	}
	cfg := &model.Config{}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 || len(rec.deletes) != 0 {
		t.Fatalf("upserts=%d deletes=%d (only the annotated Ingress may be acted on)", len(rec.upserts), len(rec.deletes))
	}
	h := rec.upserts[0]
	if h.Name != "ing-grafana.monitoring" {
		t.Fatalf("derived name = %q", h.Name)
	}
	if h.Labels[model.ManagedByLabel] != model.ManagedByIngressDiscovery {
		t.Fatalf("derived host must carry the ownership label, got %v", h.Labels)
	}
	if strings.Join(h.Domains, ",") != "grafana.example.com" {
		t.Fatalf("domains = %v", h.Domains)
	}
	// Everything security-relevant comes from the template, never the Ingress.
	if h.Upstream != (model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80}) {
		t.Fatalf("upstream = %+v, want the template's ingress-controller address", h.Upstream)
	}
	if h.TLS.CertificateRef != "wildcard" {
		t.Fatalf("certificateRef = %q, want the template's", h.TLS.CertificateRef)
	}
	if !strings.Contains(rec.message, "+1 ~0 -0") {
		t.Fatalf("commit message = %q", rec.message)
	}
	st := d.Status()
	if st.Discovered != 1 || st.Created != 1 || st.Managed != 1 || st.Commit != "commit1" {
		t.Fatalf("status = %+v", st)
	}
	if rec.changed != 1 {
		t.Fatalf("onChange fired %d times, want 1", rec.changed)
	}
}

// A reconcile that finds no drift writes nothing at all: no commit, no reload,
// no DNS trigger. The steady state must not produce empty revisions.
func TestReconcileNoOpWritesNothing(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"}, "grafana.example.com"),
		}, "")
	}
	existing := managedHostFixture("ing-grafana.monitoring", "grafana.example.com")
	existing.DisplayName = "monitoring/grafana"
	existing.CreatedAt = time.Now().Add(-time.Hour)
	existing.UpdatedAt = time.Now()
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("a no-op reconcile must not write (calls=%d)", rec.calls)
	}
	st := d.Status()
	if st.Created+st.Updated+st.Deleted != 0 || st.Commit != "" {
		t.Fatalf("status = %+v", st)
	}
	if len(st.Hosts) != 1 || st.Hosts[0].Action != ActionUnchanged {
		t.Fatalf("hosts = %+v", st.Hosts)
	}
}

func TestReconcileUpdatesChangedDomains(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"},
				"grafana.example.com", "metrics.example.com"),
		}, "")
	}
	existing := managedHostFixture("ing-grafana.monitoring", "grafana.example.com")
	existing.DisplayName = "monitoring/grafana"
	created := time.Now().Add(-48 * time.Hour).UTC()
	existing.CreatedAt = created
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	got := rec.upserts[0]
	// Sorted so a reordered API response is not a spurious update.
	if strings.Join(got.Domains, ",") != "grafana.example.com,metrics.example.com" {
		t.Fatalf("domains = %v", got.Domains)
	}
	if !got.CreatedAt.Equal(created) {
		t.Fatalf("an update must carry the original createdAt forward, got %v", got.CreatedAt)
	}
	if d.Status().Updated != 1 {
		t.Fatalf("status = %+v", d.Status())
	}
}

// An Ingress that loses its annotation, and one that is deleted outright, are
// the same event to a full-state reconciler: the derived host goes away.
func TestReconcileDeletesWhenTheIngressStopsOptingIn(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			// The annotation is gone; the Ingress itself still exists.
			ingressJSON("monitoring", "grafana", nil, "grafana.example.com"),
		}, "")
	}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{
		managedHostFixture("ing-grafana.monitoring", "grafana.example.com"),
		managedHostFixture("ing-gone.monitoring", "gone.example.com"), // Ingress deleted
	}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if strings.Join(rec.deletes, ",") != "ing-gone.monitoring,ing-grafana.monitoring" {
		t.Fatalf("deletes = %v", rec.deletes)
	}
	if len(rec.upserts) != 0 {
		t.Fatalf("upserts = %v", rec.upserts)
	}
	if st := d.Status(); st.Deleted != 2 || st.Managed != 0 {
		t.Fatalf("status = %+v", st)
	}
}

// An empty successful list is a legitimate delete-all (every annotation really
// was removed) - and is a completely different code path from an error.
func TestReconcileEmptyListDeletesManagedHosts(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) { writeList(w, nil, "") }
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{
		managedHostFixture("ing-a.ns", "a.example.com"),
	}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if strings.Join(rec.deletes, ",") != "ing-a.ns" {
		t.Fatalf("an empty list must remove the managed hosts, deletes = %v", rec.deletes)
	}
}

// ...and an ERROR must not. This is the pairing that makes "empty vs failed"
// impossible to confuse: same managed hosts, same reconciler, opposite outcome.
func TestReconcileFreezesOnListError(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"unauthorized", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Unauthorized"}`)
		}},
		{"server error", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"boom"}`)
		}},
		{"empty-looking error body", func(w http.ResponseWriter, r *http.Request) {
			// The nastiest shape: a failure whose body would decode as an empty
			// list. The status code alone must keep it out of the desired set.
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"items":[]}`)
		}},
		{"partial pagination", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("continue") == "" {
				writeList(w, []string{ingressJSON("ns", "a", map[string]string{model.AnnotationManaged: "true"}, "a.example.com")}, "next")
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, "tok")
			f.handler = tc.handler
			cfg := &model.Config{ProxyHosts: []model.ProxyHost{
				managedHostFixture("ing-a.ns", "a.example.com"),
				managedHostFixture("ing-b.ns", "b.example.com"),
			}}
			settings := &model.Settings{IngressDiscovery: baseSettings(f)}
			rec := &recorder{}
			d := newDiscoverer(f, cfg, settings, rec)

			if err := d.Reconcile(context.Background()); err == nil {
				t.Fatal("a failed list must be reported as an error")
			}
			if rec.calls != 0 {
				t.Fatalf("a failed list must write NOTHING - no creates, no deletes (calls=%d)", rec.calls)
			}
			st := d.Status()
			if st.Error == "" {
				t.Fatal("status must carry the error")
			}
			if !st.LastSuccess.IsZero() {
				t.Fatalf("lastSuccess must stay unset until a run actually succeeds, got %v", st.LastSuccess)
			}
		})
	}
}

// A freeze must not erase the previous good run's numbers: the operator has to
// be able to see both "last run failed" and "state as of the last success".
func TestFreezeKeepsThePreviousSuccessInStatus(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("ns", "a", map[string]string{model.AnnotationManaged: "true"}, "a.example.com"),
		}, "")
	}
	cfg := &model.Config{}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	good := d.Status()
	if good.LastSuccess.IsZero() || good.Discovered != 1 {
		t.Fatalf("first status = %+v", good)
	}

	f.handler = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }
	if err := d.Reconcile(context.Background()); err == nil {
		t.Fatal("second reconcile must fail")
	}
	st := d.Status()
	if !st.LastSuccess.Equal(good.LastSuccess) {
		t.Fatalf("lastSuccess moved on a failure: %v -> %v", good.LastSuccess, st.LastSuccess)
	}
	if st.LastRun.Before(good.LastRun) {
		t.Fatal("lastRun must advance even on a failure")
	}
	if st.Discovered != good.Discovered {
		t.Fatalf("the frozen status must keep the last good counts, got %+v", st)
	}
}

// The ownership invariant: an unlabelled host with the derived name is never
// written and never deleted, only skipped with a reason.
func TestUnlabelledHostIsNeverTouched(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"}, "grafana.example.com"),
		}, "")
	}
	operator := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "ing-grafana.monitoring"},
		Domains:    []string{"handwritten.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "192.0.2.9", Port: 8080},
	}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{operator}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("an operator-authored host must be neither overwritten nor deleted (calls=%d)", rec.calls)
	}
	st := d.Status()
	if st.Skipped != 1 || len(st.Hosts) != 1 || st.Hosts[0].Action != ActionSkipped {
		t.Fatalf("status = %+v", st)
	}
	if !strings.Contains(st.Hosts[0].Reason, "not managed by ingress discovery") {
		t.Fatalf("skip reason = %q", st.Hosts[0].Reason)
	}
}

// Hosts with a DIFFERENT managed-by label (some other subsystem) are equally
// off-limits: the label pair is the whole ownership test.
func TestForeignManagedByLabelIsNotOwned(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) { writeList(w, nil, "") }
	foreign := managedHostFixture("ing-a.ns", "a.example.com")
	foreign.Labels[model.ManagedByLabel] = "something-else"
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{foreign}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("a host labelled for another owner must not be deleted (calls=%d)", rec.calls)
	}
}

// A malformed annotated Ingress freezes its own derived host rather than
// deleting it: one bad manifest edit must not take a host offline.
func TestUnderivableIngressProtectsItsExistingHost(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			// A wildcard host: rejected, so nothing can be derived.
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"}, "*.example.com"),
		}, "")
	}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{
		managedHostFixture("ing-grafana.monitoring", "grafana.example.com"),
	}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("a derivation failure must not delete the existing host (calls=%d)", rec.calls)
	}
	st := d.Status()
	if st.Skipped != 1 || st.Deleted != 0 {
		t.Fatalf("status = %+v", st)
	}
}

func TestHostnameValidationAndSuffixGate(t *testing.T) {
	conf := model.IngressDiscoverySettings{
		Enabled:               true,
		AllowedDomainSuffixes: []string{"example.com", ".apps.internal"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:      model.TLSSettings{CertificateRef: "wildcard"},
		},
	}
	tests := []struct {
		name    string
		hosts   []string
		want    string // comma-joined derived domains, "" = derivation fails
		wantErr string
	}{
		{"plain host", []string{"app.example.com"}, "app.example.com", ""},
		{"uppercase normalised", []string{"App.Example.COM"}, "app.example.com", ""},
		{"trailing dot stripped", []string{"app.example.com."}, "app.example.com", ""},
		{"second suffix", []string{"api.apps.internal"}, "api.apps.internal", ""},
		{"duplicates collapse", []string{"a.example.com", "a.example.com"}, "a.example.com", ""},
		{"sorted", []string{"z.example.com", "a.example.com"}, "a.example.com,z.example.com", ""},
		{"wildcard rejected", []string{"*.example.com"}, "", "not a valid hostname"},
		{"outside suffixes", []string{"evil.attacker.test"}, "", "outside allowedDomainSuffixes"},
		{"suffix must be on a label boundary", []string{"notexample.com"}, "", "outside allowedDomainSuffixes"},
		{"url is not a hostname", []string{"http://app.example.com/x"}, "", "not a valid hostname"},
		{"underscore rejected", []string{"a_b.example.com"}, "", "not a valid hostname"},
		{"whitespace injection rejected", []string{"a.example.com b.example.com"}, "", "not a valid hostname"},
		{"single label rejected", []string{"localhost"}, "", "not a valid hostname"},
		{"empty host skipped", []string{""}, "", "no usable host"},
		{"mixed keeps the good one", []string{"*.example.com", "ok.example.com"}, "ok.example.com", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ing Ingress
			ing.Metadata.Namespace, ing.Metadata.Name = "ns", "app"
			for _, h := range tc.hosts {
				ing.Spec.Rules = append(ing.Spec.Rules, struct {
					Host string `json:"host"`
				}{Host: h})
			}
			_, host, err := derive(ing, conf)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("expected a derivation failure, got domains %v", host.Domains)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v does not mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("derive: %v", err)
			}
			if strings.Join(host.Domains, ",") != tc.want {
				t.Fatalf("domains = %v, want %v", host.Domains, tc.want)
			}
		})
	}
}

func TestDerivedNaming(t *testing.T) {
	conf := model.IngressDiscoverySettings{
		AllowedDomainSuffixes: []string{"example.com"},
		Template:              model.IngressHostTemplate{Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 80}},
	}
	tests := []struct {
		ns, name string
		want     string
		wantErr  bool
	}{
		{"monitoring", "grafana", "ing-grafana.monitoring", false},
		{"a", "b-c", "ing-b-c.a", false},
		{"a-b", "c", "ing-c.a-b", false},
		{"ns", strings.Repeat("x", 250), "", true},
		{"", "grafana", "", true},
		{"ns", "", "", true},
		// Kubernetes would never produce these, but the API response is untrusted.
		{"ns", "../../etc/passwd", "", true},
		{"ns", "Upper", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.ns+"/"+tc.name, func(t *testing.T) {
			var ing Ingress
			ing.Metadata.Namespace, ing.Metadata.Name = tc.ns, tc.name
			ing.Spec.Rules = []struct {
				Host string `json:"host"`
			}{{Host: "app.example.com"}}
			got, _, err := derive(ing, conf)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected a failure, got name %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("derive: %v", err)
			}
			if got != tc.want {
				t.Fatalf("name = %q, want %q", got, tc.want)
			}
		})
	}

	// Two Ingresses can never derive the same name from distinct identities, but
	// if the API ever returned a duplicate the second one is skipped, not applied.
	dup := []Ingress{}
	for i := 0; i < 2; i++ {
		var ing Ingress
		ing.Metadata.Namespace, ing.Metadata.Name = "ns", "app"
		ing.Metadata.Annotations = map[string]string{model.AnnotationManaged: "true"}
		ing.Spec.Rules = []struct {
			Host string `json:"host"`
		}{{Host: "app.example.com"}}
		dup = append(dup, ing)
	}
	full := conf
	full.Enabled = true
	full.Template.TLS.CertificateRef = "wildcard"
	p := planReconcile(model.Config{}, full, dup)
	if len(p.upserts) != 1 || p.skipped != 1 {
		t.Fatalf("duplicate names: upserts=%d skipped=%d", len(p.upserts), p.skipped)
	}
}

func TestDNSPolicyFromAnnotations(t *testing.T) {
	on := &model.DNSSyncPolicy{LanDirect: true}
	tests := []struct {
		name        string
		def         *model.DNSSyncPolicy
		ann         map[string]string
		wantNil     bool
		lan, public bool
	}{
		{"no default, no annotations", nil, nil, true, false, false},
		{"template default applies", on, nil, false, true, false},
		{"annotation turns lan on", nil, map[string]string{model.AnnotationLanDirect: "true"}, false, true, false},
		{"annotation turns the default off", on, map[string]string{model.AnnotationLanDirect: "false"}, true, false, false},
		{"public cname", nil, map[string]string{model.AnnotationPublicCname: "true"}, false, false, true},
		{"both", nil, map[string]string{model.AnnotationLanDirect: "true", model.AnnotationPublicCname: "true"}, false, true, true},
		{"garbage keeps the default", on, map[string]string{model.AnnotationLanDirect: "maybe"}, false, true, false},
		{"case and space tolerated", nil, map[string]string{model.AnnotationLanDirect: " TRUE "}, false, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ing Ingress
			ing.Metadata.Namespace, ing.Metadata.Name = "ns", "app"
			ing.Metadata.Annotations = tc.ann
			conf := model.IngressDiscoverySettings{Template: model.IngressHostTemplate{DefaultDNS: tc.def}}
			got := dnsPolicy(ing, conf)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("policy = %+v, want nil (a host that publishes nothing carries no dns key)", got)
				}
				return
			}
			if got == nil {
				t.Fatal("policy = nil, want a policy")
			}
			if got.LanDirect != tc.lan || got.PublicCname != tc.public {
				t.Fatalf("policy = %+v", got)
			}
		})
	}
}

// Discovery must never publish DNS itself: it only sets the policy that the
// phase-1 reconciler acts on.
func TestDerivedHostCarriesTheDNSPolicyOnly(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("ns", "app", map[string]string{
				model.AnnotationManaged:     "true",
				model.AnnotationLanDirect:   "true",
				model.AnnotationPublicCname: "false",
			}, "app.example.com"),
		}, "")
	}
	cfg := &model.Config{}
	s := baseSettings(f)
	s.Template.DefaultDNS = &model.DNSSyncPolicy{PublicCname: true}
	settings := &model.Settings{IngressDiscovery: s}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := rec.upserts[0]
	if got.DNS == nil || !got.DNS.LanDirect || got.DNS.PublicCname {
		t.Fatalf("dns policy = %+v (annotation must override the template default per flag)", got.DNS)
	}
}

func TestReconcileDisabledIsInertAndWritesNothing(t *testing.T) {
	f := newFakeAPI(t, "tok")
	hit := false
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		hit = true
		writeList(w, nil, "")
	}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{managedHostFixture("ing-a.ns", "a.example.com")}}
	s := baseSettings(f)
	s.Enabled = false
	settings := &model.Settings{IngressDiscovery: s}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if hit {
		t.Fatal("a disabled discovery must not contact the cluster")
	}
	if rec.calls != 0 {
		t.Fatal("a disabled discovery must not touch managed hosts")
	}
	if st := d.Status(); st.Enabled {
		t.Fatalf("status = %+v", st)
	}
}

func TestReconcileApplyFailureIsReported(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("ns", "app", map[string]string{model.AnnotationManaged: "true"}, "app.example.com"),
		}, "")
	}
	cfg := &model.Config{}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{err: errors.New("certificate wildcard does not exist")}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err == nil {
		t.Fatal("a failed write must be reported")
	}
	st := d.Status()
	if !strings.Contains(st.Error, "certificate wildcard") {
		t.Fatalf("status error = %q", st.Error)
	}
	if rec.changed != 0 {
		t.Fatal("onChange must not fire when the write failed")
	}
}

func TestReconcileInvalidSettingsDoNotContactTheCluster(t *testing.T) {
	f := newFakeAPI(t, "tok")
	hit := false
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		hit = true
		writeList(w, nil, "")
	}
	s := baseSettings(f)
	s.AllowedDomainSuffixes = nil // required when enabled
	settings := &model.Settings{IngressDiscovery: s}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err == nil {
		t.Fatal("invalid settings must fail the reconcile")
	}
	if hit || rec.calls != 0 {
		t.Fatal("invalid settings must stop before any cluster call or write")
	}
}

func TestReconcileLoadFailureIsRecorded(t *testing.T) {
	d := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{}, errors.New("git is broken")
	}, nil, nil)
	if err := d.Reconcile(context.Background()); err == nil {
		t.Fatal("a load failure must be returned")
	}
	if !strings.Contains(d.Status().Error, "git is broken") {
		t.Fatalf("status = %+v", d.Status())
	}
}

// The manual endpoint refuses rather than queueing behind an in-flight run, so
// the API can answer 409 instead of parking a request-scoped goroutine.
func TestReconcileNowRefusesWhileRunning(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	d := New(func(context.Context) (model.Config, model.Settings, error) {
		once.Do(func() { close(started) })
		<-release
		return model.Config{}, model.Settings{}, nil
	}, nil, nil)

	done := make(chan error, 1)
	go func() { done <- d.ReconcileNow(context.Background()) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first reconcile never started")
	}
	if err := d.ReconcileNow(context.Background()); !errors.Is(err, ErrReconcileInProgress) {
		t.Fatalf("concurrent ReconcileNow = %v, want ErrReconcileInProgress", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if err := d.ReconcileNow(context.Background()); err != nil {
		t.Fatalf("post-run ReconcileNow: %v", err)
	}
}

func TestNilDiscovererIsInert(t *testing.T) {
	var d *Discoverer
	if st := d.Status(); st.Enabled || !st.LastRun.IsZero() {
		t.Fatal("a nil discoverer must report an empty status")
	}
	if d.Enabled() {
		t.Fatal("a nil discoverer must report disabled")
	}
	if err := d.ReconcileNow(context.Background()); err == nil {
		t.Fatal("a nil discoverer must refuse a manual reconcile")
	}
	d.Run(context.Background()) // must not panic or block
}

func TestEnabledReportsSettings(t *testing.T) {
	d := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{IngressDiscovery: model.IngressDiscoverySettings{Enabled: true}}, nil
	}, nil, nil)
	if !d.Enabled() {
		t.Fatal("Enabled() must follow settings")
	}
	bad := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{}, errors.New("nope")
	}, nil, nil)
	if bad.Enabled() {
		t.Fatal("a load failure must report disabled rather than guessing")
	}
}

// The client is cached across polls (so the token TTL means something) but
// rebuilt when the connection settings change.
func TestClientIsCachedUntilSettingsChange(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) { writeList(w, nil, "") }
	cfg := &model.Config{}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)
	built := 0
	inner := d.newClient
	d.newClient = func(c ClientConfig) (*Client, error) {
		built++
		return inner(c)
	}

	for i := 0; i < 3; i++ {
		if err := d.Reconcile(context.Background()); err != nil {
			t.Fatalf("reconcile %d: %v", i, err)
		}
	}
	if built != 1 {
		t.Fatalf("client built %d times across 3 polls, want 1", built)
	}
	settings.IngressDiscovery.Namespace = "monitoring"
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if built != 2 {
		t.Fatalf("client built %d times, want a rebuild after the settings changed", built)
	}
}
