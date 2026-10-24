package dataplane

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// Regression tests for the 2026-10-18 review follow-ups (issues #49, #50, #51).

func TestIPBucketKeyGroupsIPv6ByPrefix(t *testing.T) {
	a := ipBucketKey(net.ParseIP("2001:db8:1:2::10"))
	b := ipBucketKey(net.ParseIP("2001:db8:1:2:ffff::1"))
	c := ipBucketKey(net.ParseIP("2001:db8:1:3::10"))
	if a != b {
		t.Fatalf("two addresses in one /64 must share a key: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("different /64s must not share a key: %q", a)
	}
	if !strings.HasSuffix(a, "/64") {
		t.Fatalf("IPv6 key should name the prefix: %q", a)
	}
	if got := ipBucketKey(net.ParseIP("192.0.2.10")); got != "192.0.2.10" {
		t.Fatalf("IPv4 key = %q, want the address", got)
	}
	if got := ipBucketKey(nil); got != "" {
		t.Fatalf("nil key = %q, want empty", got)
	}
}

func TestValidHostHeader(t *testing.T) {
	ok := []string{"app.example.com", "app.example.com:443", "app.example.com:8080", "[2001:db8::1]", "[2001:db8::1]:443", "[fe80::1%25eth0]:443", "[fe80::1%eth0]", "localhost:1", "a:65535", "App.Example.COM.", "192.0.2.1:80"}
	bad := []string{"", ":443", "app.example.com:", "app.example.com:0", "app.example.com:65536", "app.example.com:443x", "app.example.com:4 43", "app/evil.example.com", "[2001:db8::1", "[2001:db8::1]x", "[2001:db8::1]:", "[not-an-ip]", "[192.0.2.1]", "app.example.com:0443", "app.example.com:443:1", "a b.example.com", "app.example.com?x=1", "app.example.com#f", "user@app.example.com"}
	for _, h := range ok {
		if !validHostHeader(h) {
			t.Errorf("%q must be accepted", h)
		}
	}
	for _, h := range bad {
		if validHostHeader(h) {
			t.Errorf("%q must be refused", h)
		}
	}
}

func TestOverrideHeadersStrippedFromUntrustedPeers(t *testing.T) {
	h := http.Header{}
	for _, name := range []string{"X-Original-URL", "X-Rewrite-URL", "X-HTTP-Method-Override", "X-Method-Override", "True-Client-IP", "X-Client-IP", "CF-Connecting-IP", "X-Real-IP", "Forwarded", "X-Forwarded-Uri", "X-Forwarded-Method"} {
		h.Set(name, "x")
	}
	h.Set("X-Keep-Me", "y")
	h.Set("Remote-User", "root")
	r := httptest.NewRequest("GET", "https://app.example.com/", nil)
	r.RemoteAddr = "198.51.100.4:1234"
	r.Header = h
	(&hostHandler{}).stripUntrustedIdentity(r)
	for name := range h {
		if name != "X-Keep-Me" {
			t.Errorf("header %q survived the strip", name)
		}
	}
	if h.Get("X-Keep-Me") != "y" {
		t.Fatal("an unrelated header must survive")
	}
}

func TestRefuseLinkLocalDial(t *testing.T) {
	if err := refuseLinkLocal("tcp", "169.254.169.254:80", nil); err == nil {
		t.Fatal("a link-local destination must be refused")
	}
	if err := refuseLinkLocal("tcp", "[fe80::1]:80", nil); err == nil {
		t.Fatal("an IPv6 link-local destination must be refused")
	}
	if err := refuseLinkLocal("tcp", "[fe80::1%eth0]:80", nil); err == nil {
		t.Fatal("a zoned IPv6 link-local destination must be refused")
	}
	if err := refuseLinkLocal("tcp", "10.0.0.5:8080", nil); err != nil {
		t.Fatalf("a private destination must be allowed: %v", err)
	}
}

// The affinity cookie names no upstream: its payload carries an opaque tag,
// which the group resolves back to the member it stands for.
func TestStickyCookieIsOpaque(t *testing.T) {
	mk := func(tag string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tag) }))
	}
	a, b := mk("A"), mk("B")
	defer a.Close()
	defer b.Close()
	g := newGroupHealth(model.UpstreamGroup{
		ObjectMeta: model.ObjectMeta{Name: "g"},
		Policy:     model.PolicyRoundRobin,
		Stickiness: &model.Stickiness{TTL: "1h"},
		Upstreams:  gups(upstreamOf(t, a), upstreamOf(t, b)),
	})
	tr := &groupTransport{gh: g, base: http.DefaultTransport}
	req, _ := http.NewRequest(http.MethodGet, "http://placeholder/", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %+v, want one", cookies)
	}
	payload, ok := verifyToken(macLabelSticky, cookies[0].Value)
	if !ok {
		t.Fatal("cookie must verify")
	}
	for _, u := range g.ups {
		if strings.Contains(string(payload), u.label) {
			t.Fatalf("cookie payload %q names upstream %q", payload, u.label)
		}
	}
	pinned := g.stickyUpstream(func() *http.Request {
		r, _ := http.NewRequest("GET", "http://placeholder/", nil)
		r.AddCookie(cookies[0])
		return r
	}())
	if pinned == nil {
		t.Fatal("the group must resolve its own cookie")
	}
	// The tag resolves to the upstream that answered, not merely to a member.
	want := map[string]string{"A": g.ups[0].label, "B": g.ups[1].label}[string(body)]
	if pinned.label != want {
		t.Fatalf("cookie pins %q, but %q answered", pinned.label, want)
	}
}

