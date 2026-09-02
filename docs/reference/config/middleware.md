# Middleware

A reusable policy object attached to a proxy host or a location by name. One
object carries exactly one `type` and its matching spec.

## Types

| `type` | Spec | Purpose |
|--------|------|---------|
| `auth` | AuthMiddleware | Require authentication. |
| `headers` | HeadersMiddleware | Add/remove request/response headers. |
| `guard` | GuardMiddleware | Conditionally deny requests. |
| `rate-limit` | RateLimitMiddleware | Per-host rate limiting. |
| `rewrite` | RewriteMiddleware | Request-path replacement (upstream-facing): exact, prefix or regex rules. |
| `bouncer` | BouncerMiddleware | Deny hook: ask an external bouncer (CrowdSec LAPI or any HTTP endpoint) whether the client IP is banned. |

## AuthMiddleware (`auth`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-auth-identity-provider"></span>  `identityProvider` | string | - | yes, except in `client-cert` and `basic` mode | Name of an [IdentityProvider](identity-provider.md). Naming one in `client-cert` or `basic` mode is an error. A dangling name is a load-time error, and the data plane fails closed with `503` if one ever reaches it. |
| <span id="middleware-auth-mode"></span>  `mode` | string | the provider's `type` | no | `oidc` \| `forward-auth` \| `auth-request` \| `client-cert` \| `basic`. See the mode table below. |
| <span id="middleware-auth-required-roles"></span>  `requiredRoles` | []string | none | no | Roles from the provider's `roleMapping` that may pass. **Refused in `auth-request` mode** (the IdP application binding does authorization) and in `basic` mode (a username carries no roles). |
| <span id="middleware-auth-allow-from"></span>  `allowFrom` | []string | none | no | CIDRs exempt from this gate, compared against the derived client IP. Valid in `auth-request`, `client-cert` and `basic` mode only; **refused in `oidc` and `forward-auth` mode**, where the gate has no bypass to honour. With `mode` unset the effective mode is the provider's `type`, so an exemption against an `oidc` or `forward-auth` provider is refused too - set `mode` explicitly rather than relying on the default. See [Which IP `allowFrom` compares](../../concepts/request-pipeline.md#which-ip-allowfrom-compares). |
| <span id="middleware-auth-client-cert-roles"></span>  `clientCertRoles` | map[string]string | none | no | `client-cert` mode only. Maps a certificate subject (RFC 2253, or its bare common name) to a role that `requiredRoles` is checked against. `requiredRoles` without a mapping is refused at validation. |
| <span id="middleware-auth-basic"></span>  `basic` | BasicAuthSpec | none | yes in `basic` mode | Local credentials for `basic` mode. Table below. Setting it in any other mode is an error. |

### Auth modes

| `mode` | Identity source | Notes |
|--------|-----------------|-------|
| `oidc` | The IdP, via a browser redirect | gpm is the relying party for the host; a signed SSO cookie admits later requests. |
| `forward-auth` | A trusted upstream proxy's identity headers | Only headers asserted by the provider's `trustedProxies` are believed. |
| `auth-request` | An external `auth_request` endpoint | The auth server does authorization; `requiredRoles` is refused. |
| `client-cert` | The TLS handshake | Names no `identityProvider`; pair with `tls.clientAuth.mode: optional`. |
| `basic` | The middleware's own `basic.users` | Names no `identityProvider`; HTTP basic auth against local bcrypt hashes. |

### BasicAuthSpec (`auth.basic`, `basic` mode only)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-auth-basic-users"></span>  `users` | []BasicAuthUser | - | yes | Accepted credentials. At least one, at most 64. |
| <span id="middleware-auth-basic-users-username"></span>  `users[].username` | string | - | yes | Compared in full; no `:` or line break. |
| <span id="middleware-auth-basic-users-password-hash"></span>  `users[].passwordHash` | string | - | yes | bcrypt hash (`$2a$`/`$2b$`/`$2y$`, 60 chars). Anything else is refused at write time. |
| <span id="middleware-auth-basic-realm"></span>  `realm` | string | the host name | no | Realm in the `WWW-Authenticate` challenge. Printable ASCII, no `"` or `\`, at most 128 chars. |

```yaml
name: internal-basic
type: auth
auth:
  mode: basic
  allowFrom: [10.0.0.0/8]     # the LAN skips the password
  basic:
    realm: Internal
    users:
      - username: admin
        passwordHash: $2a$12$D4G5f18o7aMMfwasBL7GpuQWuP3pkrZrOAnqP.bmezbMng.QwJ/Bu
