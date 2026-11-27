package model

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

// IP rule actions.
const (
	ActionAllow = "allow"
	ActionDeny  = "deny"
)

// Access-list composition modes for ProxyHost.AccessListsMode and
// Location.AccessListsMode. Empty means AccessListsModeAll.
const (
	AccessListsModeAll = "all"
	AccessListsModeAny = "any"
)

// validateAccessListsMode checks one accessListsMode value against the list it
// composes. owner prefixes the error, e.g. `proxy host "x"`.
func validateAccessListsMode(owner, mode string, lists []string) error {
	switch mode {
	case "", AccessListsModeAll:
		return nil
	case AccessListsModeAny:
		if len(lists) == 0 {
			return fmt.Errorf("%s: accessListsMode: any needs at least one entry in accessLists", owner)
		}
		return nil
	}
	return fmt.Errorf("%s: accessListsMode: must be all|any, got %q", owner, mode)
}

// BasicAuthUser now lives in basicauth.go: it is shared with the auth
// middleware's `mode: basic` block, which is where username/password gating
// belongs (AccessList.BasicAuth below is deprecated).

// IPRule is one ordered allow/deny entry evaluated top-down against the client
// IP and, when Paths is set, the request path and method.
type IPRule struct {
	Action string `json:"action" yaml:"action"` // allow | deny
	// CIDR is a literal network or bare IP, e.g. 10.0.0.0/8 or 1.2.3.4/32.
	// Exactly one of CIDR or Source must be set.
	CIDR string `json:"cidr,omitempty" yaml:"cidr,omitempty"`
	// Source names an entry in the list's Sources: the rule then matches every
	// network in that source's most recently fetched set (see
	// AccessListSourceLedger). A source with no ledger entry yet (never fetched
	// successfully, or the ledger could not be loaded), and one whose set
	// expired under maxAge, resolves to the EMPTY set: the rule matches nothing
	// and evaluation moves on to the next rule and then the defaultAction. For
	// an allow rule that is the safe direction. For a deny rule it is not: the
	// rule blocks nobody, and an address it was meant to deny is decided by the
	// rules after it, which may allow it. That is kept on purpose (an outage of
	// a deny feed must not lock everyone out) and surfaced instead: such a deny
	// rule is listed in the configWarnings of GET /health (see
	// dataplane.UnresolvedDenySourceWarnings), and the data plane logs every
	// source rule with an empty set once per reload. A set over its maxCoverage
	// budget, or one whose every entry the entry guard refuses, resolves to
	// the empty set in the same way.
	Source string `json:"source,omitempty" yaml:"source,omitempty"`
	// Paths scopes the rule to a set of exact request paths. When empty the rule
	// applies to every request (the historical behaviour). When set, the rule
	// matches only a request whose cleaned path equals one of these AND whose
	// method is in Methods - so a monitoring feed can be granted the health
	// endpoints of an otherwise LAN-only host and nothing else.
	//
	// Paths are ALLOW-ONLY (validation refuses action: deny alongside them), and
	// the match is exact, case-sensitive, and does no trailing-slash folding: it
	// compares against the router's already-cleaned r.URL.Path. That is safe for
	// an allow rule - a spelling it does not cover simply falls through to
	// defaultAction - and unsafe for a deny, which is why deny-by-path is the
	// guard middleware's job.
	//
	// There is no request path at L4, so a list carrying a path-scoped rule is
	// refused for a StreamHost at validation (see Config.Validate) rather than
	// silently evaluated as if the paths were not there.
	Paths []string `json:"paths,omitempty" yaml:"paths,omitempty"`
	// Methods are the upper-case HTTP methods the path-scoped rule covers. Only
	// valid together with Paths; empty means GET and HEAD, the read-only pair a
	// health probe needs.
	Methods []string `json:"methods,omitempty" yaml:"methods,omitempty"`
}

// AccessListSourceRef is one deny rule's source reference: the list and the
// source name.
type AccessListSourceRef struct {
	List   AccessList
	Source string
}

