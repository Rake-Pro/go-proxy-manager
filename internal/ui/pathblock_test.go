package ui

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/dataplane"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// The path block list lives in five editors: the path-block middleware, the
// fleet list on the Settings page, the proxy host's switch, and the switch on
// both discovery templates and every profile row. Every one of those saves is a
// whole-object replacement, so each must both render the field and send it
// back. Like the rest of this package these are source scans of app.js: there
// is no JS runtime in the test toolchain.

// jsRegion returns app.js from start up to the first end after it.
func jsRegion(t *testing.T, js, start, end string) string {
	t.Helper()
	i := strings.Index(js, start)
	if i < 0 {
		t.Fatalf("app.js: %q not found", start)
	}
	rel := strings.Index(js[i:], end)
	if rel < 0 {
		t.Fatalf("app.js: %q not found after %q", end, start)
	}
	return js[i : i+rel]
}

func requireAll(t *testing.T, where, region string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(region, want) {
			t.Errorf("%s: missing %q", where, want)
		}
	}
}

// The shared block's get() must emit every field of model.PathBlockMiddleware.
// Reflection keeps this honest when the model grows a field: the block then
// silently drops it on every middleware and Settings save until it is wired.
func TestPathBlockBlockSendsEveryField(t *testing.T) {
	js := loadAppJS(t)
	region := jsRegion(t, js, "function wirePathBlockBlock(p, pb) {", "\nfunction pathBlockSwitchHtml(")

	rt := reflect.TypeOf(model.PathBlockMiddleware{})
	for f := 0; f < rt.NumField(); f++ {
		tag := strings.Split(rt.Field(f).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if !strings.Contains(region, "spec."+tag+" = ") {
			t.Errorf("wirePathBlockBlock never sets spec.%s; a save drops model.PathBlockMiddleware field %q", tag, tag)
		}
		if !strings.Contains(js, `data-path="pathBlock.`+tag+`"`) {
			t.Errorf("pathBlockBlockHtml renders no control for %q", tag)
		}
	}
	// No list entry at all is "no list" to the caller, never an empty spec: the
	// API refuses a path block without an entry.
	requireAll(t, "wirePathBlockBlock", region,
		"if (!extensions.length && !names.length && !globs.length) return undefined;",
		// error-page is the default and is only written when the stored object
		// spelled it out.
		"if (dw !== 'error-page' || pb.denyWith) spec.denyWith = dw;",
		// allowFrom takes the same CIDR / IP / @list entries as a rate limit's.
		"v.startsWith('@') ? ACCESS_LIST_REF_RE.test(v) : isCidrOrIP(v)",
	)
}

// The client-side bounds mirror the model's, so the UI refuses exactly what the
// API would.
func TestPathBlockBoundsMirrorModel(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "path block bounds", js,
		fmt.Sprintf("const PATH_BLOCK_MAX_ENTRIES = %d;", model.MaxPathBlockEntries),
		fmt.Sprintf("const PATH_BLOCK_MAX_GLOBS = %d;", model.MaxPathBlockGlobs),
		fmt.Sprintf("const PATH_BLOCK_MAX_LEN = %d;", model.MaxPathBlockEntryLen),
		fmt.Sprintf("const PATH_BLOCK_MAX_GLOB_BYTES = %d;", model.MaxPathBlockGlobBytes),
		fmt.Sprintf(`placeholder="%d"`, model.DefaultPathBlockDenyStatus),
	)
}

// The middleware editor offers the type, renders the block, and saves the
// {type: 'path-block', pathBlock: {...}} shape, refusing a spec with no entry.
func TestMiddlewareEditorSavesPathBlock(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "middleware editor", js,
		"'path-block': 'Path block (deny by path shape)',",
		"${enumOptions('middlewareType', ['auth', 'headers', 'guard', 'rate-limit', 'rewrite', 'bouncer', 'path-block'], type)}",
		`data-type="path-block"`,
		"const pb = o.pathBlock || {};",
		"${pathBlockBlockHtml('mwpb', pb, 'middleware')}",
		"const pbCtl = wirePathBlockBlock('mwpb', pb);",
	)
	region := jsRegion(t, js, "} else if (t === 'path-block') {", "} else if (t === 'rewrite') {")
	requireAll(t, "middleware path-block save", region,
		"const spec = pbCtl.get();",
		"if (spec === null) return null;",
		"if (!spec) { showInvalid('Entry required',",
		"body.pathBlock = spec;",
	)
	// The body starts as { type: t }, so the type travels with the spec.
	if !strings.Contains(js, "const t = $('#ed-type').value; const body = { type: t };") {
		t.Error("the middleware save no longer seeds the body with its type")
	}
	if string(model.MWTypePathBlock) != "path-block" {
		t.Fatalf("model.MWTypePathBlock is %q; the editor sends 'path-block'", model.MWTypePathBlock)
	}
}

