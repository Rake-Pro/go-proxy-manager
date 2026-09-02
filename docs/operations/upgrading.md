# Upgrading and rolling back

Pin deliberately, migrate what a release asks for, and roll back with the
config as a separate axis from the binary.

## Upgrade notes by version

Every release that needs an operator action, newest first. A version absent from
this table needs none: stop the old process, start the new one against the same
data directory.

| Version | Action required | Detail |
|---|---|---|
| Unreleased | **Rename `config/dead-hosts` to `config/parked-hosts`** before starting the new binary. The API path becomes `/api/parked-hosts`, the token-scope subject becomes `parked-hosts`, and the whole-config key becomes `parkedHosts`. | Below, and [ParkedHost](../reference/config/parked-host.md) |
| Unreleased | **Replace each stream host's `forwardHost`/`forwardPort` with a single `target: {host, port}`.** A file still using the old keys is rejected at load. | Below, and [StreamHost](../reference/config/stream-host.md) |
| Unreleased | **Only if you relied on `forwardAuth.trustedProxies` for client-IP resolution:** copy the same CIDRs into `settings.trustedProxies`. It no longer doubles as `X-Forwarded-For` trust. gpm logs a `WARN` naming the exact block to paste. | [Client IP and the three trust tiers](../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers) |
| Unreleased | **No action now, but plan for v2:** `AccessList.basicAuth` and `AccessList.satisfyAny` are deprecated and still enforced. v2 refuses a config that carries them. | [Migrate access-list basic auth](../how-to/migrate-basic-auth.md) |
| 1.0.x, all | **Roll a newer binary out before adopting config it introduces.** An older binary's kind map silently ignores a directory it does not know, so a downgrade leaves live objects invisible rather than cleanly rejected. Applies to `upstream-groups` and `api-tokens`. | Below |

## The next release, in full

> **BREAKING: the next release renames two things, and neither is migrated for
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
>    naming the new shape; it is never silently accepted with no backend.
>
> Restore is the one place the old name still works: a `GET /api/backup`
> archive taken before the rename restores fine, its `dead-hosts/` entries
> mapped onto `parked-hosts/` (logged at info). That mapping is one-way and
> restore-only; nothing is ever written back under the old name.