// DenySourceRefs returns every source a deny rule names in an enabled list,
// once per list and source. Such a rule denies no one while its source
// resolves to the empty set (see IPRule.Source).
func DenySourceRefs(lists []AccessList) []AccessListSourceRef {
	var out []AccessListSourceRef
	for _, al := range lists {
		if al.Disabled {
			continue
		}
		seen := map[string]bool{}
		for _, r := range al.Rules {
			if r.Action != ActionDeny || r.Source == "" || seen[r.Source] {
				continue
			}
			seen[r.Source] = true
			out = append(out, AccessListSourceRef{List: al, Source: r.Source})
		}
	}
	return out
}

// httpMethods is the standard method set an IPRule may name.
var httpMethods = map[string]bool{
	"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "CONNECT": true, "OPTIONS": true, "TRACE": true,
}

// PathScoped reports whether the rule only applies to specific request paths.
func (r IPRule) PathScoped() bool { return len(r.Paths) > 0 }

// EffectiveMethods returns the methods a path-scoped rule covers: the configured
// set, or GET and HEAD when none was given. It is meaningless (and returns nil)
// for a rule with no Paths, which applies to every method already.
func (r IPRule) EffectiveMethods() []string {
	if !r.PathScoped() {
		return nil
	}
	if len(r.Methods) == 0 {
		return []string{"GET", "HEAD"}
	}
	return r.Methods
}

// Access-list source defaults. Interval is how often a source is re-fetched;
// nothing below MinAccessListSourceInterval is accepted, so a config change
// cannot turn a published IP list into a request flood.
const (
	DefaultAccessListSourceInterval   = 24 * time.Hour
	MinAccessListSourceInterval       = time.Hour
	DefaultAccessListSourceMaxEntries = 10000
	// Broad-feed thresholds, as prefix-length equivalents: one /8 of IPv4
	// (16,777,216 addresses) and one /24 of IPv6 (256 /32s). A source without
	// maxCoverage has no coverage budget; one covering more than this is
	// logged as broad. With maxCoverage set, a family left at 0 is bounded by
	// this value. Monitoring feeds are a few hundred single addresses, and the
	// largest published CDN edge lists cover a few million IPv4 addresses and
	// a few dozen IPv6 /32s, so both stay well under.
	BroadAccessListSourceCoverageV4 = 8
	BroadAccessListSourceCoverageV6 = 24
)

// AccessListSource is a remote allow-list feed: a published, plain-text list of
// monitoring or CDN addresses that an IPRule references by name instead of the
// operator pasting (and re-pasting) hundreds of CIDRs into the config.
//
// The format is fixed and deliberately minimal: one IP or CIDR per line, "#"
// comments and blank lines ignored, a bare IP read as a /32 or /128. Anything
// else in the body is a REFUSAL of the whole fetch, not a skipped line - a feed
// that changed shape is a feed gpm no longer understands, and the previously
// fetched set is kept instead.
type AccessListSource struct {
	Name string `json:"name" yaml:"name"`
	// URL must be https. There is no http option: the fetched body decides who
	// reaches a host, so it may not be modifiable in transit.
	URL string `json:"url" yaml:"url"`
	// Interval is a Go duration string; empty means 24h, and anything below 1h
	// is refused.
	Interval string `json:"interval,omitempty" yaml:"interval,omitempty"`
	// MaxEntries caps how many networks the feed may carry; 0 means 10000. A
	// body with more is refused whole, so a compromised or replaced feed cannot
	// quietly become an allow-the-internet rule.
	MaxEntries int `json:"maxEntries,omitempty" yaml:"maxEntries,omitempty"`
	// MaxCoverage, when set, bounds the total address space the feed's entries
	// may cover per family. A body over either budget is refused whole, so a
	// feed made of many individually acceptable prefixes cannot allow most of
	// the internet. Absent means no budget (the per-entry rules still apply).
	MaxCoverage *AccessListSourceCoverage `json:"maxCoverage,omitempty" yaml:"maxCoverage,omitempty"`
	// MaxAge, when set, is a Go duration after which the stored set stops
	// being served: once the last successful fetch recorded in the ledger, or
	// the moment maxAge was first applied to the source if that is later, is
	// older than MaxAge and a fetch is refused, the fetcher empties the set and
	// the source matches nothing until a fetch succeeds again. It may not be
	// shorter than the fetch interval. Empty means a refused fetch keeps the
	// previous set however old it is.
	MaxAge string `json:"maxAge,omitempty" yaml:"maxAge,omitempty"`
}

