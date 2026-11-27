package ui

import (
	"regexp"
	"strings"
	"testing"
)

// The certificate wizard, the CA presets and the certificate editor's tab and
// rail layout. The editor saves with a whole-object PUT, so the picker must
// never rewrite the stored directory URL on its own. Source scans of app.js,
// as in the rest of this package: each guarantee is pinned in the function
// body that carries it, cut out by its start and end markers.

var (
	presetIDRe  = regexp.MustCompile(`\{ id: '([^']+)'`)
	presetURLRe = regexp.MustCompile(`, url: '([^']*)'`)
	// An assignment to the directory URL field's value (not a comparison).
	ctDirWriteRe = regexp.MustCompile(`\$\('#ct-dir'\)\.value\s*=[^=]`)
)

// certEditorJS is the certificate editor's body.
func certEditorJS(t *testing.T, js string) string {
	t.Helper()
	return jsRegion(t, js, "async function certEditor(c, name) {", "\n}\n")
}

// certWizardStep is one step object of the certificate wizard, from its id
// line to the next step's (or the end of the list), split at its validate().
func certWizardStep(t *testing.T, js, id, next string) (render, validate string) {
	t.Helper()
	wz := jsRegion(t, js, "async function runCertWizard() {", "\n}\n")
	step := jsRegion(t, wz, "id: '"+id+"', title:", next)
	i := strings.Index(step, "\n      validate(")
	if i < 0 {
		t.Fatalf("certificate wizard step %q has no validate()", id)
	}
	return step[:i], step[i:]
}

// The preset table holds exactly the six presets and the directory URLs
// confirmed for them; SSL.com's RSA directory is recognised, never written.
func TestCAPresetTable(t *testing.T) {
	js := loadAppJS(t)
	table := jsRegion(t, js, "const CA_PRESETS = [", "\n];")
	var ids, urls []string
	for _, m := range presetIDRe.FindAllStringSubmatch(table, -1) {
		ids = append(ids, m[1])
	}
	for _, m := range presetURLRe.FindAllStringSubmatch(table, -1) {
		urls = append(urls, m[1])
	}
	wantIDs := []string{"letsencrypt", "letsencrypt-staging", "zerossl", "google", "sslcom", "custom"}
	wantURLs := []string{
		"https://acme-v02.api.letsencrypt.org/directory",
		"https://acme-staging-v02.api.letsencrypt.org/directory",
		"https://acme.zerossl.com/v2/DV90",
		"https://dv.acme-v02.api.pki.goog/directory",
		"https://acme.ssl.com/sslcom-dv-ecc",
		"",
	}
	if strings.Join(ids, ",") != strings.Join(wantIDs, ",") {
		t.Errorf("CA_PRESETS ids = %v, want %v", ids, wantIDs)
	}
	if strings.Join(urls, ",") != strings.Join(wantURLs, ",") {
		t.Errorf("CA_PRESETS urls = %v, want %v", urls, wantURLs)
	}
	requireAll(t, "CA_PRESETS", table,
		"also: ['https://acme.ssl.com/sslcom-dv-rsa'], eab: 'optional'",
	)
	if n := strings.Count(table, "eab: 'required'"); n != 2 {
		t.Errorf("expected ZeroSSL and Google to require EAB, found %d required presets", n)
	}
	for _, gone := range []string{"Buypass", "Actalis", "buypass", "actalis"} {
		if strings.Contains(js, gone) {
			t.Errorf("app.js mentions %q, which is not offered", gone)
		}
	}
	// Spellings of one directory compare equal: trimmed, trailing slashes
	// dropped, scheme and host lowercased (the path keeps its case).
	key := jsRegion(t, js, "function caURLKey(url) {", "\n}")
	requireAll(t, "caURLKey", key,
		"String(url || '').trim().replace(/\\/+$/, '')",
		"return m ? m[1].toLowerCase() + m[2] : s;",
	)
	// Blank is Let's Encrypt, an unknown URL is Custom, every recognised
	// spelling (also) goes through caURLKey, and Let's Encrypt writes blank.
	requireAll(t, "caPresetFor", jsRegion(t, js, "function caPresetFor(url) {", "\n}"),
		"const k = caURLKey(url);",
		"if (!k) return CA_PRESETS[0];",
		"[p.url].concat(p.also || []).some((u) => caURLKey(u) === k)",
		"|| CA_PRESETS[CA_PRESETS.length - 1];",
	)
	requireAll(t, "caPresetURL", jsRegion(t, js, "function caPresetURL(id, current) {", "\n}"),
		"if (id === 'letsencrypt') return '';",
		"if (id === 'custom') return current || '';",
	)
}

