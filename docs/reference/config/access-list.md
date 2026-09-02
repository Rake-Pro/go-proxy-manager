# AccessList

Ordered IP allow/deny rules and optional GeoIP country rules over the derived
client IP (see [Client IP and the three trust
tiers](../../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers)). For username/password gating use an
[auth middleware in `basic` mode](middleware.md): the `basicAuth`/`satisfyAny`
pair below is deprecated.

| Field | Type | Notes |
|-------|------|-------|
| <span id="access-list-rules"></span>  `rules` | []IPRule | Ordered allow/deny rules (below). |
| <span id="access-list-sources"></span>  `sources` | []AccessListSource | Remote IP feeds a rule may reference by name (below). |
| <span id="access-list-default-action"></span>  `defaultAction` | string | `allow` \| `deny` (default `deny`). |
| <span id="access-list-geo"></span>  `geo` | AccessListGeo | Country allow/deny over the same resolved client IP (requires `GPM_GEOIP_DB`). |

**IPRule**

| Field | Type | Notes |
|-------|------|-------|
| <span id="access-list-rules-action"></span>  `action` | string | `allow` \| `deny`. |
| <span id="access-list-rules-cidr"></span>  `cidr` | string | CIDR or bare IP. Exactly one of `cidr` or `source` must be set. |
| <span id="access-list-rules-source"></span>  `source` | string | Names an entry in this list's `sources`; the rule covers every network in that source's last fetched set. |
| <span id="access-list-rules-paths"></span>  `paths` | []string | Exact request paths the rule is limited to. Empty = every request (the historical behaviour). |
| <span id="access-list-rules-methods"></span>  `methods` | []string | Upper-case HTTP methods a path-scoped rule covers. Only valid with `paths`; empty means `GET` and `HEAD`. |

Rules are evaluated **top-down, first match wins**, then `geo`, then
`defaultAction`. A rule with `paths` matches only when the request's already-
normalised path is exactly one of them *and* the method is in the effective
method set, so `paths` narrows a rule, it never widens one. Paths must be
absolute, clean (no `.`/`..` segments, no trailing slash, no query string) and
unique within the rule; anything else is refused at write time, because it could
never match the cleaned path the router compares against.

> **`paths` are allow-only.** `action: deny` together with `paths` is refused at
> validation. The match is **exact, case-sensitive, and does no trailing-slash
> folding**: `/admin` covers neither `/admin/` nor `/ADMIN`. That is fine for an
> allow rule: a spelling it misses falls through to `defaultAction`, so it fails
> closed, and unsafe for a deny, which would fail *open* on exactly those
> spellings. Use a [guard middleware](middleware.md) for
> deny-by-path.

**AccessListGeo** (`geo`) adds country rules over the same derived client IP the
`rules` compare. It needs an operator-supplied GeoIP database
(`GPM_GEOIP_DB`); none ships with gpm, because GeoLite2's licence forbids
redistribution.

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="access-list-geo-country-allow"></span>  `countryAllow` | []string | none | no | Whitelist of ISO-3166-1 alpha-2 codes; **only** these pass. When set it takes priority and `countryDeny` is ignored entirely. |
| <span id="access-list-geo-country-deny"></span>  `countryDeny` | []string | none | no | Codes to reject. Every other known country falls through to `defaultAction`. Consulted only when `countryAllow` is empty. |
| <span id="access-list-geo-on-unknown"></span>  `onUnknown` | string | mode-dependent (below) | no | `allow` \| `deny`. Governs an IP with no country in the database (private, loopback, link-local, or simply absent). |

**The `onUnknown` default depends on the mode, and both directions fail safe:**

| Mode | `onUnknown` unset means | Why |
|---|---|---|
| Whitelist (`countryAllow` set) | `deny` | An unresolvable address must not slip past a "these countries only" gate. |
| Deny-list (`countryDeny` set, `countryAllow` empty) | `allow` | A deny-list only ever narrows a default-allow posture, so the LAN is never geo-blocked by accident. |

An explicit `onUnknown` always wins over both defaults. Codes must be
upper-case two-letter codes; anything else is refused at write time.

**With geo rules configured and no database loaded, every IP is unknown**, so
`onUnknown` alone decides: which means a whitelist list denies outright. That
is the same fail-closed rule the L4 path applies to a stream host.

```yaml
name: no-sanctioned
defaultAction: allow
rules:
  - {action: allow, cidr: 10.0.0.0/8}
geo:
  countryDeny: [CN, RU, KP]
  onUnknown: allow          # explicit; this is also the deny-list default
```

**AccessListSource**

| Field | Type | Notes |
|-------|------|-------|
| <span id="access-list-sources-name"></span>  `name` | string | Referenced by a rule's `source`. Unique within the list. |
| <span id="access-list-sources-url"></span>  `url` | string | Absolute **https** URL of a plain-text feed. `http` is refused. |
| <span id="access-list-sources-interval"></span>  `interval` | string | Go duration between re-fetches. Empty = `24h`; below `1h` is refused. |
| <span id="access-list-sources-max-entries"></span>  `maxEntries` | int | Cap on the feed's size. `0` = `10000`. |

The feed format is fixed: one IP or CIDR per line, `#` comment lines and blank
lines ignored, a bare IP read as a `/32` or `/128`. Fetching is handled by the
[access-list source sync](settings/access-list-sync.md)
subsystem; the fetched sets live in `config/access-list-sources.yaml`, never
inline here, so a routine re-fetch never rewrites a file you own.

A source that has **never been fetched** resolves to the empty set, so rules
referencing it match nothing and the list falls through to `defaultAction`. An
unfetched or refused feed can never widen access. Entries are bounded too: a
default route (`0.0.0.0/0`, `::/0`), a prefix broader than `/8` (IPv4) or `/32`
(IPv6), or any address in a non-public range (loopback, link-local, RFC1918,
ULA, CGNAT `100.64.0.0/10`, multicast, `192.0.0.0/24`, `198.18.0.0/15`,
`64:ff9b::/96`) refuses the whole fetch.

> **Pair every `source` rule with `paths`.** A source rule *without* `paths` is
> an ordinary allow rule: it grants every address in the feed full access to the
> host, on every path and method. The path scoping is what bounds a third party's
> published list to the endpoints you chose to expose.

A list can also be attached to a **StreamHost**, where only the `rules` and `geo`
dimensions are evaluated. A list carrying the deprecated `basicAuth` users is
rejected for a stream host at validation, and so is one with any `sources` or any
rule carrying `paths` or `source`: a raw stream has no request path and resolves
no ledger, so evaluating half the gate would be worse than refusing it. See
[StreamHost](stream-host.md).

```yaml
name: internal-only
rules:
  - {action: allow, cidr: 10.0.0.0/8}
  - {action: allow, cidr: 192.168.0.0/16}
defaultAction: deny
```

## Deprecated fields

Still parsed and still enforced, so existing YAML keeps working unchanged. They
are gone from the UI and marked deprecated in the OpenAPI schema, and gpm logs a
`WARN` at load naming every list that still carries them. Both are **removed in
v2**.

| Field | Status | Migration |
|---|---|---|
| <span id="access-list-basic-auth"></span>  `basicAuth` | Deprecated, still enforced | Move the users to an [auth middleware](middleware.md) with `mode: basic`. A login mechanism in the IP/geo tier is what forced both `satisfyAny` and the "no `basicAuth` list on a StreamHost" refusal. |
| <span id="access-list-satisfy-any"></span>  `satisfyAny` | Deprecated, still enforced | Becomes `allowFrom` on that middleware: the networks that previously satisfied the list *instead of* a password become the networks exempt from it. |
