package analytics

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	browserUA = "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101"
	promUA    = "Prometheus/2.53.0"
)

type sigCase struct {
	method, path, query, ua, want string
}

var sigPositive = []sigCase{
	// proto-abuse
	{"CONNECT", "/", "", browserUA, sigProtoAbuse},
	{"connect", "/", "", "", sigProtoAbuse},
	{"TRACE", "/", "", browserUA, sigProtoAbuse},
	{"TRACE", "/", "", "zgrab/0.x", sigProtoAbuse}, // proto-abuse is evaluated before scanner-ua
	{"GET", "garbage", "", "", sigProtoAbuse},
	{"GET", "", "", "", sigProtoAbuse},
	{"GET", "\x16\x03\x01\x02", "", "", sigProtoAbuse},

	// scanner-ua
	{"GET", "/", "", "Mozilla/5.0 zgrab/0.x", sigScannerUA},
	{"GET", "/robots.txt", "", "masscan/1.3", sigScannerUA},
	{"GET", "/", "", "Mozilla/5.0 (compatible; Nmap Scripting Engine)", sigScannerUA},
	{"GET", "/index.html", "", "NIKTO/2.5", sigScannerUA},
	{"GET", "/api", "", "Nuclei/3.2", sigScannerUA},
	{"GET", "/item", "id=1", "sqlmap/1.8", sigScannerUA},
	{"GET", "/", "", "WPScan v3.8", sigScannerUA},
	{"GET", "/x", "", "gobuster/3.6", sigScannerUA},
	{"GET", "/x", "", "feroxbuster/2.10", sigScannerUA},
	{"GET", "/x", "", "DirBuster-1.0", sigScannerUA},
	{"GET", "/", "", "Mozilla/5.0 (compatible; CensysInspect/1.1)", sigScannerUA},
	{"GET", "/", "", "internet-measurement/1.0", sigScannerUA},
	{"GET", "/", "", "Expanse scanner", sigScannerUA},
	{"GET", "/", "", "l9explore/1.2.2", sigScannerUA},
	{"GET", "/", "", "LeakIX-agent", sigScannerUA},
	{"GET", "/", "", "zmap", sigScannerUA},
	{"GET", "/", "", "httpx - projectdiscovery", sigScannerUA},
	{"GET", "/", "", "WhatWeb/0.5", sigScannerUA},
	{"GET", "/", "", "Netcraft survey agent", sigScannerUA},
	{"GET", "/.env", "", "zgrab/0.x", sigScannerUA}, // scanner-ua is evaluated before secret-probe

	// secret-probe
	{"GET", "/.env", "", browserUA, sigSecretProbe},
	{"GET", "/secrets", "", browserUA, sigSecretProbe},
	{"GET", "/secrets/db.yaml", "", browserUA, sigSecretProbe},
	{"GET", "/actuator/env", "", browserUA, sigAdminProbe},
	{"GET", "/x", "cmd=/bin/sh;id", browserUA, sigInjection},
	{"GET", "/x", "c=powershell+-enc+AAAA", browserUA, sigInjection},
	{"GET", "/.ENV", "", "", sigSecretProbe},
	{"GET", "/.env.production", "", "", sigSecretProbe},
	{"GET", "/.git", "", "", sigSecretProbe},
	{"GET", "/.git/config", "", "", sigSecretProbe},
	{"GET", "/.git/HEAD", "", "", sigSecretProbe},
	{"GET", "/.svn", "", "", sigSecretProbe},
	{"GET", "/.svn/entries", "", "", sigSecretProbe},
	{"GET", "/.hg/hgrc", "", "", sigSecretProbe},
	{"GET", "/.aws/credentials", "", "", sigSecretProbe},
	{"GET", "/.ssh/id_ed25519", "", "", sigSecretProbe},
	{"GET", "/.DS_Store", "", "", sigSecretProbe},
	{"GET", "/.npmrc", "", "", sigSecretProbe},
	{"GET", "/.htpasswd", "", "", sigSecretProbe},
	{"GET", "/.htaccess", "", "", sigSecretProbe},
	{"GET", "/id_rsa.pub", "", "", sigSecretProbe},
	{"GET", "/config.yml", "", "", sigSecretProbe},
	{"GET", "/config.yaml", "", "", sigSecretProbe},
	{"GET", "/secrets", "", "", sigSecretProbe},
	{"GET", "/.vscode/sftp.json", "", "", sigSecretProbe},
	{"GET", "/.idea/workspace.xml", "", "", sigSecretProbe},
	{"GET", "/backup.zip", "", "", sigSecretProbe},
	{"GET", "/backup.sql", "", "", sigSecretProbe},
	{"GET", "/db.sql", "", "", sigSecretProbe},
	{"GET", "/dump.sql", "", "", sigSecretProbe},
	{"GET", "/.bash_history", "", "", sigSecretProbe},
	{"GET", "/server.key", "", "", sigSecretProbe},
	{"GET", "/phpinfo.php", "", "", sigSecretProbe},
	{"GET", "/info.php", "", "", sigSecretProbe},
	{"GET", "/.env", "", "python-requests/2.32", sigSecretProbe}, // not bare root, so not scanner-ua

	// wp-probe
	{"GET", "/wp-login.php", "", browserUA, sigWPProbe},
	{"POST", "/xmlrpc.php", "", "", sigWPProbe},
	{"GET", "/wp-admin", "", "", sigWPProbe},
	{"GET", "/WP-ADMIN/setup-config.php", "", "", sigWPProbe},
	{"GET", "/wp-includes/js/jquery.js", "", "", sigWPProbe},
	{"GET", "/wp-content/plugins/some-plugin/readme.txt", "", "", sigWPProbe},
	{"GET", "/wp-content/uploads/2024/10/shell.php", "", "", sigWPProbe}, // wp-probe is evaluated before webshell
	{"GET", "/wp-json/wp/v2/users", "", "", sigWPProbe},
	{"GET", "/wp-json/wp/v2/users/1", "", "", sigWPProbe},
	{"GET", "/blog/wlwmanifest.xml", "", "", sigWPProbe},
	{"GET", "/", "author=1", browserUA, sigWPProbe},
	{"GET", "/", "AUTHOR=42", "", sigWPProbe},

	// admin-probe
	{"GET", "/phpmyadmin", "", "", sigAdminProbe},
	{"GET", "/phpMyAdmin-5.2/index.php", "", "", sigAdminProbe},
	{"GET", "/pma", "", "", sigAdminProbe},
	{"GET", "/pmd/index.php", "", "", sigAdminProbe},
	{"GET", "/myadmin/", "", "", sigAdminProbe},
	{"GET", "/mysql", "", "", sigAdminProbe},
	{"GET", "/adminer", "", "", sigAdminProbe},
	{"GET", "/adminer.php", "", "", sigAdminProbe},
	{"GET", "/actuator", "", "", sigAdminProbe},
	{"GET", "/actuator/env", "", "", sigAdminProbe},
	{"GET", "/server-status", "", "", sigAdminProbe},
	{"GET", "/server-info", "", "", sigAdminProbe},
	{"GET", "/manager/html", "", "", sigAdminProbe},
	{"GET", "/manager/text/list", "", "", sigAdminProbe},
	{"GET", "/solr/", "", "", sigAdminProbe},
	{"GET", "/jenkins/login", "", "", sigAdminProbe},
	{"GET", "/jmx-console/", "", "", sigAdminProbe},
	{"GET", "/web-console/", "", "", sigAdminProbe},
	{"POST", "/_ignition/execute-solution", "", "", sigAdminProbe},
	{"GET", "/telescope/requests", "", "", sigAdminProbe},
	{"GET", "/horizon", "", "", sigAdminProbe},
	{"GET", "/debug", "", "", sigAdminProbe},
	{"GET", "/debug/pprof/goroutine", "", "", sigAdminProbe},
	{"GET", "/api/v1/pods", "", "", sigAdminProbe},
	{"GET", "/.kube/config", "", "", sigAdminProbe},
	{"POST", "/autodiscover/autodiscover.xml", "", "", sigAdminProbe},
	{"GET", "/vendor/phpunit/", "", "", sigAdminProbe},
	{"POST", "/cgi-bin/php-cgi", "", "", sigAdminProbe}, // admin-probe is evaluated before iot-rce's /cgi-bin/

	// webshell
	{"GET", "/uploads/avatar.php", "", "", sigWebshell},
	{"POST", "/upload/file.php", "", "", sigWebshell},
	{"GET", "/tmp/run.phtml", "", "", sigWebshell},
	{"GET", "/temp/run.php", "", "", sigWebshell},
	{"GET", "/assets/uploads/x.phar", "", "", sigWebshell},
	{"GET", "/uploads/y.php7", "", "", sigWebshell},
	{"GET", "/shell.php", "", "", sigWebshell},
	{"GET", "/cmd.php", "", "", sigWebshell},
	{"GET", "/c99.php", "", "", sigWebshell},
	{"GET", "/r57.php", "", "", sigWebshell},
	{"GET", "/alfa.php", "", "", sigWebshell},
	{"GET", "/x.php", "", "", sigWebshell},
	{"GET", "/1.php", "", "", sigWebshell},
	{"GET", "/up.php", "", "", sigWebshell},
	{"GET", "/deep/dir/WSO.PHP", "", "", sigWebshell},
	{"GET", "/mini.php", "", "", sigWebshell},
	{"GET", "/bypass.php", "", "", sigWebshell},
	{"GET", "/exploit.php", "", "", sigWebshell},
	{"GET", "/shell.jsp", "", "", sigWebshell},
	{"GET", "/cmd.jsp", "", "", sigWebshell},
	{"GET", "/cmd.aspx", "", "", sigWebshell},
	{"GET", "/shell.aspx", "", "", sigWebshell},

	// iot-rce
	{"GET", "/cgi-bin/luci", "", "", sigIoTRCE},
	{"GET", "/cgi-bin/config.exp", "", "", sigIoTRCE},
	{"GET", "/;stok=/locale", "form=country", "", sigIoTRCE},
	{"POST", "/boaform/admin/formLogin", "", "", sigIoTRCE},
	{"POST", "/GponForm/diag_Form", "images/", "", sigIoTRCE},
	{"POST", "/HNAP1/", "", "", sigIoTRCE},
	{"POST", "/HNAP1", "", "", sigIoTRCE},
	{"GET", "/setup.cgi", "next_file=netgear.cfg", "", sigIoTRCE},
	{"GET", "/picsdesc.xml", "", "", sigIoTRCE},
	{"POST", "/goform/set_LimitClient_cfg", "", "", sigIoTRCE},
	{"POST", "/tmUnblock.cgi", "", "", sigIoTRCE},
	{"POST", "/hndUnblock.cgi", "", "", sigIoTRCE},
	{"GET", "/device.rsp", "opt=sys&cmd=___S_O_S_T_R_E_A_MAX___", "", sigIoTRCE},
	{"GET", "/dvr/cmd", "", "", sigIoTRCE},
	{"POST", "/ws/v1/cluster/apps/new-application", "", "", sigIoTRCE},
	{"POST", "/manager/radius/server-ping", "", "", sigIoTRCE},
	{"GET", "/Autodiscover/Autodiscover.json", "", "", sigIoTRCE},
	{"POST", "/api/jsonws/invoke", "", "", sigIoTRCE},
	{"POST", "/_async/AsyncResponseService", "", "", sigIoTRCE},
	{"POST", "/wls-wsat/CoordinatorPortType", "", "", sigIoTRCE},
	{"GET", "/console/css/%2e%2e%2fconsole.portal", "", "", sigIoTRCE}, // wins over admin /console/
	{"GET", "/solr/admin/cores", "action=STATUS", "", sigIoTRCE},       // wins over admin /solr/
	{"GET", "/console/css/app.css", "", "", sigAdminProbe},             // the exclusion is only the %2e%2e form
	{"GET", "/index.php", "s=/Index/\\think\\app/invokefunction&function=call_user_func_array", "", sigIoTRCE},
	{"GET", "/index.php", "s=/index/think/app/invokefunction", "", sigIoTRCE},
	{"GET", "/index.php", "s=/Index/%5Cthink%5Capp/invokefunction", "", sigIoTRCE},
	{"POST", "/ctrlt/DeviceUpgrade_1", "", "", sigIoTRCE},
	{"GET", "/TP/public/index.php", "", "", sigIoTRCE},
	{"POST", "/vendor/phpunit/phpunit/src/Util/PHP/eval-stdin.php", "", "", sigIoTRCE}, // wins over admin /vendor/phpunit
	{"POST", "/laravel/vendor/phpunit/phpunit/src/Util/PHP/eval-stdin.php", "", "", sigIoTRCE},
	{"GET", "/shell", "cd+/tmp;rm+-rf+*;wget+x", "", sigIoTRCE},

	// traversal
	{"GET", "/docs/index.html", "../", browserUA, sigTraversal},
	{"GET", "/static/%2e%2e%2fsecret", "", "", sigTraversal},
	{"GET", "/static/%252e%252e/x", "", "", sigTraversal},
	{"GET", "/a/..;/admin", "", "", sigTraversal},
	{"GET", "/download/..\\..\\boot.ini", "", "", sigTraversal},
	{"GET", "/etc/passwd", "", "", sigTraversal},
	{"GET", "/windows/win.ini", "", "", sigTraversal},
	{"GET", "/view", "file=/etc/passwd", "", sigTraversal},
	{"GET", "/view", "file=..%2F..%2Fetc%2Fpasswd", "", sigTraversal},
	{"GET", "/view", "f=/proc/self/environ", "", sigTraversal},
	{"GET", "/view", "f=C:\\Windows\\system.ini", "", sigTraversal},
	{"GET", "/view", "f=c:%5Cwindows%5Csystem.ini", "", sigTraversal},
	{"GET", "/view", "p=%c0%ae%c0%ae/", "", sigTraversal},
	{"GET", "/view", "p=%252e%252e%252f", "", sigTraversal},

	// injection
	{"GET", "/search", "q=union select", "", sigInjection},
	{"GET", "/search", "q=1+UNION+SELECT+password", "", sigInjection},
	{"GET", "/search", "q=1%20union%20all%20select%20null", "", sigInjection},
	{"GET", "/item", "id=1'+or+1=1--", "", sigInjection},
	{"GET", "/item", "id=1%22%20or%201=1", "", sigInjection},
	{"GET", "/item", "id=1+or+1=1--", "", sigInjection},
	{"GET", "/item", "id=sleep(5)", "", sigInjection},
	{"GET", "/item", "id=BENCHMARK(1000000,md5(1))", "", sigInjection},
	{"GET", "/item", "id=1;waitfor+delay+'0:0:5'", "", sigInjection},
	{"GET", "/search", "q=<script>alert(1)</script>", "", sigInjection},
	{"GET", "/search", "q=%3CScript%3Ealert(1)%3C/script%3E", "", sigInjection},
	{"GET", "/login", "next=javascript:alert(1)", "", sigInjection},
	{"GET", "/search", "q=<img+src=x+onerror=alert(1)>", "", sigInjection},
	{"GET", "/search", "x=${jndi:ldap://x/a}", "", sigInjection},
	{"GET", "/search", "x=${${lower:j}ndi:ldap://x/a}", "", sigInjection},
	{"GET", "/hello", "name={{7*7}}", "", sigInjection},
	{"GET", "/hello", "name={{config.items()}}", "", sigInjection},
	{"GET", "/hello", "name=<%25=+7*7+%25>", "", sigInjection},
	{"GET", "/ping", "host=x%0acurl+x", "", sigInjection},
	{"GET", "/ping", "host=a|curl+x", "", sigInjection},
	{"GET", "/ping", "host=a;wget+x", "", sigInjection},
	{"GET", "/ping", "host=$(id)", "", sigInjection},
	{"GET", "/ping", "host=`id`", "", sigInjection},
	{"GET", "/run", "c=/bin/bash+-c+id", "", sigInjection},
	{"GET", "/run", "c=cmd.exe", "", sigInjection},
	{"GET", "/run", "c=PowerShell+-enc+AA", "", sigInjection},
	{"GET", "/run", "x=base64_decode(abc)", "", sigInjection},
	{"GET", "/run", "x=passthru(id)", "", sigInjection},
	{"GET", "/page", "file=php://input", "", sigInjection},
	{"GET", "/page", "file=data://text/plain;base64,AA", "", sigInjection},
	{"GET", "/page", "file=expect://id", "", sigInjection},
	{"GET", "/page", "file=php://filter/convert.base64-encode/resource=index", "", sigInjection},
	{"GET", "/search/<script>alert(1)</script>", "", "", sigInjection},
	{"GET", "/${jndi:ldap:x}", "", "", sigInjection},
}

