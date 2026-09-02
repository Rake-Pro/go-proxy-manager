// Package webhook delivers best-effort, asynchronous lifecycle notifications when
// the configuration changes. Delivery never blocks or fails the originating config
// write: each target is POSTed in its own goroutine under a short timeout, and any
// error is logged, not propagated. This keeps a slow or unreachable receiver from
// stalling the admin API.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// maxInFlight bounds concurrent webhook deliveries process-wide. It is small on
// purpose: targets are few, the payload is tiny, and the cap exists to stop a
// slow receiver from turning every config write into another pile of goroutines
// and sockets.
const maxInFlight = 8

// Event is the JSON payload POSTed to each webhook target.
type Event struct {
	Action string `json:"action"` // save | delete | restore | revert | settings | ingress-discovery
	Kind   string `json:"kind,omitempty"`
	Name   string `json:"name,omitempty"`
	Commit string `json:"commit,omitempty"`
	Time   string `json:"time"` // RFC3339
}

// Dispatcher fans an Event out to the currently-configured webhook targets.
type Dispatcher struct {
	client  *http.Client
	targets func() []model.WebhookConfig // reads the live settings on each dispatch

	mu   sync.Mutex
	last map[string]Delivery // per-target outcome of the most recent POST

	// sem bounds how many deliveries may be in flight at once. Dispatch is
	// called from config-write handlers and fans out per enabled target, so an
	// unbounded "one goroutine per target per event" is a self-inflicted
	// amplifier when a receiver is slow: every write adds another full set.
	sem chan struct{}

	// wg tracks in-flight delivery goroutines so Close can drain them.
	wg sync.WaitGroup
}

// New returns a Dispatcher. targets is called on every Dispatch so configuration
// changes take effect without re-wiring; a nil targets disables delivery.
//
// Webhook URLs are admin-configured, which makes delivery an SSRF primitive
// from gpm's network position; two guards bound it (defense in depth):
// redirects are never followed (a receiver cannot bounce gpm to a URL the
// admin didn't configure - a 3xx is logged as a failed delivery), and
// link-local destinations are refused at connect time, post-DNS, so neither a
// direct URL nor a rebinding resolver can reach a cloud metadata service
// (169.254.169.254 / fe80::). Private-range targets stay allowed on purpose:
// LAN receivers are the normal self-hosted case, and the URL scheme/shape is
// already validated at config-write time.
func New(targets func() []model.WebhookConfig) *Dispatcher {
	return &Dispatcher{
		client:  NewSecureClient(10 * time.Second),
		targets: targets,
		last:    map[string]Delivery{},
		sem:     make(chan struct{}, maxInFlight),
	}
}

// NewSecureClient returns an http.Client hardened against SSRF the same way
// the webhook dispatcher's own client is: redirects are never followed (a 3xx
// is surfaced to the caller, not chased) and link-local destinations are
// refused at connect time, post-DNS (see refuseLinkLocal). Every admin-
// configured outbound integration - webhooks, and internal/notify's
// ntfy/Discord/generic targets - POSTs to an operator-supplied URL, so they
// share this one hardened client rather than each reimplementing the guard.
func NewSecureClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // surface the 3xx; never follow
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
				Control:   refuseLinkLocal,
			}).DialContext,
			MaxIdleConns:    8,
			IdleConnTimeout: 90 * time.Second,
		},
	}
}

// refuseLinkLocal is a dialer Control hook rejecting link-local destinations.
// Control runs per connection attempt with the RESOLVED address, so the check
// cannot be dodged by a DNS name (or a rebinding resolver) pointing at the
// metadata range.
func refuseLinkLocal(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return fmt.Errorf("webhook: link-local destination %s refused", ip)
	}
	return nil
}

// Close waits for every in-flight delivery goroutine to finish. It does not
// stop Dispatch from accepting more work; call it during process shutdown,
// after nothing new will call Dispatch, so a delivery already in flight is
// allowed to land instead of being abandoned mid-POST. Mirrors
// notify.Notifier.Close, which drains its own worker pool the same way. Safe
// to call on a nil Dispatcher and safe to call more than once.
func (d *Dispatcher) Close() {
	if d == nil {
		return
	}
	d.wg.Wait()
}

// Dispatch delivers ev to every enabled target asynchronously and returns
// immediately. It is safe to call from a request handler.
func (d *Dispatcher) Dispatch(ev Event) {
	if d == nil || d.targets == nil {
		return
	}
	if ev.Time == "" {
		ev.Time = time.Now().UTC().Format(time.RFC3339)
	}
	targets := d.targets()
	if len(targets) == 0 {
		return
	}
	body, err := json.Marshal(ev)
	if err != nil {
		log.Error().Err(err).Msg("webhook: marshal event")
		return
	}
	for _, t := range targets {
		if t.Disabled {
			continue
		}
		select {
		case d.sem <- struct{}{}:
			d.wg.Add(1)
			go func(t model.WebhookConfig) {
				defer func() { <-d.sem; d.wg.Done() }()
				d.post(t, body)
			}(t)
		default:
			// Delivery is best-effort and must never block a config write, so a
			// full pool drops this event for this target rather than queueing.
			log.Warn().Str("webhook", t.Name).Int("inFlight", maxInFlight).
				Msg("webhook: too many deliveries in flight, dropped this event for this target")
		}
	}
}

