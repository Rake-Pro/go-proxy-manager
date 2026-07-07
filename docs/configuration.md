# Configuration reference

go-proxy-manager is configured by a set of typed YAML objects in a git-backed
directory (default `/data/config`). You can edit them through the web UI / REST
API, or write the files directly and let the daemon load them on start/reload.

## Layout

```
config/
  settings.yaml            # singleton app settings
  proxy-hosts/<name>.yaml
  redirect-hosts/<name>.yaml
  stream-hosts/<name>.yaml
  dead-hosts/<name>.yaml
  certificates/<name>.yaml
  dns-providers/<name>.yaml
  identity-providers/<name>.yaml
  access-lists/<name>.yaml
  middlewares/<name>.yaml
```

One object per file; the file's base name must equal the object's `name`. The
directory is a git repository — every change made through the API is a commit,
and the whole graph is validated before it is accepted (a reference to a
non-existent certificate, middleware, access list, identity provider, or DNS
provider is a load-time error, and an object cannot be deleted while another
references it).

## Common fields (every object)

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | yes | Identity and filename. Lowercase alphanumeric plus `-_.`, must start and end alphanumeric, 1–254 chars. |
| `displayName` | string | no | Human label for the UI. |
| `labels` | map | no | Arbitrary key/value metadata. |
| `tags` | []string | no | Flat, free-form labels for grouping/filtering. On the Proxy Hosts list they render as chips and are matched by the filter box. |
| `disabled` | bool | no | Keep the object in config but exclude it from the running data plane. |

## Secrets

Secret-valued fields (API tokens, client secrets, etc.) must be **placeholders**,
not literal values:

```
${ENV:CF_API_TOKEN}        # resolved from the environment variable
${FILE:/run/secrets/token} # resolved from a file (e.g. a Docker secret), trimmed
```

Placeholders resolve lazily, at the moment the secret is used. Committing a
literal secret is refused with `refusing to commit literal secret(s): ...` — on
`Save`, on `SaveSettings`, **and on `Restore`** (an uploaded backup archive cannot
smuggle a plaintext secret onto disk or into git history; a refused restore rolls
the working tree back and commits nothing). In API responses, literal secrets are
redacted to `***`; placeholders are returned verbatim.

`${FILE:...}` reads are confined to an allowlisted root, defaulting to
`/run/secrets`. A path that is relative, or outside the allowed root (including
via `..`), is refused — so a config write cannot turn a file-backed secret into
an arbitrary host-file read. Override the allowed roots with the
`GPM_SECRET_FILE_ROOTS` environment variable (a list of absolute directories,
separated by the OS path-list separator, e.g. `:` on Linux).

`${ENV:...}` resolution has two guards. gpm's own sensitive process env vars —
`GPM_SSO_SIGNING_KEY` and `GPM_LOCAL_ADMIN_PASSWORD_HASH` — are **never**
resolvable via a `${ENV:...}` placeholder, so an admin-authored config value
cannot exfiltrate them (e.g. as a webhook secret posted to an attacker URL). By
default any other env var name resolves. To lock this down further, set
`GPM_SECRET_ENV_PREFIXES` to a comma-separated list of allowed name prefixes
(e.g. `GPM_SECRET_,APP_`); then only `${ENV:...}` names carrying one of those
prefixes resolve and everything else is refused.

---

## Settings (`config/settings.yaml`)

Singleton application configuration.

| Field | Type | Notes |
|-------|------|-------|
| `schemaVersion` | int | Config schema version. |
| `appName` | string | Brand label in the UI and login page. Default `Go Proxy Manager`. |
| `externalBaseURL` | string | Canonical public URL of the admin panel. Must be an absolute URL. Used to build the OIDC `redirect_uri` so it never depends on spoofable `X-Forwarded-*` headers. |
| `adminAuth.providers` | []string | Identity-provider names allowed to log into the admin panel. |
| `adminAuth.localLoginEnabled` | bool | Keep username/password login available (anti-lockout). Default true. |
| `adminAuth.ssoOnly` | bool | Disable local login entirely. Requires at least one `providers` entry. Recovery from an SSO outage is by redeploying with local login re-enabled. |
| `webhooks` | []WebhookConfig | Outbound lifecycle notifications (below). |

**WebhookConfig**: `name` (required, name-safe identifier), `url` (required,
absolute http/https), optional `secret` (placeholder-resolved, sent as the
`X-GPM-Webhook-Secret` header), `disabled` (keep configured but do not fire). After
every successful config change gpm POSTs a JSON event
`{"action","kind","name","commit","time"}` to each enabled target. `action` is one
of `save` | `delete` | `restore` | `revert` | `settings`. Delivery is asynchronous
and best-effort under a 10s timeout — a slow or unreachable endpoint never blocks
or fails the config write, it is only logged.

