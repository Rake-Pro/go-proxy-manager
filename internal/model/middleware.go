package model

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// validCIDROrIP reports whether s parses as a CIDR or a bare IP.
func validCIDROrIP(s string) bool {
	if _, _, err := net.ParseCIDR(s); err == nil {
		return true
	}
	return net.ParseIP(s) != nil
}

// Middleware types. The compiled data plane applies a host/location's referenced
// middlewares as an ordered chain of http.Handler wrappers - so auth, headers,
// (and later rate-limit, WAF hooks) are first-class ordered steps with no
// textual-collision class of bug. New behaviours add a Type, never a rewrite.
const (
	MWTypeAuth      = "auth"
	MWTypeHeaders   = "headers"
	MWTypeGuard     = "guard"
	MWTypeRateLimit = "rate-limit" // per-host, per-client-IP token bucket
	MWTypeRewrite   = "rewrite"    // upstream-facing request-path replacement (exact, prefix or regex)
	MWTypeBouncer   = "bouncer"    // deny hook: ask an external bouncer/WAF about the client IP
)

// Bouncer providers. crowdsec speaks the CrowdSec LAPI bouncer protocol; http is
// a generic deny hook any custom bouncer can implement.
const (
	BouncerProviderCrowdSec = "crowdsec"
	BouncerProviderHTTP     = "http"
)

// Bouncer behaviour when the bouncer cannot be reached (or answers
// unintelligibly). fail-open is the default: an unreachable bouncer must not
// take the site down, which is the operationally safer posture for a
// reputation/threat feed (unlike auth, whose failure mode must deny).
const (
	BouncerOnErrorFailOpen   = "fail-open"
	BouncerOnErrorFailClosed = "fail-closed"
)

// How a bouncer denial is rendered. error-page routes the denial through the
// custom error-page renderer; plain writes a bare status body.
const (
	BouncerDenyWithErrorPage = "error-page"
	BouncerDenyWithPlain     = "plain"
)

// Bouncer defaults, applied by the data plane when the field is unset.
const (
	DefaultBouncerTimeout         = 2 * time.Second
	DefaultBouncerCacheTTL        = 60 * time.Second
	DefaultBouncerCacheMaxEntries = 10000
	// MaxBouncerErrorCacheTTL caps how long a verdict derived from an ERROR
	// (rather than a real answer) is cached. A bouncer outage must not pin a
	// whole cacheTTL worth of guessed verdicts.
	MaxBouncerErrorCacheTTL = 5 * time.Second
)

// AuthMode selects how an auth middleware authenticates.
const (
	AuthModeOIDC        = "oidc"         // redirect the browser through the OIDC flow
	AuthModeForwardAuth = "forward-auth" // accept a trusted forward-auth identity
	AuthModeAuthRequest = "auth-request" // delegate to an external auth_request endpoint
	AuthModeClientCert  = "client-cert"  // gate on a verified mTLS client certificate
	AuthModeBasic       = "basic"        // HTTP basic auth against a local username/bcrypt-hash set
)

// authModeList is the human-readable mode list every "unknown mode" error and
// the docs quote, kept next to the constants so the two cannot drift.
const authModeList = "oidc|forward-auth|auth-request|client-cert|basic"

