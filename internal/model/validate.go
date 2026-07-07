package model

import (
	"errors"
	"fmt"
	"strings"
)

// Validate checks every object individually and then verifies referential
// integrity across the whole config: no host may reference a certificate,
// middleware, access list, identity provider or DNS provider that does not
// exist. This turns the old "dangling reference / textual collision" runtime
// failure into a load-time error that blocks the commit.
func (c Config) Validate() error {
	var errs []error

	certs := map[string]bool{}
	clientCAs := map[string]bool{}
	disabledClientCAs := map[string]bool{}
	mws := map[string]bool{}
	als := map[string]bool{}
	idps := map[string]bool{}
	dns := map[string]bool{}

	// First pass: per-object validation + duplicate-name detection + name sets.
	register := func(kind, name string, seen map[string]bool) {
		if name == "" {
			return
		}
		if seen[name] {
			errs = append(errs, fmt.Errorf("duplicate %s name %q", kind, name))
		}
		seen[name] = true
	}

	for _, o := range c.Certificates {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("certificate", o.Name, certs)
	}
	for _, o := range c.ClientCAs {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("clientCA", o.Name, clientCAs)
		if o.Disabled {
			disabledClientCAs[o.Name] = true
		}
	}
	for _, o := range c.DNSProviders {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("dnsProvider", o.Name, dns)
	}
	for _, o := range c.Middlewares {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("middleware", o.Name, mws)
	}
	for _, o := range c.AccessLists {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("accessList", o.Name, als)
	}
	for _, o := range c.IdentityProviders {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("identityProvider", o.Name, idps)
	}

	seenHost := map[string]bool{}
	for _, h := range c.ProxyHosts {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("host", h.Name, seenHost)
		checkRef(&errs, "proxy host", h.Name, "certificate", h.TLS.CertificateRef, certs)
		checkClientAuthRef(&errs, "proxy host", h.Name, h.TLS, clientCAs, disabledClientCAs)
		for _, m := range h.Middlewares {
			checkRef(&errs, "proxy host", h.Name, "middleware", m, mws)
		}
		for _, a := range h.AccessLists {
			checkRef(&errs, "proxy host", h.Name, "accessList", a, als)
		}
		for _, l := range h.Locations {
			for _, m := range l.Middlewares {
				checkRef(&errs, "proxy host", h.Name+" location "+l.Path, "middleware", m, mws)
			}
			for _, a := range l.AccessLists {
				checkRef(&errs, "proxy host", h.Name+" location "+l.Path, "accessList", a, als)
			}
		}
	}
	for _, h := range c.RedirectHosts {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("host", h.Name, seenHost)
		checkRef(&errs, "redirect host", h.Name, "certificate", h.TLS.CertificateRef, certs)
		checkClientAuthRef(&errs, "redirect host", h.Name, h.TLS, clientCAs, disabledClientCAs)
	}
	for _, h := range c.DeadHosts {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("host", h.Name, seenHost)
		checkRef(&errs, "dead host", h.Name, "certificate", h.TLS.CertificateRef, certs)
		checkClientAuthRef(&errs, "dead host", h.Name, h.TLS, clientCAs, disabledClientCAs)
	}
	for _, h := range c.StreamHosts {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("host", h.Name, seenHost)
	}

	// Certificate -> DNS provider references.
	for _, ct := range c.Certificates {
		if ct.ACME != nil {
			checkRef(&errs, "certificate", ct.Name, "dnsProvider", ct.ACME.DNSProvider, dns)
		}
	}
	// Reject two enabled certificates claiming the same domain: SNI selection
	// would otherwise be order-dependent and could silently serve the wrong cert.
	certDomains := map[string]string{}
	for _, ct := range c.Certificates {
		if ct.Disabled {
			continue
		}
		for _, d := range ct.Domains {
			key := strings.ToLower(strings.TrimSpace(d))
			if key == "" {
				continue
			}
			if other, dup := certDomains[key]; dup {
				errs = append(errs, fmt.Errorf("certificates %q and %q both claim domain %q", other, ct.Name, key))
				continue
			}
			certDomains[key] = ct.Name
		}
	}
	// Auth middleware -> identity provider references.
	for _, m := range c.Middlewares {
		if m.Auth != nil {
			checkRef(&errs, "middleware", m.Name, "identityProvider", m.Auth.IdentityProvider, idps)
		}
	}

	return errors.Join(errs...)
}

// checkClientAuthRef verifies a host's mTLS trust anchor resolves to a known,
// ENABLED ClientCA. A missing CA fails closed (refusing every client); a disabled
// CA is worse - it is excluded from the compiled CA pools, yielding a nil pool and
// a hard TLS-config error that fails the entire router reload. Catching both at
// validation turns that opaque ops footgun into a clear load-time rejection.
func checkClientAuthRef(errs *[]error, ownerKind, ownerName string, tlsSettings TLSSettings, set, disabled map[string]bool) {
	if tlsSettings.ClientAuth == nil {
		return
	}
	ref := tlsSettings.ClientAuth.CARef
	checkRef(errs, ownerKind, ownerName, "clientCA", ref, set)
	if ref != "" && set[ref] && disabled[ref] {
		*errs = append(*errs, fmt.Errorf("%s %q references clientCA %q, which is disabled (a disabled trust anchor would fail the TLS reload; enable it or remove the mTLS requirement)", ownerKind, ownerName, ref))
	}
}

func checkRef(errs *[]error, ownerKind, ownerName, refKind, ref string, set map[string]bool) {
	if ref == "" {
		return
	}
	if !set[ref] {
		*errs = append(*errs, fmt.Errorf("%s %q references unknown %s %q", ownerKind, ownerName, refKind, ref))
	}
}
