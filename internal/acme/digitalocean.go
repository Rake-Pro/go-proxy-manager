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

// doMaxPages bounds how many pages the domain listing follows, so a server that
// keeps offering a next page cannot hold a challenge in a loop.
const doMaxPages = 20

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

	base, err := url.Parse(s.api.baseURL)
	if err != nil {
		return "", fmt.Errorf("digitalocean: api base url: %w", err)
	}
	// restAPI.do puts the base URL (path included) in front of every path,
	// so a next link must lie under the base path, which is then dropped.
	basePath := base.EscapedPath()

	var names []string
	// Page through the domain list; the API caps per_page at 200.
	path := "/v2/domains?per_page=200"
	for page := 1; ; page++ {
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
		if page >= doMaxPages {
			return "", fmt.Errorf("digitalocean: domain listing still has a next page after %d requests; refusing to use a partial listing", doMaxPages)
		}
		// A relative link resolves against the page that carried it. Only the
		// path and query are used, so the request (and the token with it) stays
		// on the configured API base; a link that leaves it is refused rather
		// than followed or dropped, since a partial listing can miss the zone.
		// The link comes from the server, so errors quote it cut short.
		cur, err := url.Parse(s.api.baseURL + path)
		if err != nil {
			return "", fmt.Errorf("digitalocean: domain listing: %w", err)
		}
		u, err := cur.Parse(next)
		switch {
		case err != nil:
			return "", fmt.Errorf("digitalocean: domain listing next page link %s does not parse as a URL; refusing to use a partial listing", quoteCut(next))
		case u.Opaque != "":
			return "", fmt.Errorf("digitalocean: domain listing next page link %s is opaque, not a URL on the API; refusing to use a partial listing", quoteCut(next))
		case u.Scheme != cur.Scheme:
			return "", fmt.Errorf("digitalocean: domain listing next page link %s uses scheme %s, not the API's %q; the API token is only sent to the configured API, refusing to use a partial listing", quoteCut(next), quoteCut(u.Scheme), cur.Scheme)
		case !strings.EqualFold(u.Host, cur.Host):
			return "", fmt.Errorf("digitalocean: domain listing next page link %s points at host %s, not the API host %q; the API token is only sent to the configured API, refusing to use a partial listing", quoteCut(next), quoteCut(u.Host), cur.Host)
		case hasDotSegment(u.Path):
			return "", fmt.Errorf("digitalocean: domain listing next page link %s has a \".\" or \"..\" path segment, which a server may resolve outside the API path; the API token is only sent to the configured API, refusing to use a partial listing", quoteCut(next))
		case basePath != "" && !strings.HasPrefix(u.EscapedPath(), basePath+"/"):
			return "", fmt.Errorf("digitalocean: domain listing next page link %s is outside the API base path %q; the API token is only sent to the configured API, refusing to use a partial listing", quoteCut(next), basePath)
		}
		path = strings.TrimPrefix(u.EscapedPath(), basePath)
		if u.RawQuery != "" {
			path += "?" + u.RawQuery
		}
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

// hasDotSegment reports whether the decoded path p has a "." or ".." segment.
// Resolving a link removes literal ones; an escaped one (%2e%2e) is left in
// the path and sent as it is.
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

// quoteCut returns s quoted with %q, cut by cutForError with the cut marked
// after the closing quote.
func quoteCut(s string) string {
	c, mark := cutForError(s)
	return strconv.Quote(c) + mark
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
