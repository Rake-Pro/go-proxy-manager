package model

import (
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const (
	pinLower = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	pinUpper = "9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08"
	pinColon = "9F:86:D0:81:88:4C:7D:65:9A:2F:EA:A0:C5:5A:D0:15:A3:BF:4F:1B:2B:0B:82:2C:D1:5D:6C:15:B0:F0:0A:08"
	pinOther = "60303ae22b998861bce3b28f33eec1be758a213c86c93c076dbe9f558c11c752"
)

func pinnedHost(scheme string, pins ...string) ProxyHost {
	return ProxyHost{
		ObjectMeta: ObjectMeta{Name: "bmc"}, Domains: []string{"bmc.example.com"},
		Upstream: Upstream{Scheme: scheme, Host: "192.0.2.10", Port: 443, TLS: UpstreamTLS{PinnedCertSHA256: pins}},
	}
}

func TestNormalizeCertFingerprint(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{pinLower, pinLower, true},
		{pinUpper, pinLower, true},
		{pinColon, pinLower, true},
		{strings.ToLower(pinColon), pinLower, true},
		{"", "", false},
		{pinLower[:63], "", false},
		{pinLower + "0", "", false},
		{"g" + pinLower[1:], "", false},
		{pinColon[:len(pinColon)-3], "", false},            // 31 bytes
		{pinColon + ":00", "", false},                      // 33 bytes
		{strings.Replace(pinColon, ":", "", 1), "", false}, // separator missing once
		{strings.Replace(pinColon, ":", "-", 1), "", false},
		{strings.ReplaceAll(pinColon, ":", " "), "", false},
		{"sha256 Fingerprint=" + pinColon, "", false},
		{" " + pinLower, "", false},
		{pinLower[:62] + ":" + pinLower[62:], "", false}, // a stray colon
		{strings.Repeat("ab:", 31) + "a:b", "", false},   // right length, misplaced separators
	} {
		got, ok := NormalizeCertFingerprint(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("NormalizeCertFingerprint(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestUpstreamTLSValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		host    ProxyHost
		wantErr string
	}{
		{"one pin", pinnedHost("https", pinLower), ""},
		{"upper case accepted", pinnedHost("https", pinUpper), ""},
		{"colon form accepted", pinnedHost("https", pinColon), ""},
		{"four pins", pinnedHost("https", pinLower, pinOther, strings.Repeat("a", 64), strings.Repeat("b", 64)), ""},
		{"no tls", pinnedHost("https"), ""},
		{"empty list is absent", pinnedHost("https", []string{}...), ""},
		{"five pins", pinnedHost("https", pinLower, pinOther, strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)), "at most 4"},
		{"http scheme", pinnedHost("http", pinLower), "upstream.tls: requires scheme https"},
		{"short", pinnedHost("https", pinLower[:40]), "upstream.tls.pinnedCertSHA256[0]"},
		{"not hex", pinnedHost("https", strings.Repeat("z", 64)), "is not a SHA-256 fingerprint"},
		{"empty entry", pinnedHost("https", pinLower, ""), "upstream.tls.pinnedCertSHA256[1]"},
		{"duplicate", pinnedHost("https", pinLower, pinLower), "duplicate fingerprint"},
		{"duplicate in two spellings", pinnedHost("https", pinLower, pinColon), "duplicate fingerprint"},
		{"sha1 length", pinnedHost("https", strings.Repeat("a", 40)), "is not a SHA-256 fingerprint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.host.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

// Every place an Upstream lives is held to the same rule.
func TestUpstreamTLSValidatedEverywhere(t *testing.T) {
	bad := Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80, TLS: UpstreamTLS{PinnedCertSHA256: []string{pinLower}}}
	good := bad
	good.Scheme = "https"
	loc := func(u Upstream) ProxyHost {
		h := pinnedHost("https")
		h.Locations = []Location{{Path: "/x", Upstream: &u}}
		return h
	}
	group := func(u Upstream) UpstreamGroup {
		return UpstreamGroup{ObjectMeta: ObjectMeta{Name: "g"}, Upstreams: []GroupUpstream{{Upstream: u}}}
	}
	tmpl := func(u Upstream) error {
		return IngressHostTemplate{Upstream: u, TLS: TLSSettings{CertificateRef: "wildcard"}}.validate("template")
	}
	if err := loc(good).Validate(); err != nil {
		t.Errorf("location with an https pin: %v", err)
	}
	if err := loc(bad).Validate(); err == nil || !strings.Contains(err.Error(), "requires scheme https") {
		t.Errorf("location with an http pin: %v, want rejected", err)
	}
	if err := group(good).Validate(); err != nil {
		t.Errorf("group member with an https pin: %v", err)
	}
	if err := group(bad).Validate(); err == nil || !strings.Contains(err.Error(), "upstreams[0].tls") {
		t.Errorf("group member with an http pin: %v, want rejected", err)
	}
	if err := tmpl(good); err != nil {
		t.Errorf("template with an https pin: %v", err)
	}
	if err := tmpl(bad); err == nil || !strings.Contains(err.Error(), "requires scheme https") {
		t.Errorf("template with an http pin: %v, want rejected", err)
	}
	// A pin alone does not make an upstream "set": it is not empty, so it still
	// conflicts with an upstream group reference.
	h := pinnedHost("https", pinLower)
	h.Upstream = Upstream{TLS: UpstreamTLS{PinnedCertSHA256: []string{pinLower}}}
	h.UpstreamGroupRef = "g"
	if err := h.Validate(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("pin next to upstreamGroupRef: %v, want mutually exclusive", err)
	}
}

// The API (JSON) and the store (YAML) both normalize on the way in, write the
// canonical form back out, and drop an empty list or object entirely.
func TestUpstreamTLSRoundTrip(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var u Upstream
		in := `{"scheme":"https","host":"192.0.2.10","port":443,"tls":{"pinnedCertSHA256":["` + pinColon + `","` + pinUpper[:60] + `"]}}`
		if err := json.Unmarshal([]byte(in), &u); err != nil {
			t.Fatal(err)
		}
		if got := u.TLS.PinnedCertSHA256; len(got) != 2 || got[0] != pinLower || got[1] != pinUpper[:60] {
			t.Fatalf("decoded pins = %q, want the first normalized and the malformed one kept verbatim", got)
		}
		u.TLS.PinnedCertSHA256 = u.TLS.PinnedCertSHA256[:1]
		out, err := json.Marshal(u)
		if err != nil {
			t.Fatal(err)
		}
		if want := `"tls":{"pinnedCertSHA256":["` + pinLower + `"]}`; !strings.Contains(string(out), want) {
			t.Fatalf("marshalled %s, want it to contain %s", out, want)
		}
		var back Upstream
		if err := json.Unmarshal(out, &back); err != nil || !back.Equal(u) {
			t.Fatalf("round trip = %+v, %v; want %+v", back, err, u)
		}
	})
	t.Run("yaml", func(t *testing.T) {
		var h ProxyHost
		in := "name: bmc\ndomains: [bmc.example.com]\nupstream:\n  scheme: https\n  host: 192.0.2.10\n  port: 443\n  tls:\n    pinnedCertSHA256:\n      - " + pinColon + "\n"
		if err := yaml.Unmarshal([]byte(in), &h); err != nil {
			t.Fatal(err)
		}
		if got := h.Upstream.TLS.PinnedCertSHA256; len(got) != 1 || got[0] != pinLower {
			t.Fatalf("decoded pins = %q, want [%s]", got, pinLower)
		}
		if err := h.Validate(); err != nil {
			t.Fatal(err)
		}
		out, err := yaml.Marshal(h)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "pinnedCertSHA256:\n            - "+pinLower) {
			t.Fatalf("marshalled:\n%s\nwant the canonical pin", out)
		}
		if keys := UnknownYAMLKeys([]byte(in), &h); len(keys) != 0 {
			t.Fatalf("unknown keys = %v, want none", keys)
		}
		var back ProxyHost
		if err := yaml.Unmarshal(out, &back); err != nil || !back.Upstream.Equal(h.Upstream) {
			t.Fatalf("round trip = %+v, %v", back.Upstream, err)
		}
	})
	for _, in := range []string{
		`{"scheme":"https","host":"h","port":1,"tls":{}}`,
		`{"scheme":"https","host":"h","port":1,"tls":{"pinnedCertSHA256":[]}}`,
		`{"scheme":"https","host":"h","port":1,"tls":null}`,
	} {
		var u Upstream
		if err := json.Unmarshal([]byte(in), &u); err != nil {
			t.Fatal(err)
		}
		if !u.TLS.IsZero() || u.TLS.PinnedCertSHA256 != nil {
			t.Errorf("%s decoded to %+v, want no tls", in, u.TLS)
		}
		j, _ := json.Marshal(u)
		y, _ := yaml.Marshal(u)
		if strings.Contains(string(j), "tls") || strings.Contains(string(y), "tls") {
			t.Errorf("%s re-encodes with a tls key: json %s yaml %s", in, j, y)
		}
	}
	var h ProxyHost
	if err := yaml.Unmarshal([]byte("upstream:\n  scheme: https\n  tls: {pinnedCertSHA256: []}\n"), &h); err != nil {
		t.Fatal(err)
	}
	if h.Upstream.TLS.PinnedCertSHA256 != nil {
		t.Fatalf("yaml empty list decoded to %#v, want nil", h.Upstream.TLS.PinnedCertSHA256)
	}
	// A group member embeds Upstream inline, so its tls key sits next to weight.
	var g UpstreamGroup
	if err := yaml.Unmarshal([]byte("name: g\nupstreams:\n  - {scheme: https, host: 192.0.2.10, port: 443, weight: 2, tls: {pinnedCertSHA256: ["+pinUpper+"]}}\n"), &g); err != nil {
		t.Fatal(err)
	}
	if m := g.Upstreams[0]; m.Weight != 2 || len(m.TLS.PinnedCertSHA256) != 1 || m.TLS.PinnedCertSHA256[0] != pinLower {
		t.Fatalf("group member = %+v, want weight 2 and the normalized pin", m)
	}
}

