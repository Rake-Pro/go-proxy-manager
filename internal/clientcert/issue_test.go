package clientcert

import (
	"errors"
	"strings"
	"testing"
)

// TestRequestNormalize is the unit-level table for request validation, so the
// rules are pinned once here rather than only through the HTTP handlers. The
// non-ASCII inputs are built from \u escapes so this source file stays ASCII.
func TestRequestNormalize(t *testing.T) {
	ok := "a-long-enough-pw"
	tests := []struct {
		name     string
		req      Request
		wantErr  string
		wantDays int
	}{
		{name: "minimal valid", req: Request{CommonName: "phone-01", Password: ok}, wantDays: DefaultValidityDays},
		{name: "explicit validity", req: Request{CommonName: "a", Password: ok, ValidityDays: 30}, wantDays: 30},
		{name: "trims the common name", req: Request{CommonName: "  phone-01  ", Password: ok}, wantDays: DefaultValidityDays},
		{name: "ascii sans", req: Request{CommonName: "a", Password: ok,
			SANs: []string{"a.example.com", "ops@example.com", "10.1.2.3", "  "}}, wantDays: DefaultValidityDays},

		{name: "empty common name", req: Request{Password: ok}, wantErr: "commonName is required"},
		{name: "blank common name", req: Request{CommonName: "   ", Password: ok}, wantErr: "commonName is required"},
		{name: "empty password", req: Request{CommonName: "a"}, wantErr: "password is required"},
		{name: "password one short", req: Request{CommonName: "a", Password: strings.Repeat("x", MinPasswordLen-1)},
			wantErr: "at least 12 characters"},
		{name: "password exactly at the floor", req: Request{CommonName: "a", Password: strings.Repeat("x", MinPasswordLen)},
			wantDays: DefaultValidityDays},
		{name: "validity below the floor", req: Request{CommonName: "a", Password: ok, ValidityDays: -1},
			wantErr: "validityDays must be between"},
		{name: "validity over the cap", req: Request{CommonName: "a", Password: ok, ValidityDays: MaxValidityDays + 1},
			wantErr: "validityDays must be between"},

		// SANs are IA5String (7-bit ASCII) in a certificate, so anything else has
		// to be refused here or x509.CreateCertificate fails deep in ASN.1.
		{name: "non-ASCII dns san", req: Request{CommonName: "a", Password: ok,
			SANs: []string{"b\u00fccher.example.com"}}, wantErr: "printable ASCII only"},
		{name: "non-ASCII email san", req: Request{CommonName: "a", Password: ok,
			SANs: []string{"j\u00fcrgen@example.com"}}, wantErr: "printable ASCII only"},
		{name: "control character san", req: Request{CommonName: "a", Password: ok,
			SANs: []string{"a\u0001.example.com"}}, wantErr: "printable ASCII only"},
		{name: "newline san", req: Request{CommonName: "a", Password: ok,
			SANs: []string{"a.example.com\nb.example.com"}}, wantErr: "printable ASCII only"},
		{name: "emoji san", req: Request{CommonName: "a", Password: ok,
			SANs: []string{"\U0001F600.example.com"}}, wantErr: "printable ASCII only"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			err := req.normalize()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
				}
				if !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("every normalize failure must wrap ErrInvalidRequest, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected success, got: %v", err)
			}
			if req.ValidityDays != tc.wantDays {
				t.Fatalf("validityDays = %d, want %d", req.ValidityDays, tc.wantDays)
			}
		})
	}
}

// TestSanitizeFilename pins the download-filename reduction: nothing that could
// break out of the Content-Disposition header or the client filesystem survives.
func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"phone-01":              "phone-01",
		"../../etc/passwd":      "etc-passwd",
		"a b\"c":                "a-b-c",
		"\u00fcber":             "ber",
		"":                      "client",
		"...":                   "client",
		"----":                  "client",
		strings.Repeat("a", 90): strings.Repeat("a", MaxCommonNameLen),
	}
	for in, want := range cases {
		if got := SanitizeFilename(in); got != want {
			t.Fatalf("SanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
