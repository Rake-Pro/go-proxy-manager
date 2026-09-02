# Your first host with HTTPS

Take one domain from nothing to a working, automatically renewed HTTPS host,
over HTTP-01 or DNS-01.

## Prerequisites

- A running gpm instance you can sign in to: see
  [Quickstart](quickstart.md), [Install with Docker](install-docker.md) or
  [Install the binary](install-binary.md).
- A domain whose public `A`/`AAAA` record already points at this host. gpm does
  not create that record for you on a first run; [DNS sync](../how-to/dns-sync.md)
  publishes records only once a host exists and opts in.
- For HTTP-01, inbound `tcp/80` forwarded to gpm through every router,
  firewall and load balancer in front of it. Port 80 is not optional for this
  challenge, even when the host itself only serves HTTPS.
- For DNS-01, an API token for the zone instead (no inbound port at all).
- A backend that answers HTTP on an address gpm can reach.

## Choosing a challenge

Pick the challenge that fits the deployment:

- **HTTP-01** (default when no DNS provider is referenced): no credentials at
  all, but `:80` must be reachable from the internet and the name must already
  resolve here. Single names only.
- **DNS-01**: works from behind a firewall with no inbound port, and is the only
  way to get a wildcard. Needs a DNSProvider credential
  (`cloudflare`, `digitalocean`, `hetzner`, `desec`, `rfc2136` for any
  nameserver that accepts TSIG-signed dynamic updates, or `acme-dns` for
  anything else: see
  [DNSProvider](../reference/config/dns-provider.md) for the
  `config` keys each one takes).

```yaml
# config/certificates/app.yaml - HTTP-01, no provider needed
name: app
type: acme
domains: [app.example.com]
acme: {email: admin@example.com, challenge: http-01}
```

The HTTP-01 token is served by the data plane's own `:80` listener ahead of host
routing and the force-SSL redirect, so no host or exception has to be configured
for `/.well-known/acme-challenge/`. Anything in front of gpm (a router port
forward, a cloud LB) must pass port 80 through unmodified. A CA that requires
External Account Binding (ZeroSSL, Google Public CA) takes `acme.eab.kid` +
`acme.eab.hmacKey` alongside its `directoryURL`.

## Steps

1. **Create the proxy host** with no `tls` block, so it is reachable over plain
   HTTP while you confirm routing:

   ```yaml
   # config/proxy-hosts/app.yaml
   name: app
   domains: [app.example.com]
   upstream: {scheme: http, host: 10.0.0.40, port: 8080}
   ```

2. **Confirm plain HTTP reaches the backend** before involving a CA:

   ```
   curl -sI http://app.example.com/
   ```

3. **Create the certificate** using one of the two blocks below.
4. **Wait for issuance.** The ACME manager issues on the next load and logs the
   outcome; the Certificates page shows `state` and `daysRemaining`.
5. **Attach TLS to the host** and force the redirect:

   ```yaml
   tls: {certificateRef: app, forceSSL: true}
   ```

6. **Add a second host** by repeating steps 1-5, or by pointing it at a
   wildcard certificate you already issued: one wildcard covers every
   subdomain, and no per-host issuance is needed.


## DNS-01 instead

DNS-01 needs a [DNSProvider](../reference/config/dns-provider.md) credential and
no inbound port:

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

Full walkthrough: [Issue a wildcard certificate over DNS-01](../how-to/dns-01-cloudflare.md).
For a nameserver with no native client, see
[rfc2136 and acme-dns](../how-to/generic-dns01-rfc2136-acme-dns.md).

## Verify

| Check | Expected |
|---|---|
| `GET /api/certificates` | `state: valid`, `issuer` set, `daysRemaining` near 90 |
| `curl -sI https://app.example.com/` | `200`/`3xx` and no TLS error |
| `curl -s -o /dev/null -w '%{http_code} %{redirect_url}\n' http://app.example.com/` | `308 https://app.example.com/` |
| `GET /api/health` | `certificates.valid` includes this one |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Certificate stuck at `state: pending` | The first order has not completed, or keeps failing | Read `lastError`; for HTTP-01 confirm `:80` is reachable from the internet, for DNS-01 confirm the token's zone scope |
| `curl` reports an unknown-SNI TLS error | No certificate object covers this domain | Add the domain to a certificate's `domains`, or issue one for it |
| The wrong certificate is served | Selection is by SNI, not by `tls.certificateRef` | Check which certificate's `domains` cover the name; see [Certificate](../reference/config/certificate.md) |
| HTTP-01 fails behind another proxy | Something ahead of gpm answers or rewrites `/.well-known/acme-challenge/` | Pass port 80 through unmodified, or switch to DNS-01 |
| Wildcard issuance is refused at validation | A wildcard cannot be proven over HTTP-01 | Use DNS-01 for any `*.example.com` name |
