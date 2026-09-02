package ui

import (
	"regexp"
	"strings"
	"testing"
)

// The SPA's save paths are the one place where a rendering bug becomes a data
// loss bug: every PUT is a whole-object replacement, so a key the builder forgets
// is a key the store deletes. The invariants below are string scans over app.js
// for exactly the shapes the round-trip harness proved were missing.

// Every editor must carry ObjectMeta keys it renders no control for (labels,
// tags) from the object as loaded into the object it PUTs. labels is the
// discovery ownership marker: dropping it orphans a host the ingress/docker
// reconciler created and the reconciler then stops managing it.
func TestEditorSavesCarryMetaForward(t *testing.T) {
	js := loadAppJS(t)

	// The shared helper, and the fact that it covers both keys.
	for _, want := range []string{
		"function metaCarryForward(o) {",
		"out.labels = o.labels;",
		"out.tags = arr(o.tags);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("metaCarryForward no longer carries meta forward: missing %q", want)
		}
	}
	// createdAt/updatedAt are the store's, not the form's: carrying them would
	// let a stale editor tab rewrite the audit timestamps.
	for _, unwanted := range []string{"out.createdAt", "out.updatedAt"} {
		if strings.Contains(js, unwanted) {
			t.Errorf("metaCarryForward must not carry %q - the store maintains it", unwanted)
		}
	}

	// The proxy-host editor seeds its body from the loaded host's meta.
	if !strings.Contains(js, "const obj = Object.assign({}, metaCarryForward(h), { name: nm, domains: domains });") {
		t.Error("the proxy-host save no longer seeds its body from the loaded host's meta - labels are dropped on every save")
	}

	// wireEditor is the shared save for nine object kinds, none of which renders
	// a labels or a tags control.
	if !strings.Contains(js, "function wireEditor(section, plural, meta, isNew, origName, stored, buildBody) {") {
		t.Error("wireEditor no longer takes the loaded object, so it cannot carry labels/tags forward")
	}
	if !strings.Contains(js, "Object.assign(body, metaCarryForward(stored));") {
		t.Error("wireEditor no longer applies metaCarryForward to the body it PUTs")
	}
}

var wireEditorCallRe = regexp.MustCompile(`wireEditor\('([a-z]+)', '[a-z-]+', meta, isNew, [^,]+, ([A-Za-z_$][\w$]*), \(`)

// Every wireEditor call site must pass the loaded object. A call that passes the
// buildBody closure in the stored slot would still parse, so the shape is pinned
// here rather than left to review.
func TestEveryWireEditorCallPassesStoredObject(t *testing.T) {
	js := loadAppJS(t)

	calls := strings.Count(js, "wireEditor('")
	matches := wireEditorCallRe.FindAllStringSubmatch(js, -1)
	if len(matches) != calls {
		t.Fatalf("wireEditor call sites: %d found, %d match the (..., stored, buildBody) shape - a call site is not passing the loaded object", calls, len(matches))
	}
	if calls < 9 {
		t.Fatalf("expected at least 9 wireEditor call sites, found %d - has the scan drifted?", calls)
	}
	want := map[string]bool{
		"redirects": true, "streams": true, "parked": true, "dns": true,
		"access": true, "identity": true, "middleware": true,
		"clientcas": true, "upstreams": true,
	}
	for _, m := range matches {
		delete(want, m[1])
		if m[2] == "meta" || m[2] == "isNew" {
			t.Errorf("wireEditor('%s', ...) passes %q in the stored-object slot", m[1], m[2])
		}
	}
	for section := range want {
		t.Errorf("no wireEditor call site found for section %q", section)
	}
}