**Pin explicitly, and verify before you deploy.** Bump the image tag/digest
(GitOps) or the binary version (bare metal) deliberately rather than tracking
`latest`, see [Verifying the image](../getting-started/install-docker.md#verifying-the-image) for the
cosign check to run against whatever you're about to pin. There is no
in-place "upgrade" operation: stop the old process/container, start the new
one against the same `/data` (or `/var/lib/gpm`) volume, and confirm
`GET /version` reports what you expect.

**Config compatibility.** `Config`/`Settings` carry an explicit
`schemaVersion` (currently `1`); a version bump would come with a documented,
reversible migration in the store layer, not a silent format break: none has
shipped yet. The two situations that matter *today*, both because an **older**
binary's kind map silently ignores a directory/field it doesn't know about
rather than erroring on it, so a downgrade can leave live objects invisible to
it instead of cleanly rejected:

- **Upstream groups**: roll the new binary out *before* the first host
  references an `upstream-groups` entry. Full reasoning under
  [Upstream-group health](../reference/config/upstream-group.md#watching-live-health-operations) above.
- **API tokens**: roll the new binary out *before* creating the first
  `api-tokens` object. Full reasoning under [API tokens](../reference/api.md#api-tokens-automation)
  above.

The general rule those two both follow: roll a **newer** binary out before
adopting any config it introduces; roll an **older** binary back only after
confirming your config doesn't name a kind or field that predates it (an
older loader ignoring a directory isn't a downgrade-safety net; the objects
in it are simply invisible to that instance until you roll forward again).

**`session.db` has no versioned migrations to worry about yet**: the schema
is a single idempotent `CREATE TABLE IF NOT EXISTS` with no `ALTER TABLE`
history (see `internal/session/session.go`), so every released binary to date
reads and writes the identical schema and a downgrade is safe as far as
sessions are concerned. If a future release does add a schema change, treat
`session.db` as roll-forward only for that specific hop unless its changelog
entry says otherwise; the safe fallback if you must roll back past it is
deleting `session.db` (forces every admin to log in again; does not touch
`config/` or `certs/`).

**Rollback procedure**, once you've decided you need one:

1. Stop the new version.
2. If the upgrade you're undoing crossed one of the config-compatibility
   points above, resolve that first (e.g. don't roll back past the release
   that introduced your first `upstream-groups` reference while still using
   it), `POST /api/revert` or `POST /api/restore` from a
   [pre-upgrade backup](backup-and-restore.md) roll the *config* back
   independently of the binary version, and you may need both together.
3. Start the previous version against the same data directory.
4. Confirm `GET /version`, `GET /healthz`, and that proxied traffic is still
   routing: the same checks as a fresh deploy.

Taking a [`GET /api/backup`](backup-and-restore.md) immediately before an upgrade
that changes anything nontrivial costs one `curl` and turns "roll back" into
a config restore instead of a guess.

## Rollback

Starting an **older** gpm binary against a config directory a **newer** one has
written to. Both config loaders (per-object files and `settings.yaml`) use a
non-strict YAML decode, so a key the older struct does not recognise is
silently dropped rather than rejected, and the first write after that,
including an automatic reconciler commit, makes the drop permanent. See
"Unknown YAML keys are now warned about instead of silently dropped" and
"Rolling back to 1.0.33 or earlier..." in the [changelog](https://github.com/Rake-Pro/go-proxy-manager/blob/main/CHANGELOG.md#unreleased)
for the exact field list this release introduces.

### Prerequisites

- The version you are rolling back to, and its row (if any) in
  [Upgrade notes by version](#upgrade-notes-by-version) above.
- Either a [`GET /api/backup`](backup-and-restore.md) archive taken before you
  started using the newer version, or `git log` access to `/data/config` (the
  config repo, not the gpm source checkout) to recover an earlier commit.

### Steps

1. Stop the newer binary.
2. Check the newer binary's log (or `GET /health`'s `configWarnings` field)
   for `config: <path>: unknown keys ignored: ...` lines before you stop it:
   each one names a file the older version will load incorrectly rather than
   fail on.
3. If any file has unknown keys, recover the config **first**, rather than let
   the older binary start against it:

   ```
   cd /data/config
   git log --oneline
   git checkout <commit-before-the-newer-only-feature> -- .
   git commit -m "Roll back config ahead of downgrade to <version>"
   ```

   Recover only the affected files if you know exactly which ones they are;
   `git checkout <commit> -- <path>` restores one.
4. Start the older binary against the same data directory.
5. Confirm `GET /version`, `GET /healthz`, and that proxied traffic is
   routing: the same checks as a fresh deploy.

### Verify

- `GET /version` reports the version you rolled back to.
- The process log carries no `config: ... unknown keys ignored` warning for a
  file you recovered.
- A host that had an inline `auth:`/`rateLimit:` block, or a `mode: basic`
  middleware, gates traffic the way it did before the rollback, not wide
  open, and the process is not refusing to start.

### Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| A host with an inline `auth:` or `rateLimit:` block is reachable with no login or rate limit after rollback | The older binary's first write to that file silently dropped the unknown block | Recover the file with `git checkout` above, then re-apply the block once back on a version that supports it |
| The older binary refuses to start, or a load error names `auth.mode` | A middleware or inline `auth` block uses `mode: basic` (added in 1.1.0); older versions reject the value outright | Recover the config to before that middleware/block existed, or stay on 1.1.0+ |
| `upstream.path`, `hostHeader`, `stripPrefix`, or a `trustedProxies` list reverted to empty | Same silent-drop, on those fields | Same recovery as above |
| A `rewrite` middleware stopped matching | Same silent-drop, on its prefix/regex rules | Same recovery as above |
| `settings.notifications` or `settings.dockerDiscovery` stopped firing or discovering | Same silent-drop, on `settings.yaml` | Recover `settings.yaml` from git and re-add the section once back on a version that supports it |
