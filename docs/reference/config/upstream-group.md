# UpstreamGroup

An ordered set of interchangeable backends a ProxyHost forwards to instead of a
single `upstream`, with health-checked failover. The first upstream is preferred;
the rest are backups tried in order when an earlier one is unhealthy or
unreachable. Many hosts can reference one group; its backends are probed once
per group, not once per host.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="upstream-group-upstreams"></span>  `upstreams` | []GroupUpstream | yes | Ordered backend list. Same shape as a host `upstream` plus optional `weight`; duplicates rejected. |
| <span id="upstream-group-policy"></span>  `policy` | string | no | `failover` (default) \| `round-robin` \| `least-connections` \| `ip-hash`. |
| <span id="upstream-group-stickiness"></span>  `stickiness` | Stickiness | no | Cookie-based session affinity with a TTL (below). |
| <span id="upstream-group-health-check"></span>  `healthCheck` | HealthCheck | no | Active probe tuning (defaults below). |

**GroupUpstream** embeds the whole [Upstream](proxy-host.md#proxy-host-upstream)
shape, so `path` and `hostHeader` are **per member** as well: `scheme`/`host`/`port`/`path`/`hostHeader` plus `weight`
(1-256, default 1): the relative share for the weighted policies; ignored by
`failover` and `ip-hash`.

**Policies** (unhealthy upstreams always drop to the end of the try-order,
whatever the policy):

- `failover`: strict list order: the first healthy upstream takes all traffic,
  the rest are backups. The right default for identical entry points.
- `round-robin`: smooth weighted round-robin (nginx's algorithm) across the
  healthy set.
- `least-connections`: the healthy upstream with the fewest in-flight requests
  relative to its weight.
- `ip-hash`: rendezvous hashing on the client IP: a client sticks to one
  upstream while it stays healthy, and when an upstream dies only its own
  clients move (no global reshuffle).

**Stickiness** (`stickiness`): `ttl` (required, a Go duration such as `30m` /
`12h`, with a whole-day `d` suffix also accepted, e.g. `3d`) and `cookie`
(optional name, default `gpm-sticky-<group>`). On a client's first request the
data plane assigns an upstream by the policy and sets an HMAC-signed cookie
(`HttpOnly`, `Path=/`, `SameSite=Lax`, `Secure` when the client came over
HTTPS) naming it; later requests honor the pin while the cookie is valid and
that upstream is healthy. Semantics worth knowing:

- **TTL is enforced server-side**: the expiry rides inside the signed value, so
  a client replaying a cookie past its `Max-Age` is still re-assigned. The
  window is fixed from assignment (not sliding), matching "stick for X".
- The cookie is signed with the same key as data-plane SSO sessions
  (`GPM_SSO_SIGNING_KEY` / the persisted `sso_signing.key`), so a client cannot
  forge a pin to steer itself onto a chosen backend. A restart with an
  ephemeral key re-assigns everyone once.
- Composes with any `policy`: the policy picks the initial upstream (and the
  re-pick when the pinned one dies or the TTL lapses); the cookie holds it.
- Only cookie-honoring clients get affinity; raw API clients silently fall back
  to the policy; use `ip-hash` when those need stickiness too.
- An honored pin adds no `Set-Cookie` noise; the cookie is (re)issued only when
  the assignment is new or moved.

**HealthCheck**: `path` (optional, an HTTP GET probe of this path; blank keeps a
plain TCP-connect probe), `intervalSeconds` (default 5), `timeoutSeconds`
(default 3), `rise` (consecutive successes to return an upstream to service,
default 2), `fall` (consecutive failures to remove it, default 2).

```yaml
name: edge-nodes
policy: round-robin
stickiness: {ttl: 12h}          # optional: pin each client to its upstream
upstreams:
  - {scheme: http, host: 192.0.2.11, port: 80}
  - {scheme: http, host: 192.0.2.12, port: 80}
  - {scheme: http, host: 192.0.2.13, port: 80, weight: 2}   # weight used by weighted policies only
healthCheck: {path: /ping, intervalSeconds: 5, fall: 2, rise: 2}
```

Semantics worth knowing:

- **Failover retries only connect-phase failures** (dial refused / no route /
  dial timeout / TLS handshake). A request that may already have been sent
  (reset mid-response, timeout awaiting headers, or any HTTP response including a
  5xx) is never replayed against another upstream, so non-idempotent requests
  cannot double-apply. An application error also fails through every entry point
  equally, so retrying it would buy nothing.
- **Probes measure the entry point, not the app**: an HTTP probe counts any
  response below 500 as alive. Point `path` at something that reflects the
  upstream proxy/node itself (e.g. a ping endpoint), not at a shared application.
- **Passive detection feeds the same counters**: live-traffic connect failures
  count toward `fall`, so a dead upstream is skipped quickly even between probes.
- **Fail-open**: with every upstream marked down, requests are still attempted in
  preference order rather than rejected outright.
- **Request bodies up to 1 MiB are buffered** so a failover retry can replay
  them; a larger body streams and gets a single attempt at the preferred
  (healthiest) upstream.
- A `disabled` group cannot be referenced by an enabled host (validation error).
- Live health is exposed at `GET /api/upstream-health`
  (`{"<group>": [{"upstream": "http://192.0.2.11:80", "healthy": true, "weight": 1, "active": 0}, ...]}`,
  where `active` is the in-flight request count) and shown in the UI editor.

---

## Watching live health (operations)

Hosts backed by an upstream group
fail over automatically, but during rollouts and node maintenance you can watch
the live state:

```
curl -s -b "<admin session cookie>" https://<admin>/api/upstream-health | jq
# {"edge-nodes":[{"upstream":"http://192.0.2.11:80","healthy":true,"weight":1,"active":3}, ...]}
```

`healthy` flips on the probe/traffic rise-fall thresholds; `active` is the
in-flight request count per upstream. Draining or rebooting a backend node
should show its upstream go unhealthy within roughly one probe interval plus
the fall threshold, with traffic continuing through the remaining upstreams.
Deploy ordering note: roll a new gpm binary **before** adding the first
upstream-group config: an older binary treats a host whose only backend is an
(unknown to it) `upstreamGroupRef` as having no upstream and fails config
validation.
