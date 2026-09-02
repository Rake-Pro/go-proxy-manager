package dataplane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// fakeHook records everything the data plane reports, so a test can assert on
// the observations rather than on an exposition string (that contract is pinned
// in internal/metrics).
type fakeHook struct {
	mu        sync.Mutex
	started   int
	finished  int
	requests  []fakeRequest
	upstream  map[string]int
	websocket map[string]int
	denials   map[string]int // "host|reason"
	streamsUp map[string]int
	streamsDn map[string]int
}

type fakeRequest struct {
	host, method string
	status       int
	dur          time.Duration
	in, out      int64
}

func newFakeHook() *fakeHook {
	return &fakeHook{
		upstream:  map[string]int{},
		websocket: map[string]int{},
		denials:   map[string]int{},
		streamsUp: map[string]int{},
		streamsDn: map[string]int{},
	}
}

func (f *fakeHook) RequestStarted()  { f.mu.Lock(); f.started++; f.mu.Unlock() }
func (f *fakeHook) RequestFinished() { f.mu.Lock(); f.finished++; f.mu.Unlock() }

func (f *fakeHook) HTTPRequest(host, method string, status int, dur time.Duration, in, out int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, fakeRequest{host, method, status, dur, in, out})
}

func (f *fakeHook) UpstreamError(host string)    { f.mu.Lock(); f.upstream[host]++; f.mu.Unlock() }
func (f *fakeHook) WebsocketUpgrade(host string) { f.mu.Lock(); f.websocket[host]++; f.mu.Unlock() }

func (f *fakeHook) Denial(host, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.denials[host+"|"+reason]++
}

func (f *fakeHook) StreamOpened(host string) { f.mu.Lock(); f.streamsUp[host]++; f.mu.Unlock() }
func (f *fakeHook) StreamClosed(host string) { f.mu.Lock(); f.streamsDn[host]++; f.mu.Unlock() }

// installHook wires a hook for the duration of one test and detaches it after,
// so the package-global never leaks between tests.
func installHook(t *testing.T) *fakeHook {
	t.Helper()
	h := newFakeHook()
	SetMetricsHook(h)
	t.Cleanup(func() { SetMetricsHook(nil) })
	return h
}

// TestMetricsHookCountsProxiedRequest drives a real request through the whole
// data-plane stack - observe wrapper, router, middleware chain, reverse proxy,
// upstream - and checks the observation carries the ProxyHost NAME, the method,
// the status and both byte directions.
func TestMetricsHookCountsProxiedRequest(t *testing.T) {
	hook := installHook(t)

	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer closeFn()

	s := New(Config{})
	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   up,
	}}}
	if err := s.Reload(cfg); err != nil {
		t.Fatal(err)
	}

	h := s.observe(http.HandlerFunc(s.dispatchHTTP))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://app.example.com/thing", strings.NewReader("12345"))
	req.Host = "app.example.com"
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("proxied request status = %d, body %q", rec.Code, rec.Body.String())
	}
	if len(hook.requests) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(hook.requests))
	}
	got := hook.requests[0]
	// The label is the ProxyHost name from config, NOT the Host header - that is
	// the whole cardinality defence, so it is asserted explicitly.
	if got.host != "app" {
		t.Errorf("host label = %q, want the ProxyHost name %q", got.host, "app")
	}
	if got.method != "POST" || got.status != http.StatusOK {
		t.Errorf("method/status = %s/%d, want POST/200", got.method, got.status)
	}
	if got.in != 5 {
		t.Errorf("request bytes = %d, want 5", got.in)
	}
	if got.out != int64(len("hello world")) {
		t.Errorf("response bytes = %d, want %d", got.out, len("hello world"))
	}
	if hook.started != 1 || hook.finished != 1 {
		t.Errorf("in-flight bracket = %d/%d, want 1/1", hook.started, hook.finished)
	}
}

// A request that matches no host must not mint a series per Host header: the
// label collapses to the single unknown bucket.
func TestMetricsHookFoldsUnknownHosts(t *testing.T) {
	hook := installHook(t)

	s := New(Config{})
	if err := s.Reload(model.Config{}); err != nil {
		t.Fatal(err)
	}
	h := s.observe(http.HandlerFunc(s.dispatchHTTP))

	for _, host := range []string{"a.evil.test", "b.evil.test", "c.evil.test"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "http://"+host+"/", nil)
		req.Host = host
		h.ServeHTTP(rec, req)
	}
	if len(hook.requests) != 3 {
		t.Fatalf("recorded %d requests, want 3", len(hook.requests))
	}
	for _, r := range hook.requests {
		if r.host != unknownHostLabel {
			t.Fatalf("unmatched host labelled %q, want %q - a client-chosen Host must never become a label value", r.host, unknownHostLabel)
		}
		if r.status != http.StatusNotFound {
			t.Fatalf("unmatched host status = %d, want 404", r.status)
		}
	}
}

