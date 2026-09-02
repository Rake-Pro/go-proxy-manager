# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- **Overview rewritten as a "needs attention" list.** The landing view now
  leads with what is actually wrong: certificates failing to renew, expired or
  expiring soon; upstream groups with unhealthy members; ingress/Docker
  discovery failures and skipped hosts; and config warnings, each a severity
  dot, a title, a detail line and a link (the certificate or group for object
  rows, Integrations for discovery rows, History for config warnings), error
  rows before warning rows. When there is nothing to report it shows a one-line
  summary (hosts live, certificates valid and next expiry, upstream members
  healthy) instead. A Refresh button re-runs the view without a full page
  reload.

### Removed

- **The "About this page" boxes on every list view, the Overview get-started
  checklist and the four stat tiles.** The per-field "?" hints and the glossary
  underlines stay; the boxes only described the screen they sat on, which the
  docs site already does. The stat tiles (proxy hosts, certificates, identity
  providers, upstreams) and the "Recent config changes" / "Certificates" feed
  cards are gone too: their normal, healthy state carried no information the
  new attention list needs to show, and every list they linked to is one click
  away in the sidebar.

## [1.3.0] - 2026-09-02

### Added

- **Fleet-wide TLS floor: `settings.tls.minVersion`.** One switch raises the
  minimum TLS version for every HTTPS listener, every stream host in
  `terminate` mode and an unknown or absent SNI, instead of setting
  `tls.minTLSVersion` on each host in turn. `"1.2"` (the default, and what an
  unset value means) keeps today's behaviour exactly; `"1.3"` hardens the whole
  edge. A ProxyHost's own `tls.minTLSVersion` still wins in **either**
  direction, so one legacy host can stay on 1.2 under a 1.3 fleet floor. New
  **Settings -> General -> TLS** card, `docs/reference/config/settings/tls.md`,
  and an OpenAPI `TLSFleetSettings` schema.
- **`securityHeaders` on the discovery templates.** `settings.ingressDiscovery`
  and `settings.dockerDiscovery` templates (and every named profile) now carry
  `securityHeaders`, applied verbatim to each derived host and validated by the
  same rules as a hand-written one. Without it a per-host override on a
  discovery-managed host was rebuilt away (with a git commit) on the next
  reconcile, exactly the gap `stripResponseHeaders` closed earlier.
- **Auth-gate refusals count in the denial metrics.** Every terminal 401/403 a
  `forward-auth`, `auth-request`, `oidc` or `client-cert` gate writes now
  increments `gpm_denials_total` with a per-mode reason (`auth-forward`,
  `auth-request`, `auth-oidc`, `auth-client-cert`, joining the existing
  `auth-basic`), so a host behind SSO or mTLS is no longer the one tier
  invisible on a dashboard. A redirect into a sign-in flow is not a denial and
  is not counted.
- **Prometheus series for the access-list source fetcher**:
  `gpm_access_list_sync_enabled`, `_last_run_timestamp_seconds`,
  `_last_success_timestamp_seconds`, `_sources` and `_refused_sources`, so the
  staleness alert lives beside the DNS-sync and Ingress-discovery ones instead
  of in a scripted poll of `GET /api/access-list-sources/status`.

### Changed

- **A broken inline error-page template is refused at write time.**
  `settings.errorPages.inline` is parse-checked in `Settings.Validate`, so an
  unparseable template no longer commits cleanly and surfaces only on the next
  restart.
- **A broken error-page template no longer stops gpm from starting.** Startup
  logs a warning and serves gpm's built-in error output instead of calling
  `log.Fatal`; the reload path was already fail-safe. Refusing to boot the whole
  edge over a cosmetic 502 page was the worse failure.
- **Overview's status tile now carries information.** The old "Data plane"
  tile only ever claimed the process was listening, which tells an operator
  nothing when they can already reach the admin panel to look at it. It is
  replaced with an "Upstreams" tile showing healthy vs unhealthy
  upstream-group members from `GET /api/health`. "Control plane" / "data
  plane" wording is also dropped from the Overview intro, Access Logs,
  per-host SSO session UI and their hints in favor of plain descriptions.

### Fixed

- **A rendered error page now carries `X-Content-Type-Options: nosniff`.** The
  built-in plain-text response sets it (via `http.Error`), so *configuring* an
  error page silently dropped a header the default response had, on every tier.

Earlier releases predate the public repository and are not listed.

[Unreleased]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.3.0...HEAD
[1.3.0]: https://github.com/Rake-Pro/go-proxy-manager/releases/tag/v1.3.0
