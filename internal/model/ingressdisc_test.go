package model

import (
	"strings"
	"testing"
	"time"
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
