package acme

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// WebhookDNSConfig configures a WebhookDNSSolver.
type WebhookDNSConfig struct {
	URL   string // the endpoint every request is POSTed to
	Token string // sent as "Authorization: Bearer <token>"
	// AllowInsecureLocal permits a URL whose host is a loopback literal, for an
	// endpoint running beside gpm (same meaning as for acme-dns).
	AllowInsecureLocal bool
}

// WebhookDNSSolver implements DNSSolver by calling an operator's own HTTP
// endpoint, for a DNS setup no built-in provider covers. For both Present and
// CleanUp it sends
//
//	POST <url>
//	Authorization: Bearer <token>
//	Content-Type: application/json
//
//	{"action":"present","fqdn":"_acme-challenge.example.com.","value":"<TXT value>"}
//
// with "action" either "present" or "cleanup" and "fqdn" always ending in a
// dot. Any 2xx answer is success; anything else fails the call with the status
// and the start of the body. Redirects are not followed, so the token never
// reaches a host other than the configured one.
//
// The endpoint is called once per record, and the records of one order can
// share a name: a certificate for example.com and *.example.com presents two
// different values at _acme-challenge.example.com. The endpoint must keep both
// (add a TXT value on present, remove only that value on cleanup), not replace
// one with the other.
type WebhookDNSSolver struct {
	api  *restAPI
	path string
}

// webhookDNSRequest is the body of every webhook call.
type webhookDNSRequest struct {
	Action string `json:"action"`
	FQDN   string `json:"fqdn"`
	Value  string `json:"value"`
}

// NewWebhookDNSSolver validates cfg and builds a solver. It performs no
// network I/O.
func NewWebhookDNSSolver(cfg WebhookDNSConfig) (*WebhookDNSSolver, error) {
	raw := strings.TrimSpace(cfg.URL)
	if raw == "" {
		return nil, errors.New("webhook: url is required")
	}
	// The value may come from a placeholder and hold a credential, so no error
	// or log line below contains it; the warning names the host only.
	if err := model.ValidateResolvedOutboundURL("webhook: url", raw, cfg.AllowInsecureLocal); err != nil {
		return nil, err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("webhook: url: is not an http or https URL (the value is not shown, it may hold a credential)")
	}
	if u.User != nil {
		return nil, errors.New("webhook: url: must not contain user info (user:password@); it is never sent, put the credential in token")
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		log.Warn().Str("host", u.Host).Msg("webhook dns: url is plain http; the bearer token is sent unencrypted, use an https URL")
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, errors.New("webhook: token is required")
	}
	// restAPI joins a base URL, a path and a query; splitting the configured
	// URL into origin, escaped path and query sends its path (a trailing slash
	// included) and query unchanged, while errors name the path only: the query
	// may be a credential. A URL with no path is sent to "/", and a fragment is
	// never sent.
	base := (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	api := newRESTAPI("webhook", base, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	api.query = u.RawQuery
	api.hidePath = true
	return &WebhookDNSSolver{api: api, path: path}, nil
}

// Present asks the endpoint to add the TXT value at name.
func (s *WebhookDNSSolver) Present(ctx context.Context, name, value string) error {
	return s.call(ctx, "present", name, value)
}

// CleanUp asks the endpoint to remove the TXT value at name.
func (s *WebhookDNSSolver) CleanUp(ctx context.Context, name, value string) error {
	return s.call(ctx, "cleanup", name, value)
}

func (s *WebhookDNSSolver) call(ctx context.Context, action, name, value string) error {
	fqdn := name
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}
	if err := s.api.do(ctx, http.MethodPost, s.path, webhookDNSRequest{Action: action, FQDN: fqdn, Value: value}, nil); err != nil {
		return fmt.Errorf("%s %s: %w", action, fqdn, err)
	}
	return nil
}
