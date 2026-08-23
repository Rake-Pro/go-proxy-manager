# Design: HTTP/3, GeoIP geoblocking, mTLS client certs

Status: **mixed.** mTLS (phases 1 and 2) and GeoIP geoblocking are **implemented**
(see CHANGELOG.md / FEATURES.md); HTTP/3 is **held** (decision
2026-08-23): not before Go stdlib ships an HTTP/3 server (golang/go#58547);
the quic-go quarantine design below stays as the fallback plan. Scope: three P2 edge features from [FEATURES.md](../../FEATURES.md).
This document records the intended design, the schema/wiring, and —
critically for this project — the **dependency decision** for each, so
implementation can start from a settled plan.

## Dependency posture (the deciding lens)

This project exists in large part to escape dependency/advisory churn, so every new import
is a deliberate cost. Summary for these three:

| Feature | New dependency | Verdict |
|---|---|---|
| mTLS client certs | **none** (stdlib `crypto/tls`) | cheapest; do first |
| GeoIP geoblocking | `oschwald/maxminddb-golang` (pure-Go mmdb reader) | acceptable, isolated, DB is operator-supplied |
| HTTP/3 (QUIC) | `quic-go/quic-go` + `quic-go/http3` (large, pure-Go) | the real cost; gate behind a build tag + runtime toggle |

Common thread: each new behaviour slots into an **existing** extension point
(per-SNI `GetConfigForClient`, the access-list client-IP resolver, the shared
data-plane handler) rather than reworking the core.

---

## 1. mTLS client-cert validation

> **Status: IMPLEMENTED (phases 1 and 2).** Shipped as designed below, with one
> enforcement detail firmed up during implementation: `tls.clientAuth`
> requires `forceSSL: true` and a resolvable `caRef` at config-validation
> time, and `require`/`optional` are enforced not just at the TLS handshake
> but **per request** - the negotiated SNI must resolve to the host's own
> `tls.Config` (closing an SNI != Host dodge, where a client handshakes
> against a different host's config and then targets the mTLS host by `Host`
> header) and, in `require` mode, the handshake must have produced a verified
> client-certificate chain; either failure gets `421 Misdirected Request`. An
> mTLS host is also redirected off the plaintext `:80` listener.
>
> **Phase 2 is now implemented too** - CRL revocation, identity passthrough and
> the `client-cert` auth-middleware mode - with these deltas from the proposal:
>
> - **Revocation is CRL-only; OCSP was not built.** `ClientCA` gains `crlFile`
>   (PEM or DER, confined **relative to the cert store** exactly like a custom
>   certificate's files - not a `${FILE:...}` secret placeholder, which trims
>   trailing bytes and would corrupt DER), `crlPEM` (inline, mutually exclusive)
>   and `crlPolicy`. Enforcement hangs off `tls.Config.VerifyPeerCertificate` on
>   the per-SNI config, as proposed; it additionally **validates the CRL's
>   signature against the CA bundle** (an unsigned or foreign-signed list is
>   refused, so dropping a file in the cert store cannot un-revoke anything) and
>   honours `nextUpdate`.
> - **A new fail-closed/fail-open policy field** the proposal did not name.
>   `crlPolicy` defaults to `fail-closed`: a CRL that is missing, unparseable,
>   foreign-signed or expired rejects **every** certificate for that CA. An
>   unusable CRL is deliberately **not** a router-build error - that would take
>   unrelated hosts down over one unreadable file - so, exactly like the GeoIP
>   database, the gate lives at request/handshake time and recovers by itself.
> - **CRL reload** happens on the existing config-reload path (the anchors are
>   rebuilt per `buildRouter`) *and* on a 5-minute mtime watch mirroring the
>   GeoIP `Resolver.Watch` pattern, started alongside the listeners.
> - **Identity passthrough** landed as `tls.clientAuth.identityHeaders`:
>   `subjectHeader` (default `X-Client-Cert-Subject`) plus opt-in `san` /
>   `serial` / `fingerprint` booleans with fixed header names, rather than a
>   free-form header map - fixed names are what lets the **baseline identity
>   denylist** cover them unconditionally. All four are stripped from untrusted
>   peers whether or not a host enables passthrough, a custom `subjectHeader`
>   joins that host's own strip set, and gpm sets them after the strip only from
>   a certificate with a verified chain.
> - **The `client-cert` auth-middleware mode** gates on a verified certificate
>   (401 without one) and maps subject -> role via `auth.clientCertRoles`; it is
>   the one auth mode with no `identityProvider`. Chain composition is unchanged
>   (middlewares nest, so two auth middlewares are AND, not OR): the "cert OR
>   SSO" shape is expressed by running the host in `optional` mTLS mode with
>   this middleware as its auth tier.
>
> Still open: **OCSP** (no stapling, no responder queries) and **mTLS over
> HTTP/3**, which cannot be verified until h3 ships.

**Goal.** Require (or optionally accept) a client certificate per host, verified
against an operator-supplied CA, enforced at the TLS handshake. Community ask
#768 (~82 upvotes); NPMplus already ships it.

**Dependency:** none. `crypto/tls` does it all (`ClientCAs`, `ClientAuth`,
`VerifyClientCertIfGiven` / `RequireAndVerifyClientCert`).

### Key insight: the per-SNI hook already exists

`Server.Start` wires `tls.Config.GetConfigForClient` →
`router.tlsConfigForSNI(serverName)`, today used so a host can pin a higher
minimum TLS version (`hostTLSConfig`). Client-cert enforcement is **per host**,
and `GetConfigForClient` is exactly the per-host seam: a host that wants mTLS
returns a `*tls.Config` carrying `ClientCAs` + `ClientAuth`. No listener-wide
change, no new handshake plumbing.

### Schema

New typed object for the trust anchor (keep it distinct from server certs):

```yaml
# config/client-cas/corp.yaml
name: corp
caPEM: ${FILE:/run/secrets/corp_client_ca.pem}   # or inline PEM (non-secret)
```

Per-host opt-in under `tls`:

```yaml
tls:
  certificateRef: wildcard
  clientAuth:
    caRef: corp           # ClientCA object name
    mode: require         # require | optional
```

- `require` → `tls.RequireAndVerifyClientCert` (no valid cert ⇒ handshake fails,
  request never reaches a handler).
- `optional` → `tls.VerifyClientCertIfGiven` (cert verified if presented, else
  the request proceeds — lets mTLS coexist with SSO/forward-auth as a fallback).

Validation: `clientAuth.caRef` must resolve to a `ClientCA`; the CA PEM must
parse to ≥1 cert at load; `mode` ∈ {require, optional}.

### Data-plane wiring

- `buildRouter`: when a host sets `clientAuth`, build its per-SNI `tls.Config`
  (merging with any `minTLSVersion` pin) with `ClientCAs = x509.CertPool(caPEM)`
  and the chosen `ClientAuth`. Store in `router.tlsConfigs[hostKey]` (the map
  already feeds `GetConfigForClient`). A host with *both* a TLS-version pin and
  mTLS needs the two merged into one config — today `hostTLSConfig` returns nil
  for the no-pin case, so the builder must compose rather than branch.
- Optional **identity passthrough** (phase 2): a host can map the verified client
  cert (CN / SAN) to a header for the upstream (e.g. `X-Client-Cert-Subject`),
  set in `serveHTTPS` from `r.TLS.PeerCertificates[0]`. This rides the existing
  identity-strip model (gpm-asserted header, stripped from untrusted peers).
- Optional **`client-cert` auth-middleware mode** (phase 2): gate at the chain's
  auth tier on cert presence and map subject → role, for "cert OR SSO" hosts in
  `optional` mode.

### Security / open questions

- **Revocation:** stdlib `Verify` does not check CRL/OCSP. Phase-1 ignored
  revocation; **phase-2 shipped the `VerifyPeerCertificate` CRL hook** described
  in the status note above. OCSP is still not implemented, and a ClientCA with no
  CRL configured still accepts a revoked-but-unexpired certificate.
- **HTTP/3 interaction:** quic-go honours the same `tls.Config` client-auth
  fields, but its `GetConfigForClient` timing differs; verify mTLS over h3
  separately if both ship.
- Phasing: phase 1 = handshake `require`/`optional` (small, high value). Phase 2
  = revocation + identity mapping + middleware mode. Both shipped.

**Effort:** S (phase 1), +M (phase 2). Both delivered.

---

## 2. GeoIP geoblocking

> **Status: IMPLEMENTED.** Shipped as designed below, on `AccessList.geo`
> (`countryAllow`/`countryDeny`/`onUnknown`), consulting `GPM_GEOIP_DB` /
> `-geoip-db` with a 5-minute mtime watch (picks up an out-of-band
> `geoipupdate` refresh with no restart). New `GET /api/capabilities`
> (`{"geoip":{"dbLoaded":bool}}`) lets the admin SPA grey out geo controls
> when no database is loaded. The fail-closed gate landed at a different
> (stronger) layer than first proposed - see the correction under "Security /
> open questions" below.

**Goal.** Allow/deny requests by client-IP country. Community ask #46 (51 upvotes,
128 comments); NPMplus #730 is its most-commented open issue. "Do it cleanly /
native" per FEATURES.

