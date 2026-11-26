package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// The discovery template check a settings write runs must also hold for every
// other way the stored config can change: an object write, a delete, a Revert,
// a RevertObject and a Restore. Otherwise the template can be left naming an
// object that is missing or unusable, the next derived host fails the batch,
// and that source's reconcile is refused on every poll. A config ALREADY
// stored in that state must still load (with a warning) and accept writes.
func TestObjectWritesKeepDiscoveryRefsResolvable(t *testing.T) {
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}
	list := func(name, def string) model.AccessList {
		return model.AccessList{ObjectMeta: model.ObjectMeta{Name: name}, DefaultAction: def,
			Rules: []model.IPRule{{CIDR: "192.0.2.0/24", Action: "allow"}}}
	}
	// The refusal text is the one SaveSettings gives for the same state.
	settingsErr := func(t *testing.T, cfg model.Config, s model.Settings) string {
		t.Helper()
		err := s.IngressDiscovery.ValidateRefs(cfg)
		if err == nil {
			t.Fatal("the bad state passes the settings check; the test is not testing anything")
		}
		return err.Error()
	}

	// setup stores lan (first allow-by-default, then deny), vpn, a certificate
	// and an enabled ingressDiscovery template using both lists in any mode.
	// It returns the commit holding the allow-by-default lan.
	setup := func(t *testing.T) (*Store, string) {
		t.Helper()
		st := newTestStore(t)
		old, err := st.Save(ctx, list("lan", model.ActionAllow), author)
		if err != nil {
			t.Fatalf("seed lan: %v", err)
		}
		for _, o := range []model.Object{list("lan", ""), list("vpn", ""), sampleCert("wildcard")} {
			if _, err := st.Save(ctx, o, author); err != nil {
				t.Fatalf("seed %s: %v", o.GetMeta().Name, err)
			}
		}
		_, s, err := st.Load(ctx)
		if err != nil {
			t.Fatal(err)
		}
		s.IngressDiscovery = model.IngressDiscoverySettings{
			Enabled:               true,
			AllowedDomainSuffixes: []string{"example.com"},
			Template: model.IngressHostTemplate{
				Upstream:        model.Upstream{Scheme: "http", Host: "192.0.2.40", Port: 80},
				TLS:             model.TLSSettings{CertificateRef: "wildcard"},
				AccessLists:     []string{"lan", "vpn"},
				AccessListsMode: model.AccessListsModeAny,
			},
		}
		if _, err := st.SaveSettings(ctx, s, author); err != nil {
			t.Fatalf("enable discovery: %v", err)
		}
		return st, old
	}

	tests := []struct {
		name string
		op   func(st *Store, old string) error
		// bad is the config the refused operation would have produced.
		bad      func(cfg model.Config) model.Config
		wantDang bool
	}{
		{"save makes a list unusable in any mode", func(st *Store, _ string) error {
			_, err := st.Save(ctx, list("lan", model.ActionAllow), author)
			return err
		}, func(cfg model.Config) model.Config {
			return withObject(&cfg, list("lan", model.ActionAllow))
		}, false},
		{"save batch makes a list unusable in any mode", func(st *Store, _ string) error {
			_, err := st.SaveBatch(ctx, []model.Object{list("lan", model.ActionAllow)}, "batch", author)
			return err
		}, func(cfg model.Config) model.Config {
			return withObject(&cfg, list("lan", model.ActionAllow))
		}, false},
		{"delete a list a template names", func(st *Store, _ string) error {
			_, err := st.Delete(ctx, "AccessList", "vpn", author)
			return err
		}, func(cfg model.Config) model.Config {
			return withoutObject(&cfg, "AccessList", "vpn")
		}, true},
		{"delete the template's certificate", func(st *Store, _ string) error {
			_, err := st.Delete(ctx, "Certificate", "wildcard", author)
			return err
		}, func(cfg model.Config) model.Config {
			return withoutObject(&cfg, "Certificate", "wildcard")
		}, true},
		{"revert one object", func(st *Store, old string) error {
			_, err := st.RevertObject(ctx, "AccessList", "lan", old, author)
			return err
		}, func(cfg model.Config) model.Config {
			return withObject(&cfg, list("lan", model.ActionAllow))
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, old := setup(t)
			cfg, s, err := st.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := settingsErr(t, tc.bad(cfg), s)
			head, _ := st.git.Head(ctx)
			err = tc.op(st, old)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want a refusal carrying the settings write's text %q", err, want)
			}
			if tc.wantDang && !errors.Is(err, ErrDanglingRef) {
				t.Fatalf("err = %v, want ErrDanglingRef", err)
			}
			if now, _ := st.git.Head(ctx); now != head {
				t.Fatalf("HEAD moved from %s to %s on a refused write", head, now)
			}
		})
	}

	// A state committed behind the store's back: it loads with a warning, it
	// does not block an unrelated write, and once repaired a Revert or Restore
	// back to it is refused.
	t.Run("stored bad state loads, accepts writes, and cannot be restored", func(t *testing.T) {
		st, _ := setup(t)
		raw := "name: lan\ndefaultAction: allow\nrules:\n  - cidr: 192.0.2.0/24\n    action: allow\n"
		if err := os.WriteFile(filepath.Join(st.Dir(), "access-lists", "lan.yaml"), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		bad, err := st.git.CommitAll(ctx, "hand edit", Author{})
		if err != nil || bad == "" {
			t.Fatalf("commit hand edit: %q %v", bad, err)
		}
		archive, err := st.Export(ctx)
		if err != nil {
			t.Fatal(err)
		}

		cfg, s, err := st.Load(ctx)
		if err != nil {
			t.Fatalf("a stored config failing the discovery check must still load: %v", err)
		}
		want := settingsErr(t, cfg, s)
		found := false
		for _, w := range cfg.ConfigWarnings {
			found = found || strings.Contains(w, strings.ReplaceAll(want, "\n", "; "))
		}
		if !found {
			t.Fatalf("warnings = %q, want one carrying %q", cfg.ConfigWarnings, want)
		}
		if _, err := st.Save(ctx, sampleHost("app"), author); err != nil {
			t.Fatalf("an unrelated write on an already failing config must go through: %v", err)
		}

		if _, err := st.Save(ctx, list("lan", ""), author); err != nil {
			t.Fatalf("repair: %v", err)
		}
		head, _ := st.git.Head(ctx)
		if _, err := st.Revert(ctx, bad, author); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Revert: %v, want a refusal carrying %q", err, want)
		}
		if _, err := st.Restore(ctx, archive, author); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Restore: %v, want a refusal carrying %q", err, want)
		}
		if now, _ := st.git.Head(ctx); now != head {
			t.Fatalf("HEAD moved from %s to %s on a refused restore", head, now)
		}
	})
}

