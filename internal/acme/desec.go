package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// desecTTL is the TTL used for challenge RRsets. deSEC enforces a minimum TTL
// per account (3600 by default), so a shorter value would be rejected.
const desecTTL = 3600

// DesecSolver implements DNSSolver against the deSEC API
// (https://desec.io/api/v1), which authenticates with an "Authorization: Token"
// header and models DNS as RRsets: a TXT name holds a set of values, so Present
// and CleanUp read-modify-write that set instead of adding/removing one record.
type DesecSolver struct {
	api *restAPI

	mu    sync.Mutex
	zones map[string]string // record fqdn -> owning domain name
}

// DesecOption configures a DesecSolver.
type DesecOption func(*DesecSolver)

// WithDesecBaseURL overrides the API base URL (used by tests).
func WithDesecBaseURL(u string) DesecOption {
	return func(s *DesecSolver) { s.api.baseURL = strings.TrimRight(u, "/") }
}

// WithDesecHTTPClient overrides the HTTP client used for API calls.
func WithDesecHTTPClient(c *http.Client) DesecOption {
	return func(s *DesecSolver) { s.api.client = c }
}

// NewDesecSolver builds a solver authenticated with a deSEC API token.
func NewDesecSolver(apiToken string, opts ...DesecOption) (*DesecSolver, error) {
	if apiToken == "" {
		return nil, errors.New("desec: api token must not be empty")
	}
	s := &DesecSolver{
		api: newRESTAPI("desec", "https://desec.io", func(r *http.Request) {
			r.Header.Set("Authorization", "Token "+apiToken)
		}),
		zones: map[string]string{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

type desecDomain struct {
	Name string `json:"name"`
}

type desecRRset struct {
	Subname string   `json:"subname"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	Records []string `json:"records"`
}

// zoneFor resolves the deSEC domain owning name by longest-suffix match.
func (s *DesecSolver) zoneFor(ctx context.Context, name string) (string, error) {
	s.mu.Lock()
	zone, ok := s.zones[name]
	s.mu.Unlock()
	if ok {
		return zone, nil
	}

	domains, err := s.listDomains(ctx)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(domains))
	for _, d := range domains {
		names = append(names, d.Name)
	}
	zone, ok = zoneFromList(name, names)
	if !ok {
		return "", fmt.Errorf("desec: no domain found for %q", name)
	}
	s.mu.Lock()
	s.zones[name] = zone
	s.mu.Unlock()
	return zone, nil
}

// desecMaxPages bounds how many pages the domain listing follows, so a server
// that keeps sending a next link cannot hold a challenge in a loop.
const desecMaxPages = 100

// listDomains returns every domain on the account. deSEC returns up to 500
// items per response; an account with more answers the unpaginated request with
// 400 and a Link rel="first", and each page then names the following one in a
// Link rel="next" until the last. An account that fits in one response is read
// with exactly the single request it always was.
func (s *DesecSolver) listDomains(ctx context.Context) ([]desecDomain, error) {
	path := "/api/v1/domains/?limit=500"
	var all []desecDomain
	for page := 1; ; page++ {
		var domains []desecDomain
		hdr, err := s.get(ctx, path, &domains)
		if err != nil {
			first := linkRel(hdr, "first")
			if page == 1 && statusIs(err, http.StatusBadRequest) && first != "" {
				if path, err = sameAPIPath(first); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		all = append(all, domains...)
		next := linkRel(hdr, "next")
		if next == "" {
			return all, nil
		}
		if page >= desecMaxPages {
			return nil, fmt.Errorf("desec: domain listing still has a next page after %d requests; refusing to use a partial listing", desecMaxPages)
		}
		if path, err = sameAPIPath(next); err != nil {
			return nil, err
		}
	}
}

// get is restAPI.do for a GET whose response headers are needed (the Link
// header carries deSEC's pagination). The header is returned for a non-2xx
// answer too, since the 400 that asks for pagination carries the first link.
func (s *DesecSolver) get(ctx context.Context, path string, out any) (http.Header, error) {
	a := s.api
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", a.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	a.authorize(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: GET %s: %w", a.name, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: read response (%s): %w", a.name, resp.Status, err)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, fmt.Errorf("%s: GET %s: server answered %d redirecting to another location; redirects are not followed because the request carries this provider's credentials", a.name, path, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Header, &apiError{provider: a.name, method: http.MethodGet, path: path, status: resp.StatusCode, body: snippet(raw)}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return resp.Header, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("%s: decode GET %s response: %w", a.name, path, err)
	}
	return resp.Header, nil
}

// linkRel returns the target of the RFC 8288 Link header entry with the given
// rel, or "" when there is none.
func linkRel(h http.Header, rel string) string {
	for _, v := range h.Values("Link") {
		for _, entry := range strings.Split(v, ",") {
			target, params, _ := strings.Cut(strings.TrimSpace(entry), ";")
			target = strings.TrimSpace(target)
			if len(target) < 2 || target[0] != '<' || target[len(target)-1] != '>' {
				continue
			}
			for _, p := range strings.Split(params, ";") {
				k, val, _ := strings.Cut(strings.TrimSpace(p), "=")
				if strings.EqualFold(strings.TrimSpace(k), "rel") && strings.Trim(strings.TrimSpace(val), `"`) == rel {
					return target[1 : len(target)-1]
				}
			}
		}
	}
	return ""
}

