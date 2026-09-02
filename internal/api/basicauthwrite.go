package api

import (
	"encoding/json"
	"fmt"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// basicAuthShadow is the write-only shadow of an `auth` block: it captures the
// plaintext "password" on each `auth.basic.users` entry, which the stored model
// deliberately has no field for.
//
// The plaintext exists only on the way in. It is hashed here and dropped; the
// object that is validated, committed and echoed back carries passwordHash only,
// so a password never reaches the git-backed config or an API response.
type basicAuthShadow struct {
	Basic *struct {
		Users []struct {
			Password string `json:"password"`
		} `json:"users"`
	} `json:"basic"`
}

// middlewareShadow reads the auth block of a `type: auth` middleware body.
type middlewareShadow struct {
	Auth *basicAuthShadow `json:"auth"`
}

// proxyHostShadow reads the inline auth block on a proxy host and on each of its
// locations. Both are the same AuthMiddleware shape, so both accept a plaintext
// password on exactly the same terms a middleware does - otherwise the UI could
// offer `mode: basic` in one editor and not the other.
type proxyHostShadow struct {
	Auth      *basicAuthShadow `json:"auth"`
	Locations []struct {
		Auth *basicAuthShadow `json:"auth"`
	} `json:"locations"`
}

// applyBasicAuthPasswords hashes plaintext passwords from a middleware write
// body onto the decoded auth spec.
func applyBasicAuthPasswords(body []byte, a *model.AuthMiddleware) error {
	var shadow middlewareShadow
	if err := json.Unmarshal(body, &shadow); err != nil {
		// The object itself already decoded, so a failure here is a "password"
		// field of the wrong type. Report it in the caller's vocabulary.
		return decodeError(err)
	}
	return hashInto(shadow.Auth, a, "auth")
}

// applyHostBasicAuthPasswords does the same for a proxy-host write body: the
// host's own inline auth block and each location's.
func applyHostBasicAuthPasswords(body []byte, h *model.ProxyHost) error {
	var shadow proxyHostShadow
	if err := json.Unmarshal(body, &shadow); err != nil {
		return decodeError(err)
	}
	if err := hashInto(shadow.Auth, h.Auth, "auth"); err != nil {
		return err
	}
	for i := range shadow.Locations {
		// The body and the decoded object are the same JSON array, so index i is
		// the same location in both.
		if i >= len(h.Locations) {
			break
		}
		owner := fmt.Sprintf("locations[%d].auth", i)
		if err := hashInto(shadow.Locations[i].Auth, h.Locations[i].Auth, owner); err != nil {
			return err
		}
	}
	return nil
}

// hashInto bcrypt-hashes each plaintext password in shadow onto the matching
// user of a, by position. A user that carries only a passwordHash is left
// untouched, so editing a realm or adding one user never re-hashes (or
// invalidates) the credentials already stored.
//
// The merge is deliberately POSITIONAL rather than a lookup by username: the
// body and the decoded object are the same JSON array, so index i is the same
// user in both, and a caller renaming a user in the same write still gets that
// user's new password.
//
// owner names the block in an error ("auth", "locations[1].auth"), so a failure
// on a location does not read as if it came from the host.
func hashInto(shadow *basicAuthShadow, a *model.AuthMiddleware, owner string) error {
	if shadow == nil || shadow.Basic == nil || a == nil || a.Basic == nil {
		return nil
	}
	// Bound the bcrypt work one request can ask for before doing any of it. The
	// model refuses an oversized user set too, but that check runs after this one
	// would already have spent the CPU.
	if n := len(shadow.Basic.Users); n > model.MaxBasicAuthUsers {
		return fmt.Errorf("%s.basic.users has %d users, at most %d are allowed", owner, n, model.MaxBasicAuthUsers)
	}
	for i, u := range shadow.Basic.Users {
		if u.Password == "" || i >= len(a.Basic.Users) {
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			// The only real cause is a password over bcrypt's 72-byte input limit.
			return fmt.Errorf("%s.basic.users[%q].password could not be hashed: %w", owner, a.Basic.Users[i].Username, err)
		}
		a.Basic.Users[i].PasswordHash = string(hash)
	}
	return nil
}
