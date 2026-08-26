package model

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoSigningKey reports that a ClientCA is verify-only: it has no caKeyFile /
// caKeyPEM, so it can anchor mTLS verification but cannot issue certificates.
// Callers map it to a client-visible refusal rather than a server error.
var ErrNoSigningKey = errors.New("client CA has no signing key configured (set caKeyFile or caKeyPEM)")

// HasSigningKey reports whether this CA carries a signing key and can therefore
// issue client certificates. It is a pure config check - it does not read the
// key file or resolve a placeholder.
func (c ClientCA) HasSigningKey() bool {
	return c.CAKeyFile != "" || !c.CAKeyPEM.IsEmpty()
}

// SigningKey resolves this CA's signing key and returns it together with the
// certificate from caPEM that it belongs to - the issuer a newly minted client
// certificate is signed by. certDir is the managed cert store that caKeyFile is
// resolved relative to (and confined to).
//
// It returns ErrNoSigningKey when the CA is verify-only. Any other error means
// the key is configured but unusable, which is a configuration fault.
func (c ClientCA) SigningKey(certDir string) (*x509.Certificate, crypto.Signer, error) {
	if !c.HasSigningKey() {
		return nil, nil, ErrNoSigningKey
	}
	var keyPEM []byte
	switch {
	case !c.CAKeyPEM.IsEmpty():
		v, err := c.CAKeyPEM.Resolve()
		if err != nil {
			return nil, nil, fmt.Errorf("client CA %q: caKeyPEM: %w", c.Name, err)
		}
		keyPEM = []byte(v)
	default:
		// Re-check the confinement the validator already enforced: this is the
		// call that actually opens the file, so it must not depend on the object
		// having come through Validate.
		if err := confinedStorePath("caKeyFile", c.CAKeyFile); err != nil {
			return nil, nil, fmt.Errorf("client CA %q: %w", c.Name, err)
		}
		b, err := os.ReadFile(filepath.Join(certDir, filepath.Clean(c.CAKeyFile)))
		if err != nil {
			return nil, nil, fmt.Errorf("client CA %q: caKeyFile: %w", c.Name, err)
		}
		keyPEM = b
	}
	signer, err := parsePrivateKeyPEM(keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("client CA %q: %w", c.Name, err)
	}
	caPEM, err := Secret(c.CAPEM).Resolve()
	if err != nil {
		return nil, nil, fmt.Errorf("client CA %q: caPEM: %w", c.Name, err)
	}
	issuer, err := matchIssuer([]byte(caPEM), signer)
	if err != nil {
		return nil, nil, fmt.Errorf("client CA %q: %w", c.Name, err)
	}
	return issuer, signer, nil
}

// validateInlineSigningKey performs the full key/certificate match at config
// validation time for an inline caKeyPEM, which needs no cert store to resolve.
// A placeholder that cannot be resolved in this process is skipped rather than
// rejected, exactly like the deferred caPEM parse check: validation must not
// depend on secrets that only exist at load. A caKeyFile is checked when it is
// first used to issue, since only the data plane knows the cert store path.
func (c ClientCA) validateInlineSigningKey() error {
	if c.CAKeyPEM.IsEmpty() {
		return nil
	}
	keyPEM, err := c.CAKeyPEM.Resolve()
	if err != nil {
		return nil
	}
	caPEM, err := Secret(c.CAPEM).Resolve()
	if err != nil {
		return nil
	}
	signer, err := parsePrivateKeyPEM([]byte(keyPEM))
	if err != nil {
		return fmt.Errorf("client CA %q: caKeyPEM: %w", c.Name, err)
	}
	if _, err := matchIssuer([]byte(caPEM), signer); err != nil {
		return fmt.Errorf("client CA %q: %w", c.Name, err)
	}
	return nil
}

// parsePrivateKeyPEM returns the first private key in a PEM blob, accepting
// PKCS#8, PKCS#1 (RSA) and SEC 1 (EC) encodings - the three an operator's
// existing CA key is realistically in.
func parsePrivateKeyPEM(raw []byte) (crypto.Signer, error) {
	rest := raw
	for {
		block, more := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = more
		if !strings.HasSuffix(block.Type, "PRIVATE KEY") {
			continue
		}
		if len(block.Headers) > 0 && block.Headers["Proc-Type"] != "" {
			return nil, errors.New("caKey is passphrase-encrypted; supply an unencrypted PKCS#8 key")
		}
		if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			return asSigner(k)
		}
		if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			return k, nil
		}
		if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
			return k, nil
		}
		return nil, errors.New("caKey does not parse as a PKCS#8, PKCS#1 or SEC 1 private key")
	}
	return nil, errors.New("caKey contains no PEM private-key block")
}

func asSigner(k any) (crypto.Signer, error) {
	switch v := k.(type) {
	case *rsa.PrivateKey:
		return v, nil
	case *ecdsa.PrivateKey:
		return v, nil
	case ed25519.PrivateKey:
		return v, nil
	default:
		return nil, fmt.Errorf("caKey is an unsupported private-key type %T", k)
	}
}

// publicKeyEqual is the comparison every stdlib public-key type implements.
type publicKeyEqual interface {
	Equal(crypto.PublicKey) bool
}

// matchIssuer finds the certificate in the CA bundle whose public key belongs to
// signer. That certificate is the issuer newly minted client certificates are
// signed by, so a bundle carrying an intermediate plus its root resolves to
// whichever one the operator actually holds the key for.
func matchIssuer(caPEM []byte, signer crypto.Signer) (*x509.Certificate, error) {
	pub, ok := signer.Public().(publicKeyEqual)
	if !ok {
		return nil, errors.New("caKey public key is not comparable")
	}
	var seen int
	rest := caPEM
	for {
		block, more := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = more
		if block.Type != "CERTIFICATE" {
			continue
		}
		crt, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		seen++
		if !pub.Equal(crt.PublicKey) {
			continue
		}
		if !crt.IsCA {
			return nil, fmt.Errorf("caKey matches certificate %q in caPEM, but that certificate is not a CA (BasicConstraints CA:false)", crt.Subject.String())
		}
		return crt, nil
	}
	if seen == 0 {
		return nil, errors.New("caPEM parsed to no certificates, so the signing key cannot be matched to an issuer")
	}
	return nil, errors.New("the configured signing key does not match any certificate in caPEM")
}
