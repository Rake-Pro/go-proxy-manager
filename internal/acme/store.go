package acme

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// issuedDir is the directory holding issued artifacts for a certificate.
func issuedDir(certDir, name string) string {
	return filepath.Join(certDir, "acme", "issued", name)
}

// IssuedCertPath / IssuedKeyPath expose where an ACME certificate's PEM files
// live so the data plane can load them into its SNI resolver.
func IssuedCertPath(certDir, name string) string {
	return filepath.Join(issuedDir(certDir, name), "fullchain.pem")
}
func IssuedKeyPath(certDir, name string) string {
	return filepath.Join(issuedDir(certDir, name), "privkey.pem")
}

func metaPath(certDir, name string) string {
	return filepath.Join(issuedDir(certDir, name), "meta.json")
}

// Meta is the renewal-relevant state for an issued certificate.
type Meta struct {
	Domains      []string  `json:"domains"`
	DirectoryURL string    `json:"directoryURL"`
	IssuedAt     time.Time `json:"issuedAt"`
	// NotBefore is the leaf's NotBefore, written at issuance. Metadata written
	// before the field existed has it zero; see Manager.notBefore.
	NotBefore time.Time `json:"notBefore,omitzero"`
	NotAfter  time.Time `json:"notAfter"`
	// Profile is the ACME profile the certificate was ordered with, empty for
	// the CA's default.
	Profile string `json:"profile,omitempty"`
	// ARI is the CA's latest renewal information for this certificate, nil
	// until a poll has run (and always for a CA without ARI).
	ARI *ARIState `json:"ari,omitempty"`
}

// saveMeta rewrites a certificate's metadata alone, for the ARI state a poll
// updates between issuances.
func saveMeta(certDir, name string, m Meta) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(metaPath(certDir, name), b, 0o600)
}

// LoadIssuedMeta reads an ACME certificate's issued metadata; os.ErrNotExist
// (via errors.Is) means "never issued", the pending state GET /certificates
// reports for an acme certificate whose first order has not run yet.
func LoadIssuedMeta(certDir, name string) (*Meta, error) {
	return loadMeta(certDir, name)
}

// loadMeta reads the issued metadata; os.ErrNotExist means "never issued".
func loadMeta(certDir, name string) (*Meta, error) {
	b, err := os.ReadFile(metaPath(certDir, name)) //nolint:gosec // G304: path is certDir/acme/issued/<name>/meta.json; name passed model.ValidateName (one safe segment)
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// saveIssued atomically writes the certificate chain, private key, and metadata.
func saveIssued(certDir, name string, chainPEM []byte, key *ecdsa.PrivateKey, m Meta) error {
	dir := issuedDir(certDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := writeFileAtomic(IssuedCertPath(certDir, name), chainPEM, 0o644); err != nil {
		return err
	}
	if err := writeFileAtomic(IssuedKeyPath(certDir, name), keyPEM, 0o600); err != nil {
		return err
	}
	return saveMeta(certDir, name, m)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
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
	return os.Rename(tmpName, path)
}

// LeafInfo is a certificate leaf's status fields, for the read-only decoration
// GET /certificates adds to every stored Certificate object.
type LeafInfo struct {
	NotBefore time.Time
	NotAfter  time.Time
	Issuer    string
	SANs      []string
}

// ParseLeaf reads the validity window, issuer and subject alternative names off
// the first certificate in a PEM chain (the leaf). It works on both ACME-issued
// and custom certificates: both are stored as a plain PEM chain on disk, so
// there is one parser for both, not two status code paths.
func ParseLeaf(chainPEM []byte) (LeafInfo, error) {
	block, _ := pem.Decode(chainPEM)
	if block == nil {
		return LeafInfo{}, fmt.Errorf("certificate chain is not PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return LeafInfo{}, err
	}
	sans := make([]string, 0, len(leaf.DNSNames)+len(leaf.IPAddresses))
	sans = append(sans, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		sans = append(sans, ip.String())
	}
	return LeafInfo{
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
		Issuer:    leaf.Issuer.CommonName,
		SANs:      sans,
	}, nil
}
