package model

import "fmt"

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
}

func (c ClientCA) Kind() string { return "ClientCA" }

func (c ClientCA) Validate() error {
	if err := ValidateName(c.Name); err != nil {
		return err
	}
	if len(c.CAPEM) == 0 {
		return fmt.Errorf("client CA %q: caPEM is required", c.Name)
	}
	return nil
}
