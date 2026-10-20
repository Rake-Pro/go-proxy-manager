package model

import (
	"fmt"
	"net/url"
	"strings"
)

// AdminEdgeHostName is the reserved proxy host name the admin edge compiles to.
// A stored proxy host may not use it, so the synthetic host can never collide
// with an operator's object.
const AdminEdgeHostName = "gpm-admin"

// ManagedByAdminEdge is the managed-by label value on the synthetic host, so a
// consumer that sees it (the data plane, DNS sync, discovery ownership) can
// tell it from a stored host.
const ManagedByAdminEdge = "admin-edge"

// AdminEdgeSettings serves the admin panel at the edge, on the hostname of
// externalBaseURL, through gpm's own proxy listeners: TLS by SNI like every
// other domain, the access lists and middlewares named here, and a loopback
// hop to the admin listener. It replaces the hand-written proxy host that used
// to do this (see docs/how-to/admin-ui-behind-gpm.md), and it is NOT a proxy
// host: it is never listed, disabled, deleted or discovery-managed, and the
// fleet-wide maintenance switch never applies to it, so an operator can always
// reach the panel to turn maintenance back off.
//
// The direct admin listener keeps working unchanged: this is a second door,
// not a replacement for the break-glass one.
type AdminEdgeSettings struct {
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// TLS is applied exactly as on a proxy host. certificateRef is optional:
	// the handshake picks a certificate by domain. forceSSL, HSTS, minTLSVersion
	// and clientAuth all work.
	TLS TLSSettings `json:"tls,omitempty" yaml:"tls,omitempty"`

	// Middlewares and AccessLists are the chain in front of the panel, in the
	// same order and with the same semantics as on a proxy host. The panel has
	// its own login; a sign-in middleware here gates it a second time.
	Middlewares []string `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	AccessLists []string `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`

	// DNS is the DNS sync policy for the panel's hostname, as on a proxy host.
	DNS *DNSSyncPolicy `json:"dns,omitempty" yaml:"dns,omitempty"`

	// RobotsNoIndex sends X-Robots-Tag: noindex, nofollow, as on a proxy host.
	RobotsNoIndex bool `json:"robotsNoIndex,omitempty" yaml:"robotsNoIndex,omitempty"`
}

// Hostname returns the host the panel is served on: externalBaseURL's host
// without a port. Empty when externalBaseURL is unset or unparseable.
func AdminEdgeHostname(externalBaseURL string) string {
	u, err := url.Parse(strings.TrimSpace(externalBaseURL))
	if err != nil {
		return ""
	}
	name := NormalizeHostname(u.Hostname())
	if !IsHostname(name) {
		// A wildcard, an IP literal or a one-label name is not a hostname the
		// edge can serve the panel on.
		return ""
	}
	return name
}

// validate shape-checks the block at settings-write time with the proxy host's
// own rules, by building the host it compiles to and validating that.
func (a AdminEdgeSettings) validate(externalBaseURL string) error {
	if !a.Enabled {
		return nil
	}
	name := AdminEdgeHostname(externalBaseURL)
	if name == "" {
		return fmt.Errorf("adminEdge.enabled: needs externalBaseURL with a servable hostname (not a wildcard, an IP address or a single label) to name the host the panel is served on")
	}
	if err := a.Host(name, Upstream{Scheme: "http", Host: "127.0.0.1", Port: 1}).Validate(); err != nil {
		return fmt.Errorf("adminEdge: %w", err)
	}
	// Reference NAMES are shape-checked here; whether they resolve is
	// ValidateRefs' job, which needs the rest of the config.
	for i, m := range a.Middlewares {
		if err := ValidateName(m); err != nil {
			return fmt.Errorf("adminEdge.middlewares[%d]: %w", i, err)
		}
	}
	for i, l := range a.AccessLists {
		if err := ValidateName(l); err != nil {
			return fmt.Errorf("adminEdge.accessLists[%d]: %w", i, err)
		}
	}
	return nil
}

// Host is the proxy host the block compiles to. upstream is the loopback
// address of the admin listener, supplied by the process that knows it.
func (a AdminEdgeSettings) Host(hostname string, upstream Upstream) ProxyHost {
	h := ProxyHost{
		ObjectMeta: ObjectMeta{
			Name:        AdminEdgeHostName,
			DisplayName: "Admin panel",
			Labels:      map[string]string{DefaultAnnotationPrefix + "/managed-by": ManagedByAdminEdge},
		},
		AdminEdge:     true,
		Domains:       []string{hostname},
		Upstream:      upstream,
		TLS:           a.TLS,
		RobotsNoIndex: a.RobotsNoIndex,
		Middlewares:   append([]string(nil), a.Middlewares...),
		AccessLists:   append([]string(nil), a.AccessLists...),
	}
	if a.DNS != nil {
		d := *a.DNS
		h.DNS = &d
	}
	return h
}

// ValidateRefs cross-checks the block against the rest of the config: every
// middleware and access list it names exists, and no enabled stored host
// claims the panel's hostname (the router keys by domain, so a stored host
// sorting after the synthetic one would silently replace the panel's chain).
// Called on every settings write and every object write while the block is
// enabled, so neither side can create the collision.
func (a AdminEdgeSettings) ValidateRefs(cfg Config, externalBaseURL string) error {
	if !a.Enabled {
		return nil
	}
	mws := map[string]bool{}
	for _, o := range cfg.Middlewares {
		mws[o.Name] = true
	}
	for _, m := range a.Middlewares {
		if !mws[m] {
			return fmt.Errorf("adminEdge.middlewares: names middleware %q, which does not exist", m)
		}
	}
	als := map[string]bool{}
	for _, o := range cfg.AccessLists {
		als[o.Name] = true
	}
	for _, l := range a.AccessLists {
		if !als[l] {
			return fmt.Errorf("adminEdge.accessLists: names access list %q, which does not exist", l)
		}
	}
	name := AdminEdgeHostname(externalBaseURL)
	claims := func(kind, obj string, disabled bool, domains []string) error {
		if disabled {
			return nil
		}
		for _, d := range domains {
			if strings.EqualFold(strings.TrimSpace(d), name) {
				return fmt.Errorf("%s %q: domains: %s is the admin panel's hostname (settings.adminEdge serves it); delete that host or turn adminEdge off", kind, obj, name)
			}
		}
		return nil
	}
	for _, h := range cfg.ProxyHosts {
		if h.Name == AdminEdgeHostName {
			return fmt.Errorf("proxy host %q: the name is reserved for the admin edge", h.Name)
		}
		if err := claims("proxy host", h.Name, h.Disabled, h.Domains); err != nil {
			return err
		}
	}
	for _, h := range cfg.RedirectHosts {
		if err := claims("redirect host", h.Name, h.Disabled, h.Domains); err != nil {
			return err
		}
	}
	for _, h := range cfg.ParkedHosts {
		if err := claims("parked host", h.Name, h.Disabled, h.Domains); err != nil {
			return err
		}
	}
	return nil
}

// MaintenanceExemptDomains lists the hostnames the fleet-wide maintenance
// switch must leave up so the panel stays reachable to turn it off again: the
// admin edge itself, and the hostname of every admin sign-in provider's issuer
// (an OIDC login bounces through it; with it down, so is the login). A
// per-host maintenance flag on one of these is still honoured: only the
// fleet-wide switch is bypassed.
func MaintenanceExemptDomains(cfg Config, s Settings) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	if s.AdminEdge.Enabled {
		add(AdminEdgeHostname(s.ExternalBaseURL))
	}
	for _, pname := range s.AdminAuth.Providers {
		for _, p := range cfg.IdentityProviders {
			if p.Name == pname && p.OIDC != nil {
				add(AdminEdgeHostname(p.OIDC.IssuerURL))
			}
		}
	}
	return out
}
