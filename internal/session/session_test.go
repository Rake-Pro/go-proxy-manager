package session

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sessions.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestRoundTrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	sess := &Session{
		Subject:   "sub-123",
		Email:     "admin@example.com",
		Name:      "Admin",
		Roles:     []string{"admin", "user"},
		IdP:       "local",
		AMR:       []string{"pwd", "otp"},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	if err := st.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("ID not auto-generated")
	}
	if sess.CSRFToken == "" {
		t.Fatal("CSRFToken not auto-generated")
	}
	if sess.ID == sess.CSRFToken {
		t.Fatal("ID and CSRFToken should be distinct")
	}
	if sess.CreatedAt.IsZero() {
		t.Fatal("CreatedAt not set")
	}

	got, err := st.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Subject != sess.Subject || got.Email != sess.Email || got.Name != sess.Name {
		t.Errorf("scalar fields mismatch: %+v", got)
	}
	if got.IdP != sess.IdP || got.CSRFToken != sess.CSRFToken {
		t.Errorf("idp/csrf mismatch: %+v", got)
	}
	if !equalSlice(got.Roles, sess.Roles) {
		t.Errorf("Roles mismatch: got %v want %v", got.Roles, sess.Roles)
	}
	if !equalSlice(got.AMR, sess.AMR) {
		t.Errorf("AMR mismatch: got %v want %v", got.AMR, sess.AMR)
	}

	if err := st.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := st.Get(ctx, sess.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: got %v want ErrNotFound", err)
	}
	// Delete of absent id is not an error.
	if err := st.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete absent: %v", err)
	}
}

func TestGetExpired(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	sess := &Session{
		Subject:   "sub",
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := st.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := st.Get(ctx, sess.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired Get: got %v want ErrNotFound", err)
	}

	// Row should have been deleted; verify directly.
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, sess.ID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expired row not deleted: count=%d", n)
	}
}

func TestGCMissing(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	live := &Session{Subject: "live", ExpiresAt: time.Now().Add(time.Hour)}
	dead1 := &Session{Subject: "dead1", ExpiresAt: time.Now().Add(-time.Hour)}
	dead2 := &Session{Subject: "dead2", ExpiresAt: time.Now().Add(-time.Minute)}
	for _, s := range []*Session{live, dead1, dead2} {
		if err := st.Create(ctx, s); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	n, err := st.GC(ctx)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if n != 2 {
		t.Fatalf("GC removed %d want 2", n)
	}

	if _, err := st.Get(ctx, live.ID); err != nil {
		t.Fatalf("live session gone: %v", err)
	}
}

func TestTouchExtends(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	sess := &Session{Subject: "sub", ExpiresAt: time.Now().Add(time.Second)}
	if err := st.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newExp := time.Now().Add(time.Hour)
	if err := st.Touch(ctx, sess.ID, newExp); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	got, err := st.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get after Touch: %v", err)
	}
	if got.ExpiresAt.Unix() != newExp.Unix() {
		t.Fatalf("ExpiresAt = %d want %d", got.ExpiresAt.Unix(), newExp.Unix())
	}
}

func TestConcurrency(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := &Session{Subject: "concurrent", ExpiresAt: time.Now().Add(time.Hour)}
			if err := st.Create(ctx, s); err != nil {
				errCh <- err
				return
			}
			if _, err := st.Get(ctx, s.ID); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent op: %v", err)
	}
}

func TestNilSlicesRoundTrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	sess := &Session{Subject: "sub", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := st.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Roles) != 0 || len(got.AMR) != 0 {
		t.Fatalf("expected empty slices, got roles=%v amr=%v", got.Roles, got.AMR)
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
