# Certificates for a domain whose DNS host has no API

Issue a certificate, wildcards included, when your DNS host offers no API: delegate `_acme-challenge` to an acme-dns server with one CNAME and use the `acme-dns` provider.

## Prerequisites

- DNS records for the domain that you can edit by hand, in a web panel, with no API.
- An [acme-dns](https://github.com/joohoi/acme-dns) server you can reach from gpm (self-hosted, or one you trust with the challenge values only: it never holds a credential for your real zone).
- A running gpm instance (see [Quickstart](../getting-started/quickstart.md)).

## Steps

1. Register an account on the acme-dns server and keep the answer: `curl -X POST https://acme-dns.example.com/register` returns `username`, `password`, `subdomain` and `fulldomain`. Register one account per provider object (see the limit in Troubleshooting).
2. In your DNS host's panel, add one permanent CNAME for each distinct base name in the certificate:
   `_acme-challenge.example.com.  CNAME  <subdomain>.acme-dns.example.com.`
   A wildcard `*.example.com` validates at the same name as `example.com`, so it needs no second record. Details and the `dig` check are in [DNS-01 for any nameserver](generic-dns01-rfc2136-acme-dns.md#acme-dns-prerequisites).
3. Store the `username` and `password` as secrets (a literal is refused on API and UI writes) and add the provider (keys: [`acme-dns`](../reference/config/dns-provider.md#acme-dns-delegate-the-challenge-to-a-throwaway-zone)):

   ```yaml
   # config/dns-providers/acme-dns.yaml
   name: acme-dns
   provider: acme-dns
   config:
     baseURL: https://acme-dns.example.com
     username: ${FILE:/run/secrets/acme_dns_username}
     password: ${FILE:/run/secrets/acme_dns_password}
     subdomain: d420c923-0000-4000-8000-000000000002
   ```

4. Add the certificate:

   ```yaml
   # config/certificates/wildcard.yaml
   name: wildcard
   type: acme
   domains: ["*.example.com", example.com]
   acme:
     email: admin@example.com
     dnsProvider: acme-dns
   ```

5. Reference the certificate from a host (`tls.certificateRef: wildcard`).
6. Order it now instead of waiting for the next pass: use **Renew now** in the admin UI, or `POST /api/certificates/wildcard/renew`.


## Verify

| Check | Expected |
|---|---|
| `dig +short CNAME _acme-challenge.example.com` | `<subdomain>.acme-dns.example.com.` |
| `GET /api/certificates/wildcard` after the order has run (at the next pass, or when you use Renew now) | `state` moves from `pending` to `valid` |
| gpm log during the first order | No `acme-dns: challenge name is not delegated to this acme-dns account` warning |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Log warning `acme-dns: challenge name is not delegated to this acme-dns account` | The CNAME is missing, or points at another account's `subdomain`. The check only warns; issuance continues and then fails validation | Fix the CNAME so its first label is your `subdomain` |
| Log warning `acme-dns: could not check the challenge CNAME` | The lookup from gpm failed or the record is not visible yet | Run the `dig` check from the table above; wait for the DNS host to publish it |
| `lastError` says `credentials rejected for subdomain` | Wrong `username` or `password`, or the account is restricted by `allowfrom` | Re-check the values from `/register`; check the acme-dns `allowfrom` list against gpm's address |
| `lastError` mentions DNS propagation timing out | The acme-dns zone is not answering publicly, or the CNAME is not visible to public resolvers | Check that the acme-dns server's nameserver is reachable and delegated |
| Write refused: `refusing to commit literal secret(s): [...]; use ${ENV:...} or ${FILE:...} placeholders` | `username` or `password` was written as a literal | Write both as `${ENV:}` or `${FILE:}` placeholders |
| An order that needs more than two challenge values through one account fails validation | An acme-dns account keeps two TXT values, as the comment on `Present` in `internal/acme/acmedns.go` describes it | Use one account, and one provider object, per base name |
| `lastError` contains `certificate "<name>": build solver: acme-dns: baseURL: is not an http or https URL (the value is not shown, it may hold a credential)` (or `points at a loopback address ...`, `points at a link-local address ...`, `points at an unspecified or multicast address ...`) | A placeholder `baseURL` resolved to an invalid value; the value is deliberately not shown | Check what the placeholder resolves to, and `allowInsecureLocal` for a loopback acme-dns |
| Write refused about `baseURL` | A loopback, link-local, unspecified or multicast literal | Use a routable name, or `allowInsecureLocal: "true"` for loopback only |
| Update fails on a `3xx` | Redirects are not followed, because the request carries the API credentials | Point `baseURL` at the final address |
