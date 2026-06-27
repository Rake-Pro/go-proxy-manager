package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

const plaintextPassword = "s3cr3t-pass"

// buildNPMDB creates a synthetic NPM data dir (sqlite DB + cert files) and
// returns the dir path.
func buildNPMDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.sqlite")

	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE certificate (
			id INTEGER PRIMARY KEY, provider TEXT, nice_name TEXT,
			domain_names TEXT, expires_on TEXT, meta TEXT, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE access_list (
			id INTEGER PRIMARY KEY, name TEXT, satisfy_any INTEGER, pass_auth INTEGER, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE access_list_auth (
			id INTEGER PRIMARY KEY, access_list_id INTEGER, username TEXT, password TEXT, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE access_list_client (
			id INTEGER PRIMARY KEY, access_list_id INTEGER, address TEXT, directive TEXT, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE proxy_host (
			id INTEGER PRIMARY KEY, domain_names TEXT, forward_scheme TEXT, forward_host TEXT, forward_port INTEGER,
			access_list_id INTEGER, certificate_id INTEGER, ssl_forced INTEGER, http2_support INTEGER,
			hsts_enabled INTEGER, hsts_subdomains INTEGER, allow_websocket_upgrade INTEGER, enabled INTEGER,
			locations TEXT, advanced_config TEXT, block_exploits INTEGER, caching_enabled INTEGER, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE redirection_host (
			id INTEGER PRIMARY KEY, domain_names TEXT, forward_http_code INTEGER, forward_scheme TEXT,
			forward_domain_name TEXT, preserve_path INTEGER, certificate_id INTEGER, ssl_forced INTEGER,
			hsts_enabled INTEGER, hsts_subdomains INTEGER, enabled INTEGER, advanced_config TEXT, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE dead_host (
			id INTEGER PRIMARY KEY, domain_names TEXT, certificate_id INTEGER, ssl_forced INTEGER,
			hsts_enabled INTEGER, hsts_subdomains INTEGER, enabled INTEGER, advanced_config TEXT, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE stream (
			id INTEGER PRIMARY KEY, incoming_port INTEGER, forwarding_host TEXT, forwarding_port INTEGER,
			tcp_forwarding INTEGER, udp_forwarding INTEGER, enabled INTEGER, is_deleted INTEGER DEFAULT 0)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("create: %v\n%s", err, q)
		}
	}

	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("insert: %v\n%s", err, q)
		}
	}

	// Cert 1: letsencrypt, files present (created below). Cert 2: other, missing.
	exec(`INSERT INTO certificate (id, provider, nice_name, domain_names, meta) VALUES
		(1,'letsencrypt','App Cert','["app2.example.com"]','{"letsencrypt_email":"admin@example.com"}')`)
	exec(`INSERT INTO certificate (id, provider, nice_name, domain_names, meta) VALUES
		(2,'other','Legacy Cert','["legacy.example.com"]','{}')`)

	exec(`INSERT INTO access_list (id, name, satisfy_any, pass_auth) VALUES (1,'Office',1,1)`)
	exec(`INSERT INTO access_list_auth (id, access_list_id, username, password) VALUES (1,1,'admin',?)`, plaintextPassword)
	exec(`INSERT INTO access_list_client (id, access_list_id, address, directive) VALUES (1,1,'10.0.0.0/8','allow')`)
	exec(`INSERT INTO access_list_client (id, access_list_id, address, directive) VALUES (2,1,'1.2.3.4','deny')`)

	// Proxy host 1: full-featured, references cert 1 + access list 1, a location + advanced_config.
	exec(`INSERT INTO proxy_host (id, domain_names, forward_scheme, forward_host, forward_port,
		access_list_id, certificate_id, ssl_forced, http2_support, hsts_enabled, hsts_subdomains,
		allow_websocket_upgrade, enabled, locations, advanced_config, block_exploits, caching_enabled) VALUES
		(1,'["app2.example.com"]','http','backend.local',8080,1,1,1,1,1,1,1,1,
		'[{"path":"/api","forward_scheme":"http","forward_host":"api.local","forward_port":9000,"advanced_config":""}]',
		'add_header X-Foo bar;',1,1)`)

	// Proxy host 2: minimal, disabled, references cert 2 (missing files).
	exec(`INSERT INTO proxy_host (id, domain_names, forward_scheme, forward_host, forward_port,
		access_list_id, certificate_id, ssl_forced, http2_support, hsts_enabled, hsts_subdomains,
		allow_websocket_upgrade, enabled, locations, advanced_config, block_exploits, caching_enabled) VALUES
		(2,'["old.example.com"]','http','127.0.0.1',3000,0,2,0,0,0,0,0,0,'','',0,0)`)

	exec(`INSERT INTO redirection_host (id, domain_names, forward_http_code, forward_scheme,
		forward_domain_name, preserve_path, certificate_id, ssl_forced, hsts_enabled, hsts_subdomains, enabled, advanced_config) VALUES
		(1,'["www.example.com"]',301,'auto','example.com',1,0,1,0,0,1,'')`)

	exec(`INSERT INTO dead_host (id, domain_names, certificate_id, ssl_forced, hsts_enabled, hsts_subdomains, enabled, advanced_config) VALUES
		(1,'["dead.example.com"]',0,0,0,0,1,'')`)

	exec(`INSERT INTO stream (id, incoming_port, forwarding_host, forwarding_port, tcp_forwarding, udp_forwarding, enabled) VALUES
		(1,5353,'dns.local',53,1,1,1)`)

	// Cert 1 PEM files present under custom_ssl/npm-1.
	certDir := filepath.Join(dir, "custom_ssl", "npm-1")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("mkdir cert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("FULLCHAIN"), 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("PRIVKEY"), 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return dir
}

func TestImport(t *testing.T) {
	dir := buildNPMDB(t)

	res, err := Import(context.Background(), dir)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Summary counts.
	wantCounts := map[string]int{
		"ProxyHost":    2,
		"RedirectHost": 1,
		"DeadHost":     1,
		"StreamHost":   1,
		"AccessList":   1,
		"Certificate":  2,
	}
	for k, v := range wantCounts {
		if res.Summary[k] != v {
			t.Errorf("Summary[%s] = %d, want %d", k, res.Summary[k], v)
		}
	}

	objs := indexObjects(res.Objects)

	// Proxy host 1 mapping.
	ph1 := findProxy(t, res.Objects, "app2.example.com")
	if got := ph1.Upstream; got.Scheme != "http" || got.Host != "backend.local" || got.Port != 8080 {
		t.Errorf("ph1 upstream = %+v", got)
	}
	if !ph1.WebsocketsUpgrade {
		t.Error("ph1 websockets upgrade should be true")
	}
	if !ph1.TLS.ForceSSL || !ph1.TLS.HTTP2 || !ph1.TLS.HSTS.Enabled || !ph1.TLS.HSTS.IncludeSubdomains {
		t.Errorf("ph1 TLS flags = %+v", ph1.TLS)
	}
	if ph1.TLS.CertificateRef == "" {
		t.Error("ph1 should reference a certificate")
	}
	if _, ok := objs["Certificate/"+ph1.TLS.CertificateRef]; !ok {
		t.Errorf("ph1 cert ref %q does not resolve to a Certificate object", ph1.TLS.CertificateRef)
	}
	if len(ph1.AccessLists) != 1 {
		t.Errorf("ph1 access lists = %v", ph1.AccessLists)
	} else if _, ok := objs["AccessList/"+ph1.AccessLists[0]]; !ok {
		t.Errorf("ph1 access list %q does not resolve", ph1.AccessLists[0])
	}
	if len(ph1.Locations) != 1 || ph1.Locations[0].Path != "/api" {
		t.Errorf("ph1 locations = %+v", ph1.Locations)
	} else if u := ph1.Locations[0].Upstream; u == nil || u.Host != "api.local" || u.Port != 9000 {
		t.Errorf("ph1 location upstream = %+v", ph1.Locations[0].Upstream)
	}
	if err := model.ValidateName(ph1.Name); err != nil {
		t.Errorf("ph1 name invalid: %v", err)
	}

	// Proxy host 2: disabled, cert files missing -> ref dropped.
	ph2 := findProxy(t, res.Objects, "old.example.com")
	if !ph2.Disabled {
		t.Error("ph2 should be Disabled")
	}
	if ph2.TLS.CertificateRef != "" {
		t.Errorf("ph2 cert ref should be dropped (missing files), got %q", ph2.TLS.CertificateRef)
	}

	// Access list: bcrypt of plaintext + allow/deny rules.
	al := findAccessList(t, res.Objects)
	if !al.SatisfyAny {
		t.Error("access list SatisfyAny should be true")
	}
	if len(al.BasicAuth) != 1 {
		t.Fatalf("access list basic auth = %+v", al.BasicAuth)
	}
	if al.BasicAuth[0].Username != "admin" {
		t.Errorf("auth username = %q", al.BasicAuth[0].Username)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(al.BasicAuth[0].PasswordHash), []byte(plaintextPassword)); err != nil {
		t.Errorf("password hash does not verify against plaintext: %v", err)
	}
	if len(al.Rules) != 2 {
		t.Fatalf("access list rules = %+v", al.Rules)
	}
	if al.Rules[0].Action != "allow" || al.Rules[0].CIDR != "10.0.0.0/8" {
		t.Errorf("rule 0 = %+v", al.Rules[0])
	}
	if al.Rules[1].Action != "deny" || al.Rules[1].CIDR != "1.2.3.4" {
		t.Errorf("rule 1 = %+v", al.Rules[1])
	}

	// Redirect host: auto scheme -> "".
	rh := findRedirect(t, res.Objects)
	if rh.TargetScheme != "" {
		t.Errorf("redirect target scheme should be empty (auto), got %q", rh.TargetScheme)
	}
	if rh.TargetDomain != "example.com" || rh.StatusCode != 301 || !rh.PreservePath {
		t.Errorf("redirect host = %+v", rh)
	}

	// Stream: tcp+udp -> both.
	sh := findStream(t, res.Objects)
	if sh.Protocol != "both" || sh.ListenPort != 5353 || sh.ForwardHost != "dns.local" || sh.ForwardPort != 53 {
		t.Errorf("stream host = %+v", sh)
	}

	// Cert copies: only cert 1 (files present).
	if len(res.Certs) != 1 {
		t.Fatalf("CertCopies = %+v", res.Certs)
	}
	if !strings.HasSuffix(res.Certs[0].CertPEM, filepath.Join("custom_ssl", "npm-1", "fullchain.pem")) {
		t.Errorf("cert copy CertPEM = %q", res.Certs[0].CertPEM)
	}

	// Warnings: required categories present.
	assertWarning(t, res, "reconfigure as an ACME")      // LE cert
	assertWarning(t, res, "certificate files not found") // cert 2 missing files
	assertWarning(t, res, "reference dropped")           // ph2 dropped ref
	assertWarning(t, res, "raw nginx advanced config")   // ph1 advanced_config
	assertWarning(t, res, "block_exploits")              // ph1 block_exploits
	assertWarning(t, res, "caching not supported")       // ph1 caching

	// Every produced object must validate.
	for _, o := range res.Objects {
		if err := o.Validate(); err != nil {
			t.Errorf("object %s/%s failed validation: %v", o.Kind(), o.GetMeta().Name, err)
		}
	}
}

// buildSecurityDB creates a minimal NPM data dir with the access_list,
// access_list_client and proxy_host tables and lets the caller seed rows.
func buildSecurityDB(t *testing.T, setup func(exec func(string, ...any))) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.sqlite")

	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE certificate (
			id INTEGER PRIMARY KEY, provider TEXT, nice_name TEXT,
			domain_names TEXT, expires_on TEXT, meta TEXT, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE access_list (
			id INTEGER PRIMARY KEY, name TEXT, satisfy_any INTEGER, pass_auth INTEGER, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE access_list_auth (
			id INTEGER PRIMARY KEY, access_list_id INTEGER, username TEXT, password TEXT, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE access_list_client (
			id INTEGER PRIMARY KEY, access_list_id INTEGER, address TEXT, directive TEXT, is_deleted INTEGER DEFAULT 0)`,
		`CREATE TABLE proxy_host (
			id INTEGER PRIMARY KEY, domain_names TEXT, forward_scheme TEXT, forward_host TEXT, forward_port INTEGER,
			access_list_id INTEGER, enabled INTEGER, is_deleted INTEGER DEFAULT 0)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("create: %v\n%s", err, q)
		}
	}

	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("insert: %v\n%s", err, q)
		}
	}
	setup(exec)
	return dir
}

func TestImportAccessListSecurity(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(exec func(string, ...any))
		verify func(t *testing.T, res *Result)
	}{
		{
			name: "malformed cidr drops only that rule and keeps host protected",
			setup: func(exec func(string, ...any)) {
				exec(`INSERT INTO access_list (id, name, satisfy_any, pass_auth) VALUES (1,'Office',0,1)`)
				exec(`INSERT INTO access_list_client (id, access_list_id, address, directive) VALUES (1,1,'10.0.0.0/8','allow')`)
				exec(`INSERT INTO access_list_client (id, access_list_id, address, directive) VALUES (2,1,'not-a-cidr','allow')`)
				exec(`INSERT INTO access_list_client (id, access_list_id, address, directive) VALUES (3,1,'1.2.3.4','deny')`)
				exec(`INSERT INTO proxy_host (id, domain_names, forward_scheme, forward_host, forward_port, access_list_id, enabled) VALUES
					(1,'["a.example.com"]','http','backend.local',8080,1,1)`)
			},
			verify: func(t *testing.T, res *Result) {
				if res.Summary["AccessList"] != 1 {
					t.Fatalf("AccessList count = %d, want 1 (list must survive one bad rule)", res.Summary["AccessList"])
				}
				al := findAccessList(t, res.Objects)
				if len(al.Rules) != 2 {
					t.Fatalf("rules = %+v, want only the 2 valid rules", al.Rules)
				}
				ph := findProxy(t, res.Objects, "a.example.com")
				if len(ph.AccessLists) != 1 || ph.AccessLists[0] != al.Name {
					t.Fatalf("host access lists = %v, want [%s] (host stays protected)", ph.AccessLists, al.Name)
				}
				assertWarning(t, res, `invalid cidr/ip "not-a-cidr"`)
			},
		},
		{
			name: "missing access list locks host deny-all instead of going public",
			setup: func(exec func(string, ...any)) {
				exec(`INSERT INTO proxy_host (id, domain_names, forward_scheme, forward_host, forward_port, access_list_id, enabled) VALUES
					(1,'["b.example.com"]','http','backend.local',8080,99,1)`)
			},
			verify: func(t *testing.T, res *Result) {
				ph := findProxy(t, res.Objects, "b.example.com")
				if len(ph.AccessLists) != 1 {
					t.Fatalf("host access lists = %v, want a single lock list (must NOT be public)", ph.AccessLists)
				}
				lock := findAccessListByName(t, res.Objects, ph.AccessLists[0])
				if lock.DefaultAction != model.ActionDeny {
					t.Errorf("lock DefaultAction = %q, want deny", lock.DefaultAction)
				}
				if len(lock.Rules) != 1 || lock.Rules[0].Action != model.ActionDeny || lock.Rules[0].CIDR != "0.0.0.0/0" {
					t.Errorf("lock rules = %+v, want a single deny 0.0.0.0/0", lock.Rules)
				}
				assertWarning(t, res, "LOCKED")
			},
		},
		{
			name: "access list with only a malformed rule becomes deny-all not empty",
			setup: func(exec func(string, ...any)) {
				exec(`INSERT INTO access_list (id, name, satisfy_any, pass_auth) VALUES (1,'Office',0,1)`)
				exec(`INSERT INTO access_list_client (id, access_list_id, address, directive) VALUES (1,1,'not-a-cidr','allow')`)
			},
			verify: func(t *testing.T, res *Result) {
				al := findAccessList(t, res.Objects)
				if len(al.Rules) != 0 || len(al.BasicAuth) != 0 {
					t.Fatalf("list should have lost all rules/auth, got rules=%+v auth=%+v", al.Rules, al.BasicAuth)
				}
				if al.DefaultAction != model.ActionDeny {
					t.Errorf("DefaultAction = %q, want deny", al.DefaultAction)
				}
				assertWarning(t, res, "forced to deny-all")
			},
		},
		{
			name: "cert with missing pem files is emitted disabled",
			setup: func(exec func(string, ...any)) {
				exec(`INSERT INTO certificate (id, provider, nice_name, domain_names, meta) VALUES
					(1,'other','Legacy Cert','["legacy.example.com"]','{}')`)
			},
			verify: func(t *testing.T, res *Result) {
				cert := findCert(t, res.Objects)
				if !cert.Disabled {
					t.Error("cert with missing files should be Disabled")
				}
				if len(res.Certs) != 0 {
					t.Errorf("no cert copies expected for missing files, got %+v", res.Certs)
				}
				assertWarning(t, res, "certificate files not found")
			},
		},
		{
			name: "non-bcrypt dollar-prefixed password warns",
			setup: func(exec func(string, ...any)) {
				exec(`INSERT INTO access_list (id, name, satisfy_any, pass_auth) VALUES (1,'Office',0,1)`)
				exec(`INSERT INTO access_list_auth (id, access_list_id, username, password) VALUES (1,1,'admin','$apr1$abc$def')`)
			},
			verify: func(t *testing.T, res *Result) {
				al := findAccessList(t, res.Objects)
				if len(al.BasicAuth) != 1 || al.BasicAuth[0].Username != "admin" {
					t.Fatalf("basic auth = %+v", al.BasicAuth)
				}
				assertWarning(t, res, "treated as a literal password")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := buildSecurityDB(t, tc.setup)
			res, err := Import(context.Background(), dir)
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			for _, o := range res.Objects {
				if err := o.Validate(); err != nil {
					t.Errorf("object %s/%s failed validation: %v", o.Kind(), o.GetMeta().Name, err)
				}
			}
			tc.verify(t, res)
		})
	}
}

