# REST API reference

The admin API: how to authenticate, what a token scope grants, and
where the machine-readable specification lives.

## Authenticating

| Credential | Header or cookie | CSRF | Governed by |
|---|---|---|---|
| Admin session | `gpm_session` cookie, set by the login flow | `X-CSRF-Token` from `GET /api/me` on every mutating request | The session's role (`admin` or `user`) |
| API token | `Authorization: Bearer gpm_...` | Exempt: a bearer header is never attached automatically | The token's scopes, intersected with the admin role |

A bearer token is resolved **before** the cookie path and never falls through to
it: an invalid token is a `401`, not an invitation to try a session cookie on the
same request. Both are also behind a same-origin guard.

## Token scopes

A scope is `<subject>:read`, `<subject>:write`, `*:read`, `*:write`, or `admin`.

| Scope | Grants |
|---|---|
| `<plural>:read` | Every `GET` on that resource, e.g. `proxy-hosts:read` |
| `<plural>:write` | Reads and writes on that resource; write implies read |
| `*:read` | Every read, including whole-config reads (`/api/config`, `/api/history`, `/api/logs`, `/api/upstream-health`, `/api/analytics`) |
| `*:write` | Every read and write, except the `admin`-only routes below |
| `metrics:read` | `GET /metrics` only, and nothing else |
| `admin` | Everything, and the **only** scope that reaches `/api/api-tokens`, `PUT /api/settings`, every write on `/api/dns-providers` and `/api/identity-providers`, a bouncer middleware write that creates, or changes the `url` or `apiKey` of, a bouncer whose `apiKey` is a placeholder, a certificate write that sets or changes `acme.directoryURL`, `acme.eab` or `acme.caBundle`, `POST /api/middlewares/{name}/revert` (a revert restores a stored bouncer without that check), `POST /api/certificates/{name}/revert` (a revert restores a stored certificate without the admin check on `acme.directoryURL`, `acme.eab` and `acme.caBundle`), `GET /api/backup`, `POST /api/restore`, `POST /api/revert`, `POST /api/sso/revoke`, `POST /api/sessions/revoke`, `PUT` / `DELETE /api/logs` and `/debug/pprof/*` |

Why the two provider kinds are admin-only: a DNS provider pairs a
`${ENV:...}` / `${FILE:...}` credential with an API endpoint, so whoever can
write one can point a resolved secret at a host of their choosing; an identity
provider named by `adminAuth.providers` decides who gets an admin session. A
bouncer with a placeholder `apiKey` is the same shape, so only its secret
destination is admin-gated and every other middleware write stays on
`middlewares:write`. The `dns-providers:read` and `identity-providers:read`
scopes are unchanged.

Valid subjects are the REST resource plurals plus the pseudo-resources
`settings`, `dns-sync`, `ingress-discovery`, `docker-discovery` and `metrics`.
The full rules, including why `settings:write` does not reach
`PUT /api/settings`, are in [APIToken](config/api-token.md).

## What a non-admin caller does not see

Four read endpoints redact operational detail for a caller without the `admin`
scope. A viewer-role session and a resource-scoped token both hit this.

| Endpoint | Redacted for a non-admin caller |
|---|---|
| `GET /runtime` | `paths` and `secretFileRoots` are omitted entirely; the rest of the payload is unchanged. |
| `GET /webhooks/status` | Each target URL is reduced to `scheme://host/(redacted)`. |
| `GET /notifications/status` | The same URL redaction. |
| `GET /settings` | `webhooks[].url` and `notifications.targets[].url` get the same redaction: for Discord, Slack and ntfy the URL path is the credential. |

**`GET /health` is classified for every caller, admin included.**
`acme.lastError` is always a classification plus the certificate name, e.g.
`certificate "app": dns-01 challenge or DNS provider failure`, never the
provider's raw message, whoever is asking. The full message stays on that
certificate's own status, behind `certificates:read`.

## Certificate status fields

