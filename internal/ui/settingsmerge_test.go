package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// The settings PUT is a FULL REPLACEMENT of the ingressDiscovery block, but the
// Settings form renders only three of TLSSettings' fields (certificateRef,
// forceSSL, http2). Rebuilding the tls object from those three therefore STRIPS
// clientAuth (mTLS), minTLSVersion and hsts from the template and from every
// profile on any unrelated save - and the next reconcile pushes that silent
// downgrade onto every derived host. The save handler must merge over the loaded
// object instead.
//
// This is a text assertion because there is no JS test toolchain here (and
// adding one for a three-line invariant is not worth a second build system). It
// is deliberately cheap: it catches the regression re-appearing, which has
// already happened once for the template block and once for profiles.
func TestSettingsSaveMergesDiscoveryTLS(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(b)

	for _, want := range []string{
		// the default `template` block
		"tls: Object.assign({}, idt.tls, {",
		// each named profile row
		"tls: Object.assign({}, (row._orig || {}).tls, {",
		// timeouts is the second nested object on the template, and gets the same
		// treatment for the same reason: HostTimeouts can grow a field this form
		// does not render, and a rebuild would drop it on every unrelated save.
		"timeouts: timeoutsPayload(idt.timeouts,",
		"timeouts: timeoutsPayload((row._orig || {}).timeouts,",
		"return Object.assign({}, orig, { connectSeconds: c, readSeconds: r });",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js no longer merges a nested discovery object (%q not found); a settings save now strips fields this form does not render", want)
		}
	}
	// And the row has to actually carry the loaded profile, or the merge above is
	// a merge with nothing.
	if !strings.Contains(js, "div._orig = p;") {
		t.Fatal("app.js no longer stashes the loaded profile on its row; the tls merge would silently become a rebuild")
	}
}

// The other half of the same failure mode: a field that exists on
// IngressHostTemplate but has no control in the Settings form. The save is a full
// replacement, so an operator who sets robotsNoIndex or tags in git and then
// touches anything else in the UI would have it silently cleared. Every flat
// template field must therefore be both rendered and sent back, for the default
// block AND for each profile row.
func TestSettingsSaveSendsEveryFlatTemplateField(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(b)

	for _, want := range []string{
		// default `template` block: rendered, then sent
		"switchHtml('set-id-robots'",
		"robotsNoIndex: isOn('set-id-robots')",
		"$('#set-id-tags')",
		"tags: idTagCtl.get()",
		// stripResponseHeaders has no control on this form, so it is only
		// carried forward - but the literal is a REBUILD, so leaving it out
		// strips the template's list on every unrelated save and the next
		// reconcile pushes that onto every derived host.
		"stripResponseHeaders: arr(idt.stripResponseHeaders).length ? idt.stripResponseHeaders : undefined,",
		"securityHeaders: Object.keys(idt.securityHeaders || {}).length ? idt.securityHeaders : undefined,",
		// each named profile row
		"switchHtml('pf-robots-' + i",
		"robotsNoIndex: isOn('pf-robots-' + uid)",
		"div.querySelector('.pf-tags')",
		"tags: row._tags.get()",
		"stripResponseHeaders: arr((row._orig || {}).stripResponseHeaders).length ? (row._orig || {}).stripResponseHeaders : undefined,",
		"securityHeaders: Object.keys((row._orig || {}).securityHeaders || {}).length ? (row._orig || {}).securityHeaders : undefined,",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js no longer round-trips a discovery template field (%q not found); a settings save now clears it", want)
		}
	}
}

// The fleet TLS floor is a select on the General tab whose value has to reach
// the PUT body: without the send it renders as a control that silently does
// nothing, and "1.2" must stay absent so an untouched settings.yaml does not
// gain a `tls:` block on every save.
func TestFleetTLSFloorEditorWiring(t *testing.T) {
	js := readHostEditorJS(t)

	for _, want := range []string{
		`<select class="field mono" id="set-mintls" data-hint="settings.tls.minVersion" data-path="tls.minVersion">`,
		"const stls = s.tls || {};",
		"const fleetMinTLS = $('#set-mintls').value;",
		"if (fleetMinTLS === '1.3') body.tls = { minVersion: '1.3' };",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the fleet TLS floor control is not wired: missing %q", want)
		}
	}
}