```

Generate a hash with `htpasswd -nbB admin 'hunter2'` (the part after the colon),
or POST a plaintext `password` field instead of `passwordHash` and gpm hashes it
server-side - on a `type: auth` middleware and on an inline `auth` block on a
proxy host or one of its locations alike. A password is never stored and never
returned: only the hash reaches the git-backed config and the API response.

The gate behaves exactly like the deprecated access-list basic auth it replaces -
same `401` and `WWW-Authenticate` challenge, same per-client-IP lockout after 5
failures for 15 minutes (answered identically to a wrong password, so the response
is no oracle), same bounded bcrypt work. What it adds is the treatment every other
auth mode has: it sits at the auth position of the chain (so a rate limit, an
access list and a bouncer all still run outside it), the host's custom error pages
render the refusal, denials are counted, and `allowFrom` exempts trusted networks.

In `client-cert` mode the identity comes from the TLS handshake, so no
`identityProvider` is named (setting one is an error). The gate admits a request
only when the handshake **verified** a client certificate for this host - i.e.
the host runs `tls.clientAuth` and the trust anchor (and its CRL, if configured)
accepted the certificate; otherwise it replies `401`. `clientCertRoles` maps a
certificate subject to a role: the key is the RFC 2253 subject (`CN=ops,O=Corp`)
or its bare common name, the value is the role `requiredRoles` is checked
against. With no mapping, any verified certificate passes; with a mapping, an
unmapped subject or an insufficient role gets `403`. `requiredRoles` without a
mapping is refused at validation (it could never match).

Pair it with `tls.clientAuth.mode: optional` so certless clients still reach the
chain and this middleware is what refuses them, leaving an SSO middleware free to
cover other hosts or locations.

`allowFrom` works here exactly as it does in `auth-request` mode: a client whose
resolved IP falls in one of the listed CIDRs is proxied **straight through** - no
certificate requirement and no `clientCertRoles` check at all. That is the "the LAN
does not need a client certificate, the internet does" shape: run
`tls.clientAuth.mode: optional`, list the LAN in `allowFrom`, and every other client
gets `401` without a verified certificate. An exempt, certless request necessarily
reaches the upstream with **no** client-certificate identity headers: those are set
only from a handshake-verified certificate, and it has none.

```yaml
name: cert-or-lan
type: auth
auth:
  mode: client-cert
  allowFrom: [10.0.0.0/8]   # LAN skips the certificate requirement
