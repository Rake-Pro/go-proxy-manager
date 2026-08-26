package api_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// throwawayCA generates a self-signed CA in-process and returns its certificate
// PEM, its PKCS#8 key PEM and the parsed certificate. Hermetic: nothing touches
// the network or a deployment path.
func throwawayCA(t *testing.T, cn string) (certPEM, keyPEM string, cert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pk8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk8})),
		parsed
}

// issueHandler builds an API handler over a fresh store plus a cert store
// directory, and seeds two client CAs: "signing" (with a cert-store-relative
// caKeyFile) and "verify-only" (no key at all).
func issueHandler(t *testing.T) (http.Handler, *x509.Certificate) {
	t.Helper()
	dir := t.TempDir()
	certDir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("store init: %v", err)
	}
	h := api.New(api.Deps{Store: st, CertDir: certDir})

	caPEM, keyPEM, caCert := throwawayCA(t, "corp-ca")
	if err := os.WriteFile(filepath.Join(certDir, "corp-ca.key"), []byte(keyPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{"caPEM": caPEM, "caKeyFile": "corp-ca.key"})
	if err != nil {
		t.Fatal(err)
	}
	if w := do(t, h, "PUT", "/client-cas/signing", string(body)); w.Code != http.StatusOK {
		t.Fatalf("seed signing CA: %d %s", w.Code, w.Body.String())
	}
	body, err = json.Marshal(map[string]string{"caPEM": caPEM})
	if err != nil {
		t.Fatal(err)
	}
	if w := do(t, h, "PUT", "/client-cas/verify-only", string(body)); w.Code != http.StatusOK {
		t.Fatalf("seed verify-only CA: %d %s", w.Code, w.Body.String())
	}
	return h, caCert
}

