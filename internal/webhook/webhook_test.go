package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// waitDrained blocks until every currently in-flight delivery goroutine has
// released its Dispatcher.sem slot, which happens only after post() has fully
// returned and the outcome has been recorded. It polls state Dispatch already
// maintains instead of sleeping a fixed guess for "probably done by now".
func waitDrained(t *testing.T, d *Dispatcher) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(d.sem) == 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("webhook deliveries did not drain in time")
}

func TestDispatchDeliversEventWithSecret(t *testing.T) {
	var (
		mu     sync.Mutex
		got    Event
		secret string
		hits   int
	)
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		hits++
		secret = r.Header.Get("X-GPM-Webhook-Secret")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		select {
		case done <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	t.Setenv("WEBHOOK_TOKEN", "s3cr3t")
	d := New(func() []model.WebhookConfig {
		return []model.WebhookConfig{
			{Name: "ci", URL: srv.URL, Secret: model.Secret("${ENV:WEBHOOK_TOKEN}")},
			{Name: "off", URL: srv.URL, Disabled: true},
		}
	})
	d.Dispatch(Event{Action: "save", Kind: "ProxyHost", Name: "app", Commit: "abc123"})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not delivered")
	}
	// Wait for every in-flight delivery goroutine to release its semaphore
	// slot (see Dispatcher.sem), rather than sleeping and hoping: this proves
	// nothing else is still running, including an erroneously-fired disabled
	// target (there should be none).
	waitDrained(t, d)

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("expected exactly 1 delivery (disabled target skipped), got %d", hits)
	}
	if got.Action != "save" || got.Name != "app" || got.Commit != "abc123" {
		t.Fatalf("unexpected event payload: %+v", got)
	}
	if got.Time == "" {
		t.Fatal("event time should be stamped")
	}
	if secret != "s3cr3t" {
		t.Fatalf("resolved secret header = %q, want s3cr3t", secret)
	}
}

func TestDispatchNoTargetsIsNoop(t *testing.T) {
	New(nil).Dispatch(Event{Action: "save"})                                         // nil targets
	New(func() []model.WebhookConfig { return nil }).Dispatch(Event{Action: "save"}) // empty
}

func TestDispatchDoesNotFollowRedirects(t *testing.T) {
	var pivotHits, redirHits atomic.Int32
	pivot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pivotHits.Add(1)
	}))
	defer pivot.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirHits.Add(1)
		http.Redirect(w, r, pivot.URL, http.StatusFound)
	}))
	defer redirector.Close()

	d := New(func() []model.WebhookConfig {
		return []model.WebhookConfig{{Name: "r", URL: redirector.URL}}
	})
	d.Dispatch(Event{Action: "save"})

	deadline := time.Now().Add(2 * time.Second)
	for redirHits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if redirHits.Load() != 1 {
		t.Fatal("webhook target was never called")
	}
	// A followed redirect would happen inside the same client.Do() call that
	// is still delivering to redirHits' target, so wait for that delivery
	// goroutine to fully finish (drop its semaphore slot) rather than
	// sleeping and hoping a follow would have landed by now.
	waitDrained(t, d)
	if pivotHits.Load() != 0 {
		t.Fatalf("redirect was followed to a URL the admin never configured (%d hits)", pivotHits.Load())
	}
}

func TestRefuseLinkLocal(t *testing.T) {
	tests := []struct {
		addr    string
		blocked bool
	}{
		{"169.254.169.254:80", true}, // cloud metadata
		{"[fe80::1]:80", true},       // v6 link-local
		{"192.0.2.10:443", false},    // ordinary target
		{"10.0.0.5:80", false},       // private LAN receivers stay allowed
		{"example.com:443", false},   // unresolved name: Control sees IPs, names pass
	}
	for _, tc := range tests {
		err := refuseLinkLocal("tcp", tc.addr, nil)
		if tc.blocked && err == nil {
			t.Fatalf("%s: expected refusal", tc.addr)
		}
		if !tc.blocked && err != nil {
			t.Fatalf("%s: unexpected refusal: %v", tc.addr, err)
		}
	}
}

