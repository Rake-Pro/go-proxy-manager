package auth

import (
	"net"
	"net/http"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// ForwardAuth accepts a trusted forward-auth identity asserted by an upstream
// authenticator (e.g. Authentik). This is the headline "one sign-in, no second
// click" path: instead of forcing a separate OIDC round-trip, we accept the
// identity the upstream already proved - BUT only when the request demonstrably
// came from a trusted proxy. Identity headers from any other source are
// untrusted and must be stripped so a client cannot spoof them.
type ForwardAuth struct {
	idpName string
	trusted []*net.IPNet
	userH   string
	emailH  string
	nameH   string
	groupsH string
	amrH    string
	delim   string
}

// CompileForwardAuth precomputes the trusted networks and header names.
func CompileForwardAuth(spec model.ForwardAuthSpec, idpName string) ForwardAuth {
	f := ForwardAuth{
		idpName: idpName,
		userH:   spec.UserHeader,
		emailH:  spec.EmailHeader,
		nameH:   spec.NameHeader,
		groupsH: spec.GroupsHeader,
		amrH:    spec.AMRHeader,
		delim:   spec.GroupsDelimiter,
	}
	if f.delim == "" {
		f.delim = ","
	}
	for _, c := range spec.TrustedProxies {
		if _, n, err := net.ParseCIDR(c); err == nil {
			f.trusted = append(f.trusted, n)
		} else if ip := net.ParseIP(c); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			f.trusted = append(f.trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	return f
}

// ClientTrusted reports whether the request's immediate peer is a trusted proxy.
// It deliberately uses the connection's RemoteAddr, never a forwarded header -
// trust must be established by who connected to us, not by what they claim.
func (f ForwardAuth) ClientTrusted(r *http.Request) bool {
	ip := connIP(r)
	if ip == nil {
		return false
	}
	for _, n := range f.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Identity extracts the asserted identity, but only if the peer is trusted and a
// username was actually asserted. The boolean is false when no trusted identity
// is present (caller should then fall back to another auth method, not grant).
func (f ForwardAuth) Identity(r *http.Request) (Identity, bool) {
	if !f.ClientTrusted(r) {
		return Identity{}, false
	}
	user := strings.TrimSpace(r.Header.Get(f.userH))
	if user == "" {
		return Identity{}, false
	}
	id := Identity{
		Subject: user,
		Name:    headerOr(r, f.nameH, user),
		Email:   strings.TrimSpace(r.Header.Get(f.emailH)),
		IdP:     f.idpName,
		// AMR reflects only what the upstream actually asserted via amrH; we never
		// fabricate "mfa" the upstream didn't prove.
		AMR: f.amr(r),
	}
	if f.groupsH != "" {
		if raw := strings.TrimSpace(r.Header.Get(f.groupsH)); raw != "" {
			for _, g := range strings.Split(raw, f.delim) {
				if g = strings.TrimSpace(g); g != "" {
					id.Groups = append(id.Groups, g)
				}
			}
		}
	}
	return id, true
}

// Strip removes the identity headers from a request. It MUST be called before
// proxying any request that did not come from a trusted proxy, so a client
// cannot inject forged identity headers to the upstream.
func (f ForwardAuth) Strip(r *http.Request) {
	for _, h := range []string{f.userH, f.emailH, f.nameH, f.groupsH, f.amrH} {
		if h != "" {
			r.Header.Del(h)
		}
	}
}

// amr parses the configured AMR header into method tokens (space- or
// comma-separated). Returns nil when no header is configured or present, so the
// identity asserts no authentication methods rather than a fabricated one.
func (f ForwardAuth) amr(r *http.Request) []string {
	if f.amrH == "" {
		return nil
	}
	raw := strings.TrimSpace(r.Header.Get(f.amrH))
	if raw == "" {
		return nil
	}
	var out []string
	for _, m := range strings.FieldsFunc(raw, func(c rune) bool { return c == ',' || c == ' ' }) {
		if m = strings.TrimSpace(m); m != "" {
			out = append(out, m)
		}
	}
	return out
}

func headerOr(r *http.Request, header, fallback string) string {
	if header == "" {
		return fallback
	}
	if v := strings.TrimSpace(r.Header.Get(header)); v != "" {
		return v
	}
	return fallback
}

// connIP returns the IP of the immediate connection peer.
func connIP(r *http.Request) net.IP {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	return net.ParseIP(strings.TrimSpace(host))
}
