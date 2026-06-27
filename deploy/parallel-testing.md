# Runbook: Validate go-proxy-manager beside an existing NPM, then cut over

**Purpose:** Run go-proxy-manager in parallel with a live Nginx Proxy Manager
install, import a copy of the real config, validate every feature, then cut over
- with zero risk to the running install until the final step.
**Audience:** operator with shell + docker on the NPM host.
**Time:** ~60-90 min validation; ~15 min cutover.
**Risk:** None through Step 6 (read-only copy, alternate ports, NPM untouched).
Step 8 (cutover) is the only disruptive step, and it is reversible (Rollback).

This is the generic runbook. For the example.com homelab host, the filled-in instance
lives at `homelab/go-proxy-manager/parallel-testing.md` in the GitOps repo. The
reusable template these follow is `docs/runbook-template.md`.

## Prerequisites

- [ ] SSH + `docker` / `docker compose` on the NPM host.
- [ ] `docker login ghcr.io` works (the image is private), or build locally.
- [ ] This repo's `deploy/compose.parallel.yaml` is on the host.
- [ ] A DNS-provider API token (only needed at Step 6, ACME).

## Variables

| Placeholder  | Meaning                                     | Value / example                          |
| ------------ | ------------------------------------------- | ---------------------------------------- |
| `<NPM_DIR>`  | NPM's compose dir (holds `data/`)           | `/opt/nginx-proxy-manager`               |
| `<SNAPSHOT>` | read-only copy of NPM data for import       | `/srv/gpm-import`                        |
| `<HOST>`     | a domain you are validating                 | `app.example.com`                        |
| `<ADMIN_PW>` | break-glass admin password you choose       | (your choice)                            |
| `IMAGE`      | the container image                         | `ghcr.io/rake-pro/go-proxy-manager:main` |

Run `docker compose` commands from the directory holding `compose.parallel.yaml`,
or add `-f deploy/compose.parallel.yaml`.

---

## Step 1 - Snapshot NPM's data (read-only)

Why: the import must read a copy, never the live database NPM is writing.

Run:
```
sudo cp -a <NPM_DIR>/data <SNAPSHOT>
sudo cp -a <NPM_DIR>/letsencrypt <SNAPSHOT>/letsencrypt   # if LE certs live separately
```

Expect: the snapshot contains NPM's SQLite DB (and its certs).

Verify:
```
ls <SNAPSHOT>/database.sqlite >/dev/null && echo OK
```
-> prints `OK`.

---

## Step 2 - Preview the import (writes nothing)

Why: see exactly what maps and what will not, before any write.

Run:
```
docker run --rm -v <SNAPSHOT>:/npm:ro IMAGE import --npm-data /npm --dry-run
```

Expect: a summary (host / cert / access-list counts) and a list of warnings.

