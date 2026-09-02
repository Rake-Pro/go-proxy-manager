package docker

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newSocketServer starts an httptest server on a unix socket inside t.TempDir()
// and returns a Client pointed at it. Everything here is hermetic: no real
// Docker daemon, no network listener, nothing outside the test's own directory.
func newSocketServer(t *testing.T, h http.Handler) (*Client, string) {
	t.Helper()
	sock := filepath.Join(socketDir(t), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)

	c, err := NewClient(ClientConfig{Socket: sock})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, sock
}

// socketDir returns a temp directory short enough to hold a unix socket path:
// sockaddr_un.sun_path is ~104 bytes, and t.TempDir() embeds the (long) test
// name, so a table-driven subtest can overflow it. The fallback stays inside
// the test's TMPDIR and is removed with the test.
func socketDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if len(dir) < 70 {
		return dir
	}
	short, err := os.MkdirTemp("", "gpmdk")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	return short
}

// versionMux answers GET /version with the given values and delegates every
// other path to next.
func versionMux(apiVersion, minVersion string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ApiVersion":"` + apiVersion + `","MinAPIVersion":"` + minVersion + `"}`))
			return
		}
		if next != nil {
			next(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestNewClientRejectsBadConfig(t *testing.T) {
	sock := filepath.Join(socketDir(t), "p.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	tests := []struct {
		name string
		cfg  ClientConfig
		want string
	}{
		{"relative socket", ClientConfig{Socket: "docker.sock"}, "absolute path"},
		{"missing socket", ClientConfig{Socket: filepath.Join(t.TempDir(), "nope.sock")}, "not reachable"},
		{"bad host url", ClientConfig{Host: "::not a url"}, "absolute tcp"},
		{"unsupported scheme", ClientConfig{Host: "ftp://docker.example.com:2375"}, "not supported"},
		{"tls without https", ClientConfig{Host: "https://d.example.com:2376", TLSCert: "/tmp/a.pem"}, "must be set together"},
		{"ok socket", ClientConfig{Socket: sock}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewClient(tc.cfg)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestAPIVersionNegotiation(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		minVersion string
		wantPrefix string
	}{
		{"newer daemon uses gpm's preferred version", "1.51", "1.24", "/v1.41"},
		{"older daemon drops to its own version", "1.24", "1.24", "/v1.24"},
		{"daemon minimum above the preference wins", "1.51", "1.44", "/v1.44"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got atomic.Value
			got.Store("")
			c, _ := newSocketServer(t, versionMux(tc.apiVersion, tc.minVersion, func(w http.ResponseWriter, r *http.Request) {
				got.Store(strings.TrimSuffix(r.URL.Path, "/containers/json"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}))
			if _, err := c.ListContainers(context.Background(), "gpm.rake.pro/enabled", false); err != nil {
				t.Fatalf("ListContainers: %v", err)
			}
			if got.Load().(string) != tc.wantPrefix {
				t.Fatalf("path prefix %q, want %q", got.Load(), tc.wantPrefix)
			}
		})
	}
}

func TestListContainersRequestShape(t *testing.T) {
	tests := []struct {
		name           string
		all            bool
		wantAll        string
		wantFilterPart string
	}{
		{"running only", false, "", `"label":["gpm.rake.pro/enabled=true"]`},
		{"include stopped", true, "1", `"label":["gpm.rake.pro/enabled=true"]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var filters, all atomic.Value
			filters.Store("")
			all.Store("")
			c, _ := newSocketServer(t, versionMux("1.51", "1.24", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method %s, want GET (the client must never write)", r.Method)
				}
				filters.Store(r.URL.Query().Get("filters"))
				all.Store(r.URL.Query().Get("all"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"Id":"abc","Names":["/grafana"],"State":"running"}]`))
			}))
			out, err := c.ListContainers(context.Background(), "gpm.rake.pro/enabled", tc.all)
			if err != nil {
				t.Fatalf("ListContainers: %v", err)
			}
			if len(out) != 1 || out[0].Name() != "grafana" {
				t.Fatalf("containers %+v, want one named grafana", out)
			}
			if !strings.Contains(filters.Load().(string), tc.wantFilterPart) {
				t.Fatalf("filters %q, want one containing %q", filters.Load(), tc.wantFilterPart)
			}
			if all.Load().(string) != tc.wantAll {
				t.Fatalf("all=%q, want %q", all.Load(), tc.wantAll)
			}
		})
	}
}

