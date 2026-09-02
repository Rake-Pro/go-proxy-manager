# Security model

Who can reach the admin plane, what a role or a token may do, and what gpm
deliberately never stores.

This is a deliberate stance, not a gap: gpm has **no local user table** and no
per-user permission system. Two things authenticate to the admin panel:

- **One local break-glass admin.** A single username/password pair
  (`GPM_LOCAL_ADMIN_USER` + a bcrypt hash: see
  [Environment variables, flags and CLI](../reference/env-vars-and-flags.md#flags-and-environment-variables)), always
  role `admin`, always IdP `"local"`. There is exactly one of these; it is not
  a config object, cannot be listed, and does not appear in git history. Its
  purpose is recovery when SSO is unreachable, not day-to-day login: see
  `adminAuth.ssoOnly` above. It can require a TOTP code after the password,
  see [Enable TOTP for the local admin](../how-to/totp.md).
- **OIDC group-to-role mapping.** Every other admin-panel login is an
  [IdentityProvider](../reference/config/identity-provider.md) with a
  `roleMapping`: the IdP's group claim resolves to exactly one of two local
  roles, `admin` or `user` (`RoleNone`, no access, otherwise). **Individual
  identity is not gpm's to manage**: who is in `adminGroups`/`userGroups` is a
  decision made and audited at the IdP, the same way it already has to be for
  every other application behind SSO.

## Roles

There are exactly two roles, and no per-object permission grant.

| Role | Can do | Cannot do |
|---|---|---|
| `admin` | Everything: every read, every write, settings, backup/restore, revert, API tokens, pprof and metrics | none |
| `user` | Every read: `GET /api/me`, `GET /api/capabilities`, and every `GET` under `/api/` (hosts, certificates, access lists, settings, config, history, logs, discovery status and plans) | Any `POST`/`PUT`/`DELETE` (403); **anything under `/api/api-tokens`**, in either direction; `GET /api/backup`, `/metrics` and `/debug/pprof/`, which need the `admin` scope |

Internally the `user` role is the scope grant `*:read` **minus the `api-tokens`
subject**, so it is the same mechanism the [API-token scopes](../reference/config/api-token.md#scopes) use. The
two gates compose: a token presented on a `user` session can never grant more
than that, whatever scopes the token itself holds.

**Why API tokens are excluded.** A token is a credential, not configuration:
listing them hands a viewer the name, scopes, expiry and stored digest of every
automation credential on the instance, which is nothing a viewer needs in order
to view the proxy configuration. `GET /api/config` is narrowed the same way:
a caller without `api-tokens:read` gets the whole tree with `apiTokens` omitted,
rather than the rows it was just refused at `/api/api-tokens`.

`GET /api/me` reports `"readOnly": true` for a `user` session; the admin UI
reads that flag and renders itself read-only (banner, Save controls disabled)
rather than offering controls whose every submission would answer 403.

**Assigning the `user` role.** Put the group in `roleMapping.userGroups` on the
IdentityProvider, or set `roleMapping.defaultRole: user` to make every
authenticated user a viewer by default:

```yaml
name: authentik-oidc
type: oidc
roleMapping:
  adminGroups: [proxy-admins]
  userGroups: [proxy-viewers]
  # or, to make everyone who can authenticate a read-only viewer:
  # defaultRole: user
```

The local admin is always `admin`; the `user` role is reachable through SSO
only.

**Not planned: multi-user local accounts, or per-user permissions finer than
the two roles above.** Local accounts don't scale past the one break-glass
credential they exist for: a second local user would need its own storage,
its own rotation story, and its own audit trail, all of which OIDC already
provides for free once an IdP is configured. If you need more than one named
human with admin access, put an IdentityProvider in front of gpm; if you need
per-person restriction to a subset of hosts or actions, that is out of scope
for the same reason the two-role model is: gpm's authorization boundary is
role (admin = read/write, user = read-only) plus, for automation,
[API-token scope](../reference/config/api-token.md#scopes), not identity.

**Delegation is API tokens, not accounts.** A script, CI pipeline, or
integration that needs its own credential (distinct from "is logged in as
the admin") gets a scoped [APIToken](../reference/config/api-token.md), not a
second local login. Tokens are named, individually revocable, expirable, and
restricted to exactly the resources they touch (`proxy-hosts:write` and
nothing else, for example). Sharing one local admin account is never the
delegation mechanism.

**Audit is git history plus webhooks, not an audit-log table.** Every write
through the API or UI (create, update, delete, settings, restore, revert)
is one commit to the config repo (`config/`), carrying whatever
[`Author`](architecture.md) the caller resolved to: the local admin's
username, the SSO subject/email from the session, or the token's own
`name` (its principal has no email, so the commit's email falls back to
`gpm@localhost`) for a token-authenticated write. `git log`
and `GET /api/history` / `GET /api/{plural}/{name}/history` are the audit
trail: who changed what, when, and (via `git show`) exactly what the diff
was, and it is tamper-evident by construction: rewriting it means rewriting
git history, not flipping a row in a database. `webhooks` above add a
real-time feed of the same events to an external system (SIEM, chat,
ticketing) if you want change notifications outside `git log`.
**What this does *not* cover:** authentication events themselves (successful
and failed logins) are logged to gpm's structured process log
(`GPM_LOG_LEVEL`/`GPM_LOG_CONSOLE`), not to git: a failed login attempt
changes nothing in config, so there is no commit for it. Ship the process log
to your log aggregator if you need a durable record of login attempts
alongside the config-change history.
