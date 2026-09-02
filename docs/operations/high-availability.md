# High availability (two-node active/standby)

Phase 1 of [Design: High availability (gpm itself)](../design/ha.md), as shipped. Two gpm instances, one
floating VIP, one writer. No clustering dependency: replication is `git pull
--ff-only` for config, a shared/replicated cert dir for secrets, and a static
role in the environment for the writer.

```
              client
                |
             VIP (keepalived VRRP)
          /                  \
   node-a MASTER          node-b BACKUP
   GPM_HA_ROLE=leader     GPM_HA_ROLE=follower
   ACME writer            ACME off, admin read-only
   /data/config (git) <-- git pull --ff-only (poll)
   <cert-dir>/acme    <-- shared volume / rsync
```

## 1. Roles

| | Leader | Follower |
|---|---|---|
| `GPM_HA_ROLE` | `leader` (default) | `follower` |
| ACME renewal loop | runs | off |
| Kubernetes Ingress discovery | runs | off |
| Admin/API config writes | accepted | refused, `503` |
| Reads (UI, `GET /api/*`) | yes | yes |
| Config source | its own git repo | `git pull --ff-only` from the leader |

Both nodes serve traffic-side config identically; only *who may write* differs.
An unrecognised `GPM_HA_ROLE` is fatal at startup - a typo must never start a
second ACME writer.

A follower refuses every mutating admin/API request (`PUT`/`POST`/`DELETE`) with:

```
HTTP/1.1 503 Service Unavailable
{"error":"this instance runs as an HA follower (GPM_HA_ROLE=follower) and is
read-only: make config changes on the leader (GPM_HA_ROLE=leader), they
replicate here by git pull"}
```

The admin SPA reads `GET /api/capabilities` (`ha.role`, `ha.readOnly`) and greys
out the write controls on a follower, so the read-only state is visible before a
click rather than reported after one.

Leadership does **not** move on its own. Promoting the follower after a leader
loss is an operator action: set `GPM_HA_ROLE=leader` on the survivor and restart
it (and make sure the old leader stays down or becomes the follower). Automatic
election is phase 2.

## 2. keepalived VIP (traffic failover)

Point public DNS and the firewall/NAT forwards for `:80`, `:443` and any
published stream ports at the VIP, not at either node's own address. VRRP is an
L2 same-subnet mechanism, so there is no SNAT and the real client IP still
reaches gpm - access lists, GeoIP, rate limiting and XFF trust keep working
unchanged.

`/etc/keepalived/keepalived.conf` on the MASTER (make it the same node as
`GPM_HA_ROLE=leader`):

```
vrrp_script chk_gpm {
    script  "/usr/bin/curl -fsS -o /dev/null -k https://127.0.0.1/"
    interval 2
    timeout  2
    fall     2
    rise     2
    weight  -40
}

vrrp_instance gpm {
    state           MASTER
    interface       eth0
    virtual_router_id 51
    priority        150
    advert_int      1
    authentication {
        auth_type PASS
        auth_pass <shared-secret>
    }
    virtual_ipaddress {
        192.0.2.10/24
    }
    track_script {
        chk_gpm
    }
}
```

On the BACKUP node use `state BACKUP` and a lower `priority` (e.g. `100`);
everything else is identical. The health check demotes a node whose gpm is not
answering, so the VIP moves within a couple of VRRP intervals.

`virtual_router_id` must be unique on the subnet, and the two nodes must agree on
it and on `auth_pass`.

## 3. Shared cert dir and secrets

The config repo is git; the cert dir is **not** (it holds private keys). It
replicates on its own channel. The single-writer rule makes a shared volume safe:

```
<cert-dir>/                     GPM_CERT_DIR, default /data/certs
  acme/
    accounts/<hash>.key         ACME account key - both nodes must present the same one
    issued/<name>/              leaf cert + key + meta.json - leader writes, both read
  sso_signing.key               only when GPM_SSO_SIGNING_KEY is unset (see below)
  sso_not_before                SSO revocation watermark - shared, re-read on a ticker
```

Either put `<cert-dir>` on a shared volume (NFS, replicated block device) with
the leader as the only writer, or `rsync` it leader -> follower on a short
interval. Certificates are read at TLS handshake time, so the follower picks up a
renewal with no restart.

**`GPM_SSO_SIGNING_KEY`**: set the *same* value on both nodes. Data-plane SSO
cookies are stateless HMACs, so a shared key is what lets the survivor accept
cookies the other node minted - without it every SSO user re-authenticates at
failover. Setting it explicitly also keeps the key out of the file-sync path;
leave it unset only if `<cert-dir>` itself is shared (then `sso_signing.key` is
the shared copy).

