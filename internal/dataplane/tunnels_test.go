package dataplane

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// openTunnelOn sends an upgrade request for host on c (with any extra header
// lines) and reads the 101 response.
func openTunnelOn(t *testing.T, c net.Conn, host, extra string) *bufio.Reader {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, "GET /ws HTTP/1.1\r\nHost: "+host+"\r\n"+extra+
		"Connection: Upgrade\r\nUpgrade: websocket\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("reading the upgrade response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade answered %d, want 101", resp.StatusCode)
	}
	return br
}

func openTunnel(t *testing.T, addr, host string) (net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, openTunnelOn(t, c, host, "")
}

// tunnelEcho proves the tunnel still carries data both ways.
func tunnelEcho(t *testing.T, c net.Conn, br *bufio.Reader) {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, "ping\n"); err != nil {
		t.Fatalf("tunnel write: %v", err)
	}
	line, err := br.ReadString('\n')
	if err != nil || line != "echo:ping\n" {
		t.Fatalf("tunnel read = %q, %v; want the echo", line, err)
	}
}

// tunnelClosed proves gpm closed the tunnel: a read ends with an error that
// is not the test's own deadline.
func tunnelClosed(t *testing.T, c net.Conn, br *bufio.Reader) {
	t.Helper()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(c, "ping\n")
	for {
		line, err := br.ReadString('\n')
		if err == nil {
			if line == "echo:ping\n" {
				continue // written before the close landed
			}
			t.Fatalf("tunnel read %q, want it closed", line)
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatal("the tunnel is still open")
		}
		return
	}
}

func tunnelCount() int {
	tunnels.mu.Lock()
	defer tunnels.mu.Unlock()
	return len(tunnels.m)
}

// tunnelConfig is one plain-HTTP proxy host behind an allow-all access list,
// plus an unrelated host and access list a reload can change.
func tunnelConfig(up model.Upstream) model.Config {
	return model.Config{
		AccessLists: []model.AccessList{
			{ObjectMeta: model.ObjectMeta{Name: "lan"}, DefaultAction: model.ActionDeny,
				Rules: []model.IPRule{{Action: model.ActionAllow, CIDR: "127.0.0.0/8"}}},
			{ObjectMeta: model.ObjectMeta{Name: "other"}, DefaultAction: model.ActionDeny,
				Rules: []model.IPRule{{Action: model.ActionAllow, CIDR: "192.0.2.0/24"}}},
		},
		ProxyHosts: []model.ProxyHost{
			{ObjectMeta: model.ObjectMeta{Name: "ws"}, Domains: []string{"ws.example.com"}, Upstream: up,
				WebsocketsUpgrade: true, AccessLists: []string{"lan"}},
			{ObjectMeta: model.ObjectMeta{Name: "side"}, Domains: []string{"side.example.com"}, Upstream: up,
				AccessLists: []string{"other"}},
		},
	}
}

func cloneConfig(t *testing.T, c model.Config) model.Config {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var out model.Config
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func tunnelServer(t *testing.T, cfg model.Config) (*Server, string) {
	t.Helper()
	s := New(Config{})
	if err := s.Reload(cfg); err != nil {
		t.Fatal(err)
	}
	return s, listenWithPolicy(t, s.dataHandler(s.dispatchHTTP))
}

// A reload that leaves the host's chain unchanged leaves its open tunnels
// open, whatever else it changes.
func TestTunnelSurvivesReloadWithUnchangedChain(t *testing.T) {
	up, closeFn := hijackEcho(t)
	defer closeFn()
	cfg := tunnelConfig(up)
	s, addr := tunnelServer(t, cfg)
	c, br := openTunnel(t, addr, "ws.example.com")
	tunnelEcho(t, c, br)

	for name, mutate := range map[string]func(*model.Config){
		"same config":           func(*model.Config) {},
		"unrelated access list": func(c *model.Config) { c.AccessLists[1].Rules[0].CIDR = "198.51.100.0/24" },
		"unrelated host":        func(c *model.Config) { c.ProxyHosts[1].Disabled = true },
		"new host": func(c *model.Config) {
			c.ProxyHosts = append(c.ProxyHosts, model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "new"}, Domains: []string{"new.example.com"}, Upstream: up})
		},
		"metadata only": func(c *model.Config) {
			c.AccessLists[0].Tags = []string{"edited"}
			c.AccessLists[0].UpdatedAt = time.Now()
		},
		"host cosmetic field": func(c *model.Config) { c.ProxyHosts[0].RobotsNoIndex = true; c.ProxyHosts[0].TLS.HSTS.Enabled = true },
	} {
		next := cloneConfig(t, cfg)
		mutate(&next)
		if err := s.Reload(next); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		tunnelEcho(t, c, br)
	}
}

