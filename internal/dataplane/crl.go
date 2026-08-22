package dataplane

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// crlWatchInterval is how often a configured CRL file's mtime is polled, so an
// operator's out-of-band refresh (a CA export cron, a mounted file swap) is
// picked up with no config change or restart. It mirrors the GeoIP database
// watch interval (see internal/geoip.DefaultWatchInterval).
const crlWatchInterval = 5 * time.Minute

// clientCAAnchor is a compiled ClientCA: the verification pool used as
// tls.Config.ClientCAs, the issuer certificates a CRL's signature must validate
// against, and the live revocation state. One anchor is shared by every host
// referencing that ClientCA, so a CRL reload applies to all of them at once.
type clientCAAnchor struct {
	name    string
	pool    *x509.CertPool
	issuers []*x509.Certificate

	// crlConfigured records that this CA asked for revocation checking, which is
	// what makes an unusable CRL an error rather than a no-op. crlFile is the
	// resolved on-disk path ("" for an inline crlPEM, which has no mtime to watch).
	crlConfigured bool
	crlFile       string
	failOpen      bool

	// crl is the last successfully loaded revocation list, or nil when none has
	// ever loaded. Read on every handshake, swapped by the reload path.
	crl atomic.Pointer[revocationList]

	// mu serializes reloads (the config-reload path and the mtime watch can race)
	// and guards modTime.
	mu      sync.Mutex
	modTime time.Time
}

// revocationList is the parsed, signature-verified content of a CRL: the set of
// revoked serial numbers and the list's own expiry.
type revocationList struct {
	revoked    map[string]struct{}
	nextUpdate time.Time
}

// serialKey renders a certificate serial number as the canonical map key used
// for revocation lookups (lower-case hex, no leading zeros).
func serialKey(n *big.Int) string {
	if n == nil {
		return ""
	}
	return n.Text(16)
}

// parseCRL decodes a PEM or DER revocation list and verifies its signature
// against one of the CA bundle's certificates. An unsigned or foreign-signed CRL
// is refused: without that check anyone able to drop a file in the cert store
// could un-revoke (or mass-revoke) certificates.
func parseCRL(raw []byte, issuers []*x509.Certificate) (*revocationList, error) {
	var ders [][]byte
	rest := raw
	for {
		block, more := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "X509 CRL" {
			ders = append(ders, block.Bytes)
		}
		rest = more
	}
	if len(ders) == 0 {
		ders = [][]byte{raw} // no PEM blocks -> treat the file as raw DER
	}

	out := &revocationList{revoked: map[string]struct{}{}}
	for _, der := range ders {
		list, err := x509.ParseRevocationList(der)
		if err != nil {
			return nil, fmt.Errorf("parse CRL: %w", err)
		}
		signed := false
		for _, iss := range issuers {
			if err := list.CheckSignatureFrom(iss); err == nil {
				signed = true
				break
			}
		}
		if !signed {
			return nil, fmt.Errorf("CRL is not signed by any certificate in this CA bundle")
		}
		for _, e := range list.RevokedCertificateEntries {
			out.revoked[serialKey(e.SerialNumber)] = struct{}{}
		}
		// The earliest nextUpdate across a multi-CRL bundle governs, so a stale
		// member cannot be masked by a fresher one.
		if !list.NextUpdate.IsZero() && (out.nextUpdate.IsZero() || list.NextUpdate.Before(out.nextUpdate)) {
			out.nextUpdate = list.NextUpdate
		}
	}
	return out, nil
}

// load reads and installs the anchor's CRL. A failure leaves any previously
// loaded list serving (a truncated file mid-refresh never drops a good CRL) and
// is returned for the caller to log; with nothing ever loaded, verifyPeer then
// applies the fail-closed/fail-open policy.
func (a *clientCAAnchor) load(raw []byte) error {
	list, err := parseCRL(raw, a.issuers)
	if err != nil {
		return err
	}
	a.crl.Store(list)
	return nil
}

// reloadFile re-reads the anchor's CRL file unconditionally (config-reload path).
func (a *clientCAAnchor) reloadFile() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	raw, err := os.ReadFile(a.crlFile)
	if err != nil {
		return err
	}
	if err := a.load(raw); err != nil {
		return err
	}
	if fi, err := os.Stat(a.crlFile); err == nil {
		a.modTime = fi.ModTime()
	}
	return nil
}

// reloadIfChanged re-reads the CRL file when its mtime moved. Called by the
// watch loop; a stat or parse failure keeps the last good list and is logged.
func (a *clientCAAnchor) reloadIfChanged() {
	if a.crlFile == "" {
		return
	}
	a.mu.Lock()
	last := a.modTime
	a.mu.Unlock()
	fi, err := os.Stat(a.crlFile)
	if err != nil {
		log.Warn().Err(err).Str("clientCA", a.name).Str("path", a.crlFile).
			Msg("crl: stat failed; keeping the last loaded revocation list")
		return
	}
	if !fi.ModTime().After(last) {
		return
	}
	if err := a.reloadFile(); err != nil {
		log.Error().Err(err).Str("clientCA", a.name).Str("path", a.crlFile).
			Msg("crl: reload failed; keeping the last loaded revocation list")
		return
	}
	log.Info().Str("clientCA", a.name).Str("path", a.crlFile).Msg("crl: revocation list reloaded")
}

