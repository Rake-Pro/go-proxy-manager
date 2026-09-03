package version

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Go toolchain is pinned in .go-version (CI), Dockerfile and the stream
// echo Dockerfile; the docs quote the minimum language version from go.mod.
// hack/bump-go.sh rewrites the toolchain pins; this test is what makes a
// half-done bump, or a Dependabot builder-image bump on its own, fail CI.
func TestGoVersionPinsAgree(t *testing.T) {
	root := filepath.Join("..", "..")
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return string(b)
	}
	want := strings.TrimSpace(read(".go-version"))
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(want) {
		t.Fatalf(".go-version must be X.Y.Z, got %q", want)
	}
	builder := regexp.MustCompile(`(?m)^FROM golang:(\d+\.\d+\.\d+)-alpine`)
	for _, rel := range []string{"Dockerfile", filepath.Join("test", "stream", "echo", "Dockerfile")} {
		m := builder.FindStringSubmatch(read(rel))
		if m == nil {
			t.Errorf("%s: no golang builder image found", rel)
			continue
		}
		if m[1] != want {
			t.Errorf("%s builds with Go %s, .go-version says %s (run hack/bump-go.sh %s)", rel, m[1], want, want)
		}
	}
	if strings.Contains(read(filepath.Join(".github", "workflows", "ci.yml")), "go-version:") {
		t.Errorf("ci.yml pins go-version inline; use go-version-file: .go-version")
	}

	// Minimum language version: go.mod is the source, the docs must quote its
	// major.minor.
	gomod := regexp.MustCompile(`(?m)^go (\d+\.\d+)`).FindStringSubmatch(read("go.mod"))
	if gomod == nil {
		t.Fatal("go.mod: no go directive")
	}
	min := gomod[1]
	for rel, pat := range map[string]string{
		"CONTRIBUTING.md": `(?m)^- Go (\d+\.\d+)\+`,
		filepath.Join("docs", "concepts", "architecture.md"): `\(Go (\d+\.\d+), CGO disabled\)`,
	} {
		m := regexp.MustCompile(pat).FindStringSubmatch(read(rel))
		if m == nil {
			t.Errorf("%s: expected a 'Go X.Y' mention matching %q", rel, pat)
			continue
		}
		if m[1] != min {
			t.Errorf("%s says Go %s, go.mod says %s", rel, m[1], min)
		}
	}
}