// A reload that removes or disables the host, or changes who may use it,
// closes the tunnels opened through it.
func TestTunnelClosedWhenReloadChangesHostChain(t *testing.T) {
	up, closeFn := hijackEcho(t)
	defer closeFn()
	for name, mutate := range map[string]func(*model.Config){
		"host removed":        func(c *model.Config) { c.ProxyHosts = c.ProxyHosts[1:] },
		"host disabled":       func(c *model.Config) { c.ProxyHosts[0].Disabled = true },
		"access list added":   func(c *model.Config) { c.ProxyHosts[0].AccessLists = []string{"lan", "other"} },
		"access list content": func(c *model.Config) { c.AccessLists[0].Rules[0].CIDR = "192.0.2.0/24" },
		"domain dropped":      func(c *model.Config) { c.ProxyHosts[0].Domains = []string{"ws2.example.com"} },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := tunnelConfig(up)
			s, addr := tunnelServer(t, cfg)
			c, br := openTunnel(t, addr, "ws.example.com")
			tunnelEcho(t, c, br)
			next := cloneConfig(t, cfg)
			mutate(&next)
			if err := s.Reload(next); err != nil {
				t.Fatal(err)
			}
			tunnelClosed(t, c, br)
		})
	}
	if !waitFor(t, 2*time.Second, func() bool { return tunnelCount() == 0 }) {
		t.Fatalf("%d tunnels still registered after every tunnel closed", tunnelCount())
	}
}

// ssoTunnelConfig is a plain-HTTP host gated by an OIDC identity provider. The
// gate admits a valid session cookie without contacting the provider.
func ssoTunnelConfig(up model.Upstream) model.Config {
	return model.Config{
		IdentityProviders: []model.IdentityProvider{{
			ObjectMeta: model.ObjectMeta{Name: "idp"}, Type: model.IdPTypeOIDC,
			OIDC: &model.OIDCSpec{IssuerURL: "http://127.0.0.1:9", ClientID: "gpm-client"},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"}, Upstream: up,
			WebsocketsUpgrade: true, Auth: &model.AuthMiddleware{IdentityProvider: "idp", Mode: "oidc"},
		}},
	}
}

func ssoCookieHeader(exp, iat int64) string {
	payload, _ := json.Marshal(oidcSession{Sub: "u", Host: "app", IdP: "idp", Exp: exp, Iat: iat})
	return "Cookie: " + oidcSessionCookie + "=" + signToken(macLabelSSOSession, payload) + "\r\n"
}

func openSSOTunnel(t *testing.T, addr string, exp, iat int64) (net.Conn, *bufio.Reader) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, openTunnelOn(t, c, "app.example.com", ssoCookieHeader(exp, iat))
}

// A middleware shared by several hosts: editing a headers middleware keeps the
// tunnels of the hosts that use it open, editing an auth middleware closes them.
func TestTunnelSharedMiddlewareEdit(t *testing.T) {
	up, closeFn := hijackEcho(t)
	defer closeFn()
	shared := func() model.Config {
		cfg := ssoTunnelConfig(up)
		cfg.Middlewares = []model.Middleware{
			{ObjectMeta: model.ObjectMeta{Name: "hdr"}, Type: model.MWTypeHeaders,
				Headers: &model.HeadersMiddleware{SetResponse: map[string]string{"X-A": "1"}}},
			{ObjectMeta: model.ObjectMeta{Name: "sso"}, Type: model.MWTypeAuth,
				Auth: &model.AuthMiddleware{IdentityProvider: "idp", Mode: "oidc"}},
		}
		cfg.ProxyHosts[0].Auth = nil
		cfg.ProxyHosts[0].Middlewares = []string{"hdr", "sso"}
		cfg.ProxyHosts = append(cfg.ProxyHosts, model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "other"},
			Domains: []string{"other.example.com"}, Upstream: up, Middlewares: []string{"hdr", "sso"}})
		return cfg
	}
	cfg := shared()
	s, addr := tunnelServer(t, cfg)
	now := time.Now().Unix()
	c, br := openSSOTunnel(t, addr, now+3600, now)
	tunnelEcho(t, c, br)

	next := cloneConfig(t, cfg)
	next.Middlewares[0].Headers.SetResponse["X-A"] = "2"
	next.Middlewares[0].Headers.SetRequest = map[string]string{"X-B": "1"}
	if err := s.Reload(next); err != nil {
		t.Fatal(err)
	}
	tunnelEcho(t, c, br)

	next = cloneConfig(t, next)
	next.Middlewares[1].Auth.RequiredRoles = []string{"admin"}
	if err := s.Reload(next); err != nil {
		t.Fatal(err)
	}
	tunnelClosed(t, c, br)
}

