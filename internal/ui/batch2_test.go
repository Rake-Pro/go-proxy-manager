package ui

import (
	"strings"
	"testing"
)

// Every editor that rebuilds an object on save must carry a key it renders no
// control for, or render a control for it; otherwise a UI save drops the key
// from the committed object.
func TestBatch2KeysSurviveALoadEditSave(t *testing.T) {
	js := loadAppJS(t)
	tests := []struct {
		key  string
		want []string
	}{
		{"rateLimit.ipv6Prefix", []string{
			"if (rl.ipv6Prefix) spec.ipv6Prefix = rl.ipv6Prefix;",
		}},
		{"dockerDiscovery.allowLinkLocal", []string{
			"switchHtml('set-dkr-linklocal', !!dd.allowLinkLocal, ",
			"if (isOn('set-dkr-linklocal')) dockerDiscovery.allowLinkLocal = true;",
		}},
	}
	for _, tc := range tests {
		for _, want := range tc.want {
			if !strings.Contains(js, want) {
				t.Errorf("%s is no longer carried through a load-edit-save (missing %q)", tc.key, want)
			}
		}
	}
}

// A read returns each basic-auth hash as "***" and the API keeps the stored
// hash only for the same username, so the credential rows must refuse a
// rename without a new password instead of sending a placeholder the API
// rejects, and must remember the loaded name to tell.
func TestBasicUserRowsRefuseRenameWithoutPassword(t *testing.T) {
	js := loadAppJS(t)
	for _, want := range []string{
		"div._user = u.username || '';",
		"if (!pw && r._hash && name !== r._user)",
		"if (pw) u.password = pw; else if (r._hash) u.passwordHash = r._hash;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("basic-auth credential rows: missing %q", want)
		}
	}
}

// Webhook and notification target URLs accept ${ENV:...} / ${FILE:...}
// placeholders; the hints next to those fields say so.
func TestDeliveryURLHintsMentionPlaceholders(t *testing.T) {
	js := loadAppJS(t)
	i := strings.Index(js, "const NTF_URL_HINTS = {")
	if i == -1 {
		t.Fatal("NTF_URL_HINTS not found")
	}
	block := js[i : i+strings.Index(js[i:], "};")]
	for _, typ := range []string{"ntfy:", "discord:", "generic:"} {
		j := strings.Index(block, typ)
		if j == -1 {
			t.Fatalf("NTF_URL_HINTS has no %s entry", typ)
		}
		line := block[j : j+strings.Index(block[j:], "\n")]
		if !strings.Contains(line, "${ENV:...}") || !strings.Contains(line, "${FILE:...}") {
			t.Errorf("the %s URL hint does not mention placeholders: %s", strings.TrimSuffix(typ, ":"), line)
		}
	}
	if !strings.Contains(js, `A URL may be a <span class="mono">\${ENV:...}</span> or <span class="mono">\${FILE:...}</span> placeholder`) {
		t.Error("the lifecycle webhooks hint does not mention URL placeholders")
	}
}
