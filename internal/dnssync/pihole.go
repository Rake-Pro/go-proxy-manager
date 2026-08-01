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
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// ErrPiholeForbidden is the distinct outcome for a Pi-hole 403: the session
// authenticated but is not allowed to change configuration. In practice that
// means the API is in read-only mode or the instance was built without
// `webserver.api.app_sudo`, and no amount of retrying will help - so it is
// surfaced verbatim in the sync status instead of looking like a transient error.
var ErrPiholeForbidden = errors.New("pihole: forbidden (session is read-only, or app_sudo is disabled)")

// piholeClient talks to the Pi-hole v6 REST API. A session is opened with the
// application password, carried in the `sid` header, and explicitly closed:
// Pi-hole has a small fixed pool of concurrent sessions, so leaking one per
// reconcile would eventually lock the operator out of their own admin UI.
type piholeClient struct {
	base   string
	pass   string
	client *http.Client
	sid    string
}

func newPiholeClient(baseURL, appPassword string, c *http.Client) *piholeClient {
	return &piholeClient{base: strings.TrimRight(baseURL, "/"), pass: appPassword, client: c}
}

func (p *piholeClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
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
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("pihole: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.sid != "" {
		req.Header.Set("sid", p.sid)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pihole: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	// Bound the read: the response comes from an admin-configured host, so a
	// misdirected URL must not be able to stream unbounded data into memory.
	_, _ = buf.ReadFrom(io.LimitReader(resp.Body, maxRespBody))
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w (%s %s)", ErrPiholeForbidden, method, path)
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("pihole: unauthorized on %s %s (check dnsSync.pihole.appPassword)", method, path)
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("pihole: %s %s: unexpected status %d", method, path, resp.StatusCode)
	}
	return buf.Bytes(), nil
}

// login opens a Pi-hole session and stores its sid for subsequent calls.
func (p *piholeClient) login(ctx context.Context) error {
	b, err := p.do(ctx, http.MethodPost, "/api/auth", map[string]string{"password": p.pass})
	if err != nil {
		return err
	}
	var resp struct {
		Session struct {
			Valid bool   `json:"valid"`
			SID   string `json:"sid"`
		} `json:"session"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return fmt.Errorf("pihole: decode auth response: %w", err)
	}
	if !resp.Session.Valid || resp.Session.SID == "" {
		return errors.New("pihole: authentication rejected (no valid session returned)")
	}
	p.sid = resp.Session.SID
	return nil
}

// logout releases the session slot. Best effort: a failure here is logged, never
// propagated, because the reconcile itself has already succeeded or failed.
//
// It runs on a context DETACHED from the caller's. logout is almost always
// reached through a defer, and the commonest reason to reach it is the run being
// cut short - an HTTP client disconnecting cancels the request-scoped context,
// and reusing that context here would cancel the logout as well, leaking the
// session every time. Pi-hole has a small fixed pool of them, so a leak per
// aborted run locks the operator out of their own admin UI. Cancellation
// propagation is deliberately traded for a short fixed deadline.
func (p *piholeClient) logout(ctx context.Context) {
	if p.sid == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := p.do(ctx, http.MethodDelete, "/api/auth", nil); err != nil {
		log.Debug().Err(err).Msg("dnssync: pihole logout failed")
	}
	p.sid = ""
}

// cnameRecords returns the raw "domain,target[,ttl]" entries Pi-hole holds.
//
// The field is decoded into a POINTER so "Pi-hole returned no records" and
// "Pi-hole returned a body this code cannot read" are different outcomes. They
// look identical to a plain []string - both give a nil slice - and the second one
// silently means "the backend holds nothing", which a full-state reconciler reads
// as "everything gpm owns has been deleted out of band" and answers by emptying
// the ledger and reporting a clean run. An API shape change (a renamed field, an
// envelope that moved) must be an error, not a wipe.
func (p *piholeClient) cnameRecords(ctx context.Context) ([]string, error) {
	b, err := p.do(ctx, http.MethodGet, "/api/config/dns/cnameRecords", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Config *struct {
			DNS *struct {
				CnameRecords *[]string `json:"cnameRecords"`
			} `json:"dns"`
		} `json:"config"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("pihole: decode cnameRecords: %w", err)
	}
	if resp.Config == nil || resp.Config.DNS == nil || resp.Config.DNS.CnameRecords == nil {
		return nil, errors.New("pihole: response carries no config.dns.cnameRecords list (unexpected API shape; refusing to read it as an empty resolver)")
	}
	return *resp.Config.DNS.CnameRecords, nil
}

// escapeRecord encodes a "domain,target" entry for use as a path segment. The
// comma is escaped explicitly: it is a legal sub-delimiter that url.PathEscape
// leaves alone, but Pi-hole expects the whole record as one opaque segment.
func escapeRecord(rec string) string {
	return strings.ReplaceAll(url.PathEscape(rec), ",", "%2C")
}

