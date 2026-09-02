package ui

import (
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/clientcert"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestStaticBundleClientCAEditorRoundTripsEveryField guards the regression class
// this editor is exposed to: the API PUT is a whole-object replace, so any field
// the editor renders but forgets to send back is silently reset to its default on
// the next save from the UI. A JS test toolchain does not exist here, so this is a
// text assertion like the other static-bundle tests in this package.
func TestStaticBundleClientCAEditorRoundTripsEveryField(t *testing.T) {
	js := readStatic(t, "static/app.js")

	// Every persisted ClientCA field the editor exposes must appear on the body
	// it PUTs, not just on the form it renders.
	for _, want := range []string{
		"body.crlFile = file",
		"body.crlPEM = inline",
		"body.crlPolicy = policy",
		"body.caKeyFile = keyFile",
		"body.caKeyPEM = keyPEM",
		"body.expiryWarningDays = warnDays",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("clientCA editor does not send %q back on save - the API PUT replaces the whole object, so the stored value would be reset", want)
		}
	}
	// ... and the control has to exist for the operator to set it at all.
	if !strings.Contains(js, `id="ed-warndays"`) {
		t.Error("clientCA editor has no expiryWarningDays control")
	}
}

// TestStaticBundleClientCertPasswordFloor checks the UI states and pre-checks the
// same password floor the server enforces, so an operator is told before a round
// trip rather than after one.
func TestStaticBundleClientCertPasswordFloor(t *testing.T) {
	js := readStatic(t, "static/app.js")

	if !strings.Contains(js, "const P12_MIN_PASSWORD = 12;") {
		t.Fatal("app.js P12_MIN_PASSWORD is missing")
	}
	if clientcert.MinPasswordLen != 12 {
		t.Fatalf("clientcert.MinPasswordLen is %d but app.js hard-codes 12 - keep them in step", clientcert.MinPasswordLen)
	}
	// Both the issue form and the renew form must pre-check it.
	if n := strings.Count(js, "pw.length < P12_MIN_PASSWORD"); n != 2 {
		t.Fatalf("expected the password floor to be checked in both the issue and renew flows, found %d checks", n)
	}
	if n := strings.Count(js, `minlength="12"`); n != 2 {
		t.Fatalf("expected both password inputs to carry minlength=12, found %d", n)
	}
}

// TestStaticBundleSupersededRowsAreHistorical checks a superseded issuance record
// offers no renew action: renewing one is refused with 409 by the API, so the UI
// must not present a control whose only outcome is that error.
func TestStaticBundleSupersededRowsAreHistorical(t *testing.T) {
	js := readStatic(t, "static/app.js")

	for _, want := range []string{
		// the row is marked, styled as history, and shows the successor instead
		`r.supersededBy ? ' class="superseded"' : ''`,
		"renewed as ",
		// the renew button and its form only exist for a current record
		"${r.supersededBy\n        ? ",
		"${r.supersededBy ? '' : `<tr class=\"ren-row\"",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("issued-certificate table missing superseded-row marker %q", want)
		}
	}
	css := readStatic(t, "static/app.css")
	if !strings.Contains(css, ".mini-table tr.superseded td") {
		t.Error("app.css has no superseded-row style")
	}
}

// TestStaticBundleExpiryWarningDefault checks the UI copy matches the Go
// default, so the help text does not tell operators the wrong number. The
// sentence lives in the in-app help registry now, not in an inline hint.
func TestStaticBundleExpiryWarningDefault(t *testing.T) {
	js := readStatic(t, "static/app.js")
	if model.DefaultExpiryWarningDays != 30 {
		t.Fatalf("model.DefaultExpiryWarningDays is %d but the UI says 30 - keep them in step", model.DefaultExpiryWarningDays)
	}
	if !strings.Contains(js, `placeholder="30"`) {
		t.Error("the expiryWarningDays input no longer shows the default as its placeholder")
	}
	hint, ok := loadHintRegistry(t)["clientCA.expiryWarningDays"]
	if !ok {
		t.Fatal("hints.json has no clientCA.expiryWarningDays entry")
	}
	if !strings.Contains(hint.Text, "30 days") {
		t.Errorf("clientCA.expiryWarningDays help text does not state the 30-day default: %q", hint.Text)
	}
}

func readStatic(t *testing.T, path string) string {
	t.Helper()
	b, err := staticFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestStaticBundleClientCAGenerateControls checks the new-CA page offers
// generation as a first-class alternative to pasting an external CA, and posts it
// at the right route. Without this the feature's whole point - a working CA with
// no external tooling - is one typo away from being unreachable.
func TestStaticBundleClientCAGenerateControls(t *testing.T) {
	js := readStatic(t, "static/app.js")

	for _, want := range []string{
		// the two alternatives are one either/or choice, generate first
		`{ v: 'generate', label: 'Generate new CA', panel: generatePanel }`,
		`{ v: 'paste', label: 'Paste existing CA', panel: pastePanel }`,
		// the fields and the action
		`id="gen-cn"`,
		`id="gen-days"`,
		`id="gen-org"`,
		`id="gen-btn"`,
		// posted at the documented route, and landing on the created object
		`'/api/client-cas/' + encodeURIComponent(nm) + '/generate'`,
		`location.hash = '#/clientcas/' + encodeURIComponent(nm)`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("new-CA page missing generate marker %q", want)
		}
	}
	// The generate button is a .btn.primary, which is what the HA-follower gating
	// selector greys out - so a follower cannot start a config write.
	if !strings.Contains(js, `<button class="btn primary" id="gen-btn"`) {
		t.Error("gen-btn must be a .btn.primary so applyReadOnlyGating gates it on a follower")
	}
	if !strings.Contains(js, "'.btn.primary'") {
		t.Error("RO_WRITE_CONTROLS no longer gates .btn.primary - the generate button would be live on a follower")
	}
	// The CA validity bound mirrors the Go cap.
	if !strings.Contains(js, `max="7300"`) {
		t.Error("generate validity input does not carry the 7300-day cap")
	}
}