// Settings: the fleet list has a real control, its value reaches the PUT body,
// and an empty section sends no pathBlock key at all.
func TestSettingsSavePathBlock(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "settings page", js,
		"${pathBlockBlockHtml('set-pb', s.pathBlock, 'settings')}",
		"const pathBlockCtl = wirePathBlockBlock('set-pb', s.pathBlock);",
	)
	region := jsRegion(t, js, "$('#set-save').addEventListener", "const r = await api('/api/settings', { method: 'PUT', body });")
	requireAll(t, "settings save", region,
		"const pathBlock = pathBlockCtl.get();",
		"if (pathBlock === null) return;",
		"if (pathBlock) body.pathBlock = pathBlock;",
	)
	if strings.Contains(region, "body.pathBlock = {}") || strings.Contains(region, "pathBlock || {}") {
		t.Error("the settings save can send an empty pathBlock object, which the API refuses")
	}
}

// The proxy host: a select for the switch, sent only when it says something
// ("off", or an "inherit" the stored host already spelled out).
func TestHostSavePathBlockSwitch(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "host editor", js,
		`pathBlockSwitchHtml('id="f-pathblock" data-path="pathBlock"', h.pathBlock, 'proxyHost.pathBlock')`,
		"const pathBlock = pathBlockSwitchValue($('#f-pathblock'), h.pathBlock);",
		"if (pathBlock) obj.pathBlock = pathBlock;",
		"'f-pathblock-card': () => pbSwitchSummary($('#f-pathblock').value),",
	)
	sw := jsRegion(t, js, "function pathBlockSwitchValue(el, stored) {", "\n}")
	requireAll(t, "pathBlockSwitchValue", sw,
		"if (!el) return stored || undefined;",
		"if (el.value === 'off') return 'off';",
		"return stored === 'inherit' ? 'inherit' : undefined;",
	)
	if model.PathBlockOff != "off" || model.PathBlockInherit != "inherit" {
		t.Fatalf("model path block switch values changed (%q, %q); the UI sends off/inherit", model.PathBlockOff, model.PathBlockInherit)
	}
}

// Both discovery templates and every profile row of both render the switch
// and send it back from the Integrations save; a profile row the operator never
// touched still carries its loaded value through its select.
func TestDiscoverySavesPathBlock(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "discovery render", js,
		`pathBlockSwitchHtml('id="set-id-pathblock" data-path="ingressDiscovery.template.pathBlock"', idt.pathBlock, 'settings.ingressDiscovery.template.pathBlock')`,
		`pathBlockSwitchHtml('id="set-dkr-pathblock" data-path="dockerDiscovery.template.pathBlock"', ddt.pathBlock, 'settings.dockerDiscovery.template.pathBlock')`,
		`pathBlockSwitchHtml('aria-label="Fleet path block list"', p.pathBlock, 'settings.ingressDiscovery.template.pathBlock', 'pf-pathblock')`,
		`pathBlockSwitchHtml('aria-label="Fleet path block list"', p.pathBlock, 'settings.dockerDiscovery.template.pathBlock', 'dpf-pathblock')`,
		// The row class goes into the select's one class attribute.
		"return `<select class=\"field mono${cls ? ' ' + cls : ''}\" ${attrs} data-hint=",
	)
	// A class passed inside attrs would be a second class attribute, which the
	// browser drops: the save would then never find the row's select and would
	// send the stored value back whatever the operator chose.
	if regexp.MustCompile(`pathBlockSwitchHtml\('[^']*class=`).MatchString(js) {
		t.Error("a pathBlockSwitchHtml call passes class= in attrs; pass the class as the fourth argument")
	}
	region := jsRegion(t, js, "async function saveIntegrations(btn) {", "const r = await api('/api/settings', { method: 'PUT', body });")
	requireAll(t, "integrations save", region,
		"pathBlock: pathBlockSwitchValue($('#set-id-pathblock'), idt.pathBlock),",
		"pathBlock: pathBlockSwitchValue(row.querySelector('.pf-pathblock'), (row._orig || {}).pathBlock),",
		"pathBlock: pathBlockSwitchValue($('#set-dkr-pathblock'), ddt.pathBlock),",
		"pathBlock: pathBlockSwitchValue(row.querySelector('.dpf-pathblock'), (row._orig || {}).pathBlock),",
	)
	// Each profile row keeps the loaded profile, which is what the save reads
	// the stored value from.
	if strings.Count(js, "div._orig = p;") < 2 {
		t.Error("a discovery profile row no longer keeps its loaded profile; the pathBlock carry-forward reads from it")
	}
}

