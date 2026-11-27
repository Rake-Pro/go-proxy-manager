package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// acme.caBundle decides which server gpm accepts as the ACME directory (and may
// name a file to read), so it sits behind the same admin gate as
// acme.directoryURL: setting or changing it needs admin, keeping or clearing
// it does not.
func TestCertificateACMEBundleNeedsAdmin(t *testing.T) {
	cert := func(domains, bundle string) string {
		b := ""
		if bundle != "" {
			b = `,"caBundle":"` + bundle + `"`
		}
		return `{"name":"b","type":"acme","domains":[` + domains + `],"acme":{"email":"a@example.com","challenge":"http-01"` + b + `}}`
	}
	const one, two = `"b.example.com"`, `"b.example.com","c.example.com"`
	st := sharedStore(t)
	admin := scopedHandlerOn(t, st, model.ScopeAdmin)
	writer := scopedHandlerOn(t, st, "certificates:write")

	if w := do(t, writer, "PUT", "/certificates/b", cert(one, "${FILE:/run/secrets/ca.pem}")); w.Code != http.StatusForbidden {
		t.Fatalf("create with caBundle on certificates:write: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/b", cert(one, "")); w.Code != http.StatusOK {
		t.Fatalf("a certificate without a bundle stays on certificates:write: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/b", cert(one, "${FILE:/run/secrets/ca.pem}")); w.Code != http.StatusForbidden {
		t.Fatalf("setting caBundle on certificates:write: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := do(t, admin, "PUT", "/certificates/b", cert(one, "${FILE:/run/secrets/ca.pem}")); w.Code != http.StatusOK {
		t.Fatalf("admin sets caBundle: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/b", cert(two, "${FILE:/run/secrets/ca.pem}")); w.Code != http.StatusOK {
		t.Fatalf("edit keeping caBundle: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/b", cert(two, "${FILE:/run/secrets/other-ca.pem}")); w.Code != http.StatusForbidden {
		t.Fatalf("changing caBundle on certificates:write: %d %s, want 403", w.Code, w.Body.String())
	}
	if w := do(t, writer, "PUT", "/certificates/b", cert(two, "")); w.Code != http.StatusOK {
		t.Fatalf("clearing caBundle stays on certificates:write: %d %s", w.Code, w.Body.String())
	}
}

// A revert restores a stored object without the guard, so the certificate
// revert route is admin-only: certificates:write is refused at the gate, admin
// gets past it (and fails on the made-up hash).
func TestCertificateRevertNeedsAdmin(t *testing.T) {
	st := sharedStore(t)
	admin := scopedHandlerOn(t, st, model.ScopeAdmin)
	writer := scopedHandlerOn(t, st, "certificates:write")
	const plain = `{"name":"rv","type":"acme","domains":["rv.example.com"],"acme":{"email":"a@example.com","challenge":"http-01"}}`
	if w := do(t, writer, "PUT", "/certificates/rv", plain); w.Code != http.StatusOK {
		t.Fatalf("seed: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, writer, "POST", "/certificates/rv/revert", `{"hash":"abcdef1"}`); w.Code != http.StatusForbidden {
		t.Fatalf("certificate revert on certificates:write: %d, want 403", w.Code)
	}
	if w := do(t, admin, "POST", "/certificates/rv/revert", `{"hash":"abcdef1"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("admin reaches the certificate revert: %d %s", w.Code, w.Body.String())
	}
}

// The lifetime-relative "expiring" window is the ACME manager's renewal rule,
// so it applies to ACME certificates only. A custom certificate, which nothing
// renews, keeps the fixed 30-day warning: a 6-day leaf with 5 days left reads
// valid as an ACME certificate and expiring as a custom one.
func TestExpiringStateSplitsACMEAndCustom(t *testing.T) {
	certDir := t.TempDir()
	now := time.Now()
	// Two leaves with the same validity (each certificate needs its own name).
	acmeLeaf := genLeafCert(t, "Test CA", now.Add(-24*time.Hour), now.Add(5*24*time.Hour), []string{"short-acme.example.com"})
	customLeaf := genLeafCert(t, "Test CA", now.Add(-24*time.Hour), now.Add(5*24*time.Hour), []string{"short-custom.example.com"})
	writeIssuedCert(t, certDir, "short-acme", acmeLeaf, now.Add(5*24*time.Hour))
	writeCustomCertFiles(t, certDir, "short.pem", "short-key.pem", customLeaf)

	h := newCertHealthHandler(t, certDir, api.Deps{})
	for name, body := range map[string]string{
		"short-acme":   `{"name":"short-acme","type":"acme","domains":["short-acme.example.com"],"acme":{"email":"a@example.com"}}`,
		"short-custom": `{"name":"short-custom","type":"custom","domains":["short-custom.example.com"],"custom":{"certFile":"short.pem","keyFile":"short-key.pem"}}`,
	} {
		if w := do(t, h, "PUT", "/certificates/"+name, body); w.Code != http.StatusOK {
			t.Fatalf("PUT %s: %d: %s", name, w.Code, w.Body.String())
		}
	}
	for name, want := range map[string]string{"short-acme": api.CertStateValid, "short-custom": api.CertStateExpiring} {
		var got map[string]any
		if err := json.Unmarshal(do(t, h, "GET", "/certificates/"+name, "").Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["state"] != want {
			t.Errorf("%s: state = %v, want %s", name, got["state"], want)
		}
	}
}
