package model

import (
	"strings"
	"testing"
)

// The reconciler indexes the ledger by the NORMALISED domain, so validation has
// to reject duplicates on that same form. "Foo.lan" and "foo.lan" both validating
// meant two claims for one name, of which exactly one silently shadowed the
// other - and the shadowed one could be the entry that recorded the real target.
func TestDNSLedgerDuplicateDomainIsCaseAndDotInsensitive(t *testing.T) {
	tests := [][2]string{
		{"Foo.lan", "foo.lan"},
		{"foo.lan.", "foo.lan"},
		{" foo.lan ", "foo.lan"},
	}
	for _, pair := range tests {
		l := DNSLedger{Pihole: []DNSLedgerEntry{
			{Domain: pair[0], Target: "edge.lan"},
			{Domain: pair[1], Target: "other.lan"},
		}}
		err := l.Validate()
		if err == nil {
			t.Fatalf("REGRESSION: %q and %q both validated; one would shadow the other", pair[0], pair[1])
		}
		if !strings.Contains(err.Error(), "duplicate domain") {
			t.Fatalf("err = %v, want a duplicate-domain rejection", err)
		}
	}
	// Genuinely different names still validate.
	ok := DNSLedger{Pihole: []DNSLedgerEntry{
		{Domain: "a.lan", Target: "edge.lan"},
		{Domain: "b.lan", Target: "edge.lan"},
	}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid ledger rejected: %v", err)
	}
}

// Provenance decides whether a record may ever be deleted, so the absent case has
// to read as the safe one: an entry from a ledger written before the field
// existed is treated as adopted, and adopted records are released, never deleted.
func TestDNSLedgerEntryProvenanceDefaultsToAdopted(t *testing.T) {
	if !(DNSLedgerEntry{Domain: "a.lan", Target: "edge.lan"}).IsAdopted() {
		t.Fatal("REGRESSION: an entry with no recorded provenance must be treated as adopted")
	}
	created, adopted := false, true
	if (DNSLedgerEntry{Adopted: &created}).IsAdopted() {
		t.Fatal("an entry recorded as created must not read as adopted")
	}
	if !(DNSLedgerEntry{Adopted: &adopted}).IsAdopted() {
		t.Fatal("an entry recorded as adopted must read as adopted")
	}
}

// The map/entry round trip is what the reconciler reads and writes on every run:
// it must preserve provenance, and it must write it EXPLICITLY so a claim gpm
// created is never re-read as the safe default.
func TestDNSLedgerRoundTripPreservesProvenance(t *testing.T) {
	in := map[string]DNSClaim{
		"created.lan": {Target: "edge.lan"},
		"adopted.lan": {Target: "edge.lan", Adopted: true},
	}
	entries := DNSLedgerEntries(in)
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	for _, e := range entries {
		if e.Adopted == nil {
			t.Fatalf("entry %q was written without explicit provenance", e.Domain)
		}
	}
	out := DNSLedgerMap(entries)
	if out["created.lan"].Adopted {
		t.Fatalf("created claim came back adopted: %+v", out)
	}
	if !out["adopted.lan"].Adopted {
		t.Fatalf("adopted claim came back as created: %+v", out)
	}

	// Equality is by value, not by pointer, so a round trip is not seen as a change
	// (which would commit a new ledger revision on every reconcile).
	if !DNSLedgerEntriesEqual(entries, DNSLedgerEntries(out)) {
		t.Fatal("a round trip must compare equal, or every reconcile rewrites the ledger")
	}
	// But a legacy entry and an explicit one are NOT equal: the file gets
	// normalised the first time the ledger is written after an upgrade.
	legacy := []DNSLedgerEntry{{Domain: "created.lan", Target: "edge.lan"}}
	explicit := DNSLedgerEntries(map[string]DNSClaim{"created.lan": {Target: "edge.lan", Adopted: true}})
	if DNSLedgerEntriesEqual(legacy, explicit) {
		t.Fatal("an entry with no provenance must not compare equal to an explicit one")
	}
}
