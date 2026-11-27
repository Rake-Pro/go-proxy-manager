package acme

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// TestCertIDRFC9773Example is the worked example of RFC 9773, Appendix A.
func TestCertIDRFC9773Example(t *testing.T) {
	aki, err := hex.DecodeString(strings.ReplaceAll("69:88:5B:6B:87:46:40:41:E1:B3:7B:84:7B:A0:AE:2C:DE:01:C8:D4", ":", ""))
	if err != nil {
		t.Fatal(err)
	}
	// Serial 00:87:65:43:21: the high bit of 0x87 is set, so DER carries the
	// leading zero byte.
	got, err := certIDFrom(aki, big.NewInt(0x87654321))
	if err != nil {
		t.Fatal(err)
	}
	if want := "aYhba4dGQEHhs3uEe6CuLN4ByNQ.AIdlQyE"; got != want {
		t.Errorf("certID = %q, want %q", got, want)
	}
}

func TestCertIDSerialEncoding(t *testing.T) {
	cases := []struct {
		serial int64
		want   string // base64url of the DER INTEGER content
	}{
		{0x01, "AQ"},
		{0x7f, "fw"},
		{0x80, "AIA"},             // high bit set: 00 80
		{0x0100, "AQA"},           // no padding byte needed
		{0x7fffffff, "f____w"},    // largest without the leading zero
		{0x80000000, "AIAAAAA"},   // smallest that needs it
		{0x00ff00ff00, "AP8A_wA"}, // 00 ff 00 ff 00
		{0x0102030405060708, "AQIDBAUGBwg"},
	}
	for _, tc := range cases {
		got, err := certIDFrom([]byte{0xab}, big.NewInt(tc.serial))
		if err != nil {
			t.Fatal(err)
		}
		if want := "qw." + tc.want; got != want {
			t.Errorf("serial %#x: certID = %q, want %q", tc.serial, got, want)
		}
	}
}

func TestARICertIDNeedsAKI(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(5), NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	if _, err := ariCertID(leaf); err == nil {
		t.Error("ariCertID of a leaf without an authority key identifier: want an error")
	}
}

func TestARIPollInterval(t *testing.T) {
	cases := map[string]time.Duration{
		"":                    6 * time.Hour,
		"soon":                6 * time.Hour,
		"-5":                  6 * time.Hour,
		"1.5":                 6 * time.Hour,
		"0":                   time.Hour,
		"30":                  time.Hour,
		"3600":                time.Hour,
		"7200":                2 * time.Hour,
		" 86400 ":             24 * time.Hour,
		"604800":              7 * 24 * time.Hour,
		"604801":              7 * 24 * time.Hour,
		"99999999":            7 * 24 * time.Hour,
		"9223372036854775807": 7 * 24 * time.Hour,
	}
	for header, want := range cases {
		if got := ariPollInterval(header); got != want {
			t.Errorf("Retry-After %q: interval = %s, want %s", header, got, want)
		}
	}
}

func TestExplanationURL(t *testing.T) {
	long := "https://ca.example.com/" + strings.Repeat("a", 512)
	cases := map[string]string{
		"https://ca.example.com/incident/42": "https://ca.example.com/incident/42",
		"http://ca.example.com/why":          "http://ca.example.com/why",
		"javascript:alert(1)":                "",
		"ftp://ca.example.com/why":           "",
		"/relative/path":                     "",
		"https://":                           "",
		long:                                 "",
		long[:512]:                           long[:512],
	}
	for in, want := range cases {
		if got := explanationURL(in); got != want {
			t.Errorf("explanationURL(%.40q) = %.40q, want %.40q", in, got, want)
		}
	}
}

