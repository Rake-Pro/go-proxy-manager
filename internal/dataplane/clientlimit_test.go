package dataplane

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func withClientLimits(t *testing.T, c *model.ClientLimitsSettings) {
	t.Helper()
	SetClientLimits(c)
	t.Cleanup(func() { SetClientLimits(nil) })
}

func inFlightKeys() int {
	inFlight.mu.Lock()
	defer inFlight.mu.Unlock()
	return len(inFlight.n)
}

// heldDispatch is a listener dispatch that holds every request until release
// is closed, reporting each arrival on entered.
func heldDispatch() (h http.HandlerFunc, entered chan struct{}, release chan struct{}) {
	entered, release = make(chan struct{}, 64), make(chan struct{})
	return func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}, entered, release
}

func requestFrom(remote, host string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	r.RemoteAddr = remote
	return r
}

// With settings.clientLimits absent nothing is counted and nothing is refused.
func TestClientLimitOffCountsNothing(t *testing.T) {
	SetClientLimits(nil)
	s := New(Config{})
	dispatch, entered, release := heldDispatch()
	hs := s.dataHandler(dispatch)
	var wg sync.WaitGroup
	codes := make(chan int, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			hs.ServeHTTP(rec, requestFrom("192.0.2.1:1000", "app.example.com"))
			codes <- rec.Code
		}()
	}
	for i := 0; i < 20; i++ {
		<-entered
	}
	if n := inFlightKeys(); n != 0 {
		t.Fatalf("in-flight table has %d entries with the limit off", n)
	}
	close(release)
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != http.StatusNoContent {
			t.Fatalf("a request got %d with the limit off", c)
		}
	}
}

// A client address may have at most maxInFlight requests in progress; one more
// is refused with 429 and counted as an in-flight-limit denial, other
// addresses and exempt networks are unaffected, and the count is returned as
// requests finish.
func TestClientLimitCapsRequestsInProgress(t *testing.T) {
	hook := installHook(t)
	withClientLimits(t, &model.ClientLimitsSettings{MaxInFlight: 2, AllowFrom: []string{"198.51.100.0/24"}})
	s := New(Config{})
	if err := s.Reload(model.Config{}); err != nil {
		t.Fatal(err)
	}
	dispatch, entered, release := heldDispatch()
	hs := s.dataHandler(dispatch)

	var wg sync.WaitGroup
	hold := func(remote string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hs.ServeHTTP(httptest.NewRecorder(), requestFrom(remote, "app.example.com"))
		}()
		<-entered
	}
	hold("192.0.2.1:1000")
	hold("192.0.2.1:1001")

	rec := httptest.NewRecorder()
	hs.ServeHTTP(rec, requestFrom("192.0.2.1:1002", "nohost.example.com"))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("third request from one address: %d Retry-After=%q, want 429 with Retry-After", rec.Code, rec.Header().Get("Retry-After"))
	}
	hook.mu.Lock()
	denied := hook.denials[unknownHostLabel+"|in-flight-limit"]
	hook.mu.Unlock()
	if denied != 1 {
		t.Fatalf("in-flight-limit denials = %d, want 1 (%v)", denied, hook.denials)
	}

	// Another address, and an exempt one, still get through.
	hold("192.0.2.2:1000")
	for i := 0; i < 3; i++ {
		hold("198.51.100.7:1000")
	}

	close(release)
	wg.Wait()
	if n := inFlightKeys(); n != 0 {
		t.Fatalf("in-flight table has %d entries after every request finished", n)
	}
	rec = httptest.NewRecorder()
	ok := make(chan struct{})
	go func() {
		hs.ServeHTTP(rec, requestFrom("192.0.2.1:1003", "app.example.com"))
		close(ok)
	}()
	<-entered
	<-ok
	if rec.Code != http.StatusNoContent {
		t.Fatalf("request after the others finished: %d, want it served", rec.Code)
	}
}

