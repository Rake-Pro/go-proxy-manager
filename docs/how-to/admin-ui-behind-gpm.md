# Reach the admin UI through gpm itself

[Deployment](../reference/env-vars-and-flags.md#listeners-and-ports) says to front the admin listener
(`:8081`) with "your own authenticating ingress." gpm can be that ingress: a
normal `ProxyHost` pointed at the admin listener, gated by a normal
`AccessList`, using gpm's own certificate flow. No second reverse proxy, no
manual TLS.

## Prerequisites

- A running gpm instance with LAN/console access to the admin listener for
  the bootstrap step below.
- A DNS name for the admin UI (e.g. `gpm.example.com`) pointed at gpm's data
  plane, and a way to issue a certificate for it (see
  [Automatic certificates](../getting-started/first-host-with-https.md)).
- The LAN/VPN CIDR ranges that should be allowed to reach the admin panel.

> **Set `settings.trustedProxies` before you proxy the admin panel.** It must
> list the address the admin listener sees for gpm's own data plane, for a
> loopback bind, `127.0.0.1/32` and/or `::1/128`. Without it every admin login
> attempt is attributed to that one address, and the login lockout, the TOTP
> throttle and the pending-login cap all become a single global bucket: one
> attacker can lock out every administrator. See
> [Client IP and the three trust tiers](../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers).
>
> ```yaml
> # config/settings.yaml
> trustedProxies: [127.0.0.1/32, ::1/128]
> ```

## Env vars used here

| Name | Default | Meaning | Restart needed |
|------|---------|---------|-----------------|
| `GPM_ADMIN_ADDR` | `:8081` | Admin listen address. Bare `:8081` is a wildcard bind on every interface, not loopback-only. | Yes |
| `GPM_COOKIE_SECURE` | `auto` | `Secure` flag on admin cookies. `auto` decides per request: `Secure` when the request is TLS, forwarded as `https` by a trusted proxy, or `externalBaseURL` is `https://`. `1` forces it on, `0` off. | Yes |

## Steps

1. **Bind the admin listener to loopback.** The default `GPM_ADMIN_ADDR`
   (`:8081`) is reachable on every interface, not just loopback, see
   [IPv6](../getting-started/install-docker.md#ipv6) for the same bare-`:port` behaviour on the data
   plane. Set it explicitly so the only path in is through the proxy host
   built below (or a shell on the same host/container):

   ```
   GPM_ADMIN_ADDR=127.0.0.1:8081
   ```

2. **Bootstrap over the raw admin listener.** You cannot configure the proxy
   host that will front the admin UI from a UI that isn't fronted yet. Reach
   `127.0.0.1:8081` directly: LAN, console, or an SSH tunnel:

   ```
   ssh -L 8081:127.0.0.1:8081 <host>
   ```

   This connection is always plain HTTP (the admin listener never terminates
   TLS itself). No redeploy is needed for it: with the default
   `GPM_COOKIE_SECURE=auto`, the session cookie is issued without `Secure` on
   this connection and login works. Once step 5 sets an `https://`
   `externalBaseURL` (or the request reaches gpm over TLS, or through a
   trusted proxy sending `X-Forwarded-Proto: https`), the same cookie is issued
   `Secure` and `__Host-` named, again with no restart.

3. **Create the certificate and the proxy host**, from inside the bootstrap
   session:

   ```yaml
   # config/certificates/gpm-admin.yaml - reuse an existing certificate
   # covering this name instead if you already have one.
   name: gpm-admin
   type: acme
   domains: [gpm.example.com]
   acme: {email: admin@example.com, challenge: http-01}
   ```

   ```yaml
   # config/proxy-hosts/gpm-admin.yaml
   name: gpm-admin
   domains: [gpm.example.com]
   upstream: {scheme: http, host: 127.0.0.1, port: 8081}
   tls: {certificateRef: gpm-admin, forceSSL: true}
   accessLists: [admin-lan-only]
   ```

   `upstream.host`/`upstream.port` must match whatever `GPM_ADMIN_ADDR` is
   actually bound to (step 1). `scheme: http` is correct: the admin server
   never speaks TLS; this `ProxyHost` is what terminates it.

   **UI clicks:** Proxy Hosts -> New -> Domains: `gpm.example.com` -> Upstream:
   scheme `http`, host `127.0.0.1`, port `8081` -> TLS tab: pick/issue
   `gpm-admin`, enable "Force SSL" -> Access control tab: attach
   `admin-lan-only` (next step).

4. **Restrict it with an access list.** TLS alone does not limit who can
   reach the admin panel: this does:

   ```yaml
   # config/access-lists/admin-lan-only.yaml
   name: admin-lan-only
   defaultAction: deny
   rules:
     - {action: allow, cidr: 10.0.0.0/8}       # LAN
     - {action: allow, cidr: 192.168.0.0/16}   # LAN
     - {action: allow, cidr: 100.64.0.0/10}    # e.g. a Tailscale/CGNAT VPN
   ```

   **UI clicks:** Access Lists -> New -> name `admin-lan-only` -> Default
   action: Deny -> add allow rules for your LAN/VPN CIDRs -> Save. Full field
   reference: [AccessList](../reference/config/access-list.md).

   If something else proxies ahead of gpm for this host, read
   [Client IP and the three trust tiers](../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers)
   first: by default gpm keys IP rules off the raw TCP peer, not
   `X-Forwarded-For`, until you list that proxy in `settings.trustedProxies` (or
   the host's own `trustedProxies`).

5. **Set `externalBaseURL`:**

   ```yaml
   # config/settings.yaml
   externalBaseURL: https://gpm.example.com
   ```

   This must match the exact scheme + host you are now using, not the
   bootstrap `http://127.0.0.1:8081`. It drives both the OIDC
   `redirect_uri` (see [Settings](../reference/config/settings/index.md))
   and the cookie decision from step 2: an `https://` `externalBaseURL` makes
   admin cookies `Secure` even on the loopback listener, so a later
   break-glass session on `http://127.0.0.1:8081` needs a tunnel that keeps
   the browser on `https://gpm.example.com`, or `GPM_COOKIE_SECURE=0` for that
   session only.

6. **Stop publishing `8081`.** Once `https://gpm.example.com` works end to
   end, remove any port publish/forward for `8081` beyond the host/container
   itself, drop `"127.0.0.1:8081:8081"` from Compose entirely, or leave it
   loopback-only for break-glass shell access. The `ProxyHost` from step 3 is
   now the only supported path in.

7. **(Optional) Layer SSO in front, keep local login as the anti-lockout
   path.** Use gpm's native
   [Admin OIDC](admin-oidc-sso.md), not a second `auth`
   middleware on this host: an outer middleware gate would take the local
   login page down with it if the IdP has a bad day.

   - Follow [Admin OIDC](admin-oidc-sso.md): create an
     `IdentityProvider` with `roleMapping.adminGroups`, list it under
     `settings.adminAuth.providers`.
   - Keep `adminAuth.localLoginEnabled: true`, the anti-lockout path if the
     IdP is unreachable or misconfigured.
   - Flip `adminAuth.ssoOnly: true` only after a real SSO login has
     succeeded. Recovery from `ssoOnly: true` during an SSO outage is a
     redeploy, not a UI action.
   - The `admin-lan-only` access list from step 4 stays in effect underneath
     either login method: it is a network gate, not a substitute for one.

## Verify

- `curl -fsS https://gpm.example.com/healthz` returns `ok` from an allowed
  network and times out or `403`s from outside it.
- Logging in and saving a config change at `https://gpm.example.com` succeeds
  and shows up under **History**.
- `curl -fsS http://127.0.0.1:8081/healthz` (from inside the
  host/container) still works, confirms the admin listener is still up on
  loopback for break-glass access.
- No external port publish/forward for `8081` remains (step 6).

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `502` on `https://gpm.example.com` | Wrong upstream address/port | Confirm `GPM_ADMIN_ADDR` (step 1) and the proxy host's `upstream.host`/`upstream.port` (step 3) agree, and that `GET /healthz` on that loopback address works from inside the container |
| Login redirects back to the login page in a loop | `externalBaseURL` mismatch (step 5) | Set `externalBaseURL` to exactly the scheme + host in the browser's address bar |
| `403` on every request | Client IP not covered by the access list (step 4) | Check `defaultAction` and the CIDRs; if a proxy sits ahead of gpm for this host, check whether `X-Forwarded-For` is trusted (see the note at the end of step 4) |
| Login "does nothing" over `http://127.0.0.1:8081` after step 5 | `externalBaseURL` is `https://`, so `auto` issues a `Secure` cookie the plain-HTTP browser drops | Reach the panel over `https://gpm.example.com`, or set `GPM_COOKIE_SECURE=0` for that break-glass session only |
| One bad password locks out every admin | `settings.trustedProxies` does not include the data-plane source address, so all attempts share one lockout bucket | Add that address (`127.0.0.1/32`, `::1/128` for a loopback bind) to `settings.trustedProxies` |
