package dataplane

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func testTLSCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "t"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"t"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestHostTLSConfigMinVersion(t *testing.T) {
	if hostTLSConfig("", &certResolver{}) != nil {
		t.Error(`minTLSVersion "" must use the default listener config (nil)`)
	}
	if hostTLSConfig("1.2", &certResolver{}) != nil {
		t.Error(`minTLSVersion "1.2" must use the default listener config (nil)`)
	}
	c := hostTLSConfig("1.3", &certResolver{})
	if c == nil || c.MinVersion != tls.VersionTLS13 {
		t.Fatalf(`minTLSVersion "1.3" must pin MinVersion 1.3, got %+v`, c)
	}
}

func TestRouterTLSConfigForSNI(t *testing.T) {
	up := model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}
	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{ObjectMeta: model.ObjectMeta{Name: "strict"}, Domains: []string{"strict.example"}, Upstream: up, TLS: model.TLSSettings{MinTLSVersion: "1.3"}},
		{ObjectMeta: model.ObjectMeta{Name: "normal"}, Domains: []string{"normal.example"}, Upstream: up},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := rt.tlsConfigForSNI("strict.example"); c == nil || c.MinVersion != tls.VersionTLS13 {
		t.Fatalf("pinned host must select a 1.3 config, got %+v", c)
	}
	if c := rt.tlsConfigForSNI("normal.example"); c != nil {
		t.Fatal("default host must use the listener default (nil)")
	}
	if c := rt.tlsConfigForSNI("unknown.example"); c != nil {
		t.Fatal("unknown SNI must use the listener default (nil)")
	}
}

// TestTLS13ConfigRejectsTLS12Client proves the pinned config actually enforces
// the floor at handshake time: a TLS 1.2-only client is rejected, a 1.3 client
// succeeds. Uses an in-memory net.Pipe so no listener/port is needed.
func TestTLS13ConfigRejectsTLS12Client(t *testing.T) {
	serverCfg := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{testTLSCert(t)}}

	if err := tlsHandshake(serverCfg, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12}); err == nil {
		t.Fatal("a TLS 1.2 client must be rejected by a 1.3-pinned server")
	}
	if err := tlsHandshake(serverCfg, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}); err != nil {
		t.Fatalf("a TLS 1.3 client should complete the handshake: %v", err)
	}
}

func tlsHandshake(serverCfg, clientCfg *tls.Config) error {
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
	cerr := cli.Handshake()
	serr := <-errc
	if cerr != nil {
		return cerr
	}
	return serr
}
