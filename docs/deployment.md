# Deployment

go-proxy-manager ships as a single static binary and a multi-arch container image
(`ghcr.io/rake-pro/go-proxy-manager`, linux/amd64 + arm64). It stores everything
under one data directory and exposes two HTTP surfaces: the public data plane and
the admin control plane.

## Verifying the image

Every image pushed by the release workflow (including the `latest` tag, which
shares the same manifest digest as the version tags built alongside it) is
signed keylessly with [cosign](https://docs.sigstore.dev/cosign/) via GitHub
Actions OIDC - no key material to manage or leak. Verify before you deploy:

```
cosign verify \
  --certificate-identity-regexp '^https://github.com/Rake-Pro/go-proxy-manager/\.github/workflows/release\.yml@refs/(heads/prod|tags/v.*)$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/rake-pro/go-proxy-manager:latest
```

A successful verification prints the signature payload and confirms the image
was built by the `release.yml` workflow in this repository from a `v*` tag -
not from a fork, a different workflow, or a hand-pushed image.

## Listeners & ports

| Port (default) | Surface | Expose to |
|----------------|---------|-----------|
| `:443` | Data plane HTTPS | Internet |
| `:80` | Data plane HTTP (force-SSL redirects, plaintext hosts, ACME HTTP-01 challenges) | Internet |
| `:8081` | Admin API + web UI | **Not the internet** — your ingress, LAN, or an SSH tunnel |

The admin plane is authenticated, but you should still keep it off the public
internet. Bind it to loopback (`127.0.0.1:8081`) and reach it via a tunnel, or
front it with your own authenticating ingress.

Stream hosts open one additional listener per `listenPort` (see
[StreamHost](configuration.md#streamhost-configstream-hosts)); publish those
ports explicitly.

Both data-plane listeners bind a bare `:port` by default (`GPM_HTTP_ADDR=:80`,
`GPM_HTTPS_ADDR=:443`), which in Go binds the **IPv6 wildcard with v4-mapped
addresses enabled** — one socket serving both families. See
[IPv6](#ipv6) below.

A Prometheus exposition is available at `/metrics` on the **admin** listener,
off by default (`-metrics` / `GPM_METRICS=1`) and gated like the rest of the
admin plane - see [Metrics (Prometheus)](#metrics-prometheus) below. Unauthenticated,
the admin server exposes only `/healthz` and `/version`.

### IPv6

gpm listens dual-stack out of the box: a bare `:80` / `:443` bind accepts IPv4 and
IPv6 on the same socket, the router and every middleware are address-family
agnostic, and the client IP an access list, geo rule, rate limit or
`X-Forwarded-For` sees is whichever address the client actually used — a v6 client
appears as its v6 address, not a v4 stand-in. There is no IPv6 toggle to set.

Pinning a bind to one family is still possible and is a deliberate choice:
`GPM_HTTPS_ADDR=0.0.0.0:443` is v4-only, `GPM_HTTPS_ADDR=[::]:443` is v6 (with
v4-mapped addresses, so still both on Linux unless the host has
`net.ipv6.bindv6only=1`).

What usually blocks inbound IPv6 is Docker, not gpm. Standard Docker behaviour:

- **The daemon needs IPv6 enabled.** Set `"ip6tables": true` and (for a
  user-defined bridge) `"experimental": true` where your Docker version still
  requires it in `/etc/docker/daemon.json`, then restart the daemon. Without
  `ip6tables` the daemon does not program v6 NAT/forwarding rules and published
  ports are not reachable over v6.
- **The network needs `enable_ipv6`.** A compose network must declare it (and, on
  older Compose file versions, a v6 subnet):

  ```yaml
  networks:
    edge:
      enable_ipv6: true
      ipam:
        config:
          - subnet: fd00:dead:beef::/64   # ULA is fine; a routed /64 is better
  ```

- **The container needs a real IPv6 address**, not just a v4 port mapping. A
  published port on a v4-only network is unreachable over v6 no matter what gpm
  binds.
- **`userland-proxy` changes what the app sees.** With the default
  `"userland-proxy": true` the daemon's `docker-proxy` relays the connection, so
  the container sees the *proxy's* address as the peer — the real client IP is
  lost for both families. Setting `"userland-proxy": false` in
  `/etc/docker/daemon.json` keeps the connection on the kernel path (iptables /
  ip6tables DNAT) so the original client address arrives intact.
- **Host networking is the simplest alternative.** `network_mode: host` gives the
  container the host's addresses directly: no NAT, no userland proxy, real client
  IPs in both families, at the cost of losing port isolation (gpm then binds the
  host's `:80`/`:443` and every stream port directly).

If the edge in front of gpm is an L4 load balancer rather than the client, the
client IP problem is not a Docker one — turn on
[`settings.proxyProtocol`](configuration.md#proxyprotocolsettings-settingsproxyprotocol)
so gpm reads the real client address (of either family) out of the PROXY header.

## Data directory

A single volume mounted at `/data`:

```
/data/config       git-backed config repo (see docs/configuration.md)
/data/certs        certificate store (custom certs + ACME-issued artifacts,
                   client-CA CRLs, client-CA signing keys under client-cas/,
                   and client-certificate issuance records under client-certs/)
/data/session.db   SQLite session store (pure-Go, no CGO)
```

The container runs as a non-root user; make sure the mounted volume is writable by
it (the image's `gpm` user).

`client-cas/<name>.key` is where a **generated** ClientCA's signing key lands
(`POST /api/client-cas/{name}/generate`, or "Generate new CA" in the UI): gpm
writes it at `0600` itself, so nothing has to be provisioned externally to get a
working mTLS setup. A key you place here yourself (`caKeyFile` pointing anywhere
under the cert store, for a bring-your-own CA) is the same thing by hand - give it
`0600` and owner `gpm`. The alternative is `caKeyPEM` with a
`${FILE:/run/secrets/...}` placeholder, which keeps the key in the secret mount
instead.

Either way it is a **CA private key**: back it up with the rest of `/data/certs`,
and remember it is *not* in the git config repo, so a config-only backup does not
carry it - restoring config alone gives you a ClientCA object pointing at a key
that is not there. Deleting a ClientCA does **not** delete its key file (see
[configuration.md](configuration.md#clientca-configclient-cas)), so removing a CA
for good is a config delete plus an `rm` here. CA generation, certificate issuance
and renewal are all `POST`s, so an HA **follower** refuses them with `503` like
every other write - do them on the leader.

`client-certs/<ca>.json` holds the issuance records that drive the expiry warning
and the renew action. They are runtime state, not config, so a config-only backup
does not carry them - back them up with the rest of `/data/certs`, and share the
cert dir between HA peers (which the [HA recipe](ha.md) already calls for) if you
want the follower to show the same list. Losing them loses only gpm's *memory* of
what was issued: the certificates themselves keep working, and the CA keeps
verifying them.

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
| `-access-log` | `GPM_ACCESS_LOG` | `false` | Log every data-plane request. Startup default only: capture can be flipped live from the Access Logs page or `PUT /api/logs` (admin scope); the runtime toggle is never persisted, so a restart reverts to this flag |
| `-slow-request-ms` | `GPM_SLOW_REQUEST_MS` | `0` (off) | Warn on requests slower than N ms |
| `-debug-headers` | `GPM_DEBUG_HEADERS` | `false` | Add `X-GPM-*` diagnostic response headers (leaks upstream info — keep off in production) |
| `-upstream-response-header-timeout` | `GPM_UPSTREAM_RESPONSE_HEADER_TIMEOUT` | `0` (unbounded) | Cap time-to-first-byte from an upstream |
| `-ha-role` | `GPM_HA_ROLE` | `leader` | HA role: `leader` runs ACME + Ingress discovery and accepts config writes; `follower` disables both and serves the admin API read-only (`503` on writes). See [ha.md](ha.md) |
| `-ha-poll-interval` | `GPM_HA_POLL_INTERVAL` | `20s` | How often a follower runs `git pull --ff-only` on the config repo and reloads when HEAD moved |
| `-pprof` | `GPM_PPROF` | `false` | Expose `net/http/pprof` on the admin server at `/debug/pprof/` (admin role **and** `admin` scope gated) |
| `-metrics` | `GPM_METRICS` | `false` | Expose a Prometheus text exposition on the admin server at `/metrics` (admin role **and** `metrics:read` scope gated) |
| `-geoip-db` | `GPM_GEOIP_DB` | (none) | Path to an operator-supplied GeoLite2/GeoIP2 `.mmdb` file for AccessList geo rules; unset disables geo rules, no database is bundled |

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

## Bare metal / systemd

No container runtime required - `gpm` is a single static, CGO-free binary.

**Build:**

```
git clone https://github.com/Rake-Pro/go-proxy-manager
cd go-proxy-manager
make build              # -> bin/gpm, with VERSION/COMMIT/DATE stamped via ldflags
```

(`go build -trimpath -o /usr/local/bin/gpm ./cmd/gpm` works too, but skips the
version stamping `make build` does - fine for a quick local test, not for
something you'll run `gpm version` against later.) `git` must be on `PATH` at
**runtime**, not just to build: the config store shells out to it for every
commit, the same as the container image installs it explicitly.

**User, data directory, and secrets:**

```
sudo useradd --system --no-create-home --shell /usr/sbin/nologin gpm
sudo install -d -o gpm -g gpm -m 0750 /var/lib/gpm /var/lib/gpm/config /var/lib/gpm/certs
sudo install -d -o root -g gpm -m 0750 /etc/gpm

sudo install -m 0755 bin/gpm /usr/local/bin/gpm

# Password hash: create the file 0600 FIRST, the same reasoning as the
# Ingress-discovery token below - a plain redirect creates it world-readable
# for the window between creation and chmod.
sudo install -m 0600 -o gpm -g gpm /dev/null /etc/gpm/admin_hash
/usr/local/bin/gpm hashpw 'your-password' | sudo tee /etc/gpm/admin_hash >/dev/null
```

**Environment file** (`/etc/gpm/gpm.env`, `0640 root:gpm` so only the service
can read it):

```
GPM_CONFIG_DIR=/var/lib/gpm/config
GPM_CERT_DIR=/var/lib/gpm/certs
GPM_SESSION_DB=/var/lib/gpm/session.db
GPM_LOCAL_ADMIN_USER=admin
GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE=/etc/gpm/admin_hash
GPM_LOG_LEVEL=info
```

Add any other flags from the [table above](#configuration-flags--environment)
as `GPM_*` lines here - there is no separate bare-metal flag surface.

**Unit file** (`/etc/systemd/system/gpm.service`):

```
[Unit]
Description=go-proxy-manager
Documentation=https://github.com/Rake-Pro/go-proxy-manager
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=gpm
Group=gpm
EnvironmentFile=/etc/gpm/gpm.env
ExecStart=/usr/local/bin/gpm
Restart=on-failure
RestartSec=2s

# Bind :80/:443 as the non-root gpm user, with no setuid binary and no root
# process - the systemd equivalent of the container image's non-root USER.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true

# Filesystem hardening. A container gets most of this from its own root
# filesystem being throwaway; state it explicitly here instead.
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/gpm
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```
sudo systemctl daemon-reload
sudo systemctl enable --now gpm
sudo systemctl status gpm
curl -fsS http://127.0.0.1:8081/healthz && echo   # -> ok
journalctl -u gpm -f                              # structured JSON logs; add GPM_LOG_CONSOLE=1 for human-readable
```

## Automatic certificates (ACME)

Pick the challenge that fits the deployment:

- **HTTP-01** (default when no DNS provider is referenced): no credentials at
  all, but `:80` must be reachable from the internet and the name must already
  resolve here. Single names only.
- **DNS-01**: works from behind a firewall with no inbound port, and is the only
  way to get a wildcard. Needs a DNSProvider credential
  (`cloudflare`, `digitalocean`, `hetzner`, or `desec` - see
  [configuration.md](configuration.md#dnsprovider-configdns-providers) for the
  `config` keys each one takes).

```yaml
# config/certificates/app.yaml - HTTP-01, no provider needed
name: app
type: acme
domains: [app.example.com]
acme: {email: admin@example.com, challenge: http-01}
```

The HTTP-01 token is served by the data plane's own `:80` listener ahead of host
routing and the force-SSL redirect, so no host or exception has to be configured
for `/.well-known/acme-challenge/`. Anything in front of gpm (a router port
forward, a cloud LB) must pass port 80 through unmodified. A CA that requires
External Account Binding (ZeroSSL, Google Public CA) takes `acme.eab.kid` +
`acme.eab.hmacKey` alongside its `directoryURL`.

For DNS-01:

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

## API tokens (automation)

Scripts and CI authenticate with a scoped bearer token instead of an admin
session. Mint one from the **API Tokens** page, or over the API:

```
# Create. The plaintext secret is in the response ONCE and is never retrievable.
curl -s -X PUT https://<admin>/api/api-tokens/ci-deploy \
  -b "<admin session cookie>" -H "X-CSRF-Token: <token from /api/me>" \
  -H 'Content-Type: application/json' \
  -d '{"scopes":["proxy-hosts:write","certificates:read"]}' | jq -r .token

# Use it. No CSRF header is needed - a bearer token carries no ambient authority.
curl -s -H 'Authorization: Bearer gpm_...' https://<admin>/api/proxy-hosts | jq

# Rotate (old secret dies immediately) / revoke.
curl -s -X PUT    'https://<admin>/api/api-tokens/ci-deploy?rotate=1' ...
curl -s -X DELETE  https://<admin>/api/api-tokens/ci-deploy ...
```

Verification: a token restricted to `proxy-hosts:read` must get `403` on
`GET /api/settings` and on any `PUT`, and `401` once deleted or expired.
`GET /api/api-tokens` shows each token's scopes, expiry and in-memory `lastUsed`
(which resets on restart by design — see
[configuration.md](configuration.md#apitoken-configapi-tokens)).

No new flags or environment variables: tokens are ordinary config objects under
`config/api-tokens/`, so they are versioned and reviewable like everything else -
but deliberately **not revertible**: restoring an older token file would restore
an older digest and revive a rotated secret, so both the scoped and the
whole-config revert leave `api-tokens` alone. Deploy ordering note: an older binary does not know the
`APIToken` kind, so roll the new binary **before** creating the first token —
rolling back afterwards leaves `config/api-tokens/*.yaml` on disk, which an older
loader ignores (the directory is not in its kind map) but which will reappear the
moment the newer binary runs again.

## DNS sync (Pi-hole + Cloudflare)

Configure the backends once under **Settings -> DNS sync** (see
[configuration.md](configuration.md#dnssyncsettings-settingsdnssync)), then opt
individual hosts in with `dns.lanDirect` / `dns.publicCname`.

Prerequisites:

- **Pi-hole v6** with an application password, reachable from the gpm container.
  The API session must be allowed to write configuration — a `403` means the
  session is read-only or the instance lacks `webserver.api.app_sudo`, and is
  surfaced verbatim in the sync status.
- **Cloudflare**: an existing `dns-providers` entry whose `config.apiToken` has
  `Zone:DNS:Edit` on the target zone. The same token the ACME solver uses is fine.

**Preview before you enable.** `GET /api/dns-sync/plan` (the **Preview changes**
button next to *Reconcile now* in the settings UI) reads both backends and the
ownership ledger and reports exactly what a reconcile would create, adopt,
retarget and delete — writing nothing. Do this first on any resolver that already
holds hand-written records:

```
curl -s https://<admin>/api/dns-sync/plan -H 'Authorization: Bearer gpm_...' | jq
# {"generatedAt":"...","pihole":{"enabled":true,"ok":true,"create":["app.example.com"],
#   "adopt":["www.example.com"],"retarget":[],"delete":[],"skip":[],"untouched":19}, ...}
```

`"delete": []` is the line to check. It is empty on a first enable by
construction: gpm deletes only records recorded in `config/dns-ledger.yaml`, which
is empty until it has created something, so the first run can only create and
adopt. `untouched` should equal the number of records you wrote by hand.

```
# Trigger a reconcile and read the result.
curl -s -X POST https://<admin>/api/dns-sync/reconcile -H 'Authorization: Bearer gpm_...' | jq
curl -s          https://<admin>/api/dns-sync/status    -H 'Authorization: Bearer gpm_...' | jq
# {"lastRun":"...","pihole":{"enabled":true,"ok":true,"desired":12,"managed":12,
#   "created":0,"adopted":0,"retargeted":0,"deleted":0,"skipped":0,"untouched":19}, ...}
```

Reconciles also fire automatically after any proxy-host write, settings change,
restore or whole-config revert. The manual endpoint does **not** queue: while a
reconcile is in flight it answers `409 Conflict`, so a retry loop cannot stack
requests behind a slow backend (`/plan` answers `409` in the same situation).

Verification after enabling: on the first run `created + adopted` should equal the
number of opted-in domains, `deleted` should be `0`, and `untouched` should
account for everything you maintain by hand. On the second run everything but
`desired`, `managed` and `untouched` should be `0`. `skipped` is a real finding —
it means a name a host asks for is already held by a record gpm does not own, and
gpm has left both alone; resolve it by hand.

Each reconcile that changes what gpm owns writes one commit to the config repo
(`DNS sync ledger: update`, authored `dns-sync`); a steady-state run commits
nothing. Changing `apexTarget` no longer orphans anything: records gpm created and
nobody has touched since are **retargeted** on the next reconcile. Records that
predate the ledger, or that somebody has re-pointed, are left alone and reported
as `skipped` / disowned — clean those up by hand.

Two things to expect in the logs. A record gpm **adopted** (rather than created)
is *released* when the config stops asking for it - and equally when `apexTarget`
moves, since a retarget is a delete plus a create: a warn line, the ledger entry
dropped, and the record left in the resolver exactly as you wrote it, for you to
re-point or remove by hand - gpm destroys only what it created. And every
deletion is logged at warn with the
`ledgerRev` that authorised it, because a whole-tree revert restores ownership
claims along with everything else; after reverting a config that ever contained
DNS-synced hosts, run `/api/dns-sync/plan` before letting a reconcile proceed
(see the revert caveat in [configuration.md](configuration.md#dnssyncsettings-settingsdnssync)).

Scopes: `dns-sync:read` for status and plan, `dns-sync:write` for reconcile.

## Kubernetes Ingress discovery

Turns annotated cluster `Ingress` objects into managed proxy hosts, which then
feed the DNS sync above. Configure it under **Settings -> Kubernetes Ingress
discovery** (full field reference in
[configuration.md](configuration.md#ingressdiscoverysettings-settingsingressdiscovery),
rationale in [design/ingress-discovery.md](design/ingress-discovery.md)).

**gpm reads the cluster; it never writes to it.** Apply the shipped RBAC, which
grants `list` on `ingresses` and nothing else — the reconciler works from a full
list on a poll interval, so it never reads an object by name and never opens a
watch:

```
kubectl apply -f deploy/k8s-ingress-discovery-rbac.yaml
```

**Tighten it when you scope to one namespace.** If you set
`settings.ingressDiscovery.namespace`, gpm lists a single namespace, so replace
the shipped `ClusterRole`/`ClusterRoleBinding` with a `Role`/`RoleBinding` in
that namespace — same single `list` verb, without cluster-wide read on every
`Ingress` in the cluster.

Then extract the credential for the (normal) off-cluster deployment, where gpm
runs on the edge host and there is no kubelet to project a token into it. Create
each file `0400` **first**: a plain redirect creates it `0644`, leaving a window
in which any local user can read the bearer token.

```
install -m 0400 /dev/null /run/secrets/gpm-k8s-token
install -m 0400 /dev/null /run/secrets/gpm-k8s-ca.crt
kubectl -n gpm-discovery get secret gpm-ingress-discovery-token \
  -o jsonpath='{.data.token}' | base64 -d > /run/secrets/gpm-k8s-token
kubectl -n gpm-discovery get secret gpm-ingress-discovery-token \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > /run/secrets/gpm-k8s-ca.crt
```

The `ca.crt` jsonpath escapes the dot **inside the key** only. `{.data\.ca\.crt}`
escapes the separator as well, matches nothing, and silently writes an empty CA
file — which surfaces later as `caFile ... contains no usable PEM certificate`.

Mount both into the container and point `tokenFile` / `caFile` at them. gpm
re-reads the token from disk every 5 minutes (and immediately after a `401`), so
replacing the file rotates the credential with no restart. If you instead run gpm
*as a pod in the cluster*, leave `apiURL`, `tokenFile` and `caFile` empty: the
projected ServiceAccount values are used automatically.

Then annotate the Ingresses you want published — and only those:

```yaml
metadata:
  annotations:
    gpm.rake.pro/managed: "true"
    gpm.rake.pro/profile: "sso-internal" # optional, names a settings profile
    gpm.rake.pro/lan-direct: "true"      # optional, overrides the profile's defaultDNS
    gpm.rake.pro/public-cname: "false"   # optional
```

`gpm.rake.pro/profile` names one of `settings.ingressDiscovery.profiles`; omit it
and the default `template` applies. Naming a profile that is not defined
**skips** the Ingress (visible in the status below) rather than falling back to
the default, so a typo shows up instead of silently changing a host's middleware
or access-list chain. The annotation can only carry a *name* — there is
deliberately no annotation that names a middleware, access list, certificate or
upstream, because an Ingress author is untrusted. See
[configuration.md](configuration.md#discovery-profiles).

**Before you move an existing host into discovery, diff it against the template.**
The template and every profile carry the same fields a hand-written proxy host
does — including `robotsNoIndex`, `timeouts` and `tags` — but they are *template*
fields, so anything the chosen profile does not set is not set on the derived
host either. Check the host you are retiring for `robotsNoIndex`, a `timeouts`
override and `tags`, and put them on the profile you are cutting over to.
`locations` is the one thing that has no template equivalent by design: if the
host you are moving has them, leave it hand-written (discovery never touches an
unlabelled host) or publish the paths as their own annotated `Ingress`. After the
cutover, `GET /api/ingress-discovery/status` names the profile each host
resolved to; the derived object itself is in git, so `git show` on the reconcile
commit is the authoritative diff.

```
# Reconcile on demand and read the result.
curl -s -X POST https://<admin>/api/ingress-discovery/reconcile -H 'Authorization: Bearer gpm_...' | jq
curl -s          https://<admin>/api/ingress-discovery/status    -H 'Authorization: Bearer gpm_...' | jq
# {"enabled":true,"lastRun":"...","lastSuccess":"...","discovered":7,"managed":7,
#  "created":0,"updated":0,"deleted":0,"skipped":0,"hosts":[...]}
```

**Deploy ordering.** The template's `certificateRef` must name a `Certificate`
that already exists, and any `middlewares`/`accessLists` must exist too —
otherwise the first reconcile's batch fails referential integrity and writes
nothing (reported in `status.error`). Create those objects, *then* enable
discovery.

**Verification after enabling.** On the first successful run `created` equals the
number of annotated Ingresses whose hosts pass validation; on the second run
everything should read `unchanged` and no commit should appear in
`GET /api/history`. Anything in `hosts[]` with `action: "skipped"` carries a
`reason` — the usual ones are a hostname outside `allowedDomainSuffixes`, a name
already taken by a proxy host you wrote by hand, and a **domain** already served
by a host discovery does not own.

**Ownership covers the domain, not just the name.** A derived host is skipped
whenever any of its domains is already claimed by a host discovery does not own —
including a *disabled* one — so an annotated Ingress cannot take over the
hostname of your SSO or dashboard host by deriving a name that happens to sort
after it. Two annotated Ingresses claiming the same hostname are resolved the
same way: the first by derived name wins, the second is skipped with a reason.
The rule is also enforced one layer down — the config validator rejects any two
*enabled* hosts claiming the same domain, whatever wrote them.

**Freeze on failure is expected behaviour, not an outage.** If the API server is
unreachable, returns an error, returns something that is not an `IngressList`, or
a paginated list fails part-way, the run aborts before any write and the managed
hosts stay exactly as they are. One list is bounded to two minutes end to end, so
a hung API server fails the run instead of stalling the reconciler. `status.error`
says why and `lastSuccess` says how stale the state is — watch that pair, not
`lastRun`, when alerting. The only condition that deletes a managed host is a
*complete, successful* list that no longer derives it (which includes an Ingress
that simply lost its annotation).

**A misdirected `apiURL` cannot empty your config.** A `200` from something that
is not the Kubernetes API — another HTTPS service behind the same internal CA, a
mesh or gateway envelope — is rejected as a shape error rather than decoded as an
empty list, so it lands on the freeze path instead of deleting every managed host.

**Upstream gotcha.** `template.upstream` is the **ingress controller's** address,
not a Service: gpm is off-cluster, so `*.svc.cluster.local` cannot be resolved or
reached. Use `scheme: http` to the controller's plain port unless you have a
reason not to — with `https` the upstream host is what SNI and certificate
verification use, so a bare IP will fail the handshake.

Scopes: `ingress-discovery:read` for status, `ingress-discovery:write` for
reconcile.

## Backup & restore

`GET /api/backup` returns a gzip-compressed tar of the **declarative config
only** - every object YAML under `config/<kind>/` plus `config/settings.yaml`.
It does **not** include `.git` history (that's what `git clone`/`git bundle`
the config repo is for, if you want commit history too), the `certs/`
directory, or `session.db`. It needs the `admin` scope specifically, not
`*:read` - unlike the JSON API, the raw YAML carries the `api-tokens`
directory's stored digests, which are offline-crackable.

Because backup is admin-scoped, there is no narrower "backup:read" token you
can hand to a cron job - an automation credential that can back up the config
can, by the same scope, do everything else `admin` can. Mint one deliberately
for this and nothing else, and treat it with the same care as the break-glass
password:

```
curl -s -X PUT https://<admin>/api/api-tokens/backup-cron \
  -b "<admin session cookie>" -H "X-CSRF-Token: <token from /api/me>" \
  -H 'Content-Type: application/json' \
  -d '{"scopes":["admin"]}' | jq -r .token   # save this - shown once
```

**Cron:**

```
# /etc/cron.d/gpm-backup - runs as the gpm user
0 3 * * * gpm  curl -fsS -H "Authorization: Bearer $(cat /etc/gpm/backup_token)" \
  https://127.0.0.1:8081/api/backup \
  -o /var/backups/gpm/gpm-config-$(date +\%Y-\%m-\%dT\%H\%M\%S).tar.gz \
  && find /var/backups/gpm -name 'gpm-config-*.tar.gz' -mtime +30 -delete
```

(A `systemd` timer works the same way if you'd rather not use cron - a
`Type=oneshot` service running the same `curl` line, triggered by an
`OnCalendar=` timer unit.) `%` needs escaping as `\%` in a crontab; a plain
shell script invoked by cron doesn't have that gotcha.

**Restore** replaces the *entire* current config, validates the result, and
commits it as one revision. If the archive doesn't validate (a dangling
reference, a literal un-placeholdered secret, a corrupt/oversized upload) the
working tree is rolled back to the pre-restore commit and **nothing is
committed** - a bad restore attempt is a no-op, not a half-applied one:

```
curl -fsS -X POST https://<admin>/api/restore \
  -H "Authorization: Bearer gpm_<admin-scoped-token>" \
  -H 'Content-Type: application/gzip' \
  --data-binary @gpm-config-2026-08-22T030000.tar.gz
```

Max upload size is 8MB gzipped. `X-Config-Commit` in the response header (and
`.commit` in the JSON body) names the new commit, so a restore is itself
revertible with `POST /api/revert` like any other change.

**What a restore does *not* bring back**, and what to plan for separately:

- **`certs/`** - a `custom`-type `Certificate` references cert/key files by
  relative path in that directory; restoring config that names one does not
  restore the files themselves. Back up `certs/` alongside the config archive
  (a plain file copy - it isn't git-backed) if you use custom certificates.
  `acme`-type certificates need nothing extra: DNS-01 credentials round-trip
  in the restored `DNSProvider`/`Certificate` objects, and gpm re-issues on
  next load if the on-disk artifact is missing.
- **`session.db`** - restoring config never touches it; every admin session
  active at restore time stays valid (or invalid) exactly as it was before.
- **`api-tokens`** digests restore fine (they're plain config), but remember a
  scoped revert of just `api-tokens` is refused for the reason in
  [configuration.md](configuration.md#apitoken-configapi-tokens) - a whole-config
  restore is the one path that *does* touch them, by design, since it's meant
  to bring back an entire prior state including which tokens existed then.

Verify a backup is actually restorable periodically, not just that the cron
job exits `0` - a `GET`/`POST /api/restore` round trip against a disposable
test instance is the only thing that proves the archive is usable, the same
way an untested backup anywhere else is a hope, not a backup.

## High availability

Two instances can run as an active/standby pair (keepalived VIP, one static
leader, `git pull --ff-only` config replication, shared cert dir). The full
recipe - keepalived config, cert-dir layout, `GPM_SSO_SIGNING_KEY` sharing,
promotion, and what does not survive a failover - is in [ha.md](ha.md).

## Upgrading and rolling back

> **BREAKING - the next release renames two things, and neither is migrated for
> you.** The config store is a git working tree, so gpm refuses an unmigrated
> tree at startup with the exact fix in the error rather than authoring a commit
> you never made. Do both steps in the config repo *before* starting the new
> binary:
>
> 1. **`DeadHost` is now `ParkedHost`.** Rename the directory and commit it:
>
>    ```
>    git mv config/dead-hosts config/parked-hosts
>    git commit -m "Rename config/dead-hosts to config/parked-hosts"
>    ```
>
>    Startup and every reload fail while `config/dead-hosts/` still holds
>    objects and `config/parked-hosts/` holds none. The API path becomes
>    `/api/parked-hosts`, the token scope subject becomes `parked-hosts`
>    (update any API token that names `dead-hosts:read` / `dead-hosts:write`),
>    and the whole-config `deadHosts` key becomes `parkedHosts`.
>
> 2. **A stream host's `forwardHost`/`forwardPort` become a single `target`.**
>    Edit every file under `config/stream-hosts/` and commit:
>
>    ```yaml
>    # before                    # after
>    forwardHost: db.internal    target:
>    forwardPort: 5432             host: db.internal
>                                  port: 5432
>    ```
>
>    A file still using the old keys is **rejected at load** with an error
>    naming the new shape - it is never silently accepted with no backend.
>
> Restore is the one place the old name still works: a `GET /api/backup`
> archive taken before the rename restores fine, its `dead-hosts/` entries
> mapped onto `parked-hosts/` (logged at info). That mapping is one-way and
> restore-only; nothing is ever written back under the old name.

**Pin explicitly, and verify before you deploy.** Bump the image tag/digest
(GitOps) or the binary version (bare metal) deliberately rather than tracking
`latest` - see [Verifying the image](#verifying-the-image) above for the
cosign check to run against whatever you're about to pin. There is no
in-place "upgrade" operation: stop the old process/container, start the new
one against the same `/data` (or `/var/lib/gpm`) volume, and confirm
`GET /version` reports what you expect.

**Config compatibility.** `Config`/`Settings` carry an explicit
`schemaVersion` (currently `1`); a version bump would come with a documented,
reversible migration in the store layer, not a silent format break - none has
shipped yet. The two situations that matter *today*, both because an **older**
binary's kind map silently ignores a directory/field it doesn't know about
rather than erroring on it, so a downgrade can leave live objects invisible to
it instead of cleanly rejected:

- **Upstream groups** - roll the new binary out *before* the first host
  references an `upstream-groups` entry. Full reasoning under
  [Upstream-group health](#upstream-group-health-operations) above.
- **API tokens** - roll the new binary out *before* creating the first
  `api-tokens` object. Full reasoning under [API tokens](#api-tokens-automation)
  above.

The general rule those two both follow: roll a **newer** binary out before
adopting any config it introduces; roll an **older** binary back only after
confirming your config doesn't name a kind or field that predates it (an
older loader ignoring a directory isn't a downgrade-safety net - the objects
in it are simply invisible to that instance until you roll forward again).

**`session.db` has no versioned migrations to worry about yet** - the schema
is a single idempotent `CREATE TABLE IF NOT EXISTS` with no `ALTER TABLE`
history (see `internal/session/session.go`), so every released binary to date
reads and writes the identical schema and a downgrade is safe as far as
sessions are concerned. If a future release does add a schema change, treat
`session.db` as roll-forward only for that specific hop unless its changelog
entry says otherwise - the safe fallback if you must roll back past it is
deleting `session.db` (forces every admin to log in again; does not touch
`config/` or `certs/`).

**Rollback procedure**, once you've decided you need one:

1. Stop the new version.
2. If the upgrade you're undoing crossed one of the config-compatibility
   points above, resolve that first (e.g. don't roll back past the release
   that introduced your first `upstream-groups` reference while still using
   it) - `POST /api/revert` or `POST /api/restore` from a
   [pre-upgrade backup](#backup--restore) roll the *config* back
   independently of the binary version, and you may need both together.
3. Start the previous version against the same data directory.
4. Confirm `GET /version`, `GET /healthz`, and that proxied traffic is still
   routing - the same checks as a fresh deploy.

Taking a [`GET /api/backup`](#backup--restore) immediately before an upgrade
that changes anything nontrivial costs one `curl` and turns "roll back" into
a config restore instead of a guess.

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

## Metrics (Prometheus)

Off by default. Set `GPM_METRICS=1` (or `-metrics`) and restart; with it off,
`/metrics` answers `404`.

The endpoint is on the **admin server** (`-admin-addr`, default `:8081`), not
the data plane, because the payload is admin data: it names every proxy host,
stream host and certificate you have configured. It is gated by an admin-role
principal **plus**, for an API token, an explicit `metrics:read` scope - so the
credential you park in a Prometheus config can scrape and nothing else. Mint one
with a single scope:

```
curl -sS -X PUT https://gpm.example.com/api/api-tokens/prometheus \
  -H 'Content-Type: application/json' -b "$COOKIE" -H "X-CSRF-Token: $CSRF" \
  -d '{"scopes":["metrics:read"]}'
```

then scrape with `Authorization: Bearer gpm_...`. An admin browser session works
too and needs no scope.

**Series cardinality is bounded by design.** Every `host` label is the
ProxyHost/StreamHost **name** from committed config, never the client's `Host`
header - a header is attacker-chosen, and using it would let one client mint
unbounded series and exhaust the process. A request matching no host is labelled
`-`. Each metric additionally caps its series count and folds the rest into a
single `__overflow__` series, so no bug downstream of that rule can grow memory
without limit.

There is no `prometheus/client_golang` dependency: the exposition is a small
internal implementation (`internal/metrics`), matching this project's rule that
every third-party dependency has to earn its place.

| Metric | Type | Labels |
|--------|------|--------|
| `gpm_build_info` | gauge | `version`, `commit`, `go` |
| `gpm_http_requests_total` | counter | `host`, `method`, `status` (class, e.g. `2xx`) |
| `gpm_http_request_duration_seconds` | histogram | `host` |
| `gpm_http_requests_in_flight` | gauge | — |
| `gpm_http_request_bytes_total` | counter | `host` |
| `gpm_http_response_bytes_total` | counter | `host` |
| `gpm_http_upstream_errors_total` | counter | `host` |
| `gpm_http_websocket_upgrades_total` | counter | `host` |
| `gpm_denials_total` | counter | `host`, `reason` (`rate-limit`, `access-list`, `access-list-auth`, `guard`, `geo`, `bouncer`) |
| `gpm_stream_connections_active` | gauge | `host` |
| `gpm_stream_connections_total` | counter | `host` |
| `gpm_acme_certificate_expiry_timestamp_seconds` | gauge | `certificate` |
| `gpm_acme_renew_failures_total` | counter | `certificate` |
| `gpm_dns_sync_last_run_timestamp_seconds` | gauge | — |
| `gpm_dns_sync_last_success_timestamp_seconds` | gauge | — |
| `gpm_dns_sync_backend_up` | gauge | `backend` |
| `gpm_dns_sync_records_desired` | gauge | `backend` |
| `gpm_dns_sync_records_managed` | gauge | `backend` |
| `gpm_ingress_discovery_enabled` | gauge | — |
| `gpm_ingress_discovery_last_run_timestamp_seconds` | gauge | — |
| `gpm_ingress_discovery_last_success_timestamp_seconds` | gauge | — |
| `gpm_ingress_discovery_discovered_ingresses` | gauge | — |
| `gpm_ingress_discovery_managed_hosts` | gauge | — |
| `gpm_ha_role` | gauge | `role` (1 for this instance's role, 0 for the other) |
| `gpm_go_goroutines` | gauge | — |
| `gpm_go_memstats_alloc_bytes` | gauge | — |
| `gpm_go_memstats_sys_bytes` | gauge | — |

The ACME series exist only on the **leader** (it is the only issuer; a zero
expiry on a follower would read as "expired" to any sane alert). `LastRun` and
`LastSuccess` are separate on both reconcilers on purpose - freeze-on-error is
precisely the state where they diverge, so alert on the gap between them:

```
time() - gpm_ingress_discovery_last_success_timestamp_seconds > 3600
gpm_acme_certificate_expiry_timestamp_seconds - time() < 7 * 86400
```

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
same gate as `/api/` (same-origin guard + admin-role session) **plus an `admin`
scope check**. There is no token-in-URL or basic-auth mode - you authenticate with
an admin browser session (`gpm_session` cookie), either on the LAN admin listener
(`-admin-addr`, default `:8081`) or via a proxied admin domain if you've fronted
the admin panel with a host (see "Admin OIDC" above).

An API token works too, but **only one holding the `admin` scope**: a heap dump
and `/debug/pprof/cmdline` contain resolved backend credentials (Cloudflare,
Pi-hole) in cleartext, and every token principal is admin-*role* by construction,
so the role gate alone would hand a `proxy-hosts:read` token the process memory.
A resource-scoped token gets `403` here.

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
- Keep `-pprof` off unless actively profiling; it is admin-role **and**
  admin-scope gated but still needless attack surface when idle.
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
