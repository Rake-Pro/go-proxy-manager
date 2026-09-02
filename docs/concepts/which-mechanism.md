# Which mechanism do I use?

Several gpm features look alike from the outside. This page is the decision
table for each pair that gets confused, with a link to the full reference.

## Gate, exemption, trust

Three concepts wear similar names. Learn these three words and most of the
overlap disappears.

| Concept | Where it lives | What it decides |
|---|---|---|
| **Gate** | [`AccessList`](../reference/config/access-list.md) (`rules`, `sources`, `geo`, `defaultAction`) | Who is admitted at all. Ordered, first match wins, then geo, then `defaultAction`. |
| **Exemption** | `allowFrom` on the guard, rate-limit, bouncer, `auth-request` auth, `client-cert` auth and `basic` auth middlewares | Which networks skip **one specific control**. Never the access list. |
| **Trust** | [`settings.proxyProtocol.trustedCIDRs`](../reference/config/settings/proxy-protocol.md), [`settings.trustedProxies`](../reference/config/settings/trusted-proxies.md), `identityProvider.forwardAuth.trustedProxies` | Which address the gate and the exemptions compare, and who may assert an identity. |

An access list decides who is admitted. An exemption lets a network skip one
specific control, never the access list. Trusted proxies decide which address
both of those compare against: see
[Client IP and the three trust tiers](request-pipeline.md#client-ip-and-the-three-trust-tiers).

**"Only the LAN may reach this host" is an access list**, not an `allowFrom`.
`allowFrom` on a middleware only lifts that middleware's own check; with no
access list attached, everything else still reaches the host.

## Four path-aware mechanisms

| I want to | Use | Matching |
|---|---|---|
| Send `/api` to a different backend | [`locations`](../reference/config/proxy-host.md) | Longest prefix |
| Let one network reach only `/health` | Access-list rule `paths` | Exact, allow-only |
| Block `POST /login` except from the LAN | [`guard` middleware](../reference/config/middleware.md) | Exact |
| Fix a path the client got wrong | [`rewrite` middleware](../reference/config/middleware.md) | Exact, prefix or regex, upstream-facing |

Path-deny belongs to the guard and path-allow to the access list, deliberately:
an exact match that misses fails **closed** for an allow rule and would fail
**open** for a deny rule. Validation refuses `action: deny` alongside `paths`.

## Four ways to not serve a host

| Mechanism | Domain claimed? | DNS records | Certificate | Response |
|---|---|---|---|---|
| `disabled: true` | Released | Withdrawn | n/a | `404`, no such host |
| `maintenance: true` (per host) | Kept | Kept | Kept | `503` with `Retry-After` |
| `settings.maintenance.enabled` | Kept | Kept | Kept | `503` fleet-wide; wins over a per-host `false` |
| [`ParkedHost`](../reference/config/parked-host.md) | Kept | n/a (no `dns` field) | Yes | Fixed status, default `404` |
| [`RedirectHost`](../reference/config/redirect-host.md) | Kept | n/a | Yes | `3xx` to the target |

Use `maintenance` for a downtime window you intend to end, and `disabled` to
take a name off the edge entirely. `disabled` withdrawing DNS is the difference
that matters most: a disabled host contributes nothing to the DNS desired set,
so records gpm created are deleted and records it adopted are released.

## Header mechanisms

| I want to | Use | Applies to | Overwrites? |
|---|---|---|---|
| Set a security header fleet-wide or per host | [`securityHeaders`](../reference/config/settings/security-headers.md) | Scope-selected: gpm-generated, proxied, or both | No, set-if-absent |
| Set HSTS | `tls.hsts` on the host | HTTPS responses only | Yes; `securityHeaders` refuses the name |
| Discourage indexing | `robotsNoIndex` on the host | All responses | Sugar for `X-Robots-Tag`; a headers middleware wins |
| Remove a header the backend leaked | [`stripResponseHeaders`](../reference/config/settings/security-headers.md) | The upstream's own response map | Removal only |
| Mutate headers on one path or middleware | `headers` middleware | Only where attached, inside the chain | Yes |

`securityHeaders` sets headers on responses gpm generates and/or proxies,
never clobbering the app's own. `stripResponseHeaders` removes headers the
upstream sent. A `headers` middleware is the escape hatch for anything path- or
middleware-scoped. `hsts` is separate because it is HTTPS-only.

Every page gpm serves itself (maintenance, parked, denied, upstream-down) is
the [`errorPages`](../reference/config/settings/error-pages.md) template for its
status code.

## Settings, host, template and profile precedence

| Shape | Rule |
|---|---|
| Map (`securityHeaders`, `errorPages.inline`) | The host value replaces the settings value **per key** |
| Whole object (`errorPages`) | Resolved **per status**: the host's template for that status wins, else its own `default`, else the settings-level one |
| List (`stripResponseHeaders`) | The host list is **added to** the settings list; a host cannot opt out |
| List (`trustedProxies`) | The host list **replaces** the settings list |
| Bool safety switch (`maintenance`) | Settings `true` wins over a host `false` |
| Discovery `template` vs `profiles[x]` | **Replaces**, never merges |
| Discovery `allowedDomainSuffixes` | A profile may only **narrow** the global list |

## Inline auth versus middleware objects

| Situation | Use |
|---|---|
| One host, its own gate | An inline `auth` / `rateLimit` block on the host or location |
| One policy shared across many hosts | A `Middleware` object each host references |
| Both | Legal. Every gate must pass; the inline block is evaluated first |

Both compile to the same handler at the same chain position, so behaviour,
metrics and error pages are identical. See
[Inline auth and rate limit](../reference/config/proxy-host.md).

## Where authentication can live

| Mechanism | Layer | Use it for |
|---|---|---|
| `tls.clientAuth` on the host | TLS handshake | Requiring a client certificate before any HTTP exists |
| Auth middleware `mode: client-cert` | Request | Per-request roles and a trusted-network exemption on top of the handshake |
| Auth middleware `mode: oidc` / `forward-auth` / `auth-request` | Request | Browser SSO, trusted identity headers, or an outpost subrequest |
| Auth middleware `mode: basic` | Request | Local username/bcrypt pairs |
| `AccessList.basicAuth` | Access-list tier | Deprecated. [Migrate it](../how-to/migrate-basic-auth.md) |
| `settings.adminAuth` | Admin panel | Who signs in to gpm itself, not the proxy listeners |