// The two writes that used to wedge discovery through a template's location
// and upstream group: deleting a list only a location names, and disabling the
// template's upstream group. Both are refused with the settings write's text,
// and a host derived from the template would indeed fail Config.Validate.
func TestObjectWritesKeepLocationAndGroupRefsResolvable(t *testing.T) {
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}
	pool := func(disabled bool) model.UpstreamGroup {
		return model.UpstreamGroup{ObjectMeta: model.ObjectMeta{Name: "pool", Disabled: disabled},
			Upstreams: []model.GroupUpstream{{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80}}}}
	}
	vpn := model.AccessList{ObjectMeta: model.ObjectMeta{Name: "vpn"}, Rules: []model.IPRule{{CIDR: "192.0.2.0/24", Action: "allow"}}}
	tmpl := model.IngressHostTemplate{
		UpstreamGroupRef: "pool",
		TLS:              model.TLSSettings{CertificateRef: "wildcard"},
		Locations: []model.Location{{Path: "/admin", AccessLists: []string{"vpn"},
			Upstream: &model.Upstream{Scheme: "http", Host: "192.0.2.11", Port: 80}}},
	}
	tests := []struct {
		name    string
		op      func(st *Store) error
		wantErr string
	}{
		{"delete a list only a location names", func(st *Store) error {
			_, err := st.Delete(ctx, "AccessList", "vpn", author)
			return err
		}, `ingressDiscovery.template.locations: location "/admin": accessLists: references unknown accessList "vpn"`},
		{"disable the template's upstream group", func(st *Store) error {
			_, err := st.Save(ctx, pool(true), author)
			return err
		}, `ingressDiscovery.template.upstreamGroupRef: upstreamGroup "pool" is disabled`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestStore(t)
			for _, o := range []model.Object{sampleCert("wildcard"), pool(false), vpn} {
				if _, err := st.Save(ctx, o, author); err != nil {
					t.Fatalf("seed %s: %v", o.GetMeta().Name, err)
				}
			}
			_, s, err := st.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			s.IngressDiscovery = model.IngressDiscoverySettings{Enabled: true, AllowedDomainSuffixes: []string{"example.com"}, Template: tmpl}
			if _, err := st.SaveSettings(ctx, s, author); err != nil {
				t.Fatalf("enable discovery: %v", err)
			}
			if err := tc.op(st); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want a refusal containing %q", err, tc.wantErr)
			}
			cfg, _, err := st.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.ConfigWarnings) != 0 {
				t.Fatalf("warnings after a refused write: %q", cfg.ConfigWarnings)
			}
		})
	}
}

// Docker derive never stamps the template's upstream or upstream group, so a
// group an inherited Ingress profile names (Ingress discovery off) may be
// disabled, and settings writes on such an install keep working.
func TestDockerIgnoresInheritedProfileUpstreamGroup(t *testing.T) {
	ctx := context.Background()
	author := Author{Name: "admin", Email: "admin@example.com"}
	group := func(disabled bool) model.UpstreamGroup {
		return model.UpstreamGroup{ObjectMeta: model.ObjectMeta{Name: "ingress-nodes", Disabled: disabled},
			Upstreams: []model.GroupUpstream{{Upstream: model.Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80}}}}
	}
	st := newTestStore(t)
	for _, o := range []model.Object{sampleCert("wildcard"), group(false)} {
		if _, err := st.Save(ctx, o, author); err != nil {
			t.Fatalf("seed %s: %v", o.GetMeta().Name, err)
		}
	}
	_, s, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s.IngressDiscovery.Profiles = map[string]model.IngressHostTemplate{"k8s": {
		UpstreamGroupRef: "ingress-nodes",
		TLS:              model.TLSSettings{CertificateRef: "wildcard"},
	}}
	s.DockerDiscovery = model.DockerDiscoverySettings{Enabled: true, AllowedDomainSuffixes: []string{"example.com"},
		Template: model.IngressHostTemplate{TLS: model.TLSSettings{CertificateRef: "wildcard"}}}
	if _, err := st.SaveSettings(ctx, s, author); err != nil {
		t.Fatalf("enable docker discovery: %v", err)
	}
	if _, err := st.Save(ctx, group(true), author); err != nil {
		t.Fatalf("disabling a group only an ignored profile field names was refused: %v", err)
	}
	cfg, s, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ConfigWarnings) != 0 {
		t.Fatalf("warnings: %q", cfg.ConfigWarnings)
	}
	s.DockerDiscovery.PollInterval = "2m"
	if _, err := st.SaveSettings(ctx, s, author); err != nil {
		t.Fatalf("settings write refused: %v", err)
	}
}