// AuthMiddleware gates requests through a named IdentityProvider and, optionally,
// requires the resolved identity to hold one of RequiredRoles.
type AuthMiddleware struct {
	IdentityProvider string   `json:"identityProvider" yaml:"identityProvider"`
	Mode             string   `json:"mode,omitempty" yaml:"mode,omitempty"` // oidc | forward-auth | auth-request | client-cert | basic (defaults from IdP type)
	RequiredRoles    []string `json:"requiredRoles,omitempty" yaml:"requiredRoles,omitempty"`
	// AllowFrom lists client CIDRs that bypass authentication entirely and are
	// proxied straight through (no auth subrequest, no certificate requirement,
	// no identity headers). An any-of, network-exempt bypass so trusted networks
	// (e.g. the LAN) can skip SSO, mTLS or a password. Applies to auth-request,
	// client-cert and basic modes; it is refused in oidc and forward-auth mode, where the
	// gate has no bypass to honour and a silently ignored exemption would read
	// like a security control that is not there. With Mode unset the effective
	// mode comes from the referenced provider's type, so that case is settled by
	// checkAuthAllowFromMode in Config.Validate, which has the provider set.
	//
	// The exemption is matched against the client IP the HOST resolves, which
	// honours X-Forwarded-For only from that host's own trusted proxies - and a
	// client-cert middleware contributes none, because it has no identity
	// provider. See the allowFrom notes in docs/reference/config/middleware.md before putting
	// a proxy address inside one of these networks.
	AllowFrom []string `json:"allowFrom,omitempty" yaml:"allowFrom,omitempty"`
	// ClientCertRoles maps a verified client certificate to a role in client-cert
	// mode: the key is the certificate subject (RFC 2253 form, e.g.
	// "CN=ops,O=Corp") or its bare common name, the value is the role name
	// RequiredRoles is checked against. Empty means cert presence alone passes.
	ClientCertRoles map[string]string `json:"clientCertRoles,omitempty" yaml:"clientCertRoles,omitempty"`
	// Basic is the credential set for `mode: basic` - HTTP basic auth against
	// local username/bcrypt-hash pairs, with no identity provider involved. It
	// is required in that mode and refused in every other one.
	//
	// This is the supported home for username/password gating; the identical
	// users on an AccessList (AccessList.BasicAuth) are deprecated.
	Basic *BasicAuthSpec `json:"basic,omitempty" yaml:"basic,omitempty"`
}

// validate checks one auth spec. owner is the already-quoted owner phrase the
// error is prefixed with ("middleware \"sso\"", "proxy host \"app\"", ...), so a
// `type: auth` middleware and the identical inline block on a host or location
// are held to exactly the same rules and read the same way when they fail.
func (a *AuthMiddleware) validate(owner string) error {
	// client-cert takes its identity from the TLS handshake and basic from a
	// local credential set, so they are the two auth modes with no identity
	// provider to name.
	if a.Mode != AuthModeClientCert && a.Mode != AuthModeBasic && a.IdentityProvider == "" {
		return fmt.Errorf("%s: auth.identityProvider is required", owner)
	}
	switch mode := a.Mode; mode {
	case AuthModeOIDC, AuthModeForwardAuth:
		// The oidc and forward-auth gates have no network bypass, so an
		// allowFrom here would be silently ignored - refuse it rather than
		// let a config claim an exemption that does not exist. Mode "" is
		// left alone: it defaults from the IdP type, which is not resolvable
		// here, and it may resolve to auth-request.
		if len(a.AllowFrom) > 0 {
			return fmt.Errorf("%s: auth.allowFrom is only supported in auth-request, client-cert and basic modes, not %q", owner, mode)
		}
	case "":
	case AuthModeBasic:
		// The credential set IS the identity source, so naming a provider here
		// would claim an SSO relationship the gate never uses.
		if a.IdentityProvider != "" {
			return fmt.Errorf("%s: auth.identityProvider is not used in basic mode (auth.basic.users is the credential source)", owner)
		}
		// Basic auth resolves a username, never a role: there is no group claim
		// and no roleMapping to derive one from, so requiredRoles could only ever
		// deny every request.
		if len(a.RequiredRoles) > 0 {
			return fmt.Errorf("%s: auth.requiredRoles is not supported in basic mode (a username/password carries no roles; put the users who may reach this host in auth.basic.users)", owner)
		}
		if a.Basic == nil {
			return fmt.Errorf("%s: auth.basic is required in basic mode", owner)
		}
		if err := a.Basic.validate(owner); err != nil {
			return err
		}
	case AuthModeClientCert:
		if a.IdentityProvider != "" {
			return fmt.Errorf("%s: auth.identityProvider is not used in client-cert mode (the TLS handshake is the identity source)", owner)
		}
		// requiredRoles with no mapping could never match, silently denying
		// every request; reject it rather than compile a dead gate.
		if len(a.RequiredRoles) > 0 && len(a.ClientCertRoles) == 0 {
			return fmt.Errorf("%s: auth.requiredRoles in client-cert mode needs auth.clientCertRoles to map a subject to a role", owner)
		}
	case AuthModeAuthRequest:
		if len(a.RequiredRoles) > 0 {
			return fmt.Errorf("%s: auth.requiredRoles is not supported in auth-request mode (the auth server enforces authorization via its application bindings)", owner)
		}
	default:
		return fmt.Errorf("%s: auth.mode must be %s, got %q", owner, authModeList, mode)
	}
	if len(a.ClientCertRoles) > 0 && a.Mode != AuthModeClientCert {
		return fmt.Errorf("%s: auth.clientCertRoles is only used in client-cert mode", owner)
	}
	// A basic block outside basic mode is a credential set nothing reads: the
	// gate would be an SSO/mTLS gate and the users would silently admit nobody.
	if a.Basic != nil && a.Mode != AuthModeBasic {
		return fmt.Errorf("%s: auth.basic is only used in basic mode (set auth.mode: basic)", owner)
	}
	for _, c := range a.AllowFrom {
		if !validCIDROrIP(c) {
			return fmt.Errorf("%s: auth.allowFrom has invalid CIDR/IP %q", owner, c)
		}
	}
	return nil
}

