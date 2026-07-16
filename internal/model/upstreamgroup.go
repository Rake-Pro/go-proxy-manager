package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Group load-distribution policies. Unhealthy upstreams are always demoted to
// the end of the try-order regardless of policy (fail-open), and failover
// retries stay connect-error-only.
const (
	// PolicyFailover (the default) prefers upstreams strictly in list order:
	// first healthy entry wins, the rest are backups. Weights are ignored.
	PolicyFailover = "failover"
	// PolicyRoundRobin distributes requests across healthy upstreams with
	// smooth weighted round-robin.
	PolicyRoundRobin = "round-robin"
	// PolicyLeastConnections picks the healthy upstream with the fewest
	// in-flight requests relative to its weight.
	PolicyLeastConnections = "least-connections"
	// PolicyIPHash pins each client IP to an upstream via rendezvous hashing,
	// so a client keeps hitting the same backend while it stays healthy
	// (sticky sessions). Weights are ignored.
	PolicyIPHash = "ip-hash"
)

// UpstreamGroup is an ordered set of interchangeable backends a proxy host can
// forward to instead of a single upstream. Policy selects how requests are
// spread across the healthy upstreams; with the default failover policy the
// first upstream is primary and later entries are backups in order. Health is
// tracked per group (active probes plus passive connection failures), so many
// hosts sharing the same backend set reference one group and the backends are
// probed once, not once per host.
type UpstreamGroup struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	// Upstreams is the ordered backend list. Under the failover policy order is
	// preference; under the distribution policies order only breaks ties.
	Upstreams []GroupUpstream `json:"upstreams" yaml:"upstreams"`

	// Policy is the load-distribution policy: failover (default when empty),
	// round-robin, least-connections, or ip-hash.
	Policy string `json:"policy,omitempty" yaml:"policy,omitempty"`

	// Stickiness, when set, pins each client to its assigned upstream via a
	// signed cookie for a configurable TTL, composing with Policy (which then
	// only picks the initial assignment). nil disables cookie stickiness.
	Stickiness *Stickiness `json:"stickiness,omitempty" yaml:"stickiness,omitempty"`

	// HealthCheck tunes the active probe; zero values select defaults.
	HealthCheck HealthCheck `json:"healthCheck,omitempty" yaml:"healthCheck,omitempty"`
}

// Stickiness configures cookie-based session affinity for an upstream group:
// on first assignment the data plane sets a signed cookie naming the chosen
// upstream, and honors it on later requests while it is unexpired and the
// upstream stays healthy. Expiry is enforced server-side (the timestamp rides
// inside the signed value), so a replayed cookie cannot outlive its TTL. Only
// clients that return cookies get affinity; others fall back to the policy
// (use the ip-hash policy for cookie-less clients).
type Stickiness struct {
	// Cookie is the cookie name; empty selects "gpm-sticky-<group>". Must be an
	// RFC 6265 token. Give groups distinct names if two groups ever serve the
	// same domain (the default is distinct already).
	Cookie string `json:"cookie,omitempty" yaml:"cookie,omitempty"`
	// TTL is how long an assignment lasts: a Go duration ("30m", "12h") with a
	// "d" day suffix also accepted ("3d" = 72h). Required, > 0.
	TTL string `json:"ttl" yaml:"ttl"`
}

// cookieNameRe is the RFC 6265 cookie-name token charset.
var cookieNameRe = regexp.MustCompile("^[!#$%&'*+\\-.^_`|~0-9A-Za-z]+$")

// CookieName returns the configured cookie name or the per-group default.
func (g UpstreamGroup) CookieName() string {
	if g.Stickiness != nil && g.Stickiness.Cookie != "" {
		return g.Stickiness.Cookie
	}
	return "gpm-sticky-" + g.Name
}

