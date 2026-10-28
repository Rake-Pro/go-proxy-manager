// Threat signatures.
//
// This file is the signature catalogue behind the Signatures table: a fixed,
// ordered list of request shapes that internet scanners, credential hunters
// and exploit kits produce. Classify maps one request (method, cleaned path,
// raw query, User-Agent) to the first matching label, or "" for ordinary
// traffic.
//
// Observe only. A label is a statistic, never a verdict: nothing here blocks,
// delays or rewrites a request, and nothing on the request path may branch on
// it. Enforcement belongs to the access lists, guards and bouncers.
//
// Cost. Classify runs on the analytics worker, not the request path, but the
// input is attacker-controlled, so it stays cheap and bounded: each input is
// clipped (maxSigPath, maxSigQuery, maxSigUA) before anything else, every
// string is lowercased once, exact maps are tried before prefix lists and
// prefix lists before substring lists, and the query is percent-decoded once
// (only when it holds '%' or '+'). There are no regular expressions; the one
// pattern a prefix cannot express (author=<digits>) is a hand-rolled check.
//
// Matching. All matching is case-insensitive: tables are written in lowercase
// and compared against lowercased input. The path arrives already decoded
// once by the caller, so the traversal and injection tables also carry the
// still-encoded forms a double-encoded payload leaves behind; the query is
// checked both raw and decoded.
//
// Labels are a closed set (sigLabels) because they become aggregate keys:
// counters, table rows and time series are keyed by them, so a label must
// never be derived from request data. Renaming or removing a label orphans
// stored aggregates, so treat the strings as a stable vocabulary.
//
// Adding a rule:
//   - Pick the label. If none fits, add a constant, append it to sigLabels at
//     the position where it should be evaluated, and add its check to Classify
//     in the same position.
//   - Prefer the cheapest table that expresses it: exact, then prefix, then
//     contains. Write entries in lowercase.
//   - If a new entry sits under a broader entry of an earlier label (as the
//     exploit endpoints in iotUnderAdmin do), add an exclusion so the more
//     specific label wins, and pin the result in classify_test.go.
//   - Add at least one positive and one look-alike negative test case.

package analytics

import "strings"

// Inspection caps: bytes beyond these are never looked at, so a huge path,
// query or User-Agent costs the same as a capped one.
const (
	maxSigPath  = 2048
	maxSigQuery = 512
	maxSigUA    = 512
)

const (
	sigProtoAbuse  = "proto-abuse"
	sigScannerUA   = "scanner-ua"
	sigSecretProbe = "secret-probe"
	sigWPProbe     = "wp-probe"
	sigAdminProbe  = "admin-probe"
	sigWebshell    = "webshell"
	sigIoTRCE      = "iot-rce"
	sigTraversal   = "traversal"
	sigInjection   = "injection"
)

// sigLabels is the closed label set in evaluation order.
var sigLabels = []string{
	sigProtoAbuse, sigScannerUA, sigSecretProbe, sigWPProbe, sigAdminProbe,
	sigWebshell, sigIoTRCE, sigTraversal, sigInjection,
}

// Labels lists every label Classify can return, in evaluation order.
func Labels() []string { return append([]string(nil), sigLabels...) }

// sigPaths is one label's path table, tried cheapest first.
type sigPaths struct {
	exact    map[string]bool
	prefix   []string
	contains []string
}

func (r *sigPaths) match(p string) bool {
	return r.exact[p] || hasAnyPrefix(p, r.prefix) || containsAny(p, r.contains)
}

func sigSet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// scannerUA tokens label a request on the User-Agent alone.
var scannerUA = []string{
	"zgrab", "masscan", "nmap", "nikto", "nuclei", "sqlmap", "wpscan",
	"gobuster", "dirbuster", "feroxbuster", "dirb", "httpx", "censys", "shodan",
	"internet-measurement", "expanse", "paloaltonetworks", "projectdiscovery",
	"nessus", "openvas", "acunetix", "netsparker", "whatweb", "zmap",
	"l9explore", "leakix", "netcraft",
}

