package dataplane

import (
	"crypto/tls"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestCRLRevocationAppliesToResumedSessions proves a client certificate
// revoked after its first handshake cannot keep access by resuming the
// session: the stdlib skips VerifyPeerCertificate on resumption, so the
// revocation check has to run from VerifyConnection as well.
func TestCRLRevocationAppliesToResumedSessions(t *testing.T) {
	caPEM, caCert, caKey := crlCA(t, "corp-ca")
	dir := t.TempDir()
	name := writeCRL(t, dir, makeCRL(t, caCert, caKey, time.Now().Add(time.Hour)))

	rt, err := buildRouter(crlRouterConfig(caPEM, model.ClientCA{CRLFile: name}), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	sc := serverCfgFor(t, rt)
	cert, _ := clientCertSerial(t, caCert, caKey, 70, "resumer")
	cc := forceClientCert(cert)
	cc.ServerName = "m.example"
	cc.ClientSessionCache = tls.NewLRUClientSessionCache(4)

	resumed, err := resumableHandshake(t, sc, cc)
	if err != nil {
		t.Fatalf("first handshake: %v", err)
	}
	if resumed {
		t.Fatal("fixture: the first handshake cannot be a resumption")
	}
	resumed, err = resumableHandshake(t, sc, cc)
	if err != nil {
		t.Fatalf("second handshake: %v", err)
	}
	if !resumed {
		t.Fatal("fixture: the second handshake must resume, or the test proves nothing")
	}

	// Revoke the certificate through the real reload path.
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, makeCRL(t, caCert, caKey, time.Now().Add(time.Hour), 70), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	watched := crlAnchors.Load()
	if watched == nil {
		t.Fatal("the router build must register its file-backed CRL anchors")
	}
	for _, a := range *watched {
		a.reloadIfChanged()
	}

	if _, err := resumableHandshake(t, sc, cc); err == nil {
		t.Fatal("a resumed session must not outlive the certificate's revocation")
	}
}

// resumableHandshake runs one full handshake plus one application-data round
// trip over loopback TCP (a buffered transport, unlike net.Pipe, so the
// server's post-handshake session ticket never deadlocks against the client),
// which is what makes the client consume the ticket. It reports whether the
// server resumed.
func resumableHandshake(t *testing.T, sc, cc *tls.Config) (bool, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type result struct {
		resumed bool
		err     error
	}
	done := make(chan result, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- result{err: err}
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		srv := tls.Server(c, sc)
		if err := srv.Handshake(); err != nil {
			done <- result{err: err}
			return
		}
		if _, err := srv.Write([]byte("x")); err != nil {
			done <- result{err: err}
			return
		}
		buf := make([]byte, 1)
		_, err = io.ReadFull(srv, buf)
		done <- result{resumed: srv.ConnectionState().DidResume, err: err}
	}()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	cli := tls.Client(c, cc)
	if err := cli.Handshake(); err != nil {
		<-done
		return false, err
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(cli, buf); err != nil {
		<-done
		return false, err
	}
	if _, err := cli.Write([]byte("y")); err != nil {
		<-done
		return false, err
	}
	r := <-done
	return r.resumed, r.err
}
