# Deployment

go-proxy-manager ships as a single static binary and a multi-arch container image
(`ghcr.io/rake-pro/go-proxy-manager`, linux/amd64 + arm64). It stores everything
under one data directory and exposes two HTTP surfaces: the public data plane and
the admin control plane.

## Listeners & ports

| Port (default) | Surface | Expose to |
|----------------|---------|-----------|
| `:443` | Data plane HTTPS | Internet |
| `:80` | Data plane HTTP (force-SSL redirects, plaintext hosts) | Internet |
| `:8081` | Admin API + web UI | **Not the internet** — your ingress, LAN, or an SSH tunnel |

The admin plane is authenticated, but you should still keep it off the public
internet. Bind it to loopback (`127.0.0.1:8081`) and reach it via a tunnel, or
front it with your own authenticating ingress.

## Data directory

A single volume mounted at `/data`:

```
/data/config       git-backed config repo (see docs/configuration.md)
/data/certs        certificate store (custom certs + ACME-issued artifacts)
/data/session.db   SQLite session store (pure-Go, no CGO)
```

The container runs as a non-root user; make sure the mounted volume is writable by
it (the image's `gpm` user).

## Configuration: flags & environment

Every flag has an environment-variable equivalent (the env var wins for
container deployments).

| Flag | Env | Default | Purpose |
|------|-----|---------|---------|
| `-config-dir` | `GPM_CONFIG_DIR` | `/data/config` | Git-backed config repo |
| `-cert-dir` | `GPM_CERT_DIR` | `/data/certs` | Certificate store |
| `-session-db` | `GPM_SESSION_DB` | `/data/session.db` | Session database |
| `-admin-addr` | `GPM_ADMIN_ADDR` | `:8081` | Admin listen address |
| `-https-addr` | `GPM_HTTPS_ADDR` | `:443` | Data-plane HTTPS |
| `-http-addr` | `GPM_HTTP_ADDR` | `:80` | Data-plane HTTP |
| `-local-admin-user` | `GPM_LOCAL_ADMIN_USER` | (none) | Break-glass admin username |
| `-cookie-secure` | `GPM_COOKIE_SECURE` | `true` | Session-cookie `Secure` flag; set `0` only for local/LAN plain-HTTP admin access. When Secure, the session cookie also gets the `__Host-` prefix |
| (env only) | `GPM_SECRET_FILE_ROOTS` | `/run/secrets` | Allowlisted root dir(s) for `${FILE:...}` secret resolution; OS-path-list separated |
| (env only) | `GPM_SECRET_ENV_PREFIXES` | (none) | Comma-separated allowlist of `${ENV:...}` name prefixes. Unset: any non-reserved var resolves. Set: only names with a listed prefix resolve. `GPM_SSO_SIGNING_KEY` and `GPM_LOCAL_ADMIN_PASSWORD_HASH` are never resolvable regardless |
| (env only) | `GPM_SSO_SIGNING_KEY` | (auto-persisted) | HMAC key signing data-plane per-host OIDC session cookies; auto-generated on first use and saved to `<cert-dir>/sso_signing.key` (0600) so sessions survive restarts; set explicitly to supply your own key or share one across instances |
| `-log-level` | `GPM_LOG_LEVEL` | `info` | trace/debug/info/warn/error |
| `-log-console` | `GPM_LOG_CONSOLE` | `false` | Human-readable console logs |
| `-access-log` | `GPM_ACCESS_LOG` | `false` | Log every data-plane request |
| `-slow-request-ms` | `GPM_SLOW_REQUEST_MS` | `0` (off) | Warn on requests slower than N ms |
| `-debug-headers` | `GPM_DEBUG_HEADERS` | `false` | Add `X-GPM-*` diagnostic response headers (leaks upstream info — keep off in production) |
| `-upstream-response-header-timeout` | `GPM_UPSTREAM_RESPONSE_HEADER_TIMEOUT` | `0` (unbounded) | Cap time-to-first-byte from an upstream |
| `-pprof` | `GPM_PPROF` | `false` | Expose `net/http/pprof` on the admin server at `/debug/pprof/` (admin-role gated) |

**Admin password** is not a flag. Provide a bcrypt hash via, in order of
preference:
- `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE` — path to a file (e.g. a Docker secret), or
- `GPM_LOCAL_ADMIN_PASSWORD_HASH` — the hash inline.

Generate one with `gpm hashpw 'your-password'` (or `docker run --rm <image> hashpw ...`).

> **Docker secret file permissions:** plain `docker compose` mounts file-based
> secrets with the host file's own permissions. The non-root `gpm` user must be
> able to read them, so `chmod 644` the secret files (the `uid`/`gid`/`mode`
> secret options only apply in Swarm).

## Docker Compose

```yaml
services:
  gpm:
    image: ghcr.io/rake-pro/go-proxy-manager
    restart: unless-stopped
    ports:
      - "443:443"
      - "80:80"
      - "127.0.0.1:8081:8081"     # admin: loopback only
    environment:
      GPM_LOCAL_ADMIN_USER: admin
      GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE: /run/secrets/admin_hash
    volumes:
      - gpm-data:/data
    secrets:
      - admin_hash
      - cf_token                  # for ACME (optional)
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]

secrets:
  admin_hash:
    file: ./secrets/admin_hash    # from `gpm hashpw`; chmod 644
  cf_token:
    file: ./secrets/cf_token      # Cloudflare API token; chmod 644

volumes:
  gpm-data:
```

## Automatic certificates (ACME DNS-01)

1. Create a Cloudflare API token scoped to **Zone:DNS:Edit + Zone:Read** on the
   target zone, and mount it as a secret (`./secrets/cf_token`, `chmod 644`).
2. Add a DNS provider and a certificate to the config:

```yaml
# config/dns-providers/cloudflare.yaml
name: cloudflare
provider: cloudflare
config: {apiToken: ${FILE:/run/secrets/cf_token}}
```
```yaml
# config/certificates/wildcard.yaml
name: wildcard
type: acme
domains: ["*.example.com", example.com]
acme: {email: admin@example.com, dnsProvider: cloudflare}
```

3. Reference the certificate from a host (`tls.certificateRef: wildcard`).

The manager issues on first load and renews automatically 30 days before expiry.
DNS-01 needs no inbound port — it works from behind a firewall. **Tip:** validate
against the Let's Encrypt **staging** directory first by setting
`acme.directoryURL: https://acme-staging-v02.api.letsencrypt.org/directory` on a
throwaway hostname; a staging cert is untrusted, so don't point it at a domain
serving real traffic. Switch to production (omit `directoryURL`) once it issues.

## Admin OIDC (single sign-on)

1. Set `externalBaseURL` in `settings.yaml` to the admin panel's public URL; the
   OIDC `redirect_uri` is `<externalBaseURL>/auth/callback`.
2. Create an OIDC application at your IdP with that redirect URI, and an
   `IdentityProvider` object (see [configuration.md](configuration.md)) with a
   `roleMapping.adminGroups`.
3. List the provider under `settings.adminAuth.providers`. Keep
   `localLoginEnabled: true` while you validate; flip `ssoOnly: true` once an SSO
   login succeeds (recovery from SSO-only is by redeploy).

## Upstream-group health (operations)

Hosts backed by an [UpstreamGroup](configuration.md#upstreamgroup-configupstream-groups)
fail over automatically, but during rollouts and node maintenance you can watch
the live state:

```
curl -s -b "<admin session cookie>" https://<admin>/api/upstream-health | jq
# {"edge-nodes":[{"upstream":"http://192.0.2.11:80","healthy":true,"weight":1,"active":3}, ...]}
```

`healthy` flips on the probe/traffic rise-fall thresholds; `active` is the
in-flight request count per upstream. Draining or rebooting a backend node
should show its upstream go unhealthy within roughly one probe interval plus
the fall threshold, with traffic continuing through the remaining upstreams.
Deploy ordering note: roll a new gpm binary **before** adding the first
upstream-group config — an older binary treats a host whose only backend is an
(unknown to it) `upstreamGroupRef` as having no upstream and fails config
validation.

## Migrating from Nginx Proxy Manager

```
gpm import -npm-data /path/to/npm/data -config-dir /data/config -cert-dir /data/certs
```

Best-effort: it maps proxy/redirect/dead/stream hosts, access lists, and
certificates into git-backed config and copies the cert files, printing a count,
the commit hash, and warnings for anything it could not translate (e.g. raw
`advanced_config` nginx snippets — re-express those as typed middlewares, or as
an upstream group for a custom load-balanced `upstream` block). Use
`-dry-run` first to review the mapping without writing anything.

For a full walkthrough — running gpm alongside a live NPM install, validating
parity before any traffic moves, then cutting over — see
[migrating-from-npm.md](migrating-from-npm.md).

## Profiling (pprof)

On-demand CPU/memory/goroutine attribution for the running edge - the tool for
"why is gpm hot / slow" during a throughput investigation, instead of guessing
from `docker stats`. Off by default; it only expands attack surface and adds
overhead while it's on.

**Enable/disable:** set `GPM_PPROF=1` (or `-pprof`) and restart. Leave it OFF in
normal operation: profiles can expose in-memory data (secrets, session
material), and profiling itself costs CPU. Flip on for an investigation,
capture what you need, flip back off.

The endpoints are mounted on the **admin server** at `/debug/pprof/`, behind the
same gate as `/api/` (same-origin guard + admin-role session). There is no
token-in-URL or basic-auth mode - you authenticate with an admin browser
session (`gpm_session` cookie), either on the LAN admin listener (`-admin-addr`,
default `:8081`) or via a proxied admin domain if you've fronted the admin
panel with a host (see "Admin OIDC" above).

| Endpoint | Answers |
|----------|---------|
| `/debug/pprof/profile?seconds=30` | CPU hotspots |
| `/debug/pprof/heap` | Memory growth / allocation sources |
| `/debug/pprof/goroutine?debug=2` | Stalls / deadlocks (full goroutine dump) |
| `/debug/pprof/trace?seconds=5` | Scheduler / syscall timeline (`go tool trace`) |

Capture workflow (browser session cookie required on every request):

```
# 30s CPU profile while reproducing load
curl -b "gpm_session=<session-id>" \
  "https://admin.example.com/debug/pprof/profile?seconds=30" -o cpu.pprof
go tool pprof -http :8080 cpu.pprof

# heap snapshot
curl -b "gpm_session=<session-id>" \
  "https://admin.example.com/debug/pprof/heap" -o heap.pprof

# goroutine dump (stalls/deadlocks)
curl -b "gpm_session=<session-id>" \
  "https://admin.example.com/debug/pprof/goroutine?debug=2" -o goroutines.txt

# execution trace
curl -b "gpm_session=<session-id>" \
  "https://admin.example.com/debug/pprof/trace?seconds=5" -o trace.out
go tool trace trace.out
```

**Known limitation:** `go tool pprof https://admin.example.com/debug/pprof/profile`
(direct remote attach) does not work. `go tool pprof` sends no session cookie,
and its symbolization step issues `POST /debug/pprof/symbol`, which fails
CSRF/auth like any other mutating admin request. Download the profile with the
cookie (as above) and run `go tool pprof` against the local file instead.

## Hardening notes

- Keep `-debug-headers` off in production (it exposes upstream addressing).
- Keep `-pprof` off unless actively profiling; it is admin-role gated but still
  needless attack surface when idle.
- Ensure `GPM_COOKIE_SECURE` is `1` (the default) whenever the admin plane is
  reached over HTTPS. gpm warns at startup when `GPM_COOKIE_SECURE=0` is
  combined with an `https://` `externalBaseURL` — that pairing is only sane for
  a deliberate LAN-only plain-HTTP admin listener running alongside the public
  URL.
- If a data-plane SSO session may have been exposed (device theft, cookie
  leak), `POST /api/sso/revoke` (or the button under Settings) invalidates
  every outstanding SSO session at once; users re-authenticate at the IdP.
- Run with `cap_drop: ALL` and `no-new-privileges` (as above).
- Put the admin plane behind your ingress / a tunnel, not on the public internet.
- Prefer `${FILE:...}` secrets (Docker secrets) over `${ENV:...}` so values don't
  show up in the process environment.