func (p *piholeClient) addCname(ctx context.Context, rec string) error {
	_, err := p.do(ctx, http.MethodPut, "/api/config/dns/cnameRecords/"+escapeRecord(rec), nil)
	return err
}

func (p *piholeClient) deleteCname(ctx context.Context, rec string) error {
	_, err := p.do(ctx, http.MethodDelete, "/api/config/dns/cnameRecords/"+escapeRecord(rec), nil)
	return err
}

// piholeRecordTarget returns the CNAME target of a raw Pi-hole record entry,
// which is "domain,target" with an optional trailing ",ttl".
func piholeRecordTarget(rec string) (domain, target string, ok bool) {
	parts := strings.Split(rec, ",")
	if len(parts) < 2 {
		return "", "", false
	}
	domain = strings.ToLower(strings.TrimSpace(parts[0]))
	target = strings.ToLower(strings.TrimSpace(parts[1]))
	if domain == "" || target == "" {
		return "", "", false
	}
	return domain, target, true
}

// piholeState is everything one read of the Pi-hole gives the reconciler: the
// target of every CNAME it holds, and the raw "domain,target[,ttl]" entry each of
// those came from (the delete endpoint takes the raw entry, not the name).
type piholeState struct {
	present map[string]string // domain -> target
	raw     map[string]string // domain -> raw record entry
}

func readPiholeState(ctx context.Context, c *piholeClient) (piholeState, error) {
	records, err := c.cnameRecords(ctx)
	if err != nil {
		return piholeState{}, err
	}
	st := piholeState{present: map[string]string{}, raw: map[string]string{}}
	for _, rec := range records {
		domain, target, ok := piholeRecordTarget(rec)
		if !ok {
			continue
		}
		if _, dup := st.present[domain]; dup {
			continue // first entry wins, as dnsmasq itself resolves it
		}
		st.present[domain] = target
		st.raw[domain] = rec
	}
	return st, nil
}

// piholeConnect resolves the credential, opens a session and reads the current
// records - the read-only half both the reconcile and the dry-run planner need.
// The caller must call logout on the returned client.
func (s *Syncer) piholeConnect(ctx context.Context, conf model.PiholeDNSSync) (*piholeClient, piholeState, error) {
	pass, err := conf.AppPassword.Resolve()
	if err != nil {
		return nil, piholeState{}, fmt.Errorf("pihole: resolve appPassword: %w", err)
	}
	c := newPiholeClient(conf.URL, pass, s.client)
	if err := c.login(ctx); err != nil {
		return nil, piholeState{}, err
	}
	state, err := readPiholeState(ctx, c)
	if err != nil {
		c.logout(ctx)
		return nil, piholeState{}, err
	}
	return c, state, nil
}

// piholeDecisions reads Pi-hole and works out what a reconcile would do.
// Pi-hole/dnsmasq CNAMEs carry no comment field, so there is no secondary
// ownership marker to check: the ledger is the ONLY thing that says a record is
// gpm's, which is why mark is unconditionally true here.
func piholeDecisions(cfg model.Config, apex string, state piholeState, owned map[string]model.DNSClaim) (decisions, []string) {
	desired := desiredDomains(cfg, func(p model.DNSSyncPolicy) bool { return p.LanDirect }, apex)
	return decide("pihole", desired, state.present, apex, owned, func(string) bool { return true }), desired
}

// planPihole is the read-only preview: it logs in, reads the records and reports
// the decisions without issuing a single write.
func (s *Syncer) planPihole(ctx context.Context, cfg model.Config, conf model.PiholeDNSSync, owned map[string]model.DNSClaim) BackendPlan {
	apex := strings.ToLower(strings.TrimSuffix(conf.ApexTarget, "."))
	c, state, err := s.piholeConnect(ctx, conf)
	if err != nil {
		return BackendPlan{Error: err.Error()}
	}
	defer c.logout(ctx)
	d, _ := piholeDecisions(cfg, apex, state, owned)
	return d.plan()
}

