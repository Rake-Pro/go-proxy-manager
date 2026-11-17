package dataplane

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// countingListener counts the connections an upstream accepts, so a test can
// show that no upstream request was made at all.
type countingListener struct {
	net.Listener
	accepted atomic.Int32
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.accepted.Add(1)
	}
	return c, err
}

type requestTargetEnv struct {
	tlsAddr, plainAddr string
	up                 *countingListener
}

// requestTargetSetup runs the production Server (validated config, Reload,
// dataHandler + dispatch behind newListenerServer) on both listeners, in front
// of an upstream that serves by Host. "open" is reachable by anyone; "closed"
// sits behind a deny-all access list. Neither forces SSL, so the plaintext
// listener routes too.
func requestTargetSetup(t *testing.T) *requestTargetEnv {
	t.Helper()
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	up := &countingListener{Listener: raw}
	upSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Host+":"+r.URL.Path)
	})}
	go func() { _ = upSrv.Serve(up) }()
	t.Cleanup(func() { _ = upSrv.Close() })
	port := raw.Addr().(*net.TCPAddr).Port

	cfg := model.Config{
		UpstreamGroups: []model.UpstreamGroup{{
			ObjectMeta:  model.ObjectMeta{Name: "nodes"},
			Upstreams:   []model.GroupUpstream{{Upstream: model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: port}}},
			HealthCheck: model.HealthCheck{IntervalSeconds: 3600},
		}},
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "nobody"},
			DefaultAction: "deny",
		}},
		ProxyHosts: []model.ProxyHost{
			{ObjectMeta: model.ObjectMeta{Name: "open"}, Domains: []string{"open.example.com"}, UpstreamGroupRef: "nodes"},
			{ObjectMeta: model.ObjectMeta{Name: "closed"}, Domains: []string{"closed.example.com"}, UpstreamGroupRef: "nodes",
				AccessLists: []string{"nobody"}},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config does not validate: %v", err)
	}
	s := New(Config{})
	if err := s.Reload(cfg); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	t.Cleanup(s.health.stopAll)

	tln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tsrv := newListenerServer(tln.Addr().String(), s.dataHandler(s.dispatchHTTPS))
	tsrv.TLSConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
		Certificates: []tls.Certificate{testTLSCert(t)},
	}
	go func() { _ = tsrv.ServeTLS(tln, "", "") }()
	t.Cleanup(func() { _ = tsrv.Close() })

	pln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	psrv := newListenerServer(pln.Addr().String(), s.dataHandler(s.dispatchHTTP))
	go func() { _ = psrv.Serve(pln) }()
	t.Cleanup(func() { _ = psrv.Close() })
	return &requestTargetEnv{tlsAddr: tln.Addr().String(), plainAddr: pln.Addr().String(), up: up}
}

// rawExchange writes a raw HTTP/1.1 request on conn and reads one response.
func rawExchange(t *testing.T, conn net.Conn, wire string) (int, string) {
	t.Helper()
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, wire); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (e *requestTargetEnv) dial(t *testing.T, useTLS bool) net.Conn {
	t.Helper()
	if !useTLS {
		c, err := net.Dial("tcp", e.plainAddr)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c, err := tls.Dial("tcp", e.tlsAddr, &tls.Config{InsecureSkipVerify: true, ServerName: "open.example.com", NextProtos: []string{"http/1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestRequestTargetNotInOriginFormRefused: a request target not in origin form
// is answered 400 on both listeners and no upstream request is made, while an
// origin-form request to the same host is proxied as before.
func TestRequestTargetNotInOriginFormRefused(t *testing.T) {
	env := requestTargetSetup(t)
	for _, useTLS := range []bool{true, false} {
		code, body := rawExchange(t, env.dial(t, useTLS), "GET /x HTTP/1.1\r\nHost: open.example.com\r\nConnection: close\r\n\r\n")
		if code != http.StatusOK || body != "open.example.com:/x" {
			t.Fatalf("tls=%v origin-form control: %d %q", useTLS, code, body)
		}
	}
	before := env.up.accepted.Load()

	targets := []string{
		"http:http://closed.example.com/secret?x=1",
		"http:closed.example.com/secret",
		"x:admin",
	}
	for _, useTLS := range []bool{true, false} {
		for _, target := range targets {
			code, body := rawExchange(t, env.dial(t, useTLS),
				"GET "+target+" HTTP/1.1\r\nHost: open.example.com\r\nConnection: close\r\n\r\n")
			if code != http.StatusBadRequest {
				t.Errorf("tls=%v target %q: status %d (body %q), want 400", useTLS, target, code, body)
			}
		}
	}
	if got := env.up.accepted.Load(); got != before {
		t.Fatalf("upstream accepted %d connection(s) for refused targets", got-before)
	}
}

// TestRequestTargetAsteriskAndAbsoluteFormUnaffected: "OPTIONS *" and an
// absolute-form target with a path are not refused by the origin-form check.
func TestRequestTargetAsteriskAndAbsoluteFormUnaffected(t *testing.T) {
	env := requestTargetSetup(t)
	for _, useTLS := range []bool{true, false} {
		if code, _ := rawExchange(t, env.dial(t, useTLS), "OPTIONS * HTTP/1.1\r\nHost: open.example.com\r\nConnection: close\r\n\r\n"); code != http.StatusOK {
			t.Errorf("tls=%v OPTIONS *: status %d, want 200", useTLS, code)
		}
		code, body := rawExchange(t, env.dial(t, useTLS),
			"GET http://open.example.com/y HTTP/1.1\r\nHost: open.example.com\r\nConnection: close\r\n\r\n")
		if code != http.StatusOK || !strings.HasSuffix(body, ":/y") {
			t.Errorf("tls=%v absolute-form: %d %q, want 200 for /y", useTLS, code, body)
		}
	}
}

// TestRewriteClearsOpaqueTarget: the reverse proxy builds the upstream request
// line from the path even if a request with URL.Opaque reaches it.
func TestRewriteClearsOpaqueTarget(t *testing.T) {
	var gotURI string
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
	}))
	defer closeFn()
	h := newReverseProxy(up, "app", nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)
	req.URL.Opaque = "http://other.example.com/secret"
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotURI != "/" {
		t.Fatalf("upstream request target = %q, want \"/\"", gotURI)
	}
}
