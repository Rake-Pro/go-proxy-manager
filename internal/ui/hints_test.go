package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The in-app help system is three artefacts that only work together: the
// data-hint attributes in app.js, the registry in hints/hints.json, and the
// anchors in docs/. Nothing at runtime notices when one of them drifts - a
// missing id renders no "?" at all, a dead entry is invisible, and a stale
// anchor is a "Learn more" link to a 404. So the coupling is a test.

type hintEntry struct {
	Text string `json:"text"`
	Doc  string `json:"doc"`
}

// Controls that deliberately carry no data-hint. Each key is a substring unique
// to that tag in app.js; the value is why it is exempt. A control that is not
// listed here and has no hint fails TestHintCoverage.
var hintExempt = map[string]string{
	`id="cm-typed"`:    "confirmation modal: the operator retypes a name, and the modal body is the explanation",
	`id="cm-prompt"`:   "confirmation modal: free-text prompt, labelled by the modal itself",
	`id="hostFilter"`:  "list filter box",
	`id="certFilter"`:  "list filter box",
	`id="gsFilter"`:    "list filter box",
	`id="tokFilter"`:   "list filter box",
	`id="logFilter"`:   "list filter box",
	`id="histFilter"`:  "list filter box",
	`id="hostSelAll"`:  "bulk-selection checkbox, not a config field",
	`class="host-sel"`: "bulk-selection checkbox, not a config field",
	`id="restoreFile"`: "hidden file input behind the Restore button",
	`id="tok-reveal"`:  "read-only display of the token secret that was just minted",

	`<input class="mono" placeholder="${esc(placeholder || 'add...')}"`: "the chip-input's add box; the chip-input wrapper carries the hint",
	`class="field mono kv-k"`: "generic key/value repeater; every call site's wrapper carries the hint",
	`class="field mono kv-v"`: "generic key/value repeater; every call site's wrapper carries the hint",

	`id="tok-read-all"`:      "column toggle over the scope matrix; the matrix carries the hint",
	`id="tok-write-all"`:     "column toggle over the scope matrix; the matrix carries the hint",
	`class="tok-read"`:       "one cell of the scope matrix; the matrix carries the hint",
	`class="tok-write"`:      "one cell of the scope matrix; the matrix carries the hint",
	`value="${esc(m.name)}"`: "middleware check-list item; the check-list carries the hint",
	`value="${esc(a.name)}"${selAl.indexOf(a.name) !== -1 ? ' checked' : ''}`:                  "host access-list item; the check-list carries the hint",
	`value="${esc(a.name)}"${selAl.indexOf(a.name) !== -1 && !hasBasic ? ' checked' : ''}`:     "stream access-list item; the check-list carries the hint",
	`value="${esc(p.name)}"${storedProvs.indexOf(p.name) !== -1 ? ' checked' : ''}`:            "admin SSO provider item; the check-list carries the hint",
	`<input type="checkbox" value="${esc(n)}" checked/`:                                        "admin SSO provider stored under a name this instance does not define",
	`class="field mono sh-name" style="flex:1 1 160px" value="${esc(name || '')}" aria-label=`: "read-only row for a header value this editor cannot express",
	`<input class="field mono" style="flex:2 1 190px" value="${esc(JSON.stringify(raw))}"`:     "read-only row for a header value this editor cannot express",
}

func loadHintRegistry(t *testing.T) map[string]hintEntry {
	t.Helper()
	b, err := hintsFS.ReadFile("hints/hints.json")
	if err != nil {
		t.Fatalf("read hints.json: %v", err)
	}
	var reg map[string]hintEntry
	if err := json.Unmarshal(b, &reg); err != nil {
		t.Fatalf("hints.json is not a JSON object of hint entries: %v", err)
	}
	if len(reg) == 0 {
		t.Fatal("hints.json is empty")
	}
	return reg
}

func loadAppJS(t *testing.T) string {
	t.Helper()
	b, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	return string(b)
}

var dataHintRe = regexp.MustCompile(`data-hint="([A-Za-z][\w.-]*)"`)

// (a) Every data-hint the UI renders resolves to a registry entry.
func TestHintIDsExistInRegistry(t *testing.T) {
	reg := loadHintRegistry(t)
	js := loadAppJS(t)
	seen := map[string]bool{}
	for _, m := range dataHintRe.FindAllStringSubmatch(js, -1) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		if _, ok := reg[m[1]]; !ok {
			t.Errorf("app.js renders data-hint=%q but hints.json has no such entry", m[1])
		}
	}
	if len(seen) == 0 {
		t.Fatal("app.js renders no data-hint attributes at all")
	}
	t.Logf("distinct data-hint ids rendered: %d", len(seen))
}

// (b) Every registry entry is reachable from the UI. Most ids appear as a
// data-hint literal; glossary ids reach the UI as plain string literals
// (GLOSSARY), and a few are threaded through switchHtml's hint argument, so a
// quoted occurrence counts too.
func TestNoDeadHintEntries(t *testing.T) {
	reg := loadHintRegistry(t)
	js := loadAppJS(t)
	var dead []string
	for id := range reg {
		if strings.Contains(js, `data-hint="`+id+`"`) ||
			strings.Contains(js, `'`+id+`'`) ||
			strings.Contains(js, `"`+id+`"`) {
			continue
		}
		dead = append(dead, id)
	}
	sort.Strings(dead)
	for _, id := range dead {
		t.Errorf("hints.json entry %q is never used by app.js", id)
	}
}

