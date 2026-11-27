# Gate a host with SSO

Put an identity provider in front of one proxied host, so the proxy listeners
refuse a request that carries no valid identity. This is separate from
[admin SSO](admin-oidc-sso.md), which governs the gpm panel itself.

## Prerequisites

- An [IdentityProvider](../reference/config/identity-provider.md) object, or the
  details to create one: an OIDC issuer URL and client credentials, the CIDRs of
  a trusted forward-auth proxy, or an outpost URL.
- The host already serves HTTPS with `tls.forceSSL: true`.
- For `oidc` mode, the ability to register `https://<domain>/__gpm/oidc/callback`
  as a redirect URI at the identity provider, once for every domain of the host.
- For `oidc` mode, browsers reach the host on port 443. The redirect URI gpm
  sends never carries a port, so a host published on another HTTPS port cannot
  complete the sign-in.

## Choosing a mode

| Mode | Identity comes from | Use it when |
|---|---|---|
| `oidc` | A browser redirect to the IdP; gpm is the relying party | gpm is the only thing in front of the app and you want a login page |
| `forward-auth` | Identity headers asserted by a trusted proxy | Something ahead of gpm already authenticated the user |
| `auth-request` | A subrequest to an identity outpost | You run an Authentik-style outpost that owns the sign-in flow |
| `client-cert` | The TLS handshake | Devices carry certificates: see [mTLS](mtls-client-certs.md) |
| `basic` | The middleware's own bcrypt hashes | A shared password is enough: see [Basic auth](basic-auth.md) |

Full field rules for every mode are in
[Middleware](../reference/config/middleware.md).

## Steps

