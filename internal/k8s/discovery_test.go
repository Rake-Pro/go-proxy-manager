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
	"unicode/utf8"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"gopkg.in/yaml.v3"
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
	// emptyCommit makes apply report "nothing was written" the way the store does
	// when every planned change turned out to be moot.
	emptyCommit bool
}

func (r *recorder) apply(_ context.Context, upserts []model.ProxyHost, deletes []string, message string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return "", r.err
	}
	r.upserts, r.deletes, r.message = upserts, deletes, message
	if r.emptyCommit {
		return "", nil
	}
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
			_, host, _, err := derive(ing, conf)
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
			got, _, _, err := derive(ing, conf)
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
			got := dnsPolicy(ing, model.IngressHostTemplate{DefaultDNS: tc.def}, model.IngressDiscoverySettings{})
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

// operatorHost is a hand-written (unlabelled) proxy host.
func operatorHost(name string, domains ...string) model.ProxyHost {
	return model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: name},
		Domains:    domains,
		Upstream:   model.Upstream{Scheme: "http", Host: "192.0.2.9", Port: 8080},
	}
}

// The DOMAIN ownership gate. Owning the derived NAME is not enough: hosts are
// routed by domain, so a tenant who can annotate an Ingress in their own
// namespace could otherwise claim an operator hostname - deriving a host whose
// name collides with nothing, but whose domain silently replaces the operator's
// SSO/access-list chain (and its TLS pinning) in the router's per-domain maps.
func TestDerivedHostCannotShadowAnOperatorDomain(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  model.Config
	}{
		{"proxy host", model.Config{ProxyHosts: []model.ProxyHost{operatorHost("sso", "sso.example.com")}}},
		{"disabled proxy host", model.Config{ProxyHosts: []model.ProxyHost{func() model.ProxyHost {
			h := operatorHost("sso", "sso.example.com")
			h.Disabled = true
			return h
		}()}}},
		{"redirect host", model.Config{RedirectHosts: []model.RedirectHost{{
			ObjectMeta:   model.ObjectMeta{Name: "sso"},
			Domains:      []string{"sso.example.com"},
			TargetDomain: "elsewhere.example.com",
		}}}},
		{"parked host", model.Config{ParkedHosts: []model.ParkedHost{{
			ObjectMeta: model.ObjectMeta{Name: "sso"},
			Domains:    []string{"sso.example.com"},
		}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, "tok")
			f.handler = func(w http.ResponseWriter, r *http.Request) {
				writeList(w, []string{
					ingressJSON("tenant", "grab", map[string]string{model.AnnotationManaged: "true"}, "sso.example.com"),
				}, "")
			}
			cfg := tc.cfg
			settings := &model.Settings{IngressDiscovery: baseSettings(f)}
			rec := &recorder{}
			d := newDiscoverer(f, &cfg, settings, rec)

			if err := d.Reconcile(context.Background()); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			if rec.calls != 0 {
				t.Fatalf("a domain an operator host already serves must never be written (calls=%d)", rec.calls)
			}
			st := d.Status()
			if st.Skipped != 1 || len(st.Hosts) != 1 || st.Hosts[0].Action != ActionSkipped {
				t.Fatalf("status = %+v", st)
			}
			if !strings.Contains(st.Hosts[0].Reason, "already claimed by proxy host \"sso\"") {
				t.Fatalf("skip reason must name the owning host, got %q", st.Hosts[0].Reason)
			}
		})
	}
}

// The apex of an allowed suffix is claimable by an exact-match Ingress, so the
// gate has to cover it too.
func TestDerivedHostCannotShadowTheApexDomain(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("tenant", "grab", map[string]string{model.AnnotationManaged: "true"}, "example.com"),
		}, "")
	}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{operatorHost("apex", "example.com")}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("the apex must not be claimable (calls=%d)", rec.calls)
	}
}

// Two annotated Ingresses that derive DIFFERENT names but the same domain: the
// first by derived name wins and the second is skipped, so the batch can never
// carry a duplicate domain into the config validator (which would fail the whole
// reconcile and freeze every unrelated change with it).
func TestTwoIngressesCannotClaimTheSameDomain(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("ns", "zeta", map[string]string{model.AnnotationManaged: "true"}, "app.example.com"),
			ingressJSON("ns", "alpha", map[string]string{model.AnnotationManaged: "true"}, "app.example.com"),
		}, "")
	}
	cfg := &model.Config{}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 || rec.upserts[0].Name != "ing-alpha.ns" {
		t.Fatalf("exactly the first derived host may take the domain, got %+v", rec.upserts)
	}
	st := d.Status()
	if st.Skipped != 1 {
		t.Fatalf("the second claimant must be skipped, status = %+v", st)
	}
}

// A managed host whose Ingress went bad is protected from deletion; while it is
// protected it must keep its domains, so a different Ingress cannot take the
// hostname out from under it.
func TestProtectedManagedHostKeepsItsDomainClaim(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			// Underivable: protects ing-a.ns without refreshing it.
			ingressJSON("ns", "a", map[string]string{model.AnnotationManaged: "true"}, "*.example.com"),
			ingressJSON("ns", "zz", map[string]string{model.AnnotationManaged: "true"}, "app.example.com"),
		}, "")
	}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{managedHostFixture("ing-a.ns", "app.example.com")}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("nothing may be written: the domain is held by a protected host (calls=%d, upserts=%+v)", rec.calls, rec.upserts)
	}
}

// A managed host being REPLACED releases its domains: renaming the Ingress must
// hand the hostname over rather than deadlock on its own claim.
func TestRenamedIngressHandsItsDomainOver(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("ns", "new", map[string]string{model.AnnotationManaged: "true"}, "app.example.com"),
		}, "")
	}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{managedHostFixture("ing-old.ns", "app.example.com")}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 || rec.upserts[0].Name != "ing-new.ns" {
		t.Fatalf("upserts = %+v", rec.upserts)
	}
	if len(rec.deletes) != 1 || rec.deletes[0] != "ing-old.ns" {
		t.Fatalf("deletes = %+v", rec.deletes)
	}
}

