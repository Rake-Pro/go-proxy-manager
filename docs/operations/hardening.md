# Hardening

The settings to check before an instance faces the internet.

- Treat the admin listener (`:8081`) as plain HTTP: no password, session
  cookie or API token may cross a network to it. Bind or publish it on
  loopback (`127.0.0.1:8081`) or a management address (`192.0.2.10:8081`),
  not on every interface, and reach it from elsewhere through an SSH tunnel or
  the HTTPS admin hostname
  ([Reach the admin UI through gpm itself](../how-to/admin-ui-behind-gpm.md)).
- Answer scanner paths at the edge: [`settings.pathBlock`](../reference/config/settings/path-block.md) refuses dotfiles, VCS directories and script drop paths with a `404` by default before any upstream sees them ([how-to](../how-to/block-scanner-paths.md)). It is off until set.
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
- If a per-host SSO session may have been exposed (device theft, cookie
  leak), `POST /api/sso/revoke` (or the button under Settings) invalidates
  every outstanding SSO session at once; users re-authenticate at the IdP.
- If an admin panel session may have been exposed, or you withdrew someone's
  admin access, `POST /api/sessions/revoke` (admin) ends every admin session
  on that instance except your own; run it on each HA instance, since sessions
  are per instance. A session whose sign-in method is no longer allowed (its
  provider removed from `adminAuth.providers` or deleted, or local login turned
  off) is refused on its next request without any action.
- Raise the TLS floor once every client can do 1.3:
  [`settings.tls.minVersion: "1.3"`](../reference/config/settings/tls.md#settings-tls-min-version)
  covers every host, every stream-terminate listener and an unknown SNI in one
  place, and any host still serving older clients can pin `tls.minTLSVersion:
  "1.2"` for itself.
- Bound what one client address can keep in progress:
  [`settings.clientLimits.maxInFlight`](../reference/config/settings/client-limits.md#settings-client-limits-max-in-flight)
  (off by default; `256` is a starting point) refuses a client's requests
  past the limit with `429`.
- Run with `cap_drop: ALL` and `no-new-privileges`, as in the
  [Compose file](../getting-started/install-docker.md#compose-file).
- Put the admin panel behind your ingress / a tunnel, not on the public internet.
- Prefer `${FILE:...}` secrets (Docker secrets) over `${ENV:...}` so values don't
  show up in the process environment.
- Keep `acme.caBundle` to the one private root you mean to trust: a certificate that sets it trusts **only** that bundle for its ACME directory, and setting or changing it through the API needs the `admin` scope (with `directoryURL` and `eab`), because it decides which server gpm accepts as the CA.
- Give a `webhook` DNS provider a `https` `url` and a `token` held as a `${ENV:}` or `${FILE:}` placeholder (the `url` is a plain setting: a literal is committed and readable by anyone who can read DNS providers, so use a placeholder if it carries a credential). gpm refuses loopback, link-local, unspecified and multicast literals at write time unless `allowInsecureLocal` is set, always refuses a link-local destination when it connects (the same guard covers the bouncer, webhook and notification targets, the acme-dns, webhook, DigitalOcean, Hetzner and deSEC DNS-01 solvers, Kubernetes and Docker discovery, and DNS sync, not the Cloudflare and RFC 2136 solvers or the ACME CA client; it ignores an address zone, so `[::ffff:169.254.169.254%25eth0]` is refused, and it refuses a destination it cannot read), never follows redirects with the token attached, and warns in the log about a plain-`http` URL to a non-loopback host; writes on `/api/dns-providers` need the `admin` scope.
- Give a webhook or notification `url` that carries a credential in its path
  (a chat or incident hook token) as a whole-value `${ENV:NAME}` or
  `${FILE:path}` placeholder, so only the reference is committed to git and
  shown by the status endpoints
  ([Keeping the url out of git](../reference/config/settings/webhooks.md#keeping-the-url-out-of-git)).
- What the admin listener does on its own, so you know not to re-implement it
  in front: `/version` needs a signed-in session or token (only `/healthz` is
  anonymous); local login locks a client IP after five failures and a username
  after twenty, both for 15 minutes, with one identical 429 page; an admin
  session is extended while in use but ends 7 days after sign-in; only
  `adminAuth.providers` can start a panel SSO login, and starting one stores
  nothing server-side (the state rides in a signed cookie); idle keep-alive
  connections close after 120 s and the login form bodies are capped at
  64 KiB; a guard
  `queryEquals` trigger fails closed on a repeated query key; a backup restore
  keeps the live `api-tokens/*` and ignores the archive's, as Revert does.

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

## Multipath TCP is off

gpm listens with plain TCP. Go 1.24 and later open MPTCP-capable listeners by
default wherever the kernel allows it (`net.mptcp.enabled=1`, the Linux
default), so without an explicit opt-out every Go service is an MPTCP listener
without anyone deciding that. Nothing gpm fronts needs MPTCP, and a listening
MPTCP socket was the precondition of the published root exploit for the Linux
kernel flaw CVE-2026-98432 (IPv4 TCP request-socket use-after-free, fixed
upstream in 6.12.113 / 7.2.10 / 7.3). Since 2.6.0 every listener (data plane,
stream hosts, admin) is opened with MPTCP disabled; `GPM_MPTCP=1` (`-mptcp`)
re-enables it. This removes the exploit's foothold while distribution kernels
catch up; it does not replace the kernel update.
