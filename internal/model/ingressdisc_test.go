package model

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func validIngressDiscovery() IngressDiscoverySettings {
	return IngressDiscoverySettings{
		Enabled:               true,
		APIURL:                "https://k8s.example.lan:6443",
		TokenFile:             "/run/secrets/gpm-k8s-token",
		CAFile:                "/run/secrets/gpm-k8s-ca.crt",
		PollInterval:          "60s",
		AllowedDomainSuffixes: []string{"example.com"},
		Template: IngressHostTemplate{
			Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:      TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
		},
	}
}

func TestIngressDiscoveryValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*IngressDiscoverySettings)
		wantErr string
	}{
		{"valid", func(*IngressDiscoverySettings) {}, ""},
		{"disabled skips every check", func(d *IngressDiscoverySettings) {
			*d = IngressDiscoverySettings{Enabled: false, APIURL: "not a url"}
		}, ""},
		{"empty apiURL is in-cluster", func(d *IngressDiscoverySettings) { d.APIURL = "" }, ""},
		{"plain http apiURL", func(d *IngressDiscoverySettings) { d.APIURL = "http://k8s.example.lan:6443" }, "https"},
		{"apiURL without host", func(d *IngressDiscoverySettings) { d.APIURL = "https://" }, "https"},
		{"relative tokenFile", func(d *IngressDiscoverySettings) { d.TokenFile = "token" }, "tokenFile must be an absolute path"},
		{"relative caFile", func(d *IngressDiscoverySettings) { d.CAFile = "../ca.crt" }, "caFile must be an absolute path"},
		{"empty token/ca are in-cluster", func(d *IngressDiscoverySettings) { d.TokenFile, d.CAFile = "", "" }, ""},
		{"bad namespace", func(d *IngressDiscoverySettings) { d.Namespace = "Not A Namespace" }, "namespace"},
		{"good namespace", func(d *IngressDiscoverySettings) { d.Namespace = "kube-system" }, ""},
		{"empty annotationPrefix is the default", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = "" }, ""},
		{"custom annotationPrefix accepted", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = "acme.corp.internal" }, ""},
		{"single-label annotationPrefix accepted", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = "gpm" }, ""},
		{"annotationPrefix with a slash rejected", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = "gpm.rake.pro/x" }, "annotationPrefix"},
		{"annotationPrefix with uppercase rejected", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = "GPM.rake.pro" }, "annotationPrefix"},
		{"annotationPrefix with a leading dot rejected", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = ".gpm.rake.pro" }, "annotationPrefix"},
		{"annotationPrefix with a trailing dot rejected", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = "gpm.rake.pro." }, "annotationPrefix"},
		{"annotationPrefix with a double dot rejected", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = "gpm..pro" }, "annotationPrefix"},
		{"annotationPrefix with a space rejected", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = "gpm rake pro" }, "annotationPrefix"},
		{"annotationPrefix too long rejected", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = strings.Repeat("a", 254) }, "annotationPrefix"},
		{"annotationPrefix exactly 253 chars accepted", func(d *IngressDiscoverySettings) { d.AnnotationPrefix = strings.Repeat("a", 253) }, ""},
		{"label selector newline", func(d *IngressDiscoverySettings) { d.LabelSelector = "a=b\nc=d" }, "newlines"},
		{"unparseable interval", func(d *IngressDiscoverySettings) { d.PollInterval = "soon" }, "pollInterval"},
		{"interval too small", func(d *IngressDiscoverySettings) { d.PollInterval = "1s" }, "at least 15s"},
		{"empty interval is the default", func(d *IngressDiscoverySettings) { d.PollInterval = "" }, ""},
		{"suffixes required", func(d *IngressDiscoverySettings) { d.AllowedDomainSuffixes = nil }, "allowedDomainSuffixes is required"},
		{"suffix must be a domain", func(d *IngressDiscoverySettings) { d.AllowedDomainSuffixes = []string{"*.example.com"} }, "not a valid domain suffix"},
		{"leading dot suffix accepted", func(d *IngressDiscoverySettings) { d.AllowedDomainSuffixes = []string{".example.com"} }, ""},
		{"upstream required", func(d *IngressDiscoverySettings) { d.Template.Upstream = Upstream{} }, "template.upstream"},
		{"upstream port range", func(d *IngressDiscoverySettings) { d.Template.Upstream.Port = 70000 }, "template.upstream"},
		{"upstreamGroupRef replaces upstream", func(d *IngressDiscoverySettings) {
			d.Template.Upstream = Upstream{}
			d.Template.UpstreamGroupRef = "k8s-nodes"
		}, ""},
		{"upstream and group are mutually exclusive", func(d *IngressDiscoverySettings) {
			d.Template.UpstreamGroupRef = "k8s-nodes"
		}, "mutually exclusive"},
		{"upstreamGroupRef name shape", func(d *IngressDiscoverySettings) {
			d.Template.Upstream = Upstream{}
			d.Template.UpstreamGroupRef = "Not A Name"
		}, "template.upstreamGroupRef"},
		{"certificateRef required", func(d *IngressDiscoverySettings) { d.Template.TLS.CertificateRef = "" }, "certificateRef is required"},
		{"certificateRef name shape", func(d *IngressDiscoverySettings) { d.Template.TLS.CertificateRef = "Wild Card" }, "certificateRef"},
		{"template tls validated", func(d *IngressDiscoverySettings) { d.Template.TLS.MinTLSVersion = "1.1" }, "minTLSVersion"},
		{"clientAuth needs forceSSL", func(d *IngressDiscoverySettings) {
			d.Template.TLS.ForceSSL = false
			d.Template.TLS.ClientAuth = &ClientAuth{CARef: "corp"}
		}, "forceSSL"},
		{"middleware name shape", func(d *IngressDiscoverySettings) { d.Template.Middlewares = []string{"Bad Name"} }, "template.middlewares[0]"},
		{"access list name shape", func(d *IngressDiscoverySettings) { d.Template.AccessLists = []string{""} }, "template.accessLists[0]"},
		{"middleware/access refs accepted", func(d *IngressDiscoverySettings) {
			d.Template.Middlewares = []string{"sso"}
			d.Template.AccessLists = []string{"lan-only"}
		}, ""},
		{"template timeouts accepted", func(d *IngressDiscoverySettings) {
			d.Template.Timeouts = &HostTimeouts{ConnectSeconds: 3, ReadSeconds: 7}
		}, ""},
		{"template robotsNoIndex and tags accepted", func(d *IngressDiscoverySettings) {
			d.Template.RobotsNoIndex = true
			d.Template.Tags = []string{"cluster"}
		}, ""},
		{"template connect timeout out of range", func(d *IngressDiscoverySettings) {
			d.Template.Timeouts = &HostTimeouts{ConnectSeconds: 3601}
		}, "template: timeouts.connectSeconds 3601 out of range (0-3600)"},
		{"template read timeout negative", func(d *IngressDiscoverySettings) {
			d.Template.Timeouts = &HostTimeouts{ReadSeconds: -1}
		}, "template: timeouts.readSeconds -1 out of range (0-3600)"},

		// A named profile is a full chain an untrusted Ingress can select, so it is
		// held to EXACTLY the template's standard - and at settings-write time, not
		// at reconcile time where an operator would never see the failure.
		{"valid profile accepted", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"public-ratelimited": {
				Upstream:    Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:         TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
				Middlewares: []string{"rate-limit"},
			}}
		}, ""},
		{"profile without certificateRef", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso-internal": {
				Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			}}
		}, `profiles["sso-internal"].tls.certificateRef is required`},
		{"profile with both upstream and group", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso-internal": {
				Upstream:         Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				UpstreamGroupRef: "k8s-nodes",
				TLS:              TLSSettings{CertificateRef: "wildcard"},
			}}
		}, `profiles["sso-internal"]: upstream and upstreamGroupRef are mutually exclusive`},
		{"profile with a bad middleware name", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso-internal": {
				Upstream:    Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:         TLSSettings{CertificateRef: "wildcard"},
				Middlewares: []string{"Bad Name"},
			}}
		}, `profiles["sso-internal"].middlewares[0]`},
		{"profile with a bad access list name", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso-internal": {
				Upstream:    Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:         TLSSettings{CertificateRef: "wildcard"},
				AccessLists: []string{""},
			}}
		}, `profiles["sso-internal"].accessLists[0]`},
		{"profile with an invalid upstream", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso-internal": {
				Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 70000},
				TLS:      TLSSettings{CertificateRef: "wildcard"},
			}}
		}, `profiles["sso-internal"].upstream`},
		{"profile tls validated", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso-internal": {
				Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:      TLSSettings{CertificateRef: "wildcard", MinTLSVersion: "1.1"},
			}}
		}, `profiles["sso-internal"].tls`},
		{"profile timeouts validated", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso-internal": {
				Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:      TLSSettings{CertificateRef: "wildcard"},
				Timeouts: &HostTimeouts{ReadSeconds: 4000},
			}}
		}, `profiles["sso-internal"]: timeouts.readSeconds 4000 out of range (0-3600)`},
		{"profile name shape", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"Not A Name": {
				Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:      TLSSettings{CertificateRef: "wildcard"},
			}}
		}, "profiles"},
		{"profile may not be called template", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{DefaultProfileName: {
				Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:      TLSSettings{CertificateRef: "wildcard"},
			}}
		}, "reserved"},

		// Per-profile allowedDomainSuffixes: narrowing is fine, widening is not.
		{"template suffix override, valid subset", func(d *IngressDiscoverySettings) {
			d.AllowedDomainSuffixes = []string{"example.com"}
			d.Template.AllowedDomainSuffixes = []string{"apps.example.com"}
		}, ""},
		{"template suffix override, exact match to global is a valid subset", func(d *IngressDiscoverySettings) {
			d.AllowedDomainSuffixes = []string{"example.com"}
			d.Template.AllowedDomainSuffixes = []string{"example.com"}
		}, ""},
		{"template suffix override widens the global list", func(d *IngressDiscoverySettings) {
			d.AllowedDomainSuffixes = []string{"apps.example.com"}
			d.Template.AllowedDomainSuffixes = []string{"example.com"}
		}, "not covered by the global allowedDomainSuffixes"},
		{"template suffix override names a different domain entirely", func(d *IngressDiscoverySettings) {
			d.AllowedDomainSuffixes = []string{"example.com"}
			d.Template.AllowedDomainSuffixes = []string{"example.net"}
		}, "not covered by the global allowedDomainSuffixes"},
		{"template suffix override must itself be a valid domain", func(d *IngressDiscoverySettings) {
			d.Template.AllowedDomainSuffixes = []string{"*.example.com"}
		}, "not a valid domain suffix"},
		{"profile suffix override, valid subset", func(d *IngressDiscoverySettings) {
			d.AllowedDomainSuffixes = []string{"example.com"}
			d.Profiles = map[string]IngressHostTemplate{"public": {
				Upstream:              Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:                   TLSSettings{CertificateRef: "wildcard"},
				AllowedDomainSuffixes: []string{"public.example.com"},
			}}
		}, ""},
		{"profile suffix override widens the global list", func(d *IngressDiscoverySettings) {
			d.AllowedDomainSuffixes = []string{"internal.example.com"}
			d.Profiles = map[string]IngressHostTemplate{"public": {
				Upstream:              Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:                   TLSSettings{CertificateRef: "wildcard"},
				AllowedDomainSuffixes: []string{"example.com"},
			}}
		}, `profiles["public"].allowedDomainSuffixes "example.com" is not covered`},

		// profileSelection.
		{"profileSelection empty is the default", func(d *IngressDiscoverySettings) { d.ProfileSelection = "" }, ""},
		{"profileSelection annotation-or-rules accepted", func(d *IngressDiscoverySettings) {
			d.ProfileSelection = ProfileSelectionAnnotationOrRules
		}, ""},
		{"profileSelection rules-only accepted", func(d *IngressDiscoverySettings) {
			d.ProfileSelection = ProfileSelectionRulesOnly
		}, ""},
		{"profileSelection rejects an unknown value", func(d *IngressDiscoverySettings) {
			d.ProfileSelection = "bogus"
		}, "profileSelection must be"},

		// profileRules: operator-side selection, strictly stronger than the
		// annotation, so it is validated as strictly as everything else here.
		{"rule targeting the default template", func(d *IngressDiscoverySettings) {
			d.ProfileRules = []IngressProfileRule{{Namespace: "monitoring", Profile: DefaultProfileName}}
		}, ""},
		{"rule targeting a defined profile", func(d *IngressDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso-internal": {
				Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:      TLSSettings{CertificateRef: "wildcard"},
			}}
			d.ProfileRules = []IngressProfileRule{{MatchLabels: map[string]string{"team": "core"}, Profile: "sso-internal"}}
		}, ""},
		{"rule targeting an undefined profile", func(d *IngressDiscoverySettings) {
			d.ProfileRules = []IngressProfileRule{{Namespace: "monitoring", Profile: "no-such-profile"}}
		}, `profileRules[0].profile "no-such-profile" is not defined`},
		{"rule with no profile at all", func(d *IngressDiscoverySettings) {
			d.ProfileRules = []IngressProfileRule{{Namespace: "monitoring"}}
		}, "profileRules[0].profile is required"},
		{"rule with a bad namespace", func(d *IngressDiscoverySettings) {
			d.ProfileRules = []IngressProfileRule{{Namespace: "Not A Namespace", Profile: DefaultProfileName}}
		}, "profileRules[0].namespace"},
		{"rule with an empty matchLabels key", func(d *IngressDiscoverySettings) {
			d.ProfileRules = []IngressProfileRule{{MatchLabels: map[string]string{"": "x"}, Profile: DefaultProfileName}}
		}, "profileRules[0].matchLabels has an empty key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := validIngressDiscovery()
			tc.mutate(&d)
			err := d.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want an error mentioning %q", err, tc.wantErr)
			}
		})
	}
}

