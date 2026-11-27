package acme

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type webhookRequest struct {
	method, path, query, auth, contentType, body string
}

func webhookServer(t *testing.T, status int, reply string) (*httptest.Server, *[]webhookRequest) {
	t.Helper()
	var got []webhookRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, webhookRequest{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(b)})
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestWebhookDNSContract(t *testing.T) {
	cases := []struct {
		name, suffix, wantPath, wantQuery string
	}{
		{"path and query", "/hooks/dns?zone=example", "/hooks/dns", "zone=example"},
		{"trailing slash kept", "/hooks/dns/", "/hooks/dns/", ""},
		{"no path", "", "/", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := webhookServer(t, http.StatusNoContent, "")
			s, err := NewWebhookDNSSolver(WebhookDNSConfig{URL: srv.URL + tc.suffix, Token: "hook-token", AllowInsecureLocal: true})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := s.Present(ctx, "_acme-challenge.example.com", "value-1"); err != nil {
				t.Fatalf("Present: %v", err)
			}
			if err := s.CleanUp(ctx, "_acme-challenge.example.com.", "value-1"); err != nil {
				t.Fatalf("CleanUp: %v", err)
			}
			want := []webhookRequest{
				{"POST", tc.wantPath, tc.wantQuery, "Bearer hook-token", "application/json", `{"action":"present","fqdn":"_acme-challenge.example.com.","value":"value-1"}`},
				{"POST", tc.wantPath, tc.wantQuery, "Bearer hook-token", "application/json", `{"action":"cleanup","fqdn":"_acme-challenge.example.com.","value":"value-1"}`},
			}
			if len(*got) != len(want) {
				t.Fatalf("requests = %+v, want %d", *got, len(want))
			}
			for i := range want {
				if (*got)[i] != want[i] {
					t.Errorf("request %d = %+v, want %+v", i, (*got)[i], want[i])
				}
			}
		})
	}
}

// TestWebhookDNSStatus: every 2xx is success, anything else
// is an error with the status and a bounded slice of the body.
func TestWebhookDNSStatus(t *testing.T) {
	for _, code := range []int{200, 201, 202, 204} {
		srv, _ := webhookServer(t, code, "ok")
		s, _ := NewWebhookDNSSolver(WebhookDNSConfig{URL: srv.URL, Token: "t", AllowInsecureLocal: true})
		if err := s.Present(context.Background(), "_acme-challenge.example.com", "v"); err != nil {
			t.Errorf("status %d: %v, want success", code, err)
		}
	}

	long := "refused: " + strings.Repeat("x", 5000)
	for _, code := range []int{400, 401, 404, 500, 502} {
		srv, _ := webhookServer(t, code, long)
		s, _ := NewWebhookDNSSolver(WebhookDNSConfig{URL: srv.URL + "/hook", Token: "t", AllowInsecureLocal: true})
		err := s.CleanUp(context.Background(), "_acme-challenge.example.com", "v")
		if err == nil {
			t.Fatalf("status %d: want an error", code)
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, "cleanup _acme-challenge.example.com.: webhook: POST the configured url: ") || !strings.Contains(msg, ": "+strconv.Itoa(code)+": ") || !strings.Contains(msg, "refused: xxx") {
			t.Errorf("status %d: error = %.200q, want the action, fqdn, status and body", code, msg)
		}
		if len(msg) > 400 {
			t.Errorf("status %d: error is %d bytes, want the body bounded", code, len(msg))
		}
	}
}