// AccessListSourceCoverage is a per-family coverage budget, each written as the
// prefix length whose size the summed entries may not exceed: ipv4: 6 allows up
// to four /8s worth of IPv4 addresses. 0 bounds that family at one /8 of IPv4
// or one /24 of IPv6 (the broad-feed thresholds).
type AccessListSourceCoverage struct {
	IPv4 int `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6 int `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

// FetchInterval is the configured re-fetch interval, or the 24h default.
func (s AccessListSource) FetchInterval() time.Duration {
	if s.Interval == "" {
		return DefaultAccessListSourceInterval
	}
	d, err := time.ParseDuration(s.Interval)
	if err != nil || d < MinAccessListSourceInterval {
		return DefaultAccessListSourceInterval
	}
	return d
}

// MaxAgeLimit is the configured maxAge; ok is false when none is set (or it
// does not parse, which validation refuses), meaning a set never expires.
func (s AccessListSource) MaxAgeLimit() (d time.Duration, ok bool) {
	if s.MaxAge == "" {
		return 0, false
	}
	d, err := time.ParseDuration(s.MaxAge)
	if err != nil {
		return 0, false
	}
	return d, true
}

// EntryLimit is the configured cap, or the 10000 default.
func (s AccessListSource) EntryLimit() int {
	if s.MaxEntries <= 0 {
		return DefaultAccessListSourceMaxEntries
	}
	return s.MaxEntries
}

// CoverageLimits returns the IPv4 and IPv6 coverage budgets as prefix-length
// equivalents, with a family left at 0 filled in. ok is false when the source
// sets no maxCoverage and so has no budget.
func (s AccessListSource) CoverageLimits() (v4, v6 int, ok bool) {
	c := s.MaxCoverage
	if c == nil {
		return 0, 0, false
	}
	v4, v6 = BroadAccessListSourceCoverageV4, BroadAccessListSourceCoverageV6
	if c.IPv4 > 0 {
		v4 = c.IPv4
	}
	if c.IPv6 > 0 {
		v6 = c.IPv6
	}
	return v4, v6, true
}

// AccessListGeo adds country-based allow/deny rules over the same resolved
// client IP the IP/CIDR Rules use. It requires an operator-supplied GeoIP
// database (GPM_GEOIP_DB); no database ships with gpm (GeoLite2's licence
// forbids redistribution). Evaluation order within a list is: explicit
// IP/CIDR rules, then geo, then DefaultAction - see
// design/http3-geoip-mtls.md.
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
			return fmt.Errorf("access list %q: geo.countryAllow: code %q is invalid, want ISO-3166-1 alpha-2 (e.g. US)", listName, cc)
		}
	}
	for _, cc := range g.CountryDeny {
		if !countryCodeRe.MatchString(cc) {
			return fmt.Errorf("access list %q: geo.countryDeny: code %q is invalid, want ISO-3166-1 alpha-2 (e.g. US)", listName, cc)
		}
	}
	if ou := g.OnUnknown; ou != "" && ou != ActionAllow && ou != ActionDeny {
		return fmt.Errorf("access list %q: geo.onUnknown: must be allow|deny, got %q", listName, ou)
	}
	return nil
}

