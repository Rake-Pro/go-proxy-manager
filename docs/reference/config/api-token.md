# APIToken

A non-interactive credential for the REST API: a bearer secret with an explicit
scope list, used by scripts and CI instead of an admin session cookie.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="api-token-scopes"></span>  `scopes` | []string | yes | What this token may do (below). At least one. |
| <span id="api-token-expires-at"></span>  `expiresAt` | RFC3339 time | no | Token stops authenticating after this instant. Unset never expires. |
| <span id="api-token-token-hash"></span>  `tokenHash` | string | **server-owned** | Lowercase SHA-256 hex digest of the secret. Written by the server; a client-supplied value is discarded, and it is **never returned by any endpoint** (`json:"-"`) - a digest is offline-crackable. It exists only in the YAML at rest. |
| <span id="api-token-disabled"></span>  `disabled` | bool | no | Keep the token in config without it authenticating. |

**The secret is never stored.** It is generated server-side (`gpm_` + 32 random
bytes, base64url) and returned **exactly once**, as the `token` field in the
response to the `PUT` that created it. Only its digest is committed, in a plain
string field rather than a `Secret` - a digest is not a value to resolve from the
environment, and the store refuses literal `Secret` values outright.

```
PUT  /api/api-tokens/ci            # create; response carries "token" once
PUT  /api/api-tokens/ci            # ordinary edit; digest carried forward, no new secret
PUT  /api/api-tokens/ci?rotate=1   # rotate; new secret returned once, old one dies
```

Use it as a bearer credential:

```
curl -H 'Authorization: Bearer gpm_...' https://gpm.example.com/api/proxy-hosts
```

## Scopes

A scope is `<subject>:read`, `<subject>:write`, `*:read`, `*:write`, or `admin`.

- **write implies read** on the same subject; read never implies write.
- `*` matches any subject, but a concrete subject never satisfies a `*`
  requirement - a whole-config read (`/api/config`, `/api/history`, `/api/logs`,
  `/api/upstream-health`) genuinely needs `*:read`.
- `admin` satisfies everything, and is the **only** scope that reaches:
  - `/api/api-tokens` (a token that could mint tokens could widen itself),
  - **`PUT /api/settings`** (see below),
  - `GET /api/backup` - the archive is the raw on-disk YAML, so unlike the JSON
    reads it carries the api-tokens' stored digests,
  - `POST /api/restore`, `POST /api/revert`, `POST /api/sso/revoke`,
  - `/debug/pprof/*` when profiling is enabled - a heap dump and the process
    command line contain resolved backend credentials in cleartext, and every
    token principal is admin-*role* by construction, so the role gate alone is
    not a boundary there.

  `GET /metrics` is gated the same way but on its own, narrower scope
  (`metrics:read`) rather than `admin` - an exposition is not a credential dump.

**`settings:write` does not reach `PUT /api/settings`.** Writing settings is
admin-equivalent and takes the `admin` scope: a settings write can point
`dnsSync.pihole.url` or a webhook at an attacker-controlled URL while supplying
`${ENV:SOME_TOKEN}` as its credential, and the write itself triggers the
reconcile/dispatch that resolves that env var and sends it offsite - and it can
rewrite `adminAuth` outright. `settings:read` still grants `GET /api/settings`
(reading resolves nothing). `settings:write` remains a valid scope string for
forward compatibility but grants nothing beyond `settings:read` today; the UI
greys the box out rather than offering a grant that does nothing.

**Reverting an `APIToken` is refused** - scoped (`POST
/api/api-tokens/{name}/revert`) and whole-config (`POST /api/revert`, which
preserves the `api-tokens` directory across the restore). Restoring an older
token file would restore an older `tokenHash` and silently revive a secret the
operator rotated away, so rotation would stop meaning revocation. Create a
replacement token instead.

Valid subjects are the REST resource plurals - `proxy-hosts`, `redirect-hosts`,
`stream-hosts`, `parked-hosts`, `certificates`, `client-cas`, `dns-providers`,
`identity-providers`, `upstream-groups`, `access-lists`, `middlewares`,
`api-tokens` - plus five pseudo-resources for non-CRUD endpoint groups:
`settings`, `dns-sync`, `ingress-discovery`, `docker-discovery` and `metrics`. An unknown subject or verb is rejected at write time.

**`metrics:read`** is what a Prometheus scrape credential needs for
`GET /metrics` (mounted only with `GPM_METRICS=1`, see
[Prometheus metrics](../metrics.md)). It is its own subject rather
than `*:read` so a token that lives in a monitoring config forever buys exactly
one thing: the exposition names hosts and certificates but carries no field
values, so it is not a config read.
`GET /api/capabilities` and `GET /api/me` need no scope: any authenticated caller
may ask what the instance supports.

Scopes constrain **API tokens only**. An admin session keeps full access exactly
as before.

```yaml
name: ci-deploy
scopes:
  - proxy-hosts:write
  - certificates:read
expiresAt: 2027-01-01T00:00:00Z
```

**Auth mechanics.** A request carrying `Authorization: Bearer gpm_...` is
resolved as a token *before* the cookie path and never falls through to it: a
presented-but-invalid token is a `401`, not an invitation to try a session cookie
riding along on the same request. Any other bearer scheme is left alone and the
cookie path runs as usual. Token principals are **CSRF-exempt** - the
double-submit check defends against a browser attaching ambient credentials, and
a bearer token is never attached automatically. Successful and failed token
authentications are logged (token name on success; never any secret material).
Last-use is tracked **in memory only** and surfaced as `lastUsed` on
`GET /api/api-tokens`; the config store is git-backed, so persisting a timestamp
per request would be a commit flood. It resets on restart.
