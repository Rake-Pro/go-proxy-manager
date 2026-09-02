package api_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/acme"
	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// genLeafCert builds a self-signed leaf (signed by a throwaway CA generated for
// this call) with the given validity window, issuer common name and SANs, and
// returns the leaf as PEM. Hermetic: no real CA, no network.
func genLeafCert(t *testing.T, issuerCN string, notBefore, notAfter time.Time, sans []string) []byte {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: issuerCN},
		NotBefore:             notBefore.Add(-time.Hour),
		NotAfter:              notAfter.Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: sans[0]},
		DNSNames:     sans,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
}

// writeCustomCertFiles places a custom certificate's PEM (and a dummy key file
// - certStatus never reads it) directly under certDir, exactly where
// model.CustomCertSpec.CertFile/KeyFile resolve relative to the cert store.
func writeCustomCertFiles(t *testing.T, certDir, certFile, keyFile string, leafPEM []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(certDir, certFile), leafPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certDir, keyFile), []byte("dummy key"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeIssuedCert places an ACME-issued artifact (fullchain.pem, privkey.pem,
// meta.json) exactly where acme.IssuedCertPath/IssuedKeyPath and the manager's
// internal metaPath expect it, so acme.LoadIssuedMeta and certStatus see it as
// already issued.
func writeIssuedCert(t *testing.T, certDir, name string, leafPEM []byte, notAfter time.Time) {
	t.Helper()
	certPath := acme.IssuedCertPath(certDir, name)
	keyPath := acme.IssuedKeyPath(certDir, name)
	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, leafPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("dummy key"), 0o600); err != nil {
		t.Fatal(err)
	}
	meta := acme.Meta{
		Domains:      []string{name + ".example.com"},
		DirectoryURL: "https://ca.example.com/directory",
		IssuedAt:     time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	b, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	metaFile := filepath.Join(filepath.Dir(certPath), "meta.json")
	if err := os.WriteFile(metaFile, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// newCertHealthHandler builds an API handler over a fresh git-backed store
// rooted separately from certDir, with extra's other fields (ACME hooks, data
// plane hooks) carried through unchanged.
func newCertHealthHandler(t *testing.T, certDir string, extra api.Deps) http.Handler {
	t.Helper()
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	extra.Store = st
	extra.CertDir = certDir
	return api.New(extra)
}

type certStatusResp struct {
	Name          string     `json:"name"`
	State         string     `json:"state"`
	Issuer        string     `json:"issuer"`
	DaysRemaining *int       `json:"daysRemaining"`
	NotAfter      *time.Time `json:"notAfter"`
	SANs          []string   `json:"sans"`
	LastError     string     `json:"lastError"`
}

func TestCertificateStatusFields(t *testing.T) {
	certDir := t.TempDir()
	now := time.Now()

	validPEM := genLeafCert(t, "Test CA", now.Add(-24*time.Hour), now.Add(200*24*time.Hour), []string{"valid.example.com"})
	writeCustomCertFiles(t, certDir, "valid.pem", "valid-key.pem", validPEM)

	expiredPEM := genLeafCert(t, "Test CA", now.Add(-400*24*time.Hour), now.Add(-5*24*time.Hour), []string{"expired.example.com"})
	writeCustomCertFiles(t, certDir, "expired.pem", "expired-key.pem", expiredPEM)

	expiringPEM := genLeafCert(t, "Test CA", now.Add(-80*24*time.Hour), now.Add(10*24*time.Hour), []string{"expiring.example.com"})
	writeIssuedCert(t, certDir, "expiring", expiringPEM, now.Add(10*24*time.Hour))
	// "pending" is declared below but never issued: no acme/issued/pending dir.

	h := newCertHealthHandler(t, certDir, api.Deps{})

	certs := map[string]string{
		"valid":    `{"name":"valid","type":"custom","domains":["valid.example.com"],"custom":{"certFile":"valid.pem","keyFile":"valid-key.pem"}}`,
		"expired":  `{"name":"expired","type":"custom","domains":["expired.example.com"],"custom":{"certFile":"expired.pem","keyFile":"expired-key.pem"}}`,
		"expiring": `{"name":"expiring","type":"acme","domains":["expiring.example.com"],"acme":{"email":"a@example.com"}}`,
		"pending":  `{"name":"pending","type":"acme","domains":["pending.example.com"],"acme":{"email":"a@example.com"}}`,
	}
	for name, body := range certs {
		w := do(t, h, "PUT", "/certificates/"+name, body)
		if w.Code != http.StatusOK {
			t.Fatalf("PUT %s: %d: %s", name, w.Code, w.Body.String())
		}
	}

	cases := []struct {
		name         string
		wantState    string
		wantNotAfter bool
		wantDaysSign int // +1 want positive daysRemaining, -1 want negative, 0 don't check
		wantIssuer   string
	}{
		{name: "valid", wantState: api.CertStateValid, wantNotAfter: true, wantDaysSign: +1, wantIssuer: "Test CA"},
		{name: "expired", wantState: api.CertStateExpired, wantNotAfter: true, wantDaysSign: -1, wantIssuer: "Test CA"},
		{name: "expiring", wantState: api.CertStateExpiring, wantNotAfter: true, wantDaysSign: +1, wantIssuer: "Test CA"},
		{name: "pending", wantState: api.CertStatePending, wantNotAfter: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, h, "GET", "/certificates/"+tc.name, "")
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s: %d: %s", tc.name, w.Code, w.Body.String())
			}
			var got certStatusResp
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v: %s", err, w.Body.String())
			}
			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.State, tc.wantState)
			}
			if tc.wantNotAfter && got.NotAfter == nil {
				t.Error("notAfter missing, want present")
			}
			if !tc.wantNotAfter && got.NotAfter != nil {
				t.Errorf("notAfter = %v, want absent", got.NotAfter)
			}
			if tc.wantDaysSign != 0 {
				if got.DaysRemaining == nil {
					t.Fatal("daysRemaining missing")
				}
				if tc.wantDaysSign > 0 && *got.DaysRemaining <= 0 {
					t.Errorf("daysRemaining = %d, want positive", *got.DaysRemaining)
				}
				if tc.wantDaysSign < 0 && *got.DaysRemaining >= 0 {
					t.Errorf("daysRemaining = %d, want negative", *got.DaysRemaining)
				}
			}
			if tc.wantIssuer != "" && got.Issuer != tc.wantIssuer {
				t.Errorf("issuer = %q, want %q", got.Issuer, tc.wantIssuer)
			}
		})
	}

	// The list endpoint decorates every item the same way.
	w := do(t, h, "GET", "/certificates", "")
	var list []certStatusResp
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("GET /certificates returned %d items, want 4", len(list))
	}
	for _, c := range list {
		if c.State == "" {
			t.Errorf("cert %q: state missing from list response", c.Name)
		}
	}
}

