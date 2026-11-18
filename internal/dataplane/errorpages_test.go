package dataplane

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// http.Error (the unconfigured path) sets nosniff itself, so a rendered page has
// to set it too: otherwise CONFIGURING an error page silently drops a header the
// built-in response carried, on every tier.
func TestServeErrorPageSetsNosniff(t *testing.T) {
	withGlobalErrorPages(t, nil)
	ep, err := compileErrorPages(model.ErrorPagesConfig{
		Inline: map[string]string{"default": "<h1>{{.Status}}</h1>"},
	}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	rec := httptest.NewRecorder()
	serveErrorPage(rec, http.StatusForbidden, ep, "myhost", func() {
		t.Fatal("writeDefault must not run when a template resolves")
	})
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
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

// A template outside the allow-list (a loop, a template call, a value built in
// a variable), inline or in errorPages.dir, is skipped at compile (no error, so
// a stored one never fails a reload or the start) and the request gets the
// built-in response.
func TestCompileErrorPagesSkipsDisallowedTemplates(t *testing.T) {
	withGlobalErrorPages(t, nil)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "503.html"), []byte("{{range 100000000000}}{{end}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "502.html"), []byte("<h1>bad gateway</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	ep, err := compileErrorPages(model.ErrorPagesConfig{Dir: ".", Inline: map[string]string{
		"default": `{{$n := 9000000000000000000}}{{range $n}}{{range $n}}x{{end}}{{end}}`,
		"429":     `{{define "a"}}{{template "a"}}{{end}}{{template "a"}}`,
		"500":     `{{$a := printf "%1000000s" ""}}` + strings.Repeat(`{{$a = printf "%s%s" $a $a}}`, 8) + `{{len $a}}`,
		"404":     "<h1>{{.Status}}</h1>",
	}}, dir)
	if err != nil {
		t.Fatalf("a disallowed template must not fail the compile: %v", err)
	}
	if ep.def != nil || ep.byStatus[503] != nil || ep.byStatus[429] != nil || ep.byStatus[500] != nil {
		t.Fatal("disallowed templates must be skipped")
	}
	if ep.byStatus[502] == nil || ep.byStatus[404] == nil {
		t.Fatal("the other templates must still load")
	}
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests, http.StatusInternalServerError} {
		rec := httptest.NewRecorder()
		fellBack := false
		serveErrorPage(rec, status, ep, "myhost", func() {
			fellBack = true
			http.Error(rec, http.StatusText(status), status)
		})
		if !fellBack || rec.Code != status {
			t.Fatalf("status %d: fellBack=%v code=%d, want the built-in response", status, fellBack, rec.Code)
		}
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

// A template whose output passes maxErrorPageBytes stops rendering at the cap
// and the built-in response is served instead, for gpm's own errors and for an
// intercepted upstream status alike. A page just under the cap still renders.
func TestErrorPageRenderIsSizeCapped(t *testing.T) {
	withGlobalErrorPages(t, nil)
	ep, err := compileErrorPages(model.ErrorPagesConfig{
		Inline: map[string]string{
			// In a URL query each padding space is escaped to %20, so 128
			// actions pass the 1 MiB cap within the template size limit.
			"default": strings.Repeat(`<a href="/?{{printf "%999d%999d%999d%999d" .Status .Status .Status .Status}}">`, 128),
			"404":     strings.Repeat(`{{printf "%0999d" .Status}}`, 100),
		},
		InterceptUpstream: []int{502},
	}, "")
	if err != nil {
		t.Fatalf("compileErrorPages: %v", err)
	}
	fellBack := false
	rec := httptest.NewRecorder()
	serveErrorPage(rec, http.StatusServiceUnavailable, ep, "myhost", func() {
		fellBack = true
		http.Error(rec, "Service Unavailable", http.StatusServiceUnavailable)
	})
	if !fellBack || rec.Code != http.StatusServiceUnavailable || rec.Body.Len() > 100 {
		t.Fatalf("oversized page: fellBack=%v status=%d body=%d bytes; want the built-in response", fellBack, rec.Code, rec.Body.Len())
	}

	rec = httptest.NewRecorder()
	serveErrorPage(rec, http.StatusNotFound, ep, "myhost", func() { t.Fatal("a page under the cap must render") })
	if rec.Body.Len() != 100*999 {
		t.Fatalf("page under the cap: body = %d bytes, want %d", rec.Body.Len(), 100*999)
	}

	resp := &http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("upstream body"))}
	interceptUpstreamResponse(resp, ep, "myhost")
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "upstream body" {
		t.Fatalf("intercepted status with an oversized page: body = %d bytes, want the upstream body", len(body))
	}
}

// The largest page that passes the error-page allow-list, rendered through the
// real render path with the longest field values gpm can supply (Host is an
// object name of at most 253 bytes, StatusText at most 31, and the generated
// RequestID 16; 253 is used for both), stays small in heap and in stack: the
// maximum number of the most expensive printing actions in every context
// (shown, in a comment where html/template drops the output, in a URL or CSS
// value where an unsafe value becomes a short marker), and the deepest
// nesting with the most conditions, each within the 32 KiB template limit.
// Each render runs on a fresh goroutine so its stack growth can be read.
func TestErrorPageWorstCaseRenderIsBounded(t *testing.T) {
	host := strings.Repeat("h", 253)
	rid := strings.Repeat("r", 253)
	cond := `{{if eq .Host .RequestID .Host .Host}}x{{end}}`
	// 128 units, then conditions up to the condition and size limits.
	page := func(pre, unit, post string) string {
		p := pre + strings.Repeat(unit, 128) + post
		n := min(511-strings.Count(p, "{{with"), (model.MaxErrorPageTemplateBytes-len(p))/len(cond))
		return p + strings.Repeat(cond, max(n, 0))
	}
	// 16 levels of nesting, repeated up to the condition limit.
	deep := `{{with .Host}}` + strings.Repeat(`{{if eq . "x" "y" "z"}}{{else if ne . "x"}}`, 7) + `{{if .}}{{.}}{{end}}` +
		strings.Repeat(`{{end}}`, 7) + `{{end}}`
	const wide = `{{printf "%999v%999v%999v%999v<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<" .Host .RequestID .Host .RequestID}}`
	const wideDot = `{{printf "%999v%999v%999v%999v" . . . .}}`
	pages := map[string]string{
		"printf":            page("", wide, ""),
		"printf in comment": page("<!-- ", wide, " -->"),
		"printf in script":  page("<script>", wide, "</script>"),
		"printf in a with":  page("{{with .Host}}", wideDot, "{{end}}"),
		"with in comment":   page("<!-- {{with .Host}}", wideDot, "{{end}} -->"),
		"js in comment":     page("<!-- ", "{{js .Host}}", " -->"),
		"filtered url":      page("", `<a href="{{printf "javascript:%999v%999v%999v%999v" .Host .Host .Host .Host}}">`, ""),
		"url query":         page("", `<a href="/?{{printf "%999v%999v%999v%999v" .Host .Host .Host .Host}}">`, ""),
		"css value":         page("<style>p { color: ", `{{printf "%999x%999x%999x%999x" .Host .Host .Host .Host}}`, " }</style>"),
		"css url":           page("<style>p { background: url(", `{{printf "%999v%999v%999v%999v" .Host .Host .Host .Host}}`, ") }</style>"),
		"unquoted attr":     page("", `<p title={{printf "%999v%999v%999v%999v" .Host .Host .Host .Host}}>`, ""),
		"deepest nesting":   strings.Repeat(deep, 512/16),
	}
	// One render on a fresh goroutine: heap allocated and stack grown.
	measure := func(tmpl *template.Template) (heap, stack uint64, err error) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err = renderTemplate(tmpl, http.StatusNetworkAuthenticationRequired, host, rid)
			runtime.ReadMemStats(&after)
			heap = after.TotalAlloc - before.TotalAlloc
			if after.StackInuse > before.StackInuse {
				stack = after.StackInuse - before.StackInuse
			}
		}()
		<-done
		return heap, stack, err
	}
	var worstHeap, worstStack uint64
	for name, src := range pages {
		if len(src) > model.MaxErrorPageTemplateBytes {
			t.Fatalf("%s: %d bytes, over the template limit", name, len(src))
		}
		tmpl, err := template.New("default").Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := warmErrorPage(tmpl); err != nil {
			t.Fatalf("%s: must pass the allow-list and execute: %v", name, err)
		}
		heap, stack, err := measure(tmpl)
		for range 2 { // the least of three, against other goroutines' noise
			h, s, _ := measure(tmpl)
			heap, stack = min(heap, h), min(stack, s)
		}
		worstHeap, worstStack = max(worstHeap, heap), max(worstStack, stack)
		t.Logf("%s: source %d bytes, err %v, heap %.2f MiB, stack %d KiB", name, len(src), err, float64(heap)/(1<<20), stack>>10)
		if heap > 8<<20 || stack > 256<<10 {
			t.Fatalf("%s: one render used %.2f MiB of heap and %d KiB of stack, want at most 8 MiB and 256 KiB", name, float64(heap)/(1<<20), stack>>10)
		}
	}
	t.Logf("worst: heap %.2f MiB, stack %d KiB per render", float64(worstHeap)/(1<<20), worstStack>>10)
	// The stack reading sees deep nesting: the same render of a page the
	// check refuses (4000 levels) grows the stack past 1 MiB.
	if _, stack, _ := measure(template.Must(template.New("default").Parse(
		`{{with .Host}}` + strings.Repeat(`{{if .}}`, 4000) + "x" + strings.Repeat(`{{end}}`, 4000) + `{{end}}`))); stack < 1<<20 {
		t.Fatalf("an unchecked 4000-level page grew the stack by %d KiB; the measurement does not see stack", stack>>10)
	}

	// The cases the allow-list refuses outright.
	for name, src := range map[string]string{
		"css % #x":          "<style>p { color: " + strings.Repeat(`{{printf "% #x% #x% #x% #x" .Host .Host .Host .Host}}`, 128) + " }</style>",
		"129 actions":       strings.Repeat("{{.Host}}", 129),
		"16000 levels":      `{{with .Host}}` + strings.Repeat(`{{if .}}`, 16000) + "x" + strings.Repeat(`{{end}}`, 16000) + `{{end}}`,
		"256 KiB of <":      strings.Repeat("<", 256<<10),
		"33 KiB of < and a": strings.Repeat("< a", 11<<10),
	} {
		tmpl := template.Must(template.New("default").Parse(src))
		if err := model.CheckErrorPageTemplate(tmpl); err == nil {
			t.Fatalf("%s: must be refused", name)
		}
	}
}