```yaml
schemaVersion: 1
appName: Go Proxy Manager
externalBaseURL: https://gpm.example.com
adminAuth:
  providers: [authentik-oidc]
  localLoginEnabled: true
  ssoOnly: false
webhooks:
  - name: ci
    url: https://hooks.example.com/gpm
    secret: ${FILE:/run/secrets/gpm_webhook_secret}
```

---

## ProxyHost (`config/proxy-hosts/`)

Terminates TLS for one or more domains and reverse-proxies to an upstream.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `domains` | []string | yes | One or more hostnames served by this host. |
| `upstream` | Upstream | yes | Default backend. |
| `websocketsUpgrade` | bool | no | Offer WebSocket upgrades. |
| `robotsNoIndex` | bool | no | Emit `X-Robots-Tag: noindex, nofollow` (HTTP and HTTPS) to discourage search-engine indexing. A headers middleware that sets `X-Robots-Tag` explicitly still wins. |
| `timeouts` | HostTimeouts | no | Per-host upstream timeout overrides (below). |
| `tls` | TLSSettings | no | Certificate + TLS behaviour. |
| `middlewares` | []string | no | Host-wide middleware names, applied top-down. |
| `accessLists` | []string | no | Host-wide access-list names. |
| `locations` | []Location | no | Path-scoped overrides (below). |

**Upstream**: `scheme` (`http`|`https`), `host`, `port` (1–65535) — all required.

**TLSSettings**: `certificateRef` (a Certificate name), `forceSSL` (redirect
HTTP→HTTPS), `http2`, `hsts` (`enabled`, `maxAge` — seconds, default one year when
unset, `includeSubdomains`, `preload`), `minTLSVersion` (`"1.2"` default | `"1.3"`).
When `hsts.enabled` is set, the data plane emits `Strict-Transport-Security` on
HTTPS responses for the host (never over plain HTTP).

`minTLSVersion` is a **per-host** floor selected by SNI at handshake time. The
edge already negotiates TLS 1.2 *or* 1.3 per client (1.2 is the default floor);
set `"1.3"` only on hosts where every client supports it (drops 1.2 — old smart
TVs / embedded clients / legacy scripts may then fail to connect). Leave it unset
for public hosts to keep the widest client compatibility.

**HostTimeouts** (`timeouts`): `connectSeconds` caps establishing the TCP/TLS
connection to the upstream; `readSeconds` caps time awaiting the upstream's
response headers (time-to-first-byte). Both are whole seconds (0–3600); `0`/unset
means no override. A host with any override uses its **own** cloned transport
(its own connection pool), so a custom timeout never affects another host's
keep-alive reuse; hosts without an override share the default pooled transport.
`readSeconds` bounds only time-to-first-byte, so it does not cut off a slow
streaming / SSE / websocket body once headers have arrived.

**Location**: a path-scoped override. `path` (required), optional `upstream`
override, plus `middlewares` / `accessLists` that are **appended to** (not
replace) the host-wide chain — so a location is always at least as restrictive as
its host. Matching is longest-prefix; the request path is forwarded unchanged.

```yaml
name: app
domains: [app.example.com]
upstream: {scheme: http, host: backend, port: 8080}
websocketsUpgrade: true
tls: {certificateRef: wildcard, forceSSL: true}
middlewares: [require-sso]
locations:
  - path: /metrics
    accessLists: [internal-only]      # /metrics also requires the internal CIDR
```

---

## RedirectHost (`config/redirect-hosts/`)

Issues HTTP redirects.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `domains` | []string | yes | Source hostnames. |
| `targetDomain` | string | yes | Where to redirect. |
| `targetScheme` | string | no | `http`\|`https`\|`auto`. |
| `statusCode` | int | no | `301`\|`302`\|`307`\|`308` (0 = default). |
| `preservePath` | bool | no | Keep the request path. |
| `tls` | TLSSettings | no | |

```yaml
name: apex
domains: [example.com]
targetDomain: www.example.com
statusCode: 301
preservePath: true
```

---

## StreamHost (`config/stream-hosts/`)

Raw TCP/UDP forwarding. The data plane opens a listener per `listenPort` (TCP, UDP,
or both) and forwards to the backend; listeners are reconciled on every reload
(ports added/removed, backend swapped, with no listener restart for unchanged
ports). UDP uses per-client sessions with an idle timeout.

> **No access control at L4.** Stream hosts blind-forward: access lists, geo rules,
> rate limits, and identity/SSO are HTTP-layer controls and do **not** apply here.
> The only built-in bound is `maxUDPSessions` (4096), which caps spoofed-source UDP
> memory. Expose a stream port only on a trusted network, or put IP filtering in
> front of it at the firewall / host level. Do not publish a stream port to the
> public internet expecting gpm to gate it.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `listenPort` | int | yes | 1–65535. **Publish this port from the container** (compose `ports:`) so it is reachable, and avoid colliding with the data-plane 80/443 or admin port — a bind failure is logged and that one port is skipped, never fatal. |
| `protocol` | string | yes | `tcp`\|`udp`\|`both`. |
| `forwardHost` | string | yes | Backend host. |
| `forwardPort` | int | yes | 1–65535. |

