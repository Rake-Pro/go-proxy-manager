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
	"net/http"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// Event is the JSON payload POSTed to each webhook target.
type Event struct {
	Action string `json:"action"` // save | delete | restore | revert | settings
	Kind   string `json:"kind,omitempty"`
	Name   string `json:"name,omitempty"`
	Commit string `json:"commit,omitempty"`
	Time   string `json:"time"` // RFC3339
}

// Dispatcher fans an Event out to the currently-configured webhook targets.
type Dispatcher struct {
	client  *http.Client
	targets func() []model.WebhookConfig // reads the live settings on each dispatch
}

// New returns a Dispatcher. targets is called on every Dispatch so configuration
// changes take effect without re-wiring; a nil targets disables delivery.
func New(targets func() []model.WebhookConfig) *Dispatcher {
	return &Dispatcher{
		client:  &http.Client{Timeout: 10 * time.Second},
		targets: targets,
	}
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
		go d.post(t, body)
	}
}

func (d *Dispatcher) post(t model.WebhookConfig, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		log.Warn().Err(err).Str("webhook", t.Name).Msg("webhook: build request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "go-proxy-manager-webhook")
	if !t.Secret.IsEmpty() {
		secret, err := t.Secret.Resolve()
		if err != nil {
			log.Warn().Err(err).Str("webhook", t.Name).Msg("webhook: resolve secret")
			return
		}
		req.Header.Set("X-GPM-Webhook-Secret", secret)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		log.Warn().Err(err).Str("webhook", t.Name).Str("url", t.URL).Msg("webhook: delivery failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Warn().Str("webhook", t.Name).Int("status", resp.StatusCode).Msg("webhook: non-2xx response")
		return
	}
	log.Debug().Str("webhook", t.Name).Int("status", resp.StatusCode).Msg("webhook delivered")
}
