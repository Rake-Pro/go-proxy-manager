package api

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/webhook"
)

func TestKindNoun(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{"ProxyHost", "proxy host"},
		{"RedirectHost", "redirect host"},
		{"StreamHost", "stream host"},
		{"ParkedHost", "parked host"},
		{"Certificate", "certificate"},
		{"ClientCA", "client CA"},
		{"IdentityProvider", "identity provider"},
		{"AccessList", "access list"},
		{"Middleware", "middleware"},
		{"UpstreamGroup", "upstream group"},
		{"DNSProvider", "DNS provider"},
		{"APIToken", "API token"},
		{"Settings", "settings"},
		// An unmapped kind must still not leak the PascalCase Go identifier.
		{"SomeFutureThing", "some future thing"},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			if got := kindNoun(tc.kind); got != tc.want {
				t.Errorf("kindNoun(%q) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

// TestKindNounsCoverEveryRegisteredKind fails when a resource kind is added
// without a human noun, which is exactly how "ProxyHost typo not found" got into
// the API in the first place.
func TestKindNounsCoverEveryRegisteredKind(t *testing.T) {
	kinds := []interface{ Kind() string }{
		model.ProxyHost{}, model.RedirectHost{}, model.StreamHost{}, model.ParkedHost{},
		model.Certificate{}, model.ClientCA{}, model.DNSProvider{}, model.IdentityProvider{},
		model.UpstreamGroup{}, model.AccessList{}, model.Middleware{}, model.APIToken{},
		model.Settings{},
	}
	for _, k := range kinds {
		if _, ok := kindNouns[k.Kind()]; !ok {
			t.Errorf("kindNouns has no entry for %q", k.Kind())
		}
	}
}

func TestErrNotFound(t *testing.T) {
	err := errNotFound("ProxyHost", "typo")
	if got, want := err.Error(), `proxy host "typo" not found`; got != want {
		t.Fatalf("errNotFound() = %q, want %q", got, want)
	}
	if strings.Contains(err.Error(), "ProxyHost") {
		t.Errorf("errNotFound() leaks the Go type name: %q", err)
	}
}

// decodeInto is the shape a PUT handler decodes into, with enough nesting to
// produce the dotted JSON paths encoding/json reports.
type decodeTLS struct {
	ForceSSL bool `json:"forceSSL"`
	Port     int  `json:"port"`
}

type decodeTarget struct {
	Name    string            `json:"name"`
	Domains []string          `json:"domains"`
	TLS     decodeTLS         `json:"tls"`
	Labels  map[string]string `json:"labels"`
}

func TestDecodeError(t *testing.T) {
	decode := func(body string) error {
		var v decodeTarget
		return json.Unmarshal([]byte(body), &v)
	}

	tests := []struct {
		name     string
		err      error
		want     string   // exact, when set
		contains []string // fragments, when want is empty
		absent   []string // must NOT appear
	}{
		{
			name: "nil passes through",
			err:  nil,
		},
		{
			name: "wrong type on a nested field names the JSON path",
			err:  decode(`{"tls":{"forceSSL":"yes"}}`),
			want: "field tls.forceSSL expects true or false, got a string",
		},
		{
			name: "wrong type on a nested number field",
			err:  decode(`{"tls":{"port":"8080"}}`),
			want: "field tls.port expects a number, got a string",
		},
		{
			name: "object where a list is expected",
			err:  decode(`{"domains":{"a":1}}`),
			want: "field domains expects a list, got an object",
		},
		{
			name: "list where an object is expected",
			err:  decode(`{"labels":[1,2]}`),
			want: "field labels expects an object, got a list",
		},
		{
			name: "number where a string is expected",
			err:  decode(`{"name":7}`),
			want: "field name expects a string, got a number",
		},
		{
			name: "a non-object body has no field to name",
			err:  decode(`"just a string"`),
			want: "the request body expects an object, got a string",
		},
		{
			name:     "malformed JSON reports the offset",
			err:      decode(`{"name": }`),
			contains: []string{"invalid JSON at offset"},
			absent:   []string{"Go ", "json:"},
		},
		{
			name: "truncated JSON reports the offset it ran out at",
			err:  decode(`{"name": "app"`),
			// encoding/json reports this as a *SyntaxError, so it takes the offset
			// branch; the bare-string form below covers the non-typed case.
			contains: []string{"invalid JSON at offset 14"},
			absent:   []string{"Go "},
		},
		{
			name:     "an untyped truncation error is still translated",
			err:      errors.New("unexpected end of JSON input"),
			contains: []string{"ended before the JSON was complete"},
		},
		{
			name:     "unknown field is named",
			err:      errors.New(`json: unknown field "wibble"`),
			contains: []string{"unknown field wibble"},
			absent:   []string{"json:"},
		},
		{
			name: "a model validation error passes through untouched",
			err:  errors.New(`settings: proxyProtocol.trustedCIDRs is required when proxyProtocol.enabled is true`),
			want: `settings: proxyProtocol.trustedCIDRs is required when proxyProtocol.enabled is true`,
		},
		{
			name:     "any other encoding/json error loses its Go vocabulary",
			err:      errors.New("json: cannot unmarshal string into Go value of type model.ProxyHost"),
			contains: []string{"not valid JSON for this object"},
			absent:   []string{"Go value", "model.ProxyHost"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeError(tc.err)
			if tc.err == nil {
				if got != nil {
					t.Fatalf("decodeError(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("decodeError() = nil, want an error")
			}
			msg := got.Error()
			if tc.want != "" && msg != tc.want {
				t.Fatalf("decodeError() = %q, want %q", msg, tc.want)
			}
			for _, want := range tc.contains {
				if !strings.Contains(msg, want) {
					t.Errorf("decodeError() = %q, want it to contain %q", msg, want)
				}
			}
			for _, bad := range tc.absent {
				if strings.Contains(msg, bad) {
					t.Errorf("decodeError() = %q, must not contain %q", msg, bad)
				}
			}
			// The whole point: no Go struct or type names reach the wire.
			for _, leak := range []string{"Go struct field", "Go value of type"} {
				if strings.Contains(msg, leak) {
					t.Errorf("decodeError() leaks %q: %s", leak, msg)
				}
			}
		})
	}
}

// TestRedactDeliveryURLsFailsClosed covers R2-L1: a value that is not a
// []webhook.Delivery must not be returned unredacted to a caller without the
// admin scope. Deps.WebhookStatus/NotificationStatus are func() any, so
// nothing today stops a future change to either status type from producing a
// value this function does not recognise - the "I don't know how to redact
// this" branch must default to withholding it, not to passing it through.
func TestRedactDeliveryURLsFailsClosed(t *testing.T) {
	type notDelivery struct{ Secret string }

	got := redactDeliveryURLs(notDelivery{Secret: "s3cr3t"}, false)
	ds, ok := got.([]webhook.Delivery)
	if !ok {
		t.Fatalf("redactDeliveryURLs(unrecognised type, admin=false) = %#v (%T), want []webhook.Delivery", got, got)
	}
	if len(ds) != 0 {
		t.Errorf("redactDeliveryURLs(unrecognised type, admin=false) = %#v, want an empty slice", ds)
	}

	// An admin caller is unaffected: the value passes through exactly, even
	// though it is not the expected type - admin already sees everything.
	adminGot := redactDeliveryURLs(notDelivery{Secret: "s3cr3t"}, true)
	if nd, ok := adminGot.(notDelivery); !ok || nd.Secret != "s3cr3t" {
		t.Errorf("redactDeliveryURLs(unrecognised type, admin=true) = %#v, want the value unchanged", adminGot)
	}
}
