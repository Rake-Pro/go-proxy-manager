# Settings: admin authentication

Which identity providers may sign in to the admin panel, and the anti-lockout
rules that keep at least one login method usable.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-admin-auth-providers"></span> `adminAuth.providers` | []string | Identity-provider names allowed to log into the admin panel. Only an `oidc` provider renders a sign-in button; a `forward-auth` or `auth-request` provider gates proxy hosts through an auth middleware and shows nothing here. |
| <span id="settings-admin-auth-local-login-enabled"></span> `adminAuth.localLoginEnabled` | bool | Keep username/password login available (anti-lockout). Default true. It only grants access when `GPM_LOCAL_ADMIN_USER` and a bcrypt hash are also set: see [Environment variables and flags](../../env-vars-and-flags.md). |
| <span id="settings-admin-auth-sso-only"></span> `adminAuth.ssoOnly` | bool | Disable local login entirely. Recovery from an SSO outage is by redeploying with local login re-enabled. |

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
