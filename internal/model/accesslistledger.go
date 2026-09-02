package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// AccessListSourceEntry is one fetched remote IP feed: which source it came
// from, where it was fetched from, when, and the exact network set that fetch
// produced.
//
// URL is recorded next to the entries for the same reason the DNS ledger records
// a CNAME target rather than only a name: it is how a later run can tell a set it
// fetched from the feed the config still names from one left over after the
// operator re-pointed the source somewhere else.
//
// SHA256 binds the entry set to the identity it was fetched FOR: it is taken over
// the ledger key, the URL and the normalised entries together (see
// AccessListSourceHash). Hashing the entries alone would let a set be relabelled
// - a block of networks legitimately fetched for one list's source pasted under
// another list's key, or under a different URL, with the recorded digest still
// verifying. It is validated on load, so a hand-edited or relabelled entry list
// is surfaced instead of being served.
type AccessListSourceEntry struct {
	List   string `json:"list" yaml:"list"`
	Source string `json:"source" yaml:"source"`
	URL    string `json:"url" yaml:"url"`
	// FetchedAt is RFC3339. It is a string rather than a time.Time so the
	// committed YAML is byte-stable across Go versions and marshallers.
	FetchedAt string `json:"fetchedAt" yaml:"fetchedAt"`
	SHA256    string `json:"sha256" yaml:"sha256"`
	// Entries are CIDRs in masked form, sorted and deduplicated.
	Entries []string `json:"entries" yaml:"entries"`
}

// Key is the ledger identity of an entry: "<list>/<source>".
func (e AccessListSourceEntry) Key() string { return AccessListSourceKey(e.List, e.Source) }

// AccessListSourceKey is the ledger key for one source of one access list.
func AccessListSourceKey(list, source string) string { return list + "/" + source }

// Fetched parses FetchedAt, returning the zero time when it is absent or
// unparseable - which the syncer reads as "never fetched", i.e. due now.
func (e AccessListSourceEntry) Fetched() time.Time {
	t, err := time.Parse(time.RFC3339, e.FetchedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// AccessListSourceLedger is the singleton record of what each access-list source
// last resolved to, stored as config/access-list-sources.yaml.
//
// It is a committed file rather than in-memory state for the same reasons the
// DNS ledger is: the sets decide who reaches a host, so they must be diffable,
// auditable and revertable with the rest of the config, and a restart must not
// silently drop every allow-list to empty until the next fetch completes. It is
// separate from the AccessList objects themselves so a routine 24-hourly
// re-fetch never rewrites a file the operator owns.
//
// It is written by the fetcher alone (internal/accesssync); there is no CRUD
// route over it, since a generic PUT would be an "add anything you like to an
// allow list" primitive.
type AccessListSourceLedger struct {
	SchemaVersion int                     `json:"schemaVersion" yaml:"schemaVersion"`
	Sources       []AccessListSourceEntry `json:"sources,omitempty" yaml:"sources,omitempty"`
}

func (l AccessListSourceLedger) Kind() string { return "AccessListSourceLedger" }

// Validate rejects a ledger the fetcher could not have written, so a corrupted
// or hand-edited file is surfaced rather than acted on. The entry hash is part of
// that check: an operator who pastes a network into the list without recomputing
// the hash gets a load error, not a silently widened allow list.
func (l AccessListSourceLedger) Validate() error {
	seen := map[string]struct{}{}
	for i, e := range l.Sources {
		if strings.TrimSpace(e.List) == "" {
			return fmt.Errorf("access-list source ledger: sources[%d]: list is required", i)
		}
		if strings.TrimSpace(e.Source) == "" {
			return fmt.Errorf("access-list source ledger: sources[%d] (%s): source is required", i, e.List)
		}
		if _, dup := seen[e.Key()]; dup {
			return fmt.Errorf("access-list source ledger: duplicate entry %q", e.Key())
		}
		seen[e.Key()] = struct{}{}
		if e.FetchedAt != "" {
			if _, err := time.Parse(time.RFC3339, e.FetchedAt); err != nil {
				return fmt.Errorf("access-list source ledger: %s: fetchedAt %q is not RFC3339", e.Key(), e.FetchedAt)
			}
		}
		for _, c := range e.Entries {
			if _, _, err := net.ParseCIDR(c); err != nil {
				return fmt.Errorf("access-list source ledger: %s: entry %q is not a CIDR", e.Key(), c)
			}
		}
		if !sort.StringsAreSorted(e.Entries) {
			return fmt.Errorf("access-list source ledger: %s: entries are not sorted", e.Key())
		}
		if got := AccessListSourceHash(e.Key(), e.URL, e.Entries); got != e.SHA256 {
			return fmt.Errorf("access-list source ledger: %s: sha256 %q does not match its list/source, url and entries (%q)", e.Key(), e.SHA256, got)
		}
	}
	return nil
}

// AccessListSourceHash is the sha256, hex-encoded, of the entry's full identity:
// its ledger key ("<list>/<source>"), the URL it was fetched from, and the
// normalised entry set - the CIDRs joined by newlines, in the sorted order the
// ledger stores them. Each field is length-prefixed so no combination of values
// can produce the same byte stream as a different one.
//
// The key and URL are in the digest so a fetched set is bound to the source it
// belongs to: moving a block of networks under a different list, source name or
// URL invalidates the hash and fails the load, rather than quietly granting one
// host's feed the access of another's.
func AccessListSourceHash(key, url string, entries []string) string {
	h := sha256.New()
	write := func(s string) {
		fmt.Fprintf(h, "%d:", len(s))
		h.Write([]byte(s))
		h.Write([]byte("\n"))
	}
	write(key)
	write(url)
	for _, e := range entries {
		write(e)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AccessListSourceMap indexes a ledger's entries by "<list>/<source>".
func (l AccessListSourceLedger) AccessListSourceMap() map[string]AccessListSourceEntry {
	out := make(map[string]AccessListSourceEntry, len(l.Sources))
	for _, e := range l.Sources {
		out[e.Key()] = e
	}
	return out
}

// AccessListSourceEntries turns a key -> entry map back into the sorted slice the
// ledger stores, so a reconcile that changed nothing produces no diff and
// therefore no commit.
func AccessListSourceEntries(m map[string]AccessListSourceEntry) []AccessListSourceEntry {
	if len(m) == 0 {
		return nil
	}
	out := make([]AccessListSourceEntry, 0, len(m))
	for _, e := range m {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}
