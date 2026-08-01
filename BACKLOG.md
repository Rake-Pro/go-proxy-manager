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

### Follow-ups from the API-token / DNS-sync review

All findings from that review are remediated in the same change (see CHANGELOG
`Security` and `Changed`); the one deliberately-deferred item is below.

- [x] pprof requires the `admin` scope, not just the admin role. *(High)*
- [x] `PUT /api/settings` requires the `admin` scope (`settings:write` was
  admin-equivalent: env-var exfiltration via `dnsSync`/webhook targets, and
  `adminAuth` rewrite). *(High)*
- [x] Unauthenticated bearer attempts no longer force an uncached full config
  load (token set cached, invalidated from the reload path). *(Medium)*
- [x] Pi-hole sync skips-and-warns instead of shadowing an operator-owned CNAME
  for the same name. *(Medium)*
- [x] Rotation means revocation: `APIToken` revert refused, and the whole-config
  revert preserves `api-tokens`. *(Medium)*
- [x] `tokenHash` is `json:"-"`; `GET /api/backup` raised to `admin` scope (the
  archive is the raw YAML that still carries the digest). *(Low-med)*
- [x] `ProxyHost.dns` is a pointer, so an unset policy is omitted from JSON.
  *(Low)*
- [x] Manual DNS reconcile returns 409 instead of blocking on the run lock.
  *(Low)*
- [x] `apexTarget` change orphans previously-managed records — documented in
  `docs/configuration.md` and `docs/deployment.md`. *(Low)*
- [x] The SPA gates the API Tokens nav entry and page on
  `capabilities.apiTokens.enabled`. *(Low)*