// AccessList is a set of ordered IP allow/deny rules and optional GeoIP country
// rules over the resolved client IP. It can be attached to a host or an
// individual location.
//
// It also still carries the deprecated BasicAuth/SatisfyAny pair, which is the
// only login mechanism that ever lived in this tier. Use an auth middleware with
// `mode: basic` instead (see BasicAuthSpec); POST
// /api/access-lists/{name}/migrate-basic-auth converts a list in place.
type AccessList struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	// Deprecated: use an auth middleware with `mode: basic` and put these
	// networks in its allowFrom. SatisfyAny selects OR- vs AND-evaluation across
	// this list's checks: when true, passing EITHER BasicAuth or IP/geo is
	// enough; when false both must pass. It is meaningless without BasicAuth,
	// since IP and geo are already one verdict. Still honoured; removed in v2.
	SatisfyAny bool `json:"satisfyAny,omitempty" yaml:"satisfyAny,omitempty"`
	// Deprecated: use an auth middleware with `mode: basic` (BasicAuthSpec).
	// Basic auth in the access-list tier is what forces the SatisfyAny flag and
	// the "no basicAuth list on a StreamHost" special case in Config.Validate.
	// Still honoured; removed in v2.
	BasicAuth []BasicAuthUser `json:"basicAuth,omitempty" yaml:"basicAuth,omitempty"`
	Rules     []IPRule        `json:"rules,omitempty" yaml:"rules,omitempty"`
	// Sources are remote IP feeds a rule may reference by name (rule.source).
	// The fetched sets live in the committed ledger (config/access-list-sources.yaml),
	// never inline here, so a routine re-fetch never rewrites the operator's list.
	Sources []AccessListSource `json:"sources,omitempty" yaml:"sources,omitempty"`
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
	sources := map[string]bool{}
	for _, src := range a.Sources {
		if err := ValidateName(src.Name); err != nil {
			return fmt.Errorf("access list %q: sources: %w", a.Name, err)
		}
		if sources[src.Name] {
			return fmt.Errorf("access list %q: sources: duplicate source %q", a.Name, src.Name)
		}
		sources[src.Name] = true
		u, err := url.Parse(src.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("access list %q: sources: source %q url must be an absolute https URL, got %q", a.Name, src.Name, src.URL)
		}
		if src.Interval != "" {
			d, err := time.ParseDuration(src.Interval)
			if err != nil {
				return fmt.Errorf("access list %q: sources: source %q interval must be a Go duration (e.g. \"24h\"), got %q: %w", a.Name, src.Name, src.Interval, err)
			}
			if d < MinAccessListSourceInterval {
				return fmt.Errorf("access list %q: sources: source %q interval must be at least %s, got %q", a.Name, src.Name, MinAccessListSourceInterval, src.Interval)
			}
		}
		if src.MaxAge != "" {
			d, err := time.ParseDuration(src.MaxAge)
			if err != nil {
				return fmt.Errorf("access list %q: sources: source %q maxAge must be a Go duration (e.g. \"72h\"), got %q: %w", a.Name, src.Name, src.MaxAge, err)
			}
			// Shorter than the interval would expire every set between two
			// scheduled fetches of a healthy feed.
			if iv := src.FetchInterval(); d < iv {
				return fmt.Errorf("access list %q: sources: source %q maxAge must not be shorter than its fetch interval (%s), got %q", a.Name, src.Name, iv, src.MaxAge)
			}
		}
		if src.MaxEntries < 0 {
			return fmt.Errorf("access list %q: sources: source %q maxEntries must not be negative, got %d", a.Name, src.Name, src.MaxEntries)
		}
		if c := src.MaxCoverage; c != nil {
			if c.IPv4 < 0 || c.IPv4 > 32 {
				return fmt.Errorf("access list %q: sources: source %q maxCoverage.ipv4 must be a prefix length from 1 to 32 (0 = the broad-feed threshold), got %d", a.Name, src.Name, c.IPv4)
			}
			// IPv6 coverage is counted in /64 units, so a budget narrower than
			// one /64 cannot be expressed.
			if c.IPv6 < 0 || c.IPv6 > 64 {
				return fmt.Errorf("access list %q: sources: source %q maxCoverage.ipv6 must be a prefix length from 1 to 64 (0 = the broad-feed threshold), got %d", a.Name, src.Name, c.IPv6)
			}
		}
	}
	for _, r := range a.Rules {
		if r.Action != ActionAllow && r.Action != ActionDeny {
			return fmt.Errorf("access list %q: rules: rule action must be allow|deny, got %q", a.Name, r.Action)
		}
		// A rule draws its networks from exactly one place. Allowing both would
		// leave the precedence between a literal and a fetched set undefined;
		// allowing neither is a rule that matches nothing while reading like one
		// that matches something.
		switch {
		case r.CIDR != "" && r.Source != "":
			return fmt.Errorf("access list %q: rules: rule sets both cidr %q and source %q: use exactly one", a.Name, r.CIDR, r.Source)
		case r.CIDR == "" && r.Source == "":
			return fmt.Errorf("access list %q: rules: rule must set either cidr or source", a.Name)
		case r.Source != "":
			if !sources[r.Source] {
				return fmt.Errorf("access list %q: rules: rule references source %q, which is not declared in this list's sources", a.Name, r.Source)
			}
		default:
			if _, _, err := net.ParseCIDR(r.CIDR); err != nil {
				// Permit a bare IP by trying to parse it as a single host.
				if net.ParseIP(r.CIDR) == nil {
					return fmt.Errorf("access list %q: rules: invalid cidr/ip %q", a.Name, r.CIDR)
				}
			}
		}
		if err := validateRulePaths(a.Name, r); err != nil {
			return err
		}
	}
	if da := a.DefaultAction; da != "" && da != ActionAllow && da != ActionDeny {
		return fmt.Errorf("access list %q: defaultAction: must be allow|deny, got %q", a.Name, da)
	}
	if err := a.Geo.validate(a.Name); err != nil {
		return err
	}
	for _, u := range a.BasicAuth {
		if u.Username == "" || u.PasswordHash == "" {
			return fmt.Errorf("access list %q: basicAuth: user requires username and passwordHash", a.Name)
		}
	}
	return nil
}