// A certificate revoked after its handshake is refused on the next request,
// without waiting for the connection to re-handshake.
func TestRevokedNowFollowsTheCurrentCRL(t *testing.T) {
	caPEM, caCert, caKey := crlCA(t, "corp")
	_ = caPEM
	_, leaf := clientCertSerial(t, caCert, caKey, 42, "alice")
	anchor := &clientCAAnchor{name: "corp", issuers: []*x509.Certificate{caCert}, crlConfigured: true}
	list, err := parseCRL(makeCRL(t, caCert, caKey, time.Now().Add(time.Hour)), anchor.issuers)
	if err != nil {
		t.Fatal(err)
	}
	anchor.crl.Store(list)
	registerCRLAnchors(map[string]*clientCAAnchor{"corp": anchor})
	t.Cleanup(func() { registerCRLAnchors(nil) })
	chains := [][]*x509.Certificate{{leaf, caCert}}
	if revokedNow(chains) {
		t.Fatal("an unrevoked certificate must pass")
	}
	list, err = parseCRL(makeCRL(t, caCert, caKey, time.Now().Add(time.Hour), 42), anchor.issuers)
	if err != nil {
		t.Fatal(err)
	}
	anchor.crl.Store(list)
	if !revokedNow(chains) {
		t.Fatal("a certificate on the current CRL must be refused")
	}
	// Another CA's list never speaks for this certificate.
	_, otherCA, otherKey := crlCA(t, "other")
	other := &clientCAAnchor{name: "other", issuers: []*x509.Certificate{otherCA}, crlConfigured: true}
	ol, _ := parseCRL(makeCRL(t, otherCA, otherKey, time.Now().Add(time.Hour), 42), other.issuers)
	other.crl.Store(ol)
	anchor.crl.Store(func() *revocationList {
		l, _ := parseCRL(makeCRL(t, caCert, caKey, time.Now().Add(time.Hour)), anchor.issuers)
		return l
	}())
	registerCRLAnchors(map[string]*clientCAAnchor{"corp": anchor, "other": other})
	if revokedNow(chains) {
		t.Fatal("serial 42 on another CA's list must not revoke this CA's certificate")
	}
	// The router's own gate asks the host's anchor directly, chain-wide.
	anchor.crl.Store(func() *revocationList {
		l, _ := parseCRL(makeCRL(t, caCert, caKey, time.Now().Add(time.Hour), 42), anchor.issuers)
		return l
	}())
	if !anchor.chainRevoked(chains) {
		t.Fatal("chainRevoked must see the revoked leaf")
	}
}

