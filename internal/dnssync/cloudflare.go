package dnssync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// cfBaseURL is the Cloudflare API v4 root. It is a package variable so the
// hermetic tests can point the client at an httptest server; production code
// never changes it.
var cfBaseURL = "https://api.cloudflare.com/client/v4"

// cloudflareClient manages CNAME records in one zone. It deliberately does not
// reuse the ACME package's CloudflareSolver: that type exists to add and remove
// TXT challenge records within an issuance flow, and entangling record-lifecycle
// management with certificate issuance would make either change risk the other.
// The request/envelope shape is mirrored, not shared.
type cloudflareClient struct {
	token  string
	base   string
	client *http.Client

	zoneID string // resolved once per reconcile and cached for the run
}

type cfEnvelope struct {
	Success bool            `json:"success"`
	Errors  []cfError       `json:"errors"`
	Result  json.RawMessage `json:"result"`
	Info    cfResultInfo    `json:"result_info"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfResultInfo struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

func (e cfEnvelope) errString() string {
	if len(e.Errors) == 0 {
		return "unknown error"
	}
	parts := make([]string, 0, len(e.Errors))
	for _, er := range e.Errors {
		parts = append(parts, fmt.Sprintf("[%d] %s", er.Code, er.Message))
	}
	return strings.Join(parts, "; ")
}

// cfRecord is a DNS record as the reconciler cares about it. Comment is the
// ownership marker: a record whose comment is not ManagedComment is read-only
// as far as gpm is concerned.
type cfRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Comment string `json:"comment"`
}

func (c *cloudflareClient) do(ctx context.Context, method, urlStr string, body any) (*cfEnvelope, error) {
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, rdr)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: %s: %w", method, err)
	}
	defer resp.Body.Close()
	var env cfEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRespBody)).Decode(&env); err != nil {
		return nil, fmt.Errorf("cloudflare: decode response (%s): %w", resp.Status, err)
	}
	if !env.Success {
		return &env, fmt.Errorf("cloudflare: %s failed: %s", method, env.errString())
	}
	return &env, nil
}

// zone resolves the zone ID for the configured zone name, caching it for the run.
func (c *cloudflareClient) zone(ctx context.Context, name string) (string, error) {
	if c.zoneID != "" {
		return c.zoneID, nil
	}
	q := url.Values{}
	q.Set("name", name)
	env, err := c.do(ctx, http.MethodGet, c.base+"/zones?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	var zones []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(env.Result, &zones); err != nil {
		return "", fmt.Errorf("cloudflare: decode zones: %w", err)
	}
	if len(zones) == 0 {
		return "", fmt.Errorf("cloudflare: no zone found for %q", name)
	}
	c.zoneID = zones[0].ID
	return c.zoneID, nil
}

// listCNAMEs returns every CNAME in the zone, following pagination. The whole
// zone is read (not just gpm's records) because reconcile has to know that a
// desired name is already taken by a record it does not own.
func (c *cloudflareClient) listCNAMEs(ctx context.Context, zoneID string) ([]cfRecord, error) {
	var out []cfRecord
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("type", "CNAME")
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		env, err := c.do(ctx, http.MethodGet, c.base+"/zones/"+zoneID+"/dns_records?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var recs []cfRecord
		if err := json.Unmarshal(env.Result, &recs); err != nil {
			return nil, fmt.Errorf("cloudflare: decode dns records: %w", err)
		}
		out = append(out, recs...)
		if len(recs) == 0 || env.Info.TotalPages <= page {
			return out, nil
		}
	}
}

func (c *cloudflareClient) createCNAME(ctx context.Context, zoneID, name, content string, proxied bool) error {
	_, err := c.do(ctx, http.MethodPost, c.base+"/zones/"+zoneID+"/dns_records", map[string]any{
		"type":    "CNAME",
		"name":    name,
		"content": content,
		"proxied": proxied,
		"comment": ManagedComment,
		"ttl":     1, // 1 = automatic
	})
	return err
}

// deleteRecord removes a record by ID, but only after re-checking the ownership
// comment on the record it was handed. The guard is deliberately duplicated here
// (the caller already filtered) so no future caller can turn this into an
// arbitrary-delete primitive against the operator's zone.
func (c *cloudflareClient) deleteRecord(ctx context.Context, zoneID string, rec cfRecord) error {
	if rec.Comment != ManagedComment {
		return fmt.Errorf("cloudflare: refusing to delete %s (%q): not gpm-managed", rec.Name, rec.Comment)
	}
	if rec.ID == "" {
		return errors.New("cloudflare: refusing to delete a record with no id")
	}
	_, err := c.do(ctx, http.MethodDelete, c.base+"/zones/"+zoneID+"/dns_records/"+url.PathEscape(rec.ID), nil)
	return err
}

// syncCloudflare reconciles the public CNAMEs for hosts opted into publicCname.
// The API credential is not stored here: it is read from the referenced
// DNSProvider's config["apiToken"], so rotating the ACME token rotates this too.
func (s *Syncer) syncCloudflare(ctx context.Context, cfg model.Config, conf model.CloudflareDNSSync) BackendStatus {
	st := BackendStatus{}
	fail := func(err error) BackendStatus {
		st.Error = err.Error()
		return st
	}

	token, err := cloudflareToken(cfg, conf.DNSProviderRef)
	if err != nil {
		return fail(err)
	}
	apex := strings.ToLower(strings.TrimSuffix(conf.ApexTarget, "."))
	desired := desiredDomains(cfg, func(p model.DNSSyncPolicy) bool { return p.PublicCname }, apex)
	st.Desired = len(desired)

	c := &cloudflareClient{token: token, base: cfBaseURL, client: s.client}
	zoneID, err := c.zone(ctx, conf.ZoneName)
	if err != nil {
		return fail(err)
	}
	records, err := c.listCNAMEs(ctx, zoneID)
	if err != nil {
		return fail(err)
	}

	managed := map[string]cfRecord{} // name -> our record
	present := map[string]bool{}     // every CNAME name in the zone, ours or not
	for _, rec := range records {
		name := strings.ToLower(strings.TrimSuffix(rec.Name, "."))
		present[name] = true
		if rec.Comment == ManagedComment {
			managed[name] = rec
		}
	}
	st.Managed = len(managed)

	want := map[string]bool{}
	for _, d := range desired {
		want[d] = true
		if _, mine := managed[d]; mine {
			continue
		}
		if present[d] {
			// The name exists but is somebody else's record. Creating a second
			// CNAME would either be rejected or shadow a deliberate entry, and
			// deleting theirs is exactly what the ownership rule forbids.
			log.Warn().Str("name", d).Msg("dnssync: cloudflare CNAME exists but is not gpm-managed; leaving it alone")
			continue
		}
		if err := c.createCNAME(ctx, zoneID, d, apex, conf.Proxied); err != nil {
			return fail(err)
		}
		st.Created++
		log.Info().Str("name", d).Str("target", apex).Bool("proxied", conf.Proxied).Msg("dnssync: cloudflare CNAME created")
	}
	for name, rec := range managed {
		if want[name] {
			continue
		}
		if err := c.deleteRecord(ctx, zoneID, rec); err != nil {
			return fail(err)
		}
		st.Deleted++
		log.Info().Str("name", name).Msg("dnssync: cloudflare CNAME removed")
	}

	st.OK = true
	return st
}

// cloudflareToken resolves the API token from the referenced DNSProvider object.
func cloudflareToken(cfg model.Config, ref string) (string, error) {
	for _, p := range cfg.DNSProviders {
		if p.Name != ref {
			continue
		}
		secret, ok := p.Config["apiToken"]
		if !ok || secret.IsEmpty() {
			return "", fmt.Errorf("cloudflare: dnsProvider %q has no config.apiToken", ref)
		}
		token, err := secret.Resolve()
		if err != nil {
			return "", fmt.Errorf("cloudflare: resolve dnsProvider %q apiToken: %w", ref, err)
		}
		return token, nil
	}
	return "", fmt.Errorf("cloudflare: dnsSync.cloudflare.dnsProviderRef %q does not name a known dns-providers entry", ref)
}