// TestRenewalTimeWithARI covers the pure rule: ARI brings the renewal forward,
// never later than the lifetime rule, and not before the minimum age.
func TestRenewalTimeWithARI(t *testing.T) {
	m := NewManager(Options{CertDir: t.TempDir()})
	issued := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	meta := func(lifetime time.Duration, ariAt time.Time) *Meta {
		return &Meta{IssuedAt: issued, NotBefore: issued, NotAfter: issued.Add(lifetime), ARI: &ARIState{RenewAt: ariAt}}
	}
	cases := []struct {
		name       string
		meta       *Meta
		want       time.Time
		wantReason string
	}{
		{"ari earlier than lifetime", meta(90*day, issued.Add(40*day)), issued.Add(40 * day), RenewReasonARI},
		{"ari later than lifetime", meta(90*day, issued.Add(70*day)), issued.Add(60 * day), RenewReasonLifetime},
		{"ari before the 72h minimum age", meta(90*day, issued.Add(time.Hour)), issued.Add(72 * time.Hour), RenewReasonARI},
		// 6 days: the minimum age is lifetime/3 = 48h, the lifetime rule 96h.
		{"short-lived, ari before lifetime/3", meta(6*day, issued.Add(time.Hour)), issued.Add(48 * time.Hour), RenewReasonARI},
		{"short-lived, ari after lifetime/3", meta(6*day, issued.Add(50*time.Hour)), issued.Add(50 * time.Hour), RenewReasonARI},
		// 160 hours: minimum age 53h20m, lifetime rule at 106h40m.
		{"160h, ari before lifetime/3", meta(160*time.Hour, issued.Add(time.Hour)), issued.Add(160 * time.Hour / 3), RenewReasonARI},
		// 24 hours: the minimum age equals the window, so ARI can move the
		// renewal to 8h at the earliest, the lifetime rule renews at 16h.
		{"24h, ari before lifetime/3", meta(24*time.Hour, issued.Add(time.Minute)), issued.Add(8 * time.Hour), RenewReasonARI},
		{"no ari time", &Meta{IssuedAt: issued, NotBefore: issued, NotAfter: issued.Add(90 * day), ARI: &ARIState{NextCheck: issued}}, issued.Add(60 * day), RenewReasonLifetime},
	}
	for _, tc := range cases {
		got, reason := m.renewalTime(tc.meta, tc.meta.NotBefore)
		if !got.Equal(tc.want) || reason != tc.wantReason {
			t.Errorf("%s: renewalTime = %s (%s), want %s (%s)", tc.name, got, reason, tc.want, tc.wantReason)
		}
	}
}

// issueFixture stores a certificate for app.example.com signed by the stub
// CA, issued age ago with the given lifetime, as the manager would have.
func issueFixture(t *testing.T, ca *stubCA, certDir, name string, lifetime, age time.Duration, ari *ARIState) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	notBefore := time.Now().Add(-age).UTC().Truncate(time.Second)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(0x87654321),
		Subject:      pkix.Name{CommonName: "app.example.com"},
		DNSNames:     []string{"app.example.com"},
		NotBefore:    notBefore,
		NotAfter:     notBefore.Add(lifetime),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.caCert, &key.PublicKey, ca.caKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveIssued(certDir, name, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key, Meta{
		Domains:      []string{"app.example.com"},
		DirectoryURL: ca.url("/directory"),
		IssuedAt:     notBefore,
		NotBefore:    notBefore,
		NotAfter:     notBefore.Add(lifetime),
		ARI:          ari,
	}); err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return leaf
}