var sigNegative = []sigCase{
	{"GET", "/", "", browserUA, ""},
	{"GET", "/", "", "", ""},
	{"GET", "/api/health", "", "python-requests/2.32.3", ""},
	{"GET", "/api/health", "", "Go-http-client/1.1", ""},
	{"GET", "/", "", "curl/8.5.0", ""},
	{"GET", "/api/items", "", "python-httpx/0.27", ""},
	{"GET", "/", "", "python-requests/2.32.3", ""},
	{"GET", "/", "", "Go-http-client/1.1", ""},
	{"GET", "/config.json", "", browserUA, ""},
	{"GET", "/console", "", browserUA, ""},
	{"GET", "/metrics", "", "", ""},
	{"GET", "/dashboard/", "", browserUA, ""},
	{"GET", "/owa/auth/logon.aspx", "", browserUA, ""},
	{"GET", "/actuator/health", "", "kube-probe/1.36", ""},
	{"GET", "/images/thumb.php", "", browserUA, ""},
	{"GET", "/a.php", "", browserUA, ""},
	{"GET", "/secrets-of-the-sea.html", "", browserUA, ""},
	{"GET", "/docs/bin/bash-completion", "", browserUA, ""},
	{"GET", "/search", "term=powershell+scripts", browserUA, ""},
	{"GET", "/search", "q=system(design)", browserUA, ""},
	{"HEAD", "/", "", "curl/8.5.0", ""}, // bare-root client rule is GET only
	{"POST", "/", "", "curl/8.5.0", ""},
	{"GET", "/wp-json/wp/v2/posts", "", browserUA, ""},
	{"GET", "/static/app.js", "", browserUA, ""},
	{"POST", "/login", "", browserUA, ""},
	{"GET", "/metrics", "", promUA, ""},
	{"GET", "/.well-known/acme-challenge/xyz", "", "", ""},
	{"GET", "/.well-known/security.txt", "", browserUA, ""},
	{"GET", "/images/logo.png", "", browserUA, ""},
	{"GET", "/uploads/photo.jpg", "", browserUA, ""},
	{"GET", "/wp-content/uploads/2024/10/photo.jpg", "", browserUA, ""},
	{"GET", "/search", "q=select+a+union+of+sets", browserUA, ""},
	{"GET", "/search", "q=union++select", browserUA, ""}, // phrase needs exactly one separator
	{"GET", "/search", "q=50%25+off", browserUA, ""},
	{"GET", "/docs/index.html", "", browserUA, ""},
	{"GET", "/favicon.ico", "", browserUA, ""},
	{"GET", "/robots.txt", "", browserUA, ""},
	{"GET", "/api/v1/users", "", browserUA, ""},
	{"GET", "/api/v1/podcasts", "", browserUA, ""},
	{"GET", "/consoles", "", browserUA, ""},
	{"GET", "/horizontal-scroll", "", browserUA, ""},
	{"GET", "/mysql-tips", "", browserUA, ""},
	{"GET", "/dashboard", "", browserUA, ""},
	{"GET", "/debug/vars", "", browserUA, ""},
	{"GET", "/index.php", "page=2", browserUA, ""},
	{"GET", "/?author=abc", "", browserUA, ""},
	{"GET", "/", "author=", browserUA, ""},
	{"GET", "/blog", "author=1", browserUA, ""},
	{"GET", "/shell", "", browserUA, ""},
	{"GET", "/environment", "", browserUA, ""},
	{"GET", "/gitlab", "", browserUA, ""},
	{"OPTIONS", "/api/items", "", browserUA, ""},
	{"PUT", "/api/items/42", "", browserUA, ""},
	{"GET", "/login", "next=/dashboard", browserUA, ""},
	{"GET", "/api/report", "from=2026-01-01&to=2026-12-31&sort=-created", browserUA, ""},
	{"GET", "/assets/index-3f2a.css", "", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)", ""},
}

