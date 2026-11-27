package model

import (
	"strings"
	"testing"
)

func TestPathBlockValidate(t *testing.T) {
	long := strings.Repeat("a", MaxPathBlockEntryLen)
	many := func(n int, f func(i int) string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = f(i)
		}
		return out
	}
	ext := func(i int) string { return "." + strings.Repeat("x", i%100+1) + "y" }
	cases := []struct {
		name    string
		spec    PathBlockMiddleware
		wantErr string // "" = valid
	}{
		{"extensions only", PathBlockMiddleware{Extensions: []string{".php", ".ENV", ".tar.gz"}}, ""},
		{"names only", PathBlockMiddleware{Names: []string{".git", "wp-admin", "wp-login.php", "a b"}}, ""},
		{"globs only", PathBlockMiddleware{Globs: []string{"/server-status", "/backup/*.zip", `/a\*b`, "/[a-z]?", "/Admin/*", "/", "/a.b/c d", `/[^a-z0-9]\A`}}, ""},
		{"everything", PathBlockMiddleware{
			Extensions: []string{".php"}, Names: []string{".git"}, Globs: []string{"/x/*"},
			AllowFrom: []string{"192.0.2.0/24", "2001:db8::1", "@office"}, DenyStatus: 410, DenyWith: "plain",
		}, ""},
		{"entry at the length bound", PathBlockMiddleware{Names: []string{long}}, ""},
		{"lists at the count bound", PathBlockMiddleware{
			Extensions: many(MaxPathBlockEntries, ext),
			Globs:      many(MaxPathBlockGlobs, func(int) string { return "/x" }),
		}, ""},
		{"globs at the total size", PathBlockMiddleware{Globs: many(16, func(i int) string { return "/" + long[:126] + string(rune('a'+i)) })}, ""},

		{"empty", PathBlockMiddleware{}, "requires at least one extensions, names or globs entry"},
		{"only an exemption", PathBlockMiddleware{AllowFrom: []string{"192.0.2.0/24"}}, "requires at least one"},
		{"extension without dot", PathBlockMiddleware{Extensions: []string{"php"}}, `extensions[0]: "php" must be a dot followed by the extension`},
		{"extension is a dot", PathBlockMiddleware{Extensions: []string{".php", "."}}, "extensions[1]"},
		{"empty extension", PathBlockMiddleware{Extensions: []string{""}}, "extensions[0]"},
		{"extension with slash", PathBlockMiddleware{Extensions: []string{".a/b"}}, `must not contain "/"`},
		{"extension with trailing dot", PathBlockMiddleware{Extensions: []string{".php."}}, "must not end with a dot or a space"},
		{"name with trailing space", PathBlockMiddleware{Names: []string{"x "}}, "must not end with a dot or a space"},
		{"name is a dot segment", PathBlockMiddleware{Names: []string{".."}}, "names[0]"},
		{"empty name", PathBlockMiddleware{Names: []string{".git", ""}}, "names[1]"},
		{"name with slash", PathBlockMiddleware{Names: []string{".git/config"}}, `names[0]: ".git/config" must not contain "/"`},
		{"name with control character", PathBlockMiddleware{Names: []string{"a\x00b"}}, "control character"},
		{"name too long", PathBlockMiddleware{Names: []string{long + "a"}}, "at most 128"},
		{"too many extensions", PathBlockMiddleware{Extensions: many(MaxPathBlockEntries+1, ext)}, "extensions: has 257 entries"},
		{"too many names", PathBlockMiddleware{Names: many(MaxPathBlockEntries+1, func(int) string { return "n" })}, "names: has 257 entries"},
		{"too many globs", PathBlockMiddleware{Globs: many(MaxPathBlockGlobs+1, func(int) string { return "/x" })}, "globs: has 65 entries"},
		{"glob without leading slash", PathBlockMiddleware{Globs: []string{"*.php"}}, `globs[0]: "*.php" must start with "/"`},
		{"malformed glob", PathBlockMiddleware{Globs: []string{"/[a-"}}, "is not a valid pattern"},
		{"glob too long", PathBlockMiddleware{Globs: []string{"/" + long}}, "globs[0]: is 129 bytes"},
		{"glob class in upper case", PathBlockMiddleware{Globs: []string{"/[^A-Z]*"}}, "upper-case letter inside [...]"},
		{"glob class range across cases", PathBlockMiddleware{Globs: []string{"/x[Z-a]"}}, "upper-case letter inside [...]"},
		{"glob class with a quoted upper-case letter", PathBlockMiddleware{Globs: []string{`/x[\A]`}}, "upper-case letter inside [...]"},
		{"glob not UTF-8", PathBlockMiddleware{Globs: []string{"/a\xff"}}, "not valid UTF-8"},
		{"glob with trailing slash", PathBlockMiddleware{Globs: []string{"/uploads/*/"}}, `must not end with "/" (a path is compared without its trailing slash; write "/uploads/*")`},
		{"glob with empty segment", PathBlockMiddleware{Globs: []string{"/a//b"}}, "must not contain an empty segment"},
		{"glob of slashes", PathBlockMiddleware{Globs: []string{"//"}}, "must not contain an empty segment"},
		{"glob segment ending with a dot", PathBlockMiddleware{Globs: []string{"/a./b"}}, "segment ending with a dot or a space"},
		{"glob ending with a space", PathBlockMiddleware{Globs: []string{"/a "}}, "segment ending with a dot or a space"},
		{"glob with control character", PathBlockMiddleware{Globs: []string{"/a\x01"}}, "control character"},
		{"globs over the total size", PathBlockMiddleware{Globs: many(17, func(i int) string { return "/" + long[:126] + string(rune('a'+i)) })}, "globs: are more than 2048 bytes in total"},
		{"name not UTF-8", PathBlockMiddleware{Names: []string{"a\xff"}}, "must be valid UTF-8"},
		{"extension not UTF-8", PathBlockMiddleware{Extensions: []string{".\xfe"}}, "must be valid UTF-8"},
		{"bad allowFrom", PathBlockMiddleware{Names: []string{"x"}, AllowFrom: []string{"office"}}, "allowFrom: has invalid CIDR/IP or @access-list"},
		{"bad list reference", PathBlockMiddleware{Names: []string{"x"}, AllowFrom: []string{"@Bad Name"}}, "is not a valid access list reference"},
		{"denyStatus out of range", PathBlockMiddleware{Names: []string{"x"}, DenyStatus: 200}, "denyStatus: must be a 4xx/5xx code"},
		{"unknown denyWith", PathBlockMiddleware{Names: []string{"x"}, DenyWith: "page"}, "denyWith: must be error-page|plain"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := c.spec
			mw := Middleware{ObjectMeta: ObjectMeta{Name: "probes"}, Type: MWTypePathBlock, PathBlock: &spec}
			settings := DefaultSettings()
			settings.PathBlock = &spec
			for owner, err := range map[string]error{`middleware "probes": pathBlock`: mw.Validate(), "pathBlock": settings.Validate()} {
				if c.wantErr == "" {
					if err != nil {
						t.Errorf("%s: unexpected error: %v", owner, err)
					}
					continue
				}
				if err == nil {
					t.Errorf("%s: accepted, want an error containing %q", owner, c.wantErr)
					continue
				}
				if !strings.Contains(err.Error(), c.wantErr) || !strings.HasPrefix(err.Error(), owner) {
					t.Errorf("%s: error %q, want it to start with the owner and contain %q", owner, err, c.wantErr)
				}
			}
		})
	}
}