// Generic HTTP client libraries (curl, python-requests, Go's default client)
// are deliberately NOT scanner signatures, not even for a bare "GET /": health
// checks, uptime monitors and deploy scripts look exactly like that, and a
// label that fires on every one of them stops meaning anything.

// secretPaths: dotfiles, VCS metadata, keys, dumps and config files. Entries
// match as literal prefixes, except "/.env" and "/.git" (exact; their
// children are covered by "/.env." and "/.git/"). Directory entries also
// match the bare directory, since a cleaned path drops the trailing slash.
// /.well-known/security.txt is deliberately not here: it is a published file
// that well-behaved clients fetch.
var secretPaths = sigPaths{
	exact: sigSet("/.env", "/.git", "/.svn", "/.hg", "/.aws", "/.ssh", "/.vscode", "/.idea", "/secrets"),
	prefix: []string{
		"/.env.", "/.git/", "/.svn/", "/.hg/", "/.aws/", "/.ssh/", "/.vscode/", "/.idea/",
		"/.ds_store", "/.npmrc", "/.htpasswd", "/.htaccess", "/.bash_history",
		"/id_rsa", "/server.key", "/config.yml", "/config.yaml", "/secrets/", "/secrets.",
		"/backup.zip", "/backup.sql", "/db.sql", "/dump.sql", "/phpinfo.php", "/info.php",
	},
}

// wpPaths: blog-engine login, RPC, admin, plugin and user-enumeration paths.
// The uploads-dir .php rule and ?author=N enumeration are checked in Classify.
// /wp-json/wp/v2/users enumerates accounts; other /wp-json routes (posts,
// pages) are ordinary API traffic.
var wpPaths = sigPaths{
	exact:    sigSet("/wp-login.php", "/xmlrpc.php"),
	prefix:   []string{"/wp-admin", "/wp-includes/", "/wp-content/plugins/", "/wp-json/wp/v2/users"},
	contains: []string{"/wlwmanifest.xml"},
}

// adminPaths: database consoles, app-server managers, debug and metrics
// endpoints, mail front ends. Short generic names (/pma, /mysql, /console,
// /horizon ...) match as a whole segment (exact or followed by "/") so that
// /consoles or /horizontal-scroll stay clean; distinctive names match as
// literal prefixes. /debug is exact (other /debug/* paths are left alone
// except /debug/pprof). /metrics is deliberately absent: scrapers hit it.
var adminPaths = sigPaths{
	exact: sigSet("/pma", "/pmd", "/mysql", "/solr", "/jenkins", "/telescope", "/horizon", "/debug"),
	prefix: []string{
		"/pma/", "/pmd/", "/mysql/", "/console/", "/solr/", "/jenkins/", "/telescope/", "/horizon/",
		"/phpmyadmin", "/myadmin", "/adminer", // "/adminer" also covers /adminer.php
		"/actuator", "/server-status", "/server-info", "/manager/html", "/manager/text",
		"/jmx-console", "/web-console", "/_ignition", "/debug/pprof", "/api/v1/pods", "/.kube",
		"/autodiscover/autodiscover.xml",
		"/vendor/phpunit", "/cgi-bin/php",
	},
}

// iotUnderAdmin are exploit endpoints (iot-rce) that live under an
// admin-probe prefix. admin-probe skips paths containing them so the more
// specific label wins; iot-rce matches them. This keeps the evaluation order
// intact (admin-probe is still evaluated first) without moving whole prefixes
// between labels.
//
// adminBenign are ordinary endpoints under an admin-probe prefix that real
// deployments expose on purpose (a health or info endpoint behind the
// management prefix); they are not probes.
var adminBenign = []string{"/actuator/health", "/actuator/info", "/actuator/prometheus"}

