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

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/dnssync"
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
	// RecentLogs, if set, returns the data-plane access-log viewer payload
	// (marshalled as-is) for GET /logs. May be nil.
	RecentLogs func() any
	// GeoDBLoaded reports whether a GeoIP database is currently loaded, for the
	// read-only GET /capabilities probe the UI uses to enable/grey-out geo
	// controls. May be nil (reported as not loaded).
	GeoDBLoaded func() bool
	// UpstreamHealth, if set, returns the live per-group upstream health payload
	// (marshalled as-is) for GET /upstream-health. May be nil.
	UpstreamHealth func() any
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
	// IngressDiscoveryEnabled, if set, reports whether Ingress discovery is
	// configured, for the capability probe. May be nil (reported disabled).
	IngressDiscoveryEnabled func() bool
	// Role is the static HA role of this instance (docs/design/ha.md phase 1).
	// A follower refuses every config write with 503 and reports itself
	// read-only in the capability probe; reads are unaffected. The zero value is
	// the leader, i.e. today's single-node behaviour.
	Role ha.Role
}

// capabilities is the read-only runtime feature-availability payload returned by
// GET /capabilities. It is intentionally an object-of-objects so new capability
// groups can be added without breaking existing clients.
type capabilities struct {
	GeoIP            geoIPCapability            `json:"geoip"`
	APITokens        apiTokenCapability         `json:"apiTokens"`
	DNSSync          dnsSyncCapability          `json:"dnsSync"`
	IngressDiscovery ingressDiscoveryCapability `json:"ingressDiscovery"`
	HA               haCapability               `json:"ha"`
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

	register(mux, d, "proxy-hosts", resource[model.ProxyHost]{
		kind: "ProxyHost",
		list: func(c model.Config) []model.ProxyHost { return c.ProxyHosts },
		decode: func(b []byte, name string) (model.ProxyHost, error) {
			var v model.ProxyHost
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
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
	register(mux, d, "dead-hosts", resource[model.DeadHost]{
		kind: "DeadHost",
		list: func(c model.Config) []model.DeadHost { return c.DeadHosts },
		decode: func(b []byte, name string) (model.DeadHost, error) {
			var v model.DeadHost
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
	})
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
	register(mux, d, "middlewares", resource[model.Middleware]{
		kind: "Middleware",
		list: func(c model.Config) []model.Middleware { return c.Middlewares },
		decode: func(b []byte, name string) (model.Middleware, error) {
			var v model.Middleware
			err := json.Unmarshal(b, &v)
			v.Name = name
			return v, err
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
		writeJSON(w, http.StatusOK, capabilities{
			GeoIP:     geoIPCapability{DBLoaded: d.GeoDBLoaded != nil && d.GeoDBLoaded()},
			APITokens: apiTokenCapability{Enabled: d.RequireScope != nil},
			DNSSync:   dnsSyncCapability{PiholeEnabled: pihole, CloudflareEnabled: cloudflare},
			IngressDiscovery: ingressDiscoveryCapability{
				Enabled: d.IngressDiscoveryEnabled != nil && d.IngressDiscoveryEnabled(),
			},
			HA:            haCapability{Role: d.Role.String(), ReadOnly: d.Role.IsFollower()},
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
		writeJSON(w, http.StatusOK, cfg)
	}))
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
			writeErr(w, http.StatusBadRequest, err)
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
			writeErr(w, http.StatusBadRequest, err)
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
	decorate func(T) any
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
	project := func(v T) any {
		if res.decorate != nil {
			return res.decorate(v)
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
			items = append(items, project(it))
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
				writeJSON(w, http.StatusOK, project(it))
				return
			}
		}
		writeErr(w, http.StatusNotFound, errors.New(res.kind+" "+name+" not found"))
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
			writeErr(w, http.StatusBadRequest, err)
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
			writeErr(w, deleteStatus(err), err)
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
			writeErr(w, http.StatusBadRequest, err)
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
func (d Deps) decorateToken(t model.APIToken) any {
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
