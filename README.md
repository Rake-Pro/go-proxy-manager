# go-proxy-manager

A standalone ingress and proxy manager for homelabs and small sites: TLS and
ACME, SSO and mTLS, access control, DNS publishing, Kubernetes and Docker
discovery, git-backed config, a REST API and a web UI, in one static binary.

[![CI](https://img.shields.io/github/actions/workflow/status/Rake-Pro/go-proxy-manager/ci.yml?branch=main&label=CI)](https://github.com/Rake-Pro/go-proxy-manager/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/actions/workflow/status/Rake-Pro/go-proxy-manager/release.yml?label=release)](https://github.com/Rake-Pro/go-proxy-manager/actions/workflows/release.yml)
[![GHCR](https://img.shields.io/badge/ghcr.io-go--proxy--manager-blue?logo=docker)](https://github.com/Rake-Pro/go-proxy-manager/pkgs/container/go-proxy-manager)
[![License](https://img.shields.io/github/license/Rake-Pro/go-proxy-manager)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/Rake-Pro/go-proxy-manager)](go.mod)

## What it is

- One CGO-free binary (and a multi-arch container image) terminates TLS for
  many domains and reverse-proxies them to your backends.
- Certificates are issued and renewed automatically from Let's Encrypt or any
  other ACME CA, over HTTP-01 or DNS-01.
- Access is gated by IP access lists, OpenID Connect single sign-on,
  forward-auth, mTLS client certificates and basic auth.
- Configuration is declarative YAML in a git repository gpm manages itself,
  reachable through a REST API and an embedded web UI.

## Highlights

**Proxying**

- **TLS termination and host routing.** SNI-based certificate selection with exact
  and wildcard matching, HTTP/2 and WebSockets, an HTTP-to-HTTPS 308 redirect, and
  path-scoped locations within one host.
- **Upstream groups.** Failover, weighted round-robin, least-connections or
  sticky ip-hash balancing, with active health checks and passive failure detection.
- **Redirect, stream and parked hosts.** Raw TCP/UDP forwarding with SNI routing
  and L4 access lists, reserved names, and a catch-all for unmatched vhosts.

**Certificates**

- **ACME issuance and renewal.** HTTP-01 or DNS-01 against any ACME CA, including
  EAB-gated ones, with four named DNS providers plus the generic RFC2136 and
  acme-dns solvers, and renewal 30 days before expiry.
- **Built-in client CA.** Generate a CA, mint mTLS client certificates as
  password-protected PKCS#12 bundles, and enforce them per host with CRL
  revocation.

**Access and auth**

- **Access control rooted in one client IP.** Access lists are ordered
  allow/deny CIDR rules with default-deny, optional GeoIP country rules, path
  and method scoping and remote CIDR feeds, and they compare the same derived
  client IP as rate limits, `allowFrom` exemptions, logs and upstream headers:
  see [Client IP and the three trust
  tiers](docs/concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers).
- **Authentication as typed config.** OIDC, forward-auth, `auth_request`,
  client-certificate and basic auth are validated objects with referential
  integrity. The admin panel adds a local bcrypt login with optional TOTP,
  group-to-role mapping, a read-only viewer role and scoped API tokens.
- **Composable middleware chain.** Ordered per host and per location: rate
  limits, guards, rewrites, scoped security headers, response-header stripping,
  a CrowdSec or generic HTTP bouncer hook, and maintenance mode.

**Automation**

- **Kubernetes and Docker discovery.** An annotated `Ingress` or a labelled
  container becomes a managed proxy host, and the annotation can only name an
  operator-authored profile, never a middleware or upstream.
- **DNS publishing.** Sync records to a Pi-hole resolver and/or Cloudflare through
  a git-backed ownership ledger that never deletes a record it did not create.

**Operations**

- **Git-backed config with history and revert.** Every object is a YAML file and
  every change a commit, with full referential validation and revert scoped to
  one object or the whole tree.
- **API, UI and observability.** An embedded SPA and an OpenAPI-described REST
  API, structured logging with an optional access log, opt-in Prometheus metrics,
  a fleet health endpoint, and ntfy, Discord or webhook notifications.

## Quick start

Prerequisites: Docker and Docker Compose.

1. Generate an admin password hash and write it to a file:

   ```
   docker run --rm ghcr.io/rake-pro/go-proxy-manager hashpw 'your-password' > admin_hash
   chmod 644 admin_hash
   ```

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

4. Open `http://127.0.0.1:8081/` (tunnel to it for a remote host) and sign in.

5. Add a proxy host, point it at a backend, and attach a certificate. Every
   object is written as YAML under the `/data/config` git repository, so the
   same configuration can also be managed as code.

The [Quickstart](docs/getting-started/quickstart.md) covers the same path in
full (HTTP-01 and DNS-01 issuance, a first HTTPS host), and
[Install the binary](docs/getting-started/install-binary.md) covers bare metal
and systemd.

## Documentation

Published at <https://rake-pro.github.io/go-proxy-manager/>, source in [docs/](docs/index.md).

- **[Quickstart](docs/getting-started/quickstart.md).** Container to signed-in
  admin panel to a first HTTPS host.
- **[Concepts](docs/concepts/architecture.md).** Architecture, configuration
  model, request pipeline, security model, glossary.
- **[How-to guides](docs/how-to/add-https-host.md).** Hosts, certificates, SSO,
  mTLS, DNS sync, discovery, migration.
- **[Reference](docs/reference/config/README.md).** Every config object kind,
  [env vars and flags](docs/reference/env-vars-and-flags.md), the
  [REST API](docs/reference/api.md), [metrics](docs/reference/metrics.md).
- **[Operations](docs/operations/backup-and-restore.md).** Backup, upgrades, HA,
  hardening, certificate health, profiling,
  [troubleshooting](docs/operations/troubleshooting.md).
- **[Roadmap](docs/roadmap.md).** What is planned next, and what is not.

## Architecture

```
                         +---------------------- gpm (one binary) -----------------------+
   Internet -- :443/:80 -|  proxying:   SNI TLS -> host routing -> middleware -> upstream |
                         |     (force-SSL, locations, access-lists, guards, auth, headers)|
                         |                                                                |
   Operator -- :8081 ----|  admin panel:   REST API + web UI  -->  git-backed config store|
                         |  auth: OIDC / local bcrypt          |   (per-object YAML)      |
                         |                                     \/                         |
                         |  ACME manager -- DNS-01/HTTP-01 --> ACME CA   certs on disk    |
                         +----------------------------------------------------------------+
```

A config change in the store atomically recompiles the proxy routing
table and certificate set. See [docs/concepts/architecture.md](docs/concepts/architecture.md).

## Contributing

Build and test prerequisites, project conventions, the docs-in-sync checklist and
the PR process are in [CONTRIBUTING.md](CONTRIBUTING.md). Bugs and feature requests
use the templates under [.github/ISSUE_TEMPLATE/](.github/ISSUE_TEMPLATE/).

## Security

- The admin panel is authenticated but should not be exposed directly to
  the internet; put it behind gpm itself ([how-to](docs/how-to/admin-ui-behind-gpm.md)) or another ingress.
- Trust and role model: [docs/concepts/security-model.md](docs/concepts/security-model.md).
- Deployment hardening: [docs/operations/hardening.md](docs/operations/hardening.md).
- Report a suspected vulnerability privately per [SECURITY.md](SECURITY.md);
  do not open a public issue.

## License

MIT, see [LICENSE](LICENSE).
