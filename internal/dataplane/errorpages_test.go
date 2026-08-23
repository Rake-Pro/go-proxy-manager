package dataplane

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// withGlobalErrorPages installs cep as the settings-level pages for the
// duration of the test and restores whatever was there before on cleanup, so
// tests never leak state into each other via the package-level handle.
func withGlobalErrorPages(t *testing.T, cep *compiledErrorPages) {
	t.Helper()
	prev := globalErrorPages.Load()
	globalErrorPages.Store(cep)
	t.Cleanup(func() { globalErrorPages.Store(prev) })
}

func TestServeErrorPageUnconfiguredMatchesLegacyOutput(t *testing.T) {
	withGlobalErrorPages(t, nil)
	rec := httptest.NewRecorder()
	called := false
	serveErrorPage(rec, http.StatusForbidden, nil, "myhost", func() {
		called = true
		http.Error(rec, "Forbidden", http.StatusForbidden)
	})
	if !called {
		t.Fatal("writeDefault was not called for an unconfigured host")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if got := rec.Body.String(); got != "Forbidden\n" {
		t.Fatalf("body = %q, want the untouched http.Error output", got)
	}
}

func TestServeErrorPagePerStatusTemplate(t *testing.T) {
	withGlobalErrorPages(t, nil)
	ep, err := compileErrorPages(model.ErrorPagesConfig{
		Inline: map[string]string{"502": "<h1>{{.Status}} {{.StatusText}}</h1>"},
	}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	rec := httptest.NewRecorder()
	serveErrorPage(rec, http.StatusBadGateway, ep, "myhost", func() {
		t.Fatal("writeDefault must not run when a template resolves")
	})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if got := rec.Body.String(); got != "<h1>502 Bad Gateway</h1>" {
		t.Fatalf("body = %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestServeErrorPageDefaultFallback(t *testing.T) {
	withGlobalErrorPages(t, nil)
	ep, err := compileErrorPages(model.ErrorPagesConfig{
		Inline: map[string]string{"default": "<p>default page host={{.Host}}</p>"},
	}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	rec := httptest.NewRecorder()
	// 503 has no dedicated template - the "default" entry must fill in.
	serveErrorPage(rec, http.StatusServiceUnavailable, ep, "myhost", func() {
		t.Fatal("writeDefault must not run when the default template resolves")
	})
	if got := rec.Body.String(); got != "<p>default page host=myhost</p>" {
		t.Fatalf("body = %q", got)
	}
}

func TestServeErrorPageHostOverrideWinsOverSettings(t *testing.T) {
	global, err := compileErrorPages(model.ErrorPagesConfig{Inline: map[string]string{"502": "global"}}, "")
	if err != nil {
		t.Fatalf("compileErrorPages(global): %v", err)
	}
	withGlobalErrorPages(t, global)
	host, err := compileErrorPages(model.ErrorPagesConfig{Inline: map[string]string{"502": "host"}}, "")
	if err != nil {
		t.Fatalf("compileErrorPages(host): %v", err)
	}
	rec := httptest.NewRecorder()
	serveErrorPage(rec, http.StatusBadGateway, host, "myhost", func() { t.Fatal("unexpected fallback") })
	if got := rec.Body.String(); got != "host" {
		t.Fatalf("body = %q, want the host override to win over the settings-level page", got)
	}

	// A host with no override still falls back to the settings-level page.
	rec2 := httptest.NewRecorder()
	serveErrorPage(rec2, http.StatusBadGateway, nil, "otherhost", func() { t.Fatal("unexpected fallback") })
	if got := rec2.Body.String(); got != "global" {
		t.Fatalf("body = %q, want the settings-level page for a host with no override", got)
	}
}

func TestServeErrorPageTemplateEscaping(t *testing.T) {
	withGlobalErrorPages(t, nil)
	ep, err := compileErrorPages(model.ErrorPagesConfig{
		Inline: map[string]string{"default": "<p>{{.Host}}</p>"},
	}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	rec := httptest.NewRecorder()
	serveErrorPage(rec, http.StatusForbidden, ep, `<script>alert(1)</script>`, func() {})
	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Fatalf("template output was not escaped: %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("expected html/template to escape the Host value, got %q", body)
	}
}

func TestInterceptUpstreamResponse(t *testing.T) {
	ep, err := compileErrorPages(model.ErrorPagesConfig{
		Inline:            map[string]string{"502": "<h1>upstream down</h1>"},
		InterceptUpstream: []int{502},
	}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": {"text/plain"}, "Content-Encoding": {"gzip"}},
		Body:       http.NoBody,
	}
	interceptUpstreamResponse(resp, ep, "myhost")
	if resp.Header.Get("Content-Encoding") != "" {
		t.Fatal("Content-Encoding must be cleared when the body is replaced")
	}
	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", resp.Header.Get("Content-Type"))
	}
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	if got := string(buf[:n]); got != "<h1>upstream down</h1>" {
		t.Fatalf("body = %q", got)
	}
}

func TestInterceptUpstreamResponseNotConfiguredPassesThrough(t *testing.T) {
	ep, err := compileErrorPages(model.ErrorPagesConfig{
		Inline: map[string]string{"502": "<h1>upstream down</h1>"},
		// InterceptUpstream deliberately empty: the upstream's own 502 body must
		// pass through untouched (today's, and the default, behaviour).
	}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": {"text/plain"}},
		Body:       http.NoBody,
	}
	interceptUpstreamResponse(resp, ep, "myhost")
	if resp.Body != http.NoBody {
		t.Fatal("body must be untouched when the status is not in interceptUpstream")
	}
}

func TestCompileErrorPagesDirAndDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "502.html"), []byte("<h1>bad gateway</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "default.html"), []byte("<h1>default</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Dir ("." ) is relative to certDir (dir), exercising the same
	// resolvePath join a custom certificate's files use.
	ep, err := compileErrorPages(model.ErrorPagesConfig{Dir: "."}, dir)
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	if ep.byStatus[502] == nil {
		t.Fatal("502.html was not loaded")
	}
	if ep.def == nil {
		t.Fatal("default.html was not loaded")
	}
}

func TestCompileErrorPagesParseError(t *testing.T) {
	_, err := compileErrorPages(model.ErrorPagesConfig{
		Inline: map[string]string{"502": "{{.Status"}, // malformed
	}, "")
	if err == nil {
		t.Fatal("expected a parse error for malformed template source")
	}
}

func TestCompileErrorPagesUnconfiguredReturnsNil(t *testing.T) {
	ep, err := compileErrorPages(model.ErrorPagesConfig{}, "")
	if err != nil || ep != nil {
		t.Fatalf("compileErrorPages(empty) = (%v, %v), want (nil, nil)", ep, err)
	}
}

func TestSetErrorPagesFailureLeavesPreviousInstalled(t *testing.T) {
	good, err := compileErrorPages(model.ErrorPagesConfig{Inline: map[string]string{"502": "good"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	withGlobalErrorPages(t, good)
	if err := SetErrorPages(model.ErrorPagesConfig{Inline: map[string]string{"502": "{{.Status"}}, ""); err == nil {
		t.Fatal("expected an error for a malformed inline template")
	}
	if currentErrorPages() != good {
		t.Fatal("a failed SetErrorPages must not replace the previously installed pages")
	}
}
