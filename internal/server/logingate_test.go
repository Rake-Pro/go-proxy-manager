package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClientIPKeyGroupsIPv6By64(t *testing.T) {
	key := func(addr string) string {
		r := httptest.NewRequest("GET", "/auth/login", nil)
		r.RemoteAddr = addr
		return clientIPKey(r)
	}
	a, b := key("[2001:db8:1:2:3::1]:1234"), key("[2001:db8:1:2:ffff::9]:5")
	if a != b || a != "2001:db8:1:2::/64" {
		t.Fatalf("two addresses in one /64 must share a key: %q vs %q", a, b)
	}
	if c := key("[2001:db8:1:3::1]:1"); c == a {
		t.Fatal("a different /64 must get its own key")
	}
	if v4 := key("203.0.113.7:9"); v4 != "203.0.113.7" {
		t.Fatalf("IPv4 keys are the address: %q", v4)
	}
}

func TestIdPErrorForLogIsBounded(t *testing.T) {
	long := strings.Repeat("a", 500000)
	if got := idpErrorForLog(long); len(got) > 67 || !strings.HasSuffix(got, "...") {
		t.Fatalf("a huge value must be trimmed: %d bytes", len(got))
	}
	if got := idpErrorForLog("access_denied\n<img>\x00\"q\""); got != "access_denied<img>q" {
		t.Fatalf("control bytes and quotes must be dropped: %q", got)
	}
}

func TestLoginPageIsNotCached(t *testing.T) {
	e := newTestEnv(t, envOpts{})
	res := e.get("/auth/login?select=1")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("got %d", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
}

// A distributed guess against one account (a fresh client IP each time, so the
// per-IP gate never trips) must lock that account's local login, and the locked
// response must be identical to a locked-IP response.
func TestLocalLoginLockoutPerUsername(t *testing.T) {
	e := newTestEnv(t, envOpts{
		localUser: "admin", localPass: "s3cret", localEnabled: true, noProviders: true,
	})
	post := func(ip, user, pass string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/auth/local", strings.NewReader(url.Values{
			"username": {user}, "password": {pass}, "return": {"/"},
		}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = ip + ":1"
		res := e.do(req)
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	// 20 guesses (auth.maxUserLoginFails), each from its own IP so the per-IP
	// gate never trips: only the username counter can lock this.
	for i := 0; i < 20; i++ {
		ip := fmt.Sprintf("198.51.100.%d", i+1)
		if code, _ := post(ip, "Admin", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("guess %d = %d, want 401", i, code)
		}
	}
	// Fresh IP, correct password: the account is locked (case variants share a counter).
	lockedCode, lockedBody := post("198.51.100.99", "admin", "s3cret")
	if lockedCode != http.StatusTooManyRequests {
		t.Fatalf("locked username from a fresh IP = %d, want 429", lockedCode)
	}

	// Same response as a locked IP, for a username that does not exist.
	for i := 0; i < 5; i++ {
		post("203.0.113.7", fmt.Sprintf("ghost%d", i), "x")
	}
	ipCode, ipBody := post("203.0.113.7", "ghost-final", "x")
	if ipCode != lockedCode {
		t.Fatalf("locked IP = %d, locked username = %d, want identical", ipCode, lockedCode)
	}
	if norm := func(b string) string { return strings.NewReplacer("ghost-final", "U", "admin", "U").Replace(b) }; norm(ipBody) != norm(lockedBody) {
		t.Fatalf("locked-IP and locked-username pages differ:\nIP:   %s\nUSER: %s", ipBody, lockedBody)
	}

	// An unrelated username from an unrelated IP is unaffected.
	if code, _ := post("192.0.2.200", "other", "x"); code != http.StatusUnauthorized {
		t.Fatalf("unrelated username = %d, want 401", code)
	}
}

// A completed login clears the username counter, like the per-IP one.
func TestLocalLoginSuccessClearsUsernameCounter(t *testing.T) {
	e := newTestEnv(t, envOpts{
		localUser: "admin", localPass: "s3cret", localEnabled: true, noProviders: true,
	})
	post := func(ip, pass string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/local", strings.NewReader(url.Values{
			"username": {"admin"}, "password": {pass}, "return": {"/"},
		}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = ip + ":1"
		return e.do(req).StatusCode
	}
	for i := 0; i < 4; i++ {
		post(fmt.Sprintf("198.51.100.%d", i+1), "wrong")
	}
	if code := post("198.51.100.50", "s3cret"); code != http.StatusFound {
		t.Fatalf("login with 4 prior failures = %d, want 302", code)
	}
	for i := 0; i < 4; i++ {
		if code := post(fmt.Sprintf("198.51.100.%d", 60+i), "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("post-success failure %d = %d, want 401 (counter was cleared)", i, code)
		}
	}
}
