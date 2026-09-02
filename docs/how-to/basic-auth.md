# Put a password on a host (basic auth)

Gate a host on local username/password pairs, with no identity provider. The
supported home for this is an auth middleware in `basic` mode; the
`AccessList.basicAuth` field is deprecated: see
[Migrate access-list basic auth](migrate-basic-auth.md).

## Prerequisites

- An admin session, or a token with `middlewares:write` and `proxy-hosts:write`.
- `htpasswd` (from `apache2-utils` / `httpd-tools`) if you want to hash
  passwords yourself; otherwise gpm hashes a plaintext `password` field
  server-side.

## Steps

1. **Generate a bcrypt hash**, or skip this and let gpm do it:

   ```
   htpasswd -nbB admin 'hunter2'
   ```

   Use only the part after the colon.

2. **Create the middleware:**

   ```yaml
   # config/middlewares/internal-basic.yaml
   name: internal-basic
   type: auth
   auth:
     mode: basic
     allowFrom: [10.0.0.0/8]     # the LAN skips the password
     basic:
       realm: Internal
       users:
         - username: admin
           passwordHash: $2a$12$D4G5f18o7aMMfwasBL7GpuQWuP3pkrZrOAnqP.bmezbMng.QwJ/Bu
   ```

   Omit `allowFrom` if every client must supply the password.

3. **Attach it to the host**, or to one location:

   ```yaml
   middlewares: [internal-basic]
   ```

   For a single host, an inline `auth` block with the same `basic` spec works
   the same way and creates no middleware object.

4. **Drop `allowFrom` deliberately, not by accident.** It is an exemption from
   the password, compared against the derived client IP: read
   [Which IP `allowFrom` compares](../concepts/request-pipeline.md#which-ip-allowfrom-compares)
   before relying on it behind another proxy.

## Verify

| Check | Expected |
|---|---|
| `curl -sI https://<host>/` from outside `allowFrom` | `401` with `WWW-Authenticate: Basic realm="Internal"` |
| `curl -sI -u admin:hunter2 https://<host>/` | The app's response |
| Six wrong passwords from one IP | `429`, locked out for 15 minutes |
| `GET /api/middlewares/internal-basic` | `passwordHash` present, no plaintext password |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Write refused: hash is not bcrypt | `passwordHash` is not a 60-character `$2a$`/`$2b$`/`$2y$` string | Regenerate with `htpasswd -nbB`, or POST a plaintext `password` field instead |
| A LAN client is still prompted | The LAN CIDR is not in `allowFrom`, or the compared address is the proxy's | Add the CIDR, and declare any L7 proxy in `settings.trustedProxies` |
| The browser never prompts | Something ahead of gpm strips `WWW-Authenticate`, or the host also strips it | Check `stripResponseHeaders` does not list `WWW-Authenticate` |
| A stream host refuses to validate with this list | Basic auth has no challenge/response on a raw stream | Keep the middleware off stream hosts; use L4 access lists there |
