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
	if f := c.CRLFile; f != "" {
		// Same confinement as a custom certificate's files: reject absolute paths
		// and traversal, OS-agnostically (filepath.IsAbs only sees a leading "/"
		// on Unix, so a Windows build would otherwise accept "/etc/shadow").
		if filepath.IsAbs(f) || strings.HasPrefix(f, "/") || strings.HasPrefix(f, `\`) || strings.Contains(filepath.Clean(f), "..") {
			return fmt.Errorf("client CA %q: crlFile %q must be relative to the cert store (no absolute or .. paths)", c.Name, f)
		}
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