// A settings write must not be able to smuggle an invalid discovery block past
// the whole-settings gate.
func TestSettingsValidateIncludesIngressDiscovery(t *testing.T) {
	s := DefaultSettings()
	s.IngressDiscovery = validIngressDiscovery()
	s.IngressDiscovery.AllowedDomainSuffixes = nil
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "allowedDomainSuffixes") {
		t.Fatalf("Settings.Validate() = %v, want the ingressDiscovery failure", err)
	}
	s.IngressDiscovery.AllowedDomainSuffixes = []string{"example.com"}
	if err := s.Validate(); err != nil {
		t.Fatalf("Settings.Validate() = %v", err)
	}
}

// An invalid profile must be rejected by the WHOLE-settings gate too - a
// settings PUT is the only place an operator will ever see the error.
func TestSettingsValidateIncludesProfiles(t *testing.T) {
	s := DefaultSettings()
	s.IngressDiscovery = validIngressDiscovery()
	s.IngressDiscovery.Profiles = map[string]IngressHostTemplate{
		"public-ratelimited": {Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80}},
	}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "certificateRef is required") {
		t.Fatalf("Settings.Validate() = %v, want the profile's missing certificateRef", err)
	}
	s.IngressDiscovery.Profiles["public-ratelimited"] = IngressHostTemplate{
		Upstream:    Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
		TLS:         TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
		Middlewares: []string{"rate-limit"},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Settings.Validate() = %v", err)
	}
}

