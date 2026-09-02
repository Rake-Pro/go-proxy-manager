// Package notify delivers best-effort, asynchronous operational alerts - cert
// renewal failure, approaching/actual expiry, upstream health flaps, ACME
// account errors, a frozen discovery reconciler, and (opt-in) config changes -
// to admin-configured ntfy, Discord, or generic-webhook targets.
//
// It mirrors internal/webhook's shape (async dispatch, per-target
// last-delivery status, a synchronous test send) and reuses its SSRF-hardened
// HTTP client (webhook.NewSecureClient) and Delivery/TestResult types rather
// than building a second implementation of either. The two packages stay
// separate because their config objects, payload shapes and event-filtering
// rules differ enough that merging them would make both harder to read.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/webhook"
	"github.com/rs/zerolog/log"
)

// Event is one operational event a source (ACME, upstream health, a
// discovery reconciler, a config write) reports to Emit.
type Event struct {
	// Kind is a settings.notifications event kind, e.g. model.EventCertExpired.
	Kind string
	// Subject scopes deduplication narrower than Kind alone - e.g. the
	// certificate or upstream name - so "cert A renewal failed" does not
	// suppress "cert B renewal failed". Empty defaults to Kind (one bucket per
	// kind, e.g. the daily cert.expiring digest, which has exactly one message).
	Subject string
	// Title is the short summary sent as the ntfy Title header, the Discord
	// embed title, and the generic envelope's "title".
	Title string
	// Body is the human-readable detail: the ntfy message body, the Discord
	// embed description, and the generic envelope's "body".
	Body string
	// Fields is optional structured detail, rendered as Discord embed fields
	// and included verbatim in the generic envelope. Nil is fine.
	Fields map[string]string
	// Severity is "info" (default), "warning", or "critical". It selects the
	// ntfy priority/tag and the Discord embed color.
	Severity string
	// State is an optional token distinguishing this event's current state
	// (e.g. "unhealthy" vs "healthy" for an upstream flap). Two events for the
	// same Kind+Subject with a DIFFERENT State always bypass the dedup window -
	// a recovery must never be swallowed by a recent failure's dedup entry.
	State string
}

func severityOrDefault(s string) string {
	if s == "" {
		return "info"
	}
	return s
}

// dedupWindow is how long a repeat of the same Kind+Subject+State is
// suppressed. An hour matches the coarsest recurring source (the daily
// cert.expiring digest fires once a day anyway); every other source is a
// state transition, which the State field lets bypass the window immediately.
const dedupWindow = time.Hour

// queueCapacity bounds the number of pending (event, target) delivery jobs.
// It is generous relative to any plausible burst (a config-changed storm, a
// health-check flap on every upstream in a group at once) while still being a
// hard cap: Emit must never block a caller on a stuck or slow receiver.
const queueCapacity = 256

// workerCount is the number of goroutines draining the delivery queue. Small
// and fixed: notifications are low-volume by nature (they exist specifically
// to NOT be per-request), so a handful of workers is ample headroom.
const workerCount = 4

// deliverTimeout bounds one HTTP POST to a target.
const deliverTimeout = 10 * time.Second

// TestTimeout bounds a synchronous Test call, mirroring webhook.TestTimeout:
// an operator is watching a spinner, so "no answer in 5s" is the useful one.
const TestTimeout = 5 * time.Second

// ErrUnknownTarget is returned by Test when no notification target of that
// name is configured.
var ErrUnknownTarget = errors.New("no notification target with that name is configured")

type dedupEntry struct {
	at    time.Time
	state string
}

type job struct {
	target model.NotificationTarget
	ev     Event
}

// Notifier fans an Event out to the currently-configured, subscribed,
// enabled notification targets. It never blocks the caller: Emit enqueues
// onto a bounded queue drained by a small worker pool, dropping the oldest
// pending job (with a WARN) rather than growing without bound or stalling.
type Notifier struct {
	client  *http.Client
	targets func() []model.NotificationTarget

	mu   sync.Mutex
	last map[string]webhook.Delivery // per-target outcome of the most recent send

	dmu   sync.Mutex
	dedup map[string]dedupEntry // key: kind + "\x00" + subject

	queue chan job
	done  chan struct{}
	wg    sync.WaitGroup
	// closed is checked before every enqueue so a send after Close cannot panic
	// on a closed channel.
	closed    atomic.Bool
	closeOnce sync.Once
}