// TestSecurityHeadersEditorRenders covers the editor that closed the "config- and
// API-only" gap on securityHeaders: a row of name/value/scope in BOTH places the
// field exists - the Settings page (the fleet default) and the host editor (the
// per-key override) - fed by the one shared control.
func TestSecurityHeadersEditorRenders(t *testing.T) {
	js := readHostEditorJS(t)

	for _, want := range []string{
		// the shared row editor, its three scopes and the name check
		"function makeSecurityHeaderRows(wrap, initial) {",
		"const SECURITY_SCOPES = ['all', 'generated-only', 'proxied-only'];",
		"const HEADER_NAME_RE = ",
		// name / value / scope controls per row
		`class="field mono sh-name"`,
		`class="field mono sh-value"`,
		`class="field mono sh-scope"`,
		`class="icon-btn sh-del"`,
		// Settings page: container, add button, wiring
		`<div id="set-secheaders" data-hint="settings.securityHeaders" data-path="securityHeaders"></div>`,
		`id="set-secheaders-add"`,
		"const secHdrCtl = makeSecurityHeaderRows($('#set-secheaders'), s.securityHeaders);",
		// host editor: container, add button, wiring
		`<div id="f-secheaders" data-hint="proxyHost.securityHeaders" data-path="securityHeaders"></div>`,
		`id="f-secheaders-add"`,
		"const secHdrCtl = makeSecurityHeaderRows($('#f-secheaders'), h.securityHeaders);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the securityHeaders editor is not wired: missing %q", want)
		}
	}
}

// TestSecurityHeadersEditorSerialization is the guard the old carry-forward test
// became. The editor now OWNS the field in both whole-object PUTs, so the
// invariant "a save never drops securityHeaders" is carried by its serialization
// instead of by a copy of the loaded map. Three shapes have to survive a save
// through an untouched form:
//
//	bare string          -> bare string (scope "all" is never gratuitously
//	                        objectified, so a GitOps YAML diff stays empty)
//	{value, scope: ...}  -> {value, scope} for a non-default scope
//	absent               -> absent (never {})
//
// plus a fourth the editor deliberately does not understand: an unrecognized
// shape (a scope this build has never heard of) is emitted verbatim rather than
// flattened or dropped.
func TestSecurityHeadersEditorSerialization(t *testing.T) {
	js := readHostEditorJS(t)

	// scope "all" serializes as a bare string; anything else as {value, scope}.
	if !strings.Contains(js, "out[name] = scope === 'all' ? value : { value: value, scope: scope };") {
		t.Fatal("the securityHeaders editor no longer round-trips an all-scope header as a bare string; every save would rewrite the config into object form")
	}
	// An unrecognized shape is stashed on the row and emitted untouched.
	for _, want := range []string{
		"div._raw = raw;",
		"if ('_raw' in r) { out[name] = r._raw; return; }",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("an unrecognized securityHeaders value is no longer preserved verbatim: missing %q", want)
		}
	}
	// Empty-name rows are skipped, and no rows at all yields null (not {}).
	for _, want := range []string{
		"const name = r.querySelector('.sh-name').value.trim();\n        if (!name) return;",
		"return Object.keys(out).length ? out : null;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the securityHeaders editor no longer distinguishes empty from absent: missing %q", want)
		}
	}
	// Both saves attach the editor's output ONLY when there is one, so an absent
	// field stays absent instead of becoming an empty map.
	for _, want := range []string{
		"if (secHdrs) body.securityHeaders = secHdrs;",
		"if (secHdrs) obj.securityHeaders = secHdrs;",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("a save no longer sends the securityHeaders editor's output (%q not found); the configured response headers would be wiped", want)
		}
	}
	// The old carry-forward guards must be GONE - keeping one alongside the editor
	// would resurrect a deleted header on the next save.
	for _, gone := range []string{
		"if (s.securityHeaders && Object.keys(s.securityHeaders).length) body.securityHeaders = s.securityHeaders;",
		"if (h.securityHeaders && Object.keys(h.securityHeaders).length) obj.securityHeaders = h.securityHeaders;",
	} {
		if strings.Contains(js, gone) {
			t.Errorf("the securityHeaders carry-forward guard %q still runs alongside the editor; a header removed in the UI would come back on save", gone)
		}
	}
	// Client-side validation of the two mistakes the object build itself would
	// otherwise swallow or bounce off the API.
	for _, want := range []string{
		"if (!HEADER_NAME_RE.test(name)) {",
		"is listed more than once",
		"toast('Invalid security header', secHdrErr, 'err'); return;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the securityHeaders editor no longer validates its rows: missing %q", want)
		}
	}
	// The validation guard's literal is byte-identical in the host and settings
	// save handlers, so Contains alone cannot tell one save losing it: count.
	if got := strings.Count(js, "const secHdrErr = secHdrCtl.error();"); got != 2 {
		t.Errorf("expected the securityHeaders validation guard in BOTH save handlers (host + settings), found %d", got)
	}
}

