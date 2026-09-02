package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/acme"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// Certificate status states, reported in the "state" field GET /certificates
// decorates onto every stored Certificate and counted by GET /health.
const (
	CertStateValid    = "valid"
	CertStateExpiring = "expiring"
	CertStateExpired  = "expired"
	CertStatePending  = "pending" // acme certificate: no order has completed yet
	CertStateError    = "error"   // last attempt (order or file read) failed
)

// certExpiringWindow matches the default ACME renewal window (30 days): a
// certificate at or inside it is "expiring", the same threshold the ACME
// manager itself renews at, so the two never disagree about a certificate on
// the edge.
const certExpiringWindow = 30 * 24 * time.Hour

// maxLastErrorLen bounds the lastError field: an ACME CA's rejection detail or a
// Go file error can run long, and this is a status summary, not a log.
const maxLastErrorLen = 300

// UpstreamGroupHealth is one upstream group's healthy/unhealthy upstream count,
// for GET /health. The API package does not import internal/dataplane's
// per-upstream type; the daemon derives this from it (see Deps.UpstreamGroupSummary).
type UpstreamGroupHealth struct {
	Name      string `json:"name"`
	Healthy   int    `json:"healthy"`
	Unhealthy int    `json:"unhealthy"`
}

// certObservation looks up the ACME manager's in-memory state for one
// certificate. ok is false when no ACME manager is wired (a follower, which
// does not run it) or the manager has not looked at this certificate yet.
func (d Deps) certObservation(name string) (acme.CertObservation, bool) {
	if d.ACMEObservations == nil {
		return acme.CertObservation{}, false
	}
	for _, o := range d.ACMEObservations() {
		if o.Name == name {
			return o, true
		}
	}
	return acme.CertObservation{}, false
}

// certState derives the expiry state at time now. A certificate at or inside
// certExpiringWindow of its expiry is "expiring", matching the ACME manager's
// own renewal threshold.
func certState(now, notAfter time.Time) string {
	switch {
	case !now.Before(notAfter):
		return CertStateExpired
	case notAfter.Sub(now) <= certExpiringWindow:
		return CertStateExpiring
	default:
		return CertStateValid
	}
}

// truncateLastError bounds an error message to maxLastErrorLen, ASCII-only (no
// ellipsis character - see CLAUDE.md typography rule).
func truncateLastError(msg string) string {
	if len(msg) <= maxLastErrorLen {
		return msg
	}
	return msg[:maxLastErrorLen] + "..."
}

// readLeaf reads a PEM certificate chain from disk and parses its leaf's
// status fields. Shared by ACME-issued and custom certificates: both are a
// plain PEM chain on disk, so there is one read+parse path for both.
func readLeaf(path string) (acme.LeafInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return acme.LeafInfo{}, err
	}
	return acme.ParseLeaf(b)
}

// certStatus computes the read-only status fields GET /certificates decorates
// onto a stored Certificate object: expiry, issuer, SANs, and (for ACME
// certificates) the manager's most recent renewal attempt. It never touches the
// stored config - only the cert store on disk and the ACME manager's in-memory
// observations.
func (d Deps) certStatus(c model.Certificate, admin bool) map[string]any {
	now := time.Now()
	out := map[string]any{"state": CertStateError} // overwritten below on every path

	obs, hasObs := d.certObservation(c.Name)
	if hasObs && obs.LastError != "" {
		out["lastError"] = certStatusError(obs.LastError, admin)
	}
	if hasObs && !obs.LastAttempt.IsZero() {
		out["lastAttempt"] = obs.LastAttempt
	}

	var (
		leafPath string
		pending  bool // ACME certificate with no issued artifact yet
	)
	switch c.Type {
	case model.CertTypeACME:
		if _, err := acme.LoadIssuedMeta(d.CertDir, c.Name); err != nil {
			pending = true
		} else {
			leafPath = acme.IssuedCertPath(d.CertDir, c.Name)
		}
	case model.CertTypeCustom:
		if c.Custom != nil {
			leafPath = filepath.Join(d.CertDir, c.Custom.CertFile)
		} else {
			out["lastError"] = "no custom certificate configured"
		}
	}

	switch {
	case pending:
		if hasObs && obs.LastError != "" {
			out["state"] = CertStateError
		} else {
			out["state"] = CertStatePending
		}
	case leafPath != "":
		info, err := readLeaf(leafPath)
		if err != nil {
			out["state"] = CertStateError
			out["lastError"] = certStatusError(err.Error(), admin)
			return out
		}
		out["notBefore"] = info.NotBefore
		out["notAfter"] = info.NotAfter
		out["daysRemaining"] = int(info.NotAfter.Sub(now) / (24 * time.Hour))
		if info.Issuer != "" {
			out["issuer"] = info.Issuer
		}
		if len(info.SANs) > 0 {
			out["sans"] = info.SANs
		}
		out["state"] = certState(now, info.NotAfter)
	}
	return out
}

