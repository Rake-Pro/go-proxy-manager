package dataplane

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// pinCert is a generated test certificate and its pin (SHA-256 of the DER).
type pinCert struct {
	cert tls.Certificate
	fp   string
	pool *x509.CertPool // the certificate as a root, for normal verification
}

// newPinCert makes the appliance-style certificate pinning exists for:
// self-signed, CA:TRUE, a CN that is not the address and, unless ipSAN, no
// subjectAltName at all, valid for ten years. rsaKey selects RSA 2048 (the real
// case) over a faster P-256 key.
func newPinCert(t *testing.T, rsaKey, ipSAN bool) pinCert {
	t.Helper()
	var key crypto.Signer
	var err error
	if rsaKey {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "appliance-console"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
	}
	if ipSAN {
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	if !ipSAN && (len(leaf.DNSNames) != 0 || len(leaf.IPAddresses) != 0) {
		t.Fatal("test certificate unexpectedly carries a SAN")
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	sum := sha256.Sum256(der)
	return pinCert{cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, fp: hex.EncodeToString(sum[:]), pool: pool}
}

var rsaPinCert = struct {
	once sync.Once
	c    pinCert
}{}

func applianceCert(t *testing.T) pinCert {
	rsaPinCert.once.Do(func() { rsaPinCert.c = newPinCert(t, true, false) })
	return rsaPinCert.c
}

// colonUpper writes a pin the way openssl prints it.
func colonUpper(fp string) string {
	var parts []string
	for i := 0; i < len(fp); i += 2 {
		parts = append(parts, strings.ToUpper(fp[i:i+2]))
	}
	return strings.Join(parts, ":")
}

// pinUpstream is an HTTPS test backend that counts accepted connections and
// served requests, and whose certificate can be swapped between connections.
type pinUpstream struct {
	srv          *httptest.Server
	cert         atomic.Pointer[tls.Certificate]
	conns        atomic.Int32 // TCP connections accepted (one per handshake attempt)
	closed       atomic.Int32
	reqs         atomic.Int32
	resumed      atomic.Int32 // requests that arrived on a resumed TLS session
	port         int
	lastProtoMaj atomic.Int32
}

func newPinUpstream(t *testing.T, c pinCert, h http.Handler, h2 bool) *pinUpstream {
	t.Helper()
	p := &pinUpstream{}
	p.cert.Store(&c.cert)
	if h == nil {
		h = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "pinned") })
	}
	p.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.reqs.Add(1)
		p.lastProtoMaj.Store(int32(r.ProtoMajor))
		if r.TLS != nil && r.TLS.DidResume {
			p.resumed.Add(1)
		}
		h.ServeHTTP(w, r)
	}))
	p.srv.EnableHTTP2 = h2
	protos := []string{"http/1.1"}
	if h2 {
		protos = []string{"h2", "http/1.1"}
	}
	// Per-handshake config so the presented certificate follows p.cert; the
	// session ticket keys stay the server's, so a session survives a swap.
	p.srv.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		return &tls.Config{Certificates: []tls.Certificate{*p.cert.Load()}, NextProtos: protos}, nil
	}}
	p.srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		switch s {
		case http.StateNew:
			p.conns.Add(1)
		case http.StateClosed:
			p.closed.Add(1)
		}
	}
	p.srv.StartTLS()
	t.Cleanup(p.srv.Close)
	p.port = p.srv.Listener.Addr().(*net.TCPAddr).Port
	return p
}

func (p *pinUpstream) upstream(pins ...string) model.Upstream {
	return model.Upstream{Scheme: "https", Host: "127.0.0.1", Port: p.port, TLS: model.UpstreamTLS{PinnedCertSHA256: pins}}
}

func proxyHostTo(name string, up model.Upstream) model.ProxyHost {
	return model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: name}, Domains: []string{name + ".example.com"}, Upstream: up}
}

// proxyGet sends one request through gpm's plain-HTTP listener.
func proxyGet(t *testing.T, addr, host, path string) (int, string) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// lockedBuf is a log sink safe for the proxy's goroutines.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func capturePinLog(t *testing.T) *lockedBuf {
	t.Helper()
	buf := &lockedBuf{}
	prev := log.Logger
	log.Logger = zerolog.New(buf)
	t.Cleanup(func() { log.Logger = prev })
	return buf
}

