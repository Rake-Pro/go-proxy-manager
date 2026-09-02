# Use remote IP feeds in an access list

Let a published prober list reach only the health endpoints of a host that is
otherwise LAN/VPN-only.

## Prerequisites

- An access list already attached to the host, with `defaultAction: deny`.
- The provider's published feed URL. It must be `https`, one IP or CIDR per
  line, `#` comments and blanks ignored.
- `settings.accessListSync` left enabled (it is by default) so the fetcher runs.
- The exact health paths and methods the prober needs, so the grant can be
  scoped to them.

## Steps

The case this exists for: a host is LAN/VPN-only, but an external uptime monitor
has to be able to probe it. Grant the provider's published prober addresses the
health paths, and nothing else:

```yaml
name: home-vpn
defaultAction: deny
rules:
  # Unchanged LAN/VPN rules: every path, every method.
  - {action: allow, cidr: 10.0.0.0/8}
  - {action: allow, cidr: 192.168.0.0/16}
  # The monitor: health endpoints only, read-only methods only.
  - action: allow
    source: uptimerobot
    paths: [/api/health, /v1/health, /-/healthy, /status.php]
    methods: [GET, HEAD]
sources:
  - name: uptimerobot
    url: https://uptimerobot.com/inc/files/ips/IPv4andIPv6.txt
    interval: 24h
```

A prober hitting `/api/health` gets through; the same prober hitting `/` or
`POST`ing to `/api/health` gets a `403`, and so does anyone else off-LAN. The
~200 IPv4/IPv6 entries are fetched and kept current automatically: you never
paste them into this file.

**Pair every `source` rule with `paths`.** A `source` rule without them is an
ordinary allow rule: it grants every address in a third party's feed full access
to the host, on every path and method.

## Verify

| Check | Expected |
|---|---|
| `GET /api/access-list-sources/status` | The source listed with a recent `fetchedAt` and a plausible `entryCount` |
| `refused` in that status | `0`. Anything above zero means a list is being served from a set that is no longer refreshed |
| `curl` a health path from an allowed address | `2xx` |
| `curl` `/` from the same address | `403` |
| `POST` to a health path from the same address | `403`: the rule defaults to `GET` and `HEAD` |
| `config/access-list-sources.yaml` | One entry per source, with a `sha256` over its network set |

## How the fetcher behaves

An [AccessList](../reference/config/access-list.md) may declare
`sources`: published, plain-text IP feeds (one IP or CIDR per line) that a rule
references by name instead of you pasting hundreds of CIDRs into the config. The
motivating case is letting a monitoring provider's prober addresses reach only
the health paths of a host that is otherwise LAN/VPN-only.

The fetcher runs in-process, on `settings.accessListSync.pollInterval` (default
`15m`), and downloads a source only once its own `interval` (default `24h`) has
elapsed. It is **leader-only**: the fetched sets are a config-repo commit, and a
follower that committed locally would break its fast-forward-only pull. A
follower still serves the sets, because it replicates the ledger file.

**Ledger file.** The fetched sets live in `config/access-list-sources.yaml`
(`list`, `source`, `url`, `fetchedAt`, `sha256`, sorted `entries`), committed like
everything else: diffable, auditable and reverted with the rest of the config.
**Do not hand-edit it:** every entry carries a `sha256` over its network set, and
a mismatch fails the load rather than serving a set somebody widened by hand. A
source whose entry is missing (never fetched, or refused before the first
success) resolves to the **empty set**, so its rules match nothing and the list
falls through to `defaultAction`.

**Commit behaviour.** Only a *changed* set writes the ledger. An unchanged feed
deliberately does **not** advance `fetchedAt`, because doing so would commit a
timestamp-only diff to the config repo every interval, forever; the process keeps
the last attempt in memory instead, so a re-fetch still waits out the interval. A
restart therefore re-fetches each source once, finds it unchanged, and commits
nothing.

**Refusals fail closed.** A fetch is rejected **whole** (keeping whatever was
fetched before) when the source answers anything but `200`, returns no valid
entries, returns more than `maxEntries` (default 10000), sends a body over 1 MiB,
or contains a single line that is neither a comment nor a valid IP/CIDR. A feed
that changed shape is a feed gpm no longer understands; keeping the subset it
still parses would be a silent, partial allow list.

