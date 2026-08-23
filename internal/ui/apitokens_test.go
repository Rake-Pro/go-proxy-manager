package ui

import (
	"strings"
	"testing"
)

// TestStaticBundleScopeTableReadOnlySet checks that the API Tokens scope table
// in app.js renders no write checkbox for a read-only subject, and that its
// read-only set names exactly "metrics" - the only subject with no write
// action (model.ScopeMetricsRead is the sole scope GET /metrics checks; there
// is no metrics:write endpoint - see internal/model/apitoken.go and
// server.go's "GET /metrics" registration). A JS test toolchain doesn't exist
// here, so this is a text assertion like the other static-bundle tests in this
// package: cheap, and it catches the read-only set silently drifting from the
// Go model (e.g. gaining or losing "metrics", or picking up a subject that
// does have a write endpoint).
func TestStaticBundleScopeTableReadOnlySet(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(b)

	if !strings.Contains(js, "const SCOPE_READONLY = new Set(['metrics']);") {
		t.Fatalf("app.js scope table read-only set is not exactly {\"metrics\"} - it must mirror model.ScopeMetricsRead being the only subject with no write action")
	}

	for _, want := range []string{
		// the scope table renders no write checkbox for a read-only subject
		"SCOPE_READONLY.has(p)",
		"class=\"tok-write\"",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing scope table marker %q", want)
		}
	}
}

// TestStaticBundleScopeTableGrouping checks that the scope table groups every
// subject a create form can offer under the labelled sections called for in
// the tokens-page rework (Hosts / Trust & auth / Routing / Operations), rather
// than an unlabelled auto-fill card grid.
func TestStaticBundleScopeTableGrouping(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(b)

	for _, want := range []string{
		"const SCOPE_GROUPS = [",
		"{ label: 'Hosts', subjects: ['proxy-hosts', 'redirect-hosts', 'stream-hosts', 'parked-hosts'] }",
		"{ label: 'Trust & auth', subjects: ['certificates', 'client-cas', 'identity-providers', 'access-lists', 'middlewares'] }",
		"{ label: 'Routing', subjects: ['upstream-groups', 'dns-providers'] }",
		"{ label: 'Operations', subjects: ['settings', 'dns-sync', 'ingress-discovery', 'api-tokens', 'metrics'] }",
		"function groupedScopeSubjects()",
		// write implies read: checking write auto-selects the read box
		"if (r && w.checked) r.checked = true;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing scope table grouping marker %q", want)
		}
	}
}
