# Issue a wildcard certificate over DNS-01 (Cloudflare)

Get a wildcard certificate with no inbound port open, using a Cloudflare API
token scoped to one zone.

## Prerequisites

- A domain whose DNS is hosted on Cloudflare.
- A Cloudflare API token scoped to **Zone:DNS:Edit + Zone:Read** on the target
  zone (a token, not the account's global API key).
- A running gpm instance (see [Quickstart](../getting-started/quickstart.md)).

## Steps

1. Create the Cloudflare API token and mount it as a secret
   (`./secrets/cf_token`, owner the image's `gpm` user, mode `0400`, token
   pasted at a prompt rather than typed on a command line: see
   [Secret files](../getting-started/install-docker.md#secret-files)).
2. Add a DNS provider and a certificate to the config:

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

3. Reference the certificate from a host (`tls.certificateRef: wildcard`).

The manager orders the certificate at its next pass, not when you save it: that is the next regular 12-hour pass or a restart. To order at once, use **Renew now** in the admin UI or `POST /api/certificates/wildcard/renew`. It then renews automatically when `min(30 days, lifetime / 3)` of the certificate's life remains (30 days for a 90-day certificate, see [Renewal](../reference/config/certificate.md#renewal)).
DNS-01 needs no inbound port: it works from behind a firewall. **Tip:** validate
against the Let's Encrypt **staging** directory first by setting
`acme.directoryURL: https://acme-staging-v02.api.letsencrypt.org/directory` on a
throwaway hostname; a staging cert is untrusted, so don't point it at a domain
serving real traffic. Switch to production (omit `directoryURL`) once it issues.

## Verify

| Check | Expected |
|---|---|
| `GET /api/certificates` | `wildcard` shows `state: valid`, `issuer` set |
| `curl -sI https://anything.example.com/` (any subdomain) | A certificate covering `*.example.com` |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `lastError` mentions "no zone found" | The token cannot see the zone | Recheck the token's zone scope; it must include the exact zone (`example.com`), not just a subdomain |
| `lastError` reports a Cloudflare API failure | Wrong token, revoked token, or missing `Zone:DNS:Edit` permission | Regenerate the token with both **Zone:DNS:Edit** and **Zone:Read** on the target zone |
| Certificate stuck at `state: pending` | The TXT record has not propagated yet, or the token cannot write it | Wait for the next pass or use Renew now; confirm the token's permissions match above |