// A 200 whose body is not a JSON array must be an ERROR, never an empty list:
// an empty list is a delete-every-managed-host input, so a misdirected request
// (a socket proxy answering with an error object, an HTML page, a bare null)
// has to land on the freeze path instead.
func TestListContainersRefusesNonArrayBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"null", `null`},
		{"object", `{"message":"unauthorized"}`},
		{"html", `<html>nope</html>`},
		{"empty", ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newSocketServer(t, versionMux("1.51", "1.24", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			out, err := c.ListContainers(context.Background(), "gpm.rake.pro/enabled", false)
			if err == nil {
				t.Fatalf("got %d containers and no error, want an error", len(out))
			}
			if out != nil {
				t.Fatalf("items returned alongside an error: %+v", out)
			}
		})
	}
}

func TestListContainersStatusError(t *testing.T) {
	c, _ := newSocketServer(t, versionMux("1.51", "1.24", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"container list is not allowed"}`))
	}))
	_, err := c.ListContainers(context.Background(), "gpm.rake.pro/enabled", false)
	if err == nil || !strings.Contains(err.Error(), "container list is not allowed") {
		t.Fatalf("error %v, want the endpoint's own message", err)
	}
	var se *statusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Fatalf("error %v, want a statusError carrying 403", err)
	}
}

func TestPingReportsUnreachableEndpoint(t *testing.T) {
	c, _ := newSocketServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("Ping succeeded against an endpoint that is not a Docker Engine")
	}

	c2, _ := newSocketServer(t, versionMux("1.51", "1.24", nil))
	if err := c2.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestWatchEventsCallsBackPerLine(t *testing.T) {
	c, _ := newSocketServer(t, versionMux("1.51", "1.24", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if f := r.URL.Query().Get("filters"); !strings.Contains(f, `"container"`) || !strings.Contains(f, `"start"`) {
			t.Errorf("filters %q, want container/start scoped", f)
		}
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 3; i++ {
			_, _ = w.Write([]byte(`{"Type":"container","Action":"start"}` + "\n"))
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
		}
	}))

	var n atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The stream always ends in an error: a closed stream is a retry condition,
	// never a completion.
	if err := c.WatchEvents(ctx, func() { n.Add(1) }); err == nil {
		t.Fatal("WatchEvents returned nil; a finished stream must be an error")
	}
	if got := n.Load(); got != 3 {
		t.Fatalf("callbacks %d, want 3", got)
	}
}

func TestWatchEventsStatusError(t *testing.T) {
	c, _ := newSocketServer(t, versionMux("1.51", "1.24", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"events are not allowed"}`))
	}))
	err := c.WatchEvents(context.Background(), func() { t.Error("callback fired on an error response") })
	if err == nil || !strings.Contains(err.Error(), "events are not allowed") {
		t.Fatalf("error %v, want the endpoint's own message", err)
	}
}

