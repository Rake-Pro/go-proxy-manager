# Environment variables, flags and CLI

Every runtime flag and its environment-variable equivalent, the listeners they
bind, and the subcommands the binary ships.

## Listeners and ports

| Port (default) | Surface | Expose to |
|----------------|---------|-----------|
| `:443` | Proxy listener HTTPS | Internet |
| `:80` | Proxy listener HTTP (force-SSL redirects, plaintext hosts, ACME HTTP-01 challenges) | Internet |
| `:8081` | Admin API + web UI | **Not the internet**: your ingress, LAN, or an SSH tunnel |

The admin panel is authenticated, but you should still keep it off the public
internet. Bind it to loopback (`127.0.0.1:8081`) and reach it via a tunnel, or
front it with your own authenticating ingress (including gpm itself): see
[Reach the admin UI through gpm itself](../how-to/admin-ui-behind-gpm.md) for the recipe.

If gpm's own proxy listeners `:80`/`:443` can't be reached from the internet at all
(CGNAT, no port forwarding), see [Using gpm behind CGNAT](../how-to/tunnels.md) for
Cloudflare Tunnel, Tailscale and WireGuard/VPS-relay recipes.

Stream hosts open one additional listener per `listenPort` (see
[StreamHost](config/stream-host.md)); publish those
ports explicitly.

