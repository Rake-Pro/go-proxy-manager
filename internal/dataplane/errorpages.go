package dataplane

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog/log"
)

// compiledErrorPages is a fully-parsed set of custom error-page templates,
// ready to execute per request with no further I/O or parse work. It mirrors
// the geoDB/SSO-key global-handle pattern for the settings-level pages (see
// SetErrorPages) and is threaded through buildRouter/buildChain for the
// per-host override.
type compiledErrorPages struct {
	byStatus  map[int]*template.Template
	def       *template.Template // "default" fallback, or nil
	intercept map[int]bool       // interceptUpstream set
}

// errorPageData is the template execution context.
type errorPageData struct {
	Status     int
	StatusText string
	Host       string
	RequestID  string
}

// compileErrorPages parses cfg's templates (dir and/or inline) with
// html/template, so an operator-authored page can never inject markup from the
// (mostly gpm-controlled) template data. Returns (nil, nil) when cfg configures
// nothing, so callers can treat "not configured" and "configured but empty" the
// same way. A parse (or unreadable dir) error is returned verbatim, naming the
// offending file/status, so the reload it is part of fails with a clear message.
func compileErrorPages(cfg model.ErrorPagesConfig, certDir string) (*compiledErrorPages, error) {
	if cfg.Dir == "" && len(cfg.Inline) == 0 {
		return nil, nil
	}
	cep := &compiledErrorPages{byStatus: map[int]*template.Template{}}
	if len(cfg.InterceptUpstream) > 0 {
		cep.intercept = make(map[int]bool, len(cfg.InterceptUpstream))
		for _, s := range cfg.InterceptUpstream {
			cep.intercept[s] = true
		}
	}
	if cfg.Dir != "" {
		base := resolvePath(cfg.Dir, certDir)
		entries, err := os.ReadDir(base)
		if err != nil {
			return nil, fmt.Errorf("errorPages.dir %q: %w", cfg.Dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".html") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			tmpl, err := template.ParseFiles(filepath.Join(base, e.Name()))
			if err != nil {
				return nil, fmt.Errorf("errorPages.dir: parsing %q: %w", e.Name(), err)
			}
			if name == "default" {
				cep.def = tmpl
				continue
			}
			status, err := strconv.Atoi(name)
			if err != nil || status < 100 || status > 599 {
				continue // not a status-named template; ignore
			}
			cep.byStatus[status] = tmpl
		}
	}
	for key, src := range cfg.Inline {
		tmpl, err := template.New(key).Parse(src)
		if err != nil {
			return nil, fmt.Errorf("errorPages.inline[%q]: %w", key, err)
		}
		if key == "default" {
			cep.def = tmpl
			continue
		}
		status, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("errorPages.inline: key %q must be a status code or \"default\"", key)
		}
		cep.byStatus[status] = tmpl
	}
	return cep, nil
}

// globalErrorPages is the settings-level compiled pages, set once per config
// reload via SetErrorPages and consulted live by every host that does not
// configure its own errorPages override. Mirrors geoDB's package-level handle
// (see geo.go): the alternative - threading Settings through buildRouter,
// buildChain and every middleware constructor - would touch far more of the
// data plane for a value that changes only alongside a full config reload.
var globalErrorPages atomic.Pointer[compiledErrorPages]

// SetErrorPages compiles and installs the settings-level error pages. certDir
// confines a dir-relative path exactly like a custom certificate's files. An
// error leaves the previously installed pages in place (compile fully, then
// swap), so a rejected reload never serves a half-updated set; the caller
// should treat a non-nil error as a reload failure.
func SetErrorPages(cfg model.ErrorPagesConfig, certDir string) error {
	cep, err := compileErrorPages(cfg, certDir)
	if err != nil {
		return err
	}
	globalErrorPages.Store(cep)
	return nil
}

func currentErrorPages() *compiledErrorPages { return globalErrorPages.Load() }

// lookupTemplate returns ep's template for status, falling back to ep's own
// "default" template, or nil when ep is nil or has neither.
func lookupTemplate(ep *compiledErrorPages, status int) *template.Template {
	if ep == nil {
		return nil
	}
	if t, ok := ep.byStatus[status]; ok {
		return t
	}
	return ep.def
}

// resolveTemplate looks up status against hostEP (a host's own errorPages
// override), falling back to the settings-level pages when hostEP resolves
// nothing - so "host override wins" for any status/default it actually
// configures, while a host that configures error pages at all but not this
// particular status still benefits from the settings-level fallback.
func resolveTemplate(hostEP *compiledErrorPages, status int) *template.Template {
	if t := lookupTemplate(hostEP, status); t != nil {
		return t
	}
	return lookupTemplate(currentErrorPages(), status)
}

// renderTemplate executes tmpl into a buffer first (never partially written on
// a template error) and returns the rendered bytes.
func renderTemplate(tmpl *template.Template, status int, host, requestID string) ([]byte, error) {
	var buf bytes.Buffer
	data := errorPageData{Status: status, StatusText: http.StatusText(status), Host: host, RequestID: requestID}
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// serveErrorPage writes a gpm-generated error response for status: the
// resolved custom template (hostEP override, else the settings-level pages)
// when one is configured, or writeDefault - today's exact plain-text output -
// when neither is. This is the single seam that keeps an unconfigured host's
// error output byte-identical to its pre-feature behaviour.
func serveErrorPage(w http.ResponseWriter, status int, hostEP *compiledErrorPages, host string, writeDefault func()) {
	tmpl := resolveTemplate(hostEP, status)
	if tmpl == nil {
		writeDefault()
		return
	}
	body, err := renderTemplate(tmpl, status, host, w.Header().Get("X-GPM-Request-Id"))
	if err != nil {
		log.Error().Err(err).Int("status", status).Str("host", host).
			Msg("dataplane: error page template execution failed; falling back to default")
		writeDefault()
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// wantsUpstreamIntercept reports whether status is configured (in hostEP if it
// is non-nil, else the settings-level pages) to also replace the UPSTREAM's own
// response body. A host that sets its own errorPages block is authoritative for
// this decision - its own (possibly empty) interceptUpstream list is used
// as-is, not merged with the settings-level one.
func wantsUpstreamIntercept(hostEP *compiledErrorPages, status int) bool {
	ep := hostEP
	if ep == nil {
		ep = currentErrorPages()
	}
	return ep != nil && ep.intercept[status]
}

// interceptUpstreamResponse replaces resp's body with the configured error page
// when its status is listed in InterceptUpstream and a template resolves for
// it; otherwise it is a no-op and the upstream's own body passes through
// unchanged (today's behaviour, and the default even when errorPages is
// configured). The upstream's own status code is preserved.
func interceptUpstreamResponse(resp *http.Response, hostEP *compiledErrorPages, host string) {
	if !wantsUpstreamIntercept(hostEP, resp.StatusCode) {
		return
	}
	tmpl := resolveTemplate(hostEP, resp.StatusCode)
	if tmpl == nil {
		return
	}
	// No response writer here (this runs inside the reverse proxy's
	// ModifyResponse, before headers reach the client), so RequestID is left
	// empty - the template must treat it as optional, per its documented contract.
	body, err := renderTemplate(tmpl, resp.StatusCode, host, "")
	if err != nil {
		log.Error().Err(err).Int("status", resp.StatusCode).Str("host", host).
			Msg("dataplane: error page template execution failed for interceptUpstream; passing upstream body through")
		return
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.Header.Del("Content-Encoding") // the body is fresh, unencoded HTML
}
