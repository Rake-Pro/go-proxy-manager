package model

import (
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// A discovery template's trustedProxies is validated like a proxy host's: a
// present-but-empty list is valid (trust nobody), and every entry of a list
// must parse as a CIDR or an address. The error names the block it is in.
func TestTemplateTrustedProxiesValidation(t *testing.T) {
	list := func(v ...string) *[]string { s := append([]string{}, v...); return &s }
	tests := []struct {
		name    string
		tp      *[]string
		wantErr string
	}{
		{"unset", nil, ""},
		{"empty means trust nobody", list(), ""},
		{"cidr and address", list("192.0.2.0/24", "2001:db8::1"), ""},
		{"bad entry", list("nope"), `trustedProxies: invalid cidr/ip "nope" (expected a CIDR like 10.0.0.0/8 or a bare address)`},
		{"blank entry", list("192.0.2.1", " "), `trustedProxies[1]: empty entry`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			check := func(label string, err error, prefix string) {
				t.Helper()
				if tc.wantErr == "" {
					if err != nil {
						t.Fatalf("%s: unexpected error %v", label, err)
					}
					return
				}
				if err == nil || err.Error() != prefix+tc.wantErr {
					t.Fatalf("%s: error %v, want %q", label, err, prefix+tc.wantErr)
				}
			}

			d := validIngressDiscovery()
			d.Template.TrustedProxies = tc.tp
			check("ingress template", d.Validate(), "ingressDiscovery.template.")

			d = validIngressDiscovery()
			d.Profiles = map[string]IngressHostTemplate{"edge": {
				Upstream:       Upstream{Scheme: "http", Host: "10.0.0.40", Port: 80},
				TLS:            TLSSettings{CertificateRef: "wildcard"},
				TrustedProxies: tc.tp,
			}}
			check("ingress profile", d.Validate(), "ingressDiscovery.profiles.edge.")

			dd := dockerBase()
			dd.Template.TrustedProxies = tc.tp
			check("docker template", dd.Validate(), "dockerDiscovery.template.")
		})
	}
}

// The three states survive the settings file and the API body, exactly as
// ProxyHost.TrustedProxies does: absent stays nil, an explicit empty list stays
// a non-nil empty list, and a list keeps its entries.
func TestTemplateTrustedProxiesRoundTrip(t *testing.T) {
	list := func(v ...string) *[]string { s := append([]string{}, v...); return &s }
	tests := []struct {
		name string
		tp   *[]string
		yaml string // the line the YAML encoding must carry, "" for none
	}{
		{"unset", nil, ""},
		{"empty", list(), "trustedProxies: []"},
		{"list", list("192.0.2.10"), "- 192.0.2.10"},
	}
	same := func(a, b *[]string) bool {
		if (a == nil) != (b == nil) {
			return false
		}
		if a == nil {
			return true
		}
		return (*a != nil) == (*b != nil) && strings.Join(*a, ",") == strings.Join(*b, ",")
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := Settings{IngressDiscovery: IngressDiscoverySettings{Template: IngressHostTemplate{TrustedProxies: tc.tp}}}

			yb, err := yaml.Marshal(in)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			if tc.yaml == "" && strings.Contains(string(yb), "trustedProxies") {
				t.Fatalf("unset trustedProxies was encoded:\n%s", yb)
			}
			if tc.yaml != "" && !strings.Contains(string(yb), tc.yaml) {
				t.Fatalf("yaml lacks %q:\n%s", tc.yaml, yb)
			}
			var fromYAML Settings
			if err := yaml.Unmarshal(yb, &fromYAML); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if got := fromYAML.IngressDiscovery.Template.TrustedProxies; !same(got, tc.tp) {
				t.Fatalf("yaml round trip: got %v, want %v\n%s", got, tc.tp, yb)
			}

			jb, err := json.Marshal(in)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if tc.tp == nil && strings.Contains(string(jb), "trustedProxies") {
				t.Fatalf("unset trustedProxies was encoded: %s", jb)
			}
			var fromJSON Settings
			if err := json.Unmarshal(jb, &fromJSON); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if got := fromJSON.IngressDiscovery.Template.TrustedProxies; !same(got, tc.tp) {
				t.Fatalf("json round trip: got %v, want %v (%s)", got, tc.tp, jb)
			}
		})
	}

	// An explicit null is the unset state, not an empty list.
	var s Settings
	if err := json.Unmarshal([]byte(`{"ingressDiscovery":{"template":{"trustedProxies":null}}}`), &s); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if s.IngressDiscovery.Template.TrustedProxies != nil {
		t.Fatalf("null decoded to %v, want unset", *s.IngressDiscovery.Template.TrustedProxies)
	}
}

// ValidateRefs builds its stand-in host from the template, trustedProxies
// included; a template that sets it must not produce errors of its own there.
func TestTemplateTrustedProxiesPassValidateRefs(t *testing.T) {
	d := validIngressDiscovery()
	empty := []string{}
	d.Template.TrustedProxies = &empty
	cfg := Config{Certificates: []Certificate{{ObjectMeta: ObjectMeta{Name: "wildcard"}, Type: CertTypeCustom, Domains: []string{"*.example.com"},
		Custom: &CustomCertSpec{CertFile: "wildcard.pem", KeyFile: "wildcard-key.pem"}}}}
	if err := d.ValidateRefs(cfg); err != nil {
		t.Fatalf("ValidateRefs: %v", err)
	}
}