// An upstream that cannot be reached is a 502 through the proxy's ErrorHandler,
// and must be counted against the host that owns it.
func TestMetricsHookCountsUpstreamErrors(t *testing.T) {
	hook := installHook(t)

	// Port 1 on loopback: reserved, nothing listens, connect fails fast.
	s := New(Config{})
	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "broken"},
		Domains:    []string{"broken.example.com"},
		Upstream:   model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 1},
	}}}
	if err := s.Reload(cfg); err != nil {
		t.Fatal(err)
	}
	h := s.observe(http.HandlerFunc(s.dispatchHTTP))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://broken.example.com/", nil)
	req.Host = "broken.example.com"
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if hook.upstream["broken"] != 1 {
		t.Fatalf("upstream errors for %q = %d, want 1", "broken", hook.upstream["broken"])
	}
	if len(hook.requests) != 1 || hook.requests[0].status != http.StatusBadGateway {
		t.Fatalf("request observation = %+v, want one 502", hook.requests)
	}
}

// Denials are attributed to the host and to the tier that refused, which is what
// makes "everything is 403" diagnosable from a dashboard.
func TestMetricsHookCountsDenials(t *testing.T) {
	hook := installHook(t)

	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()

	cfg := model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "deny-all"},
			Rules:         []model.IPRule{{Action: model.ActionDeny, CIDR: "0.0.0.0/0"}},
			DefaultAction: model.ActionDeny,
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "gated"},
			Domains:     []string{"gated.example.com"},
			Upstream:    up,
			AccessLists: []string{"deny-all"},
		}},
	}
	s := New(Config{})
	if err := s.Reload(cfg); err != nil {
		t.Fatal(err)
	}
	h := s.observe(http.HandlerFunc(s.dispatchHTTP))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://gated.example.com/", nil)
	req.Host = "gated.example.com"
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if hook.denials["gated|access-list"] != 1 {
		t.Fatalf("denials = %v, want one gated|access-list", hook.denials)
	}
}

// Auth gates count their refusals too: without this a host behind SSO or mTLS
// shows no denials in /metrics no matter how many 401/403s it serves, so the one
// tier most likely to be misconfigured is the one tier invisible on a dashboard.
func TestMetricsHookCountsAuthDenials(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := func() *http.Request {
		r := httptest.NewRequest("GET", "https://gated.example.com/", nil)
		r.RemoteAddr = "203.0.113.5:5000"
		return withHostName(r, "gated")
	}

	t.Run("client-cert with no certificate", func(t *testing.T) {
		hook := installHook(t)
		w := httptest.NewRecorder()
		spec := model.AuthMiddleware{Mode: model.AuthModeClientCert}
		clientCertGate(spec, peerIP, nil, "gated", nil, ok).ServeHTTP(w, req())
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", w.Code)
		}
		if hook.denials["gated|auth-client-cert"] != 1 {
			t.Fatalf("denials = %v, want one gated|auth-client-cert", hook.denials)
		}
	})

	t.Run("forward-auth with no asserted identity", func(t *testing.T) {
		hook := installHook(t)
		w := httptest.NewRecorder()
		fa := auth.CompileForwardAuth(model.ForwardAuthSpec{
			TrustedProxies: []string{"192.0.2.10/32"}, UserHeader: "Remote-User",
		}, "idp")
		forwardAuthGate(fa, nil, nil, "gated", nil, ok).ServeHTTP(w, req())
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", w.Code)
		}
		if hook.denials["gated|auth-forward"] != 1 {
			t.Fatalf("denials = %v, want one gated|auth-forward", hook.denials)
		}
	})
}

// With no hook installed (and every other toggle off) the handler switch must
// serve the PLAIN chain, so the default configuration pays one atomic pointer
// load and nothing else for metrics - the same zero-overhead promise the
// access-log toggle makes. The observe wrapper itself always wraps now; the
// switch is what keeps it off the hot path.
func TestNoHookMeansNoWrapper(t *testing.T) {
	SetMetricsHook(nil)
	s := New(Config{})
	inner := func(http.ResponseWriter, *http.Request) {}
	if hs := s.dataHandler(inner); hs.active.Load() != &hs.plain {
		t.Fatal("with metrics (and every other toggle) off, the switch must serve the plain chain")
	}

	SetMetricsHook(newFakeHook())
	t.Cleanup(func() { SetMetricsHook(nil) })
	if hs := s.dataHandler(inner); hs.active.Load() != &hs.observed {
		t.Fatal("with a metrics hook installed, the switch must serve the observed chain")
	}
}