**Dependency:** `oschwald/maxminddb-golang` — pure-Go, no CGO, small, widely
used, reads MaxMind GeoLite2/GeoIP2 `.mmdb`. Hand-rolling an mmdb parser is not
worth it; this is a justified, isolated import. **No DB is bundled** (GeoLite2's
licence forbids redistribution): the operator mounts the file and refreshes it
(e.g. `geoipupdate` cron). DB-less ⇒ the feature is simply unavailable.

### Key insight: geo is another access-list dimension

Access lists already (a) resolve the **real** client IP (XFF honoured only from
trusted proxies, via `clientIPResolver`) and (b) run at the right chain position
(`access-list` tier). Country matching is just another rule dimension over the
same resolved IP — so **extend `AccessList`** rather than add a parallel
`geoblock` middleware. One client-IP resolver, one chain slot, one mental model.

### Schema

Operator points gpm at the DB (daemon flag / env, hot-reloaded on file change):

```
GPM_GEOIP_DB=/data/geoip/GeoLite2-Country.mmdb
```

Extend the AccessList object:

```yaml
name: no-sanctioned
defaultAction: allow
rules: [...]                 # existing IP/CIDR rules, evaluated first
geo:
  countryAllow: []           # if set, only these ISO-3166 alpha-2 pass
  countryDeny: [CN, RU, KP]  # these are rejected
  onUnknown: allow           # IP not in DB (incl. private/LAN) ⇒ allow | deny
```

