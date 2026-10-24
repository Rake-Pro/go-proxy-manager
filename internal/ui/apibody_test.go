package ui

import (
	"regexp"
	"testing"
)

// TestAPICallersPassObjectsNotStrings guards the api() helper's contract: it
// JSON-encodes opts.body itself, so a caller that pre-stringifies sends a JSON
// string where the server expects an object or list ("the request body
// expects a list, got a string"). Found once on the saved-filters PUT.
func TestAPICallersPassObjectsNotStrings(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`api\([^)]*body:\s*JSON\.stringify\(`)
	if m := re.FindAll(b, -1); len(m) > 0 {
		t.Fatalf("app.js passes a pre-stringified body to api(), which double-encodes it: %q", m)
	}
}
