package model

import (
	"strings"
	"testing"
)

// The rule shape is the security boundary: a rule that draws its networks from
// nowhere (or from two places at once) and a path that could never match the
// cleaned request path are both refused at write time rather than becoming a
// gate that silently never fires.
func TestAccessListSourceAndPathRuleValidation(t *testing.T) {
	src := []AccessListSource{{Name: "uptimerobot", URL: "https://example.com/ips.txt"}}

	tests := []struct {
		name    string
		al      AccessList
		wantErr string
	}{
		{
			name: "plain cidr rule unchanged",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "lan"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8"}},
			},
		},
		{
			name: "source rule with paths and methods",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "home-vpn"},
				Sources:    src,
				Rules: []IPRule{
					{Action: ActionAllow, CIDR: "10.0.0.0/8"},
					{Action: ActionAllow, Source: "uptimerobot", Paths: []string{"/api/health", "/-/healthy"}, Methods: []string{"GET", "HEAD"}},
				},
			},
		},
		{
			name: "paths without methods defaults, still valid",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources:    src,
				Rules:      []IPRule{{Action: ActionAllow, Source: "uptimerobot", Paths: []string{"/status.php"}}},
			},
		},
		{
			name: "both cidr and source",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources:    src,
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Source: "uptimerobot"}},
			},
			wantErr: "use exactly one",
		},
		{
			name: "neither cidr nor source",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow}},
			},
			wantErr: "must set either cidr or source",
		},
		{
			name: "source not declared",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, Source: "nope"}},
			},
			wantErr: "not declared in this list's sources",
		},
		{
			name: "relative path",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"health"}}},
			},
			wantErr: `must start with "/"`,
		},
		{
			name: "dot segment path",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"/api/../health"}}},
			},
			wantErr: "is not clean",
		},
		{
			name: "trailing slash is not clean",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"/health/"}}},
			},
			wantErr: "is not clean",
		},
		{
			name: "query string in path",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"/health?x=1"}}},
			},
			wantErr: "no query string",
		},
		{
			name: "duplicate path",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"/health", "/health"}}},
			},
			wantErr: "duplicate rule path",
		},
		{
			name: "lowercase method",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"/health"}, Methods: []string{"get"}}},
			},
			wantErr: "not an upper-case standard HTTP method",
		},
		{
			name: "unknown method",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"/health"}, Methods: []string{"FETCH"}}},
			},
			wantErr: "not an upper-case standard HTTP method",
		},
		{
			name: "deny with paths is refused",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionDeny, CIDR: "203.0.113.0/24", Paths: []string{"/admin"}}},
			},
			wantErr: "paths are allow-only",
		},
		{
			name: "methods without paths",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Methods: []string{"GET"}}},
			},
			wantErr: "methods only scope a path rule",
		},
		{
			name: "source name shape",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources:    []AccessListSource{{Name: "Uptime Robot", URL: "https://example.com/i.txt"}},
			},
			wantErr: "invalid name",
		},
		{
			name: "duplicate source name",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources: []AccessListSource{
					{Name: "a", URL: "https://example.com/i.txt"},
					{Name: "a", URL: "https://example.com/j.txt"},
				},
			},
			wantErr: "duplicate source",
		},
		{
			name: "http source url refused",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources:    []AccessListSource{{Name: "a", URL: "http://example.com/i.txt"}},
			},
			wantErr: "absolute https URL",
		},
		{
			name: "relative source url refused",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources:    []AccessListSource{{Name: "a", URL: "/ips.txt"}},
			},
			wantErr: "absolute https URL",
		},
		{
			name: "unparseable interval",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources:    []AccessListSource{{Name: "a", URL: "https://example.com/i.txt", Interval: "daily"}},
			},
			wantErr: "must be a Go duration",
		},
		{
			name: "interval below the floor",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources:    []AccessListSource{{Name: "a", URL: "https://example.com/i.txt", Interval: "30s"}},
			},
			wantErr: "must be at least 1h0m0s",
		},
		{
			name: "negative maxEntries",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "l"},
				Sources:    []AccessListSource{{Name: "a", URL: "https://example.com/i.txt", MaxEntries: -1}},
			},
			wantErr: "must not be negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.al.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestAccessListSourceDefaults(t *testing.T) {
	var s AccessListSource
	if got := s.FetchInterval(); got != DefaultAccessListSourceInterval {
		t.Fatalf("default interval = %s", got)
	}
	if got := s.EntryLimit(); got != DefaultAccessListSourceMaxEntries {
		t.Fatalf("default maxEntries = %d", got)
	}
	s = AccessListSource{Interval: "6h", MaxEntries: 5}
	if got := s.FetchInterval(); got.Hours() != 6 {
		t.Fatalf("interval = %s", got)
	}
	if got := s.EntryLimit(); got != 5 {
		t.Fatalf("maxEntries = %d", got)
	}
}

