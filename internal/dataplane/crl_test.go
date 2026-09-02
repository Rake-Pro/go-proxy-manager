package dataplane

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// crlCA generates a self-signed CA that may sign CRLs (KeyUsageCRLSign, which
// x509.CreateRevocationList and CheckSignatureFrom both require).
func crlCA(t *testing.T, cn string) (caPEM string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), cert, key
}

// clientCertSerial issues a client certificate with an explicit serial and CN so
// a test can revoke exactly one of several certificates.
func clientCertSerial(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, serial int64, cn string) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"Corp"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{cn + ".corp.example"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, leaf
}

// makeCRL returns a PEM-encoded CRL revoking the given serials, signed by the CA.
func makeCRL(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, nextUpdate time.Time, serials ...int64) []byte {
	t.Helper()
	var entries []x509.RevocationListEntry
	for _, s := range serials {
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   big.NewInt(s),
			RevocationTime: time.Now().Add(-time.Minute),
		})
	}
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:                    big.NewInt(1),
		ThisUpdate:                time.Now().Add(-time.Hour),
		NextUpdate:                nextUpdate,
		RevokedCertificateEntries: entries,
	}, caCert, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der})
}

// crlRouterConfig builds a one-host mTLS config whose ClientCA carries the given
// CRL settings.
func crlRouterConfig(caPEM string, ca model.ClientCA) model.Config {
	ca.Name = "corp"
	ca.CAPEM = caPEM
	return model.Config{
		ClientCAs: []model.ClientCA{ca},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "m"},
			Domains:    []string{"m.example"},
			Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
			TLS: model.TLSSettings{
				ForceSSL:   true,
				ClientAuth: &model.ClientAuth{CARef: "corp", Mode: "require"},
			},
		}},
	}
}

// serverCfgFor returns the host's compiled per-SNI config with a usable server
// certificate, so a handshake exercises the real VerifyPeerCertificate wiring.
func serverCfgFor(t *testing.T, rt *router) *tls.Config {
	t.Helper()
	c := rt.tlsConfigForSNI("m.example")
	if c == nil {
		t.Fatal("host must have a per-SNI TLS config")
	}
	sc := c.Clone()
	sc.GetCertificate = nil
	sc.Certificates = []tls.Certificate{testTLSCert(t)}
	return sc
}

// writeCRL writes a CRL into dir and returns its base name (the confined,
// cert-store-relative form the config carries).
func writeCRL(t *testing.T, dir string, pemBytes []byte) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "corp.crl"), pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return "corp.crl"
}