// The profile annotation is attacker-controlled: a cluster tenant may be able to
// create or edit an Ingress. Every value must either match a defined profile
// EXACTLY or fail to resolve; there is no partial match, no case folding, no
// nearest-neighbour guess, and never a silent fall back to the default for a
// value that named something.
func TestResolveProfile(t *testing.T) {
	sso := IngressHostTemplate{
		Upstream:    Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
		TLS:         TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
		Middlewares: []string{"sso"},
		AccessLists: []string{"home-vpn"},
	}
	def := IngressHostTemplate{
		Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
		TLS:      TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
	}
	d := IngressDiscoverySettings{
		Template: def,
		Profiles: map[string]IngressHostTemplate{"sso-internal": sso, "public-ratelimited": {}},
	}

	tests := []struct {
		name     string
		raw      string
		wantName string
		wantOK   bool
	}{
		{"absent", "", DefaultProfileName, true},
		{"whitespace only is absent", "   ", DefaultProfileName, true},
		{"tab and newline only is absent", "\t\n", DefaultProfileName, true},
		{"exact match", "sso-internal", "sso-internal", true},
		{"surrounding whitespace trimmed", "  sso-internal  ", "sso-internal", true},
		{"unknown name", "does-not-exist", "does-not-exist", false},
		{"prefix must not match", "sso", "sso", false},
		{"suffix must not match", "internal", "internal", false},
		{"substring must not match", "so-intern", "so-intern", false},
		{"case must not fold", "SSO-Internal", "SSO-Internal", false},
		{"reserved default name is not a profile", DefaultProfileName, DefaultProfileName, false},
		{"path traversal", "../../etc/passwd", "../../etc/passwd", false},
		{"path traversal onto a real profile", "../sso-internal", "../sso-internal", false},
		{"separator injection", "sso-internal,public-ratelimited", "sso-internal,public-ratelimited", false},
		{"null byte", "sso-internal\x00", "sso-internal\x00", false},
		{"newline injection", "sso-internal\nrate-limit", "sso-internal\nrate-limit", false},
		{"glob", "*", "*", false},
		{"yaml-ish", "{sso-internal}", "{sso-internal}", false},
		{"template injection", "${sso-internal}", "${sso-internal}", false},
		{"unicode lookalike", "ѕso-internal", "ѕso-internal", false}, // Cyrillic 'ѕ'
		{"unicode zero width", "sso-​internal", "sso-​internal", false},
		{"unicode normalisation is not applied", "sso-internaĺ", "sso-internaĺ", false},
		{"very long", strings.Repeat("a", 8192), strings.Repeat("a", 8192), false},
		{"long with a real name inside", strings.Repeat("a", 4096) + "sso-internal", strings.Repeat("a", 4096) + "sso-internal", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, name, ok := d.ResolveProfile(tc.raw)
			if ok != tc.wantOK || name != tc.wantName {
				t.Fatalf("ResolveProfile(%q) = (_, %q, %v), want (%q, %v)", tc.raw, name, ok, tc.wantName, tc.wantOK)
			}
			if !ok {
				// A failed resolution must hand back nothing usable: a caller that
				// ignored ok must not accidentally build a host from a real chain.
				if got.TLS.CertificateRef != "" || len(got.Middlewares) != 0 || len(got.AccessLists) != 0 {
					t.Fatalf("a failed resolution returned a usable template: %+v", got)
				}
				return
			}
			want := def
			if name != DefaultProfileName {
				want = d.Profiles[name]
			}
			if got.TLS.CertificateRef != want.TLS.CertificateRef ||
				strings.Join(got.Middlewares, ",") != strings.Join(want.Middlewares, ",") ||
				strings.Join(got.AccessLists, ",") != strings.Join(want.AccessLists, ",") {
				t.Fatalf("ResolveProfile(%q) returned %+v, want %+v", tc.raw, got, want)
			}
		})
	}

	// With no profiles configured at all, only the default resolves - naming
	// anything fails closed rather than matching a nil map loosely.
	bare := IngressDiscoverySettings{Template: def}
	if _, name, ok := bare.ResolveProfile(""); !ok || name != DefaultProfileName {
		t.Fatalf("no profiles configured: absent annotation must still give the default, got %q/%v", name, ok)
	}
	if _, _, ok := bare.ResolveProfile("anything"); ok {
		t.Fatal("no profiles configured: naming one must not resolve")
	}
}

