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
	// Proxied is carried so a retarget that has to be rolled back can put the
	// record back exactly as it was, orange cloud included.
	Proxied bool `json:"proxied"`
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

// cfPageSize is the per_page value every listing asks for, and therefore what a
// FULL page looks like. cfMaxPages bounds the walk so a backend that keeps
// answering with full pages cannot spin here forever.
const (
	cfPageSize = 100
	cfMaxPages = 1000
)

// listCNAMEs returns every CNAME in the zone, following pagination. The whole
// zone is read (not just gpm's records) because reconcile has to know that a
// desired name is already taken by a record it does not own.
//
// Termination is driven by the page itself: a short page is the end of the
// listing. result_info is only advisory - it can be absent, or zero, and
// trusting it as the sole terminator meant a full page with no result_info
// stopped the walk at page 1. That truncation reads as "the zone does not hold
// these records", which orphans ledger entries and re-attempts creates that
// already exist. So the advisory can extend the walk, never cut it short.
func (c *cloudflareClient) listCNAMEs(ctx context.Context, zoneID string) ([]cfRecord, error) {
	var out []cfRecord
	for page := 1; page <= cfMaxPages; page++ {
		q := url.Values{}
		q.Set("type", "CNAME")
		q.Set("per_page", strconv.Itoa(cfPageSize))
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
		if len(recs) == 0 || (len(recs) < cfPageSize && env.Info.TotalPages <= page) {
			return out, nil
		}
	}
	return nil, fmt.Errorf("cloudflare: zone listing did not terminate within %d pages", cfMaxPages)
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
// (the caller already checked the ledger AND the comment) so no future caller can
// turn this into an arbitrary-delete primitive against the operator's zone.
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

// cloudflareState is one read of the zone: the CNAME target of every name it
// holds, plus the record itself (the delete call needs its ID and comment).
type cloudflareState struct {
	zoneID  string
	present map[string]string   // name -> target
	byName  map[string]cfRecord // name -> the record it came from
}

// cloudflareConnect resolves the zone and lists its CNAMEs - the read-only half
// both the reconcile and the dry-run planner need.
func (s *Syncer) cloudflareConnect(ctx context.Context, cfg model.Config, conf model.CloudflareDNSSync) (*cloudflareClient, cloudflareState, error) {
	token, err := cloudflareToken(cfg, conf.DNSProviderRef)
	if err != nil {
		return nil, cloudflareState{}, err
	}
	c := &cloudflareClient{token: token, base: cfBaseURL, client: s.client}
	zoneID, err := c.zone(ctx, conf.ZoneName)
	if err != nil {
		return nil, cloudflareState{}, err
	}
	records, err := c.listCNAMEs(ctx, zoneID)
	if err != nil {
		return nil, cloudflareState{}, err
	}
	state := cloudflareState{zoneID: zoneID, present: map[string]string{}, byName: map[string]cfRecord{}}
	for _, rec := range records {
		name := strings.ToLower(strings.TrimSuffix(rec.Name, "."))
		if _, dup := state.present[name]; dup {
			continue
		}
		state.present[name] = strings.ToLower(strings.TrimSuffix(rec.Content, "."))
		state.byName[name] = rec
	}
	return c, state, nil
}

// cloudflareDecisions works out what a reconcile would do to the zone. Unlike
// Pi-hole, Cloudflare records CAN carry a marker, so the "managed-by:gpm" comment
// stays in force as a SECOND, independent condition: the ledger authorises a
// delete and the comment still has to agree. That is strictly stronger than the
// comment alone, so nothing this backend already guaranteed is weakened.
func cloudflareDecisions(cfg model.Config, apex string, state cloudflareState, owned map[string]model.DNSClaim) (decisions, []string) {
	desired := desiredDomains(cfg, func(p model.DNSSyncPolicy) bool { return p.PublicCname }, apex)
	mark := func(name string) bool { return state.byName[name].Comment == ManagedComment }
	return decide("cloudflare", desired, state.present, apex, owned, mark), desired
}

// planCloudflare is the read-only preview: it resolves the zone, lists it and
// reports the decisions without issuing a single write.
func (s *Syncer) planCloudflare(ctx context.Context, cfg model.Config, conf model.CloudflareDNSSync, owned map[string]model.DNSClaim) BackendPlan {
	apex := strings.ToLower(strings.TrimSuffix(conf.ApexTarget, "."))
	_, state, err := s.cloudflareConnect(ctx, cfg, conf)
	if err != nil {
		return BackendPlan{Error: err.Error()}
	}
	d, _ := cloudflareDecisions(cfg, apex, state, owned)
	return d.plan()
}

// syncCloudflare reconciles the public CNAMEs for hosts opted into publicCname
// and returns the ownership ledger the run ended with. The API credential is not
// stored here: it is read from the referenced DNSProvider's config["apiToken"],
// so rotating the ACME token rotates this too.
//
// ledgerRev identifies the config-repo revision the ledger was read at; it is
// logged with every deletion so an operator can tell which recorded claim
// authorised destroying a record (see the revert caveat in
// docs/how-to/dns-sync.md).
func (s *Syncer) syncCloudflare(ctx context.Context, cfg model.Config, conf model.CloudflareDNSSync, owned map[string]model.DNSClaim, ledgerRev string) (BackendStatus, map[string]model.DNSClaim) {
	st := BackendStatus{}
	apex := strings.ToLower(strings.TrimSuffix(conf.ApexTarget, "."))

	c, state, err := s.cloudflareConnect(ctx, cfg, conf)
	if err != nil {
		st.Error = err.Error()
		st.Desired = len(desiredDomains(cfg, func(p model.DNSSyncPolicy) bool { return p.PublicCname }, apex))
		return st, owned
	}

	d, desired := cloudflareDecisions(cfg, apex, state, owned)
	st.Desired = len(desired)
	st.Skipped = len(d.skip)
	st.Untouched = d.untouched

	live := map[string]model.DNSClaim{}
	for k, v := range owned {
		live[k] = v
	}
	fail := func(err error) (BackendStatus, map[string]model.DNSClaim) {
		st.Error = err.Error()
		st.Managed = len(live)
		return st, live
	}

	for _, name := range d.adopt {
		live[name] = model.DNSClaim{Target: apex, Adopted: true}
		st.Adopted++
		log.Info().Str("name", name).Str("target", apex).
			Msg("dnssync: cloudflare CNAME already present, correct and gpm-commented; adopted as gpm-managed (gpm will never delete it, only release it)")
	}
	for _, name := range d.retarget {
		// Delete then create, with no atomic update in between: the counter is bumped
		// as soon as the DELETE lands so a failure afterwards cannot be reported as a
		// run that changed nothing.
		rec := state.byName[name]
		if err := c.deleteRecord(ctx, state.zoneID, rec); err != nil {
			return fail(err)
		}
		delete(live, name)
		st.Retargeted++
		if err := c.createCNAME(ctx, state.zoneID, name, apex, conf.Proxied); err != nil {
			// Restore the record exactly as it was rather than leaving the name
			// unresolved until some later reconcile happens to heal it.
			if rbErr := c.createCNAME(ctx, state.zoneID, name, rec.Content, rec.Proxied); rbErr != nil {
				log.Error().Err(rbErr).Str("name", name).Str("content", rec.Content).
					Msg("dnssync: cloudflare retarget failed AND the original record could not be restored; the name is currently unresolved")
				return fail(fmt.Errorf("cloudflare: retarget %s: %w (restoring the original record also failed: %v)", name, err, rbErr))
			}
			live[name] = owned[name]
			log.Error().Err(err).Str("name", name).Str("content", rec.Content).
				Msg("dnssync: cloudflare retarget failed; the original record was restored")
			return fail(fmt.Errorf("cloudflare: retarget %s: %w (the original record was restored)", name, err))
		}
		live[name] = model.DNSClaim{Target: apex}
		log.Info().Str("name", name).Str("from", state.present[name]).Str("to", apex).
			Msg("dnssync: cloudflare CNAME retargeted")
	}
	for _, name := range d.create {
		if err := c.createCNAME(ctx, state.zoneID, name, apex, conf.Proxied); err != nil {
			return fail(err)
		}
		live[name] = model.DNSClaim{Target: apex}
		st.Created++
		log.Info().Str("name", name).Str("target", apex).Bool("proxied", conf.Proxied).Msg("dnssync: cloudflare CNAME created")
	}
	for _, name := range d.del {
		// Loud on purpose, and stamped with the ledger revision that authorised it:
		// a whole-tree revert restores the ledger with everything else, so a claim
		// can outlive the record gpm created (see docs/how-to/dns-sync.md).
		log.Warn().Str("name", name).Str("target", state.present[name]).Str("ledgerRev", ledgerRev).
			Msg("dnssync: deleting a cloudflare CNAME on the authority of the ownership ledger")
		if err := c.deleteRecord(ctx, state.zoneID, state.byName[name]); err != nil {
			return fail(err)
		}
		delete(live, name)
		st.Deleted++
	}
	live = d.owned

	st.Managed = len(live)
	st.OK = true
	if st.Adopted > 0 || st.Untouched > 0 {
		log.Info().Str("backend", "cloudflare").Int("adopted", st.Adopted).Int("untouched", st.Untouched).
			Int("created", st.Created).Int("deleted", st.Deleted).
			Msg("dnssync: reconcile complete; records gpm does not own were left exactly as they were")
	}
	return st, live
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
