package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// CloudflareSolver implements DNSSolver against the Cloudflare API v4, managing
// the TXT records that satisfy DNS-01 challenges.
type CloudflareSolver struct {
	apiToken   string
	baseURL    string
	httpClient *http.Client

	mu      sync.Mutex
	zoneIDs map[string]string // zone name -> zone id
}

// CloudflareOption configures a CloudflareSolver.
type CloudflareOption func(*CloudflareSolver)

// WithBaseURL overrides the Cloudflare API base URL (used by tests). The trailing
// slash, if any, is stripped.
func WithBaseURL(u string) CloudflareOption {
	return func(s *CloudflareSolver) {
		s.baseURL = strings.TrimRight(u, "/")
	}
}

// WithHTTPClient overrides the HTTP client used for API calls.
func WithHTTPClient(c *http.Client) CloudflareOption {
	return func(s *CloudflareSolver) {
		s.httpClient = c
	}
}

// NewCloudflareSolver builds a solver authenticated with the given API token.
func NewCloudflareSolver(apiToken string, opts ...CloudflareOption) (*CloudflareSolver, error) {
	if apiToken == "" {
		return nil, errors.New("cloudflare: api token must not be empty")
	}
	s := &CloudflareSolver{
		apiToken:   apiToken,
		baseURL:    "https://api.cloudflare.com/client/v4",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		zoneIDs:    map[string]string{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// cfEnvelope is the standard Cloudflare API v4 response envelope.
type cfEnvelope struct {
	Success  bool            `json:"success"`
	Errors   []cfError       `json:"errors"`
	Messages []cfMessage     `json:"messages"`
	Result   json.RawMessage `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfDNSRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

// do executes an API request and decodes the envelope, returning an error if the
// envelope reports failure. The decoded result is left in env.Result for callers.
func (s *CloudflareSolver) do(ctx context.Context, method, urlStr string, body any) (*cfEnvelope, error) {
	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("cloudflare: marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, reqBody)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: %s %s: %w", method, urlStr, err)
	}
	defer resp.Body.Close()

	var env cfEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("cloudflare: decode response (%s): %w", resp.Status, err)
	}
	if !env.Success {
		return &env, fmt.Errorf("cloudflare: %s %s failed: %s", method, urlStr, env.errString())
	}
	return &env, nil
}

func (e *cfEnvelope) errString() string {
	if len(e.Errors) == 0 {
		return "unknown error"
	}
	parts := make([]string, 0, len(e.Errors))
	for _, er := range e.Errors {
		parts = append(parts, fmt.Sprintf("[%d] %s", er.Code, er.Message))
	}
	return strings.Join(parts, "; ")
}

// zoneID resolves and caches the Cloudflare zone ID owning the given record name.
func (s *CloudflareSolver) zoneID(ctx context.Context, name string) (string, error) {
	candidate := strings.TrimPrefix(name, "_acme-challenge.")
	candidate = strings.TrimSuffix(candidate, ".")

	for candidate != "" {
		s.mu.Lock()
		id, ok := s.zoneIDs[candidate]
		s.mu.Unlock()
		if ok {
			return id, nil
		}

		q := url.Values{}
		q.Set("name", candidate)
		env, err := s.do(ctx, http.MethodGet, s.baseURL+"/zones?"+q.Encode(), nil)
		if err != nil {
			return "", err
		}
		var zones []cfZone
		if err := json.Unmarshal(env.Result, &zones); err != nil {
			return "", fmt.Errorf("cloudflare: decode zones: %w", err)
		}
		if len(zones) > 0 {
			id := zones[0].ID
			s.mu.Lock()
			s.zoneIDs[candidate] = id
			s.mu.Unlock()
			return id, nil
		}

		// Strip the most-specific label and try the parent domain.
		idx := strings.Index(candidate, ".")
		if idx < 0 {
			break
		}
		candidate = candidate[idx+1:]
	}
	return "", fmt.Errorf("cloudflare: no zone found for %q", name)
}

// Present adds a TXT record at name with the given value.
func (s *CloudflareSolver) Present(ctx context.Context, name, value string) error {
	zoneID, err := s.zoneID(ctx, name)
	if err != nil {
		return err
	}
	body := map[string]any{
		"type":    "TXT",
		"name":    name,
		"content": value,
		"ttl":     120,
	}
	_, err = s.do(ctx, http.MethodPost, s.baseURL+"/zones/"+zoneID+"/dns_records", body)
	if err != nil {
		if isAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}

// isAlreadyExists reports whether err is Cloudflare signaling a duplicate record.
func isAlreadyExists(err error) bool {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "already exists") || strings.Contains(msg, "identical") {
		return true
	}
	// Cloudflare duplicate-record error codes.
	return strings.Contains(msg, "81057") || strings.Contains(msg, "81058")
}

// CleanUp removes the TXT record matching both name and value. It is best-effort.
func (s *CloudflareSolver) CleanUp(ctx context.Context, name, value string) error {
	zoneID, err := s.zoneID(ctx, name)
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("type", "TXT")
	q.Set("name", name)
	q.Set("content", value)
	env, err := s.do(ctx, http.MethodGet, s.baseURL+"/zones/"+zoneID+"/dns_records?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	var records []cfDNSRecord
	if err := json.Unmarshal(env.Result, &records); err != nil {
		return fmt.Errorf("cloudflare: decode dns records: %w", err)
	}
	if len(records) == 0 {
		return nil
	}

	for _, rec := range records {
		if rec.ID == "" {
			continue
		}
		if _, err := s.do(ctx, http.MethodDelete, s.baseURL+"/zones/"+zoneID+"/dns_records/"+rec.ID, nil); err != nil {
			if isNotFound(err) {
				continue
			}
			return err
		}
	}
	return nil
}

// isNotFound reports whether err indicates the record was already gone.
func isNotFound(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "404") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "81044") // record does not exist
}
