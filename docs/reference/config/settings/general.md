# Settings: general

Instance-wide identity keys: the schema version, the brand label and the
canonical admin URL.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-schema-version"></span> `schemaVersion` | int | Config schema version. |
| <span id="settings-app-name"></span> `appName` | string | Brand label in the UI and login page. Default `Go Proxy Manager`. |
| <span id="settings-external-base-url"></span> `externalBaseURL` | string | Canonical public URL of the admin panel. Must be an absolute URL. Used to build the OIDC `redirect_uri` so it never depends on spoofable `X-Forwarded-*` headers. |

> The local admin's password hash and its optional TOTP secret are **env/file
> only** (`GPM_LOCAL_ADMIN_PASSWORD_HASH*`, `GPM_LOCAL_ADMIN_TOTP_SECRET*`);
> they are never config fields, never committed to the git-backed config, and
> are not resolvable through `${ENV:...}`. See
> [Deployment](../../../how-to/totp.md).