**SSO revocation** (`POST /api/sso/revoke`, leader-only) moves the watermark in
`<cert-dir>/sso_not_before`. Every instance re-reads that file every 30s and
advances its in-memory watermark (never backwards), so a revoke issued on the
leader is honoured by the follower within one interval instead of at its next
restart.

## 4. Config replication

The follower pulls the leader's repo. Easiest is to seed the follower's config
dir by cloning the leader (a clone sets up `origin` and the branch tracking that
`git pull --ff-only` needs):

```
git clone ssh://gpm@node-a/data/config /data/config
```

For a follower that already has its own `/data/config`, wire the same thing by
hand (`BRANCH` is what `git -C /data/config branch --show-current` reports - gpm
creates the repo with git's default branch name):

```
git -C /data/config remote add origin ssh://gpm@node-a/data/config
git -C /data/config fetch origin
git -C /data/config branch --set-upstream-to=origin/$BRANCH $BRANCH
```

Then run the follower with `GPM_HA_ROLE=follower`. Every `GPM_HA_POLL_INTERVAL`
(default 20s) it runs `git pull --ff-only` and reloads the running config **only
when HEAD actually moved**.

- The follower never commits, so the repo cannot diverge by normal operation.
- A pull that is not a clean fast-forward is refused and logged as an error - gpm
  never merges, rebases or resets. Fix it by hand on the follower (it keeps
  serving the last synced config meanwhile).
- While the leader is down the pull fails every interval and is logged; the
  follower keeps serving the config it already has.

The SSH account needs read access to the leader's `/data/config` only.

## 5. What does not fail over

| State | Behaviour at failover |
|---|---|
| TCP/UDP stream sessions | **dropped, client reconnects** - listeners are already bound on both nodes, so the survivor accepts immediately; a UDP client's next packet opens a fresh backend session. Same semantics as a single-node restart |
| Admin sessions | lost; an admin logs in again on the survivor |
| Access-log ring | per-instance; the survivor shows only its own traffic |
| Rate-limit buckets | per-instance; in active/standby only one node serves, so limits stay correct |
| Login lockout / OIDC-login throttle counters | per-instance, reset |

HTTP requests in flight are lost like any connection reset; browsers retry.

## 6. Checklist

1. Both nodes on the same subnet, same gpm version, same image.
2. keepalived on both (`MASTER` = the leader node), VIP in DNS and NAT forwards.
3. `<cert-dir>` shared or rsynced; leader is the only writer.
4. Identical `GPM_SSO_SIGNING_KEY`, `GPM_LOCAL_ADMIN_PASSWORD_HASH` and
   `${ENV:...}` secrets on both nodes.
5. Leader: `GPM_HA_ROLE=leader` (or unset). Follower: `GPM_HA_ROLE=follower`
   plus the `origin` remote on `/data/config`.
6. Verify: `GET /api/capabilities` on the follower reports
   `"ha":{"role":"follower","readOnly":true}`; a write there returns `503`; a
   change committed on the leader appears on the follower within the poll
   interval (check the log line "HA follower: pulled and applied new config").
## 7. Role environment variables

Two instances can run as an active/standby pair with no clustering dependency.
The role is environment-only - it is not a config object, because it describes
*this process*, not the shared configuration.

| Env | Default | Effect |
|-----|---------|--------|
| `GPM_HA_ROLE` | `leader` | `leader`: runs the ACME renewal loop and the Ingress/Docker discovery reconcilers, accepts admin/API writes. `follower`: those loops off, every write refused with `503`, reads unaffected. An unrecognised value is a startup error |
| `GPM_HA_POLL_INTERVAL` | `20s` | How often a follower runs `git pull --ff-only` on the config repo and reloads if HEAD moved. Ignored on a leader |

A follower's config arrives only by pulling the leader's repo, so it never
commits and the two repos cannot diverge; a pull that is not a clean
fast-forward is logged and refused, never merged or reset.

`GET /api/capabilities` reports `"ha": {"role": "...", "readOnly": true|false}`,
which is what the admin UI uses to grey out write controls on a follower.

Both nodes must share `GPM_SSO_SIGNING_KEY` (identical value) and the ACME
material under `<cert-dir>/acme`, and the SSO revocation watermark
(`<cert-dir>/sso_not_before`) is re-read every 30s so a revoke propagates
without a restart. Full recipe - keepalived VIP, shared cert dir, promotion,
stream failover-with-reconnect - in sections 1-6 above.

## 8. Where this fits in deployment

Two instances can run as an active/standby pair (keepalived VIP, one static
leader, `git pull --ff-only` config replication, shared cert dir). The full
recipe - keepalived config, cert-dir layout, `GPM_SSO_SIGNING_KEY` sharing,
promotion, and what does not survive a failover - is in sections 1-6 above.