// New returns a Notifier and starts its worker pool. targets is called on
// every Emit/Test so a settings change takes effect without re-wiring; a nil
// targets disables delivery (Emit and Test become no-ops).
func New(targets func() []model.NotificationTarget) *Notifier {
	n := &Notifier{
		client:  webhook.NewSecureClient(deliverTimeout),
		targets: targets,
		last:    map[string]webhook.Delivery{},
		dedup:   map[string]dedupEntry{},
		queue:   make(chan job, queueCapacity),
		done:    make(chan struct{}),
	}
	n.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go n.worker()
	}
	return n
}

func (n *Notifier) worker() {
	defer n.wg.Done()
	for {
		select {
		case <-n.done:
			return
		case j := <-n.queue:
			n.deliver(j.target, j.ev)
		}
	}
}

// Close stops the worker pool and waits for the deliveries in flight. A
// Notifier built once per process never needs it; it exists so a caller that
// builds one per reload (or a test) does not leak workers. Emit after Close is
// a no-op, and whatever was still queued is dropped - delivery is best-effort
// by design. The queue channel is deliberately NOT closed: a producer racing
// Close would panic on the send.
func (n *Notifier) Close() {
	n.closeOnce.Do(func() {
		n.closed.Store(true)
		close(n.done)
		n.wg.Wait()
	})
}

// enqueue is the bounded, non-blocking push: if the queue is full it drops
// the oldest pending job to make room for this one, logging a WARN either
// way. It never blocks the caller.
func (n *Notifier) enqueue(j job) {
	if n.closed.Load() {
		return
	}
	select {
	case n.queue <- j:
		return
	default:
	}
	select {
	case <-n.queue:
		log.Warn().Str("kind", j.ev.Kind).Str("target", j.target.Name).
			Msg("notify: queue full, dropped the oldest pending notification")
	default:
	}
	select {
	case n.queue <- j:
	default:
		// Lost the race to another producer between the drop and this push;
		// dropping THIS job (instead of blocking) keeps the "never blocks" promise.
		log.Warn().Str("kind", j.ev.Kind).Str("target", j.target.Name).
			Msg("notify: queue full, dropped notification")
	}
}

// Emit delivers ev to every enabled target subscribed to ev.Kind, subject to
// the dedup window. It returns immediately; delivery happens on the worker
// pool under its own timeout, detached from ctx - the caller (a health check,
// an ACME renewal, a request handler) must be free to return without
// cancelling an in-flight send.
func (n *Notifier) Emit(_ context.Context, ev Event) {
	if n == nil || n.targets == nil || ev.Kind == "" {
		return
	}
	if !n.shouldEmit(ev) {
		return
	}
	for _, t := range n.targets() {
		if t.Disabled || !subscribed(t, ev.Kind) {
			continue
		}
		n.enqueue(job{target: t, ev: ev})
	}
}

func subscribed(t model.NotificationTarget, kind string) bool {
	events := t.Events
	if len(events) == 0 {
		events = model.DefaultNotificationEvents()
	}
	for _, e := range events {
		if e == kind {
			return true
		}
	}
	return false
}

// shouldEmit applies the dedup window: a repeat within dedupWindow is
// suppressed UNLESS State differs from the last recorded value for the same
// key, which always bypasses the window - a recovery must never be
// swallowed by a recent failure's dedup entry.
//
// A STATELESS event (State == "", e.g. cert.renewal_failed, the daily
// cert.expiring digest) keys on Kind+Subject: two different kinds about the
// same subject (a cert's renewal failure vs its expiry digest) must not
// suppress each other.
//
// A STATE-TRACKED event (State != "", e.g. an upstream health flap) keys on
// Subject ALONE, deliberately spanning its whole family of Kinds
// (upstream.unhealthy / upstream.recovered): a flap uses a DIFFERENT Kind for
// each direction, and the bypass-on-flip guarantee only works if both
// directions read and write the same dedup entry.
func (n *Notifier) shouldEmit(ev Event) bool {
	subject := ev.Subject
	if subject == "" {
		subject = ev.Kind
	}
	key := subject
	if ev.State == "" {
		key = ev.Kind + "\x00" + subject
	}
	now := time.Now()

	n.dmu.Lock()
	defer n.dmu.Unlock()
	n.gcDedupLocked(now)
	if prev, ok := n.dedup[key]; ok && prev.state == ev.State && now.Sub(prev.at) < dedupWindow {
		return false
	}
	n.dedup[key] = dedupEntry{at: now, state: ev.State}
	return true
}

