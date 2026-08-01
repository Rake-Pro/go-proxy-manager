# FEATURES.md

Target feature set and roadmap for `go-proxy-manager`, synthesized from **NPM**
(baseline), **NPMplus** (hardened/expanded fork), and **lessons from running an
OIDC-enabled reverse proxy in production**.

> **Status:** this is the roadmap, not a status board. The **P0** tier (proxy/
> redirect/stream/dead hosts, DNS-01 + custom certs, IP access lists, OIDC +
> forward-auth + auth-request, the NPM importer, typed per-host middleware, REST
> API + web UI, git-backed config) is **implemented** today; later tiers are
> aspirational. For what actually ships and how to use it, see
> [README.md](README.md) and [docs/](docs/).

**Legend (source):**
`[NPM]` in upstream NPM · `[NPM+]` added by NPMplus · `[FORK]` added by an
earlier OIDC-enabled iteration of this idea · `[GOAL]` net-new design goal for
the Go version (gap we want to close).

---

## 1. NPM baseline (MIT)

- `[NPM]` Host types: **proxy hosts**, **redirection hosts**, **stream hosts
  (TCP/UDP)**, **404/dead hosts**.
- `[NPM]` SSL: **Let's Encrypt** via certbot (HTTP-01 + DNS-01), **wildcard** via
  DNS-01, **custom certificates**.
- `[NPM]` Access control: **Access Lists** - HTTP basic auth (user/pass) +
  **IP allow/deny**.
- `[NPM]` **Custom nginx snippets** via `/data/nginx/custom/*.conf` (per scope:
  root/http/events/stream/server_*).
- `[NPM]` **Admin UI** (Tabler, port 81 HTTP) + **multi-user, permissions,
  audit log** + **REST API** (Swagger).
- `[NPM]` **Databases**: SQLite (default), MySQL/MariaDB, PostgreSQL.
- `[NPM]` Runtime: amd64/arm64, IPv6 (disableable), PUID/PGID non-root, Docker
  secrets via `__FILE`, healthcheck, Cloudflare/CloudFront IP-range fetch.
- `[NPM]` Websockets (per-host toggle), HTTP/2.

## 2. NPMplus additions (AGPLv3) - reference only

### Security / WAF / threat protection
- `[NPM+]` **CrowdSec** - nginx bouncer (IP blocking) + **AppSec** inline WAF.
- `[NPM+]` **OpenAppSec** WAF (loadable module; optional cloud or local policy).
- `[NPM+]` **Anubis** bot/anti-scraper challenge (per host, via auth_request).
- `[NPM+]` Always-on **security headers**, enforced **HSTS**, **secure cookies +
  CSP** (token not in localStorage).
- `[NPM+]` Hardened container: `cap_drop: ALL`, `no-new-privileges`.
- `[NPM+]` Per-host toggles: URI sanitization, noindex + crawler blocking.
- (ModSecurity, Fail2ban, OWASP CRS, Prometheus metrics: **not** present in either.)

### Forward-auth / SSO integrations (all via nginx `auth_request`)
- `[NPM+]` Authelia, **Authentik** (single-app, embedded outpost), OAuth2Proxy,
  Tinyauth, VoidAuth, Anubis - selectable per host/location, upstream via env.
- `[NPM+]` **OIDC admin login** for the panel: `OIDC_ISSUER_URL`,
  `OIDC_CLIENT_ID/SECRET`, `OIDC_REDIRECT_DOMAIN`, `OIDC_REQUIRE_VERIFIED_EMAIL`,
  **`OIDC_DISABLE_PASSWORD`** (disables local login entirely).
- `[NPM+]` **mTLS** - custom CA cert upload for client-cert validation.

### Protocols / performance
- `[NPM+]` **HTTP/3 (QUIC)** (optional BPF), **Brotli + zstd** compression,
  **TLS certificate compression** (zlib-ng+brotli).
- `[NPM+]` **ECH** (Encrypted Client Hello) with auto key rotation.
- `[NPM+]` **ML-KEM / post-quantum TLS**, hardened TLS, optional TLS 1.2 disable.
- `[NPM+]` **Proxy protocol**, **MPTCP**, upstream keep-alive, custom nginx build
  (aws-lc).