// The pin is the whole trust decision: a self-signed, SAN-less certificate is
// accepted when (and only when) one of the pins is its fingerprint, in any of
// the accepted spellings; without pins the same upstream fails like today.
func TestPinnedUpstreamTrust(t *testing.T) {
	appliance := applianceCert(t)
	other := newPinCert(t, false, false)
	for _, tc := range []struct {
		name     string
		pins     []string
		wantCode int
	}{
		{"pin matches a self-signed certificate without SAN", []string{appliance.fp}, http.StatusOK},
		{"openssl spelling", []string{colonUpper(appliance.fp)}, http.StatusOK},
		{"second pin matches", []string{other.fp, appliance.fp}, http.StatusOK},
		{"wrong pin", []string{other.fp}, http.StatusBadGateway},
		{"no pin verifies as today", nil, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := capturePinLog(t)
			up := newPinUpstream(t, appliance, nil, false)
			// cloneConfig goes through JSON, as an API write does, so the
			// openssl spelling is normalized the way it would be stored.
			cfg := cloneConfig(t, model.Config{ProxyHosts: []model.ProxyHost{proxyHostTo("bmc", up.upstream(tc.pins...))}})
			if err := cfg.ProxyHosts[0].Validate(); err != nil {
				t.Fatal(err)
			}
			_, addr := tunnelServer(t, cfg)
			code, body := proxyGet(t, addr, "bmc.example.com", "/")
			if code != tc.wantCode {
				t.Fatalf("status = %d (%q), want %d", code, body, tc.wantCode)
			}
			if tc.wantCode == http.StatusOK {
				if body != "pinned" || up.reqs.Load() != 1 {
					t.Fatalf("body %q, upstream requests %d; want the upstream's answer once", body, up.reqs.Load())
				}
				return
			}
			if n := up.reqs.Load(); n != 0 {
				t.Fatalf("the upstream saw %d request(s) over a refused connection", n)
			}
			if up.conns.Load() == 0 {
				t.Fatal("no handshake was attempted")
			}
			if tc.pins != nil {
				l := logs.String()
				if !strings.Contains(l, `"host":"bmc"`) || !strings.Contains(l, `"presentedCertSHA256":"`+appliance.fp+`"`) ||
					!strings.Contains(l, "does not match any pinned fingerprint") {
					t.Fatalf("log does not name the host and the presented fingerprint:\n%s", l)
				}
			}
		})
	}
}

