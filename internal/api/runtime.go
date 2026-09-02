package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/notify"
	"github.com/Rake-Pro/go-proxy-manager/internal/webhook"
)

// RuntimeConfig is the set of process-level facts that come from flags and
// environment variables rather than from the git-backed config, captured once at
// startup and handed to the API.
//
// It exists because the admin UI previously had NO way to see how the daemon was
// actually started: which addresses it listens on, where its config and
// certificates live, whether a local admin credential exists, whether pprof is
// exposed. An operator debugging "why can't I log in" or "which directory do I
// back up" had to read the container's command line. The handler never re-reads
// the environment, so what the UI shows is what this process is running with,
// not what the environment happens to say now.
type RuntimeConfig struct {
	Version   string
	HTTPAddr  string
	HTTPSAddr string
	AdminAddr string
	ConfigDir string
	CertDir   string
	SessionDB string
	// SecretFileRoots is the effective ${FILE:...} allowlist (model.SecretFileRoots).
	SecretFileRoots []string
	// LocalAdminConfigured is true when BOTH the local admin username and its
	// bcrypt hash were supplied, i.e. the local login form can actually succeed.
	// Neither the username nor the hash is ever reported - only whether a usable
	// pair exists.
	LocalAdminConfigured bool
	// LocalAdminTOTP is true when a TOTP secret was supplied for the local admin
	// (GPM_LOCAL_ADMIN_TOTP_SECRET / _FILE), i.e. the local login has a second
	// factor. The secret itself is never reported.
	LocalAdminTOTP bool
	PprofEnabled   bool
}

// runtimeInfo is the GET /runtime payload: RuntimeConfig plus the few facts that
// are live rather than fixed at startup (the access log can be toggled at
// runtime, the GeoIP database can appear or vanish under the daemon).
type runtimeInfo struct {
	Version   string           `json:"version"`
	HARole    string           `json:"haRole"`
	Listeners runtimeListeners `json:"listeners"`
	// Paths maps the deployment's filesystem layout and is OMITTED for a caller
	// without the admin scope (the read-only viewer role, and any narrower
	// token); SecretFileRoots is empty for the same caller. Everything else here
	// is a version, an address or a boolean an operator already reads off their
	// own deployment manifest.
	Paths                *runtimePaths `json:"paths,omitempty"`
	MetricsEnabled       bool          `json:"metricsEnabled"`
	AccessLogEnabled     bool          `json:"accessLogEnabled"`
	GeoIPLoaded          bool          `json:"geoipLoaded"`
	SecretFileRoots      []string      `json:"secretFileRoots"`
	LocalAdminConfigured bool          `json:"localAdminConfigured"`
	LocalAdminTOTP       bool          `json:"localAdminTOTP"`
	PprofEnabled         bool          `json:"pprofEnabled"`
}

type runtimeListeners struct {
	HTTP  string `json:"http"`
	HTTPS string `json:"https"`
	Admin string `json:"admin"`
}

type runtimePaths struct {
	ConfigDir string `json:"configDir"`
	CertDir   string `json:"certDir"`
	SessionDB string `json:"sessionDB"`
}

// redactDeliveryURLs strips the path and query from every target URL unless the
// caller holds the admin scope. For a Discord, Slack or ntfy receiver the URL
// path IS the credential, and these two status routes ride "settings:read",
// which the read-only viewer role holds. Scheme and host are kept so the status
// list stays readable ("which target is this").
//
// A caller without admin gets an empty slice, not v itself, when v is not a
// []webhook.Delivery: this function must fail CLOSED. Both current callers
// happen to always pass a []webhook.Delivery, but "I did not recognise this
// value" is the wrong branch to default to unredacted - a future change to
// either status type must not silently start leaking receiver URLs with no
// test failure.
func redactDeliveryURLs(v any, admin bool) any {
	ds, ok := v.([]webhook.Delivery)
	if admin {
		return v
	}
	if !ok {
		return []webhook.Delivery{}
	}
	out := make([]webhook.Delivery, 0, len(ds))
	for _, d := range ds {
		if u, err := url.Parse(d.URL); err == nil && u.Host != "" {
			d.URL = u.Scheme + "://" + u.Host + "/(redacted)"
		} else if d.URL != "" {
			d.URL = "(redacted)"
		}
		out = append(out, d)
	}
	return out
}