func TestUpstreamCloneAndEqual(t *testing.T) {
	orig := Upstream{Scheme: "https", Host: "192.0.2.10", Port: 443, TLS: UpstreamTLS{PinnedCertSHA256: []string{pinLower, pinOther}}}
	c := orig.Clone()
	c.TLS.PinnedCertSHA256[0] = strings.Repeat("0", 64)
	if orig.TLS.PinnedCertSHA256[0] != pinLower {
		t.Fatal("Clone shares the pin slice with the original")
	}
	locs := CloneLocations([]Location{{Path: "/x", Upstream: &orig}})
	locs[0].Upstream.TLS.PinnedCertSHA256[1] = strings.Repeat("0", 64)
	if orig.TLS.PinnedCertSHA256[1] != pinOther {
		t.Fatal("CloneLocations shares the location upstream's pin slice")
	}

	if !(Upstream{Scheme: "https", TLS: UpstreamTLS{PinnedCertSHA256: []string{}}}).Equal(Upstream{Scheme: "https"}) {
		t.Error("an empty and a nil pin list must compare equal")
	}
	if orig.Equal(Upstream{Scheme: "https", Host: "192.0.2.10", Port: 443, TLS: UpstreamTLS{PinnedCertSHA256: []string{pinLower}}}) {
		t.Error("different pin lists compared equal")
	}
	if orig.Equal(Upstream{Scheme: "https", Host: "192.0.2.10", Port: 443, TLS: UpstreamTLS{PinnedCertSHA256: []string{pinOther, pinLower}}}) {
		t.Error("pin order is part of the stored object; a reorder is a change")
	}
	if (Upstream{TLS: UpstreamTLS{PinnedCertSHA256: []string{pinLower}}}).Empty() || !(Upstream{}).Empty() {
		t.Error("Empty must be true only for the zero upstream")
	}
}