// The picker is derived from the stored URL and writes the URL field only in
// its own change listener: nothing else in the editor, the load path
// included, assigns the field's value.
func TestCertPickerNeverWritesAtLoad(t *testing.T) {
	js := loadAppJS(t)
	ed := certEditorJS(t, js)
	requireAll(t, "certificate editor", ed,
		"let preset = caPresetFor(acme.directoryURL);",
		`id="ct-dir" data-hint="certificate.acme.directoryURL" data-path="acme.directoryURL" value="${esc(acme.directoryURL || '')}"`,
	)
	const listenerStart = "$('#ct-ca').addEventListener('change', () => {"
	onChange := jsRegion(t, ed, listenerStart, "\n  });")
	if !ctDirWriteRe.MatchString(onChange) || !strings.Contains(onChange, "else $('#ct-dir').value = caPresetURL(p.id, '');") {
		t.Error("the picker's change listener no longer writes the URL field")
	}
	rest := strings.Replace(ed, onChange, "", 1)
	if loc := ctDirWriteRe.FindStringIndex(rest); loc != nil {
		t.Errorf("the certificate editor writes the directory URL field outside the picker's change listener: %q", rest[loc[0]:min(len(rest), loc[1]+60)])
	}
	// Typing in the URL field re-syncs the picker, never the other way round;
	// emptying it while the picker is on Custom keeps Custom, so the save's
	// blank-URL refusal cannot be skipped by clearing the field.
	onInput := jsRegion(t, ed, "$('#ct-dir').addEventListener('input', () => {", "\n  });")
	requireAll(t, "URL re-sync", onInput, "const p = caPresetFor($('#ct-dir').value);")
	keep := strings.Index(onInput, "if (!$('#ct-dir').value.trim() && preset.id === 'custom') return;")
	resync := strings.Index(onInput, "$('#ct-ca').value = p.id;")
	if keep < 0 || resync < 0 || keep > resync {
		t.Error("emptying the URL field on Custom must return before the picker is re-synced")
	}
}

// A file error from the server names no field: the typed path or its last
// segment that the message contains (longest first) picks the field, the
// key file's match maps to the key file, and a message about the private key
// goes to the key file when nothing else matched.
func TestCertEditorFileErrorMapping(t *testing.T) {
	js := loadAppJS(t)
	save := jsRegion(t, js, "$('#ct-save').addEventListener('click', async () => {", "\n  });")
	catchBlock := jsRegion(t, save, "} catch (e) {", "btn.disabled = false;")
	requireAll(t, "certificate save error mapping", catchBlock,
		"const hit = [['#ct-certfile', obj.custom.certFile], ['#ct-keyfile', obj.custom.keyFile]]",
		".flatMap(([sel, v]) => [[sel, v], [sel, v.split(/[\\\\/]/).pop()]])",
		".filter(([, v]) => v && msg.includes(v)).sort((x, y) => y[1].length - x[1].length)[0];",
		"if (hit) field = $(hit[0]);",
		"else if (/^(?:certificate \"[^\"]*\":\\s*)?custom:/.test(msg)) field = $('#ct-certfile');",
		"else if (/private key/i.test(msg)) field = $('#ct-keyfile');",
	)
}

