package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

const testHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// redactForReply blanks hashes on a copy: the value handed in, and everything
// it shares with the stored or cached config, keeps the real hash, and a
// member that only happens to be called passwordHash is left alone.
func TestRedactForReplyCopies(t *testing.T) {
	newHost := func() model.ProxyHost {
		return model.ProxyHost{
			ObjectMeta: model.ObjectMeta{Name: "app", Labels: map[string]string{"passwordHash": "keep-me"}},
			Auth:       &model.AuthMiddleware{Mode: model.AuthModeBasic, Basic: &model.BasicAuthSpec{Users: []model.BasicAuthUser{{Username: "a", PasswordHash: testHash}}}},
			Locations: []model.Location{{Path: "/x", Auth: &model.AuthMiddleware{Mode: model.AuthModeBasic, Basic: &model.BasicAuthSpec{Users: []model.BasicAuthUser{
				{Username: "b", PasswordHash: testHash}, {Username: "c"}}}}}},
		}
	}
	h := newHost()
	cfg := model.Config{ProxyHosts: []model.ProxyHost{h}}
	tests := []struct {
		name string
		v    any
	}{
		{"value", h},
		{"pointer", &h},
		{"list", []any{h}},
		{"whole config", cfg},
		{"map", map[string]any{"object": &h}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := redactForReply(tc.v)
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			out := string(b)
			if strings.Contains(out, testHash) || strings.Count(out, `"passwordHash":"***"`) != 2 {
				t.Fatalf("reply = %s, want both hashes as ***", out)
			}
			if !strings.Contains(out, `"passwordHash":"keep-me"`) {
				t.Fatalf("a label named passwordHash was redacted: %s", out)
			}
			if !strings.Contains(out, `{"username":"c","passwordHash":""}`) {
				t.Fatalf("an empty hash must stay empty: %s", out)
			}
			for _, x := range []model.ProxyHost{h, cfg.ProxyHosts[0]} {
				if x.Auth.Basic.Users[0].PasswordHash != testHash || x.Locations[0].Auth.Basic.Users[0].PasswordHash != testHash {
					t.Fatal("redactForReply modified the value it was given")
				}
			}
		})
	}
}

