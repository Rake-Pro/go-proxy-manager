package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
	"github.com/Rake-Pro/go-proxy-manager/internal/clientip"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/Rake-Pro/go-proxy-manager/internal/session"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// A request that never went through RequireRole carries no principal. api.Deps
// treats a nil RequireScope as "allow" (the unwired/test case), so the daemon's
// implementation is the only thing between such a request and every route: it
// has to fail CLOSED.
func TestRequireScopeDeniesWithoutPrincipal(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/proxy-hosts", nil)
	for _, required := range []string{"proxy-hosts:read", "*:read", model.ScopeAdmin} {
		if err := requireScope(r, required); err == nil {
			t.Fatalf("requireScope(%q) with no principal must deny", required)
		}
	}
}

// With a real principal attached by the real middleware, the gate behaves as
// documented: sessions are unconstrained, tokens are held to their scopes.
func TestRequireScopeWithPrincipal(t *testing.T) {
	sessStore, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sessStore.Close() })

	secret, hash, err := auth.NewTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	authn := auth.NewAuthenticator(auth.Options{Store: sessStore})
	authn.SetTokenSource(func() []model.APIToken {
		return []model.APIToken{{
			ObjectMeta: model.ObjectMeta{Name: "ci"},
			TokenHash:  hash,
			Scopes:     []string{"proxy-hosts:write"},
		}}
	})

	tests := []struct {
		name     string
		required string
		wantErr  bool
	}{
		{"granted scope passes", "proxy-hosts:read", false},
		{"write implies read", "proxy-hosts:write", false},
		{"other resource is refused", "certificates:read", true},
		{"admin endpoints are refused", model.ScopeAdmin, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got error
			var reached bool
			h := authn.RequireRole(auth.RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				got = requireScope(r, tc.required)
			}))
			r := httptest.NewRequest("GET", "/api/proxy-hosts", nil)
			r.Header.Set("Authorization", "Bearer "+secret)
			h.ServeHTTP(httptest.NewRecorder(), r)
			if !reached {
				t.Fatal("the token did not authenticate")
			}
			if (got != nil) != tc.wantErr {
				t.Fatalf("requireScope(%q) = %v, wantErr %v", tc.required, got, tc.wantErr)
			}
		})
	}
}

// A reconcile can be driven by an HTTP request, and the client hanging up cancels
// that request's context. SaveDNSLedger writes the ledger file and THEN commits
// it, so a cancellation reaching git leaves the file on disk and out of history,
// to be swept into whatever unrelated commit lands next. The adapter therefore
// detaches the write from the caller's context.
func TestDNSLedgerSaveCommitsDespiteACancelledContext(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir, store.NewExecGit(dir))
	if err := st.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	head, err := st.Head(context.Background())
	if err != nil {
		t.Fatalf("head: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the caller has already gone away
	adapter := dnsLedgerStore{st: st}
	if err := adapter.Save(ctx, model.DNSLedger{
		Pihole: []model.DNSLedgerEntry{{Domain: "app.example.com", Target: "edge.example.com"}},
	}, ""); err != nil {
		t.Fatalf("save with a cancelled context: %v", err)
	}

	after, err := st.Head(context.Background())
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if after == head {
		t.Fatal("REGRESSION: the ledger was written but never committed, leaving it to be swept into an unrelated commit")
	}
	if _, err := os.Stat(filepath.Join(dir, "dns-ledger.yaml")); err != nil {
		t.Fatalf("ledger file: %v", err)
	}
}

// A set but unreadable <KEY>_FILE is an error, never an empty value, and an
// unreadable TOTP secret refuses local login rather than dropping to the
// password alone.
func TestUnreadableSecretFileFailsClosed(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	t.Run("totp secret file", func(t *testing.T) {
		t.Setenv("GPM_LOCAL_ADMIN_PASSWORD_HASH", "$2a$10$example")
		t.Setenv("GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE", "")
		t.Setenv("GPM_LOCAL_ADMIN_TOTP_SECRET_FILE", missing)
		if _, err := secretFromEnv("GPM_LOCAL_ADMIN_TOTP_SECRET"); err == nil {
			t.Fatal("an unreadable _FILE must be reported as an error")
		}
		hash, totp := localAdminSecrets()
		if hash != "" || totp != "" {
			t.Fatalf("localAdminSecrets() = (%q, %q), want both empty so local login is refused", hash, totp)
		}
		st, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.Close() })
		authn := auth.NewAuthenticator(auth.Options{Store: st, LocalUser: "admin", LocalHash: hash, LocalTOTPSecret: totp})
		authn.Configure(model.Config{}, model.Settings{AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true}})
		if _, _, err := authn.LocalLogin(context.Background(), "admin", "pw"); err == nil {
			t.Fatal("local login must be refused while the TOTP secret file is unreadable")
		}
	})

	t.Run("password hash file", func(t *testing.T) {
		t.Setenv("GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE", missing)
		t.Setenv("GPM_LOCAL_ADMIN_TOTP_SECRET_FILE", "")
		t.Setenv("GPM_LOCAL_ADMIN_TOTP_SECRET", "")
		if hash, _ := localAdminSecrets(); hash != "" {
			t.Fatalf("hash = %q, want empty", hash)
		}
	})
}