// Override headers ride the client-IP trust tier: a peer in the host's
// trustedProxies may carry them, any other peer loses them.
func TestOverrideHeadersKeptFromTrustedProxies(t *testing.T) {
	hh := &hostHandler{trustedProxies: []*net.IPNet{parseNet("10.0.0.0/8")}}
	mk := func(peer string) *http.Request {
		r := httptest.NewRequest("GET", "https://app.example.com/", nil)
		r.RemoteAddr = peer + ":1234"
		r.Header.Set("True-Client-IP", "203.0.113.9")
		r.Header.Set("Cf-Connecting-Ip", "203.0.113.9")
		r.Header.Set("Remote-User", "root")
		return r
	}
	r := mk("10.0.0.7")
	r.Header.Set("X-Original-URL", "/admin")
	r.Header.Set("X-HTTP-Method-Override", "DELETE")
	hh.stripUntrustedIdentity(r)
	if r.Header.Get("True-Client-IP") == "" || r.Header.Get("Cf-Connecting-Ip") == "" {
		t.Fatal("a trusted proxy's client-address headers must reach the backend")
	}
	if r.Header.Get("X-Original-URL") != "" || r.Header.Get("X-HTTP-Method-Override") != "" {
		t.Fatal("URL and method overrides are dropped even from a trusted proxy (a CDN forwards them from the client)")
	}
	if r.Header.Get("Remote-User") != "" {
		t.Fatal("identity headers are a different tier and stay stripped (no identity trust configured)")
	}
	r = mk("198.51.100.4")
	hh.stripUntrustedIdentity(r)
	if r.Header.Get("True-Client-IP") != "" || r.Header.Get("Cf-Connecting-Ip") != "" {
		t.Fatal("an untrusted peer's override headers must be stripped")
	}
}

// A bodied request's read deadline measures stalls between body chunks and
// stops being touched once the body has ended, so a slow RESPONSE after a
// complete upload is not cut off by it. The request goes through the real
// reverse proxy to a real backend: the HTTP/1 transport reads the body once
// more after its Content-Length is exhausted, which is exactly the read that
// re-armed the deadline before the fix.
func TestIdleBodyBoundsStallsNotResponses(t *testing.T) {
	const idle = 150 * time.Millisecond
	prev := bodyIdleTimeout
	bodyIdleTimeout = idle
	t.Cleanup(func() { bodyIdleTimeout = prev }) // runs last: both servers are closed first

	up, closeUp := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "body: "+err.Error(), http.StatusBadRequest)
			return
		}
		time.Sleep(3 * idle) // the slow part is the response, not the upload
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "done")
	}))
	t.Cleanup(closeUp)
	srv := httptest.NewUnstartedServer(newReverseProxy(up, "app", nil, nil, nil, nil))
	srv.Config.ReadTimeout = 10 * idle
	srv.Start()
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL, "text/plain", strings.NewReader(strings.Repeat("x", 64<<10)))
	if err != nil {
		t.Fatalf("complete upload then slow response: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "done" {
		t.Fatalf("slow response after a complete body must succeed: %d %q", resp.StatusCode, body)
	}

	// A body that stalls longer than the idle timeout is cut off.
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("partial"))
		time.Sleep(3 * idle)
		_ = pw.Close()
	}()
	req, _ := http.NewRequest(http.MethodPost, srv.URL, pr)
	req.ContentLength = -1
	req.Header.Set("Content-Type", "text/plain")
	resp2, err := http.DefaultClient.Do(req)
	if err == nil {
		defer resp2.Body.Close()
		if resp2.StatusCode == http.StatusOK {
			t.Fatal("a stalled body must not complete")
		}
	}
}

// The client-cert gate refuses a certificate the handshake verified under
// another host's SNI configuration.
func TestClientCertGateBindsSNIToTheHost(t *testing.T) {
	_, caCert, caKey := crlCA(t, "corp")
	_, leaf := clientCertSerial(t, caCert, caKey, 7, "alice")
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	gate := clientCertGate(model.AuthMiddleware{Mode: model.AuthModeClientCert}, nil, nil, "app", nil, ok, "app.example.com")
	mk := func(sni string) *http.Request {
		r := httptest.NewRequest("GET", "https://app.example.com/", nil)
		r.TLS = &tls.ConnectionState{
			ServerName:       sni,
			PeerCertificates: []*x509.Certificate{leaf},
			VerifiedChains:   [][]*x509.Certificate{{leaf, caCert}},
		}
		return r
	}
	w := httptest.NewRecorder()
	gate.ServeHTTP(w, mk("app.example.com"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("own SNI: %d", w.Code)
	}
	w = httptest.NewRecorder()
	gate.ServeHTTP(w, mk("other.example.com"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("foreign SNI: %d, want 401", w.Code)
	}
}
