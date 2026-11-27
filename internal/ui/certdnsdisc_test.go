package ui

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// Admin UI for the certificate, DNS-01 and discovery batch: acme.profile and
// acme.caBundle in the certificate editor, renewAt in the certificate status,
// the webhook DNS provider, trustedProxies on both discovery templates and
// every profile row, and bulk tag edits on discovery-managed hosts. Every save
// here is a whole-object replacement, so each field has to be rendered, read
// back and sent, and omitted when it says nothing. Source scans of app.js, as
// in the rest of this package.

// The certificate editor's save rebuilds acme from the form: every ACMESpec
// field must be written by it, or a save silently drops the field. Reflection
// keeps this honest when the model grows another one.
func TestCertEditorSendsEveryACMEField(t *testing.T) {
	js := loadAppJS(t)
	save := jsRegion(t, js, "$('#ct-save').addEventListener('click', async () => {", "const r = await api('/api/certificates/'")
	rt := reflect.TypeOf(model.ACMESpec{})
	for f := 0; f < rt.NumField(); f++ {
		tag := strings.Split(rt.Field(f).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if !strings.Contains(save, "a."+tag+" = ") && !strings.Contains(save, tag+": ") {
			t.Errorf("the certificate save never writes acme.%s; saving a loaded certificate drops it", tag)
		}
	}
}

// profile and caBundle: rendered from the loaded object, read on save, and
// omitted when blank.
func TestCertEditorSendsProfileAndCABundle(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "certificate editor render", js,
		`id="ct-profile" data-hint="certificate.acme.profile" data-path="acme.profile" value="${esc(acme.profile || '')}"`,
		`<textarea class="field mono" id="ct-cabundle" data-hint="certificate.acme.caBundle" data-path="acme.caBundle"`,
		// Set as a property: markup between the tags loses a leading newline.
		"$('#ct-cabundle').value = acme.caBundle || '';",
	)
	save := jsRegion(t, js, "$('#ct-save').addEventListener('click', async () => {", "const r = await api('/api/certificates/'")
	requireAll(t, "certificate save", save,
		"const prof = $('#ct-profile').value.trim();",
		"if (prof) {",
		"if (!ACME_PROFILE_RE.test(prof)) {",
		"a.profile = prof;",
		"const cab = caBundleValue($('#ct-cabundle').value, acme.caBundle);",
		"if (cab) {",
		"if (n > ACME_CA_BUNDLE_MAX_BYTES) {",
		"a.caBundle = cab;",
	)
	// Blank means omitted, never an empty string in the YAML.
	for _, bad := range []string{"a.profile = $(", "a.caBundle = $(", "profile: $(", "caBundle: $("} {
		if strings.Contains(save, bad) {
			t.Errorf("the certificate save writes %q unconditionally; a blank field must be omitted", bad)
		}
	}
	// caBundle is not a secret: no *** check, no masking.
	if strings.Contains(save, "cab === '***'") {
		t.Error("the certificate save treats acme.caBundle as a masked secret; it is returned as written")
	}
	fn := jsRegion(t, js, "function caBundleValue(typed, stored) {", "\n}")
	requireAll(t, "caBundleValue", fn,
		// An untouched textarea sends the stored text back byte for byte.
		"if (stored && typed === stored.replace(/\\r\\n?/g, '\\n')) return stored;",
		"return typed.trim() ? typed.trim() : '';",
	)
}

