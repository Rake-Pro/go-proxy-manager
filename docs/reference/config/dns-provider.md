# DNSProvider

Solves ACME `dns-01` challenges. Not needed for `http-01`.

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="dns-provider-provider"></span>  `provider` | string | - | yes | One of the six ids in the table below. Anything else is rejected at write time. |
| <span id="dns-provider-config"></span>  `config` | map[string]Secret | - | yes | Credential map, secret-valued. Which keys are required depends on `provider`. |

## Provider types

| `provider` | Reaches the zone via | Use it when |
|------------|----------------------|-------------|
| `cloudflare` | Cloudflare API | The zone is hosted at Cloudflare. |
| `digitalocean` | DigitalOcean API | The zone is hosted at DigitalOcean. |
| `hetzner` | Hetzner DNS API | The zone is hosted at Hetzner. |
| `desec` | deSEC API | The zone is hosted at deSEC. |
| `rfc2136` | Dynamic DNS UPDATE + TSIG | You run the nameserver (BIND, Knot, PowerDNS) or it accepts dynamic updates. |
| `acme-dns` | acme-dns HTTP API + a CNAME | Everything else. The real zone needs one hand-made CNAME and nothing more. |

## Token providers (`cloudflare`, `digitalocean`, `hetzner`, `desec`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="dns-provider-config-api-token"></span>  `apiToken` | Secret | - | yes | The provider API token. See the credential table below. |

| `provider` | Credential |
|------------|------------|
| `cloudflare` | API token with `Zone:DNS:Edit` + `Zone:Read` on the zone (`Authorization: Bearer`). |
| `digitalocean` | Personal access token with write scope on domains (`Authorization: Bearer`). |
| `hetzner` | Hetzner DNS API token, from the DNS console (`Auth-API-Token` header). |
| `desec` | deSEC API token (`Authorization: Token`). RRsets are read-modify-written; TTL is 3600, deSEC's minimum. |

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
| <span id="dns-provider-config-server"></span>  `server` | Secret | - | yes | Nameserver as `host`, `host:port` or `[v6]:port`. Port defaults to `53`. |
| <span id="dns-provider-config-zone"></span>  `zone` | Secret | - | yes | Zone the UPDATE is addressed to, e.g. `example.com`. Leave the key out only if you want the solver to auto-detect it with SOA queries; validation requires it, so an auto-detect setup has to be written by hand. **A detected zone is accepted only when it is a suffix of the challenge name**; anything else is refused with the detected zone named. |
| <span id="dns-provider-config-tsig-key-name"></span>  `tsigKeyName` | Secret | - | yes | Key name exactly as configured on the nameserver, e.g. `gpm-acme`. |
| <span id="dns-provider-config-tsig-secret"></span>  `tsigSecret` | Secret | - | yes | Base64 TSIG secret, as printed by `tsig-keygen`. Use a `${ENV:...}` or `${FILE:...}` placeholder. |
| <span id="dns-provider-config-tsig-algorithm"></span>  `tsigAlgorithm` | Secret | `hmac-sha256` | no | `hmac-sha1` \| `hmac-sha224` \| `hmac-sha256` \| `hmac-sha384` \| `hmac-sha512`. `hmac-md5` is refused. |
| <span id="dns-provider-config-ttl"></span>  `ttl` | Secret | `60` | no | TTL of the challenge TXT record, 1 to 86400 seconds. |
| <span id="dns-provider-config-transport"></span>  `transport` | Secret | `tcp` | no | `tcp` or `udp`. UDP falls back to TCP when the reply is truncated. |
| <span id="dns-provider-config-timeout"></span>  `timeout` | Secret | `30s` | no | Go duration for dial, write and read on one exchange. |

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
| <span id="dns-provider-config-base-url"></span>  `baseURL` | Secret | - | yes | acme-dns API root, e.g. `https://acme-dns.example.com`. Must be `http` or `https`. A loopback, link-local, unspecified or multicast IP literal is refused at write time, and link-local is refused again at connect time. Set `allowInsecureLocal` to permit a loopback host. |
| <span id="dns-provider-config-username"></span>  `username` | Secret | - | yes | `username` from the acme-dns `/register` response (a UUID). |
| <span id="dns-provider-config-password"></span>  `password` | Secret | - | yes | `password` from `/register`. Use a `${ENV:...}` or `${FILE:...}` placeholder. |
| <span id="dns-provider-config-subdomain"></span>  `subdomain` | Secret | - | yes | `subdomain` from `/register` (a UUID); the record the CNAME points at. |
| <span id="dns-provider-config-allow-insecure-local"></span>  `allowInsecureLocal` | string | `""` | no | Set to `"true"` to allow a `baseURL` whose host is a loopback literal (an acme-dns running beside gpm). Link-local, unspecified and multicast literals are always refused. |

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
  username: c0f8ba55-0000-4000-8000-000000000001
  password: ${FILE:/run/secrets/acme_dns_password}
  subdomain: d420c923-0000-4000-8000-000000000002
```
