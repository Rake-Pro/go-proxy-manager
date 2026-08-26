# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Generate a client CA from the UI/API**: `POST /api/client-cas/{name}/generate`
  ("Generate new CA" on the new-ClientCA page) creates a self-signed RSA-4096 CA
  (`CA:TRUE, pathlen:0`, `certSign|cRLSign`, random serial), writes its private
  key to `<certDir>/client-cas/{name}.key` at `0600`, and saves the ClientCA
  object pointing at it - so a working mTLS setup needs no external tooling.
  Optional `commonName` (defaults to the object name), `validityDays` (1-7300,
  default 3650) and `organization`. Unlike issuance this is a config mutation: it
  commits and appears in history like a `PUT`, and returns the created object. The
  key is never returned or logged; an existing object name is a `409`, as is a key
  file another ClientCA still references (the error names it), with nothing on
  disk changed. An unreferenced key file at that path is reclaimed and logged - it
  can only be residue from a crash or a deleted CA, and refusing it would burn the
  name permanently. A failed config save removes the key it just wrote so the name
  can be retried, and abandoned `.tmp-*` key files older than an hour are swept. Deleting a ClientCA does **not** delete its key file, the
  same way a deleted Certificate keeps its ACME artifacts.
- **Client-certificate issuance from a ClientCA**: an optional signing key
  (`caKeyFile`, cert-store-relative like `crlFile`, or an inline `caKeyPEM`
  secret) turns a verify-only trust anchor into an issuing CA, and
  `POST /api/client-cas/{name}/issue` mints a client certificate signed by it,
  returned as a password-protected PKCS#12 download (`commonName`, `password`,
  optional `validityDays` 1-3650 default 365, optional `sans`). RSA-2048 and the
  legacy PKCS#12 encoder are deliberate: iOS rejects ECDSA client certificates
  and PBES2 bundles, as do several Android/Wear OS releases. The generated
  private key is never persisted or logged - it exists only in the response - and
  the route creates no config revision or history entry. Gated by
  `client-cas:write` like the CA's other writes. New UI card on the ClientCA
  editor, greyed out with the reason when no signing key is configured. New
  dependency: `software.sslmate.com/src/go-pkcs12` (pure Go, no transitive tree
  beyond `golang.org/x/crypto`, which was already direct).
- **Client-certificate expiry warnings and renewal**: every issuance is now
  recorded (CA, common name, SANs, serial, validity - never key material) as
  runtime state under `<certDir>/client-certs/<ca>.json`, written atomically at
  `0600` like the ACME issued-certificate metadata, so records survive a restart
  without entering the git-backed config or creating a revision.
  `GET /api/client-cas/{name}/certificates` lists them with a derived
  `ok`/`expiring`/`expired` status against the CA's new `expiryWarningDays`
  (0-3650, default 30), and the ClientCA page shows a banner before expiry naming
  each certificate and its remaining days.
  `POST /api/client-cas/{name}/certificates/{serial}/renew` reissues the recorded
  identity (same common name and SANs, not accepted from the request) with a new
  key and serial and marks the old record superseded; the UI action is behind an
  explicit confirmation. **Renewing does not revoke** - the old certificate stays
  valid until it expires (revocation is CRL-only), and there is no client-side
  renewal, so every device must import the new `.p12` by hand.
- **`allowFrom` in `client-cert` auth mode**: the same network exemption
  `auth-request` mode has. A client whose resolved IP (trusted-proxy-aware
  `X-Forwarded-For` walk) matches a listed CIDR is proxied straight through with
  no certificate requirement and no `clientCertRoles` check, so a host can run
  `tls.clientAuth.mode: optional` and require certificates from everyone except
  the LAN. Such a request carries no client-certificate identity headers upstream.
  The exemption is matched against the client IP the *host* resolves; a
  client-cert middleware has no identity provider, so it contributes no trusted
  proxies and on a pure client-cert host the comparison is against the raw TCP
  peer. docs/configuration.md carries the warning: gpm must be the edge, or an L7
  proxy whose address falls inside an `allowFrom` network exempts all traffic
  through it.
- **Prometheus metrics** (opt-in): `GET /metrics` on the admin listener,
  enabled with `-metrics` / `GPM_METRICS=1` (404 when off), gated by admin
  role plus a dedicated `metrics:read` API-token scope. Text exposition from a
  small internal package (`internal/metrics`), no new dependency. Per-host
  request counts / latency histogram / bytes / in-flight, upstream errors,
  websocket upgrades, denials by tier, stream connections, ACME expiry and
  renew failures, DNS-sync and Ingress-discovery timestamps, HA role, build
  info, Go runtime. Host labels are operator object names, never client
  `Host` headers; every metric caps its series count. `dnsSync` status now
  reports `lastSuccess`.
- **Bouncer middleware (WAF/CrowdSec deny hook)**: new `bouncer` middleware
  type, hook-only. Providers `crowdsec` (LAPI flow; `ban`/`captcha` deny;
  optional `stream: true` keeps decisions in memory with CIDR support) and
  `http` (generic 2xx=allow / 403=deny). Runs after the access list and
  before auth; `allowFrom` bypass; `onError: fail-open|fail-closed` (default
  fail-open); bounded per-IP verdict cache, error verdicts capped at 5s.
- **Inbound PROXY protocol** v1 + v2 via `settings.proxyProtocol` (`enabled`,
  `trustedCIDRs` required, `timeout` 5s) on the :80/:443 and TCP stream
  listeners. Honoured only from a trusted peer; the parsed source replaces
  the connection address so every IP-based control sees the real client.
  Config is read live per connection.
- **Stream TLS/SNI routing**: `StreamHost.tls.mode: passthrough|terminate`
  with `sniMatch` and (terminate) `certificateRef`; several hosts may share
  one TCP port when all are SNI-routed.
- **L4 access lists on stream hosts** (`accessLists`, IP/CIDR + geo),
  evaluated before any backend dial.
- **Per-host gzip compression** (`compression`: `enabled`, `minBytes` 1024,
  `types`), stdlib only, Accept-Encoding aware, skips upgrades/event-streams/
  already-encoded responses, sets `Vary`.
- **Custom error pages** (`settings.errorPages`, per-host `errorPages`:
  `dir`, `inline`, `interceptUpstream`) for gpm-generated errors (upstream
  unreachable, denied, rate-limited, dangling reference, parked host), rendered
  through html/template.
- Web UI: **Clone** action on every object kind (name/secrets cleared,
  domains cleared for host-like kinds); **light theme** (auto/light/dark
  toggle in the top bar, persisted, applied before first paint); login page
  follows the OS preference. Stream editor gains TLS/SNI/cert/ACL fields;
  Settings gains PROXY protocol, error pages and metrics cards.
- IPv6: dual-stack binds verified by test; `IPv6` subsection in
  docs/deployment.md for the Docker-side requirements.
- **ACME HTTP-01 challenge support**: `certificate.acme.challenge: http-01`
  issues without any DNS credential; the data plane's plaintext `:80` listener
  serves `/.well-known/acme-challenge/<token>` ahead of host routing, the
  force-SSL redirect and auth. `dns-01` stays the default for certificates that
  reference a DNS provider and remains required for wildcards. Unknown tokens
  fall through to normal routing so an upstream's own ACME client keeps working.
- **ACME External Account Binding** (`acme.eab.kid` + `acme.eab.hmacKey`) for
  CAs that require it (ZeroSSL, Google Public CA examples in docs); EAB
  accounts get their own account key per key id.
- **Three more DNS-01 providers** on their plain REST APIs, no new
  dependencies: `digitalocean`, `hetzner`, `desec`.
- **`ingressDiscovery.annotationPrefix`** makes the Kubernetes Ingress
  discovery annotation/label prefix configurable (default `gpm.rake.pro`,
  unchanged for existing deployments). Changing it while hosts are labelled
  under the old prefix is refused at settings-write time unless
  `ingressDiscovery.annotationPrefixMigrate: true`, which relabels them in the
  next reconcile's single commit.
- **OpenAPI 3.1 spec** at `docs/api/openapi.yaml` (102 operations), served at
  `GET /api/openapi.yaml`, with a route-coverage test.
- Admin UI: per-location middleware and access-list pickers in the host
  editor; certificate editor gains a challenge selector (dns-01 greyed until a
  DNS provider exists) and EAB fields; DNS provider editor offers all four
  providers.
- Docs: bare-metal/systemd deployment, backup/restore, upgrade/rollback,
  image signature verification, and a "Users, roles and audit" stance section.

### Changed

- **ClientCA screen rework.** The new/edit page was inconsistent - some fields
  carried three-line explanations and some none, and each either/or pair (CRL
  file vs inline, CA key file vs inline) rendered as two stacked controls the
  operator had to know were mutually exclusive. Now: one field pattern
  everywhere (label, control, at most one hint line), each either/or pair is a
  single segmented picker over one control, and Revocation and Signing key are
  collapsed sections showing a one-line summary of what is configured, opening
  automatically when they hold values. The multi-sentence helper prose moved to
  docs/configuration.md. Gating, the expiry banner, the superseded-row treatment
  and follower read-only gating are unchanged, but the **save semantics of the
  two either/or pairs did change**: a save now submits the selected side when it
  holds a value and otherwise preserves whatever the unselected side already
  held, so toggling a picker to look at the other option and saving is a
  byte-for-byte no-op. Clearing the visible field still removes the value. On the
  new-CA page the Revocation and Signing key sections are hidden while "Generate
  new CA" is selected, since `POST /generate` does not accept them, and cloning a
  ClientCA lands on "Paste existing CA" because the clone already carries a
  certificate.
- `auth.allowFrom` is now **refused at validation** in `oidc` and `forward-auth`
  mode, including when `mode` is unset and the referenced provider's `type`
  resolves to one of them (or cannot be resolved at all). Those gates have no
  network bypass, so the value was silently ignored; it was already documented as
  unsupported there. `auth-request` and `client-cert` are unaffected.
- Client-certificate issuance now enforces a **12-character minimum bundle
  password** on both issue and renew, and refuses non-ASCII or control characters
  in `sans` with `400` instead of failing inside ASN.1 encoding. The password floor
  exists because the legacy PKCS#12 encoder (kept for iOS/Android import) derives
  its integrity MAC with a single KDF iteration, so the password is effectively the
  bundle's only at-rest protection.
- Renewing an already-superseded certificate record is refused with `409` naming
  the superseding serial, and the ledger's supersede write now errors instead of
  silently appending when its target is absent. Either would otherwise leave two
  records looking current.
- API Tokens page: the scope table no longer scrolls inside the card, and
  checking write simply auto-selects read instead of greying it out.
- **BREAKING - two renames, neither migrated automatically.** `DeadHost` is now
  `ParkedHost` (`config/dead-hosts/` -> `config/parked-hosts/`, `/api/dead-hosts`
  -> `/api/parked-hosts`, scope subject `dead-hosts` -> `parked-hosts`,
  `deadHosts` -> `parkedHosts`), and a stream host's `forwardHost`/`forwardPort`
  are now a single `target: {host, port}`. Migration, before starting the new
  binary: (1) `git mv config/dead-hosts config/parked-hosts` and commit -
  startup and reload fail with that command in the error while the old directory
  still holds objects, because gpm will not author a commit in your config repo;
  (2) rewrite every `config/stream-hosts/*.yaml` `forwardHost`/`forwardPort` pair
  as `target`, and update any API token naming a `dead-hosts:*` scope. A file
  still using the old stream keys is rejected at load rather than accepted with
  no backend; a backup archive taken before the rename still restores, its
  `dead-hosts/` entries mapped onto `parked-hosts/` one-way at restore time.
