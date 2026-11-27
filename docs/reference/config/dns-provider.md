# DNSProvider

Solves ACME `dns-01` challenges. Not needed for `http-01`.

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="dns-provider-provider"></span>  `provider` | string | none | yes | One of the seven ids in the table below. Anything else is rejected at write time. |
| <span id="dns-provider-config"></span>  `config` | map[string]string | none | yes | Settings and credentials. Which keys are required depends on `provider`. Each key is either a **secret** or a **plain** setting, see below. |

## Secret and plain keys

- **Secret keys** must be a `${ENV:NAME}` or `${FILE:/path}` placeholder. A literal is refused on API and UI writes, and a literal stored by hand in the file reads back as `***`. An unknown key is treated as a secret.
- **Plain keys** may be a literal or a placeholder. They are stored as written and returned by the API as written.
- In every key table below, the **Type** column says `Secret` for a secret key and `string` for a plain key.

| `provider` | Secret keys | Plain keys |
|---|---|---|
| `cloudflare`, `digitalocean`, `hetzner`, `desec` | `apiToken` | none |
| `rfc2136` | `tsigSecret` | `server`, `zone`, `tsigKeyName`, `tsigAlgorithm`, `transport`, `ttl`, `timeout` |
| `acme-dns` | `username`, `password` | `baseURL`, `subdomain`, `allowInsecureLocal` |
| `webhook` | `token` | `url`, `allowInsecureLocal` |

## Provider types

| `provider` | Reaches the zone via | Use it when |
|------------|----------------------|-------------|
| `cloudflare` | Cloudflare API | The zone is hosted at Cloudflare. |
| `digitalocean` | DigitalOcean API | The zone is hosted at DigitalOcean. |
| `hetzner` | Hetzner DNS API | The zone is hosted at Hetzner. |
| `desec` | deSEC API | The zone is hosted at deSEC. |
| `rfc2136` | Dynamic DNS UPDATE + TSIG | You run the nameserver (BIND, Knot, PowerDNS) or it accepts dynamic updates. |
| `acme-dns` | acme-dns HTTP API + a CNAME | The DNS host has no API. The real zone needs one hand-made CNAME and nothing more. See [DNS without an API](../../how-to/dns-without-api.md). |
| `webhook` | Your own HTTPS endpoint | Nothing above fits and you can run a small service that writes the record. See [DNS-01 through your own endpoint](../../how-to/dns-webhook.md). |

## Token providers (`cloudflare`, `digitalocean`, `hetzner`, `desec`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="dns-provider-config-api-token"></span>  `apiToken` | Secret | none | yes | The provider API token. See the credential table below. |

| `provider` | Credential |
|------------|------------|
| `cloudflare` | API token with `Zone:DNS:Edit` + `Zone:Read` on the zone (`Authorization: Bearer`). |
| `digitalocean` | Personal access token with write scope on domains (`Authorization: Bearer`). |
| `hetzner` | Hetzner DNS API token, from the DNS console (`Auth-API-Token` header). Zone and record listings follow `page` / `meta.pagination.last_page`, up to 100 pages; more is an error, never a partial listing. |
| `desec` | deSEC API token (`Authorization: Token`). RRsets are read-modify-written; TTL is 3600, deSEC's minimum. An account with more than 500 domains is listed through deSEC's `Link` cursor pagination, up to 100 pages; more is an error, never a partial listing. |

Behaviour shared by all four:

- **Direct REST, no SDK.** Each solver calls the provider API itself.
- **Longest-suffix zone match.** A delegated `sub.example.com` wins over `example.com`.
- **Adds, never replaces.** An apex + wildcard order sharing `_acme-challenge.example.com` validates.

```yaml
name: cloudflare
provider: cloudflare
config:
  apiToken: ${FILE:/run/secrets/cf_token}   # scope: Zone:DNS:Edit + Zone:Read
```

```yaml
name: hetzner
provider: hetzner
config:
  apiToken: ${ENV:HETZNER_DNS_TOKEN}
```

## `rfc2136` (dynamic update with a TSIG key)