### TLS / certificates
- `[NPM+]` Configurable **ACME server**: Let's Encrypt (default, shortlived
  profile), **ZeroSSL**, **Google Public CA**, any ACME server; **EAB** support.
- `[NPM+]` **OCSP stapling** + must-staple (non-LE CAs only - LE dropped OCSP).
- `[NPM+]` ACME key type/size choice (ecdsa default), custom-cert editing,
  **reuse-key** toggle (TLSA/pubkey pinning).
- `[NPM+]` DNS-01 via certbot plugins downloaded at runtime - **route53 NOT
  supported**; a handful of plugins replaced/non-functional.

### Observability / GeoIP
- `[NPM+]` **GoAccess** dashboard (port 91) with optional **GeoIP2**; log
  rotation to disk.
- `[NPM+]` **Geoblocking** (GeoIP2 module, per-scope country allow/deny).

### Proxy/location features
- `[NPM+]` **Multiple access lists per host AND per location** (top-down),
  location lists independent of host.
- `[NPM+]` **Load balancing** (custom upstream blocks), **file server / static**
  (forward scheme `path`), **PHP-FPM**, **FancyIndex**.
- `[NPM+]` Per-host: X-Frame-Options control, spoof Host header, request/response
  buffering, upstream compression passthrough, TLS-to-upstream for streams.

### Admin / runtime
- `[NPM+]` Admin UI on **HTTPS** (port 81), Swagger at `/api/docs`, **CLI password
  reset**, configurable initial admin + cookie secret, local TOTP QR generation.
- `[NPM+]` Alpine base, **amd64-v2 / arm64 only** (no x86-64-v1, no armv7),
  SQLite recommended (others "unsupported"), **one-way** migration from NPM,
  loadable nginx modules via env, `network_mode: host` for CrowdSec.
- `[NPM+]` Note: Cloudflare proxy discouraged (overrides HSTS/H3/TLS); DNS-only OK.

## 3. Earlier OIDC-enabled iteration `[FORK]`

- `[FORK]` Native **generic OIDC admin login** (auth-code + PKCE, discovery,
  encrypted secret at rest), **account linking** to existing local accounts,
  declarative provider config via `OIDC_PROVIDER_*` env or
  `/data/oidc-providers.json` (secret placeholders).
- `[FORK]` Local login preserved alongside OIDC (anti-lockout).

## 4. Lessons / gaps to close `[GOAL]`

Distilled from running an OIDC-enabled reverse proxy against Authentik in
production. These are the design north stars - the reasons this project is
worth building.

1. **Native trusted forward-auth login.** NPM ignores `X-authentik-*` headers, so
   forward-auth can gate the page but can't log you in - forcing OIDC *and*
   forward-auth = double Authentik hops + an unavoidable "Login with Authentik"
   click. `[GOAL]` Accept a trusted forward-auth identity to establish the admin
   session directly: one authentication, no second click.
2. **First-class OIDC/SSO** (not an unmerged PR) with **IdP group/claim -> role
   mapping**, so SSO users become admins by claim - replacing manual account
   linking and the "auto-provisioned = role user only" limit.
