# Solve DNS-01 through your own endpoint

Let gpm create and remove the `_acme-challenge` TXT record by calling an HTTP endpoint you run, for a DNS setup no built-in provider covers.

## Prerequisites

- A small service you control that can write TXT records into your DNS (a script behind a web server is enough), reachable from gpm.
- An `https` URL for it. A plain `http` URL to a non-loopback host is accepted with a log warning, because the bearer token then travels unencrypted.
- An `admin` session or token to write the DNS provider through the API: every write on `/api/dns-providers` needs the `admin` scope.
- The contract below. Key reference: [`webhook`](../reference/config/dns-provider.md#webhook-your-own-endpoint-writes-the-record).

## Steps

1. Implement the endpoint. For every TXT record gpm sends:

   | Part | Value |
   |---|---|
   | Request | `POST <url>` |
   | Headers | `Authorization: Bearer <token>`, `Content-Type: application/json`, `Accept: application/json` |
   | Body | `{"action":"present","fqdn":"_acme-challenge.example.com.","value":"<TXT value>"}` |

   - `present`: add a TXT record with that `value` at `fqdn` (the name has a trailing dot).
   - `cleanup`: remove the TXT record with that `value` at `fqdn`, and only that one.
   - Answer any `2xx` when done; the body is ignored.
   - A non-2xx answer to `present` fails the order, and the status plus the first 300 characters of the body are in the error.
   - A non-2xx answer to `cleanup` is only logged as a warning (`failed to clean up DNS-01 TXT record`) and does not fail the order.
   - **`cleanup` of an absent value must answer `2xx`.** After a failed `present`, gpm sends a `cleanup` for every record of the order, including values that were never presented.
   - Answer within 30 seconds: that is gpm's per-request timeout (5 seconds to connect). gpm does not use a proxy from the environment, so the endpoint must be reachable directly.
   - **Add, do not replace.** An order for `example.com` and `*.example.com` presents two values at the same name; both must exist at once.
   - Do not redirect: a `3xx` answer fails the call.
2. Try the endpoint by hand with the same request gpm makes:

   ```
   curl -i -X POST https://dns-hook.example.com/acme \
     -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"action":"present","fqdn":"_acme-challenge.example.com.","value":"test-value-1"}'
   ```

   Expect a `2xx` status, then check the record with `dig +short TXT _acme-challenge.example.com`. Repeat with a second value and `"action":"present"`: both must be listed. Then send `"action":"cleanup"` for one value and confirm the other remains.
3. Store the token as a secret and add the provider:

   ```yaml
   # config/dns-providers/my-dns-hook.yaml
   name: my-dns-hook
   provider: webhook
   config:
     url: https://dns-hook.example.com/acme
     token: ${FILE:/run/secrets/dns_hook_token}
   ```

   An endpoint running beside gpm on the same host may use a loopback URL only with `allowInsecureLocal: "true"` (`url: http://127.0.0.1:8081/acme`). `allowInsecureLocal` skips the address-literal checks at write time, but a link-local destination is still refused when gpm connects.
4. Add the certificate:

   ```yaml
   # config/certificates/wildcard.yaml
   name: wildcard
   type: acme
   domains: ["*.example.com", example.com]
   acme:
     email: admin@example.com
     dnsProvider: my-dns-hook
   ```

5. Reference the certificate from a host (`tls.certificateRef: wildcard`).

6. Order it now instead of waiting for the next pass: use **Renew now** in the admin UI, or `POST /api/certificates/wildcard/renew`.
7. Know the timing and safety rules:
   - **Propagation.** After `present`, gpm waits up to 3 minutes for the record to be visible in public DNS before asking the CA to validate, so your endpoint may answer before the record has propagated.
   - **Token.** Use a `${ENV:}` or `${FILE:}` placeholder, never a literal; the token authorizes writes to your DNS. Redirects are never followed, so the token is sent only to the configured host.
   - **URL.** `url` is a plain setting: written literally it is committed to the config repository and visible to anyone who can read DNS providers. Keep credentials out of it, or write the whole `url` as a placeholder. Error and log text names the call as `the configured url` and never shows the URL, its path or its query string.
   - **Placeholders.** A placeholder `url` or `allowInsecureLocal` is resolved and checked every time the solver is built, at each attempt; a bad value fails that attempt.
   - **Scope the endpoint.** Let it write only `_acme-challenge` names.

## Verify

| Check | Expected |
|---|---|
| Your endpoint's log during the first order | One `present` per name on the certificate, then one `cleanup` per value |
| `dig +short TXT _acme-challenge.example.com` during an order | The challenge value(s) |
| `GET /api/certificates/wildcard` after the order has run (at the next pass, or when you use Renew now) | `state` moves from `pending` to `valid` |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Write refused: `refusing to commit literal secret(s): [...]; use ${ENV:...} or ${FILE:...} placeholders` | A secret key such as `token` was written as a literal | Write it as a `${ENV:}` or `${FILE:}` placeholder |
| Write refused: `dns provider "my-dns-hook": config.url: is required for provider webhook` | `url` is missing | Set `config.url` |
| Write refused: `dns provider "my-dns-hook": config.token: is required for provider webhook` | `token` is missing | Set `config.token` to a placeholder |
| Write refused: `config.url: must be an http or https URL, got "<value>"` | Another scheme, or no host | Use an `http` or `https` URL |
| Write refused: `config.url: must not contain user info (user:password@); it is never sent, put the credential in config.token` | The URL carries `user:password@` | Remove it and put the credential in `token` |
| `lastError` contains `certificate "<name>": build solver: webhook: url: is not an http or https URL (the value is not shown, it may hold a credential)` (or `points at a loopback address ...`, `points at a link-local address ...`, `points at an unspecified or multicast address ...`, `must not contain user info ...`) | A placeholder `url` resolved to an invalid value; the value is deliberately not shown | Check what the `${ENV:}` or `${FILE:}` placeholder resolves to, and `allowInsecureLocal` for a loopback endpoint |
| `lastError` contains `the attempt did not finish within 13m0s` | The whole attempt (order, propagation wait and validation) ran past the propagation timeout plus 10 minutes | Check the endpoint's answers and the CA; the backoff applies and the TXT records are cleaned up |
| Write refused: `config.url: points at the loopback address 127.0.0.1; set config.allowInsecureLocal: "true" if the service really runs beside gpm` | The endpoint runs on the same host | Set `allowInsecureLocal: "true"`, or use a routable name |
| Write refused: `points at the link-local address ...` | A link-local address (the cloud metadata range) | Use a routable address. With `allowInsecureLocal` the write passes, but the connection is still refused |
| `lastError` shows `present <name>.: webhook: POST the configured url: 401: ...` (or another non-2xx status) | The endpoint refused the request | Check the token the endpoint expects; the status and body start are in the message |
| `lastError` says `server answered 301 redirecting to another location; redirects are not followed ...` | The URL redirects (for example http to https, or a missing trailing slash) | Configure the final URL exactly |
| `lastError` says `timed out after 3m0s waiting for DNS-01 TXT records to propagate` | The endpoint answered `2xx` but the record is not visible in public DNS | Check that the endpoint really wrote the record at that exact name, and that your DNS host publishes it |
| Order for apex and wildcard fails, a single name works | The endpoint replaced the record set instead of adding the second value | Make `present` add a value and `cleanup` remove only the given one |