Sends an RFC 2136 dynamic UPDATE signed with a TSIG key (RFC 8945), so any
nameserver that accepts dynamic updates can serve `dns-01`, wildcards included.

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="dns-provider-config-server"></span>  `server` | string | none | yes | Nameserver as `host`, `host:port` or `[v6]:port`. Port defaults to `53`. |
| <span id="dns-provider-config-zone"></span>  `zone` | string | none | yes | Zone the UPDATE is addressed to, e.g. `example.com`. Leave the key out only if you want the solver to auto-detect it with SOA queries; validation requires it, so an auto-detect setup has to be written by hand. **A detected zone is accepted only when it is a suffix of the challenge name**; anything else is refused with the detected zone named. |
| <span id="dns-provider-config-tsig-key-name"></span>  `tsigKeyName` | string | none | yes | Key name exactly as configured on the nameserver, e.g. `gpm-acme`. |
| <span id="dns-provider-config-tsig-secret"></span>  `tsigSecret` | Secret | none | yes | Base64 TSIG secret, as printed by `tsig-keygen`. Use a `${ENV:...}` or `${FILE:...}` placeholder. |
| <span id="dns-provider-config-tsig-algorithm"></span>  `tsigAlgorithm` | string | `hmac-sha256` | no | `hmac-sha1` \| `hmac-sha224` \| `hmac-sha256` \| `hmac-sha384` \| `hmac-sha512`. `hmac-md5` is refused. |
| <span id="dns-provider-config-ttl"></span>  `ttl` | string | `60` | no | TTL of the challenge TXT record, 1 to 86400 seconds. |
| <span id="dns-provider-config-transport"></span>  `transport` | string | `tcp` | no | `tcp` or `udp`. UDP falls back to TCP when the reply is truncated. |
| <span id="dns-provider-config-timeout"></span>  `timeout` | string | `30s` | no | Go duration for dial, write and read on one exchange. |

Behaviour:

- **Present** sends an UPDATE that adds the TXT RR; **CleanUp** sends an UPDATE that deletes that exact RR (class `NONE`), leaving any other value at the same name alone.
- **The reply is not TSIG-verified.** A forged `NOERROR` could only hide a failure the propagation check catches before the CA is asked to validate.
- **Errors name the cause.** `REFUSED` points at the server's update policy, `NOTAUTH` at the key name, secret, algorithm or a skewed clock.

```yaml
name: rfc2136-example
provider: rfc2136
config:
  server: 192.0.2.53                       # host, host:port, or [2001:db8::53]:53
  zone: example.com
  tsigKeyName: gpm-acme
  tsigSecret: ${FILE:/run/secrets/tsig_key}
  tsigAlgorithm: hmac-sha256               # default
  ttl: "60"                                # default
  transport: tcp                           # default; udp also supported
  timeout: 30s                             # default
```

## `acme-dns` (delegate the challenge to a throwaway zone)