- API Tokens page: the scope picker is a compact grouped table (Hosts / Trust
  and auth / Routing / Operations) with header select-all toggles and a locked
  read box when write is checked, replacing the card grid; `metrics` no longer
  offers a write box (`metrics:write` does not exist); the token list is a
  table with a Last used column; the top-bar identity block ellipsizes instead
  of overflowing; the sidebar "data plane: live" indicator is gone.
- Public-release prep: example zones, fleet tables and test fixtures use
  `example.com` / `proxy-admins`; status glyphs replaced with plain text;
  "Relationship to Nginx Proxy Manager" section added to docs/architecture.md.
- Reverse proxy: upstream timeouts now respond 504 Gateway Timeout instead of
  502 (connect/reset failures remain 502).
- Startup logs a warning naming `GPM_LOCAL_ADMIN_USER` /
  `GPM_LOCAL_ADMIN_PASSWORD_HASH` and `settings.adminAuth.providers` when
  neither is configured (the admin panel was previously unreachable with no
  diagnostic).
- CI: every third-party GitHub Action is pinned to a full commit SHA;
  `staticcheck` and `govulncheck` are required jobs (`make lint`, `make vuln`);
  the Trivy gate now fails on HIGH as well as CRITICAL.
- FEATURES.md rewritten as a status board; CHANGELOG restructured into
  per-version sections; `docs/deployment.md` documents `GPM_GEOIP_DB` and the
  absence of a `/metrics` endpoint.

### Fixed

- API tokens minted before the ParkedHost rename that carry `dead-hosts:read` /
  `dead-hosts:write` scopes load again and grant the equivalent `parked-hosts`
  scope; 1.0.18 refused the whole config (and crash-looped the edge) on such a
  token. Rotate them at leisure; no action is required to start.
- Admin UI: saving a proxy host with `locations` no longer drops each
  location's `middlewares` / `accessLists` when the host was authored outside
  the UI; the save merges over the loaded location by path, mirroring the
  `tls.clientAuth` merge.
- Certificate type "Custom (upload)" relabelled "Custom (file on server)" with
  the cert-store-relative path convention (there is no upload endpoint).
- Dead `ghcr.io/rake-pro/go-proxy-manager:main` image references and a dead
  runbook link in `deploy/compose.parallel.yaml`.

### Security

- A PROXY header from a peer outside `trustedCIDRs` is treated as payload and
  the peer address stands (warned once per peer); a malformed header closes
  the connection; a stalled one is cut at `timeout`.
- Config validation rejects a stream host referencing an access list with
  `basicAuth` users, `tls` on a udp/both stream, port sharing unless all hosts
  are SNI-routed, and duplicate SNI claims; a stream host whose access list or
  certificate cannot be resolved is dropped rather than served ungated.
- Released images are signed keylessly with cosign via GitHub Actions OIDC.
- Data-plane OIDC gate: a request whose `Host` is not a configured domain of
  the gated host is refused (404) instead of minting and caching a per-Host
  relying-party client with live IdP discovery. Discovery runs outside the
  gate's mutex with single-flight per domain; the client cache is bounded.
- HA follower: the config pull has a 60s timeout, the network fetch runs
  without the config write lock (only the fast-forward takes it), and
  `GIT_TERMINAL_PROMPT=0` is in force.
- Data-plane listeners set `ReadTimeout` (60s) and `IdleTimeout` (90s); upgrades
  and body-bearing proxied requests clear their deadlines, so websockets,
  streams and large uploads are never truncated.
- Data-plane basic auth: failed attempts are throttled per client IP (5 / 15
  min, bounded map that fails closed when saturated) and concurrent bcrypt
  verifications are bounded process-wide. A locked-out client receives the
  same 401 as a wrong password.
- Reverse proxy re-asserts gpm's own identity headers on the outbound request;
  a client `Connection: X-Forwarded-User` previously had the header removed by
  the hop-by-hop purge.
- Guard middleware: a guard matching on `queryEquals` rejects (400) a request
  whose raw query contains `;` rather than evaluating a query gpm and the
  upstream may parse differently.
- Cookie MACs are domain-separated (SSO session, SSO login-state, sticky
  cookie). **Existing SSO sessions and sticky assignments are invalidated once
  on upgrade.**
- `Principal.SessionID` is no longer serialised by `GET /api/me` (landed in
  1.0.16, recorded here for visibility).
- Test coverage for the admin login flow: `internal/server` 44% -> 91%,
  `internal/auth` 64% -> 93%.

## [1.0.16] - 2026-08-22

### Added

- **High availability, phase 1** (active/standby pair, no new dependency):
  `GPM_HA_ROLE=leader|follower` (default `leader`, unchanged single-node
  behaviour) designates the single writer. A follower runs no ACME renewal and
  no Ingress-discovery loop, refuses every admin/API config write with a `503`
  naming the leader, and pulls the leader's config repo with
  `git pull --ff-only` every `GPM_HA_POLL_INTERVAL` (default 20s), reloading
  only when HEAD moved and never merging, rebasing or resetting a diverged
  repo. `GET /api/capabilities` reports `ha.role` / `ha.readOnly` and the admin
  UI greys out write controls on a follower. Every instance re-reads the
  persisted SSO revocation watermark on a ticker and advances it
  monotonically, so a revoke takes effect within one interval instead of at
  the next restart. Deployment recipe in `docs/ha.md`.
- **mTLS phase 2: client-certificate revocation.** `ClientCA` gains `crlFile`
  (PEM or DER, confined relative to the cert store), `crlPEM` (inline) and
  `crlPolicy` (`fail-closed` default | `fail-open`). Revoked serials are
  rejected at the TLS handshake via `VerifyPeerCertificate`, the CRL signature
  is validated against the CA bundle, and `nextUpdate` is honoured. CRLs
  reload on the config-reload path and on a 5-minute mtime watch.
- **mTLS phase 2: identity passthrough.** `tls.clientAuth.identityHeaders`
  forwards the verified certificate's subject (`X-Client-Cert-Subject`, name
  overridable) plus opt-in `san` / `serial` / `fingerprint` headers. All four
  default names joined the baseline identity denylist, so they are stripped
  from untrusted peers whether or not a host enables passthrough.
- **mTLS phase 2: `client-cert` auth-middleware mode**, gating on a verified
  client certificate with an optional `clientCertRoles` subject-to-role mapping
  (no identity provider). "cert OR SSO" = `tls.clientAuth.mode: optional` plus
  this middleware as the host's auth tier.
- Admin UI: Client CAs section with the CRL fields, mTLS passthrough controls
  in the host TLS editor, `client-cert` mode in the middleware auth editor.
- `GET /api/ingress-discovery/plan` dry-run endpoint, mirroring
  `GET /api/dns-sync/plan`; wired into the settings UI as *Preview changes*.
- Operator-side Ingress-discovery profile selection
  (`ingressDiscovery.profileRules`, `profileSelection: "rules-only"`),
  evaluated before the `gpm.rake.pro/profile` annotation.
- Per-profile `allowedDomainSuffixes` narrowing the global Ingress-discovery
  suffix list, validated as a subset at settings-write time.

### Fixed

- A discovery-managed proxy host disabled by an operator is no longer
  re-enabled by the next poll; discovery only clears a `disabled` state it set
  itself (unresolvable-profile hold), tracked via the `gpm.rake.pro/disabled-by`
  label.
- Admin UI: saving a proxy host no longer drops a GitOps-authored
  `tls.clientAuth` block.

### Security

- `Principal.SessionID` is no longer serialised by `GET /api/me`.

## [1.0.15] - 2026-08-21

### Changed

- Go toolchain 1.27rc2 -> 1.27.0

## [1.0.13] - 2026-08-16

### Changed

- **The host editor's middleware / access-list pickers show attached entries
  first and grew from ~7 to ~18 visible rows.** Both check-lists rendered in
  collection (file) order inside a 220px scroll box, so an attached entry
  sorting past the cutoff — `sso` sorts *after* `sso-lan`, `-` < `.` in file
  names — was clipped out of view behind an overlay scrollbar and the host
  looked unprotected in the UI while the data plane was enforcing the chain
  all along. Checked entries now render at the top in the host's stored chain
  order (which the DOM-order save path then preserves on re-save), and the
  list cap is 560px. Display-only; no host was ever actually missing its
  middleware.

## [1.0.10] - 2026-08-01

### Added

- **Discovery templates and profiles reach parity with a hand-written proxy
  host: `robotsNoIndex`, `timeouts` and `tags`.** `IngressHostTemplate` carried
  the upstream, TLS, websockets, middlewares, access lists and default DNS policy
  and nothing else, so a service cut over to Ingress discovery **silently lost**
  its `robotsNoIndex` — the derived host had no way to express a field the model
  already has. The workaround, a `headers` middleware setting `X-Robots-Tag`, is
  a second mechanism for the same thing and has to be remembered per host; it was
  reverted in production, and `robotsNoIndex` was simply off for derived hosts
  until this.

  All three are applied verbatim to every derived host, and because a profile
  *is* an `IngressHostTemplate`, profiles get them with no extra plumbing (tested,
  not assumed). `timeouts` is validated by the **same helper** `ProxyHost.Validate`
  uses, at settings-write time — a template that would produce a host the config
  validator rejects fails the operator's own write instead of every tenant's
  reconcile batch. `timeouts` is a pointer and `tags` a slice, so a template that
  sets neither still produces exactly the object it did before (no `timeouts: {}`
  in the YAML). Both, like `middlewares`/`accessLists`/`defaultDNS`/
  `tls.clientAuth`, are **deep-copied** per host: no derived host shares backing
  memory with another or with the loaded settings.

  `locations` is **deliberately not** a template field, recorded as a decision in
  `docs/design/ingress-discovery.md` §5 and `BACKLOG.md` rather than left as a
  silent omission: locations are per-service path routing, and discovery forwards
  everything to the cluster ingress controller by vhost so the controller can do
  that routing from the same `Ingress`. `displayName` stays derived as
  `<namespace>/<name>`. The Settings UI renders all three new fields on the
  default template and on every profile row, and the save path **merges** the
  nested `timeouts` object over what was loaded rather than rebuilding it — the
  guard test in `internal/ui/settingsmerge_test.go` now covers the new fields, so
  the rebuild-instead-of-merge regression (which has already stripped
  `clientAuth`/`hsts`/`minTLSVersion` once) fails CI.

## [1.0.9] - 2026-07-31

### Added

