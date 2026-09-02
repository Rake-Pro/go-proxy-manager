# IdentityProvider

| Field | Type | Notes |
|-------|------|-------|
| <span id="identity-provider-type"></span>  `type` | string | `oidc` \| `forward-auth` \| `auth-request`. |
| <span id="identity-provider-oidc"></span>  `oidc` | OIDCSpec | Required when `type: oidc`; an error otherwise. |
| <span id="identity-provider-forward-auth"></span>  `forwardAuth` | ForwardAuthSpec | Required when `type: forward-auth`; an error otherwise. |
| <span id="identity-provider-auth-request"></span>  `authRequest` | AuthRequestSpec | Required when `type: auth-request`; an error otherwise. |
| <span id="identity-provider-role-mapping"></span>  `roleMapping` | RoleMapping | Map IdP groups -> roles. |

## OIDCSpec (`oidc`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="identity-provider-oidc-issuer-url"></span>  `issuerURL` | string | none | yes | The IdP's issuer URL, used for OIDC discovery. |
| <span id="identity-provider-oidc-client-id"></span>  `clientID` | string | none | yes | Client id of the application registered at the IdP. |
| <span id="identity-provider-oidc-client-secret"></span>  `clientSecret` | Secret | none | no | Client secret, as a `${ENV:...}` / `${FILE:...}` placeholder. Omit it for a public client relying on PKCE alone. |
| <span id="identity-provider-oidc-scopes"></span>  `scopes` | []string | `openid profile email groups` | no | Scopes requested in the authorization request. |
| <span id="identity-provider-oidc-use-pkce"></span>  `usePKCE` | bool (nullable) | `true` | no | Send a PKCE code challenge. Omitted means on; set `false` only for an IdP that rejects it. |
| <span id="identity-provider-oidc-require-verified-email"></span>  `requireVerifiedEmail` | bool | `false` | no | Refuse a login whose ID token does not carry `email_verified: true`. |

## ForwardAuthSpec (`forwardAuth`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="identity-provider-forward-auth-trusted-proxies"></span>  `trustedProxies` | []string | none | yes | CIDRs whose identity headers are believed. This is the **identity** tier only: see the note below. |
| <span id="identity-provider-forward-auth-user-header"></span>  `userHeader` | string | none | yes | Header carrying the username, e.g. `X-authentik-username`. |
| <span id="identity-provider-forward-auth-email-header"></span>  `emailHeader` | string | none | no | Header carrying the email address. |
| <span id="identity-provider-forward-auth-name-header"></span>  `nameHeader` | string | none | no | Header carrying the display name. |
| <span id="identity-provider-forward-auth-groups-header"></span>  `groupsHeader` | string | none | no | Header carrying the group list that `roleMapping` reads. |
| <span id="identity-provider-forward-auth-groups-delimiter"></span>  `groupsDelimiter` | string | `,` | no | Separator splitting `groupsHeader` into group names. |
| <span id="identity-provider-forward-auth-amr-header"></span>  `amrHeader` | string | none | no | Header carrying the authentication-methods reference, recorded but not currently gated on. |

> `forwardAuth.trustedProxies` is the **identity** tier and nothing else: it says
> which peers may assert `Remote-User` and friends. It does **not** decide the
> client IP: `settings.trustedProxies` (or a proxy host's own `trustedProxies`)
> does. See [Client IP and the three trust
> tiers](../../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers). A config that sets this field but
> leaves `settings.trustedProxies` empty logs a warning at load with the exact
> YAML block to add.

## AuthRequestSpec (`authRequest`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="identity-provider-auth-request-outpost-url"></span>  `outpostURL` | string | none | yes | Base URL of the identity outpost gpm subrequests and proxies the sign-in flow to. |
| <span id="identity-provider-auth-request-path-prefix"></span>  `pathPrefix` | string | `/outpost.goauthentik.io` | no | Path prefix on the gated host that gpm proxies verbatim to the outpost (sign-in, callback, sign-out). |
| <span id="identity-provider-auth-request-auth-path"></span>  `authPath` | string | `<pathPrefix>/auth/nginx` | no | The subrequest endpoint that answers allow or deny. |
| <span id="identity-provider-auth-request-copy-headers"></span>  `copyHeaders` | []string | the Authentik `X-authentik-*` set | no | Response headers copied from the auth subrequest onto the upstream request. |

## RoleMapping (`roleMapping`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="identity-provider-role-mapping-groups-claim"></span>  `groupsClaim` | string | `groups` | no | ID-token claim (or header field) holding the group list. |
| <span id="identity-provider-role-mapping-admin-groups"></span>  `adminGroups` | []string | none | no | Groups mapped to the `admin` role. |
| <span id="identity-provider-role-mapping-user-groups"></span>  `userGroups` | []string | none | no | Groups mapped to the read-only `user` role. |
| <span id="identity-provider-role-mapping-default-role"></span>  `defaultRole` | string | `""` (deny) | no | Role for an authenticated user in none of the groups above: `""` denies, `user` grants read-only, `admin` grants everything. |
| <span id="identity-provider-role-mapping-allow-default-admin"></span>  `allowDefaultAdmin` | bool | `false` | required with `defaultRole: admin` | Must be `true` when `defaultRole` is `admin`. It exists to stop a config silently granting admin to every authenticated user when no group gating is configured. |

```yaml
name: authentik-oidc
type: oidc
oidc:
  issuerURL: https://auth.example.com/application/o/gpm/
  clientID: gpm
  clientSecret: ${FILE:/run/secrets/oidc_secret}
roleMapping:
  adminGroups: [proxy-admins]      # no defaultRole -> anyone not in the group is denied
```

> The OIDC client reads claims from the **ID token**, so if your provider only
> emits groups via the userinfo endpoint you must configure it to include the
> groups claim in the ID token.

> **SSO session lifetime / offboarding.** For `type: oidc` hosts, gpm mints a
> signed `__Host-gpm_sso` session cookie with a **1-hour absolute TTL** (not a sliding
> window, it is not extended by activity). On expiry the next request re-runs the
> OIDC flow against the IdP, which is silent when the IdP session is still valid
> and re-checks group membership. This bounds the offboarding window: a user
> removed from a group or disabled at the IdP loses data-plane access within an
> hour, without gpm holding server-side session state. There is no per-user
> revocation, but there is a global one: `POST /api/sso/revoke` (admin-gated;
> also a button under Settings) moves a signed revocation watermark to "now",
> invalidating every outstanding SSO session on this instance immediately:
> users re-authenticate at the IdP on their next request. The watermark
> persists next to the signing key, so it survives restarts. Scope note: the
> watermark is read at startup, so a *second* gpm instance sharing the same
> signing key only picks a revocation up on its next restart. For a
> single-user cutoff, revoke at the IdP; access ends at that user's next
> hourly re-auth.

## Deprecated fields

| Field | Status | Reason |
|---|---|---|
| <span id="identity-provider-oidc-trust-id-pmfa"></span>  `oidc.trustIdPMFA` | Deprecated, ignored | It was meant to suppress a second local MFA prompt by trusting the IdP's `acr`/`amr`. gpm has no local TOTP prompt to suppress and never reads `acr`/`amr`, so the flag decides nothing. Still parsed; see `FEATURES.md` for the roadmap item that would give it meaning. |

---
