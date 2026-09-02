# Roadmap

What is planned next, in progress, and deliberately not planned. For full
detail and history see
[FEATURES.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/FEATURES.md)
(the P0-P3 tier roadmap) and
[BACKLOG.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/BACKLOG.md)
(concrete follow-ups on shipped features) on GitHub.

## Planned

| Feature | Why it matters | Status |
|---|---|---|
| HTTP/3 (QUIC) | Faster, more resilient connections for clients that support it | Not started |
| Hardened TLS as a fleet-wide switch (TLS 1.3 floor, optional TLS 1.2 off) | A per-host `tls.minTLSVersion` override already ships; a fleet default simplifies hardening every host at once | Not started |
| Tunnel integration (CGNAT bypass) | A documented `cloudflared` sidecar recipe and/or a `tsnet` listener would serve the growing no-port-forward homelab cohort with no required new dependency | Not started |
| WebAuthn/passkeys for local admin login | Extends local (IdP-less) login past TOTP for deployments with no OIDC provider | Not started |
| MFA delegation (trust IdP `acr`/`amr`) | Skips a redundant local prompt when the IdP already enforced MFA; the config field exists but is unread and deprecated | Not started |
| v2 config consolidations (single backend "slot", fold `ParkedHost` into `ProxyHost`, `certificateRef` authoritative-or-removed, drop deprecated fields) | Batches several breaking cleanups into one major-version migration instead of many small breaks | Not started |
| Help-hint registry coverage backfill | A handful of icon-only controls and fields still have no inline help popover | Spec not yet written |

## In progress

| Feature | Why it matters | Status |
|---|---|---|
| Inline host/location `auth` and `rateLimit`: UI editors | The model and API already carry inline blocks; the host and location row editors still need the fold UI | Spec written, UI work pending |
| Path/Host escape hatch UI editors (upstream base path, host header, strip prefix, rewrite rule groups) | API/YAML support ships today, but the current UI editor wipes these fields on save until it lands | Spec written (`ui-specs/escape-hatch.md`), UI work pending |

## Not planned

| Feature | Why not |
|---|---|
| Brotli/zstd compression | Gzip already covers response compression; extra codecs add dependency surface for marginal gain |
| OCSP stapling | CRL revocation already covers mTLS client-cert revocation; stapling for gpm's own server certificate is low value for a homelab deployment |
| Email notifications | ntfy/Discord/generic-webhook targets already cover alerting without an SMTP dependency |
| SAML/LDAP login | OIDC covers modern IdP integration; SAML/LDAP would add complexity the minimal-dependency goal doesn't justify |
| PHP / file server | Out of scope: gpm is a proxy/ingress manager, not an application or file server |
| FancyIndex | Out of scope, same reason: no built-in directory-listing feature |
| ECH (Encrypted Client Hello) | Out of scope for now |
| ML-KEM | Out of scope for now |
| MPTCP | Out of scope for now |
| Anubis | Out of scope for now |

Released changes are in
[CHANGELOG.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/CHANGELOG.md);
anything requiring an operator action on upgrade is repeated in
[Upgrading and rolling back](operations/upgrading.md).
