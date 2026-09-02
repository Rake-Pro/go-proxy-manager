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
| `*:read` | Every read, including whole-config reads (`/api/config`, `/api/history`, `/api/logs`, `/api/upstream-health`) |
| `*:write` | Every read and write, except the `admin`-only routes below |
| `metrics:read` | `GET /metrics` only, and nothing else |
| `admin` | Everything, and the **only** scope that reaches `/api/api-tokens`, `PUT /api/settings`, `GET /api/backup`, `POST /api/restore`, `POST /api/revert`, `POST /api/sso/revoke` and `/debug/pprof/*` |

Valid subjects are the REST resource plurals plus the pseudo-resources
`settings`, `dns-sync`, `ingress-discovery`, `docker-discovery` and `metrics`.
The full rules, including why `settings:write` does not reach
`PUT /api/settings`, are in [APIToken](config/api-token.md).

## What a non-admin caller does not see

Three read endpoints redact operational detail for a caller without the `admin`
scope. A viewer-role session and a resource-scoped token both hit this.

| Endpoint | Redacted for a non-admin caller |
|---|---|
| `GET /runtime` | `paths` and `secretFileRoots` are omitted entirely; the rest of the payload is unchanged. |
| `GET /webhooks/status` | Each target URL is reduced to `scheme://host/(redacted)`. |
| `GET /notifications/status` | The same URL redaction. |

**`GET /health` is classified for every caller, admin included.**
`acme.lastError` is always a classification plus the certificate name, e.g.
`certificate "app": dns-01 challenge or DNS provider failure`, never the
provider's raw message, whoever is asking. The full message stays on that
certificate's own status, behind `certificates:read`.

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

- Every route `internal/api/api.go` registers (CRUD per config kind, settings,
  backup/restore, whole-config and per-object history/revert, API tokens,
  DNS-sync status/reconcile/plan, Ingress-discovery status/reconcile/plan,
  capabilities, upstream health, logs, SSO revocation) plus the admin-session
  routes in `internal/server` (login, callback, local login, logout, `/api/me`,
  `/healthz`, `/version`, this spec's own route) and the opt-in `pprof`
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
`internal/model` when a field changes.

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
