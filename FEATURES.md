# FEATURES.md

Roadmap for `go-proxy-manager`: what ships today, what is planned next, and what
is deliberately out of scope. [README.md](README.md) describes the product as it
stands; [docs/](docs/) is the reference.

Priority tiers are historical planning labels, not release gates: **P0** is the
core an operator cannot run without, **P1** high-value work layered on top,
**P2** hardening, **P3** nice-to-have.

## Status board

| Tier | Status | Notes |
|---|---|---|
| P0 | Shipped | Proxy/redirect/stream/parked hosts, DNS-01 + custom certs, IP access lists, OIDC + data-plane forward-auth + auth-request, typed per-host middleware, REST API + web UI, git-backed config. Two design goals were deliberately **reversed** after security review: trusted forward-auth *admin panel* login, and an in-band break-glass for SSO-only mode (design goals 1 and 3). |
| P1 | Shipped | Every P1 item is complete, including HTTP-01 challenge support. |
| P2 | Mostly shipped | GeoIP geoblocking, mTLS (phases 1-3: verification, CRL + identity passthrough, `client-cert` mode + issuance), lifecycle webhooks, operational notifications (ntfy/Discord/generic), host tagging, reusable DNS credentials, cosign image signing, the WAF/CrowdSec hook (`bouncer` middleware), PROXY protocol (v1+v2), inbound IPv6, gzip compression, custom error pages, ACME EAB (ZeroSSL / Google Public CA) and the fleet-wide TLS floor (`settings.tls.minVersion`) are all shipped. HTTP/3 is the only tier item not started. |
| P3 | Aspirational | Nothing in this tier is built. |

For what ships today and how to use it, see [README.md](README.md) and
[docs/](docs/).

## 1. Design goals

Distilled from running an OIDC-gated reverse proxy against Authentik in
production. These are the design north stars: the reasons this project exists.

1. **Native trusted forward-auth login.** A proxy that ignores `X-authentik-*`
   headers can gate a page with forward-auth but cannot log you in, so enabling
   OIDC *and* forward-auth costs a double IdP hop plus an unavoidable "Login
   with Authentik" click. **Goal:** accept a trusted forward-auth identity to
   establish the admin session directly: one authentication, no second click.
   **Reversed:** shipped, then removed; see `internal/auth/authenticator.go`.
   Minting an admin session from `X-*` identity headers on peer-IP/CIDR trust
   alone is a spoofing risk if anything inside the trusted CIDR forwards client
   headers, so trusted forward-auth login for the *admin panel* was dropped.
   The data-plane forward-auth that gates proxied hosts is unaffected: it
   never established an admin session and remains supported.
2. **First-class OIDC/SSO** with **IdP group/claim -> role mapping**, so SSO
   users become admins by claim: no manual account linking and no
   "auto-provisioned means role `user` only" limit.
