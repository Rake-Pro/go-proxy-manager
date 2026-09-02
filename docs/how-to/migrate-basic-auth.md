# Migrate access-list basic auth to a middleware

Convert a deprecated `AccessList.basicAuth` block into an auth middleware in
`basic` mode, in a single commit.

Converts one list in a single commit: creates the middleware, attaches it
wherever the list is referenced, and clears the deprecated fields.

## What the migration writes

- **The middleware.** A `type: auth`, `mode: basic` object named `<list>-basic`,
  carrying the same usernames and bcrypt hashes.
- **The attachment.** Every proxy host and location that referenced the list
  gains the middleware, listed in the plan's `attachTo`.
- **The exemption.** A `satisfyAny` list's allow rules become the middleware's
  `allowFrom`: the networks that previously satisfied the list *instead of* a
  password become the networks exempt from it.
- **The detach, when the swap is complete.** If the list set `satisfyAny` **and**
  every rule moved into `allowFrom`, the access list is also **detached** from
  the hosts and locations that gained the middleware (`detachAccessList: true` in
  the plan). Leaving it attached would turn the old "IP match OR password" into
  "IP match AND password" and refuse every password user outside the allow rules.
- **Nothing detached otherwise.** A list that still carries a deny rule, a
  source-backed rule or geo rules stays attached, and the plan carries an
  explicit warning that its remaining rules become mandatory alongside the
  password.

Run `?plan=1` and read `detachAccessList` and `warnings` before applying.

## Prerequisites

- An access list that still has `basicAuth` users.
- A caller with the `admin` scope (one call rewrites access lists, middlewares
  and proxy hosts together).
- No existing middleware named `<list>-basic`.

## Steps

1. Preview the change:

   ```
   curl -sS -X POST "https://gpm.example.com/api/access-lists/legacy/migrate-basic-auth?plan=1"
   ```

2. Read the `attachTo` list (every host and location that gains the middleware),
   the `allowFrom` list, `detachAccessList`, and any `warnings`.
3. Apply it:

   ```
   curl -sS -X POST https://gpm.example.com/api/access-lists/legacy/migrate-basic-auth
   ```

## Verify

- `GET /api/middlewares/legacy-basic` returns `type: auth`, `mode: basic` and
  the same usernames.
- `GET /api/access-lists/legacy` shows no `basicAuth` and no `satisfyAny`; the
  `rules` are untouched.
- When the plan reported `detachAccessList: true`, the hosts in `attachTo` no
  longer reference the list: the middleware is the only gate, and a password
  user outside the old allow rules can still sign in.
- The load-time `WARN` no longer names the list after the next reload.
- An unauthenticated request to a gated host still gets `401` with a
  `WWW-Authenticate: Basic realm="legacy"` challenge.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `409`, "middleware ... already exists" | Something already owns the derived name. | Rename or delete that middleware, or migrate by hand. |
| `400`, "has no basicAuth users to migrate" | Already migrated, or the list never had users. | Nothing to do. |
| `warnings` names a `source`- or `paths`-scoped allow rule | An auth exemption is a fixed, host-wide CIDR list; those rules are neither. | Leave the access list attached so the rule keeps working, or write the exemption explicitly in `allowFrom`. |
| A LAN client is now prompted for a password | The list did **not** set `satisfyAny`, so it required *both* the IP and the password, and no `allowFrom` was copied. | If the LAN really should skip the password, add the CIDR to the middleware's `allowFrom` deliberately. |
| Every password user off the allow-listed networks is refused after migrating | The access list stayed attached, so "IP match OR password" became "IP match AND password" | Check `detachAccessList` in the plan; a list with a deny, `source` or geo rule is kept attached on purpose: detach it by hand, or keep the remaining rules as a deliberate second gate. |
