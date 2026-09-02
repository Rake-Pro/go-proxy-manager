# Gate a host with SSO

Put an identity provider in front of one proxied host, so the data plane
refuses a request that carries no valid identity. This is separate from
[admin SSO](admin-oidc-sso.md), which governs the gpm panel itself.

## Prerequisites

- An [IdentityProvider](../reference/config/identity-provider.md) object, or the
  details to create one: an OIDC issuer URL and client credentials, the CIDRs of
  a trusted forward-auth proxy, or an outpost URL.
- The host already serves HTTPS with `tls.forceSSL: true`.
- For `oidc` mode, the ability to register `https://<host>/__gpm/oidc/callback`
  as a redirect URI at the identity provider.

## Choosing a mode

| Mode | Identity comes from | Use it when |
|---|---|---|
| `oidc` | A browser redirect to the IdP; gpm is the relying party | gpm is the only thing in front of the app and you want a login page |
| `forward-auth` | Identity headers asserted by a trusted proxy | Something ahead of gpm already authenticated the user |
| `auth-request` | A subrequest to an identity outpost | You run an Authentik-style outpost that owns the sign-in flow |
| `client-cert` | The TLS handshake | Devices carry certificates - see [mTLS](mtls-client-certs.md) |
| `basic` | The middleware's own bcrypt hashes | A shared password is enough - see [Basic auth](basic-auth.md) |

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

2. **Register the callback URI** at the IdP:
   `https://<host>/__gpm/oidc/callback`, once per gated host.
3. **Gate the host.** For a single host, write the block inline:

   ```yaml
   # config/proxy-hosts/grafana.yaml
   auth:
     identityProvider: authentik
     mode: oidc
     requiredRoles: [staff]
   ```

   For a policy shared by several hosts, create a middleware instead and
   reference it from each host's `middlewares`:

   ```yaml
   # config/middlewares/require-sso.yaml
   name: require-sso
   type: auth
   auth:
     identityProvider: authentik
     mode: oidc
     requiredRoles: [staff]
   ```

4. **Exempt a trusted network**, if wanted. `allowFrom` is accepted in
   `auth-request`, `client-cert` and `basic` modes only; it is refused in
   `oidc` and `forward-auth` mode, where the gate has no bypass to honour.
5. **Scope it to a path**, if only part of the host needs the gate, by putting
   the same block on a `locations` entry instead of the host.

## Verify

| Check | Expected |
|---|---|
| A fresh browser at `https://<host>/` | Redirected to the IdP, then back, then the app |
| The same request with `curl` and no cookie | `302` to the IdP, not the app's response |
| A user outside `requiredRoles` | `403`, rendered through the host's error page |
| `GET /api/history` | One commit per object you created |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `503` and "authentication not available" | The middleware could not be compiled, usually a dangling `identityProvider` | Fix the reference; the data plane fails closed rather than serving the host open |
| Redirect loop at the IdP | The redirect URI registered at the IdP does not match `https://<host>/__gpm/oidc/callback` exactly | Register the exact URI for that host |
| Everyone is denied with `403` | The groups claim is not in the ID token | gpm reads claims from the ID token; configure the IdP to include the groups claim there |
| A write is refused: `allowFrom` not permitted | `allowFrom` in `oidc` or `forward-auth` mode, including via an unset `mode` inheriting the provider type | Set `mode` explicitly, or move the network rule into an [access list](../reference/config/access-list.md) |
| Sessions end after an hour | The data-plane SSO cookie has a 1-hour absolute TTL, not a sliding window | Expected. Re-auth is silent while the IdP session is valid |
