package model

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSecurityHeaderValueAcceptsLegacyPlainMap is the backward-compatibility
// guard: a config or API caller sending the historical map[string]string form
// (name -> bare value, no scope) must still unmarshal, with the scope defaulting
// to "all". Both wire formats (YAML at rest, JSON over the API) are checked.
func TestSecurityHeaderValueAcceptsLegacyPlainMap(t *testing.T) {
	t.Run("yaml old form", func(t *testing.T) {
		const in = `
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
`
		var m map[string]SecurityHeaderValue
		if err := yaml.Unmarshal([]byte(in), &m); err != nil {
			t.Fatalf("unmarshal legacy yaml: %v", err)
		}
		if got := m["X-Frame-Options"]; got.Value != "DENY" || got.Scope != "" {
			t.Fatalf("X-Frame-Options = %+v, want value DENY and empty (=all) scope", got)
		}
		// The empty scope must validate and behave as "all".
		if err := validateSecurityHeaders(m); err != nil {
			t.Fatalf("legacy map must validate, got %v", err)
		}
	})

	t.Run("json old form", func(t *testing.T) {
		const in = `{"X-Frame-Options":"DENY","X-Content-Type-Options":"nosniff"}`
		var m map[string]SecurityHeaderValue
		if err := json.Unmarshal([]byte(in), &m); err != nil {
			t.Fatalf("unmarshal legacy json: %v", err)
		}
		if got := m["X-Content-Type-Options"]; got.Value != "nosniff" || got.Scope != "" {
			t.Fatalf("X-Content-Type-Options = %+v, want value nosniff and empty scope", got)
		}
	})
}

// TestSecurityHeaderValueAcceptsScopedObject covers the new object form and the
// mixed case (some headers plain, one scoped) in the same map.
func TestSecurityHeaderValueAcceptsScopedObject(t *testing.T) {
	const in = `
X-Frame-Options: DENY
Content-Security-Policy:
  value: "frame-ancestors 'none'"
  scope: generated-only
`
	var m map[string]SecurityHeaderValue
	if err := yaml.Unmarshal([]byte(in), &m); err != nil {
		t.Fatalf("unmarshal mixed yaml: %v", err)
	}
	if got := m["X-Frame-Options"]; got.Value != "DENY" || got.Scope != "" {
		t.Fatalf("plain entry = %+v, want DENY/empty", got)
	}
	if got := m["Content-Security-Policy"]; got.Value != "frame-ancestors 'none'" || got.Scope != SecurityScopeGenerated {
		t.Fatalf("scoped entry = %+v, want the CSP at generated-only", got)
	}
	if err := validateSecurityHeaders(m); err != nil {
		t.Fatalf("mixed map must validate, got %v", err)
	}
}

// TestSecurityHeaderValueMarshalRoundTrips pins the marshaller: an all/empty
// scope header marshals back to a bare string (so an unchanged config and
// existing API consumers are byte-for-byte unaffected), and only a non-default
// scope marshals as the {value, scope} object. Round-trips through both codecs.
func TestSecurityHeaderValueMarshalRoundTrips(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		if b, _ := json.Marshal(SecurityHeaderValue{Value: "DENY"}); string(b) != `"DENY"` {
			t.Fatalf("all-scope json = %s, want the bare string \"DENY\"", b)
		}
		if b, _ := json.Marshal(SecurityHeaderValue{Value: "DENY", Scope: SecurityScopeAll}); string(b) != `"DENY"` {
			t.Fatalf("explicit-all json = %s, want the bare string \"DENY\"", b)
		}
		b, _ := json.Marshal(SecurityHeaderValue{Value: "x", Scope: SecurityScopeGenerated})
		var back SecurityHeaderValue
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("round-trip: %v", err)
		}
		if back.Value != "x" || back.Scope != SecurityScopeGenerated {
			t.Fatalf("round-trip = %+v, want x/generated-only (marshalled %s)", back, b)
		}
	})

	t.Run("yaml", func(t *testing.T) {
		b, _ := yaml.Marshal(map[string]SecurityHeaderValue{"X-Frame-Options": {Value: "DENY"}})
		if got := string(b); got != "X-Frame-Options: DENY\n" {
			t.Fatalf("all-scope yaml = %q, want the bare string form", got)
		}
		b, _ = yaml.Marshal(map[string]SecurityHeaderValue{"CSP": {Value: "x", Scope: SecurityScopeProxied}})
		var back map[string]SecurityHeaderValue
		if err := yaml.Unmarshal(b, &back); err != nil {
			t.Fatalf("round-trip: %v", err)
		}
		if got := back["CSP"]; got.Value != "x" || got.Scope != SecurityScopeProxied {
			t.Fatalf("round-trip = %+v, want x/proxied-only (marshalled %q)", got, b)
		}
	})
}