func TestLabels(t *testing.T) {
	want := []string{"proto-abuse", "scanner-ua", "secret-probe", "wp-probe", "admin-probe", "webshell", "iot-rce", "traversal", "injection"}
	got := Labels()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Labels() = %v, want %v", got, want)
	}
	got[0] = "mutated"
	if Labels()[0] != "proto-abuse" {
		t.Fatal("Labels() exposes its backing array")
	}
}

func TestClassifyPositive(t *testing.T) {
	if len(sigPositive) < 60 {
		t.Fatalf("only %d positive cases", len(sigPositive))
	}
	seen := map[string]int{}
	for _, c := range sigPositive {
		if got := Classify(c.method, c.path, c.query, c.ua); got != c.want {
			t.Errorf("Classify(%q, %q, %q, %q) = %q, want %q", c.method, c.path, c.query, c.ua, got, c.want)
		}
		seen[c.want]++
	}
	for _, l := range Labels() {
		if seen[l] == 0 {
			t.Errorf("label %q has no positive case", l)
		}
	}
}

func TestClassifyNegative(t *testing.T) {
	if len(sigNegative) < 25 {
		t.Fatalf("only %d negative cases", len(sigNegative))
	}
	for _, c := range sigNegative {
		if got := Classify(c.method, c.path, c.query, c.ua); got != "" {
			t.Errorf("Classify(%q, %q, %q, %q) = %q, want \"\"", c.method, c.path, c.query, c.ua, got)
		}
	}
}