func findAccessListByName(t *testing.T, objs []model.Object, name string) model.AccessList {
	t.Helper()
	for _, o := range objs {
		if al, ok := o.(model.AccessList); ok && al.Name == name {
			return al
		}
	}
	t.Fatalf("access list %q not found", name)
	return model.AccessList{}
}

func TestImportNoDB(t *testing.T) {
	if _, err := Import(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected error when no sqlite DB present")
	}
}

func indexObjects(objs []model.Object) map[string]model.Object {
	m := map[string]model.Object{}
	for _, o := range objs {
		m[o.Kind()+"/"+o.GetMeta().Name] = o
	}
	return m
}

func findProxy(t *testing.T, objs []model.Object, domain string) model.ProxyHost {
	t.Helper()
	for _, o := range objs {
		if ph, ok := o.(model.ProxyHost); ok {
			for _, d := range ph.Domains {
				if d == domain {
					return ph
				}
			}
		}
	}
	t.Fatalf("proxy host with domain %q not found", domain)
	return model.ProxyHost{}
}

func findAccessList(t *testing.T, objs []model.Object) model.AccessList {
	t.Helper()
	for _, o := range objs {
		if al, ok := o.(model.AccessList); ok {
			return al
		}
	}
	t.Fatal("access list not found")
	return model.AccessList{}
}

func findCert(t *testing.T, objs []model.Object) model.Certificate {
	t.Helper()
	for _, o := range objs {
		if c, ok := o.(model.Certificate); ok {
			return c
		}
	}
	t.Fatal("certificate not found")
	return model.Certificate{}
}

func findRedirect(t *testing.T, objs []model.Object) model.RedirectHost {
	t.Helper()
	for _, o := range objs {
		if rh, ok := o.(model.RedirectHost); ok {
			return rh
		}
	}
	t.Fatal("redirect host not found")
	return model.RedirectHost{}
}

func findStream(t *testing.T, objs []model.Object) model.StreamHost {
	t.Helper()
	for _, o := range objs {
		if sh, ok := o.(model.StreamHost); ok {
			return sh
		}
	}
	t.Fatal("stream host not found")
	return model.StreamHost{}
}

func assertWarning(t *testing.T, res *Result, substr string) {
	t.Helper()
	for _, w := range res.Warnings {
		if strings.Contains(w.Reason, substr) {
			return
		}
	}
	t.Errorf("expected a warning containing %q; got %+v", substr, res.Warnings)
}