// A refused pin is classified exactly like a certificate verification failure
// today: a connect-phase error (retried by a group, counted against the member).
func TestPinMismatchClassifiedLikeVerificationFailure(t *testing.T) {
	appliance := applianceCert(t)
	up := newPinUpstream(t, appliance, nil, false)
	url := "https://127.0.0.1:" + strconv.Itoa(up.port) + "/"
	do := func(rt http.RoundTripper) error {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		resp, err := rt.RoundTrip(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		return err
	}
	wrong := newPinnedTransport(nil, model.UpstreamTLS{PinnedCertSHA256: []string{strings.Repeat("0", 64)}})
	defer wrong.CloseIdleConnections()
	pinErr := do(wrong)
	plainErr := do(dataplaneTransport)
	for name, err := range map[string]error{"pin mismatch": pinErr, "unpinned verification failure": plainErr} {
		var cv *tls.CertificateVerificationError
		if err == nil || !errors.As(err, &cv) {
			t.Fatalf("%s: err = %v, want a *tls.CertificateVerificationError", name, err)
		}
		if !isConnectError(err) {
			t.Fatalf("%s: not classified as a connect-phase error: %v", name, err)
		}
		if isUpstreamTimeout(err) {
			t.Fatalf("%s: classified as a timeout (504) instead of 502: %v", name, err)
		}
	}
	if fp, ok := pinMismatch(pinErr); !ok || fp != appliance.fp {
		t.Fatalf("pinMismatch = %q, %v; want the presented fingerprint", fp, ok)
	}
	if _, ok := pinMismatch(plainErr); ok {
		t.Fatal("an ordinary verification failure reported as a pin mismatch")
	}
	if up.reqs.Load() != 0 {
		t.Fatal("a refused handshake reached the handler")
	}
}

// Invariant 1 and the transport keying: unpinned upstreams get exactly what
// they got before (the shared transport, or the timeout clone); pinned ones get
// a transport per (timeouts, pin set) carrying the pin check, shared only by
// upstreams with the same key.
func TestPinnedTransportSelection(t *testing.T) {
	reg := &registry{states: &chainStates{}}
	plain := model.Upstream{Scheme: "https", Host: "192.0.2.10", Port: 443}
	if rt := reg.upstreamTransport(nil, plain); rt != http.RoundTripper(dataplaneTransport) {
		t.Fatal("an unpinned upstream does not use the shared transport")
	}
	if len(reg.owned) != 0 {
		t.Fatal("the shared transport was recorded as a pool the build owns")
	}
	clone, ok := reg.upstreamTransport(&model.HostTimeouts{ConnectSeconds: 5}, plain).(*http.Transport)
	if !ok || clone == dataplaneTransport || (clone.TLSClientConfig != nil && (clone.TLSClientConfig.InsecureSkipVerify || clone.TLSClientConfig.VerifyConnection != nil)) {
		t.Fatal("an unpinned upstream with timeouts does not get today's plain clone")
	}

	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	pinned := func(to *model.HostTimeouts, pins ...string) *http.Transport {
		u := plain
		u.TLS.PinnedCertSHA256 = pins
		tr, ok := reg.upstreamTransport(to, u).(*http.Transport)
		if !ok {
			t.Fatal("a pinned upstream did not get an *http.Transport")
		}
		return tr
	}
	ab := pinned(nil, a, b)
	cfg := ab.TLSClientConfig
	if ab == dataplaneTransport || cfg == nil || !cfg.InsecureSkipVerify || cfg.VerifyConnection == nil || cfg.VerifyPeerCertificate != nil {
		t.Fatal("the pinned transport does not carry the pin check as its only verification")
	}
	if cfg.ClientSessionCache != nil {
		t.Fatal("the pinned transport resumes sessions; production must not (see TestPinnedSessionResumption)")
	}
	if !ab.ForceAttemptHTTP2 || ab.MaxIdleConnsPerHost != dataplaneTransport.MaxIdleConnsPerHost || !ab.DisableCompression {
		t.Fatal("the pinned transport is not a clone of the shared one")
	}
	if pinned(nil, b, a) != ab || pinned(nil, colonUpper(a), b) != ab {
		t.Fatal("the same pin set in another order or spelling got its own transport")
	}
	if pinned(nil, a) == ab {
		t.Fatal("a different pin set shares a transport")
	}
	to := pinned(&model.HostTimeouts{ReadSeconds: 7}, a, b)
	if to == ab || to.ResponseHeaderTimeout != 7*time.Second || to.TLSClientConfig.VerifyConnection == nil {
		t.Fatal("the timeout override and the pin do not combine into their own transport")
	}
	for _, o := range reg.owned {
		if o == idleCloser(dataplaneTransport) {
			t.Fatal("the shared transport was recorded as owned")
		}
	}
}

// Invariant 2, proven on the wire: a connection opened under a pin never
// serves an unpinned or differently pinned upstream at the same address, and a
// pinned request never rides a connection that was verified the normal way.
func TestPinnedPoolIsolation(t *testing.T) {
	t.Run("pinned connection is not reused for others", func(t *testing.T) {
		appliance := applianceCert(t)
		up := newPinUpstream(t, appliance, nil, false)
		cfg := model.Config{ProxyHosts: []model.ProxyHost{
			proxyHostTo("pinned", up.upstream(appliance.fp)),
			proxyHostTo("plain", up.upstream()),
			proxyHostTo("pinned2", up.upstream(appliance.fp, strings.Repeat("c", 64))),
		}}
		_, addr := tunnelServer(t, cfg)
		step := func(host string, wantCode int, wantConns int32) {
			t.Helper()
			code, _ := proxyGet(t, addr, host+".example.com", "/")
			if code != wantCode || up.conns.Load() != wantConns {
				t.Fatalf("%s: status %d, connections %d; want %d and %d", host, code, up.conns.Load(), wantCode, wantConns)
			}
		}
		step("pinned", 200, 1)
		step("pinned", 200, 1) // pooled: the counts below mean something
		step("plain", 502, 2)  // a reused pinned connection would have answered 200
		step("pinned2", 200, 3)
		step("pinned", 200, 3)
		step("pinned2", 200, 3)
	})
	t.Run("normally verified connection is not reused for a pin", func(t *testing.T) {
		trusted := newPinCert(t, false, true)
		up := newPinUpstream(t, trusted, nil, false)
		// Let the unpinned upstream verify the normal way against a test root.
		prev := dataplaneTransport.TLSClientConfig
		dataplaneTransport.TLSClientConfig = &tls.Config{RootCAs: trusted.pool}
		t.Cleanup(func() {
			dataplaneTransport.TLSClientConfig = prev
			dataplaneTransport.CloseIdleConnections()
		})
		cfg := model.Config{ProxyHosts: []model.ProxyHost{
			proxyHostTo("plain", up.upstream()),
			proxyHostTo("pinned", up.upstream(trusted.fp)),
		}}
		_, addr := tunnelServer(t, cfg)
		for i, s := range []struct {
			host  string
			conns int32
		}{{"plain", 1}, {"plain", 1}, {"pinned", 2}, {"plain", 2}, {"pinned", 2}} {
			if code, _ := proxyGet(t, addr, s.host+".example.com", "/"); code != 200 || up.conns.Load() != s.conns {
				t.Fatalf("step %d (%s): status %d, connections %d; want 200 and %d", i, s.host, code, up.conns.Load(), s.conns)
			}
		}
	})
}

// Invariant 5: the pin is checked on a resumed session too. Production never
// resumes (no ClientSessionCache anywhere), so the test supplies one, shared
// across two pin sets, against a server whose certificate changed between the
// connections. On resumption the server presents no certificate; the session
// carries the one first verified, and VerifyConnection checks THAT against the
// current pins. VerifyPeerCertificate would not run at all here.
func TestPinnedSessionResumption(t *testing.T) {
	first := newPinCert(t, false, false)
	second := newPinCert(t, false, false)
	up := newPinUpstream(t, first, nil, false)
	url := "https://127.0.0.1:" + strconv.Itoa(up.port) + "/"
	get := func(cache tls.ClientSessionCache, pins ...string) error {
		tr := newPinnedTransport(nil, model.UpstreamTLS{PinnedCertSHA256: pins})
		tr.TLSClientConfig.ClientSessionCache = cache
		tr.DisableKeepAlives = true
		defer tr.CloseIdleConnections()
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body) // reading lets the TLS 1.3 ticket arrive
		return resp.Body.Close()
	}
	cache := tls.NewLRUClientSessionCache(4)
	if err := get(cache, first.fp); err != nil {
		t.Fatalf("first connection: %v", err)
	}
	up.cert.Store(&second.cert)

	// Control: the cached session does resume, and it carries the first
	// certificate (a pin on the second alone would refuse it, below).
	if err := get(cache, first.fp, second.fp); err != nil || up.resumed.Load() != 1 {
		t.Fatalf("control resumption: err %v, resumed %d; want a resumed session", err, up.resumed.Load())
	}
	reqs := up.reqs.Load()
	err := get(cache, second.fp)
	if fp, ok := pinMismatch(err); !ok || fp != first.fp {
		t.Fatalf("resumed session under a pin on the new certificate only: err %v, want refused with the session's certificate %s", err, first.fp)
	}
	if up.reqs.Load() != reqs {
		t.Fatal("the refused resumed session carried a request")
	}
	// A full handshake presents the new certificate, which that pin accepts.
	if err := get(tls.NewLRUClientSessionCache(4), second.fp); err != nil {
		t.Fatalf("fresh handshake with the new certificate: %v", err)
	}
}