// TestRefuseLinkLocal is a direct unit test of the SSRF guard installed as
// net.Dialer.Control for a tcp:// / https:// Host: it must refuse link-local
// unicast and multicast destinations (the metadata-service and router
// address ranges) and let everything else, including loopback, through - the
// actual rule in refuseLinkLocal, not the (larger) set a reader might expect.
func TestRefuseLinkLocal(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		wantRefused bool
	}{
		{"IPv4 link-local unicast", "169.254.1.1:2375", true},
		{"IPv4 cloud metadata address", "169.254.169.254:80", true},
		{"IPv4 link-local multicast (mDNS)", "224.0.0.251:5353", true},
		{"IPv6 link-local unicast", "[fe80::1]:2375", true},
		{"IPv6 link-local multicast", "[ff02::1]:2375", true},
		{"IPv4 loopback is not refused by this guard", "127.0.0.1:2375", false},
		{"IPv6 loopback is not refused by this guard", "[::1]:2375", false},
		{"private IPv4 is allowed", "10.0.0.5:2375", false},
		{"public IPv4 is allowed", "203.0.113.5:2375", false},
		{"host with no port falls back to the whole address", "169.254.169.254", true},
		{"unparseable address is not refused (nothing to compare)", "docker.example.com:2375", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := refuseLinkLocal("tcp", tc.address, nil)
			if tc.wantRefused {
				if err == nil {
					t.Fatalf("refuseLinkLocal(%q) = nil, want an error", tc.address)
				}
				if !strings.Contains(err.Error(), "link-local") {
					t.Errorf("refuseLinkLocal(%q) error %v, want it to mention link-local", tc.address, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("refuseLinkLocal(%q) = %v, want nil", tc.address, err)
			}
		})
	}
}

// TestTCPHostRefusesLinkLocalAtConnect is the end-to-end check that a tcp://
// Host actually wires refuseLinkLocal into the dialer: connecting to a
// link-local-addressed listener must fail even though the port itself is
// open, because the guard runs before the handshake, not after.
func TestTCPHostRefusesLinkLocalAtConnect(t *testing.T) {
	c, err := NewClient(ClientConfig{Host: "tcp://169.254.169.254:1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	err = c.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping against a link-local host succeeded, want the dial to be refused")
	}
	if !strings.Contains(err.Error(), "link-local") {
		t.Fatalf("Ping error %v, want it to surface the link-local refusal", err)
	}
}

// genTestCertKeyPEM returns a self-signed leaf certificate and its private
// key, both PEM-encoded, plus the PEM of the CA that signed it. Hermetic: a
// throwaway CA generated for this call, no real PKI.
func genTestCertKeyPEM(t *testing.T) (certPEM, keyPEM, caPEM []byte) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "docker.example.com"},
		DNSNames:     []string{"docker.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return certPEM, keyPEM, caPEM
}

