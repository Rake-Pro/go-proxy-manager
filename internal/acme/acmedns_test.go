package acme

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubCNAME is a cnameLookuper that returns a fixed answer.
type stubCNAME struct {
	target string
	err    error
	calls  int
}

func (s *stubCNAME) LookupCNAME(ctx context.Context, host string) (string, error) {
	s.calls++
	return s.target, s.err
}

func testACMEDNSConfig(baseURL string) ACMEDNSConfig {
	return ACMEDNSConfig{
		BaseURL:   baseURL,
		Username:  "c0f8ba55-0000-4000-8000-000000000001",
		Password:  "super-secret-password",
		Subdomain: "d420c923-0000-4000-8000-000000000002",
		// The stub server is an httptest listener on loopback, which a real
		// config has to opt into (see ValidateOutboundBaseURL).
		AllowInsecureLocal: true,
	}
}

func TestACMEDNSPresent(t *testing.T) {
	var (
		gotPath string
		gotUser string
		gotKey  string
		gotBody acmeDNSUpdate
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		gotUser = r.Header.Get("X-Api-User")
		gotKey = r.Header.Get("X-Api-Key")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"txt":"`+gotBody.TXT+`"}`)
	}))
	defer srv.Close()

	cfg := testACMEDNSConfig(srv.URL)
	resolver := &stubCNAME{target: cfg.Subdomain + ".acme-dns.example.com."}
	s, err := NewACMEDNSSolver(cfg, WithACMEDNSResolver(resolver))
	if err != nil {
		t.Fatalf("NewACMEDNSSolver() = %v", err)
	}
	if err := s.Present(context.Background(), "_acme-challenge.example.com", "challenge-value"); err != nil {
		t.Fatalf("Present() = %v", err)
	}
	if gotPath != "POST /update" {
		t.Errorf("request = %q, want %q", gotPath, "POST /update")
	}
	if gotUser != cfg.Username || gotKey != cfg.Password {
		t.Errorf("auth headers = %q/%q, want the username/password", gotUser, gotKey)
	}
	if gotBody.Subdomain != cfg.Subdomain || gotBody.TXT != "challenge-value" {
		t.Errorf("body = %+v, want the account subdomain and the challenge value", gotBody)
	}
	if resolver.calls != 1 {
		t.Errorf("CNAME pre-flight ran %d times, want 1", resolver.calls)
	}
}

func TestACMEDNSCleanUpIsNoOp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("CleanUp made an HTTP request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	s, err := NewACMEDNSSolver(testACMEDNSConfig(srv.URL))
	if err != nil {
		t.Fatalf("NewACMEDNSSolver() = %v", err)
	}
	if err := s.CleanUp(context.Background(), "_acme-challenge.example.com", "v"); err != nil {
		t.Fatalf("CleanUp() = %v", err)
	}
}

// A missing or wrong CNAME delegation is a warning, never a failure: the
// resolver in front of gpm may simply not see the record yet.
func TestACMEDNSPreflightNeverFails(t *testing.T) {
	cases := []struct {
		name     string
		resolver *stubCNAME
	}{
		{"no cname at all", &stubCNAME{target: "_acme-challenge.example.com."}},
		{"cname to the wrong account", &stubCNAME{target: "someone-else.acme-dns.example.com."}},
		{"resolver error", &stubCNAME{err: errors.New("no such host")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var called bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				io.WriteString(w, `{}`)
			}))
			defer srv.Close()
			s, err := NewACMEDNSSolver(testACMEDNSConfig(srv.URL), WithACMEDNSResolver(tc.resolver))
			if err != nil {
				t.Fatalf("NewACMEDNSSolver() = %v", err)
			}
			if err := s.Present(context.Background(), "_acme-challenge.example.com", "v"); err != nil {
				t.Fatalf("Present() = %v, want the pre-flight to be warning-only", err)
			}
			if !called {
				t.Error("Present() did not call the acme-dns API")
			}
		})
	}
}

func TestACMEDNSUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"forbidden"}`)
	}))
	defer srv.Close()

	cfg := testACMEDNSConfig(srv.URL)
	s, err := NewACMEDNSSolver(cfg, WithACMEDNSResolver(&stubCNAME{target: cfg.Subdomain + ".acme-dns.example.com."}))
	if err != nil {
		t.Fatalf("NewACMEDNSSolver() = %v", err)
	}
	err = s.Present(context.Background(), "_acme-challenge.example.com", "v")
	if err == nil {
		t.Fatal("Present() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "credentials rejected") {
		t.Errorf("Present() = %q, want it to explain that the credentials were rejected", err)
	}
}

func TestNewACMEDNSSolverValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*ACMEDNSConfig)
		wantErr string
	}{
		{"missing baseURL", func(c *ACMEDNSConfig) { c.BaseURL = "" }, "baseURL is required"},
		{"bad baseURL", func(c *ACMEDNSConfig) { c.BaseURL = "acme-dns.example.com" }, "must be an http or https URL"},
		{"missing username", func(c *ACMEDNSConfig) { c.Username = "" }, "username is required"},
		{"missing password", func(c *ACMEDNSConfig) { c.Password = "" }, "password is required"},
		{"missing subdomain", func(c *ACMEDNSConfig) { c.Subdomain = "" }, "subdomain is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testACMEDNSConfig("https://acme-dns.example.com")
			tc.mutate(&cfg)
			_, err := NewACMEDNSSolver(cfg)
			if err == nil {
				t.Fatalf("NewACMEDNSSolver() = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("NewACMEDNSSolver() = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
