package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/clientcert"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// clientCertItem is one issuance record as the API reports it: the stored record
// plus the expiry state derived at request time, so the SPA never has to
// re-implement the threshold arithmetic.
type clientCertItem struct {
	clientcert.Record
	Status        string `json:"status"`
	DaysRemaining int    `json:"daysRemaining"`
}

// clientCertList is the GET payload. It is an object rather than the bare array
// the config-resource lists return, because the caller needs the CA's effective
// warning window alongside the records to render the banner - and because these
// are runtime records, not config objects.
type clientCertList struct {
	ExpiryWarningDays int              `json:"expiryWarningDays"`
	Certificates      []clientCertItem `json:"certificates"`
}

// findClientCA loads the config and returns the named ClientCA, writing a 404 or
// 500 and returning false when it cannot.
func (d Deps) findClientCA(w http.ResponseWriter, r *http.Request, name string) (model.ClientCA, bool) {
	cfg, _, err := d.Store.Load(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return model.ClientCA{}, false
	}
	for _, ca := range cfg.ClientCAs {
		if ca.Name == name {
			return ca, true
		}
	}
	writeErr(w, http.StatusNotFound, errors.New("ClientCA "+name+" not found"))
	return model.ClientCA{}, false
}

// handleListClientCerts reports every client certificate this CA has issued,
// newest first, each with its derived expiry status. It is a read of runtime
// state, so it takes the same client-cas:read scope as reading the CA itself.
func (d Deps) handleListClientCerts(led *clientcert.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := model.ValidateName(name); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ca, ok := d.findClientCA(w, r, name)
		if !ok {
			return
		}
		recs, err := led.List(name)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		warn := ca.WarningDays()
		now := time.Now()
		items := make([]clientCertItem, 0, len(recs))
		for _, rec := range recs {
			items = append(items, clientCertItem{
				Record:        rec,
				Status:        rec.Status(now, warn),
				DaysRemaining: rec.DaysRemaining(now),
			})
		}
		writeJSON(w, http.StatusOK, clientCertList{ExpiryWarningDays: warn, Certificates: items})
	}
}

// handleIssueClientCert mints a client certificate signed by the named ClientCA
// and streams it back as a password-protected PKCS#12 bundle.
//
// It commits nothing to the config: no config object changes, so there is no
// revision, no history entry and no lifecycle event. What it does persist is an
// issuance RECORD in the runtime ledger (subject, serial, validity - never key
// material), which is what the expiry warning and the renew flow read. The
// generated private key lives only in the response body.
func (d Deps) handleIssueClientCert(led *clientcert.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := model.ValidateName(name); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		body, err := readBody(w, r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		var req clientcert.Request
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ca, ok := d.findClientCA(w, r, name)
		if !ok {
			return
		}
		d.mintAndStream(w, ca, led, req, "")
	}
}

// renewRequest is the renew body. The common name and SANs are NOT accepted from
// the client: a renewal reissues the identity already on record, so changing the
// subject means issuing a new certificate, not renewing one.
type renewRequest struct {
	Password     string `json:"password"`
	ValidityDays int    `json:"validityDays,omitempty"`
}

// handleRenewClientCert reissues the certificate named by an existing record:
// same common name and SANs, a brand-new key and serial, and the old record
// marked superseded by the new one.
//
// It does NOT revoke the superseded certificate. Revocation stays CRL-based
// (ClientCA.crlFile / crlPEM), so the old bundle keeps working on every device
// that still holds it until it expires or its serial is added to the CRL - which
// is exactly why the superseded record stays listed.
func (d Deps) handleRenewClientCert(led *clientcert.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, serial := r.PathValue("name"), r.PathValue("serial")
		if err := model.ValidateName(name); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		body, err := readBody(w, r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		var req renewRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		ca, ok := d.findClientCA(w, r, name)
		if !ok {
			return
		}
		rec, err := led.Get(name, serial)
		if err != nil {
			if errors.Is(err, clientcert.ErrRecordNotFound) {
				writeErr(w, http.StatusNotFound, err)
				return
			}
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		// A record that was already renewed must not be renewed again: doing so
		// would mint a SECOND live certificate for the same identity and rewrite
		// the supersede link, leaving the first renewal looking current while
		// nothing points at it. Renew the current certificate instead - which is
		// the one this record already names.
		if rec.SupersededBy != "" {
			writeErr(w, http.StatusConflict, fmt.Errorf(
				"certificate %s was already renewed as %s; renew that certificate instead (this record is historical - the certificate it names stays valid until it expires, it is simply no longer the current one)",
				rec.Serial, rec.SupersededBy))
			return
		}
		d.mintAndStream(w, ca, led, clientcert.Request{
			CommonName:   rec.CommonName,
			SANs:         rec.SANs,
			Password:     req.Password,
			ValidityDays: req.ValidityDays,
		}, rec.Serial)
	}
}

// mintAndStream is the shared tail of issue and renew: sign, record, stream. The
// record is written BEFORE the bundle is handed over, so a bundle can never exist
// that gpm has no memory of; a failed write refuses the issuance instead.
func (d Deps) mintAndStream(w http.ResponseWriter, ca model.ClientCA, led *clientcert.Ledger, req clientcert.Request, supersedes string) {
	res, err := clientcert.Issue(ca, d.CertDir, req)
	if err != nil {
		writeErr(w, issueStatus(err), err)
		return
	}
	rec := clientcert.Record{
		CA:         ca.Name,
		CommonName: res.CommonName,
		SANs:       res.SANs,
		Serial:     res.Serial,
		NotBefore:  res.NotBefore,
		NotAfter:   res.NotAfter,
		IssuedAt:   time.Now().UTC(),
	}
	if supersedes != "" {
		err = led.AppendSuperseding(rec, supersedes)
	} else {
		err = led.Append(rec)
	}
	if err != nil {
		// The record is written BEFORE the bundle is streamed, so this is the one
		// failure that refuses an already-signed certificate. That is deliberate:
		// a bundle gpm has no memory of would never appear in the expiry warnings.
		writeErr(w, recordStatus(err), fmt.Errorf("certificate signed but could not be recorded, so it was not issued: %w", err))
		return
	}

	// Issuance is a credential-minting event, so it is logged like one: who the
	// certificate is for and how long it is good for, never any key material and
	// never the bundle password.
	ev := log.Info().
		Str("clientCA", ca.Name).
		Str("subject", res.Subject).
		Str("serial", res.Serial).
		Time("notBefore", res.NotBefore).
		Time("notAfter", res.NotAfter)
	if supersedes != "" {
		ev = ev.Str("supersedes", supersedes)
	}
	ev.Msg("issued client certificate")

	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Header().Set("Content-Disposition", `attachment; filename="`+res.Filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(res.PKCS12)))
	// The bundle is a freshly minted secret; keep it out of every cache.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Client-Cert-Serial", res.Serial)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.PKCS12)
}

// recordStatus maps a ledger write failure. A vanished supersede target is a
// conflict (something changed the records underneath this request), anything else
// - including an unwired or unwritable certificate store - is a server fault.
func recordStatus(err error) int {
	if errors.Is(err, clientcert.ErrRecordNotFound) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// issueStatus maps an issuance failure onto the status shape the rest of this
// package uses: a malformed request is 400, a verify-only CA is 422 (the request
// is well-formed but this CA can never satisfy it), and a key that is configured
// but unusable is a server-side configuration fault, 500.
func issueStatus(err error) int {
	switch {
	case errors.Is(err, clientcert.ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, model.ErrNoSigningKey):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}
