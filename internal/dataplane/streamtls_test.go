package dataplane

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// taggedBackend answers every connection with a fixed byte, so a test can tell
// which backend a stream was routed to.
func taggedBackend(t *testing.T, tag string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte(tag))
				_, _ = io.Copy(io.Discard, c)
			}(c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func streamHostFor(name, addr string, port int, tlsCfg *model.StreamTLS, acls ...string) model.StreamHost {
	host, p, _ := net.SplitHostPort(addr)
	fp := 0
	_, _ = fmt.Sscanf(p, "%d", &fp)
	return model.StreamHost{
		ObjectMeta:  model.ObjectMeta{Name: name},
		ListenPort:  port,
		Protocol:    "tcp",
		ForwardHost: host,
		ForwardPort: fp,
		TLS:         tlsCfg,
		AccessLists: acls,
	}
}

// TestStreamSNIRoutingTwoHostsOnOnePort proves two passthrough stream hosts can
// share one listen port and are separated purely by the SNI in the ClientHello -
// with the handshake bytes replayed to the backend intact, so gpm never has to
// hold either host's key.
func TestStreamSNIRoutingTwoHostsOnOnePort(t *testing.T) {
	aAddr, closeA := taggedBackend(t, "A")
	defer closeA()
	bAddr, closeB := taggedBackend(t, "B")
	defer closeB()

	port := freePort(t)
	cfg := model.Config{StreamHosts: []model.StreamHost{
		streamHostFor("a", aAddr, port, &model.StreamTLS{Mode: model.StreamTLSPassthrough, SNIMatch: []string{"a.example.com"}}),
		streamHostFor("b", bAddr, port, &model.StreamTLS{Mode: model.StreamTLSPassthrough, SNIMatch: []string{"*.b.example.com"}}),
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("two SNI-routed hosts on one port must validate: %v", err)
	}
	tcpRoutes, _ := buildStreamRoutes(cfg, nil)
	routes := tcpRoutes[port]
	if routes == nil || !routes.sni {
		t.Fatalf("expected an SNI-routed table on port %d, got %+v", port, routes)
	}
	f, err := startTCPForwarder(0, routes, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()
	listen := f.ln.Addr().(*net.TCPAddr).Port

	for _, tc := range []struct {
		sni, want string
	}{
		{"a.example.com", "A"},
		{"one.b.example.com", "B"},
	} {
		t.Run(tc.sni, func(t *testing.T) {
			c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listen))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			if _, err := c.Write(tlsRecords(buildClientHello(tc.sni), 4096)); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 1)
			if _, err := io.ReadFull(c, buf); err != nil {
				t.Fatal(err)
			}
			if string(buf) != tc.want {
				t.Fatalf("sni %q reached backend %q, want %q", tc.sni, buf, tc.want)
			}
		})
	}

	// A name no host claims gets no backend at all: the connection is closed
	// rather than handed to whichever host happened to compile first.
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listen))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write(tlsRecords(buildClientHello("unclaimed.example.net"), 4096)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(c, make([]byte, 1)); err == nil {
		t.Fatal("an unmatched server name must not reach any backend")
	}
}

// TestStreamTLSTerminate proves terminate mode completes the handshake at gpm
// with the referenced certificate and forwards plaintext to the backend.
func TestStreamTLSTerminate(t *testing.T) {
	dir := t.TempDir()
	writeSelfSigned(t, dir, "stream.example.com")

	backend, stopBackend := tcpEcho(t)
	defer stopBackend()

	port := freePort(t)
	cfg := model.Config{
		Certificates: []model.Certificate{{
			ObjectMeta: model.ObjectMeta{Name: "stream-cert"},
			Type:       model.CertTypeCustom,
			Domains:    []string{"stream.example.com"},
			Custom:     &model.CustomCertSpec{CertFile: "cert.pem", KeyFile: "key.pem"},
		}},
		StreamHosts: []model.StreamHost{streamHostFor("secure-db", backend, port, &model.StreamTLS{
			Mode:           model.StreamTLSTerminate,
			SNIMatch:       []string{"stream.example.com"},
			CertificateRef: "stream-cert",
		})},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("terminate config must validate: %v", err)
	}
	certs, err := buildCertResolver(cfg.Certificates, dir)
	if err != nil {
		t.Fatal(err)
	}
	tcpRoutes, _ := buildStreamRoutes(cfg, certs)
	routes := tcpRoutes[port]
	if routes == nil || routes.match("stream.example.com") == nil || routes.match("stream.example.com").tlsConf == nil {
		t.Fatal("terminate mode must compile a server TLS config")
	}
	f, err := startTCPForwarder(0, routes, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()

	addr := fmt.Sprintf("127.0.0.1:%d", f.ln.Addr().(*net.TCPAddr).Port)
	c, err := tls.Dial("tcp", addr, &tls.Config{ServerName: "stream.example.com", InsecureSkipVerify: true}) //nolint:gosec // self-signed test cert
	if err != nil {
		t.Fatalf("TLS handshake against the terminating stream host failed: %v", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("plaintext")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 9)
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "plaintext" {
		t.Fatalf("echo through the terminated stream = %q, want %q", buf, "plaintext")
	}
}

// TestStreamAccessListDeniesBeforeDial proves an L4 access list closes the
// connection before ANY upstream dial happens - a denied client must not be able
// to make gpm open a socket to the backend at all.
func TestStreamAccessListDeniesBeforeDial(t *testing.T) {
	var dials atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			dials.Add(1)
			c.Close()
		}
	}()

	port := freePort(t)
	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "no-one"},
			DefaultAction: model.ActionDeny,
			Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "203.0.113.0/24"}},
		}},
		StreamHosts: []model.StreamHost{streamHostFor("gated", ln.Addr().String(), port, nil, "no-one")},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	tcpRoutes, _ := buildStreamRoutes(cfg, nil)
	f, err := startTCPForwarder(0, tcpRoutes[port], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()

	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.ln.Addr().(*net.TCPAddr).Port))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(c, make([]byte, 1)); err == nil {
		t.Fatal("a denied client must not be served")
	}
	if n := dials.Load(); n != 0 {
		t.Fatalf("backend saw %d dials, want 0 (the gate runs before the dial)", n)
	}
}