// HeadersMiddleware mutates request/response headers declaratively (security
// headers, Host spoofing, etc.) instead of via raw config text.
type HeadersMiddleware struct {
	SetRequest     map[string]string `json:"setRequest,omitempty" yaml:"setRequest,omitempty"`
	SetResponse    map[string]string `json:"setResponse,omitempty" yaml:"setResponse,omitempty"`
	RemoveRequest  []string          `json:"removeRequest,omitempty" yaml:"removeRequest,omitempty"`
	RemoveResponse []string          `json:"removeResponse,omitempty" yaml:"removeResponse,omitempty"`
}

// GuardMiddleware expresses conditional deny rules (block a path/method unless
// the client is in an allow-list): it denies a matching request unless the
// client is in an allow-list of networks.
// It fires when ANY Trigger matches; a request that matches and whose client IP
// is not in AllowFrom gets DenyStatus. This expresses rules the whole-host/
// location AccessList cannot - e.g. "POST to /login is LAN-only" (break-glass).
type GuardMiddleware struct {
	Triggers   []GuardTrigger `json:"triggers" yaml:"triggers"`
	AllowFrom  []string       `json:"allowFrom,omitempty" yaml:"allowFrom,omitempty"`   // client CIDRs exempt from the deny
	DenyStatus int            `json:"denyStatus,omitempty" yaml:"denyStatus,omitempty"` // default 403
}

// GuardTrigger matches a request. A trigger matches when ALL of its set fields
// match: the path equals one of Paths (if any), the method is one of Methods (if
// any), and every QueryEquals arg equals its value.
type GuardTrigger struct {
	Paths       []string          `json:"paths,omitempty" yaml:"paths,omitempty"`
	Methods     []string          `json:"methods,omitempty" yaml:"methods,omitempty"`
	QueryEquals map[string]string `json:"queryEquals,omitempty" yaml:"queryEquals,omitempty"`
}

// RateLimitMiddleware throttles a host's requests with a per-client-IP token
// bucket: steady-state rate with a Burst allowance (default ceil(rate)).
// Enforced on the data plane (internal/dataplane/ratelimit.go).
//
// The rate can be expressed two ways, and exactly one must be set:
//   - legacy shorthand: RequestsPerSecond (requests per 1s window), or
//   - Requests + Window (a Go duration string, e.g. "10s", "1m", "1h"), for
//     limits like "100 requests per 1m" that don't reduce cleanly to a
//     per-second rate.
type RateLimitMiddleware struct {
	RequestsPerSecond float64 `json:"requestsPerSecond,omitempty" yaml:"requestsPerSecond,omitempty"`
	// Requests and Window together express "N requests per window" (e.g.
	// Requests: 100, Window: "1m"). Mutually exclusive with RequestsPerSecond.
	Requests float64 `json:"requests,omitempty" yaml:"requests,omitempty"`
	Window   string  `json:"window,omitempty" yaml:"window,omitempty"`
	Burst    int     `json:"burst,omitempty" yaml:"burst,omitempty"`
	// AllowFrom lists client CIDRs that bypass rate limiting entirely: a matching
	// request skips the limiter (no token consumed, no 429). An any-of,
	// network-exempt bypass so trusted networks (e.g. LAN) are never throttled.
	AllowFrom []string `json:"allowFrom,omitempty" yaml:"allowFrom,omitempty"`
	// BlockFor, if set, is a Go duration string: once a client exceeds the limit,
	// further requests from it are rejected for this long regardless of token
	// refill. The block is fixed - it does not extend on repeat requests during
	// the block window. Optional; empty means no extra block (today's behavior).
	BlockFor string `json:"blockFor,omitempty" yaml:"blockFor,omitempty"`
}

