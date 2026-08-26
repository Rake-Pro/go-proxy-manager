// Package clientcert mints client certificates from a ClientCA that carries a
// signing key, and packages them as a password-protected PKCS#12 bundle for an
// operator to install on a device.
//
// It is deliberately stateless and write-free: nothing it produces is persisted.
// The generated private key exists only inside the returned bundle - it is never
// written to the config store, the cert store, or a log line - so a lost bundle
// means re-issuing, not recovering.
package clientcert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

const (
	// DefaultValidityDays is the validity of a bundle that does not ask for one.
	DefaultValidityDays = 365
	// MaxValidityDays caps issuance at ten years; MinValidityDays at one day.
	MaxValidityDays = 3650
	MinValidityDays = 1
	// MaxCommonNameLen is the X.509 upper bound on a common name (RFC 5280
	// ub-common-name-length), enforced here so an oversized CN is a 400 rather
	// than an opaque ASN.1 failure.
	MaxCommonNameLen = 64
	// MaxSANs bounds the subject-alternative-name list so one request cannot
	// build an unbounded certificate.
	MaxSANs = 32

	// MinPasswordLen is a floor, not a suggestion. The bundle is encoded with
	// pkcs12.Legacy for device compatibility (see below), and that encoder derives
	// its integrity MAC with a SINGLE PBKDF iteration - there is essentially no
	// work factor between the password and the bundle. A .p12 travels by email,
	// chat and shared folder and then sits on a phone, so anyone who obtains one
	// can crack a short password offline at line rate. Twelve characters is the
	// shortest length that keeps that attack uninteresting; the encoder cannot be
	// hardened without breaking the iOS/Android imports this whole feature exists
	// for, so the password is the only control left.
	MinPasswordLen = 12

	// backdate absorbs clock skew between gpm and the device that will present
	// the certificate, so a freshly issued bundle is never "not yet valid".
	backdate = 5 * time.Minute

	// rsaBits is 2048 on purpose. ECDSA client certificates are rejected during
	// the TLS handshake by the iOS companion app this issuance flow exists for
	// (the keychain imports them, then never offers them), and RSA-2048 is the
	// one key type every client in the fleet - iOS, Android, Wear, desktop
	// browsers - handles. Do not "modernise" this to P-256 without re-testing
	// iOS end to end.
	rsaBits = 2048
)

// ErrInvalidRequest is the sentinel for a caller mistake (bad common name,
// missing password, out-of-range validity) as opposed to a server fault.
var ErrInvalidRequest = errors.New("invalid certificate request")

// Request is one issuance: a subject common name, a validity window, the PKCS#12
// password the bundle is encrypted with, and optional subject alternative names.
type Request struct {
	CommonName   string   `json:"commonName"`
	ValidityDays int      `json:"validityDays,omitempty"`
	Password     string   `json:"password"`
	SANs         []string `json:"sans,omitempty"`
}

// Result is a minted bundle plus the metadata worth logging. It deliberately
// carries no private key field: the key is inside PKCS12 and nowhere else.
type Result struct {
	PKCS12   []byte
	Filename string
	Subject  string
	// CommonName and SANs are echoed back as issued (trimmed, empty entries
	// dropped) so a renewal can reissue the same identity from the record rather
	// than trusting a client to resend it.
	CommonName string
	SANs       []string
	Serial     string
	NotBefore  time.Time
	NotAfter   time.Time
}

// normalize validates and defaults the request in place.
func (r *Request) normalize() error {
	r.CommonName = strings.TrimSpace(r.CommonName)
	switch {
	case r.CommonName == "":
		return fmt.Errorf("%w: commonName is required", ErrInvalidRequest)
	case len(r.CommonName) > MaxCommonNameLen:
		return fmt.Errorf("%w: commonName must be at most %d characters", ErrInvalidRequest, MaxCommonNameLen)
	case strings.ContainsAny(r.CommonName, "\x00\n\r"):
		return fmt.Errorf("%w: commonName contains control characters", ErrInvalidRequest)
	}
	switch {
	case r.Password == "":
		return fmt.Errorf("%w: password is required (it encrypts the PKCS#12 bundle)", ErrInvalidRequest)
	case len(r.Password) < MinPasswordLen:
		return fmt.Errorf("%w: password must be at least %d characters - the PKCS#12 bundle is encoded with the legacy (single-iteration) KDF for device compatibility, so its password is the only thing protecting it once the file leaves gpm",
			ErrInvalidRequest, MinPasswordLen)
	}
	if r.ValidityDays == 0 {
		r.ValidityDays = DefaultValidityDays
	}
	if r.ValidityDays < MinValidityDays || r.ValidityDays > MaxValidityDays {
		return fmt.Errorf("%w: validityDays must be between %d and %d", ErrInvalidRequest, MinValidityDays, MaxValidityDays)
	}
	if len(r.SANs) > MaxSANs {
		return fmt.Errorf("%w: at most %d sans may be requested", ErrInvalidRequest, MaxSANs)
	}
	for _, s := range r.SANs {
		if err := validSAN(strings.TrimSpace(s)); err != nil {
			return err
		}
	}
	return nil
}

