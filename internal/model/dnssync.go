package model

import (
	"fmt"
	"net/url"
)

// DNSSyncPolicy opts a proxy host into automatic DNS record management. It is
// per-host and per-backend, so a host can be resolvable on the LAN, publicly, or
// both, without gpm ever guessing.
type DNSSyncPolicy struct {
	// LanDirect publishes each of the host's domains as a local CNAME on the
	// LAN resolver (Pi-hole), pointing at the edge so internal clients reach gpm
	// directly instead of hairpinning through the WAN address.
	LanDirect bool `json:"lanDirect,omitempty" yaml:"lanDirect,omitempty"`
	// PublicCname publishes each of the host's domains as a public CNAME in the
	// authoritative zone (Cloudflare), pointing at the public apex target.
	PublicCname bool `json:"publicCname,omitempty" yaml:"publicCname,omitempty"`
}

// Enabled reports whether the policy asks for any DNS record at all.
func (p DNSSyncPolicy) Enabled() bool { return p.LanDirect || p.PublicCname }

// PiholeDNSSync configures the local (LAN) DNS backend: a Pi-hole v6 instance
// whose CNAME records are reconciled from the proxy hosts opted into lanDirect.
type PiholeDNSSync struct {
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	// URL is the Pi-hole base URL, e.g. http://pihole.lan (no /api suffix).
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// AppPassword is the Pi-hole application password used for POST /api/auth.
	// Stored as a ${ENV:...}/${FILE:...} placeholder like every other secret.
	AppPassword Secret `json:"appPassword,omitempty" yaml:"appPassword,omitempty"`
	// ApexTarget is the CNAME target every managed record points at (the edge
	// hostname resolvable on the LAN). It is NOT an ownership marker: it once was,
	// and treating a hand-written CNAME aimed here as gpm's cost an operator 19
	// records (see DNSLedger). Deletion is authorised by the ledger alone.
	ApexTarget string `json:"apexTarget,omitempty" yaml:"apexTarget,omitempty"`
}

// CloudflareDNSSync configures the public DNS backend. The API credential is not
// duplicated here: DNSProviderRef names an existing dns-providers object and its
// config["apiToken"] is reused, so a token rotation happens in one place.
type CloudflareDNSSync struct {
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	// DNSProviderRef names a DNSProvider object whose config["apiToken"] is the
	// Cloudflare API token used for record management.
	DNSProviderRef string `json:"dnsProviderRef,omitempty" yaml:"dnsProviderRef,omitempty"`
	// ZoneName is the Cloudflare zone the records live in, e.g. example.com.
	ZoneName string `json:"zoneName,omitempty" yaml:"zoneName,omitempty"`
	// ApexTarget is the CNAME content every managed record points at.
	ApexTarget string `json:"apexTarget,omitempty" yaml:"apexTarget,omitempty"`
	// Proxied sets the Cloudflare orange-cloud flag on created records
	// (default false: DNS-only, traffic goes straight to the edge).
	Proxied bool `json:"proxied,omitempty" yaml:"proxied,omitempty"`
}

// DNSSyncSettings is the instance-wide DNS sync configuration. Each backend is
// independently enabled; both disabled (the default) means the subsystem is
// inert and never contacts anything.
type DNSSyncSettings struct {
	Pihole     PiholeDNSSync     `json:"pihole,omitempty" yaml:"pihole,omitempty"`
	Cloudflare CloudflareDNSSync `json:"cloudflare,omitempty" yaml:"cloudflare,omitempty"`
}

// Validate checks each enabled backend has everything it needs. The Cloudflare
// dnsProviderRef is only checked for name shape here: Settings is a separate
// singleton from the object graph, so the reference is resolved (and reported)
// at reconcile time rather than at settings-write time.
func (d DNSSyncSettings) Validate() error {
	if d.Pihole.Enabled {
		u, err := url.Parse(d.Pihole.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("settings: dnsSync.pihole.url must be an absolute http(s) URL, got %q", d.Pihole.URL)
		}
		if d.Pihole.ApexTarget == "" {
			return fmt.Errorf("settings: dnsSync.pihole.apexTarget is required when pihole sync is enabled")
		}
	}
	if d.Cloudflare.Enabled {
		if err := ValidateName(d.Cloudflare.DNSProviderRef); err != nil {
			return fmt.Errorf("settings: dnsSync.cloudflare.dnsProviderRef: %w", err)
		}
		if d.Cloudflare.ZoneName == "" {
			return fmt.Errorf("settings: dnsSync.cloudflare.zoneName is required when cloudflare sync is enabled")
		}
		if d.Cloudflare.ApexTarget == "" {
			return fmt.Errorf("settings: dnsSync.cloudflare.apexTarget is required when cloudflare sync is enabled")
		}
	}
	return nil
}
