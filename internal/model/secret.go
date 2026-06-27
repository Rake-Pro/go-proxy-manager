package model

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Secret is a configuration value that must never be committed in plaintext.
// In the git-backed config files it is stored as a placeholder and the real
// value is resolved at load time from the environment or a file:
//
//	${ENV:OIDC_CLIENT_SECRET}      -> os.Getenv("OIDC_CLIENT_SECRET")
//	${FILE:/run/secrets/cf_token}  -> contents of that file (trimmed)
//
// A bare value with no placeholder is treated as literal (allowed for local dev,
// but the store/linter warns when a literal secret would be committed).
type Secret string

var secretRefRe = regexp.MustCompile(`\$\{(ENV|FILE):([^}]+)\}`)

// IsPlaceholder reports whether s contains a ${ENV:...} or ${FILE:...} reference.
func (s Secret) IsPlaceholder() bool {
	return secretRefRe.MatchString(string(s))
}

// IsEmpty reports whether the secret is unset.
func (s Secret) IsEmpty() bool { return strings.TrimSpace(string(s)) == "" }

// Resolve expands any ${ENV:...} / ${FILE:...} placeholders to their real values.
// Multiple placeholders within one value are supported. A missing env var or
// unreadable file is an error so misconfiguration fails loud, not silent.
func (s Secret) Resolve() (string, error) {
	in := string(s)
	var resErr error
	out := secretRefRe.ReplaceAllStringFunc(in, func(match string) string {
		m := secretRefRe.FindStringSubmatch(match)
		kind, ref := m[1], strings.TrimSpace(m[2])
		switch kind {
		case "ENV":
			v, ok := os.LookupEnv(ref)
			if !ok {
				resErr = fmt.Errorf("secret env var %q is not set", ref)
				return match
			}
			return v
		case "FILE":
			b, err := os.ReadFile(ref)
			if err != nil {
				resErr = fmt.Errorf("secret file %q: %w", ref, err)
				return match
			}
			return strings.TrimRight(string(b), "\r\n")
		default:
			return match
		}
	})
	if resErr != nil {
		return "", resErr
	}
	return out, nil
}
