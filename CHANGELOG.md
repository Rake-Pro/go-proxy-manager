# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

Nothing yet.

## [1.2.0] - 2026-09-02

### Changed

- **Status and expiry chips are quieter.** Healthy/neutral states (valid
  certificate, live host, up upstream) now show as a coloured dot plus text
  instead of a filled pill, the certificates list shows a short relative
  expiry ("in 84 days") instead of a wrapping timestamp, and its Domains
  column wraps only at "." or "," with a "+N" and tooltip for long lists.

### Removed

- **The one-time configuration importer and its `gpm import` subcommand.**
  `internal/importer/` and `cmd/gpm/import.go` are gone, together with the
  `import` case in the subcommand dispatch. gpm's configuration is its own flat
  model; there is no migration path from another proxy manager.
- **The parallel-run compose file** under `deploy/`, which existed only to run
  gpm on alternate ports beside an installation being migrated away from.
- **The migration guide** and its redirect stub under `docs/`, their `mkdocs.yml`
  nav entries, and the `gpm import` row in
  `docs/reference/env-vars-and-flags.md`.

### Upgrade notes

- **No operator action is required.** No configuration key, API route, flag or
  environment variable changed, and existing YAML under `/data/config` loads
  unchanged.
- **Still need to run the importer?** Do it with **1.1.0** first, commit the
  result, then upgrade. The subcommand does not exist in this release and will
  not return.

## [1.1.0] - 2026-09-02

### Added

- **A documentation site.** `mkdocs.yml` (MkDocs Material) and
  `.github/workflows/docs.yml` build `docs/` with `mkdocs build --strict` on
  every change and publish it to GitHub Pages. Enabling Pages once, with
  **Settings -> Pages -> Source: GitHub Actions**, is a one-time owner action.
- **Community health files for a public repository.** `CONTRIBUTING.md`,
  `CODE_OF_CONDUCT.md`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/` and
  `.github/PULL_REQUEST_TEMPLATE.md`.
- **In-app help on every form control.** A `?` next to each field label opens a
  one- or two-sentence explanation plus a **Learn more** link straight to the
  matching row in the configuration reference.
  - The text and doc anchor for each control live in one registry,
    `internal/ui/hints/hints.json`, embedded in the binary and served next to
    `app.js` at `/hints.json`. No API change and no new endpoint.
  - Every list page carries an **About this page** block: what the objects are,
    three to five things that decide how they behave, and a link to the page's
    reference.
  - Terms from [terminology](docs/concepts/terminology.md) - upstream, location,
    access list, identity provider, middleware, parked, maintenance, ledger,
    reconcile, apex target, outpost - get a dotted underline in page intros and
    fold summaries, with the glossary definition on hover or focus.
  - `internal/ui/hints_test.go` enforces the coupling: every `data-hint` id
    resolves to a registry entry, every entry is used by the UI, every doc target
    resolves to a real anchor under `docs/`, and every form control either has a
    hint or is on a documented exemption list (filter boxes, bulk-selection
    checkboxes, generic key/value rows).

- **Auth middleware `mode: basic` - HTTP basic auth as a first-class auth mode.**
  Username/password gating now lives in the auth tier with every other login
  mechanism, instead of inside an access list.
  - `auth.basic.users` is a list of `username` + `passwordHash` (bcrypt);
    `auth.basic.realm` sets the `WWW-Authenticate` realm (default: the host name).
    No `identityProvider` is named - the credential set is the identity source.
  - Gates identically to the access-list basic auth it replaces: same `401` and
    challenge, same per-client-IP lockout (5 failures, 15 minutes, answered
    exactly like a wrong password), same process-wide bcrypt bound. One verifier
    serves both, so they cannot drift.
  - Adds what the access-list form never had: it sits at the auth position of the
    chain (a rate limit, an access list and a bouncer still run outside it), the
    host's custom error pages render the refusal, denials are counted, and
    `allowFrom` exempts trusted networks - the same any-of bypass `auth-request`
    and `client-cert` carry.
  - Works as an inline `auth:` block on a proxy host or location too, since both
    reuse `AuthMiddleware`.
  - A write may send a plaintext `password` per user instead of `passwordHash`;
    the API hashes it with bcrypt and stores only the hash. This works on
    `PUT /api/middlewares/{name}` and on `PUT /api/proxy-hosts/{name}` (the
    host's inline `auth` block and each location's). The plaintext is never
    persisted and never echoed back. A `passwordHash` that is not a bcrypt hash
    is refused at write time.
- **`POST /api/access-lists/{name}/migrate-basic-auth` (`admin`).** Converts one
  access list off the deprecated fields in a single commit: creates
  `<name>-basic` with `mode: basic` from the list's users, attaches it to every
  proxy host and location that references the list, copies the list's literal
  allow CIDRs to the middleware's `allowFrom` **only** when `satisfyAny` was set,
  and clears `basicAuth`/`satisfyAny`. `?plan=1` is a dry run returning the same
  payload and changing nothing. Source-backed and path-scoped allow rules are
  reported in `warnings` rather than silently widened into an auth exemption.
- **Notifications (`settings.notifications`).** Outbound ntfy/Discord/generic-
  webhook alerts on renewal failure, cert expiry, upstream health flaps, ACME
  account errors, a frozen Kubernetes/Docker discovery reconciler, and
  (opt-in) config changes.
  - Per-target `type` (`ntfy` | `discord` | `generic`), `url`, optional
    `secret` (bearer token for ntfy/generic; unused for discord), `disabled`,
    and an `events` allowlist - empty selects the default set (every kind
    except `config.changed`, which is opt-in per target).
  - `cert.expiring` and `cert.expired` are daily digests (one message listing
    every certificate in that state), not one message per certificate.
    `expiringThresholdDays` (default 14) controls the digest window.
  - A repeat of the same event within an hour is suppressed unless its state
    changed (e.g. an upstream flip from healthy to unhealthy always alerts
    immediately even inside the window).
  - `GET /api/notifications/status` (`settings:read`), `POST
    /api/notifications/{name}/test` (admin) - mirrors the existing webhook
    status/test endpoints and reuses their delivery-outcome shape.
  - Reuses `internal/webhook`'s SSRF-hardened HTTP client (redirects never
    followed, link-local destinations refused post-DNS) rather than a second
    implementation of either guard.
- **`GET /api/config/summary` (`*:read`).** Object counts per config kind
  (`proxy-hosts`, `redirect-hosts`, `stream-hosts`, `parked-hosts`,
  `certificates`, `client-cas`, `identity-providers`, `access-lists`,
  `middlewares`, `upstream-groups`, `dns-providers`, `api-tokens`), plus
  disabled/maintenance proxy-host counts and the current config HEAD. Same
  store read `GET /api/config` already does, reduced to counts, so a caller
  that only needs "how many" (the UI sidebar, to decide whether the advanced
  nav group starts open) no longer fetches the whole object graph.
- **Path and Host escape hatches: `location.stripPrefix`, `upstream.path`,
  `upstream.hostHeader`, and prefix/regex rewrite rules.** Fronting a backend
  that expects to live at a different path, or that keys its virtual host off its
  own address, previously had no answer short of changing the backend.
  - `location.stripPrefix: true` removes the location's matched prefix before
    forwarding: `/app/foo` reaches the backend as `/foo`, `/app` as `/`. A root
    location strips nothing.
  - `upstream.path` is a base path prefixed to every forwarded request (`/api` +
    `/v1/x` -> `/api/v1/x`). Absolute, no dot-segments, no query string. It works
    on a host upstream, a location upstream, and each upstream-group member,
    where the failover transport applies it per attempt. `healthCheck.path` is
    used verbatim and is not prefixed.
  - `upstream.hostHeader` selects the Host header sent upstream: empty keeps the
    client's Host (unchanged behaviour), `upstream` sends the upstream's own
    `host:port`, and anything else is sent literally (validated as a hostname,
    optionally `host:port`). Honoured per upstream-group member too.
  - The `rewrite` middleware gains `prefixRules` (boundary-matched, longest
    prefix wins) and `regexRules` (implicitly `^`-anchored, `$1` capture groups,
    compiled at config load, max 32 rules of 256 characters). Existing
    `replacePath` is unchanged and still evaluated first: exact, then prefix,
    then regex, first match wins.
  - Composition order is **location match -> `stripPrefix` -> rewrite ->
    `upstream.path` -> upstream**, all of it inside the security chain: rate
    limiting, access lists, the bouncer, auth and guards still evaluate the
    ORIGINAL client path. The query string is forwarded untouched and nothing
    here is ever an HTTP redirect - method and body are preserved.
  - Additive and `omitempty`: existing configs are unchanged on disk and in the
    API, and behaviour is identical when the fields are unset.

  ```yaml
  # config/proxy-hosts/tools.yaml
  name: tools
  domains: [tools.example.com]
  upstream: {scheme: http, host: 192.0.2.20, port: 80}
  locations:
    - path: /metrics
      stripPrefix: true            # backend expects to be at /
      upstream: {scheme: http, host: 192.0.2.21, port: 3000, hostHeader: upstream}
  ```

- **Inline `auth` and `rateLimit` on a proxy host and on a location.** Gating one
  host by SSO took three objects across three pages - an `IdentityProvider`, an
  `auth` `Middleware`, and the reference attaching it. A host (or a location) can
  now carry the `auth` / `rateLimit` block directly, so a five-host deployment
  needs only the identity provider.
  - The blocks carry the `AuthMiddleware` / `RateLimitMiddleware` shapes
    verbatim, share one validator per kind with the middleware, and compile
    through the same data-plane builder into the same handler - identical
    behaviour, metrics, error pages and denial counting.
  - `Middleware` objects remain the reuse path: one gate shared by a fleet is
    still one object every host references.
  - Not mutually exclusive. An inline block sits at its kind's chain position
    just outside the referenced middlewares of that kind, so it is evaluated
    first and every gate must pass. A location's block stacks on its host's.
  - An inline auth block's identity provider must resolve (a dangling name is a
    load-time error, and the data plane fails closed with `503`), and it
    contributes to the host's identity-header strip set and trusted-proxy set
    exactly as a referenced auth middleware does.
  - Additive and `omitempty`: existing configs are unchanged on disk and in the
    API. `settings.ingressDiscovery` / `dockerDiscovery` templates do NOT carry
    the new fields yet (see BACKLOG.md).

  ```yaml
  # config/proxy-hosts/grafana.yaml - no middleware objects needed
  name: grafana
  domains: [grafana.example.com]
  upstream: {scheme: http, host: 192.0.2.20, port: 3000}
  auth: {identityProvider: authentik, mode: forward-auth, requiredRoles: [admin]}
  rateLimit: {requests: 100, window: 1m, allowFrom: [10.0.0.0/8]}
  ```

- **Docker container discovery (`settings.dockerDiscovery`).** Containers
  labelled `gpm.rake.pro/enabled: "true"` are reconciled into managed proxy
  hosts, the way annotated Kubernetes `Ingress` objects already were. The
  homelab fleet this project targets is far more often Compose than Kubernetes,
  and hand-writing a proxy host per container was the same repetitive step
  Ingress discovery had already removed for clusters.
  - Labels (prefixed with `ingressDiscovery.annotationPrefix`, default
    `gpm.rake.pro`): `enabled`, `domains` (comma-separated, required), `port`
    (required unless exactly one TCP port is exposed), `scheme`, `profile`,
    `lan-direct`, `public-cname`. A label can never carry a middleware, an
    access list, a certificate, or an address outside its own container.
  - Upstream is the container's IP on a configurable `network`, or the
    host-published port with `usePublishedPorts: true` and `publishedHost`.
  - Reads the Engine API strictly read-only (`GET /version`,
    `/containers/json`, `/events`) over `/var/run/docker.sock`, a configurable
    `socket`, or a `tcp://`/`https://` `host` with optional client TLS - so a
    read-only socket proxy is a supported deployment, not a workaround.
  - Reconciles on container events (2s debounce) with `pollInterval` (default
    `1m`, floor `15s`) as the fallback that guarantees correctness.
  - `GET /api/docker-discovery/plan`, `POST /api/docker-discovery/reconcile`
    (409 while one is in flight), `GET /api/docker-discovery/status`; new token
    scope subject `docker-discovery`; `capabilities.dockerDiscovery.enabled`.
  - Derived hosts carry `gpm.rake.pro/managed-by: docker-discovery`, a
    different value from the Ingress reconciler's, so the two run side by side
    and neither can update or delete the other's hosts.
  - `dockerDiscovery.template` reuses the `ingressDiscovery` template type
    verbatim and `dockerDiscovery.profiles` defaults to
    `ingressDiscovery.profiles`, so a chain is written once.

  ```yaml
  dockerDiscovery:
    enabled: true
    host: tcp://socket-proxy:2375
    network: edge
    allowedDomainSuffixes: [example.com]
    template:
      tls: { certificateRef: wildcard, forceSSL: true }
      accessLists: [home-vpn]
  ```

- **Two generic DNS-01 solvers: `rfc2136` and `acme-dns`.** DNS-01 was limited to
  four named REST providers, so a zone hosted anywhere else could not get a
  certificate - and only DNS-01 can get a wildcard. Both new provider types are
  stdlib-only; no dependency was added.
  - `rfc2136` sends an RFC 2136 dynamic UPDATE signed with a TSIG key (RFC 8945)
    to any nameserver that accepts one (BIND, Knot, PowerDNS). Config:
    `server`, `zone`, `tsigKeyName`, `tsigSecret`, and optional
    `tsigAlgorithm` (default `hmac-sha256`; `hmac-md5` is refused), `ttl`
    (default 60), `transport` (`tcp` default, `udp` retries over TCP when
    truncated) and `timeout`. Present adds the TXT RR, CleanUp deletes that exact
    RR, so an apex + wildcard order sharing one challenge name is safe.
  - `acme-dns` writes the challenge to a [joohoi/acme-dns](https://github.com/joohoi/acme-dns)
    account over HTTP. Config: `baseURL`, `username`, `password`, `subdomain`.
    The operator adds one permanent
    `_acme-challenge.example.com CNAME <subdomain>.<acme-dns zone>` and gpm never
    holds a credential that can touch the real zone. CleanUp is a no-op (acme-dns
    rotates its own values); a missing delegation logs a warning rather than
    failing issuance.

  ```yaml
  name: rfc2136-example
  provider: rfc2136
  config:
    server: 192.0.2.53
    zone: example.com
    tsigKeyName: gpm-acme
    tsigSecret: ${FILE:/run/secrets/tsig_key}
  ```

- **`GET /api/runtime` (scope `settings:read`) reports how the process was
  started.** Version, HA role, the three listen addresses, config/cert/session
  paths, `metricsEnabled`, `accessLogEnabled` (live), `geoipLoaded`,
  `secretFileRoots`, `localAdminConfigured` and `pprofEnabled`. Until now none of
  this was visible from the panel, so "which directory do I back up" or "is pprof
  exposed" meant reading the container's command line. No username, hash or
  secret value is reported, and the values are captured from the flags parsed at
  startup rather than re-read per request.
- **Webhook delivery is observable and testable.** `GET /api/webhooks/status`
  returns the outcome of each target's most recent attempt (`lastAttempt`,
  `status`, `durationMs`, `ok`, `error`), and `POST /api/webhooks/{name}/test`
  (admin scope) sends a synthetic `action: "test"` event and waits up to 5s for
  the answer. Delivery was fire-and-forget, so a receiver that had been failing
  for weeks looked exactly like a working one. A disabled target is still
  testable, so a receiver can be proved before it is turned on; a refused or
  timed-out delivery is a `200` with `ok: false`, and only an unknown name is a
  `404`. The status is in-memory and per-process: an operational hint, not config.
- **The login page says when no administrator login is configured.** A banner
  names the two ways to fix it (`GPM_LOCAL_ADMIN_USER` +
  `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE`, or an `oidc` provider listed under
  `adminAuth.providers`) and shows the `hashpw` command. The same condition is
  reported as `adminLogin.configured: false` by `GET /api/capabilities`.
- **TOTP second factor for the local admin.** The one local account could only
  ever be as strong as its password. Setting `GPM_LOCAL_ADMIN_TOTP_SECRET` (or
  `_FILE`) to a base32 secret turns local login into two steps: the password
  form, then a server-rendered page asking for a 6-digit code (RFC 6238, SHA-1,
  30-second step, one step of drift either way). Implemented on the standard
  library alone - no new dependency.
  - `gpm totp-secret` mints a secret and prints an `otpauth://` enrolment URI
    for any QR encoder (`-account`, `-issuer` flags). No QR renderer ships.
  - The secret is env/file only, like the password hash: never a config field,
    never committed, and never resolvable through `${ENV:...}`.
  - A code is single-use (the accepted step is remembered in memory), the
    pending login between the two steps is single-use and expires in five
    minutes, and a wrong code counts toward the same per-client-IP lockout as a
    wrong password - a correct password with the code still outstanding no
    longer clears that lockout.
  - Reported as `adminLogin.totp` in `GET /api/capabilities` and
    `localAdminTOTP` in `GET /api/runtime`. Off unless the secret is set, so an
    existing deployment is unchanged.
- **The `user` role is a real read-only viewer.** It previously reached nothing
  but `GET /api/me`, which made it useless: anyone who needed to see the config
  had to be an admin. It now grants every `GET` under `/api/` and refuses every
  `POST`/`PUT`/`DELETE` with `403`.
  - The role is expressed as the scope grant `*:read`, the same mechanism API
    tokens use, so the two gates compose: a token presented on a `user` session
    can never grant more than that, whatever scopes it holds. Writes are
    additionally refused by method before any route is reached.
  - `api-tokens` is excluded from the grant in both directions: a token is a
    credential, not configuration, and listing them would hand a viewer the
    name, scopes, expiry and stored digest of every automation credential on the
    instance. `GET /api/config` is narrowed to match - a caller without
    `api-tokens:read` gets the whole tree with `apiTokens` omitted, so the
    exclusion cannot be walked around by asking for the config instead.
  - `GET /api/me` reports `"readOnly": true` for the role, which the admin UI
    uses to render itself read-only instead of offering controls that answer
    403.
  - Assign it with `roleMapping.userGroups` (or `defaultRole: user`) on an
    OIDC identity provider; see docs/configuration.md. Purely additive - the
    role gained access and nothing lost any.