// usesWindow reports whether the Requests+Window form is set (vs. the legacy
// RequestsPerSecond shorthand).
func (r RateLimitMiddleware) usesWindow() bool {
	return r.Requests > 0 || r.Window != ""
}

// RateAndDefaultBurst returns the steady-state refill rate (tokens/second) and
// the default burst/capacity (ceil of the configured requests, floored at 1)
// for whichever form (legacy or Requests+Window) is set. Callers must validate
// the middleware first; an invalid Window here falls back to 0/1 rather than
// panicking.
func (r RateLimitMiddleware) RateAndDefaultBurst() (rate float64, defaultBurst int) {
	requests := r.RequestsPerSecond
	if r.usesWindow() {
		requests = r.Requests
		d, err := time.ParseDuration(r.Window)
		if err != nil || d <= 0 {
			return 0, 1
		}
		rate = r.Requests / d.Seconds()
	} else {
		rate = r.RequestsPerSecond
	}
	burst := int(math.Ceil(requests))
	if burst < 1 {
		burst = 1
	}
	return rate, burst
}

// validate checks one rate-limit spec. owner is the already-quoted owner phrase
// the error is prefixed with, so a `type: rate-limit` middleware and the
// identical inline block on a host or location share one set of rules.
func (r *RateLimitMiddleware) validate(owner string) error {
	legacySet := r.RequestsPerSecond > 0
	windowSet := r.usesWindow()
	switch {
	case legacySet && windowSet:
		return fmt.Errorf("%s: rateLimit must set either requestsPerSecond or requests+window, not both", owner)
	case !legacySet && !windowSet:
		return fmt.Errorf("%s: rateLimit.requestsPerSecond must be > 0 (or set requests+window)", owner)
	case windowSet:
		if r.Requests <= 0 {
			return fmt.Errorf("%s: rateLimit.requests must be > 0", owner)
		}
		d, err := time.ParseDuration(r.Window)
		if err != nil {
			return fmt.Errorf("%s: rateLimit.window must be a valid duration (e.g. \"10s\", \"1m\", \"1h\"), got %q: %w", owner, r.Window, err)
		}
		if d <= 0 {
			return fmt.Errorf("%s: rateLimit.window must be > 0, got %q", owner, r.Window)
		}
	}
	for _, c := range r.AllowFrom {
		if !validCIDROrIP(c) {
			return fmt.Errorf("%s: rateLimit.allowFrom has invalid CIDR/IP %q", owner, c)
		}
	}
	if r.BlockFor != "" {
		d, err := time.ParseDuration(r.BlockFor)
		if err != nil {
			return fmt.Errorf("%s: rateLimit.blockFor must be a valid duration (e.g. \"10s\", \"1m\", \"1h\"), got %q: %w", owner, r.BlockFor, err)
		}
		if d <= 0 {
			return fmt.Errorf("%s: rateLimit.blockFor must be > 0, got %q", owner, r.BlockFor)
		}
	}
	return nil
}

// validateInline validates an optional inline auth block on a host or location.
// A nil block is simply absent, which is why it is separate from validate: a
// `type: auth` middleware with no spec is an error, an unset host field is not.
func (a *AuthMiddleware) validateInline(owner string) error {
	if a == nil {
		return nil
	}
	return a.validate(owner)
}

// validateInline validates an optional inline rateLimit block on a host or
// location; nil means the block is absent.
func (r *RateLimitMiddleware) validateInline(owner string) error {
	if r == nil {
		return nil
	}
	return r.validate(owner)
}

