package acme

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func TestDNSName(t *testing.T) {
	cases := map[string]string{
		"example.com":      "_acme-challenge.example.com",
		"*.example.com":    "_acme-challenge.example.com",
		"app2.example.com": "_acme-challenge.app2.example.com",
	}
	for in, want := range cases {
		if got := dnsName(in); got != want {
			t.Errorf("dnsName(%q)=%q want %q", in, got, want)
		}
	}
}

func writeMeta(t *testing.T, certDir, name string, m Meta) {
	t.Helper()
	dir := issuedDir(certDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(metaPath(certDir, name), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNeedsRenewal(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir, RenewBefore: 30 * 24 * time.Hour})

	cert := model.Certificate{
		ObjectMeta: model.ObjectMeta{Name: "wild"},
		Type:       model.CertTypeACME,
		Domains:    []string{"*.example.com", "example.com"},
		ACME:       &model.ACMESpec{Email: "a@b.c", DNSProvider: "cf"},
	}

	// 1. Never issued -> needs renewal.
	if !m.needsRenewal(cert) {
		t.Error("never-issued cert should need renewal")
	}

	// 2. Healthy, far from expiry -> no renewal.
	writeMeta(t, dir, "wild", Meta{Domains: cert.Domains, NotAfter: time.Now().Add(60 * 24 * time.Hour)})
	if m.needsRenewal(cert) {
		t.Error("healthy cert should not need renewal")
	}

	// 3. Near expiry -> needs renewal.
	writeMeta(t, dir, "wild", Meta{Domains: cert.Domains, NotAfter: time.Now().Add(10 * 24 * time.Hour)})
	if !m.needsRenewal(cert) {
		t.Error("cert near expiry should need renewal")
	}

	// 4. Domain set changed -> needs renewal.
	writeMeta(t, dir, "wild", Meta{Domains: []string{"example.com"}, NotAfter: time.Now().Add(60 * 24 * time.Hour)})
	if !m.needsRenewal(cert) {
		t.Error("changed domain set should need renewal")
	}
}

type fakeLookuper struct {
	records map[string][]string
}

func (f fakeLookuper) LookupTXT(_ context.Context, name string) ([]string, error) {
	return f.records[name], nil
}

func TestWaitPropagation(t *testing.T) {
	records := []challengeRecord{
		{name: "_acme-challenge.example.com", value: "val-apex"},
		{name: "_acme-challenge.example.com", value: "val-wild"},
	}
	// Both values present at the shared name -> success.
	ok := fakeLookuper{records: map[string][]string{
		"_acme-challenge.example.com": {"val-apex", "val-wild", "unrelated"},
	}}
	if err := waitPropagation(context.Background(), records, ok, time.Second, 10*time.Millisecond); err != nil {
		t.Errorf("expected propagation success, got %v", err)
	}

	// Missing one value -> times out.
	partial := fakeLookuper{records: map[string][]string{
		"_acme-challenge.example.com": {"val-apex"},
	}}
	if err := waitPropagation(context.Background(), records, partial, 50*time.Millisecond, 10*time.Millisecond); err == nil {
		t.Error("expected timeout when a TXT value is missing")
	}
}

func TestAccountKeyPersistence(t *testing.T) {
	dir := t.TempDir()
	url := "https://acme.example/directory"
	k1, err := loadOrCreateAccountKey(dir, url, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(accountKeyPath(dir, url, "")); err != nil {
		t.Fatalf("account key not persisted: %v", err)
	}
	k2, err := loadOrCreateAccountKey(dir, url, "")
	if err != nil {
		t.Fatal(err)
	}
	if !k1.Equal(k2) {
		t.Error("expected the same account key to be reloaded, not regenerated")
	}
}

func TestEnsureAllUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(Options{CertDir: dir})
	cfg := model.Config{Certificates: []model.Certificate{{
		ObjectMeta: model.ObjectMeta{Name: "wild"},
		Type:       model.CertTypeACME,
		Domains:    []string{"*.example.com"},
		ACME:       &model.ACMESpec{Email: "a@b.c", DNSProvider: "missing"},
	}}}
	changed, err := m.EnsureAll(context.Background(), cfg)
	if changed {
		t.Error("no cert should have changed")
	}
	if err == nil {
		t.Error("expected an error for missing dns provider")
	}
}

func TestSaveAndLoadMeta(t *testing.T) {
	dir := t.TempDir()
	want := Meta{Domains: []string{"a.test"}, DirectoryURL: "u", NotAfter: time.Now().Add(time.Hour).Truncate(time.Second)}
	b, _ := json.Marshal(want)
	_ = os.MkdirAll(issuedDir(dir, "c"), 0o700)
	_ = os.WriteFile(metaPath(dir, "c"), b, 0o600)
	got, err := loadMeta(dir, "c")
	if err != nil {
		t.Fatal(err)
	}
	if got.DirectoryURL != "u" || len(got.Domains) != 1 {
		t.Fatalf("unexpected meta: %+v", got)
	}
}