// ResolveProfileFor is strictly stronger than the annotation: a matching rule
// wins regardless of what the (possibly hostile) annotation says, and
// "rules-only" mode never reads the annotation at all.
func TestResolveProfileFor(t *testing.T) {
	sso := IngressHostTemplate{
		Upstream:    Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
		TLS:         TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
		Middlewares: []string{"sso"},
	}
	def := IngressHostTemplate{
		Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
		TLS:      TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
	}
	base := IngressDiscoverySettings{
		Template: def,
		Profiles: map[string]IngressHostTemplate{"sso-internal": sso, "public-ratelimited": {}},
	}

	t.Run("first matching rule wins over the annotation", func(t *testing.T) {
		d := base
		d.ProfileRules = []IngressProfileRule{
			{Namespace: "monitoring", Profile: "sso-internal"},
			{Profile: DefaultProfileName}, // catch-all; must never be reached first
		}
		// The Ingress asks for public-ratelimited via the annotation; the rule
		// overrides it without even looking.
		got, name, ok := d.ResolveProfileFor("monitoring", nil, "public-ratelimited")
		if !ok || name != "sso-internal" || strings.Join(got.Middlewares, ",") != "sso" {
			t.Fatalf("ResolveProfileFor = (%+v, %q, %v), want the ruled sso-internal", got, name, ok)
		}
	})

	t.Run("rule matches on labels", func(t *testing.T) {
		d := base
		d.ProfileRules = []IngressProfileRule{{MatchLabels: map[string]string{"team": "core"}, Profile: "sso-internal"}}
		if _, name, ok := d.ResolveProfileFor("other-ns", map[string]string{"team": "core", "extra": "x"}, ""); !ok || name != "sso-internal" {
			t.Fatalf("ResolveProfileFor = (_, %q, %v), want sso-internal (label subset matched)", name, ok)
		}
		if _, name, ok := d.ResolveProfileFor("other-ns", map[string]string{"team": "edge"}, ""); !ok || name != DefaultProfileName {
			t.Fatalf("ResolveProfileFor = (_, %q, %v), want the default (label mismatch, no rule matches)", name, ok)
		}
	})

	t.Run("rule can target the default template by name", func(t *testing.T) {
		d := base
		d.ProfileRules = []IngressProfileRule{{Namespace: "public", Profile: DefaultProfileName}}
		if _, name, ok := d.ResolveProfileFor("public", nil, "sso-internal"); !ok || name != DefaultProfileName {
			t.Fatalf("ResolveProfileFor = (_, %q, %v), want template (rule overrides the annotation)", name, ok)
		}
	})

	t.Run("no rule matches: annotation-or-rules falls back to the annotation", func(t *testing.T) {
		d := base
		d.ProfileRules = []IngressProfileRule{{Namespace: "monitoring", Profile: "sso-internal"}}
		if _, name, ok := d.ResolveProfileFor("other-ns", nil, "public-ratelimited"); !ok || name != "public-ratelimited" {
			t.Fatalf("ResolveProfileFor = (_, %q, %v), want the annotation's choice", name, ok)
		}
		if _, name, ok := d.ResolveProfileFor("other-ns", nil, ""); !ok || name != DefaultProfileName {
			t.Fatalf("ResolveProfileFor = (_, %q, %v), want the default (absent annotation)", name, ok)
		}
		if _, _, ok := d.ResolveProfileFor("other-ns", nil, "no-such-profile"); ok {
			t.Fatal("an unresolvable annotation must still fail closed when no rule matches")
		}
	})

	t.Run("rules-only never reads the annotation", func(t *testing.T) {
		d := base
		d.ProfileSelection = ProfileSelectionRulesOnly
		d.ProfileRules = []IngressProfileRule{{Namespace: "monitoring", Profile: "sso-internal"}}
		// A namespace match still wins.
		if _, name, ok := d.ResolveProfileFor("monitoring", nil, "public-ratelimited"); !ok || name != "sso-internal" {
			t.Fatalf("ResolveProfileFor = (_, %q, %v), want the rule's sso-internal", name, ok)
		}
		// No rule matches: the default template, NOT the annotation's choice, and
		// NOT a skip - exactly as if the Ingress carried no annotation at all.
		got, name, ok := d.ResolveProfileFor("other-ns", nil, "public-ratelimited")
		if !ok || name != DefaultProfileName || got.TLS.CertificateRef != def.TLS.CertificateRef {
			t.Fatalf("ResolveProfileFor = (%+v, %q, %v), want the default template, annotation ignored", got, name, ok)
		}
		// Even a garbage annotation value must not cause a skip in rules-only mode.
		if _, name, ok := d.ResolveProfileFor("other-ns", nil, "../../etc/passwd"); !ok || name != DefaultProfileName {
			t.Fatalf("ResolveProfileFor = (_, %q, %v), want the default (annotation never consulted)", name, ok)
		}
	})
}

