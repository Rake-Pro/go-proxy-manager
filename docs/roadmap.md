# Roadmap

What is planned next and what is deliberately not planned. For full detail and
history see
[FEATURES.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/FEATURES.md)
(the P0-P3 tier roadmap) and
[BACKLOG.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/BACKLOG.md)
(concrete follow-ups on shipped features) on GitHub.

## Planned

| Feature | Why it matters | Status |
|---|---|---|
| HTTP/3 (QUIC) | Faster, more resilient connections for clients that support it | Held until the Go standard library ships an HTTP/3 server (golang/go#58547); no third-party QUIC stack is light enough for the dependency budget |
| Native tunnel listener (`tsnet`) | A second bind alongside the normal listeners would serve the no-port-forward homelab cohort with no inbound port at all. The `cloudflared`, Tailscale and WireGuard/VPS recipes are already documented in [Using gpm behind CGNAT](how-to/tunnels.md); this is the integration those recipes work around | Not started |
| WebAuthn/passkeys for local admin login | Extends local (IdP-less) login past TOTP for deployments with no OIDC provider | Not started |
| `trustedProxies` on discovery templates | A per-host client-IP override on a discovery-managed host is rebuilt away on the next reconcile. `securityHeaders` and `stripResponseHeaders` already ship; `trustedProxies` is a three-state nullable field, so templating it needs a decision on what "unset in the template" means | Not started |
| Inline `auth` / `rateLimit` on discovery templates | A discovery-managed host can only share a gate through `middlewares` / `accessLists` today; the template mirrors proxy-host fields one by one, so this is not a model-only change | Deferred until the template editor is worked on |
| High availability, phase 2 (lease-file election, shared bare repo, active/active) | Phase 1 (static leader, read-only follower, keepalived VIP) ships; phase 2 removes the manual role assignment | Design sketch only |
| v2 config consolidations (single backend "slot", fold `ParkedHost` into `ProxyHost`, `certificateRef` authoritative-or-removed, drop deprecated fields) | Batches several breaking cleanups into one major-version migration instead of many small breaks | Not started |

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

Released changes are in
[CHANGELOG.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/CHANGELOG.md);
anything requiring an operator action on upgrade is repeated in
[Upgrading and rolling back](operations/upgrading.md).
