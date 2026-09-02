// Package ha holds the phase-1 high-availability role model: a two-node pair
// designates exactly one writer (the leader) statically via the environment.
// See design/ha.md.
package ha

import (
	"fmt"
	"strings"
)

// Role is the static HA role of this gpm instance.
type Role string

const (
	// RoleLeader runs the ACME loop and accepts admin/API config writes. It is
	// the default, so a single-node deployment behaves exactly as before.
	RoleLeader Role = "leader"
	// RoleFollower never writes: no ACME loop, no config commits. Its config
	// arrives by pulling the leader's repo (see store.FollowRemote).
	RoleFollower Role = "follower"
)

// EnvRole is the environment variable that selects the role.
const EnvRole = "GPM_HA_ROLE"

// ParseRole maps a GPM_HA_ROLE value to a Role. Empty means leader. An
// unrecognised value is an error rather than a silent default: falling back to
// leader on a typo ("folower") would start a second ACME writer racing the real
// leader on the same account, which is exactly what the role gate prevents.
func ParseRole(v string) (Role, error) {
	switch r := Role(strings.ToLower(strings.TrimSpace(v))); r {
	case "", RoleLeader:
		return RoleLeader, nil
	case RoleFollower:
		return RoleFollower, nil
	default:
		return "", fmt.Errorf("invalid %s %q: want %q or %q", EnvRole, v, RoleLeader, RoleFollower)
	}
}

// IsFollower reports whether this instance must refuse writes.
func (r Role) IsFollower() bool { return r == RoleFollower }

// String normalises the zero value to the leader role.
func (r Role) String() string {
	if r == "" {
		return string(RoleLeader)
	}
	return string(r)
}