// TestClassifyLabelsClosed checks every return value is "" or in Labels(),
// over the case tables and a deterministic mix of fragments.
func TestClassifyLabelsClosed(t *testing.T) {
	allowed := map[string]bool{"": true}
	for _, l := range Labels() {
		allowed[l] = true
	}
	check := func(c sigCase) {
		if got := Classify(c.method, c.path, c.query, c.ua); !allowed[got] {
			t.Errorf("Classify(%q, %q, %q, %q) = %q, not in Labels()", c.method, c.path, c.query, c.ua, got)
		}
	}
	for _, c := range sigPositive {
		check(c)
	}
	for _, c := range sigNegative {
		check(c)
	}
	frags := []string{"/", "..", "%", "%2", "%2e", "+", ";", "a", "Z", ".php", "/uploads", "/.git", "\x00", "\xff", "é", "${", "<", "?", "cd"}
	methods := []string{"GET", "POST", "trace", "", "\xff"}
	rng := rand.New(rand.NewSource(1))
	gen := func() string {
		var b strings.Builder
		for n := rng.Intn(8); n > 0; n-- {
			b.WriteString(frags[rng.Intn(len(frags))])
		}
		return b.String()
	}
	for i := 0; i < 5000; i++ {
		check(sigCase{methods[rng.Intn(len(methods))], gen(), gen(), gen(), ""})
	}
}