// ALPN and HTTP/2 negotiation are unchanged by a pin.
func TestPinnedUpstreamNegotiatesHTTP2(t *testing.T) {
	c := newPinCert(t, false, false)
	up := newPinUpstream(t, c, nil, true)
	_, addr := tunnelServer(t, model.Config{ProxyHosts: []model.ProxyHost{proxyHostTo("h2", up.upstream(c.fp))}})
	if code, _ := proxyGet(t, addr, "h2.example.com", "/"); code != 200 || up.lastProtoMaj.Load() != 2 {
		t.Fatalf("status %d, upstream protocol HTTP/%d; want 200 over HTTP/2", code, up.lastProtoMaj.Load())
	}
}

func TestPinnedLocationUpstream(t *testing.T) {
	c := newPinCert(t, false, false)
	wrong := strings.Repeat("d", 64)
	for _, tc := range []struct {
		name     string
		pin      string
		wantCode int
	}{{"matching pin", c.fp, 200}, {"wrong pin", wrong, 502}} {
		t.Run(tc.name, func(t *testing.T) {
			up := newPinUpstream(t, c, nil, false)
			hostUp, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "host") }))
			defer closeFn()
			lu := up.upstream(tc.pin)
			h := proxyHostTo("app", hostUp)
			h.Locations = []model.Location{{Path: "/bmc", Upstream: &lu}}
			_, addr := tunnelServer(t, model.Config{ProxyHosts: []model.ProxyHost{h}})
			if code, body := proxyGet(t, addr, "app.example.com", "/"); code != 200 || body != "host" {
				t.Fatalf("host route: %d %q", code, body)
			}
			code, body := proxyGet(t, addr, "app.example.com", "/bmc/x")
			if code != tc.wantCode {
				t.Fatalf("location: status %d (%q), want %d", code, body, tc.wantCode)
			}
			if tc.wantCode == 200 && body != "pinned" {
				t.Fatalf("location body %q", body)
			}
			if tc.wantCode != 200 && up.reqs.Load() != 0 {
				t.Fatal("the location upstream saw a request over a refused connection")
			}
		})
	}
}

func groupStatus(s *Server, group, label string) (UpstreamStatus, bool) {
	for _, st := range s.health.snapshot()[group] {
		if st.Upstream == label {
			return st, true
		}
	}
	return UpstreamStatus{}, false
}