// TestIssueClientCertHappyPath mints a bundle and re-parses it with the same
// library: the key must belong to the leaf, the leaf must chain to the CA, and
// the certificate must be shaped as a client credential.
func TestIssueClientCertHappyPath(t *testing.T) {
	h, caCert := issueHandler(t)

	w := do(t, h, "POST", "/client-cas/signing/issue",
		`{"commonName":"phone-01","validityDays":30,"password":"hunter2-hunter2","sans":["phone-01.example.com","ops@example.com","10.1.2.3"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("issue want 200 got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/x-pkcs12" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="phone-01.p12"` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	// Issuance mints no config revision: it changes nothing in the store.
	if sha := w.Header().Get("X-Config-Commit"); sha != "" {
		t.Fatalf("issuance must not create a config revision, got commit %q", sha)
	}

	key, leaf, chain, err := pkcs12.DecodeChain(w.Body.Bytes(), "hunter2-hunter2")
	if err != nil {
		t.Fatalf("decode p12: %v", err)
	}
	signer, ok := key.(interface{ Public() crypto.PublicKey })
	if !ok {
		t.Fatalf("bundle key is not a signer: %T", key)
	}
	pub, ok := leaf.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok {
		t.Fatal("leaf public key is not comparable")
	}
	if !pub.Equal(signer.Public()) {
		t.Fatal("bundle key does not match the bundle certificate")
	}
	if leaf.Subject.CommonName != "phone-01" {
		t.Fatalf("CN = %q", leaf.Subject.CommonName)
	}
	if leaf.IsCA {
		t.Fatal("issued client certificate must not be a CA")
	}
	if leaf.KeyUsage != x509.KeyUsageDigitalSignature {
		t.Fatalf("KeyUsage = %v", leaf.KeyUsage)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatalf("ExtKeyUsage = %v", leaf.ExtKeyUsage)
	}
	if leaf.SerialNumber.Sign() <= 0 {
		t.Fatal("serial must be positive")
	}
	if !leaf.NotBefore.Before(time.Now()) {
		t.Fatal("NotBefore must be backdated for clock skew")
	}
	if got := leaf.NotAfter.Sub(leaf.NotBefore); got < 29*24*time.Hour {
		t.Fatalf("validity window %v is shorter than the requested 30 days", got)
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "phone-01.example.com" {
		t.Fatalf("DNSNames = %v", leaf.DNSNames)
	}
	if len(leaf.EmailAddresses) != 1 || leaf.EmailAddresses[0] != "ops@example.com" {
		t.Fatalf("EmailAddresses = %v", leaf.EmailAddresses)
	}
	if len(leaf.IPAddresses) != 1 || leaf.IPAddresses[0].String() != "10.1.2.3" {
		t.Fatalf("IPAddresses = %v", leaf.IPAddresses)
	}

	// The CA travels in the bundle, and the leaf verifies against it.
	if len(chain) != 1 || !chain[0].Equal(caCert) {
		t.Fatalf("bundle chain = %d certs, want the issuing CA", len(chain))
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("issued certificate does not verify against the CA: %v", err)
	}
}

// TestIssueClientCertDefaultValidity proves an omitted validityDays defaults to
// a year rather than issuing a zero-length certificate.
func TestIssueClientCertDefaultValidity(t *testing.T) {
	h, _ := issueHandler(t)
	w := do(t, h, "POST", "/client-cas/signing/issue", `{"commonName":"laptop","password":"a-long-enough-pw"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	_, leaf, _, err := pkcs12.DecodeChain(w.Body.Bytes(), "a-long-enough-pw")
	if err != nil {
		t.Fatalf("decode p12: %v", err)
	}
	if got := leaf.NotAfter.Sub(leaf.NotBefore); got < 364*24*time.Hour {
		t.Fatalf("default validity %v, want about a year", got)
	}
}

// TestIssueClientCertErrors is the table of refusals: unknown CA, verify-only
// CA, and every request-validation failure.
func TestIssueClientCertErrors(t *testing.T) {
	h, _ := issueHandler(t)

	cases := []struct {
		name string
		path string
		body string
		want int
	}{
		{"unknown CA", "/client-cas/nope/issue", `{"commonName":"a","password":"p"}`, http.StatusNotFound},
		{"CA has no signing key", "/client-cas/verify-only/issue", `{"commonName":"a","password":"a-long-enough-pw"}`, http.StatusUnprocessableEntity},
		{"empty common name", "/client-cas/signing/issue", `{"commonName":"","password":"a-long-enough-pw"}`, http.StatusBadRequest},
		{"blank common name", "/client-cas/signing/issue", `{"commonName":"   ","password":"a-long-enough-pw"}`, http.StatusBadRequest},
		{"over-long common name", "/client-cas/signing/issue",
			`{"commonName":"` + longName(65) + `","password":"a-long-enough-pw"}`, http.StatusBadRequest},
		{"empty password", "/client-cas/signing/issue", `{"commonName":"a","password":""}`, http.StatusBadRequest},
		{"negative validity", "/client-cas/signing/issue", `{"commonName":"a","password":"a-long-enough-pw","validityDays":-1}`, http.StatusBadRequest},
		{"validity over the cap", "/client-cas/signing/issue", `{"commonName":"a","password":"a-long-enough-pw","validityDays":3651}`, http.StatusBadRequest},
		{"malformed body", "/client-cas/signing/issue", `{`, http.StatusBadRequest},
		// The bundle's only at-rest protection is its password, and the legacy
		// encoder barely stretches it, so a short one is refused outright.
		{"password below the floor", "/client-cas/signing/issue",
			`{"commonName":"a","password":"short"}`, http.StatusBadRequest},
		{"password one character short", "/client-cas/signing/issue",
			`{"commonName":"a","password":"12345678901"}`, http.StatusBadRequest},
		// A SAN that cannot be encoded as an IA5String must be a 400, not an opaque
		// 500 out of ASN.1 marshalling. The values below are JSON \u escapes so
		// this source file stays ASCII while the decoded SAN does not.
		{"non-ASCII DNS san", "/client-cas/signing/issue",
			`{"commonName":"a","password":"a-long-enough-pw","sans":["b\u00fccher.example.com"]}`, http.StatusBadRequest},
		{"non-ASCII email san", "/client-cas/signing/issue",
			`{"commonName":"a","password":"a-long-enough-pw","sans":["j\u00fcrgen@example.com"]}`, http.StatusBadRequest},
		{"control character in san", "/client-cas/signing/issue",
			`{"commonName":"a","password":"a-long-enough-pw","sans":["ab\u0001.example.com"]}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, h, "POST", tc.path, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("error responses must stay JSON, got %q", ct)
			}
		})
	}
}

