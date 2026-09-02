package auth

import (
	"context"
	"encoding/base32"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// totpLoginAuth builds an authenticator with local login enabled and, when
// secret is non-empty, a TOTP second factor.
func totpLoginAuth(t *testing.T, secret string) *Authenticator {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAuthenticator(Options{
		Store:           testStore(t),
		LocalUser:       "admin",
		LocalHash:       string(hash),
		LocalTOTPSecret: secret,
		SessionTTL:      time.Hour,
	})
	a.Configure(model.Config{}, model.Settings{
		AdminAuth: model.AdminAuthSettings{LocalLoginEnabled: true},
	})
	return a
}

func testSecret() string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))
}

func codeNow(t *testing.T, secret string, offsetSteps int64) string {
	t.Helper()
	key, err := NormalizeTOTPSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	return TOTPCode(key, uint64(time.Now().Unix()/totpStep)+uint64(offsetSteps))
}

// TestLocalLoginTOTPGate pins which of the two results LocalLogin returns.
func TestLocalLoginTOTPGate(t *testing.T) {
	tests := []struct {
		name        string
		secret      string
		user, pass  string
		wantSession bool
		wantPending bool
		wantErr     bool
	}{
		{name: "no totp configured", user: "admin", pass: "s3cret", wantSession: true},
		{name: "totp configured", secret: testSecret(), user: "admin", pass: "s3cret", wantPending: true},
		{name: "wrong password never reaches the second step", secret: testSecret(), user: "admin", pass: "nope", wantErr: true},
		{name: "wrong username never reaches the second step", secret: testSecret(), user: "root", pass: "s3cret", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := totpLoginAuth(t, tc.secret)
			sess, pending, err := a.LocalLogin(context.Background(), tc.user, tc.pass)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got session=%v pending=%q", sess != nil, pending)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (sess != nil) != tc.wantSession {
				t.Errorf("session present = %v, want %v", sess != nil, tc.wantSession)
			}
			if (pending != "") != tc.wantPending {
				t.Errorf("pending token present = %v, want %v", pending != "", tc.wantPending)
			}
		})
	}
}

// TestCompleteTOTPLoginSession checks the session a completed second factor
// mints: admin role, local IdP, and an AMR that records the second factor so a
// downstream policy can tell it from a password-only login.
func TestCompleteTOTPLoginSession(t *testing.T) {
	secret := testSecret()
	a := totpLoginAuth(t, secret)

	_, pending, err := a.LocalLogin(context.Background(), "admin", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := a.CompleteTOTPLogin(context.Background(), pending, codeNow(t, secret, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Roles) != 1 || sess.Roles[0] != string(RoleAdmin) {
		t.Errorf("roles = %v, want [admin]", sess.Roles)
	}
	if sess.IdP != "local" || sess.Subject != "admin" {
		t.Errorf("idp/subject = %q/%q, want local/admin", sess.IdP, sess.Subject)
	}
	var sawOTP bool
	for _, m := range sess.AMR {
		if m == "otp" {
			sawOTP = true
		}
	}
	if !sawOTP {
		t.Errorf("amr = %v, want it to record the second factor", sess.AMR)
	}
}

// TestCompleteTOTPLoginPendingExpiry checks an expired pending login is
// refused even with the right code. The clock is not injectable, so the entry's
// expiry is aged directly rather than sleeping out the five-minute TTL.
func TestCompleteTOTPLoginPendingExpiry(t *testing.T) {
	secret := testSecret()
	a := totpLoginAuth(t, secret)

	_, pending, err := a.LocalLogin(context.Background(), "admin", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	a.pmu.Lock()
	p := a.pending[pending]
	p.expires = time.Now().Add(-time.Second)
	a.pending[pending] = p
	a.pmu.Unlock()

	if _, err := a.CompleteTOTPLogin(context.Background(), pending, codeNow(t, secret, 0)); err == nil {
		t.Fatal("an expired pending login was accepted")
	}
}

// TestPendingTOTPCannotBeRedeemedAsOIDC checks the two half-finished login
// flows share the pending map without being interchangeable.
func TestPendingTOTPCannotBeRedeemedAsOIDC(t *testing.T) {
	a := totpLoginAuth(t, testSecret())
	_, pending, err := a.LocalLogin(context.Background(), "admin", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.CompleteLogin(context.Background(), pending, "some-code"); err == nil {
		t.Fatal("a pending TOTP login was redeemable at the OIDC callback")
	}
}

// TestMalformedTOTPSecretFailsClosed checks a secret that cannot be decoded
// leaves TOTP demanded and unsatisfiable, rather than silently disabling the
// factor the operator believes is protecting them.
func TestMalformedTOTPSecretFailsClosed(t *testing.T) {
	a := totpLoginAuth(t, "this is not base32 !!!")
	if !a.LocalTOTPEnabled() {
		t.Fatal("a malformed secret disabled TOTP")
	}
	_, pending, err := a.LocalLogin(context.Background(), "admin", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if pending == "" {
		t.Fatal("no second step was demanded")
	}
	for _, code := range []string{"000000", "123456", codeNow(t, testSecret(), 0)} {
		if _, err := a.CompleteTOTPLogin(context.Background(), pending, code); err == nil {
			t.Fatalf("code %q was accepted against an undecodable secret", code)
		}
	}
}
