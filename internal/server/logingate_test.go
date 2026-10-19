package server

import (
	"net/http"
	"net/http/httptest"
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
