package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// stubCA is a minimal RFC 8555 server: enough of the protocol for a full order
// (directory, nonce, account, order, authorization, challenge, finalize,
// certificate) to run against x/crypto/acme, with no signature verification.
// It exists so Manager.issue is exercised end to end - challenge selection, the
// HTTP-01 token store, EAB, and artifact persistence - without a real CA.
type stubCA struct {
	t      *testing.T
	srv    *httptest.Server
	caKey  *ecdsa.PrivateKey
	caCert *x509.Certificate

	// offer lists the challenge types the authorization advertises.
	offer []string
	// onAccept is called when a challenge is accepted, with its type and token.
	// Returning an error fails the order (the CA "could not validate").
	onAccept func(chalType, token string) error

	// captured request state
	newAccount   map[string]any
	acceptedType string
	acceptedTok  string
	csrDNSNames  []string
	authzValid   bool
}

func newStubCA(t *testing.T, offer []string) *stubCA {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "stub ACME CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	ca := &stubCA{t: t, caKey: caKey, caCert: caCert, offer: offer}
	ca.srv = httptest.NewServer(ca.handler())
	t.Cleanup(ca.srv.Close)
	return ca
}

func (c *stubCA) url(path string) string { return c.srv.URL + path }

// jwsPayload decodes the JSON payload of a JWS request body (signatures are not
// verified - the stub trusts its single test client).
func (c *stubCA) jwsPayload(r *http.Request) map[string]any {
	c.t.Helper()
	var jws struct {
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&jws); err != nil {
		c.t.Fatalf("decode jws: %v", err)
	}
	if jws.Payload == "" {
		return map[string]any{} // POST-as-GET
	}
	raw, err := base64.RawURLEncoding.DecodeString(jws.Payload)
	if err != nil {
		c.t.Fatalf("decode jws payload: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		c.t.Fatalf("unmarshal jws payload: %v", err)
	}
	return out
}

func (c *stubCA) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/directory", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(c.t, w, map[string]string{
			"newNonce":   c.url("/new-nonce"),
			"newAccount": c.url("/new-account"),
			"newOrder":   c.url("/new-order"),
			"revokeCert": c.url("/revoke"),
			"keyChange":  c.url("/key-change"),
		})
	})

	mux.HandleFunc("/new-nonce", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/new-account", func(w http.ResponseWriter, r *http.Request) {
		c.newAccount = c.jwsPayload(r)
		w.Header().Set("Location", c.url("/acct/1"))
		w.WriteHeader(http.StatusCreated)
		writeJSON(c.t, w, map[string]any{"status": "valid"})
	})

	mux.HandleFunc("/new-order", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", c.url("/order/1"))
		w.WriteHeader(http.StatusCreated)
		writeJSON(c.t, w, map[string]any{
			"status":         "pending",
			"authorizations": []string{c.url("/authz/1")},
			"finalize":       c.url("/finalize/1"),
		})
	})

	mux.HandleFunc("/authz/1", func(w http.ResponseWriter, r *http.Request) {
		status := "pending"
		if c.authzValid {
			status = "valid"
		}
		var challenges []map[string]any
		for _, typ := range c.offer {
			challenges = append(challenges, map[string]any{
				"type":   typ,
				"url":    c.url("/chal/" + typ),
				"token":  "tok-" + typ,
				"status": status,
			})
		}
		writeJSON(c.t, w, map[string]any{
			"identifier": map[string]string{"type": "dns", "value": "app.example.com"},
			"status":     status,
			"challenges": challenges,
		})
	})

	mux.HandleFunc("/chal/", func(w http.ResponseWriter, r *http.Request) {
		typ := strings.TrimPrefix(r.URL.Path, "/chal/")
		c.acceptedType = typ
		c.acceptedTok = "tok-" + typ
		if c.onAccept != nil {
			if err := c.onAccept(typ, c.acceptedTok); err != nil {
				w.WriteHeader(http.StatusForbidden)
				writeJSON(c.t, w, map[string]any{"type": "urn:ietf:params:acme:error:unauthorized", "detail": err.Error()})
				return
			}
		}
		c.authzValid = true
		writeJSON(c.t, w, map[string]any{"type": typ, "url": c.url(r.URL.Path), "token": c.acceptedTok, "status": "valid"})
	})

	mux.HandleFunc("/finalize/1", func(w http.ResponseWriter, r *http.Request) {
		payload := c.jwsPayload(r)
		csrB64, _ := payload["csr"].(string)
		csrDER, err := base64.RawURLEncoding.DecodeString(csrB64)
		if err != nil {
			c.t.Fatalf("decode csr: %v", err)
		}
		csr, err := x509.ParseCertificateRequest(csrDER)
		if err != nil {
			c.t.Fatalf("parse csr: %v", err)
		}
		c.csrDNSNames = csr.DNSNames
		writeJSON(c.t, w, map[string]any{"status": "valid", "certificate": c.url("/cert/1")})
	})

	mux.HandleFunc("/cert/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		_, _ = w.Write(c.issueChain())
	})

	// Every response carries a fresh nonce, as the protocol requires.
	return nonceMiddleware(mux)
}