// validSAN rejects anything x509 cannot encode as an IA5String. DNS and email
// SANs are IA5 (7-bit ASCII) by definition, so a non-ASCII rune - a smart quote
// pasted from a document, an IDN in its unicode form, a stray control character -
// makes x509.CreateCertificate fail deep inside ASN.1 marshalling. Catching it
// here turns an opaque 500 into a 400 that says what to fix. An IDN must be
// supplied already punycoded ("xn--..."), which is what a certificate carries
// anyway.
func validSAN(s string) error {
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			return fmt.Errorf("%w: san %q contains a character that cannot be encoded in a certificate (%q) - subject alternative names are printable ASCII only; supply an internationalised domain in its punycode form",
				ErrInvalidRequest, s, r)
		}
	}
	return nil
}

// Issue mints a client certificate signed by ca's signing key and returns it as
// a PKCS#12 bundle. It returns model.ErrNoSigningKey when the CA is verify-only
// and ErrInvalidRequest (wrapped) for a malformed request; certDir resolves a
// cert-store-relative caKeyFile.
func Issue(ca model.ClientCA, certDir string, req Request) (*Result, error) {
	if err := req.normalize(); err != nil {
		return nil, err
	}
	issuer, signer, err := ca.SigningKey(certDir)
	if err != nil {
		return nil, err
	}

	serial, err := newSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: req.CommonName},
		NotBefore:    now.Add(-backdate),
		NotAfter:     now.AddDate(0, 0, req.ValidityDays),
		// A client credential signs the handshake and nothing else: no key
		// encipherment, no certificate signing, and explicitly not a CA.
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	applySANs(tmpl, req.SANs)

	key, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, fmt.Errorf("generate client key: %w", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer, key.Public(), signer)
	if err != nil {
		return nil, fmt.Errorf("sign client certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse issued certificate: %w", err)
	}

	// pkcs12.Legacy (SHA-1 / 3DES) rather than the modern PBES2 encoder: iOS
	// refuses a PBES2 bundle at keychain import, and several Android and Wear OS
	// releases do the same. The bundle is transported over the authenticated
	// admin session and its password is operator-chosen, so the weaker KDF is an
	// acceptable trade for a bundle that actually installs. Revisit only when the
	// whole client fleet is known to accept Modern.
	pfx, err := pkcs12.Legacy.Encode(key, leaf, []*x509.Certificate{issuer}, req.Password)
	if err != nil {
		return nil, fmt.Errorf("encode pkcs12 bundle: %w", err)
	}

	return &Result{
		PKCS12:     pfx,
		Filename:   SanitizeFilename(req.CommonName) + ".p12",
		Subject:    leaf.Subject.String(),
		CommonName: req.CommonName,
		SANs:       issuedSANs(leaf),
		Serial:     fmt.Sprintf("%x", leaf.SerialNumber),
		NotBefore:  leaf.NotBefore,
		NotAfter:   leaf.NotAfter,
	}, nil
}

// applySANs sorts each requested SAN into the right x509 field. An entry that
// parses as an IP is one; one containing "@" is an email; anything else is a DNS
// name. Unparseable entries are simply not added rather than failing issuance.
func applySANs(tmpl *x509.Certificate, sans []string) {
	for _, s := range sans {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		switch {
		case net.ParseIP(s) != nil:
			tmpl.IPAddresses = append(tmpl.IPAddresses, net.ParseIP(s))
		case strings.Contains(s, "@"):
			tmpl.EmailAddresses = append(tmpl.EmailAddresses, s)
		default:
			tmpl.DNSNames = append(tmpl.DNSNames, s)
		}
	}
}

// issuedSANs flattens the SANs actually placed on the certificate back into the
// flat request form, so a renewal round-trips them unchanged.
func issuedSANs(c *x509.Certificate) []string {
	var out []string
	out = append(out, c.DNSNames...)
	out = append(out, c.EmailAddresses...)
	for _, ip := range c.IPAddresses {
		out = append(out, ip.String())
	}
	return out
}

// newSerial draws a positive 128-bit serial from crypto/rand, the same width the
// public CAs use. It is unguessable and collision-free on its own: the issuance
// ledger in records.go remembers serials, but it is never consulted to CHOOSE
// one, so a lost or pruned record can never cause a serial to be reused.
func newSerial() (*big.Int, error) {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}
	return n.Add(n, big.NewInt(1)), nil
}

// SanitizeFilename reduces a common name to a safe download filename stem: it
// keeps letters, digits, dot, dash and underscore and replaces every other rune
// (spaces, slashes, quotes, control characters, non-ASCII) with a dash, so the
// value can never break out of the Content-Disposition header or the client's
// filesystem.
func SanitizeFilename(cn string) string {
	var b strings.Builder
	for _, r := range cn {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if len(out) > MaxCommonNameLen {
		out = out[:MaxCommonNameLen]
	}
	if out == "" {
		return "client"
	}
	return out
}