func TestPathBlockMiddlewareNeedsSpec(t *testing.T) {
	err := Middleware{ObjectMeta: ObjectMeta{Name: "probes"}, Type: MWTypePathBlock}.Validate()
	if err == nil || !strings.Contains(err.Error(), `middleware "probes": pathBlock: requires at least one`) {
		t.Fatalf("nil spec: %v", err)
	}
}

func TestPathBlockDefaults(t *testing.T) {
	if got := (PathBlockMiddleware{}).DenyStatusOrDefault(); got != 404 {
		t.Errorf("default deny status = %d, want 404", got)
	}
	if got := (PathBlockMiddleware{DenyStatus: 403}).DenyStatusOrDefault(); got != 403 {
		t.Errorf("deny status = %d, want 403", got)
	}
	if DefaultSettings().PathBlock != nil {
		t.Error("the default settings must not carry a path block list")
	}
}

func TestPathBlockSwitch(t *testing.T) {
	for _, v := range []string{"", PathBlockInherit, PathBlockOff} {
		h := ProxyHost{ObjectMeta: ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"},
			Upstream: Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80}, PathBlock: v}
		if err := h.Validate(); err != nil {
			t.Errorf("pathBlock %q: %v", v, err)
		}
	}
	h := ProxyHost{ObjectMeta: ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"},
		Upstream: Upstream{Scheme: "http", Host: "192.0.2.10", Port: 80}, PathBlock: "on"}
	if err := h.Validate(); err == nil || !strings.Contains(err.Error(), `proxy host "app": pathBlock: must be "inherit" or "off", got "on"`) {
		t.Errorf("pathBlock on: %v", err)
	}
}

