package dataplane

import (
	"crypto"
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
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// testCA generates a self-signed CA and returns its PEM plus the cert/key needed
// to sign client certificates against it.
func testCA(t *testing.T, cn string) (caPEM string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) {
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
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
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
	p := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return p, cert, key
}

// signedClientCert issues a client certificate signed by the given CA.
func signedClientCert(t *testing.T, caCert *x509.Certificate, caKey crypto.Signer) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// mtlsServerConfig builds a server tls.Config the way buildRouter composes an
// mTLS host, exercising the real pool builder and mode mapping.
func mtlsServerConfig(t *testing.T, caPEM, mode string) *tls.Config {
	t.Helper()
	pools, err := buildClientCAPools([]model.ClientCA{{ObjectMeta: model.ObjectMeta{Name: "corp"}, CAPEM: caPEM}})
	if err != nil {
		t.Fatalf("buildClientCAPools: %v", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{testTLSCert(t)},
		ClientCAs:    pools["corp"],
		ClientAuth:   clientAuthType(mode),
	}
}

// forceClientCert returns a client config that always presents cert, bypassing
// the Go client's default filtering by the server's acceptable-CA list, so the
// server's own verification (not client-side selection) is what the test exercises.
func forceClientCert(cert tls.Certificate) *tls.Config {
	c := cert
	return &tls.Config{
		InsecureSkipVerify:   true,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &c, nil },
	}
}

// TestMTLSRequireHandshake proves the require mode actually enforces the trust
// anchor at handshake time: no cert is rejected, a cert from the configured CA is
// accepted, and a cert from a different CA is rejected.
func TestMTLSRequireHandshake(t *testing.T) {
	caPEM, caCert, caKey := testCA(t, "trusted-ca")
	_, otherCert, otherKey := testCA(t, "other-ca")
	serverCfg := mtlsServerConfig(t, caPEM, "require")

	good := signedClientCert(t, caCert, caKey)
	bad := signedClientCert(t, otherCert, otherKey)

	// No client certificate -> handshake must fail.
	if err := tlsHandshake(serverCfg, &tls.Config{InsecureSkipVerify: true}); err == nil {
		t.Fatal("require mode must reject a client presenting no certificate")
	}
	// Certificate from the configured CA -> handshake must succeed.
	if err := tlsHandshake(serverCfg, forceClientCert(good)); err != nil {
		t.Fatalf("require mode must accept a cert from the configured CA: %v", err)
	}
	// Certificate from a different CA -> handshake must fail.
	if err := tlsHandshake(serverCfg, forceClientCert(bad)); err == nil {
		t.Fatal("require mode must reject a cert from a different CA")
	}
}

// TestMTLSOptionalHandshake proves optional mode lets a certless client through
// but still verifies a presented cert against the trust anchor.
func TestMTLSOptionalHandshake(t *testing.T) {
	caPEM, _, _ := testCA(t, "trusted-ca")
	_, otherCert, otherKey := testCA(t, "other-ca")
	serverCfg := mtlsServerConfig(t, caPEM, "optional")

	bad := signedClientCert(t, otherCert, otherKey)

	// No client certificate -> optional mode proceeds.
	if err := tlsHandshake(serverCfg, &tls.Config{InsecureSkipVerify: true}); err != nil {
		t.Fatalf("optional mode must accept a client presenting no certificate: %v", err)
	}
	// A presented cert from a different CA is still verified -> rejected.
	if err := tlsHandshake(serverCfg, forceClientCert(bad)); err == nil {
		t.Fatal("optional mode must reject a presented cert from a different CA")
	}
}

// TestBuildRouterMTLSConfig checks buildRouter wires the per-SNI config: the pool
// is set, the mode maps correctly, and it merges with a 1.3 version pin.
func TestBuildRouterMTLSConfig(t *testing.T) {
	caPEM, _, _ := testCA(t, "trusted-ca")
	up := model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}
	cfg := model.Config{
		ClientCAs: []model.ClientCA{{ObjectMeta: model.ObjectMeta{Name: "corp"}, CAPEM: caPEM}},
		ProxyHosts: []model.ProxyHost{
			{ObjectMeta: model.ObjectMeta{Name: "req"}, Domains: []string{"req.example"}, Upstream: up,
				TLS: model.TLSSettings{MinTLSVersion: "1.3", ClientAuth: &model.ClientAuth{CARef: "corp", Mode: "require"}}},
			{ObjectMeta: model.ObjectMeta{Name: "opt"}, Domains: []string{"opt.example"}, Upstream: up,
				TLS: model.TLSSettings{ClientAuth: &model.ClientAuth{CARef: "corp", Mode: "optional"}}},
			{ObjectMeta: model.ObjectMeta{Name: "plain"}, Domains: []string{"plain.example"}, Upstream: up},
		},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatal(err)
	}

	req := rt.tlsConfigForSNI("req.example")
	if req == nil || req.ClientCAs == nil {
		t.Fatalf("require host must have a ClientCAs pool, got %+v", req)
	}
	if req.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("require host must map to RequireAndVerifyClientCert, got %v", req.ClientAuth)
	}
	if req.MinVersion != tls.VersionTLS13 {
		t.Fatalf("require host must merge the 1.3 version pin, got %x", req.MinVersion)
	}

	opt := rt.tlsConfigForSNI("opt.example")
	if opt == nil || opt.ClientAuth != tls.VerifyClientCertIfGiven {
		t.Fatalf("optional host must map to VerifyClientCertIfGiven, got %+v", opt)
	}
	if opt.MinVersion != tls.VersionTLS12 {
		t.Fatalf("optional host without a pin keeps the 1.2 floor, got %x", opt.MinVersion)
	}

	if c := rt.tlsConfigForSNI("plain.example"); c != nil {
		t.Fatalf("host without mTLS or a version pin must use the listener default (nil), got %+v", c)
	}
}