// The client-side checks mirror model.ACMESpec: acmeProfileRe and maxCABundle
// (64 KiB) are unexported, so the literal values are pinned here.
func TestCertEditorBoundsMirrorModel(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "acme bounds", js,
		"const ACME_PROFILE_RE = /^[A-Za-z0-9_-]{1,64}$/;",
		"const ACME_CA_BUNDLE_MAX_BYTES = 65536;",
		"new TextEncoder().encode(cab).length",
		"acme.profile: must be 1 to 64 characters of letters, digits, \"_\" or \"-\", got",
		"acme.caBundle: must be at most ${ACME_CA_BUNDLE_MAX_BYTES} bytes, got ${n}.",
	)
	ok := (&model.Certificate{ObjectMeta: model.ObjectMeta{Name: "c"}, Type: model.CertTypeACME, Domains: []string{"example.com"},
		ACME: &model.ACMESpec{Email: "a@example.com", Challenge: "http-01", Profile: strings.Repeat("a", 64)}}).Validate()
	if ok != nil {
		t.Fatalf("model refuses a 64-character profile the UI accepts: %v", ok)
	}
	bad := (&model.Certificate{ObjectMeta: model.ObjectMeta{Name: "c"}, Type: model.CertTypeACME, Domains: []string{"example.com"},
		ACME: &model.ACMESpec{Email: "a@example.com", Challenge: "http-01", Profile: strings.Repeat("a", 65)}}).Validate()
	if bad == nil {
		t.Fatal("model accepts a 65-character profile the UI refuses")
	}
	big := (&model.Certificate{ObjectMeta: model.ObjectMeta{Name: "c"}, Type: model.CertTypeACME, Domains: []string{"example.com"},
		ACME: &model.ACMESpec{Email: "a@example.com", Challenge: "http-01", CABundle: strings.Repeat("x", 65537)}}).Validate()
	if big == nil || !strings.Contains(big.Error(), "at most 65536 bytes") {
		t.Fatalf("model caBundle cap is no longer 65536 bytes (got %v); update ACME_CA_BUNDLE_MAX_BYTES", big)
	}
}

// renewAt / renewReason: shown on the editor's rail, in the list's expiry cell
// and in the state chip's tooltip; a past renewAt is "due", not a date.
func TestCertStatusShowsRenewAt(t *testing.T) {
	js := loadAppJS(t)
	fn := jsRegion(t, js, "function certRenewInfo(ct) {", "\n}")
	requireAll(t, "certRenewInfo", fn,
		"if (!ct || !ct.renewAt) return null;",
		"if (t <= Date.now()) {",
		"short: 'renewal due, not yet succeeded'",
		// changed is due now, whatever renewAt says
		"if (ct.renewReason === 'changed') {",
		"label: 'Due now: domains or profile changed'",
		// an unknown reason is shown raw
		"const why = CERT_RENEW_REASONS[ct.renewReason] || ct.renewReason || '';",
	)
	changed := strings.Index(fn, "if (ct.renewReason === 'changed') {")
	past := strings.Index(fn, "if (t <= Date.now()) {")
	if changed < 0 || past < 0 || changed > past {
		t.Error("certRenewInfo must handle renewReason \"changed\" before the past/future split")
	}
	requireAll(t, "renew reasons", js,
		"const CERT_RENEW_REASONS = { ari: 'time requested by the CA (ARI)', lifetime: 'lifetime rule' };",
	)
	card := jsRegion(t, js, "function certRailHtml(ct, isNew) {", "\n}")
	requireAll(t, "certificate rail", card,
		"const renew = acme ? certRenewInfo(ct) : null;",
		"(renewHtml ? row('Renews', renewHtml) : '')",
		"<span class=\"warn-text\">${esc(renew.label)}</span>",
		"(renew.why ? ` <span class=\"faint\" title=\"${esc(CERT_RENEW_RULE)}\">- ${esc(renew.why)}</span>` : '')",
	)
	cell := jsRegion(t, js, "function certExpiryCellHtml(ct) {", "\n}")
	requireAll(t, "expiry cell", cell, "const renew = certRenewInfo(ct);")
	chip := jsRegion(t, js, "function certStateChip(ct) {", "\n}")
	requireAll(t, "state chip", chip, "if (renew) titleBits.push(renew.text);")
	if strings.Contains(js, "30-day schedule") {
		t.Error("app.js still describes a fixed 30-day renewal schedule")
	}
}

