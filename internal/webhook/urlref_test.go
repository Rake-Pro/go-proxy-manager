package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// lockedBuf is a log sink safe to read while delivery goroutines write to it.
type lockedBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog points the global logger at a buffer for the test.
func captureLog(t *testing.T) *lockedBuf {
	t.Helper()
	buf := &lockedBuf{}
	prev := log.Logger
	log.Logger = zerolog.New(buf)
	t.Cleanup(func() { log.Logger = prev })
	return buf
}

// closedAddr returns a loopback address nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// A url given as ${ENV:...} is resolved at send time and delivered to; the
// status and test result show the placeholder, never the resolved value.
func TestPlaceholderURLDelivers(t *testing.T) {
	const token = "receiver-path-token"
	var hits atomic.Int32
	var gotPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotPath.Store(r.URL.Path)
	}))
	defer srv.Close()
	t.Setenv("WH_URL", srv.URL+"/hooks/"+token)
	logs := captureLog(t)

	d := New(func() []model.WebhookConfig {
		return []model.WebhookConfig{{Name: "ref", URL: "${ENV:WH_URL}"}}
	})
	d.Dispatch(Event{Action: "save", Kind: "ProxyHost", Name: "app"})
	d.Close() // wg.Wait: the delivery goroutine has exited before the test moves on
	if hits.Load() != 1 || gotPath.Load() != "/hooks/"+token {
		t.Fatalf("hits=%d path=%v, want one delivery to the resolved url", hits.Load(), gotPath.Load())
	}

	res, err := d.Test(context.Background(), "ref")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.URL != "${ENV:WH_URL}" {
		t.Fatalf("test result = %+v, want ok with the placeholder url", res)
	}
	for _, dl := range d.Status() {
		if !dl.OK || dl.URL != "${ENV:WH_URL}" {
			t.Fatalf("status = %+v, want ok with the placeholder url", dl)
		}
	}
	if strings.Contains(logs.String(), token) {
		t.Errorf("log repeats the resolved url: %s", logs.String())
	}
}

// A literal url is sent and reported exactly as before.
func TestLiteralURLUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	d := New(func() []model.WebhookConfig {
		return []model.WebhookConfig{{Name: "lit", URL: srv.URL + "/x"}}
	})
	res, err := d.Test(context.Background(), "lit")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.URL != srv.URL+"/x" {
		t.Fatalf("test result = %+v, want ok with the literal url", res)
	}
}

// secretHost stands for a receiver host the placeholder is meant to hide.
const secretHost = "secret-receiver-host.invalid"

// failNameResolution makes c fail every dial the way net.Dialer reports a
// name that does not resolve (net/dial.go:544: an *OpError wrapping the
// resolver's *DNSError), without a real lookup.
func failNameResolution(c *http.Client) {
	c.Transport.(*http.Transport).DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(addr)
		return nil, &net.OpError{Op: "dial", Net: network, Err: &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}}
	}
}

// failCase is one failing delivery: the url, the expected cause phrase, the
// parts of the url that must not leak for a placeholder, and whether name
// resolution is failed in-process (failNameResolution).
type failCase struct {
	name, url, cause string
	secret           []string
	fakeDNS          bool
}

// failingURLs returns urls whose delivery fails at name resolution, at
// connect, in the TLS handshake and at the outbound policy. Every case stays
// inside the process: loopback listeners, a refused link-local dial, and a
// faked name-resolution failure.
func failingURLs(t *testing.T) []failCase {
	t.Helper()
	const path = "/hooks/receiver-path-token"
	down := closedAddr(t)
	downHost, downPort, _ := net.SplitHostPort(down)
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(tlsSrv.Close)
	tlsAddr := strings.TrimPrefix(tlsSrv.URL, "https://")
	return []failCase{
		{"dns", "https://" + secretHost + path, "name resolution failed", []string{secretHost, path}, true},
		{"refused", "http://" + down + path, "connection failed", []string{downHost, ":" + downPort, path}, false},
		{"tls", tlsSrv.URL + path, "TLS handshake failed", []string{tlsAddr, path}, false},
		{"link-local", "http://[fe80::1]:80" + path, "refused by the outbound policy", []string{"fe80::1", path}, false},
	}
}

// A failing delivery to a resolved url reports a fixed cause, and neither the
// host nor the path of the resolved url appears in the test result, the
// status record or the log.
func TestPlaceholderURLFailureOmitsResolvedURL(t *testing.T) {
	for _, tc := range failingURLs(t) {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WH_DOWN", tc.url)
			logs := captureLog(t)
			d := New(func() []model.WebhookConfig {
				return []model.WebhookConfig{{Name: "down", URL: "${ENV:WH_DOWN}"}}
			})
			if tc.fakeDNS {
				failNameResolution(d.client)
			}
			res, err := d.Test(context.Background(), "down")
			if err != nil {
				t.Fatal(err)
			}
			if res.OK || !strings.HasPrefix(res.Error, tc.cause) {
				t.Fatalf("test result = %+v, want a failure starting %q", res, tc.cause)
			}
			d.Dispatch(Event{Action: "save", Kind: "ProxyHost", Name: "app"})
			d.Close()
			if !strings.Contains(logs.String(), "webhook: delivery failed") {
				t.Fatalf("no delivery-failure log line: %s", logs.String())
			}
			sinks := map[string]string{"test error": res.Error, "test url": res.URL, "log": logs.String()}
			for _, dl := range d.Status() {
				sinks["status error"], sinks["status url"] = dl.Error, dl.URL
			}
			for sink, text := range sinks {
				for _, part := range tc.secret {
					if strings.Contains(text, part) {
						t.Errorf("%s repeats %q of the resolved url: %q", sink, part, text)
					}
				}
			}
		})
	}
}