// EAB lock: a required-EAB preset locks the switch on only when the operator
// picked it, on a new certificate, or when EAB is already on. The load path
// passes picked=false, so a stored certificate with such a URL and EAB off is
// shown as stored and left unlocked.
func TestCertEditorEABLockRule(t *testing.T) {
	js := loadAppJS(t)
	ed := certEditorJS(t, js)
	apply := jsRegion(t, ed, "function applyCA(p, picked) {", "\n  }")
	requireAll(t, "applyCA", apply,
		"const lock = p.eab === 'required' && (picked || isNew || isOn('ct-eab'));",
		"if (lock) sw.setAttribute('aria-checked', 'true');",
		"gateControl(sw, !lock, caEABText(p));",
	)
	if strings.Contains(apply, "ct-dir") {
		t.Error("applyCA, which also runs at load, touches the directory URL field")
	}
	if !strings.Contains(ed, "\n  applyCA(preset, false);\n") {
		t.Error("the editor's load path no longer calls applyCA(preset, false)")
	}
	if strings.Contains(ed, "applyCA(preset, true)") {
		t.Error("the editor applies the CA at load as if the operator had picked it")
	}
}

// Custom with a blank URL would save Let's Encrypt; the save refuses it on
// the URL field before anything is sent.
func TestCertEditorRefusesBlankCustomURL(t *testing.T) {
	js := loadAppJS(t)
	save := jsRegion(t, js, "$('#ct-save').addEventListener('click', async () => {", "const r = await api('/api/certificates/'")
	requireAll(t, "certificate save", save,
		"const dir = $('#ct-dir').value.trim(); if (dir) a.directoryURL = dir;",
		"if (!dir && $('#ct-ca').value === 'custom') { showInvalid('Directory URL required', \"Enter the CA's directory URL, or pick Let's Encrypt.\", $('#ct-dir')); return; }",
	)
}

// The editor's tab set follows the type: ACME has Certificate, Authority and
// Validation, custom has Certificate and Files, and a type change rebuilds the
// band's buttons and wires them again.
func TestCertEditorTabsPerType(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "certificate tabs", js,
		"acme: [['cert', 'Certificate'], ['authority', 'Authority'], ['validation', 'Validation']],",
		"custom: [['cert', 'Certificate'], ['files', 'Files']],",
	)
	ed := certEditorJS(t, js)
	requireAll(t, "certificate editor layout", ed,
		"certTabsHtml(type))}",
		`<div class="edit-grid">`,
		`<div class="edit-main">`,
		`<aside class="rail">`,
		`<div class="card rail-card"><dl class="dl" id="ct-rail">${certRailHtml(ct, isNew)}</dl></div>`,
		"rail.innerHTML = certRailHtml(fresh, false);",
		// a clone seed without an origin name gets no "-copy" placeholder
		"placeholder: seed && seed.origName ? seed.origName + '-copy' : 'wild'",
	)
	onType := jsRegion(t, ed, "$('#ct-type').addEventListener('change', () => {", "\n  });")
	rebuild := strings.Index(onType, "$('#pageTabs').innerHTML = certTabsHtml($('#ct-type').value);")
	wire := strings.Index(onType, "wirePageTabs('cert');")
	if rebuild < 0 || wire < rebuild {
		t.Error("a type change must rebuild the band's tabs and then wire them (wirePageTabs)")
	}
	// The save bar sits inside edit-main, before the rail.
	if strings.Index(ed, `id="ct-save"`) > strings.Index(ed, `<aside class="rail">`) {
		t.Error("the certificate save bar is outside edit-main")
	}
	rail := jsRegion(t, js, "function certRailHtml(ct, isNew) {", "\n}")
	requireAll(t, "certificate rail", rail,
		"row('Status', certStateChip(ct)",
		"(acme && ct.lastError ? row('Last error', `<span class=\"err-text\">${esc(ct.lastError)}</span>`) : '')",
		"row('Config file',",
	)
	for _, gone := range []string{`id="ct-status"`, `id="acme-fields"`, `id="custom-fields"`, "Required by ZeroSSL and Google Public CA"} {
		if strings.Contains(js, gone) {
			t.Errorf("app.js still contains %q", gone)
		}
	}
}