// The one-time escaping cost of a template is paid in compileErrorPages (at
// reload or in the pre-write build check), not by the first request, and for
// the costliest 32 KiB texts it stays under a second.
func TestErrorPageEscapingCostIsPaidAtCompile(t *testing.T) {
	fill := func(pre, unit, post string) string {
		n := (model.MaxErrorPageTemplateBytes - len(pre) - len(post)) / len(unit)
		return pre + strings.Repeat(unit, n) + post
	}
	texts := map[string]string{
		"<":                fill("", "<", ""),
		"< ":               fill("", "< ", ""),
		"textarea of <a":   fill("<textarea>", "<a", "</textarea>"),
		"style of quotes":  fill("<style>", "''", "</style>"),
		"script of quotes": fill("<script>", "''", "</script>"),
		`script of <\/'`:   fill("<script>", `<\/'`, "</script>"),
	}
	var worst time.Duration
	for name, src := range texts {
		start := time.Now()
		ep, err := compileErrorPages(model.ErrorPagesConfig{Inline: map[string]string{"default": src}}, "")
		took := time.Since(start)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		worst = max(worst, took)
		t.Logf("%s: %d bytes compiled in %v", name, len(src), took)
		if took > 2*time.Second {
			t.Fatalf("%s: compiling took %v", name, took)
		}
		if ep.def == nil {
			continue // skipped (an escape error); nothing reaches a request
		}
		start = time.Now()
		rec := httptest.NewRecorder()
		serveErrorPage(rec, http.StatusBadGateway, ep, "myhost", func() { http.Error(rec, "x", http.StatusBadGateway) })
		if first := time.Since(start); first > 50*time.Millisecond {
			t.Fatalf("%s: the first request took %v; the escaping must already be done", name, first)
		}
	}
	t.Logf("worst compile: %v", worst)
}

// A template that does not execute with gpm's field values is skipped at
// compile like one outside the allow-list, and a file in errorPages.dir over
// the template size limit is skipped before it is read.
func TestCompileErrorPagesSkipsFailingAndOversizedTemplates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "502.html"), []byte(strings.Repeat("a", model.MaxErrorPageTemplateBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "504.html"), []byte("<h1>{{.Status}}</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	ep, err := compileErrorPages(model.ErrorPagesConfig{Dir: ".", Inline: map[string]string{
		"500": `{{with .Host}}{{.Status}}{{end}}`, // .Status of a string fails to execute
		"503": strings.Repeat("a", model.MaxErrorPageTemplateBytes+1),
	}}, dir)
	if err != nil {
		t.Fatalf("a failing template must not fail the compile: %v", err)
	}
	if ep.byStatus[500] != nil || ep.byStatus[502] != nil || ep.byStatus[503] != nil {
		t.Fatal("the failing and oversized templates must be skipped")
	}
	if ep.byStatus[504] == nil {
		t.Fatal("the other template must still load")
	}
}
