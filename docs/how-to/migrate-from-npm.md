# Migrating from Nginx Proxy Manager

A safe path from an existing Nginx Proxy Manager (NPM) or NPMplus install to
go-proxy-manager: run the two side by side, import a copy of the real config,
validate every feature against the new instance without touching live traffic,
then cut over. NPM stays untouched - and available as instant rollback - until
you flip the final switch.

**Time:** ~60-90 min validation, ~15 min cutover.
**Risk:** none through validation (read-only import, alternate ports, NPM
untouched). Cutover is the only disruptive step, and it is reversible.

## Why run in parallel

Importing straight into production and hoping the mapping is right is how
outages happen. Standing gpm up next to NPM on different ports lets you:

- Import a **copy** of NPM's config and certs, never the live database.
- Exercise TLS/SNI, routing, access lists, SSO, and forward-auth against the
  real config, before any client ever reaches gpm.
- Keep NPM serving all real traffic the entire time - public DNS does not
  change until Step 6.

## Prerequisites

- Docker / Docker Compose on the NPM host (or wherever you'll run gpm).
- Shell access to NPM's data directory (its SQLite database and, if
  Let's Encrypt is in use, its certificate files).
- A copy of this repo's `deploy/compose.parallel.yaml`, which runs gpm on
  non-conflicting ports (`8880`/`8843` data plane, `127.0.0.1:8081` admin) so
  it can sit beside NPM's `80`/`443`/`81` without collision. Adjust the ports
  if any of those are also taken on your host.

## Step 1 - Snapshot NPM's data (read-only)

The import must read a **copy**, never the database NPM is actively writing.
Certificate private keys are often root-owned, so copy with elevated
permissions and make the snapshot world-readable so gpm's non-root container
user can read it:

```
sudo rm -rf /srv/gpm-import
mkdir -p /srv/gpm-import/letsencrypt
sudo cp -a /path/to/npm/data/.        /srv/gpm-import/
sudo cp -a /path/to/npm/letsencrypt/. /srv/gpm-import/letsencrypt/   # if certs live separately
sudo chmod -R a+rX /srv/gpm-import
```

Verify the snapshot actually contains a database and at least one cert:

```
ls /srv/gpm-import/database.sqlite /srv/gpm-import/letsencrypt/live/*/fullchain.pem >/dev/null && echo OK
```

The snapshot holds private keys - delete it once you're done (`sudo rm -rf`).

## Step 2 - Preview the import (writes nothing)

```
docker run --rm -v /srv/gpm-import:/npm:ro ghcr.io/rake-pro/go-proxy-manager \
  import -npm-data /npm -dry-run
```

This prints a summary (host / certificate / access-list counts) and warnings
for anything it can't map automatically - most commonly raw `advanced_config`
nginx snippets, which need re-expressing as typed middlewares by hand. Read
every warning before continuing; note what you'll need to recreate manually.

If it fails with "no NPM sqlite database found", the snapshot directory must
directly contain `database.sqlite`. If it reports missing certificate files,
the `letsencrypt/` copy wasn't done with `sudo` + `chmod -R a+rX` above.

### What NPM's tables become

The importer reads NPM's schema as-is and emits gpm's own object kinds. Where
the two projects name the same thing differently, the gpm name is what you will
see in the UI, the API and `config/`:

| NPM table | gpm kind | `config/` directory | API path |
|-----------|----------|---------------------|----------|
| `proxy_host` | ProxyHost | `proxy-hosts/` | `/api/proxy-hosts` |
| `redirection_host` | RedirectHost | `redirect-hosts/` | `/api/redirect-hosts` |
| `dead_host` | **ParkedHost** | `parked-hosts/` | `/api/parked-hosts` |
| `stream` | StreamHost | `stream-hosts/` | `/api/stream-hosts` |
| `access_list` (+ `access_list_auth` / `access_list_client`) | AccessList | `access-lists/` | `/api/access-lists` |
| `certificate` | Certificate | `certificates/` | `/api/certificates` |

NPM's `dead_host` is a domain that answers 404 and nothing else. gpm calls that
a **parked host** - the domain is reserved without serving anything - because
nothing about it is dead: it is a live, TLS-terminating vhost doing exactly what
it was configured to do. NPM's `stream.forwarding_host`/`forwarding_port` become
the stream host's single `target: {host, port}`.

**Not imported: NPM's local user accounts, 2FA settings, or audit log** -
go-proxy-manager has no local-user table to import them into (a single
break-glass local admin plus OIDC group-to-role mapping instead; see
[Security model](../concepts/security-model.md)), so plan your
post-cutover admin access (SSO groups, or the break-glass password) before
Step 5 rather than assuming any NPM login carries over.

## Step 3 - Stand up gpm alongside NPM

Bring gpm up on the non-conflicting ports from `deploy/compose.parallel.yaml`,
with a break-glass local admin password so you have a login independent of SSO
while you validate:

```
docker run --rm ghcr.io/rake-pro/go-proxy-manager hashpw 'choose-a-password'
# put the resulting hash in GPM_LOCAL_ADMIN_PASSWORD_HASH (see .env.example)

docker compose -f deploy/compose.parallel.yaml run --rm \
  -v /srv/gpm-import:/npm:ro gpm import -npm-data /npm
docker compose -f deploy/compose.parallel.yaml up -d
```

Check the admin API is up:

```
curl -fsS http://127.0.0.1:8081/healthz && echo   # -> ok
```

## Step 4 - Validate parity, without taking any real traffic

Public DNS still points at NPM, so reach gpm's alternate ports directly with
curl's `--connect-to` (or `--resolve`), which lets you address a real hostname
against a specific IP/port without changing DNS or `/etc/hosts`:

```
# TLS + SNI + routing for a given host
curl -sv --connect-to app.example.com:8843:127.0.0.1:8843 \
  https://app.example.com:8843/ -o /dev/null
# -> 2xx/3xx, and the served certificate's subject matches app.example.com

# force-SSL redirect
curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' \
  --connect-to app.example.com:8880:127.0.0.1:8880 \
  http://app.example.com:8880/
# -> 308 https://app.example.com/
```

If gpm is on a different host than the client you're testing from, use
`--resolve app.example.com:8843:203.0.113.10` instead, pointing at that host's
address (a `203.0.113.0/24` example address here - use the real one).

Work through the features your NPM install actually uses:

- **Admin login** - log in to the web UI with the break-glass password.
- **TLS / SNI / routing** - per proxied host, as above.
- **Force-SSL** and **HTTP/2** - as above, plus `-w '%{http_version}\n'`.
- **WebSockets** - a host that upgrades connections still works through gpm.
- **Access lists** - allow/deny by CIDR and basic-auth behave as configured;
  a denied client's logged IP is its real address.
- **REST API / config history** - edit a host in the UI and confirm a new
  commit appears in the config store and the change takes effect live.
- **SSO** - configure a **new, separate** OIDC application at your identity
  provider for gpm (don't reuse NPM's), pointed at gpm's admin URL, and confirm
  login. See [Set up admin single sign-on (OIDC)](admin-oidc-sso.md).
- **Forward-auth**, if you use it - trusted source is let through, untrusted
  is rejected.
- **ACME**, if certificates aren't purely imported - issue against your CA's
  **staging** directory for a throwaway subdomain first, then issue for real
  once that succeeds.

Don't proceed to cutover until every feature you rely on has been checked
against gpm this way, and any warnings from Step 2 have been resolved.

## Step 5 - Cut over

The only disruptive step. Once validation is complete:

1. Re-create anything the import warned about that you haven't yet; switch
   any imported Let's Encrypt certs to gpm-managed ACME if you want automatic
   renewal.
2. Stop NPM - keep it installed, don't remove it, so rollback stays cheap:
   ```
   docker stop <npm-container>
   ```
3. Move gpm onto the real ports (edit `80`/`443` into its compose file in
   place of the parallel-test ports) and restart it:
   ```
   docker compose up -d
   ```
4. Point your edge (DNS, load balancer, or upstream router - whatever
   currently sends traffic to the NPM host) at the gpm host/port instead.

Verify from an external client, over real DNS, that each host loads over
HTTPS with a valid certificate and reaches its backend. Re-run the Step 4
checks against the real ports (no `--connect-to` needed anymore) - spot-check
at least one SSO-gated and one access-list-gated host.

## Rollback

```
docker compose down
docker start <npm-container>
```

Point your edge back at NPM. The import in Step 2/3 was read-only against a
copy, so NPM's data directory was never modified and comes back exactly as it
was.

## Done when

- Every feature you rely on has been validated against gpm per Step 4.
- After cutover, external clients reach every host over HTTPS via gpm, SSO and
  access lists behave as before, and NPM is stopped-but-retained for rollback.
## Appendix: the one-shot importer

```
gpm import -npm-data /path/to/npm/data -config-dir /data/config -cert-dir /data/certs
```

Best-effort: it maps proxy/redirect/dead/stream hosts, access lists, and
certificates into git-backed config and copies the cert files, printing a count,
the commit hash, and warnings for anything it could not translate (e.g. raw
`advanced_config` nginx snippets - re-express those as typed middlewares, or as
an upstream group for a custom load-balanced `upstream` block). Use
`-dry-run` first to review the mapping without writing anything.

Steps 1-5 above are the full walkthrough: running gpm alongside a live NPM
install, validating parity before any traffic moves, then cutting over.