// Every entry has to say something and point somewhere.
func TestHintEntriesAreWellFormed(t *testing.T) {
	for id, e := range loadHintRegistry(t) {
		if strings.TrimSpace(e.Text) == "" {
			t.Errorf("%s: empty text", id)
		}
		if strings.TrimSpace(e.Doc) == "" {
			t.Errorf("%s: empty doc target", id)
		}
		for _, r := range e.Text {
			if r > 127 {
				t.Errorf("%s: text is not ASCII (%q)", id, r)
				break
			}
		}
	}
}

// (c) Every doc target resolves to a file under docs/, and every fragment
// resolves to an anchor in that file.
func TestHintDocTargetsResolve(t *testing.T) {
	reg := loadHintRegistry(t)
	docsDir := filepath.Join("..", "..", "docs")
	if _, err := os.Stat(docsDir); err != nil {
		t.Skipf("docs/ not present next to the package: %v", err)
	}
	cache := map[string]map[string]bool{}
	for _, id := range sortedKeys(reg) {
		doc := reg[id].Doc
		path, frag, _ := strings.Cut(doc, "#")
		full := filepath.Join(docsDir, filepath.FromSlash(path))
		if st, err := os.Stat(full); err != nil || st.IsDir() {
			t.Errorf("%s: doc target %q does not resolve to a file under docs/", id, doc)
			continue
		}
		if frag == "" {
			continue
		}
		anchors, ok := cache[path]
		if !ok {
			b, err := os.ReadFile(full)
			if err != nil {
				t.Errorf("%s: read %s: %v", id, path, err)
				continue
			}
			anchors = docAnchors(string(b))
			cache[path] = anchors
		}
		if !anchors[frag] {
			t.Errorf("%s: %s has no anchor %q (add the anchor, or point the hint at one that exists)", id, path, frag)
		}
	}
}

var (
	htmlIDRe  = regexp.MustCompile(`id="([^"]+)"`)
	attrIDRe  = regexp.MustCompile(`\{\s*#([A-Za-z0-9_.:-]+)\s*\}`)
	headingRe = regexp.MustCompile(`(?m)^#{1,6}\s+(.*)$`)
	linkRe    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	slugDrop  = regexp.MustCompile(`[^\w\s-]`)
	slugSpace = regexp.MustCompile(`\s+`)
)

// docAnchors collects every id a MkDocs page exposes: explicit HTML anchors,
// attr_list ids on headings, and the slugs python-markdown's toc extension
// generates from heading text.
func docAnchors(md string) map[string]bool {
	out := map[string]bool{}
	for _, m := range htmlIDRe.FindAllStringSubmatch(md, -1) {
		out[m[1]] = true
	}
	for _, m := range attrIDRe.FindAllStringSubmatch(md, -1) {
		out[m[1]] = true
	}
	for _, m := range headingRe.FindAllStringSubmatch(md, -1) {
		out[slugify(m[1])] = true
	}
	return out
}

func slugify(s string) string {
	s = attrIDRe.ReplaceAllString(s, "")
	s = linkRe.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "`", "")
	s = slugDrop.ReplaceAllString(s, "")
	s = strings.ToLower(strings.TrimSpace(s))
	return slugSpace.ReplaceAllString(s, "-")
}

var controlTagRe = regexp.MustCompile(`<(?:input|select|textarea)\b[^>]*`)

// (d) Every form control in the SPA carries a hint, unless it is on the
// documented exemption list above.
func TestHintCoverage(t *testing.T) {
	js := loadAppJS(t)
	total, withHint, exempt := 0, 0, 0
	var uncovered []string
	for _, tag := range controlTagRe.FindAllString(js, -1) {
		// "<select>" written in a comment matches with no attributes at all.
		if !strings.ContainsAny(tag, " \t\n") {
			continue
		}
		total++
		if strings.Contains(tag, "data-hint=") {
			withHint++
			continue
		}
		hit := false
		for frag := range hintExempt {
			if strings.Contains(tag, frag) {
				hit = true
				break
			}
		}
		if hit {
			exempt++
			continue
		}
		flat := strings.Join(strings.Fields(tag), " ")
		if len(flat) > 160 {
			flat = flat[:160]
		}
		uncovered = append(uncovered, flat)
	}
	for _, u := range uncovered {
		t.Errorf("form control has no data-hint and is not on the exemption list: %s", u)
	}
	t.Logf("form controls: %d total, %d with a hint, %d allowlisted", total, withHint, exempt)
	if withHint+exempt != total {
		t.Errorf("coverage accounting is off: %d + %d != %d", withHint, exempt, total)
	}
}

// Every exemption must still match something: a stale entry would silently
// widen the allowlist as the UI changes.
func TestHintExemptionsAreLive(t *testing.T) {
	js := loadAppJS(t)
	for frag, why := range hintExempt {
		if !strings.Contains(js, frag) {
			t.Errorf("exemption %q (%s) no longer matches anything in app.js - drop it", frag, why)
		}
	}
}

func sortedKeys(m map[string]hintEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