// TestStripResponseHeadersEditorRenders covers the chip-list editor that closed
// the "config- and API-only" gap on stripResponseHeaders, in BOTH places the
// field exists: the Settings page (the fleet default) and the host editor (the
// host's additions, unioned with it).
func TestStripResponseHeadersEditorRenders(t *testing.T) {
	js := readHostEditorJS(t)

	for _, want := range []string{
		// Settings page: chip container + wiring
		`<div class="chip-input" id="set-striphdrs" data-hint="settings.stripResponseHeaders" data-path="stripResponseHeaders"></div>`,
		"const stripCtl = makeChipInput($('#set-striphdrs'), arr(s.stripResponseHeaders), 'add header...');",
		// host editor: chip container + wiring
		`<div class="chip-input" id="f-striphdrs" data-hint="proxyHost.stripResponseHeaders" data-path="stripResponseHeaders"></div>`,
		"const stripCtl = makeChipInput($('#f-striphdrs'), arr(h.stripResponseHeaders), 'add header...');",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the stripResponseHeaders editor is not wired: missing %q", want)
		}
	}
}

// TestStripResponseHeadersEditorSerialization is the guard the old carry-forward
// test became. The chip editor now OWNS the field in both whole-object PUTs, so
// the invariant "a save never drops stripResponseHeaders" is carried by its
// serialization instead of by a copy of the loaded list - and an emptied list
// leaves the key off the body rather than committing an empty array.
func TestStripResponseHeadersEditorSerialization(t *testing.T) {
	js := readHostEditorJS(t)

	for _, want := range []string{
		"if (stripHdrs.length) body.stripResponseHeaders = stripHdrs;",
		"if (stripHdrs.length) obj.stripResponseHeaders = stripHdrs;",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("a save no longer sends the strip editor's output (%q not found); the configured strip list would be wiped", want)
		}
	}
	// The old carry-forward guards must be GONE - keeping one alongside the
	// editor would resurrect a deleted name on the next save.
	for _, gone := range []string{
		"if (arr(s.stripResponseHeaders).length) body.stripResponseHeaders = s.stripResponseHeaders;",
		"if (arr(h.stripResponseHeaders).length) obj.stripResponseHeaders = h.stripResponseHeaders;",
	} {
		if strings.Contains(js, gone) {
			t.Errorf("the stripResponseHeaders carry-forward guard %q still runs alongside the editor; a name removed in the UI would come back on save", gone)
		}
	}
	// Client-side validation: token syntax and case-insensitive duplicates (the
	// chip input only dedupes exact matches). The refused-name policy stays
	// server-side so the two lists cannot drift.
	for _, want := range []string{
		"function stripHeaderListError(names) {",
		"if (!HEADER_NAME_RE.test(n)) return",
		"is listed more than once (names are case-insensitive).",
		"toast('Invalid strip header', stripErr, 'err'); return;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the stripResponseHeaders editor no longer validates its chips: missing %q", want)
		}
	}
	// The validation guard's literal is byte-identical in the host and settings
	// save handlers, so Contains alone cannot tell one save losing it: count.
	if got := strings.Count(js, "const stripErr = stripHeaderListError(stripHdrs);"); got != 2 {
		t.Errorf("expected the strip validation guard in BOTH save handlers (host + settings), found %d", got)
	}
}

