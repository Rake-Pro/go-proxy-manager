package acme

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func TestRetryBackoff(t *testing.T) {
	h := time.Hour
	m := time.Minute
	cases := []struct {
		name   string
		window time.Duration
		waits  []time.Duration // after the 1st, 2nd, ... consecutive failure
	}{
		{"90-day or never issued (30d window)", 30 * day, []time.Duration{h, 2 * h, 4 * h, 8 * h, 12 * h, 12 * h, 12 * h}},
		{"24-hour (8h window)", 8 * h, []time.Duration{h, 2 * h, 4 * h, 8 * h, 12 * h, 12 * h}},
		{"6-hour (2h window)", 2 * h, []time.Duration{30 * m, h, 2 * h, 4 * h, 8 * h, 12 * h}},
		{"1-hour (20m window)", 20 * m, []time.Duration{5 * m, 10 * m, 20 * m, 40 * m, 80 * m, 160 * m, 320 * m, 640 * m, 12 * h, 12 * h}},
		{"tiny window", 4 * m, []time.Duration{5 * m, 10 * m}},
	}
	for _, tc := range cases {
		for i, want := range tc.waits {
			if got := retryBackoff(tc.window, i+1); got != want {
				t.Errorf("%s: failure %d waits %s, want %s", tc.name, i+1, got, want)
			}
		}
	}
	// No overflow however long the streak.
	if got := retryBackoff(30*day, 1000); got != 12*h {
		t.Errorf("failure 1000 waits %s, want the 12h cap", got)
	}
}

// notFoundCA fails every ACME request at once with a 404, so each attempt
// is one quick failure.
func notFoundCA(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/directory"
}

// TestFailedRenewalRetriesWithBackoff drives the manager's clock: after a
// failed attempt the certificate is retried once its backoff has passed, not
// before, the wait doubles with each further consecutive failure, and the
// wake computation names that retry time instead of the 12-hour tick.
func TestFailedRenewalRetriesWithBackoff(t *testing.T) {
	t0 := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		lifetime time.Duration // zero: never issued
		failAt   time.Duration // clock offset from NotBefore of the first failure
		waits    []time.Duration
	}{
		{"24-hour certificate", 24 * time.Hour, 16*time.Hour + time.Minute, []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 12 * time.Hour}},
		{"90-day certificate", 90 * day, 61 * day, []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 12 * time.Hour, 12 * time.Hour}},
		{"3-hour certificate", 3 * time.Hour, 2*time.Hour + time.Minute, []time.Duration{15 * time.Minute, 30 * time.Minute, time.Hour}},
		{"first issuance", 0, 0, []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirURL := notFoundCA(t)
			dir := t.TempDir()
			m := NewManager(Options{CertDir: dir})
			clock := t0.Add(tc.failAt)
			m.now = func() time.Time { return clock }
			cfg := acmeCert(dirURL, "app.example.com")
			if tc.lifetime != 0 {
				writeMeta(t, dir, "app", Meta{Domains: []string{"app.example.com"}, DirectoryURL: dirURL, IssuedAt: t0, NotBefore: t0, NotAfter: t0.Add(tc.lifetime)})
			}
			failures := func() int64 { return m.CertObservations()[0].RenewFailures }

			// Due and never attempted: attempted on the pass that finds it due.
			if _, err := m.EnsureAll(context.Background(), cfg); err == nil || failures() != 1 {
				t.Fatalf("first pass: err = %v, failures = %d; want one failed attempt", err, failures())
			}
			for i, wait := range tc.waits {
				last := clock
				if got := m.nextDue(cfg, clock); !got.Equal(last.Add(wait)) {
					t.Fatalf("after failure %d: nextDue = %s, want %s later", i+1, got.Sub(last), wait)
				}
				// A pass inside the backoff (woken for something else, or the
				// regular tick) does not retry it.
				clock = last.Add(wait - time.Second)
				if _, err := m.EnsureAll(context.Background(), cfg); err != nil || failures() != int64(i+1) {
					t.Fatalf("pass inside backoff %d: err = %v, failures = %d; want no attempt", i+1, err, failures())
				}
				clock = last.Add(wait)
				if _, err := m.EnsureAll(context.Background(), cfg); err == nil || failures() != int64(i+2) {
					t.Fatalf("pass at retry %d: err = %v, failures = %d; want another attempt", i+1, err, failures())
				}
			}
			if tc.lifetime == 24*time.Hour {
				// 4 tries inside the 8-hour window: at 0, 1h, 3h and 7h.
				if inside := tc.waits[0] + tc.waits[1] + tc.waits[2]; inside >= 8*time.Hour {
					t.Errorf("fourth try at %s, want inside the window", inside)
				}
			}
		})
	}
}