// The wizard's steps: the existing-name check, the CA rules decided from the
// URL that will be written, wildcard forcing, the bundle only for Custom, and
// a finish that waits for the Review dry run.
func TestCertWizardSteps(t *testing.T) {
	js := loadAppJS(t)
	wz := jsRegion(t, js, "async function runCertWizard() {", "\n}\n")
	requireAll(t, "certificate wizard", wz, "const existing = new Set(arr(certsR).map((ct) => ct.name));")

	_, domainsValidate := certWizardStep(t, js, "domains", "id: 'authority', title:")
	requireAll(t, "Domains step", domainsValidate,
		"const bad = domainsProblem(d.domains);",
		"if (existing.has(d.name)) return",
	)

	authRender, authValidate := certWizardStep(t, js, "authority", "id: 'validation', title:")
	onRadio := jsRegion(t, authRender, "pane.querySelectorAll('input[name=\"wz-ct-ca\"]').forEach((r) => r.addEventListener('change', () => {", "\n        }));")
	requireAll(t, "Authority radio change", onRadio,
		"if (np.id !== 'custom') dirEl.value = caPresetURL(np.id, '');",
		// Custom never inherits the URL of the preset passed before it.
		"else if (caPresetFor(dirEl.value).id !== 'custom') dirEl.value = '';",
	)
	requireAll(t, "Authority validate", authValidate,
		"const ca = caPresetFor(d.dirURL);",
		"if (d.preset === 'custom' && !d.dirURL.trim()) return",
		"if (ca.eab === 'required' && !d.eab) return",
		"if (d.eab && (!d.kid || !d.hmac)) return",
	)
	if strings.Contains(authValidate, "presetOf(d).eab") {
		t.Error("the required-EAB rule is keyed on the radio, not on the URL that will be written")
	}

	valRender, valValidate := certWizardStep(t, js, "validation", "id: 'review', title:")
	requireAll(t, "Validation render", valRender,
		"if (w) d.challenge = 'dns-01';",
		`value="http-01"${d.challenge === 'http-01' ? ' checked' : ''}${w ? ' disabled' : ''}`,
		// no provider: dns-01 disabled with the reason, unless it is the
		// current choice (a wildcard or an imported draft); the hint names a
		// wildcard only when there is one
		"const noDNS = dnsR !== null && !providers.length;",
		"const dnsOff = noDNS && !w && d.challenge !== 'dns-01';",
		`value="dns-01"${d.challenge === 'dns-01' ? ' checked' : ''}${dnsOff ? ' disabled' : ''}`,
		"${w ? 'A wildcard needs dns-01, and dns-01' : 'dns-01'} needs a DNS provider",
	)
	requireAll(t, "Validation validate", valValidate,
		"if (d.challenge !== 'dns-01') return wild(d) ? 'A wildcard domain can only be validated over dns-01.' : '';",
		"if (!providers.length) return",
	)

	reviewRender, reviewValidate := certWizardStep(t, js, "review", "\n  ];")
	requireAll(t, "Review render", reviewRender,
		"reviewPending = true;",
		// the answer clears pending and a leftover wait message
		"const answered = () => {\n          reviewPending = false;",
		"if (el && el.textContent === REVIEW_WAIT) el.textContent = '';",
		// a failure with no HTTP status never reached the server
		"reviewOffline = !(e && e.status);",
		"<b>The server could not be reached.</b>",
	)
	// Only the current render's answer counts: both answers (success and
	// rejection) clear pending after the stale-reply check, never before.
	if n := strings.Count(reviewRender, "reviewPending = false;"); n != 1 {
		t.Errorf("reviewPending is cleared %d times in the Review render; only answered() may", n)
	}
	if n := strings.Count(reviewRender, "if (tok !== reviewSeq || !pane.isConnected) return null;\n          answered();"); n != 2 {
		t.Errorf("both dry-run answers must call answered() right after the stale-reply check; found %d", n)
	}
	if n := strings.Count(reviewRender, "answered();"); n != 2 {
		t.Errorf("answered() is called %d times; an extra call could clear a newer render's pending state", n)
	}
	requireAll(t, "certificate wizard", jsRegion(t, js, "async function runCertWizard() {", "\n}\n"),
		"const REVIEW_WAIT = 'Wait for the server check to finish.';")
	requireAll(t, "Review validate", reviewValidate,
		"if (reviewPending) return REVIEW_WAIT;",
		"if (reviewOffline) return 'The server could not be reached. Go Back, then Next to check again.';",
		"return reviewErr ? 'Fix the rejected step first: ' + reviewErr : '';",
	)

	obj := jsRegion(t, js, "function certWizardObject(d) {", "\n}")
	if n := strings.Count(obj, "a.caBundle ="); n != 1 || !strings.Contains(obj, "if (d.preset === 'custom' && d.caBundle) a.caBundle = d.caBundle;") {
		t.Error("certWizardObject must write acme.caBundle only for the Custom preset")
	}
}