// Each group member carries its own pin on the failover path: a member whose
// pin does not match is refused as a connect-phase failure (retried on the next
// member, counted against the refused one) and never sees the request.
func TestPinnedUpstreamGroupFailover(t *testing.T) {
	a, b := newPinCert(t, false, false), newPinCert(t, false, false)
	s1 := newPinUpstream(t, a, nil, false) // presents a, pinned to b: refused
	s2 := newPinUpstream(t, b, nil, false)
	g := model.UpstreamGroup{
		ObjectMeta: model.ObjectMeta{Name: "grp"},
		Upstreams: []model.GroupUpstream{
			{Upstream: s1.upstream(b.fp)},
			{Upstream: s2.upstream(b.fp)},
		},
		// TCP probe, long interval: only the passive accounting moves health.
		HealthCheck: model.HealthCheck{IntervalSeconds: 3600, Fall: 1},
	}
	h := model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"}, UpstreamGroupRef: "grp"}
	cfg := model.Config{UpstreamGroups: []model.UpstreamGroup{g}, ProxyHosts: []model.ProxyHost{h}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	logs := capturePinLog(t)
	s, addr := tunnelServer(t, cfg)
	if code, body := proxyGet(t, addr, "app.example.com", "/"); code != 200 || body != "pinned" {
		t.Fatalf("status %d (%q), want the failover to the second member", code, body)
	}
	var failover string
	for _, l := range strings.Split(logs.String(), "\n") {
		if strings.Contains(l, "upstream connect failed, trying next") {
			failover = l
		}
	}
	if !strings.Contains(failover, `"group":"grp"`) || !strings.Contains(failover, `"upstream":"`+upstreamLabel(s1.upstream())+`"`) ||
		!strings.Contains(failover, `"presentedCertSHA256":"`+a.fp+`"`) {
		t.Fatalf("failover log line does not name the group, the member and the presented fingerprint: %q", failover)
	}
	if s1.reqs.Load() != 0 || s1.conns.Load() == 0 || s2.reqs.Load() != 1 {
		t.Fatalf("first member: %d request(s), %d connection(s); second: %d request(s)", s1.reqs.Load(), s1.conns.Load(), s2.reqs.Load())
	}
	if st, ok := groupStatus(s, "grp", upstreamLabel(s1.upstream())); !ok || st.Healthy {
		t.Fatalf("refused member status = %+v, want marked unhealthy by the passive accounting", st)
	}
	if st, ok := groupStatus(s, "grp", upstreamLabel(s2.upstream())); !ok || !st.Healthy {
		t.Fatalf("good member status = %+v, want healthy", st)
	}
}

// An https health probe of a pinned member uses the member's pin. Unpinned,
// the probe verifies against the system roots, so the same self-signed member
// probes as down - which is what it does today.
func TestPinnedUpstreamGroupHealthProbe(t *testing.T) {
	a, b := newPinCert(t, false, false), newPinCert(t, false, false)
	wrong := newPinUpstream(t, a, nil, false)
	good := newPinUpstream(t, b, nil, false)
	unpinned := newPinUpstream(t, b, nil, false)
	g := model.UpstreamGroup{
		ObjectMeta: model.ObjectMeta{Name: "grp"},
		Upstreams: []model.GroupUpstream{
			{Upstream: wrong.upstream(b.fp)},
			{Upstream: good.upstream(b.fp)},
			{Upstream: unpinned.upstream()},
		},
		HealthCheck: model.HealthCheck{Path: "/healthz", IntervalSeconds: 3600, Rise: 1, Fall: 1},
	}
	s, _ := tunnelServer(t, model.Config{UpstreamGroups: []model.UpstreamGroup{g}})
	deadline := time.Now().Add(5 * time.Second)
	for {
		w, _ := groupStatus(s, "grp", upstreamLabel(wrong.upstream()))
		u, _ := groupStatus(s, "grp", upstreamLabel(unpinned.upstream()))
		if !w.Healthy && !u.Healthy && good.reqs.Load() > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("probe round: wrong-pin healthy=%v, unpinned healthy=%v, pinned probe requests=%d", w.Healthy, u.Healthy, good.reqs.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st, _ := groupStatus(s, "grp", upstreamLabel(good.upstream())); !st.Healthy {
		t.Fatal("the pinned member probed as down")
	}
	if wrong.reqs.Load() != 0 || unpinned.reqs.Load() != 0 {
		t.Fatal("a probe reached a member over a refused connection")
	}
	if wrong.conns.Load() == 0 || unpinned.conns.Load() == 0 {
		t.Fatal("a refused member was never probed")
	}
}

// A WebSocket upgrade rides the pinned transport too.
func TestPinnedUpstreamWebSocket(t *testing.T) {
	c := newPinCert(t, false, false)
	echo := pinEchoHandler(t)
	t.Run("matching pin", func(t *testing.T) {
		up := newPinUpstream(t, c, echo, false)
		_, addr := tunnelServer(t, model.Config{ProxyHosts: []model.ProxyHost{proxyHostTo("ws", up.upstream(c.fp))}})
		conn, br := openTunnel(t, addr, "ws.example.com")
		tunnelEcho(t, conn, br)
	})
	t.Run("wrong pin", func(t *testing.T) {
		up := newPinUpstream(t, c, echo, false)
		_, addr := tunnelServer(t, model.Config{ProxyHosts: []model.ProxyHost{proxyHostTo("ws", up.upstream(strings.Repeat("e", 64)))}})
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: ws.example.com\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		line, err := readStatusLine(conn)
		if err != nil || !strings.Contains(line, " 502 ") {
			t.Fatalf("upgrade answered %q (%v), want 502", line, err)
		}
		if up.reqs.Load() != 0 {
			t.Fatal("the upgrade reached the upstream over a refused connection")
		}
	})
}

// pinEchoHandler switches protocols and echoes every line back.
func pinEchoHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: " + r.Header.Get("Upgrade") + "\r\nConnection: Upgrade\r\n\r\n")
		_ = brw.Flush()
		for {
			line, err := brw.ReadString('\n')
			if err != nil {
				return
			}
			_, _ = brw.WriteString("echo:" + line)
			_ = brw.Flush()
		}
	})
}

func readStatusLine(c net.Conn) (string, error) {
	var b []byte
	one := make([]byte, 1)
	for len(b) < 256 {
		if _, err := c.Read(one); err != nil {
			return string(b), err
		}
		if one[0] == '\n' {
			break
		}
		b = append(b, one[0])
	}
	return string(b), nil
}

// A reload with the same pin keeps the pinned pool (the same connection serves
// the next request); a reload with a changed pin set does not reuse it, and the
// replaced pool's idle connection is closed.
func TestPinnedPoolAcrossReload(t *testing.T) {
	c := applianceCert(t)
	up := newPinUpstream(t, c, nil, false)
	cfg := model.Config{ProxyHosts: []model.ProxyHost{proxyHostTo("bmc", up.upstream(c.fp))}}
	s, addr := tunnelServer(t, cfg)
	get := func(wantConns int32) {
		t.Helper()
		if code, _ := proxyGet(t, addr, "bmc.example.com", "/"); code != 200 || up.conns.Load() != wantConns {
			t.Fatalf("status %d, connections %d; want 200 and %d", code, up.conns.Load(), wantConns)
		}
	}
	get(1)
	if err := s.Reload(cloneConfig(t, cfg)); err != nil {
		t.Fatal(err)
	}
	get(1)
	if n := up.closed.Load(); n != 0 {
		t.Fatalf("an unchanged pin closed %d pooled connection(s)", n)
	}
	changed := cloneConfig(t, cfg)
	changed.ProxyHosts[0].Upstream.TLS.PinnedCertSHA256 = []string{strings.Repeat("f", 64), c.fp}
	if err := s.Reload(changed); err != nil {
		t.Fatal(err)
	}
	get(2)
	if !waitForCount(&up.closed, 1, 2*time.Second) {
		t.Fatal("the replaced pin set's idle connection is still open")
	}
}