Writes the challenge to a [joohoi/acme-dns](https://github.com/joohoi/acme-dns)
account over HTTP. The real zone keeps a single static CNAME and gpm never holds a
credential that can touch it, which is the point.

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="dns-provider-config-base-url"></span>  `baseURL` | string | none | yes | acme-dns API root, e.g. `https://acme-dns.example.com`. Must be `http` or `https`. A loopback, link-local, unspecified or multicast IP literal is refused at write time unless `allowInsecureLocal` is set, and a link-local destination is refused again when gpm connects (even with it set). A placeholder `baseURL` or `allowInsecureLocal` is resolved and checked each time the solver is built, at every attempt, and a bad value fails that attempt. |
| <span id="dns-provider-config-username"></span>  `username` | Secret | none | yes | `username` from the acme-dns `/register` response (a UUID). Secret: a placeholder is required. |
| <span id="dns-provider-config-password"></span>  `password` | Secret | none | yes | `password` from `/register`. Secret: use a `${ENV:...}` or `${FILE:...}` placeholder. |
| <span id="dns-provider-config-subdomain"></span>  `subdomain` | string | none | yes | `subdomain` from `/register` (a UUID); the record the CNAME points at. |
| <span id="dns-provider-config-allow-insecure-local"></span>  `allowInsecureLocal` | string | `""` | no | Plain key. `"true"` (also `yes`, `1`, `on`) lets a loopback `baseURL` pass validation, for an acme-dns running beside gpm; it skips the address-literal checks at validation altogether, so a link-local, unspecified or multicast literal also passes there. A link-local destination is still refused when gpm connects. May be a placeholder, which is resolved when the solver is built; write-time validation reads the value as written, so a placeholder does not lift the write-time loopback check. |

Behaviour:

- **Present** posts `{"subdomain","txt"}` to `POST {baseURL}/update` with `X-Api-User` and `X-Api-Key` headers.
- **CleanUp is a no-op.** An acme-dns account holds a fixed ring of TXT values with no delete endpoint; the next issuance overwrites them.
- **The delegation CNAME is checked, not enforced.** A missing or wrong CNAME logs a warning and issuance continues, because a resolver that cannot see the record yet is not proof the delegation is missing.
- **A rejected credential says so.** A `401` is reported as "credentials rejected", including the `allowfrom` restriction as a likely cause.
- **Redirects are never followed.** The update request carries `X-Api-User` and `X-Api-Key`, so a `3xx` from the acme-dns server is reported as a failed update rather than replayed at whatever it points to.

```yaml
name: acme-dns-example
provider: acme-dns
config:
  baseURL: https://acme-dns.example.com
  username: ${FILE:/run/secrets/acme_dns_username}
  password: ${FILE:/run/secrets/acme_dns_password}
  subdomain: d420c923-0000-4000-8000-000000000002
```

## `webhook` (your own endpoint writes the record)

For each challenge record gpm sends one HTTP request to an endpoint you run, which creates or removes the TXT record in whatever DNS you have.

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="dns-provider-config-url"></span>  `url` | string | none | yes | Endpoint every request is POSTed to. Must be an `http` or `https` URL. **Plain key: a literal is committed to the config repository and visible to any caller that can read DNS providers.** Put the credential in `token`; when the URL itself is sensitive (a secret in its path or query), write the whole value as a placeholder. A loopback, link-local, unspecified or multicast IP literal is refused at write time unless `allowInsecureLocal` is set, and a link-local destination is refused again when gpm connects (even with it set). Error and log text from the solver names the call as `the configured url` and never shows the URL, its path or its query string. A plain `http` URL to a non-loopback host is accepted with a warning in the log, because the token then travels unencrypted. **User info (`user:password@`) is refused**: it is never sent, so put the credential in `token`. A placeholder `url` is not checked at write time: it is resolved and checked each time the solver is built, at every attempt, and a bad value fails that attempt without showing the value (see the errors below). |
| <span id="dns-provider-config-token"></span>  `token` | Secret | none | yes | Sent as `Authorization: Bearer <token>`. Secret: use a `${ENV:...}` or `${FILE:...}` placeholder. |
| `allowInsecureLocal` | string | `""` | no | Plain key, same behaviour (and may be a placeholder) as for [`acme-dns`](#dns-provider-config-allow-insecure-local): `"true"` lets a loopback `url` pass validation and skips the address-literal checks there; a link-local destination is still refused when gpm connects. |

Request, once per TXT record, for both `present` and `cleanup`:

| Part | Value |
|------|-------|
| Method and URL | `POST <url>`, exactly as configured (a path or query string is kept) |
| `Authorization` | `Bearer <token>` |
| `Content-Type` | `application/json` |
| Body | `{"action":"present","fqdn":"_acme-challenge.example.com.","value":"<TXT value>"}` |

- **`action`** is `present` to create the record and `cleanup` to remove it.
- **`fqdn`** always ends in a dot. **`value`** is the TXT value to add or remove.
- **Success** is any `2xx` answer; the response body is ignored. The request also carries `Accept: application/json`.
- **Failure** is anything else: a non-2xx status fails the call with the status and the first 300 characters of the body (`webhook: POST the configured url: 500: ...`), and a transport error fails it with the error.
- **Failure of `present` vs `cleanup`.** A failed `present` fails the order. A failed `cleanup` is logged as a warning (`failed to clean up DNS-01 TXT record`) and does not.
- **Cleanup after a failed `present`.** gpm sends a `cleanup` for every record of the order, including values that were never presented, so `cleanup` of an absent value must answer `2xx`.
- **Timeouts and proxy.** Each request has a 30-second timeout (5 seconds to connect), and no proxy from the environment is used.
- **Redirects are not followed.** A `3xx` answer fails the call (`redirects are not followed because the request carries this provider's credentials`), so the token never reaches another host.
- **Apex plus wildcard.** An order for `example.com` and `*.example.com` presents two different values at the same `fqdn`. The endpoint must add the value, not replace the record set, and on `cleanup` remove only the given value.
- **Propagation.** After `present`, gpm waits until the record is visible in public DNS (the shared 3-minute propagation wait) before asking the CA to validate, so the endpoint may answer before the record has propagated.
- **Validation errors:** `dns provider "<name>": config.url: is required for provider webhook`, the same for `config.token`, and `dns provider "<name>": config.url: must be an http or https URL, got "<value>"`.
- **Write-time errors** for a literal `url` also include `dns provider "<name>": config.url: must not contain user info (user:password@); it is never sent, put the credential in config.token`, and the loopback and link-local refusals shown above.
- **Build-time errors** for a placeholder `url` (or acme-dns `baseURL`) that resolves to something invalid never show the value, and are wrapped as `certificate "<name>": build solver: ...`. With `<field>` being `webhook: url` or `acme-dns: baseURL`:
  - `<field>: is not an http or https URL (the value is not shown, it may hold a credential)`
  - `<field>: points at a loopback address; set config.allowInsecureLocal: "true" if the service really runs beside gpm`
  - `<field>: points at a link-local address, which is the cloud metadata range and never a DNS provider`
  - `<field>: points at an unspecified or multicast address, which is not a destination gpm can send credentials to`
  - `webhook: url: must not contain user info (user:password@); it is never sent, put the credential in token`

```yaml
name: my-dns-hook
provider: webhook
config:
  url: https://dns-hook.example.com/acme
  token: ${FILE:/run/secrets/dns_hook_token}
```