```

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

## HeadersMiddleware (`headers`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-headers-set-request"></span>  `setRequest` | map[string]string | none | no | Request headers set on the way to the upstream, overwriting whatever the client sent. |
| <span id="middleware-headers-set-response"></span>  `setResponse` | map[string]string | none | no | Response headers set on the way back, overwriting the upstream's value. Not set-if-absent, unlike [`securityHeaders`](settings/security-headers.md). |
| <span id="middleware-headers-remove-request"></span>  `removeRequest` | []string | none | no | Request headers deleted before the upstream sees them. |
| <span id="middleware-headers-remove-response"></span>  `removeResponse` | []string | none | no | Response headers deleted, **only where this middleware is attached** and from inside the chain, so a response an auth gate wrote ahead of it is untouched. For edge-wide removal of leaked backend headers prefer [`stripResponseHeaders`](settings/security-headers.md#strip-response-headers-section), which runs in the reverse proxy on the upstream's own response for every response that upstream returns. |

At least one of the four must be set.

## GuardMiddleware (`guard`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-guard-triggers"></span>  `triggers` | []GuardTrigger | - | yes | At least one. A trigger fires when **every** field it sets matches; any firing trigger denies. |
| <span id="middleware-guard-allow-from"></span>  `allowFrom` | []string | none | no | CIDRs exempt from this guard's deny, compared against the derived client IP. See [Which IP `allowFrom` compares](../../concepts/request-pipeline.md#which-ip-allowfrom-compares). |
| <span id="middleware-guard-deny-status"></span>  `denyStatus` | int | `403` | no | Status returned when a trigger fires and the client is not exempt. |

### GuardTrigger (`guard.triggers[]`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-guard-triggers-paths"></span>  `paths` | []string | none | no | Exact request paths this trigger covers. Matching is exact - see [the four path mechanisms](../../concepts/which-mechanism.md#four-path-aware-mechanisms). |
| <span id="middleware-guard-triggers-methods"></span>  `methods` | []string | none | no | Upper-case HTTP methods this trigger covers. |
| <span id="middleware-guard-triggers-query-equals"></span>  `queryEquals` | map[string]string | none | no | Query parameters that must equal these values for the trigger to fire. See the `;` note below. |

A trigger with no field set matches every request, which is a deny-all guard;
that is legal and occasionally what you want behind an `allowFrom`.

> **`queryEquals` and `;`.** A guard carrying any `queryEquals` trigger rejects a
> request whose raw query string contains a `;`, with **400** (before the
> allow/deny decision, so `allowFrom` does not exempt it). gpm parses the query
> the modern way, where only `&` separates parameters, so `?a=1;direct=1` is one
> parameter `a` with the value `1;direct=1` and a `direct: "1"` trigger would not
> fire - but the raw query is forwarded to the upstream unchanged, and a backend
> still honouring the legacy `;` separator would read `direct=1` and act on it.
> Rather than evaluate a query it cannot read the same way the upstream will, the
> guard fails closed. This mirrors the same rule for `;` in request paths. Guards
> with no `queryEquals` trigger are unaffected, as is every other middleware.

## RewriteMiddleware (`rewrite`)

Replaces the request path on the way to the upstream. It carries three kinds of
rule; at least one rule of any kind is required.

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-rewrite-replace-path"></span>  `replacePath` | map[string]string | none | one of | **Exact** match: the incoming path must equal the key. Key and value must be absolute paths that satisfy the `to` rules below; a key may not map to itself. |
| <span id="middleware-rewrite-prefix-rules"></span>  `prefixRules` | []RewriteRule | none | one of | **Prefix** match on a segment boundary (the path equals `from`, or continues with `/`), so `/reports` never captures `/reports-evil`. The longest matching `from` wins. Max 32 rules. |
| <span id="middleware-rewrite-regex-rules"></span>  `regexRules` | []RewriteRule | none | one of | **Regex** match, implicitly anchored with `^`. `to` may reference capture groups as `$1` / `${name}`. Max 32 rules, max 256 characters per pattern. |

### RewriteRule (`prefixRules[]` / `regexRules[]`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-rewrite-rules-from"></span>  `from` | string | - | yes | The prefix (for `prefixRules`) or the pattern (for `regexRules`) to match. |
| <span id="middleware-rewrite-rules-to"></span>  `to` | string | - | yes | The replacement path, absolute and confined by the rules below. |

**Every `to` is confined, on the same rules as `upstream.path`.** It must be an
absolute path with **no `.` or `..` segment, no backslash, no `;`, and no query
string or fragment**. The rule holds for a regex replacement template too - `$1`
expands at request time, so the composed path is re-cleaned and re-confined to
the upstream base path before it is forwarded, and a rewritten path that still
carries a path separator an upstream could re-interpret is answered `400`.

The confinement is not cosmetic: a rewrite runs inside the security tiers, and
`middlewares:write` is a narrower scope than `proxy-hosts:write`, so a dot
segment here would let a middleware-scoped token escape the base path an
operator pinned a backend into.

Evaluation order is fixed: **exact, then prefix (longest first), then regex (in
config order)**. The **first** match wins and no rule ever sees another rule's
output, so rules cannot chain into a path nobody wrote.

- **Prefix rules** replace the matched prefix and append the remainder: with
  `from: /old`, `to: /new`, `/old/thing` becomes `/new/thing` and `/old` becomes
  `/new`.
- **Regex rules** replace the span matched at the start of the path and append
  the remainder: with `from: /user/([0-9]+)`, `to: /u/$1`, `/user/42/profile`
  becomes `/u/42/profile`. Patterns are compiled once at config load, so a
  pattern that does not compile is a **validation error naming the rule index**,
  never a request-time failure. Go's `regexp` is RE2 (linear time, no
  backtracking), so a pattern cannot be made to blow up on a crafted path.
- **Never a redirect.** The rewrite is internal: it mutates the proxied path in
  place, preserving the method and body (a `POST` is forwarded unchanged) and the
  client sees no 3xx. The query string is forwarded untouched.
- **Security position.** Rewrites run **innermost** (closest to the upstream), so
  rate limiting, access lists, the bouncer, auth and guards all evaluate the
  ORIGINAL client path; a rewrite can never move a request past a path-scoped
  security control.

```yaml
name: legacy-paths
type: rewrite
rewrite:
  replacePath:
    /application/o/token: /application/o/token/
  prefixRules:
    - from: /old-app
      to: /app
  regexRules:
    - from: /user/([0-9]+)
      to: /u/$1