// Invariant 5's guard: InsecureSkipVerify appears in exactly one place in the
// non-test code, as a field of a tls.Config literal that also sets
// VerifyConnection (pinnedTLSConfig). Any other use fails here.
func TestInsecureSkipVerifyOnlyWithPinCheck(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root not found: %v", err)
	}
	var allowed, total int
	var where []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "testdata", "node_modules", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil // not Go this package builds; the compiler owns that
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				if n.Name == "InsecureSkipVerify" {
					total++
					where = append(where, rel)
				}
			case *ast.CompositeLit:
				keys := map[string]bool{}
				for _, e := range n.Elts {
					if kv, ok := e.(*ast.KeyValueExpr); ok {
						if id, ok := kv.Key.(*ast.Ident); ok {
							keys[id.Name] = true
						}
					}
				}
				if keys["InsecureSkipVerify"] && keys["VerifyConnection"] && filepath.ToSlash(rel) == "internal/dataplane/upstreampin.go" {
					allowed++
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if allowed != 1 || total != 1 {
		t.Fatalf("InsecureSkipVerify used %d time(s) (%v), %d of them in the pinned tls.Config with VerifyConnection; want exactly one, there", total, where, allowed)
	}
}

// checkCertPin compares the full 32-byte digest: a pin that shares the first
// 31 bytes, or only the first 4, with the presented certificate is refused,
// and so is a connection that presented no certificate at all.
func TestCheckCertPinComparesFullDigest(t *testing.T) {
	c := newPinCert(t, false, false)
	cs := tls.ConnectionState{PeerCertificates: []*x509.Certificate{c.cert.Leaf}}
	sum := sha256.Sum256(c.cert.Leaf.Raw)
	if err := checkCertPin(cs, [][sha256.Size]byte{sum}); err != nil {
		t.Fatalf("exact pin refused: %v", err)
	}
	lastByte := sum
	lastByte[sha256.Size-1] ^= 1
	prefix4 := sum
	for i := 4; i < sha256.Size; i++ {
		prefix4[i] ^= 0xff
	}
	for name, pins := range map[string][][sha256.Size]byte{
		"same first 31 bytes":  {lastByte},
		"same first 4 bytes":   {prefix4},
		"both near misses":     {lastByte, prefix4},
		"no usable pin at all": nil,
	} {
		err := checkCertPin(cs, pins)
		var cv *tls.CertificateVerificationError
		if fp, ok := pinMismatch(err); !ok || fp != c.fp || !errors.As(err, &cv) {
			t.Errorf("%s: err = %v, want refused naming %s", name, err, c.fp)
		}
	}
	for name, cs := range map[string]tls.ConnectionState{
		"nil peer certificates":   {},
		"empty peer certificates": {PeerCertificates: []*x509.Certificate{}},
	} {
		err := checkCertPin(cs, [][sha256.Size]byte{sum})
		var cv *tls.CertificateVerificationError
		if fp, ok := pinMismatch(err); !ok || fp != "" || !errors.As(err, &cv) {
			t.Errorf("%s: err = %v, want refused as no certificate", name, err)
		}
	}
}

// A pin list of only malformed entries (an unvalidated config) is still a pin:
// it refuses every handshake, and never falls back to normal verification,
// even for an upstream that normal verification trusts.
func TestMalformedPinsFailClosed(t *testing.T) {
	trusted := newPinCert(t, false, true)
	up := newPinUpstream(t, trusted, nil, false)
	prev := dataplaneTransport.TLSClientConfig
	dataplaneTransport.TLSClientConfig = &tls.Config{RootCAs: trusted.pool}
	t.Cleanup(func() {
		dataplaneTransport.TLSClientConfig = prev
		dataplaneTransport.CloseIdleConnections()
	})
	malformed := up.upstream("zz", strings.Repeat("g", 64), "AB:CD")
	loc := proxyHostTo("loc", up.upstream())
	loc.Locations = []model.Location{{Path: "/bmc", Upstream: &malformed}}
	cfg := model.Config{ProxyHosts: []model.ProxyHost{
		proxyHostTo("plain", up.upstream()),
		proxyHostTo("one", up.upstream("zz")),
		proxyHostTo("all", malformed),
		loc,
	}}
	_, addr := tunnelServer(t, cfg)
	if code, _ := proxyGet(t, addr, "plain.example.com", "/"); code != 200 {
		t.Fatalf("control: unpinned upstream answered %d, want 200 (trusted by normal verification)", code)
	}
	for _, r := range []struct{ host, path string }{{"one", "/"}, {"all", "/"}, {"loc", "/bmc/x"}} {
		if code, _ := proxyGet(t, addr, r.host+".example.com", r.path); code != 502 {
			t.Errorf("%s%s with only malformed pins: %d, want 502", r.host, r.path, code)
		}
	}
	if n := up.reqs.Load(); n != 1 {
		t.Fatalf("upstream saw %d request(s), want only the control's", n)
	}
}

