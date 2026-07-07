package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// MarshalJSON redacts literal secrets so real values never leave the process in
// an API response. A placeholder marshals verbatim (the UI needs it to
// round-trip), an empty secret marshals as "", and any literal non-empty value
// marshals as a fixed sentinel. UnmarshalJSON is intentionally left as the
// default so inbound writes keep their raw value, and there is no MarshalYAML so
// the at-rest config keeps storing real placeholders.
func (s Secret) MarshalJSON() ([]byte, error) {
	switch {
	case s.IsEmpty():
		return json.Marshal("")
	case s.IsPlaceholder():
		return json.Marshal(string(s))
	default:
		return json.Marshal("***")
	}
}

var secretType = reflect.TypeOf(Secret(""))

// LiteralSecrets walks v via reflection (structs, pointers, slices, arrays and
// maps) and returns the dotted field paths of every Secret-typed value holding a
// literal secret (non-placeholder, non-empty). The store uses it to refuse to
// commit plaintext secrets to the git config.
func LiteralSecrets(v any) []string {
	var out []string
	walkSecrets(reflect.ValueOf(v), "", &out)
	return out
}

func walkSecrets(v reflect.Value, path string, out *[]string) {
	if !v.IsValid() {
		return
	}
	if v.Type() == secretType {
		s := Secret(v.String())
		if !s.IsPlaceholder() && !s.IsEmpty() {
			*out = append(*out, path)
		}
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			walkSecrets(v.Elem(), path, out)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if t.Field(i).PkgPath != "" {
				continue
			}
			walkSecrets(v.Field(i), secretPath(path, t.Field(i).Name), out)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkSecrets(v.Index(i), fmt.Sprintf("%s[%d]", path, i), out)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkSecrets(v.MapIndex(k), fmt.Sprintf("%s[%v]", path, k.Interface()), out)
		}
	}
}

func secretPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// secretFileRoots are the directories a ${FILE:...} secret may be read from.
// It defaults to the Docker secret mount; override with GPM_SECRET_FILE_ROOTS,
// a list of absolute directories separated by the OS path-list separator.
func secretFileRoots() []string {
	if v := strings.TrimSpace(os.Getenv("GPM_SECRET_FILE_ROOTS")); v != "" {
		var roots []string
		for _, r := range filepath.SplitList(v) {
			if r = strings.TrimSpace(r); r != "" {
				roots = append(roots, r)
			}
		}
		if len(roots) > 0 {
			return roots
		}
	}
	return []string{"/run/secrets"}
}

// allowedSecretFile confines ${FILE:...} resolution to an allowlisted root so a
// config write cannot point the reader at an arbitrary host file (e.g.
// /etc/shadow). The path must be absolute and, once cleaned (which removes any
// "..") fall within one of secretFileRoots.
func allowedSecretFile(ref string) error {
	if !filepath.IsAbs(ref) {
		return fmt.Errorf("secret file %q must be an absolute path", ref)
	}
	clean := filepath.Clean(ref)
	for _, root := range secretFileRoots() {
		root = filepath.Clean(root)
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("secret file %q is outside the allowed secret roots (set GPM_SECRET_FILE_ROOTS to permit it)", ref)
}

// reservedSecretEnv are gpm's own sensitive process env vars. A config
// ${ENV:...} reference may never resolve them: the configuration never
// legitimately needs gpm's admin password hash or SSO signing key, and an
// admin-authored value must not be able to exfiltrate them (e.g. as a webhook
// secret posted to an attacker-chosen URL). Resolving these always fails.
var reservedSecretEnv = map[string]bool{
	"GPM_LOCAL_ADMIN_PASSWORD_HASH": true,
	"GPM_SSO_SIGNING_KEY":           true,
}

// secretEnvPrefixes returns the operator-configured strict allowlist of ${ENV:}
// name prefixes (GPM_SECRET_ENV_PREFIXES, comma-separated). Empty (the default)
// means no prefix restriction beyond the reserved denylist, preserving configs
// that reference arbitrarily named env vars.
func secretEnvPrefixes() []string {
	v := strings.TrimSpace(os.Getenv("GPM_SECRET_ENV_PREFIXES"))
	if v == "" {
		return nil
	}
	var ps []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			ps = append(ps, p)
		}
	}
	return ps
}

// allowedSecretEnv gates ${ENV:...} resolution, mirroring allowedSecretFile.
// gpm's reserved secrets are always refused. When GPM_SECRET_ENV_PREFIXES is set,
// resolution is further confined to names carrying one of those prefixes (strict
// allowlist mode); unset, any non-reserved name resolves.
func allowedSecretEnv(ref string) error {
	if reservedSecretEnv[ref] {
		return fmt.Errorf("secret env var %q is reserved by gpm and cannot be resolved via ${ENV:...}", ref)
	}
	prefixes := secretEnvPrefixes()
	if len(prefixes) == 0 {
		return nil
	}
	for _, p := range prefixes {
		if strings.HasPrefix(ref, p) {
			return nil
		}
	}
	return fmt.Errorf("secret env var %q is outside the allowed ${ENV:...} prefixes (set GPM_SECRET_ENV_PREFIXES to permit it)", ref)
}

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
			if err := allowedSecretEnv(ref); err != nil {
				resErr = err
				return match
			}
			v, ok := os.LookupEnv(ref)
			if !ok {
				resErr = fmt.Errorf("secret env var %q is not set", ref)
				return match
			}
			return v
		case "FILE":
			if err := allowedSecretFile(ref); err != nil {
				resErr = err
				return match
			}
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