// registerRuntime mounts the read-only runtime probe and the webhook
// status/test routes. They live here rather than in New's body to keep that
// function about resource registration.
func registerRuntime(mux *http.ServeMux, d Deps) {
	// Read-only startup facts. settings:read rather than admin: it reports no
	// secret, no username and no hash - only paths, addresses and booleans an
	// operator already sees in their own deployment manifest.
	mux.HandleFunc("GET /runtime", d.scoped("settings:read", func(w http.ResponseWriter, r *http.Request) {
		var paths *runtimePaths
		roots := []string{}
		if d.allows(r, model.ScopeAdmin) {
			paths = &runtimePaths{
				ConfigDir: d.Runtime.ConfigDir,
				CertDir:   d.Runtime.CertDir,
				SessionDB: d.Runtime.SessionDB,
			}
			if d.Runtime.SecretFileRoots != nil {
				roots = d.Runtime.SecretFileRoots
			}
		}
		writeJSON(w, http.StatusOK, runtimeInfo{
			Version: d.Runtime.Version,
			HARole:  d.Role.String(),
			Listeners: runtimeListeners{
				HTTP:  d.Runtime.HTTPAddr,
				HTTPS: d.Runtime.HTTPSAddr,
				Admin: d.Runtime.AdminAddr,
			},
			Paths:                paths,
			MetricsEnabled:       d.MetricsEnabled,
			AccessLogEnabled:     d.AccessLogEnabled != nil && d.AccessLogEnabled(),
			GeoIPLoaded:          d.GeoDBLoaded != nil && d.GeoDBLoaded(),
			SecretFileRoots:      roots,
			LocalAdminConfigured: d.Runtime.LocalAdminConfigured,
			LocalAdminTOTP:       d.Runtime.LocalAdminTOTP,
			PprofEnabled:         d.Runtime.PprofEnabled,
		})
	}))

	// Per-target last-delivery state. Webhook delivery is fire-and-forget, so
	// until now a receiver that had been 404ing for a month looked identical to
	// one working perfectly: the only evidence was a warn line in the daemon log.
	mux.HandleFunc("GET /webhooks/status", d.scoped("settings:read", func(w http.ResponseWriter, r *http.Request) {
		if d.WebhookStatus == nil {
			writeJSON(w, http.StatusOK, []webhook.Delivery{})
			return
		}
		writeJSON(w, http.StatusOK, redactDeliveryURLs(d.WebhookStatus(), d.allows(r, model.ScopeAdmin)))
	}))

	// Synchronous test delivery. Admin scope: it makes gpm POST to an
	// admin-configured URL on demand and resolves that target's secret, which is
	// exactly the reach a settings write has.
	mux.HandleFunc("POST /webhooks/{name}/test", d.scoped(model.ScopeAdmin, func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := model.ValidateName(name); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if d.WebhookTest == nil {
			writeErr(w, http.StatusNotImplemented, errors.New("webhook delivery is not wired"))
			return
		}
		res, err := d.WebhookTest(r.Context(), name)
		if err != nil {
			if errors.Is(err, webhook.ErrUnknownTarget) {
				writeErr(w, http.StatusNotFound, errNotFound("Webhook", name))
				return
			}
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		// A receiver that refused or timed out is a successful TEST with a failed
		// delivery: the operator asked "what happens if I send one", and the answer
		// is in the body. Reporting it as an HTTP error would put the receiver's
		// failure in the same bucket as "gpm could not run the test".
		writeJSON(w, http.StatusOK, res)
	}))

	// Per-target last-delivery state for settings.notifications, mirroring
	// GET /webhooks/status above.
	mux.HandleFunc("GET /notifications/status", d.scoped("settings:read", func(w http.ResponseWriter, r *http.Request) {
		if d.NotificationStatus == nil {
			writeJSON(w, http.StatusOK, []webhook.Delivery{})
			return
		}
		writeJSON(w, http.StatusOK, redactDeliveryURLs(d.NotificationStatus(), d.allows(r, model.ScopeAdmin)))
	}))

	// Synchronous test send. Admin scope for the same reason as the webhook
	// test above: it POSTs to an admin-configured URL on demand and resolves
	// that target's secret.
	mux.HandleFunc("POST /notifications/{name}/test", d.scoped(model.ScopeAdmin, func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := model.ValidateName(name); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if d.NotificationTest == nil {
			writeErr(w, http.StatusNotImplemented, errors.New("notification delivery is not wired"))
			return
		}
		res, err := d.NotificationTest(r.Context(), name)
		if err != nil {
			if errors.Is(err, notify.ErrUnknownTarget) {
				writeErr(w, http.StatusNotFound, errNotFound("NotificationTarget", name))
				return
			}
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}))
}