```

## RateLimitMiddleware (`rateLimit`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-rate-limit-requests"></span>  `requests` | float | - | one of | "N requests per `window`". Use this for limits that do not reduce cleanly to a per-second rate, e.g. `100` per `1m` or `5` per `1h`. Set with `window`. |
| <span id="middleware-rate-limit-window"></span>  `window` | string | - | with `requests` | Go duration string (`"1s"`, `"10s"`, `"1m"`, `"1h"`, ...) the `requests` count applies over. |
| <span id="middleware-rate-limit-requests-per-second"></span>  `requestsPerSecond` | float | - | one of | Legacy shorthand, `> 0`, equivalent to `requests: <value>` with `window: "1s"`. Kept for backward compatibility; new configs should prefer `requests`/`window`. Mutually exclusive with them. |
| <span id="middleware-rate-limit-burst"></span>  `burst` | int | `ceil(requests)` | no | Token-bucket capacity: how far above the steady rate a short spike may go. |
| <span id="middleware-rate-limit-allow-from"></span>  `allowFrom` | []string | none | no | CIDRs exempt from rate limiting entirely - no token consumed, no `429` - compared against the derived client IP. See [Which IP `allowFrom` compares](../../concepts/request-pipeline.md#which-ip-allowfrom-compares). |
| <span id="middleware-rate-limit-block-for"></span>  `blockFor` | string | none | no | Go duration. Adds a fixed lockout on top of the bucket - see below. Omitted means the token bucket alone governs. |

Exactly one of `requests`+`window` or `requestsPerSecond` must be set.

Enforced as a per-host, per-client-IP token bucket (capacity = `burst`, refill
= `requests`/`window` in tokens/sec). Over-limit requests get `429 Too Many
Requests` with a `Retry-After` header computed from the refill rate (a slow
limit like `5` per `1h` can report a Retry-After of several minutes); the
request is not proxied. The bucket is keyed on the derived client IP (see
[Client IP and the three trust tiers](../../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers));
a request whose client IP cannot be resolved falls back to a
single shared bucket (fail-safe, never unlimited, and never matches
`allowFrom`). The middleware sits **outermost** in the chain (evaluated first)
so a flood is shed before it can drive an auth subrequest or any other
per-request work: rate-limit -> access-list -> bouncer -> auth -> guard -> headers ->
rewrite -> upstream.

`blockFor` (a Go duration string, e.g. `"30s"`, `"5m"`) adds an extra,
harsher penalty on top of the token bucket: the first request that exceeds
the limit blocks that client for `blockFor`, and every request from it is
rejected (`429`, `Retry-After` counting down to the end of the block) for the
whole period - independent of token refill, so a client that merely pauses
and resumes cannot slip back through once tokens would otherwise have
refilled. The block is **fixed, not sliding**: repeat requests during the
block do not push it back out, so it always expires exactly `blockFor` after
the trip that started it. Once it expires, ordinary token-bucket rules
resume (the bucket has been refilling in the background the whole time, up
to `burst`, so the client gets a normal allotment, not an instant re-burst).
Omit `blockFor` (the default) for today's behavior: only the token bucket
governs, and a client that outwaits the refill rate is let back through
immediately.

## BouncerMiddleware (`bouncer`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="middleware-bouncer-provider"></span>  `provider` | string | `crowdsec` | no | `crowdsec` \| `http`. Selects the verdict protocol. |
| <span id="middleware-bouncer-url"></span>  `url` | string | - | yes | Base URL of the bouncer. For `crowdsec`, the LAPI root; for `http`, the endpoint that answers the query below. |
| <span id="middleware-bouncer-api-key"></span>  `apiKey` | Secret | none | no | Sent as `X-Api-Key`. Must be a `${ENV:...}` / `${FILE:...}` placeholder; a literal is refused at commit. |
| <span id="middleware-bouncer-timeout"></span>  `timeout` | string | `2s` | no | Go duration for one verdict lookup. |
| <span id="middleware-bouncer-cache-ttl"></span>  `cacheTTL` | string | `60s` | no | How long a verdict is cached per client IP. In `stream` mode this is also the delta-pull interval. A verdict derived from an **error** is capped at `5s` regardless. |
| <span id="middleware-bouncer-cache-max-entries"></span>  `cacheMaxEntries` | int | `10000` | no | LRU bound on the verdict cache, so a rotating-source-IP flood cannot grow it without bound. |
| <span id="middleware-bouncer-on-error"></span>  `onError` | string | `fail-open` | no | `fail-open` \| `fail-closed`. What an unanswerable lookup means - see below. |
| <span id="middleware-bouncer-deny-status"></span>  `denyStatus` | int | `403` | no | Status returned on a deny verdict. |
| <span id="middleware-bouncer-deny-with"></span>  `denyWith` | string | `error-page` | no | `error-page` renders the host's configured page for `denyStatus`; `plain` sends a bare status body, giving a scanner nothing to fingerprint. |
| <span id="middleware-bouncer-stream"></span>  `stream` | bool | `false` | no | `crowdsec` only. Pull the whole decision set once and delta it every `cacheTTL`, so the request hot path never calls the LAPI. |
| <span id="middleware-bouncer-allow-from"></span>  `allowFrom` | []string | none | no | CIDRs exempt from the external verdict, compared against the derived client IP. See [Which IP `allowFrom` compares](../../concepts/request-pipeline.md#which-ip-allowfrom-compares). |

This is a **hook, not a WAF**. gpm ships no rules, no signatures and no
detection engine: it asks an operator-run bouncer whether the client IP is
currently banned and acts on that verdict. What "banned" means lives entirely
outside gpm.

It sits **after the access list and before auth**: an operator allow-list still
wins outright (it is evaluated first, so an explicitly allowed IP is never
overridden by an external feed), and a banned IP never reaches the IdP - no
forward-auth subrequest, no OIDC redirect. A denial is reported to the per-host
denial counter.

**`crowdsec` provider.** Per uncached client IP, gpm calls the LAPI bouncer
endpoint `GET {url}/v1/decisions?ip=<client>` with `X-Api-Key: <apiKey>`. A
`null` or empty body means "no decisions" (allow); any decision of type `ban`
or `captcha` denies. **`captcha` is treated as a deny**: it is the LAPI telling
the bouncer this client must prove it is human, and gpm has no captcha flow to
hand it - serving the request anyway would silently downgrade the operator's
decision to an allow. Decision types gpm does not implement are ignored rather
than guessed at. The LAPI resolves range (CIDR) decisions itself, so one `ip=`
query covers those too.

**`stream: true`** (crowdsec only) swaps the per-IP lookup for a local one: gpm
pulls the whole decision set once
(`GET {url}/v1/decisions/stream?startup=true`) and then deltas every `cacheTTL`,
keeping the banned IPs and CIDR ranges in memory, so the request hot path never
calls the LAPI at all. Only the very first request waits on the startup pull;
refreshes happen in the background while the current set keeps serving, and a
failed refresh logs and keeps the previous set rather than dropping it. Use it
on a busy edge or with a large decision set; leave it off for the simpler
live-lookup mode.

**`http` provider** is a generic deny hook so any custom bouncer can plug in:

```
GET {url}?ip=<client>&host=<host>&path=<path>
X-Forwarded-For: <client>          # the RESOLVED client IP, not the inbound header
X-Original-URL: <absolute request URL>
X-Api-Key: <apiKey>                # only when apiKey is set
```

`2xx` = allow, `403` = deny, **anything else** = no usable answer, so `onError`
governs. The contract is deliberately trivial: a shell script, a fail2ban shim
or a corporate threat feed can implement it in a few lines.

`onError` covers a timeout, a connection failure, an unexpected status, an
undecodable body, an unresolvable `apiKey` secret, and a client IP that cannot
be resolved at all. It defaults to **`fail-open`** (allow): an unreachable
threat feed must not take the site down, which is the opposite of the right
default for auth. Choose `fail-closed` when the bouncer is a hard requirement
and you would rather serve `403` than serve an unvetted client.

Verdicts are cached per middleware, keyed by client IP, for `cacheTTL`, bounded
at `cacheMaxEntries` with LRU eviction (so a rotating-source-IP flood cannot
grow it without bound). A verdict derived from an **error** rather than a real
answer is capped at **5s** regardless of `cacheTTL`, so an outage cannot pin a
minute of guessed verdicts and keep guessing long after the bouncer recovered.

`denyWith: error-page` (the default) renders the host's configured custom error
page for `denyStatus`, falling back to the plain status body when none is
configured; `plain` opts out of the custom page deliberately -
a bare status body gives a scanner nothing to fingerprint.

**CrowdSec quickstart.** On the host running the CrowdSec LAPI:

```
cscli bouncers add gpm
```

Copy the printed key into the environment gpm runs with (e.g.
`CROWDSEC_BOUNCER_KEY`) and reference it as a placeholder - never commit the
literal:

```yaml
name: crowdsec
type: bouncer
bouncer:
  provider: crowdsec
  url: http://crowdsec:8080
  apiKey: ${ENV:CROWDSEC_BOUNCER_KEY}
  stream: true          # local lookups; deltas pulled every cacheTTL
  cacheTTL: 60s
  onError: fail-open    # an unreachable LAPI must not take the site down
