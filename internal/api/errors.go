package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// This file is the one translation layer between internal error text and the
// {"error": "..."} envelope the SPA renders as a toast. Nothing here changes the
// response SHAPE - only the words.
//
// Two classes of internal vocabulary used to reach users verbatim:
//
//  1. The generic CRUD 404 built its message from the Go type name, so
//     GET /api/proxy-hosts/typo answered {"error":"ProxyHost typo not found"} -
//     a raw Go identifier in a public API response.
//  2. A malformed request body returned encoding/json's error verbatim, e.g.
//     "json: cannot unmarshal string into Go struct field ProxyHost.tls.forceSSL
//     of type bool", which names a Go struct and a Go type for a mistake the user
//     made in a form field.
//
// model.Validate() errors are deliberately NOT rewritten: they already name the
// JSON field path the operator edits (e.g. "proxyProtocol.trustedCIDRs"), which
// is the same vocabulary as the YAML and the API, so passing them through is the
// correct behaviour rather than an omission.

// kindNouns maps a config kind (the Go type name carried as resource.kind) to
// the human singular noun used in the UI, the docs and the API paths.
var kindNouns = map[string]string{
	"ProxyHost":        "proxy host",
	"RedirectHost":     "redirect host",
	"StreamHost":       "stream host",
	"ParkedHost":       "parked host",
	"Certificate":      "certificate",
	"ClientCA":         "client CA",
	"IdentityProvider": "identity provider",
	"AccessList":       "access list",
	"Middleware":       "middleware",
	"UpstreamGroup":    "upstream group",
	"DNSProvider":      "DNS provider",
	"APIToken":         "API token",
	"Settings":         "settings",
}

// kindNoun renders a kind as its human singular noun. An unmapped kind (a new
// resource whose entry was forgotten) degrades to a space-separated lowercase
// form of the Go name rather than leaking the PascalCase identifier.
func kindNoun(kind string) string {
	if n, ok := kindNouns[kind]; ok {
		return n
	}
	var b strings.Builder
	for i, r := range kind {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// errNotFound is the user-facing "no such object" error for every CRUD route.
func errNotFound(kind, name string) error {
	return fmt.Errorf("%s %q not found", kindNoun(kind), name)
}

// decodeError translates an encoding/json failure into a message naming the JSON
// field the caller actually sent, with no Go struct or Go type names in it. Any
// error that is not a JSON decode failure is returned unchanged, so
// model.Validate() messages keep their field paths.
func decodeError(err error) error {
	if err == nil {
		return nil
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		want := jsonTypeName(typeErr.Type)
		got := jsonValueName(typeErr.Value)
		if typeErr.Field == "" {
			return fmt.Errorf("the request body expects %s, got %s", want, got)
		}
		return fmt.Errorf("field %s expects %s, got %s", typeErr.Field, want, got)
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Errorf("invalid JSON at offset %d: %s", syntaxErr.Offset, syntaxErr.Error())
	}
	msg := err.Error()
	if field, ok := unknownFieldName(msg); ok {
		return fmt.Errorf("unknown field %s", field)
	}
	// Anything else encoding/json produces (a bare "unexpected end of JSON input",
	// or a "Go value of type ..." for a non-struct target) still carries package
	// vocabulary, so it is replaced wholesale rather than edited.
	if strings.HasPrefix(msg, "json: ") || strings.Contains(msg, "Go struct field") || strings.Contains(msg, "Go value of type") {
		return errors.New("the request body is not valid JSON for this object")
	}
	if strings.Contains(msg, "unexpected end of JSON input") {
		return errors.New("the request body ended before the JSON was complete")
	}
	return err
}

// unknownFieldName pulls the field out of encoding/json's
// `json: unknown field "x"` (produced when a decoder sets DisallowUnknownFields).
func unknownFieldName(msg string) (string, bool) {
	const prefix = `json: unknown field `
	i := strings.Index(msg, prefix)
	if i < 0 {
		return "", false
	}
	name := strings.Trim(strings.TrimSpace(msg[i+len(prefix):]), `"`)
	if name == "" {
		return "", false
	}
	return name, true
}

// jsonTypeName describes a Go target type in JSON vocabulary.
func jsonTypeName(t reflect.Type) string {
	if t == nil {
		return "a different type"
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "true or false"
	case reflect.String:
		return "a string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Map, reflect.Struct:
		return "an object"
	case reflect.Interface:
		return "any JSON value"
	default:
		return "a different type"
	}
}

// jsonValueName renders encoding/json's description of the offending value
// ("string", "number", "bool", "array", "object", "null") as English.
func jsonValueName(v string) string {
	switch v {
	case "":
		return "something else"
	case "bool":
		return "true or false"
	case "string":
		return "a string"
	case "number":
		return "a number"
	case "array":
		return "a list"
	case "object":
		return "an object"
	case "null":
		return "null"
	default:
		return v
	}
}