// The wizard writes create-only, maps a 409, dry-runs on Review, hands its
// draft to the full editor and imports through _parse; one click handler
// opens either wizard.
func TestCertWizardMarkers(t *testing.T) {
	js := loadAppJS(t)
	wz := jsRegion(t, js, "async function runCertWizard() {", "\n}\n")
	requireAll(t, "certificate wizard", wz,
		"{ method: 'PUT', body: certWizardObject(d), headers: { 'If-None-Match': '*' } }",
		"if (e && e.status === 409) throw new Error(",
		"'?dryRun=1', { method: 'PUT', body: certWizardObject(d) }",
		"filter((l) => !/^(createdAt|updatedAt):/.test(l))",
		"api('/api/certificates/_parse', { method: 'POST', raw: text, contentType: 'application/yaml' })",
		"cloneSeed = { section: 'certs', origName: '', data: seed };",
		"carry: (d) => toEditor(certWizardObject(d))",
		"refreshHeadSha();",
	)
	imp := jsRegion(t, wz, "importYAML: async (text) => {", "\n    },")
	requireAll(t, "certificate import", imp,
		"if (o.type === 'custom') {\n        return { fullOnly: true, full,",
		// an unknown type is refused, never turned into an ACME draft
		"if (o.type !== 'acme') {\n        throw new Error(",
		// a literal HMAC key (*** from _parse) is not carried into the draft
		"const masked = eab.hmacKey === '***';",
		"hmac: masked ? '' : (eab.hmacKey || '')",
		"A literal EAB HMAC key is not carried over by an import",
	)
	// A fullOnly import never replaces the operator's draft.
	prim := jsRegion(t, js, "function wizard(opts) {", "\n}\n")
	// The imported draft is taken (and the wizard marked dirty) only when the
	// operator continues with it: "Continue in the wizard" or the no-note
	// path. "Back to the steps" and a fullOnly document leave the draft alone.
	parse := jsRegion(t, prim, "main.querySelector('#wz-parse').addEventListener('click', async () => {", "\n      });")
	requireAll(t, "wizard() import", parse,
		"const take = () => { draft = r.draft; dirty = true; return landing(); };",
		"if (go) go.addEventListener('click', take);",
		"await take();",
	)
	if n := strings.Count(parse, "draft = r.draft"); n != 1 {
		t.Errorf("the import assigns the imported draft %d times; only take() may", n)
	}
	if n := strings.Count(parse, "dirty = true"); n != 1 {
		t.Errorf("the import marks the wizard dirty %d times; only take() may", n)
	}
	if strings.Contains(parse, "addEventListener('click', landing)") {
		t.Error("Continue in the wizard lands without taking the imported draft")
	}
	// Typing in the import box does not mark the draft dirty.
	requireAll(t, "wizard() dirty marking", prim,
		"const mark = (e) => { if (!(e && e.target && e.target.id === 'wz-yaml')) dirty = true; setErr(''); };",
		"wrap.addEventListener('input', mark);",
	)
	requireAll(t, "busy guard", js,
		"if (certWizardBusy) return null;",
		"if (hostWizardBusy) return null;",
	)
	// The host wizard shares the domain check instead of a copy of the regex.
	if n := strings.Count(js, `[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/`); n != 1 {
		t.Errorf("the wizard domain regex appears %d times; share domainsProblem", n)
	}
	// One generalised [data-wizard] handler, not one listener per wizard.
	requireAll(t, "wizard click handler", js,
		"const WIZARDS = { hosts: openHostWizard, certs: openCertWizard };",
		"const b = e.target.closest && e.target.closest('[data-wizard]');",
		"if (b.getAttribute('aria-disabled') === 'true') return;",
	)
	if strings.Contains(js, `closest('[data-wizard="hosts"]')`) {
		t.Error("the hosts-only wizard click handler is still there")
	}
	list := jsRegion(t, js, "async function listCerts(c) {", "\n}\n")
	requireAll(t, "certificates list", list,
		`<a class="btn primary" href="#/certs/_new" data-wizard="certs">`,
		"if (es) es.setAttribute('data-wizard', 'certs');",
	)
	requireAll(t, "certNameFrom", jsRegion(t, js, "function certNameFrom(domain) {", "\n}"),
		"if (wild) d = d.slice(2);",
		".replace(/\\./g, '-')",
		"wild ? 'wild-' + base : base",
	)
}

