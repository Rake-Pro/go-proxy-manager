package dataplane

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestAuthGateOrder pins the order in which several auth gates on one route
// run: a location's inline block, then the host's inline block, then the
// referenced auth middlewares last-referenced first (the location's
// references come after the host's in the combined list, so they run before
// them).
//
// Every gate is a basic gate with its own realm. The gate expected at position
// k exempts the clients 198.51.100.k, 198.51.100.k+1, ..., so client
// 198.51.100.j passes exactly the first j gates and is challenged by gate j+1;
// the realm in the challenge names that gate.
func TestAuthGateOrder(t *testing.T) {
	cases := []struct {
		name  string
		route string   // "host" or "location"
		order []string // expected run order, first to last
	}{
		{"host route", "host", []string{"host-inline", "h2", "h1"}},
		{"location route", "location", []string{"loc-inline", "host-inline", "l2", "l1", "h2", "h1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate := func(realm string) model.AuthMiddleware {
				pos := -1
				for i, n := range tc.order {
					if n == realm {
						pos = i + 1
					}
				}
				var allow []string
				if pos > 0 {
					for j := pos; j <= len(tc.order); j++ {
						allow = append(allow, fmt.Sprintf("198.51.100.%d/32", j))
					}
				} else {
					allow = []string{"0.0.0.0/0"} // not on this route
				}
				return basicMW(t, "u", "p", realm, allow...)
			}
			var mws []model.Middleware
			for _, n := range []string{"h1", "h2", "l1", "l2"} {
				a := gate(n)
				mws = append(mws, model.Middleware{ObjectMeta: model.ObjectMeta{Name: n}, Type: model.MWTypeAuth, Auth: &a})
			}
			hostInline, locInline := gate("host-inline"), gate("loc-inline")
			cfg := model.Config{
				Middlewares: mws,
				ProxyHosts: []model.ProxyHost{{
					ObjectMeta:  model.ObjectMeta{Name: "app"},
					Domains:     []string{"app.example.com"},
					Upstream:    model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 9},
					Middlewares: []string{"h1", "h2"},
					Auth:        &hostInline,
					Locations: []model.Location{{
						Path:        "/loc",
						Middlewares: []string{"l1", "l2"},
						Auth:        &locInline,
					}},
				}},
			}
			rt, err := buildRouter(cfg, "", nil)
			if err != nil {
				t.Fatalf("buildRouter: %v", err)
			}
			hh := rt.hosts["app.example.com"]
			h, path := hh.handler, "/"
			if tc.route == "location" {
				h, path = hh.locations[0].handler, "/loc"
			}
			for j := 0; j < len(tc.order); j++ {
				remote := fmt.Sprintf("198.51.100.%d:1000", j)
				if j == 0 {
					remote = "203.0.113.9:1000"
				}
				rec := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "http://app.example.com"+path, nil)
				req.RemoteAddr = remote
				h.ServeHTTP(rec, req)
				want := `Basic realm="` + tc.order[j] + `"`
				if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != want {
					t.Fatalf("client passing %d gates: got %d %q, want 401 %q (gate %d of %v)",
						j, rec.Code, rec.Header().Get("WWW-Authenticate"), want, j+1, tc.order)
				}
			}
		})
	}
}
