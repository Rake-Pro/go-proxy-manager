package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

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
	// Give any erroneously-fired disabled target a moment (there should be none).
	time.Sleep(50 * time.Millisecond)

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
	time.Sleep(100 * time.Millisecond) // a follow, if any, would land by now
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
