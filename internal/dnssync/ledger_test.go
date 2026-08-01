package dnssync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// memLedger is a hermetic stand-in for the git-backed ownership ledger. It
// versions its state the way the store does, so the staleness path is testable.
type memLedger struct {
	mu     sync.Mutex
	l      model.DNSLedger
	rev    string
	saves  int
	loadEr error
	saveEr error
	// concurrent, when set, models another writer (a config revert) landing while
	// a reconcile is in flight: the first Save is refused as stale and this ledger
	// becomes the current one, at a new revision.
	concurrent *model.DNSLedger
}

func (m *memLedger) Load(context.Context) (model.DNSLedger, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loadEr != nil {
		return model.DNSLedger{}, "", m.loadEr
	}
	return m.l, m.rev, nil
}

func (m *memLedger) Save(_ context.Context, l model.DNSLedger, rev string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveEr != nil {
		return m.saveEr
	}
	if m.concurrent != nil {
		m.l = *m.concurrent
		m.concurrent = nil
		m.rev = "rev-after-revert"
		return errors.New("ledger changed since it was read")
	}
	if rev != m.rev {
		return fmt.Errorf("stale write: read at %q, now %q", rev, m.rev)
	}
	m.saves++
	m.l = l
	return nil
}

// pihole returns the ledger's Pi-hole entries as domain -> claim.
func (m *memLedger) pihole() map[string]model.DNSClaim {
	m.mu.Lock()
	defer m.mu.Unlock()
	return model.DNSLedgerMap(m.l.Pihole)
}

func (m *memLedger) cloudflare() map[string]model.DNSClaim {
	m.mu.Lock()
	defer m.mu.Unlock()
	return model.DNSLedgerMap(m.l.Cloudflare)
}

// createdEntries builds ledger entries for records gpm is to be treated as having
// CREATED - the only kind it is ever allowed to delete.
func createdEntries(entries ...string) []model.DNSLedgerEntry {
	created := false
	var out []model.DNSLedgerEntry
	for i := 0; i+1 < len(entries); i += 2 {
		out = append(out, model.DNSLedgerEntry{Domain: entries[i], Target: entries[i+1], Adopted: &created})
	}
	return out
}

// adoptedEntries builds ledger entries for records gpm merely ADOPTED, which it
// may manage but must never delete.
func adoptedEntries(entries ...string) []model.DNSLedgerEntry {
	adopted := true
	var out []model.DNSLedgerEntry
	for i := 0; i+1 < len(entries); i += 2 {
		out = append(out, model.DNSLedgerEntry{Domain: entries[i], Target: entries[i+1], Adopted: &adopted})
	}
	return out
}

// ownsPihole seeds the ledger with records gpm is to be treated as having created.
func ownsPihole(entries ...string) *memLedger {
	m := &memLedger{}
	m.l.Pihole = createdEntries(entries...)
	return m
}

func ownsCloudflare(entries ...string) *memLedger {
	m := &memLedger{}
	m.l.Cloudflare = createdEntries(entries...)
	return m
}

func wantLedger(t *testing.T, got map[string]model.DNSClaim, want ...string) {
	t.Helper()
	if len(got) != len(want)/2 {
		t.Fatalf("ledger = %v, want %v", got, want)
	}
	for i := 0; i+1 < len(want); i += 2 {
		if got[want[i]].Target != want[i+1] {
			t.Fatalf("ledger = %v, want %v", got, want)
		}
	}
}

// syncBuffer is a concurrency-safe sink for the zerolog output a test inspects.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs redirects the global logger for the duration of one test, so an
// assertion can be made about what a run told the operator.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := log.Logger
	log.Logger = zerolog.New(buf).With().Timestamp().Logger()
	t.Cleanup(func() { log.Logger = prev })
	return buf
}

// A ledger the syncer cannot read must stop the run dead. Proceeding as if
// nothing were owned is safe for deletion but would re-adopt and re-create on
// every run, so it is reported instead.
func TestReconcileRefusesWhenLedgerUnreadable(t *testing.T) {
	led := &memLedger{loadEr: context.DeadlineExceeded}
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{}, nil
	}, led)
	if err := s.Reconcile(context.Background()); err == nil {
		t.Fatal("an unreadable ledger must fail the reconcile")
	}
	if s.Status().Error == "" {
		t.Fatalf("status = %+v", s.Status())
	}
}

// A steady-state reconcile must not rewrite (and so re-commit) an identical
// ledger: the config repo would collect one revision per reconcile.
func TestLedgerNotRewrittenWhenUnchanged(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{"app.example.com,edge.example.com"}}
	srv := startPihole(t, fake)

	led := ownsPihole("app.example.com", "edge.example.com")
	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")}, led)
	for i := 0; i < 3; i++ {
		if err := s.Reconcile(context.Background()); err != nil {
			t.Fatalf("reconcile %d: %v", i, err)
		}
	}
	led.mu.Lock()
	saves := led.saves
	led.mu.Unlock()
	if saves != 0 {
		t.Fatalf("ledger saved %d times, want 0 (nothing changed)", saves)
	}
}

