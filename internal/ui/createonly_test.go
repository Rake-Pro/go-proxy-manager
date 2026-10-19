package ui

import (
	"regexp"
	"strings"
	"testing"
)

// A PUT is an upsert, so a create that does not say so silently replaces a
// stored object of the same name. Every create path in the SPA sends
// If-None-Match: * (the server answers 409 for a stored name) and renders that
// 409 through showNameTaken. String scans, like the other bundle tests.
func TestCreatePathsAreCreateOnly(t *testing.T) {
	js := loadAppJS(t)
	for site, want := range map[string]string{
		"proxy host editor":  "api('/api/proxy-hosts/' + encodeURIComponent(nm), { method: 'PUT', body: obj, headers: isNew ? { 'If-None-Match': '*' } : {} })",
		"certificate editor": "api('/api/certificates/' + encodeURIComponent(nm), { method: 'PUT', body: obj, headers: isNew ? { 'If-None-Match': '*' } : {} })",
		// wireEditor: redirect, stream, parked, DNS, client CA (paste), IdP,
		// access list, middleware, upstream group.
		"wireEditor":        "api('/api/' + plural + '/' + encodeURIComponent(nm), { method: 'PUT', body, headers: isNew ? { 'If-None-Match': '*' } : {} })",
		"token create":      "api('/api/api-tokens/' + encodeURIComponent(nm), { method: 'PUT', body, headers: { 'If-None-Match': '*' } })",
		"proxy host wizard": "{ method: 'PUT', body: hostWizardObject(d), headers: { 'If-None-Match': '*' } }",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("%s no longer sends If-None-Match: * on create (missing %q)", site, want)
		}
	}
	if !strings.Contains(js, "function showNameTaken(e, singular, name, href, title, nameEl)") {
		t.Fatalf("showNameTaken (the 409 banner) is missing")
	}
	// The four editor create paths render the 409 through it (the wizard has
	// its own footer line).
	if n := strings.Count(js, "showNameTaken(e, "); n < 4 {
		t.Errorf("expected the 409 banner on 4 create paths, found %d", n)
	}
}

// The wizard's "Open the full editor instead" hands the draft over instead of
// dropping it, and Force SSL defaults on only when every domain is covered.
func TestHostWizardCarriesDraft(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"opts.altLink.carry(draft);",
		"cloneSeed = { section: 'hosts', origName: '', data: o };",
		"if (!d.forceSSLTouched) d.forceSSL = every;",
		"if (!dd.forceSSLTouched) dd.forceSSL = allCovered(dd);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing host wizard marker %q", want)
		}
	}
}

// Every confirmation goes through confirmModal: the browser's confirm() has no
// room for a consequence, ignores the theme and starts on the wrong button.
// Line comments are skipped (the confirmModal comment names window.confirm).
func TestNoNativeConfirm(t *testing.T) {
	js := loadAppJS(t)
	for i, line := range strings.Split(js, "\n") {
		code := line
		if j := strings.Index(code, "//"); j >= 0 && !strings.Contains(code[:j], "'") && !strings.Contains(code[:j], "`") {
			code = code[:j]
		}
		if nativeConfirmRe.MatchString(code) {
			t.Errorf("app.js:%d calls the browser's confirm(): %s", i+1, strings.TrimSpace(line))
		}
	}
}

var nativeConfirmRe = regexp.MustCompile(`(^|[^\w])confirm\(`)

// Delete lives on the object's own page, never on a card list, and every
// delete, token rotation and certificate renewal is a titled confirmModal.
func TestDeleteGoesThroughConfirmModal(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"async function deleteObject(section, kind, name, path, btn)",
		"title: `Delete ${kind} ${name}?`",
		"confirmLabel: `Delete ${kind}`",
		"const DELETE_UNDO = 'Recorded as a commit; History can undo it.';",
		"showSaveError(e, `Could not delete ${name}`)",
		"deleteObject('hosts', 'proxy host', h.name,",
		"deleteObject('certs', 'certificate', ct.name,",
		"deleteObject(section, meta.singular, origName,",
		"confirmLabel: 'Rotate token'",
		"confirmLabel: 'Delete token'",
		"confirmLabel: 'Renew certificate'",
		// confirmModal closes on navigation and only its own inputs or confirm
		// button take Enter.
		"window.addEventListener('hashchange', onHash);\n    wrap.addEventListener('click', (e) => { if (e.target === wrap) finish(false); });",
		"if (e.key === 'Enter' && (e.target === okBtn || e.target === typedInput || e.target === promptInput)) {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing delete/confirm marker %q", want)
		}
	}
	if strings.Contains(js, "gs-del") {
		t.Error("the card lists carry a Delete button again (gs-del)")
	}
}