// mtlsDispatchRouter builds a router with a require-mTLS host, an optional-mTLS
// host, and a plain host that all share one test server certificate, so a
// handshake can complete for any SNI and the per-request dispatch gate (not the
// handshake) is what accepts or rejects. It returns the router and the CA needed
// to sign a client certificate the mTLS hosts accept.
func mtlsDispatchRouter(t *testing.T) (*router, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	caPEM, caCert, caKey := testCA(t, "trusted-ca")
	up := model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}
	cfg := model.Config{
		ClientCAs: []model.ClientCA{{ObjectMeta: model.ObjectMeta{Name: "corp"}, CAPEM: caPEM}},
		ProxyHosts: []model.ProxyHost{
			{ObjectMeta: model.ObjectMeta{Name: "req"}, Domains: []string{"req.example"}, Upstream: up,
				TLS: model.TLSSettings{ClientAuth: &model.ClientAuth{CARef: "corp", Mode: "require"}}},
			{ObjectMeta: model.ObjectMeta{Name: "opt"}, Domains: []string{"opt.example"}, Upstream: up,
				TLS: model.TLSSettings{ClientAuth: &model.ClientAuth{CARef: "corp", Mode: "optional"}}},
			{ObjectMeta: model.ObjectMeta{Name: "plain"}, Domains: []string{"plain.example"}, Upstream: up},
		},
	}
	rt, err := buildRouter(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	cert := testTLSCert(t)
	for _, d := range []string{"req.example", "opt.example", "plain.example"} {
		rt.certs.exact[d] = &cert
	}
	return rt, caCert, caKey
}

// dispatchTLS performs a real net.Pipe handshake against the router's listener
// wiring (per-SNI GetConfigForClient) for the given SNI and optional client cert,
// then dispatches an HTTP request with the given Host header through serveHTTPS and
// returns the resulting status. A default cert backs any empty/foreign SNI so the
// handshake still completes and the dispatch gate is what does the rejecting.
func dispatchTLS(t *testing.T, rt *router, sni, host string, clientCert *tls.Certificate) (int, error) {
	t.Helper()
	def := testTLSCert(t)
	serverCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
		GetCertificate: func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if c, err := rt.certs.GetCertificate(h); err == nil {
				return c, nil
			}
			return &def, nil
		},
		GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			return rt.tlsConfigForSNI(h.ServerName), nil
		},
	}
	clientCfg := &tls.Config{InsecureSkipVerify: true, ServerName: sni}
	if clientCert != nil {
		c := *clientCert
		clientCfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &c, nil }
	}

	sConn, cConn := net.Pipe()
	deadline := time.Now().Add(5 * time.Second)
	_ = sConn.SetDeadline(deadline)
	_ = cConn.SetDeadline(deadline)
	defer sConn.Close()
	defer cConn.Close()

	srv := tls.Server(sConn, serverCfg)
	cli := tls.Client(cConn, clientCfg)
	errc := make(chan error, 1)
	go func() { errc <- srv.Handshake() }()
	if err := cli.Handshake(); err != nil {
		<-errc
		return 0, err
	}
	if err := <-errc; err != nil {
		return 0, err
	}

	state := srv.ConnectionState()
	req := httptest.NewRequest(http.MethodGet, "https://"+host+"/", nil)
	req.Host = host
	req.TLS = &state
	w := httptest.NewRecorder()
	rt.serveHTTPS(w, req)
	return w.Code, nil
}

// TestMTLSDispatchRejectsForeignSNI closes the core bypass: a client handshakes
// with a non-mTLS host's SNI (so no client cert is asked for), then targets the
// mTLS host by Host header. The per-request gate must reject it.
func TestMTLSDispatchRejectsForeignSNI(t *testing.T) {
	rt, _, _ := mtlsDispatchRouter(t)
	code, err := dispatchTLS(t, rt, "plain.example", "req.example", nil)
	if err != nil {
		t.Fatalf("handshake with a non-mTLS SNI should complete: %v", err)
	}
	if code != http.StatusMisdirectedRequest {
		t.Fatalf("Host=req.example over a plain.example handshake must be rejected (421), got %d", code)
	}
}