func nonceMiddleware(h http.Handler) http.Handler {
	var n int
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.Header().Set("Replay-Nonce", base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("n", 8)+string(rune('a'+n%26)))))
		h.ServeHTTP(w, r)
	})
}

// issueChain signs a leaf for the CSR's names and returns leaf + CA as PEM.
func (c *stubCA) issueChain() []byte {
	c.t.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		c.t.Fatal(err)
	}
	names := c.csrDNSNames
	if len(names) == 0 {
		names = []string{"app.example.com"}
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: names[0]},
		DNSNames:     names,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.caCert, &leafKey.PublicKey, c.caKey)
	if err != nil {
		c.t.Fatal(err)
	}
	var buf []byte
	buf = append(buf, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	buf = append(buf, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.caCert.Raw})...)
	return buf
}

// recordingSolver captures the TXT records a dns-01 order provisions.
type recordingSolver struct {
	presented map[string]string
	cleaned   map[string]string
}

func newRecordingSolver() *recordingSolver {
	return &recordingSolver{presented: map[string]string{}, cleaned: map[string]string{}}
}

func (s *recordingSolver) Present(_ context.Context, name, value string) error {
	s.presented[name] = value
	return nil
}

func (s *recordingSolver) CleanUp(_ context.Context, name, value string) error {
	s.cleaned[name] = value
	return nil
}

func TestEnsureAllHTTP01EndToEnd(t *testing.T) {
	ca := newStubCA(t, []string{"http-01", "dns-01"})
	dir := t.TempDir()
	var changedCalls int
	m := NewManager(Options{CertDir: dir, OnChange: func() { changedCalls++ }})

	// The CA "validates" by looking the token up exactly where the data plane
	// would: the manager's in-flight HTTP-01 store.
	var servedKeyAuth string
	ca.onAccept = func(chalType, token string) error {
		if chalType != "http-01" {
			t.Errorf("accepted challenge type = %q, want http-01", chalType)
		}
		ka, ok := m.HTTP01Challenges().KeyAuth(token)
		if !ok {
			t.Errorf("token %q is not servable while the order is in flight", token)
		}
		servedKeyAuth = ka
		return nil
	}

	cfg := model.Config{Certificates: []model.Certificate{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Type:       model.CertTypeACME,
		Domains:    []string{"app.example.com"},
		ACME: &model.ACMESpec{
			Email:        "admin@example.com",
			DirectoryURL: ca.url("/directory"),
			EAB:          &model.EABSpec{KID: "kid-42", HMACKey: model.Secret(base64.RawURLEncoding.EncodeToString([]byte("hmac-key")))},
		},
	}}}

	changed, err := m.EnsureAll(context.Background(), cfg)
	if err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	if !changed || changedCalls != 1 {
		t.Errorf("changed = %v, OnChange calls = %d; want true, 1", changed, changedCalls)
	}

	// 1. Challenge selection: no dnsProvider -> http-01, even though the CA also
	// offered dns-01.
	if ca.acceptedType != "http-01" {
		t.Errorf("accepted challenge = %q, want http-01", ca.acceptedType)
	}
	if servedKeyAuth == "" || !strings.HasPrefix(servedKeyAuth, ca.acceptedTok+".") {
		t.Errorf("served key authorization = %q, want %q.<thumbprint>", servedKeyAuth, ca.acceptedTok)
	}

	// 2. The token is dropped once the order finishes.
	if _, ok := m.HTTP01Challenges().KeyAuth(ca.acceptedTok); ok {
		t.Error("HTTP-01 token outlived its order")
	}

	// 3. EAB reached the new-account request.
	eab, ok := ca.newAccount["externalAccountBinding"].(map[string]any)
	if !ok {
		t.Fatalf("new-account payload has no externalAccountBinding: %v", ca.newAccount)
	}
	protected, _ := eab["protected"].(string)
	raw, err := base64.RawURLEncoding.DecodeString(protected)
	if err != nil {
		t.Fatalf("decode eab protected header: %v", err)
	}
	if !strings.Contains(string(raw), `"kid":"kid-42"`) {
		t.Errorf("eab protected header = %s, want kid-42", raw)
	}

	// 4. Artifacts persisted for the data plane, with matching metadata.
	if _, err := os.Stat(IssuedCertPath(dir, "app")); err != nil {
		t.Errorf("fullchain.pem not written: %v", err)
	}
	if _, err := os.Stat(IssuedKeyPath(dir, "app")); err != nil {
		t.Errorf("privkey.pem not written: %v", err)
	}
	meta, err := loadMeta(dir, "app")
	if err != nil {
		t.Fatalf("loadMeta: %v", err)
	}
	if len(meta.Domains) != 1 || meta.Domains[0] != "app.example.com" {
		t.Errorf("meta domains = %v", meta.Domains)
	}
	if meta.NotAfter.Before(time.Now()) {
		t.Errorf("meta notAfter = %s, want a future expiry", meta.NotAfter)
	}

	// 5. A second pass is a no-op: the cert is fresh.
	changed, err = m.EnsureAll(context.Background(), cfg)
	if err != nil || changed {
		t.Errorf("second EnsureAll: changed = %v, err = %v; want false, nil", changed, err)
	}
}

