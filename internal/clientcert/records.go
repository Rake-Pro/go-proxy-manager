package clientcert

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// Expiry statuses derived from a record's notAfter and its CA's warning window.
const (
	StatusOK       = "ok"
	StatusExpiring = "expiring"
	StatusExpired  = "expired"
)

// retention is how long an expired record is kept before an append prunes it.
// Records are operational memory, not an audit log: an operator needs to know
// which devices still hold a certificate that is about to die or has just died,
// and a year past expiry nobody is still hunting for that device. Superseded
// records are NOT pruned early - a renewed certificate's predecessor stays
// visible until it expires, precisely so the operator remembers old copies are
// still installed somewhere.
const retention = 365 * 24 * time.Hour

// ErrNoStore reports that no certificate store directory was configured, so
// issuance records cannot be persisted.
var ErrNoStore = errors.New("no certificate store directory configured, so client-certificate issuance cannot be recorded")

// ErrRecordNotFound reports that no issuance record with the given serial exists
// for the CA.
var ErrRecordNotFound = errors.New("no issued certificate with that serial")

// Record is one issued client certificate, as remembered by gpm. It carries
// exactly what an operator needs to find the device holding it and decide
// whether to renew - and deliberately NO key material, no PKCS#12 bundle and no
// bundle password. The certificate itself is not stored either: it was handed to
// the operator once and gpm keeps no copy.
type Record struct {
	CA         string    `json:"ca"`
	CommonName string    `json:"commonName"`
	SANs       []string  `json:"sans,omitempty"`
	Serial     string    `json:"serial"`
	NotBefore  time.Time `json:"notBefore"`
	NotAfter   time.Time `json:"notAfter"`
	IssuedAt   time.Time `json:"issuedAt"`

	// SupersededBy is the serial of the certificate that replaced this one via a
	// renewal, with the time it happened. Superseding does NOT revoke: the old
	// certificate stays valid until it expires (revocation is CRL-only), which is
	// why the record stays listed - every device still holding it keeps working
	// until someone imports the replacement.
	SupersededBy string    `json:"supersededBy,omitempty"`
	SupersededAt time.Time `json:"supersededAt,omitempty"`
}

// Status derives the expiry state at time now, given a warning lead time in days.
// A certificate whose remaining life is at or below the window is "expiring", so
// a 30-day window reports a certificate with exactly 30 days left as expiring and
// one with 31 days left as ok.
func (r Record) Status(now time.Time, warnDays int) string {
	switch {
	case !now.Before(r.NotAfter):
		return StatusExpired
	case r.NotAfter.Sub(now) <= time.Duration(warnDays)*24*time.Hour:
		return StatusExpiring
	default:
		return StatusOK
	}
}

// DaysRemaining is whole days until expiry, floored, and negative once expired.
func (r Record) DaysRemaining(now time.Time) int {
	return int(r.NotAfter.Sub(now) / (24 * time.Hour))
}

// Ledger persists issuance records per ClientCA under the managed certificate
// store, following the same shape as the ACME issued-certificate metadata: one
// JSON file per object, written atomically (temp file + rename) at 0600 so a
// crash mid-write can never leave a truncated file behind. This is runtime state,
// not declarative config: it never enters the git-backed config repo and never
// produces a config revision, because it records what gpm DID rather than what
// the operator DECLARED.
//
// A single Ledger value is shared by every handler, and its mutex serializes the
// read-modify-write that appending and superseding both need.
type Ledger struct {
	certDir string
	mu      sync.Mutex
}

// NewLedger returns the ledger rooted at the managed certificate store. An empty
// certDir yields a ledger that reads as empty and refuses to write, rather than
// scattering state through the process working directory.
func NewLedger(certDir string) *Ledger { return &Ledger{certDir: certDir} }

func (l *Ledger) path(ca string) (string, error) {
	if l.certDir == "" {
		return "", ErrNoStore
	}
	// CA names are validated to a safe, single-segment charset before they can be
	// stored, but this is the call that builds a filesystem path from one, so it
	// re-checks rather than trusting the caller.
	if err := model.ValidateName(ca); err != nil {
		return "", err
	}
	return filepath.Join(l.certDir, "client-certs", ca+".json"), nil
}

// List returns this CA's issuance records, newest first. A CA that has never
// issued reads as an empty list, not an error.
func (l *Ledger) List(ca string) ([]Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.load(ca)
}

// load reads the record file. The caller must hold the mutex.
func (l *Ledger) load(ca string) ([]Record, error) {
	p, err := l.path(ca)
	if err != nil {
		if errors.Is(err, ErrNoStore) {
			return nil, nil
		}
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []Record
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil, fmt.Errorf("client-certificate records for %q are unreadable: %w", ca, err)
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].IssuedAt.After(recs[j].IssuedAt) })
	return recs, nil
}

// save writes the record file atomically. The caller must hold the mutex.
func (l *Ledger) save(ca string, recs []Record) error {
	p, err := l.path(ca)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p, b, 0o600)
}

// Append records one issuance, pruning records that expired longer ago than the
// retention window. It returns ErrNoStore when no certificate store is wired.
func (l *Ledger) Append(rec Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	recs, err := l.load(rec.CA)
	if err != nil {
		return err
	}
	kept := make([]Record, 0, len(recs)+1)
	cutoff := time.Now().Add(-retention)
	for _, r := range recs {
		if r.NotAfter.Before(cutoff) {
			continue
		}
		kept = append(kept, r)
	}
	return l.save(rec.CA, append([]Record{rec}, kept...))
}

// Get returns one record by serial.
func (l *Ledger) Get(ca, serial string) (Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	recs, err := l.load(ca)
	if err != nil {
		return Record{}, err
	}
	for _, r := range recs {
		if r.Serial == serial {
			return r, nil
		}
	}
	return Record{}, ErrRecordNotFound
}

// AppendSuperseding records a renewal: the new certificate is appended and the
// record it replaces is marked superseded by it, in one atomic write so the two
// can never disagree. The old record is kept and stays listed - the certificate
// it names is still valid on every device that holds it.
//
// It returns ErrRecordNotFound when oldSerial is not present, rather than writing
// the new record with a dangling supersede link. Silently appending would leave
// two records that both look current, which is exactly the state the supersede
// link exists to prevent.
func (l *Ledger) AppendSuperseding(rec Record, oldSerial string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	recs, err := l.load(rec.CA)
	if err != nil {
		return err
	}
	kept := make([]Record, 0, len(recs)+1)
	cutoff := time.Now().Add(-retention)
	found := false
	for _, r := range recs {
		if r.Serial == oldSerial {
			found = true
			r.SupersededBy = rec.Serial
			r.SupersededAt = rec.IssuedAt
		} else if r.NotAfter.Before(cutoff) {
			// Never prune the record being superseded, however old it is: the
			// supersede link is the whole point of this write.
			continue
		}
		kept = append(kept, r)
	}
	if !found {
		return fmt.Errorf("%w: %s (nothing was recorded)", ErrRecordNotFound, oldSerial)
	}
	return l.save(rec.CA, append([]Record{rec}, kept...))
}

// writeFileAtomic writes data to a temp file in the target directory and renames
// it into place, so a reader never sees a partial file. Same shape as the ACME
// store's writer (internal/acme/store.go).
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
