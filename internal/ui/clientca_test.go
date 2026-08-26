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

// TestStaticBundleExpiryWarningDefault checks the UI's placeholder matches the Go
// default, so the hint does not tell operators the wrong number.
func TestStaticBundleExpiryWarningDefault(t *testing.T) {
	js := readStatic(t, "static/app.js")
	if model.DefaultExpiryWarningDays != 30 {
		t.Fatalf("model.DefaultExpiryWarningDays is %d but app.js says 30 - keep them in step", model.DefaultExpiryWarningDays)
	}
	if !strings.Contains(js, "uses the default of 30 days") {
		t.Error("expiryWarningDays hint does not state the default")
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
