package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/acme"
)

// accountKeyPath returns the on-disk path for the account key for a directory.
// One account key per ACME directory URL, shared across all certs. An EAB key id
// widens the identity: two external accounts on the same CA must not share one
// ACME account key. An empty kid hashes exactly as before, so keys registered
// before EAB existed keep their path.
func accountKeyPath(certDir, directoryURL, eabKID string) string {
	seed := directoryURL
	if eabKID != "" {
		seed = directoryURL + "|" + eabKID
	}
	sum := sha256.Sum256([]byte(seed))
	return filepath.Join(certDir, "acme", "accounts", hex.EncodeToString(sum[:8])+".key")
}

func loadOrCreateAccountKey(certDir, directoryURL, eabKID string) (*ecdsa.PrivateKey, error) {
	path := accountKeyPath(certDir, directoryURL, eabKID)
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // G304: path is certDir/acme/accounts/<sha256 hex>.key, no caller-controlled segment
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

// externalAccountBinding resolves an EABSpec into the binding x/crypto/acme
// sends with the new-account request. A nil spec means "no EAB". The HMAC key is
// whatever the CA handed out: base64url (the RFC 8555 form, padded or not) with
// standard base64 accepted as a fallback.
func externalAccountBinding(spec *model.EABSpec) (*acme.ExternalAccountBinding, error) {
	if spec == nil {
		return nil, nil
	}
	kid := strings.TrimSpace(spec.KID)
	if kid == "" {
		return nil, errors.New("acme eab: kid must not be empty")
	}
	raw, err := spec.HMACKey.Resolve()
	if err != nil {
		return nil, fmt.Errorf("acme eab hmacKey: %w", err)
	}
	key, err := decodeEABKey(raw)
	if err != nil {
		return nil, err
	}
	return &acme.ExternalAccountBinding{KID: kid, Key: key}, nil
}

func decodeEABKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("acme eab: hmacKey must not be empty")
	}
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.StdEncoding,
	} {
		if b, err := enc.DecodeString(raw); err == nil && len(b) > 0 {
			return b, nil
		}
	}
	return nil, errors.New("acme eab: hmacKey is not valid base64url")
}

// newClient builds an ACME client for a directory and ensures the account is
// registered (idempotent: an already-registered key is accepted). When the CA
// requires External Account Binding (ZeroSSL, Google Public CA), eab carries the
// kid/HMAC pair that ties the new account to the external one.
func newClient(ctx context.Context, certDir, directoryURL, email string, eab *acme.ExternalAccountBinding) (*acme.Client, error) {
	if directoryURL == "" {
		directoryURL = acme.LetsEncryptURL
	}
	kid := ""
	if eab != nil {
		kid = eab.KID
	}
	key, err := loadOrCreateAccountKey(certDir, directoryURL, kid)
	if err != nil {
		return nil, fmt.Errorf("account key: %w", err)
	}
	client := &acme.Client{Key: key, DirectoryURL: directoryURL, HTTPClient: acmeHTTPClient}

	acct := &acme.Account{ExternalAccountBinding: eab}
	if email != "" {
		acct.Contact = []string{"mailto:" + email}
	}
	if _, err := client.Register(ctx, acct, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("register ACME account: %w", err)
	}
	return client, nil
}

// Bounds for every exchange with an ACME CA. A real CA answers each request in
// well under the timeout and with bodies of a few KiB (a certificate chain is
// the largest); the caps keep a slow, redirecting or oversized answer from
// stalling an order or growing memory with the CA's response.
const (
	acmeHTTPTimeout = 30 * time.Second
	maxACMEBody     = 1 << 20
)

var errACMERedirect = errors.New("acme: the CA answered with a redirect, which is not followed")

// acmeHTTPClient is the HTTP client of every acme.Client. It keeps the default
// transport (so proxy settings from the environment still apply), does not
// follow redirects and caps each response body at maxACMEBody.
var acmeHTTPClient = &http.Client{
	Timeout:       acmeHTTPTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error { return errACMERedirect },
	Transport:     cappedTransport{base: http.DefaultTransport},
}

// cappedTransport wraps every response body in a reader that fails once the
// body exceeds maxACMEBody.
type cappedTransport struct{ base http.RoundTripper }

func (t cappedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	res.Body = &cappedBody{r: io.LimitReader(res.Body, maxACMEBody+1), c: res.Body}
	return res, nil
}

type cappedBody struct {
	r io.Reader
	c io.Closer
	n int64
}

func (b *cappedBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.n += int64(n)
	if b.n > maxACMEBody {
		return n - int(b.n-maxACMEBody), fmt.Errorf("acme: response body exceeds %d bytes", maxACMEBody)
	}
	return n, err
}

func (b *cappedBody) Close() error { return b.c.Close() }
