# Configuration model

How gpm stores configuration: typed YAML objects in a git repository, one
object per file, validated as a whole graph on every write.

go-proxy-manager is configured by a set of typed YAML objects in a git-backed
directory (default `/data/config`). You can edit them through the web UI / REST
API, or write the files directly and let the daemon load them on start/reload.
## Layout

```
config/
  settings.yaml            # singleton app settings
  dns-ledger.yaml          # singleton DNS record-ownership ledger (reconciler-written)
  access-list-sources.yaml # singleton fetched access-list source sets (fetcher-written)
  proxy-hosts/<name>.yaml
  redirect-hosts/<name>.yaml
  stream-hosts/<name>.yaml
  parked-hosts/<name>.yaml
  certificates/<name>.yaml
  dns-providers/<name>.yaml
  identity-providers/<name>.yaml
  access-lists/<name>.yaml
  middlewares/<name>.yaml
  api-tokens/<name>.yaml
```

One object per file; the file's base name must equal the object's `name`. The
directory is a git repository: every change made through the API is a commit,
and the whole graph is validated before it is accepted (a reference to a
non-existent certificate, middleware, access list, identity provider, or DNS
provider is a load-time error, and an object cannot be deleted while another
references it).

## Unknown keys

Both loaders (per-object files and `settings.yaml`) decode leniently: a key the
running binary's struct does not recognise (most often one only a **newer**
gpm writes) is silently ignored rather than rejected, so a config a newer
release wrote still loads on an older one instead of failing outright. Load
logs one `WARN` per affected file and lists it under `GET /health`'s
`configWarnings`. Silently ignored is also silently **dropped** the moment
that object is next written (including by an automatic reconciler commit, not
only an operator save), which makes an unsupported rollback the one case this
leniency does not cover cleanly; see [Rollback](../operations/upgrading.md#rollback).

## Common fields (every object)

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | yes | Identity and filename. Lowercase alphanumeric plus `-_.`, must start and end alphanumeric, 1-254 chars. |
| `displayName` | string | no | Human label for the UI. |
| `labels` | map | no | Arbitrary key/value metadata. **`gpm.rake.pro/managed-by` is reserved**: see below. (The exact key follows `ingressDiscovery.annotationPrefix`; `gpm.rake.pro` is the default and what every example below uses.) |
| `tags` | []string | no | Flat, free-form labels for grouping/filtering. On the Proxy Hosts list they render as chips and are matched by the filter box. |
| `disabled` | bool | no | Keep the object in config but exclude it from the running data plane. |

> **`gpm.rake.pro/managed-by` is a reserved label: do not set it by hand.** It
> marks an object as owned by an automated reconciler. Adding
> `gpm.rake.pro/managed-by: ingress-discovery` (or
> `docker-discovery`) to a proxy host you wrote yourself hands it to that
> reconciler, which will **delete it** on the next poll, because no annotated
> `Ingress` (or labelled container) derives it. Removing the label is the supported
> way to adopt a discovered host permanently; adding it is never the way to give
> one away.

> **A managed host is not editable by hand: every edit besides `disabled` and
> `maintenance` is reverted on the next poll.** Discovery derives the whole object from the
> template and the `Ingress` and writes it back whenever it differs from what is
> stored, so an edited `displayName`, added `tags`, `timeouts`, `locations` or
> `robotsNoIndex` all survive at most until the next reconcile (default 60s).
>
> **`disabled: true` is the exception: it is operator-owned state.** Discovery
> honours an operator-set `disabled` and never clears it itself: hand-disabling a
> managed host (in the UI or by editing
> `config/proxy-hosts/<name>.yaml`) survives every subsequent poll, keeps the
> object out of the running data plane, and withdraws its DNS records exactly
> like disabling a hand-written host does. Editing the Ingress cannot undo this:
> a cluster user has no way to re-enable a host you disabled. The one case where
> a `disabled: true` a poll wrote itself IS cleared automatically is the
> fail-closed hold below (an unresolvable profile): that disable is discovery's
> own, and the very next reconcile that resolves the profile again lifts it. The
> label `gpm.rake.pro/disabled-by: ingress-discovery` on the stored object is how
> the two are told apart: never set or remove it by hand; it exists only for
> discovery to recognise a hold it placed itself.
>
> **`maintenance: true` is operator-owned in exactly the same way.** No Ingress
> annotation derives it, so discovery carries the stored value forward on every
> reconcile instead of resetting it: a managed host put into maintenance (in the
> UI or by editing `config/proxy-hosts/<name>.yaml`) stays in maintenance until
> an operator takes it out, whatever the cluster does to the `Ingress` in the
> meantime. Unlike `disabled` it keeps the host's domains, certificate and DNS
> records: see [Maintenance mode](../reference/config/settings/maintenance.md).
>
> With that, there is no longer only an "emergency" off-switch (disabling in the
> UI now works), but the other two routes remain useful:
>
> - **Remove `gpm.rake.pro/managed` from the `Ingress`** (or delete the
>   `Ingress`). Discovery stops deriving the host and deletes it on the next
>   successful reconcile. This is the clean route for taking a service out of
>   discovery for good, but it needs cluster access.
> - **Remove the `gpm.rake.pro/managed-by` label from the proxy host.** The
>   object becomes operator-authored, discovery refuses to touch it ever again,
>   and you can then edit it freely. This still needs no cluster access, and is
>   still **permanent**: the corresponding `Ingress` is skipped with a warning
>   from then on, and putting the host back under discovery means deleting it and
>   letting the next poll recreate it.
>
> Preview what a reconcile would do to a specific host with `GET
> /api/ingress-discovery/plan` (or **Preview changes** in the settings UI) before
> disabling by hand, if in doubt.

## Domains are exclusive

The data plane routes by hostname, so **at most one enabled host may claim a
given domain**. Two enabled proxy, redirect or parked hosts listing the same domain
are rejected at load time (`hosts "a" and "b" both claim domain "x.example.com"`)
rather than resolved by whichever file happens to be read last. *Disabled* hosts
are exempt: they are excluded from the running data plane entirely, so staging a
replacement host beside the live one stays legal: enable the new one in the same
change that disables the old one.

## Secrets

Secret-valued fields (API tokens, client secrets, etc.) must be **placeholders**,
not literal values:

```
${ENV:CF_API_TOKEN}        # resolved from the environment variable
${FILE:/run/secrets/token} # resolved from a file (e.g. a Docker secret), trimmed
```

Placeholders resolve lazily, at the moment the secret is used. Committing a
literal secret is refused with `refusing to commit literal secret(s): ...`, on
`Save`, on `SaveSettings`, **and on `Restore`** (an uploaded backup archive cannot
smuggle a plaintext secret onto disk or into git history; a refused restore rolls
the working tree back and commits nothing). In API responses, literal secrets are
redacted to `***`; placeholders are returned verbatim.

`${FILE:...}` reads are confined to an allowlisted root, defaulting to
`/run/secrets`. A path that is relative, or outside the allowed root (including
via `..`), is refused, so a config write cannot turn a file-backed secret into
an arbitrary host-file read. Override the allowed roots with the
`GPM_SECRET_FILE_ROOTS` environment variable (a list of absolute directories,
separated by the OS path-list separator, e.g. `:` on Linux).

`${ENV:...}` resolution has two guards. gpm's own sensitive process env vars
(`GPM_SSO_SIGNING_KEY` and `GPM_LOCAL_ADMIN_PASSWORD_HASH`) are **never**
resolvable via a `${ENV:...}` placeholder, so an admin-authored config value
cannot exfiltrate them (e.g. as a webhook secret posted to an attacker URL). By
default any other env var name resolves. To lock this down further, set
`GPM_SECRET_ENV_PREFIXES` to a comma-separated list of allowed name prefixes
(e.g. `GPM_SECRET_,APP_`); then only `${ENV:...}` names carrying one of those
prefixes resolve and everything else is refused.
