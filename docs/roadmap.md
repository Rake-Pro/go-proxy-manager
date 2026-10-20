# Roadmap

What is planned next and what is deliberately not planned. Every planned item
is a [GitHub issue](https://github.com/Rake-Pro/go-proxy-manager/issues); the
table below is the themes, the issues carry the detail and the state.

## Planned

| Theme | Why it matters | Where |
|---|---|---|
| Security hardening follow-ups | Findings of the 2026-10-18 project-wide review, remediation-level | [#49](https://github.com/Rake-Pro/go-proxy-manager/issues/49) (high), [#50](https://github.com/Rake-Pro/go-proxy-manager/issues/50) (medium), [#51](https://github.com/Rake-Pro/go-proxy-manager/issues/51) (low) |
| Admin UI: certificate wizard with CA presets, wizards for the other kinds, Overview panels | Finishes the 2026-10 redesign on the same `wizard()` primitive the proxy host uses | [#54](https://github.com/Rake-Pro/go-proxy-manager/issues/54), [#55](https://github.com/Rake-Pro/go-proxy-manager/issues/55), [#56](https://github.com/Rake-Pro/go-proxy-manager/issues/56) |
| Certificate providers | ZeroSSL EAB provisioning and per-CA how-tos; more DNS-01 solvers (DuckDNS, Porkbun, Linode, Route53, generic webhook); ARI, ACME profiles, lifetime-relative renewal, IP certificates, Origin CA, private CA trust | [#60](https://github.com/Rake-Pro/go-proxy-manager/issues/60), [#61](https://github.com/Rake-Pro/go-proxy-manager/issues/61), [#62](https://github.com/Rake-Pro/go-proxy-manager/issues/62) |
| Discovery template parity | Inline `auth` / `rateLimit`, `trustedProxies` (a three-state field: what "unset in the template" means is the open decision) and hand-added tags surviving a reconcile | [#53](https://github.com/Rake-Pro/go-proxy-manager/issues/53) |
| Maintenance mode | A dedicated metric and a bulk toggle on the hosts list | [#52](https://github.com/Rake-Pro/go-proxy-manager/issues/52) |
| Native tunnel listener (`tsnet`) | No inbound port at all, instead of the relay recipes in [Using gpm behind CGNAT](how-to/tunnels.md); the Tailscale dependency tree is the open decision | [#63](https://github.com/Rake-Pro/go-proxy-manager/issues/63) |
| WebAuthn / passkeys for the local admin login | Extends IdP-less login past TOTP | [#66](https://github.com/Rake-Pro/go-proxy-manager/issues/66) |
| v2 config consolidations | Batches the breaking cleanups (single backend slot, `ParkedHost` into `ProxyHost`, `certificateRef` authoritative-or-removed, deprecated fields removed) into one major-version migration | [#64](https://github.com/Rake-Pro/go-proxy-manager/issues/64) |
| HTTP/3 (QUIC) | Faster, more resilient connections for clients that support it | Held until the Go standard library ships an HTTP/3 server (golang/go#58547); no third-party QUIC stack is light enough for the dependency budget. Design: [design/http3-geoip-mtls.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/design/http3-geoip-mtls.md) |
| High availability, phase 2 | Lease-file election, shared bare repo, active/active; phase 1 (static leader, read-only follower, keepalived VIP) ships | Design sketch in [design/ha.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/design/ha.md) |

## Not planned

| Feature | Why not |
|---|---|
| MFA delegation (trust IdP `acr`/`amr`) | There is no redundant prompt to skip: TOTP is demanded only on the local admin account, which no identity provider authenticates. The config field is deprecated and gone from the UI |
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
| Scheduled maintenance windows | A window opens and closes when an operator flips the switch; a deploy pipeline does the rest with two API calls. A scheduler is a whole subsystem (timezones, overlap, hand-flips inside a window) |
| `healthCheck.path` prefixed with a member's `upstream.path` | The probe path is configured in full on purpose |
| Per-host ACME for discovered names outside the wildcard | Rate-limit budget on an externally driven object set, and permanent CT disclosure of internal service names |
| Watch-based Ingress discovery | A full-state poll self-heals from a missed event; a watch means resourceVersion tracking and reconnect loops for convergence that is not latency-critical |
| Gateway API as a discovery source | Until a need appears |
| Per-Ingress `locations` | Reading paths, upstreams and chains out of an untrusted cluster manifest is the self-service grant the annotation model forbids; profiles carry locations operator-side instead (see `design/ingress-discovery.md`) |
| Adopting a pre-ledger Cloudflare record without the `managed-by:gpm` comment | Deletion needs both marks; add the comment by hand to let gpm own the record |
| A DNS ledger-repair endpoint | Hand-edit `config/dns-ledger.yaml` and reload; an API route onto the ledger is an "authorise a DNS deletion" primitive |

Released changes are in
[CHANGELOG.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/CHANGELOG.md);
anything requiring an operator action on upgrade is repeated in
[Upgrading and rolling back](operations/upgrading.md).
