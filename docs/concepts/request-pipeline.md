# Request pipeline

The fixed order every proxied request runs through, how the client IP is
derived once per request, and how the upstream path is composed.

## Middleware ordering

Middlewares are applied in a fixed order per request regardless of the order you
list them: **rate-limit -> access-list -> bouncer -> auth -> guard -> headers ->
location `stripPrefix` -> rewrite -> upstream `path` -> upstream**. Rate limiting is
outermost (evaluated first, so floods are shed before any work); the access-list
is evaluated ahead of the bouncer, so an explicit operator allow-list is never
overridden by an external feed's verdict, and both are ahead of auth, so a denied
or banned IP never reaches the IdP; the three path-composition steps are
innermost (closest to the backend), so every security tier above still sees the
original client path. Host-wide middlewares run before any location-scoped ones.

Path composition therefore runs: the location match (on the original client
path), then that location's `stripPrefix`, then the rewrite rules, then the
upstream's base `path`. Example: a request for `/app/old/x?z=1` on a location
`/app` with `stripPrefix: true`, a prefix rule `/old` -> `/new`, and an upstream
`path: /api`, reaches the backend as `/api/new/x?z=1`.

An inline `auth` / `rateLimit` block on a proxy host or location occupies the
**same position** as a middleware of that kind and compiles to the same handler;
it is evaluated just **before** the referenced middlewares of that kind. Setting
both is legal and both must pass. See
[Inline auth and rate limit](../reference/config/proxy-host.md#proxy-host-inline-auth-and-rate-limit).

## How the chain is compiled

**Middleware chain** (`internal/dataplane/chain.go`). Each host/location compiles
to a handler that wraps the reverse proxy in a fixed order:

```
request -> rate-limit -> access-list -> bouncer -> auth -> guard -> headers -> stripPrefix -> rewrite -> reverse proxy -> upstream
```

Rate limiting is outermost; path composition is innermost (closest to the backend).
The access-list sits ahead of auth, so an IP the list would deny is dropped
before any auth work runs (no forward-auth subrequest to the IdP, no OIDC
redirect).

The auth position takes every mode, including `basic` (HTTP basic auth against
the middleware's own bcrypt hashes, `internal/dataplane/basicauth.go`), which is
the supported home for username/password gating; `AccessList.basicAuth` is the
deprecated form of the same thing and shares the one verifier, so the two cannot
drift.

A chain step comes from one of two places, and the data plane cannot tell them
apart: a `Middleware` object the host/location **references**, or an inline
`auth` / `rateLimit` block written **on the host or location itself**. Both
compile through the same per-kind builder into the same handler at the same
position, so the gate, its metrics and its error pages are identical; the inline
block is wrapped just outside the referenced ones of its kind, so it is evaluated
first. Middleware objects are the reuse path (one gate, many hosts); the inline
block is the direct path (one host, its own gate), and a host may use both. An
inline auth block's identity provider also contributes to that host's
identity-header strip set and trusted-proxy set, exactly as a referenced auth
middleware's does.

**Request-aware access-list rules.** An IP rule may carry `paths` (and
`methods`), in which case it matches only a request to one of those exact paths
with one of those methods; without them it applies to every request exactly as
before. Ordering is unchanged - top-down, first match wins, then geo, then
`defaultAction`.

Path rules are **allow-only**, and validation refuses `action: deny` alongside
`paths`. Exact matching on the cleaned path is a sound basis for an allow rule
and an unsound one for a deny: the router preserves a trailing slash and does no
case folding, so `/admin` does not cover `/admin/` or `/ADMIN`. A missed *allow*
falls through to `defaultAction` (deny, fail closed); a missed *deny* would let
the request past (fail open). Deny-by-path is the guard middleware's job, which
owns that matching problem. With that restriction in place `paths` only ever
*narrows* a rule. The comparison is
against the already-normalised `r.URL.Path` (`normalizeRequestPath` runs at
router dispatch, and a path carrying `\` or `;` is rejected outright), which is
the same string the upstream will see, so there is no matcher/backend divergence
to exploit; a path that is not itself clean is refused at config-write time
because it could never match. This is what makes "let the uptime monitor reach
`/api/health` on a host that is otherwise VPN-only" expressible in one rule
instead of a second host. There is no request path at L4, so a path-scoped rule
never matches on a stream host, and validation refuses such a list there rather
than evaluating half the gate an operator wrote. A rule may also draw its
networks from a fetched `source` instead of a literal `cidr`; the compiled list
reads those sets from a package-level handle installed before each reload
(`SetAccessListSources`, mirroring `SetSecurityHeaders`), so a completed fetch
takes effect on the next request without threading the ledger through every
build function.

## Path composition

**Path composition** (`internal/dataplane/rewrite.go`, `proxy.go`,
`upstreamgroup.go`). The path a backend sees is built in a fixed order:
**location match -> location `stripPrefix` -> rewrite rules -> upstream base
`path` -> upstream**. The location is matched on the original, cleaned client
path; `stripPrefix` then removes that matched prefix; the rewrite middleware
then applies its rules; and the upstream's base `path` is prefixed last, riding
on the reverse proxy's target URL for a single upstream and applied per attempt
by the failover transport for an upstream group (so each group member can carry
its own base path and Host header). Every stage is wrapped innermost, so
rate-limit, access-list, bouncer, auth and guard all evaluate the ORIGINAL
client path and no composition step can carry a request past a path-scoped
security control. `RawQuery` is never touched, and none of it is ever an HTTP
redirect - the method and body reach the backend unchanged.

The **rewrite** middleware carries three rule kinds, evaluated exact, then
prefix (longest first), then regex (config order), with the first match winning
and no rule seeing another rule's output. Prefix rules are boundary-matched the
same way a location is, so `/reports` cannot capture `/reports-evil`. Regex
rules are implicitly anchored with `^`, compiled once at config load (a bad
pattern is a validation error naming the rule index, never a request-time
failure), capped at 32 rules of 256 characters, and run on Go's RE2 engine -
linear time, no backtracking, so no ReDoS class exists. Its motivating case is
repairing a client that mangles an upstream path - e.g. adding the trailing
slash a mobile OIDC client strips off Authentik's `/application/o/token`
endpoint, which Django would otherwise answer `405`. The reverse proxy sets `X-Forwarded-*`, preserves the client `Host` (unless the
upstream sets `hostHeader` to `upstream` or an explicit hostname), and
carries WebSocket upgrades transparently. Redirects that an upstream emits to its
own address are rewritten to the public scheme/host.

## Client IP and the three trust tiers

Every IP-based rule in gpm compares **one** address, derived once per request.
This section defines it; the rest of this page links here rather than repeating
it.

### The three tiers

Each tier is a separate grant. None substitutes for another, and none is
inferred from another.

| Tier | Setting | What it decides | Compared against |
|------|---------|-----------------|------------------|
| L4 | `settings.proxyProtocol.trustedCIDRs` | Which peers may rewrite the connection address itself with a PROXY protocol header. | The real TCP peer. |
| L7 | `settings.trustedProxies`, or a proxy host's own `trustedProxies` | Which peers' `X-Forwarded-For` is believed when deriving the client IP. | The connection address (after L4). |
| Identity | `identityProvider.forwardAuth.trustedProxies` | Which peers may assert identity headers (`Remote-User`, `X-authentik-*`, ...). | The connection address (after L4). |

### How the client IP is derived

1. The connection address is `RemoteAddr` - or, on a PROXY protocol listener
   whose TCP peer is inside `proxyProtocol.trustedCIDRs`, the address that
   header asserts.
2. If that address is **not** inside the host's effective `trustedProxies` set,
   it **is** the client IP and `X-Forwarded-For` is never read.
3. Otherwise gpm walks `X-Forwarded-For` from the right and takes the first
   entry that is not itself a trusted proxy (the rightmost-untrusted
   algorithm). If every entry is trusted, the connection address stands.

The effective set for a host is its own `trustedProxies` when it declares one,
otherwise `settings.trustedProxies`. The per-host key is **nullable and
three-state** - omitted inherits the fleet list, `[]` trusts nobody, a non-empty
list replaces the fleet list - see
[Per-host override](../reference/config/settings/trusted-proxies.md#per-host-override-absent-is-not-the-same-as-empty).

**Bounds on the walk.** The rightmost-untrusted walk is deliberately strict, so
a header gpm cannot fully read never becomes a client identity:

- **An entry that does not parse as an address ends the walk** and gpm falls
  back to the connection peer. A token it cannot read - an RFC 7239 obfuscated
  identifier such as `_abc`, the literal `unknown`, a `unix:` marker - is
  evidence the chain is not the one you configured, so it is not guessed past.
- **Unspecified (`0.0.0.0`, `::`), broadcast and multicast addresses are treated
  as unparseable** and never become the client IP.
- **At most 64 entries are examined**, counted from the right. The
  `X-Forwarded-For` gpm sends upstream is rebuilt from parsed addresses only and
  is bounded the same way, so a long or malformed inbound header cannot be
  amplified onward.

### What uses the derived client IP

Everything, with no per-feature configuration:

- **Access control** - `AccessList` rules and sources, geo rules.
- **Exemptions** - `allowFrom` on guard, rate-limit, bouncer, `auth-request`
  auth and `client-cert` auth middlewares.
- **Throttling** - the rate-limit bucket key and the basic-auth lockout key.
- **Outbound** - the `X-Real-IP` and `X-Forwarded-For` gpm sends to the
  upstream, to a forward-auth/auth-request server, and to a bouncer.
- **Observability** - the access log `client` field and the `/api/logs` viewer.
- **Admin login throttling** - the admin listener derives the login and TOTP
  lockout key with the **same** function and the same `settings.trustedProxies`
  list, so a proxied admin panel does not collapse every operator into one
  lockout bucket. See
  [Reach the admin UI through gpm itself](../how-to/admin-ui-behind-gpm.md).
- **Upstream stickiness** - the `ip-hash` upstream-group policy hashes the
  derived client IP, not `RemoteAddr`.

### Defaults and sharp edges

- **The default is trust nobody.** With `trustedProxies` empty (the shipped
  default), `X-Forwarded-For` is ignored entirely and the connection peer is the
  client. That is correct when gpm is the internet-facing edge.
- **If gpm is not the edge, you must declare the proxy.** Until you do, every
  request through that proxy is attributed to the proxy's own address: one
  shared rate-limit bucket, one address in the access log, and any `allowFrom`
  network containing the proxy exempts **everyone** who arrives through it. On a
  `client-cert` host that is a total mTLS bypass.
- **Never list `0.0.0.0/0` or `::/0`.** Trusting every peer means believing any
  `X-Forwarded-For`, which hands every client full control of the address every
  rule above compares. gpm accepts it (an operator on a closed network may mean
  it) and logs a warning on every reload.
- **`forwardAuth.trustedProxies` does not affect the client IP.** It governs
  identity headers only. If your config relies on it for IP resolution, gpm logs
  a warning at load with the exact `settings.trustedProxies` block to add.
- **PROXY protocol is the alternative when gpm is behind an L4 balancer.** It
  replaces the connection address before any of this runs, so no header trust is
  needed at all.

```yaml
# config/settings.yaml - gpm sits behind two edge proxies
trustedProxies:
  - 192.0.2.10/32
  - 2001:db8:1::/64
```

```yaml
# config/proxy-hosts/direct.yaml - this one host is reached directly
name: direct
domains: [direct.example.com]
upstream: {scheme: http, host: 192.0.2.20, port: 8080}
trustedProxies: [10.0.0.0/8]   # replaces the fleet list for this host
```

```yaml
# config/proxy-hosts/edge-only.yaml - trust nobody here, despite the fleet list
name: edge-only
domains: [edge.example.com]
upstream: {scheme: http, host: 192.0.2.21, port: 8080}
trustedProxies: []             # present but empty: not the same as omitting it
```

## Which IP `allowFrom` compares

The one derived client IP - see [Client IP and the three trust
tiers](#client-ip-and-the-three-trust-tiers). Two consequences worth stating
here, because `allowFrom` on a `client-cert` middleware is the highest-stakes
place they apply:

- **If gpm is the edge**, `allowFrom` means what it reads like: the machine that
  opened the connection. This is the default (`trustedProxies` empty) and is
  fail-safe - a client cannot forge a header gpm does not read.
- **If gpm is behind an L7 proxy and you have not declared it in
  `trustedProxies`**, every request through that proxy is attributed to the
  proxy's own address. Should that address fall inside an `allowFrom` network -
  a Docker or Kubernetes bridge address, a sidecar, a `10.0.0.0/8` that happens
  to contain the pod network - then **every** request arriving through it is
  exempt, from the internet as much as from the LAN. On a `client-cert` host
  that is a total mTLS bypass, and nothing logs it as unusual: the requests
  simply succeed.

Fix it by declaring the proxy, per fleet or per host:

```yaml
# config/settings.yaml
trustedProxies: [192.0.2.10/32]
```

Alternatives when you cannot: put the network rule in an
[AccessList](../reference/config/access-list.md) instead, terminate the LAN on a
separate host name with its own policy, or - behind an L4 balancer - enable the
[PROXY protocol](../reference/config/settings/proxy-protocol.md), which replaces
the connection address before any of this runs.
