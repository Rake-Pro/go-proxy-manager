package acme

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

const day = 24 * time.Hour

// TestRenewalTimeLifetimeRule: the window is min(renewBefore, lifetime/3).
func TestRenewalTimeLifetimeRule(t *testing.T) {
	m := NewManager(Options{CertDir: t.TempDir()})
	notAfter := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		notBefore time.Time
		window    time.Duration
	}{
		{"90 days exactly", notAfter.Add(-90 * day), 30 * day},
		// The usual shape: NotAfter one second short of the round period.
		{"90 days minus one second", notAfter.Add(-90*day + time.Second), 30 * day},
		{"one year", notAfter.Add(-365 * day), 30 * day},
		{"45 days", notAfter.Add(-45 * day), 15 * day},
		{"6 days", notAfter.Add(-6 * day), 2 * day},
		{"24 hours", notAfter.Add(-24 * time.Hour), 8 * time.Hour},
		{"NotBefore unknown", time.Time{}, 30 * day},
		{"NotBefore after NotAfter", notAfter.Add(time.Hour), 30 * day},
	}
	for _, tc := range cases {
		at, reason := m.renewalTime(&Meta{NotAfter: notAfter}, tc.notBefore)
		if want := notAfter.Add(-tc.window); !at.Equal(want) || reason != RenewReasonLifetime {
			t.Errorf("%s: renewalTime = %s (%s), want %s (lifetime)", tc.name, at, reason, want)
		}
	}

	// A configured renewBefore below lifetime/3 still wins.
	short := NewManager(Options{CertDir: t.TempDir(), RenewBefore: 10 * day})
	if at, _ := short.renewalTime(&Meta{NotAfter: notAfter}, notAfter.Add(-90*day)); !at.Equal(notAfter.Add(-10 * day)) {
		t.Errorf("renewBefore 10d: renewalTime = %s, want 10 days before expiry", at)
	}
}

// TestNeedsRenewalShortLived: a 6-day certificate renews with 2 days left,
// not on every pass as the fixed 30-day window made it.
func TestNeedsRenewalShortLived(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir})
	cert := model.Certificate{ObjectMeta: model.ObjectMeta{Name: "short"}, Type: model.CertTypeACME, Domains: []string{"app.example.com"}, ACME: &model.ACMESpec{Email: "a@example.com"}}
	now := time.Now()

	writeMeta(t, dir, "short", Meta{Domains: cert.Domains, IssuedAt: now.Add(-2 * day), NotBefore: now.Add(-2 * day), NotAfter: now.Add(4 * day)})
	if m.needsRenewal(cert) {
		t.Error("6-day certificate with 4 days left: needs renewal, want not yet")
	}
	writeMeta(t, dir, "short", Meta{Domains: cert.Domains, IssuedAt: now.Add(-5 * day), NotBefore: now.Add(-5 * day), NotAfter: now.Add(day)})
	if !m.needsRenewal(cert) {
		t.Error("6-day certificate with 1 day left: no renewal, want due")
	}
}

// writeLeafFile stores a self-signed leaf with the given validity as the
// certificate's fullchain.pem and returns it.
func writeLeafFile(t *testing.T, certDir, name string, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "app.example.com"}, DNSNames: []string{"app.example.com"}, NotBefore: notBefore, NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(issuedDir(certDir, name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(IssuedCertPath(certDir, name), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}

// TestNotBeforeForOldMetadata: metadata without notBefore takes it from the
// stored leaf, then from issuedAt, and the file is not rewritten to add it.
func TestNotBeforeForOldMetadata(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir})
	cfg := acmeCert(ca.url("/directory"), "app.example.com")
	now := time.Now().Truncate(time.Second)

	// A 6-day leaf issued 2 days ago, metadata from before the field existed:
	// the leaf says 2-day window, so nothing is due with 4 days left.
	leafStart := now.Add(-2 * day)
	writeLeafFile(t, dir, "app", leafStart, leafStart.Add(6*day))
	writeMeta(t, dir, "app", Meta{Domains: []string{"app.example.com"}, DirectoryURL: ca.url("/directory"), IssuedAt: now.Add(-time.Hour), NotAfter: leafStart.Add(6 * day)})
	before, err := os.ReadFile(metaPath(dir, "app"))
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := loadMeta(dir, "app")
	if got := m.notBefore("app", meta); !got.Equal(leafStart) {
		t.Errorf("notBefore = %s, want the leaf's %s", got, leafStart)
	}
	if changed, err := m.EnsureAll(context.Background(), cfg); err != nil || changed {
		t.Fatalf("EnsureAll: changed = %v, err = %v; want no renewal", changed, err)
	}
	after, _ := os.ReadFile(metaPath(dir, "app"))
	if !bytes.Equal(before, after) || ca.orders != 0 {
		t.Errorf("metadata rewritten (%v) or order placed (%d): want neither", !bytes.Equal(before, after), ca.orders)
	}

	// Without a readable leaf, issuedAt stands in.
	if err := os.Remove(IssuedCertPath(dir, "app")); err != nil {
		t.Fatal(err)
	}
	if got := m.notBefore("app", meta); !got.Equal(meta.IssuedAt) {
		t.Errorf("notBefore without a leaf = %s, want issuedAt %s", got, meta.IssuedAt)
	}
}

