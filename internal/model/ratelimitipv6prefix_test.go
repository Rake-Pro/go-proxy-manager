package model

import (
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// rateLimit.ipv6Prefix accepts 48..64 (0 = unset = 64) and refuses anything
// else, on a middleware and on an inline host block alike.
func TestRateLimitIPv6PrefixValidation(t *testing.T) {
	for _, tc := range []struct {
		prefix int
		ok     bool
	}{
		{0, true}, {48, true}, {56, true}, {64, true},
		{47, false}, {65, false}, {128, false}, {-1, false}, {32, false},
	} {
		mw := Middleware{ObjectMeta: ObjectMeta{Name: "rl"}, Type: MWTypeRateLimit,
			RateLimit: &RateLimitMiddleware{RequestsPerSecond: 1, IPv6Prefix: tc.prefix}}
		err := mw.Validate()
		if tc.ok != (err == nil) {
			t.Fatalf("middleware ipv6Prefix %d: err = %v, want ok=%v", tc.prefix, err, tc.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "rateLimit.ipv6Prefix") {
			t.Fatalf("middleware ipv6Prefix %d: err = %v, want a rateLimit.ipv6Prefix error", tc.prefix, err)
		}
		h := ProxyHost{ObjectMeta: ObjectMeta{Name: "h"}, Domains: []string{"h.example.com"},
			Upstream:  Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80},
			RateLimit: &RateLimitMiddleware{RequestsPerSecond: 1, IPv6Prefix: tc.prefix}}
		if err := h.Validate(); tc.ok != (err == nil) {
			t.Fatalf("inline ipv6Prefix %d: err = %v, want ok=%v", tc.prefix, err, tc.ok)
		}
	}
}

// The key is spelled ipv6Prefix in YAML and JSON, and an unset value is
// omitted, so an existing spec encodes exactly as before (its rate-limit
// state, keyed by the encoded spec, survives the upgrade).
func TestRateLimitIPv6PrefixEncoding(t *testing.T) {
	var y RateLimitMiddleware
	if err := yaml.Unmarshal([]byte("requestsPerSecond: 1\nipv6Prefix: 56\n"), &y); err != nil || y.IPv6Prefix != 56 {
		t.Fatalf("yaml: got %+v, %v", y, err)
	}
	var j RateLimitMiddleware
	if err := json.Unmarshal([]byte(`{"requestsPerSecond":1,"ipv6Prefix":48}`), &j); err != nil || j.IPv6Prefix != 48 {
		t.Fatalf("json: got %+v, %v", j, err)
	}
	b, _ := json.Marshal(RateLimitMiddleware{RequestsPerSecond: 1})
	if strings.Contains(string(b), "ipv6Prefix") {
		t.Fatalf("unset ipv6Prefix must be omitted, got %s", b)
	}
	out, _ := yaml.Marshal(RateLimitMiddleware{RequestsPerSecond: 1})
	if strings.Contains(string(out), "ipv6Prefix") {
		t.Fatalf("unset ipv6Prefix must be omitted from YAML, got %s", out)
	}
}