// TestMTLSDispatchRejectsEmptySNI proves an empty SNI (which never selected the
// mTLS config) cannot reach the mTLS host by Host header.
func TestMTLSDispatchRejectsEmptySNI(t *testing.T) {
	rt, _, _ := mtlsDispatchRouter(t)
	code, err := dispatchTLS(t, rt, "", "req.example", nil)
	if err != nil {
		t.Fatalf("handshake with empty SNI should complete against the default cert: %v", err)
	}
	if code != http.StatusMisdirectedRequest {
		t.Fatalf("Host=req.example with empty SNI must be rejected (421), got %d", code)
	}
}

// TestMTLSDispatchAllowsMatchedSNIAndCert proves the legitimate path still works:
// SNI==Host on the mTLS host with a valid client cert passes the gate.
func TestMTLSDispatchAllowsMatchedSNIAndCert(t *testing.T) {
	rt, caCert, caKey := mtlsDispatchRouter(t)
	client := signedClientCert(t, caCert, caKey)
	code, err := dispatchTLS(t, rt, "req.example", "req.example", &client)
	if err != nil {
		t.Fatalf("SNI==Host with a valid client cert must complete the handshake: %v", err)
	}
	if code == http.StatusMisdirectedRequest {
		t.Fatal("SNI=req.example + valid client cert + Host=req.example must pass the mTLS gate")
	}
}

// TestMTLSDispatchOptionalMode proves optional mode allows a certless request when
// SNI==Host (the correct config was used) but still rejects an SNI mismatch, which
// also closes the min-TLS-version-by-SNI dodge for any mTLS host.
func TestMTLSDispatchOptionalMode(t *testing.T) {
	rt, _, _ := mtlsDispatchRouter(t)
	code, err := dispatchTLS(t, rt, "opt.example", "opt.example", nil)
	if err != nil {
		t.Fatalf("optional-mode certless handshake must complete: %v", err)
	}
	if code == http.StatusMisdirectedRequest {
		t.Fatal("optional mode with SNI==Host must pass the gate even without a client cert")
	}
	code, err = dispatchTLS(t, rt, "plain.example", "opt.example", nil)
	if err != nil {
		t.Fatalf("handshake should complete: %v", err)
	}
	if code != http.StatusMisdirectedRequest {
		t.Fatalf("optional mode with SNI!=Host must be rejected (421), got %d", code)
	}
}

// TestMTLSPlaintextRedirects proves the plaintext listener never serves an mTLS
// host in the clear: it is redirected to HTTPS (where the per-request client-cert
// gate applies), while a non-mTLS host on :80 is served unchanged.
func TestMTLSPlaintextRedirects(t *testing.T) {
	rt, _, _ := mtlsDispatchRouter(t)
	// An mTLS host hit over plain HTTP must be redirected to https, not served.
	if rec := serveOn(rt, false, "GET", "http://req.example/x?y=1", "req.example"); rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != "https://req.example/x?y=1" {
		t.Fatalf("mTLS host on :80 must redirect to https: got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// optional mode is still mTLS-required (has a clientAuth entry) -> also redirected.
	if rec := serveOn(rt, false, "GET", "http://opt.example/", "opt.example"); rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("optional-mTLS host on :80 must redirect to https: got %d", rec.Code)
	}
	// A non-mTLS host is unchanged on :80: it reaches its handler (proxy to a dead
	// upstream here yields 502, not a redirect and not a 404).
	if rec := serveOn(rt, false, "GET", "http://plain.example/", "plain.example"); rec.Code == http.StatusPermanentRedirect || rec.Code == http.StatusNotFound {
		t.Fatalf("non-mTLS host on :80 must be served, not redirected: got %d", rec.Code)
	}
}

// TestBuildRouterMTLSFailClosed proves an unparseable CA bundle fails the compile
// rather than silently serving a host without its client-cert check.
func TestBuildRouterMTLSFailClosed(t *testing.T) {
	up := model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}
	cfg := model.Config{
		ClientCAs: []model.ClientCA{{ObjectMeta: model.ObjectMeta{Name: "corp"}, CAPEM: "not a pem block"}},
		ProxyHosts: []model.ProxyHost{
			{ObjectMeta: model.ObjectMeta{Name: "req"}, Domains: []string{"req.example"}, Upstream: up,
				TLS: model.TLSSettings{ClientAuth: &model.ClientAuth{CARef: "corp", Mode: "require"}}},
		},
	}
	if _, err := buildRouter(cfg, ""); err == nil {
		t.Fatal("buildRouter must fail closed when a client CA parses to no certificates")
	}
}
