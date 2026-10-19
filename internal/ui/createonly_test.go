package ui

import (
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