// The same failure mode again, one level down: the proxy-host editor rebuilds
// its `locations` array from the DOM, one block per Location. A Location
// carries middlewares/accessLists (and any field added later) that a rebuild
// with only path/upstream/upstreamGroupRef would silently drop on every save
// of a host whose locations were authored outside the UI (git/import). The
// save handler must merge each rebuilt location over its original, the same way
// the tls block above is merged rather than rebuilt.
//
// Identity for that merge is the ROW (the loaded object stashed on it), not the
// path: keying by path meant renaming a location dropped every field the editor
// does not render, which is the very bug the merge exists to prevent.
func TestHostSaveMergesLocations(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(b)

	for _, want := range []string{
		// the loaded location, stashed on the row that renders it
		"div._orig = loc;",
		"const ctl = {\n      div, p, _orig: loc,",
		// merge over the original location instead of a bare rebuild
		"const orig = Object.assign({}, ctl._orig);",
		"locs.push(Object.assign(orig, loc));",
		// the fold-owned keys must be cleared from the original first, or an
		// off fold could never actually remove a stored block
		"delete orig.auth; delete orig.rateLimit; delete orig.stripPrefix;",
		// per-location middleware/access-list pickers, round-tripped like the
		// host-level ones
		"loc-mw",
		"loc-al",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js no longer merges each location over the object it loaded (%q not found); a host save now strips location fields the editor doesn't render", want)
		}
	}
}

// TestHostEditorRendersMTLSControls guards the gap this feature closed: the host
// editor used to render the "Client certificates (mTLS)" block ONLY when
// tls.clientAuth already existed, and even then caRef/mode were display-only. A
// host could not be put behind mTLS from the UI at all. The block must now always
// render, with real controls.
func TestHostEditorRendersMTLSControls(t *testing.T) {
	js := readHostEditorJS(t)

	// The block is unconditional - no `${tls.clientAuth ? ...}` wrapper.
	if strings.Contains(js, "${tls.clientAuth ? `") {
		t.Error("the mTLS block is conditional on tls.clientAuth again - a host with no clientAuth cannot enable it")
	}
	// The old display-only wording must be gone with it.
	if strings.Contains(js, "this page preserves them") {
		t.Error("caRef/mode are display-only again")
	}
	for _, want := range []string{
		// enable switch, CA picker, mode picker
		`switchHtml('f-mtls', mtlsOn, 'Client certificates', 'proxyHost.tls.clientAuth')`,
		`id="f-mtls-ca"`,
		`id="f-mtls-mode"`,
		// the two modes come from the shared enum-label map, so the select shows
		// a human label with the raw token beside it rather than a bare token
		`enumOptions('mtlsMode', ['require', 'optional']`,
		// the picker is populated from the client-cas list, loaded like the other
		// object pickers this editor already fetches (its three loading states are
		// pinned by TestHostEditorClientCAPickerStates)
		"caList.map((ca) =>",
		// identity passthrough is nested inside the enabled state
		`id="mtls-fields"`,
		`switchHtml('f-certid', certIDOn, 'Identity passthrough', 'proxyHost.tls.clientAuth.identityHeaders')`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("host editor mTLS block missing %q", want)
		}
	}
	// One hint line each, naming the two modes' semantics.
	for _, want := range []string{
		"the handshake rejects certless clients",
		"certless clients still reach the chain",
		"for LAN-exempt enforcement",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("mode control does not explain itself: missing %q", want)
		}
	}
}