// The proxy host wizard's Add waits for the Review dry run the way the
// certificate wizard's does: same flag, same reason text, cleared only by the
// current render's answer.
func TestHostWizardReviewWaitsForDryRun(t *testing.T) {
	js := loadAppJS(t)
	wz := jsRegion(t, js, "async function runHostWizard() {", "\n}\n")
	requireAll(t, "host wizard", wz,
		"let reviewPending = false;",
		"const REVIEW_WAIT = 'Wait for the server check to finish.';",
	)
	review := jsRegion(t, wz, "id: 'review', title: 'Review',", "\n  ];")
	i := strings.Index(review, "\n      validate(")
	if i < 0 {
		t.Fatal("host wizard Review step has no validate()")
	}
	render, validate := review[:i], review[i:]
	requireAll(t, "host wizard Review render", render,
		"reviewPending = true; // Add waits for the server's answer",
		"const answered = () => {\n          reviewPending = false;",
		"if (el && el.textContent === REVIEW_WAIT) el.textContent = '';",
	)
	if n := strings.Count(render, "if (tok !== reviewSeq || !pane.isConnected) return null;\n          answered();"); n != 2 {
		t.Errorf("both dry-run answers must call answered() right after the stale-reply check; found %d", n)
	}
	if n := strings.Count(render, "answered();"); n != 2 {
		t.Errorf("answered() is called %d times in the host Review render, want 2", n)
	}
	requireAll(t, "host wizard Review validate", validate,
		"if (reviewPending) return REVIEW_WAIT;",
		"if (reviewOffline) return 'The server could not be reached. Go Back, then Next to check again.';",
		"return reviewErr ? 'Fix the rejected step first: ' + reviewErr : '';",
	)
	// A failure without an HTTP status never reached the server; a 4xx stays
	// a rejection. Same wording as the certificate wizard.
	requireAll(t, "host wizard", wz, "let reviewOffline = false;")
	requireAll(t, "host wizard Review render", render,
		"reviewOffline = false;",
		"reviewOffline = !(e && e.status);",
		"? `<div class=\"inline-error\"><b>The server could not be reached.</b><br/>${esc(reviewErr)}. Go Back, then Next to check again.</div>`",
		": `<div class=\"inline-error\"><b>The server rejected this host.</b><br/>${esc(reviewErr)}</div>`;",
	)
	// The Review wait only holds because finish() validates every step,
	// the last one included.
	fin := jsRegion(t, jsRegion(t, js, "function wizard(opts) {", "\n}\n"), "async function finish() {", "\n    }\n")
	requireAll(t, "wizard finish()", fin,
		"for (let i = 0; i < steps.length; i++) {\n        const err = steps[i].validate ? steps[i].validate(draft) : '';\n        if (err) { if (i !== cur) await show(i); setErr(err); return; }\n      }\n      nextBtn.disabled = true;")
}