- [x] The API scope gate denies when no principal is on the request. *(Info)*
- [ ] **`Principal.SessionID` is serialised by `GET /api/me`.** The struct has no
  `json:"-"` on `SessionID`, so the SPA's own session id is echoed back into the
  page. Pre-existing, out of scope for the token/DNS change that surfaced it, and
  low impact (the value is already in the caller's own cookie), but it should
  either be tagged `json:"-"` or `/api/me` should serve a purpose-built response
  type. *(Info)*

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
- [x] **Per-object revert.** Revert today is whole-tree: `Store.Revert` →
  `RestoreTree` (`git read-tree --reset -u` + `clean -fd`) resets the entire
  config to the target commit, so reverting one object from its History view
  silently deletes every object created after that commit (bit the operator
  2026-07-16: reverting a proxy host wiped three newer Certificate objects).
  Offer a scoped revert that restores only the selected object's file from the
  target commit (`git restore --source=<hash> -- <rel>` semantics) and commits
  just that change; keep the whole-tree revert as an explicit, clearly-labeled
  separate action.
  *(Done: `Store.RevertObject(kind,name,hash)` restores a single file via
  `git checkout <hash> -- <rel>` — rel derived from the trusted kind mapping, hash
  validated, whole-config re-validated with HEAD rollback on failure, absent-at-
  commit refused (no delete-on-revert); endpoint `POST /api/{kind}/{name}/revert`;
  History view gains "revert this object" and the whole-tree action is relabeled
  "revert entire config".)*
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
- [x] **Domain-group filter chips on the Proxy Hosts list.** Hosts are grouped
  into "zones" derived from their domains (wildcard remainder, or last-two-labels
  of a regular domain) and shown as toggleable chips when 2+ zones exist;
  clicking excludes/includes a zone, composing with the existing text filter.
  Exclusions persist in `localStorage`.

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

## UI polish

- [x] **UI middleware editor: support the `rewrite` type.** The middleware-type
  `<select>` in `internal/ui/static/app.js` now offers `rewrite` with a
  `replacePath` key/value row editor (mirroring the headers map editor),
  validation (absolute paths, key != value), an `mwIcon` entry, and a summary
  chip in the middleware list. An existing `rewrite` middleware loaded from git
  now round-trips correctly instead of being coerced into another type on save.
- [ ] **UI middleware editor: support the `rewrite` type.** The middleware-type
  `<select>` in `internal/ui/static/app.js` (~line 1786) enumerates
  `auth`/`headers`/`guard`/`rate-limit`; the new `rewrite` type is not yet
  offerable, so a rewrite middleware can only be authored via git/API. Add a
  `rewrite` option plus a `replacePath` key/value row editor (mirroring the
  headers map editor) and an icon in `mwIcon`. Until then, per the homelab
  ui-disable-unavailable rule, an existing `rewrite` middleware loaded from git
  should render read-only / clearly flagged rather than silently mis-editing as
  another type.
- [x] **Drop the " admin" suffix from the browser tab title.** The tab should
  read just the app name (default "Go Proxy Manager"), not "Go Proxy Manager
  admin". Two spots: the static fallback `<title>` in
  `internal/ui/static/index.html:6` and the dynamic
  `document.title = s.appName + ' admin'` in `internal/ui/static/app.js`
  (`refreshAppName`, ~line 235). The login page title
  (`internal/server/authhttp.go`, "{{.AppName}} - Sign in") is fine as is.
  (Done: both spots now read the app name with no suffix.)

## Upstream-group follow-ups

- [x] **Per-location upstream group references.** `upstreamGroupRef` on Location
  (mutually exclusive with the location `upstream`), validated like the
  host-level ref, with a per-row group select in the UI.
- [x] **Load-distribution policies.** `policy` on UpstreamGroup: `failover`
  (default), `round-robin` (smooth weighted), `least-connections`
  (in-flight/weight), `ip-hash` (rendezvous, sticky per client IP), plus
  per-upstream `weight`. Unhealthy demotion + connect-only retry unchanged.
- Not planned until a need appears: slow-start after recovery, per-upstream
  max-connections caps.

## DNS sync (phase 2: Kubernetes Ingress discovery)

Phase 1 (Pi-hole + Cloudflare CNAME reconciliation for opted-in proxy hosts) is
✓ shipped — see CHANGELOG `Added` and
[docs/configuration.md](docs/configuration.md#dnssyncsettings-settingsdnssync).
Phase 2 closes the remaining manual step: today a cluster service still has to be
hand-entered as a proxy host before its DNS follows.

- [ ] **Discover cluster Ingresses and reconcile them into managed proxy hosts.**
  Opt-in per Ingress via annotations (never opt-out, and never a namespace-wide
  sweep):

  | Annotation | Value | Meaning |
  |------------|-------|---------|
  | `gpm.rake.pro/managed` | `"true"` | Opt this Ingress into gpm discovery. Anything else (including absent) means gpm ignores it entirely. |
  | `gpm.rake.pro/lan-direct` | `"true"` \| `"false"` | Desired `dns.lanDirect` on the derived proxy host. |
  | `gpm.rake.pro/public-cname` | `"true"` \| `"false"` | Desired `dns.publicCname` on the derived proxy host. |

  The contract is already documented as **reserved/planned** in
  [docs/configuration.md](docs/configuration.md#reserved--planned-kubernetes-ingress-annotations)
  so operators can start labelling and the keys are not claimed for anything else.

  Design constraints, settled up front:
  - **Plain Kubernetes REST, no `client-go`.** The dependency budget is the whole
    point of this project (see CLAUDE.md); `client-go` and its transitive tree
    would dwarf the current direct-dependency set. `GET /apis/networking.k8s.io/v1/ingresses`
    with `?watch=1` (or a poll interval) over the standard in-cluster
    `https://kubernetes.default.svc` endpoint, the projected SA token and the
    projected CA bundle, is enough.
  - **Scoped read-only ServiceAccount.** A ClusterRole with `get`/`list`/`watch`
    on `ingresses` only. gpm must never hold a token that can write to the cluster.
  - **Template-derived, managed-labelled objects.** Each discovered Ingress
    produces a ProxyHost from an operator-supplied template (upstream scheme/port,
    TLS certificate ref, default middleware/access-list chain), carrying a
    `gpm.rake.pro/managed-by: ingress-discovery` label. Reconciliation touches
    **only** labelled objects: a hand-written proxy host with the same name is
    never overwritten or deleted, mirroring the ownership rule the DNS backends
    already use.
  - **Feeds the existing DNS sync.** Discovery sets the `dns` policy on the
    derived hosts and then does nothing else — the phase-1 reconciler publishes
    the records, so there is one code path for DNS, not two.
  - Open questions to resolve in a design doc before implementation: how a
    discovered host picks its certificate (single wildcard ref vs per-host ACME),
    whether writes land as one commit per reconcile or one per object, and what
    happens to managed hosts when the API server is unreachable (freeze, the
    likely answer, rather than delete).

## High availability (gpm itself)

- [ ] **HA support for gpm.** Upstream groups remove the single-node dependency
  *behind* gpm; the gpm instance itself is still a single point of failure. Design
  and ship a supported multi-instance story.
  - [x] **Design doc** resolving the open questions -
    [docs/design/ha.md](docs/design/ha.md): recommends phase-1 active/standby for
    a 2-node homelab (static leader owns ACME + admin writes; follower pulls
    config via `git pull --ff-only` and reads replicated certs; `keepalived` VRRP
    VIP; SSO watermark refresh loop; streams as failover-with-reconnect), with a
    phase-2 sketch (lease-file election, shared bare repo, active/active) and
    explicit non-goals (no etcd/consul/raft, no multi-writer, no live stream-state
    replication).
  - [ ] Implement phase 1 (see the doc's suggested sequencing: SSO watermark
    refresh loop -> leader/follower role gate -> follower config poll loop ->
    deploy doc).

## Roadmap

See [FEATURES.md](FEATURES.md) for P1 (redirect/stream/dead hosts ✓, backup/
restore, rate limiting, access-log viewer, custom timeouts, load balancing), P2
(HTTP/3, hardened TLS, proxy protocol, IPv6, multi-ACME EAB - GeoIP
geoblocking and mTLS client certs phase 1 are now ✓ shipped, see FEATURES.md),
P3 (local-admin passkeys + TOTP for IdP-less deployments), and the "Not planned at this time" list (Brotli/zstd, OCSP,
WAF/CrowdSec, email notifications, SAML/LDAP, PHP/FancyIndex, ECH, ML-KEM,
MPTCP, Anubis, cosign signing).

### Design proposals

- **High availability (gpm itself)** -
  [docs/design/ha.md](docs/design/ha.md) (design complete, implementation not
  started). Phase-1 active/standby for a 2-node homelab; phase-2 sketch for
  automatic election / active/active.
- **HTTP/3** — [docs/design/http3-geoip-mtls.md](docs/design/http3-geoip-mtls.md)
  (not started). **GeoIP geoblocking** and **mTLS client certs (phase 1)**
  from the same document are now ✓ shipped - see FEATURES.md and
  CHANGELOG.md. Remaining mTLS follow-up: **phase 2** (CRL/OCSP revocation,
  identity-passthrough header) - still open, see
  [docs/design/http3-geoip-mtls.md](docs/design/http3-geoip-mtls.md) §1.