Evaluation order within a list: explicit IP/CIDR rules → geo → `defaultAction`.
Private/loopback/link-local IPs have no country ⇒ governed by `onUnknown`. When
`onUnknown` is unset the default is **mode-dependent**: deny-list (`countryDeny`)
defaults to `allow` (it only ever narrows a default-allow posture, so the LAN is
never geo-blocked by accident); whitelist (`countryAllow`) defaults to `deny`
(fail closed) so an IP absent from the database - unallocated space, a stale-DB
gap, some cloud/VPN ranges - cannot slip past a "these countries only" gate. Set
`onUnknown` explicitly to override either default.

### Data-plane wiring

- A `geoResolver` holds an `atomic.Pointer[maxminddb.Reader]`, loaded at startup
  from `GPM_GEOIP_DB` and swapped on file change (mirror the cert reload pattern).
  Injected into `compileAccessList` alongside `clientIP`.
- The compiled access-list handler, after IP/CIDR rules, looks up the country for
  the already-resolved client IP and applies allow/deny.
- **Fail-closed, final shape (corrected from the proposal above):** the gate
  is not in `Config.Validate` - validation and reload never fail solely
  because a geo rule has no database (a boot with the DB missing does not
  `log.Fatal`; the affected hosts just start out denying). Instead there are
  two independent fail-closed layers: (1) **reject-at-write** -
  `store.Store.Save`/`SaveBatch`/`Restore`/`Revert` refuse (surfaced as HTTP
  400) to commit any config with `geo` rules while `GPM_GEOIP_DB` has no
  database loaded, so such a rule can never land in git; (2) **live
  fail-closed evaluation** - `accessList.ipAllowed` checks database
  availability at request-evaluation time, not baked into the compiled
  access list at build time, denying all traffic on the affected hosts while
  unavailable and auto-recovering the instant the watch loads a database,
  with no restart or config change. You still cannot half-configure geo; the
  enforcement point just moved from validate-time to write-time + eval-time.