// verifyPeer is wired to tls.Config.VerifyPeerCertificate for any host whose
// ClientCA configures revocation. The stdlib chain verification has already run
// (and, in optional mode with no certificate presented, there is nothing to
// check), so this only adds the revocation decision stdlib Verify does not make.
func (a *clientCAAnchor) verifyPeer(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return nil // optional mode, no client certificate presented
	}
	snap := a.crl.Load()
	if snap == nil {
		if a.failOpen {
			log.Warn().Str("clientCA", a.name).
				Msg("crl: no usable revocation list loaded; accepting the client certificate (crlPolicy fail-open)")
			return nil
		}
		return fmt.Errorf("client CA %q: no usable revocation list loaded (crlPolicy fail-closed)", a.name)
	}
	if !snap.nextUpdate.IsZero() && time.Now().After(snap.nextUpdate) {
		if a.failOpen {
			log.Warn().Str("clientCA", a.name).Time("nextUpdate", snap.nextUpdate).
				Msg("crl: revocation list is past its nextUpdate; accepting the client certificate (crlPolicy fail-open)")
			return nil
		}
		return fmt.Errorf("client CA %q: revocation list expired at %s (crlPolicy fail-closed)", a.name, snap.nextUpdate.Format(time.RFC3339))
	}
	for _, chain := range verifiedChains {
		for _, c := range chain {
			if _, revoked := snap.revoked[serialKey(c.SerialNumber)]; revoked {
				return fmt.Errorf("client CA %q: certificate serial %s is revoked", a.name, serialKey(c.SerialNumber))
			}
		}
	}
	if len(verifiedChains) == 0 {
		// Defence in depth: no verified chain was handed over, so check the leaf
		// the peer actually presented rather than skipping revocation entirely.
		leaf, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("client CA %q: unparseable client certificate: %w", a.name, err)
		}
		if _, revoked := snap.revoked[serialKey(leaf.SerialNumber)]; revoked {
			return fmt.Errorf("client CA %q: certificate serial %s is revoked", a.name, serialKey(leaf.SerialNumber))
		}
	}
	return nil
}

// buildClientCAAnchors resolves each ClientCA object's PEM (inline or via a
// ${FILE:...}/${ENV:...} placeholder) into a verification pool keyed by name,
// and loads its revocation list when one is configured. A CA whose PEM resolves
// but parses to zero certificates is a hard error so a host requiring mTLS never
// compiles against an empty trust anchor (which would reject everyone).
//
// A CRL that cannot be loaded is NOT a build error: that would take the whole
// config reload (every unrelated host with it) down over one unreadable file.
// It is enforced at the handshake instead, where crlPolicy decides - fail-closed
// by default - exactly like the GeoIP database's live fail-closed evaluation.
func buildClientCAAnchors(cas []model.ClientCA, certDir string) (map[string]*clientCAAnchor, error) {
	anchors := map[string]*clientCAAnchor{}
	for _, ca := range cas {
		if ca.Disabled {
			continue
		}
		caPEM, err := model.Secret(ca.CAPEM).Resolve()
		if err != nil {
			return nil, fmt.Errorf("client CA %q: %w", ca.Name, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, fmt.Errorf("client CA %q: caPEM parsed to no certificates", ca.Name)
		}
		a := &clientCAAnchor{
			name:          ca.Name,
			pool:          pool,
			issuers:       parseCertsPEM([]byte(caPEM)),
			crlConfigured: ca.CRLFile != "" || ca.CRLPEM != "",
			failOpen:      ca.CRLPolicy == model.CRLPolicyFailOpen,
		}
		switch {
		case ca.CRLPEM != "":
			if err := a.load([]byte(ca.CRLPEM)); err != nil {
				log.Error().Err(err).Str("clientCA", ca.Name).
					Msg("crl: inline crlPEM is unusable; client certificates for this CA follow crlPolicy")
			}
		case ca.CRLFile != "":
			a.crlFile = resolvePath(ca.CRLFile, certDir)
			if err := a.reloadFile(); err != nil {
				log.Error().Err(err).Str("clientCA", ca.Name).Str("path", a.crlFile).
					Msg("crl: revocation list is unusable; client certificates for this CA follow crlPolicy")
			}
		}
		anchors[ca.Name] = a
	}
	return anchors, nil
}

// parseCertsPEM returns every certificate in a PEM bundle. These are the issuers
// a CRL signature is checked against; x509.CertPool does not expose its own
// certificates in a usable form for CheckSignatureFrom.
func parseCertsPEM(raw []byte) []*x509.Certificate {
	var out []*x509.Certificate
	rest := raw
	for {
		block, more := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = more
		if block.Type != "CERTIFICATE" {
			continue
		}
		if c, err := x509.ParseCertificate(block.Bytes); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// crlAnchors holds the file-backed anchors of the CURRENT router build, so the
// watch loop (started once, outliving every reload) always polls the live set.
var crlAnchors atomic.Pointer[[]*clientCAAnchor]

// registerCRLAnchors publishes the file-backed anchors of a freshly built router
// to the watch loop. Anchors from a superseded build are simply dropped.
func registerCRLAnchors(anchors map[string]*clientCAAnchor) {
	watched := []*clientCAAnchor{}
	for _, a := range anchors {
		if a.crlFile != "" {
			watched = append(watched, a)
		}
	}
	crlAnchors.Store(&watched)
}

// watchCRLs polls every configured CRL file's mtime until ctx is cancelled,
// reloading the ones that changed. Callers run it in its own goroutine.
func watchCRLs(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if p := crlAnchors.Load(); p != nil {
				for _, a := range *p {
					a.reloadIfChanged()
				}
			}
		}
	}
}
