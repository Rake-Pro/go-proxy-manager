# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

Nothing yet.

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

[Unreleased]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/Rake-Pro/go-proxy-manager/releases/tag/v1.0.0