// TestIssueRecordsNotBeforeAndProfile: the order carries the configured
// profile, the metadata records it with the leaf's NotBefore, and a changed
// profile makes the certificate due like a changed domain set.
func TestIssueRecordsNotBeforeAndProfile(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	ca.profiles = map[string]string{"classic": "https://ca.example.com/docs/classic", "shortlived": "https://ca.example.com/docs/shortlived"}
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir})
	cfg := acmeCert(ca.url("/directory"), "app.example.com")
	cfg.Certificates[0].ACME.Profile = "shortlived"

	if _, err := m.EnsureAll(context.Background(), cfg); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	if ca.orderProfile != "shortlived" {
		t.Errorf("order profile = %q, want shortlived", ca.orderProfile)
	}
	meta, err := loadMeta(dir, "app")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := readStoredLeaf(dir, "app")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Profile != "shortlived" || !meta.NotBefore.Equal(leaf.NotBefore) {
		t.Errorf("meta profile = %q, notBefore = %s; want shortlived, %s", meta.Profile, meta.NotBefore, leaf.NotBefore)
	}

	if changed, _ := m.EnsureAll(context.Background(), cfg); changed || ca.orders != 1 {
		t.Fatalf("second pass: changed = %v, orders = %d; want no renewal", changed, ca.orders)
	}
	cfg.Certificates[0].ACME.Profile = "classic"
	if !m.needsRenewal(cfg.Certificates[0]) {
		t.Error("changed profile: no renewal, want due")
	}
	cfg.Certificates[0].ACME.Profile = ""
	if !m.needsRenewal(cfg.Certificates[0]) {
		t.Error("profile removed: no renewal, want due")
	}
	if _, err := m.EnsureAll(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if ca.orderProfile != "" {
		t.Errorf("order without a profile sent profile %q", ca.orderProfile)
	}
}

// TestProfileTheCADoesNotOffer: the library's refusal reaches the failure
// record with the certificate and profile named, and no order is sent.
func TestProfileTheCADoesNotOffer(t *testing.T) {
	cases := []struct {
		name     string
		profiles map[string]string
		want     string
	}{
		{"no profiles at all", nil, `certificate "app": authorize order with profile "shortlived": acme: certificate authority does not support profiles`},
		{"another profile", map[string]string{"classic": "https://ca.example.com/docs/classic"}, `certificate "app": authorize order with profile "shortlived": acme: certificate authority does not advertise a profile with name shortlived (supported profiles: classic)`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ca := newStubCA(t, []string{"http-01"})
			ca.profiles = tc.profiles
			m := NewManager(Options{CertDir: t.TempDir()})
			cfg := acmeCert(ca.url("/directory"), "app.example.com")
			cfg.Certificates[0].ACME.Profile = "shortlived"
			_, err := m.EnsureAll(context.Background(), cfg)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("EnsureAll err = %v, want %q", err, tc.want)
			}
			if obs := m.CertObservations(); len(obs) != 1 || obs[0].LastError != tc.want || obs[0].RenewFailures != 1 {
				t.Errorf("observation = %+v, want the error recorded once", obs)
			}
			if ca.orders != 0 {
				t.Errorf("%d orders sent, want none", ca.orders)
			}
		})
	}
}