// A literal url that fails the same ways keeps exactly the error text it had
// before placeholders: TransportCause of the client's own error.
func TestLiteralURLFailureTextUnchanged(t *testing.T) {
	for _, tc := range failingURLs(t) {
		t.Run(tc.name, func(t *testing.T) {
			ref := NewSecureClient(TestTimeout)
			if tc.fakeDNS {
				failNameResolution(ref)
			}
			_, werr := ref.Post(tc.url, "application/json", strings.NewReader("{}"))
			if werr == nil {
				t.Fatal("expected the reference request to fail")
			}
			want := TransportCause(werr).Error()
			d := New(func() []model.WebhookConfig {
				return []model.WebhookConfig{{Name: "lit", URL: tc.url}}
			})
			if tc.fakeDNS {
				failNameResolution(d.client)
			}
			res, err := d.Test(context.Background(), "lit")
			if err != nil {
				t.Fatal(err)
			}
			if res.Error != want {
				t.Fatalf("literal error = %q, want today's %q", res.Error, want)
			}
		})
	}
}

func TestPlaceholderTransportCause(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 443}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"dns", &url.Error{Op: "Post", URL: "https://" + secretHost + "/x", Err: &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Name: secretHost, Err: "no such host", IsNotFound: true}}}, "name resolution failed"},
		{"refused", &net.OpError{Op: "dial", Net: "tcp", Addr: addr, Err: syscall.ECONNREFUSED}, "connection failed"},
		{"reset after connect", &net.OpError{Op: "read", Net: "tcp", Addr: addr, Err: syscall.ECONNRESET}, "connection failed"},
		{"policy", &net.OpError{Op: "dial", Net: "tcp", Addr: addr, Err: linkLocalRefused{"fe80::1", "link-local"}}, "refused by the outbound policy"},
		{"certificate", &tls.CertificateVerificationError{Err: x509.HostnameError{Host: secretHost}}, "TLS handshake failed"},
		{"not tls", tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}, "TLS handshake failed"},
		{"https to a plain-HTTP receiver", &url.Error{Op: "Post", URL: "https://" + secretHost + "/x", Err: http.ErrSchemeMismatch}, "TLS handshake failed"},
		{"tls alert", &net.OpError{Op: "remote error", Err: errors.New("tls: bad certificate")}, "TLS handshake failed"},
		{"deadline", fmt.Errorf("post: %w", context.DeadlineExceeded), "timed out"},
		{"dial timeout", &net.OpError{Op: "dial", Net: "tcp", Addr: addr, Err: os.ErrDeadlineExceeded}, "timed out"},
		{"other", errors.New("unexpected EOF from " + secretHost), "other transport error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PlaceholderTransportCause(tc.err).Error()
			if !strings.HasPrefix(got, tc.want+" (") {
				t.Fatalf("cause = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, secretHost) || strings.Contains(got, "192.0.2.1") || strings.Contains(got, "fe80") {
				t.Fatalf("cause repeats part of the url: %q", got)
			}
		})
	}
}

// An unresolvable or badly shaped placeholder fails only its own delivery,
// with an error naming the target and the placeholder; the other target still
// receives the event.
func TestPlaceholderURLUnresolvable(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	t.Setenv("WH_FTP", "ftp://hooks.example.com/receiver-path-token")
	d := New(func() []model.WebhookConfig {
		return []model.WebhookConfig{
			{Name: "missing", URL: "${ENV:WH_NOT_SET}"},
			{Name: "ftp", URL: "${ENV:WH_FTP}"},
			{Name: "ok", URL: srv.URL},
		}
	})
	d.Dispatch(Event{Action: "save", Kind: "ProxyHost", Name: "app"})
	d.Close() // wg.Wait: the delivery goroutine has exited before the test moves on
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want the working target delivered once", hits.Load())
	}
	want := map[string][]string{
		"missing": {`webhook "missing"`, "${ENV:WH_NOT_SET}", "not set"},
		"ftp":     {`webhook "ftp"`, "${ENV:WH_FTP}", "absolute http(s)"},
	}
	for _, dl := range d.Status() {
		subs, failing := want[dl.Name]
		if !failing {
			if !dl.OK {
				t.Errorf("target %q: %+v, want ok", dl.Name, dl)
			}
			continue
		}
		if dl.OK || dl.LastAttempt == "" {
			t.Errorf("target %q: %+v, want a recorded failure", dl.Name, dl)
		}
		for _, s := range subs {
			if !strings.Contains(dl.Error, s) {
				t.Errorf("target %q error %q does not mention %q", dl.Name, dl.Error, s)
			}
		}
		if strings.Contains(dl.Error, "receiver-path-token") {
			t.Errorf("target %q error repeats the resolved url: %q", dl.Name, dl.Error)
		}
	}
}