// window answers the renewal-information endpoint with a fixed window.
func window(start, end time.Time, retryAfter string) func(http.ResponseWriter, string) {
	return func(w http.ResponseWriter, _ string) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"suggestedWindow":{"start":%q,"end":%q},"explanationURL":"https://ca.example.com/why"}`, start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
}

func ariCertCfg(ca *stubCA, names ...string) model.Config {
	var cfg model.Config
	for _, n := range names {
		cfg.Certificates = append(cfg.Certificates, model.Certificate{
			ObjectMeta: model.ObjectMeta{Name: n},
			Type:       model.CertTypeACME,
			Domains:    []string{"app.example.com"},
			ACME:       &model.ACMESpec{Email: "admin@example.com", DirectoryURL: ca.url("/directory")},
		})
	}
	return cfg
}

func mustMeta(t *testing.T, dir, name string) *Meta {
	t.Helper()
	meta, err := loadMeta(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	return meta
}

func TestARINotOfferedByTheCA(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	dir := t.TempDir()
	issueFixture(t, ca, dir, "app", 90*day, 10*day, nil)
	before, _ := os.ReadFile(metaPath(dir, "app"))

	m := NewManager(Options{CertDir: dir})
	if changed, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app")); err != nil || changed {
		t.Fatalf("EnsureAll: changed = %v, err = %v", changed, err)
	}
	after, _ := os.ReadFile(metaPath(dir, "app"))
	if len(ca.ariIDs) != 0 || !bytes.Equal(before, after) {
		t.Errorf("ari requests = %v, metadata rewritten = %v; want neither for a CA without renewalInfo", ca.ariIDs, !bytes.Equal(before, after))
	}
}

// TestARIEndpointOnAnotherOrigin: a renewalInfo URL on another host or
// scheme than the directory is skipped with one warning per pass, and the
// directory is fetched once per pass for all certificates using it.
func TestARIEndpointOnAnotherOrigin(t *testing.T) {
	for _, other := range []string{"http://ari.example.net/renewal-info", "https://" + "127.0.0.1" + ":1/renewal-info"} {
		t.Run(other, func(t *testing.T) {
			ca := newStubCA(t, []string{"http-01"})
			if strings.HasPrefix(other, "https://") {
				// Same host and port as the plain-http stub, other scheme.
				other = strings.Replace(ca.ariURL(), "http://", "https://", 1)
			}
			ca.renewalInfo = other
			ca.onRenewalInfo = window(time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour), "")
			dir := t.TempDir()
			issueFixture(t, ca, dir, "one", 90*day, 10*day, nil)
			issueFixture(t, ca, dir, "two", 90*day, 10*day, nil)

			var buf bytes.Buffer
			prev := log.Logger
			log.Logger = zerolog.New(&buf)
			t.Cleanup(func() { log.Logger = prev })

			m := NewManager(Options{CertDir: dir})
			if changed, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "one", "two")); err != nil || changed {
				t.Fatalf("EnsureAll: changed = %v, err = %v; want nothing renewed", changed, err)
			}
			if n := strings.Count(buf.String(), "renewal information is not used"); n != 1 {
				t.Errorf("warnings = %d, want 1 per pass; log: %s", n, buf.String())
			}
			if ca.directoryGets != 1 {
				t.Errorf("directory fetched %d times in one pass, want 1", ca.directoryGets)
			}
			if len(ca.ariIDs) != 0 || mustMeta(t, dir, "one").ARI != nil {
				t.Errorf("renewal information used from another origin: requests %v", ca.ariIDs)
			}
		})
	}
}

// TestARIWindowInThePast: a CA asking for renewal now gets it for a
// certificate old enough, not for one younger than the minimum age.
func TestARIWindowInThePast(t *testing.T) {
	cases := []struct {
		name      string
		age       time.Duration
		wantRenew bool
	}{
		{"old enough", 10 * day, true},
		{"too young", time.Hour, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ca := newStubCA(t, []string{"http-01"})
			ca.renewalInfo = ca.ariURL()
			// In the past, and inside the validity of both certificates.
			start, end := time.Now().Add(-50*time.Minute).UTC().Truncate(time.Second), time.Now().Add(-30*time.Minute).UTC().Truncate(time.Second)
			ca.onRenewalInfo = window(start, end, "")
			dir := t.TempDir()
			leaf := issueFixture(t, ca, dir, "app", 90*day, tc.age, nil)

			m := NewManager(Options{CertDir: dir})
			changed, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app"))
			if err != nil {
				t.Fatal(err)
			}
			wantID, _ := ariCertID(leaf)
			if len(ca.ariIDs) != 1 || ca.ariIDs[0] != wantID {
				t.Errorf("ari requests = %v, want one for %s", ca.ariIDs, wantID)
			}
			if changed != tc.wantRenew || (ca.orders == 1) != tc.wantRenew {
				t.Fatalf("changed = %v, orders = %d; want renewal %v", changed, ca.orders, tc.wantRenew)
			}
			obs := m.CertObservations()[0]
			if tc.wantRenew {
				// The new certificate starts with no renewal information.
				if meta := mustMeta(t, dir, "app"); meta.ARI != nil || obs.RenewReason != RenewReasonLifetime {
					t.Errorf("after renewal: ari = %+v, reason = %q; want none, lifetime", meta.ARI, obs.RenewReason)
				}
				return
			}
			meta := mustMeta(t, dir, "app")
			a := meta.ARI
			if a == nil || !a.WindowStart.Equal(start) || !a.WindowEnd.Equal(end) || a.RenewAt.Before(start) || !a.RenewAt.Before(end) {
				t.Fatalf("ari state = %+v, want the window with a time inside it", a)
			}
			if a.ExplanationURL != "https://ca.example.com/why" {
				t.Errorf("explanationURL = %q", a.ExplanationURL)
			}
			if want := meta.IssuedAt.Add(72 * time.Hour); obs.RenewReason != RenewReasonARI || !obs.RenewAt.Equal(want) {
				t.Errorf("observation renewAt = %s (%s), want %s (ari, held back to the minimum age)", obs.RenewAt, obs.RenewReason, want)
			}
		})
	}
}

// TestARIWindowLaterThanTheLifetimeRule: ARI never delays a renewal.
func TestARIWindowLaterThanTheLifetimeRule(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	ca.renewalInfo = ca.ariURL()
	ca.onRenewalInfo = window(time.Now().Add(80*day), time.Now().Add(81*day), "")
	dir := t.TempDir()
	issueFixture(t, ca, dir, "app", 90*day, 10*day, nil)

	m := NewManager(Options{CertDir: dir})
	if changed, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app")); err != nil || changed {
		t.Fatalf("EnsureAll: changed = %v, err = %v", changed, err)
	}
	meta := mustMeta(t, dir, "app")
	obs := m.CertObservations()[0]
	if want := meta.NotAfter.Add(-30 * day); obs.RenewReason != RenewReasonLifetime || !obs.RenewAt.Equal(want) {
		t.Errorf("observation = %s (%s), want %s (lifetime)", obs.RenewAt, obs.RenewReason, want)
	}

	// Near expiry the lifetime rule renews even though the window is later.
	dir2 := t.TempDir()
	issueFixture(t, ca, dir2, "app", 90*day, 80*day, nil)
	m2 := NewManager(Options{CertDir: dir2})
	if changed, err := m2.EnsureAll(context.Background(), ariCertCfg(ca, "app")); err != nil || !changed {
		t.Fatalf("near expiry: changed = %v, err = %v; want renewed", changed, err)
	}
}

// TestARIPollFailureKeepsState: a failed poll keeps the previous window and
// chosen time, retries an hour later, and is not a renewal failure.
func TestARIPollFailureKeepsState(t *testing.T) {
	prevStart := time.Now().Add(40 * day).UTC().Truncate(time.Second)
	cases := map[string]func(http.ResponseWriter, string){
		"server error": func(w http.ResponseWriter, _ string) { w.WriteHeader(http.StatusInternalServerError) },
		"not found":    func(w http.ResponseWriter, _ string) { w.WriteHeader(http.StatusNotFound) },
		"bad json":     func(w http.ResponseWriter, _ string) { _, _ = w.Write([]byte(`{"suggestedWindow":`)) },
		"bad time": func(w http.ResponseWriter, _ string) {
			_, _ = w.Write([]byte(`{"suggestedWindow":{"start":"soon","end":"later"}}`))
		},
		"inverted window": window(time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour), ""),
		"empty window":    window(time.Now(), time.Now(), ""),
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			ca := newStubCA(t, []string{"http-01"})
			ca.renewalInfo = ca.ariURL()
			ca.onRenewalInfo = h
			dir := t.TempDir()
			old := ARIState{
				WindowStart: prevStart, WindowEnd: prevStart.Add(day), RenewAt: prevStart.Add(time.Hour),
				LastChecked: time.Now().Add(-7 * time.Hour).UTC().Truncate(time.Second), NextCheck: time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
				ExplanationURL: "https://ca.example.com/old",
			}
			issueFixture(t, ca, dir, "app", 90*day, 10*day, &old)

			m := NewManager(Options{CertDir: dir})
			before := time.Now()
			changed, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app"))
			after := time.Now()
			if err != nil || changed {
				t.Fatalf("EnsureAll: changed = %v, err = %v; want a quiet pass", changed, err)
			}
			if len(ca.ariIDs) != 1 {
				t.Fatalf("ari requests = %d, want 1", len(ca.ariIDs))
			}
			got := *mustMeta(t, dir, "app").ARI
			next := got.NextCheck
			got.NextCheck = old.NextCheck
			if got != old {
				t.Errorf("state = %+v, want the previous %+v", got, old)
			}
			if next.Before(before.Add(time.Hour).Truncate(time.Second)) || next.After(after.Add(time.Hour)) {
				t.Errorf("nextCheck = %s, want one hour after the poll", next)
			}
			if obs := m.CertObservations()[0]; obs.RenewFailures != 0 || obs.LastError != "" || !obs.LastAttempt.IsZero() {
				t.Errorf("observation = %+v, want no attempt or failure recorded", obs)
			}

			// The next check is an hour away: a pass now does not poll again.
			if _, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app")); err != nil {
				t.Fatal(err)
			}
			if len(ca.ariIDs) != 1 {
				t.Errorf("ari requests = %d after a pass before nextCheck, want still 1", len(ca.ariIDs))
			}
		})
	}
}

// TestARISameWindowKeepsChosenTime: the renewal time is drawn once per
// window; a changed window draws again inside the new one.
func TestARISameWindowKeepsChosenTime(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	ca.renewalInfo = ca.ariURL()
	start := time.Now().Add(40 * day).UTC().Truncate(time.Second)
	ca.onRenewalInfo = window(start, start.Add(day), "")
	dir := t.TempDir()
	issueFixture(t, ca, dir, "app", 90*day, 10*day, nil)
	m := NewManager(Options{CertDir: dir})
	cfg := ariCertCfg(ca, "app")

	// dueForCheck moves the next check into the past, as time passing would.
	dueForCheck := func() {
		meta := mustMeta(t, dir, "app")
		meta.ARI.NextCheck = time.Now().Add(-time.Minute)
		if err := saveMeta(dir, "app", *meta); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := m.EnsureAll(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	first := *mustMeta(t, dir, "app").ARI
	if first.RenewAt.Before(start) || !first.RenewAt.Before(start.Add(day)) {
		t.Fatalf("renewAt = %s, want inside [%s, %s)", first.RenewAt, start, start.Add(day))
	}
	if obs := m.CertObservations()[0]; obs.RenewReason != RenewReasonARI || !obs.RenewAt.Equal(first.RenewAt) {
		t.Errorf("observation = %s (%s), want the ARI time %s", obs.RenewAt, obs.RenewReason, first.RenewAt)
	}

	for i := 0; i < 5; i++ {
		dueForCheck()
		if _, err := m.EnsureAll(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		if again := mustMeta(t, dir, "app").ARI; !again.RenewAt.Equal(first.RenewAt) {
			t.Fatalf("poll %d of the same window: renewAt = %s, want unchanged %s", i+2, again.RenewAt, first.RenewAt)
		}
	}
	if len(ca.ariIDs) != 6 {
		t.Errorf("ari requests = %d, want 6", len(ca.ariIDs))
	}

	moved := start.Add(5 * day)
	ca.onRenewalInfo = window(moved, moved.Add(time.Hour), "")
	dueForCheck()
	if _, err := m.EnsureAll(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if a := mustMeta(t, dir, "app").ARI; a.RenewAt.Before(moved) || !a.RenewAt.Before(moved.Add(time.Hour)) {
		t.Errorf("changed window: renewAt = %s, want inside [%s, %s)", a.RenewAt, moved, moved.Add(time.Hour))
	}
}

// TestARIRetryAfterSetsNextCheck: the CA's Retry-After, clamped, decides the
// next poll.
func TestARIRetryAfterSetsNextCheck(t *testing.T) {
	for header, want := range map[string]time.Duration{"": 6 * time.Hour, "7200": 2 * time.Hour, "60": time.Hour, "2592000": 7 * day} {
		t.Run("retry-after "+header, func(t *testing.T) {
			ca := newStubCA(t, []string{"http-01"})
			ca.renewalInfo = ca.ariURL()
			start := time.Now().Add(40 * day)
			ca.onRenewalInfo = window(start, start.Add(day), header)
			dir := t.TempDir()
			issueFixture(t, ca, dir, "app", 90*day, 10*day, nil)
			m := NewManager(Options{CertDir: dir})
			if _, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app")); err != nil {
				t.Fatal(err)
			}
			a := mustMeta(t, dir, "app").ARI
			if got := a.NextCheck.Sub(a.LastChecked); got != want {
				t.Errorf("nextCheck - lastChecked = %s, want %s", got, want)
			}
		})
	}
}

// TestARINotPolledBeforeNextCheck: a certificate whose next check has not
// come is not polled, and an unissued certificate never is.
func TestARINotPolledBeforeNextCheck(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	ca.renewalInfo = ca.ariURL()
	ca.onRenewalInfo = window(time.Now().Add(40*day), time.Now().Add(41*day), "")
	dir := t.TempDir()
	issueFixture(t, ca, dir, "app", 90*day, 10*day, &ARIState{NextCheck: time.Now().Add(time.Hour)})
	m := NewManager(Options{CertDir: dir})
	if _, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app")); err != nil {
		t.Fatal(err)
	}
	if len(ca.ariIDs) != 0 || ca.directoryGets != 0 {
		t.Errorf("ari requests = %d, directory fetches = %d; want none before nextCheck", len(ca.ariIDs), ca.directoryGets)
	}
}

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		raw, base string
		want      bool
	}{
		{"https://ca.example.com/ari", "https://ca.example.com/directory", true},
		{"https://CA.example.com/ari", "https://ca.example.com/directory", true},
		{"https://ca.example.com:8443/ari", "https://ca.example.com:8443/directory", true},
		{"https://ca.example.com:8443/ari", "https://ca.example.com/directory", false},
		{"https://ca.example.com:1/ari", "https://ca.example.com:2/directory", false},
		{"http://ca.example.com/ari", "https://ca.example.com/directory", false},
		{"https://ari.example.com/ari", "https://ca.example.com/directory", false},
		{"/relative", "https://ca.example.com/directory", false},
	}
	for _, tc := range cases {
		if got := sameOrigin(tc.raw, tc.base); got != tc.want {
			t.Errorf("sameOrigin(%q, %q) = %v, want %v", tc.raw, tc.base, got, tc.want)
		}
	}
}

// TestARIEndpointOnAnotherPort: same scheme and host but another port is
// another origin.
func TestARIEndpointOnAnotherPort(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	other := newStubCA(t, []string{"http-01"})
	ca.renewalInfo = other.ariURL() // 127.0.0.1, another port
	other.onRenewalInfo = window(time.Now().Add(40*day), time.Now().Add(41*day), "")
	dir := t.TempDir()
	issueFixture(t, ca, dir, "app", 90*day, 10*day, nil)
	m := NewManager(Options{CertDir: dir})
	if _, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app")); err != nil {
		t.Fatal(err)
	}
	if len(other.ariIDs) != 0 || mustMeta(t, dir, "app").ARI != nil {
		t.Errorf("renewal information used from another port: requests %v", other.ariIDs)
	}
}

// TestARIUsesTheBundle: with acme.caBundle, renewal information is fetched
// from a directory that only the bundle's CA vouches for.
func TestARIUsesTheBundle(t *testing.T) {
	ca, root := newTLSStubCA(t, []string{"http-01"})
	ca.renewalInfo = ca.ariURL()
	start := time.Now().Add(40 * day).UTC().Truncate(time.Second)
	ca.onRenewalInfo = window(start, start.Add(day), "")
	dir := t.TempDir()
	issueFixture(t, ca, dir, "app", 90*day, 10*day, nil)
	cfg := ariCertCfg(ca, "app")
	cfg.Certificates[0].ACME.CABundle = root

	m := NewManager(Options{CertDir: dir})
	if _, err := m.EnsureAll(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	a := mustMeta(t, dir, "app").ARI
	if ca.directoryGets != 1 || len(ca.ariIDs) != 1 || a == nil || !a.WindowStart.Equal(start) {
		t.Errorf("directory fetches = %d, polls = %d, state = %+v; want both over the bundle's trust", ca.directoryGets, len(ca.ariIDs), a)
	}
}

// TestARIClientTimeout: renewal-information requests have their own 10s
// timeout, shorter than the ACME exchanges', and keep the certificate's
// trust, redirect refusal and body cap.
func TestARIClientTimeout(t *testing.T) {
	_, root := testServerCert(t)
	m := NewManager(Options{CertDir: t.TempDir()})
	pass := m.newARIPass()
	d := pass.directory(context.Background(), &model.ACMESpec{DirectoryURL: "http://127.0.0.1:1/directory", CABundle: root})
	if d.client == nil {
		t.Fatalf("no client built: %v", d.err)
	}
	if d.client.Timeout != 10*time.Second || acmeHTTPTimeout == 10*time.Second {
		t.Errorf("ari timeout = %s (acme %s), want 10s and shorter than the ACME one", d.client.Timeout, acmeHTTPTimeout)
	}
	if d.client.CheckRedirect == nil {
		t.Error("ari client follows redirects")
	}
	tr, ok := d.client.Transport.(cappedTransport)
	if !ok || tr.base.(*http.Transport).TLSClientConfig == nil {
		t.Errorf("ari client transport = %T, want the capped transport with the bundle's trust", d.client.Transport)
	}
}

// TestARIHangingEndpoint: a renewal-information endpoint that never answers
// costs one timeout per pass, not one per certificate.
func TestARIHangingEndpoint(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) }) // runs before the server closes
	ca.renewalInfo = ca.ariURL()
	var polls atomic.Int32
	ca.onRenewalInfo = func(http.ResponseWriter, string) {
		polls.Add(1)
		<-release
	}
	dir := t.TempDir()
	for _, n := range []string{"one", "two", "three"} {
		issueFixture(t, ca, dir, n, 90*day, 10*day, nil)
	}
	m := NewManager(Options{CertDir: dir})
	m.ariTimeout = 200 * time.Millisecond

	began := time.Now()
	if _, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "one", "two", "three")); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("pass took %s with a hanging endpoint, want about one timeout", took)
	}
	if n := polls.Load(); n != 1 {
		t.Errorf("polls = %d, want the rest skipped after the first transport error", n)
	}
	if a := mustMeta(t, dir, "one").ARI; a == nil || a.NextCheck.IsZero() {
		t.Errorf("first certificate: state = %+v, want a retry an hour later", a)
	}
	for _, n := range []string{"two", "three"} {
		if a := mustMeta(t, dir, n).ARI; a != nil {
			t.Errorf("%s: state = %+v, want untouched (polled on the next pass)", n, a)
		}
	}
}

// TestARIHangingDirectory: a directory that never answers is waited on once
// per pass.
func TestARIHangingDirectory(t *testing.T) {
	ca := newStubCA(t, []string{"http-01"})
	release := make(chan struct{})
	var fetches atomic.Int32
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		<-release
	}))
	t.Cleanup(hang.Close)
	t.Cleanup(func() { close(release) })
	dir := t.TempDir()
	cfg := model.Config{}
	for _, n := range []string{"one", "two"} {
		issueFixture(t, ca, dir, n, 90*day, 10*day, nil)
		cfg.Certificates = append(cfg.Certificates, model.Certificate{
			ObjectMeta: model.ObjectMeta{Name: n}, Type: model.CertTypeACME, Domains: []string{"app.example.com"},
			ACME: &model.ACMESpec{Email: "admin@example.com", DirectoryURL: hang.URL + "/directory"},
		})
	}
	m := NewManager(Options{CertDir: dir})
	m.ariTimeout = 200 * time.Millisecond
	began := time.Now()
	if _, err := m.EnsureAll(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took > 2*time.Second || fetches.Load() != 1 {
		t.Errorf("pass took %s with %d directory fetches, want one timeout", took, fetches.Load())
	}
}

// TestARIWindowClampedToValidity: a window reaching outside the leaf's
// validity is cut to it, so it is stored; one with nothing left inside is a
// failed poll.
func TestARIWindowClampedToValidity(t *testing.T) {
	cases := []struct {
		name      string
		body      func(leaf *x509.Certificate) string
		wantStart func(leaf *x509.Certificate) time.Time
		wantEnd   func(leaf *x509.Certificate) time.Time
		failed    bool
	}{
		{
			name: "start before NotBefore",
			body: func(l *x509.Certificate) string {
				return ariBody(l.NotBefore.Add(-5*day).Format(time.RFC3339), l.NotBefore.Add(50*day).Format(time.RFC3339))
			},
			wantStart: func(l *x509.Certificate) time.Time { return l.NotBefore },
			wantEnd:   func(l *x509.Certificate) time.Time { return l.NotBefore.Add(50 * day) },
		},
		{
			// Year 10000 once converted to UTC: not storable as JSON.
			name: "end after NotAfter, beyond JSON",
			body: func(l *x509.Certificate) string {
				return ariBody(l.NotBefore.Add(50*day).Format(time.RFC3339), "9999-12-31T23:00:00-05:00")
			},
			wantStart: func(l *x509.Certificate) time.Time { return l.NotBefore.Add(50 * day) },
			wantEnd:   func(l *x509.Certificate) time.Time { return l.NotAfter },
		},
		{
			name: "wholly after NotAfter",
			body: func(l *x509.Certificate) string {
				return ariBody(l.NotAfter.Add(day).Format(time.RFC3339), l.NotAfter.Add(2*day).Format(time.RFC3339))
			},
			failed: true,
		},
		{
			name: "wholly before NotBefore",
			body: func(l *x509.Certificate) string {
				return ariBody(l.NotBefore.Add(-2*day).Format(time.RFC3339), l.NotBefore.Add(-day).Format(time.RFC3339))
			},
			failed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ca := newStubCA(t, []string{"http-01"})
			ca.renewalInfo = ca.ariURL()
			dir := t.TempDir()
			// An hour old: the age guard keeps any time drawn inside the
			// clamped window from renewing the certificate during the test.
			leaf := issueFixture(t, ca, dir, "app", 90*day, time.Hour, nil)
			ca.onRenewalInfo = func(w http.ResponseWriter, _ string) { _, _ = w.Write([]byte(tc.body(leaf))) }
			m := NewManager(Options{CertDir: dir})
			if _, err := m.EnsureAll(context.Background(), ariCertCfg(ca, "app")); err != nil {
				t.Fatal(err)
			}
			a := mustMeta(t, dir, "app").ARI
			if a == nil {
				t.Fatal("no state saved")
			}
			if tc.failed {
				if !a.WindowStart.IsZero() || !a.RenewAt.IsZero() || time.Until(a.NextCheck) > time.Hour {
					t.Errorf("state = %+v, want a failed poll retried within the hour", a)
				}
				return
			}
			if !a.WindowStart.Equal(tc.wantStart(leaf)) || !a.WindowEnd.Equal(tc.wantEnd(leaf)) || a.RenewAt.Before(a.WindowStart) || !a.RenewAt.Before(a.WindowEnd) {
				t.Errorf("state = %+v, want window [%s, %s) with a time inside", a, tc.wantStart(leaf), tc.wantEnd(leaf))
			}
		})
	}
}

func ariBody(start, end string) string {
	return fmt.Sprintf(`{"suggestedWindow":{"start":%q,"end":%q}}`, start, end)
}
