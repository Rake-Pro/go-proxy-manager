package model

import (
	"strings"
	"testing"
)

// A template that passes ValidateRefs must yield derived hosts that pass the
// reference part of Config.Validate, at host level and per location, for the
// template and for profiles, in both discovery blocks. Each case is also run
// through Config.Validate on a host built the way that block derives it (Docker:
// the container's upstream, never the template's upstream or group), so the two
// checks are shown to agree, and Docker never refuses a group it ignores.
func TestDiscoveryValidateRefsMatchesHostValidation(t *testing.T) {
	up := Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80}
	pool := func(disabled bool) UpstreamGroup {
		return UpstreamGroup{ObjectMeta: ObjectMeta{Name: "pool", Disabled: disabled}, Upstreams: []GroupUpstream{{Upstream: up}}}
	}
	vpn := AccessList{ObjectMeta: ObjectMeta{Name: "vpn"}, Rules: []IPRule{{CIDR: "192.0.2.0/24", Action: "allow"}}}
	cert := Certificate{ObjectMeta: ObjectMeta{Name: "wildcard"}, Type: CertTypeCustom, Domains: []string{"*.example.com"},
		Custom: &CustomCertSpec{CertFile: "wildcard.pem", KeyFile: "wildcard-key.pem"}}
	loc := func(l Location) []Location {
		l.Path = "/admin"
		if l.Upstream == nil && l.UpstreamGroupRef == "" {
			l.Upstream = &up
		}
		return []Location{l}
	}
	tests := []struct {
		name    string
		groups  []UpstreamGroup
		lists   []AccessList
		tmpl    IngressHostTemplate
		wantErr string
		// dockerOK marks an error about the template's own upstream group,
		// which Docker derive does not stamp.
		dockerOK bool
	}{
		{"all references resolve", []UpstreamGroup{pool(false)}, []AccessList{vpn},
			IngressHostTemplate{UpstreamGroupRef: "pool", Locations: loc(Location{AccessLists: []string{"vpn"}})}, "", false},
		{"location access list deleted", []UpstreamGroup{pool(false)}, nil,
			IngressHostTemplate{UpstreamGroupRef: "pool", Locations: loc(Location{AccessLists: []string{"vpn"}})},
			`locations: location "/admin": accessLists: references unknown accessList "vpn"`, false},
		{"upstream group disabled", []UpstreamGroup{pool(true)}, []AccessList{vpn},
			IngressHostTemplate{UpstreamGroupRef: "pool"},
			`upstreamGroupRef: upstreamGroup "pool" is disabled`, true},
		{"upstream group missing", nil, nil,
			IngressHostTemplate{UpstreamGroupRef: "pool"},
			`upstreamGroupRef: references unknown upstreamGroup "pool"`, true},
		{"location middleware missing", nil, nil,
			IngressHostTemplate{Upstream: up, Locations: loc(Location{Middlewares: []string{"sso"}})},
			`locations: location "/admin": middlewares: references unknown middleware "sso"`, false},
		{"location upstream group missing", nil, nil,
			IngressHostTemplate{Upstream: up, Locations: loc(Location{UpstreamGroupRef: "pool"})},
			`locations: location "/admin": upstreamGroupRef: references unknown upstreamGroup "pool"`, false},
		{"location upstream group disabled", []UpstreamGroup{pool(true)}, nil,
			IngressHostTemplate{Upstream: up, Locations: loc(Location{UpstreamGroupRef: "pool"})},
			`locations: location "/admin": upstreamGroupRef: upstreamGroup "pool" is disabled`, false},
		{"location rateLimit allowFrom list missing", nil, nil,
			IngressHostTemplate{Upstream: up, Locations: loc(Location{RateLimit: &RateLimitMiddleware{RequestsPerSecond: 5, AllowFrom: []string{"@vpn"}}})},
			`locations: location "/admin": rateLimit.allowFrom: references unknown accessList "vpn"`, false},
	}
	for _, tc := range tests {
		tmpl := tc.tmpl
		tmpl.TLS = TLSSettings{CertificateRef: "wildcard"}
		cfg := Config{Certificates: []Certificate{cert}, UpstreamGroups: tc.groups, AccessLists: tc.lists}

		blocks := []struct {
			name, path string
			docker     bool
			refs       func() error
		}{
			{"ingress template", "ingressDiscovery.template.", false, func() error {
				return IngressDiscoverySettings{Enabled: true, Template: tmpl}.ValidateRefs(cfg)
			}},
			{"ingress profile", "ingressDiscovery.profiles.p.", false, func() error {
				return IngressDiscoverySettings{Enabled: true, Template: IngressHostTemplate{Upstream: up},
					Profiles: map[string]IngressHostTemplate{"p": tmpl}}.ValidateRefs(cfg)
			}},
			{"docker profile", "dockerDiscovery.profiles.p.", true, func() error {
				d := dockerBase()
				d.Profiles = map[string]IngressHostTemplate{"p": tmpl}
				return d.ValidateRefs(cfg)
			}},
		}
		for _, b := range blocks {
			t.Run(tc.name+"/"+b.name, func(t *testing.T) {
				wantErr := tc.wantErr
				if b.docker && tc.dockerOK {
					wantErr = ""
				}
				err := b.refs()
				if wantErr == "" {
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), b.path+wantErr) {
					t.Fatalf("error %v, want one containing %q", err, b.path+wantErr)
				}
				// The derived host agrees.
				host := ProxyHost{ObjectMeta: ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"},
					Upstream: tmpl.Upstream, UpstreamGroupRef: tmpl.UpstreamGroupRef, TLS: tmpl.TLS, Locations: tmpl.Locations}
				if b.docker {
					host.Upstream, host.UpstreamGroupRef = Upstream{Scheme: "http", Host: "192.0.2.20", Port: 8080}, ""
				}
				withHost := cfg
				withHost.ProxyHosts = []ProxyHost{host}
				if herr := withHost.Validate(); (herr == nil) != (wantErr == "") ||
					(herr != nil && !strings.Contains(herr.Error(), wantErr)) {
					t.Fatalf("derived host: Config.Validate = %v, want agreement with %q", herr, wantErr)
				}
			})
		}
	}
}
