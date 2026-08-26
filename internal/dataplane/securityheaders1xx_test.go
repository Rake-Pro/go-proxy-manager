package dataplane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// build1xxRouter compiles a single proxy host ("front.example") pointing at the
// given upstream, with a settings-level X-Frame-Options the upstream does NOT set
// and HSTS enabled, so both the set-if-absent security header and the override
// HSTS have to survive to the final response.
func build1xxRouter(t *testing.T, up model.Upstream) *router {
	t.Helper()
	withGlobalSecurityHeaders(t, map[string]string{"X-Frame-Options": "DENY"})
	withGlobalErrorPages(t, nil)
	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta:    model.ObjectMeta{Name: "front"},
		Domains:       []string{"front.example"},
		Upstream:      up,
		RobotsNoIndex: true,
		TLS:           model.TLSSettings{HSTS: model.HSTS{Enabled: true, MaxAge: 120}},
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	return rt
}

// front wraps the router in a real HTTP server so the reverse proxy's
// Got1xxResponse trace hook fires over a real connection (httptest.ResponseRecorder
// latches the first WriteHeader and cannot model an interim + final response).
func frontServer(t *testing.T, rt *router) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "front.example"
		rt.serveHTTPS(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func upstreamFor(t *testing.T, h http.Handler) model.Upstream {
	t.Helper()
	be := httptest.NewServer(h)
	t.Cleanup(be.Close)
	return upstreamFromURL(t, be.URL)
}

func upstreamFromURL(t *testing.T, raw string) model.Upstream {
	t.Helper()
	// raw is http://127.0.0.1:PORT
	rest := strings.TrimPrefix(raw, "http://")
	host, port, ok := strings.Cut(rest, ":")
	if !ok {
		t.Fatalf("cannot parse upstream url %q", raw)
	}
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return model.Upstream{Scheme: "http", Host: host, Port: p}
}

// TestSecurityHeadersSurvive1xx is the regression for the MEDIUM: an upstream 1xx
// interim response must not drop the injected security headers (or HSTS/robots)
// from the FINAL proxied response. Covers 103 Early Hints and 100-Continue.
func TestSecurityHeadersSurvive1xx(t *testing.T) {
	t.Run("103 early hints", func(t *testing.T) {
		rt := build1xxRouter(t, upstreamFor(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// Interim 103 with a Link header, then the final 200.
			w.Header().Add("Link", "</style.css>; rel=preload; as=style")
			w.WriteHeader(http.StatusEarlyHints)
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("final body"))
		})))
		srv := frontServer(t, rt)

		resp, err := http.Get(srv.URL + "/")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || string(body) != "final body" {
			t.Fatalf("final response = %d %q", resp.StatusCode, body)
		}
		assert1xxHeaders(t, resp)
	})

	t.Run("100 continue", func(t *testing.T) {
		rt := build1xxRouter(t, upstreamFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Reading the body is what makes the server emit the 100-Continue the
			// client asked for with Expect: 100-continue.
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte("posted"))
		})))
		srv := frontServer(t, rt)

		req, err := http.NewRequest("POST", srv.URL+"/", strings.NewReader("payload"))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Expect", "100-continue")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || string(body) != "posted" {
			t.Fatalf("final response = %d %q", resp.StatusCode, body)
		}
		assert1xxHeaders(t, resp)
	})
}

func assert1xxHeaders(t *testing.T, resp *http.Response) {
	t.Helper()
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY to survive the 1xx interim response", got)
	}
	if got := resp.Header.Get("Strict-Transport-Security"); got != "max-age=120" {
		t.Fatalf("Strict-Transport-Security = %q, want max-age=120 to survive the 1xx", got)
	}
	if got := resp.Header.Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("X-Robots-Tag = %q, want it to survive the 1xx", got)
	}
}
