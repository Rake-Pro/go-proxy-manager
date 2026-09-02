# Quickstart

From nothing to a signed-in admin panel and one host serving HTTPS. Budget 15
minutes, plus certificate issuance time.

## Prerequisites

- Docker and Docker Compose.
- A domain you control, e.g. `app.example.com`, with an `A`/`AAAA` record
  pointing at this host.
- For HTTP-01: inbound `tcp/80` and `tcp/443` forwarded to this host.
- For DNS-01: an API token for your DNS provider instead of any inbound port.

## Steps

1. **Generate an admin password hash and write it to a file.** The container
   runs as a non-root user, so the file must be readable by it:

   ```
   docker run --rm ghcr.io/rake-pro/go-proxy-manager hashpw 'your-password' > admin_hash
   chmod 644 admin_hash
   ```

   Without the `chmod`, gpm starts, logs one `warn` line, and serves a login
   page that can never succeed. See
   [Troubleshooting](../operations/troubleshooting.md).

2. **Write a compose file:**

   ```yaml
   # compose.yaml
   services:
     gpm:
       image: ghcr.io/rake-pro/go-proxy-manager
       restart: unless-stopped
       ports: ["80:80", "443:443", "127.0.0.1:8081:8081"]
       environment:
         GPM_LOCAL_ADMIN_USER: admin
         GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE: /run/secrets/admin_hash
       volumes: ["gpm-data:/data"]
       secrets: [admin_hash]
       cap_drop: ["ALL"]
       security_opt: ["no-new-privileges:true"]
   secrets:
     admin_hash:
       file: ./admin_hash
   volumes:
     gpm-data:
   ```

3. **Start it:**

   ```
   docker compose up -d
   ```

4. **Sign in.** Open `http://127.0.0.1:8081/` (tunnel to it for a remote host)
   and log in with the username and password from step 1. This works out of the
   box: `GPM_COOKIE_SECURE` defaults to `auto`, so the session cookie is issued
   without `Secure` over plain HTTP and becomes `Secure` (and `__Host-` named)
   automatically once the panel is served over HTTPS or `externalBaseURL` is an
   `https://` URL.

5. **Add a proxy host.** In the UI: **Proxy Hosts -> New**, domain
   `app.example.com`, upstream scheme `http`, host and port of your backend.
   The equivalent YAML, written to `config/proxy-hosts/app.yaml`:

   ```yaml
   name: app
   domains: [app.example.com]
   upstream: {scheme: http, host: 10.0.0.40, port: 8080}
   ```

   `tls` is optional: omit it and the host is reachable over plain HTTP on
   `:80` with no certificate configured yet.

6. **Add a certificate.** Pick one path.

   **HTTP-01** (no credentials, needs public `:80`):

   ```yaml
   # config/certificates/app.yaml
   name: app
   type: acme
   domains: [app.example.com]
   acme: {email: admin@example.com, challenge: http-01}
   ```

   **DNS-01** (no inbound port, and the only way to get a wildcard):

   ```yaml
   # config/dns-providers/cloudflare.yaml
   name: cloudflare
   provider: cloudflare
   config: {apiToken: ${FILE:/run/secrets/cf_token}}
   ```

   ```yaml
   # config/certificates/wildcard.yaml
   name: wildcard
   type: acme
   domains: ["*.example.com", example.com]
   acme: {email: admin@example.com, dnsProvider: cloudflare}
   ```

7. **Turn on TLS for the host** by adding a `tls` block:

   ```yaml
   tls: {certificateRef: app, forceSSL: true}
   ```

   `certificateRef` records intent; the served certificate is selected by SNI
   across every certificate object. See
   [Which certificate a host serves](../reference/config/certificate.md).

## Verify

| Check | Expected |
|---|---|
| `curl -fsS http://127.0.0.1:8081/healthz` | `ok` |
| Certificates page, or `GET /api/certificates` | `state: valid`, a non-zero `daysRemaining` |
| `curl -sI https://app.example.com/` | `200`/`3xx`, a certificate covering the name |
| `curl -s -o /dev/null -w '%{http_code}\n' http://app.example.com/` | `308` when `forceSSL` is set |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Login always fails, no error in the UI | The hash file is unreadable by the non-root container user | `chmod 644` the secret file and restart |
| Login page says no administrator login is configured | Neither a local credential nor an `oidc` provider is usable | Set `GPM_LOCAL_ADMIN_USER` plus a hash, or finish [admin OIDC](../how-to/admin-oidc-sso.md) |
| Certificate stays `pending` | HTTP-01 cannot reach `:80`, or the DNS-01 credential is wrong | Check `lastError` on the certificate; see [Certificate health](../operations/certificate-health.md) |
| `404` for a domain you configured | No enabled host claims it, or another host claims it first | Check the host is enabled and that no other host lists the same domain |

## Next

- [Your first host with HTTPS](first-host-with-https.md) walks the same ground
  in more detail, including firewall and DNS prerequisites.
- [Which mechanism do I use?](../concepts/which-mechanism.md) once you need
  access control, authentication or path routing.
