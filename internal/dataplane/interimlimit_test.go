package dataplane

import (
	"io"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// Interim responses relayed from the upstream are stripped of the host's
// stripResponseHeaders (but keep what gpm itself sets on them), capped at
// maxRelayedInterim, and an upstream that sends more than maxUpstreamInterim
// gets the exchange aborted with a 502.
func TestInterimResponsesStrippedAndBounded(t *testing.T) {
	for _, tc := range []struct {
		name        string
		sent        int
		wantRelayed int
		wantStatus  int
	}{
		{name: "103 early hints", sent: 1, wantRelayed: 1, wantStatus: http.StatusOK},
		{name: "102 then 103", sent: 3, wantRelayed: 3, wantStatus: http.StatusOK},
		{name: "at the relay cap", sent: maxRelayedInterim, wantRelayed: maxRelayedInterim, wantStatus: http.StatusOK},
		{name: "past the relay cap", sent: 40, wantRelayed: maxRelayedInterim, wantStatus: http.StatusOK},
		{name: "at the upstream cap", sent: maxUpstreamInterim, wantRelayed: maxRelayedInterim, wantStatus: http.StatusOK},
		{name: "past the upstream cap", sent: maxUpstreamInterim + 1, wantRelayed: maxRelayedInterim, wantStatus: http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withGlobalErrorPages(t, nil)
			up := upstreamFor(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for i := range tc.sent {
					w.Header().Set("Link", "</style.css>; rel=preload; as=style")
					w.Header().Set("Server", "backend/1.0")
					w.Header().Set("X-Powered-By", "leak")
					code := http.StatusEarlyHints
					if i == 0 && tc.sent > 1 {
						code = http.StatusProcessing
					}
					w.WriteHeader(code)
				}
				_, _ = io.WriteString(w, "final body")
			}))
			cfg := model.Config{
				Middlewares: []model.Middleware{{
					ObjectMeta: model.ObjectMeta{Name: "brand"}, Type: model.MWTypeHeaders,
					Headers: &model.HeadersMiddleware{SetResponse: map[string]string{"Server": "gpm"}},
				}},
				ProxyHosts: []model.ProxyHost{{
					ObjectMeta:           model.ObjectMeta{Name: "front"},
					Domains:              []string{"front.example"},
					Upstream:             up,
					Middlewares:          []string{"brand"},
					StripResponseHeaders: []string{"Server", "x-powered-by"},
				}},
			}
			rt, err := buildRouter(cfg, "", nil)
			if err != nil {
				t.Fatalf("buildRouter: %v", err)
			}
			srv := frontServer(t, rt)

			var got []textproto.MIMEHeader
			req, err := http.NewRequest("GET", srv.URL+"/", nil)
			if err != nil {
				t.Fatal(err)
			}
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
				Got1xxResponse: func(_ int, h textproto.MIMEHeader) error {
					got = append(got, h)
					return nil
				},
			}))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("final status = %d %q, want %d", resp.StatusCode, body, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK && string(body) != "final body" {
				t.Fatalf("final body = %q", body)
			}
			if len(got) != tc.wantRelayed {
				t.Fatalf("client saw %d interim responses, want %d", len(got), tc.wantRelayed)
			}
			for i, h := range got {
				if v := h.Get("X-Powered-By"); v != "" {
					t.Errorf("interim %d X-Powered-By = %q, want it stripped", i, v)
				}
				if v := h.Get("Server"); v != "gpm" {
					t.Errorf("interim %d Server = %q, want gpm's own value, not the upstream's", i, v)
				}
				if v := h.Get("Link"); v == "" {
					t.Errorf("interim %d lost its Link header", i)
				}
			}
		})
	}
}
