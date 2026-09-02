# Settings: DNS sync

Publish CNAME records for opted-in proxy hosts to Pi-hole and/or Cloudflare,
tracked in a git-backed ownership ledger.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-dns-sync"></span> `dnsSync` | DNSSyncSettings | Optional DNS record reconcilers (below). |

Publishes CNAME records for the proxy hosts that opted in via their
[`dns` policy](../proxy-host.md). Both backends are independently
enabled; with both disabled (the default) the subsystem is inert and never
contacts anything.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-dns-sync-pihole-enabled"></span> `pihole.enabled` | bool | Turn on local (LAN) CNAME reconciliation. |
| <span id="settings-dns-sync-pihole-url"></span> `pihole.url` | string | Pi-hole base URL, absolute http/https, no `/api` suffix. Required when enabled. |
| <span id="settings-dns-sync-pihole-app-password"></span> `pihole.appPassword` | Secret | Pi-hole **application password** (placeholder-resolved). Used for `POST /api/auth`. |
| <span id="settings-dns-sync-pihole-apex-target"></span> `pihole.apexTarget` | string | CNAME target every managed record points at. Required when enabled. **Not an ownership marker**, see below. |
| <span id="settings-dns-sync-cloudflare-enabled"></span> `cloudflare.enabled` | bool | Turn on public zone reconciliation. |
| <span id="settings-dns-sync-cloudflare-dns-provider-ref"></span> `cloudflare.dnsProviderRef` | string | Name of an existing [DNSProvider](../dns-provider.md) whose `config.apiToken` is reused. Required when enabled. |
| <span id="settings-dns-sync-cloudflare-zone-name"></span> `cloudflare.zoneName` | string | Zone the records live in, e.g. `example.com`. Required when enabled. |
| <span id="settings-dns-sync-cloudflare-apex-target"></span> `cloudflare.apexTarget` | string | CNAME content every managed record points at. Required when enabled. |
| <span id="settings-dns-sync-cloudflare-proxied"></span> `cloudflare.proxied` | bool | Cloudflare orange-cloud flag on created records. Default `false` (DNS only). |

**Ownership: what gpm will and will not delete.** Reconcile is *full-state*: the
desired set is recomputed from the whole config on every run, so a record deleted
out of band is recreated and a host removed while gpm was down is still cleaned
up. Deletion, however, is limited to records **gpm created itself**, recorded
explicitly in the ownership ledger at `config/dns-ledger.yaml`:

```yaml
# config/dns-ledger.yaml - written by the reconciler, committed like everything else
schemaVersion: 1
pihole:
  - domain: app.example.com
    target: edge.example.com
    adopted: false      # gpm created this record, so gpm may delete it
cloudflare:
  - domain: www.example.com
    target: edge.example.com
    adopted: true       # the record was already there; gpm only claimed it
```

`adopted` records **how** the claim was acquired, and it is what decides whether
the record can ever be deleted. An entry with no `adopted` key at all (a ledger
written before this field existed) is read as **adopted**, deliberately: it is the
only reading of a missing field that cannot destroy a record on upgrade.

Per desired record, on every run:

| Backend state | What gpm does |
|---------------|---------------|
| absent | **create**, and record ownership (`adopted: false`) |
| present, right target, **not** in the ledger | **adopt**: record ownership (`adopted: true`), do not recreate (logged at info) |
| present, right target, already owned | nothing |
| **created by gpm**, still holding the target gpm wrote, but `apexTarget` has since changed | **retarget**: replace it and update the ledger (the replacement is again a record gpm created) |
| **adopted**, and `apexTarget` has since changed | **released, not retargeted**: the claim is dropped and the record left exactly where it is (logged at warn, counted as a skip) |
| present, different target, not owned | **skip and warn**: never shadowed, never replaced |
| in the ledger as **created by gpm**, no longer desired | **delete**, and drop from the ledger (logged at warn, with the ledger revision that authorised it) |
| in the ledger as **adopted**, no longer desired | **released, not deleted**: the claim is dropped and the record left exactly where it is (logged at warn) |
| **not in the ledger** | **never deleted**, whatever it points at |

