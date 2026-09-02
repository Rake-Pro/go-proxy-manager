package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/webhook"
)

// restAPI is the shared plumbing behind the token-authenticated REST DNS
// providers (DigitalOcean, Hetzner, deSEC). Cloudflare keeps its own client
// because its API wraps every response in a success/errors envelope.
type restAPI struct {
	name    string // provider name, used in error messages
	baseURL string
	client  *http.Client
	// authorize stamps the provider's auth header onto each request.
	authorize func(*http.Request)
}

// newRESTAPI builds the shared client. It uses webhook.NewSecureClient - the
// same hardened client every other admin-configured outbound integration uses -
// for two reasons that both bite here:
//
//   - Redirects are never followed. Go's stdlib strips only Authorization,
//     Www-Authenticate, Cookie and Cookie2 across a host change; a provider's
//     custom credential headers (acme-dns sends X-Api-User / X-Api-Key) would be
//     replayed verbatim to whatever host a 302 named. A 3xx is now surfaced as
//     the response instead.
//   - Link-local destinations are refused at connect time, post-DNS, so a
//     provider baseURL (or a rebinding resolver) cannot aim gpm's own network
//     position at a cloud metadata service.
func newRESTAPI(name, baseURL string, authorize func(*http.Request)) *restAPI {
	return &restAPI{
		name:      name,
		baseURL:   strings.TrimRight(baseURL, "/"),
		client:    webhook.NewSecureClient(30 * time.Second),
		authorize: authorize,
	}
}

// do issues a JSON request and decodes a 2xx response body into out (skipped
// when out is nil or the body is empty). A non-2xx status becomes an error
// carrying the status and a bounded slice of the body, so callers can classify
// "already exists" / "not found" the same way the Cloudflare solver does.
func (a *restAPI) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: marshal request body: %w", a.name, err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", a.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if a.authorize != nil {
		a.authorize(req)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %s %s: %w", a.name, method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%s: read response (%s): %w", a.name, resp.Status, err)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// Not followed on purpose (see newRESTAPI): the redirect target is not
		// the endpoint the operator configured, and the request carries this
		// provider's credentials.
		return fmt.Errorf("%s: %s %s: server answered %d redirecting to another location; redirects are not followed because the request carries this provider's credentials", a.name, method, path, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{provider: a.name, method: method, path: path, status: resp.StatusCode, body: snippet(raw)}
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: decode %s %s response: %w", a.name, method, path, err)
	}
	return nil
}

// apiError is a non-2xx provider response.
type apiError struct {
	provider string
	method   string
	path     string
	status   int
	body     string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s %s: %d: %s", e.provider, e.method, e.path, e.status, e.body)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

// statusIs reports whether err is an apiError with the given status code.
func statusIs(err error, code int) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.status == code
}

// zoneFromList picks the zone whose name is the longest suffix of fqdn, so
// "_acme-challenge.app.example.com" resolves to "example.com" and, when the
// account also holds it, to the more specific "app.example.com".
func zoneFromList(fqdn string, zones []string) (string, bool) {
	name := strings.TrimSuffix(fqdn, ".")
	best := ""
	for _, z := range zones {
		z = strings.TrimSuffix(z, ".")
		if z == "" {
			continue
		}
		if name != z && !strings.HasSuffix(name, "."+z) {
			continue
		}
		if len(z) > len(best) {
			best = z
		}
	}
	return best, best != ""
}

// relativeName returns fqdn expressed relative to zone: "" for the zone apex.
func relativeName(fqdn, zone string) string {
	name := strings.TrimSuffix(fqdn, ".")
	zone = strings.TrimSuffix(zone, ".")
	if name == zone {
		return ""
	}
	return strings.TrimSuffix(name, "."+zone)
}
