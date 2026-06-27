package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend"))
	})
}

func ipFrom(s string) func(*http.Request) net.IP {
	return func(*http.Request) net.IP { return net.ParseIP(s) }
}

func TestAccessListIP(t *testing.T) {
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "lan"},
		DefaultAction: model.ActionDeny,
		Rules: []model.IPRule{
			{Action: model.ActionAllow, CIDR: "10.0.0.0/8"},
			{Action: model.ActionDeny, CIDR: "0.0.0.0/0"},
		},
	}
	h := accessListHandler(compileAccessList(al), nil, okHandler())

	cases := []struct {
		ip   string
		want int
	}{
		{"10.1.2.3", http.StatusOK},
		{"192.168.1.1", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			h := accessListHandler(compileAccessList(al), ipFrom(c.ip), okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != c.want {
				t.Fatalf("ip %s: got %d want %d", c.ip, rec.Code, c.want)
			}
		})
	}
	_ = h
}

func TestAccessListBasicAuth(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	al := model.AccessList{
		ObjectMeta: model.ObjectMeta{Name: "auth"},
		BasicAuth:  []model.BasicAuthUser{{Username: "admin", PasswordHash: string(hash)}},
	}
	h := accessListHandler(compileAccessList(al), ipFrom("1.2.3.4"), okHandler())

	// No creds -> 401 with challenge.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("expected 401 challenge, got %d (%q)", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}

	// Good creds -> 200.
	rec = httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("admin", "hunter2")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with good creds, got %d", rec.Code)
	}

	// Wrong password -> 401.
	rec = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("admin", "wrong")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with bad creds, got %d", rec.Code)
	}
}

func TestAccessListSatisfyAny(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	al := model.AccessList{
		ObjectMeta:    model.ObjectMeta{Name: "any"},
		SatisfyAny:    true,
		DefaultAction: model.ActionDeny,
		Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "10.0.0.0/8"}},
		BasicAuth:     []model.BasicAuthUser{{Username: "u", PasswordHash: string(hash)}},
	}
	// Trusted IP, no creds -> allowed because satisfyAny.
	h := accessListHandler(compileAccessList(al), ipFrom("10.9.9.9"), okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("satisfyAny trusted IP should pass without creds, got %d", rec.Code)
	}
}

func TestAccessListEmptyIsOpen(t *testing.T) {
	al := model.AccessList{ObjectMeta: model.ObjectMeta{Name: "empty"}}
	h := accessListHandler(compileAccessList(al), ipFrom("8.8.8.8"), okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("empty access list should impose no restriction, got %d", rec.Code)
	}
}
