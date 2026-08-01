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
- [ ] **Kubernetes Ingress discovery** against the real cluster: apply
  `deploy/k8s-ingress-discovery-rbac.yaml`, extract the token, enable discovery
  with the wildcard `certificateRef`, annotate one Ingress, and confirm the
  derived host appears, the DNS sync publishes its records, a second reconcile is
  a no-op, and removing the annotation deletes the host in one revertible commit.

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

Phase 1 (Pi-hole + Cloudflare CNAME reconciliation for opted-in proxy hosts) and
phase 2 (Kubernetes Ingress discovery) are both ✓ shipped — see CHANGELOG
`Added`, [docs/configuration.md](docs/configuration.md#ingressdiscoverysettings-settingsingressdiscovery)
and the design record
[docs/design/ingress-discovery.md](docs/design/ingress-discovery.md).

- [x] **Discover cluster Ingresses and reconcile them into managed proxy hosts.**
  Opt-in per Ingress via `gpm.rake.pro/managed: "true"` (never opt-out, never a
  namespace-wide sweep), with `gpm.rake.pro/lan-direct` /
  `gpm.rake.pro/public-cname` setting the derived host's `dns` policy.
  - [x] **Plain Kubernetes REST, no `client-go`** (`internal/k8s`): `net/http` +
    `encoding/json` against `/apis/networking.k8s.io/v1/ingresses`, in-cluster or
    explicit `apiURL`/`tokenFile`/`caFile` (the latter is the real deployment —
    gpm runs off-cluster on the edge host), token re-read on a TTL for projected
    SA rotation, hardened transport, bounded pagination.
  - [x] **Scoped read-only ServiceAccount**: `deploy/k8s-ingress-discovery-rbac.yaml`
    grants `list` on `ingresses` only (the reconciler works from a full list and
    never gets by name or watches). gpm never writes to the cluster.
  - [x] **Template-derived, managed-labelled objects** carrying
    `gpm.rake.pro/managed-by: ingress-discovery`; only those are written or
    deleted, and a name collision with a hand-written host is skipped + warned.
  - [x] **Per-host chains via named profiles** (`ingressDiscovery.profiles`,
    selected with `gpm.rake.pro/profile`). One template only fits a uniform
    fleet; adopting a host with a different chain would drop its
    `sso`/`rate-limit`/login middleware or impose an access list on a host that
    is public on purpose. The annotation carries a **name only** - the Ingress
    author is untrusted, so a manifest picks among operator-authored chains and
    can never name a middleware, access list, certificate or upstream. An
    undefined name skips the Ingress rather than downgrading it to the default.
    `template` stays the default profile, so existing configs are unaffected.
  - [x] **Revocation fails closed.** An Ingress whose profile no longer resolves
    (typo, renamed profile, retired profile, profile rows cleared in the UI) has
    its existing derived host **disabled**, not frozen - otherwise a tenant could
    pin a chain the operator had just tightened simply by naming a profile that
    does not exist. Other derive failures still freeze.
  - [x] **Discovery refs are cross-checked at settings-write time**
    (`IngressDiscoverySettings.ValidateRefs`). One dangling
    certificate/upstreamGroup/middleware/accessList/clientCA name used to pass
    `SaveSettings` and then reject the whole reconcile batch on every poll,
    dropping every unrelated tenant's changes with it.
  - [x] **Domain ownership, not just name ownership** — a derived host whose
    domains are already claimed by a host discovery does not own is skipped and
    reported, and `Config.Validate` rejects two enabled hosts claiming one domain
    whatever wrote them. Ownership is re-checked under the store lock at write
    time via `Store.ApplyBatch`'s `ApplyGuard`, since the plan predates the list.
  - [x] **Feeds the existing DNS sync** — discovery sets the `dns` policy and
    reuses the phase-1 trigger, so there is one DNS code path.
  - [x] Open questions resolved in the design doc:
    - **Certificates**: a single wildcard `certificateRef` from the template;
      discovery never issues per-host ACME (rate-limit blast radius on an
      externally-driven object set, and surprise permanent CT disclosure of
      internal service names).
    - **Commit granularity**: one commit per reconcile via the new
      `Store.ApplyBatch` (the store's `Save` is load-validate-write-commit per
      object; per-object would mean a commit storm plus a reload/webhook/DNS
      trigger apiece, and intermediate revisions nobody wants to revert to).
    - **API server unreachable**: freeze. A managed host is deleted *only* after a
      complete, successful, fully-paginated list that no longer derives it; every
      error path aborts before any write, and the client never returns a partial
      list with a nil error, so "empty" and "failed" are different return shapes.
      The LIST decode asserts `kind: IngressList` and a present `items` field, so a
      `200` from something that is not the API server freezes rather than reading
      as an empty cluster; one list is bounded to two minutes end to end.
  - [x] Poll (configurable `pollInterval`, default 1m, floor 15s) rather than
    watch: a watch means hand-rolling resourceVersion tracking, `410 Gone`
    re-lists and a reconnect loop over a LAN hop, for convergence that is not
    latency-critical. A full-state poll self-heals from a missed event by
    construction.

Deliberately deferred (not planned until a need appears):

- **Per-host ACME** for discovered names outside the wildcard's coverage. Would
  need its own rate-limit budget and a CT-disclosure note in the UI.
- **Watch-based discovery**, if a sub-minute convergence requirement ever appears.
- **Gateway API** (`Gateway`/`HTTPRoute`) as a second discovery source.
- **A dry-run endpoint** (`GET /ingress-discovery/plan`) reporting the diff
  without writing. Cheap to add on top of the existing plan/apply split.
- **Operator-side profile selection** — mapping rules in settings
  (`namespace`/label ⇒ profile) so the `Ingress` selects nothing at all. Strictly
  stronger than the annotation, and the answer to the one residual risk profiles
  carry: **every profile is selectable by every annotating Ingress**, so a tenant
  can pick the most permissive profile you defined. Until this exists, the
  mitigation is documented policy — define only profiles you are willing for any
  tenant to choose (now stated in `docs/configuration.md` and in the settings UI).
  Cost: a settings commit per new service. Named
  profiles are the substrate it would sit on. See
  [design/ingress-discovery.md §5a](docs/design/ingress-discovery.md).
- **Per-profile `allowedDomainSuffixes`**, so a permissive profile can be
  restricted to a subset of the published namespace. The suffix list is currently
  global to the whole discovery block.
- **Live validation** against the real cluster: the subsystem is unit-tested
  hermetically (httptest fake API server) but has not yet run against
  production RBAC and a real Ingress set.

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

- **Kubernetes Ingress discovery** —
  [docs/design/ingress-discovery.md](docs/design/ingress-discovery.md)
  (✓ implemented). Settles the certificate strategy (template wildcard ref, not
  per-host ACME), commit granularity (one per reconcile), freeze-on-error
  semantics, poll-vs-watch, and the Ingress → ProxyHost field mapping for an
  off-cluster gpm.
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