`GET /api/certificates` and `GET /api/certificates/{name}` add read-only status fields to each object; the full list is in [Certificate: status fields](config/certificate.md#status-fields-get-only).

| Field | Meaning |
|---|---|
| `renewAt` | When the ACME manager next renews an issued `acme` certificate (RFC 3339). A time in the past means the renewal is due and has not succeeded yet. Omitted for `custom`, pending and disabled certificates, on an HA follower, and before the manager's first pass. |
| `renewReason` | `lifetime`, `ari` or `changed` (domains or `acme.profile` changed, so the certificate is due now and `renewAt` is the time of the pass): which rule set `renewAt`. Omitted with it. |
| `state: expiring` | At or inside `min(30 days, lifetime / 3)` of `notAfter`, lifetime being `notAfter - notBefore`. This holds for `acme` certificates only; a `custom` certificate keeps the fixed 30 days, because nothing renews it. It was a fixed 30 days for every certificate before, so a short-lived `acme` certificate is no longer `expiring` from the day it is issued. The `certificates.expiring` count of `GET /api/health` follows the same split. |

`acme.caBundle` is returned as written: it holds public trust anchors, not a secret.

## Basic-auth password hashes

No API read returns a stored basic-auth `passwordHash`, whoever is asking: each
one reads as `***`, like a literal secret. A write that sends `***` back keeps
the stored hash, so a client can GET an object, edit it and PUT it back.

| Topic | Rule |
|---|---|
| Where hashes live | `auth.basic.users[]` on a `type: auth` middleware, on a proxy host's inline `auth` and on each location's, the same inline blocks in discovery templates and profiles in settings, and the deprecated access-list `basicAuth`. |
| Reads that return `***` | Every JSON reply: object and list reads, `GET /config`, `GET /settings`, the reply to a write, discovery and migration plans. Only the `passwordHash` of a basic-auth user is replaced; a label, header or other key that happens to be named `passwordHash` is returned as stored. |
| Previews and diffs | The `yaml` of a dry run and the `patch` of `GET /history/diff` replace the value of every visible `passwordHash` key, plain or quoted, in block, flow or JSON style, through anchors, tags, block scalars and quoted values continued on later lines. In a proxy host, middleware, access list or settings file, a diff hunk that starts inside a value blanks, up to its first key, each line that could continue a hash (only hash characters and quotes, at most a hash long); other lines, such as an error page or a PEM, are shown as they are. Any bcrypt-shaped string anywhere is replaced too, with `$` written plainly or as `\x24`, `\u0024` or `\U00000024`. A non-hash key named `passwordHash` may therefore show `***` here too. |
| What previews do not cover | The YAML gpm writes is always covered. A hand-written file that hides the key itself (an escaped key name, a complex `? passwordHash` key, or an alias to an anchor defined under another key) with a value that is not bcrypt-shaped can show that value in a diff. The full files are available to an admin through `GET /backup` in any case. |
| `_parse` | Echoes the document it was sent verbatim, hashes included, because the import wizard creates the object from that reply. |

What a `***` sent back resolves to, checked per user list in the write:

| The user list in the write | Resolves to | Otherwise |
|---|---|---|
| Exists in the stored object at the same place: the same field; a location with the same `path` (locations sharing a path are told apart by order, only while the write keeps as many of them as are stored); a profile with the same name | The stored hash of the user with the same `username` in that same list | `400`: a placeholder never takes a hash from another list into an existing one |
| The single rename at its level: exactly one written and one stored entry at that level are left unmatched, counting every location of a shared `path` whose number of locations differs between the write and the stored object (one profile renamed, one location given a new `path`) | The hash of the same `username` in that stored list, else the one hash that `username` has anywhere in the stored object | `400` when the username has no stored hash, or several different ones (the message names the lists) |
| No counterpart: a new location, profile or auth block; several lists renamed or replaced at one level; a different number of locations on a shared `path` | The one hash that `username` has anywhere in the stored object | `400` as above |

A list with no counterpart, and the single rename at its level when its paired list does not hold the username, may therefore take the one hash that `username` has anywhere in the stored object, in any other list. A username listed more than once in one list (only the deprecated access-list `basicAuth` allows it) is matched occurrence by occurrence when the write lists it as often as the stored list does; otherwise the write is refused.

| Topic | Rule |
|---|---|
| Changing a password | Send `password` (plaintext, hashed server-side) or a new bcrypt `passwordHash`. `password` wins over `***` on the same user. |
| Renaming a user | Needs a password: the stored hash belongs to the old name. |
| Not affected | The backup archive (`GET /backup`) and the config files in git keep the real hashes: a restore must reproduce working credentials. |

## Previewing and importing objects

Two calls on every config kind let a client show the committed YAML before a
save, or fill a form from pasted YAML. Neither persists anything: no file, no
commit, no reload, no webhook, no `X-Config-Commit` header.

| Call | Scope | Request | Reply |
|---|---|---|---|
| `PUT /api/{plural}/{name}?dryRun=1` | Same as the real `PUT` | The JSON body the real `PUT` takes | `200` `{"kind","name","path","yaml","exists"}`, or the same `400` the real save would give |
| `PUT /api/{plural}/{name}` with `If-None-Match: *` | Same as the real `PUT` | Same | Create-only: `409` when the name is already stored, nothing written; otherwise the normal reply |
| `POST /api/{plural}/_parse` | `<plural>:write` (`admin` for `api-tokens`) | One YAML document, `Content-Type: application/yaml`, `application/x-yaml` or `text/yaml` | `200` `{"name","object","valid","error"}` |

- **Dry run runs the real checks.** Object validation, references across the
  whole config, the GeoIP gate and the literal-secret refusal all run, through
  the same store code as the save.
- **`dryRun` values.** `1` or `true` previews; `0`, `false` or absent saves;
  anything else is a `400`, so a typo never commits.
- **`path` and `yaml`.** `path` is relative to the config dir (e.g.
  `proxy-hosts/app.yaml`); `yaml` is the exact file content, with `createdAt`
  and `updatedAt` stamped as the save would stamp them and every `passwordHash`
  shown as `***` ([above](#basic-auth-password-hashes)).
- **API tokens.** A dry run never mints a secret, so the previewed APIToken
  YAML has no `tokenHash`.
- **`_parse` name.** Taken from the document's top-level `name` key, else the
  `name` query parameter; with neither it is a `400`.
- **`_parse` validation.** The kind's own decode rules apply (inline basic-auth
  passwords come back hashed). A document that decodes but fails the object's
  own validation is still a `200` with `valid: false` and the message in
  `error`; references to other objects are checked by the save or a dry run.
- **`_parse` refusals.** Wrong `Content-Type` is `415`. Invalid YAML, a
  document that is not a mapping, more than one document, or a body over 1 MiB
  is `400`.
- **`_parse` aliases.** Anchors, aliases and `<<` merge keys are accepted. A
  document whose aliases add more than 100,000 nodes once expanded is a `400`
  (`excessive aliasing`), checked before anything is decoded.
- **HA followers.** Both calls work on a follower, because neither writes:
  `PUT /api/{plural}/{name}?dryRun=1` (or `dryRun=true`) and
  `POST /api/{plural}/_parse` answer as on the leader. A real object `PUT` (no `dryRun`, `dryRun=0` or `dryRun=false`) answers `503`,
  and so does a `PUT` with an invalid `dryRun` value (the `400` is a leader
  answer). See [High availability](../operations/high-availability.md#1-roles).
- **Only the object `PUT` has a dry run.** Any other write (a `POST`, `PUT`
  or `DELETE`, for example `PUT /api/settings` or `DELETE /api/{plural}/{name}`)
  that carries `dryRun=1`, `dryRun=true` or an unrecognised value is refused and
  writes nothing. `dryRun=0` or `dryRun=false` is a normal write, and
  `POST .../_parse` writes nothing and ignores the parameter.
  - **Leader:** `400` with `{"error": "dryRun is not supported for <METHOD> <route pattern>: nothing was written; send the request without dryRun to make the change"}`,
    where the route pattern is the routed path without the `/api` prefix (for
    example `PUT /settings`).
  - **HA follower:** the usual `503` read-only answer, because that guard runs
    first. `POST /api/sessions/revoke?dryRun=1`, which a follower otherwise
    allows, gets the `400`.
  - **Object `PUT`, unrecognised value:** unchanged, `400` (`invalid dryRun
    value`) on the leader and `503` on a follower.

```
curl -s -X PUT 'https://<admin>/api/proxy-hosts/app?dryRun=1' \
  -H 'Authorization: Bearer gpm_...' -H 'Content-Type: application/json' \
  -d '{"domains":["app.example.com"],"upstream":{"scheme":"http","host":"10.0.0.5","port":8080}}' | jq -r .yaml

curl -s -X POST https://<admin>/api/proxy-hosts/_parse \
  -H 'Authorization: Bearer gpm_...' -H 'Content-Type: application/yaml' \
  --data-binary @app.yaml | jq '{valid, error}'
```

## The specification

[`openapi.yaml`](../api/openapi.yaml) is an OpenAPI 3.1 specification for the whole
admin API (config CRUD, settings, backup/restore, DNS sync,
Ingress discovery, and the auth endpoints), everything reachable on the admin
listener (`:8081` by default), not the proxy listeners that serve proxied
traffic.

## Reading it

- **Paste it into an OpenAPI viewer**: [Swagger Editor](https://editor.swagger.io/)
  or [Redocly](https://redocly.github.io/redoc/) both render it with no setup.
- **From a running instance**: `GET /api/openapi.yaml` serves the exact file
  embedded in the binary, so it always matches the code that built it. Auth is
  the same as `GET /api/me`: any authenticated admin session or API token, no
  particular role or scope required beyond being authenticated:

  ```
  curl -s -H 'Authorization: Bearer gpm_...' https://<admin>/api/openapi.yaml
  ```
- For the config **object model** (validation rules, YAML examples, cross-field
  behaviour) see [../configuration.md](../configuration.md): the spec documents
  the wire shape, configuration.md documents what the fields mean and do.
- For **auth/CSRF/scopes as a concept** (not per-endpoint) see the spec's
  top-level `info.description` and the `sessionCookie`/`bearerAuth` security
  schemes under `components.securitySchemes`.

## What's in it, and what isn't

- Every route `internal/api/api.go` registers (CRUD per config kind, the
  `?dryRun=1` preview and `_parse` YAML import per kind, settings,
  backup/restore, whole-config and per-object history/revert, API tokens,
  DNS-sync status/reconcile/plan, Ingress-discovery status/reconcile/plan,
  capabilities, upstream health, logs, SSO revocation) plus the admin-session
  routes in `internal/server` (login, callback, local login, logout, `/api/me`,
  `/healthz`, `/version` (any signed-in role), this spec's own route) and the opt-in `pprof`
  endpoints (only mounted with `GPM_PPROF=1`).
- Request/response schemas are derived from `internal/model`'s Go structs (the
  same types the store persists as YAML), so a field's name, type and
  `omitempty`-ness here matches the config reference exactly.
- `x-scope` on each operation names the API-token scope that route requires
  (see `components.schemas.Error` and the security description for how scopes
  compose). A session principal is governed by its role alone and is
  unaffected by scopes; scopes only ever constrain a token principal.
- The public **proxy listeners** (`:80`/`:443`) aren't an API in the REST
  sense and aren't in this document.

## Keeping it in sync

The spec is hand-authored (`internal/api/api.go` is stdlib `http.ServeMux`,
with no framework to derive an OpenAPI document from at build time), so a
route or a model field added to the code has to be added here too, **in the
same change**, per this repo's `CONTRIBUTING.md` doc-sync rule.

`go test ./internal/server` runs `TestOpenAPISpecCoversRegisteredRoutes`
(`internal/server/openapi_test.go`), which scrapes every `mux.HandleFunc`/
`mux.Handle` route literal and every `register(mux, d, "<plural>", ...)` call
out of `internal/api/api.go` and `internal/server/server.go` and fails if any
of them has no matching `path`+method in `openapi.yaml`. It catches a missing
route; it does **not** catch a route whose documented request/response shape
has drifted from the actual Go struct; review that by eye against
`internal/model` when a field changes. The per-kind template it expands from
each `register` call does not include `POST /api/{plural}/_parse`, so that
route is kept in the spec by hand.

`openapi.yaml` was scaffolded once from a generator script (to keep ~100
routes' worth of CRUD boilerplate consistent); that script isn't part of the
repo. Going forward, edit the YAML directly: a route or field at a time is
the normal case, and `TestOpenAPISpecCoversRegisteredRoutes` is what keeps
routes (not deep schema shape) honest as you do.
## API tokens (automation)

Scripts and CI authenticate with a scoped bearer token instead of an admin
session. Mint one from the **API Tokens** page, or over the API:

```
# Create. The plaintext secret is in the response ONCE and is never retrievable.
curl -s -X PUT https://<admin>/api/api-tokens/ci-deploy \
  -b "<admin session cookie>" -H "X-CSRF-Token: <token from /api/me>" \
  -H 'Content-Type: application/json' \
  -d '{"scopes":["proxy-hosts:write","certificates:read"]}' | jq -r .token

# Use it. No CSRF header is needed - a bearer token carries no ambient authority.
curl -s -H 'Authorization: Bearer gpm_...' https://<admin>/api/proxy-hosts | jq

# Rotate (old secret dies immediately) / revoke.
curl -s -X PUT    'https://<admin>/api/api-tokens/ci-deploy?rotate=1' ...
curl -s -X DELETE  https://<admin>/api/api-tokens/ci-deploy ...
```

Verification: a token restricted to `proxy-hosts:read` must get `403` on
`GET /api/settings` and on any `PUT`, and `401` once deleted or expired.
`GET /api/api-tokens` shows each token's scopes, expiry and in-memory `lastUsed`
(which resets on restart by design, see
[APIToken](config/api-token.md)).

No new flags or environment variables: tokens are ordinary config objects under
`config/api-tokens/`, so they are versioned and reviewable like everything else,
but deliberately **not revertible**: restoring an older token file would restore
an older digest and revive a rotated secret, so both the scoped and the
whole-config revert leave `api-tokens` alone. Deploy ordering note: an older binary does not know the
`APIToken` kind, so roll the new binary **before** creating the first token;
rolling back afterwards leaves `config/api-tokens/*.yaml` on disk, which an older
loader ignores (the directory is not in its kind map) but which will reappear the
moment the newer binary runs again.
