package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// An unresolvable reference must never be silently skipped. On an ACCESS LIST
// that skip turns a restricted host into an open one - the exact opposite of
// what the reference was written for. Config.Validate rejects dangling refs and
// is the primary guard, but it must not be the only thing between a typo and an
// unauthenticated route.
func TestUnresolvableRefFailsClosed(t *testing.T) {
	reg := buildRegistry(model.Config{
		AccessLists: []model.AccessList{{
			ObjectMeta:    model.ObjectMeta{Name: "home-vpn"},
			DefaultAction: model.ActionDeny,
			Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "10.0.0.0/8"}},
		}},
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "sec-headers"},
			Type:       model.MWTypeHeaders,
			Headers:    &model.HeadersMiddleware{SetResponse: map[string]string{"X-Test": "1"}},
		}},
	})

	tests := []struct {
		name string
		host model.ProxyHost
		want int
	}{
		{
			name: "access list that does not exist",
			host: model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "a"}, AccessLists: []string{"home-vpnn"}},
			want: http.StatusServiceUnavailable,
		},
		{
			name: "middleware that does not exist",
			host: model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "b"}, Middlewares: []string{"ssoo"}},
			want: http.StatusServiceUnavailable,
		},
		{
			name: "one good ref and one bad one",
			host: model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "c"},
				Middlewares: []string{"sec-headers"}, AccessLists: []string{"home-vpn", "typo"}},
			want: http.StatusServiceUnavailable,
		},
		{
			name: "every ref resolves",
			host: model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "d"},
				Middlewares: []string{"sec-headers"}, AccessLists: []string{"home-vpn"}},
			want: http.StatusOK,
		},
		{
			name: "no refs at all",
			host: model.ProxyHost{ObjectMeta: model.ObjectMeta{Name: "e"}},
			want: http.StatusOK,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := buildChain(okHandler(), tc.host, reg, nil)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "10.1.2.3:1234"
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusServiceUnavailable && rec.Body.String() == "backend" {
				t.Fatal("the request reached the upstream despite an unresolvable reference")
			}
		})
	}

	// A LOCATION's own access lists are carried beside the host copy, not
	// folded into it, so the check has to cover them too: with the host's list
	// resolving and the client allowed by it, a typo on the location must still
	// fail that route closed, under either mode.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("backend")) }))
	defer backend.Close()
	for _, mode := range []string{"", model.AccessListsModeAny} {
		t.Run("location access list that does not exist, mode "+mode, func(t *testing.T) {
			cfg := model.Config{
				AccessLists: []model.AccessList{{
					ObjectMeta:    model.ObjectMeta{Name: "home-vpn"},
					DefaultAction: model.ActionDeny,
					Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "10.0.0.0/8"}},
				}},
				ProxyHosts: []model.ProxyHost{{
					ObjectMeta:  model.ObjectMeta{Name: "app"},
					Domains:     []string{"app.example.com"},
					Upstream:    upstreamOf(t, backend),
					AccessLists: []string{"home-vpn"},
					Locations:   []model.Location{{Path: "/api", AccessLists: []string{"home-vpn", "typo"}, AccessListsMode: mode}},
				}},
			}
			rt, err := buildRouter(cfg, "", nil)
			if err != nil {
				t.Fatalf("buildRouter: %v", err)
			}
			req := httptest.NewRequest("GET", "http://app.example.com/api/x", nil)
			req.RemoteAddr = "10.1.2.3:1234"
			rec := httptest.NewRecorder()
			rt.serveHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503 (body %q)", rec.Code, rec.Body.String())
			}
		})
	}
}