func TestNextDue(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir})
	now := time.Now()
	m.now = func() time.Time { return now }
	acmeSpec := &model.ACMESpec{Email: "a@example.com"}
	cert := func(name string, disabled bool) model.Certificate {
		c := model.Certificate{ObjectMeta: model.ObjectMeta{Name: name}, Type: model.CertTypeACME, Domains: []string{name + ".example.com"}, ACME: acmeSpec}
		c.Disabled = disabled
		return c
	}
	// 3-hour lifetimes: due one hour before expiry.
	meta := func(name string, dueIn time.Duration) Meta {
		notAfter := now.Add(dueIn + time.Hour)
		return Meta{Domains: []string{name + ".example.com"}, IssuedAt: notAfter.Add(-3 * time.Hour), NotBefore: notAfter.Add(-3 * time.Hour), NotAfter: notAfter}
	}
	attempted := func(name string, at time.Time, failed bool) {
		m.obsMu.Lock()
		o := m.observation(name)
		o.LastAttempt = at
		o.LastError = ""
		if failed {
			o.LastError = "boom"
		}
		m.obsMu.Unlock()
	}
	writeMeta(t, dir, "later", meta("later", 2*time.Hour))
	writeMeta(t, dir, "sooner", meta("sooner", 30*time.Minute))
	writeMeta(t, dir, "off", meta("off", 5*time.Minute))
	moved := meta("moved", 10*time.Minute)
	moved.Domains = []string{"elsewhere.example.com"}
	writeMeta(t, dir, "moved", moved)
	soonest := meta("sooner", 30*time.Minute).NotAfter.Add(-time.Hour)

	// Disabled, changed and unissued certificates that have not failed are
	// attempted on every pass that finds them due, so they add no wake.
	base := []model.Certificate{cert("later", false), cert("sooner", false), cert("off", true), cert("moved", false), cert("never", false)}
	if got := m.nextDue(model.Config{Certificates: base}, now); !got.Equal(soonest) {
		t.Errorf("nextDue = %s, want the soonest renewal %s", got, soonest)
	}

	// Overdue and attempted successfully since it became due: left to the
	// regular tick (a CA handing out certificates due on arrival).
	writeMeta(t, dir, "overdue", meta("overdue", -10*time.Minute))
	overdueAt := meta("overdue", -10*time.Minute).NotAfter.Add(-time.Hour)
	attempted("overdue", now.Add(-time.Minute), false)
	if got := m.nextDue(model.Config{Certificates: []model.Certificate{cert("overdue", false)}}, now); !got.IsZero() {
		t.Errorf("overdue, attempted successfully since: nextDue = %s, want zero", got)
	}
	// Overdue with no attempt since it became due (it crossed into its window
	// while the pass ran): its renewal time, so Run passes again at once.
	attempted("overdue", overdueAt.Add(-time.Minute), false)
	if got := m.nextDue(model.Config{Certificates: []model.Certificate{cert("overdue", false)}}, now); !got.Equal(overdueAt) {
		t.Errorf("overdue, not attempted since: nextDue = %s, want %s", got, overdueAt)
	}
	// Overdue and failed: the retry time, a quarter of the 1-hour window.
	attempted("overdue", now.Add(-time.Minute), true)
	if got, want := m.nextDue(model.Config{Certificates: []model.Certificate{cert("overdue", false)}}, now), now.Add(-time.Minute+15*time.Minute); !got.Equal(want) {
		t.Errorf("overdue, failed: nextDue = %s, want the retry at %s", got, want)
	}
	// A failed first issuance and a failed changed order retry after an hour.
	attempted("never", now.Add(-10*time.Minute), true)
	if got, want := m.nextDue(model.Config{Certificates: []model.Certificate{cert("never", false)}}, now), now.Add(50*time.Minute); !got.Equal(want) {
		t.Errorf("unissued, failed: nextDue = %s, want %s", got, want)
	}
	// A failed attempt on a certificate that is not due (a RenewNow) adds
	// nothing but its normal renewal time.
	attempted("later", now.Add(-time.Minute), true)
	if got, want := m.nextDue(model.Config{Certificates: []model.Certificate{cert("later", false)}}, now), meta("later", 2*time.Hour).NotAfter.Add(-time.Hour); !got.Equal(want) {
		t.Errorf("not due, failed: nextDue = %s, want its renewal time %s", got, want)
	}
}