// Docker derives every host's upstream from its container, so a pin on a
// Docker template (or a profile the docker block defines itself) is refused
// rather than accepted and dropped. Kubernetes Ingress templates and profiles
// keep their pins, including a profile docker discovery inherits.
func TestDockerDiscoveryTemplateRejectsPins(t *testing.T) {
	pinned := Upstream{Scheme: "https", Host: "192.0.2.30", Port: 443, TLS: UpstreamTLS{PinnedCertSHA256: []string{pinLower}}}
	tmpl := IngressHostTemplate{Upstream: pinned, TLS: TLSSettings{CertificateRef: "wildcard"}}
	for _, tc := range []struct {
		name    string
		mutate  func(*Settings)
		wantErr string
	}{
		{"no pins", func(*Settings) {}, ""},
		{"docker template", func(s *Settings) { s.DockerDiscovery.Template.Upstream = pinned },
			"dockerDiscovery.template.upstream.tls.pinnedCertSHA256: certificate pins are not supported on Docker discovery templates"},
		{"docker profile", func(s *Settings) {
			s.DockerDiscovery.Profiles = map[string]IngressHostTemplate{"ok": {TLS: TLSSettings{CertificateRef: "wildcard"}}, "pinned": tmpl}
		}, "dockerDiscovery.profiles.pinned.upstream.tls.pinnedCertSHA256: certificate pins are not supported on Docker discovery templates"},
		{"docker block disabled", func(s *Settings) { s.DockerDiscovery.Enabled = false; s.DockerDiscovery.Template.Upstream = pinned }, ""},
		{"docker template location", func(s *Settings) {
			s.DockerDiscovery.Template.Locations = []Location{{Path: "/console", Upstream: &pinned}}
		}, ""},
		{"ingress template and inherited profile", func(s *Settings) {
			s.IngressDiscovery = validIngressDiscovery()
			s.IngressDiscovery.Template.Upstream = pinned
			s.IngressDiscovery.Profiles = map[string]IngressHostTemplate{"pinned": tmpl}
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Settings{AdminAuth: AdminAuthSettings{LocalLoginEnabled: true}, DockerDiscovery: dockerBase()}
			tc.mutate(&s)
			err := s.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v, want accepted", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "pin the upstream on a hand-written proxy host instead") {
				t.Fatalf("Validate: %v, want %q", err, tc.wantErr)
			}
		})
	}
}