// The wizard batch: the name follows the first domain until it is typed in, a
// declined navigation prompt puts the wizard's own page back, the setup wizard
// settles unsaved edits first and hands off to the host wizard only on an
// instance with no proxy host and only once that page has rendered, a write
// without a commit says "No changes", and one rule per field is shared with
// the editors.
func TestWizardBatchMarkers(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		`data-auto="${d.nameTouched ? '0' : '1'}"`,
		"if (location.hash !== openHash) { restoringHash = true; location.hash = openHash; }",
		"Leaving this page throws the steps away.",
		"The setup can end on the Proxy Hosts page, which discards them.",
		"if (r && firstHost && (await hostCount()) === 0) {",
		"const rendered = nextRouteRendered('#/hosts');",
		"window.dispatchEvent(new CustomEvent('gpm:routed'",
		"toast('No changes', ",
		`${isReadOnly() ? '' : '<button type="button" id="setupBtn">Setup wizard</button>'}`,
		`<div class="hint warn" id="su-tp-warn"`,
		"showInvalid('HSTS needs Force SSL'",
		"filter((l) => !/^(createdAt|updatedAt):/.test(l))",
		"function plural(n, one, many)",
		"function historyText(msg)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing wizard/vocabulary marker %q", want)
		}
	}
	for _, gone := range []string{"Trusting every address lets any visitor set their own client address.", "'Create host'", ">Log out<", "revision.", "(s)</span>"} {
		if strings.Contains(js, gone) {
			t.Errorf("app.js still contains %q", gone)
		}
	}
}

// The lists and shell batch: hosts sort by Domain from the first paint and
// group by the primary domain's zone, tags fold to three chips, the phone
// table keeps Domain and Status, the drawer toggles and holds Tab, the help
// popover sits after its owner, and the main landmark holds the title band.
func TestListsAndShellBatchMarkers(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"let sort = { key: 'domains', dir: 1 };",
		"const zoneOk = !showZones || !r.zone || !zonesOff.has(r.zone);",
		"r.tags.length > 3 ? moreToggle(r.tags.slice(3)",
		"{ key: 'upstream', label: 'Upstream', cls: 'col-upstream hide-sm hide-md' },",
		`<table class="hosts-table">`,
		"Proxy host <b>${esc(arr(h.domains)[0] || h.name)}</b> is in maintenance",
		", ${maintHosts.length} in maintenance",
		"domainFirst: true,",
		"if (v == null || typeof v === 'boolean') return;",
		"aria-controls=\"tok-form\"",
		"tok-toggle",
		"title=\"${esc(fmtTime(e.time, true))}\">${esc(fmtClock(e.time))}</td>",
		".join(', ') || '0'}</span>` +\n      upstreamHealthKv(o, ctx)",
		"b.setAttribute('aria-label', 'Close navigation');",
		"if (m) m.inert = true;",
		`<a class="skip-link" id="skipLink" href="#content">Skip to content</a>`,
		`<main class="main" id="main">`,
		"host.insertAdjacentElement('afterend', el);",
		"el.setAttribute('aria-label', 'Help');",
		`aria-controls="hint-pop"`,
		"function scheduleHintPopClose()",
		`(?:\[\d+\])?`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js missing lists/shell marker %q", want)
		}
	}
	for _, gone := range []string{`aria-haspopup="true"`, `<main class="content"`, "join(' -&gt; ')", "No ${meta.singular === 'middleware'"} {
		if strings.Contains(js, gone) {
			t.Errorf("app.js still contains %q", gone)
		}
	}
	css := readStatic(t, "static/app.css")
	for _, want := range []string{
		"summary:focus-visible, [tabindex]:focus-visible",
		".tab-btn:focus-visible { outline-offset:-2px; }",
		"@media (max-width:800px) {",
		".hosts-table .hide-sm { display:none; }",
		".hosts-table .hide-md { display:none; }",
		// Dark mode separates a popover from the card under it with the hairline
		// ring every --shadow-md carries, not a border.
		"--shadow-md:0 0 0 1px rgba(255,255,255,.09)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css missing lists/shell marker %q", want)
		}
	}
}