// IPv6 clients are counted per /64, like the rate limiter's buckets.
func TestClientLimitGroupsIPv6By64(t *testing.T) {
	withClientLimits(t, &model.ClientLimitsSettings{MaxInFlight: 1})
	s := New(Config{})
	dispatch, entered, release := heldDispatch()
	hs := s.dataHandler(dispatch)
	done := make(chan struct{})
	go func() {
		hs.ServeHTTP(httptest.NewRecorder(), requestFrom("[2001:db8:0:1::1]:1000", "app.example.com"))
		close(done)
	}()
	<-entered
	rec := httptest.NewRecorder()
	hs.ServeHTTP(rec, requestFrom("[2001:db8:0:1:ffff::2]:1000", "app.example.com"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second address in the same /64: %d, want 429", rec.Code)
	}
	close(release)
	<-done
}

// The count is returned when dispatch panics, as net/http does for an aborted
// handler.
func TestClientLimitReleasedOnPanic(t *testing.T) {
	withClientLimits(t, &model.ClientLimitsSettings{MaxInFlight: 1})
	s := New(Config{})
	hs := s.dataHandler(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	func() {
		defer func() { _ = recover() }()
		hs.ServeHTTP(httptest.NewRecorder(), requestFrom("192.0.2.9:1000", "app.example.com"))
	}()
	if n := inFlightKeys(); n != 0 {
		t.Fatalf("in-flight table has %d entries after a panicking request", n)
	}
}

// An upgraded connection counts against the limit for as long as it is open,
// and its count is returned when it closes.
func TestClientLimitCountsOpenTunnel(t *testing.T) {
	withClientLimits(t, &model.ClientLimitsSettings{MaxInFlight: 1})
	up, closeFn := hijackEcho(t)
	defer closeFn()
	s := New(Config{})
	if err := s.Reload(model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "ws"}, Domains: []string{"ws.example.com"}, Upstream: up,
	}}}); err != nil {
		t.Fatal(err)
	}
	addr := listenWithPolicy(t, s.dataHandler(s.dispatchHTTP))
	c, br := openTunnel(t, addr, "ws.example.com")

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("request while the client holds an open tunnel: %d, want 429", resp.StatusCode)
	}
	tunnelEcho(t, c, br)
	_ = c.Close()
	if !waitFor(t, 2*time.Second, func() bool { return inFlightKeys() == 0 }) {
		t.Fatal("the closed tunnel is still counted")
	}
}

// A full in-flight table serves a new client address uncounted instead of
// refusing it; an address already tracked keeps its own limit.
func TestClientLimitFullTableFailsOpen(t *testing.T) {
	withClientLimits(t, &model.ClientLimitsSettings{MaxInFlight: 1})
	inFlight.mu.Lock()
	inFlight.n = make(map[string]int, maxInFlightClients)
	for i := 0; i < maxInFlightClients-1; i++ {
		inFlight.n["filler-"+strconv.Itoa(i)] = 1
	}
	inFlight.mu.Unlock()
	t.Cleanup(func() {
		inFlight.mu.Lock()
		inFlight.n = nil
		inFlight.mu.Unlock()
	})

	s := New(Config{})
	dispatch, entered, release := heldDispatch()
	hs := s.dataHandler(dispatch)
	done := make(chan struct{})
	go func() {
		hs.ServeHTTP(httptest.NewRecorder(), requestFrom("192.0.2.1:1000", "app.example.com"))
		close(done)
	}()
	<-entered
	if n := inFlightKeys(); n != maxInFlightClients {
		t.Fatalf("table has %d entries, want it full (%d)", n, maxInFlightClients)
	}

	// The tracked address is still held to its limit.
	rec := httptest.NewRecorder()
	hs.ServeHTTP(rec, requestFrom("192.0.2.1:1001", "app.example.com"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("tracked address over its limit with a full table: %d, want 429", rec.Code)
	}

	// A new address is served, and leaves no entry behind.
	served := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		hs.ServeHTTP(rec, requestFrom("203.0.113.5:1000", "app.example.com"))
		served <- rec.Code
	}()
	<-entered
	if n := inFlightKeys(); n != maxInFlightClients {
		t.Fatalf("table has %d entries while the uncounted request runs, want %d", n, maxInFlightClients)
	}
	close(release)
	if code := <-served; code != http.StatusNoContent {
		t.Fatalf("new address with a full table: %d, want it served", code)
	}
	<-done
	if n := inFlightKeys(); n != maxInFlightClients-1 {
		t.Fatalf("table has %d entries after both finished, want %d", n, maxInFlightClients-1)
	}
}
