# Backlog

Outstanding, actionable work. The long-range feature roadmap (P0–P3 tiers) lives
in [FEATURES.md](FEATURES.md); this file tracks concrete near-term tasks.

> Note: items under *Security & hardening* derive from an internal security
> review. Descriptions are kept at the remediation level on purpose. Review this
> section before making the repository public.

## Security & hardening

- [ ] **Normalize request paths before location/guard matching.** Clean
  dot-segments and decide a case policy, then match prefixes on a path boundary
  (`path == prefix` or `prefix + "/"`), and forward the cleaned path. Prevents a
  mismatch between gpm's view of the path and the upstream's. *(High)*
- [ ] **Confine `${FILE:...}` secret resolution** to an allowlisted root (e.g. the
  secrets / cert dirs), rejecting absolute/`..` paths — mirroring the custom-cert
  path guard. *(Medium)*
- [ ] **Harden return-URL handling** (`sanitizeReturnTo`): reject backslash and
  protocol-relative forms, not just `//`. *(Medium)*
- [ ] **Bind the OIDC login flow to the browser**: set a short-lived state cookie
  at login start and verify it at the callback. *(Medium)*
- [ ] **Strip a baseline identity-header denylist** from untrusted peers (e.g.
  `Remote-User`, `X-Forwarded-User`, `X-Auth-Request-*`), not only the headers a
  configured IdP references. *(Medium)*
- [ ] **Scope identity-header trust per host/provider** instead of a global union
  of every provider's trusted CIDRs. *(Medium)*
- [ ] **Bound importer resource use** on large/malformed input (row/object caps,
  map-based dedup instead of linear upsert). *(Medium)*
- [ ] `__Host-` prefix on the session cookie; force `Secure` when an external
  HTTPS base URL is set. *(Low)*
- [ ] Emit baseline security headers (HSTS, `X-Content-Type-Options`,
  frame-ancestors) on admin/login responses. *(Low)*
- [ ] Rate-limit / lockout on local login; cap the pending-login map; sliding
  session expiry (wire the existing `Touch`). *(Low)*

## Functionality gaps

- [ ] **HSTS emission.** `TLSSettings.hsts` is modeled and accepted but the data
  plane never sends a `Strict-Transport-Security` header — implement emission, or
  remove the field until it does.
- [ ] **Per-host OIDC relying-party gating** on the data plane (currently returns
  501 / not implemented; use forward-auth or auth-request meanwhile).
- [ ] **Backup / export / restore** of the whole config (portable archive).
- [ ] **Config history**: a revert endpoint + UI (history is recorded; revert is
  shown disabled).
- [ ] Full field-level forms in the UI for every object kind (some still use a
  labelled raw-JSON editor).
- [ ] Confirm/complete rate-limit middleware enforcement end-to-end.

## Code hygiene

- [ ] Fix the stale middleware-order comment in `internal/dataplane/chain.go`
  (actual order is auth → guard → access-list → headers).
- [ ] Remove or keep-in-sync the unused `router.tlsConfig()` (the server builds an
  equivalent `tls.Config` separately).
- [ ] Consider a TLS 1.3 floor for the public edge (currently 1.2+).

## Roadmap

See [FEATURES.md](FEATURES.md) for P1 (redirect/stream/dead hosts ✓, backup/
restore, rate limiting, access-log viewer, custom timeouts, load balancing), P2
(HTTP/3, Brotli/zstd, OCSP, WAF/CrowdSec, GeoIP, mTLS, IPv6, SAML/LDAP), and P3
tiers.
