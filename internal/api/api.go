// Package api exposes a stdlib-only REST surface for CRUD over the typed config
// objects. Every write goes through the git-backed store, which validates the
// whole object graph and commits, so the API never bypasses referential
// integrity or history. The handler is an *http.ServeMux registered without an
// "/api" prefix; the caller mounts it under /api/.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/accesssync"
	"github.com/Rake-Pro/go-proxy-manager/internal/acme"
	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/clientcert"
	"github.com/Rake-Pro/go-proxy-manager/internal/dnssync"
	"github.com/Rake-Pro/go-proxy-manager/internal/docker"
	"github.com/Rake-Pro/go-proxy-manager/internal/ha"
	"github.com/Rake-Pro/go-proxy-manager/internal/k8s"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

const (
	maxBody        = 1 << 20 // 1MB request body cap
	maxArchiveBody = 8 << 20 // 8MB cap for an uploaded restore archive (gzipped)
	commitHeader   = "X-Config-Commit"
	contentTypeJS  = "application/json"
)

// Deps wires the API to the store and the rest of the app.
type Deps struct {
	Store *store.Store
	// OnChange applies the just-committed config to the running state (reload). It
	// returns an error when the write committed but could not take effect (e.g. a
	// geo rule saved while no GeoIP database is loaded), so the mutation response
	// surfaces the failure instead of a silent 200. May be nil.
	OnChange func() error
	Author   func(*http.Request) store.Author // derives the commit author; if nil, a zero Author is used
	// OnEvent, if set, is fired after every successful write with the change
	// details (action/kind/name/commit), for lifecycle webhooks. May be nil.
	OnEvent func(action, kind, name, commit string)
	// SetAccessLog, if set, flips data-plane request capture live. The change
	// is runtime-only: the -access-log flag decides the state after a restart.
	SetAccessLog func(enabled bool)
	// RecentLogs, if set, returns the data-plane access-log viewer payload
	// (marshalled as-is) for GET /logs. May be nil.
	RecentLogs func() any
	// GeoDBLoaded reports whether a GeoIP database is currently loaded, for the
	// read-only GET /capabilities probe the UI uses to enable/grey-out geo
	// controls. May be nil (reported as not loaded).
	GeoDBLoaded func() bool
	// MaintenanceGlobal, if set, reports whether the fleet-wide maintenance
	// switch (settings.maintenance.enabled) is currently on, for the capability
	// probe. May be nil (reported off).
	MaintenanceGlobal func() bool
	// UpstreamHealth, if set, returns the live per-group upstream health payload
	// (marshalled as-is) for GET /upstream-health. May be nil.
	UpstreamHealth func() any
	// UpstreamGroupSummary, if set, returns the healthy/unhealthy upstream count
	// per group for GET /health. May be nil (reported as an empty list). Kept
	// separate from UpstreamHealth (which the daemon derives it from) so this
	// package never has to import internal/dataplane's per-upstream type.
	UpstreamGroupSummary func() []UpstreamGroupHealth
	// ACMEObservations, if set, returns the ACME manager's per-certificate
	// expiry, failure count and last error, for the status GET /certificates
	// decorates onto each object and for GET /health. May be nil (a follower:
	// only the leader runs the ACME manager, so it is not the issuer and has
	// nothing to report).
	ACMEObservations func() []acme.CertObservation
	// ACMERenewNow, if set, forces an immediate ACME order for one certificate
	// (POST /certificates/{name}/renew), ignoring the normal renewal window. It
	// returns acme.ErrCertNotFound, acme.ErrNotACME or acme.ErrRenewInFlight for
	// the handler to map onto the right HTTP status. May be nil (endpoint
	// responds 501; true on a follower, which does not run the ACME manager).
	ACMERenewNow func(ctx context.Context, cfg model.Config, name string) error
	// ACMELastRun, if set, returns when the ACME renewal loop last ran, for
	// GET /health. May be nil (reported as never run).
	ACMELastRun func() time.Time
	// DataPlaneListening, if set, reports whether the HTTPS and HTTP data-plane
	// listeners are currently bound, for GET /health. May be nil (both reported
	// not listening).
	DataPlaneListening func() (httpsListening, httpListening bool)
	// RevokeSSOSessions, if set, invalidates every outstanding data-plane SSO
	// session (POST /sso/revoke). May be nil (endpoint responds 501).
	RevokeSSOSessions func() error
	// RequireScope, if set, gates every route on the calling principal's API
	// token scopes, returning a non-nil error to refuse (403). It is injected
	// rather than imported so the API stays testable unauthenticated.
	//
	// A nil RequireScope means "allow every route". That is the UNWIRED case
	// only - tests, and any embedder that mounts this mux without API tokens.
	// The daemon always wires it, and its implementation denies when there is no
	// principal on the request, so the fail-open path is unreachable in
	// production (see cmd/gpm/main.go).
	RequireScope func(r *http.Request, required string) error
	// TokenLastUsed, if set, returns the in-memory last-use timestamp per API
	// token name, decorated onto GET /api-tokens responses. May be nil.
	TokenLastUsed func() map[string]time.Time
	// DNSSyncReconcile, if set, runs a full DNS reconcile synchronously
	// (POST /dns-sync/reconcile). May be nil (endpoint responds 501).
	DNSSyncReconcile func(context.Context) error
	// DNSSyncStatus, if set, returns the last reconcile result (marshalled
	// as-is) for GET /dns-sync/status. May be nil (endpoint responds 501).
	DNSSyncStatus func() any
	// DNSSyncPlan, if set, returns the read-only preview of what a reconcile
	// WOULD create, adopt and delete per backend (GET /dns-sync/plan), without
	// changing anything. May be nil (endpoint responds 501).
	DNSSyncPlan func(context.Context) (any, error)
	// DNSSyncEnabled, if set, reports which DNS sync backends are configured,
	// for the capability probe. May be nil (both reported disabled).
	DNSSyncEnabled func() (pihole, cloudflare bool)
	// IngressDiscoveryReconcile, if set, runs a full Ingress-discovery reconcile
	// synchronously (POST /ingress-discovery/reconcile). May be nil (501).
	IngressDiscoveryReconcile func(context.Context) error
	// IngressDiscoveryStatus, if set, returns the last reconcile result
	// (marshalled as-is) for GET /ingress-discovery/status. May be nil (501).
	IngressDiscoveryStatus func() any
	// IngressDiscoveryPlan, if set, returns the read-only preview of what a
	// reconcile WOULD create, update, delete and skip (GET
	// /ingress-discovery/plan), without changing anything. Mirrors DNSSyncPlan.
	// May be nil (501).
	IngressDiscoveryPlan func(context.Context) (any, error)
	// DockerDiscoveryReconcile, if set, runs a full Docker-discovery reconcile
	// synchronously (POST /docker-discovery/reconcile). May be nil (501).
	DockerDiscoveryReconcile func(context.Context) error
	// DockerDiscoveryStatus, if set, returns the last reconcile result
	// (marshalled as-is) for GET /docker-discovery/status. May be nil (501).
	DockerDiscoveryStatus func() any
	// DockerDiscoveryPlan, if set, returns the read-only preview of what a
	// reconcile WOULD create, update, delete and skip (GET
	// /docker-discovery/plan), without changing anything. May be nil (501).
	DockerDiscoveryPlan func(context.Context) (any, error)
	// DockerDiscoveryEnabled, if set, reports whether container discovery is
	// configured, for the capability probe. May be nil (reported disabled).
	DockerDiscoveryEnabled func() bool
	// AccessListSourceStatus, if set, returns the last access-list source fetch
	// result (marshalled as-is) for GET /access-list-sources/status. May be nil (501).
	AccessListSourceStatus func() any
	// AccessListSourceReconcile, if set, fetches every due access-list source
	// synchronously (POST /access-list-sources/reconcile). May be nil (501).
	AccessListSourceReconcile func(context.Context) error
	// IngressDiscoveryEnabled, if set, reports whether Ingress discovery is
	// configured, for the capability probe. May be nil (reported disabled).
	IngressDiscoveryEnabled func() bool
	// MetricsEnabled reports whether the Prometheus exposition is mounted at
	// /metrics on the admin listener, for the capability probe. The endpoint
	// itself lives in internal/server (it is not an /api/ route), so the API
	// only ever reports on it.
	MetricsEnabled bool
	// Role is the static HA role of this instance (docs/design/ha.md phase 1).
	// A follower refuses every config write with 503 and reports itself
	// read-only in the capability probe; reads are unaffected. The zero value is
	// the leader, i.e. today's single-node behaviour.
	Role ha.Role
	// CertDir is the managed certificate store. The API needs it only to resolve
	// a ClientCA's cert-store-relative caKeyFile when issuing a client
	// certificate; every other path in this package is store-relative.
	CertDir string
	// Runtime carries the flag/env-derived startup facts reported by
	// GET /runtime. The daemon fills it once; the handler never reads the
	// environment itself, so the probe describes THIS process.
	Runtime RuntimeConfig
	// AccessLogEnabled, if set, reports whether data-plane request capture is
	// currently on (it is a live toggle, see PUT /logs). May be nil (off).
	AccessLogEnabled func() bool
	// WebhookStatus, if set, returns the per-target last-delivery state
	// (marshalled as-is) for GET /webhooks/status. May be nil (empty list).
	WebhookStatus func() any
	// WebhookTest, if set, POSTs a synthetic test event to one webhook target
	// and waits for the outcome (POST /webhooks/{name}/test). It returns
	// webhook.ErrUnknownTarget for a name that is not configured; a refused or
	// timed-out delivery is reported in the result, not as an error. May be nil
	// (endpoint responds 501).
	WebhookTest func(ctx context.Context, name string) (any, error)
	// NotificationStatus, if set, returns the per-target last-delivery state
	// (marshalled as-is) for GET /notifications/status. May be nil (empty list).
	NotificationStatus func() any
	// NotificationTest, if set, sends a synthetic event to one notification
	// target and waits for the outcome (POST /notifications/{name}/test). It
	// returns notify.ErrUnknownTarget for a name that is not configured; a
	// refused or timed-out delivery is reported in the result, not as an
	// error. May be nil (endpoint responds 501).
	NotificationTest func(ctx context.Context, name string) (any, error)
	// NoAdminLogin reports the bootstrap failure state: no usable local admin
	// credential AND no admin SSO provider that can render a sign-in button, so
	// the login page cannot succeed for anyone. Surfaced in the capability probe
	// (and on the login page itself) instead of only in one startup log line.
	NoAdminLogin func() bool

	// CookieSecureState reports how the admin session cookie was decided for THIS
	// request: "secure", "insecure-private" or "insecure-public" (see
	// internal/auth). The SPA turns the last one into a banner. Empty when the
	// daemon did not wire it.
	CookieSecureState func(*http.Request) string
}

