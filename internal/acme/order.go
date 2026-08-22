package acme

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/acme"
)

// issue runs a full order (dns-01 or http-01) for cert's domains and persists
// the result. For dns-01 solver provisions the TXT records; for http-01 the
// key authorizations are parked in the manager's token store, which the data
// plane's plaintext listener serves.
func (m *Manager) issue(ctx context.Context, client *acme.Client, cert model.Certificate, solver DNSSolver) error {
	if kt := cert.ACME.KeyType; kt != "" && kt != "ecdsa" {
		return fmt.Errorf("certificate %q: key type %q not supported (P0c issues ecdsa P-256)", cert.Name, kt)
	}
	domains := cert.Domains
	chalType := cert.ACME.EffectiveChallenge()
	if chalType == model.ChallengeDNS01 && solver == nil {
		return fmt.Errorf("certificate %q: dns-01 needs a dns provider solver", cert.Name)
	}

	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domains...))
	if err != nil {
		return fmt.Errorf("authorize order: %w", err)
	}

	type pending struct {
		chal     *acme.Challenge
		authzURL string
	}
	var (
		records    []challengeRecord
		httpTokens []string
		todo       []pending
	)
	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return fmt.Errorf("get authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue // already authorized
		}
		chal := findChallenge(authz, chalType)
		if chal == nil {
			return fmt.Errorf("no %s challenge offered for %q", chalType, authz.Identifier.Value)
		}
		if chalType == model.ChallengeHTTP01 {
			keyAuth, err := client.HTTP01ChallengeResponse(chal.Token)
			if err != nil {
				m.http01.Delete(httpTokens...)
				return fmt.Errorf("compute http-01 response: %w", err)
			}
			m.http01.Put(chal.Token, keyAuth, http01TokenTTL)
			httpTokens = append(httpTokens, chal.Token)
		} else {
			val, err := client.DNS01ChallengeRecord(chal.Token)
			if err != nil {
				return fmt.Errorf("compute dns-01 record: %w", err)
			}
			records = append(records, challengeRecord{name: dnsName(authz.Identifier.Value), value: val})
		}
		todo = append(todo, pending{chal: chal, authzURL: authzURL})
	}

	if chalType == model.ChallengeHTTP01 {
		// The tokens are only servable while this order is in flight.
		defer m.http01.Delete(httpTokens...)
	} else {
		// Provision every TXT record first (apex + wildcard may share a name), then
		// clean them all up once the order resolves.
		for _, r := range records {
			if err := solver.Present(ctx, r.name, r.value); err != nil {
				m.cleanup(ctx, solver, records)
				return fmt.Errorf("present TXT %s: %w", r.name, err)
			}
		}
		defer m.cleanup(ctx, solver, records)

		if err := waitPropagation(ctx, records, m.resolver, m.propagationTimeout, m.propagationInterval); err != nil {
			return err
		}
	}

	for _, p := range todo {
		if _, err := client.Accept(ctx, p.chal); err != nil {
			return fmt.Errorf("accept challenge: %w", err)
		}
	}
	for _, p := range todo {
		if _, err := client.WaitAuthorization(ctx, p.authzURL); err != nil {
			return fmt.Errorf("authorization failed: %w", err)
		}
	}

	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}, certKey)
	if err != nil {
		return fmt.Errorf("create CSR: %w", err)
	}

	chainDER, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		return fmt.Errorf("finalize order: %w", err)
	}

	chainPEM := encodeChain(chainDER)
	notAfter, err := leafNotAfter(chainPEM)
	if err != nil {
		return err
	}
	meta := Meta{
		Domains:      domains,
		DirectoryURL: client.DirectoryURL,
		IssuedAt:     time.Now().UTC(),
		NotAfter:     notAfter,
	}
	if err := saveIssued(m.certDir, cert.Name, chainPEM, certKey, meta); err != nil {
		return fmt.Errorf("persist certificate: %w", err)
	}
	log.Info().Str("cert", cert.Name).Time("notAfter", notAfter).Strs("domains", domains).Msg("certificate issued")
	return nil
}

func (m *Manager) cleanup(ctx context.Context, solver DNSSolver, records []challengeRecord) {
	for _, r := range records {
		if err := solver.CleanUp(ctx, r.name, r.value); err != nil {
			log.Warn().Str("record", r.name).Err(err).Msg("failed to clean up DNS-01 TXT record")
		}
	}
}

func findChallenge(authz *acme.Authorization, typ string) *acme.Challenge {
	for _, c := range authz.Challenges {
		if c.Type == typ {
			return c
		}
	}
	return nil
}

func encodeChain(chainDER [][]byte) []byte {
	var buf bytes.Buffer
	for _, der := range chainDER {
		_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	return buf.Bytes()
}
