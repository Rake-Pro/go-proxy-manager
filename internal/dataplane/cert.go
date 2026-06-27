// Package dataplane compiles the typed config into a live reverse-proxy data
// plane: a single TLS listener with SNI-based certificate selection, an HTTP
// listener for redirects/ACME, and per-host composable middleware chains.
package dataplane

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/acme"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// certResolver selects a TLS certificate by SNI server name. Built from the
// Certificate objects of type custom (ACME-issued certs join the same map in
// P0c). Exact-domain matches win over wildcard matches.
type certResolver struct {
	exact    map[string]*tls.Certificate // host -> cert
	wildcard map[string]*tls.Certificate // base domain (after "*.") -> cert
}

// buildCertResolver loads certificates referenced by the config into the SNI
// map. Custom certs come from their configured file paths; ACME certs come from
// the issued artifacts on disk. An ACME cert that has not been issued yet is
// skipped (its host has no TLS until the manager issues it), not a fatal error.
func buildCertResolver(certs []model.Certificate, certDir string) (*certResolver, error) {
	r := &certResolver{exact: map[string]*tls.Certificate{}, wildcard: map[string]*tls.Certificate{}}
	for _, c := range certs {
		if c.Disabled {
			continue
		}
		switch c.Type {
		case model.CertTypeCustom:
			if c.Custom == nil {
				continue
			}
			crt, err := loadKeyPair(c.Custom.CertFile, c.Custom.KeyFile, certDir)
			if err != nil {
				return nil, fmt.Errorf("certificate %q: %w", c.Name, err)
			}
			r.add(c.Domains, crt)
		case model.CertTypeACME:
			certPath := acme.IssuedCertPath(certDir, c.Name)
			keyPath := acme.IssuedKeyPath(certDir, c.Name)
			if _, err := os.Stat(certPath); err != nil {
				log.Debug().Str("cert", c.Name).Msg("ACME certificate not issued yet; skipping until issuance")
				continue
			}
			crt, err := loadKeyPair(certPath, keyPath, certDir)
			if err != nil {
				return nil, fmt.Errorf("certificate %q: %w", c.Name, err)
			}
			r.add(c.Domains, crt)
		}
	}
	return r, nil
}

func (r *certResolver) add(domains []string, crt *tls.Certificate) {
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if strings.HasPrefix(d, "*.") {
			r.wildcard[d[2:]] = crt
		} else {
			r.exact[d] = crt
		}
	}
}

// GetCertificate satisfies tls.Config.GetCertificate.
func (r *certResolver) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(strings.TrimSpace(hello.ServerName))
	if name == "" {
		return nil, fmt.Errorf("no SNI server name provided")
	}
	if crt, ok := r.exact[name]; ok {
		return crt, nil
	}
	// Wildcard: strip the left-most label and match the parent domain.
	if i := strings.IndexByte(name, '.'); i >= 0 {
		if crt, ok := r.wildcard[name[i+1:]]; ok {
			return crt, nil
		}
	}
	return nil, fmt.Errorf("no certificate for %q", name)
}

func loadKeyPair(certFile, keyFile, base string) (*tls.Certificate, error) {
	crt, err := tls.LoadX509KeyPair(resolvePath(certFile, base), resolvePath(keyFile, base))
	if err != nil {
		return nil, err
	}
	return &crt, nil
}

func resolvePath(p, base string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}