// capabilities is the read-only runtime feature-availability payload returned by
// GET /capabilities. It is intentionally an object-of-objects so new capability
// groups can be added without breaking existing clients.
type capabilities struct {
	GeoIP            geoIPCapability            `json:"geoip"`
	APITokens        apiTokenCapability         `json:"apiTokens"`
	DNSSync          dnsSyncCapability          `json:"dnsSync"`
	IngressDiscovery ingressDiscoveryCapability `json:"ingressDiscovery"`
	DockerDiscovery  dockerDiscoveryCapability  `json:"dockerDiscovery"`
	HA               haCapability               `json:"ha"`
	Metrics          metricsCapability          `json:"metrics"`
	Maintenance      maintenanceCapability      `json:"maintenance"`
	AdminLogin       adminLoginCapability       `json:"adminLogin"`
	// ScopeSubjects is model.ScopePlurals, served so the SPA renders the token
	// form from the authoritative list instead of a hand-maintained copy. The
	// copy drifted the moment ingress-discovery was added, granting the UI no
	// way to mint a token for it.
	ScopeSubjects []string `json:"scopeSubjects"`
}

type geoIPCapability struct {
	// DBLoaded is true when a GeoIP database is loaded and geo access-list rules
	// can be evaluated (and saved). When false the UI should grey out geo controls.
	DBLoaded bool `json:"dbLoaded"`
}

type apiTokenCapability struct {
	// Enabled reports that scoped API tokens can be minted and used. It is true
	// whenever the daemon wired a token source; the SPA uses it to show the
	// API Tokens page rather than offering a page that cannot work.
	Enabled bool `json:"enabled"`
}