3. **Real SSO-only mode with safe break-glass.** An anti-lockout local login
   form that cannot be hidden is not SSO-only, and an unauthenticated plaintext
   admin port is not a break-glass. **Goal:** enforce SSO-only while providing a
   *proper* recovery path (localhost-only admin, a time-limited emergency token
   or a CLI reset, never an open port.
   **Reversed:** SSO-only enforcement shipped; the break-glass half did not.
   `internal/model/settings.go` (`AdminAuthSettings.SSOOnly`) states the call
   explicitly: no in-band break-glass, deliberately: a network-position-trusted
   local door is itself a spoofing risk. Recovery from an SSO outage is by
   redeploying with local login re-enabled, not a built-in escape hatch.
4. **MFA delegation.** Don't double-prompt a user who already satisfied MFA at
   the IdP. Trust IdP `acr`/`amr`; keep local TOTP only for local/break-glass
   accounts.
   **Reversed:** local TOTP shipped for the break-glass admin
   (`GPM_LOCAL_ADMIN_TOTP_SECRET`, RFC 6238, `gpm totp-secret`), which is the
   half that matters: the SSO path was never double-prompted, because gpm has
   no local prompt on it. Delegation itself is **not planned**: TOTP applies
   only to the local admin, whom no IdP authenticates, so there is no redundant
   prompt to suppress. The field (`OIDCSpec.TrustIdPMFA`) is deprecated and gone
   from the UI, OpenAPI and the config reference.
5. **Explicit external base URL.** Stop deriving origin from `X-Forwarded-*` (the
   redirect_uri port/scheme footgun that broke login in an earlier iteration).
   Configure the canonical public URL once.
6. **Auth and middleware as native config, not raw config-file snippets.** A
   duplicate `location /` in an injected snippet once broke TLS while the
   config-file syntax check still passed. **Goal:** forward-auth, access lists
   and headers are first-class config objects, with no textual-collision class
   of bug.
7. **Declarative, GitOps-friendly provider and host config** (file plus env
   with secret placeholders) as a supported path, never DB- or UI-only.
8. **Honest version + working update check.** The false "v2.15.1 available" came
   from a hardcoded version string. **Goal:** report the real build version;
   configurable (or disable-able) upstream check that's correct for self-built
   images.
9. **Clean, reversible, documented DB migrations + backup/restore.** An earlier
   iteration reused `setting`/`auth` tables and the merge-back path was
   uncertain. Own the schema; make migrations reversible and the data portable
   (no one-way trap).
10. **Minimal, auditable dependencies.** The whole motivation: keep the
    advisory and patch surface small. **Prefer the Go stdlib (and `golang.org/x/...`) wherever
    feasible**; zerolog is the one accepted logging dep; justify and vet every
    other import. No runtime pip/yarn fetches.

## 2. Feature tiers

Ordering principle: **build for the homelab operator first**, then layer
hardening and breadth on top. P0 is architected so nothing in P1-P3 requires
reworking earlier tiers or duplicating work (see "Architecture for extension").

### P0: must-have (shipped)
- **Proxy hosts** for `*.example.com`, TLS termination, HTTP/2, websockets.
- **Let's Encrypt wildcard via DNS-01** for the homelab DNS provider (the
  `*.example.com` wildcard, like the existing wildcard cert); custom certs.
- **IP access lists** (LAN / VPN gating): keep the current allow-list model.
- **Authentik done right** (the reason this exists): native **OIDC admin login**
  with IdP group->role mapping; **SSO-only mode** (shipped, `adminAuth.ssoOnly`)
  with no in-band break-glass by design (design goal 3, reversed from the
  original goal). Trusted forward-auth *admin panel* login and MFA delegation
  were also reversed/deferred (design goals 1 and 4); trusted forward-auth still
  gates data-plane hosts, just not the admin panel.
- **First-class per-host config / middleware** escape hatch: never get blocked
  like the forward-auth-snippet saga; composable middleware, not raw nginx text.
  Shipped both ways: a reusable `Middleware` object for a gate shared across a
  fleet, and an inline `auth` / `rateLimit` block written on the host or location
  itself for the small deployment that wants one gate on one host.
- **Path and Host escape hatches** (shipped): the routing equivalent of the
  middleware escape hatch, so fronting a backend that expects a different path or
  virtual host never needs a config snippet. `location.stripPrefix` removes the
  matched prefix; `upstream.path` prefixes a base path (host upstream, location
  upstream, and each upstream-group member); `upstream.hostHeader` sends the
  upstream's own `host:port` or an explicit hostname instead of the client's
  Host; and the `rewrite` middleware adds boundary-matched `prefixRules` and
  `^`-anchored `regexRules` beside the existing exact `replacePath`. Composition
  is **location match -> stripPrefix -> rewrite -> upstream.path -> upstream**,
  all inside the security chain, so every gate still evaluates the original
  client path.
- Ops hygiene: Admin UI (HTTPS) + API, **declarative/GitOps config** with secret
  placeholders, **honest versioning**, reversible migrations, **minimal deps**,
  GHCR multi-arch build, digest-pinned.
