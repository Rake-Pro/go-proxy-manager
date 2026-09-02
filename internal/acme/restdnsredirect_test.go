package acme

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRESTAPIDoesNotFollowRedirects: the REST DNS clients send provider
// credentials in CUSTOM headers (acme-dns uses X-Api-User / X-Api-Key), and Go's
// stdlib only strips Authorization/Cookie across a host change - so a followed
// redirect would replay the credentials to whatever host the 3xx named. The
// redirect is surfaced as an error and the second host is never contacted.
func TestRESTAPIDoesNotFollowRedirects(t *testing.T) {
	var sawCredentials bool
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Api-User") != "" {
			sawCredentials = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, attacker.URL+"/update", http.StatusFound)
	}))
	defer provider.Close()

	api := newRESTAPI("acme-dns", provider.URL, func(r *http.Request) {
		r.Header.Set("X-Api-User", "user-uuid")
		r.Header.Set("X-Api-Key", "super-secret-password")
	})
	err := api.do(context.Background(), http.MethodPost, "/update", map[string]string{"txt": "v"}, nil)
	if err == nil {
		t.Fatal("do() = nil, want the redirect surfaced as an error")
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("do() = %v, want an error naming the redirect", err)
	}
	if sawCredentials {
		t.Fatal("the provider's credentials were replayed to the redirect target")
	}
}