Verify: the counts roughly match your NPM setup and you have read every warning
(raw `advanced_config`, `block_exploits`, `caching`, "reconfigure as ACME" for
Let's Encrypt certs). Note what you will re-create by hand.

If it fails (`no NPM sqlite database found`): `<SNAPSHOT>` must directly contain
`database.sqlite` (re-check `<NPM_DIR>/data`).

---

## Step 3 - Configure the break-glass admin

Why: local admin is the anti-lockout login while you validate SSO.

Run:
```
cp .env.example .env   # if present; otherwise set the var in your environment
echo "GPM_LOCAL_ADMIN_PASSWORD_HASH=$(docker run --rm IMAGE hashpw '<ADMIN_PW>')" >> .env
```

Expect: `.env` has a `GPM_LOCAL_ADMIN_PASSWORD_HASH=$2a$...` line.

Verify:
```
grep -q 'GPM_LOCAL_ADMIN_PASSWORD_HASH=\$2' .env && echo OK
```
-> prints `OK`. (Never commit `.env`.)

---

## Step 4 - Run the import

Why: write the mapped config + certs into the stack's data volume.

Run:
```
docker compose run --rm -v <SNAPSHOT>:/npm:ro gpm import --npm-data /npm
```

Expect: ends with `Imported N objects into /data/config (commit <sha>).`

Verify:
```
docker compose run --rm gpm sh -c 'git -C /data/config log --oneline -1'
```
-> shows the `Import from NPM/NPMplus` commit.

---

## Step 5 - Start the parallel stack

Why: bring the new app up on alternate ports, beside the untouched NPM.

Run:
```
docker compose up -d
docker compose logs -f gpm
```

Expect: `config loaded`, then both data-plane listeners and the admin server come
up with no repeated errors. (Ctrl-C stops following; the container keeps running.)

Verify:
```
curl -fsS http://127.0.0.1:8081/healthz && echo
```
-> prints `ok`.

---

## Step 6 - Validate every feature

Why: confirm parity against production reality before trusting it. Public DNS
still points at NPM throughout - these checks use `--connect-to` to reach the new
app directly. Tunnel the admin: `ssh -L 8081:127.0.0.1:8081 <NPM_HOST>`.

Tick each only when it passes:

- [ ] **Admin login** - `http://127.0.0.1:8081/`, log in `admin` / `<ADMIN_PW>`.
- [ ] **Honest version** - `curl -s 127.0.0.1:8081/version` shows the real commit.
- [ ] **TLS + SNI + proxy** (per `<HOST>`):
      `curl -sv --connect-to <HOST>:8843:127.0.0.1:8843 https://<HOST>:8843/ -o /dev/null`
      -> 2xx/3xx; served cert matches `<HOST>`.
- [ ] **Force-SSL** -
      `curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' --connect-to <HOST>:8880:127.0.0.1:8880 http://<HOST>:8880/`
      -> `308 https://<HOST>/`.
- [ ] **HTTP/2** - add `-w '%{http_version}\n'` to the TLS call -> `2`.
- [ ] **Websockets** - a ws host upgrades through 8843.
- [ ] **Access lists** - allow/deny by IP + basic auth behave; the denied client's
      logged IP is its real address (proves the single-homed network is correct).
- [ ] **REST API + git history** - edit a host in the UI; a new authored commit
      appears in `/data/config` and the change is live.
- [ ] **OIDC admin login** - via a **new, separate** OIDC app on your IdP; set
      `externalBaseURL` to match the redirect.
- [ ] **Forward-auth** - trusted source -> one-click admin; untrusted -> rejected.
- [ ] **ACME (staging first)** - issue for a throwaway subdomain against your CA's
      **staging** directory, confirm pickup, then one production issuance.

---

## Step 7 - GATE: do not proceed until both are true

> **Cutover is blocked until:**
> 1. Every box in Step 6 is checked, and
> 2. The codebase security review is complete and its findings are resolved.
>
> If either is incomplete, stop. The parallel stack can run indefinitely with no
> impact on NPM.

---

## Step 8 - Cut over from NPM

Why: make go-proxy-manager the edge. The only disruptive step.

Run:
```
# 1. re-create anything the import warned about; switch imported LE certs to
#    managed ACME in the UI.
# 2. stop NPM - keep the container for rollback, do not remove it:
docker stop <npm-container>
# 3. move the stack onto 80/443 (edit the ports in the compose) and:
docker compose up -d
```

Expect: go-proxy-manager listens on 80/443; NPM is stopped but present.

Verify: from an external client (real DNS), each `<HOST>` loads over HTTPS with a
valid cert and reaches its backend.

---

## Step 9 - Verify after cutover

Run:
```
docker compose logs --tail=50 gpm
```

Expect: no error churn; certs load and proxied requests succeed.

Verify: re-run the Step 6 `<HOST>` checks against the real ports (plain
`https://<HOST>/`, no `--connect-to`). Spot-check an SSO-gated and an
access-list-gated host.

---

## Rollback

```
docker compose down
docker start <npm-container>
```
The import was read-only against a copy, so NPM's `/data` is byte-for-byte intact.

## Done when

- All Step 6 checks pass and the security review is resolved (Step 7 gate).
- After cutover, external clients reach every host over HTTPS via go-proxy-manager,
  SSO and access lists behave, and NPM is stopped-but-retained for rollback.
