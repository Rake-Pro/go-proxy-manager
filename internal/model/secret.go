package model

import (
	"encoding/json"
	"fmt"
	"os"
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