// TestClassifyCaps checks the inspection caps: payloads past the cap are not
// seen, payloads inside it are, and a 1 MB path and query cost microseconds.
func TestClassifyCaps(t *testing.T) {
	mb := 1 << 20
	cases := []sigCase{
		{"GET", "/" + strings.Repeat("a", maxSigPath) + "/etc/passwd", "", "", ""},
		{"GET", "/x", strings.Repeat("b", maxSigQuery) + "&f=../", "", ""},
		{"GET", "/", "", strings.Repeat("x", maxSigUA) + "zgrab", ""},
		{"GET", "/etc/passwd" + strings.Repeat("a", mb), "", "", sigTraversal},
		{"GET", "/x", "f=../" + strings.Repeat("b", mb), "", sigTraversal},
		{"GET", "/", "", "zgrab" + strings.Repeat("x", mb), sigScannerUA},
	}
	for _, c := range cases {
		if got := Classify(c.method, c.path, c.query, c.ua); got != c.want {
			t.Errorf("Classify(len %d path, len %d query, len %d ua) = %q, want %q", len(c.path), len(c.query), len(c.ua), got, c.want)
		}
	}

	// Uppercase and '%' force the lowercase copy and the query decode.
	path := "/" + strings.Repeat("A%2", mb/3)
	query := strings.Repeat("Q%2+", mb/4)
	ua := strings.Repeat("U", mb)
	const n = 200
	start := time.Now()
	for i := 0; i < n; i++ {
		Classify("GET", path, query, ua)
	}
	per := time.Since(start) / n
	t.Logf("1 MB path+query+ua: %v per call", per)
	if per > 200*time.Microsecond {
		t.Errorf("Classify on 1 MB inputs took %v per call, want well under 1ms", per)
	}
}

func BenchmarkClassify(b *testing.B) {
	inputs := []sigCase{
		{"GET", "/static/app.js", "", browserUA, ""},
		{"GET", "/api/v1/items", "page=2&sort=-created&filter=a%2Bb", browserUA, ""},
		{"GET", "/.env", "", "", ""},
		{"POST", "/xmlrpc.php", "", "", ""},
		{"GET", "/index.php", "s=/Index/%5Cthink%5Capp/invokefunction", "", ""},
		{"GET", "/search", "q=1+UNION+SELECT+password", browserUA, ""},
		{"GET", "/", "", "Go-http-client/1.1", ""},
		{"GET", "/Some/Mixed/Case/Path/That/Is/Clean", "Q=Hello+World", browserUA, ""},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c := inputs[i%len(inputs)]
		Classify(c.method, c.path, c.query, c.ua)
	}
}
