package dataplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// basicMW builds a `mode: basic` auth middleware with one user.
func basicMW(t *testing.T, user, pass, realm string, allowFrom ...string) model.AuthMiddleware {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return model.AuthMiddleware{
		Mode:      model.AuthModeBasic,
		AllowFrom: allowFrom,
		Basic: &model.BasicAuthSpec{
			Users: []model.BasicAuthUser{{Username: user, PasswordHash: string(hash)}},
			Realm: realm,
		},
	}
}

// TestBasicAuthModeGatesLikeAccessList is the parity check against the oracle
// this mode replaces (TestAccessListBasicAuth): the same 401 + challenge with no
// credentials, the same 200 with the right ones, the same 401 with the wrong
// password.
func TestBasicAuthModeGatesLikeAccessList(t *testing.T) {
	spec := basicMW(t, "admin", "hunter2", "")
	h := basicAuthGate(spec, "app", ipFrom("1.2.3.4"), nil, "app", nil, okHandler())

	cases := []struct {
		name          string
		user, pass    string
		creds         bool
		want          int
		wantChallenge string
	}{
		{name: "no credentials", want: http.StatusUnauthorized, wantChallenge: `Basic realm="app"`},
		{name: "good credentials", user: "admin", pass: "hunter2", creds: true, want: http.StatusOK},
		{name: "wrong password", user: "admin", pass: "wrong", creds: true, want: http.StatusUnauthorized, wantChallenge: `Basic realm="app"`},
		{name: "unknown user", user: "nobody", pass: "hunter2", creds: true, want: http.StatusUnauthorized, wantChallenge: `Basic realm="app"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if c.creds {
				r.SetBasicAuth(c.user, c.pass)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != c.want {
				t.Fatalf("got %d want %d", rec.Code, c.want)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != c.wantChallenge {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, c.wantChallenge)
			}
		})
	}
}

// TestBasicAuthModeRealm checks the configured realm is what the challenge
// advertises, and that the owner name is the fallback.
func TestBasicAuthModeRealm(t *testing.T) {
	cases := []struct{ realm, want string }{
		{"", `Basic realm="app"`},
		{"Staging area", `Basic realm="Staging area"`},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			h := basicAuthGate(basicMW(t, "u", "p", c.realm), "app", ipFrom("1.2.3.4"), nil, "app", nil, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if got := rec.Header().Get("WWW-Authenticate"); got != c.want {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, c.want)
			}
		})
	}
}

// TestBasicAuthModeAllowFrom checks the network exemption: a client inside
// allowFrom is proxied straight through with no credentials and no challenge,
// one outside it is challenged as usual.
func TestBasicAuthModeAllowFrom(t *testing.T) {
	spec := basicMW(t, "admin", "hunter2", "", "10.0.0.0/8")
	nets := allowFromNets(spec.AllowFrom)

	cases := []struct {
		ip   string
		want int
	}{
		{"10.1.2.3", http.StatusOK},
		{"203.0.113.7", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			h := basicAuthGate(spec, "app", ipFrom(c.ip), nets, "app", nil, okHandler())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
			if rec.Code != c.want {
				t.Fatalf("ip %s: got %d want %d", c.ip, rec.Code, c.want)
			}
		})
	}
}

// TestBasicAuthModeLocksOutAfterRepeatedFailures mirrors the access-list
// throttle: a locked-out client is answered with the same 401 + challenge, never
// a 429, so the response is no oracle for the lockout.
func TestBasicAuthModeLocksOutAfterRepeatedFailures(t *testing.T) {
	h := basicAuthGate(basicMW(t, "admin", "hunter2", ""), "app", ipFrom("203.0.113.9"), nil, "app", nil, okHandler())
	attempt := func(pass string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/", nil)
		r.SetBasicAuth("admin", pass)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	for range maxBasicAuthFails {
		if got := attempt("wrong").Code; got != http.StatusUnauthorized {
			t.Fatalf("failed attempt: got %d want 401", got)
		}
	}
	// The correct password is now refused too, with the identical response.
	locked := attempt("hunter2")
	if locked.Code != http.StatusUnauthorized {
		t.Fatalf("locked out: got %d want 401", locked.Code)
	}
	if locked.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("locked out: expected the same challenge as a wrong password, got none")
	}
}

// TestBasicAuthModeNoUsersFailsClosed checks a spec that reached the compiler
// with no credentials denies rather than admits. Validation refuses it, so this
// is the belt.
func TestBasicAuthModeNoUsersFailsClosed(t *testing.T) {
	for _, spec := range []model.AuthMiddleware{
		{Mode: model.AuthModeBasic},
		{Mode: model.AuthModeBasic, Basic: &model.BasicAuthSpec{}},
	} {
		h := basicAuthGate(spec, "app", ipFrom("1.2.3.4"), nil, "app", nil, okHandler())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("empty basic spec: got %d want 503", rec.Code)
		}
	}
}

// TestAuthHandlerRoutesBasicMode checks the shared auth compiler dispatches
// mode basic without an identity provider - which is what makes an inline host
// `auth:` block with mode basic work, since it reuses this same function.
func TestAuthHandlerRoutesBasicMode(t *testing.T) {
	reg := &registry{}
	h := authHandler(basicMW(t, "admin", "hunter2", "inline"), reg, "app", []string{"app.example.com"}, ipFrom("1.2.3.4"), nil, okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no credentials: got %d want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != `Basic realm="inline"` {
		t.Fatalf("WWW-Authenticate = %q", got)
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("admin", "hunter2")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("good credentials: got %d want 200", rec.Code)
	}
}
