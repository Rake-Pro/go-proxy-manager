package model

import (
	"fmt"
	"sort"
	"strings"
)

// DNSLedgerEntry is one DNS record gpm owns: the name it published and the CNAME
// target it published it with. The target is recorded, not merely the name, so a
// later reconcile can tell a record it created and nobody has touched from one an
// operator has since re-pointed - the first is safe to retarget or remove, the
// second is not gpm's to touch.
type DNSLedgerEntry struct {
	Domain string `json:"domain" yaml:"domain"`
	Target string `json:"target" yaml:"target"`
}

// DNSLedger is the singleton record-ownership ledger, stored as
// config/dns-ledger.yaml. It is the AUTHORITATIVE answer to "did gpm create this
// DNS record?", and therefore the only thing that can authorise a delete.
//
// It exists because the Pi-hole/dnsmasq CNAME format has no comment field, so the
// original backend used "the CNAME target equals apexTarget" as a stand-in for
// ownership. On a shared apex that is not ownership at all: on 2026-08-01 the
// first reconcile after enabling the Pi-hole backend deleted 19 hand-written LAN
// CNAMEs that happened to point at the same edge host, because the desired set
// was still empty. Ownership is now recorded explicitly instead of inferred, and
// a record that is not in this ledger is never deleted, whatever it points at.
//
// It is a singleton file rather than a CRUD object kind on purpose: it is
// instance-wide state with exactly one instance (like Settings), it must be
// written by the reconciler alone (a generic PUT/DELETE over it would be an
// arbitrary "authorise a DNS deletion" primitive), and one file per record would
// turn a routine reconcile into dozens of git blobs. It lives in the config repo
// so it is committed, diffable, auditable and reverted with everything else.
type DNSLedger struct {
	SchemaVersion int `json:"schemaVersion" yaml:"schemaVersion"`
	// Pihole are the LAN CNAMEs gpm created or adopted on the Pi-hole backend.
	Pihole []DNSLedgerEntry `json:"pihole,omitempty" yaml:"pihole,omitempty"`
	// Cloudflare are the public CNAMEs gpm created or adopted in the zone.
	Cloudflare []DNSLedgerEntry `json:"cloudflare,omitempty" yaml:"cloudflare,omitempty"`
}

func (l DNSLedger) Kind() string { return "DNSLedger" }

// Validate rejects a ledger that could not have been written by the reconciler,
// so a hand-edited or corrupted file is surfaced instead of being acted on.
func (l DNSLedger) Validate() error {
	for _, b := range []struct {
		name    string
		entries []DNSLedgerEntry
	}{{"pihole", l.Pihole}, {"cloudflare", l.Cloudflare}} {
		seen := map[string]struct{}{}
		for i, e := range b.entries {
			if strings.TrimSpace(e.Domain) == "" {
				return fmt.Errorf("dns ledger: %s[%d]: domain is required", b.name, i)
			}
			if strings.TrimSpace(e.Target) == "" {
				return fmt.Errorf("dns ledger: %s[%d] (%s): target is required", b.name, i, e.Domain)
			}
			if _, dup := seen[e.Domain]; dup {
				return fmt.Errorf("dns ledger: %s: duplicate domain %q", b.name, e.Domain)
			}
			seen[e.Domain] = struct{}{}
		}
	}
	return nil
}

// DNSLedgerMap indexes entries as domain -> target for lookup during a reconcile.
func DNSLedgerMap(entries []DNSLedgerEntry) map[string]string {
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[strings.ToLower(strings.TrimSuffix(e.Domain, "."))] = strings.ToLower(strings.TrimSuffix(e.Target, "."))
	}
	return out
}

// DNSLedgerEntries turns a domain -> target map back into a sorted entry list, so
// the committed YAML is stable and a reconcile that changed nothing produces no
// diff (and therefore no commit).
func DNSLedgerEntries(m map[string]string) []DNSLedgerEntry {
	if len(m) == 0 {
		return nil
	}
	out := make([]DNSLedgerEntry, 0, len(m))
	for d, t := range m {
		out = append(out, DNSLedgerEntry{Domain: d, Target: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}
