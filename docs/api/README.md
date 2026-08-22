# REST API reference

[`openapi.yaml`](openapi.yaml) is an OpenAPI 3.1 specification for the whole
admin control-plane API (config CRUD, settings, backup/restore, DNS sync,
Ingress discovery, and the auth endpoints) - everything reachable on the admin
listener (`:8081` by default), not the public data plane that serves proxied
traffic.

## Reading it

- **Paste it into an OpenAPI viewer** - [Swagger Editor](https://editor.swagger.io/)
  or [Redocly](https://redocly.github.io/redoc/) both render it with no setup.
- **From a running instance**: `GET /api/openapi.yaml` serves the exact file
  embedded in the binary, so it always matches the code that built it. Auth is
  the same as `GET /api/me` - any authenticated admin session or API token, no
  particular role or scope required beyond being authenticated:

  ```
  curl -s -H 'Authorization: Bearer gpm_...' https://<admin>/api/openapi.yaml
  ```
- For the config **object model** (validation rules, YAML examples, cross-field
  behaviour) see [../configuration.md](../configuration.md) - the spec documents
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
- The public **data plane** (the proxied traffic on `:80`/`:443`) isn't an API
  in the REST sense and isn't in this document.

## Keeping it in sync

The spec is hand-authored (`internal/api/api.go` is stdlib `http.ServeMux`,
with no framework to derive an OpenAPI document from at build time), so a
route or a model field added to the code has to be added here too - **in the
same change**, per this repo's `CLAUDE.md` doc-sync rule.

`go test ./internal/server` runs `TestOpenAPISpecCoversRegisteredRoutes`
(`internal/server/openapi_test.go`), which scrapes every `mux.HandleFunc`/
`mux.Handle` route literal and every `register(mux, d, "<plural>", ...)` call
out of `internal/api/api.go` and `internal/server/server.go` and fails if any
of them has no matching `path`+method in `openapi.yaml`. It catches a missing
route; it does **not** catch a route whose documented request/response shape
has drifted from the actual Go struct - review that by eye against
`internal/model` when a field changes.

`openapi.yaml` was scaffolded once from a generator script (to keep ~100
routes' worth of CRUD boilerplate consistent); that script isn't part of the
repo. Going forward, edit the YAML directly - a route or field at a time is
the normal case, and `TestOpenAPISpecCoversRegisteredRoutes` is what keeps
routes (not deep schema shape) honest as you do.
