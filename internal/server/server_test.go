package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/login", nil))

	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "frame-ancestors 'none'",
		"Referrer-Policy":         "same-origin",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	// HSTS must NOT be set by the admin server: the data plane is the TLS edge and
	// owns it; emitting it here duplicated the header on the proxied admin path.
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("admin server must not set Strict-Transport-Security, got %q", got)
	}
}
