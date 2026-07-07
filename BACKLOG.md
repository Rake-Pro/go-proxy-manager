# Backlog

Outstanding, actionable work. The long-range feature roadmap (P0–P3 tiers) lives
in [FEATURES.md](FEATURES.md); this file tracks concrete near-term tasks.

> Note: items under *Security & hardening* derive from an internal security
> review. Descriptions are kept at the remediation level on purpose.

## Security & hardening

All items from the internal review are remediated (see CHANGELOG `Security`).

- [x] **Normalize request paths before location/guard matching.** `route()` and
  the guard now match a cleaned (`path.Clean`, RawPath dropped) path on a segment
  boundary (`path == prefix` or `prefix + "/"`); the cleaned path is forwarded
  upstream. *(High)*
- [x] **Confine `${FILE:...}` secret resolution** to an allowlisted root
  (`/run/secrets` by default, override with `GPM_SECRET_FILE_ROOTS`), rejecting
  relative and out-of-root/`..` paths. *(Medium)*
- [x] **Harden return-URL handling** (`sanitizeReturnTo`): rejects backslash,
  protocol-relative, and control-char forms. *(Medium)*
- [x] **Bind the OIDC login flow to the browser**: a short-lived state cookie is
  set at login start and compared (constant-time) at the callback. *(Medium)*
- [x] **Strip a baseline identity-header denylist** from untrusted peers
  (`Remote-User`, `X-Forwarded-User`, the `X-Auth-Request-*` / `X-Authentik-*`
  families, etc.), regardless of which IdP is configured. *(Medium)*
- [x] **Scope identity-header trust per host/provider** — trust is now the union
  of a host's own (and its locations') providers, not a global union. *(Medium)*
- [x] **Bound importer resource use**: a per-table `LIMIT` / row cap
  (`maxImportRows`) fails an over-large source loudly; dedup is map-based. *(Medium)*
- [x] `__Host-` prefix on the session cookie when cookies are `Secure` (gated on
  `GPM_COOKIE_SECURE` so a plain-HTTP admin deployment is not forced onto Secure
  cookies). *(Low)*
- [x] Emit baseline security headers (HSTS, `X-Content-Type-Options`,
  `X-Frame-Options`, CSP `frame-ancestors`, `Referrer-Policy`) on admin/login
  responses. *(Low)*
- [x] Rate-limit / lockout on local login (per client IP); cap the pending-login
  and throttle maps; sliding session expiry (wired `Touch`). *(Low)*

## Functionality gaps

- [x] **HSTS emission.** The data plane now emits `Strict-Transport-Security` on
  HTTPS responses for hosts with `tls.hsts.enabled`.
- [x] **Per-host OIDC relying-party gating** on the data plane: auth mode `oidc`
  makes gpm the relying party (redirect → callback → signed SSO session cookie),
  replacing the prior 501.
- [x] **Backup / export / restore** of the whole config (portable gzip-tar
  archive): `GET /api/backup`, `POST /api/restore`, with UI controls.
- [x] **Config history**: revert endpoint (`POST /api/revert`) + live per-commit
  revert action in the History view.
- [x] Full field-level forms in the UI for every object kind (redirect, stream,
  dead, DNS provider, identity provider, access list, middleware now have typed
  field editors with add/remove rows, enum selects, pickers, and secret-aware
  inputs; the raw-JSON fallback is gone).
- [x] Confirm/complete rate-limit middleware enforcement end-to-end.
- [x] **Per-host no-index toggle** (`robotsNoIndex`): emits
  `X-Robots-Tag: noindex, nofollow` on HTTP/HTTPS; explicit headers-middleware
  value still wins.
- [x] **Host tags** (`tags` on object metadata): chips + filter on the Proxy Hosts
  list.
- [x] **Per-host upstream timeouts** (`timeouts.connectSeconds`/`readSeconds`):
  isolated per-host transport so an override never affects the shared pool.
- [x] **Access-log viewer**: in-memory ring + `GET /api/logs` + "Access Logs"
  view, gated on the access-log toggle (zero overhead when off).
- [x] **Lifecycle webhooks** (`settings.webhooks`): async, best-effort POST per
  config change; optional placeholder-resolved `X-GPM-Webhook-Secret`.

## Security & hardening (post-public follow-ups)

- [x] **Authentik CSRF regression** (from the `d0ceb82` identity-header strip):
  the `X-Authentik-*` prefix strip removed Authentik's own `X-authentik-CSRF`
  token header when Authentik is itself proxied through gpm, breaking every admin
  login with "CSRF token missing". `X-Authentik-Csrf` is now exempt from the strip.

## Live-validation follow-ups

These features are built and unit-tested; they still want an end-to-end check on
the live deployment (the rest of the 2026-06-28 batch was validated live).

- [x] **Stream hosts (TCP/UDP) forwarding** validated end-to-end on the live
  `1b6eeaf` image via the `test/stream/` harness (TCP + UDP echo round-tripped
  from an external client through a published 15432 → gpm forwarder → backend).
- [ ] **Per-host OIDC relying-party gating** against a real Authentik OIDC app
  (register the `/__gpm/oidc/callback` redirect URI, set `GPM_SSO_SIGNING_KEY`,
  walk a host through redirect → callback → session).

## Code hygiene

- [x] Fix the stale middleware-order comment in `internal/dataplane/chain.go`
  (order is rate-limit → access-list → auth → guard → headers; access-list moved
  ahead of auth per GPM-L1).
- [x] Remove or keep-in-sync the unused `router.tlsConfig()` (the server builds an
  equivalent `tls.Config` separately).
- [x] TLS 1.3 floor: implemented as an opt-in **per-host** `tls.minTLSVersion`
  (`"1.2"` default | `"1.3"`) selected by SNI, rather than a global edge pin that
  would drop older clients.

## Roadmap

See [FEATURES.md](FEATURES.md) for P1 (redirect/stream/dead hosts ✓, backup/
restore, rate limiting, access-log viewer, custom timeouts, load balancing), P2
(HTTP/3, hardened TLS, proxy protocol, IPv6, multi-ACME EAB - GeoIP
geoblocking and mTLS client certs phase 1 are now ✓ shipped, see FEATURES.md),
P3 (local-admin passkeys + TOTP for IdP-less deployments), and the "Not planned at this time" list (Brotli/zstd, OCSP,
WAF/CrowdSec, email notifications, SAML/LDAP, PHP/FancyIndex, ECH, ML-KEM,
MPTCP, Anubis, cosign signing).

### Design proposals

- **HTTP/3** — [docs/design/http3-geoip-mtls.md](docs/design/http3-geoip-mtls.md)
  (not started). **GeoIP geoblocking** and **mTLS client certs (phase 1)**
  from the same document are now ✓ shipped - see FEATURES.md and
  CHANGELOG.md. Remaining mTLS follow-up: **phase 2** (CRL/OCSP revocation,
  identity-passthrough header) - still open, see
  [docs/design/http3-geoip-mtls.md](docs/design/http3-geoip-mtls.md) §1.