// A reference list (middlewares, access lists, upstream groups, identity
// providers, DNS providers, certificates) must never degrade to an empty array.
// An empty picker saved back strips every reference the object already holds, so
// the failure has to stay visible as a third state.
func TestReferenceListsUseTheThreeStateGuard(t *testing.T) {
	js := loadAppJS(t)

	if n := strings.Count(js, ".catch(() => ({ data: [] }))"); n != 0 {
		t.Errorf("%d reference-list prefetch(es) still degrade to an empty array; use refList(path, label) so a failure stays distinguishable from an empty list", n)
	}
	for _, want := range []string{
		"function refList(path, label) {",
		"state.refListFailed.push(label);",
		"function applyRefListGuard(container) {",
		"save is disabled to avoid stripping references",
		"function resetRefListFailures() { state.refListFailed = []; }",
		// route() has to reset the list and apply the guard on every render.
		"  resetRefListFailures();",
		"    applyRefListGuard(c);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("reference-list guard missing %q", want)
		}
	}
	// Every list the host editor picks from goes through the guard.
	for _, want := range []string{
		"refList('/api/middlewares', 'middlewares'),",
		"refList('/api/access-lists', 'access lists'),",
		"refList('/api/upstream-groups', 'upstream groups'),",
		"refList('/api/identity-providers', 'identity providers'),",
		"refList('/api/certificates', 'certificates'),",
		"refList('/api/dns-providers', 'DNS providers'),",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("a reference list is still fetched without the guard: missing %q", want)
		}
	}
	// ...and the save path carries the stored value when a list did not load.
	for _, want := range []string{
		"const mws = mwListOK ? curMw() : arr(h.middlewares);",
		"const als = alListOK ? curAl() : arr(h.accessLists);",
		"if (!ugListOK && h.upstreamGroupRef) {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the host save no longer carries a reference through when its list failed: missing %q", want)
		}
	}
}

// A data-hint written outside its opening tag still matches the registry scan in
// hints_test.go, so both TestHintIDsExistInRegistry and TestNoDeadHintEntries go
// green while the attribute renders as literal text in the admin UI and the
// control gets no "?" button. Four of these shipped. This is the lint that sees
// them.
var hintOutsideTagRe = regexp.MustCompile(`>\s+data-hint=`)

func TestNoDataHintOutsideItsTag(t *testing.T) {
	js := loadAppJS(t)
	for _, m := range hintOutsideTagRe.FindAllStringIndex(js, -1) {
		start := m[0] - 90
		if start < 0 {
			start = 0
		}
		t.Errorf("data-hint written outside its tag (renders as visible text, control gets no \"?\"): ...%s...",
			strings.Join(strings.Fields(js[start:m[1]+40]), " "))
	}
}

// A Settings save nulls the capability cache, and a null cache reads as "every
// capability false". Every route must therefore re-read it before anything
// gates on it, and the save itself must refresh the banners on the view that is
// already on screen.
func TestCapabilitiesAreReloadedAfterASettingsSave(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		// route() refreshes before the view runs.
		"  await loadCapabilities();\n\n  try {",
		// both settings saves re-read rather than only invalidating.
		"      state.capabilities = null;\n      await loadCapabilities();\n      refreshShellBanners();",
		"function refreshShellBanners() {",
		"  applyMaintenanceBanner(c);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("capability refresh after a settings save is missing %q", want)
		}
	}
	if n := strings.Count(js, "state.capabilities = null;\n      await loadCapabilities();"); n != 2 {
		t.Errorf("expected both settings saves to re-read capabilities, found %d", n)
	}
}

// One helper decides what an expired session looks like, and every request goes
// through it - including the two raw fetch() calls that cannot use api().
func TestEveryFetchRedirectsOn401(t *testing.T) {
	js := loadAppJS(t)
	if !strings.Contains(js, "function redirectOn401(res) {") {
		t.Fatal("the shared 401 handler is gone")
	}
	if n := strings.Count(js, "redirectOn401(res);"); n < 3 {
		t.Errorf("redirectOn401 is called %d times; api(), downloadP12() and the /api/restore upload all need it", n)
	}
	// The restore upload is the one that used to surface "Restore failed (401)".
	restore := js[strings.Index(js, "fetch('/api/restore'"):]
	if len(restore) > 400 {
		restore = restore[:400]
	}
	if !strings.Contains(restore, "redirectOn401(res);") {
		t.Error("the /api/restore upload no longer redirects to the login page on 401")
	}
}

// The API Tokens page is admin-scope for reads as well as writes, so a viewer's
// GET comes back 403. Hide the nav entry and answer a direct #/tokens link with
// the reason rather than a generic load error.
func TestAPITokensIsGatedForViewers(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"if (isRoleReadOnly()) { if (item) item.hidden = true; return; }",
		"  if (isRoleReadOnly()) {\n    c.innerHTML = viewHead('API Tokens',",
		"'Your role cannot manage API tokens',",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("API token viewer gating missing %q", want)
		}
	}
	// The gate must come before the fetch.
	view := js[strings.Index(js, "async function viewTokens(c) {"):]
	gate := strings.Index(view, "isRoleReadOnly()")
	fetch := strings.Index(view, "api('/api/api-tokens')")
	if gate < 0 || fetch < 0 || gate > fetch {
		t.Error("viewTokens fetches /api/api-tokens before checking the caller's role")
	}
}

