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
	if hostTLSConfig("", tls.VersionTLS12, &certResolver{}) != nil {
		t.Error(`minTLSVersion "" must use the default listener config (nil)`)
	}
	if hostTLSConfig("1.2", tls.VersionTLS12, &certResolver{}) != nil {
		t.Error(`minTLSVersion "1.2" must use the default listener config (nil)`)
	}
	c := hostTLSConfig("1.3", tls.VersionTLS12, &certResolver{})
	if c == nil || c.MinVersion != tls.VersionTLS13 {
		t.Fatalf(`minTLSVersion "1.3" must pin MinVersion 1.3, got %+v`, c)
	}
}

// The fleet floor (settings.tls.minVersion) is the DEFAULT a host inherits, and
// a host pin overrides it in both directions.
func TestHostTLSConfigFleetFloor(t *testing.T) {
	if c := hostTLSConfig("", tls.VersionTLS13, &certResolver{}); c != nil {
		t.Errorf("an unpinned host under a 1.3 fleet floor needs no per-SNI config, got %+v", c)
	}
	if c := hostTLSConfig("1.3", tls.VersionTLS13, &certResolver{}); c != nil {
		t.Errorf("a host pinning the fleet floor itself needs no per-SNI config, got %+v", c)
	}
	c := hostTLSConfig("1.2", tls.VersionTLS13, &certResolver{})
	if c == nil || c.MinVersion != tls.VersionTLS12 {
		t.Fatalf(`a host pinning "1.2" under a 1.3 fleet floor must keep 1.2, got %+v`, c)
	}
}

// setFleetTLS installs a fleet floor for the duration of one test and restores
// the previous value, so the package-level handle cannot leak between tests.
func setFleetTLS(t *testing.T, minVersion string) {
	t.Helper()
	prev := globalTLSFloor.Load()
	SetTLSFleetDefaults(model.TLSFleetSettings{MinVersion: minVersion})
	t.Cleanup(func() { globalTLSFloor.Store(prev) })
}

// The fleet-wide switch has to reach hosts that pin nothing AND an unknown or
// absent SNI, or "hardened edge" is only true for the names in the config.
func TestRouterFleetTLSFloor(t *testing.T) {
	setFleetTLS(t, "1.3")
	up := model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}
	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{ObjectMeta: model.ObjectMeta{Name: "legacy"}, Domains: []string{"legacy.example"}, Upstream: up, TLS: model.TLSSettings{MinTLSVersion: "1.2"}},
		{ObjectMeta: model.ObjectMeta{Name: "normal"}, Domains: []string{"normal.example"}, Upstream: up},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := rt.tlsConfigForSNI("normal.example"); c == nil || c.MinVersion != tls.VersionTLS13 {
		t.Fatalf("an unpinned host must inherit the 1.3 fleet floor, got %+v", c)
	}
	if c := rt.tlsConfigForSNI("unknown.example"); c == nil || c.MinVersion != tls.VersionTLS13 {
		t.Fatalf("an unknown SNI must still get the 1.3 fleet floor, got %+v", c)
	}
	if c := rt.tlsConfigForSNI("legacy.example"); c == nil || c.MinVersion != tls.VersionTLS12 {
		t.Fatalf("a host pinning 1.2 must override the fleet floor, got %+v", c)
	}
}

// The default (unset) fleet floor must leave today's behaviour byte-identical:
// no per-SNI config for an unpinned host, and none for an unknown SNI.
func TestRouterFleetTLSFloorDefaultUnchanged(t *testing.T) {
	setFleetTLS(t, "")
	up := model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}
	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		{ObjectMeta: model.ObjectMeta{Name: "normal"}, Domains: []string{"normal.example"}, Upstream: up},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if rt.defaultTLS != nil {
		t.Error("an unset fleet floor must install no default per-SNI config")
	}
	if c := rt.tlsConfigForSNI("normal.example"); c != nil {
		t.Fatalf("an unpinned host under the default fleet floor must use the listener config, got %+v", c)
	}
	if c := rt.tlsConfigForSNI("unknown.example"); c != nil {
		t.Fatalf("an unknown SNI under the default fleet floor must use the listener config, got %+v", c)
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
