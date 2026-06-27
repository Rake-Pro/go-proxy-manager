# FEATURES.md

Target feature set for `go-proxy-manager`, synthesized from **NPM** (baseline),
**NPMplus** (hardened/expanded fork), and the **lessons from our own NPM+OIDC
fork**. Idea stage - this is a backlog/design doc, not a spec of shipped code.

**Legend (source):**
`[NPM]` in upstream NPM · `[NPM+]` added by NPMplus · `[FORK]` added by our OIDC
fork · `[GOAL]` net-new design goal for the Go version (gap we want to close).

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

## 3. Our OIDC fork additions `[FORK]`

- `[FORK]` Native **generic OIDC admin login** (auth-code + PKCE, discovery,
  encrypted secret at rest), **account linking** to existing local accounts,
  declarative provider config via `OIDC_PROVIDER_*` env or
  `/data/oidc-providers.json` (secret placeholders).
- `[FORK]` Local login preserved alongside OIDC (anti-lockout).

## 4. Lessons / gaps to close `[GOAL]`

Distilled from running the fork against Authentik. These are the design north
stars - the reasons a rewrite is worth considering.

1. **Native trusted forward-auth login.** NPM ignores `X-authentik-*` headers, so
   forward-auth can gate the page but can't log you in - forcing OIDC *and*
   forward-auth = double Authentik hops + an unavoidable "Login with Authentik"
   click. `[GOAL]` Accept a trusted forward-auth identity to establish the admin
   session directly: one authentication, no second click.
2. **First-class OIDC/SSO** (not an unmerged PR) with **IdP group/claim -> role
   mapping**, so SSO users become admins by claim - replacing manual account
   linking and the "auto-provisioned = role user only" limit.
3. **Real SSO-only mode with safe break-glass.** NPM can't hide the local form
   (anti-lockout), and our break-glass was an unauthenticated plaintext `:81`.
   `[GOAL]` Enforce SSO-only while providing a *proper* break-glass:
   localhost-only admin / time-limited emergency token / CLI reset - never an open
   port. (NPMplus's `OIDC_DISABLE_PASSWORD` is the blunt version; do it safely.)
4. **MFA delegation.** Don't double-prompt (NPM TOTP + Authentik MFA). Trust IdP
   `acr/amr`; keep local TOTP only for local/break-glass accounts.
5. **Explicit external base URL.** Stop deriving origin from `X-Forwarded-*` (the
   redirect_uri port/scheme footgun that broke our login). Configure the canonical
   public URL once.
6. **Auth / middleware as native config, not raw nginx snippet injection.** The
   duplicate `location /` collision broke TLS while `nginx -t` still passed.
   `[GOAL]` Forward-auth, access lists, headers = first-class config objects with
   no textual-collision class of bug.
7. **Declarative, GitOps-friendly provider/host config** (file+env with secret
   placeholders) as a supported path, not DB/UI-only. Keep the good part of the
   fork.
8. **Honest version + working update check.** The false "v2.15.1 available" came
   from a hardcoded `package.json`. `[GOAL]` Report the real build version;
   configurable (or disable-able) upstream check that's correct for self-built
   images.
9. **Clean, reversible, documented DB migrations + backup/restore.** The fork
   reused `setting`/`auth` tables and the merge-back path was uncertain. Own the
   schema; make migrations reversible and the data portable (no one-way trap).
10. **Minimal, auditable dependencies.** The whole motivation: escape the Node
    advisory churn. Small vendored Go dep set, no runtime pip/yarn fetches.

## 5. Target feature set for the Go version (proposed tiers)

**MVP (parity + the headline auth win):**
- Proxy / redirect / stream (TCP/UDP) / 404 hosts; config generation (nginx or a
  native Go reverse proxy - TBD in design).
- Let's Encrypt HTTP-01 + DNS-01 wildcard (broad provider support incl. route53),
  custom certs. ACME-server-agnostic (LE/ZeroSSL/Google) `[from NPM+]`.
- Access lists: IP allow/deny + basic auth, **multiple per host/location**
  `[from NPM+]`.
- **First-class SSO**: OIDC login + **trusted forward-auth header login** +
  group->role mapping + MFA delegation + **safe SSO-only mode w/ break-glass**
  `[GOAL]`.
- Admin UI (HTTPS) + REST API + audit log + multi-user; declarative config path.
- HTTP/2, websockets always-on, security headers + HSTS by default.
- Honest versioning, reversible migrations, minimal deps `[GOAL]`.

**Tier 2 (hardening / NPMplus-class):**
- HTTP/3 (QUIC), Brotli/zstd, hardened TLS (TLS1.3, optional 1.2 off), OCSP for
  non-LE CAs.
- WAF hook (CrowdSec bouncer/AppSec integration), GeoIP2 geoblocking.
- mTLS client-cert validation, proxy protocol.
- GoAccess-style log dashboard or built-in metrics (consider **Prometheus** -
  absent from both today).

**Tier 3 / nice-to-have:**
- Load balancing upstreams via UI, file server / FancyIndex, PHP-FPM, ECH,
  ML-KEM, MPTCP.

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

- **route53** must work as a DNS-01 provider (NPMplus dropped it; we use AWS).
- **Permissive license / clean-room** - don't inherit AGPLv3 by copying NPMplus.
- **No one-way migration trap** - portable data, documented schema.
- **Broad arch + no surprise CPU baseline** (NPMplus needs x86-64-v2).
- **Cloudflare-proxy interplay** - if proxied, document what it overrides; we run
  DNS-only today.
- **DNS-01 wildcard via the homelab's existing setup** (shared `npm-1` wildcard).

## 8. Sources

- NPM README + advanced-config/setup docs (`NginxProxyManager/nginx-proxy-manager`)
- NPMplus README + compose.yaml (`ZoeyVid/NPMplus`)
- This homelab's NPM+OIDC fork experience (`example/nginx-proxy-manager`)
