package notify

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/webhook"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const pathToken = "receiver-path-token"

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

func statusOf(n *Notifier, name string) webhook.Delivery {
	for _, dl := range n.Status() {
		if dl.Name == name {
			return dl
		}
	}
	return webhook.Delivery{}
}

// Each target type resolves a whole-value placeholder url at send time and
// reports the placeholder, never the resolved value.
func TestPlaceholderURLDelivers(t *testing.T) {
	var hits atomic.Int32
	var gotPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotPath.Store(r.URL.Path)
	}))
	defer srv.Close()
	tests := []struct {
		typ, path string
	}{
		{model.NotificationTypeNtfy, "/" + pathToken},
		{model.NotificationTypeDiscord, "/api/webhooks/1/" + pathToken},
		{model.NotificationTypeGeneric, "/hooks/" + pathToken},
	}
	for _, tc := range tests {
		t.Run(tc.typ, func(t *testing.T) {
			hits.Store(0)
			t.Setenv("NTF_URL", srv.URL+tc.path)
			logs := captureLog(t)
			n := New(func() []model.NotificationTarget {
				return []model.NotificationTarget{{Name: "ref", Type: tc.typ, URL: "${ENV:NTF_URL}"}}
			})
			t.Cleanup(n.Close)

			n.Emit(context.Background(), Event{Kind: model.EventCertRenewalFailed, Title: "t", Body: "b"})
			waitFor(t, func() bool { return statusOf(n, "ref").LastAttempt != "" }, "emit was never delivered")
			if st := statusOf(n, "ref"); !st.OK || st.URL != "${ENV:NTF_URL}" {
				t.Fatalf("status = %+v, want ok with the placeholder url", st)
			}
			res, err := n.Test(context.Background(), "ref")
			if err != nil {
				t.Fatal(err)
			}
			if !res.OK || res.URL != "${ENV:NTF_URL}" {
				t.Fatalf("test result = %+v, want ok with the placeholder url", res)
			}
			if hits.Load() != 2 || gotPath.Load() != tc.path {
				t.Fatalf("hits=%d path=%v, want two sends to the resolved url", hits.Load(), gotPath.Load())
			}
			if strings.Contains(logs.String(), pathToken) {
				t.Errorf("log repeats the resolved url: %s", logs.String())
			}
		})
	}
}

// failNameResolution makes c fail every dial the way net.Dialer reports a
// name that does not resolve (net/dial.go:544: an *OpError wrapping the
// resolver's *DNSError), without a real lookup.
func failNameResolution(c *http.Client) {
	c.Transport.(*http.Transport).DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(addr)
		return nil, &net.OpError{Op: "dial", Net: network, Err: &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}}
	}
}

// A failing send to a resolved url reports a fixed cause, and neither the host
// nor the path of the resolved url appears in the test result, the status
// record or the log. A literal url failing the same way keeps today's text.
func TestPlaceholderURLFailureOmitsResolvedURL(t *testing.T) {
	const host = "secret-receiver-host.invalid"
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	down := ln.Addr().String()
	ln.Close()
	downHost, downPort, _ := net.SplitHostPort(down)
	path := "/" + pathToken
	tests := []struct {
		name, url, cause string
		secret           []string
		fakeDNS          bool // fail name resolution in-process (failNameResolution)
	}{
		{"dns", "https://" + host + path, "name resolution failed", []string{host, path}, true},
		{"refused", "http://" + down + path, "connection failed", []string{downHost, ":" + downPort, path}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NTF_DOWN", tc.url)
			logs := captureLog(t)
			n := New(func() []model.NotificationTarget {
				return []model.NotificationTarget{
					{Name: "down", Type: model.NotificationTypeNtfy, URL: "${ENV:NTF_DOWN}"},
					{Name: "lit", Type: model.NotificationTypeNtfy, URL: tc.url, Disabled: true},
				}
			})
			t.Cleanup(n.Close)
			if tc.fakeDNS {
				failNameResolution(n.client)
			}

			res, err := n.Test(context.Background(), "down")
			if err != nil {
				t.Fatal(err)
			}
			if res.OK || !strings.HasPrefix(res.Error, tc.cause) {
				t.Fatalf("test result = %+v, want a failure starting %q", res, tc.cause)
			}
			n.Emit(context.Background(), Event{Kind: model.EventCertRenewalFailed, Title: "t", Body: "b"})
			waitFor(t, func() bool { return strings.Contains(logs.String(), "notify: delivery failed") }, "no delivery-failure log line")
			st := statusOf(n, "down")
			sinks := map[string]string{"test error": res.Error, "test url": res.URL, "status error": st.Error, "status url": st.URL, "log": logs.String()}
			for sink, text := range sinks {
				for _, part := range tc.secret {
					if strings.Contains(text, part) {
						t.Errorf("%s repeats %q of the resolved url: %q", sink, part, text)
					}
				}
			}

			ref := webhook.NewSecureClient(TestTimeout)
			if tc.fakeDNS {
				failNameResolution(ref)
			}
			_, werr := ref.Post(tc.url, "text/plain", strings.NewReader("b"))
			if werr == nil {
				t.Fatal("expected the reference request to fail")
			}
			lit, err := n.Test(context.Background(), "lit")
			if err != nil {
				t.Fatal(err)
			}
			if want := webhook.TransportCause(werr).Error(); lit.Error != want {
				t.Fatalf("literal error = %q, want today's %q", lit.Error, want)
			}
		})
	}
}

// An unresolvable placeholder, or one resolving to a url that breaks the shape
// rules (here a discord url without /api/webhooks/), fails only its own send
// with an error naming the target and the placeholder, and nothing is sent
// for it; the other target still receives the event.
func TestPlaceholderURLUnresolvableOrMisshapen(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	t.Setenv("NTF_DISCORD_NOPATH", srv.URL+"/"+pathToken)
	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{
			{Name: "missing", Type: model.NotificationTypeGeneric, URL: "${ENV:NTF_NOT_SET}"},
			{Name: "nopath", Type: model.NotificationTypeDiscord, URL: "${ENV:NTF_DISCORD_NOPATH}"},
			{Name: "ok", Type: model.NotificationTypeGeneric, URL: srv.URL},
		}
	})
	t.Cleanup(n.Close)
	n.Emit(context.Background(), Event{Kind: model.EventCertRenewalFailed, Title: "t", Body: "b"})
	waitFor(t, func() bool {
		return statusOf(n, "missing").LastAttempt != "" && statusOf(n, "nopath").LastAttempt != "" && statusOf(n, "ok").LastAttempt != ""
	}, "not every target was attempted")

	if hits.Load() != 1 || !statusOf(n, "ok").OK {
		t.Fatalf("hits = %d, ok = %+v; want only the working target delivered", hits.Load(), statusOf(n, "ok"))
	}
	want := map[string][]string{
		"missing": {`notification target "missing"`, "${ENV:NTF_NOT_SET}", "not set"},
		"nopath":  {`notification target "nopath"`, "${ENV:NTF_DISCORD_NOPATH}", "/api/webhooks/"},
	}
	for name, subs := range want {
		st := statusOf(n, name)
		if st.OK {
			t.Errorf("target %q: %+v, want a recorded failure", name, st)
		}
		for _, s := range subs {
			if !strings.Contains(st.Error, s) {
				t.Errorf("target %q error %q does not mention %q", name, st.Error, s)
			}
		}
		if strings.Contains(st.Error, pathToken) {
			t.Errorf("target %q error repeats the resolved url: %q", name, st.Error)
		}
	}
}
