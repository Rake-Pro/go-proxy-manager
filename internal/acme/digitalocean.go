package acme

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// DigitalOceanSolver implements DNSSolver against the DigitalOcean API v2
// (https://api.digitalocean.com/v2), which authenticates with a bearer token.
type DigitalOceanSolver struct {
	api *restAPI

	mu    sync.Mutex
	zones map[string]string // record fqdn -> owning domain name
}

// DigitalOceanOption configures a DigitalOceanSolver.
type DigitalOceanOption func(*DigitalOceanSolver)

// WithDOBaseURL overrides the API base URL (used by tests).
func WithDOBaseURL(u string) DigitalOceanOption {
	return func(s *DigitalOceanSolver) { s.api.baseURL = strings.TrimRight(u, "/") }
}

// WithDOHTTPClient overrides the HTTP client used for API calls.
func WithDOHTTPClient(c *http.Client) DigitalOceanOption {
	return func(s *DigitalOceanSolver) { s.api.client = c }
}

// NewDigitalOceanSolver builds a solver authenticated with a personal access
// token that has write scope on domains.
func NewDigitalOceanSolver(apiToken string, opts ...DigitalOceanOption) (*DigitalOceanSolver, error) {
	if apiToken == "" {
		return nil, errors.New("digitalocean: api token must not be empty")
	}
	s := &DigitalOceanSolver{
		api: newRESTAPI("digitalocean", "https://api.digitalocean.com", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+apiToken)
		}),
		zones: map[string]string{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

type doDomainList struct {
	Domains []struct {
		Name string `json:"name"`
	} `json:"domains"`
	Links struct {
		Pages struct {
			Next string `json:"next"`
		} `json:"pages"`
	} `json:"links"`
}

type doRecordList struct {
	Records []doRecord `json:"domain_records"`
}

type doRecord struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	Data string `json:"data"`
}

// zoneFor resolves the domain that owns name by longest-suffix match over the
// account's domains, and caches it.
func (s *DigitalOceanSolver) zoneFor(ctx context.Context, name string) (string, error) {
	s.mu.Lock()
	zone, ok := s.zones[name]
	s.mu.Unlock()
	if ok {
		return zone, nil
	}

	var names []string
	// Page through the domain list; the API caps per_page at 200.
	path := "/v2/domains?per_page=200"
	for range 20 {
		var list doDomainList
		if err := s.api.do(ctx, http.MethodGet, path, nil, &list); err != nil {
			return "", err
		}
		for _, d := range list.Domains {
			names = append(names, d.Name)
		}
		next := list.Links.Pages.Next
		if next == "" {
			break
		}
		u, err := url.Parse(next)
		if err != nil {
			break
		}
		path = u.RequestURI()
	}

	zone, ok = zoneFromList(name, names)
	if !ok {
		return "", fmt.Errorf("digitalocean: no domain found for %q", name)
	}
	s.mu.Lock()
	s.zones[name] = zone
	s.mu.Unlock()
	return zone, nil
}

// Present adds a TXT record at name with the given value.
func (s *DigitalOceanSolver) Present(ctx context.Context, name, value string) error {
	zone, err := s.zoneFor(ctx, name)
	if err != nil {
		return err
	}
	// An identical record already in place (a retried order) is success.
	existing, err := s.findRecords(ctx, zone, name, value)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	body := map[string]any{
		"type": "TXT",
		"name": relativeName(name, zone),
		"data": value,
		"ttl":  30,
	}
	return s.api.do(ctx, http.MethodPost, "/v2/domains/"+url.PathEscape(zone)+"/records", body, nil)
}

// CleanUp removes the TXT record matching both name and value. It is best-effort.
func (s *DigitalOceanSolver) CleanUp(ctx context.Context, name, value string) error {
	zone, err := s.zoneFor(ctx, name)
	if err != nil {
		return err
	}
	records, err := s.findRecords(ctx, zone, name, value)
	if err != nil {
		return err
	}
	for _, rec := range records {
		path := "/v2/domains/" + url.PathEscape(zone) + "/records/" + strconv.FormatInt(rec.ID, 10)
		if err := s.api.do(ctx, http.MethodDelete, path, nil, nil); err != nil && !statusIs(err, http.StatusNotFound) {
			return err
		}
	}
	return nil
}

// findRecords lists the account's TXT records at name whose data equals value.
func (s *DigitalOceanSolver) findRecords(ctx context.Context, zone, name, value string) ([]doRecord, error) {
	q := url.Values{}
	q.Set("type", "TXT")
	q.Set("name", strings.TrimSuffix(name, "."))
	q.Set("per_page", "200")
	var list doRecordList
	err := s.api.do(ctx, http.MethodGet, "/v2/domains/"+url.PathEscape(zone)+"/records?"+q.Encode(), nil, &list)
	if err != nil {
		if statusIs(err, http.StatusNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var out []doRecord
	rel := relativeName(name, zone)
	for _, rec := range list.Records {
		if rec.Type != "TXT" || rec.Data != value {
			continue
		}
		// The API filter is advisory: match the name ourselves too, accepting the
		// relative or absolute form.
		recName := strings.TrimSuffix(rec.Name, ".")
		if recName != rel && recName != strings.TrimSuffix(name, ".") {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}
