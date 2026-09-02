# DNS-01 for any nameserver: rfc2136 and acme-dns

Solve ACME `dns-01` on a nameserver with no gpm-native API client: a
TSIG-signed dynamic update, or a delegated acme-dns zone.

## rfc2136: prerequisites

1. Generate a key on the nameserver host: `tsig-keygen -a hmac-sha256 gpm-acme` (BIND 9.11+), or `ddns-confgen -a hmac-sha256 -k gpm-acme` on older BIND. Knot and PowerDNS accept the same key material.
2. Add the generated `key` block to the nameserver config and grant it update rights on the zone. For BIND, in the `zone` clause:
   `update-policy { grant gpm-acme name _acme-challenge.example.com. TXT; };`
3. Reload the nameserver (`rndc reload`), and make sure `tcp/53` (or `udp/53`) is reachable from gpm.
4. Put the base64 secret where gpm can read it, e.g. `/run/secrets/tsig_key` or a `GPM_TSIG_SECRET` env var. Never commit it literally.
5. Keep the clock in sync. TSIG rejects a signature more than 300 seconds off.
6. Decide the zone. `config.zone` is **required**: a provider written without it
   is refused at write time, naming the missing key. Set it to the zone apex the
   UPDATE is addressed to (`example.com`), not the challenge name. The solver
   also carries an SOA-based zone detector, and it accepts a detected zone only
   when that zone is a suffix of the challenge name - but that path is not
   reachable through the API or the UI today, because validation requires the
   key.

## acme-dns: prerequisites

1. Register an account against your acme-dns server, once per certificate:
   `curl -X POST https://acme-dns.example.com/register`
2. Keep the returned `username`, `password`, `subdomain` and `fulldomain`.
3. Add one permanent CNAME in the real zone, for every domain on the certificate:
   `_acme-challenge.example.com. CNAME <subdomain>.acme-dns.example.com.`
   A wildcard `*.example.com` validates at the same `_acme-challenge.example.com`, so it needs no second record.
4. Verify the delegation resolves: `dig +short CNAME _acme-challenge.example.com`
5. Store the password as a secret placeholder; the account is worthless without the CNAME, but it is still a credential.

For a local acme-dns, `baseURL` may name a loopback address only when
`allowInsecureLocal` is set:

```yaml
name: acme-dns-local
provider: acme-dns
config:
  baseURL: http://127.0.0.1:8053
  allowInsecureLocal: "true"     # required for a loopback baseURL
  username: c0f8ba55-0000-4000-8000-000000000001
  password: ${FILE:/run/secrets/acme_dns_password}
  subdomain: d420c923-0000-4000-8000-000000000002
```

Link-local, unspecified and multicast literals are refused whatever this is set
to, and a `3xx` from the acme-dns server is reported as a failed update rather
than followed - the request carries `X-Api-User` and `X-Api-Key`.

## Verify

| Check | Expected |
|---|---|
| `GET /api/certificates/<name>` | `state` moves off `pending` to `valid` after the first order |
| `dig +short TXT _acme-challenge.example.com` during an order | The challenge value, from the nameserver or the acme-dns zone |
| `dig +short CNAME _acme-challenge.example.com` (acme-dns) | `<subdomain>.acme-dns.example.com.` |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Write refused: `config.zone is required` | `zone` is mandatory for the `rfc2136` provider; there is no optional-zone mode today | Set `config.zone` to the zone apex, e.g. `example.com` |
| `SOA lookup for ... answered with zone ..., which is not a suffix of that name` | Something answering for the nameserver returned an SOA for an unrelated zone | Set `config.zone` explicitly, and check the `server` address and transport (`tcp` is the safer choice) |
| `REFUSED` from the nameserver | The server's update policy does not grant this key the `_acme-challenge` name | Widen `update-policy` for that name only, and reload the nameserver |
| `NOTAUTH` from the nameserver | Wrong key name, secret or algorithm, or a clock more than 300 seconds off | Re-check `tsigKeyName`/`tsigSecret`/`tsigAlgorithm` and fix NTP on both hosts |
| Write refused: `baseURL` host is not permitted | The acme-dns host is a loopback, link-local, unspecified or multicast literal | Use a routable name, or set `allowInsecureLocal: "true"` for loopback only |
| acme-dns update fails on a `3xx` | Redirects are never followed, because the request carries the API credentials | Point `baseURL` at the final address the acme-dns server actually serves |
