# Parallel testing & cutover runbook

Validate go-proxy-manager fully **before touching the live Nginx Proxy Manager
install**. The plan: run the new app in parallel on the same server with
non-conflicting ports, import a *copy* of the real config, exercise every
feature, complete a security review, and only then cut over - with NPM kept
intact for instant rollback.

## Safety principles (do not skip)

- **Never read or write the live install directly.** Import from a *copy* of
  NPM's `/data`, not the running database.
- **No port conflicts.** The new app uses 8080/8443 (data plane) and
  127.0.0.1:8081 (admin). NPM keeps 80/443/81. Nothing overlaps.
- **Admin is localhost-only** until cutover (`127.0.0.1:8081`). Reach it with an
  SSH tunnel: `ssh -L 8081:127.0.0.1:8081 <server>`.
- **DNS is untouched.** All host testing uses `curl --resolve` / `--connect-to`,
  so public resolution still points at NPM the whole time.
- **ACME against Let's Encrypt STAGING first** (a throwaway test subdomain), so
  you never risk prod certs or hit rate limits. Imported certs are static copies,
  so day-one parallel testing needs no issuance at all.
- Keep the NPM container running and unchanged throughout.

## Phase 0 - build the image

```
# On the server (or build in CI and pull the GHCR image):
git clone https://github.com/Rake-Pro/go-proxy-manager && cd go-proxy-manager
docker build -t gpm:test .
```

## Phase 1 - snapshot the live NPM data (read-only)

```
# Copy, never mount the live dir. NPM keeps running.
sudo cp -a /path/to/npm/data /srv/npm-data-copy
```

## Phase 2 - preview the import (dry run, writes nothing)

```
docker run --rm -v /srv/npm-data-copy:/npm:ro gpm:test \
    import --npm-data /npm --dry-run
```

Read the summary and **every warning**. Expect warnings for: `advanced_config`
(raw nginx - re-create as typed middleware), `block_exploits`/`caching` (no
equivalent yet), and Let's Encrypt certs ("reconfigure as ACME for auto-renewal").
Decide what you'll re-create by hand before relying on the result.

## Phase 3 - real import into the test volume

```
export GPM_ADMIN_HASH="$(docker run --rm gpm:test hashpw 'choose-a-strong-pass')"

# import writes the config + copies cert PEMs into the gpm-data volume
docker compose -f deploy/compose.parallel.yaml run --rm \
    -v /srv/npm-data-copy:/npm:ro gpm import --npm-data /npm
```

## Phase 4 - start the parallel stack

```
docker compose -f deploy/compose.parallel.yaml up -d
docker logs -f gpm-test     # confirm "config loaded" + "data plane ... listening"
```

## Phase 5 - validation checklist

Run from the server. Replace `HOST` with each imported domain; the imported
*real* certs mean TLS validates, and upstreams are the same backends NPM proxies.

- [ ] **Admin loads**: tunnel, open `http://127.0.0.1:8081/`, log in with the
      break-glass admin. Every view renders from live data.
- [ ] **Honest version**: `curl -s 127.0.0.1:8081/version` shows the real commit.
- [ ] **TLS + SNI + proxy** (per host):
      `curl -sv --connect-to HOST:8443:127.0.0.1:8443 https://HOST:8443/ -o /dev/null`
      → expect HTTP 2xx/3xx from the real backend and the correct cert in the
      handshake.
- [ ] **Force-SSL**: `curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n'
      --connect-to HOST:8080:127.0.0.1:8080 http://HOST:8080/` → 308 to https.
- [ ] **HTTP/2**: confirm `http_version: 2` on the https call above.
- [ ] **Websockets**: exercise a host that uses them (e.g. Home Assistant) and
      confirm the upgrade works through 8443.
- [ ] **Access lists**: a host behind `lan-only` allows your LAN IP and the
      basic-auth user (re-set the password via `gpm hashpw` if needed), denies
      others.
- [ ] **REST API + git history**: edit a host in the UI; confirm a new commit in
      the config repo authored by you (`git -C <gpm-data>/config log`), and that
      the change takes effect live.
- [ ] **OIDC admin login**: in Authentik, create a **new, separate** OIDC app
      (do not reuse NPM's) with redirect URI `https://<test-host>/auth/callback`;
      set `externalBaseURL` in Settings to match; log in via Authentik.
- [ ] **Trusted forward-auth**: send a request through your Authentik forward-auth
      outpost (or simulate the trusted headers from a trusted source) and confirm
      one-click admin login; confirm an untrusted source is rejected.
- [ ] **ACME (staging)**: add a test certificate for a throwaway subdomain with
      `directoryURL = https://acme-staging-v02.api.letsencrypt.org/directory` and
      your Cloudflare DNS provider; confirm issuance + the data plane picks it up.
      Only after staging succeeds, try one prod issuance for a non-critical host.

## Phase 6 - cutover (only after Phase 5 + security review pass)

1. Re-create anything the import warned about (raw-nginx bits as typed middleware).
2. Switch imported LE certs from static-custom to managed ACME (Cloudflare DNS-01)
   and confirm renewal works.
3. Flip ports: stop NPM (`docker stop nginx-proxy-manager` - keep it, don't
   delete) and restart gpm on 80/443/81, or move it behind your existing entry.
   Same server IP → no DNS change needed.
4. Watch logs and re-run the Phase 5 host checks against the real ports.

## Rollback

The NPM container is untouched and stopped, not removed. To revert: stop gpm,
`docker start` NPM. Because the import was read-only against a copy, the original
`/data` is byte-for-byte intact.

## Gate

Do not begin Phase 6 until: (a) every Phase 5 box is checked, and (b) the
security review is complete and its findings resolved.
