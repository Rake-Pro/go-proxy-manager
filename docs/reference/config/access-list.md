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
| <span id="access-list-rules-cidr"></span>  `cidr` | string | CIDR or bare IP; a bare IPv4-mapped address (`::ffff:a.b.c.d`) is the IPv4 address `a.b.c.d`. Exactly one of `cidr` or `source` must be set. |
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
| <span id="access-list-sources-max-coverage"></span>  `maxCoverage` | object | Optional. Total address space the feed may cover, per family, as prefix-length equivalents: `ipv4` (1-32; `0` = `8`, one /8 = 16,777,216 addresses) and `ipv6` (1-64; `0` = `24`, 256 /32s). A smaller number allows more. Absent = no total budget; the per-entry rules below still apply. |
| <span id="access-list-sources-max-age"></span>  `maxAge` | string | Optional. Go duration; once the last **successful** fetch recorded in the ledger (or the time `maxAge` was added, if later) is older, the stored set is emptied and the source matches nothing until a fetch succeeds again ([below](#access-list-sources-expiry)). Must not be shorter than `interval` (or the `24h` default). Absent = a refused fetch keeps the previous set however old it is. |

The feed format is fixed: one IP or CIDR per line, `#` comment lines and blank
lines ignored, a bare IP read as a `/32` or `/128`. Fetching is handled by the
[access-list source sync](settings/access-list-sync.md)
subsystem; the fetched sets live in `config/access-list-sources.yaml`, never
inline here, so a routine re-fetch never rewrites a file you own.

<span id="access-list-sources-unfetched"></span>
A source that has **never been fetched** successfully (or whose ledger cannot be
loaded) resolves to the empty set, so rules referencing it match nothing and
evaluation moves on to the next rule and then `defaultAction`.

| Rule | Effect of an empty set | Surfaced as |
|---|---|---|
| `allow` | Admits nobody through that rule; an unfetched or refused feed can never widen access. | One `WARN` per reload (`access-list source has no fetched entries yet`). |
| `deny` | Denies nobody: an address it should block is decided by the later rules and `defaultAction`, which may allow it. Kept on purpose, so an outage of a deny feed does not lock everyone out. | The same `WARN` per reload, plus a config warning in `GET /api/health` `configWarnings` (the UI's Needs attention list) for each such deny rule. The warning uses the data plane's own resolution, so it also covers a set that [expired](#access-list-sources-expiry) under `maxAge`, one over its `maxCoverage`, and one whose every stored entry the entry guard refuses. |

Entries are bounded too: a
default route (`0.0.0.0/0`, `::/0`), a prefix broader than `/8` (IPv4) or `/32`
(IPv6), or any network that overlaps a non-public range (loopback, link-local,
RFC1918, ULA, CGNAT `100.64.0.0/10`, multicast, `192.0.0.0/24`, `198.18.0.0/15`,
`64:ff9b::/96`) refuses the whole fetch. An IPv4-mapped entry
(`::ffff:192.0.2.0/120`) is judged and stored as the IPv4 network it matches
(`192.0.2.0/24`), so the IPv4 limits apply to it.

The feed as a whole can be bounded too, and is not by default: with
`maxCoverage` set, its entries, summed per family, may cover at most that
budget, and a fetch over it is refused whole. Without `maxCoverage` there is no
total budget, only the per-entry rules above. A fetch of a source without
`maxCoverage` that covers more than one /8 of IPv4 or one /24 of IPv6 in total
logs a `WARN` that the feed is broad. Monitoring feeds are a few hundred single
addresses and published CDN edge lists a few million IPv4 addresses, so
`maxCoverage: {}` (one /8 and one /24) fits them; a feed that publishes a
provider's whole address space needs a larger budget:

```yaml
sources:
  - name: monitors
    url: https://example.com/monitors.txt
    maxCoverage: {}                    # one /8 of IPv4 and one /24 of IPv6
  - name: cloud
    url: https://example.com/ranges.txt
    maxCoverage: {ipv4: 4, ipv6: 20}   # up to sixteen /8s and sixteen /24s
```

Both bounds also apply to the stored set: when the data plane compiles the
ledger it drops (and logs) any stored entry the entry rules refuse, and a stored
set over its `maxCoverage` matches nothing, so a set fetched before a bound was
set is not served while later fetches are refused. A set that matches nothing
also stops a `deny` rule that references it from denying, so set `maxCoverage`
on a deny feed only with room for what it publishes.

<span id="access-list-sources-expiry"></span>

**Source expiry (`maxAge`)**

A refused fetch keeps the previous set, so without `maxAge` a feed that stays
down is served from its last fetch indefinitely. `maxAge` bounds that: it is off
by default, and setting it is a choice between serving an old list and serving
none.

| Aspect | Behaviour |
|--------|-----------|
| When a set expires | At the first fetcher poll (`accessListSync.pollInterval`, default `15m`) after the age passes `maxAge` whose fetch is refused; a refused fetch is retried on every poll. The age is measured from the last successful fetch recorded in the ledger (`fetchedAt`), or from when `maxAge` was first applied to the source (`maxAgeSince`) if that is later; a success that could not be committed before a restart is not counted. |
| What changes | The ledger entry loses its entries and gains `expiredAt`, in one commit, and the data plane reloads. `fetchedAt` keeps the last successful fetch. One `WARN` (`access-list source expired`) is logged. |
| `allow` rule on an expired source | Admits nobody through that rule; such clients fall through to later rules and `defaultAction`. |
| `deny` rule on an expired source | Matches nothing, so it denies nobody; addresses it used to deny fall through to later rules and `defaultAction`. Listed in `GET /api/health` `configWarnings` until a fetch succeeds. |
| Recovery | The next successful fetch restores the set and clears `expiredAt`, with no operator action. Until then the source is retried on every poll. |
| Restart | Everything the decision uses is in the committed ledger, so a restart or failover neither brings an expired set back nor delays an expiry. |
| Status | `GET /api/access-list-sources/status` reports `expired: true` with `entryCount: 0`; a never-fetched source has no `fetchedAt` and is not `expired`. |
| Commits | For a source with `maxAge`, every successful fetch advances `fetchedAt`: an unchanged one commits once per `interval`, with no data-plane reload on the leader (an HA follower reloads on every commit it pulls). Adding or removing `maxAge` writes or removes `maxAgeSince` (one commit, no reload). A success whose commit failed is committed on a later poll, only if the ledger still holds the same set. Sources without `maxAge` keep the no-commit-on-unchanged behaviour and never get `maxAgeSince`. |

- **Leave room for retries.** A successful fetch never expires a set, but with `maxAge` equal to `interval` the first refused fetch does; `maxAge: 72h` at the default `24h` interval tolerates a feed outage of up to three days.
- **Fetcher only.** Expiry is applied by the fetcher, which runs on the leader only; with `accessListSync.enabled: false` nothing is fetched or expired, and the stored set is served as it is.

```yaml
sources:
  - name: monitors
    url: https://example.com/monitors.txt
    interval: 24h
    maxAge: 72h      # serve nothing once three days pass without a successful fetch
```

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

<span id="access-list-composing"></span>

## Composing access lists

A host or location that names several lists combines them with
[`accessListsMode`](proxy-host.md#proxy-host-access-lists-mode). Each list is
evaluated on its own, exactly as described above; the mode decides how the verdicts combine.

| `accessListsMode` | The request passes when | Use it for |
|---|---|---|
| `all` (default, also when unset) | every list allows it | layering restrictions, for example a LAN list plus a country list |
| `any` | at least one list allows it | widening a base list with an independently maintained one, for example a prober list scoped to health paths |

Rules for `any`:

- **`any` composes allow-lists only.** A member grants only what its own `action: allow` rules or `geo.countryAllow` name, so a list with `defaultAction: allow` or a `geo.countryDeny` deny-list is refused at validation: under `any` it would admit everyone it does not block, including every address the GeoIP database cannot place.
- **A list allows only on its own full verdict.** Its rules, `geo` and `defaultAction` all count, and so does every fail-closed state: a list with `geo` rules and no database loaded does not allow.
- **An unfetched source grants nothing.** A `source` rule whose feed has not been fetched yet matches nothing, so its list can allow only through its other rules.
- **A deny rule does not veto.** Under `any`, an explicit `action: deny` in one list stops that list from allowing, but another list may still allow the same request.
- **A list with nothing to match on never allows.** A list with no rules and no `geo` has nothing to grant under `any`. (Under `all` the same list passes every request, unless `defaultAction: deny` makes it deny-all.)
- **A disabled list is skipped.** Setting `disabled: true` on a member revokes what it grants; a group whose members are all disabled denies everyone. Under `all`, a disabled list is still enforced, as before.
- **Path scoping keeps its meaning.** A list whose allow rules carry `paths` widens only those paths.
- **Refused at validation:** `any` with no lists, any value other than `all` or `any`, and a member with `defaultAction: allow`, with `geo.countryDeny`, or with the deprecated `basicAuth` users.
- **Host and location are separate groups.** The host's lists are combined under the host's mode, a location's own lists under the location's mode, and both groups must pass.

```yaml
# access-lists/monitor.yaml: an uptime provider, health endpoints only
name: monitor
defaultAction: deny
sources:
  - {name: uptime, url: "https://uptime.example.com/ips.txt"}
rules:
  - {action: allow, source: uptime, paths: [/healthz, /api/health]}
---
# proxy-hosts/app.yaml: the LAN everywhere, the prober on the health paths
name: app
domains: [app.example.com]
upstream: {scheme: http, host: 10.0.0.40, port: 8080}
accessLists: [lan, monitor]
accessListsMode: any
```

With `all` (the default) the same pair would refuse everyone: a LAN client
fails `monitor` on every path, and the prober fails `lan`. Under `all`, the
fallback is to copy the prober rules into the base list instead of attaching a
second list.

A `403` from an `any` group is counted in
[`gpm_denials_total`](../metrics.md) and in the analytics denials as
`reason="geo"` when a member has `geo` rules and no GeoIP database is loaded,
otherwise as `reason="access-list-any"`. The host, mode and list names are
logged at `debug` level.

## Deprecated fields

Still parsed and still enforced, so existing YAML keeps working unchanged. They
are gone from the UI and marked deprecated in the OpenAPI schema, and gpm logs a
`WARN` at load naming every list that still carries them. Both are **removed in
v2**.

| Field | Status | Migration |
|---|---|---|
| <span id="access-list-basic-auth"></span>  `basicAuth` | Deprecated, still enforced | Move the users to an [auth middleware](middleware.md) with `mode: basic`. A login mechanism in the IP/geo tier is what forced both `satisfyAny` and the "no `basicAuth` list on a StreamHost" refusal. API reads return each `passwordHash` as `***`, and a write that sends `***` back keeps the stored hash, as for every basic-auth user ([API reference](../api.md#basic-auth-password-hashes)). |
| <span id="access-list-satisfy-any"></span>  `satisfyAny` | Deprecated, still enforced | Becomes `allowFrom` on that middleware: the networks that previously satisfied the list *instead of* a password become the networks exempt from it. |