// Pins on a plain-http upstream in a config that skipped validation fail the
// router build: never proxied in the clear with the pin ignored.
func TestPinsOverPlainHTTPFailTheBuild(t *testing.T) {
	pin := strings.Repeat("a", 64)
	plain := model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9, TLS: model.UpstreamTLS{PinnedCertSHA256: []string{pin}}}
	loc := proxyHostTo("loc", model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9})
	loc.Locations = []model.Location{{Path: "/x", Upstream: &plain}}
	for name, cfg := range map[string]model.Config{
		"host upstream":     {ProxyHosts: []model.ProxyHost{proxyHostTo("bmc", plain)}},
		"location upstream": {ProxyHosts: []model.ProxyHost{loc}},
		"group member": {
			UpstreamGroups: []model.UpstreamGroup{{ObjectMeta: model.ObjectMeta{Name: "g"}, Upstreams: []model.GroupUpstream{{Upstream: plain}}}},
			ProxyHosts:     []model.ProxyHost{{ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"}, UpstreamGroupRef: "g"}},
		},
	} {
		if err := New(Config{}).Reload(cfg); err == nil || !strings.Contains(err.Error(), "requires scheme https") {
			t.Errorf("%s: Reload = %v, want the build refused", name, err)
		}
	}
}

// A group member's pin change on reload takes effect for traffic: a now-wrong
// pin is refused, and so is the self-signed member once its pin is removed.
func TestPinnedGroupMemberPinChangeOnReload(t *testing.T) {
	a := newPinCert(t, false, false)
	s1 := newPinUpstream(t, a, nil, false)
	g := model.UpstreamGroup{ObjectMeta: model.ObjectMeta{Name: "grp"},
		Upstreams:   []model.GroupUpstream{{Upstream: s1.upstream(a.fp)}},
		HealthCheck: model.HealthCheck{IntervalSeconds: 3600}}
	h := model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"}, UpstreamGroupRef: "grp"}
	cfg := model.Config{UpstreamGroups: []model.UpstreamGroup{g}, ProxyHosts: []model.ProxyHost{h}}
	s, addr := tunnelServer(t, cfg)
	if code, _ := proxyGet(t, addr, "app.example.com", "/"); code != 200 {
		t.Fatalf("matching member pin: %d, want 200", code)
	}
	for _, pins := range [][]string{{strings.Repeat("2", 64)}, nil} {
		next := cloneConfig(t, cfg)
		next.UpstreamGroups[0].Upstreams[0].TLS.PinnedCertSHA256 = pins
		if err := s.Reload(next); err != nil {
			t.Fatal(err)
		}
		if code, _ := proxyGet(t, addr, "app.example.com", "/"); code != 502 {
			t.Fatalf("member pins %v after reload: %d, want 502", pins, code)
		}
	}
}

// An open tunnel follows the pin set of the upstream it rides, on a host
// upstream, a location's own upstream and an upstream-group member: a reload
// that adds, removes or replaces a pin closes it, a reload that leaves the pin
// set as it was (same config, another order, another spelling) keeps it open.
func TestPinnedTunnelFollowsPinSet(t *testing.T) {
	c := newPinCert(t, false, false)
	spare, other := strings.Repeat("4", 64), strings.Repeat("3", 64)
	plain, closeFn := hijackEcho(t)
	defer closeFn()
	setups := map[string]struct {
		cfg  func(up model.Upstream) model.Config
		pins func(*model.Config) *[]string
	}{
		"host upstream": {
			func(up model.Upstream) model.Config {
				return model.Config{ProxyHosts: []model.ProxyHost{proxyHostTo("ws", up)}}
			},
			func(c *model.Config) *[]string { return &c.ProxyHosts[0].Upstream.TLS.PinnedCertSHA256 },
		},
		"location upstream": {
			func(up model.Upstream) model.Config {
				h := proxyHostTo("ws", plain)
				h.Locations = []model.Location{{Path: "/ws", Upstream: &up}}
				return model.Config{ProxyHosts: []model.ProxyHost{h}}
			},
			func(c *model.Config) *[]string { return &c.ProxyHosts[0].Locations[0].Upstream.TLS.PinnedCertSHA256 },
		},
		"group member": {
			func(up model.Upstream) model.Config {
				return model.Config{
					UpstreamGroups: []model.UpstreamGroup{{ObjectMeta: model.ObjectMeta{Name: "grp"},
						Upstreams:   []model.GroupUpstream{{Upstream: up}},
						HealthCheck: model.HealthCheck{IntervalSeconds: 3600}}},
					ProxyHosts: []model.ProxyHost{{ObjectMeta: model.ObjectMeta{Name: "ws"}, Domains: []string{"ws.example.com"}, UpstreamGroupRef: "grp"}},
				}
			},
			func(c *model.Config) *[]string { return &c.UpstreamGroups[0].Upstreams[0].TLS.PinnedCertSHA256 },
		},
		"location group member": {
			func(up model.Upstream) model.Config {
				h := proxyHostTo("ws", plain)
				h.Locations = []model.Location{{Path: "/ws", UpstreamGroupRef: "grp"}}
				return model.Config{
					UpstreamGroups: []model.UpstreamGroup{{ObjectMeta: model.ObjectMeta{Name: "grp"},
						Upstreams:   []model.GroupUpstream{{Upstream: up}},
						HealthCheck: model.HealthCheck{IntervalSeconds: 3600}}},
					ProxyHosts: []model.ProxyHost{h},
				}
			},
			func(c *model.Config) *[]string { return &c.UpstreamGroups[0].Upstreams[0].TLS.PinnedCertSHA256 },
		},
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			up := newPinUpstream(t, c, pinEchoHandler(t), false)
			cfg := setup.cfg(up.upstream(c.fp, spare))
			s, addr := tunnelServer(t, cfg)
			conn, br := openTunnel(t, addr, "ws.example.com")
			tunnelEcho(t, conn, br)
			for _, keep := range [][]string{{c.fp, spare}, {spare, c.fp}, {colonUpper(c.fp), spare}} {
				next := cloneConfig(t, cfg)
				*setup.pins(&next) = keep
				if err := s.Reload(next); err != nil {
					t.Fatal(err)
				}
				tunnelEcho(t, conn, br)
			}
			for _, change := range []struct {
				name string
				pins []string
			}{{"added", []string{c.fp, spare, other}}, {"removed one", []string{c.fp}}, {"replaced", []string{other}}, {"removed all", nil}} {
				next := cloneConfig(t, cfg)
				*setup.pins(&next) = change.pins
				if err := s.Reload(next); err != nil {
					t.Fatal(err)
				}
				tunnelClosed(t, conn, br)
				// Back to the original pins, on a fresh tunnel, for the next change.
				if err := s.Reload(cloneConfig(t, cfg)); err != nil {
					t.Fatal(err)
				}
				conn, br = openTunnel(t, addr, "ws.example.com")
				tunnelEcho(t, conn, br)
			}
		})
	}
}

