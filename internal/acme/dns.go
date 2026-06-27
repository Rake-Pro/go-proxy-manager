// Package acme issues and renews certificates from an ACME CA using the DNS-01
// challenge. It drives golang.org/x/crypto/acme directly (near-stdlib) and
// solves challenges through a pluggable DNSSolver; Cloudflare ships in P0c.
package acme

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// DNSSolver provisions and removes the TXT records that satisfy DNS-01
// challenges. Implementations are provider-specific (Cloudflare, etc.) and are
// selected per certificate via the referenced DNSProvider config object.
type DNSSolver interface {
	// Present creates a TXT record at the given FQDN name (e.g.
	// "_acme-challenge.example.com") with value. Multiple values may legitimately
	// coexist at the same name (apex + wildcard), so Present must add, not replace.
	Present(ctx context.Context, name, value string) error
	// CleanUp removes the TXT record matching name and value. It is best-effort
	// and must not fail issuance if the record is already gone.
	CleanUp(ctx context.Context, name, value string) error
}

// challengeRecord is one TXT record to provision for an order.
type challengeRecord struct {
	name  string // _acme-challenge.<domain>
	value string // DNS01ChallengeRecord output
}

// dnsName returns the DNS-01 record name for a certificate identifier domain.
// The leading "*." of a wildcard is stripped: *.example.com is validated at
// _acme-challenge.example.com, same as the apex.
func dnsName(domain string) string {
	if len(domain) > 2 && domain[0] == '*' && domain[1] == '.' {
		domain = domain[2:]
	}
	return "_acme-challenge." + domain
}

// txtLookuper resolves TXT records; *net.Resolver satisfies it. The seam keeps
// propagation logic unit-testable without real DNS.
type txtLookuper interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// waitPropagation blocks until every record's value is observable via DNS, or
// ctx/timeout elapses. Checking real DNS before asking the CA to validate avoids
// the classic "challenge accepted before the record propagated" failure.
func waitPropagation(ctx context.Context, records []challengeRecord, resolver txtLookuper, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if allPresent(ctx, records, resolver) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for DNS-01 TXT records to propagate", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func allPresent(ctx context.Context, records []challengeRecord, resolver txtLookuper) bool {
	// Group expected values by name so apex+wildcard sharing a name both resolve.
	want := map[string][]string{}
	for _, r := range records {
		want[r.name] = append(want[r.name], r.value)
	}
	for name, values := range want {
		got, err := resolver.LookupTXT(ctx, name)
		if err != nil {
			return false
		}
		sort.Strings(got)
		for _, v := range values {
			if !contains(got, v) {
				return false
			}
		}
	}
	return true
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
