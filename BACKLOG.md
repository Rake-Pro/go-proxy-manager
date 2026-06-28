# Backlog

Outstanding, actionable work. The long-range feature roadmap (P0–P3 tiers) lives
in [FEATURES.md](FEATURES.md); this file tracks concrete near-term tasks.

> Note: items under *Security & hardening* derive from an internal security
> review. Descriptions are kept at the remediation level on purpose. Review this
> section before making the repository public.

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

## Code hygiene

- [x] Fix the stale middleware-order comment in `internal/dataplane/chain.go`
  (order is rate-limit → auth → guard → access-list → headers).
- [x] Remove or keep-in-sync the unused `router.tlsConfig()` (the server builds an
  equivalent `tls.Config` separately).
- [x] TLS 1.3 floor: implemented as an opt-in **per-host** `tls.minTLSVersion`
  (`"1.2"` default | `"1.3"`) selected by SNI, rather than a global edge pin that
  would drop older clients.

## Roadmap

See [FEATURES.md](FEATURES.md) for P1 (redirect/stream/dead hosts ✓, backup/
restore, rate limiting, access-log viewer, custom timeouts, load balancing), P2
(HTTP/3, Brotli/zstd, OCSP, WAF/CrowdSec, GeoIP, mTLS, IPv6, SAML/LDAP), and P3
tiers.
