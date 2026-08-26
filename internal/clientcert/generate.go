package clientcert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

const (
	// DefaultCAValidityDays is ten years. A private client-certificate CA is a
	// trust anchor pinned into device configuration, so rotating it means
	// re-provisioning every device - the opposite trade-off from the leaf
	// certificates it signs.
	DefaultCAValidityDays = 3650
	// MaxCAValidityDays caps a generated CA at twenty years; MinCAValidityDays
	// at one day (useful only for tests and short-lived labs).
	MaxCAValidityDays = 7300
	MinCAValidityDays = 1

	// MaxOrganizationLen is the X.509 upper bound on an organization name
	// (RFC 5280 ub-organization-name-length), enforced here so an oversized
	// value is a 400 rather than an opaque ASN.1 failure.
	MaxOrganizationLen = 64

	// caRSABits is 4096, where an issued client certificate gets 2048. The key
	// type is RSA for the same reason the leaves are (see rsaBits: iOS rejects
	// ECDSA client certificates, so an ECDSA CA would only be able to sign keys
	// the fleet cannot use). The size is doubled because a CA generated here is
	// dated for a decade by default and cannot be rotated without touching every
	// device that trusts it, whereas a leaf is renewed in a year.
	caRSABits = 4096

	// caKeyDir is where a generated CA's private key lands inside the managed
	// certificate store, beside the client-certs/ issuance records.
	caKeyDir = "client-cas"
)

// ErrCAExists reports that a ClientCA of this name is already configured.
var ErrCAExists = errors.New("a client CA with this name already exists")

// ErrKeyFileExists reports that the derived key path is already occupied. It is
// never overwritten: that file may be the signing key of a CA some other config
// (or an earlier, deleted one) still depends on, and replacing it would silently
// invalidate every certificate issued from it.
var ErrKeyFileExists = errors.New("a key file for this client CA already exists in the certificate store")

// GenerateRequest is one "create a CA from nothing" call: everything an operator
// would otherwise have to produce with openssl and copy in by hand.
type GenerateRequest struct {
	CommonName   string `json:"commonName,omitempty"`
	ValidityDays int    `json:"validityDays,omitempty"`
	Organization string `json:"organization,omitempty"`
}

// GenerateResult is a freshly generated CA: the public certificate and the
// cert-store-relative path its private key was written to. The key itself is
// deliberately absent - it exists only on disk, at 0600, and is never returned
// or logged.
type GenerateResult struct {
	CertPEM   string
	KeyFile   string
	Subject   string
	Serial    string
	NotBefore time.Time
	NotAfter  time.Time
}

// normalize validates and defaults the request in place. name is the ClientCA
// object name, which is also the default common name.
func (r *GenerateRequest) normalize(name string) error {
	r.CommonName = strings.TrimSpace(r.CommonName)
	if r.CommonName == "" {
		r.CommonName = name
	}
	switch {
	case len(r.CommonName) > MaxCommonNameLen:
		return fmt.Errorf("%w: commonName must be at most %d characters", ErrInvalidRequest, MaxCommonNameLen)
	case strings.ContainsAny(r.CommonName, "\x00\n\r"):
		return fmt.Errorf("%w: commonName contains control characters", ErrInvalidRequest)
	}
	r.Organization = strings.TrimSpace(r.Organization)
	switch {
	case len(r.Organization) > MaxOrganizationLen:
		return fmt.Errorf("%w: organization must be at most %d characters", ErrInvalidRequest, MaxOrganizationLen)
	case strings.ContainsAny(r.Organization, "\x00\n\r"):
		return fmt.Errorf("%w: organization contains control characters", ErrInvalidRequest)
	}
	if r.ValidityDays == 0 {
		r.ValidityDays = DefaultCAValidityDays
	}
	if r.ValidityDays < MinCAValidityDays || r.ValidityDays > MaxCAValidityDays {
		return fmt.Errorf("%w: validityDays must be between %d and %d", ErrInvalidRequest, MinCAValidityDays, MaxCAValidityDays)
	}
	return nil
}

// KeyFileFor returns the cert-store-relative path a generated CA's private key
// is written to. It is derived from the object name rather than chosen by the
// caller, so nothing about the path is attacker-influenced beyond a name the
// store already validated.
func KeyFileFor(name string) string { return caKeyDir + "/" + name + ".key" }