func TestEnsureAllDNS01EndToEnd(t *testing.T) {
	ca := newStubCA(t, []string{"http-01", "dns-01"})
	dir := t.TempDir()
	solver := newRecordingSolver()
	m := NewManager(Options{
		CertDir:   dir,
		Resolver:  nil,
		NewSolver: func(model.DNSProvider) (DNSSolver, error) { return solver, nil },
	})
	// Propagation is checked against the solver's own view in this test.
	m.resolver = stubTXTLookuper{solver}
	m.propagationInterval = time.Millisecond

	ca.onAccept = func(chalType, token string) error {
		if chalType != "dns-01" {
			t.Errorf("accepted challenge type = %q, want dns-01", chalType)
		}
		if len(solver.presented) != 1 {
			t.Errorf("TXT record not provisioned before validation: %v", solver.presented)
		}
		return nil
	}

	cfg := model.Config{
		DNSProviders: []model.DNSProvider{{
			ObjectMeta: model.ObjectMeta{Name: "cf"},
			Provider:   "cloudflare",
			Config:     map[string]model.Secret{"apiToken": "t"},
		}},
		Certificates: []model.Certificate{{
			ObjectMeta: model.ObjectMeta{Name: "app"},
			Type:       model.CertTypeACME,
			Domains:    []string{"app.example.com"},
			ACME: &model.ACMESpec{
				Email:        "admin@example.com",
				DirectoryURL: ca.url("/directory"),
				DNSProvider:  "cf", // no explicit challenge: dns-01 by back-compat
			},
		}},
	}

	changed, err := m.EnsureAll(context.Background(), cfg)
	if err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	if !changed {
		t.Error("expected the certificate to be issued")
	}
	if ca.acceptedType != "dns-01" {
		t.Errorf("accepted challenge = %q, want dns-01", ca.acceptedType)
	}
	if _, ok := solver.presented["_acme-challenge.app.example.com"]; !ok {
		t.Errorf("presented records = %v", solver.presented)
	}
	if _, ok := solver.cleaned["_acme-challenge.app.example.com"]; !ok {
		t.Errorf("TXT record not cleaned up: %v", solver.cleaned)
	}
	// No HTTP-01 token is ever registered on a dns-01 order.
	if n := m.HTTP01Challenges().Len(); n != 0 {
		t.Errorf("http-01 store holds %d tokens on a dns-01 order", n)
	}
}

// stubTXTLookuper resolves TXT records straight out of the recording solver, so
// propagation "succeeds" as soon as the record is provisioned.
type stubTXTLookuper struct{ s *recordingSolver }

func (l stubTXTLookuper) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := l.s.presented[name]; ok {
		return []string{v}, nil
	}
	return nil, nil
}