// An unset methods list on a path rule means the read-only pair a health probe
// needs, never "every method".
func TestIPRuleEffectiveMethods(t *testing.T) {
	if got := (IPRule{CIDR: "10.0.0.0/8"}).EffectiveMethods(); got != nil {
		t.Fatalf("a rule without paths has no method scope, got %v", got)
	}
	got := IPRule{CIDR: "10.0.0.0/8", Paths: []string{"/h"}}.EffectiveMethods()
	if len(got) != 2 || got[0] != "GET" || got[1] != "HEAD" {
		t.Fatalf("default methods = %v, want GET and HEAD", got)
	}
	got = IPRule{CIDR: "10.0.0.0/8", Paths: []string{"/h"}, Methods: []string{"POST"}}.EffectiveMethods()
	if len(got) != 1 || got[0] != "POST" {
		t.Fatalf("methods = %v", got)
	}
}

// A raw stream carries no request path and resolves no ledger, so a list with
// path-scoped or source-backed rules is refused for a StreamHost exactly the way
// a basicAuth list already is - never silently evaluated as half a gate.
func TestStreamHostRefusesRequestScopedAccessLists(t *testing.T) {
	base := func(al AccessList) Config {
		return Config{
			AccessLists: []AccessList{al},
			StreamHosts: []StreamHost{{
				ObjectMeta:  ObjectMeta{Name: "db"},
				Protocol:    "tcp",
				ListenPort:  5432,
				Target:      StreamTarget{Host: "10.0.0.5", Port: 5432},
				AccessLists: []string{al.Name},
			}},
		}
	}
	tests := []struct {
		name    string
		al      AccessList
		wantErr string
	}{
		{
			name: "literal cidr rules are still fine at L4",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "lan"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8"}},
			},
		},
		{
			name: "path-scoped rule refused",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "lan"},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"/health"}}},
			},
			wantErr: "path-scoped and/or source-backed rules",
		},
		{
			name: "source-backed rule refused",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "lan"},
				Sources:    []AccessListSource{{Name: "up", URL: "https://example.com/i.txt"}},
				Rules:      []IPRule{{Action: ActionAllow, Source: "up"}},
			},
			wantErr: "path-scoped and/or source-backed rules",
		},
		{
			name: "a declared source alone is enough to refuse",
			al: AccessList{
				ObjectMeta: ObjectMeta{Name: "lan"},
				Sources:    []AccessListSource{{Name: "up", URL: "https://example.com/i.txt"}},
				Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8"}},
			},
			wantErr: "path-scoped and/or source-backed rules",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := base(tt.al).Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestAccessListSourceLedgerValidate(t *testing.T) {
	good := []string{"1.2.3.0/24", "2001:db8::/32"}
	tests := []struct {
		name    string
		ledger  AccessListSourceLedger
		wantErr string
	}{
		{
			name: "empty ledger",
		},
		{
			name: "well-formed entry",
			ledger: AccessListSourceLedger{Sources: []AccessListSourceEntry{{
				List: "home-vpn", Source: "uptimerobot", URL: "https://example.com/i.txt",
				FetchedAt: "2026-09-01T00:00:00Z", SHA256: AccessListSourceHash("home-vpn/uptimerobot", "https://example.com/i.txt", good), Entries: good,
			}}},
		},
		{
			name: "missing list",
			ledger: AccessListSourceLedger{Sources: []AccessListSourceEntry{{
				Source: "s", SHA256: AccessListSourceHash("/s", "", nil),
			}}},
			wantErr: "list is required",
		},
		{
			name: "duplicate key",
			ledger: AccessListSourceLedger{Sources: []AccessListSourceEntry{
				{List: "l", Source: "s", SHA256: AccessListSourceHash("l/s", "", nil)},
				{List: "l", Source: "s", SHA256: AccessListSourceHash("l/s", "", nil)},
			}},
			wantErr: "duplicate entry",
		},
		{
			name: "non-RFC3339 fetchedAt",
			ledger: AccessListSourceLedger{Sources: []AccessListSourceEntry{{
				List: "l", Source: "s", FetchedAt: "yesterday", SHA256: AccessListSourceHash("l/s", "", nil),
			}}},
			wantErr: "not RFC3339",
		},
		{
			name: "entry is not a CIDR",
			ledger: AccessListSourceLedger{Sources: []AccessListSourceEntry{{
				List: "l", Source: "s", Entries: []string{"1.2.3.4"}, SHA256: AccessListSourceHash("l/s", "", []string{"1.2.3.4"}),
			}}},
			wantErr: "is not a CIDR",
		},
		{
			name: "entries not sorted",
			ledger: AccessListSourceLedger{Sources: []AccessListSourceEntry{{
				List: "l", Source: "s", Entries: []string{"9.0.0.0/8", "1.0.0.0/8"},
				SHA256: AccessListSourceHash("l/s", "", []string{"9.0.0.0/8", "1.0.0.0/8"}),
			}}},
			wantErr: "not sorted",
		},
		{
			// A hand-pasted network without a recomputed hash is exactly the edit
			// that would silently widen an allow list.
			name: "hash does not match entries",
			ledger: AccessListSourceLedger{Sources: []AccessListSourceEntry{{
				List: "l", Source: "s", Entries: good, SHA256: AccessListSourceHash("l/s", "", []string{"1.2.3.0/24"}),
			}}},
			wantErr: "does not match its list/source, url and entries",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ledger.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// The digest binds a fetched set to the list/source and URL it was fetched for,
// so a block of networks cannot be relabelled onto another list (or another
// feed's URL) with its recorded hash still verifying.
func TestAccessListSourceHashBindsIdentity(t *testing.T) {
	entries := []string{"203.0.113.0/24"}
	base := AccessListSourceHash("home-vpn/uptimerobot", "https://example.com/i.txt", entries)
	for _, other := range []struct{ key, url string }{
		{"public/uptimerobot", "https://example.com/i.txt"},
		{"home-vpn/other", "https://example.com/i.txt"},
		{"home-vpn/uptimerobot", "https://evil.example/i.txt"},
	} {
		if AccessListSourceHash(other.key, other.url, entries) == base {
			t.Fatalf("hash must change with %s / %s", other.key, other.url)
		}
	}
	// Length prefixing: no field boundary can be shifted to forge a collision.
	if AccessListSourceHash("a/bc", "d", entries) == AccessListSourceHash("a/b", "cd", entries) {
		t.Fatal("field boundaries must not be ambiguous")
	}
	// A relabelled entry fails the ledger's own validation, not just this helper.
	l := AccessListSourceLedger{Sources: []AccessListSourceEntry{{
		List: "public", Source: "uptimerobot", URL: "https://example.com/i.txt",
		SHA256: base, Entries: entries,
	}}}
	if err := l.Validate(); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v, want a relabelled set to fail validation", err)
	}
}

func TestAccessListSourceEntriesRoundTrip(t *testing.T) {
	m := map[string]AccessListSourceEntry{
		"b/two": {List: "b", Source: "two"},
		"a/one": {List: "a", Source: "one"},
	}
	got := AccessListSourceEntries(m)
	if len(got) != 2 || got[0].Key() != "a/one" || got[1].Key() != "b/two" {
		t.Fatalf("entries = %+v, want sorted by key", got)
	}
	back := AccessListSourceLedger{Sources: got}.AccessListSourceMap()
	if len(back) != 2 || back["b/two"].Source != "two" {
		t.Fatalf("map = %+v", back)
	}
	if AccessListSourceEntries(nil) != nil {
		t.Fatal("an empty map must produce a nil slice so the committed YAML stays clean")
	}
}

func TestAccessListSyncSettingsValidate(t *testing.T) {
	if !(AccessListSyncSettings{}).IsEnabled() {
		t.Fatal("an unset accessListSync block must default to enabled")
	}
	off := false
	if (AccessListSyncSettings{Enabled: &off}).IsEnabled() {
		t.Fatal("an explicit false must disable the fetcher")
	}
	if got := (AccessListSyncSettings{}).Poll(); got != DefaultAccessListSyncPollInterval {
		t.Fatalf("default poll = %s", got)
	}
	if err := (AccessListSyncSettings{PollInterval: "5s"}).Validate(); err == nil ||
		!strings.Contains(err.Error(), "at least 1m0s") {
		t.Fatalf("err = %v, want a floor error", err)
	}
	if err := (AccessListSyncSettings{PollInterval: "often"}).Validate(); err == nil ||
		!strings.Contains(err.Error(), "Go duration") {
		t.Fatalf("err = %v", err)
	}
	if err := (AccessListSyncSettings{PollInterval: "30m"}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
