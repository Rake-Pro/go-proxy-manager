# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The project is
pre-1.0 and has no tagged releases yet; everything to date lives under
*Unreleased*.

## [Unreleased]

### Fixed

- **Duplicate `Strict-Transport-Security` on the proxied admin path.** The admin
  server emitted its own HSTS header in addition to the one the data plane (the
  actual TLS edge) emits for the admin host, so a request to the admin panel
  through gpm carried two identical HSTS headers. HSTS is now set only by the data
  plane; the admin server no longer sets it (over its direct plain-HTTP port HSTS
  was ignored anyway, and via the proxy the edge owns it).

### Security

- **Access lists are evaluated ahead of authentication.** The middleware chain now
  runs `rate-limit → access-list → auth → guard → headers → upstream` (previously
  the access-list sat inside auth). An IP the access-list would deny is now dropped
  before any auth work runs, so a non-allowed client hitting an OIDC/forward-auth
  host no longer drives a forward-auth subrequest to the IdP or receives an OIDC
  redirect/401 that discloses the auth flow. IP/geo access policy is enforced as an
  edge control, before identity, as intended (GPM-L1).

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
  now exempt from the strip — it is a CSRF token validated against a cookie, not an
  identity assertion, so forwarding it is no escalation risk.

### Added

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
  (HTTP→HTTPS 308 redirects).
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
  HTTP basic-auth); OIDC admin login (authorization code + PKCE, group→role
  mapping); forward-auth (trusted-peer identity headers); auth-request
  (nginx `auth_request`-style outpost subrequest); request guards (deny by
  path/method/query with CIDR exemptions); headers and rate-limit middleware.
- **Rate-limit middleware enforcement**: the `rate-limit` type is now applied on
  the data plane as a per-host, per-client-IP token bucket (capacity = `burst`,
  refill = `requestsPerSecond`); over-limit requests get `429 Too Many Requests`
  with a `Retry-After` header. Tracked buckets are capped with idle eviction so a
  flood of distinct source IPs cannot grow memory without bound.
- **Composable middleware chain** per host and per location, applied in a fixed
  order (rate-limit → auth → guard → access-list → headers → upstream).
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

- Remediated an internal security review (8 high / 3 medium / 11 low): identity
  headers stripped from untrusted peers; CSRF double-submit token + same-origin
  guard on the admin API; literal-secret commit guard; API secret redaction;
  object-name validation closing config-path traversal; fail-closed access-list
  and auth gates; XFF honored only from configured trusted proxies.
- Remediated a second review (1 high / 7 medium / 3 low) on the data plane and
  admin auth added since:
  - **Request-path normalization (high)**: location and guard matching now run on
    a cleaned path (`path.Clean`, decoded, RawPath dropped) matched on a segment
    boundary, and the cleaned path is forwarded upstream — closing
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
    `X-Forwarded-User`, the `X-Auth-Request-*` / `X-Authentik-*` families, …) is
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

[Unreleased]: https://github.com/Rake-Pro/go-proxy-manager/commits/main