// TestClientTLSConstruction exercises clientTLS's file-loading paths: a CA
// alone, a cert+key pair alone, both together, and every way the inputs can
// be malformed or mismatched. This is the TLS-client construction the docker
// client uses for an https:// Host; it has no skip-verify escape hatch, so
// MinVersion and RootCAs/Certificates are exactly what a caller can rely on.
func TestClientTLSConstruction(t *testing.T) {
	certPEM, keyPEM, caPEM := genTestCertKeyPEM(t)
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	caPath := write("ca.pem", caPEM)
	certPath := write("cert.pem", certPEM)
	keyPath := write("key.pem", keyPEM)
	badPEMPath := write("bad.pem", []byte("not a certificate"))

	t.Run("CA only", func(t *testing.T) {
		cfg, err := clientTLS(ClientConfig{TLSCA: caPath})
		if err != nil {
			t.Fatalf("clientTLS: %v", err)
		}
		if cfg.MinVersion != tls.VersionTLS12 {
			t.Errorf("MinVersion = %x, want TLS 1.2", cfg.MinVersion)
		}
		if cfg.RootCAs == nil {
			t.Error("RootCAs not set from TLSCA")
		}
		if len(cfg.Certificates) != 0 {
			t.Error("Certificates set without TLSCert/TLSKey configured")
		}
	})

	t.Run("cert and key only", func(t *testing.T) {
		cfg, err := clientTLS(ClientConfig{TLSCert: certPath, TLSKey: keyPath})
		if err != nil {
			t.Fatalf("clientTLS: %v", err)
		}
		if len(cfg.Certificates) != 1 {
			t.Fatalf("Certificates = %d, want 1", len(cfg.Certificates))
		}
		if cfg.RootCAs != nil {
			t.Error("RootCAs set without TLSCA configured")
		}
	})

	t.Run("CA, cert and key together", func(t *testing.T) {
		cfg, err := clientTLS(ClientConfig{TLSCA: caPath, TLSCert: certPath, TLSKey: keyPath})
		if err != nil {
			t.Fatalf("clientTLS: %v", err)
		}
		if cfg.RootCAs == nil || len(cfg.Certificates) != 1 {
			t.Fatalf("cfg = %+v, want both RootCAs and one client certificate", cfg)
		}
	})

	t.Run("missing CA file", func(t *testing.T) {
		_, err := clientTLS(ClientConfig{TLSCA: filepath.Join(dir, "nope.pem")})
		if err == nil || !strings.Contains(err.Error(), "read tlsCA") {
			t.Fatalf("error %v, want a read tlsCA error", err)
		}
	})

	t.Run("CA file has no usable PEM", func(t *testing.T) {
		_, err := clientTLS(ClientConfig{TLSCA: badPEMPath})
		if err == nil || !strings.Contains(err.Error(), "no usable PEM") {
			t.Fatalf("error %v, want a no-usable-PEM error", err)
		}
	})

	t.Run("cert without key is rejected", func(t *testing.T) {
		_, err := clientTLS(ClientConfig{TLSCert: certPath})
		if err == nil || !strings.Contains(err.Error(), "must be set together") {
			t.Fatalf("error %v, want a must-be-set-together error", err)
		}
	})

	t.Run("key without cert is rejected", func(t *testing.T) {
		_, err := clientTLS(ClientConfig{TLSKey: keyPath})
		if err == nil || !strings.Contains(err.Error(), "must be set together") {
			t.Fatalf("error %v, want a must-be-set-together error", err)
		}
	})

	t.Run("unloadable cert/key pair", func(t *testing.T) {
		_, err := clientTLS(ClientConfig{TLSCert: badPEMPath, TLSKey: keyPath})
		if err == nil || !strings.Contains(err.Error(), "load tlsCert/tlsKey") {
			t.Fatalf("error %v, want a load tlsCert/tlsKey error", err)
		}
	})

	t.Run("no TLS options at all is a bare config", func(t *testing.T) {
		cfg, err := clientTLS(ClientConfig{})
		if err != nil {
			t.Fatalf("clientTLS: %v", err)
		}
		if cfg.RootCAs != nil || len(cfg.Certificates) != 0 {
			t.Fatalf("cfg = %+v, want no RootCAs and no Certificates", cfg)
		}
	})
}

// TestNewClientHTTPSWiresClientTLS is the integration path: an https:// Host
// with TLS material actually reaches clientTLS via NewClient, not just via a
// direct call.
func TestNewClientHTTPSWiresClientTLS(t *testing.T) {
	certPEM, keyPEM, _ := genTestCertKeyPEM(t)
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewClient(ClientConfig{Host: "https://docker.example.com:2376", TLSCert: certPath, TLSKey: keyPath}); err != nil {
		t.Fatalf("NewClient with https host and TLS material: %v", err)
	}

	if _, err := NewClient(ClientConfig{Host: "https://docker.example.com:2376", TLSCert: filepath.Join(dir, "missing.pem"), TLSKey: keyPath}); err == nil {
		t.Fatal("NewClient with a missing cert file succeeded, want an error")
	}
}

func TestCompareAPIVersion(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.41", "1.41", 0},
		{"1.40", "1.41", -1},
		{"1.42", "1.41", 1},
		{"2.0", "1.99", 1},
		{"v1.41", "1.41", 0},
	}
	for _, tc := range tests {
		if got := compareAPIVersion(tc.a, tc.b); got != tc.want {
			t.Errorf("compareAPIVersion(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
