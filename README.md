# go-proxy-manager

A reverse-proxy manager written in Go: a single static binary that terminates
TLS for many domains, reverse-proxies them to your backends, issues and renews
Let's Encrypt certificates over DNS-01 or HTTP-01, and gates access with IP access lists,
forward-auth, and OpenID Connect single sign-on. Configuration is declarative,
git-backed YAML; there is a REST API and an embedded web UI.

It is a clean-room reimplementation of the ideas behind
[Nginx Proxy Manager](https://github.com/NginxProxyManager/nginx-proxy-manager)
and [NPMplus](https://github.com/ZoeyVid/NPMplus), with first-class SSO and a
small, vetted dependency set as the headline differences.

## Why

Running an Nginx-Proxy-Manager-style edge means inheriting a large Node
dependency surface and configuring authentication through raw nginx snippets that
are easy to get subtly wrong. go-proxy-manager takes the same feature set and
builds it with a narrower, more focused design:

- **One CGO-free binary.** ~7 direct dependencies, all pure-Go or `golang.org/x`.
- **Authentication is first-class config, not text snippets.** OIDC, forward-auth,
  and an nginx-`auth_request`-style outpost mode are typed objects with validation,
  not hand-written directives.
- **GitOps-native.** Every config object is a YAML file; every change is a git
  commit. The whole graph is validated (dangling references are a load-time error,
  not a 2 a.m. outage).
- **Secrets never live in the config.** Values use `${ENV:...}` / `${FILE:...}`
  placeholders; committing a literal secret is refused.

## Features

**Proxying**
- SNI-based TLS termination with exact and wildcard certificate selection
- HTTP/2, WebSocket upgrades, force-SSL (HTTP→HTTPS 308)
- Path-scoped **locations** (per-path upstream and middleware on one host)
- **Upstream groups**: failover, weighted round-robin, least-connections, and
  sticky ip-hash across multiple backends, with active TCP/HTTP health checks,
  passive connect-failure detection, and signed sticky-session cookies with a
  configurable TTL
- Redirect hosts, raw TCP/UDP **stream** forwarding, and parked hosts (reserve a
  name without serving anything; absorb unmatched vhosts)
- **Stream TLS/SNI**: several TCP stream hosts share one port, routed by the SNI
  in the ClientHello, either passed through untouched (never decrypted) or
  terminated at gpm; plus **L4 access lists** (IP + geo) evaluated before any
  backend is dialled
- **Inbound PROXY protocol** (v1 + v2) from trusted load-balancer CIDRs, so the
  real client IP drives access lists, geo, rate limits, `X-Forwarded-For` and the
  access log
- **Dual-stack**: one listener serves IPv4 and IPv6, and a v6 client is gated and
  logged as its own address

**Certificates**
- Let's Encrypt (or any ACME CA) via **DNS-01** (Cloudflare, DigitalOcean,
  Hetzner, deSEC) or **HTTP-01** on the plaintext `:80` listener
- External Account Binding for CAs that require it (ZeroSSL, Google Public CA)
- Automatic renewal (30 days before expiry), ECDSA P-256
- Bring-your-own custom certificates
- **mTLS client certificates** - per-host `tls.clientAuth` against a `ClientCA`
  trust anchor (`require` or `optional`), CRL revocation, and identity
  passthrough headers, all switchable from the host editor
- **One-click client CA** - generate a self-signed, issuance-ready CA from the UI
  (RSA-4096, `pathlen:0`, key stored at `0600` in the cert store). No openssl, no
  files to place by hand; pasting an existing CA stays fully supported
- **Client-certificate issuance** - give a `ClientCA` a signing key and mint
  client certificates from the UI or `POST /api/client-cas/{name}/issue`,
  downloaded as a password-protected PKCS#12 bundle. The private key is never
  stored or logged; it exists only in the download
- **Client-certificate expiry warnings and renewal** - gpm records what each CA
  issued (subject, serial, validity, never key material), warns before a
  certificate expires, and renews one in place with a new key and serial. There is
  no client-side renewal: every device has to import the new `.p12` by hand, and
  the UI says so before you start

**Access control & auth**
- IP access lists (allow/deny CIDR rules, default-deny, HTTP basic-auth)
- **OIDC** admin login (authorization code + PKCE, group→role mapping)
- **Forward-auth** (trust upstream-asserted identity headers from trusted peers)
- **Auth-request** (nginx `auth_request`-style subrequest to an Authentik outpost)
- **Client-certificate auth** (`client-cert` middleware mode: require a verified
  mTLS certificate, optionally mapped to a role, with an `allowFrom` CIDR
  exemption so a trusted network skips the certificate requirement - read
  [the note on which IP that compares](docs/configuration.md#which-ip-allowfrom-actually-compares)
  if gpm is not your internet-facing edge)
- Request **guards** (deny by path/method/query, with CIDR exemptions)
- Exact-match path **rewrite** (upstream-facing, method/body preserved, never a redirect)
- **Bouncer** deny hook (CrowdSec LAPI or any generic HTTP endpoint) — hook-only, no bundled WAF
- **Security response headers on gpm's own responses** — `settings.securityHeaders`
  (plus a per-host override) emitted on every response gpm generates itself
  (auth-gate denials, sign-in redirects, error pages, `400`/`404`/`421`,
  parked/redirect hosts), at the same layer as HSTS so denials get them too;
  applied set-if-absent so a proxied app's own headers are never clobbered.
  Opt-in (empty by default); HSTS stays separate
- Composable, ordered middleware chain per host and per location

**Automation & DNS**
- **Scoped API tokens** — bearer credentials for scripts and CI, minted
  server-side and shown once (only a SHA-256 digest is committed), with
  per-resource `read`/`write` scopes, optional expiry, and instant revocation
- **DNS sync** — publishes CNAMEs for opted-in hosts to a local Pi-hole v6
  resolver and/or a Cloudflare zone; full-state reconcile that only ever deletes
  records it *recorded creating*, in a git-backed ownership ledger. Enabling it on
  a resolver full of hand-written records adopts what matches and touches nothing
  else - and a record it adopted is later *released*, never deleted or retargeted -
  and `GET /api/dns-sync/plan` previews the whole run before you commit to it
- **Kubernetes Ingress discovery** — opt an `Ingress` in with one annotation and
  gpm derives a proxy host from your template, or from one of your **named
  profiles** the manifest picks by name (read-only against the cluster, no
  `client-go`), which then feeds the same DNS sync; the annotation can only carry
  a profile *name*, never a middleware/access-list/upstream, so a cluster
  manifest can never weaken a chain you configured; a derived host is a *full*
  proxy host (TLS/mTLS, chains, websockets, `robotsNoIndex`, `timeouts`, tags),
  so moving a service into discovery does not quietly drop half its settings;
  only gpm's own labelled objects are ever touched, and it freezes rather than
  deleting when the cluster cannot be read

**Operations**
- REST API + embedded single-page web UI
- Git-backed declarative config with full referential validation, per-commit
  history, and revert (whole-config or scoped to a single object)
- One-time importer from an existing Nginx-Proxy-Manager/NPMplus data directory
- Structured logging (zerolog), optional access log, slow-request warnings
- Opt-in Prometheus metrics at `/metrics` on the admin listener (`GPM_METRICS=1`),
  with no `client_golang` dependency and host labels bounded by config, not by
  client-supplied `Host` headers

See [FEATURES.md](FEATURES.md) for the full roadmap (P0–P3 tiers).

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

Two independent listeners: the **data plane** (public, ports 80/443) serves
proxied traffic; the **control plane** (admin, port 8081) serves the API and UI
and is meant to sit behind your own ingress or an SSH tunnel. A config change in
the store atomically recompiles the data plane's routing table and certificate
set. See [docs/architecture.md](docs/architecture.md).

## Quick start

With Docker (see [docs/deployment.md](docs/deployment.md) for the full guide):

```
# 1. Generate an admin password hash
docker run --rm ghcr.io/rake-pro/go-proxy-manager hashpw 'your-password'

# 2. Minimal compose
cat > compose.yaml <<'YAML'
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
    file: ./admin_hash      # the hash from step 1; chmod 644 so the non-root user can read it
volumes:
  gpm-data:
YAML

docker compose up -d
```

Open `http://127.0.0.1:8081/` (tunnel to it for a remote host) and sign in. Add a
proxy host, point it at a backend, and attach a certificate. Configuration is
written as YAML under the `/data/config` git repo, so you can also manage it as
code.

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
validation rules and examples is in **[docs/configuration.md](docs/configuration.md)**.

## Building from source

Requires Go 1.26+.

```
go build -o gpm ./cmd/gpm     # build the binary
go test ./...                 # run the test suite (hermetic; needs `git` on PATH)
```

Subcommands: `gpm` (daemon), `gpm hashpw <password>`, `gpm import -npm-data <dir>`.

## Security model

- The admin/control plane is authenticated (local bcrypt and/or OIDC) and not
  meant to be exposed directly to the internet; front it with your own ingress.
- Identity headers are stripped from any peer that is not a configured trusted
  proxy, so a direct client cannot forge an identity to a backend.
- All trust decisions are rooted in the connection peer IP, never a forwarded
  header, unless that peer is explicitly trusted.
- Secrets are referenced, never stored: literal secret values are rejected at
  commit time. API token secrets are never stored at all — only their SHA-256
  digest is committed, and the plaintext is shown exactly once at creation.
- API tokens are scope-limited, not just role-limited: token management, writing
  settings, backup, restore, whole-config revert and the pprof endpoints all
  require the `admin` scope, so a resource-scoped automation credential cannot
  widen itself. The stored digest is never returned by any endpoint, and
  reverting an `APIToken` from history is refused so a rotation always means
  revocation.

For deployment hardening notes see [docs/deployment.md](docs/deployment.md).

## Project layout

```
cmd/gpm/            entrypoint, subcommands (daemon, hashpw, import)
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

## Acknowledgements & license

Nginx Proxy Manager (MIT) and NPMplus (AGPLv3) are referenced as behavioural
inspiration only; this is a clean-room implementation with no copied code or
configuration. See the repository license for terms.
