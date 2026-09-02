package model

import (
	"fmt"
	"strings"
)

// BasicAuthUser is an HTTP basic-auth credential. Only the bcrypt hash is ever
// stored; the plaintext is hashed at the API/CLI boundary before it reaches here.
//
// It is shared by the auth middleware's `mode: basic` block (BasicAuthSpec) and
// by the deprecated AccessList.BasicAuth, so both express one credential the
// same way and one verifier serves both (see internal/dataplane/basicauth.go).
type BasicAuthUser struct {
	Username     string `json:"username" yaml:"username"`
	PasswordHash string `json:"passwordHash" yaml:"passwordHash"` // bcrypt
}

// MaxBasicAuthRealmLen caps the realm string. It is echoed verbatim into a
// WWW-Authenticate header, so it is bounded like any other operator value that
// reaches the wire.
const MaxBasicAuthRealmLen = 128

// MaxBasicAuthUsers caps how many credentials one basic-auth spec may carry.
// Every write of the spec costs one bcrypt hash per plaintext password the API
// is handed, and bcrypt is deliberately expensive; 64 is far above any real
// local credential set and keeps that cost bounded.
const MaxBasicAuthUsers = 64

// BasicAuthSpec is the `mode: basic` block of an auth middleware: a set of
// username/bcrypt-hash credentials and the realm the 401 challenge advertises.
//
// This is where username/password gating belongs. The same users on an
// AccessList (AccessList.BasicAuth) are deprecated: a login mechanism living in
// the IP/geo tier is what forced both the SatisfyAny flag and the L4 rejection
// special case in Config.Validate.
type BasicAuthSpec struct {
	// Users are the accepted credentials. At least one is required; a spec with
	// none could never admit anyone and is refused rather than compiled into a
	// gate that always 401s.
	Users []BasicAuthUser `json:"users,omitempty" yaml:"users,omitempty"`
	// Realm is the realm advertised in the WWW-Authenticate challenge. Empty
	// means the gated host's name. It is echoed verbatim into a quoted header
	// parameter, so it must be printable ASCII with no quote or backslash.
	Realm string `json:"realm,omitempty" yaml:"realm,omitempty"`
}

// RealmOrDefault returns the configured realm, or def when none is set.
func (b BasicAuthSpec) RealmOrDefault(def string) string {
	if b.Realm == "" {
		return def
	}
	return b.Realm
}

// looksLikeBcrypt reports whether h has the shape of a bcrypt hash: the modular
// crypt prefix ($2a$ / $2b$ / $2y$), a two-digit cost, and the 53-character
// salt+digest tail. It is a SHAPE check, not a verification - a hash that parses
// here can still be wrong, but a value that fails here (a plaintext password,
// most of all) could never authenticate anyone and is better refused at write
// time than discovered as a permanent 401.
func looksLikeBcrypt(h string) bool {
	if len(h) != 60 {
		return false
	}
	switch h[:4] {
	case "$2a$", "$2b$", "$2y$":
	default:
		return false
	}
	return h[4] >= '0' && h[4] <= '9' && h[5] >= '0' && h[5] <= '9' && h[6] == '$'
}

// validate checks one basic-auth spec. owner is the already-quoted owner phrase
// the error is prefixed with ("middleware \"basic\"", "proxy host \"app\"",
// ...), so a `type: auth` middleware and the identical inline block on a host or
// location are held to exactly the same rules.
func (b *BasicAuthSpec) validate(owner string) error {
	if len(b.Users) == 0 {
		return fmt.Errorf("%s: auth.basic.users requires at least one user", owner)
	}
	if len(b.Users) > MaxBasicAuthUsers {
		return fmt.Errorf("%s: auth.basic.users has %d users, at most %d are allowed", owner, len(b.Users), MaxBasicAuthUsers)
	}
	seen := map[string]bool{}
	for _, u := range b.Users {
		if u.Username == "" {
			return fmt.Errorf("%s: auth.basic.users has an entry with no username", owner)
		}
		if strings.ContainsAny(u.Username, ":\r\n") {
			return fmt.Errorf("%s: auth.basic.users username %q must not contain %q or a line break (it is compared against the colon-separated Authorization header)", owner, u.Username, ":")
		}
		if seen[u.Username] {
			return fmt.Errorf("%s: auth.basic.users has duplicate username %q", owner, u.Username)
		}
		seen[u.Username] = true
		if u.PasswordHash == "" {
			return fmt.Errorf("%s: auth.basic.users[%q] requires passwordHash", owner, u.Username)
		}
		if !looksLikeBcrypt(u.PasswordHash) {
			return fmt.Errorf("%s: auth.basic.users[%q].passwordHash is not a bcrypt hash (expected a 60-character $2a$/$2b$/$2y$ value); POST the plaintext as \"password\" and gpm hashes it, or generate one with \"htpasswd -nbB\"", owner, u.Username)
		}
	}
	if len(b.Realm) > MaxBasicAuthRealmLen {
		return fmt.Errorf("%s: auth.basic.realm is %d characters, at most %d are allowed", owner, len(b.Realm), MaxBasicAuthRealmLen)
	}
	for _, c := range b.Realm {
		// The realm is echoed into a quoted WWW-Authenticate parameter. A quote
		// or backslash would break out of it, and a control or non-ASCII
		// character is not representable there at all (RFC 7235 quoted-string).
		if c == '"' || c == '\\' || c < 0x20 || c > 0x7e {
			return fmt.Errorf("%s: auth.basic.realm contains %q: use printable ASCII without %q or %q (it is sent verbatim in a quoted WWW-Authenticate parameter)", owner, c, "\"", "\\")
		}
	}
	return nil
}
