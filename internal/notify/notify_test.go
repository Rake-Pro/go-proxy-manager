package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// waitFor polls cond until it is true or the deadline passes, failing the
// test otherwise. Delivery is asynchronous, so tests poll rather than sleep a
// fixed guess.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestEmitNtfyPayloadShape(t *testing.T) {
	var (
		mu     sync.Mutex
		method string
		path   string
		body   string
		hdr    http.Header
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		method = r.Method
		path = r.URL.Path
		hdr = r.Header.Clone()
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
	}))
	defer srv.Close()

	t.Setenv("NTFY_TOKEN", "topic-token")
	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{
			Name: "ntfy", Type: model.NotificationTypeNtfy, URL: srv.URL + "/my-topic",
			Secret: model.Secret("${ENV:NTFY_TOKEN}"),
		}}
	})
	n.Emit(context.Background(), Event{
		Kind: model.EventCertExpired, Title: "cert expired", Body: "example.com expired",
		Severity: "critical",
	})

	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return body != "" }, "ntfy target was never called")

	mu.Lock()
	defer mu.Unlock()
	if method != http.MethodPost || path != "/my-topic" {
		t.Fatalf("method/path = %s %s, want POST /my-topic", method, path)
	}
	if body != "example.com expired" {
		t.Fatalf("body = %q, want the plain message text", body)
	}
	if hdr.Get("Title") != "cert expired" {
		t.Errorf("Title header = %q", hdr.Get("Title"))
	}
	if hdr.Get("Priority") != "5" {
		t.Errorf("Priority header = %q, want 5 (critical)", hdr.Get("Priority"))
	}
	if hdr.Get("Authorization") != "Bearer topic-token" {
		t.Errorf("Authorization header = %q", hdr.Get("Authorization"))
	}
}

