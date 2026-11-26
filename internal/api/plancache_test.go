package api

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// fakePlan is a plan function with a call counter that blocks each call until
// release is closed (or returns at once when release is nil).
type fakePlan struct {
	calls   atomic.Int32
	release chan struct{}
	err     error
	mu      sync.Mutex
	ctxs    []context.Context
	endErrs []error // ctx.Err() of each call when it returned
}

func (f *fakePlan) plan(ctx context.Context) (any, error) {
	n := f.calls.Add(1)
	f.mu.Lock()
	f.ctxs = append(f.ctxs, ctx)
	f.mu.Unlock()
	if f.release != nil {
		<-f.release
	}
	f.mu.Lock()
	f.endErrs = append(f.endErrs, ctx.Err())
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return int(n), nil
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestPlanCache() (*planCache, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	return &planCache{now: clk.now}, clk
}

// Callers that join while a computation is in flight share it, whether it
// succeeds or fails: one provider query for the whole burst.
func TestPlanCacheCoalescesConcurrentCalls(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		reusable func(any) bool
	}{
		{"success", nil, nil},
		{"error", errors.New("backend unreachable"), nil},
		{"failure inside the plan", nil, func(any) bool { return false }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestPlanCache()
			c.reusable = tc.reusable
			f := &fakePlan{release: make(chan struct{}), err: tc.err}
			calls := make([]*planCall, 10)
			for i := range calls {
				calls[i] = c.join(context.Background(), "h1", f.plan)
			}
			close(f.release)
			for i, call := range calls {
				<-call.done
				if call != calls[0] {
					t.Fatalf("caller %d got its own computation", i)
				}
				if !errors.Is(call.err, tc.err) {
					t.Fatalf("caller %d err = %v, want %v", i, call.err, tc.err)
				}
			}
			if n := f.calls.Load(); n != 1 {
				t.Fatalf("plan computed %d times, want 1", n)
			}
		})
	}
}

// A finished success is reused for the window, a finished error is not reused
// at all, and a HEAD change always recomputes.
func TestPlanCacheReuse(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		advance   time.Duration
		head2     string
		failValue bool // the plan reports a failure inside its value
		wantCalls int32
	}{
		{"success within the window is reused", nil, planReuseWindow - time.Millisecond, "h1", false, 1},
		{"success after the window is recomputed", nil, planReuseWindow, "h1", false, 2},
		{"error is not reused", errors.New("backend unreachable"), 0, "h1", false, 2},
		{"config change recomputes within the window", nil, 0, "h2", false, 2},
		{"failure inside the plan is not reused", nil, 0, "h1", true, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, clk := newTestPlanCache()
			c.reusable = func(any) bool { return !tc.failValue }
			f := &fakePlan{err: tc.err}
			first, err1 := c.get(context.Background(), "h1", f.plan)
			clk.advance(tc.advance)
			second, err2 := c.get(context.Background(), tc.head2, f.plan)
			if n := f.calls.Load(); n != tc.wantCalls {
				t.Fatalf("plan computed %d times, want %d", n, tc.wantCalls)
			}
			if !errors.Is(err1, tc.err) || !errors.Is(err2, tc.err) {
				t.Fatalf("errs = %v, %v, want %v", err1, err2, tc.err)
			}
			if tc.err == nil && tc.wantCalls == 1 && second != first {
				t.Fatalf("reused plan = %v, want %v", second, first)
			}
			if tc.err == nil && tc.wantCalls == 2 && second != 2 {
				t.Fatalf("recomputed plan = %v, want the second computation", second)
			}
		})
	}
}

// The caller that started a computation hanging up must not fail it for the
// others; the computation still runs under its own deadline.
func TestPlanCacheDetachedFromFirstCaller(t *testing.T) {
	c, _ := newTestPlanCache()
	f := &fakePlan{release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() {
		_, err := c.get(ctx, "h1", f.plan)
		firstErr <- err
	}()
	// The second caller joins the same computation.
	for f.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	second := c.join(context.Background(), "h1", f.plan)
	cancel()
	if err := <-firstErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller err = %v, want context.Canceled", err)
	}
	close(f.release)
	<-second.done
	if second.err != nil || second.plan != 1 {
		t.Fatalf("shared result = %v, %v; want 1, nil", second.plan, second.err)
	}
	f.mu.Lock()
	planCtx, endErr := f.ctxs[0], f.endErrs[0]
	f.mu.Unlock()
	if endErr != nil {
		t.Fatalf("computation context ended with %v after the first caller left", endErr)
	}
	dl, ok := planCtx.Deadline()
	if !ok || time.Until(dl) > planTimeout {
		t.Fatalf("computation deadline = %v (set %v), want within %v", dl, ok, planTimeout)
	}
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("plan computed %d times, want 1", n)
	}
}

// A dropped result is recomputed even within the window and for the same HEAD.
func TestPlanCacheDrop(t *testing.T) {
	c, _ := newTestPlanCache()
	f := &fakePlan{}
	_, _ = c.get(context.Background(), "h1", f.plan)
	c.drop()
	if v, _ := c.get(context.Background(), "h1", f.plan); v != 2 {
		t.Fatalf("plan after drop = %v, want the second computation", v)
	}
}

// A panicking plan function becomes a fixed error for its callers instead of
// crashing the process from a goroutine net/http does not recover; the panic
// value and stack go to the log, not to the client.
func TestPlanCacheRecoversPanic(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })

	c, _ := newTestPlanCache()
	_, err := c.get(context.Background(), "h1", func(context.Context) (any, error) { panic("secret-detail") })
	if err == nil || strings.Contains(err.Error(), "secret-detail") {
		t.Fatalf("err = %v, want a fixed message without the panic value", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"level":"error"`) || !strings.Contains(out, "secret-detail") || !strings.Contains(out, "plancache_test.go") {
		t.Fatalf("log = %s, want an error line with the panic value and stack", out)
	}
}