// Nothing in wizard() writes markup except the known static frames: every
// error and server message goes through setErr (textContent), and the import
// note escapes its text.
func TestWizardWritesNoMessageAsMarkup(t *testing.T) {
	wz := jsRegion(t, loadAppJS(t), "function wizard(opts) {", "\n}\n")
	allowed := map[string]bool{
		"wrap.innerHTML = `": true,
		"wrap.querySelector('#wz-steps').innerHTML = steps.map((st, i) => {": true,
		"main.innerHTML = '';":                        true,
		"main.innerHTML = `<h3>Import from YAML</h3>": true,
		"note.innerHTML = `<div class=\"ro-banner warn\" style=\"margin:16px 0 0\"><b>${r.fullOnly ? 'Not a wizard case.' : 'Partly covered.'}</b> ${esc(r.note)}</div>": true,
	}
	re := regexp.MustCompile(`[^\s;{}]+\.(?:innerHTML|outerHTML)\s*\+?=|insertAdjacentHTML\(|document\.write\(`)
	seen := 0
	for _, loc := range re.FindAllStringIndex(wz, -1) {
		line := wz[loc[0]:]
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		line = strings.TrimRight(line, " ")
		if !allowed[line] {
			t.Errorf("wizard() writes markup outside the known frames: %q", line)
		}
		seen++
	}
	if seen != len(allowed) {
		t.Errorf("found %d markup writes in wizard(), want the %d known ones", seen, len(allowed))
	}
}

// An import the server calls invalid still fills the steps (an incomplete
// document is finished in the wizard). The verdict follows the message of the
// step that objects, as "The pasted YAML: ..."; Review shows none of it (its dry
// run checks the filled draft). The verdict is used nowhere else in the import
// handler, so no form of early return on it can stop the draft being filled.
func TestWizardImportShowsInvalidVerdict(t *testing.T) {
	js := loadAppJS(t)
	wz := jsRegion(t, js, "function wizard(opts) {", "\n}\n")
	parse := jsRegion(t, wz, "main.querySelector('#wz-parse').addEventListener('click', async () => {", "\n      });")
	landing := jsRegion(t, parse, "const landing = () => {", "\n        };")
	decl := "const inv = r.invalid || '';"
	requireAll(t, "wizard import", parse, decl)
	requireAll(t, "wizard import landing", landing,
		"const msg = !inv || err === inv ? err : `${err} The pasted YAML: ${inv}`;",
		"if (err) { doneUpTo = i - 1; return show(i).then(() => setErr(msg)); }",
		"doneUpTo = steps.length - 2;\n          return show(steps.length - 1);",
	)
	if strings.Contains(landing, "The server reports:") {
		t.Error("the verdict is worded as if it were about the draft")
	}
	// Outside the landing walk the verdict is only declared: any other use of
	// it (an early return, a refusal) would act on it before take().
	rest := strings.Replace(strings.Replace(parse, landing, "", 1), decl, "", 1)
	rest = regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(rest, "")
	if m := regexp.MustCompile(`\binv\b|\.invalid\b`).FindString(rest); m != "" {
		t.Errorf("the import handler acts on the verdict outside the landing step (%q)", m)
	}
	// After the walk, nothing on Review touches the error line.
	tail := landing[strings.Index(landing, "doneUpTo = steps.length - 2;"):]
	if strings.Contains(tail, "setErr(") {
		t.Error("Review shows a message from the import")
	}
	// The draft is still replaced only by take(), and an unparseable document
	// (the importer throws) shows its error and fills nothing.
	requireAll(t, "wizard import", parse,
		"const take = () => { draft = r.draft; dirty = true; return landing(); };",
		"catch (e) { setErr(e && e.message ? e.message : String(e)); btn.disabled = false; return; }",
	)
	if a, b := strings.Index(parse, "catch (e) { setErr("), strings.Index(parse, "const take = () =>"); a < 0 || b < a {
		t.Error("the importer's thrown error must return before the draft can be replaced")
	}
	// The verdict and every other message is text, never markup.
	requireAll(t, "wizard()", wz, "function setErr(msg) { errEl.textContent = msg || ''; }")
	if regexp.MustCompile(`errEl\.(innerHTML|outerHTML|insertAdjacentHTML)`).MatchString(wz) {
		t.Error("wizard() writes the error line as markup")
	}
	// Both importers report the verdict under the name the wizard reads.
	for _, fn := range []string{"async function runHostWizard() {", "async function runCertWizard() {"} {
		requireAll(t, fn, jsRegion(t, js, fn, "\n}\n"),
			"invalid: data.valid === false ? (data.error || 'The server could not validate this object.') : '',")
	}
}