func TestStatusListsEveryTargetIncludingNeverFired(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	targets := []model.WebhookConfig{
		{Name: "ci", URL: srv.URL},
		{Name: "off", URL: srv.URL, Disabled: true},
	}
	d := New(func() []model.WebhookConfig { return targets })

	st := d.Status()
	if len(st) != 2 {
		t.Fatalf("Status() = %+v, want both targets listed before any delivery", st)
	}
	for _, dl := range st {
		if dl.LastAttempt != "" || dl.OK {
			t.Errorf("target %q reports a delivery that never happened: %+v", dl.Name, dl)
		}
	}
	if !st[1].Disabled {
		t.Errorf("disabled flag not carried through: %+v", st[1])
	}

	d.Dispatch(Event{Action: "save", Kind: "ProxyHost", Name: "app"})
	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// The async post records after the response, so poll for the recorded state.
	for time.Now().Before(deadline) {
		if s := d.Status(); s[0].LastAttempt != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	st = d.Status()
	if !st[0].OK || st[0].Status != http.StatusOK || st[0].LastAttempt == "" {
		t.Fatalf("enabled target state after a dispatch = %+v", st[0])
	}
	if st[1].LastAttempt != "" {
		t.Errorf("disabled target was delivered to: %+v", st[1])
	}

	// A target removed from settings drops out of the listing entirely.
	targets = targets[1:]
	st = d.Status()
	if len(st) != 1 || st[0].Name != "off" {
		t.Fatalf("Status() after removing a target = %+v", st)
	}
}

func TestTestSend(t *testing.T) {
	var got Event
	var secret string
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret = r.Header.Get("X-GPM-Webhook-Secret")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ok.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer broken.Close()

	t.Setenv("WEBHOOK_TOKEN", "s3cr3t")
	d := New(func() []model.WebhookConfig {
		return []model.WebhookConfig{
			{Name: "ci", URL: ok.URL, Secret: model.Secret("${ENV:WEBHOOK_TOKEN}")},
			{Name: "off", URL: ok.URL, Disabled: true},
			{Name: "broken", URL: broken.URL},
		}
	})

	tests := []struct {
		name       string
		target     string
		wantErr    error
		wantOK     bool
		wantStatus int
	}{
		{name: "enabled target", target: "ci", wantOK: true, wantStatus: http.StatusAccepted},
		// A disabled target is tested on purpose: proving a receiver before
		// turning it on is the whole point of the button.
		{name: "disabled target is still tested", target: "off", wantOK: true, wantStatus: http.StatusAccepted},
		{name: "receiver failure is reported, not raised", target: "broken", wantStatus: http.StatusBadGateway},
		{name: "unknown target", target: "ghost", wantErr: ErrUnknownTarget},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := d.Test(context.Background(), tc.target)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Test() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Test() error = %v", err)
			}
			if res.OK != tc.wantOK || res.Status != tc.wantStatus {
				t.Fatalf("Test() = %+v, want ok=%v status=%d", res, tc.wantOK, tc.wantStatus)
			}
			if res.Name != tc.target {
				t.Errorf("Test() name = %q, want %q", res.Name, tc.target)
			}
			if tc.wantOK && res.Error != "" {
				t.Errorf("Test() reported an error on success: %q", res.Error)
			}
			if !tc.wantOK && res.Error == "" {
				t.Error("Test() reported a failure with no error text")
			}
		})
	}

	if got.Action != "test" || got.Kind != "Webhook" || got.Name != "off" || got.Time == "" {
		t.Fatalf("last received event = %+v, want a stamped action:test event", got)
	}
	if secret != "s3cr3t" && secret != "" {
		t.Fatalf("unexpected secret header %q", secret)
	}

	// Every test send is recorded like an ordinary delivery.
	st := d.Status()
	byName := map[string]Delivery{}
	for _, dl := range st {
		byName[dl.Name] = dl
	}
	if !byName["ci"].OK || byName["ci"].LastAttempt == "" {
		t.Errorf("ci not recorded: %+v", byName["ci"])
	}
	if byName["broken"].OK || byName["broken"].Error == "" {
		t.Errorf("broken not recorded as a failure: %+v", byName["broken"])
	}
}

// TestDispatcherCloseWaitsForInFlightDelivery covers R2-L3's "give Dispatcher
// the same drain" fix: Close must block until a delivery already in flight has
// actually reached the receiver, not just return once Dispatch has been
// called.
func TestDispatcherCloseWaitsForInFlightDelivery(t *testing.T) {
	reached := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(reached)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := New(func() []model.WebhookConfig {
		return []model.WebhookConfig{{Name: "ci", URL: srv.URL + "/hook"}}
	})
	d.Dispatch(Event{Action: "save", Kind: "ProxyHost", Name: "app"})

	select {
	case <-reached:
	case <-time.After(2 * time.Second):
		t.Fatal("delivery never reached the receiver")
	}

	closed := make(chan struct{})
	go func() {
		d.Close()
		close(closed)
	}()

	select {
	case <-closed:
		t.Fatal("Close returned before the in-flight delivery finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return once the in-flight delivery finished (deadlock?)")
	}
}

// TestDispatcherCloseIsIdempotentAndNilSafe proves a second Close (and Close on
// a nil Dispatcher, the same nil-safety every other Dispatcher method has)
// neither panics nor blocks.
func TestDispatcherCloseIsIdempotentAndNilSafe(t *testing.T) {
	var nilDispatcher *Dispatcher
	nilDispatcher.Close()

	d := New(nil)
	d.Close()
	done := make(chan struct{})
	go func() { d.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second Close deadlocked")
	}
}

func TestTestSendOnUnwiredDispatcher(t *testing.T) {
	if _, err := New(nil).Test(context.Background(), "ci"); !errors.Is(err, ErrUnknownTarget) {
		t.Fatal("a dispatcher with no targets must report an unknown target")
	}
	var nilDispatcher *Dispatcher
	if got := nilDispatcher.Status(); len(got) != 0 {
		t.Fatalf("nil dispatcher Status() = %v, want empty", got)
	}
}
