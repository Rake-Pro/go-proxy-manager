package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func serveOn(rt *router, tls bool, method, rawurl, host string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, rawurl, nil)
	req.Host = host
	if tls {
		rt.serveHTTPS(rec, req)
	} else {
		rt.serveHTTP(rec, req)
	}
	return rec
}

func TestRedirectHostServes(t *testing.T) {
	cfg := model.Config{RedirectHosts: []model.RedirectHost{
		{ObjectMeta: model.ObjectMeta{Name: "apex"}, Domains: []string{"example.com"}, TargetDomain: "www.example.com", StatusCode: 301, PreservePath: true},
		{ObjectMeta: model.ObjectMeta{Name: "auto"}, Domains: []string{"auto.com"}, TargetDomain: "dest.com"}, // default 301, auto scheme, no path
		{ObjectMeta: model.ObjectMeta{Name: "fixed"}, Domains: []string{"fixed.com"}, TargetScheme: "https", TargetDomain: "secure.com", StatusCode: 302},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// preservePath keeps path + query; status as configured.
	rec := serveOn(rt, true, "GET", "https://example.com/a/b?x=1", "example.com")
	if rec.Code != 301 || rec.Header().Get("Location") != "https://www.example.com/a/b?x=1" {
		t.Fatalf("apex: got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// auto scheme follows the inbound scheme; no path when preservePath is false.
	if rec := serveOn(rt, true, "GET", "https://auto.com/ignored", "auto.com"); rec.Header().Get("Location") != "https://dest.com" {
		t.Fatalf("auto https: %q", rec.Header().Get("Location"))
	}
	if rec := serveOn(rt, false, "GET", "http://auto.com/ignored", "auto.com"); rec.Header().Get("Location") != "http://dest.com" {
		t.Fatalf("auto http: %q", rec.Header().Get("Location"))
	}
	// explicit scheme + status override the inbound scheme.
	if rec := serveOn(rt, false, "GET", "http://fixed.com/", "fixed.com"); rec.Code != 302 || rec.Header().Get("Location") != "https://secure.com" {
		t.Fatalf("fixed: got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestParkedHostServes(t *testing.T) {
	cfg := model.Config{ParkedHosts: []model.ParkedHost{
		{ObjectMeta: model.ObjectMeta{Name: "gone"}, Domains: []string{"gone.com"}},
		{ObjectMeta: model.ObjectMeta{Name: "teapot"}, Domains: []string{"teapot.com"}, StatusCode: 418},
	}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if rec := serveOn(rt, true, "GET", "https://gone.com/whatever", "gone.com"); rec.Code != http.StatusNotFound {
		t.Fatalf("parked default: got %d, want 404", rec.Code)
	}
	if rec := serveOn(rt, true, "GET", "https://teapot.com/", "teapot.com"); rec.Code != http.StatusTeapot {
		t.Fatalf("parked custom: got %d, want 418", rec.Code)
	}
	// Unknown host still 404s.
	if rec := serveOn(rt, true, "GET", "https://unknown.com/", "unknown.com"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown host: got %d", rec.Code)
	}
}

func TestRedirectAndParkedForceSSL(t *testing.T) {
	cfg := model.Config{
		RedirectHosts: []model.RedirectHost{{ObjectMeta: model.ObjectMeta{Name: "r"}, Domains: []string{"r.com"}, TargetDomain: "t.com", TLS: model.TLSSettings{ForceSSL: true}}},
		ParkedHosts:   []model.ParkedHost{{ObjectMeta: model.ObjectMeta{Name: "d"}, Domains: []string{"d.com"}, TLS: model.TLSSettings{ForceSSL: true}}},
	}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Plain HTTP with forceSSL -> 308 to https on the same host, before serving.
	if rec := serveOn(rt, false, "GET", "http://r.com/x?y=1", "r.com"); rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != "https://r.com/x?y=1" {
		t.Fatalf("redirect forceSSL: got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := serveOn(rt, false, "GET", "http://d.com/x", "d.com"); rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != "https://d.com/x" {
		t.Fatalf("parked forceSSL: got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// Over HTTPS the redirect host serves its redirect (auto scheme = https).
	if rec := serveOn(rt, true, "GET", "https://r.com/", "r.com"); rec.Header().Get("Location") != "https://t.com" {
		t.Fatalf("redirect over https: %q", rec.Header().Get("Location"))
	}
}