// TestWebhookDNSDoesNotFollowRedirects: the redirect target never sees the
// request, so the bearer token stays with the configured endpoint.
func TestWebhookDNSDoesNotFollowRedirects(t *testing.T) {
	for _, code := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		var reached bool
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
			if r.Header.Get("Authorization") != "" {
				t.Errorf("status %d: the bearer token reached the redirect target", code)
			}
		}))
		hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/steal", code)
		}))
		s, err := NewWebhookDNSSolver(WebhookDNSConfig{URL: hook.URL + "/hook", Token: "hook-token", AllowInsecureLocal: true})
		if err != nil {
			t.Fatal(err)
		}
		err = s.Present(context.Background(), "_acme-challenge.example.com", "v")
		if err == nil || !strings.Contains(err.Error(), "redirects are not followed") {
			t.Errorf("status %d: err = %v, want the redirect refused", code, err)
		}
		if reached {
			t.Errorf("status %d: the redirect target was contacted", code)
		}
		hook.Close()
		target.Close()
	}
}

func TestNewWebhookDNSSolverValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  WebhookDNSConfig
		want string
	}{
		{"missing url", WebhookDNSConfig{Token: "t"}, "webhook: url is required"},
		{"missing token", WebhookDNSConfig{URL: "https://dns-hook.example.com/acme"}, "webhook: token is required"},
		{"blank token", WebhookDNSConfig{URL: "https://dns-hook.example.com/acme", Token: "  "}, "webhook: token is required"},
		{"no scheme", WebhookDNSConfig{URL: "dns-hook.example.com/acme", Token: "t"}, "webhook: url: is not an http or https URL (the value is not shown, it may hold a credential)"},
		{"other scheme", WebhookDNSConfig{URL: "ftp://dns-hook.example.com/acme", Token: "t"}, "webhook: url: is not an http or https URL"},
		{"loopback without opt-in", WebhookDNSConfig{URL: "http://127.0.0.1:8053/hook", Token: "t"}, `webhook: url: points at a loopback address; set config.allowInsecureLocal: "true" if the service really runs beside gpm`},
		{"link-local", WebhookDNSConfig{URL: "http://169.254.169.254/hook", Token: "t"}, "webhook: url: points at a link-local address, which is the cloud metadata range and never a DNS provider"},
		{"unspecified", WebhookDNSConfig{URL: "http://0.0.0.0/hook", Token: "t"}, "webhook: url: points at an unspecified or multicast address, which is not a destination gpm can send credentials to"},
		{"user info", WebhookDNSConfig{URL: "https://user:pass@dns-hook.example.com/acme", Token: "t"}, "webhook: url: must not contain user info (user:password@); it is never sent, put the credential in token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewWebhookDNSSolver(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestNewWebhookDNSSolverWarnsOnPlainHTTP(t *testing.T) {
	cases := []struct {
		url      string
		local    bool
		wantWarn bool
	}{
		{"http://dns-hook.example.com/acme", false, true},
		{"http://192.0.2.10/acme", false, true},
		{"https://dns-hook.example.com/acme", false, false},
		{"http://127.0.0.1:8053/acme", true, false},
		{"http://[::1]:8053/acme", true, false},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		prev := log.Logger
		log.Logger = zerolog.New(&buf)
		if _, err := NewWebhookDNSSolver(WebhookDNSConfig{URL: tc.url, Token: "t", AllowInsecureLocal: tc.local}); err != nil {
			t.Fatalf("%s: %v", tc.url, err)
		}
		log.Logger = prev
		if got := strings.Contains(buf.String(), "url is plain http"); got != tc.wantWarn {
			t.Errorf("%s: warned = %v, want %v", tc.url, got, tc.wantWarn)
		}
	}
}

// TestWebhookDNSErrorsOmitQuery: a key in the URL's query never reaches an
// error, which ends up in the certificate's last error, notifications and
// logs. The request itself still carries the query. (User info is refused
// when the solver is built.)
func TestWebhookDNSErrorsOmitQuery(t *testing.T) {
	secrets := []string{"SEKRIT", "key="}
	check := func(kind string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: want an error", kind)
		}
		for _, s := range secrets {
			if strings.Contains(err.Error(), s) {
				t.Errorf("%s: error %q contains %q", kind, err, s)
			}
		}
		if !strings.Contains(err.Error(), "webhook: POST the configured url") {
			t.Errorf("%s: error %q does not name the path", kind, err)
		}
	}

	srv, got := webhookServer(t, http.StatusInternalServerError, "nope")
	withSecrets := srv.URL + "/hook?key=SEKRIT"
	s, err := NewWebhookDNSSolver(WebhookDNSConfig{URL: withSecrets, Token: "t", AllowInsecureLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	check("status error", s.Present(context.Background(), "_acme-challenge.example.com", "v"))
	if len(*got) != 1 || (*got)[0].query != "key=SEKRIT" {
		t.Errorf("requests = %+v, want the query sent", *got)
	}

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer redirect.Close()
	s, _ = NewWebhookDNSSolver(WebhookDNSConfig{URL: redirect.URL + "/hook?key=SEKRIT", Token: "t", AllowInsecureLocal: true})
	check("redirect error", s.Present(context.Background(), "_acme-challenge.example.com", "v"))

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL + "/hook?key=SEKRIT"
	closed.Close()
	s, _ = NewWebhookDNSSolver(WebhookDNSConfig{URL: closedURL, Token: "t", AllowInsecureLocal: true})
	check("transport error", s.CleanUp(context.Background(), "_acme-challenge.example.com", "v"))
}

// TestWebhookDNSLinkLocalRefusedAtConnect: allowInsecureLocal skips the
// address checks at validation time, so a link-local URL builds a solver; the
// hardened client still refuses the destination when it dials. The guard runs
// before the connect call, so no packet is sent.
func TestWebhookDNSLinkLocalRefusedAtConnect(t *testing.T) {
	for _, u := range []string{"http://169.254.169.254/hook", "http://[fe80::1]:8080/hook"} {
		s, err := NewWebhookDNSSolver(WebhookDNSConfig{URL: u, Token: "t", AllowInsecureLocal: true})
		if err != nil {
			t.Fatalf("%s with allowInsecureLocal: %v, want the solver built (validation skips the address checks)", u, err)
		}
		err = s.Present(context.Background(), "_acme-challenge.example.com", "v")
		if err == nil || !strings.Contains(err.Error(), "link-local destination") || !strings.Contains(err.Error(), "refused") {
			t.Errorf("%s: Present err = %v, want the link-local destination refused at connect", u, err)
		}
	}
}

// TestResolvedURLNeverEchoed: a placeholder URL resolving to something the
// solver refuses fails without the resolved value in the error, the last
// error or the notification; it can carry a credential.
func TestResolvedURLNeverEchoed(t *testing.T) {
	secrets := []string{"pa55", "S3CRET", "hookusr"}
	values := map[string]string{
		"malformed scheme":        "htps://hookusr:pa55@hook.example.com/dns?token=S3CRET",
		"loopback without switch": "http://hookusr:pa55@127.0.0.1:9000/dns?token=S3CRET",
		"link-local":              "http://hookusr:pa55@169.254.169.254/dns?token=S3CRET",
		"user info":               "https://hookusr:pa55@hook.example.com/dns?token=S3CRET",
	}
	providers := map[string]map[string]model.Secret{
		model.DNSProviderWebhook: {"url": "${ENV:GPM_TEST_RESOLVED_URL}", "token": "t"},
		model.DNSProviderACMEDNS: {"baseURL": "${ENV:GPM_TEST_RESOLVED_URL}", "username": "u", "password": "p", "subdomain": "s"},
	}
	for provider, cfg := range providers {
		for name, v := range values {
			if provider == model.DNSProviderACMEDNS && name == "user info" {
				continue // acme-dns does not refuse user info
			}
			t.Run(provider+"/"+name, func(t *testing.T) {
				t.Setenv("GPM_TEST_RESOLVED_URL", v)
				var buf bytes.Buffer
				prev := log.Logger
				log.Logger = zerolog.New(&buf)
				t.Cleanup(func() { log.Logger = prev })

				p := model.DNSProvider{ObjectMeta: model.ObjectMeta{Name: "dns"}, Provider: provider, Config: cfg}
				_, err := defaultNewSolver(p)
				if err == nil {
					t.Fatal("defaultNewSolver: want an error")
				}
				// Through a renewal attempt: the last error and the notification.
				var notified string
				m := NewManager(Options{CertDir: t.TempDir(), OnRenewFailure: func(_ string, err error) { notified = err.Error() }})
				c := acmeCert("https://ca.example.com/directory", "app.example.com")
				c.Certificates[0].ACME.Challenge = model.ChallengeDNS01
				c.Certificates[0].ACME.DNSProvider = "dns"
				c.DNSProviders = []model.DNSProvider{p}
				_, _ = m.EnsureAll(context.Background(), c)
				last := m.CertObservations()[0].LastError
				for _, text := range []string{err.Error(), last, notified, buf.String()} {
					for _, s := range secrets {
						if strings.Contains(text, s) {
							t.Errorf("%q contains %q", text, s)
						}
					}
				}
				if last == "" || notified == "" {
					t.Errorf("last error %q, notification %q: want the failure recorded", last, notified)
				}
			})
		}
	}
}

// TestPlainHTTPWarningNamesTheHostOnly: the plain-http warnings log the host
// and nothing else of the URL.
func TestPlainHTTPWarningNamesTheHostOnly(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = prev })

	if _, err := NewWebhookDNSSolver(WebhookDNSConfig{URL: "http://hook.example.com:8080/dns/path?token=S3CRET", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	cfg := testACMEDNSConfig("http://hookusr:pa55@acme-dns.example.com:8053/base?token=S3CRET")
	cfg.AllowInsecureLocal = false
	if _, err := NewACMEDNSSolver(cfg); err != nil {
		t.Fatal(err)
	}
	logged := buf.String()
	if strings.Count(logged, "plain http") != 2 {
		t.Fatalf("log = %s, want both warnings", logged)
	}
	for _, s := range []string{"S3CRET", "pa55", "hookusr", "/dns/path", "/base"} {
		if strings.Contains(logged, s) {
			t.Errorf("warning log contains %q: %s", s, logged)
		}
	}
	if !strings.Contains(logged, `"host":"hook.example.com:8080"`) || !strings.Contains(logged, `"host":"acme-dns.example.com:8053"`) {
		t.Errorf("warning log = %s, want the hosts", logged)
	}
}

// A webhook URL may carry a credential in its path (a hook id), so no failure
// message names the path: a status error, a redirect and a transport error
// all say "the configured url". The path is still what the request is sent to.
func TestWebhookDNSErrorsDoNotNameThePath(t *testing.T) {
	const secret = "PATHSECRET"
	var gotPath string
	status := http.StatusInternalServerError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if status == http.StatusFound {
			w.Header().Set("Location", "http://127.0.0.1:1/elsewhere")
		}
		w.WriteHeader(status)
	}))
	url := srv.URL + "/hooks/T000/" + secret
	s, err := NewWebhookDNSSolver(WebhookDNSConfig{URL: url, Token: "t", AllowInsecureLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	check := func(what string) {
		t.Helper()
		err := s.Present(context.Background(), "_acme-challenge.example.com", "v")
		if err == nil {
			t.Fatalf("%s: expected an error", what)
		}
		if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "/hooks/") {
			t.Errorf("%s: the error names the url path: %v", what, err)
		}
		if !strings.Contains(err.Error(), "webhook: POST the configured url") {
			t.Errorf("%s: the error should name the call without the path: %v", what, err)
		}
	}
	check("status error")
	if gotPath != "/hooks/T000/"+secret {
		t.Fatalf("request path = %q, the configured path must still be used", gotPath)
	}
	status = http.StatusFound
	check("redirect")
	srv.Close()
	check("transport error")
}
