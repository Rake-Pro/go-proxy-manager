package dnssync

import (
	"context"
	"sync"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// memLedger is a hermetic stand-in for the git-backed ownership ledger.
type memLedger struct {
	mu     sync.Mutex
	l      model.DNSLedger
	saves  int
	loadEr error
	saveEr error
}

func (m *memLedger) Load() (model.DNSLedger, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loadEr != nil {
		return model.DNSLedger{}, m.loadEr
	}
	return m.l, nil
}

func (m *memLedger) Save(_ context.Context, l model.DNSLedger) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveEr != nil {
		return m.saveEr
	}
	m.saves++
	m.l = l
	return nil
}

// pihole returns the ledger's Pi-hole entries as domain -> target.
func (m *memLedger) pihole() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return model.DNSLedgerMap(m.l.Pihole)
}

func (m *memLedger) cloudflare() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return model.DNSLedgerMap(m.l.Cloudflare)
}

// ownsPihole seeds the ledger with records gpm is to be treated as having created.
func ownsPihole(entries ...string) *memLedger {
	m := &memLedger{}
	for i := 0; i+1 < len(entries); i += 2 {
		m.l.Pihole = append(m.l.Pihole, model.DNSLedgerEntry{Domain: entries[i], Target: entries[i+1]})
	}
	return m
}

func ownsCloudflare(entries ...string) *memLedger {
	m := &memLedger{}
	for i := 0; i+1 < len(entries); i += 2 {
		m.l.Cloudflare = append(m.l.Cloudflare, model.DNSLedgerEntry{Domain: entries[i], Target: entries[i+1]})
	}
	return m
}

func wantLedger(t *testing.T, got map[string]string, want ...string) {
	t.Helper()
	if len(got) != len(want)/2 {
		t.Fatalf("ledger = %v, want %v", got, want)
	}
	for i := 0; i+1 < len(want); i += 2 {
		if got[want[i]] != want[i+1] {
			t.Fatalf("ledger = %v, want %v", got, want)
		}
	}
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
	persisted, err := cfgStore.LoadDNSLedger()
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
	after, err := cfgStore.LoadDNSLedger()
	if err != nil {
		t.Fatalf("load ledger: %v", err)
	}
	if len(after.Pihole) != 0 {
		t.Fatalf("ledger = %+v, want the deleted record dropped", after)
	}
}

// storeLedger is the same adapter the daemon wires (see cmd/gpm/main.go).
type storeLedger struct{ st *store.Store }

func (s storeLedger) Load() (model.DNSLedger, error) { return s.st.LoadDNSLedger() }

func (s storeLedger) Save(ctx context.Context, l model.DNSLedger) error {
	_, err := s.st.SaveDNSLedger(ctx, l, store.Author{Name: "dns-sync", Email: "gpm@localhost"})
	return err
}