3. **Real SSO-only mode with safe break-glass.** NPM can't hide the local form
   (anti-lockout), and the break-glass in an earlier iteration was an unauthenticated plaintext `:81`.
   `[GOAL]` Enforce SSO-only while providing a *proper* break-glass:
   localhost-only admin / time-limited emergency token / CLI reset - never an open
   port. (NPMplus's `OIDC_DISABLE_PASSWORD` is the blunt version; do it safely.)
4. **MFA delegation.** Don't double-prompt (NPM TOTP + Authentik MFA). Trust IdP
   `acr/amr`; keep local TOTP only for local/break-glass accounts.
5. **Explicit external base URL.** Stop deriving origin from `X-Forwarded-*` (the
   redirect_uri port/scheme footgun that broke login in an earlier iteration).
   Configure the canonical public URL once.
6. **Auth / middleware as native config, not raw nginx snippet injection.** The
   duplicate `location /` collision broke TLS while `nginx -t` still passed.
   `[GOAL]` Forward-auth, access lists, headers = first-class config objects with
   no textual-collision class of bug.
7. **Declarative, GitOps-friendly provider/host config** (file+env with secret
   placeholders) as a supported path, not DB/UI-only. Keep the good part of that
   approach.
8. **Honest version + working update check.** The false "v2.15.1 available" came
   from a hardcoded `package.json`. `[GOAL]` Report the real build version;
   configurable (or disable-able) upstream check that's correct for self-built
   images.
9. **Clean, reversible, documented DB migrations + backup/restore.** An earlier
   iteration reused `setting`/`auth` tables and the merge-back path was
   uncertain. Own the schema; make migrations reversible and the data portable
   (no one-way trap).
10. **Minimal, auditable dependencies.** The whole motivation: escape the Node
    advisory churn. **Prefer the Go stdlib (and `golang.org/x/...`) wherever
    feasible**; zerolog is the one accepted logging dep; justify and vet every
    other import. No runtime pip/yarn fetches.
11. **Import from an existing NPM / NPMplus install** `[GOAL]`. Best-effort,
    one-time importer that reads an existing NPM/NPMplus `/data` (SQLite DB +
    `nginx/proxy_host` confs + certs) and maps proxy/redirect/stream/404 hosts,
    access lists, and certificates into our schema. Explicitly **not** a
    guaranteed long-term-supported path - the goal is to migrate the current
    homelab config once and tweak by hand afterward. Lossy/unsupported fields
    should be reported, not silently dropped.

## 5. Target feature set - prioritized (my needs first)

Ordering principle: **build for the homelab/owner first** - match and extend the
feature set of a typical NPM-based setup - then layer parity + community
features on top. P0 is architected so nothing in P1-P3 requires reworking
earlier tiers or duplicating work (see "Architecture for extension").

### P0 - homelab must-have
- **Proxy hosts** for `*.example.com`, TLS termination, HTTP/2, websockets.
- **Let's Encrypt wildcard via DNS-01** for the homelab DNS provider (the
  `*.example.com` wildcard, like the existing wildcard cert); custom certs.
- **IP access lists** (LAN / VPN gating) - keep the current allow-list model.
- **Authentik done right** (the reason this exists): native **OIDC admin login**
  **+ trusted forward-auth header login** = one sign-in, no double prompt;
  group->role mapping; MFA delegation; **safe SSO-only mode with a real
  break-glass** (not an open `:81`).
