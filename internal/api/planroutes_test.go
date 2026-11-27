package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/api"
	"github.com/Rake-Pro/go-proxy-manager/internal/dnssync"
	"github.com/Rake-Pro/go-proxy-manager/internal/docker"
	"github.com/Rake-Pro/go-proxy-manager/internal/k8s"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// Each dry-run route reuses a successful plan for repeated reads, recomputes
// once a config commit lands, and never reuses a failure.
func TestPlanRoutesReuseAndInvalidate(t *testing.T) {
	tests := []struct {
		route string
		wire  func(*api.Deps, func(context.Context) (any, error))
	}{
		{"/dns-sync/plan", func(d *api.Deps, f func(context.Context) (any, error)) { d.DNSSyncPlan = f }},
		{"/ingress-discovery/plan", func(d *api.Deps, f func(context.Context) (any, error)) { d.IngressDiscoveryPlan = f }},
		{"/docker-discovery/plan", func(d *api.Deps, f func(context.Context) (any, error)) { d.DockerDiscoveryPlan = f }},
	}
	for _, tc := range tests {
		t.Run(tc.route, func(t *testing.T) {
			dir := t.TempDir()
			st := store.New(dir, store.NewExecGit(dir))
			if err := st.Init(context.Background()); err != nil {
				t.Fatalf("store init: %v", err)
			}
			var calls atomic.Int32
			var fail atomic.Bool
			d := api.Deps{Store: st}
			tc.wire(&d, func(context.Context) (any, error) {
				n := calls.Add(1)
				if fail.Load() {
					return nil, errors.New("backend unreachable")
				}
				return map[string]any{"computation": n}, nil
			})
			h := api.New(d)
			get := func(wantStatus int) string {
				t.Helper()
				w := do(t, h, "GET", tc.route, "")
				if w.Code != wantStatus {
					t.Fatalf("GET %s = %d, want %d: %s", tc.route, w.Code, wantStatus, w.Body.String())
				}
				return w.Body.String()
			}
			want := func(n int32) {
				t.Helper()
				if got := calls.Load(); got != n {
					t.Fatalf("plan computed %d times, want %d", got, n)
				}
			}

			first := get(http.StatusOK)
			if again := get(http.StatusOK); again != first {
				t.Fatalf("reused plan = %s, want %s", again, first)
			}
			want(1)

			// A commit moves HEAD: the next read is a fresh plan.
			body := `{"domains":["app.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080}}`
			if w := do(t, h, "PUT", "/proxy-hosts/app", body); w.Code != http.StatusOK {
				t.Fatalf("PUT = %d: %s", w.Code, w.Body.String())
			}
			if after := get(http.StatusOK); after == first {
				t.Fatalf("plan after a config change = %s, served from before the change", after)
			}
			want(2)

			// Failures are recomputed on every read. The commit first moves HEAD
			// past the success that would otherwise still be reused.
			if w := do(t, h, "PUT", "/proxy-hosts/app2", strings.Replace(body, "app.example.com", "app2.example.com", 1)); w.Code != http.StatusOK {
				t.Fatalf("PUT = %d: %s", w.Code, w.Body.String())
			}
			fail.Store(true)
			get(http.StatusBadGateway)
			get(http.StatusBadGateway)
			want(4)
		})
	}
}

// A plan that reports a failure inside its value (a DNS backend that could not
// be read) is answered 200 like before but never reused; a reconcile through
// the API drops the cached plan, and a plan request during that reconcile gets
// the 409 it got before the cache existed.
func TestPlanRoutesFailureInValueAndReconcile(t *testing.T) {
	tests := []struct {
		route      string
		reconcile  string
		inProgress error
		failed     any
		wire       func(*api.Deps, func(context.Context) (any, error), func(context.Context) error)
	}{
		{
			"/dns-sync/plan", "/dns-sync/reconcile", dnssync.ErrReconcileInProgress,
			dnssync.Plan{Pihole: dnssync.BackendPlan{Enabled: true, Error: "pihole unreachable"}},
			func(d *api.Deps, p func(context.Context) (any, error), r func(context.Context) error) {
				d.DNSSyncPlan, d.DNSSyncReconcile = p, r
			},
		},
		{
			"/ingress-discovery/plan", "/ingress-discovery/reconcile", k8s.ErrReconcileInProgress,
			k8s.Plan{Enabled: true, Error: "cluster unreachable"},
			func(d *api.Deps, p func(context.Context) (any, error), r func(context.Context) error) {
				d.IngressDiscoveryPlan, d.IngressDiscoveryReconcile = p, r
			},
		},
		{
			"/docker-discovery/plan", "/docker-discovery/reconcile", docker.ErrReconcileInProgress,
			docker.Plan{Enabled: true, Error: "engine unreachable"},
			func(d *api.Deps, p func(context.Context) (any, error), r func(context.Context) error) {
				d.DockerDiscoveryPlan, d.DockerDiscoveryReconcile = p, r
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.route, func(t *testing.T) {
			dir := t.TempDir()
			st := store.New(dir, store.NewExecGit(dir))
			if err := st.Init(context.Background()); err != nil {
				t.Fatalf("store init: %v", err)
			}
			var calls atomic.Int32
			var failInValue, reconciling atomic.Bool
			var h http.Handler
			duringRun := 0
			d := api.Deps{Store: st}
			tc.wire(&d,
				func(context.Context) (any, error) {
					n := calls.Add(1)
					if reconciling.Load() {
						return nil, tc.inProgress
					}
					if failInValue.Load() {
						return tc.failed, nil
					}
					return map[string]any{"computation": n}, nil
				},
				func(context.Context) error {
					reconciling.Store(true)
					defer reconciling.Store(false)
					duringRun = do(t, h, "GET", tc.route, "").Code
					return nil
				})
			h = api.New(d)
			want := func(n int32) {
				t.Helper()
				if got := calls.Load(); got != n {
					t.Fatalf("plan computed %d times, want %d", got, n)
				}
			}

			failInValue.Store(true)
			for range 2 {
				if w := do(t, h, "GET", tc.route, ""); w.Code != http.StatusOK {
					t.Fatalf("GET %s = %d, want 200: %s", tc.route, w.Code, w.Body.String())
				}
			}
			want(2)

			failInValue.Store(false)
			do(t, h, "GET", tc.route, "")
			want(3)
			if w := do(t, h, "POST", tc.reconcile, ""); w.Code != http.StatusOK {
				t.Fatalf("POST %s = %d: %s", tc.reconcile, w.Code, w.Body.String())
			}
			if duringRun != http.StatusConflict {
				t.Fatalf("plan during the reconcile = %d, want 409", duringRun)
			}
			want(4)
			do(t, h, "GET", tc.route, "")
			want(5)
		})
	}
}
