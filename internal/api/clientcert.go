package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
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
	writeErr(w, http.StatusNotFound, errNotFound("ClientCA", name))
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
			writeErr(w, http.StatusBadRequest, decodeError(err))
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
			writeErr(w, http.StatusBadRequest, decodeError(err))
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

// generateRequest is the POST /client-cas/{name}/generate body.
type generateRequest struct {
	CommonName   string `json:"commonName,omitempty"`
	ValidityDays int    `json:"validityDays,omitempty"`
	Organization string `json:"organization,omitempty"`
}

// handleGenerateClientCA creates a brand-new, self-signed, issuance-capable
// ClientCA from nothing: it generates the key pair, writes the private key into
// the managed certificate store, and saves the ClientCA object pointing at it.
//
// Unlike issue and renew this IS a config mutation, so it goes through the normal
// store save path - validated against the whole object graph, committed, and
// visible in history exactly like a UI PUT of the same object. The response is
// the created object, again like a PUT.
//
// The generated private key is never in a response and never in a log line. It
// exists only as a 0600 file in the cert store, which is also why it is never
// overwritten: that file may still be the signing key behind certificates already
// deployed to devices.
func (d Deps) handleGenerateClientCA(w http.ResponseWriter, r *http.Request) {
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
	var req generateRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, decodeError(err))
			return
		}
	}

	// Validate the request BEFORE anything below touches the certificate store.
	// The reclaim further down is a deletion, so a request that was going to be
	// refused anyway must never reach it: a rejected generate leaves the store
	// byte-for-byte untouched. GenerateCA re-runs this itself, so the invariant
	// does not rest on this call being remembered.
	plan, err := clientcert.ValidatePlan(name, clientcert.GenerateRequest{
		CommonName:   req.CommonName,
		ValidityDays: req.ValidityDays,
		Organization: req.Organization,
	})
	if err != nil {
		writeErr(w, generateStatus(err), err)
		return
	}

	// Refuse before generating anything: a name collision is the common mistake
	// and RSA-4096 keygen is expensive, but more importantly a refusal here
	// leaves nothing on disk to roll back.
	//
	// This snapshot is also what the orphan-key referrer scan below reads. It is
	// taken before the reclaim, so a PUT that starts referencing the orphan in
	// that window would not be seen - accepted: config writes are single-leader
	// admin actions, and the window is one config load.
	cfg, _, err := d.Store.Load(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	for _, ca := range cfg.ClientCAs {
		if ca.Name == name {
			writeErr(w, http.StatusConflict, fmt.Errorf(
				"%w: %s (edit it, or choose another name - generating would replace a trust anchor that hosts may already reference)",
				clientcert.ErrCAExists, name))
			return
		}
	}

	// A key file with no object behind it is the residue of a crash between the
	// key write and the config save (or of a ClientCA someone deleted, since a
	// delete deliberately keeps the key). Refusing it forever would make the name
	// permanently unusable from the UI with no way to fix it, so reclaim it - but
	// only once it is provably referenced by nothing, because that same file may
	// still be the signing key behind certificates already on devices.
	orphan, err := clientcert.KeyFileExists(name, d.CertDir)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if orphan {
		keyRel := clientcert.KeyFileFor(name)
		if ref := clientCAReferencing(cfg, keyRel); ref != "" {
			writeErr(w, http.StatusConflict, fmt.Errorf(
				"%w: %s is the signing key of client CA %q - generating would replace a key that CA is still issuing and signing CRLs with",
				clientcert.ErrKeyFileExists, keyRel, ref))
			return
		}
		if err := clientcert.RemoveGeneratedKey(name, d.CertDir); err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf(
				"an orphaned key file at %s could not be removed: %w", keyRel, err))
			return
		}
		log.Warn().Str("clientCA", name).Str("keyFile", keyRel).
			Msg("reclaimed an orphaned client CA signing key: no ClientCA object referenced it")
	}

	res, err := clientcert.GenerateCA(name, d.CertDir, plan)
	if err != nil {
		writeErr(w, generateStatus(err), err)
		return
	}

	obj := model.ClientCA{
		ObjectMeta: model.ObjectMeta{Name: name},
		CAPEM:      res.CertPEM,
		CAKeyFile:  res.KeyFile,
	}
	sha, err := d.Store.Save(r.Context(), obj, d.author(r))
	if err != nil {
		// The key file landed but the object did not. Remove it, or the
		// no-overwrite rule would block every retry of this same name and leave
		// the operator stuck with no way to fix it from the UI.
		if rmErr := clientcert.RemoveGeneratedKey(name, d.CertDir); rmErr != nil {
			log.Error().Err(rmErr).Str("clientCA", name).Str("keyFile", res.KeyFile).
				Msg("generated CA key could not be removed after a failed config save; remove it by hand before retrying")
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !d.applyChange(w, "save", "ClientCA", name, sha) {
		return
	}

	log.Info().
		Str("clientCA", name).
		Str("subject", res.Subject).
		Str("serial", res.Serial).
		Str("keyFile", res.KeyFile).
		Time("notBefore", res.NotBefore).
		Time("notAfter", res.NotAfter).
		Msg("generated client CA")

	w.Header().Set(commitHeader, sha)
	writeJSON(w, http.StatusOK, obj)
}

// clientCAReferencing returns the name of a ClientCA whose caKeyFile resolves to
// keyRel, or "" when none does. Paths are compared cleaned and slash-normalised
// so "client-cas/corp.key" and "./client-cas/corp.key" are recognised as the same
// file rather than letting a cosmetic difference wave through a key that is
// actually in use.
func clientCAReferencing(cfg model.Config, keyRel string) string {
	want := path.Clean(filepath.ToSlash(keyRel))
	for _, ca := range cfg.ClientCAs {
		if ca.CAKeyFile == "" {
			continue
		}
		if path.Clean(filepath.ToSlash(ca.CAKeyFile)) == want {
			return ca.Name
		}
	}
	return ""
}

// generateStatus maps a generation failure: a bad request is 400, an occupied
// name or key path is 409, anything else (keygen, disk) is a server fault.
func generateStatus(err error) int {
	switch {
	case errors.Is(err, clientcert.ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, clientcert.ErrCAExists), errors.Is(err, clientcert.ErrKeyFileExists):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
