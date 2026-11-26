package model

import (
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// allowLinkLocal is read under exactly that name in YAML and JSON, defaults to
// false, and is a known settings key.
func TestDockerDiscoveryAllowLinkLocalKey(t *testing.T) {
	tests := []struct {
		name   string
		decode func([]byte, any) error
		doc    string
		want   bool
	}{
		{"yaml set", yaml.Unmarshal, "dockerDiscovery:\n  allowLinkLocal: true\n", true},
		{"yaml absent", yaml.Unmarshal, "dockerDiscovery:\n  enabled: false\n", false},
		{"json set", json.Unmarshal, `{"dockerDiscovery":{"allowLinkLocal":true}}`, true},
		{"json absent", json.Unmarshal, `{"dockerDiscovery":{}}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var s Settings
			if err := tc.decode([]byte(tc.doc), &s); err != nil {
				t.Fatal(err)
			}
			if got := s.DockerDiscoveryResolved().AllowLinkLocal; got != tc.want {
				t.Fatalf("allowLinkLocal = %v, want %v", got, tc.want)
			}
		})
	}
	if unknown := UnknownYAMLKeys([]byte("dockerDiscovery:\n  allowLinkLocal: true\n"), &Settings{}); len(unknown) != 0 {
		t.Fatalf("unknown keys %v, want allowLinkLocal recognised", unknown)
	}
	out, err := json.Marshal(DockerDiscoverySettings{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "allowLinkLocal") {
		t.Fatalf("default block marshals to %s, want the key omitted", out)
	}
	out, err = json.Marshal(DockerDiscoverySettings{AllowLinkLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"allowLinkLocal":true`) {
		t.Fatalf("block marshals to %s, want allowLinkLocal", out)
	}
}