// A LIST that decodes to zero items only because the body was not an IngressList
// must freeze, not delete. Each of these bodies is a plausible 200 from
// something that is not the API server (a mistyped apiURL onto another HTTPS
// service, a mesh or gateway envelope, a Status reply).
func TestMalformedListBodyDeletesNothing(t *testing.T) {
	bodies := map[string]string{
		"null":         `null`,
		"empty":        `{}`,
		"status":       `{"kind":"Status","apiVersion":"v1","status":"Success"}`,
		"null items":   `{"kind":"IngressList","items":null}`,
		"items typo":   `{"kind":"IngressList","itemz":[]}`,
		"wrong kind":   `{"kind":"ServiceList","items":[]}`,
		"html-ish 200": `{"ok":true}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			f := newFakeAPI(t, "tok")
			f.handler = func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, body)
			}
			cfg := &model.Config{ProxyHosts: []model.ProxyHost{
				managedHostFixture("ing-a.ns", "a.example.com"),
				managedHostFixture("ing-b.ns", "b.example.com"),
			}}
			settings := &model.Settings{IngressDiscovery: baseSettings(f)}
			rec := &recorder{}
			d := newDiscoverer(f, cfg, settings, rec)

			err := d.Reconcile(context.Background())
			if err == nil {
				t.Fatal("a response that is not an IngressList must be an error, not an empty list")
			}
			if rec.calls != 0 {
				t.Fatalf("a malformed list must delete nothing (calls=%d, deletes=%+v)", rec.calls, rec.deletes)
			}
			if st := d.Status(); st.Error == "" || st.LastSuccess != (time.Time{}) {
				t.Fatalf("the run must be recorded as a failure, status = %+v", st)
			}
		})
	}
}

// A namespace containing a dot would make the derived "<name>.<namespace>"
// ambiguous, so it is refused rather than trusted to have been enforced upstream.
func TestNamespaceMustBeADNSLabel(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("ns.evil", "app", map[string]string{model.AnnotationManaged: "true"}, "app.example.com"),
		}, "")
	}
	cfg := &model.Config{}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("an ambiguous namespace must derive nothing (calls=%d)", rec.calls)
	}
	st := d.Status()
	if st.Skipped != 1 || !strings.Contains(st.Hosts[0].Reason, "DNS-1123") {
		t.Fatalf("status = %+v", st)
	}
	if st.Hosts[0].Name != "" {
		t.Fatalf("an ambiguous name must not be reported as a protectable host, got %q", st.Hosts[0].Name)
	}
}

// A write that turned out to change nothing on disk returns an empty commit. The
// reload, webhook and DNS trigger must not fire on it: there is no revision to
// point at and nothing reloaded.
func TestEmptyCommitDoesNotNotify(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) { writeList(w, nil, "") }
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{managedHostFixture("ing-a.ns", "a.example.com")}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{emptyCommit: true}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 1 || len(rec.deletes) != 1 {
		t.Fatalf("the delete should still have been attempted (calls=%d deletes=%+v)", rec.calls, rec.deletes)
	}
	if rec.changed != 0 {
		t.Fatalf("onChange must not fire without a commit (fired %d times)", rec.changed)
	}
	if st := d.Status(); st.Commit != "" {
		t.Fatalf("status commit = %q, want empty", st.Commit)
	}
}

// One reconcile's list is bounded, so a hung API server cannot hold the reconcile
// mutex for the page limit times the per-request timeout.
func TestListIsBoundedByAPerReconcileDeadline(t *testing.T) {
	prev := listDeadline
	listDeadline = 50 * time.Millisecond
	t.Cleanup(func() { listDeadline = prev })

	f := newFakeAPI(t, "tok")
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{managedHostFixture("ing-a.ns", "a.example.com")}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	start := time.Now()
	err := d.Reconcile(context.Background())
	if err == nil {
		t.Fatal("a list that outruns the deadline must fail the reconcile")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("reconcile took %s; the deadline did not apply", elapsed)
	}
	if rec.calls != 0 {
		t.Fatalf("a timed-out list must write nothing (calls=%d)", rec.calls)
	}
}

// The capability probe is hit on every admin page load, so it must not turn into
// a full config load (and a store read-lock behind an in-flight reconcile commit)
// each time.
func TestEnabledIsAnsweredFromTheCachedFlag(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) { writeList(w, nil, "") }
	settings := model.Settings{IngressDiscovery: baseSettings(f)}
	var loads int
	d := New(
		func(context.Context) (model.Config, model.Settings, error) {
			loads++
			return model.Config{}, settings, nil
		},
		(&recorder{}).apply, nil)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	after := loads
	for i := 0; i < 5; i++ {
		if !d.Enabled() {
			t.Fatal("discovery is enabled in settings")
		}
	}
	if loads != after {
		t.Fatalf("Enabled() loaded the config %d extra times; it must answer from the cache", loads-after)
	}
}

// A derived host must be able to name an UpstreamGroup rather than a single
// backend. The cluster ingress controller runs on every node, so pinning
// discovery to one address would leave every discovered service single-node
// while the operator's hand-written hosts keep failing over.
func TestDerivedHostInheritsUpstreamGroup(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"}, "grafana.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	settings.IngressDiscovery.Template.Upstream = model.Upstream{}
	settings.IngressDiscovery.Template.UpstreamGroupRef = "k8s-nodes"

	rec := &recorder{}
	cfg := &model.Config{}
	d := newDiscoverer(f, cfg, settings, rec)
	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	h := rec.upserts[0]
	if h.UpstreamGroupRef != "k8s-nodes" {
		t.Fatalf("upstreamGroupRef = %q, want the template's group", h.UpstreamGroupRef)
	}
	if h.Upstream != (model.Upstream{}) {
		t.Fatalf("upstream = %+v, want empty when a group is named (they are mutually exclusive)", h.Upstream)
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("derived host must be valid: %v", err)
	}
}

// ---------- discovery profiles ----------

// profileSettings adds the two named profiles the docs use as the worked
// example: one deliberately public with a rate limit and NO access list, one
// SSO-gated behind the VPN list. The default template stays as baseSettings has
// it (no middlewares, no access lists).
func profileSettings(f *fakeAPI) model.IngressDiscoverySettings {
	s := baseSettings(f)
	s.Profiles = map[string]model.IngressHostTemplate{
		"public-ratelimited": {
			Upstream:    model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:         model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true, HTTP2: true},
			Middlewares: []string{"rate-limit"},
		},
		"sso-internal": {
			UpstreamGroupRef:  "k8s-nodes",
			TLS:               model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true, MinTLSVersion: "1.3"},
			WebsocketsUpgrade: true,
			Middlewares:       []string{"sso", "rate-limit"},
			AccessLists:       []string{"home-vpn"},
			DefaultDNS:        &model.DNSSyncPolicy{LanDirect: true},
		},
	}
	return s
}

// REGRESSION: an Ingress that names no profile must behave exactly as it did
// before profiles existed - the default `template` chain, verbatim. Every
// deployed config is in this state, so this is the compatibility gate.
func TestReconcileWithoutProfileAnnotationUsesTheDefaultTemplate(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"}, "grafana.example.com"),
			// An empty annotation value is "absent", not "a profile called ''".
			ingressJSON("monitoring", "loki", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "",
			}, "loki.example.com"),
			// So is a whitespace-only one.
			ingressJSON("monitoring", "tempo", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "   ",
			}, "tempo.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	settings.IngressDiscovery.Template.Middlewares = []string{"rate-limit"}
	settings.IngressDiscovery.Template.AccessLists = []string{"home-vpn"}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 3 || len(rec.deletes) != 0 {
		t.Fatalf("upserts=%d deletes=%d, want all three derived from the default template", len(rec.upserts), len(rec.deletes))
	}
	for _, h := range rec.upserts {
		if strings.Join(h.Middlewares, ",") != "rate-limit" || strings.Join(h.AccessLists, ",") != "home-vpn" {
			t.Fatalf("%s got mw=%v al=%v, want the DEFAULT template's chain", h.Name, h.Middlewares, h.AccessLists)
		}
		if h.Upstream != (model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80}) {
			t.Fatalf("%s upstream = %+v, want the default template's", h.Name, h.Upstream)
		}
	}
	for _, r := range d.Status().Hosts {
		if r.Profile != model.DefaultProfileName {
			t.Fatalf("status for %s reports profile %q, want %q", r.Name, r.Profile, model.DefaultProfileName)
		}
	}
}

// A named profile is applied VERBATIM - the whole chain, not a merge with the
// default. A merge would mean the default's access list silently leaks onto a
// profile that is public on purpose.
func TestReconcileAppliesTheNamedProfileVerbatim(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "sso-internal",
			}, "paste.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	// A default that would be WRONG for this host, so inheriting any of it shows up.
	settings.IngressDiscovery.Template.Middlewares = []string{"joplin-login-lan"}
	settings.IngressDiscovery.Template.AccessLists = []string{"lan-only"}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	h := rec.upserts[0]
	want := settings.IngressDiscovery.Profiles["sso-internal"]
	if strings.Join(h.Middlewares, ",") != "sso,rate-limit" {
		t.Fatalf("middlewares = %v, want the profile's chain in order", h.Middlewares)
	}
	if strings.Join(h.AccessLists, ",") != "home-vpn" {
		t.Fatalf("accessLists = %v, want the profile's", h.AccessLists)
	}
	if h.TLS != want.TLS {
		t.Fatalf("tls = %+v, want the profile's %+v", h.TLS, want.TLS)
	}
	if h.UpstreamGroupRef != "k8s-nodes" || h.Upstream != (model.Upstream{}) {
		t.Fatalf("upstream = %+v / group = %q, want the profile's group and no single upstream", h.Upstream, h.UpstreamGroupRef)
	}
	if !h.WebsocketsUpgrade {
		t.Fatal("websocketsUpgrade must come from the profile")
	}
	if h.DNS == nil || !h.DNS.LanDirect {
		t.Fatalf("dns = %+v, want the profile's defaultDNS", h.DNS)
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("derived host must be valid: %v", err)
	}
	st := d.Status()
	if len(st.Hosts) != 1 || st.Hosts[0].Profile != "sso-internal" {
		t.Fatalf("status must report the resolved profile for the audit trail: %+v", st.Hosts)
	}
}

// Naming a profile that does not exist NEVER creates a host: it must not get the
// default chain (that is the silent-downgrade bug) and must not be adopted with a
// partial chain.
func TestReconcileSkipsAnUndefinedProfileWithNoExistingHost(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "no-such-profile",
			}, "paste.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	settings.IngressDiscovery.Template.Middlewares = []string{"should-not-appear"}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("apply was called %d times; an unresolvable profile must never CREATE a host", rec.calls)
	}
	st := d.Status()
	if st.Skipped != 1 || st.Created != 0 || st.Updated != 0 || st.Deleted != 0 {
		t.Fatalf("status = %+v, want exactly one skip", st)
	}
	r := st.Hosts[0]
	if r.Action != ActionSkipped {
		t.Fatalf("action = %q, want %q", r.Action, ActionSkipped)
	}
	if !strings.Contains(r.Reason, "no-such-profile") || !strings.Contains(r.Reason, "not defined") {
		t.Fatalf("reason = %q, want it to name the undefined profile", r.Reason)
	}
	if r.Profile != "no-such-profile" {
		t.Fatalf("status must report the REQUESTED profile so the operator can see the typo, got %q", r.Profile)
	}
}

// REVOCATION. An Ingress whose profile no longer resolves must not keep serving
// the chain it was last given: freezing it would let a tenant pin a chain the
// operator has just tightened or retired simply by pointing the annotation at a
// name that does not exist. The host is DISABLED instead - preserved, reversible,
// but off the data plane - and it is still never deleted.
func TestUnresolvableProfileDisablesTheExistingHost(t *testing.T) {
	tests := []struct {
		name       string
		annotation string
		// retire drops every profile from settings, which is what "the operator
		// retired the profile" and "the operator cleared the profile rows in the
		// UI" both look like from here.
		retire bool
	}{
		{name: "tenant points at an undefined name", annotation: "public-open-x"},
		{name: "operator retired the profile", annotation: "sso-internal", retire: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, "tok")
			f.handler = func(w http.ResponseWriter, r *http.Request) {
				writeList(w, []string{
					ingressJSON("web", "paste", map[string]string{
						model.AnnotationManaged: "true", model.AnnotationProfile: tc.annotation,
					}, "paste.example.com"),
				}, "")
			}
			settings := &model.Settings{IngressDiscovery: profileSettings(f)}
			settings.IngressDiscovery.Template.Middlewares = []string{"should-not-appear"}
			if tc.retire {
				settings.IngressDiscovery.Profiles = nil
			}

			// The live host, still serving the pre-revocation (unauthenticated) chain.
			existing := managedHostFixture("ing-paste.web", "paste.example.com")
			existing.Middlewares = []string{"rate-limit"}
			cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
			rec := &recorder{}
			d := newDiscoverer(f, cfg, settings, rec)

			if err := d.Reconcile(context.Background()); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			if len(rec.upserts) != 1 || len(rec.deletes) != 0 {
				t.Fatalf("upserts=%d deletes=%d, want exactly one disabling update and no delete", len(rec.upserts), len(rec.deletes))
			}
			got := rec.upserts[0]
			if got.Name != "ing-paste.web" || !got.Disabled {
				t.Fatalf("upsert = %+v, want ing-paste.web with disabled: true", got)
			}
			// Nothing else about the object may change: this is a hold, not a rewrite.
			if strings.Join(got.Middlewares, ",") != "rate-limit" || len(got.Domains) != 1 || got.Domains[0] != "paste.example.com" {
				t.Fatalf("upsert = %+v, want the stored object untouched apart from disabled", got)
			}
			st := d.Status()
			if st.Updated != 1 || st.Created != 0 || st.Deleted != 0 || st.Skipped != 0 {
				t.Fatalf("status = %+v, want exactly one update", st)
			}
			if len(st.Hosts) != 1 || st.Hosts[0].Action != ActionUpdated ||
				!strings.Contains(st.Hosts[0].Reason, "disabled") {
				t.Fatalf("hosts = %+v, want one update whose reason says the host was disabled", st.Hosts)
			}
			if st.Hosts[0].Profile != tc.annotation {
				t.Fatalf("profile = %q, want the REQUESTED name %q", st.Hosts[0].Profile, tc.annotation)
			}

			// Steady state: once disabled, the next reconcile writes nothing at all.
			cfg.ProxyHosts[0] = got
			rec2 := &recorder{}
			d2 := newDiscoverer(f, cfg, settings, rec2)
			if err := d2.Reconcile(context.Background()); err != nil {
				t.Fatalf("second reconcile: %v", err)
			}
			if rec2.calls != 0 {
				t.Fatalf("apply called %d times on the second run; an already-disabled host must not churn", rec2.calls)
			}
		})
	}
}

// The fail-closed rule is scoped to PROFILE RESOLUTION. Every other derive
// failure - a malformed hostname, an unusable derived name - still freezes: the
// operator's policy has not changed there, and disabling would let any tenant
// take their own service offline with a one-character manifest edit.
func TestMalformedIngressStillFreezesRatherThanDisabling(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "sso-internal",
			}, "*.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	existing := managedHostFixture("ing-paste.web", "paste.example.com")
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("apply was called %d times; a malformed Ingress must leave its host exactly as it is", rec.calls)
	}
	st := d.Status()
	if st.Skipped != 1 || st.Updated != 0 || st.Deleted != 0 {
		t.Fatalf("status = %+v, want one skip and no write", st)
	}
	if st.Managed != 1 {
		t.Fatalf("managed = %d, want the existing host still counted (the skip protects it)", st.Managed)
	}
}

// An unresolvable profile must never take an OPERATOR-authored host down: the
// disable is an ownership-scoped write like every other, so a hand-written host
// that happens to carry the derived name is left completely alone.
func TestUnresolvableProfileNeverDisablesAnUnownedHost(t *testing.T) {
	conf := model.IngressDiscoverySettings{
		Enabled:               true,
		AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:      model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
		},
	}
	var ing Ingress
	ing.Metadata.Namespace, ing.Metadata.Name = "web", "paste"
	ing.Metadata.Annotations = map[string]string{
		model.AnnotationManaged: "true", model.AnnotationProfile: "no-such-profile",
	}
	ing.Spec.Rules = []struct {
		Host string `json:"host"`
	}{{Host: "paste.example.com"}}

	// Same name, but hand-written: no managed-by label.
	operator := model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "ing-paste.web"},
		Domains:    []string{"paste.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.9", Port: 80},
	}
	p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{operator}}, conf, []Ingress{ing})
	if len(p.upserts) != 0 || len(p.deletes) != 0 {
		t.Fatalf("plan = +%d -%d, want neither: the host is not one discovery owns", len(p.upserts), len(p.deletes))
	}
}

// The profile name is untrusted input from a cluster manifest. Every junk value
// must either match a defined profile EXACTLY or be skipped - never a partial
// match, never a fall back to the default, never a panic.
func TestUndefinedProfileNamesAreAllRejected(t *testing.T) {
	conf := model.IngressDiscoverySettings{
		Enabled:               true,
		AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream:    model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:         model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
			AccessLists: []string{"home-vpn"},
		},
		Profiles: map[string]model.IngressHostTemplate{
			"public-ratelimited": {
				Upstream:    model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:         model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
				Middlewares: []string{"rate-limit"},
			},
		},
	}
	tests := []struct {
		name    string
		value   string
		wantMW  string // "" = the Ingress must be skipped
		wantAL  string
		profile string
	}{
		{"absent", "", "", "home-vpn", model.DefaultProfileName},
		{"whitespace only", "  \t ", "", "home-vpn", model.DefaultProfileName},
		{"exact", "public-ratelimited", "rate-limit", "", "public-ratelimited"},
		{"padded exact", " public-ratelimited ", "rate-limit", "", "public-ratelimited"},
		{"unknown", "nope", "skip", "", ""},
		{"prefix of a real profile", "public", "skip", "", ""},
		{"suffix of a real profile", "ratelimited", "skip", "", ""},
		{"real profile with a suffix", "public-ratelimited-x", "skip", "", ""},
		{"case folded", "PUBLIC-RATELIMITED", "skip", "", ""},
		{"the reserved default name", model.DefaultProfileName, "skip", "", ""},
		{"path traversal", "../../etc/passwd", "skip", "", ""},
		{"path traversal onto a profile", "../public-ratelimited", "skip", "", ""},
		{"glob", "*", "skip", "", ""},
		{"comma list", "public-ratelimited,sso", "skip", "", ""},
		{"newline injection", "public-ratelimited\nsso", "skip", "", ""},
		{"null byte", "public-ratelimited\x00", "skip", "", ""},
		{"template injection", "${public-ratelimited}", "skip", "", ""},
		{"unicode lookalike", "рublic-ratelimited", "skip", "", ""}, // Cyrillic 'р'
		{"very long", strings.Repeat("z", 16384), "skip", "", ""},
		{"long with a real name inside", strings.Repeat("z", 8192) + "public-ratelimited", "skip", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ing Ingress
			ing.Metadata.Namespace, ing.Metadata.Name = "web", "app"
			ing.Metadata.Annotations = map[string]string{
				model.AnnotationManaged: "true",
				model.AnnotationProfile: tc.value,
			}
			ing.Spec.Rules = []struct {
				Host string `json:"host"`
			}{{Host: "app.example.com"}}

			p := planReconcile(model.Config{}, conf, []Ingress{ing})
			if tc.wantMW == "skip" {
				if len(p.upserts) != 0 || p.skipped != 1 {
					t.Fatalf("upserts=%d skipped=%d, want the Ingress skipped", len(p.upserts), p.skipped)
				}
				if len(p.results) != 1 || !strings.Contains(p.results[0].Reason, "not defined") {
					t.Fatalf("results = %+v, want a not-defined skip reason", p.results)
				}
				return
			}
			if len(p.upserts) != 1 {
				t.Fatalf("upserts = %d, want 1 (%q must resolve)", len(p.upserts), tc.value)
			}
			h := p.upserts[0]
			if strings.Join(h.Middlewares, ",") != tc.wantMW || strings.Join(h.AccessLists, ",") != tc.wantAL {
				t.Fatalf("mw=%v al=%v, want mw=%q al=%q", h.Middlewares, h.AccessLists, tc.wantMW, tc.wantAL)
			}
			if p.results[0].Profile != tc.profile {
				t.Fatalf("profile = %q, want %q", p.results[0].Profile, tc.profile)
			}
		})
	}
}

// Two Ingresses selecting different profiles in one reconcile must each get
// their own chain - profile resolution is per-Ingress, not per-run.
func TestReconcileDerivesTwoProfilesInOneRun(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "public-ratelimited",
			}, "paste.example.com"),
			ingressJSON("media", "radarr", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "sso-internal",
			}, "radarr.example.com"),
			// And one on the default, to prove all three coexist.
			ingressJSON("monitoring", "grafana", map[string]string{
				model.AnnotationManaged: "true",
			}, "grafana.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 3 {
		t.Fatalf("upserts = %d, want 3", len(rec.upserts))
	}
	got := map[string]model.ProxyHost{}
	for _, h := range rec.upserts {
		got[h.Name] = h
	}
	// Deliberately public: a rate limit and NO access list. The default's access
	// list must not have leaked onto it.
	pub := got["ing-paste.web"]
	if strings.Join(pub.Middlewares, ",") != "rate-limit" || len(pub.AccessLists) != 0 {
		t.Fatalf("public host: mw=%v al=%v, want rate-limit and no access list", pub.Middlewares, pub.AccessLists)
	}
	sso := got["ing-radarr.media"]
	if strings.Join(sso.Middlewares, ",") != "sso,rate-limit" || strings.Join(sso.AccessLists, ",") != "home-vpn" {
		t.Fatalf("sso host: mw=%v al=%v", sso.Middlewares, sso.AccessLists)
	}
	if sso.UpstreamGroupRef != "k8s-nodes" {
		t.Fatalf("sso host upstreamGroupRef = %q, want the profile's", sso.UpstreamGroupRef)
	}
	def := got["ing-grafana.monitoring"]
	if len(def.Middlewares) != 0 || len(def.AccessLists) != 0 || def.UpstreamGroupRef != "" {
		t.Fatalf("default host picked up a profile's chain: %+v", def)
	}
	profiles := map[string]string{}
	for _, r := range d.Status().Hosts {
		profiles[r.Name] = r.Profile
	}
	want := map[string]string{
		"ing-paste.web":          "public-ratelimited",
		"ing-radarr.media":       "sso-internal",
		"ing-grafana.monitoring": model.DefaultProfileName,
	}
	for name, w := range want {
		if profiles[name] != w {
			t.Fatalf("status profile for %s = %q, want %q", name, profiles[name], w)
		}
	}
}

// ProfileRules are strictly stronger than the annotation: a matching rule wins
// even when the Ingress's own annotation asks for something else.
func TestReconcileHonoursProfileRulesOverTheAnnotation(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			// The tenant asks for the wide-open public-ratelimited profile via the
			// annotation; the rule on this namespace overrides it to sso-internal.
			ingressJSON("media", "radarr", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "public-ratelimited",
			}, "radarr.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	settings.IngressDiscovery.ProfileRules = []model.IngressProfileRule{
		{Namespace: "media", Profile: "sso-internal"},
	}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	got := rec.upserts[0]
	if strings.Join(got.Middlewares, ",") != "sso,rate-limit" {
		t.Fatalf("middlewares = %v, want the RULED profile's chain, not the annotation's", got.Middlewares)
	}
	if d.Status().Hosts[0].Profile != "sso-internal" {
		t.Fatalf("status profile = %q, want sso-internal", d.Status().Hosts[0].Profile)
	}
}

// A rule can match on labels instead of (or as well as) namespace.
func TestReconcileProfileRuleMatchesOnLabels(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSONWithLabels("web", "internal-tool", map[string]string{"team": "core"}, map[string]string{
				model.AnnotationManaged: "true",
			}, "tool.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	settings.IngressDiscovery.ProfileRules = []model.IngressProfileRule{
		{MatchLabels: map[string]string{"team": "core"}, Profile: "sso-internal"},
	}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 || strings.Join(rec.upserts[0].Middlewares, ",") != "sso,rate-limit" {
		t.Fatalf("upserts = %+v, want the label-matched profile's chain", rec.upserts)
	}
}

// "rules-only" mode must never consult the annotation: an Ingress that matches
// no rule gets the default template, exactly as if it named no profile at all -
// even when its own annotation names a real profile.
func TestReconcileRulesOnlyIgnoresTheAnnotation(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("other", "app", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "sso-internal",
			}, "app.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	settings.IngressDiscovery.ProfileSelection = model.ProfileSelectionRulesOnly
	settings.IngressDiscovery.ProfileRules = []model.IngressProfileRule{
		{Namespace: "media", Profile: "sso-internal"}, // does not match this Ingress's namespace
	}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	got := rec.upserts[0]
	if len(got.Middlewares) != 0 || len(got.AccessLists) != 0 {
		t.Fatalf("host = %+v, want the plain default template - the annotation must never have been consulted", got)
	}
	if d.Status().Hosts[0].Profile != model.DefaultProfileName {
		t.Fatalf("status profile = %q, want %q", d.Status().Hosts[0].Profile, model.DefaultProfileName)
	}
}

// A profile's own allowedDomainSuffixes NARROWS the global list: a host it
// derives is rejected for a hostname the global list alone would have allowed.
func TestReconcilePerProfileAllowedDomainSuffixes(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "public-ratelimited",
			}, "paste.example.com", "internal.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	prof := settings.IngressDiscovery.Profiles["public-ratelimited"]
	prof.AllowedDomainSuffixes = []string{"paste.example.com"}
	settings.IngressDiscovery.Profiles["public-ratelimited"] = prof
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	if strings.Join(rec.upserts[0].Domains, ",") != "paste.example.com" {
		t.Fatalf("domains = %v, want only the one covered by the profile's narrower list", rec.upserts[0].Domains)
	}
}

// The domain-shadowing gate is orthogonal to profiles: naming a profile must not
// buy an Ingress a domain an operator-authored host already serves.
func TestProfileDoesNotBypassDomainShadowing(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("tenant", "grab", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "public-ratelimited",
			}, "sso.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "sso"},
		Domains:    []string{"sso.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "10.0.0.9", Port: 80},
	}}}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("apply was called; a profile must not let an Ingress shadow an operator host")
	}
	st := d.Status()
	if st.Skipped != 1 || !strings.Contains(st.Hosts[0].Reason, "already claimed") {
		t.Fatalf("status = %+v, want a domain-claim skip", st.Hosts)
	}
}

// The rejected profile name is echoed into the status payload and the log, and
// it comes from a cluster annotation, so it must be bounded - and cut on a rune
// boundary, or a truncated multi-byte sequence would put invalid UTF-8 into the
// JSON response.
func TestEllipsizeBoundsUntrustedNames(t *testing.T) {
	tests := []struct {
		in  string
		max int
	}{
		{"short", 64},
		{strings.Repeat("a", 64), 64},
		{strings.Repeat("a", 65), 64},
		{strings.Repeat("z", 16384), 64},
		{strings.Repeat("é", 100), 9}, // 2 bytes per rune: the cut lands mid-rune
		{strings.Repeat("é世", 50), 7},
	}
	for _, tc := range tests {
		got := ellipsize(tc.in, tc.max)
		if len(got) > tc.max+3 {
			t.Fatalf("ellipsize(len %d, %d) returned %d bytes", len(tc.in), tc.max, len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("ellipsize produced invalid UTF-8 from a %d-byte input", len(tc.in))
		}
		if len(tc.in) <= tc.max && got != tc.in {
			t.Fatalf("ellipsize must not touch a string within the bound, got %q", got)
		}
	}
}

// End to end: a 16 KiB annotation value must not put 16 KiB into the status.
func TestUndefinedProfileReasonIsBounded(t *testing.T) {
	conf := model.IngressDiscoverySettings{
		Enabled:               true,
		AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:      model.TLSSettings{CertificateRef: "wildcard"},
		},
	}
	var ing Ingress
	ing.Metadata.Namespace, ing.Metadata.Name = "web", "app"
	ing.Metadata.Annotations = map[string]string{
		model.AnnotationManaged: "true",
		model.AnnotationProfile: strings.Repeat("q", 16384),
	}
	ing.Spec.Rules = []struct {
		Host string `json:"host"`
	}{{Host: "app.example.com"}}

	p := planReconcile(model.Config{}, conf, []Ingress{ing})
	if p.skipped != 1 || len(p.results) != 1 {
		t.Fatalf("plan = %+v, want one skip", p.results)
	}
	if len(p.results[0].Profile) > 128 || len(p.results[0].Reason) > 512 {
		t.Fatalf("status echoed %d bytes of profile and %d of reason from one annotation",
			len(p.results[0].Profile), len(p.results[0].Reason))
	}
}

// TLSSettings is a value but its ClientAuth is a POINTER. Every derived host must
// get its own copy, or a later writer of one host's mTLS requirement reaches into
// the settings object and every other host derived from it.
func TestDerivedHostsDoNotShareTheClientAuthPointer(t *testing.T) {
	conf := model.IngressDiscoverySettings{
		Enabled:               true,
		AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS: model.TLSSettings{
				CertificateRef: "wildcard",
				ForceSSL:       true,
				ClientAuth:     &model.ClientAuth{CARef: "corp-ca", Mode: "require"},
			},
		},
	}
	mk := func(ns, name, host string) Ingress {
		var ing Ingress
		ing.Metadata.Namespace, ing.Metadata.Name = ns, name
		ing.Metadata.Annotations = map[string]string{model.AnnotationManaged: "true"}
		ing.Spec.Rules = []struct {
			Host string `json:"host"`
		}{{Host: host}}
		return ing
	}
	p := planReconcile(model.Config{}, conf, []Ingress{
		mk("web", "a", "a.example.com"), mk("web", "b", "b.example.com"),
	})
	if len(p.upserts) != 2 {
		t.Fatalf("upserts = %d, want 2", len(p.upserts))
	}
	a, b := p.upserts[0], p.upserts[1]
	if a.TLS.ClientAuth == nil || b.TLS.ClientAuth == nil {
		t.Fatal("both derived hosts must carry the template's mTLS requirement")
	}
	if a.TLS.ClientAuth == b.TLS.ClientAuth {
		t.Fatal("two derived hosts share one *ClientAuth")
	}
	if a.TLS.ClientAuth == conf.Template.TLS.ClientAuth {
		t.Fatal("a derived host aliases the settings object's *ClientAuth")
	}
	// Mutating one must not be visible anywhere else.
	a.TLS.ClientAuth.Mode = "optional"
	if b.TLS.ClientAuth.Mode != "require" || conf.Template.TLS.ClientAuth.Mode != "require" {
		t.Fatalf("mutating one host's clientAuth leaked: b=%+v settings=%+v", b.TLS.ClientAuth, conf.Template.TLS.ClientAuth)
	}
}

// The disable is reversible: it preserves the object, so re-adding the profile
// puts the host straight back on the data plane at the next reconcile. That is
// what makes failing closed acceptable rather than destructive.
func TestReAddingTheProfileReEnablesTheHost(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "sso-internal",
			}, "paste.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: profileSettings(f)}
	disabled := managedHostFixture("ing-paste.web", "paste.example.com")
	disabled.Disabled = true
	// Marked as DISCOVERY's own disable (not an operator's): only that label is
	// what makes a clean re-derive free to clear it.
	disabled.Labels[model.DisabledByLabel] = model.DisabledByIngressDiscovery
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{disabled}}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want the host rewritten from the restored profile", len(rec.upserts))
	}
	if rec.upserts[0].Disabled {
		t.Fatal("the host must be re-enabled once its profile resolves again")
	}
	if _, has := rec.upserts[0].Labels[model.DisabledByLabel]; has {
		t.Fatal("a re-enabled host must not keep discovery's disabled-by label")
	}
	if strings.Join(rec.upserts[0].Middlewares, ",") != "sso,rate-limit" {
		t.Fatalf("middlewares = %v, want the restored profile's chain", rec.upserts[0].Middlewares)
	}
}

// An operator hand-disabling a managed host - the obvious move when an app has
// to come offline NOW - must not be undone by the very next poll just because
// the Ingress still derives cleanly. `disabled: true` with no
// gpm.rake.pro/disabled-by label is operator-owned state.
func TestOperatorDisabledHostIsNotReEnabled(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"}, "grafana.example.com"),
		}, "")
	}
	existing := managedHostFixture("ing-grafana.monitoring", "grafana.example.com")
	existing.DisplayName = "monitoring/grafana"
	existing.Disabled = true // operator hand-disabled; no discovery label
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("an operator-disabled host with nothing else changed must write nothing, calls=%d", rec.calls)
	}
	st := d.Status()
	if len(st.Hosts) != 1 || st.Hosts[0].Action != ActionUnchanged {
		t.Fatalf("hosts = %+v, want unchanged (still disabled)", st.Hosts)
	}
}

// A cluster tenant must never be able to re-enable an operator-disabled host by
// editing their Ingress: even a real change (new domain) is applied with
// disabled preserved.
func TestOperatorDisabledHostStaysDisabledAcrossOtherChanges(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"},
				"grafana.example.com", "metrics.example.com"),
		}, "")
	}
	existing := managedHostFixture("ing-grafana.monitoring", "grafana.example.com")
	existing.Disabled = true
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want the domain change written", len(rec.upserts))
	}
	if !rec.upserts[0].Disabled {
		t.Fatal("an operator-disabled host must stay disabled even when other fields change")
	}
	if strings.Join(rec.upserts[0].Domains, ",") != "grafana.example.com,metrics.example.com" {
		t.Fatalf("domains = %v, want the new set applied", rec.upserts[0].Domains)
	}
	if d.Status().Hosts[0].Reason == "" {
		t.Fatal("the status should explain that the disabled state was preserved")
	}
}

// A host discovery disabled itself (unresolvable profile, label set) is exactly
// the opposite: it is expected to clear on the very next clean derive, and the
// disabling upsert must not corrupt the labels of the config it read from.
func TestDiscoveryDisableSetsTheOwnershipLabelWithoutMutatingTheSourceHost(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "no-such-profile",
			}, "paste.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	existing := managedHostFixture("ing-paste.web", "paste.example.com")
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 || !rec.upserts[0].Disabled {
		t.Fatalf("upserts = %+v, want one disabling upsert", rec.upserts)
	}
	if rec.upserts[0].Labels[model.DisabledByLabel] != model.DisabledByIngressDiscovery {
		t.Fatalf("labels = %v, want the disabled-by label set to discovery's own value", rec.upserts[0].Labels)
	}
	// The upsert's Labels map must be a fresh copy, not an alias of cfg's: cfg is
	// the caller's config, read under no lock, and must come back unmutated.
	if _, has := cfg.ProxyHosts[0].Labels[model.DisabledByLabel]; has {
		t.Fatal("the source config's host must not have been mutated in place")
	}
}

// Cutting a service over to discovery used to SILENTLY drop its robotsNoIndex:
// the template had no such field, so the derived host came back without one and
// the only workaround was a headers middleware setting X-Robots-Tag - a second
// mechanism for something the model already expresses. Both robotsNoIndex and
// timeouts must now be inherited verbatim, exactly like the middleware chain.
func TestDerivedHostInheritsRobotsAndTimeouts(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{model.AnnotationManaged: "true"}, "paste.example.com"),
		}, "")
	}
	s := baseSettings(f)
	s.Template.RobotsNoIndex = true
	s.Template.Timeouts = &model.HostTimeouts{ConnectSeconds: 3, ReadSeconds: 7}
	s.Template.Tags = []string{"cluster", "discovered"}
	settings := &model.Settings{IngressDiscovery: s}
	rec := &recorder{}
	d := newDiscoverer(f, &model.Config{}, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	h := rec.upserts[0]
	if !h.RobotsNoIndex {
		t.Fatal("robotsNoIndex must be inherited from the template")
	}
	if h.Timeouts == nil || h.Timeouts.ConnectSeconds != 3 || h.Timeouts.ReadSeconds != 7 {
		t.Fatalf("timeouts = %+v, want the template's", h.Timeouts)
	}
	if strings.Join(h.Tags, ",") != "cluster,discovered" {
		t.Fatalf("tags = %v, want the template's in order", h.Tags)
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("derived host must be valid: %v", err)
	}
}

// Profiles ARE IngressHostTemplate, so the same fields have to arrive through a
// named profile without any extra plumbing. Proven, not assumed.
func TestDerivedHostInheritsRobotsAndTimeoutsFromAProfile(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "sso-internal",
			}, "paste.example.com"),
		}, "")
	}
	s := profileSettings(f)
	prof := s.Profiles["sso-internal"]
	prof.RobotsNoIndex = true
	prof.Timeouts = &model.HostTimeouts{ConnectSeconds: 5}
	prof.Tags = []string{"sso"}
	s.Profiles["sso-internal"] = prof
	// A default that would be WRONG for this host, so inheriting any of it shows up.
	s.Template.RobotsNoIndex = false
	s.Template.Timeouts = &model.HostTimeouts{ConnectSeconds: 90, ReadSeconds: 90}
	s.Template.Tags = []string{"default-chain"}
	settings := &model.Settings{IngressDiscovery: s}
	rec := &recorder{}
	d := newDiscoverer(f, &model.Config{}, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	h := rec.upserts[0]
	if !h.RobotsNoIndex {
		t.Fatal("robotsNoIndex must come from the resolved profile")
	}
	if h.Timeouts == nil || h.Timeouts.ConnectSeconds != 5 || h.Timeouts.ReadSeconds != 0 {
		t.Fatalf("timeouts = %+v, want the profile's verbatim (never merged with the template's)", h.Timeouts)
	}
	if strings.Join(h.Tags, ",") != "sso" {
		t.Fatalf("tags = %v, want the profile's verbatim", h.Tags)
	}
}

// A template that sets neither must produce a host that carries neither - not a
// zero-valued `timeouts: {}` / `tags: []` on every derived object. ProxyHost.DNS
// needed a pointer for exactly this reason, so the encoded form is asserted, not
// just the Go value.
func TestDerivedHostOmitsUnsetRobotsAndTimeouts(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{model.AnnotationManaged: "true"}, "paste.example.com"),
		}, "")
	}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, &model.Config{}, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	h := rec.upserts[0]
	if h.RobotsNoIndex || h.Timeouts != nil || h.Tags != nil {
		t.Fatalf("unset template fields leaked onto the derived host: robots=%v timeouts=%+v tags=%v", h.RobotsNoIndex, h.Timeouts, h.Tags)
	}
	b, err := yaml.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"robotsNoIndex", "timeouts", "tags"} {
		if strings.Contains(string(b), key) {
			t.Fatalf("derived host YAML carries a spurious %q key:\n%s", key, b)
		}
	}
}

// The aliasing rule the *ClientAuth copy established, extended to the two
// reference types this change adds: a derived host must never share backing
// memory with the settings object or with another derived host.
func TestDerivedHostsDoNotShareTheTimeoutsOrTagsBacking(t *testing.T) {
	conf := model.IngressDiscoverySettings{
		Enabled:               true,
		AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:      model.TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
			Timeouts: &model.HostTimeouts{ConnectSeconds: 3, ReadSeconds: 7},
			Tags:     []string{"cluster"},
		},
	}
	mk := func(ns, name, host string) Ingress {
		var ing Ingress
		ing.Metadata.Namespace, ing.Metadata.Name = ns, name
		ing.Metadata.Annotations = map[string]string{model.AnnotationManaged: "true"}
		ing.Spec.Rules = []struct {
			Host string `json:"host"`
		}{{Host: host}}
		return ing
	}
	p := planReconcile(model.Config{}, conf, []Ingress{
		mk("web", "a", "a.example.com"), mk("web", "b", "b.example.com"),
	})
	if len(p.upserts) != 2 {
		t.Fatalf("upserts = %d, want 2", len(p.upserts))
	}
	a, b := p.upserts[0], p.upserts[1]
	if a.Timeouts == nil || b.Timeouts == nil {
		t.Fatal("both derived hosts must carry the template's timeouts")
	}
	if a.Timeouts == b.Timeouts || a.Timeouts == conf.Template.Timeouts {
		t.Fatal("derived hosts share one *HostTimeouts with each other or with settings")
	}
	a.Timeouts.ConnectSeconds = 99
	a.Tags[0] = "mutated"
	if b.Timeouts.ConnectSeconds != 3 || conf.Template.Timeouts.ConnectSeconds != 3 {
		t.Fatalf("mutating one host's timeouts leaked: b=%+v settings=%+v", b.Timeouts, conf.Template.Timeouts)
	}
	if b.Tags[0] != "cluster" || conf.Template.Tags[0] != "cluster" {
		t.Fatalf("mutating one host's tags leaked: b=%v settings=%v", b.Tags, conf.Template.Tags)
	}
}

// Plan must report EXACTLY the decisions Reconcile would take - create,
// update, delete, skip, and every derived host's action/domains/profile -
// without writing anything.
func TestPlanMirrorsReconcileDecisionsAndWritesNothing(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			// Creates a new host.
			ingressJSON("web", "new", map[string]string{model.AnnotationManaged: "true"}, "new.example.com"),
			// Updates an existing one (new domain).
			ingressJSON("web", "changed", map[string]string{model.AnnotationManaged: "true"},
				"changed.example.com", "extra.example.com"),
			// Unknown profile: skip, no existing host to disable.
			ingressJSON("web", "bad-profile", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "no-such-profile",
			}, "bad.example.com"),
		}, "")
	}
	changed := managedHostFixture("ing-changed.web", "changed.example.com")
	changed.DisplayName = "web/changed"
	gone := managedHostFixture("ing-gone.web", "gone.example.com") // no Ingress derives this any more
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{changed, gone}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}

	planRec := &recorder{}
	planD := newDiscoverer(f, cfg, settings, planRec)
	plan, err := planD.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if planRec.calls != 0 {
		t.Fatalf("Plan must never call apply, calls=%d", planRec.calls)
	}
	if !plan.Enabled {
		t.Fatal("plan.enabled = false, want true")
	}
	if plan.Created != 1 || plan.Updated != 1 || plan.Deleted != 1 || plan.Skipped != 1 {
		t.Fatalf("plan = %+v, want +1 ~1 -1 skip1", plan)
	}

	reconcileRec := &recorder{}
	reconcileD := newDiscoverer(f, cfg, settings, reconcileRec)
	if err := reconcileD.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	status := reconcileD.Status()

	if plan.Created != status.Created || plan.Updated != status.Updated ||
		plan.Deleted != status.Deleted || plan.Skipped != status.Skipped ||
		plan.Discovered != status.Discovered || plan.Managed != status.Managed {
		t.Fatalf("plan counts %+v disagree with reconcile status %+v", plan, status)
	}
	if len(plan.Hosts) != len(status.Hosts) {
		t.Fatalf("plan has %d hosts, reconcile status has %d", len(plan.Hosts), len(status.Hosts))
	}
	byName := map[string]HostResult{}
	for _, h := range status.Hosts {
		byName[h.Name] = h
	}
	for _, ph := range plan.Hosts {
		h, ok := byName[ph.Name]
		if !ok {
			t.Fatalf("plan named host %q that reconcile's status does not have", ph.Name)
		}
		if h.Action != ph.Action || h.Profile != ph.Profile || strings.Join(h.Domains, ",") != strings.Join(ph.Domains, ",") {
			t.Fatalf("host %q: plan=%+v reconcile=%+v", ph.Name, ph, h)
		}
	}
}

// A disabled discovery block must not be contacted for a preview either.
func TestPlanDisabledIsInertAndContactsNothing(t *testing.T) {
	f := newFakeAPI(t, "tok")
	hit := false
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		hit = true
		writeList(w, nil, "")
	}
	s := baseSettings(f)
	s.Enabled = false
	settings := &model.Settings{IngressDiscovery: s}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	plan, err := d.Plan(context.Background())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if hit {
		t.Fatal("a disabled discovery must not contact the cluster for a preview")
	}
	if plan.Enabled {
		t.Fatalf("plan = %+v, want enabled=false", plan)
	}
}

// Invalid settings fail the preview exactly as they fail a real reconcile,
// before any cluster contact.
func TestPlanInvalidSettingsDoNotContactTheCluster(t *testing.T) {
	f := newFakeAPI(t, "tok")
	hit := false
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		hit = true
		writeList(w, nil, "")
	}
	s := baseSettings(f)
	s.AllowedDomainSuffixes = nil
	settings := &model.Settings{IngressDiscovery: s}
	cfg := &model.Config{}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if _, err := d.Plan(context.Background()); err == nil {
		t.Fatal("invalid settings must fail the plan")
	}
	if hit {
		t.Fatal("invalid settings must stop before any cluster call")
	}
}

// Like ReconcileNow, Plan refuses rather than queues behind a run already in
// flight: a preview of a moving target is worth less than an honest 409.
func TestPlanRefusesWhileAReconcileIsRunning(t *testing.T) {
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
	if _, err := d.Plan(context.Background()); !errors.Is(err, ErrReconcileInProgress) {
		t.Fatalf("Plan while running = %v, want ErrReconcileInProgress", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
}

// derive() must read every annotation - opt-in, profile, and the two DNS
// booleans - and write the managed-by label, all under the settings-
// configured prefix, and NOT under the default one, even when an Ingress
// carries stray default-prefix annotations too (an operator migrating prefixes
// gradually should never have the OLD prefix accidentally still work).
func TestDeriveUsesCustomAnnotationPrefix(t *testing.T) {
	conf := model.IngressDiscoverySettings{
		AnnotationPrefix:      "acme.corp.internal",
		AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 80},
			TLS:      model.TLSSettings{CertificateRef: "wildcard"},
		},
		Profiles: map[string]model.IngressHostTemplate{
			"special": {
				Upstream: model.Upstream{Scheme: "http", Host: "h2", Port: 81},
				TLS:      model.TLSSettings{CertificateRef: "wildcard"},
			},
		},
	}
	var ing Ingress
	ing.Metadata.Namespace, ing.Metadata.Name = "ns", "app"
	ing.Metadata.Annotations = map[string]string{
		"acme.corp.internal/profile":      "special",
		"acme.corp.internal/lan-direct":   "true",
		"acme.corp.internal/public-cname": "true",
		// Stray default-prefix annotations: must be ignored entirely under a
		// custom prefix, not merged or treated as a fallback.
		model.AnnotationProfile:     "does-not-exist",
		model.AnnotationLanDirect:   "false",
		model.AnnotationPublicCname: "false",
	}
	ing.Spec.Rules = []struct {
		Host string `json:"host"`
	}{{Host: "app.example.com"}}

	name, host, prof, err := derive(ing, conf)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if name != "ing-app.ns" {
		t.Fatalf("name = %q", name)
	}
	if prof != "special" {
		t.Fatalf("profile = %q, want the custom-prefix annotation to have been read (special)", prof)
	}
	if host.Upstream != (model.Upstream{Scheme: "http", Host: "h2", Port: 81}) {
		t.Fatalf("upstream = %+v, want the \"special\" profile's (the default-prefix annotation must be ignored)", host.Upstream)
	}
	wantLabels := map[string]string{"acme.corp.internal/managed-by": model.ManagedByIngressDiscovery}
	if !mapsEqual(host.Labels, wantLabels) {
		t.Fatalf("labels = %v, want %v (current-prefix managed-by only)", host.Labels, wantLabels)
	}
	if host.DNS == nil || !host.DNS.LanDirect || !host.DNS.PublicCname {
		t.Fatalf("dns = %+v, want both flags on from the custom-prefix annotations", host.DNS)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Under a custom prefix, opt-in itself is read from the custom key: an Ingress
// carrying only the DEFAULT-prefix "managed" annotation is invisible, exactly
// like one carrying no annotation at all.
func TestReconcileOptInIsReadUnderTheConfiguredPrefix(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			// Default-prefix opt-in only: invisible under a custom prefix.
			ingressJSON("default", "old-style", map[string]string{model.AnnotationManaged: "true"}, "old.example.com"),
			// Custom-prefix opt-in: this is the one discovery must see.
			ingressJSON("default", "new-style", map[string]string{"acme.corp.internal/managed": "true"}, "new.example.com"),
		}, "")
	}
	settings := baseSettings(f)
	settings.AnnotationPrefix = "acme.corp.internal"
	cfg := &model.Config{}
	sVal := &model.Settings{IngressDiscovery: settings}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, sVal, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts=%d, want exactly the custom-prefix Ingress", len(rec.upserts))
	}
	if rec.upserts[0].Name != "ing-new-style.default" {
		t.Fatalf("upserted host = %q, want the custom-prefix Ingress's", rec.upserts[0].Name)
	}
	if rec.upserts[0].Labels["acme.corp.internal/managed-by"] != model.ManagedByIngressDiscovery {
		t.Fatalf("labels = %v, want the custom-prefix managed-by", rec.upserts[0].Labels)
	}
}

// The safety property changing the prefix relies on: a proxy host discovery
// wrote under the OLD prefix is NOT recognised as managed once the prefix is
// changed (without annotationPrefixMigrate), so it is left exactly as it is -
// neither deleted, nor silently adopted/overwritten. This is what makes the
// settings-write refusal (model.IngressDiscoverySettings.ValidateRefs) a real
// guarantee rather than a paper one: even if that check were bypassed, the
// reconciler itself never touches a stale-labelled host.
func TestOwnershipDoesNotCarryOverOnPrefixChangeWithoutMigrate(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) { writeList(w, nil, "") } // nothing derives any more
	stale := managedHostFixture("ing-app.default", "app.example.com")                  // labelled under the DEFAULT prefix
	settings := baseSettings(f)
	settings.AnnotationPrefix = "acme.corp.internal" // changed, migrate NOT set
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{stale}}
	sVal := &model.Settings{IngressDiscovery: settings}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, sVal, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("a stale-prefix host must be neither deleted nor rewritten once ownership is unrecognised (calls=%d)", rec.calls)
	}
	st := d.Status()
	if st.Managed != 0 {
		t.Fatalf("status.Managed = %d, want 0 (the stale host is not recognised as owned under the new prefix)", st.Managed)
	}
}

// annotationPrefixMigrate:true is the opposite of the previous test: the same
// stale-prefix host, with its Ingress still deriving cleanly under the NEW
// prefix's opt-in annotation, is relabelled onto the new prefix as an ordinary
// update - in the SAME single commit as everything else that reconcile writes,
// with the old prefix's label gone from the result.
func TestAnnotationPrefixMigrateRelabelsInOneCommit(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("default", "app", map[string]string{"acme.corp.internal/managed": "true"}, "app.example.com"),
		}, "")
	}
	stale := managedHostFixture("ing-app.default", "app.example.com") // still under the OLD (default) prefix
	settings := baseSettings(f)
	settings.AnnotationPrefix = "acme.corp.internal"
	settings.AnnotationPrefixMigrate = true
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{stale}}
	sVal := &model.Settings{IngressDiscovery: settings}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, sVal, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 1 {
		t.Fatalf("calls=%d, want exactly one commit for the whole relabel", rec.calls)
	}
	if len(rec.upserts) != 1 || len(rec.deletes) != 0 {
		t.Fatalf("upserts=%d deletes=%d, want one relabelling update and no deletes", len(rec.upserts), len(rec.deletes))
	}
	got := rec.upserts[0]
	if got.Name != "ing-app.default" {
		t.Fatalf("relabelled host = %q", got.Name)
	}
	wantLabels := map[string]string{"acme.corp.internal/managed-by": model.ManagedByIngressDiscovery}
	if !mapsEqual(got.Labels, wantLabels) {
		t.Fatalf("labels after migrate = %v, want ONLY the new prefix's managed-by (old key must be gone)", got.Labels)
	}
	st := d.Status()
	if len(st.Hosts) != 1 || st.Hosts[0].Action != ActionUpdated {
		t.Fatalf("status.Hosts = %+v, want a single Updated result", st.Hosts)
	}
}

// The fail-closed disable path relabels too: a stale-prefix managed host whose
// Ingress now names an undefined profile is disabled AND relabelled onto the
// new prefix in the same write, with the old managed-by/disabled-by keys gone.
func TestAnnotationPrefixMigrateRelabelsOnFailClosedDisable(t *testing.T) {
	stale := managedHostFixture("ing-app.default", "app.example.com")
	conf := model.IngressDiscoverySettings{
		Enabled:                 true,
		AnnotationPrefix:        "acme.corp.internal",
		AnnotationPrefixMigrate: true,
		AllowedDomainSuffixes:   []string{"example.com"},
		Template: model.IngressHostTemplate{
			Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:      model.TLSSettings{CertificateRef: "wildcard"},
		},
	}
	var ing Ingress
	ing.Metadata.Namespace, ing.Metadata.Name = "default", "app"
	ing.Metadata.Annotations = map[string]string{
		"acme.corp.internal/managed": "true",
		"acme.corp.internal/profile": "does-not-exist",
	}
	ing.Spec.Rules = []struct {
		Host string `json:"host"`
	}{{Host: "app.example.com"}}

	p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{stale}}, conf, []Ingress{ing})
	if len(p.upserts) != 1 {
		t.Fatalf("upserts=%d, want exactly the disabled+relabelled host", len(p.upserts))
	}
	got := p.upserts[0]
	if !got.Disabled {
		t.Fatalf("host must be fail-closed disabled, got %+v", got)
	}
	wantLabels := map[string]string{
		"acme.corp.internal/managed-by":  model.ManagedByIngressDiscovery,
		"acme.corp.internal/disabled-by": model.DisabledByIngressDiscovery,
	}
	if !mapsEqual(got.Labels, wantLabels) {
		t.Fatalf("labels = %v, want only the new prefix's pair", got.Labels)
	}
}

// A per-host stripResponseHeaders list on a discovery-managed host is rebuilt
// from the template on every reconcile, so without a template field the
// operator's list is reverted - with a git commit - on every poll. It must be
// inherited verbatim, from the default template and from a named profile
// (profiles ARE IngressHostTemplate, proven rather than assumed).
func TestDerivedHostInheritsStripResponseHeaders(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{model.AnnotationManaged: "true"}, "paste.example.com"),
		}, "")
	}
	s := baseSettings(f)
	s.Template.StripResponseHeaders = []string{"Server", "X-Powered-By"}
	settings := &model.Settings{IngressDiscovery: s}
	rec := &recorder{}
	d := newDiscoverer(f, &model.Config{}, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(rec.upserts))
	}
	h := rec.upserts[0]
	if strings.Join(h.StripResponseHeaders, ",") != "Server,X-Powered-By" {
		t.Fatalf("stripResponseHeaders = %v, want the template's in order", h.StripResponseHeaders)
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("derived host must be valid: %v", err)
	}
}

func TestDerivedHostInheritsStripResponseHeadersFromAProfile(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("web", "paste", map[string]string{
				model.AnnotationManaged: "true", model.AnnotationProfile: "sso-internal",
			}, "paste.example.com"),
		}, "")
	}
	s := profileSettings(f)
	prof := s.Profiles["sso-internal"]
	prof.StripResponseHeaders = []string{"X-AspNet-Version"}
	s.Profiles["sso-internal"] = prof
	// A default that would be WRONG for this host, so inheriting it shows up.
	s.Template.StripResponseHeaders = []string{"X-Default-Only"}
	settings := &model.Settings{IngressDiscovery: s}
	rec := &recorder{}
	d := newDiscoverer(f, &model.Config{}, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	h := rec.upserts[0]
	if strings.Join(h.StripResponseHeaders, ",") != "X-AspNet-Version" {
		t.Fatalf("stripResponseHeaders = %v, want the profile's, not the default template's", h.StripResponseHeaders)
	}
}

// maintenance is operator-owned runtime state, like disabled: no Ingress
// annotation derives it, so a reconcile must carry the stored value forward
// rather than reset it. Without that, the next poll would put a host back into
// service while someone is still working on its backend.
func TestMaintenanceFlagSurvivesReconcile(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"},
				"grafana.example.com", "metrics.example.com"),
		}, "")
	}
	existing := managedHostFixture("ing-grafana.monitoring", "grafana.example.com")
	existing.Maintenance = true
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(rec.upserts) != 1 {
		t.Fatalf("upserts = %d, want the domain change written", len(rec.upserts))
	}
	if !rec.upserts[0].Maintenance {
		t.Fatal("a host in maintenance must stay in maintenance across a reconcile that rewrites it")
	}
}

// A host in maintenance whose Ingress is otherwise unchanged must produce NO
// write at all: if the carried-forward flag did not participate in the
// steady-state comparison, every poll would commit a spurious revision.
func TestMaintenanceFlagIsSteadyState(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{model.AnnotationManaged: "true"},
				"grafana.example.com"),
		}, "")
	}
	existing := managedHostFixture("ing-grafana.monitoring", "grafana.example.com")
	existing.DisplayName = "monitoring/grafana"
	existing.Maintenance = true
	cfg := &model.Config{ProxyHosts: []model.ProxyHost{existing}}
	settings := &model.Settings{IngressDiscovery: baseSettings(f)}
	rec := &recorder{}
	d := newDiscoverer(f, cfg, settings, rec)

	if err := d.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("a host in maintenance with nothing else changed must write nothing, calls=%d", rec.calls)
	}
	if st := d.Status(); len(st.Hosts) != 1 || st.Hosts[0].Action != ActionUnchanged {
		t.Fatalf("hosts = %+v, want unchanged", st.Hosts)
	}
}