// syncPihole reconciles the LAN CNAMEs and returns the ownership ledger the run
// ended with. Ownership is the ledger and nothing else: a CNAME gpm did not
// create is never deleted, however exactly its target matches apexTarget - which
// is precisely the inference that cost an operator 19 hand-written records on
// 2026-08-01.
//
// The returned ledger reflects the writes that actually landed, so a run that
// fails half way still records the records it created before failing.
//
// ledgerRev identifies the config-repo revision the ledger was read at; it is
// logged with every deletion so an operator can tell which recorded claim
// authorised destroying a record (see the revert caveat in docs/configuration.md).
func (s *Syncer) syncPihole(ctx context.Context, cfg model.Config, conf model.PiholeDNSSync, owned map[string]model.DNSClaim, ledgerRev string) (BackendStatus, map[string]model.DNSClaim) {
	st := BackendStatus{}
	apex := strings.ToLower(strings.TrimSuffix(conf.ApexTarget, "."))

	c, state, err := s.piholeConnect(ctx, conf)
	if err != nil {
		st.Error = err.Error()
		st.Desired = len(desiredDomains(cfg, func(p model.DNSSyncPolicy) bool { return p.LanDirect }, apex))
		return st, owned
	}
	defer c.logout(ctx)

	d, desired := piholeDecisions(cfg, apex, state, owned)
	st.Desired = len(desired)
	st.Skipped = len(d.skip)
	st.Untouched = d.untouched

	// live is the ledger as it stands after each successful write, so an error part
	// way through still leaves gpm owning exactly what it managed to create.
	live := map[string]model.DNSClaim{}
	for k, v := range owned {
		live[k] = v
	}
	fail := func(err error) (BackendStatus, map[string]model.DNSClaim) {
		st.Error = err.Error()
		st.Managed = len(live)
		return st, live
	}

	// Adoptions first: they are ledger-only and cannot fail, and doing them before
	// any write means a later failure still leaves the pre-existing records claimed
	// rather than looking unowned (and so re-adoptable, or worse, duplicable).
	for _, name := range d.adopt {
		live[name] = model.DNSClaim{Target: apex, Adopted: true}
		st.Adopted++
		log.Info().Str("domain", name).Str("target", apex).
			Msg("dnssync: pihole CNAME already present and correct; adopted as gpm-managed (gpm will never delete it, only release it)")
	}
	for _, name := range d.retarget {
		// dnsmasq has no update: a retarget is a delete followed by a create, and the
		// window in between is one where the record does not exist at all. The
		// counter is bumped as soon as the DELETE lands, so a run that fails after it
		// can never report "nothing happened" while having removed a record.
		if err := c.deleteCname(ctx, state.raw[name]); err != nil {
			return fail(err)
		}
		delete(live, name)
		st.Retargeted++
		if err := c.addCname(ctx, name+","+apex); err != nil {
			// The record is gone and its replacement did not land. Put the ORIGINAL
			// back before giving up: a later reconcile would heal this, but until then
			// the name simply does not resolve, and an outage of unbounded length is
			// not an acceptable cost for a failed write.
			if rbErr := c.addCname(ctx, state.raw[name]); rbErr != nil {
				log.Error().Err(rbErr).Str("domain", name).Str("record", state.raw[name]).
					Msg("dnssync: pihole retarget failed AND the original record could not be restored; the name is currently unresolved")
				return fail(fmt.Errorf("pihole: retarget %s: %w (restoring the original record also failed: %v)", name, err, rbErr))
			}
			live[name] = owned[name]
			log.Error().Err(err).Str("domain", name).Str("target", state.present[name]).
				Msg("dnssync: pihole retarget failed; the original record was restored")
			return fail(fmt.Errorf("pihole: retarget %s: %w (the original record was restored)", name, err))
		}
		live[name] = model.DNSClaim{Target: apex}
		log.Info().Str("domain", name).Str("from", state.present[name]).Str("to", apex).
			Msg("dnssync: pihole CNAME retargeted")
	}
	for _, name := range d.create {
		if err := c.addCname(ctx, name+","+apex); err != nil {
			return fail(err)
		}
		live[name] = model.DNSClaim{Target: apex}
		st.Created++
		log.Info().Str("domain", name).Str("target", apex).Msg("dnssync: pihole CNAME created")
	}
	for _, name := range d.del {
		// Loud on purpose: this is the one operation that destroys something. The
		// authority for it is a recorded claim, and a claim can be older than the
		// config it is being applied to - a whole-tree revert restores the ledger
		// along with everything else, so an entry can outlive the record gpm created
		// (see the revert caveat in docs/configuration.md). Naming the revision the
		// claim was read at makes that auditable after the fact.
		log.Warn().Str("domain", name).Str("target", state.present[name]).Str("ledgerRev", ledgerRev).
			Msg("dnssync: deleting a pihole CNAME on the authority of the ownership ledger")
		if err := c.deleteCname(ctx, state.raw[name]); err != nil {
			return fail(err)
		}
		delete(live, name)
		st.Deleted++
	}
	// Every planned write landed, so the ledger is exactly the planned end state -
	// including the names gpm disowned this run (changed or removed out of band),
	// which leave the ledger without any record being touched.
	live = d.owned

	st.Managed = len(live)
	st.OK = true
	if st.Adopted > 0 || st.Untouched > 0 {
		log.Info().Str("backend", "pihole").Int("adopted", st.Adopted).Int("untouched", st.Untouched).
			Int("created", st.Created).Int("deleted", st.Deleted).
			Msg("dnssync: reconcile complete; records gpm does not own were left exactly as they were")
	}
	return st, live
}
