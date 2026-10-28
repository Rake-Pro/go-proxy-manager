package dataplane

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/analytics"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

type fakeSink struct {
	mu     sync.Mutex
	active bool
	events []analytics.Event
}

func (f *fakeSink) Record(ev analytics.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
}
func (f *fakeSink) Active() bool { return f.active }

// The observe wrapper hands every request to the sink with the denial reason
// and signature the tiers noted, and nothing reaches the sink while it is off.
func TestObserveFeedsAnalyticsSink(t *testing.T) {
	sink := &fakeSink{active: true}
	SetAnalyticsSink(sink)
	t.Cleanup(func() { SetAnalyticsSink(nil) })

	s := New(Config{})
	h := s.observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/deny" {
			countDenial(r, "guard")
			noteSignature(r, "guard:login")
			noteSignature(r, "guard:second") // first label wins
			countDenial(r, "bouncer")        // first reason wins
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte("ok"))
	}))
	for _, p := range []string{"/", "/deny"} {
		req := httptest.NewRequest("GET", "http://app.example"+p, nil)
		req.Header.Set("User-Agent", "zgrab/0.x")
		req.RemoteAddr = "203.0.113.9:4000"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if len(sink.events) != 2 {
		t.Fatalf("events = %d", len(sink.events))
	}
	ok, den := sink.events[0], sink.events[1]
	if ok.Status != 200 || ok.Deny != "" || ok.Signature != "" || ok.Host != unknownHostLabel || ok.UserAgent != "zgrab/0.x" || ok.Client.String() != "203.0.113.9" || ok.BytesOut != 2 {
		t.Errorf("served event = %+v", ok)
	}
	if den.Status != 403 || den.Deny != "guard" || den.Signature != "guard:login" || den.Path != "/deny" {
		t.Errorf("denied event = %+v", den)
	}

	sink.active = false
	if analyticsActive() {
		t.Fatal("inactive sink must not count as active")
	}
	h2 := s.observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqMetaOf(r) != nil {
			t.Error("no meta must be allocated while the sink is off")
		}
	}))
	h2.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "http://app.example/", nil))
	if len(sink.events) != 2 {
		t.Fatalf("inactive sink received an event")
	}
}

// Named guard triggers report their label; unnamed ones only the reason.
func TestGuardNamedTriggerNotesSignature(t *testing.T) {
	sink := &fakeSink{active: true}
	SetAnalyticsSink(sink)
	t.Cleanup(func() { SetAnalyticsSink(nil) })
	s := New(Config{})
	g := compileGuard(guardSpec(t))
	inner := guardHandler(g, peerIP, "app", nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	h := s.observe(inner)
	for _, p := range []string{"/xmlrpc.php", "/wp-login.php"} {
		req := httptest.NewRequest("POST", "http://app.example"+p, nil)
		req.RemoteAddr = "203.0.113.9:1"
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if len(sink.events) != 2 || sink.events[0].Signature != "guard:xmlrpc" || sink.events[0].Deny != "guard" || sink.events[1].Signature != "" || sink.events[1].Deny != "guard" {
		t.Errorf("events = %+v", sink.events)
	}
}

func guardSpec(t *testing.T) model.GuardMiddleware {
	t.Helper()
	return model.GuardMiddleware{Triggers: []model.GuardTrigger{
		{Name: "xmlrpc", Paths: []string{"/xmlrpc.php"}},
		{Paths: []string{"/wp-login.php"}},
	}}
}
