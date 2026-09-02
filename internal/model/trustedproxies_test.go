package model

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidateTrustedProxies(t *testing.T) {
	cases := []struct {
		name    string
		entries []string
		wantErr bool
	}{
		{"empty list is the trust-nobody default", nil, false},
		{"cidr", []string{"10.0.0.0/8"}, false},
		{"bare ipv4", []string{"192.0.2.10"}, false},
		{"ipv6 cidr", []string{"2001:db8::/32"}, false},
		{"bare ipv6", []string{"2001:db8::1"}, false},
		{"wildcards parse (warned, not refused)", []string{"0.0.0.0/0", "::/0"}, false},
		{"mixed list", []string{"10.0.0.0/8", "192.0.2.10", "2001:db8::/32"}, false},
		{"not an address", []string{"proxy.example.com"}, true},
		{"bad mask", []string{"10.0.0.0/99"}, true},
		{"empty entry", []string{""}, true},
		{"blank entry", []string{"   "}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTrustedProxies("settings.trustedProxies", tc.entries)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestTrustedProxyWildcards(t *testing.T) {
	got := TrustedProxyWildcards([]string{"10.0.0.0/8", "0.0.0.0/0", "192.0.2.10", "::/0", "2001:db8::/32"})
	want := []string{"0.0.0.0/0", "::/0"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestTrustedProxiesReachValidation proves both objects that carry the field run
// it through the shared validator, so a bad CIDR is refused at config-write time
// rather than silently dropped in the data plane.
func TestTrustedProxiesReachValidation(t *testing.T) {
	s := DefaultSettings()
	s.TrustedProxies = []string{"nope"}
	if err := s.Validate(); err == nil {
		t.Fatal("settings.trustedProxies with a bad entry must not validate")
	}
	s.TrustedProxies = []string{"10.0.0.0/8"}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid settings.trustedProxies rejected: %v", err)
	}

	h := ProxyHost{
		ObjectMeta:     ObjectMeta{Name: "app"},
		Domains:        []string{"app.example.com"},
		Upstream:       Upstream{Scheme: "http", Host: "192.0.2.20", Port: 80},
		TrustedProxies: &[]string{"nope"},
	}
	if err := h.Validate(); err == nil {
		t.Fatal("proxyHost trustedProxies with a bad entry must not validate")
	}
	h.TrustedProxies = &[]string{"192.0.2.10/32"}
	if err := h.Validate(); err != nil {
		t.Fatalf("valid proxyHost trustedProxies rejected: %v", err)
	}
	// Present-but-empty is the "trust nobody" override and must validate.
	h.TrustedProxies = &[]string{}
	if err := h.Validate(); err != nil {
		t.Fatalf("an empty proxyHost trustedProxies list must validate (it means trust nobody): %v", err)
	}
}

// TestHostTrustedProxiesRoundTrip: "present and empty" is a distinct value from
// "absent", so it has to survive both encodings - an empty list that could not
// round-trip through the API or the store would be an unusable override.
func TestHostTrustedProxiesRoundTrip(t *testing.T) {
	h := ProxyHost{
		ObjectMeta: ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   Upstream{Scheme: "http", Host: "192.0.2.20", Port: 80},
		// The documented "trust nobody" override.
		TrustedProxies: &[]string{},
	}

	yb, err := yaml.Marshal(h)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var fromYAML ProxyHost
	if err := yaml.Unmarshal(yb, &fromYAML); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if fromYAML.TrustedProxies == nil || len(*fromYAML.TrustedProxies) != 0 {
		t.Fatalf("yaml round trip lost the empty list: %v\n%s", fromYAML.TrustedProxies, yb)
	}

	jb, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var fromJSON ProxyHost
	if err := json.Unmarshal(jb, &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if fromJSON.TrustedProxies == nil || len(*fromJSON.TrustedProxies) != 0 {
		t.Fatalf("json round trip lost the empty list: %v\n%s", fromJSON.TrustedProxies, jb)
	}

	// An omitted key stays nil, which is what inherits the fleet-wide list.
	var absent ProxyHost
	if err := yaml.Unmarshal([]byte("name: app\n"), &absent); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if absent.TrustedProxies != nil {
		t.Fatalf("an omitted trustedProxies must stay nil, got %v", absent.TrustedProxies)
	}
}
