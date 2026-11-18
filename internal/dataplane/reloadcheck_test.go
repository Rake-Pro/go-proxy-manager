package dataplane

import (
	"context"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// lateFailingConfig adds a host to cfg whose build fails after the client-CA
// anchors are compiled (its clientAuth names a CA that does not exist).
func lateFailingConfig(cfg model.Config) model.Config {
	bad := cfg
	bad.ProxyHosts = append(append([]model.ProxyHost{}, cfg.ProxyHosts...), model.ProxyHost{
		ObjectMeta: model.ObjectMeta{Name: "n"}, Domains: []string{"n.example"},
		Upstream: model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
		TLS:      model.TLSSettings{ForceSSL: true, ClientAuth: &model.ClientAuth{CARef: "missing", Mode: "require"}},
	})
	return bad
}

func watchedAnchor(a *clientCAAnchor) bool {
	p := crlAnchors.Load()
	if p == nil {
		return false
	}
	for _, w := range *p {
		if w == a {
			return true
		}
	}
	return false
}

// TestFailedReloadKeepsLiveCRLAnchorsWatched proves the CRL watch keeps the
// live router's anchors when a later reload, or a compile check, fails or is
// discarded: a CRL refreshed afterwards still revokes on the live host.
func TestFailedReloadKeepsLiveCRLAnchorsWatched(t *testing.T) {
	t.Cleanup(func() { registerCRLAnchors(nil) })
	dir := t.TempDir()
	caPEM, caCert, caKey := crlCA(t, "corp")
	crlPath := filepath.Join(dir, "crl.pem")
	if err := os.WriteFile(crlPath, makeCRL(t, caCert, caKey, time.Now().Add(24*time.Hour)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, leaf := clientCertSerial(t, caCert, caKey, 5, "laptop")
	chains := [][]*x509.Certificate{{leaf, caCert}}

	s := New(Config{CertDir: dir})
	cfg := crlRouterConfig(caPEM, model.ClientCA{CRLFile: "crl.pem"})
	if err := s.Reload(cfg); err != nil {
		t.Fatal(err)
	}
	live := s.current()
	liveAnchor := live.clientAuth["m.example"].anchor
	if !watchedAnchor(liveAnchor) {
		t.Fatal("a successful reload must publish its anchors to the CRL watch")
	}

	bad := lateFailingConfig(cfg)
	if err := s.Reload(bad); err == nil {
		t.Fatal("the late-failing reload must fail")
	}
	if err := s.CheckConfig(bad, model.ErrorPagesConfig{}); err == nil {
		t.Fatal("CheckConfig must report the same build failure")
	}
	if err := s.CheckConfig(cfg, model.ErrorPagesConfig{}); err != nil {
		t.Fatalf("CheckConfig of the live config: %v", err)
	}
	if s.current() != live {
		t.Fatal("a failed reload or a compile check must not replace the live router")
	}
	if !watchedAnchor(liveAnchor) {
		t.Fatal("a failed reload or a compile check must not detach the live anchors from the CRL watch")
	}

	// The operator revokes serial 5 and the watch tick runs.
	if err := os.WriteFile(crlPath, makeCRL(t, caCert, caKey, time.Now().Add(24*time.Hour), 5), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(crlPath, future, future); err != nil {
		t.Fatal(err)
	}
	for _, a := range *crlAnchors.Load() {
		a.reloadIfChanged()
	}
	if err := liveAnchor.verifyPeer([][]byte{leaf.Raw}, chains); err == nil {
		t.Fatal("the refreshed CRL must revoke the certificate on the live host")
	}
}

// TestCheckConfigReportsSettingsErrorPages proves the compile check covers the
// settings-level error pages a reload compiles, without installing them.
func TestCheckConfigReportsSettingsErrorPages(t *testing.T) {
	s := New(Config{CertDir: t.TempDir()})
	before := currentErrorPages()
	if err := s.CheckConfig(model.Config{}, model.ErrorPagesConfig{Dir: "no-such-dir"}); err == nil {
		t.Fatal("an unreadable settings errorPages.dir must fail the compile check")
	}
	if currentErrorPages() != before {
		t.Fatal("the compile check must not install error pages")
	}
}

// TestStoreRefusesObjectsTheDataPlaneCannotBuild wires the store to the real
// dry-run build, as main does, and proves an object that passes validation but
// does not compile is refused before it is committed.
func TestStoreRefusesObjectsTheDataPlaneCannotBuild(t *testing.T) {
	upstream := model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9}
	cases := []model.Object{
		model.Certificate{ObjectMeta: model.ObjectMeta{Name: "unused-cert"}, Type: model.CertTypeCustom,
			Domains: []string{"nothing.example.com"}, Custom: &model.CustomCertSpec{CertFile: "missing.pem", KeyFile: "missing.key"}},
		model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "side-host"}, Domains: []string{"side.example.com"},
			Upstream: upstream, ErrorPages: &model.ErrorPagesConfig{Dir: "no-such-dir"}},
	}
	for _, obj := range cases {
		dir := t.TempDir()
		ctx := context.Background()
		st := store.New(dir, store.NewExecGit(dir))
		if err := st.Init(ctx); err != nil {
			t.Fatal(err)
		}
		good := model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "good"}, Domains: []string{"good.example.com"}, Upstream: upstream}
		if _, err := st.Save(ctx, good, store.Author{}); err != nil {
			t.Fatal(err)
		}
		s := New(Config{CertDir: dir})
		st.SetCompileCheck(func(c model.Config, set model.Settings) error { return s.CheckConfig(c, set.ErrorPages) })
		if _, err := st.Save(ctx, obj, store.Author{}); !errors.Is(err, store.ErrNotCompilable) {
			t.Fatalf("%s %q: err = %v, want store.ErrNotCompilable", obj.Kind(), obj.GetMeta().Name, err)
		}
		cfg, _, err := st.Load(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.CheckConfig(cfg, model.ErrorPagesConfig{}); err != nil {
			t.Fatalf("%s %q: the stored config must still build: %v", obj.Kind(), obj.GetMeta().Name, err)
		}
	}
}

// TestDomainKeyMatchesRouterKey pins the store's duplicate-domain key to the
// router's map key, so two domains the router serves as one name are always
// seen as one name at write time.
func TestDomainKeyMatchesRouterKey(t *testing.T) {
	for _, d := range []string{
		"app.example.com", "APP.example.com", " app.example.com ", "app.example.com:8443",
		"app.example.com:x", "app.example.com.", "[2001:db8::1]:443", "", ":443",
	} {
		if got, want := model.DomainKey(d), hostKey(d); got != want {
			t.Errorf("model.DomainKey(%q) = %q, router hostKey = %q", d, got, want)
		}
	}
}