// Settings are persisted as YAML, so a profile map that did not round-trip would
// be silently dropped on the next save - the chain would vanish and every host
// selecting it would start being skipped.
func TestProfilesSurviveYAMLRoundTrip(t *testing.T) {
	in := DefaultSettings()
	in.IngressDiscovery = validIngressDiscovery()
	in.IngressDiscovery.Profiles = map[string]IngressHostTemplate{
		"public-ratelimited": {
			Upstream:    Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:         TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
			Middlewares: []string{"rate-limit"},
		},
		"sso-internal": {
			UpstreamGroupRef:  "k8s-nodes",
			TLS:               TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
			WebsocketsUpgrade: true,
			RobotsNoIndex:     true,
			Timeouts:          &HostTimeouts{ConnectSeconds: 3, ReadSeconds: 7},
			Middlewares:       []string{"sso", "rate-limit"},
			AccessLists:       []string{"home-vpn"},
			Tags:              []string{"sso"},
			DefaultDNS:        &DNSSyncPolicy{LanDirect: true},
		},
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("fixture must be valid: %v", err)
	}
	b, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Settings
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in.IngressDiscovery.Profiles, out.IngressDiscovery.Profiles) {
		t.Fatalf("profiles did not round-trip:\n got %+v\nwant %+v", out.IngressDiscovery.Profiles, in.IngressDiscovery.Profiles)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("round-tripped settings must still validate: %v", err)
	}
}

