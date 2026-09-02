# Hardening

The settings to check before an instance faces the internet.

- Keep `-debug-headers` off in production (it exposes upstream addressing).
- Keep `-pprof` off unless actively profiling; it is admin-role **and**
  admin-scope gated but still needless attack surface when idle.
- Leave `GPM_COOKIE_SECURE` at its `auto` default. The session cookie becomes
  `Secure` (and `__Host-`-prefixed) on its own once the request reaches gpm over
  TLS, arrives through a **trusted** proxy asserting `X-Forwarded-Proto: https`,
  or `settings.externalBaseURL` is an `https://` URL, so a bootstrap login over
  `http://127.0.0.1:8081` works and the same deployment hardens itself the
  moment it is fronted. Set `1` to force `Secure` everywhere and `0` to force it
  off; `0` against an `https://` `externalBaseURL` is only sane for a deliberate
  LAN-only plain-HTTP admin listener beside the public URL.
- <span id="admin-session-cookie"></span>Watch for `cookieSecure: insecure-public`
  on `GET /api/capabilities`. It means a session cookie was issued without
  `Secure` to a **routable** client address,
  so it is crossing untrusted networks in the clear; the SPA shows a banner and
  gpm logs a rate-limited warning. `insecure-private` (loopback or RFC 1918 /
  ULA) is the ordinary first-run and LAN case.
- If a data-plane SSO session may have been exposed (device theft, cookie
  leak), `POST /api/sso/revoke` (or the button under Settings) invalidates
  every outstanding SSO session at once; users re-authenticate at the IdP.
- Raise the TLS floor once every client can do 1.3:
  [`settings.tls.minVersion: "1.3"`](../reference/config/settings/tls.md#settings-tls-min-version)
  covers every host, every stream-terminate listener and an unknown SNI in one
  place, and any host still serving older clients can pin `tls.minTLSVersion:
  "1.2"` for itself.
- Run with `cap_drop: ALL` and `no-new-privileges`, as in the
  [Compose file](../getting-started/install-docker.md#compose-file).
- Put the admin plane behind your ingress / a tunnel, not on the public internet.
- Prefer `${FILE:...}` secrets (Docker secrets) over `${ENV:...}` so values don't
  show up in the process environment.

## What the read-only `user` role cannot see

Handing out a viewer login is bounded by more than "no writes": three read
surfaces are redacted for any caller without the `admin` scope.

- **Deployment paths.** `GET /runtime` omits `paths` and `secretFileRoots`, so a
  viewer cannot read the config, certificate or session-database locations.
- **Notification and webhook targets.** `GET /webhooks/status` and
  `GET /notifications/status` reduce each URL to `scheme://host/(redacted)`, so a
  token embedded in a webhook path is not a viewer-readable secret.
- **Raw upstream errors.** `GET /health` reports `acme.lastError` as a
  classification plus the certificate name, never the provider's own message.
- **API tokens.** The `user` role is `*:read` **minus** the `api-tokens`
  subject, in either direction, and `GET /api/config` drops `apiTokens` for the
  same caller.

Full table: [What a non-admin caller does not see](../reference/api.md#what-a-non-admin-caller-does-not-see).