func TestEarlyWake(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		due  time.Time
		want time.Duration
		ok   bool
	}{
		{"nothing due", time.Time{}, 0, false},
		{"due in two hours", now.Add(2 * time.Hour), 2 * time.Hour, true},
		{"due in ten seconds", now.Add(10 * time.Second), time.Minute, true},
		{"due already", now.Add(-time.Hour), time.Minute, true},
	}
	for _, tc := range cases {
		got, ok := earlyWake(now, tc.due, time.Minute)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: earlyWake = %s, %v; want %s, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// TestRunWakesEarlyForAShortWindow: with a regular interval far longer than
// the time to the next renewal, Run passes again when the certificate enters
// its window, then waits for the regular tick. A renewal that fails at that
// pass is not retried in a loop, and cancelling the context stops Run.
// TestRunWakesEarlyForAShortWindow: with a regular interval far longer than
// the time to the next renewal, Run passes again when the certificate enters
// its window. A renewal that then fails is retried after its backoff (15
// minutes for this 3-hour certificate), not in a loop: with a one-millisecond
// wake floor, any retry without the backoff would show up as more passes and
// more failed attempts within the observation time. Cancelling the context
// stops Run.
func TestRunWakesEarlyForAShortWindow(t *testing.T) {
	cases := []struct {
		name    string
		failing bool
	}{
		{"renewal succeeds", false},
		{"renewal fails", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirURL := ""
			var ca *stubCA
			if tc.failing {
				// 404 fails the account registration at once (the library
				// retries 5xx answers until the context ends).
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
				t.Cleanup(srv.Close)
				dirURL = srv.URL + "/directory"
			} else {
				ca = newStubCA(t, []string{"http-01"})
				dirURL = ca.url("/directory")
			}
			dir := t.TempDir()
			m := NewManager(Options{CertDir: dir})
			m.minWake = time.Millisecond
			// A 3-hour certificate entering its 1-hour window 500ms from now.
			notBefore := time.Now().Add(-2*time.Hour + 500*time.Millisecond)
			writeMeta(t, dir, "app", Meta{Domains: []string{"app.example.com"}, DirectoryURL: dirURL, IssuedAt: notBefore, NotBefore: notBefore, NotAfter: notBefore.Add(3 * time.Hour)})

			var passes atomic.Int32
			cfg := acmeCert(dirURL, "app.example.com")
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				m.Run(ctx, time.Hour, func(context.Context) (model.Config, error) {
					passes.Add(1)
					return cfg, nil
				})
			}()
			attempts := func() (int64, time.Time) {
				for _, o := range m.CertObservations() {
					return o.RenewFailures, o.LastAttempt
				}
				return 0, time.Time{}
			}

			deadline := time.Now().Add(30 * time.Second) // failure bound only
			for time.Now().Before(deadline) {
				if _, last := attempts(); !last.IsZero() {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if n := passes.Load(); n != 2 {
				cancel()
				<-done
				t.Fatalf("passes = %d at the first attempt, want the early second pass", n)
			}
			// Watch long enough for hundreds of passes at the 1ms floor.
			time.Sleep(500 * time.Millisecond)
			failures, _ := attempts()
			if n := passes.Load(); n != 2 {
				t.Errorf("passes = %d after the early pass, want 2 until the retry or the regular tick", n)
			}
			cancel()
			select {
			case <-done:
			case <-time.After(30 * time.Second): // failure bound only
				t.Fatal("Run did not return after ctx was cancelled")
			}

			obs := m.CertObservations()
			if len(obs) != 1 {
				t.Fatalf("observations = %+v", obs)
			}
			if tc.failing {
				if failures != 1 || !strings.Contains(obs[0].LastError, "register ACME account") {
					t.Errorf("failures = %d, observation = %+v; want exactly one failed attempt", failures, obs[0])
				}
				return
			}
			if ca.orders != 1 || obs[0].LastError != "" {
				t.Errorf("orders = %d, observation = %+v; want one successful renewal", ca.orders, obs[0])
			}
		})
	}
}