// The webhook provider: offered, typed fields for url and token, the
// allowInsecureLocal switch, the request body described inline, and the
// switch read on save. Every model provider is offered.
func TestDNSProviderWebhook(t *testing.T) {
	js := loadAppJS(t)
	list := jsRegion(t, js, "const DNS_PROVIDERS = [", "\n];")
	for _, id := range model.KnownDNSProviders {
		if !strings.Contains(list, "id: '"+id+"'") {
			t.Errorf("DNS_PROVIDERS does not offer model provider %q", id)
		}
	}
	wh := jsRegion(t, list, "id: 'webhook',", "\n  },")
	requireAll(t, "webhook provider", wh,
		"required: ['url', 'token'],",
		"{ key: 'url', hint: 'dnsProvider.config.url',",
		"{ key: 'token', hint: 'dnsProvider.config.token',",
		"{ key: 'allowInsecureLocal', hint: 'dnsProvider.config.webhook.allowInsecureLocal', label: 'Allow a loopback URL', toggle: true,",
		`{"action":"present","fqdn":"_acme-challenge.example.com.","value":"&lt;TXT value&gt;"}`,
		"Authorization: Bearer &lt;token&gt;",
		"cleanup",
	)
	// acme-dns has the same switch, so a stored allowInsecureLocal is no longer
	// dropped by the typed form's rebuild.
	ad := jsRegion(t, list, "id: 'acme-dns',", "\n  },")
	requireAll(t, "acme-dns provider", ad, "{ key: 'allowInsecureLocal', hint: 'dnsProvider.config.allowInsecureLocal',")

	ed := jsRegion(t, js, "async function dnsEditor(c, name) {", "\n}")
	requireAll(t, "dns editor", ed,
		// rendered as a switch, on for a truthy, masked or placeholder value
		"${switchHtml('dnsf-' + f.key, dnsToggleOn(v), f.label, f.hint)}",
		// only a secret field goes through the *** refusal
		"const masked = $$('#ed-typed .dnsf[data-secret]').find((el) => el.value.trim() === '***');",
		// a URL setting may be literal or a placeholder
		"if (cfg.url && !SECRET_REF_RE.test(cfg.url) && !/^https?:\\/\\//.test(cfg.url)) {",
		"if (cfg.baseURL && !SECRET_REF_RE.test(cfg.baseURL) && !/^https?:\\/\\//.test(cfg.baseURL)) {",
		// a select never invents its default on an untouched save, and keeps
		// a stored value it does not list
		"if (el.dataset.dflt && v === el.dataset.dflt && !String((o.config || {})[el.dataset.key] || '').trim()) return;",
		"v && f.select.options.indexOf(v) === -1 ? f.select.options.concat(v) : f.select.options",
		"${f.secret ? ' data-secret=\"1\" autocomplete=\"off\"' : ''}",
	)
	if strings.Contains(ed, "Object.values(cfg).some((v) => v === '***')") {
		t.Error("the typed DNS form treats every key as a maskable secret; plain keys are returned as written")
	}
	if model.DNSProviderWebhook != "webhook" {
		t.Fatalf("model.DNSProviderWebhook is %q; the editor sends 'webhook'", model.DNSProviderWebhook)
	}
	// The values the switch can produce must be ones the model reads as on.
	if !model.IsTruthyConfig("true") {
		t.Fatal(`model.IsTruthyConfig("true") is false; the switch sends "true"`)
	}
}