// TestRetryBackoffResetsOnSuccess: a success resets the streak, so the next
// failure waits the base again; a failed RenewNow counts toward the streak.
func TestRetryBackoffResetsOnSuccess(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	failing := true
	ca.onAccept = func(string, string) error {
		if failing {
			return errors.New("validation failed")
		}
		return nil
	}
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir, RenewCooldown: time.Nanosecond})
	clock := time.Now()
	m.now = func() time.Time { return clock }
	cfg := acmeCert(ca.url("/directory"), "app.example.com")

	if _, err := m.EnsureAll(context.Background(), cfg); err == nil {
		t.Fatal("first attempt: want a failure")
	}
	clock = clock.Add(time.Hour)
	if _, err := m.EnsureAll(context.Background(), cfg); err == nil {
		t.Fatal("second attempt: want a failure")
	}
	if got := m.nextDue(cfg, clock).Sub(clock); got != 2*time.Hour {
		t.Fatalf("after two failures: wait = %s, want 2h", got)
	}
	failing = false
	clock = clock.Add(2 * time.Hour)
	if changed, err := m.EnsureAll(context.Background(), cfg); err != nil || !changed {
		t.Fatalf("third attempt: changed = %v, err = %v; want issued", changed, err)
	}

	// A changed domain set makes it due again; the first failure after the
	// success waits the base, not a doubled wait.
	failing = true
	ca.authzValid = false // the stub would otherwise reuse the valid authorization
	cfg.Certificates[0].Domains = []string{"app.example.com", "www.example.com"}
	clock = clock.Add(time.Minute)
	if _, err := m.EnsureAll(context.Background(), cfg); err == nil {
		t.Fatal("attempt after the success: want a failure")
	}
	if got := m.nextDue(cfg, clock).Sub(clock); got != time.Hour {
		t.Errorf("first failure after a success: wait = %s, want the 1h base", got)
	}

	// A failed RenewNow extends the streak: the next wait doubles.
	done := make(chan struct{})
	m.onRenewFailure = func(string, error) { close(done) }
	clock = clock.Add(time.Second) // past the test's 1ns RenewNow cooldown
	if err := m.RenewNow(context.Background(), cfg, "app"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second): // failure bound only
		t.Fatal("RenewNow did not finish")
	}
	if !m.mu.TryLock() {
		t.Fatal("RenewNow left the lock held")
	}
	m.mu.Unlock()
	if got := m.nextDue(cfg, clock).Sub(clock); got != 2*time.Hour {
		t.Errorf("after a failed RenewNow: wait = %s, want 2h", got)
	}
}

// TestWorstCaseAttemptsPerDay plays Run's schedule (the earlier of the next
// wake and the 12-hour tick) against a CA that fails every attempt, and
// counts the attempts in the first and the second day.
func TestWorstCaseAttemptsPerDay(t *testing.T) {
	t0 := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name          string
		lifetime      time.Duration // zero: never issued
		failAt        time.Duration
		first, second int
	}{
		{"never issued", 0, 0, 5, 2},
		{"90-day", 90 * day, 61 * day, 5, 2},
		{"24-hour", 24 * time.Hour, 16*time.Hour + time.Minute, 5, 2},
		{"1-hour", time.Hour, 41 * time.Minute, 9, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirURL := notFoundCA(t)
			dir := t.TempDir()
			m := NewManager(Options{CertDir: dir})
			start := t0.Add(tc.failAt)
			clock := start
			m.now = func() time.Time { return clock }
			cfg := acmeCert(dirURL, "app.example.com")
			if tc.lifetime != 0 {
				writeMeta(t, dir, "app", Meta{Domains: []string{"app.example.com"}, DirectoryURL: dirURL, IssuedAt: t0, NotBefore: t0, NotAfter: t0.Add(tc.lifetime)})
			}
			counts := [2]int{}
			tick := start.Add(12 * time.Hour)
			for clock.Before(start.Add(48 * time.Hour)) {
				before := m.CertObservations()
				_, _ = m.EnsureAll(context.Background(), cfg)
				after := m.CertObservations()
				if len(after) == 1 && (len(before) == 0 || after[0].RenewFailures > before[0].RenewFailures) {
					counts[clock.Sub(start)/(24*time.Hour)]++
				}
				next := tick
				if due := m.nextDue(cfg, clock); !due.IsZero() && due.Before(next) {
					next = due
					if floor := clock.Add(time.Minute); next.Before(floor) {
						next = floor // Run's one-minute wake floor
					}
				}
				if !next.After(clock) {
					next = clock.Add(time.Minute)
				}
				if !next.Before(tick) {
					tick = tick.Add(12 * time.Hour)
				}
				clock = next
			}
			if counts[0] != tc.first || counts[1] != tc.second {
				t.Errorf("attempts: first day %d, second day %d; want %d, %d", counts[0], counts[1], tc.first, tc.second)
			}
		})
	}
}