// RewriteMiddleware rewrites the request path before proxying to the upstream.
// Three rule kinds are evaluated in a fixed order - exact (ReplacePath), then
// prefix (PrefixRules, longest first), then regex (RegexRules, in order) - and
// the FIRST match wins; no rule ever sees another rule's output, so rules cannot
// chain into a path the operator did not write.
//
// The rewrite is internal - it mutates the proxied request path in place,
// preserving the method and body; it is never an HTTP redirect, so the client
// sees no 3xx and non-idempotent POSTs are forwarded unchanged. It is purely
// upstream-facing and runs innermost (closest to the backend), so auth, guards
// and access lists all evaluate the ORIGINAL client path - a rewrite can never
// move a request past a path-scoped security control.
type RewriteMiddleware struct {
	// ReplacePath maps a full request path to its replacement. The incoming path
	// must equal the key exactly; no prefix or pattern matching.
	ReplacePath map[string]string `json:"replacePath,omitempty" yaml:"replacePath,omitempty"`

	// PrefixRules replace a matched path PREFIX. Matching is boundary-aware, the
	// same way a location matches (the path equals From, or begins with From plus
	// "/"), so "/reports" never captures "/reports-evil". The longest matching
	// From wins.
	PrefixRules []RewriteRule `json:"prefixRules,omitempty" yaml:"prefixRules,omitempty"`

	// RegexRules replace the span a Go regexp matches at the START of the path
	// (the pattern is implicitly anchored with "^"); anything after the match is
	// appended unchanged. To may reference capture groups as $1, ${name}.
	// Patterns are compiled once at config load, so a bad pattern is a validation
	// error rather than a request-time failure.
	RegexRules []RewriteRule `json:"regexRules,omitempty" yaml:"regexRules,omitempty"`
}

// MaxRewriteRules caps how many prefix or regex rules one rewrite middleware may
// carry. Rules are evaluated linearly per request, and an unbounded list is a
// per-request cost an operator cannot see; 32 is far above any real config.
const MaxRewriteRules = 32

// MaxRewritePatternLen caps a regex rule's pattern length. Combined with the
// rule cap it bounds the compiled program size, which is the practical lever on
// pathological backtracking cost (Go's RE2 engine is already linear-time, so
// this is a resource bound, not a ReDoS fix).
const MaxRewritePatternLen = 256

// RewriteRule is one ordered prefix or regex rewrite: From is the path prefix or
// the regular expression, To the replacement path.
type RewriteRule struct {
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
}