// A path-block middleware's "@<list>" exemption must resolve, like a rate
// limit's; settings.pathBlock is cross-checked by ValidatePathBlockRefs.
func TestPathBlockAccessListRefs(t *testing.T) {
	spec := &PathBlockMiddleware{Names: []string{".git"}, AllowFrom: []string{"192.0.2.0/24", "@office", "@vpn"}}
	if got := strings.Join(spec.AccessListRefs(), ","); got != "office,vpn" {
		t.Fatalf("AccessListRefs = %q", got)
	}
	office := AccessList{ObjectMeta: ObjectMeta{Name: "office"}, DefaultAction: ActionDeny}
	vpn := AccessList{ObjectMeta: ObjectMeta{Name: "vpn"}, DefaultAction: ActionDeny}
	cfg := Config{
		AccessLists: []AccessList{office},
		Middlewares: []Middleware{{ObjectMeta: ObjectMeta{Name: "probes"}, Type: MWTypePathBlock, PathBlock: spec}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `pathBlock.allowFrom`) || !strings.Contains(err.Error(), `unknown accessList "vpn"`) {
		t.Fatalf("dangling middleware reference: %v", err)
	}
	settings := DefaultSettings()
	settings.PathBlock = spec
	if err := settings.ValidatePathBlockRefs(cfg); err == nil || !strings.Contains(err.Error(), `pathBlock.allowFrom: references unknown accessList "vpn"`) {
		t.Fatalf("dangling settings reference: %v", err)
	}
	cfg.AccessLists = append(cfg.AccessLists, vpn)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("resolved middleware reference: %v", err)
	}
	if err := settings.ValidatePathBlockRefs(cfg); err != nil {
		t.Fatalf("resolved settings reference: %v", err)
	}
	if err := DefaultSettings().ValidatePathBlockRefs(Config{}); err != nil {
		t.Fatalf("no path block list: %v", err)
	}
}

// The switch on a discovery template is held to the host's rule and reaches
// the stand-in host the reference check validates.
func TestPathBlockTemplateSwitch(t *testing.T) {
	settings := DefaultSettings()
	settings.IngressDiscovery = validIngressDiscovery()
	for _, v := range []string{"", PathBlockInherit, PathBlockOff} {
		settings.IngressDiscovery.Template.PathBlock = v
		if err := settings.Validate(); err != nil {
			t.Fatalf("template pathBlock %q: %v", v, err)
		}
	}
	settings.IngressDiscovery.Template.PathBlock = "on"
	err := settings.Validate()
	if err == nil || !strings.Contains(err.Error(), `pathBlock: must be "inherit" or "off"`) || !strings.Contains(err.Error(), "ingressDiscovery.") {
		t.Fatalf("template pathBlock on: %v", err)
	}
}