// gcDedupLocked drops entries older than the dedup window, which can no longer
// suppress anything. Subjects come from configuration today (cert, host and
// upstream names), so the map is bounded in practice - but nothing structural
// bounded it, and an expiry sweep on write is the same shape the login gate
// uses. Caller holds dmu.
func (n *Notifier) gcDedupLocked(now time.Time) {
	for k, e := range n.dedup {
		if now.Sub(e.at) >= dedupWindow {
			delete(n.dedup, k)
		}
	}
}

func (n *Notifier) deliver(t model.NotificationTarget, ev Event) {
	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()
	status, dur, err := n.post(ctx, t, ev)
	n.record(t, status, dur, err)
	switch {
	case err != nil:
		log.Warn().Err(err).Str("target", t.Name).Str("kind", ev.Kind).Msg("notify: delivery failed")
	case status >= 300:
		log.Warn().Str("target", t.Name).Int("status", status).Msg("notify: non-2xx response")
	default:
		log.Debug().Str("target", t.Name).Int("status", status).Msg("notify: delivered")
	}
}

// post performs one send and reports the status, duration and any
// transport-level error. A non-2xx status is not an error here, matching
// webhook.deliver: the caller classifies it, so Test and the worker path
// agree on what a receiver's 500 means.
func (n *Notifier) post(ctx context.Context, t model.NotificationTarget, ev Event) (int, time.Duration, error) {
	started := time.Now()
	req, err := buildRequest(ctx, t, ev)
	if err != nil {
		return 0, time.Since(started), err
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return 0, time.Since(started), err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode, time.Since(started), nil
}

func buildRequest(ctx context.Context, t model.NotificationTarget, ev Event) (*http.Request, error) {
	switch t.Type {
	case model.NotificationTypeNtfy:
		return buildNtfyRequest(ctx, t, ev)
	case model.NotificationTypeDiscord:
		return buildDiscordRequest(ctx, t, ev)
	default:
		return buildGenericRequest(ctx, t, ev)
	}
}

// buildNtfyRequest follows ntfy's publish-by-POST contract
// (https://docs.ntfy.sh/publish/): the message is the plain-text body, and
// Title/Priority/Tags ride as headers. Secret, if set, is the topic's access
// token, sent as a bearer credential.
func buildNtfyRequest(ctx context.Context, t model.NotificationTarget, ev Event) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, strings.NewReader(ev.Body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if ev.Title != "" {
		req.Header.Set("Title", ev.Title)
	}
	req.Header.Set("Priority", ntfyPriority(ev.Severity))
	req.Header.Set("Tags", ntfyTag(ev.Severity))
	if err := setBearer(req, t.Secret); err != nil {
		return nil, err
	}
	return req, nil
}

func ntfyPriority(sev string) string {
	switch sev {
	case "critical":
		return "5"
	case "warning":
		return "4"
	default:
		return "3"
	}
}

func ntfyTag(sev string) string {
	switch sev {
	case "critical":
		return "rotating_light"
	case "warning":
		return "warning"
	default:
		return "information_source"
	}
}

type discordPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []discordEmbed `json:"embeds,omitempty"`
}

type discordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color,omitempty"`
	Fields      []discordField `json:"fields,omitempty"`
}