// TestClockSteppedBack: a failure recorded at a wall time later than now (the
// clock was set back since) does not hold off the retry or RenewNow.
func TestClockSteppedBack(t *testing.T) {
	dirURL := notFoundCA(t)
	m := NewManager(Options{CertDir: t.TempDir()})
	clock := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return clock }
	cfg := acmeCert(dirURL, "app.example.com")
	if _, err := m.EnsureAll(context.Background(), cfg); err == nil {
		t.Fatal("want a failure")
	}

	clock = time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	if at, failed := m.retryAt(cfg.Certificates[0], nil, clock); !failed || !at.Equal(clock) {
		t.Errorf("retryAt = %s (failed %v), want now", at, failed)
	}
	if got := m.nextDue(cfg, clock); !got.Equal(clock) {
		t.Errorf("nextDue = %s, want now", got)
	}
	if got := m.renewCooldownRemaining("app"); got != 0 {
		t.Errorf("RenewNow cooldown = %s, want zero", got)
	}
	if _, err := m.EnsureAll(context.Background(), cfg); err == nil || m.CertObservations()[0].RenewFailures != 2 {
		t.Fatalf("pass after the clock stepped back: want a second attempt, failures = %d", m.CertObservations()[0].RenewFailures)
	}
	// From the new attempt on, the backoff counts normally again.
	if got := m.nextDue(cfg, clock).Sub(clock); got != 2*time.Hour {
		t.Errorf("after the retry: wait = %s, want 2h", got)
	}
	// The regular cooldown still applies to an attempt in the past.
	if got := m.renewCooldownRemaining("app"); got != time.Hour {
		t.Errorf("RenewNow cooldown right after an attempt = %s, want 1h", got)
	}
}

// TestAttemptDeadline: a CA whose authorization never leaves pending fails
// the attempt at its deadline, records it, and frees the issuance lock.
func TestAttemptDeadline(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	ca.authzStuck = true
	m := NewManager(Options{CertDir: t.TempDir()})
	m.orderBudget = 300 * time.Millisecond
	began := time.Now()
	_, err := m.EnsureAll(context.Background(), acmeCert(ca.url("/directory"), "app.example.com"))
	if took := time.Since(began); took > 10*time.Second {
		t.Fatalf("attempt took %s, want it stopped near its deadline", took)
	}
	want := `certificate "app": the attempt did not finish within 300ms: authorization failed: context deadline exceeded`
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if o := m.CertObservations()[0]; o.RenewFailures != 1 || o.LastError != want {
		t.Errorf("observation = %+v, want the failure recorded", o)
	}
	if !m.mu.TryLock() {
		t.Fatal("the issuance lock is still held")
	}
	m.mu.Unlock()
	// The default budget is the propagation timeout plus ten minutes.
	if d := NewManager(Options{CertDir: t.TempDir(), PropagationTimeout: 2 * time.Minute}); d.orderBudget != 0 || d.propagationTimeout+10*time.Minute != 12*time.Minute {
		t.Errorf("default budget inputs = %s, %s", d.orderBudget, d.propagationTimeout)
	}
}