func TestRenewCertificate(t *testing.T) {
	cases := []struct {
		name       string
		wired      bool
		renewErr   error
		wantStatus int
	}{
		{name: "accepted starts the order", wired: true, renewErr: nil, wantStatus: http.StatusOK},
		{name: "not found", wired: true, renewErr: acme.ErrCertNotFound, wantStatus: http.StatusNotFound},
		{name: "not acme", wired: true, renewErr: acme.ErrNotACME, wantStatus: http.StatusBadRequest},
		{name: "already in flight", wired: true, renewErr: acme.ErrRenewInFlight, wantStatus: http.StatusConflict},
		{name: "cooldown", wired: true, renewErr: acme.ErrRenewCooldown, wantStatus: http.StatusConflict},
		{name: "not wired", wired: false, wantStatus: http.StatusNotImplemented},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := api.Deps{}
			var calls int
			if tc.wired {
				deps.ACMERenewNow = func(_ context.Context, _ model.Config, name string) error {
					calls++
					if name != "app" {
						t.Errorf("renew called for %q, want app", name)
					}
					return tc.renewErr
				}
			}
			h := newCertHealthHandler(t, t.TempDir(), deps)

			w := do(t, h, "POST", "/certificates/app/renew", "")
			if w.Code != tc.wantStatus {
				t.Fatalf("POST renew: %d, want %d: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				var resp struct {
					Started bool `json:"started"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if !resp.Started {
					t.Error("started = false, want true")
				}
				if calls != 1 {
					t.Errorf("ACMERenewNow called %d times, want 1", calls)
				}
			}
		})
	}
}

// TestCertificateLastErrorRedactedForViewer covers R2-L2: the raw ACME failure
// text - which can embed a third party's HTTP response body, e.g. a DNS
// provider's rejection detail - must not reach a caller without the admin
// scope on GET /certificates or GET /certificates/{name}, exactly like GET
// /health already withholds it. A non-admin caller gets the same fixed
// classification; only an admin sees the message verbatim.
func TestCertificateLastErrorRedactedForViewer(t *testing.T) {
	certDir := t.TempDir()
	const rawErr = `certificate "app": dns-01 challenge: provider rejected TXT record: {"error":"invalid_credentials","account":"acct-98765"}`

	newHandler := func(admin bool) http.Handler {
		extra := api.Deps{
			ACMEObservations: func() []acme.CertObservation {
				return []acme.CertObservation{{Name: "app", LastAttempt: time.Now(), LastError: rawErr}}
			},
		}
		if !admin {
			extra.RequireScope = func(_ *http.Request, required string) error {
				if required == model.ScopeAdmin {
					return errors.New("the \"user\" role is read-only")
				}
				return nil
			}
		}
		return newCertHealthHandler(t, certDir, extra)
	}
	putCert := func(h http.Handler) {
		w := do(t, h, "PUT", "/certificates/app", `{"name":"app","type":"acme","domains":["app.example.com"],"acme":{"email":"a@example.com"}}`)
		if w.Code != http.StatusOK {
			t.Fatalf("PUT app: %d: %s", w.Code, w.Body.String())
		}
	}

	adminH := newHandler(true)
	putCert(adminH)
	adminBody := do(t, adminH, "GET", "/certificates/app", "").Body.String()
	if !strings.Contains(adminBody, "acct-98765") {
		t.Errorf("admin GET /certificates/app should carry the raw error, got: %s", adminBody)
	}

	viewerH := newHandler(false)
	putCert(viewerH)
	viewerBody := do(t, viewerH, "GET", "/certificates/app", "").Body.String()
	if strings.Contains(viewerBody, "acct-98765") || strings.Contains(viewerBody, "invalid_credentials") {
		t.Fatalf("viewer GET /certificates/app leaked the raw ACME error: %s", viewerBody)
	}
	if !strings.Contains(viewerBody, "dns-01 challenge or DNS provider failure") {
		t.Errorf("viewer GET /certificates/app = %s, want the classified failure", viewerBody)
	}

	// GET /certificates (the list route) decorates the same way.
	viewerList := do(t, viewerH, "GET", "/certificates", "").Body.String()
	if strings.Contains(viewerList, "acct-98765") {
		t.Fatalf("viewer GET /certificates leaked the raw ACME error: %s", viewerList)
	}
}

func TestHealth(t *testing.T) {
	certDir := t.TempDir()
	now := time.Now()
	expiringPEM := genLeafCert(t, "Test CA", now.Add(-time.Hour), now.Add(5*24*time.Hour), []string{"e.example.com"})
	writeIssuedCert(t, certDir, "expiring", expiringPEM, now.Add(5*24*time.Hour))

	deps := api.Deps{
		DataPlaneListening: func() (httpsListening, httpListening bool) { return true, false },
		UpstreamGroupSummary: func() []api.UpstreamGroupHealth {
			return []api.UpstreamGroupHealth{{Name: "g1", Healthy: 2, Unhealthy: 1}}
		},
		ACMELastRun: func() time.Time { return now },
		ACMEObservations: func() []acme.CertObservation {
			return []acme.CertObservation{{Name: "expiring", LastAttempt: now}}
		},
		Runtime: api.RuntimeConfig{HTTPAddr: ":80", HTTPSAddr: ":443"},
	}
	h := newCertHealthHandler(t, certDir, deps)

	w := do(t, h, "PUT", "/certificates/expiring", `{"name":"expiring","type":"acme","domains":["e.example.com"],"acme":{"email":"a@example.com"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT expiring: %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, "GET", "/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /health: %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		DataPlane struct {
			HTTPS struct {
				Listening bool   `json:"listening"`
				Addr      string `json:"addr"`
			} `json:"https"`
			HTTP struct {
				Listening bool   `json:"listening"`
				Addr      string `json:"addr"`
			} `json:"http"`
		} `json:"dataPlane"`
		Certificates struct {
			Total    int `json:"total"`
			Expiring int `json:"expiring"`
			Expired  int `json:"expired"`
			Error    int `json:"error"`
		} `json:"certificates"`
		UpstreamGroups []api.UpstreamGroupHealth `json:"upstreamGroups"`
		HARole         string                    `json:"haRole"`
		ConfigHead     string                    `json:"configHead"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}

	if !resp.DataPlane.HTTPS.Listening || resp.DataPlane.HTTPS.Addr != ":443" {
		t.Errorf("dataPlane.https = %+v, want listening at :443", resp.DataPlane.HTTPS)
	}
	if resp.DataPlane.HTTP.Listening || resp.DataPlane.HTTP.Addr != ":80" {
		t.Errorf("dataPlane.http = %+v, want not listening, addr :80", resp.DataPlane.HTTP)
	}
	if resp.Certificates.Total != 1 || resp.Certificates.Expiring != 1 {
		t.Errorf("certificates = %+v, want total=1 expiring=1", resp.Certificates)
	}
	if len(resp.UpstreamGroups) != 1 || resp.UpstreamGroups[0].Name != "g1" || resp.UpstreamGroups[0].Healthy != 2 || resp.UpstreamGroups[0].Unhealthy != 1 {
		t.Errorf("upstreamGroups = %+v", resp.UpstreamGroups)
	}
	if resp.HARole == "" {
		t.Error("haRole missing")
	}
	if resp.ConfigHead == "" {
		t.Error("configHead missing")
	}
}