// trustedProxies on the four discovery editors. Each save site is pinned in
// its own slice of saveIntegrations: the settings body also carries the
// settings-level trustedProxies key, so a search over the whole function would
// pass with any one of them missing.
func TestDiscoverySavesTrustedProxies(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "discovery render", js,
		`<div class="field-group" id="set-id-tp" style="margin-top:10px"><label>Trusted proxies</label>${tpTemplateHtml(idt.trustedProxies, 'settings.ingressDiscovery.template.trustedProxies')}</div>`,
		`<div class="field-group" id="set-dkr-tp" style="margin-top:10px"><label>Trusted proxies</label>${tpTemplateHtml(ddt.trustedProxies, 'settings.dockerDiscovery.template.trustedProxies')}</div>`,
		`<div class="field-group pf-tp" style="margin-top:10px"><label>Trusted proxies</label>${tpTemplateHtml(p.trustedProxies, 'settings.ingressDiscovery.template.trustedProxies')}</div>`,
		`<div class="field-group dpf-tp" style="margin-top:10px"><label>Trusted proxies</label>${tpTemplateHtml(p.trustedProxies, 'settings.dockerDiscovery.template.trustedProxies')}</div>`,
		// every control is loaded from the stored value: the template's, or the
		// row's own profile (p is row._orig; a new row passes {}).
		"const idTpCtl = wireTpTemplate($('#set-id-tp'), idt.trustedProxies, 'Ingress discovery template');",
		"const dkrTpCtl = wireTpTemplate($('#set-dkr-tp'), ddt.trustedProxies, 'Docker discovery template');",
		"div._tp = wireTpTemplate(div.querySelector('.pf-tp'), p.trustedProxies, 'Ingress discovery profile');",
		"div._tp = wireTpTemplate(div.querySelector('.dpf-tp'), p.trustedProxies, 'Docker discovery profile');",
		"$('#addIdProfile').addEventListener('click', () => profileRow('', {}));",
		"$('#addDkrProfile').addEventListener('click', () => dockerProfileRow('', {}));",
	)

	save := jsRegion(t, js, "async function saveIntegrations(btn) {", "const r = await api('/api/settings', { method: 'PUT', body });")
	sites := []struct{ name, start, end, read, send string }{
		{"ingress template", "const idTp = idTpCtl.get();", "const profiles = Object.create(null);",
			"if (idTp === null) return;", "trustedProxies: idTp,"},
		{"ingress profiles", "$$('#set-id-profiles .id-profile').forEach((row) => {", "if (Object.keys(profiles).length)",
			"const tp = row._tp.get();\n      if (tp === null) { pfTpBad = true; return; }", "trustedProxies: tp,"},
		{"docker template", "const dkrTp = dkrTpCtl.get();", "const dkrProfiles = Object.create(null);",
			"if (dkrTp === null) return;", "trustedProxies: dkrTp,"},
		{"docker profiles", "$$('#set-dkr-profiles .dkr-profile').forEach((row) => {", "if (Object.keys(dkrProfiles).length)",
			"const tp = row._tp.get();\n      if (tp === null) { dpfTpBad = true; return; }", "trustedProxies: tp,"},
	}
	for _, s := range sites {
		region := jsRegion(t, save, s.start, s.end)
		requireAll(t, s.name+" save", region, s.read, s.send)
		// The value goes out as the control returned it: undefined (dropped by
		// JSON.stringify, so unset is omitted), [] or the list. Any coercion
		// would fold [] into unset or the other way round.
		for _, line := range strings.Split(region, "\n") {
			if strings.Contains(line, "trustedProxies:") && (strings.Contains(line, "||") || strings.Contains(line, "length") || strings.Contains(line, "arr(")) {
				t.Errorf("%s save coerces trustedProxies: %s", s.name, strings.TrimSpace(line))
			}
		}
	}
	// An invalid list in a profile row stops the whole save.
	requireAll(t, "profile validation", save, "if (pfTpBad) return;", "if (dpfTpBad) return;")

	// The three states, both ways.
	mode := jsRegion(t, js, "function tpTemplateMode(v) {", "\n}")
	requireAll(t, "tpTemplateMode", mode,
		"if (v === undefined) return 'unset';",
		"return arr(v).length ? 'custom' : 'none';",
	)
	get := jsRegion(t, js, "function wireTpTemplate(wrap, v, where) {", "\n}")
	requireAll(t, "wireTpTemplate", get,
		"if (m === 'unset') return undefined;",
		"if (m === 'none') return [];",
		"const bad = firstBadCidr(list);",
		"return list;",
	)
	if !strings.Contains(js, "init.body = JSON.stringify(opts.body);") {
		t.Error("api() no longer serialises with JSON.stringify; an unset trustedProxies (undefined) relies on it being dropped")
	}
	// The model field exists with this key on the template type.
	f, ok := reflect.TypeOf(model.IngressHostTemplate{}).FieldByName("TrustedProxies")
	if !ok || strings.Split(f.Tag.Get("json"), ",")[0] != "trustedProxies" || f.Type.Kind() != reflect.Pointer {
		t.Fatal("model.IngressHostTemplate.TrustedProxies is no longer a *[]string under json key trustedProxies")
	}
}

// Bulk Add tag / Remove tag apply to discovery-managed hosts; a remove that
// touches one says a template tag comes back.
func TestBulkTagsIncludeManagedHosts(t *testing.T) {
	js := loadAppJS(t)
	bulk := jsRegion(t, js, "async function runBulk(action) {", "wireFilter('hostFilter'")
	for _, gone := range []string{"targets.filter((h) => !managedBy(h))", "skipped"} {
		if strings.Contains(bulk, gone) {
			t.Errorf("runBulk still skips discovery-managed hosts (%q found)", gone)
		}
	}
	requireAll(t, "runBulk", bulk,
		"const managedTargets = targets.filter((h) => managedBy(h));",
		"(action === 'tag-remove' && managedTargets.length)",
		"is stamped again at the next reconcile",
		"${listHtml}${managedHtml}",
	)
	if strings.Contains(js, "tags come from the discovery template") {
		t.Error("app.js still says a managed host's tags come from the discovery template")
	}
	chips := jsRegion(t, js, "const MANAGED_CHIPS = {", "\n};")
	if strings.Contains(chips, "Edits other than disable and maintenance are reverted") {
		t.Error("the managed-host note still says every edit but disable and maintenance is reverted")
	}
	requireAll(t, "managed-host note", chips, "tags added by hand",
		"trusted proxies set by hand while the discovery template leaves them unset",
		"a trusted-proxy value the template stamped follows the template")
}