// path-block shows up wherever the UI enumerates middleware types: the rail's
// request chain, in its data-plane position right after the rate limit.
func TestPathBlockInRequestChain(t *testing.T) {
	js := loadAppJS(t)
	requireAll(t, "request chain", js,
		"const CHAIN_TYPE_ORDER = ['rate-limit', 'path-block', 'bouncer', 'auth', 'guard', 'headers', 'rewrite'];",
		"'path-block': 'path block'",
	)
	chain := jsRegion(t, js, "function renderRailChain(el, ctx) {", "\n}")
	rl := strings.Index(chain, "items.push(...byType('rate-limit'));")
	fleet := strings.Index(chain, "if (ctx.fleetPathBlock)")
	pb := strings.Index(chain, "items.push(...byType('path-block'));")
	al := strings.Index(chain, "if (ctx.alMode === 'any'")
	if rl < 0 || fleet < 0 || pb < 0 || al < 0 || !(rl < fleet && fleet < pb && pb < al) {
		t.Error("the rail chain no longer lists rate limit, fleet path block list, path-block middlewares, access lists in that order")
	}
}

// The access log carries the tier that refused a request (AccessEntry.Deny).
// The Access Logs page shows it on the row, matches it in the free-text
// filter, offers it as the `deny:` filter key (so `deny:path-block` shows only
// path-block denials and `-deny:path-block` hides them) and exports it.
func TestAccessLogsSurfaceDenyReason(t *testing.T) {
	js := loadAppJS(t)

	f, ok := reflect.TypeOf(dataplane.AccessEntry{}).FieldByName("Deny")
	if !ok || strings.Split(f.Tag.Get("json"), ",")[0] != "deny" {
		t.Fatal("dataplane.AccessEntry no longer has a Deny field with json key \"deny\"; the Access Logs page reads e.deny")
	}

	fields := jsRegion(t, js, "const LOG_FIELDS = {", "\n};")
	if !strings.Contains(fields, "\n  deny: '") {
		t.Error("LOG_FIELDS does not offer the deny filter key")
	}
	rec := jsRegion(t, js, "function logRec(e, nets) {", "\n}")
	requireAll(t, "logRec", rec,
		"e.country, e.deny].join(' ')",
		"deny: fvals(e.deny || ''),",
	)
	row := jsRegion(t, js, "function logRowHtml(e, i) {", "\n}")
	requireAll(t, "logRowHtml", row,
		"${e.deny ? ` <span class=\"chip kind cyan\" title=\"${esc('Refused by ' + e.deny)}\">${esc(e.deny)}</span>` : ''}",
	)
	exp := jsRegion(t, js, "function exportLogs(fmt) {", "\n  }")
	requireAll(t, "CSV export", exp, ".concat(['source', 'deny']);")
}

// The client-side glob check mirrors model.globClassHasUpper, including the
// backslash quoting, and is wired into the globs list check. A trailing "/"
// is legal (the matcher ignores it), so nothing refuses it.
func TestGlobClassCaseCheckMirrorsModel(t *testing.T) {
	js := loadAppJS(t)
	fn := jsRegion(t, js, "function globClassHasUpper(g) {", "\n}")
	requireAll(t, "globClassHasUpper", fn,
		"if (c === '\\\\' && i + 1 < cs.length) {",
		"if (inClass && q.toLowerCase() !== q) return true;",
		"} else if (!inClass && c === '[') inClass = true;",
		"else if (inClass && c === ']') inClass = false;",
		"else if (inClass && c.toLowerCase() !== c) return true;",
	)
	block := jsRegion(t, js, "function wirePathBlockBlock(p, pb) {", "\nfunction pathBlockSwitchHtml(")
	requireAll(t, "globs check", block,
		"(globClassHasUpper(e) ? 'has an upper-case letter inside [...]: matching is case-insensitive, write the class in lower case' : globShapeError(e))",
	)
	// The shape rules (trailing slash, empty segment, segment ending in a dot or
	// a space, control character) and the total size are checked like the model.
	requireAll(t, "globs check", block,
		"globShapeError(e)",
		"> PATH_BLOCK_MAX_GLOB_BYTES) {",
	)
	shape := jsRegion(t, js, "function globShapeError(g) {", "\n}")
	requireAll(t, "globShapeError", shape,
		"if (g === '/') return '';",
		"return 'must not contain a control character';",
		"if (t !== g && t !== '') return `must not end with \"/\"",
		"if (seg === '') return 'must not contain an empty segment",
		"if (seg.endsWith('.') || seg.endsWith(' ')) return",
	)

}
