package dataplane

import (
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// maintenanceStatus is the status every maintenance response carries. 503 is
// the correct code for a host that is deliberately, temporarily out of service:
// it is what Retry-After is defined against, and it is the one 5xx a search
// engine treats as "come back later" rather than as a broken or removed site.
const maintenanceStatus = http.StatusServiceUnavailable

// globalMaintenance is the settings-level maintenance switch, installed once per
// config reload via SetMaintenance and consulted LIVE on every request. It
// mirrors globalErrorPages' package-level handle for the same reason: Settings
// is not part of model.Config, so threading it through buildRouter/buildChain
// would touch the whole data plane for a value that changes only alongside a
// reload.
var globalMaintenance atomic.Pointer[model.MaintenanceSettings]

// SetMaintenance installs the settings-level maintenance switch. Unlike a
// per-host flag (which is compiled into the router) this takes effect on the
// very next request, so the fleet-wide toggle never waits on a router rebuild.
func SetMaintenance(m model.MaintenanceSettings) { globalMaintenance.Store(&m) }

// currentMaintenance returns the installed settings, or the zero value (off)
// when SetMaintenance has never run - e.g. an embedder or a test that builds a
// router directly.
func currentMaintenance() model.MaintenanceSettings {
	if p := globalMaintenance.Load(); p != nil {
		return *p
	}
	return model.MaintenanceSettings{}
}

// MaintenanceGlobalEnabled reports whether the fleet-wide switch is currently
// on, for the admin capability probe (the SPA greys the per-host toggle's
// meaning accordingly rather than showing "off" for a host that is in fact
// down).
func MaintenanceGlobalEnabled() bool { return currentMaintenance().Enabled }

// maintenanceActive reports whether a host whose own flag is hostFlag must be
// answered with the maintenance page. The fleet-wide toggle wins outright: an
// operator taking the whole edge down must not have to walk every host, and a
// host that is individually out stays out when the global switch is off.
func maintenanceActive(hostFlag bool) bool {
	return currentMaintenance().Enabled || hostFlag
}

// serveMaintenance writes the maintenance response for host: the configured
// errorPages template for 503 (the host's own override first, then the
// settings-level pages - the same resolution every other gpm-generated error
// uses, so a custom maintenance page needs no second mechanism), or gpm's
// built-in body when neither configures one.
//
// Retry-After is set before the body is chosen so it rides a custom template
// too: a maintenance 503 without one reads to a crawler like an outage of
// unknown length.
func serveMaintenance(w http.ResponseWriter, r *http.Request, ep *compiledErrorPages, host string) {
	w.Header().Set("Retry-After", strconv.Itoa(currentMaintenance().EffectiveRetryAfter()))
	serveErrorPage(w, maintenanceStatus, ep, host, func() {
		writeDefaultMaintenance(w, r)
	})
}

const maintenanceHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Down for maintenance</title>
<style>
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
background:#0f1115;color:#e6e8ee;font:16px/1.6 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
main{max-width:32rem;padding:2rem;text-align:center}
h1{font-size:1.5rem;margin:0 0 .5rem}
p{margin:0;color:#9aa3b2}
</style>
</head>
<body><main>
<h1>Down for maintenance</h1>
<p>This service is temporarily unavailable while maintenance is in progress. Please try again shortly.</p>
</main></body>
</html>
`

const maintenanceJSON = `{"error":"maintenance","status":503,"message":"This service is temporarily unavailable for maintenance. Please try again shortly."}
`

const maintenancePlain = "Down for maintenance: this service is temporarily unavailable. Please try again shortly.\n"

// writeDefaultMaintenance writes gpm's built-in maintenance body when no custom
// 503 page is configured.
//
// It ALWAYS sets Content-Type and writes a body. A bodyless, type-less error
// response from this proxy is not a cosmetic problem: a Home Assistant
// integration crashed parsing exactly that shape in production, so the body is
// negotiated by Accept - JSON for a JSON client, HTML for a browser, plain text
// for anything else - and is never empty.
//
// Only an explicit media type in Accept selects a shape; "*/*" (curl, most
// scripts) lands on plain text rather than being handed markup.
func writeDefaultMaintenance(w http.ResponseWriter, r *http.Request) {
	body, ctype := maintenanceBody(r.Header.Get("Accept"))
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(maintenanceStatus)
	_, _ = w.Write([]byte(body))
}

// maintenanceBody picks the built-in body and its Content-Type for an Accept
// header.
func maintenanceBody(accept string) (string, string) {
	switch {
	case acceptsMediaType(accept, "application/json"):
		return maintenanceJSON, "application/json; charset=utf-8"
	case acceptsMediaType(accept, "text/html"):
		return maintenanceHTML, "text/html; charset=utf-8"
	default:
		return maintenancePlain, "text/plain; charset=utf-8"
	}
}

// acceptsMediaType reports whether an Accept header names want explicitly.
// Parameters (q-values, charset) are ignored and wildcards deliberately do NOT
// match: "*/*" means "anything", which is exactly the client that should get
// the plain-text default.
func acceptsMediaType(accept, want string) bool {
	for _, part := range strings.Split(accept, ",") {
		t, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(t), want) {
			return true
		}
	}
	return false
}