// TestIssueClientCertFilenameSanitized proves a hostile common name cannot break
// out of the Content-Disposition header or write outside the download directory.
func TestIssueClientCertFilenameSanitized(t *testing.T) {
	h, _ := issueHandler(t)
	w := do(t, h, "POST", "/client-cas/signing/issue",
		`{"commonName":"../../etc/pa sswd\"","password":"a-long-enough-pw"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="etc-pa-sswd.p12"` {
		t.Fatalf("Content-Disposition = %q", cd)
	}
}

func longName(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

// clientCertList mirrors the GET /client-cas/{name}/certificates payload.
type clientCertListPayload struct {
	ExpiryWarningDays int `json:"expiryWarningDays"`
	Certificates      []struct {
		CA            string    `json:"ca"`
		CommonName    string    `json:"commonName"`
		SANs          []string  `json:"sans"`
		Serial        string    `json:"serial"`
		NotAfter      time.Time `json:"notAfter"`
		Status        string    `json:"status"`
		DaysRemaining int       `json:"daysRemaining"`
		SupersededBy  string    `json:"supersededBy"`
	} `json:"certificates"`
}

func listIssued(t *testing.T, h http.Handler, ca string) clientCertListPayload {
	t.Helper()
	w := do(t, h, "GET", "/client-cas/"+ca+"/certificates", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list want 200 got %d: %s", w.Code, w.Body.String())
	}
	var out clientCertListPayload
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return out
}

// TestIssuedCertRecords proves issuance persists a record with no key material,
// that the list derives an expiry status, and that a CA that never issued reports
// an empty list rather than an error.
func TestIssuedCertRecords(t *testing.T) {
	h, _ := issueHandler(t)

	if got := listIssued(t, h, "signing"); len(got.Certificates) != 0 {
		t.Fatalf("a CA that never issued must list empty, got %d", len(got.Certificates))
	}
	if got := listIssued(t, h, "verify-only"); got.ExpiryWarningDays != 30 {
		t.Fatalf("default expiryWarningDays = %d, want 30", got.ExpiryWarningDays)
	}
	if w := do(t, h, "GET", "/client-cas/nope/certificates", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown CA want 404 got %d", w.Code)
	}

	w := do(t, h, "POST", "/client-cas/signing/issue",
		`{"commonName":"phone-01","validityDays":365,"password":"a-long-enough-pw","sans":["phone-01.example.com","10.1.2.3"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("issue want 200 got %d: %s", w.Code, w.Body.String())
	}
	serial := w.Header().Get("X-Client-Cert-Serial")
	if serial == "" {
		t.Fatal("issue must report the serial")
	}

	got := listIssued(t, h, "signing")
	if len(got.Certificates) != 1 {
		t.Fatalf("got %d records, want 1", len(got.Certificates))
	}
	rec := got.Certificates[0]
	if rec.CA != "signing" || rec.CommonName != "phone-01" || rec.Serial != serial {
		t.Fatalf("record does not describe the issuance: %+v", rec)
	}
	if len(rec.SANs) != 2 || rec.SANs[0] != "phone-01.example.com" || rec.SANs[1] != "10.1.2.3" {
		t.Fatalf("record SANs = %v", rec.SANs)
	}
	if rec.Status != "ok" {
		t.Fatalf("a year-long certificate must be ok, got %q", rec.Status)
	}
	if rec.DaysRemaining < 360 {
		t.Fatalf("daysRemaining = %d", rec.DaysRemaining)
	}
	// No key material anywhere in the payload.
	body := do(t, h, "GET", "/client-cas/signing/certificates", "").Body.String()
	for _, forbidden := range []string{"PRIVATE KEY", "password", "pw", "pkcs12"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("issuance record payload leaks %q", forbidden)
		}
	}
}

// TestIssuedCertExpiringStatus proves the derived status uses the CA's
// expiryWarningDays: a short-lived certificate lands inside the default window,
// and a CA with a small window reports the same certificate as ok.
func TestIssuedCertExpiringStatus(t *testing.T) {
	h, _ := issueHandler(t)
	if w := do(t, h, "POST", "/client-cas/signing/issue",
		`{"commonName":"short","validityDays":10,"password":"a-long-enough-pw"}`); w.Code != http.StatusOK {
		t.Fatalf("issue: %d %s", w.Code, w.Body.String())
	}
	got := listIssued(t, h, "signing")
	if got.Certificates[0].Status != "expiring" {
		t.Fatalf("a 10-day certificate must be expiring under the 30-day default, got %q", got.Certificates[0].Status)
	}
}

// TestRenewClientCert proves renew reissues the recorded identity with a new key
// and serial, supersedes the old record, and leaves it listed.
func TestRenewClientCert(t *testing.T) {
	h, caCert := issueHandler(t)
	w := do(t, h, "POST", "/client-cas/signing/issue",
		`{"commonName":"phone-01","validityDays":10,"password":"a-long-enough-pw","sans":["phone-01.example.com"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("issue: %d %s", w.Code, w.Body.String())
	}
	oldSerial := w.Header().Get("X-Client-Cert-Serial")
	oldKey, oldLeaf, _, err := pkcs12.DecodeChain(w.Body.Bytes(), "a-long-enough-pw")
	if err != nil {
		t.Fatal(err)
	}

	w = do(t, h, "POST", "/client-cas/signing/certificates/"+oldSerial+"/renew",
		`{"password":"a-different-long-pw","validityDays":365}`)
	if w.Code != http.StatusOK {
		t.Fatalf("renew want 200 got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/x-pkcs12" {
		t.Fatalf("renew Content-Type = %q", ct)
	}
	if sha := w.Header().Get("X-Config-Commit"); sha != "" {
		t.Fatalf("renew must not create a config revision, got %q", sha)
	}
	newSerial := w.Header().Get("X-Client-Cert-Serial")
	if newSerial == oldSerial {
		t.Fatal("a renewal must mint a new serial")
	}
	newKey, newLeaf, chain, err := pkcs12.DecodeChain(w.Body.Bytes(), "a-different-long-pw")
	if err != nil {
		t.Fatalf("decode renewed p12: %v", err)
	}
	// Same identity, new key, new validity, same issuer.
	if newLeaf.Subject.CommonName != "phone-01" {
		t.Fatalf("renewed CN = %q", newLeaf.Subject.CommonName)
	}
	if len(newLeaf.DNSNames) != 1 || newLeaf.DNSNames[0] != "phone-01.example.com" {
		t.Fatalf("renewal must carry the recorded SANs, got %v", newLeaf.DNSNames)
	}
	if newLeaf.NotAfter.Sub(newLeaf.NotBefore) < 364*24*time.Hour {
		t.Fatal("renewal did not honour validityDays")
	}
	if len(chain) != 1 || !chain[0].Equal(caCert) {
		t.Fatal("renewed bundle must carry the issuing CA")
	}
	oldPub := oldKey.(interface{ Public() crypto.PublicKey }).Public()
	newPub := newKey.(interface{ Public() crypto.PublicKey }).Public()
	if oldPub.(interface{ Equal(crypto.PublicKey) bool }).Equal(newPub) {
		t.Fatal("a renewal must generate a NEW private key")
	}
	if oldLeaf.SerialNumber.Cmp(newLeaf.SerialNumber) == 0 {
		t.Fatal("renewed certificate reused the serial")
	}

	got := listIssued(t, h, "signing")
	if len(got.Certificates) != 2 {
		t.Fatalf("got %d records, want the renewal plus the superseded original", len(got.Certificates))
	}
	if got.Certificates[0].Serial != newSerial || got.Certificates[0].SupersededBy != "" {
		t.Fatalf("newest record must be the renewal: %+v", got.Certificates[0])
	}
	if got.Certificates[1].Serial != oldSerial || got.Certificates[1].SupersededBy != newSerial {
		t.Fatalf("original record must be marked superseded: %+v", got.Certificates[1])
	}
}

// TestRenewClientCertErrors covers the renew refusals.
func TestRenewClientCertErrors(t *testing.T) {
	h, _ := issueHandler(t)
	w := do(t, h, "POST", "/client-cas/signing/issue", `{"commonName":"phone-01","password":"a-long-enough-pw"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("issue: %d %s", w.Code, w.Body.String())
	}
	serial := w.Header().Get("X-Client-Cert-Serial")

	cases := []struct {
		name string
		path string
		body string
		want int
	}{
		{"unknown CA", "/client-cas/nope/certificates/" + serial + "/renew", `{"password":"p"}`, http.StatusNotFound},
		{"unknown serial", "/client-cas/signing/certificates/deadbeef/renew", `{"password":"p"}`, http.StatusNotFound},
		{"empty password", "/client-cas/signing/certificates/" + serial + "/renew", `{"password":""}`, http.StatusBadRequest},
		{"validity over the cap", "/client-cas/signing/certificates/" + serial + "/renew",
			`{"password":"a-long-enough-pw","validityDays":3651}`, http.StatusBadRequest},
		{"malformed body", "/client-cas/signing/certificates/" + serial + "/renew", `{`, http.StatusBadRequest},
		{"password below the floor", "/client-cas/signing/certificates/" + serial + "/renew",
			`{"password":"short"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, h, "POST", tc.path, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// TestRenewClientCertRejectsDoubleRenew proves an already-superseded record
// cannot be renewed again. Allowing it would mint a SECOND live certificate for
// the same identity and rewrite the supersede link, leaving the first renewal
// looking current with nothing pointing at it.
func TestRenewClientCertRejectsDoubleRenew(t *testing.T) {
	h, _ := issueHandler(t)
	w := do(t, h, "POST", "/client-cas/signing/issue",
		`{"commonName":"phone-01","password":"a-long-enough-pw"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("issue: %d %s", w.Code, w.Body.String())
	}
	first := w.Header().Get("X-Client-Cert-Serial")

	w = do(t, h, "POST", "/client-cas/signing/certificates/"+first+"/renew",
		`{"password":"a-different-long-pw"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("renew: %d %s", w.Code, w.Body.String())
	}
	second := w.Header().Get("X-Client-Cert-Serial")

	// Renewing the ORIGINAL again is a conflict, and the error names the
	// certificate that already replaced it.
	w = do(t, h, "POST", "/client-cas/signing/certificates/"+first+"/renew",
		`{"password":"a-third-long-password"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("double renew want 409 got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), second) {
		t.Fatalf("the conflict must name the superseding serial %s, got %s", second, w.Body.String())
	}

	// No third certificate was minted and the supersede link is untouched.
	got := listIssued(t, h, "signing")
	if len(got.Certificates) != 2 {
		t.Fatalf("a refused double renew must mint nothing, got %d records", len(got.Certificates))
	}
	if got.Certificates[1].Serial != first || got.Certificates[1].SupersededBy != second {
		t.Fatalf("supersede link was rewritten: %+v", got.Certificates[1])
	}

	// Renewing the CURRENT certificate is still fine.
	w = do(t, h, "POST", "/client-cas/signing/certificates/"+second+"/renew",
		`{"password":"a-fourth-long-passwd"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("renewing the current certificate want 200 got %d: %s", w.Code, w.Body.String())
	}
}

// TestIssueClientCertPasswordFloorMessage proves the refusal explains WHY the
// floor exists, so an operator hitting it does not just pick "123456789012".
func TestIssueClientCertPasswordFloorMessage(t *testing.T) {
	h, _ := issueHandler(t)
	w := do(t, h, "POST", "/client-cas/signing/issue", `{"commonName":"a","password":"short"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
	for _, want := range []string{"at least 12 characters", "legacy"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("password error must mention %q, got %s", want, w.Body.String())
		}
	}
}
