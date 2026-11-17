package dataplane

import (
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestSetRequestSurvivesConnectionHeader: a header set by a headers middleware
// (setRequest) reaches the upstream even when the client names it in its
// Connection header, which makes the reverse proxy delete it before Rewrite.
// Names the proxy writes itself keep the proxy's value.
func TestSetRequestSurvivesConnectionHeader(t *testing.T) {
	var mu sync.Mutex
	var seen http.Header
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = r.Header.Clone()
		mu.Unlock()
	}))
	defer closeFn()
	cfg := model.Config{
		Middlewares: []model.Middleware{{
			ObjectMeta: model.ObjectMeta{Name: "pin-header"},
			Type:       model.MWTypeHeaders,
			Headers: &model.HeadersMiddleware{SetRequest: map[string]string{
				"X-Tenant":         "restricted",
				"X-Forwarded-Host": "operator.example.com",
			}},
		}},
		ProxyHosts: []model.ProxyHost{{
			ObjectMeta:  model.ObjectMeta{Name: "app"},
			Domains:     []string{"app.example.com"},
			Upstream:    up,
			Middlewares: []string{"pin-header"},
		}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	addr := listenWithPolicy(t, http.HandlerFunc(rt.serveHTTP))

	for _, conn := range []string{"close", "close, X-Tenant", "close, x-tenant, X-Forwarded-Host"} {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		code, _ := rawExchange(t, c, "GET / HTTP/1.1\r\nHost: app.example.com\r\nX-Tenant: other\r\nConnection: "+conn+"\r\n\r\n")
		if code != http.StatusOK {
			t.Fatalf("Connection %q: status %d", conn, code)
		}
		mu.Lock()
		got, xfh := seen.Values("X-Tenant"), seen.Get("X-Forwarded-Host")
		mu.Unlock()
		if len(got) != 1 || got[0] != "restricted" {
			t.Errorf("Connection %q: upstream X-Tenant = %q, want the setRequest value only", conn, got)
		}
		if xfh != "app.example.com" {
			t.Errorf("Connection %q: upstream X-Forwarded-Host = %q, want the proxy's own value", conn, xfh)
		}
	}
}
