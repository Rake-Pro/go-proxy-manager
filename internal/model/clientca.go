package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ClientCA is the trust anchor for mTLS client-certificate verification: a
// PEM-encoded CA bundle that proxied hosts verify presented client certificates
// against. It is kept distinct from server-side Certificate objects because it
// serves the opposite role (verifying peers, not identifying this server) and it
// never carries a private key.
type ClientCA struct {
	ObjectMeta `json:",inline" yaml:",inline"`

	// CAPEM is the PEM-encoded CA certificate bundle (one or more certificates).
	// A CA certificate is public, so it may be stored inline; it may also be a
	// ${FILE:...} / ${ENV:...} placeholder resolved at load. It is a plain string
	// (not a Secret) so an inline PEM is not treated as a committable secret. The
	// "parses to >=1 certificate" check is deferred to data-plane compile, where a
	// placeholder can actually be resolved.
	CAPEM string `json:"caPEM" yaml:"caPEM"`

	// CRLFile is an optional certificate revocation list for this CA, PEM or DER
	// encoded. Like a custom certificate's files it is confined to the managed
	// cert store: the path must be relative, with no "..", so a config write can
	// never point the loader at an arbitrary host file. The file is re-read on
	// every config reload and on an mtime watch, so an out-of-band CRL refresh
	// applies with no restart.
	CRLFile string `json:"crlFile,omitempty" yaml:"crlFile,omitempty"`

	// CRLPEM is an optional inline PEM-encoded CRL, for a small, rarely-changing
	// revocation list an operator would rather keep in git than mount. Mutually
	// exclusive with CRLFile; an inline CRL has no mtime, so it only changes on a
	// config reload.
	CRLPEM string `json:"crlPEM,omitempty" yaml:"crlPEM,omitempty"`

	// CRLPolicy decides what happens when a CRL is configured but unusable -
	// missing, unparseable, not signed by this CA, or past its nextUpdate:
	// "fail-closed" (default) rejects every client certificate verified against
	// this CA, "fail-open" accepts them and logs. Fail-closed is the default
	// because an operator who configured revocation asked for revocation to be
	// enforced; fail-open exists for a host where availability outranks it.
	CRLPolicy string `json:"crlPolicy,omitempty" yaml:"crlPolicy,omitempty"`

	// CAKeyFile is the OPTIONAL private key of one certificate in the bundle. It
	// is what turns a verify-only trust anchor into an issuing CA: with it set,
	// an operator can mint client certificates from the UI/API (POST
	// /api/client-cas/{name}/issue). Confined to the managed cert store exactly
	// like CRLFile - the path must be relative, with no "..", so a config write
	// can never point the loader at an arbitrary host file. Mutually exclusive
	// with CAKeyPEM. A ClientCA with neither is perfectly valid; it simply cannot
	// issue.
	CAKeyFile string `json:"caKeyFile,omitempty" yaml:"caKeyFile,omitempty"`

	// CAKeyPEM is the same signing key supplied inline. Unlike CAPEM it is a
	// Secret: a CA private key must never be committed in plaintext, so the store
	// refuses a literal value and it is expected to be a ${FILE:...} / ${ENV:...}
	// placeholder resolved at load. Mutually exclusive with CAKeyFile.
	CAKeyPEM Secret `json:"caKeyPEM,omitempty" yaml:"caKeyPEM,omitempty"`

	// ExpiryWarningDays is how far ahead a certificate issued by this CA counts
	// as "expiring" in the issuance list and the admin UI banner. Zero means
	// DefaultExpiryWarningDays. It is a per-CA knob because the right lead time
	// follows the certificate lifetime, which is chosen per CA: a fleet on 90-day
	// certificates needs a shorter warning than one on 10-year certificates.
	//
	// It is advisory only - nothing expires or renews on its own. Renewal is
	// always an operator action, because every device holding the old bundle has
	// to import the new one by hand.
	ExpiryWarningDays int `json:"expiryWarningDays,omitempty" yaml:"expiryWarningDays,omitempty"`
}

// DefaultExpiryWarningDays is the lead time an issued client certificate is
// reported as expiring when a ClientCA sets no expiryWarningDays. It matches the
// ACME renewal window, so both kinds of certificate warn at the same distance.
const DefaultExpiryWarningDays = 30

// MaxExpiryWarningDays bounds the knob at the maximum certificate lifetime: a
// longer window than any certificate can live would mark every certificate
// expiring from the moment it is issued.
const MaxExpiryWarningDays = 3650

// WarningDays returns the effective expiry warning lead time for this CA.
func (c ClientCA) WarningDays() int {
	if c.ExpiryWarningDays <= 0 {
		return DefaultExpiryWarningDays
	}
	return c.ExpiryWarningDays
}

// CRL policy values for ClientCA.CRLPolicy.
const (
	CRLPolicyFailClosed = "fail-closed"
	CRLPolicyFailOpen   = "fail-open"
)

func (c ClientCA) Kind() string { return "ClientCA" }

func (c ClientCA) Validate() error {
	if err := ValidateName(c.Name); err != nil {
		return err
	}
	if len(c.CAPEM) == 0 {
		return fmt.Errorf("client CA %q: caPEM is required", c.Name)
	}
	if c.CRLFile != "" && c.CRLPEM != "" {
		return fmt.Errorf("client CA %q: set crlFile or crlPEM, not both", c.Name)
	}
	if err := confinedStorePath("crlFile", c.CRLFile); err != nil {
		return fmt.Errorf("client CA %q: %w", c.Name, err)
	}
	if c.CAKeyFile != "" && !c.CAKeyPEM.IsEmpty() {
		return fmt.Errorf("client CA %q: set caKeyFile or caKeyPEM, not both", c.Name)
	}
	if err := confinedStorePath("caKeyFile", c.CAKeyFile); err != nil {
		return fmt.Errorf("client CA %q: %w", c.Name, err)
	}
	if err := c.validateInlineSigningKey(); err != nil {
		return err
	}
	if d := c.ExpiryWarningDays; d < 0 || d > MaxExpiryWarningDays {
		return fmt.Errorf("client CA %q: expiryWarningDays must be between 0 (default %d) and %d, got %d",
			c.Name, DefaultExpiryWarningDays, MaxExpiryWarningDays, d)
	}
	switch c.CRLPolicy {
	case "", CRLPolicyFailClosed, CRLPolicyFailOpen:
	default:
		return fmt.Errorf("client CA %q: crlPolicy must be %q or %q, got %q", c.Name, CRLPolicyFailClosed, CRLPolicyFailOpen, c.CRLPolicy)
	}
	if c.CRLPolicy != "" && c.CRLFile == "" && c.CRLPEM == "" {
		return fmt.Errorf("client CA %q: crlPolicy is set but no crlFile or crlPEM is configured", c.Name)
	}
	return nil
}

// confinedStorePath is the shared cert-store confinement check for a ClientCA's
// file references (crlFile, caKeyFile): the path must be relative with no
// traversal, so a config write can never point the loader at an arbitrary host
// file. Checked OS-agnostically - filepath.IsAbs only sees a leading "/" on
// Unix, so a Windows build would otherwise accept "/etc/shadow".
func confinedStorePath(field, p string) error {
	if p == "" {
		return nil
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || strings.Contains(filepath.Clean(p), "..") {
		return fmt.Errorf("%s %q must be relative to the cert store (no absolute or .. paths)", field, p)
	}
	return nil
}
