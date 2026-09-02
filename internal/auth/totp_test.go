package auth

import (
	"encoding/base32"
	"net/url"
	"strings"
	"testing"
)

// rfcSecret is the shared secret RFC 6238's appendix B test vectors use for the
// SHA-1 flavour: the ASCII string "12345678901234567890".
var rfcSecret = []byte("12345678901234567890")

// TestTOTPCodeRFC6238Vectors checks TOTPCode against every SHA-1 row of the
// RFC 6238 appendix B table. The table publishes 8-digit values; gpm issues the
// 6-digit codes authenticator apps use, which are the same truncation modulo
// 10^6 - i.e. the last six digits of the published value.
func TestTOTPCodeRFC6238Vectors(t *testing.T) {
	tests := []struct {
		name  string
		unix  int64
		want8 string // as published by the RFC
		want  string // the 6-digit code gpm issues
	}{
		{"t=59", 59, "94287082", "287082"},
		{"t=1111111109", 1111111109, "07081804", "081804"},
		{"t=1111111111", 1111111111, "14050471", "050471"},
		{"t=1234567890", 1234567890, "89005924", "005924"},
		{"t=2000000000", 2000000000, "69279037", "279037"},
		{"t=20000000000", 20000000000, "65353130", "353130"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.HasSuffix(tc.want8, tc.want) {
				t.Fatalf("test table is inconsistent: %q is not the tail of %q", tc.want, tc.want8)
			}
			got := TOTPCode(rfcSecret, totpCounter(tc.unix))
			if got != tc.want {
				t.Fatalf("TOTPCode at t=%d = %q, want %q", tc.unix, got, tc.want)
			}
		})
	}
}

// TestVerifyTOTPWindow pins the acceptance window: the current step and one
// step either side, and nothing beyond.
func TestVerifyTOTPWindow(t *testing.T) {
	const now int64 = 1111111109
	center := totpCounter(now)

	tests := []struct {
		name    string
		counter uint64
		want    bool
	}{
		{"two steps early", center - 2, false},
		{"one step early", center - 1, true},
		{"current step", center, true},
		{"one step late", center + 1, true},
		{"two steps late", center + 2, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code := TOTPCode(rfcSecret, tc.counter)
			matched, ok := verifyTOTP(rfcSecret, code, now)
			if ok != tc.want {
				t.Fatalf("verifyTOTP(code for counter %d) ok = %v, want %v", tc.counter, ok, tc.want)
			}
			if ok && matched != tc.counter {
				t.Fatalf("verifyTOTP matched counter %d, want %d", matched, tc.counter)
			}
		})
	}
}

func TestVerifyTOTPRejectsMalformed(t *testing.T) {
	const now int64 = 1111111109
	valid := TOTPCode(rfcSecret, totpCounter(now))

	tests := []struct {
		name string
		key  []byte
		code string
	}{
		{"empty code", rfcSecret, ""},
		{"too few digits", rfcSecret, valid[:5]},
		{"too many digits", rfcSecret, valid + "0"},
		{"not a code", rfcSecret, "abcdef"},
		{"wrong code", rfcSecret, TOTPCode(rfcSecret, totpCounter(now)+10)},
		{"no key configured", nil, valid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := verifyTOTP(tc.key, tc.code, now); ok {
				t.Fatalf("verifyTOTP accepted %q", tc.code)
			}
		})
	}
}

// TestAcceptTOTPCounterReplay checks a code is single-use: the same step (or an
// earlier one) is refused once accepted, while the next step is fine.
func TestAcceptTOTPCounterReplay(t *testing.T) {
	a := &Authenticator{}
	const c uint64 = 37037037

	if !a.acceptTOTPCounter("admin", c) {
		t.Fatal("first use of a counter was rejected")
	}
	if a.acceptTOTPCounter("admin", c) {
		t.Fatal("replay of the same counter was accepted")
	}
	if a.acceptTOTPCounter("admin", c-1) {
		t.Fatal("an earlier counter was accepted after a later one")
	}
	if !a.acceptTOTPCounter("admin", c+1) {
		t.Fatal("the next counter was rejected")
	}
	// The replay ledger is per user, so a second account is unaffected.
	if !a.acceptTOTPCounter("other", c) {
		t.Fatal("a different user was blocked by the first user's counter")
	}
}

func TestNormalizeTOTPSecret(t *testing.T) {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	good := enc.EncodeToString(rfcSecret)

	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"plain", good, false},
		{"lowercase", strings.ToLower(good), false},
		{"spaced in groups", spaceEvery(good, 4), false},
		{"hyphenated", strings.Join([]string{good[:8], good[8:]}, "-"), false},
		{"padded", good + "======", false},
		{"empty", "", true},
		{"not base32", "not-base-32!!", true},
		{"too short", enc.EncodeToString([]byte("12345")), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, err := NormalizeTOTPSecret(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeTOTPSecret(%q) = %x, want an error", tc.in, key)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeTOTPSecret(%q): %v", tc.in, err)
			}
			if string(key) != string(rfcSecret) {
				t.Fatalf("decoded %x, want %x", key, rfcSecret)
			}
		})
	}
}

func TestNewTOTPSecretIsUsable(t *testing.T) {
	s1, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	if s1 == s2 {
		t.Fatal("two generated secrets are identical")
	}
	key, err := NormalizeTOTPSecret(s1)
	if err != nil {
		t.Fatalf("a generated secret does not round-trip: %v", err)
	}
	if len(key) != totpSecretBytes {
		t.Fatalf("generated key is %d bytes, want %d", len(key), totpSecretBytes)
	}
}

func TestTOTPURI(t *testing.T) {
	uri := TOTPURI("Example Proxy", "admin", "ABCDEFGHIJKLMNOP")
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("TOTPURI produced an unparseable URI %q: %v", uri, err)
	}
	if u.Scheme != "otpauth" || u.Host != "totp" {
		t.Fatalf("scheme/host = %q/%q, want otpauth/totp", u.Scheme, u.Host)
	}
	if got, want := u.Path, "/Example%20Proxy:admin"; u.EscapedPath() != want {
		t.Fatalf("label = %q (raw %q), want %q", u.EscapedPath(), got, want)
	}
	q := u.Query()
	for _, kv := range [][2]string{
		{"secret", "ABCDEFGHIJKLMNOP"},
		{"issuer", "Example Proxy"},
		{"algorithm", "SHA1"},
		{"digits", "6"},
		{"period", "30"},
	} {
		if got := q.Get(kv[0]); got != kv[1] {
			t.Errorf("%s = %q, want %q", kv[0], got, kv[1])
		}
	}

	// Spaces are percent-encoded, never "+": authenticator apps do not decode
	// the form-encoded variant.
	if strings.Contains(uri, "+") {
		t.Errorf("URI encodes a space as \"+\": %s", uri)
	}
	if !strings.Contains(uri, "issuer=Example%20Proxy") {
		t.Errorf("issuer is not percent-encoded: %s", uri)
	}
	// A literal "+" in a value stays unambiguous.
	if got := TOTPURI("a+b", "admin", "SECRET"); !strings.Contains(got, "issuer=a%2Bb") {
		t.Errorf("a literal + was not escaped: %s", got)
	}

	// Empty issuer/account fall back rather than producing a malformed label.
	if !strings.Contains(TOTPURI("", "", "SECRET"), "gpm:admin") {
		t.Error("empty issuer/account did not fall back to gpm:admin")
	}
}

func spaceEvery(s string, n int) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%n == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