A refused fetch does **not** start the source's interval clock, so the retry is
on the next poll tick (`pollInterval`, default 15m) rather than after the
source's own `interval` (up to a day). One transient blip therefore costs minutes
of staleness, not a day of it, and the previously fetched set is served
throughout.

**What a feed may contain.** Individual entries are bounded as well as counted. A
line is refused (and with it the whole fetch) when it is a default route
(`0.0.0.0/0`, `::/0`), a prefix broader than `/8` for IPv4 or `/32` for IPv6, or
any address in a range a public monitoring feed never legitimately carries
(loopback, link-local, RFC1918, ULA, CGNAT `100.64.0.0/10`, multicast,
`192.0.0.0/24`, `198.18.0.0/15`, `64:ff9b::/96`). Without this a single hijacked
or fat-fingered `0.0.0.0/0` would pass every other check (valid CIDR, HTTP 200,
under `maxEntries`) and a source rule with no `paths` would then allow the
entire internet.

**SSRF guard.** A source URL must be `https`, redirects are never followed, **no
proxy is honoured** (`HTTP_PROXY`/`HTTPS_PROXY` are deliberately ignored on this
client: through a proxy the address the guard inspects is the proxy, not the
destination), and the dialer refuses the destination **post-DNS, at connect time**
if it falls in any of the ranges above. Unlike DNS sync (whose whole point is a
LAN Pi-hole), nothing legitimate here is internal, so a source cannot be aimed at
`169.254.169.254` or an internal endpoint even via a rebinding resolver.

**Scope a source rule with `paths`.** A rule that names a `source` but no `paths`
grants every address in that feed full access to the host. Path scoping is what
bounds a third party's published list to the endpoints you chose to expose; and
because an exact path match cannot be relied on to *deny*, `paths` are allow-only
(`action: deny` alongside them is refused at validation: use a guard middleware
for deny-by-path).

```
# What each source last resolved to.
curl -s https://<admin>/api/access-list-sources/status -H 'Authorization: Bearer gpm_...' | jq
# {"enabled":true,"lastRun":"...","refused":0,"sources":[
#   {"list":"home-vpn","name":"uptimerobot","url":"https://uptimerobot.com/inc/files/ips/IPv4andIPv6.txt",
#    "fetchedAt":"...","entryCount":206,"lastAttempt":"..."}]}

# Fetch every due source now.
curl -s -X POST https://<admin>/api/access-list-sources/reconcile -H 'Authorization: Bearer gpm_...' | jq
```

A reconcile also fires automatically after any access-list write, restore or
whole-config revert, so a newly added source is fetched without waiting out the
poll. The manual endpoint does **not** queue: while a run is in flight it answers
`409 Conflict`. After a successful ledger write the data plane is reloaded, so
the new set is served on the very next request.

Scopes: `access-lists:read` for status, `access-lists:write` for reconcile.
`refused > 0` in the status is the number to alert on: it means at least one
list is being served from a set that is no longer being refreshed.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `refused > 0` in the status | The feed answered non-`200`, was empty, exceeded `maxEntries`, sent over 1 MiB, or held one unparseable line. A fetch is refused **whole** | Fix the feed; the previously fetched set keeps serving, and the retry is on the next poll tick rather than after the source's own interval |
| A `source` rule matches nothing | The source has never been fetched, so it resolves to the **empty set** | Trigger a reconcile and check `fetchedAt`; an unfetched feed can never widen access |
| The whole fetch is refused over one line | The feed carries a default route, a prefix broader than `/8` (v4) or `/32` (v6), or a non-public address | That bound is deliberate: one hijacked `0.0.0.0/0` would otherwise allow the internet. Take it up with the feed publisher |
| Config write refused: `url` must be https | `http` source URLs are refused | Use the `https` endpoint |
| Config load fails on the source ledger | `config/access-list-sources.yaml` was hand-edited; each entry carries a `sha256` over its set | Never hand-edit it: revert the file and let the fetcher rewrite it |
| The list is refused on a stream host | A stream host resolves no ledger and has no request path, so `sources` and `paths` rules are refused there | Use a plain CIDR list at L4 |
| Nothing is ever fetched | `settings.accessListSync.enabled: false`, or this instance is an HA follower | Re-enable it; only the leader fetches, and a follower serves the replicated ledger |