// certStatusError renders one certificate status error: the full text for an
// admin caller, the same fixed classification GET /health uses for anyone
// else. The raw text can embed a third party's HTTP response body (a DNS
// provider's error snippet, an ACME problem document) or a local file/parse
// detail, and certificates:read - unlike admin - is held by the read-only
// viewer role, so GET /certificates and GET /certificates/{name} must not leak
// it any more than /health does.
func certStatusError(msg string, admin bool) string {
	if admin {
		return truncateLastError(msg)
	}
	return classifyACMEError(msg)
}

// decorateCertificate is the resource[model.Certificate].decorate hook: it
// merges certStatus's fields onto the stored object for GET /certificates and
// GET /certificates/{name}. None of it is written back - see model.Certificate's
// doc comment.
func (d Deps) decorateCertificate(c model.Certificate, r *http.Request) any {
	out, err := mergeExtra(c, d.certStatus(c, d.allows(r, model.ScopeAdmin)))
	if err != nil {
		return c
	}
	return out
}

// handleRenewCertificate is POST /certificates/{name}/renew: force an
// immediate ACME order for one certificate, ignoring the normal 30-day renewal
// window. It responds as soon as the order has started (or is refused), not
// once it completes - see acme.Manager.RenewNow.
func (d Deps) handleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := model.ValidateName(name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if d.ACMERenewNow == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("certificate renewal is not wired (this instance is not the ACME issuer)"))
		return
	}
	cfg, _, err := d.Store.Load(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := d.ACMERenewNow(r.Context(), cfg, name); err != nil {
		switch {
		case errors.Is(err, acme.ErrCertNotFound):
			writeErr(w, http.StatusNotFound, errNotFound("Certificate", name))
		case errors.Is(err, acme.ErrNotACME):
			writeErr(w, http.StatusBadRequest, fmt.Errorf("certificate %q is not an acme certificate; custom certificates are not renewed by gpm", name))
		case errors.Is(err, acme.ErrRenewInFlight), errors.Is(err, acme.ErrRenewCooldown):
			writeErr(w, http.StatusConflict, err)
		default:
			writeErr(w, http.StatusBadGateway, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"started": true})
}

// healthListener is one data-plane listener's live status, for GET /health.
type healthListener struct {
	Listening bool   `json:"listening"`
	Addr      string `json:"addr"`
}

// healthCertificates summarizes every certificate's state, for GET /health.
type healthCertificates struct {
	Total    int `json:"total"`
	Expiring int `json:"expiring"`
	Expired  int `json:"expired"`
	Error    int `json:"error"`
}

// healthACME is the ACME renewal loop's live status, for GET /health.
type healthACME struct {
	// LastRenewalRun is zero when the loop has never run (a follower, which does
	// not run it, or before the first tick).
	LastRenewalRun time.Time `json:"lastRenewalRun,omitempty"`
	// LastError is the most recent renewal attempt's failure across every
	// certificate, empty if none has failed since startup.
	LastError string `json:"lastError,omitempty"`
}

// healthResponse is the GET /health payload: everything in-memory the admin
// Overview page needs to answer "is this instance actually working", without
// the client stitching it together from /runtime, /certificates and
// /upstream-health itself.
type healthResponse struct {
	DataPlane struct {
		HTTPS healthListener `json:"https"`
		HTTP  healthListener `json:"http"`
	} `json:"dataPlane"`
	Certificates   healthCertificates    `json:"certificates"`
	UpstreamGroups []UpstreamGroupHealth `json:"upstreamGroups"`
	ACME           healthACME            `json:"acme"`
	HARole         string                `json:"haRole"`
	ConfigHead     string                `json:"configHead"`
	// ConfigWarnings mirrors cfg.Warnings(): non-fatal config smells, including
	// an unknown YAML key an older struct silently ignored (see
	// docs/operations/upgrading.md#rollback). Empty when there is nothing to
	// warn about.
	ConfigWarnings []string `json:"configWarnings,omitempty"`
}

// handleHealth is GET /health: an aggregate, read-only snapshot of the running
// process, sourced entirely from in-memory state (plus the cached cert files
// certStatus already reads for GET /certificates) - no ACME CA or DNS calls.
func (d Deps) handleHealth(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := d.Store.Load(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	head, err := d.Store.Head(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	var resp healthResponse
	if d.DataPlaneListening != nil {
		httpsListening, httpListening := d.DataPlaneListening()
		resp.DataPlane.HTTPS = healthListener{Listening: httpsListening, Addr: d.Runtime.HTTPSAddr}
		resp.DataPlane.HTTP = healthListener{Listening: httpListening, Addr: d.Runtime.HTTPAddr}
	} else {
		resp.DataPlane.HTTPS = healthListener{Addr: d.Runtime.HTTPSAddr}
		resp.DataPlane.HTTP = healthListener{Addr: d.Runtime.HTTPAddr}
	}

	resp.Certificates.Total = len(cfg.Certificates)
	for _, c := range cfg.Certificates {
		// Only "state" is read here; classification of lastError does not apply -
		// GET /health already reports its own, separately classified summary via
		// latestACMEError below.
		switch d.certStatus(c, true)["state"] {
		case CertStateExpiring:
			resp.Certificates.Expiring++
		case CertStateExpired:
			resp.Certificates.Expired++
		case CertStateError:
			resp.Certificates.Error++
		}
	}

	if d.UpstreamGroupSummary != nil {
		resp.UpstreamGroups = d.UpstreamGroupSummary()
	}
	if resp.UpstreamGroups == nil {
		resp.UpstreamGroups = []UpstreamGroupHealth{}
	}

	if d.ACMELastRun != nil {
		resp.ACME.LastRenewalRun = d.ACMELastRun()
	}
	resp.ACME.LastError = d.latestACMEError()

	resp.HARole = d.Role.String()
	resp.ConfigHead = head
	resp.ConfigWarnings = cfg.Warnings()

	writeJSON(w, http.StatusOK, resp)
}

// latestACMEError names the certificate whose most recent attempt failed most
// recently, and CLASSIFIES the failure - it does not echo the message. The raw
// text can embed a third party's HTTP response body (a DNS provider's error
// snippet, an ACME problem document), and this route is readable by every role,
// including the read-only viewer. The full message is available only to an
// admin caller, on the certificate's own status route (see certStatusError).
func (d Deps) latestACMEError() string {
	if d.ACMEObservations == nil {
		return ""
	}
	var (
		latest    time.Time
		lastError string
		certName  string
	)
	for _, o := range d.ACMEObservations() {
		if o.LastError == "" {
			continue
		}
		if lastError == "" || o.LastAttempt.After(latest) {
			latest = o.LastAttempt
			lastError = o.LastError
			certName = o.Name
		}
	}
	if lastError == "" {
		return ""
	}
	return fmt.Sprintf("certificate %q: %s", certName, classifyACMEError(lastError))
}

// classifyACMEError reduces a renewal failure to one of a fixed set of phrases.
// Every branch matches on gpm's own error wording, and the fallback says only
// that the attempt failed - no part of the input is ever returned.
func classifyACMEError(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "rate limit") || strings.Contains(m, "toomanyrequests") || strings.Contains(m, "429"):
		return "rate limited by the ACME directory"
	case strings.Contains(m, "timeout") || strings.Contains(m, "timed out") || strings.Contains(m, "deadline exceeded"):
		return "timed out"
	case strings.Contains(m, "dns") || strings.Contains(m, "txt record") || strings.Contains(m, "propagat"):
		return "dns-01 challenge or DNS provider failure"
	case strings.Contains(m, "http-01") || strings.Contains(m, "challenge"):
		return "challenge failure"
	case strings.Contains(m, "account") || strings.Contains(m, "unauthorized") || strings.Contains(m, "forbidden"):
		return "ACME account or authorization failure"
	default:
		return "renewal failed; ask an admin for the detailed error"
	}
}
