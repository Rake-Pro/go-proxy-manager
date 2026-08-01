package model

import (
	"testing"
	"time"
)

func TestAPITokenValidate(t *testing.T) {
	tests := []struct {
		name    string
		token   APIToken
		wantErr bool
	}{
		{"single read scope", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{"proxy-hosts:read"}}, false},
		{"write scope", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{"certificates:write"}}, false},
		{"wildcard scopes", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{"*:read", "*:write"}}, false},
		{"admin scope", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{ScopeAdmin}}, false},
		{"pseudo resources", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{"settings:read", "dns-sync:write"}}, false},
		{"no scopes", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}}, true},
		{"empty scope list", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{}}, true},
		{"unknown verb", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{"proxy-hosts:delete"}}, true},
		{"missing verb", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{"proxy-hosts"}}, true},
		{"unknown plural", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{"widgets:read"}}, true},
		{"empty plural", APIToken{ObjectMeta: ObjectMeta{Name: "ci"}, Scopes: []string{":read"}}, true},
		{"bad name", APIToken{ObjectMeta: ObjectMeta{Name: "Bad Name"}, Scopes: []string{"*:read"}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.token.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestScopeSatisfied(t *testing.T) {
	tests := []struct {
		name     string
		granted  []string
		required string
		want     bool
	}{
		{"exact read", []string{"proxy-hosts:read"}, "proxy-hosts:read", true},
		{"read does not imply write", []string{"proxy-hosts:read"}, "proxy-hosts:write", false},
		{"write implies read", []string{"proxy-hosts:write"}, "proxy-hosts:read", true},
		{"wrong resource", []string{"certificates:write"}, "proxy-hosts:read", false},
		{"wildcard read", []string{"*:read"}, "proxy-hosts:read", true},
		{"wildcard read is not write", []string{"*:read"}, "proxy-hosts:write", false},
		{"wildcard write covers all", []string{"*:write"}, "certificates:write", true},
		{"admin covers everything", []string{ScopeAdmin}, "api-tokens:write", true},
		{"admin required needs admin", []string{"*:write"}, ScopeAdmin, false},
		{"admin required satisfied", []string{"proxy-hosts:read", ScopeAdmin}, ScopeAdmin, true},
		{"concrete does not satisfy wildcard read", []string{"proxy-hosts:read"}, "*:read", false},
		{"wildcard satisfies wildcard", []string{"*:read"}, "*:read", true},
		{"empty grants nothing", nil, "proxy-hosts:read", false},
		{"garbage grant ignored", []string{"nonsense", "proxy-hosts:read"}, "proxy-hosts:read", true},
		{"garbage requirement denied", []string{ScopeAdmin + "x"}, "junk", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ScopeSatisfied(tc.granted, tc.required); got != tc.want {
				t.Fatalf("ScopeSatisfied(%v, %q) = %v, want %v", tc.granted, tc.required, got, tc.want)
			}
		})
	}
}

func TestAPITokenExpired(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	if (APIToken{}).Expired(now) {
		t.Fatal("a token with no expiry must never expire")
	}
	if !(APIToken{ExpiresAt: &past}).Expired(now) {
		t.Fatal("a past expiry must read as expired")
	}
	if (APIToken{ExpiresAt: &future}).Expired(now) {
		t.Fatal("a future expiry must not read as expired")
	}
}

func TestDNSSyncSettingsValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      DNSSyncSettings
		wantErr bool
	}{
		{"both disabled", DNSSyncSettings{}, false},
		{
			"pihole ok",
			DNSSyncSettings{Pihole: PiholeDNSSync{Enabled: true, URL: "http://pihole.lan", ApexTarget: "edge.example.com"}},
			false,
		},
		{
			"pihole bad url",
			DNSSyncSettings{Pihole: PiholeDNSSync{Enabled: true, URL: "pihole.lan", ApexTarget: "edge.example.com"}},
			true,
		},
		{
			"pihole missing apex",
			DNSSyncSettings{Pihole: PiholeDNSSync{Enabled: true, URL: "http://pihole.lan"}},
			true,
		},
		{
			"cloudflare ok",
			DNSSyncSettings{Cloudflare: CloudflareDNSSync{Enabled: true, DNSProviderRef: "cf", ZoneName: "example.com", ApexTarget: "edge.example.com"}},
			false,
		},
		{
			"cloudflare missing ref",
			DNSSyncSettings{Cloudflare: CloudflareDNSSync{Enabled: true, ZoneName: "example.com", ApexTarget: "edge.example.com"}},
			true,
		},
		{
			"cloudflare missing zone",
			DNSSyncSettings{Cloudflare: CloudflareDNSSync{Enabled: true, DNSProviderRef: "cf", ApexTarget: "edge.example.com"}},
			true,
		},
		{
			"cloudflare missing apex",
			DNSSyncSettings{Cloudflare: CloudflareDNSSync{Enabled: true, DNSProviderRef: "cf", ZoneName: "example.com"}},
			true,
		},
		{
			"disabled backends are not validated",
			DNSSyncSettings{Pihole: PiholeDNSSync{URL: "nonsense"}, Cloudflare: CloudflareDNSSync{ZoneName: ""}},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestSettingsValidateIncludesDNSSync(t *testing.T) {
	s := DefaultSettings()
	s.DNSSync.Pihole = PiholeDNSSync{Enabled: true, URL: "not-a-url"}
	if err := s.Validate(); err == nil {
		t.Fatal("settings validation must reject an invalid dnsSync block")
	}
}
