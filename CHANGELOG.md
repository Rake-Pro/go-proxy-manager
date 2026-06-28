# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The project is
pre-1.0 and has no tagged releases yet; everything to date lives under
*Unreleased*.

## [Unreleased]

### Added

- **Data plane**: native-Go reverse proxy with SNI-based TLS termination (exact +
  wildcard certificate selection), HTTP/2, WebSocket upgrades, and force-SSL
  (HTTP→HTTPS 308 redirects).
- **Host types**: proxy hosts, redirect hosts, raw TCP/UDP stream hosts, and dead
  hosts (absorb unmatched vhosts; no default-host leakage).
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
