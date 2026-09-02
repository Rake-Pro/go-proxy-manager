# Backup and restore

Take a config archive on a schedule, and restore one as a single validated
revision.

`GET /api/backup` returns a gzip-compressed tar of the **declarative config
only**: every object YAML under `config/<kind>/` plus `config/settings.yaml`.
It does **not** include `.git` history (that's what `git clone`/`git bundle`
the config repo is for, if you want commit history too), the `certs/`
directory, or `session.db`. It needs the `admin` scope specifically, not
`*:read`: unlike the JSON API, the raw YAML carries the `api-tokens`
directory's stored digests, which are offline-crackable.

Because backup is admin-scoped, there is no narrower "backup:read" token you
can hand to a cron job: an automation credential that can back up the config
can, by the same scope, do everything else `admin` can. Mint one deliberately
for this and nothing else, and treat it with the same care as the break-glass
password:

```
curl -s -X PUT https://<admin>/api/api-tokens/backup-cron \
  -b "<admin session cookie>" -H "X-CSRF-Token: <token from /api/me>" \
  -H 'Content-Type: application/json' \
  -d '{"scopes":["admin"]}' | jq -r .token   # save this, it is shown once
```

**Cron:**

```
# /etc/cron.d/gpm-backup - runs as the gpm user
0 3 * * * gpm  curl -fsS -H "Authorization: Bearer $(cat /etc/gpm/backup_token)" \
  https://127.0.0.1:8081/api/backup \
  -o /var/backups/gpm/gpm-config-$(date +\%Y-\%m-\%dT\%H\%M\%S).tar.gz \
  && find /var/backups/gpm -name 'gpm-config-*.tar.gz' -mtime +30 -delete
```

(A `systemd` timer works the same way if you'd rather not use cron: a
`Type=oneshot` service running the same `curl` line, triggered by an
`OnCalendar=` timer unit.) `%` needs escaping as `\%` in a crontab; a plain
shell script invoked by cron doesn't have that gotcha.

**Restore** replaces the *entire* current config, validates the result, and
commits it as one revision. If the archive doesn't validate (a dangling
reference, a literal un-placeholdered secret, a corrupt/oversized upload) the
working tree is rolled back to the pre-restore commit and **nothing is
committed**: a bad restore attempt is a no-op, not a half-applied one:

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

- **`certs/`**: a `custom`-type `Certificate` references cert/key files by
  relative path in that directory; restoring config that names one does not
  restore the files themselves. Back up `certs/` alongside the config archive
  (a plain file copy, it isn't git-backed) if you use custom certificates.
  `acme`-type certificates need nothing extra: DNS-01 credentials round-trip
  in the restored `DNSProvider`/`Certificate` objects, and gpm re-issues on
  next load if the on-disk artifact is missing.
- **`session.db`**: restoring config never touches it; every admin session
  active at restore time stays valid (or invalid) exactly as it was before.
- **`api-tokens`** digests restore fine (they're plain config), but remember a
  scoped revert of just `api-tokens` is refused for the reason in
  [APIToken](../reference/config/api-token.md): a whole-config
  restore is the one path that *does* touch them, by design, since it's meant
  to bring back an entire prior state including which tokens existed then.

Verify a backup is actually restorable periodically, not just that the cron
job exits `0`: a `GET`/`POST /api/restore` round trip against a disposable
test instance is the only thing that proves the archive is usable, the same
way an untested backup anywhere else is a hope, not a backup.
