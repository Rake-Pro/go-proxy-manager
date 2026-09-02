package api

import "net/http"

// configSummaryResponse is GET /config/summary: the sidebar needs object
// counts (to decide whether the ADVANCED nav group starts open) and nothing
// else, so this exists to spare it fetching the whole GET /config payload -
// every host, certificate and secret placeholder - just to read len() on a
// dozen slices.
type configSummaryResponse struct {
	// Counts is keyed by the same plural resource names used in API paths
	// (e.g. "proxy-hosts"), one entry per first-class config kind.
	Counts map[string]int `json:"counts"`
	// Disabled/Maintenance report operator-owned runtime flags the sidebar
	// also cares about. Today that is proxy hosts only: Disabled lives on
	// every kind's ObjectMeta, but Maintenance is a ProxyHost-only field (see
	// model.ProxyHost.Maintenance), and the sidebar has no use for a disabled
	// count on kinds it does not badge.
	Disabled    map[string]int `json:"disabled"`
	Maintenance map[string]int `json:"maintenance"`
	// Head is the config repo's current commit, same value as GET /health's
	// configHead, so a caller that already has the summary need not make a
	// second request just to detect a config change.
	Head string `json:"head"`
}

// handleConfigSummary is GET /config/summary: the same store read GET /config
// does, reduced to counts instead of the full object graph. No disk I/O beyond
// the Store.Load/Head calls every other read-only aggregate route already
// makes.
func (d Deps) handleConfigSummary(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := d.Store.Load(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	head, err := d.Store.Head(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	// API tokens are credentials, not configuration: a caller who cannot list
	// them at GET /api-tokens must not learn how many exist here either. Same
	// rule GET /config applies to the rows themselves.
	apiTokens := len(cfg.APITokens)
	if !d.allows(r, "api-tokens:read") {
		apiTokens = 0
	}

	disabledHosts, maintenanceHosts := 0, 0
	for _, h := range cfg.ProxyHosts {
		if h.Disabled {
			disabledHosts++
		}
		if h.Maintenance {
			maintenanceHosts++
		}
	}

	writeJSON(w, http.StatusOK, configSummaryResponse{
		Counts: map[string]int{
			"proxy-hosts":        len(cfg.ProxyHosts),
			"redirect-hosts":     len(cfg.RedirectHosts),
			"stream-hosts":       len(cfg.StreamHosts),
			"parked-hosts":       len(cfg.ParkedHosts),
			"certificates":       len(cfg.Certificates),
			"client-cas":         len(cfg.ClientCAs),
			"identity-providers": len(cfg.IdentityProviders),
			"access-lists":       len(cfg.AccessLists),
			"middlewares":        len(cfg.Middlewares),
			"upstream-groups":    len(cfg.UpstreamGroups),
			"dns-providers":      len(cfg.DNSProviders),
			"api-tokens":         apiTokens,
		},
		Disabled:    map[string]int{"proxy-hosts": disabledHosts},
		Maintenance: map[string]int{"proxy-hosts": maintenanceHosts},
		Head:        head,
	})
}
