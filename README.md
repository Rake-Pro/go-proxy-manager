# go-proxy-manager

[![CI](https://img.shields.io/github/actions/workflow/status/Rake-Pro/go-proxy-manager/ci.yml?branch=main&label=CI)](https://github.com/Rake-Pro/go-proxy-manager/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/actions/workflow/status/Rake-Pro/go-proxy-manager/release.yml?label=release)](https://github.com/Rake-Pro/go-proxy-manager/actions/workflows/release.yml)
[![GHCR](https://img.shields.io/badge/ghcr.io-go--proxy--manager-blue?logo=docker)](https://github.com/Rake-Pro/go-proxy-manager/pkgs/container/go-proxy-manager)
[![License](https://img.shields.io/github/license/Rake-Pro/go-proxy-manager)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/Rake-Pro/go-proxy-manager)](go.mod)

A standalone edge for homelab and small-site ingress - TLS termination and
ACME, SSO and mTLS, access control, DNS publishing, Kubernetes and Docker
discovery,
git-backed config, a REST API and a web UI - in one static binary.

- A single static binary that terminates TLS for many domains
- Reverse-proxies them to your backends
- Issues and renews Let's Encrypt certificates over DNS-01 or HTTP-01
- Gates access with IP access lists, forward-auth, and OpenID Connect single
  sign-on
- Configuration is declarative, git-backed YAML; there is a REST API and an
  embedded web UI

## Why gpm

- **Git history and revert, per object.** Every config object is a YAML file
  and every change is a commit; revert one object or the whole tree from the
  History view or the API.
- **Typed auth as config, not text snippets.** OIDC, forward-auth,
  auth-request, client-cert and basic auth are validated objects with referential
  integrity - a dangling reference is a load-time error, not a 2 a.m. outage.
- **An in-product certificate authority.** Generate a client CA, issue mTLS
  client certificates as PKCS#12 downloads, and track expiry, with no
  external tooling.
- **DNS that follows the config.** DNS sync publishes records to Pi-hole
  and/or Cloudflare and tracks ownership in a git-backed ledger, so it never
  deletes a record it did not create.
- **Kubernetes Ingress discovery with name-only profiles.** An annotated
  `Ingress` becomes a managed proxy host; the annotation can only name an
  operator-authored profile, never a middleware or upstream, so an untrusted
  manifest can't weaken a chain you configured.
- **One CGO-free binary.** 9 direct dependencies, all pure Go or
  `golang.org/x`; every import is justified against the stdlib first.
- **The one non-core dependency is SQLite.** `modernc.org/sqlite` (plus 7
  indirect modules) backs the local session store and the one-time NPM
  importer, not the proxying path.

## Features

**Proxying**
- **TLS termination.** SNI-based, with exact and wildcard certificate
  selection.
- **HTTP/2 and WebSockets.** Always on, plus an HTTP-to-HTTPS 308 redirect.
- **Locations.** Path-scoped upstream and middleware within one host.
- **Upstream groups.** Failover, weighted round-robin, least-connections, or
  sticky ip-hash, with active health checks and passive failure detection.
- **Redirect, stream and parked hosts.** Raw TCP/UDP forwarding, reserved
  names, and a catch-all for unmatched vhosts.
- **Stream TLS/SNI routing.** Several TCP hosts share one port, routed by
  SNI, passthrough or terminated, plus L4 access lists evaluated before any
  backend dial.
- **PROXY protocol.** Inbound v1/v2 from trusted CIDRs, so the real client IP
  drives every downstream check.
- **Dual-stack.** One listener serves IPv4 and IPv6, each gated and logged as
  its own address.

**Certificates**
- **ACME.** DNS-01 (four named providers plus the generic RFC2136 and
  acme-dns solvers, covering any nameserver) or HTTP-01, against any ACME CA
  including EAB-gated ones like ZeroSSL and Google Public CA.
- **Auto-renewal.** 30 days before expiry, ECDSA P-256.
- **Custom certificates.** Bring your own.
- **mTLS.** Per-host `tls.clientAuth` against a `ClientCA` trust anchor, with
  CRL revocation and identity-passthrough headers.
- **Client CA generation.** A self-signed, issuance-ready CA from the UI, no
  external tooling required.
- **Client-certificate issuance.** Mint certificates from a signing CA,
  downloaded as a password-protected PKCS#12 bundle.
- **Expiry warnings and renewal.** gpm tracks what it issued and warns before
  expiry; renewal needs a manual re-import on the device.

**Access control & auth**
- **One client IP, three trust tiers.** `trustedProxies` (fleet-wide, or
  per-host as a nullable three-state override) is the single setting that
  decides whose `X-Forwarded-For` is believed, and the address it derives is
  what access lists, geo, rate limits, every `allowFrom` exemption, the admin
  login lockout, the access log and the upstream headers all compare - see
  [Client IP and the three trust
  tiers](docs/concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers).
- **IP access lists.** Allow/deny CIDR rules, default-deny, optional GeoIP
  country rules.
- **Path-scoped rules and remote sources.** A rule can be limited to exact
  paths and methods, or draw its CIDRs from a feed gpm re-fetches on a
  schedule.
- **OIDC.** Admin login via authorization code + PKCE, with group-to-role
  mapping.
- **Local admin TOTP.** An optional RFC 6238 second factor on the local admin
  password, configured by secret file and enrolled with `gpm totp-secret`.
- **Read-only viewer role.** The `user` role reads every `GET` and is refused
  every write, so a viewer can be handed a login without handing over control.
- **Forward-auth.** Trusts upstream-asserted identity headers from trusted
  peers.
- **Auth-request.** An `auth_request`-style subrequest to an identity
  outpost.
- **Client-certificate auth.** Requires a verified mTLS certificate, with an
  optional trusted-network exemption - see
  [which IP `allowFrom` compares](docs/concepts/request-pipeline.md#which-ip-allowfrom-compares)
  before relying on it.
- **HTTP basic auth.** An auth middleware in `basic` mode gates a host on local
  username/bcrypt-hash pairs, with the same trusted-network exemption.
- **Auth and rate limit on the host itself.** Attach SSO or a rate limit
  directly on a host or location; middleware objects remain for reuse.
- **Guards.** Deny by path, method, or query, with CIDR exemptions.
- **Rewrite.** Upstream-facing path rewrite with exact, prefix and regex rules,
  never a redirect.
- **Path and Host escape hatches.** Strip a location's prefix, prefix an
  upstream base path, and override the Host header sent upstream.
- **Bouncer hook.** Denies on a CrowdSec LAPI or generic HTTP verdict - a deny
  hook, not a bundled WAF.
- **Scoped security headers.** Per-header scope (`all` / `generated-only` /
  `proxied-only`) so a header safe on gpm's own pages never breaks a proxied
  app.
- **Response-header stripping.** Removes leaked backend headers (`Server`,
  `X-Powered-By`, ...) case-insensitively, including on WebSocket upgrades.
- **Maintenance mode.** Per-host or fleet-wide: answers `503` with
  `Retry-After` and never dials the upstream, with no restart.
- **Composable middleware chain.** Ordered, per host and per location.

**Automation & DNS**
- **Scoped API tokens.** Bearer credentials with per-resource read/write
  scopes, shown once, revocable instantly.
- **DNS sync.** Publishes records to Pi-hole and/or Cloudflare, tracked in a
  git-backed ownership ledger that only ever deletes what it recorded
  creating.
- **Kubernetes Ingress discovery.** An annotated `Ingress` derives a full
  proxy host from an operator-authored template or named profile; the
  annotation can only name a profile, never a middleware or upstream.
- **Docker container discovery.** The same reconciler and the same name-only
  profile contract for containers: `gpm.rake.pro/enabled: "true"` opts a
  container in, with domains, port, scheme and profile set by label. Reads the
  Engine API read-only (a socket proxy is enough) and reconciles on container
  events.

**Operations**
- **REST API and web UI.** A single embedded SPA.
- **Certificate health.** Expiry, issuer and last-renewal status on every
  certificate, a force-renew endpoint, and a fleet-wide `GET /api/health`
  summary covering data-plane listeners, certificate counts and
  upstream-group health.
- **Git-backed config.** Full referential validation, per-commit history, and
  revert - whole-config or scoped to one object.
- **NPM importer.** One-time, best-effort import from an existing NPM/NPMplus
  data directory.
- **Structured logging.** zerolog, with a toggleable access log and
  slow-request warnings.
- **Prometheus metrics.** Opt-in `/metrics`, no `client_golang` dependency,
  host labels bounded by config.
- **Notifications.** ntfy/Discord/generic-webhook alerts on renewal failure,
  cert expiry, upstream health flaps, and a frozen discovery reconciler, with
  per-target event filtering and a test-send endpoint.

See [FEATURES.md](FEATURES.md) for the full roadmap (P0-P3 tiers).

## Architecture

```
                         +---------------------- gpm (one binary) -----------------------+
   Internet -- :443/:80 -|  data plane: SNI TLS -> host routing -> middleware -> upstream |
                         |     (force-SSL, locations, access-lists, guards, auth, headers)|
                         |                                                                |
   Operator -- :8081 ----|  control plane: REST API + web UI  -->  git-backed config store|
                         |  auth: OIDC / local bcrypt          |   (per-object YAML)      |
                         |                                     \/                         |
                         |  ACME manager -- DNS-01/HTTP-01 --> ACME CA   certs on disk    |
                         +----------------------------------------------------------------+
```

| Plane | Port | Purpose |
|---|---|---|
| Data plane | 80 / 443 | Proxied traffic: SNI TLS, host routing, middleware chain, upstream dial |
| Control plane | 8081 | REST API + web UI, authenticated by OIDC or local bcrypt |

A config change in the store atomically recompiles the data plane's routing
table and certificate set. See
[docs/concepts/architecture.md](docs/concepts/architecture.md).

## Quick start

See [docs/getting-started/quickstart.md](docs/getting-started/quickstart.md)
for the full guide, and
[docs/getting-started/install-binary.md](docs/getting-started/install-binary.md)
for bare metal and systemd.

Prerequisites: Docker and Docker Compose.

1. Generate an admin password hash and write it to a file:

   ```
   docker run --rm ghcr.io/rake-pro/go-proxy-manager hashpw 'your-password' > admin_hash
   chmod 644 admin_hash
   ```

   The container runs as a non-root user; without the `chmod`, that user
   cannot read the Docker secret, and the admin panel starts with no
   authentication configured instead of failing loudly.

2. Write a minimal compose file:

   ```yaml
   # compose.yaml
   services:
     gpm:
       image: ghcr.io/rake-pro/go-proxy-manager
       ports: ["80:80", "443:443", "127.0.0.1:8081:8081"]
       environment:
         GPM_LOCAL_ADMIN_USER: admin
         GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE: /run/secrets/admin_hash
       volumes: ["gpm-data:/data"]
       secrets: [admin_hash]
       cap_drop: ["ALL"]
       security_opt: ["no-new-privileges:true"]
   secrets:
     admin_hash:
       file: ./admin_hash
   volumes:
     gpm-data:
   ```

3. Start it:

   ```
   docker compose up -d
   ```

4. Open `http://127.0.0.1:8081/` (tunnel to it for a remote host) and sign
   in. No cookie setting is needed: the session cookie is issued without
   `Secure` over plain HTTP and becomes `Secure` (and `__Host-` named)
   automatically once the panel is served over HTTPS or `externalBaseURL` is an
   `https://` URL. Add a proxy host, point it at a backend, and attach a
   certificate. Configuration is written as YAML under the `/data/config` git
   repo, so you can also manage it as code.

## Configuration

Everything is a typed object stored as `config/<kind>/<name>.yaml` plus a single
`config/settings.yaml`. A minimal proxy host:

```yaml
# config/proxy-hosts/app.yaml
name: app
domains: [app.example.com]
upstream: {scheme: http, host: backend, port: 8080}
tls: {certificateRef: wildcard, forceSSL: true}
```

`tls` is optional - omit it entirely for a first run and the host is reachable
over plain HTTP on `:80` with no certificate configured yet; add `tls` once
you're ready to issue or attach one.

The complete object reference (proxy/redirect/stream/parked hosts, certificates,
DNS providers, identity providers, access lists, middlewares, settings) with
validation rules and examples is in
**[docs/reference/config/](docs/reference/config/README.md)**.

## Documentation

Full documentation lives in [docs/](docs/index.md).

- **[Quickstart](docs/getting-started/quickstart.md).** Container to signed-in
  admin panel to a first HTTPS host, in 15 minutes.
- **[Concepts](docs/concepts/architecture.md).** Architecture, the request
  pipeline, the security model, and a
  [glossary](docs/concepts/terminology.md).
- **[How-to guides](docs/how-to/add-https-host.md).** One page per task: hosts,
  certificates, SSO, mTLS, DNS sync, discovery, migration.
- **[Configuration reference](docs/reference/config/README.md).** One page per
  object kind, plus every
  [environment variable and flag](docs/reference/env-vars-and-flags.md).
- **[Operations](docs/operations/backup-and-restore.md).** Backup, upgrading,
  high availability, hardening, and a consolidated
  [troubleshooting table](docs/operations/troubleshooting.md).
- **[REST API](docs/reference/api.md).** Authentication, token scopes, and the
  OpenAPI specification.

## Building from source

Requires Go 1.26+.

```
go build -o gpm ./cmd/gpm     # build the binary
go test ./...                 # run the test suite (hermetic; needs `git` on PATH)
```

Subcommands: `gpm` (daemon), `gpm hashpw <password>`, `gpm totp-secret`,
`gpm import -npm-data <dir>`.

## Security model

- The admin/control plane is authenticated (local bcrypt and/or OIDC) and not
  meant to be exposed directly to the internet; front it with your own ingress.
- Identity headers are stripped from any peer that is not a configured trusted
  proxy, so a direct client cannot forge an identity to a backend.
- All trust decisions are rooted in the connection peer IP, never a forwarded
  header, unless that peer is explicitly trusted.
- Secrets are referenced, never stored: literal secret values are rejected at
  commit time. API token secrets are never stored at all - only their SHA-256
  digest is committed, and the plaintext is shown exactly once at creation.
- API tokens are scope-limited, not just role-limited: token management, writing
  settings, backup, restore, whole-config revert and the pprof endpoints all
  require the `admin` scope, so a resource-scoped automation credential cannot
  widen itself. The stored digest is never returned by any endpoint, and
  reverting an `APIToken` from history is refused so a rotation always means
  revocation.

For deployment hardening notes see
[docs/operations/hardening.md](docs/operations/hardening.md).

## Project layout

```
cmd/gpm/            entrypoint, subcommands (daemon, hashpw, totp-secret, import)
internal/model/     config object types + validation
internal/store/     git-backed config store
internal/dataplane/ TLS, routing, middleware chain, reverse proxy
internal/acme/      ACME issuance + renewal (DNS-01 solvers, HTTP-01 tokens)
internal/dnssync/   Pi-hole / Cloudflare DNS record reconciler
internal/auth/      sessions, OIDC, forward-auth, role mapping
internal/oidc/      OIDC client (discovery, PKCE, token verification)
internal/api/       REST API
internal/ui/        embedded web UI (go:embed)
internal/importer/  Nginx-Proxy-Manager importer
```

## Migrating from Nginx Proxy Manager

- **One-time importer.** Reads an existing NPM/NPMplus `/data` directory and
  maps proxy/redirect/stream/parked hosts, access lists, and certificates
  into gpm's schema. See
  [docs/how-to/migrate-from-npm.md](docs/how-to/migrate-from-npm.md).
