package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"go.yaml.in/yaml/v3"
)

// failBroken is a compile check that fails any config holding an enabled proxy
// host whose name starts with "broken", or settings whose errorPages.dir is
// "bad", standing in for the data plane's dry-run build.
func failBroken(cfg model.Config, settings model.Settings) error {
	for _, h := range cfg.ProxyHosts {
		if !h.Disabled && strings.HasPrefix(h.Name, "broken") {
			return errors.New("proxy host " + h.Name + " does not build")
		}
	}
	if settings.ErrorPages.Dir == "bad" {
		return errors.New("settings errorPages do not build")
	}
	return nil
}

func headOf(t *testing.T, st *Store) string {
	t.Helper()
	h, err := st.Head(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestWritesRefuseConfigThatDoesNotCompile proves every write path that can
// introduce an object refuses, before committing, a result the data plane
// cannot build, while the stored config builds.
func TestWritesRefuseConfigThatDoesNotCompile(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	good, err := st.Save(ctx, sampleHost("app"), Author{})
	if err != nil {
		t.Fatal(err)
	}
	// A commit holding a broken host, made while no check is wired, then
	// removed again, so Revert and RevertObject have a target to restore.
	brokenSHA, err := st.Save(ctx, sampleHost("broken"), Author{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Delete(ctx, "ProxyHost", "broken", Author{}); err != nil {
		t.Fatal(err)
	}
	archive, err := st.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st.SetCompileCheck(failBroken)
	head := headOf(t, st)

	brokenFile, err := yaml.Marshal(sampleHost("broken2"))
	if err != nil {
		t.Fatal(err)
	}
	brokenArchive := archiveWith(t, map[string][]byte{"proxy-hosts/broken2.yaml": brokenFile})

	writes := map[string]func() error{
		"save":   func() error { _, err := st.Save(ctx, sampleHost("broken1"), Author{}); return err },
		"create": func() error { _, err := st.Create(ctx, sampleHost("broken1"), Author{}); return err },
		"dry run": func() error {
			_, _, _, err := st.DryRun(ctx, sampleHost("broken1"))
			return err
		},
		"batch": func() error {
			_, err := st.SaveBatch(ctx, []model.Object{sampleHost("ok"), sampleHost("broken1")}, "", Author{})
			return err
		},
		"apply": func() error {
			_, err := st.ApplyBatch(ctx, []model.Object{sampleHost("broken1")}, nil, "", Author{}, nil)
			return err
		},
		"revert":        func() error { _, err := st.Revert(ctx, brokenSHA, Author{}); return err },
		"revert object": func() error { _, err := st.RevertObject(ctx, "ProxyHost", "broken", brokenSHA, Author{}); return err },
		"restore":       func() error { _, err := st.Restore(ctx, brokenArchive, Author{}); return err },
		"settings": func() error {
			_, _, err := st.UpdateSettings(ctx, func(s *model.Settings) error { s.ErrorPages.Dir = "bad"; return nil }, Author{})
			return err
		},
	}
	for name, write := range writes {
		err := write()
		if !errors.Is(err, ErrNotCompilable) {
			t.Errorf("%s: err = %v, want ErrNotCompilable", name, err)
		}
		if h := headOf(t, st); h != head {
			t.Errorf("%s: a refused write committed %s", name, h)
		}
		if clean, err := st.git.IsClean(ctx); err != nil || !clean {
			t.Errorf("%s: a refused write left the working tree dirty (clean=%v err=%v)", name, clean, err)
		}
	}

	// Writes that build still go through on every path.
	if _, err := st.Save(ctx, sampleHost("other"), Author{}); err != nil {
		t.Fatalf("save of a buildable host: %v", err)
	}
	if _, err := st.Revert(ctx, good, Author{}); err != nil {
		t.Fatalf("revert to a buildable commit: %v", err)
	}
	if _, err := st.Restore(ctx, archive, Author{}); err != nil {
		t.Fatalf("restore of a buildable archive: %v", err)
	}
}

// TestCompileCheckLetsABrokenStoreBeRepaired proves the repair rule: when the
// stored config already fails to build, writes are let through so the
// operator can fix it, a delete of the failing object always works, and once
// the config builds again a failing write is refused.
func TestCompileCheckLetsABrokenStoreBeRepaired(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.Save(ctx, sampleHost("broken"), Author{}); err != nil {
		t.Fatal(err)
	}
	st.SetCompileCheck(failBroken)

	if _, err := st.Save(ctx, sampleHost("app"), Author{}); err != nil {
		t.Fatalf("an unrelated edit of a config that already fails must be allowed: %v", err)
	}
	fixed := sampleHost("broken")
	fixed.Disabled = true
	if _, err := st.Save(ctx, fixed, Author{}); err != nil {
		t.Fatalf("the repairing edit must be allowed: %v", err)
	}
	if _, err := st.Delete(ctx, "ProxyHost", "broken", Author{}); err != nil {
		t.Fatalf("deleting the failing object must be allowed: %v", err)
	}
	if _, err := st.Save(ctx, sampleHost("broken"), Author{}); !errors.Is(err, ErrNotCompilable) {
		t.Fatalf("with the config building again, a failing write must be refused, got %v", err)
	}
}

// TestDomainKeyCollisionRefusedOnWriteWarnedOnLoad proves two hosts the router
// files under one name ("app.example.com" and "app.example.com:8443") are
// refused at write time, while a stored config that already has such a pair
// keeps loading (with a warning) and stays editable.
func TestDomainKeyCollisionRefusedOnWriteWarnedOnLoad(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	victim := sampleHost("victim")
	victim.Domains = []string{"app.example.com"}
	if _, err := st.Save(ctx, victim, Author{}); err != nil {
		t.Fatal(err)
	}
	shadow := sampleHost("zz-shadow")
	shadow.Domains = []string{"APP.example.com:8443"}
	if _, err := st.Save(ctx, shadow, Author{}); err == nil || !strings.Contains(err.Error(), "app.example.com") {
		t.Fatalf("a domain that routes as an existing host's name must be refused, got %v", err)
	}
	if _, err := st.SaveBatch(ctx, []model.Object{shadow}, "", Author{}); err == nil {
		t.Fatal("batch: the colliding domain must be refused")
	}
	if _, err := st.ApplyBatch(ctx, []model.Object{shadow}, nil, "", Author{}, nil); err == nil {
		t.Fatal("apply: the colliding domain must be refused")
	}

	// A collision that arrived out of band (hand edit, pull) still loads.
	b, err := yaml.Marshal(shadow)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "zz-shadow.yaml"), b, 0o640); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("a stored collision must not fail the load: %v", err)
	}
	warned := false
	for _, w := range cfg.Warnings() {
		if strings.Contains(w, "domain app.example.com is claimed by") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("a stored collision must be reported as a load warning, got %v", cfg.Warnings())
	}
	// It stays editable: an unrelated write and the edit that removes it pass.
	if _, err := st.Save(ctx, sampleHost("other"), Author{}); err != nil {
		t.Fatalf("an unrelated write with an existing collision: %v", err)
	}
	shadow.Domains = []string{"other-name.example.com"}
	if _, err := st.Save(ctx, shadow, Author{}); err != nil {
		t.Fatalf("the edit that removes the collision: %v", err)
	}
}

// TestDomainKeyCollisionMayShrinkButNotGrow proves the write check refuses a
// write that adds a router-key collision or adds a host to an existing one,
// while edits that keep or shrink a stored collision pass: an unrelated edit
// of a colliding host, disabling one (also through ApplyBatch, as discovery
// writes), and removing its colliding domain.
func TestDomainKeyCollisionMayShrinkButNotGrow(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	a := sampleHost("a")
	a.Domains = []string{"app.example.com"}
	if _, err := st.Save(ctx, a, Author{}); err != nil {
		t.Fatal(err)
	}
	b := sampleHost("b")
	b.Domains = []string{"app.example.com:1"}
	c := sampleHost("c")
	c.Domains = []string{"app.example.com:2", "c.example.com"}
	// Three hosts on one key, stored out of band.
	for _, h := range []model.ProxyHost{b, c} {
		out, err := yaml.Marshal(h)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", h.Name+".yaml"), out, 0o640); err != nil {
			t.Fatal(err)
		}
	}

	d := sampleHost("d")
	d.Domains = []string{"app.example.com:3"}
	if _, err := st.Save(ctx, d, Author{}); err == nil {
		t.Fatal("a host joining an existing collision must be refused")
	}
	x := sampleHost("x")
	x.Domains = []string{"C.example.com:9"}
	if _, err := st.Save(ctx, x, Author{}); err == nil {
		t.Fatal("a new collision must be refused")
	}

	c.Upstream.Port = 9090
	c.Domains = []string{"APP.example.com:2", "c.example.com"}
	if _, err := st.Save(ctx, c, Author{}); err != nil {
		t.Fatalf("an unrelated edit of a host in an existing collision: %v", err)
	}
	b.Disabled = true
	if _, err := st.ApplyBatch(ctx, []model.Object{b}, nil, "", Author{}, nil); err != nil {
		t.Fatalf("disabling a colliding host through a batch: %v", err)
	}
	b.Disabled = false
	if _, err := st.Save(ctx, b, Author{}); err == nil {
		t.Fatal("re-enabling a host into a collision it had left must be refused")
	}
	c.Domains = []string{"c.example.com"}
	if _, err := st.Save(ctx, c, Author{}); err != nil {
		t.Fatalf("removing a colliding domain: %v", err)
	}
}