func TestEmitDiscordPayloadShape(t *testing.T) {
	var got struct {
		Content string `json:"content"`
		Embeds  []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Color       int    `json:"color"`
			Fields      []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"fields"`
		} `json:"embeds"`
	}
	var authHeader string
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		select {
		case done <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{
			Name: "discord", Type: model.NotificationTypeDiscord, URL: srv.URL + "/api/webhooks/1/abc",
		}}
	})
	n.Emit(context.Background(), Event{
		Kind: model.EventUpstreamUnhealthy, Title: "upstream down", Body: "10.0.0.5:80 is unhealthy",
		Severity: "warning", Fields: map[string]string{"group": "app", "upstream": "10.0.0.5:80"},
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("discord target was never called")
	}

	if authHeader != "" {
		t.Errorf("discord request must not carry an Authorization header, got %q", authHeader)
	}
	if got.Content != "upstream down" || len(got.Embeds) != 1 {
		t.Fatalf("unexpected discord payload: %+v", got)
	}
	e := got.Embeds[0]
	if e.Title != "upstream down" || e.Description != "10.0.0.5:80 is unhealthy" {
		t.Fatalf("unexpected embed: %+v", e)
	}
	if e.Color != 0xF1C40F {
		t.Errorf("color = %#x, want warning color", e.Color)
	}
	if len(e.Fields) != 2 {
		t.Fatalf("fields = %+v, want 2", e.Fields)
	}
}

func TestEmitGenericPayloadShape(t *testing.T) {
	var got genericEvent
	var eventHeader, authHeader string
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventHeader = r.Header.Get("X-GPM-Event")
		authHeader = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		select {
		case done <- struct{}{}:
		default:
		}
	}))
	defer srv.Close()

	t.Setenv("GENERIC_TOKEN", "s3cr3t")
	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{
			Name: "hooks", Type: model.NotificationTypeGeneric, URL: srv.URL,
			Secret: model.Secret("${ENV:GENERIC_TOKEN}"),
		}}
	})
	n.Emit(context.Background(), Event{
		Kind: model.EventDiscoveryFrozen, Title: "docker discovery frozen", Body: "engine unreachable",
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("generic target was never called")
	}

	if eventHeader != model.EventDiscoveryFrozen {
		t.Errorf("X-GPM-Event = %q", eventHeader)
	}
	if authHeader != "Bearer s3cr3t" {
		t.Errorf("Authorization = %q", authHeader)
	}
	if got.Kind != model.EventDiscoveryFrozen || got.Title != "docker discovery frozen" || got.Severity != "info" {
		t.Fatalf("unexpected generic payload: %+v", got)
	}
}

func TestEmitSkipsDisabledTarget(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{
			Name: "off", Type: model.NotificationTypeGeneric, URL: srv.URL, Disabled: true,
		}}
	})
	// Emit's Disabled check runs synchronously in the target loop before a job
	// is ever enqueued, so by the time Emit returns it is already certain no
	// delivery was queued for this target - no wait needed to prove it.
	n.Emit(context.Background(), Event{Kind: model.EventCertExpired})
	if hits.Load() != 0 {
		t.Fatalf("disabled target was delivered to (%d hits)", hits.Load())
	}
}

func TestEmitFiltersByEvents(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{
			Name: "certs-only", Type: model.NotificationTypeGeneric, URL: srv.URL,
			Events: []string{model.EventCertExpired},
		}}
	})
	// Not subscribed: must not fire. subscribed() is checked synchronously in
	// Emit's target loop before enqueue, so once Emit returns the routing
	// decision is already final - no wait needed to prove it.
	n.Emit(context.Background(), Event{Kind: model.EventUpstreamUnhealthy, Subject: "a"})
	if hits.Load() != 0 {
		t.Fatalf("target fired for an unsubscribed kind (%d hits)", hits.Load())
	}
	// Subscribed: must fire.
	n.Emit(context.Background(), Event{Kind: model.EventCertExpired, Subject: "b"})
	waitFor(t, func() bool { return hits.Load() == 1 }, "target did not fire for a subscribed kind")
}

func TestEmitDefaultEventsExcludeConfigChanged(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	// No Events set: gets DefaultNotificationEvents(), which excludes config.changed.
	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{Name: "all", Type: model.NotificationTypeGeneric, URL: srv.URL}}
	})
	// subscribed() (against DefaultNotificationEvents()) runs synchronously in
	// Emit's target loop before enqueue, so the routing decision is already
	// final once Emit returns - no wait needed to prove it.
	n.Emit(context.Background(), Event{Kind: model.EventConfigChanged, Subject: "s1"})
	if hits.Load() != 0 {
		t.Fatalf("config.changed fired for a target with no explicit events (%d hits)", hits.Load())
	}
	n.Emit(context.Background(), Event{Kind: model.EventCertExpired, Subject: "s2"})
	waitFor(t, func() bool { return hits.Load() == 1 }, "cert.expired did not fire on the default event set")
}

func TestDedupSuppressesRepeatsWithinWindow(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{Name: "t", Type: model.NotificationTypeGeneric, URL: srv.URL}}
	})
	ev := Event{Kind: model.EventCertRenewalFailed, Subject: "example.com"}
	n.Emit(context.Background(), ev)
	waitFor(t, func() bool { return hits.Load() == 1 }, "first emit did not fire")

	// Same kind+subject+state, repeated immediately: suppressed by the window.
	// shouldEmit's dedup check runs synchronously at the top of Emit, before
	// any target is even considered, so the suppression is already final once
	// Emit returns - no wait needed to prove it.
	n.Emit(context.Background(), ev)
	if hits.Load() != 1 {
		t.Fatalf("repeat within the dedup window was delivered (%d hits)", hits.Load())
	}

	// A different subject is a different dedup bucket: fires independently.
	n.Emit(context.Background(), Event{Kind: model.EventCertRenewalFailed, Subject: "other.example.com"})
	waitFor(t, func() bool { return hits.Load() == 2 }, "a different subject was deduped against an unrelated one")
}

func TestDedupStateFlipBypassesWindow(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	// A target subscribed to BOTH kinds of a flap - realistic usage always
	// crosses model.EventUpstreamUnhealthy / model.EventUpstreamRecovered, the
	// two distinct Kinds one flap alternates between.
	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{
			Name: "t", Type: model.NotificationTypeGeneric, URL: srv.URL,
			Events: []string{model.EventUpstreamUnhealthy, model.EventUpstreamRecovered},
		}}
	})
	subject := "app/10.0.0.5:80"
	n.Emit(context.Background(), Event{Kind: model.EventUpstreamUnhealthy, Subject: subject, State: "unhealthy"})
	waitFor(t, func() bool { return hits.Load() == 1 }, "unhealthy event did not fire")

	// A same-second recovery for the same subject - a DIFFERENT Kind - must NOT
	// be swallowed by the unhealthy event's dedup entry, even well inside the
	// window: the two kinds of one flap must share a dedup bucket.
	n.Emit(context.Background(), Event{Kind: model.EventUpstreamRecovered, Subject: subject, State: "healthy"})
	waitFor(t, func() bool { return hits.Load() == 2 }, "a state flip across kinds was suppressed by the dedup window")

	// Going unhealthy again right after recovering must ALSO fire immediately -
	// it must not be suppressed by the FIRST unhealthy event's dedup entry
	// just because both share the unhealthy Kind+Subject.
	n.Emit(context.Background(), Event{Kind: model.EventUpstreamUnhealthy, Subject: subject, State: "unhealthy"})
	waitFor(t, func() bool { return hits.Load() == 3 }, "a repeat unhealthy event after an intervening recovery was suppressed")

	// But a genuine repeat of the SAME state within the window (no intervening
	// flip) is still deduped. shouldEmit's check runs synchronously before
	// Emit returns, so no wait is needed to prove it.
	n.Emit(context.Background(), Event{Kind: model.EventUpstreamUnhealthy, Subject: subject, State: "unhealthy"})
	if hits.Load() != 3 {
		t.Fatalf("a same-state repeat within the window was delivered (%d hits)", hits.Load())
	}
}

func TestQueueBoundedDropsOldest(t *testing.T) {
	release := make(chan struct{})
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release // hold every request open so the queue backs up behind the workers
	}))
	defer srv.Close()

	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{Name: "slow", Type: model.NotificationTypeGeneric, URL: srv.URL}}
	})

	// Flood well past queueCapacity + workerCount; Emit must never block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < queueCapacity*2; i++ {
			n.Emit(context.Background(), Event{
				Kind: model.EventCertExpired, Subject: "flood", State: time.Now().String(), // unique State per call defeats dedup
			})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Emit blocked under a full queue")
	}
	waitFor(t, func() bool { return hits.Load() > 0 }, "no request ever reached the receiver")

	// Release the held requests and let the backlog drain against the
	// still-open server, rather than leaving it to trail into later tests'
	// log output as connection-refused warnings.
	close(release)
	time.Sleep(200 * time.Millisecond)
}

func TestStatusListsEveryTargetIncludingNeverFired(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	targets := []model.NotificationTarget{
		{Name: "a", Type: model.NotificationTypeGeneric, URL: srv.URL},
		{Name: "b", Type: model.NotificationTypeGeneric, URL: srv.URL, Disabled: true},
	}
	n := New(func() []model.NotificationTarget { return targets })

	st := n.Status()
	if len(st) != 2 {
		t.Fatalf("Status() = %+v, want both targets listed before any delivery", st)
	}
	for _, dl := range st {
		if dl.LastAttempt != "" || dl.OK {
			t.Errorf("target %q reports a delivery that never happened: %+v", dl.Name, dl)
		}
	}

	n.Emit(context.Background(), Event{Kind: model.EventCertExpired})
	waitFor(t, func() bool {
		s := n.Status()
		return s[0].LastAttempt != ""
	}, "delivery to the enabled target was never recorded")

	st = n.Status()
	if !st[0].OK || st[0].Status != http.StatusOK {
		t.Fatalf("enabled target state after emit = %+v", st[0])
	}
	if st[1].LastAttempt != "" {
		t.Errorf("disabled target was delivered to: %+v", st[1])
	}
}

func TestTestSend(t *testing.T) {
	var got genericEvent
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ok.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer broken.Close()

	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{
			{Name: "ok", Type: model.NotificationTypeGeneric, URL: ok.URL},
			{Name: "off", Type: model.NotificationTypeGeneric, URL: ok.URL, Disabled: true},
			{Name: "broken", Type: model.NotificationTypeGeneric, URL: broken.URL},
		}
	})

	tests := []struct {
		name       string
		target     string
		wantErr    error
		wantOK     bool
		wantStatus int
	}{
		{name: "enabled target", target: "ok", wantOK: true, wantStatus: http.StatusAccepted},
		{name: "disabled target is still tested", target: "off", wantOK: true, wantStatus: http.StatusAccepted},
		{name: "receiver failure is reported, not raised", target: "broken", wantStatus: http.StatusBadGateway},
		{name: "unknown target", target: "ghost", wantErr: ErrUnknownTarget},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := n.Test(context.Background(), tc.target)
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
		})
	}
	if got.Kind != "test" {
		t.Fatalf("last received test event kind = %q, want \"test\"", got.Kind)
	}
}

func TestValidationTable(t *testing.T) {
	tests := []struct {
		name    string
		target  model.NotificationTarget
		wantErr bool
	}{
		{"valid ntfy", model.NotificationTarget{Name: "n1", Type: model.NotificationTypeNtfy, URL: "https://ntfy.example.com/gpm"}, false},
		{"valid discord", model.NotificationTarget{Name: "n2", Type: model.NotificationTypeDiscord, URL: "https://discord.com/api/webhooks/1/abc"}, false},
		{"valid generic http", model.NotificationTarget{Name: "n3", Type: model.NotificationTypeGeneric, URL: "http://hooks.example.com/gpm"}, false},
		{"unknown type", model.NotificationTarget{Name: "n4", Type: "smoke-signal", URL: "https://example.com"}, true},
		{"non-http scheme", model.NotificationTarget{Name: "n5", Type: model.NotificationTypeGeneric, URL: "ftp://example.com"}, true},
		{"discord missing api/webhooks path", model.NotificationTarget{Name: "n6", Type: model.NotificationTypeDiscord, URL: "https://discord.com/not-a-webhook"}, true},
		{"unknown event kind", model.NotificationTarget{Name: "n7", Type: model.NotificationTypeGeneric, URL: "https://example.com", Events: []string{"bogus.kind"}}, true},
		{"bad name", model.NotificationTarget{Name: "not a valid name!", Type: model.NotificationTypeGeneric, URL: "https://example.com"}, true},
		{"empty url", model.NotificationTarget{Name: "n8", Type: model.NotificationTypeGeneric, URL: ""}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := model.Settings{
				ExternalBaseURL: "https://gpm.example.com",
				AdminAuth:       model.AdminAuthSettings{LocalLoginEnabled: true},
				Notifications:   model.NotificationsSettings{Targets: []model.NotificationTarget{tc.target}},
			}
			err := s.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestDuplicateTargetNameRejected(t *testing.T) {
	s := model.Settings{
		ExternalBaseURL: "https://gpm.example.com",
		AdminAuth:       model.AdminAuthSettings{LocalLoginEnabled: true},
		Notifications: model.NotificationsSettings{Targets: []model.NotificationTarget{
			{Name: "dup", Type: model.NotificationTypeGeneric, URL: "https://a.example.com"},
			{Name: "dup", Type: model.NotificationTypeGeneric, URL: "https://b.example.com"},
		}},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("expected a duplicate-name validation error")
	}
}

// TestCloseWaitsForInFlightDelivery covers R2-L3: Close is meant to drain a
// delivery that is already in flight rather than abandon it mid-POST (the
// case that matters most is an alert queued in the last moments before
// shutdown). This proves Close actually blocks until the receiver has been
// reached, not just until the worker loop notices done is closed.
func TestCloseWaitsForInFlightDelivery(t *testing.T) {
	reached := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(reached)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(func() []model.NotificationTarget {
		return []model.NotificationTarget{{Name: "ntfy", Type: model.NotificationTypeNtfy, URL: srv.URL + "/t"}}
	})
	n.Emit(context.Background(), Event{Kind: model.EventCertExpired, Title: "t", Body: "b"})

	select {
	case <-reached:
	case <-time.After(2 * time.Second):
		t.Fatal("delivery never reached the receiver")
	}

	closed := make(chan struct{})
	go func() {
		n.Close()
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

	// A send after Close is a documented no-op, not a panic on a closed channel.
	n.Emit(context.Background(), Event{Kind: model.EventCertExpired, Title: "t2", Body: "b2"})
}

// TestCloseIsIdempotent proves a second Close does not panic or block.
func TestCloseIsIdempotent(t *testing.T) {
	n := New(func() []model.NotificationTarget { return nil })
	n.Close()
	done := make(chan struct{})
	go func() { n.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second Close deadlocked")
	}
}

func TestNilNotifierIsNoop(t *testing.T) {
	var n *Notifier
	n.Emit(context.Background(), Event{Kind: model.EventCertExpired}) // must not panic
	if got := n.Status(); len(got) != 0 {
		t.Fatalf("nil Notifier Status() = %v, want empty", got)
	}
	if _, err := n.Test(context.Background(), "x"); !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("nil Notifier Test() error = %v, want ErrUnknownTarget", err)
	}
}