Both proxy listeners bind a bare `:port` by default (`GPM_HTTP_ADDR=:80`,
`GPM_HTTPS_ADDR=:443`), which in Go binds the **IPv6 wildcard with v4-mapped
addresses enabled**: one socket serving both families. See
[IPv6](../getting-started/install-docker.md#ipv6).

A Prometheus exposition is available at `/metrics` on the **admin** listener,
off by default (`-metrics` / `GPM_METRICS=1`) and gated like the rest of the
admin panel, see [Metrics (Prometheus)](metrics.md) below. Unauthenticated,
the admin server exposes only `/healthz` and `/version`.

## Flags and environment variables

Every flag has an environment-variable equivalent (the env var wins for
container deployments).

| Name | Default | Meaning | Restart needed |
|------|---------|---------|----------------|
| `GPM_CONFIG_DIR`<br>`-config-dir` | `/data/config` | Git-backed config repo | Yes |
| `GPM_CERT_DIR`<br>`-cert-dir` | `/data/certs` | Certificate store | Yes |
| `GPM_SESSION_DB`<br>`-session-db` | `/data/session.db` | Session database | Yes |
| `GPM_ADMIN_ADDR`<br>`-admin-addr` | `:8081` | Admin listen address | Yes |
| `GPM_HTTPS_ADDR`<br>`-https-addr` | `:443` | Proxy listener HTTPS | Yes |
| `GPM_HTTP_ADDR`<br>`-http-addr` | `:80` | Proxy listener HTTP | Yes |
| `GPM_LOCAL_ADMIN_USER`<br>`-local-admin-user` | (none) | Break-glass admin username | Yes |
| `GPM_COOKIE_SECURE`<br>`-cookie-secure=<v>` | `auto` | `Secure` flag on admin cookies (session and OIDC login state). `auto` decides per request: `Secure` when the request arrived over TLS, through a trusted proxy sending `X-Forwarded-Proto: https`, or when `externalBaseURL` is an `https://` URL. `1` forces it on, `0` off (and logs a warning when `externalBaseURL` is `https://`). A `Secure` cookie is named `__Host-gpm_session`, a non-Secure one `gpm_session`; both names are accepted on the way in | Yes |
| `GPM_SECRET_FILE_ROOTS` | `/run/secrets` | Allowlisted root dir(s) for `${FILE:...}` secret resolution; OS-path-list separated | Yes |
| `GPM_SECRET_ENV_PREFIXES` | (none) | Comma-separated allowlist of `${ENV:...}` name prefixes. Unset: any non-reserved var resolves. Set: only names with a listed prefix resolve. `GPM_SSO_SIGNING_KEY`, `GPM_LOCAL_ADMIN_PASSWORD_HASH` and `GPM_LOCAL_ADMIN_TOTP_SECRET` are never resolvable regardless | Yes |
| `GPM_SSO_SIGNING_KEY` | (auto-persisted) | HMAC key signing per-host OIDC session cookies; auto-generated on first use and saved to `<cert-dir>/sso_signing.key` (0600) so sessions survive restarts; set explicitly to supply your own key or share one across instances | Yes |
| `GPM_LOCAL_ADMIN_TOTP_SECRET` | (none) | Base32 TOTP secret for the local admin. Set (or its `_FILE` form) to require a 6-digit code after the password; unset means password only. Generate with `gpm totp-secret`. | Yes |
| `GPM_LOG_LEVEL`<br>`-log-level` | `info` | trace/debug/info/warn/error | Yes |
| `GPM_LOG_CONSOLE`<br>`-log-console` | `false` | Human-readable console logs | Yes |
| `GPM_ACCESS_LOG`<br>`-access-log` | `false` | Log every proxied request. Startup default only: capture can be flipped live from the Access Logs page or `PUT /api/logs` (admin scope); the runtime toggle is never persisted, so a restart reverts to this flag | Yes |
| `GPM_SLOW_REQUEST_MS`<br>`-slow-request-ms` | `0` (off) | Warn on requests slower than N ms | Yes |
| `GPM_DEBUG_HEADERS`<br>`-debug-headers` | `false` | Add `X-GPM-*` diagnostic response headers (leaks upstream info, keep off in production) | Yes |
| `GPM_UPSTREAM_RESPONSE_HEADER_TIMEOUT`<br>`-upstream-response-header-timeout` | `0` (unbounded) | Cap time-to-first-byte from an upstream | Yes |
| `GPM_HA_ROLE`<br>`-ha-role` | `leader` | HA role: `leader` runs ACME + Ingress/Docker discovery and accepts config writes; `follower` disables them and serves the admin API read-only (`503` on writes). See [High availability (two-node active/standby)](../operations/high-availability.md) | Yes |
| `GPM_HA_POLL_INTERVAL`<br>`-ha-poll-interval` | `20s` | How often a follower runs `git pull --ff-only` on the config repo and reloads when HEAD moved | Yes |
| `GPM_PPROF`<br>`-pprof` | `false` | Expose `net/http/pprof` on the admin server at `/debug/pprof/` (admin role **and** `admin` scope gated) | Yes |
| `GPM_METRICS`<br>`-metrics` | `false` | Expose a Prometheus text exposition on the admin server at `/metrics` (admin role **and** `metrics:read` scope gated) | Yes |
| `GPM_GEOIP_DB`<br>`-geoip-db` | (none) | Path to an operator-supplied GeoLite2/GeoIP2 `.mmdb` file for AccessList geo rules; unset disables geo rules, no database is bundled | Yes |

**Admin password** is not a flag. Provide a bcrypt hash via, in order of
preference:
- `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE`: path to a file (e.g. a Docker secret), or
- `GPM_LOCAL_ADMIN_PASSWORD_HASH`: the hash inline.

Generate one with `gpm hashpw 'your-password'` (or `docker run --rm <image> hashpw ...`).

**Admin second factor (TOTP)** is not a flag either. Supply a base32 secret via
`GPM_LOCAL_ADMIN_TOTP_SECRET_FILE` or `GPM_LOCAL_ADMIN_TOTP_SECRET`; see
[Enable TOTP for the local admin](../how-to/totp.md).

## CLI subcommands

The default invocation with no subcommand runs the daemon. Each subcommand
below exits when it is done and never starts a listener.

| Command | Flags | Does |
|---|---|---|
| `gpm` | every flag in the table above, plus `-version` | Runs the daemon: proxy listeners, admin panel, ACME, reconcilers. |
| `gpm -version` | none | Prints the build version and exits. |
| `gpm hashpw [password]` | none; the password is a positional argument, or read from stdin when omitted | Prints a bcrypt hash on stdout for `GPM_LOCAL_ADMIN_PASSWORD_HASH*`. |
| `gpm totp-secret` | `-account` (default `GPM_LOCAL_ADMIN_USER`, else `admin`), `-issuer` (default `gpm`) | Prints a base32 TOTP secret on stdout and the `otpauth://` enrolment URI on stderr. No QR code is rendered. |

```
gpm hashpw 'your-password' > admin_hash
gpm totp-secret -account admin -issuer gpm > admin_totp
```

## Runtime facts (operations)

`GET /api/runtime` (scope `settings:read`) reports how the running process was
started, so the flags and paths behind a deployment can be read from the panel
instead of the container's command line:

```
curl -s -b "<admin session cookie>" https://<admin>/api/runtime | jq
```

| Field | Meaning |
|---|---|
| `version` | Build version of the running binary. |
| `haRole` | `leader` or `follower` (`GPM_HA_ROLE`). |
| `listeners.http` / `.https` / `.admin` | The three listen addresses. |
| `paths.configDir` / `.certDir` / `.sessionDB` | What to back up, and where the session DB lives. |
| `metricsEnabled` / `pprofEnabled` | Whether `/metrics` and `/debug/pprof/` are mounted. |
| `accessLogEnabled` | Live value, including a runtime toggle via `PUT /api/logs`. |
| `geoipLoaded` | Whether geo access-list rules can be evaluated. |
| `secretFileRoots` | Effective `${FILE:...}` allowlist (`GPM_SECRET_FILE_ROOTS`). |
| `localAdminConfigured` | Whether a usable local admin credential exists. The username and hash are never reported. |
| `localAdminTOTP` | Whether a TOTP second factor is configured for the local admin. |

> **`paths` and `secretFileRoots` are admin-scope only.** A caller without the
> `admin` scope (a viewer-role session, or a resource-scoped token) gets the
> payload with both fields omitted, so the deployment's filesystem layout is not
> a read-only credential. See
> [What a non-admin caller does not see](api.md#what-a-non-admin-caller-does-not-see).
