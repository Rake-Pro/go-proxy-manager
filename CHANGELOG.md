# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- Docs accuracy and structure pass; neutralized wording in test fixtures and
  in failure-mode comments and docs. No behavior change.

## [1.0.10] - 2026-09-26

### Fixed

- ACME `keyType` was documented and offered in the UI as accepting `rsa`, but
  the CSR path only ever issues ECDSA P-256 certificates; a config with
  `keyType: rsa` validated successfully and only failed at the next
  issue/renew attempt. `Validate()` now rejects an unsupported
  `acme.keyType` at config-write time, and the docs, OpenAPI enum, and UI
  placeholder (`EC256` -> `ecdsa`) were corrected to match.
- Corrected the `acme.email` hint and docs text, which still described Let's
  Encrypt using the address for expiry notices; Let's Encrypt stopped
  sending those in 2025, and the address is now used only for
  account/policy notices.

## [1.0.9] - 2026-09-24

### Security

- Alpine base image bumped from 3.24.1 to 3.24.2.

### Changed

- Dependencies: modernc.org/sqlite v1.58.0 -> v1.59.0.

## [1.0.8] - 2026-09-22

### Security

- Dependencies: golang.org/x/crypto v0.56.0 -> v0.57.0.

### Changed

- Dependencies: github.com/oschwald/maxminddb-golang/v2 v2.5.0 -> v2.6.0
  (GeoIP lookups).

## [1.0.7] - 2026-09-10

### Security

- Dependencies: github.com/go-jose/go-jose/v4 v4.1.4 -> v4.1.5,
  golang.org/x/oauth2 v0.36.0 -> v0.37.0.

## [1.0.6] - 2026-09-06

### Changed

- CI: added a weekly Trivy rescan (Monday cron) of the released image on
  GHCR, so CVEs disclosed after release surface without needing a code push.

## [1.0.5] - 2026-09-06

### Changed

- CI: release flow renamed from `main -> prod` to `dev -> main`. `dev` is the
  default working branch, `main` the protected release branch; the bot PR is
  now "Merge dev to main". Mechanics unchanged. Pushes to `dev` now also
  publish `:dev` and `:dev-<sha>` images.

## [1.0.4] - 2026-09-05

### Changed

- Dependencies: migrated from the archived `gopkg.in/yaml.v3` to the
  maintained `go.yaml.in/yaml/v3` fork (same API).

## [1.0.3] - 2026-09-05

### Fixed

- The sidebar wordmark uses the same fan-out glyph as the favicon; the two
  had diverged in 1.0.2.

## [1.0.2] - 2026-09-03

### Added

- The admin UI declares a favicon (`favicon.svg`: a two-way fan-out on the
  brand gradient tile), so browser tabs and bookmarks show the app icon
  instead of the generic page glyph.

### Changed

- Removed the unused `deploy/runbook-template.md` authoring scaffold.
- The Go toolchain is pinned once, in `.go-version`: CI reads it, `make
  bump-go VERSION=X.Y.Z` rewrites it together with both builder images (tag
  and digest), and a test fails when any of them drift or when the docs quote
  a language minimum other than go.mod's.

## [1.0.1] - 2026-09-02

### Changed

- Build on Go 1.27.1 (builder image and CI toolchain).
- Dependencies: go-oidc v3.21.0, golang.org/x/crypto v0.56.0,
  modernc.org/sqlite v1.58.0 (with its libc and memory modules).

## [1.0.0] - 2026-09-02

First public release.

### Added

- **Proxying.** SNI-based TLS termination with exact and wildcard certificate
  matching, HTTP/2 and WebSockets, HTTP-to-HTTPS redirect, path-scoped
  locations, upstream groups (failover, weighted round-robin,
  least-connections, sticky ip-hash) with active and passive health checks,
  redirect hosts, raw TCP/UDP streams with SNI routing, and parked hosts.
- **Certificates.** ACME issuance and renewal over HTTP-01 or DNS-01 against
  any CA, four named DNS providers plus RFC2136 and acme-dns solvers, custom
  PEM certificates, a fleet-wide TLS floor, and a built-in client CA with
  PKCS#12 client bundles and CRL revocation.
- **Access and auth.** Ordered allow/deny access lists with GeoIP, path and
  method scoping and remote CIDR feeds, all keyed on one derived client IP;
  OIDC, forward-auth, `auth_request`, client-certificate and basic auth as
  typed config; a composable middleware chain (rate limits, guards, rewrites,
  security headers, bouncer hooks, maintenance mode).
- **Automation.** Kubernetes Ingress and Docker label discovery with plan and
  reconcile, DNS record sync, and notifications to ntfy, Discord or a generic
  webhook.
- **Admin panel and API.** Git-backed per-object YAML config with history,
  a REST API with an OpenAPI description and scoped tokens, local login with
  optional TOTP, OIDC admin sign-in with group-to-role mapping, a read-only
  viewer role, Prometheus metrics, and an Overview that lists only what needs
  attention.

[Unreleased]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.10...HEAD
[1.0.10]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.9...v1.0.10
[1.0.9]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.8...v1.0.9
[1.0.8]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.7...v1.0.8
[1.0.7]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.6...v1.0.7
[1.0.6]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.5...v1.0.6
[1.0.5]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.4...v1.0.5
[1.0.4]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.3...v1.0.4
[1.0.3]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/Rake-Pro/go-proxy-manager/releases/tag/v1.0.0
