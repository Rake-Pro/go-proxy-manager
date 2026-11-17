package dataplane

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// recheckConfig is a plain host, a host pinned to TLS 1.3, and an mTLS host
// whose client CA is caRef.
func recheckConfig(up model.Upstream, caAPEM, caBPEM, caRef string) model.Config {
	meta := func(n string) model.ObjectMeta { return model.ObjectMeta{Name: n} }
	return model.Config{
		ClientCAs: []model.ClientCA{
			{ObjectMeta: meta("ca-a"), CAPEM: caAPEM},
			{ObjectMeta: meta("ca-b"), CAPEM: caBPEM},
		},
		ProxyHosts: []model.ProxyHost{
			{ObjectMeta: meta("plain"), Domains: []string{"plain.example"}, Upstream: up},
			{ObjectMeta: meta("pin13"), Domains: []string{"pin13.example"}, Upstream: up,
				TLS: model.TLSSettings{MinTLSVersion: "1.3"}},
			{ObjectMeta: meta("mtls"), Domains: []string{"mtls.example"}, Upstream: up,
				TLS: model.TLSSettings{ClientAuth: &model.ClientAuth{CARef: caRef, Mode: "require"}}},
		},
	}
}

func recheckRouter(t *testing.T, cfg model.Config, cert *tls.Certificate) *router {
	t.Helper()
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	rt.certs.wildcard["example"] = cert
	return rt
}

// recheckListen serves cur over TLS with the same listener wiring as Start. It
// fails the test when a request arrives without the per-connection anchor record.
func recheckListen(t *testing.T, cur *atomic.Pointer[router]) string {
	t.Helper()
	srv := newListenerServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value(connAnchorKey{}).(*atomic.Pointer[clientCAAnchor]); !ok {
			t.Errorf("%s request has no per-connection anchor record", r.Proto)
		}
		cur.Load().serveHTTPS(w, r)
	}))
	srv.TLSConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: secureCipherSuites,
		NextProtos:   []string{"h2", "http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return cur.Load().certs.GetCertificate(hello)
		},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			return cur.Load().tlsConfigForSNI(hello.ServerName), nil
		},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func recheckClient(addr string, h2 bool, clientCert *tls.Certificate, maxVersion uint16) *http.Client {
	tc := &tls.Config{InsecureSkipVerify: true, MaxVersion: maxVersion}
	if clientCert != nil {
		c := *clientCert
		tc.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &c, nil }
	}
	tr := &http.Transport{
		TLSClientConfig:   tc,
		ForceAttemptHTTP2: h2,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}
	if !h2 {
		tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}

// recheckGet sends GET https://sni/ with the given Host and reports the status,
// whether the response closes the connection and whether it was reused.
func recheckGet(t *testing.T, c *http.Client, sni, host string) (status int, closes bool, reused bool) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, "https://"+sni+"/", nil)
	req.Host = host
	trace := &httptrace.ClientTrace{GotConn: func(i httptrace.GotConnInfo) { reused = i.Reused }}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("sni=%s host=%s: %v", sni, host, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Close, reused
}

type recheckEnv struct {
	addr           string
	cur            *atomic.Pointer[router]
	up             model.Upstream
	caAPEM, caBPEM string
	certA          tls.Certificate
	srvCert        tls.Certificate
}

func recheckSetup(t *testing.T) *recheckEnv {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(backend.Close)
	u, _ := url.Parse(backend.URL)
	port, _ := strconv.Atoi(u.Port())
	e := &recheckEnv{cur: &atomic.Pointer[router]{}}
	e.up = model.Upstream{Scheme: "http", Host: u.Hostname(), Port: port}
	caAPEM, caA, keyA := testCA(t, "ca-a")
	e.caBPEM, _, _ = testCA(t, "ca-b")
	e.caAPEM = caAPEM
	e.certA = signedClientCert(t, caA, keyA)
	e.srvCert = testTLSCert(t)
	e.cur.Store(recheckRouter(t, recheckConfig(e.up, e.caAPEM, e.caBPEM, "ca-a"), &e.srvCert))
	e.addr = recheckListen(t, e.cur)
	return e
}

