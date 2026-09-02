# Enable TOTP for the local admin

Require a time-based one-time password after the local admin's password.

RFC 6238: SHA-1, 6 digits, 30-second step. It is off unless a secret is
configured, and it applies to the local account only: SSO logins keep whatever
MFA the identity provider enforces.

## Prerequisites

- A working local admin (`GPM_LOCAL_ADMIN_USER` plus a bcrypt hash).
- An authenticator app (any RFC 6238 client).
- A writable secret file under an allowlisted `GPM_SECRET_FILE_ROOTS` root.

## Steps

1. Generate a secret. The base32 secret goes to stdout, the enrolment URI to
   stderr:

   ```
   docker run --rm ghcr.io/rake-pro/go-proxy-manager totp-secret -account admin -issuer gpm > ./admin_totp
   ```

2. Add it to the authenticator app. Either type the base32 secret in by hand, or
   render the printed `otpauth://totp/...` URI as a QR code with any QR tool and
   scan it. gpm ships no QR renderer.
3. Mount the file as a secret and point gpm at it:

   ```yaml
   environment:
     GPM_LOCAL_ADMIN_TOTP_SECRET_FILE: /run/secrets/gpm_admin_totp
   secrets:
     - gpm_admin_totp
   ```

   The inline form `GPM_LOCAL_ADMIN_TOTP_SECRET` also works. Both are read once
   at startup, exactly like the password hash; neither is ever written to the
   git-backed config, and neither can be resolved by a `${ENV:...}` reference.
4. Restart the container. The startup log prints `local admin TOTP is enabled`.
   A secret that is not usable base32 is fatal: gpm refuses to start rather than
   silently dropping the second factor.
5. Sign in. The password form now leads to a second page asking for the 6-digit
   code.

## Verify

| Check | Expected |
|---|---|
| `GET /api/capabilities` | `adminLogin.totp: true` |
| `GET /api/runtime` | `localAdminTOTP: true` |
| Startup log | `local admin TOTP is enabled` |
| Login | Password page, then a "Verification code" page, then the panel |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Every code is rejected | Server and phone clocks differ by more than 30 seconds; only one step either side is accepted | Fix time sync (NTP) on the host |
| Every code is rejected, and the log says the secret is not usable base32 | gpm refuses to start with an unparseable secret, so this is a stale container | Regenerate with `gpm totp-secret` and restart |
| "authentication failed" on a code that just worked | Codes are single-use: the accepted step cannot be replayed | Wait for the next 30-second step |
| `429 too many login attempts` | Five failed attempts (wrong password **or** wrong code) from one client IP within 15 minutes | Wait 15 minutes, or restart gpm: the lockout and the used-code ledger are in memory only |
| Locked out with no authenticator (phone lost) | The secret is the only enrolment | Unset `GPM_LOCAL_ADMIN_TOTP_SECRET*` and restart to sign in with the password alone, then enrol a new secret |
