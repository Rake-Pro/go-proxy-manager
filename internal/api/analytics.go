package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"

	"github.com/Rake-Pro/go-proxy-manager/internal/analytics"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// handleAnalytics is GET /analytics: the traffic analytics report for the
// Analytics view. Query: range (one of analytics.Ranges(), default 24h) and an
// optional host (proxy host NAME). Unwired (no collector, no seed provider)
// responds 501 so the UI can show its "analytics is off" state.
func (d Deps) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	if d.Analytics == nil {
		writeErr(w, http.StatusNotImplemented, errors.New("analytics is not enabled on this instance"))
		return
	}
	// url.ParseQuery rather than r.URL.Query(): the latter silently drops any
	// pair containing ";", which would turn "host=x;y" into the fleet view.
	qs, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("malformed query string"))
		return
	}
	rng := qs.Get("range")
	if rng == "" {
		rng = "24h"
	}
	if _, _, ok := analytics.ParseRange(rng); !ok {
		writeErr(w, http.StatusBadRequest, errors.New("range must be one of 1h, 6h, 24h, 7d, 30d"))
		return
	}
	host := qs.Get("host")
	if host != "" && model.ValidateName(host) != nil {
		writeErr(w, http.StatusBadRequest, errors.New("invalid host name"))
		return
	}
	rep, err := d.Analytics(rng, host)
	if err != nil {
		// Only the one caller error is reflected; a collector's internal
		// failure is logged and answered generically, never with its text.
		if errors.Is(err, analytics.ErrUnknownHost) {
			writeErr(w, http.StatusBadRequest, errors.New("unknown host"))
			return
		}
		log.Error().Err(err).Str("range", rng).Str("host", host).Msg("analytics report failed")
		writeErr(w, http.StatusInternalServerError, errors.New("analytics report unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