// TestHostSaveRoundTripsMTLS pins the save path. Reading the controls is the
// whole point of the feature; merging over the stored object is what keeps a
// GitOps-authored field this form does not render from being stripped.
func TestHostSaveRoundTripsMTLS(t *testing.T) {
	js := readHostEditorJS(t)

	// The switch owns the object, and the merge happens BEFORE the pickers are
	// read. Pinned as one contiguous block: reordering these so the stored values
	// land on top of the picker values would make caRef/mode uneditable again
	// while every substring assertion still passed.
	const mergeFirst = `    if (isOn('f-mtls')) {
      const ca = Object.assign({}, tls.clientAuth);
      delete ca.identityHeaders;
`
	if !strings.Contains(js, mergeFirst) {
		t.Error("the mTLS save no longer merges over the stored clientAuth (and clears identityHeaders) before reading the controls")
	}
	// Belt for the same ordering, in case the block above is reformatted: both
	// picker reads must come after the merge.
	base := strings.Index(js, "const ca = Object.assign({}, tls.clientAuth);")
	for _, want := range []string{"ca.caRef = caRefSel;", "ca.mode = $('#f-mtls-mode').value;"} {
		at := strings.Index(js, want)
		if at < 0 {
			t.Errorf("host save does not round-trip mTLS: missing %q", want)
			continue
		}
		if base < 0 || at < base {
			t.Errorf("%q is applied before the merge over the stored clientAuth, so the stored value would win", want)
		}
	}
	if !strings.Contains(js, "tlsObj.clientAuth = ca;") {
		t.Error("the built clientAuth is never attached to tls")
	}
	// The old preserve-only guard is gone.
	if strings.Contains(js, "it is carried through verbatim; only its identity-passthrough block is edited") {
		t.Error("the save path is preserve-only again - caRef/mode would not be editable")
	}
	// identityHeaders lives INSIDE clientAuth, so turning mTLS off drops it with
	// the object. That is the model's shape, not an oversight - assert the code
	// says so, so nobody "fixes" it by preserving orphaned identity headers.
	if !strings.Contains(js, "which drops identityHeaders with it - correct, they are nested inside") {
		t.Error("the deliberate drop of identityHeaders on mTLS-off is no longer documented at the save path")
	}
	// identityHeaders is merged too, then every rendered field is set explicitly -
	// including the false ones, or a cleared switch could not clear a stored true.
	for _, want := range []string{
		"const ih = Object.assign({}, (tls.clientAuth || {}).identityHeaders);",
		"if (sh) ih.subjectHeader = sh; else delete ih.subjectHeader;",
		"ih.san = isOn('f-certid-san');",
		"ih.serial = isOn('f-certid-serial');",
		"ih.fingerprint = isOn('f-certid-fp');",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("identityHeaders is not merge-then-set-explicitly: missing %q", want)
		}
	}
	// Refusing to save an enabled block with no CA gives a better message than
	// the API's 400 would - but only when the list actually loaded.
	if !strings.Contains(js, "if (!caRefSel) { toast('Client CA required'") {
		t.Error("saving mTLS with no client CA selected is no longer refused client-side")
	}
	if !strings.Contains(js, "      if (caListOK) {\n        const caRefSel = $('#f-mtls-ca').value;") {
		t.Error("caRef/mode are written even when the client CA list failed to load - a failed fetch must never retarget the trust anchor")
	}
}

// TestHostEditorGatesMTLSPreconditions covers the ui-disable-unavailable rule for
// the two things the model requires before tls.clientAuth can validate - forceSSL,
// and a caRef resolving to an enabled ClientCA - plus the direction of the gate.
func TestHostEditorGatesMTLSPreconditions(t *testing.T) {
	js := readHostEditorJS(t)

	for _, want := range []string{
		"function refreshMtls() {",
		"const ssl = isOn('f-forcessl');",
		"No enabled client CA defined yet",
		"require Force SSL",
		// turning forceSSL off under live mTLS is blocked, not silently applied
		"if (!isOn('f-forcessl') && isOn('f-mtls')) {",
		"$('#f-forcessl').setAttribute('aria-checked', 'true');",
		"toast('Force SSL is required'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("mTLS precondition gating missing %q", want)
		}
	}
	// The gate is one-way. Gating both directions traps a host whose stored
	// combination is already invalid: the toggle is the only way out of it.
	if !strings.Contains(js, "gateControl($('#f-mtls'), !reason || isOn('f-mtls'), reason);") {
		t.Error("the mTLS gate must never disable turning mTLS OFF - an invalid stored combination would be unrecoverable from this page")
	}
	// The on-load evaluation is a STANDALONE statement, not the one inside the
	// Force SSL handler - matching "refreshMtls();" alone would pass with the
	// load-time call deleted, leaving the gate unevaluated until something moves.
	if !strings.Contains(js, "\n  refreshMtls();\n") {
		t.Error("refreshMtls is no longer called on load, so the gate is unevaluated until a switch changes")
	}
	if !strings.Contains(js, "$('#f-mtls').addEventListener('switchchange', refreshMtls);") {
		t.Error("the mTLS switch no longer re-evaluates the gate")
	}
}

