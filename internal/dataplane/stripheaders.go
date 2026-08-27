package dataplane

import (
	"net/http"
	"net/textproto"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// globalStripResponseHeaders holds the compiled settings-level default list of
// response headers removed from what an upstream sends, installed once per
// config reload via SetStripResponseHeaders and read by hostProxy when it
// composes each proxy host's effective list. It mirrors globalSecurityHeaders'
// package-level handle for the same reason: the value only ever changes
// alongside a full reload, and threading Settings through buildRouter for it
// would touch far more of the data plane.
//
// Only the reverse proxy reads it. A redirect/parked host and the host-less
// 404/421 have no upstream response at all, so there is nothing there to strip.
var globalStripResponseHeaders atomic.Pointer[[]string]

// SetStripResponseHeaders compiles and installs the settings-level default strip
// list. Called before the data-plane Reload so the list is in place before any
// request is served. The list has already passed model validation at config-write
// time; compile canonicalizes, de-duplicates, and defensively drops anything a
// valid config could not contain.
func SetStripResponseHeaders(names []string) {
	c := compileStripResponseHeaders(names)
	globalStripResponseHeaders.Store(&c)
}

func currentStripResponseHeaders() []string {
	if p := globalStripResponseHeaders.Load(); p != nil {
		return *p
	}
	return nil
}

// compileStripResponseHeaders canonicalizes names to MIME header form (so the
// match is case-insensitive: "x-powered-by" in the config removes the upstream's
// "X-Powered-By") and drops empties, duplicates, and the names validation
// refuses - hop-by-hop headers and the response-semantic set
// (Content-Type/-Length/-Encoding, Vary, Location). Model validation is
// authoritative; dropping them here too is the same defence in depth
// compileSecurityHeaders applies to its own refused keys, so a config that
// bypassed validation cannot corrupt a response. Returns nil for an empty list,
// so an unconfigured host takes the zero-overhead path.
func compileStripResponseHeaders(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		ck := textproto.CanonicalMIMEHeaderKey(n)
		if ck == "" || model.StripResponseHeaderRefused(ck) {
			continue
		}
		if _, dup := seen[ck]; dup {
			continue
		}
		seen[ck] = struct{}{}
		out = append(out, ck)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// mergedStripResponseHeaders composes the effective strip list for one proxy
// host: the UNION of the settings-level default and the host's own list. Unlike
// securityHeaders - a map, where a host overrides the settings value for a key it
// names - a list carries no per-name value to override, so the only two possible
// semantics are "host replaces the fleet baseline" and "host adds to it". Union
// is the safe one: a strip list is a hardening baseline, and a host must not be
// able to silently re-expose a header the fleet strips by naming an unrelated
// one. Returns nil when neither level configures anything.
func mergedStripResponseHeaders(override []string) []string {
	base := currentStripResponseHeaders()
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	return compileStripResponseHeaders(append(append([]string{}, base...), override...))
}

// stripUpstreamResponseHeaders removes the compiled names from an UPSTREAM
// response's own header map. It is called from the reverse proxy's
// ModifyResponse hook, which is the only point where the backend's headers exist
// on their own, before httputil.ReverseProxy copies them onto the client
// response (and, for a 101, before handleUpgradeResponse writes them to the
// hijacked connection). An interim 1xx is the exception: the stdlib forwards it
// through Got1xxResponse, which does not run this hook, and then clears the
// header map - so interim headers are neither stripped nor carried forward.
//
// Doing it here rather than on the way out of the dispatch writer is what keeps
// the removal honest: the writer sees ONE merged header map, so a name in the
// strip list would also delete a header gpm itself put there - the forward-auth
// Set-Cookie session refresh, gzip's Content-Encoding/Vary, a headers
// middleware's setResponse value, an injected security header. Here there is
// nothing but what the backend sent.
func stripUpstreamResponseHeaders(resp *http.Response, names []string) {
	if resp == nil || len(names) == 0 {
		return
	}
	for _, k := range names {
		resp.Header.Del(k)
	}
}