// TestStreamAccessListAllowsLoopback is the positive control for the gate above:
// the same wiring with a list that allows the client forwards normally.
func TestStreamAccessListAllowsLoopback(t *testing.T) {
	backend, stop := tcpEcho(t)
	defer stop()

	port := freePort(t)
	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "loopback"},
			DefaultAction: model.ActionDeny,
			Rules: []model.IPRule{
				{Action: model.ActionAllow, CIDR: "127.0.0.0/8"},
				{Action: model.ActionAllow, CIDR: "::1/128"},
			},
		}},
		StreamHosts: []model.StreamHost{streamHostFor("open", backend, port, nil, "loopback")},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	tcpRoutes, _ := buildStreamRoutes(cfg, nil)
	f, err := startTCPForwarder(0, tcpRoutes[port], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer f.stop()

	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", f.ln.Addr().(*net.TCPAddr).Port))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo = %q, want ping", buf)
	}
}

// TestStreamUDPAccessList proves the same gate applies to UDP: a denied source
// never gets a session (nor an upstream socket), an allowed one is forwarded.
func TestStreamUDPAccessList(t *testing.T) {
	bpc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bpc.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := bpc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = bpc.WriteTo(buf[:n], addr)
		}
	}()

	host, p, _ := net.SplitHostPort(bpc.LocalAddr().String())
	bport := 0
	_, _ = fmt.Sscanf(p, "%d", &bport)

	newForwarder := func(t *testing.T, listName, cidr string) *udpForwarder {
		t.Helper()
		port := freePort(t)
		cfg := model.Config{
			AccessLists: []model.AccessList{{
				ObjectMeta:    model.ObjectMeta{Name: listName},
				DefaultAction: model.ActionDeny,
				Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: cidr}},
			}},
			StreamHosts: []model.StreamHost{{
				ObjectMeta:  model.ObjectMeta{Name: "udp-host"},
				ListenPort:  port,
				Protocol:    "udp",
				ForwardHost: host,
				ForwardPort: bport,
				AccessLists: []string{listName},
			}},
		}
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}
		_, udpRoutes := buildStreamRoutes(cfg, nil)
		target := udpRoutes[port]
		f, err := startUDPForwarder(0, target)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}

	roundTrip := func(t *testing.T, f *udpForwarder) error {
		t.Helper()
		c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", f.pc.LocalAddr().(*net.UDPAddr).Port))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if _, err := c.Write([]byte("ping")); err != nil {
			t.Fatal(err)
		}
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, err = c.Read(make([]byte, 4))
		return err
	}

	denied := newForwarder(t, "no-one", "203.0.113.0/24")
	defer denied.stop()
	if err := roundTrip(t, denied); err == nil {
		t.Fatal("a denied udp source must not be forwarded")
	}
	denied.mu.Lock()
	sessions := len(denied.sessions)
	denied.mu.Unlock()
	if sessions != 0 {
		t.Fatalf("denied source created %d sessions, want 0", sessions)
	}

	allowed := newForwarder(t, "loopback", "127.0.0.0/8")
	defer allowed.stop()
	if err := roundTrip(t, allowed); err != nil {
		t.Fatalf("an allowed udp source must be forwarded: %v", err)
	}
}