// validateRewriteTarget checks one rewrite target path. A rewrite runs INSIDE
// the security tiers, closest to the upstream, and its output is what gpm sends
// on - so a "to" carrying a dot segment or a backslash climbs straight out of
// the base path an operator pinned the backend into (Upstream.Path is validated
// against exactly this, see validateUpstreamPath) and lands on whatever the
// backend re-collapses it to. The rewrite scope is narrower than the proxy-host
// scope, so this is also a privilege boundary, not only a hygiene rule.
//
// A regex replacement template is held to the same rules: "$1" may expand to
// anything at request time, so the template is checked statically here AND the
// composed path is re-cleaned at request time (see dataplane's rewritePath).
func validateRewriteTarget(owner, field, to string) error {
	if to == "" || !strings.HasPrefix(to, "/") {
		return fmt.Errorf("middleware %q: %s %q must be an absolute path (start with %q)", owner, field, to, "/")
	}
	if strings.ContainsAny(to, "?#") {
		return fmt.Errorf("middleware %q: %s %q must not contain a query string or fragment", owner, field, to)
	}
	if strings.ContainsAny(to, `\;`) {
		return fmt.Errorf("middleware %q: %s %q must not contain %q or %q", owner, field, to, `\`, ";")
	}
	for _, seg := range strings.Split(to, "/") {
		if seg == "." || seg == ".." {
			return fmt.Errorf("middleware %q: %s %q must not contain %q or %q segments", owner, field, to, ".", "..")
		}
	}
	return nil
}

func (rw RewriteMiddleware) validate(owner string) error {
	if len(rw.ReplacePath) == 0 && len(rw.PrefixRules) == 0 && len(rw.RegexRules) == 0 {
		return fmt.Errorf("middleware %q: rewrite requires at least one replacePath, prefixRules or regexRules entry", owner)
	}
	for k, v := range rw.ReplacePath {
		if k == "" || !strings.HasPrefix(k, "/") {
			return fmt.Errorf("middleware %q: rewrite.replacePath key %q must be an absolute path (start with %q)", owner, k, "/")
		}
		if err := validateRewriteTarget(owner, fmt.Sprintf("rewrite.replacePath[%q] target", k), v); err != nil {
			return err
		}
		if k == v {
			return fmt.Errorf("middleware %q: rewrite.replacePath[%q] rewrites a path to itself (no-op)", owner, k)
		}
	}
	if len(rw.PrefixRules) > MaxRewriteRules {
		return fmt.Errorf("middleware %q: rewrite.prefixRules has %d rules, at most %d are allowed", owner, len(rw.PrefixRules), MaxRewriteRules)
	}
	for i, r := range rw.PrefixRules {
		if r.From == "" || !strings.HasPrefix(r.From, "/") {
			return fmt.Errorf("middleware %q: rewrite.prefixRules[%d].from %q must be an absolute path (start with %q)", owner, i, r.From, "/")
		}
		if err := validateRewriteTarget(owner, fmt.Sprintf("rewrite.prefixRules[%d].to", i), r.To); err != nil {
			return err
		}
		if r.From == r.To {
			return fmt.Errorf("middleware %q: rewrite.prefixRules[%d] rewrites %q to itself (no-op)", owner, i, r.From)
		}
	}
	if len(rw.RegexRules) > MaxRewriteRules {
		return fmt.Errorf("middleware %q: rewrite.regexRules has %d rules, at most %d are allowed", owner, len(rw.RegexRules), MaxRewriteRules)
	}
	for i, r := range rw.RegexRules {
		if r.From == "" {
			return fmt.Errorf("middleware %q: rewrite.regexRules[%d].from must not be empty", owner, i)
		}
		if len(r.From) > MaxRewritePatternLen {
			return fmt.Errorf("middleware %q: rewrite.regexRules[%d].from is %d characters, at most %d are allowed", owner, i, len(r.From), MaxRewritePatternLen)
		}
		if _, err := regexp.Compile(AnchorRewritePattern(r.From)); err != nil {
			return fmt.Errorf("middleware %q: rewrite.regexRules[%d].from %q is not a valid regular expression: %w", owner, i, r.From, err)
		}
		if err := validateRewriteTarget(owner, fmt.Sprintf("rewrite.regexRules[%d].to", i), r.To); err != nil {
			return err
		}
	}
	return nil
}

// AnchorRewritePattern returns the pattern a regex rewrite rule is actually
// compiled from: implicitly anchored at the start of the path, so a rule can
// only ever replace a leading span and never float into the middle of a path.
// An operator-written leading "^" is not doubled.
func AnchorRewritePattern(pattern string) string {
	if strings.HasPrefix(pattern, "^") {
		return pattern
	}
	return "^" + pattern
}

// BouncerMiddleware is a deny hook: before a request reaches authentication, gpm
// asks an operator-run bouncer whether the client IP is currently banned. It is
// a HOOK, not a bundled WAF - gpm ships no rules, no engine and no signature
// feed; the verdict is entirely the external service's.
//
// It sits after the access list and before auth, so an operator allow-list still
// wins outright and a banned IP never reaches the IdP (no forward-auth
// subrequest, no OIDC redirect).
type BouncerMiddleware struct {
	// Provider selects the wire protocol: "crowdsec" (CrowdSec LAPI bouncer) or
	// "http" (generic deny hook). Empty defaults to crowdsec.
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	// URL is the bouncer base URL: the LAPI root (e.g. http://crowdsec:8080) for
	// crowdsec, or the full endpoint to GET for http.
	URL string `json:"url" yaml:"url"`
	// APIKey is sent as X-Api-Key. Required for crowdsec (register it with
	// "cscli bouncers add gpm"); optional for http.
	APIKey Secret `json:"apiKey,omitempty" yaml:"apiKey,omitempty"`
	// Timeout is a Go duration string bounding one bouncer call (default "2s").
	// A bouncer is on the request hot path; it never gets to hang a request.
	Timeout string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	// CacheTTL is a Go duration string: how long a verdict is reused for the same
	// client IP (default "60s"). In stream mode it is also the delta poll
	// interval. A verdict derived from an error is capped at 5s regardless.
	CacheTTL string `json:"cacheTTL,omitempty" yaml:"cacheTTL,omitempty"`
	// CacheMaxEntries bounds the per-middleware verdict cache (default 10000),
	// so a rotating-source-IP flood cannot grow it without bound.
	CacheMaxEntries int `json:"cacheMaxEntries,omitempty" yaml:"cacheMaxEntries,omitempty"`
	// OnError is the verdict when the bouncer errors, times out or answers
	// unintelligibly: "fail-open" (default, allow) or "fail-closed" (deny).
	OnError string `json:"onError,omitempty" yaml:"onError,omitempty"`
	// DenyStatus is the status a denied request gets (default 403).
	DenyStatus int `json:"denyStatus,omitempty" yaml:"denyStatus,omitempty"`
	// DenyWith selects the denial body: "error-page" (default) renders through
	// the custom error-page path, "plain" writes a bare status body.
	DenyWith string `json:"denyWith,omitempty" yaml:"denyWith,omitempty"`
	// AllowFrom lists client CIDRs that bypass the bouncer entirely: a matching
	// request is never looked up and never denied by it. An operator allow-list
	// must be able to win outright over an external feed's verdict - the same
	// any-of, network-exempt bypass auth/guard/rate-limit already carry.
	AllowFrom []string `json:"allowFrom,omitempty" yaml:"allowFrom,omitempty"`
	// Stream (crowdsec only) pulls the full decision set once
	// (/v1/decisions/stream?startup=true) and then deltas every CacheTTL,
	// keeping an in-memory IP/range set so the hot path is a local lookup with
	// no per-request call to the LAPI. Off means a live lookup per uncached IP.
	Stream bool `json:"stream,omitempty" yaml:"stream,omitempty"`
}

// ProviderOrDefault returns the configured provider, defaulting to crowdsec.
func (b BouncerMiddleware) ProviderOrDefault() string {
	if b.Provider == "" {
		return BouncerProviderCrowdSec
	}
	return b.Provider
}

// TimeoutOrDefault returns the per-call timeout. An unset or unparseable value
// (validation rejects the latter) yields DefaultBouncerTimeout.
func (b BouncerMiddleware) TimeoutOrDefault() time.Duration {
	return durationOr(b.Timeout, DefaultBouncerTimeout)
}

// CacheTTLOrDefault returns the verdict cache TTL / stream poll interval.
func (b BouncerMiddleware) CacheTTLOrDefault() time.Duration {
	return durationOr(b.CacheTTL, DefaultBouncerCacheTTL)
}

// CacheMaxEntriesOrDefault returns the verdict-cache bound.
func (b BouncerMiddleware) CacheMaxEntriesOrDefault() int {
	if b.CacheMaxEntries <= 0 {
		return DefaultBouncerCacheMaxEntries
	}
	return b.CacheMaxEntries
}

// FailOpen reports whether an errored bouncer call allows the request.
func (b BouncerMiddleware) FailOpen() bool { return b.OnError != BouncerOnErrorFailClosed }

// DenyStatusOrDefault returns the status a denied request gets.
func (b BouncerMiddleware) DenyStatusOrDefault() int {
	if b.DenyStatus == 0 {
		return 403
	}
	return b.DenyStatus
}

func durationOr(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

// Middleware is a reusable, named processing step referenced by hosts/locations.
type Middleware struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	Type      string               `json:"type" yaml:"type"`
	Auth      *AuthMiddleware      `json:"auth,omitempty" yaml:"auth,omitempty"`
	Headers   *HeadersMiddleware   `json:"headers,omitempty" yaml:"headers,omitempty"`
	Guard     *GuardMiddleware     `json:"guard,omitempty" yaml:"guard,omitempty"`
	RateLimit *RateLimitMiddleware `json:"rateLimit,omitempty" yaml:"rateLimit,omitempty"`
	Rewrite   *RewriteMiddleware   `json:"rewrite,omitempty" yaml:"rewrite,omitempty"`
	Bouncer   *BouncerMiddleware   `json:"bouncer,omitempty" yaml:"bouncer,omitempty"`
}

func (m Middleware) Kind() string { return "Middleware" }

func (m Middleware) Validate() error {
	if err := ValidateName(m.Name); err != nil {
		return err
	}
	switch m.Type {
	case MWTypeAuth:
		if m.Auth == nil {
			return fmt.Errorf("middleware %q: auth spec required", m.Name)
		}
		if err := m.Auth.validate(fmt.Sprintf("middleware %q", m.Name)); err != nil {
			return err
		}
	case MWTypeHeaders:
		if m.Headers == nil {
			return fmt.Errorf("middleware %q: headers spec required", m.Name)
		}
	case MWTypeGuard:
		if m.Guard == nil || len(m.Guard.Triggers) == 0 {
			return fmt.Errorf("middleware %q: guard requires at least one trigger", m.Name)
		}
		for _, c := range m.Guard.AllowFrom {
			if !validCIDROrIP(c) {
				return fmt.Errorf("middleware %q: guard.allowFrom has invalid CIDR/IP %q", m.Name, c)
			}
		}
		if s := m.Guard.DenyStatus; s != 0 && (s < 400 || s > 599) {
			return fmt.Errorf("middleware %q: guard.denyStatus must be a 4xx/5xx code, got %d", m.Name, s)
		}
	case MWTypeRateLimit:
		if m.RateLimit == nil {
			return fmt.Errorf("middleware %q: rateLimit.requestsPerSecond must be > 0", m.Name)
		}
		if err := m.RateLimit.validate(fmt.Sprintf("middleware %q", m.Name)); err != nil {
			return err
		}
	case MWTypeRewrite:
		// A nil spec is the same defect as an empty one - no rules - so both take
		// the "needs at least one rule" path rather than two different errors.
		var rw RewriteMiddleware
		if m.Rewrite != nil {
			rw = *m.Rewrite
		}
		if err := rw.validate(m.Name); err != nil {
			return err
		}
	case MWTypeBouncer:
		if m.Bouncer == nil {
			return fmt.Errorf("middleware %q: bouncer spec required", m.Name)
		}
		if err := m.Bouncer.validate(m.Name); err != nil {
			return err
		}
	default:
		return fmt.Errorf("middleware %q: unknown type %q", m.Name, m.Type)
	}
	return nil
}

// validate checks a BouncerMiddleware; name is the owning middleware's name, so
// the error reads the same way every other middleware error does.
func (b *BouncerMiddleware) validate(name string) error {
	provider := b.ProviderOrDefault()
	switch provider {
	case BouncerProviderCrowdSec, BouncerProviderHTTP:
	default:
		return fmt.Errorf("middleware %q: bouncer.provider must be crowdsec|http, got %q", name, b.Provider)
	}
	if strings.TrimSpace(b.URL) == "" {
		return fmt.Errorf("middleware %q: bouncer.url is required", name)
	}
	u, err := url.Parse(b.URL)
	if err != nil {
		return fmt.Errorf("middleware %q: bouncer.url is not a valid URL: %w", name, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("middleware %q: bouncer.url must be an absolute http(s) URL, got %q", name, b.URL)
	}
	// The CrowdSec LAPI authenticates every bouncer call by API key; without one
	// every request would 403 and the middleware would silently run on its
	// onError policy forever.
	if provider == BouncerProviderCrowdSec && b.APIKey.IsEmpty() {
		return fmt.Errorf("middleware %q: bouncer.apiKey is required for the crowdsec provider (register one with \"cscli bouncers add gpm\")", name)
	}
	if b.Stream && provider != BouncerProviderCrowdSec {
		return fmt.Errorf("middleware %q: bouncer.stream is only supported by the crowdsec provider", name)
	}
	for _, f := range []struct{ field, value string }{{"timeout", b.Timeout}, {"cacheTTL", b.CacheTTL}} {
		if f.value == "" {
			continue
		}
		d, err := time.ParseDuration(f.value)
		if err != nil {
			return fmt.Errorf("middleware %q: bouncer.%s must be a valid duration (e.g. \"2s\", \"60s\"), got %q: %w", name, f.field, f.value, err)
		}
		if d <= 0 {
			return fmt.Errorf("middleware %q: bouncer.%s must be > 0, got %q", name, f.field, f.value)
		}
	}
	if b.CacheMaxEntries < 0 {
		return fmt.Errorf("middleware %q: bouncer.cacheMaxEntries must be >= 0, got %d", name, b.CacheMaxEntries)
	}
	switch b.OnError {
	case "", BouncerOnErrorFailOpen, BouncerOnErrorFailClosed:
	default:
		return fmt.Errorf("middleware %q: bouncer.onError must be fail-open|fail-closed, got %q", name, b.OnError)
	}
	switch b.DenyWith {
	case "", BouncerDenyWithErrorPage, BouncerDenyWithPlain:
	default:
		return fmt.Errorf("middleware %q: bouncer.denyWith must be error-page|plain, got %q", name, b.DenyWith)
	}
	if s := b.DenyStatus; s != 0 && (s < 400 || s > 599) {
		return fmt.Errorf("middleware %q: bouncer.denyStatus must be a 4xx/5xx code, got %d", name, s)
	}
	for _, c := range b.AllowFrom {
		if !validCIDROrIP(c) {
			return fmt.Errorf("middleware %q: bouncer.allowFrom has invalid CIDR/IP %q", name, c)
		}
	}
	return nil
}
