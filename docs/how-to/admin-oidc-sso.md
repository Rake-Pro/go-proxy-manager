# Set up admin single sign-on (OIDC)

Let operators sign in to the admin panel with an OIDC identity provider, with
local login kept as the anti-lockout path.

## Steps

1. Set `externalBaseURL` in `settings.yaml` to the admin panel's public URL; the
   OIDC `redirect_uri` is `<externalBaseURL>/auth/callback`.
2. Create an OIDC application at your IdP with that redirect URI, and an
   `IdentityProvider` object (see [IdentityProvider](../reference/config/identity-provider.md)) with a
   `roleMapping.adminGroups`.
3. List the provider under `settings.adminAuth.providers`. Keep
   `localLoginEnabled: true` while you validate; flip `ssoOnly: true` once an SSO
   login succeeds (recovery from SSO-only is by redeploy).

Only an `oidc` provider renders a sign-in button. A settings write that turns on
`ssoOnly` while `adminAuth.providers` names a provider that does not exist, or
one of type `forward-auth` / `auth-request`, is **refused** - it would leave a
login page with no buttons and no password form.

### No admin login is configured

| Symptom | Cause | Fix |
|---|---|---|
| Login page shows a "No administrator login is configured" banner; startup log has an `error`-level line saying nobody can sign in | No usable local credential (`GPM_LOCAL_ADMIN_USER` plus a bcrypt hash) **and** no `oidc` provider listed in `adminAuth.providers` | Set the local pair (below) **or** finish the OIDC steps above, then restart |
| Login form accepts a password but always answers "authentication failed" | `GPM_LOCAL_ADMIN_USER` is set but the hash is not (or is unreadable) | Generate the hash and mount it as `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE` |

```
docker run --rm ghcr.io/rake-pro/go-proxy-manager hashpw 'your-password' > ./admin_hash
```

Then set `GPM_LOCAL_ADMIN_USER=admin` and
`GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE=/run/secrets/gpm_admin_hash` (the file must
sit under an allowlisted secret root; see `GPM_SECRET_FILE_ROOTS`).

The same condition is reported by `GET /api/capabilities` as
`adminLogin.configured: false`.