// No part of a passwordHash value survives in a YAML preview, whatever form
// the file was written in.
func TestRedactHashesYAML(t *testing.T) {
	legacy, err := yaml.Marshal(map[string]any{"basicAuth": []map[string]string{
		{"username": "a", "passwordHash": "my SECRETA pw"}, {"username": "b", "passwordHash": `x"y SECRETB`}}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, in string
		want     string // exact output, or "" to only check that SECRET is gone
	}{
		{"plain", "      passwordHash: " + testHash + "\n", "      passwordHash: \"***\"\n"},
		{"double quoted", `passwordHash: "SECRET"`, `passwordHash: "***"`},
		{"single quoted", `passwordHash: 'SECRET'`, `passwordHash: "***"`},
		{"quoted key", `        "passwordHash": "SECRET"`, `        "passwordHash": "***"`},
		{"anchor", `passwordHash: &h "SECRET"`, `passwordHash: "***"`},
		{"value with spaces", string(legacy), ""},
		{"flow", `- {username: a, passwordHash: my SECRET pw}`, `- {username: a, passwordHash: "***"}`},
		{"json on one line", `{"auth":{"basic":{"users":[{"username":"a","passwordHash":"SECRET"},{"username":"b","passwordHash":"SECRET2"}]}},"x":1}`,
			`{"auth":{"basic":{"users":[{"username":"a","passwordHash": "***"},{"username":"b","passwordHash": "***"}]}},"x":1}`},
		{"flow with anchor and escape", `{passwordHash: &h "SE\"CRET", realm: r}`, `{passwordHash: "***", realm: r}`},
		{"block scalar", "users:\n  - username: a\n    passwordHash: >-\n      SECRET\n      SECRET\n    realm: r\n",
			"users:\n  - username: a\n    passwordHash: \"***\"\n      ***\n      ***\n    realm: r\n"},
		{"literal with a blank line", "passwordHash: |\n  SECRET\n\n  SECRET\nrealm: r\n", "passwordHash: \"***\"\n  ***\n\n  ***\nrealm: r\n"},
		{"value on the next line", "passwordHash:\n    SECRET\nrealm: r\n", "passwordHash: \"***\"\n    ***\nrealm: r\n"},
		{"flow quoted over two lines", "{passwordHash: \"SEC\n  RET\", realm: r}", "{passwordHash: \"***\"\n  ***, realm: r}"},
		{"bcrypt under another key", "x: &h " + testHash + "\n", "x: &h ***\n"},
		{"no hash", "username: a\nrealm: r\n", "username: a\nrealm: r\n"},
		{"double-quoted continuation at the key column", "users:\n  - username: a\n    passwordHash: \"$2a$10$SECRETxx\\\n    SECRETyy\"\n    realm: r\n",
			"users:\n  - username: a\n    passwordHash: \"***\"\n    ***\n    realm: r\n"},
		{"double-quoted continuation at column 0", "users:\n  - username: a\n    passwordHash: \"$2a$10$SECRETxx\\\nSECRETyy\"\n  - username: b\n",
			"users:\n  - username: a\n    passwordHash: \"***\"\n***\n  - username: b\n"},
		{"multi-line flow mapping", "users: [\n  {\n    username: a,\n    passwordHash: \"$2a$10$SECRETxx\\\nSECRETyy\"\n  }\n]\n", ""},
		{"escaped dollars under an anchor elsewhere", "x-h: &h \"" + strings.ReplaceAll(testHash, "$", `\x24`) + "\"\npasswordHash: *h\n", ""},
		{"escaped dollars, complex key", "? passwordHash\n: \"" + strings.ReplaceAll(testHash, "$", `\u0024`) + "\"\n", ""},
		{"crlf block scalar", "users:\r\n  - username: a\r\n    passwordHash: >-\r\n      SECRET\r\n    realm: r\r\n", "users:\r\n  - username: a\r\n    passwordHash: \"***\"\n      ***\n    realm: r\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactHashesYAML(tc.in)
			if tc.want != "" && got != tc.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tc.want)
			}
			for _, part := range []string{"SECRET", "SEC", "RET", "pw", "y SECRETB", testHash[7:20], testHash[20:]} {
				if strings.Contains(got, part) {
					t.Fatalf("%q survives in\n%s", part, got)
				}
			}
		})
	}
}

// In a diff the old and the new side continue a value independently.
func TestRedactHashesPatch(t *testing.T) {
	patch := strings.Join([]string{
		"diff --git a/proxy-hosts/a.yaml b/proxy-hosts/a.yaml",
		"--- a/proxy-hosts/a.yaml",
		"+++ b/proxy-hosts/a.yaml",
		"@@ -1,6 +1,6 @@",
		"   users:",
		"     - username: a",
		"-      passwordHash: >-",
		"-        OLDSECRET",
		"+      passwordHash: >-",
		"+        NEWSECRET",
		"   realm: r",
		"@@ -9,3 +9,3 @@",
		"       passwordHash: >-",
		"-        OLDSECRET",
		"+        NEWSECRET",
		"     realm: r",
		"-  \"passwordHash\": \"OLDSECRET\"",
		"+  {passwordHash: NEWSECRET, x: 1}",
	}, "\n")
	patch += "\n" + strings.Join([]string{
		// A hunk that starts on the continuation line of a value whose key
		// line is outside it, and a header that repeats a key line.
		"@@ -20,4 +20,4 @@",
		"         SECRETTAIL\"",
		"     realm: r",
		"-    x: 1",
		"+    x: 2",
		"@@ -30,2 +30,2 @@     passwordHash: plain SECRET words",
		"-    x: 1",
		"+    x: 2",
	}, "\n")
	got := redactHashesPatch(patch)
	if strings.Contains(got, "SECRET") {
		t.Fatalf("a hash survives:\n%s", got)
	}
	for _, keep := range []string{"--- a/proxy-hosts/a.yaml", "   realm: r", "     realm: r", "+  {passwordHash: \"***\", x: 1}"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("%q lost:\n%s", keep, got)
		}
	}
}

