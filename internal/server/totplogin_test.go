package server

import (
	"encoding/base32"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/auth"
)

// totpTestSecret is a fixed base32 secret so the tests can compute the code the
// server expects. Test-only material, never a deployment value.
var totpTestSecret = base32.StdEncoding.WithPadding(base32.NoPadding).
	EncodeToString([]byte("12345678901234567890"))

func totpEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnv(t, envOpts{
		localUser:    "admin",
		localPass:    "s3cret",
		localEnabled: true,
		noProviders:  true,
		localTOTP:    totpTestSecret,
	})
}

// currentCode is the code an authenticator app would be showing right now.
func currentCode(t *testing.T, offsetSteps int64) string {
	t.Helper()
	key, err := auth.NormalizeTOTPSecret(totpTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	counter := uint64(time.Now().Unix()/30) + uint64(offsetSteps)
	return auth.TOTPCode(key, counter)
}

// password submits step one and returns the pending token from the rendered
// second-step form.
func (e *testEnv) password(t *testing.T, user, pass, returnTo string) (*http.Response, string) {
	t.Helper()
	res := e.postForm("/auth/local", url.Values{
		"username": {user}, "password": {pass}, "return": {returnTo},
	})
	return res, pendingTokenFrom(t, res)
}

func pendingTokenFrom(t *testing.T, res *http.Response) string {
	t.Helper()
	body := readBody(t, res)
	const marker = `name="pending" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		t.Fatal("second-step form has an unterminated pending token")
	}
	return rest[:j]
}

// TestTOTPLoginTwoStep walks the happy path: a correct password yields the
// code form and NO session cookie, and the correct code then yields the
// session and the post-login redirect.
func TestTOTPLoginTwoStep(t *testing.T) {
	e := totpEnv(t)

	res, pending := e.password(t, "admin", "s3cret", "/settings")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("password step status = %d, want 200 (the code form)", res.StatusCode)
	}
	if pending == "" {
		t.Fatal("password step did not render a pending token")
	}
	for _, c := range res.Cookies() {
		if c.Name == "gpm_session" && c.Value != "" {
			t.Fatal("a session cookie was issued before the second factor")
		}
	}

	res2 := e.postForm("/auth/local/totp", url.Values{
		"pending": {pending}, "code": {currentCode(t, 0)}, "return": {"/settings"},
	})
	if res2.StatusCode != http.StatusFound {
		t.Fatalf("code step status = %d, want 302", res2.StatusCode)
	}
	if got := res2.Header.Get("Location"); got != "/settings" {
		t.Fatalf("redirect = %q, want /settings", got)
	}
	if c := cookie(t, res2, "gpm_session"); c == nil || c.Value == "" {
		t.Fatal("no session cookie after a correct code")
	}
}

// TestTOTPLoginRejections covers every way step two can fail. Each case starts
// from a fresh password step, because the pending token is single-use.
func TestTOTPLoginRejections(t *testing.T) {
	tests := []struct {
		name string
		// code is computed per case; steps is the offset from "now".
		steps    int64
		badToken bool
	}{
		{name: "code from too far in the past", steps: -5},
		{name: "code from too far in the future", steps: 5},
		{name: "unknown pending token", badToken: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := totpEnv(t)
			_, pending := e.password(t, "admin", "s3cret", "/")
			if pending == "" {
				t.Fatal("no pending token")
			}
			code := currentCode(t, tc.steps)
			if tc.badToken {
				pending, code = "not-a-real-pending-token", currentCode(t, 0)
			}
			res := e.postForm("/auth/local/totp", url.Values{
				"pending": {pending}, "code": {code}, "return": {"/"},
			})
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", res.StatusCode)
			}
			if c := cookie(t, res, "gpm_session"); c != nil && c.Value != "" {
				t.Fatal("a session cookie was issued for a rejected code")
			}
		})
	}
}

// TestTOTPPendingTokenIsSingleUse checks that a pending token is consumed by
// its first redemption, right or wrong: a wrong code cannot be retried against
// the same token, and a correct code cannot be replayed to mint a second
// session.
func TestTOTPPendingTokenIsSingleUse(t *testing.T) {
	t.Run("after a wrong code", func(t *testing.T) {
		e := totpEnv(t)
		_, pending := e.password(t, "admin", "s3cret", "/")
		bad := e.postForm("/auth/local/totp", url.Values{
			"pending": {pending}, "code": {"000000"}, "return": {"/"},
		})
		if bad.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong code status = %d, want 401", bad.StatusCode)
		}
		retry := e.postForm("/auth/local/totp", url.Values{
			"pending": {pending}, "code": {currentCode(t, 0)}, "return": {"/"},
		})
		if retry.StatusCode != http.StatusUnauthorized {
			t.Fatalf("retry with the right code on a spent token = %d, want 401", retry.StatusCode)
		}
	})

	t.Run("after a correct code", func(t *testing.T) {
		e := totpEnv(t)
		_, pending := e.password(t, "admin", "s3cret", "/")
		code := currentCode(t, 0)
		if res := e.postForm("/auth/local/totp", url.Values{
			"pending": {pending}, "code": {code}, "return": {"/"},
		}); res.StatusCode != http.StatusFound {
			t.Fatalf("first redemption = %d, want 302", res.StatusCode)
		}
		if res := e.postForm("/auth/local/totp", url.Values{
			"pending": {pending}, "code": {code}, "return": {"/"},
		}); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("replayed redemption = %d, want 401", res.StatusCode)
		}
	})
}

// TestTOTPCodeReplayAcrossLogins is the replay test that matters operationally:
// a code observed once cannot be reused in a SECOND, independently started
// login within the same 30-second step.
func TestTOTPCodeReplayAcrossLogins(t *testing.T) {
	e := totpEnv(t)

	_, first := e.password(t, "admin", "s3cret", "/")
	code := currentCode(t, 0)
	if res := e.postForm("/auth/local/totp", url.Values{
		"pending": {first}, "code": {code}, "return": {"/"},
	}); res.StatusCode != http.StatusFound {
		t.Fatalf("first login = %d, want 302", res.StatusCode)
	}

	_, second := e.password(t, "admin", "s3cret", "/")
	if second == "" {
		t.Fatal("second login did not reach the code step")
	}
	res := e.postForm("/auth/local/totp", url.Values{
		"pending": {second}, "code": {code}, "return": {"/"},
	})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replayed code in a second login = %d, want 401", res.StatusCode)
	}
}

// TestTOTPWrongCodesCountTowardLockout checks the second factor shares the
// password step's per-IP lockout: six wrong codes lock the client out, and the
// lockout then applies to the password step too.
func TestTOTPWrongCodesCountTowardLockout(t *testing.T) {
	e := totpEnv(t)

	var last *http.Response
	for i := 0; i < 6; i++ {
		_, pending := e.password(t, "admin", "s3cret", "/")
		last = e.postForm("/auth/local/totp", url.Values{
			"pending": {pending}, "code": {"000000"}, "return": {"/"},
		})
		if last.StatusCode == http.StatusTooManyRequests {
			break
		}
	}
	if last.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("last code attempt = %d, want 429 after repeated wrong codes", last.StatusCode)
	}

	res := e.postForm("/auth/local", url.Values{
		"username": {"admin"}, "password": {"s3cret"}, "return": {"/"},
	})
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("password step during the lockout = %d, want 429", res.StatusCode)
	}
}

// TestLocalLoginWithoutTOTPIsUnchanged is the regression guard for the default
// deployment: with no secret set, one POST still returns the session.
func TestLocalLoginWithoutTOTPIsUnchanged(t *testing.T) {
	e := newTestEnv(t, envOpts{
		localUser: "admin", localPass: "s3cret", localEnabled: true, noProviders: true,
	})
	res := e.postForm("/auth/local", url.Values{
		"username": {"admin"}, "password": {"s3cret"}, "return": {"/hosts"},
	})
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if c := cookie(t, res, "gpm_session"); c == nil || c.Value == "" {
		t.Fatal("no session cookie from a single-step local login")
	}
}