var iotUnderAdmin = []string{
	"/console/css/%2e%2e",                  // app-server console auth bypass (double-encoded ../)
	"/solr/admin/cores",                    // search-server core admin RCE chain
	"/phpunit/src/util/php/eval-stdin.php", // test-framework eval endpoint left in vendor/
}

// iotPaths: router, camera, NAS and appliance exploit endpoints plus a few
// well-known server RCE routes. /cgi-bin/php* never reaches here: it is
// admin-probe, which is evaluated first. /shell?cd... and the PHP framework
// routing RCE (/index.php?s=/index/think...) are checked in Classify.
var iotPaths = sigPaths{
	prefix: []string{
		"/cgi-bin/", "/dvr/", "/ws/v1/cluster/apps", "/manager/radius/server-ping",
		"/autodiscover/", "/tp/public/index.php",
	},
	contains: append([]string{
		"/boaform/", "/gponform/", "/hnap1", "/setup.cgi", "/picsdesc.xml", "/goform/",
		"/tmunblock.cgi", "/hndunblock.cgi", "/cgi-bin/luci", "/device.rsp", "stok=",
		"/api/jsonws/invoke", "/_async/asyncresponseservice", "/wls-wsat/",
		"/ctrlt/deviceupgrade_1",
	}, iotUnderAdmin...),
}

// thinkRoute matches the query of the PHP framework routing RCE; the decoded
// query also catches the %5c (backslash) form.
var thinkRoute = []string{"s=/index/\\think", "s=/index/think"}

// webshellNames are well-known shell file names, wherever they sit.
var webshellNames = sigSet(
	"shell.php", "cmd.php", "c99.php", "r57.php", "alfa.php", "x.php", "1.php",
	"up.php", "wso.php", "mini.php", "bypass.php", "exploit.php",
	"shell.jsp", "cmd.jsp", "cmd.aspx", "shell.aspx",
)

// webshellDirs + webshellExts: any script in a directory that should only
// ever hold static uploads.
var (
	webshellDirs = []string{"/uploads/", "/upload/", "/tmp/", "/temp/"}
	webshellExts = []string{".php", ".php7", ".phtml", ".phar"}
)

// traversalNeedles cover plain and still-encoded dot-dot forms (%252e and
// %c0%ae survive one decode), path-parameter tricks and the usual targets.
var traversalNeedles = []string{
	"../", "..\\", "%2e%2e", "%252e", "..;/", "/etc/passwd", "/proc/self",
	"c:\\windows", "/windows/win.ini", "%c0%ae",
}

// injectionNeedles: SQL, script/markup, template, expression-language, shell
// and PHP stream payloads. Phrases with spaces need exactly one space; in the
// query, '+' and %20 decode to that space, so "union+select" matches while
// "union of sets" does not.
var injectionNeedles = []string{
	"union select", "union all select", "' or 1=1", "\" or 1=1", "or 1=1--",
	"sleep(", "benchmark(", "waitfor delay",
	"<script", "javascript:", "onerror=",
	"${jndi:", "${${", "{{7*7}}", "{{config", "<%",
	"%0acurl", "|curl ", ";curl ", "|wget ", ";wget ", "$(", "`id`",
	"cmd.exe", "powershell -", "powershell.exe", "powershell%20-",
	"base64_decode(", "passthru(",
	"php://input", "data://text", "expect://", "php://filter",
}

// shellNeedles are matched as whole tokens: "/bin/sh" followed by the end of
// the value or a shell separator, so "/bin/bash-completion" or a docs path
// about shells does not count. "system(", "exec(" and "eval(" are left out on
// purpose: as bare words in a search box they are far more often prose than
// payload, and a real PHP payload almost always carries one of the needles
// above as well.
var shellNeedles = []string{"/bin/sh", "/bin/bash"}

func hasShellToken(s string) bool {
	for _, n := range shellNeedles {
		for i := strings.Index(s, n); i >= 0; {
			j := i + len(n)
			if j == len(s) || strings.IndexByte(" ;|&'\"%+)", s[j]) >= 0 {
				return true
			}
			k := strings.Index(s[j:], n)
			if k < 0 {
				break
			}
			i = j + k
		}
	}
	return false
}