// TestStaticBundleClientCAScreenLayout guards the presentation cleanup: the
// optional sections collapse by default and open only when they hold something,
// and each either/or pair renders ONE control rather than two stacked ones.
func TestStaticBundleClientCAScreenLayout(t *testing.T) {
	js := readStatic(t, "static/app.js")

	// Revocation and the signing key are folds, summarised when closed and opened
	// automatically when configured (hasCRL / hasKey are the open flags).
	for _, want := range []string{
		`foldHtml('crl-card', 'Revocation (CRL)', crlSummary, hasCRL,`,
		`foldHtml('cakey-card', 'Signing key', keySummary, hasKey,`,
		"not configured - a revoked certificate passes until it expires",
		"not configured - issuance disabled",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("clientCA screen missing collapsed-section marker %q", want)
		}
	}
	// All three either/or pairs go through the one segmented widget.
	for _, group := range []string{"'anchor'", "'crl'", "'cakey'"} {
		if !strings.Contains(js, "segHtml("+group+",") {
			t.Errorf("either/or pair %s is not rendered as a segmented choice", group)
		}
	}
	// Only the selected side of a pair is read on save, so the two can never both
	// be sent - the mutual exclusion is structural, not a validation check.
	for _, want := range []string{
		`segValue('crl') === 'inline'`,
		`segValue('cakey') === 'inline'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("save path does not read the selected source only: missing %q", want)
		}
	}
	// The old free-floating "no CRL configured" paragraph competed with the real
	// expiry banner; it is now the collapsed summary line.
	if strings.Contains(js, "No CRL configured:") {
		t.Error("the free-floating no-CRL warning paragraph is back - it belongs in the fold summary")
	}
	// Every section on the page uses the app-wide caps section-label treatment.
	if !strings.Contains(js, `<summary><p class="section-label">`) {
		t.Error("fold sections must use the same section-label header treatment as the plain cards")
	}
}

// TestStaticBundleEitherOrSaveIsNonDestructive is the guard for the worst way a
// segmented picker can go wrong: reading only the selected control means toggling
// the picker to LOOK at the other option and saving wipes whatever was stored
// there. For the CRL pair that silently switches revocation off; for the CA key
// pair it silently switches issuance off. The save must be a byte-for-byte no-op
// when nothing was edited.
func TestStaticBundleEitherOrSaveIsNonDestructive(t *testing.T) {
	js := readStatic(t, "static/app.js")

	// The rule is one named function, so both pairs cannot drift apart.
	if !strings.Contains(js, "function resolvePair(typed, storedOther)") {
		t.Fatal("resolvePair is missing - the either/or save rule must be one shared function")
	}
	if !strings.Contains(js, "return typed ? [typed, ''] : ['', storedOther];") {
		t.Error("resolvePair no longer preserves the stored other side when the selected control is empty")
	}
	// Both pairs, both directions, must pass the STORED value of the unselected
	// side as the fallback. The DESTRUCTURING TARGETS are pinned too: resolvePair
	// returns [selected, other], so transposing them would assign the typed value
	// to the wrong field - a bug that matching on the call alone would wave
	// through. Missing or transposing any of these four is the wipe bug.
	for _, want := range []string{
		"[inline, file] = resolvePair($('#ed-crlpem').value.trim(), o.crlFile || '');",
		"[file, inline] = resolvePair($('#ed-crlfile').value.trim(), o.crlPEM || '');",
		"[keyPEM, keyFile] = resolvePair($('#ed-cakeypem').value.trim(), o.caKeyFile || '');",
		"[keyFile, keyPEM] = resolvePair($('#ed-cakeyfile').value.trim(), o.caKeyPEM || '');",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("either/or save drops or transposes the unselected side: missing %q", want)
		}
	}
	// crlPolicy rides on whichever side survived, so a toggle cannot orphan it.
	if !strings.Contains(js, "if (policy && (file || inline)) body.crlPolicy = policy;") {
		t.Error("crlPolicy is no longer preserved whenever a CRL source survives")
	}
	// The redaction guard must run on the RESOLVED key, not on the input, or it
	// stops firing as soon as the picker is on the file side.
	i := strings.Index(js, "[keyFile, keyPEM] = resolvePair")
	j := strings.Index(js, "if (keyPEM === '***')")
	if i < 0 || j < 0 || j < i {
		t.Error("the '***' redaction guard must be checked after the key pair is resolved")
	}
}

// TestStaticBundleGenerateHidesUnusedSections guards the two new-CA page traps:
// a clone seed must land on the paste side (its PEM is already filled in), and
// the sections POST /generate does not accept must not be on screen while
// generating, where anything typed into them would be silently discarded.
func TestStaticBundleGenerateHidesUnusedSections(t *testing.T) {
	js := readStatic(t, "static/app.js")

	if !strings.Contains(js, "], seed ? 'paste' : 'generate') : pastePanel}") {
		t.Error("cloning a ClientCA must default the trust-anchor picker to paste - the seed already carries a certificate")
	}
	for _, want := range []string{
		"const generating = v === 'generate';",
		"[$('#crl-card'), $('#cakey-card')].forEach((el) => { if (el) el.hidden = generating; });",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("new-CA page does not hide the sections generate ignores: missing %q", want)
		}
	}
	// Only #ed-save is gated by the picker; the generate button is inside the
	// panel the picker already hides.
	if !strings.Contains(js, `gateControl($('#ed-save'), !generating,`) {
		t.Error("the trust-anchor picker must gate #ed-save on the generate choice")
	}
}
