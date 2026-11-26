package dataplane

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// connCounts counts the connections a test server accepted and closed.
type connCounts struct{ opened, closed atomic.Int32 }

func countingServer(t *testing.T, h http.Handler) (*httptest.Server, *connCounts) {
	t.Helper()
	cc := &connCounts{}
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		switch s {
		case http.StateNew:
			cc.opened.Add(1)
		case http.StateClosed:
			cc.closed.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, cc
}

func waitForCount(c *atomic.Int32, want int32, d time.Duration) bool {
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if c.Load() >= want {
			return true
		}
	}
	return c.Load() >= want
}

// A reload closes the idle connections of the pools the replaced router built
// for itself (per-host transport clones, auth-request and bouncer clients) and
// leaves the shared data-plane transport's pool alone.
func TestReloadClosesReplacedIdleConnections(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
	for _, tc := range []struct {
		name       string
		build      func(watched, other *httptest.Server) model.Config
		wantClosed bool
	}{
		{
			name: "shared transport is not closed",
			build: func(watched, _ *httptest.Server) model.Config {
				return model.Config{ProxyHosts: []model.ProxyHost{{
					ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"},
					Upstream: upstreamFromURL(t, watched.URL),
				}}}
			},
		},
		{
			name: "per-host transport clone",
			build: func(watched, _ *httptest.Server) model.Config {
				return model.Config{ProxyHosts: []model.ProxyHost{{
					ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"},
					Upstream: upstreamFromURL(t, watched.URL),
					Timeouts: &model.HostTimeouts{ConnectSeconds: 5},
				}}}
			},
			wantClosed: true,
		},
		{
			name: "auth-request client",
			build: func(watched, other *httptest.Server) model.Config {
				return model.Config{
					IdentityProviders: []model.IdentityProvider{{
						ObjectMeta: model.ObjectMeta{Name: "ar"}, Type: model.IdPTypeAuthRequest,
						AuthRequest: &model.AuthRequestSpec{OutpostURL: watched.URL},
					}},
					ProxyHosts: []model.ProxyHost{{
						ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"},
						Upstream: upstreamFromURL(t, other.URL),
						Auth:     &model.AuthMiddleware{IdentityProvider: "ar", Mode: model.AuthModeAuthRequest},
					}},
				}
			},
			wantClosed: true,
		},
		{
			name: "bouncer client",
			build: func(watched, other *httptest.Server) model.Config {
				return model.Config{
					Middlewares: []model.Middleware{{
						ObjectMeta: model.ObjectMeta{Name: "bnc"}, Type: model.MWTypeBouncer,
						Bouncer: &model.BouncerMiddleware{Provider: model.BouncerProviderHTTP, URL: watched.URL + "/check"},
					}},
					ProxyHosts: []model.ProxyHost{{
						ObjectMeta: model.ObjectMeta{Name: "app"}, Domains: []string{"app.example.com"},
						Upstream:    upstreamFromURL(t, other.URL),
						Middlewares: []string{"bnc"},
					}},
				}
			},
			wantClosed: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			watched, cc := countingServer(t, ok)
			other, _ := countingServer(t, ok)
			cfg := tc.build(watched, other)
			s, addr := tunnelServer(t, cfg)
			client := &http.Client{Transport: &http.Transport{}}
			t.Cleanup(client.CloseIdleConnections)
			get := func() {
				t.Helper()
				req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
				req.Host = "app.example.com"
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("status = %d, want 200", resp.StatusCode)
				}
			}

			get()
			if !waitForCount(&cc.opened, 1, 2*time.Second) {
				t.Fatal("the watched server saw no connection")
			}
			if err := s.Reload(cloneConfig(t, cfg)); err != nil {
				t.Fatal(err)
			}
			if tc.wantClosed {
				if !waitForCount(&cc.closed, 1, 2*time.Second) {
					t.Fatal("the replaced router's idle connection is still open after the reload")
				}
				return
			}
			time.Sleep(200 * time.Millisecond)
			if n := cc.closed.Load(); n != 0 {
				t.Fatalf("%d shared-pool connection(s) closed by the reload, want none", n)
			}
			get()
			if n := cc.opened.Load(); n != 1 {
				t.Fatalf("the request after the reload opened a new connection (%d total), want the pooled one reused", n)
			}
		})
	}
}

// Closing the replaced pools never interrupts an established upgraded
// connection riding one of them.
func TestReloadKeepsTunnelOnReplacedClone(t *testing.T) {
	up, closeFn := hijackEcho(t)
	defer closeFn()
	cfg := tunnelConfig(up)
	cfg.ProxyHosts[0].Timeouts = &model.HostTimeouts{ConnectSeconds: 5}
	s, addr := tunnelServer(t, cfg)
	c, br := openTunnel(t, addr, "ws.example.com")
	tunnelEcho(t, c, br)
	if err := s.Reload(cloneConfig(t, cfg)); err != nil {
		t.Fatal(err)
	}
	tunnelEcho(t, c, br)
}

type fakeIdlePool struct{ closed int }

func (f *fakeIdlePool) CloseIdleConnections() { f.closed++ }

// Only pools the old router owned and the new one does not are closed.
func TestCloseReplacedIdleSkipsReusedPools(t *testing.T) {
	gone, kept := &fakeIdlePool{}, &fakeIdlePool{}
	closeReplacedIdle(&router{owned: []idleCloser{gone, kept}}, &router{owned: []idleCloser{kept}})
	if gone.closed != 1 || kept.closed != 0 {
		t.Fatalf("closed: replaced=%d reused=%d, want 1 and 0", gone.closed, kept.closed)
	}
}