type embeddedUsers struct {
	Users []model.BasicAuthUser `json:"users"`
}

// encoding/json promotes the exported fields of an embedded unexported struct.
type replyEmbeddingUnexported struct {
	embeddedUsers
	Name string `json:"name"`
}

type cyclicWithUsers struct {
	Next  *cyclicWithUsers      `json:"next"`
	Users []model.BasicAuthUser `json:"users"`
}

type cyclicWithoutUsers struct {
	Next *cyclicWithoutUsers `json:"next"`
}

// A reply the walk cannot redact safely fails instead of leaking or
// overflowing the stack; writeJSON then answers 500 with no hash in it.
func TestRedactForReplyRefusesWhatItCannotCopy(t *testing.T) {
	loop := &cyclicWithUsers{Users: []model.BasicAuthUser{{Username: "a", PasswordHash: testHash}}}
	loop.Next = loop
	plain := &cyclicWithoutUsers{}
	plain.Next = plain
	tests := []struct {
		name    string
		v       any
		wantErr bool
	}{
		{"embedded unexported struct", replyEmbeddingUnexported{embeddedUsers: embeddedUsers{Users: []model.BasicAuthUser{{Username: "a", PasswordHash: testHash}}}}, true},
		{"pointer cycle holding users", loop, true},
		{"pointer cycle without users", plain, false},
		{"same pointer twice", []*cyclicWithUsers{{Users: loop.Users}, {Users: loop.Users}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := redactForReply(tc.v)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				return
			}
			w := httptest.NewRecorder()
			writeJSON(w, http.StatusOK, tc.v)
			if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), testHash) {
				t.Fatalf("writeJSON = %d %s, want 500 without the hash", w.Code, w.Body.String())
			}
		})
	}
	if loop.Users[0].PasswordHash != testHash {
		t.Fatal("input modified")
	}
}

// A hunk start is only treated as possibly inside a hash in a file that can
// hold users, and there only for a line that could continue a hash.
func TestRedactHashesPatchHunkStart(t *testing.T) {
	// A PEM line with no "+" or "=" looks like a hash continuation, so the
	// file kind decides.
	const pemLine = "MIIBszCCAVmgAwIBAgIUQ5h1xY2dB9kD8vX3sZ0lP6mTqJ4wCgYIKoZIzj0EAwIw"
	tests := []struct {
		name, file, first string
		redacted          bool
	}{
		{"client CA PEM", "client-cas/corp.yaml", "      " + pemLine, false},
		{"page text in settings", "settings.yaml", "            <p>line j</p>", false},
		{"word of a settings block scalar", "settings.yaml", "            maintenance", true},
		{"quoted hash tail in a proxy host", "proxy-hosts/app.yaml", `          eIjZAgcfl7p92ldGxad68LJZdL17lhWy"`, true},
		{"escaped hash tail in a middleware", "middlewares/gate.yaml", `          \x2410\x24N9qo8uLOickgx2ZMRZoMy\`, true},
		{"hash tail closing a flow mapping", "proxy-hosts/app.yaml", `          eIjZAgcfl7p92ldGxad68LJZdL17lhWy"}`, true},
		{"hash tail before the next flow key", "proxy-hosts/app.yaml", `          eIjZAgcfl7p92ldGxad68LJZdL17lhWy", realm: r}`, true},
		{"hash tail before a comma", "middlewares/gate.yaml", `          eIjZAgcfl7p92ldGxad68LJZdL17lhWy",`, true},
		{"hash tail before a comment", "access-lists/lan.yaml", `          eIjZAgcfl7p92ldGxad68LJZdL17lhWy" # note`, true},
		{"hash tail with trailing blanks", "settings.yaml", `          eIjZAgcfl7p92ldGxad68LJZdL17lhWy"  `, true},
		{"quoted key in settings", "settings.yaml", `        "404": |`, false},
		{"unreadable header counts as a users file", `"proxy-hosts/caf\303\251.yaml"`, `          eIjZAgcfl7p92ldGxad68LJZdL17lhWy"`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := "diff --git a/" + tc.file + " b/" + tc.file
			if strings.HasPrefix(tc.file, `"`) {
				header = `diff --git "a/` + tc.file[1:] + ` "b/` + tc.file[1:]
			}
			patch := strings.Join([]string{
				header,
				"--- a/" + tc.file,
				"+++ b/" + tc.file,
				"@@ -9,4 +9,4 @@",
				" " + tc.first,
				"-  x: 1",
				"+  x: 2",
			}, "\n")
			got := redactHashesPatch(patch)
			if kept := strings.Contains(got, tc.first); kept == tc.redacted {
				t.Fatalf("redacted = %v, want %v:\n%s", !kept, tc.redacted, got)
			}
			if !strings.Contains(got, "+  x: 2") {
				t.Fatalf("the entry after the first line was redacted:\n%s", got)
			}
		})
	}
}

