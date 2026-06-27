package model

import (
	"fmt"
	"net"
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

// AccessList combines HTTP basic auth and ordered IP allow/deny rules. It can be
// attached to a host or an individual location. SatisfyAny mirrors NPM's
// "satisfy any": when true, passing EITHER auth or IP is enough; when false both
// must pass.
type AccessList struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	SatisfyAny bool            `json:"satisfyAny,omitempty" yaml:"satisfyAny,omitempty"`
	BasicAuth  []BasicAuthUser `json:"basicAuth,omitempty" yaml:"basicAuth,omitempty"`
	Rules      []IPRule        `json:"rules,omitempty" yaml:"rules,omitempty"`
	// DefaultAction applies when no IP rule matches: deny (default) or allow.
	DefaultAction string `json:"defaultAction,omitempty" yaml:"defaultAction,omitempty"`
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
	for _, u := range a.BasicAuth {
		if u.Username == "" || u.PasswordHash == "" {
			return fmt.Errorf("access list %q: basic-auth user requires username and passwordHash", a.Name)
		}
	}
	return nil
}
