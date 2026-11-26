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

// HetznerSolver implements DNSSolver against the Hetzner DNS API v1
// (https://dns.hetzner.com/api/v1), which authenticates with the Auth-API-Token
// header rather than a bearer token.
type HetznerSolver struct {
	api *restAPI

	mu    sync.Mutex
	zones map[string]hetznerZone // record fqdn -> owning zone
}

type hetznerZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// HetznerOption configures a HetznerSolver.
type HetznerOption func(*HetznerSolver)

// WithHetznerBaseURL overrides the API base URL (used by tests).
func WithHetznerBaseURL(u string) HetznerOption {
	return func(s *HetznerSolver) { s.api.baseURL = strings.TrimRight(u, "/") }
}

// WithHetznerHTTPClient overrides the HTTP client used for API calls.
func WithHetznerHTTPClient(c *http.Client) HetznerOption {
	return func(s *HetznerSolver) { s.api.client = c }
}

// NewHetznerSolver builds a solver authenticated with a Hetzner DNS API token.
func NewHetznerSolver(apiToken string, opts ...HetznerOption) (*HetznerSolver, error) {
	if apiToken == "" {
		return nil, errors.New("hetzner: api token must not be empty")
	}
	s := &HetznerSolver{
		api: newRESTAPI("hetzner", "https://dns.hetzner.com", func(r *http.Request) {
			r.Header.Set("Auth-API-Token", apiToken)
		}),
		zones: map[string]hetznerZone{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// hetznerMaxPages bounds how many pages one listing follows, so a server that
// keeps reporting more pages cannot hold a challenge in a loop.
const hetznerMaxPages = 100

// hetznerPage is one page of a zone or record listing; meta.pagination.last_page
// is the total page count, absent or 0 on an unpaginated response.
type hetznerPage struct {
	Zones   []hetznerZone   `json:"zones"`
	Records []hetznerRecord `json:"records"`
	Meta    struct {
		Pagination struct {
			LastPage int `json:"last_page"`
		} `json:"pagination"`
	} `json:"meta"`
}

type hetznerRecord struct {
	ID     string `json:"id"`
	ZoneID string `json:"zone_id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

// zoneFor resolves the zone owning name by longest-suffix match, and caches it.
func (s *HetznerSolver) zoneFor(ctx context.Context, name string) (hetznerZone, error) {
	s.mu.Lock()
	z, ok := s.zones[name]
	s.mu.Unlock()
	if ok {
		return z, nil
	}

	list, err := s.listAll(ctx, "/api/v1/zones", url.Values{})
	if err != nil {
		return hetznerZone{}, err
	}
	names := make([]string, 0, len(list.Zones))
	for _, zone := range list.Zones {
		names = append(names, zone.Name)
	}
	match, ok := zoneFromList(name, names)
	if !ok {
		return hetznerZone{}, fmt.Errorf("hetzner: no zone found for %q", name)
	}
	for _, zone := range list.Zones {
		if strings.TrimSuffix(zone.Name, ".") == match {
			s.mu.Lock()
			s.zones[name] = zone
			s.mu.Unlock()
			return zone, nil
		}
	}
	return hetznerZone{}, fmt.Errorf("hetzner: no zone found for %q", name)
}

// hetznerRecordName is the record name relative to its zone; the apex is "@".
func hetznerRecordName(fqdn, zone string) string {
	rel := relativeName(fqdn, zone)
	if rel == "" {
		return "@"
	}
	return rel
}

// Present adds a TXT record at name with the given value.
func (s *HetznerSolver) Present(ctx context.Context, name, value string) error {
	zone, err := s.zoneFor(ctx, name)
	if err != nil {
		return err
	}
	existing, err := s.findRecords(ctx, zone, name, value)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil // identical record already in place (retried order)
	}
	body := map[string]any{
		"zone_id": zone.ID,
		"type":    "TXT",
		"name":    hetznerRecordName(name, zone.Name),
		"value":   value,
		"ttl":     60,
	}
	return s.api.do(ctx, http.MethodPost, "/api/v1/records", body, nil)
}

// CleanUp removes the TXT record matching both name and value. It is best-effort.
func (s *HetznerSolver) CleanUp(ctx context.Context, name, value string) error {
	zone, err := s.zoneFor(ctx, name)
	if err != nil {
		return err
	}
	records, err := s.findRecords(ctx, zone, name, value)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if err := s.api.do(ctx, http.MethodDelete, "/api/v1/records/"+url.PathEscape(rec.ID), nil, nil); err != nil {
			if statusIs(err, http.StatusNotFound) {
				continue
			}
			return err
		}
	}
	return nil
}

// findRecords returns the zone's TXT records at name holding value.
func (s *HetznerSolver) findRecords(ctx context.Context, zone hetznerZone, name, value string) ([]hetznerRecord, error) {
	q := url.Values{}
	q.Set("zone_id", zone.ID)
	q.Set("per_page", "500")
	list, err := s.listAll(ctx, "/api/v1/records", q)
	if err != nil {
		if statusIs(err, http.StatusNotFound) {
			return nil, nil
		}
		return nil, err
	}
	want := hetznerRecordName(name, zone.Name)
	var out []hetznerRecord
	for _, rec := range list.Records {
		if rec.Type != "TXT" || rec.Name != want {
			continue
		}
		// Hetzner returns TXT values verbatim, but a quoted round-trip is possible.
		if strings.Trim(rec.Value, `"`) != value {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// listAll fetches every page of a zone or record listing. The first request is
// sent without a page parameter, as an unpaginated listing always was; later
// pages add page=N until meta.pagination.last_page is reached. A 404 on the
// first page is returned as is (callers treat it as "nothing there"); on a
// later page it is an ordinary error, since a partial listing must not pass for
// a complete one.
func (s *HetznerSolver) listAll(ctx context.Context, path string, q url.Values) (hetznerPage, error) {
	var all hetznerPage
	for page := 1; ; page++ {
		if page > 1 {
			q.Set("page", strconv.Itoa(page))
		}
		p := path
		if len(q) > 0 {
			p += "?" + q.Encode()
		}
		var list hetznerPage
		if err := s.api.do(ctx, http.MethodGet, p, nil, &list); err != nil {
			if page > 1 {
				return hetznerPage{}, fmt.Errorf("hetzner: %s page %d of a paginated listing: %v", path, page, err)
			}
			return hetznerPage{}, err
		}
		all.Zones = append(all.Zones, list.Zones...)
		all.Records = append(all.Records, list.Records...)
		last := list.Meta.Pagination.LastPage
		if last <= page {
			return all, nil
		}
		if page == hetznerMaxPages {
			return hetznerPage{}, fmt.Errorf("hetzner: %s reports %d pages, more than the %d this client follows; refusing to use a partial listing", path, last, hetznerMaxPages)
		}
	}
}
