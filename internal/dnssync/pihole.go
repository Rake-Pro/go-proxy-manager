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
func (p *piholeClient) logout(ctx context.Context) {
	if p.sid == "" {
		return
	}
	if _, err := p.do(ctx, http.MethodDelete, "/api/auth", nil); err != nil {
		log.Debug().Err(err).Msg("dnssync: pihole logout failed")
	}
	p.sid = ""
}

// cnameRecords returns the raw "domain,target[,ttl]" entries Pi-hole holds.
func (p *piholeClient) cnameRecords(ctx context.Context) ([]string, error) {
	b, err := p.do(ctx, http.MethodGet, "/api/config/dns/cnameRecords", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Config struct {
			DNS struct {
				CnameRecords []string `json:"cnameRecords"`
			} `json:"dns"`
		} `json:"config"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("pihole: decode cnameRecords: %w", err)
	}
	return resp.Config.DNS.CnameRecords, nil
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

// syncPihole reconciles the LAN CNAMEs. Ownership is decided by the target: a
// record is gpm-managed exactly when its CNAME target equals apexTarget, so
// hand-written Pi-hole entries pointing anywhere else are never touched, even if
// they name a domain gpm also serves.
func (s *Syncer) syncPihole(ctx context.Context, cfg model.Config, conf model.PiholeDNSSync) BackendStatus {
	st := BackendStatus{}
	fail := func(err error) BackendStatus {
		st.Error = err.Error()
		return st
	}

	pass, err := conf.AppPassword.Resolve()
	if err != nil {
		return fail(fmt.Errorf("pihole: resolve appPassword: %w", err))
	}
	apex := strings.ToLower(strings.TrimSuffix(conf.ApexTarget, "."))
	desired := desiredDomains(cfg, func(p model.DNSSyncPolicy) bool { return p.LanDirect }, apex)
	st.Desired = len(desired)

	c := newPiholeClient(conf.URL, pass, s.client)
	if err := c.login(ctx); err != nil {
		return fail(err)
	}
	defer c.logout(ctx)

	records, err := c.cnameRecords(ctx)
	if err != nil {
		return fail(err)
	}

	// Managed set: only records whose target is exactly the configured apex.
	// present carries EVERY CNAME name Pi-hole returned, ours or not, so a name an
	// operator deliberately points somewhere else is left intact rather than
	// shadowed by a second entry for the same domain.
	managed := map[string]string{} // domain -> raw record entry
	present := map[string]bool{}
	for _, rec := range records {
		domain, target, ok := piholeRecordTarget(rec)
		if !ok {
			continue
		}
		present[domain] = true
		if target != apex {
			continue
		}
		managed[domain] = rec
	}
	st.Managed = len(managed)

	want := map[string]bool{}
	for _, d := range desired {
		want[d] = true
		if _, exists := managed[d]; exists {
			continue
		}
		if present[d] {
			// The name exists but points somewhere else, so it is somebody else's
			// record. Adding ours would shadow a deliberate entry, and removing
			// theirs is exactly what the ownership rule forbids. Same behaviour as
			// the Cloudflare backend.
			log.Warn().Str("domain", d).Str("target", apex).
				Msg("dnssync: pihole CNAME exists with a different target; leaving it alone")
			continue
		}
		if err := c.addCname(ctx, d+","+apex); err != nil {
			return fail(err)
		}
		st.Created++
		log.Info().Str("domain", d).Str("target", apex).Msg("dnssync: pihole CNAME created")
	}
	for domain, rec := range managed {
		if want[domain] {
			continue
		}
		if err := c.deleteCname(ctx, rec); err != nil {
			return fail(err)
		}
		st.Deleted++
		log.Info().Str("domain", domain).Msg("dnssync: pihole CNAME removed")
	}

	st.OK = true
	return st
}
