package dataplane

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// A bare IPv4-mapped rule (::ffff:a.b.c.d) is the IPv4 address a.b.c.d, for
// the gate and the rate-limit exemption alike, whether the client address is
// in 4-byte or 16-byte form; it is not the IPv6 network ::/32.
func TestBareIPv4MappedRule(t *testing.T) {
	v4 := func(s string) net.IP { return net.ParseIP(s).To4() }
	v16 := func(s string) net.IP { return net.ParseIP(s).To16() }
	for _, tc := range []struct {
		name    string
		al      model.AccessList
		clients map[string]net.IP
		allowed map[string]bool
	}{
		{
			name: "allow",
			al: model.AccessList{ObjectMeta: model.ObjectMeta{Name: "m"}, DefaultAction: model.ActionDeny,
				Rules: []model.IPRule{{Action: model.ActionAllow, CIDR: "::ffff:192.0.2.10"}}},
			clients: map[string]net.IP{
				"4-byte": v4("192.0.2.10"), "16-byte": v16("192.0.2.10"),
				"neighbour": v4("192.0.2.11"), "in ::/32": net.ParseIP("::c000:20a"), "loopback": net.ParseIP("::1"),
			},
			allowed: map[string]bool{"4-byte": true, "16-byte": true},
		},
		{
			name: "deny",
			al: model.AccessList{ObjectMeta: model.ObjectMeta{Name: "m"}, DefaultAction: model.ActionDeny,
				Rules: []model.IPRule{
					{Action: model.ActionDeny, CIDR: "::ffff:203.0.113.5"},
					{Action: model.ActionAllow, CIDR: "0.0.0.0/0"},
					{Action: model.ActionAllow, CIDR: "::/0"},
				}},
			clients: map[string]net.IP{
				"4-byte": v4("203.0.113.5"), "16-byte": v16("203.0.113.5"),
				"neighbour": v4("203.0.113.6"), "in ::/32": net.ParseIP("::cb00:7105"), "loopback": net.ParseIP("::1"),
			},
			allowed: map[string]bool{"neighbour": true, "in ::/32": true, "loopback": true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.al.Validate(); err != nil {
				t.Fatalf("validate: %v", err)
			}
			exempt := exemptionOf(t, tc.al)
			for label, ip := range tc.clients {
				want := tc.allowed[label]
				rec := httptest.NewRecorder()
				accessListHandler(compileAccessList(tc.al), func(*http.Request) net.IP { return ip }, nil, nil, "", nil, okHandler()).
					ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
				if (rec.Code == http.StatusOK) != want {
					t.Fatalf("%s %s: gate answered %d, allowed want %v", label, ip, rec.Code, want)
				}
				if got := exempt(ip); got != want {
					t.Fatalf("%s %s: exempt = %v, want %v", label, ip, got, want)
				}
			}
		})
	}
}
