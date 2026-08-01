package ui

import (
	"strings"
	"testing"
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
		// each named profile row
		"switchHtml('pf-robots-' + i",
		"robotsNoIndex: isOn('pf-robots-' + uid)",
		"div.querySelector('.pf-tags')",
		"tags: row._tags.get()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("app.js no longer round-trips a discovery template field (%q not found); a settings save now clears it", want)
		}
	}
}