1. **Create the identity provider:**

   ```yaml
   # config/identity-providers/authentik.yaml
   name: authentik
   type: oidc
   oidc:
     issuerURL: https://auth.example.com/application/o/gpm/
     clientID: gpm
     clientSecret: ${FILE:/run/secrets/oidc_secret}
   roleMapping:
     adminGroups: [proxy-admins]
     userGroups: [staff]
   ```

   - **Scopes.** gpm requests `openid profile email groups` unless
     [`oidc.scopes`](../reference/config/identity-provider.md#identity-provider-oidc-scopes)
     says otherwise. Set it when the IdP has no `groups` scope.
   - **Groups.** The group list is read from the ID token's `groups` claim
     (`roleMapping.groupsClaim` renames it).

2. **Register the callback URI** at the IdP:
   `https://<domain>/__gpm/oidc/callback`, once per domain of every gated
   host. One OIDC client can carry all of them.
3. **Gate the host.** For a single host, write the block inline:

   ```yaml
   # config/proxy-hosts/grafana.yaml
   auth:
     identityProvider: authentik
     mode: oidc
     requiredRoles: [admin, user]
   ```

   - **`requiredRoles` holds roles, not groups.** The roles are `admin` and
     `user`, matched exactly. The provider's `roleMapping` above turns the
     group `proxy-admins` into `admin` and `staff` into `user`.
   - **List every role that may pass.** `[admin, user]` admits both groups;
     `[user]` alone refuses a member of `proxy-admins`, who holds `admin`.
   - **No `requiredRoles`.** Any user the `roleMapping` gives a role passes. A
     user in none of the mapped groups is refused unless `defaultRole` is set.
   - **No `roleMapping` on the provider.** In `oidc` mode every signed-in
     user passes, and `requiredRoles` must stay empty. In `forward-auth` mode
     nobody passes without a `roleMapping`.

   For a policy shared by several hosts, create a middleware instead and
   reference it from each host's `middlewares`:

   ```yaml
   # config/middlewares/require-sso.yaml
   name: require-sso
   type: auth
   auth:
     identityProvider: authentik
     mode: oidc
     requiredRoles: [admin, user]
   ```

4. **Exempt a trusted network**, if wanted. `allowFrom` is accepted in
   `auth-request`, `client-cert` and `basic` modes only; it is refused in
   `oidc` and `forward-auth` mode, where the gate has no bypass to honour.
5. **Scope it to a path**, if only part of the host needs the gate, by putting
   the same block on a `locations` entry instead of the host. The redirect URI
   stays `https://<domain>/__gpm/oidc/callback`, outside the location, and gpm
   lets the location's gate answer it when all of these hold:
   - **No interactive gate on the host.** A host with its own `oidc` gate
     answers the callback itself, and a location can still add a stricter
     `requiredRoles` for the same provider. A host `auth-request` gate with a
     location `oidc` gate is not supported.
   - **No other location covers the callback path.** A `/` location does;
     put the gate on that location as well, or on the host.
   - **One `oidc` gate for all locations.** Locations of one host that use
     `oidc` reference the same middleware or carry equal blocks. Blocks with
     different `requiredRoles` or providers are two gates, and neither
     sign-in completes.
6. **Know how a session ends.**
   - **One hour.** The `__Host-gpm_sso` session cookie lasts one hour from
     sign-in and is not extended by use. The next request then goes through
     the IdP again, silently while the IdP session is alive.
   - **Per domain.** The cookie belongs to one domain, so each gated domain
     signs in on its own (again silently at the IdP).
   - **No sign-out URL.** gpm has no per-user sign-out path. Signing out at the
     IdP ends the IdP session, not a gpm session that is still running.
   - **Revoke all.** `POST /api/sso/revoke`, or the button under Settings,
     ends every per-host SSO session at once.
   - **Signing key.** The key is created on first use at
     `<cert-dir>/sso_signing.key`. Set `GPM_SSO_SIGNING_KEY` only to supply
     your own, for example the same value on both nodes of an HA pair.

## Verify

| Check | Expected |
|---|---|
| A fresh browser at `https://<host>/some/page?x=1` | Redirected to the IdP, then back to the same path and query, then the app |
| The same request with `curl` and no cookie | `302` to the IdP, not the app's response |
| The request the app receives | `X-Forwarded-User`, `X-Forwarded-Email` and `X-Forwarded-Groups` set by gpm, and no `__Host-gpm_sso` cookie |
| A user outside `requiredRoles` | `403` on the callback, rendered through the host's error page, and no session cookie |
| `GET /api/health` | No entry about `requiredRoles` under `configWarnings` |
| `GET /api/history` | One commit per object you created |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `503` and "authentication not available" | The middleware could not be compiled, usually a dangling `identityProvider` | Fix the reference; the proxy listeners fail closed rather than serving the host open |
| The IdP shows an error about the redirect URI | The redirect URI registered at the IdP does not match `https://<domain>/__gpm/oidc/callback` exactly, or is missing for this domain | Register the exact URI for every domain of the host |
| The IdP answers `invalid_scope` | gpm asks for the `groups` scope by default and the IdP does not define it | Set `oidc.scopes` on the identity provider to the scopes the IdP offers |
| Everyone is denied with `403` | `requiredRoles` lists a group name; only `admin` and `user` are roles | Use the role the group maps to. The Overview shows a config warning naming the entry |
| Everyone is denied with `403` | `requiredRoles` is set and the provider has no `roleMapping` | Add a `roleMapping`, or remove `requiredRoles` |
| Everyone is denied with `403` | The groups claim is not in the ID token | gpm reads claims from the ID token; configure the IdP to include the groups claim there |
| One user is denied with `403` | The user is in none of the mapped groups and `roleMapping.defaultRole` is empty | Add the user to a mapped group, or set `defaultRole: user` |
| A page stops loading data after about an hour | The session expired; a background request is redirected to the IdP and the browser blocks that cross-origin answer | Reload the page; the sign-in is silent while the IdP session is alive |
| Sign-in on a gated location ends on an application page for `/__gpm/oidc/callback` | The callback did not reach the gate: another location covers that path, the host carries an `auth-request` gate, or two locations use different `oidc` gates | See step 5 |
| A write is refused: `allowFrom` not permitted | `allowFrom` in `oidc` or `forward-auth` mode, including via an unset `mode` inheriting the provider type | Set `mode` explicitly, or move the network rule into an [access list](../reference/config/access-list.md) |
| Sessions end after an hour | The per-host SSO cookie has a 1-hour absolute TTL, not a sliding window | Expected. Re-auth is silent while the IdP session is valid |