// The insecure-cookie banner. Only "insecure-public" is a finding; the private
// and secure states are the ordinary first-run and LAN cases and must stay quiet.
func TestInsecureCookieBanner(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"function applyInsecureCookieBanner(container) {",
		"if (state.capabilities.adminLogin.cookieSecure !== 'insecure-public') return;",
		"operations/hardening.md#admin-session-cookie",
		"    applyInsecureCookieBanner(c);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("insecure-cookie banner missing %q", want)
		}
	}
}

// absent-stays-absent / present-stays-present, the rule the rest of the save
// builders follow. Both of these materialised a key that was never stored.
func TestSaveBuildersDoNotMaterialiseDefaults(t *testing.T) {
	js := loadAppJS(t)
	if !strings.Contains(js, "if (minTLS && (minTLS !== '1.2' || tls.minTLSVersion === '1.2')) tlsObj.minTLSVersion = minTLS;") {
		t.Error("an explicitly stored tls.minTLSVersion: \"1.2\" is dropped on save again")
	}
	if !strings.Contains(js, "if (!Object.keys(spec).length) { toast('Header rule required'") {
		t.Error("a headers middleware with an empty spec still commits `headers: {}` - guard and rewrite both refuse the equivalent")
	}
}

// A gated <a> has no disabled property, so without this it stays reachable with
// Tab+Enter behind a pointer-events:none that only stops the mouse.
func TestGatedAnchorsLeaveTheTabOrder(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"if (el.tagName === 'A') {",
		"el.dataset.gatedHref = el.getAttribute('href'); el.removeAttribute('href');",
		"el.setAttribute('tabindex', '-1');",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("gateControl no longer neutralises a gated anchor: missing %q", want)
		}
	}
}

// The shell and the view that follows it both want these three GETs; without a
// memo a cold #/overview load fetched each of them twice.
func TestPerRouteRequestMemo(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"function memoGet(path) {",
		"function resetRouteMemo() { state.routeMemo = {}; }",
		"  resetRouteMemo();",
		"    ok(memoGet('/api/history')),",
		"    ok(memoGet('/api/settings')),",
		"    const summary = (await memoGet('/api/config/summary')).data || {};",
		"function routeSettings() {",
		"function routeHistory() {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("per-route request memo missing %q", want)
		}
	}
	// A settings save supersedes the memoised settings object.
	if n := strings.Count(js, "      // The per-route memo now holds a settings object this save superseded.\n      resetRouteMemo();"); n != 3 {
		t.Errorf("expected all 3 settings PUT sites to drop the memo, found %d", n)
	}
}

// The admin panel must make no third-party request: it routinely runs on a
// management VLAN with no outbound internet, and a webfont <link> also sends the
// operator's IP and Referer to a third party on every page view.
func TestNoExternalOriginsInTheBundle(t *testing.T) {
	for _, name := range []string{"static/index.html", "static/app.css", "static/app.js", "static/theme-init.js"} {
		b, err := staticFS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		s := string(b)
		for _, bad := range []string{"fonts.googleapis.com", "fonts.gstatic.com", "//cdn.", "https://unpkg", "cdnjs."} {
			if strings.Contains(s, bad) {
				t.Errorf("%s references the external origin %q", name, bad)
			}
		}
	}
	// The three faces app.css declares have to actually be in the bundle.
	for _, f := range []string{
		"static/fonts/inter-latin.woff2",
		"static/fonts/space-grotesk-latin.woff2",
		"static/fonts/jetbrains-mono-latin.woff2",
	} {
		if _, err := staticFS.ReadFile(f); err != nil {
			t.Errorf("vendored font %s is missing from the embedded bundle: %v", f, err)
		}
	}
	css, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(css), "@font-face"); n != 3 {
		t.Errorf("app.css declares %d @font-face blocks, want 3 (one variable file per family)", n)
	}
}