// The new controls have hint entries (several ids are passed through a
// variable, which the generic hint tests cannot see), and the hints the new
// behaviour made wrong are corrected.
func TestBatchHintsPresent(t *testing.T) {
	reg := loadHintRegistry(t)
	js := loadAppJS(t)
	for _, id := range []string{
		"certificate.acme.profile", "certificate.acme.caBundle",
		"dnsProvider.config.url", "dnsProvider.config.token",
		"dnsProvider.config.allowInsecureLocal", "dnsProvider.config.webhook.allowInsecureLocal",
		"settings.ingressDiscovery.template.trustedProxies", "settings.dockerDiscovery.template.trustedProxies",
	} {
		if _, ok := reg[id]; !ok {
			t.Errorf("hints.json has no entry %q", id)
		}
		if !strings.Contains(js, "'"+id+"'") && !strings.Contains(js, `"`+id+`"`) {
			t.Errorf("app.js never references hint %q", id)
		}
	}
	for _, id := range []string{"settings.ingressDiscovery.template.tags", "settings.dockerDiscovery.template.tags"} {
		if !strings.Contains(reg[id].Text, "keeps the tags added to a host by hand") {
			t.Errorf("%s still describes tags as replaced on every reconcile", id)
		}
	}
	if !strings.Contains(reg["certificate.acme.caBundle"].Text, "replaces the system roots") {
		t.Error("certificate.acme.caBundle hint no longer says the bundle replaces the system roots")
	}
	if !strings.Contains(reg["dnsProvider.provider"].Text, "webhook") {
		t.Error("dnsProvider.provider hint does not mention the webhook provider")
	}
}

// Each typed DNS provider form marks exactly the keys model.DNSProviderPlainKey
// calls secrets with `secret: true` (placeholder help, *** refusal), and leaves
// every plain key unmarked (stored and shown as written). The token providers
// have no typed form: their one key, apiToken, and any extra key typed into the
// key/value rows are secrets, and the rows refuse a *** value for all of them.
func TestDNSProviderFormsMatchPlainKeyRule(t *testing.T) {
	js := loadAppJS(t)
	list := jsRegion(t, js, "const DNS_PROVIDERS = [", "\n];")
	fieldRe := regexp.MustCompile(`\{ key: '([A-Za-z]+)',[^\n]*`)
	for _, id := range model.KnownDNSProviders {
		start := strings.Index(list, "id: '"+id+"'")
		if start < 0 {
			t.Errorf("DNS_PROVIDERS has no %q", id)
			continue
		}
		block := list[start:]
		if next := strings.Index(block[1:], "id: '"); next >= 0 {
			block = block[:next+1]
		}
		if !strings.Contains(block, "fields: [") {
			if !strings.Contains(block, "required: ['apiToken']") {
				t.Errorf("%s: no typed form and not a single-apiToken provider", id)
			}
			if model.DNSProviderPlainKey(id, "apiToken") {
				t.Errorf("%s: apiToken is plain in the model, but the key/value rows treat it as a secret", id)
			}
			continue
		}
		fields := fieldRe.FindAllStringSubmatch(block, -1)
		if len(fields) == 0 {
			t.Errorf("%s: typed form has no fields", id)
		}
		for _, m := range fields {
			key, secret := m[1], strings.Contains(m[0], "secret: true")
			if plain := model.DNSProviderPlainKey(id, key); plain == secret {
				t.Errorf("%s.%s: model plain=%v, form secret=%v; the form must treat exactly the secret keys as secrets", id, key, plain, secret)
			}
		}
	}
	requireAll(t, "key/value rows", js,
		"if (cfgCtl.masked()) { showInvalid('Secret masked',",
	)
}

