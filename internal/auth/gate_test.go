package auth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A burst of parallel attempts gets exactly the limit, never more: the gate
// reserves the attempt under the lock before the credential is checked.
func TestLoginGateReservesBeforeVerify(t *testing.T) {
	g := newRateGate(time.Minute, 5, 16)
	var wg sync.WaitGroup
	var allowed int32
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g.tryBegin("k") {
				atomic.AddInt32(&allowed, 1)
			}
		}()
	}
	wg.Wait()
	if allowed != 5 {
		t.Fatalf("64 parallel attempts: %d passed the gate, want exactly 5", allowed)
	}
	if g.tryBegin("k") {
		t.Fatal("a key at its limit must stay refused")
	}
	g.clear("k")
	if !g.tryBegin("k") {
		t.Fatal("a cleared key must be admitted again")
	}
}

// A saturated map full of expired entries must not keep every new client out.
func TestLoginGateSweepsExpiredWhenSaturated(t *testing.T) {
	const window = 40 * time.Millisecond
	g := newRateGate(window, 1, 3)
	for _, k := range []string{"a", "b", "c"} {
		if !g.tryBegin(k) {
			t.Fatalf("key %s must be admitted", k)
		}
	}
	if g.tryBegin("d") {
		t.Fatal("a map full of live entries must fail closed")
	}
	if g.remaining("d") == 0 {
		t.Fatal("a refused new client must be told to wait")
	}
	time.Sleep(window + 30*time.Millisecond)
	if g.remaining("e") != 0 {
		t.Fatal("once every entry has expired a new client must not be told to wait")
	}
	if !g.tryBegin("d") {
		t.Fatal("expired entries must be swept so a new client gets in")
	}
}

func TestAbandonLoginDropsPending(t *testing.T) {
	a := NewAuthenticator(Options{Store: testStore(t)})
	a.pmu.Lock()
	a.pending["st"] = pendingLogin{expires: time.Now().Add(time.Minute)}
	a.pmu.Unlock()
	a.AbandonLogin("st")
	a.AbandonLogin("")
	a.pmu.Lock()
	_, ok := a.pending["st"]
	a.pmu.Unlock()
	if ok {
		t.Fatal("an abandoned login must be gone")
	}
}
