package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// TOTP for the local admin account, per RFC 6238 with the parameters every
// authenticator app defaults to: HMAC-SHA-1, 6 digits, a 30-second step.
// Implemented on the standard library alone (crypto/hmac + crypto/sha1 +
// encoding/base32) - a second factor is not worth a dependency.
const (
	totpDigits = 6
	totpStep   = 30 // seconds per counter step
	// totpSkewSteps is how many steps either side of "now" are accepted, to
	// absorb clock drift between the server and the authenticator app. One step
	// each way is the RFC's own suggestion and keeps the acceptance window at
	// 90 seconds.
	totpSkewSteps = 1
	// totpSecretBytes is the entropy behind a generated secret (160 bits, the
	// RFC 4226 recommendation and what every authenticator app expects).
	totpSecretBytes = 20
)

// totpBase32 is the alphabet authenticator apps use for the shared secret:
// standard base32, unpadded on output, case-insensitive and padding-tolerant on
// input (see NormalizeTOTPSecret).
var totpBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NormalizeTOTPSecret decodes a base32 TOTP secret as an authenticator app
// would: case-insensitive, with spaces and hyphens (the grouping people paste
// out of a setup page) removed and any "=" padding tolerated. It returns the
// raw key bytes.
func NormalizeTOTPSecret(s string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', '-':
			return -1
		}
		return r
	}, s)
	cleaned = strings.ToUpper(strings.TrimRight(cleaned, "="))
	if cleaned == "" {
		return nil, fmt.Errorf("totp secret is empty")
	}
	key, err := totpBase32.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("totp secret is not valid base32: %w", err)
	}
	if len(key) < 10 {
		return nil, fmt.Errorf("totp secret decodes to %d bytes; at least 10 (80 bits) are required", len(key))
	}
	return key, nil
}

// NewTOTPSecret mints a fresh 160-bit secret, base32-encoded for entry into an
// authenticator app.
func NewTOTPSecret() (string, error) {
	b := make([]byte, totpSecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return totpBase32.EncodeToString(b), nil
}

// TOTPURI builds the otpauth:// enrolment URI for secret. Any QR encoder can
// render it; gpm deliberately ships no QR renderer.
func TOTPURI(issuer, account, secret string) string {
	if issuer == "" {
		issuer = "gpm"
	}
	if account == "" {
		account = "admin"
	}
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", strconv.Itoa(totpDigits))
	q.Set("period", strconv.Itoa(totpStep))
	// The label is "issuer:account", each part percent-encoded but the ":"
	// separator left literal, which is what the otpauth key-uri format specifies.
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	// Encode() writes a space as "+", which the otpauth key-uri format does not
	// define; percent-encoding is what authenticator apps agree on. A literal
	// "+" in a value is already escaped to %2B by Encode, so this is unambiguous.
	return "otpauth://totp/" + label + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
}

// TOTPCode returns the RFC 6238 code for key at the given counter (unix time
// divided by the step). Exported for tests and for any future enrolment
// verification step.
func TOTPCode(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	trunc := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, trunc%mod)
}

// totpCounter is the RFC 6238 time step for a unix timestamp.
func totpCounter(unix int64) uint64 {
	if unix < 0 {
		return 0
	}
	return uint64(unix) / totpStep
}

// verifyTOTP checks code against key over the accepted window centred on now,
// and returns the counter that matched. Codes are compared in constant time and
// every candidate step is evaluated, so neither the comparison nor the loop
// leaks which step (or whether an early one) matched.
func verifyTOTP(key []byte, code string, now int64) (uint64, bool) {
	code = strings.TrimSpace(code)
	if len(key) == 0 || len(code) != totpDigits {
		return 0, false
	}
	center := totpCounter(now)
	var (
		matched   uint64
		matchedOK bool
	)
	for skew := -totpSkewSteps; skew <= totpSkewSteps; skew++ {
		c := center
		switch {
		case skew < 0:
			d := uint64(-skew)
			if c < d {
				continue
			}
			c -= d
		case skew > 0:
			c += uint64(skew)
		}
		if subtle.ConstantTimeCompare([]byte(TOTPCode(key, c)), []byte(code)) == 1 {
			matched, matchedOK = c, true
		}
	}
	return matched, matchedOK
}
