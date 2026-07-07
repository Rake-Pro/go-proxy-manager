package model

import (
	"fmt"
	"net"
	"regexp"
)

// IP rule actions.
const (
	ActionAllow = "allow"
	ActionDeny  = "deny"
)

// BasicAuthUser is an HTTP basic-auth credential. Only the bcrypt hash is ever
// stored; the plaintext is hashed at the API/CLI boundary before it reaches here.
type BasicAuthUser struct {
	Username     string `json:"username" yaml:"username"`
	PasswordHash string `json:"passwordHash" yaml:"passwordHash"` // bcrypt
}

// IPRule is one ordered allow/deny entry evaluated top-down against the client IP.
type IPRule struct {
	Action string `json:"action" yaml:"action"` // allow | deny
	CIDR   string `json:"cidr" yaml:"cidr"`     // e.g. 10.0.0.0/8 or 1.2.3.4/32
}

// AccessListGeo adds country-based allow/deny rules over the same resolved
// client IP the IP/CIDR Rules use. It requires an operator-supplied GeoIP
// database (GPM_GEOIP_DB); no database ships with gpm (GeoLite2's licence
// forbids redistribution). Evaluation order within a list is: explicit
// IP/CIDR rules, then geo, then DefaultAction - see
// docs/design/http3-geoip-mtls.md.
type AccessListGeo struct {
	// CountryAllow, if non-empty, is a whitelist: only these ISO-3166-1
	// alpha-2 country codes pass. Takes priority over CountryDeny - when set,
	// CountryDeny is ignored.
	CountryAllow []string `json:"countryAllow,omitempty" yaml:"countryAllow,omitempty"`
	// CountryDeny rejects these ISO-3166-1 alpha-2 country codes; every other
	// known country falls through to DefaultAction. Only consulted when
	// CountryAllow is empty.
	CountryDeny []string `json:"countryDeny,omitempty" yaml:"countryDeny,omitempty"`
	// OnUnknown governs an IP with no country in the database - private,
	// loopback, link-local, or simply absent: allow | deny. When unset the
	// default depends on mode: whitelist (CountryAllow) fails closed (deny) so
	// an unresolvable IP cannot slip past a "these countries only" gate;
	// deny-list (CountryDeny) defaults to allow, since it only ever narrows.
	OnUnknown string `json:"onUnknown,omitempty" yaml:"onUnknown,omitempty"`
}

// HasRules reports whether g configures any country-based rule, i.e. whether
// it requires a loaded GeoIP database to be meaningful. A nil g (no geo block
// configured at all) has no rules.
func (g *AccessListGeo) HasRules() bool {
	return g != nil && (len(g.CountryAllow) > 0 || len(g.CountryDeny) > 0)
}

var countryCodeRe = regexp.MustCompile(`^[A-Z]{2}$`)

func (g *AccessListGeo) validate(listName string) error {
	if g == nil {
		return nil
	}
	for _, cc := range g.CountryAllow {
		if !countryCodeRe.MatchString(cc) {
			return fmt.Errorf("access list %q: invalid geo countryAllow code %q, want ISO-3166-1 alpha-2 (e.g. US)", listName, cc)
		}
	}
	for _, cc := range g.CountryDeny {
		if !countryCodeRe.MatchString(cc) {
			return fmt.Errorf("access list %q: invalid geo countryDeny code %q, want ISO-3166-1 alpha-2 (e.g. US)", listName, cc)
		}
	}
	if ou := g.OnUnknown; ou != "" && ou != ActionAllow && ou != ActionDeny {
		return fmt.Errorf("access list %q: geo onUnknown must be allow|deny, got %q", listName, ou)
	}
	return nil
}

// AccessList combines HTTP basic auth, ordered IP allow/deny rules, and
// optional GeoIP country rules. It can be attached to a host or an individual
// location. SatisfyAny selects OR- vs AND-evaluation across the list's checks:
// when true, passing EITHER auth or IP/geo is enough; when false both must pass.
type AccessList struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	SatisfyAny bool            `json:"satisfyAny,omitempty" yaml:"satisfyAny,omitempty"`
	BasicAuth  []BasicAuthUser `json:"basicAuth,omitempty" yaml:"basicAuth,omitempty"`
	Rules      []IPRule        `json:"rules,omitempty" yaml:"rules,omitempty"`
	// DefaultAction applies when no IP rule matches: deny (default) or allow.
	DefaultAction string `json:"defaultAction,omitempty" yaml:"defaultAction,omitempty"`
	// Geo adds country-based allow/deny rules; nil means no geo restriction.
	Geo *AccessListGeo `json:"geo,omitempty" yaml:"geo,omitempty"`
}

func (a AccessList) Kind() string { return "AccessList" }

func (a AccessList) Validate() error {
	if err := ValidateName(a.Name); err != nil {
		return err
	}
	for _, r := range a.Rules {
		if r.Action != ActionAllow && r.Action != ActionDeny {
			return fmt.Errorf("access list %q: rule action must be allow|deny, got %q", a.Name, r.Action)
		}
		if _, _, err := net.ParseCIDR(r.CIDR); err != nil {
			// Permit a bare IP by trying to parse it as a single host.
			if net.ParseIP(r.CIDR) == nil {
				return fmt.Errorf("access list %q: invalid cidr/ip %q", a.Name, r.CIDR)
			}
		}
	}
	if da := a.DefaultAction; da != "" && da != ActionAllow && da != ActionDeny {
		return fmt.Errorf("access list %q: defaultAction must be allow|deny, got %q", a.Name, da)
	}
	if err := a.Geo.validate(a.Name); err != nil {
		return err
	}
	for _, u := range a.BasicAuth {
		if u.Username == "" || u.PasswordHash == "" {
			return fmt.Errorf("access list %q: basic-auth user requires username and passwordHash", a.Name)
		}
	}
	return nil
}
