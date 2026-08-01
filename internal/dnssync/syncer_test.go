package dnssync

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func TestReconcileWithBothBackendsDisabled(t *testing.T) {
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{}, nil
	})
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	st := s.Status()
	if st.Pihole.Enabled || st.Cloudflare.Enabled {
		t.Fatalf("status = %+v", st)
	}
	if st.LastRun.IsZero() {
		t.Fatal("a no-op run should still stamp lastRun")
	}
}

func TestReconcileLoadFailureRecorded(t *testing.T) {
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{}, errors.New("boom")
	})
	err := s.Reconcile(context.Background())
	if err == nil {
		t.Fatal("a load failure must be returned")
	}
	if !strings.Contains(s.Status().Error, "boom") {
		t.Fatalf("status = %+v", s.Status())
	}
}

func TestNilSyncerIsInert(t *testing.T) {
	var s *Syncer
	s.Trigger() // must not panic
	if st := s.Status(); !st.LastRun.IsZero() {
		t.Fatal("nil syncer must report an empty status")
	}
	if p, c := s.Enabled(); p || c {
		t.Fatal("nil syncer must report no backends")
	}
}

func TestEnabledReportsBackends(t *testing.T) {
	s := New(func(context.Context) (model.Config, model.Settings, error) {
		return model.Config{}, model.Settings{DNSSync: model.DNSSyncSettings{
			Pihole: model.PiholeDNSSync{Enabled: true},
		}}, nil
	})
	p, c := s.Enabled()
	if !p || c {
		t.Fatalf("Enabled() = %v, %v", p, c)
	}
}

// Trigger must never block its caller (a config-write handler) and a burst of
// triggers during an in-flight run must collapse into at most one follow-up.
func TestTriggerCoalescesBursts(t *testing.T) {
	var mu sync.Mutex
	runs := 0
	release := make(chan struct{})
	first := make(chan struct{})
	var once sync.Once

	s := New(func(context.Context) (model.Config, model.Settings, error) {
		mu.Lock()
		runs++
		n := runs
		mu.Unlock()
		if n == 1 {
			once.Do(func() { close(first) })
			<-release // hold the first run open while the burst arrives
		}
		return model.Config{}, model.Settings{}, nil
	})

	s.Trigger()
	select {
	case <-first:
	case <-time.After(5 * time.Second):
		t.Fatal("first reconcile never started")
	}
	for i := 0; i < 20; i++ {
		s.Trigger() // all of these must coalesce into one queued run
	}
	close(release)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := runs
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	n := runs
	mu.Unlock()
	if n != 2 {
		t.Fatalf("runs = %d, want exactly 2 (one in-flight + one coalesced follow-up)", n)
	}
}

// The HTTP-triggered reconcile must never queue behind an in-flight run: it
// reports a conflict so the caller sees a 409 instead of parking a goroutine
// (and its request context) behind a slow backend.
func TestReconcileNowRefusesWhileRunning(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once

	s := New(func(context.Context) (model.Config, model.Settings, error) {
		once.Do(func() { close(started) })
		<-release
		return model.Config{}, model.Settings{}, nil
	})

	done := make(chan error, 1)
	go func() { done <- s.ReconcileNow(context.Background()) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first reconcile never started")
	}

	if err := s.ReconcileNow(context.Background()); !errors.Is(err, ErrReconcileInProgress) {
		t.Fatalf("concurrent ReconcileNow = %v, want ErrReconcileInProgress", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	// The lock is released again, so a later manual run is accepted.
	if err := s.ReconcileNow(context.Background()); err != nil {
		t.Fatalf("post-run ReconcileNow: %v", err)
	}
}

func TestReconcileNowUnwired(t *testing.T) {
	var s *Syncer
	if err := s.ReconcileNow(context.Background()); err == nil {
		t.Fatal("a nil syncer must refuse a manual reconcile")
	}
}

func TestDesiredDomains(t *testing.T) {
	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{ObjectMeta: model.ObjectMeta{Name: "a"}, Domains: []string{"B.example.com", "a.example.com"}, DNS: &model.DNSSyncPolicy{LanDirect: true, PublicCname: true}},
		{ObjectMeta: model.ObjectMeta{Name: "b"}, Domains: []string{"a.example.com"}, DNS: &model.DNSSyncPolicy{LanDirect: true}},
		{ObjectMeta: model.ObjectMeta{Name: "c"}, Domains: []string{"c.example.com"}, DNS: &model.DNSSyncPolicy{PublicCname: true}},
		{ObjectMeta: model.ObjectMeta{Name: "d"}, Domains: []string{"d.example.com"}},
		{ObjectMeta: model.ObjectMeta{Name: "e", Disabled: true}, Domains: []string{"e.example.com"}, DNS: &model.DNSSyncPolicy{LanDirect: true, PublicCname: true}},
		{ObjectMeta: model.ObjectMeta{Name: "f"}, Domains: []string{"*.f.example.com", "  ", "edge.example.com"}, DNS: &model.DNSSyncPolicy{LanDirect: true, PublicCname: true}},
	}}

	lan := desiredDomains(cfg, func(p model.DNSSyncPolicy) bool { return p.LanDirect }, "edge.example.com")
	if strings.Join(lan, ",") != "a.example.com,b.example.com" {
		t.Fatalf("lan = %v", lan)
	}
	pub := desiredDomains(cfg, func(p model.DNSSyncPolicy) bool { return p.PublicCname }, "edge.example.com")
	if strings.Join(pub, ",") != "a.example.com,b.example.com,c.example.com" {
		t.Fatalf("public = %v", pub)
	}
}

func TestPiholeRecordTarget(t *testing.T) {
	tests := []struct {
		in             string
		domain, target string
		ok             bool
	}{
		{"a.example.com,edge.example.com", "a.example.com", "edge.example.com", true},
		{"a.example.com,edge.example.com,300", "a.example.com", "edge.example.com", true},
		{"A.Example.com , Edge.Example.com", "a.example.com", "edge.example.com", true},
		{"nonsense", "", "", false},
		{"a.example.com,", "", "", false},
		{",edge.example.com", "", "", false},
	}
	for _, tc := range tests {
		d, tg, ok := piholeRecordTarget(tc.in)
		if ok != tc.ok || d != tc.domain || tg != tc.target {
			t.Fatalf("piholeRecordTarget(%q) = %q,%q,%v", tc.in, d, tg, ok)
		}
	}
}
