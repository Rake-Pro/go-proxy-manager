# Settings: admin authentication

Which identity providers may sign in to the admin panel, and the anti-lockout
rules that keep at least one login method usable.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-admin-auth-providers"></span> `adminAuth.providers` | []string | Identity-provider names allowed to log into the admin panel. Only an `oidc` provider renders a sign-in button; a `forward-auth` or `auth-request` provider gates proxy hosts through an auth middleware and shows nothing here. An `oidc` provider that is not listed cannot start or complete a panel login (`/auth/login?idp=<name>` answers as for an unknown name). |
| <span id="settings-admin-auth-local-login-enabled"></span> `adminAuth.localLoginEnabled` | bool | Keep username/password login available (anti-lockout). Default true. It only grants access when `GPM_LOCAL_ADMIN_USER` and a bcrypt hash are also set: see [Environment variables and flags](../../env-vars-and-flags.md). |
| <span id="settings-admin-auth-sso-only"></span> `adminAuth.ssoOnly` | bool | Disable local login entirely, whatever `localLoginEnabled` says. Recovery from an SSO outage: [Break-glass recovery](#break-glass-recovery). |

**Brute-force lockout on local login.** Five failures from one client IP, or
twenty against one username from any clients, within 15 minutes lock that IP
or that username for the window with the same 429 page; a successful sign-in
clears both counters. Not configurable.

**Anti-lockout rules on `adminAuth`.** A settings write is refused when:

- no login method is left at all (`localLoginEnabled: false` and no `providers`), or
- `ssoOnly: true` and `providers` is empty, or
- `ssoOnly: true` and any `providers` entry does not resolve to an existing
  `IdentityProvider` of type `oidc`.

The third rule is the one that used to bite: with `ssoOnly` there is no password
form, so a typo'd provider name (or one naming a `forward-auth` provider) renders
a login page with **zero** buttons, a total lockout recoverable only by editing
the config repo and redeploying. The error names the provider and why it cannot
be used.

## Break-glass recovery

`ssoOnly: true` overrides `localLoginEnabled`: with both set to `true`, local
login stays off. Setting only `localLoginEnabled: true` during an
identity-provider outage therefore does not open the password form.

1. Edit the live `settings.yaml` in the config directory (`GPM_CONFIG_DIR`,
   default `/data/config`) and set **both** flags:

   ```yaml
   adminAuth:
     ssoOnly: false
     localLoginEnabled: true
   ```

2. Restart gpm. A hand edit of the config directory is read at start, not
   while running.
3. Sign in with `GPM_LOCAL_ADMIN_USER` and the break-glass password, and repair
   the identity provider or the settings that locked you out.
4. Restore the login settings. The config directory is a git repository and
   every save commits the whole working tree:
   - Nothing saved since the edit: discard it with
     `git -C /data/config checkout -- settings.yaml` (for Docker,
     `docker exec <container> git -C /data/config checkout -- settings.yaml`),
     then restart.
   - A change was saved since the edit: the edit was committed with it. Set
     **Require SSO for admins** and **Local sign-in** in Settings back to
     their values before the edit, and check **History** for the commit that
     carried it.
5. Confirm an SSO sign-in works, then rotate the break-glass password: write a
   new hash with `gpm hashpw` (see
   [Environment variables and flags](../../env-vars-and-flags.md#cli-subcommands))
   to the hash file, and restart.