// An untouched save sends allowInsecureLocal back exactly as stored ("yes",
// "on", "false", a placeholder): the switch only writes "true", or drops the
// key, when the operator left it in a state the stored value does not mean.
func TestDNSSwitchKeepsStoredValue(t *testing.T) {
	js := loadAppJS(t)
	ed := jsRegion(t, js, "async function dnsEditor(c, name) {", "\n}")
	requireAll(t, "dns switch save", ed,
		"const on = isOn('dnsf-' + f.key);",
		"const was = provId === current ? storedCfg[f.key] : undefined;",
		"if (was != null && String(was).trim() !== '' && on === dnsToggleOn(was)) cfg[f.key] = was;",
		"else if (on) cfg[f.key] = 'true';",
		// rendered from the same rule the save compares with
		"${switchHtml('dnsf-' + f.key, dnsToggleOn(v), f.label, f.hint)}",
	)
	on := jsRegion(t, js, "function dnsToggleOn(v) {", "\n}")
	requireAll(t, "dnsToggleOn", on, "return SECRET_REF_RE.test(s) || /^(true|yes|1|on)$/i.test(s);")
	// The switch's notion of "on" is the model's.
	for _, v := range []string{"true", "yes", "1", "on", "TRUE", "On"} {
		if !model.IsTruthyConfig(v) {
			t.Errorf("model.IsTruthyConfig(%q) is false; dnsToggleOn shows it as on", v)
		}
	}
	for _, v := range []string{"false", "no", "0", "off", ""} {
		if model.IsTruthyConfig(v) {
			t.Errorf("model.IsTruthyConfig(%q) is true; dnsToggleOn shows it as off", v)
		}
	}
}

// A typed form (rfc2136, acme-dns, webhook) carries forward, unchanged, every
// key of the stored config it does not render, when the type is the stored one.
func TestDNSTypedFormCarriesUnrenderedKeys(t *testing.T) {
	js := loadAppJS(t)
	ed := jsRegion(t, js, "async function dnsEditor(c, name) {", "\n}")
	requireAll(t, "dns typed save", ed,
		"const storedCfg = o.config || {};",
		"if (provId === current) {",
		"const rendered = p.fields.map((f) => f.key);",
		"const extra = Object.keys(storedCfg).filter((k) => rendered.indexOf(k) === -1);",
		"extra.forEach((k) => { cfg[k] = storedCfg[k]; });",
		// a masked one cannot be sent back, and is not silently dropped
		"const maskedExtra = extra.find((k) => storedCfg[k] === '***');",
	)
	// the carry-forward sits in the typed branch, before the required check
	typed := strings.Index(ed, "if (p.fields) {\n      cfg = {};")
	carry := strings.Index(ed, "extra.forEach((k) => { cfg[k] = storedCfg[k]; });")
	kv := strings.Index(ed, "cfg = cfgCtl.get();")
	if typed < 0 || carry < 0 || kv < 0 || !(typed < carry && carry < kv) {
		t.Error("the unrendered-key carry-forward is not in the typed-form branch of the DNS provider save")
	}
}

// Changing the provider type starts the new type from its own keys only: the
// typed values and the key/value rows are rebuilt empty (an empty apiToken row
// for a token provider), or from the stored config when the change goes back
// to the stored type.
func TestDNSTypeChangeDropsOldKeys(t *testing.T) {
	js := loadAppJS(t)
	ed := jsRegion(t, js, "async function dnsEditor(c, name) {", "\n}")
	requireAll(t, "dns type change", ed,
		"$('#ed-provider').addEventListener('change', () => syncProvider(true));",
		"const back = p.id === current;",
		"typedValues = back ? Object.assign({}, storedCfg) : {};",
		"$('#ed-config').innerHTML = '';",
		"cfgCtl = makeKVRows($('#ed-config'), back ? storedCfg : (p.fields ? {} : { apiToken: '' }), kvKey, kvVal, true);",
		"let cfgCtl = makeKVRows(",
	)
	// no reseed path may keep the old type's values by key
	if strings.Contains(ed, "if (typedValues[f.key] != null) keep[f.key] = typedValues[f.key];") {
		t.Error("a provider change still keeps the previous type's values for matching keys")
	}
}
