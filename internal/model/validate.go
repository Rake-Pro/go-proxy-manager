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
	ugs := map[string]bool{}
	disabledUGs := map[string]bool{}

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
	// idpTypes carries each provider's type as well as its existence: an auth
	// middleware that leaves mode unset inherits it from the provider, so a
	// mode-dependent rule can only be checked once every provider is known.
	idpTypes := map[string]string{}
	for _, o := range c.IdentityProviders {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("identityProvider", o.Name, idps)
		idpTypes[o.Name] = o.Type
	}
	for _, o := range c.UpstreamGroups {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("upstreamGroup", o.Name, ugs)
		if o.Disabled {
			disabledUGs[o.Name] = true
		}
	}
	tokens := map[string]bool{}
	for _, o := range c.APITokens {
		if err := o.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("apiToken", o.Name, tokens)
	}

	seenHost := map[string]bool{}
	for _, h := range c.ProxyHosts {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("host", h.Name, seenHost)
		checkRef(&errs, "proxy host", h.Name, "certificate", h.TLS.CertificateRef, certs)
		checkClientAuthRef(&errs, "proxy host", h.Name, h.TLS, clientCAs, disabledClientCAs)
		// A disabled group is excluded from the compiled health state, so an enabled
		// host referencing it would fail the whole router build; reject at validation
		// like a disabled clientCA.
		checkGroupRef := func(owner, ref string) {
			checkRef(&errs, "proxy host", owner, "upstreamGroup", ref, ugs)
			if !h.Disabled && ref != "" && ugs[ref] && disabledUGs[ref] {
				errs = append(errs, fmt.Errorf("proxy host %q references upstreamGroup %q, which is disabled (enable the group or use a single upstream)", owner, ref))
			}
		}
		checkGroupRef(h.Name, h.UpstreamGroupRef)
		for _, m := range h.Middlewares {
			checkRef(&errs, "proxy host", h.Name, "middleware", m, mws)
		}
		for _, a := range h.AccessLists {
			checkRef(&errs, "proxy host", h.Name, "accessList", a, als)
		}
		for _, l := range h.Locations {
			checkGroupRef(h.Name+" location "+l.Path, l.UpstreamGroupRef)
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
	for _, h := range c.ParkedHosts {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("host", h.Name, seenHost)
		checkRef(&errs, "parked host", h.Name, "certificate", h.TLS.CertificateRef, certs)
		checkClientAuthRef(&errs, "parked host", h.Name, h.TLS, clientCAs, disabledClientCAs)
	}
	// Stream hosts: per-object validation, then the two cross-object rules that
	// need the whole config - what an access list actually contains, and who else
	// is already on the listen port.
	basicAuthLists := map[string]bool{}
	requestScopedLists := map[string]bool{}
	for _, a := range c.AccessLists {
		if len(a.BasicAuth) > 0 {
			basicAuthLists[a.Name] = true
		}
		if a.HasRequestScopedRules() {
			requestScopedLists[a.Name] = true
		}
	}
	type portClaim struct {
		host string
		sni  []string
	}
	tcpPorts := map[int][]portClaim{}
	udpPorts := map[int]string{}
	for _, h := range c.StreamHosts {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		register("host", h.Name, seenHost)
		if h.TLS != nil {
			checkRef(&errs, "stream host", h.Name, "certificate", h.TLS.CertificateRef, certs)
		}
		for _, a := range h.AccessLists {
			checkRef(&errs, "stream host", h.Name, "accessList", a, als)
			// Basic auth is an HTTP challenge/response; a raw TCP/UDP stream has no
			// way to issue one. Reject the reference rather than silently evaluating
			// only half the list an operator believed was gating the port.
			if basicAuthLists[a] {
				errs = append(errs, fmt.Errorf("stream host %q references accessList %q, which has basicAuth users: basic auth cannot be evaluated on a raw stream (use a list with only ip rules and/or geo, or attach this list to a proxy host)", h.Name, a))
			}
			// Same reasoning one step further: a path-scoped rule needs a request
			// path and a source-backed rule needs the fetched ledger the HTTP data
			// plane resolves. A raw stream has neither, so the reference is
			// rejected rather than evaluated as half the gate the operator wrote.
			if requestScopedLists[a] {
				errs = append(errs, fmt.Errorf("stream host %q references accessList %q, which has path-scoped and/or source-backed rules: those need an HTTP request path and cannot be evaluated on a raw stream (use a list with only literal cidr rules and/or geo, or attach this list to a proxy host)", h.Name, a))
			}
		}
		if h.Disabled {
			continue
		}
		if h.Protocol == "tcp" || h.Protocol == "both" {
			tcpPorts[h.ListenPort] = append(tcpPorts[h.ListenPort], portClaim{host: h.Name, sni: h.SNINames()})
		}
		if h.Protocol == "udp" || h.Protocol == "both" {
			if other, dup := udpPorts[h.ListenPort]; dup {
				errs = append(errs, fmt.Errorf("stream hosts %q and %q both listen on udp port %d (a udp port carries no SNI and cannot be shared; give one of them a different port)", other, h.Name, h.ListenPort))
			} else {
				udpPorts[h.ListenPort] = h.Name
			}
		}
	}
	// A TCP listen port may be shared ONLY when every host on it routes by SNI -
	// that is the sole thing that tells two connections apart. Without it the
	// forwarder would have to pick one backend arbitrarily (previously: whichever
	// host compiled last), which is exactly the order-dependent routing the
	// duplicate-domain check above exists to prevent.
	for port, claims := range tcpPorts {
		if len(claims) < 2 {
			continue
		}
		names := map[string]string{}
		for _, cl := range claims {
			if len(cl.sni) == 0 {
				errs = append(errs, fmt.Errorf("stream host %q shares tcp port %d with another stream host but sets no tls.sniMatch (hosts may only share a port when every one of them routes by SNI)", cl.host, port))
				continue
			}
			for _, n := range cl.sni {
				if other, dup := names[n]; dup {
					errs = append(errs, fmt.Errorf("stream hosts %q and %q both claim sni %q on tcp port %d", other, cl.host, n, port))
					continue
				}
				names[n] = cl.host
			}
		}
	}

	// Reject two ENABLED hosts claiming the same domain. The router keys its
	// host/redirect/dead maps by domain, so a duplicate is resolved by whichever
	// object happens to be compiled last (i.e. by YAML filename order), not by
	// intent - and an automated writer such as Ingress discovery could therefore
	// shadow an operator-authored host simply by sorting after it. Making a
	// duplicate a load-time error means no source of writes can produce one.
	//
	// The check is scoped to ENABLED hosts on purpose: a disabled host is excluded
	// from the compiled data plane entirely, so staging a replacement host beside
	// the live one (disable old, enable new, or vice versa) stays legal.
	hostDomains := map[string]string{}
	claimDomains := func(name string, disabled bool, domains []string) {
		if disabled {
			return
		}
		for _, d := range domains {
			key := strings.ToLower(strings.TrimSpace(d))
			if key == "" {
				continue
			}
			if other, dup := hostDomains[key]; dup {
				errs = append(errs, fmt.Errorf("hosts %q and %q both claim domain %q (an enabled domain may be served by exactly one host; disable one of them)", other, name, key))
				continue
			}
			hostDomains[key] = name
		}
	}
	for _, h := range c.ProxyHosts {
		claimDomains(h.Name, h.Disabled, h.Domains)
	}
	for _, h := range c.RedirectHosts {
		claimDomains(h.Name, h.Disabled, h.Domains)
	}
	for _, h := range c.ParkedHosts {
		claimDomains(h.Name, h.Disabled, h.Domains)
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
			checkAuthAllowFromMode(&errs, m, idpTypes)
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

// checkAuthAllowFromMode closes the gap the per-object validator cannot: an auth
// middleware with allowFrom and NO explicit mode inherits its mode from the
// referenced provider's type, which only the whole config knows. Middleware.Validate
// already refuses allowFrom against an explicit oidc/forward-auth mode; without
// this, the same middleware written with mode omitted passes validation and then
// silently drops the exemption at runtime (internal/dataplane/auth.go resolves the
// mode from idp.Type, and neither of those branches is handed the parsed networks).
//
// A silently ignored network exemption is the worst failure shape available here:
// the operator believes the LAN is exempt, every LAN client is challenged instead,
// and nothing anywhere says why. An unresolvable provider is refused for the same
// reason - the effective mode cannot be established, so neither can the promise.
func checkAuthAllowFromMode(errs *[]error, m Middleware, idpTypes map[string]string) {
	if len(m.Auth.AllowFrom) == 0 || m.Auth.Mode != "" {
		return // explicit modes are settled by Middleware.Validate
	}
	t, ok := idpTypes[m.Auth.IdentityProvider]
	if !ok {
		// The dangling-reference error above already names the missing provider;
		// this adds why it matters for allowFrom specifically.
		*errs = append(*errs, fmt.Errorf("middleware %q: auth.allowFrom needs a known auth mode, but auth.mode is unset and identityProvider %q does not resolve, so the effective mode cannot be determined - set auth.mode explicitly",
			m.Name, m.Auth.IdentityProvider))
		return
	}
	switch t {
	case IdPTypeAuthRequest:
	default:
		*errs = append(*errs, fmt.Errorf("middleware %q: auth.allowFrom is only supported in auth-request and client-cert modes; auth.mode is unset, so it defaults to identityProvider %q's type (%q), where the exemption would be silently ignored - set auth.mode explicitly or remove auth.allowFrom",
			m.Name, m.Auth.IdentityProvider, t))
	}
}