```yaml
name: postgres
listenPort: 5432
protocol: tcp
forwardHost: db.internal
forwardPort: 5432
```

---

## DeadHost (`config/dead-hosts/`)

Returns a fixed status for claimed domains — useful to absorb unmatched vhosts and
stop default-host leakage.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `domains` | []string | yes | |
| `statusCode` | int | no | Default 404. |
| `tls` | TLSSettings | no | |

---

## Certificate (`config/certificates/`)

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `type` | string | yes | `custom` or `acme`. |
| `domains` | []string | yes | Domains the cert covers (`*.example.com` for wildcard). |
| `acme` | ACMESpec | when `type: acme` | |
| `custom` | CustomCertSpec | when `type: custom` | |

**ACMESpec**: `email` (required), `dnsProvider` (required — a DNSProvider name),
`directoryURL` (optional, defaults to Let's Encrypt production), `keyType`
(`ecdsa` default | `rsa`), `challenge` (only `dns-01` is supported).

**CustomCertSpec**: `certFile`, `keyFile` — paths **relative to the cert store**
(absolute paths and `..` are rejected). These are file references, not inline PEM.

```yaml
# ACME wildcard
name: wildcard
type: acme
domains: ["*.example.com", example.com]
acme:
  email: admin@example.com
  dnsProvider: cloudflare
  keyType: ecdsa
```
```yaml
# Bring-your-own
name: internal
type: custom
domains: [internal.example.com]
custom: {certFile: internal.crt, keyFile: internal.key}
```

Certificates are selected at TLS time by SNI: an exact-domain match wins, else a
wildcard match on the parent domain. An ACME certificate that has not been issued
yet is simply skipped until the manager issues it.

---

## DNSProvider (`config/dns-providers/`)

Solves ACME `dns-01` challenges.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `provider` | string | yes | `cloudflare`. |
| `config` | map[string]Secret | yes | Provider-specific, secret-valued. |

```yaml
name: cloudflare
provider: cloudflare
config:
  apiToken: ${FILE:/run/secrets/cf_token}   # scope: Zone:DNS:Edit + Zone:Read
```

---

## IdentityProvider (`config/identity-providers/`)

| Field | Type | Notes |
|-------|------|-------|
| `type` | string | `oidc` \| `forward-auth` \| `auth-request`. |
| `oidc` / `forwardAuth` / `authRequest` | spec | The spec matching `type`. |
| `roleMapping` | RoleMapping | Map IdP groups → roles. |

**OIDCSpec**: `issuerURL` (req), `clientID` (req), `clientSecret` (Secret),
`scopes` (default `openid profile email groups`), `usePKCE` (default true),
`requireVerifiedEmail`, `trustIdPMFA`.

**ForwardAuthSpec**: `trustedProxies` (req, CIDRs allowed to assert identity),
`userHeader` (req), `emailHeader`, `nameHeader`, `groupsHeader`,
`groupsDelimiter` (default `,`), `amrHeader`.

> `trustedProxies` is also the per-host source of truth for **client-IP
> resolution**. `X-Forwarded-For` is honoured (for access-list, rate-limit, geo,
> and auth-request `allowFrom`) only for proxies a host actually trusts — the
> `trustedProxies` of the forward-auth IdPs *that host* references — not a global
> union across every IdP. A host with no forward-auth IdP in front therefore trusts
> no `XFF` and keys IP controls off the connection peer. If you IP-filter a host
> that sits behind a real proxy, give that host a forward-auth IdP declaring the
> proxy CIDR so its forwarded client IP is trusted.

**AuthRequestSpec**: `outpostURL` (req), `pathPrefix` (default
`/outpost.goauthentik.io`), `authPath` (default `<pathPrefix>/auth/nginx`),
`copyHeaders` (default the Authentik `X-authentik-*` set).

**RoleMapping**: `groupsClaim` (default `groups`), `adminGroups`, `userGroups`,
`defaultRole` (`""` = deny | `user` | `admin`), `allowDefaultAdmin` (bool; must be
`true` when `defaultRole` is `"admin"` — prevents silently granting admin to every
authenticated user when no group gating is configured).

```yaml
name: authentik-oidc
type: oidc
oidc:
  issuerURL: https://auth.example.com/application/o/gpm/
  clientID: gpm
  clientSecret: ${FILE:/run/secrets/oidc_secret}
roleMapping:
  adminGroups: [proxy-admins]      # no defaultRole -> anyone not in the group is denied
```

> The OIDC client reads claims from the **ID token**, so if your provider only
> emits groups via the userinfo endpoint you must configure it to include the
> groups claim in the ID token.

> **SSO session lifetime / offboarding.** For `type: oidc` hosts, gpm mints a
> signed `__Host-gpm_sso` session cookie with a **1-hour absolute TTL** (not a sliding
> window — it is not extended by activity). On expiry the next request re-runs the
> OIDC flow against the IdP, which is silent when the IdP session is still valid
> and re-checks group membership. This bounds the offboarding window: a user
> removed from a group or disabled at the IdP loses data-plane access within an
> hour, without gpm holding server-side session state. There is no per-request
> revocation — if you need instant cutoff, revoke at the IdP and, where it matters,
> restart is not required but access ends at the next hourly re-auth.

---

## AccessList (`config/access-lists/`)

| Field | Type | Notes |
|-------|------|-------|
| `satisfyAny` | bool | false = require both auth AND IP; true = either suffices. |
| `basicAuth` | []BasicAuthUser | `username` + `passwordHash` (bcrypt). |
| `rules` | []IPRule | Ordered `action` (`allow`/`deny`) + `cidr` (CIDR or bare IP). |
| `defaultAction` | string | `allow` \| `deny` (default `deny`). |

```yaml
name: internal-only
rules:
  - {action: allow, cidr: 10.0.0.0/8}
  - {action: allow, cidr: 192.168.0.0/16}
defaultAction: deny
```

---

## Middleware (`config/middlewares/`)

| `type` | Spec | Purpose |
|--------|------|---------|
| `auth` | AuthMiddleware | Require authentication. |
| `headers` | HeadersMiddleware | Add/remove request/response headers. |
| `guard` | GuardMiddleware | Conditionally deny requests. |
| `rate-limit` | RateLimitMiddleware | Per-host rate limiting. |

**AuthMiddleware**: `identityProvider` (req), `mode` (`oidc`|`forward-auth`|
`auth-request`, defaults from the IdP type), `requiredRoles` (forbidden in
`auth-request` mode — the IdP application binding does authorization),
`allowFrom` (CIDRs that bypass auth; `auth-request` mode only — e.g. let a LAN
skip SSO).

In `oidc` mode gpm is itself the OIDC relying party for the host: an
unauthenticated request is redirected to the IdP, the reserved callback path
`/__gpm/oidc/callback` exchanges the code (PKCE + nonce) and sets a signed,
stateless SSO session cookie, and `requiredRoles` (via the IdP's `roleMapping`)
is enforced; with no role mapping or required roles the gate just requires a
valid login. Register `https://<host>/__gpm/oidc/callback` as a redirect URI on
the IdP. The signing key is auto-generated and persisted at
`<cert-dir>/sso_signing.key` (0600) on first use, so sessions survive restarts
without any operator action. Set `GPM_SSO_SIGNING_KEY` explicitly to supply your
own key (useful when rotating or sharing a key across instances).

**HeadersMiddleware**: `setRequest`, `setResponse` (maps), `removeRequest`,
`removeResponse` (lists).

**GuardMiddleware**: `triggers` (≥1; each has `paths`, `methods`, `queryEquals`
and matches when all set fields match), `allowFrom` (exempt CIDRs), `denyStatus`
(default 403).

**RateLimitMiddleware**: `requestsPerSecond` (req, >0), `burst` (default
`ceil(rps)`). Enforced as a per-host, per-client-IP token bucket (capacity =
`burst`, refill = `requestsPerSecond`/sec). Over-limit requests get `429 Too Many
Requests` with a `Retry-After` header; the request is not proxied. The client IP
is resolved the same XFF-aware way as access lists; a request whose client IP
cannot be resolved falls back to a single shared bucket (fail-safe, never
unlimited). The middleware sits **outermost** in the chain (evaluated first) so a
flood is shed before it can drive an auth subrequest or any other per-request
work: rate-limit → access-list → auth → guard → headers → upstream.

```yaml
# Require SSO, but let the LAN through without it
name: require-sso
type: auth
auth:
  identityProvider: authentik-outpost
  mode: auth-request
  allowFrom: [10.0.0.0/8]
```
```yaml
# Block POSTs to a login path except from the LAN (break-glass guard)
name: login-lan-only
type: guard
guard:
  triggers:
    - {paths: [/login], methods: [POST]}
  allowFrom: [10.0.0.0/8]
```

### Middleware ordering

Middlewares are applied in a fixed order per request regardless of the order you
list them: **rate-limit → access-list → auth → guard → headers → upstream**. Rate
limiting is outermost (evaluated first, so floods are shed before any work); the
access-list is evaluated ahead of auth, so a denied IP never reaches the IdP;
header mutations are innermost (closest to the backend). Host-wide middlewares run
before any location-scoped ones.