type dnsSyncCapability struct {
	PiholeEnabled     bool `json:"piholeEnabled"`
	CloudflareEnabled bool `json:"cloudflareEnabled"`
}

type ingressDiscoveryCapability struct {
	// Enabled reports that Kubernetes Ingress discovery is wired AND turned on in
	// settings. The SPA uses it to show the status panel rather than offering a
	// control that cannot work.
	Enabled bool `json:"enabled"`
}

type dockerDiscoveryCapability struct {
	// Enabled reports that Docker container discovery is wired AND turned on in
	// settings. The SPA uses it to show the status panel rather than offering a
	// control that cannot work.
	Enabled bool `json:"enabled"`
}

type metricsCapability struct {
	// Enabled reports that GET /metrics is mounted on the admin listener
	// (-metrics / GPM_METRICS=1). The SPA greys the settings-page link out when
	// false rather than linking at a route that answers 404.
	Enabled bool `json:"enabled"`
}

type maintenanceCapability struct {
	// GlobalEnabled reports that settings.maintenance.enabled is on, i.e. EVERY
	// proxy host is currently serving the maintenance page whatever its own
	// maintenance flag says. The SPA uses it to say so on the host editor rather
	// than showing a per-host toggle that appears to be off while the host is in
	// fact down.
	GlobalEnabled bool `json:"globalEnabled"`
}

type adminLoginCapability struct {
	// Configured is false in the bootstrap failure state: no local admin
	// credential AND no admin SSO provider that renders a sign-in button, so the
	// login page cannot succeed for anyone. It used to be visible only as a
	// single warn line in the daemon's startup log.
	Configured bool `json:"configured"`
	// TOTP reports that the local admin must present a TOTP code after their
	// password (GPM_LOCAL_ADMIN_TOTP_SECRET is set). It says nothing about SSO
	// logins, whose MFA belongs to the IdP.
	TOTP bool `json:"totp"`
	// CookieSecure says how the admin session cookie was issued for this request:
	// "secure", "insecure-private" (plain HTTP from loopback or an RFC 1918 / ULA
	// address - the ordinary bootstrap case) or "insecure-public" (plain HTTP
	// from a routable address, which the SPA flags). Empty when unwired.
	CookieSecure string `json:"cookieSecure,omitempty"`
}

type haCapability struct {
	// Role is "leader" or "follower". ReadOnly is true on a follower: the SPA
	// greys out every write control rather than accepting a change the API will
	// refuse (see internal/ui/static/app.js).
	Role     string `json:"role"`
	ReadOnly bool   `json:"readOnly"`
}

func (d Deps) author(r *http.Request) store.Author {
	if d.Author != nil {
		return d.Author(r)
	}
	return store.Author{}
}

// onChange applies the committed config to the running state. A non-nil error
// means the change was committed but could not be applied, which callers surface
// to the client rather than reporting a misleading success.
func (d Deps) onChange() error {
	if d.OnChange != nil {
		return d.OnChange()
	}
	return nil
}

func (d Deps) onEvent(action, kind, name, commit string) {
	if d.OnEvent != nil {
		d.OnEvent(action, kind, name, commit)
	}
}

// scope enforces the API-token scope required by a route, responding 403 and
// returning false when the caller lacks it. With no RequireScope wired (tests,
// and any caller that is not an API token) it always allows, so session
// principals keep the unchanged full-admin access they have always had.
func (d Deps) scope(w http.ResponseWriter, r *http.Request, required string) bool {
	if d.RequireScope == nil {
		return true
	}
	if err := d.RequireScope(r, required); err != nil {
		writeErr(w, http.StatusForbidden, err)
		return false
	}
	return true
}

// allows reports whether the caller holds required, without writing a response.
// It exists for payloads that must be NARROWED for a caller rather than refused
// outright (see GET /config). With no RequireScope wired it allows, exactly like
// scope() above.
func (d Deps) allows(r *http.Request, required string) bool {
	if d.RequireScope == nil {
		return true
	}
	return d.RequireScope(r, required) == nil
}

// scoped wraps h in the scope gate for required.
func (d Deps) scoped(required string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !d.scope(w, r, required) {
			return
		}
		h(w, r)
	}
}

// applyChange reloads the running state after a committed write. When the reload
// fails the write is durably committed but not live (e.g. a geo rule saved while
// no GeoIP database is loaded), so it responds 500 with a message saying exactly
// that and returns false to stop the handler - never a misleading 200. On success
// it fires the lifecycle event and returns true.
func (d Deps) applyChange(w http.ResponseWriter, action, kind, name, sha string) bool {
	if err := d.onChange(); err != nil {
		writeErr(w, http.StatusInternalServerError,
			fmt.Errorf("change committed as %s but could not be applied to the running configuration: %w", sha, err))
		return false
	}
	d.onEvent(action, kind, name, sha)
	return true
}