// TestCRLRevokedSerialRejected proves the CRL hook actually enforces revocation
// at the handshake: a good certificate still connects, the revoked one does not.
func TestCRLRevokedSerialRejected(t *testing.T) {
	caPEM, caCert, caKey := crlCA(t, "corp-ca")
	dir := t.TempDir()
	name := writeCRL(t, dir, makeCRL(t, caCert, caKey, time.Now().Add(time.Hour), 10))

	rt, err := buildRouter(crlRouterConfig(caPEM, model.ClientCA{CRLFile: name}), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	sc := serverCfgFor(t, rt)

	revoked, _ := clientCertSerial(t, caCert, caKey, 10, "revoked")
	good, _ := clientCertSerial(t, caCert, caKey, 11, "good")

	if err := tlsHandshake(sc, forceClientCert(good)); err != nil {
		t.Fatalf("a certificate absent from the CRL must be accepted: %v", err)
	}
	if err := tlsHandshake(sc, forceClientCert(revoked)); err == nil {
		t.Fatal("a revoked certificate must be rejected")
	}
}

// TestCRLInlinePEM proves an inline crlPEM is enforced the same way a file is.
func TestCRLInlinePEM(t *testing.T) {
	caPEM, caCert, caKey := crlCA(t, "corp-ca")
	crl := makeCRL(t, caCert, caKey, time.Now().Add(time.Hour), 20)
	rt, err := buildRouter(crlRouterConfig(caPEM, model.ClientCA{CRLPEM: string(crl)}), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sc := serverCfgFor(t, rt)

	revoked, _ := clientCertSerial(t, caCert, caKey, 20, "revoked")
	if err := tlsHandshake(sc, forceClientCert(revoked)); err == nil {
		t.Fatal("an inline CRL must reject a revoked certificate")
	}
}

// TestCRLPolicyExpired covers the documented policy field: a CRL past its
// nextUpdate rejects every certificate by default (fail-closed) and accepts them
// only when the operator explicitly chose fail-open.
func TestCRLPolicyExpired(t *testing.T) {
	caPEM, caCert, caKey := crlCA(t, "corp-ca")
	expired := makeCRL(t, caCert, caKey, time.Now().Add(-time.Minute))
	good, _ := clientCertSerial(t, caCert, caKey, 30, "good")

	cases := []struct {
		name      string
		policy    string
		wantError bool
	}{
		{"default is fail-closed", "", true},
		{"explicit fail-closed", model.CRLPolicyFailClosed, true},
		{"fail-open serves", model.CRLPolicyFailOpen, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			name := writeCRL(t, dir, expired)
			rt, err := buildRouter(crlRouterConfig(caPEM, model.ClientCA{CRLFile: name, CRLPolicy: tc.policy}), dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = tlsHandshake(serverCfgFor(t, rt), forceClientCert(good))
			if tc.wantError && err == nil {
				t.Fatal("an expired CRL must reject the handshake under fail-closed")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("fail-open must accept despite the expired CRL: %v", err)
			}
		})
	}
}

// TestCRLPolicyUnloadable covers the same policy for a CRL that never loaded at
// all (missing file): fail-closed by default, servable only under fail-open.
func TestCRLPolicyUnloadable(t *testing.T) {
	caPEM, caCert, caKey := crlCA(t, "corp-ca")
	good, _ := clientCertSerial(t, caCert, caKey, 40, "good")

	for _, policy := range []string{"", model.CRLPolicyFailOpen} {
		dir := t.TempDir()
		rt, err := buildRouter(crlRouterConfig(caPEM, model.ClientCA{CRLFile: "missing.crl", CRLPolicy: policy}), dir, nil)
		if err != nil {
			t.Fatalf("an unreadable CRL must not fail the router build: %v", err)
		}
		err = tlsHandshake(serverCfgFor(t, rt), forceClientCert(good))
		if policy == "" && err == nil {
			t.Fatal("a missing CRL must reject the handshake under the fail-closed default")
		}
		if policy == model.CRLPolicyFailOpen && err != nil {
			t.Fatalf("fail-open must accept despite the missing CRL: %v", err)
		}
	}
}

// TestCRLForeignSignatureRefused proves the CRL signature is validated against
// the CA: a list signed by anyone else is refused, so dropping a file in the
// cert store cannot un-revoke (or mass-revoke) certificates.
func TestCRLForeignSignatureRefused(t *testing.T) {
	_, caCert, _ := crlCA(t, "corp-ca")
	_, otherCert, otherKey := crlCA(t, "other-ca")
	foreign := makeCRL(t, otherCert, otherKey, time.Now().Add(time.Hour), 50)

	if _, err := parseCRL(foreign, []*x509.Certificate{caCert}); err == nil {
		t.Fatal("a CRL signed by a different CA must be refused")
	}
	own := makeCRL(t, otherCert, otherKey, time.Now().Add(time.Hour), 50)
	if _, err := parseCRL(own, []*x509.Certificate{otherCert}); err != nil {
		t.Fatalf("a CRL signed by the CA itself must parse: %v", err)
	}
}

// TestCRLReloadOnMTimeChange proves the watch path picks up an out-of-band CRL
// refresh: a certificate accepted before the file changes is rejected after,
// with no config reload.
func TestCRLReloadOnMTimeChange(t *testing.T) {
	caPEM, caCert, caKey := crlCA(t, "corp-ca")
	dir := t.TempDir()
	name := writeCRL(t, dir, makeCRL(t, caCert, caKey, time.Now().Add(time.Hour)))

	rt, err := buildRouter(crlRouterConfig(caPEM, model.ClientCA{CRLFile: name}), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	sc := serverCfgFor(t, rt)
	cert, _ := clientCertSerial(t, caCert, caKey, 60, "later-revoked")
	if err := tlsHandshake(sc, forceClientCert(cert)); err != nil {
		t.Fatalf("certificate must be accepted before revocation: %v", err)
	}

	// Rewrite the CRL with a newer mtime, then run one watch pass.
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, makeCRL(t, caCert, caKey, time.Now().Add(time.Hour), 60), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	watched := crlAnchors.Load()
	if watched == nil || len(*watched) != 1 {
		t.Fatalf("the router build must register its file-backed CRL anchors, got %v", watched)
	}
	for _, a := range *watched {
		a.reloadIfChanged()
	}

	if err := tlsHandshake(sc, forceClientCert(cert)); err == nil {
		t.Fatal("the reloaded CRL must reject the newly revoked certificate")
	}
}

// certPassthroughRouter builds a host with client-certificate identity
// passthrough enabled, proxying to a backend that records what it received.
func certPassthroughRouter(t *testing.T, headers *model.ClientCertHeaders) (*router, *http.Header) {
	t.Helper()
	var got http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	t.Cleanup(backend.Close)
	u, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	caPEM, _, _ := crlCA(t, "corp-ca")
	cfg := model.Config{
		ClientCAs: []model.ClientCA{{ObjectMeta: model.ObjectMeta{Name: "corp"}, CAPEM: caPEM}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "m"},
			Domains:    []string{"m.example"},
			Upstream:   model.Upstream{Scheme: "http", Host: u.Hostname(), Port: port},
			TLS: model.TLSSettings{
				ForceSSL:   true,
				ClientAuth: &model.ClientAuth{CARef: "corp", Mode: "require", IdentityHeaders: headers},
			},
		}},
	}
	rt, err := buildRouter(cfg, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return rt, &got
}

// TestClientCertIdentityPassthrough proves gpm asserts the verified certificate's
// identity upstream, and that a forged header from the client is stripped first -
// the value the backend sees is always gpm's, never the client's.
func TestClientCertIdentityPassthrough(t *testing.T) {
	rt, got := certPassthroughRouter(t, &model.ClientCertHeaders{SAN: true, Serial: true, Fingerprint: true})
	_, caCert, caKey := crlCA(t, "corp-ca")
	_, leaf := clientCertSerial(t, caCert, caKey, 0x2a, "ops")

	r := httptest.NewRequest("GET", "https://m.example/", nil)
	r.Host = "m.example"
	r.Header.Set(model.DefaultClientCertSubjectHeader, "CN=forged")
	r.Header.Set(model.ClientCertSerialHeader, "deadbeef")
	r.TLS = &tls.ConnectionState{
		ServerName:       "m.example",
		PeerCertificates: []*x509.Certificate{leaf},
		VerifiedChains:   [][]*x509.Certificate{{leaf, caCert}},
	}
	rt.serveHTTPS(httptest.NewRecorder(), r)

	if h := got.Get(model.DefaultClientCertSubjectHeader); h != leaf.Subject.String() {
		t.Fatalf("subject header = %q, want %q", h, leaf.Subject.String())
	}
	if h := got.Get(model.ClientCertSerialHeader); h != "2a" {
		t.Fatalf("serial header = %q, want %q", h, "2a")
	}
	if h := got.Get(model.ClientCertSANHeader); h != "ops.corp.example" {
		t.Fatalf("SAN header = %q, want %q", h, "ops.corp.example")
	}
	if h := got.Get(model.ClientCertFingerprintHeader); len(h) != 64 {
		t.Fatalf("fingerprint header = %q, want a 64-char sha256 hex digest", h)
	}
}

// TestClientCertHeadersStrippedWithoutPassthrough proves the passthrough headers
// are in the baseline denylist: a host that never enabled them still strips a
// client's forged values rather than forwarding them.
func TestClientCertHeadersStrippedWithoutPassthrough(t *testing.T) {
	rt, got := certPassthroughRouter(t, nil)
	r := httptest.NewRequest("GET", "https://m.example/", nil)
	r.Host = "m.example"
	r.Header.Set(model.DefaultClientCertSubjectHeader, "CN=forged")
	r.Header.Set(model.ClientCertSANHeader, "forged.example")
	r.Header.Set(model.ClientCertSerialHeader, "deadbeef")
	r.Header.Set(model.ClientCertFingerprintHeader, "beef")
	_, caCert, caKey := crlCA(t, "corp-ca")
	_, leaf := clientCertSerial(t, caCert, caKey, 1, "ops")
	r.TLS = &tls.ConnectionState{
		ServerName:       "m.example",
		PeerCertificates: []*x509.Certificate{leaf},
		VerifiedChains:   [][]*x509.Certificate{{leaf, caCert}},
	}
	rt.serveHTTPS(httptest.NewRecorder(), r)

	for _, h := range []string{
		model.DefaultClientCertSubjectHeader, model.ClientCertSANHeader,
		model.ClientCertSerialHeader, model.ClientCertFingerprintHeader,
	} {
		if v := got.Get(h); v != "" {
			t.Fatalf("%s must be stripped from an untrusted peer, got %q", h, v)
		}
	}
}

// TestClientCertCustomSubjectHeaderStripped proves a custom subject header name
// joins this host's strip set, so it is asserted only by gpm.
func TestClientCertCustomSubjectHeaderStripped(t *testing.T) {
	rt, got := certPassthroughRouter(t, &model.ClientCertHeaders{SubjectHeader: "X-Corp-Client"})
	r := httptest.NewRequest("GET", "https://m.example/", nil)
	r.Host = "m.example"
	r.Header.Set("X-Corp-Client", "CN=forged")
	_, caCert, caKey := crlCA(t, "corp-ca")
	_, leaf := clientCertSerial(t, caCert, caKey, 2, "ops")
	r.TLS = &tls.ConnectionState{
		ServerName:       "m.example",
		PeerCertificates: []*x509.Certificate{leaf},
		VerifiedChains:   [][]*x509.Certificate{{leaf, caCert}},
	}
	rt.serveHTTPS(httptest.NewRecorder(), r)
	if v := got.Get("X-Corp-Client"); v != leaf.Subject.String() {
		t.Fatalf("custom subject header = %q, want gpm's %q", v, leaf.Subject.String())
	}
}

// TestClientCertGateMiddleware covers the client-cert auth mode: no verified
// certificate is 401, an unmapped subject is 403, and a mapped subject holding a
// required role is admitted.
func TestClientCertGateMiddleware(t *testing.T) {
	_, caCert, caKey := crlCA(t, "corp-ca")
	_, ops := clientCertSerial(t, caCert, caKey, 70, "ops")
	_, guest := clientCertSerial(t, caCert, caKey, 71, "guest")

	withCert := func(leaf *x509.Certificate) *http.Request {
		r := httptest.NewRequest("GET", "https://m.example/", nil)
		r.TLS = &tls.ConnectionState{
			ServerName:       "m.example",
			PeerCertificates: []*x509.Certificate{leaf},
			VerifiedChains:   [][]*x509.Certificate{{leaf, caCert}},
		}
		return r
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	cases := []struct {
		name string
		spec model.AuthMiddleware
		req  *http.Request
		want int
	}{
		{"no TLS at all", model.AuthMiddleware{Mode: model.AuthModeClientCert},
			httptest.NewRequest("GET", "https://m.example/", nil), http.StatusUnauthorized},
		{"verified cert, no mapping", model.AuthMiddleware{Mode: model.AuthModeClientCert},
			withCert(ops), http.StatusOK},
		{"mapped subject with required role", model.AuthMiddleware{
			Mode: model.AuthModeClientCert, RequiredRoles: []string{"admin"},
			ClientCertRoles: map[string]string{"ops": "admin"}}, withCert(ops), http.StatusOK},
		{"unmapped subject", model.AuthMiddleware{
			Mode: model.AuthModeClientCert, RequiredRoles: []string{"admin"},
			ClientCertRoles: map[string]string{"ops": "admin"}}, withCert(guest), http.StatusForbidden},
		{"mapped subject without the required role", model.AuthMiddleware{
			Mode: model.AuthModeClientCert, RequiredRoles: []string{"admin"},
			ClientCertRoles: map[string]string{"guest": "viewer"}}, withCert(guest), http.StatusForbidden},
		{"full RFC 2253 subject key", model.AuthMiddleware{
			Mode:            model.AuthModeClientCert,
			ClientCertRoles: map[string]string{ops.Subject.String(): "admin"}}, withCert(ops), http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			clientCertGate(tc.spec, peerIP, nil, "m", nil, ok).ServeHTTP(w, tc.req)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
		})
	}
}

// TestClientCertGateWiredFromMiddleware proves the mode reaches the gate through
// the normal auth-middleware dispatch, with no identity provider configured.
func TestClientCertGateWiredFromMiddleware(t *testing.T) {
	mw := model.Middleware{
		ObjectMeta: model.ObjectMeta{Name: "cert"},
		Type:       model.MWTypeAuth,
		Auth:       &model.AuthMiddleware{Mode: model.AuthModeClientCert},
	}
	h := authHandler(*mw.Auth, buildRegistry(model.Config{}), "m", []string{"m.example"},
		func(*http.Request) net.IP { return nil }, nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "https://m.example/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("client-cert mode with no certificate must be 401, got %d", w.Code)
	}
}
