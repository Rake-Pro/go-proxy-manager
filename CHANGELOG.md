# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

- Admin UI: on card views (identity providers, middlewares and the other
  generic object lists) the filter box was a fixed 360px wide, a few pixels
  wider than the card column under it. The toolbar now shares the card
  grid's columns, so the box ends exactly where the first card ends at any
  window width.
- Admin UI: text typed into a chip field (domains, allow-from networks,
  trusted proxies, scopes, tags) but not committed with Enter was silently
  left out of the save. It is now committed when the field loses focus and
  included in every save.
- Admin UI: saving a certificate dropped its display name, labels, tags and
  disabled flag, and rotating an API token dropped its display name, labels
  and tags. Both now carry them through. A location's middleware and
  access-list references are also kept when their list failed to load.
- Admin UI: a zone filter remembered from an earlier visit could hide every
  proxy host with no chip left to clear it. The filter now applies only while
  the zone chips are shown, and zones that no longer exist are forgotten.
- Admin UI: issuing or renewing a client certificate, migrating an access
  list's basic auth and generating a client CA no longer throw away unsaved
  edits on the page without asking.
- Admin UI: navigating quickly between pages could leave the previous page's
  content, or its "Reference list unavailable" banner, on the new page.
- Admin UI: clicking a field's label opened its help popover instead of
  focusing the field. Most field labels are now tied to their controls
  (screen readers announce them), and the "?" sits beside the label. On a
  collapsed section the "?" now appears once the section is opened.
- Admin UI: hints, column headers, section labels and every coloured text
  (links, warnings, error and status chips) are now at least 4.5:1 against
  their background in both themes; the light-theme off switch and keyboard
  focus ring are clearly visible.
- Admin UI: layout fixes - keyboard focus no longer squares off pill switches
  and round buttons; segmented pickers have rounded corners; the issued
  client certificates table no longer overflows its card; inline field rows
  keep the same 14px spacing as other fields; the Settings SSO provider list
  matches the other check lists; discovery upstream fields line up with the
  fields around them; the menu and row-arrow icons have a fixed size; banners,
  nested blocks and Integrations save bars share one style; long domains no
  longer break mid-label in list cards; long log paths are truncated with the
  full path on hover; toasts fit a phone screen; the page title lines up with
  the page content on wide screens and the topbar stays one line on a phone.
- Admin UI: proxy host and certificate rows, and object cards, can now be
  opened from the keyboard; sortable column headers keep their header
  semantics and announce the sort order; the off-screen navigation drawer on
  narrow screens is out of the tab order and closes on Escape; Settings tabs
  support the arrow keys; the current page, segmented pickers and zone chips
  expose their state to screen readers.
- Admin UI: Enter on a card's Clone or Delete button no longer also opens the
  object, and destructive confirm dialogs now open with Cancel focused.
- Admin UI: the hosts bulk-edit bar is greyed out for read-only users, not
  only on HA followers; refreshing or re-rendering the hosts, certificates,
  tokens, access logs and history pages keeps the read-only and maintenance
  banners and gating.
- Admin UI: the "config @" commit badge now updates after a restore or
  revert, and appears after the first write on a fresh install.
- Admin UI: deleting a certificate with unsaved edits no longer raises an
  "unsaved changes" prompt; the new-token card's border uses the accent
  colour; the certificate expiry sort keeps certificates without an expiry
  last in both directions; a failed DNS provider list now also disables the
  certificate editor's Save; the client certificate download is no longer
  cancelled in browsers that start it asynchronously.
- Admin UI: a webhook or notification secret that reads `***` (a redacted
  literal) is refused before saving, and the Settings page explains where to
  fix a refused literal secret it has no field for.

### Changed

- Docs accuracy and structure pass; neutralized wording in test fixtures and
  in failure-mode comments and docs. No behavior change.
- Admin UI: blocking validation messages are shown in the sticky banner at
  the top of the page, like a failed save, instead of a 7-second toast.
- Admin UI: every Integrations save bar is labelled "Save integrations",
  since each one saves every card on the page; all of them are disabled
  while a save is in flight.
- Admin UI: the middleware rate-limit card uses the same per-window /
  per-second form as the host and location rate limits, with free-form
  durations, so a middleware stored with `requestsPerSecond` keeps that form.
  A new rate-limit middleware now defaults to a 1m window (was 1s), as host
  and location rate limits already did.
- Admin UI: the stream, DNS provider, access list and middleware editors use
  the full page width instead of the left half; the Overview "Retry now"
  action, which only opened the certificate, is labelled "Open".
- Admin UI: domains, certificate names, upstreams and other names no longer
  break mid-word in tables and cards. They stay on one line and are cut off
  with "..." only when unusually long, with the full value on hover. Every
  Proxy Hosts row now has the same two lines: the domain, then the
  ingress/docker badge, source and tags.
- Admin UI: quieter lists. Timestamps and numbers are right-aligned, row
  buttons line up in one column, the certificate type and issuance method
  share one column, API tokens show three scopes plus a count (folded lists
  open with a tap or keypress on their "+N"), a commit author's email moves
  to a tooltip, discovery counters are only coloured when non-zero, and
  filter boxes use short placeholders with the full hint on hover.

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
