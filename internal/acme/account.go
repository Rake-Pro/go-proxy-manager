package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/acme"
)

// accountKeyPath returns the on-disk path for the account key for a directory.
// One account key per ACME directory URL, shared across all certs.
func accountKeyPath(certDir, directoryURL string) string {
	sum := sha256.Sum256([]byte(directoryURL))
	return filepath.Join(certDir, "acme", "accounts", hex.EncodeToString(sum[:8])+".key")
}

func loadOrCreateAccountKey(certDir, directoryURL string) (*ecdsa.PrivateKey, error) {
	path := accountKeyPath(certDir, directoryURL)
	if b, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(b)
		if block == nil {
			return nil, fmt.Errorf("account key %s: not PEM", path)
		}
		return x509.ParseECPrivateKey(block.Bytes)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// newClient builds an ACME client for a directory and ensures the account is
// registered (idempotent: an already-registered key is accepted).
func newClient(ctx context.Context, certDir, directoryURL, email string) (*acme.Client, error) {
	if directoryURL == "" {
		directoryURL = acme.LetsEncryptURL
	}
	key, err := loadOrCreateAccountKey(certDir, directoryURL)
	if err != nil {
		return nil, fmt.Errorf("account key: %w", err)
	}
	client := &acme.Client{Key: key, DirectoryURL: directoryURL}

	acct := &acme.Account{}
	if email != "" {
		acct.Contact = []string{"mailto:" + email}
	}
	if _, err := client.Register(ctx, acct, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("register ACME account: %w", err)
	}
	return client, nil
}