// sameAPIPath keeps only the path and query of a pagination link, so the next
// request (which carries the API token) stays on the configured API base
// whatever host the link names.
func sameAPIPath(link string) (string, error) {
	u, err := url.Parse(link)
	if err != nil || u.Opaque != "" || !strings.HasPrefix(u.Path, "/") {
		return "", fmt.Errorf("desec: unusable pagination link %q", link)
	}
	p := u.EscapedPath()
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p, nil
}

// desecSubname is the RRset subname for fqdn within zone; "@" addresses the apex
// in the URL path.
func desecSubname(fqdn, zone string) string {
	rel := relativeName(fqdn, zone)
	if rel == "" {
		return "@"
	}
	return rel
}

func desecQuote(v string) string { return `"` + v + `"` }

// rrsetPath is the RRset endpoint for a TXT name.
func (s *DesecSolver) rrsetPath(zone, subname string) string {
	return "/api/v1/domains/" + url.PathEscape(zone) + "/rrsets/" + url.PathEscape(subname) + "/TXT/"
}

// currentRecords fetches the TXT values already at subname (nil when the RRset
// does not exist).
func (s *DesecSolver) currentRecords(ctx context.Context, zone, subname string) ([]string, error) {
	var rr desecRRset
	err := s.api.do(ctx, http.MethodGet, s.rrsetPath(zone, subname), nil, &rr)
	if err != nil {
		if statusIs(err, http.StatusNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rr.Records, nil
}

// Present adds value to the TXT RRset at name, preserving any values already
// there (apex + wildcard orders share one name).
func (s *DesecSolver) Present(ctx context.Context, name, value string) error {
	zone, err := s.zoneFor(ctx, name)
	if err != nil {
		return err
	}
	subname := desecSubname(name, zone)
	existing, err := s.currentRecords(ctx, zone, subname)
	if err != nil {
		return err
	}
	quoted := desecQuote(value)
	for _, r := range existing {
		if r == quoted {
			return nil // already present (retried order)
		}
	}
	records := append(append([]string{}, existing...), quoted)
	if len(existing) == 0 {
		body := desecRRset{Subname: relativeName(name, zone), Type: "TXT", TTL: desecTTL, Records: records}
		return s.api.do(ctx, http.MethodPost, "/api/v1/domains/"+url.PathEscape(zone)+"/rrsets/", body, nil)
	}
	body := map[string]any{"ttl": desecTTL, "records": records}
	return s.api.do(ctx, http.MethodPatch, s.rrsetPath(zone, subname), body, nil)
}

// CleanUp removes value from the TXT RRset at name; an empty record list deletes
// the RRset. It is best-effort.
func (s *DesecSolver) CleanUp(ctx context.Context, name, value string) error {
	zone, err := s.zoneFor(ctx, name)
	if err != nil {
		return err
	}
	subname := desecSubname(name, zone)
	existing, err := s.currentRecords(ctx, zone, subname)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}
	quoted := desecQuote(value)
	remaining := make([]string, 0, len(existing))
	for _, r := range existing {
		if r != quoted {
			remaining = append(remaining, r)
		}
	}
	if len(remaining) == len(existing) {
		return nil // value was not there
	}
	body := map[string]any{"ttl": desecTTL, "records": remaining}
	if err := s.api.do(ctx, http.MethodPatch, s.rrsetPath(zone, subname), body, nil); err != nil {
		if statusIs(err, http.StatusNotFound) {
			return nil
		}
		return err
	}
	return nil
}
