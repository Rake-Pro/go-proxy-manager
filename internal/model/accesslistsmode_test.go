package model

import (
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func modeLists() []AccessList {
	return []AccessList{
		{ObjectMeta: ObjectMeta{Name: "lan"}, DefaultAction: ActionDeny,
			Rules: []IPRule{{Action: ActionAllow, CIDR: "192.168.0.0/16"}}},
		{ObjectMeta: ObjectMeta{Name: "monitor"}, DefaultAction: ActionDeny,
			Rules: []IPRule{{Action: ActionAllow, CIDR: "203.0.113.0/24", Paths: []string{"/healthz"}}}},
		{ObjectMeta: ObjectMeta{Name: "legacy"}, DefaultAction: ActionDeny,
			BasicAuth: []BasicAuthUser{{Username: "u", PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"}},
			Rules:     []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8"}}},
		{ObjectMeta: ObjectMeta{Name: "blocknet"}, DefaultAction: ActionAllow,
			Rules: []IPRule{{Action: ActionDeny, CIDR: "10.66.0.0/16"}}},
		{ObjectMeta: ObjectMeta{Name: "no-kp"}, Geo: &AccessListGeo{CountryDeny: []string{"KP"}}},
		{ObjectMeta: ObjectMeta{Name: "us-only"}, DefaultAction: ActionDeny, Geo: &AccessListGeo{CountryAllow: []string{"US"}}},
	}
}

func TestAccessListsModeValidate(t *testing.T) {
	tests := []struct {
		name    string
		host    func(*ProxyHost)
		wantErr string // substring; empty means expect success
	}{
		{"unset mode", func(h *ProxyHost) { h.AccessLists = []string{"lan", "monitor"} }, ""},
		{"all", func(h *ProxyHost) { h.AccessLists, h.AccessListsMode = []string{"lan", "legacy"}, "all" }, ""},
		{"any", func(h *ProxyHost) { h.AccessLists, h.AccessListsMode = []string{"lan", "monitor"}, "any" }, ""},
		{"unknown mode", func(h *ProxyHost) { h.AccessLists, h.AccessListsMode = []string{"lan"}, "or" }, `accessListsMode: must be all|any, got "or"`},
		{"mode is case sensitive", func(h *ProxyHost) { h.AccessLists, h.AccessListsMode = []string{"lan"}, "ANY" }, "must be all|any"},
		{"any with no lists", func(h *ProxyHost) { h.AccessListsMode = "any" }, "any needs at least one entry in accessLists"},
		{"all with no lists", func(h *ProxyHost) { h.AccessListsMode = "all" }, ""},
		{"any with a basicAuth list", func(h *ProxyHost) {
			h.AccessLists, h.AccessListsMode = []string{"lan", "legacy"}, "any"
		}, `"legacy" has basicAuth users, which cannot be combined under accessListsMode: any`},
		{"location unknown mode", func(h *ProxyHost) {
			h.Locations = []Location{{Path: "/x", AccessLists: []string{"lan"}, AccessListsMode: "either"}}
		}, `location "/x": accessListsMode: must be all|any`},
		{"location any with no lists", func(h *ProxyHost) {
			h.AccessLists = []string{"lan", "monitor"}
			h.Locations = []Location{{Path: "/x", AccessListsMode: "any"}}
		}, `location "/x": accessListsMode: any needs at least one entry`},
		{"location any with a basicAuth list", func(h *ProxyHost) {
			h.Locations = []Location{{Path: "/x", AccessLists: []string{"legacy", "lan"}, AccessListsMode: "any"}}
		}, `locations: location "/x": accessLists: "legacy" has basicAuth users`},
		{"location any", func(h *ProxyHost) {
			h.Locations = []Location{{Path: "/x", AccessLists: []string{"lan", "monitor"}, AccessListsMode: "any"}}
		}, ""},
		{"any with a defaultAction allow list", func(h *ProxyHost) {
			h.AccessLists, h.AccessListsMode = []string{"lan", "blocknet"}, "any"
		}, `"blocknet" has defaultAction: allow, which under accessListsMode: any would admit every client its rules do not deny`},
		{"any with a geo deny-list", func(h *ProxyHost) {
			h.AccessLists, h.AccessListsMode = []string{"lan", "no-kp"}, "any"
		}, `"no-kp" has geo.countryDeny`},
		{"any with a geo allow-list", func(h *ProxyHost) {
			h.AccessLists, h.AccessListsMode = []string{"lan", "us-only"}, "any"
		}, ""},
		{"all keeps blocklists", func(h *ProxyHost) { h.AccessLists = []string{"lan", "blocknet", "no-kp"} }, ""},
		{"location any with a defaultAction allow list", func(h *ProxyHost) {
			h.Locations = []Location{{Path: "/x", AccessLists: []string{"lan", "blocknet"}, AccessListsMode: "any"}}
		}, `locations: location "/x": accessLists: "blocknet" has defaultAction: allow`},
		{"location any with a geo deny-list", func(h *ProxyHost) {
			h.Locations = []Location{{Path: "/x", AccessLists: []string{"no-kp"}, AccessListsMode: "any"}}
		}, `locations: location "/x": accessLists: "no-kp" has geo.countryDeny`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{AccessLists: modeLists(), ProxyHosts: []ProxyHost{proxyHost("app", tc.host)}}
			err := cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// The mode is optional: a host or template that does not set it encodes no key,
// so existing objects do not churn in git, and a set mode round-trips.
func TestAccessListsModeOmittedWhenUnset(t *testing.T) {
	h := proxyHost("app", func(h *ProxyHost) {
		h.AccessLists = []string{"lan"}
		h.Locations = []Location{{Path: "/x", AccessLists: []string{"lan"}}}
	})
	y, err := yaml.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(y), "accessListsMode") || strings.Contains(string(j), "accessListsMode") {
		t.Fatalf("unset mode must not be encoded:\n%s\n%s", y, j)
	}
	h.AccessListsMode = AccessListsModeAny
	h.Locations[0].AccessListsMode = AccessListsModeAny
	y, _ = yaml.Marshal(h)
	var back ProxyHost
	if err := yaml.Unmarshal(y, &back); err != nil {
		t.Fatal(err)
	}
	if back.AccessListsMode != "any" || back.Locations[0].AccessListsMode != "any" {
		t.Fatalf("mode did not round-trip: %+v", back)
	}
	b, _ := yaml.Marshal(IngressHostTemplate{AccessLists: []string{"lan"}})
	if strings.Contains(string(b), "accessListsMode") {
		t.Fatalf("an unset template mode must not be encoded:\n%s", b)
	}
}

// A discovery template is held to the same mode rules at settings-write time.
func TestAccessListsModeTemplateValidate(t *testing.T) {
	cfg := Config{AccessLists: modeLists(), Certificates: []Certificate{{ObjectMeta: ObjectMeta{Name: "wildcard"}}}}
	tests := []struct {
		name    string
		mutate  func(*IngressHostTemplate)
		wantErr string
	}{
		{"any", func(t *IngressHostTemplate) { t.AccessLists, t.AccessListsMode = []string{"lan", "monitor"}, "any" }, ""},
		{"unknown mode", func(t *IngressHostTemplate) { t.AccessLists, t.AccessListsMode = []string{"lan"}, "some" }, "must be all|any"},
		{"any with no lists", func(t *IngressHostTemplate) { t.AccessListsMode = "any" }, "any needs at least one entry"},
		{"any with a basicAuth list", func(t *IngressHostTemplate) {
			t.AccessLists, t.AccessListsMode = []string{"legacy", "lan"}, "any"
		}, `"legacy" has basicAuth users`},
		{"any with a defaultAction allow list", func(t *IngressHostTemplate) {
			t.AccessLists, t.AccessListsMode = []string{"lan", "blocknet"}, "any"
		}, `"blocknet" has defaultAction: allow`},
		{"location any with a geo deny-list", func(t *IngressHostTemplate) {
			t.Locations = []Location{{Path: "/x", AccessLists: []string{"no-kp"}, AccessListsMode: "any"}}
		}, `locations: location "/x": accessLists: "no-kp" has geo.countryDeny`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := validIngressDiscovery()
			tc.mutate(&d.Template)
			err := d.Validate()
			if err == nil {
				err = d.ValidateRefs(cfg)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// Named profiles, and the Docker block, refuse a blocklist-style member under
// "any" exactly like the default template.
func TestAccessListsModeProfileAndDockerRefuseBlocklists(t *testing.T) {
	cfg := Config{AccessLists: modeLists(), Certificates: []Certificate{{ObjectMeta: ObjectMeta{Name: "wildcard"}}}}
	blocky := IngressHostTemplate{
		TLS:             TLSSettings{CertificateRef: "wildcard"},
		AccessLists:     []string{"lan", "blocknet"},
		AccessListsMode: AccessListsModeAny,
	}
	id := validIngressDiscovery()
	id.Profiles = map[string]IngressHostTemplate{"public": blocky}
	if err := id.ValidateRefs(cfg); err == nil || !strings.Contains(err.Error(), `profiles.public.accessLists: "blocknet" has defaultAction: allow`) {
		t.Fatalf("ingress profile: err = %v", err)
	}
	dd := DockerDiscoverySettings{Enabled: true, Template: blocky}
	if err := dd.ValidateRefs(cfg); err == nil || !strings.Contains(err.Error(), `dockerDiscovery.template.accessLists: "blocknet" has defaultAction: allow`) {
		t.Fatalf("docker template: err = %v", err)
	}
	dd = DockerDiscoverySettings{Enabled: true, Template: IngressHostTemplate{TLS: TLSSettings{CertificateRef: "wildcard"}},
		Profiles: map[string]IngressHostTemplate{"edge": {AccessLists: []string{"no-kp"}, AccessListsMode: AccessListsModeAny}}}
	if err := dd.ValidateRefs(cfg); err == nil || !strings.Contains(err.Error(), `dockerDiscovery.profiles.edge.accessLists: "no-kp" has geo.countryDeny`) {
		t.Fatalf("docker profile: err = %v", err)
	}
}
