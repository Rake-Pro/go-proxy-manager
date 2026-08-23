package ui

import (
	"strings"
	"testing"
)

// The clone action and the theme toggle are pure client-side SPA features with
// no server-side counterpart to unit test (the daemon just serves the static
// bundle - see ui.go). Same rationale as TestSettingsSaveMergesDiscoveryTLS:
// no JS test toolchain here, so these are cheap text assertions that catch the
// feature (or its wiring) silently disappearing from the shipped bundle.

// TestStaticBundleHasThemeToggle checks that app.js defines the light/dark
// toggle (persisted under the gpm.theme localStorage key and applied via
// documentElement's data-theme attribute) and that index.html's <head> applies
// a saved choice before first paint via an inline (non-module) script, so a
// saved theme never flashes the other palette on load.
func TestStaticBundleHasThemeToggle(t *testing.T) {
	js, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	for _, want := range []string{
		"gpm.theme",
		"id=\"themeBtn\"",
		"function applyTheme(",
		"function setTheme(",
		"document.documentElement.setAttribute('data-theme', t)",
	} {
		if !strings.Contains(string(js), want) {
			t.Errorf("app.js missing theme toggle marker %q", want)
		}
	}

	html, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	h := string(html)
	head, rest, ok := strings.Cut(h, "</head>")
	if !ok {
		t.Fatalf("index.html has no </head>")
	}
	if !strings.Contains(head, "<script>") {
		t.Fatalf("index.html <head> has no inline <script> (a data-theme flash guard must run before first paint)")
	}
	if !strings.Contains(head, "gpm.theme") || !strings.Contains(head, "data-theme") {
		t.Fatalf("index.html <head> script does not look like it sets data-theme from the gpm.theme choice")
	}
	// The head's inline script must precede app.css's <link> and app.js's
	// <script type="module">: both are what determine first paint, so the
	// data-theme attribute has to land before either loads or the flash guard
	// is pointless.
	scriptIdx := strings.Index(head, "<script>")
	cssIdx := strings.Index(head, `<link rel="stylesheet" href="app.css"`)
	if cssIdx == -1 {
		t.Fatalf("index.html <head> has no app.css link to compare ordering against")
	}
	if scriptIdx > cssIdx {
		t.Fatalf("index.html's inline theme script must come before the app.css <link>, or the flash guard can lose the race")
	}
	if !strings.Contains(rest, `<script type="module" src="app.js">`) {
		t.Fatalf("index.html no longer loads app.js as a module script after </head>")
	}
}

// TestStaticBundleHasCloneHelper checks that app.js has ONE shared clone
// implementation (cloneObject/stripSecrets/startClone/takeCloneSeed) reused by
// every editor and list view, rather than each object kind reimplementing its
// own deep-copy-and-blank-the-name logic.
func TestStaticBundleHasCloneHelper(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(b)

	for _, want := range []string{
		"function cloneObject(obj, clearDomains)",
		"function stripSecrets(v)",
		"function startClone(section, obj)",
		"function takeCloneSeed(section)",
		"function wireCloneButton(section, orig, btnId)",
		// the sentinel model.Secret.MarshalJSON uses for a literal secret -
		// this must be the ONLY thing stripSecrets blanks (not every
		// ${ENV:...}/${FILE:...}-shaped string: some of those are legitimate
		// non-secret fields, e.g. a client CA's caPEM).
		"val === '***'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing clone helper marker %q", want)
		}
	}

	// Every list view for a kind with a "Clone" affordance must call the
	// shared startClone helper rather than hand-rolling its own navigation -
	// one call site per kind covers proxy hosts, certificates, and (via
	// genericList) every other listed kind (redirects, streams, parked hosts,
	// access lists, middlewares, upstream groups, identity/DNS providers,
	// client CAs).
	for _, want := range []string{
		"startClone('hosts', h)",
		"startClone('certs', ct)",
		"startClone(section, obj)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js list view no longer wires a Clone action via startClone (%q not found)", want)
		}
	}

	// Every typed object editor wires its save-bar Clone button through the
	// one shared wireCloneButton helper - a per-kind reimplementation here
	// would be exactly the duplication the shared helper exists to avoid.
	for _, section := range []string{
		"hosts", "certs", "redirects", "streams", "parked", "dns",
		"access", "identity", "middleware", "clientcas", "upstreams",
	} {
		if !strings.Contains(js, "wireCloneButton('"+section+"'") {
			t.Errorf("app.js editor for %q does not wire a Clone button via wireCloneButton", section)
		}
	}
}