// profileRules, profileSelection and a profile's own allowedDomainSuffixes are
// as persistence-critical as the profile map itself - a dropped rule silently
// hands profile selection back to the untrusted annotation.
func TestProfileRulesSurviveYAMLRoundTrip(t *testing.T) {
	in := DefaultSettings()
	in.IngressDiscovery = validIngressDiscovery()
	in.IngressDiscovery.AllowedDomainSuffixes = []string{"example.com"}
	in.IngressDiscovery.Template.AllowedDomainSuffixes = []string{"apps.example.com"}
	in.IngressDiscovery.Profiles = map[string]IngressHostTemplate{
		"sso-internal": {
			Upstream:              Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
			TLS:                   TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
			AllowedDomainSuffixes: []string{"internal.example.com"},
		},
	}
	in.IngressDiscovery.ProfileSelection = ProfileSelectionRulesOnly
	in.IngressDiscovery.ProfileRules = []IngressProfileRule{
		{Namespace: "monitoring", MatchLabels: map[string]string{"team": "core"}, Profile: "sso-internal"},
		{Profile: DefaultProfileName},
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("fixture must be valid: %v", err)
	}
	b, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Settings
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in.IngressDiscovery.ProfileRules, out.IngressDiscovery.ProfileRules) {
		t.Fatalf("profileRules did not round-trip:\n got %+v\nwant %+v", out.IngressDiscovery.ProfileRules, in.IngressDiscovery.ProfileRules)
	}
	if out.IngressDiscovery.ProfileSelection != ProfileSelectionRulesOnly {
		t.Fatalf("profileSelection = %q, want %q", out.IngressDiscovery.ProfileSelection, ProfileSelectionRulesOnly)
	}
	if strings.Join(out.IngressDiscovery.Template.AllowedDomainSuffixes, ",") != "apps.example.com" {
		t.Fatalf("template.allowedDomainSuffixes = %v", out.IngressDiscovery.Template.AllowedDomainSuffixes)
	}
	if strings.Join(out.IngressDiscovery.Profiles["sso-internal"].AllowedDomainSuffixes, ",") != "internal.example.com" {
		t.Fatalf("profiles[sso-internal].allowedDomainSuffixes = %v", out.IngressDiscovery.Profiles["sso-internal"].AllowedDomainSuffixes)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("round-tripped settings must still validate: %v", err)
	}
}

// The timeouts rules are the proxy host's, reused - not restated. If the two
// ever drift, a template could pass a settings write and then produce hosts the
// config validator rejects as one batch, which is the failure mode ValidateRefs
// exists to prevent. Asserting the same error TEXT is the cheapest way to pin
// that they run through the same helper.
func TestTemplateTimeoutsUseTheProxyHostRules(t *testing.T) {
	bad := &HostTimeouts{ConnectSeconds: 3601}

	host := ProxyHost{
		ObjectMeta: ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
		Timeouts:   bad,
	}
	hostErr := host.Validate()
	if hostErr == nil {
		t.Fatal("fixture: the proxy host must reject these timeouts")
	}

	d := validIngressDiscovery()
	d.Template.Timeouts = bad
	tmplErr := d.Validate()
	if tmplErr == nil {
		t.Fatal("the template must reject the timeouts a proxy host rejects")
	}

	const want = "timeouts.connectSeconds 3601 out of range (0-3600)"
	if !strings.Contains(hostErr.Error(), want) || !strings.Contains(tmplErr.Error(), want) {
		t.Fatalf("error shapes differ:\n host: %v\n tmpl: %v", hostErr, tmplErr)
	}
}

// An operator who sets none of the new fields must get exactly the config they
// had before: no zero-valued keys anywhere in the encoded settings, and nothing
// new on the derived hosts. ProxyHost.DNS needed a pointer to omit correctly,
// so timeouts is asserted the same way rather than trusted.
func TestUnsetTemplateFieldsAreOmittedFromTheEncodedSettings(t *testing.T) {
	s := DefaultSettings()
	s.IngressDiscovery = validIngressDiscovery()
	s.IngressDiscovery.Profiles = map[string]IngressHostTemplate{"public": {
		Upstream: Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
		TLS:      TLSSettings{CertificateRef: "wildcard", ForceSSL: true},
	}}
	if err := s.Validate(); err != nil {
		t.Fatalf("fixture must be valid: %v", err)
	}
	b, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"robotsNoIndex", "timeouts", "tags"} {
		if strings.Contains(string(b), key) {
			t.Fatalf("a template that sets no %q still encodes the key:\n%s", key, b)
		}
	}
}

func TestProfileNamesSorted(t *testing.T) {
	d := IngressDiscoverySettings{Profiles: map[string]IngressHostTemplate{"z": {}, "a": {}, "m": {}}}
	if got := strings.Join(d.ProfileNames(), ","); got != "a,m,z" {
		t.Fatalf("ProfileNames() = %q, want a deterministic sorted list", got)
	}
	if got := (IngressDiscoverySettings{}).ProfileNames(); len(got) != 0 {
		t.Fatalf("ProfileNames() on an empty config = %v", got)
	}
}

