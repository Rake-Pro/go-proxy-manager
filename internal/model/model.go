// Package model defines the first-class, typed configuration objects that the
// whole system is built around: hosts, certificates, identity providers, access
// lists and middleware. Everything else (UI, API, git-backed store, importer,
// data plane) reads and writes these same types, so they cannot diverge.
//
// Design rules:
//   - Stable schema + explicit SchemaVersion so later feature tiers add fields,
//     never force a rewrite.
//   - Every object carries ObjectMeta whose Name is its stable identity and the
//     basename of its YAML file in the git-backed config store.
//   - Secrets are never stored inline; they are placeholders resolved at load.
package model

import (
	"fmt"
	"regexp"
	"time"
)

// SchemaVersion is the current on-disk schema version. Bumping it triggers a
// (reversible, documented) migration in the store layer.
const SchemaVersion = 1

// Config is the fully-assembled in-memory snapshot of all config objects. The
// store loads it from many per-object YAML files and the data plane compiles it.
type Config struct {
	SchemaVersion     int                `json:"schemaVersion" yaml:"schemaVersion"`
	ProxyHosts        []ProxyHost        `json:"proxyHosts,omitempty" yaml:"proxyHosts,omitempty"`
	RedirectHosts     []RedirectHost     `json:"redirectHosts,omitempty" yaml:"redirectHosts,omitempty"`
	StreamHosts       []StreamHost       `json:"streamHosts,omitempty" yaml:"streamHosts,omitempty"`
	DeadHosts         []DeadHost         `json:"deadHosts,omitempty" yaml:"deadHosts,omitempty"`
	Certificates      []Certificate      `json:"certificates,omitempty" yaml:"certificates,omitempty"`
	ClientCAs         []ClientCA         `json:"clientCAs,omitempty" yaml:"clientCAs,omitempty"`
	DNSProviders      []DNSProvider      `json:"dnsProviders,omitempty" yaml:"dnsProviders,omitempty"`
	IdentityProviders []IdentityProvider `json:"identityProviders,omitempty" yaml:"identityProviders,omitempty"`
	UpstreamGroups    []UpstreamGroup    `json:"upstreamGroups,omitempty" yaml:"upstreamGroups,omitempty"`
	AccessLists       []AccessList       `json:"accessLists,omitempty" yaml:"accessLists,omitempty"`
	Middlewares       []Middleware       `json:"middlewares,omitempty" yaml:"middlewares,omitempty"`
	APITokens         []APIToken         `json:"apiTokens,omitempty" yaml:"apiTokens,omitempty"`
}

// Object is implemented by every first-class config type. Kind/Name give each
// object a stable (kind, name) identity used for file paths and cross-references.
type Object interface {
	GetMeta() ObjectMeta
	Kind() string
	Validate() error
}

// ObjectMeta is embedded in every config object.
type ObjectMeta struct {
	// Name is the stable identifier and the basename of the object's YAML file.
	// It must be DNS-label-ish so it is filesystem- and reference-safe.
	Name string `json:"name" yaml:"name"`
	// DisplayName is an optional human label for the UI.
	DisplayName string `json:"displayName,omitempty" yaml:"displayName,omitempty"`
	// Labels are free-form key/value metadata for grouping/filtering.
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	// Tags are flat, free-form labels for grouping/filtering objects in the UI
	// (host grouping/tagging). Order is preserved; duplicates are harmless.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	// Disabled keeps an object in config but excludes it from the compiled data plane.
	Disabled bool `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	// CreatedAt/UpdatedAt are maintained by the store on write.
	CreatedAt time.Time `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

// GetMeta satisfies the shared accessor used by the store/API.
func (m ObjectMeta) GetMeta() ObjectMeta { return m }

var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-_.]{0,251}[a-z0-9])?$`)

// ValidateName enforces a filesystem- and reference-safe object name.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid name %q: must be lowercase alphanumeric with -_. and start/end alphanumeric", name)
	}
	return nil
}
