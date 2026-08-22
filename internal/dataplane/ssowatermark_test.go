package dataplane

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// writeWatermark persists v as the on-disk revocation watermark in dir.
func writeWatermark(t *testing.T, dir string, v int64) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ssoNotBeforeFile), []byte(strconv.FormatInt(v, 10)), 0o600); err != nil {
		t.Fatalf("write watermark: %v", err)
	}
}

// A watermark written out of band (an HA peer on the shared volume, or an
// operator edit) is picked up by the refresh, without a restart.
func TestRefreshSSOWatermarkAdvancesFromDisk(t *testing.T) {
	dir := t.TempDir()
	SetSSOKeyDir(dir)
	t.Cleanup(func() { SetSSOKeyDir(""); ssoNotBefore.Store(0) })
	ssoNotBefore.Store(0)

	want := time.Now().Add(time.Hour).Unix()
	writeWatermark(t, dir, want)
	refreshSSOWatermark()
	if got := ssoRevokedAt(); got != want {
		t.Fatalf("watermark = %d, want %d", got, want)
	}
}

// The watermark is monotonic: a stale or clock-skewed file must never weaken a
// revocation already in force on this instance.
func TestRefreshSSOWatermarkNeverMovesBackwards(t *testing.T) {
	dir := t.TempDir()
	SetSSOKeyDir(dir)
	t.Cleanup(func() { SetSSOKeyDir(""); ssoNotBefore.Store(0) })

	high := time.Now().Add(2 * time.Hour).Unix()
	ssoNotBefore.Store(high)
	writeWatermark(t, dir, high-3600)
	refreshSSOWatermark()
	if got := ssoRevokedAt(); got != high {
		t.Fatalf("watermark moved backwards to %d, want %d", got, high)
	}

	// A malformed file is ignored for the same reason.
	if err := os.WriteFile(filepath.Join(dir, ssoNotBeforeFile), []byte("not-a-number"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	refreshSSOWatermark()
	if got := ssoRevokedAt(); got != high {
		t.Fatalf("malformed watermark changed the value to %d, want %d", got, high)
	}
}

// Without a state dir the refresh is a no-op rather than resetting anything.
func TestRefreshSSOWatermarkWithoutStateDir(t *testing.T) {
	SetSSOKeyDir("")
	t.Cleanup(func() { ssoNotBefore.Store(0) })
	v := time.Now().Add(3 * time.Hour).Unix()
	ssoNotBefore.Store(v)
	refreshSSOWatermark()
	if got := ssoRevokedAt(); got != v {
		t.Fatalf("watermark = %d, want %d", got, v)
	}
}

// The loop honors a revocation written after startup and exits with its context.
func TestWatchSSOWatermarkPicksUpLaterWrites(t *testing.T) {
	dir := t.TempDir()
	SetSSOKeyDir(dir)
	t.Cleanup(func() { SetSSOKeyDir(""); ssoNotBefore.Store(0) })
	ssoNotBefore.Store(0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { WatchSSOWatermark(ctx, 5*time.Millisecond); close(done) }()

	want := time.Now().Add(4 * time.Hour).Unix()
	writeWatermark(t, dir, want)

	deadline := time.After(2 * time.Second)
	for ssoRevokedAt() != want {
		select {
		case <-deadline:
			t.Fatalf("watermark still %d after 2s, want %d", ssoRevokedAt(), want)
		case <-time.After(2 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WatchSSOWatermark did not return after cancel")
	}
}