- **DNS that follows the config** (shipped, phase 1): a per-host `dns` policy
  publishes CNAMEs to the LAN resolver (Pi-hole v6) and/or the public zone
  (Cloudflare), so adding a host no longer means a second manual DNS edit in two
  places. Full-state reconcile; deletion strictly limited to records gpm
  *recorded creating*, in the git-backed ownership ledger
  `config/dns-ledger.yaml`: inferring ownership from a record's target instead
  cost an operator 19 hand-written CNAMEs on 2026-08-01. First enable adopts what
  matches and touches nothing else; an adopted record is *released* rather than
  deleted when the config stops asking for it, so adoption never becomes a licence
  to destroy somebody else's record. `GET /api/dns-sync/plan` previews the run.
  **Phase 2 (shipped): Kubernetes Ingress discovery**: an annotated
  cluster `Ingress` becomes a template-derived, managed-labelled proxy host that
  feeds the same reconciler, so a cluster service no longer has to be hand-entered
  first. Read-only against the cluster (stdlib REST, no `client-go`), opt-in per
  Ingress, one commit per reconcile, and freeze-not-delete when the API server
  cannot be read. Heterogeneous fleets are handled by **operator-defined named
  profiles** an Ingress selects with `gpm.rake.pro/profile`: the annotation
  carries a name and nothing else, so an untrusted manifest picks among chains
  you authored instead of describing one, and an undefined name skips rather than
  silently downgrading to the default. A derived host is a **full** proxy host:
  templates and profiles carry `robotsNoIndex`, `timeouts` and `tags` alongside
  the upstream/TLS/chains, so moving a service into discovery does not quietly
  drop settings it already had. The one deliberate exception is `locations`: path
  routing belongs to the cluster ingress controller, which does it from the same
  `Ingress`. See
  [design/ingress-discovery.md](design/ingress-discovery.md).
  **Docker container discovery** (shipped): the same reconciler and the same
  name-only profile contract for containers (`internal/docker`), opt-in per
  container via `gpm.rake.pro/enabled: "true"`, configured under
  `settings.dockerDiscovery`. The plan itself (ownership, freeze, fail-closed,
  operator-owned state) is shared with the Ingress reconciler in
  `internal/discovery`; the Engine API is read strictly read-only and reconciles
  are driven by container events with the poll interval as the fallback. See
  [design/docker-discovery.md](design/docker-discovery.md).

### P1: high-value (shipped)
- Redirect / stream (TCP/UDP) / 404 hosts (shipped: the data plane now serves
  redirect, dead, and raw TCP/UDP stream hosts, not just proxy hosts); multiple
  access lists per host/location.
- **Stream TLS/SNI + L4 access lists** (shipped): a TCP StreamHost gains
  `tls.mode: passthrough|terminate` with `sniMatch` and (for terminate) a
  `certificateRef`. Passthrough peeks the ClientHello with a hand-written,
  bounded, stdlib-only parser and routes on SNI **without decrypting**, so
  several hosts share one public port; terminate completes the handshake at gpm
  from the normal certificate store and forwards plaintext. Sharing a port is
  allowed only when every host on it is SNI-routed (validated), and `tls` is
  rejected on `udp`/`both` (no ClientHello to read). StreamHosts also take
  `accessLists`, evaluated on the client IP (IP rules and geo only, **before any
  backend dial**); a list carrying the deprecated `basicAuth` is rejected at
  validation rather than silently half-applied, since a raw stream cannot issue
  an HTTP challenge. Moving those users to an auth middleware with `mode: basic`
  removes the conflict: that middleware is HTTP-only and never reaches L4.
- **Consolidated client-IP trust** (shipped): `settings.trustedProxies` plus a
  per-host `proxyHost.trustedProxies` override is now the ONE setting deciding
  whose `X-Forwarded-For` gpm believes, and the address it derives (standard
  rightmost-untrusted walk; empty list = trust nobody) is what access lists, geo,
  rate limits, the basic-auth lockout, every `allowFrom` exemption, the access log
  and the upstream `X-Forwarded-For`/`X-Real-IP` all compare. It sits alongside
  two deliberately separate tiers (`proxyProtocol.trustedCIDRs` at L4 and
  `forwardAuth.trustedProxies` for identity headers) and closes the documented
  mTLS-bypass footgun, since a `client-cert` `allowFrom` now has a trusted-proxy
  source it can declare without inventing a dummy identity provider.
