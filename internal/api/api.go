// Package api exposes a stdlib-only REST surface for CRUD over the typed config
// objects. Every write goes through the git-backed store, which validates the
// whole object graph and commits, so the API never bypasses referential
// integrity or history. The handler is an *http.ServeMux registered without an
// "/api" prefix; the caller mounts it under /api/.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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
}

// capabilities is the read-only runtime feature-availability payload returned by
// GET /capabilities. It is intentionally an object-of-objects so new capability
// groups can be added without breaking existing clients.
type capabilities struct {
	GeoIP geoIPCapability `json:"geoip"`
}

type geoIPCapability struct {
	// DBLoaded is true when a GeoIP database is loaded and geo access-list rules
	// can be evaluated (and saved). When false the UI should grey out geo controls.
	DBLoaded bool `json:"dbLoaded"`
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

	// Read-only runtime capability probe: lets the SPA discover which optional
	// features are currently usable (e.g. geo rules need a loaded GeoIP DB). Auth
	// is whatever the admin server applies to every other GET route on this mux.
	mux.HandleFunc("GET /capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, capabilities{
			GeoIP: geoIPCapability{DBLoaded: d.GeoDBLoaded != nil && d.GeoDBLoaded()},
		})
	})

	// Live upstream-group health (read-only): which upstreams each group currently
	// considers healthy, for the UI status view and operational checks.
	mux.HandleFunc("GET /upstream-health", func(w http.ResponseWriter, r *http.Request) {
		if d.UpstreamHealth == nil {
			writeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		writeJSON(w, http.StatusOK, d.UpstreamHealth())
	})

	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		cfg, _, err := d.Store.Load(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, r *http.Request) {
		commits, err := d.Store.RepoHistory(r.Context(), 100)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilCommits(commits))
	})

	// Revert the whole config to a past commit (recorded as a new commit, so the
	// revert is itself revertible). Body: {"hash":"<commit>"}.
	mux.HandleFunc("POST /revert", func(w http.ResponseWriter, r *http.Request) {
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
	})

	// Recent data-plane access entries (newest first) for the in-UI log viewer.
	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		if d.RecentLogs == nil {
			writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "entries": []any{}})
			return
		}
		writeJSON(w, http.StatusOK, d.RecentLogs())
	})

	// Download a portable archive of the entire config.
	mux.HandleFunc("GET /backup", func(w http.ResponseWriter, r *http.Request) {
		archive, err := d.Store.Export(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", `attachment; filename="gpm-config-backup.tar.gz"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	})

	// Restore the whole config from an uploaded archive (replaces current config,
	// validated, committed as one revision; rolled back if it does not validate).
	mux.HandleFunc("POST /restore", func(w http.ResponseWriter, r *http.Request) {
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
	})
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		_, settings, err := d.Store.Load(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
	mux.HandleFunc("PUT /settings", func(w http.ResponseWriter, r *http.Request) {
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
	})

	return mux
}

// resource describes one config kind and how to project/decode it.
type resource[T model.Object] struct {
	kind   string
	list   func(model.Config) []T
	decode func(body []byte, name string) (T, error) // json.Unmarshal then set Name=name
}

func register[T model.Object](mux *http.ServeMux, d Deps, plural string, res resource[T]) {
	base := "/" + plural

	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		cfg, _, err := d.Store.Load(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		items := res.list(cfg)
		if items == nil {
			items = []T{}
		}
		writeJSON(w, http.StatusOK, items)
	})

	mux.HandleFunc("GET "+base+"/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		cfg, _, err := d.Store.Load(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		for _, it := range res.list(cfg) {
			if it.GetMeta().Name == name {
				writeJSON(w, http.StatusOK, it)
				return
			}
		}
		writeErr(w, http.StatusNotFound, errors.New(res.kind+" "+name+" not found"))
	})

	mux.HandleFunc("PUT "+base+"/{name}", func(w http.ResponseWriter, r *http.Request) {
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
		sha, err := d.Store.Save(r.Context(), obj, d.author(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if !d.applyChange(w, "save", res.kind, name, sha) {
			return
		}
		w.Header().Set(commitHeader, sha)
		writeJSON(w, http.StatusOK, obj)
	})

	mux.HandleFunc("DELETE "+base+"/{name}", func(w http.ResponseWriter, r *http.Request) {
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
	})

	mux.HandleFunc("GET "+base+"/{name}/history", func(w http.ResponseWriter, r *http.Request) {
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
	})
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