func (d *Dispatcher) post(t model.WebhookConfig, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status, dur, err := d.deliver(ctx, t, body)
	d.record(t, status, dur, err)
	switch {
	case err != nil:
		log.Warn().Err(err).Str("webhook", t.Name).Str("url", t.URL).Msg("webhook: delivery failed")
	case status >= 300:
		log.Warn().Str("webhook", t.Name).Int("status", status).Msg("webhook: non-2xx response")
	default:
		log.Debug().Str("webhook", t.Name).Int("status", status).Msg("webhook delivered")
	}
}

// deliver performs one POST and reports the status, how long it took, and any
// transport-level error. A non-2xx status is NOT an error here: the caller
// decides what to make of it, so both the async path and the synchronous test
// classify a 500 from the receiver the same way.
func (d *Dispatcher) deliver(ctx context.Context, t model.WebhookConfig, body []byte) (int, time.Duration, error) {
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		return 0, time.Since(started), fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "go-proxy-manager-webhook")
	if !t.Secret.IsEmpty() {
		secret, err := t.Secret.Resolve()
		if err != nil {
			return 0, time.Since(started), fmt.Errorf("resolve secret: %w", err)
		}
		req.Header.Set("X-GPM-Webhook-Secret", secret)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, time.Since(started), err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode, time.Since(started), nil
}

// record stores the outcome of the most recent POST to a target. It is in-memory
// and per-process on purpose: it is an operational hint ("did the last one land"),
// never config, so it is not committed and does not survive a restart.
func (d *Dispatcher) record(t model.WebhookConfig, status int, dur time.Duration, err error) {
	dl := Delivery{
		Name:        t.Name,
		URL:         t.URL,
		LastAttempt: time.Now().UTC().Format(time.RFC3339),
		Status:      status,
		DurationMS:  dur.Milliseconds(),
		OK:          err == nil && status >= 200 && status < 300,
	}
	if err != nil {
		dl.Error = err.Error()
	} else if status >= 300 {
		dl.Error = fmt.Sprintf("receiver answered HTTP %d", status)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.last[t.Name] = dl
}

// Delivery is the outcome of the most recent POST to one webhook target.
type Delivery struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Disabled    bool   `json:"disabled"`
	LastAttempt string `json:"lastAttempt,omitempty"` // RFC3339, empty = never fired
	Status      int    `json:"status,omitempty"`      // 0 when the request never got a response
	DurationMS  int64  `json:"durationMs,omitempty"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
}

// Status reports every configured target with the outcome of its most recent
// delivery attempt. Targets that have never fired are listed with an empty
// LastAttempt rather than omitted, so the UI can show "never delivered" instead
// of nothing at all. Entries for targets that no longer exist are dropped.
func (d *Dispatcher) Status() []Delivery {
	if d == nil || d.targets == nil {
		return []Delivery{}
	}
	targets := d.targets()
	out := make([]Delivery, 0, len(targets))
	d.mu.Lock()
	defer d.mu.Unlock()
	live := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		live[t.Name] = struct{}{}
		dl := d.last[t.Name]
		dl.Name, dl.URL, dl.Disabled = t.Name, t.URL, t.Disabled
		out = append(out, dl)
	}
	for name := range d.last {
		if _, ok := live[name]; !ok {
			delete(d.last, name)
		}
	}
	return out
}

// ErrUnknownTarget is returned by Test when no webhook of that name is configured.
var ErrUnknownTarget = errors.New("no webhook target with that name is configured")

// TestResult is the synchronous outcome of Test.
type TestResult struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Status     int    `json:"status"`
	DurationMS int64  `json:"durationMs"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

// TestTimeout bounds a synchronous test delivery. It is much shorter than the
// async path's: an operator is watching a spinner, and the answer "it did not
// respond in 5s" is the useful one.
const TestTimeout = 5 * time.Second

// Test POSTs a synthetic action:"test" event to one target and waits for the
// result, so an operator can prove a receiver works without making a config
// change. A DISABLED target is still tested on purpose - that is how you check a
// receiver before turning it on - and the result records the attempt like any
// other delivery.
//
// A transport failure or a non-2xx status is reported in the result, not as an
// error: only an unknown target name is an error (ErrUnknownTarget), because
// that is the one case with nothing to report on.
func (d *Dispatcher) Test(ctx context.Context, name string) (TestResult, error) {
	if d == nil || d.targets == nil {
		return TestResult{}, ErrUnknownTarget
	}
	var target *model.WebhookConfig
	for _, t := range d.targets() {
		if t.Name == name {
			t := t
			target = &t
			break
		}
	}
	if target == nil {
		return TestResult{}, ErrUnknownTarget
	}
	body, err := json.Marshal(Event{
		Action: "test",
		Kind:   "Webhook",
		Name:   target.Name,
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return TestResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, TestTimeout)
	defer cancel()
	status, dur, derr := d.deliver(ctx, *target, body)
	d.record(*target, status, dur, derr)
	res := TestResult{
		Name:       target.Name,
		URL:        target.URL,
		Status:     status,
		DurationMS: dur.Milliseconds(),
		OK:         derr == nil && status >= 200 && status < 300,
	}
	switch {
	case derr != nil:
		res.Error = derr.Error()
	case status >= 300:
		res.Error = fmt.Sprintf("receiver answered HTTP %d", status)
	}
	return res, nil
}