// GenerateCA creates a self-signed client CA and writes its private key into the
// managed certificate store, returning the public certificate and the relative
// key path for the caller to save as a ClientCA object.
//
// ValidatePlan checks everything about a generate request that can be decided
// without touching the certificate store - the object name, the request fields,
// and the confinement of the key path the name derives - and returns the
// normalized request.
//
// It is exported because the ORDER matters to a caller that touches the store on
// the way in: the API handler reclaims an orphaned key file before generating, and
// that reclaim is a deletion, so it must run only once the request is known good.
// A rejected request has to leave the store byte-for-byte untouched. Calling this
// first and passing the result to GenerateCA achieves that; GenerateCA calls it
// again anyway (it is idempotent) so the invariant does not depend on the caller
// remembering.
func ValidatePlan(name string, req GenerateRequest) (GenerateRequest, error) {
	if err := model.ValidateName(name); err != nil {
		return req, fmt.Errorf("%w: %s", ErrInvalidRequest, err)
	}
	if err := req.normalize(name); err != nil {
		return req, err
	}
	keyRel := KeyFileFor(name)
	// ValidateName permits an embedded "..", which would make the derived path
	// fail cert-store confinement. The caller never supplied a path, so the
	// refusal has to talk about the NAME - an error naming caKeyFile would send
	// an operator looking for a field they never filled in.
	if strings.Contains(name, "..") {
		return req, fmt.Errorf(`%w: the CA name %q cannot be generated - its signing key would be stored at %q, which is not confined to the certificate store; choose a name without ".."`,
			ErrInvalidRequest, name, keyRel)
	}
	// Confinement is then checked authoritatively by building the object this call
	// will produce and running the real validator over it, rather than trusting
	// the shortcut above to stay exhaustive. The placeholder caPEM stands in for
	// the certificate that does not exist yet; Validate only requires it to be
	// non-empty (the parse check is deferred to data-plane compile, as it is for
	// any ClientCA).
	probe := model.ClientCA{
		ObjectMeta: model.ObjectMeta{Name: name},
		CAPEM:      "placeholder",
		CAKeyFile:  keyRel,
	}
	if err := probe.Validate(); err != nil {
		return req, fmt.Errorf("%w: %s", ErrInvalidRequest, err)
	}
	return req, nil
}

// Everything that can be checked is checked BEFORE any key is generated: the
// object name, the request fields, the confinement of the derived key path, and
// the absence of the key file. RSA-4096 keygen takes real time, so a request that
// was always going to be refused is refused immediately - and, more importantly,
// a refusal then leaves nothing behind to clean up.
//
// The caller is responsible for the config write and, if that fails, for calling
// RemoveGeneratedKey: this function deliberately does not know about the store.
func GenerateCA(name, certDir string, req GenerateRequest) (*GenerateResult, error) {
	req, err := ValidatePlan(name, req)
	if err != nil {
		return nil, err
	}
	if certDir == "" {
		return nil, ErrNoStore
	}
	keyRel := KeyFileFor(name)

	// Janitorial, before anything is written: a crash between the temp write and
	// the link leaves a temp file holding a raw RSA key.
	sweepStaleTemps(filepath.Join(certDir, caKeyDir))

	keyPath := filepath.Join(certDir, filepath.Clean(keyRel))
	if _, err := os.Stat(keyPath); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrKeyFileExists, keyRel)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("client CA %q: key path %s: %w", name, keyRel, err)
	}

	key, err := rsa.GenerateKey(rand.Reader, caRSABits)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}
	serial, err := newSerial()
	if err != nil {
		return nil, err
	}
	subject := pkix.Name{CommonName: req.CommonName}
	if req.Organization != "" {
		subject.Organization = []string{req.Organization}
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      subject,
		NotBefore:    now.Add(-backdate),
		NotAfter:     now.AddDate(0, 0, req.ValidityDays),
		// A CA that signs end-entity certificates and its own CRLs, and nothing
		// else. MaxPathLen 0 with BasicConstraintsValid marks it pathlen:0, so it
		// can never be used to mint a subordinate CA - this anchor exists to sign
		// client leaves, and a device that trusts it should not be trusting a
		// whole tree underneath it. Both extensions are marked critical by
		// crypto/x509 when IsCA and KeyUsage are set.
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, fmt.Errorf("self-sign CA certificate: %w", err)
	}
	crt, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse generated CA certificate: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: mustMarshalPKCS8(key),
	})
	if err := writeFileExclusive(keyPath, keyPEM, 0o600); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: %s", ErrKeyFileExists, keyRel)
		}
		return nil, fmt.Errorf("client CA %q: write signing key: %w", name, err)
	}

	return &GenerateResult{
		CertPEM:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		KeyFile:   keyRel,
		Subject:   crt.Subject.String(),
		Serial:    fmt.Sprintf("%x", crt.SerialNumber),
		NotBefore: crt.NotBefore,
		NotAfter:  crt.NotAfter,
	}, nil
}

