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
- **Composable middleware chain** per host and per location, applied in a fixed
  order (auth → guard → access-list → headers → upstream).
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

### Security

- Remediated an internal security review (8 high / 3 medium / 11 low): identity
  headers stripped from untrusted peers; CSRF double-submit token + same-origin
  guard on the admin API; literal-secret commit guard; API secret redaction;
  object-name validation closing config-path traversal; fail-closed access-list
  and auth gates; XFF honored only from configured trusted proxies.

[Unreleased]: https://github.com/Rake-Pro/go-proxy-manager/commits/main