- **`GET /api/dns-sync/plan` — dry run before you enable.** Reads both backends and
  the ownership ledger and reports exactly what a reconcile would create, adopt,
  retarget, delete and skip, plus how many records it would leave alone, without
  issuing a single write. Scope `dns-sync:read`; `409 Conflict` while a reconcile
  is in flight, for the same reason the manual reconcile refuses to queue. Wired
  into the settings UI as **Preview changes**, next to *Reconcile now*. The
  2026-08-01 incident was unpreviewable — the only way to learn what the first
  reconcile would do was to run it, and by then the records were gone.
- **Ingress discovery: operator-defined named profiles, selected per Ingress.**
  `settings.ingressDiscovery.profiles` is a map of named chains, each with the
  same shape and the same validation as `template`; an `Ingress` selects one with
  `gpm.rake.pro/profile: "<name>"`. Discovery previously derived every host from
  the single `template`, so it could only adopt the uniform tail of a fleet -
  publishing anything else would silently **drop** its `sso`/`rate-limit`/login
  middleware or **impose** an access list on a host that is public on purpose,
  both security-relevant regressions that leave the host serving under a chain
  nobody chose. `template` is unchanged and now acts as the default profile, so
  existing configs and existing annotated Ingresses behave identically (covered
  by a regression test).

  **The annotation carries a name and nothing else, and that is the security
  constraint.** An Ingress author is untrusted - in a shared cluster a tenant may
  be able to create or edit an `Ingress`, and gpm sits at the edge in front of
  everything - so there is deliberately **no** annotation that lets a manifest
  name a middleware, an access list, a certificate or an upstream. Such an
  annotation would be a self-service privilege grant (`access-lists: ""` on your
  own namespace's Ingress removes `home-vpn` from a hostname at the edge). Every
  profile is authored by the operator in the config repo; a manifest chooses
  among sanctioned chains and can never invent one, nor end up weaker than
  something the operator explicitly allowed. An **undefined** profile name
  **skips** the Ingress - never a silent fall back to the default, never a
  partial chain - and if that Ingress already has a derived host, the host is
  **disabled** rather than left alone, so a retired or tightened profile cannot
  be pinned (see Security below). Matching is
  exact (no prefix match, no case folding); a profile is applied verbatim rather
  than merged with the default, so the default's access list cannot leak onto a
  deliberately-public profile. Every profile validates at settings-**write** time
  (`certificateRef` required, `upstream` XOR `upstreamGroupRef`, name-checked
  middleware/access-list refs), so an invalid one is rejected where an operator
  sees it rather than surfacing later as a skipped host.
  `GET /ingress-discovery/status` now reports the resolved `profile` per host
  (the literal `template` for the default block) as the audit trail for what
  chain a given Ingress actually got, and the settings UI grows a profile editor
  plus the resolved profile in its status panel. Threat model and resolution
  rules in
  [docs/design/ingress-discovery.md §5a](docs/design/ingress-discovery.md);
  schema and a worked example in
  [docs/configuration.md](docs/configuration.md#discovery-profiles).

### Changed

- **The per-host "LAN direct" / "Public CNAME" toggles stay usable when their DNS
  backend is not configured.** They were greyed out via `gateControl`; they now
  render an inline note instead ("Pi-hole DNS sync is not configured yet - this
  will take effect when it is"). Setting the flag ahead of the backend is
  legitimate staging - the host is the declaration, the syncer publishes when it
  is wired - not an error to refuse, and the capability probe is cached per page
  load, so a stale "not configured" could otherwise outlive the fact and block a
  valid edit. Every other capability-gated control still greys out.
- **`GET /api/dns-sync/status` reports adoption.** Each backend now carries
  `adopted`, `retargeted`, `skipped` and `untouched` alongside `created` and
  `deleted`. `managed` changes meaning from "records whose target matched the
  apex" to "records gpm owns" (the ledger entry count after the run). `untouched`
  is the number to check after a first enable: it should equal everything you
  maintain by hand on that backend.
- **Changing `apexTarget` no longer orphans records.** Previously ownership *was*
  target equality, so moving the apex made every record gpm had created
  unrecognisable to it — never updated, never deleted, and never recreated either
  because the name now conflicted. With the ledger, a record gpm created and
  nobody has touched since is still identifiably gpm's, so it is retargeted on the
  next reconcile. The manual-cleanup warning in `docs/configuration.md` and
  `docs/deployment.md` is retired.

### Fixed

- **A record gpm ADOPTED is never deleted — it is released.** Adoption used to be
  a one-way trap. An operator hand-writes `x.example.com`; a proxy host is later
  given `dns.lanDirect` for that name, so the reconcile adopts the existing record
  into the ownership ledger; the operator later takes the flag off again — and the
  next reconcile **deleted their record**, because the ledger no longer
  distinguished a record gpm made from one it had merely claimed. On Pi-hole,
  where every correctly-targeted record is adoptable, that was the 2026-08-01
  incident deferred by one config edit.

  **The guarantee now: an adopted DNS record is never deleted *or* retargeted -
  gpm destroys only records it CREATED.** Ledger entries
  record their provenance (`adopted: true|false`), and an adopted entry the config
  no longer wants is *released* — dropped from the ledger with a warning, with the
  record left exactly where it stands. Deletion remains available for records gpm
  created itself. Existing ledgers upgrade safely: an entry written before the
  field existed carries no provenance, and a missing `adopted` is read as
  **adopted**, the only reading that cannot destroy a record on upgrade. The
  trade-off is stated in `docs/configuration.md`: gpm will not clean up an adopted
  record for you.
- **An `apexTarget` change no longer deletes the records gpm only ADOPTED.** A
  retarget is a delete followed by a create, and the retarget branch did not look
  at the claim's provenance: after the edge host moved, a record an operator had
  hand-written and gpm had merely adopted was **destroyed and recreated**, and the
  replacement was recorded as `adopted: false`. Two failures in one - an
  operator-authored record deleted by the subsystem that exists to prevent exactly
  that, and a claim silently upgraded from adopted to created, so a *later* host
  removal would hard-delete the name for good. Adopt -> change `apexTarget` ->
  remove the host reproduced the 2026-08-01 incident one record at a time.

  An adopted claim whose record no longer matches the apex is now **released**:
  dropped from the ledger with a warning and counted as a skip, with nothing
  written to the backend. Retarget stays available for records gpm created itself,
  which it may safely replace. Provenance is carried forward unchanged everywhere
  else; the single place a claim may become "created" is a record that has gone
  and is genuinely re-created by gpm. To move an adopted name to a new apex,
  delete or re-point the record by hand (`docs/configuration.md`).
- **Pi-hole sessions leaked whenever a run was cut short.** `logout` ran on the
  caller's context, so an HTTP client disconnecting mid-reconcile cancelled the
  logout along with the run (measurably: one login, zero logouts). Pi-hole has a
  small fixed session pool, so a leak per aborted run eventually locks the
  operator out of their own admin UI. Logout now runs on a detached context with a
  5s deadline.
- **A failed retarget no longer destroys the record.** Neither backend can update
  a CNAME in place, so a retarget is delete-then-create; a create that failed left
  the name unresolved until some later reconcile happened to heal it, while the
  status reported `Deleted:0 Retargeted:0` — a run that destroyed something and
  said nothing had happened. The original record is now restored (Cloudflare
  included, orange-cloud flag and all), the run fails loudly, and the counter is
  incremented as soon as the *delete* lands.
- **A Pi-hole API shape change is an error, not a silent ledger wipe.** A renamed
  or missing `config.dns.cnameRecords` field decoded to a nil slice, which a
  full-state reconciler reads as "the resolver holds nothing" — status OK, zero
  counters, ledger emptied and committed. The field is now required to be present
  and a list; anything else fails the run with the ledger intact.
- **Cloudflare pagination no longer truncates when `result_info` is absent.** A
  full 100-record page with no (or a zeroed) `result_info` stopped the walk at
  page 1, hiding the rest of the zone: no false deletes, but orphaned ledger
  entries and repeated creates of records that already exist. Termination is now
  driven by a short page; `result_info` is advisory.
- **A reconcile can no longer clobber a concurrent revert of the ownership
  ledger.** The reconcile's read-modify-write spans minutes of backend I/O, and
  `Revert` rewrites `dns-ledger.yaml` with the rest of the tree; a reconcile that
  had loaded the ledger first would write its own version back afterwards,
  re-establishing claims the revert withdrew — and a claim authorises a deletion.
  `SaveDNSLedger` now takes the repo revision the ledger was read at and refuses a
  stale write (`ErrLedgerStale`); the reconciler re-reads and rewrites *without*
  the withdrawn claims rather than resurrecting them.
- **A revert can still restore an ownership claim reality has moved past** (gpm
  created a record, deleted it, an operator recreated it by hand, the config is
  reverted to before the deletion). This is documented prominently beside the
  existing revert note in `docs/configuration.md`, and every deletion is now
  logged at **warn** with the ledger revision that authorised it, so a record
  removed on the strength of a stale claim is identifiable after the fact.
- **Ledger duplicate-domain validation is case-insensitive**, matching the
  normalised form the reconciler indexes by. `Foo.lan` and `foo.lan` could both
  validate, leaving one claim silently shadowing the other.
- **The ledger commit survives a cancelled request.** The reconciler's ledger save
  passed the (possibly request-scoped) context to `git.CommitAll`, so a cancelled
  reconcile could leave the file written but uncommitted, to be swept into an
  unrelated commit later. It now writes on a detached context.
- **DNS sync deleted 19 records it did not create. It can no longer delete
  anything it did not create.** On 2026-08-01 an operator enabled
  `settings.dnsSync.pihole` for the first time. Their Pi-hole already held 19
  hand-written LAN CNAMEs (their LAN-direct bypass list: `plex`, `argo`, `cloud`,
  `wiki`, ...) pointing at the same edge host they had just configured as
  `apexTarget`. No proxy host carried `dns.lanDirect` yet, so the desired set was
  empty. The Pi-hole backend treated *any* CNAME whose target equalled
  `apexTarget` as gpm-managed, found all 19 unwanted, and deleted every one on the
  very first reconcile. LAN DNS broke until they were restored by hand.

  The bug was the ownership test. Pi-hole/dnsmasq CNAMEs have no comment field, so
  target equality was used as a stand-in for "gpm created this" — and on a shared
  apex that is not ownership at all, it is a coincidence. (The Cloudflare backend
  was safe in the identical situation only because its marker, the
  `managed-by:gpm` record comment, is one pre-existing records do not carry.)

  **The guarantee now: gpm deletes only DNS records it recorded creating.**
  Ownership is written down, not inferred, in a new git-backed ledger
  (`model.DNSLedger`, the singleton `config/dns-ledger.yaml`). A record absent from
  that ledger is never in a delete list, whatever its name and whatever it points
  at. `apexTarget` says where managed records point and nothing more.

  What a reconcile does per desired name: **create** what is absent (recording
  ownership); **adopt** a record that already holds the right target but predates
  the ledger — claimed, not recreated, logged at info; **retarget** a record that
  still holds exactly what gpm wrote after `apexTarget` moved; **skip and warn** on
  a name held by a record gpm does not own (unchanged — never shadowed, never
  replaced). It **deletes** only ledger entries the config no longer wants, and
  only while the record still matches what gpm left there; re-pointed out of band,
  it is disowned rather than deleted.

  **Migration for existing deployments is a no-op plus adoptions.** An empty ledger
  means gpm owns nothing, so a first reconcile can only create and adopt: records
  matching the desired set are adopted, every other record on the backend is left
  exactly as it is, and the run logs a one-line summary of both counts. Nothing has
  to be done before upgrading, and no ledger file has to be seeded.

  Cloudflare is held to the same discipline: the ledger is authoritative for
  deletion and the `managed-by:gpm` comment remains an independent second
  condition (adoption there requires the comment too), so none of that backend's
  existing guarantees are weakened.

  Regression coverage names the incident directly: pre-existing records pointing
  at `apexTarget`, an empty desired set and an empty ledger must produce **zero**
  deletions, on both backends.
- **One dangling reference in `ingressDiscovery` could wedge the entire
  reconcile, forever.** `Settings.Validate` checks only the *shape* of the names
  a template or profile carries, so a `certificateRef` / `upstreamGroupRef` /
  middleware / access list / `clientAuth.caRef` naming an object that does not
  exist passed `SaveSettings` - and then failed at `merged.Validate()` in
  `ApplyBatch`, which rejects the **whole batch**. Every other tenant's create,
  update and delete was therefore dropped on every poll, indefinitely, surfacing
  only as an opaque batch-validation error in the reconcile status.
  `SaveSettings` now cross-checks the template and every profile against the
  loaded config (`IngressDiscoverySettings.ValidateRefs`), so the error lands on
  the operator's own write with the offending name in it. A **disabled**
  discovery block is not cross-checked, so a half-filled draft still never blocks
  an unrelated settings write.
- **The residual risk of profiles is now documented where operators read it.**
  `docs/configuration.md` and the settings UI both claimed a manifest "can never
  produce a host weaker than something you explicitly sanctioned" while the
  worked example shipped a `public-ratelimited` profile with no access list. Both
  now say the operative rule plainly: every profile is selectable by every
  annotating Ingress, so define only profiles you are willing for any cluster
  tenant to choose.

### Security

- **Ingress discovery: revoking a profile now actually revokes it.** An annotated
  Ingress whose profile does not resolve used to be treated like any other derive
  failure - protected from deletion and otherwise untouched - which meant the
  security property held for *creating* a host but not for *revoking* one. An
  operator who tightened a profile (added `sso`, added an access list), renamed
  it, or retired it could be defeated by a tenant flipping the annotation to a
  name that does not exist: no upsert, no delete, and the live host kept serving
  the pre-tightening, unauthenticated chain indefinitely. Deleting every profile
  row in the settings UI froze every derived host the same way. Discovery now
  plans an update setting `disabled: true` on the existing managed host instead -
  fails closed, destroys nothing, and re-adding the profile re-enables it on the
  next reconcile. Every **other** derive failure (malformed hostname, unusable
  derived name) still freezes the existing host: the operator's policy has not
  changed there, and failing closed would let any tenant take their own service
  offline with a one-character manifest edit.
- **A settings save no longer strips mTLS from discovery profiles or the
  template.** The settings form rebuilt `tls` from exactly the three fields it
  renders (`certificateRef`, `forceSSL`, `http2`), and a settings write is a full
  replacement - so a template or profile authored in YAML/GitOps with
  `tls.clientAuth: {caRef: …, mode: require}` silently lost its
  client-certificate requirement (and its `minTLSVersion` and `hsts`) on **any**
  unrelated Settings save, after which the next reconcile pushed the downgrade
  onto every derived host. The handler now merges over the loaded object. The
  template block had this bug since it shipped; profiles inherited it. A guard
  test in `internal/ui` fails if the merge is ever rebuilt away.
- **The data plane fails closed on an unresolvable middleware or access-list
  name.** `buildChain` skipped any name it could not resolve, so a typo in a
  host's `accessLists` turned a restricted host into an open one, with
  `Config.Validate`'s reference check the single thing standing between the two.
  An unresolvable reference now replaces that host's chain with a `503` and logs
  at `error`. Scoped to the one host on purpose: a config that cannot pass
  validation anyway must not take unrelated hosts down as a side effect.
- **Derived hosts no longer share one `*ClientAuth` with the settings object.**
  `TLSSettings` is a value but its `ClientAuth` is a pointer, so every host
  derived from a template aliased the same mTLS struct (middlewares, access lists
  and the DNS policy were already copied). Not exploitable through any current
  path, but a mutation of one host's mTLS requirement could have reached every
  other host's. Now deep-copied.

## [1.0.8] - 2026-07-31

### Added

- **Ingress discovery templates can name an upstream group.**
  `settings.ingressDiscovery.template.upstreamGroupRef` is an alternative to a
  single `upstream` address, mutually exclusive with it exactly as on a proxy
  host. A cluster ingress controller normally runs on every node, so pinning
  discovery to one address made every discovered service single-node while the
  operator's hand-written hosts kept failing over - a silent availability
  downgrade for anything discovery adopted.

## [1.0.7] - 2026-07-31

### Fixed

- **The API-token form could not grant `ingress-discovery`.** The SPA rendered
  its scope checkboxes from a hand-maintained copy of `model.ScopePlurals`,
  which went stale the moment Ingress discovery added a subject - so the server
  accepted `ingress-discovery:read` while the UI offered no way to ask for it.
  `GET /api/capabilities` now serves `scopeSubjects` (the authoritative list)
  and the form renders from it, with the local list demoted to a
  fetch-failure fallback. Covered by a test asserting the served list matches
  `model.ScopePlurals` exactly.

## [1.0.5] - 2026-07-31

### Added

- **Kubernetes Ingress discovery (DNS sync phase 2).** A new `internal/k8s`
  subsystem reconciles annotated cluster `Ingress` objects into gpm-managed proxy
  hosts, which then feed the phase-1 DNS reconciler — so a cluster service no
  longer has to be hand-entered as a proxy host before its DNS follows. Opt-in is
  per Ingress and absolute: `gpm.rake.pro/managed: "true"`, with
  `gpm.rake.pro/lan-direct` / `gpm.rake.pro/public-cname` setting the derived
  host's `dns` policy; anything else (including an absent annotation) is invisible,
  and there is no namespace-sweep mode. Configured under
  `settings.ingressDiscovery`, with an operator-supplied **template** that is the
  only source for the upstream, certificate ref, middleware and access-list chain
  — an Ingress contributes strictly-validated, suffix-restricted hostnames and two
  booleans, nothing else. Because gpm runs off-cluster, in-cluster Service DNS is
  unusable: the template upstream is the **cluster ingress controller's** address,
  and the data plane's preserved `Host` header is what routes the request to the
  right workload. The client is plain `net/http` + `encoding/json` against
  `/apis/networking.k8s.io/v1` (no `client-go` — its transitive tree would dwarf
  the whole direct dependency set), works in-cluster *or* with explicit
  `apiURL`/`tokenFile`/`caFile`, re-reads the bearer token from disk on a TTL (and
  drops it on a `401`) so a rotated projected ServiceAccount token keeps working,
  and is hardened like `internal/dnssync`: CA-verified TLS with no skip-verify,
  redirects never followed, link-local destinations refused at connect time,
  bounded reads and bounded pagination. Reconcile is **full-state** and
  **ownership-gated**: only proxy hosts labelled
  `gpm.rake.pro/managed-by: ingress-discovery` are created, updated or deleted, and
  a name collision with a hand-written host is skipped with a warning. It
  **freezes on error** — a managed host is deleted only after a complete,
  successful, fully-paginated list, and any transport/status/decode/pagination
  failure aborts before any write (an empty *successful* list is a different
  return shape entirely, and is a legitimate delete-all). A whole reconcile lands
  as **one commit**; a no-drift run writes nothing. New endpoints
  `GET /api/ingress-discovery/status` (`ingress-discovery:read`) and
  `POST /api/ingress-discovery/reconcile` (`ingress-discovery:write`, **409** while
  a run is in flight), a new `ingressDiscovery.enabled` capability, a settings
  block + status panel in the web UI, and cluster-side RBAC at
  [`deploy/k8s-ingress-discovery-rbac.yaml`](deploy/k8s-ingress-discovery-rbac.yaml).
  Design record: [docs/design/ingress-discovery.md](docs/design/ingress-discovery.md).
- **`Store.ApplyBatch`.** Commits many upserts *and* deletes as a single
  revision, validating the merged graph once (including the dangling-reference
  check `Delete` performs) before touching the working tree. It is what makes
  "one commit per reconcile" possible for Ingress discovery; an empty batch is a
  no-op that commits nothing.
- **Scoped API tokens.** A new `APIToken` config object
  (`config/api-tokens/<name>.yaml`) gives scripts and CI a bearer credential
  instead of an admin session cookie. The secret (`gpm_` + 32 random bytes,
  base64url) is generated **server-side** and returned exactly once, in the
  response to the `PUT` that created it (or `?rotate=1`); only its SHA-256 digest
  is committed, in a plain string field, so the git config never holds a usable
  credential. Presented as `Authorization: Bearer gpm_...`, it is resolved before
  the cookie path and never falls through to it, by constant-time digest compare
  across the enabled, unexpired tokens. The token set is cached in the
  authenticator and invalidated from the daemon's single config-reload path, so
  revoking, disabling or expiring a token still takes effect on the next request
  without an unauthenticated bearer attempt being able to force a full config load
  per request. Scopes are `<plural>:read` / `<plural>:write` / `*:read` /
  `*:write` / `admin` (write implies read; `admin` is required for token
  management, `PUT /settings`, `GET /backup`, `POST /restore`, the whole-config
  `POST /revert` and `/debug/pprof/*`). Sessions are unaffected: scopes constrain
  API tokens only. The stored digest is never returned by any endpoint, and
  reverting an `APIToken` is refused so a rotation always means revocation.
  `GET /api/api-tokens` surfaces an in-memory `lastUsed` per token
  (deliberately not persisted — the store is git-backed). New page in the web UI
  with a scope picker, one-time reveal, rotate and delete
  (`internal/model/apitoken.go`, `internal/auth/apitoken.go`, `internal/api`).
- **DNS sync (Pi-hole + Cloudflare).** A new `internal/dnssync` subsystem
  publishes CNAME records for the proxy hosts that opt in via a per-host
  `dns` policy (`lanDirect` for the LAN resolver, `publicCname` for the public
  zone), configured once under `settings.dnsSync`. Reconcile is **full-state**,
  not diff-based: the desired set is recomputed from the whole config on every run
  and compared with what each backend actually holds, so out-of-band drift is
  repaired in both directions. Deletion is **ownership-gated** — on Pi-hole only a
  CNAME whose target is exactly the configured `apexTarget`, on Cloudflare only a
  record carrying the `managed-by:gpm` comment (re-checked inside the delete call
  itself) — so a record gpm did not create is read and ignored, never removed.
  Creation is gated the same way: a desired name that already exists as somebody
  else's record is logged and skipped rather than shadowed. Reconciles fire
  automatically after proxy-host/settings writes and whole-tree restore/revert
  (non-blocking, bursts coalesced into a single follow-up run) and on demand via
  `POST /api/dns-sync/reconcile`, which answers `409 Conflict` rather than
  queueing while a run is in flight; `GET /api/dns-sync/status` reports
  the last run per backend. The Pi-hole v6 client opens and explicitly releases its
  session (Pi-hole has a small session pool) and reports a `403` as the distinct
  "read-only session / `app_sudo` disabled" condition. The Cloudflare client is
  separate from the ACME solver on purpose, so record lifecycle and certificate
  issuance cannot break each other. Both use the webhook dispatcher's hardened
  HTTP client (no redirects, link-local refused at connect time).
- **`GET /api/capabilities` gains `apiTokens` and `dnsSync` groups**, so the SPA
  greys out the per-host DNS toggles whose backend is not configured, and the
  "API Tokens" nav entry and page when no token source is wired, instead of
  accepting input that could not work (`internal/api`, `internal/ui`).

### Changed

- **One Ingress reconcile's cluster list is bounded to two minutes.** The only
  bound was the 30s per-request timeout times the 100-page limit — roughly 50
  minutes holding the reconcile mutex, during which every poll and every manual
  reconcile queues behind it. The list now runs under a `context.WithTimeout`
  (`internal/k8s`).
- **`GET /api/capabilities` no longer loads and validates the whole config to
  answer whether Ingress discovery is enabled.** The probe runs on every admin page
  load and took the store read lock behind in-flight reconcile commits. The
  `Discoverer` now caches the flag, refreshing it on every reconcile and every poll
  interval read, so the answer lags a settings change by at most one poll
  (`internal/k8s`).
- **Manual DNS reconciles no longer queue.** `POST /api/dns-sync/reconcile` used
  a blocking lock acquire that ignored the request context, so repeated calls
  piled goroutines (each holding a request-scoped context) up behind a slow
  backend. It now uses `Syncer.ReconcileNow`, which refuses with
  `ErrReconcileInProgress` → **409 Conflict**. The event-triggered path still
  waits, which is what makes trigger coalescing correct (`internal/dnssync`,
  `internal/api`).
- **`ProxyHost.dns` is now omitted from JSON when unset.** `encoding/json`
  ignores `omitempty` on a struct value, so every proxy-host response carried a
  noise `"dns":{}`; the field is a pointer now (`internal/model`).

### Fixed

- **`Store.ApplyBatch` is now actually atomic, as its doc comment claimed.** YAML
  was written and files removed *before* `CommitAll`, so a failing `os.Remove` or a
  failing/cancelled commit left the working tree mutated but uncommitted: the next
  `Load` served the deletions as live config while the status reported failure, and
  the following unrelated write swept the orphans into its commit. Shutdown made it
  reachable — the discovery goroutine was fire-and-forget, so `main` could cancel the
  context mid-commit and return. Every file a batch touches is now snapshotted first
  and the tree is rolled back on any failure (the `snapshotDirs`/`writeDirs` precedent
  from `RevertObject`), and the reconciler runs under a `sync.WaitGroup` that
  shutdown waits on (`internal/store`, `cmd/gpm`).
- **Ingress discovery no longer fires a reload, webhook and DNS trigger on an
  empty commit.** When every planned delete turned out to be already gone,
  `ApplyBatch` returned `("", nil)` and `onChange("")` ran anyway — reloading,
  dispatching a lifecycle webhook and triggering DNS with an empty
  `status.commit` while `deleted` was non-zero. Notification now requires a real
  commit (`internal/k8s`).
- **An oversized Kubernetes response is reported as such, not as a decode
  failure.** `io.ReadAll(io.LimitReader(...))` truncated silently at the 8MiB body
  cap, surfacing as a JSON syntax error and a permanent freeze — triggerable by a
  tenant putting large annotations on an annotated `Ingress`. The read now goes one
  byte past the cap to detect the overflow and says so, and `listPageSize` drops
  from 500 to 100 so a page of realistically-sized objects stays well clear of the
  cap (`internal/k8s`).
- **A namespace containing a dot is refused rather than assumed impossible.**
  `derive()` builds `ing-<name>.<namespace>` and relied on the API server having
  enforced that a namespace is a DNS-1123 label; the derived name is an ownership
  boundary, so gpm now validates it itself and skips the Ingress (returning no name,
  so an ambiguous one cannot protect a host from deletion) (`internal/k8s`,
  `internal/model`).

### Security

- **Ingress discovery: ownership now gates the DOMAIN, not just the derived
  name.** A derived host was skipped only when its *name* collided with a host gpm
  did not own, but the data plane routes by hostname and fills its per-domain maps
  in config load order — so a cluster tenant who could annotate an `Ingress` in
  their own namespace could claim `sso.example.com`, derive a name colliding with
  nothing, sort after the operator's host, and replace that host's SSO/access-list
  chain with the template's (and overwrite its mTLS pinning). `allowedDomainSuffixes`
  was no defence: exact-suffix matching makes even the apex claimable. `planReconcile`
  now refuses any derived host whose domains are already claimed by a host discovery
  does not own — proxy, redirect or dead, enabled or disabled — reporting it per host
  in `status.hosts[].reason`, and resolves two Ingresses claiming one hostname
  first-by-derived-name-wins. Backstopped in `Config.Validate`, which now rejects any
  two **enabled** hosts claiming the same domain whatever wrote them (disabled hosts
  are exempt so staging a replacement beside the live host still works)
  (`internal/k8s`, `internal/model`).
- **Ingress discovery: a `200` that is not an `IngressList` no longer reads as an
  empty cluster.** The LIST decode asserted nothing about the response shape, so
  `null`, `{}`, a `kind: Status` reply and `{"items":null}` all decoded to zero
  items, no continue token and a nil error — which the reconciler treats as a
  complete list and answers by **deleting every managed host**. It needed no
  compromised API server: an `apiURL` typo'd onto another HTTPS service behind the
  same internal CA, a mesh/gateway `200` envelope, or a namespace/label-selector
  typo would do it. The page struct now asserts `kind: IngressList` and a present
  `items` field, and anything else is an error on the freeze path (`internal/k8s`).
- **`Store.ApplyBatch` re-checks ownership under the store lock.** It trusted the
  caller's ownership filter and never inspected labels, while the caller's plan was
  computed from a snapshot taken *before* a multi-second cluster list — so an object
  created, deleted or relabelled inside that window was written or removed on the
  strength of a check against state that no longer existed. `ApplyBatch` takes an
  `ApplyGuard` it evaluates against freshly loaded state, under the lock, for every
  delete target and every pre-existing upsert target; a guarded-away object is
  skipped, not clobbered, and a batch left with nothing to do commits nothing. The
  Ingress reconciler installs a managed-by-label guard (`internal/store`,
  `cmd/gpm`).
- **Shipped Ingress RBAC narrowed to `list`, and the token-extraction recipe no
  longer leaves a world-readable window.** The `ClusterRole` granted `get`/`list`/
  `watch` while the client only ever lists. The manifest's `ca.crt` jsonpath
  (`{.data\.ca\.crt}`) also escaped the `data` separator, matched nothing, and
  silently wrote an **empty CA file**; and both files were created by a plain shell
  redirect (mode `0644`) and only `chmod 0400`'d afterwards, leaving the bearer
  token readable by any local user in between. The manifest and
  `docs/deployment.md` now use `install -m 0400 /dev/null` first, the correct
  jsonpath, and document that a namespaced `Role`/`RoleBinding` is tighter when
  `settings.ingressDiscovery.namespace` is set
  (`deploy/k8s-ingress-discovery-rbac.yaml`, `docs/deployment.md`).
- **`/debug/pprof/*` now requires the `admin` scope, not just the admin role.**
  Every API-token principal is admin-*role* by construction (the coarse gate is
  satisfied and the real authorization is the per-route scope check), so a
  `proxy-hosts:read` token could fetch the heap profile and `cmdline` — which
  carry resolved Cloudflare/Pi-hole credentials in cleartext. The profiling
  endpoints now run an admin-scope check inside the role gate. Admin *sessions*
  are unchanged (`internal/server`).
- **`PUT /api/settings` now requires the `admin` scope.** `settings:write` was
  admin-equivalent in practice: it could point `dnsSync.pihole.url` or a webhook
  at an attacker-controlled URL with `${ENV:SOME_TOKEN}` as the credential, and
  the settings write itself triggers the reconcile/dispatch that resolves and
  sends that env var offsite — and it could rewrite `adminAuth` outright.
  `GET /api/settings` stays on `settings:read` (reading resolves nothing). The
  UI greys out the `settings` write box rather than offering a grant that no
  longer does anything (`internal/api`, `internal/ui`).
- **`GET /api/backup` now requires the `admin` scope** (was `*:read`). The
  archive is the raw on-disk YAML, so unlike the JSON reads it carries the
  api-tokens' stored digests — and it is the exact input `POST /api/restore`
  takes, which was already admin-scoped (`internal/api`).
- **`APIToken.tokenHash` is no longer returned by any endpoint** (`json:"-"`,
  yaml tag unchanged so at-rest persistence is identical). It was previously
  readable through `GET /api-tokens`, `GET /config` and `GET /backup` on a
  read-only scope; a SHA-256 digest is offline-crackable, so handing it to a
  read-only caller let them grind for the secret at leisure (`internal/model`).
- **Rotation now means revocation: reverting an `APIToken` is refused.**
  `POST /api/api-tokens/{name}/revert` restored an older token file — and with
  it an older `tokenHash` — silently reviving a secret the operator had rotated
  away, while the UI promised "the current secret stops working immediately".
  `Store.RevertObject` refuses the kind with `ErrNotRevertible`, and the
  whole-config `POST /api/revert` snapshots the `api-tokens` directory before the
  tree restore and writes it back over the result, so it neither revives a
  rotated digest nor resurrects a deleted token. Everything else still reverts
  normally, and the UI copy now says so (`internal/store`, `internal/ui`).
- **Unauthenticated bearer attempts no longer force a config load.** Resolving a
  presented token read the whole git-backed config (directory walk, YAML parse,
  whole-graph validation) on every request, and a failed bearer auth never
  reaches the login rate gate — an unthrottled DoS lever for anything
  internet-facing. The authenticator now caches the token set and drops it from
  the daemon's single reload path, so token edits still take effect immediately
  (`internal/auth`, `cmd/gpm`).
- **The API scope gate fails closed on a missing principal.** The daemon's
  `RequireScope` returned nil (allow) when no principal was on the request. That
  is unreachable through the mounted route, which is exactly why it must deny: a
  broken authorization wiring should refuse, not wave requests through. A nil
  `Deps.RequireScope` still means "allow" and is documented as the unwired/test
  case only (`cmd/gpm`, `internal/api`).
- **Pi-hole DNS sync no longer overwrites an operator-owned CNAME.** The
  reconciler compared only against records it already owned, so a name an
  operator had deliberately pointed elsewhere got a second, shadowing entry. It
  now builds a present-set from every returned record and skips-and-warns on a
  name that exists with a different target — the behaviour the Cloudflare backend
  already had (`internal/dnssync`).

## [1.0.4] - 2026-07-20

### Added

- **Web UI: middleware editor supports `rewrite`.** The middleware-type
  dropdown now offers `rewrite` with a `replacePath` key/value row editor
  (parity with the `headers` editor), so an existing `rewrite` middleware can
  be opened and saved without being coerced into another type
  (`internal/ui/static/app.js`).

### Security

- **Aikido SAST suppression documented at the git exec site.** The
  command-injection finding on `internal/store/gitrepo.go` is a false
  positive - git runs as a fixed argv (no shell) with internal literal
  arguments - now marked with an inline `noaikido` justification instead of
  recurring in every scan.
- **Admin CSP tightened from frame-ancestors-only to a strict policy**
  (defense-in-depth). Every admin/login response now carries
  `script-src 'self'` (plus `default-src 'self'`, `connect-src 'self'`,
  `object-src 'none'`, `base-uri 'none'`, `form-action 'self'`), so injected
  markup could not execute script even if the SPA's output escaping ever
  regressed. Carve-outs: `style-src 'unsafe-inline'` for the SPA's inline
  style attributes, and Google Fonts (`style-src` fonts.googleapis.com,
  `font-src` fonts.gstatic.com). Admin listener only — proxied data-plane
  traffic is untouched (`internal/server`).

## [1.0.1] - 2026-07-19

### Added

- **`rewrite` middleware: exact-match request-path replacement.** A new
  middleware type (`type: rewrite`) with a `replacePath` map that swaps an
  incoming request path for a target before proxying, when the path matches a key
  **exactly** (no regex/prefix matching, sidestepping the path-confusion/ReDoS
  classes). The rewrite is internal and upstream-facing: it preserves the method
  and body (never an HTTP redirect) and runs **innermost** in the chain (closest
  to the upstream, after headers), so auth/guard/access-list all still evaluate
  the ORIGINAL client path. Motivating case: repair a client that mangles an
  upstream path - e.g. a mobile OIDC app that strips the trailing slash off
  Authentik's `/application/o/token`, which Django answers `405`; a
  `/application/o/token → /application/o/token/` rewrite at the edge fixes it.
  Both key and value must be absolute paths and a key may not map to itself
  (`internal/dataplane/rewrite.go`, model `RewriteMiddleware`).

## [1.0.0] - 2026-07-18

### Added

- **HA design doc** ([docs/design/ha.md](docs/design/ha.md)): a settled
  multi-instance story for gpm itself. Recommends a phase-1 active/standby
  two-node homelab pair - static leader owns ACME renewal and admin writes, the
  follower pulls config via git `pull --ff-only` and reads replicated
  certs/secrets, a keepalived VRRP VIP steers traffic (client IP preserved), the
  `sso_not_before` watermark gains a refresh loop so revocation propagates
  without a restart, and TCP/UDP streams are failover-with-reconnect. Per-instance
  state (admin sessions, access-log ring, rate-limit buckets, login-lockout maps)
  is documented as lossy. No new dependency; no data-plane hot-path change.
- **Per-object revert.** A scoped revert restores only one object's file to its
  state at a past commit, committing just that change and leaving every other
  object untouched. New `Store.RevertObject(kind, name, hash)` uses
  `git checkout <hash> -- <rel>` (path always after `--`); the rel path is derived
  from the trusted object-kind directory mapping (never client-supplied), the hash
  is validated, and the whole config is re-validated with a rollback to HEAD on
  failure exactly like the whole-tree revert. New endpoint
  `POST /api/{kind}/{name}/revert` (body `{"hash":"<commit>"}`), same
  auth/CSRF/reload/webhook wiring as the whole-config revert. The History view now
  offers "revert this object" per single-object commit, and the existing
  whole-tree action is relabeled "revert entire config" to make its scope
  unmistakable. This closes the gap where reverting one object from its History
  view silently deleted every object created after that commit (operator incident
  2026-07-16: a proxy-host revert wiped three newer Certificate objects).
- **Upstream groups: health-checked failover across multiple backends.** New
  first-class `UpstreamGroup` object (`config/upstream-groups/`): an ordered
  list of upstreams (first = primary, rest = backups) with a per-group health
  check (TCP connect by default, or HTTP GET via `healthCheck.path`; tunable
  `intervalSeconds`/`timeoutSeconds`/`rise`/`fall`). A ProxyHost selects it via
  `upstreamGroupRef` (mutually exclusive with `upstream`; locations inherit the
  group unless they set their own single `upstream`). The data plane tries
  candidates healthy-first in config order and retries a request on the next
  upstream **only on connect-phase failures** (dial/TLS — the request was never
  sent, so retrying is safe for non-idempotent methods; an HTTP response,
  including a 5xx, is never replayed). Live-traffic connect failures feed the
  same fall counter as the active probe, an immediate probe round runs at
  (re)load, and a group with every upstream down fails open (attempts in
  preference order). Request bodies up to 1 MiB are buffered for replay; larger
  bodies stream with a single attempt at the preferred upstream. Group health
  state and probers survive config reloads that don't change the group, and a
  rejected config never disturbs the running probers. Live status at
  `GET /api/upstream-health`; full CRUD at `/api/upstream-groups/`; UI gains an
  "Upstream Groups" section (typed editor with per-upstream up/down chips) and
  a group selector on the proxy-host editor. Referential integrity enforced:
  unknown or disabled group references are rejected at write time.
- **Load-distribution policies and weights on upstream groups.** `policy` on
  `UpstreamGroup` selects how traffic spreads across the healthy upstreams:
  `failover` (default — strict list order, unchanged behavior), `round-robin`
  (smooth weighted round-robin, nginx's algorithm), `least-connections`
  (fewest in-flight requests relative to weight, tracked per upstream until
  each response body closes), and `ip-hash` (rendezvous hashing on the client
  IP for sticky sessions — when an upstream dies only its own clients move).
  Per-upstream `weight` (1–256, default 1) sets the relative share for the
  weighted policies and is ignored by `failover`/`ip-hash`. Unhealthy
  upstreams are demoted to the end of the try-order under every policy
  (fail-open preserved), and the connect-error-only retry rule is unchanged.
  `GET /api/upstream-health` now also reports each upstream's `weight` and
  in-flight `active` count. UI: policy select and per-upstream weight column
  on the group editor.
- **Cookie-based sticky sessions with a server-enforced TTL on upstream
  groups.** `stickiness: {ttl: "12h", cookie: "..."}` on `UpstreamGroup` pins
  each client to its assigned upstream via an HMAC-signed cookie (default name
  `gpm-sticky-<group>`; `HttpOnly`, `Path=/`, `SameSite=Lax`, `Secure` when the
  client arrived over HTTPS). The TTL (Go duration, plus a `d` day suffix,
  e.g. `3d`) rides inside the signed value, so replaying a cookie past its
  `Max-Age` still re-assigns — expiry is authoritative server-side and the
  window is fixed from assignment, not sliding. Signed with the existing
  data-plane SSO key (`GPM_SSO_SIGNING_KEY` / persisted `sso_signing.key`), so
  clients cannot forge a pin to a chosen backend; tampered, expired,
  foreign-group, or dead-upstream pins all fall back to the policy and get a
  fresh assignment cookie, and an honored pin adds no Set-Cookie noise.
  Composes with every policy (the policy picks initial/replacement
  assignments). UI: sticky TTL + cookie-name fields on the group editor.
- **Per-location upstream group references.** `upstreamGroupRef` on a
  `Location` (mutually exclusive with the location's `upstream`) points one
  path at its own group; with neither set the location inherits the host
  backend as before. Validated like the host-level reference (unknown or
  disabled groups rejected). UI: a group select per location row on the
  proxy-host editor.
- **Configurable rate-limit window on `RateLimitMiddleware`.** Rate limits can
  now be expressed as `requests` + `window` (a Go duration string, e.g. `10s`,
  `1m`, `1h`) for limits that don't reduce cleanly to a per-second rate (e.g.
  "100 requests per 1m", "5 per 1h"). The legacy `requestsPerSecond` field is
  kept as shorthand for `requests`/`window: 1s` and remains fully supported;
  exactly one of the two forms may be set, validated at config load. Burst
  still defaults to `ceil(requests)` and can still be overridden explicitly.
  Data-plane refill rate and `Retry-After` math both derive from the
  configured window rather than assuming seconds. UI editor (middleware
  rate-limit form) updated to a `Requests` + `Per` (window) pair; existing
  `requestsPerSecond`-only configs still load and behave identically, and
  editing one through the UI migrates it to the new form on save.
- **`allowFrom` exemption on `RateLimitMiddleware`.** Rate-limit middlewares now
  accept an `allowFrom` list of client CIDRs/IPs (same shape and validation as
  `AuthMiddleware.AllowFrom` / `GuardMiddleware.AllowFrom`); a matching client
  bypasses the limiter entirely (no token consumed, no 429), resolved via the
  same XFF-aware client-IP logic as the rest of the chain. Non-matching and
  unresolvable-IP clients are limited exactly as before. Config, data-plane
  enforcement, UI editor, and tests all updated.
- **Optional `blockFor` block period on `RateLimitMiddleware`.** Once a client
  exceeds the limit, further requests from it are rejected for a configured
  duration (a Go duration string, e.g. `"30s"`, `"5m"`) regardless of token
  refill, so a briefly-paused client cannot immediately sail back through. The
  block is fixed, not sliding - repeat requests during the block do not extend
  it, and once it expires ordinary token-bucket rules resume (no instant
  re-burst beyond `burst`). Optional and off by default (empty `blockFor` is
  today's refill-only behavior); validated via `time.ParseDuration` (must be
  > 0) alongside either rate form. UI editor adds a "Block for" select
  (none/10s/30s/1m/5m/15m/1h, with round-trip preservation for a
  hand-authored value like `2m`) next to the rate-limit form.
- **Opt-in `net/http/pprof` on the admin server.** New `-pprof` flag / `GPM_PPROF`
  env var (default off, same pattern as `-debug-headers`) mounts `net/http/pprof`
  under `/debug/pprof/` on the admin listener only, behind the same admin-role +
  same-origin CSRF gate as the REST API (`RequireRole(RoleAdmin, ...)` +
  `sameOriginGuard`). Registered explicitly on the admin mux, never via the
  side-effecting `_ "net/http/pprof"` import, so nothing is ever exposed on
  `http.DefaultServeMux`. Never touches the data-plane router.
- **Domain-group filter chips on the Proxy Hosts list.** Each host's domains are
  grouped into a "zone" (a wildcard's remainder, or the last two labels of a
  regular domain — `sensor.iot.example.com` and `*.iot.example.com` both group
  under `iot.example.com`). When a list has 2+ zones, a chip row renders below the
  search box (label "zone (count)", sorted by count then name); clicking a chip
  excludes/includes that zone, composing with the existing text filter. Excluded
  zones persist in `localStorage` (`gpm.hosts.zonesOff`) across navigation and
  reloads.
- **mTLS (phase 1): per-host client-certificate auth** (`tls.clientAuth`): a
  proxy/redirect/dead host can require or accept a client certificate verified
  against a new `ClientCA` trust-anchor object (`caPEM`, inline or
  `${FILE:...}`, confined to `GPM_SECRET_FILE_ROOTS`). `mode: require`
  (default) rejects the handshake without a valid client cert; `mode:
  optional` verifies a presented cert but lets certless requests through.
  Enforced per request, not just per handshake: the negotiated SNI must
  resolve to the host's own `tls.Config` (closing an SNI != Host dodge) and,
  in `require` mode, carry a verified chain, or the request gets `421
  Misdirected Request`; an mTLS host is also redirected off the plaintext
  `:80` listener. `clientAuth` requires `forceSSL: true` and a resolvable
  `caRef` at validation time. Revocation (CRL/OCSP) and identity-passthrough
  headers are not implemented yet (phase 2).
- **GeoIP geoblocking** (`geo` on `AccessList`): `countryAllow` /
  `countryDeny` (ISO-3166-1 alpha-2) plus `onUnknown: allow|deny` for IPs with
  no country (private/LAN, or a stale/missing database). Reuses the existing
  access-list client-IP resolution. New runtime dependency
  `github.com/oschwald/maxminddb-golang/v2` (pure Go, no CGO) reads an
  operator-supplied MaxMind GeoLite2/GeoIP2 `.mmdb` (not bundled - GeoLite2
  licensing) via `GPM_GEOIP_DB` / `-geoip-db`, hot-reloaded on a 5-minute
  mtime watch so an out-of-band `geoipupdate` refresh takes effect without a
  restart. Fail-closed throughout: saving an access list with geo rules while
  no database is loaded is rejected at write (`400`, nothing committed); a
  geo list evaluated with no database loaded denies all traffic on its hosts
  rather than allowing, evaluated live per request so it self-heals the
  moment the database loads, no restart or config change needed; startup no
  longer fatals on a geo config with no database - the edge boots with the
  affected hosts denying.
- **`GET /api/capabilities`**: read-only, admin-session-gated probe
  (`{"geoip":{"dbLoaded":bool}}`) the admin SPA uses to grey out/disable geo
  access-list controls when no GeoIP database is loaded.
- **Per-host no-index toggle** (`robotsNoIndex`): emits
  `X-Robots-Tag: noindex, nofollow` on HTTP and HTTPS responses. A headers
  middleware that sets `X-Robots-Tag` explicitly still wins.
- **Host tags** (`tags` on every object's metadata): free-form flat labels for
  grouping/filtering, surfaced as chips on the Proxy Hosts list and matched by the
  existing filter box.
- **Per-host upstream timeouts** (`timeouts.connectSeconds` / `readSeconds`): a
  host with an override uses its own cloned transport (its own connection pool), so
  the override cannot affect any other host. Hosts without an override keep using
  the shared, pooled transport unchanged. `readSeconds` bounds time-to-first-byte
  only, so it is safe for streaming/websocket upstreams.
- **Access-log viewer** (`GET /api/logs` + "Access Logs" view): an in-memory ring
  of recent requests (method, host, path, status, duration, client), newest first.
  Only filled while access logging is enabled (`--access-log` / `GPM_ACCESS_LOG`),
  so the default off-path keeps zero per-request overhead; the buffer is bounded.
- **Lifecycle webhooks** (`settings.webhooks`): gpm POSTs a small JSON event
  (`action`, `kind`, `name`, `commit`, `time`) to each configured URL after every
  config change. Delivery is asynchronous and best-effort under a 10s timeout, so a
  slow or unreachable endpoint never blocks or fails a config write. An optional
  per-target secret (placeholder-resolved) is sent as `X-GPM-Webhook-Secret`.
- **Data plane**: native-Go reverse proxy with SNI-based TLS termination (exact +
  wildcard certificate selection), HTTP/2, WebSocket upgrades, and force-SSL
  (HTTP→HTTPS 308 redirects).
- **Host types served by the data plane**: proxy hosts (reverse proxy + chain),
  **redirect hosts** (configurable scheme/status/path-preserving 3xx), **dead
  hosts** (fixed status, default 404; absorb unmatched vhosts, no default-host
  leakage), and **stream hosts** (raw TCP/UDP forwarding on their own per-port
  listeners, reconciled on reload). Previously only proxy hosts routed.
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
- **Rate-limit middleware enforcement**: the `rate-limit` type is now applied on
  the data plane as a per-host, per-client-IP token bucket (capacity = `burst`,
  refill = `requestsPerSecond`); over-limit requests get `429 Too Many Requests`
  with a `Retry-After` header. Tracked buckets are capped with idle eviction so a
  flood of distinct source IPs cannot grow memory without bound.
- **Composable middleware chain** per host and per location, applied in a fixed
  order (rate-limit → auth → guard → access-list → headers → upstream).
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
- **Per-host OIDC gating** on the data plane: a proxy host with auth mode `oidc`
  makes gpm the OIDC relying party - unauthenticated requests are redirected to
  the IdP, the callback (`/__gpm/oidc/callback`) exchanges the code (PKCE + nonce)
  and issues a signed, stateless SSO session cookie, and the role mapping is
  enforced. Sign the cookie with `GPM_SSO_SIGNING_KEY` to persist sessions across
  restarts.
- **HSTS emission**: the data plane now sends `Strict-Transport-Security` on HTTPS
  responses for hosts with `tls.hsts.enabled` (honouring `maxAge`,
  `includeSubdomains`, `preload`; one-year default).
- **Per-host minimum TLS version**: `tls.minTLSVersion` (`"1.2"` default | `"1.3"`)
  pins a host's TLS floor by SNI via `GetConfigForClient`; the edge still
  negotiates 1.2/1.3 per client by default, and a host can require 1.3 where all
  its clients support it.
- **Config backup / restore**: `GET /api/backup` downloads a portable gzip-tar of
  the whole config; `POST /api/restore` replaces it from such an archive
  (validated, committed as one revision, rolled back if invalid) - with UI
  controls on the History view.
- **Config revert**: `POST /api/revert` restores the entire config to a past
  commit as a new commit (so it is itself revertible); the History view's per-
  commit revert action is now live.
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

- **Data-plane reverse proxy tuned for large/streamed upstream responses.** The
  shared upstream transport now sets `DisableCompression: true` (the proxy must
  not transparently gunzip upstream bodies — CPU cost, and it strips
  Content-Length, forcing the flush-per-write chunked path), and the main
  `httputil.ReverseProxy` now uses a fixed 512KiB `sync.Pool`-backed `BufferPool`
  (a larger copy buffer cuts the per-write flush count for chunked upstream
  responses, which stdlib flushes after every write). The outpost auth
  subrequest client (`internal/dataplane/authrequest.go`) now uses a dedicated,
  tuned `http.Transport` (no proxy env, 5s dial/TLS timeouts, 64 idle conns per
  host) instead of the default transport's 2-conn-per-host cap.
- **Browser tab title dropped the " admin" suffix.** The static fallback title
  and the dynamic `document.title` (set from `settings.appName` once loaded)
  now both read just "Go Proxy Manager" (or the configured app name), matching
  the sidebar wordmark.
- **Lifecycle webhooks now fire only after a config change is applied to the
  running data plane, not merely committed to git** - a write that commits
  but fails to apply (e.g. a geo rule saved the instant its GeoIP database
  goes missing) emits no webhook. The event payload
  (`action`/`kind`/`name`/`commit`/`time`) is unchanged; the delivered event
  now means "applied and live," not just "committed."
- **Admin UI**: replaced the labelled raw-JSON editor with full field-level forms
  for every remaining object kind - redirect hosts, stream hosts, dead hosts, DNS
  providers, identity providers, access lists, and middleware. Each form has typed
  inputs (text/number/enum selects), add/remove rows for list and map fields
  (domains, CIDR rules, basic-auth users, header maps, guard triggers, etc.), an
  identity-provider picker for auth middleware, and secret-aware fields that show
  `${ENV:...}` placeholders and refuse to overwrite a masked (`***`) secret.
  Polymorphic kinds (identity provider type, middleware type) toggle the relevant
  sub-spec section on change.
- Build on Go 1.26.4.
- Tuned the shared upstream transport (HTTP/2, higher idle-conns-per-host) to cut
  connection churn for backends that share an upstream.
- Admin password hash read from `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE` (Docker
  secret) in preference to an inline env value, avoiding compose `$`-interpolation
  of bcrypt hashes.
- Moved the parallel-test data-plane ports off 8080 to 8880/8843.

### Fixed

- **Duplicate `Strict-Transport-Security` on the proxied admin path.** The admin
  server emitted its own HSTS header in addition to the one the data plane (the
  actual TLS edge) emits for the admin host, so a request to the admin panel
  through gpm carried two identical HSTS headers. HSTS is now set only by the data
  plane; the admin server no longer sets it (over its direct plain-HTTP port HSTS
  was ignored anyway, and via the proxy the edge owns it).
- Logout now lands on the login page under SSO-only instead of silently
  re-authenticating against a live IdP session.
- Rewrite upstream `Location` redirects that point back at the upstream's own
  address (e.g. Pi-hole/civetweb absolute redirects) to the public scheme/host.
- Custom-certificate absolute-path validation is now OS-agnostic (a leading `/`
  or `\` is rejected on every platform, not just where `filepath.IsAbs` treats it
  as absolute).
- Removed the dead, unreferenced `router.tlsConfig()` (the HTTPS server builds its
  own `tls.Config` inline); the TLS 1.2 floor is unchanged.

### Security

- **Webhook delivery is SSRF-bounded** (issue #1, low/defense-in-depth).
  Lifecycle-webhook targets are admin-configured URLs, which made delivery an
  SSRF primitive from gpm's network position. Redirects are no longer followed
  (a 3xx counts as a failed delivery instead of bouncing gpm to a URL the admin
  never configured), and link-local destinations (`169.254.0.0/16`, `fe80::/10`
  — cloud metadata services) are refused at connect time on the resolved
  address, so a rebinding resolver cannot dodge the check. Private/LAN targets
  remain allowed (the normal self-hosted case); URL scheme/shape was already
  validated at config-write time.
- **Data-plane SSO sessions gained global revocation** (issue #1,
  low/defense-in-depth). Sessions stay stateless (1h absolute TTL), but
  `POST /api/sso/revoke` (admin-gated, CSRF-protected; also a button under
  Settings) moves a persisted revocation watermark to "now": any session issued
  before it — including legacy cookies minted before this change — fails the
  gate and the user re-authenticates at the IdP. The watermark is stored next
  to the SSO signing key so revocation survives restarts. Session cookies now
  carry an `iat` claim to support the check.
- **`GPM_COOKIE_SECURE=0` production guard** (issue #1, low/defense-in-depth).
  gpm now logs a prominent startup warning when Secure cookies are disabled
  while `settings.externalBaseURL` is `https://` — the signature of a
  TLS-fronted deployment mis-set for local testing. A warning rather than a
  hard failure, because a LAN-only plain-HTTP admin listener alongside the
  public URL is a known deliberate topology.
- **Data-plane SSO cookies use the `__Host-` prefix.** `gpm_sso` / `gpm_sso_state`
  are now `__Host-gpm_sso` / `__Host-gpm_sso_state`, so the browser enforces their
  Secure + host-locked (no `Domain`) + `Path=/` scope and a sibling subdomain cannot
  plant a same-named shadow cookie. (Forged values already failed the HMAC; this
  closes the shadowing vector.) Active sessions re-authenticate once (GPM-I2).
- **Settings validation rejects an admin-lockout configuration.** A settings write
  with neither local login nor any SSO provider — no way into the admin panel — is
  now refused at validation instead of committing and locking the operator out
  (previously recoverable only by redeploy) (GPM-I4).
- **A host referencing a disabled ClientCA is rejected at validation.** Previously
  referential validation only checked the CA name existed; a disabled CA then
  produced a nil pool and a hard TLS-config error that failed the whole router
  reload. The disabled reference is now a clear load-time error (GPM-I3).
- **Access lists are evaluated ahead of authentication.** The middleware chain now
  runs `rate-limit → access-list → auth → guard → headers → upstream` (previously
  the access-list sat inside auth). An IP the access-list would deny is now dropped
  before any auth work runs, so a non-allowed client hitting an OIDC/forward-auth
  host no longer drives a forward-auth subrequest to the IdP or receives an OIDC
  redirect/401 that discloses the auth flow. IP/geo access policy is enforced as an
  edge control, before identity, as intended (GPM-L1).
- **Client-IP resolution for IP-based controls is now per-host, not a global
  union.** `X-Forwarded-For` is honoured (for access-list, rate-limit, geo, and
  auth-request `allowFrom`) only for proxies the target host actually trusts — the
  `trustedProxies` of the forward-auth IdPs that host references — mirroring the
  host-scoped identity-header trust. Previously the trusted-proxy set was the union
  across every IdP, so a proxy trusted by one host could have its `XFF` honoured
  when resolving another host's client IP. A host with no forward-auth IdP in front
  now keys IP controls off the connection peer (GPM-L4).
- **Data-plane SSO sessions use a 1-hour absolute TTL (was 12h).** The `gpm_sso`
  cookie is no longer valid for 12h with no revocation path; it now expires 1h after
  login and is not extended by activity. On expiry the gate re-runs the OIDC flow
  (silent while the IdP session is valid) and re-checks group membership, bounding
  the offboarding window for a deprivileged or disabled user to ~1h without
  server-side session state (GPM-L3).
- **Login/OIDC-begin rate gate fails closed under a distinct-key flood.** When the
  gate's per-IP map is saturated with live entries, a new key could previously go
  unrecorded and thus never reach its limit (a brute-force bypass). The read path
  now treats an untracked key as at-limit while the map is saturated, so a flood
  degrades login into a lockout rather than an unthrottled guess (GPM-L2).
- **`Restore` enforces the no-literal-secrets guard.** An uploaded backup archive is
  now checked for literal (non-placeholder) secrets after load, matching `Save`/
  `SaveSettings`; a violation rolls the working tree back and commits nothing, so a
  crafted archive cannot land a plaintext secret on disk or in git history. The
  rollback itself was hardened: `RestoreTree` now also removes untracked files
  (`git clean -fd`), closing a gap where a refused restore's newly-written files
  survived on disk and would be picked up by the next config load (GPM-L6).
- **`${ENV:...}` cannot resolve gpm's own reserved secrets, and can be prefix-gated.**
  `GPM_SSO_SIGNING_KEY` and `GPM_LOCAL_ADMIN_PASSWORD_HASH` are never resolvable via
  an `${ENV:...}` placeholder, so an admin-authored value cannot exfiltrate the SSO
  signing key or admin password hash (e.g. via a webhook secret). Operators can
  further restrict resolution to an allowlist of name prefixes with
  `GPM_SECRET_ENV_PREFIXES`, mirroring `${FILE:...}` confinement (GPM-L7).
- **Rate-limiter bucket eviction is now O(1) (LRU) instead of an O(n) map scan.**
  At the 16 384-bucket cap, a rotating-source-IP flood forced a full-map scan under
  the limiter mutex per new key, serializing rate-limit checks for the host (a DoS
  amplifier). Buckets are held in an LRU list and the least-recently-used one is
  evicted in constant time; memory stays bounded as before (GPM-L5).
- **Access lists reject `\` and `;` in the request path.** Path matching for
  path-scoped locations and guard triggers is exact after `path.Clean`, which does
  not treat a backslash or a `;` matrix parameter as path structure. A request like
  `/admin;x` or `/admin\..\x` therefore failed to match a location/guard keyed on
  `/admin` yet could be re-collapsed onto `/admin` by a backend that strips
  `;`-parameters (Servlet containers) or treats `\` as a separator (IIS/Windows),
  slipping past that path's auth. The data plane now rejects any request whose
  canonical path still contains `\` or `;` with `400 Bad Request`, keeping the
  matched path and the forwarded path byte-identical.
- **Data-plane SSO session cookies are bound to their issuing host.** The `gpm_sso`
  cookie is HMAC-signed with a process-wide key, so a valid session minted for one
  OIDC-gated host verified on every other. With two hosts fronted by different IdPs
  that share a group name, a cookie copied from host A would be re-evaluated against
  host B's role mapping using A's groups (cross-IdP privilege confusion). The signed
  payload now carries the issuing host and a gate rejects any cookie whose host is
  not its own (a mismatched cookie triggers a fresh login).
- **Geo whitelist (`countryAllow`) fails closed on unknown IPs by default.**
  Previously `onUnknown` defaulted to `allow` in all modes, so an IP the GeoIP
  database could not place in a country - unallocated space, stale-DB gaps, some
  cloud/VPN ranges - was admitted even by a `countryAllow: [US]` whitelist,
  defeating it. When `onUnknown` is unset, whitelist mode now defaults to `deny`;
  deny-list mode keeps `allow` (it only narrows). An explicit `onUnknown` still
  wins.
- **`defaultAction: deny` on a rule-less access list now denies.** An access list
  with no IP, basic-auth, or geo rules but an explicit `defaultAction: deny` (a
  deliberate "lock this host down") was treated as an empty, unrestricted list and
  silently allowed all traffic. Such a list now denies; an unset or `allow` default
  remains open, as before.
- **Session database created 0600 in a 0700 directory.** The SQLite session
  store (`session.db`) was previously created with default OS permissions (typically
  0644 dir / 0644 file), making session IDs and CSRF tokens world-readable. The
  parent directory is now created 0700 and the file is pre-created 0600 (and
  `chmod`'d on open to tighten pre-existing deployments).
- **`GET /version` no longer exposes the config-repo HEAD commit.** The response
  previously included a `configCommit` field containing the current HEAD SHA of the
  git-backed config repository, leaking internal state to any authenticated caller.
  The field has been removed; the response now contains only the binary version info.
- **`roleMapping.defaultRole: "admin"` now requires explicit opt-in.** Setting
  `defaultRole` to `"admin"` without also setting `roleMapping.allowDefaultAdmin:
  true` is rejected at validation time. Without group gating, `defaultRole: "admin"`
  grants full admin to every user the IdP authenticates; the new required field stops
  that from happening silently by config typo. Unknown `defaultRole` values are also
  rejected (previously silently treated as deny).
- **Per-IP rate limit on OIDC admin login starts.** A client IP may initiate at
  most 30 OIDC login flows within any 10-minute window. Attempts beyond the cap
  receive a 502 immediately, before any redirect to the IdP. This prevents a single
  client from exhausting the global pending-login budget and blocking OIDC logins for
  all users. Legitimate flows behind shared NAT are unaffected by the generous cap.
- **Data-plane SSO signing key is now persisted across restarts.** When
  `GPM_SSO_SIGNING_KEY` is not set, gpm previously generated a random ephemeral key
  per process, invalidating all per-host OIDC sessions on every restart. The key is
  now generated once and written to `<cert-dir>/sso_signing.key` (0600) on first
  use, so sessions survive restarts. If the file is found corrupt on startup it is
  renamed to `sso_signing.key.corrupt` and a fresh key is generated and persisted.
  `GPM_SSO_SIGNING_KEY` still takes precedence when set.
- **Forward-auth identity strip no longer breaks a proxied Authentik.** The
  baseline identity-header denylist stripped the entire `X-Authentik-*` request
  family from untrusted peers, which also removed Authentik's own CSRF token header
  (`X-authentik-CSRF`, read from the `authentik_csrf` cookie and sent on every
  flow-executor API POST). When Authentik itself is proxied through gpm, every
  admin login failed with "CSRF Failed: CSRF token missing". `X-Authentik-Csrf` is
  now exempt from the strip — it is a CSRF token validated against a cookie, not an
  identity assertion, so forwarding it is no escalation risk.
- Remediated an internal security review (8 high / 3 medium / 11 low): identity
  headers stripped from untrusted peers; CSRF double-submit token + same-origin
  guard on the admin API; literal-secret commit guard; API secret redaction;
  object-name validation closing config-path traversal; fail-closed access-list
  and auth gates; XFF honored only from configured trusted proxies.
- Remediated a second review (1 high / 7 medium / 3 low) on the data plane and
  admin auth added since:
  - **Request-path normalization (high)**: location and guard matching now run on
    a cleaned path (`path.Clean`, decoded, RawPath dropped) matched on a segment
    boundary, and the cleaned path is forwarded upstream — closing
    `/x/../admin`, `//admin`, encoded-traversal, and `/reports-evil` over-match
    bypasses of per-location auth and guard rules.
  - **`${FILE:}` secret confinement**: file-backed secrets resolve only within an
    allowlisted root (`/run/secrets` by default, override `GPM_SECRET_FILE_ROOTS`),
    rejecting relative and out-of-root/`..` paths.
  - **Open-redirect hardening**: `sanitizeReturnTo` now rejects backslash,
    protocol-relative, and control-character return URLs.
  - **OIDC login binding**: a short-lived state cookie set at login start is
    compared (constant-time) at the callback, blocking login-CSRF / fixation.
  - **Baseline identity-header strip**: a fixed denylist (`Remote-User`,
    `X-Forwarded-User`, the `X-Auth-Request-*` / `X-Authentik-*` families, …) is
    stripped from untrusted peers regardless of the configured IdP.
  - **Per-host identity trust**: a proxy trusted by one host's IdP is no longer
    implicitly trusted to assert identity to other hosts (no global CIDR union).
  - **Importer bounds**: a per-table row cap fails an over-large source loudly.
  - **Session cookie**: `__Host-` prefix adopted when cookies are `Secure`
    (honouring `GPM_COOKIE_SECURE`, so a plain-HTTP admin deployment is unaffected).
  - **Admin security headers**: HSTS, `X-Content-Type-Options`, `X-Frame-Options`,
    CSP `frame-ancestors`, and `Referrer-Policy` on every admin/login response.
  - **Local-login throttling**: per-client-IP lockout after repeated failures,
    bounded pending-login/throttle maps, and sliding session expiry.

[Unreleased]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.16...HEAD
[1.0.16]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.15...v1.0.16
[1.0.15]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.13...v1.0.15
[1.0.13]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.10...v1.0.13
[1.0.10]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.9...v1.0.10
[1.0.9]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.8...v1.0.9
[1.0.8]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.7...v1.0.8
[1.0.7]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.5...v1.0.7
[1.0.5]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.4...v1.0.5
[1.0.4]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.1...v1.0.4
[1.0.1]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/Rake-Pro/go-proxy-manager/releases/tag/v1.0.0