// KeyFileExists reports whether the key file GenerateCA would write for this name
// is already present. The API uses it to decide, with the config in hand, whether
// an existing file is a live signing key or a reclaimable orphan - a decision this
// package cannot make, because it deliberately knows nothing about the store.
func KeyFileExists(name, certDir string) (bool, error) {
	if certDir == "" {
		return false, nil
	}
	if err := model.ValidateName(name); err != nil {
		return false, err
	}
	_, err := os.Stat(filepath.Join(certDir, filepath.Clean(KeyFileFor(name))))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// RemoveGeneratedKey deletes a key file GenerateCA just wrote. It is the caller's
// rollback for a config save that failed after the key landed: without it the
// key file would survive as an orphan and the no-overwrite rule would then block
// every retry, leaving the operator stuck with a name they cannot use and no way
// to fix it from the UI.
func RemoveGeneratedKey(name, certDir string) error {
	if certDir == "" {
		return nil
	}
	err := os.Remove(filepath.Join(certDir, filepath.Clean(KeyFileFor(name))))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// staleTempAge is how old a leftover temp file must be before the sweep removes
// it. It is generously long because the only way to distinguish "abandoned by a
// crash" from "being written right now by a concurrent generate" is age, and
// deleting a live one would break that request. RSA-4096 keygen is seconds, not
// hours, so an hour is far past any legitimate in-flight write.
const staleTempAge = time.Hour

// sweepStaleTemps removes abandoned temp files from the CA key directory. Every
// one of them holds a raw, unencrypted RSA private key: writeFileExclusive writes
// the key to a temp file before linking it into place, so a crash in that window
// leaves the key sitting there at 0600 under a name nothing references and
// nothing will ever clean up.
//
// Best effort by design - it is housekeeping, not part of the operation, so a
// failure here must never fail a generate. Removals are logged because a temp
// file surviving an hour means gpm died mid-generate, which is worth knowing.
func sweepStaleTemps(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no directory yet, or unreadable: nothing to sweep
	}
	cutoff := time.Now().Add(-staleTempAge)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), ".tmp-") {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := os.Remove(path); err != nil {
			log.Warn().Err(err).Str("path", path).
				Msg("could not remove an abandoned client-CA key temp file; it holds a private key, remove it by hand")
			continue
		}
		log.Warn().Str("path", path).Dur("age", time.Since(info.ModTime())).
			Msg("removed an abandoned client-CA key temp file left by an interrupted generate")
	}
}

// mustMarshalPKCS8 marshals an RSA key we just generated. x509.MarshalPKCS8PrivateKey
// only fails on an unsupported key type, which *rsa.PrivateKey is not.
func mustMarshalPKCS8(key *rsa.PrivateKey) []byte {
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic("clientcert: marshalling a freshly generated RSA key cannot fail: " + err.Error())
	}
	return b
}

// writeFileExclusive writes data to a temp file in the target directory and hard
// links it into place. Link is the primitive that gives BOTH properties this file
// needs: it is atomic (a reader never sees a partial key) and it fails with
// os.ErrExist rather than clobbering, which os.Rename would not - and a CA
// signing key is exactly the file where a silent overwrite would invalidate every
// certificate already issued from it.
func writeFileExclusive(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Link(tmpName, path)
}
