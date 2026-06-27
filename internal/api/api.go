// Package api exposes a stdlib-only REST surface for CRUD over the typed config
// objects. Every write goes through the git-backed store, which validates the
// whole object graph and commits, so the API never bypasses referential
// integrity or history. The handler is an *http.ServeMux registered without an
// "/api" prefix; the caller mounts it under /api/.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

const (
	maxBody       = 1 << 20 // 1MB request body cap
	commitHeader  = "X-Config-Commit"
	contentTypeJS = "application/json"
)

// Deps wires the API to the store and the rest of the app.
type Deps struct {
	Store    *store.Store
	OnChange func()                           // called after every successful write; may be nil
	Author   func(*http.Request) store.Author // derives the commit author; if nil, a zero Author is used
}

func (d Deps) author(r *http.Request) store.Author {
	if d.Author != nil {
		return d.Author(r)
	}
	return store.Author{}
}

func (d Deps) onChange() {
	if d.OnChange != nil {
		d.OnChange()
	}
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
		d.onChange()
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
		d.onChange()
		w.Header().Set(commitHeader, sha)
		writeJSON(w, http.StatusOK, obj)
	})

	mux.HandleFunc("DELETE "+base+"/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		sha, err := d.Store.Delete(r.Context(), res.kind, name, d.author(r))
		if err != nil {
			writeErr(w, deleteStatus(err), err)
			return
		}
		d.onChange()
		w.Header().Set(commitHeader, sha)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET "+base+"/{name}/history", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
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
