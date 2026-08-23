package model

import (
	"fmt"
	"strings"
	"time"
)

// ScopeAdmin is the catch-all scope: it satisfies every read and write check,
// including the admin-only endpoints (api-tokens, restore, whole-config revert)
// that no per-resource scope can reach.
const ScopeAdmin = "admin"

// ScopeMetricsRead is the scope a token needs to scrape the Prometheus
// exposition at GET /metrics. It is its own subject rather than "*:read"
// because a scrape credential lives in a monitoring config forever and should
// buy nothing else - and because the exposition is not a config read: it names
// hosts and certificates but carries no field values.
const ScopeMetricsRead = "metrics:read"

// ScopePlurals is the central list of scope subjects. Every entry is either a
// REST resource plural (the path segment under /api/) or a pseudo-resource for a
// non-CRUD endpoint group ("settings", "dns-sync", "ingress-discovery",
// "metrics").
// APIToken.Validate checks
// every configured scope against this list, so a typo is rejected at write time
// instead of silently granting nothing.
var ScopePlurals = []string{
	"proxy-hosts",
	"redirect-hosts",
	"stream-hosts",
	"parked-hosts",
	"certificates",
	"client-cas",
	"dns-providers",
	"identity-providers",
	"upstream-groups",
	"access-lists",
	"middlewares",
	"api-tokens",
	"settings",
	"dns-sync",
	"ingress-discovery",
	"metrics",
}

func knownScopePlural(p string) bool {
	for _, k := range ScopePlurals {
		if k == p {
			return true
		}
	}
	return false
}

// splitScope parses "<plural>:<verb>" into its parts. ok is false for anything
// that is not in that shape (including the bare "admin" scope).
// legacyScopePlurals maps a retired scope subject to its replacement so tokens
// minted before a kind rename keep loading and keep granting the same access.
var legacyScopePlurals = map[string]string{
	"dead-hosts": "parked-hosts",
}

func splitScope(s string) (plural, verb string, ok bool) {
	plural, verb, ok = splitScopeRaw(s)
	if ok {
		if canon, legacy := legacyScopePlurals[plural]; legacy {
			plural = canon
		}
	}
	return plural, verb, ok
}

func splitScopeRaw(s string) (plural, verb string, ok bool) {
	i := strings.IndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	plural, verb = s[:i], s[i+1:]
	if verb != "read" && verb != "write" {
		return "", "", false
	}
	return plural, verb, true
}

// ScopeSatisfied reports whether a token holding granted scopes may perform the
// action described by required (e.g. "proxy-hosts:write", "*:read", "admin").
//
// The rules, in order: "admin" satisfies everything; a "*" subject in a granted
// scope matches any required subject (but a granted concrete subject never
// matches a required "*", so a whole-config read still needs "*:read"); and
// write implies read on the same subject, never the reverse.
func ScopeSatisfied(granted []string, required string) bool {
	if required == ScopeAdmin {
		for _, g := range granted {
			if g == ScopeAdmin {
				return true
			}
		}
		return false
	}
	rp, rv, ok := splitScope(required)
	if !ok {
		return false
	}
	for _, g := range granted {
		if g == ScopeAdmin {
			return true
		}
		gp, gv, ok := splitScope(g)
		if !ok {
			continue
		}
		if gp != "*" && gp != rp {
			continue
		}
		if gv == rv || gv == "write" {
			return true
		}
	}
	return false
}

// APIToken is a non-interactive credential for the REST API: a bearer secret
// with an explicit scope list, used by scripts and CI instead of an admin
// session cookie.
//
// The secret itself is NEVER stored. Only its sha256 hex digest is committed,
// in a plain string field rather than a Secret - the store refuses to commit a
// literal Secret value, and a hash is not a secret to be resolved from the
// environment at load time. The plaintext is generated server-side and returned
// exactly once, in the response to the PUT that created (or rotated) it.
type APIToken struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	// TokenHash is the lowercase sha256 hex digest of the token secret. It is
	// set by the server on create/rotate and is not client-writable.
	//
	// json:"-" keeps it out of EVERY API response - not just GET /api-tokens but
	// the whole-config dump and the backup archive, which only need "*:read".
	// A digest is offline-crackable, so handing it to a read-only caller would
	// let them grind for the secret at leisure. The yaml tag is unchanged: at
	// rest in git the digest is exactly what has to persist.
	TokenHash string `json:"-" yaml:"tokenHash,omitempty"`

	// Scopes lists what this token may do, e.g. "proxy-hosts:read",
	// "certificates:write", "*:read" or "admin". At least one is required.
	Scopes []string `json:"scopes" yaml:"scopes"`

	// ExpiresAt, when set, makes the token stop authenticating after that
	// instant. nil means the token never expires on its own.
	ExpiresAt *time.Time `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty"`
}

func (t APIToken) Kind() string { return "APIToken" }

func (t APIToken) Validate() error {
	if err := ValidateName(t.Name); err != nil {
		return err
	}
	if len(t.Scopes) == 0 {
		return fmt.Errorf("api token %q: at least one scope is required", t.Name)
	}
	for _, s := range t.Scopes {
		if s == ScopeAdmin {
			continue
		}
		plural, _, ok := splitScope(s)
		if !ok {
			return fmt.Errorf("api token %q: invalid scope %q (want \"<plural>:read\", \"<plural>:write\", \"*:read\", \"*:write\" or %q)", t.Name, s, ScopeAdmin)
		}
		if plural != "*" && !knownScopePlural(plural) {
			return fmt.Errorf("api token %q: unknown scope subject %q in %q (known: %s, *)", t.Name, plural, s, strings.Join(ScopePlurals, ", "))
		}
	}
	return nil
}

// Expired reports whether the token's expiry has passed at now.
func (t APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && now.After(*t.ExpiresAt)
}