// A host's minimum TLS version holds for the Host it serves, not only for the
// SNI the handshake used: a TLS 1.2 connection opened under another name gets
// 421 with Connection: close for the 1.3-pinned host.
func TestTLSFloorEnforcedForRoutedHost(t *testing.T) {
	e := recheckSetup(t)
	for _, sni := range []string{"plain.example", "unconfigured.example"} {
		c := recheckClient(e.addr, false, nil, tls.VersionTLS12)
		if st, _, _ := recheckGet(t, c, sni, sni); st == http.StatusMisdirectedRequest {
			t.Fatalf("sni=%s: the host's own request must not be refused, got %d", sni, st)
		}
		st, conn, _ := recheckGet(t, c, sni, "pin13.example")
		if st != http.StatusMisdirectedRequest || !conn {
			t.Errorf("sni=%s host=pin13.example over TLS 1.2: got %d close=%v, want 421 with Connection: close", sni, st, conn)
		}
	}
	// A TLS 1.3 connection under another name still reaches the pinned host.
	c := recheckClient(e.addr, false, nil, 0)
	if st, _, _ := recheckGet(t, c, "plain.example", "pin13.example"); st == http.StatusMisdirectedRequest {
		t.Errorf("a TLS 1.3 connection must reach the 1.3-pinned host, got %d", st)
	}
}

// A connection whose client certificate was verified under a client CA the host
// no longer references is refused on its next request (HTTP/1.1 and HTTP/2),
// while a reload that keeps the same CA leaves the connection working.
func TestMTLSConnectionRecheckedAfterClientCAChange(t *testing.T) {
	e := recheckSetup(t)
	for _, h2 := range []bool{false, true} {
		e.cur.Store(recheckRouter(t, recheckConfig(e.up, e.caAPEM, e.caBPEM, "ca-a"), &e.srvCert))
		c := recheckClient(e.addr, h2, &e.certA, 0)
		if st, _, _ := recheckGet(t, c, "mtls.example", "mtls.example"); st != http.StatusOK {
			t.Fatalf("h2=%v: before the change: got %d, want 200", h2, st)
		}
		// Same CA, new build (new anchor pointer): still accepted.
		e.cur.Store(recheckRouter(t, recheckConfig(e.up, e.caAPEM, e.caBPEM, "ca-a"), &e.srvCert))
		if st, _, reused := recheckGet(t, c, "mtls.example", "mtls.example"); st != http.StatusOK || !reused {
			t.Fatalf("h2=%v: reload with the same CA: got %d reused=%v, want 200 on the same connection", h2, st, reused)
		}
		// The host now trusts CA B only.
		e.cur.Store(recheckRouter(t, recheckConfig(e.up, e.caAPEM, e.caBPEM, "ca-b"), &e.srvCert))
		st, _, reused := recheckGet(t, c, "mtls.example", "mtls.example")
		if !reused {
			t.Fatalf("h2=%v: connection was not reused", h2)
		}
		if st != http.StatusMisdirectedRequest {
			t.Errorf("h2=%v: after the CA change: got %d, want 421", h2, st)
		}
	}
}

// The per-connection record caches a positive result for one anchor only.
func TestAnchorsConnCachesPerAnchor(t *testing.T) {
	caAPEM, caA, keyA := testCA(t, "ca-a")
	caBPEM, _, _ := testCA(t, "ca-b")
	anchors, err := buildClientCAAnchors([]model.ClientCA{
		{ObjectMeta: model.ObjectMeta{Name: "a"}, CAPEM: caAPEM},
		{ObjectMeta: model.ObjectMeta{Name: "b"}, CAPEM: caBPEM},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(signedClientCert(t, caA, keyA).Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	chains := [][]*x509.Certificate{{leaf, caA}}
	ctx := withConnAnchorCache(context.Background())
	if !anchors["a"].anchorsConn(ctx, chains) {
		t.Fatal("a chain ending in CA A must be anchored in A")
	}
	if anchors["b"].anchorsConn(ctx, chains) {
		t.Fatal("a cached result for A must not answer for B")
	}
	if (*clientCAAnchor)(nil).anchorsConn(ctx, chains) {
		t.Fatal("a nil anchor anchors nothing")
	}
}