// TestErrorPageTemplateRefusedOnWriteKeptOnLoad proves a new or changed inline
// error-page template outside the allow-list (a loop, a value built in a
// variable) is refused on every write path (proxy host save, batch, apply,
// settings), while one already stored loads and does not block an edit that
// keeps it unchanged.
func TestErrorPageTemplateRefusedOnWriteKeptOnLoad(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	const loop = "{{range 1000000}}x{{end}}"
	doubled := `{{$a := printf "%1000000s" ""}}` + strings.Repeat(`{{$a = printf "%s%s" $a $a}}`, 8) + `{{len $a}}`
	host := sampleHost("app")
	oversized := strings.Repeat("<", model.MaxErrorPageTemplateBytes+1)
	deep := strings.Repeat("{{if .Host}}", 17) + "x" + strings.Repeat("{{end}}", 17)
	for _, src := range []string{doubled, oversized, deep, loop} {
		host.ErrorPages = &model.ErrorPagesConfig{Inline: map[string]string{"503": src}}
		if _, err := st.Save(ctx, host, Author{}); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("save %.40q: must be refused, got %v", src, err)
		}
		if _, err := st.SaveBatch(ctx, []model.Object{host}, "", Author{}); err == nil {
			t.Fatalf("batch %.40q: must be refused", src)
		}
		if _, err := st.ApplyBatch(ctx, []model.Object{host}, nil, "", Author{}, nil); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("apply %.40q: must be refused, got %v", src, err)
		}
		settings := model.DefaultSettings()
		settings.ErrorPages = model.ErrorPagesConfig{Inline: map[string]string{"default": src}}
		if _, err := st.SaveSettings(ctx, settings, Author{}); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("settings %.40q: must be refused, got %v", src, err)
		}
	}

	// One that arrived out of band (hand edit, pull, an older version) loads.
	b, err := yaml.Marshal(host)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "app.yaml"), b, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Load(ctx); err != nil {
		t.Fatalf("a stored looping template must not fail the load: %v", err)
	}
	host.Domains = []string{"renamed.example.com"}
	if _, err := st.Save(ctx, host, Author{}); err != nil {
		t.Fatalf("an edit that keeps the stored template unchanged: %v", err)
	}
	if _, err := st.Save(ctx, sampleHost("other"), Author{}); err != nil {
		t.Fatalf("an unrelated write: %v", err)
	}
	host.ErrorPages.Inline["503"] = loop + "y"
	if _, err := st.Save(ctx, host, Author{}); err == nil {
		t.Fatal("changing the looping template must be refused")
	}
}

