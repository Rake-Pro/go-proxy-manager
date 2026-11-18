package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// hostBodyWithPasswords is a proxy-host write body whose locations each carry an
// inline basic-auth block with usersPerLoc plaintext passwords.
func hostBodyWithPasswords(locations, usersPerLoc int) string {
	var b strings.Builder
	b.WriteString(`{"domains":["x.example.com"],"upstream":{"scheme":"http","host":"192.0.2.1","port":80},"locations":[`)
	for l := 0; l < locations; l++ {
		if l > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"path":"/p","auth":{"mode":"basic","basic":{"users":[`)
		for u := 0; u < usersPerLoc; u++ {
			if u > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`{"username":"u","password":"p"}`)
		}
		b.WriteString(`]}}}`)
	}
	b.WriteString(`]}`)
	return b.String()
}

func decodeHost(t *testing.T, body string) model.ProxyHost {
	t.Helper()
	var h model.ProxyHost
	if err := json.Unmarshal([]byte(body), &h); err != nil {
		t.Fatal(err)
	}
	return h
}

// The plaintext passwords of one write are counted across the host block and
// every location before any hashing, so many blocks each under the per-block
// cap cannot add up to an unbounded amount of bcrypt work.
func TestHostWriteCapsPlaintextPasswordsPerRequest(t *testing.T) {
	body := hostBodyWithPasswords(3, 30) // 90 passwords, every block under 64
	h := decodeHost(t, body)
	err := applyHostBasicAuthPasswords(context.Background(), []byte(body), &h)
	if err == nil || !strings.Contains(err.Error(), "plaintext passwords") {
		t.Fatalf("err = %v, want the per-request cap", err)
	}
	for i, l := range h.Locations {
		for _, u := range l.Auth.Basic.Users {
			if u.PasswordHash != "" {
				t.Fatalf("location %d was hashed before the cap was checked", i)
			}
		}
	}
}

// A cancelled request stops hashing.
func TestHostWriteStopsHashingWhenTheRequestIsCancelled(t *testing.T) {
	body := hostBodyWithPasswords(2, 2)
	h := decodeHost(t, body)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := applyHostBasicAuthPasswords(ctx, []byte(body), &h)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if h.Locations[0].Auth.Basic.Users[0].PasswordHash != "" {
		t.Fatal("a password was hashed after the request was cancelled")
	}
}