// The combined diff of a merge commit has one marker column per parent; a
// hash tail in it is blanked in a file that can hold users, and only there.
func TestRedactHashesPatchCombinedDiff(t *testing.T) {
	const tail = `JZdL17lhWy"`
	patch := func(file string) string {
		return strings.Join([]string{
			"diff --cc " + file,
			"index 1111111,2222222..3333333",
			"--- a/" + file,
			"+++ b/" + file,
			"@@@ -9,4 -9,4 +9,4 @@@",
			"  name: app",
			"-           " + tail,
			"++          " + tail,
			"++  passwordHash: $2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
		}, "\n")
	}
	got := redactHashesPatch(patch("proxy-hosts/app.yaml"))
	if strings.Contains(got, "JZdL17lhWy") || !strings.Contains(got, "  name: app") {
		t.Fatalf("users file:\n%s", got)
	}
	got = redactHashesPatch(patch("client-cas/corp.yaml"))
	if !strings.Contains(got, "++          "+tail) || strings.Contains(got, "$2a$10$") {
		t.Fatalf("other file:\n%s", got)
	}
}

// keys runs in one pass: a line with many flow keys (a string value such as
// x{passwordHash:b,passwordHash:b,...) costs time in proportion to its
// length. The ratio between two sizes is compared, not wall time, so a slow
// machine cannot flake it: quadrupling the input quadruples the time of a
// linear pass and multiplies a rescanning one by sixteen.
func TestRedactorIsLinear(t *testing.T) {
	line := func(n int) string {
		return "displayName: x{" + strings.Repeat("passwordHash:b,", n/len("passwordHash:b,"))
	}
	best := func(s string, runs int) time.Duration {
		var min time.Duration
		for i := 0; i < runs; i++ {
			start := time.Now()
			out := redactHashesYAML(s)
			d := time.Since(start)
			if strings.Contains(out, "passwordHash:b") {
				t.Fatalf("a value survived")
			}
			if min == 0 || d < min {
				min = d
			}
		}
		return min
	}
	small, big := line(128<<10), line(512<<10)
	if got := redactHashesYAML(small); strings.Count(got, `passwordHash: "***"`) != strings.Count(small, "passwordHash:b") {
		t.Fatalf("not every value was redacted")
	}
	// The large input runs twice: a quadratic pass takes many seconds on it.
	ts, tb := best(small, 3), best(big, 2)
	if tb > 10*ts+50*time.Millisecond {
		t.Fatalf("512 KiB took %v, 128 KiB %v: more than linear", tb, ts)
	}
}