// TestInvalidDomainRefusedOnWriteWarnedOnLoad proves a host or certificate
// domain that is not a hostname is refused on every write path that adds it,
// while one stored out of band keeps loading with a warning and its object
// stays editable: an unrelated edit, disabling it (also through ApplyBatch, as
// discovery writes) and removing the domain pass; adding another does not.
func TestInvalidDomainRefusedOnWriteWarnedOnLoad(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, d := range []string{"app.example.com:8443", "app.example.com.", "app example.com", "*.example.com"} {
		h := sampleHost("bad")
		h.Domains = []string{d}
		if _, err := st.Save(ctx, h, Author{}); err == nil || !strings.Contains(err.Error(), "must be a hostname") {
			t.Fatalf("save with domain %q: err = %v, want refused", d, err)
		}
		if _, err := st.SaveBatch(ctx, []model.Object{h}, "", Author{}); err == nil {
			t.Fatalf("batch with domain %q must be refused", d)
		}
		if _, err := st.ApplyBatch(ctx, []model.Object{h}, nil, "", Author{}, nil); err == nil {
			t.Fatalf("apply with domain %q must be refused", d)
		}
	}
	cert := model.Certificate{ObjectMeta: model.ObjectMeta{Name: "wild"}, Type: model.CertTypeCustom,
		Domains: []string{"*.example.com", "example.com"}, Custom: &model.CustomCertSpec{CertFile: "w.pem", KeyFile: "w.key"}}
	if _, err := st.Save(ctx, cert, Author{}); err != nil {
		t.Fatalf("a wildcard certificate domain is valid: %v", err)
	}
	cert.Domains = append(cert.Domains, "example.com:443")
	if _, err := st.Save(ctx, cert, Author{}); err == nil || !strings.Contains(err.Error(), "must be a hostname or *.<hostname>") {
		t.Fatalf("a certificate domain with a port must be refused: %v", err)
	}

	odd := sampleHost("odd")
	odd.Domains = []string{"odd.example.com:8443", "odd2.example.com"}
	b, err := yaml.Marshal(odd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(st.Dir(), "proxy-hosts"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "odd.yaml"), b, 0o640); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := st.Load(ctx)
	if err != nil {
		t.Fatalf("a stored invalid domain must not fail the load: %v", err)
	}
	warned := false
	for _, w := range cfg.Warnings() {
		if strings.Contains(w, `proxy host "odd": domains: "odd.example.com:8443" must be a hostname`) {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("a stored invalid domain must be a load warning, got %v", cfg.Warnings())
	}
	odd.Upstream.Port = 9090
	if _, err := st.Save(ctx, odd, Author{}); err != nil {
		t.Fatalf("an unrelated edit of a host with a stored invalid domain: %v", err)
	}
	odd.Disabled = true
	if _, err := st.ApplyBatch(ctx, []model.Object{odd}, nil, "", Author{}, nil); err != nil {
		t.Fatalf("disabling a host with a stored invalid domain through a batch: %v", err)
	}
	odd.Domains = append(odd.Domains, "odd3.example.com.")
	if _, err := st.Save(ctx, odd, Author{}); err == nil {
		t.Fatal("adding a second invalid domain must be refused")
	}
	odd.Domains = []string{"odd.example.com", "odd2.example.com"}
	if _, err := st.Save(ctx, odd, Author{}); err != nil {
		t.Fatalf("the edit that fixes the domain: %v", err)
	}
}

// TestRollbackKeepsStoredInvalidDomain proves revert, scoped revert and
// restore of a state that holds a domain that is not a hostname are accepted
// (the config loads with a warning), so such a state can be rolled back to.
func TestRollbackKeepsStoredInvalidDomain(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	odd := sampleHost("odd")
	odd.Domains = []string{"odd.example.com:8443"}
	b, err := yaml.Marshal(odd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Dir(), "proxy-hosts", "odd.yaml"), b, 0o640); err != nil {
		t.Fatal(err)
	}
	// An unrelated write commits the hand-written file with it.
	withOdd, err := st.Save(ctx, sampleHost("other"), Author{})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := st.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fix := func() {
		t.Helper()
		odd.Domains = []string{"odd.example.com"}
		if _, err := st.Save(ctx, odd, Author{}); err != nil {
			t.Fatalf("fixing the domain: %v", err)
		}
	}
	hasOdd := func(what string) {
		t.Helper()
		cfg, _, err := st.Load(ctx)
		if err != nil {
			t.Fatalf("%s: load: %v", what, err)
		}
		for _, h := range cfg.ProxyHosts {
			if h.Name == "odd" && len(h.Domains) == 1 && h.Domains[0] == "odd.example.com:8443" {
				return
			}
		}
		t.Fatalf("%s: the stored domain was not brought back: %+v", what, cfg.ProxyHosts)
	}
	fix()
	if _, err := st.Revert(ctx, withOdd, Author{}); err != nil {
		t.Fatalf("revert: %v", err)
	}
	hasOdd("revert")
	fix()
	if _, err := st.RevertObject(ctx, "ProxyHost", "odd", withOdd, Author{}); err != nil {
		t.Fatalf("scoped revert: %v", err)
	}
	hasOdd("scoped revert")
	fix()
	if _, err := st.Restore(ctx, archive, Author{}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	hasOdd("restore")
	// Onto a fresh install too.
	fresh := newTestStore(t)
	if _, err := fresh.Restore(ctx, archive, Author{}); err != nil {
		t.Fatalf("restore onto a fresh store: %v", err)
	}
}