// validateRulePaths checks the path/method scoping of one rule. Paths are
// compared for EXACT equality against the router's already-cleaned request
// path, so a value that is not itself clean (a trailing slash, a dot segment, a
// query string) could never match anything and is refused at write time rather
// than becoming a rule that silently never fires.
func validateRulePaths(listName string, r IPRule) error {
	if !r.PathScoped() {
		if len(r.Methods) > 0 {
			return fmt.Errorf("access list %q: rules: rule sets methods %v without paths: methods only scope a path rule", listName, r.Methods)
		}
		return nil
	}
	// Path rules are ALLOW-ONLY. A path-scoped deny would have to be airtight to
	// be worth anything, and exact matching on the cleaned path is not: the
	// router preserves a trailing slash, so "/admin" would not cover "/admin/",
	// and an upstream that treats paths case-insensitively would not be covered
	// by "/ADMIN" either. An allow rule that misses simply falls through to the
	// list's defaultAction (fails closed); a deny rule that misses lets the
	// request past (fails OPEN). Deny-by-path belongs to the guard middleware,
	// which owns that matching problem - see docs/reference/config/middleware.md.
	if r.Action == ActionDeny {
		return fmt.Errorf("access list %q: rules: rule paths are allow-only, got action deny: an exact path match cannot be relied on to DENY (it would miss %q and other equivalent spellings); use a guard middleware for deny-by-path", listName, r.Paths[0]+"/")
	}
	seen := map[string]bool{}
	for _, p := range r.Paths {
		if !strings.HasPrefix(p, "/") {
			return fmt.Errorf("access list %q: rules: rule path %q must start with \"/\"", listName, p)
		}
		if strings.ContainsAny(p, "?#") {
			return fmt.Errorf("access list %q: rules: rule path %q must be a path only (no query string or fragment)", listName, p)
		}
		if path.Clean(p) != p {
			return fmt.Errorf("access list %q: rules: rule path %q is not clean: use %q", listName, p, path.Clean(p))
		}
		if seen[p] {
			return fmt.Errorf("access list %q: rules: duplicate rule path %q", listName, p)
		}
		seen[p] = true
	}
	for _, m := range r.Methods {
		if !httpMethods[m] {
			return fmt.Errorf("access list %q: rules: rule method %q is not an upper-case standard HTTP method", listName, m)
		}
	}
	return nil
}

// HasRequestScopedRules reports whether the list can only be evaluated against
// an HTTP request: a path-scoped rule needs a request path, and a source-backed
// rule needs the fetched ledger the HTTP data plane resolves. Both are refused
// on a StreamHost (see Config.Validate), which has neither.
func (a AccessList) HasRequestScopedRules() bool {
	if len(a.Sources) > 0 {
		return true
	}
	for _, r := range a.Rules {
		if r.PathScoped() || r.Source != "" {
			return true
		}
	}
	return false
}