- Backup / export / restore (shipped: gzip-tar export + validated restore +
  config revert, with History-view UI), rate limiting (shipped: per-host,
  per-client-IP token bucket with 429 + Retry-After), light/dark/auto theme
  (shipped: a top-bar toggle cycles `auto` -> `light` -> `dark`, persisted in
  `localStorage`, applied before first paint; `auto` follows the OS
  preference, including on the login page),
  robots/no-index toggle (shipped: per-host `robotsNoIndex` -> `X-Robots-Tag`),
  custom timeouts (shipped: per-host `timeouts.connectSeconds`/`readSeconds`,
  isolated per-host transport), load balancing / upstream groups (shipped:
  first-class `UpstreamGroup` objects (failover / weighted round-robin /
  least-connections / ip-hash policies, signed sticky-session cookies with a
  server-enforced TTL, active TCP/HTTP health probes + passive connect-failure
  detection, `GET /api/upstream-health`, typed UI editor),
  access-log viewer (shipped: in-memory ring + `GET /api/logs` + "Access Logs"
  view, gated on the access-log toggle, flippable live via `PUT /api/logs`
  since the handler-switch change) or metrics (shipped: opt-in
  Prometheus `/metrics`, see P2).
- Per-host OIDC relying-party gating on the data plane (shipped: redirect ->
  callback -> signed SSO session cookie) and HSTS emission (shipped).
- **Scoped API tokens** for automation (shipped): bearer credentials minted
  server-side and shown once (only a SHA-256 digest is committed), with
  per-resource `read`/`write` scopes, optional expiry, instant revocation, and
  `admin`-only access to token management / settings writes / backup / restore /
  whole-config revert / pprof. The stored digest is never served, and reverting a
  token is refused so a rotation always means revocation. Closes the "scripting
  against the API means handing out an admin session" gap.
- **HTTP-01 challenge** (shipped): `ACMESpec.Challenge` accepts `http-01`; the
  ACME client-side ordering/validation (`internal/acme/order.go`,
  `HTTP01Store`) and the data-plane HTTP listener's
  `/.well-known/acme-challenge/<token>` responder
  (`internal/dataplane/acmechallenge.go`, `internal/acme/http01.go`) complete
  an order end-to-end with no DNS credential required.
- **Basic auth as an auth mode** (shipped): an auth middleware with
  `mode: basic` gates a host on local `username`/bcrypt `passwordHash` pairs, at
  the auth position of the chain, with `allowFrom` network exemptions, custom
  error pages, denial metrics and the same per-client-IP lockout the access-list
  form had. `AccessList.basicAuth`/`satisfyAny` are the deprecated form of the
  same thing (still enforced, `WARN` at load, removed in v2);
  `POST /api/access-lists/{name}/migrate-basic-auth` converts a list in one
  commit, with `?plan=1` as the dry run.
- **High availability, phase 1** (shipped: two-node active/standby, one
  floating VIP via keepalived, one writer (`GPM_HA_ROLE=leader`), a read-only
  follower that replicates config via `git pull --ff-only` and shares/rsyncs
  the cert dir; no clustering dependency). See [docs/ha.md](docs/ha.md).

### P2: hardening
- HTTP/3 (QUIC): not started. **Hardened TLS: shipped** as a fleet-wide switch,
  `settings.tls.minVersion` (`"1.2"` default | `"1.3"`), applied to every HTTPS
  listener, every stream host in `terminate` mode and an unknown SNI; a
  ProxyHost's own `tls.minTLSVersion` overrides it in either direction.
- GeoIP geoblocking (shipped: `geo` on `AccessList` with
  `countryAllow`/`countryDeny`/`onUnknown`, fail-closed at write and at live
  evaluation, `GPM_GEOIP_DB` with a 5-minute hot-reload watch, `GET
  /api/capabilities` gates the SPA controls), mTLS client certs (shipped,
  phase 1 **and** phase 2: per-host `tls.clientAuth` `require`/`optional`
  against a `ClientCA` trust anchor, enforced per request via SNI==Host +
  verified chain, `421` on mismatch, plus CRL revocation
  (`internal/dataplane/crl.go`, watched/hot-reloaded) and identity-passthrough
  headers (`X-Client-Cert-{SAN,Serial,Fingerprint,Subject}`); OCSP was never
  implemented and is not currently planned). **Phase 3** also shipped: a
  `client-cert` auth middleware honours `allowFrom` exactly as `auth-request`
  does (a trusted network skips the certificate requirement entirely), and a
  `ClientCA` with an optional signing key (`caKeyFile` / `caKeyPEM`) can **issue**
  client certificates: `POST /api/client-cas/{name}/issue` and a UI card mint an
  RSA-2048 client certificate and return a password-protected PKCS#12 bundle
  (`internal/clientcert`; RSA and the legacy PKCS#12 encoder are deliberate iOS /
  Android keychain compatibility choices, and the private key is never stored).
  The whole per-host switch-on is UI-driven too: the host editor's TLS section
  toggles `tls.clientAuth`, picks the ClientCA and the require/optional mode, with
  the forceSSL and enabled-CA preconditions greyed out rather than left to fail.
  A CA itself can be **generated** from the UI (`POST /api/client-cas/{name}/generate`:
  self-signed RSA-4096, `pathlen:0`, key stored by gpm at `0600`), so the whole
  mTLS path (CA, client certificates, revocation) is reachable with no external
  tooling; pasting a bring-your-own CA remains fully supported.
  Issuances are recorded as runtime state under `<certDir>/client-certs/`, which
  drives an `ok`/`expiring`/`expired` status (per-CA `expiryWarningDays`, default
  30), a pre-expiry UI banner and a per-certificate renew action. Renewal is
  deliberately manual end to end: a client certificate lives in a device keychain,
  so gpm cannot install it, and renewing does not revoke, which stays CRL-only.
- **Inbound PROXY protocol** (shipped): `settings.proxyProtocol`
  (`enabled`, `trustedCIDRs`, `timeout: 5s`) applies a hand-written v1 (text) and
  v2 (binary) parser (stdlib only, TLVs consumed and ignored) as a listener
  wrapper on `:80`/`:443` **and** on every TCP stream listener. The parsed source
  replaces the connection `RemoteAddr`, so access lists, geo, guards, rate
  limits, the basic-auth lockout, `X-Forwarded-For`, the access log, the OIDC
  gate and the metrics labels all see the real client with no per-feature wiring.
  The header is an unauthenticated claim, so it is honoured ONLY from a peer
  inside `trustedCIDRs` (otherwise the bytes are payload and the peer address
  stands, warned once per peer); a malformed header closes the connection and a
  stalled one is cut at `timeout`. Config is read live per connection, so a
  settings change needs no listener restart.
- **WAF/CrowdSec hook** (hook-only, not a bundled WAF; shipped): the
  `bouncer` middleware type (`internal/dataplane/bouncer.go`) asks an
  operator-run bouncer whether the client IP is banned and acts on its verdict:
  no embedded WAF, no rules, no CrowdSec engine. Two providers: `crowdsec`
  (LAPI bouncer flow, `ban`/`captcha` = deny, optional `stream: true` for a
  local in-memory decision set with CIDR support and background delta pulls) and
  `http` (a generic `2xx`=allow / `403`=deny hook any custom bouncer can
  implement). Sits after the access list and before auth, so an operator
  allow-list wins outright and a banned IP never reaches the IdP;
  `onError: fail-open|fail-closed` (default fail-open), bounded per-IP verdict
  cache with an error-verdict cap, `apiKey` as a `${ENV:}`/`${FILE:}` Secret.
- **Prometheus metrics** (shipped): opt-in `GET /metrics` on the admin
  listener (`-metrics` / `GPM_METRICS=1`, `404` when off), gated by admin role
  plus a dedicated `metrics:read` API-token scope so a scrape credential buys
  nothing else. The exposition is a ~300-line internal implementation
  (`internal/metrics`) rather than `prometheus/client_golang`, whose transitive
  tree would have multiplied this project's vetted dependency set for one
  read-only endpoint. Namespace `gpm_`: per-host request counts / latency
  histogram / byte totals / in-flight, upstream errors, WebSocket upgrades,
  access-control denials by tier (`rate-limit`, `access-list`,
  `access-list-auth`, `guard`, `geo`, `bouncer`), stream connections, ACME cert
  expiry + renew failures, DNS-sync and Ingress-discovery run/success
  timestamps and counts, HA role, build info, Go runtime basics. Host labels are
  the operator's ProxyHost/StreamHost **names**, never client `Host` headers, and
  each metric caps its series count, so no client can inflate cardinality.
- **cosign image signing** (build item, shipped): every release image is
  signed keylessly via GitHub Actions OIDC (`.github/workflows/release.yml`);
  `docs/deployment.md` documents `cosign verify`.
- **Inbound IPv6** (shipped): the data plane binds a bare `:80`/`:443`, which
  in Go is the IPv6 wildcard with v4-mapped addresses (one socket, both
  families, no toggle) and the client-IP resolver, access lists, geo rules and
  `X-Forwarded-For` treat a v6 client as its own v6 address rather than a v4
  stand-in (covered by an end-to-end `[::1]` test). What is left is deployment,
  not code, and `docs/deployment.md` now carries an **IPv6** subsection for it:
  the container needs a real v6 address (`enable_ipv6` + daemon `ip6tables`), not
  just a v4 port-map; `userland-proxy: false` (or host networking) is what keeps
  the real client address instead of the docker-proxy's. Still an operator
  concern outside gpm: dynamic-prefix ISPs (e.g. a residential ISP that rotates
  the prefix on reconnect) need DDNS for the AAAA: a hardcoded SLAAC/EUI-64
  address black-holes on every prefix change and breaks dual-stack clients via
  Happy Eyeballs while v4 silently masks it.
- **Operational notifications** (shipped: `settings.notifications`, async
  best-effort ntfy/Discord/generic-webhook alerts on ACME renewal failure,
  cert expiry (daily digest, not per-cert), upstream health flaps, and a
  frozen Kubernetes/Docker discovery reconciler; per-target event filtering,
  `GET /api/notifications/status`, `POST /api/notifications/{name}/test`).
- Lifecycle **webhooks** (shipped: `settings.webhooks`, async best-effort POST
  per config change), host grouping/tagging (shipped: `tags` on every object +
  Proxy Hosts list chips/filter), multiple ACME servers (shipped: per-cert
  `directoryURL` works for any ACME server, and External Account Binding
  (`acme.eab.kid` + `acme.eab.hmacKey`, `internal/acme/account.go`) covers CAs
  that require it (ZeroSSL, Google Public CA), reusable DNS creds (shipped: DNS providers are
  shared first-class objects; certificates reference one credential set by
  name).
- **Gzip response compression** (shipped): per-host `compression`
  (`enabled`, `minBytes` default 1024, `types` allow-list), stdlib
  `compress/gzip` only, pooled writers (`sync.Pool`). Honours the client's
  `Accept-Encoding`, buffers up to `minBytes` before deciding so a small body is
  never compressed, skips a response the upstream already encoded or that
  doesn't match `types`, and never touches `204`/`304`/`101`, a WebSocket
  upgrade, or anything that starts streaming (an early flush, which also covers
  `text/event-stream`). A compressed response gets `Content-Encoding: gzip`,
  `Vary: Accept-Encoding`, and loses its `Content-Length`. BREACH-aware: opt-in
  per host rather than default-on, since compressing a response whose size
  depends on attacker-controlled input reflected alongside a secret can leak
  that secret through the compressed size.
- **Custom error pages** (shipped): `settings.errorPages` (default) and a
  per-`ProxyHost` `errorPages` override (`dir` of `<status>.html` +
  `default.html` templates, and/or `inline` status->HTML), `html/template`
  (contextually escaped), rendered for errors **gpm itself** generates:
  upstream unreachable (502/504), access denied (access-list/guard/geo, 403),
  rate-limited (429), a dangling middleware/access-list reference (503), a
  parked host, and every terminal **auth-gate refusal** (forward-auth and
  client-cert 401/403, auth-request 403/502, the OIDC gate's callback failures,
  and the 503 an uncompilable auth middleware serves), never for the upstream's
  own error response unless its status is also listed in `interceptUpstream`,
  and never for a sign-in redirect or a page the identity provider itself served
  (the IdP response wins). Templates see `{{.Status}}`
  `{{.StatusText}}` `{{.Host}}` `{{.RequestID}}`; a host override wins over the
  settings-level pages; parse errors fail the config reload with a clear
  message; unconfigured behaviour is byte-identical to before this shipped.
- **Access-list remote sources + path-scoped rules** (shipped): an `AccessList`
  rule can be scoped to exact `paths` and `methods` (default `GET`/`HEAD`) and
  can draw its networks from a named remote feed (`sources[].url`, https only)
  instead of a literal `cidr`. Closes the gap a purely ANDed allow-list could not
  express: a monitoring provider's published prober addresses reach only the
  health endpoints of a host that is otherwise LAN/VPN-only. A leader-only
  fetcher (`internal/accesssync`) keeps the sets in the committed singleton
  `config/access-list-sources.yaml`, failing closed at every step (https-only,
  SSRF-guarded dialer refusing loopback/link-local/private/multicast, 1 MiB body
  cap, whole-fetch refusal on non-200 / empty / over-`maxEntries` / any
  unparseable line, previous set kept). An unchanged feed writes nothing, so
  there is no commit churn. `settings.accessListSync` (on by default) tunes the
  poll; `GET /api/access-list-sources/status` and
  `POST /api/access-list-sources/reconcile` expose it. Refused for a `StreamHost`
  the same way a `basicAuth` list is: there is no request path at L4.
- **Security response headers** (shipped): `settings.securityHeaders` (fleet
  default) and a per-`ProxyHost` `securityHeaders` that merges over it per key.
  Each header carries a **scope**: `all` (default), `generated-only` or
  `proxied-only`, selecting whether it lands on the responses **gpm itself**
  generates (auth-gate denials, sign-in redirects, error pages, path-rejection
  400, the no-such-host 404, misdirected 421, parked/redirect hosts), on proxied
  upstream responses, or both. Generated responses are injected at the same
  data-plane layer as HSTS, so denials get them even though the auth chain runs
  outside any headers middleware. Applied **set-if-absent**, so a proxied
  upstream's own `X-Frame-Options`/`Referrer-Policy`/etc. are never clobbered;
  `generated-only` additionally keeps app-breaking headers
  (`Content-Security-Policy` frame-ancestors, `Permissions-Policy`) off proxied
  apps entirely. The value is a bare string (scope `all`) or a `{value, scope}`
  object, so the legacy plain map stays valid. Opt-in (empty by default); names
  validated (no CR/LF, no hop-by-hop, case-insensitive dedup, unknown scope
  rejected); `Strict-Transport-Security` refused: HSTS is unchanged and
  separate. Resolves the auth-refusal/error-page "no `nosniff` on rendered pages"
  observation from BACKLOG. Authorable from the UI as well as from git: the
  Settings page ("Response security headers") edits the fleet default and the
  host editor edits that host's per-key override, both as name/value/scope rows.
- **Response-header stripping** (shipped): `settings.stripResponseHeaders`
  (fleet default) plus a per-`ProxyHost` `stripResponseHeaders` **unioned** with
  it, and an `ingressDiscovery` template/profile field so managed hosts inherit
  it. Removes backend-identifying headers (`Server`, `X-Powered-By`,
  `X-AspNet-Version`) from what an upstream sends. Applied in the reverse proxy's
  `ModifyResponse`, on the upstream's own header map, so it reaches only what the
  backend sent, never an injected `securityHeader`, HSTS, `X-Robots-Tag`, a
  forward-auth `Set-Cookie` refresh, gzip's `Content-Encoding` or a headers
  middleware's `setResponse`, and it covers a `101` WebSocket handshake, which
  the response-writer layer cannot see. gpm-generated responses have no upstream
  response and are untouched. Union (not per-key override) because a host must
  not be able to re-expose a header the fleet strips. Case-insensitive; opt-in
  (empty by default); invalid names, duplicates, hop-by-hop and response-semantic
  headers (`Content-Type`/`-Length`/`-Encoding`, `Vary`, `Location`,
  `Sec-WebSocket-*`) rejected.
  The headers middleware's `removeResponse` stays for per-middleware/per-location
  removal; this is the edge-wide mechanism. Authorable from the UI as well as
  from git: the Settings page ("Strip response headers") edits the fleet default
  and the host editor edits that host's additions, both as chip lists.

- [x] **Maintenance mode**: `proxyHost.maintenance` (per host) and
  `settings.maintenance.enabled` (fleet-wide, wins over the per-host flag) take
  hosts out of service for a downtime window: gpm answers `503` with a
  `Retry-After` (`settings.maintenance.retryAfterSeconds`, default 300) and never
  dials the upstream, while the host keeps its domains, certificate and DNS
  records. Both apply with no restart: the fleet-wide switch is read live, so it
  needs no router rebuild either. The page reuses the error-page seam (a custom
  maintenance page is the `503` entry in `errorPages`); the built-in fallback
  negotiates JSON / HTML / plain text on `Accept` and never sends an empty body.
  Redirect, parked and stream hosts keep serving; ACME HTTP-01 still renews
  certificates during a window. Ingress discovery preserves the flag as
  operator-owned state, like `disabled`. Deliberately out of scope: scheduled
  windows, per-path maintenance, and a per-host custom page editor beyond what
  `errorPages` already gives.

### P3: nice-to-have (not started)
- WebAuthn/passkeys for local admin login in IdP-less deployments; local auth
  capped at TOTP (shipped) + WebAuthn, not applicable with OIDC.

### Not planned at this time
- **MFA delegation** (was design goal 4, then P3). Trust IdP `acr`/`amr` to skip
  a redundant local prompt: there is no redundant prompt to skip. TOTP is
  demanded only on the LOCAL admin account, which no IdP authenticates, and the
  config field (`OIDCSpec.TrustIdPMFA`) was deprecated and dropped from the UI,
  OpenAPI and the config reference.
- Brotli/zstd compression (was P2)
- OCSP stapling (was P2)
- Email notifications (was P2)
- SAML/LDAP login (was P2)
- PHP / file server (was P3)
- FancyIndex (was P3)
- ECH (was P3)
- ML-KEM (was P3)
- MPTCP (was P3)
- Anubis (was P3)

### Architecture for extension
- Model **hosts, certs, identity providers, access rules, middleware** as
  first-class typed config objects with a stable schema + versioned migrations
  from day one, so P1-P3 add new types/fields, never a rewrite.
- A **composable middleware chain** (rate-limit -> access-list -> bouncer (WAF/deny
  hook) -> auth -> guard -> headers -> stripPrefix -> rewrite -> proxy) so new behaviors slot in as ordered steps,
  never the textual-collision bug that broke our forward-auth `location /`.
- **Pluggable interfaces**: ACME/DNS provider, identity provider
  (OIDC / forward-auth / SAML / LDAP), data store: adding one is implementing an
  interface, not touching core.

## 3. Constraints to design around

- **Broad DNS-01 provider coverage.** Four named REST providers (Cloudflare,
  DigitalOcean, Hetzner, deSEC) plus two generic solvers, RFC2136 and
  acme-dns, now cover any nameserver or registrar, not a fixed or
  AWS-specific requirement. route53 is not a named provider.
- **No one-way data trap**: portable data, documented schema, configuration
  that is plain YAML in git.
- **Broad architecture support**: amd64 and arm64, with no raised CPU
  baseline.
- **Cloudflare-proxy interplay**: if proxied, document what it overrides; we run
  DNS-only today.
- **DNS-01 wildcard via the homelab's existing setup** (the existing wildcard cert).