type discordField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// buildDiscordRequest posts Discord's documented webhook-execute body: a
// top-level "content" plus one embed carrying the detail. Secret is unused -
// the webhook URL itself is the credential, and Discord rejects an unknown
// Authorization header on this endpoint.
func buildDiscordRequest(ctx context.Context, t model.NotificationTarget, ev Event) (*http.Request, error) {
	fields := make([]discordField, 0, len(ev.Fields))
	for k, v := range ev.Fields {
		fields = append(fields, discordField{Name: k, Value: v})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

	payload := discordPayload{
		Content: ev.Title,
		Embeds: []discordEmbed{{
			Title:       ev.Title,
			Description: ev.Body,
			Color:       discordColor(ev.Severity),
			Fields:      fields,
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal discord payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "go-proxy-manager-notify")
	return req, nil
}

func discordColor(sev string) int {
	switch sev {
	case "critical":
		return 0xE74C3C
	case "warning":
		return 0xF1C40F
	default:
		return 0x3498DB
	}
}

// genericEvent is the JSON envelope POSTed to a "generic" target.
type genericEvent struct {
	Kind     string            `json:"kind"`
	Title    string            `json:"title,omitempty"`
	Body     string            `json:"body,omitempty"`
	Severity string            `json:"severity"`
	Fields   map[string]string `json:"fields,omitempty"`
	Time     string            `json:"time"` // RFC3339
}

// buildGenericRequest posts a small self-describing JSON event, with the kind
// repeated in the X-GPM-Event header so a receiver can route without parsing
// the body. Secret, if set, is a bearer token.
func buildGenericRequest(ctx context.Context, t model.NotificationTarget, ev Event) (*http.Request, error) {
	payload := genericEvent{
		Kind:     ev.Kind,
		Title:    ev.Title,
		Body:     ev.Body,
		Severity: severityOrDefault(ev.Severity),
		Fields:   ev.Fields,
		Time:     time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "go-proxy-manager-notify")
	req.Header.Set("X-GPM-Event", ev.Kind)
	if err := setBearer(req, t.Secret); err != nil {
		return nil, err
	}
	return req, nil
}

func setBearer(req *http.Request, secret model.Secret) error {
	if secret.IsEmpty() {
		return nil
	}
	v, err := secret.Resolve()
	if err != nil {
		return fmt.Errorf("resolve secret: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+v)
	return nil
}

// record stores the outcome of the most recent send to one target. In-memory
// and per-process, exactly like webhook.Dispatcher.record: an operational
// hint, never config, so it does not survive a restart.
func (n *Notifier) record(t model.NotificationTarget, status int, dur time.Duration, err error) {
	dl := webhook.Delivery{
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
	n.mu.Lock()
	defer n.mu.Unlock()
	n.last[t.Name] = dl
}

// Status reports every configured target with the outcome of its most recent
// send, mirroring webhook.Dispatcher.Status: a target that has never fired
// carries an empty LastAttempt rather than being omitted, and an entry for a
// target that no longer exists is dropped.
func (n *Notifier) Status() []webhook.Delivery {
	if n == nil || n.targets == nil {
		return []webhook.Delivery{}
	}
	targets := n.targets()
	out := make([]webhook.Delivery, 0, len(targets))
	n.mu.Lock()
	defer n.mu.Unlock()
	live := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		live[t.Name] = struct{}{}
		dl := n.last[t.Name]
		dl.Name, dl.URL, dl.Disabled = t.Name, t.URL, t.Disabled
		out = append(out, dl)
	}
	for name := range n.last {
		if _, ok := live[name]; !ok {
			delete(n.last, name)
		}
	}
	return out
}

// Test sends a synthetic event to one target synchronously and waits for the
// result, so an operator can prove a receiver works without waiting for a
// real event. It bypasses Events filtering and the dedup window - that
// filtering only applies to Emit - and, like webhook.Dispatcher.Test, a
// DISABLED target is still tested on purpose.
//
// A transport failure or a non-2xx status is reported in the result, not as
// an error: only an unknown target name is an error (ErrUnknownTarget).
func (n *Notifier) Test(ctx context.Context, name string) (webhook.TestResult, error) {
	if n == nil || n.targets == nil {
		return webhook.TestResult{}, ErrUnknownTarget
	}
	var target *model.NotificationTarget
	for _, t := range n.targets() {
		if t.Name == name {
			t := t
			target = &t
			break
		}
	}
	if target == nil {
		return webhook.TestResult{}, ErrUnknownTarget
	}

	ev := Event{
		Kind:     "test",
		Subject:  name,
		Title:    "go-proxy-manager test notification",
		Body:     fmt.Sprintf("This is a test notification for target %q.", name),
		Severity: "info",
	}
	ctx, cancel := context.WithTimeout(ctx, TestTimeout)
	defer cancel()
	status, dur, err := n.post(ctx, *target, ev)
	n.record(*target, status, dur, err)

	res := webhook.TestResult{
		Name:       target.Name,
		URL:        target.URL,
		Status:     status,
		DurationMS: dur.Milliseconds(),
		OK:         err == nil && status >= 200 && status < 300,
	}
	switch {
	case err != nil:
		res.Error = err.Error()
	case status >= 300:
		res.Error = fmt.Sprintf("receiver answered HTTP %d", status)
	}
	return res, nil
}