// A tunnel admitted by an SSO session is closed when that session expires.
func TestTunnelClosedWhenSSOSessionExpires(t *testing.T) {
	up, closeFn := hijackEcho(t)
	defer closeFn()
	_, addr := tunnelServer(t, ssoTunnelConfig(up))
	now := time.Now().Unix()
	short, sb := openSSOTunnel(t, addr, now+2, now)
	long, lb := openSSOTunnel(t, addr, now+3600, now)
	tunnelEcho(t, short, sb)
	time.Sleep(time.Until(time.Unix(now+2, 0)) + 100*time.Millisecond)
	tunnelClosed(t, short, sb)
	tunnelEcho(t, long, lb)
}

// A tunnel admitted by an SSO session is closed when the revocation watermark
// moves past the session's issue time; a session issued after it is kept.
func TestTunnelClosedWhenSSOSessionsRevoked(t *testing.T) {
	t.Cleanup(func() { ssoNotBefore.Store(0) })
	up, closeFn := hijackEcho(t)
	defer closeFn()
	_, addr := tunnelServer(t, ssoTunnelConfig(up))
	now := time.Now().Unix()
	old, ob := openSSOTunnel(t, addr, now+3600, now-10)
	fresh, fb := openSSOTunnel(t, addr, now+3600, now+5)
	tunnelEcho(t, old, ob)
	if err := RevokeAllSSOSessions(); err != nil {
		t.Fatal(err)
	}
	tunnelClosed(t, old, ob)
	tunnelEcho(t, fresh, fb)
}

// A tunnel opened with a client certificate is closed when a CRL update lists
// that certificate; a tunnel opened with another certificate stays open.
func TestTunnelClosedWhenClientCertRevoked(t *testing.T) {
	up, closeFn := hijackEcho(t)
	defer closeFn()
	caPEM, caCert, caKey := crlCA(t, "corp-ca")
	dir := t.TempDir()
	crlName := writeCRL(t, dir, makeCRL(t, caCert, caKey, time.Now().Add(time.Hour)))
	cfg := crlRouterConfig(caPEM, model.ClientCA{CRLFile: crlName})
	cfg.ProxyHosts[0].Upstream = up

	s := New(Config{CertDir: dir})
	if err := s.Reload(cfg); err != nil {
		t.Fatal(err)
	}
	srvCert := testTLSCert(t)
	s.current().certs.exact["m.example"] = &srvCert
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := newListenerServer(ln.Addr().String(), s.dataHandler(s.dispatchHTTPS))
	srv.TLSConfig = &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{srvCert},
		GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			return s.current().tlsConfigForSNI(h.ServerName), nil
		},
	}
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })

	open := func(serial int64) (net.Conn, *bufio.Reader) {
		cert, _ := clientCertSerial(t, caCert, caKey, serial, "client")
		c, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
			InsecureSkipVerify: true, ServerName: "m.example", NextProtos: []string{"http/1.1"},
			GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &cert, nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c, openTunnelOn(t, c, "m.example", "")
	}
	revoked, rb := open(70)
	kept, kb := open(71)
	tunnelEcho(t, revoked, rb)

	path := filepath.Join(dir, crlName)
	if err := os.WriteFile(path, makeCRL(t, caCert, caKey, time.Now().Add(time.Hour), 70), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	for _, a := range *crlAnchors.Load() {
		a.reloadIfChanged()
	}
	tunnelClosed(t, revoked, rb)
	tunnelEcho(t, kept, kb)
}

// The registry is bounded: an upgrade that finds it full is refused and no
// connection is taken over.
func TestTunnelRegistryBounded(t *testing.T) {
	up, closeFn := hijackEcho(t)
	defer closeFn()
	_, addr := tunnelServer(t, tunnelConfig(up))
	prev := maxTunnels
	maxTunnels = tunnelCount() + 1
	t.Cleanup(func() { maxTunnels = prev })
	c, br := openTunnel(t, addr, "ws.example.com")
	tunnelEcho(t, c, br)

	c2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	_ = c2.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(c2, "GET /ws HTTP/1.1\r\nHost: ws.example.com\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(c2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("an upgrade past the registry bound was switched")
	}
	if !strings.HasPrefix(resp.Status, "502") {
		t.Fatalf("upgrade past the bound answered %q, want 502", resp.Status)
	}
}
