// Package geoip wraps a MaxMind .mmdb reader behind an atomically-swappable
// handle, so AccessList geo rules can resolve a client IP to an ISO-3166-1
// alpha-2 country code without blocking concurrent requests during a reload.
//
// No database ships with gpm: GeoLite2's licence forbids redistribution, so
// the operator supplies the .mmdb path (GPM_GEOIP_DB) and refreshes it
// themselves (e.g. via geoipupdate). A Resolver with nothing loaded is the
// normal "feature disabled" state - see docs/design/http3-geoip-mtls.md.
package geoip

import (
	"context"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"time"

	maxminddb "github.com/oschwald/maxminddb-golang/v2"
	"github.com/rs/zerolog/log"
)

// DefaultWatchInterval is the recommended poll interval for Watch: frequent
// enough to notice an operator's geoipupdate refresh (typically daily/weekly)
// within a reasonable window, infrequent enough that stat-ing the file is
// negligible overhead.
const DefaultWatchInterval = 5 * time.Minute

// Resolver holds a reloadable mmdb Reader behind an atomic pointer. The zero
// Resolver has no database loaded: Country always reports "not found" and
// Loaded reports false - the state config validation treats as fail-closed
// for any AccessList that configures geo rules.
type Resolver struct {
	reader atomic.Pointer[maxminddb.Reader]
}

// countryRecord decodes only the country ISO code, which both the Country and
// City editions of GeoLite2/GeoIP2 carry.
type countryRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

// Open reads path and returns a Resolver ready to serve lookups. path must be
// an operator-supplied GeoLite2/GeoIP2 .mmdb file.
func Open(path string) (*Resolver, error) {
	res := &Resolver{}
	if err := res.Reload(path); err != nil {
		return nil, err
	}
	return res, nil
}

// Reload re-opens path and, on success, atomically swaps it in. The
// previously loaded reader is NOT closed here: a concurrent Country call may
// have already loaded it and be mid-Lookup, and maxminddb.Reader.Close()
// synchronously munmaps the backing buffer, which would crash that lookup.
// Instead the old reader is left for the garbage collector - maxminddb.Open
// registers a runtime cleanup that munmaps once the Reader becomes
// unreachable, which only happens after every in-flight Lookup releases its
// reference. On failure the previously loaded reader keeps serving unchanged
// and the error is returned for the caller to log - a corrupt or
// momentarily-truncated refresh (e.g. a geoipupdate run overlapping a read)
// never drops a working database.
func (res *Resolver) Reload(path string) error {
	r, err := maxminddb.Open(path)
	if err != nil {
		return err
	}
	res.reader.Swap(r)
	return nil
}

// Loaded reports whether a database is currently available. A nil Resolver
// reports false, so callers need not nil-check before calling it.
func (res *Resolver) Loaded() bool {
	return res != nil && res.reader.Load() != nil
}

// Country returns the ISO-3166-1 alpha-2 country code for ip and whether one
// was found. It returns ("", false) for a nil Resolver, an unloaded database,
// an unparseable IP, a private/loopback/link-local IP (absent from every
// GeoIP database), or any IP the database has no record for - all of which
// are the "unknown" case an AccessList's geo.onUnknown setting governs.
func (res *Resolver) Country(ip net.IP) (string, bool) {
	if res == nil {
		return "", false
	}
	r := res.reader.Load()
	if r == nil {
		return "", false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return "", false
	}
	addr = addr.Unmap()
	result := r.Lookup(addr)
	if !result.Found() {
		return "", false
	}
	var rec countryRecord
	if err := result.Decode(&rec); err != nil || rec.Country.ISOCode == "" {
		return "", false
	}
	return rec.Country.ISOCode, true
}

// Watch polls path's mtime every interval and calls Reload when it changes,
// so an operator's out-of-band refresh (e.g. a geoipupdate cron) is picked up
// without a gpm config change or restart. It blocks until ctx is cancelled,
// so callers run it in its own goroutine. A Reload failure is logged and does
// not stop the watch - the previously loaded database keeps serving and the
// next tick tries again.
func (res *Resolver) Watch(ctx context.Context, path string, interval time.Duration) {
	var lastMod time.Time
	if fi, err := os.Stat(path); err == nil {
		lastMod = fi.ModTime()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fi, err := os.Stat(path)
			if err != nil {
				log.Warn().Err(err).Str("path", path).Msg("geoip: database file stat failed; keeping last loaded database")
				continue
			}
			if !fi.ModTime().After(lastMod) {
				continue
			}
			if err := res.Reload(path); err != nil {
				log.Error().Err(err).Str("path", path).Msg("geoip: reload failed; keeping last loaded database")
				continue
			}
			lastMod = fi.ModTime()
			log.Info().Str("path", path).Msg("geoip: database reloaded")
		}
	}
}

// Close releases the underlying database, if any.
func (res *Resolver) Close() error {
	if res == nil {
		return nil
	}
	if r := res.reader.Load(); r != nil {
		return r.Close()
	}
	return nil
}
