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
//
// Adopted records HOW the claim was acquired, which decides whether gpm may ever
// delete the record: false means gpm created the record itself (so removing it
// only undoes gpm's own write), true means the record already existed and gpm
// merely claimed it (so removing it would destroy something an operator made).
// gpm deletes only what it created; an adopted record that is no longer wanted is
// RELEASED from the ledger, never deleted.
//
// It is a *bool rather than a bool so that "the field is absent" is a third,
// distinguishable state. Ledgers written before this field existed carry no
// provenance at all, and the two possible readings are not equally safe: reading
// them as "created" would let an upgrade delete records gpm had adopted (exactly
// the incident this whole subsystem exists to prevent), while reading them as
// "adopted" can only ever leave a record in place. Absent therefore means
// ADOPTED - see IsAdopted.
type DNSLedgerEntry struct {
	Domain  string `json:"domain" yaml:"domain"`
	Target  string `json:"target" yaml:"target"`
	Adopted *bool  `json:"adopted" yaml:"adopted"`
}

// IsAdopted reports whether the record must be treated as one gpm adopted rather
// than created, and so must never be deleted. An entry with no recorded
// provenance (a ledger written before the field existed) reads as adopted: it is
// the only reading of a missing field that cannot destroy an operator's record.
func (e DNSLedgerEntry) IsAdopted() bool { return e.Adopted == nil || *e.Adopted }

// DNSClaim is one ledger entry as the reconciler works with it: the target gpm
// recorded for the name, and whether the claim came from adoption.
type DNSClaim struct {
	Target  string
	Adopted bool
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
			// Keyed on the normalised form the reconciler indexes by (DNSLedgerMap),
			// so "Foo.lan" and "foo.lan" cannot both validate and then have one
			// silently shadow the other.
			key := normaliseDomain(e.Domain)
			if _, dup := seen[key]; dup {
				return fmt.Errorf("dns ledger: %s: duplicate domain %q", b.name, e.Domain)
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

// normaliseDomain is the single form a ledger domain (or target) is compared and
// indexed by: lowercased, trimmed, no trailing dot.
func normaliseDomain(s string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
}

// DNSLedgerMap indexes entries as domain -> claim for lookup during a reconcile.
func DNSLedgerMap(entries []DNSLedgerEntry) map[string]DNSClaim {
	out := make(map[string]DNSClaim, len(entries))
	for _, e := range entries {
		out[normaliseDomain(e.Domain)] = DNSClaim{Target: normaliseDomain(e.Target), Adopted: e.IsAdopted()}
	}
	return out
}

// DNSLedgerEntries turns a domain -> claim map back into a sorted entry list, so
// the committed YAML is stable and a reconcile that changed nothing produces no
// diff (and therefore no commit). Provenance is always written explicitly, so an
// entry written by this version is never read back as the safe-default "adopted".
func DNSLedgerEntries(m map[string]DNSClaim) []DNSLedgerEntry {
	if len(m) == 0 {
		return nil
	}
	out := make([]DNSLedgerEntry, 0, len(m))
	for d, c := range m {
		adopted := c.Adopted
		out = append(out, DNSLedgerEntry{Domain: d, Target: c.Target, Adopted: &adopted})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}

// DNSLedgerEntriesEqual compares two entry lists by value. It exists because the
// entries hold a pointer field: slices.Equal would compare the pointers, so a
// ledger that is byte-identical after a round trip would look changed (or, worse,
// a legacy entry with no provenance would look identical to an explicit one and
// never be normalised).
func DNSLedgerEntriesEqual(a, b []DNSLedgerEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Domain != b[i].Domain || a[i].Target != b[i].Target {
			return false
		}
		if (a[i].Adopted == nil) != (b[i].Adopted == nil) {
			return false
		}
		if a[i].Adopted != nil && *a[i].Adopted != *b[i].Adopted {
			return false
		}
	}
	return true
}