// TestHostEditorClientCAPickerStates covers the three states the client-CA list
// can be in and the stale-reference case. Each one is a way to silently corrupt a
// host's trust anchor if it is collapsed into another.
func TestHostEditorClientCAPickerStates(t *testing.T) {
	js := readHostEditorJS(t)

	// A failed fetch must NOT read as "no client CAs defined". refList() is the
	// shared three-state loader every reference list now goes through: it
	// resolves to null on failure, never to an empty array.
	if !strings.Contains(js, "refList('/api/client-cas', 'client CAs'),") {
		t.Error("a failed client-cas fetch is collapsed into an empty list again - the picker would state a falsehood and every mTLS save would bail")
	}
	for _, want := range []string{
		"const caListOK = clientCAs !== null;",
		"const caUsable = caList.some((ca) => !ca.disabled);",
		"Client CA list unavailable",
		// the select itself is disabled in that state
		`id="f-mtls-ca" data-hint="proxyHost.tls.clientAuth.caRef" data-path="tls.clientAuth.caRef"${caListOK ? '' : ' disabled'}`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("client CA list state handling missing %q", want)
		}
	}
	// Zero CAs still points at where to make one.
	for _, want := range []string{"no client CAs defined", `<a href="#/clientcas">Client CAs</a>`} {
		if !strings.Contains(js, want) {
			t.Errorf("zero-CA state missing %q", want)
		}
	}
	// A caRef the list does not contain is rendered as itself, selected and
	// flagged, so a save round-trips it instead of the select silently falling
	// through to the first option and retargeting the host's trust anchor.
	for _, want := range []string{
		"const caKnown = caList.some((ca) => ca.name === caRef);",
		"caRef && !caKnown ? `<option value=\"${esc(caRef)}\" selected>${esc(caRef)} (not found)</option>` : ''",
		"is not in the client CA list. It is kept as-is on save",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("a stale caRef is not preserved: missing %q", want)
		}
	}
	// Every interpolated CA name is escaped, in the attribute AND the text node.
	for _, want := range []string{
		`<option value="${esc(ca.name)}"`,
		`>${esc(ca.name)}${ca.disabled ? ' (disabled)' : ''}</option>`,
		`<option value="${esc(caRef)}" selected>${esc(caRef || '(unknown)')}</option>`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("client CA option template is not escaped: missing %q", want)
		}
	}
	// A disabled CA cannot be selected.
	if !strings.Contains(js, "${ca.disabled ? ' disabled' : ''}") {
		t.Error("a disabled client CA is selectable again - the API refuses a host that references one")
	}
}

func readHostEditorJS(t *testing.T) string {
	t.Helper()
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	return string(b)
}

// TestErrorPagesHasItsOwnSection covers the move of error pages out of the
// Settings page into a top-level section. The nav entry, the title and the route
// must all exist, or the section is unreachable.
func TestErrorPagesHasItsOwnSection(t *testing.T) {
	js := readHostEditorJS(t)

	for _, want := range []string{
		"{ id: 'errorpages', label: 'Error Pages', icon: ICON.headers },",
		"errorpages: 'Error Pages',",
		"case 'errorpages': await viewErrorPages(c); break;",
		"async function viewErrorPages(c) {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the Error pages section is not wired: missing %q", want)
		}
	}
	// Its save button joins the explicit HA-follower gating list, like the
	// Settings save it was split out of.
	if !strings.Contains(js, "'#errp-save'") {
		t.Error("#errp-save is not in RO_WRITE_CONTROLS - a follower could start a write from this page")
	}
}

// TestErrorPagesSectionEditsSettings pins the round-trip: the section reads and
// writes settings.errorPages (the config schema is unchanged by the UI move),
// and merges over the loaded settings so saving one field cannot strip the rest.
func TestErrorPagesSectionEditsSettings(t *testing.T) {
	js := readHostEditorJS(t)

	for _, want := range []string{
		// reads settings.errorPages
		"const s = (await api('/api/settings')).data || {};\n  const ep = s.errorPages || {};",
		// the three fields round-trip
		`id="errp-dir"`,
		`id="errp-inline"`,
		`id="errp-intercept"`,
		"if (dir) errp.dir = dir;",
		"errp.inline = JSON.parse(inlineRaw);",
		"if (intercept.length) errp.interceptUpstream = intercept;",
		// writes back through the settings endpoint
		"const r = await api('/api/settings', { method: 'PUT', body });",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the Error pages section does not round-trip settings.errorPages: missing %q", want)
		}
	}
	// A settings PUT replaces the whole object, so this page must merge over what
	// it loaded rather than sending only its own field.
	if !strings.Contains(js, "const body = Object.assign({}, s);") {
		t.Error("the Error pages save does not merge over the loaded settings - it would strip adminAuth, dnsSync and everything else")
	}
	// Clearing every field removes errorPages rather than committing an empty object.
	if !strings.Contains(js, "if (Object.keys(errp).length) body.errorPages = errp; else delete body.errorPages;") {
		t.Error("clearing the Error pages form no longer removes settings.errorPages")
	}
}

