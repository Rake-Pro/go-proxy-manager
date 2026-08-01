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