- **Certificate health surface.** `GET /api/certificates` and
  `GET /api/certificates/{name}` now decorate every stored object with
  read-only status fields: `notBefore`, `notAfter`, `daysRemaining`, `issuer`,
  `state` (`valid` \| `expiring` \| `expired` \| `pending` \| `error`),
  `lastError`, `lastAttempt` and `sans`, computed from the certificate store
  on disk (and the ACME manager's in-memory state for the last two). Nothing
  is written back and no config field changed. See
  [Status fields](docs/reference/config/certificate.md#status-fields-get-only).
  - `POST /api/certificates/{name}/renew` (`certificates:write`) forces an
    immediate ACME order, ignoring the normal 30-day renewal window. It
    responds `{"started": true}` once the order has started, not once it
    completes; `409` if another order is already in flight anywhere on the
    instance, `400` for a `custom` certificate, `501` when this instance is
    not the ACME issuer (an HA follower).
  - `GET /api/health` (`*:read`) is a single-request aggregate: data-plane
    listener state, certificate counts by state, per-upstream-group
    healthy/unhealthy counts, the ACME renewal loop's last run and last
    error, HA role, and the current config HEAD. All in-memory - no ACME CA
    or DNS calls.
  - The ACME manager now remembers each certificate's last renewal attempt
    (`lastAttempt`) and last failure (`lastError`), and its own last full
    renewal-loop run, for the fields above.
  - See [Certificate health](docs/operations/certificate-health.md)
    for the operational walkthrough.
- **`settings.trustedProxies` and `proxyHost.trustedProxies` - one client-IP
  trust setting for the whole product.** A list of CIDRs (or bare IPs) naming the
  L7 proxies whose `X-Forwarded-For` gpm believes.
  - Empty (the default, and the value every existing config has) trusts nobody:
    `X-Forwarded-For` is ignored and the connection peer is the client.
  - When the peer is trusted, the client IP is the rightmost `X-Forwarded-For`
    entry that is not itself a trusted proxy (the standard rightmost-untrusted
    walk). Entries carrying a port and IPv6 in either form are parsed.
  - A proxy host's own `trustedProxies` **replaces** the fleet list for that
    host, so one host behind a proxy never widens another host's trust and a
    directly reached host can trust nothing.
  - That single derived address is now what **everything** compares: access-list
    rules and sources, geo rules, rate-limit buckets, the basic-auth lockout key,
    `allowFrom` on guard / rate-limit / bouncer / `auth-request` / `client-cert`
    middlewares, the access log and `/api/logs`, the request sent to a
    forward-auth or auth-request server, and the bouncer query. Derived once per
    request at dispatch, so no two tiers can disagree.
  - Entries are validated at write time; `0.0.0.0/0` and `::/0` are accepted but
    warned about on every load, because trusting every peer hands the client
    control of the address every rule compares.
  - Documented as one section, [Client IP and the three trust
    tiers](docs/concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers), which
    replaces the four separate `X-Forwarded-For` warnings scattered through the
    config reference. This closes the documented mTLS-bypass footgun: a
    `client-cert` `allowFrom` exemption now has a trusted-proxy source it can
    declare for itself.

- **Unknown YAML keys are now warned about instead of silently dropped.** Both
  config loaders (per-object files and `settings.yaml`) already used a
  non-strict decode, so a file carrying a key the running binary's struct does
  not know - most commonly one only a newer gpm writes - loaded fine before
  this release too, just silently. Load now also decodes each file a second
  time against its own generic shape, diffs the keys, and logs one `WARN` per
  affected file:

  ```
  config: proxy-hosts/app.yaml: unknown keys ignored: auth, rateLimit - written by a newer gpm? see docs/operations/upgrading.md#rollback
  ```

  - Checked recursively through nested objects (e.g. `upstream.path`) and
    slices of objects (`locations[0].rewrite...`); a `map[string]string` field
    or any other map-typed field (`labels`, `securityHeaders`, discovery
    `profiles`) is opaque - its own keys are data, not schema, and are never
    flagged.
  - The same warnings are appended to the existing config-warnings list
    (`Config.Warnings()`), so they show up everywhere that list already does:
    the startup and reload log, and `GET /health`'s new `configWarnings` field.
  - `ApplyBatch` - the write path used only by the Ingress and Docker discovery
    reconcilers, never an operator - now refuses to upsert an object whose file
    currently has unknown keys, logging a `WARN` and skipping it instead. An
    automatic reconcile can no longer be the write that turns a silently
    ignored key into a permanently lost one; an explicit `PUT` from the API or
    UI is unaffected and can still update the object. See "Rolling back" under
    Upgrade notes below.

### Security

- **The admin panel makes no third-party request.** Space Grotesk, Inter and
  JetBrains Mono are vendored into the binary (`internal/ui/static/fonts/`,
  latin subset, one variable `woff2` per family) instead of loaded from Google
  Fonts, so the operator's browser IP and `Referer` no longer reach a third
  party on every admin page view and the panel works on a management VLAN with
  no outbound internet.
  - The Content-Security-Policy loses both external origins with it:
    `style-src 'self' 'unsafe-inline'; font-src 'self'`. It now names no host
    other than `'self'`.
- **A warning banner when the admin session cookie is not `Secure` on a public
  address.** `capabilities.adminLogin.cookieSecure == "insecure-public"` means
  this session's cookie went out without the `Secure` flag over plain HTTP to a
  routable client, so it can be read in transit. The banner links to
  [hardening](docs/operations/hardening.md#admin-session-cookie).
  `insecure-private` (loopback / RFC 1918) and `secure` stay quiet.

- **Admin cookies are `Secure` on every channel that can carry one, and never
  downgraded.** With `GPM_COOKIE_SECURE=auto`, a request over TLS, through a
  trusted proxy sending `X-Forwarded-Proto: https`, or against an `https://`
  `externalBaseURL` gets a `Secure`, `__Host-gpm_session` cookie.
  - An **untrusted** peer's `X-Forwarded-Proto` is never read, so a client cannot
    talk gpm into or out of `Secure` with a header; the trusted set is
    `settings.trustedProxies`, the same one the data plane uses.
  - A presented `__Host-` cookie on a request that would answer non-`Secure` is
    refused outright rather than re-issued without `Secure`: the session is never
    silently downgraded, and the operator logs in again.
  - Logging out clears both cookie names, so no dead credential is left behind.
  - Plain HTTP from a non-private client logs one `warn` per hour and is reported
    as `capabilities.adminLogin.cookieSecure: "insecure-public"` for the UI.

- **A rewrite target can no longer escape the upstream base path.** A `rewrite`
  middleware's `to` (exact, prefix and regex forms) is now held to the same rules
  as `upstream.path`: no `.`/`..` segment, no backslash, no `;`, no query or
  fragment. The composed path is re-cleaned and re-confined to the upstream base
  path in the data plane as well, so both the single-upstream and the
  upstream-group paths compose identically.
- **A per-host `trustedProxies: []` now means "trust nobody".** The field is a
  nullable list: absent inherits `settings.trustedProxies`, present-and-empty
  trusts no peer for that host. It previously fell back to the fleet-wide list,
  which trusted more than the operator asked for.
- **An unreadable `X-Forwarded-For` entry ends the client-IP walk.** A token that
  is not an address (an RFC 7239 obfuscated node identifier, `unknown`, a `unix:`
  marker) now stops the rightmost-untrusted walk and falls back to the connection
  peer instead of continuing into entries the client wrote. Unspecified,
  broadcast and multicast addresses are treated the same way, the walk and the
  chain gpm forwards are bounded, and entries that do not parse are dropped from
  the outbound `X-Forwarded-For`.
- **Admin login and TOTP lockouts key on the derived client IP.** With the admin
  UI fronted by a gpm proxy host (a documented deployment) every attempt arrived
  from one address, collapsing the login lockout, the TOTP throttle and the
  pending-login cap into a single global bucket. The admin server now derives the
  client from `settings.trustedProxies` exactly as the data plane does; an
  untrusted peer's `X-Forwarded-For` is still ignored.
- **The REST DNS clients no longer follow redirects, and their base URL is
  checked.** acme-dns and the token-authenticated REST providers use the same
  SSRF-hardened HTTP client as webhooks and notifications, so a 3xx is surfaced
  rather than chased (their credentials ride custom headers the stdlib does not
  strip across hosts) and link-local destinations are refused post-DNS. An
  acme-dns `baseURL` pointing at a loopback, link-local, unspecified or multicast
  literal is refused at write time; `config.allowInsecureLocal: "true"` opts a
  loopback deployment back in.
- **The basic-auth migration no longer turns an OR into an AND.** When the list
  set `satisfyAny` and every rule moved into the middleware's `allowFrom`, the
  list is now detached from the hosts and locations that gained the middleware -
  leaving it attached made its rules mandatory and locked out password users
  outside them. A list that still carries a deny rule, a source-backed rule or
  geo rules stays attached and the plan payload says so (`detachAccessList`, plus
  an explicit warning).
- **RFC 2136 zone auto-detection validates the SOA owner.** An owner name from an
  unauthenticated SOA probe is accepted only when it is a suffix of the name
  being solved, so a spoofed answer cannot choose the zone a TSIG-signed UPDATE
  names. An UPDATE reply is also rejected unless it is a response (`QR` set) with
  the UPDATE opcode.
- **Deployment layout and receiver URLs are admin-only.** `GET /api/runtime`
  omits `paths` and `secretFileRoots` for a caller without the admin scope, and
  `GET /api/webhooks/status` / `GET /api/notifications/status` redact the path
  and query of each target URL for the same caller (for Discord, Slack and ntfy
  receivers the URL path is the credential). The redaction now fails CLOSED: a
  status value it does not recognise as the expected delivery-list type is
  withheld entirely for a non-admin caller instead of passed through unredacted.
- **`GET /api/health` and `GET /api/certificates` report a classification, not
  the raw ACME error, for a caller without the admin scope.** The message could
  embed a snippet of a third party's HTTP response body (a DNS provider's error
  detail); both routes now name the certificate and the failure class for a
  non-admin caller. The full message is available only to an admin caller, on
  either route.
- **Forced certificate renewal is throttled per certificate.**
  `POST /api/certificates/{name}/renew` now refuses a second request for the
  same certificate within 1 hour of its last attempt (successful or not) with
  `409`, naming the remaining wait. `certificates:write` is deliberately
  narrower than admin, and without this cooldown a caller holding only that
  scope could script repeated renews and exhaust the ACME directory's
  duplicate-certificate rate limit, taking the operator's own renewal loop
  offline for the rest of the window.
- **Bounded outbound fan-out and dedup growth.** Webhook delivery is capped at 8
  concurrent posts instead of one goroutine per target per event, the
  notification dedup map evicts entries past its window, and `Notifier` has a
  `Close()` so its workers can be stopped.
- **The local admin can require a second factor.** See the TOTP entry above: a
  stolen or guessed admin password is no longer sufficient on its own, and the
  code step is throttled by the same per-IP lockout as the password step, is
  single-use, and cannot be retried against a spent pending login.
- **A read-only role means a viewer login is no longer an admin login.** Giving
  someone visibility into the proxy configuration used to mean giving them the
  ability to change it. The `user` role now reads everything except API tokens
  and writes nothing, enforced twice over (method gate plus scope gate) and
  composed with API-token scopes so neither can widen the other. The API-token
  exclusion also narrows `GET /api/config`, so the credential rows cannot be
  read out of the whole-config payload instead.

### Fixed

- **Every editor save keeps the object's `labels` and `tags`.** A `PUT` is a
  whole-object replacement and the save bodies were rebuilt from the form, so
  two keys no editor renders a control for were deleted on every save.
  - The proxy-host editor dropped `labels`, which is the discovery ownership
    marker (`<prefix>/managed-by: ingress-discovery` / `docker-discovery`): a
    no-op Save on a discovered host orphaned it and the reconciler stopped
    managing it.
  - The shared editor behind middleware, access lists, identity providers, DNS
    providers, upstream groups, redirects, streams, parked hosts and client CAs
    dropped both `labels` and `tags`.
  - Both now seed the body from the object as loaded. `createdAt` / `updatedAt`
    are still left to the store.
- **A failed reference-list fetch can no longer strip a host's middleware chain
  or its access lists.** `GET /api/middlewares` returning 500 rendered an empty
  check-list with no warning, and the next Save wrote the host back without its
  auth middleware and without its IP allowlist while the toast said "Saved".
  Every reference list now has the three states the client-CA picker already
  had - loaded, empty, failed - and a failed one shows an explicit banner,
  disables Save, and leaves the stored references exactly as they are.
- **Four `data-hint` attributes rendered as visible text.** They were written
  outside their opening tag on the auth and rate-limit blocks, so the literal
  `data-hint="..."` string appeared on most editor pages and those four controls
  had no `?` button.
- **Capabilities are re-read after a Settings save.** Saving Settings dropped the
  capability cache, and a dropped cache reads as "every capability false" on
  every route that did not reload it - so fleet-wide maintenance was invisible
  on the hosts list the moment an operator turned it on, including on the view
  they turned it on from. Every route now refreshes the probe before it renders,
  and the save refreshes the banners on the view already on screen.
- **`/api/restore` redirects to the login page on 401.** An expired session
  during a restore surfaced as `Restore failed (401)` and the operator re-picked
  the archive instead of being re-authenticated. All three request paths (the
  `api()` helper, the PKCS#12 download and the restore upload) now share one
  401 handler.
- **A read-only viewer no longer gets a generic load error on `#/tokens`.** API
  tokens are admin-scope for reads as well as writes; the page now says so
  instead of issuing a `GET` that comes back 403.
- **An explicitly stored `tls.minTLSVersion: "1.2"` survives a save.** It was
  materialised away because 1.2 is the default; absent stays absent and present
  stays present.
- **A `headers` middleware with nothing filled in is refused.** It used to commit
  `headers: {}`, a middleware that changes no request - `guard` and `rewrite`
  both already refused the equivalent.
- **A capability-gated `<a>` leaves the tab order.** Greying one out relied on
  `pointer-events: none`, which stops the mouse but not Tab+Enter; gated anchors
  now lose their `href` and get `tabindex="-1"`.
- **In-app help "Learn more" links resolve again.** 71 configuration-reference
  anchors moved when the settings pages were split per sub-block; the hint
  registry follows them.

- **The first login of a fresh install works.** `GPM_COOKIE_SECURE` defaulted to
  `1`, so the quick start's `http://127.0.0.1:8081` login set a `Secure` cookie
  the browser refused to store: the POST succeeded, the next request was
  anonymous, and login appeared to do nothing. The new `auto` default issues a
  non-`Secure`, bare-named `gpm_session` cookie on that connection and upgrades
  it the moment the panel is reached over HTTPS. No cookie setting or redeploy is
  part of the quick start any more.

- **A save with no edits no longer writes defaults into the config.** Several
  editors serialised whatever their widget was rendering, so clicking Save
  without changing anything materialised keys the file never had:
  `tls.hsts.maxAge: 31536000` on a proxy host, `accessListSync.enabled: true` and
  `ingressDiscovery.template.upstream` (and the same block on a discovery
  profile) in settings, `targetScheme: auto` and `statusCode: 301` on a redirect
  host, `statusCode: 404` on a parked host, and `defaultAction: deny` on an
  access list. Each of these is now sent only when the loaded object already
  carried the key or the operator chose a value other than the rendered default,
  so an untouched save produces an empty diff.

- **Every save from the web UI reset the object's `createdAt`.** The UI never
  sends `createdAt` on `PUT /api/<kind>/{name}`, and the store only preserved
  an existing object's creation time when the incoming value was non-zero -
  otherwise it stamped `now`, so every edit rewrote the true creation time in
  git. The store now looks up the object already on disk under the same
  `kind`/`name` and, when found, always keeps its `createdAt`: an update can no
  longer lose the field by omitting it, and - server wins - can no longer
  overwrite it by sending a different value either. `createdAt` is honoured
  from the request only when the object is genuinely new (e.g. a batch
  import restoring timestamps); an existing object whose stored `createdAt` is
  itself zero (written before this field existed) is backfilled to `now`
  instead of staying zero. Applies to `PUT` (`Save`), `SaveBatch` and
  `ApplyBatch` (the Ingress/Docker discovery reconcilers) for every kind that
  carries `ObjectMeta`; `Revert`/`RevertObject` restore the file exactly as
  committed and were never affected. No operator action required.

- **Saving Settings wiped `appName` and `accessListSync`.** `PUT /api/settings`
  is a whole-object replacement and the Settings page rendered neither field, so
  every save from the UI silently reset the instance name to the default and
  re-enabled (or reset the interval of) the access-list source fetcher. Both now
  have controls on the page, and a test reflects over `model.Settings` so a
  top-level field added later fails the build until it is either edited on that
  page or explicitly carried forward.

- **`adminAuth.ssoOnly` could lock every operator out on a typo.** Validation
  only checked that `adminAuth.providers` was non-empty, so a name that did not
  resolve - or one naming a `forward-auth` / `auth-request` provider, which
  renders no sign-in button - produced a login page with zero buttons and, under
  `ssoOnly`, no password form either: recoverable only by editing the config repo
  and redeploying. A settings write now resolves every entry against the real
  identity providers and is refused with an error naming the provider and why it
  cannot be used. Configs without `ssoOnly` are unaffected (the local form is
  still there), and nothing is checked at load time, so an existing tree still
  boots.
- **Starting with no admin login at all was a single warn line.** It is now an
  `error`-level line naming both remedies and the `hashpw` command, and the state
  is visible in the API and on the login page instead of only in the log. The
  check also covers the half-configured cases the old one missed (a username with
  no hash, `localLoginEnabled: false` with no providers).
- **API errors leaked Go type names.** `GET /api/proxy-hosts/typo` answered
  `{"error":"ProxyHost typo not found"}`, and a malformed body returned
  encoding/json's own text (`... into Go struct field ProxyHost.tls.forceSSL of
  type bool`). Both now go through one translation layer: not-found uses the
  human noun (`proxy host "typo" not found`), and a decode failure names the JSON
  path the caller sent (`field tls.forceSSL expects true or false, got a string`;
  `invalid JSON at offset 14: ...`). `model.Validate()` messages pass through
  unchanged - they already name the JSON field path. The `{"error": "..."}`
  envelope and every status code are unchanged.
- **A proxy, redirect or parked host pointing at a certificate that does not
  cover it said nothing.** `tls.certificateRef` is inert for L7 hosts (selection
  is by SNI), so such a config validates, commits, and then serves a different
  certificate - or fails the handshake. gpm now logs a `warn` line at load and on
  every reload naming the host, the certificate and the domains. It is a warning,
  not an error: the config is legal.

### Changed

- **Embedded UI assets are conditionally cacheable.** `app.js`, `app.css`,
  `theme-init.js`, `hints.json` and the vendored fonts are served with a strong
  `ETag` computed at startup from their bytes plus `Cache-Control: no-cache`, so
  a page load costs one conditional request per asset instead of re-downloading
  ~600 KB. The SPA shell keeps `Cache-Control: no-store`, and an upgraded binary
  invalidates every entry with no cache-busting query string to maintain.
- **The shell and the view no longer fetch the same thing twice.**
  `/api/settings`, `/api/history` and `/api/config/summary` are memoised for the
  duration of one route render (a cold `#/overview` issued all three twice); a
  Settings save drops the memo.
- **The API Tokens nav entry is hidden for the read-only `user` role**, which
  cannot read or write tokens.

- **`GPM_COOKIE_SECURE` is tri-state and defaults to `auto`.** The value is now
  `auto` (per-request, the default), `1` (always `Secure`) or `0` (never). The
  explicit values keep exactly their previous meaning, `0` still logs the
  startup warning against an `https://` `externalBaseURL`, and a deployment that
  reaches gpm over TLS or sets an `https://` `externalBaseURL` keeps `Secure`
  cookies without changing anything.
  - The cookie name follows the decision: `__Host-gpm_session` when `Secure`,
    `gpm_session` when not. Both names are accepted on the way in, so a live
    session survives putting TLS in front of the admin panel.
  - `GET /api/capabilities` gained `adminLogin.cookieSecure`
    (`"secure"` / `"insecure-private"` / `"insecure-public"`).
  - `-cookie-secure` still accepts `-cookie-secure` and `-cookie-secure=0`; the
    new third value is written `-cookie-secure=auto`.

- **Admin UI, second polish pass: Settings split into tabs, a real Integrations
  page, and every backend feature that shipped without a control now has one.**
  - **Settings** is four tabs with stable deep links (`#/settings/general`,
    `/headers`, `/advanced`, `/operations`). General opens with a read-only
    Runtime card (version, HA role, listeners, config and certificate
    directories, secret-file roots, local admin login and TOTP, metrics, access
    log, GeoIP, pprof), then Identity, Admin sign-in and a new Client IP card for
    `settings.trustedProxies`. Response headers gains a **Load recommended**
    button that pastes `model.RecommendedSecurityHeaders` without overwriting a
    header you already set. Advanced holds PROXY protocol, the shared annotation
    and label prefix, metrics and HA facts. Operations is a visually distinct
    danger zone for fleet-wide maintenance and Revoke all SSO sessions. Every
    tab's Save sends the WHOLE settings object, so no tab can wipe another.
  - **Integrations** (`#/integrations`) is no longer a placeholder: DNS sync,
    Kubernetes Ingress discovery (moved here whole, with its template, profiles
    and selection rules behind folds), Docker discovery, Access-list sync,
    Lifecycle webhooks and Notifications, each with its own status panel, plan or
    reconcile actions and Save. All six save through one carry-forward helper
    that starts from the settings object as loaded.
  - **Proxy host editor**: inline **Sign-in** (`auth`) and **Rate limit**
    (`rateLimit`) folds on the host and on every location, a location **Strip
    prefix** switch with a live preview, upstream **Base path** and **Host
    header** on the host, each location and each upstream-group member, and a
    three-state **Trusted proxies (override)** control (inherit / trust nobody /
    custom list) under Advanced. The request-flow diagram shows the inline blocks
    at their chain positions.
  - **Middleware editor**: `mode: basic` with a write-only plaintext password
    field and a realm, and the rewrite editor split into Exact, Prefix and Regex
    groups with per-row validation.
  - **Access lists** stop offering `basicAuth` and `satisfyAny`. A list that
    still carries them shows a read-only deprecation card with a **Migrate**
    button (plan preview, then apply) and a Deprecated badge in the list.
  - **DNS providers** gain `rfc2136` and `acme-dns` with typed credential forms;
    the `apiToken` requirement is now per provider instead of unconditional.
  - **Certificates** show issuer, a state-coloured expiry chip and a per-row
    **Renew now**; the editor gains a read-only Status card and the same action.
  - **Overview** reads `GET /api/health` for a real data-plane liveness tile
    (which listener is down, which upstream group is degraded), links the
    Certificates tile when something is expiring, and shows a dismissable
    get-started checklist driven by `GET /api/config/summary`.
  - The sidebar counts come from `GET /api/config/summary` instead of pulling the
    whole config, and a caller whose role is `user` gets a read-only banner, every
    write control disabled and the API Tokens entry hidden.
- **The pre-paint theme guard is a file, not an inline script.** The admin
  listener sends `Content-Security-Policy: script-src 'self'`, which blocked the
  inline block in `index.html` outright - so the guard it exists for never ran and
  every page load logged a CSP violation. It now loads as
  `static/theme-init.js`, still synchronously in `<head>` before the stylesheet.


- **`identityProvider.forwardAuth.trustedProxies` no longer influences the client
  IP.** It keeps its original and only documented meaning - which peers may
  assert identity headers (`Remote-User`, `X-authentik-*`, ...) - and is now one
  of three independent trust tiers, alongside `settings.proxyProtocol.trustedCIDRs`
  (L4: who may rewrite the connection address) and `settings.trustedProxies`
  (L7: whose `X-Forwarded-For` is believed). The two are deliberately NOT merged
  on upgrade: an identity-header grant and an address-rewrite grant are different
  permissions, and copying one into the other would widen an operator's intent
  without being asked. A config that sets `forwardAuth.trustedProxies` while
  `settings.trustedProxies` is empty logs a `WARN` at every load carrying the
  exact YAML block to add. See the upgrade note below.
- **`X-Real-IP` is now sent to upstreams**, carrying the derived client IP, and
  `X-Forwarded-For` is rebuilt from it: the genuine inbound chain plus gpm's peer
  when the peer is a trusted proxy, and the peer alone otherwise. Previously the
  upstream always received only gpm's connection peer (Go's reverse proxy drops
  the client-supplied chain before gpm's rewrite hook runs), so a backend behind
  a declared trusted proxy saw the proxy's address rather than the browser's.
- **Admin UI: grouped navigation, progressive disclosure and list tooling.** The
  sidebar was seventeen flat entries that gave a daily task the same rank as a
  yearly one, and the proxy-host editor put thirteen sections on one page.
  - The sidebar is grouped (Overview, HOSTS, SECURITY, OPERATIONS, and a
    collapsible ADVANCED that opens itself when the install actually uses
    something in it, and remembers the choice per browser). Route ids are
    unchanged, so every existing deep link still resolves; only labels moved,
    and multi-word labels are now Title Case ("Parked Hosts", "Error Pages",
    "Identity Providers").
  - New `#/integrations` section, a placeholder until DNS sync, ingress and
    Docker discovery, access-list sync and webhooks move into it.
  - Proxy-host, redirect, stream and parked editors fold every optional section
    to a one-line summary of what it holds ("Locations (2)", "Timeouts
    (defaults)"). A section holding a non-default value opens itself.
  - Every list page has a text filter that matches the object's stored values
    rather than the rendered chip text; Proxy Hosts and Certificates sort by
    column; Proxy Hosts gained row selection with confirmed bulk enable,
    disable, add tag and remove tag.
  - Selects show a human label beside the raw config token
    ("Forward-auth (trusted headers) - forward-auth") from one shared map.
  - A failed save is a banner pinned to the top of the editor until the next
    attempt, not a toast that disappears after seven seconds; a server error
    naming a dotted field path highlights and scrolls to that field.
  - Half-filled rows in the location, access-list rule, source and basic-auth
    editors now block the save with an inline row error instead of being
    dropped silently.
  - Per-object history revert and "revoke all SSO sessions" use the in-app
    confirmation dialog instead of `window.confirm`.
  - The certificates list has an Expiry column. `GET /api/certificates` returns
    the certificate spec rather than the issued material, so it renders `-`
    until the API surfaces `notAfter`.

- **The discovery reconcile plan moved to `internal/discovery`.** Ownership (by
  name and by domain), freeze-on-unreadable-source, the fail-closed disable on
  an unresolvable profile, and the operator-owned `disabled`/`maintenance`
  carry-forward are now one implementation shared by the Kubernetes and Docker
  reconcilers instead of two. No behaviour, wire shape or label changed;
  `internal/k8s` keeps its own status payload.

- **Three config fields are deprecated and documented as ignored.**
  `proxyHost.tls.http2` (HTTP/2 is always offered - ALPN is unconditionally
  `h2,http/1.1`), `proxyHost.websocketsUpgrade` and its
  `ingressDiscovery.template` copy (upgrades always work), and
  `identityProvider.oidc.trustIdPMFA` (gpm has no local MFA prompt to suppress
  and never reads `acr`/`amr`). All three are still parsed, so existing YAML
  loads unchanged and the NPM importer keeps round-tripping them; they are gone
  from `docs/api/openapi.yaml` and from the `docs/configuration.md` field tables,
  each replaced by a "Deprecated fields" row saying what it was for and why it
  decides nothing. `ingressDiscovery` still stamps `websocketsUpgrade` onto
  derived hosts so a pre-existing template does not show up as an update on every
  reconcile.
- **The documentation is reorganised into a task-shaped tree.** `docs/` is now
  `getting-started/`, `concepts/`, `how-to/`, `reference/` and `operations/`,
  with one reference page per config object kind and one per settings section.
  The three monolithic pages are split: `configuration.md` into
  `reference/config/`, `deployment.md` across `getting-started/`, `reference/`,
  `how-to/` and `operations/`, and `architecture.md` into
  `concepts/architecture.md` plus `concepts/request-pipeline.md`. New pages
  cover terminology, "which mechanism do I use?", a consolidated troubleshooting
  table and a quickstart that ends at a working certificate.
  **The five old paths are kept as redirect stubs**, so an external link to
  `docs/configuration.md`, `docs/deployment.md`, `docs/architecture.md`,
  `docs/ha.md` or `docs/migrating-from-npm.md` still resolves and names its
  replacement.
- **Every config-reference field row carries a stable `<kind>-<key>` anchor.**
  The convention, including the rule that an existing anchor is never renamed,
  is in `docs/reference/config/README.md`; the in-app hint registry is generated
  against it.
- **`docs/reference/config/certificate.md` now states where `tls.certificateRef`
  is honoured.** A "Which certificate a host serves" section: SNI selects the
  certificate for proxy, redirect and parked hosts (the ref is an intent record
  only), while a stream host in `terminate` mode is the one place the ref is
  authoritative.
- **Admin UI: controls that decided nothing are gone.** The per-host HTTP/2 and
  WebSocket switches, the OIDC "Trust IdP MFA" switch, and the certificate picker
  on proxy, redirect and parked hosts backed config fields the data plane does
  not read (`tls.http2`, `websocketsUpgrade`, `oidc.trustIdPMFA`, and
  `tls.certificateRef` on an L7 host, which is selected by SNI). The certificate
  picker is replaced by a read-only line naming the certificate each domain
  actually resolves to, and warning when none covers it; the Stream host editor
  keeps its picker, where `certificateRef` is authoritative. Existing values are
  carried forward verbatim on save, so nothing written in git is dropped. The
  "Break-glass localhost only" switch is also gone: it wrote a key
  `AdminAuthSettings` does not have, so it was discarded on read.
- **Admin UI: destructive and hard-to-undo actions now confirm properly.** One
  in-app dialog replaces the browser's `confirm()` for whole-config revert and
  archive restore (both require typing `REVERT` / `RESTORE`), fleet-wide
  maintenance (typing `MAINTENANCE`, and it says how many hosts will answer 503),
  HSTS preload, and SSO-only mode. While fleet-wide maintenance is on, every page
  carries a banner saying so. Unsaved changes in an editor or on Settings now
  prompt before navigating away or closing the tab, and the History page's revert
  controls are real buttons, reachable from the keyboard.
- **Admin UI: admin providers are picked from a list.** The free-text chip input
  on Settings is now a checkbox list of the OIDC identity providers that exist
  (a stored name that is not one is still listed and kept), and SSO-only mode is
  disabled until at least one exists.

### Deprecated

- **`AccessList.basicAuth` and `AccessList.satisfyAny`.** Still parsed and still
  enforced - existing YAML keeps working unchanged - but gone from the UI, marked
  `deprecated` in the OpenAPI schema, and named in a load-time `WARN`. Migrate
  with `POST /api/access-lists/{name}/migrate-basic-auth`, or by hand: move the
  users to an auth middleware with `mode: basic`, and turn `satisfyAny` into
  `allowFrom` on that middleware. **Removed in v2.**

### Upgrade notes

- **`GPM_COOKIE_SECURE` now defaults to `auto`.** No action is needed for a
  deployment that terminates TLS in gpm, sets an `https://` `externalBaseURL`,
  or lists its TLS-terminating proxy in `settings.trustedProxies`: cookies stay
  `Secure` and `__Host-` named, and an explicit `GPM_COOKIE_SECURE=1` or `0`
  keeps exactly its previous meaning.
  - **Check one case:** an external terminator that is NOT in
    `settings.trustedProxies` while `externalBaseURL` is unset or `http://`.
    Cookies there were `Secure` before and are not now. Fix it by setting
    `externalBaseURL` to the `https://` URL (recommended), adding the
    terminator's address to `settings.trustedProxies` so its
    `X-Forwarded-Proto: https` is believed, or pinning `GPM_COOKIE_SECURE=1`.
    `GET /api/capabilities` reports the live answer as
    `adminLogin.cookieSecure`, and a plain-HTTP session from a public address
    logs one `warn` per hour.

- **API clients can no longer set `createdAt` on an existing object.** A `PUT`
  to a `kind`/`name` that already exists now keeps the stored `createdAt`
  outright: an incoming value is ignored rather than merged. A client that reads
  an object, edits a field and `PUT`s the whole thing back is unaffected - the
  round-trip simply stops being able to move the timestamp. `createdAt` is still
  honoured on a **create**, so an importer restoring historical timestamps keeps
  working. No config change is required.
- **Action required only if you relied on `forwardAuth.trustedProxies` for
  client-IP resolution.** Until this release, the forward-auth
  `trustedProxies` of the identity providers a host referenced doubled as that
  host's `X-Forwarded-For` trust. It no longer does. If your gpm sits behind an
  L7 proxy and you configured a forward-auth provider naming that proxy, add the
  same CIDRs to `config/settings.yaml`:

  ```yaml
  trustedProxies:
    - 10.0.0.0/8       # the CIDRs from your forwardAuth.trustedProxies
  ```

  gpm logs a `WARN` at every load naming the exact block to paste when it detects
  this case (a `forwardAuth.trustedProxies` set, with no `settings.trustedProxies`
  and no per-host `trustedProxies` anywhere). Until you add it, `X-Forwarded-For`
  is ignored and every IP-based rule compares the proxy's own address: access
  lists and geo rules match the proxy, all clients share one rate-limit bucket
  and one basic-auth lockout, the access log records the proxy, and any
  `allowFrom` network containing the proxy exempts everyone arriving through it.
  Configs that never set `forwardAuth.trustedProxies`, or where gpm is the
  internet-facing edge, are unaffected - the new default is the behaviour they
  already had.
- **`proxyHost.trustedProxies` is now a nullable list.** Existing YAML and JSON
  keep loading unchanged: omitting the key still inherits `settings.trustedProxies`
  and a non-empty list still replaces it. What changed is that an explicit
  `trustedProxies: []` now means "trust nobody for this host" instead of silently
  inheriting the fleet list - if you wrote an empty list expecting the fleet
  default, remove the key. The field was added in this same unreleased cycle, so
  no released config is affected.
- **The basic-auth migration now detaches a fully-migrated access list.** Running
  `POST /api/access-lists/{name}/migrate-basic-auth` on a list that set
  `satisfyAny` and carries only literal allow CIDRs removes that list from the
  hosts and locations that gain the new middleware. Run `?plan=1` first and check
  `detachAccessList` if you relied on those hosts keeping the reference.
- Nothing else breaks in this release. An access list carrying `basicAuth` loads,
  validates and gates exactly as before; the only visible change is a `WARN` at
  startup naming it. No operator action is required now.
- **v2 will remove `AccessList.basicAuth` and `AccessList.satisfyAny`**, at which
  point a config still carrying them will fail to load. Migrate at any point
  before then; the endpoint above does it in one commit and `?plan=1` shows what
  it would do first.

- **Rolling back to 1.0.33 or earlier after using any feature added in this
  release is NOT safe.** This release is a **minor version** (1.1.0), not a
  patch, for exactly this reason: both config loaders use a non-strict YAML
  decode, so a key an older struct does not recognise is silently dropped -
  and the first write after that (including an automatic reconciler commit,
  not just an operator save) makes the drop permanent. Downgrading past a
  config a 1.1.0+ binary wrote, or has written to since, can silently lose:
  - an inline `auth:` or `rateLimit:` block on a proxy host or location - the
    host loads **unauthenticated** on 1.0.33, with no error;
  - `upstream.path`, `upstream.hostHeader`, a location's `stripPrefix`, or any
    `trustedProxies` list (host or fleet-wide `settings.trustedProxies`);
  - a `rewrite` middleware's prefix or regex rules;
  - `settings.notifications` and `settings.dockerDiscovery`.

  A middleware with `auth.mode: basic` is worse than silently dropped: 1.0.33
  does not know the `basic` mode at all and **refuses to start** with that
  middleware in config.

  If you must roll back, recover the config first rather than let 1.0.33 start
  against it: `git log --oneline` inside `/data/config` (the config repo, not
  the gpm source checkout) to find the commit before you started using a
  1.1.0+-only feature, then `git checkout <that commit> -- .` to restore it,
  commit, and only then start 1.0.33. Load now warns about exactly this
  situation - see "Unknown YAML keys are now warned about instead of silently
  dropped" above and [Rollback](docs/operations/upgrading.md#rollback).

## [1.0.33] - 2026-09-01

### Fixed

- **Release image rebuilt its `apk upgrade` layer from cache.** The final
  Dockerfile stage was cached by the GitHub Actions layer cache, so the
  `apk upgrade` meant to pick up Alpine security patches only ran when its RUN
  line changed. v1.0.32 failed the release scan gate on libexpat 2.8.3-r0 with
  2.8.4-r0 already in the 3.24 repos. The final stage is now named and excluded
  from the layer cache (`no-cache-filters: final`) in both the CI and release
  builds; the Go build stage stays cached.

## [1.0.32] - 2026-09-01

### Added

- **Access-list remote sources and path-scoped rules.** An `AccessList` rule can
  now be limited to exact request paths and methods (`paths`, `methods` -
  defaulting to `GET`/`HEAD`), and can draw its networks from a named remote feed
  (`source`) instead of a literal `cidr`. Together they answer the case a single
  ANDed allow-list could not express: letting a monitoring provider's published
  prober addresses reach *only* the health endpoints of a host that is otherwise
  LAN/VPN-only.

  ```yaml
  rules:
    - {action: allow, cidr: 10.0.0.0/8}
    - action: allow
      source: uptimerobot
      paths: [/api/health, /v1/health, /-/healthy, /status.php]
      methods: [GET, HEAD]
  sources:
    - name: uptimerobot
      url: https://uptimerobot.com/inc/files/ips/IPv4andIPv6.txt
      interval: 24h
  ```

  Ordering is unchanged (top-down, first match wins, then geo, then
  `defaultAction`), and a rule without `paths` behaves exactly as before, so
  `paths` only ever narrows. Matching is against the already-normalised request
  path, so gpm's matched path and the forwarded path stay byte-identical.
  `paths` are **allow-only** (`action: deny` alongside them is refused at
  validation): the match is exact, case-sensitive and does no trailing-slash
  folding, which fails closed for an allow and would fail *open* for a deny -
  deny-by-path stays the guard middleware's job.

  A new leader-only fetcher (`internal/accesssync`) keeps the sets current on
  `settings.accessListSync.pollInterval` (default `15m`, per-source `interval`
  default `24h`), writing them to the committed singleton
  `config/access-list-sources.yaml`. It fails closed throughout: `https` only,
  no proxy honoured (a proxy would move the address the SSRF guard inspects),
  the dialer refuses loopback/link-local/private/ULA/CGNAT/multicast and other
  non-public destinations post-DNS, the body is capped at 1 MiB, and a non-200,
  an empty result, more than `maxEntries` (default 10000), a *single* unparseable
  line, or any entry that is a default route, broader than a `/8` (v4) or `/32`
  (v6), or inside one of those non-public ranges refuses the fetch whole and
  keeps the previously fetched set. A refused fetch is retried on the next poll
  rather than after the source's interval. A source that has never been
  fetched resolves to the empty set, so it matches nothing. An unchanged feed
  writes no ledger and therefore produces no commit churn. New endpoints
  `GET /api/access-list-sources/status` (`access-lists:read`) and
  `POST /api/access-list-sources/reconcile` (`access-lists:write`).

  Because a raw stream carries no request path and resolves no ledger, an
  AccessList with any `sources`, or any rule carrying `paths` or `source`, is
  refused for a `StreamHost` at validation - the same way a `basicAuth` list
  already was.

  The admin UI's access-list editor authors all of this: sources (name/url/
  interval/maxEntries rows), a cidr-vs-source match per rule, and its `paths`/
  `methods`, plus a per-source sync status panel and a manual reconcile button.

### Fixed

- **WebSockets through an upstream group.** A host backed by an `upstreamGroupRef`
  answered every WebSocket handshake with a 502 (`internal error: 101 switching
  protocols response with non-writable body`) and leaked the upstream connection
  until garbage collection: the group transport wrapped the upstream response
  body in a read-only counter, but a 101 body is the hijacked upstream
  connection and `httputil.ReverseProxy` requires it to be an
  `io.ReadWriteCloser`. The counter now keeps the Writer side when the body has
  one, so protocol switches proxy exactly as they do on a single-upstream host.
  Regression test covers a full handshake plus bidirectional bytes through a
  group. (Every group-backed host was affected since upstream groups shipped.)

## [1.0.31] - 2026-08-31

### Added

- **Maintenance mode.** A proxy host can be taken out of service for a downtime
  window without being deleted, disabled, or losing its DNS: while
  `proxyHost.maintenance` is true gpm answers every request to that host itself
  - HTTP `503` with a `Retry-After` - and never dials the upstream. The host
  keeps its domains, its certificate and its DNS records, so clearing the flag
  restores service with no other change. `settings.maintenance.enabled` is the
  fleet-wide half: it puts **every** proxy host into maintenance whatever its own
  flag says, so the whole edge goes down (and comes back) in one write.
  `settings.maintenance.retryAfterSeconds` sets the header on both (default 300,
  max 24h). Both switches are in the UI: a Maintenance toggle beside Disabled in
  the host editor, a Maintenance mode card on the Settings page, and a
  `maintenance` status chip in the host list.

  Neither switch needs a restart. The fleet-wide one is a package-level atomic
  installed alongside the settings-level error pages and security headers, read
  live on every request, so it applies without even a router rebuild; the
  per-host flag is compiled into the host handler by the reload every config
  write already triggers. The check sits at the router dispatch layer, ahead of
  path normalization, the identity strip and the whole middleware chain, so a
  window costs no auth subrequest and no upstream dial - but *inside* the mTLS
  gate, so a host requiring a client certificate still answers `421` rather than
  disclosing a window to an uncertified caller.

  The page is not a second mechanism: maintenance renders through the same
  `serveErrorPage` seam as every other gpm-generated error, so a custom
  maintenance page is simply the `503` entry in `errorPages` (host override
  first, then the settings-level pages). The built-in fallback negotiates on
  `Accept` - JSON for `application/json`, HTML for a browser, plain text for
  anything else including `*/*` - and **always** sets `Content-Type` and a body:
  a bodyless, type-less error response from this proxy has crashed a real API
  client before.

  Redirect, parked and stream hosts proxy nothing and keep serving. ACME
  HTTP-01 is answered before host routing, so certificates still renew during a
  window. There is no scheduling: a window opens and closes when an operator
  flips a switch.
- **Ingress discovery preserves `maintenance`.** Discovery derives the whole
  managed host from the template and the `Ingress` on every poll, so a flag no
  annotation expresses would be reset within a minute of being set. `maintenance`
  is now carried forward from the stored host exactly as `disabled` is: it is
  operator-owned state, and the next poll putting a host back into service while
  someone is working on its backend is precisely the failure the flag exists to
  prevent. Steady state is unaffected - a host in maintenance whose `Ingress` has
  not changed still writes nothing.

## [1.0.30] - 2026-08-31

### Changed

- Bumped `modernc.org/sqlite` from 1.56.0 to 1.57.0 (dependency maintenance, no functional change).

## [1.0.29] - 2026-08-27

### Added

- **UI editor for `stripResponseHeaders`.** The last of the API-first response
  header fields gains its editor: the Settings page ("Strip response headers")
  edits the fleet-default list and the proxy-host editor edits that host's
  additions (unioned on the data plane), both as chip lists, replacing the
  save-time carry-forward. An emptied list leaves the field off the save body
  rather than committing an empty array. Client-side validation covers token
  syntax and case-insensitive duplicates; hop-by-hop and response-semantic
  refusals stay server-side, where the 400 names the exact header, so the two
  policies cannot drift.
- **Live access-log toggle.** Request capture no longer needs a restart:
  `PUT /api/logs {"enabled": true|false}` (admin scope) flips it at runtime,
  and the Access Logs page gains an Enable/Disable capture button. The toggle
  is runtime-only - never persisted - so a restart reverts to `-access-log` /
  `GPM_ACCESS_LOG`.

  The zero-overhead promise survives: instead of a permanently installed
  observer checking a flag, each data-plane listener serves through a
  two-position handler switch (plain dispatch chain vs observed chain) selected
  by one atomic pointer load per request. While every observability toggle is
  off the plain chain serves - no wrapper, no allocations, no clock reads - and
  toggling swaps the observed chain in for subsequent requests; an in-flight
  request finishes on the chain it started with, so it logs (or doesn't)
  consistently. A startup toggle that always needs the wrapper (`-metrics`,
  `-slow-request-ms`, `-debug-headers`) keeps the observed chain active, and
  only the access-log work inside it is skipped.

## [1.0.28] - 2026-08-26

### Added

- **UI editor for `securityHeaders`.** The field was config- and API-only; both
  places it exists now have a real editor. The Settings page gains a "Response
  security headers" section for the fleet default, and the proxy-host editor
  gains the same section for that host's per-key override, each a list of rows:
  header name, value, and a scope select (`all` / `generated-only` /
  `proxied-only`), with add and remove controls.

  Serialization is chosen to keep git diffs empty rather than to mirror the
  editor's internal shape: a header at scope `all` is written back as a **bare
  string**, exactly as the Go marshaller emits it, so an untouched map
  round-trips byte-for-byte through a save; only a non-default scope is written
  as `{value, scope}`. A hand-written `{value, scope: all}` object is normalized
  to the bare string on the first save through the editor (the two spellings mean
  the same thing, and the API already renders that header as a bare string). A
  value shape this build does not understand - for instance a scope added to the
  API later - is shown read-only and emitted **verbatim**, so a newer config
  survives an older UI. An empty editor leaves the field **absent** rather than
  committing an empty map, and rows with a blank header name are ignored.

  Both saves are whole-object replacements, so the editor now owns the field and
  the previous carry-forward guards (which copied the loaded map onto every save)
  are removed with it; the "a save never drops `securityHeaders`" invariant is
  pinned by the editor's serialization tests instead. Duplicate header names
  (case-insensitive) and names that are not valid tokens are refused client-side
  with the usual toast; everything else is left to the API's `400`.

### Security

- **Container base packages upgraded at build time.** The final image now runs
  `apk upgrade` against the Alpine 3.24 repositories before installing runtime
  deps: the digest-pinned base image lags security patches that ship to the
  repos without a new base release (openssl CVE-2026-14456, fixed in 3.5.8-r0,
  failed the v1.0.27 release's image scan gate this way).

## [1.0.27] - 2026-08-26

### Added

- **Edge response-header stripping.** A new `settings.stripResponseHeaders` list
  (with a per-`ProxyHost` `stripResponseHeaders` that is **unioned** with it, and
  an `ingressDiscovery` template/profile field so discovery-managed hosts inherit
  it instead of having it reverted every reconcile) removes backend-identifying
  response headers - `Server`, `X-Powered-By`, `X-AspNet-Version`, ... - from
  what an upstream sends. Matching is case-insensitive (names are canonicalized
  to MIME header form).

  The removal happens in the reverse proxy's `ModifyResponse` hook, on the
  upstream's own header map before it is copied onto the client response. That
  placement means it can reach **only** what the backend sent: everything gpm
  adds - injected `securityHeaders`, HSTS, `X-Robots-Tag`, the `Set-Cookie`
  forward-auth copies back on an IdP session refresh, gzip's
  `Content-Encoding`/`Vary`, a headers middleware's `setResponse` - survives
  regardless of the strip list. It also covers a `101 Switching Protocols`
  handshake, which the response-writer layer never sees (the stdlib hijacks the
  connection), so a WebSocket upgrade no longer leaks the fingerprint every other
  response hides. gpm-generated responses (denials, sign-in redirects, error
  pages, the `400`/`404`/`421`, the upstream-unreachable `502`/`504`,
  parked/redirect hosts) involve no upstream response and are untouched.

  A header named in both the strip list and `securityHeaders` ends up **present
  with gpm's configured value**. Per-host is a union rather than a per-key
  override because a list has no per-name value to override, and a host must not
  be able to re-expose a header the fleet strips. Rejected at config write
  (`400`): invalid header names, case-insensitive duplicates, hop-by-hop headers,
  and the response-semantic headers `Content-Type`, `Content-Length`,
  `Content-Encoding`, `Vary`, `Location` and the `Sec-WebSocket-*` handshake trio
  (stripping `Content-Type` would hand the body to Go's content sniffing, which
  can re-label a JSON response as `text/html`; stripping `Sec-WebSocket-Accept`
  would break every browser WebSocket handshake now that 101s are in scope). `Set-Cookie` and `WWW-Authenticate` are allowed and documented as
  sharp edges - they remove the backend's own values. Empty (the default) strips
  nothing. The headers middleware's `removeResponse` is unchanged and remains for
  per-middleware/per-location removal; the new list is the recommended edge-wide
  mechanism (`removeResponse` runs inside the auth tier of the chain, so it never
  sees a response generated by a gate ahead of it).

## [1.0.26] - 2026-08-26

### Added

- **Per-header scope for `securityHeaders`.** Each configured header now carries
  a `scope`: `all` (default - both gpm-generated and proxied responses, today's
  behaviour), `generated-only` (gpm's own responses only, **never** a proxied
  upstream), or `proxied-only` (proxied responses only). This closes the gap
  where a header safe on gpm's own pages breaks a backed app when injected onto
  its proxied responses - e.g. `Content-Security-Policy: frame-ancestors 'none'`
  and `Permissions-Policy` break Home Assistant (no CSP of its own; relies on
  same-origin add-on iframes), so the recommended set now places those two at
  `generated-only` and the rest at `all`. The value is **either** a bare string
  (scope `all`) **or** a `{value, scope}` object; the legacy plain
  `map[string]string` config and API payload keep working unchanged (bare string
  => scope `all`), and an all-scope header still marshals back to a bare string.
  A header is declared once, at one scope (a duplicate name across scopes is
  rejected); an unknown scope is rejected. The data plane distinguishes
  generated from proxied at inject time via the reverse proxy's `ModifyResponse`
  (which fires only when an upstream actually answered), so the
  upstream-unreachable `502`/`504` stays gpm-generated. HSTS/`X-Robots-Tag`,
  set-if-absent, and `1xx` survival are all unchanged.

## [1.0.25] - 2026-08-26

### Added

- **Configurable security response headers on gpm's own responses.** A new
  `settings.securityHeaders` map (with a per-`ProxyHost` `securityHeaders` that
  merges over it per key) sets response headers gpm emits on the responses **it**
  generates - auth-gate denials (`401`/`403`/`503`), the OIDC/forward-auth sign-in
  redirects (`302`), rendered error pages, the path-rejection `400`, the
  no-such-host `404`, the misdirected `421`, and parked/redirect hosts. They are
  injected at the data-plane dispatch layer, the same place per-host HSTS is
  emitted and deliberately **outside** the middleware chain, which is why these
  denials previously carried only HSTS (a headers middleware sits inside the
  chain and never runs when a gate refuses ahead of it). Injection is
  **set-if-absent**, so a proxied upstream response keeps its own
  `X-Frame-Options`/`Referrer-Policy`/etc. - gpm never clobbers or duplicates an
  app's own security header. Ships **nothing** by default (opt-in; existing
  deployments are unchanged); a recommended paste-ready set is documented in
  `docs/configuration.md`. Names are validated (no CR/LF, no hop-by-hop headers,
  de-duplicated case-insensitively) and `Strict-Transport-Security` is refused -
  the per-host `tls.hsts` setting still owns HSTS, whose behaviour is unchanged.
  The Settings and host editors carry the field forward untouched on save (no
  dedicated editor yet), guarded by a test. An upstream `1xx` interim
  response (`103 Early Hints` / `100 Continue`) does not drop the injected
  headers - they land on the final response; the per-host HSTS and `X-Robots-Tag`
  emission was moved onto the same writer so it survives a `1xx` too.

## [1.0.24] - 2026-08-26

### Added

- **Error pages get their own UI section.** Custom error pages moved out of the
  Settings page into a top-level **Error pages** section, alongside the other
  object sections, since they are content an operator iterates on rather than a
  switch set once. The config schema is unchanged - the section edits
  `settings.errorPages`, a host's override stays in the host editor, and a
  git-authored `settings.yaml` is unaffected. Settings keeps a one-line pointer
  to the new section and now carries `errorPages` forward untouched on save.
- **Auth-gate refusals honour custom error pages.** Every terminal refusal an
  auth middleware writes now renders through the same `errorPages` machinery the
  access-list, guard, bouncer and rate-limit tiers already use, instead of a
  hardcoded plain-text body: forward-auth `401`/`403`, client-cert `401`/`403`,
  auth-request `403`/`502`, the OIDC gate's `403`/`401`/`400`/`404`/`502`/`500`,
  and the `503` a middleware that cannot be compiled serves. The per-host
  `errorPages` override applies exactly as it does elsewhere, and the default
  bodies are unchanged - with nothing configured the output is byte-identical,
  headers included. Redirects into a sign-in flow are untouched, and in
  `auth-request` mode a response proxied from the identity provider always wins:
  gpm never overwrites the IdP's own sign-in, callback or sign-out pages.
- **Enable mTLS on a proxy host from the UI.** The host editor's TLS section now
  always shows the "Client certificates (mTLS)" block with real controls - an
  enable toggle, a Client CA picker populated from the client-cas list, and the
  `require`/`optional` mode - instead of rendering a read-only summary only when
  `tls.clientAuth` already existed. A host can finally be put behind mTLS without
  hand-editing config. Preconditions follow the grey-out convention: the toggle is
  disabled with the reason when `forceSSL` is off or no enabled ClientCA exists,
  the picker shows a disabled state pointing at the Client CAs page when there are
  none, a disabled CA cannot be selected, and turning `forceSSL` off under a live
  mTLS host is blocked rather than silently dropping `clientAuth`. The gate is
  one-way - turning mTLS *off* is always allowed, so an already-invalid stored
  combination is recoverable from the page. A `caRef` missing from the CA list is
  rendered flagged and round-tripped on save instead of the select silently
  retargeting the trust anchor, and a failed client-cas fetch is a distinct state
  from "no CAs defined": the picker says so, saves of other fields still work, and
  `caRef`/`mode` are left exactly as stored. Identity
  passthrough is unchanged and now nests inside the enabled state; turning the
  toggle off omits `clientAuth` entirely, which drops `identityHeaders` with it -
  they live inside `clientAuth` in the model and mean nothing without it. Both
  `clientAuth` and its `identityHeaders` are merged over the stored object before
  the rendered fields are set explicitly, so a field this form does not render -
  GitOps-authored, or added to the model later - survives a save from this page.

### Changed

- **The client-cert auth gate's 401 body is now the generic "authentication
  required"**, identical to the forward-auth gate, instead of "client certificate
  required" - an external probe can no longer learn from the body that a client
  certificate is what gates the host. A deployment that wants a specific,
  operator-written message there (a private host where the hint is useful rather
  than a disclosure) can now supply one as a `401` error page, per host - see the
  auth-refusal entry under Added.

## [1.0.23] - 2026-08-26

### Added

- **Generate a client CA from the UI/API**: `POST /api/client-cas/{name}/generate`
  ("Generate new CA" on the new-ClientCA page) creates a self-signed RSA-4096 CA
  (`CA:TRUE, pathlen:0`, `certSign|cRLSign`, random serial), writes its private
  key to `<certDir>/client-cas/{name}.key` at `0600`, and saves the ClientCA
  object pointing at it - so a working mTLS setup needs no external tooling.
  Optional `commonName` (defaults to the object name), `validityDays` (1-7300,
  default 3650) and `organization`. Unlike issuance this is a config mutation: it
  commits and appears in history like a `PUT`, and returns the created object. The
  key is never returned or logged; an existing object name is a `409`, as is a key
  file another ClientCA still references (the error names it), with nothing on
  disk changed. An unreferenced key file at that path is reclaimed and logged - it
  can only be residue from a crash or a deleted CA, and refusing it would burn the
  name permanently. A failed config save removes the key it just wrote so the name
  can be retried, and abandoned `.tmp-*` key files older than an hour are swept. Deleting a ClientCA does **not** delete its key file, the
  same way a deleted Certificate keeps its ACME artifacts.

### Changed

- **ClientCA screen rework.** The new/edit page was inconsistent - some fields
  carried three-line explanations and some none, and each either/or pair (CRL
  file vs inline, CA key file vs inline) rendered as two stacked controls the
  operator had to know were mutually exclusive. Now: one field pattern
  everywhere (label, control, at most one hint line), each either/or pair is a
  single segmented picker over one control, and Revocation and Signing key are
  collapsed sections showing a one-line summary of what is configured, opening
  automatically when they hold values. The multi-sentence helper prose moved to
  docs/configuration.md. Gating, the expiry banner, the superseded-row treatment
  and follower read-only gating are unchanged, but the **save semantics of the
  two either/or pairs did change**: a save now submits the selected side when it
  holds a value and otherwise preserves whatever the unselected side already
  held, so toggling a picker to look at the other option and saving is a
  byte-for-byte no-op. Clearing the visible field still removes the value. On the
  new-CA page the Revocation and Signing key sections are hidden while "Generate
  new CA" is selected, since `POST /generate` does not accept them, and cloning a
  ClientCA lands on "Paste existing CA" because the clone already carries a
  certificate.

## [1.0.22] - 2026-08-25

### Added

- **Client-certificate issuance from a ClientCA**: an optional signing key
  (`caKeyFile`, cert-store-relative like `crlFile`, or an inline `caKeyPEM`
  secret) turns a verify-only trust anchor into an issuing CA, and
  `POST /api/client-cas/{name}/issue` mints a client certificate signed by it,
  returned as a password-protected PKCS#12 download (`commonName`, `password`,
  optional `validityDays` 1-3650 default 365, optional `sans`). RSA-2048 and the
  legacy PKCS#12 encoder are deliberate: iOS rejects ECDSA client certificates
  and PBES2 bundles, as do several Android/Wear OS releases. The generated
  private key is never persisted or logged - it exists only in the response - and
  the route creates no config revision or history entry. Gated by
  `client-cas:write` like the CA's other writes. New UI card on the ClientCA
  editor, greyed out with the reason when no signing key is configured. New
  dependency: `software.sslmate.com/src/go-pkcs12` (pure Go, no transitive tree
  beyond `golang.org/x/crypto`, which was already direct).
- **Client-certificate expiry warnings and renewal**: every issuance is now
  recorded (CA, common name, SANs, serial, validity - never key material) as
  runtime state under `<certDir>/client-certs/<ca>.json`, written atomically at
  `0600` like the ACME issued-certificate metadata, so records survive a restart
  without entering the git-backed config or creating a revision.
  `GET /api/client-cas/{name}/certificates` lists them with a derived
  `ok`/`expiring`/`expired` status against the CA's new `expiryWarningDays`
  (0-3650, default 30), and the ClientCA page shows a banner before expiry naming
  each certificate and its remaining days.
  `POST /api/client-cas/{name}/certificates/{serial}/renew` reissues the recorded
  identity (same common name and SANs, not accepted from the request) with a new
  key and serial and marks the old record superseded; the UI action is behind an
  explicit confirmation. **Renewing does not revoke** - the old certificate stays
  valid until it expires (revocation is CRL-only), and there is no client-side
  renewal, so every device must import the new `.p12` by hand.
- **`allowFrom` in `client-cert` auth mode**: the same network exemption
  `auth-request` mode has. A client whose resolved IP (trusted-proxy-aware
  `X-Forwarded-For` walk) matches a listed CIDR is proxied straight through with
  no certificate requirement and no `clientCertRoles` check, so a host can run
  `tls.clientAuth.mode: optional` and require certificates from everyone except
  the LAN. Such a request carries no client-certificate identity headers upstream.
  The exemption is matched against the client IP the *host* resolves; a
  client-cert middleware has no identity provider, so it contributes no trusted
  proxies and on a pure client-cert host the comparison is against the raw TCP
  peer. docs/configuration.md carries the warning: gpm must be the edge, or an L7
  proxy whose address falls inside an `allowFrom` network exempts all traffic
  through it.

### Changed

- Client-certificate issuance now enforces a **12-character minimum bundle
  password** on both issue and renew, and refuses non-ASCII or control characters
  in `sans` with `400` instead of failing inside ASN.1 encoding. The password floor
  exists because the legacy PKCS#12 encoder (kept for iOS/Android import) derives
  its integrity MAC with a single KDF iteration, so the password is effectively the
  bundle's only at-rest protection.
- Renewing an already-superseded certificate record is refused with `409` naming
  the superseding serial, and the ledger's supersede write now errors instead of
  silently appending when its target is absent. Either would otherwise leave two
  records looking current.

## [1.0.21] - 2026-08-22

### Changed

- API Tokens page: the scope table no longer scrolls inside the card, and
  checking write simply auto-selects read instead of greying it out.

## [1.0.20] - 2026-08-22

### Fixed

- API tokens minted before the ParkedHost rename that carry `dead-hosts:read` /
  `dead-hosts:write` scopes load again and grant the equivalent `parked-hosts`
  scope; 1.0.18 refused the whole config (and crash-looped the edge) on such a
  token. Rotate them at leisure; no action is required to start.

## [1.0.19] - 2026-08-23

### Changed

- Added `CLAUDE.md` documenting project conventions and doc-sync rules (no functional change).

## [1.0.18] - 2026-08-22

### Changed

- **BREAKING - two renames, neither migrated automatically.** `DeadHost` is now
  `ParkedHost` (`config/dead-hosts/` -> `config/parked-hosts/`, `/api/dead-hosts`
  -> `/api/parked-hosts`, scope subject `dead-hosts` -> `parked-hosts`,
  `deadHosts` -> `parkedHosts`), and a stream host's `forwardHost`/`forwardPort`
  are now a single `target: {host, port}`. Migration, before starting the new
  binary: (1) `git mv config/dead-hosts config/parked-hosts` and commit -
  startup and reload fail with that command in the error while the old directory
  still holds objects, because gpm will not author a commit in your config repo;
  (2) rewrite every `config/stream-hosts/*.yaml` `forwardHost`/`forwardPort` pair
  as `target`, and update any API token naming a `dead-hosts:*` scope. A file
  still using the old stream keys is rejected at load rather than accepted with
  no backend; a backup archive taken before the rename still restores, its
  `dead-hosts/` entries mapped onto `parked-hosts/` one-way at restore time.
- API Tokens page: the scope picker is a compact grouped table (Hosts / Trust
  and auth / Routing / Operations) with header select-all toggles and a locked
  read box when write is checked, replacing the card grid; `metrics` no longer
  offers a write box (`metrics:write` does not exist); the token list is a
  table with a Last used column; the top-bar identity block ellipsizes instead
  of overflowing; the sidebar "data plane: live" indicator is gone.
- Public-release prep: example zones, fleet tables and test fixtures use
  `example.com` / `proxy-admins`; status glyphs replaced with plain text;
  "Relationship to Nginx Proxy Manager" section added to docs/architecture.md.

## [1.0.17] - 2026-08-22

### Added

- **Prometheus metrics** (opt-in): `GET /metrics` on the admin listener,
  enabled with `-metrics` / `GPM_METRICS=1` (404 when off), gated by admin
  role plus a dedicated `metrics:read` API-token scope. Text exposition from a
  small internal package (`internal/metrics`), no new dependency. Per-host
  request counts / latency histogram / bytes / in-flight, upstream errors,
  websocket upgrades, denials by tier, stream connections, ACME expiry and
  renew failures, DNS-sync and Ingress-discovery timestamps, HA role, build
  info, Go runtime. Host labels are operator object names, never client
  `Host` headers; every metric caps its series count. `dnsSync` status now
  reports `lastSuccess`.
- **Bouncer middleware (WAF/CrowdSec deny hook)**: new `bouncer` middleware
  type, hook-only. Providers `crowdsec` (LAPI flow; `ban`/`captcha` deny;
  optional `stream: true` keeps decisions in memory with CIDR support) and
  `http` (generic 2xx=allow / 403=deny). Runs after the access list and
  before auth; `allowFrom` bypass; `onError: fail-open|fail-closed` (default
  fail-open); bounded per-IP verdict cache, error verdicts capped at 5s.
- **Inbound PROXY protocol** v1 + v2 via `settings.proxyProtocol` (`enabled`,
  `trustedCIDRs` required, `timeout` 5s) on the :80/:443 and TCP stream
  listeners. Honoured only from a trusted peer; the parsed source replaces
  the connection address so every IP-based control sees the real client.
  Config is read live per connection.
- **Stream TLS/SNI routing**: `StreamHost.tls.mode: passthrough|terminate`
  with `sniMatch` and (terminate) `certificateRef`; several hosts may share
  one TCP port when all are SNI-routed.
- **L4 access lists on stream hosts** (`accessLists`, IP/CIDR + geo),
  evaluated before any backend dial.
- **Per-host gzip compression** (`compression`: `enabled`, `minBytes` 1024,
  `types`), stdlib only, Accept-Encoding aware, skips upgrades/event-streams/
  already-encoded responses, sets `Vary`.
- **Custom error pages** (`settings.errorPages`, per-host `errorPages`:
  `dir`, `inline`, `interceptUpstream`) for gpm-generated errors (upstream
  unreachable, denied, rate-limited, dangling reference, parked host), rendered
  through html/template.
- Web UI: **Clone** action on every object kind (name/secrets cleared,
  domains cleared for host-like kinds); **light theme** (auto/light/dark
  toggle in the top bar, persisted, applied before first paint); login page
  follows the OS preference. Stream editor gains TLS/SNI/cert/ACL fields;
  Settings gains PROXY protocol, error pages and metrics cards.
- IPv6: dual-stack binds verified by test; `IPv6` subsection in
  docs/deployment.md for the Docker-side requirements.
- **ACME HTTP-01 challenge support**: `certificate.acme.challenge: http-01`
  issues without any DNS credential; the data plane's plaintext `:80` listener
  serves `/.well-known/acme-challenge/<token>` ahead of host routing, the
  force-SSL redirect and auth. `dns-01` stays the default for certificates that
  reference a DNS provider and remains required for wildcards. Unknown tokens
  fall through to normal routing so an upstream's own ACME client keeps working.
- **ACME External Account Binding** (`acme.eab.kid` + `acme.eab.hmacKey`) for
  CAs that require it (ZeroSSL, Google Public CA examples in docs); EAB
  accounts get their own account key per key id.
- **Three more DNS-01 providers** on their plain REST APIs, no new
  dependencies: `digitalocean`, `hetzner`, `desec`.
- **`ingressDiscovery.annotationPrefix`** makes the Kubernetes Ingress
  discovery annotation/label prefix configurable (default `gpm.rake.pro`,
  unchanged for existing deployments). Changing it while hosts are labelled
  under the old prefix is refused at settings-write time unless
  `ingressDiscovery.annotationPrefixMigrate: true`, which relabels them in the
  next reconcile's single commit.
- **OpenAPI 3.1 spec** at `docs/api/openapi.yaml` (102 operations), served at
  `GET /api/openapi.yaml`, with a route-coverage test.
- Admin UI: per-location middleware and access-list pickers in the host
  editor; certificate editor gains a challenge selector (dns-01 greyed until a
  DNS provider exists) and EAB fields; DNS provider editor offers all four
  providers.
- Docs: bare-metal/systemd deployment, backup/restore, upgrade/rollback,
  image signature verification, and a "Users, roles and audit" stance section.

### Changed

- `auth.allowFrom` is now **refused at validation** in `oidc` and `forward-auth`
  mode, including when `mode` is unset and the referenced provider's `type`
  resolves to one of them (or cannot be resolved at all). Those gates have no
  network bypass, so the value was silently ignored; it was already documented as
  unsupported there. `auth-request` and `client-cert` are unaffected.
- Reverse proxy: upstream timeouts now respond 504 Gateway Timeout instead of
  502 (connect/reset failures remain 502).
- Startup logs a warning naming `GPM_LOCAL_ADMIN_USER` /
  `GPM_LOCAL_ADMIN_PASSWORD_HASH` and `settings.adminAuth.providers` when
  neither is configured (the admin panel was previously unreachable with no
  diagnostic).
- CI: every third-party GitHub Action is pinned to a full commit SHA;
  `staticcheck` and `govulncheck` are required jobs (`make lint`, `make vuln`);
  the Trivy gate now fails on HIGH as well as CRITICAL.
- FEATURES.md rewritten as a status board; CHANGELOG restructured into
  per-version sections; `docs/deployment.md` documents `GPM_GEOIP_DB` and the
  absence of a `/metrics` endpoint.

### Fixed

- Admin UI: saving a proxy host with `locations` no longer drops each
  location's `middlewares` / `accessLists` when the host was authored outside
  the UI; the save merges over the loaded location by path, mirroring the
  `tls.clientAuth` merge.
- Certificate type "Custom (upload)" relabelled "Custom (file on server)" with
  the cert-store-relative path convention (there is no upload endpoint).
- Dead `ghcr.io/rake-pro/go-proxy-manager:main` image references and a dead
  runbook link in `deploy/compose.parallel.yaml`.

### Security

- A PROXY header from a peer outside `trustedCIDRs` is treated as payload and
  the peer address stands (warned once per peer); a malformed header closes
  the connection; a stalled one is cut at `timeout`.
- Config validation rejects a stream host referencing an access list with
  `basicAuth` users, `tls` on a udp/both stream, port sharing unless all hosts
  are SNI-routed, and duplicate SNI claims; a stream host whose access list or
  certificate cannot be resolved is dropped rather than served ungated.
- Released images are signed keylessly with cosign via GitHub Actions OIDC.
- Data-plane OIDC gate: a request whose `Host` is not a configured domain of
  the gated host is refused (404) instead of minting and caching a per-Host
  relying-party client with live IdP discovery. Discovery runs outside the
  gate's mutex with single-flight per domain; the client cache is bounded.
- HA follower: the config pull has a 60s timeout, the network fetch runs
  without the config write lock (only the fast-forward takes it), and
  `GIT_TERMINAL_PROMPT=0` is in force.
- Data-plane listeners set `ReadTimeout` (60s) and `IdleTimeout` (90s); upgrades
  and body-bearing proxied requests clear their deadlines, so websockets,
  streams and large uploads are never truncated.
- Data-plane basic auth: failed attempts are throttled per client IP (5 / 15
  min, bounded map that fails closed when saturated) and concurrent bcrypt
  verifications are bounded process-wide. A locked-out client receives the
  same 401 as a wrong password.
- Reverse proxy re-asserts gpm's own identity headers on the outbound request;
  a client `Connection: X-Forwarded-User` previously had the header removed by
  the hop-by-hop purge.
- Guard middleware: a guard matching on `queryEquals` rejects (400) a request
  whose raw query contains `;` rather than evaluating a query gpm and the
  upstream may parse differently.
- Cookie MACs are domain-separated (SSO session, SSO login-state, sticky
  cookie). **Existing SSO sessions and sticky assignments are invalidated once
  on upgrade.**
- Test coverage for the admin login flow: `internal/server` 44% -> 91%,
  `internal/auth` 64% -> 93%.

## [1.0.16] - 2026-08-22

### Added

- **High availability, phase 1** (active/standby pair, no new dependency):
  `GPM_HA_ROLE=leader|follower` (default `leader`, unchanged single-node
  behaviour) designates the single writer. A follower runs no ACME renewal and
  no Ingress-discovery loop, refuses every admin/API config write with a `503`
  naming the leader, and pulls the leader's config repo with
  `git pull --ff-only` every `GPM_HA_POLL_INTERVAL` (default 20s), reloading
  only when HEAD moved and never merging, rebasing or resetting a diverged
  repo. `GET /api/capabilities` reports `ha.role` / `ha.readOnly` and the admin
  UI greys out write controls on a follower. Every instance re-reads the
  persisted SSO revocation watermark on a ticker and advances it
  monotonically, so a revoke takes effect within one interval instead of at
  the next restart. Deployment recipe in `docs/ha.md`.
- **mTLS phase 2: client-certificate revocation.** `ClientCA` gains `crlFile`
  (PEM or DER, confined relative to the cert store), `crlPEM` (inline) and
  `crlPolicy` (`fail-closed` default | `fail-open`). Revoked serials are
  rejected at the TLS handshake via `VerifyPeerCertificate`, the CRL signature
  is validated against the CA bundle, and `nextUpdate` is honoured. CRLs
  reload on the config-reload path and on a 5-minute mtime watch.
- **mTLS phase 2: identity passthrough.** `tls.clientAuth.identityHeaders`
  forwards the verified certificate's subject (`X-Client-Cert-Subject`, name
  overridable) plus opt-in `san` / `serial` / `fingerprint` headers. All four
  default names joined the baseline identity denylist, so they are stripped
  from untrusted peers whether or not a host enables passthrough.
- **mTLS phase 2: `client-cert` auth-middleware mode**, gating on a verified
  client certificate with an optional `clientCertRoles` subject-to-role mapping
  (no identity provider). "cert OR SSO" = `tls.clientAuth.mode: optional` plus
  this middleware as the host's auth tier.
- Admin UI: Client CAs section with the CRL fields, mTLS passthrough controls
  in the host TLS editor, `client-cert` mode in the middleware auth editor.
- `GET /api/ingress-discovery/plan` dry-run endpoint, mirroring
  `GET /api/dns-sync/plan`; wired into the settings UI as *Preview changes*.
- Operator-side Ingress-discovery profile selection
  (`ingressDiscovery.profileRules`, `profileSelection: "rules-only"`),
  evaluated before the `gpm.rake.pro/profile` annotation.
- Per-profile `allowedDomainSuffixes` narrowing the global Ingress-discovery
  suffix list, validated as a subset at settings-write time.

### Fixed

- A discovery-managed proxy host disabled by an operator is no longer
  re-enabled by the next poll; discovery only clears a `disabled` state it set
  itself (unresolvable-profile hold), tracked via the `gpm.rake.pro/disabled-by`
  label.
- Admin UI: saving a proxy host no longer drops a GitOps-authored
  `tls.clientAuth` block.

### Security

- `Principal.SessionID` is no longer serialised by `GET /api/me`.

## [1.0.15] - 2026-08-21

### Changed

- Go toolchain 1.27rc2 -> 1.27.0

## [1.0.13] - 2026-08-16

### Changed

- **The host editor's middleware / access-list pickers show attached entries
  first and grew from ~7 to ~18 visible rows.** Both check-lists rendered in
  collection (file) order inside a 220px scroll box, so an attached entry
  sorting past the cutoff - `sso` sorts *after* `sso-lan`, `-` < `.` in file
  names - was clipped out of view behind an overlay scrollbar and the host
  looked unprotected in the UI while the data plane was enforcing the chain
  all along. Checked entries now render at the top in the host's stored chain
  order (which the DOM-order save path then preserves on re-save), and the
  list cap is 560px. Display-only; no host was ever actually missing its
  middleware.

## [1.0.10] - 2026-08-01

### Added

- **Discovery templates and profiles reach parity with a hand-written proxy
  host: `robotsNoIndex`, `timeouts` and `tags`.** `IngressHostTemplate` carried
  the upstream, TLS, websockets, middlewares, access lists and default DNS policy
  and nothing else, so a service cut over to Ingress discovery **silently lost**
  its `robotsNoIndex` - the derived host had no way to express a field the model
  already has. The workaround, a `headers` middleware setting `X-Robots-Tag`, is
  a second mechanism for the same thing and has to be remembered per host; it was
  reverted in production, and `robotsNoIndex` was simply off for derived hosts
  until this.

  All three are applied verbatim to every derived host, and because a profile
  *is* an `IngressHostTemplate`, profiles get them with no extra plumbing (tested,
  not assumed). `timeouts` is validated by the **same helper** `ProxyHost.Validate`
  uses, at settings-write time - a template that would produce a host the config
  validator rejects fails the operator's own write instead of every tenant's
  reconcile batch. `timeouts` is a pointer and `tags` a slice, so a template that
  sets neither still produces exactly the object it did before (no `timeouts: {}`
  in the YAML). Both, like `middlewares`/`accessLists`/`defaultDNS`/
  `tls.clientAuth`, are **deep-copied** per host: no derived host shares backing
  memory with another or with the loaded settings.

  `locations` is **deliberately not** a template field, recorded as a decision in
  `docs/design/ingress-discovery.md` section 5 and `BACKLOG.md` rather than left as a
  silent omission: locations are per-service path routing, and discovery forwards
  everything to the cluster ingress controller by vhost so the controller can do
  that routing from the same `Ingress`. `displayName` stays derived as
  `<namespace>/<name>`. The Settings UI renders all three new fields on the
  default template and on every profile row, and the save path **merges** the
  nested `timeouts` object over what was loaded rather than rebuilding it - the
  guard test in `internal/ui/settingsmerge_test.go` now covers the new fields, so
  the rebuild-instead-of-merge regression (which has already stripped
  `clientAuth`/`hsts`/`minTLSVersion` once) fails CI.

## [1.0.9] - 2026-07-31

### Added

- **`GET /api/dns-sync/plan` - dry run before you enable.** Reads both backends and
  the ownership ledger and reports exactly what a reconcile would create, adopt,
  retarget, delete and skip, plus how many records it would leave alone, without
  issuing a single write. Scope `dns-sync:read`; `409 Conflict` while a reconcile
  is in flight, for the same reason the manual reconcile refuses to queue. Wired
  into the settings UI as **Preview changes**, next to *Reconcile now*. The
  2026-08-01 incident was unpreviewable - the only way to learn what the first
  reconcile would do was to run it, and by then the records were gone.
- **Ingress discovery: operator-defined named profiles, selected per Ingress.**
  `settings.ingressDiscovery.profiles` is a map of named chains, each with the
  same shape and the same validation as `template`; an `Ingress` selects one with
  `gpm.rake.pro/profile: "<name>"`. Discovery previously derived every host from
  the single `template`, so it could only adopt the uniform tail of a fleet -
  publishing anything else would silently **drop** its `sso`/`rate-limit`/login
  middleware or **impose** an access list on a host that is public on purpose,
  both security-relevant regressions that leave the host serving under a chain
  nobody chose. `template` is unchanged and now acts as the default profile, so
  existing configs and existing annotated Ingresses behave identically (covered
  by a regression test).

  **The annotation carries a name and nothing else, and that is the security
  constraint.** An Ingress author is untrusted - in a shared cluster a tenant may
  be able to create or edit an `Ingress`, and gpm sits at the edge in front of
  everything - so there is deliberately **no** annotation that lets a manifest
  name a middleware, an access list, a certificate or an upstream. Such an
  annotation would be a self-service privilege grant (`access-lists: ""` on your
  own namespace's Ingress removes `home-vpn` from a hostname at the edge). Every
  profile is authored by the operator in the config repo; a manifest chooses
  among sanctioned chains and can never invent one, nor end up weaker than
  something the operator explicitly allowed. An **undefined** profile name
  **skips** the Ingress - never a silent fall back to the default, never a
  partial chain - and if that Ingress already has a derived host, the host is
  **disabled** rather than left alone, so a retired or tightened profile cannot
  be pinned (see Security below). Matching is
  exact (no prefix match, no case folding); a profile is applied verbatim rather
  than merged with the default, so the default's access list cannot leak onto a
  deliberately-public profile. Every profile validates at settings-**write** time
  (`certificateRef` required, `upstream` XOR `upstreamGroupRef`, name-checked
  middleware/access-list refs), so an invalid one is rejected where an operator
  sees it rather than surfacing later as a skipped host.
  `GET /ingress-discovery/status` now reports the resolved `profile` per host
  (the literal `template` for the default block) as the audit trail for what
  chain a given Ingress actually got, and the settings UI grows a profile editor
  plus the resolved profile in its status panel. Threat model and resolution
  rules in
  [docs/design/ingress-discovery.md section 5a](docs/design/ingress-discovery.md);
  schema and a worked example in
  [docs/configuration.md](docs/reference/config/settings/ingress-discovery.md#discovery-profiles).

### Changed

- **The per-host "LAN direct" / "Public CNAME" toggles stay usable when their DNS
  backend is not configured.** They were greyed out via `gateControl`; they now
  render an inline note instead ("Pi-hole DNS sync is not configured yet - this
  will take effect when it is"). Setting the flag ahead of the backend is
  legitimate staging - the host is the declaration, the syncer publishes when it
  is wired - not an error to refuse, and the capability probe is cached per page
  load, so a stale "not configured" could otherwise outlive the fact and block a
  valid edit. Every other capability-gated control still greys out.
- **`GET /api/dns-sync/status` reports adoption.** Each backend now carries
  `adopted`, `retargeted`, `skipped` and `untouched` alongside `created` and
  `deleted`. `managed` changes meaning from "records whose target matched the
  apex" to "records gpm owns" (the ledger entry count after the run). `untouched`
  is the number to check after a first enable: it should equal everything you
  maintain by hand on that backend.
- **Changing `apexTarget` no longer orphans records.** Previously ownership *was*
  target equality, so moving the apex made every record gpm had created
  unrecognisable to it - never updated, never deleted, and never recreated either
  because the name now conflicted. With the ledger, a record gpm created and
  nobody has touched since is still identifiably gpm's, so it is retargeted on the
  next reconcile. The manual-cleanup warning in `docs/configuration.md` and
  `docs/deployment.md` is retired.

### Fixed

- **A record gpm ADOPTED is never deleted - it is released.** Adoption used to be
  a one-way trap. An operator hand-writes `x.example.com`; a proxy host is later
  given `dns.lanDirect` for that name, so the reconcile adopts the existing record
  into the ownership ledger; the operator later takes the flag off again - and the
  next reconcile **deleted their record**, because the ledger no longer
  distinguished a record gpm made from one it had merely claimed. On Pi-hole,
  where every correctly-targeted record is adoptable, that was the 2026-08-01
  incident deferred by one config edit.

  **The guarantee now: an adopted DNS record is never deleted *or* retargeted -
  gpm destroys only records it CREATED.** Ledger entries
  record their provenance (`adopted: true|false`), and an adopted entry the config
  no longer wants is *released* - dropped from the ledger with a warning, with the
  record left exactly where it stands. Deletion remains available for records gpm
  created itself. Existing ledgers upgrade safely: an entry written before the
  field existed carries no provenance, and a missing `adopted` is read as
  **adopted**, the only reading that cannot destroy a record on upgrade. The
  trade-off is stated in `docs/configuration.md`: gpm will not clean up an adopted
  record for you.
- **An `apexTarget` change no longer deletes the records gpm only ADOPTED.** A
  retarget is a delete followed by a create, and the retarget branch did not look
  at the claim's provenance: after the edge host moved, a record an operator had
  hand-written and gpm had merely adopted was **destroyed and recreated**, and the
  replacement was recorded as `adopted: false`. Two failures in one - an
  operator-authored record deleted by the subsystem that exists to prevent exactly
  that, and a claim silently upgraded from adopted to created, so a *later* host
  removal would hard-delete the name for good. Adopt -> change `apexTarget` ->
  remove the host reproduced the 2026-08-01 incident one record at a time.

  An adopted claim whose record no longer matches the apex is now **released**:
  dropped from the ledger with a warning and counted as a skip, with nothing
  written to the backend. Retarget stays available for records gpm created itself,
  which it may safely replace. Provenance is carried forward unchanged everywhere
  else; the single place a claim may become "created" is a record that has gone
  and is genuinely re-created by gpm. To move an adopted name to a new apex,
  delete or re-point the record by hand (`docs/configuration.md`).
- **Pi-hole sessions leaked whenever a run was cut short.** `logout` ran on the
  caller's context, so an HTTP client disconnecting mid-reconcile cancelled the
  logout along with the run (measurably: one login, zero logouts). Pi-hole has a
  small fixed session pool, so a leak per aborted run eventually locks the
  operator out of their own admin UI. Logout now runs on a detached context with a
  5s deadline.
- **A failed retarget no longer destroys the record.** Neither backend can update
  a CNAME in place, so a retarget is delete-then-create; a create that failed left
  the name unresolved until some later reconcile happened to heal it, while the
  status reported `Deleted:0 Retargeted:0` - a run that destroyed something and
  said nothing had happened. The original record is now restored (Cloudflare
  included, orange-cloud flag and all), the run fails loudly, and the counter is
  incremented as soon as the *delete* lands.
- **A Pi-hole API shape change is an error, not a silent ledger wipe.** A renamed
  or missing `config.dns.cnameRecords` field decoded to a nil slice, which a
  full-state reconciler reads as "the resolver holds nothing" - status OK, zero
  counters, ledger emptied and committed. The field is now required to be present
  and a list; anything else fails the run with the ledger intact.
- **Cloudflare pagination no longer truncates when `result_info` is absent.** A
  full 100-record page with no (or a zeroed) `result_info` stopped the walk at
  page 1, hiding the rest of the zone: no false deletes, but orphaned ledger
  entries and repeated creates of records that already exist. Termination is now
  driven by a short page; `result_info` is advisory.
- **A reconcile can no longer clobber a concurrent revert of the ownership
  ledger.** The reconcile's read-modify-write spans minutes of backend I/O, and
  `Revert` rewrites `dns-ledger.yaml` with the rest of the tree; a reconcile that
  had loaded the ledger first would write its own version back afterwards,
  re-establishing claims the revert withdrew - and a claim authorises a deletion.
  `SaveDNSLedger` now takes the repo revision the ledger was read at and refuses a
  stale write (`ErrLedgerStale`); the reconciler re-reads and rewrites *without*
  the withdrawn claims rather than resurrecting them.
- **A revert can still restore an ownership claim reality has moved past** (gpm
  created a record, deleted it, an operator recreated it by hand, the config is
  reverted to before the deletion). This is documented prominently beside the
  existing revert note in `docs/configuration.md`, and every deletion is now
  logged at **warn** with the ledger revision that authorised it, so a record
  removed on the strength of a stale claim is identifiable after the fact.
- **Ledger duplicate-domain validation is case-insensitive**, matching the
  normalised form the reconciler indexes by. `Foo.lan` and `foo.lan` could both
  validate, leaving one claim silently shadowing the other.
- **The ledger commit survives a cancelled request.** The reconciler's ledger save
  passed the (possibly request-scoped) context to `git.CommitAll`, so a cancelled
  reconcile could leave the file written but uncommitted, to be swept into an
  unrelated commit later. It now writes on a detached context.
- **DNS sync deleted 19 records it did not create. It can no longer delete
  anything it did not create.** On 2026-08-01 an operator enabled
  `settings.dnsSync.pihole` for the first time. Their Pi-hole already held 19
  hand-written LAN CNAMEs (their LAN-direct bypass list: `plex`, `argo`, `cloud`,
  `wiki`, ...) pointing at the same edge host they had just configured as
  `apexTarget`. No proxy host carried `dns.lanDirect` yet, so the desired set was
  empty. The Pi-hole backend treated *any* CNAME whose target equalled
  `apexTarget` as gpm-managed, found all 19 unwanted, and deleted every one on the
  very first reconcile. LAN DNS broke until they were restored by hand.

  The bug was the ownership test. Pi-hole/dnsmasq CNAMEs have no comment field, so
  target equality was used as a stand-in for "gpm created this" - and on a shared
  apex that is not ownership at all, it is a coincidence. (The Cloudflare backend
  was safe in the identical situation only because its marker, the
  `managed-by:gpm` record comment, is one pre-existing records do not carry.)

  **The guarantee now: gpm deletes only DNS records it recorded creating.**
  Ownership is written down, not inferred, in a new git-backed ledger
  (`model.DNSLedger`, the singleton `config/dns-ledger.yaml`). A record absent from
  that ledger is never in a delete list, whatever its name and whatever it points
  at. `apexTarget` says where managed records point and nothing more.

  What a reconcile does per desired name: **create** what is absent (recording
  ownership); **adopt** a record that already holds the right target but predates
  the ledger - claimed, not recreated, logged at info; **retarget** a record that
  still holds exactly what gpm wrote after `apexTarget` moved; **skip and warn** on
  a name held by a record gpm does not own (unchanged - never shadowed, never
  replaced). It **deletes** only ledger entries the config no longer wants, and
  only while the record still matches what gpm left there; re-pointed out of band,
  it is disowned rather than deleted.

  **Migration for existing deployments is a no-op plus adoptions.** An empty ledger
  means gpm owns nothing, so a first reconcile can only create and adopt: records
  matching the desired set are adopted, every other record on the backend is left
  exactly as it is, and the run logs a one-line summary of both counts. Nothing has
  to be done before upgrading, and no ledger file has to be seeded.

  Cloudflare is held to the same discipline: the ledger is authoritative for
  deletion and the `managed-by:gpm` comment remains an independent second
  condition (adoption there requires the comment too), so none of that backend's
  existing guarantees are weakened.

  Regression coverage names the incident directly: pre-existing records pointing
  at `apexTarget`, an empty desired set and an empty ledger must produce **zero**
  deletions, on both backends.
- **One dangling reference in `ingressDiscovery` could wedge the entire
  reconcile, forever.** `Settings.Validate` checks only the *shape* of the names
  a template or profile carries, so a `certificateRef` / `upstreamGroupRef` /
  middleware / access list / `clientAuth.caRef` naming an object that does not
  exist passed `SaveSettings` - and then failed at `merged.Validate()` in
  `ApplyBatch`, which rejects the **whole batch**. Every other tenant's create,
  update and delete was therefore dropped on every poll, indefinitely, surfacing
  only as an opaque batch-validation error in the reconcile status.
  `SaveSettings` now cross-checks the template and every profile against the
  loaded config (`IngressDiscoverySettings.ValidateRefs`), so the error lands on
  the operator's own write with the offending name in it. A **disabled**
  discovery block is not cross-checked, so a half-filled draft still never blocks
  an unrelated settings write.
- **The residual risk of profiles is now documented where operators read it.**
  `docs/configuration.md` and the settings UI both claimed a manifest "can never
  produce a host weaker than something you explicitly sanctioned" while the
  worked example shipped a `public-ratelimited` profile with no access list. Both
  now say the operative rule plainly: every profile is selectable by every
  annotating Ingress, so define only profiles you are willing for any cluster
  tenant to choose.

### Security

- **Ingress discovery: revoking a profile now actually revokes it.** An annotated
  Ingress whose profile does not resolve used to be treated like any other derive
  failure - protected from deletion and otherwise untouched - which meant the
  security property held for *creating* a host but not for *revoking* one. An
  operator who tightened a profile (added `sso`, added an access list), renamed
  it, or retired it could be defeated by a tenant flipping the annotation to a
  name that does not exist: no upsert, no delete, and the live host kept serving
  the pre-tightening, unauthenticated chain indefinitely. Deleting every profile
  row in the settings UI froze every derived host the same way. Discovery now
  plans an update setting `disabled: true` on the existing managed host instead -
  fails closed, destroys nothing, and re-adding the profile re-enables it on the
  next reconcile. Every **other** derive failure (malformed hostname, unusable
  derived name) still freezes the existing host: the operator's policy has not
  changed there, and failing closed would let any tenant take their own service
  offline with a one-character manifest edit.
- **A settings save no longer strips mTLS from discovery profiles or the
  template.** The settings form rebuilt `tls` from exactly the three fields it
  renders (`certificateRef`, `forceSSL`, `http2`), and a settings write is a full
  replacement - so a template or profile authored in YAML/GitOps with
  `tls.clientAuth: {caRef: ..., mode: require}` silently lost its
  client-certificate requirement (and its `minTLSVersion` and `hsts`) on **any**
  unrelated Settings save, after which the next reconcile pushed the downgrade
  onto every derived host. The handler now merges over the loaded object. The
  template block had this bug since it shipped; profiles inherited it. A guard
  test in `internal/ui` fails if the merge is ever rebuilt away.
- **The data plane fails closed on an unresolvable middleware or access-list
  name.** `buildChain` skipped any name it could not resolve, so a typo in a
  host's `accessLists` turned a restricted host into an open one, with
  `Config.Validate`'s reference check the single thing standing between the two.
  An unresolvable reference now replaces that host's chain with a `503` and logs
  at `error`. Scoped to the one host on purpose: a config that cannot pass
  validation anyway must not take unrelated hosts down as a side effect.
- **Derived hosts no longer share one `*ClientAuth` with the settings object.**
  `TLSSettings` is a value but its `ClientAuth` is a pointer, so every host
  derived from a template aliased the same mTLS struct (middlewares, access lists
  and the DNS policy were already copied). Not exploitable through any current
  path, but a mutation of one host's mTLS requirement could have reached every
  other host's. Now deep-copied.

## [1.0.8] - 2026-07-31

### Added

- **Ingress discovery templates can name an upstream group.**
  `settings.ingressDiscovery.template.upstreamGroupRef` is an alternative to a
  single `upstream` address, mutually exclusive with it exactly as on a proxy
  host. A cluster ingress controller normally runs on every node, so pinning
  discovery to one address made every discovered service single-node while the
  operator's hand-written hosts kept failing over - a silent availability
  downgrade for anything discovery adopted.

## [1.0.7] - 2026-07-31

### Fixed

- **The API-token form could not grant `ingress-discovery`.** The SPA rendered
  its scope checkboxes from a hand-maintained copy of `model.ScopePlurals`,
  which went stale the moment Ingress discovery added a subject - so the server
  accepted `ingress-discovery:read` while the UI offered no way to ask for it.
  `GET /api/capabilities` now serves `scopeSubjects` (the authoritative list)
  and the form renders from it, with the local list demoted to a
  fetch-failure fallback. Covered by a test asserting the served list matches
  `model.ScopePlurals` exactly.

## [1.0.5] - 2026-07-31

### Added

- **Kubernetes Ingress discovery (DNS sync phase 2).** A new `internal/k8s`
  subsystem reconciles annotated cluster `Ingress` objects into gpm-managed proxy
  hosts, which then feed the phase-1 DNS reconciler - so a cluster service no
  longer has to be hand-entered as a proxy host before its DNS follows. Opt-in is
  per Ingress and absolute: `gpm.rake.pro/managed: "true"`, with
  `gpm.rake.pro/lan-direct` / `gpm.rake.pro/public-cname` setting the derived
  host's `dns` policy; anything else (including an absent annotation) is invisible,
  and there is no namespace-sweep mode. Configured under
  `settings.ingressDiscovery`, with an operator-supplied **template** that is the
  only source for the upstream, certificate ref, middleware and access-list chain
   -  an Ingress contributes strictly-validated, suffix-restricted hostnames and two
  booleans, nothing else. Because gpm runs off-cluster, in-cluster Service DNS is
  unusable: the template upstream is the **cluster ingress controller's** address,
  and the data plane's preserved `Host` header is what routes the request to the
  right workload. The client is plain `net/http` + `encoding/json` against
  `/apis/networking.k8s.io/v1` (no `client-go` - its transitive tree would dwarf
  the whole direct dependency set), works in-cluster *or* with explicit
  `apiURL`/`tokenFile`/`caFile`, re-reads the bearer token from disk on a TTL (and
  drops it on a `401`) so a rotated projected ServiceAccount token keeps working,
  and is hardened like `internal/dnssync`: CA-verified TLS with no skip-verify,
  redirects never followed, link-local destinations refused at connect time,
  bounded reads and bounded pagination. Reconcile is **full-state** and
  **ownership-gated**: only proxy hosts labelled
  `gpm.rake.pro/managed-by: ingress-discovery` are created, updated or deleted, and
  a name collision with a hand-written host is skipped with a warning. It
  **freezes on error** - a managed host is deleted only after a complete,
  successful, fully-paginated list, and any transport/status/decode/pagination
  failure aborts before any write (an empty *successful* list is a different
  return shape entirely, and is a legitimate delete-all). A whole reconcile lands
  as **one commit**; a no-drift run writes nothing. New endpoints
  `GET /api/ingress-discovery/status` (`ingress-discovery:read`) and
  `POST /api/ingress-discovery/reconcile` (`ingress-discovery:write`, **409** while
  a run is in flight), a new `ingressDiscovery.enabled` capability, a settings
  block + status panel in the web UI, and cluster-side RBAC at
  [`deploy/k8s-ingress-discovery-rbac.yaml`](deploy/k8s-ingress-discovery-rbac.yaml).
  Design record: [docs/design/ingress-discovery.md](docs/design/ingress-discovery.md).
- **`Store.ApplyBatch`.** Commits many upserts *and* deletes as a single
  revision, validating the merged graph once (including the dangling-reference
  check `Delete` performs) before touching the working tree. It is what makes
  "one commit per reconcile" possible for Ingress discovery; an empty batch is a
  no-op that commits nothing.
- **Scoped API tokens.** A new `APIToken` config object
  (`config/api-tokens/<name>.yaml`) gives scripts and CI a bearer credential
  instead of an admin session cookie. The secret (`gpm_` + 32 random bytes,
  base64url) is generated **server-side** and returned exactly once, in the
  response to the `PUT` that created it (or `?rotate=1`); only its SHA-256 digest
  is committed, in a plain string field, so the git config never holds a usable
  credential. Presented as `Authorization: Bearer gpm_...`, it is resolved before
  the cookie path and never falls through to it, by constant-time digest compare
  across the enabled, unexpired tokens. The token set is cached in the
  authenticator and invalidated from the daemon's single config-reload path, so
  revoking, disabling or expiring a token still takes effect on the next request
  without an unauthenticated bearer attempt being able to force a full config load
  per request. Scopes are `<plural>:read` / `<plural>:write` / `*:read` /
  `*:write` / `admin` (write implies read; `admin` is required for token
  management, `PUT /settings`, `GET /backup`, `POST /restore`, the whole-config
  `POST /revert` and `/debug/pprof/*`). Sessions are unaffected: scopes constrain
  API tokens only. The stored digest is never returned by any endpoint, and
  reverting an `APIToken` is refused so a rotation always means revocation.
  `GET /api/api-tokens` surfaces an in-memory `lastUsed` per token
  (deliberately not persisted - the store is git-backed). New page in the web UI
  with a scope picker, one-time reveal, rotate and delete
  (`internal/model/apitoken.go`, `internal/auth/apitoken.go`, `internal/api`).
- **DNS sync (Pi-hole + Cloudflare).** A new `internal/dnssync` subsystem
  publishes CNAME records for the proxy hosts that opt in via a per-host
  `dns` policy (`lanDirect` for the LAN resolver, `publicCname` for the public
  zone), configured once under `settings.dnsSync`. Reconcile is **full-state**,
  not diff-based: the desired set is recomputed from the whole config on every run
  and compared with what each backend actually holds, so out-of-band drift is
  repaired in both directions. Deletion is **ownership-gated** - on Pi-hole only a
  CNAME whose target is exactly the configured `apexTarget`, on Cloudflare only a
  record carrying the `managed-by:gpm` comment (re-checked inside the delete call
  itself) - so a record gpm did not create is read and ignored, never removed.
  Creation is gated the same way: a desired name that already exists as somebody
  else's record is logged and skipped rather than shadowed. Reconciles fire
  automatically after proxy-host/settings writes and whole-tree restore/revert
  (non-blocking, bursts coalesced into a single follow-up run) and on demand via
  `POST /api/dns-sync/reconcile`, which answers `409 Conflict` rather than
  queueing while a run is in flight; `GET /api/dns-sync/status` reports
  the last run per backend. The Pi-hole v6 client opens and explicitly releases its
  session (Pi-hole has a small session pool) and reports a `403` as the distinct
  "read-only session / `app_sudo` disabled" condition. The Cloudflare client is
  separate from the ACME solver on purpose, so record lifecycle and certificate
  issuance cannot break each other. Both use the webhook dispatcher's hardened
  HTTP client (no redirects, link-local refused at connect time).
- **`GET /api/capabilities` gains `apiTokens` and `dnsSync` groups**, so the SPA
  greys out the per-host DNS toggles whose backend is not configured, and the
  "API Tokens" nav entry and page when no token source is wired, instead of
  accepting input that could not work (`internal/api`, `internal/ui`).

### Changed

- **One Ingress reconcile's cluster list is bounded to two minutes.** The only
  bound was the 30s per-request timeout times the 100-page limit - roughly 50
  minutes holding the reconcile mutex, during which every poll and every manual
  reconcile queues behind it. The list now runs under a `context.WithTimeout`
  (`internal/k8s`).
- **`GET /api/capabilities` no longer loads and validates the whole config to
  answer whether Ingress discovery is enabled.** The probe runs on every admin page
  load and took the store read lock behind in-flight reconcile commits. The
  `Discoverer` now caches the flag, refreshing it on every reconcile and every poll
  interval read, so the answer lags a settings change by at most one poll
  (`internal/k8s`).
- **Manual DNS reconciles no longer queue.** `POST /api/dns-sync/reconcile` used
  a blocking lock acquire that ignored the request context, so repeated calls
  piled goroutines (each holding a request-scoped context) up behind a slow
  backend. It now uses `Syncer.ReconcileNow`, which refuses with
  `ErrReconcileInProgress` -> **409 Conflict**. The event-triggered path still
  waits, which is what makes trigger coalescing correct (`internal/dnssync`,
  `internal/api`).
- **`ProxyHost.dns` is now omitted from JSON when unset.** `encoding/json`
  ignores `omitempty` on a struct value, so every proxy-host response carried a
  noise `"dns":{}`; the field is a pointer now (`internal/model`).

### Fixed

- **`Store.ApplyBatch` is now actually atomic, as its doc comment claimed.** YAML
  was written and files removed *before* `CommitAll`, so a failing `os.Remove` or a
  failing/cancelled commit left the working tree mutated but uncommitted: the next
  `Load` served the deletions as live config while the status reported failure, and
  the following unrelated write swept the orphans into its commit. Shutdown made it
  reachable - the discovery goroutine was fire-and-forget, so `main` could cancel the
  context mid-commit and return. Every file a batch touches is now snapshotted first
  and the tree is rolled back on any failure (the `snapshotDirs`/`writeDirs` precedent
  from `RevertObject`), and the reconciler runs under a `sync.WaitGroup` that
  shutdown waits on (`internal/store`, `cmd/gpm`).
- **Ingress discovery no longer fires a reload, webhook and DNS trigger on an
  empty commit.** When every planned delete turned out to be already gone,
  `ApplyBatch` returned `("", nil)` and `onChange("")` ran anyway - reloading,
  dispatching a lifecycle webhook and triggering DNS with an empty
  `status.commit` while `deleted` was non-zero. Notification now requires a real
  commit (`internal/k8s`).
- **An oversized Kubernetes response is reported as such, not as a decode
  failure.** `io.ReadAll(io.LimitReader(...))` truncated silently at the 8MiB body
  cap, surfacing as a JSON syntax error and a permanent freeze - triggerable by a
  tenant putting large annotations on an annotated `Ingress`. The read now goes one
  byte past the cap to detect the overflow and says so, and `listPageSize` drops
  from 500 to 100 so a page of realistically-sized objects stays well clear of the
  cap (`internal/k8s`).
- **A namespace containing a dot is refused rather than assumed impossible.**
  `derive()` builds `ing-<name>.<namespace>` and relied on the API server having
  enforced that a namespace is a DNS-1123 label; the derived name is an ownership
  boundary, so gpm now validates it itself and skips the Ingress (returning no name,
  so an ambiguous one cannot protect a host from deletion) (`internal/k8s`,
  `internal/model`).

### Security

- **Ingress discovery: ownership now gates the DOMAIN, not just the derived
  name.** A derived host was skipped only when its *name* collided with a host gpm
  did not own, but the data plane routes by hostname and fills its per-domain maps
  in config load order - so a cluster tenant who could annotate an `Ingress` in
  their own namespace could claim `sso.example.com`, derive a name colliding with
  nothing, sort after the operator's host, and replace that host's SSO/access-list
  chain with the template's (and overwrite its mTLS pinning). `allowedDomainSuffixes`
  was no defence: exact-suffix matching makes even the apex claimable. `planReconcile`
  now refuses any derived host whose domains are already claimed by a host discovery
  does not own - proxy, redirect or dead, enabled or disabled - reporting it per host
  in `status.hosts[].reason`, and resolves two Ingresses claiming one hostname
  first-by-derived-name-wins. Backstopped in `Config.Validate`, which now rejects any
  two **enabled** hosts claiming the same domain whatever wrote them (disabled hosts
  are exempt so staging a replacement beside the live host still works)
  (`internal/k8s`, `internal/model`).
- **Ingress discovery: a `200` that is not an `IngressList` no longer reads as an
  empty cluster.** The LIST decode asserted nothing about the response shape, so
  `null`, `{}`, a `kind: Status` reply and `{"items":null}` all decoded to zero
  items, no continue token and a nil error - which the reconciler treats as a
  complete list and answers by **deleting every managed host**. It needed no
  compromised API server: an `apiURL` typo'd onto another HTTPS service behind the
  same internal CA, a mesh/gateway `200` envelope, or a namespace/label-selector
  typo would do it. The page struct now asserts `kind: IngressList` and a present
  `items` field, and anything else is an error on the freeze path (`internal/k8s`).
- **`Store.ApplyBatch` re-checks ownership under the store lock.** It trusted the
  caller's ownership filter and never inspected labels, while the caller's plan was
  computed from a snapshot taken *before* a multi-second cluster list - so an object
  created, deleted or relabelled inside that window was written or removed on the
  strength of a check against state that no longer existed. `ApplyBatch` takes an
  `ApplyGuard` it evaluates against freshly loaded state, under the lock, for every
  delete target and every pre-existing upsert target; a guarded-away object is
  skipped, not clobbered, and a batch left with nothing to do commits nothing. The
  Ingress reconciler installs a managed-by-label guard (`internal/store`,
  `cmd/gpm`).
- **Shipped Ingress RBAC narrowed to `list`, and the token-extraction recipe no
  longer leaves a world-readable window.** The `ClusterRole` granted `get`/`list`/
  `watch` while the client only ever lists. The manifest's `ca.crt` jsonpath
  (`{.data\.ca\.crt}`) also escaped the `data` separator, matched nothing, and
  silently wrote an **empty CA file**; and both files were created by a plain shell
  redirect (mode `0644`) and only `chmod 0400`'d afterwards, leaving the bearer
  token readable by any local user in between. The manifest and
  `docs/deployment.md` now use `install -m 0400 /dev/null` first, the correct
  jsonpath, and document that a namespaced `Role`/`RoleBinding` is tighter when
  `settings.ingressDiscovery.namespace` is set
  (`deploy/k8s-ingress-discovery-rbac.yaml`, `docs/deployment.md`).
- **`/debug/pprof/*` now requires the `admin` scope, not just the admin role.**
  Every API-token principal is admin-*role* by construction (the coarse gate is
  satisfied and the real authorization is the per-route scope check), so a
  `proxy-hosts:read` token could fetch the heap profile and `cmdline` - which
  carry resolved Cloudflare/Pi-hole credentials in cleartext. The profiling
  endpoints now run an admin-scope check inside the role gate. Admin *sessions*
  are unchanged (`internal/server`).
- **`PUT /api/settings` now requires the `admin` scope.** `settings:write` was
  admin-equivalent in practice: it could point `dnsSync.pihole.url` or a webhook
  at an attacker-controlled URL with `${ENV:SOME_TOKEN}` as the credential, and
  the settings write itself triggers the reconcile/dispatch that resolves and
  sends that env var offsite - and it could rewrite `adminAuth` outright.
  `GET /api/settings` stays on `settings:read` (reading resolves nothing). The
  UI greys out the `settings` write box rather than offering a grant that no
  longer does anything (`internal/api`, `internal/ui`).
- **`GET /api/backup` now requires the `admin` scope** (was `*:read`). The
  archive is the raw on-disk YAML, so unlike the JSON reads it carries the
  api-tokens' stored digests - and it is the exact input `POST /api/restore`
  takes, which was already admin-scoped (`internal/api`).
- **`APIToken.tokenHash` is no longer returned by any endpoint** (`json:"-"`,
  yaml tag unchanged so at-rest persistence is identical). It was previously
  readable through `GET /api-tokens`, `GET /config` and `GET /backup` on a
  read-only scope; a SHA-256 digest is offline-crackable, so handing it to a
  read-only caller let them grind for the secret at leisure (`internal/model`).
- **Rotation now means revocation: reverting an `APIToken` is refused.**
  `POST /api/api-tokens/{name}/revert` restored an older token file - and with
  it an older `tokenHash` - silently reviving a secret the operator had rotated
  away, while the UI promised "the current secret stops working immediately".
  `Store.RevertObject` refuses the kind with `ErrNotRevertible`, and the
  whole-config `POST /api/revert` snapshots the `api-tokens` directory before the
  tree restore and writes it back over the result, so it neither revives a
  rotated digest nor resurrects a deleted token. Everything else still reverts
  normally, and the UI copy now says so (`internal/store`, `internal/ui`).
- **Unauthenticated bearer attempts no longer force a config load.** Resolving a
  presented token read the whole git-backed config (directory walk, YAML parse,
  whole-graph validation) on every request, and a failed bearer auth never
  reaches the login rate gate - an unthrottled DoS lever for anything
  internet-facing. The authenticator now caches the token set and drops it from
  the daemon's single reload path, so token edits still take effect immediately
  (`internal/auth`, `cmd/gpm`).
- **The API scope gate fails closed on a missing principal.** The daemon's
  `RequireScope` returned nil (allow) when no principal was on the request. That
  is unreachable through the mounted route, which is exactly why it must deny: a
  broken authorization wiring should refuse, not wave requests through. A nil
  `Deps.RequireScope` still means "allow" and is documented as the unwired/test
  case only (`cmd/gpm`, `internal/api`).
- **Pi-hole DNS sync no longer overwrites an operator-owned CNAME.** The
  reconciler compared only against records it already owned, so a name an
  operator had deliberately pointed elsewhere got a second, shadowing entry. It
  now builds a present-set from every returned record and skips-and-warns on a
  name that exists with a different target - the behaviour the Cloudflare backend
  already had (`internal/dnssync`).

## [1.0.4] - 2026-07-20

### Added

- **Web UI: middleware editor supports `rewrite`.** The middleware-type
  dropdown now offers `rewrite` with a `replacePath` key/value row editor
  (parity with the `headers` editor), so an existing `rewrite` middleware can
  be opened and saved without being coerced into another type
  (`internal/ui/static/app.js`).

### Security

- **Aikido SAST suppression documented at the git exec site.** The
  command-injection finding on `internal/store/gitrepo.go` is a false
  positive - git runs as a fixed argv (no shell) with internal literal
  arguments - now marked with an inline `noaikido` justification instead of
  recurring in every scan.
- **Admin CSP tightened from frame-ancestors-only to a strict policy**
  (defense-in-depth). Every admin/login response now carries
  `script-src 'self'` (plus `default-src 'self'`, `connect-src 'self'`,
  `object-src 'none'`, `base-uri 'none'`, `form-action 'self'`), so injected
  markup could not execute script even if the SPA's output escaping ever
  regressed. Carve-outs: `style-src 'unsafe-inline'` for the SPA's inline
  style attributes, and Google Fonts (`style-src` fonts.googleapis.com,
  `font-src` fonts.gstatic.com). Admin listener only - proxied data-plane
  traffic is untouched (`internal/server`).

## [1.0.1] - 2026-07-19

### Added

- **`rewrite` middleware: exact-match request-path replacement.** A new
  middleware type (`type: rewrite`) with a `replacePath` map that swaps an
  incoming request path for a target before proxying, when the path matches a key
  **exactly** (no regex/prefix matching, sidestepping the path-confusion/ReDoS
  classes). The rewrite is internal and upstream-facing: it preserves the method
  and body (never an HTTP redirect) and runs **innermost** in the chain (closest
  to the upstream, after headers), so auth/guard/access-list all still evaluate
  the ORIGINAL client path. Motivating case: repair a client that mangles an
  upstream path - e.g. a mobile OIDC app that strips the trailing slash off
  Authentik's `/application/o/token`, which Django answers `405`; a
  `/application/o/token -> /application/o/token/` rewrite at the edge fixes it.
  Both key and value must be absolute paths and a key may not map to itself
  (`internal/dataplane/rewrite.go`, model `RewriteMiddleware`).

## [1.0.0] - 2026-07-18

### Added

- **HA design doc** ([docs/design/ha.md](docs/design/ha.md)): a settled
  multi-instance story for gpm itself. Recommends a phase-1 active/standby
  two-node homelab pair - static leader owns ACME renewal and admin writes, the
  follower pulls config via git `pull --ff-only` and reads replicated
  certs/secrets, a keepalived VRRP VIP steers traffic (client IP preserved), the
  `sso_not_before` watermark gains a refresh loop so revocation propagates
  without a restart, and TCP/UDP streams are failover-with-reconnect. Per-instance
  state (admin sessions, access-log ring, rate-limit buckets, login-lockout maps)
  is documented as lossy. No new dependency; no data-plane hot-path change.
- **Per-object revert.** A scoped revert restores only one object's file to its
  state at a past commit, committing just that change and leaving every other
  object untouched. New `Store.RevertObject(kind, name, hash)` uses
  `git checkout <hash> -- <rel>` (path always after `--`); the rel path is derived
  from the trusted object-kind directory mapping (never client-supplied), the hash
  is validated, and the whole config is re-validated with a rollback to HEAD on
  failure exactly like the whole-tree revert. New endpoint
  `POST /api/{kind}/{name}/revert` (body `{"hash":"<commit>"}`), same
  auth/CSRF/reload/webhook wiring as the whole-config revert. The History view now
  offers "revert this object" per single-object commit, and the existing
  whole-tree action is relabeled "revert entire config" to make its scope
  unmistakable. This closes the gap where reverting one object from its History
  view silently deleted every object created after that commit (operator incident
  2026-07-16: a proxy-host revert wiped three newer Certificate objects).
- **Upstream groups: health-checked failover across multiple backends.** New
  first-class `UpstreamGroup` object (`config/upstream-groups/`): an ordered
  list of upstreams (first = primary, rest = backups) with a per-group health
  check (TCP connect by default, or HTTP GET via `healthCheck.path`; tunable
  `intervalSeconds`/`timeoutSeconds`/`rise`/`fall`). A ProxyHost selects it via
  `upstreamGroupRef` (mutually exclusive with `upstream`; locations inherit the
  group unless they set their own single `upstream`). The data plane tries
  candidates healthy-first in config order and retries a request on the next
  upstream **only on connect-phase failures** (dial/TLS - the request was never
  sent, so retrying is safe for non-idempotent methods; an HTTP response,
  including a 5xx, is never replayed). Live-traffic connect failures feed the
  same fall counter as the active probe, an immediate probe round runs at
  (re)load, and a group with every upstream down fails open (attempts in
  preference order). Request bodies up to 1 MiB are buffered for replay; larger
  bodies stream with a single attempt at the preferred upstream. Group health
  state and probers survive config reloads that don't change the group, and a
  rejected config never disturbs the running probers. Live status at
  `GET /api/upstream-health`; full CRUD at `/api/upstream-groups/`; UI gains an
  "Upstream Groups" section (typed editor with per-upstream up/down chips) and
  a group selector on the proxy-host editor. Referential integrity enforced:
  unknown or disabled group references are rejected at write time.
- **Load-distribution policies and weights on upstream groups.** `policy` on
  `UpstreamGroup` selects how traffic spreads across the healthy upstreams:
  `failover` (default - strict list order, unchanged behavior), `round-robin`
  (smooth weighted round-robin, nginx's algorithm), `least-connections`
  (fewest in-flight requests relative to weight, tracked per upstream until
  each response body closes), and `ip-hash` (rendezvous hashing on the client
  IP for sticky sessions - when an upstream dies only its own clients move).
  Per-upstream `weight` (1-256, default 1) sets the relative share for the
  weighted policies and is ignored by `failover`/`ip-hash`. Unhealthy
  upstreams are demoted to the end of the try-order under every policy
  (fail-open preserved), and the connect-error-only retry rule is unchanged.
  `GET /api/upstream-health` now also reports each upstream's `weight` and
  in-flight `active` count. UI: policy select and per-upstream weight column
  on the group editor.
- **Cookie-based sticky sessions with a server-enforced TTL on upstream
  groups.** `stickiness: {ttl: "12h", cookie: "..."}` on `UpstreamGroup` pins
  each client to its assigned upstream via an HMAC-signed cookie (default name
  `gpm-sticky-<group>`; `HttpOnly`, `Path=/`, `SameSite=Lax`, `Secure` when the
  client arrived over HTTPS). The TTL (Go duration, plus a `d` day suffix,
  e.g. `3d`) rides inside the signed value, so replaying a cookie past its
  `Max-Age` still re-assigns - expiry is authoritative server-side and the
  window is fixed from assignment, not sliding. Signed with the existing
  data-plane SSO key (`GPM_SSO_SIGNING_KEY` / persisted `sso_signing.key`), so
  clients cannot forge a pin to a chosen backend; tampered, expired,
  foreign-group, or dead-upstream pins all fall back to the policy and get a
  fresh assignment cookie, and an honored pin adds no Set-Cookie noise.
  Composes with every policy (the policy picks initial/replacement
  assignments). UI: sticky TTL + cookie-name fields on the group editor.
- **Per-location upstream group references.** `upstreamGroupRef` on a
  `Location` (mutually exclusive with the location's `upstream`) points one
  path at its own group; with neither set the location inherits the host
  backend as before. Validated like the host-level reference (unknown or
  disabled groups rejected). UI: a group select per location row on the
  proxy-host editor.
- **Configurable rate-limit window on `RateLimitMiddleware`.** Rate limits can
  now be expressed as `requests` + `window` (a Go duration string, e.g. `10s`,
  `1m`, `1h`) for limits that don't reduce cleanly to a per-second rate (e.g.
  "100 requests per 1m", "5 per 1h"). The legacy `requestsPerSecond` field is
  kept as shorthand for `requests`/`window: 1s` and remains fully supported;
  exactly one of the two forms may be set, validated at config load. Burst
  still defaults to `ceil(requests)` and can still be overridden explicitly.
  Data-plane refill rate and `Retry-After` math both derive from the
  configured window rather than assuming seconds. UI editor (middleware
  rate-limit form) updated to a `Requests` + `Per` (window) pair; existing
  `requestsPerSecond`-only configs still load and behave identically, and
  editing one through the UI migrates it to the new form on save.
- **`allowFrom` exemption on `RateLimitMiddleware`.** Rate-limit middlewares now
  accept an `allowFrom` list of client CIDRs/IPs (same shape and validation as
  `AuthMiddleware.AllowFrom` / `GuardMiddleware.AllowFrom`); a matching client
  bypasses the limiter entirely (no token consumed, no 429), resolved via the
  same XFF-aware client-IP logic as the rest of the chain. Non-matching and
  unresolvable-IP clients are limited exactly as before. Config, data-plane
  enforcement, UI editor, and tests all updated.
- **Optional `blockFor` block period on `RateLimitMiddleware`.** Once a client
  exceeds the limit, further requests from it are rejected for a configured
  duration (a Go duration string, e.g. `"30s"`, `"5m"`) regardless of token
  refill, so a briefly-paused client cannot immediately sail back through. The
  block is fixed, not sliding - repeat requests during the block do not extend
  it, and once it expires ordinary token-bucket rules resume (no instant
  re-burst beyond `burst`). Optional and off by default (empty `blockFor` is
  today's refill-only behavior); validated via `time.ParseDuration` (must be
  > 0) alongside either rate form. UI editor adds a "Block for" select
  (none/10s/30s/1m/5m/15m/1h, with round-trip preservation for a
  hand-authored value like `2m`) next to the rate-limit form.
- **Opt-in `net/http/pprof` on the admin server.** New `-pprof` flag / `GPM_PPROF`
  env var (default off, same pattern as `-debug-headers`) mounts `net/http/pprof`
  under `/debug/pprof/` on the admin listener only, behind the same admin-role +
  same-origin CSRF gate as the REST API (`RequireRole(RoleAdmin, ...)` +
  `sameOriginGuard`). Registered explicitly on the admin mux, never via the
  side-effecting `_ "net/http/pprof"` import, so nothing is ever exposed on
  `http.DefaultServeMux`. Never touches the data-plane router.
- **Domain-group filter chips on the Proxy Hosts list.** Each host's domains are
  grouped into a "zone" (a wildcard's remainder, or the last two labels of a
  regular domain - `sensor.iot.example.com` and `*.iot.example.com` both group
  under `iot.example.com`). When a list has 2+ zones, a chip row renders below the
  search box (label "zone (count)", sorted by count then name); clicking a chip
  excludes/includes that zone, composing with the existing text filter. Excluded
  zones persist in `localStorage` (`gpm.hosts.zonesOff`) across navigation and
  reloads.
- **mTLS (phase 1): per-host client-certificate auth** (`tls.clientAuth`): a
  proxy/redirect/dead host can require or accept a client certificate verified
  against a new `ClientCA` trust-anchor object (`caPEM`, inline or
  `${FILE:...}`, confined to `GPM_SECRET_FILE_ROOTS`). `mode: require`
  (default) rejects the handshake without a valid client cert; `mode:
  optional` verifies a presented cert but lets certless requests through.
  Enforced per request, not just per handshake: the negotiated SNI must
  resolve to the host's own `tls.Config` (closing an SNI != Host dodge) and,
  in `require` mode, carry a verified chain, or the request gets `421
  Misdirected Request`; an mTLS host is also redirected off the plaintext
  `:80` listener. `clientAuth` requires `forceSSL: true` and a resolvable
  `caRef` at validation time. Revocation (CRL/OCSP) and identity-passthrough
  headers are not implemented yet (phase 2).
- **GeoIP geoblocking** (`geo` on `AccessList`): `countryAllow` /
  `countryDeny` (ISO-3166-1 alpha-2) plus `onUnknown: allow|deny` for IPs with
  no country (private/LAN, or a stale/missing database). Reuses the existing
  access-list client-IP resolution. New runtime dependency
  `github.com/oschwald/maxminddb-golang/v2` (pure Go, no CGO) reads an
  operator-supplied MaxMind GeoLite2/GeoIP2 `.mmdb` (not bundled - GeoLite2
  licensing) via `GPM_GEOIP_DB` / `-geoip-db`, hot-reloaded on a 5-minute
  mtime watch so an out-of-band `geoipupdate` refresh takes effect without a
  restart. Fail-closed throughout: saving an access list with geo rules while
  no database is loaded is rejected at write (`400`, nothing committed); a
  geo list evaluated with no database loaded denies all traffic on its hosts
  rather than allowing, evaluated live per request so it self-heals the
  moment the database loads, no restart or config change needed; startup no
  longer fatals on a geo config with no database - the edge boots with the
  affected hosts denying.
- **`GET /api/capabilities`**: read-only, admin-session-gated probe
  (`{"geoip":{"dbLoaded":bool}}`) the admin SPA uses to grey out/disable geo
  access-list controls when no GeoIP database is loaded.
- **Per-host no-index toggle** (`robotsNoIndex`): emits
  `X-Robots-Tag: noindex, nofollow` on HTTP and HTTPS responses. A headers
  middleware that sets `X-Robots-Tag` explicitly still wins.
- **Host tags** (`tags` on every object's metadata): free-form flat labels for
  grouping/filtering, surfaced as chips on the Proxy Hosts list and matched by the
  existing filter box.
- **Per-host upstream timeouts** (`timeouts.connectSeconds` / `readSeconds`): a
  host with an override uses its own cloned transport (its own connection pool), so
  the override cannot affect any other host. Hosts without an override keep using
  the shared, pooled transport unchanged. `readSeconds` bounds time-to-first-byte
  only, so it is safe for streaming/websocket upstreams.
- **Access-log viewer** (`GET /api/logs` + "Access Logs" view): an in-memory ring
  of recent requests (method, host, path, status, duration, client), newest first.
  Only filled while access logging is enabled (`--access-log` / `GPM_ACCESS_LOG`),
  so the default off-path keeps zero per-request overhead; the buffer is bounded.
- **Lifecycle webhooks** (`settings.webhooks`): gpm POSTs a small JSON event
  (`action`, `kind`, `name`, `commit`, `time`) to each configured URL after every
  config change. Delivery is asynchronous and best-effort under a 10s timeout, so a
  slow or unreachable endpoint never blocks or fails a config write. An optional
  per-target secret (placeholder-resolved) is sent as `X-GPM-Webhook-Secret`.
- **Data plane**: native-Go reverse proxy with SNI-based TLS termination (exact +
  wildcard certificate selection), HTTP/2, WebSocket upgrades, and force-SSL
  (HTTP->HTTPS 308 redirects).
- **Host types served by the data plane**: proxy hosts (reverse proxy + chain),
  **redirect hosts** (configurable scheme/status/path-preserving 3xx), **dead
  hosts** (fixed status, default 404; absorb unmatched vhosts, no default-host
  leakage), and **stream hosts** (raw TCP/UDP forwarding on their own per-port
  listeners, reconciled on reload). Previously only proxy hosts routed.
- **Path-scoped locations**: per-path upstream and middleware/access-list overrides
  on a single host (longest-prefix match, chains appended to the host's).
- **Certificates**: ACME DNS-01 issuance and automatic renewal (Cloudflare
  provider, ECDSA P-256, renew 30 days before expiry), plus bring-your-own custom
  certificates.
- **Access control & auth**: IP access lists (allow/deny CIDR rules, default-deny,
  HTTP basic-auth); OIDC admin login (authorization code + PKCE, group->role
  mapping); forward-auth (trusted-peer identity headers); auth-request
  (nginx `auth_request`-style outpost subrequest); request guards (deny by
  path/method/query with CIDR exemptions); headers and rate-limit middleware.
- **Rate-limit middleware enforcement**: the `rate-limit` type is now applied on
  the data plane as a per-host, per-client-IP token bucket (capacity = `burst`,
  refill = `requestsPerSecond`); over-limit requests get `429 Too Many Requests`
  with a `Retry-After` header. Tracked buckets are capped with idle eviction so a
  flood of distinct source IPs cannot grow memory without bound.
- **Composable middleware chain** per host and per location, applied in a fixed
  order (rate-limit -> auth -> guard -> access-list -> headers -> upstream).
- **Configurable admin auth**: local bcrypt break-glass and/or OIDC; `ssoOnly`
  mode; configurable provider list.
- **Login experience**: dark, self-contained login page; provider `DisplayName`
  as the button label; configurable `appName` (UI nav + login); auto-redirect to
  the IdP under SSO-only with a `?select=1` escape for logout.
- **Config store**: git-backed declarative config (one YAML object per file),
  whole-graph referential validation, every change a commit.
- **Secrets**: `${ENV:...}` / `${FILE:...}` placeholders resolved at use; literal
  secrets refused at commit and redacted in API responses.
- **Control plane**: REST CRUD API + embedded vanilla-JS single-page admin UI
  (`go:embed`).
- **Per-host OIDC gating** on the data plane: a proxy host with auth mode `oidc`
  makes gpm the OIDC relying party - unauthenticated requests are redirected to
  the IdP, the callback (`/__gpm/oidc/callback`) exchanges the code (PKCE + nonce)
  and issues a signed, stateless SSO session cookie, and the role mapping is
  enforced. Sign the cookie with `GPM_SSO_SIGNING_KEY` to persist sessions across
  restarts.
- **HSTS emission**: the data plane now sends `Strict-Transport-Security` on HTTPS
  responses for hosts with `tls.hsts.enabled` (honouring `maxAge`,
  `includeSubdomains`, `preload`; one-year default).
- **Per-host minimum TLS version**: `tls.minTLSVersion` (`"1.2"` default | `"1.3"`)
  pins a host's TLS floor by SNI via `GetConfigForClient`; the edge still
  negotiates 1.2/1.3 per client by default, and a host can require 1.3 where all
  its clients support it.
- **Config backup / restore**: `GET /api/backup` downloads a portable gzip-tar of
  the whole config; `POST /api/restore` replaces it from such an archive
  (validated, committed as one revision, rolled back if invalid) - with UI
  controls on the History view.
- **Config revert**: `POST /api/revert` restores the entire config to a past
  commit as a new commit (so it is itself revertible); the History view's per-
  commit revert action is now live.
- **Tooling**: one-time Nginx-Proxy-Manager/NPMplus importer (`gpm import`),
  `gpm hashpw` for admin/basic-auth hashes.
- **Observability**: structured zerolog logging, optional access log, slow-request
  warnings, opt-in `X-GPM-*` debug headers, configurable upstream
  response-header timeout.
- **Packaging**: multi-arch container image (linux/amd64 + arm64) to GHCR,
  Docker Compose deployment, `cap_drop: ALL` + `no-new-privileges`.
- **Documentation**: README, `docs/configuration.md`, `docs/deployment.md`,
  `docs/architecture.md`.

### Changed

- **Data-plane reverse proxy tuned for large/streamed upstream responses.** The
  shared upstream transport now sets `DisableCompression: true` (the proxy must
  not transparently gunzip upstream bodies - CPU cost, and it strips
  Content-Length, forcing the flush-per-write chunked path), and the main
  `httputil.ReverseProxy` now uses a fixed 512KiB `sync.Pool`-backed `BufferPool`
  (a larger copy buffer cuts the per-write flush count for chunked upstream
  responses, which stdlib flushes after every write). The outpost auth
  subrequest client (`internal/dataplane/authrequest.go`) now uses a dedicated,
  tuned `http.Transport` (no proxy env, 5s dial/TLS timeouts, 64 idle conns per
  host) instead of the default transport's 2-conn-per-host cap.
- **Browser tab title dropped the " admin" suffix.** The static fallback title
  and the dynamic `document.title` (set from `settings.appName` once loaded)
  now both read just "Go Proxy Manager" (or the configured app name), matching
  the sidebar wordmark.
- **Lifecycle webhooks now fire only after a config change is applied to the
  running data plane, not merely committed to git** - a write that commits
  but fails to apply (e.g. a geo rule saved the instant its GeoIP database
  goes missing) emits no webhook. The event payload
  (`action`/`kind`/`name`/`commit`/`time`) is unchanged; the delivered event
  now means "applied and live," not just "committed."
- **Admin UI**: replaced the labelled raw-JSON editor with full field-level forms
  for every remaining object kind - redirect hosts, stream hosts, dead hosts, DNS
  providers, identity providers, access lists, and middleware. Each form has typed
  inputs (text/number/enum selects), add/remove rows for list and map fields
  (domains, CIDR rules, basic-auth users, header maps, guard triggers, etc.), an
  identity-provider picker for auth middleware, and secret-aware fields that show
  `${ENV:...}` placeholders and refuse to overwrite a masked (`***`) secret.
  Polymorphic kinds (identity provider type, middleware type) toggle the relevant
  sub-spec section on change.
- Build on Go 1.26.4.
- Tuned the shared upstream transport (HTTP/2, higher idle-conns-per-host) to cut
  connection churn for backends that share an upstream.
- Admin password hash read from `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE` (Docker
  secret) in preference to an inline env value, avoiding compose `$`-interpolation
  of bcrypt hashes.
- Moved the parallel-test data-plane ports off 8080 to 8880/8843.

### Fixed

- **Duplicate `Strict-Transport-Security` on the proxied admin path.** The admin
  server emitted its own HSTS header in addition to the one the data plane (the
  actual TLS edge) emits for the admin host, so a request to the admin panel
  through gpm carried two identical HSTS headers. HSTS is now set only by the data
  plane; the admin server no longer sets it (over its direct plain-HTTP port HSTS
  was ignored anyway, and via the proxy the edge owns it).
- Logout now lands on the login page under SSO-only instead of silently
  re-authenticating against a live IdP session.
- Rewrite upstream `Location` redirects that point back at the upstream's own
  address (e.g. Pi-hole/civetweb absolute redirects) to the public scheme/host.
- Custom-certificate absolute-path validation is now OS-agnostic (a leading `/`
  or `\` is rejected on every platform, not just where `filepath.IsAbs` treats it
  as absolute).
- Removed the dead, unreferenced `router.tlsConfig()` (the HTTPS server builds its
  own `tls.Config` inline); the TLS 1.2 floor is unchanged.

### Security

- **Webhook delivery is SSRF-bounded** (issue #1, low/defense-in-depth).
  Lifecycle-webhook targets are admin-configured URLs, which made delivery an
  SSRF primitive from gpm's network position. Redirects are no longer followed
  (a 3xx counts as a failed delivery instead of bouncing gpm to a URL the admin
  never configured), and link-local destinations (`169.254.0.0/16`, `fe80::/10`
   -  cloud metadata services) are refused at connect time on the resolved
  address, so a rebinding resolver cannot dodge the check. Private/LAN targets
  remain allowed (the normal self-hosted case); URL scheme/shape was already
  validated at config-write time.
- **Data-plane SSO sessions gained global revocation** (issue #1,
  low/defense-in-depth). Sessions stay stateless (1h absolute TTL), but
  `POST /api/sso/revoke` (admin-gated, CSRF-protected; also a button under
  Settings) moves a persisted revocation watermark to "now": any session issued
  before it - including legacy cookies minted before this change - fails the
  gate and the user re-authenticates at the IdP. The watermark is stored next
  to the SSO signing key so revocation survives restarts. Session cookies now
  carry an `iat` claim to support the check.
- **`GPM_COOKIE_SECURE=0` production guard** (issue #1, low/defense-in-depth).
  gpm now logs a prominent startup warning when Secure cookies are disabled
  while `settings.externalBaseURL` is `https://` - the signature of a
  TLS-fronted deployment mis-set for local testing. A warning rather than a
  hard failure, because a LAN-only plain-HTTP admin listener alongside the
  public URL is a known deliberate topology.
- **Data-plane SSO cookies use the `__Host-` prefix.** `gpm_sso` / `gpm_sso_state`
  are now `__Host-gpm_sso` / `__Host-gpm_sso_state`, so the browser enforces their
  Secure + host-locked (no `Domain`) + `Path=/` scope and a sibling subdomain cannot
  plant a same-named shadow cookie. (Forged values already failed the HMAC; this
  closes the shadowing vector.) Active sessions re-authenticate once (GPM-I2).
- **Settings validation rejects an admin-lockout configuration.** A settings write
  with neither local login nor any SSO provider - no way into the admin panel - is
  now refused at validation instead of committing and locking the operator out
  (previously recoverable only by redeploy) (GPM-I4).
- **A host referencing a disabled ClientCA is rejected at validation.** Previously
  referential validation only checked the CA name existed; a disabled CA then
  produced a nil pool and a hard TLS-config error that failed the whole router
  reload. The disabled reference is now a clear load-time error (GPM-I3).
- **Access lists are evaluated ahead of authentication.** The middleware chain now
  runs `rate-limit -> access-list -> auth -> guard -> headers -> upstream` (previously
  the access-list sat inside auth). An IP the access-list would deny is now dropped
  before any auth work runs, so a non-allowed client hitting an OIDC/forward-auth
  host no longer drives a forward-auth subrequest to the IdP or receives an OIDC
  redirect/401 that discloses the auth flow. IP/geo access policy is enforced as an
  edge control, before identity, as intended (GPM-L1).
- **Client-IP resolution for IP-based controls is now per-host, not a global
  union.** `X-Forwarded-For` is honoured (for access-list, rate-limit, geo, and
  auth-request `allowFrom`) only for proxies the target host actually trusts - the
  `trustedProxies` of the forward-auth IdPs that host references - mirroring the
  host-scoped identity-header trust. Previously the trusted-proxy set was the union
  across every IdP, so a proxy trusted by one host could have its `XFF` honoured
  when resolving another host's client IP. A host with no forward-auth IdP in front
  now keys IP controls off the connection peer (GPM-L4).
- **Data-plane SSO sessions use a 1-hour absolute TTL (was 12h).** The `gpm_sso`
  cookie is no longer valid for 12h with no revocation path; it now expires 1h after
  login and is not extended by activity. On expiry the gate re-runs the OIDC flow
  (silent while the IdP session is valid) and re-checks group membership, bounding
  the offboarding window for a deprivileged or disabled user to ~1h without
  server-side session state (GPM-L3).
- **Login/OIDC-begin rate gate fails closed under a distinct-key flood.** When the
  gate's per-IP map is saturated with live entries, a new key could previously go
  unrecorded and thus never reach its limit (a brute-force bypass). The read path
  now treats an untracked key as at-limit while the map is saturated, so a flood
  degrades login into a lockout rather than an unthrottled guess (GPM-L2).
- **`Restore` enforces the no-literal-secrets guard.** An uploaded backup archive is
  now checked for literal (non-placeholder) secrets after load, matching `Save`/
  `SaveSettings`; a violation rolls the working tree back and commits nothing, so a
  crafted archive cannot land a plaintext secret on disk or in git history. The
  rollback itself was hardened: `RestoreTree` now also removes untracked files
  (`git clean -fd`), closing a gap where a refused restore's newly-written files
  survived on disk and would be picked up by the next config load (GPM-L6).
- **`${ENV:...}` cannot resolve gpm's own reserved secrets, and can be prefix-gated.**
  `GPM_SSO_SIGNING_KEY` and `GPM_LOCAL_ADMIN_PASSWORD_HASH` are never resolvable via
  an `${ENV:...}` placeholder, so an admin-authored value cannot exfiltrate the SSO
  signing key or admin password hash (e.g. via a webhook secret). Operators can
  further restrict resolution to an allowlist of name prefixes with
  `GPM_SECRET_ENV_PREFIXES`, mirroring `${FILE:...}` confinement (GPM-L7).
- **Rate-limiter bucket eviction is now O(1) (LRU) instead of an O(n) map scan.**
  At the 16 384-bucket cap, a rotating-source-IP flood forced a full-map scan under
  the limiter mutex per new key, serializing rate-limit checks for the host (a DoS
  amplifier). Buckets are held in an LRU list and the least-recently-used one is
  evicted in constant time; memory stays bounded as before (GPM-L5).
- **Access lists reject `\` and `;` in the request path.** Path matching for
  path-scoped locations and guard triggers is exact after `path.Clean`, which does
  not treat a backslash or a `;` matrix parameter as path structure. A request like
  `/admin;x` or `/admin\..\x` therefore failed to match a location/guard keyed on
  `/admin` yet could be re-collapsed onto `/admin` by a backend that strips
  `;`-parameters (Servlet containers) or treats `\` as a separator (IIS/Windows),
  slipping past that path's auth. The data plane now rejects any request whose
  canonical path still contains `\` or `;` with `400 Bad Request`, keeping the
  matched path and the forwarded path byte-identical.
- **Data-plane SSO session cookies are bound to their issuing host.** The `gpm_sso`
  cookie is HMAC-signed with a process-wide key, so a valid session minted for one
  OIDC-gated host verified on every other. With two hosts fronted by different IdPs
  that share a group name, a cookie copied from host A would be re-evaluated against
  host B's role mapping using A's groups (cross-IdP privilege confusion). The signed
  payload now carries the issuing host and a gate rejects any cookie whose host is
  not its own (a mismatched cookie triggers a fresh login).
- **Geo whitelist (`countryAllow`) fails closed on unknown IPs by default.**
  Previously `onUnknown` defaulted to `allow` in all modes, so an IP the GeoIP
  database could not place in a country - unallocated space, stale-DB gaps, some
  cloud/VPN ranges - was admitted even by a `countryAllow: [US]` whitelist,
  defeating it. When `onUnknown` is unset, whitelist mode now defaults to `deny`;
  deny-list mode keeps `allow` (it only narrows). An explicit `onUnknown` still
  wins.
- **`defaultAction: deny` on a rule-less access list now denies.** An access list
  with no IP, basic-auth, or geo rules but an explicit `defaultAction: deny` (a
  deliberate "lock this host down") was treated as an empty, unrestricted list and
  silently allowed all traffic. Such a list now denies; an unset or `allow` default
  remains open, as before.
- **Session database created 0600 in a 0700 directory.** The SQLite session
  store (`session.db`) was previously created with default OS permissions (typically
  0644 dir / 0644 file), making session IDs and CSRF tokens world-readable. The
  parent directory is now created 0700 and the file is pre-created 0600 (and
  `chmod`'d on open to tighten pre-existing deployments).
- **`GET /version` no longer exposes the config-repo HEAD commit.** The response
  previously included a `configCommit` field containing the current HEAD SHA of the
  git-backed config repository, leaking internal state to any authenticated caller.
  The field has been removed; the response now contains only the binary version info.
- **`roleMapping.defaultRole: "admin"` now requires explicit opt-in.** Setting
  `defaultRole` to `"admin"` without also setting `roleMapping.allowDefaultAdmin:
  true` is rejected at validation time. Without group gating, `defaultRole: "admin"`
  grants full admin to every user the IdP authenticates; the new required field stops
  that from happening silently by config typo. Unknown `defaultRole` values are also
  rejected (previously silently treated as deny).
- **Per-IP rate limit on OIDC admin login starts.** A client IP may initiate at
  most 30 OIDC login flows within any 10-minute window. Attempts beyond the cap
  receive a 502 immediately, before any redirect to the IdP. This prevents a single
  client from exhausting the global pending-login budget and blocking OIDC logins for
  all users. Legitimate flows behind shared NAT are unaffected by the generous cap.
- **Data-plane SSO signing key is now persisted across restarts.** When
  `GPM_SSO_SIGNING_KEY` is not set, gpm previously generated a random ephemeral key
  per process, invalidating all per-host OIDC sessions on every restart. The key is
  now generated once and written to `<cert-dir>/sso_signing.key` (0600) on first
  use, so sessions survive restarts. If the file is found corrupt on startup it is
  renamed to `sso_signing.key.corrupt` and a fresh key is generated and persisted.
  `GPM_SSO_SIGNING_KEY` still takes precedence when set.
- **Forward-auth identity strip no longer breaks a proxied Authentik.** The
  baseline identity-header denylist stripped the entire `X-Authentik-*` request
  family from untrusted peers, which also removed Authentik's own CSRF token header
  (`X-authentik-CSRF`, read from the `authentik_csrf` cookie and sent on every
  flow-executor API POST). When Authentik itself is proxied through gpm, every
  admin login failed with "CSRF Failed: CSRF token missing". `X-Authentik-Csrf` is
  now exempt from the strip - it is a CSRF token validated against a cookie, not an
  identity assertion, so forwarding it is no escalation risk.
- Remediated an internal security review (8 high / 3 medium / 11 low): identity
  headers stripped from untrusted peers; CSRF double-submit token + same-origin
  guard on the admin API; literal-secret commit guard; API secret redaction;
  object-name validation closing config-path traversal; fail-closed access-list
  and auth gates; XFF honored only from configured trusted proxies.
- Remediated a second review (1 high / 7 medium / 3 low) on the data plane and
  admin auth added since:
  - **Request-path normalization (high)**: location and guard matching now run on
    a cleaned path (`path.Clean`, decoded, RawPath dropped) matched on a segment
    boundary, and the cleaned path is forwarded upstream - closing
    `/x/../admin`, `//admin`, encoded-traversal, and `/reports-evil` over-match
    bypasses of per-location auth and guard rules.
  - **`${FILE:}` secret confinement**: file-backed secrets resolve only within an
    allowlisted root (`/run/secrets` by default, override `GPM_SECRET_FILE_ROOTS`),
    rejecting relative and out-of-root/`..` paths.
  - **Open-redirect hardening**: `sanitizeReturnTo` now rejects backslash,
    protocol-relative, and control-character return URLs.
  - **OIDC login binding**: a short-lived state cookie set at login start is
    compared (constant-time) at the callback, blocking login-CSRF / fixation.
  - **Baseline identity-header strip**: a fixed denylist (`Remote-User`,
    `X-Forwarded-User`, the `X-Auth-Request-*` / `X-Authentik-*` families, ...) is
    stripped from untrusted peers regardless of the configured IdP.
  - **Per-host identity trust**: a proxy trusted by one host's IdP is no longer
    implicitly trusted to assert identity to other hosts (no global CIDR union).
  - **Importer bounds**: a per-table row cap fails an over-large source loudly.
  - **Session cookie**: `__Host-` prefix adopted when cookies are `Secure`
    (honouring `GPM_COOKIE_SECURE`, so a plain-HTTP admin deployment is unaffected).
  - **Admin security headers**: HSTS, `X-Content-Type-Options`, `X-Frame-Options`,
    CSP `frame-ancestors`, and `Referrer-Policy` on every admin/login response.
  - **Local-login throttling**: per-client-IP lockout after repeated failures,
    bounded pending-login/throttle maps, and sliding session expiry.

[Unreleased]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.33...v1.1.0
[1.0.16]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.15...v1.0.16
[1.0.15]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.13...v1.0.15
[1.0.13]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.10...v1.0.13
[1.0.10]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.9...v1.0.10
[1.0.9]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.8...v1.0.9
[1.0.8]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.7...v1.0.8
[1.0.7]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.5...v1.0.7
[1.0.5]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.4...v1.0.5
[1.0.4]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.1...v1.0.4
[1.0.1]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/Rake-Pro/go-proxy-manager/releases/tag/v1.0.0