**A record gpm adopted is never deleted *or* retargeted.** Adoption claims a
record somebody else made; it is not, and must never become, permission to
destroy it. So turning `dns.lanDirect` on for a name an operator had
hand-written, and then turning it off again, leaves their record untouched; gpm
simply stops managing it. A retarget is a delete followed by a create, so the
same rule applies when `apexTarget` moves: an adopted record is **released**
rather than re-pointed, both because re-pointing would destroy the operator's
record and because the replacement would be recorded as gpm-created, leaving a
later host removal free to delete the name outright. To move an adopted name to
a new apex, either delete the record and let the next reconcile create it (gpm
then owns it, and may later remove it), or re-point it yourself and let gpm
re-adopt it. The flip side is that gpm will not clean up an
adopted record for you: once released it is yours to remove by hand.

`apexTarget` is *not* an ownership marker. It says where managed records point,
nothing more. A hand-written CNAME aimed at the same host is adopted only if a
proxy host asks for that exact name, and is otherwise left completely alone.

**Cloudflare keeps a second marker.** Every record gpm creates there also carries
the comment `managed-by:gpm`, and deletion needs **both** the ledger entry and
the comment (re-checked inside the delete call itself). Adoption likewise only
claims records that already carry the comment; a record with the right content
but no comment is somebody else's and is skipped. Pi-hole/dnsmasq CNAMEs have no
comment field at all, which is exactly why the ledger exists.

**Enabling a backend for the first time is safe, and previewable.** With an empty
ledger gpm owns nothing, so it can only create and adopt: records matching the
desired set are adopted, and every other record on the backend is left untouched
and counted in the `untouched` field of the status. Run
`GET /api/dns-sync/plan` (or **Preview changes** in the settings UI) first: it
reads the backends and the ledger and reports exactly what a reconcile would
create, adopt, retarget and delete, without writing anything.

> **Do not hand-edit `config/dns-ledger.yaml`.** It is what authorises a DNS
> deletion. An entry with a missing domain or target, or a duplicate domain
> (compared case-insensitively, as the reconciler indexes it), is rejected at load
> and stops the reconcile rather than being acted on. To make gpm forget a record,
> delete the entry: that disowns it, it is not a deletion of the record itself.
> It is reverted along with the rest of the config by `POST /api/restore` / a
> whole-tree revert, which is deliberate: rolling the config back to before a host
> existed also rolls back gpm's claim on the record that host published.

> **Caveat: a revert can restore an ownership claim that is no longer true.** The
> ledger reverts with the tree, and history does not know what happened at the DNS
> backend in the meantime. If gpm created `x.example.com`, later deleted it, and an
> operator then recreated that name by hand, a revert to a commit from before the
> deletion restores gpm's claim on it, and the next reconcile, finding the name
> unwanted and the record matching what the claim says gpm left there, deletes the
> operator's record. The `adopted` rule above does **not** cover this case: the
> restored entry is one gpm genuinely created at the time it was written.
>
> Two things limit the damage. A reconcile whose ledger write is refused because
> the repo moved under it (which is what a concurrent revert looks like) re-reads
> and rewrites *without* the claims the revert withdrew, so a revert cannot be
> silently undone by a run already in flight. And every deletion is logged at
> **warn** with the ledger revision that authorised it (`ledgerRev`), so a record
> removed on the strength of a stale claim is identifiable after the fact. After
> reverting a config that ever contained DNS-synced hosts, run
> `GET /api/dns-sync/plan` before letting a reconcile proceed.

Wildcard domains (`*.example.com`) are skipped by both backends, as is a domain
equal to the apex target (which would be a CNAME loop). Disabled proxy hosts
contribute nothing.

`dnsProviderRef` is validated for *name shape* at settings-write time only:
settings are a separate singleton from the object graph, so a reference to a
missing DNSProvider surfaces at reconcile time in the sync status, not as a
rejected write.

A reconcile is triggered automatically after any proxy-host write, settings
change, restore or whole-config revert (non-blocking, and bursts coalesce into a
single run), and can be run on demand with `POST /api/dns-sync/reconcile`. The
manual endpoint never queues: if a reconcile is already running it answers **409
Conflict** rather than blocking, so repeated clicks cannot stack requests behind a
slow backend. `GET /api/dns-sync/status` reports the last run per backend
(`desired`, `managed`, `created`, `adopted`, `retargeted`, `deleted`, `skipped`,
`untouched`), and `GET /api/dns-sync/plan` returns the same decisions as a dry
run without touching anything (`409` while a reconcile is in flight, for the same
reason). Both take `dns-sync:read`. A Pi-hole `403` is
surfaced as a distinct error: it means the session is read-only or the instance
was built without `webserver.api.app_sudo`, which retrying will not fix.