// Every pinned upstream counts on its own in the tunnel key: a pin change on
// the upstream a tunnel rides closes it when another pinned location or member
// sits next to it, even when two locations share a path (validation accepts
// that; the first one serves).
func TestPinnedTunnelKeyPerUpstream(t *testing.T) {
	c := newPinCert(t, false, false)
	spare := strings.Repeat("4", 64)
	plain, closeFn := hijackEcho(t)
	defer closeFn()
	locations := func(paths ...string) func(up model.Upstream) model.Config {
		return func(up model.Upstream) model.Config {
			h := proxyHostTo("ws", plain)
			for _, p := range paths {
				u := up.Clone()
				h.Locations = append(h.Locations, model.Location{Path: p, Upstream: &u})
			}
			return model.Config{ProxyHosts: []model.ProxyHost{h}}
		}
	}
	setups := map[string]struct {
		cfg  func(up model.Upstream) model.Config
		pins func(*model.Config) *[]string
	}{
		"two locations": {
			locations("/ws", "/zz"),
			func(c *model.Config) *[]string { return &c.ProxyHosts[0].Locations[0].Upstream.TLS.PinnedCertSHA256 },
		},
		"duplicate location path": {
			locations("/ws", "/ws"),
			func(c *model.Config) *[]string { return &c.ProxyHosts[0].Locations[0].Upstream.TLS.PinnedCertSHA256 },
		},
		"two group members": {
			func(up model.Upstream) model.Config {
				second := newPinUpstream(t, c, pinEchoHandler(t), false).upstream(c.fp)
				return model.Config{
					UpstreamGroups: []model.UpstreamGroup{{ObjectMeta: model.ObjectMeta{Name: "grp"},
						Upstreams:   []model.GroupUpstream{{Upstream: up}, {Upstream: second}},
						HealthCheck: model.HealthCheck{IntervalSeconds: 3600}}},
					ProxyHosts: []model.ProxyHost{{ObjectMeta: model.ObjectMeta{Name: "ws"}, Domains: []string{"ws.example.com"}, UpstreamGroupRef: "grp"}},
				}
			},
			func(c *model.Config) *[]string { return &c.UpstreamGroups[0].Upstreams[0].TLS.PinnedCertSHA256 },
		},
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			up := newPinUpstream(t, c, pinEchoHandler(t), false)
			cfg := setup.cfg(up.upstream(c.fp))
			if err := cfg.ProxyHosts[0].Validate(); err != nil {
				t.Fatal(err)
			}
			s, addr := tunnelServer(t, cfg)
			conn, br := openTunnel(t, addr, "ws.example.com")
			tunnelEcho(t, conn, br)
			next := cloneConfig(t, cfg)
			*setup.pins(&next) = []string{spare}
			if err := s.Reload(next); err != nil {
				t.Fatal(err)
			}
			// The changed upstream is the one that serves.
			if code, _ := proxyGet(t, addr, "ws.example.com", "/ws"); code != http.StatusBadGateway {
				t.Errorf("request after the pin change: %d, want 502", code)
			}
			tunnelClosed(t, conn, br)
		})
	}
}

// A pin change on one host leaves another host's tunnel open, and an unpinned
// host whose upstream gains a pin list that does not decode (only an
// unvalidated config can carry one) has its tunnel closed: "only malformed
// pins" is not "no pins".
func TestPinnedTunnelOtherHostAndMalformedPins(t *testing.T) {
	c := newPinCert(t, false, false)
	plain, closeFn := hijackEcho(t)
	defer closeFn()
	up := newPinUpstream(t, c, pinEchoHandler(t), false)
	cfg := model.Config{ProxyHosts: []model.ProxyHost{proxyHostTo("a", up.upstream(c.fp)), proxyHostTo("b", plain)}}
	s, addr := tunnelServer(t, cfg)
	ca, bra := openTunnel(t, addr, "a.example.com")
	cb, brb := openTunnel(t, addr, "b.example.com")
	next := cloneConfig(t, cfg)
	next.ProxyHosts[0].Upstream.TLS.PinnedCertSHA256 = []string{strings.Repeat("4", 64)}
	if err := s.Reload(next); err != nil {
		t.Fatal(err)
	}
	tunnelClosed(t, ca, bra)
	tunnelEcho(t, cb, brb)
	bad := cloneConfig(t, next)
	// The upstream address is not part of the key, only its pins.
	bad.ProxyHosts[1].Upstream = up.upstream("zz")
	if err := s.Reload(bad); err != nil {
		t.Fatal(err)
	}
	tunnelClosed(t, cb, brb)
}
