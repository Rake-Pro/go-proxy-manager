package server

import (
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/clientip"
)

// TestClientIPKeyUsesTrustedProxies covers the admin-side lockout key. The
// supported "admin UI behind a gpm proxy host" deployment delivers every login
// attempt from one source address, so keying on RemoteAddr collapsed the login
// lockout, the TOTP throttle and the pending-login cap into a single global
// bucket - one attacker could lock out every administrator. The key now derives
// the client exactly the way the data plane does, from settings.trustedProxies.
func TestClientIPKeyUsesTrustedProxies(t *testing.T) {
	tests := []struct {
		name     string
		trusted  []string
		remote   string
		xff      string
		want     string
		wantSame string // a second request that must NOT share the bucket
	}{
		{
			name:     "behind a trusted proxy each forwarded client gets its own bucket",
			trusted:  []string{"192.0.2.10/32"},
			remote:   "192.0.2.10:4444",
			xff:      "10.1.2.3",
			want:     "10.1.2.3",
			wantSame: "10.1.2.4",
		},
		{
			name:    "an untrusted peer's X-Forwarded-For is ignored",
			trusted: nil,
			remote:  "198.51.100.7:5555",
			xff:     "10.1.2.3",
			want:    "198.51.100.7",
		},
		{
			name:    "an untrusted peer cannot mint a fresh bucket per attempt",
			trusted: []string{"192.0.2.10/32"},
			remote:  "198.51.100.7:5555",
			xff:     "10.9.9.9",
			want:    "198.51.100.7",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientip.SetTrusted(tc.trusted)
			t.Cleanup(func() { clientip.SetTrusted(nil) })

			r := httptest.NewRequest("POST", "/auth/local", nil)
			r.RemoteAddr = tc.remote
			r.Header.Set("X-Forwarded-For", tc.xff)
			if got := clientIPKey(r); got != tc.want {
				t.Fatalf("clientIPKey = %q, want %q", got, tc.want)
			}
			if tc.wantSame == "" {
				return
			}
			r2 := httptest.NewRequest("POST", "/auth/local", nil)
			r2.RemoteAddr = tc.remote
			r2.Header.Set("X-Forwarded-For", tc.wantSame)
			if got := clientIPKey(r2); got == tc.want {
				t.Fatalf("two forwarded clients share the lockout bucket %q", got)
			}
		})
	}
}
