package geoip

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// testDBPath returns the bundled MaxMind test fixture (see testdata/README.md
// for its origin), skipping the test if it is somehow absent so the rule-
// matching/fail-closed logic tests elsewhere never depend on it.
func testDBPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("testdata", "GeoIP2-Country-Test.mmdb")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("test fixture not available: %v", err)
	}
	return path
}

func TestOpenMissingFile(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "does-not-exist.mmdb")); err == nil {
		t.Fatal("expected error opening a missing database file")
	}
}

func TestResolverZeroValue(t *testing.T) {
	var res *Resolver
	if res.Loaded() {
		t.Fatal("nil resolver must report Loaded() == false")
	}
	if cc, ok := res.Country(net.ParseIP("8.8.8.8")); ok || cc != "" {
		t.Fatalf("nil resolver must report not-found, got (%q, %v)", cc, ok)
	}
	if err := res.Close(); err != nil {
		t.Fatalf("nil resolver Close must be a no-op, got %v", err)
	}

	res = &Resolver{}
	if res.Loaded() {
		t.Fatal("unloaded resolver must report Loaded() == false")
	}
	if cc, ok := res.Country(net.ParseIP("8.8.8.8")); ok || cc != "" {
		t.Fatalf("unloaded resolver must report not-found, got (%q, %v)", cc, ok)
	}
}

func TestCountryLookups(t *testing.T) {
	path := testDBPath(t)
	res, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = res.Close() })

	if !res.Loaded() {
		t.Fatal("expected Loaded() == true after a successful Open")
	}

	cases := []struct {
		ip      string
		want    string
		wantOK  bool
		comment string
	}{
		{"81.2.69.142", "GB", true, "known GB sample IP"},
		{"89.160.20.128", "SE", true, "known SE sample IP"},
		{"216.160.83.56", "US", true, "known US sample IP"},
		{"192.168.1.1", "", false, "private IP has no country"},
		{"::1", "", false, "loopback has no country"},
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			cc, ok := res.Country(net.ParseIP(c.ip))
			if ok != c.wantOK || cc != c.want {
				t.Fatalf("%s (%s): got (%q, %v), want (%q, %v)", c.ip, c.comment, cc, ok, c.want, c.wantOK)
			}
		})
	}
}

func TestReloadSwapsOnSuccessKeepsLastGoodOnFailure(t *testing.T) {
	path := testDBPath(t)
	res, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = res.Close() })

	if err := res.Reload(path); err != nil {
		t.Fatalf("Reload with the same valid path: %v", err)
	}
	if !res.Loaded() {
		t.Fatal("expected Loaded() == true after a successful Reload")
	}

	// A failing reload (missing file) must not clear the previously loaded DB.
	if err := res.Reload(filepath.Join(t.TempDir(), "missing.mmdb")); err == nil {
		t.Fatal("expected an error reloading a missing file")
	}
	if !res.Loaded() {
		t.Fatal("a failed Reload must keep the last-known-good database loaded")
	}
	if cc, ok := res.Country(net.ParseIP("81.2.69.142")); !ok || cc != "GB" {
		t.Fatalf("last-known-good database should still answer lookups, got (%q, %v)", cc, ok)
	}
}

// TestReloadDoesNotUseAfterFreeConcurrentLookups guards against a
// use-after-free: Reload must not synchronously Close() the reader it just
// swapped out, because a concurrent Country call may have already loaded
// that reader and be mid-Lookup - maxminddb.Reader.Close() munmaps the
// backing buffer immediately, which would crash or corrupt that lookup. Run
// with -race to catch a reintroduced eager Close.
func TestReloadDoesNotUseAfterFreeConcurrentLookups(t *testing.T) {
	path := testDBPath(t)
	res, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = res.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				res.Country(net.ParseIP("81.2.69.142"))
			}
		}()
	}

	for i := 0; i < 50; i++ {
		if err := res.Reload(path); err != nil {
			t.Fatalf("Reload: %v", err)
		}
	}

	cancel()
	wg.Wait()
}

func TestWatchPicksUpFileChange(t *testing.T) {
	path := testDBPath(t)
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	watched := filepath.Join(t.TempDir(), "watched.mmdb")
	if err := os.WriteFile(watched, orig, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	res, err := Open(watched)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = res.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go res.Watch(ctx, watched, 10*time.Millisecond)

	// Rewrite the file (same bytes is fine - Watch only needs a newer mtime)
	// with an explicit future mtime so the poll reliably observes a change
	// even on filesystems with coarse mtime resolution.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(watched, orig, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(watched, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if res.Loaded() {
			if cc, ok := res.Country(net.ParseIP("81.2.69.142")); ok && cc == "GB" {
				return // reload observed and the reloaded DB answers lookups
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("Watch did not pick up the file change within the deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
