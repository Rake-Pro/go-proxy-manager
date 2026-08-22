package acme

import (
	"testing"
	"time"
)

func TestHTTP01StorePutGetDelete(t *testing.T) {
	s := NewHTTP01Store()

	if _, ok := s.KeyAuth("nope"); ok {
		t.Error("unknown token should not resolve")
	}
	if _, ok := s.KeyAuth(""); ok {
		t.Error("empty token should not resolve")
	}

	s.Put("tok", "tok.keyauth", 0)
	ka, ok := s.KeyAuth("tok")
	if !ok || ka != "tok.keyauth" {
		t.Fatalf("KeyAuth = %q,%v want the key authorization", ka, ok)
	}
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1", s.Len())
	}

	// An empty token is never stored.
	s.Put("", "x", 0)
	if s.Len() != 1 {
		t.Errorf("empty token stored: Len = %d", s.Len())
	}

	s.Delete("tok", "never-there")
	if _, ok := s.KeyAuth("tok"); ok {
		t.Error("token should be gone after Delete")
	}
	if s.Len() != 0 {
		t.Errorf("Len = %d, want 0", s.Len())
	}
}

func TestHTTP01StoreExpiry(t *testing.T) {
	now := time.Now()
	s := NewHTTP01Store()
	s.now = func() time.Time { return now }

	s.Put("tok", "ka", time.Minute)
	if _, ok := s.KeyAuth("tok"); !ok {
		t.Fatal("token should resolve inside its TTL")
	}

	now = now.Add(time.Minute) // exactly at expiry counts as expired
	if _, ok := s.KeyAuth("tok"); ok {
		t.Error("expired token should not resolve")
	}
	if s.Len() != 0 {
		t.Errorf("expired entry not dropped: Len = %d", s.Len())
	}
}