// End to end through the real git-backed store: a reconcile records what it
// created, a fresh syncer reading the same store knows it owns those records, and
// the claim is durable across the restart. Without this the ledger would be
// in-memory state and the very first restart would re-open the incident window.
func TestLedgerSurvivesRoundTripThroughTheStore(t *testing.T) {
	dir := t.TempDir()
	cfgStore := store.New(dir, store.NewExecGit(dir))
	if err := cfgStore.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}
	led := storeLedger{cfgStore}

	fake := &fakePihole{password: "secret", records: []string{
		"hand.example.com,edge.example.com", // hand written, never desired
	}}
	srv := startPihole(t, fake)

	s := piholeSyncerWith(t, srv, []model.ProxyHost{lanHost("app", "app.example.com")}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if st := s.Status().Pihole; st.Created != 1 || st.Deleted != 0 {
		t.Fatalf("status = %+v", st)
	}

	// The claim landed in git, on its own file, alongside everything else.
	persisted, _, err := cfgStore.LoadDNSLedger(context.Background())
	if err != nil {
		t.Fatalf("load ledger: %v", err)
	}
	if len(persisted.Pihole) != 1 || persisted.Pihole[0].Domain != "app.example.com" {
		t.Fatalf("persisted ledger = %+v", persisted)
	}

	// A brand new syncer (i.e. after a restart) still knows what it owns: dropping
	// the host removes exactly gpm's record and leaves the hand-written one.
	s2 := piholeSyncerWith(t, srv, nil, storeLedger{cfgStore})
	if err := s2.Reconcile(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if st := s2.Status().Pihole; st.Deleted != 1 || st.Untouched != 1 {
		t.Fatalf("status = %+v", st)
	}
	if got := fake.snapshot(); len(got) != 1 || got[0] != "hand.example.com,edge.example.com" {
		t.Fatalf("records = %v, want only the hand-written one", got)
	}
	after, _, err := cfgStore.LoadDNSLedger(context.Background())
	if err != nil {
		t.Fatalf("load ledger: %v", err)
	}
	if len(after.Pihole) != 0 {
		t.Fatalf("ledger = %+v, want the deleted record dropped", after)
	}
}

// A reconcile is a read-modify-write over a file a REVERT can rewrite in the
// middle of it. The store refuses the stale write; the syncer must then re-read
// and re-write WITHOUT resurrecting the claims the revert withdrew - a restored
// claim is a licence to delete a record the operator may since have recreated by
// hand.
func TestLedgerSaveDoesNotResurrectConcurrentlyWithdrawnClaims(t *testing.T) {
	fake := &fakePihole{password: "secret", records: []string{
		"app.example.com,edge.example.com",   // desired: stays claimed
		"other.example.com,edge.example.com", // claimed, and withdrawn mid-run
	}}
	srv := startPihole(t, fake)

	led := &memLedger{rev: "rev-1"}
	led.l.Pihole = createdEntries(
		"app.example.com", "edge.example.com",
		"other.example.com", "edge.example.com",
	)
	// While the run is in flight a revert lands, dropping the claim on other.
	after := model.DNSLedger{Pihole: createdEntries("app.example.com", "edge.example.com")}
	led.concurrent = &after

	s := piholeSyncerWith(t, srv, []model.ProxyHost{
		lanHost("app", "app.example.com"),
		lanHost("new", "new.example.com"),
	}, led)
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if e := s.Status().Error; e != "" {
		t.Fatalf("the retry must succeed, status error = %q", e)
	}

	got := led.pihole()
	if _, resurrected := got["other.example.com"]; resurrected {
		t.Fatalf("REGRESSION: the reconcile re-established a claim the revert withdrew: %v", got)
	}
	// What this run genuinely did still lands: the record it created is claimed.
	if _, ok := got["new.example.com"]; !ok {
		t.Fatalf("ledger = %v, want the record this run created to be recorded", got)
	}
	led.mu.Lock()
	saves := led.saves
	led.mu.Unlock()
	if saves != 1 {
		t.Fatalf("saves = %d, want exactly one successful (retried) write", saves)
	}
}

// storeLedger is the same adapter the daemon wires (see cmd/gpm/main.go).
type storeLedger struct{ st *store.Store }

func (s storeLedger) Load(ctx context.Context) (model.DNSLedger, string, error) {
	return s.st.LoadDNSLedger(ctx)
}

func (s storeLedger) Save(ctx context.Context, l model.DNSLedger, rev string) error {
	_, err := s.st.SaveDNSLedger(context.WithoutCancel(ctx), l, store.Author{Name: "dns-sync", Email: "gpm@localhost"}, rev)
	return err
}
