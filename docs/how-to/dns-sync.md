# Publish DNS records with DNS sync

Point Pi-hole and/or Cloudflare records at the edge automatically, and preview
every change before the first write.

Configure the backends once under **Settings -> DNS sync** (see
[Settings: DNS sync](../reference/config/settings/dns-sync.md)), then opt
individual hosts in with `dns.lanDirect` / `dns.publicCname`.

## Prerequisites

- **Pi-hole v6** with an application password, reachable from the gpm container.
  The API session must be allowed to write configuration - a `403` means the
  session is read-only or the instance lacks `webserver.api.app_sudo`, and is
  surfaced verbatim in the sync status.
- **Cloudflare**: an existing `dns-providers` entry whose `config.apiToken` has
  `Zone:DNS:Edit` on the target zone. The same token the ACME solver uses is fine.

## Steps

1. **Preview before you enable.** `GET /api/dns-sync/plan` (the **Preview changes**
button next to *Reconcile now* in the settings UI) reads both backends and the
ownership ledger and reports exactly what a reconcile would create, adopt,
retarget and delete - writing nothing. Do this first on any resolver that already
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

2. **Opt hosts in** with a `dns` block, one host at a time:

   ```yaml
   # config/proxy-hosts/app.yaml
   dns: {lanDirect: true, publicCname: true}
   ```

3. **Reconcile and read the result.** Reconciles also fire automatically after
   any proxy-host write, settings change, restore or whole-config revert.

```
# Trigger a reconcile and read the result.
curl -s -X POST https://<admin>/api/dns-sync/reconcile -H 'Authorization: Bearer gpm_...' | jq
curl -s          https://<admin>/api/dns-sync/status    -H 'Authorization: Bearer gpm_...' | jq
# {"lastRun":"...","pihole":{"enabled":true,"ok":true,"desired":12,"managed":12,
#   "created":0,"adopted":0,"retargeted":0,"deleted":0,"skipped":0,"untouched":19}, ...}
```

The manual endpoint does **not** queue: while a reconcile is in flight it answers
`409 Conflict`, so a retry loop cannot stack requests behind a slow backend
(`/plan` answers `409` in the same situation).

## Verify

| Check | Expected on the first run | Expected on the second |
|---|---|---|
| `created + adopted` | Equals the number of opted-in domains | `0` |
| `deleted` | `0` - the ledger is empty, so nothing is deletable | `0` |
| `untouched` | Everything you maintain by hand | Unchanged |
| `skipped` | `0`. Anything else is a real finding: a name a host wants is held by a record gpm does not own, and both were left alone | `0` |
| `desired` / `managed` | Equal to each other | Equal, and stable |
| `GET /api/history` | One `DNS sync ledger: update` commit | No new commit |

Each reconcile that changes what gpm owns writes one commit to the config repo
(`DNS sync ledger: update`, authored `dns-sync`); a steady-state run commits
nothing. Changing `apexTarget` no longer orphans anything: records gpm created and
nobody has touched since are **retargeted** on the next reconcile. Records that
predate the ledger, or that somebody has re-pointed, are left alone and reported
as `skipped` / disowned - clean those up by hand.

Two things to expect in the logs. A record gpm **adopted** (rather than created)
is *released* when the config stops asking for it - and equally when `apexTarget`
moves, since a retarget is a delete plus a create: a warn line, the ledger entry
dropped, and the record left in the resolver exactly as you wrote it, for you to
re-point or remove by hand - gpm destroys only what it created. And every
deletion is logged at warn with the
`ledgerRev` that authorised it, because a whole-tree revert restores ownership
claims along with everything else; after reverting a config that ever contained
DNS-synced hosts, run `/api/dns-sync/plan` before letting a reconcile proceed
(see the revert caveat in [Settings: DNS sync](../reference/config/settings/dns-sync.md)).

Scopes: `dns-sync:read` for status and plan, `dns-sync:write` for reconcile.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `skipped > 0` | A name a host wants is held by a record gpm does not own | Resolve it by hand; gpm never shadows or replaces a foreign record |
| Pi-hole answers `403` | The API session is read-only, or the instance lacks `webserver.api.app_sudo` | Fix it on the Pi-hole side; retrying will not help |
| `409 Conflict` from `/reconcile` or `/plan` | A reconcile is already in flight; the endpoint never queues | Retry once it finishes |
| A record gpm published disappeared | The host was disabled, which withdraws its DNS | Re-enable the host; created records are recreated, adopted ones were released |
| A record was deleted after a revert | The revert restored an ownership claim reality had moved past | Run `/api/dns-sync/plan` before letting a reconcile proceed after any revert |
| An adopted record is not being cleaned up | Adoption is a claim, never permission to destroy; a released record is yours | Remove it by hand |
| A wildcard domain publishes nothing | Wildcards, and a domain equal to the apex target, are skipped by both backends | Publish the concrete names instead |