// A named secret file that holds only whitespace is an error like an unreadable
// one, so an empty TOTP file refuses local login instead of dropping the second
// factor.
func TestEmptySecretFileFailsClosed(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GPM_LOCAL_ADMIN_PASSWORD_HASH", "$2a$10$example")
	t.Setenv("GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE", "")
	t.Setenv("GPM_LOCAL_ADMIN_TOTP_SECRET_FILE", empty)
	if _, err := secretFromEnv("GPM_LOCAL_ADMIN_TOTP_SECRET"); err == nil {
		t.Fatal("an empty _FILE must be reported as an error")
	}
	if hash, totp := localAdminSecrets(); hash != "" || totp != "" {
		t.Fatalf("localAdminSecrets() = (%q, %q), want both empty so local login is refused", hash, totp)
	}

	t.Setenv("GPM_LOCAL_ADMIN_TOTP_SECRET_FILE", "")
	t.Setenv("GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE", empty)
	if hash, _ := localAdminSecrets(); hash != "" {
		t.Fatalf("an empty hash file must leave no usable hash, got %q", hash)
	}
}

// The startup warning fires only for an enabled admin edge whose loopback hop
// is not a trusted proxy.
func TestAdminEdgeHopUntrusted(t *testing.T) {
	t.Cleanup(func() { clientip.SetTrusted(nil) })
	on := model.Settings{AdminEdge: model.AdminEdgeSettings{Enabled: true}}

	clientip.SetTrusted(nil)
	if !adminEdgeHopUntrusted(on, ":8081") {
		t.Fatal("enabled edge with no trusted proxies must be reported")
	}
	if adminEdgeHopUntrusted(model.Settings{}, ":8081") {
		t.Fatal("a disabled edge must not be reported")
	}
	clientip.SetTrusted([]string{"127.0.0.1/32", "::1/128"})
	if adminEdgeHopUntrusted(on, ":8081") {
		t.Fatal("a trusted loopback hop must not be reported")
	}
}

// GPM_SSO_SIGNING_KEY_FILE follows the other _FILE secrets: a named file that
// cannot be read or holds only whitespace is a startup error, never a silent
// fall back to a generated key; a readable file wins over the plain variable;
// and with neither set an empty key is handed over, which leaves the data
// plane's own resolution in charge.
func TestSSOSigningKeyFileFailsStartup(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	good := filepath.Join(dir, "key")
	if err := os.WriteFile(empty, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good, []byte("k3y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file, plain string
		wantErr     bool
		want        string
	}{
		{"", "", false, ""},
		{"", "pl4in", false, "pl4in"},
		{filepath.Join(dir, "missing"), "pl4in", true, ""},
		{empty, "pl4in", true, ""},
		{good, "", false, "k3y"},
		{good, "pl4in", false, "k3y"},
	} {
		t.Setenv("GPM_SSO_SIGNING_KEY_FILE", tc.file)
		t.Setenv("GPM_SSO_SIGNING_KEY", tc.plain)
		got, called := "", false
		err := applySSOSigningKey(func(k string) { got, called = k, true })
		if (err != nil) != tc.wantErr {
			t.Fatalf("file=%q plain=%q: err = %v, wantErr %v", tc.file, tc.plain, err, tc.wantErr)
		}
		if err != nil {
			if !strings.Contains(err.Error(), "GPM_SSO_SIGNING_KEY_FILE") {
				t.Fatalf("error %q does not name GPM_SSO_SIGNING_KEY_FILE", err)
			}
			if called {
				t.Fatalf("file=%q: a key was handed over despite the error", tc.file)
			}
			continue
		}
		if !called || got != tc.want {
			t.Fatalf("file=%q plain=%q: handed over %q (called %v), want %q", tc.file, tc.plain, got, called, tc.want)
		}
	}
}

// main stops at startup, with the SSO key message, when the key file is
// unreadable. The child runs main itself; -ha-role is invalid so that a main
// which did not check the key fails at the next fatal check instead of
// starting listeners.
func TestMainRefusesUnreadableSSOKeyFile(t *testing.T) {
	if os.Getenv("GPM_TEST_RUN_MAIN") == "1" {
		os.Args = []string{"gpm", "-ha-role", "not-a-role"}
		main()
		return
	}
	for _, tc := range []struct {
		file, want string
	}{
		{filepath.Join(t.TempDir(), "missing"), "SSO signing key file is unreadable or empty"},
		{"", "invalid HA role"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMainRefusesUnreadableSSOKeyFile$")
		// Only the GPM_ variables this test sets: the caller's own (a log
		// level, a TOTP secret, a _FILE naming a FIFO) must not decide it.
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "GPM_") {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		cmd.Env = append(cmd.Env, "GPM_TEST_RUN_MAIN=1", "GPM_SSO_SIGNING_KEY_FILE="+tc.file)
		out, err := cmd.CombinedOutput()
		cancel()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() == 0 {
			t.Fatalf("file=%q: main did not exit with an error (err %v):\n%s", tc.file, err, out)
		}
		if !strings.Contains(string(out), tc.want) {
			t.Fatalf("file=%q: output lacks %q:\n%s", tc.file, tc.want, out)
		}
		if tc.file != "" && strings.Contains(string(out), "invalid HA role") {
			t.Fatalf("main went past the SSO key check:\n%s", out)
		}
	}
}