func TestIngressDiscoveryInterval(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", time.Minute},
		{"90s", 90 * time.Second},
		{"5m", 5 * time.Minute},
		{"nonsense", time.Minute},
		{"1s", time.Minute}, // below the floor: fall back rather than hot-loop
	}
	for _, tc := range tests {
		d := IngressDiscoverySettings{PollInterval: tc.in}
		if got := d.Interval(); got != tc.want {
			t.Fatalf("Interval(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestAllowedDomain(t *testing.T) {
	d := IngressDiscoverySettings{AllowedDomainSuffixes: []string{"example.com", ".apps.internal", "  Example.NET  "}}
	tests := []struct {
		name string
		want bool
	}{
		{"app.example.com", true},
		{"deep.sub.example.com", true},
		{"example.com", true},
		{"api.apps.internal", true},
		{"host.example.net", true},
		{"notexample.com", false}, // must match on a label boundary
		{"example.com.attacker.test", false},
		{"attacker.test", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := d.AllowedDomain(tc.name); got != tc.want {
			t.Fatalf("AllowedDomain(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
	// With no suffixes configured nothing is allowed (fail closed).
	if (IngressDiscoverySettings{}).AllowedDomain("app.example.com") {
		t.Fatal("an empty suffix list must allow nothing")
	}
}

// AllowedDomainFor uses the resolved template's OWN narrower list when it sets
// one, and only falls back to the global list when it does not - a profile
// with an override must never see names the global list alone would allow.
func TestAllowedDomainFor(t *testing.T) {
	d := IngressDiscoverySettings{AllowedDomainSuffixes: []string{"example.com"}}
	unset := IngressHostTemplate{}
	narrowed := IngressHostTemplate{AllowedDomainSuffixes: []string{"public.example.com"}}

	if !d.AllowedDomainFor(unset, "anything.example.com") {
		t.Fatal("a template with no override must fall back to the global list")
	}
	if !d.AllowedDomainFor(narrowed, "app.public.example.com") {
		t.Fatal("a name within the narrowed suffix must be allowed")
	}
	if d.AllowedDomainFor(narrowed, "other.example.com") {
		t.Fatal("a name outside the narrowed suffix must be rejected even though the global list would allow it")
	}
}

func TestIsHostname(t *testing.T) {
	ok := []string{"a.example.com", "x-y.example.com", "a.b.c.d.example.com", "1a.example.com"}
	bad := []string{"", "localhost", "*.example.com", "-a.example.com", "a-.example.com",
		"a..example.com", "a_b.example.com", "a.example.com/x", "a b.example.com",
		"a.example.com\n", strings.Repeat("a.", 130) + "example.com"}
	for _, s := range ok {
		if !IsHostname(s) {
			t.Fatalf("IsHostname(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if IsHostname(s) {
			t.Fatalf("IsHostname(%q) = true, want false", s)
		}
	}
}

func TestNormalizeHostname(t *testing.T) {
	tests := map[string]string{
		"  App.Example.COM. ": "app.example.com",
		"app.example.com":     "app.example.com",
		"":                    "",
	}
	for in, want := range tests {
		if got := NormalizeHostname(in); got != want {
			t.Fatalf("NormalizeHostname(%q) = %q, want %q", in, got, want)
		}
	}
}

// The scope subject has to be registered or a token could never be granted it.
func TestIngressDiscoveryScopeIsKnown(t *testing.T) {
	tok := APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{"ingress-discovery:write"}}
	if err := tok.Validate(); err != nil {
		t.Fatalf("ingress-discovery must be a known scope subject: %v", err)
	}
	if !ScopeSatisfied([]string{"ingress-discovery:write"}, "ingress-discovery:read") {
		t.Fatal("write must imply read")
	}
	if ScopeSatisfied([]string{"ingress-discovery:read"}, "ingress-discovery:write") {
		t.Fatal("read must not imply write")
	}
}

func TestIsDNSLabel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"monitoring", true},
		{"kube-system", true},
		{"a", true},
		{"", false},
		{"ns.evil", false},    // a dot would make the derived "<name>.<namespace>" ambiguous
		{"Monitoring", false}, // namespaces are lowercase
		{"-ns", false},
		{"ns-", false},
		{"ns/other", false},
	} {
		if got := IsDNSLabel(tc.in); got != tc.want {
			t.Errorf("IsDNSLabel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// Every Annotation*/ManagedByLabel/DisabledByLabel method must default to
// EXACTLY the package-level constant it replaces, so an unconfigured
// deployment's annotations and labels never change.
func TestAnnotationPrefixMethodsDefault(t *testing.T) {
	var d IngressDiscoverySettings
	if got := d.Prefix(); got != DefaultAnnotationPrefix {
		t.Fatalf("Prefix() = %q, want %q", got, DefaultAnnotationPrefix)
	}
	cases := []struct {
		got, want string
	}{
		{d.AnnotationManaged(), AnnotationManaged},
		{d.AnnotationLanDirect(), AnnotationLanDirect},
		{d.AnnotationPublicCname(), AnnotationPublicCname},
		{d.AnnotationProfile(), AnnotationProfile},
		{d.ManagedByLabel(), ManagedByLabel},
		{d.DisabledByLabel(), DisabledByLabel},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

// A custom prefix rewrites every key the same way, and nothing else.
func TestAnnotationPrefixMethodsCustom(t *testing.T) {
	d := IngressDiscoverySettings{AnnotationPrefix: "acme.corp.internal"}
	cases := []struct {
		got, want string
	}{
		{d.AnnotationManaged(), "acme.corp.internal/managed"},
		{d.AnnotationLanDirect(), "acme.corp.internal/lan-direct"},
		{d.AnnotationPublicCname(), "acme.corp.internal/public-cname"},
		{d.AnnotationProfile(), "acme.corp.internal/profile"},
		{d.ManagedByLabel(), "acme.corp.internal/managed-by"},
		{d.DisabledByLabel(), "acme.corp.internal/disabled-by"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

func TestHasStaleManagedByLabel(t *testing.T) {
	d := IngressDiscoverySettings{AnnotationPrefix: "acme.corp.internal"}
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"no labels", nil, false},
		{"current prefix only", map[string]string{"acme.corp.internal/managed-by": "ingress-discovery"}, false},
		{"old default-prefix label", map[string]string{"gpm.rake.pro/managed-by": "ingress-discovery"}, true},
		{"old label with a different value is not discovery's", map[string]string{"gpm.rake.pro/managed-by": "someone-else"}, false},
		{"unrelated label ending in managed-by suffix but not gpm's value", map[string]string{"other/managed-by": "x"}, false},
		{"operator-authored host has no managed-by at all", map[string]string{"team": "platform"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := d.HasStaleManagedByLabel(tc.labels); got != tc.want {
				t.Errorf("HasStaleManagedByLabel(%v) = %v, want %v", tc.labels, got, tc.want)
			}
		})
	}
}

func TestStripStaleDiscoveryLabels(t *testing.T) {
	d := IngressDiscoverySettings{AnnotationPrefix: "acme.corp.internal"}
	labels := map[string]string{
		"gpm.rake.pro/managed-by":       "ingress-discovery",
		"gpm.rake.pro/disabled-by":      "ingress-discovery",
		"acme.corp.internal/managed-by": "ingress-discovery",
		"team":                          "platform", // unrelated, must survive
	}
	d.StripStaleDiscoveryLabels(labels)
	want := map[string]string{
		"acme.corp.internal/managed-by": "ingress-discovery",
		"team":                          "platform",
	}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("StripStaleDiscoveryLabels: got %v, want %v", labels, want)
	}
}

// The core safety property: saving a changed annotationPrefix while a proxy
// host still carries managed-by under the OLD prefix must be refused, naming
// the count, UNLESS annotationPrefixMigrate is set - and switching that flag on
// changes nothing else about the write.
func TestIngressDiscoveryValidateRefsAnnotationPrefixMigration(t *testing.T) {
	d := validIngressDiscovery()
	d.AnnotationPrefix = "acme.corp.internal"
	staleHost := ProxyHost{
		ObjectMeta: ObjectMeta{Name: "ing-app.default", Labels: map[string]string{
			ManagedByLabel: ManagedByIngressDiscovery, // the OLD (default) prefix's key
		}},
		Domains:  []string{"app.example.com"},
		Upstream: Upstream{Scheme: "http", Host: "10.0.0.5", Port: 80},
		TLS:      TLSSettings{CertificateRef: "wildcard"},
	}
	cfg := Config{ProxyHosts: []ProxyHost{staleHost}}

	err := d.ValidateRefs(cfg)
	if err == nil {
		t.Fatal("ValidateRefs() = nil, want a refusal naming the orphaned host")
	}
	if !strings.Contains(err.Error(), "1 proxy host") {
		t.Errorf("ValidateRefs() = %v, want the count of orphaned hosts", err)
	}
	if !strings.Contains(err.Error(), "annotationPrefixMigrate") {
		t.Errorf("ValidateRefs() = %v, want it to name the escape hatch", err)
	}

	// A second stale host raises the count.
	cfg.ProxyHosts = append(cfg.ProxyHosts, ProxyHost{
		ObjectMeta: ObjectMeta{Name: "ing-other.default", Labels: map[string]string{
			ManagedByLabel: ManagedByIngressDiscovery,
		}},
		Domains:  []string{"other.example.com"},
		Upstream: Upstream{Scheme: "http", Host: "10.0.0.6", Port: 80},
		TLS:      TLSSettings{CertificateRef: "wildcard"},
	})
	err = d.ValidateRefs(cfg)
	if err == nil || !strings.Contains(err.Error(), "2 proxy host") {
		t.Fatalf("ValidateRefs() = %v, want a refusal naming 2 hosts", err)
	}

	// The certificate every one of these hosts' template references, so a
	// config with no orphaned host at all passes ValidateRefs OUTRIGHT (not
	// just past the annotationPrefix gate).
	withCert := []Certificate{{ObjectMeta: ObjectMeta{Name: "wildcard"}}}

	// Unrelated (non-discovery) hosts never count toward the refusal.
	handWritten := Config{Certificates: withCert, ProxyHosts: []ProxyHost{{
		ObjectMeta: ObjectMeta{Name: "hand-written"},
		Domains:    []string{"hand.example.com"},
		Upstream:   Upstream{Scheme: "http", Host: "10.0.0.7", Port: 80},
		TLS:        TLSSettings{CertificateRef: "wildcard"},
	}}}
	if err := d.ValidateRefs(handWritten); err != nil {
		t.Fatalf("ValidateRefs() = %v, want nil (no discovery-managed host to orphan)", err)
	}

	// A host already labelled under the CURRENT (new) prefix never counts.
	current := Config{Certificates: withCert, ProxyHosts: []ProxyHost{{
		ObjectMeta: ObjectMeta{Name: "already-migrated", Labels: map[string]string{
			d.ManagedByLabel(): ManagedByIngressDiscovery,
		}},
		Domains:  []string{"cur.example.com"},
		Upstream: Upstream{Scheme: "http", Host: "10.0.0.8", Port: 80},
		TLS:      TLSSettings{CertificateRef: "wildcard"},
	}}}
	if err := d.ValidateRefs(current); err != nil {
		t.Fatalf("ValidateRefs() = %v, want nil (already under the current prefix)", err)
	}

	// The escape hatch: annotationPrefixMigrate:true lifts the refusal even
	// though the same stale-labelled hosts are still present. The rest of
	// ValidateRefs' checks still run (and here still pass, since every ref this
	// config names resolves).
	migrating := d
	migrating.AnnotationPrefixMigrate = true
	cfgWithCert := Config{
		ProxyHosts:   cfg.ProxyHosts,
		Certificates: []Certificate{{ObjectMeta: ObjectMeta{Name: "wildcard"}}},
	}
	if err := migrating.ValidateRefs(cfgWithCert); err != nil {
		t.Fatalf("ValidateRefs() with annotationPrefixMigrate = %v, want nil", err)
	}

	// A disabled discovery block is never checked at all - same rule as every
	// other ValidateRefs gate.
	disabled := d
	disabled.Enabled = false
	if err := disabled.ValidateRefs(cfg); err != nil {
		t.Fatalf("ValidateRefs() on a disabled block = %v, want nil", err)
	}
}