// New returns an http.Handler with all API routes registered without any "/api"
// prefix.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()

	// One shared ledger for the client-certificate issuance records: its mutex is
	// what serializes the read-modify-write that appending and superseding do.
	clientCertLedger := clientcert.NewLedger(d.CertDir)

	register(mux, d, "proxy-hosts", resource[model.ProxyHost]{
		kind: "ProxyHost",
		list: func(c model.Config) []model.ProxyHost { return c.ProxyHosts },
		decode: func(b []byte, name string) (model.ProxyHost, error) {
			var v model.ProxyHost
			if err := json.Unmarshal(b, &v); err != nil {
				return v, err
			}
			v.Name = name
			// An inline `auth` block in `mode: basic`, on the host or on any of
			// its locations, may carry plaintext passwords the stored object has
			// no field for; hash them here on exactly the terms a `type: auth`
			// middleware write gets.
			return v, applyHostBasicAuthPasswords(b, &v)
		},
	})
	register(mux, d, "redirect-hosts", resource[model.RedirectHost]{
		kind: "RedirectHost",
		list: func(c model.Config) []model.RedirectHost { return c.RedirectHosts },
		decode: func(b []byte, name string) (model.RedirectHost, error) {
			var v model.RedirectHost
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
	})
	register(mux, d, "stream-hosts", resource[model.StreamHost]{
		kind: "StreamHost",
		list: func(c model.Config) []model.StreamHost { return c.StreamHosts },
		decode: func(b []byte, name string) (model.StreamHost, error) {
			var v model.StreamHost
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
	})
	register(mux, d, "parked-hosts", resource[model.ParkedHost]{
		kind: "ParkedHost",
		list: func(c model.Config) []model.ParkedHost { return c.ParkedHosts },
		decode: func(b []byte, name string) (model.ParkedHost, error) {
			var v model.ParkedHost
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
	})
	register(mux, d, "certificates", resource[model.Certificate]{
		kind: "Certificate",
		list: func(c model.Config) []model.Certificate { return c.Certificates },
		decode: func(b []byte, name string) (model.Certificate, error) {
			var v model.Certificate
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
		// decorate attaches the read-only expiry/issuer/renewal status GET
		// /certificates and GET /certificates/{name} report (see certhealth.go).
		// It is runtime state derived from the cert store on disk, never part of
		// the stored object.
		decorate: d.decorateCertificate,
	})
	// Force an immediate ACME order for one certificate, bypassing the normal
	// 30-day renewal window. See certhealth.go.
	mux.HandleFunc("POST /certificates/{name}/renew", d.scoped("certificates:write", d.handleRenewCertificate))

	register(mux, d, "client-cas", resource[model.ClientCA]{
		kind: "ClientCA",
		list: func(c model.Config) []model.ClientCA { return c.ClientCAs },
		decode: func(b []byte, name string) (model.ClientCA, error) {
			var v model.ClientCA
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
	})
	// Client-certificate issuance. It is a POST under the client-cas subtree and
	// is gated exactly like that resource's other mutating routes (PUT, DELETE,
	// scoped revert): "client-cas:write" for a token principal, the admin session
	// gate + CSRF + same-origin guard for everyone else. Using the CA's private
	// key to mint a credential is at least as privileged as editing the object.
	//
	// It changes nothing in the config, so unlike every other write here it makes
	// no commit and produces no history entry - the response IS the artifact.
	mux.HandleFunc("POST /client-cas/{name}/issue", d.scoped("client-cas:write", d.handleIssueClientCert(clientCertLedger)))

	// Create a self-signed, issuance-capable ClientCA from nothing, so a working
	// mTLS setup needs no openssl and no hand-placed key file. Unlike issue and
	// renew this DOES mutate config: it saves the ClientCA object through the
	// normal store path, so it commits and shows up in history like a PUT.
	mux.HandleFunc("POST /client-cas/{name}/generate", d.scoped("client-cas:write", d.handleGenerateClientCA))

	// Issued-certificate records (runtime state, never config): what this CA has
	// issued, each with a derived ok/expiring/expired status, and the renewal that
	// reissues one of them under the same identity with a new key and serial.
	mux.HandleFunc("GET /client-cas/{name}/certificates", d.scoped("client-cas:read", d.handleListClientCerts(clientCertLedger)))
	mux.HandleFunc("POST /client-cas/{name}/certificates/{serial}/renew", d.scoped("client-cas:write", d.handleRenewClientCert(clientCertLedger)))

	// Aggregate runtime health: data-plane listeners, certificate expiry counts,
	// upstream-group health, ACME renewal loop status, HA role and the current
	// config HEAD. See certhealth.go. Same "*:read" scope as GET /upstream-health
	// and GET /config: no secret is in the payload, only counts and booleans an
	// operator already sees spread across other pages.
	mux.HandleFunc("GET /health", d.scoped("*:read", d.handleHealth))

	register(mux, d, "dns-providers", resource[model.DNSProvider]{
		kind: "DNSProvider",
		list: func(c model.Config) []model.DNSProvider { return c.DNSProviders },
		decode: func(b []byte, name string) (model.DNSProvider, error) {
			var v model.DNSProvider
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
	})
	register(mux, d, "identity-providers", resource[model.IdentityProvider]{
		kind: "IdentityProvider",
		list: func(c model.Config) []model.IdentityProvider { return c.IdentityProviders },
		decode: func(b []byte, name string) (model.IdentityProvider, error) {
			var v model.IdentityProvider
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
	})
	register(mux, d, "upstream-groups", resource[model.UpstreamGroup]{
		kind: "UpstreamGroup",
		list: func(c model.Config) []model.UpstreamGroup { return c.UpstreamGroups },
		decode: func(b []byte, name string) (model.UpstreamGroup, error) {
			var v model.UpstreamGroup
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
	})
	register(mux, d, "access-lists", resource[model.AccessList]{
		kind: "AccessList",
		list: func(c model.Config) []model.AccessList { return c.AccessLists },
		decode: func(b []byte, name string) (model.AccessList, error) {
			var v model.AccessList
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
		},
	})
	// One-shot migration off the deprecated AccessList.basicAuth/satisfyAny: it
	// creates an auth middleware with mode basic from the list's users, attaches
	// it wherever the list is referenced, and clears the fields - in a single
	// commit. "?plan=1" is the dry run. Admin-scoped because one call rewrites
	// access lists, middlewares and proxy hosts together (see the handler).
	mux.HandleFunc("POST /access-lists/{name}/migrate-basic-auth", d.scoped(model.ScopeAdmin, d.handleMigrateBasicAuth))

	register(mux, d, "middlewares", resource[model.Middleware]{
		kind: "Middleware",
		list: func(c model.Config) []model.Middleware { return c.Middlewares },
		decode: func(b []byte, name string) (model.Middleware, error) {
			var v model.Middleware
			if err := json.Unmarshal(b, &v); err != nil {
				return v, err
			}
			v.Name = name
			// A `mode: basic` write may carry plaintext passwords the stored
			// object has no field for; hash them here so only passwordHash is
			// ever validated, committed or echoed back.
			return v, applyBasicAuthPasswords(b, v.Auth)
		},
	})

	// Scoped API tokens. Every route here requires the "admin" scope for a token
	// principal (an admin SESSION is unaffected): a token that could create or
	// edit tokens could mint itself a wider scope, so token management is never
	// reachable through a per-resource scope.
	register(mux, d, "api-tokens", resource[model.APIToken]{
		kind:       "APIToken",
		readScope:  model.ScopeAdmin,
		writeScope: model.ScopeAdmin,
		list:       func(c model.Config) []model.APIToken { return c.APITokens },
		decode: func(b []byte, name string) (model.APIToken, error) {
			var v model.APIToken
			err := json.Unmarshal(b, &v)
			v.Name = name
			// TokenHash is server-owned: never accept one from the client, or a
			// caller could install a digest whose preimage only they know. The
			// field is json:"-" so it is already unreachable from the wire; this
			// is the belt that survives that tag ever being loosened.
			v.TokenHash = ""
			return v, err
		},
		beforeSave: mintTokenSecret,
		decorate:   d.decorateToken,
	})

	// Read-only runtime capability probe: lets the SPA discover which optional
	// features are currently usable (e.g. geo rules need a loaded GeoIP DB). Auth
	// is whatever the admin server applies to every other GET route on this mux.
	// It is deliberately NOT scope-gated: any authenticated caller (session or
	// token, whatever its scopes) may ask what this instance can do.
	mux.HandleFunc("GET /capabilities", func(w http.ResponseWriter, r *http.Request) {
		var pihole, cloudflare bool
		if d.DNSSyncEnabled != nil {
			pihole, cloudflare = d.DNSSyncEnabled()
		}
		var cookieSecure string
		if d.CookieSecureState != nil {
			cookieSecure = d.CookieSecureState(r)
		}
		writeJSON(w, http.StatusOK, capabilities{
			GeoIP:     geoIPCapability{DBLoaded: d.GeoDBLoaded != nil && d.GeoDBLoaded()},
			APITokens: apiTokenCapability{Enabled: d.RequireScope != nil},
			DNSSync:   dnsSyncCapability{PiholeEnabled: pihole, CloudflareEnabled: cloudflare},
			IngressDiscovery: ingressDiscoveryCapability{
				Enabled: d.IngressDiscoveryEnabled != nil && d.IngressDiscoveryEnabled(),
			},
			DockerDiscovery: dockerDiscoveryCapability{
				Enabled: d.DockerDiscoveryEnabled != nil && d.DockerDiscoveryEnabled(),
			},
			HA:          haCapability{Role: d.Role.String(), ReadOnly: d.Role.IsFollower()},
			Metrics:     metricsCapability{Enabled: d.MetricsEnabled},
			Maintenance: maintenanceCapability{GlobalEnabled: d.MaintenanceGlobal != nil && d.MaintenanceGlobal()},
			AdminLogin: adminLoginCapability{
				Configured:   d.NoAdminLogin == nil || !d.NoAdminLogin(),
				TOTP:         d.Runtime.LocalAdminTOTP,
				CookieSecure: cookieSecure,
			},
			ScopeSubjects: model.ScopePlurals,
		})
	})

	// DNS sync: reconcile every opted-in proxy host's domains into the configured
	// local (Pi-hole) and public (Cloudflare) DNS backends, and report the last run.
	mux.HandleFunc("POST /dns-sync/reconcile", d.scoped("dns-sync:write", func(w http.ResponseWriter, r *http.Request) {
		if d.DNSSyncReconcile == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("DNS sync is not wired"))
			return
		}
		if err := d.DNSSyncReconcile(r.Context()); err != nil {
			// A run already in flight is a conflict, not a backend failure: the
			// manual endpoint deliberately never queues behind one, so repeated
			// clicks cannot pile blocked goroutines up behind a slow backend.
			if errors.Is(err, dnssync.ErrReconcileInProgress) {
				writeErr(w, http.StatusConflict, err)
				return
			}
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		if d.DNSSyncStatus != nil {
			writeJSON(w, http.StatusOK, d.DNSSyncStatus())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "reconciled"})
	}))
	mux.HandleFunc("GET /dns-sync/status", d.scoped("dns-sync:read", func(w http.ResponseWriter, r *http.Request) {
		if d.DNSSyncStatus == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("DNS sync is not wired"))
			return
		}
		writeJSON(w, http.StatusOK, d.DNSSyncStatus())
	}))
	// Dry run. It reads the backends and the ownership ledger and reports what a
	// reconcile WOULD do, changing nothing - so enabling a backend on a resolver
	// that already holds hand-written records can be checked before it is done, not
	// discovered afterwards. It is a read, so it takes dns-sync:read.
	mux.HandleFunc("GET /dns-sync/plan", d.scoped("dns-sync:read", func(w http.ResponseWriter, r *http.Request) {
		if d.DNSSyncPlan == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("DNS sync is not wired"))
			return
		}
		plan, err := d.DNSSyncPlan(r.Context())
		if err != nil {
			// Same reasoning as the reconcile route: a run in flight is a conflict,
			// and a preview of a moving target is worth less than an honest 409.
			if errors.Is(err, dnssync.ErrReconcileInProgress) {
				writeErr(w, http.StatusConflict, err)
				return
			}
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, plan)
	}))

	// Access-list remote sources: report what each declared source last resolved
	// to, and fetch the due ones now. Both are scoped on access-lists rather than
	// a subject of their own - a source is part of an access list, and a token
	// that may already read or rewrite the list gains nothing new here.
	mux.HandleFunc("GET /access-list-sources/status", d.scoped("access-lists:read", func(w http.ResponseWriter, r *http.Request) {
		if d.AccessListSourceStatus == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("access-list source sync is not wired"))
			return
		}
		writeJSON(w, http.StatusOK, d.AccessListSourceStatus())
	}))
	mux.HandleFunc("POST /access-list-sources/reconcile", d.scoped("access-lists:write", func(w http.ResponseWriter, r *http.Request) {
		if d.AccessListSourceReconcile == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("access-list source sync is not wired"))
			return
		}
		if err := d.AccessListSourceReconcile(r.Context()); err != nil {
			// Same reasoning as the DNS and Ingress reconcile routes: a run already
			// in flight is a conflict, not a backend failure.
			if errors.Is(err, accesssync.ErrReconcileInProgress) {
				writeErr(w, http.StatusConflict, err)
				return
			}
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		if d.AccessListSourceStatus != nil {
			writeJSON(w, http.StatusOK, d.AccessListSourceStatus())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "reconciled"})
	}))

	// Kubernetes Ingress discovery: reconcile annotated cluster Ingresses into
	// template-derived, managed-labelled proxy hosts (which then feed DNS sync),
	// and report the last run. Writes are ownership-gated and the cluster read is
	// strictly read-only; see docs/design/ingress-discovery.md.
	mux.HandleFunc("POST /ingress-discovery/reconcile", d.scoped("ingress-discovery:write", func(w http.ResponseWriter, r *http.Request) {
		if d.IngressDiscoveryReconcile == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("ingress discovery is not wired"))
			return
		}
		if err := d.IngressDiscoveryReconcile(r.Context()); err != nil {
			// Same reasoning as the DNS reconcile: an in-flight run is a conflict,
			// not a backend failure, and the manual endpoint never queues behind one.
			if errors.Is(err, k8s.ErrReconcileInProgress) {
				writeErr(w, http.StatusConflict, err)
				return
			}
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		if d.IngressDiscoveryStatus != nil {
			writeJSON(w, http.StatusOK, d.IngressDiscoveryStatus())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "reconciled"})
	}))
	mux.HandleFunc("GET /ingress-discovery/status", d.scoped("ingress-discovery:read", func(w http.ResponseWriter, r *http.Request) {
		if d.IngressDiscoveryStatus == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("ingress discovery is not wired"))
			return
		}
		writeJSON(w, http.StatusOK, d.IngressDiscoveryStatus())
	}))
	// Dry run, mirroring GET /dns-sync/plan exactly: the same per-host decisions
	// Reconcile would take, computed without writing anything. It is a read, so
	// it takes ingress-discovery:read.
	mux.HandleFunc("GET /ingress-discovery/plan", d.scoped("ingress-discovery:read", func(w http.ResponseWriter, r *http.Request) {
		if d.IngressDiscoveryPlan == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("ingress discovery is not wired"))
			return
		}
		plan, err := d.IngressDiscoveryPlan(r.Context())
		if err != nil {
			// Same reasoning as the reconcile route: a run in flight is a conflict,
			// and a preview of a moving target is worth less than an honest 409.
			if errors.Is(err, k8s.ErrReconcileInProgress) {
				writeErr(w, http.StatusConflict, err)
				return
			}
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, plan)
	}))

	// Docker container discovery: reconcile labelled containers into
	// template-derived, managed-labelled proxy hosts (which then feed DNS sync),
	// and report the last run. The Engine API is read strictly read-only, and
	// writes are ownership-gated on a label value this reconciler alone uses, so
	// it can never touch a host Ingress discovery owns. See
	// docs/design/docker-discovery.md.
	mux.HandleFunc("POST /docker-discovery/reconcile", d.scoped("docker-discovery:write", func(w http.ResponseWriter, r *http.Request) {
		if d.DockerDiscoveryReconcile == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("docker discovery is not wired"))
			return
		}
		if err := d.DockerDiscoveryReconcile(r.Context()); err != nil {
			// Same reasoning as the DNS and Ingress reconcile routes: an in-flight
			// run is a conflict, not a backend failure.
			if errors.Is(err, docker.ErrReconcileInProgress) {
				writeErr(w, http.StatusConflict, err)
				return
			}
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		if d.DockerDiscoveryStatus != nil {
			writeJSON(w, http.StatusOK, d.DockerDiscoveryStatus())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "reconciled"})
	}))
	mux.HandleFunc("GET /docker-discovery/status", d.scoped("docker-discovery:read", func(w http.ResponseWriter, r *http.Request) {
		if d.DockerDiscoveryStatus == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("docker discovery is not wired"))
			return
		}
		writeJSON(w, http.StatusOK, d.DockerDiscoveryStatus())
	}))
	// Dry run, mirroring GET /ingress-discovery/plan exactly.
	mux.HandleFunc("GET /docker-discovery/plan", d.scoped("docker-discovery:read", func(w http.ResponseWriter, r *http.Request) {
		if d.DockerDiscoveryPlan == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("docker discovery is not wired"))
			return
		}
		plan, err := d.DockerDiscoveryPlan(r.Context())
		if err != nil {
			if errors.Is(err, docker.ErrReconcileInProgress) {
				writeErr(w, http.StatusConflict, err)
				return
			}
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, plan)
	}))

	// Live upstream-group health (read-only): which upstreams each group currently
	// considers healthy, for the UI status view and operational checks.
	mux.HandleFunc("GET /upstream-health", d.scoped("*:read", func(w http.ResponseWriter, r *http.Request) {
		if d.UpstreamHealth == nil {
			writeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		writeJSON(w, http.StatusOK, d.UpstreamHealth())
	}))

	// Revoke every outstanding data-plane SSO session (users re-authenticate at
	// the IdP on their next request). Mutating POST, so the server's CSRF and
	// admin-session gates apply like any other write.
	mux.HandleFunc("POST /sso/revoke", d.scoped(model.ScopeAdmin, func(w http.ResponseWriter, r *http.Request) {
		if d.RevokeSSOSessions == nil {
			writeErr(w, http.StatusNotImplemented, fmt.Errorf("SSO revocation is not wired"))
			return
		}
		if err := d.RevokeSSOSessions(); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	}))

	// Whole-config reads (config dump, repo history) expose every object at once,
	// so they need the wildcard read scope - no single resource scope is enough.
	mux.HandleFunc("GET /config", d.scoped("*:read", func(w http.ResponseWriter, r *http.Request) {
		cfg, _, err := d.Store.Load(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		// API tokens are credentials, not configuration. A caller who may not
		// list them at GET /api-tokens must not get the same rows here by asking
		// for the whole tree instead - which is exactly what the read-only `user`
		// role does. Narrow the payload rather than refuse it: everything else in
		// the config is legitimately readable. A caller that DOES hold
		// api-tokens:read (any admin, and any token granted it explicitly or via
		// *:read) sees the unchanged payload.
		if !d.allows(r, "api-tokens:read") {
			cfg.APITokens = nil
		}
		writeJSON(w, http.StatusOK, cfg)
	}))
	// Sidebar object counts without the whole config graph - see summary.go.
	mux.HandleFunc("GET /config/summary", d.scoped("*:read", d.handleConfigSummary))
	mux.HandleFunc("GET /history", d.scoped("*:read", func(w http.ResponseWriter, r *http.Request) {
		commits, err := d.Store.RepoHistory(r.Context(), 100)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilCommits(commits))
	}))

	// Revert the whole config to a past commit (recorded as a new commit, so the
	// revert is itself revertible). Body: {"hash":"<commit>"}.
	// Whole-tree revert rewrites every object, including api-tokens, so it is an
	// admin-scope operation for a token principal.
	mux.HandleFunc("POST /revert", d.scoped(model.ScopeAdmin, func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(w, r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		var req struct {
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, decodeError(err))
			return
		}
		sha, err := d.Store.Revert(r.Context(), req.Hash, d.author(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if !d.applyChange(w, "revert", "", "", sha) {
			return
		}
		w.Header().Set(commitHeader, sha)
		writeJSON(w, http.StatusOK, map[string]string{"commit": sha})
	}))

	// Recent data-plane access entries (newest first) for the in-UI log viewer.
	mux.HandleFunc("GET /logs", d.scoped("*:read", func(w http.ResponseWriter, r *http.Request) {
		if d.RecentLogs == nil {
			writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "entries": []any{}})
			return
		}
		writeJSON(w, http.StatusOK, d.RecentLogs())
	}))

	// Flip data-plane request capture live. Runtime-only - never persisted, so
	// a restart reverts to the -access-log flag. Admin scope: it changes global
	// logging output (volume, captured client IPs and paths), not one resource.
	mux.HandleFunc("PUT /logs", d.scoped(model.ScopeAdmin, func(w http.ResponseWriter, r *http.Request) {
		if d.SetAccessLog == nil {
			writeErr(w, http.StatusNotImplemented, errors.New("access-log toggle is not wired"))
			return
		}
		body, err := readBody(w, r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Enabled == nil {
			writeErr(w, http.StatusBadRequest, errors.New(`body must be {"enabled": true|false}`))
			return
		}
		d.SetAccessLog(*req.Enabled)
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": *req.Enabled})
	}))

	// Download a portable archive of the entire config. Admin scope, not "*:read":
	// the archive is the raw on-disk YAML, so unlike the JSON reads it does carry
	// the api-tokens' stored digests (which are offline-crackable). It is also the
	// exact input POST /restore takes, which is admin-scoped for the same reason.
	mux.HandleFunc("GET /backup", d.scoped(model.ScopeAdmin, func(w http.ResponseWriter, r *http.Request) {
		archive, err := d.Store.Export(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", `attachment; filename="gpm-config-backup.tar.gz"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))

	// Restore the whole config from an uploaded archive (replaces current config,
	// validated, committed as one revision; rolled back if it does not validate).
	mux.HandleFunc("POST /restore", d.scoped(model.ScopeAdmin, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxArchiveBody))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		sha, err := d.Store.Restore(r.Context(), body, d.author(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if !d.applyChange(w, "restore", "", "", sha) {
			return
		}
		w.Header().Set(commitHeader, sha)
		writeJSON(w, http.StatusOK, map[string]string{"commit": sha})
	}))
	mux.HandleFunc("GET /settings", d.scoped("settings:read", func(w http.ResponseWriter, r *http.Request) {
		_, settings, err := d.Store.Load(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}))
	// Writing settings is admin-equivalent, so it takes the admin scope rather
	// than a "settings:write" grant. A settings write can point dnsSync.pihole or
	// a webhook at an attacker-controlled URL with a ${ENV:...} placeholder as its
	// credential, and the write itself triggers the reconcile/dispatch that
	// resolves and POSTs that env var offsite - and it can rewrite adminAuth. GET
	// stays on "settings:read": reading settings resolves nothing.
	mux.HandleFunc("PUT /settings", d.scoped(model.ScopeAdmin, func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(w, r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		var settings model.Settings
		if err := json.Unmarshal(body, &settings); err != nil {
			writeErr(w, http.StatusBadRequest, decodeError(err))
			return
		}
		sha, err := d.Store.SaveSettings(r.Context(), settings, d.author(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if !d.applyChange(w, "settings", "Settings", "", sha) {
			return
		}
		w.Header().Set(commitHeader, sha)
		writeJSON(w, http.StatusOK, settings)
	}))

	registerRuntime(mux, d)

	if d.Role.IsFollower() {
		return followerReadOnly(mux)
	}
	return mux
}

// errFollowerReadOnly names the role that CAN take the write, so the operator
// (or the SPA) is told where to make the change rather than just that it failed.
var errFollowerReadOnly = fmt.Errorf(
	"this instance runs as an HA follower (%s=%s) and is read-only: make config changes on the leader (%s=%s), they replicate here by git pull",
	ha.EnvRole, ha.RoleFollower, ha.EnvRole, ha.RoleLeader)

// followerReadOnly refuses every mutating request on a follower. Method-based
// rather than route-based so a route added later is refused by default: on this
// mux every write is a POST/PUT/DELETE and every GET/HEAD is a read.
func followerReadOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
		default:
			writeErr(w, http.StatusServiceUnavailable, errFollowerReadOnly)
		}
	})
}

// resource describes one config kind and how to project/decode it.
type resource[T model.Object] struct {
	kind   string
	list   func(model.Config) []T
	decode func(body []byte, name string) (T, error) // json.Unmarshal then set Name=name

	// readScope/writeScope override the default "<plural>:read"/"<plural>:write"
	// API-token scopes for this resource. api-tokens sets both to "admin": a
	// token must never be able to mint or edit other tokens (or itself) on the
	// strength of a resource scope, which would be a privilege-escalation path.
	readScope  string
	writeScope string

	// beforeSave, if set, runs on PUT after decoding and before the store write.
	// It sees the current config so it can act on whether the object already
	// exists, and returns extra top-level fields to merge into the JSON reply
	// (nil for the plain object reply). It is how an API token secret is minted
	// server-side and surfaced exactly once.
	beforeSave func(r *http.Request, cfg model.Config, obj T) (T, map[string]any, error)

	// decorate, if set, projects an object for GET responses (list and single),
	// e.g. to attach runtime-only fields that are not part of the stored object.
	// It sees the request so it can vary what it attaches by caller scope (e.g.
	// certStatusError, which only shows an admin caller the raw ACME error).
	decorate func(T, *http.Request) any
}

func register[T model.Object](mux *http.ServeMux, d Deps, plural string, res resource[T]) {
	base := "/" + plural
	readScope, writeScope := res.readScope, res.writeScope
	if readScope == "" {
		readScope = plural + ":read"
	}
	if writeScope == "" {
		writeScope = plural + ":write"
	}
	project := func(v T, r *http.Request) any {
		if res.decorate != nil {
			return res.decorate(v, r)
		}
		return v
	}

	mux.HandleFunc("GET "+base, d.scoped(readScope, func(w http.ResponseWriter, r *http.Request) {
		cfg, _, err := d.Store.Load(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		src := res.list(cfg)
		items := make([]any, 0, len(src))
		for _, it := range src {
			items = append(items, project(it, r))
		}
		writeJSON(w, http.StatusOK, items)
	}))

	mux.HandleFunc("GET "+base+"/{name}", d.scoped(readScope, func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		cfg, _, err := d.Store.Load(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		for _, it := range res.list(cfg) {
			if it.GetMeta().Name == name {
				writeJSON(w, http.StatusOK, project(it, r))
				return
			}
		}
		writeErr(w, http.StatusNotFound, errNotFound(res.kind, name))
	}))

	mux.HandleFunc("PUT "+base+"/{name}", d.scoped(writeScope, func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			writeErr(w, http.StatusBadRequest, errors.New("name is required"))
			return
		}
		body, err := readBody(w, r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		obj, err := res.decode(body, name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, decodeError(err))
			return
		}
		var extra map[string]any
		if res.beforeSave != nil {
			cfg, _, err := d.Store.Load(r.Context())
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
			if obj, extra, err = res.beforeSave(r, cfg, obj); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		sha, err := d.Store.Save(r.Context(), obj, d.author(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if !d.applyChange(w, "save", res.kind, name, sha) {
			return
		}
		w.Header().Set(commitHeader, sha)
		if extra == nil {
			writeJSON(w, http.StatusOK, obj)
			return
		}
		merged, err := mergeExtra(obj, extra)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, merged)
	}))

	mux.HandleFunc("DELETE "+base+"/{name}", d.scoped(writeScope, func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := model.ValidateName(name); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		sha, err := d.Store.Delete(r.Context(), res.kind, name, d.author(r))
		if err != nil {
			// Map the status from the SENTINEL first: store.Delete wraps the Go
			// kind name into its not-found text, and the human noun replaces the
			// message only after deleteStatus has read the wrapped error.
			status := deleteStatus(err)
			if errors.Is(err, store.ErrNotFound) {
				err = errNotFound(res.kind, name)
			}
			writeErr(w, status, err)
			return
		}
		if !d.applyChange(w, "delete", res.kind, name, sha) {
			return
		}
		w.Header().Set(commitHeader, sha)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET "+base+"/{name}/history", d.scoped(readScope, func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := model.ValidateName(name); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		commits, err := d.Store.History(r.Context(), res.kind, name, 50)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilCommits(commits))
	}))

	// Scoped revert: restore ONLY this object's file to its state at a past
	// commit, committing just that change (every other object is left as-is).
	// Body: {"hash":"<commit>"}. Contrast POST /revert, which resets the whole
	// config tree. Same auth/CSRF/reload/webhook wiring as every other write.
	mux.HandleFunc("POST "+base+"/{name}/revert", d.scoped(writeScope, func(w http.ResponseWriter, r *http.Request) {
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
		var req struct {
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, decodeError(err))
			return
		}
		sha, err := d.Store.RevertObject(r.Context(), res.kind, name, req.Hash, d.author(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if !d.applyChange(w, "revert", res.kind, name, sha) {
			return
		}
		w.Header().Set(commitHeader, sha)
		writeJSON(w, http.StatusOK, map[string]string{"commit": sha})
	}))
}

// tokenRevealNote is returned alongside a freshly minted secret so a client (or
// a human reading curl output) cannot mistake it for a value they can fetch again.
const tokenRevealNote = "Copy this token now - it is shown once and cannot be retrieved again. Re-run the PUT with ?rotate=1 to mint a replacement."

// mintTokenSecret is the api-tokens PUT hook. It generates a secret server-side
// when the token is new, or when the caller asks for a rotation with ?rotate=1,
// and otherwise carries the stored digest forward so an ordinary edit (scopes,
// expiry, disabled) never silently invalidates a token in use. The plaintext is
// returned once in the response and never persisted.
func mintTokenSecret(r *http.Request, cfg model.Config, obj model.APIToken) (model.APIToken, map[string]any, error) {
	var existing *model.APIToken
	for i := range cfg.APITokens {
		if cfg.APITokens[i].Name == obj.Name {
			existing = &cfg.APITokens[i]
			break
		}
	}
	rotate := r.URL.Query().Get("rotate") == "1"
	if existing != nil && existing.TokenHash != "" && !rotate {
		obj.TokenHash = existing.TokenHash
		if obj.CreatedAt.IsZero() {
			obj.CreatedAt = existing.CreatedAt
		}
		return obj, nil, nil
	}
	secret, hash, err := auth.NewTokenSecret()
	if err != nil {
		return obj, nil, err
	}
	obj.TokenHash = hash
	if existing != nil && obj.CreatedAt.IsZero() {
		obj.CreatedAt = existing.CreatedAt
	}
	return obj, map[string]any{"token": secret, "tokenNote": tokenRevealNote}, nil
}

// decorateToken attaches the in-memory last-use timestamp to an API token in GET
// responses. The value is runtime-only (see Authenticator.TokenLastUsed): it is
// never written back to the git-backed store.
func (d Deps) decorateToken(t model.APIToken, _ *http.Request) any {
	out, err := mergeExtra(t, nil)
	if err != nil {
		return t
	}
	if d.TokenLastUsed != nil {
		if ts, ok := d.TokenLastUsed()[t.Name]; ok {
			out["lastUsed"] = ts.Format(time.RFC3339)
		}
	}
	return out
}

// mergeExtra renders obj as a JSON object and merges extra's fields into it, so
// a write can return the stored object plus one-time fields (the freshly minted
// token secret) without a bespoke response type per resource.
func mergeExtra(obj any, extra map[string]any) (map[string]any, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	for k, v := range extra {
		out[k] = v
	}
	return out, nil
}

// deleteStatus maps a store.Delete error to an HTTP status: 404 when the object
// is absent, 409 when the delete is refused to avoid dangling references.
func deleteStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrDanglingRef):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
}

func nonNilCommits(c []store.Commit) []store.Commit {
	if c == nil {
		return []store.Commit{}
	}
	return c
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentTypeJS)
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
