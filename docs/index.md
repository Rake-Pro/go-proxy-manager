# go-proxy-manager

A standalone ingress and proxy manager for homelabs and small sites. One static
binary terminates TLS for many domains, proxies them to your backends, issues
and renews their certificates, and gates who may reach them.

go-proxy-manager ships as a single static binary and a multi-arch container
image (`ghcr.io/rake-pro/go-proxy-manager`, linux/amd64 + arm64). It stores
everything under one data directory and exposes two HTTP surfaces: the public
data plane and the admin control plane.

## What it does

- **TLS and ACME.** Issues and renews Let's Encrypt certificates over DNS-01 or
  HTTP-01, against any ACME CA including EAB-gated ones.
- **Authentication as typed config.** OIDC, forward-auth, `auth_request`, mTLS
  and basic auth are validated objects, not text snippets.
- **Access control.** Ordered IP/CIDR allow-deny lists, optional GeoIP country
  rules, remote IP feeds, and path-scoped rules.
- **DNS publishing.** Publishes records to Pi-hole and Cloudflare through a
  git-backed ownership ledger that never deletes a record it did not create.
- **Kubernetes and Docker discovery.** An annotated `Ingress` or a labelled
  container becomes a managed proxy host from an operator-authored profile.
- **Git-backed config, REST API and web UI.** Every change is a commit with
  per-object history and revert, over an embedded single-page app.

## Where to start

| You want to | Read |
|---|---|
| Get running in 15 minutes | [Quickstart](getting-started/quickstart.md) |
| Understand the moving parts | [Architecture](concepts/architecture.md), [Terminology](concepts/terminology.md) |
| Pick between two similar knobs | [Which mechanism do I use?](concepts/which-mechanism.md) |
| Do one specific task | [How-to guides](how-to/add-https-host.md) |
| Look up a config key | [Configuration reference](reference/config/README.md) |
| Run it in production | [Operations](operations/backup-and-restore.md) |
| Automate it | [REST API](reference/api.md) |

## Planes and ports

| Plane | Port | Purpose |
|---|---|---|
| Data plane | 80 / 443 | Proxied traffic: SNI TLS, host routing, middleware chain, upstream dial |
| Control plane | 8081 | REST API and web UI, authenticated by OIDC or local bcrypt |

A config change in the store atomically recompiles the data plane's routing
table and certificate set.