// Classify returns the first matching signature label for a request, or ""
// when nothing matches. method is the HTTP method (any case), path the
// cleaned request path (no query), rawQuery the raw query string (may be
// empty) and userAgent the User-Agent header (may be empty). The label set is
// closed: every non-empty return value is one of Labels().
func Classify(method, path, rawQuery, userAgent string) string {
	path = clip(path, maxSigPath)
	// A path not starting with "/" is an absolute-URI or garbage request line
	// (this includes the rare server-wide "OPTIONS *").
	if strings.EqualFold(method, "CONNECT") || strings.EqualFold(method, "TRACE") || !strings.HasPrefix(path, "/") {
		return sigProtoAbuse
	}

	ua := strings.ToLower(clip(userAgent, maxSigUA))
	p := strings.ToLower(path)
	// python-httpx is an ordinary client library whose name happens to contain
	// the "httpx" scanner token.
	if containsAny(ua, scannerUA) && !strings.Contains(ua, "python-httpx") {
		return sigScannerUA
	}

	if secretPaths.match(p) {
		return sigSecretProbe
	}

	q := strings.ToLower(clip(rawQuery, maxSigQuery))
	if wpPaths.match(p) ||
		(strings.HasPrefix(p, "/wp-content/uploads/") && strings.HasSuffix(p, ".php")) ||
		(p == "/" && isAuthorEnum(q)) {
		return sigWPProbe
	}

	if adminPaths.match(p) && !containsAny(p, iotUnderAdmin) && !hasAnyPrefix(p, adminBenign) {
		return sigAdminProbe
	}

	if isWebshell(p) {
		return sigWebshell
	}

	dq := decodeQuery(q)
	if iotPaths.match(p) ||
		(p == "/shell" && strings.HasPrefix(q, "cd")) ||
		(p == "/index.php" && (containsAny(q, thinkRoute) || containsAny(dq, thinkRoute))) {
		return sigIoTRCE
	}

	if containsAny(p, traversalNeedles) || containsAny(q, traversalNeedles) || (dq != q && containsAny(dq, traversalNeedles)) {
		return sigTraversal
	}

	if containsAny(p, injectionNeedles) || containsAny(q, injectionNeedles) || (dq != q && containsAny(dq, injectionNeedles)) ||
		hasShellToken(p) || hasShellToken(q) || (dq != q && hasShellToken(dq)) {
		return sigInjection
	}
	return ""
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, x := range prefixes {
		if strings.HasPrefix(s, x) {
			return true
		}
	}
	return false
}

// isAuthorEnum reports whether q is exactly author=<digits>.
func isAuthorEnum(q string) bool {
	rest, ok := strings.CutPrefix(q, "author=")
	if !ok || rest == "" {
		return false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return false
		}
	}
	return true
}

func isWebshell(p string) bool {
	if webshellNames[p[strings.LastIndexByte(p, '/')+1:]] {
		return true
	}
	for _, ext := range webshellExts {
		if strings.HasSuffix(p, ext) {
			return containsAny(p, webshellDirs)
		}
	}
	return false
}

// decodeQuery percent-decodes q once ('+' becomes a space) and lowercases the
// result. It is lenient: a malformed escape is kept literally instead of
// failing, so "%zz" cannot hide the rest of the query. q is returned as is
// when there is nothing to decode.
func decodeQuery(q string) string {
	if strings.IndexByte(q, '%') < 0 && strings.IndexByte(q, '+') < 0 {
		return q
	}
	var b strings.Builder
	b.Grow(len(q))
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '+':
			c = ' '
		case c == '%' && i+2 < len(q):
			hi, ok1 := unhexSig(q[i+1])
			lo, ok2 := unhexSig(q[i+2])
			if ok1 && ok2 {
				c = hi<<4 | lo
				i += 2
			}
		}
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

func unhexSig(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