```

Verify with `cscli decisions add --ip <your-test-ip> --duration 1m` and confirm
the host answers `403`, then `cscli decisions delete --ip <your-test-ip>`.

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
```yaml
# Rate-limit the host to 100 requests/minute, but let the LAN through uncapped
name: api-rate-limit
type: rate-limit
rateLimit:
  requests: 100
  window: 1m
  burst: 150
  allowFrom: [10.0.0.0/8]
```
```yaml
# Legacy shorthand (requests per second); equivalent to requests: 10, window: 1s
name: api-rate-limit-legacy
type: rate-limit
rateLimit:
  requestsPerSecond: 10
  burst: 20
  allowFrom: [10.0.0.0/8]
```
```yaml
# Trip the limit and the client is locked out for 5 minutes, regardless of
# token refill - not just throttled back to the steady-state rate.
name: api-rate-limit-blocked
type: rate-limit
rateLimit:
  requests: 100
  window: 1m
  blockFor: 5m
```
```yaml
# Generic deny hook: any endpoint answering 2xx/403. Fail closed - this bouncer
# is a hard requirement, so an outage denies rather than admits.
name: threat-feed
type: bouncer
bouncer:
  provider: http
  url: https://bouncer.internal/check
  apiKey: ${FILE:/run/secrets/bouncer_key}
  timeout: 1s
  cacheTTL: 30s
  onError: fail-closed
  denyStatus: 403
```
```yaml
# Add the trailing slash a client strips off Authentik's token endpoint, so the
# request reaches Django as /application/o/token/ (POST + body preserved) instead
# of getting a 405. Exact-match, upstream-facing only.
name: authentik-token-slash
type: rewrite
rewrite:
  replacePath:
    /application/o/token: /application/o/token/
```