// ParseTTL parses the stickiness TTL, accepting Go durations plus a whole-day
// "d" suffix.
func (s Stickiness) ParseTTL() (time.Duration, error) {
	v := strings.TrimSpace(s.TTL)
	if days, ok := strings.CutSuffix(v, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid day duration %q", s.TTL)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(v)
}

func (s Stickiness) validate(group string) error {
	if s.Cookie != "" && !cookieNameRe.MatchString(s.Cookie) {
		return fmt.Errorf("upstream group %q: stickiness.cookie %q is not a valid cookie name", group, s.Cookie)
	}
	d, err := s.ParseTTL()
	if err != nil {
		return fmt.Errorf("upstream group %q: stickiness.ttl: %v", group, err)
	}
	if d <= 0 {
		return fmt.Errorf("upstream group %q: stickiness.ttl must be > 0, got %q", group, s.TTL)
	}
	return nil
}

// GroupUpstream is one backend in an UpstreamGroup: a plain upstream plus an
// optional relative weight used by the weighted policies.
type GroupUpstream struct {
	Upstream `json:",inline" yaml:",inline"`

	// Weight is the relative share for round-robin and least-connections
	// (1-256; 0 means the default of 1). Ignored by failover and ip-hash.
	Weight int `json:"weight,omitempty" yaml:"weight,omitempty"`
}

// EffectiveWeight returns the weight with the default applied.
func (u GroupUpstream) EffectiveWeight() int {
	if u.Weight > 0 {
		return u.Weight
	}
	return 1
}

// HealthCheck configures the active probe for an upstream group. The probe is a
// TCP connect by default; setting Path upgrades it to an HTTP GET where any
// response below 500 counts as healthy (the probe asks "is this entry point
// alive", not "is the application healthy" - a shared backend app failing would
// fail through every entry point equally, so it must not drive failover).
type HealthCheck struct {
	// Path, when set, makes the probe an HTTP GET of this path (e.g. "/ping")
	// against each upstream; empty keeps the plain TCP-connect probe.
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
	// IntervalSeconds is the time between probe rounds (default 5).
	IntervalSeconds int `json:"intervalSeconds,omitempty" yaml:"intervalSeconds,omitempty"`
	// TimeoutSeconds caps each individual probe attempt (default 3).
	TimeoutSeconds int `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
	// Rise is the number of consecutive probe successes that returns an
	// unhealthy upstream to service (default 2).
	Rise int `json:"rise,omitempty" yaml:"rise,omitempty"`
	// Fall is the number of consecutive failures (probe or live-traffic connect
	// errors) that removes an upstream from service (default 2).
	Fall int `json:"fall,omitempty" yaml:"fall,omitempty"`
}

// Health-check defaults and bounds, shared by validation and the data plane.
const (
	DefaultHealthIntervalSeconds = 5
	DefaultHealthTimeoutSeconds  = 3
	DefaultHealthRise            = 2
	DefaultHealthFall            = 2
)

func (g UpstreamGroup) Kind() string { return "UpstreamGroup" }

func (g UpstreamGroup) Validate() error {
	if err := ValidateName(g.Name); err != nil {
		return err
	}
	if len(g.Upstreams) == 0 {
		return fmt.Errorf("upstream group %q: at least one upstream is required", g.Name)
	}
	switch g.Policy {
	case "", PolicyFailover, PolicyRoundRobin, PolicyLeastConnections, PolicyIPHash:
	default:
		return fmt.Errorf("upstream group %q: policy must be failover|round-robin|least-connections|ip-hash, got %q", g.Name, g.Policy)
	}
	seen := map[string]bool{}
	for i, u := range g.Upstreams {
		if err := u.Upstream.validate(); err != nil {
			return fmt.Errorf("upstream group %q upstream[%d]: %w", g.Name, i, err)
		}
		if u.Weight < 0 || u.Weight > 256 {
			return fmt.Errorf("upstream group %q upstream[%d]: weight %d out of range (0-256)", g.Name, i, u.Weight)
		}
		key := fmt.Sprintf("%s://%s:%d", u.Scheme, strings.ToLower(u.Host), u.Port)
		if seen[key] {
			return fmt.Errorf("upstream group %q: duplicate upstream %s", g.Name, key)
		}
		seen[key] = true
	}
	if g.Stickiness != nil {
		if err := g.Stickiness.validate(g.Name); err != nil {
			return err
		}
	}
	return g.HealthCheck.validate(g.Name)
}

func (h HealthCheck) validate(group string) error {
	if h.Path != "" && !strings.HasPrefix(h.Path, "/") {
		return fmt.Errorf("upstream group %q: healthCheck.path must start with \"/\", got %q", group, h.Path)
	}
	if h.IntervalSeconds < 0 || h.IntervalSeconds > 3600 {
		return fmt.Errorf("upstream group %q: healthCheck.intervalSeconds %d out of range (0-3600)", group, h.IntervalSeconds)
	}
	if h.TimeoutSeconds < 0 || h.TimeoutSeconds > 60 {
		return fmt.Errorf("upstream group %q: healthCheck.timeoutSeconds %d out of range (0-60)", group, h.TimeoutSeconds)
	}
	if h.Rise < 0 || h.Rise > 10 {
		return fmt.Errorf("upstream group %q: healthCheck.rise %d out of range (0-10)", group, h.Rise)
	}
	if h.Fall < 0 || h.Fall > 10 {
		return fmt.Errorf("upstream group %q: healthCheck.fall %d out of range (0-10)", group, h.Fall)
	}
	return nil
}

// Interval returns the effective probe interval in seconds.
func (h HealthCheck) Interval() int {
	if h.IntervalSeconds > 0 {
		return h.IntervalSeconds
	}
	return DefaultHealthIntervalSeconds
}

// Timeout returns the effective per-probe timeout in seconds.
func (h HealthCheck) Timeout() int {
	if h.TimeoutSeconds > 0 {
		return h.TimeoutSeconds
	}
	return DefaultHealthTimeoutSeconds
}

// RiseCount returns the effective rise threshold.
func (h HealthCheck) RiseCount() int {
	if h.Rise > 0 {
		return h.Rise
	}
	return DefaultHealthRise
}

// FallCount returns the effective fall threshold.
func (h HealthCheck) FallCount() int {
	if h.Fall > 0 {
		return h.Fall
	}
	return DefaultHealthFall
}
