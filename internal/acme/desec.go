package acme

import (
	"context"
	"errors"
	"fmt"
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

	var domains []desecDomain
	if err := s.api.do(ctx, http.MethodGet, "/api/v1/domains/?limit=500", nil, &domains); err != nil {
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