### Security / open questions

- Must use the **trusted** client IP, never raw `X-Forwarded-For` — reuse the
  existing resolver; do not add a second IP path.
- GeoIP is advisory (VPNs, shared CGNAT, stale DB). Document it as defence-in-
  depth, not an authz boundary.
- DB licensing + update cadence belongs in `docs/deployment.md`.
- IPv6: mmdb covers it; the resolver already treats v6 like v4.

**Effort:** M.

---

## 3. HTTP/3 (QUIC)

**Goal.** Serve HTTP/3 over QUIC (UDP/443), advertised via `Alt-Svc`. Community
ask #1550 (~80 upvotes); NPMplus has it.

**Dependency — the real cost.** Go's stdlib has **no** HTTP/3 server. The only
realistic path is `quic-go/quic-go` + `quic-go/http3` (pure-Go, no CGO, used by
Caddy, actively maintained) — but it is a **large** surface, the opposite of the
minimal-deps thesis. Recommendation: adopt it **but quarantine it**:

- Put all quic-go-touching code behind a **build tag** (`//go:build http3`) so the
  **default binary does not link it** — operators who want the minimal surface get
  a gpm with zero QUIC code. A separate `gpm:http3` image variant carries it.
- Behind that, a **runtime toggle** (`GPM_HTTP3=1` / `--http3-addr`) actually
  enables the listener. Off ⇒ no UDP listener, no `Alt-Svc`.

This keeps the costly dependency opt-in at *both* build and run time.

### Key insight: reuse the shared handler + cert resolver

HTTP/3 is a third listener that serves the **same** handler the HTTPS listener
already does (`s.observe(dispatchHTTPS)` → the atomic router), so reloads need no
QUIC restart, and routing/middleware/access-lists are identical across h1/h2/h3.

### Wiring

- A third server: `http3.Server{ Addr: ":443/udp", Handler: <same>, TLSConfig:
  <reuse GetCertificate + GetConfigForClient> }`, with `NextProtos` including
  `"h3"`. Started alongside `httpSrv`/`httpsSrv` in `Server.Start`, shut down on
  ctx cancel.
- **Alt-Svc advertisement:** when h3 is enabled, emit
  `Alt-Svc: h3=":443"; ma=86400` on HTTPS responses (same injection point as
  `hsts`/`robots` in `serveHTTPS`). Clients upgrade themselves; h3 is never forced.
- **TLS:** QUIC is TLS 1.3-only by protocol. Per-host `minTLSVersion: "1.2"` is a
  floor, not a ceiling, so 1.3-over-h3 is fine; document that h3 always implies
  1.3 regardless of the host pin. Per-host mTLS (feature 1) over h3 is an open
  item to verify.

### Deployment (must be documented, easy to miss)

- **Publish UDP/443** in compose (today only TCP/443 is mapped) and open it on the
  firewall/UniFi forward; with `userland-proxy=false` the UDP DNAT must preserve
  the real client IP for access lists (same constraint already handled for TCP).
- quic-go wants a large UDP receive buffer; raise `net.core.rmem_max` /
  `wmem_max` (e.g. 7 MB) or it logs a degraded-performance warning.
- **0-RTT disabled** initially (replay risk) — quic-go's default; keep it.

### Security / open questions

- UDP amplification: QUIC mandates address validation (built into quic-go) — but
  confirm config doesn't disable it.
- Reload semantics for the QUIC listener's own `tls.Config` (cert rotation) —
  verify GetCertificate is consulted live as it is for TLS-over-TCP.
- mTLS-over-h3 timing (see feature 1).

**Effort:** L (plus the build-variant CI/image work).

---

## Suggested sequencing

1. **mTLS** — no dep, reuses the per-SNI hook, closes a parity gap cheaply.
2. **GeoIP** — one isolated pure-Go dep, extends the access-list model.
3. **HTTP/3** — last, behind a build tag + runtime toggle; it is the only one that
   meaningfully grows the dependency surface, and it needs an image/CI variant.