- **Import the existing NPM/NPMplus config** (one-time, best-effort) to cut over
  (section 4 #11).
- **First-class per-host config / middleware** escape hatch - never get blocked
  like the forward-auth-snippet saga; composable middleware, not raw nginx text.
- Ops hygiene: Admin UI (HTTPS) + API, **declarative/GitOps config** with secret
  placeholders, **honest versioning**, reversible migrations, **minimal deps**,
  GHCR multi-arch build, digest-pinned.
- **DNS that follows the config** (✓ shipped, phase 1): a per-host `dns` policy
  publishes CNAMEs to the LAN resolver (Pi-hole v6) and/or the public zone
  (Cloudflare), so adding a host no longer means a second manual DNS edit in two
  places. Full-state reconcile; deletion strictly limited to records gpm
  *recorded creating*, in the git-backed ownership ledger
  `config/dns-ledger.yaml` — inferring ownership from a record's target instead
  cost an operator 19 hand-written CNAMEs on 2026-08-01. First enable adopts what
  matches and touches nothing else; an adopted record is *released* rather than
  deleted when the config stops asking for it, so adoption never becomes a licence
  to destroy somebody else's record. `GET /api/dns-sync/plan` previews the run.
  **Phase 2 (✓ shipped): Kubernetes Ingress discovery** — an annotated
  cluster `Ingress` becomes a template-derived, managed-labelled proxy host that
  feeds the same reconciler, so a cluster service no longer has to be hand-entered
  first. Read-only against the cluster (stdlib REST, no `client-go`), opt-in per
  Ingress, one commit per reconcile, and freeze-not-delete when the API server
  cannot be read. Heterogeneous fleets are handled by **operator-defined named
  profiles** an Ingress selects with `gpm.rake.pro/profile` — the annotation
  carries a name and nothing else, so an untrusted manifest picks among chains
  you authored instead of describing one, and an undefined name skips rather than
  silently downgrading to the default. See
  [docs/design/ingress-discovery.md](docs/design/ingress-discovery.md).

### P1 - high-value, layer in next (parity + best community asks)
- Redirect / stream (TCP/UDP) / 404 hosts (✓ shipped: the data plane now serves
  redirect, dead, and raw TCP/UDP stream hosts, not just proxy hosts); multiple
  access lists per host/location.
- Backup / export / restore ★ (✓ shipped: gzip-tar export + validated restore +
  config revert, with History-view UI), rate limiting ★ (✓ shipped: per-host,
  per-client-IP token bucket with 429 + Retry-After), dark mode (✓ shipped: the
  UI is dark-only by default, satisfying the original ask; remaining gap is an
  optional light-theme toggle),
  robots/no-index toggle (✓ shipped: per-host `robotsNoIndex` → `X-Robots-Tag`),
  custom timeouts (✓ shipped: per-host `timeouts.connectSeconds`/`readSeconds`,
  isolated per-host transport), load balancing / upstream groups (✓ shipped:
  first-class `UpstreamGroup` objects — failover / weighted round-robin /
  least-connections / ip-hash policies, signed sticky-session cookies with a
  server-enforced TTL, active TCP/HTTP health probes + passive connect-failure
  detection, `GET /api/upstream-health`, typed UI editor),
  access-log viewer (✓ shipped: in-memory ring + `GET /api/logs` + "Access Logs"
  view, gated on the access-log toggle) or metrics.
- Per-host OIDC relying-party gating on the data plane (✓ shipped: redirect →
  callback → signed SSO session cookie) and HSTS emission (✓ shipped).
- **Scoped API tokens** for automation (✓ shipped): bearer credentials minted
  server-side and shown once (only a SHA-256 digest is committed), with
  per-resource `read`/`write` scopes, optional expiry, instant revocation, and
  `admin`-only access to token management / settings writes / backup / restore /
  whole-config revert / pprof. The stored digest is never served, and reverting a
  token is refused so a rotation always means revocation. Closes the "scripting
  against the API means handing out an admin session" gap.

### P2 - hardening (NPMplus-class) + community gaps
- HTTP/3 (QUIC), hardened TLS (1.3; optional 1.2 off).
- GeoIP geoblocking (✓ shipped: `geo` on `AccessList` -
  `countryAllow`/`countryDeny`/`onUnknown`, fail-closed at write and at live
  evaluation, `GPM_GEOIP_DB` with a 5-minute hot-reload watch, `GET
  /api/capabilities` gates the SPA controls), mTLS client certs (✓ shipped,
  phase 1: per-host `tls.clientAuth` `require`/`optional` against a
  `ClientCA` trust anchor, enforced per request via SNI==Host + verified
  chain, `421` on mismatch; CRL/OCSP revocation and identity-passthrough
  headers are phase 2, still open), proxy protocol.
- **Inbound IPv6** (NPM had it, disableable): bind the data plane on v6 and make
  it actually reachable end-to-end. Gotchas learned in the field: with Docker
  `userland-proxy:false` you can't DNAT v6 -> a v4-only container, so the gpm
  container needs a real v6 address (Docker IPv6 + a routed prefix), not just a
  v4 port-map; preserve the real client v6 in access lists / X-Forwarded-For (the
  client-IP resolver must treat v6 the same as v4); and document that
  dynamic-prefix ISPs (e.g. a residential ISP that rotates the prefix on reconnect) need DDNS for
  the AAAA - a hardcoded SLAAC/EUI-64 address black-holes on every prefix change
  and breaks dual-stack clients via Happy Eyeballs while v4 silently masks it.
- Lifecycle **webhooks** ★ (✓ shipped: `settings.webhooks`, async best-effort POST
  per config change), host grouping/tagging ★ (✓ shipped: `tags` on every object +
  Proxy Hosts list chips/filter), multiple ACME servers (partial: per-cert
  `directoryURL` already works for any non-EAB CA; EAB support for ZeroSSL /
  Google Public CA still open), reusable DNS creds (✓ shipped: DNS providers are
  shared first-class objects; certificates reference one credential set by
  name).

### P3 - nice-to-have
- WebAuthn/passkeys for local admin login in IdP-less deployments; local auth
  capped at TOTP + WebAuthn, not applicable with OIDC.

### Not planned at this time
- Brotli/zstd compression (was P2)
- OCSP stapling (was P2)
- WAF/CrowdSec hook (was P2)
- Email notifications (was P2)
- SAML/LDAP login (was P2)
- PHP / file server (was P3)
- FancyIndex (was P3)
- ECH (was P3)
- ML-KEM (was P3)
- MPTCP (was P3)
- Anubis (was P3)
- cosign image signing (build-side) (was P3)

### Architecture for extension (so later tiers aren't rework/duplication)
- Model **hosts, certs, identity providers, access rules, middleware** as
  first-class typed config objects with a stable schema + versioned migrations
  from day one - so P1-P3 add new types/fields, never a rewrite.
- A **composable middleware chain** (rate-limit -> access-list -> auth -> guard
  -> headers -> rewrite -> WAF hook -> proxy) so new behaviors slot in as ordered steps -
  never the textual-collision bug that broke our forward-auth `location /`.
- **Pluggable interfaces**: ACME/DNS provider, identity provider
  (OIDC / forward-auth / SAML / LDAP), data store - adding one is implementing an
  interface, not touching core.
- The **importer reads into the same typed model** the UI/API/config use, so it
  can't diverge from the real schema.

## 6. Comparison matrix (NPM vs NPMplus)

| Feature | NPM | NPMplus |
|---|---|---|
| Proxy / redirect / stream / 404 hosts | yes | yes |
| HTTP/2 | yes | yes |
| HTTP/3 (QUIC) | no | yes |
| Brotli / zstd compression | no | yes |
| TLS cert compression | no | yes |
| ECH | no | yes |
| ML-KEM / post-quantum TLS | no | yes |
| TLS 1.2 disable | no | yes |
| Proxy protocol / MPTCP | no | yes |
| WAF: CrowdSec / OpenAppSec | no | yes |
| WAF: ModSecurity / Fail2ban | no | no |
| Always-on security headers | no | yes |
| HSTS | configurable | enforced |
| mTLS CA upload | no | yes |
| Geoblocking (GeoIP2) | no | yes |
| GoAccess analytics | no | yes |
| Log rotation to disk | no | yes |
| Prometheus metrics | no | no |
| Let's Encrypt | yes | yes |
| ZeroSSL / Google Public CA | no | yes |
| ACME profiles (shortlived) | no | yes |
| OCSP stapling | no | yes (non-LE only) |
| DNS-01 / wildcard | yes | yes (fewer providers) |
| route53 DNS challenge | yes | **no** |
| Custom cert editing | no | yes |
| Forward-auth (Authelia/Authentik/...) | no | yes (6 providers) |
| OIDC admin login | no | yes |
| Disable local password login | no | yes (env) |
| Multiple access lists per host | no | yes |
| PHP / file server / FancyIndex | no | yes |
| Load balancing (custom upstream) | no | yes |
| Websockets | toggle | always on |
| Admin via HTTPS | no | yes |
| Admin CLI password reset | no | yes (SQLite) |
| Database | SQLite / MySQL / PG | SQLite (others unsupported) |
| Base image | Debian/Ubuntu | Alpine |
| x86-64-v1 / armv7 | v1 yes / armv7 no | **no** / no |
| License | MIT | AGPLv3 |

## 7. Constraints to design around (learned from both)

- **Broad DNS-01 provider coverage** as a backlog item (NPMplus dropped several,
  incl. route53); choose providers per need - not a fixed or AWS-specific
  requirement.
- **Permissive license / clean-room** - don't inherit AGPLv3 by copying NPMplus.
- **No one-way migration trap** - portable data, documented schema.
- **Broad arch + no surprise CPU baseline** (NPMplus needs x86-64-v2).
- **Cloudflare-proxy interplay** - if proxied, document what it overrides; we run
  DNS-only today.
- **DNS-01 wildcard via the homelab's existing setup** (the existing wildcard cert).

## 8. Community-demanded features (issue-mined)

Mined from open GitHub issues by 👍 reactions + comment volume (2026-06-27).
**Key finding:** NPMplus has *already shipped* most of NPM's long-standing top
asks (OIDC on hosts, client certs, multiple access lists per host, named streams,
geoblocking, copy-host, i18n). So the strongest differentiation for the Go version
is the high-demand items that **neither** project serves well (marked ★).

| Feature | NPM demand (issue, 👍/comments) | NPMplus | Go-version call |
|---|---|---|---|
| Dark mode UI | #707/#3538, ~169👍 | likely (new frontend) | MVP UI |
| Backup / export / restore in UI ★ | #168, ~151👍 | no | **Yes** - underserved; pairs with portable-data goal |
| Mail proxying (SMTP/IMAP/POP3, SNI/STARTTLS) | #1110, 148👍; NPMplus #1756 open | partial (streams) | Tier 2 |
| Brute-force protection (Fail2Ban/CrowdSec) | #39/#1131, ~139👍 | yes (CrowdSec) | Tier 2 (WAF/bouncer hook) |
| SSO: OIDC **+ SAML** admin login | ~134👍 across #437/#1624/#5126 | OIDC yes, SAML no | MVP (OIDC, headline); SAML backlog |
| LDAP / AD admin login ★ | #159, ~89👍 | nginx LDAP module only, not admin login | Backlog - underserved for admin login |
| Client cert / mTLS | #768, ~82👍 | shipped | Tier 2 |
| HTTP/3 + QUIC | #1550, ~80👍 | yes | Tier 2 |
| Rate limiting ★ | #116, 56👍 | no native UI | **Yes** - underserved |
| Load balancing / upstream groups | #156, 69👍 | yes | MVP/Tier 2 with UI |
| GeoIP / geoblocking | #46, 51👍/128c; NPMplus #730 most-commented-open | module (community) | Tier 2 - do it cleanly/native |
| Custom SSL cert mgmt (local path, edit existing) | #87/#1618/#593, ~117👍 | shipped (edit custom certs) | MVP |
| Stream SNI/TLS | #1829, 53👍 | yes | Tier 2 |
| Anubis / bot protection | #4682, 36👍 (recent) | yes | Tier 3 / optional |
| Access-log viewer in UI | #401, 27👍 | GoAccess | Tier 2 (or metrics) |
| robots.txt / no-index toggle | #245, 35👍 | yes | MVP toggle |
| Custom timeouts | #257, 29👍 | config | MVP config |
| Offline fallback / custom location | #3512, 32👍/47c | custom locations | Tier 2 |

**From NPMplus issues/roadmap worth adopting:**
- **Webhooks / event hooks on host lifecycle** (#2191) ★ - neither ships it; strong
  fit with our GitOps/automation ethos. **Yes.**
- **Reusable / shared DNS provider credentials** (#2911) - QoL. Yes.
- **Host grouping / tagging / folders** (#2741) ★ - neither ships it; UX at scale. Yes.
- **Multiple ACME servers per instance** (#1332) - Tier 2.
- **WebAuthn / passkeys** (NPMplus roadmap #2440) - backlog; complements SSO.
- **Email notifications / password reset** (roadmap) - Tier 2.
- **cosign image signing** (roadmap) - adopt as a build-pipeline goal (supply chain).
- **2FA controls** (require TOTP after OIDC / admin-disable TOTP for others, roadmap)
  - folds into our MFA-delegation goal (#4 above).

**Best differentiation opportunities (★ = high demand, poorly served by both):**
backup/restore, rate limiting, LDAP admin login, lifecycle webhooks, host grouping.
Plus our own headline (native trusted forward-auth login + safe SSO-only) and the
config importer (section 4 #11), which no one offers as a clean path.

**Note on ACME:** NPM uses certbot, NPMplus uses certbot-with-fewer-providers, and
both have open "switch to acme.sh" asks. A Go implementation sidesteps that debate:
`golang.org/x/crypto/acme` keeps us near-stdlib for ACME (DNS providers
implemented in-house per need), or a vetted lib (lego/CertMagic) if broad
provider breadth outweighs the added dependency - decide per the minimal-deps rule.

## 9. Sources

- NPM README + advanced-config/setup docs (`NginxProxyManager/nginx-proxy-manager`)
- NPMplus README + compose.yaml (`ZoeyVid/NPMplus`)
- Operational experience running a self-hosted, OIDC-gated reverse proxy in production