// TestSettingsPageDroppedErrorPagesBlock proves the block is gone from Settings
// AND - the part that is easy to get wrong - that the Settings save still
// carries settings.errorPages forward. A settings write is a whole-object
// replacement, so a page that stopped rendering the field without carrying it
// would wipe an operator's error pages on the next unrelated save.
func TestSettingsPageDroppedErrorPagesBlock(t *testing.T) {
	js := readHostEditorJS(t)

	for _, gone := range []string{`id="set-errp-dir"`, `id="set-errp-inline"`, `id="set-errp-intercept"`} {
		if strings.Contains(js, gone) {
			t.Errorf("the Settings page still renders the error-pages control %q", gone)
		}
	}
	if !strings.Contains(js, "if (s.errorPages && Object.keys(s.errorPages).length) body.errorPages = s.errorPages;") {
		t.Fatal("the Settings save no longer carries settings.errorPages forward - saving Settings would wipe the operator's error pages")
	}
	// The pointer to where it went, using the app's in-app link convention.
	if !strings.Contains(js, `<a href="#/errorpages">Error pages</a>`) {
		t.Error("the Settings page does not point at the new Error pages section")
	}
}

// The whole-object failure mode one level up from
// TestSettingsSaveSendsEveryFlatTemplateField: PUT /api/settings replaces the
// ENTIRE Settings object, so any top-level field the Settings page neither
// renders nor carries forward is silently wiped on every save made there. That
// has already shipped twice (appName and accessListSync), so the invariant is
// enforced structurally: every json tag on model.Settings must appear somewhere
// in the save-body region of the Settings page.
//
// Reflection over model.Settings is what makes it carry forward - a field added
// to the struct later fails this test until it is either edited or explicitly
// carried forward by the save handler.
//
// A field that is deliberately not UI-editable belongs in notEditable below WITH
// a reason, and its value must still be carried forward verbatim from the loaded
// settings object so a save never drops it.
func TestSettingsSaveSendsEveryTopLevelSettingsField(t *testing.T) {
	js := readHostEditorJS(t)

	const start = "$('#set-save').addEventListener"
	const end = "const r = await api('/api/settings', { method: 'PUT', body });"
	i := strings.Index(js, start)
	if i < 0 {
		t.Fatalf("app.js no longer has the Settings save handler (%q not found)", start)
	}
	rel := strings.Index(js[i:], end)
	if rel < 0 {
		t.Fatalf("app.js Settings save handler no longer PUTs /api/settings (%q not found after the handler)", end)
	}
	region := js[i : i+rel]

	// Top-level Settings fields with no control on the Settings page. Each one
	// must still appear in the region (as a carry-forward from the loaded `s`),
	// so this map documents WHY there is no editor rather than exempting the
	// field from the check.
	// Every entry has a REAL editor on another page. None is a gap: the Settings
	// save carries the loaded value forward verbatim so a save here cannot wipe
	// what that page owns, and that page's own save does the same in reverse.
	notEditable := map[string]string{
		"errorPages":      "edited in the Error pages section (#/errorpages), carried forward here",
		"dnsSync":         "edited on the Integrations page, DNS sync card, carried forward here",
		"dockerDiscovery": "edited on the Integrations page, Docker discovery card, carried forward here",
		"accessListSync":  "edited on the Integrations page, Access-list sync card, carried forward here",
		"webhooks":        "edited on the Integrations page, Lifecycle webhooks card, carried forward here",
		"notifications":   "edited on the Integrations page, Notifications card, carried forward here",
	}

	rt := reflect.TypeOf(model.Settings{})
	for f := 0; f < rt.NumField(); f++ {
		tag := strings.Split(rt.Field(f).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if strings.Contains(region, tag) {
			continue
		}
		why := notEditable[tag]
		if why == "" {
			why = "add a control for it, or carry it forward from the loaded settings object"
		}
		t.Errorf("settings save body does not mention model.Settings field %q (%s); a Settings save wipes it", tag, why)
	}
}

// Docker discovery renders the SAME IngressHostTemplate as the Kubernetes card,
// so it inherits both merge invariants. Without them a Settings save from the
// Docker card would strip clientAuth / minTLSVersion / hsts from the docker
// template and from every docker profile, and the next reconcile would push that
// silent downgrade onto every container-derived host.
func TestIntegrationsSaveMergesDockerTemplate(t *testing.T) {
	js := readHostEditorJS(t)

	for _, want := range []string{
		// the docker `template` block
		"tls: Object.assign({}, ddt.tls, {",
		"timeouts: timeoutsPayload(ddt.timeouts,",
		// flat template fields: rendered, then sent
		"switchHtml('set-dkr-robots'",
		"robotsNoIndex: isOn('set-dkr-robots')",
		"$('#set-dkr-tags')",
		"tags: dkrTagCtl.get()",
		"stripResponseHeaders: arr(ddt.stripResponseHeaders).length ? ddt.stripResponseHeaders : undefined,",
		"securityHeaders: Object.keys(ddt.securityHeaders || {}).length ? ddt.securityHeaders : undefined,",
		// each named docker profile row
		"switchHtml('dpf-robots-' + i",
		"robotsNoIndex: isOn('dpf-robots-' + uid)",
		"div.querySelector('.dpf-tags')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js no longer round-trips a docker discovery template field (%q not found); a settings save now clears it", want)
		}
	}
}

