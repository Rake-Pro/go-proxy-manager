package ui

import (
	"strings"
	"testing"
)

// The access-list editor rebuilds each source from its row on save. Keys the
// row renders no control for must be carried forward from the loaded source, or
// a UI save silently drops them from the committed list.
func TestAccessListSourceKeysWithoutAControlAreCarriedOnSave(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, key := range []string{"maxCoverage", "maxAge"} {
		for _, want := range []string{
			"if (s." + key + ") d._" + key + " = s." + key + ";",
			"if (r._" + key + ") src." + key + " = r._" + key + ";",
		} {
			if !strings.Contains(js, want) {
				t.Errorf("access-list source editor no longer carries %s forward on save (missing %q)", key, want)
			}
		}
	}
}