// TestRunRetriesAFailedConfigLoad: a pass that cannot load the config is
// followed by another after loadRetry, not sooner, until a load succeeds;
// then Run is back to waiting for its regular tick.
func TestRunRetriesAFailedConfigLoad(t *testing.T) {
	m := NewManager(Options{CertDir: t.TempDir()})
	m.loadRetry = 100 * time.Millisecond
	var (
		mu    sync.Mutex
		calls []time.Time
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx, time.Hour, func(context.Context) (model.Config, error) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, time.Now())
			if len(calls) <= 3 {
				return model.Config{}, errors.New("store unavailable")
			}
			return model.Config{}, nil
		})
	}()
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(calls)
	}
	deadline := time.Now().Add(30 * time.Second) // failure bound only
	for count() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(400 * time.Millisecond)
	cancel()
	<-done

	if len(calls) != 4 {
		t.Fatalf("loads = %d, want 3 failures, 1 success and then quiet until the tick", len(calls))
	}
	for i := 1; i < len(calls); i++ {
		if gap := calls[i].Sub(calls[i-1]); gap < m.loadRetry {
			t.Errorf("load %d came %s after the failed one, want at least %s", i+1, gap, m.loadRetry)
		}
	}
}

// TestChangedOrderReportsDueNow: a certificate whose domains or profile
// changed is due at once, and the status says so.
func TestChangedOrderReportsDueNow(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir})
	clock := time.Date(2030, 1, 10, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return clock }
	t0 := clock.Add(-10 * day)
	writeMeta(t, dir, "app", Meta{Domains: []string{"app.example.com"}, IssuedAt: t0, NotBefore: t0, NotAfter: t0.Add(90 * day)})
	meta := mustMeta(t, dir, "app")

	cert := acmeCert("https://ca.example.com/directory", "app.example.com").Certificates[0]
	m.recordIssued(cert, meta, clock)
	if o := m.CertObservations()[0]; o.RenewReason != RenewReasonLifetime || !o.RenewAt.Equal(t0.Add(60*day)) {
		t.Errorf("unchanged: renewAt = %s (%s), want %s (lifetime)", o.RenewAt, o.RenewReason, t0.Add(60*day))
	}
	for name, mut := range map[string]func(*model.Certificate){
		"domains": func(c *model.Certificate) { c.Domains = []string{"app.example.com", "www.example.com"} },
		"profile": func(c *model.Certificate) { a := *c.ACME; a.Profile = "shortlived"; c.ACME = &a },
	} {
		changed := cert
		mut(&changed)
		m.recordIssued(changed, meta, clock)
		if o := m.CertObservations()[0]; o.RenewReason != RenewReasonChanged || !o.RenewAt.Equal(clock) {
			t.Errorf("%s changed: renewAt = %s (%s), want %s (changed)", name, o.RenewAt, o.RenewReason, clock)
		}
	}
}

// ctxSolver records a cleanup only when its context is still live, as a real
// provider call would fail on an expired one.
type ctxSolver struct{ *recordingSolver }

func (s ctxSolver) CleanUp(ctx context.Context, name, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.recordingSolver.CleanUp(ctx, name, value)
}

// TestAttemptDeadlineStillCleansUp: the TXT records of an attempt that ran
// out of time are still removed.
func TestAttemptDeadlineStillCleansUp(t *testing.T) {
	ca := newStubCA(t, []string{"dns-01"})
	ca.authzStuck = true
	rec := newRecordingSolver()
	m := NewManager(Options{CertDir: t.TempDir(), NewSolver: func(model.DNSProvider) (DNSSolver, error) { return ctxSolver{rec}, nil }})
	m.resolver = stubTXTLookuper{rec}
	m.propagationInterval = time.Millisecond
	m.orderBudget = 300 * time.Millisecond
	cfg := acmeCert(ca.url("/directory"), "app.example.com")
	cfg.Certificates[0].ACME.DNSProvider = "dns"
	cfg.DNSProviders = []model.DNSProvider{{ObjectMeta: model.ObjectMeta{Name: "dns"}, Provider: model.DNSProviderCloudflare, Config: map[string]model.Secret{"apiToken": "t"}}}
	if _, err := m.EnsureAll(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "did not finish within") {
		t.Fatalf("err = %v, want the deadline", err)
	}
	if _, ok := rec.cleaned["_acme-challenge.app.example.com"]; !ok {
		t.Errorf("TXT record not cleaned up after the deadline: %v", rec.cleaned)
	}
}
