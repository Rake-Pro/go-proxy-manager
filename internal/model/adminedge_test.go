package model

import (
	"strings"
	"testing"
)

func TestAdminEdgeHostname(t *testing.T) {
	for in, want := range map[string]string{
		"https://gpm.example.com":       "gpm.example.com",
		"https://GPM.example.com:8443/": "gpm.example.com",
		"":                              "",
		"not a url":                     "",
	} {
		if got := AdminEdgeHostname(in); got != want {
			t.Errorf("AdminEdgeHostname(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAdminEdgeValidateNeedsExternalBaseURL(t *testing.T) {
	s := Settings{AdminAuth: AdminAuthSettings{LocalLoginEnabled: true}, AdminEdge: AdminEdgeSettings{Enabled: true}}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "externalBaseURL") {
		t.Fatalf("want externalBaseURL error, got %v", err)
	}
	s.ExternalBaseURL = "https://gpm.example.com"
	if err := s.Validate(); err != nil {
		t.Fatalf("enabled with a URL must validate: %v", err)
	}
	// Shape rules are the proxy host's own.
	s.AdminEdge.Middlewares = []string{"bad name!"}
	if err := s.Validate(); err == nil {
		t.Fatal("a bad middleware name must fail settings validation")
	}
}

func TestAdminEdgeHostAndRefs(t *testing.T) {
	a := AdminEdgeSettings{Enabled: true, AccessLists: []string{"lan"}, Middlewares: []string{"mw"},
		TLS: TLSSettings{ForceSSL: true}, DNS: &DNSSyncPolicy{LanDirect: true}, RobotsNoIndex: true}
	h := a.Host("gpm.example.com", Upstream{Scheme: "http", Host: "127.0.0.1", Port: 8081})
	if h.Name != AdminEdgeHostName || h.Domains[0] != "gpm.example.com" || h.Upstream.Port != 8081 || !h.TLS.ForceSSL || h.DNS == nil || !h.RobotsNoIndex {
		t.Fatalf("host = %+v", h)
	}
	if h.Labels[DefaultAnnotationPrefix+"/managed-by"] != ManagedByAdminEdge {
		t.Fatalf("labels = %v", h.Labels)
	}
	if err := h.Validate(); err != nil {
		t.Fatalf("compiled host must validate: %v", err)
	}

	cfg := Config{
		Middlewares: []Middleware{{ObjectMeta: ObjectMeta{Name: "mw"}, Type: MWTypeHeaders}},
		AccessLists: []AccessList{{ObjectMeta: ObjectMeta{Name: "lan"}}},
	}
	if err := a.ValidateRefs(cfg, "https://gpm.example.com"); err != nil {
		t.Fatalf("refs resolve: %v", err)
	}
	a2 := a
	a2.AccessLists = []string{"nope"}
	if err := a2.ValidateRefs(cfg, "https://gpm.example.com"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("missing access list must be named: %v", err)
	}
	// A stored host on the panel's hostname is a collision; a disabled one is not.
	cfg.ProxyHosts = []ProxyHost{{ObjectMeta: ObjectMeta{Name: "old"}, Domains: []string{"GPM.example.com"}, Upstream: Upstream{Scheme: "http", Host: "127.0.0.1", Port: 8081}}}
	if err := a.ValidateRefs(cfg, "https://gpm.example.com"); err == nil || !strings.Contains(err.Error(), `"old"`) {
		t.Fatalf("collision must name the host: %v", err)
	}
	cfg.ProxyHosts[0].Disabled = true
	if err := a.ValidateRefs(cfg, "https://gpm.example.com"); err != nil {
		t.Fatalf("a disabled host does not collide: %v", err)
	}
	cfg.ProxyHosts = []ProxyHost{{ObjectMeta: ObjectMeta{Name: AdminEdgeHostName}, Domains: []string{"x.example.com"}}}
	if err := a.ValidateRefs(cfg, "https://gpm.example.com"); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved name must be refused: %v", err)
	}
	// Off: nothing is checked.
	if err := (AdminEdgeSettings{}).ValidateRefs(cfg, "https://gpm.example.com"); err != nil {
		t.Fatalf("disabled block must not check: %v", err)
	}
}

func TestMaintenanceExemptDomains(t *testing.T) {
	cfg := Config{IdentityProviders: []IdentityProvider{
		{ObjectMeta: ObjectMeta{Name: "sso"}, Type: IdPTypeOIDC, OIDC: &OIDCSpec{IssuerURL: "https://auth.example.com/application/o/gpm/", ClientID: "x"}},
		{ObjectMeta: ObjectMeta{Name: "other"}, Type: IdPTypeOIDC, OIDC: &OIDCSpec{IssuerURL: "https://else.example.com/", ClientID: "y"}},
	}}
	s := Settings{ExternalBaseURL: "https://gpm.example.com", AdminAuth: AdminAuthSettings{Providers: []string{"sso"}},
		AdminEdge: AdminEdgeSettings{Enabled: true}}
	got := MaintenanceExemptDomains(cfg, s)
	if strings.Join(got, ",") != "gpm.example.com,auth.example.com" {
		t.Fatalf("exempt = %v", got)
	}
	s.AdminEdge.Enabled = false
	if got := MaintenanceExemptDomains(cfg, s); strings.Join(got, ",") != "auth.example.com" {
		t.Fatalf("exempt without the edge = %v", got)
	}
}