// An untouched save of a stored certificate without acme.challenge keeps the
// key absent; a new certificate and a changed or stored value still write it.
func TestCertEditorKeepsImpliedChallengeAbsent(t *testing.T) {
	ed := certEditorJS(t, loadAppJS(t))
	requireAll(t, "certEditor save", ed,
		"const a = { email: $('#ct-email').value.trim() };",
		"if (isNew || acme.challenge || ch !== challenge) a.challenge = ch;",
		// The initial control value is the implied one for a stored certificate.
		"const challenge = (isNew && !seed) ? (dnsProviders.length ? 'dns-01' : 'http-01') : certChallenge(ct);",
		// dnsProvider is written only for dns-01, so the omission keeps the
		// same effective challenge.
		"if (ch === 'dns-01') {\n        a.dnsProvider = $('#ct-dns').value;",
	)
	if strings.Contains(ed, "challenge: ch }") {
		t.Error("the save writes acme.challenge unconditionally")
	}
	// The implied value is the model's EffectiveChallenge: dns-01 when a DNS
	// provider is named, else http-01.
	requireAll(t, "certChallenge", jsRegion(t, loadAppJS(t), "function certChallenge(ct) {", "\n}\n"),
		"if (a.challenge) return a.challenge;",
		"return a.dnsProvider ? 'dns-01' : 'http-01';",
	)
}

// A public CA preset with a private CA bundle warns next to the picker; the
// bundle is never cleared for the operator.
func TestCertEditorBundleWarning(t *testing.T) {
	ed := certEditorJS(t, loadAppJS(t))
	caGroup := jsRegion(t, ed, `<select class="field" id="ct-ca"`, "\n          </div>")
	requireAll(t, "CA field group", caGroup, `<div class="hint warn" id="ct-ca-bundle-warn" hidden>`)
	requireAll(t, "certEditor", ed,
		// From the URL that will be written; blank is Let's Encrypt unless
		// the picker says Custom.
		"const url = $('#ct-dir').value.trim();\n    const ca = url ? caPresetFor(url) : preset;",
		"$('#ct-ca-bundle-warn').hidden = ca.id === 'custom' || !$('#ct-cabundle').value.trim();",
		"$('#ct-cabundle').addEventListener('input', bundleWarn);",
		"$('#ct-dir').addEventListener('input', bundleWarn);",
	)
	if strings.Contains(ed, "$('#ct-ca-bundle-warn').hidden = preset.id === 'custom'") {
		t.Error("the bundle warning follows the picker label, not the URL")
	}
	// The bundle is in the field before the load-time applyCA decides the
	// warning, or the warning is wrong on load.
	if fill, apply := strings.Index(ed, "$('#ct-cabundle').value = acme.caBundle || '';"), strings.Index(ed, "applyCA(preset, false);"); fill < 0 || apply < 0 || fill > apply {
		t.Error("the CA bundle field is filled after the load-time applyCA")
	}
	apply := jsRegion(t, ed, "function applyCA(p, picked) {", "\n  }\n")
	if !strings.Contains(apply, "bundleWarn();") {
		t.Error("applyCA does not refresh the bundle warning")
	}
	if regexp.MustCompile(`\$\('#ct-cabundle'\)\.value\s*=\s*''`).MatchString(ed) {
		t.Error("the editor clears the CA bundle on its own")
	}
}

// The Files tab's cert store line is a hint inside a field group, so it takes
// the hint size like the editor's other hints.
func TestCertEditorFilesHintInGroup(t *testing.T) {
	ed := certEditorJS(t, loadAppJS(t))
	grp := jsRegion(t, ed, `<div class="field-group"><label>Certificate file</label>`, "\n          </div>")
	requireAll(t, "Certificate file group", grp, `<div class="hint">Paths are relative to the cert store`)
	if strings.Contains(ed, `<div class="hint" style="margin-bottom:8px">Paths are relative`) {
		t.Error("the cert store line still sits outside a field group")
	}
}