// The Integrations page owns six settings blocks and renders none of the rest,
// so its save MUST start from the settings object as loaded. Rebuilding the body
// from the form would wipe appName, adminAuth, trustedProxies, securityHeaders,
// maintenance, proxyProtocol and errorPages on every save made there.
func TestIntegrationsSaveCarriesSettingsForward(t *testing.T) {
	js := readHostEditorJS(t)

	const start = "async function saveIntegrations(btn) {"
	const end = "const r = await api('/api/settings', { method: 'PUT', body });"
	i := strings.Index(js, start)
	if i < 0 {
		t.Fatalf("app.js no longer has the Integrations save handler (%q not found)", start)
	}
	rel := strings.Index(js[i:], end)
	if rel < 0 {
		t.Fatalf("the Integrations save handler no longer PUTs /api/settings (%q not found after it)", end)
	}
	region := js[i : i+rel]
	if !strings.Contains(region, "const body = Object.assign({}, s);") {
		t.Fatal("the Integrations save no longer starts from the loaded settings object; a save there would wipe every field Settings owns")
	}
	// Sanity: the blocks this page DOES own are overlaid onto that copy.
	for _, want := range []string{"body.dnsSync =", "body.ingressDiscovery =", "body.dockerDiscovery =", "body.accessListSync =", "body.webhooks =", "body.notifications ="} {
		if !strings.Contains(region, want) {
			t.Errorf("the Integrations save no longer writes %q", want)
		}
	}
}

// The "Load recommended" button pastes model.RecommendedSecurityHeaders into the
// editor. The API does not expose that map, so app.js carries a copy - and a
// copy that drifts is worse than no button at all, since it would recommend a
// header set the project does not document. Reflect over the Go value and
// require every name, value and scope to appear in the JS literal.
func TestRecommendedSecurityHeadersMirrored(t *testing.T) {
	js := readHostEditorJS(t)

	if !strings.Contains(js, "const RECOMMENDED_SECURITY_HEADERS = {") {
		t.Fatal("app.js no longer carries a copy of model.RecommendedSecurityHeaders")
	}
	for name, v := range model.RecommendedSecurityHeaders {
		want := "'" + name + "': { value: " + jsString(v.Value) + ", scope: '" + string(v.Scope) + "' }"
		if !strings.Contains(js, want) {
			t.Errorf("app.js RECOMMENDED_SECURITY_HEADERS drifted from model.RecommendedSecurityHeaders: missing %s", want)
		}
	}
}

// jsString renders a Go string as the JS literal the app.js map uses: single
// quotes normally, double quotes when the value itself contains a single quote
// (Content-Security-Policy's frame-ancestors 'none').
func jsString(s string) string {
	if strings.Contains(s, "'") {
		return `"` + s + `"`
	}
	return "'" + s + "'"
}
