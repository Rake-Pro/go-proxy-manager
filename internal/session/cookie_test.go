package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetSessionCookie(t *testing.T) {
	tests := []struct {
		name   string
		secure bool
	}{
		{"secure", true},
		{"insecure", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			exp := time.Now().Add(time.Hour)
			SetSessionCookie(rec, "sid", "the-id", exp, tt.secure)

			c := readCookie(t, rec, "sid")
			if c.Value != "the-id" {
				t.Errorf("Value = %q want the-id", c.Value)
			}
			if c.Path != "/" {
				t.Errorf("Path = %q want /", c.Path)
			}
			if !c.HttpOnly {
				t.Error("HttpOnly not set")
			}
			if c.Secure != tt.secure {
				t.Errorf("Secure = %v want %v", c.Secure, tt.secure)
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("SameSite = %v want Lax", c.SameSite)
			}
			if c.MaxAge <= 0 {
				t.Errorf("MaxAge = %d want > 0", c.MaxAge)
			}
		})
	}
}

func TestClearSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearSessionCookie(rec, "sid", true)

	c := readCookie(t, rec, "sid")
	if c.Value != "" {
		t.Errorf("Value = %q want empty", c.Value)
	}
	if c.MaxAge != -1 {
		t.Errorf("MaxAge = %d want -1", c.MaxAge)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q want /", c.Path)
	}
}

func readCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	resp := rec.Result()
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cookie %q not found", name)
	return nil
}
