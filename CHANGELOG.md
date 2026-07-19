# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The project is
pre-1.0 and has no tagged releases yet; everything to date lives under
*Unreleased*.

## [Unreleased]

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
  `/application/o/token → /application/o/token/` rewrite at the edge fixes it.
  Both key and value must be absolute paths and a key may not map to itself
  (`internal/dataplane/rewrite.go`, model `RewriteMiddleware`).
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
  upstream **only on connect-phase failures** (dial/TLS — the request was never
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
  `failover` (default — strict list order, unchanged behavior), `round-robin`
  (smooth weighted round-robin, nginx's algorithm), `least-connections`
  (fewest in-flight requests relative to weight, tracked per upstream until
  each response body closes), and `ip-hash` (rendezvous hashing on the client
  IP for sticky sessions — when an upstream dies only its own clients move).
  Per-upstream `weight` (1–256, default 1) sets the relative share for the
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
  `Max-Age` still re-assigns — expiry is authoritative server-side and the
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
  regular domain — `sensor.iot.example.com` and `*.iot.example.com` both group
  under `iot.example.com`). When a list has 2+ zones, a chip row renders below the
  search box (label "zone (count)", sorted by count then name); clicking a chip
  excludes/includes that zone, composing with the existing text filter. Excluded
  zones persist in `localStorage` (`gpm.hosts.zonesOff`) across navigation and
  reloads.

### Changed

- **Data-plane reverse proxy tuned for large/streamed upstream responses.** The
  shared upstream transport now sets `DisableCompression: true` (the proxy must
  not transparently gunzip upstream bodies — CPU cost, and it strips
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

### Fixed

- **Duplicate `Strict-Transport-Security` on the proxied admin path.** The admin
  server emitted its own HSTS header in addition to the one the data plane (the
  actual TLS edge) emits for the admin host, so a request to the admin panel
  through gpm carried two identical HSTS headers. HSTS is now set only by the data
  plane; the admin server no longer sets it (over its direct plain-HTTP port HSTS
  was ignored anyway, and via the proxy the edge owns it).

### Security

- **Webhook delivery is SSRF-bounded** (issue #1, low/defense-in-depth).
  Lifecycle-webhook targets are admin-configured URLs, which made delivery an
  SSRF primitive from gpm's network position. Redirects are no longer followed
  (a 3xx counts as a failed delivery instead of bouncing gpm to a URL the admin
  never configured), and link-local destinations (`169.254.0.0/16`, `fe80::/10`
  — cloud metadata services) are refused at connect time on the resolved
  address, so a rebinding resolver cannot dodge the check. Private/LAN targets
  remain allowed (the normal self-hosted case); URL scheme/shape was already
  validated at config-write time.
- **Data-plane SSO sessions gained global revocation** (issue #1,
  low/defense-in-depth). Sessions stay stateless (1h absolute TTL), but
  `POST /api/sso/revoke` (admin-gated, CSRF-protected; also a button under
  Settings) moves a persisted revocation watermark to "now": any session issued
  before it — including legacy cookies minted before this change — fails the
  gate and the user re-authenticates at the IdP. The watermark is stored next
  to the SSO signing key so revocation survives restarts. Session cookies now
  carry an `iat` claim to support the check.
- **`GPM_COOKIE_SECURE=0` production guard** (issue #1, low/defense-in-depth).
  gpm now logs a prominent startup warning when Secure cookies are disabled
  while `settings.externalBaseURL` is `https://` — the signature of a
  TLS-fronted deployment mis-set for local testing. A warning rather than a
  hard failure, because a LAN-only plain-HTTP admin listener alongside the
  public URL is a known deliberate topology.
- **Data-plane SSO cookies use the `__Host-` prefix.** `gpm_sso` / `gpm_sso_state`
  are now `__Host-gpm_sso` / `__Host-gpm_sso_state`, so the browser enforces their
  Secure + host-locked (no `Domain`) + `Path=/` scope and a sibling subdomain cannot
  plant a same-named shadow cookie. (Forged values already failed the HMAC; this
  closes the shadowing vector.) Active sessions re-authenticate once (GPM-I2).

- **Settings validation rejects an admin-lockout configuration.** A settings write
  with neither local login nor any SSO provider — no way into the admin panel — is
  now refused at validation instead of committing and locking the operator out
  (previously recoverable only by redeploy) (GPM-I4).

- **A host referencing a disabled ClientCA is rejected at validation.** Previously
  referential validation only checked the CA name existed; a disabled CA then
  produced a nil pool and a hard TLS-config error that failed the whole router
  reload. The disabled reference is now a clear load-time error (GPM-I3).

- **Access lists are evaluated ahead of authentication.** The middleware chain now
  runs `rate-limit → access-list → auth → guard → headers → upstream` (previously
  the access-list sat inside auth). An IP the access-list would deny is now dropped
  before any auth work runs, so a non-allowed client hitting an OIDC/forward-auth
  host no longer drives a forward-auth subrequest to the IdP or receives an OIDC
  redirect/401 that discloses the auth flow. IP/geo access policy is enforced as an
  edge control, before identity, as intended (GPM-L1).

- **Client-IP resolution for IP-based controls is now per-host, not a global
  union.** `X-Forwarded-For` is honoured (for access-list, rate-limit, geo, and
  auth-request `allowFrom`) only for proxies the target host actually trusts — the
  `trustedProxies` of the forward-auth IdPs that host references — mirroring the
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
